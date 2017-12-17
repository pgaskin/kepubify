package kepub

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"time"

	"github.com/beevik/etree"
	"github.com/cheggaaa/pb"
	zglob "github.com/mattn/go-zglob"
)

// Kepubify converts a .epub into a .kepub.epub
func Kepubify(src, dest string, printlog bool) error {
	defer func() {
		if printlog {
			fmt.Printf("\n")
		}
	}()

	td, err := ioutil.TempDir("", "kepubify")
	if err != nil {
		return fmt.Errorf("could not create temp dir: %s", err)
	}
	defer os.RemoveAll(td)

	if printlog {
		fmt.Printf("Unpacking ePub")
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

	if printlog {
		fmt.Printf("\rProcessing %v content files              \n", len(contentfiles))
	}

	var bar *pb.ProgressBar

	if printlog {
		bar = pb.New(len(contentfiles))
		bar.SetRefreshRate(time.Millisecond * 60)
		bar.SetMaxWidth(60)
		bar.Format("[=> ]")
		bar.Start()
	}
	defer func() {
		if printlog && bar != nil {
			bar.Finish()
		}
	}()

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
			err = process(&str)
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
			time.Sleep(time.Millisecond * 5)
			if printlog {
				bar.Increment()
			}
		}(f)
	}
	wg.Wait()
	if len(cerr) > 0 {
		return <-cerr
	}

	if printlog {
		bar.Finish()
		fmt.Printf("\rCleaning content.opf              ")
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

	if printlog {
		fmt.Printf("\rCleaning epub files             ")
	}
	cleanFiles(td)

	if printlog {
		fmt.Printf("\rPacking ePub                    ")
	}
	PackEPUB(td, dest, true)
	return nil
}
