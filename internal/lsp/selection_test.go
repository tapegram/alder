package lsp

import (
	"strings"
	"testing"
)

func selectionRangesURI(t *testing.T, c *client, uri string, ps ...Position) []SelectionRange {
	t.Helper()
	var out []SelectionRange
	if e := c.call("textDocument/selectionRange", map[string]any{
		"textDocument": map[string]any{"uri": uri}, "positions": ps}, &out); e != nil {
		t.Fatalf("selectionRange: %v", e)
	}
	return out
}

func selectionRanges(t *testing.T, c *client, rel string, ps ...Position) []SelectionRange {
	t.Helper()
	return selectionRangesURI(t, c, c.uri(rel), ps...)
}

// rangeText is the text of r in rel's open text (utf-8: characters are bytes).
func rangeText(c *client, rel string, r Range) string {
	c.mu.Lock()
	text := c.texts[rel]
	c.mu.Unlock()
	lines := strings.Split(text, "\n")
	if r.Start.Line == r.End.Line {
		return lines[r.Start.Line][r.Start.Character:r.End.Character]
	}
	out := []string{lines[r.Start.Line][r.Start.Character:]}
	out = append(out, lines[r.Start.Line+1:r.End.Line]...)
	return strings.Join(append(out, lines[r.End.Line][:r.End.Character]), "\n")
}

// TestSelectionRange_Show: at `title` of `{model.post.title}` the chain grows through the
// access, the attribute, the element, the declaration and the module.
func TestSelectionRange_Show(t *testing.T) {
	c := start(t, blogCopy(t))
	c.open(showRel, c.disk(showRel))
	got := selectionRanges(t, c, showRel, c.at(showRel, "{model.post.title}>", 12))
	if len(got) != 1 {
		t.Fatalf("%d answers", len(got))
	}
	var texts []string
	for r := &got[0]; r != nil; r = r.Parent {
		texts = append(texts, rangeText(c, showRel, r.Range))
	}
	checks := []func(string) bool{
		func(s string) bool { return s == "model.post.title" },
		func(s string) bool { return s == "title={model.post.title}" },
		func(s string) bool {
			return strings.HasPrefix(s, "<Layout.Page") && strings.HasSuffix(s, "</Layout.Page>")
		},
		func(s string) bool { return strings.HasPrefix(s, "Show.view : Model") },
		func(s string) bool { return strings.HasPrefix(s, "module Posts.Show") },
	}
	if len(texts) != len(checks) {
		t.Fatalf("chain of %d: %q", len(texts), texts)
	}
	for i, ok := range checks {
		if !ok(texts[i]) {
			t.Errorf("level %d: %q", i, texts[i])
		}
	}
}

// TestSelectionRange_Edges: one answer per position, a zero-width range where no node holds
// the position (a comment before `module`, the end of the file) and for documents that cannot
// be answered (outside the root, unknown, not Alder, no `module` line).
func TestSelectionRange_Edges(t *testing.T) {
	c := start(t, blogCopy(t))
	const rel = "app/Edge.alder"
	c.open(rel, "-- lead\nmodule Edge\n\nx = 1\n")
	got := selectionRanges(t, c, rel, Position{0, 2}, Position{3, 4}, Position{4, 0})
	if len(got) != 3 {
		t.Fatalf("%d answers for 3 positions", len(got))
	}
	if r := got[0].Range; r.Start != (Position{0, 2}) || r.End != r.Start {
		t.Errorf("in the leading comment: %+v", got[0])
	}
	if rangeText(c, rel, got[1].Range) != "1" {
		t.Errorf("at 1: %q", rangeText(c, rel, got[1].Range))
	}
	if r := got[2].Range; r.Start != (Position{4, 0}) || r.End != r.Start {
		t.Errorf("at the end: %+v", got[2])
	}

	const bare = "app/Bare.alder"
	c.open(bare, "x = 1\ny = 2\n")
	ps := []Position{{0, 1}, {1, 2}}
	// What the lexer takes for a module: a BOM, a tab and a bare `module` line are modules.
	for name, text := range map[string]string{
		"bom":  "\uFEFFmodule BomMod\n\nx = 1\n",
		"tab":  "module\tTabMod\n\nx = 1\n",
		"bare": "module\n\nx = 1\n",
	} {
		rel := "app/" + name + ".alder"
		c.open(rel, text)
		out := selectionRanges(t, c, rel, Position{2, 4})
		if len(out) != 1 || out[0].Parent == nil {
			t.Errorf("%s: want a chain, got %+v", name, out)
		}
	}
	for name, uri := range map[string]string{
		"outside the root": "file:///nowhere/x.alder",
		"unknown":          c.uri("app/Unknown.alder"),
		"not alder":        c.uri("README.md"),
		"no module line":   c.uri(bare),
	} {
		out := selectionRangesURI(t, c, uri, ps...)
		if len(out) != len(ps) {
			t.Errorf("%s: %d answers for %d positions", name, len(out), len(ps))
			continue
		}
		for i, o := range out {
			if o.Parent != nil || o.Range.Start != ps[i] || o.Range.End != ps[i] {
				t.Errorf("%s: position %d answered %+v", name, i, o)
			}
		}
	}
}

// TestSelectionRange_Scratch: in a scratch file the chain is in file coordinates; a position
// inside a watch grows outward through the watch's expression, and one on the watch's `>` (which
// maps into the generated `itK = ` prefix) answers sane in-line ranges.
func TestSelectionRange_Scratch(t *testing.T) {
	c := start(t, blogCopy(t))
	const rel = "scratch.alder"
	c.open(rel, "sq x =\n  x * x\n\n> sq (1 + 2)\n")
	got := selectionRanges(t, c, rel, c.at(rel, "1 + 2", 4), c.at(rel, "> sq", 0), c.at(rel, "sq x", 1))
	if len(got) != 3 {
		t.Fatalf("%d answers", len(got))
	}
	var texts []string
	for r := &got[0]; r != nil; r = r.Parent {
		texts = append(texts, rangeText(c, rel, r.Range))
	}
	if len(texts) < 2 || texts[0] != "2" || !contains(texts, "1 + 2") || !contains(texts, "sq (1 + 2)") {
		t.Errorf("inside the watch: %q", texts)
	}
	prev := Range{}
	for r := &got[1]; r != nil; r = r.Parent {
		rg := r.Range
		if rg.Start.Line < 0 || rg.End.Line < rg.Start.Line || rg.End.Line > 4 ||
			rg.Start.Line == rg.End.Line && rg.End.Character < rg.Start.Character {
			t.Errorf("on `>`: bad range %+v", rg)
		}
		if rg.Start.Line == 3 && rg.Start.Character < 0 {
			t.Errorf("on `>`: %+v", rg)
		}
		if r != &got[1] && (rg.Start.Line > prev.Start.Line || rg.Start.Line == prev.Start.Line && rg.Start.Character > prev.Start.Character) {
			t.Errorf("on `>`: parent %+v does not contain child %+v", rg, prev)
		}
		prev = rg
	}
	if got[2].Range.Start.Line != 0 {
		t.Errorf("in sq's declaration: %+v", got[2])
	}
}

func contains(ss []string, s string) bool {
	for _, x := range ss {
		if x == s {
			return true
		}
	}
	return false
}
