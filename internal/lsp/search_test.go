package lsp

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"unicode/utf16"

	"alder/internal/format"
	"alder/internal/syntax"
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

// TestSearch_FirstBuildErrored: when no build has been without errors (the first has a type
// error), the index is the standard library's alone: none of the app's declarations, whose types
// a build with errors may have poisoned.
func TestSearch_FirstBuildErrored(t *testing.T) {
	root := blogCopy(t)
	rel := "app/Posts/Show.alder"
	b, err := os.ReadFile(filepath.Join(root, rel))
	if err != nil {
		t.Fatal(err)
	}
	broken := string(b) + "\nzorbleCount : Int\nzorbleCount = \"three\"\n"
	if err := os.WriteFile(filepath.Join(root, rel), []byte(broken), 0o644); err != nil {
		t.Fatal(err)
	}
	c := start(t, root)
	c.open(rel, broken)
	if !hasError(c.diags(rel)) {
		t.Fatal("the fixture has no type error")
	}
	if c.srv.lastClean() != nil || c.srv.lastGood() == nil {
		t.Fatalf("lastClean %v, lastGood %v: want only an errored build", c.srv.lastClean(), c.srv.lastGood())
	}
	r := searchAt(t, c, map[string]any{"query": "split"})
	if len(r.Results) == 0 || r.Results[0].Module != "Text" || r.Results[0].Name != "split" {
		t.Errorf("split: %+v", r)
	}
	for _, q := range []string{"notFound", "zorbleCount"} {
		for _, it := range searchAt(t, c, map[string]any{"query": q, "limit": -1}).Results {
			if it.Source == "app" {
				t.Errorf("%s: an app declaration of a build with errors: %+v", q, it)
			}
		}
	}
}

func TestSearch_NeverErrors(t *testing.T) {
	c := start(t, blogCopy(t))
	openClean(t, c, "app/Posts/Show.alder")
	for _, q := range []string{":", ": ", ": ->", ": (List", "Text ->", "-> ->", "Text -> (List", "(List", "List ("} {
		searchAt(t, c, map[string]any{"query": q})
	}
	// empty, all space, and long
	for _, q := range []string{"", "   \t ", strings.Repeat("Text -> ", 250)} {
		if r := searchAt(t, c, map[string]any{"query": q}); r.Query != strings.TrimSpace(q) {
			t.Errorf("query of %d bytes answered as %q", len(q), r.Query)
		}
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

// The alder/searchEdit tests pick results in app/Pick.alder, a module of the blog written for
// each test, beside the modules of pickModules.
const pickRel = "app/Pick.alder"

const pickSrc = `module Pick

import Layout
import Util

double : Int -> Int
double n = n

fill : List Text
fill = ?h

lengthOf : Int
lengthOf = Text.length ?k

greet : Text
greet = "héllo😀" ++ ?u
`

var pickModules = map[string]string{
	"app/Util.alder": "module Util\n\nsplit : Text -> List Text\nsplit t = [t]\n\ngreeting : Text\ngreeting = \"hi\"\n",
	"app/Mid.alder":  "module Mid\n\nmid : Int -> Int\nmid n = n\n",
	"app/Zeta.alder": "module Zeta\n\nzap : Int -> Int\nzap n = n\n",
}

// pickClient is a server (offering the encodings) on the blog with pickModules and Pick as src,
// open and checked without errors.
func pickClient(t *testing.T, src string, offered ...string) *client {
	t.Helper()
	root := blogCopy(t)
	for rel, text := range pickModules {
		if err := os.WriteFile(filepath.Join(root, rel), []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(root, pickRel), []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	if len(offered) == 0 {
		offered = []string{"utf-8", "utf-16"}
	}
	c := startOffering(t, root, Options{}, offered)
	c.open(pickRel, src)
	if ds := c.diags(pickRel); hasError(ds) {
		t.Fatalf("the fixture does not check: %+v", ds)
	}
	return c
}

// searchEdit sends alder/searchEdit for the result module.name (kind, args) at p in rel.
func searchEdit(c *client, rel string, p Position, module, name, kind string, args int) (*workspaceEdit, *rpcError) {
	var out workspaceEdit
	e := c.call("alder/searchEdit", map[string]any{"uri": c.uri(rel), "position": p, "module": module, "name": name, "kind": kind, "args": args}, &out)
	if e != nil {
		return nil, e
	}
	return &out, nil
}

// applyPick is text with the edit's edits (wire units of enc) applied; the edit must be one
// change of rel.
func applyPick(t *testing.T, c *client, rel, text string, enc posEncoding, we *workspaceEdit) string {
	t.Helper()
	if len(we.DocumentChanges) != 1 || we.DocumentChanges[0].TextDocument.URI != c.uri(rel) {
		t.Fatalf("edit %+v, want one change of %s", we, rel)
	}
	lines := strings.Split(text, "\n")
	offset := func(p Position) int {
		n := 0
		for i := 0; i < p.Line; i++ {
			n += len(lines[i]) + 1
		}
		return n + enc.fromWire(lines[p.Line], p.Character)
	}
	edits := slices.Clone(we.DocumentChanges[0].Edits)
	slices.SortStableFunc(edits, func(a, b TextEdit) int { return offset(b.Range.Start) - offset(a.Range.Start) })
	for _, e := range edits {
		text = text[:offset(e.Range.Start)] + e.NewText + text[offset(e.Range.End):]
	}
	return text
}

// pick applies the edit for module.name (kind, args) at p in Pick's text src, and checks that
// the result parses with no diagnostics (TestSearchEdit_Parses).
func pick(t *testing.T, c *client, src string, p Position, module, name, kind string, args int) string {
	t.Helper()
	we, e := searchEdit(c, pickRel, p, module, name, kind, args)
	if e != nil {
		t.Fatalf("alder/searchEdit %s.%s: %v", module, name, e)
	}
	got := applyPick(t, c, pickRel, src, utf8Enc, we)
	if _, ds := syntax.ParseFile(pickRel, []byte(got)); len(ds) > 0 {
		t.Errorf("%s.%s: the edited text does not parse: %+v\n%s", module, name, ds, got)
	}
	return got
}

func wantLine(t *testing.T, got, line string) {
	t.Helper()
	if !strings.Contains(got, "\n"+line+"\n") {
		t.Errorf("no line %q in\n%s", line, got)
	}
}

func TestSearchEdit_FillHole(t *testing.T) {
	c := pickClient(t, pickSrc)
	got := pick(t, c, pickSrc, c.at(pickRel, "?h", 1), "Text", "split", "function", 2)
	wantLine(t, got, "fill = Text.split ?a ?b")
	if strings.Count(got, "import") != 2 {
		t.Errorf("an import added for the stdlib:\n%s", got)
	}
}

func TestSearchEdit_FillArgument(t *testing.T) {
	c := pickClient(t, pickSrc)
	wantLine(t, pick(t, c, pickSrc, c.at(pickRel, "?k", 0), "Text", "trim", "function", 1), "lengthOf = Text.length (Text.trim ?a)")
}

func TestSearchEdit_NoArgsNoParens(t *testing.T) {
	c := pickClient(t, pickSrc)
	wantLine(t, pick(t, c, pickSrc, c.at(pickRel, "?k", 1), "Util", "greeting", "value", 0), "lengthOf = Text.length Util.greeting")
}

func TestSearchEdit_HoleNamesSkipUsed(t *testing.T) {
	src := pickSrc + "\nused : Int\nused = ?a\n"
	c := pickClient(t, src)
	wantLine(t, pick(t, c, src, c.at(pickRel, "?h", 1), "Text", "split", "function", 2), "fill = Text.split ?b ?c")
}

func TestSearchEdit_InsertAtCursor(t *testing.T) {
	src := pickSrc + "\nsized : Int\nsized = List.length  []\n"
	c := pickClient(t, src)
	p := c.at(pickRel, "List.length  []", len("List.length "))
	we, e := searchEdit(c, pickRel, p, "Text", "split", "function", 2)
	if e != nil {
		t.Fatal(e)
	}
	if es := we.DocumentChanges[0].Edits; len(es) != 1 || es[0].NewText != "Text.split" || es[0].Range != (Range{Start: p, End: p}) {
		t.Errorf("edits %+v, want Text.split inserted at %+v", es, p)
	}
	wantLine(t, pick(t, c, src, p, "Text", "split", "function", 2), "sized = List.length Text.split []")
}

func TestSearchEdit_Prelude(t *testing.T) {
	c := pickClient(t, pickSrc)
	wantLine(t, pick(t, c, pickSrc, c.at(pickRel, "?h", 1), "Prelude", "Just", "constructor", 1), "fill = Just ?a")
}

func TestSearchEdit_OwnModule(t *testing.T) {
	c := pickClient(t, pickSrc)
	wantLine(t, pick(t, c, pickSrc, c.at(pickRel, "?h", 1), "Pick", "double", "function", 1), "fill = double ?a")
}

// formatted fails the test unless text is as the formatter writes it.
func formatted(t *testing.T, text string) {
	t.Helper()
	out, ds, err := format.Source(pickRel, []byte(text))
	if err != nil || len(ds) > 0 || string(out) != text {
		t.Errorf("not formatted (%v %+v):\n%s\nformatter:\n%s", err, ds, text, out)
	}
}

func TestSearchEdit_AddsImport(t *testing.T) {
	formatted(t, pickSrc)
	c := pickClient(t, pickSrc)
	got := pick(t, c, pickSrc, c.at(pickRel, "?h", 1), "Zeta", "zap", "function", 1)
	wantLine(t, got, "fill = Zeta.zap ?a")
	wantLine(t, got, "import Util\nimport Zeta")
	formatted(t, got)

	// a path that sorts between the imports goes between them, where the formatter puts it
	got = pick(t, c, pickSrc, c.at(pickRel, "?h", 1), "Mid", "mid", "function", 0)
	wantLine(t, got, "import Layout\nimport Mid\nimport Util")
	formatted(t, got)

	// no imports: after the module line
	src := "-- picks\nmodule Pick\n\nfill : Int\nfill = ?h\n"
	formatted(t, src)
	c = pickClient(t, src)
	got = pick(t, c, src, c.at(pickRel, "?h", 1), "Zeta", "zap", "function", 1)
	if want := "-- picks\nmodule Pick\n\nimport Zeta\n\nfill : Int\nfill = Zeta.zap ?a\n"; got != want {
		t.Errorf("got\n%s\nwant\n%s", got, want)
	}
	formatted(t, got)
}

func TestSearchEdit_ImportAlias(t *testing.T) {
	c := pickClient(t, pickSrc)
	cx := c.srv.completionAt(pickRel, c.at(pickRel, "?h", 1))
	clause := importClause(cx.a.prog, cx.m, "Admin.Layout")
	if clause != "Admin.Layout as AdminLayout" || clause != cx.importText("Admin.Layout") {
		t.Fatalf("importClause %q, importText %q", clause, cx.importText("Admin.Layout"))
	}
	got := pick(t, c, pickSrc, c.at(pickRel, "?h", 1), "Admin.Layout", "isAdmin", "function", 1)
	wantLine(t, got, "import "+clause)
	wantLine(t, got, "fill = AdminLayout.isAdmin ?a")
	formatted(t, got)

	// the app's split and the stdlib's
	wantLine(t, pick(t, c, pickSrc, c.at(pickRel, "?h", 1), "Util", "split", "function", 1), "fill = Util.split ?a")
	wantLine(t, pick(t, c, pickSrc, c.at(pickRel, "?h", 1), "Text", "split", "function", 1), "fill = Text.split ?a")
}

func TestSearchEdit_UTF16(t *testing.T) {
	c := pickClient(t, pickSrc, "utf-16")
	p := c.at(pickRel, "?u", 1) // byte column: the server reads it as UTF-16 units
	line := `greet = "héllo😀" ++ `
	ch := len(utf16.Encode([]rune(line)))
	p.Character = ch + 1
	we, e := searchEdit(c, pickRel, p, "Text", "trim", "function", 1)
	if e != nil {
		t.Fatal(e)
	}
	l := strings.Count(pickSrc[:strings.Index(pickSrc, line)], "\n")
	want := Range{Start: Position{l, ch}, End: Position{l, ch + 2}}
	if es := we.DocumentChanges[0].Edits; len(es) != 1 || es[0].Range != want {
		t.Fatalf("edits %+v, want the range %+v of ?u", es, want)
	}
	wantLine(t, applyPick(t, c, pickRel, pickSrc, utf16Enc, we), line+"Text.trim ?a")
}

func TestSearchEdit_Version(t *testing.T) {
	c := pickClient(t, pickSrc)
	c.change(pickRel, pickSrc+"\n")
	c.diags(pickRel)
	we, e := searchEdit(c, pickRel, c.at(pickRel, "?h", 1), "Text", "split", "function", 0)
	if e != nil {
		t.Fatal(e)
	}
	if v := we.DocumentChanges[0].TextDocument.Version; v == nil || *v != 2 {
		t.Errorf("version %v, want 2", v)
	}
	// Util is not open
	we, e = searchEdit(c, "app/Util.alder", Position{Line: 5, Character: 11}, "Text", "split", "function", 0)
	if e != nil {
		t.Fatal(e)
	}
	if v := we.DocumentChanges[0].TextDocument.Version; v != nil {
		t.Errorf("version %d for a closed file", *v)
	}
	if es := we.DocumentChanges[0].Edits; len(es) != 1 || es[0].NewText != "Text.split" {
		t.Errorf("edits %+v", es)
	}
}

// TestSearchEdit_Parses: every edit of the tests above parses (pick checks); here the remaining
// shapes, a hole as the function of an application among them.
func TestSearchEdit_Parses(t *testing.T) {
	src := pickSrc + "\napplied : Int\napplied = ?f 1\n"
	c := pickClient(t, src)
	wantLine(t, pick(t, c, src, c.at(pickRel, "?f", 1), "Pick", "double", "function", 1), "applied = (double ?a) 1")
	wantLine(t, pick(t, c, src, c.at(pickRel, "?f", 1), "Pick", "double", "function", 0), "applied = double 1")
	wantLine(t, pick(t, c, src, c.at(pickRel, "?u", 1), "Prelude", "Nothing", "constructor", 0), `greet = "héllo😀" ++ Nothing`)
}

// TestSearchEdit_NewHoleToken: on a `?name` the last good build does not have (the program has a
// parse error since), the pick replaces the token, bare, in wire units past non-ASCII text.
func TestSearchEdit_NewHoleToken(t *testing.T) {
	c := pickClient(t, pickSrc, "utf-16")
	line := `greet2 = "héllo😀" ++ ?fresh`
	src := pickSrc + "\ngreet2 : Text\n" + line + "\n\nbroken = (\n"
	c.change(pickRel, src)
	if !hasError(c.diags(pickRel)) {
		t.Fatal("the change parses")
	}
	l := strings.Count(src[:strings.Index(src, line)], "\n")
	ch := len(utf16.Encode([]rune(strings.TrimSuffix(line, "?fresh"))))
	want := Range{Start: Position{l, ch}, End: Position{l, ch + len("?fresh")}}
	for _, at := range []int{0, 3, len("?fresh")} {
		we, e := searchEdit(c, pickRel, Position{l, ch + at}, "Text", "trim", "function", 1)
		if e != nil {
			t.Fatal(e)
		}
		if es := we.DocumentChanges[0].Edits; len(es) != 1 || es[0].Range != want || es[0].NewText != "Text.trim" {
			t.Fatalf("at +%d: edits %+v, want Text.trim over %+v", at, es, want)
		}
		wantLine(t, applyPick(t, c, pickRel, src, utf16Enc, we), `greet2 = "héllo😀" ++ Text.trim`)
	}
}

func TestNewHoles_Capped(t *testing.T) {
	if got := newHoles(nil, 1000); len(got) != maxNewHoles || got[0] != "?a" || got[25] != "?z" {
		t.Errorf("%d holes: %v", len(got), got)
	}
}

func TestSearchEdit_UnknownItem(t *testing.T) {
	c := pickClient(t, pickSrc)
	for _, it := range [][3]string{{"Text", "nosuch", "function"}, {"Text", "split", "value"}, {"Nowhere", "split", "function"}} {
		if _, e := searchEdit(c, pickRel, c.at(pickRel, "?h", 1), it[0], it[1], it[2], 0); e == nil || e.Code != codeInvalidParams {
			t.Errorf("%v: %v, want invalid params", it, e)
		}
	}
	if e := c.call("alder/searchEdit", []int{1}, nil); e == nil || e.Code != codeInvalidParams {
		t.Errorf("malformed params: %v", e)
	}
}

// TestComplete_ImportTextUnchanged: import completion writes what importClause gives.
func TestComplete_ImportTextUnchanged(t *testing.T) {
	c := start(t, blogCopy(t))
	rel := "app/Posts/Index.alder"
	openClean(t, c, rel)
	cx := c.srv.completionAt(rel, Position{})
	for _, path := range cx.moduleNames() {
		if got, want := cx.importText(path), importClause(cx.a.prog, cx.m, path); got != want {
			t.Errorf("%s: importText %q, importClause %q", path, got, want)
		}
	}
	if got := cx.importText("Admin.Layout"); got != "Admin.Layout as AdminLayout" {
		t.Errorf("Admin.Layout: %q", got)
	}
}

func symbolsFor(t *testing.T, c *client, query string) []symbolInformation {
	t.Helper()
	var out []symbolInformation
	if e := c.call("workspace/symbol", map[string]any{"query": query}, &out); e != nil {
		t.Fatalf("workspace/symbol %q: %v", query, e)
	}
	return out
}

func TestSymbol_App(t *testing.T) {
	c := start(t, blogCopy(t))
	text := openClean(t, c, "app/App.alder")
	var got *symbolInformation
	syms := symbolsFor(t, c, "error")
	for i := range syms {
		if syms[i].Name == "error" && syms[i].ContainerName == "App" {
			got = &syms[i]
		}
	}
	if got == nil {
		t.Fatalf("no error in %+v", syms)
	}
	line := strings.Count(text[:strings.Index(text, "error : Error")], "\n")
	if got.Kind != 12 || got.ContainerName != "App" || !strings.HasSuffix(got.Location.URI, "/app/App.alder") || got.Location.Range.Start.Line != line {
		t.Errorf("symbol %+v, want a function of App at line %d", got, line)
	}
	for _, s := range symbolsFor(t, c, "split") {
		if s.Name == "split" && s.ContainerName == "Text" {
			t.Errorf("a stdlib symbol: %+v", s)
		}
	}
}

func TestSymbol_Empty(t *testing.T) {
	c := start(t, blogCopy(t))
	openClean(t, c, "app/App.alder")
	syms := symbolsFor(t, c, "")
	want := 0
	for _, e := range c.srv.searchSnapshot().searchIndex().Entries {
		if e.Source == "app" {
			want++
		}
	}
	if len(syms) < 2 || len(syms) != want {
		t.Fatalf("%d symbols, want the %d app entries: %+v", len(syms), want, syms)
	}
	for _, s := range syms {
		if !strings.Contains(s.Location.URI, "/app/") {
			t.Errorf("not under app/: %+v", s)
		}
	}
}

func TestSymbol_TypeLikeQuery(t *testing.T) {
	c := start(t, blogCopy(t))
	openClean(t, c, "app/App.alder")
	for _, q := range []string{": Int", "a -> b"} {
		if syms := symbolsFor(t, c, q); len(syms) != 0 {
			t.Errorf("%q taken as a type query: %+v", q, syms)
		}
	}
}

func TestSymbol_Capability(t *testing.T) {
	c := start(t, blogCopy(t))
	caps, _ := c.initResult["capabilities"].(map[string]any)
	if caps["workspaceSymbolProvider"] != true {
		t.Errorf("capabilities %v", caps)
	}
}

var searchCaps = map[string]any{"experimental": map[string]any{"alderSearch": true}}

func searchActOf(acts []action) *action {
	for i := range acts {
		if acts[i].Command != nil && acts[i].Command.Command == "alder.search" {
			return &acts[i]
		}
	}
	return nil
}

func TestCodeAction_SearchHole(t *testing.T) {
	c := fillClient(t, fillRoot(t), &answers{list: []string{"x"}}, searchCaps)
	acts := actionsAt(t, c, fillRel, c.at(fillRel, "?out", 1))
	a := searchActOf(acts)
	if a == nil {
		t.Fatalf("no search action in %+v", acts)
	}
	want := c.at(fillRel, "?out", 0)
	pos, _ := a.Command.Arguments[0]["position"].(map[string]any)
	if a.Title != "Search for Task Response" || a.Kind != "quickfix" || a.Command.Arguments[0]["uri"] != c.uri(fillRel) ||
		pos["line"] != float64(want.Line) || pos["character"] != float64(want.Character) {
		t.Errorf("action %+v args %+v, want position %+v", a, a.Command.Arguments, want)
	}
}

func TestCodeAction_SearchHoleGated(t *testing.T) {
	c := fillClient(t, fillRoot(t), &answers{list: []string{"x"}}, fullCaps)
	acts := actionsAt(t, c, fillRel, c.at(fillRel, "?out", 1))
	if a := searchActOf(acts); a != nil {
		t.Errorf("a search action for a default client: %+v", a)
	}
	if fillAct(t, c) == nil {
		t.Error("no Fill action")
	}
	c2 := fillClient(t, fillRoot(t), &answers{list: []string{"x"}}, searchCaps)
	for _, a := range actionsAt(t, c2, fillRel, c2.at(fillRel, "?out", 1)) {
		if a.Command != nil && a.Command.Command == "alder.fill" {
			t.Errorf("Fill offered without applyEdit: %+v", a)
		}
	}
}
