package lsp

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// needsCopy copies the W0014-route-helper corpus app (a route module whose helpers state no
// need, and a session with user and org keys) to a temporary root.
func needsCopy(t *testing.T) string {
	t.Helper()
	src, root := "../../testdata/corpus/W0014-route-helper/app", t.TempDir()
	err := filepath.Walk(src, func(p string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return err
		}
		rel, _ := filepath.Rel(src, p)
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		dst := filepath.Join(root, "app", rel)
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return err
		}
		return os.WriteFile(dst, b, 0o644)
	})
	if err != nil {
		t.Fatal(err)
	}
	return root
}

// A value's hover ends its signature with its `requires` clause, at the declaration and at a
// use; a W0014 helper (no clause) gets a second line with the need its body has.
func TestHover_RequiresClause(t *testing.T) {
	c := start(t, needsCopy(t))
	rel := "app/Todos.alder"
	text := c.disk(rel)
	text = strings.Replace(text, "Index.load :", "mine : Query Int requires user\nmine = Db.mine Todo |> Db.count\n\nIndex.load :", 1)
	text = strings.Replace(text, "let b <- both", "let b <- both\n  let m <- mine", 1)
	c.open(rel, text)
	c.diags(rel)
	cases := []struct {
		at   string
		off  int
		want string
		not  string
	}{
		{"mine : Query", 0, "```alder\nmine : Query Int requires user\n```", "inferred"},
		{"<- mine", len("<- "), "```alder\nmine : Query Int requires user\n```", "inferred"},
		{"recent : Select", 0, "recent : Select Todo\n```\n\nneeds user (inferred)", ""},
		{"both : Query", 0, "needs user, org (inferred)", ""},
		{"<- count", len("<- "), "count : Query Int\n```\n\nneeds user (inferred)", ""},
	}
	for _, k := range cases {
		got := hover(t, c, rel, c.at(rel, k.at, k.off))
		if !strings.Contains(got, k.want) || k.not != "" && strings.Contains(got, k.not) {
			t.Errorf("hover at %q: %q, want it to contain %q", k.at, got, k.want)
		}
	}
}

// After a signature's type `requires` is offered; after `requires ` the session's Maybe keys
// (not `role`, which has a default), on the signature's line or the next.
func TestComplete_RequiresClause(t *testing.T) {
	c := start(t, needsCopy(t))
	rel := "app/Todos.alder"
	c.open(rel, c.disk(rel))
	c.diags(rel)
	at := typing(t, c, rel, "recent = Db.mine Todo |> Db.limit 10\n", "other : Query Int ")
	if _, ok := labels(complete(t, c, rel, at))["requires"]; !ok {
		t.Errorf("no `requires` after a signature's type")
	}
	for _, typed := range []string{"other2 : Query Int requires ", "other3 : Query Int requires user, "} {
		at = typing(t, c, rel, "recent = Db.mine Todo |> Db.limit 10\n", typed)
		got := labels(complete(t, c, rel, at))
		if got["org"].Label == "" || got["role"].Label != "" || got["Text"].Label != "" {
			t.Errorf("after %q: %v", typed, labelNames(got))
		}
		if strings.HasSuffix(typed, "user, ") && got["user"].Label != "" {
			t.Errorf("after %q: `user` offered again", typed)
		}
		if strings.HasSuffix(typed, "requires ") && got["user"].Label == "" {
			t.Errorf("after %q: no `user`", typed)
		}
	}
	// the clause on the line after the signature
	at = typing(t, c, rel, "recent : Select Todo\n", "  requires ")
	if got := labels(complete(t, c, rel, at)); got["user"].Label == "" || got["org"].Label == "" || got["role"].Label != "" {
		t.Errorf("on the next line: %v", labelNames(got))
	}
}

// The W0014 helper's insert-the-clause fix is a quick fix.
func TestCodeAction_InsertRequires(t *testing.T) {
	c := start(t, needsCopy(t))
	rel := "app/Todos.alder"
	c.open(rel, c.disk(rel))
	c.diags(rel)
	line := c.at(rel, "recent : Select Todo", 0).Line
	var fix *action
	for _, a := range actionsIn(t, c, rel, 0, line+len(strings.Split(c.texts[rel], "\n"))) {
		a := a
		if a.Kind == "quickfix" && a.Edit != nil && len(a.Edit.DocumentChanges) > 0 && strings.Contains(a.Edit.DocumentChanges[0].Edits[0].NewText, "requires user") &&
			a.Edit.DocumentChanges[0].Edits[0].Range.Start.Line == line {
			fix = &a
		}
	}
	if fix == nil {
		t.Fatalf("no insert-clause quick fix for `recent`")
	}
	e := fix.Edit.DocumentChanges[0].Edits[0]
	if e.NewText != " requires user" || e.Range.Start.Character != len("recent : Select Todo") {
		t.Errorf("edit %+v", e)
	}
}

func labelNames(m map[string]item) []string {
	var out []string
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
