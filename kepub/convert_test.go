package kepub

import (
	"bytes"
	"context"
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"image"
	"image/png"
	"io"
	"io/fs"
	"io/ioutil"
	"math/rand"
	"sort"
	"strconv"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/pgaskin/kepubify/v4/internal/zip"
)

// The intention of these tests are to provide quick checks for important
// functionality, not to comprehensively test every feature and its
// implementation details (those tests belong in transform_test.go). They mainly
// check for obvious problems rather than correctness. They also ensure the
// converter behaves consistently.

var testEPUB fstest.MapFS

func TestConvert(t *testing.T) {
	ConvertTestCase{
		What:        "empty source",
		EPUB:        fstest.MapFS{},
		ShouldError: true,
	}.Run(t)

	ConvertTestCase{
		What: "invalid container",
		EPUB: overlayMapFS(testEPUB, fstest.MapFS{
			"META-INF/container.xml": &fstest.MapFile{
				Data: []byte(`test`),
				Mode: testEPUB["META-INF/container.xml"].Mode,
			},
		}),
		ShouldError: true,
	}.Run(t)

	ConvertTestCase{
		What: "invalid container version",
		EPUB: overlayMapFS(testEPUB, fstest.MapFS{
			"META-INF/container.xml": &fstest.MapFile{
				Data: []byte(`<?xml version="1.0" encoding="UTF-8"?><container version="1234.5" xmlns="urn:oasis:names:tc:opendocument:xmlns:container"></container>`),
				Mode: testEPUB["META-INF/container.xml"].Mode,
			},
		}),
		ShouldError: true,
	}.Run(t)

	ConvertTestCase{
		What: "no package documents in container",
		EPUB: overlayMapFS(testEPUB, fstest.MapFS{
			"META-INF/container.xml": &fstest.MapFile{
				Data: []byte(`<?xml version="1.0" encoding="UTF-8"?><container version="1.0" xmlns="urn:oasis:names:tc:opendocument:xmlns:container"></container>`),
				Mode: testEPUB["META-INF/container.xml"].Mode,
			},
		}),
		ShouldError: true,
	}.Run(t)

	ConvertTestCase{
		What: "invalid package",
		EPUB: overlayMapFS(testEPUB, fstest.MapFS{
			"OEBPS/content.opf": &fstest.MapFile{
				Data: []byte(`test`),
				Mode: testEPUB["OEBPS/content.opf"].Mode,
			},
		}),
		ShouldError: true,
	}.Run(t)

	ConvertTestCase{
		What:        "simple",
		EPUB:        testEPUB,
		ShouldError: false,

		Options: []ConverterOption{},
		Checks: []ShouldFunc{
			ShouldHaveAllSourceDocumentsWithSaneOPF(0),
			AllDocumentsShould(DocumentProbablyHasSpans, []string{"OEBPS/xhtml/title.xhtml"}),
			ShouldBeUnchanged("OEBPS/cover.png"),
		},
	}.Run(t)

	ConvertTestCase{
		What: "with missing mimetype",
		EPUB: overlayMapFS(testEPUB, fstest.MapFS{
			"mimetype": nil,
		}),
		ShouldError: false,

		Options: []ConverterOption{},
		Checks: []ShouldFunc{
			ShouldHaveAllSourceDocumentsWithSaneOPF(0),
			AllDocumentsShould(DocumentProbablyHasSpans, []string{"OEBPS/xhtml/title.xhtml"}),
			ShouldBeUnchanged("OEBPS/cover.png"),
		},
	}.Run(t)

	ConvertTestCase{
		What: "with incorrect content filename casing",
		EPUB: overlayMapFS(testEPUB, fstest.MapFS{
			"OEBPS/xhtml/ch01.xhtml": nil,
			"OEBPS/xhtml/Ch01.XHTML": testEPUB["OEBPS/xhtml/ch01.xhtml"],
		}),
		ShouldError: false,

		Options: []ConverterOption{},
		Checks: []ShouldFunc{
			ShouldHaveAllSourceDocumentsWithSaneOPF(0),
			ShouldHaveFile("OEBPS/xhtml/Ch01.XHTML").Because("should be lenient during conversion, but still keep the output file as close as possible to the original when invalid"),
			FileShould("OEBPS/xhtml/Ch01.XHTML", DocumentProbablyHasSpans).Because("should be lenient when finding content documents"),
		},
	}.Run(t)

	ConvertTestCase{
		What: "with incorrect extensions and mimetypes",
		EPUB: overlayMapFS(testEPUB, fstest.MapFS{
			"META-INF/container.xml": &fstest.MapFile{
				Data: bytes.ReplaceAll(testEPUB["META-INF/container.xml"].Data, []byte(`content.opf`), []byte(`content.xhtml`)),
				Mode: testEPUB["META-INF/container.xml"].Mode,
			},
			"OEBPS/content.opf": nil,
			"OEBPS/content.xhtml": &fstest.MapFile{
				Data: []byte(strings.NewReplacer(
					`href="xhtml/ch01.xhtml" media-type="application/xhtml+xml"/>`, `href="xhtml/ch01.xhtml" media-type="application/xml"/><!--1-->`,
					`href="xhtml/ch02.xhtml" media-type="application/xhtml+xml"/>`, `href="xhtml/ch02.xhtml" media-type="text/xml"/><!--2-->`,
					`href="xhtml/ch03.xhtml" media-type="application/xhtml+xml"/>`, `href="xhtml/ch03.xhtml" media-type="text/html"/><!--3-->`,
					`href="xhtml/ch04.xhtml" media-type="application/xhtml+xml"/>`, `href="xhtml/ch04.xml" media-type="application/xhtml+xml"/><!--4-->`,
					`href="xhtml/ch05.xhtml" media-type="application/xhtml+xml"/>`, `href="xhtml/ch05.XHTML" media-type="application/xhtml+xml"/><!--5-->`,
					`href="xhtml/ch06.xhtml" media-type="application/xhtml+xml"/>`, `href="xhtml/ch06.invalid" media-type="application/xhtml+xml"/><!--6-->`,
				).Replace(string(testEPUB["OEBPS/content.opf"].Data))),
				Mode: testEPUB["OEBPS/content.opf"].Mode,
			},
			"OEBPS/xhtml/ch04.xhtml":   nil,
			"OEBPS/xhtml/ch04.xml":     testEPUB["OEBPS/xhtml/ch04.xhtml"],
			"OEBPS/xhtml/ch05.xhtml":   nil,
			"OEBPS/xhtml/ch05.XHTML":   testEPUB["OEBPS/xhtml/ch05.xhtml"],
			"OEBPS/xhtml/ch06.xhtml":   nil,
			"OEBPS/xhtml/ch06.invalid": testEPUB["OEBPS/xhtml/ch04.xhtml"],
		}),
		ShouldError: false,

		Options: []ConverterOption{
			ConverterOptionDummyTitlepage(false),
		},
		Checks: []ShouldFunc{
			FileShould("OEBPS/content.xhtml", func(contents string) error {
				for i := 1; i <= 6; i++ {
					if !strings.Contains(contents, `<!--`+strconv.Itoa(i)+`-->`) {
						panic("opf patch failed")
					}
				}
				return nil
			}),
			ShouldHaveAllSourceDocumentsWithSaneOPF(0),
			FileShould("OEBPS/xhtml/ch01.xhtml", DocumentProbablyHasSpans).Because("should handle incorrect mime"),
			FileShould("OEBPS/xhtml/ch02.xhtml", DocumentProbablyHasSpans).Because("should handle incorrect mime"),
			FileShould("OEBPS/xhtml/ch03.xhtml", DocumentProbablyHasSpans).Because("should handle incorrect mime"),
			FileShould("OEBPS/xhtml/ch04.xml", DocumentProbablyHasSpans).Because("should handle incorrect extension"),
			FileShould("OEBPS/xhtml/ch05.XHTML", DocumentProbablyHasSpans).Because("should handle incorrect extension"),
			FileShould("OEBPS/xhtml/ch06.invalid", DocumentProbablyHasSpans).Because("should handle incorrect extension"),
			FileShould("OEBPS/content.xhtml", func(contents string) error {
				if strings.Contains(contents, `koboSpan`) {
					return fmt.Errorf("should not contain spans")
				}
				return nil
			}).Because("should not blindly change all files with an extension of .xhtml"),
		},
	}.Run(t)

	ConvertTestCase{
		What:        "with cover fix forced",
		EPUB:        testEPUB,
		ShouldError: false,

		Options: []ConverterOption{
			ConverterOptionDummyTitlepage(true),
		},
		Checks: []ShouldFunc{
			ShouldHaveAllSourceDocumentsWithSaneOPF(1).Because("should have all source documents, and the new dummy titlepage"),
			ShouldHaveFile("OEBPS/kepubify-titlepage-dummy.xhtml"),
			AllDocumentsShould(DocumentProbablyHasSpans, []string{"OEBPS/xhtml/title.xhtml", "OEBPS/kepubify-titlepage-dummy.xhtml"}),
			ShouldBeUnchanged("OEBPS/cover.png"),
		},
	}.Run(t)

	ConvertTestCase{
		What: "with cover fix detected",
		EPUB: overlayMapFS(testEPUB, fstest.MapFS{
			"OEBPS/content.opf": &fstest.MapFile{
				Data: bytes.ReplaceAll(testEPUB["OEBPS/content.opf"].Data, []byte(`<itemref idref="xhtml_title"/>`), []byte(`<!--removed-->`)),
				Mode: testEPUB["OEBPS/content.opf"].Mode,
			},
		}),
		ShouldError: false,

		Options: []ConverterOption{},
		Checks: []ShouldFunc{
			ShouldHaveAllSourceDocumentsWithSaneOPF(1).Because("should have all source documents, and the new dummy titlepage"),
			FileShould("OEBPS/content.opf", func(contents string) error {
				if !strings.Contains(contents, `<!--removed-->`) {
					panic("opf patch failed")
				}
				return nil
			}),
			ShouldHaveFile("OEBPS/kepubify-titlepage-dummy.xhtml"),
		},
	}.Run(t)

	ConvertTestCase{
		What: "with cover fix detected and disabled",
		EPUB: overlayMapFS(testEPUB, fstest.MapFS{
			"OEBPS/content.opf": &fstest.MapFile{
				Data: bytes.ReplaceAll(testEPUB["OEBPS/content.opf"].Data, []byte(`<itemref idref="xhtml_title"/>`), []byte(`<!--removed-->`)),
				Mode: testEPUB["OEBPS/content.opf"].Mode,
			},
		}),
		ShouldError: false,

		Options: []ConverterOption{
			ConverterOptionDummyTitlepage(false),
		},
		Checks: []ShouldFunc{
			FileShould("OEBPS/content.opf", func(contents string) error {
				if !strings.Contains(contents, `<!--removed-->`) {
					panic("opf patch failed")
				}
				return nil
			}),
			ShouldHaveAllSourceDocumentsWithSaneOPF(0).Because("should have all source documents, but not the new dummy titlepage"),
		},
	}.Run(t)

	ConvertTestCase{
		What: "with replacement",
		EPUB: overlayMapFS(testEPUB, fstest.MapFS{
			"OEBPS/xhtml/ch01.xhtml": &fstest.MapFile{
				Data: []byte(`<!DOCTYPE html><html><head><title>Replaced Chapter</title></head><body><p>Lorem ipsum dolor sit amet.</p></body></html>`),
				Mode: 0644,
			},
		}),
		ShouldError: false,

		Options: []ConverterOption{
			ConverterOptionFindReplace("Lorem", "*****"),
			ConverterOptionFindReplace("*****", "***"),
			ConverterOptionFindReplace("ipsum", "dolor"),
			ConverterOptionFindReplace("dolor", "ipsum"),
			ConverterOptionFindReplace("<i>", "<em>"),
			ConverterOptionFindReplace("</i>", "</em>"),
		},
		Checks: []ShouldFunc{
			ShouldHaveAllSourceDocumentsWithSaneOPF(0),
			AllDocumentsShould(DocumentProbablyHasSpans, []string{"OEBPS/xhtml/title.xhtml"}),
			AllDocumentsShould(func(doc string) error {
				if strings.Contains(doc, "Lorem") {
					return fmt.Errorf("shouldn't contain Lorem")
				}
				if strings.Contains(doc, "dolor") {
					return fmt.Errorf("shouldn't contain Lorem")
				}
				if strings.Contains(doc, "<i>") || strings.Contains(doc, "</i>") {
					return fmt.Errorf("shouldn't contain <i> or </i>")
				}
				return nil
			}, nil),
			FileShould("OEBPS/xhtml/ch01.xhtml", func(doc string) error {
				if !strings.Contains(doc, ">*** ipsum ipsum sit amet.<") {
					return fmt.Errorf("incorrect replacement: %q", doc)
				}
				return nil
			}),
			ShouldBeUnchanged("OEBPS/cover.png"),
		},
	}.Run(t)

	ConvertTestCase{
		What: "with cleanup",
		EPUB: overlayMapFS(testEPUB, fstest.MapFS{
			"META-INF/calibre_bookmarks.txt": &fstest.MapFile{
				Data: []byte("dummy"),
				Mode: 0644,
			},
			"META-INF/not_calibre_bookmarks.txt": &fstest.MapFile{
				Data: []byte("dummy"),
				Mode: 0644,
			},
			"OEBPS/xhtml/ch01.xhtml": &fstest.MapFile{
				Data: []byte(`<!DOCTYPE html><html><head><title>Replaced Chapter</title></head><body><o:p></o:p><p>Test</p></body></html>`),
				Mode: 0644,
			},
		}),
		ShouldError: false,

		Options: []ConverterOption{},
		Checks: []ShouldFunc{
			ShouldHaveAllSourceDocumentsWithSaneOPF(0),
			AllDocumentsShould(DocumentProbablyHasSpans, []string{"OEBPS/xhtml/title.xhtml"}),
			ShouldNotHaveFile("META-INF/calibre_bookmarks.txt"),
			ShouldHaveFile("META-INF/not_calibre_bookmarks.txt"),
			FileShould("OEBPS/xhtml/ch01.xhtml", func(doc string) error {
				if strings.Contains(doc, "o:p") {
					return fmt.Errorf("contains o:p tag")
				}
				return nil
			}),
			ShouldBeUnchanged("OEBPS/cover.png"),
		},
	}.Run(t)

	ConvertTestCase{
		What: "with smart punctuation",
		EPUB: overlayMapFS(testEPUB, fstest.MapFS{
			"OEBPS/xhtml/ch01.xhtml": &fstest.MapFile{
				Data: []byte(`<!DOCTYPE html><html><head><title>Replaced Chapter</title></head><body><p>"asd sdf" 'asd asd' asd-sdf asd - sdf asd--sdf asd -- sdf asd... sdf 1/2 1/4 3/4 (c) (r) (tm)</p></body></html>`),
				Mode: 0644,
			},
		}),
		ShouldError: false,

		Options: []ConverterOption{
			ConverterOptionSmartypants(),
		},
		Checks: []ShouldFunc{
			FileShould("OEBPS/xhtml/ch01.xhtml", DocumentProbablyHasSpans),
			FileShould("OEBPS/xhtml/ch01.xhtml", func(doc string) error {
				for _, x := range []string{
					`“asd sdf”`, `‘asd asd’`, `asd-sdf`, `asd - sdf`, `asd–sdf`,
					`asd – sdf`, `asd…`, `½`, `¼`, `¾`, `©`, `®`, `™`,
				} {
					if !strings.Contains(doc, x) {
						return fmt.Errorf("%q does not contain %q", doc, x)
					}
				}
				return nil
			}),
		},
	}.Run(t)

	ConvertTestCase{
		What:        "with hyphenation enable css",
		EPUB:        testEPUB,
		ShouldError: false,

		Options: []ConverterOption{
			ConverterOptionHyphenate(true),
		},
		Checks: []ShouldFunc{
			ShouldHaveAllSourceDocumentsWithSaneOPF(0),
			AllDocumentsShould(DocumentProbablyHasSpans, []string{"OEBPS/xhtml/title.xhtml"}),
			AllDocumentsShould(func(doc string) error {
				if !strings.Contains(doc, cssHyphenate) {
					return fmt.Errorf("missing css")
				}
				return nil
			}, nil),
		},
	}.Run(t)

	ConvertTestCase{
		What:        "with hyphenation disable css",
		EPUB:        testEPUB,
		ShouldError: false,

		Options: []ConverterOption{
			ConverterOptionHyphenate(false),
		},
		Checks: []ShouldFunc{
			ShouldHaveAllSourceDocumentsWithSaneOPF(0),
			AllDocumentsShould(DocumentProbablyHasSpans, []string{"OEBPS/xhtml/title.xhtml"}),
			AllDocumentsShould(func(doc string) error {
				if !strings.Contains(doc, cssNoHyphenate) {
					return fmt.Errorf("missing css")
				}
				return nil
			}, nil),
		},
	}.Run(t)

	ConvertTestCase{
		What:        "with full-screen fixes css",
		EPUB:        testEPUB,
		ShouldError: false,

		Options: []ConverterOption{
			ConverterOptionFullScreenFixes(),
		},
		Checks: []ShouldFunc{
			ShouldHaveAllSourceDocumentsWithSaneOPF(0),
			AllDocumentsShould(DocumentProbablyHasSpans, []string{"OEBPS/xhtml/title.xhtml"}),
			AllDocumentsShould(func(doc string) error {
				if !strings.Contains(doc, cssFullScreenFixes) {
					return fmt.Errorf("missing css")
				}
				return nil
			}, nil),
		},
	}.Run(t)

	ConvertTestCase{
		What:        "with custom css",
		EPUB:        testEPUB,
		ShouldError: false,

		Options: []ConverterOption{
			ConverterOptionAddCSS(".css1 {}"),
			ConverterOptionAddCSS(".css2 {}"),
		},
		Checks: []ShouldFunc{
			ShouldHaveAllSourceDocumentsWithSaneOPF(0),
			AllDocumentsShould(DocumentProbablyHasSpans, []string{"OEBPS/xhtml/title.xhtml"}),
			AllDocumentsShould(func(doc string) error {
				if !strings.Contains(doc, ".css1") || !strings.Contains(doc, ".css2") {
					return fmt.Errorf("missing css")
				}
				return nil
			}, nil),
		},
	}.Run(t)
}

type ConvertTestCase struct {
	What        string
	EPUB        fs.FS
	ShouldError bool

	Options []ConverterOption
	Checks  []ShouldFunc
}

func (tc ConvertTestCase) Run(t *testing.T) {
	t.Logf("case %q", tc.What)

	kepub := bytes.NewBuffer(nil)

	if err := NewConverterWithOptions(tc.Options...).Convert(context.Background(), kepub, tc.EPUB); err != nil {
		if !tc.ShouldError {
			t.Errorf("case %q: convert: unexpected error: %v", tc.What, err)
		}
		return
	} else if tc.ShouldError {
		t.Errorf("case %q: convert: expected error", tc.What)
		return
	}

	zr, err := zip.NewReader(bytes.NewReader(kepub.Bytes()), int64(kepub.Len()))
	if err != nil {
		panic(err)
	}

	for _, c := range tc.Checks {
		if err := c(tc.EPUB, zr); err != nil {
			t.Errorf("case %q: check: %v", tc.What, err)
		}
	}

	if _, ok := tc.EPUB.(*zip.Reader); ok {
		return
	}
	if _, ok := tc.EPUB.(*zip.ReadCloser); ok {
		return
	}
	if _, ok := tc.EPUB.(fstest.MapFS); !ok {
		panic("convert test case should be passed a zip or a MapFS")
	}

	if err := func() error {
		epub1, err := epubFsToZip(tc.EPUB)
		if err != nil {
			panic(err)
		}

		kepub1 := bytes.NewBuffer(nil)

		if err := NewConverterWithOptions(tc.Options...).Convert(context.Background(), kepub1, epub1); err != nil {
			return fmt.Errorf("failed to convert: %w", err)
		}

		zr1, err := zip.NewReader(bytes.NewReader(kepub1.Bytes()), int64(kepub1.Len()))
		if err != nil {
			return fmt.Errorf("failed to open output: %w", err)
		}

		type Item struct {
			Name  string
			Hash  string
			entry *zip.File
		}

		items := func(z *zip.Reader) []Item {
			x := make([]Item, 0, len(z.File))
			for _, f := range z.File {
				if !f.Mode().IsDir() {
					rc, err := f.Open()
					if err != nil {
						panic(err)
					}

					s := sha1.New()
					_, err = io.Copy(s, rc)
					rc.Close()

					if err != nil {
						panic(err)
					}

					x = append(x, Item{
						Name:  f.Name,
						Hash:  hex.EncodeToString(s.Sum(nil)),
						entry: f,
					})
				}
			}
			sort.SliceStable(x, func(i, j int) bool {
				return x[i].Name < x[j].Name
			})
			return x
		}

		a, b := items(zr), items(zr1)

		if len(a) != len(b) {
			return fmt.Errorf("file mismatch (\nfs  = %#v\n zip = %#v\n)", a, b)
		}

		for i, it := range a {
			if it.Name != b[i].Name || it.Hash != b[i].Hash {
				return fmt.Errorf("file mismatch (\nfs  = %#v\n zip = %#v\n)", a, b)
			}
		}

		return nil
	}(); err != nil {
		t.Errorf("case %q: ensure conversion of zipped EPUB matches conversion of FS: %v", tc.What, err)
		return
	}
}

type ShouldFunc func(old fs.FS, new *zip.Reader) error

func (c ShouldFunc) Because(what string) ShouldFunc {
	return func(old fs.FS, new *zip.Reader) error {
		if err := c(old, new); err != nil {
			return fmt.Errorf("%s: error: %w", what, err)
		}
		return nil
	}
}

func ShouldHaveAllSourceDocumentsWithSaneOPF(withNew int) ShouldFunc {
	return func(old fs.FS, new *zip.Reader) error {
		pkgO, err := epubPackage(old)
		if err != nil {
			panic(err)
		}

		cdO, err := epubContentDocuments(old, pkgO)
		if err != nil {
			panic(err)
		}

		pkgN, err := epubPackage(new)
		if err != nil {
			return fmt.Errorf("failed to parse kepub: %v", err)
		}

		cdN, err := epubContentDocuments(new, pkgN)
		if err != nil {
			return fmt.Errorf("failed to parse kepub: %v", err)
		}

		n := map[string]struct{}{}
		for _, d := range cdN {
			n[d] = struct{}{}
		}

		for _, d := range cdO {
			if _, ok := n[d]; !ok {
				return fmt.Errorf("epub has %q, but not the kepub", d)
			}
		}

		if new := len(cdN) - len(cdO); new != withNew {
			return fmt.Errorf("kepub has %d new content document(s), expected %d", new, withNew)
		}

		return nil
	}
}

func AllDocumentsShould(fn func(doc string) error, exclude []string) ShouldFunc {
	return func(old fs.FS, new *zip.Reader) error {
		pkg, err := epubPackage(new)
		if err != nil {
			return fmt.Errorf("failed to parse kepub: %v", err)
		}

		cd, err := epubContentDocuments(new, pkg)
		if err != nil {
			return fmt.Errorf("failed to parse kepub: %v", err)
		}

		for _, d := range cd {
			var x bool
			for _, ex := range exclude {
				if d == ex {
					x = true
				}
			}
			if x {
				continue
			}
			if err := FileShould(d, fn)(old, new); err != nil {
				return fmt.Errorf("document %q: %v", d, err)
			}
		}

		return nil
	}
}

func DocumentProbablyHasSpans(doc string) error {
	if !strings.Contains(doc, `class="koboSpan"`) {
		return fmt.Errorf("no spans in document")
	}
	return nil
}

func FileShould(file string, fn func(contents string) error) ShouldFunc {
	return func(old fs.FS, new *zip.Reader) error {
		rc, err := new.Open(file)
		if err != nil {
			return fmt.Errorf("failed to open file %q in kepub: %v", file, err)
		}
		defer rc.Close()

		buf, err := ioutil.ReadAll(rc)
		if err != nil {
			return fmt.Errorf("failed to read file %q in kepub: %v", file, err)
		}

		return fn(string(buf))
	}
}

func ShouldBeUnchanged(files ...string) ShouldFunc {
	return func(old fs.FS, new *zip.Reader) error {
		for _, f := range files {
			rcO, err := old.Open(f)
			if err != nil {
				panic(err)
			}
			defer rcO.Close()

			rcN, err := new.Open(f)
			if err != nil {
				return fmt.Errorf("file %q: %v", f, err)
			}
			defer rcN.Close()

			sO := sha1.New()
			if _, err := io.Copy(sO, rcO); err != nil {
				panic(err)
			}

			sN := sha1.New()
			if _, err := io.Copy(sN, rcN); err != nil {
				return fmt.Errorf("file %q: %v", f, err)
			}

			if !bytes.Equal(sO.Sum(nil), sN.Sum(nil)) {
				return fmt.Errorf("file %q is different in kepub", f)
			}
		}
		return nil
	}
}

func ShouldHaveFile(files ...string) ShouldFunc {
	return func(old fs.FS, new *zip.Reader) error {
		m := map[string]struct{}{}
		for _, fn := range new.File {
			m[fn.Name] = struct{}{}
		}
		for _, fn := range files {
			if _, ok := m[fn]; !ok {
				return fmt.Errorf("missing file %q", fn)
			}
		}
		return nil
	}
}

func ShouldNotHaveFile(files ...string) ShouldFunc {
	return func(old fs.FS, new *zip.Reader) error {
		m := map[string]struct{}{}
		for _, fn := range new.File {
			m[fn.Name] = struct{}{}
		}
		for _, fn := range files {
			if _, ok := m[fn]; ok {
				return fmt.Errorf("should not have file %q", fn)
			}
		}
		return nil
	}
}

func epubFsToZip(epub fs.FS) (*zip.Reader, error) {
	epub1 := bytes.NewBuffer(nil)

	zw := zip.NewWriter(epub1)

	if err := epubWriteMimetype(zw); err != nil {
		panic(err)
	}

	if err := fs.WalkDir(epub, ".", func(path string, d fs.DirEntry, err error) error {
		if d.IsDir() || d.Name() == "mimetype" {
			return nil
		}

		info, err := d.Info()
		if err != nil {
			return err
		}

		fh, err := zip.FileInfoHeader(info)
		if err != nil {
			return err
		}
		fh.Name = path

		w, err := zw.CreateHeader(fh)
		if err != nil {
			return err
		}

		rc, err := epub.Open(path)
		if err != nil {
			return err
		}
		defer rc.Close()

		_, err = io.Copy(w, rc)
		return err
	}); err != nil {
		return nil, err
	}

	if err := zw.Close(); err != nil {
		return nil, err
	}

	zr, err := zip.NewReader(bytes.NewReader(epub1.Bytes()), int64(epub1.Len()))
	if err != nil {
		return nil, err
	}

	return zr, nil
}

func init() {
	testEPUB = fstest.MapFS{}

	testEPUB["mimetype"] = &fstest.MapFile{
		Data: []byte("application/epub+zip"),
		Mode: 0666,
	}

	testEPUB["META-INF/container.xml"] = &fstest.MapFile{
		Data: []byte(`<?xml version="1.0" encoding="UTF-8"?>
<container version="1.0" xmlns="urn:oasis:names:tc:opendocument:xmlns:container">
	<rootfiles>
		<rootfile full-path="OEBPS/content.opf" media-type="application/oebps-package+xml"/>
	</rootfiles>
</container>
`),
		Mode: 0666,
	}

	testEPUB["OEBPS/content.opf"] = &fstest.MapFile{
		Data: []byte(`<?xml version="1.0" encoding="UTF-8"?>
<package xmlns="http://www.idpf.org/2007/opf" version="3.0">
	<metadata xmlns:dc="http://purl.org/dc/elements/1.1/">
		<dc:title>Test</dc:title>
	</metadata>
	<manifest>
		<item href="nav.xhtml" media-type="application/xhtml+xml" properties="nav"/>
		<item id="xhtml_title" href="xhtml/title.xhtml" media-type="application/xhtml+xml"/>` + stringFor(`
		<item id="xhtml_ch00" href="xhtml/ch00.xhtml" media-type="application/xhtml+xml"/>`, 1, 100, func(s string, i int) string {
			return strings.ReplaceAll(s, "00", fmt.Sprintf("%02d", i))
		}) + `
		<item id="cover" href="cover.png" media-type="image/png"/>
	</manifest>
	<spine>
		<itemref idref="xhtml_title"/>` + stringFor(`
		<itemref idref="xhtml_ch00"/>`, 1, 100, func(s string, i int) string {
			return strings.ReplaceAll(s, "00", fmt.Sprintf("%02d", i))
		}) + `
	</spine>
	<guide>
		<reference href="xhtml/title.xhtml" title="Cover Page" type="cover"/>
	</guide>
</package>
`),
		Mode: 0666,
	}

	cover := bytes.NewBuffer(nil)
	_ = png.Encode(cover, image.NewRGBA(image.Rect(0, 0, 300, 600)))

	testEPUB["OEBPS/cover.png"] = &fstest.MapFile{
		Data: cover.Bytes(),
		Mode: 0666,
	}

	testEPUB["OEBPS/nav.xhtml"] = &fstest.MapFile{
		Data: []byte(`<!DOCTYPE html>
<html xmlns="http://www.w3.org/1999/xhtml" xmlns:epub="http://www.idpf.org/2007/ops" lang="en">
	<head>
		<title>Test Book</title>
		<meta charset="utf-8/>
	</head>
	<body>
		<nav epub:type="toc">
			<ol>
				<li><a href="xhtml/title.xhtml">Cover Page</a></li>` + stringFor(`
				<li><a href="xhtml/ch00.xhtml">Chapter #</a></li>`, 1, 100, func(s string, i int) string {
			return strings.ReplaceAll(strings.ReplaceAll(s, "00", fmt.Sprintf("%02d", i)), "#", strconv.Itoa(i))
		}) + `
			</ol>
		</nav>
	</body>
</html>	
`),
		Mode: 0666,
	}

	testEPUB["OEBPS/xhtml/title.xhtml"] = &fstest.MapFile{
		Data: []byte(`<!DOCTYPE html>
<html xmlns="http://www.w3.org/1999/xhtml" xmlns:epub="http://www.idpf.org/2007/ops" lang="en">
	<head>
		<title>Test Book</title>
		<meta charset="utf-8/>
	</head>
	<body>
		<img style="height: 100%; max-width: 100%" src="cover.png" alt="Cover image"/>
	</body>
</html>	
`),
		Mode: 0666,
	}

	for i := 1; i < 100; i++ {
		var b strings.Builder
		b.WriteString(`<!DOCTYPE html>
<html xmlns="http://www.w3.org/1999/xhtml" xmlns:epub="http://www.idpf.org/2007/ops" lang="en">
<head>
	<title>Test Book</title>
	<meta charset="utf-8/>
</head>
<body>
	<h1>Chapter `)
		b.WriteString(strconv.Itoa(i))
		b.WriteString(`</h1>
`)

		r := rand.New(rand.NewSource(int64(i)))
		for j, jn := 0, r.Intn(200)+50; j < jn; j++ {
			b.WriteByte('\t')
			b.WriteByte('\t')
			switch r.Intn(3) {
			case 0:
				b.WriteString(`<p>`)
				switch r.Intn(4) {
				case 0:
					b.WriteString(`Lorem ipsum dolor sit amet, consectetur adipisicing elit. Nostrum eveniet doloremque pariatur facilis doloribus id amet laudantium voluptatibus, libero a esse. Corrupti sunt earum laborum eum unde aspernatur assumenda deleniti.`)
				case 1:
					b.WriteString(`Lorem <b>ipsum dolor sit amet consectetur</b> adipisicing elit. Non pariatur veniam ratione cupiditate et, enim <i>aperiam necessitatibus</i> ad quisquam molestiae quae delectus tempora fuga distinctio minima repellendus, maxime nobis? Eligendi!`)
				case 2:
					b.WriteString(`Lorem ipsum dolor sit amet consectetur <i>adipisicing elit. Beatae alias ipsa</i>, quisquam deserunt et error sequi deleniti odio harum quae odit tempora recusandae magni expedita temporibus, quia modi officiis dolorum.`)
				case 3:
					b.WriteString(`Lorem ipsum, dolor sit amet consectetur adipisicing elit. Eligendi, vero! Mollitia explicabo quod aperiam hic dolores commodi vero perferendis sequi. Ratione accusantium repellat distinctio quaerat architecto fuga totam, minima deserunt?`)
				}
				b.WriteString(`</p>`)
			case 1:
				b.WriteString(`<img src="cover.png"/>`)
				b.WriteByte('\n')
			case 2:
				b.WriteString(`<ul>`)
				for k := 0; k < r.Intn(5)+5; k++ {
					b.WriteString(`<li>Test item.</li>`)
				}
				b.WriteString(`</ul>`)
			}
			b.WriteByte('\n')
		}

		b.WriteString(`
</body>
</html>	
`)
		testEPUB["OEBPS/xhtml/ch"+fmt.Sprintf("%02d", i)+".xhtml"] = &fstest.MapFile{
			Data: []byte(b.String()),
			Mode: 0666,
		}
	}
}

func overlayMapFS(orig fstest.MapFS, overlay fstest.MapFS) fstest.MapFS {
	new := fstest.MapFS{}
	for f, c := range orig {
		new[f] = c
	}
	for f, c := range overlay {
		if c == nil {
			delete(new, f)
		} else {
			new[f] = c
		}
	}
	return new
}

func stringFor(s string, start int, end int, fn func(s string, i int) string) string {
	var b strings.Builder
	for i := start; i < end; i++ {
		b.WriteString(fn(s, i))
	}
	return b.String()
}
