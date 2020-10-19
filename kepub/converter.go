// Package kepub provides functions for converting EPUBs to KEPUBs.
package kepub

import (
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"golang.org/x/net/html"
	"golang.org/x/sync/errgroup"
)

// Verbose controls whether this package writes verbose output (it is silent
// otherwise).
var Verbose = false // TODO: implement

// Converter converts EPUB2/EPUB3 books to KEPUB.
type Converter struct {
	// extra css
	extraCSS      []string
	extraCSSClass []string
	// smart punctuation
	smartypants bool
	// find/replace in raw html output (note: inefficient, but more efficient
	// than working with strings)
	find    [][]byte
	replace [][]byte

	// addSpans is for use by the kobotest command and should note be used elsewhere.
	addSpans func(*html.Node)
}

// NewConverter creates a new Converter. By default, no options are applied.
func NewConverter() *Converter {
	return NewConverterWithOptions()
}

// NewConverterWithOptions is like NewConverter, with options.
func NewConverterWithOptions(opts ...ConverterOption) *Converter {
	c := new(Converter)
	for _, f := range opts {
		f(c)
	}
	c.addSpans = transform2koboSpans
	return c
}

// Convert converts the dir as the root of an EPUB.
func (c *Converter) Convert(dir string) error {
	if _, err := FindOPF(dir); err != nil {
		// sanity check to help guard against mistakes (i.e. we don't want to mess up someone's files if they make a mistake with the path)
		return fmt.Errorf("not an epub: %s", dir)
	}

	if err := c.transformAllContentParallel(dir); err != nil {
		return fmt.Errorf("transform content: %w", err)
	}

	opfPath, err := FindOPF(dir)
	if err != nil {
		return fmt.Errorf("transform opf: find: %w", err)
	}

	opfFile, err := os.OpenFile(opfPath, os.O_RDWR, 0)
	if err != nil {
		return fmt.Errorf("transform opf: open: %w", err)
	}
	defer opfFile.Close()

	if err := c.TransformOPFFile(opfFile); err != nil {
		return fmt.Errorf("transform opf: %w", err)
	}

	if err := c.transformEPUB(dir); err != nil {
		return fmt.Errorf("transform epub: %w", err)
	}

	return nil
}

// ConvertEPUB converts an EPUB file.
func (c *Converter) ConvertEPUB(epub, kepub string) error {
	td, err := ioutil.TempDir("", "kepubify")
	if err != nil {
		return fmt.Errorf("create temp dir: %w", err)
	}
	defer os.RemoveAll(td)

	dir := filepath.Join(td, "unpacked")

	if err := UnpackEPUB(epub, dir); err != nil {
		return fmt.Errorf("unpack epub: %w", err)
	}

	if err := c.Convert(dir); err != nil {
		return err
	}

	if err := PackEPUB(dir, kepub); err != nil {
		return fmt.Errorf("pack kepub: %w", err)
	}

	return nil
}

func (c *Converter) transformAllContentParallel(dir string) error {
	g, ctx := errgroup.WithContext(context.Background())
	contentFiles := make(chan string)

	g.Go(func() error {
		defer close(contentFiles)
		return filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if !info.Mode().IsRegular() {
				return nil
			}
			switch strings.ToLower(filepath.Ext(path)) {
			case ".html", ".xhtml", ".htm", ".xhtm":
				select {
				case contentFiles <- path:
				case <-ctx.Done():
					return ctx.Err()
				}
			}
			return nil
		})
	})

	for i := 0; i < runtime.NumCPU(); i++ {
		g.Go(func() error {
			for fn := range contentFiles {
				if f, err := os.OpenFile(fn, os.O_RDWR, 0); err != nil {
					return fmt.Errorf("open content file %#v: %w", fn, err)
				} else if err := c.TransformContentDocFile(f); err != nil {
					f.Close()
					return fmt.Errorf("transform content file %#v: %w", fn, err)
				} else {
					f.Close()
				}

				select {
				case <-ctx.Done():
					return ctx.Err()
				default:
					continue
				}
			}
			return nil
		})
	}

	return g.Wait()
}

// TransformContentDoc transforms an HTML4/HTML5/XHTML1.1 content document for
// a KEPUB.
func (c *Converter) TransformContentDoc(w io.Writer, r io.Reader) error {
	doc, err := c.transform1(r)
	if err != nil {
		return fmt.Errorf("parse: %w", err)
	}

	if err := c.transform2(doc); err != nil {
		return fmt.Errorf("transform: %w", err)
	}

	if err := c.transform3(w, doc); err != nil {
		return fmt.Errorf("render: %w", err)
	}

	return nil
}

// TransformContentDocFile is like TransformContentDoc, but works inplace.
func (c *Converter) TransformContentDocFile(rws interface {
	io.ReadWriteSeeker
	Truncate(size int64) error
}) error {
	if _, err := rws.Seek(0, io.SeekStart); err != nil {
		return fmt.Errorf("parse: seek to file start: %w", err)
	}

	doc, err := c.transform1(rws)
	if err != nil {
		return fmt.Errorf("parse: %w", err)
	}

	if err := c.transform2(doc); err != nil {
		return fmt.Errorf("transform: %w", err)
	}

	if _, err := rws.Seek(0, io.SeekStart); err != nil {
		return fmt.Errorf("render: seek to file start: %w", err)
	}

	if err := rws.Truncate(0); err != nil {
		return fmt.Errorf("render: truncate: %w", err)
	}

	if err := c.transform3(rws, doc); err != nil {
		return fmt.Errorf("render: %w", err)
	}

	return nil
}

// TransformOPF transforms the OPF document for a KEPUB.
func (c *Converter) TransformOPF(w io.Writer, r io.Reader) error {
	doc, err := c.transformOPF1(r)
	if err != nil {
		return fmt.Errorf("parse: %w", err)
	}

	if err := c.transformOPF2(doc); err != nil {
		return fmt.Errorf("transform: %w", err)
	}

	if err := c.transformOPF3(w, doc); err != nil {
		return fmt.Errorf("render: %w", err)
	}

	return nil
}

// TransformOPFFile is like TransformOPF, but works inplace.
func (c *Converter) TransformOPFFile(rws interface {
	io.ReadWriteSeeker
	Truncate(size int64) error
}) error {
	if _, err := rws.Seek(0, io.SeekStart); err != nil {
		return fmt.Errorf("parse: seek to file start: %w", err)
	}

	doc, err := c.transformOPF1(rws)
	if err != nil {
		return fmt.Errorf("parse: %w", err)
	}

	if err := c.transformOPF2(doc); err != nil {
		return fmt.Errorf("transform: %w", err)
	}

	if _, err := rws.Seek(0, io.SeekStart); err != nil {
		return fmt.Errorf("render: seek to file start: %w", err)
	}

	if err := rws.Truncate(0); err != nil {
		return fmt.Errorf("render: truncate: %w", err)
	}

	if err := c.transformOPF3(rws, doc); err != nil {
		return fmt.Errorf("render: %w", err)
	}

	return nil
}

// --- Options --- //

// ConverterOption configures a Converter.
type ConverterOption func(*Converter)

// ConverterOptionSmartypants enables smart punctuation.
func ConverterOptionSmartypants() ConverterOption {
	return func(c *Converter) {
		c.smartypants = true
	}
}

// ConverterOptionFindReplace replaces a raw string in the transformed HTML (
// note that this reduces efficiency by requiring the HTML to be encoded into
// a temporary buffer before being written).
func ConverterOptionFindReplace(find, replace string) ConverterOption {
	return func(c *Converter) {
		c.find = append(c.find, []byte(find))
		c.replace = append(c.replace, []byte(replace))
	}
}

// ConverterOptionAddCSS adds CSS code to a book.
func ConverterOptionAddCSS(css string) ConverterOption {
	return converterOptionAddCSS("kepubify-extracss", css)
}

// ConverterOptionHyphenate enables or disables hyphenation. If not set, no
// specific state is enforced by kepubify.
func ConverterOptionHyphenate(enabled bool) ConverterOption {
	if enabled {
		return converterOptionAddCSS("kepubify-hyphenate", cssHyphenate)
	}
	return converterOptionAddCSS("kepubify-nohyphenate", cssNoHyphenate)
}

// ConverterOptionFullScreenFixes applies fullscreen fixes for firmwares older
// than 4.19.11911.
func ConverterOptionFullScreenFixes() ConverterOption {
	return converterOptionAddCSS("kepubify-fullscreenfixes", cssFullScreenFixes)
}

func converterOptionAddCSS(class, css string) ConverterOption {
	return func(c *Converter) {
		c.extraCSS = append(c.extraCSS, css)
		c.extraCSSClass = append(c.extraCSSClass, class)
	}
}

const cssHyphenate = `* {
    -webkit-hyphens: auto;
    -moz-hyphens: auto;
    hyphens: auto;

    -webkit-hyphenate-limit-after: 3;
    -webkit-hyphenate-limit-before: 3;
    -webkit-hyphenate-limit-lines: 2;
}

h1, h2, h3, h4, h5, h6, td {
    -moz-hyphens: none !important;
    -webkit-hyphens: none !important;
    hyphens: none !important;
}`

const cssNoHyphenate = `* {
    -moz-hyphens: none !important;
    -webkit-hyphens: none !important;
    hyphens: none !important;
}`

const cssFullScreenFixes = `body {
    margin: 0 !important;
    padding: 0 !important;
}

body>div {
    padding-left: 0.2em !important;
    padding-right: 0.2em !important;
}`
