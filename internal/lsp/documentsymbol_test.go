package lsp

import (
	"strings"
	"testing"
)

func documentSymbols(t *testing.T, c *client, uri string) []DocumentSymbol {
	t.Helper()
	var out []DocumentSymbol
	if e := c.call("textDocument/documentSymbol", map[string]any{"textDocument": map[string]any{"uri": uri}}, &out); e != nil {
		t.Fatalf("documentSymbol: %v", e)
	}
	return out
}

// symbolNamed is the symbol called name among syms, nil when there is none.
func symbolNamed(syms []DocumentSymbol, name string) *DocumentSymbol {
	for i := range syms {
		if syms[i].Name == name {
			return &syms[i]
		}
	}
	return nil
}

const outlineRel = "app/Outline.alder"

const outlineText = `module Outline

type Shape = Circle Int | Square

type Pair = { a: Int, b: Int }

double : Int -> Int
double n = n * 2

limit : Int
limit = 10

bare = 3

inc n = n + 1

job Greet = { who: Text } retries 2

Greet.run : Greet.Params -> Task Unit
Greet.run p = Log.info p.who

job Orphan = { who: Text }

route Home = GET "/" { }

Home.view : Int -> Html
Home.view m = <p>hi</p>
`

// The outline is hierarchical: functions and values with and without signatures, a type with its
// constructors, a job with its `run` arm, a route with its members.
func TestDocumentSymbol_Outline(t *testing.T) {
	c := start(t, blogCopy(t))
	c.open(outlineRel, outlineText)
	got := documentSymbols(t, c, c.uri(outlineRel))
	names := []string{}
	for _, s := range got {
		names = append(names, s.Name)
	}
	if want := "Shape Pair double limit bare inc Greet Orphan Home"; strings.Join(names, " ") != want {
		t.Fatalf("top level %q, want %q", names, want)
	}
	for _, w := range []struct {
		name, detail string
		kind         int
	}{
		{"double", "Int -> Int", 12}, {"limit", "Int", 13}, {"bare", "", 13}, {"inc", "", 12},
		{"Shape", "", 23}, {"Pair", "", 23}, {"Greet", "", 24}, {"Orphan", "", 24}, {"Home", "GET /", 6},
	} {
		s := symbolNamed(got, w.name)
		if s.Kind != w.kind || s.Detail != w.detail {
			t.Errorf("%s: kind %d detail %q, want %d %q", w.name, s.Kind, s.Detail, w.kind, w.detail)
		}
		if s.Name != rangeText(c, outlineRel, s.SelectionRange) {
			t.Errorf("%s: selection %q", w.name, rangeText(c, outlineRel, s.SelectionRange))
		}
	}
	if r := rangeText(c, outlineRel, symbolNamed(got, "double").Range); r != "double : Int -> Int\ndouble n = n * 2" {
		t.Errorf("double's range %q", r)
	}

	shape := symbolNamed(got, "Shape")
	if len(shape.Children) != 2 || shape.Children[0].Name != "Circle" || shape.Children[0].Kind != 22 || shape.Children[1].Name != "Square" {
		t.Fatalf("Shape's children %+v", shape.Children)
	}
	for _, ch := range shape.Children {
		if rangeText(c, outlineRel, ch.SelectionRange) != ch.Name {
			t.Errorf("constructor %s: selection %q", ch.Name, rangeText(c, outlineRel, ch.SelectionRange))
		}
	}
	if pair := symbolNamed(got, "Pair"); len(pair.Children) != 2 || pair.Children[0].Name != "a" || pair.Children[0].Kind != 8 {
		t.Errorf("Pair's children %+v", pair.Children)
	}

	// a job's arm in the same file is its child (kind 12), and the job's range covers it; a job
	// without an arm has no children
	greet := symbolNamed(got, "Greet")
	if len(greet.Children) != 1 || greet.Children[0].Name != "Greet.run" || greet.Children[0].Kind != 12 || greet.Children[0].Detail != "Greet.Params -> Task Unit" {
		t.Fatalf("Greet's children %+v", greet.Children)
	}
	if r := rangeText(c, outlineRel, greet.Range); !strings.HasPrefix(r, "job Greet") || !strings.HasSuffix(r, "Log.info p.who") {
		t.Errorf("Greet's range %q", r)
	}
	if o := symbolNamed(got, "Orphan"); len(o.Children) != 0 {
		t.Errorf("Orphan's children %+v", o.Children)
	}
	if symbolNamed(got, "Greet.run") != nil || symbolNamed(got, "Home.view") != nil {
		t.Error("an arm or a route member at the top level")
	}
	if home := symbolNamed(got, "Home"); len(home.Children) != 1 || home.Children[0].Name != "Home.view" {
		t.Errorf("Home's children %+v", home.Children)
	}
}

// A `run` arm whose job is in another file is no child of anything: the job's file has the job
// alone, the arm's file has the arm alone.
func TestDocumentSymbol_ArmWithoutJob(t *testing.T) {
	c := start(t, blogCopy(t))
	const jobRel = "app/JobHere.alder"
	c.open(jobRel, "module JobHere\n\njob Task1 = { x: Int }\n")
	c.open(outlineRel, "module Outline\n\nTask1.run : Task1.Params -> Task Unit\nTask1.run p = Task.succeed ()\n")
	jobs := documentSymbols(t, c, c.uri(jobRel))
	if len(jobs) != 1 || jobs[0].Name != "Task1" || jobs[0].Kind != 24 || len(jobs[0].Children) != 0 {
		t.Errorf("the job's file: %+v", jobs)
	}
	arm := documentSymbols(t, c, c.uri(outlineRel))
	if len(arm) != 1 || arm[0].Name != "Task1.run" || arm[0].Kind != 12 {
		t.Errorf("the arm's file: %+v", arm)
	}
}

// The other top-level forms of a real app module: every symbol is complete and its name lies in
// its range; the session is an object.
func TestDocumentSymbol_AppForms(t *testing.T) {
	c := start(t, blogCopy(t))
	c.open("app/App.alder", c.disk("app/App.alder"))
	got := documentSymbols(t, c, c.uri("app/App.alder"))
	if len(got) < 2 {
		t.Fatalf("%+v", got)
	}
	for _, s := range got {
		if s.Name == "" || s.Kind == 0 {
			t.Errorf("incomplete symbol %+v", s)
		}
		r, sel := s.Range, s.SelectionRange
		if sel.Start.Line < r.Start.Line || sel.End.Line > r.End.Line {
			t.Errorf("%s: selection %+v outside range %+v", s.Name, sel, r)
		}
	}
	if s := symbolNamed(got, "session"); s == nil || s.Kind != 19 {
		t.Errorf("session: %+v", s)
	}
}

// Unsaved text with a type error still has its outline; text that parses to nothing falls back
// to the last good build's outline of the file, and a file with no good build gives [].
func TestDocumentSymbol_Errors(t *testing.T) {
	c := start(t, blogCopy(t))
	const rel = "app/Posts/Index.alder"
	text := openClean(t, c, rel)
	want := documentSymbols(t, c, c.uri(rel))
	if symbolNamed(want, "lastPage") == nil {
		t.Fatalf("no lastPage in %+v", want)
	}

	c.change(rel, text+"\nbad : Int\nbad = \"x\"\n")
	c.diags(rel)
	got := documentSymbols(t, c, c.uri(rel))
	if symbolNamed(got, "bad") == nil || len(got) != len(want)+1 {
		t.Errorf("with a type error: %d symbols, want %d with bad", len(got), len(want)+1)
	}

	c.change(rel, ")))((( = = =\n")
	c.diags(rel)
	typed := got // the build of the text above, type error and all, is the last good one
	got = documentSymbols(t, c, c.uri(rel))
	if len(got) != len(typed) || symbolNamed(got, "lastPage") == nil {
		t.Errorf("unparsable text: %d symbols, want the last good build's %d", len(got), len(typed))
	}

	c.open("app/Broken.alder", ")))((( = = =\n")
	if got := documentSymbols(t, c, c.uri("app/Broken.alder")); got == nil || len(got) != 0 {
		t.Errorf("no good build of the file: %+v", got)
	}
	if got := documentSymbols(t, c, c.uri("README.md")); got == nil || len(got) != 0 {
		t.Errorf("not an Alder file: %+v", got)
	}
}

// Ranges are in the negotiated encoding: a declaration whose last line has é and an astral
// character before its end.
func TestDocumentSymbol_Encoding(t *testing.T) {
	const line = `after = "é😀"`
	text := "module Enc\n\nafter : Text\n" + line + "\n"
	for _, e := range encodings {
		t.Run(e.name, func(t *testing.T) {
			c := startOffering(t, blogCopy(t), Options{}, e.offered)
			c.open("app/Enc.alder", text)
			got := documentSymbols(t, c, c.uri("app/Enc.alder"))
			if len(got) != 1 || got[0].Name != "after" || got[0].Range.End.Line != 3 {
				t.Fatalf("%+v", got)
			}
			if want := wireCol(e.enc, line, len(line)); got[0].Range.End.Character != want {
				t.Errorf("range end %d, want %d", got[0].Range.End.Character, want)
			}
		})
	}
}

func TestDocumentSymbol_Capability(t *testing.T) {
	c := start(t, blogCopy(t))
	caps, _ := c.initResult["capabilities"].(map[string]any)
	if caps["documentSymbolProvider"] != true {
		t.Errorf("capabilities %v", caps)
	}
}
