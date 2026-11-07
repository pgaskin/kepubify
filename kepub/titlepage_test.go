package kepub

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"path"
	"strings"
	"testing"
	"testing/fstest"

	xunicode "golang.org/x/text/encoding/unicode"
)

func TestTransformDummyTitlepage(t *testing.T) {
	const lorem = "Lorem ipsum dolor, sit amet consectetur adipisicing elit. Dolorem, placeat. Porro animi architecto pariatur laudantium voluptate, at, odit delectus fugiat beatae autem odio. Iure iste maiores corrupti porro quibusdam. Sunt?"
	twoItems := `
			<item id="item1" href="item1.html" media-type="application/xhtml+xml"/>
			<item id="item2" href="item2.html" media-type="application/xhtml+xml"/>
			<item id="cover" href="cover.png" media-type="image/png"/>
		`
	twoItemrefs := `
			<itemref idref="item1"/>
			<itemref idref="item2"/>
		`
	coverPage := `<!DOCTYPE html><html><head><title></title></head><body><img src="cover.png"></body></html>`
	longPage := `<!DOCTYPE html><html><head><title></title></head><body>` + strings.Repeat(`<p>`+lorem+`</p>`, 5) + `</body></html>`
	for _, tc := range []titlepageTest{
		{
			name:     "separate titlepage (no dummy)",
			manifest: twoItems,
			spine:    twoItemrefs,
			content:  map[string]string{"item1.html": coverPage, "item2.html": longPage, "cover.png": ``},
		},
		{
			name: "separate titlepage, but non-standard extension (no dummy)",
			manifest: `
			<item id="item1" href="item1.xml" media-type="application/xhtml+xml"/>
			<item id="item2" href="item2.html" media-type="application/xhtml+xml"/>
			<item id="cover" href="cover.png" media-type="image/png"/>
		`,
			spine:   twoItemrefs,
			content: map[string]string{"item1.xml": coverPage, "item2.html": longPage, "cover.png": ``},
		},
		{
			name: "separate titlepage, but non-standard media type (no dummy)",
			manifest: `
			<item id="item1" href="item1.html" media-type="text/xml"/>
			<item id="item2" href="item2.html" media-type="application/xhtml+xml"/>
			<item id="cover" href="cover.png" media-type="image/png"/>
		`,
			spine:   twoItemrefs,
			content: map[string]string{"item1.html": coverPage, "item2.html": longPage, "cover.png": ``},
		},
		{
			name: "separate titlepage, but non-linear content spine item before it (no dummy)",
			manifest: `
			<item id="item1" href="item1.html" media-type="application/xhtml+xml"/>
			<item id="item2" href="item2.html" media-type="application/xhtml+xml"/>
			<item id="item3" href="item3.html" media-type="application/xhtml+xml"/>
			<item id="cover" href="cover.png" media-type="image/png"/>
		`,
			spine: `
			<itemref idref="item3" linear="no"/>
			<itemref idref="item1"/>
			<itemref idref="item2"/>
		`,
			content: map[string]string{"item1.html": coverPage, "item2.html": longPage, "item3.html": longPage, "cover.png": ``},
		},
		{
			name:     "separate titlepage, but non-existent spine item before it (no dummy)",
			manifest: twoItems,
			spine: `
			<itemref idref="item3"/>
			<itemref idref="item1"/>
			<itemref idref="item2"/>
		`,
			content: map[string]string{"item1.html": coverPage, "item2.html": longPage, "cover.png": ``},
		},
		{
			name:     "separate titlepage (force enable)",
			force:    new(true),
			manifest: twoItems,
			spine:    twoItemrefs,
			content:  map[string]string{"item1.html": coverPage, "item2.html": longPage, "cover.png": ``},
			added:    true,
		},
		{
			name:     "separate titlepage (force disable)",
			force:    new(false),
			manifest: twoItems,
			spine:    twoItemrefs,
			content:  map[string]string{"item1.html": coverPage, "item2.html": longPage, "cover.png": ``},
		},
		{
			name:     "no titlepage, many words (dummy)",
			manifest: `<item id="item1" href="item1.html" media-type="application/xhtml+xml"/>`,
			spine:    `<itemref idref="item1"/>`,
			content:  map[string]string{"item1.html": `<!DOCTYPE html><html><head><title></title></head><body>` + strings.Repeat(`<div>`+lorem+`</div>`, 5) + `</body></html>`},
			added:    true,
		},
		{
			name:     "no titlepage, many images (dummy)",
			manifest: `<item id="item1" href="item1.html" media-type="application/xhtml+xml"/>`,
			spine:    `<itemref idref="item1"/>`,
			content:  map[string]string{"item1.html": `<!DOCTYPE html><html><head><title></title></head><body>` + strings.Repeat(`<img><svg></svg>`, 4) + `</body></html>`},
			added:    true,
		},
		{
			name:     "no titlepage, many short paragraphs (dummy)",
			manifest: `<item id="item1" href="item1.html" media-type="application/xhtml+xml"/>`,
			spine:    `<itemref idref="item1"/>`,
			content:  map[string]string{"item1.html": `<!DOCTYPE html><html><head><title></title></head><body><p>Paragraph 1.</p><p>Paragraph 2.</p><p>Paragraph 3.</p><p>Paragraph 4.</p><p>Paragraph 5.</p></body></html>`},
			added:    true,
		},
		{
			name:     "titlepage doesn't match heuristic but name includes cover (no dummy)",
			manifest: `<item id="item1" href="Cover.html" media-type="application/xhtml+xml"/>`,
			spine:    `<itemref idref="item1"/>`,
			content:  map[string]string{"item1.html": longPage},
		},
		{
			name:     "titlepage doesn't match heuristic but name includes title (no dummy)",
			manifest: `<item id="item1" href="Title.html" media-type="application/xhtml+xml"/>`,
			spine:    `<itemref idref="item1"/>`,
			content:  map[string]string{"item1.html": longPage},
		},
		{
			name: "textual titlepage (no dummy)",
			manifest: `
			<item id="item1" href="item1.html" media-type="application/xhtml+xml"/>
			<item id="item2" href="item2.html" media-type="application/xhtml+xml"/>
		`,
			spine:   twoItemrefs,
			content: map[string]string{"item1.html": `<!DOCTYPE html><html><head><title></title></head><body><h1>A Long Title on a Title Page</h1><h2>The Subtitle of the Book</h2></body></html>`, "item2.html": longPage},
		},
		{
			name: "nonexistent first manifest file (no dummy)",
			manifest: `
			<item id="item1" href="item1.html" media-type="application/xhtml+xml"/>
			<item id="item2" href="item2.html" media-type="application/xhtml+xml"/>
		`,
			spine:   twoItemrefs,
			content: map[string]string{"Item2.html": longPage, "item2.html": longPage},
		},
		{
			name:     "spine without whitespace",
			force:    new(true),
			manifest: `<item id="item1" href="item1.html" media-type="application/xhtml+xml"/>`,
			spine:    `<itemref idref="item1"/>`,
			content:  map[string]string{"item1.html": longPage},
			added:    true,
		},
		{
			name: "empty opf package",
		},
		// note: we don't check well-formedness
		{
			name:     "bad opf package",
			manifest: `<invalid>`,
			spine:    `<itemref idref="item1"/>`,
			content:  map[string]string{"item1.html": longPage},
		},
		{
			name:     "bad opf package, but titlepage force disabled (no dummy)",
			force:    new(false),
			manifest: `<invalid>`,
			spine:    `<itemref idref="item1"/>`,
			content:  map[string]string{"item1.html": longPage},
		},
	} {
		tc.run(t)
	}
}

func TestTransformDummyTitlepageCover(t *testing.T) {
	const img = "image"
	for _, tc := range []titlepageTest{
		{
			name:     "declared in the opf",
			manifest: `<item id="c" href="images/front.jpg" media-type="image/jpeg" properties="cover-image"/>`,
			content:  map[string]string{"images/front.jpg": img, "cover.jpg": img},
			cover:    "images/front.jpg",
		},
		{
			name:     "declared in the opf, but not an image",
			manifest: `<item id="cover" href="cover.xhtml" media-type="application/xhtml+xml" properties="cover-image"/>`,
			content:  map[string]string{"cover.xhtml": "", "cover.png": img},
			cover:    "cover.png",
		},
		{
			name:     "declared in the opf, but missing",
			manifest: `<item id="c" href="front.jpg" media-type="image/jpeg" properties="cover-image"/>`,
			content:  map[string]string{"cover.jpg": img},
			cover:    "cover.jpg",
		},
		{
			name:    "package directory first",
			content: map[string]string{"cover.jpg": img, "images/cover.jpg": img, "../cover.jpg": img},
			cover:   "cover.jpg",
		},
		{
			name:    "anywhere before the next pattern",
			content: map[string]string{"cover_front.png": img, "../cover.jpg": img},
			cover:   "../cover.jpg",
		},
		{
			name:    "cover_ before _cover",
			content: map[string]string{"img/x_cover.jpg": img, "img/cover_x.jpg": img},
			cover:   "img/cover_x.jpg",
		},
		{
			name:    "_cover before cover suffix",
			content: map[string]string{"bookcover.jpg": img, "book_cover.jpg": img},
			cover:   "book_cover.jpg",
		},
		{
			name:    "cover suffix before containing cover",
			content: map[string]string{"covers.png": img, "bookcover.gif": img},
			cover:   "bookcover.gif",
		},
		{
			name:    "case-insensitive",
			content: map[string]string{"Images/COVER.JPEG": img},
			cover:   "Images/COVER.JPEG",
		},
		{
			name:    "escaped",
			content: map[string]string{"my cover.jpg": img},
			cover:   "my%20cover.jpg",
		},
		{
			name:    "filtered files and other types",
			content: map[string]string{"../__MACOSX/OEBPS/._cover.jpg": img, "cover.xhtml": "", "cover.svg": img},
		},
	} {
		tc.force, tc.added, tc.detectCover = new(true), true, true
		tc.run(t)
	}
	titlepageTest{
		name:    "disabled",
		force:   new(true),
		added:   true,
		content: map[string]string{"cover.jpg": img},
	}.run(t)
}

func TestRelativeHref(t *testing.T) {
	for _, tc := range [][3]string{
		{"OEBPS", "OEBPS/cover.jpg", "cover.jpg"},
		{"OEBPS", "OEBPS/images/cover.jpg", "images/cover.jpg"},
		{"OEBPS/text", "OEBPS/images/cover.jpg", "../images/cover.jpg"},
		{"OEBPS", "cover.jpg", "../cover.jpg"},
		{".", "OEBPS/cover.jpg", "OEBPS/cover.jpg"},
		{"", "cover.jpg", "cover.jpg"},
		{"OEBPS", "OEBPS/a:b.jpg", "./a:b.jpg"},
		{"OEBPS", "OEBPS/my cover#1?.jpg", "my%20cover%231%3F.jpg"},
		{"a/b", "a/b", "../b"},
	} {
		if got := relativeHref(tc[0], tc[1]); got != tc[2] {
			t.Errorf("relativeHref(%q, %q) = %q, want %q", tc[0], tc[1], got, tc[2])
		}
	}
}

func TestTransformDummyTitlepageParts(t *testing.T) {
	const lorem = "Lorem ipsum dolor sit amet, consectetur adipiscing elit, sed do eiusmod tempor incididunt ut labore et dolore magna aliqua."
	words := map[string]string{"text.xhtml": `<html><body><div>` + lorem + ` ` + lorem + `</div></body></html>`}
	for _, tc := range []titlepageTest{
		{
			name: "indentation",
			opf: `<?xml version="1.0" encoding="UTF-8"?>
<package xmlns="http://www.idpf.org/2007/opf" version="3.0">
  <manifest>
    <item id="text" href="text.xhtml" media-type="application/xhtml+xml"/>
  </manifest>
  <spine>
    <itemref idref="text"/>
  </spine>
</package>
`,
			content: words,
			added:   true,
			want: `<?xml version="1.0" encoding="UTF-8"?>
<package xmlns="http://www.idpf.org/2007/opf" version="3.0">
  <manifest>
    <item id="text" href="text.xhtml" media-type="application/xhtml+xml"/>
    <item id="kepubify-titlepage-dummy" href="kepubify-titlepage-dummy.xhtml" media-type="application/xhtml+xml" />
  </manifest>
  <spine>
    <itemref idref="kepubify-titlepage-dummy" />
    <itemref idref="text"/>
  </spine>
</package>
`,
		},
		{
			name:    "no whitespace",
			opf:     `<package xmlns="http://www.idpf.org/2007/opf"><manifest><item id="text" href="text.xhtml"/></manifest><spine><itemref idref="text"/><itemref idref="x"/></spine></package>`,
			content: words,
			added:   true,
			want:    `<package xmlns="http://www.idpf.org/2007/opf"><manifest><item id="text" href="text.xhtml"/><item id="kepubify-titlepage-dummy" href="kepubify-titlepage-dummy.xhtml" media-type="application/xhtml+xml" /></manifest><spine><itemref idref="kepubify-titlepage-dummy" /><itemref idref="text"/><itemref idref="x"/></spine></package>`,
		},
		{
			name:    "prefixes",
			opf:     `<opf:package xmlns:opf="http://www.idpf.org/2007/opf" xmlns="urn:x"><opf:manifest><opf:item id="text" href="text.xhtml"/></opf:manifest><opf:spine><opf:itemref idref="text"/></opf:spine></opf:package>`,
			content: words,
			added:   true,
			want:    `<opf:package xmlns:opf="http://www.idpf.org/2007/opf" xmlns="urn:x"><opf:manifest><opf:item id="text" href="text.xhtml"/><opf:item id="kepubify-titlepage-dummy" href="kepubify-titlepage-dummy.xhtml" media-type="application/xhtml+xml" /></opf:manifest><opf:spine><opf:itemref idref="kepubify-titlepage-dummy" /><opf:itemref idref="text"/></opf:spine></opf:package>`,
		},
		{
			name:    "other namespaces and attributes",
			opf:     `<package xmlns="http://www.idpf.org/2007/opf" xmlns:x="urn:x"><manifest><item x:id="text" href="text.xhtml"/><x:item id="text" href="text.xhtml"/></manifest><spine><itemref idref="text"/></spine></package>`,
			content: words,
		},
		{
			name:    "manifest and spine elsewhere",
			opf:     `<package xmlns="http://www.idpf.org/2007/opf"><x><manifest><item id="text" href="text.xhtml"/></manifest><spine><itemref idref="text"/></spine></x></package>`,
			content: words,
		},
		{
			name:    "first elements only",
			opf:     `<package xmlns="http://www.idpf.org/2007/opf"><manifest/><manifest><item id="text" href="text.xhtml"/></manifest><spine><itemref idref="text"/></spine><spine/></package>`,
			content: words,
			added:   true,
			want:    `<package xmlns="http://www.idpf.org/2007/opf"><manifest><item id="kepubify-titlepage-dummy" href="kepubify-titlepage-dummy.xhtml" media-type="application/xhtml+xml" /></manifest><manifest><item id="text" href="text.xhtml"/></manifest><spine><itemref idref="kepubify-titlepage-dummy" /><itemref idref="text"/></spine><spine/></package>`,
		},
		{
			name:  "empty elements",
			force: new(true),
			opf:   `<package xmlns="http://www.idpf.org/2007/opf"><manifest x="/"/><spine/></package>`,
			added: true,
			want:  `<package xmlns="http://www.idpf.org/2007/opf"><manifest x="/"><item id="kepubify-titlepage-dummy" href="kepubify-titlepage-dummy.xhtml" media-type="application/xhtml+xml" /></manifest><spine><itemref idref="kepubify-titlepage-dummy" /></spine></package>`,
		},
		{
			name:  "empty elements with whitespace",
			force: new(true),
			opf:   "<package xmlns=\"http://www.idpf.org/2007/opf\">\n <manifest>\n </manifest>\n <spine>\n </spine>\n</package>",
			added: true,
			want:  "<package xmlns=\"http://www.idpf.org/2007/opf\">\n <manifest>\n <item id=\"kepubify-titlepage-dummy\" href=\"kepubify-titlepage-dummy.xhtml\" media-type=\"application/xhtml+xml\" />\n </manifest>\n <spine>\n <itemref idref=\"kepubify-titlepage-dummy\" />\n </spine>\n</package>",
		},
		{
			name: "root element isn't a package element",
			opf:  `<x:package xmlns:x="urn:x"/>`,
			err:  true,
		},
		{
			name: "no root element",
			opf:  `<!-- -->`,
			err:  true,
		},
		{
			name:    "escaped href",
			opf:     `<package xmlns="http://www.idpf.org/2007/opf"><manifest><item id="text" href="my%20text.xhtml"/></manifest><spine><itemref idref="text"/></spine></package>`,
			content: map[string]string{"my text.xhtml": words["text.xhtml"]},
			added:   true,
		},
		{
			name:    "linear spine item without idref",
			opf:     `<package xmlns="http://www.idpf.org/2007/opf"><manifest><item id="text" href="text.xhtml"/></manifest><spine><itemref linear="no" idref="text"/><itemref/><itemref idref="text"/></spine></package>`,
			content: words,
		},
		{
			name:    "duplicate manifest items",
			opf:     `<package xmlns="http://www.idpf.org/2007/opf"><manifest><item id="text" href="text.xhtml"/><item id="text" href="text.png"/></manifest><spine><itemref idref="text"/></spine></package>`,
			content: words,
		},
		{
			name:    "content in p elements isn't counted",
			opf:     `<package xmlns="http://www.idpf.org/2007/opf"><manifest><item id="text" href="text.xhtml"/></manifest><spine><itemref idref="text"/></spine></package>`,
			content: map[string]string{"text.xhtml": `<html><body><p><img src="cover.jpg"/></p><p>` + lorem + `</p></body></html>`},
			added:   true,
		},
		{
			name:    "images",
			opf:     `<package xmlns="http://www.idpf.org/2007/opf"><manifest><item id="text" href="text.xhtml"/></manifest><spine><itemref idref="text"/></spine></package>`,
			content: map[string]string{"text.xhtml": `<html><body><div><image src="cover.jpg"/><span>A title</span></div></body></html>`},
		},
		{
			name:    "words split by elements and comments",
			opf:     `<package xmlns="http://www.idpf.org/2007/opf"><manifest><item id="text" href="text.xhtml"/></manifest><spine><itemref idref="text"/></spine></package>`,
			content: map[string]string{"text.xhtml": `<html><head><title>` + lorem + ` ` + lorem + `</title></head><body><img src="x"/>` + strings.Repeat(`ab<b>cd</b>ef<!---->gh `, 21) + `</body></html>`},
		},
		{
			name:    "words not split by references",
			opf:     `<package xmlns="http://www.idpf.org/2007/opf"><manifest><item id="text" href="text.xhtml"/></manifest><spine><itemref idref="text"/></spine></package>`,
			content: map[string]string{"text.xhtml": `<html><body><img src="x"/>` + strings.Repeat(`ab&amp;c&#32;`, 21) + `</body></html>`},
			added:   true,
		},
		{
			name:    "stray p end tags",
			opf:     `<package xmlns="http://www.idpf.org/2007/opf"><manifest><item id="text" href="text.xhtml"/></manifest><spine><itemref idref="text"/></spine></package>`,
			content: map[string]string{"text.xhtml": `<html><body><img src="x"/><div></p></p></p></p></p></div></body></html>`},
			added:   true,
		},
		{
			// test html5 parsing rules
			name:    "misnested formatting elements with a warning",
			opf:     `<package xmlns="http://www.idpf.org/2007/opf"><manifest><item id="text" href="text.xhtml"/></manifest><spine><itemref idref="text"/></spine></package>`,
			content: map[string]string{"text.xhtml": `<html><body><img src="x"/><b><p>x</b>` + lorem + ` ` + lorem + `</p></body></html>`},
			added:   true,
		},
		{
			name:    "frameset",
			opf:     `<package xmlns="http://www.idpf.org/2007/opf"><manifest><item id="text" href="text.xhtml"/></manifest><spine><itemref idref="text"/></spine></package>`,
			content: map[string]string{"text.xhtml": `<html><head></head><frameset><frame src="a.html"/></frameset></html>`},
			added:   true,
		},
		{
			name:    "UTF-16 content document read as UTF-8",
			opf:     `<package xmlns="http://www.idpf.org/2007/opf"><manifest><item id="text" href="text.xhtml"/></manifest><spine><itemref idref="text"/></spine></package>`,
			content: map[string]string{"text.xhtml": "\xff\xfe<\x00h\x00t\x00m\x00l\x00>\x00"},
			charset: "utf-8",
		},
		{
			name:    "UTF-16 content document with charset detection",
			opf:     `<package xmlns="http://www.idpf.org/2007/opf"><manifest><item id="text" href="text.xhtml"/></manifest><spine><itemref idref="text"/></spine></package>`,
			content: map[string]string{"text.xhtml": "\xff\xfe" + must(xunicode.UTF16(xunicode.LittleEndian, xunicode.IgnoreBOM).NewEncoder().String(words["text.xhtml"]))},
			charset: "auto",
			added:   true,
		},
		{
			name:    "UTF-16 content document with a charset",
			opf:     `<package xmlns="http://www.idpf.org/2007/opf"><manifest><item id="text" href="text.xhtml"/></manifest><spine><itemref idref="text"/></spine></package>`,
			content: map[string]string{"text.xhtml": must(xunicode.UTF16(xunicode.BigEndian, xunicode.IgnoreBOM).NewEncoder().String(words["text.xhtml"]))},
			charset: "utf-16be",
			added:   true,
		},
		{
			name:    "invalid charset",
			opf:     `<package xmlns="http://www.idpf.org/2007/opf"><manifest><item id="text" href="text.xhtml"/></manifest><spine><itemref idref="text"/></spine></package>`,
			content: words,
			charset: "nonexistent",
			err:     true,
		},
	} {
		tc.run(t)
	}
}

type titlepageTest struct {
	name     string
	force    *bool
	opf      string // override default test package document
	manifest string
	spine    string
	content  map[string]string // relative to the package document
	err      bool
	added    bool
	want     string // new opf to check (if not empty)

	detectCover bool
	cover       string // expected cover image src (if not empty)

	charset string
}

const titlepageOPF = "OEBPS/content.opf"

func (tc titlepageTest) run(t *testing.T) {
	t.Run(tc.name, func(t *testing.T) {
		epub := fstest.MapFS{}
		opf := tc.opf
		if opf == "" {
			opf = `<?xml version="1.0" encoding="UTF-8"?>
<package xmlns="http://www.idpf.org/2007/opf" version="3.0">
    <metadata xmlns:dc="http://purl.org/dc/elements/1.1/">
        <dc:title>Test</dc:title>
    </metadata>
    <manifest>` + tc.manifest + `</manifest>
    <spine toc="ncx">` + tc.spine + `</spine>
</package>
`
		}
		epub[titlepageOPF] = &fstest.MapFile{Data: []byte(opf)}
		for name, s := range tc.content {
			epub[path.Join(path.Dir(titlepageOPF), name)] = &fstest.MapFile{Data: []byte(s)}
		}
		buf := bytes.NewBufferString(opf)
		name, r, added, err := transformDummyTitlepage(epub, titlepageOPF, buf, Options{DummyTitlepage: tc.force, DetectCover: tc.detectCover, Charset: tc.charset})
		if tc.err != (err != nil) {
			t.Errorf("error %v, want error %v", err, tc.err)
		}
		if added != tc.added {
			t.Errorf("added %v, want %v", added, tc.added)
		}
		if err != nil {
			return
		}
		if !added {
			if buf.String() != opf {
				t.Errorf("package document changed:\n%s", buf.String())
			}
			return
		}
		if name != "OEBPS/kepubify-titlepage-dummy.xhtml" {
			t.Errorf("name %q", name)
		}
		if b, err := io.ReadAll(r); err != nil {
			t.Errorf("content document: %v", err)
		} else if tc.cover == "" && string(b) != dummyTitlepage {
			t.Errorf("content document %q", b)
		} else if tc.cover != "" && string(b) != dummyTitlepageWithCover(tc.cover) {
			t.Errorf("content document %q, want cover %q", b, tc.cover)
		}
		if err := checkTitlepageOPF(buf.Bytes()); err != nil {
			t.Errorf("%v:\n%s", err, buf.String())
		}
		if tc.want != "" && buf.String() != tc.want {
			t.Errorf("got:\n%s\nwant:\n%s", buf.String(), tc.want)
		}
	})
}

func checkTitlepageOPF(opf []byte) error {
	var p struct {
		Items []struct {
			ID        string `xml:"id,attr"`
			Href      string `xml:"href,attr"`
			MediaType string `xml:"media-type,attr"`
		} `xml:"http://www.idpf.org/2007/opf manifest>item"`
		Itemrefs []struct {
			IDRef  string `xml:"idref,attr"`
			Linear string `xml:"linear,attr"`
		} `xml:"http://www.idpf.org/2007/opf spine>itemref"`
	}
	if err := xml.Unmarshal(opf, &p); err != nil {
		return err
	}
	var item bool
	for _, it := range p.Items {
		if it.Href == "kepubify-titlepage-dummy.xhtml" {
			if it.ID != "kepubify-titlepage-dummy" || it.MediaType != "application/xhtml+xml" {
				return fmt.Errorf("incorrect manifest item %+v", it)
			}
			item = true
		}
	}
	if !item {
		return fmt.Errorf("no manifest item")
	}
	for _, it := range p.Itemrefs {
		if it.Linear != "no" {
			if it.IDRef != "kepubify-titlepage-dummy" {
				return fmt.Errorf("first linear spine item is %q, not the title page", it.IDRef)
			}
			return nil
		}
	}
	return fmt.Errorf("no linear spine item")
}
