package diag

import "testing"

func TestBag_ErrorAndSort(t *testing.T) {
	var b Bag
	b.Error("E0003", Span{File: "b.alder", Start: Pos{2, 1}, End: Pos{2, 4}}, "expected %s", "Int")
	b.Error("E0002", Span{File: "a.alder", Start: Pos{1, 5}, End: Pos{1, 6}}, "unknown name")
	if !b.HasErrors() {
		t.Fatal("expected errors")
	}
	b.Sort()
	if b.Items[0].Span.File != "a.alder" || b.Items[0].Code != "E0002" {
		t.Fatalf("sort order wrong: %+v", b.Items)
	}
	if b.Items[1].Message != "expected Int" {
		t.Fatalf("message not formatted: %q", b.Items[1].Message)
	}
}

func TestRenderJSON(t *testing.T) {
	d := Diagnostic{Code: "E0101", Severity: "error",
		Span:    Span{File: "app/X.alder", Start: Pos{3, 7}, End: Pos{3, 12}},
		Message: "Db.get takes an id", Fix: &Fix{Line: 3, Col: 7, Replace: "Db.first"}}
	got := RenderJSON(d)
	want := `{"code":"E0101","severity":"error","file":"app/X.alder","line":3,"col":7,"endLine":3,"endCol":12,"message":"Db.get takes an id","fix":{"line":3,"col":7,"replace":"Db.first"}}`
	if got != want {
		t.Fatalf("\n got %s\nwant %s", got, want)
	}
	d.Fix = nil
	if got := RenderJSON(d); got[len(got)-1] != '}' || contains(got, `"fix"`) {
		t.Fatalf("fix should be omitted when nil: %s", got)
	}
}

func TestRenderText(t *testing.T) {
	d := Diagnostic{Code: "E0003", Severity: "error",
		Span:    Span{File: "app/X.alder", Start: Pos{2, 5}, End: Pos{2, 8}},
		Message: "expected Int, got Text", Fix: &Fix{Line: 2, Col: 5, Replace: "Text.fromInt n"}}
	lines := []string{"module X", "x = foo + 1"}
	got := RenderText(d, lines)
	want := "app/X.alder:2:5: error[E0003]: expected Int, got Text\n" +
		"  x = foo + 1\n" +
		"      ^^^\n" +
		"  fix: Text.fromInt n\n"
	if got != want {
		t.Fatalf("\n got %q\nwant %q", got, want)
	}
}

func contains(s, sub string) bool { return len(s) >= len(sub) && (func() bool { for i := 0; i+len(sub) <= len(s); i++ { if s[i:i+len(sub)] == sub { return true } }; return false })() }

func TestExplainCoversEveryCode(t *testing.T) {
	codes := []string{"E0001", "E0002", "E0003", "E0004", "E0005", "E0006", "E0007", "E0008", "E0009", "E0010", "E0011",
		"E0101", "E0102", "E0201", "E0202", "E0203", "E0301", "E0302", "E0303", "E0304", "E0310", "E0401", "E0402",
		"E0501", "E0502", "E0601", "E0602", "W0001", "W0002"}
	for _, c := range codes {
		if len(Explain[c]) < 80 {
			t.Errorf("Explain[%s] missing or too short", c)
		}
	}
	if len(Explain) != len(codes) {
		t.Errorf("Explain has %d entries, want %d", len(Explain), len(codes))
	}
}
