package kepub

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"strings"
	"unicode/utf8"

	"github.com/pgaskin/htmlstream"
	"github.com/pgaskin/htmlstream/htmlutil"
	"github.com/pgaskin/htmlstream/polyglot"
	"golang.org/x/text/encoding/htmlindex"
	xunicode "golang.org/x/text/encoding/unicode"
	"golang.org/x/text/transform"
)

const cssHyphenate = `* {
    -webkit-hyphens: auto;
    -moz-hyphens: auto;
    hyphens: auto;

    -webkit-hyphenate-limit-after: 3;
    -webkit-hyphenate-limit-before: 3;
    -webkit-hyphenate-limit-lines: 2;
}

h1, h2, h3, h4, h5, h6, td,
h1 *, h2 *, h3 *, h4 *, h5 *, h6 *, td * {
    -moz-hyphens: none !important;
    -webkit-hyphens: none !important;
    hyphens: none !important;
}`

const cssNoHyphenate = `* {
    -moz-hyphens: none !important;
    -webkit-hyphens: none !important;
    hyphens: none !important;
}`

const cssFullScreenFixes = `body {
    margin: 0 !important;
    padding: 0 !important;
}

body>div {
    padding-left: 0.2em !important;
    padding-right: 0.2em !important;
}`

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
