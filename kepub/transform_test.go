package kepub

import (
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"path"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"testing/fstest"
	"testing/iotest"
	"unicode/utf8"

	xunicode "golang.org/x/text/encoding/unicode"
	"golang.org/x/text/encoding/unicode/utf32"
)

func testTransformContent(t *testing.T, in string, opt Options) string {
	t.Helper()
	var a, b bytes.Buffer
	if err := TransformContent(&a, strings.NewReader(in), opt); err != nil {
		t.Fatalf("transform: %v", err)
	}
	// one-byte reader to ensure streaming works correctly
	if err := TransformContent(&b, iotest.OneByteReader(strings.NewReader(in)), opt); err != nil {
		t.Fatalf("transform (one byte reader): %v", err)
	}
	if a.String() != b.String() {
		t.Fatalf("output differs with a one byte reader:\n%s\n%s", a.String(), b.String())
	}
	return a.String()
}

func fragmentDoc(s string) string {
	return `<!DOCTYPE html><head><title>kepubify test</title><meta charset="utf-8"/></head><body>` + s + `</body></html>`
}

func bookInner(t *testing.T, out string) string {
	t.Helper()
	const start, end = `<body><div id="book-columns"><div id="book-inner">`, `</div></div></body>` // yeah, we hard-code it, but it's fine
	i, j := strings.Index(out, start), strings.LastIndex(out, end)
	if i < 0 || j < i {
		t.Fatalf("no wrapper divs in %q", out)
	}
	return out[i+len(start) : j]
}

func TestTransformContent(t *testing.T) {
	opt := Options{
		CSS: []string{"body { color: black; }"},
		Replace: []Replacement{
			{"Test sentence 2.", "Replaced sentence 2."},
			{`<a id="test">`, `<a id="test1">`},
			{"sdfsdfsdf", "dfgdfgdfg"},
			{" sentence 2", ""},
		},
		Smartypants: true,
	}
	in := strings.TrimSpace(`
<?xml version="1.0" charset="utf-8"?>
<!DOCTYPE html>
<html>
<head>
    <title>Kepubify Test</title>
    <meta charset="utf-8">
    <!-- Note: this tests a few of the extended features from my fork of
         x/net/html, but this isn't the focus. Those features are fully tested
         in my tests for that library. -->
</head>
<body>
    <p>Test sentence 1. <a id="test"/> Test sentence 2. <b>Test sentence 3<i>Test sentence 4</p>
    <p>Test sentence 5. <i>"This is quoted"</i> -- <b>and this is not</b>.</p>
    <p>Sentence.<ul><li>Another sentence.</li><li>Another sentence.<ul><li>Another sentence.</li><li>Another sentence.</li></ul></li><li>Another sentence.</li></ul> Another sentence.</p>
    <pre>Test
</pre>
    <table borders><tr><td>test</td></tr></table>
    <p>  </p>
    <p></p>
    <img src="test">
    <p>&nbsp;</p>
    <svg></svg>
</body>
</html>`)
	want := strings.TrimSpace(`
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE html>
<html xmlns="http://www.w3.org/1999/xhtml"><head><meta charset="UTF-8" />
    <title>Kepubify Test</title>
` + "    " + `
    <!-- Note: this tests a few of the extended features from my fork of
         x/net/html, but this isn't the focus. Those features are fully tested
         in my tests for that library. -->
<style type="text/css" class="kobostylehacks">div#book-inner { margin-top: 0; margin-bottom: 0;}</style><style type="text/css" class="kepubify-extracss">body { color: black; }</style></head>
<body><div id="book-columns"><div id="book-inner">
    <p><span class="koboSpan" id="kobo.1.1">Test sentence 1. </span><a id="test1"></a><span class="koboSpan" id="kobo.1.2"> Replaced. </span><b><span class="koboSpan" id="kobo.1.3">Test sentence 3</span><i><span class="koboSpan" id="kobo.1.4">Test sentence 4</span></i></b></p><b><i>
    <p><span class="koboSpan" id="kobo.2.1">Test sentence 5. </span><i><span class="koboSpan" id="kobo.2.2">“This is quoted”</span></i><span class="koboSpan" id="kobo.2.3"> – </span><b><span class="koboSpan" id="kobo.2.4">and this is not</span></b><span class="koboSpan" id="kobo.2.5">.</span></p>
    <p><span class="koboSpan" id="kobo.3.1">Sentence.</span></p><ul><li><span class="koboSpan" id="kobo.4.1">Another sentence.</span></li><li><span class="koboSpan" id="kobo.4.2">Another sentence.</span><ul><li><span class="koboSpan" id="kobo.5.1">Another sentence.</span></li><li><span class="koboSpan" id="kobo.5.2">Another sentence.</span></li></ul></li><li><span class="koboSpan" id="kobo.5.3">Another sentence.</span></li></ul><span class="koboSpan" id="kobo.5.4"> Another sentence.</span><p></p>
    <pre>Test
</pre>
    <table borders=""><tbody><tr><td><span class="koboSpan" id="kobo.6.1">test</span></td></tr></tbody></table>
    <p><span class="koboSpan" id="kobo.7.1">  </span></p>
    <p></p>
    <span class="koboSpan" id="kobo.8.1"><img src="test" /></span>
    <p><span class="koboSpan" id="kobo.9.1">&#160;</span></p>
    <svg xmlns="http://www.w3.org/2000/svg" xmlns:xlink="http://www.w3.org/1999/xlink"></svg>

</i></b></div></div></body></html>`)
	if got := strings.TrimSpace(testTransformContent(t, in, opt)); got != want {
		t.Errorf("got:\n%s\nwant:\n%s", got, want)
	}
}

type partTest struct {
	name     string
	in       string // body fragment, or document if doc
	doc      bool
	opt      Options
	want     string // contents of book-inner, or substring of the output if contains
	contains bool
	not      string // must not be in the output
}

func (tc partTest) run(t *testing.T) {
	t.Run(tc.name, func(t *testing.T) {
		in := tc.in
		if !tc.doc {
			in = fragmentDoc(in)
		}
		out := testTransformContent(t, in, tc.opt)
		if tc.not != "" && strings.Contains(out, tc.not) {
			t.Errorf("output contains %q:\n%s", tc.not, out)
		}
		if tc.contains {
			if !strings.Contains(out, tc.want) {
				t.Errorf("output doesn't contain %q:\n%s", tc.want, out)
			}
			return
		}
		if got := bookInner(t, out); got != tc.want {
			t.Errorf("got:\n%s\nwant:\n%s", got, tc.want)
		}
	})
}

func TestTransformContentParts(t *testing.T) {
	t.Run("Charset", func(t *testing.T) {
		// charset declarations should all be replaced
		for _, tc := range []partTest{
			{name: "charset meta element", in: `<!DOCTYPE html><head><title>Kepubify Test Case</title><meta charset="iso-8859-1"/></head><body></body></html>`},
			{name: "charset meta element (case-insensitive, unchanged)", in: `<!DOCTYPE html><head><title>Kepubify Test Case</title><meta charset="UTf-8"/></head><body></body></html>`},
			{name: "http-equiv content-type charset meta element", in: `<!DOCTYPE html><head><title>Kepubify Test Case</title><meta http-equiv="content-type" content="application/xhtml+xml; charset=iso-8859-1"/></head><body></body></html>`},
			{name: "http-equiv content-type charset meta element (case-insensitive, unchanged)", in: `<!DOCTYPE html><head><title>Kepubify Test Case</title><meta http-equiv="content-type" content="application/xhtml+xml; charset=UTf-8"/></head><body></body></html>`},
		} {
			tc.doc, tc.contains = true, true
			tc.want = `<head><meta charset="UTF-8" /><title>Kepubify Test Case</title><style`
			tc.run(t)
		}
	})

	t.Run("KoboStyles", func(t *testing.T) {
		for _, tc := range []partTest{
			{
				name:     "add kobo style hacks",
				in:       `<!DOCTYPE html><head><title>Kepubify Test Case</title><meta charset="utf-8"/></head><body></body></html>`,
				doc:      true,
				contains: true,
				want:     `<style type="text/css" class="kobostylehacks">div#book-inner { margin-top: 0; margin-bottom: 0;}</style></head>`,
			},
			{
				name:     "replace existing kepubify styles",
				in:       `<!DOCTYPE html><head><title>Kepubify Test Case</title><style type="text/css" class="kobostylehacks">old</style><style type="text/css" class="kepubify-extracss">old</style><style type="text/css" class="kepubify-hyphenate">old</style><style>p { color: red }</style></head><body></body></html>`,
				doc:      true,
				opt:      Options{CSS: []string{"new"}},
				contains: true,
				want:     `<title>Kepubify Test Case</title><style>p { color: red }</style><style type="text/css" class="kobostylehacks">div#book-inner { margin-top: 0; margin-bottom: 0;}</style><style type="text/css" class="kepubify-extracss">new</style></head>`,
			},
			{
				name:     "replace existing kobo styles",
				in:       `<!DOCTYPE html><head><title>Kepubify Test Case</title><style xmlns="http://www.w3.org/1999/xhtml" type="text/css" id="kobostylehacks">div#book-inner p, div#book-inner div { font-size: 1.0em; }</style></head><body></body></html>`,
				doc:      true,
				contains: true,
				want:     `<title>Kepubify Test Case</title><style type="text/css" class="kobostylehacks">div#book-inner { margin-top: 0; margin-bottom: 0;}</style></head>`,
			},
			{
				name:     "keep other styles with similar names",
				in:       `<!DOCTYPE html><head><title>Kepubify Test Case</title><style id="kobostylehacks2">a</style><style class="x-kepubify-y kepubifyz">b</style></head><body></body></html>`,
				doc:      true,
				contains: true,
				want:     `<style id="kobostylehacks2">a</style><style class="x-kepubify-y kepubifyz">b</style><style type="text/css" class="kobostylehacks">`,
			},
		} {
			tc.run(t)
		}
	})

	t.Run("KoboDivs", func(t *testing.T) {
		for _, tc := range []partTest{
			{name: "no content", in: ``, want: ``},
			{name: "already has divs", in: `<div id="book-columns"><div id="book-inner"></div></div>`, want: ``},
			// with spans
			{name: "single text node", in: `test`, want: `<span class="koboSpan" id="kobo.0.1">test</span>`},
			{name: "multiple elements and children", in: `<p>Test 1</p><p>Test <b>2</b></p><p>Test 3</p>`, want: `<p><span class="koboSpan" id="kobo.1.1">Test 1</span></p><p><span class="koboSpan" id="kobo.2.1">Test </span><b><span class="koboSpan" id="kobo.2.2">2</span></b></p><p><span class="koboSpan" id="kobo.3.1">Test 3</span></p>`},
		} {
			tc.run(t)
		}
	})

	t.Run("KoboSpans", func(t *testing.T) {
		for _, tc := range []partTest{
			{
				name: "no content",
				in:   ``,
				want: ``,
			},
			{
				name: "already has spans",
				in:   `<p><span class="koboSpan" id="kobo.1.1">Test</span></p>`,
				want: `<p><span class="koboSpan" id="kobo.1.1">Test</span></p>`,
			},
			{
				name: "increment segment counter from 1 for every sentence",
				in:   `<p>Sentence 1. Sentence 2. Sentence 3.</p>`,
				want: `<p><span class="koboSpan" id="kobo.1.1">Sentence 1. </span><span class="koboSpan" id="kobo.1.2">Sentence 2. </span><span class="koboSpan" id="kobo.1.3">Sentence 3.</span></p>`,
			},
			{
				name: "increment paragraph counter from 1 and reset segment counter for every p, ul, ol, or table",
				in:   `<p>Sentence 1. Sentence 2.</p><p>Sentence 3.</p><ul><li>Sentence 4</li><li>Sentence 5</li></ul><ol><li>Sentence 6</li><li>Sentence 7</li></ol><table><tbody><tr><td>Test</td></tr><tr><td>Test</td></tr></tbody></table>`,
				want: `<p><span class="koboSpan" id="kobo.1.1">Sentence 1. </span><span class="koboSpan" id="kobo.1.2">Sentence 2.</span></p><p><span class="koboSpan" id="kobo.2.1">Sentence 3.</span></p><ul><li><span class="koboSpan" id="kobo.3.1">Sentence 4</span></li><li><span class="koboSpan" id="kobo.3.2">Sentence 5</span></li></ul><ol><li><span class="koboSpan" id="kobo.4.1">Sentence 6</span></li><li><span class="koboSpan" id="kobo.4.2">Sentence 7</span></li></ol><table><tbody><tr><td><span class="koboSpan" id="kobo.5.1">Test</span></td></tr><tr><td><span class="koboSpan" id="kobo.5.2">Test</span></td></tr></tbody></table>`,
			},
			{
				name: "merge stray text at the end of lines into the next sentence (between regexp matches)",
				in:   `<p>Sentence 1. Sentence 2. Stray text` + "\n" + `Another sentence.</p>`,
				want: `<p><span class="koboSpan" id="kobo.1.1">Sentence 1. </span><span class="koboSpan" id="kobo.1.2">Sentence 2. </span><span class="koboSpan" id="kobo.1.3">Stray text` + "\n" + `Another sentence.</span></p>`,
			},
			{
				name: "don't lose stray text not part of a sentence (after the last regexp match)",
				in:   `<p>Sentence 1. Sentence 2. Stray text</p>`,
				want: `<p><span class="koboSpan" id="kobo.1.1">Sentence 1. </span><span class="koboSpan" id="kobo.1.2">Sentence 2. </span><span class="koboSpan" id="kobo.1.3">Stray text</span></p>`,
			},
			{
				name: "preserve but don't wrap extra whitespace outside of P elements",
				in:   `<p>This is a test.` + "\n" + `    This is another sentence on the next line.<span> </span>Another sentence.</p>` + "\n" + "    <p>Another paragraph.</p><p> </p><p></p>",
				want: `<p><span class="koboSpan" id="kobo.1.1">This is a test.` + "\n" + `    </span><span class="koboSpan" id="kobo.1.2">This is another sentence on the next line.</span><span> </span><span class="koboSpan" id="kobo.1.3">Another sentence.</span></p>` + "\n" + `    <p><span class="koboSpan" id="kobo.2.1">Another paragraph.</span></p><p><span class="koboSpan" id="kobo.3.1"> </span></p><p></p>`,
			},
			{
				name: "preserve and split segments on formatting and links",
				in:   `<p>Sentence<b> 1. </b>Sentence <span>2. Se</span>nten<a href="test.html">ce 3. Another word</a></p>`,
				want: `<p><span class="koboSpan" id="kobo.1.1">Sentence</span><b><span class="koboSpan" id="kobo.1.2"> 1. </span></b><span class="koboSpan" id="kobo.1.3">Sentence </span><span><span class="koboSpan" id="kobo.1.4">2. </span><span class="koboSpan" id="kobo.1.5">Se</span></span><span class="koboSpan" id="kobo.1.6">nten</span><a href="test.html"><span class="koboSpan" id="kobo.1.7">ce 3. </span><span class="koboSpan" id="kobo.1.8">Another word</span></a></p>`,
			},
			{
				name: "preserve and split segments on nested formatting and links",
				in:   `<p>Sentence<b> 1. Sente<i>nce <span>2. Se</span>nt</i>en<a href="test.html">ce 3. Another word</a></b></p>`,
				want: `<p><span class="koboSpan" id="kobo.1.1">Sentence</span><b><span class="koboSpan" id="kobo.1.2"> 1. </span><span class="koboSpan" id="kobo.1.3">Sente</span><i><span class="koboSpan" id="kobo.1.4">nce </span><span><span class="koboSpan" id="kobo.1.5">2. </span><span class="koboSpan" id="kobo.1.6">Se</span></span><span class="koboSpan" id="kobo.1.7">nt</span></i><span class="koboSpan" id="kobo.1.8">en</span><a href="test.html"><span class="koboSpan" id="kobo.1.9">ce 3. </span><span class="koboSpan" id="kobo.1.10">Another word</span></a></b></p>`,
			},
			{
				name: "nested lists/paragraphs should not reset numbering once out of scope",
				in:   `<p>Sentence.<ul><li>Another sentence.</li><li>Another sentence.<ul><li>Another sentence.</li><li>Another sentence.</li></ul></li><li>Another sentence.</li></ul> Another sentence.</p>`,
				want: `<p><span class="koboSpan" id="kobo.1.1">Sentence.</span></p><ul><li><span class="koboSpan" id="kobo.2.1">Another sentence.</span></li><li><span class="koboSpan" id="kobo.2.2">Another sentence.</span><ul><li><span class="koboSpan" id="kobo.3.1">Another sentence.</span></li><li><span class="koboSpan" id="kobo.3.2">Another sentence.</span></li></ul></li><li><span class="koboSpan" id="kobo.3.3">Another sentence.</span></li></ul><span class="koboSpan" id="kobo.3.4"> Another sentence.</span><p></p>`,
			},
			{
				name: "don't touch the contents of script, style, pre, audio, video tags",
				in:   `<p>Touch this.</p><script>not this</script><style>or this</style><pre>or this</pre><audio>or this</audio><video>or this</video><p>Touch this.</p>`,
				want: `<p><span class="koboSpan" id="kobo.1.1">Touch this.</span></p><script>not this</script><style>or this</style><pre>or this</pre><audio>or this</audio><video>or this</video><p><span class="koboSpan" id="kobo.2.1">Touch this.</span></p>`,
			},
			{
				name: "treat an img as a new paragraph and add a span around it",
				in:   `<p>One.</p><img src="test"><p>Three.</p>`,
				want: `<p><span class="koboSpan" id="kobo.1.1">One.</span></p><span class="koboSpan" id="kobo.2.1"><img src="test" /></span><p><span class="koboSpan" id="kobo.3.1">Three.</span></p>`,
			},
			{
				name: "don't increment paragraph counter if no spans were added",
				in:   `<p>One.</p><p> </p><p><!-- comment --></p><p>Two.</p><p><b>Three.</b></p>`,
				want: `<p><span class="koboSpan" id="kobo.1.1">One.</span></p><p><span class="koboSpan" id="kobo.2.1"> </span></p><p><!-- comment --></p><p><span class="koboSpan" id="kobo.3.1">Two.</span></p><p><b><span class="koboSpan" id="kobo.4.1">Three.</span></b></p>`,
			},
			{
				name: "don't add spans to svg and math elements",
				in:   `<svg xmlns="http://www.w3.org/2000/svg"><g><text font-size="24" y="20" x="0">kepubify</text></g></svg><math xmlns="http://www.w3.org/1998/Math/MathML"><mi>x</mi><mo>=</mo><mfrac><mrow><mo>-</mo><mi>b</mi><mo>±</mo><msqrt><msup><mi>b</mi><mn>2</mn></msup><mo>-</mo><mn>4</mn><mi>a</mi><mi>c</mi></msqrt></mrow><mrow><mn>2</mn><mi>a</mi></mrow></mfrac></math>`,
				want: `<svg xmlns="http://www.w3.org/2000/svg" xmlns:xlink="http://www.w3.org/1999/xlink"><g><text font-size="24" y="20" x="0">kepubify</text></g></svg><math xmlns="http://www.w3.org/1998/Math/MathML"><mi>x</mi><mo>=</mo><mfrac><mrow><mo>-</mo><mi>b</mi><mo>±</mo><msqrt><msup><mi>b</mi><mn>2</mn></msup><mo>-</mo><mn>4</mn><mi>a</mi><mi>c</mi></msqrt></mrow><mrow><mn>2</mn><mi>a</mi></mrow></mfrac></math>`,
			},
			{
				name: "also increment paragraph counter on heading tags and don't split sentences after colons or if there isn't any spaces after a period",
				in:   `<div class="img_container"><p class="ad_image"><img src="bookwire_ad_cover1.jpg" alt="image"/></p></div>` + "\n" + `    <h2 class="subheadline">The Christmas Collection: All Of Your Favourite Classic Christmas Stories, Novels, Poems, Carols in One Ebook</h2>` + "\n" + `    <p class="subheadline2"></p>` + "\n" + `    <p class="metadata">Carr, Annie Roe</p>`,
				want: `<div class="img_container"><p class="ad_image"><span class="koboSpan" id="kobo.1.1"><img src="bookwire_ad_cover1.jpg" alt="image" /></span></p></div>` + "\n" + `    <h2 class="subheadline"><span class="koboSpan" id="kobo.2.1">The Christmas Collection: All Of Your Favourite Classic Christmas Stories, Novels, Poems, Carols in One Ebook</span></h2>` + "\n" + `    <p class="subheadline2"></p>` + "\n" + `    <p class="metadata"><span class="koboSpan" id="kobo.3.1">Carr, Annie Roe</span></p>`,
			},
		} {
			tc.run(t)
		}
	})

	t.Run("AddStyle", func(t *testing.T) {
		partTest{
			name:     "add style to head",
			in:       `<!DOCTYPE html><html><head><title>Kepubify Test</title></head><body></body></html>`,
			doc:      true,
			opt:      Options{CSS: []string{"div > div { color: black; }"}},
			contains: true,
			want:     `<title>Kepubify Test</title><style type="text/css" class="kobostylehacks">div#book-inner { margin-top: 0; margin-bottom: 0;}</style><style type="text/css" class="kepubify-extracss">div > div { color: black; }</style></head>`,
		}.run(t)
	})

	t.Run("SmartyPants", func(t *testing.T) {
		smarty := Options{Smartypants: true}
		for _, tc := range []partTest{
			{
				name: "smart punctuation",
				in:   `<p>This is a test sentence to test smartypants' conversion of "quotation marks", dashes like - / -- / ---, and symbols like (c).</p>`,
				want: `<p><span class="koboSpan" id="kobo.1.1">This is a test sentence to test smartypants’ conversion of “quotation marks”, dashes like - / – / —, and symbols like ©.</span></p>`,
			},
			{
				name: "skip pre, code, style, and script elements",
				in:   `<p>This is a test sentence to test smartypants' conversion of <code>"quotation marks"</code>, dashes like <pre>- / -- / ---</pre>, and symbols like (c).</p><style>div{font-family:"Test"}</style><script>var a="test"</script>`,
				want: `<p><span class="koboSpan" id="kobo.1.1">This is a test sentence to test smartypants’ conversion of </span><code><span class="koboSpan" id="kobo.1.2">"quotation marks"</span></code><span class="koboSpan" id="kobo.1.3">, dashes like </span></p><pre>- / -- / ---</pre><span class="koboSpan" id="kobo.1.4">, and symbols like ©.</span><p></p><style>div{font-family:"Test"}</style><script>var a="test"</script>`,
			},
			{
				name: "properly handle entity escaping",
				in:   `<p>&amp;&quot;&lt;&gt;&quot;</p><pre>&quot;</pre>`,
				want: `<p><span class="koboSpan" id="kobo.1.1">&amp;“&lt;&gt;”</span></p><pre>"</pre>`,
			},
		} {
			tc.opt = smarty
			tc.run(t)
		}
	})

	t.Run("CleanHTML", func(t *testing.T) {
		for _, tc := range []partTest{
			{
				name: "remove adobe adept metadata",
				in:   `<meta charset="utf-8"/><meta name="Adept.expected.resource"/>`,
				// the charset declaration is replaced by polyglot.Writer
				want: ``,
				not:  `Adept`,
			},
			{
				name: "remove useless MS Word tags",
				in:   `<o:p></o:p><st1:test></st1:test><div><o:p> dfg </o:p><o:p></o:p></div>`,
				want: `<div><span class="koboSpan" id="kobo.0.1"> dfg </span></div>`,
			},
			{
				name: "remove useless MS Word tags (declared prefixes)",
				in:   `<!DOCTYPE html><html xmlns:o="urn:schemas-microsoft-com:office:office" xmlns:st1="urn:schemas-microsoft-com:office:smarttags"><body><o:p></o:p><st1:test> <!-- x --> </st1:test><div><o:p> dfg </o:p><o:p>&nbsp;</o:p></div></body></html>`,
				doc:  true,
				want: `<div><o:p><span class="koboSpan" id="kobo.0.1"> dfg </span></o:p></div>`,
			},
			{
				name: "remove unicode replacement chars",
				in:   "\ufffd<p>test\ufffding</p><p>asd<b>fgh</b></p>",
				// the character is removed after adding the spans, so the span
				// around it is empty
				want: `<span class="koboSpan" id="kobo.0.1"></span><p><span class="koboSpan" id="kobo.1.1">testing</span></p><p><span class="koboSpan" id="kobo.2.1">asd</span><b><span class="koboSpan" id="kobo.2.2">fgh</span></b></p>`,
			},
		} {
			tc.run(t)
		}
	})
}

func TestTransformContentReplacements(t *testing.T) {
	const corpus = `<!DOCTYPE html><html><head><title></title></head><body><b>Lorem ipsum</b> dolor sit amet, <a href="https://example.com">consectetur adipiscing elit</a>, sed do eiusmod tempor incididunt ut labore et dolore magna aliqua. Ut enim ad minim veniam, quis nostrud exercitation ullamco laboris nisi ut aliquip ex ea commodo consequat. Duis aute irure dolor in reprehenderit in voluptate velit esse cillum dolore eu fugiat nulla pariatur. Excepteur sint occaecat cupidatat non proident, sunt in culpa qui officia deserunt mollit anim id est laborum.</body></html>`
	plain := testTransformContent(t, corpus, Options{})
	for _, tc := range []struct {
		name string
		repl []string
	}{
		{"simple removal", []string{" ipsum", ""}},
		{"simple replacement", []string{". ", "_ "}},
		{"complex removal", []string{"<b>", ""}},
		{"long replacement", []string{"ipsum", strings.Repeat(".", 4096)}},
		{"simple chained", []string{"ipsum", "test1", "amet", "test2", "commodo", ""}},
		{"ordered chained", []string{"ipsum", "Lorem", "Lorem", "test1", "ipsum", ""}},
		{"overlapping chained", []string{"Lorem", "test1", "test1", "test2", "ipsum", "test2 ipsum", "test2", "test3"}},
		{"complex chained", []string{"Lorem", "ipsum", "or", "ar", "dolar", "dolor", "</", "__________", "________", "__</", "_", " ", ". ", "; "}},
		{"empty find", []string{"", "x", "ipsum", "y"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var opt Options
			want := plain
			for i := 0; i < len(tc.repl); i += 2 {
				opt.Replace = append(opt.Replace, Replacement{tc.repl[i], tc.repl[i+1]})
				if tc.repl[i] != "" {
					want = strings.ReplaceAll(want, tc.repl[i], tc.repl[i+1])
				}
			}
			if want == plain {
				t.Fatal("no replacements")
			}
			if got := testTransformContent(t, corpus, opt); got != want {
				t.Errorf("got:\n%s\nwant:\n%s", got, want)
			}
		})
	}
}
func TestTransformContentEdgeCases(t *testing.T) {
	for _, tc := range []partTest{
		{
			// #104
			name: "self-closing tags of non-void elements are empty",
			in:   `<p>One.<i/></p><p>Two.<b/> <span/><a id="x"/><td/></p><div/>`,
			want: `<p><span class="koboSpan" id="kobo.1.1">One.</span><i></i></p><p><span class="koboSpan" id="kobo.2.1">Two.</span><b></b><span class="koboSpan" id="kobo.2.2"> </span><span></span><a id="x"></a><td></td></p><div></div>`,
		},
		{
			name: "self-closing tags in foreign content",
			in:   `<p>x</p><svg><g/><rect/></svg><math><mi/></math>`,
			want: `<p><span class="koboSpan" id="kobo.1.1">x</span></p><svg xmlns="http://www.w3.org/2000/svg" xmlns:xlink="http://www.w3.org/1999/xlink"><g></g><rect></rect></svg><math xmlns="http://www.w3.org/1998/Math/MathML"><mi></mi></math>`,
		},
		{
			// #98
			name:     "hyphenation stylesheet",
			opt:      Options{Hyphenate: new(true)},
			contains: true,
			want:     "h1, h2, h3, h4, h5, h6, td,\nh1 *, h2 *, h3 *, h4 *, h5 *, h6 *, td * {",
		},
		{
			name:     "no hyphenation stylesheet",
			opt:      Options{Hyphenate: new(false)},
			contains: true,
			want:     `<style type="text/css" class="kepubify-nohyphenate">* {`,
		},
		{
			// #95
			name:     "XML declaration after a comment",
			in:       `<!-- generated --><?xml version="1.0" encoding="iso-8859-1"?><!DOCTYPE html PUBLIC "-//W3C//DTD XHTML 1.1//EN" "http://www.w3.org/TR/xhtml11/DTD/xhtml11.dtd"><html><body>x</body></html>`,
			doc:      true,
			contains: true,
			want:     "<?xml version=\"1.0\" encoding=\"UTF-8\"?>\n<!-- generated -->\n" + `<!DOCTYPE html PUBLIC "-//W3C//DTD XHTML 1.1//EN" "http://www.w3.org/TR/xhtml11/DTD/xhtml11.dtd">` + "\n<html",
		},
		{
			// #95
			name: "XML declaration in the body",
			in:   `<p>x</p><?xml version="1.0"?><p>y</p>`,
			want: `<p><span class="koboSpan" id="kobo.1.1">x</span></p><!--?xml version="1.0"?--><p><span class="koboSpan" id="kobo.2.1">y</span></p>`,
			not:  `<?xml`,
		},
		{
			name:     "malformed doctype",
			in:       `<!DOCTYPE"x><html><body>x</body></html>`,
			doc:      true,
			contains: true,
			want:     `<html xmlns="http://www.w3.org/1999/xhtml"><head>`,
			not:      `DOCTYPE`,
		},
		{
			name:     "no doctype",
			in:       "\ufeff<?xml version='1.0'?>\n<html><body>x</body></html>",
			doc:      true,
			contains: true,
			want:     "<?xml version=\"1.0\" encoding=\"UTF-8\"?>\n" + `<html xmlns="http://www.w3.org/1999/xhtml">`,
		},
		{
			name: "noscript contents are markup",
			in:   `<noscript><p>a. b</p></noscript>`,
			want: `<noscript><p><span class="koboSpan" id="kobo.1.1">a. </span><span class="koboSpan" id="kobo.1.2">b</span></p></noscript>`,
		},
		{
			name: "no spans in textarea, title and xmp",
			in:   `<textarea>a. b</textarea><svg><title>c</title></svg><title>d. e</title><xmp>f</xmp>`,
			want: `<textarea>a. b</textarea><svg xmlns="http://www.w3.org/2000/svg" xmlns:xlink="http://www.w3.org/1999/xlink"><title>c</title></svg><title>d. e</title><pre>f</pre>`,
		},
		{
			name: "CDATA sections are text",
			in:   `<p>a<![CDATA[ <b>. ]]>c</p>`,
			want: `<p><span class="koboSpan" id="kobo.1.1">a &lt;b&gt;. </span><span class="koboSpan" id="kobo.1.2">c</span></p>`,
		},
		{
			name: "backticks are kept",
			in:   "<p>a`b ``c''</p>",
			opt:  Options{Smartypants: true},
			want: "<p><span class=\"koboSpan\" id=\"kobo.1.1\">a`b “c”</span></p>",
		},
		{
			name: "invalid UTF-8 is removed",
			in:   "<p>a\xffb\x01c</p>",
			want: `<p><span class="koboSpan" id="kobo.1.1">abc</span></p>`,
		},
		{
			name: "bogus comment <!>",
			in:   `<p>a<!>b</p>`,
			want: `<p><span class="koboSpan" id="kobo.1.1">a</span><!----><span class="koboSpan" id="kobo.1.2">b</span></p>`,
		},
		{
			name: "svg with xmlns:xlink",
			in:   `<svg xmlns:xlink="http://www.w3.org/1999/xlink"><rect/></svg>`,
			want: `<svg xmlns:xlink="http://www.w3.org/1999/xlink" xmlns="http://www.w3.org/2000/svg"><rect></rect></svg>`,
		},
		{
			name: "foreign elements named like HTML void elements",
			in:   `<p>a</p><svg><wbr>x</wbr></svg>`,
			want: `<p><span class="koboSpan" id="kobo.1.1">a</span></p><svg xmlns="http://www.w3.org/2000/svg" xmlns:xlink="http://www.w3.org/1999/xlink"><wbr>x</wbr></svg>`,
		},
		{
			name:     "stylesheets with < or &",
			opt:      Options{CSS: []string{`p::after { content: "<&]]>"; }`}},
			contains: true,
			want:     "<style type=\"text/css\" class=\"kepubify-extracss\">/*<![CDATA[*/\np::after { content: \"<&]]]]><![CDATA[>\"; }\n/*]]>*/</style>",
		},
	} {
		tc.run(t)
	}
}

func TestTransformContentStreaming(t *testing.T) {
	// tests for lookahead
	for _, tc := range []partTest{
		{
			name:     "wrapper divs after whitespace and comments",
			in:       "\n<!-- a --> <div id=\"book-columns\"> <!-- b --><div id=\"book-inner\"><p>x</p></div></div>\n",
			contains: true,
			want:     "<body>\n<!-- a --> <div id=\"book-columns\"> <!-- b --><div id=\"book-inner\"><p><span class=\"koboSpan\" id=\"kobo.1.1\">x</span></p></div></div>\n</body>",
		},
		{
			name: "wrapper divs after text",
			in:   "x<div id=\"book-columns\"><div id=\"book-inner\"></div></div>",
			want: "<span class=\"koboSpan\" id=\"kobo.0.1\">x</span><div id=\"book-columns\"><div id=\"book-inner\"></div></div>",
		},
		{
			name: "book-columns without book-inner",
			in:   " <div id=\"book-columns\"> <p>x</p></div>",
			want: " <div id=\"book-columns\"> <p><span class=\"koboSpan\" id=\"kobo.1.1\">x</span></p></div>",
		},
		{
			name: "existing spans after an image",
			in:   `<p><img src="x"/><span class="koboSpan" id="kobo.1.1">x</span>y</p>`,
			want: `<p><span class="koboSpan" id="kobo.1.1"><img src="x" /></span><span class="koboSpan" id="kobo.1.1">x</span><span class="koboSpan" id="kobo.1.2">y</span></p>`,
		},
		{
			name: "existing spans after whitespace",
			in:   "<div> <p><span class=\"koboSpan\" id=\"kobo.1.1\">x</span>y</p></div>",
			want: "<div> <p><span class=\"koboSpan\" id=\"kobo.1.1\">x</span>y</p></div>",
		},
		{
			name: "whitespace sentence outside p",
			in:   "<div>a. \u00a0\u2003</div><div>\u00a0 <b>b</b></div>",
			want: "<div><span class=\"koboSpan\" id=\"kobo.0.1\">a. </span>&#160;\u2003</div><div>&#160; <b><span class=\"koboSpan\" id=\"kobo.0.2\">b</span></b></div>",
		},
	} {
		tc.run(t)
	}

	t.Run("long text", func(t *testing.T) {
		// longer than the chunk size of the Reader
		var text strings.Builder
		for i := range 20000 {
			text.WriteString("Sentence number " + strings.Repeat("x", i%7) + `. "Quoted" -- `)
		}
		text.WriteString(strings.Repeat(" ", 100000) + "end")
		var in, want strings.Builder
		in.WriteString("<p>" + text.String() + "</p><div>" + strings.Repeat("\u00a0", 1000) + "</div>")
		want.WriteString("<p>")
		for i, s := range splitSentencesRegexp(text.String()) {
			var sp smartypants
			want.WriteString(`<span class="koboSpan" id="kobo.1.` + strconv.Itoa(i+1) + `">` + string(sp.Convert(nil, []byte(s), true)) + "</span>")
		}
		want.WriteString("</p><div>" + strings.Repeat("&#160;", 1000) + "</div>")
		got := bookInner(t, testTransformContent(t, fragmentDoc(in.String()), Options{Smartypants: true}))
		if got != want.String() {
			i := 0
			for i < len(got) && i < len(want.String()) && got[i] == want.String()[i] {
				i++
			}
			t.Errorf("differs at %d: %q, want %q", i, got[i:min(len(got), i+100)], want.String()[i:min(len(want.String()), i+100)])
		}
	})

	t.Run("long whitespace outside p", func(t *testing.T) {
		// treated as text after holdMax bytes
		in := "<div>" + strings.Repeat(" ", holdMax+1) + "</div>"
		want := "<div><span class=\"koboSpan\" id=\"kobo.0.1\">" + strings.Repeat(" ", holdMax+1) + "</span></div>"
		if got := bookInner(t, testTransformContent(t, fragmentDoc(in), Options{})); got != want {
			t.Errorf("got %q", got[:min(len(got), 100)])
		}
	})
}

func TestTransformContentCharset(t *testing.T) {
	latin1 := "<?xml version=\"1.0\" encoding=\"iso-8859-1\"?><html><body><p>caf\xe9</p></body></html>"
	meta := "<html><head><meta charset=\"windows-1252\"></head><body><p>\x93x\x94</p></body></html>"
	for _, tc := range []struct {
		name, in, charset, want string
	}{
		{"auto, XML declaration", latin1, "auto", "café"},
		{"auto, meta", meta, "auto", "“x”"},
		{"auto, UTF-8", "<p>café</p>", "auto", "café"},
		{"auto, undeclared latin1", "<p>caf\xe9</p>", "auto", "café"},
		{"label", "<p>caf\xe9</p>", "latin1", "café"},
		{"default, XML declaration", latin1, "", "café"},
		{"default, UTF-8", "<p>café</p>", "", "café"},
		{"UTF-8", "<p>café</p>", "UTF-8", "café"},
		{"UTF-8, wrong declaration", "<?xml version=\"1.0\" encoding=\"iso-8859-1\"?><p>café</p>", "UTF-8", "café"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out := testTransformContent(t, tc.in, Options{Charset: tc.charset})
			if !strings.Contains(out, `<span class="koboSpan" id="kobo.1.1">`+tc.want+`</span>`) {
				t.Errorf("got %s", out)
			}
			if strings.Contains(out, "iso-8859-1") || strings.Contains(out, "windows-1252") {
				t.Errorf("charset not replaced: %s", out)
			}
		})
	}
	if err := TransformContent(io.Discard, strings.NewReader(""), Options{Charset: "nonexistent"}); err == nil {
		t.Error("no error for an invalid charset")
	}
}

func TestTransformContentErrors(t *testing.T) {
	errRead := errors.New("read error")
	doc := fragmentDoc(strings.Repeat("<p>Some text. More text.</p>", 10000))
	for _, n := range []int{0, 10, 100, 10000, 100000} {
		r := io.MultiReader(strings.NewReader(doc[:n]), iotest.ErrReader(errRead))
		if err := TransformContent(io.Discard, r, Options{}); !errors.Is(err, errRead) {
			t.Errorf("read error after %d bytes: got %v", n, err)
		}
		w := &failWriter{n: n}
		if err := TransformContent(w, strings.NewReader(doc), Options{Replace: []Replacement{{"text", "txet"}}}); !errors.Is(err, errWrite) {
			t.Errorf("write error after %d bytes: got %v", n, err)
		}
	}
}

var errWrite = errors.New("write error")

// failWriter fails after n bytes.
type failWriter struct{ n int }

func (w *failWriter) Write(p []byte) (int, error) {
	if len(p) > w.n {
		n := w.n
		w.n = 0
		return n, errWrite
	}
	w.n -= len(p)
	return len(p), nil
}

func TestTransformContentWarn(t *testing.T) {
	var warns []string
	opt := Options{
		Warn: func(name, msg string) {
			warns = append(warns, name+": "+msg)
		},
	}
	if err := TransformContent(io.Discard, strings.NewReader(fragmentDoc(`<p><o:p>x</o:p></p>`)), opt); err != nil {
		t.Fatalf("transform: %v", err)
	}
	if len(warns) == 0 {
		t.Error("no warnings")
	}
	for _, w := range warns {
		if !strings.HasPrefix(w, ": ") {
			t.Errorf("unexpected warning %q", w)
		}
	}
}

func TestTransformContentReplacementsWrites(t *testing.T) {
	const in = "aab abab ba aaab b"
	for _, repl := range [][][2]string{
		{{"ab", "ba"}, {"ba", "x"}, {"", "y"}, {"aa", "a"}},
		{{"", "y"}},
		nil,
	} {
		var rs []Replacement
		want := in
		for _, r := range repl {
			rs = append(rs, Replacement{r[0], r[1]})
			if r[0] != "" {
				want = strings.ReplaceAll(want, r[0], r[1])
			}
		}
		for _, n := range []int{1, 2, 3, len(in)} {
			var buf bytes.Buffer
			wc := transformContentReplacements(&buf, rs)
			for i := 0; i < len(in); i += n {
				if _, err := wc.Write([]byte(in[i:min(i+n, len(in))])); err != nil {
					t.Fatalf("write: %v", err)
				}
			}
			if err := wc.Close(); err != nil {
				t.Fatalf("close: %v", err)
			}
			if got := buf.String(); got != want {
				t.Errorf("%q, writes of %d bytes: got %q, want %q", repl, n, got, want)
			}
		}
	}
}

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
			in: `<?xml version="1.0" encoding="UTF-8"?>
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
</package>`,
			out: `<?xml version="1.0" encoding="UTF-8"?>
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
</package>`,
		},
		{
			name: "set cover-image property on #cover if meta[name=cover] not present",
			in: `<?xml version="1.0" encoding="UTF-8"?>
<package xmlns="http://www.idpf.org/2007/opf" version="3.0" unique-identifier="uuid_id">
    <metadata xmlns:dc="http://purl.org/dc/elements/1.1/">
        <!-- other stuff left out for brevity -->
    </metadata>
    <manifest>
        <item id="cover" href="book_cover.jpg" media-type="image/jpeg"/>
        <item id="xhtml_text1" href="xhtml/text1.xhtml" media-type="application/xhtml+xml"/>
    </manifest>
</package>`,
			out: `<?xml version="1.0" encoding="UTF-8"?>
<package xmlns="http://www.idpf.org/2007/opf" version="3.0" unique-identifier="uuid_id">
    <metadata xmlns:dc="http://purl.org/dc/elements/1.1/">
        <!-- other stuff left out for brevity -->
    </metadata>
    <manifest>
        <item id="cover" href="book_cover.jpg" media-type="image/jpeg" properties="cover-image"/>
        <item id="xhtml_text1" href="xhtml/text1.xhtml" media-type="application/xhtml+xml"/>
    </manifest>
</package>`,
		},
		{
			name: "remove calibre:timestamp and calibre contributor",
			in: `<?xml version="1.0" encoding="UTF-8"?>
<package xmlns="http://www.idpf.org/2007/opf" version="3.0" unique-identifier="uuid_id">
    <metadata xmlns:dc="http://purl.org/dc/elements/1.1/">
        <!-- --><meta content="whatever" name="calibre:timestamp"/>
        <!-- --><dc:contributor file-as="calibre" role="bkp">calibre (#.#.#) [https://calibre-ebook.com]</dc:contributor>
        <!-- --><dc:contributor>whatever</dc:contributor>
    </metadata>
    <!-- other stuff left out for brevity -->
</package>`,
			out: `<?xml version="1.0" encoding="UTF-8"?>
<package xmlns="http://www.idpf.org/2007/opf" version="3.0" unique-identifier="uuid_id">
    <metadata xmlns:dc="http://purl.org/dc/elements/1.1/">
        <!-- -->
        <!-- -->
        <!-- --><dc:contributor>whatever</dc:contributor>
    </metadata>
    <!-- other stuff left out for brevity -->
</package>`,
		},
		{
			name: "more calibre metadata, epub2 prefixes",
			in: `<?xml version='1.0' encoding='utf-8'?>
<package xmlns="http://www.idpf.org/2007/opf" unique-identifier="uuid_id" version="2.0">
  <metadata xmlns:dc="http://purl.org/dc/elements/1.1/" xmlns:opf="http://www.idpf.org/2007/opf">
    <dc:title>Title</dc:title>
    <dc:contributor opf:file-as="calibre" opf:role="bkp">calibre (5.0.0) [https://calibre-ebook.com]</dc:contributor>
    <dc:contributor opf:role="edt">Editor</dc:contributor>
    <opf:meta name="calibre:timestamp" content="2020-01-01T00:00:00+00:00"/>
  </metadata>
</package>`,
			out: `<?xml version='1.0' encoding='utf-8'?>
<package xmlns="http://www.idpf.org/2007/opf" unique-identifier="uuid_id" version="2.0">
  <metadata xmlns:dc="http://purl.org/dc/elements/1.1/" xmlns:opf="http://www.idpf.org/2007/opf">
    <dc:title>Title</dc:title>
    <dc:contributor opf:role="edt">Editor</dc:contributor>
  </metadata>
</package>`,
		},
		{
			name: "prefixed package document",
			in: `<opf:package xmlns:opf="http://www.idpf.org/2007/opf" xmlns:x="http://purl.org/dc/elements/1.1/" version="2.0">
<opf:metadata><x:contributor opf:role="bkp">calibre</x:contributor><opf:meta name="cover" content="img"/></opf:metadata>
<opf:manifest><opf:item id="img" href="c.jpg" media-type="image/jpeg"/></opf:manifest>
</opf:package>`,
			out: `<opf:package xmlns:opf="http://www.idpf.org/2007/opf" xmlns:x="http://purl.org/dc/elements/1.1/" version="2.0">
<opf:metadata><opf:meta name="cover" content="img"/></opf:metadata>
<opf:manifest><opf:item id="img" href="c.jpg" media-type="image/jpeg" properties="cover-image"/></opf:manifest>
</opf:package>`,
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
			out:  `<package xmlns="http://www.idpf.org/2007/opf"><manifest><item id="cover" properties="cover-image" href="c.xhtml"/></manifest></package>`,
		},
		{
			name: "existing cover-image property",
			in:   `<package xmlns="http://www.idpf.org/2007/opf"><manifest><item id="cover" properties=' cover-image' href="c.jpg"/></manifest></package>`,
			out:  `<package xmlns="http://www.idpf.org/2007/opf"><manifest><item id="cover" properties="cover-image" href="c.jpg"/></manifest></package>`,
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
	if err := transformOPF(&a, strings.NewReader(in)); err != nil {
		t.Fatalf("transform: %v", err)
	}
	// one-byte reader to ensure streaming works correctly
	if err := transformOPF(&b, iotest.OneByteReader(strings.NewReader(in))); err != nil {
		t.Fatalf("transform (one byte reader): %v", err)
	}
	if a.String() != b.String() {
		t.Fatalf("output differs with a one byte reader:\n%s\n%s", a.String(), b.String())
	}
	return a.String()
}

func TestTransformFileFilter(t *testing.T) {
	for _, tc := range []struct {
		fn   string
		want bool
	}{
		{"META-INF/calibre_bookmarks.txt", true},
		{"iTunesMetadata.plist", true},
		{"iTunesArtwork.plist", true},
		{".DS_STORE", true},
		{"__MACOSX/test.txt", true},
		{"thumbs.db", true},
		{"OEBPS/Thumbs.db", true},
		{"OEBPS/.DS_Store", true},
		{"__MACOSX/OEBPS/._text.xhtml", true},
		{"OEBPS/__MACOSX/x", false},
		{"OEBPS/text.xhtml", false},
		{"mimetype", false},
	} {
		if got := transformFileFilter(tc.fn); got != tc.want {
			t.Errorf("%q: got %v, want %v", tc.fn, got, tc.want)
		}
	}
}

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

var testSentences = []string{
	" ! Lorem ipsum dolor, sit amet. Consectetur adipiscing elit?\n Sed do eiusmod tempor incididunt!?! Ut labore et dolore \"magna aliqua.\". Ut enim ad “minim veniam”, quis nostrud exercitation 'ullamco laboris' nisi ut aliquip ex ea commodo consequat?… Duis aute irure dolor in reprehenderit in voluptate velit’s esse cillum dolore eu fugiat nulla pariatur. Excepteur sint occaecat cupidatat non proident, sunt in culpa qui officia deserunt mollit anim id est laborum.      sdfsdfsdf",
	"Lorem ipsum dolor, sit amet. Consectetur adipiscing elit? Sed do eiusmod tempor incididunt!?! Ut labore et dolore magna aliqua.",
	strings.Repeat("Lorem ipsum dolor sit amet. Consectetur adipiscing elit ut labore et dolore magna aliqua. ", 40),
	"                                ",
	"...       !!!       ???       .'.'.'.'   ",
	"test\u00a0.\u0080.\u00a0.",
	"",
	"🌝. 🌝      🌝.    🌝",
	"!",
	"? ",
	"? ?",
	"?  ",
	"  ?  ",
	" ?'  .",
	" ?'  .   ",
	" ?'  .   \xFF",
	" ?'  .   \xFF .",
	" ?'  .   \xe2\x82\x28\xFF",
	" ?'  .   \xe2\x82\x28\xFF .",
	" ?'  .   .\xe2\x82\x28\xFF",
	" ?'  .   .\xe2\x82\x28\xFF .",
	" ?'  .   .'\xe2\x82\x28\xFF",
	" ?'  .   .'\xe2\x82\x28\xFF .",
	" ?'  .   .'\xe2\x82\x28\xFF.",
}

// splitSentences splits s with sentenceSplitter, passing it in parts of
// ~chunkSize bytes, split between characters like Reader.
func splitSentences(s string, chunkSize int) []string {
	var sp sentenceSplitter
	var r []string
	var cur []byte
	for len(s) != 0 {
		part := []byte(s[:partEnd(s, chunkSize)])
		s = s[len(part):]
		for len(part) != 0 {
			i := sp.next(part)
			if i < 0 {
				cur = append(cur, part...)
				break
			}
			cur = append(cur, part[:i]...)
			r = append(r, string(cur))
			cur = cur[:0]
			part = part[i:]
		}
	}
	return append(r, string(cur))
}

func partEnd(s string, n int) int {
	i := min(n, len(s))
	for i < len(s) && !utf8.RuneStart(s[i]) { // don't split utf8 sequences
		i++
	}
	return i
}

func TestSplitSentences(t *testing.T) {
	for _, s := range testSentences {
		want := splitSentencesRegexp(s)
		for _, n := range []int{1, 2, 3, 7, len(s) + 1} {
			got := splitSentences(s, n)
			if strings.Join(got, "\x00") != strings.Join(want, "\x00") {
				t.Errorf("%q in parts of %d: got %q, want %q", s, n, got, want)
			}
		}
	}
}

func BenchmarkSplitSentences(b *testing.B) {
	b.SetParallelism(1) // for more accurate results
	b.Run("Regexp", func(b *testing.B) {
		for b.Loop() {
			for _, v := range testSentences {
				splitSentencesRegexp(v)
			}
		}
	})
	b.Run("StateMachine", func(b *testing.B) {
		for b.Loop() {
			for _, v := range testSentences {
				splitSentences(v, len(v)+1)
			}
		}
	})
}

var sentenceRe = regexp.MustCompile(`((?ms).*?[\.\!\?]['"”’“…]?\s+)`)

func splitSentencesRegexp(str string) (r []string) {
	if matches := sentenceRe.FindAllStringIndex(str, -1); len(matches) == 0 {
		r = []string{str} // nothing matched, use the whole string
	} else {
		var pos int
		r = make([]string, len(matches))
		for i, match := range matches {
			r[i] = str[pos:match[1]] // end of last match to end of the current one
			pos = match[1]
		}
		if len(str) > pos {
			r = append(r, str[pos:]) // rest of the string, if any
		}
	}
	return
}
