package kepub

import (
	"bytes"
	"strings"
	"testing"
	"testing/iotest"

	xunicode "golang.org/x/text/encoding/unicode"
	"golang.org/x/text/encoding/unicode/utf32"
)

func TestTransformOPF(t *testing.T) {
	for _, tc := range []struct {
		name    string
		in, out string
	}{
		{
			name: "unchanged",
			in: `<?xml version="1.0" encoding="UTF-8"?>
<package xmlns="http://www.idpf.org/2007/opf" version="3.0" unique-identifier="uuid_id">
    <metadata xmlns:dc="http://purl.org/dc/elements/1.1/">
        <!-- other stuff left out for brevity -->
    </metadata><manifest>
    <item id="cover-image" href="book_cover.jpg" media-type="image/jpeg"/>
       <item id="xhtml_text1" href="xhtml/text1.xhtml" media-type="application/xhtml+xml"/>
       <item id="ncx" href="toc.ncx" media-type="application/x-dtbncx+xml"/>
  <item id="nav" href="nav.xhtml" media-type="application/xhtml+xml" properties="nav"/>
    </manifest>
    <spine toc="ncx">
        <itemref idref="xhtml_text1"/>
    </spine>
</package>`,
		},
		{
			name: "set cover-image property on ID from meta[name=cover]",
			in: fixIndent("    ", false, `
				<?xml version="1.0" encoding="UTF-8"?>
				<package xmlns="http://www.idpf.org/2007/opf" version="3.0" unique-identifier="uuid_id">
					<metadata xmlns:dc="http://purl.org/dc/elements/1.1/">
						<!-- other stuff left out for brevity -->
						<meta name="cover" content="cover-image"/>
					</metadata>
					<manifest>
						<item id="cover-image" href="book_cover.jpg" media-type="image/jpeg"/>
						<item id="xhtml_text1" href="xhtml/text1.xhtml" media-type="application/xhtml+xml"/>
						<item id="ncx" href="toc.ncx" media-type="application/x-dtbncx+xml"/>
						<item id="nav" href="nav.xhtml" media-type="application/xhtml+xml" properties="nav"/>
					</manifest>
					<spine toc="ncx">
						<itemref idref="xhtml_text1"/>
					</spine>
				</package>
			`),
			out: fixIndent("    ", false, `
				<?xml version="1.0" encoding="UTF-8"?>
				<package xmlns="http://www.idpf.org/2007/opf" version="3.0" unique-identifier="uuid_id">
					<metadata xmlns:dc="http://purl.org/dc/elements/1.1/">
						<!-- other stuff left out for brevity -->
						<meta name="cover" content="cover-image"/>
					</metadata>
					<manifest>
						<item id="cover-image" href="book_cover.jpg" media-type="image/jpeg" properties="cover-image"/>
						<item id="xhtml_text1" href="xhtml/text1.xhtml" media-type="application/xhtml+xml"/>
						<item id="ncx" href="toc.ncx" media-type="application/x-dtbncx+xml"/>
						<item id="nav" href="nav.xhtml" media-type="application/xhtml+xml" properties="nav"/>
					</manifest>
					<spine toc="ncx">
						<itemref idref="xhtml_text1"/>
					</spine>
				</package>
			`),
		},
		{
			name: "set cover-image property on #cover if meta[name=cover] not present",
			in: fixIndent("    ", false, `
				<?xml version="1.0" encoding="UTF-8"?>
				<package xmlns="http://www.idpf.org/2007/opf" version="3.0" unique-identifier="uuid_id">
					<metadata xmlns:dc="http://purl.org/dc/elements/1.1/">
						<!-- other stuff left out for brevity -->
					</metadata>
					<manifest>
						<item id="cover" href="book_cover.jpg" media-type="image/jpeg"/>
						<item id="xhtml_text1" href="xhtml/text1.xhtml" media-type="application/xhtml+xml"/>
					</manifest>
				</package>
			`),
			out: fixIndent("    ", false, `
				<?xml version="1.0" encoding="UTF-8"?>
				<package xmlns="http://www.idpf.org/2007/opf" version="3.0" unique-identifier="uuid_id">
					<metadata xmlns:dc="http://purl.org/dc/elements/1.1/">
						<!-- other stuff left out for brevity -->
					</metadata>
					<manifest>
						<item id="cover" href="book_cover.jpg" media-type="image/jpeg" properties="cover-image"/>
						<item id="xhtml_text1" href="xhtml/text1.xhtml" media-type="application/xhtml+xml"/>
					</manifest>
				</package>
			`),
		},
		{
			name: "remove calibre:timestamp and calibre contributor",
			in: fixIndent("    ", false, `
				<?xml version="1.0" encoding="UTF-8"?>
				<package xmlns="http://www.idpf.org/2007/opf" version="3.0" unique-identifier="uuid_id">
					<metadata xmlns:dc="http://purl.org/dc/elements/1.1/">
						<!-- --><meta content="whatever" name="calibre:timestamp"/>
						<!-- --><dc:contributor file-as="calibre" role="bkp">calibre (#.#.#) [https://calibre-ebook.com]</dc:contributor>
						<!-- --><dc:contributor>whatever</dc:contributor>
					</metadata>
					<!-- other stuff left out for brevity -->
				</package>
			`),
			out: fixIndent("    ", false, `
				<?xml version="1.0" encoding="UTF-8"?>
				<package xmlns="http://www.idpf.org/2007/opf" version="3.0" unique-identifier="uuid_id">
					<metadata xmlns:dc="http://purl.org/dc/elements/1.1/">
						<!-- -->
						<!-- -->
						<!-- --><dc:contributor>whatever</dc:contributor>
					</metadata>
					<!-- other stuff left out for brevity -->
				</package>
			`),
		},
		{
			name: "more calibre metadata, epub2 prefixes",
			in: fixIndent("  ", false, `
				<?xml version='1.0' encoding='utf-8'?>
				<package xmlns="http://www.idpf.org/2007/opf" unique-identifier="uuid_id" version="2.0">
					<metadata xmlns:dc="http://purl.org/dc/elements/1.1/" xmlns:opf="http://www.idpf.org/2007/opf">
						<dc:title>Title</dc:title>
						<dc:contributor opf:file-as="calibre" opf:role="bkp">calibre (5.0.0) [https://calibre-ebook.com]</dc:contributor>
						<dc:contributor opf:role="edt">Editor</dc:contributor>
						<opf:meta name="calibre:timestamp" content="2020-01-01T00:00:00+00:00"/>
					</metadata>
				</package>
			`),
			out: fixIndent("  ", false, `
				<?xml version='1.0' encoding='utf-8'?>
				<package xmlns="http://www.idpf.org/2007/opf" unique-identifier="uuid_id" version="2.0">
					<metadata xmlns:dc="http://purl.org/dc/elements/1.1/" xmlns:opf="http://www.idpf.org/2007/opf">
						<dc:title>Title</dc:title>
						<dc:contributor opf:role="edt">Editor</dc:contributor>
					</metadata>
				</package>
			`),
		},
		{
			name: "prefixed package document",
			in: fixIndent("    ", false, `
				<opf:package xmlns:opf="http://www.idpf.org/2007/opf" xmlns:x="http://purl.org/dc/elements/1.1/" version="2.0">
				<opf:metadata><x:contributor opf:role="bkp">calibre</x:contributor><opf:meta name="cover" content="img"/></opf:metadata>
				<opf:manifest><opf:item id="img" href="c.jpg" media-type="image/jpeg"/></opf:manifest>
				</opf:package>
			`),
			out: fixIndent("    ", false, `
				<opf:package xmlns:opf="http://www.idpf.org/2007/opf" xmlns:x="http://purl.org/dc/elements/1.1/" version="2.0">
				<opf:metadata><opf:meta name="cover" content="img"/></opf:metadata>
				<opf:manifest><opf:item id="img" href="c.jpg" media-type="image/jpeg" properties="cover-image"/></opf:manifest>
				</opf:package>
			`),
		},
		{
			name: "cover meta element after the manifest",
			in: `<package xmlns="http://www.idpf.org/2007/opf"><manifest><item id="cover" href="c.xhtml"/><item id="img" href="c.jpg"/></manifest>` +
				`<metadata><meta name="cover" content="img"/></metadata></package>`,
			out: `<package xmlns="http://www.idpf.org/2007/opf"><manifest><item id="cover" href="c.xhtml"/><item id="img" href="c.jpg" properties="cover-image"/></manifest>` +
				`<metadata><meta name="cover" content="img"/></metadata></package>`,
		},
		{
			name: "first match by id",
			in: `<package xmlns="http://www.idpf.org/2007/opf"><metadata><meta name="cover" content="a"/><meta name="cover" content="b"/></metadata>` +
				`<manifest><item id="b" href="b.jpg"/><item id="a" href="a.jpg"/><item id="a" href="a2.jpg"/></manifest></package>`,
			out: `<package xmlns="http://www.idpf.org/2007/opf"><metadata><meta name="cover" content="a"/><meta name="cover" content="b"/></metadata>` +
				`<manifest><item id="b" href="b.jpg"/><item id="a" href="a.jpg" properties="cover-image"/><item id="a" href="a2.jpg"/></manifest></package>`,
		},
		{
			name: "breadth-first id match",
			in: `<package xmlns="http://www.idpf.org/2007/opf"><metadata><x-metadata><meta name="cover" content="a"/></x-metadata><meta name="cover" content="b"/></metadata>` +
				`<manifest><x><item id="b" href="b1.jpg"/></x><item id="b" href="b2.jpg"/></manifest></package>`,
			out: `<package xmlns="http://www.idpf.org/2007/opf"><metadata><x-metadata><meta name="cover" content="a"/></x-metadata><meta name="cover" content="b"/></metadata>` +
				`<manifest><x><item id="b" href="b1.jpg"/></x><item id="b" href="b2.jpg" properties="cover-image"/></manifest></package>`,
		},
		{
			name: "existing properties",
			in:   `<package xmlns="http://www.idpf.org/2007/opf"><manifest><item id="cover" properties="svg  scripted" href="c.xhtml"/></manifest></package>`,
			out:  `<package xmlns="http://www.idpf.org/2007/opf"><manifest><item id="cover" properties="svg scripted cover-image" href="c.xhtml"/></manifest></package>`,
		},
		{
			name: "existing cover-image property",
			in:   `<package xmlns="http://www.idpf.org/2007/opf"><manifest><item id="cover" properties=' cover-image' href="c.jpg"/></manifest></package>`,
			out:  `<package xmlns="http://www.idpf.org/2007/opf"><manifest><item id="cover" properties=' cover-image' href="c.jpg"/></manifest></package>`,
		},
		{
			name: "id with quotes",
			in:   `<package xmlns="http://www.idpf.org/2007/opf"><metadata><meta name="cover" content="a'&quot;b"/></metadata><manifest><item id='a&apos;"b'/></manifest></package>`,
			out:  `<package xmlns="http://www.idpf.org/2007/opf"><metadata><meta name="cover" content="a'&quot;b"/></metadata><manifest><item id='a&apos;"b' properties="cover-image"/></manifest></package>`,
		},
		{
			name: "elements in other namespaces and prefixed attributes",
			in: `<package xmlns="http://www.idpf.org/2007/opf" xmlns:x="urn:x"><metadata xmlns:dc="http://purl.org/dc/elements/1.1/">` +
				`<x:meta name="cover" content="a"/><meta x:name="cover" content="b"/><meta xmlns="" name="calibre:timestamp"/><x:contributor role="bkp"/><dc:contributor x:role="bkp"/>` +
				`</metadata><manifest><item x:id="cover"/><item xml:id="cover"/></manifest></package>`,
		},
		{
			name: "unbound prefixes",
			in:   `<package xmlns="http://www.idpf.org/2007/opf"><metadata><dc:contributor opf:role="bkp"/><opf:meta name="cover" content="a"/></metadata><manifest><item id="cover"/></manifest></package>`,
			out:  `<package xmlns="http://www.idpf.org/2007/opf"><metadata><dc:contributor opf:role="bkp"/><opf:meta name="cover" content="a"/></metadata><manifest><item id="cover" properties="cover-image"/></manifest></package>`,
		},
		{
			name: "malformed",
			in:   `<package xmlns="http://www.idpf.org/2007/opf"><metadata><meta name="calibre:timestamp"><manifest><item id="cover">`,
			out:  `<package xmlns="http://www.idpf.org/2007/opf"><metadata>`,
		},
		{
			name: "malformed token",
			in:   `<package xmlns="http://www.idpf.org/2007/opf"><metadata><dc:title xmlns:dc="http://purl.org/dc/elements/1.1/">A & B</dc:title><meta name="calibre:timestamp"/></metadata><manifest><item id="cover"/></manifest></package>`,
			out:  `<package xmlns="http://www.idpf.org/2007/opf"><metadata><dc:title xmlns:dc="http://purl.org/dc/elements/1.1/">A & B</dc:title></metadata><manifest><item id="cover" properties="cover-image"/></manifest></package>`,
		},
		{
			name: "utf-16le with bom",
			in:   "\xff\xfe" + must(xunicode.UTF16(xunicode.LittleEndian, xunicode.IgnoreBOM).NewEncoder().String(`<?xml version="1.0" encoding="UTF-16"?><package xmlns="http://www.idpf.org/2007/opf"><metadata><meta name="cover" content="é"/></metadata><manifest><item id="é"/></manifest></package>`)),
			out:  `<?xml version="1.0" encoding="UTF-8"?><package xmlns="http://www.idpf.org/2007/opf"><metadata><meta name="cover" content="é"/></metadata><manifest><item id="é" properties="cover-image"/></manifest></package>`,
		},
		{
			name: "utf-16be without bom or encoding",
			in:   must(xunicode.UTF16(xunicode.BigEndian, xunicode.IgnoreBOM).NewEncoder().String(`<?xml version="1.0"?><package xmlns="http://www.idpf.org/2007/opf"><manifest><item id="cover"/></manifest></package>`)),
			out:  `<?xml version="1.0"?><package xmlns="http://www.idpf.org/2007/opf"><manifest><item id="cover" properties="cover-image"/></manifest></package>`,
		},
		{
			name: "utf-32le with bom",
			in:   "\xff\xfe\x00\x00" + must(utf32.UTF32(utf32.LittleEndian, utf32.IgnoreBOM).NewEncoder().String(`<package xmlns="http://www.idpf.org/2007/opf"><manifest><item id="cover"/></manifest></package>`)),
			out:  `<package xmlns="http://www.idpf.org/2007/opf"><manifest><item id="cover" properties="cover-image"/></manifest></package>`,
		},
		{
			name: "iso-8859-1",
			in:   "<?xml version='1.0' encoding='ISO-8859-1' standalone='yes'?>\n<package xmlns=\"http://www.idpf.org/2007/opf\"><metadata><dc:title xmlns:dc=\"http://purl.org/dc/elements/1.1/\">Caf\xe9</dc:title></metadata></package>",
			out:  "<?xml version=\"1.0\" encoding=\"UTF-8\" standalone=\"yes\"?>\n<package xmlns=\"http://www.idpf.org/2007/opf\"><metadata><dc:title xmlns:dc=\"http://purl.org/dc/elements/1.1/\">Café</dc:title></metadata></package>",
		},
		{
			name: "xml decl without version or quotes",
			in:   "<?xml encoding=latin1?><package xmlns=\"http://www.idpf.org/2007/opf\">\xe9</package>",
			out:  `<?xml version="1.0" encoding="UTF-8"?><package xmlns="http://www.idpf.org/2007/opf">é</package>`,
		},
		{
			name: "keep utf-8 bom and declaration",
			in:   "\xef\xbb\xbf<?xml version='1.0' encoding='utf-8'?><package xmlns=\"http://www.idpf.org/2007/opf\">é</package>",
		},
		{
			// note: browsers do this too
			name: "utf-16 declared without utf-16 bytes",
			in:   `<?xml version="1.0" encoding="UTF-16"?><package xmlns="http://www.idpf.org/2007/opf">é</package>`,
			out:  `<?xml version="1.0" encoding="UTF-8"?><package xmlns="http://www.idpf.org/2007/opf">é</package>`,
		},
		{
			name: "unknown encoding",
			in:   "<?xml version=\"1.0\" encoding=\"x-unknown\"?><package xmlns=\"http://www.idpf.org/2007/opf\">\xe9</package>",
		},
		{
			name: "replacement encoding",
			in:   "<?xml version=\"1.0\" encoding=\"ISO-2022-KR\"?><package xmlns=\"http://www.idpf.org/2007/opf\">x</package>",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			want := tc.out
			if want == "" {
				want = tc.in
			}
			if got := testTransformOPF(t, tc.in); got != want {
				t.Errorf("got:\n%s\nwant:\n%s", got, want)
			}
		})
	}
}

func must[T any](v T, err error) T {
	if err != nil {
		panic(err)
	}
	return v
}

func testTransformOPF(t *testing.T, in string) string {
	t.Helper()
	var a, b bytes.Buffer
	if err := transformOPF(&a, strings.NewReader(in), nil); err != nil {
		t.Fatalf("transform: %v", err)
	}
	// one-byte reader to ensure streaming works correctly
	if err := transformOPF(&b, iotest.OneByteReader(strings.NewReader(in)), nil); err != nil {
		t.Fatalf("transform (one byte reader): %v", err)
	}
	if a.String() != b.String() {
		t.Fatalf("output differs with a one byte reader:\n%s\n%s", a.String(), b.String())
	}
	return a.String()
}
