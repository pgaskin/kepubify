package kepub

import (
	"bufio"
	"bytes"
	"cmp"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/url"
	"path"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/pgaskin/htmlstream"
	"github.com/pgaskin/htmlstream/ascii"
	"github.com/pgaskin/htmlstream/htmlutil"
	"github.com/pgaskin/htmlstream/polyglot"
	"github.com/pgaskin/htmlstream/xmlutil"
	"golang.org/x/text/encoding"
	"golang.org/x/text/encoding/htmlindex"
	xunicode "golang.org/x/text/encoding/unicode"
	"golang.org/x/text/encoding/unicode/utf32"
	"golang.org/x/text/transform"
)

// transformFileFilter returns true if a file should be filtered from the EPUB.
func transformFileFilter(fn string) bool {
	switch path.Base(fn) {
	case "calibre_bookmarks.txt": // Calibre
		return true
	case "iTunesMetadata.plist", "iTunesArtwork.plist": // iBooks
		return true
	case ".DS_Store", ".DS_STORE": // macOS
		return true
	case "thumbs.db", "Thumbs.db": // Windows
		return true
	}
	return strings.HasPrefix(fn, "__MACOSX/") // macOS archivers
}

const (
	nsOPF = "http://www.idpf.org/2007/opf"
	nsDC  = "http://purl.org/dc/elements/1.1/"
)

var (
	isCoverMeta          = htmlstream.TagNS(nsOPF, "meta", htmlstream.AttrEquals("name", "cover"))
	isCalibreMeta        = htmlstream.TagNS(nsOPF, "meta", htmlstream.AttrEquals("name", "calibre:timestamp"))
	isCalibreContributor = htmlstream.TagNS(nsDC, "contributor", htmlstream.Or(
		htmlstream.AttrEquals("role", "bkp"),
		htmlstream.AttrNSEquals(nsOPF, "role", "bkp"),
	))
)

// transformOPF transforms the OPF document for a KEPUB.
//
// See [Convert] for the list of changes.
//
// Formatting is preserved where possible, and malformed documents are
// transformed on a best-effort basis. Element matching is namespace-aware. The
// output document is in UTF-8 if possible.
func transformOPF(w io.Writer, r io.Reader) error {
	opf, err := io.ReadAll(r)
	if err != nil {
		return err
	}
	if opf, err = transformOPFUTF8(opf); err != nil {
		return err
	}

	// note: we need two passes since the ID references may be out-of-rder
	type element struct {
		depth int
		pos   htmlstream.Pos
	}
	var (
		coverID   = "cover"
		coverMeta = -1 // cover meta depth
		ids       = map[string]element{}
	)
	rd := htmlstream.NewReader(bytes.NewReader(opf))
	rd.SetMode(htmlstream.XML)
	for tok := range rd.All() {
		if tok.Kind() != htmlstream.StartTag {
			continue
		}
		depth := rd.Depth()
		if isCoverMeta(tok) && (coverMeta < 0 || depth < coverMeta) {
			coverMeta, coverID = depth, "cover"
			if a, ok := tok.Attr("content"); ok {
				coverID = a.Value()
			}
		}
		if a, ok := tok.Attr("id"); ok {
			if e, ok := ids[a.Value()]; !ok || depth < e.depth {
				ids[a.Value()] = element{depth, tok.Pos()}
			}
		}
	}
	if err := rd.Err(); err != nil {
		return err
	}
	cover, ok := ids[coverID]
	if !ok {
		cover.pos = -1
	}

	t := htmlstream.NewTransformer(w, bytes.NewReader(opf))
	t.SetMode(htmlstream.XML)
	var space indentHolder
	for tok := range t.All() {
		if tok.Kind() == htmlstream.StartTag {
			if isCalibreMeta(tok) || isCalibreContributor(tok) {
				space.Drop(t)
				t.Remove()
			} else if tok.Pos() == cover.pos {
				t.SetAttr("properties", "cover-image")
			}
		}
		space.Next(t, nil)
	}
	return t.Close()
}

// transformOPFUTF8 converts opf to UTF-8, detecting the encoding, and updating
// the declaration. Documents in unknown or unsupported encodings are returned
// unchanged (if they're ascii-compatible, they should transform fine in most
// cases).
func transformOPFUTF8(opf []byte) ([]byte, error) {
	e := xmlutil.DetectEncoding(opf)
	var enc encoding.Encoding
	switch e.Name {
	case "":
		return opf, nil // unknown
	case "UTF-8":
		// no conversion needed (and keep BOM)
		// TODO: should we remove the BOM?
	case "UTF-32BE":
		enc = utf32.UTF32(utf32.BigEndian, utf32.IgnoreBOM)
	case "UTF-32LE":
		enc = utf32.UTF32(utf32.LittleEndian, utf32.IgnoreBOM)
	default:
		var err error
		if enc, err = htmlindex.Get(e.Name); err != nil || enc == encoding.Replacement {
			return opf, nil // unsupported
		}
	}
	start := e.BOM
	if enc != nil {
		b, err := enc.NewDecoder().Bytes(opf[e.BOM:])
		if err != nil {
			return nil, fmt.Errorf("decode %s: %w", e.Name, err)
		}
		opf, start = b, 0
	}
	if e.Declared != "" {
		var name string
		if denc, err := htmlindex.Get(e.Declared); err == nil {
			name, _ = htmlindex.Name(denc)
		}
		if name != "utf-8" {
			if d, n, err := xmlutil.ParseDecl(opf[start:]); err != xmlutil.ErrNoDecl && n != 0 {
				d.Version = cmp.Or(d.Version, "1.0")
				d.Encoding = "UTF-8"
				opf = slices.Concat(opf[:start], xmlutil.AppendDecl(nil, d), opf[start+n:])
			}
		}
	}
	return opf, nil
}

type indentHolder struct {
	hold htmlstream.Hold
}

// Next stops holding the whitespace before the current token, writing raw (if
// not nil) before it, then holds the current token if it is a whitespace-only
// text token.
func (s *indentHolder) Next(t *htmlstream.Transformer, raw []byte) {
	if s.hold != (htmlstream.Hold{}) {
		if raw != nil {
			t.HoldDiscard(s.hold, append(raw, t.Held(s.hold)...))
		} else {
			t.HoldRelease(s.hold)
		}
		s.hold = htmlstream.Hold{}
	} else if raw != nil {
		t.InsertBefore(raw)
	}
	if tok := t.Token(); tok != nil && isBlankText(tok) {
		s.hold = t.Hold()
	}
}

// Held returns the whitespace before the current token.
func (s *indentHolder) Held(t *htmlstream.Transformer) []byte {
	if s.hold == (htmlstream.Hold{}) {
		return nil
	}
	return t.Held(s.hold)
}

// Drop removes the whitespace before the current token.
func (s *indentHolder) Drop(t *htmlstream.Transformer) {
	if s.hold != (htmlstream.Hold{}) {
		t.HoldDiscard(s.hold, nil)
		s.hold = htmlstream.Hold{}
	}
}

// isBlankText returns true if tok is a text token with only whitespace. If tok
// is split (i.e., it's too long), it always returns false.
func isBlankText(tok *htmlstream.Token) bool {
	return tok.Kind() == htmlstream.Text && !tok.Partial() && !tok.Continued() && ascii.IsBlank(tok.Raw())
}

// TransformContent transforms an HTML4/HTML5/XHTML1.1 document for a KEPUB.
//
//   - [important] parses the XHTML with XHTML/XML/HTML4/HTML5-compatible rules
//     Quite a few books have invalid XHTML, and this prevents the markup from
//     being mangled any more than absolutely necessary. This also has the side
//     effect of fixing bad markup when combined with the render step at the end.
//     The intention is to match or exceed the kepub renderer's leniency. This
//     lenient parsing is also why kepubify often works better with badly-formed
//     HTML than Calibre.
//
//     The document is mostly parsed as HTML5, except that self-closing non-void
//     elements, CDATA, and other XML-isms are accepted, and UTF-8 BOMs are
//     ignored.
//
//   - [mandatory] add Kobo style tweaks
//     To match official KEPUBs.
//
//   - [mandatory] add Kobo div wrappers
//     To match official KEPUBs. Kobo wraps the body with two div tags,
//     `div#book-columns > div#book-inner`, to provide a target for applying
//     pagination styles.
//
//   - [mandatory] add Kobo spans
//     To match official KEPUBs. Kobo adds spans surrounding each fragment (see
//     the regexp and matching logic) to provide better references to chunks of
//     text. Highlighting, bookmarking, and other related features don't work
//     without this.
//
//   - [optional] add extra CSS
//     For customization or to fix common issues.
//
//   - [optional] smarten punctuation
//     A common tweak to improve badly-formatted books.
//
//   - [extra] content cleanup
//     Removes Adept tags, extraneous MS Office tags, Unicode replacement chars,
//     etc.
//
//   - [important] renders the HTML as polyglot XHTML/HTML5
//     The HTML is rendered for maximum compatibility and to be as close to the
//     original HTML (the way it would be parsed by browsers) as possible. See
//     pgaskin.net/htmlstream/polyglottify for more information about how this
//     works.
//
//   - [optional] find/replace
//     To allow users to apply quick one-off fixes to the generated HTML.
//
//   - [important] ensure charset is UTF-8
//     EPUBs (and KEPUBs by extension) must be UTF-8/UTF-16.
//
// The document is streamed, so existing Kobo divs and spans are only detected
// if they are close to the start of the body (which they should be).
func TransformContent(w io.Writer, r io.Reader, opt Options) error {
	return transformContent(w, r, opt, "")
}

// transformContent is TransformContent for the content document at the path
// name in the EPUB, for Warn.
func transformContent(w io.Writer, r io.Reader, opt Options, name string) error {
	r, err := transformContentCharset(r, opt.Charset)
	if err != nil {
		return fmt.Errorf("parse html: %w", err)
	}

	var wc io.WriteCloser
	if len(opt.Replace) != 0 {
		wc = transformContentReplacements(w, opt.Replace)
		w = wc
	}

	k := newKobo(opt)
	popt := polyglot.Options{
		DoctypeMode: polyglot.KeepDoctype,
		XMLDecl:     polyglot.KeepXMLDecl, // only at the start (see #95)
		OnTag:       k.OnTag,
		OnText:      k.OnText,
	}
	if opt.Warn != nil {
		popt.Warn = func(w polyglot.Warning) {
			opt.Warn(name, w.Msg)
		}
	}
	if err := convert(w, r, popt); err != nil {
		return fmt.Errorf("transform html: %w", err)
	}

	if wc != nil {
		if err := wc.Close(); err != nil {
			return fmt.Errorf("render html: %w", err)
		}
	}

	return nil
}

// transformContentCharset returns a reader decoding r to UTF-8 from the
// specified encoding, or the detected encoding if "auto" or empty (same prescan
// rules as browsers), or otherwise guessed (UTF-8 if valid, or
// latin1/windows-1252).
func transformContentCharset(r io.Reader, label string) (io.Reader, error) {
	switch strings.ToLower(label) {
	case "utf-8":
		return r, nil
	case "auto", "":
		br := bufio.NewReaderSize(r, 1024)
		head, err := br.Peek(1024)
		if err != nil && err != io.EOF {
			return nil, fmt.Errorf("detect charset: %w", err)
		}
		name, ok := htmlutil.PrescanCharset(head)
		if !ok {
			// note: ignore a partial rune at the end
			if i := len(head) - utf8.UTFMax; i > 0 {
				for j := len(head) - 1; j > i; j-- {
					if utf8.RuneStart(head[j]) {
						head = head[:j]
						break
					}
				}
			}
			if name = "windows-1252"; utf8.Valid(head) {
				name = "utf-8"
			}
		}
		enc, err := htmlindex.Get(name)
		if err != nil {
			return nil, fmt.Errorf("detect charset: %w", err)
		}
		// note: BOMOverride decodes UTF-8 or UTF-16 instead if there is a
		// byte order mark, and removes it
		return transform.NewReader(br, xunicode.BOMOverride(enc.NewDecoder())), nil
	default:
		enc, err := htmlindex.Get(label)
		if err != nil {
			return nil, fmt.Errorf("invalid charset %q", label)
		}
		return enc.NewDecoder().Reader(r), nil
	}
}

// transformContentReplacements returns a writer which replaces each find with
// the corresponding replace in the data written to w, in order (each
// replacement applies to the output of the previous ones). Empty finds are
// ignored.
func transformContentReplacements(w io.Writer, repl []Replacement) io.WriteCloser {
	var rw *replaceWriter
	out := w
	for i := len(repl) - 1; i >= 0; i-- {
		if repl[i].Find != "" {
			rw = &replaceWriter{w: out, next: rw, find: []byte(repl[i].Find), repl: []byte(repl[i].Replace)}
			out = rw
		}
	}
	if rw == nil {
		return nopWriteCloser{w}
	}
	return rw
}

const (
	dummyTitlepageID   = "kepubify-titlepage-dummy"
	dummyTitlepageHref = dummyTitlepageID + ".xhtml"
	dummyTitlepage     = `<!DOCTYPE html><html xmlns="http://www.w3.org/1999/xhtml" lang="en"><head><title></title></head><body><p style="text-align: center; margin: 4em 0; font-size: .7em; font-style: italic;">Page intentionally left blank by kepubify.</p></body></html>`
)

// transformDummyTitlepage adds a dummy titlepage if forced or the heuristic
// determines that is is necessary. If there was an error determining if the
// titlepage is required, false and an error is returned. If it is not required,
// false is returned. If it is required, but there was an error when adding it,
// true and an error is returned.  If it was added successfully, the filename
// and contents of the content document to add to the epub and true is returned.
//
// The heuristic determines whether the first content file in the spine is a
// title page without other content, and if it isn't (or if force is true), it
// adds a blank content document and modifies the OPF to add it to the manifest
// and start of the spine. This is required because Kobo will treat the first
// spine entry specially (e.g. no margins) for full-screen book covers. See #33.
//
// Note that the heuristic is subject to change between kepubify versions.
//
// The heuristic counts the paragraphs, images and words in the body of the
// first linear spine item, as transformed by TransformContent. Formatting is
// preserved where possible, and malformed documents are transformed on a
// best-effort basis. Element matching is namespace-aware.
//
// If detectCover is true, the dummy titlepage shows the cover image if one is
// found (see transformDummyTitlepageCover) instead of being blank.
func transformDummyTitlepage(epub fs.FS, opfF string, opf *bytes.Buffer, force *bool, detectCover bool) (string, io.Reader, bool, error) {
	switch {
	case force == nil:
		if req, err := transformDummyTitlepageRequired(epub, opfF, opf.Bytes()); err != nil {
			return "", nil, false, fmt.Errorf("check if dummy titlepage is required: %w", err)
		} else if !req {
			return "", nil, false, nil
		}
	case !*force:
		return "", nil, false, nil
	}
	var b bytes.Buffer
	if err := transformDummyTitlepageAdd(&b, opf.Bytes()); err != nil {
		return "", nil, true, fmt.Errorf("apply title page fix: %w", err)
	}
	page := dummyTitlepage
	if detectCover {
		if cover := transformDummyTitlepageCover(epub, opfF, opf.Bytes()); cover != "" {
			page = dummyTitlepageWithCover(relativeHref(path.Dir(opfF), cover))
		}
	}
	opf.Reset()
	opf.Write(b.Bytes())
	return path.Join(path.Dir(opfF), dummyTitlepageHref), strings.NewReader(page), true, nil
}

// dummyTitlepageWithCover returns a dummy titlepage showing the image at href.
func dummyTitlepageWithCover(href string) string {
	b := []byte(`<!DOCTYPE html><html xmlns="http://www.w3.org/1999/xhtml" lang="en"><head><title></title><style type="text/css">html, body { height: 100%; margin: 0; padding: 0; } body { text-align: center; } img { max-width: 100%; max-height: 100%; }</style></head><body>`)
	b = htmlstream.AppendStartTag(b, "img", true, "src", href, "alt", "")
	b = append(b, `</body></html>`...)
	return string(b)
}

// transformDummyTitlepageCover returns the path in epub of the cover image of
// the package document opf at opfF, or "" if none is found.
//
// The cover image is the manifest item with the cover-image property, if it is
// an image which exists. Otherwise, it is the first image file (by
// extension) whose name (without the extension, case-insensitive) is cover,
// starts with cover_, ends with _cover, ends with cover, or contains cover, in
// that order of preference, preferring the ones in the directory of the
// package document over the others for each.
func transformDummyTitlepageCover(epub fs.FS, opfF string, opf []byte) string {
	// TODO: add the detected cover to the manifest if not already listed? it
	// should work fine as-is, though it is technically supposed to be listed
	// there

	isImage := func(name string) bool {
		switch strings.ToLower(path.Ext(name)) {
		case ".jpg", ".jpeg", ".png", ".gif":
			return true
		}
		return false
	}

	// note: transformOPF sets the cover-image property on the cover
	rd := htmlstream.NewReader(bytes.NewReader(opf))
	rd.SetMode(htmlstream.XML)
	var manifest bool
	for tok := range rd.All() {
		switch {
		case tok.Kind() == htmlstream.EndTag && rd.Depth() == 2:
			manifest = false
		case tok.Kind() != htmlstream.StartTag || tok.NS() != nsOPF:
		case rd.Depth() == 2:
			manifest = tok.Opens() && string(tok.Local()) == "manifest"
		case rd.Depth() == 3 && manifest && string(tok.Local()) == "item":
			if !slices.Contains(strings.Fields(attrValue(tok, "properties")), "cover-image") {
				continue
			}
			href := attrValue(tok, "href")
			if p, err := url.PathUnescape(href); err == nil {
				href = p
			}
			name := path.Join(path.Dir(opfF), href)
			if isImage(name) {
				if _, err := fs.Stat(epub, name); err == nil {
					return name
				}
			}
		}
	}

	var images []string
	fs.WalkDir(epub, ".", func(name string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() && isImage(name) && !transformFileFilter(name) {
			images = append(images, name)
		}
		return nil
	})
	slices.Sort(images)
	for _, match := range []func(base string) bool{
		func(base string) bool { return base == "cover" },
		func(base string) bool { return strings.HasPrefix(base, "cover_") },
		func(base string) bool { return strings.HasSuffix(base, "_cover") },
		func(base string) bool { return strings.HasSuffix(base, "cover") },
		func(base string) bool { return strings.Contains(base, "cover") },
	} {
		for _, inPackage := range []bool{true, false} {
			for _, name := range images {
				if inPackage && path.Dir(name) != path.Dir(opfF) {
					continue
				}
				base := strings.ToLower(path.Base(name))
				if match(strings.TrimSuffix(base, path.Ext(base))) {
					return name
				}
			}
		}
	}
	return ""
}

// relativeHref returns a relative URL referencing the slash-separated path
// name from a document in the directory dir.
func relativeHref(dir, name string) string {
	// TODO: refactor?
	var from []string
	if dir = path.Clean(dir); dir != "." {
		from = strings.Split(dir, "/")
	}
	to := strings.Split(name, "/")
	i := 0
	for i < len(from) && i < len(to)-1 && from[i] == to[i] {
		i++
	}
	rel := strings.Repeat("../", len(from)-i) + strings.Join(to[i:], "/")
	if first, _, _ := strings.Cut(rel, "/"); strings.Contains(first, ":") {
		rel = "./" + rel // not a scheme
	}
	return (&url.URL{Path: rel}).EscapedPath()
}

func transformDummyTitlepageRequired(epub fs.FS, opfF string, opf []byte) (bool, error) {
	type item struct {
		id   string
		href string
	}
	var (
		items  []item
		idref  string
		linear bool   // first linear spine item found
		root   bool   // root element found
		parent string // local name of open child element of package element
	)
	rd := htmlstream.NewReader(bytes.NewReader(opf))
	rd.SetMode(htmlstream.XML)
	for tok := range rd.All() {
		if tok.Kind() == htmlstream.EndTag {
			if rd.Depth() == 1 {
				break // the end of the package element
			}
			if rd.Depth() == 2 {
				parent = ""
			}
			continue
		}
		if tok.Kind() != htmlstream.StartTag {
			continue
		}
		local := string(tok.Local())
		if tok.NS() != nsOPF {
			local = ""
		}
		switch rd.Depth() {
		case 1:
			if local != "package" {
				return false, fmt.Errorf("root element %s isn't an OPF package element", tok.Name())
			}
			root = true
		case 2:
			if tok.Opens() {
				parent = local
			}
		case 3:
			switch {
			case parent == "manifest" && local == "item":
				items = append(items, item{attrValue(tok, "id"), attrValue(tok, "href")})
			case parent == "spine" && local == "itemref" && !linear:
				if attrValue(tok, "linear") != "no" {
					linear, idref = true, attrValue(tok, "idref")
				}
			}
		}
	}
	if err := rd.Err(); err != nil {
		return false, err
	}
	if !root {
		return false, errors.New("no root element")
	}
	if idref == "" {
		return false, nil // no linear spine item, or one without an idref
	}

	var href string
	for _, it := range items {
		if it.id != idref {
			continue
		}
		switch strings.ToLower(path.Ext(it.href)) {
		case ".htm", ".html", ".xhtml":
			href = it.href
		default:
			return false, nil // not a content document
		}
	}
	if href == "" {
		return false, nil // not in the manifest
	}
	if p, err := url.PathUnescape(href); err == nil {
		href = p
	}
	if n := strings.ToLower(path.Base(href)); strings.Contains(n, "cover") || strings.Contains(n, "title") {
		return false, nil // a cover or title page
	}
	f, err := epub.Open(path.Join(path.Dir(opfF), href))
	if err != nil {
		return false, nil // a missing file
	}
	defer f.Close()
	x, err := isTitlepage(f)
	if err != nil {
		return false, nil // if unparseable, assume it's a titlepage
	}
	return !x, nil
}

func attrValue(tok *htmlstream.Token, name string) string {
	if a, ok := tok.Attr(name); ok {
		return a.Value()
	}
	return ""
}

func isTitlepage(r io.Reader) (bool, error) {
	var errNotTitlepage = errors.New("not title page") // sentinel error so we can stop reading

	// note: we use polyglot so it gets parsed with proper html rules
	var (
		paras, images, words int
		word                 int  // length of current word
		body                 bool // in body
		skip                 int  // id of skipped element
	)
	err := convert(io.Discard, r, polyglot.Options{
		OnTag: func(e *polyglot.Tag) {
			html := e.NS == htmlstream.NamespaceXHTML
			switch {
			case skip != 0:
				if e.ID == skip && !e.Start {
					skip = 0
				}
			case html && e.Local == "body":
				body = e.Start
			case !body || !e.Start:
			case html && e.Local == "p":
				if paras++; paras > 4 {
					e.Writer().Fail(errNotTitlepage)
				}
				skip = e.ID // don't count words/images in p elements (e.g., cover image in p)
			case html && (e.Local == "img" || e.Local == "image"), e.NS == htmlstream.NamespaceSVG:
				images++
				skip = e.ID
			case html && (e.Local == "script" || e.Local == "style" || e.Local == "pre" || e.Local == "audio" || e.Local == "video"), e.NS == htmlstream.NamespaceMathML:
				skip = e.ID
			}
		},
		OnText: func(t *polyglot.Text) {
			if skip != 0 || !body {
				return
			}
			for s := t.Data; len(s) != 0; {
				r, n := utf8.DecodeRune(s)
				if unicode.IsSpace(r) {
					if word > 3 {
						words++
					}
					word = 0
				} else {
					word += n
				}
				s = s[n:]
			}
			if t.Last {
				if word > 3 {
					words++
				}
				word = 0
			}
			if words > 20 {
				t.Writer().Fail(errNotTitlepage)
			}
		},
	})
	if errors.Is(err, errNotTitlepage) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return !(images == 0 && words < 5 || images > 4), nil
}

// transformDummyTitlepageAdd writes opf to w, adding the dummy titlepage
// manifest/spine items.
func transformDummyTitlepageAdd(w io.Writer, opf []byte) error {
	t := htmlstream.NewTransformer(w, bytes.NewReader(opf))
	t.SetMode(htmlstream.XML)
	var (
		space    indentHolder
		root     bool                // root element found
		pkg      bool                // package element in root element
		manifest bool                // first manifest element found
		spine    bool                // first spine element found
		open     htmlstream.Pos = -1 // position of the first manifest element (while open)
		item     []byte              // manifest item to add before the end tag of the manifest element
		last     []byte              // whitespace before the last child element of the manifest element
		itemref  []byte              // spine item to add before the token after the spine start tag
	)
	for tok := range t.All() {
		if itemref != nil {
			// start of spine, after whitespace
			if isBlankText(tok) {
				t.InsertAfter(append(itemref, tok.Raw()...))
			} else {
				t.InsertBefore(itemref)
			}
			itemref = nil
		}
		var before []byte // stuff to add before whitespace before tok
		switch tok.Kind() {
		case htmlstream.StartTag:
			switch depth := t.Depth(); {
			case depth == 1 && !root:
				root = true
				pkg = tok.NS() == nsOPF && string(tok.Local()) == "package"
			case depth == 2 && pkg && tok.NS() == nsOPF && string(tok.Local()) == "manifest" && !manifest:
				manifest = true
				item = appendSibling(nil, tok, "item", "id", dummyTitlepageID, "href", dummyTitlepageHref, "media-type", "application/xhtml+xml")
				if tok.Opens() {
					open = tok.Pos()
				} else {
					t.ExpandSelfClosing()
					t.InsertAfter(item)
				}
			case depth == 2 && pkg && tok.NS() == nsOPF && string(tok.Local()) == "spine" && !spine:
				spine = true
				itemref = appendSibling(nil, tok, "itemref", "idref", dummyTitlepageID)
				if !tok.Opens() {
					t.ExpandSelfClosing()
					t.InsertAfter(itemref)
					itemref = nil
				}
			case depth == 3 && open >= 0:
				last = append(last[:0], space.Held(t)...)
			}
		case htmlstream.EndTag:
			if tok.StartPos() == open {
				// end of the manifest, same indentation as items (or end tag if
				// none)
				if len(last) == 0 {
					last = space.Held(t)
				}
				before = append(append(before, last...), item...)
				open = -1
			}
			if t.Depth() == 1 {
				pkg = false
			}
		}
		space.Next(t, before)
	}
	return t.Close()
}

// appendSibling appends an empty element named local with the specified
// attributes (name/value pairs) to dst, with the same namespace prefix as tok.
func appendSibling(dst []byte, tok *htmlstream.Token, local string, attrs ...string) []byte {
	if p := tok.Prefix(); p != nil {
		local = string(p) + ":" + local
	}
	return htmlstream.AppendStartTag(dst, local, true, attrs...)
}

// convert reads a content document from r, and writes filtered polyglot XHTML
// to w.
func convert(w io.Writer, r io.Reader, popt polyglot.Options) (err error) {
	// note: we parse with HTML rules, but with lenient parsing of XML-isms
	popt.XMLSyntax = true
	pw := polyglot.NewWriter(w, popt)
	f := filter{pw: pw}
	rd := popt.NewReader(r)
	defer func() {
		pw.Fail(rd.Err()) // a read error comes first
		pw.Fail(err)      // nothing more is written after an error
		if cerr := pw.Close(); cerr != nil {
			err = cerr
		}
	}()
	for tok := range rd.All() {
		if err := f.Filter(tok); err != nil {
			return err
		}
	}
	return nil // the deferred Close ends the document and sets the error
}

// filter filters content document tokens, skipping adept meta elements,
// whitespace/comment-only o:p/st1:* elements, and the style elements added by
// Kobo or a previous conversion (which are added again by kobo).
type filter struct {
	pw *polyglot.Writer

	// skipped style element
	skipping bool
	skipPos  htmlstream.Pos // of its start tag

	// held empty o:p, st1:*, or Adept meta element
	held    htmlstream.TokenBuffer
	holding bool
	holdPos htmlstream.Pos // of its start tag

	buf []byte // decoded text of a held text token
}

func (f *filter) Filter(tok *htmlstream.Token) error {
	if f.skipping {
		if tok.Kind() == htmlstream.EndTag && tok.StartPos() == f.skipPos {
			f.skipping = false
		}
		return nil
	}
	if f.holding {
		switch kind := tok.Kind(); {
		case kind == htmlstream.EndTag && tok.StartPos() == f.holdPos:
			// held token is empty, drop it
			f.holding = false
			f.held.Reset()
			return nil
		case f.held.Size() >= holdMax:
		case kind == htmlstream.Comment,
			kind == htmlstream.Text && f.isSpace(tok):
			f.held.Append(tok)
			return nil
		}
		f.holding = false
		for t := range f.held.All() {
			if err := f.pw.WriteToken(t); err != nil {
				return err
			}
		}
		f.held.Reset()
	}
	if tok.Kind() == htmlstream.StartTag && !tok.Incomplete() && tok.NS() == htmlstream.NamespaceXHTML {
		switch {
		case (isAdeptMeta(tok) || isMSOTag(tok)) && !tok.Opens():
			return nil // nothing left for self-closing o:p or meta
		case isAdeptMeta(tok), isMSOTag(tok):
			// if xml syntax, might be open and closed by (possibly implied) end tag
			f.holding, f.holdPos = true, tok.Pos()
			f.held.Append(tok)
			return nil
		case isKoboStyle(tok):
			if tok.Opens() {
				f.skipping, f.skipPos = true, tok.Pos()
			}
			return nil
		}
	}
	return f.pw.WriteToken(tok)
}

// isSpace returns true if the decoded text token is whitespace.
func (f *filter) isSpace(tok *htmlstream.Token) bool {
	f.buf = tok.AppendText(f.buf[:0])
	return isOnlyUnicodeSpace(f.buf)
}

var isAdeptMeta = htmlstream.Tag("meta", htmlstream.Or(
	htmlstream.AttrEquals("name", "Adept.expected.resource"),
	htmlstream.AttrEquals("name", "Adept.resource"),
))

// isKoboStyle matches the style elements added by Kobo (kobostylehacks by id)
// or kepubify (kobostylehacks or kepubify-* by class), see newKobo.
var isKoboStyle = htmlstream.Tag("style", htmlstream.Or(
	htmlstream.AttrEquals("id", "kobostylehacks"),
	htmlstream.HasClass("kobostylehacks"),
	htmlstream.AttrFunc("class", func(v []byte) bool {
		for _, class := range bytes.Fields(v) {
			if bytes.HasPrefix(class, []byte("kepubify-")) {
				return true
			}
		}
		return false
	}),
))

func isMSOTag(tok *htmlstream.Token) bool {
	var b [16]byte
	n := tok.AppendLocalName(b[:0])
	return string(n) == "o:p" || bytes.HasPrefix(n, []byte("st1:"))
}

// replaceWriter replaces a string in the data written to it, before writing it
// to w.
type replaceWriter struct {
	w          io.Writer
	next       *replaceWriter // w, to close
	find, repl []byte
	buf, out   []byte
}

func (rw *replaceWriter) Write(p []byte) (int, error) {
	rw.buf = append(rw.buf, p...)
	if err := rw.flush(false); err != nil {
		return 0, err
	}
	return len(p), nil
}

// flush writes the buffered data, except for a suffix which could be the
// start of a match unless final.
func (rw *replaceWriter) flush(final bool) error {
	b := rw.buf
	out := rw.out[:0]
	for {
		i := bytes.Index(b, rw.find)
		if i < 0 {
			break
		}
		out = append(append(out, b[:i]...), rw.repl...)
		b = b[i+len(rw.find):]
	}
	keep := 0
	if !final {
		keep = min(len(b), len(rw.find)-1)
	}
	out = append(out, b[:len(b)-keep]...)
	rw.buf = append(rw.buf[:0], b[len(b)-keep:]...)
	rw.out = out
	if len(out) == 0 {
		return nil
	}
	_, err := rw.w.Write(out)
	return err
}

// Close writes the rest of the data and closes the next writer in the chain.
func (rw *replaceWriter) Close() error {
	if err := rw.flush(true); err != nil {
		return err
	}
	if rw.next != nil {
		return rw.next.Close()
	}
	return nil
}

type nopWriteCloser struct {
	io.Writer
}

func (nopWriteCloser) Close() error {
	return nil
}
