// Copyright 2011 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Modifications Copyright 2020 Patrick Gaskin.

package html

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"strings"
	"unicode/utf8"

	a "github.com/pgaskin/kepubify/_/html/golang.org/x/net/html/atom"
)

// MOD(pgaskin): Add render options
type renderOpts struct {
	// xmlDecl uncomments XML declarations which were commented out during
	// parsing.
	xmlDecl bool
	// polyglot produces HTML which is mostly valid XML/XHTML as well. See the
	// comment for RenderOptionPolyglot for more details.
	polyglot bool
}

// RenderOption configures a renderer.
type RenderOption func(o *renderOpts)

// RenderOptionAllowXMLDeclarations uncomments XML declarations which were
// commented out during parsing. Note that this does not ensure the content is
// valid XML (although if the content had an XML declaration to begin with, it
// probably was and likely remain that way even after being re-rendered).
func RenderOptionAllowXMLDeclarations(enabled bool) RenderOption {
	return func(o *renderOpts) {
		o.xmlDecl = enabled
	}
}

// RenderOptionPolyglot produces HTML which is mostly valid XML/XHTML as well.
//
// The output will also be mostly XHTML 1.1 compatible if the source was XHTML
// 1.1 or HTML4 and the RenderOptionAllowXMLDeclarations option is used.
// Essentially, this option will keep the output document with as wide a
// compatiblity compared to the source document as possible.
//
// The following characteristics are implemented by this option:
//     - Replacing literal NBSP characters with the &#160; escape. This is because
//       quite a few parsers seem to trim literal NBSPs.
//     - Adding the xmlns attribute to the root element (note that this does not
//       cause any issues when parsing as normal HTML5) (see
//       https://stackoverflow.com/a/14564065).
//     - Adding xmlns to math and svg elements.
//     - Adding type="text/css" to style elements without a type,
//       type="text/javascript" to script elements without a type (required for
//       HTML4/XHTML1.1, default for HTML5) (note that a type isn't required on
//       link elements).
//
// The following characteristics are already part of the Go renderer by default:
//     - Always including a value for boolean attributes (i.e. <input enabled="" />
//       rather than <input enabled />).
//     - Always putting the self-closing / in void elements (i.e. <br /> rather than
//       <br>). This is required for XML compatibility.
//     - All non-void elements will have a closing tag, even if they are empty. This
//       is required for HTML compatibility (see ParseOptionLenientSelfClosing).
//     - Only escaping <>& using named escapes, everything else is as-is or
//       uses numerical escapes (but see the note about NBSPs in the next section).
//     - Only using <!-- and --> for comments.
//     - Wrapping table contents in tbody if not already done (note that this is
//       done in the parser as per the HTML5 spec, not the renderer).
//
// The following characteristics are NOT implemented by this option:
//     - CDATA escaping for scripts and stylesheets. This will cause parsing as
//       strict XML to fail for embedded scripts with the characters <>&.
//     - Declaring encoding as UTF-8. XML is required to be UTF-8, and many HTML
//       parsers will default to it anyways. Also, most source documents will
//       already have it declared.
//     - The DOCTYPE will be left however it was in the source document. This
//       doesn't usually have any effect in either direction for most recent parsers.
//     - xlink:href on links. This usually causes more issues than it solves, and
//       all but the most strict XML parsers will work fine without it. Although,
//       if it is already set, it will be preserved.
//
// Note that you will need to enable the RenderOptionAllowXMLDeclarations option
// if using this to manipulate strict EPUB2 XHTML content for some readers to
// parse it correctly (i.e. the Kobo KEPUB reader).
//
// Basically, as long as the source document is mostly correct HTML or XHTML
// (see my parser mods for making the parsing more lenient about XMLisms), the
// output will be parseable correctly as XML or HTML by most parsers.
//
// References:
//     - https://www.w3.org/TR/html-polyglot/
//     - https://html.spec.whatwg.org/multipage/parsing.html
//     - https://stackoverflow.com/a/39560454
func RenderOptionPolyglot(enabled bool) RenderOption {
	return func(o *renderOpts) {
		o.polyglot = enabled
	}
}

// RenderWithOptions is like Render, with options.
func RenderWithOptions(w io.Writer, n *Node, opts ...RenderOption) error {
	o := &renderOpts{}
	for _, f := range opts {
		f(o)
	}

	// copied from Render
	if x, ok := w.(writer); ok {
		return render(x, n, o)
	}
	buf := bufio.NewWriter(w)
	if err := render(buf, n, o); err != nil {
		return err
	}
	return buf.Flush()
}

// note that the *renderOpts needed to also be added to the render functions
// END MOD

type writer interface {
	io.Writer
	io.ByteWriter
	WriteString(string) (int, error)
}

// Render renders the parse tree n to the given writer.
//
// Rendering is done on a 'best effort' basis: calling Parse on the output of
// Render will always result in something similar to the original tree, but it
// is not necessarily an exact clone unless the original tree was 'well-formed'.
// 'Well-formed' is not easily specified; the HTML5 specification is
// complicated.
//
// Calling Parse on arbitrary input typically results in a 'well-formed' parse
// tree. However, it is possible for Parse to yield a 'badly-formed' parse tree.
// For example, in a 'well-formed' parse tree, no <a> element is a child of
// another <a> element: parsing "<a><a>" results in two sibling elements.
// Similarly, in a 'well-formed' parse tree, no <a> element is a child of a
// <table> element: parsing "<p><table><a>" results in a <p> with two sibling
// children; the <a> is reparented to the <table>'s parent. However, calling
// Parse on "<a><table><a>" does not return an error, but the result has an <a>
// element with an <a> child, and is therefore not 'well-formed'.
//
// Programmatically constructed trees are typically also 'well-formed', but it
// is possible to construct a tree that looks innocuous but, when rendered and
// re-parsed, results in a different tree. A simple example is that a solitary
// text node would become a tree containing <html>, <head> and <body> elements.
// Another example is that the programmatic equivalent of "a<head>b</head>c"
// becomes "<html><head><head/><body>abc</body></html>".
func Render(w io.Writer, n *Node) error {
	if x, ok := w.(writer); ok {
		return render(x, n, &renderOpts{})
	}
	buf := bufio.NewWriter(w)
	if err := render(buf, n, &renderOpts{}); err != nil {
		return err
	}
	return buf.Flush()
}

// plaintextAbort is returned from render1 when a <plaintext> element
// has been rendered. No more end tags should be rendered after that.
var plaintextAbort = errors.New("html: internal error (plaintext abort)")

func render(w writer, n *Node, o *renderOpts) error {
	err := render1(w, n, o)
	if err == plaintextAbort {
		err = nil
	}
	return err
}

func render1(w writer, n *Node, o *renderOpts) error {
	// Render non-element nodes; these are the easy cases.
	switch n.Type {
	case ErrorNode:
		return errors.New("html: cannot render an ErrorNode node")
	case TextNode:
		// MOD(pgaskin): Escape NBSPs with &#160;.
		if o.polyglot {
			nbsp := rune('\u00a0') // note: the length is 2 (UTF-8 encoding is C2 A0)
			i := strings.IndexRune(n.Data, nbsp)
			if i != -1 {
				c := utf8.RuneLen(nbsp)
				s := n.Data
				for i != -1 {
					if err := escape(w, s[:i]); err != nil { // pass everything else to the normal escaper
						return err
					}
					if _, err := w.WriteString("&#160;"); err != nil {
						return err
					}
					s = s[i+c:]                    // skip the nbsp (note that a nbsp is 2 bytes long in UTF-8)
					i = strings.IndexRune(s, nbsp) // find the next one
				}
				return escape(w, s) // pass on the rest
			}
		}
		// END MOD
		return escape(w, n.Data)
	case DocumentNode:
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			if err := render1(w, c, o); err != nil {
				return err
			}
		}
		return nil
	case ElementNode:
		// No-op.
	case CommentNode:
		// MOD(pgaskin): Preserve XML declarations. INSECURE against xml
		//     injection. Note: The behaviour of treating XML declarations as
		//     comments is in the tokenizer and standardized in section 12.2.2
		//     (unexpected-question-mark-instead-of-tag-name).
		if o.xmlDecl && strings.HasPrefix(n.Data, "?xml ") && strings.HasSuffix(n.Data, "?") {
			if err := w.WriteByte('<'); err != nil {
				return err
			}
			if _, err := w.WriteString(n.Data); err != nil {
				return err
			}
			if err := w.WriteByte('>'); err != nil {
				return err
			}
			return nil
		}
		// END MOD
		if _, err := w.WriteString("<!--"); err != nil {
			return err
		}
		if _, err := w.WriteString(n.Data); err != nil {
			return err
		}
		if _, err := w.WriteString("-->"); err != nil {
			return err
		}
		return nil
	case DoctypeNode:
		if _, err := w.WriteString("<!DOCTYPE "); err != nil {
			return err
		}
		if _, err := w.WriteString(n.Data); err != nil {
			return err
		}
		if n.Attr != nil {
			var p, s string
			for _, a := range n.Attr {
				switch a.Key {
				case "public":
					p = a.Val
				case "system":
					s = a.Val
				}
			}
			if p != "" {
				if _, err := w.WriteString(" PUBLIC "); err != nil {
					return err
				}
				if err := writeQuoted(w, p); err != nil {
					return err
				}
				if s != "" {
					if err := w.WriteByte(' '); err != nil {
						return err
					}
					if err := writeQuoted(w, s); err != nil {
						return err
					}
				}
			} else if s != "" {
				if _, err := w.WriteString(" SYSTEM "); err != nil {
					return err
				}
				if err := writeQuoted(w, s); err != nil {
					return err
				}
			}
		}
		return w.WriteByte('>')
	case RawNode:
		_, err := w.WriteString(n.Data)
		return err
	default:
		return errors.New("html: unknown node type")
	}

	// Render the <xxx> opening tag.
	if err := w.WriteByte('<'); err != nil {
		return err
	}
	if _, err := w.WriteString(n.Data); err != nil {
		return err
	}
	// MOD(pgaskin): Add html, svg, math xmlns and script, style type
	// ensureAttr ensures an attribute is set on the current element, and
	// returns a function to call to remove any attributes added temporarily.
	ensureAttr := func(ns, key, defValue string) func() {
		for _, a := range n.Attr {
			if (strings.EqualFold(a.Namespace, ns) && strings.EqualFold(a.Key, key)) || strings.EqualFold(a.Namespace, key) {
				return func() {}
			}
		}
		n.Attr = append(n.Attr, Attribute{
			Namespace: ns,
			Key:       key,
			Val:       defValue,
		})
		return func() { n.Attr = n.Attr[:len(n.Attr)-1] }
	}
	if o.polyglot {
		switch n.DataAtom {
		case a.Html:
			defer ensureAttr("", "xmlns", "http://www.w3.org/1999/xhtml")()
		case a.Svg:
			defer ensureAttr("", "xmlns", "http://www.w3.org/2000/svg")()
			defer ensureAttr("xmlns", "xlink", "http://www.w3.org/1999/xlink")()
		case a.Math:
			defer ensureAttr("", "xmlns", "http://www.w3.org/1998/Math/MathML")()
		case a.Script:
			defer ensureAttr("", "type", "text/javascript")()
		case a.Style:
			defer ensureAttr("", "type", "text/css")()
		}
	}
	// END MOD
	for _, a := range n.Attr {
		if err := w.WriteByte(' '); err != nil {
			return err
		}
		if a.Namespace != "" {
			if _, err := w.WriteString(a.Namespace); err != nil {
				return err
			}
			if err := w.WriteByte(':'); err != nil {
				return err
			}
		}
		if _, err := w.WriteString(a.Key); err != nil {
			return err
		}
		if _, err := w.WriteString(`="`); err != nil {
			return err
		}
		if err := escape(w, a.Val); err != nil {
			return err
		}
		if err := w.WriteByte('"'); err != nil {
			return err
		}
	}
	if voidElements[n.Data] {
		if n.FirstChild != nil {
			return fmt.Errorf("html: void element <%s> has child nodes", n.Data)
		}
		_, err := w.WriteString("/>")
		return err
	}
	if err := w.WriteByte('>'); err != nil {
		return err
	}

	// Add initial newline where there is danger of a newline beging ignored.
	if c := n.FirstChild; c != nil && c.Type == TextNode && strings.HasPrefix(c.Data, "\n") {
		switch n.Data {
		case "pre", "listing", "textarea":
			if err := w.WriteByte('\n'); err != nil {
				return err
			}
		}
	}

	// Render any child nodes.
	switch n.Data {
	case "iframe", "noembed", "noframes", "noscript", "plaintext", "script", "style", "xmp":
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			if c.Type == TextNode {
				if _, err := w.WriteString(c.Data); err != nil {
					return err
				}
			} else {
				if err := render1(w, c, o); err != nil {
					return err
				}
			}
		}
		if n.Data == "plaintext" {
			// Don't render anything else. <plaintext> must be the
			// last element in the file, with no closing tag.
			return plaintextAbort
		}
	default:
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			if err := render1(w, c, o); err != nil {
				return err
			}
		}
	}

	// Render the </xxx> closing tag.
	if _, err := w.WriteString("</"); err != nil {
		return err
	}
	if _, err := w.WriteString(n.Data); err != nil {
		return err
	}
	return w.WriteByte('>')
}

// writeQuoted writes s to w surrounded by quotes. Normally it will use double
// quotes, but if s contains a double quote, it will use single quotes.
// It is used for writing the identifiers in a doctype declaration.
// In valid HTML, they can't contain both types of quotes.
func writeQuoted(w writer, s string) error {
	var q byte = '"'
	if strings.Contains(s, `"`) {
		q = '\''
	}
	if err := w.WriteByte(q); err != nil {
		return err
	}
	if _, err := w.WriteString(s); err != nil {
		return err
	}
	if err := w.WriteByte(q); err != nil {
		return err
	}
	return nil
}

// Section 12.1.2, "Elements", gives this list of void elements. Void elements
// are those that can't have any contents.
var voidElements = map[string]bool{
	"area":   true,
	"base":   true,
	"br":     true,
	"col":    true,
	"embed":  true,
	"hr":     true,
	"img":    true,
	"input":  true,
	"keygen": true, // "keygen" has been removed from the spec, but are kept here for backwards compatibility.
	"link":   true,
	"meta":   true,
	"param":  true,
	"source": true,
	"track":  true,
	"wbr":    true,
}
