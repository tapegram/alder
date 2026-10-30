package deploy

import (
	"io"
	"regexp"
	"strings"

	"alder/internal/style"
)

// Styled is w restyled for a terminal: a writer that colours the lines this package prints
// (the gate's `  test     ok  116 passed` rows by their status, the headings `gate`, `plan`,
// `ship`, `artifact …`, `deployed … to …`, `warning:`, DROPS DATA, and `alder status`'s labels
// and its `*` row). It adds no characters: Strip of its output is what was written. With
// style.Plain it is w itself. The returned function writes a trailing partial line; call it
// when the command is done.
func Styled(w io.Writer, st style.Style) (io.Writer, func()) {
	if !st.On() {
		return w, func() {}
	}
	lw := style.Lines(w, StyleLine(st))
	return lw, func() { _ = lw.Flush() }
}

var (
	stepRow     = regexp.MustCompile(`^(  [a-z]+ +)(ok|FAIL|SKIPPED)((?:  .*)?)$`)
	statusLabel = regexp.MustCompile(`^(current|builds|database|backup|silos) `)
)

// StyleLine is the styled form of one line of a deploy's or status's output.
func StyleLine(st style.Style) func(string) string {
	return func(l string) string {
		switch {
		case stepRow.MatchString(l):
			m := stepRow.FindStringSubmatch(l)
			return m[1] + st.Mark(m[2]) + m[3]
		case l == "gate" || l == "plan" || l == "ship" || l == "target" || l == "builds":
			return st.Bold(l)
		case strings.HasPrefix(l, "artifact ") || strings.HasPrefix(l, "deployed "):
			return st.Bold(l)
		case strings.HasPrefix(l, "warning:"):
			return st.Yellow("warning:") + l[len("warning:"):]
		case strings.HasPrefix(l, "  * "):
			return st.Green(l)
		case strings.HasPrefix(l, "  DROPS DATA"):
			return "  " + st.BoldRed("DROPS DATA") + l[len("  DROPS DATA"):]
		case statusLabel.MatchString(l):
			label := statusLabel.FindString(l)
			return st.Bold(strings.TrimRight(label, " ")) + label[len(strings.TrimRight(label, " ")):] + l[len(label):]
		}
		return l
	}
}
