package kepub

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

func TestConverter(t *testing.T) {
	// tests the overall conversion logic, not the actual transformations

	td, err := ioutil.TempDir("", "kepubify-test")
	if err != nil {
		panic(err)
	}
	defer os.RemoveAll(td)

	t.Run("TestConvert", func(t *testing.T) {
		converterTestCase{
			In:   "test.epub",
			Opts: nil,
			Check: map[string]func(dir string) error{
				"xhtml file written correctly": func(dir string) error {
					if !strings.HasSuffix(strings.TrimSpace(readFile(filepath.Join(dir, "OEBPS", "text1.xhtml"))), "</html>") {
						return fmt.Errorf("couldn't find closing tag in xhtml document")
					}
					return nil
				},
				"xhtml file was transformed": func(dir string) error {
					if !strings.Contains(readFile(filepath.Join(dir, "OEBPS", "text1.xhtml")), "koboSpan") {
						return fmt.Errorf("couldn't find koboSpan in xhtml document")
					}
					return nil
				},
			},
		}.Run(t)
	})

	t.Run("TestConversionOptionSmartyPants", func(t *testing.T) {
		// note: this doesn't check the actual result, just that something happened
		converterTestCase{
			In:   "test.epub",
			Opts: []ConverterOption{ConverterOptionSmartypants()},
			Check: map[string]func(dir string) error{
				"smartypants applied": func(dir string) error {
					if !strings.Contains(readFile(filepath.Join(dir, "OEBPS", "text1.xhtml")), "“") {
						return fmt.Errorf("couldn't find smart quote in xhtml document")
					}
					return nil
				},
			},
		}.Run(t)
	})

	t.Run("TestConversionOptionFindReplace", func(t *testing.T) {
		converterTestCase{
			In:   "test.epub",
			Opts: []ConverterOption{ConverterOptionFindReplace("</html>", "</whatever>")},
			Check: map[string]func(dir string) error{
				"find replace done": func(dir string) error {
					ft := readFile(filepath.Join(dir, "OEBPS", "text1.xhtml"))
					if strings.Contains(ft, "</html>") {
						return fmt.Errorf("found original text in xhtml document")
					}
					if !strings.Contains(ft, "</whatever>") {
						return fmt.Errorf("couldn't find replacement in xhtml document")
					}
					return nil
				},
			},
		}.Run(t)
	})

	t.Run("TestConversionOptionAddCSS", func(t *testing.T) {
		// note: this doesn't check the actual result, just that something happened
		converterTestCase{
			In: "test.epub",
			Opts: []ConverterOption{
				ConverterOptionAddCSS("test{}"),
				ConverterOptionHyphenate(true), // TODO: maybe check this specifically?
				ConverterOptionFullScreenFixes(),
			},
			Check: map[string]func(dir string) error{
				"style added": func(dir string) error {
					if !strings.Contains(readFile(filepath.Join(dir, "OEBPS", "text1.xhtml")), "<style type=\"text/css\" class=\"kepubify") {
						return fmt.Errorf("couldn't find inline kepubify style in xhtml document")
					}
					return nil
				},
			},
		}.Run(t)
	})
}

type converterTestCase struct {
	In    string
	Opts  []ConverterOption
	Check map[string]func(dir string) error
}

func (tc converterTestCase) Run(t *testing.T) {
	td, err := ioutil.TempDir("", "kepubify-test")
	if err != nil {
		panic(err)
	}
	defer os.RemoveAll(td)

	var c *Converter
	if len(tc.Opts) == 0 {
		c = NewConverter()
	} else {
		c = NewConverterWithOptions(tc.Opts...)
	}

	dir := filepath.Join(td, "epub")
	if err := UnpackEPUB("test.epub", dir); err != nil {
		t.Fatalf("error unpacking epub (did the PackUnpack tests fail?): %v", err)
	}

	if err := c.ConvertEPUB("test.epub", filepath.Join(td, "out.kepub.epub")); err != nil {
		t.Fatalf("error running conversion directly from epub: %v", err)
	}

	if err := c.Convert(dir); err != nil {
		t.Fatalf("error running conversion: %v", err)
	}

	var w []string
	for ww := range tc.Check {
		w = append(w, ww)
	}

	sort.Strings(w)

	for _, ww := range w {
		t.Logf("case %#v", ww)
		if err := tc.Check[ww](dir); err != nil {
			t.Errorf("%v", err)
		}
	}
}

func readFile(fn string) string {
	buf, err := ioutil.ReadFile(fn)
	if err != nil {
		panic(err)
	}
	return string(buf)
}
