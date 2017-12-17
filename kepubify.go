package main

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/geek1011/kepubify/kepub"
	zglob "github.com/mattn/go-zglob"
)

var version = "dev"
var helpText = `USAGE: kepubify INPUT_PATH [OUTPUT_PATH]

VERSION: {{.Version}}

INPUT_PATH:
   The input file or directory. If it is a directory,
   OUTPUT_PATH must be a nonexistent directory.

OUTPUT_PATH:
   The path to place the converted ebook(s). Can only
   be a directory if INPUT_PATH is a directory.

   By default, this is the basename of the input file, 
   with the extension .kepub.epub

The full documentation is available at:
   https://geek1011.github.io/kepubify
`

func helpExit() {
	fmt.Print(helpText)
	if runtime.GOOS == "windows" {
		time.Sleep(time.Second)
	}
	os.Exit(1)
}

func versionExit() {
	fmt.Printf("kepubify %s\n", version)
	if runtime.GOOS == "windows" {
		time.Sleep(time.Second)
	}
	os.Exit(1)
}

func errExit(format string, a ...interface{}) {
	fmt.Fprintf(os.Stderr, format, a...)
	if runtime.GOOS == "windows" {
		time.Sleep(time.Second)
	}
	os.Exit(1)
}

func msgExit(format string, a ...interface{}) {
	fmt.Printf(format, a...)
	if runtime.GOOS == "windows" {
		time.Sleep(time.Second)
	}
	os.Exit(0)
}

func main() {
	helpText = strings.Replace(helpText, "{{.Version}}", version, 1)

	var src, dest string
	switch len(os.Args) {
	case 2:
		src = os.Args[1]
		src, err := filepath.Abs(src)
		if err != nil {
			errExit("Error resolving source file path: %v\n", dest)
		}

		if src == "--help" || src == "-h" {
			helpExit()
		} else if src == "--version" || src == "-v" {
			versionExit()
		} else if isFile(src) {
			if strings.HasSuffix(src, ".kepub.epub") {
				errExit("Input file is already a kepub.\n")
			} else if strings.HasSuffix(src, ".epub") {
				dest := fmt.Sprintf("%s.kepub.epub", strings.Replace(filepath.Base(src), ".epub", "", -1))
				dest, err := filepath.Abs(dest)
				if err != nil {
					errExit("Error resolving destination file path: %v\n", dest)
				}

				fmt.Printf("Converting '%s' to '%s'\n\n", os.Args[1], dest)

				err = kepub.Kepubify(src, dest, true)
				if err != nil {
					errExit("Error converting file: %v\n", err)
				}

				msgExit("\nSuccessfully converted '%s' to '%s'\n", os.Args[1], dest)
			} else {
				errExit("Input file must end with .epub. See kepubify --help for more details.\n")
			}
		} else if isDir(src) {
			errExit("To batch convert, a second argument must be supplied with an output dir. See kepubify --help for more details.\n")
		} else if !exists(src) {
			errExit("Input file '%s' does not exist.\n", src)
		} else {
			helpExit()
		}

	case 3:
		src = os.Args[1]
		src, err := filepath.Abs(src)
		if err != nil {
			errExit("Error resolving source file path: %v\n", dest)
		}

		dest = os.Args[2]
		src, err = filepath.Abs(src)
		if err != nil {
			errExit("Error resolving dest file path: %v\n", dest)
		}

		if isFile(src) {
			if strings.HasSuffix(src, ".kepub.epub") {
				errExit("Input file is already a kepub.\n")
			} else if strings.HasSuffix(src, ".epub") {
				if isDir(dest) {
					dest = filepath.Join(dest, fmt.Sprintf("%s.kepub.epub", strings.Replace(filepath.Base(src), ".epub", "", -1)))

					fmt.Printf("Converting '%s' to '%s'\n\n", os.Args[1], dest)

					err = kepub.Kepubify(src, dest, true)
					if err != nil {
						errExit("Error converting file: %v\n", err)
					}

					msgExit("\nSuccessfully converted '%s' to '%s'\n", os.Args[1], dest)
				} else if strings.HasSuffix(dest, ".kepub.epub") {
					fmt.Printf("Converting '%s' to '%s'\n\n", os.Args[1], os.Args[2])

					err = kepub.Kepubify(src, dest, true)
					if err != nil {
						errExit("Error converting file: %v\n", err)
					}

					msgExit("\nSuccessfully converted '%s' to '%s'\n", os.Args[1], os.Args[2])
				} else {
					errExit("Output file must end with .kepub.epub. See kepubify --help for more details.\n")
				}
			} else {
				errExit("Input file must end with .epub. See kepubify --help for more details.\n")
			}
		} else if isDir(src) {
			if !exists(dest) || isEmptyDir(dest) {
				if !exists(dest) {
					err := os.Mkdir(dest, os.ModePerm)
					if err != nil {
						errExit("Error creating output dir: %s\n", err)
					}
				}

				lst, err := zglob.Glob(filepath.Join(src, "**", "*.epub"))
				if err != nil {
					errExit("Error searching for epubs in input dir: %s\n", err)
				}

				epubs := []string{}
				for _, f := range lst {
					if !strings.HasSuffix(f, ".kepub.epub") {
						epubs = append(epubs, f)
					}
				}

				fmt.Printf("%v books found\n", len(epubs))

				errs := map[string]error{}
				for i, epub := range epubs {
					rel, err := filepath.Rel(src, epub)
					if err != nil {
						fmt.Printf("[%v/%v] Error resolving relative path of %s: %v\n", i+1, len(epubs), epub, err)
						errs[epub] = err
						continue
					}

					err = os.MkdirAll(filepath.Join(dest, filepath.Dir(rel)), os.ModePerm)
					if err != nil {
						fmt.Printf("[%v/%v] Error creating output dir for %s: %v\n", i+1, len(epubs), epub, err)
						errs[rel] = err
						continue
					}

					outfile := fmt.Sprintf("%s.kepub.epub", filepath.Join(dest, strings.Replace(rel, ".epub", "", -1)))
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

				os.Exit(0)
			} else {
				errExit("Output must be a nonexistent or empty directory when batch converting. See kepubify --help for more details.\n\n")
			}
		} else if !exists(src) {
			errExit("Input '%s' does not exist.\n", src)
		} else {
			helpExit()
		}
	default:
		helpExit()
	}
}
