package check

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"alder/internal/diag"
)

// #327: while a module has errors, the `requires` clause W0014 adds is held (diag.Fix.Held) when
// the helper's need comes through another module, and not when it is the module's own; with no
// module in error nothing is held.
func TestRequiresClauseHeldThroughAnotherModule(t *testing.T) {
	for _, broken := range []bool{false, true} {
		dir := t.TempDir()
		if err := os.CopyFS(dir, os.DirFS("../../testdata/corpus/W0014-route-helper")); err != nil {
			t.Fatal(err)
		}
		add := func(rel, text string) {
			p := filepath.Join(dir, rel)
			b, _ := os.ReadFile(p)
			if err := os.WriteFile(p, append(b, text...), 0o644); err != nil {
				t.Fatal(err)
			}
		}
		add("app/Table/Todo.alder", "\nmine : Select Todo requires user\nmine = Db.mine Todo\n")
		add("app/Todos.alder", "\nviaTable : Query Int\nviaTable = Todo.mine |> Db.count\n")
		if broken {
			add("app/Broken.alder", "module Broken\n\nx : Int\nx = nope\n")
		}
		held := map[int]bool{} // by the fix's line
		for _, d := range runDir(t, dir).Items {
			if d.Code == diag.HelperNeedUnstated && d.Span.File == "app/Todos.alder" {
				held[d.Fix.Line] = d.Fix.Held
			}
		}
		want := map[int]bool{12: false, 16: false, 20: false, 38: broken}
		if len(held) != len(want) {
			t.Fatalf("broken=%v: W0014 fixes at lines %v, want %v", broken, held, want)
		}
		for line, w := range want {
			if held[line] != w {
				t.Errorf("broken=%v: fix at line %d held=%v, want %v", broken, line, held[line], w)
			}
		}
	}
}

// #327: W0013's rename of a handler is held at its declaration when a module with errors imports
// the route's module (its references there were not checked), and at a reference from another
// module while any module has errors; a reference in the route's own module is not held itself
// (its group waits with the declaration).
func TestHandlerRenameHeld(t *testing.T) {
	cases := []struct {
		name, broken string
		want         map[string]bool // by file and fix line
	}{
		{"none", "", map[string]bool{"app/Todos.alder:5": false, "app/Todos.alder:9": false, "app/Checks.alder:6": false}},
		{"importer", "app/Checks.alder", map[string]bool{"app/Todos.alder:5": true, "app/Todos.alder:9": false}},
		{"unrelated", "app/App.alder", map[string]bool{"app/Todos.alder:5": false, "app/Todos.alder:9": false, "app/Checks.alder:6": true}},
	}
	for _, tc := range cases {
		dir := t.TempDir()
		if err := os.CopyFS(dir, os.DirFS("../../testdata/corpus/W0013-handler-update")); err != nil {
			t.Fatal(err)
		}
		if tc.broken != "" {
			p := filepath.Join(dir, tc.broken)
			b, _ := os.ReadFile(p)
			if err := os.WriteFile(p, append(b, "\nbrokenX : Int\nbrokenX = nope\n"...), 0o644); err != nil {
				t.Fatal(err)
			}
		}
		got := map[string]bool{}
		for _, d := range runDir(t, dir).Items {
			if d.Code == diag.HandlerRenamed {
				got[fmt.Sprintf("%s:%d", d.Span.File, d.Fix.Line)] = d.Fix.Held
			}
		}
		if fmt.Sprint(got) != fmt.Sprint(tc.want) {
			t.Errorf("%s: held %v, want %v", tc.name, got, tc.want)
		}
	}
}
