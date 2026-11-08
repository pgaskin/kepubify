package kepub

import (
	"bytes"
	"cmp"
	"fmt"
	"io"
	"slices"

	"github.com/pgaskin/htmlstream"
	"github.com/pgaskin/htmlstream/ascii"
	"github.com/pgaskin/htmlstream/xmlutil"
	"golang.org/x/text/encoding"
	"golang.org/x/text/encoding/htmlindex"
	"golang.org/x/text/encoding/unicode/utf32"
)

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
	isPackage        = htmlstream.TagNS(nsOPF, "package")
	isManifest       = htmlstream.TagNS(nsOPF, "manifest")
	isManifestItem   = htmlstream.TagNS(nsOPF, "item")
	isGuideReference = htmlstream.TagNS(nsOPF, "reference")
)

// transformOPF transforms the OPF document for a KEPUB. If m is not nil, the
// guide path references are rewritten to the manifest's encoding.
//
// See [Convert] for the list of changes.
//
// Formatting is preserved where possible, and malformed documents are
// transformed on a best-effort basis. Element matching is namespace-aware. The
// output document is in UTF-8 if possible.
func transformOPF(w io.Writer, r io.Reader, m *manifestHrefs) error {
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
			} else {
				if tok.Pos() == cover.pos {
					t.SetAttr("properties", "cover-image")
				}
				switch a, ok := tok.Attr("href"); {
				case !ok:
				case isManifestItem(tok):
					if href := escapeHref(a.Value()); href != a.Value() {
						t.SetAttr("href", href)
					}
				case isGuideReference(tok) && m != nil:
					if href, _ := m.Fix(m.opf, a.Value()); href != a.Value() {
						t.SetAttr("href", href)
					}
				}
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

func attrValue(tok *htmlstream.Token, name string) string {
	if a, ok := tok.Attr(name); ok {
		return a.Value()
	}
	return ""
}
