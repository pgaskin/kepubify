package kepub

import (
	"regexp"
	"strings"
	"testing"
	"unicode/utf8"
)

var testSentences = []string{
	" ! Lorem ipsum dolor, sit amet. Consectetur adipiscing elit?\n Sed do eiusmod tempor incididunt!?! Ut labore et dolore \"magna aliqua.\". Ut enim ad “minim veniam”, quis nostrud exercitation 'ullamco laboris' nisi ut aliquip ex ea commodo consequat?… Duis aute irure dolor in reprehenderit in voluptate velit’s esse cillum dolore eu fugiat nulla pariatur. Excepteur sint occaecat cupidatat non proident, sunt in culpa qui officia deserunt mollit anim id est laborum.      sdfsdfsdf",
	"Lorem ipsum dolor, sit amet. Consectetur adipiscing elit? Sed do eiusmod tempor incididunt!?! Ut labore et dolore magna aliqua.",
	strings.Repeat("Lorem ipsum dolor sit amet. Consectetur adipiscing elit ut labore et dolore magna aliqua. ", 40),
	"                                ",
	"...       !!!       ???       .'.'.'.'   ",
	"test\u00a0.\u0080.\u00a0.",
	"",
	"🌝. 🌝      🌝.    🌝",
	"!",
	"? ",
	"? ?",
	"?  ",
	"  ?  ",
	" ?'  .",
	" ?'  .   ",
	" ?'  .   \xFF",
	" ?'  .   \xFF .",
	" ?'  .   \xe2\x82\x28\xFF",
	" ?'  .   \xe2\x82\x28\xFF .",
	" ?'  .   .\xe2\x82\x28\xFF",
	" ?'  .   .\xe2\x82\x28\xFF .",
	" ?'  .   .'\xe2\x82\x28\xFF",
	" ?'  .   .'\xe2\x82\x28\xFF .",
	" ?'  .   .'\xe2\x82\x28\xFF.",
}

// splitSentences splits s with sentenceSplitter, passing it in parts of
// ~chunkSize bytes, split between characters like Reader.
func splitSentences(s string, chunkSize int) []string {
	var sp sentenceSplitter
	var r []string
	var cur []byte
	for len(s) != 0 {
		part := []byte(s[:partEnd(s, chunkSize)])
		s = s[len(part):]
		for len(part) != 0 {
			i := sp.next(part)
			if i < 0 {
				cur = append(cur, part...)
				break
			}
			cur = append(cur, part[:i]...)
			r = append(r, string(cur))
			cur = cur[:0]
			part = part[i:]
		}
	}
	return append(r, string(cur))
}

func partEnd(s string, n int) int {
	i := min(n, len(s))
	for i < len(s) && !utf8.RuneStart(s[i]) { // don't split utf8 sequences
		i++
	}
	return i
}

func TestSplitSentences(t *testing.T) {
	for _, s := range testSentences {
		want := splitSentencesRegexp(s)
		for _, n := range []int{1, 2, 3, 7, len(s) + 1} {
			got := splitSentences(s, n)
			if strings.Join(got, "\x00") != strings.Join(want, "\x00") {
				t.Errorf("%q in parts of %d: got %q, want %q", s, n, got, want)
			}
		}
	}
}

func BenchmarkSplitSentences(b *testing.B) {
	b.SetParallelism(1) // for more accurate results
	b.Run("Regexp", func(b *testing.B) {
		for b.Loop() {
			for _, v := range testSentences {
				splitSentencesRegexp(v)
			}
		}
	})
	b.Run("StateMachine", func(b *testing.B) {
		for b.Loop() {
			for _, v := range testSentences {
				splitSentences(v, len(v)+1)
			}
		}
	})
}

var sentenceRe = regexp.MustCompile(`((?ms).*?[\.\!\?]['"”’“…]?\s+)`)

func splitSentencesRegexp(str string) (r []string) {
	if matches := sentenceRe.FindAllStringIndex(str, -1); len(matches) == 0 {
		r = []string{str} // nothing matched, use the whole string
	} else {
		var pos int
		r = make([]string, len(matches))
		for i, match := range matches {
			r[i] = str[pos:match[1]] // end of last match to end of the current one
			pos = match[1]
		}
		if len(str) > pos {
			r = append(r, str[pos:]) // rest of the string, if any
		}
	}
	return
}
