package lsp

import (
	"strings"
	"testing"
)

type action struct {
	Title, Kind string
	Edit        *struct {
		Changes map[string][]struct {
			Range   Range
			NewText string
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
	edits := acts[0].Edit.Changes[c.uri("app/Posts/Index.alder")]
	if len(edits) != 1 || edits[0].NewText != "import Layout\n" {
		t.Fatalf("edit %+v", edits)
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
