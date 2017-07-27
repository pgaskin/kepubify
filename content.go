package main

import (
	"bytes"
	"fmt"
	"regexp"
	"strings"

	"golang.org/x/net/html"

	"github.com/PuerkitoBio/goquery"
)

// addDivs adds kobo divs.
func addDivs(doc *goquery.Document) error {
	doc.Find("body>*").WrapAllHtml(`<div class="book-inner"></div>`)
	doc.Find("body>*").WrapAllHtml(`<div class="book-columns"></div>`)
	return nil
}

// addSpansToNode is a recursive helper function for addSpans.
func addSpansToNode(node *html.Node, paragraph *int, segment *int) {
	sentencere := regexp.MustCompile(`((?m).*?[\.\!\?\:]['"”’“…]?\s*)`)

	// Part 2 of hacky way of setting innerhtml of a textnode by double escaping everything, and deescaping once afterwards
	newAttr := []html.Attribute{}
	for _, a := range node.Attr {
		a.Key = html.EscapeString(a.Key)
		a.Val = html.EscapeString(a.Val)
		newAttr = append(newAttr, a)
	}
	node.Attr = newAttr

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

		var newhtml bytes.Buffer

		for _, sentence := range sentences {
			if strings.TrimSpace(sentence) != "" {
				newhtml.WriteString(fmt.Sprintf(`<span class="koboSpan" id="kobo.%v.%v">%s</span>`, *paragraph, *segment, sentence))
				*segment++
			}
		}

		// Part 1 of hacky way of setting innerhtml of a textnode by double escaping everything, and deescaping once afterwards
		node.Data = newhtml.String()

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
	for c := node.FirstChild; c != nil; c = c.NextSibling {
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

// openSelfClosingPs opens self-closing p tags.
func openSelfClosingPs(html *string) error {
	re := regexp.MustCompile(`<p[^>/]*/>`)
	*html = re.ReplaceAllString(*html, `<p></p>`)
	return nil
}

// smartenPunctuation smartens punctuation in html code. It must be run last.
func smartenPunctuation(html *string) error {
	// em and en dashes
	*html = strings.Replace(*html, "---", " &#x2013; ", -1)
	*html = strings.Replace(*html, "--", " &#x2014; ", -1)

	// TODO: smart quotes

	// Fix comments
	*html = strings.Replace(*html, "<! &#x2014; ", "<!-- ", -1)
	*html = strings.Replace(*html, " &#x2014; >", " -->", -1)
	return nil
}

// cleanHTML cleans up html for a kobo epub.
func cleanHTML(html *string) error {
	emptyHeadingRe := regexp.MustCompile(`<h\d+>\s*</h\d+>`)
	*html = emptyHeadingRe.ReplaceAllString(*html, "")

	msPRe := regexp.MustCompile(`\s*<o:p>\s*<\/o:p>`)
	*html = msPRe.ReplaceAllString(*html, " ")

	msStRe := regexp.MustCompile(`<\/?st1:\w+>`)
	*html = msStRe.ReplaceAllString(*html, "")

	// unicode replacement chars
	*html = strings.Replace(*html, "�", "", -1)

	// ADEPT drm tags
	adeptRe := regexp.MustCompile(`(<meta\s+content=".+"\s+name="Adept.expected.resource"\s+\/>)`)
	*html = adeptRe.ReplaceAllString(*html, "")

	return nil
}

// process processes the html of a content file in an ordinary epub and converts it into a kobo epub by adding kobo divs, kobo spans, smartening punctuation, and cleaning html.
func process(content *string) error {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(*content))
	if err != nil {
		return err
	}

	if err := addDivs(doc); err != nil {
		return err
	}

	if err := addSpans(doc); err != nil {
		return err
	}

	h, err := doc.Html()
	if err != nil {
		return err
	}

	// Part 3 of hacky way of setting innerhtml of a textnode by double escaping everything, and deescaping once afterwards. Must be done before further html processing
	h = html.UnescapeString(h)

	if err := openSelfClosingPs(&h); err != nil {
		return err
	}

	if err := cleanHTML(&h); err != nil {
		return err
	}

	if err := smartenPunctuation(&h); err != nil {
		return err
	}

	*content = h

	return nil
}
