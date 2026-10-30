package generate

import (
	"strings"
	"testing"
)

// noteModule is a table module with a typed hole: the target of the table-name addresses.
const noteModule = "module Table.Note\n\ntype NoteId = brand Text\n\ntable Note {\n  id: NoteId @id,\n  title: Text,\n}\n\nweight : Int\nweight = ?weight\n"

func noteApp(t *testing.T) *Program {
	t.Helper()
	p, err := Load(scaffoldApp(t, map[string]string{"app/Table/Note.alder": noteModule}))
	if err != nil {
		t.Fatal(err)
	}
	return p
}

// TestTableNameAddress (#33): a module address that names no module but equals a table's name
// stands for the table's module, in every generator that takes one.
func TestTableNameAddress(t *testing.T) {
	p := noteApp(t)
	const path = "app/Table/Note.alder"
	t.Run("new decl", func(t *testing.T) {
		edits, err := NewDecl(p, "Note", "size", "Int")
		if err != nil {
			t.Fatal(err)
		}
		if e := editAt(edits, path); e == nil || !strings.Contains(e.Body, "size : Int\nsize = ?size") {
			t.Fatalf("edits: %+v", edits)
		}
	})
	t.Run("new test", func(t *testing.T) {
		edits, err := NewTest(p, "Note", "has a title")
		if err != nil {
			t.Fatal(err)
		}
		if e := editAt(edits, path); e == nil || !strings.Contains(e.Body, `test "has a title"`) {
			t.Fatalf("edits: %+v", edits)
		}
	})
	t.Run("fill", func(t *testing.T) {
		edits, h, _, err := Fill(p, "Note.weight", "", "3")
		if err != nil {
			t.Fatal(err)
		}
		if h.Module != "Table.Note" {
			t.Errorf("hole module %q", h.Module)
		}
		if e := editAt(edits, path); e == nil || !strings.Contains(e.Body, "weight = 3") {
			t.Fatalf("edits: %+v", edits)
		}
	})
	t.Run("fill --decl", func(t *testing.T) {
		edits, _, err := FillDecl(p, "Note.weight", "4")
		if err != nil {
			t.Fatal(err)
		}
		if e := editAt(edits, path); e == nil || !strings.Contains(e.Body, "weight = 4") {
			t.Fatalf("edits: %+v", edits)
		}
	})
	t.Run("edit decl", func(t *testing.T) {
		// the address resolves; a missing declaration is named with the full module
		_, err := EditDecl(p, "Note.nope", "add-prop", []string{"a:Int=1"})
		if err == nil || !strings.Contains(err.Error(), "`Table.Note` has no declaration `nope`") {
			t.Fatalf("err: %v", err)
		}
	})
	t.Run("neither module nor table", func(t *testing.T) {
		if _, err := NewDecl(p, "Nope", "x", "Int"); err == nil || !strings.Contains(err.Error(), "there is no module `Nope`") {
			t.Fatalf("err: %v", err)
		}
	})
}
