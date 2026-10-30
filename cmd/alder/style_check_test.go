package main

import (
	"bytes"
	"io"
	"path/filepath"
	"strings"
	"testing"

	"alder/internal/diag"
	"alder/internal/style"
)

// alwaysStyle is the Style that ALDER_COLOR=always gives.
func alwaysStyle() style.Style {
	return style.ForEnv(io.Discard, func(k string) string {
		if k == "ALDER_COLOR" {
			return "always"
		}
		return ""
	})
}

// twoErrorsOneWarning is an app whose check prints two errors and a warning (a typed hole).
func twoErrorsOneWarning(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "app", "X.alder"), "module X\n\nx : Text\nx = 1\n\ny : Int\ny = \"s\"\n\nz : Int\nz = 1 + ?count\n")
	return dir
}

func TestCheck_CountLineOnTerminalOnly(t *testing.T) {
	dir := twoErrorsOneWarning(t)
	st := alwaysStyle()
	t.Setenv("ALDER_COLOR", "always")
	got := runIn(func(o, e *bytes.Buffer) int { return runCheckTo([]string{dir}, o, e) })
	if got.code != 1 || !strings.HasSuffix(got.err, st.BoldRed("2 errors, 1 warning")+"\n") {
		t.Fatalf("the count is not the last stderr line, bold red: %+v", got)
	}
	if !strings.Contains(got.stdout, st.BoldRed("error[E0003]")) {
		t.Errorf("diagnostic not styled: %q", got.stdout)
	}
	t.Setenv("ALDER_COLOR", "never")
	plain := runIn(func(o, e *bytes.Buffer) int { return runCheckTo([]string{dir}, o, e) })
	if strings.Contains(plain.err, "2 errors") || strings.Contains(plain.stdout+plain.err, "\x1b") {
		t.Errorf("plain: %+v", plain)
	}
	if style.Strip(got.stdout) != plain.stdout || style.Strip(got.err) != plain.err+"2 errors, 1 warning\n" {
		t.Errorf("styled output differs from plain apart from escapes and the count:\n%+v\n%+v", got, plain)
	}
	// a clean app prints nothing either way
	for _, mode := range []string{"always", "never"} {
		t.Setenv("ALDER_COLOR", mode)
		clean := runIn(func(o, e *bytes.Buffer) int { return runCheckTo([]string{todosDir(t)}, o, e) })
		if clean.code != 0 || clean.stdout != "" || clean.err != "" {
			t.Errorf("clean app, %s: %+v", mode, clean)
		}
	}
}

func TestCountLine(t *testing.T) {
	st := alwaysStyle()
	if got := countLine([]diag.Diagnostic{{Severity: "warning"}}, st); got != st.Yellow("1 warning") {
		t.Errorf("got %q", got)
	}
	if got := countLine([]diag.Diagnostic{{Severity: "error"}}, st); got != st.BoldRed("1 error") {
		t.Errorf("got %q", got)
	}
}
