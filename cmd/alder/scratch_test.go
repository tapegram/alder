package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"alder/internal/scratch"
)

func runScratch(t *testing.T, args ...string) (int, string, string) {
	t.Helper()
	var out, errb bytes.Buffer
	code := runScratchTo(withBuildDir(t, args), &out, &errb)
	return code, out.String(), errb.String()
}

func writeScratch(t *testing.T, path, text string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
}

// TestScratch runs `alder scratch` on one temporary copy of examples/todos, from inside it
// (serial: t.Chdir). The cases that run the program share a file, so they share its build.
func TestScratch(t *testing.T) {
	needGoTool(t)
	tmp := t.TempDir()
	app := filepath.Join(tmp, "todos")
	copyDir(t, todosDir(t), app)
	os.RemoveAll(filepath.Join(app, "build"))
	t.Chdir(app)
	writeScratch(t, "scratch.alder", "-- scratch\n> 1 + 1\n> Todo.countOpen []\n")

	t.Run("Text", func(t *testing.T) {
		code, stdout, stderr := runScratch(t, "scratch.alder")
		if code != 0 || !strings.Contains(stdout, "scratch.alder:2: 1 + 1\n  ⇒ 2 : Int\n") {
			t.Errorf("exit %d\nstdout: %s\nstderr: %s", code, stdout, stderr)
		}
	})
	t.Run("JSON", func(t *testing.T) {
		code, stdout, stderr := runScratch(t, "scratch.alder", "--json")
		var got struct {
			Watches []struct {
				Line  int
				Value string
				Type  string
			}
			Diagnostics []json.RawMessage
			Ran         bool
		}
		if err := json.Unmarshal([]byte(stdout), &got); err != nil {
			t.Fatalf("%v\nstdout: %s", err, stdout)
		}
		if code != 0 || !got.Ran || len(got.Watches) != 2 || got.Watches[0].Value != "2" || got.Watches[0].Type != "Int" || got.Watches[0].Line != 2 {
			t.Errorf("exit %d, got %+v\nstderr: %s", code, got, stderr)
		}
	})
	t.Run("DefaultFile", func(t *testing.T) {
		code, stdout, stderr := runScratch(t)
		if code != 0 || !strings.Contains(stdout, "scratch.alder:2: 1 + 1\n  ⇒ 2 : Int\n") {
			t.Errorf("exit %d\nstdout: %s\nstderr: %s", code, stdout, stderr)
		}
	})
	t.Run("WatchFlagIsAccepted", func(t *testing.T) {
		if code, _, stderr := runScratch(t, "--watch"); code != 0 {
			t.Errorf("exit %d\nstderr: %s", code, stderr)
		}
	})
	t.Run("ErrorsExit1", func(t *testing.T) {
		writeScratch(t, "err.scratch.alder", "> 1 + \"x\"\n")
		code, stdout, stderr := runScratch(t, "err.scratch.alder")
		if code != 1 || !strings.Contains(stderr, "err.scratch.alder:1:") {
			t.Errorf("exit %d\nstdout: %s\nstderr: %s", code, stdout, stderr)
		}
		code, stdout, _ = runScratch(t, "err.scratch.alder", "--json")
		if code != 1 || !strings.Contains(stdout, `"ran":false`) {
			t.Errorf("json: exit %d, stdout %s", code, stdout)
		}
	})
	t.Run("HoleExits1", func(t *testing.T) {
		writeScratch(t, "hole.scratch.alder", "> 1 + ?x\n")
		code, stdout, stderr := runScratch(t, "hole.scratch.alder")
		if code != 1 || !strings.Contains(stderr, "W0002") || !strings.Contains(stderr, "typed hole") {
			t.Errorf("exit %d\nstdout: %s\nstderr: %s", code, stdout, stderr)
		}
	})
	t.Run("EffectNotRun", func(t *testing.T) {
		writeScratch(t, "eff.scratch.alder", "> Db.from Todo |> Db.all\n")
		code, stdout, stderr := runScratch(t, "eff.scratch.alder")
		if code != 0 || !strings.Contains(stdout, "<Query>") || !strings.Contains(stdout, "(not run: scratch evaluates pure values only)") {
			t.Errorf("stdout: %s\nstderr: %s", stdout, stderr)
		}
	})
	t.Run("RefusesAppFile", func(t *testing.T) {
		writeScratch(t, "app/foo.scratch.alder", "> 1\n")
		code, _, stderr := runScratch(t, "app/foo.scratch.alder")
		want := "alder scratch: app/foo.scratch.alder is not a scratch file (a name ending in scratch.alder, outside app/)"
		if code != 2 || !strings.Contains(stderr, want) {
			t.Errorf("exit %d, stderr %s", code, stderr)
		}
		os.Remove("app/foo.scratch.alder")
	})
	t.Run("ModuleWithInHeader", func(t *testing.T) {
		writeScratch(t, "in.scratch.alder", "in Todo\n> 1\n")
		code, _, stderr := runScratch(t, "in.scratch.alder", "--module", "Probe")
		if code != 2 || !strings.Contains(stderr, "--in and --module do not combine") {
			t.Errorf("exit %d, stderr %s", code, stderr)
		}
		code, _, stderr = runScratch(t, "scratch.alder", "--module", "bad name")
		if code != 2 || !strings.Contains(stderr, "--module wants a module name") {
			t.Errorf("exit %d, stderr %s", code, stderr)
		}
	})
	t.Run("NoApp", func(t *testing.T) {
		empty := t.TempDir()
		writeScratch(t, filepath.Join(empty, "scratch.alder"), "> 1\n")
		code, _, stderr := runScratch(t, filepath.Join(empty, "scratch.alder"))
		if code != 2 || !strings.Contains(stderr, "alder scratch: no app found above") {
			t.Errorf("exit %d, stderr %s", code, stderr)
		}
	})
}

func TestScratchPrintsWatches(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		w    scratch.WatchResult
		want string
	}{
		{"multi-line", scratch.WatchResult{Line: 3, Expr: "f\n  x", Type: "Text", Value: "a\nb"}, "s:3: f\n  ⇒ a\n    b : Text\n"},
		{"truncated", scratch.WatchResult{Line: 1, Expr: "x", Type: "Text", Value: "a", Truncated: true}, "s:1: x\n  ⇒ a : Text\n  … (truncated at 64 KB)\n"},
		{"error", scratch.WatchResult{Line: 1, Expr: "x", Type: "Int", Error: "boom\nmore"}, "s:1: x\n  ✗ boom\n    more\n"},
		{"effect", scratch.WatchResult{Line: 1, Expr: "x", Type: "Query Int", Value: "<Query>", Note: "not run: scratch evaluates pure values only"}, "s:1: x\n  <Query> : Query Int   (not run: scratch evaluates pure values only)\n"},
		{"not run", scratch.WatchResult{Line: 1, Expr: "x", Type: "Int"}, "s:1: x\n  : Int\n"},
	}
	for _, c := range cases {
		var b bytes.Buffer
		printScratchWatch(&b, "s", c.w)
		if b.String() != c.want {
			t.Errorf("%s: got %q, want %q", c.name, b.String(), c.want)
		}
	}
}

func TestHelpJSON_Scratch(t *testing.T) {
	t.Parallel()
	for _, c := range commandTree {
		if c.Name != "scratch" {
			continue
		}
		have := map[string]bool{}
		for _, f := range c.Flags {
			have[f.Name] = true
		}
		for _, want := range []string{"--watch", "--json", "--build-dir", "--module"} {
			if !have[want] {
				t.Errorf("scratch lists no %s flag", want)
			}
		}
		if c.Usage != scratchUsage {
			t.Errorf("usage %q", c.Usage)
		}
		return
	}
	t.Error("scratch is not in commandTree")
}
