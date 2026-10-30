package lsp

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"alder/internal/scratch"
)

// The editor features inside a scratch file (alder#31): hover, completion, definition and code
// actions through the file's own analysis, formatting, and inlay hints from its last run. The
// runs themselves are faked (noScratchRuns): these features never wait for a build.

// noScratchRuns fakes every scratch run as one that checked nothing.
func noScratchRuns(t *testing.T) {
	fakeScratch(t, func(int, scratch.File) scratch.FileResult { return scratch.FileResult{} })
}

const scratchDouble = "double : Int -> Int\ndouble n = n * 2\n\n> double (Todo.countOpen [])\n"

func TestScratchHover(t *testing.T) {
	noScratchRuns(t)
	c := start(t, copyExample(t, "todos"))
	c.open("scratch.alder", scratchDouble)
	if got := hover(t, c, "scratch.alder", c.at("scratch.alder", "> double", 3)); !strings.Contains(got, "double : Int -> Int") {
		t.Errorf("hover on a scratch definition in a watch: %q", got)
	}
	if got := hover(t, c, "scratch.alder", c.at("scratch.alder", "countOpen []", 1)); !strings.Contains(got, "countOpen : List Todo -> Int") {
		t.Errorf("hover on an app value in a watch: %q", got)
	}
	if got := hover(t, c, "scratch.alder", c.at("scratch.alder", "n * 2", 0)); !strings.Contains(got, "n : Int") {
		t.Errorf("hover on a parameter: %q", got)
	}
}

func TestScratchHoverInModule(t *testing.T) {
	noScratchRuns(t)
	c := start(t, copyExample(t, "todos"))
	c.open("scratch.alder", "in Table.Todo\n\n> countOpen []\n")
	if got := hover(t, c, "scratch.alder", c.at("scratch.alder", "countOpen", 2)); !strings.Contains(got, "countOpen : List Todo -> Int") {
		t.Errorf("hover: %q", got)
	}
}

func TestScratchDefinition(t *testing.T) {
	noScratchRuns(t)
	c := start(t, copyExample(t, "todos"))
	c.open("scratch.alder", scratchDouble)
	locs := definitionAt(t, c, "scratch.alder", c.at("scratch.alder", "> double", 3))
	if len(locs) != 1 || locs[0].URI != c.uri("scratch.alder") || locs[0].Range.Start.Line > 1 {
		t.Errorf("definition of a scratch value: %+v", locs)
	}
	locs = definitionAt(t, c, "scratch.alder", c.at("scratch.alder", "countOpen []", 1))
	todo := c.disk("app/Table/Todo.alder")
	if len(locs) != 1 || locs[0].URI != c.uri("app/Table/Todo.alder") || !strings.HasPrefix(strings.Split(todo, "\n")[locs[0].Range.Start.Line], "countOpen") {
		t.Errorf("definition of an app value: %+v", locs)
	}
	// a parameter, through the shift-free lines of a definition
	locs = definitionAt(t, c, "scratch.alder", c.at("scratch.alder", "n * 2", 0))
	if len(locs) != 1 || locs[0].Range.Start != (Position{1, 7}) {
		t.Errorf("definition of a parameter: %+v", locs)
	}
}

func TestScratchDefinitionInModule(t *testing.T) {
	noScratchRuns(t)
	c := start(t, copyExample(t, "todos"))
	c.open("scratch.alder", "in Table.Todo\n\nhalf : Int -> Int\nhalf n = n / 2\n\n> half (countOpen [])\n")
	locs := definitionAt(t, c, "scratch.alder", c.at("scratch.alder", "countOpen []", 1))
	todo := c.disk("app/Table/Todo.alder")
	if len(locs) != 1 || locs[0].URI != c.uri("app/Table/Todo.alder") || !strings.HasPrefix(strings.Split(todo, "\n")[locs[0].Range.Start.Line], "countOpen") {
		t.Errorf("definition in the module: %+v", locs)
	}
	locs = definitionAt(t, c, "scratch.alder", c.at("scratch.alder", "> half", 3))
	if len(locs) != 1 || locs[0].URI != c.uri("scratch.alder") || locs[0].Range.Start.Line != 3 && locs[0].Range.Start.Line != 2 {
		t.Errorf("definition in the file: %+v", locs)
	}
}

// A change to the app on disk retires the last view that checked: when the app no longer parses,
// hover says nothing rather than what the old app said.
func TestScratchViewRetiredByAppChange(t *testing.T) {
	noScratchRuns(t)
	c := start(t, copyExample(t, "todos"))
	c.open("scratch.alder", "> Todo.countOpen []\n")
	at := c.at("scratch.alder", "countOpen", 2)
	if got := hover(t, c, "scratch.alder", at); !strings.Contains(got, "countOpen : List Todo -> Int") {
		t.Fatalf("hover before: %q", got)
	}
	rel := "app/Table/Todo.alder"
	if err := os.WriteFile(filepath.Join(c.root, rel), []byte(c.disk(rel)+"\nbroken =\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := hover(t, c, "scratch.alder", at); got != "" {
		t.Fatalf("hover from a view of the old app: %q", got)
	}
}

// Completion works while the line being typed does not parse, from the file's last analysis that
// checked; its edit is in the file's columns.
func TestScratchCompletion(t *testing.T) {
	noScratchRuns(t)
	c := start(t, copyExample(t, "todos"))
	c.open("scratch.alder", "double : Int -> Int\ndouble n = n * 2\n\n> dou\n")
	end := Position{3, len("> dou")}
	items := labels(complete(t, c, "scratch.alder", end))
	it, ok := items["double"]
	if !ok {
		t.Fatalf("no `double` in %v", items)
	}
	if r := it.TextEdit.Range; r.Start != (Position{3, 2}) || r.End != end {
		t.Errorf("edit range %+v", r)
	}
	c.change("scratch.alder", "double : Int -> Int\ndouble n = n * 2\n\n> Todo.\n")
	end = Position{3, len("> Todo.")}
	items = labels(complete(t, c, "scratch.alder", end))
	it, ok = items["countOpen"]
	if !ok {
		t.Fatalf("no `countOpen` in %v", items)
	}
	if r := it.TextEdit.Range; r.Start != end || r.End != end {
		t.Errorf("edit range %+v", r)
	}
}

// A fix of a scratch diagnostic is offered with its edit in the file's columns.
func TestScratchCodeAction(t *testing.T) {
	noScratchRuns(t)
	c := start(t, copyExample(t, "todos"))
	c.open("scratch.alder", "x = 1\n\n> Table.Todo.countOpen []\n")
	var fix *action
	for _, a := range actions(t, c, "scratch.alder", 2) {
		if a.Kind == "quickfix" {
			fix = &a
			break
		}
	}
	if fix == nil || fix.Edit == nil || len(fix.Edit.DocumentChanges) != 1 {
		t.Fatalf("no fix: %+v", fix)
	}
	dc := fix.Edit.DocumentChanges[0]
	if dc.TextDocument.URI != c.uri("scratch.alder") || dc.TextDocument.Version == nil || *dc.TextDocument.Version != 1 {
		t.Fatalf("document %+v", dc.TextDocument)
	}
	if got := applyEdits(t, c.texts["scratch.alder"], dc.Edits); got != "x = 1\n\n> Todo.countOpen []\n" {
		t.Fatalf("fixed text %q", got)
	}
	if as := actions(t, c, "scratch.alder", 0); len(as) != 0 {
		t.Fatalf("actions on a clean line: %+v", as)
	}
}

func TestScratchFormatting(t *testing.T) {
	noScratchRuns(t)
	c := start(t, copyExample(t, "todos"))
	c.open("scratch.alder", "x=1\n> x   +1\n")
	if got := applyEdits(t, c.texts["scratch.alder"], formatting(t, c, "scratch.alder")); got != "x = 1\n\n> x + 1\n" {
		t.Fatalf("formatted %q", got)
	}
}

type gotHint struct {
	Position    Position
	Label       string
	Kind        int
	PaddingLeft bool
}

func inlayHints(t *testing.T, c *client, rel string) []gotHint {
	t.Helper()
	var out []gotHint
	if e := c.call("textDocument/inlayHint", map[string]any{
		"textDocument": map[string]any{"uri": c.uri(rel)},
		"range":        Range{Start: Position{0, 0}, End: Position{1000, 0}},
	}, &out); e != nil {
		t.Fatal(e)
	}
	return out
}

func TestScratchInlayHints(t *testing.T) {
	fakeScratch(t, func(k int, f scratch.File) scratch.FileResult {
		return scratch.FileResult{Ran: true, Watches: []scratch.WatchResult{
			{Line: 1, End: 1, Expr: "1 + 1", Type: "Int", Value: "2"},
			{Line: 2, End: 3, Expr: "[1,\n  2]", Type: "List Int", Value: "[1,\n 2]"},
			{Line: 4, End: 4, Expr: "Db.from Todo", Type: "Select Todo", Kind: "select", Value: "<Select>", Note: "not run"},
			{Line: 5, End: 5, Expr: "1 // 0", Type: "Int", Error: "division by zero"},
		}}
	})
	c := start(t, copyExample(t, "todos"))
	if caps, _ := c.initResult["capabilities"].(map[string]any); caps["inlayHintProvider"] != true {
		t.Fatalf("capabilities %v", c.initResult["capabilities"])
	}
	text := "> 1 + 1\n> [1,\n  2]\n> Db.from Todo\n> 1 // 0\n"
	c.open("scratch.alder", text)
	c.scratchNote() // values (the fake reports no types phase)
	hs := inlayHints(t, c, "scratch.alder")
	want := []gotHint{
		{Position{0, 7}, "⇒ 2 : Int", 0, true},
		{Position{2, 4}, "⇒ [1, … : List Int", 0, true},
		{Position{3, 14}, "⇒ <Select> : Select Todo", 0, true},
		{Position{4, 8}, "✗ division by zero", 0, true},
	}
	if len(hs) != len(want) {
		t.Fatalf("hints %+v", hs)
	}
	for i := range want {
		if hs[i] != want[i] {
			t.Errorf("hint %d: %+v, want %+v", i, hs[i], want[i])
		}
	}
	// an edited watch loses its hint until the next run; the others keep theirs
	c.change("scratch.alder", strings.Replace(text, "> 1 + 1", "> 1 + 2", 1))
	if hs := inlayHints(t, c, "scratch.alder"); len(hs) != 3 || hs[0].Position.Line != 2 {
		t.Fatalf("after an edit: %+v", hs)
	}
	// an app file has none
	if hs := inlayHints(t, c, "app/Table/Todo.alder"); len(hs) != 0 {
		t.Fatalf("app file: %+v", hs)
	}
}

// A hint's column counts bytes, as every position the server sends does (positionEncoding
// utf-8, which initialize announces): after `"é"` it is one more than the characters.
func TestScratchInlayHintColumnIsBytes(t *testing.T) {
	fakeScratch(t, func(int, scratch.File) scratch.FileResult {
		return scratch.FileResult{Ran: true, Watches: []scratch.WatchResult{{Line: 1, End: 1, Type: "Text", Value: `"é"`}}}
	})
	c := start(t, copyExample(t, "todos"))
	if caps, _ := c.initResult["capabilities"].(map[string]any); caps["positionEncoding"] != "utf-8" {
		t.Fatalf("positionEncoding %v", caps["positionEncoding"])
	}
	c.open("scratch.alder", "> \"é\"\n")
	c.scratchNote()
	if hs := inlayHints(t, c, "scratch.alder"); len(hs) != 1 || hs[0].Position != (Position{0, len("> \"é\"")}) {
		t.Fatalf("hints %+v", hs)
	}
}

// A client that offers only utf-16 gets the hint column in UTF-16 units.
func TestScratchInlayHintColumnUTF16(t *testing.T) {
	fakeScratch(t, func(int, scratch.File) scratch.FileResult {
		return scratch.FileResult{Ran: true, Watches: []scratch.WatchResult{{Line: 1, End: 1, Type: "Text", Value: `"😀"`}}}
	})
	c := startOffering(t, copyExample(t, "todos"), Options{}, []string{"utf-16"})
	c.open("scratch.alder", "> \"😀\"\n")
	c.scratchNote()
	if hs := inlayHints(t, c, "scratch.alder"); len(hs) != 1 || hs[0].Position != (Position{0, 6}) {
		t.Fatalf("hints %+v", hs)
	}
}

// A client that supports it is asked to refresh its inlay hints after each phase of a run.
func TestScratchInlayHintRefresh(t *testing.T) {
	noScratchRuns(t)
	c := startCaps(t, copyExample(t, "todos"), Options{}, map[string]any{
		"workspace": map[string]any{"inlayHint": map[string]any{"refreshSupport": true}},
	})
	c.open("scratch.alder", "> 1\n")
	c.request("workspace/inlayHint/refresh")

	plain := start(t, copyExample(t, "todos"))
	plain.open("scratch.alder", "> 1\n")
	plain.scratchNote()
	plain.sync()
	for {
		select {
		case m := <-plain.requests:
			if m.Method == "workspace/inlayHint/refresh" {
				t.Fatal("refresh sent to a client without refreshSupport")
			}
			continue
		default:
		}
		break
	}
}
