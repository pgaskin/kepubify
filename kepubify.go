package main

import (
	"fmt"
	"io/ioutil"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/cheggaaa/pb"
	zglob "github.com/mattn/go-zglob"
	cli "gopkg.in/urfave/cli.v1"
)

var version = "dev"

func kepubify(src, dest string) error {
	td, err := ioutil.TempDir("", "kepubify")
	if err != nil {
		return fmt.Errorf("Could not create temp dir: %s", err)
	}
	defer os.RemoveAll(td)

	fmt.Println("Unpacking ePub.")
	UnpackEPUB(src, td, true)
	fmt.Println()

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

	fmt.Printf("Processing %v content files.\n", len(contentfiles))

	bar := pb.New(len(contentfiles))
	bar.SetRefreshRate(time.Millisecond * 300)
	bar.SetMaxWidth(60)
	bar.Format("[=> ]")
	bar.Start()

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
		time.Sleep(time.Millisecond * 25)
		bar.Increment()
	}

	bar.Finish()
	fmt.Println()

	fmt.Println("Packing ePub.")
	fmt.Println()
	PackEPUB(td, dest, true)
	return nil
}

func convert(c *cli.Context) error {
	if len(c.Args()) < 1 || len(c.Args()) > 2 {
		return fmt.Errorf("Invalid number of arguments. Usage: kepubify EPUB_INPUT_PATH [KEPUB_OUTPUT_PATH]")
	}

	infile := c.Args().Get(0)
	if infile == "" {
		return fmt.Errorf("Input file path must not be blank.")
	}

	infile, err := filepath.Abs(infile)
	if err != nil {
		return fmt.Errorf("Error resolving input file path: %s.", err)
	}

	if !exists(infile) {
		return fmt.Errorf("Input file does not exist.")
	}

	if isDir(infile) {
		return fmt.Errorf("Input file must be a file, not a directory.")
	}

	if filepath.Ext(infile) != ".epub" {
		return fmt.Errorf("Input file must have the extension \".epub\".")
	}

	outfile := fmt.Sprintf("%s.kepub.epub", strings.Replace(filepath.Base(infile), ".epub", "", -1))
	if len(c.Args()) == 2 {
		if exists(c.Args().Get(1)) {
			if isDir(c.Args().Get(1)) {
				outfile = path.Join(c.Args().Get(1), outfile)
			} else {
				return fmt.Errorf("Output path must be a nonexistent file ending with .kepub.epub, or be an existing directory")
			}
		} else {
			if strings.HasSuffix(c.Args().Get(1), ".kepub.epub") {
				outfile = c.Args().Get(1)
			} else {
				return fmt.Errorf("Output path must be a nonexistent file ending with .kepub.epub, or be an existing directory")
			}
		}
	}

	outfile, err = filepath.Abs(outfile)
	if err != nil {
		return fmt.Errorf("Error resolving output file path: %s.", err)
	}

	fmt.Printf("Input file: %s\n", infile)
	fmt.Printf("Output file: %s\n", outfile)

	err = kepubify(infile, outfile)
	if err != nil {
		return fmt.Errorf("Error converting epub to kepub: %s.", err)
	}

	fmt.Printf("Succesfully converted \"%s\" to a kepub.\nYou can find the converted file at \"%s\"\n", infile, outfile)

	if runtime.GOOS == "windows" {
		time.Sleep(5000 * time.Second)
	}

	return nil
}

func main() {
	app := cli.NewApp()

	app.Name = "kepubify"
	app.Description = "Convert your ePubs into kepubs, with a easy-to-use command-line tool."
	app.Version = version

	app.ArgsUsage = "EPUB_INPUT_PATH [KEPUB_OUTPUT_PATH]"
	app.Action = convert

	app.Run(os.Args)
}
