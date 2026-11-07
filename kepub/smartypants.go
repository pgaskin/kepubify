package kepub

import "bytes"

// smartypants is like github.com/russross/blackfriday@4ca828/smartypants.go,
// but rewritten as a streaming implementation, plus a few tweaks
type smartypants struct {
	inSingleQuote bool
	inDoubleQuote bool
	prev          byte   // escaped byte before p, or 0 at the start
	p             []byte // unconverted escaped text
}

// Convert converts the next chunk of text, appending the result to dst. The
// remaining text is written and the state is reset for the final chunk.
func (s *smartypants) Convert(dst, text []byte, final bool) []byte {
	const lookahead = len("&quot;") + 1
	// note: we escape "&<> as entities internally to match the original impl,
	// but unescape them again when writing
	p := s.escape(s.p, text)
	i := 0
	for i < len(p) {
		if !s.touches(p[i]) {
			j := i + 1
			for j < len(p) && !s.touches(p[j]) {
				j++
			}
			dst = append(dst, p[i:j]...)
			i = j
			continue
		}
		if !final && len(p)-i < lookahead {
			break
		}
		prev := s.prev
		if i > 0 {
			prev = p[i-1]
		}
		var n int
		dst, n = s.convert(dst, prev, p[i:])
		i += n + 1
	}
	if i > 0 {
		s.prev = p[i-1]
	}
	s.p = append(p[:0], p[i:]...)
	if final {
		*s = smartypants{p: s.p[:0]}
	}
	return dst
}

func (s *smartypants) escape(b, x []byte) []byte {
	for _, c := range x {
		switch c {
		case '"':
			b = append(b, "&quot;"...)
		case '&':
			b = append(b, "&amp;"...)
		case '<':
			b = append(b, "&lt;"...)
		case '>':
			b = append(b, "&gt;"...)
		default:
			b = append(b, c)
		}
	}
	return b
}

func (s *smartypants) touches(c byte) bool {
	// note: & includes " and < since they're always escaped
	switch c {
	case '&', '\'', '(', '-', '.', '1', '3', '`':
		return true
	}
	return false
}

// convert converts the start of the escaped text, appending the result
// to dst, and returns the number of bytes consumed after the first one.
func (s *smartypants) convert(dst []byte, prev byte, text []byte) ([]byte, int) {
	switch text[0] {
	case '&':
		return s.entity(dst, prev, text)
	case '\'':
		return s.singleQuote(dst, prev, text)
	case '(':
		return s.parens(dst, text)
	case '-':
		return s.dash(dst, text)
	case '.':
		return s.period(dst, text)
	case '1', '3':
		return s.number(dst, prev, text)
	case '`':
		return s.backtick(dst, prev, text)
	}
	return append(dst, text[0]), 0
}

func (s *smartypants) entity(dst []byte, prev byte, text []byte) ([]byte, int) {
	// note: we unescape amp/lt/gt unlike the original smartypants impl
	switch {
	case bytes.HasPrefix(text, []byte("&quot;")):
		next := byte(0)
		if len(text) >= 7 {
			next = text[6]
		}
		return s.quote(dst, prev, next, 'd', &s.inDoubleQuote), 5
	case bytes.HasPrefix(text, []byte("&amp;")):
		return append(dst, '&'), 4
	case bytes.HasPrefix(text, []byte("&lt;")):
		return append(dst, '<'), 3
	case bytes.HasPrefix(text, []byte("&gt;")):
		return append(dst, '>'), 3
	}
	return append(dst, '&'), 0
}

func (s *smartypants) singleQuote(dst []byte, prev byte, text []byte) ([]byte, int) {
	if len(text) >= 2 {
		t1 := s.toLowerASCII(text[1])
		if t1 == '\'' {
			next := byte(0)
			if len(text) >= 3 {
				next = text[2]
			}
			return s.quote(dst, prev, next, 'd', &s.inDoubleQuote), 1
		}
		if (t1 == 's' || t1 == 't' || t1 == 'm' || t1 == 'd') && (len(text) < 3 || s.isWordBoundary(text[2])) {
			return append(dst, "\u2019"...), 0
		}
		if len(text) >= 3 {
			t2 := s.toLowerASCII(text[2])
			if ((t1 == 'r' && t2 == 'e') || (t1 == 'l' && t2 == 'l') || (t1 == 'v' && t2 == 'e')) &&
				(len(text) < 4 || s.isWordBoundary(text[3])) {
				return append(dst, "\u2019"...), 0
			}
		}
	}
	next := byte(0)
	if len(text) > 1 {
		next = text[1]
	}
	return s.quote(dst, prev, next, 's', &s.inSingleQuote), 0
}

// backtick converts two backticks to a double quote.
func (s *smartypants) backtick(dst []byte, prev byte, text []byte) ([]byte, int) {
	if len(text) >= 2 && text[1] == '`' {
		next := byte(0)
		if len(text) >= 3 {
			next = text[2]
		}
		return s.quote(dst, prev, next, 'd', &s.inDoubleQuote), 1
	}
	// note: we keep a single backtick unlike the original smartypants impl
	return append(dst, '`'), 0
}

func (s *smartypants) parens(dst, text []byte) ([]byte, int) {
	if len(text) >= 3 {
		t1 := s.toLowerASCII(text[1])
		t2 := s.toLowerASCII(text[2])
		if t1 == 'c' && t2 == ')' {
			return append(dst, "\u00a9"...), 2
		}
		if t1 == 'r' && t2 == ')' {
			return append(dst, "\u00ae"...), 2
		}
		if len(text) >= 4 && t1 == 't' && t2 == 'm' && text[3] == ')' {
			return append(dst, "\u2122"...), 3
		}
	}
	return append(dst, '('), 0
}

func (s *smartypants) dash(dst, text []byte) ([]byte, int) {
	if len(text) >= 3 && text[1] == '-' && text[2] == '-' {
		return append(dst, "\u2014"...), 2
	}
	if len(text) >= 2 && text[1] == '-' {
		return append(dst, "\u2013"...), 1
	}
	return append(dst, '-'), 0
}

func (*smartypants) period(dst, text []byte) ([]byte, int) {
	if len(text) >= 3 && text[1] == '.' && text[2] == '.' {
		return append(dst, "\u2026"...), 2
	}
	if len(text) >= 5 && text[1] == ' ' && text[2] == '.' && text[3] == ' ' && text[4] == '.' {
		return append(dst, "\u2026"...), 4
	}
	return append(dst, '.'), 0
}

func (s *smartypants) number(dst []byte, prev byte, text []byte) ([]byte, int) {
	if s.isWordBoundary(prev) && len(text) >= 3 {
		if text[0] == '1' && text[1] == '/' && text[2] == '4' {
			if len(text) < 4 || s.isWordBoundary(text[3]) || (len(text) >= 5 && s.toLowerASCII(text[3]) == 't' && s.toLowerASCII(text[4]) == 'h') {
				return append(dst, "\u00bc"...), 2
			}
		}
		if text[0] == '1' && text[1] == '/' && text[2] == '2' {
			if len(text) < 4 || s.isWordBoundary(text[3]) {
				return append(dst, "\u00bd"...), 2
			}
		}
		if text[0] == '3' && text[1] == '/' && text[2] == '4' {
			if len(text) < 4 || s.isWordBoundary(text[3]) || (len(text) >= 6 && s.toLowerASCII(text[3]) == 't' && s.toLowerASCII(text[4]) == 'h' && s.toLowerASCII(text[5]) == 's') {
				return append(dst, "\u00be"...), 2
			}
		}
	}
	return append(dst, text[0]), 0
}

// quote appends an opening or closing quote based on the bytes before and after
// it (0 at the edges of the text).
func (s *smartypants) quote(dst []byte, prev, next, quote byte, isOpen *bool) []byte {
	switch {
	case prev == 0 && next == 0:
		// context is not any help here, so toggle
		*isOpen = !*isOpen
	case s.isSmartSpace(prev) && next == 0:
		*isOpen = true
	case s.isSmartPunct(prev) && next == 0:
		*isOpen = false
	case next == 0:
		*isOpen = false
	case prev == 0 && s.isSmartSpace(next):
		*isOpen = false
	case s.isSmartSpace(prev) && s.isSmartSpace(next):
		*isOpen = !*isOpen
	case s.isSmartPunct(prev) && s.isSmartSpace(next):
		*isOpen = false
	case s.isSmartSpace(next):
		*isOpen = false
	case prev == 0 && s.isSmartPunct(next):
		*isOpen = false
	case s.isSmartSpace(prev) && s.isSmartPunct(next):
		*isOpen = true
	case s.isSmartPunct(prev) && s.isSmartPunct(next):
		*isOpen = !*isOpen
	case s.isSmartPunct(next):
		*isOpen = false
	case prev == 0:
		*isOpen = true
	case s.isSmartSpace(prev):
		*isOpen = true
	case s.isSmartPunct(prev):
		*isOpen = true
	default:
		// maybe a contraction
		*isOpen = false
	}
	switch {
	case quote == 's' && *isOpen:
		return append(dst, "\u2018"...)
	case quote == 's':
		return append(dst, "\u2019"...)
	case *isOpen:
		return append(dst, "\u201c"...)
	default:
		return append(dst, "\u201d"...)
	}
}

func (s *smartypants) isWordBoundary(c byte) bool {
	return c == 0 || s.isSmartSpace(c) || s.isSmartPunct(c)
}

func (s *smartypants) isSmartSpace(c byte) bool {
	return c == ' ' || c == '\t' || c == '\n' || c == '\r' || c == '\f' || c == '\v'
}

func (s *smartypants) isSmartPunct(c byte) bool {
	return bytes.IndexByte([]byte("!\"#$%&'()*+,-./:;<=>?@[\\]^_`{|}~"), c) >= 0
}

func (s *smartypants) toLowerASCII(c byte) byte {
	if 'A' <= c && c <= 'Z' {
		return c + 'a' - 'A'
	}
	return c
}
