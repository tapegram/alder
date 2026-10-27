package rtesting

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"alder/rt/db"
	"alder/rt/eff"
	"alder/rt/web"
)

// The mutate build's Mutant, as the generated code has it.
var active int

// mutantCases are cases whose behaviour depends on the active mutant: "a" fails under 1, "b"
// panics under 2, "c" hangs under 3, "d" (a task) breaks a unique column under 5, whose
// Tables make title @unique; nothing notices 4.
func mutantCases() []Case {
	return []Case{
		{Module: "M", Name: "a", Pure: func() bool { return active != 1 }},
		{Module: "M", Name: "b", Pure: func() bool {
			if active == 2 {
				var m map[string]int
				m["x"] = 1 // a Go runtime panic, as a crashing mutant's would be
			}
			return true
		}},
		{Module: "M", Name: "c", Pure: func() bool {
			if active == 3 {
				time.Sleep(time.Hour)
			}
			return true
		}},
		{Module: "N", Name: "d", Task: func() eff.Task[bool] {
			return then(insertStep("Db.insert T", "x"), then(insertStep("Db.insert T", "x"), eff.Pure(true)))
		}},
	}
}

func setMutants(t *testing.T) {
	t.Helper()
	unique := tb.Desc()
	unique.Columns = []db.Column{{Name: "id", Type: db.CText}, {Name: "title", Type: db.CText, Unique: true}}
	SetMutants(&active, []Mutant{
		{ID: 1, Module: "M", Decl: "f", Kind: "bool flipped", Line: 3, Before: "True", After: "False"},
		{ID: 2, Module: "M", Decl: "g", Kind: "arm swapped", Line: 10, Before: "match x { … }", After: "the arms swapped"},
		{ID: 3, Module: "M", Decl: "loop", Kind: "condition negated", Line: 12, Before: "if done", After: "if not done"},
		{ID: 4, Module: "Todos.Index", Decl: "update", Kind: "redirect → rerender", File: "app/Todos/Index.alder", Line: 37, Col: 9, Before: "Redirect (Todos.Index {})", After: "Rerender"},
		{ID: 5, Module: "T", Decl: "T", Kind: "default changed", Line: 4, Before: "title = \"\"", After: "title = \"x\"", Tables: []db.TableDesc{unique}},
	}, 2, 3)
	t.Cleanup(func() { SetMutants(nil, nil, 0, 0) })
	for _, k := range []string{"ALDER_TEST_RUN", "ALDER_TEST_JSON", EnvMutate, EnvMin, EnvTimeout, EnvMutateFrom, EnvMutateLog, EnvDeadline, EnvStatus} {
		t.Setenv(k, "")
	}
}

func runMutate(cs []Case, args ...string) (string, string, int) {
	var out, errb bytes.Buffer
	code := Main(cs, tables, nil, nil, nil, nil, nil, web.Options{}, append([]string{"--mutate", "-timeout", "300ms"}, args...), &out, &errb)
	return out.String(), errb.String(), code
}

// TestMutate_KillsAndReport: a mutant is killed by a failing test (by that test), by a panic
// (crashed), by the timeout (timed out: the run goes on) and by a schema change made with
// Ensure; the survivor is listed with its columns aligned; the score is over the mutants that
// ran, the refused ones apart.
func TestMutate_KillsAndReport(t *testing.T) {
	setMutants(t)
	start := time.Now()
	out, errs, code := runMutate(mutantCases())
	if code != 0 {
		t.Fatalf("exit %d\n%s\n%s", code, out, errs)
	}
	want := `ok   M "a"
ok   M "b"
ok   M "c"
ok   N "d"
4 passed, 0 failed
5 mutants: 4 killed (of which 1 timed out), 1 survived  (score 0.80)
survived:
  Todos.Index   update:37   redirect → rerender   Redirect (Todos.Index {}) → Rerender
`
	if out != want {
		t.Fatalf("report:\n%s\nwant:\n%s", out, want)
	}
	if time.Since(start) > 5*time.Second {
		t.Fatalf("the run took %v: the hanging mutant was not cut off", time.Since(start))
	}

	out, _, code = runMutate(mutantCases(), "--json")
	if code != 0 {
		t.Fatalf("exit %d", code)
	}
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) != 6 {
		t.Fatalf("want 4 test objects, the mutants and the score:\n%s", out)
	}
	var ms struct{ Mutants []map[string]any }
	if err := json.Unmarshal([]byte(lines[4]), &ms); err != nil {
		t.Fatal(err)
	}
	got := []string{}
	for _, m := range ms.Mutants {
		b, _ := json.Marshal(m)
		got = append(got, string(b))
	}
	wantMs := []string{
		`{"after":"False","before":"True","by":"M \"a\"","col":0,"decl":"f","file":"","id":1,"killed":true,"kind":"bool flipped","line":3,"module":"M","note":""}`,
		`{"after":"the arms swapped","before":"match x { … }","by":"M \"b\"","col":0,"decl":"g","file":"","id":2,"killed":true,"kind":"arm swapped","line":10,"module":"M","note":"crashed"}`,
		`{"after":"if not done","before":"if done","by":"M \"c\"","col":0,"decl":"loop","file":"","id":3,"killed":true,"kind":"condition negated","line":12,"module":"M","note":"timed out"}`,
		`{"after":"Rerender","before":"Redirect (Todos.Index {})","by":"","col":9,"decl":"update","file":"app/Todos/Index.alder","id":4,"killed":false,"kind":"redirect → rerender","line":37,"module":"Todos.Index","note":""}`,
		`{"after":"title = \"x\"","before":"title = \"\"","by":"N \"d\"","col":0,"decl":"T","file":"","id":5,"killed":true,"kind":"default changed","line":4,"module":"T","note":"","tables":"ensure"}`,
	}
	if strings.Join(got, "\n") != strings.Join(wantMs, "\n") {
		t.Fatalf("mutants:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(wantMs, "\n"))
	}
	if lines[5] != `{"score":0.8,"killed":4,"survived":1,"suppressed":2,"skipped":3}` {
		t.Fatalf("score object: %s", lines[5])
	}
	if active != 0 {
		t.Fatalf("Mutant left at %d", active)
	}
}

// TestMutate_Min: --min exits 1 below the score (the same report), 0 at or above it; without
// --mutate it is a usage error.
func TestMutate_Min(t *testing.T) {
	setMutants(t)
	cs := mutantCases()[:1] // only mutant 1 is killed: 1/5
	out, _, code := runMutate(cs, "--min", "0.5")
	if code != 1 || !strings.Contains(out, "5 mutants: 1 killed, 4 survived  (score 0.20)\n") {
		t.Fatalf("exit %d\n%s", code, out)
	}
	if _, _, code = runMutate(cs, "--min", "0.2"); code != 0 {
		t.Fatalf("--min 0.2 at score 0.2: exit %d", code)
	}
	var o, e bytes.Buffer
	if code := Main(cs, tables, nil, nil, nil, nil, nil, web.Options{}, []string{"--min", "0.5"}, &o, &e); code != 2 || !strings.Contains(e.String(), "--min needs --mutate") {
		t.Fatalf("--min alone: exit %d %s", code, e.String())
	}
	if _, errs, code := runMutate(cs, "--min", "2"); code != 2 || !strings.Contains(errs, "bad --min") {
		t.Fatalf("--min 2: exit %d %s", code, errs)
	}
}

// TestMutate_RunNarrows: mutants are judged by the selected tests only; a -run that selects
// none says so (not "no tests").
func TestMutate_RunNarrows(t *testing.T) {
	setMutants(t)
	out, _, code := runMutate(mutantCases(), "-run", `M "[ab]"`)
	if code != 0 || !strings.Contains(out, "2 passed, 0 failed\n5 mutants: 2 killed, 3 survived  (score 0.40)\n") {
		t.Fatalf("exit %d\n%s", code, out)
	}
	out, _, code = runMutate(mutantCases(), "-run", `nothing`)
	if code != 0 || out != "0 passed, 0 failed\nno test matches -run: nothing can kill a mutant\n" {
		t.Fatalf("-run matching nothing: exit %d\n%s", code, out)
	}
}

// TestMutate_SurvivorLines: survivors of one kind at different columns of a line are told
// apart by their column (not those of one node, told apart by their change); an arm swap's line
// is elided at 120 columns, its before shortened so the after, which names the swap, stays whole
// (and from its end when that is not enough).
func TestMutate_SurvivorLines(t *testing.T) {
	setMutants(t)
	long := "match step { Go -> \"a long body of the first arm\", Stop -> \"a long body of the second arm\", Wait -> \"the third\" }"
	SetMutants(&active, []Mutant{
		{ID: 1, Module: "M", Decl: "f", Kind: "operator flipped", Line: 13, Col: 7, Before: "a < b", After: "a <= b"},
		{ID: 2, Module: "M", Decl: "f", Kind: "operator flipped", Line: 13, Col: 20, Before: "c > d", After: "c >= d"},
		{ID: 3, Module: "M", Decl: "u", Kind: "update field dropped", Line: 14, Col: 5, Before: "{ t | a: 1, b: 2 }", After: "{ t | b: 2 }"},
		{ID: 4, Module: "M", Decl: "u", Kind: "update field dropped", Line: 14, Col: 5, Before: "{ t | a: 1, b: 2 }", After: "{ t | a: 1 }"},
		{ID: 5, Module: "M", Decl: "g", Kind: "arm swapped", Line: 20, Col: 3, Before: long, After: "the Go and Stop bodies swapped"},
		{ID: 6, Module: "M", Decl: "g", Kind: "arm swapped", Line: 20, Col: 3, Before: long, After: long},
	}, 0, 0)
	out, _, code := runMutate([]Case{{Module: "M", Name: "a", Pure: func() bool { return true }}})
	if code != 0 {
		t.Fatalf("exit %d\n%s", code, out)
	}
	lines := strings.Split(strings.TrimSuffix(out, "\n"), "\n")
	want := []string{
		"  M   f:13:7    operator flipped       a < b → a <= b",
		"  M   f:13:20   operator flipped       c > d → c >= d",
		"  M   u:14      update field dropped   { t | a: 1, b: 2 } → { t | b: 2 }",
		"  M   u:14      update field dropped   { t | a: 1, b: 2 } → { t | a: 1 }",
	}
	if strings.Join(lines[4:8], "\n") != strings.Join(want, "\n") {
		t.Fatalf("survivors:\n%s\nwant:\n%s", strings.Join(lines[4:8], "\n"), strings.Join(want, "\n"))
	}
	named := lines[8]
	if utf8.RuneCountInString(named) != 120 || !strings.HasSuffix(named, "… → the Go and Stop bodies swapped") ||
		!strings.HasPrefix(named, "  M   g:20      arm swapped            match step { Go") {
		t.Fatalf("arm swap line (%d columns): %q", utf8.RuneCountInString(named), named)
	}
	if whole := lines[9]; utf8.RuneCountInString(whole) != 120 || !strings.HasSuffix(whole, "…") {
		t.Fatalf("arm swap line (%d columns): %q", utf8.RuneCountInString(whole), whole)
	}
}

// TestMutate_RelativeDeadline: a mutant's time is 3 × the suite's own run when that is more than
// -timeout: a suite near the timeout warns, and a mutant a little slower than the suite (over
// -timeout, under 3 × the baseline) survives instead of timing out.
func TestMutate_RelativeDeadline(t *testing.T) {
	setMutants(t)
	SetMutants(&active, []Mutant{{ID: 1, Module: "M", Decl: "f", Kind: "bool flipped", Line: 3, Before: "True", After: "False"}}, 0, 0)
	slow := Case{Module: "M", Name: "slow", Pure: func() bool {
		if active == 1 {
			time.Sleep(400 * time.Millisecond)
		} else {
			time.Sleep(200 * time.Millisecond)
		}
		return true
	}}
	out, errs, code := runMutate([]Case{slow})
	if code != 0 || !strings.HasSuffix(out, "1 mutants: 0 killed, 1 survived  (score 0.00)\nsurvived:\n  M   f:3   bool flipped   True → False\n") {
		t.Fatalf("exit %d\n%s\n%s", code, out, errs)
	}
	if !strings.Contains(errs, "over half the -timeout 300ms: each mutant gets ") {
		t.Fatalf("no warning: %q", errs)
	}
	if got := mutantLimit(10*time.Second, 7*time.Second); got != 21*time.Second {
		t.Fatalf("limit of a 7s suite under 10s: %v", got)
	}
	if got := mutantLimit(10*time.Second, time.Second); got != 10*time.Second {
		t.Fatalf("limit of a 1s suite under 10s: %v", got)
	}
}

// TestMutate_TimeFirst: when a case's result and the end of the mutant's time are both there,
// the time decides (a timeout, not a plain kill); a result that comes after the deadline is a
// timeout too.
func TestMutate_TimeFirst(t *testing.T) {
	for i := 0; i < 200; i++ {
		done, up := make(chan CaseResult, 1), make(chan time.Time, 1)
		done <- CaseResult{OK: false}
		up <- time.Now()
		if _, isUp, finished := await(done, up, time.Now().Add(time.Hour)); !isUp || !finished {
			t.Fatalf("both ready: up %v, finished %v", isUp, finished)
		}
	}
	done := make(chan CaseResult, 1)
	done <- CaseResult{OK: false}
	if _, isUp, _ := await(done, make(chan time.Time), time.Now().Add(-time.Millisecond)); !isUp {
		t.Fatal("a result after the deadline: not a timeout")
	}
	done <- CaseResult{OK: false}
	if _, isUp, _ := await(done, make(chan time.Time), time.Now().Add(time.Hour)); isUp {
		t.Fatal("a result before the deadline: a timeout")
	}
}

// TestMutate_SuiteTimesOut: under --mutate a suite that runs out of time unmutated measures
// nothing either: the timeout's report, then the refusal; at --mutate's default -timeout the
// message says how to raise it.
func TestMutate_SuiteTimesOut(t *testing.T) {
	setMutants(t)
	cs := append(mutantCases(), Case{Module: "M", Name: "slow", Pure: func() bool { time.Sleep(time.Hour); return true }})
	for _, tc := range []struct {
		args []string
		hint bool
	}{{nil, true}, {[]string{"-timeout", "300ms"}, false}} {
		t.Setenv(EnvDeadline, time.Now().Add(300*time.Millisecond).Format(time.RFC3339Nano))
		var out, errb bytes.Buffer
		code := Main(cs, tables, nil, nil, nil, nil, nil, web.Options{}, append([]string{"--mutate"}, tc.args...), &out, &errb)
		if code != 1 || !strings.HasSuffix(out.String(), "4 passed, 1 failed\ntests fail: fix them before measuring mutants\n") {
			t.Fatalf("%v: exit %d\n%s", tc.args, code, out.String())
		}
		if got := strings.Contains(errb.String(), "the tests timed out after "); !got {
			t.Fatalf("%v: stderr %q", tc.args, errb.String())
		}
		if got := strings.HasSuffix(errb.String(), " (--mutate's default 10s; raise it with -timeout)\n"); got != tc.hint {
			t.Fatalf("%v: hint %v, want %v: stderr %q", tc.args, got, tc.hint, errb.String())
		}
	}
}

// TestMutate_Refusals: a suite that fails measures nothing (after its normal report); no tests
// measure nothing either (exit 1 only with --min).
func TestMutate_Refusals(t *testing.T) {
	setMutants(t)
	cs := append(mutantCases(), Case{Module: "M", Name: "broken", Pure: func() bool { return false }})
	out, _, code := runMutate(cs)
	if code != 1 || !strings.HasSuffix(out, "FAIL M \"broken\"\n4 passed, 1 failed\ntests fail: fix them before measuring mutants\n") {
		t.Fatalf("exit %d\n%s", code, out)
	}
	out, errs, code := runMutate(cs, "--json")
	if code != 1 || strings.Contains(out, "tests fail") || !strings.HasSuffix(errs, "tests fail: fix them before measuring mutants\n") {
		t.Fatalf("--json: exit %d\n%s\n%s", code, out, errs)
	}
	out, _, code = runMutate(nil)
	if code != 0 || out != "0 passed, 0 failed\nno tests: nothing can kill a mutant\n" {
		t.Fatalf("no tests: exit %d\n%s", code, out)
	}
	if _, _, code = runMutate(nil, "--min", "0.5"); code != 1 {
		t.Fatalf("no tests with --min: exit %d", code)
	}
	var o, e bytes.Buffer
	SetMutants(nil, nil, 0, 0)
	if code := Main(nil, tables, nil, nil, nil, nil, nil, web.Options{}, []string{"--mutate"}, &o, &e); code != 2 || !strings.Contains(e.String(), "not built with mutants") {
		t.Fatalf("no mutants registered: exit %d %s", code, e.String())
	}
}

// TestMutate_Continuation: with a log, a timed-out mutant ends the program (logged, exit
// ExitMutantTimedOut); the next run, from the next id, skips the suite's run, reads the log and
// reports every mutant; a status line names the mutant and case running.
func TestMutate_Continuation(t *testing.T) {
	setMutants(t)
	dir := t.TempDir()
	log, status := filepath.Join(dir, "log"), filepath.Join(dir, "status")
	t.Setenv(EnvMutateLog, log)
	t.Setenv(EnvStatus, status)
	out, _, code := runMutate(mutantCases())
	if code != ExitMutantTimedOut || !strings.HasSuffix(out, "4 passed, 0 failed\n") {
		t.Fatalf("exit %d\n%s", code, out)
	}
	logged, err := ReadMutantLog(log)
	if err != nil || len(logged) != 3 || logged[3].Note != "timed out" {
		t.Fatalf("log %v %v", logged, err)
	}
	t.Setenv(EnvMutateFrom, "4")
	out, _, code = runMutate(mutantCases())
	if code != 0 || out != "5 mutants: 4 killed (of which 1 timed out), 1 survived  (score 0.80)\nsurvived:\n  Todos.Index   update:37   redirect → rerender   Redirect (Todos.Index {}) → Rerender\n" {
		t.Fatalf("exit %d\n%s", code, out)
	}
	if b, _ := os.ReadFile(status); len(b) != 0 {
		t.Fatalf("status left %q", b)
	}
	if id, mod, name, ok := ParseMutantStatus(MutantStatus(7, "Todos.Index", "a\tb")); !ok || id != 7 || mod != "Todos.Index" || name != "a\tb" {
		t.Fatalf("status round trip: %d %q %q %v", id, mod, name, ok)
	}
	if _, _, _, ok := ParseMutantStatus("M\tname\n"); ok {
		t.Fatal("a test's status line read as a mutant's")
	}
}

func TestOwnModuleFirst(t *testing.T) {
	cases := []Case{{Module: "A", Name: "a1"}, {Module: "B", Name: "b1"}, {Module: "A", Name: "a2"}, {Module: "B", Name: "b2"}}
	var got []string
	for _, c := range ownModuleFirst(cases, "B") {
		got = append(got, c.Module+"."+c.Name)
	}
	if want := "B.b1 B.b2 A.a1 A.a2"; strings.Join(got, " ") != want {
		t.Errorf("ownModuleFirst(…, B) = %v, want %s", got, want)
	}
	// a module with no cases: the suite order, every case kept
	got = nil
	for _, c := range ownModuleFirst(cases, "C") {
		got = append(got, c.Module+"."+c.Name)
	}
	if want := "A.a1 B.b1 A.a2 B.b2"; strings.Join(got, " ") != want {
		t.Errorf("ownModuleFirst(…, C) = %v, want %s", got, want)
	}
}

// TestMutate_Property: under --mutate a property counts as one test: one that fails under a
// mutant kills it, as a plain test does. The seed is fixed and nothing is shrunk or saved, so
// two runs give the same score and no fuzz directory is written.
func TestMutate_Property(t *testing.T) {
	setMutants(t)
	SetMutants(&active, []Mutant{
		{ID: 1, Module: "M", Decl: "f", Kind: "bool flipped", Line: 3, Before: "True", After: "False"},
		{ID: 2, Module: "M", Decl: "g", Kind: "bool flipped", Line: 5, Before: "True", After: "False"},
	}, 0, 0)
	dbDir := t.TempDir()
	t.Setenv(EnvFuzzDB, dbDir)
	t.Setenv(EnvSeed, "")
	t.Setenv(EnvFuzz, "")
	cases := []Case{{Module: "M", Name: "p", Prop: listProp(func(xs []int64) bool { return active != 1 || len(xs) < 3 })}}
	var outs []string
	for range 2 {
		out, errs, code := runMutate(cases)
		if code != 0 || !strings.Contains(out, "2 mutants: 1 killed, 1 survived  (score 0.50)\n") {
			t.Fatalf("exit %d\n%s\n%s", code, out, errs)
		}
		outs = append(outs, out)
	}
	if outs[0] != outs[1] {
		t.Errorf("two runs differ:\n%s\n%s", outs[0], outs[1])
	}
	if es, _ := os.ReadDir(dbDir); len(es) != 0 {
		t.Errorf("--mutate wrote %v", es)
	}
}
