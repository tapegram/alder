package lsp

import (
	"strings"
	"testing"
)

type action struct {
	Title, Kind string
	Edit        *struct {
		DocumentChanges []struct {
			TextDocument struct {
				URI     string
				Version *int
			}
			Edits []TextEdit
		}
	}
	Data *struct {
		ID   string
		Sets map[string]string
		Why  string
	}
	Command *struct {
		Command   string
		Arguments []map[string]any
	}
}

func actions(t *testing.T, c *client, rel string, line int) []action {
	t.Helper()
	var out []action
	if e := c.call("textDocument/codeAction", map[string]any{
		"textDocument": map[string]any{"uri": c.uri(rel)},
		"range":        Range{Start: Position{Line: line}, End: Position{Line: line + 1}},
		"context":      map[string]any{"diagnostics": []any{}},
	}, &out); e != nil {
		t.Fatalf("codeAction: %v", e)
	}
	return out
}

func TestCodeAction_ImportFix(t *testing.T) {
	c := start(t, blogCopy(t))
	text := strings.Replace(c.disk("app/Posts/Index.alder"), "import Layout\n", "", 1)
	c.open("app/Posts/Index.alder", text)
	line := -1
	for _, d := range c.diags("app/Posts/Index.alder") {
		// importFix attaches the fix to the first diagnostic naming the module only.
		if strings.Contains(d.Message, "Layout") && d.Data != nil && d.Data.Fix != nil {
			line = d.Range.Start.Line
			break
		}
	}
	if line < 0 {
		t.Fatal("no diagnostic with a fix for the missing import")
	}
	acts := actions(t, c, "app/Posts/Index.alder", line)
	if len(acts) == 0 || acts[0].Kind != "quickfix" || !strings.HasPrefix(acts[0].Title, "Fix: ") || acts[0].Edit == nil {
		t.Fatalf("actions %+v", acts)
	}
	dc := acts[0].Edit.DocumentChanges
	if len(dc) != 1 || dc[0].TextDocument.URI != c.uri("app/Posts/Index.alder") {
		t.Fatalf("documentChanges %+v", dc)
	}
	// the version the build saw, so the client refuses the edit once the buffer is newer
	if v := dc[0].TextDocument.Version; v == nil || *v != 1 {
		t.Fatalf("version %v, want 1", v)
	}
	if edits := dc[0].Edits; len(edits) != 1 || edits[0].NewText != "import Layout\n" {
		t.Fatalf("edit %+v", edits)
	}
}

// TestCodeAction_FixStaleLine: once the fix's lines have changed since the build that found it
// (the rebuild not done yet), the fix is not offered.
func TestCodeAction_FixStaleLine(t *testing.T) {
	c := start(t, blogCopy(t))
	rel := "app/Posts/Index.alder"
	text := strings.Replace(c.disk(rel), "import Layout\n", "", 1)
	c.open(rel, text)
	var fix *TextEdit
	line := -1
	for _, d := range c.diags(rel) {
		if strings.Contains(d.Message, "Layout") && d.Data != nil && d.Data.Fix != nil {
			fix, line = d.Data.Fix, d.Range.Start.Line
			break
		}
	}
	if fix == nil {
		t.Fatal("no diagnostic with a fix for the missing import")
	}
	// the user types on the fix's end line; the server has the text, the build does not yet
	lines := strings.Split(text, "\n")
	lines[fix.Range.End.Line] += " -- typed"
	c.srv.docs.put(&doc{URI: c.uri(rel), Rel: rel, Version: 2, Text: strings.Join(lines, "\n")})
	for _, a := range actions(t, c, rel, line) {
		if a.Kind == "quickfix" {
			t.Fatalf("a fix for a changed line: %+v", a)
		}
	}
}

func TestCodeAction_HereGenerators(t *testing.T) {
	c := start(t, blogCopy(t))
	openClean(t, c, "app/Posts/Index.alder")
	acts := actions(t, c, "app/Posts/Index.alder", c.at("app/Posts/Index.alder", "load params session =", 0).Line)
	var ids []string
	for _, a := range acts {
		if a.Kind == "refactor" {
			ids = append(ids, a.Data.ID)
			if a.Command == nil || a.Command.Command != "alder.generate" || len(a.Command.Arguments) != 1 || a.Command.Arguments[0]["id"] != a.Data.ID {
				t.Fatalf("%s: no alder.generate command: %+v", a.Title, a.Command)
			}
		}
	}
	joined := strings.Join(ids, ",")
	for _, want := range []string{"new msg", "new form", "edit page", "remove page", "edit decl", "new decl", "new test"} {
		if !strings.Contains(joined, want) {
			t.Errorf("no %s in %v", want, ids)
		}
	}
	for _, a := range acts {
		if a.Data != nil && a.Data.ID == "new msg" && (a.Data.Sets["page"] != "Posts.Index" || a.Title != "new msg (page = Posts.Index)") {
			t.Fatalf("new msg action %+v", a)
		}
		if a.Data != nil && a.Data.ID == "edit decl" && a.Data.Sets["address"] != "Posts.Index.load" {
			t.Fatalf("edit decl action %+v", a)
		}
	}
}

// Spec §7: <leader>aa on a table offers editing and removing it.
func TestCodeAction_OnTable(t *testing.T) {
	c := start(t, blogCopy(t))
	openClean(t, c, "app/Post.alder")
	acts := actions(t, c, "app/Post.alder", c.at("app/Post.alder", "table Post {", 0).Line)
	found := map[string]bool{}
	for _, a := range acts {
		if a.Data == nil {
			continue
		}
		switch a.Data.ID {
		case "edit table", "remove table":
			if a.Data.Sets["table"] != "Post" || len(a.Data.Sets) != 1 || a.Title != a.Data.ID+" (table = Post)" {
				t.Fatalf("%s action %+v", a.Data.ID, a)
			}
			found[a.Data.ID] = true
		case "new msg", "edit decl":
			t.Fatalf("%s offered on a table: %+v", a.Data.ID, a)
		}
	}
	if !found["edit table"] || !found["remove table"] {
		t.Fatalf("actions %+v", acts)
	}
}

func TestCodeAction_StaleAbove(t *testing.T) {
	c := start(t, blogCopy(t))
	openClean(t, c, "app/Posts/Index.alder")
	line := c.at("app/Posts/Index.alder", "load params session =", 0).Line
	c.change("app/Posts/Index.alder", strings.Replace(c.disk("app/Posts/Index.alder"), "perPage = 10", "perPage = (", 1))
	c.diags("app/Posts/Index.alder")
	for _, a := range actions(t, c, "app/Posts/Index.alder", line) {
		if a.Kind == "refactor" {
			t.Fatalf("a generator below a changed line: %+v", a)
		}
	}
}

// TestCodeAction_StaleCursorLine: a change on the cursor's own line also hides the generators,
// as hereAt does (inclusive freshness), so no prefill comes from text that has changed.
func TestCodeAction_StaleCursorLine(t *testing.T) {
	c := start(t, blogCopy(t))
	rel := "app/Posts/Index.alder"
	openClean(t, c, rel)
	line := c.at(rel, "load params session =", 0).Line
	c.change(rel, strings.Replace(c.disk(rel), "load params session =", "load params session = (", 1))
	if ds := c.diags(rel); len(ds) == 0 || ds[0].Code != "E0001" {
		t.Fatalf("want a parse error, got %+v", ds)
	}
	for _, a := range actions(t, c, rel, line) {
		if a.Kind == "refactor" {
			t.Fatalf("a generator on a changed line: %+v", a)
		}
	}
}

func TestExplain(t *testing.T) {
	c := start(t, blogCopy(t))
	var text string
	if e := c.call("workspace/executeCommand", map[string]any{"command": "alder.explain", "arguments": []string{"W0002"}}, &text); e != nil || text == "" {
		t.Fatalf("explain: %v %q", e, text)
	}
	if e := c.call("workspace/executeCommand", map[string]any{"command": "alder.explain", "arguments": []string{"X9999"}}, &text); e == nil || !strings.Contains(e.Message, "unknown diagnostic code X9999") {
		t.Fatalf("unknown code: %v", e)
	}
}

// TestAction_E0205Fix: a full path where the qualifier is written offers the fix to it.
func TestAction_E0205Fix(t *testing.T) {
	c, rel := qualCopy(t, "Table.Todo.find")
	c.open(rel, c.disk(rel))
	line := -1
	for _, d := range c.diags(rel) {
		if d.Data != nil && d.Data.Fix != nil && d.Data.Fix.NewText == "Todo.find" {
			line = d.Range.Start.Line
		}
	}
	if line < 0 {
		t.Fatalf("no E0205 fix: %+v", c.diags(rel))
	}
	for _, a := range actions(t, c, rel, line) {
		if a.Kind == "quickfix" && a.Edit != nil && len(a.Edit.DocumentChanges[0].Edits) == 1 && a.Edit.DocumentChanges[0].Edits[0].NewText == "Todo.find" {
			return
		}
	}
	t.Fatalf("no quick fix replacing with Todo.find: %+v", actions(t, c, rel, line))
}
