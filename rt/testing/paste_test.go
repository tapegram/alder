package rtesting

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"alder/rt/fuzz"
)

// Paste fills a paste-ready case's template: an atom slot ('a') parenthesises a literal that is
// no atom, an application slot ('p') takes it as it is, and their capitals (inside a match
// subject) also parenthesise one that starts with `{`.
func TestPaste(t *testing.T) {
	tmpl := "test \"t (case)\" = f \x00a0\x00 == \x00p0\x00 && g \x00a1\x00 \x00p1\x00 && h \x00A2\x00 \x00P2\x00 \x00p2\x00"
	got := Paste(tmpl, []string{"Just -1", "-2", "{ a: 1 }"})
	want := `test "t (case)" = f (Just -1) == Just -1 && g (-2) -2 && h ({ a: 1 }) ({ a: 1 }) { a: 1 }`
	if got != want {
		t.Errorf("got  %s\nwant %s", got, want)
	}
	if got := Paste("\x00a0\x00 \x00a1\x00", []string{"[1]", "Leaf"}); got != "[1] Leaf" {
		t.Errorf("atoms: %s", got)
	}
}

// pasteProp is listProp failing on two elements, its Case printing the case over lines lines.
func pasteProp(lines int) *Property {
	p := listProp(func(xs []int64) bool { return len(xs) < 2 })
	p.Case = func(lits []string) string {
		return "test \"p (case)\" = {" + strings.Repeat("\n  let a = 1", lines-3) + "\n  Task.succeed (List.length " + lits[0] + " < 2)\n}"
	}
	return p
}

// The report keeps a failing case as a plain test; one over 20 lines, or one whose literal has
// no Alder (a Text with a control character), says to paste the values instead.
func TestProp_PasteCase(t *testing.T) {
	r := runProp(pasteProp(20), Options{})
	out := report(t, r, false)
	if !strings.Contains(out, "  to keep this case:\n    test \"p (case)\" = {\n      let a = 1\n") || !strings.HasSuffix(out, "\n      Task.succeed (List.length [0, 0] < 2)\n    }\n") {
		t.Errorf("report:\n%s", out)
	}
	if js := report(t, r, true); !strings.Contains(js, `"case":"test \"p (case)\" = {\n  let a = 1`) {
		t.Errorf("json:\n%s", js)
	}
	for name, p := range map[string]*Property{"long": pasteProp(21), "no literal": func() *Property {
		p := pasteProp(3)
		p.Lit = func(vs []any) []string { return []string{fuzz.TextLit("\x01")} }
		return p
	}()} {
		r := runProp(p, Options{})
		out := report(t, r, false)
		if r.Paste != "" || strings.Contains(out, "to keep this case") || !strings.HasSuffix(out, "  seed: "+strconv.FormatUint(r.Seed, 10)+"\n  paste the values above into a plain test\n") {
			t.Errorf("%s: report:\n%s", name, out)
		}
	}
	// a passing property says nothing of pasting
	if out := report(t, runProp(listProp(func([]int64) bool { return true }), Options{}), false); strings.Contains(out, "paste") {
		t.Errorf("pass: %s", out)
	}
}

// Before each attempt Main's status file names the property, its seed and the run, so a crash
// of the whole program (a stack overflow) can report them; a --mutate run writes no such line.
func TestProp_StatusSeedRun(t *testing.T) {
	status := filepath.Join(t.TempDir(), "status")
	var lines []string
	p := listProp(func(xs []int64) bool {
		b, _ := os.ReadFile(status)
		lines = append(lines, string(b))
		return len(xs) < 2
	})
	r := runProp(p, Options{Seed: 9, SeedSet: true, Status: status})
	if r.OK || len(lines) < r.Run {
		t.Fatalf("got %+v", r)
	}
	if lines[0] != "M\tp\t9\t1\n" || lines[r.Run-1] != "M\tp\t9\t"+strconv.Itoa(r.Run)+"\n" {
		t.Errorf("status lines %q", lines[:r.Run])
	}
	mod, name, seed, run, ok := ParseStatus(lines[r.Run-1])
	if !ok || mod != "M" || name != "p" || seed != 9 || run != r.Run {
		t.Errorf("ParseStatus: %q %q %d %d %v", mod, name, seed, run, ok)
	}
	if mod, name, seed, run, ok := ParseStatus("M\ta plain test\n"); !ok || mod != "M" || name != "a plain test" || seed != 0 || run != 0 {
		t.Errorf("plain: %q %q %d %d %v", mod, name, seed, run, ok)
	}
	if _, _, _, _, ok := ParseStatus(""); ok {
		t.Error("empty status parsed")
	}
	os.WriteFile(status, nil, 0o644)
	lines = nil
	runProp(p, Options{Status: status, Mutating: true})
	for _, l := range lines {
		if l != "" {
			t.Fatalf("--mutate wrote %q", l)
		}
	}
	// a crash report: the seed and the run, no arguments
	out := report(t, CaseResult{Module: "M", Name: "p", Error: "fatal error: stack overflow", Seed: 9, Run: 4, Crash: true}, false)
	if out != "FAIL M \"p\"\n  error: fatal error: stack overflow\n  seed: 9\n  run: 4\n" {
		t.Errorf("crash report %q", out)
	}
	if js := report(t, CaseResult{Module: "M", Name: "p", Error: "x", Seed: 9, Run: 4, Crash: true}, true); !strings.Contains(js, `"property":{"runs":0,"seed":9,"run":4,`) {
		t.Errorf("crash json %s", js)
	}
}

// While a saved failure replays, the status line's run is savedRun: a crash then is reported
// as the saved case's (no seed reproduces it), not as run 1 of the seed.
func TestProp_StatusSavedReplay(t *testing.T) {
	dir := t.TempDir()
	status := filepath.Join(dir, "status")
	var first string
	p := listProp(func(xs []int64) bool {
		if first == "" {
			b, _ := os.ReadFile(status)
			first = string(b)
		}
		return len(xs) < 2
	})
	runProp(p, Options{FuzzDB: dir, Seed: 9, SeedSet: true}) // saves the failure
	runProp(p, Options{FuzzDB: dir, Seed: 9, SeedSet: true, Status: status})
	if first != "M\tp\t9\t-1\n" {
		t.Fatalf("status while replaying %q", first)
	}
	if _, _, _, run, ok := ParseStatus(first); !ok || run != savedRun {
		t.Errorf("ParseStatus: run %d %v", run, ok)
	}
	out := report(t, CaseResult{Module: "M", Name: "p", Error: "fatal error: stack overflow", Seed: 9, Run: savedRun, Crash: true}, false)
	if out != "FAIL M \"p\"\n  error: fatal error: stack overflow\n  run: the saved failing case, replayed first (build/<app>/fuzz/M/)\n" {
		t.Errorf("crash report %q", out)
	}
}
