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
   The input file or directory.

   For single converting, this must be an existing file with
   the extension .epub.
   
   For batch converting, this must be a existing directory.

OUTPUT_PATH:
   The path to place the converted ebook(s).
   
   If single converting, this path will be overwritten if it exists. It
   must also end with .kepub.epub.
   
   If batch converting, this path must not exist, an will be automatically
   created.

   If not specified, then for a single converting, this is the basename
   of the input file with the extension .kepub.epub. For batch converting,
   this is the directory name, with the suffix _converted.

The full documentation is available at:
   https://geek1011.github.io/kepubify
`

func helpExit() {
	fmt.Print(helpText)
	winerrsleep()
	os.Exit(1)
}

func versionExit() {
	fmt.Printf("kepubify %s\n", version)
	winerrsleep()
	os.Exit(1)
}

func cprintf(do bool, format string, a ...interface{}) {
	if do {
		fmt.Printf(format, a...)
	}
}

func winsleep() {
	if runtime.GOOS == "windows" {
		time.Sleep(time.Second)
	}
}

func winerrsleep() {
	if runtime.GOOS == "windows" {
		time.Sleep(time.Second * 2)
	}
}

func doSingle(src, dest string, overwrite, verbose, validateFilenames bool) {
	cprintf(verbose, "Converting '%s' to '%s'\n\n", src, dest)

	if !exists(src) {
		cprintf(verbose, "Error: Source file does not exist.\n")
		winerrsleep()
		os.Exit(1)
		return
	}

	if validateFilenames && !strings.HasSuffix(src, ".epub") {
		cprintf(verbose, "Error: Source file does not end with .epub.\n")
		winerrsleep()
		os.Exit(1)
		return
	}

	if validateFilenames && strings.HasSuffix(src, ".kepub.epub") {
		cprintf(verbose, "Error: Source file is already a kepub.\n")
		winerrsleep()
		os.Exit(1)
		return
	}

	if validateFilenames && !strings.HasSuffix(dest, ".kepub.epub") {
		cprintf(verbose, "Error: Destination file does not end with .kepub.epub.\n")
		winerrsleep()
		os.Exit(1)
		return
	}

	if !overwrite && exists(dest) {
		cprintf(verbose, "Error: Destination file already exists.\n")
		winerrsleep()
		os.Exit(1)
		return
	}

	err := kepub.Kepubify(src, dest, true)
	if err != nil {
		cprintf(verbose, "Error: Could not convert file: %v\n", err)
		winerrsleep()
		os.Exit(1)
		return
	}

	cprintf(verbose, "\nSuccessfully converted '%s' to '%s'\n", os.Args[1], dest)

	winsleep()
	os.Exit(0)
	return
}

func doBatch(src, dest string, overwrite, verbose, validateFilenames, reconvertExisting bool) {
	if isFile(dest) {
		cprintf(verbose, "Error: Input is a directory, but output is a file.\n")
		winerrsleep()
		os.Exit(1)
		return
	}
	if isFile(src) {
		cprintf(verbose, "Error: Input is not a directory.\n", src)
		winerrsleep()
		os.Exit(1)
		return
	}

	if !overwrite && exists(dest) && !isEmptyDir(dest) {
		cprintf(verbose, "Error: Output dir already exists, and is not empty.\n")
		winerrsleep()
		os.Exit(1)
		return
	}

	if !exists(dest) {
		err := os.Mkdir(dest, os.ModePerm)
		if err != nil {
			cprintf(verbose, "Error: Could not create output dir: %s\n", err)
			winerrsleep()
			os.Exit(1)
			return
		}
	}

	lst, err := zglob.Glob(filepath.Join(src, "**", "*.epub"))
	if err != nil {
		cprintf(verbose, "Error: Could not search for epubs in input dir: %s\n", err)
		winerrsleep()
		os.Exit(1)
		return
	}

	cprintf(verbose, "Converting '%s' to '%s'.\n", src, dest)

	epubs := []string{}
	for _, f := range lst {
		if !strings.HasSuffix(f, ".kepub.epub") {
			epubs = append(epubs, f)
		}
	}

	cprintf(verbose, "%v books found\n", len(epubs))

	errs := map[string]error{}
	for i, epub := range epubs {
		rel, err := filepath.Rel(src, epub)
		if err != nil {
			cprintf(verbose, "[%v/%v] Error resolving relative path of %s: %v\n", i+1, len(epubs), epub, err)
			errs[epub] = err
			continue
		}

		err = os.MkdirAll(filepath.Join(dest, filepath.Dir(rel)), os.ModePerm)
		if err != nil {
			cprintf(verbose, "[%v/%v] Error creating output dir for %s: %v\n", i+1, len(epubs), epub, err)
			errs[rel] = err
			continue
		}

		outfile := fmt.Sprintf("%s.kepub.epub", filepath.Join(dest, strings.Replace(rel, ".epub", "", -1)))
		if !reconvertExisting && exists(outfile) {
			cprintf(verbose, "[%v/%v] Skipping already converted file %s\n", i+1, len(epubs), rel)
			continue
		}
		cprintf(verbose, "[%v/%v] Converting %s\n", i+1, len(epubs), rel)

		err = kepub.Kepubify(epub, outfile, false)
		if err != nil {
			cprintf(verbose, "[%v/%v] Error converting %s: %v\n", i+1, len(epubs), rel, err)
			errs[rel] = err
			continue
		}
	}

	cprintf(verbose, "\nSucessfully converted %v of %v ebooks\n", len(epubs)-len(errs), len(epubs))
	if len(errs) > 0 {
		cprintf(verbose, "Errors:\n")
		for epub, err := range errs {
			cprintf(verbose, "%s: %v\n", epub, err)
		}
	}

	os.Exit(0)
	return
}

func mustResolve(path string) string {
	// Note: does not error if not exists; only cleans and makes absolute.
	p, err := filepath.Abs(path)
	if err != nil {
		fmt.Printf("Error: Could not resolve path '%s': '%s'\n", path, err)
		winerrsleep()
		os.Exit(1)
	}

	return p
}

func main() {
	helpText = strings.Replace(helpText, "{{.Version}}", version, 1)

	var src, dest string

	switch len(os.Args) {
	case 2:
		src = mustResolve(os.Args[1])

		if src == "--help" || src == "-h" {
			helpExit()
		} else if src == "--version" || src == "-v" {
			versionExit()
		} else if isFile(src) {
			dest := mustResolve(fmt.Sprintf("%s.kepub.epub", strings.Replace(filepath.Base(src), ".epub", "", -1)))

			doSingle(src, dest, true, true, true)
		} else if isDir(src) {
			dest := mustResolve(fmt.Sprintf("%s_converted", filepath.Base(src)))

			if !exists(dest) {
				err := os.Mkdir(dest, os.ModePerm)
				if err != nil {
					fmt.Printf("Error: Could not create output dir: %s\n", err)
					winerrsleep()
					os.Exit(1)
				}
			}

			doBatch(src, dest, true, true, true, true)
		} else if !exists(src) {
			fmt.Printf("Error: Input file '%s' does not exist.\n", src)
			winerrsleep()
			os.Exit(1)
		} else {
			helpExit()
		}

	case 3:
		src = mustResolve(os.Args[1])
		dest = mustResolve(os.Args[2])

		if isFile(src) {
			if isDir(dest) {
				dest = filepath.Join(dest, fmt.Sprintf("%s.kepub.epub", strings.Replace(filepath.Base(src), ".epub", "", -1)))
			}
			doSingle(src, dest, true, true, true)
		} else if isDir(src) {
			doBatch(src, dest, false, true, true, true)
		} else if !exists(src) {
			fmt.Printf("Error: Input '%s' does not exist.\n", src)
			winerrsleep()
			os.Exit(1)
		} else {
			helpExit()
		}
	default:
		helpExit()
	}
}
