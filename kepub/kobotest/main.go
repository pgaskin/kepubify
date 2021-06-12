// Command kobotest tests kepub span logic (only, not divs or other kepub stuff,
// which is pretty straightforward anyways) against other kepubs. It reads the
// HTML from stdin, removes spans, re-adds them with kepubify, and checks the
// output.
package main

import (
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/hexops/gotextdiff/myers"
	"github.com/hexops/gotextdiff/span"

	"github.com/pgaskin/kepubify/_/html/golang.org/x/net/html"
	"github.com/pgaskin/kepubify/_/html/golang.org/x/net/html/atom"

	//go:linkname transformContentKoboSpans github.com/pgaskin/kepubify/v4/kepub.transformContentKoboSpans

	_ "unsafe"

	_ "github.com/pgaskin/kepubify/v4/kepub"
)

func transformContentKoboSpans(*html.Node)

func main() {
	doc, err := html.ParseWithOptions(os.Stdin, html.ParseOptionIgnoreBOM(true), html.ParseOptionEnableScripting(true), html.ParseOptionLenientSelfClosing(true))
	if err != nil {
		panic(err)
	}

	koboTree := mkTree(doc)

	fmt.Print("\n\n=== ORIGINAL ===\n\n")
	if err := html.Render(os.Stdout, doc); err != nil {
		panic(err)
	}

	removeSpans(doc)

	fmt.Print("\n\n=== SPANS REMOVED ===\n\n")
	if err := html.Render(os.Stdout, doc); err != nil {
		panic(err)
	}

	transformContentKoboSpans(doc)

	fmt.Print("\n\n=== SPANS ADDED ===\n\n")
	if err := html.Render(os.Stdout, doc); err != nil {
		panic(err)
	}

	kepubifyTree := mkTree(doc)

	fmt.Print("\n\n=== RESULT (blue=kepubify, yellow=kobo) ===\n\n")

	if a, b := kepubifyTree, koboTree; a != b {
		txt, prev := span.NewContentConverter("", []byte(a)), 0
		for _, edit := range myers.ComputeEdits(span.URI(""), a, b) {
			span, _ := edit.Span.WithOffset(txt)
			start, end := span.Start().Offset(), span.End().Offset()
			if start > prev {
				io.WriteString(os.Stdout, a[prev:start])
				prev = start
			}
			if end > start {
				io.WriteString(os.Stdout, "\x1b[34m") // blue
				io.WriteString(os.Stdout, a[start:end])
				io.WriteString(os.Stdout, "\x1b[0m")
			}
			if edit.NewText != "" {
				io.WriteString(os.Stdout, "\x1b[33m") // yellow
				io.WriteString(os.Stdout, edit.NewText)
				io.WriteString(os.Stdout, "\x1b[0m")
			}
			prev = end
		}
		if prev < len(a) {
			io.WriteString(os.Stdout, a[prev:])
		}

		lines := strings.SplitAfter(a, "\n")
		if lines[len(lines)-1] == "" {
			lines = lines[:len(lines)-1]
		}

		os.Exit(1)
		return
	}

	fmt.Println("All spans match.")
	os.Exit(0)
}

func removeSpans(node *html.Node) {
	var stack []*html.Node
	var cur *html.Node

	stack = append(stack, node)

	for len(stack) != 0 {
		stack, cur = stack[:len(stack)-1], stack[len(stack)-1]

		for possibleSpan := cur.FirstChild; possibleSpan != nil; possibleSpan = possibleSpan.NextSibling {
			var isKoboSpan bool
			if possibleSpan.Type == html.ElementNode && possibleSpan.DataAtom == atom.Span {
				for _, attr := range possibleSpan.Attr {
					if attr.Key == "id" && strings.HasPrefix(attr.Val, "kobo.") {
						isKoboSpan = true
						break
					}
				}
			}

			if isKoboSpan {
				for spanChild := possibleSpan.FirstChild; spanChild != nil; spanChild = spanChild.NextSibling {
					spanChild.Parent = possibleSpan.Parent
				}

				if possibleSpan.Parent.FirstChild == possibleSpan {
					possibleSpan.Parent.FirstChild = possibleSpan.FirstChild
				}

				if possibleSpan.PrevSibling != nil {
					possibleSpan.PrevSibling.NextSibling = possibleSpan.FirstChild
				}

				if possibleSpan.NextSibling != nil {
					possibleSpan.NextSibling.PrevSibling = possibleSpan.LastChild
				}

				if possibleSpan.Parent.LastChild == possibleSpan {
					possibleSpan.Parent.LastChild = possibleSpan.LastChild
				}

				if possibleSpan.FirstChild != nil {
					possibleSpan.FirstChild.PrevSibling = possibleSpan.PrevSibling
				}

				if possibleSpan.LastChild != nil {
					possibleSpan.LastChild.NextSibling = possibleSpan.NextSibling
				}

				continue
			}

			stack = append(stack, possibleSpan)
		}
	}
}

func mkTree(node *html.Node) string {
	var b strings.Builder

	var stack []*html.Node
	var cur *html.Node

	var lvls []int
	var lvl int

	stack = append(stack, node)
	lvls = append(lvls, 0)

	for len(stack) != 0 {
		stack, cur = stack[:len(stack)-1], stack[len(stack)-1]
		lvls, lvl = lvls[:len(lvls)-1], lvls[len(lvls)-1]
		indent := strings.Repeat("  ", lvl)

		switch cur.Type {
		case html.TextNode:
			if cur.PrevSibling != nil && cur.PrevSibling.Type == html.ElementNode {
				b.WriteByte('\n')
			}
			b.WriteString(indent)
			b.WriteString("\x1b[2mTextNode:  » \x1b[22m")
			q := strconv.Quote(cur.Data)
			if q[0] == '"' && q[len(q)-1] == '"' {
				if t := q[1 : len(q)-1]; t != "" {
					var unquoted bool
					if r, _ := utf8.DecodeLastRuneInString(t); !unicode.IsSpace(r) {
						if r, _ := utf8.DecodeLastRuneInString(t); !unicode.IsSpace(r) {
							b.WriteString(t)
							unquoted = true
						}
					}
					if !unquoted {
						b.WriteString("\x1b[2m\"\x1b[22m")
						b.WriteString(t)
						b.WriteString("\x1b[2m\"\x1b[22m")
					}
				}
			}
			b.WriteByte('\n')
			if cur.NextSibling != nil && cur.NextSibling.Type == html.ElementNode {
				b.WriteByte('\n')
			}
			continue
		case html.ElementNode:
			b.WriteString(indent)
			b.WriteString("\x1b[2mElementNode: \x1b[22m")
			b.WriteString(cur.Data)
			for _, attr := range cur.Attr {
				if attr.Key == "class" {
					b.WriteByte('.')
					b.WriteString(strings.Join(strings.Fields(attr.Val), "."))
					break
				}
			}
			for _, attr := range cur.Attr {
				if attr.Key == "id" {
					b.WriteByte('#')
					b.WriteString(strings.TrimSpace(attr.Val))
					break
				}
			}
			for _, attr := range cur.Attr {
				if attr.Key != "class" && attr.Key != "id" && !strings.HasPrefix(attr.Key, "xmlns") {
					b.WriteByte('[')
					b.WriteString(attr.Key)
					b.WriteByte('=')
					b.WriteString(attr.Val)
					b.WriteByte(']')
					break
				}
			}
			b.WriteByte('\n')
			if cur.PrevSibling != nil && (cur.PrevSibling.LastChild != nil && cur.PrevSibling.LastChild.Type == html.TextNode) {
				b.WriteByte('\n')
			}
		case html.DocumentNode:
			b.WriteString(indent)
			b.WriteString("\x1b[2mDocumentNode:\x1b[22m\n")
		}

		for c := cur.LastChild; c != nil; c = c.PrevSibling {
			stack = append(stack, c)
			lvls = append(lvls, lvl+1)
		}
	}

	return b.String()
}
