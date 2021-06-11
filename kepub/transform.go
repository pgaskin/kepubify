package kepub

import (
	"bytes"
	"fmt"
	"io"
	"path"
	"regexp"
	"strconv"
	"strings"
	"unicode"

	"github.com/beevik/etree"
	"github.com/kr/smartypants"

	"github.com/pgaskin/kepubify/_/html/golang.org/x/net/html"
	"github.com/pgaskin/kepubify/_/html/golang.org/x/net/html/atom"
)

// TransformFileFilter returns true if a file should be filtered from the EPUB.
//
//  * [extra] remove calibre_bookmarks.txt
//
//  * [extra] remove iBooks metadata
//
//  * [extra] remove macOS metadata
//
//  * [extra] remove Windows metadata
//
func (c *Converter) TransformFileFilter(fn string) bool {
	switch path.Base(fn) {
	case "calibre_bookmarks.txt": // Calibre
		return true
	case "iTunesMetadata.plist", "iTunesArtwork.plist": // iBooks
		return true
	case ".DS_STORE": // macOS
		return true
	case "thumbs.db", "Thumbs.db": // Windows
		return true
	}
	switch path.Dir(fn) {
	case "__MACOSX":
		return true
	}
	return false
}

// TransformOPF transforms the OPF document for a KEPUB.
//
//  * [mandatory] add the cover-image property to the cover.
//    Kobo only supports the standardized EPUB3-style method of specifying the
//    cover (`manifest>item[properties="cover-image"]`), but most older EPUBs
//    will reference the manifest item with a meta element like
//    `meta[name="cover"][content="{manifest-item-id}"]`. or just set the
//    manifest item ID to `cover` instead of using `properties`.
//
//  * [extra] remove unnecessary Calibre metadata.
//    Removes extraneous metadata elements commonly added by Calibre.
//
func (c *Converter) TransformOPF(w io.Writer, r io.Reader) error {
	doc := etree.NewDocument()
	if _, err := doc.ReadFrom(r); err != nil {
		return fmt.Errorf("parse: %w", err)
	}

	transformOPFCoverImage(doc) // mandatory
	transformOPFCalibreMeta(doc)
	doc.Indent(4)

	if _, err := doc.WriteTo(w); err != nil {
		return fmt.Errorf("render: %w", err)
	}

	return nil
}

func transformOPFCoverImage(doc *etree.Document) {
	// property based on Kobo (checked with 3 books) as of 2020-01-12
	coverID := "cover"
	if el := doc.FindElement("//meta[@name='cover']"); el != nil {
		coverID = el.SelectAttrValue("content", coverID)
	}
	if el := doc.FindElement("//[@id='" + coverID + "']"); el != nil {
		el.CreateAttr("properties", "cover-image")
	}
}

func transformOPFCalibreMeta(doc *etree.Document) {
	for _, el := range doc.FindElements("//meta[@name='calibre:timestamp']") {
		el.Parent().RemoveChild(el)
	}
	for _, el := range doc.FindElements("//contributor[@role='bkp']") {
		el.Parent().RemoveChild(el)
	}
}

// TransformContent transforms an HTML4/HTML5/XHTML1.1 document for a KEPUB.
//
//  * [important] parses the XHTML with XHTML/XML/HTML4/HTML5-compatible rules
//    Quite a few books have invalid XHTML, and this prevents the markup from
//    being mangled any more than absolutely necessary. This also has the side
//    effect of fixing bad markup when combined with the render step at the end.
//    The intention is to match or exceed the kepub renderer's leniency. This
//    lenient parsing is also why kepubify often works better with badly-formed
//    HTML than Calibre. See the documentation in the x/net/html fork for more
//    information about how this works.
//
//    The most important changes to default HTML5 parsing rules are to allow
//    more tags to be self-closing, to ignore UTF-8 byte order marks, and to
//    preserve XML instructions.
//
//  * [mandatory] add Kobo style tweaks
//    To match official KEPUBs.
//
//  * [mandatory] add Kobo div wrappers
//    To match official KEPUBs. Kobo wraps the body with two div tags,
//    `div#book-columns > div#book-inner`, to provide a target for applying
//    pagination styles.
//
//  * [mandatory] add Kobo spans
//    To match official KEPUBs. Kobo adds spans surrounding each fragment (see
//    the regexp and matching logic) to provide better references to chunks of
//    text. Highlighting, bookmarking, and other related features don't work
//    without this.
//
//  * [optional] add extra CSS
//    For customization or to fix common issues.
//
//  * [optional] smarten punctuation
//    A common tweak to improve badly-formatted books.
//
//  * [extra] content cleanup
//    Removes Adept tags, extraneous MS Office tags, Unicode replacement chars,
//    etc.
//
//  * [important] renders the HTML as polyglot XHTML/HTML4/HTML5
//    The HTML is rendered for maximum compatibility and to be as close to the
//    original HTML as possible. See the documentation in the x/net/html fork
//    for more information about how this works.
//
//    The most important aspects are: the use of &#160; for non-breaking spaces,
//    always specifying xmlns on html/math/svg, always specifying a type on
//    script and style, always specifying a value for boolean attributes, always
//    adding a closing slash to void elements, never self-closing non-void
//    elements, only using XML-defined named escapes `<>&`, only using
//    HTML-style comments, ensuring table contents are well-formed, and
//    preserving the XML declaration if in the original code.
//
//  * [optional] find/replace
//    To allow users to apply quick one-off fixes to the generated HTML.
//
func (c *Converter) TransformContent(w io.Writer, r io.Reader) error {
	doc, err := html.ParseWithOptions(r,
		html.ParseOptionEnableScripting(true),
		html.ParseOptionIgnoreBOM(true),
		html.ParseOptionLenientSelfClosing(true))
	if err != nil {
		return fmt.Errorf("parse html: %w", err)
	}

	transformContentKoboStyles(doc) // mandatory
	transformContentKoboDivs(doc)   // mandatory
	transformContentKoboSpans(doc)  // mandatory

	for i := range c.extraCSS {
		transformContentAddStyle(doc, c.extraCSSClass[i], c.extraCSS[i])
	}

	if c.smartypants {
		transformContentPunctuation(doc)
	}

	transformContentClean(doc)

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

	err = html.RenderWithOptions(w, doc,
		html.RenderOptionAllowXMLDeclarations(true),
		html.RenderOptionPolyglot(true))
	if err != nil {
		return fmt.Errorf("render html: %w", err)
	}

	return nil
}

func transformContentKoboStyles(doc *html.Node) {
	// behavior based on Kobo (checked with 3 books) as of 2020-01-12
	// original looks like the following at the end of the head element:
	//     <style xmlns="http://www.w3.org/1999/xhtml" type="text/css" id="kobostylehacks">div#book-inner p, div#book-inner div { font-size: 1.0em; } a { color: black; } a:link, a:visited, a:hover, a:active { color: blue; } div#book-inner * { margin-top: 0 !important; margin-bottom: 0 !important;}</style>
	// modified to:
	// - get rid of the font size stuff, as it causes more problems than it solves
	// - get rid of the link colouring, as it's useless on e-ink (remember this same stylesheet is used on the apps and desktop too)
	// - not override all margins
	transformContentAddStyle(doc, "kobostylehacks", `div#book-inner { margin-top: 0; margin-bottom: 0;}`)
}

func transformContentKoboDivs(doc *html.Node) {
	// behavior matches Kobo (checked with 3 books) as of 2020-01-12
	// wrap body contents with div#book-columns > div#book-inner
	body := findAtom(doc, atom.Body)
	for c := body.FirstChild; c != nil; c = c.NextSibling {
		if c.Type == html.ElementNode && matchAttr(c, "id", "book-columns") {
			for ci := c.FirstChild; ci != nil; ci = ci.NextSibling {
				if ci.Type == html.ElementNode && matchAttr(ci, "id", "book-inner") {
					return // already has them
				}
			}
		}
	}
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

func transformContentKoboSpans(doc *html.Node) {
	// behavior matches Kobo (checked with 3 books) as of 2020-01-12
	if findClass(findAtom(doc, atom.Body), "koboSpan") != nil {
		return // already has kobo spans
	}

	var para, seg int
	var incParaNext bool

	var stack []*html.Node
	var cur *html.Node
	stack = append(stack, findAtom(doc, atom.Body))

	for len(stack) != 0 {
		stack, cur = stack[:len(stack)-1], stack[len(stack)-1]
		switch cur.Type {
		case html.TextNode:
			// split after each sentence (matches Kobo's behavior, and can't leave anything behind)
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
				if isSpace(sentence) && cur.Parent.DataAtom != atom.P {
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
					cur.Parent.InsertBefore(withText(koboSpan(para, seg), sentence), cur)
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
				if cur.Data == "math" || cur.Data == "svg" {
					continue
				}
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

func transformContentAddStyle(doc *html.Node, class, css string) {
	findAtom(doc, atom.Head).AppendChild(withText(&html.Node{
		Type:     html.ElementNode,
		DataAtom: atom.Style,
		Data:     "style",
		Attr: []html.Attribute{
			{Key: "type", Val: "text/css"},
			{Key: "class", Val: class},
		},
	}, css))
}

func transformContentPunctuation(doc *html.Node) {
	var stack []*html.Node
	var cur *html.Node
	stack = append(stack, findAtom(doc, atom.Body))

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
			if !isSpace(cur.Data) {
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

func transformContentClean(doc *html.Node) {
	var stack []*html.Node
	var cur *html.Node
	stack = append(stack, doc)

	for len(stack) != 0 {
		stack, cur = stack[:len(stack)-1], stack[len(stack)-1]
		switch cur.Type {
		case html.TextNode:
			if strings.ContainsRune(cur.Data, '�') {
				cur.Data = strings.ReplaceAll(cur.Data, "�", "")
			}
		case html.ElementNode:
			var remove bool

			// Adobe Adept meta tags
			remove = remove || (cur.DataAtom == atom.Meta && (matchAttr(cur, "name", "Adept.expected.resource") || matchAttr(cur, "name", "Adept.resource")))

			// Empty MS Word o:p and smart tags
			remove = remove || ((cur.Data == "o:p" || strings.HasPrefix(cur.Data, "st1:")) && matchEmpty(cur))

			if remove {
				cur.Parent.RemoveChild(cur)
				break
			}

			fallthrough
		case html.DocumentNode:
			for c := cur.LastChild; c != nil; c = c.PrevSibling {
				stack = append(stack, c)
			}
		}
	}
}

// withText adds text to a node and returns it.
func withText(node *html.Node, text string) *html.Node {
	if node.Type != html.ElementNode {
		panic("not an element node")
	}
	node.AppendChild(&html.Node{
		Type: html.TextNode,
		Data: text,
	})
	return node
}

// matchAttr compares the value of an attribute on an ElementNode, ignoring
// namespaces.
func matchAttr(n *html.Node, key, value string) bool {
	if n.Type == html.ElementNode {
		for _, a := range n.Attr {
			if a.Key == key && a.Val == value {
				return true
			}
		}
	}
	return false
}

// findAtom finds the first occurrence of an ElementNode matching the Atom.
func findAtom(n *html.Node, a atom.Atom) *html.Node {
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if c.Type == html.ElementNode && (c.DataAtom == a || (c.DataAtom == 0 && strings.ToLower(c.Data) == a.String())) {
			return c
		}
		if m := findAtom(c, a); m != nil {
			return m
		}
	}
	return nil
}

// findAtom finds the first occurrence of an ElementNode containing the class.
func findClass(n *html.Node, t string) *html.Node {
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if c.Type == html.ElementNode {
			for _, a := range c.Attr {
				if a.Key == "class" && includes(a.Val, t) {
					return c
				}
			}
		}
		if m := findClass(c, t); m != nil {
			return m
		}
	}
	return nil
}

// matchEmpty checks if a node only has comments or whitespace as direct children.
func matchEmpty(n *html.Node) bool {
	for child := n.FirstChild; child != nil; child = child.NextSibling {
		switch child.Type {
		case html.CommentNode:
			continue
		case html.TextNode:
			if !isSpace(child.Data) {
				return false
			}
			continue
		case html.ErrorNode:
			continue
		default:
			return false
		}
	}
	return true
}

// isSpace returns true if the string consists of only Unicode whitespace.
func isSpace(s string) bool {
	for _, c := range s {
		if !unicode.IsSpace(c) {
			return false
		}
	}
	return true
}

// includes returns true if the token is in the whitespace-delimited string.
func includes(s, token string) bool {
	for s != "" {
		i := strings.IndexAny(s, " \t\r\n\f")
		if i == -1 {
			return s == token
		}
		if s[:i] == token {
			return true
		}
		s = s[i+1:]
	}
	return false
}
