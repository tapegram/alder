package generate

import (
	"strings"
	"testing"

	"alder/internal/diag"
)

// tagTable is a table written by hand: its last field has a comment and no comma (#28).
const tagTable = "module Table.Tag\n\ntype TagId = brand Text\n\ntable Tag {\n  id: TagId @id,\n  name: Text,\n  note: Text = \"\" -- free text\n}\n"

func tagEdit(t *testing.T, op string, args ...string) string {
	t.Helper()
	p, err := Load(scaffoldApp(t, map[string]string{"app/Table/Tag.alder": tagTable}))
	if err != nil {
		t.Fatal(err)
	}
	edits, err := EditTable(p, "Tag", op, args)
	if err != nil {
		t.Fatalf("%s: %v", op, err)
	}
	e := editAt(edits, "app/Table/Tag.alder")
	if e == nil {
		t.Fatalf("no edit: %+v", edits)
	}
	return e.Body
}

func TestEditTable_DropLastFieldWithCommentNoComma(t *testing.T) {
	got := tagEdit(t, "drop-field", "note")
	if strings.Contains(got, "note") || !strings.Contains(got, "  name: Text,\n}") {
		t.Errorf("note not dropped cleanly:\n%s", got)
	}
}

func TestEditTable_AddFieldAfterLastFieldWithoutComma(t *testing.T) {
	got := tagEdit(t, "add-field", "size:Int=0")
	if !strings.Contains(got, "  note: Text = \"\", -- free text\n  size: Int = 0,\n}") {
		t.Errorf("comma not added:\n%s", got)
	}
}

// closeRecord adds to a record whose span runs past its `}`, and says no when there is no `}`.
func TestCloseRecord_Shapes(t *testing.T) {
	sp := func(line, endCol int) diag.Span {
		return diag.Span{Start: diag.Pos{Line: line, Col: 1}, End: diag.Pos{Line: line, Col: endCol}}
	}
	tight := newSrcEdit("x = ({ a: 1 })")
	if !closeRecord(tight, sp(1, 14), "b: 2") {
		t.Fatal("tight span refused")
	}
	e := newSrcEdit("x = ({ a: 1 })")
	if !closeRecord(e, sp(1, 15), "b: 2") || e.result() != tight.result() {
		t.Errorf("span past the brace: %q, want %q", e.result(), tight.result())
	}
	e = newSrcEdit("x = f 1")
	if closeRecord(e, sp(1, 8), "b: 2") {
		t.Errorf("no brace: edited %q", e.result())
	}
}
