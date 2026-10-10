package snippets

import (
	"strings"
	"testing"
)

func TestStarts(t *testing.T) {
	cases := []struct {
		src  string
		want string // one character per line: s for a start, . otherwise
	}{
		{"1 + 2", "s"},
		{"   1 + 2", "s"}, // the first item's indentation is ignored
		{"-- c\n\nlet a = 1\nlet b =\n    a + 1\nb * 10", "..ss.s"},
		{"match x {\n  Just v -> v,\n  Nothing -> 0,\n}", "s..."}, // a closing brace continues
		{"<ul>\n  <li>a</li>\n</ul>", "s.."},                      // a closing tag continues
		{"f (\n  1\n)\n[ 1,\n  2\n]\nx", "s..s..s"},               // `)` and `]` too
		{"let r = {\n  a: 1,\n}\n-- note\nr.a", "s...s"},
	}
	for _, c := range cases {
		got := ""
		for _, s := range Starts(strings.Split(c.src, "\n")) {
			if s {
				got += "s"
			} else {
				got += "."
			}
		}
		if got != c.want {
			t.Errorf("Starts(%q) = %s, want %s", c.src, got, c.want)
		}
	}
}
