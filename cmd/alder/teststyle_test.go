package main

import (
	"bytes"
	"strings"
	"testing"

	"alder/internal/style"
)

func styleReport(t *testing.T, report string) string {
	t.Helper()
	st := alwaysStyle()
	var out bytes.Buffer
	lw := style.Lines(&out, (&reportStyler{st: st}).line)
	lw.Write([]byte(report))
	lw.Flush()
	return out.String()
}

func TestReportStyler_Shapes(t *testing.T) {
	st := alwaysStyle()
	report := `ok   A "fine"
FAIL B "bad"
  left:  1
  right: 2
  trace: ["x"]
FAIL C "prop"  (failed on run 3 of 100, shrunk in 2 steps)
  xs: [1]
  seed: 7
1 passed, 2 failed
`
	got := styleReport(t, report)
	for _, want := range []string{
		st.Green("ok") + "   A \"fine\"\n",
		st.BoldRed("FAIL") + " " + st.Bold("B \"bad\"") + "\n",
		"  " + st.Cyan("left:") + "  1\n",
		"  " + st.Cyan("right:") + " 2\n",
		st.Dim("  trace: [\"x\"]") + "\n",
		"  " + st.Cyan("seed:") + " 7\n",
		st.BoldRed("1 passed, 2 failed") + "\n  " + st.BoldRed("FAIL") + " B \"bad\"\n  " + st.BoldRed("FAIL") + " C \"prop\"\n",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in\n%q", want, got)
		}
	}
	recap := "  FAIL B \"bad\"\n  FAIL C \"prop\"\n"
	if style.Strip(got) != report+recap {
		t.Errorf("Strip(styled) is not the report plus the recap:\n%q", style.Strip(got))
	}
}

func TestReportStyler_NoRecapWhenAllPass(t *testing.T) {
	st := alwaysStyle()
	report := "ok   A \"a\"\nok   A \"b\"\n2 passed, 0 failed\n"
	got := styleReport(t, report)
	if got != st.Green("ok")+"   A \"a\"\n"+st.Green("ok")+"   A \"b\"\n"+st.BoldGreen("2 passed, 0 failed")+"\n" {
		t.Errorf("got %q", got)
	}
	if style.Strip(got) != report {
		t.Errorf("strip: %q", style.Strip(got))
	}
}

func TestReportStyler_MutateAndUnknown(t *testing.T) {
	st := alwaysStyle()
	report := "5 passed, 0 failed\n10 mutants: 8 killed, 2 survived  (score 0.80)\nsurvived:\n  Mod   f:3   flip ==   x\nsomething else\nno tests: `alder test --mutate` kills nothing (chapter 10)\n"
	got := styleReport(t, report)
	for _, want := range []string{
		st.Bold("10 mutants: 8 killed, 2 survived  (score 0.80)"),
		st.BoldRed("survived:"),
		"  Mod   f:3   " + st.Yellow("flip") + " ==   x\n",
		"something else\n",
		st.Dim("no tests: `alder test --mutate` kills nothing (chapter 10)"),
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in\n%q", want, got)
		}
	}
	if style.Strip(got) != report {
		t.Errorf("strip: %q", style.Strip(got))
	}
}

func TestStillRunning(t *testing.T) {
	st := alwaysStyle()
	f := stillRunning(st)
	if f(`still running A "x" (10s)`) != st.Yellow(`still running A "x" (10s)`) || f("other") != "other" {
		t.Error("stillRunning")
	}
}

// ALDER_COLOR=always on a failing todos variant: the styled report is the plain one with
// escapes, plus the recap after the summary.
func TestTest_StyledReport(t *testing.T) {
	needGoTool(t)
	dir := tempTodos(t, "Table/Todo.alder", "\ntest \"wrong\" = countOpen [] == 1\n\ntest \"also wrong\" = countOpen [] == 2\n")
	t.Setenv("ALDER_COLOR", "never")
	pcode, plain, _ := runTest(t, dir)
	t.Setenv("ALDER_COLOR", "always")
	code, styled, stderr := runTest(t, dir)
	if code != 1 || pcode != 1 {
		t.Fatalf("exit %d/%d\n%s", pcode, code, stderr)
	}
	st := alwaysStyle()
	for _, want := range []string{
		st.BoldRed("FAIL") + " " + st.Bold("Table.Todo \"wrong\""),
		st.Green("ok") + "   ",
		st.BoldRed("116 passed, 2 failed") + "\n  " + st.BoldRed("FAIL") + " Table.Todo \"wrong\"\n  " + st.BoldRed("FAIL") + " Table.Todo \"also wrong\"\n",
	} {
		if !strings.Contains(styled, want) {
			t.Errorf("styled report lacks %q:\n%s", want, styled)
		}
	}
	recap := "  FAIL Table.Todo \"wrong\"\n  FAIL Table.Todo \"also wrong\"\n"
	if strings.Contains(plain, "\x1b") || strings.Contains(plain, recap) {
		t.Errorf("plain run was styled or has a recap:\n%s", plain)
	}
	if style.Strip(styled) != plain+recap {
		t.Errorf("Strip(styled) is not the plain report plus the recap")
	}
}

func TestTest_NoRecapWhenAllPass(t *testing.T) {
	needGoTool(t)
	t.Setenv("ALDER_COLOR", "always")
	code, out, errs := runTest(t, todosDir(t), "-run", "Toggle flips done")
	if code != 0 || strings.Contains(style.Strip(out), "  FAIL") || !strings.Contains(out, alwaysStyle().BoldGreen("1 passed, 0 failed")) {
		t.Errorf("exit %d\n%s\n%s", code, out, errs)
	}
}
