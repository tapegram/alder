package lsp

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// searchAt sends alder/search with params and decodes the result; an RPC error fails the test.
func searchAt(t *testing.T, c *client, params map[string]any) searchResult {
	t.Helper()
	var raw json.RawMessage
	if e := c.call("alder/search", params, &raw); e != nil {
		t.Fatalf("alder/search %v: %v", params, e)
	}
	var out searchResult
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("alder/search %v: %s: %v", params, raw, err)
	}
	var shape struct {
		Results json.RawMessage `json:"results"`
	}
	json.Unmarshal(raw, &shape)
	if !strings.HasPrefix(strings.TrimSpace(string(shape.Results)), "[") {
		t.Fatalf("alder/search %v: results is not an array: %s", params, raw)
	}
	return out
}

func hasError(ds []Diagnostic) bool {
	for _, d := range ds {
		if d.Severity == 1 {
			return true
		}
	}
	return false
}

func TestSearch_Name(t *testing.T) {
	c := start(t, blogCopy(t))
	openClean(t, c, "app/Posts/Show.alder")
	r := searchAt(t, c, map[string]any{"query": "split"})
	if r.Query != "split" || r.Mode != "name" || len(r.Results) == 0 {
		t.Fatalf("result %+v", r)
	}
	if got := r.Results[0]; got.Module != "Text" || got.Name != "split" || got.Source != "stdlib" || got.URI != "" || got.Range != nil {
		t.Errorf("first result %+v", got)
	}
}

func TestSearch_AppItem(t *testing.T) {
	c := start(t, blogCopy(t))
	text := openClean(t, c, "app/App.alder")
	r := searchAt(t, c, map[string]any{"query": "notFound"})
	var got *searchItem
	for i := range r.Results {
		if r.Results[i].Module == "App" && r.Results[i].Name == "notFound" {
			got = &r.Results[i]
		}
	}
	if got == nil {
		t.Fatalf("no App.notFound in %+v", r.Results)
	}
	line := strings.Count(text[:strings.Index(text, "notFound : Html")], "\n")
	if got.Source != "app" || !strings.HasSuffix(got.URI, "/app/App.alder") || got.Range == nil || got.Range.Start.Line != line || got.Range.Start.Character != 0 {
		t.Errorf("App.notFound %+v (range %+v), want its declaration at line %d", got, got.Range, line)
	}
}

func TestSearch_AtHole(t *testing.T) {
	c := fillClient(t, fillRoot(t), &answers{list: []string{"x"}}, fullCaps)
	r := searchAt(t, c, map[string]any{"uri": c.uri(fillRel), "position": c.at(fillRel, "?out", 1)})
	if !strings.HasPrefix(r.Query, ": ") || r.Mode != "produces" || len(r.Results) == 0 {
		t.Errorf("result %+v", r)
	}
	// a query that is not empty is searched as it is
	r = searchAt(t, c, map[string]any{"query": "split", "uri": c.uri(fillRel), "position": c.at(fillRel, "?out", 1)})
	if r.Query != "split" || r.Mode != "name" {
		t.Errorf("with a query: %+v", r)
	}
}

const holesRel = "app/Holes.alder"

// holesSrc is a module of the blog with a hole of each kind of type TestSearch_HoleRoundTrip
// searches for.
const holesSrc = `module Holes

import Table.Post

type Shape = Circle Int | Square Int

type Item = { label: Text }

makeItems : Int -> List Item
makeItems n = [{ label: "a" }]

anInt : Int
anInt = ?i

aShape : Shape
aShape = ?s

someItems : List Item
someItems = ?l

entry : a -> { index: Int, item: a }
entry x = ?r

same : a -> a
same x = ?v

unknown : Int
unknown = List.length [?u]

aPost : Post
aPost = ?p
`

func TestSearch_HoleRoundTrip(t *testing.T) {
	root := blogCopy(t)
	if err := os.WriteFile(filepath.Join(root, holesRel), []byte(holesSrc), 0o644); err != nil {
		t.Fatal(err)
	}
	c := start(t, root)
	for _, d := range func() []Diagnostic { c.open(holesRel, holesSrc); return c.diags(holesRel) }() {
		if d.Severity == 1 {
			t.Fatalf("the fixture does not check: %+v", d)
		}
	}
	query := func(hole string) string {
		t.Helper()
		q, ok := c.srv.holeQuery(holesRel, c.at(holesRel, hole, 1))
		if !ok {
			t.Fatalf("%s: no hole query", hole)
		}
		return q
	}
	all := func(hole string) searchResult {
		t.Helper()
		q := query(hole)
		r := searchAt(t, c, map[string]any{"query": q, "limit": -1})
		if r.Query != q {
			t.Errorf("%s: searched %q, want %q", hole, r.Query, q)
		}
		if r.Note != "" && len(r.Results) != 0 {
			t.Errorf("%s (%s): note %q with %d results", hole, q, r.Note, len(r.Results))
		}
		return r
	}
	find := func(r searchResult, ok func(searchItem) bool) bool {
		for _, it := range r.Results {
			if ok(it) {
				return true
			}
		}
		return false
	}

	if r := all("?i"); r.Note != "" || !find(r, func(it searchItem) bool {
		return it.Module == "Text" && it.Name == "length" && it.Args != nil && *it.Args == 1
	}) {
		t.Errorf("?i (%s): note %q, no Text.length with args 1 in %d results", r.Query, r.Note, len(r.Results))
	}
	if r := all("?s"); r.Note != "" || !find(r, func(it searchItem) bool {
		return it.Module == "Holes" && (it.Name == "Circle" || it.Name == "Square") && it.Source == "app"
	}) {
		t.Errorf("?s (%s): note %q, no constructor of Shape in %+v", r.Query, r.Note, r.Results)
	}
	if r := all("?l"); r.Note != "" || !find(r, func(it searchItem) bool {
		return it.Module == "Holes" && it.Name == "makeItems" && it.Source == "app"
	}) {
		t.Errorf("?l (%s): note %q, no Holes.makeItems in %+v", r.Query, r.Note, r.Results)
	}
	all("?r")
	all("?v")
	all("?p")
	if q := query("?u"); q != ": _" {
		t.Errorf("?u: query %q, want \": _\"", q)
	}
	all("?u")

	// not on a hole
	if q, ok := c.srv.holeQuery(holesRel, c.at(holesRel, "makeItems n", 0)); ok {
		t.Errorf("off a hole: %q", q)
	}
}

func TestSearch_UnsavedBuffer(t *testing.T) {
	c := start(t, blogCopy(t))
	rel := "app/Posts/Show.alder"
	text := openClean(t, c, rel)
	c.change(rel, text+"\nzorbleCount : Int\nzorbleCount = 3\n")
	for _, d := range c.diags(rel) {
		if d.Severity == 1 {
			t.Fatalf("the change does not check: %+v", d)
		}
	}
	r := searchAt(t, c, map[string]any{"query": "zorbleCount"})
	if len(r.Results) == 0 || r.Results[0].Name != "zorbleCount" || r.Results[0].Source != "app" {
		t.Errorf("result %+v", r)
	}
}

func TestSearch_StaleBuild(t *testing.T) {
	c := start(t, blogCopy(t))
	rel := "app/Posts/Show.alder"
	text := openClean(t, c, rel)
	c.change(rel, text+"\nbroken = (\n")
	if !hasError(c.diags(rel)) {
		t.Fatal("the change parses")
	}
	r := searchAt(t, c, map[string]any{"query": "split"})
	if len(r.Results) == 0 || r.Results[0].Module != "Text" || r.Results[0].Name != "split" {
		t.Errorf("result %+v", r)
	}
}

// TestSearch_TypeError: while the program has a type error, the app's declarations come from the
// last build without one (a build with errors would index poison types).
func TestSearch_TypeError(t *testing.T) {
	c := start(t, blogCopy(t))
	rel := "app/Posts/Show.alder"
	text := openClean(t, c, rel)
	c.change(rel, text+"\nzorbleCount : Int\nzorbleCount = \"three\"\n")
	if !hasError(c.diags(rel)) {
		t.Fatal("the change has no type error")
	}
	r := searchAt(t, c, map[string]any{"query": "zorbleCount"})
	for _, it := range r.Results {
		if it.Name == "zorbleCount" {
			t.Errorf("a declaration of a build with errors: %+v", it)
		}
	}
	r = searchAt(t, c, map[string]any{"query": "notFound"})
	if len(r.Results) == 0 || r.Results[0].Module != "App" {
		t.Errorf("the last clean build's app declarations are gone: %+v", r)
	}
}

func TestSearch_NeverErrors(t *testing.T) {
	c := start(t, blogCopy(t))
	openClean(t, c, "app/Posts/Show.alder")
	for _, q := range []string{":", ": ", ": ->", ": (List", "Text ->", "-> ->", "Text -> (List", "(List", "List ("} {
		searchAt(t, c, map[string]any{"query": q})
	}
	// position without a uri, a uri outside the root, a position past the end
	searchAt(t, c, map[string]any{"position": Position{Line: 3, Character: 2}})
	searchAt(t, c, map[string]any{"uri": "file:///nowhere/X.alder", "position": Position{}})
	searchAt(t, c, map[string]any{"uri": c.uri("app/Posts/Show.alder"), "position": Position{Line: 9999, Character: 2}})
	// malformed params are the only error
	if e := c.call("alder/search", []int{1}, nil); e == nil || e.Code != codeInvalidParams {
		t.Errorf("malformed params: %v", e)
	}
}

func TestSearch_BeforeBuild(t *testing.T) {
	c := start(t, blogCopy(t))
	r := searchAt(t, c, map[string]any{"query": "split"})
	if len(r.Results) != 0 && r.Results[0].Name != "split" {
		t.Errorf("result %+v", r)
	}
	if len(r.Results) == 0 && r.Mode != "name" {
		t.Errorf("empty result in mode %q", r.Mode)
	}
}
