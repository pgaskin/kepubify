package kepub

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"path"
	"regexp"
	"slices"
	"strings"
	"testing"
	"testing/fstest"
	"testing/iotest"

	xunicode "golang.org/x/text/encoding/unicode"
	"golang.org/x/text/encoding/unicode/utf32"

	"github.com/pgaskin/kepubify/_/html/golang.org/x/net/html"
)

func TestTransformContent(t *testing.T) {
	c := &Converter{
		extraCSS:      []string{"body { color: black; }"},
		extraCSSClass: []string{"kepubify-test"},
		find: [][]byte{
			[]byte("Test sentence 2."),
			[]byte("<a id=\"test\">"),
			[]byte("sdfsdfsdf"),
			[]byte(" sentence 2"),
		},
		replace: [][]byte{
			[]byte("Replaced sentence 2."),
			[]byte("<a id=\"test1\">"),
			[]byte("dfgdfgdfg"),
			[]byte(nil),
		},
		smartypants: true,
	}

	// yes, I know it isn't valid XML, but I'm testing preserving the XML declaration
	buf := bytes.NewBuffer(nil)
	if err := c.TransformContent(buf, strings.NewReader(strings.TrimSpace(`
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
</html>`))); err != nil {
		t.Fatalf("transform: unexpected error: %v", err)
	}

	if a, b := strings.TrimSpace(buf.String()), strings.TrimSpace(`
<?xml version="1.0" charset="utf-8"?><!DOCTYPE html><html xmlns="http://www.w3.org/1999/xhtml"><head>
    <title>Kepubify Test</title>
    <meta charset="utf-8"/>
    <!-- Note: this tests a few of the extended features from my fork of
         x/net/html, but this isn't the focus. Those features are fully tested
         in my tests for that library. -->
<style type="text/css" class="kobostylehacks">div#book-inner { margin-top: 0; margin-bottom: 0;}</style><style type="text/css" class="kepubify-test">body { color: black; }</style></head>
<body><div id="book-columns"><div id="book-inner">
    <p><span class="koboSpan" id="kobo.1.1">Test sentence 1. </span><a id="test1"></a><span class="koboSpan" id="kobo.1.2"> Replaced. </span><b><span class="koboSpan" id="kobo.1.3">Test sentence 3</span><i><span class="koboSpan" id="kobo.1.4">Test sentence 4</span></i></b></p><b><i>
    <p><span class="koboSpan" id="kobo.2.1">Test sentence 5. </span><i><span class="koboSpan" id="kobo.2.2">“This is quoted”</span></i><span class="koboSpan" id="kobo.2.3"> – </span><b><span class="koboSpan" id="kobo.2.4">and this is not</span></b><span class="koboSpan" id="kobo.2.5">.</span></p>
    <p><span class="koboSpan" id="kobo.3.1">Sentence.</span></p><ul><li><span class="koboSpan" id="kobo.4.1">Another sentence.</span></li><li><span class="koboSpan" id="kobo.4.2">Another sentence.</span><ul><li><span class="koboSpan" id="kobo.5.1">Another sentence.</span></li><li><span class="koboSpan" id="kobo.5.2">Another sentence.</span></li></ul></li><li><span class="koboSpan" id="kobo.5.3">Another sentence.</span></li></ul><span class="koboSpan" id="kobo.5.4"> Another sentence.</span><p></p>
    <pre>Test
</pre>
    <table borders=""><tbody><tr><td><span class="koboSpan" id="kobo.6.1">test</span></td></tr></tbody></table>
    <p><span class="koboSpan" id="kobo.7.1">  </span></p>
    <p></p>
    <span class="koboSpan" id="kobo.8.1"><img src="test"/></span>
    <p><span class="koboSpan" id="kobo.9.1">&#160;</span></p>
    <svg xmlns="http://www.w3.org/2000/svg" xmlns:xlink="http://www.w3.org/1999/xlink"></svg>

</i></b></div></div></body></html>`); a != b {
		t.Error("not equal")
		fmt.Println(a)
		fmt.Println("---")
		fmt.Println(b)
	}
}

func TestTransformContentParts(t *testing.T) {
	t.Run("Charset", func(t *testing.T) {
		transformContentCase{
			Func:     transformContentCharsetUTF8,
			What:     "charset meta element",
			Fragment: false,
			Contains: true,
			In:       `<!DOCTYPE html><head><title>Kepubify Test Case</title><meta charset="iso-8859-1"/></head><body></body></html>`,
			Out:      `meta charset="UTF-8"`,
		}.Run(t)
		transformContentCase{
			Func:     transformContentCharsetUTF8,
			What:     "charset meta element (case-insensitive, unchanged)",
			Fragment: false,
			Contains: true,
			In:       `<!DOCTYPE html><head><title>Kepubify Test Case</title><meta charset="UTf-8"/></head><body></body></html>`,
			Out:      `meta charset="UTf-8"`,
		}.Run(t)
		transformContentCase{
			Func:     transformContentCharsetUTF8,
			What:     "http-equiv content-type charset meta element",
			Fragment: false,
			Contains: true,
			In:       `<!DOCTYPE html><head><title>Kepubify Test Case</title><meta http-equiv="content-type" content="application/xhtml+xml; charset=iso-8859-1"/></head><body></body></html>`,
			Out:      `meta http-equiv="content-type" content="application/xhtml+xml; charset=utf-8"`,
		}.Run(t)
		transformContentCase{
			Func:     transformContentCharsetUTF8,
			What:     "http-equiv content-type charset meta element (case-insensitive, unchanged)",
			Fragment: false,
			Contains: true,
			In:       `<!DOCTYPE html><head><title>Kepubify Test Case</title><meta http-equiv="content-type" content="application/xhtml+xml; charset=UTf-8"/></head><body></body></html>`,
			Out:      `meta http-equiv="content-type" content="application/xhtml+xml; charset=UTf-8"`,
		}.Run(t)
	})

	t.Run("KoboStyles", func(t *testing.T) {
		transformContentCase{
			Func:     transformContentKoboStyles,
			What:     "add kobo style hacks",
			Fragment: false,
			Contains: true,
			In:       `<!DOCTYPE html><head><title>Kepubify Test Case</title><meta charset="utf-8"/></head><body></body></html>`,
			Out:      `kobostylehacks`,
		}.Run(t)
	})

	t.Run("KoboDivs", func(t *testing.T) {
		transformContentCase{
			Func:     transformContentKoboDivs,
			What:     "no content",
			Fragment: true,
			In:       ``,
			Out:      `<div id="book-columns"><div id="book-inner"></div></div>`,
		}.Run(t)

		transformContentCase{
			Func:     transformContentKoboDivs,
			What:     "already has divs",
			Fragment: true,
			In:       `<div id="book-columns"><div id="book-inner"></div></div>`,
			Out:      `<div id="book-columns"><div id="book-inner"></div></div>`,
		}.Run(t)

		transformContentCase{
			Func:     transformContentKoboDivs,
			What:     "single text node",
			Fragment: true,
			In:       `test`,
			Out:      `<div id="book-columns"><div id="book-inner">test</div></div>`,
		}.Run(t)

		transformContentCase{
			Func:     transformContentKoboDivs,
			What:     "multiple elements and children",
			Fragment: true,
			In:       `<p>Test 1</p><p>Test <b>2</b></p><p>Test 3</p>`,
			Out:      `<div id="book-columns"><div id="book-inner"><p>Test 1</p><p>Test <b>2</b></p><p>Test 3</p></div></div>`,
		}.Run(t)
	})

	t.Run("KoboSpans", func(t *testing.T) {
		transformContentCase{
			Func:     transformContentKoboSpans,
			What:     "no content",
			Fragment: true,
			In:       ``,
			Out:      ``,
		}.Run(t)

		transformContentCase{
			Func:     transformContentKoboSpans,
			What:     "already has spans",
			Fragment: true,
			In:       `<p><span class="koboSpan" id="kobo.1.1">Test</span></p>`,
			Out:      `<p><span class="koboSpan" id="kobo.1.1">Test</span></p>`,
		}.Run(t)

		transformContentCase{
			Func:     transformContentKoboSpans,
			What:     "increment segment counter from 1 for every sentence",
			Fragment: true,
			In:       `<p>Sentence 1. Sentence 2. Sentence 3.</p>`,
			Out:      `<p><span class="koboSpan" id="kobo.1.1">Sentence 1. </span><span class="koboSpan" id="kobo.1.2">Sentence 2. </span><span class="koboSpan" id="kobo.1.3">Sentence 3.</span></p>`,
		}.Run(t)

		transformContentCase{
			Func:     transformContentKoboSpans,
			What:     "increment paragraph counter from 1 and reset segment counter for every p, ul, ol, or table",
			Fragment: true,
			In:       `<p>Sentence 1. Sentence 2.</p><p>Sentence 3.</p><ul><li>Sentence 4</li><li>Sentence 5</li></ul><ol><li>Sentence 6</li><li>Sentence 7</li></ol><table><tbody><tr><td>Test</td></tr><tr><td>Test</td></tr></tbody></table>`,
			Out:      `<p><span class="koboSpan" id="kobo.1.1">Sentence 1. </span><span class="koboSpan" id="kobo.1.2">Sentence 2.</span></p><p><span class="koboSpan" id="kobo.2.1">Sentence 3.</span></p><ul><li><span class="koboSpan" id="kobo.3.1">Sentence 4</span></li><li><span class="koboSpan" id="kobo.3.2">Sentence 5</span></li></ul><ol><li><span class="koboSpan" id="kobo.4.1">Sentence 6</span></li><li><span class="koboSpan" id="kobo.4.2">Sentence 7</span></li></ol><table><tbody><tr><td><span class="koboSpan" id="kobo.5.1">Test</span></td></tr><tr><td><span class="koboSpan" id="kobo.5.2">Test</span></td></tr></tbody></table>`,
		}.Run(t)

		transformContentCase{
			Func:     transformContentKoboSpans,
			What:     "merge stray text at the end of lines into the next sentence (between regexp matches)",
			Fragment: true,
			In:       `<p>Sentence 1. Sentence 2. Stray text` + "\n" + `Another sentence.</p>`,
			Out:      `<p><span class="koboSpan" id="kobo.1.1">Sentence 1. </span><span class="koboSpan" id="kobo.1.2">Sentence 2. </span><span class="koboSpan" id="kobo.1.3">Stray text` + "\n" + `Another sentence.</span></p>`,
		}.Run(t)

		transformContentCase{
			Func:     transformContentKoboSpans,
			What:     "don't lose stray text not part of a sentence (after the last regexp match)",
			Fragment: true,
			In:       `<p>Sentence 1. Sentence 2. Stray text</p>`,
			Out:      `<p><span class="koboSpan" id="kobo.1.1">Sentence 1. </span><span class="koboSpan" id="kobo.1.2">Sentence 2. </span><span class="koboSpan" id="kobo.1.3">Stray text</span></p>`,
		}.Run(t)

		transformContentCase{
			Func:     transformContentKoboSpans,
			What:     "preserve but don't wrap extra whitespace outside of P elements", // TODO: are there any other cases where we need to still wrap whitespace to match Kobo's behaviour?
			Fragment: true,
			In:       `<p>This is a test.` + "\n" + `    This is another sentence on the next line.<span> </span>Another sentence.</p>` + "\n" + "    <p>Another paragraph.</p><p> </p><p></p>",
			Out:      `<p><span class="koboSpan" id="kobo.1.1">This is a test.` + "\n" + `    </span><span class="koboSpan" id="kobo.1.2">This is another sentence on the next line.</span><span> </span><span class="koboSpan" id="kobo.1.3">Another sentence.</span></p>` + "\n" + `    <p><span class="koboSpan" id="kobo.2.1">Another paragraph.</span></p><p><span class="koboSpan" id="kobo.3.1"> </span></p><p></p>`,
		}.Run(t)

		transformContentCase{
			Func:     transformContentKoboSpans,
			What:     "preserve and split segments on formatting and links",
			Fragment: true,
			In:       `<p>Sentence<b> 1. </b>Sentence <span>2. Se</span>nten<a href="test.html">ce 3. Another word</a></p>`,
			Out:      `<p><span class="koboSpan" id="kobo.1.1">Sentence</span><b><span class="koboSpan" id="kobo.1.2"> 1. </span></b><span class="koboSpan" id="kobo.1.3">Sentence </span><span><span class="koboSpan" id="kobo.1.4">2. </span><span class="koboSpan" id="kobo.1.5">Se</span></span><span class="koboSpan" id="kobo.1.6">nten</span><a href="test.html"><span class="koboSpan" id="kobo.1.7">ce 3. </span><span class="koboSpan" id="kobo.1.8">Another word</span></a></p>`,
		}.Run(t)

		transformContentCase{
			Func:     transformContentKoboSpans,
			What:     "preserve and split segments on nested formatting and links",
			Fragment: true,
			In:       `<p>Sentence<b> 1. Sente<i>nce <span>2. Se</span>nt</i>en<a href="test.html">ce 3. Another word</a></b></p>`,
			Out:      `<p><span class="koboSpan" id="kobo.1.1">Sentence</span><b><span class="koboSpan" id="kobo.1.2"> 1. </span><span class="koboSpan" id="kobo.1.3">Sente</span><i><span class="koboSpan" id="kobo.1.4">nce </span><span><span class="koboSpan" id="kobo.1.5">2. </span><span class="koboSpan" id="kobo.1.6">Se</span></span><span class="koboSpan" id="kobo.1.7">nt</span></i><span class="koboSpan" id="kobo.1.8">en</span><a href="test.html"><span class="koboSpan" id="kobo.1.9">ce 3. </span><span class="koboSpan" id="kobo.1.10">Another word</span></a></b></p>`,
		}.Run(t)

		transformContentCase{
			Func:     transformContentKoboSpans,
			What:     "nested lists/paragraphs should not reset numbering once out of scope (i.e. <p [para1]>[span1.1]<ul [para2]><li>[span2.1]</li></ul>[span2.2]</p>)", // TODO: verify against an actual kepub (I'll have to find a free non-fiction one or one with a nested TOC)
			Fragment: true,
			In:       `<p>Sentence.<ul><li>Another sentence.</li><li>Another sentence.<ul><li>Another sentence.</li><li>Another sentence.</li></ul></li><li>Another sentence.</li></ul> Another sentence.</p>`,
			Out:      `<p><span class="koboSpan" id="kobo.1.1">Sentence.</span></p><ul><li><span class="koboSpan" id="kobo.2.1">Another sentence.</span></li><li><span class="koboSpan" id="kobo.2.2">Another sentence.</span><ul><li><span class="koboSpan" id="kobo.3.1">Another sentence.</span></li><li><span class="koboSpan" id="kobo.3.2">Another sentence.</span></li></ul></li><li><span class="koboSpan" id="kobo.3.3">Another sentence.</span></li></ul><span class="koboSpan" id="kobo.3.4"> Another sentence.</span><p></p>`,
		}.Run(t)

		transformContentCase{
			Func:     transformContentKoboSpans,
			What:     "don't touch the contents of script, style, pre, audio, video tags",
			Fragment: true,
			In:       `<p>Touch this.</p><script>not this</script><style>or this</style><pre>or this</pre><audio>or this</audio><video>or this</video><p>Touch this.</p>`,
			Out:      `<p><span class="koboSpan" id="kobo.1.1">Touch this.</span></p><script>not this</script><style>or this</style><pre>or this</pre><audio>or this</audio><video>or this</video><p><span class="koboSpan" id="kobo.2.1">Touch this.</span></p>`,
		}.Run(t)

		transformContentCase{
			Func:     transformContentKoboSpans,
			What:     "treat an img as a new paragraph and add a span around it",
			Fragment: true,
			In:       `<p>One.</p><img src="test"><p>Three.</p>`,
			Out:      `<p><span class="koboSpan" id="kobo.1.1">One.</span></p><span class="koboSpan" id="kobo.2.1"><img src="test"/></span><p><span class="koboSpan" id="kobo.3.1">Three.</span></p>`,
		}.Run(t)

		transformContentCase{
			Func:     transformContentKoboSpans,
			What:     "don't increment paragraph counter if no spans were added",
			Fragment: true,
			In:       `<p>One.</p><p> </p><p><!-- comment --></p><p>Two.</p><p><b>Three.</b></p>`,
			Out:      `<p><span class="koboSpan" id="kobo.1.1">One.</span></p><p><span class="koboSpan" id="kobo.2.1"> </span></p><p><!-- comment --></p><p><span class="koboSpan" id="kobo.3.1">Two.</span></p><p><b><span class="koboSpan" id="kobo.4.1">Three.</span></b></p>`,
		}.Run(t)

		transformContentCase{
			Func:     transformContentKoboSpans,
			What:     "don't add spans to svg and math elements",
			Fragment: true,
			In:       `<svg xmlns="http://www.w3.org/2000/svg"><g><text font-size="24" y="20" x="0">kepubify</text></g></svg><math xmlns="http://www.w3.org/1998/Math/MathML"><mi>x</mi><mo>=</mo><mfrac><mrow><mo>-</mo><mi>b</mi><mo>±</mo><msqrt><msup><mi>b</mi><mn>2</mn></msup><mo>-</mo><mn>4</mn><mi>a</mi><mi>c</mi></msqrt></mrow><mrow><mn>2</mn><mi>a</mi></mrow></mfrac></math>`,
			Out:      `<svg xmlns="http://www.w3.org/2000/svg"><g><text font-size="24" y="20" x="0">kepubify</text></g></svg><math xmlns="http://www.w3.org/1998/Math/MathML"><mi>x</mi><mo>=</mo><mfrac><mrow><mo>-</mo><mi>b</mi><mo>±</mo><msqrt><msup><mi>b</mi><mn>2</mn></msup><mo>-</mo><mn>4</mn><mi>a</mi><mi>c</mi></msqrt></mrow><mrow><mn>2</mn><mi>a</mi></mrow></mfrac></math>`,
		}.Run(t)

		// The following cases were found after using kobotest on a bunch of files (the previous cases are also based on kepubs, but I did them manually and didn't keep track):

		transformContentCase{
			Func:     transformContentKoboSpans,
			What:     "also increment paragraph counter on heading tags and don't split sentences after colons or if there isn't any spaces after a period", // 69b8ba8c-1799-4e0d-ba3a-ce366410335e (Janurary 2020): Arthur Conan Doyle - The Complete Sherlock Holmes.kepub/OEBPS/bookwire/bookwire_advertisement1.xhtml
			Fragment: true,
			In:       `<div class="img_container"><p class="ad_image"><img src="bookwire_ad_cover1.jpg" alt="image"/></p></div>` + "\n" + `    <h2 class="subheadline">The Christmas Collection: All Of Your Favourite Classic Christmas Stories, Novels, Poems, Carols in One Ebook</h2>` + "\n" + `    <p class="subheadline2"></p>` + "\n" + `    <p class="metadata">Carr, Annie Roe</p>`,
			Out:      `<div class="img_container"><p class="ad_image"><span class="koboSpan" id="kobo.1.1"><img src="bookwire_ad_cover1.jpg" alt="image"/></span></p></div>` + "\n" + `    <h2 class="subheadline"><span class="koboSpan" id="kobo.2.1">The Christmas Collection: All Of Your Favourite Classic Christmas Stories, Novels, Poems, Carols in One Ebook</span></h2>` + "\n" + `    <p class="subheadline2"></p>` + "\n" + `    <p class="metadata"><span class="koboSpan" id="kobo.3.1">Carr, Annie Roe</span></p>`,
		}.Run(t)
	})

	t.Run("AddStyle", func(t *testing.T) {
		transformContentCase{
			Func:     func(doc *html.Node) { transformContentAddStyle(doc, "kepubify-test", "div > div { color: black; }") },
			What:     "add style to head",
			Fragment: false,
			In:       `<!DOCTYPE html><html><head><title>Kepubify Test</title></head><body></body></html>`,
			Out:      `<!DOCTYPE html><html><head><title>Kepubify Test</title><style type="text/css" class="kepubify-test">div > div { color: black; }</style></head><body></body></html>`,
		}.Run(t)
	})

	t.Run("SmartyPants", func(t *testing.T) {
		transformContentCase{
			Func:     transformContentPunctuation,
			What:     "smart punctuation",
			Fragment: true,
			In:       `<p>This is a test sentence to test smartypants' conversion of "quotation marks", dashes like - / -- / ---, and symbols like (c).</p>`,
			Out:      `<p>This is a test sentence to test smartypants’ conversion of “quotation marks”, dashes like - / – / —, and symbols like ©.</p>`,
		}.Run(t)

		transformContentCase{
			Func:     transformContentPunctuation,
			What:     "skip pre, code, style, and script elements",
			Fragment: true,
			In:       `<p>This is a test sentence to test smartypants' conversion of <code>"quotation marks"</code>, dashes like <pre>- / -- / ---</pre>, and symbols like (c).</p><style>div{font-family:"Test"}</style><script>var a="test"</script>`,
			Out:      `<p>This is a test sentence to test smartypants’ conversion of <code>&#34;quotation marks&#34;</code>, dashes like </p><pre>- / -- / ---</pre>, and symbols like ©.<p></p><style>div{font-family:"Test"}</style><script>var a="test"</script>`,
		}.Run(t)

		transformContentCase{
			Func:     transformContentPunctuation,
			What:     "properly handle entity escaping",
			Fragment: true,
			In:       `<p>&amp;&quot;&lt;&gt;&quot;</p><pre>&quot;</pre>`,
			Out:      `<p>&amp;“&lt;&gt;”</p><pre>&#34;</pre>`,
		}.Run(t)
	})

	t.Run("CleanHTML", func(t *testing.T) {
		transformContentCase{
			Func:     transformContentClean,
			What:     "remove adobe adept metadata",
			Fragment: true,
			In:       `<meta charset="utf-8"/><meta name="Adept.expected.resource"/>`,
			Out:      `<meta charset="utf-8"/>`,
		}.Run(t)

		transformContentCase{
			Func:     transformContentClean,
			What:     "remove useless MS Word tags",
			Fragment: true,
			In:       `<o:p></o:p><st1:test></st1:test><div><o:p> dfg </o:p><o:p></o:p></div>`,
			Out:      `<div><o:p> dfg </o:p></div>`,
		}.Run(t)

		transformContentCase{
			Func:     transformContentClean,
			What:     "remove unicode replacement chars",
			Fragment: true,
			In:       `�<p>test�ing</p><p>asd<b>fgh</b></p>`,
			Out:      `<p>testing</p><p>asd<b>fgh</b></p>`,
		}.Run(t)
	})

	t.Run("Replacements", func(t *testing.T) {
		const corpus = `<!DOCTYPE html><html><head><title></title></head><body><b>Lorem ipsum</b> dolor sit amet, <a href="https://example.com">consectetur adipiscing elit</a>, sed do eiusmod tempor incididunt ut labore et dolore magna aliqua. Ut enim ad minim veniam, quis nostrud exercitation ullamco laboris nisi ut aliquip ex ea commodo consequat. Duis aute irure dolor in reprehenderit in voluptate velit esse cillum dolore eu fugiat nulla pariatur. Excepteur sint occaecat cupidatat non proident, sunt in culpa qui officia deserunt mollit anim id est laborum.</body></html>`
		for _, tc := range []struct {
			What         string
			Replacements []string
		}{
			{
				What: "simple removal",
				Replacements: []string{
					" ipsum", "",
				},
			},
			{
				What: "simple replacement",
				Replacements: []string{
					". ", "_ ",
				},
			},
			{
				What: "complex removal", // to ensure the transformer behaves correctly when it requires multiple writes from the renderer
				Replacements: []string{
					"<b>Lorem ipsum</b>", "",
				},
			},
			{
				What: "long replacement",
				Replacements: []string{
					"ipsum", strings.Repeat(".", 4096),
				},
			},
			{
				What: "simple chained",
				Replacements: []string{
					"ipsum", "test1",
					"amet", "test2",
					"commodo", "",
				},
			},
			{
				What: "ordered chained",
				Replacements: []string{
					"ipsum", "Lorem",
					"Lorem", "test1",
					"ipsum", "",
				},
			},
			{
				What: "overlapping chained",
				Replacements: []string{
					"Lorem", "test1",
					"test1", "test2",
					"ipsum", "test2 ipsum",
					"test2", "test3",
				},
			},
			{
				What: "complex chained", // to ensure order matters
				Replacements: []string{
					"Lorem", "ipsum",
					"or", "ar",
					"dolar", "dolor",
					"</", "__________",
					"________", "__</",
					"_", " ",
					". ", "; ",
				},
			},
		} {
			out := corpus
			for i := 0; i < len(tc.Replacements)/2; i++ {
				out = strings.ReplaceAll(out, tc.Replacements[i*2], tc.Replacements[i*2+1])
			}
			if out == corpus {
				panic("strings don't differ")
			}
			transformContentCase{
				Func: func(repl ...string) func(io.Writer) io.WriteCloser {
					if len(repl)%2 != 0 {
						panic("replacements not a multiple of 2")
					}
					f := make([][]byte, len(repl)/2)
					r := make([][]byte, len(repl)/2)
					for i := 0; i < len(repl)/2; i++ {
						f[i], r[i] = []byte(repl[i*2]), []byte(repl[i*2+1])
					}
					return func(w io.Writer) io.WriteCloser {
						return transformContentReplacements(w, f, r)
					}
				}(tc.Replacements...),
				What: tc.What,
				In:   corpus,
				Out:  out,
			}.Run(t)
		}
	})
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
			if got := transformOPF(t, tc.in); got != want {
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

func transformOPF(t *testing.T, in string) string {
	t.Helper()
	var a, b bytes.Buffer
	if err := (&Converter{}).TransformOPF(&a, strings.NewReader(in)); err != nil {
		t.Fatalf("transform: %v", err)
	}
	// one-byte reader to ensure streaming works correctly
	if err := (&Converter{}).TransformOPF(&b, iotest.OneByteReader(strings.NewReader(in))); err != nil {
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
		if got := (&Converter{}).TransformFileFilter(tc.fn); got != tc.want {
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
			name:    "UTF-16 content document",
			opf:     `<package xmlns="http://www.idpf.org/2007/opf"><manifest><item id="text" href="text.xhtml"/></manifest><spine><itemref idref="text"/></spine></package>`,
			content: map[string]string{"text.xhtml": "\xff\xfe<\x00h\x00t\x00m\x00l\x00>\x00"},
		},
	} {
		tc.run(t)
	}
}

type transformContentCase struct {
	Func     interface{}
	What     string
	Fragment bool
	Contains bool

	In  string
	Out string
}

func (tc transformContentCase) Run(t *testing.T) {
	// note: the HTML is parsed and rendered without my extensions to be to test
	//       just the transformation function without depending on the behaviour
	//       of the extensions.
	t.Logf("case %#v", tc.What)

	var inReader io.Reader
	if tc.Fragment {
		inReader = strings.NewReader(`<!DOCTYPE html><head><title>Kepubify Test Case</title><meta charset="utf-8"/></head><body>` + tc.In + `</body></html>`)
	} else {
		inReader = strings.NewReader(tc.In)
	}

	doc, err := html.Parse(inReader)
	if err != nil {
		t.Errorf("case %#v: parse: %v", tc.What, err)
		return
	}

	var w io.Writer
	buf := bytes.NewBuffer(nil)
	w = buf

	switch fn := tc.Func.(type) {
	case func(doc *html.Node):
		fn(doc)
	case func(w io.Writer) io.WriteCloser:
		w = fn(w)
	default:
		panic(fmt.Sprintf("unknown type for func: %T", fn))
	}

	if err := html.Render(w, doc); err != nil {
		t.Errorf("case %#v: render: %v", tc.What, err)
		return
	}

	if wc, ok := w.(io.WriteCloser); ok {
		if err := wc.Close(); err != nil {
			t.Errorf("case %#v: render: %v", tc.What, err)
		}
	}

	hstr := buf.String()
	if tc.Fragment {
		hstr = strings.Split(strings.Split(hstr, "<body>")[1], "</body>")[0] // hacky, but the easiest way to do it
	}

	if tc.Contains {
		if !strings.Contains(hstr, tc.Out) {
			t.Errorf("case %#v: \ngot:`%v` doesn't contain \nexp:`%v`", tc.What, strings.ReplaceAll(hstr, "\n", "`+\"\\n\"+`"), strings.ReplaceAll(tc.Out, "\n", "`+\"\\n\"+`"))
		}
	} else {
		if hstr != tc.Out {
			t.Errorf("case %#v: \ngot:`%v` != \nexp:`%v`", tc.What, strings.ReplaceAll(hstr, "\n", "`+\"\\n\"+`"), strings.ReplaceAll(tc.Out, "\n", "`+\"\\n\"+`"))
		}
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
		c := NewConverter()
		if tc.force != nil {
			c = NewConverterWithOptions(ConverterOptionDummyTitlepage(*tc.force))
		}
		buf := bytes.NewBufferString(opf)
		name, r, added, err := c.TransformDummyTitlepage(epub, titlepageOPF, buf)
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
		if b, err := io.ReadAll(r); err != nil || string(b) != dummyTitlepage {
			t.Errorf("content document %q, %v", b, err)
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

func TestSplitSentences(t *testing.T) {
	for _, v := range testSentences {
		sss := slices.Collect(splitSentencesSeq(v))
		ssr := splitSentencesRegexp(v)

		if len(sss) == len(ssr) {
			for i := range sss {
				if sss[i] != ssr[i] {
					t.Errorf("%q (new state-machine) != %q (old regexp)", sss, ssr)
				}
			}
		} else {
			t.Errorf("%q (new state-machine) != %q (old regexp)", sss, ssr)
		}

		if j := strings.Join(sss, ""); j != v {
			t.Errorf("%q (joined sentence) != %q (original sentence)", j, v)
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
				for range splitSentencesSeq(v) {
					// nop
				}
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
