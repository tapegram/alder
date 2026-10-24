package lsp

import (
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
	c.open(rel, "module Posts.Index\n\nf = {\n")
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
		{"a\n\n\nb\n", "a\n\nb\n"}, {"é\nx\n", "é\ny\n"}, {"a\r\nb\r\n", "a\nb\n"}, {"x\n", "x\n"},
	}
	for _, c := range cases {
		if got := applyEdits(t, c[0], lineEdits(c[0], c[1])); got != c[1] {
			t.Errorf("%q -> %q: got %q", c[0], c[1], got)
		}
	}
}
