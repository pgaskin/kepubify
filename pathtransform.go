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

	// entire first matched suffix replaced
	TargetSuffix string
}

// TransformPaths transforms the input paths into the output dir. See the test
// cases for more details. All inputs must exist, but this may or may not be
// checked. Output should be left blank if not specified by the user.
func (t transformer) TransformPaths(output string, inputs ...string) (map[string]string, []string, error) {
	oneInput := len(inputs) == 1

	matchingInputFiles := map[string][]string{}
	matchingInputRelFilesNoSuffix := map[string][]string{}
	fileIsDir := map[string]bool{}
nextInput:
	for _, input := range inputs {
		if len(matchingInputFiles[input]) != 0 {
			continue nextInput // already seen
		}

		inputInfo, err := os.Stat(input)
		if err != nil {
			return nil, nil, fmt.Errorf("scan input %#v: %w", input, err)
		}

		fileIsDir[input] = inputInfo.IsDir()
		fileIsDir[filepath.Clean(input)] = inputInfo.IsDir()

		if !inputInfo.IsDir() {
			for _, suffix := range t.ExcludeSuffixes {
				if hasSuffixFold(input, suffix) {
					return nil, nil, fmt.Errorf("invalid extension %#v for input file %#v", suffix, input)
				}
			}
			for _, suffix := range t.Suffixes {
				if hasSuffixFold(input, suffix) {
					path := filepath.Clean(input)
					matchingInputFiles[input] = append(matchingInputFiles[input], path)
					matchingInputRelFilesNoSuffix[input] = append(matchingInputRelFilesNoSuffix[input], path[:len(path)-len(suffix)])
					continue nextInput
				}
			}
			return nil, nil, fmt.Errorf("invalid extension %#v for input file %#v", filepath.Ext(input), input)
		}

		if err := filepath.Walk(input, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err // passthrough errors
			}

			fileIsDir[path] = info.IsDir()
			fileIsDir[filepath.Clean(path)] = info.IsDir()

			if info.IsDir() {
				return nil // skip non-files
			}

			path = filepath.Clean(path)

			for _, suffix := range t.ExcludeSuffixes {
				if hasSuffixFold(path, suffix) {
					return nil // skip
				}
			}

			for _, suffix := range t.Suffixes {
				if hasSuffixFold(path, suffix) {
					matchingInputFiles[input] = append(matchingInputFiles[input], path)

					rel, err := filepath.Rel(input, path)
					if err != nil {
						return err
					}

					matchingInputRelFilesNoSuffix[input] = append(matchingInputRelFilesNoSuffix[input], rel[:len(rel)-len(suffix)])
					return nil // next
				}
			}

			return nil // skip
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
	}

	pathMap := map[string]string{}
	for input, matchingFiles := range matchingInputFiles {
		for i, matchingFile := range matchingFiles {
			target := matchingInputRelFilesNoSuffix[input][i]

			if t.NoPreserveDirs || !fileIsDir[input] {
				target = filepath.Base(target)
			}

			if t.Inplace {
				if fileIsDir[input] {
					if outputProvided {
						target = filepath.Join(filepath.Base(filepath.Clean(input)), target)
					} else {
						target = filepath.Join(filepath.Clean(input), target)
					}
				}
			} else {
				if fileIsDir[input] {
					if !t.NoPreserveDirs || oneInput {
						target = filepath.Join(filepath.Base(filepath.Clean(input))+"_converted", target)
					}
				} else {
					target += "_converted"
				}
			}

			target += t.TargetSuffix

			if outputProvided {
				if oneInput {
					if fileIsDir[input] {
						spl := strings.Split(target, string(os.PathSeparator))
						spl[0] = output // dir1_converted/whatever.kepub => output/whatever.kepub
						target = filepath.Join(spl...)
					} else if (outputAccessible && outputIsDir) || strings.HasSuffix(output, string(os.PathSeparator)) {
						target = filepath.Join(output, target) // whatever_converted.kepub.epub => output/whatever_converted.kepub.epub
					} else {
						target = output // whatever_converted.kepub.epub => output.kepub.epub
					}
				} else {
					target = filepath.Join(output, target)
				}
			}

			if _, ok := pathMap[matchingFile]; ok {
				return nil, nil, fmt.Errorf("%#v from input %#v included in more than one input (did you have one input as the parent directory of another?)", matchingFile, input)
			}
			pathMap[matchingFile] = target
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

func hasSuffixFold(s, suffix string) bool {
	if len(suffix) > len(s) {
		return false
	}
	return strings.EqualFold(s[len(s)-len(suffix):], suffix)
}
