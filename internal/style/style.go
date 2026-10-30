// Package style colours and weights the CLI's human-facing output on a terminal, and does
// nothing anywhere else: a Style from For is Plain for a pipe, a buffer, NO_COLOR and TERM=dumb,
// so the bytes between the escape sequences are always the plain output. It is the one place
// that looks at the terminal. It uses the 16 ANSI colours only, so the terminal's theme applies.
package style

import (
	"io"
	"os"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"unicode/utf8"
)

// Style renders text for one output stream. The zero value is Plain.
type Style struct {
	ansi  bool
	width int
}

// Plain emits no sequences: every method returns its argument.
var Plain Style

// For is the Style of w: styled only when w is a terminal (and the environment allows it) or
// ALDER_COLOR=always. Any other writer is Plain.
func For(w io.Writer) Style { return ForEnv(w, os.Getenv) }

// ForEnv is For reading its variables through getenv (the deploy commands read none from the
// process's own environment).
func ForEnv(w io.Writer, getenv func(string) string) Style {
	if _, ok := w.(plainWriter); ok {
		return Plain
	}
	f, isFile := w.(*os.File)
	isTTY := false
	if isFile {
		isTTY = isTerminal(f)
	}
	if !decide(isTTY, runtime.GOOS == "windows", getenv) {
		return Plain
	}
	width := 80
	if isFile {
		width = termWidth(f, getenv)
	} else if c, err := strconv.Atoi(getenv("COLUMNS")); err == nil && c > 0 {
		width = max(40, c)
	}
	return Style{ansi: true, width: width}
}

type plainWriter struct{ io.Writer }

// NoStyle is w marked as never styled, even under ALDER_COLOR=always: for a buffer the command
// itself reads back (the deploy gate parses `alder test`'s report), not one a person sees.
func NoStyle(w io.Writer) io.Writer { return plainWriter{w} }

// isTerminal reports whether f is a character device that is not the null device (which is
// one, but no terminal).
func isTerminal(f *os.File) bool {
	fi, err := f.Stat()
	if err != nil || fi.Mode()&os.ModeCharDevice == 0 {
		return false
	}
	if null, err := os.Stat(os.DevNull); err == nil && os.SameFile(fi, null) {
		return false
	}
	return true
}

// decide is whether output is styled. ALDER_COLOR=never wins; =always forces; otherwise a
// terminal, NO_COLOR unset or empty, and TERM neither dumb nor empty. On Windows only
// ALDER_COLOR=always styles.
func decide(isTTY, windows bool, getenv func(string) string) bool {
	switch getenv("ALDER_COLOR") {
	case "never":
		return false
	case "always":
		return true
	}
	if windows || !isTTY || getenv("NO_COLOR") != "" {
		return false
	}
	t := getenv("TERM")
	return t != "" && t != "dumb"
}

// On is true when sequences are emitted.
func (s Style) On() bool { return s.ansi }

// Width is the terminal's columns; 80 when unknown, never below 40.
func (s Style) Width() int {
	if s.width == 0 {
		return 80
	}
	return s.width
}

func (s Style) sgr(code, text string) string {
	if !s.ansi || text == "" {
		return text
	}
	return "\x1b[" + code + "m" + text + "\x1b[0m"
}

func (s Style) Bold(text string) string    { return s.sgr("1", text) }
func (s Style) Dim(text string) string     { return s.sgr("2", text) }
func (s Style) Red(text string) string     { return s.sgr("31", text) }
func (s Style) Green(text string) string   { return s.sgr("32", text) }
func (s Style) Yellow(text string) string  { return s.sgr("33", text) }
func (s Style) Cyan(text string) string    { return s.sgr("36", text) }
func (s Style) BoldRed(text string) string { return s.sgr("1;31", text) }

// BoldGreen is bold and green in one sequence.
func (s Style) BoldGreen(text string) string { return s.sgr("1;32", text) }

// BoldYellow is bold and yellow in one sequence.
func (s Style) BoldYellow(text string) string { return s.sgr("1;33", text) }

// Mark styles a status word: ok green; FAIL and error bold red; warning and SKIPPED yellow;
// anything else as it is.
func (s Style) Mark(status string) string {
	switch status {
	case "ok":
		return s.Green(status)
	case "FAIL", "error":
		return s.BoldRed(status)
	case "warning", "SKIPPED":
		return s.Yellow(status)
	}
	return status
}

// Severity styles a diagnostic's severity word like Mark, with the "[CODE]" riding along.
func (s Style) Severity(severity, text string) string {
	switch severity {
	case "error":
		return s.BoldRed(text)
	case "warning":
		return s.Yellow(text)
	}
	return text
}

// Wrap breaks text at spaces so no line is longer than width runes, keeping its existing
// newlines. A word longer than width stays whole on a line of its own.
func (s Style) Wrap(text string, width int) string {
	var out []string
	for _, para := range strings.Split(text, "\n") {
		words := strings.Fields(para)
		if len(words) == 0 {
			out = append(out, para)
			continue
		}
		line := words[0]
		for _, w := range words[1:] {
			if utf8.RuneCountInString(line)+1+utf8.RuneCountInString(w) > width {
				out = append(out, line)
				line = w
			} else {
				line += " " + w
			}
		}
		out = append(out, line)
	}
	return strings.Join(out, "\n")
}

// Columns lays rows out left-aligned, each column as wide as its widest cell by rune count,
// two spaces between, every line prefixed by indent and ending in a newline, with no trailing
// spaces.
func Columns(indent string, rows [][]string) string {
	return ColumnsStyled(indent, rows, nil)
}

// ColumnsStyled is Columns with f applied to each cell after the widths are fixed (nil: none),
// so a styled cell does not disturb the alignment.
func ColumnsStyled(indent string, rows [][]string, f func(col int, cell string) string) string {
	var widths []int
	for _, r := range rows {
		for i, c := range r {
			if i >= len(widths) {
				widths = append(widths, 0)
			}
			widths[i] = max(widths[i], utf8.RuneCountInString(c))
		}
	}
	var sb strings.Builder
	for _, r := range rows {
		sb.WriteString(indent)
		for i, c := range r {
			cell := c
			if f != nil {
				cell = f(i, c)
			}
			sb.WriteString(cell)
			if i < len(r)-1 {
				sb.WriteString(strings.Repeat(" ", widths[i]-utf8.RuneCountInString(c)+2))
			}
		}
		sb.WriteString("\n")
	}
	return sb.String()
}

// LineWriter applies a function to each complete line written to it.
type LineWriter struct {
	mu  sync.Mutex
	w   io.Writer
	f   func(string) string
	buf []byte
}

// Lines is a writer that buffers partial writes until a newline, calls f on each line (without
// its newline) and writes the result plus a newline to w. f returns an unrecognised line
// unchanged. Flush writes a trailing partial line.
func Lines(w io.Writer, f func(line string) string) *LineWriter { return &LineWriter{w: w, f: f} }

func (l *LineWriter) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.buf = append(l.buf, p...)
	for {
		i := strings.IndexByte(string(l.buf), '\n')
		if i < 0 {
			return len(p), nil
		}
		line := string(l.buf[:i])
		l.buf = l.buf[i+1:]
		if _, err := io.WriteString(l.w, l.f(line)+"\n"); err != nil {
			return len(p), err
		}
	}
}

// Flush writes the buffered partial line, if any, without adding a newline.
func (l *LineWriter) Flush() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if len(l.buf) == 0 {
		return nil
	}
	line := string(l.buf)
	l.buf = nil
	_, err := io.WriteString(l.w, l.f(line))
	return err
}

var sgrSeq = regexp.MustCompile("\x1b\\[[0-9;]*m")

// Strip removes SGR sequences from s.
func Strip(s string) string { return sgrSeq.ReplaceAllString(s, "") }
