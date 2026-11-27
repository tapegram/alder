package format

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"alder/internal/diag"
	"alder/internal/names"
	"alder/internal/types"
)

const migLayout = "module Layout\n\nPage : { x: Int } -> Html a\nPage props = <p>x</p>\n"
const migAdminLayout = "module Admin.Layout\n\nPage : { x: Int } -> Html a\nPage props = <p>x</p>\n"
const migTableTodo = "module Table.Todo\n\ntype Todo = { title: Text }\n\nfind : Int -> Todo\nfind n = ?find\n"
const migIndex = "module Todos.Index\n\npage \"/todos\" { }\n"

const migTodos = "module Todos\n\nimport Layout\nimport Admin.Layout\nimport Table.Todo\nimport Todos.Index\n\n" +
	"a : Table.Todo\na = Table.Todo.find 1\n\n" +
	"v : Html a\nv = <Admin.Layout.Page x={1}><a href={Todos.Index {}}>y</a></Admin.Layout.Page>\n"

// migrateIn loads a project of files (path -> source) and migrates file.
func migrateIn(t *testing.T, files map[string]string, file string) (string, bool) {
	t.Helper()
	dir := t.TempDir()
	for p, s := range files {
		full := filepath.Join(dir, p)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(s), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	std, err := types.LoadStd()
	if err != nil {
		t.Fatal(err)
	}
	prog, bag, ioErrs := names.LoadOverlay(dir, std, nil)
	if len(ioErrs) > 0 {
		t.Fatal(ioErrs)
	}
	prog.Resolve(bag)
	out, changed, err := Migrate(bag, file, []byte(files[file]), nil)
	if err != nil {
		t.Fatal(err)
	}
	return string(out), changed
}

func migProject(todos string) map[string]string {
	return map[string]string{
		"app/Layout.alder":       migLayout,
		"app/Admin/Layout.alder": migAdminLayout,
		"app/Table/Todo.alder":   migTableTodo,
		"app/Todos/Index.alder":  migIndex,
		"app/Todos.alder":        todos,
	}
}

func TestMigrate_FullPaths(t *testing.T) {
	out, changed := migrateIn(t, migProject(migTodos), "app/Todos.alder")
	if !changed {
		t.Fatal("not changed")
	}
	for _, want := range []string{"import Admin.Layout as AdminLayout\n", "a : Todo\n", "a = Todo.find 1\n", "<AdminLayout.Page x={1}>", "</AdminLayout.Page>", "import Layout\n", "import Table.Todo\n", "Todos.Index {}"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in:\n%s", want, out)
		}
	}
	if strings.Contains(out, "Table.Todo.find") || strings.Contains(out, "Admin.Layout.Page") {
		t.Errorf("full paths remain:\n%s", out)
	}
}

func TestMigrate_Idempotent(t *testing.T) {
	out, _ := migrateIn(t, migProject(migTodos), "app/Todos.alder")
	again, changed := migrateIn(t, migProject(out), "app/Todos.alder")
	if changed || again != out {
		t.Fatalf("second migration changed the file:\n%s", again)
	}
}

func TestMigrate_KeepsPagesFull(t *testing.T) {
	src := "module Todos\n\nimport Todos.Index\n\nv : Html a\nv = <a href={Todos.Index {}}>y</a>\n"
	out, changed := migrateIn(t, migProject(src), "app/Todos.alder")
	if changed || out != src {
		t.Fatalf("changed:\n%s", out)
	}
}

// An unrelated error elsewhere in the file does not stop the migration.
func TestMigrate_WithTypeErrors(t *testing.T) {
	src := migTodos + "\nbad : Int\nbad = nope\n"
	out, changed := migrateIn(t, migProject(src), "app/Todos.alder")
	if !changed || !strings.Contains(out, "a = Todo.find 1") || !strings.Contains(out, "import Admin.Layout as AdminLayout") {
		t.Fatalf("not migrated:\n%s", out)
	}
}

// Duplicate and overlapping fixes cannot corrupt the output, and a fix of another code on an
// E0205's span is never applied.
func TestMigrate_SharedAndOverlappingSpans(t *testing.T) {
	src := "module Use\n\nb = Table.Todo.Open\n"
	sp := diag.Span{File: "app/Use.alder", Start: diag.Pos{Line: 3, Col: 5}, End: diag.Pos{Line: 3, Col: 20}}
	bag := &diag.Bag{}
	bag.Error(diag.FullPath, sp, "full path").Fix = diag.ReplaceSpan(sp, "Todo.Open")
	bag.Error(diag.FullPath, sp, "full path again").Fix = diag.ReplaceSpan(sp, "Todo.Open")
	inner := diag.Span{File: "app/Use.alder", Start: diag.Pos{Line: 3, Col: 5}, End: diag.Pos{Line: 3, Col: 15}}
	bag.Error(diag.FullPath, inner, "overlap").Fix = diag.ReplaceSpan(inner, "Todo")
	bag.Error("E0203", sp, "other code").Fix = diag.ReplaceSpan(sp, "garbage")
	bag.Error(diag.FullPath, diag.Span{File: "app/Other.alder", Start: sp.Start, End: sp.End}, "other file").Fix = diag.ReplaceSpan(sp, "garbage")
	out, changed, err := Migrate(bag, "app/Use.alder", []byte(src), nil)
	if err != nil || !changed || string(out) != "module Use\n\nb = Todo.Open\n" {
		t.Fatalf("changed=%v err=%v:\n%s", changed, err, out)
	}
}

// #308: a write route's handler still named `update` (W0013) is renamed, and so is a reference to
// a handler already renamed (an E0002 whose fix is marked Migration); another E0002's fix is not
// applied.
func TestMigrate_HandlerRenameFixes(t *testing.T) {
	src := "module Todos\n\nA.update : Int\nA.update = 1\n\nb = A.update\n\nc = nope\n"
	file := "app/Todos.alder"
	at := func(l1, c1, l2, c2 int) diag.Span {
		return diag.Span{File: file, Start: diag.Pos{Line: l1, Col: c1}, End: diag.Pos{Line: l2, Col: c2}}
	}
	bag := &diag.Bag{}
	bag.Warn(diag.HandlerRenamed, at(3, 1, 3, 9), "renamed").Fix = diag.ReplaceSpan(at(3, 3, 4, 9), "post : Int\nA.post")
	ref := bag.Error(diag.UnknownName, at(6, 5, 6, 13), "now A.post")
	ref.Fix = diag.ReplaceSpan(at(6, 5, 6, 13), "A.post")
	ref.Fix.Migration = true
	bag.Error(diag.UnknownName, at(8, 5, 8, 9), "no nope").Fix = diag.ReplaceSpan(at(8, 5, 8, 9), "garbage")
	out, changed, err := Migrate(bag, file, []byte(src), nil)
	if want := "module Todos\n\nA.post : Int\nA.post = 1\n\nb = A.post\n\nc = nope\n"; err != nil || !changed || string(out) != want {
		t.Fatalf("changed=%v err=%v:\n%s", changed, err, out)
	}
}

// A fix of a group (a handler rename across files) is applied only when the run migrates every
// file of its group; a fix with no group is applied regardless.
func TestMigrate_GroupNeedsEveryFile(t *testing.T) {
	src := "module Todos\n\nA.update : Int\nA.update = 1\n\nb = Table.Todo.find 1\n"
	file := "app/Todos.alder"
	at := func(f string, l1, c1, l2, c2 int) diag.Span {
		return diag.Span{File: f, Start: diag.Pos{Line: l1, Col: c1}, End: diag.Pos{Line: l2, Col: c2}}
	}
	bag := &diag.Bag{}
	decl := bag.Warn(diag.HandlerRenamed, at(file, 3, 1, 3, 9), "renamed")
	decl.Fix = diag.ReplaceSpan(at(file, 3, 3, 4, 9), "post : Int\nA.post")
	decl.Fix.Group = "W0013 Todos.A"
	ref := bag.Warn(diag.HandlerRenamed, at("app/Checks.alder", 5, 1, 5, 15), "renamed")
	ref.Fix = diag.ReplaceSpan(at("app/Checks.alder", 5, 1, 5, 15), "Todos.A.post")
	ref.Fix.Group = "W0013 Todos.A"
	bag.Error(diag.FullPath, at(file, 6, 5, 6, 20), "full path").Fix = diag.ReplaceSpan(at(file, 6, 5, 6, 20), "Todo.find")
	only := func(f string) bool { return f == file }
	out, _, err := Migrate(bag, file, []byte(src), only)
	if want := "module Todos\n\nA.update : Int\nA.update = 1\n\nb = Todo.find 1\n"; err != nil || string(out) != want {
		t.Fatalf("one file: err=%v:\n%q", err, out)
	}
	both := func(f string) bool { return f == file || f == "app/Checks.alder" }
	out, _, err = Migrate(bag, file, []byte(src), both)
	if want := "module Todos\n\nA.post : Int\nA.post = 1\n\nb = Todo.find 1\n"; err != nil || string(out) != want {
		t.Fatalf("both files: err=%v:\n%s", err, out)
	}
}

// #310: a route module's helper that does not state its need (W0014) gets its `requires` clause;
// two warnings of one helper carry the same fix, applied once.
func TestMigrate_RequiresClauseFix(t *testing.T) {
	src := "module Todos\n\nboth : Query Int\nboth = ?both\n"
	file := "app/Todos.alder"
	at := func(l1, c1, l2, c2 int) diag.Span {
		return diag.Span{File: file, Start: diag.Pos{Line: l1, Col: c1}, End: diag.Pos{Line: l2, Col: c2}}
	}
	bag := &diag.Bag{}
	bag.Warn(diag.HelperNeedUnstated, at(4, 8, 4, 13), "needs user").Fix = diag.InsertAt(3, 17, " requires user, org")
	bag.Warn(diag.HelperNeedUnstated, at(4, 8, 4, 13), "needs org").Fix = diag.InsertAt(3, 17, " requires user, org")
	out, changed, err := Migrate(bag, file, []byte(src), nil)
	if want := "module Todos\n\nboth : Query Int requires user, org\nboth = ?both\n"; err != nil || !changed || string(out) != want {
		t.Fatalf("changed=%v err=%v:\n%s", changed, err, out)
	}
}

// #327: a fix marked Held is not applied, and neither is the rest of its group, even when every
// file of the group is migrated; a held fix with no group stands alone.
func TestMigrate_HeldFixWaits(t *testing.T) {
	src := "module Todos\n\nA.update : Int\nA.update = 1\n\nb = A.update\n\nh : Int\nh = 1\n"
	file := "app/Todos.alder"
	at := func(f string, l1, c1, l2, c2 int) diag.Span {
		return diag.Span{File: f, Start: diag.Pos{Line: l1, Col: c1}, End: diag.Pos{Line: l2, Col: c2}}
	}
	bag := &diag.Bag{}
	decl := bag.Warn(diag.HandlerRenamed, at(file, 3, 1, 3, 9), "renamed")
	decl.Fix = diag.ReplaceSpan(at(file, 3, 3, 4, 9), "post : Int\nA.post")
	decl.Fix.Group = "W0013 Todos.A"
	self := bag.Warn(diag.HandlerRenamed, at(file, 6, 5, 6, 13), "renamed")
	self.Fix = diag.ReplaceSpan(at(file, 6, 5, 6, 13), "A.post")
	self.Fix.Group = "W0013 Todos.A"
	ref := bag.Warn(diag.HandlerRenamed, at("app/Checks.alder", 5, 1, 5, 15), "renamed")
	ref.Fix = diag.ReplaceSpan(at("app/Checks.alder", 5, 1, 5, 15), "Todos.A.post")
	ref.Fix.Group = "W0013 Todos.A"
	ref.Fix.Held = true
	clause := bag.Warn(diag.HelperNeedUnstated, at(file, 9, 5, 9, 6), "needs user")
	clause.Fix = diag.InsertAt(8, 8, " requires user")
	clause.Fix.Held = true
	out, changed, err := Migrate(bag, file, []byte(src), nil)
	if err != nil || changed || string(out) != src {
		t.Fatalf("changed=%v err=%v:\n%s", changed, err, out)
	}
	ref.Fix.Held, clause.Fix.Held = false, false
	out, _, err = Migrate(bag, file, []byte(src), nil)
	if want := "module Todos\n\nA.post : Int\nA.post = 1\n\nb = A.post\n\nh : Int requires user\nh = 1\n"; err != nil || string(out) != want {
		t.Fatalf("nothing held: err=%v:\n%s", err, out)
	}
}
