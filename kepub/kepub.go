// Package kepub converts EPUBs to KEPUBs.
package kepub

import (
	"context"
	"math"
)

// Converter converts EPUB2/EPUB3 books to Kobo's KEPUB format.
type Converter struct {
	// extra css
	extraCSS      []string
	extraCSSClass []string

	// smart punctuation
	smartypants bool

	// find/replace in raw html output
	find    [][]byte
	replace [][]byte

	// titlepage fix
	dummyTitlepageForce      bool
	dummyTitlepageForceValue bool

	// charset override
	charset string // "auto" for auto-detection
}

// ConverterOption configures a Converter.
type ConverterOption func(*Converter)

// NewConverter creates a new Converter. By default, no options are applied.
func NewConverter() *Converter {
	return NewConverterWithOptions()
}

// NewConverterWithOptions is like NewConverter, with options.
func NewConverterWithOptions(opts ...ConverterOption) *Converter {
	c := new(Converter)
	for _, f := range opts {
		f(c)
	}
	return c
}

// ConverterOptionSmartypants enables smart punctuation.
func ConverterOptionSmartypants() ConverterOption {
	return func(c *Converter) {
		c.smartypants = true
	}
}

// ConverterOptionFindReplace replaces a raw string in the transformed HTML.
func ConverterOptionFindReplace(find, replace string) ConverterOption {
	return func(c *Converter) {
		c.find = append(c.find, []byte(find))
		c.replace = append(c.replace, []byte(replace))
	}
}

// ConverterOptionDummyTitlepage force-enables or force-disables the fix which
// adds a dummy titlepage to the start of the book to fix layout issues on
// certain books. If not set, a heuristic is used to determine whether it should
// be added.
func ConverterOptionDummyTitlepage(add bool) ConverterOption {
	return func(c *Converter) {
		c.dummyTitlepageForce = true
		c.dummyTitlepageForceValue = add
	}
}

// ConverterOptionAddCSS adds CSS code to a book.
func ConverterOptionAddCSS(css string) ConverterOption {
	return converterOptionAddCSS("kepubify-extracss", css)
}

// ConverterOptionHyphenate force-enables or force-disables hyphenation. If not
// set, no specific state is enforced by kepubify.
func ConverterOptionHyphenate(hyphenate bool) ConverterOption {
	if hyphenate {
		return converterOptionAddCSS("kepubify-hyphenate", cssHyphenate)
	}
	return converterOptionAddCSS("kepubify-nohyphenate", cssNoHyphenate)
}

// ConverterOptionFullScreenFixes applies fullscreen fixes for firmware versions
// older than 4.19.11911.
func ConverterOptionFullScreenFixes() ConverterOption {
	return converterOptionAddCSS("kepubify-fullscreenfixes", cssFullScreenFixes)
}

// ConverterOptionCharset overrides the charset for all content documents. Use
// "auto" to automatically detect the charset.
func ConverterOptionCharset(charset string) ConverterOption {
	return func(c *Converter) {
		c.charset = charset
	}
}

func converterOptionAddCSS(class, css string) ConverterOption {
	return func(c *Converter) {
		c.extraCSS = append(c.extraCSS, css)
		c.extraCSSClass = append(c.extraCSSClass, class)
	}
}

const cssHyphenate = `* {
    -webkit-hyphens: auto;
    -moz-hyphens: auto;
    hyphens: auto;

    -webkit-hyphenate-limit-after: 3;
    -webkit-hyphenate-limit-before: 3;
    -webkit-hyphenate-limit-lines: 2;
}

h1, h2, h3, h4, h5, h6, td {
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

// These are for use by certain kepubify frontends for progress information
// during conversions. It is not exported for general use, must be imported via
// an unsafe go:linkname directive, and is subject to change.
type (
	progressKey      struct{}
	progressDeltaKey struct{}
)

// withProgress adds a function to be called synchronously as the conversion
// progresses. If delta is in the range [0, 1], the callback will be
// rate-limited to when there is an important change or when the percentage
// changes by more than delta.
func withProgress(ctx context.Context, delta float64, fn func(n, total int)) context.Context {
	ctx = context.WithValue(ctx, progressDeltaKey{}, delta)
	ctx = context.WithValue(ctx, progressKey{}, fn)
	return ctx
}

// ctxProgress creates a rate-limited progress callback for the provided
// context. It returns nil if a callback has not been set.
func ctxProgress(ctx context.Context) func(force bool, n, total int) {
	if v := ctx.Value(progressKey{}); v != nil {
		fn := v.(func(n, total int))
		dt := ctx.Value(progressDeltaKey{}).(float64)

		var pct, lastPct float64
		return func(force bool, n, total int) {
			if dt < 0 || dt > 1 {
				fn(n, total)
				return
			}
			if n == 0 && total == 0 {
				pct = 1
			} else if n == 0 || total == 0 {
				pct = 0
			} else {
				pct = float64(n) / float64(total)
			}
			if force || math.Abs(pct-lastPct) >= dt {
				lastPct = pct
				fn(n, total)
			}
		}
	}
	return nil
}
