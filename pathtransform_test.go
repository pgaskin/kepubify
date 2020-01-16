package main

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
)

func TestTransformPaths(t *testing.T) {
	mkTestTransformer := func(t transformer) transformer {
		t.Suffixes = []string{".epub"}
		t.ExcludeSuffixes = []string{".kepub.epub"}
		t.TargetSuffix = ".kepub.epub"
		return t
	}

	transformPathsCase{
		What: "converting a single file should convert into (name of file)_converted.kepub.epub",
		Input: []string{
			"./book.epub",
		},
		Transformer: mkTestTransformer(transformer{}),
		Inputs:      []string{"book.epub"},
		Outputs: []string{
			"book_converted.kepub.epub",
		},
	}.Run(t)

	transformPathsCase{
		What: "converting a single file with an explicit output filename should convert into it",
		Input: []string{
			"./book.epub",
		},
		Transformer: mkTestTransformer(transformer{}),
		Output:      "whatever.kepub.epub",
		Inputs:      []string{"book.epub"},
		Outputs: []string{
			"whatever.kepub.epub",
		},
	}.Run(t)

	transformPathsCase{
		What: "converting a single file with an explicit output filename should convert into it even if the extension is not supported",
		Input: []string{
			"./book.epub",
		},
		Transformer: mkTestTransformer(transformer{}),
		Output:      "whatever.zip",
		Inputs:      []string{"book.epub"},
		Outputs: []string{
			"whatever.zip",
		},
	}.Run(t)

	transformPathsCase{
		What: "converting a single file with an explicit output filename should convert into a file inside it if it exists and is a directory",
		Input: []string{
			"./book.epub",
			"./output/placeholder.txt",
		},
		Transformer: mkTestTransformer(transformer{}),
		Output:      "output",
		Inputs:      []string{"book.epub"},
		Outputs: []string{
			"output/book_converted.kepub.epub",
		},
	}.Run(t)

	transformPathsCase{
		What: "converting a single file with an explicit output filename should work in subdirs and not be affected by the inplace option",
		Input: []string{
			"./book.epub",
			"./output/placeholder.txt",
		},
		Transformer: mkTestTransformer(transformer{
			Inplace: true,
		}),
		Output: "output/whatever.kepub.epub",
		Inputs: []string{"book.epub"},
		Outputs: []string{
			"output/whatever.kepub.epub",
		},
	}.Run(t)

	transformPathsCase{
		What: "converting a single file with an explicit output filename should convert into a file inside it if it exists and is a directory (explicit filename)",
		Input: []string{
			"./book.epub",
			"./output/placeholder.txt",
		},
		Transformer: mkTestTransformer(transformer{}),
		Output:      "output/whatever.zip",
		Inputs:      []string{"book.epub"},
		Outputs: []string{
			"output/whatever.zip",
		},
	}.Run(t)

	transformPathsCase{
		What: "converting a single file with an explicit output filename ending with a slash should treat the output as a dir even if it doesn't exist",
		Input: []string{
			"./book.epub",
		},
		Transformer: mkTestTransformer(transformer{}),
		Output:      "output" + string(os.PathSeparator),
		Inputs:      []string{"book.epub"},
		Outputs: []string{
			"output/book_converted.kepub.epub",
		},
	}.Run(t)

	transformPathsCase{
		What: "converting a file should convert into (name of file)_converted.kepub.epub and skip other files",
		Input: []string{
			"./book.epub",
			"./another.epub",
			"./subdir/book1.epub",
			"./dontconvert.txt",
		},
		Transformer: mkTestTransformer(transformer{}),
		Inputs:      []string{"book.epub", "subdir/book1.epub"},
		Outputs: []string{
			"book_converted.kepub.epub",
			"book1_converted.kepub.epub",
		},
	}.Run(t)

	transformPathsCase{
		What: "converting a file should ignore duplicate inputs",
		Input: []string{
			"./book.epub",
			"./another.epub",
			"./subdir/book1.epub",
			"./dontconvert.txt",
		},
		Transformer: mkTestTransformer(transformer{}),
		Inputs:      []string{"book.epub", "subdir/book1.epub", "book.epub"},
		Outputs: []string{
			"book_converted.kepub.epub",
			"book1_converted.kepub.epub",
		},
	}.Run(t)

	transformPathsCase{
		What: "converting a file should not be affected by nopreservedirs",
		Input: []string{
			"./book.epub",
			"./subdir/book1.epub",
		},
		Transformer: mkTestTransformer(transformer{
			NoPreserveDirs: true,
		}),
		Inputs: []string{"book.epub", "subdir/book1.epub"},
		Outputs: []string{
			"book_converted.kepub.epub",
			"book1_converted.kepub.epub",
		},
	}.Run(t)

	transformPathsCase{
		What: "converting multiple files should convert into the specified output dir",
		Input: []string{
			"./book.epub",
			"./subdir/book1.epub",
		},
		Transformer: mkTestTransformer(transformer{}),
		Inputs:      []string{"book.epub", "subdir/book1.epub"},
		Output:      "./output",
		Outputs: []string{
			"output/book_converted.kepub.epub",
			"output/book1_converted.kepub.epub",
		},
	}.Run(t)

	transformPathsCase{
		What: "converting multiple files should convert into the specified output dir without the _converted suffix if inplace is specified",
		Input: []string{
			"./book.epub",
			"./subdir/book1.epub",
		},
		Transformer: mkTestTransformer(transformer{
			Inplace: true,
		}),
		Inputs: []string{"book.epub", "subdir/book1.epub"},
		Output: "./output",
		Outputs: []string{
			"output/book.kepub.epub",
			"output/book1.kepub.epub",
		},
	}.Run(t)

	transformPathsCase{
		What: "converting a file with the update option should convert into (name of file)_converted.kepub.epub and skip already converted files",
		Input: []string{
			"./book.epub",
			"./another.epub",
			"./subdir/book1.epub",
			"./dontconvert.txt",
			"./book1_converted.kepub.epub",
		},
		Transformer: mkTestTransformer(transformer{
			Update: true,
		}),
		Inputs: []string{"book.epub", "subdir/book1.epub"},
		Outputs: []string{
			"book_converted.kepub.epub",
		},
	}.Run(t)

	transformPathsCase{
		What: "converting a file in place should not add the _converted suffix",
		Input: []string{
			"./book.epub",
		},
		Transformer: mkTestTransformer(transformer{
			Inplace: true,
		}),
		Inputs: []string{"book.epub"},
		Outputs: []string{
			"book.kepub.epub",
		},
	}.Run(t)

	for _, tp := range []string{"txt", "kepub.epub"} {
		transformPathsCase{
			What: "converting a file should error if not a supported or excluded type - " + tp,
			Input: []string{
				"./book.epub",
				"./another.epub",
				"./subdir/book1.epub",
				"./dontconvert." + tp,
			},
			Transformer: mkTestTransformer(transformer{}),
			Inputs:      []string{"book.epub", "dontconvert." + tp},
			ShouldError: true,
		}.Run(t)
	}

	transformPathsCase{
		What: "converting a file should error if output files overlap",
		Input: []string{
			"./book.epub",
			"./another.epub",
			"./subdir/book.epub",
		},
		Transformer: mkTestTransformer(transformer{}),
		Inputs:      []string{"book.epub", "another.epub", "subdir/book.epub"},
		ShouldError: true, // 2x book_converted.kepub.epub
	}.Run(t)

	transformPathsCase{
		What: "converting a file should error if output files overlap even if update is true",
		Input: []string{
			"./book.epub",
			"./another.epub",
			"./subdir/book.epub",
		},
		Transformer: mkTestTransformer(transformer{
			Update: true,
		}),
		Inputs:      []string{"book.epub", "another.epub", "subdir/book.epub"},
		ShouldError: true, // 2x book_converted.kepub.epub
	}.Run(t)

	transformPathsCase{
		What: "converting a file in place should still check for overlaps",
		Input: []string{
			"./book.epub",
			"./subdir/book.epub",
		},
		Transformer: mkTestTransformer(transformer{
			Inplace: true,
		}),
		Inputs:      []string{"book.epub", "subdir/book.epub"},
		ShouldError: true,
	}.Run(t)

	transformPathsCase{
		What: "converting a single dir should convert into (dir name)_converted and preserve subdirs",
		Input: []string{
			"./dir1/book1.epub",
			"./dir1/book2.epub",
			"./dir1/subdir/book3.epub",
			"./dir2/book4.epub",
		},
		Transformer: mkTestTransformer(transformer{}),
		Inputs:      []string{"./dir1"},
		Outputs: []string{
			"dir1_converted/book1.kepub.epub",
			"dir1_converted/book2.kepub.epub",
			"dir1_converted/subdir/book3.kepub.epub",
		},
	}.Run(t)

	transformPathsCase{
		What: "converting a single dir should convert into (dir name)_converted and flatten subdirs if nopreservedirs is specified",
		Input: []string{
			"./dir1/book1.epub",
			"./dir1/book2.epub",
			"./dir1/subdir/book3.epub",
			"./dir2/book4.epub",
		},
		Transformer: mkTestTransformer(transformer{
			NoPreserveDirs: true,
		}),
		Inputs: []string{"./dir1"},
		Outputs: []string{
			"dir1_converted/book1.kepub.epub",
			"dir1_converted/book2.kepub.epub",
			"dir1_converted/book3.kepub.epub",
		},
	}.Run(t)

	transformPathsCase{
		What: "converting a single dir should convert into the specified output dir",
		Input: []string{
			"./dir1/book1.epub",
			"./dir1/book2.epub",
			"./dir1/subdir/book3.epub",
			"./dir2/book4.epub",
		},
		Transformer: mkTestTransformer(transformer{}),
		Output:      "dir3",
		Inputs:      []string{"./dir1"},
		Outputs: []string{
			"dir3/book1.kepub.epub",
			"dir3/book2.kepub.epub",
			"dir3/subdir/book3.kepub.epub",
		},
	}.Run(t)

	transformPathsCase{
		What: "converting a single dir should convert into the specified output dir and flatten subdirs if nopreservedirs is specified",
		Input: []string{
			"./dir1/book1.epub",
			"./dir1/book2.epub",
			"./dir1/subdir/book3.epub",
			"./dir2/book4.epub",
		},
		Transformer: mkTestTransformer(transformer{
			NoPreserveDirs: true,
		}),
		Output: "dir3",
		Inputs: []string{"./dir1"},
		Outputs: []string{
			"dir3/book1.kepub.epub",
			"dir3/book2.kepub.epub",
			"dir3/book3.kepub.epub",
		},
	}.Run(t)

	transformPathsCase{
		What: "converting a dir inplace with no explicit output path should convert back into the original location",
		Input: []string{
			"./dir1/subdir/book1.epub",
			"./dir1/subdir/book2.epub",
			"./dir1/subdir/another/book3.epub",
			"./dir2/subdir/another/one/book4.epub",
		},
		Transformer: mkTestTransformer(transformer{
			Inplace: true,
		}),
		Inputs: []string{"./dir1/subdir", "./dir2/subdir"},
		Outputs: []string{
			"dir1/subdir/book1.kepub.epub",
			"dir1/subdir/book2.kepub.epub",
			"dir1/subdir/another/book3.kepub.epub",
			"dir2/subdir/another/one/book4.kepub.epub",
		},
	}.Run(t)

	transformPathsCase{
		What: "converting multiple dirs should convert into subdirs of the specified output dir",
		Input: []string{
			"./dir1/book1.epub",
			"./dir1/book2.epub",
			"./dir1/subdir/book3.epub",
			"./dir2/book4.epub",
		},
		Transformer: mkTestTransformer(transformer{}),
		Output:      "dir3",
		Inputs:      []string{"./dir1", "./dir2"},
		Outputs: []string{
			"dir3/dir1_converted/book1.kepub.epub",
			"dir3/dir1_converted/book2.kepub.epub",
			"dir3/dir1_converted/subdir/book3.kepub.epub",
			"dir3/dir2_converted/book4.kepub.epub",
		},
	}.Run(t)

	transformPathsCase{
		What: "converting multiple dirs should convert into subdirs of the specified output dir and flatten it if nopreservedirs is specified",
		Input: []string{
			"./dir1/book1.epub",
			"./dir1/book2.epub",
			"./dir1/subdir/book3.epub",
			"./dir2/book4.epub",
		},
		Transformer: mkTestTransformer(transformer{
			NoPreserveDirs: true,
		}),
		Output: "dir3",
		Inputs: []string{"./dir1", "./dir2"},
		Outputs: []string{
			"dir3/book1.kepub.epub",
			"dir3/book2.kepub.epub",
			"dir3/book3.kepub.epub",
			"dir3/book4.kepub.epub",
		},
	}.Run(t)

	transformPathsCase{
		What: "converting multiple dirs should convert into subdirs of the specified output dir and flatten it if nopreservedirs is specified while still checking for overlapping files",
		Input: []string{
			"./dir1/book1.epub",
			"./dir1/book2.epub",
			"./dir1/subdir/book3.epub",
			"./dir2/book3.epub",
		},
		Transformer: mkTestTransformer(transformer{
			NoPreserveDirs: true,
		}),
		Output:      "dir3",
		Inputs:      []string{"./dir1", "./dir2"},
		ShouldError: true,
	}.Run(t)

	transformPathsCase{
		What: "converting multiple dirs and files should work properly",
		Input: []string{
			"./dir1/book1.epub",
			"./dir1/book2.epub",
			"./dir1/subdir/book3.epub",
			"./dir2/book4.epub",
			"./book5.epub",
			"./dir3/subdir/book6.epub",
			"./dir4/subdir/another/book7.epub",
			"./dir4/subdir/another/one/book8.epub",
		},
		Transformer: mkTestTransformer(transformer{}),
		Inputs:      []string{"./dir1", "./dir2", "./book5.epub", "./dir3/subdir/book6.epub", "./dir4"},
		Outputs: []string{
			"dir1_converted/book1.kepub.epub",
			"dir1_converted/book2.kepub.epub",
			"dir1_converted/subdir/book3.kepub.epub",
			"dir2_converted/book4.kepub.epub",
			"book5_converted.kepub.epub",
			"book6_converted.kepub.epub",
			"dir4_converted/subdir/another/book7.kepub.epub",
			"dir4_converted/subdir/another/one/book8.kepub.epub",
		},
	}.Run(t)

	transformPathsCase{
		What: "converting multiple dirs and files should convert into the specified output dir",
		Input: []string{
			"./dir1/book1.epub",
			"./dir1/book2.epub",
			"./dir1/subdir/book3.epub",
			"./dir2/book4.epub",
			"./book5.epub",
		},
		Transformer: mkTestTransformer(transformer{}),
		Output:      "dir3",
		Inputs:      []string{"./dir1", "./dir2", "./book5.epub"},
		Outputs: []string{
			"dir3/dir1_converted/book1.kepub.epub",
			"dir3/dir1_converted/book2.kepub.epub",
			"dir3/dir1_converted/subdir/book3.kepub.epub",
			"dir3/dir2_converted/book4.kepub.epub",
			"dir3/book5_converted.kepub.epub",
		},
	}.Run(t)

	transformPathsCase{
		What: "converting multiple dirs and files should work properly with inplace",
		Input: []string{
			"./dir1/book1.epub",
			"./dir1/book2.epub",
			"./dir1/subdir/book3.epub",
			"./dir2/book4.epub",
			"./book5.epub",
		},
		Transformer: mkTestTransformer(transformer{
			Inplace: true,
		}),
		Inputs: []string{"./dir1", "./dir2", "./book5.epub"},
		Outputs: []string{
			"dir1/book1.kepub.epub",
			"dir1/book2.kepub.epub",
			"dir1/subdir/book3.kepub.epub",
			"dir2/book4.kepub.epub",
			"book5.kepub.epub",
		},
	}.Run(t)

	transformPathsCase{
		What: "converting nested dirs should fail",
		Input: []string{
			"./dir1/book1.epub",
			"./dir1/book2.epub",
			"./dir1/subdir/book3.epub",
			"./dir1/subdir/book4.epub",
		},
		Transformer: mkTestTransformer(transformer{
			Inplace: true,
		}),
		Inputs:      []string{"./dir1", "./dir1/subdir"},
		ShouldError: true,
	}.Run(t)

	// TODO: more mixed tests
}

func TestHasSuffixFold(t *testing.T) {
	for _, tc := range []struct {
		String, Suffix string
		HasSuffix      bool
	}{
		{"", "", true},
		{"", "test", false},
		{"test", "", true},
		{"test", "test", true},
		{"tESt", "TesT", true},
		{"asdfgh", "FGH", true},
	} {
		t.Logf("hasSuffixFold(%#v, %#v) ?= %#v", tc.String, tc.Suffix, tc.HasSuffix)
		if hasSuffixFold(tc.String, tc.Suffix) != tc.HasSuffix {
			t.Errorf("hasSuffixFold(%#v, %#v) != %#v", tc.String, tc.Suffix, tc.HasSuffix)
		}
	}
}

type transformPathsCase struct {
	What        string
	Input       []string
	Transformer transformer

	Output string
	Inputs []string

	ShouldError bool
	Outputs     []string
}

func (tc transformPathsCase) Run(t *testing.T) {
	t.Log("\n")
	t.Log("===\n")
	t.Logf("case %#v", tc.What)

	td, err := ioutil.TempDir("", "kepubify-test")
	if err != nil {
		panic(err)
	}
	defer os.RemoveAll(td)

	if cwd, err := os.Getwd(); err != nil {
		panic(err)
	} else {
		defer os.Chdir(cwd)
	}

	if err := os.Chdir(td); err != nil {
		panic(err)
	}

	for _, in := range tc.Input {
		fn := filepath.Join(td, filepath.FromSlash(in))
		if err := os.MkdirAll(filepath.Dir(in), 0755); err != nil {
			panic(err)
		}
		if err := ioutil.WriteFile(fn, []byte(in), 0644); err != nil {
			panic(err)
		}
	}

	pathMap, _, err := tc.Transformer.TransformPaths(tc.Output, tc.Inputs...)
	if err == nil && tc.ShouldError {
		t.Errorf("expected error, got map %#v", pathMap)
		return
	} else if err != nil && !tc.ShouldError {
		t.Errorf("unexpected error: %v", err)
		return
	} else if err != nil && tc.ShouldError {
		t.Log()
		t.Log(tc.KepubifyArgs())
		t.Logf("    Error: %v", err)
		return
	}

	var outputs []string
	for _, output := range pathMap {
		outputs = append(outputs, filepath.ToSlash(output))
	}

	sort.Strings(outputs)
	sort.Strings(tc.Outputs)

	if !reflect.DeepEqual(outputs, tc.Outputs) {
		t.Errorf("unexpected outputs\nexpected:%#v\ngot:%#v", tc.Outputs, outputs)
	}

	t.Log()
	t.Log(tc.KepubifyArgs())

	var ins []string
	for in := range pathMap {
		ins = append(ins, filepath.ToSlash(in))
	}
	for _, other := range tc.Inputs {
		other = filepath.Clean(other)
		if _, ok := pathMap[other]; !ok {
			ins = append(ins, filepath.ToSlash(other))
		}
	}
	sort.Strings(ins)

	for _, in := range ins {
		t.Logf("    %30s => %s", in, pathMap[in])
	}
}

func (tc transformPathsCase) KepubifyArgs() string {
	args := []string{"kepubify"}
	if tc.Output != "" {
		args = append(args, "--output", tc.Output)
	}
	if tc.Transformer.Update {
		args = append(args, "--update")
	}
	if tc.Transformer.Inplace {
		args = append(args, "--inplace")
	}
	if tc.Transformer.NoPreserveDirs {
		args = append(args, "--no-preserve-dirs")
	}
	return strings.Join(append(args, tc.Inputs...), " ")
}
