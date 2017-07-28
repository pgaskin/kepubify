package main

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/geek1011/kepubify/kepub"

	cli "gopkg.in/urfave/cli.v1"
)

var version = "dev"

// exists checks whether a path exists
func exists(path string) bool {
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		return true
	}
	return false
}

// isDir checks if a exists and is a dir
func isDir(path string) bool {
	if fi, err := os.Stat(path); err == nil && fi.IsDir() {
		return true
	}
	return false
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
	fmt.Println()

	err = kepub.Kepubify(infile, outfile, true)
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
