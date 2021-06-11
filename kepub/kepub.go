// Package kepub converts EPUBs to KEPUBs.
package kepub

// Converter converts EPUB2/EPUB3 books to Kobo's KEPUB format.
type Converter struct {
	// extra css
	extraCSS      []string
	extraCSSClass []string

	// smart punctuation
	smartypants bool

	// find/replace in raw html output (note: inefficient, but more efficient
	// than working with strings)
	find    [][]byte
	replace [][]byte
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

// ConverterOptionFindReplace replaces a raw string in the transformed HTML
// (note that this impacts performance since it requires an additional temporary
// buffer to be created for each document).
func ConverterOptionFindReplace(find, replace string) ConverterOption {
	return func(c *Converter) {
		c.find = append(c.find, []byte(find))
		c.replace = append(c.replace, []byte(replace))
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
