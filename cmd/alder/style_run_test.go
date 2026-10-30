package main

import (
	"bytes"
	"strings"
	"testing"

	"alder/internal/style"
)

func TestRun_LogLineStyling(t *testing.T) {
	st := alwaysStyle()
	f := logLine(st)
	ts := "2026/10/29 23:32:51 "
	for _, c := range []struct{ in, want string }{
		{ts + "info: migrated 0001_initial", st.Dim(ts) + st.Green("info:") + " migrated 0001_initial"},
		{ts + "warn: ALDER_SECRET is unset", st.Dim(ts) + st.Yellow("warn:") + " ALDER_SECRET is unset"},
		{ts + "error: boom", st.Dim(ts) + st.Red("error:") + " boom"},
		{ts + "something else", st.Dim(ts) + "something else"},
		{"info: no timestamp", "info: no timestamp"},
		{"listening on 127.0.0.1:1", "listening on 127.0.0.1:1"},
	} {
		if got := f(c.in); got != c.want {
			t.Errorf("%q: got %q want %q", c.in, got, c.want)
		}
		if style.Strip(f(c.in)) != c.in {
			t.Errorf("Strip changed %q", c.in)
		}
	}
}

func TestExplain_WrapsOnTerminalOnly(t *testing.T) {
	t.Setenv("ALDER_COLOR", "never")
	var plain, errb bytes.Buffer
	if code := explainTo("E0003", &plain, &errb); code != 0 {
		t.Fatal(errb.String())
	}
	if strings.Count(plain.String(), "\n") != 1 {
		t.Errorf("piped explain is not one line:\n%s", plain.String())
	}
	t.Setenv("ALDER_COLOR", "always")
	t.Setenv("COLUMNS", "60")
	var wrapped bytes.Buffer
	explainTo("E0003", &wrapped, &errb)
	lines := strings.Split(strings.TrimRight(wrapped.String(), "\n"), "\n")
	if len(lines) < 5 {
		t.Fatalf("not wrapped: %q", wrapped.String())
	}
	for _, l := range lines {
		if len(l) > 100 {
			t.Errorf("line over 100: %q", l)
		}
	}
	if strings.Join(strings.Fields(wrapped.String()), " ") != strings.Join(strings.Fields(plain.String()), " ") {
		t.Error("wrapping changed the words")
	}
	var unknown bytes.Buffer
	if code := explainTo("E9999", &bytes.Buffer{}, &unknown); code != 2 || !strings.Contains(unknown.String(), "unknown diagnostic code E9999") {
		t.Errorf("unknown code: %d %q", code, unknown.String())
	}
}
