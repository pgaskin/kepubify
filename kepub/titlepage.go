package kepub

import (
	"bytes"
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
	"github.com/pgaskin/htmlstream/polyglot"
)

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
// The options used are DummyTitlepage, DetectCover (if true, the dummy
// titlepage shows the cover image if one is found, see
// transformDummyTitlepageCover, instead of being blank), and Charset (for
// reading the first content document).
func transformDummyTitlepage(epub fs.FS, opfF string, opf *bytes.Buffer, opt Options) (string, io.Reader, bool, error) {
	switch force := opt.DummyTitlepage; {
	case force == nil:
		if req, err := transformDummyTitlepageRequired(epub, opfF, opf.Bytes(), opt.Charset); err != nil {
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
	if opt.DetectCover {
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
			name := epubHref(epub, opfF, attrValue(tok, "href"))
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

func transformDummyTitlepageRequired(epub fs.FS, opfF string, opf []byte, charset string) (bool, error) {
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
	name := epubHref(epub, opfF, href)
	if n := strings.ToLower(path.Base(name)); strings.Contains(n, "cover") || strings.Contains(n, "title") {
		return false, nil // a cover or title page
	}
	f, err := epub.Open(name)
	if err != nil {
		return false, nil // a missing file
	}
	defer f.Close()
	r, err := transformContentCharset(f, charset)
	if err != nil {
		return false, err
	}
	x, err := isTitlepage(r)
	if err != nil {
		return false, nil // if unparseable, assume it's a titlepage
	}
	return !x, nil
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
