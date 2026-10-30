package main

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
)

func runBench(t *testing.T, args ...string) (int, string, string) {
	t.Helper()
	var out, errb bytes.Buffer
	code := runBenchTo(withBuildDir(t, args), &out, &errb)
	return code, out.String(), errb.String()
}

// TestBench_Rows runs `alder bench` on the fixture (testdata/bench/rows) once, text and --json,
// and checks the shape of what it measures, never a time: a row per step, the loads of a
// Rerender and of a followed Redirect, the redirect's two requests, the N+1 page's statements and
// its one repeat, none on the plain page.
func TestBench_Rows(t *testing.T) {
	t.Parallel()
	needGoTool(t)
	src, err := filepath.Abs("../../testdata/bench/rows")
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(t.TempDir(), "rows")
	copyDir(t, src, dir)
	const sel = `"index 100"|toggle|add|jump|slow`
	code, out, errs := runBench(t, dir, "-run", sel, "--count", "1", "--json")
	if code != 0 {
		t.Fatalf("exit %d\n%s\n%s", code, out, errs)
	}
	type measure struct {
		Step                               string
		Ms                                 float64
		Bytes, Requests, Loads, Steps, SQL int
		Allocs                             uint64
		Repeats                            []struct{ Count, Args int }
	}
	byTest := map[string][]measure{}
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		var r struct {
			Name     string
			OK       bool
			Warnings []string
			Bench    []measure
		}
		if err := json.Unmarshal([]byte(line), &r); err != nil || !r.OK {
			t.Fatalf("%q: %v", line, err)
		}
		byTest[r.Name] = r.Bench
		if (r.Name == "slow 100") != (len(r.Warnings) == 1) {
			t.Errorf("%s: warnings %q", r.Name, r.Warnings)
		}
	}
	steps := map[string][]string{
		"index 100":   {"GET /"},
		"toggle 1000": {"GET /", "POST /_m/Rows.Index/Toggle"},
		"add 1000":    {"GET /", "POST /_m/Rows.Index/Add"},
		"jump 1000":   {"GET /", "POST /_m/Rows.Index/Jump"},
		"slow 100":    {"GET /slow"},
	}
	if len(byTest) != len(steps) {
		t.Fatalf("tests %v", byTest)
	}
	for name, want := range steps {
		ms := byTest[name]
		if len(ms) != len(want) {
			t.Fatalf("%s: %+v", name, ms)
		}
		for i, m := range ms {
			if m.Step != want[i] || m.Bytes == 0 || m.Allocs == 0 || m.Requests < 1 {
				t.Errorf("%s step %d: %+v", name, i, m)
			}
		}
	}
	if m := byTest["toggle 1000"][1]; m.Loads != 2 || m.Requests != 1 {
		t.Errorf("toggle: %+v", m)
	}
	if m := byTest["jump 1000"][1]; m.Loads != 2 || m.Requests != 2 {
		t.Errorf("jump: %+v", m)
	}
	if m := byTest["slow 100"][0]; m.SQL != 101 || len(m.Repeats) != 1 || m.Repeats[0].Count != 100 || m.Repeats[0].Args != 100 {
		t.Errorf("slow: %+v", m)
	}
	if m := byTest["index 100"][0]; len(m.Repeats) != 0 || m.SQL != 1 || m.Loads != 1 {
		t.Errorf("index: %+v", m)
	}

	code, out, errs = runBench(t, dir, "-run", sel, "--count", "1")
	if code != 0 || !strings.Contains(out, "5 passed, 0 failed") {
		t.Fatalf("text: exit %d\n%s\n%s", code, out, errs)
	}
	for _, want := range []string{
		"ok   Rows.Bench \"toggle 1000\"\n  step ",
		"\n  POST /_m/Rows.Index/Jump ",
		"\n  warn: GET /slow: N+1: SELECT ",
		"100x (100 args)",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("text: no %q in\n%s", want, out)
		}
	}
}

// --mutate, --min, --fuzz and --seed have nothing to measure: the usage, exit 2.
func TestBench_RefusesMutateFuzz(t *testing.T) {
	t.Parallel()
	for _, args := range [][]string{{"--mutate"}, {"--fuzz", "10"}, {"--seed=3"}, {"--min", "0.5"}, {"--count", "0"}} {
		var out, errb bytes.Buffer
		code := runBenchTo(args, &out, &errb)
		if code != 2 || !strings.Contains(errb.String(), "alder bench") {
			t.Errorf("%v: exit %d, %q", args, code, errb.String())
		}
	}
}

func TestHelp_Bench(t *testing.T) {
	t.Parallel()
	var out bytes.Buffer
	if !helpTo([]string{"help", "bench"}, &out) || !strings.HasPrefix(out.String(), benchUsage+"\n") || !strings.Contains(out.String(), "median") {
		t.Errorf("%q", out.String())
	}
	out.Reset()
	writeHelpJSON(&out)
	if !strings.Contains(out.String(), `"name":"bench"`) {
		t.Error("help --json has no bench")
	}
}
