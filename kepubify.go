package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/geek1011/kepubify/v3/kepub"
	"github.com/spf13/pflag"
)

var version = "v3-dev"

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
	replace := pflag.StringArrayP("replace", "r", nil, "Find and replace on all html files (repeat any number of times) (format: find|replace)")

	for _, flag := range []string{"smarten-punctuation", "css", "hyphenate", "no-hyphenate", "fullscreen-reading-fixes", "replace"} {
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

	kepub.Verbose = *verbose

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
	for _, r := range *replace {
		spl := strings.SplitN(r, "|", 2)
		if len(spl) != 2 {
			fmt.Fprintf(os.Stderr, "Error: Parse replacement %#v: must be in format `find|replace`\n", r)
			exit(1)
		}
		opts = append(opts, kepub.ConverterOptionFindReplace(spl[0], spl[1]))
	}
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

	var converted, copied, skipped, errored int
	errs := map[string]error{}
	for i, input := range inputs {
		output, ok := pathMap[input]
		if !ok {
			fmt.Printf("[%3d/%3d] Skipping %s\n", i+1, len(pathMap)+len(skipList), input)
			skipped++
			continue
		}

		switch {
		case !strings.HasSuffix(output, ext):
			fmt.Printf("[%3d/%3d] Copying %s\n", i+1, len(pathMap)+len(skipList), input)
			if *verbose {
				fmt.Printf("          => %s\n", output)
			}
			if err := os.MkdirAll(filepath.Dir(output), 0755); err != nil {
				fmt.Fprintf(os.Stderr, "          Error: %v\n", err)
				errs[input] = err
				errored++
				continue
			}
			if err := copyFile(input, output); err != nil {
				fmt.Fprintf(os.Stderr, "          Error: %v\n", err)
				errs[input] = err
				errored++
				continue
			}
			copied++
		default:
			fmt.Printf("[%3d/%3d] Converting %s\n", i+1, len(pathMap)+len(skipList), input)
			if *verbose {
				fmt.Printf("          => %s\n", output)
			}
			if err := os.MkdirAll(filepath.Dir(output), 0755); err != nil {
				fmt.Fprintf(os.Stderr, "          Error: %v\n", err)
				errs[input] = err
				errored++
				continue
			}
			if err := converter.ConvertEPUB(input, output); err != nil {
				fmt.Fprintf(os.Stderr, "          Error: %v\n", err)
				errs[input] = err
				errored++
				continue
			}
			converted++
		}
	}

	fmt.Printf("\n%d total: %d converted, %d copied, %d skipped, %d errored\n", len(pathMap)+len(skipList), converted, copied, skipped, errored)

	if len(errs) > 0 {
		fmt.Fprintf(os.Stderr, "\nErrors:\n")
		for _, input := range inputs {
			fmt.Fprintf(os.Stderr, "  %#v\n  => %#v\n  Error: %v\n\n", input, pathMap[input], err)
		}
		exit(1)
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

	// TODO: examples?

	fmt.Fprintf(os.Stderr, "\nLinks:\n")
	for _, v := range [][]string{
		{"Website", "https://pgaskin.net/kepubify"},
		{"Source Code", "https://github.com/geek1011/kepubify"},
		{"Bugs/Support", "https://github.com/geek1011/kepubify/issues"},
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
