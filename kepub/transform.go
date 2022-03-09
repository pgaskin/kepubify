package kepub

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"io/fs"
	"mime"
	"path"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/beevik/etree"
	"github.com/kr/smartypants"
	"golang.org/x/text/transform"

	"github.com/pgaskin/kepubify/_/html/golang.org/x/net/html"
	"github.com/pgaskin/kepubify/_/html/golang.org/x/net/html/atom"
	"github.com/pgaskin/kepubify/_/html/golang.org/x/net/html/charset"
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
//  * [important] ensure charset is UTF-8
//    EPUBs (and KEPUBs by extension) must be UTF-8/UTF-16.
//
func (c *Converter) TransformContent(w io.Writer, r io.Reader) error {
	switch strings.ToLower(c.charset) {
	case "utf-8", "":
		// do nothing
	case "auto":
		cr, err := charset.NewReader(r, "")
		if err != nil {
			return fmt.Errorf("parse html: detect charset: %w", err)
		}
		r = cr
	default:
		enc, _ := charset.Lookup(c.charset)
		if enc == nil {
			return fmt.Errorf("parset html: invalid charset %q", c.charset)
		}
		r = enc.NewDecoder().Reader(r)
	}

	doc, err := html.ParseWithOptions(r,
		html.ParseOptionEnableScripting(true),
		html.ParseOptionIgnoreBOM(true),
		html.ParseOptionLenientSelfClosing(true))
	if err != nil {
		return fmt.Errorf("parse html: %w", err)
	}

	transformContentCharsetUTF8(doc) // charset.NewReader always outputs UTF-8

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
		wc := transformContentReplacements(w, c.find, c.replace)
		w = wc
		defer wc.Close()
	}

	err = html.RenderWithOptions(w, doc,
		html.RenderOptionAllowXMLDeclarations(true),
		html.RenderOptionPolyglot(true))
	if err != nil {
		return fmt.Errorf("render html: %w", err)
	}

	return nil
}

func transformContentCharsetUTF8(doc *html.Node) {
	var stack []*html.Node
	var cur *html.Node
	stack = append(stack, findAtom(doc, atom.Head))

	// update the meta[charset] or meta[http-equiv="content-type"] if it's there
	for len(stack) != 0 {
		stack, cur = stack[:len(stack)-1], stack[len(stack)-1]
		if cur.Type == html.ElementNode {
			if cur.DataAtom == atom.Meta {
				for i, a := range cur.Attr {
					if a.Key == "charset" && strings.ToUpper(cur.Attr[i].Val) != "UTF-8" {
						cur.Attr[i].Val = "UTF-8"
						break
					}
					if a.Key == "http-equiv" && strings.ToLower(a.Val) == "content-type" {
						for j, b := range cur.Attr {
							if b.Key == "content" {
								if t, p, err := mime.ParseMediaType(b.Val); err == nil {
									if _, ok := p["charset"]; ok && strings.ToLower(p["charset"]) != "utf-8" {
										p["charset"] = "utf-8"
										cur.Attr[j].Val = mime.FormatMediaType(t, p)
									}
								}
								break
							}
						}
						break
					}
				}
			}
		}
		for c := cur.LastChild; c != nil; c = c.PrevSibling {
			stack = append(stack, c)
		}
	}
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

	sentences := make([]string, 0, 8)

	for len(stack) != 0 {
		stack, cur = stack[:len(stack)-1], stack[len(stack)-1]
		switch cur.Type {
		case html.TextNode:
			sentences = splitSentences(cur.Data, sentences[:0])

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

// splitSentences splits the string into sentences using the rules for creating
// koboSpans. To make this zero-allocation, pass a zero-length slice for
// splitSentences to take ownership of. To re-use the slice, pass the returned
// slice, sliced to zero. If the slice is too small, it will be grown, causing
// an allocation.
//
// This state-machine based implementation is around three times as fast as the
// regexp-based one on average, and even faster when pre-allocating and re-using
// the sentences slice. It should have the same output. For the original
// implementation, see splitSentencesRegexp in the tests.
func splitSentences(str string, sentences []string) []string {
	const (
		InputPunct   = iota // sentence-terminating punctuation
		InputExtra          // additional punctuation (one is optionionally consumed after punct if present)
		InputSpace          // whitespace
		InputAny            // any valid rune not previously matched
		InputInvalid        // an invalid byte
		InputEOS            // end-of-string
	)
	const (
		OutputNone = iota // moves to the next rune.
		OutputNext        // adds everything from the last call up to (but not including) the current rune, and moves to the next rune.
		OutputRest        // adds everything not yet added by OutputNext (state must be -1)
	)
	const (
		StateDefault         = iota // in a sentence
		StateAfterPunct             // after the sentence-terminating rune
		StateAfterPunctExtra        // after the optional additional punctuation rune
		StateAfterSpace             // the trailing whitespace after the sentence
	)

	if sentences == nil {
		sentences = make([]string, 0, 4) // pre-allocate some room
	}

	for i, state := 0, 0; state != -1; {
		x, z := utf8.DecodeRuneInString(str[i:])

		var input int
		switch x {
		case utf8.RuneError:
			switch z {
			case 0:
				input = InputEOS
			default:
				input = InputInvalid
			}
		case '.', '!', '?':
			input = InputPunct
		case '\'', '"', '”', '’', '“', '…':
			input = InputExtra
		case '\t', '\n', '\f', '\r', ' ': // \s only matches only ASCII whitespace
			input = InputSpace
		default:
			input = InputAny
		}

		var output int
		switch state {
		case StateDefault:
			switch input {
			case InputPunct:
				output, state = OutputNone, StateAfterPunct
			case InputExtra:
				output, state = OutputNone, StateDefault
			case InputSpace:
				output, state = OutputNone, StateDefault
			case InputAny:
				output, state = OutputNone, StateDefault
			case InputInvalid:
				output, state = OutputNone, StateDefault
			case InputEOS:
				output, state = OutputRest, -1
			default:
				panic("unhandled input")
			}
		case StateAfterPunct:
			switch input {
			case InputPunct:
				output, state = OutputNone, StateAfterPunct
			case InputExtra:
				output, state = OutputNone, StateAfterPunctExtra
			case InputSpace:
				output, state = OutputNone, StateAfterSpace
			case InputAny:
				output, state = OutputNone, StateDefault
			case InputInvalid:
				output, state = OutputNone, StateDefault
			case InputEOS:
				output, state = OutputRest, -1
			default:
				panic("unhandled input")
			}
		case StateAfterPunctExtra:
			switch input {
			case InputPunct:
				output, state = OutputNone, StateAfterPunct
			case InputExtra:
				output, state = OutputNone, StateDefault
			case InputSpace:
				output, state = OutputNone, StateAfterSpace
			case InputAny:
				output, state = OutputNone, StateDefault
			case InputInvalid:
				output, state = OutputNone, StateDefault
			case InputEOS:
				output, state = OutputRest, -1
			default:
				panic("unhandled input")
			}
		case StateAfterSpace:
			switch input {
			case InputPunct:
				output, state = OutputNext, StateAfterPunct
			case InputExtra:
				output, state = OutputNext, StateDefault
			case InputSpace:
				output, state = OutputNone, StateAfterSpace
			case InputAny:
				output, state = OutputNext, StateDefault
			case InputInvalid:
				output, state = OutputNext, StateDefault
			case InputEOS:
				output, state = OutputRest, -1
			default:
				panic("unhandled input")
			}
		default:
			panic("unhandled state")
		}

		switch output {
		case OutputNone:
			i += z
		case OutputNext:
			sentences = append(sentences, str[:i])
			str, i = str[i:], z
		case OutputRest:
			if len(str) != 0 || len(sentences) == 0 {
				sentences = append(sentences, str)
			}
			if state != -1 {
				panic("invalid state")
			}
		default:
			panic("unhandled output")
		}
	}

	return sentences
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

func transformContentReplacements(w io.Writer, find, replace [][]byte) io.WriteCloser {
	var t []transform.Transformer
	if len(find) != len(replace) {
		panic("find and replace must be the same length")
	}
	for i := range find {
		if len(find[i]) == 0 {
			continue
		}
		t = append(t, &byteReplacer{
			Find:    find[i],
			Replace: replace[i],
		})
	}
	return transform.NewWriter(w, transform.Chain(t...))
}

// TransformDummyTitlepage adds a dummy titlepage if forced or the heuristic
// determines that is is necessary. If there was an error determining if the
// titlepage is required, false and an error is returned. If it is not required,
// false is returned. If it is required, but there was an error when adding it,
// true and an error is returned.  If it was added successfully, the filename
// and contents of the content document to add to the epub and true is returned.
//
// The heuristic determines whether the first content file in the spine is a
// title page without other content, and if it isn't (or if force is true), it
// adds a blank content document and modifies the OPF to add it to the manifest
// and start of the spine. This is required because Kobo will treat the first
// spine entry specially (e.g. no margins) for full-screen book covers. See #33.
//
// Note that the heuristic is subject to change between kepubify versions.
func (c *Converter) TransformDummyTitlepage(epub fs.FS, opfF string, opf *bytes.Buffer) (string, io.Reader, bool, error) {
	if c.dummyTitlepageForce {
		if !c.dummyTitlepageForceValue {
			return "", nil, false, nil
		}
	} else {
		if req, err := transformDummyTitlepageRequired(epub, opfF, bytes.NewReader(opf.Bytes())); err != nil {
			return "", nil, false, fmt.Errorf("check if dummy titlepage is required: %w", err)
		} else if !req {
			return "", nil, false, nil
		}
	}
	fn, r, err := transformDummyTitlepageAdd(opf, opfF)
	if err != nil {
		return "", nil, true, fmt.Errorf("apply title page fix: %w", err)
	}
	return fn, r, true, nil
}

func transformDummyTitlepageRequired(epub fs.FS, opfF string, opfR io.Reader) (bool, error) {
	var opf struct {
		XMLName      xml.Name `xml:"http://www.idpf.org/2007/opf package"`
		ManifestItem []struct {
			Id        string `xml:"id,attr"`
			Href      string `xml:"href,attr"`
			MediaType string `xml:"media-type,attr"`
		} `xml:"http://www.idpf.org/2007/opf manifest>item"`
		SpineItemref []struct {
			Idref  string `xml:"idref,attr"`
			Linear string `xml:"linear,attr"`
		} `xml:"http://www.idpf.org/2007/opf spine>itemref"`
	}

	if err := xml.NewDecoder(opfR).Decode(&opf); err != nil {
		return false, fmt.Errorf("parse OPF package: %w", err)
	}

	var idref string
	for _, it := range opf.SpineItemref {
		if it.Linear != "no" {
			idref = it.Idref
			break
		}
	}
	if idref == "" {
		return false, nil // there should always be at least one linear spine item, and it should always reference something, but we'll be lenient
	}

	var href string
	for _, it := range opf.ManifestItem {
		if it.Id == idref {
			switch it.MediaType {
			case "application/xhtml+xml", "text/html":
				href = it.Href
				break
			}
			switch strings.ToLower(path.Ext(it.Href)) {
			case ".htm", ".html", ".xhtml":
				href = it.Href
				break
			default:
				return false, nil // the first spine item is not a content document, so let it be
			}
		}
	}
	if href == "" {
		return false, nil // the thing it references should always exist, but we'll be lenient
	}

	if n := strings.ToLower(path.Base(href)); strings.Contains(n, "cover") || strings.Contains(n, "title") {
		return false, nil // this is intended to be the cover/title page (or else why would it have cover/title in the name?)
	}

	href = path.Join(path.Dir(opfF), href)

	rc, err := epub.Open(href)
	if err != nil {
		return false, nil // the file it references should exist, but we'll be lenient
	}
	defer rc.Close()

	doc, err := html.ParseWithOptions(rc,
		html.ParseOptionEnableScripting(true),
		html.ParseOptionIgnoreBOM(true),
		html.ParseOptionLenientSelfClosing(true))
	if err != nil {
		return false, nil // we'll ignore it here
	}

	var wc, pc, ic int
	var cur *html.Node
	var stack []*html.Node
	stack = append(stack, findAtom(doc, atom.Body))
	for len(stack) != 0 {
		stack, cur = stack[:len(stack)-1], stack[len(stack)-1]
		switch cur.Type {
		case html.ElementNode:
			switch cur.DataAtom {
			case atom.P:
				pc++
				if pc > 4 {
					return true, nil
				}
			case atom.Img, atom.Svg:
				ic++
				fallthrough
			case atom.Script, atom.Style, atom.Pre, atom.Audio, atom.Video, atom.Math:
				continue
			default:
				for c := cur.LastChild; c != nil; c = c.PrevSibling {
					stack = append(stack, c)
				}
			}
		case html.TextNode:
			for _, w := range strings.Fields(cur.Data) {
				if len(w) > 3 {
					wc++
				}
			}
			if wc > 20 {
				return true, nil
			}
		}
	}

	if (ic == 0 && wc < 5) || ic > 4 {
		return true, nil
	}

	return false, nil
}

func transformDummyTitlepageAdd(opf *bytes.Buffer, opfF string) (string, io.Reader, error) {
	id, mime := "kepubify-titlepage-dummy", "application/xhtml+xml"
	href := id + ".xhtml"
	fn := path.Join(path.Dir(opfF), href)

	doc := etree.NewDocument()
	if _, err := doc.ReadFrom(bytes.NewReader(opf.Bytes())); err != nil {
		return "", nil, fmt.Errorf("parse opf: %w", err)
	}

	if m := doc.FindElement("/package/manifest"); m != nil {
		it := m.CreateElement("item")
		it.Space = m.Space // shouldn't usually be needed, but just in case they used a namespace prefix
		it.CreateAttr("id", id)
		it.CreateAttr("href", href)
		it.CreateAttr("media-type", mime)
	}
	if m := doc.FindElement("/package/spine"); m != nil {
		it := m.CreateElement("itemref")
		it.Space = m.Space // shouldn't usually be needed, but just in case they used a namespace prefix
		it.CreateAttr("idref", id)
		m.InsertChildAt(1, it)
	}
	doc.Indent(4) // same as TransformOPF

	opf.Reset()
	if _, err := doc.WriteTo(opf); err != nil {
		return "", nil, fmt.Errorf("render opf: %w", err)
	}

	return fn, strings.NewReader(`<!DOCTYPE html><html xmlns="http://www.w3.org/1999/xhtml" lang="en"><head><title></title></head><body><p style="text-align: center; margin: 4em 0; font-size: .7em; font-style: italic;">Page intentionally left blank by kepubify.</p></body></html>`), nil
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

// byteReplacer is a Transformer which finds and replaces sequences of bytes.
type byteReplacer struct {
	transform.NopResetter
	Find, Replace []byte
}

func (b *byteReplacer) Transform(dst, src []byte, atEOF bool) (nDst, nSrc int, err error) {
	if len(b.Find) == 0 {
		panic("find length must not be zero")
	}

	for {
		// find the next match
		i := bytes.Index(src[nSrc:], b.Find)
		if i == -1 {
			break
		}

		// copy the non-matching prefix
		if n := copy(dst[nDst:], src[nSrc:nSrc+i]); n == len(src[nSrc:nSrc+i]) {
			nSrc += n
			nDst += n
		} else {
			// skip what we've already processed
			nSrc += n
			nDst += n
			// have it call us again with a larger destination buffer
			err = transform.ErrShortDst
			return
		}

		// copy the new value
		if n := copy(dst[nDst:], b.Replace); n == len(b.Replace) {
			nSrc += len(b.Find)
			nDst += n
		} else {
			// have it call us again with a larger destination buffer
			err = transform.ErrShortDst
			return
		}
	}

	if !atEOF {
		// skip everything, minus the last len(b.Replace)-1 in case there is another
		// partial match at the end
		if skip := len(src[nSrc:]) - (len(b.Find) - 1); skip > 0 {
			if n := copy(dst[nDst:], src[nSrc:nSrc+skip]); n == len(src[nSrc:nSrc+skip]) {
				nSrc += n
				nDst += n
			} else {
				// skip what we've already processed
				nSrc += n
				nDst += n
				// have it call us again with a larger destination buffer
				err = transform.ErrShortDst
				return
			}
		}

		// have it call us again with more source bytes to find another match in
		err = transform.ErrShortSrc
		return
	}

	// at EOF, and no more replacements, so copy the remaining bytes
	if n := copy(dst[nDst:], src[nSrc:]); n == len(src[nSrc:]) {
		nDst += n
		nSrc += n
	} else {
		// skip what we've already copied
		nDst += n
		nSrc += n
		// have it call us again with a larger destination buffer
		err = transform.ErrShortDst
	}
	return
}
