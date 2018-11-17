package kepub

import (
	"fmt"
	"regexp"
	"strings"

	"golang.org/x/net/html"

	"github.com/PuerkitoBio/goquery"
)

// addDivs adds kobo divs.
func addDivs(doc *goquery.Document) error {
	// If there are more divs than ps, divs are probably being used as paragraphs, and adding the kobo divs will most likely break the book.
	if len(doc.Find("div").Nodes) > len(doc.Find("p").Nodes) {
		return nil
	}

	// Add the kobo divs
	doc.Find("body>*").WrapAllHtml(`<div class="book-inner"></div>`)
	doc.Find("body>*").WrapAllHtml(`<div class="book-columns"></div>`)

	return nil
}

// createSpan creates a Kobo span
func createSpan(paragraph, segment int, text string) *html.Node {
	// Create the span
	span := &html.Node{
		Type: html.ElementNode,
		Data: "span",
		Attr: []html.Attribute{{
			Key: "class",
			Val: "koboSpan",
		}, {
			Key: "id",
			Val: fmt.Sprintf("kobo.%v.%v", paragraph, segment),
		}},
	}

	// Add the text
	span.AppendChild(&html.Node{
		Type: html.TextNode,
		Data: text,
	})

	return span
}

var sentencere = regexp.MustCompile(`((?ms).*?[\.\!\?\:]['"”’“…]?\s*)`)
var nbspre = regexp.MustCompile(`^\xa0+$`)

// addSpansToNode is a recursive helper function for addSpans.
func addSpansToNode(node *html.Node, paragraph *int, segment *int) {
	nextNodes := []*html.Node{}
	for c := node.FirstChild; c != nil; c = c.NextSibling {
		nextNodes = append(nextNodes, c)
	}

	if node.Type == html.TextNode {
		if node.Parent.Data == "pre" {
			// Do not add spans to pre elements
			return
		}

		*segment++

		sentencesindexes := sentencere.FindAllStringIndex(node.Data, -1)
		sentences := []string{}
		lasti := []int{0, 0}
		for _, i := range sentencesindexes {
			if lasti[1] != i[0] {
				// If gap in regex matches, add the gap to the sentence list to avoid losing text
				sentences = append(sentences, node.Data[lasti[1]:i[0]])
			}
			sentences = append(sentences, node.Data[i[0]:i[1]])
			lasti = i
		}
		if lasti[1] != len(node.Data) {
			// If gap in regex matches, add the gap to the sentence list to avoid losing text
			sentences = append(sentences, node.Data[lasti[1]:len(node.Data)])
		}

		for i, sentence := range sentences {
			// if only 1 space, don't remove the element (issue #14) (issue #21)
			if (i == 0 && node.Data == " ") || (i == 0 && nbspre.MatchString(node.Data)) || strings.TrimSpace(sentence) != "" {
				node.Parent.InsertBefore(createSpan(*paragraph, *segment, sentence), node)
				*segment++
			}
		}

		node.Parent.RemoveChild(node)

		return
	}

	if node.Type != html.ElementNode {
		return
	}
	if node.Data == "img" {
		return
	}
	if node.Data == "p" || node.Data == "ol" || node.Data == "ul" {
		*segment = 0
		*paragraph++
	}

	for _, c := range nextNodes {
		addSpansToNode(c, paragraph, segment)
	}
}

// addSpans adds kobo spans.
func addSpans(doc *goquery.Document) error {
	alreadyHasSpans := false
	doc.Find("span").Each(func(i int, s *goquery.Selection) {
		if val, _ := s.Attr("class"); strings.Contains(val, "koboSpan") {
			alreadyHasSpans = true
		}
	})
	if alreadyHasSpans {
		return nil
	}

	paragraph := 0
	segment := 0

	for _, n := range doc.Find("body").Nodes {
		addSpansToNode(n, &paragraph, &segment)
	}

	return nil
}

// addKoboStyles adds kobo styles.
func addKoboStyles(doc *goquery.Document) error {
	s := doc.Find("head").First().AppendHtml(`<style type="text/css">div#book-inner{margin-top: 0;margin-bottom: 0;}</style>`)
	if s.Length() != 1 {
		return fmt.Errorf("could not append kobo styles")
	}
	return nil
}

// smartenPunctuation smartens punctuation in html code. It must be run last.
func smartenPunctuation(html string) string {
	// em and en dashes
	html = strings.Replace(html, "---", " &#x2013; ", -1)
	html = strings.Replace(html, "--", " &#x2014; ", -1)

	// TODO: smart quotes

	// Fix comments
	html = strings.Replace(html, "<! &#x2014; ", "<!-- ", -1)
	html = strings.Replace(html, " &#x2014; >", " -->", -1)
	return html
}

// cleanHTML cleans up html for a kobo epub.
func cleanHTML(doc *goquery.Document) error {
	// Remove Adobe DRM tags
	doc.Find(`meta[name="Adept.expected.resource"]`).Remove()

	// Remove empty MS <o:p> tags
	doc.Find(`o\:p`).FilterFunction(func(_ int, s *goquery.Selection) bool {
		return strings.Trim(s.Text(), "\t \n") == ""
	}).Remove()

	// Remove empty headings
	doc.Find(`h1,h2,h3,h4,h5,h6`).FilterFunction(func(_ int, s *goquery.Selection) bool {
		h, _ := s.Html()
		return strings.Trim(h, "\t \n") == ""
	}).Remove()

	// Remove MS <st1:whatever> tags
	doc.Find(`*`).FilterFunction(func(_ int, s *goquery.Selection) bool {
		return strings.HasPrefix(goquery.NodeName(s), "st1:")
	}).Remove()

	// Open self closing p tags
	doc.Find(`p`).Each(func(_ int, s *goquery.Selection) {
		if s.Children().Length() == 0 && strings.Trim(s.Text(), "\n \t") == "" {
			s.SetHtml("")
		}
	})

	doc.Find("svg").SetAttr("xmlns:xlink", "http://www.w3.org/1999/xlink")
	doc.Find("svg a").RemoveAttr("xmlns:xlink")
	doc.Find("svg image").RemoveAttr("xmlns:xlink")

	// Add type to style tags
	doc.Find(`style`).SetAttr("type", "text/css")

	return nil
}

var selfClosingScriptRe = regexp.MustCompile(`<(script)([^>]*?)\/>`)
var selfClosingTitleRe = regexp.MustCompile("<title */>")

// fixInvalidSelfClosingTags fixes invalid self-closing tags which cause breakages. It must be run first.
func fixInvalidSelfClosingTags(html string) string {
	html = selfClosingTitleRe.ReplaceAllString(html, "<title>book</title>")
	html = selfClosingScriptRe.ReplaceAllString(html, "<$1$2> </$1>")
	return html
}
