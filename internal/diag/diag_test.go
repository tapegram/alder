package diag

import (
	"strings"
	"testing"
)

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
		Message: "Db.get takes an id", Fix: InsertAt(3, 7, "Db.first")}
	got := RenderJSON(d)
	want := `{"code":"E0101","severity":"error","file":"app/X.alder","line":3,"col":7,"endLine":3,"endCol":12,"message":"Db.get takes an id","fix":{"line":3,"col":7,"endLine":3,"endCol":7,"replace":"Db.first"}}`
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
		Message: "expected Int, got Text", Fix: ReplaceSpan(Span{Start: Pos{2, 5}, End: Pos{2, 8}}, "Text.fromInt n")}
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

// A column before the line start (a bad span from a checker bug) must not panic.
func TestRenderTextClampsColumn(t *testing.T) {
	d := Diagnostic{Code: "E0011", Severity: "error", Span: Span{File: "app/X.alder", Start: Pos{1, -3}, End: Pos{1, 1}}, Message: "m"}
	if got := RenderText(d, []string{"module X"}); !contains(got, "\n  ^") {
		t.Fatalf("caret not clamped to column 1: %q", got)
	}
}

func TestRenderJSONDoesNotEscapeHTML(t *testing.T) {
	d := Diagnostic{Code: "E0304", Severity: "error", Span: Span{File: "app/X.alder", Start: Pos{1, 1}, End: Pos{1, 2}},
		Message: "`<Input>` needs `|>` & more"}
	if got := RenderJSON(d); !contains(got, "`<Input>` needs `|>` & more") {
		t.Fatalf("HTML-escaped: %s", got)
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (func() bool {
		for i := 0; i+len(sub) <= len(s); i++ {
			if s[i:i+len(sub)] == sub {
				return true
			}
		}
		return false
	})()
}

func TestExplainCoversEveryCode(t *testing.T) {
	codes := []string{"E0001", "E0002", "E0003", "E0004", "E0005", "E0006", "E0007", "E0008", "E0009", "E0010", "E0011", "E0012", "E0014", "E0015", "E0016", "E0017", "E0018", "E0019", "E0020", "E0021", "E0022", "E0023", "E0024", "E0025", "E0026", "E0027", "E0028", "E0029", "E0030", "E0031", "E0032", "E0033", "E0034", "E0035", "E0036", "E0037", "E0038", "E0039", "E0041",
		"E0101", "E0102", "E0201", "E0202", "E0203", "E0204", "E0205", "E0206", "E0301", "E0302", "E0303", "E0304", "E0310", "E0401", "E0402",
		"E0501", "E0502", "E0601", "E0602", "W0001", "W0002", "W0003", "W0004", "W0005", "W0006", "W0007", "W0008", "W0010", "W0011"}
	for _, c := range codes {
		if len(Explain[c]) < 80 {
			t.Errorf("Explain[%s] missing or too short", c)
		}
	}
	if len(Explain) != len(codes) {
		t.Errorf("Explain has %d entries, want %d", len(Explain), len(codes))
	}
}

func TestFixHelpers(t *testing.T) {
	sp := Span{File: "f", Start: Pos{3, 7}, End: Pos{3, 12}}
	f := ReplaceSpan(sp, "X")
	if f.Line != 3 || f.Col != 7 || f.EndLine != 3 || f.EndCol != 12 || f.Replace != "X" {
		t.Fatalf("ReplaceSpan: %+v", f)
	}
	i := InsertAt(5, 1, "import Todo\n")
	if i.Line != 5 || i.Col != 1 || i.EndLine != 5 || i.EndCol != 1 {
		t.Fatalf("InsertAt must be a zero-width span: %+v", i)
	}
}

func TestRenderJSON_FixHasEnd(t *testing.T) {
	d := Diagnostic{Code: "E0101", Severity: "error",
		Span: Span{File: "a.alder", Start: Pos{2, 3}, End: Pos{2, 9}}, Message: "m",
		Fix: ReplaceSpan(Span{Start: Pos{2, 3}, End: Pos{2, 9}}, "Db.first")}
	got := RenderJSON(d)
	want := `{"code":"E0101","severity":"error","file":"a.alder","line":2,"col":3,"endLine":2,"endCol":9,"message":"m","fix":{"line":2,"col":3,"endLine":2,"endCol":9,"replace":"Db.first"}}`
	if got != want {
		t.Fatalf("\n got %s\nwant %s", got, want)
	}
}

func TestApply(t *testing.T) {
	src := "module M\n\nx : Int\nx = Db.get Todo slug\n"
	// replace `Db.get Todo slug` (line 4, cols 5..21) and insert an import on line 2
	out, err := Apply(src, []Fix{
		*ReplaceSpan(Span{Start: Pos{4, 5}, End: Pos{4, 21}}, "Db.first"),
		*InsertAt(2, 1, "import Todo\n"),
	})
	if err != nil {
		t.Fatal(err)
	}
	want := "module M\nimport Todo\n\nx : Int\nx = Db.first\n"
	if out != want {
		t.Fatalf("\n got %q\nwant %q", out, want)
	}
	if _, err := Apply(src, []Fix{*ReplaceSpan(Span{Start: Pos{9, 1}, End: Pos{9, 2}}, "x")}); err == nil {
		t.Fatal("out-of-range fix must error")
	}
	if _, err := Apply(src, []Fix{
		*ReplaceSpan(Span{Start: Pos{4, 5}, End: Pos{4, 21}}, "a"),
		*ReplaceSpan(Span{Start: Pos{4, 8}, End: Pos{4, 10}}, "b"),
	}); err == nil {
		t.Fatal("overlapping fixes must error")
	}
}

func TestRenderTextNoSource(t *testing.T) {
	d := Diagnostic{Code: "E0030", Severity: "error", Span: Span{File: "f", Start: Pos{1, 3}, End: Pos{1, 9}}, Message: "m", NoSource: true}
	got := RenderText(d, []string{"  sk_live_secret"})
	if strings.Contains(got, "sk_live") || strings.Contains(got, "^") {
		t.Fatalf("source echoed: %q", got)
	}
}

func TestRenderTextNoCode(t *testing.T) {
	d := Diagnostic{Severity: "error", Span: Span{File: "s", Start: Pos{1, 1}, End: Pos{1, 1}}, Message: "m"}
	if got := RenderText(d, nil); got != "s:1:1: error: m\n" {
		t.Fatalf("got %q", got)
	}
}
