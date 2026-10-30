package main

import (
	"regexp"
	"strings"

	"alder/internal/style"
)

// reportStyler restyles the test program's text report, line by line, on a terminal. It
// recognises the shapes rt/testing writes and leaves any other line as it is; the bytes between
// the escape sequences are the program's. After a summary with failures it adds the recap: the
// failed tests' names again, so they are on screen when the run ends.
type reportStyler struct {
	st          style.Style
	failed      []string // the `Module "name"` of each FAIL line since the last summary
	inSurvivors bool     // the rows under `survived:`
}

var (
	summaryLine  = regexp.MustCompile(`^(\d+) passed, (\d+) failed$`)
	mutantsLine  = regexp.MustCompile(`^\d+ mutants: `)
	detailLabel  = regexp.MustCompile(`^(  )(left:|right:|note:|error:|seed:|run:)(.*)$`)
	survivorKind = regexp.MustCompile(`^(  \S+\s+\S+\s+)(\S+)(.*)$`)
)

// line is the styled form of one report line (without its newline), plus the recap when it is a
// summary with failures.
func (r *reportStyler) line(l string) string {
	st := r.st
	if r.inSurvivors && survivorKind.MatchString(l) && strings.HasPrefix(l, "  ") {
		m := survivorKind.FindStringSubmatch(l)
		return m[1] + st.Yellow(m[2]) + m[3]
	}
	r.inSurvivors = false
	switch {
	case strings.HasPrefix(l, "ok   "):
		return st.Green("ok") + l[2:]
	case strings.HasPrefix(l, "FAIL "):
		name := l[len("FAIL "):]
		if i := strings.Index(name, "  (failed on"); i >= 0 {
			name = name[:i]
		}
		r.failed = append(r.failed, name)
		return st.BoldRed("FAIL") + " " + st.Bold(l[len("FAIL "):])
	case detailLabel.MatchString(l):
		m := detailLabel.FindStringSubmatch(l)
		return m[1] + st.Cyan(m[2]) + m[3]
	case strings.HasPrefix(l, "  trace: "):
		return st.Dim(l)
	case summaryLine.MatchString(l):
		m := summaryLine.FindStringSubmatch(l)
		failed := r.failed
		r.failed = nil
		if m[2] == "0" {
			return st.BoldGreen(l)
		}
		out := st.BoldRed(l)
		for _, name := range failed {
			out += "\n  " + st.BoldRed("FAIL") + " " + name
		}
		return out
	case mutantsLine.MatchString(l):
		return st.Bold(l)
	case l == "survived:":
		r.inSurvivors = true
		return st.BoldRed(l)
	case strings.HasPrefix(l, "no tests: "):
		return st.Dim(l)
	}
	return l
}

// stillRunning styles the stderr line `still running M "name" (10s)` yellow.
func stillRunning(st style.Style) func(string) string {
	return func(l string) string {
		if strings.HasPrefix(l, "still running ") {
			return st.Yellow(l)
		}
		return l
	}
}
