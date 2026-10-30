package style

import (
	"bytes"
	"os"
	"strings"
	"testing"
)

func env(kv ...string) func(string) string {
	m := map[string]string{}
	for i := 0; i+1 < len(kv); i += 2 {
		m[kv[i]] = kv[i+1]
	}
	return func(k string) string { return m[k] }
}

func TestFor_NonTerminalsPlain(t *testing.T) {
	var buf bytes.Buffer
	if For(&buf).On() {
		t.Error("a buffer is styled")
	}
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	defer w.Close()
	if For(w).On() {
		t.Error("a pipe is styled")
	}
	null, err := os.OpenFile(os.DevNull, os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer null.Close()
	if ForEnv(null, env("TERM", "xterm")).On() {
		t.Error("/dev/null is styled")
	}
}

func TestDecide(t *testing.T) {
	for _, c := range []struct {
		name    string
		tty     bool
		windows bool
		env     []string
		want    bool
	}{
		{"terminal", true, false, []string{"TERM", "xterm-256color"}, true},
		{"not a terminal", false, false, []string{"TERM", "xterm"}, false},
		{"NO_COLOR set", true, false, []string{"TERM", "xterm", "NO_COLOR", "1"}, false},
		{"NO_COLOR empty", true, false, []string{"TERM", "xterm", "NO_COLOR", ""}, true},
		{"TERM dumb", true, false, []string{"TERM", "dumb"}, false},
		{"TERM empty", true, false, []string{}, false},
		{"always styles a buffer", false, false, []string{"ALDER_COLOR", "always"}, true},
		{"always beats NO_COLOR", false, false, []string{"ALDER_COLOR", "always", "NO_COLOR", "1", "TERM", "dumb"}, true},
		{"never unstyles a terminal", true, false, []string{"ALDER_COLOR", "never", "TERM", "xterm"}, false},
		{"windows terminal alone", true, true, []string{"TERM", "xterm"}, false},
		{"windows with always", true, true, []string{"ALDER_COLOR", "always"}, true},
	} {
		if got := decide(c.tty, c.windows, env(c.env...)); got != c.want {
			t.Errorf("%s: got %v", c.name, got)
		}
	}
}

func TestFor_AlwaysStylesABuffer(t *testing.T) {
	var buf bytes.Buffer
	st := ForEnv(&buf, env("ALDER_COLOR", "always"))
	if !st.On() || st.Width() != 80 {
		t.Errorf("on %v width %d", st.On(), st.Width())
	}
	if st := ForEnv(&buf, env("ALDER_COLOR", "always", "COLUMNS", "120")); st.Width() != 120 {
		t.Errorf("width %d", st.Width())
	}
	if st := ForEnv(&buf, env("ALDER_COLOR", "always", "COLUMNS", "10")); st.Width() != 40 {
		t.Errorf("width %d, want the floor 40", st.Width())
	}
}

func TestPlain_Identity(t *testing.T) {
	p := Plain
	for _, f := range []func(string) string{p.Bold, p.Dim, p.Red, p.Green, p.Yellow, p.Cyan, p.BoldRed, p.BoldGreen, p.BoldYellow, p.Mark} {
		if f("ok x") != "ok x" {
			t.Fatal("Plain changed its argument")
		}
	}
	if p.Severity("error", "error[E1]") != "error[E1]" || p.Width() != 80 || p.On() {
		t.Error("Plain")
	}
}

func TestSGR(t *testing.T) {
	s := Style{ansi: true}
	for _, c := range []struct{ got, want string }{
		{s.Bold("x"), "\x1b[1mx\x1b[0m"},
		{s.Dim("x"), "\x1b[2mx\x1b[0m"},
		{s.Red("x"), "\x1b[31mx\x1b[0m"},
		{s.Green("x"), "\x1b[32mx\x1b[0m"},
		{s.Yellow("x"), "\x1b[33mx\x1b[0m"},
		{s.Cyan("x"), "\x1b[36mx\x1b[0m"},
		{s.BoldRed("x"), "\x1b[1;31mx\x1b[0m"},
		{s.Bold(""), ""},
	} {
		if c.got != c.want {
			t.Errorf("got %q want %q", c.got, c.want)
		}
	}
}

func TestMark(t *testing.T) {
	s := Style{ansi: true}
	for word, want := range map[string]string{
		"ok": s.Green("ok"), "FAIL": s.BoldRed("FAIL"), "error": s.BoldRed("error"),
		"warning": s.Yellow("warning"), "SKIPPED": s.Yellow("SKIPPED"), "other": "other",
	} {
		if got := s.Mark(word); got != want {
			t.Errorf("Mark(%q) = %q", word, got)
		}
	}
}

func TestColumns_RuneWidths(t *testing.T) {
	got := Columns("  ", [][]string{{"a→b", "one"}, {"café", "two"}, {"x", "three"}})
	want := "  a→b   one\n  café  two\n  x     three\n"
	if got != want {
		t.Errorf("got\n%q\nwant\n%q", got, want)
	}
	st := ColumnsStyled("", [][]string{{"ab", "c"}, {"d", "e"}}, func(col int, cell string) string {
		if col == 0 {
			return "[" + cell + "]"
		}
		return cell
	})
	if st != "[ab]  c\n[d]   e\n" {
		t.Errorf("styled columns: %q", st)
	}
}

func TestWrap(t *testing.T) {
	var s Style
	got := s.Wrap("one two three\nfour  five", 9)
	if want := "one two\nthree\nfour five"; got != want {
		t.Errorf("got %q want %q", got, want)
	}
	if got := s.Wrap("a verylongwordhere b", 5); got != "a\nverylongwordhere\nb" {
		t.Errorf("long word: %q", got)
	}
	if got := s.Wrap("a\n\nb", 20); got != "a\n\nb" {
		t.Errorf("blank line: %q", got)
	}
}

func TestLines_PartialWrites(t *testing.T) {
	var out bytes.Buffer
	lw := Lines(&out, strings.ToUpper)
	lw.Write([]byte("ab"))
	lw.Write([]byte("c\nde"))
	if out.String() != "ABC\n" {
		t.Fatalf("after two writes: %q", out.String())
	}
	lw.Write([]byte("f\ng\n"))
	if out.String() != "ABC\nDEF\nG\n" {
		t.Fatalf("got %q", out.String())
	}
	lw.Write([]byte("tail"))
	if out.String() != "ABC\nDEF\nG\n" {
		t.Fatalf("partial line flushed early: %q", out.String())
	}
	lw.Flush()
	if out.String() != "ABC\nDEF\nG\nTAIL" {
		t.Fatalf("after flush: %q", out.String())
	}
}

func TestStrip(t *testing.T) {
	s := Style{ansi: true}
	if got := Strip(s.BoldRed("FAIL") + " " + s.Dim("x") + " " + s.Green("ok")); got != "FAIL x ok" {
		t.Errorf("got %q", got)
	}
}
