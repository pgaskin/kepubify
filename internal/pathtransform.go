package internal

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type Transformer struct {
	NoPreserveDirs bool
	Inplace        bool
	Update         bool // if not set, output files set may already exist

	// Suffixes are the suffixes of files to convert, in order of priority.
	// Suffixes are compared case-insensitively, and the longest matching one
	// wins. When two files have the same output path, the one matched by the
	// earlier suffix is kept and the other is skipped (e.g., book.kepub.epub
	// beside book.epub is the already converted sibling).
	Suffixes []string

	// ExplicitSuffixes are converted like Suffixes, but only for files given
	// directly as inputs. They are skipped in input directories, and ranked
	// after Suffixes.
	ExplicitSuffixes []string

	// PreserveSuffixes are copied with the suffix unchanged, and only included
	// in the output if the path differs from the original file.
	PreserveSuffixes []string

	// TargetSuffix replaces the matched suffix of converted files.
	TargetSuffix string
}

// suffixKind is how a file matching a suffix is handled.
type suffixKind int

const (
	suffixConvert  suffixKind = iota // converted
	suffixExplicit                   // converted if given directly, skipped in directories
	suffixPreserve                   // copied unchanged
)

// matchedFile is a file found at or under an input.
type matchedFile struct {
	path      string // path of the file, under the input path as given
	dir       string // directory of the file relative to the input, "." for a file input
	stem      string // base name without the matched suffix
	matched   string // matched suffix
	rank      int    // priority of the matched suffix (lower is higher), for convert and explicit suffixes
	suffix    string // output suffix
	preserved bool   // copied unchanged rather than converted
}

// TransformPaths transforms the input paths into the output dir. See the test
// cases for more details. All inputs must exist, but this may or may not be
// checked. Output should be left blank if not specified by the user.
//
// It returns the output path for each input file, and the input files skipped
// (existing outputs if Update is set, and files whose output is the same as a
// file matched by a higher priority suffix, see [Transformer.Suffixes]).
func (t Transformer) TransformPaths(output string, inputs ...string) (map[string]string, []string, error) {
	for _, suffix := range t.PreserveSuffixes {
		if strings.EqualFold(suffix, t.TargetSuffix) {
			return nil, nil, fmt.Errorf("preserved suffix %#v overlaps with target suffix %#v", suffix, t.TargetSuffix)
		}
		for _, x := range t.Suffixes {
			if strings.EqualFold(suffix, x) {
				return nil, nil, fmt.Errorf("preserved suffix %#v overlaps with input suffix %#v", suffix, x)
			}
		}
		for _, x := range t.ExplicitSuffixes {
			if strings.EqualFold(suffix, x) {
				return nil, nil, fmt.Errorf("preserved suffix %#v overlaps with input suffix %#v", suffix, x)
			}
		}
	}

	// clean and dedupe the inputs so different spellings of the same path
	// don't count as separate inputs
	var cleanInputs []string
	seenInputs := map[string]bool{}
	for _, input := range inputs {
		input = filepath.Clean(input)
		if !seenInputs[input] {
			seenInputs[input] = true
			cleanInputs = append(cleanInputs, input)
		}
	}
	inputs = cleanInputs
	oneInput := len(inputs) == 1

	matchingInputFiles := map[string][]matchedFile{}
	fileIsDir := map[string]bool{}

	for _, input := range inputs {
		inputInfo, err := os.Stat(input)
		if err != nil {
			return nil, nil, fmt.Errorf("scan input %#v: %w", input, err)
		}
		fileIsDir[input] = inputInfo.IsDir()

		if !inputInfo.IsDir() {
			name := filepath.Base(input)
			suffix, kind, rank, ok := t.matchSuffix(name)
			if !ok {
				return nil, nil, fmt.Errorf("invalid extension %#v for input file %#v", filepath.Ext(input), input)
			}
			f := matchedFile{
				path:      input,
				dir:       ".",
				stem:      name[:len(name)-len(suffix)],
				matched:   suffix,
				rank:      rank,
				suffix:    t.TargetSuffix,
				preserved: kind == suffixPreserve,
			}
			if f.preserved {
				f.suffix = suffix
			}
			matchingInputFiles[input] = append(matchingInputFiles[input], f)
			continue
		}

		// filepath.Walk doesn't follow a symlink given as the root
		root, err := filepath.EvalSymlinks(input)
		if err != nil {
			return nil, nil, fmt.Errorf("scan input %#v: %w", input, err)
		}

		if err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err // abort on unreadable entries
			}
			if info.IsDir() {
				return nil // skip non-files
			}

			suffix, kind, rank, ok := t.matchSuffix(info.Name())
			if !ok || kind == suffixExplicit {
				return nil // skip
			}

			rel, err := filepath.Rel(root, path)
			if err != nil {
				return err
			}

			f := matchedFile{
				path:      filepath.Join(input, rel),
				dir:       filepath.Dir(rel),
				stem:      info.Name()[:len(info.Name())-len(suffix)],
				matched:   suffix,
				rank:      rank,
				suffix:    t.TargetSuffix,
				preserved: kind == suffixPreserve,
			}
			if f.preserved {
				f.suffix = suffix
			}
			matchingInputFiles[input] = append(matchingInputFiles[input], f)
			return nil
		}); err != nil {
			return nil, nil, fmt.Errorf("scan input %#v: %w", input, err)
		}
	}

	var outputProvided, outputAccessible, outputIsDir bool
	if output != "" {
		outputProvided = true
		if fi, err := os.Stat(output); err == nil {
			outputAccessible, outputIsDir = true, fi.IsDir()
		}
		// only a single file input can be written to an output file
		if outputAccessible && !outputIsDir && !(oneInput && !fileIsDir[inputs[0]]) {
			return nil, nil, fmt.Errorf("output %#v is not a directory", output)
		}
	}

	var (
		skipList []string
		pathMap  = map[string]string{}
		byTarget = map[string]matchedFile{}
	)
	for _, input := range inputs {
		files := matchingInputFiles[input]
		if len(files) == 0 {
			continue
		}

		// parent is the directory the files of a directory input go into,
		// relative to the output directory if one is provided
		var parent string
		if fileIsDir[input] {
			switch {
			case t.Inplace && !outputProvided:
				parent = input
			case outputProvided && oneInput:
				parent = "" // the output directory replaces the input directory
			case outputProvided && !t.Inplace && t.NoPreserveDirs:
				parent = "" // flattened directly into the output directory
			default:
				name, err := t.inputName(input)
				if err != nil {
					return nil, nil, err
				}
				if t.Inplace {
					parent = name
				} else {
					parent = name + "_converted"
				}
			}
		}

		for _, f := range files {
			var target string
			if fileIsDir[input] {
				dir := f.dir
				if t.NoPreserveDirs {
					dir = "."
				}
				target = filepath.Join(parent, dir, f.stem+f.suffix)
			} else {
				name := f.stem
				if !t.Inplace && !f.preserved { // don't add _converted to standalone preserved files
					name += "_converted"
				}
				target = name + f.suffix
			}

			if outputProvided {
				if oneInput && !fileIsDir[input] && !((outputAccessible && outputIsDir) || strings.HasSuffix(output, string(os.PathSeparator))) {
					target = output // whatever_converted.kepub.epub => output.kepub.epub
				} else {
					target = filepath.Join(output, target) // whatever_converted.kepub.epub => output/whatever_converted.kepub.epub
				}
			}

			if f.preserved {
				if f.path == target {
					continue // skip if preserved same as original
				} else if ia, err := filepath.Abs(f.path); err != nil {
					return nil, nil, fmt.Errorf("%#v from input %#v: resolve path of preserved input %#v: %w", f.path, input, f.path, err)
				} else if ta, err := filepath.Abs(target); err != nil {
					return nil, nil, fmt.Errorf("%#v from input %#v: resolve path of preserved output %#v: %w", f.path, input, target, err)
				} else if ia == ta {
					continue // skip if preserved same as original
				}
			}

			if _, ok := pathMap[f.path]; ok {
				return nil, nil, fmt.Errorf("%#v from input %#v included in more than one input (did you have one input as the parent directory of another?)", f.path, input)
			}

			if other, ok := byTarget[target]; ok {
				switch {
				case f.preserved || other.preserved || f.rank == other.rank:
					return nil, nil, fmt.Errorf("overlapping output file %#v for %#v and %#v", target, other.path, f.path)
				case f.rank > other.rank:
					skipList = append(skipList, f.path) // lower priority suffix
					continue
				default:
					skipList = append(skipList, other.path) // lower priority suffix
					delete(pathMap, other.path)
				}
			}
			byTarget[target] = f
			pathMap[f.path] = target
		}
	}

	if t.Update {
		for in, out := range pathMap {
			if _, err := os.Stat(out); !os.IsNotExist(err) {
				skipList = append(skipList, in)
				delete(pathMap, in)
			}
		}
	}

	return pathMap, skipList, nil
}

// matchSuffix returns the longest Transformer suffix matching name, how files
// with it are handled, and its priority (the position in Suffixes followed by
// ExplicitSuffixes).
func (t Transformer) matchSuffix(name string) (suffix string, kind suffixKind, rank int, ok bool) {
	var n int
	for i, suffixes := range [][]string{t.Suffixes, t.ExplicitSuffixes, t.PreserveSuffixes} {
		for j, x := range suffixes {
			if len(x) > len(suffix) && hasSuffixFold(name, x) {
				suffix, kind, rank, ok = x, suffixKind(i), n+j, true
			}
		}
		n += len(suffixes)
	}
	return suffix, kind, rank, ok
}

// inputName returns the base name of a cleaned input directory for use in the
// name of its output directory, resolving "." and ".." against the working
// directory.
func (t Transformer) inputName(input string) (string, error) {
	name := filepath.Base(input)
	if name == "." || name == ".." {
		abs, err := filepath.Abs(input)
		if err != nil {
			return "", fmt.Errorf("resolve path of input %#v: %w", input, err)
		}
		name = filepath.Base(abs)
	}
	if name == string(filepath.Separator) {
		return "", fmt.Errorf("cannot derive an output directory name for input %#v, specify an output directory", input)
	}
	return name, nil
}

func hasSuffixFold(s, suffix string) bool {
	if len(suffix) > len(s) {
		return false
	}
	return strings.EqualFold(s[len(s)-len(suffix):], suffix)
}
