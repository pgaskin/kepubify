// Command kepubify is a fast, standalone, EPUB to KEPUB converter.
package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/pgaskin/kepubify/v4/internal/zip"
	"github.com/pgaskin/kepubify/v4/kepub"
	"github.com/spf13/pflag"
)

var version = "v4-dev"

func main() {
	pflag.CommandLine.SortFlags = false

	verbose := pflag.BoolP("verbose", "v", false, "Show extra information in output")
	sversion := pflag.Bool("version", false, "Show the version")
	help := pflag.BoolP("help", "h", false, "Show this help text")

	for _, flag := range []string{"verbose", "version", "help"} {
		pflag.CommandLine.SetAnnotation(flag, "category", []string{"1.General Options"})
	}

	update := pflag.BoolP("update", "u", false, "Don't reconvert files which have already been converted (i.e. don't overwrite output files)")
	inplace := pflag.BoolP("inplace", "i", false, "Don't add the _converted suffix to converted files and directories")
	nopreservedirs := pflag.Bool("no-preserve-dirs", false, "Flatten the directory structure of the input (an error will be shown if there are conflicts)")
	output := pflag.StringP("output", "o", "", "[>1 inputs || 1 file input with existing dir output]: Directory to place converted files/dirs under; [1 file input with nonexistent output]: Output filename; [1 dir input]: Output directory for contents of input (default: current directory)")
	calibre := pflag.Bool("calibre", false, "Use .kepub instead of .kepub.epub as the output extension (for Calibre compatibility, only use if you know what you are doing)")
	copy := pflag.StringSliceP("copy", "x", nil, "Copy files with the specified extension (with a leading period) to the output unchanged (no effect if the filename ends up the same)")

	for _, flag := range []string{"update", "inplace", "no-preserve-dirs", "output", "calibre", "copy"} {
		pflag.CommandLine.SetAnnotation(flag, "category", []string{"2.Output Options"})
	}

	smartenpunct := pflag.Bool("smarten-punctuation", false, "Smarten punctuation (smart quotes, dashes, etc) (excluding pre and code tags)")
	css := pflag.StringArrayP("css", "c", nil, "Custom CSS to add to ebook")
	hyphenate := pflag.Bool("hyphenate", false, "Force enable hyphenation")
	nohyphenate := pflag.Bool("no-hyphenate", false, "Force disable hyphenation")
	fullscreenfixes := pflag.Bool("fullscreen-reading-fixes", false, "Enable fullscreen reading bugfixes based on https://www.mobileread.com/forums/showpost.php?p=3113460&postcount=16")
	adddummytitlepage := pflag.Bool("add-dummy-titlepage", false, "Force-enables the dummy titlepage to fix layout issues with the first content file on certain books (this is enabled when needed using a heuristic if not specified)")
	noadddummytitlepage := pflag.Bool("no-add-dummy-titlepage", false, "Force-disables the dummy titlepage")
	replace := pflag.StringArrayP("replace", "r", nil, "Find and replace on all html files (repeat any number of times) (format: find|replace)")
	charset := pflag.String("charset", "utf-8", "Override the HTML charset (use \"auto\" to detect it from the content)")
	leaveaudio := pflag.Bool("leave-audio", false, "Leave audio files in the book")

	for _, flag := range []string{"smarten-punctuation", "css", "hyphenate", "no-hyphenate", "fullscreen-reading-fixes", "add-dummy-titlepage", "no-add-dummy-titlepage", "replace", "charset"} {
		pflag.CommandLine.SetAnnotation(flag, "category", []string{"3.Conversion Options"})
	}

	// --- Parse options --- //

	pflag.Parse()

	if *sversion {
		fmt.Printf("kepubify %s\n", version)
		exit(0)
		return
	}

	if *help || pflag.NArg() == 0 {
		helpExit()
		return
	}

	if *hyphenate && *nohyphenate {
		fmt.Printf("Error: --hyphenate and --no-hyphenate are mutally exclusive. See --help for more details.\n")
		exit(2)
		return
	}

	for _, c := range *copy {
		if len(c) == 0 || c[0] != '.' {
			fmt.Printf("Error: --copy argument %#v doesn't have a leading period. See --help for more details.\n", c)
			exit(2)
			return
		}
	}

	// --- Make converter --- //

	var opts []kepub.ConverterOption
	for _, v := range *css {
		opts = append(opts, kepub.ConverterOptionAddCSS(v))
	}
	if *hyphenate {
		opts = append(opts, kepub.ConverterOptionHyphenate(true))
	} else if *nohyphenate {
		opts = append(opts, kepub.ConverterOptionHyphenate(false))
	}
	if *smartenpunct {
		opts = append(opts, kepub.ConverterOptionSmartypants())
	}
	if *fullscreenfixes {
		opts = append(opts, kepub.ConverterOptionFullScreenFixes())
	}
	if *adddummytitlepage {
		opts = append(opts, kepub.ConverterOptionDummyTitlepage(true))
	} else if *noadddummytitlepage {
		opts = append(opts, kepub.ConverterOptionDummyTitlepage(false))
	}
	if *leaveaudio {
		opts = append(opts, kepub.ConverterOptionLeaveAudio(true))
	}
	for _, r := range *replace {
		spl := strings.SplitN(r, "|", 2)
		if len(spl) != 2 {
			fmt.Fprintf(os.Stderr, "Error: Parse replacement %#v: must be in format `find|replace`\n", r)
			exit(1)
			return
		}
		opts = append(opts, kepub.ConverterOptionFindReplace(spl[0], spl[1]))
	}
	opts = append(opts, kepub.ConverterOptionCharset(*charset))
	converter := kepub.NewConverterWithOptions(opts...)

	// --- Transform paths --- //

	ext := ".kepub.epub"
	if *calibre {
		ext = ".kepub"
	}

	pathMap, skipList, err := transformer{
		NoPreserveDirs:   *nopreservedirs,
		Update:           *update,
		Inplace:          *inplace,
		Suffixes:         []string{".epub"},
		ExcludeSuffixes:  []string{".kepub.epub"},
		PreserveSuffixes: *copy,
		TargetSuffix:     ext,
	}.TransformPaths(*output, pflag.Args()...)

	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		exit(1)
		return
	}

	// --- Convert --- //

	fmt.Printf("Kepubify %s\n", version)
	if *calibre {
		fmt.Printf("Using extension %s for Calibre compatibility (this is meant for use with a Calibre library and will not work directly on a Kobo reader)\n", ext)
	}
	fmt.Printf("\n")

	var inputs []string
	for key := range pathMap {
		inputs = append(inputs, key)
	}
	inputs = append(inputs, skipList...)
	sort.Strings(inputs)

	// --- Conversion Pipeline --- //
	var cur int64                                 // progress
	var converted, copied, skipped, errored int64 // counters (sum == total)
	var errs sync.Map                             // input -> error
	total := int64(len(pathMap) + len(skipList))  // immutable

	var allWg sync.WaitGroup

	// logging

	type loge struct {
		stderr bool
		msg    string
	}
	logCh := make(chan loge)
	log := func(stderr bool, format string, a ...interface{}) {
		logCh <- loge{stderr, fmt.Sprintf(format, a...)}
	}

	allWg.Add(1)
	go func() {
		defer allWg.Done()
		for l := range logCh {
			if l.stderr {
				fmt.Fprint(os.Stderr, l.msg)
			} else {
				fmt.Print(l.msg)
			}
		}
	}()

	// tasks

	inCh := make(chan [2]string)

	allWg.Add(1)
	go func() {
		defer allWg.Done()
		defer close(inCh)
		for _, input := range inputs {
			output, ok := pathMap[input]
			if !ok {
				atomic.AddInt64(&cur, 1)
				atomic.AddInt64(&skipped, 1)
				log(false, "[%3d/%3d] Skipping %s\n", atomic.LoadInt64(&cur), total, input)
				continue
			}
			input := input // copy
			inCh <- [2]string{input, output}
		}
	}()

	// processing

	allWg.Add(1)
	go func() {
		defer allWg.Done()
		defer close(logCh)

		var workerWg sync.WaitGroup
		defer workerWg.Wait()

		for i := 0; i < runtime.NumCPU(); i++ {
			workerWg.Add(1)
			go func() {
				defer workerWg.Done()
				for t := range inCh {
					var i int64
					for {
						if tmp := atomic.LoadInt64(&cur); atomic.CompareAndSwapInt64(&cur, tmp, tmp+1) {
							i = tmp + 1
							break
						}
					}
					input, output := t[0], t[1]
					switch {
					case !strings.HasSuffix(output, ext):
						if !*verbose {
							log(false, "[%3d/%3d] Copying %s\n", i, total, input)
						} else {
							log(false, "[%3d/%3d] Copying %s\n          => %s\n", atomic.LoadInt64(&cur), total, input, output)
						}
						if err := os.MkdirAll(filepath.Dir(output), 0755); err != nil {
							errs.Store(input, err)
							atomic.AddInt64(&errored, 1)
							log(true, "          Error (%d): %v\n", i, err)
							continue
						}
						if err := copyFile(input, output); err != nil {
							errs.Store(input, err)
							atomic.AddInt64(&errored, 1)
							log(true, "          Error (%d): %v\n", i, err)
							continue
						}
						atomic.AddInt64(&copied, 1)
					default:
						if !*verbose {
							log(false, "[%3d/%3d] Converting %s\n", i, total, input)
						} else {
							log(false, "[%3d/%3d] Converting %s\n          => %s\n", atomic.LoadInt64(&cur), total, input, output)
						}
						if err := os.MkdirAll(filepath.Dir(output), 0755); err != nil {
							errs.Store(input, err)
							atomic.AddInt64(&errored, 1)
							log(true, "          Error (%d): %v\n", i, err)
							continue
						}
						if err := func() error {
							fi, err := zip.OpenReader(input)
							if err != nil {
								return err
							}
							defer fi.Close()

							fo, err := os.CreateTemp(filepath.Dir(output), ".kepubify."+filepath.Base(output)+".*")
							if err != nil {
								return err
							}
							defer os.Remove(fo.Name())

							if err := converter.Convert(context.Background(), fo, fi); err != nil {
								return err
							}

							if err := fo.Sync(); err != nil {
								return err
							}

							if err := fo.Close(); err != nil {
								return err
							}

							if err := os.Rename(fo.Name(), output); err != nil {
								return err
							}

							return nil
						}(); err != nil {
							errs.Store(input, err)
							atomic.AddInt64(&errored, 1)
							log(true, "          Error (%d): %v\n", i, err)
							continue
						}
						atomic.AddInt64(&converted, 1)
					}
				}
			}()
		}
	}()

	allWg.Wait()

	fmt.Printf("\n%d total: %d converted, %d copied, %d skipped, %d errored\n", total, converted, copied, skipped, errored)

	var tmp bool
	errs.Range(func(input, err interface{}) bool {
		if !tmp {
			fmt.Fprintf(os.Stderr, "\nErrors:\n")
			tmp = true
		}
		fmt.Fprintf(os.Stderr, "  %#v\n  => %#v\n  Error: %v\n\n", input, pathMap[input.(string)], err)
		return true
	})
	if tmp {
		exit(1)
		return
	}
	exit(0)
}

func helpExit() {
	fmt.Fprintf(os.Stderr, "Usage: kepubify [options] input_path [input_path]...\n")
	fmt.Fprintf(os.Stderr, "\nVersion:\n  kepubify %s\n", version)

	categories := map[string]*pflag.FlagSet{}
	pflag.VisitAll(func(flag *pflag.Flag) {
		category := flag.Annotations["category"][0] // this will panic if the category is not set, which is intended
		if _, ok := categories[category]; !ok {
			categories[category] = pflag.NewFlagSet("tmp", pflag.ExitOnError)
			categories[category].SortFlags = false
		}
		categories[category].AddFlag(flag)
	})

	var categoriesSort []string
	for category := range categories {
		categoriesSort = append(categoriesSort, category)
	}
	sort.Strings(categoriesSort)

	for _, category := range categoriesSort {
		fmt.Fprintf(os.Stderr, "\n%s:\n%s", strings.Split(category, ".")[1], categories[category].FlagUsagesWrapped(160))
	}

	fmt.Fprintf(os.Stderr, "\nLinks:\n")
	for _, v := range [][]string{
		{"Website", "https://pgaskin.net/kepubify"},
		{"Source Code", "https://github.com/pgaskin/kepubify"},
		{"Bugs/Support", "https://github.com/pgaskin/kepubify/issues"},
		{"MobileRead", "http://mr.gd/forums/showthread.php?t=295287"},
	} {
		fmt.Fprintf(os.Stderr, "  %-12s - %s\n", v[0], v[1])
	}

	exit(0)
}

func exit(status int) {
	if runtime.GOOS == "windows" {
		time.Sleep(time.Second * 2)
	}
	os.Exit(status)
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.OpenFile(dst, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer out.Close()

	if _, err = io.Copy(out, in); err != nil {
		return err
	}
	return out.Close() // note: this runs before the deferred Close, so errors will work correctly
}
