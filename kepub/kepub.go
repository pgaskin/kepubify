package kepub

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/beevik/etree"
	zglob "github.com/mattn/go-zglob"
)

// Kepubify converts a .epub into a .kepub.epub.
// It can also optionally run a postprocessor for each file on the goquery.Document, or the html string.
func Kepubify(src, dest string, verbose bool, postDoc *func(doc *goquery.Document) error, postHTML *func(h *string) error) error {
	td, err := ioutil.TempDir("", "kepubify")
	if err != nil {
		return fmt.Errorf("could not create temp dir: %s", err)
	}
	defer os.RemoveAll(td)

	if verbose {
		fmt.Printf("  Unpacking ePub\n")
	}
	UnpackEPUB(src, td, true)

	a, err := zglob.Glob(filepath.Join(td, "**", "*.html"))
	if err != nil {
		return fmt.Errorf("could not create find content files: %s", err)
	}
	b, err := zglob.Glob(filepath.Join(td, "**", "*.xhtml"))
	if err != nil {
		return fmt.Errorf("could not create find content files: %s", err)
	}
	c, err := zglob.Glob(filepath.Join(td, "**", "*.htm"))
	if err != nil {
		return fmt.Errorf("could not create find content files: %s", err)
	}
	contentfiles := append(append(a, b...), c...)

	if verbose {
		fmt.Printf("  Processing %v content files\n  ", len(contentfiles))
	}

	runtime.GOMAXPROCS(runtime.NumCPU() + 1)
	wg := sync.WaitGroup{}
	cerr := make(chan error, 1)
	for _, f := range contentfiles {
		wg.Add(1)
		go func(cf string) {
			defer wg.Done()
			buf, err := ioutil.ReadFile(cf)
			if err != nil {
				select {
				case cerr <- fmt.Errorf("Could not open content file \"%s\" for reading: %s", cf, err): // Put err in the channel unless it is full
				default:
				}
				return
			}
			str := string(buf)
			err = process(&str, postDoc, postHTML)
			if err != nil {
				select {
				case cerr <- fmt.Errorf("Error processing content file \"%s\": %s", cf, err): // Put err in the channel unless it is full
				default:
				}
				return
			}
			err = ioutil.WriteFile(cf, []byte(str), 0644)
			if err != nil {
				select {
				case cerr <- fmt.Errorf("Error writing content file \"%s\": %s", cf, err): // Put err in the channel unless it is full
				default:
				}
				return
			}
			if verbose {
				fmt.Print(".")
			}
			time.Sleep(time.Millisecond * 5)
		}(f)
	}
	wg.Wait()
	if len(cerr) > 0 {
		return <-cerr
	}

	if verbose {
		fmt.Printf("\n  Cleaning content.opf\n")
	}

	rsk, err := os.Open(filepath.Join(td, "META-INF", "container.xml"))
	if err != nil {
		return fmt.Errorf("error opening container.xml: %s", err)
	}
	defer rsk.Close()

	container := etree.NewDocument()
	_, err = container.ReadFrom(rsk)
	if err != nil {
		return fmt.Errorf("error parsing container.xml: %s", err)
	}

	rootfile := ""
	for _, e := range container.FindElements("//rootfiles/rootfile[@full-path]") {
		rootfile = e.SelectAttrValue("full-path", "")
	}
	if rootfile == "" {
		return fmt.Errorf("error parsing container.xml")
	}

	buf, err := ioutil.ReadFile(filepath.Join(td, rootfile))
	if err != nil {
		return fmt.Errorf("error parsing content.opf: %s", err)
	}

	opf := string(buf)

	err = processOPF(&opf)
	if err != nil {
		return fmt.Errorf("error cleaning content.opf: %s", err)
	}

	err = ioutil.WriteFile(filepath.Join(td, rootfile), []byte(opf), 0644)
	if err != nil {
		return fmt.Errorf("error writing new content.opf: %s", err)
	}

	if verbose {
		fmt.Printf("  Cleaning epub files\n")
	}
	cleanFiles(td)

	if verbose {
		fmt.Printf("  Packing ePub\n")
	}
	PackEPUB(td, dest, true)
	return nil
}
