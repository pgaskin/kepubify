package kepub

import (
	"bytes"
	"strings"
	"testing"
	"testing/fstest"
)

func TestEscapeHref(t *testing.T) {
	for _, tc := range [][2]string{
		{"", ""},
		{"Text/ch1.html", "Text/ch1.html"},
		{"Text/My Book_split_001.html", "Text/My%20Book_split_001.html"},
		{"Text/My%20Book.html", "Text/My%20Book.html"},
		{"Text/100%.html", "Text/100%25.html"},
		{"Text/a%2.html", "Text/a%252.html"},
		{"Text/a(1)!$&'*+,;=:@~.html", "Text/a(1)!$&'*+,;=:@~.html"},
		{"Text/a\"<>[]^`{|}\\.html", "Text/a%22%3C%3E%5B%5D%5E%60%7B%7C%7D%5C.html"},
		{"Text/é.html", "Text/é.html"},
		{"Text/a\tb\x7f.html", "Text/a%09b%7F.html"},
		{"ch1.html#a b", "ch1.html#a%20b"},
	} {
		if got := escapeHref(tc[0]); got != tc[1] {
			t.Errorf("escapeHref(%q) = %q, want %q", tc[0], got, tc[1])
		}
	}
}

func TestHasScheme(t *testing.T) {
	for _, tc := range []struct {
		href string
		want bool
	}{
		{"http://example.com", true},
		{"mailto:a@example.com", true},
		{"a+b.c-d:x", true},
		{"ch1.html", false},
		{"a b:c", false},
		{"1a:b", false},
		{":a", false},
		{"Text/a:b.html", false},
		{"", false},
	} {
		if got := hasScheme(tc.href); got != tc.want {
			t.Errorf("hasScheme(%q) = %v, want %v", tc.href, got, tc.want)
		}
	}
}

func TestRebaseHref(t *testing.T) {
	for _, tc := range [][4]string{
		{"OEBPS", "Text/My%20Book.html", "OEBPS/Text", "My%20Book.html"},
		{"OEBPS", "Text/My%20Book.html", "OEBPS/Other", "../Text/My%20Book.html"},
		{"OEBPS", "Text/My%20Book.html", ".", "OEBPS/Text/My%20Book.html"},
		{"OEBPS", "./Text/../ch%201.html", "OEBPS", "ch%201.html"},
		{"OEBPS", "../ch1.html", "OEBPS/Text", "../../ch1.html"},
		{"My Dir", "ch1.html", ".", "My%20Dir/ch1.html"},
		{"100%", "ch1.html", ".", "100%25/ch1.html"},
		{"OEBPS", "Text/a:b.html", "OEBPS/Text", "./a:b.html"},
	} {
		if got := rebaseHref(tc[0], tc[1], tc[2]); got != tc[3] {
			t.Errorf("rebaseHref(%q, %q, %q) = %q, want %q", tc[0], tc[1], tc[2], got, tc[3])
		}
	}
}

func TestManifestHrefs(t *testing.T) {
	epub := fstest.MapFS{
		"OEBPS/Text/My Book.html": {},
		"OEBPS/Text/ch%201.html":  {}, // only the undecoded name exists
		"OEBPS/Text/A.html":       {},
		"OEBPS/Text/a.html":       {},
		"OEBPS/toc.ncx":           {},
		"OEBPS/nav.xhtml":         {},
	}
	opf := fixIndent("\t", true, `
		<?xml version="1.0" encoding="UTF-8"?>
		<package xmlns="http://www.idpf.org/2007/opf" version="3.0">
			<manifest>
				<item id="nav" href="nav.xhtml" media-type="application/xhtml+xml" properties="nav"/>
				<item id="toc" href="toc.ncx" media-type="application/x-dtbncx+xml"/>
				<item id="book" href="Text/My Book.html" media-type="application/xhtml+xml"/>
				<item id="ch1" href="Text/ch%201.html" media-type="application/xhtml+xml"/>
				<item id="A" href="Text/A.html" media-type="application/xhtml+xml"/>
				<item id="a" href="Text/a.html" media-type="application/xhtml+xml"/>
			</manifest>
			<spine toc="toc">
				<itemref idref="book"/>
			</spine>
		</package>
	`)
	m, err := newManifestHrefs(epub, "OEBPS/content.opf", []byte(opf))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if m.ncx != "OEBPS/toc.ncx" || m.nav != "OEBPS/nav.xhtml" {
		t.Errorf("got ncx %q nav %q", m.ncx, m.nav)
	}
	for _, tc := range []struct {
		base, href, want string
		ok               bool
	}{
		{"OEBPS/toc.ncx", "Text/My%20Book.html", "Text/My%20Book.html", true},
		{"OEBPS/toc.ncx", "Text/My Book.html#a", "Text/My%20Book.html#a", true},
		{"OEBPS/toc.ncx", "./Text/My%20Book.html", "Text/My%20Book.html", true},
		{"OEBPS/Text/nav.xhtml", "My Book.html", "My%20Book.html", true},
		{"OEBPS/Text/nav.xhtml", "../Text/My Book.html", "My%20Book.html", true},
		{"toc.ncx", "OEBPS/Text/My Book.html", "OEBPS/Text/My%20Book.html", true},
		{"OEBPS/toc.ncx", "Text/ch%201.html", "Text/ch%201.html", true},
		{"OEBPS/toc.ncx", "Text/missing.html", "Text/missing.html", false},
		{"OEBPS/toc.ncx", "Text/my book.html", "Text/My%20Book.html", true},
		{"OEBPS/toc.ncx", "TEXT/MY%20BOOK.HTML#A", "Text/My%20Book.html#A", true},
		{"OEBPS/Text/nav.xhtml", "CH%201.HTML", "ch%201.html", true},
		{"OEBPS/toc.ncx", "Text/a.html", "Text/a.html", true},
		{"OEBPS/toc.ncx", "Text/A.html", "Text/A.html", true},
		{"OEBPS/toc.ncx", "TEXT/A.HTML", "TEXT/A.HTML", false},
		{"OEBPS/toc.ncx", "#a", "#a", true},
		{"OEBPS/toc.ncx", "", "", true},
		{"OEBPS/toc.ncx", "http://example.com/My Book.html", "http://example.com/My Book.html", true},
		{"OEBPS/toc.ncx", "/OEBPS/Text/My Book.html", "/OEBPS/Text/My Book.html", true},
		{"OEBPS/toc.ncx", "Text/My Book.html?a", "Text/My Book.html?a", true},
	} {
		if got, ok := m.Fix(tc.base, tc.href); got != tc.want || ok != tc.ok {
			t.Errorf("Fix(%q, %q) = %q, %v, want %q, %v", tc.base, tc.href, got, ok, tc.want, tc.ok)
		}
	}
}

func TestTransformNCX(t *testing.T) {
	m := &manifestHrefs{
		opf: "OEBPS/content.opf",
		hrefs: map[string]string{
			"OEBPS/Text/My Book.html": "Text/My%20Book.html",
			"OEBPS/Text/ch1.html":     "Text/ch1.html",
		},
	}
	ncx := func(src ...string) string {
		var b strings.Builder
		b.WriteString(`<?xml version="1.0" encoding="UTF-8"?>
<ncx xmlns="http://www.daisy.org/z3986/2005/ncx/" version="2005-1">
  <navMap>`)
		for _, s := range src {
			b.WriteString(`
    <navPoint id="a"><navLabel><text>A</text></navLabel><content src="` + s + `" /></navPoint>`)
		}
		b.WriteString(`
  </navMap>
</ncx>
`)
		return b.String()
	}
	for _, tc := range []struct {
		name    string
		in, out string
		changed bool
		warn    []string
	}{
		{
			name: "unchanged",
			in:   ncx("Text/My%20Book.html", "Text/ch1.html#a"),
			out:  ncx("Text/My%20Book.html", "Text/ch1.html#a"),
		},
		{
			name:    "fixed",
			in:      ncx("Text/My Book.html", "./Text/ch1.html#a", "Text/missing.html"),
			out:     ncx("Text/My%20Book.html", "Text/ch1.html#a", "Text/missing.html"),
			changed: true,
			warn:    []string{"Text/missing.html"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var buf bytes.Buffer
			var warn []string
			changed, err := transformNCX(&buf, strings.NewReader(tc.in), "OEBPS/toc.ncx", m, func(name, href string) {
				if name != "OEBPS/toc.ncx" {
					t.Errorf("warning for %q", name)
				}
				warn = append(warn, href)
			})
			if err != nil {
				t.Fatalf("transform: %v", err)
			}
			if changed != tc.changed {
				t.Errorf("changed = %v, want %v", changed, tc.changed)
			}
			if buf.String() != tc.out {
				t.Errorf("got:\n%s\nwant:\n%s", buf.String(), tc.out)
			}
			if strings.Join(warn, "\n") != strings.Join(tc.warn, "\n") {
				t.Errorf("warnings %q, want %q", warn, tc.warn)
			}
		})
	}
}
