// Command kobotest tests kepub span logic (only, not divs or other kepub stuff,
// which is pretty straightforward anyways) against other kepubs. It reads the
// HTML from stdin, removes spans, re-adds them with kepubify, and checks the
// output.
package main

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"reflect"
	"strings"
	"unsafe"

	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"

	"github.com/pgaskin/kepubify/v3/kepub"
	"github.com/sergi/go-diff/diffmatchpatch"
)

func main() {
	doc, err := html.ParseWithOptions(os.Stdin, html.ParseOptionIgnoreBOM(true), html.ParseOptionEnableScripting(true), html.ParseOptionLenientSelfClosing(true))
	if err != nil {
		panic(err)
	}

	koboTree := bytes.NewBuffer(nil)
	if err := mkTree(koboTree, doc); err != nil {
		panic(err)
	}

	fmt.Print("\n\n=== ORIGINAL ===\n\n")
	if err := html.Render(os.Stdout, doc); err != nil {
		panic(err)
	}

	removeSpans(doc)

	fmt.Print("\n\n=== SPANS REMOVED ===\n\n")
	if err := html.Render(os.Stdout, doc); err != nil {
		panic(err)
	}

	addSpans(doc)

	fmt.Print("\n\n=== SPANS ADDED ===\n\n")
	if err := html.Render(os.Stdout, doc); err != nil {
		panic(err)
	}

	kepubifyTree := bytes.NewBuffer(nil)
	if err := mkTree(kepubifyTree, doc); err != nil {
		panic(err)
	}

	koboTreeStr := koboTree.String()
	kepubifyTreeStr := kepubifyTree.String()

	fmt.Print("\n\n=== RESULT (red=incorrect green=correct) ===\n\n")

	if kepubifyTreeStr != koboTreeStr {
		dmp := diffmatchpatch.New()
		fmt.Println(dmp.DiffPrettyText(dmp.DiffMain(kepubifyTreeStr, koboTreeStr, false)))
		os.Exit(1)
		return
	}

	fmt.Println("All spans match.")
	os.Exit(0)
}

var addSpans = (*(*func(*html.Node))(unsafe.Pointer(reflect.ValueOf(kepub.NewConverter()).Elem().FieldByName("addSpans").UnsafeAddr())))

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

func mkTree(w io.Writer, node *html.Node) error {
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
			if _, err := fmt.Fprintf(w, "%s- TextNode: %#v\n", indent, cur.Data); err != nil {
				return err
			}
			continue
		case html.ElementNode:
			desc := cur.Data
			for _, attr := range cur.Attr {
				if attr.Key == "class" {
					desc += "." + strings.Join(strings.Fields(attr.Val), ".")
					break
				}
			}
			for _, attr := range cur.Attr {
				if attr.Key == "id" {
					desc += "#" + strings.TrimSpace(attr.Val)
					break
				}
			}
			for _, attr := range cur.Attr {
				if attr.Key != "class" && attr.Key != "id" && !strings.HasPrefix(attr.Key, "xmlns") {
					desc += fmt.Sprintf("[%s=%#v]", attr.Key, attr.Val)
					break
				}
			}
			if _, err := fmt.Fprintf(w, "%s- ElementNode: %s\n", indent, desc); err != nil {
				return err
			}
		case html.DocumentNode:
			if _, err := fmt.Fprintf(w, "%sDocumentNode:\n", indent); err != nil {
				return err
			}
		}

		for c := cur.LastChild; c != nil; c = c.PrevSibling {
			stack = append(stack, c)
			lvls = append(lvls, lvl+1)
		}
	}

	return nil
}
