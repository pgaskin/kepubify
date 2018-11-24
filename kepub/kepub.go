package kepub

import (
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/beevik/etree"
	zglob "github.com/mattn/go-zglob"
	"golang.org/x/net/html"
)

const HyphenateCSS = `* {
	-webkit-hyphens: auto;
	-moz-hyphens: auto;
	hyphens: auto;

	-webkit-hyphenate-after: 3;
	-webkit-hyphenate-before: 3;
	-webkit-hyphenate-lines: 2;
	hyphenate-after: 3;
	hyphenate-before: 3;
	hyphenate-lines: 2;
}

h1, h2, h3, h4, h5, h6, td {
	-moz-hyphens: none !important;
	-webkit-hyphens: none !important;
	hyphens: none !important;
}`

const NoHyphenateCSS = `* {
	-moz-hyphens: none !important;
	-webkit-hyphens: none !important;
	hyphens: none !important;
}`

const FullScreenFixesCSS = `body {
	margin: 0 !important;
	padding: 0 !important;
}

body>div {
	padding-left: 0.2em !important;
	padding-right: 0.2em !important;
}`

type Converter struct {
	PostDoc         func(doc *goquery.Document) error // post-processing the parsed document
	PostHTML        func(h string) (string, error)    // post-processing the resulting html string (after PostDoc)
	ExtraCSS        string                            // extra css to add
	NoHyphenate     bool                              // force disable hyphenation
	Hyphenate       bool                              // force enable hyphenation
	InlineStyles    bool                              // inline all stylesheets
	FullScreenFixes bool                              // add css to fix FullScreenReading (probably not needed since 4.11.11911)
	FindReplace     map[string]string                 // find and replace on the html of all content files
	Verbose         bool                              // verbose output to stdout during conversion
}

func (c *Converter) Convert(src, dest string) error {
	td, err := ioutil.TempDir("", "kepubify")
	if err != nil {
		return fmt.Errorf("could not create temp dir: %s", err)
	}
	defer os.RemoveAll(td)

	if err := UnpackEPUB(src, td, true); err != nil {
		return err
	}

	var contentFiles []string
	for _, glob := range []string{"**/*.html", "**/*.xhtml", "**/*.htm"} {
		if files, err := zglob.Glob(filepath.Join(td, glob)); err != nil {
			return fmt.Errorf("could not search for content files: %v", err)
		} else {
			contentFiles = append(contentFiles, files...)
		}
	}

	runtime.GOMAXPROCS(runtime.NumCPU() + 1)
	var wg sync.WaitGroup
	err = nil
	var errOnce sync.Once // this is a slightly less efficient method for handling errors, but it is way cleaner
	errfn := func(ferr error) {
		errOnce.Do(func() {
			err = ferr
		})
	}
	for _, f := range contentFiles {
		wg.Add(1)
		go func(f string) {
			defer wg.Done()
			if buf, err := ioutil.ReadFile(f); err != nil {
				errfn(err)
				return
			} else if html, err := c.ProcessHTML(string(buf), f); err != nil {
				errfn(err)
				return
			} else if err := ioutil.WriteFile(f, []byte(html), 0644); err != nil {
				errfn(err)
				return
			}
			time.Sleep(time.Millisecond * 5)
		}(f)
	}
	if wg.Wait(); err != nil {
		return err
	}

	rsk, err := os.Open(filepath.Join(td, "META-INF", "container.xml"))
	if err != nil {
		return fmt.Errorf("error opening container.xml: %v", err)
	}
	defer rsk.Close()

	container := etree.NewDocument()
	if _, err = container.ReadFrom(rsk); err != nil {
		return fmt.Errorf("error parsing container.xml: %v", err)
	}

	var rootfile string
	for _, e := range container.FindElements("//rootfiles/rootfile[@full-path]") {
		rootfile = e.SelectAttrValue("full-path", "")
	}
	if rootfile == "" {
		return fmt.Errorf("error parsing container.xml")
	}

	if buf, err := ioutil.ReadFile(filepath.Join(td, rootfile)); err != nil {
		return fmt.Errorf("error parsing content.opf: %v", err)
	} else if opf, err := c.ProcessOPF(string(buf)); err != nil {
		return fmt.Errorf("error cleaning content.opf: %v", err)
	} else if err := ioutil.WriteFile(filepath.Join(td, rootfile), []byte(opf), 0644); err != nil {
		return fmt.Errorf("error writing new content.opf: %v", err)
	}

	if err := c.CleanFiles(td); err != nil {
		return fmt.Errorf("error cleaning extra files: %v", err)
	}

	if err := PackEPUB(td, dest, true); err != nil {
		return fmt.Errorf("error creating new epub: %v", err)
	}

	return nil
}

// ProcessHTML processes a html file. filename is the full path to the file and is required if inlining styles.
func (c *Converter) ProcessHTML(html, filename string) (string, error) {
	html = fixInvalidSelfClosingTags(html)

	var doc *goquery.Document
	var err error
	if doc, err = goquery.NewDocumentFromReader(strings.NewReader(html)); err != nil {
		return html, err
	} else {
		for _, fn := range []func(*goquery.Document) error{addDivs, addSpans, addKoboStyles, cleanHTML} {
			if err = fn(doc); err != nil {
				return html, err
			}
		}

		if c.InlineStyles {
			if filename == "" {
				return html, errors.New("filename required for inlining styles")
			}
			doc.Find("link[rel='stylesheet'][href$='.css']").Each(func(_ int, s *goquery.Selection) {
				if buf, err := ioutil.ReadFile(filepath.Join(filepath.Dir(filename), s.AttrOr("href", ""))); err == nil {
					s.ReplaceWithNodes(styleNode(string(buf), "kepubify-inlinestyle"))
				}
			})
		}

		if c.FullScreenFixes {
			doc.Find("body").AppendNodes(styleNode(FullScreenFixesCSS, "kepubify-fullscreenfixes"))
		}

		if c.Hyphenate {
			doc.Find("body").AppendNodes(styleNode(HyphenateCSS, "kepubify-hyphenate"))
		}

		if c.NoHyphenate {
			doc.Find("body").AppendNodes(styleNode(NoHyphenateCSS, "kepubify-nohyphenate"))
		}

		if c.ExtraCSS != "" {
			doc.Find("body").AppendNodes(styleNode(c.ExtraCSS, "kepubify-extracss"))
		}

		if c.PostDoc != nil {
			if err = c.PostDoc(doc); err != nil {
				return html, err
			}
		}

		if html, err = doc.Html(); err != nil {
			return html, err
		}
	}

	html = smartenPunctuation(html)
	html = strings.Replace(html, "�", "", -1)                                                                                  // Remove unicode replacement chars
	html = strings.Replace(html, `<!-- ?xml version="1.0" encoding="utf-8"? -->`, `<?xml version="1.0" encoding="utf-8"?>`, 1) // Fix commented xml tag
	html = strings.Replace(html, `<!--?xml version="1.0" encoding="utf-8"?-->`, `<?xml version="1.0" encoding="utf-8"?>`, 1)   // Fix commented xml tag
	html = strings.Replace(html, "\u00a0", "&#160;", -1)                                                                       // Fix nbsps removed

	if c.FindReplace != nil {
		for find, replace := range c.FindReplace {
			html = strings.Replace(html, find, replace, -1)
		}
	}

	if c.PostHTML != nil {
		if html, err = c.PostHTML(html); err != nil {
			return html, err
		}
	}

	return html, nil
}

func (c *Converter) ProcessOPF(opf string) (string, error) {
	doc := etree.NewDocument()
	err := doc.ReadFromString(opf)
	if err != nil {
		return "", err
	}

	// Add properties="cover-image" to cover file item entry to enable the kobo
	// to find the cover image.
	for _, meta := range doc.FindElements("//meta[@name='cover']") {
		coverID := meta.SelectAttrValue("content", "")
		if coverID == "" {
			coverID = "cover"
		}
		for _, item := range doc.FindElements("//[@id='" + coverID + "']") {
			item.CreateAttr("properties", "cover-image")
		}
	}

	for _, meta := range doc.FindElements("//meta[@name='calibre:timestamp']") {
		meta.Parent().RemoveChild(meta)
	}

	for _, contributor := range doc.FindElements("//dc:contributor[@role='bkp']") {
		contributor.Parent().RemoveChild(contributor)
	}

	doc.Indent(4)
	if opf, err = doc.WriteToString(); err != nil {
		return opf, err
	}

	return opf, nil
}

func (c *Converter) CleanFiles(epubDir string) error {
	for _, file := range []string{
		"META-INF/calibre_bookmarks.txt",
		"META-INF/iTunesMetadata.plist",
		"META-INF/iTunesArtwork.plist",
		"META-INF/.DS_STORE",
		"META-INF/thumbs.db",
		".DS_STORE",
		"thumbs.db",
		"iTunesMetadata.plist",
		"iTunesArtwork.plist",
	} {
		os.Remove(filepath.Join(epubDir, file))
	}
	return nil
}

func styleNode(cssstr, class string) *html.Node {
	style := &html.Node{
		Type: html.ElementNode,
		Data: "style",
		Attr: []html.Attribute{{
			Key: "type",
			Val: "text/css",
		}, {
			Key: "class",
			Val: class,
		}},
	}

	style.AppendChild(&html.Node{
		Type: html.TextNode,
		Data: cssstr,
	})
	return style
}
