package lsp

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"alder/internal/format"
)

// applyEdits applies non-overlapping edits to text (positions: line, byte offset).
func applyEdits(t *testing.T, text string, edits []TextEdit) string {
	t.Helper()
	lines := strings.SplitAfter(text, "\n")
	off := func(p Position) int {
		n := 0
		for i := 0; i < p.Line && i < len(lines); i++ {
			n += len(lines[i])
		}
		return n + p.Character
	}
	for i := len(edits) - 1; i >= 0; i-- {
		e := edits[i]
		text = text[:off(e.Range.Start)] + e.NewText + text[off(e.Range.End):]
	}
	return text
}

func formatting(t *testing.T, c *client, rel string) []TextEdit {
	t.Helper()
	var edits []TextEdit
	if e := c.call("textDocument/formatting", map[string]any{"textDocument": map[string]any{"uri": c.uri(rel)}, "options": map[string]any{"tabSize": 2}}, &edits); e != nil {
		t.Fatal(e)
	}
	return edits
}

func TestFormatting_ReturnsEdits(t *testing.T) {
	c := start(t, blogCopy(t))
	rel := "app/Posts/Index.alder"
	text := strings.Replace(c.disk(rel), "\nimport ", "\n\n\nimport ", 1) + "\n\n"
	c.open(rel, text)
	want, _, err := format.Source(rel, []byte(text))
	if err != nil || want == nil {
		t.Fatal(err)
	}
	edits := formatting(t, c, rel)
	if len(edits) == 0 {
		t.Fatal("no edits")
	}
	if got := applyEdits(t, text, edits); got != string(want) {
		t.Fatalf("applied edits give\n%s\nwant\n%s", got, want)
	}
}

func TestFormatting_ParseErrorNull(t *testing.T) {
	c := start(t, blogCopy(t))
	rel := "app/Posts/Index.alder"
	c.open(rel, "module Table.Posts.Index\n\nf = {\n")
	var raw *[]TextEdit
	if e := c.call("textDocument/formatting", map[string]any{"textDocument": map[string]any{"uri": c.uri(rel)}}, &raw); e != nil {
		t.Fatal(e)
	}
	if raw != nil {
		t.Fatalf("want null, got %v", *raw)
	}
}

func TestLineEdits_Minimal(t *testing.T) {
	var old, new []string
	for i := 0; i < 50; i++ {
		old = append(old, "line")
	}
	new = append(new, old...)
	old[20], new[20] = "x:Int", "x : Int"
	o, n := strings.Join(old, "\n")+"\n", strings.Join(new, "\n")+"\n"
	edits := lineEdits(o, n)
	if len(edits) != 1 || edits[0].Range.Start != (Position{Line: 20}) || edits[0].Range.End != (Position{Line: 21}) || edits[0].NewText != "x : Int\n" {
		t.Fatalf("edits %+v", edits)
	}
}

func TestLineEdits_Roundtrip(t *testing.T) {
	cases := [][2]string{
		{"", "a\n"}, {"a\n", ""}, {"a", "a\n"}, {"a\nb", "a\nc\n"}, {"a\nb\nc\nd\n", "b\nx\nd\ne\n"},
		{"a\n\n\nb\n", "a\n\nb\n"}, {"é\nx\n", "é\ny\n"}, {"a\r\nb\r\n", "a\nb\n"}, {"x\n", "x\n"}, {"x\né", "x\né\n"}, {"é", "é\nb"},
	}
	for _, c := range cases {
		if got := applyEdits(t, c[0], lineEdits(c[0], c[1])); got != c[1] {
			t.Errorf("%q -> %q: got %q", c[0], c[1], got)
		}
	}
}

// TestLSPDoesNotFormatScratchFile: a scratch file is not ordinary Alder: no formatting and no
// code actions.
func TestLSPDoesNotFormatScratchFile(t *testing.T) {
	c := start(t, blogCopy(t))
	c.open("scratch.alder", "module Scratch\n\nx:Int\nx   =   1\n> 1 +\n")
	var raw *[]TextEdit
	if e := c.call("textDocument/formatting", map[string]any{"textDocument": map[string]any{"uri": c.uri("scratch.alder")}}, &raw); e != nil {
		t.Fatal(e)
	}
	if raw != nil {
		t.Fatalf("formatting: want null, got %v", *raw)
	}
	var acts []json.RawMessage
	rng := map[string]any{"start": map[string]any{"line": 0, "character": 0}, "end": map[string]any{"line": 0, "character": 0}}
	if e := c.call("textDocument/codeAction", map[string]any{"textDocument": map[string]any{"uri": c.uri("scratch.alder")}, "range": rng, "context": map[string]any{"diagnostics": []any{}}}, &acts); e != nil {
		t.Fatal(e)
	}
	if len(acts) != 0 {
		t.Fatalf("code actions: %s", acts)
	}
}

// migrateRoot is the blog plus two modules, Table.Todo and Admin.Todo, which need aliases
// when Posts.Index imports both; extra files are added too.
func migrateRoot(t *testing.T, extra map[string]string) string {
	t.Helper()
	root := blogCopy(t)
	files := map[string]string{
		"app/Table/Todo.alder": "module Table.Todo\n\nfind : Int -> Int\nfind n = n\n",
		"app/Admin/Todo.alder": "module Admin.Todo\n\nfind : Int -> Int\nfind n = n\n",
	}
	for k, v := range extra {
		files[k] = v
	}
	for rel, src := range files {
		if err := os.MkdirAll(filepath.Dir(filepath.Join(root, rel)), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, rel), []byte(src), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

const migrateRel = "app/Posts/Index.alder"

func migrateText(c *client) string {
	text := strings.Replace(c.disk(migrateRel), "import Layout\n", "import Layout\nimport Table.Todo\nimport Admin.Todo\n", 1)
	return text + "\n\n\nuseTodo : Int\nuseTodo = Table.Todo.find 1\n"
}

// formatting migrates this file's import qualifiers (E0204, E0205) and then formats, as
// `alder fmt` does in a project; a second request has nothing left to do. Both when the
// server's analysis is current for the buffer and when it is not yet.
func TestFormatting_Migrates(t *testing.T) {
	for _, analysed := range []bool{false, true} {
		c := start(t, migrateRoot(t, nil))
		text := migrateText(c)
		c.open(migrateRel, text)
		if analysed {
			c.diags(migrateRel)
		}
		got := applyEdits(t, text, formatting(t, c, migrateRel))
		for _, want := range []string{"import Table.Todo as TableTodo\n", "import Admin.Todo as AdminTodo\n", "useTodo = TableTodo.find 1\n"} {
			if !strings.Contains(got, want) {
				t.Fatalf("analysed=%v: missing %q in\n%s", analysed, want, got)
			}
		}
		if strings.Contains(got, "\n\n\n") {
			t.Fatalf("not formatted:\n%s", got)
		}
		c.change(migrateRel, got)
		if again := formatting(t, c, migrateRel); len(again) != 0 {
			t.Fatalf("analysed=%v: second request: %+v", analysed, again)
		}
	}
}

// A project file that does not parse turns migration off: the buffer is only formatted.
func TestFormatting_NoMigrationWhenAFileDoesNotParse(t *testing.T) {
	c := start(t, migrateRoot(t, map[string]string{"app/Broken.alder": "module Broken\n\nx = (\n"}))
	text := migrateText(c)
	c.open(migrateRel, text)
	got := applyEdits(t, text, formatting(t, c, migrateRel))
	if strings.Contains(got, "TableTodo") || strings.Contains(got, "\n\n\n") {
		t.Fatalf("migrated despite an unparsed file, or not formatted:\n%s", got)
	}
	if !strings.Contains(got, "import Admin.Todo\nimport Layout\n") || !strings.Contains(got, "import Table.Todo\n") {
		t.Fatalf("imports changed:\n%s", got)
	}
}
