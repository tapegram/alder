// Package snippets splits Alder typed or pasted as a sequence of items (`alder eval -`'s input,
// a REPL entry, an expression block of the book) into its items: where each one starts. It is
// the one rule `alder eval` and the book verifier share.
package snippets

import "strings"

// IsCloser reports whether a line (trimmed) starts with a closer: `}`, `)`, `]` or a closing
// tag `</…>`. Such a line ends something an earlier line opened, so it continues that item
// even at column 1 (the book's style for a multi-line `match`, record or element).
func IsCloser(trimmed string) bool {
	return strings.HasPrefix(trimmed, "}") || strings.HasPrefix(trimmed, ")") ||
		strings.HasPrefix(trimmed, "]") || strings.HasPrefix(trimmed, "</")
}

// Starts reports, for each line, whether an item starts there. The first item starts on the
// first line that is neither blank nor a `--` comment, whatever its indentation; every later
// item on a line at column 1 that is not blank, a comment or a closer (IsCloser). Every other
// line continues the item before it: indented lines, blank lines, comments and closers.
func Starts(lines []string) []bool {
	starts := make([]bool, len(lines))
	first := true
	for i, l := range lines {
		t := strings.TrimSpace(l)
		if t == "" || strings.HasPrefix(t, "--") {
			continue
		}
		if first {
			starts[i], first = true, false
			continue
		}
		starts[i] = l[0] != ' ' && l[0] != '\t' && !IsCloser(t)
	}
	return starts
}
