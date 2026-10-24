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
	out, changed, err := Migrate(bag, file, []byte(files[file]))
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
	out, changed, err := Migrate(bag, "app/Use.alder", []byte(src))
	if err != nil || !changed || string(out) != "module Use\n\nb = Todo.Open\n" {
		t.Fatalf("changed=%v err=%v:\n%s", changed, err, out)
	}
}
