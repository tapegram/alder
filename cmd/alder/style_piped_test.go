package main

import (
	"bytes"
	"regexp"
	"strings"
	"testing"
)

// TestCLI_PipedUnchanged pins the piped text of check, test and the test --mutate score line
// (what the deploy gate and nightvote-replay read): no escapes, no count line, no recap, and the
// exact lines they have always had.
func TestCLI_PipedUnchanged(t *testing.T) {
	needGoTool(t)
	t.Setenv("ALDER_COLOR", "never")

	chk := runIn(func(o, e *bytes.Buffer) int { return runCheckTo([]string{twoErrorsOneWarning(t)}, o, e) })
	wantOut := "app/X.alder:4:5: error[E0003]: expected Text, got Int\n  x = 1\n      ^\n  fix: Text.fromInt (1)\n" +
		"app/X.alder:7:5: error[E0003]: expected Int, got Text\n  y = \"s\"\n      ^^^\n"
	if chk.code != 1 || chk.stdout != wantOut {
		t.Errorf("check stdout:\n%q\nwant\n%q (exit %d)", chk.stdout, wantOut, chk.code)
	}
	if chk.err != "app/X.alder:10:9: warning[W0002]: hole ?count has type Int\n  z = 1 + ?count\n          ^^^^^^\nalder check: 1 typed hole left\n" {
		t.Errorf("check stderr: %q", chk.err)
	}

	dir := tempTodos(t, "Table/Todo.alder", "\ntest \"wrong\" = countOpen [] == 1\n")
	code, out, errs := runTest(t, dir)
	if code != 1 || errs != "" || strings.Contains(out, "\x1b") || strings.Contains(out, "\n  FAIL ") ||
		!strings.Contains(out, "FAIL Table.Todo \"wrong\"\n  left:  0\n  right: 1\n") || !strings.HasSuffix(out, "116 passed, 1 failed\n") {
		t.Errorf("test (exit %d):\n%s\n%s", code, out, errs)
	}

	app := genApp(t, "scaffold")
	code, out, errs = runTest(t, app, "--mutate")
	score := regexp.MustCompile(`(?m)^\d+ mutants: \d+ killed, \d+ survived  \(score \d\.\d\d\)$`)
	if code != 0 || strings.Contains(out, "\x1b") || !score.MatchString(out) {
		t.Errorf("test --mutate (exit %d):\n%s\n%s", code, out, errs)
	}
}
