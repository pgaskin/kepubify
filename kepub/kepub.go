// Package kepub converts EPUBs to KEPUBs.
package kepub

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

const cssHyphenate = `* {
    -webkit-hyphens: auto;
    -moz-hyphens: auto;
    hyphens: auto;

    -webkit-hyphenate-limit-after: 3;
    -webkit-hyphenate-limit-before: 3;
    -webkit-hyphenate-limit-lines: 2;
}

h1, h2, h3, h4, h5, h6, td,
h1 *, h2 *, h3 *, h4 *, h5 *, h6 *, td * {
    -moz-hyphens: none !important;
    -webkit-hyphens: none !important;
    hyphens: none !important;
}`

const cssNoHyphenate = `* {
    -moz-hyphens: none !important;
    -webkit-hyphens: none !important;
    hyphens: none !important;
}`

const cssFullScreenFixes = `body {
    margin: 0 !important;
    padding: 0 !important;
}

body>div {
    padding-left: 0.2em !important;
    padding-right: 0.2em !important;
}`

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
