package rtesting

import (
	"os"
	"strings"
	"testing"
	"time"

	"alder/rt/fuzz"
)

// A health check that stops a property says so: a slow run was stopped on, an overrunning draw
// never made its run.
func TestProp_StoppedWording(t *testing.T) {
	defer func(d time.Duration) { attemptLimit = d }(attemptLimit)
	attemptLimit = 50 * time.Millisecond
	calls := 0
	slow := ForAll(fuzz.IntF(), func(int64) bool {
		calls++
		if calls == 3 {
			time.Sleep(80 * time.Millisecond)
		}
		return true
	})
	slow.Args = []Arg{{Name: "n", Type: "Int"}}
	out := report(t, runProp(slow, Options{}), false)
	if !strings.HasPrefix(out, "FAIL M \"p\"  (stopped on run 3 of 100)\n") || strings.Contains(out, "failed on run") {
		t.Errorf("slow:\n%s", out)
	}

	big := ForAll(fuzz.Fuzzer[int64](func(c *fuzz.Choices) int64 {
		for range 9000 {
			c.Choice(1)
		}
		return 0
	}), func(int64) bool { return true })
	out = report(t, runProp(big, Options{}), false)
	if !strings.HasPrefix(out, "FAIL M \"p\"  (stopped before run 1 of 100)\n") || strings.Contains(out, "failed on run") {
		t.Errorf("draws too much:\n%s", out)
	}
}

// A too-slow case is saved like a failing one, so the next run replays it first.
func TestProp_TooSlowIsSaved(t *testing.T) {
	defer func(d time.Duration) { attemptLimit = d }(attemptLimit)
	attemptLimit = 30 * time.Millisecond
	dir := t.TempDir()
	calls := 0
	p := ForAll(fuzz.IntF(), func(int64) bool {
		calls++
		if calls == 2 {
			time.Sleep(50 * time.Millisecond)
		}
		return true
	})
	p.Args = []Arg{{Name: "n", Type: "Int"}}
	r := runProp(p, Options{FuzzDB: dir})
	if r.OK || !strings.Contains(r.Error, "run 2 took ") {
		t.Fatalf("got %+v", r)
	}
	saved, ok := readSaved(savedPath(dir, "M", "p"))
	if !ok || len(saved) == 0 {
		t.Fatalf("the too-slow case was not saved: %v %v", saved, ok)
	}
	// the saved case replays first, so the next run is slow on it again
	calls = 1
	if r := runProp(p, Options{FuzzDB: dir}); r.OK || r.Run != savedRun {
		t.Errorf("the saved slow case did not replay first: %+v", r)
	}
}

// The attempt limit applies to the shrinker's candidates too: once one runs over, shrinking
// stops with the best case so far instead of spending the budget on a body that is slow.
func TestProp_SlowCandidateStopsShrinking(t *testing.T) {
	o := Options{Seed: 3, SeedSet: true}
	calls, firstFail := 0, 0
	probe := listProp(func(xs []int64) bool {
		calls++
		if len(xs) >= 4 && firstFail == 0 {
			firstFail = calls
		}
		return len(xs) < 4
	})
	if r := runProp(probe, o); r.OK || firstFail == 0 || r.Shrinks == 0 {
		t.Fatalf("probe: %+v (first failure at call %d)", r, firstFail)
	}

	defer func(d time.Duration) { attemptLimit = d }(attemptLimit)
	attemptLimit = 20 * time.Millisecond
	calls = 0
	slow := listProp(func(xs []int64) bool {
		calls++
		if calls > firstFail {
			time.Sleep(30 * time.Millisecond)
		}
		return len(xs) < 4
	})
	r := runProp(slow, o)
	if r.OK || calls > firstFail+2 || r.Args[0].Value == "[0, 0, 0, 0]" { // not shrunk to the minimum
		t.Errorf("got %+v after %d calls (the failure was call %d)", r, calls, firstFail)
	}
}

// Fuzz.oneOf of nothing fails the test with its own message, not a crash.
func TestProp_OneOfEmptyFails(t *testing.T) {
	p := ForAll(fuzz.OneOf[int64](nil), func(int64) bool { return true })
	p.Args = []Arg{{Name: "n", Type: "Int"}}
	r := runProp(p, Options{})
	if r.OK || r.Error != "Fuzz.oneOf: the list of fuzzers is empty" {
		t.Errorf("got %+v", r)
	}
}

// The paste-ready case is capped by how long its lines are, not only by how many there are: a
// 500-element list on one line is no case to paste.
func TestProp_PasteCaseWideLine(t *testing.T) {
	p := pasteProp(3)
	p.Lit = func(vs []any) []string { return []string{"[" + strings.Repeat("0, ", 1000) + "0]"} }
	r := runProp(p, Options{})
	if r.Paste != "" || !strings.Contains(report(t, r, false), pasteFallback) {
		t.Errorf("a wide case was kept:\n%s", report(t, r, false))
	}
}

// A literal with a record in it that is no atom needs parentheses in a match subject (`match
// Just { a: 1 } {` reads the record as the arms), though it does not start with `{`.
func TestPaste_MatchSubjectNestedRecord(t *testing.T) {
	for _, c := range []struct{ mode, lit, want string }{
		{"P", "Just { a: 1 }", "(Just { a: 1 })"},
		{"P", `Just { a: "}" }`, `(Just { a: "}" })`},
		{"P", "{ a: 1 }", "({ a: 1 })"},
		{"P", "[{ a: 1 }]", "[{ a: 1 }]"},
		{"P", "Just ({ a: 1 })", "Just ({ a: 1 })"},
		{"P", `"{"`, `"{"`},
		{"A", "Just { a: 1 }", "(Just { a: 1 })"},
		{"p", "Just { a: 1 }", "Just { a: 1 }"},
	} {
		if got := Paste("\x00"+c.mode+"0\x00", []string{c.lit}); got != c.want {
			t.Errorf("%s slot of %s: got %s, want %s", c.mode, c.lit, got, c.want)
		}
	}
}

// A plain test whose name ends in tab, digits, tab, digits is not a property's status line.
func TestParseStatus_PlainNameLikeARecord(t *testing.T) {
	mod, name, seed, run, ok := ParseStatus("M\tweird\t12\t3\n")
	if !ok || mod != "M" || name != "weird\t12\t3" || seed != 0 || run != 0 {
		t.Errorf("plain: %q %q %d %d %v", mod, name, seed, run, ok)
	}
	// a property whose own name looks like one
	var status = t.TempDir() + "/status"
	p := listProp(func(xs []int64) bool {
		b, _ := os.ReadFile(status)
		mod, name, seed, run, ok := ParseStatus(string(b))
		if !ok || mod != "M" || name != "a\t5\t6" || seed != 9 || run < 1 {
			t.Errorf("property: %q %q %d %d %v", mod, name, seed, run, ok)
		}
		return true
	})
	Run([]Case{{Module: "M", Name: "a\t5\t6", Prop: p}}, Options{Tables: tables, Seed: 9, SeedSet: true, Status: status, Fuzz: 2})
}

// An unseeded run picks a random seed below 10^10 (and the seeds differ).
func TestRandomSeed(t *testing.T) {
	seen := map[uint64]bool{}
	for range 5 {
		s := randomSeed()
		if s >= 1e10 {
			t.Fatalf("seed %d is not below 10^10", s)
		}
		seen[s] = true
	}
	if len(seen) == 1 {
		t.Error("five runs picked the same seed")
	}
}
