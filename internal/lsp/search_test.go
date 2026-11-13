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
