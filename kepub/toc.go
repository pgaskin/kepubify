package kepub

import (
	"bytes"
	"io"
	"io/fs"
	"net/url"
	"path"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/pgaskin/htmlstream"
)

const nsNCX = "http://www.daisy.org/z3986/2005/ncx/"

var isNCXContent = htmlstream.TagNS(nsNCX, "content")

// manifestHrefs maps the files of the manifest items to their hrefs, to rewrite
// references to them with the same encoding as the manifest (see #101).
type manifestHrefs struct {
	opf      string            // name of the OPF
	ncx, nav string            // names of the NCX and nav document, if any
	hrefs    map[string]string // name -> href (with escapeHref) relative to the OPF
	lower    map[string]string // lowercase name -> name, or "" if ambiguous
}

// newManifestHrefs parses the manifest of the OPF named opf in epub.
func newManifestHrefs(epub fs.FS, opf string, data []byte) (*manifestHrefs, error) {
	data, err := transformOPFUTF8(data)
	if err != nil {
		return nil, err
	}
	m := &manifestHrefs{opf: opf, hrefs: map[string]string{}, lower: map[string]string{}}
	var (
		parent string                // local name of open child element of package element
		toc    string                // spine toc idref
		ids    = map[string]string{} // id -> name
		ncx    string                // first item with the NCX media type
	)
	rd := htmlstream.NewReader(bytes.NewReader(data))
	rd.SetMode(htmlstream.XML)
	for tok := range rd.All() {
		switch {
		case tok.Kind() == htmlstream.EndTag && rd.Depth() == 2:
			parent = ""
		case tok.Kind() != htmlstream.StartTag || tok.NS() != nsOPF:
		case rd.Depth() == 2:
			if parent = ""; tok.Opens() {
				parent = string(tok.Local())
			}
			if parent == "spine" {
				toc = attrValue(tok, "toc")
			}
		case rd.Depth() == 3 && parent == "manifest" && string(tok.Local()) == "item":
			a, ok := tok.Attr("href")
			if !ok {
				continue
			}
			href := escapeHref(a.Value())
			name := epubHref(epub, opf, href)
			if _, ok := m.hrefs[name]; !ok {
				m.hrefs[name] = href
				l := strings.ToLower(name)
				if _, ok := m.lower[l]; ok {
					m.lower[l] = "" // ambiguous
				} else {
					m.lower[l] = name
				}
			}
			if id := attrValue(tok, "id"); id != "" {
				ids[id] = name
			}
			if ncx == "" && attrValue(tok, "media-type") == "application/x-dtbncx+xml" {
				ncx = name
			}
			if m.nav == "" && slices.Contains(strings.Fields(attrValue(tok, "properties")), "nav") {
				m.nav = name
			}
		}
	}
	if err := rd.Err(); err != nil {
		return nil, err
	}
	if m.ncx = ids[toc]; m.ncx == "" {
		m.ncx = ncx
	}
	return m, nil
}

// Fix returns href, in the file named base, encoded like the manifest if it
// refers to a manifest item (case-insensitively if only one matches). It
// returns false if it is a relative reference to a file which isn't in the
// manifest.
func (m *manifestHrefs) Fix(base, href string) (string, bool) {
	p, frag, hasFrag := strings.Cut(href, "#")
	if p == "" || strings.HasPrefix(p, "/") || strings.Contains(p, "?") || hasScheme(p) {
		return href, true
	}
	dir := path.Dir(base)
	names := []string{path.Join(dir, p)}
	if d, err := url.PathUnescape(p); err == nil && d != p {
		names = append([]string{path.Join(dir, d)}, names...)
	}
	fixed, ok := func() (string, bool) {
		for _, lower := range []bool{false, true} {
			for _, name := range names {
				if lower {
					name = m.lower[strings.ToLower(name)]
				}
				if h, ok := m.hrefs[name]; ok && name != "" {
					return h, true
				}
			}
		}
		return "", false
	}()
	if !ok {
		return href, false
	}
	if opfDir := path.Dir(m.opf); dir != opfDir {
		fixed = rebaseHref(opfDir, fixed, dir)
	}
	if hasFrag {
		fixed += "#" + frag
	}
	return fixed, true
}

// rebaseHref returns href, a relative URL from a document in the directory
// from, relative to a document in the directory to, keeping the encoding of the
// path segments of href.
func rebaseHref(from, href, to string) string {
	type segment struct{ name, raw string }
	split := func(dir string) (s []segment) {
		if dir = path.Clean(dir); dir != "." {
			for name := range strings.SplitSeq(dir, "/") {
				s = append(s, segment{name, escapeSegment(name)})
			}
		}
		return s
	}
	abs := split(from)
	for raw := range strings.SplitSeq(href, "/") {
		switch raw {
		case "", ".":
		case "..":
			if len(abs) != 0 {
				abs = abs[:len(abs)-1]
			}
		default:
			name := raw
			if d, err := url.PathUnescape(raw); err == nil {
				name = d
			}
			abs = append(abs, segment{name, raw})
		}
	}
	dir := split(to)
	i := 0
	for i < len(dir) && i < len(abs)-1 && dir[i].name == abs[i].name {
		i++
	}
	var b strings.Builder
	for range len(dir) - i {
		b.WriteString("../")
	}
	if b.Len() == 0 && i < len(abs) && strings.Contains(abs[i].raw, ":") {
		b.WriteString("./") // not a scheme
	}
	for j, s := range abs[i:] {
		if j != 0 {
			b.WriteByte('/')
		}
		b.WriteString(s.raw)
	}
	return b.String()
}

// escapeSegment returns a URL path segment for the file name.
func escapeSegment(name string) string {
	return escapeHref(strings.NewReplacer("%", "%25", "?", "%3F", "#", "%23", "/", "%2F").Replace(name))
}

// hasScheme returns true if href starts with a URL scheme.
func hasScheme(href string) bool {
	for i := 0; i < len(href); i++ {
		switch c := href[i]; {
		case 'a' <= c && c <= 'z', 'A' <= c && c <= 'Z':
		case '0' <= c && c <= '9', c == '+', c == '-', c == '.':
			if i == 0 {
				return false
			}
		case c == ':':
			return i != 0
		default:
			return false
		}
	}
	return false
}

// escapeHref percent-encodes the characters of href which aren't allowed in a
// URL (e.g., spaces), leaving the others (including non-ASCII characters and
// percent-encoded sequences) as-is.
func escapeHref(href string) string {
	const hex = "0123456789ABCDEF"
	isHex := func(c byte) bool {
		return '0' <= c && c <= '9' || 'a' <= c && c <= 'f' || 'A' <= c && c <= 'F'
	}
	var b []byte
	for i := 0; i < len(href); i++ {
		c := href[i]
		switch {
		case 'a' <= c && c <= 'z', 'A' <= c && c <= 'Z', '0' <= c && c <= '9',
			strings.IndexByte("-._~!$&'()*+,;=:@/?#", c) >= 0,
			c >= utf8.RuneSelf,
			c == '%' && i+2 < len(href) && isHex(href[i+1]) && isHex(href[i+2]):
			if b != nil {
				b = append(b, c)
			}
		default:
			if b == nil {
				b = append(make([]byte, 0, len(href)+8), href[:i]...)
			}
			b = append(b, '%', hex[c>>4], hex[c&15])
		}
	}
	if b == nil {
		return href
	}
	return string(b)
}

// transformNCX rewrites the content element src attributes of the NCX named
// name which refer to manifest items to the manifest's encoding. It returns
// false if nothing was changed. The output document is in UTF-8 if possible.
//
// Formatting is preserved where possible, and malformed documents are
// transformed on a best-effort basis.
func transformNCX(w io.Writer, r io.Reader, name string, m *manifestHrefs, warn func(name, href string)) (bool, error) {
	ncx, err := io.ReadAll(r)
	if err != nil {
		return false, err
	}
	if ncx, err = transformOPFUTF8(ncx); err != nil {
		return false, err
	}
	var changed bool
	t := htmlstream.NewTransformer(w, bytes.NewReader(ncx))
	t.SetMode(htmlstream.XML)
	for tok := range t.All() {
		if tok.Kind() != htmlstream.StartTag || !isNCXContent(tok) {
			continue
		}
		a, ok := tok.Attr("src")
		if !ok {
			continue
		}
		src := a.Value()
		if fixed, ok := m.Fix(name, src); !ok {
			warn(name, src)
		} else if fixed != src {
			t.SetAttr("src", fixed)
			changed = true
		}
	}
	return changed, t.Close()
}
