package kepub

import (
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/beevik/etree"
	"golang.org/x/net/html"
)

func TestTransformContent(t *testing.T) {
	c := &Converter{
		extraCSS:      []string{"body { color: black; }"},
		extraCSSClass: []string{"kepubify-test"},
		smartypants:   true,
	}

	// yes, I know it isn't valid XML, but I'm testing preserving the XML declaration
	doc, err := c.transform1(strings.NewReader(strings.TrimSpace(`
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
</html>`)))
	if err != nil {
		t.Fatalf("transform1: unexpected error: %v", err)
	}

	if err := c.transform2(doc); err != nil {
		t.Fatalf("transform2: unexpected error: %v", err)
	}

	buf := bytes.NewBuffer(nil)
	if err := c.transform3(buf, doc); err != nil {
		t.Fatalf("transform3: unexpected error: %v", err)
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
    <p><span class="koboSpan" id="kobo.1.1">Test sentence 1. </span><a id="test"></a><span class="koboSpan" id="kobo.1.2"> Test sentence 2. </span><b><span class="koboSpan" id="kobo.1.3">Test sentence 3</span><i><span class="koboSpan" id="kobo.1.4">Test sentence 4</span></i></b></p><b><i>
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
	t.Run("KoboStyles", func(t *testing.T) {
		transformContentCase{
			Func:     transform2koboStyles,
			What:     "add kobo style hacks",
			Fragment: false,
			Contains: true,
			In:       `<!DOCTYPE html><head><title>Kepubify Test Case</title><meta charset="utf-8"/></head><body></body></html>`,
			Out:      `kobostylehacks`,
		}.Run(t)
	})

	t.Run("KoboDivs", func(t *testing.T) {
		transformContentCase{
			Func:     transform2koboDivs,
			What:     "no content",
			Fragment: true,
			In:       ``,
			Out:      `<div id="book-columns"><div id="book-inner"></div></div>`,
		}.Run(t)

		transformContentCase{
			Func:     transform2koboDivs,
			What:     "already has divs",
			Fragment: true,
			In:       `<div id="book-columns"><div id="book-inner"></div></div>`,
			Out:      `<div id="book-columns"><div id="book-inner"></div></div>`,
		}.Run(t)

		transformContentCase{
			Func:     transform2koboDivs,
			What:     "single text node",
			Fragment: true,
			In:       `test`,
			Out:      `<div id="book-columns"><div id="book-inner">test</div></div>`,
		}.Run(t)

		transformContentCase{
			Func:     transform2koboDivs,
			What:     "multiple elements and children",
			Fragment: true,
			In:       `<p>Test 1</p><p>Test <b>2</b></p><p>Test 3</p>`,
			Out:      `<div id="book-columns"><div id="book-inner"><p>Test 1</p><p>Test <b>2</b></p><p>Test 3</p></div></div>`,
		}.Run(t)
	})

	t.Run("KoboSpans", func(t *testing.T) {
		transformContentCase{
			Func:     transform2koboSpans,
			What:     "no content",
			Fragment: true,
			In:       ``,
			Out:      ``,
		}.Run(t)

		transformContentCase{
			Func:     transform2koboSpans,
			What:     "already has spans",
			Fragment: true,
			In:       `<p><span class="koboSpan" id="kobo.1.1">Test</span></p>`,
			Out:      `<p><span class="koboSpan" id="kobo.1.1">Test</span></p>`,
		}.Run(t)

		transformContentCase{
			Func:     transform2koboSpans,
			What:     "increment segment counter from 1 for every sentence",
			Fragment: true,
			In:       `<p>Sentence 1. Sentence 2. Sentence 3.</p>`,
			Out:      `<p><span class="koboSpan" id="kobo.1.1">Sentence 1. </span><span class="koboSpan" id="kobo.1.2">Sentence 2. </span><span class="koboSpan" id="kobo.1.3">Sentence 3.</span></p>`,
		}.Run(t)

		transformContentCase{
			Func:     transform2koboSpans,
			What:     "increment paragraph counter from 1 and reset segment counter for every p, ul, ol, or table",
			Fragment: true,
			In:       `<p>Sentence 1. Sentence 2.</p><p>Sentence 3.</p><ul><li>Sentence 4</li><li>Sentence 5</li></ul><ol><li>Sentence 6</li><li>Sentence 7</li></ol><table><tbody><tr><td>Test</td></tr><tr><td>Test</td></tr></tbody></table>`,
			Out:      `<p><span class="koboSpan" id="kobo.1.1">Sentence 1. </span><span class="koboSpan" id="kobo.1.2">Sentence 2.</span></p><p><span class="koboSpan" id="kobo.2.1">Sentence 3.</span></p><ul><li><span class="koboSpan" id="kobo.3.1">Sentence 4</span></li><li><span class="koboSpan" id="kobo.3.2">Sentence 5</span></li></ul><ol><li><span class="koboSpan" id="kobo.4.1">Sentence 6</span></li><li><span class="koboSpan" id="kobo.4.2">Sentence 7</span></li></ol><table><tbody><tr><td><span class="koboSpan" id="kobo.5.1">Test</span></td></tr><tr><td><span class="koboSpan" id="kobo.5.2">Test</span></td></tr></tbody></table>`,
		}.Run(t)

		transformContentCase{
			Func:     transform2koboSpans,
			What:     "merge stray text at the end of lines into the next sentence (between regexp matches)",
			Fragment: true,
			In:       `<p>Sentence 1. Sentence 2. Stray text` + "\n" + `Another sentence.</p>`,
			Out:      `<p><span class="koboSpan" id="kobo.1.1">Sentence 1. </span><span class="koboSpan" id="kobo.1.2">Sentence 2. </span><span class="koboSpan" id="kobo.1.3">Stray text` + "\n" + `Another sentence.</span></p>`,
		}.Run(t)

		transformContentCase{
			Func:     transform2koboSpans,
			What:     "don't lose stray text not part of a sentence (after the last regexp match)",
			Fragment: true,
			In:       `<p>Sentence 1. Sentence 2. Stray text</p>`,
			Out:      `<p><span class="koboSpan" id="kobo.1.1">Sentence 1. </span><span class="koboSpan" id="kobo.1.2">Sentence 2. </span><span class="koboSpan" id="kobo.1.3">Stray text</span></p>`,
		}.Run(t)

		transformContentCase{
			Func:     transform2koboSpans,
			What:     "preserve but don't wrap extra whitespace outside of P elements", // TODO: are there any other cases where we need to still wrap whitespace to match Kobo's behaviour?
			Fragment: true,
			In:       `<p>This is a test.` + "\n" + `    This is another sentence on the next line.<span> </span>Another sentence.</p>` + "\n" + "    <p>Another paragraph.</p><p> </p><p></p>",
			Out:      `<p><span class="koboSpan" id="kobo.1.1">This is a test.` + "\n" + `    </span><span class="koboSpan" id="kobo.1.2">This is another sentence on the next line.</span><span> </span><span class="koboSpan" id="kobo.1.3">Another sentence.</span></p>` + "\n" + `    <p><span class="koboSpan" id="kobo.2.1">Another paragraph.</span></p><p><span class="koboSpan" id="kobo.3.1"> </span></p><p></p>`,
		}.Run(t)

		transformContentCase{
			Func:     transform2koboSpans,
			What:     "preserve and split segments on formatting and links",
			Fragment: true,
			In:       `<p>Sentence<b> 1. </b>Sentence <span>2. Se</span>nten<a href="test.html">ce 3. Another word</a></p>`,
			Out:      `<p><span class="koboSpan" id="kobo.1.1">Sentence</span><b><span class="koboSpan" id="kobo.1.2"> 1. </span></b><span class="koboSpan" id="kobo.1.3">Sentence </span><span><span class="koboSpan" id="kobo.1.4">2. </span><span class="koboSpan" id="kobo.1.5">Se</span></span><span class="koboSpan" id="kobo.1.6">nten</span><a href="test.html"><span class="koboSpan" id="kobo.1.7">ce 3. </span><span class="koboSpan" id="kobo.1.8">Another word</span></a></p>`,
		}.Run(t)

		transformContentCase{
			Func:     transform2koboSpans,
			What:     "preserve and split segments on nested formatting and links",
			Fragment: true,
			In:       `<p>Sentence<b> 1. Sente<i>nce <span>2. Se</span>nt</i>en<a href="test.html">ce 3. Another word</a></b></p>`,
			Out:      `<p><span class="koboSpan" id="kobo.1.1">Sentence</span><b><span class="koboSpan" id="kobo.1.2"> 1. </span><span class="koboSpan" id="kobo.1.3">Sente</span><i><span class="koboSpan" id="kobo.1.4">nce </span><span><span class="koboSpan" id="kobo.1.5">2. </span><span class="koboSpan" id="kobo.1.6">Se</span></span><span class="koboSpan" id="kobo.1.7">nt</span></i><span class="koboSpan" id="kobo.1.8">en</span><a href="test.html"><span class="koboSpan" id="kobo.1.9">ce 3. </span><span class="koboSpan" id="kobo.1.10">Another word</span></a></b></p>`,
		}.Run(t)

		transformContentCase{
			Func:     transform2koboSpans,
			What:     "nested lists/paragraphs should not reset numbering once out of scope (i.e. <p [para1]>[span1.1]<ul [para2]><li>[span2.1]</li></ul>[span2.2]</p>)", // TODO: verify against an actual kepub (I'll have to find a free non-fiction one or one with a nested TOC)
			Fragment: true,
			In:       `<p>Sentence.<ul><li>Another sentence.</li><li>Another sentence.<ul><li>Another sentence.</li><li>Another sentence.</li></ul></li><li>Another sentence.</li></ul> Another sentence.</p>`,
			Out:      `<p><span class="koboSpan" id="kobo.1.1">Sentence.</span></p><ul><li><span class="koboSpan" id="kobo.2.1">Another sentence.</span></li><li><span class="koboSpan" id="kobo.2.2">Another sentence.</span><ul><li><span class="koboSpan" id="kobo.3.1">Another sentence.</span></li><li><span class="koboSpan" id="kobo.3.2">Another sentence.</span></li></ul></li><li><span class="koboSpan" id="kobo.3.3">Another sentence.</span></li></ul><span class="koboSpan" id="kobo.3.4"> Another sentence.</span><p></p>`,
		}.Run(t)

		transformContentCase{
			Func:     transform2koboSpans,
			What:     "don't touch the contents of script, style, pre, audio, video tags",
			Fragment: true,
			In:       `<p>Touch this.</p><script>not this</script><style>or this</style><pre>or this</pre><audio>or this</audio><video>or this</video><p>Touch this.</p>`,
			Out:      `<p><span class="koboSpan" id="kobo.1.1">Touch this.</span></p><script>not this</script><style>or this</style><pre>or this</pre><audio>or this</audio><video>or this</video><p><span class="koboSpan" id="kobo.2.1">Touch this.</span></p>`,
		}.Run(t)

		transformContentCase{
			Func:     transform2koboSpans,
			What:     "treat an img as a new paragraph and add a span around it",
			Fragment: true,
			In:       `<p>One.</p><img src="test"><p>Three.</p>`,
			Out:      `<p><span class="koboSpan" id="kobo.1.1">One.</span></p><span class="koboSpan" id="kobo.2.1"><img src="test"/></span><p><span class="koboSpan" id="kobo.3.1">Three.</span></p>`,
		}.Run(t)

		transformContentCase{
			Func:     transform2koboSpans,
			What:     "don't increment paragraph counter if no spans were added",
			Fragment: true,
			In:       `<p>One.</p><p> </p><p><!-- comment --></p><p>Two.</p><p><b>Three.</b></p>`,
			Out:      `<p><span class="koboSpan" id="kobo.1.1">One.</span></p><p><span class="koboSpan" id="kobo.2.1"> </span></p><p><!-- comment --></p><p><span class="koboSpan" id="kobo.3.1">Two.</span></p><p><b><span class="koboSpan" id="kobo.4.1">Three.</span></b></p>`,
		}.Run(t)

		transformContentCase{
			Func:     transform2koboSpans,
			What:     "don't add spans to svg and math elements",
			Fragment: true,
			In:       `<svg xmlns="http://www.w3.org/2000/svg"><g><text font-size="24" y="20" x="0">kepubify</text></g></svg><math xmlns="http://www.w3.org/1998/Math/MathML"><mi>x</mi><mo>=</mo><mfrac><mrow><mo>-</mo><mi>b</mi><mo>±</mo><msqrt><msup><mi>b</mi><mn>2</mn></msup><mo>-</mo><mn>4</mn><mi>a</mi><mi>c</mi></msqrt></mrow><mrow><mn>2</mn><mi>a</mi></mrow></mfrac></math>`,
			Out:      `<svg xmlns="http://www.w3.org/2000/svg"><g><text font-size="24" y="20" x="0">kepubify</text></g></svg><math xmlns="http://www.w3.org/1998/Math/MathML"><mi>x</mi><mo>=</mo><mfrac><mrow><mo>-</mo><mi>b</mi><mo>±</mo><msqrt><msup><mi>b</mi><mn>2</mn></msup><mo>-</mo><mn>4</mn><mi>a</mi><mi>c</mi></msqrt></mrow><mrow><mn>2</mn><mi>a</mi></mrow></mfrac></math>`,
		}.Run(t)

		// The following cases were found after using kobotest on a bunch of files (the previous cases are also based on kepubs, but I did them manually and didn't keep track):

		transformContentCase{
			Func:     transform2koboSpans,
			What:     "also increment paragraph counter on heading tags and don't split sentences after colons or if there isn't any spaces after a period", // 69b8ba8c-1799-4e0d-ba3a-ce366410335e (Janurary 2020): Arthur Conan Doyle - The Complete Sherlock Holmes.kepub/OEBPS/bookwire/bookwire_advertisement1.xhtml
			Fragment: true,
			In:       `<div class="img_container"><p class="ad_image"><img src="bookwire_ad_cover1.jpg" alt="image"/></p></div>` + "\n" + `    <h2 class="subheadline">The Christmas Collection: All Of Your Favourite Classic Christmas Stories, Novels, Poems, Carols in One Ebook</h2>` + "\n" + `    <p class="subheadline2"></p>` + "\n" + `    <p class="metadata">Carr, Annie Roe</p>`,
			Out:      `<div class="img_container"><p class="ad_image"><span class="koboSpan" id="kobo.1.1"><img src="bookwire_ad_cover1.jpg" alt="image"/></span></p></div>` + "\n" + `    <h2 class="subheadline"><span class="koboSpan" id="kobo.2.1">The Christmas Collection: All Of Your Favourite Classic Christmas Stories, Novels, Poems, Carols in One Ebook</span></h2>` + "\n" + `    <p class="subheadline2"></p>` + "\n" + `    <p class="metadata"><span class="koboSpan" id="kobo.3.1">Carr, Annie Roe</span></p>`,
		}.Run(t)
	})

	t.Run("AddStyle", func(t *testing.T) {
		transformContentCase{
			Func:     func(doc *html.Node) { transform2addStyle(doc, "kepubify-test", "div > div { color: black; }") },
			What:     "add style to head",
			Fragment: false,
			In:       `<!DOCTYPE html><html><head><title>Kepubify Test</title></head><body></body></html>`,
			Out:      `<!DOCTYPE html><html><head><title>Kepubify Test</title><style type="text/css" class="kepubify-test">div > div { color: black; }</style></head><body></body></html>`,
		}.Run(t)
	})

	t.Run("SmartyPants", func(t *testing.T) {
		transformContentCase{
			Func:     transform2smartypants,
			What:     "smart punctuation",
			Fragment: true,
			In:       `<p>This is a test sentence to test smartypants' conversion of "quotation marks", dashes like - / -- / ---, and symbols like (c).</p>`,
			Out:      `<p>This is a test sentence to test smartypants’ conversion of “quotation marks”, dashes like - / – / —, and symbols like ©.</p>`,
		}.Run(t)

		transformContentCase{
			Func:     transform2smartypants,
			What:     "skip pre, code, style, and script elements",
			Fragment: true,
			In:       `<p>This is a test sentence to test smartypants' conversion of <code>"quotation marks"</code>, dashes like <pre>- / -- / ---</pre>, and symbols like (c).</p><style>div{font-family:"Test"}</style><script>var a="test"</script>`,
			Out:      `<p>This is a test sentence to test smartypants’ conversion of <code>&#34;quotation marks&#34;</code>, dashes like </p><pre>- / -- / ---</pre>, and symbols like ©.<p></p><style>div{font-family:"Test"}</style><script>var a="test"</script>`,
		}.Run(t)

		transformContentCase{
			Func:     transform2smartypants,
			What:     "properly handle entity escaping",
			Fragment: true,
			In:       `<p>&amp;&quot;&lt;&gt;&quot;</p><pre>&quot;</pre>`,
			Out:      `<p>&amp;“&lt;&gt;”</p><pre>&#34;</pre>`,
		}.Run(t)
	})

	t.Run("CleanHTML", func(t *testing.T) {
		transformContentCase{
			Func:     transform2cleanHTML,
			What:     "remove adobe adept metadata",
			Fragment: true,
			In:       `<meta charset="utf-8"/><meta name="Adept.expected.resource"/>`,
			Out:      `<meta charset="utf-8"/>`,
		}.Run(t)

		transformContentCase{
			Func:     transform2cleanHTML,
			What:     "remove useless MS Word tags",
			Fragment: true,
			In:       `<o:p></o:p><st1:test></st1:test><div><o:p> dfg </o:p><o:p></o:p></div>`,
			Out:      `<div><o:p> dfg </o:p></div>`,
		}.Run(t)

		transformContentCase{
			Func:     transform2cleanHTML,
			What:     "remove unicode replacement chars",
			Fragment: true,
			In:       `�<p>test�ing</p><p>asd<b>fgh</b></p>`,
			Out:      `<p>testing</p><p>asd<b>fgh</b></p>`,
		}.Run(t)
	})
}

func TestTransformOPF(t *testing.T) {
	// note: the individual parts below aren't tested again here
	c := &Converter{}

	doc, err := c.transformOPF1(strings.NewReader(strings.TrimSpace(`
<?xml version="1.0" encoding="UTF-8"?>
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
</package>`)))
	if err != nil {
		t.Fatalf("transform1: unexpected error: %v", err)
	}

	if err := c.transformOPF2(doc); err != nil {
		t.Fatalf("transform2: unexpected error: %v", err)
	}

	buf := bytes.NewBuffer(nil)
	if err := c.transformOPF3(buf, doc); err != nil {
		t.Fatalf("transform3: unexpected error: %v", err)
	}

	if a, b := strings.TrimSpace(buf.String()), strings.TrimSpace(`
<?xml version="1.0" encoding="UTF-8"?>
<package xmlns="http://www.idpf.org/2007/opf" version="3.0" unique-identifier="uuid_id">
    <metadata xmlns:dc="http://purl.org/dc/elements/1.1/">
        <!-- other stuff left out for brevity -->
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
</package>`); a != b {
		t.Error("not equal")
		fmt.Println(a)
		fmt.Println("---")
		fmt.Println(b)
	}
}

func TestTransformOPFParts(t *testing.T) {
	t.Run("CoverImage", func(t *testing.T) {
		transformXMLTestCase{
			Func: transformOPF2coverImage,
			What: "set cover-image property on ID from meta[name=cover]",
			In: `<?xml version="1.0" encoding="UTF-8"?>
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
			Out: `<?xml version="1.0" encoding="UTF-8"?>
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
		}.Run(t)

		transformXMLTestCase{
			Func: transformOPF2coverImage,
			What: "set cover-image property on #cover if meta[name=cover] not present",
			In: `<?xml version="1.0" encoding="UTF-8"?>
<package xmlns="http://www.idpf.org/2007/opf" version="3.0" unique-identifier="uuid_id">
    <metadata xmlns:dc="http://purl.org/dc/elements/1.1/">
        <!-- other stuff left out for brevity -->
    </metadata>
    <manifest>
        <item id="cover" href="book_cover.jpg" media-type="image/jpeg"/>
        <item id="xhtml_text1" href="xhtml/text1.xhtml" media-type="application/xhtml+xml"/>
        <item id="ncx" href="toc.ncx" media-type="application/x-dtbncx+xml"/>
        <item id="nav" href="nav.xhtml" media-type="application/xhtml+xml" properties="nav"/>
    </manifest>
    <spine toc="ncx">
        <itemref idref="xhtml_text1"/>
    </spine>
</package>`,
			Out: `<?xml version="1.0" encoding="UTF-8"?>
<package xmlns="http://www.idpf.org/2007/opf" version="3.0" unique-identifier="uuid_id">
    <metadata xmlns:dc="http://purl.org/dc/elements/1.1/">
        <!-- other stuff left out for brevity -->
    </metadata>
    <manifest>
        <item id="cover" href="book_cover.jpg" media-type="image/jpeg" properties="cover-image"/>
        <item id="xhtml_text1" href="xhtml/text1.xhtml" media-type="application/xhtml+xml"/>
        <item id="ncx" href="toc.ncx" media-type="application/x-dtbncx+xml"/>
        <item id="nav" href="nav.xhtml" media-type="application/xhtml+xml" properties="nav"/>
    </manifest>
    <spine toc="ncx">
        <itemref idref="xhtml_text1"/>
    </spine>
</package>`,
		}.Run(t)
	})

	t.Run("CalibreMeta", func(t *testing.T) {
		// note: the <!-- --> is to preven editors from trimming the whitespace
		transformXMLTestCase{
			Func: transformOPF2calibreMeta,
			What: "remove calibre:timestamp and calibre contributor",
			In: `<?xml version="1.0" encoding="UTF-8"?>
<package xmlns="http://www.idpf.org/2007/opf" version="3.0" unique-identifier="uuid_id">
    <metadata xmlns:dc="http://purl.org/dc/elements/1.1/">
        <!-- --><meta content="whatever" name="calibre:timestamp"/>
        <!-- --><dc:contributor file-as="calibre" role="bkp">calibre (#.#.#) [https://calibre-ebook.com]</dc:contributor>
        <!-- --><dc:contributor>whatever</dc:contributor>
    </metadata>
    <!-- other stuff left out for brevity -->
</package>`,
			Out: `<?xml version="1.0" encoding="UTF-8"?>
<package xmlns="http://www.idpf.org/2007/opf" version="3.0" unique-identifier="uuid_id">
    <metadata xmlns:dc="http://purl.org/dc/elements/1.1/">
        <!-- -->
        <!-- -->
        <!-- --><dc:contributor>whatever</dc:contributor>
    </metadata>
    <!-- other stuff left out for brevity -->
</package>`,
		}.Run(t)
	})
}

func TestTransformEPUB(t *testing.T) {
	c := &Converter{}

	td, err := ioutil.TempDir("", "kepubify-test")
	if err != nil {
		panic(err)
	}
	defer os.RemoveAll(td)

	if err := os.Mkdir(filepath.Join(td, "META-INF"), 0755); err != nil {
		panic(err)
	}

	empty, err := countDir(td)
	if err != nil {
		panic(err)
	}

	for _, fn := range []string{
		"META-INF/calibre_bookmarks.txt",
		"iTunesMetadata.plist",
		"iTunesArtwork.plist",
		".DS_STORE",
		"__MACOSX/test.txt",
		"thumbs.db",
	} {
		ffn := filepath.Join(td, filepath.FromSlash(fn))
		if err := os.MkdirAll(filepath.Dir(ffn), 0755); err != nil {
			panic(err)
		}
		if err := ioutil.WriteFile(ffn, []byte("test"), 0644); err != nil {
			panic(err)
		}
	}

	t.Log("transform EPUB: including cleaning extra files")
	if err := c.transformEPUB(td); err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	new, err := countDir(td)
	if err != nil {
		panic(err)
	}

	if new != empty {
		t.Errorf("expected %d files, got %d", empty, new)
	}
}

type transformContentCase struct {
	Func     func(doc *html.Node)
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

	tc.Func(doc)

	buf := bytes.NewBuffer(nil)
	if err := html.Render(buf, doc); err != nil {
		t.Errorf("case %#v: render: %v", tc.What, err)
		return
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

type transformXMLTestCase struct {
	Func func(*etree.Document)
	What string
	In   string
	Out  string
}

func (tc transformXMLTestCase) Run(t *testing.T) {
	t.Logf("case %#v", tc.What)

	doc := etree.NewDocument()
	if _, err := doc.ReadFrom(strings.NewReader(tc.In)); err != nil {
		t.Errorf("case %#v: parse: %v", tc.What, err)
		return
	}

	tc.Func(doc)

	buf := bytes.NewBuffer(nil)
	if _, err := doc.WriteTo(buf); err != nil {
		t.Errorf("case %#v: write: %v", tc.What, err)
		return
	}

	if a, b := strings.TrimSpace(buf.String()), strings.TrimSpace(tc.Out); a != b {
		t.Error("not equal")
		fmt.Println(a)
		fmt.Println("---")
		fmt.Println(b)
	}
}

func countDir(dir string) (int, error) {
	var n int
	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		n++
		return nil
	})
	return n, err
}
