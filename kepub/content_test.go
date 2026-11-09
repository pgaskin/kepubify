package kepub

import (
	"bytes"
	"errors"
	"io"
	"strconv"
	"strings"
	"testing"
	"testing/iotest"
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
				name: "existing spans after added spans",
				in:   `<p>Some <span class="koboSpan" id="kobo.1.1">existing</span> <span class="a koboSpan  b" id="kobo.1.2">spans</span><span class="koboSpan" id="x">.</span></p>`,
				want: `<p><span class="koboSpan" id="kobo.1.1">Some </span><span class="koboSpan" id="kobo.1.2">existing</span><span class="koboSpan" id="kobo.1.3"> </span><span class="a b"><span class="koboSpan" id="kobo.1.4">spans</span></span><span id="x"><span class="koboSpan" id="kobo.1.5">.</span></span></p>`,
			},
			{
				name: "reopened existing span after added spans",
				in:   `<p>Some <i><b class="koboSpan" id="kobo.1.1">existing</i> span</b></p>`,
				want: `<p><span class="koboSpan" id="kobo.1.1">Some </span><i><b><span class="koboSpan" id="kobo.1.2">existing</span></b></i><b><span class="koboSpan" id="kobo.1.3"> span</span></b></p>`,
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
			want: `<p><span class="koboSpan" id="kobo.1.1"><img src="x" /></span><span class="koboSpan" id="kobo.1.2">x</span><span class="koboSpan" id="kobo.1.3">y</span></p>`,
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
			text.WriteString("Sentence number ")
			text.WriteString(strings.Repeat("x", i%7))
			text.WriteString(`. "Quoted" -- `)
		}
		text.WriteString(strings.Repeat(" ", 100000))
		text.WriteString("end")
		var in, want strings.Builder
		in.WriteString("<p>")
		in.WriteString(text.String())
		in.WriteString("</p><div>")
		in.WriteString(strings.Repeat("\u00a0", 1000))
		in.WriteString("</div>")
		want.WriteString("<p>")
		for i, s := range splitSentencesRegexp(text.String()) {
			var sp smartypants
			want.WriteString(`<span class="koboSpan" id="kobo.1.`)
			want.WriteString(strconv.Itoa(i + 1))
			want.WriteString(`">`)
			want.WriteString(string(sp.Convert(nil, []byte(s), true)))
			want.WriteString("</span>")
		}
		want.WriteString("</p><div>")
		want.WriteString(strings.Repeat("&#160;", 1000))
		want.WriteString("</div>")
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

func TestTransformContentWarnIgnored(t *testing.T) {
	var warns []string
	opt := Options{
		Warn: func(name, msg string) {
			warns = append(warns, msg)
		},
	}
	doc := `<html xmlns="http://www.w3.org/1999/xhtml" xmlns:epub="http://www.idpf.org/2007/ops"><head><title>t</title></head><body><section epub:type="chapter"><p xmlns:x="u" x:a="1">x</p></section></body></html>`
	if err := TransformContent(io.Discard, strings.NewReader(doc), opt); err != nil {
		t.Fatalf("transform: %v", err)
	}
	if len(warns) != 1 || !strings.Contains(warns[0], `"x:a"`) {
		t.Errorf("got %q, expected only x:a", warns)
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
