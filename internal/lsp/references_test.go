package lsp

import (
	"fmt"
	"testing"
)

func refsAt(t *testing.T, c *client, rel string, p Position, decl bool) []Location {
	t.Helper()
	var locs []Location
	if e := c.call("textDocument/references", map[string]any{
		"textDocument": map[string]any{"uri": c.uri(rel)},
		"position":     p,
		"context":      map[string]any{"includeDeclaration": decl},
	}, &locs); e != nil {
		t.Fatal(e)
	}
	return locs
}

func TestReferences(t *testing.T) {
	c := start(t, blogCopy(t))
	const rel = "app/Posts/Index.alder"
	openClean(t, c, rel)
	// perPage is used four times: the limit, the offset, and twice in the page count (a
	// fifth mention is in a comment).
	locs := refsAt(t, c, rel, c.at(rel, "perPage : Int", 0), false)
	if want := 4; len(locs) != want {
		t.Fatalf("%d uses, want %d: %+v", len(locs), want, locs)
	}
	if with := refsAt(t, c, rel, c.at(rel, "perPage : Int", 0), true); len(with) != 5 || with[0].Range.Start.Line != c.at(rel, "perPage : Int", 0).Line {
		t.Fatalf("with the declaration: %+v", with)
	}
	// a type used across modules: Post
	locs = refsAt(t, c, rel, c.at(rel, "List Post", len("List ")), true)
	files := map[string]bool{}
	for _, l := range locs {
		files[l.URI] = true
	}
	if len(files) < 3 {
		t.Fatalf("Post used in %d files: %v", len(files), files)
	}
}

func TestReferences_Local(t *testing.T) {
	c := start(t, blogCopy(t))
	const rel = "app/Posts/Index.alder"
	openClean(t, c, rel)
	// `let q = …` is used in the blank check, twice in the filter and in the model; the
	// parameter field q and the form field q are other names.
	p := c.at(rel, "Text.isEmpty q", len("Text.isEmpty "))
	locs := refsAt(t, c, rel, p, false)
	if len(locs) != 4 {
		t.Fatalf("%d refs to q, want 4: %+v", len(locs), locs)
	}
	with := refsAt(t, c, rel, p, true)
	if len(with) != 5 || with[0].Range.Start.Line != c.at(rel, "let q = ", 0).Line {
		t.Fatalf("with the declaration: %+v", with)
	}
	for _, l := range with {
		if l.URI != c.uri(rel) {
			t.Fatalf("a local leaked into %s", l.URI)
		}
	}
}

// A table used as a value (RefTable) and in a type (RefType) is one declaration, as is a
// table seen from its own declaration: all three starting points give the same places.
func TestReferences_KindPairs(t *testing.T) {
	c := start(t, blogCopy(t))
	openClean(t, c, "app/Posts/Index.alder")
	openClean(t, c, "app/Write.alder")
	key := func(ls []Location) map[string]bool {
		m := map[string]bool{}
		for _, l := range ls {
			m[fmt.Sprintf("%s:%d:%d", l.URI, l.Range.Start.Line, l.Range.Start.Character)] = true
		}
		return m
	}
	byType := key(refsAt(t, c, "app/Posts/Index.alder", c.at("app/Posts/Index.alder", "List Post", len("List ")), true))
	byValue := key(refsAt(t, c, "app/Write.alder", c.at("app/Write.alder", "      Db.mine Post\n", len("      Db.mine ")), true))
	if len(byType) < 3 || len(byType) != len(byValue) {
		t.Fatalf("as type %d places, as value %d", len(byType), len(byValue))
	}
	for k := range byType {
		if !byValue[k] {
			t.Fatalf("%s missing from the value-side results", k)
		}
	}
}
