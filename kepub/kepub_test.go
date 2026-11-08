package kepub

import "testing"

func TestFixIndent(t *testing.T) {
	for _, tc := range []struct {
		indent  string
		newline bool
		in      string
		want    string
	}{
		{"    ", true, "\n\t\ta {\n\t\t\tb;\n\n\t\t\t\n\t\t}\n\t\t", "a {\n    b;\n\n\n}\n"},
		{"    ", false, "\n\t\ta {\n\t\t\tb;\n\t\t}\n\t\t", "a {\n    b;\n}"},
		{"  ", false, "\n\ta\n\t\t\tb  c\n\t", "a\n    b  c"},
		{"\t", true, "\n\ta\n\t\tb\n\t", "a\n\tb\n"},
		{"    ", false, "\na\n\tb\n", "a\n    b"},
		{"    ", false, "\n", ""},
		{"    ", false, "\n\t\t", ""},
		{"    ", true, "\n\n\t\t", "\n"},
		{"    ", true, "\n\t\ta {\n\t\t\tb;\n\t\t}\n\t", "a {\n    b;\n}\n"},
		{"    ", false, "\n\t\ta\n\t\t\tb\n", "a\n    b"},
	} {
		if got := fixIndent(tc.indent, tc.newline, tc.in); got != tc.want {
			t.Errorf("fixIndent(%q, %v, %q) = %q, want %q", tc.indent, tc.newline, tc.in, got, tc.want)
		}
	}
	for _, in := range []string{"a\n\t", "\n\ta\n\t\t"} {
		func() {
			defer func() {
				if recover() == nil {
					t.Errorf("fixIndent(%q) didn't panic", in)
				}
			}()
			fixIndent("    ", true, in)
		}()
	}
}
