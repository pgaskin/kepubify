// Package kepub converts EPUBs to KEPUBs.
package kepub

import (
	"errors"
	"strconv"
	"strings"
)

// ErrAlreadyKepub is returned by [Convert] when the input is already a KEPUB
// and [Options.ReprocessKepub] is not set. It is returned before anything is
// written to the output writer.
var ErrAlreadyKepub = errors.New("already a kepub")

// Options configures the conversion of EPUB2/EPUB3 books to Kobo's KEPUB
// format. The zero value applies only the mandatory changes.
type Options struct {
	// Charset overrides the charset for all content documents (not including
	// the OPF). If empty or "auto", it is detected from the content (byte order
	// mark, meta element, or XML declaration), falling back to UTF-8 if valid,
	// or windows-1252 otherwise. Otherwise, it is an encoding label (e.g.,
	// "utf-8") which overrides the charset.
	Charset string

	// CSS adds the specified code to new style elements in content documents.
	CSS []string

	// Hyphenate force-enables (except in headings and table cells) or
	// force-disables hyphenation. If nil, it is left to the book/reader
	// styling.
	Hyphenate *bool

	// FullScreenFixes applies fullscreen fixes. This is only needed for
	// firmware versions older than 4.19.11911, and is generally harmless on
	// newer versions.
	FullScreenFixes bool

	// Smartypants enables automatic smart punctuation conversion.
	Smartypants bool

	// Replace replaces raw strings in the transformed HTML, in order (each
	// replacement applies to the output of the previous ones). Replacements
	// with an empty Find are ignored.
	Replace []Replacement

	// DummyTitlepage force-enables or force-disables the fix which adds a
	// dummy titlepage to the start of the book to fix layout issues on
	// certain books. If nil, a heuristic is used to determine whether it
	// should be added.
	DummyTitlepage *bool

	// ReprocessKepub converts input which is already a KEPUB (detected by the
	// Kobo spans in its content documents) instead of returning
	// [ErrAlreadyKepub]. Existing spans are kept as-is rather than re-split,
	// style elements added by a previous conversion are replaced, and the rest
	// of the conversion is the same as usual.
	ReprocessKepub bool

	// DetectCover makes the dummy titlepage show the book's cover image, if
	// one is found, instead of being blank. The cover image is the one
	// declared in the OPF, or an image file named like a cover (e.g.,
	// cover.jpg).
	DetectCover bool

	// Warn, if not nil, is called for content which can't be converted exactly
	// (e.g., due to malformed on ambiguous HTML), with the path of the file in
	// the EPUB (empty if TransformContent is called directly, or if not
	// applicable to any specific file). It may be called concurrently.
	//
	// It is usually okay to ignore this (e.g., if you're doing bulk
	// conversions, don't have a good way to show warnings, or your thing is
	// being used by non-technical users), as it only really affects either
	// extremely malformed documents, or mostly-inconsequential minor things in
	// weirder documents.
	Warn func(name, msg string)

	// Progress, if not nil, is called synchronously by Convert as the
	// conversion progresses. The percentage is from 0 to 1. It is always called
	// at the start, and at the end on success, with 0/1 respectively.
	Progress func(percent float64)
}

// Replacement is a raw string to replace in the transformed HTML.
type Replacement struct {
	Find    string
	Replace string
}

func (opt Options) progress() func(pct float64, force bool) {
	if opt.Progress == nil {
		return func(float64, bool) {}
	}
	last := -1.0
	return func(pct float64, force bool) {
		if pct != last && (force || pct-last >= .01) {
			last = pct
			opt.Progress(pct)
		}
	}
}

// fixIndent removes the indentation from s, which must start with a newline
// (e.g., a raw string literal where the contents start on the line after the
// opening backtick). The indentation removed is based on the common leading
// tabs across lines, and the final line (the closing backtick's) must only
// contain less or equal indentation. Each remaining leading tab is replaced
// with indent, and lines with only tabs become empty lines. If newline is
// false, the final newline is removed.
//
// ONLY USE THIS IN TESTS AND PACKAGE-LEVEL VARIABLES (it panics if the
// requirements aren't met)
func fixIndent(indent string, newline bool, s string) string {
	s, ok := strings.CutPrefix(s, "\n")
	if !ok {
		panic("fixIndent: doesn't start with a newline")
	}
	body := strings.TrimRight(s, "\t")
	closing := len(s) - len(body)
	base := -1
	for line := range strings.Lines(body) {
		if strings.Trim(line, "\t\n") != "" {
			if n := len(line) - len(strings.TrimLeft(line, "\t")); base < 0 || n < base {
				base = n
			}
		}
	}
	if base >= 0 && closing > base {
		panic("fixIndent: too much indentation on final line: " + strconv.Quote(s))
	}
	var b strings.Builder
	for line := range strings.Lines(body) {
		line = strings.TrimSuffix(line, "\n")
		if strings.Trim(line, "\t") != "" {
			rest := line[base:]
			n := len(rest) - len(strings.TrimLeft(rest, "\t"))
			b.WriteString(strings.Repeat(indent, n))
			b.WriteString(rest[n:])
		}
		b.WriteByte('\n')
	}
	if !newline {
		return strings.TrimSuffix(b.String(), "\n")
	}
	return b.String()
}
