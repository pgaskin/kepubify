package kepub

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"unicode"

	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"

	"github.com/andybalholm/cascadia"
	"github.com/beevik/etree"
	"github.com/kr/smartypants"
)

// --- Content Transformations --- //

// transform1 loads the document from r.
func (c *Converter) transform1(r io.Reader) (*html.Node, error) {
	return html.ParseWithOptions(r,
		html.ParseOptionEnableScripting(true),
		html.ParseOptionIgnoreBOM(true),
		html.ParseOptionLenientSelfClosing(true))
}

// transform2 makes the necessary changes to the document.
func (c *Converter) transform2(doc *html.Node) error {
	// TODO: inline styles option?

	transform2koboStyles(doc) // mandatory
	transform2koboDivs(doc)   // mandatory
	transform2koboSpans(doc)  // mandatory

	for i := range c.extraCSS {
		transform2addStyle(doc, c.extraCSSClass[i], c.extraCSS[i])
	}

	if c.smartypants {
		transform2smartypants(doc)
	}

	transform2cleanHTML(doc)

	return nil
}

// transform3 writes the transformed document to w.
func (c *Converter) transform3(w io.Writer, doc *html.Node) error {
	if len(c.find) != 0 {
		buf := bytes.NewBuffer(nil)
		if err := html.RenderWithOptions(buf, doc,
			html.RenderOptionAllowXMLDeclarations(true),
			html.RenderOptionPolyglot(true)); err != nil {
			return err
		}
		b := buf.Bytes()
		for i := range c.find {
			b = bytes.ReplaceAll(b, c.find[i], c.replace[i])
		}
		_, err := w.Write(b)
		return err
	}

	return html.RenderWithOptions(w, doc,
		html.RenderOptionAllowXMLDeclarations(true),
		html.RenderOptionPolyglot(true))
}

func transform2koboStyles(doc *html.Node) {
	// behaviour based on Kobo (checked with 3 books) as of 2020-01-12
	// original looks like the following at the end of the head element:
	//     <style xmlns="http://www.w3.org/1999/xhtml" type="text/css" id="kobostylehacks">div#book-inner p, div#book-inner div { font-size: 1.0em; } a { color: black; } a:link, a:visited, a:hover, a:active { color: blue; } div#book-inner * { margin-top: 0 !important; margin-bottom: 0 !important;}</style>
	// modified to:
	// - get rid of the font size stuff, as it causes more problems than it solves
	// - get rid of the link colouring, as it's useless on eink (remember this same stylesheet is used on the apps and desktop too)
	// - not override all margins
	transform2addStyle(doc, "kobostylehacks", `div#book-inner { margin-top: 0; margin-bottom: 0;}`)
}

func transform2koboDivs(doc *html.Node) {
	// behaviour matches Kobo (checked with 3 books) as of 2020-01-12
	// wrap body contents with div#book-columns > div#book-inner
	if cascadia.Query(doc, sel("#book-columns > #book-inner")) != nil {
		return // already has kobo divs
	}

	body := cascadia.Query(doc, sel("body"))
	for _, layer := range []string{"book-inner", "book-columns"} {
		div := &html.Node{
			Type:     html.ElementNode,
			DataAtom: atom.Div,
			Data:     "div",
			Attr: []html.Attribute{{
				Key: "id",
				Val: layer,
			}},
		}

		// wrap the body contents with the div
		for child := body.FirstChild; child != nil; child = child.NextSibling {
			child.Parent = div // put each child in the chain into the div
		}
		div.FirstChild, div.LastChild = body.FirstChild, body.LastChild // put the chain in the div
		div.Parent, body.FirstChild, body.LastChild = body, div, div    // replace body's contents with the div
	}
}

var sentencere = regexp.MustCompile(`((?ms).*?[\.\!\?]['"”’“…]?\s+)`)

func transform2koboSpans(doc *html.Node) {
	// behaviour matches Kobo (checked with 3 books) as of 2020-01-12
	if cascadia.Query(doc, sel(".koboSpan")) != nil {
		return // already has kobo spans
	}

	var para, seg int
	var incParaNext bool

	var stack []*html.Node
	var cur *html.Node
	stack = append(stack, cascadia.Query(doc, sel("body")))

	for len(stack) != 0 {
		stack, cur = stack[:len(stack)-1], stack[len(stack)-1]
		switch cur.Type {
		case html.TextNode:
			// split after each sentence (matches Kobo's behaviour, and can't leave anything behind)
			var sentences []string
			if matches := sentencere.FindAllStringIndex(cur.Data, -1); len(matches) == 0 {
				sentences = []string{cur.Data} // nothing matched, use the whole string
			} else {
				var pos int
				sentences = make([]string, len(matches))
				for i, match := range matches {
					sentences[i] = cur.Data[pos:match[1]] // end of last match to end of the current one
					pos = match[1]
				}
				if len(cur.Data) > pos {
					sentences = append(sentences, cur.Data[pos:]) // rest of the string, if any
				}
			}

			// wrap each sentence in a span (don't wrap whitespace unless it is
			// directly under a P tag [TODO: are there any other cases we wrap
			// whitespace? ... I need to find a kepub like this]) and add it
			// back to the parent.
			for _, sentence := range sentences {
				if allSpace(sentence) && cur.Parent.DataAtom != atom.P {
					cur.Parent.InsertBefore(&html.Node{
						Type: html.TextNode,
						Data: sentence,
					}, cur)
				} else {
					if incParaNext {
						para++
						seg = 0
						incParaNext = false
					}

					seg++
					cur.Parent.InsertBefore(nodeWithText(koboSpan(para, seg), sentence), cur)
				}
			}

			// remove the old TextNode with everything
			cur.Parent.RemoveChild(cur)

		case html.ElementNode:
			switch cur.DataAtom {
			case atom.Img:
				// increment the paragraph immediately
				para++
				seg = 0
				incParaNext = false

				// add a span around the image
				seg++
				s := koboSpan(para, seg)
				s.AppendChild(&html.Node{
					Type:      cur.Type,
					DataAtom:  cur.DataAtom,
					Namespace: cur.Namespace,
					Data:      cur.Data,
					Attr:      cur.Attr,
					// note: img elements don't have children
				})
				cur.Parent.InsertBefore(s, cur)
				cur.Parent.RemoveChild(cur)

				fallthrough
			case atom.Script, atom.Style, atom.Pre, atom.Audio, atom.Video, atom.Svg, atom.Math:
				continue // don't add spans to elements which should keep text as-is
			case atom.P, atom.Ol, atom.Ul, atom.Table, atom.H1, atom.H2, atom.H3, atom.H4, atom.H5, atom.H6:
				incParaNext = true // increment it only if it will have spans in it
				fallthrough
			default:
				// add the next nodes to the stack (in reverse order, since
				// we're doing a depth-first traversal from top to bottom).
				for c := cur.LastChild; c != nil; c = c.PrevSibling {
					stack = append(stack, c)
				}
			}
		}
	}
}

func koboSpan(para, seg int) *html.Node {
	return &html.Node{
		Type:     html.ElementNode,
		Data:     "span",
		DataAtom: atom.Span,
		Attr: []html.Attribute{
			{Key: "class", Val: "koboSpan"},
			{Key: "id", Val: "kobo." + strconv.Itoa(para) + "." + strconv.Itoa(seg)},
		},
	}
}

func transform2addStyle(doc *html.Node, class, css string) {
	cascadia.Query(doc, sel("head")).AppendChild(nodeWithText(&html.Node{
		Type:     html.ElementNode,
		DataAtom: atom.Style,
		Data:     "style",
		Attr: []html.Attribute{
			{Key: "type", Val: "text/css"},
			{Key: "class", Val: class},
		},
	}, css))
}

func transform2smartypants(doc *html.Node) {
	var stack []*html.Node
	var cur *html.Node
	stack = append(stack, cascadia.Query(doc, sel("body")))

	for len(stack) != 0 {
		stack, cur = stack[:len(stack)-1], stack[len(stack)-1]
		switch cur.Type {
		case html.ElementNode:
			switch cur.DataAtom {
			case atom.Pre, atom.Code, atom.Style, atom.Script:
				continue
			default:
				for c := cur.LastChild; c != nil; c = c.PrevSibling {
					stack = append(stack, c)
				}
			}
		case html.TextNode:
			if !allSpace(cur.Data) {
				buf := bytes.NewBuffer(nil)
				if _, err := smartypants.New(buf, smartypants.LatexDashes).Write([]byte(cur.Data)); err != nil {
					panic(err) // smartypants should never error on its own
				}
				// (*smartypants.writer).write calls smartypants.attrEscape on
				// the passed data (which has been unescaped by the parser),
				// which escapes the HTML entities, so we need to unescape it
				// after it has been processed.
				cur.Data = html.UnescapeString(buf.String())
			}
		}
	}
}

func transform2cleanHTML(doc *html.Node) {
	for _, el := range cascadia.QueryAll(doc, matcherGroup{
		// Adobe Adept meta tags
		sel(`meta[name="Adept.expected.resource"]`),
		sel(`meta[name="Adept.resource"]`),
		// Empty MS Word o:p tags
		onlyEmptyText(sel(`o\:p`)),
		// MS Word smart tags
		selPrefix("st1:"),
	}) {
		el.Parent.RemoveChild(el)
	}

	var stack []*html.Node
	var cur *html.Node
	stack = append(stack, cascadia.Query(doc, sel("body")))

	for len(stack) != 0 {
		stack, cur = stack[:len(stack)-1], stack[len(stack)-1]
		switch cur.Type {
		case html.ElementNode:
			for c := cur.LastChild; c != nil; c = c.PrevSibling {
				stack = append(stack, c)
			}
		case html.TextNode:
			if strings.ContainsRune(cur.Data, '�') {
				cur.Data = strings.ReplaceAll(cur.Data, "�", "")
			}
		}
	}

	// Note: The other cleanups for ensuring valid XHTML and other misc HTML
	//       fixes are now part of my forked golang.org/x/net/html package (see
	//       go test -run "^TestMod_" golang.org/x/net/html -v).
}

// --- OPF Transformations --- //

// transformOPF1 loads the package document from r.
func (c *Converter) transformOPF1(r io.Reader) (*etree.Document, error) {
	doc := etree.NewDocument()
	_, err := doc.ReadFrom(r)
	return doc, err
}

// transformOPF2 makes the necessary changes to the package document.
func (c *Converter) transformOPF2(doc *etree.Document) error {
	transformOPF2coverImage(doc) // mandatory
	transformOPF2calibreMeta(doc)
	doc.Indent(4)
	return nil
}

// transformOPF3 writes the transformed package document to w.
func (c *Converter) transformOPF3(w io.Writer, doc *etree.Document) error {
	_, err := doc.WriteTo(w)
	return err
}

func transformOPF2coverImage(doc *etree.Document) {
	// property based on Kobo (checked with 3 books) as of 2020-01-12
	coverID := "cover"
	if el := doc.FindElement("//meta[@name='cover']"); el != nil {
		coverID = el.SelectAttrValue("content", coverID)
	}

	if el := doc.FindElement("//[@id='" + coverID + "']"); el != nil {
		el.CreateAttr("properties", "cover-image")
	}
}

func transformOPF2calibreMeta(doc *etree.Document) {
	for _, el := range doc.FindElements("//meta[@name='calibre:timestamp']") {
		el.Parent().RemoveChild(el)
	}

	for _, el := range doc.FindElements("//contributor[@role='bkp']") {
		el.Parent().RemoveChild(el)
	}
}

// --- EPUB Transformations --- //

// transformEPUB makes the nessary changes to the EPUB layout.
func (c *Converter) transformEPUB(dir string) error {
	transformEPUBcleanupFiles(dir)
	return nil
}

func transformEPUBcleanupFiles(dir string) {
	_ = filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // ignore errors
		}
		for _, fn := range []string{
			"calibre_bookmarks.txt", // Calibre
			"iTunesMetadata.plist",  // iBooks
			"iTunesArtwork.plist",   // iBooks
			".DS_STORE",             // macOS
			"__MACOSX",              // macOS
			"thumbs.db",             // Windows
		} {
			if filepath.Base(path) == fn {
				_ = os.RemoveAll(path)
			}
		}
		return nil
	})
}

// --- Utilities --- //

// matcherFunc wraps a function into a cascadia.Matcher.
type matcherFunc func(*html.Node) bool

func (m matcherFunc) Match(n *html.Node) bool { return m(n) }

// matcherGroup matches any of the cascadia.Matchers within.
type matcherGroup []cascadia.Matcher

func (m matcherGroup) Match(n *html.Node) bool {
	for _, v := range m {
		if v.Match(n) {
			return true
		}
	}
	return false
}

// sel returns a cascadia.Matcher to match a CSS selector.
func sel(s string) cascadia.Matcher {
	ss, err := cascadia.ParseGroup(s)
	if err != nil {
		panic(err)
	}
	return ss
}

// selPrefix returns a cascadia.Matcher to select elements by their tag prefix.
func selPrefix(p string) cascadia.Matcher {
	return matcherFunc(func(n *html.Node) bool {
		return n.Type == html.ElementNode && strings.HasPrefix(n.Data, p)
	})
}

// onlyEmptyText wraps a cascadia.Matcher to only match elements with only
// comments or whitespace.
func onlyEmptyText(m cascadia.Matcher) cascadia.Matcher {
	return matcherFunc(func(n *html.Node) bool {
		if !m.Match(n) {
			return false
		}
		for child := n.FirstChild; child != nil; child = child.NextSibling {
			if child.Type != html.CommentNode && (child.Type == html.TextNode && !allSpace(child.Data)) {
				return false
			}
		}
		return true
	})
}

// allSpace returns true if the string only contains whitespace.
func allSpace(str string) bool {
	for _, c := range str {
		if !unicode.IsSpace(c) {
			return false
		}
	}
	return true
}

// nodeWithText adds text to a node and returns it.
func nodeWithText(node *html.Node, text string) *html.Node {
	if node.Type != html.ElementNode {
		panic("not an element node")
	}
	node.AppendChild(&html.Node{
		Type: html.TextNode,
		Data: text,
	})
	return node
}
