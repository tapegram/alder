package deploy

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os/exec"
	"strings"
)

// Refusal is a deploy that stopped on purpose: the gate, a missing environment variable, a
// migration that drops data, a health check. The command prints "refused: <Msg>" and Detail
// under it, and exits 1.
type Refusal struct {
	Msg    string
	Detail string
}

func (r *Refusal) Error() string {
	if r.Detail == "" {
		return "refused: " + r.Msg
	}
	return "refused: " + r.Msg + "\n" + strings.TrimRight(r.Detail, "\n")
}

func refuse(detail, format string, args ...any) *Refusal {
	return &Refusal{Msg: fmt.Sprintf(format, args...), Detail: detail}
}

// Runner runs the gate's fixed steps, as `alder check`, `alder build`, `alder test` and `alder
// test --mutate` do (the command passes its own code paths). Each returns what the step printed
// (shown when it refuses) and whether it passed.
type Runner interface {
	// Check is `alder check`: the counts of errors, warnings and typed holes.
	Check() (errors, warnings, holes int, out string)
	Build() (out string, ok bool)
	Test() (r TestResult, out string, ok bool)
	// Mutate is `alder test --mutate`: ok is false when the tests themselves failed.
	Mutate() (score float64, mutants, survived int, out string, ok bool)
}

// Floor is the mutation score the gate demands and where it comes from.
type Floor struct {
	Value float64
	From  string // "declared", "last deploy", "none yet"
}

// GateCheck is the gate's first step: errors refuse, and so do typed holes.
func GateCheck(r Runner, w io.Writer) (GateResult, error) {
	errs, warns, holes, out := r.Check()
	g := GateResult{Warnings: warns}
	switch {
	case errs > 0:
		line(w, "check", "FAIL", plural(errs, "error"))
		return g, refuse(out, "`alder check` found %s: fix them before deploying", plural(errs, "error"))
	case holes > 0:
		line(w, "check", "FAIL", plural(holes, "typed hole"))
		return g, refuse(out, "the program has %s left: fill them before deploying", plural(holes, "typed hole"))
	}
	g.Check = "ok"
	note := ""
	if warns > 0 {
		note = plural(warns, "warning")
	}
	line(w, "check", "ok", note)
	return g, nil
}

// GateRest is the rest of the gate after GateCheck: build, tests, the mutation score against
// floor, then each declared check (a shell command run from appDir; a non-zero exit refuses
// with its last lines). skip (--unsafe-skip-tests) skips everything after the build.
func GateRest(ctx context.Context, g GateResult, r Runner, appDir string, checks []string, floor Floor, skip bool, w io.Writer) (GateResult, error) {
	out, ok := r.Build()
	if !ok {
		line(w, "build", "FAIL", "")
		return g, refuse(lastLines(out, 30), "`alder build` failed")
	}
	g.Build = "ok"
	line(w, "build", "ok", "")
	if skip {
		g.Skipped = true
		line(w, "test", "SKIPPED", "--unsafe-skip-tests: no tests, no mutation score, no checks")
		return g, nil
	}
	tr, out, ok := r.Test()
	g.Tests = &tr
	if !ok {
		line(w, "test", "FAIL", fmt.Sprintf("%d passed, %d failed", tr.Passed, tr.Failed))
		return g, refuse(lastLines(out, 30), "`alder test` failed: %d of %d tests failed", tr.Failed, tr.Passed+tr.Failed)
	}
	line(w, "test", "ok", fmt.Sprintf("%d passed", tr.Passed))
	score, mutants, survived, out, ok := r.Mutate()
	if !ok {
		line(w, "mutate", "FAIL", "")
		return g, refuse(lastLines(out, 30), "`alder test --mutate` did not finish")
	}
	g.Mutate = &MutateResult{Score: score, Floor: floor.Value, FloorFrom: floor.From, Mutants: mutants, Survived: survived}
	if score+1e-9 < floor.Value {
		line(w, "mutate", "FAIL", fmt.Sprintf("score %.2f < floor %.2f (%s)", score, floor.Value, floor.From))
		return g, refuse("", "the mutation score %.2f is below the floor %.2f (%s): %s survived; `alder test --mutate` lists them, and a test that kills them raises the score",
			score, floor.Value, floor.From, plural(survived, "mutant"))
	}
	line(w, "mutate", "ok", fmt.Sprintf("score %.2f ≥ floor %.2f (%s; %d mutants, %d survived)", score, floor.Value, floor.From, mutants, survived))
	for _, c := range checks {
		cmd := exec.CommandContext(ctx, "sh", "-c", c)
		cmd.Dir = appDir
		var buf bytes.Buffer
		cmd.Stdout, cmd.Stderr = &buf, &buf
		err := cmd.Run()
		code := 0
		if err != nil {
			code = -1
			if ee, ok := err.(*exec.ExitError); ok {
				code = ee.ExitCode()
			}
		}
		g.Checks = append(g.Checks, CheckResult{Command: c, Exit: code})
		if err != nil {
			line(w, "check", "FAIL", c)
			return g, refuse(lastLines(buf.String(), 20), "the check `%s` exited %d", c, code)
		}
		line(w, "check", "ok", c)
	}
	return g, nil
}

// line prints one step of a stage: "  build    ok  note".
func line(w io.Writer, step, status, note string) {
	if note != "" {
		note = "  " + note
	}
	fmt.Fprintf(w, "  %-8s %s%s\n", step, status, note)
}

func plural(n int, what string) string {
	if n == 1 {
		return "1 " + what
	}
	return fmt.Sprintf("%d %ss", n, what)
}

// lastLines is the last n lines of s.
func lastLines(s string, n int) string {
	lines := strings.Split(strings.TrimRight(s, "\n"), "\n")
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return strings.Join(lines, "\n")
}
