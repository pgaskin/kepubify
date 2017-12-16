package kepub

import (
	"crypto/sha256"
	"fmt"
	"strings"
	"testing"

	"golang.org/x/net/html"

	"github.com/PuerkitoBio/goquery"
	"github.com/stretchr/testify/assert"
)

func TestCleanHTML(t *testing.T) {
	h := `<html><head></head><body><p /><p>test</p><p /><p  /><p>test</p><meta  content="urn:uuid:asd--asdasd-asdasdas-dasdasd234234"   name="Adept.expected.resource"   /><st1:asd></st1:asd><o:p></o:p><h1></h1><h3></h3><h2>test</h2><p>test</p><style></style></body></html>`

	doc, err := goquery.NewDocumentFromReader(strings.NewReader(h))
	assert.Nil(t, err, "err should be nil")

	cleanHTML(doc)

	nh, err := doc.Html()
	assert.Nil(t, err, "err should be nil")

	assert.Equal(t, `<html><head></head><body><p></p><p>test</p><p></p><p></p><p>test</p><h2>test</h2><p>test</p><style type="text/css"></style></body></html>`, nh, "should be equal if cleaned correctly")
}

func TestSmartenPunctuation(t *testing.T) {
	h := `-- --- <!--test-->`
	smartenPunctuation(&h)
	assert.Equal(t, " &#x2014;   &#x2013;  <!-- test -->", h, "should be equal if smartened correctly")
}

func TestAddSpans(t *testing.T) {
	h := `<!DOCTYPE html>
<html xmlns="http://www.w3.org/1999/xhtml">
<head>
    <title>Test Book 1</title>
    <meta content="http://www.w3.org/1999/xhtml; charset=utf-8" http-equiv="Content-Type"/>
    <style type="text/css">
        @page { margin-bottom: 5.000000pt; margin-top: 5.000000pt; }
    </style>
</head>
<body id="p1">
	<p>This is the first sentence. This is the second sentence. This is the third sentence.</p>
	<p>This is the first sentence. This is the second sentence? This is the third sentence!</p>
	<p>This is the first <b>sentence</b>. This is the second sentence? This is the third sentence!</p>
	<p>This is the first <b>sentence. This is the </b>second sentence? This is the third sentence!</p>
	<p>This is <i>t<b>h</b>e</i> first <a href="test.html">sentence <b>here</b></a>. This is the second sentence? This is the third sentence!</p>
	<ul>
		<li>test</li>
		<li>test</li>
	</ul>
    </div>
</body>
</html>`

	doc, err := goquery.NewDocumentFromReader(strings.NewReader(h))
	assert.Nil(t, err, "err should be nil")

	addSpans(doc)

	nh, err := doc.Html()
	assert.Nil(t, err, "err should be nil")

	nh = html.UnescapeString(nh)

	hs := sha256.New()
	hs.Write([]byte(nh))

	hxs := fmt.Sprintf("%x", hs.Sum(nil))

	assert.Equal(t, "ae78fe3c38c263e2ad43879cb6c2eaf3c0e8dafdf1fdf877bff449f2c2c44eee", hxs, "hash of content should be equal if processed correctly")
}

func TestAddDivs(t *testing.T) {
	h := `<!DOCTYPE html>
<html xmlns="http://www.w3.org/1999/xhtml">
<head>
    <title>Test Book 1</title>
    <meta content="http://www.w3.org/1999/xhtml; charset=utf-8" http-equiv="Content-Type"/>
    <style type="text/css">
        @page { margin-bottom: 5.000000pt; margin-top: 5.000000pt; }
    </style>
</head>
<body id="p1">
	<p>This is the first sentence. This is the second sentence. This is the third sentence.</p>
	<p>This is the first sentence. This is the second sentence? This is the third sentence!</p>
	<p>This is the first <b>sentence</b>. This is the second sentence? This is the third sentence!</p>
	<p>This is the first <b>sentence. This is the </b>second sentence? This is the third sentence!</p>
	<p>This is <i>t<b>h</b>e</i> first <a href="test.html">sentence <b>here</b></a>. This is the second sentence? This is the third sentence!</p>
	<ul>
		<li>test</li>
		<li>test</li>
	</ul>
    </div>
</body>
</html>`

	doc, err := goquery.NewDocumentFromReader(strings.NewReader(h))
	assert.Nil(t, err, "err should be nil")

	addDivs(doc)

	nh, err := doc.Html()
	assert.Nil(t, err, "err should be nil")

	hs := sha256.New()
	hs.Write([]byte(nh))

	hxs := fmt.Sprintf("%x", hs.Sum(nil))

	assert.Equal(t, "d24ae5a8f438358828d50b036007fe06c9e24b55d6aa238f4628a24d77a15485", hxs, "hash of content should be equal if processed correctly")
}

func TestProcess(t *testing.T) {
	h := `<!DOCTYPE html>
<html xmlns="http://www.w3.org/1999/xhtml">
<head>
    <title>Test Book 1</title>
    <meta content="http://www.w3.org/1999/xhtml; charset=utf-8" http-equiv="Content-Type"/>
    <style type="text/css">
        @page { margin-bottom: 5.000000pt; margin-top: 5.000000pt; }
    </style>
</head>
<body id="p1">
	<p>This is the first sentence. This is the second sentence. This is the third sentence.</p>
	<p>This is the first sentence. This is the second sentence? This is the third sentence!</p>
	<p>This is the first <b>sentence</b>. This is the second sentence? This is the third sentence!</p>
	<p>This is the first <b>sentence. This is the </b>second sentence? This is the third sentence!</p>
	<p>This is <i>t<b>h</b>e</i> first <a href="test.html">sentence <b>here</b></a>. This is the second sentence? This is the third sentence!</p>
	<ul>
		<li>test</li>
		<li>test</li>
	</ul>
    </div>
</body>
</html>`

	process(&h)

	hs := sha256.New()
	hs.Write([]byte(h))

	hxs := fmt.Sprintf("%x", hs.Sum(nil))

	assert.Equal(t, "3abc0810906b322e3860b3d7fd1bafd5133a4a66ced286497eaafb40c94612fd", hxs, "hash of content should be equal if processed correctly")
}

func TestCleanOPF(t *testing.T) {
	opf := `<?xml version='1.0' encoding='utf-8'?>
<package xmlns="http://www.idpf.org/2007/opf" version="2.0" unique-identifier="uuid_id">
	<metadata xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance" xmlns:opf="http://www.idpf.org/2007/opf" xmlns:dcterms="http://purl.org/dc/terms/" xmlns:calibre="http://calibre.kovidgoyal.net/2009/metadata" xmlns:dc="http://purl.org/dc/elements/1.1/">
		<dc:publisher>Patrick G</dc:publisher>
		<dc:description>&lt;p&gt;This is a test book for &lt;i&gt;kepubify&lt;/i&gt;.&lt;/p&gt;</dc:description>
		<meta name="calibre:series" content="Test Series"/>
		<meta name="calibre:series_index" content="1"/>
		<meta name="calibre:timestamp" content="1"/>
		<dc:contributor role="bkp">calibre</dc:contributor>
		<dc:language>en</dc:language>
		<dc:creator opf:role="aut">Patrick G</dc:creator>
		<dc:title>epubtool Test Book 1</dc:title>
		<meta name="cover" content="cover"/>
		<dc:date>2017-07-26T14:00:00+00:00</dc:date>
		<dc:identifier id="uuid_id" opf:scheme="uuid">cf8fd6fa-3998-4e25-bfc0-8e9b529f8556</dc:identifier>
	</metadata>
	<manifest>
		<item href="cover.jpeg" id="cover" media-type="image/jpeg"/>
		<item href="title.html" id="p0" media-type="application/xhtml+xml"/>
		<item href="text01.html" id="p1" media-type="application/xhtml+xml"/>
		<item href="toc.ncx" media-type="application/x-dtbncx+xml" id="ncx"/>
	</manifest>
	<spine toc="ncx">
		<itemref idref="p0"/>
		<itemref idref="p1"/>
	</spine>
</package>`
	cleanOPF(&opf)

	assert.Equal(t, "<?xml version='1.0' encoding='utf-8'?>\n<package xmlns=\"http://www.idpf.org/2007/opf\" version=\"2.0\" unique-identifier=\"uuid_id\">\n    <metadata xmlns:xsi=\"http://www.w3.org/2001/XMLSchema-instance\" xmlns:opf=\"http://www.idpf.org/2007/opf\" xmlns:dcterms=\"http://purl.org/dc/terms/\" xmlns:calibre=\"http://calibre.kovidgoyal.net/2009/metadata\" xmlns:dc=\"http://purl.org/dc/elements/1.1/\">\n        <dc:publisher>Patrick G</dc:publisher>\n        <dc:description>&lt;p&gt;This is a test book for &lt;i&gt;kepubify&lt;/i&gt;.&lt;/p&gt;</dc:description>\n        <meta name=\"calibre:series\" content=\"Test Series\"/>\n        <meta name=\"calibre:series_index\" content=\"1\"/>\n        <dc:language>en</dc:language>\n        <dc:creator opf:role=\"aut\">Patrick G</dc:creator>\n        <dc:title>epubtool Test Book 1</dc:title>\n        <meta name=\"cover\" content=\"cover\"/>\n        <dc:date>2017-07-26T14:00:00+00:00</dc:date>\n        <dc:identifier id=\"uuid_id\" opf:scheme=\"uuid\">cf8fd6fa-3998-4e25-bfc0-8e9b529f8556</dc:identifier>\n    </metadata>\n    <manifest>\n        <item href=\"cover.jpeg\" id=\"cover\" media-type=\"image/jpeg\"/>\n        <item href=\"title.html\" id=\"p0\" media-type=\"application/xhtml+xml\"/>\n        <item href=\"text01.html\" id=\"p1\" media-type=\"application/xhtml+xml\"/>\n        <item href=\"toc.ncx\" media-type=\"application/x-dtbncx+xml\" id=\"ncx\"/>\n    </manifest>\n    <spine toc=\"ncx\">\n        <itemref idref=\"p0\"/>\n        <itemref idref=\"p1\"/>\n    </spine>\n</package>\n", opf, "should be equal if cleaned correctly")
}
