package kepub

import (
	"bytes"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/pgaskin/htmlstream"
	"github.com/pgaskin/htmlstream/htmlutil"
	"github.com/pgaskin/htmlstream/polyglot"
)

// kobo adds the Kobo markup to the output of a [polyglot.Writer].
type kobo struct {
	smartypants bool
	styles      []byte // style elements added to the head

	open    []openElement // open elements in the output
	inBody  bool
	headEnd bool // style elements added
	bufArr  [1024]byte
	buf     []byte // markup written for a text part (bufArr while it fits to reduce unnecessary allocations)

	// Formatting elements with the koboSpan class (while spans are added) or
	// the wrapper div ids (while deciding the wrapper divs), by ID, for copies
	// reopened after a misnested end tag (for which the events don't have end
	// tokens). This is effectively never used since most EPUBs have properly
	// nested tags, and the attributes are on span and div elements.
	flagged []flaggedElement

	// wrapper divs
	hold     int           // 0: not holding, 1: after body start tag, 2: after book-columns start tag
	body     polyglot.Hold // from after the body start tag (zero after an error)
	pendText []byte        // decoded text of the current text node while holding
	wrapped  bool          // wrapper divs added

	spans   int8 // 0: undecided, 1: adding spans, -1: the document already has spans
	para    int
	seg     int
	newPara bool // next span starts a new paragraph

	node   textState // text node being processed
	inText bool      // in the text node
}

// note: we don't need to return the errors for the Hold methods since they're
// sticky and will be returned by WriteToken/Close

// openElement tracks the state of an open element.
type openElement struct {
	p        bool // an HTML p element
	noSpans  bool // don't add spans
	noSmarty bool // don't convert punctuation
	wrap     bool // element is wrapped in a span (i.e., img)
}

// flaggedElement is formatting element we need to identify later.
type flaggedElement struct {
	id    int
	flags uint8
}

const (
	flagKoboSpan    = 1 << iota // class koboSpan
	flagBookColumns             // id book-columns (with stuff within it)
	flagBookInner               // id book-inner
)

var (
	isKoboSpan    = htmlstream.HasClass("koboSpan")
	isBookColumns = htmlstream.AttrEquals("id", "book-columns")
	isBookInner   = htmlstream.AttrEquals("id", "book-inner")
)

const (
	wrapStart = `<div id="book-columns"><div id="book-inner">`
	wrapEnd   = `</div></div>`
)

var (
	wrapEndBytes = []byte(wrapEnd)
	spanEnd      = []byte("</span>")
)

// holdMax is the maximum held markup for transformations (mostly used when
// deciding whether or not to transform something, and if exceeded, only stuff
// up to the limit is considered) (e.g., when removing empty ms word elements,
// checking for existing wrapper divs, skipping spans for whitespace-only
// sentences).
const holdMax = 64 << 10 // KiB

// Based on Kobo (checked with 3 books) as of 2020-01-12. Kobo adds the
// following at the end of the head element:
//
//	<style xmlns="http://www.w3.org/1999/xhtml" type="text/css" id="kobostylehacks">div#book-inner p, div#book-inner div { font-size: 1.0em; } a { color: black; } a:link, a:visited, a:hover, a:active { color: blue; } div#book-inner * { margin-top: 0 !important; margin-bottom: 0 !important;}</style>
//
// We modify it to:
//   - get rid of the font size stuff, as it causes more problems than it solves
//   - get rid of the link colouring, as it's useless on e-ink (remember this same stylesheet is used on the apps and desktop too)
//   - not override all margins
const cssKobo = `div#book-inner { margin-top: 0; margin-bottom: 0;}`

// newKobo returns the state for a document, with the style elements to add
// for opt.
func newKobo(opt Options) *kobo {
	k := &kobo{smartypants: opt.Smartypants}
	k.buf = k.bufArr[:0]
	k.styles = appendStyle(k.styles, "kobostylehacks", cssKobo)
	for _, css := range opt.CSS {
		k.styles = appendStyle(k.styles, "kepubify-extracss", css)
	}
	switch {
	case opt.Hyphenate == nil:
	case *opt.Hyphenate:
		k.styles = appendStyle(k.styles, "kepubify-hyphenate", cssHyphenate)
	default:
		k.styles = appendStyle(k.styles, "kepubify-nohyphenate", cssNoHyphenate)
	}
	if opt.FullScreenFixes {
		k.styles = appendStyle(k.styles, "kepubify-fullscreenfixes", cssFullScreenFixes)
	}
	return k
}

func appendStyle(b []byte, class, css string) []byte {
	// note: technically class isn't allowed on a style element for XHTML, but
	// it's fine
	b = htmlstream.AppendStartTag(b, "style", false, "type", "text/css", "class", class)
	if strings.ContainsAny(css, "<&") {
		// like [polyglot.Writer]
		b = append(b, "/*<![CDATA[*/\n"...)
		b = append(b, strings.ReplaceAll(css, "]]>", "]]]]><![CDATA[>")...)
		b = append(b, "\n/*]]>*/"...)
	} else {
		b = append(b, css...)
	}
	return htmlstream.AppendEndTag(b, "style")
}

// Implements [polyglot.Options.OnTag].
func (k *kobo) OnTag(e *polyglot.Tag) {
	flags := k.flagsOf(e)
	if k.hold != 0 {
		k.holdTag(e, flags)
	}
	if e.Start {
		k.startTag(e, flags)
	} else {
		k.endTag(e)
	}
}

// flagsOf returns the flags of a start tag from its token, or for a reopened
// formatting element, the ones recorded for the element (which matter for
// processing).
func (k *kobo) flagsOf(e *polyglot.Tag) uint8 {
	if !e.Start {
		return 0
	}
	if e.Token == nil {
		if e.Reopened {
			for _, f := range k.flagged {
				if f.id == e.ID {
					return f.flags
				}
			}
		}
		return 0
	}
	var flags uint8
	if k.inBody && isKoboSpan(e.Token) {
		flags |= flagKoboSpan
	}
	if k.hold != 0 {
		if e.Token.Opens() && isBookColumns(e.Token) {
			flags |= flagBookColumns
		}
		if isBookInner(e.Token) {
			flags |= flagBookInner
		}
	}
	if flags != 0 && (k.hold != 0 || k.spans == 1) && htmlutil.IsFormatting(e.Token) {
		k.flagged = append(k.flagged, flaggedElement{e.ID, flags})
	}
	return flags
}

// holdTag decides whether to add the wrapper divs, ending the hold at the
// book-inner start tag within a book-columns, or after adding the divs.
func (k *kobo) holdTag(e *polyglot.Tag, flags uint8) {
	switch {
	case len(e.Writer().Held(k.body)) >= holdMax:
	case k.hold == 1 && flags&flagBookColumns != 0:
		k.hold = 2
		return
	case k.hold == 2 && flags&flagBookInner != 0:
		k.hold = 0
		e.Writer().HoldRelease(k.body)
		return
	}
	e.InsertBefore(k.endHold(e.Writer()))
}

// endHold ends the hold after the body start tag, adding the wrapper divs
// before the held output. It returns the markup to write before the current tag
// or text (the wrapper divs, if nothing is held back).
func (k *kobo) endHold(pw *polyglot.Writer) []byte {
	k.hold = 0
	k.wrapped = true
	if k.body == (polyglot.Hold{}) {
		return []byte(wrapStart)
	}
	pw.HoldDiscard(k.body, append([]byte(wrapStart), pw.Held(k.body)...))
	return nil
}

func (k *kobo) startTag(e *polyglot.Tag, flags uint8) {
	var parent openElement
	if n := len(k.open); n != 0 {
		parent = k.open[n-1]
	}
	f := openElement{noSpans: parent.noSpans, noSmarty: parent.noSmarty}
	html := e.NS == htmlstream.NamespaceXHTML
	name := e.Local
	switch name {
	case "pre", "code", "script", "style":
		f.noSmarty = true
	}
	if k.inBody {
		if flags&flagKoboSpan != 0 {
			switch k.spans {
			case 0:
				k.spans = -1
			case 1:
				f.noSpans = true // don't nest spans
			}
		}
		if !parent.noSpans && k.spans >= 0 {
			// spans are added in the element
			switch name {
			case "script", "style", "pre", "audio", "video", "svg", "math":
				f.noSpans = true
			}
			if html {
				switch name {
				case "img":
					k.newPara = true
					k.buf = k.appendSpanStart(k.buf[:0])
					e.InsertBefore(k.buf)
					f.wrap = true
				case "p", "ol", "ul", "table", "h1", "h2", "h3", "h4", "h5", "h6":
					k.newPara = true
				case "textarea", "title":
					f.noSpans = true // html parsers always read the contents as text
				}
			}
		}
		f.p = html && name == "p"
	} else if html && name == "body" {
		k.inBody = true
		k.hold = 1
		k.body = e.Writer().Hold() // from after the body start tag
	}
	k.open = append(k.open, f)
}

func (k *kobo) endTag(e *polyglot.Tag) {
	var f openElement
	if n := len(k.open); n != 0 {
		f = k.open[n-1]
		k.open = k.open[:n-1]
	}
	if f.wrap {
		e.InsertBefore(spanEnd)
	}
	if e.NS != htmlstream.NamespaceXHTML {
		return
	}
	switch e.Local {
	case "head":
		if !k.headEnd {
			k.headEnd = true
			e.InsertBefore(k.styles)
		}
	case "body":
		if k.inBody {
			k.inBody = false
			if k.wrapped {
				e.InsertBefore(wrapEndBytes)
			}
		}
	}
}

// appendSpanStart appends the start tag of the next span to b.
func (k *kobo) appendSpanStart(b []byte) []byte {
	k.spans = 1
	if k.newPara {
		k.newPara = false
		k.para++
		k.seg = 0
	}
	k.seg++
	b = append(b, `<span class="koboSpan" id="kobo.`...)
	b = strconv.AppendInt(b, int64(k.para), 10)
	b = append(b, '.')
	b = strconv.AppendInt(b, int64(k.seg), 10)
	return append(b, `">`...)
}

// Implements [polyglot.Options.OnText].
func (k *kobo) OnText(t *polyglot.Text) {
	if t.Raw {
		// script and style contents, with the replacement characters
		// removed like in text
		// TODO: drop this?
		if bytes.Contains(t.Data, replacementChar) {
			t.SetText(bytes.ReplaceAll(t.Data, replacementChar, nil))
		}
		return
	}
	k.buf = k.textMarkup(k.buf[:0], t)
	t.SetMarkup(k.buf)
}

// textMarkup appends the markup written for the text part t to dst.
func (k *kobo) textMarkup(dst []byte, t *polyglot.Text) []byte {
	if k.hold != 0 {
		// note: whitespace is held up to holdMax until we get to non-whitespace
		// so we can decide whether to add the divs and process the text
		if !t.Last {
			k.pendText = append(k.pendText, t.Data...)
			if len(k.pendText) <= holdMax && isOnlyUnicodeSpace(t.Data) {
				return dst
			}
		} else if isOnlyUnicodeSpace(k.pendText) {
			dst = polyglot.AppendEscapeText(dst, k.pendText)
			k.pendText = k.pendText[:0]
			return dst
		}
		dst = append(dst, k.endHold(t.Writer())...)
		data := k.pendText
		k.pendText = k.pendText[:0]
		dst = k.textPart(dst, data, true)
		if t.Last {
			return k.textEnd(dst)
		}
		return dst
	}
	if t.Last {
		return k.textEnd(dst)
	}
	return k.textPart(dst, t.Data, t.First)
}

// textPart processes a chunk of a text node. Whether we add spans and convert
// punctuation is decided based on the first chunk (htmlstream splits large
// chunks of text).
func (k *kobo) textPart(dst, data []byte, first bool) []byte {
	if first {
		k.inText = false
		if k.inBody {
			var f openElement
			if n := len(k.open); n != 0 {
				f = k.open[n-1]
			}
			spans := k.spans >= 0 && !f.noSpans
			smarty := k.smartypants && !f.noSmarty
			if spans || smarty {
				k.inText = true
				k.node.Reset(spans, f.p, smarty)
			}
		}
	}
	if !k.inText {
		// remove replacement characters
		if bytes.Contains(data, replacementChar) {
			data = bytes.ReplaceAll(data, replacementChar, nil)
		}
		return polyglot.AppendEscapeText(dst, data)
	}
	return k.node.Process(k, dst, data, false)
}

// textEnd ends a text node.
func (k *kobo) textEnd(dst []byte) []byte {
	if k.inText {
		dst = k.node.Process(k, dst, nil, true)
		k.inText = false
	}
	return dst
}

var replacementChar = []byte("\ufffd")

// textState splits a text node into sentences, wraps them in spans, and
// converts their punctuation.
type textState struct {
	spans  bool // add spans
	p      bool // text is directly in a p element
	smarty bool // convert punctuation

	split    sentenceSplitter
	started  bool   // sentence started
	spanDone bool   // whether we've determined whether the current sentence has a span
	span     bool   // current sentence has a span
	pending  []byte // whitespace at the start of the current sentence (before spanDone)
	sp       smartypants
	smartBuf []byte
}

// Reset starts a text node, reusing the internal buffers.
func (t *textState) Reset(spans, p, smarty bool) {
	t.spans, t.p, t.smarty = spans, p, smarty
	t.split = sentenceSplitter{}
	t.started, t.spanDone, t.span = false, false, false
	t.pending = t.pending[:0]
	t.sp = smartypants{p: t.sp.p[:0]}
}

// Process processes the next chunk of the text (with final true for the last
// chunk), appending the output to dst.
func (t *textState) Process(k *kobo, dst, s []byte, final bool) []byte {
	if !t.spans {
		// the whole text node is one piece
		dst = t.text(dst, s, final)
		return dst
	}
	for len(s) != 0 {
		if !t.started {
			t.started = true
			t.spanDone, t.span = t.p, t.p
			if t.span {
				dst = k.appendSpanStart(dst)
			}
		}
		i := t.split.next(s)
		if i < 0 {
			dst = t.sentence(k, dst, s, false)
			break
		}
		dst = t.sentence(k, dst, s[:i], true)
		t.started = false
		s = s[i:]
	}
	if final && t.started {
		dst = t.sentence(k, dst, nil, true)
		t.started = false
	}
	return dst
}

// sentence processes a chunk of the current sentence (with end true for the
// last chunk).
func (t *textState) sentence(k *kobo, dst, s []byte, end bool) []byte {
	if !t.spanDone {
		if len(t.pending)+len(s) > holdMax || !isOnlyUnicodeSpace(s) {
			t.spanDone, t.span = true, true
			dst = k.appendSpanStart(dst)
			dst = t.text(dst, t.pending, false)
			t.pending = t.pending[:0]
		} else {
			t.pending = append(t.pending, s...)
			if !end {
				return dst
			}
			// whitespace only, without a span
			t.spanDone = true
			s, t.pending = t.pending, t.pending[:0]
			dst = t.text(dst, s, true)
			return dst
		}
	}
	dst = t.text(dst, s, end)
	if end && t.span {
		dst = append(dst, "</span>"...)
	}
	return dst
}

// text processes and emits a chunk of text.
func (t *textState) text(dst, s []byte, end bool) []byte {
	if t.smarty {
		t.smartBuf = t.sp.Convert(t.smartBuf[:0], s, end)
		s = t.smartBuf
	}
	if bytes.Contains(s, replacementChar) {
		s = bytes.ReplaceAll(s, replacementChar, nil)
	}
	return polyglot.AppendEscapeText(dst, s)
}

// isOnlyUnicodeSpace returns true if s consists of only Unicode whitespace.
func isOnlyUnicodeSpace(s []byte) bool {
	for _, r := range string(s) {
		if !unicode.IsSpace(r) {
			return false
		}
	}
	return true
}

// sentenceSplitter finds the ends of sentences with a state machine equivalent
// to the regular expression:
//
//	`((?ms).*?[\.\!\?]['"\u201d\u2019\u201c\u2026]?\s+)`
//
// A sentence ends after punctuation, optionally followed by a quote or
// ellipsis, and ASCII whitespace.
type sentenceSplitter struct {
	state uint8
}

const (
	splitDefault    = iota // in a sentence
	splitPunct             // after sentence-ending punctuation
	splitPunctExtra        // after the quote or ellipsis following it
	splitAfterSpace        // in the whitespace after the sentence
)

// next consumes s up to the start of the next sentence, returning its
// index, or -1 if the sentence doesn't end in s.
func (sp *sentenceSplitter) next(s []byte) int {
	for i := 0; i < len(s); {
		if sp.state == splitDefault {
			// only sentence-ending punctuation changes the state
			j := bytes.IndexAny(s[i:], ".!?")
			if j < 0 {
				return -1
			}
			i += j
		}
		r, n := rune(s[i]), 1
		if r >= utf8.RuneSelf {
			r, n = utf8.DecodeRune(s[i:])
		}
		var punct, extra, space bool
		switch r {
		case '.', '!', '?':
			punct = true
		case '\'', '"', '\u201d', '\u2019', '\u201c', '\u2026':
			extra = true
		case '\t', '\n', '\f', '\r', ' ':
			space = true
		}
		if sp.state == splitAfterSpace && !space {
			// the character starts the next sentence, where it has the
			// same effect as in the default state
			sp.state = splitDefault
			return i
		}
		switch {
		case punct:
			sp.state = splitPunct
		case extra && sp.state == splitPunct:
			sp.state = splitPunctExtra
		case space && sp.state != splitDefault:
			sp.state = splitAfterSpace
		default:
			sp.state = splitDefault
		}
		i += n
	}
	return -1
}
