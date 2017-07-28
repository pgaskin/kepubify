package kepub

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"time"

	"github.com/beevik/etree"
	"github.com/cheggaaa/pb"
	zglob "github.com/mattn/go-zglob"
)

// Kepubify converts a .epub into a .kepub.epub
func Kepubify(src, dest string, printlog bool) error {
	td, err := ioutil.TempDir("", "kepubify")
	if err != nil {
		return fmt.Errorf("Could not create temp dir: %s", err)
	}
	defer os.RemoveAll(td)

	if printlog {
		fmt.Println("Unpacking ePub.")
	}
	UnpackEPUB(src, td, true)
	if printlog {
		fmt.Println()
	}

	a, err := zglob.Glob(filepath.Join(td, "**", "*.html"))
	if err != nil {
		return fmt.Errorf("Could not create find content files: %s", err)
	}
	b, err := zglob.Glob(filepath.Join(td, "**", "*.xhtml"))
	if err != nil {
		return fmt.Errorf("Could not create find content files: %s", err)
	}
	c, err := zglob.Glob(filepath.Join(td, "**", "*.htm"))
	if err != nil {
		return fmt.Errorf("Could not create find content files: %s", err)
	}
	contentfiles := append(append(a, b...), c...)

	if printlog {
		fmt.Printf("Processing %v content files.\n", len(contentfiles))
	}

	var bar *pb.ProgressBar

	if printlog {
		bar = pb.New(len(contentfiles))
		bar.SetRefreshRate(time.Millisecond * 60)
		bar.SetMaxWidth(60)
		bar.Format("[=> ]")
		bar.Start()
	}

	for _, cf := range contentfiles {
		buf, err := ioutil.ReadFile(cf)
		if err != nil {
			return fmt.Errorf("Could not open content file \"%s\" for reading: %s", cf, err)
		}
		str := string(buf)
		err = process(&str)
		if err != nil {
			return fmt.Errorf("Error processing content file \"%s\": %s", cf, err)
		}
		err = ioutil.WriteFile(cf, []byte(str), 0644)
		if err != nil {
			return fmt.Errorf("Error writing content file \"%s\": %s", cf, err)
		}
		time.Sleep(time.Millisecond * 5)
		if printlog {
			bar.Increment()
		}
	}

	if printlog {
		bar.Finish()
		fmt.Println()

		fmt.Println("Cleaning content.opf.")
		fmt.Println()
	}

	rsk, err := os.Open(filepath.Join(td, "META-INF", "container.xml"))
	if err != nil {
		return fmt.Errorf("Error parsing container.xml: %s", err)
	}
	defer rsk.Close()

	container := etree.NewDocument()
	_, err = container.ReadFrom(rsk)
	if err != nil {
		return fmt.Errorf("Error parsing container.xml: %s", err)
	}

	rootfile := ""
	for _, e := range container.FindElements("//rootfiles/rootfile[@full-path]") {
		rootfile = e.SelectAttrValue("full-path", "")
	}
	if rootfile == "" {
		return fmt.Errorf("Error parsing container.xml")
	}

	buf, err := ioutil.ReadFile(filepath.Join(td, rootfile))
	if err != nil {
		return fmt.Errorf("Error parsing content.opf: %s", err)
	}

	opf := string(buf)

	err = cleanOPF(&opf)
	if err != nil {
		return fmt.Errorf("Error cleaning content.opf: %s", err)
	}

	err = ioutil.WriteFile(filepath.Join(td, rootfile), []byte(opf), 0644)
	if err != nil {
		return fmt.Errorf("Error writing new content.opf: %s", err)
	}

	if printlog {
		fmt.Println("Cleaning epub files.")
		fmt.Println()
	}
	cleanFiles(td)

	if printlog {
		fmt.Println("Packing ePub.")
		fmt.Println()
	}
	PackEPUB(td, dest, true)
	return nil
}
