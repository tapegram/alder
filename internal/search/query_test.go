package search

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"alder/internal/buildkit"
	"alder/internal/types"
)

func TestQuery_Types(t *testing.T) {
	ix := std(t)
	for _, src := range []string{
		"Text -> Int",
		"List a -> Maybe a",
		"Maybe Int",
		"Text.Builder",
		"{ index: Int, item: a }",
		"Dict Text (List a)",
		"Text -> Html", // the Prelude's Html is Html Route, as in a signature
	} {
		ty, _, note, bad := ix.queryType(src)
		if note != "" || bad {
			t.Errorf("%q: note %q bad %v", src, note, bad)
			continue
		}
		if got := types.String(ty); got != src {
			t.Errorf("%q prints %q", src, got)
		}
	}
	html, _, _, _ := ix.queryType("Html")
	if a, ok := html.(*types.App); !ok || a.Con != "Html" {
		t.Errorf("Html: %#v, want the App Html Route", html)
	}
	for _, src := range []string{"()", "Query ()"} {
		if ty, _, note, bad := ix.queryType(src); note != "" || bad || ty == nil {
			t.Errorf("%q: type %v note %q bad %v", src, ty, note, bad)
		}
	}
}

func TestQuery_Wildcards(t *testing.T) {
	ix := std(t)
	ty, _, note, _ := ix.queryType("Text -> _ -> _")
	f, ok := ty.(*types.Fun)
	if note != "" || !ok || len(f.Params) != 2 {
		t.Fatalf("Text -> _ -> _: %#v note %q", ty, note)
	}
	v1, ok1 := f.Params[1].(*types.Var)
	v2, ok2 := f.Result.(*types.Var)
	if !ok1 || !ok2 || v1.ID == v2.ID {
		t.Errorf("wildcards %#v and %#v, want two distinct variables", f.Params[1], f.Result)
	}

	ty, _, note, _ = ix.queryType("a -> a")
	f, ok = ty.(*types.Fun)
	if note != "" || !ok || len(f.Params) != 1 {
		t.Fatalf("a -> a: %#v note %q", ty, note)
	}
	for _, x := range []types.Type{f.Params[0], f.Result} {
		if p, ok := x.(*types.Param); !ok || p.Name != "a" {
			t.Errorf("a -> a: %#v, want Param a", x)
		}
	}
}

func TestQuery_Unknown(t *testing.T) {
	ix := std(t)
	for src, want := range map[string]string{
		"Foo -> Int":    "no type named `Foo`",
		"Text.Nope":     "no type named `Text.Nope`",
		"List":          "`List` takes 1 type argument",
		"Maybe Int Int": "`Maybe` takes 1 type argument",
	} {
		ty, _, note, bad := ix.queryType(src)
		if note != want || bad || ty != nil {
			t.Errorf("%q: type %v note %q bad %v, want note %q", src, ty, note, bad, want)
		}
	}
}

// appIndex indexes a temporary app with modules A and B, each declaring `Thing`, and Text2
// declaring `Shape`.
func appIndex(t *testing.T) *Index {
	t.Helper()
	dir := t.TempDir()
	files := map[string]string{
		"App.alder":   "module App\n\nerror : Error -> Html\nerror err = <h1>Something went wrong</h1>\n\nnotFound : Html\nnotFound = <h1>Not found</h1>\n",
		"A.alder":     "module A\n\ntype Thing = Thing\n",
		"B.alder":     "module B\n\ntype Thing = Thing\n",
		"Text2.alder": "module Text2\n\ntype Shape = Shape\n",
	}
	if err := os.MkdirAll(filepath.Join(dir, "app"), 0o755); err != nil {
		t.Fatal(err)
	}
	for name, src := range files {
		if err := os.WriteFile(filepath.Join(dir, "app", name), []byte(src), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	prog, res, bag, errs := buildkit.LoadChecked(dir, nil)
	if len(errs) > 0 || res == nil || bag.HasErrors() {
		t.Fatalf("load: %v %v", errs, bag)
	}
	return Build(prog, res)
}

func TestQuery_Ambiguous(t *testing.T) {
	ix := appIndex(t)
	if _, _, note, _ := ix.queryType("Thing -> Int"); note != "`Thing` is ambiguous: A.Thing, B.Thing" {
		t.Errorf("Thing -> Int: note %q", note)
	}
	if _, _, note, _ := ix.queryType("A.Thing -> Int"); note != "" {
		t.Errorf("A.Thing -> Int: note %q", note)
	}
}

func TestQuery_PreludeWins(t *testing.T) {
	ix := appIndex(t)
	ty, _, note, _ := ix.queryType("Text -> Shape")
	if note != "" {
		t.Fatalf("Text -> Shape: note %q", note)
	}
	if f, ok := ty.(*types.Fun); !ok || f.Params[0] != types.Text {
		t.Errorf("Text -> Shape: %#v, want the Prelude's Text first", ty)
	}
}

func TestQuery_HalfTyped(t *testing.T) {
	ix := std(t)
	for _, q := range []string{":", ": ", ": ->", ": (List", "Text ->", "-> ->", "Text -> (List"} {
		r := ix.Search(q, Options{})
		if !r.Bad || len(r.Results) != 0 || r.Note != "not a complete type" {
			t.Errorf("%q: bad %v note %q, %d results", q, r.Bad, r.Note, len(r.Results))
		}
	}
	for _, q := range []string{"(List", "List ("} {
		r := ix.Search(q, Options{})
		if r.Bad || r.Note != "" || len(r.Results) != 0 || r.Mode != "name" {
			t.Errorf("%q: mode %s bad %v note %q, %d results", q, r.Mode, r.Bad, r.Note, len(r.Results))
		}
	}
	long := strings.Repeat("a -> ", 400) + "a"
	start := time.Now()
	r := ix.Search(long, Options{})
	if d := time.Since(start); d > time.Second {
		t.Errorf("long query took %v", d)
	}
	if r.Bad || len(r.Results) != 0 {
		t.Errorf("long query: bad %v note %q, %d results", r.Bad, r.Note, len(r.Results))
	}
}

func TestSearch_TypeHint(t *testing.T) {
	ix := std(t)
	r := ix.Search("List Text", Options{})
	if r.Mode != "name" || len(r.Results) != 0 || r.Note != "to search by type, start with `:`" {
		t.Errorf("List Text: mode %s note %q, %d results", r.Mode, r.Note, len(r.Results))
	}
	// a lowercase word parses as a type variable, which is no reason for the hint
	for _, q := range []string{"zzzz", "qxqxq"} {
		if r = ix.Search(q, Options{}); r.Note != "" {
			t.Errorf("%s: note %q", q, r.Note)
		}
	}
	if r = ix.Search("qxqxq", Options{}); len(r.Results) != 0 {
		t.Errorf("qxqxq: %d results, want none", len(r.Results))
	}
}
