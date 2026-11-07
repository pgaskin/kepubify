package kepub

import "testing"

// TODO: smartypants test cases

func TestSmartypantsParts(t *testing.T) {
	texts := []string{
		`"Test," test test. 'Test's 1/2 test--test--3/4ths... (tm)' "Test!" ''x'' ` + "``y``" + ` . . . (C) (r) 1/4th &amp; <b> "&" a"b'c`,
		`'test test '20s, test 'test' test; test'test'test "quoted 'nested'" -- --- -`,
	}
	for _, text := range texts {
		var whole smartypants
		want := string(whole.Convert(nil, []byte(text), true))
		for n := 1; n < 10; n++ {
			var sp smartypants
			var got []byte
			for s := text; len(s) != 0; {
				part := s[:partEnd(s, n)]
				s = s[len(part):]
				got = sp.Convert(got, []byte(part), s == "")
			}
			if string(got) != want {
				t.Errorf("%q in parts of %d:\ngot  %q\nwant %q", text, n, got, want)
			}
		}
	}
}
