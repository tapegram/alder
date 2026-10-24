package scratch

import (
	"fmt"
	"reflect"
	"strings"
	"testing"
)

func TestIsScratchFile(t *testing.T) {
	for rel, want := range map[string]bool{
		"scratch.alder":                  true,
		"todos.scratch.alder":            true,
		"experiments/auth.scratch.alder": true,
		"app/scratch.alder":              false,
		"app/X/foo.scratch.alder":        false,
		"Scratch.alder":                  false,
		"scratch.alder.bak":              false,
	} {
		if got := IsScratchFile(rel); got != want {
			t.Errorf("IsScratchFile(%q) = %v, want %v", rel, got, want)
		}
	}
}

func TestParseFileWatches(t *testing.T) {
	text := "sample = 2\n\n> sample + 1\n> List.map\n    (\\x -> x)\n    [1]\n"
	f, diags := ParseFile("scratch.alder", text)
	if len(diags) != 0 {
		t.Fatalf("diagnostics: %v", diags)
	}
	want := []Watch{
		{Line: 3, End: 3, Name: "it1", Expr: "sample + 1"},
		{Line: 4, End: 6, Name: "it2", Expr: "List.map\n    (\\x -> x)\n    [1]"},
	}
	if !reflect.DeepEqual(f.Watches, want) {
		t.Errorf("watches = %#v, want %#v", f.Watches, want)
	}
	src := strings.Split(f.Source, "\n")
	if len(src) != len(strings.Split(text, "\n")) {
		t.Errorf("source has %d lines, input %d", len(src), len(strings.Split(text, "\n")))
	}
	if src[2] != "it1 = sample + 1" || src[3] != "it2 = List.map" || src[4] != "    (\\x -> x)" {
		t.Errorf("source = %q", src)
	}
	if f.Map.Shift[2] != -4 || f.Map.InLine[2] != 3 || len(f.Map.Lines) != len(src) {
		t.Errorf("map = %+v", f.Map)
	}
}

func TestParseFileInHeader(t *testing.T) {
	f, diags := ParseFile("scratch.alder", "-- notes\nin Todos.Index\n> countOpen []\n")
	if len(diags) != 0 {
		t.Fatalf("diagnostics: %v", diags)
	}
	if f.In != "Todos.Index" {
		t.Errorf("In = %q", f.In)
	}
	if l := strings.Split(f.Source, "\n"); l[1] != "" || l[0] != "-- notes" || l[2] != "it1 = countOpen []" {
		t.Errorf("source = %q", l)
	}
}

func TestParseFileIndentedGtIsContinuation(t *testing.T) {
	f, diags := ParseFile("scratch.alder", "x =\n    1\n  > 2\n")
	if len(diags) != 0 || len(f.Watches) != 0 {
		t.Errorf("watches %v, diagnostics %v", f.Watches, diags)
	}
}

func TestParseFileDiagnostics(t *testing.T) {
	cases := []struct{ name, text, want string }{
		{"task", "x = 1\n>! save 1\n", "scratch.alder:2:1: `>!` (run a Task) is not supported yet: scratch evaluates pure values only"},
		{"empty", "x = 1\n>\n", "scratch.alder:2:1: a watch needs an expression: `> expr`"},
		{"empty spaces", "x = 1\n>   \n", "scratch.alder:2:1: a watch needs an expression: `> expr`"},
		{"bad in", "in todos\n", "scratch.alder:1:4: `in` wants an app module name such as Todo or Todos.Index"},
		{"bare in", "in\n", "scratch.alder:1:3: `in` wants an app module name such as Todo or Todos.Index"},
		{"late in", "x = 1\nin Todo\n", "scratch.alder:2:1: `in Module` must be the file's first line"},
		{"in after watch", "> 1\nin Todo\n", "scratch.alder:2:1: `in Module` must be the file's first line"},
		{"declared", "it1 = 2\n", "scratch.alder:1:1: `it1` is reserved for the file's watches; rename it"},
		{"referenced", "> it2 + 1\n", "scratch.alder:1:3: `it2` is reserved for the file's watches; rename it"},
		{"in comment and string", "-- it3\n> \"it4\"\n", ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, diags := ParseFile("scratch.alder", c.text)
			if c.want == "" {
				if len(diags) != 0 {
					t.Fatalf("diagnostics: %v", diags)
				}
				return
			}
			if len(diags) != 1 {
				t.Fatalf("diagnostics: %v", diags)
			}
			d := diags[0]
			got := fmt.Sprintf("%s:%d:%d: %s", d.Span.File, d.Span.Start.Line, d.Span.Start.Col, d.Message)
			if got != c.want || d.Severity != "error" || d.Code != "" {
				t.Errorf("got %q (severity %q, code %q), want %q", got, d.Severity, d.Code, c.want)
			}
		})
	}
}

func TestParseFileDroppedItemContinuation(t *testing.T) {
	for _, text := range []string{"x = 1\n>!\n    save 1\n", "x = 1\n>\n    1\n", "x = 1\nin Todo\n    junk\n"} {
		f, diags := ParseFile("scratch.alder", text)
		if len(diags) != 1 {
			t.Errorf("%q: diagnostics %v", text, diags)
		}
		if l := strings.Split(f.Source, "\n"); l[0] != "x = 1" || l[1] != "" || l[2] != "" {
			t.Errorf("%q: source %q", text, l)
		}
	}
}

// Alder strings end with their line, so a `>` inside one is never at column 1.
func TestParseFileGtInStringIsNotWatch(t *testing.T) {
	f, diags := ParseFile("scratch.alder", "x = \"a\n> 2\"\ny = \"> 3\"\n")
	if len(f.Watches) != 1 || f.Watches[0].Line != 2 {
		t.Errorf("watches %v", f.Watches)
	}
	_ = diags
}
