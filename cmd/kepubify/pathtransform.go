package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type transformer struct {
	NoPreserveDirs bool
	Inplace        bool
	Update         bool // if not set, output files set may already exist

	// compared case-insensitively
	Suffixes        []string
	ExcludeSuffixes []string
	// suffix not unchanged, only included in output if different from original
	// filename and not matched or excluded above
	PreserveSuffixes []string

	// entire first matched suffix replaced
	TargetSuffix string
}

// matchedFile is a file found at or under an input.
type matchedFile struct {
	path      string // path of the file, under the input path as given
	dir       string // directory of the file relative to the input, "." for a file input
	stem      string // base name without the matched suffix
	suffix    string // output suffix
	preserved bool   // copied unchanged rather than converted
}

// TransformPaths transforms the input paths into the output dir. See the test
// cases for more details. All inputs must exist, but this may or may not be
// checked. Output should be left blank if not specified by the user.
func (t transformer) TransformPaths(output string, inputs ...string) (map[string]string, []string, error) {
	for _, suffix := range t.PreserveSuffixes {
		if strings.EqualFold(suffix, t.TargetSuffix) {
			return nil, nil, fmt.Errorf("preserved suffix %#v overlaps with target suffix %#v", suffix, t.TargetSuffix)
		}
		for _, x := range t.Suffixes {
			if strings.EqualFold(suffix, x) {
				return nil, nil, fmt.Errorf("preserved suffix %#v overlaps with input suffix %#v", suffix, x)
			}
		}
		// we don't check excluded suffixes for convenience
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
			suffix, excluded, preserved, ok := t.matchSuffix(name)
			if !ok {
				return nil, nil, fmt.Errorf("invalid extension %#v for input file %#v", filepath.Ext(input), input)
			}
			if excluded {
				return nil, nil, fmt.Errorf("invalid extension %#v for input file %#v", suffix, input)
			}
			f := matchedFile{
				path:      input,
				dir:       ".",
				stem:      name[:len(name)-len(suffix)],
				suffix:    t.TargetSuffix,
				preserved: preserved,
			}
			if preserved {
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

			suffix, excluded, preserved, ok := t.matchSuffix(info.Name())
			if !ok || excluded {
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
				suffix:    t.TargetSuffix,
				preserved: preserved,
			}
			if preserved {
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

	pathMap := map[string]string{}
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
			pathMap[f.path] = target
		}
	}

	var skipList []string
	seen := map[string]string{}
	for in, out := range pathMap {
		if t.Update {
			if _, err := os.Stat(out); !os.IsNotExist(err) {
				skipList = append(skipList, in)
			}
		}
		if _, ok := seen[out]; ok {
			return nil, nil, fmt.Errorf("overlapping output file %#v for %#v and %#v", out, seen[out], in)
		}
		seen[out] = in
	}
	for _, f := range skipList {
		delete(pathMap, f)
	}

	return pathMap, skipList, nil
}

// matchSuffix returns the first transformer suffix matching name, and whether
// it is an excluded or preserved one.
func (t transformer) matchSuffix(name string) (suffix string, excluded, preserved, ok bool) {
	for _, suffix := range t.ExcludeSuffixes {
		if hasSuffixFold(name, suffix) {
			return suffix, true, false, true
		}
	}
	for _, suffix := range t.Suffixes {
		if hasSuffixFold(name, suffix) {
			return suffix, false, false, true
		}
	}
	for _, suffix := range t.PreserveSuffixes {
		if hasSuffixFold(name, suffix) {
			return suffix, false, true, true
		}
	}
	return "", false, false, false
}

// inputName returns the base name of a cleaned input directory for use in the
// name of its output directory, resolving "." and ".." against the working
// directory.
func (t transformer) inputName(input string) (string, error) {
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
