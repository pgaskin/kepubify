package main

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/mattn/go-zglob"

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
		return fmt.Errorf("Input path must not be blank.")
	}

	infile, err := filepath.Abs(infile)
	if err != nil {
		return fmt.Errorf("Error resolving input path: %s.", err)
	}

	if !exists(infile) {
		return fmt.Errorf("Input file or directory does not exist.")
	}

	if isDir(infile) {
		if len(c.Args()) != 2 {
			return fmt.Errorf("Because input is a dir, a second argument must be supplied with a nonexistent dir for the conversion output.")
		}

		outdir := c.Args().Get(1)
		if exists(outdir) {
			return fmt.Errorf("Because input is a dir, a second argument must be supplied with a nonexistent dir for the conversion output.")
		}

		outdir, err = filepath.Abs(outdir)
		if err != nil {
			return fmt.Errorf("Error resolving output dir path: %s.", err)
		}

		err := os.Mkdir(outdir, os.ModePerm)
		if err != nil {
			return fmt.Errorf("Error creating output dir: %s.", err)
		}

		epubs, err := zglob.Glob(filepath.Join(infile, "**", "*.epub"))
		if err != nil {
			return fmt.Errorf("Error searching for epubs in input dir: %s.", err)
		}

		errs := map[string]error{}
		for i, epub := range epubs {
			rel, err := filepath.Rel(infile, epub)
			if err != nil {
				fmt.Printf("[%v/%v] Error resolving relative path of %s: %v\n", i+1, len(epubs), epub, err)
				errs[epub] = err
				continue
			}

			err = os.MkdirAll(filepath.Join(outdir, filepath.Dir(rel)), os.ModePerm)
			if err != nil {
				fmt.Printf("[%v/%v] Error creating output dir for %s: %v\n", i+1, len(epubs), epub, err)
				errs[rel] = err
				continue
			}

			outfile := fmt.Sprintf("%s.kepub.epub", filepath.Join(outdir, strings.Replace(rel, ".epub", "", -1)))
			fmt.Printf("[%v/%v] Converting %s\n", i+1, len(epubs), rel)

			err = kepub.Kepubify(epub, outfile, false)
			if err != nil {
				fmt.Printf("[%v/%v] Error converting %s: %v\n", i+1, len(epubs), rel, err)
				errs[rel] = err
				continue
			}
		}

		fmt.Printf("\nSucessfully converted %v of %v ebooks\n", len(epubs)-len(errs), len(epubs))
		if len(errs) > 0 {
			fmt.Printf("Errors:\n")
			for epub, err := range errs {
				fmt.Printf("%s: %v\n", epub, err)
			}
		}
	} else {
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

		fmt.Printf("Successfully converted \"%s\" to a kepub.\nYou can find the converted file at \"%s\"\n", infile, outfile)
	}
	return nil
}

func main() {
	app := cli.NewApp()

	app.Name = "kepubify"
	app.Usage = "Convert epub to kepub"
	app.Description = "Convert your ePubs into kepubs, with a easy-to-use command-line tool."
	app.Version = version

	app.ArgsUsage = "EPUB_INPUT_PATH [KEPUB_OUTPUT_PATH]"
	app.Action = func(c *cli.Context) error {
		err := convert(c)
		if err != nil {
			fmt.Println(err)
		}

		if runtime.GOOS == "windows" && len(c.Args()) == 1 {
			time.Sleep(1 * time.Second)
		}

		return err
	}

	app.Run(os.Args)
}
