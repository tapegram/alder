package lsp

import "testing"

func foldingRanges(t *testing.T, c *client, uri string) []FoldingRange {
	t.Helper()
	var out []FoldingRange
	if e := c.call("textDocument/foldingRange", map[string]any{"textDocument": map[string]any{"uri": uri}}, &out); e != nil {
		t.Fatalf("foldingRange: %v", e)
	}
	return out
}

func TestFoldingRange_Show(t *testing.T) {
	c := start(t, blogCopy(t))
	c.open(showRel, c.disk(showRel))
	got := foldingRanges(t, c, c.uri(showRel))
	has := func(start, end int, kind string) bool {
		for _, r := range got {
			if r.StartLine == start && r.EndLine == end && r.Kind == kind {
				return true
			}
		}
		return false
	}
	for _, w := range []struct {
		start, end int
		kind, what string
	}{
		{2, 6, "imports", "the imports"},
		{9, 10, "comment", "the comment above route AddComment"},
		{13, 17, "", "type Model"},
		{19, 20, "comment", "the comment above load"},
		{21, 42, "", "load"},
		{24, 41, "", "the match in load"},
		{67, 74, "", "<article>"},
	} {
		if !has(w.start, w.end, w.kind) {
			t.Errorf("no %q range %d–%d (%s)", w.kind, w.start, w.end, w.what)
		}
	}
	for i, r := range got {
		if r.StartLine == 44 {
			t.Errorf("the one-line form declaration folds: %+v", r)
		}
		if r.EndLine <= r.StartLine {
			t.Errorf("single-line range %+v", r)
		}
		if i > 0 && got[i-1].StartLine > r.StartLine {
			t.Errorf("not in source order at %d: %+v after %+v", i, r, got[i-1])
		}
	}
}

func TestFoldingRange_Scratch(t *testing.T) {
	c := start(t, blogCopy(t))
	c.open("scratch.alder", "> 1\n  + 2\n\nsq x =\n  x * x\n")
	got := foldingRanges(t, c, c.uri("scratch.alder"))
	want := map[[2]int]bool{{0, 1}: false, {3, 4}: false}
	for _, r := range got {
		if _, ok := want[[2]int{r.StartLine, r.EndLine}]; ok {
			want[[2]int{r.StartLine, r.EndLine}] = true
		}
	}
	for k, ok := range want {
		if !ok {
			t.Errorf("no range %v in %+v", k, got)
		}
	}
}

// TestFoldingRange_Edges: unknown and non-Alder documents answer [] (Review Focus 5).
func TestFoldingRange_Edges(t *testing.T) {
	c := start(t, blogCopy(t))
	for _, uri := range []string{"file:///nowhere/x.alder", c.uri("README.md")} {
		var out []FoldingRange
		if e := c.call("textDocument/foldingRange", map[string]any{"textDocument": map[string]any{"uri": uri}}, &out); e != nil || out == nil || len(out) != 0 {
			t.Fatalf("%s: %v, %v", uri, out, e)
		}
	}
}
