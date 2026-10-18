package lsp

import (
	"encoding/json"
	"strings"
	"testing"
)

type preview struct {
	Fields []struct {
		Name, Value, Source, Why string
		Valid                    bool
	}
	Files []struct {
		Path, Action   string
		Added, Removed int
	}
	Diff  string
	Holes []struct{ Name string }
	Ready bool
	Error string
}

func req(c *client, rel string, p Position, extra map[string]any) map[string]any {
	m := map[string]any{"textDocument": map[string]any{"uri": c.uri(rel)}, "position": p}
	for k, v := range extra {
		m[k] = v
	}
	return m
}

func TestGenerators_HereFirst(t *testing.T) {
	c := start(t, blogCopy(t))
	openClean(t, c, "app/Posts/Index.alder")
	var out struct {
		Generators []struct {
			ID   string
			Here bool
			Why  string
		}
	}
	c.call("alder/generators", req(c, "app/Posts/Index.alder", c.at("app/Posts/Index.alder", "load params", 0), nil), &out)
	if len(out.Generators) != 19 || !out.Generators[0].Here {
		t.Fatalf("generators %+v", out.Generators)
	}
	seenRest := false
	for _, g := range out.Generators {
		if !g.Here {
			seenRest = true
		} else if seenRest {
			t.Fatalf("here generator %s after the rest", g.ID)
		}
	}
}

func TestPreview_NewMsgWalkThrough(t *testing.T) {
	c := start(t, blogCopy(t))
	openClean(t, c, "app/Posts/Index.alder")
	p := c.at("app/Posts/Index.alder", "load params", 0)
	var pv preview
	c.call("alder/preview", req(c, "app/Posts/Index.alder", p, map[string]any{"id": "new msg", "sets": map[string]string{}}), &pv)
	if pv.Ready || pv.Fields[0].Value != "Posts.Index" || pv.Fields[0].Source != "here" || pv.Fields[1].Why != "required" {
		t.Fatalf("empty form %+v", pv)
	}
	c.call("alder/preview", req(c, "app/Posts/Index.alder", p, map[string]any{"id": "new msg", "sets": map[string]string{"name": "Delete"}}), &pv)
	if !pv.Ready || pv.Error != "" || len(pv.Files) != 1 || pv.Files[0].Path != "app/Posts/Index.alder" || pv.Files[0].Added == 0 || !strings.Contains(pv.Diff, "Delete") {
		t.Fatalf("ready preview %+v", pv)
	}
}

func TestPlan_UsesUnsavedBuffer(t *testing.T) {
	c := start(t, blogCopy(t))
	text := c.disk("app/Posts/Index.alder") + "\n-- unsaved note\n"
	c.open("app/Posts/Index.alder", text)
	c.diags("app/Posts/Index.alder")
	var plan struct {
		Files []struct {
			Path          string
			Before, After *string
		}
		Error string
	}
	c.call("alder/plan", req(c, "app/Posts/Index.alder", c.at("app/Posts/Index.alder", "load params", 0), map[string]any{"id": "new msg", "sets": map[string]string{"name": "Delete"}}), &plan)
	if plan.Error != "" || len(plan.Files) != 1 || plan.Files[0].Before == nil || *plan.Files[0].Before != text || !strings.Contains(*plan.Files[0].After, "-- unsaved note") {
		t.Fatalf("plan %+v", plan)
	}
	if strings.Contains(c.disk("app/Posts/Index.alder"), "Delete") {
		t.Fatal("the plan wrote to disk")
	}
}

// refusedPage is new page's refusal for a module that exists (internal/generate/page.go).
const refusedPage = "module `Posts.Index` exists (`app/Posts/Index.alder`): a generator never overwrites a file; choose another module name"

func TestPreview_RefusalText(t *testing.T) {
	c := start(t, blogCopy(t))
	openClean(t, c, "app/Posts/Index.alder")
	var pv preview
	c.call("alder/preview", req(c, "app/Posts/Index.alder", Position{}, map[string]any{"id": "new page", "sets": map[string]string{"module": "Posts.Index", "path": "/x"}}), &pv)
	if pv.Ready || pv.Error != refusedPage || len(pv.Files) != 0 {
		t.Fatalf("refusal %+v", pv)
	}
}

// TestPlan_Refusal: a refused plan has its text, no files, and the empty lists (never null).
func TestPlan_Refusal(t *testing.T) {
	c := start(t, blogCopy(t))
	openClean(t, c, "app/Posts/Index.alder")
	var raw map[string]json.RawMessage
	c.call("alder/plan", req(c, "app/Posts/Index.alder", Position{}, map[string]any{"id": "new page", "sets": map[string]string{"module": "Posts.Index", "path": "/x"}}), &raw)
	var msg string
	json.Unmarshal(raw["error"], &msg)
	if msg != refusedPage {
		t.Fatalf("error %s", raw["error"])
	}
	for _, k := range []string{"files", "diagnostics", "holes", "imports", "migration"} {
		if string(raw[k]) != "[]" {
			t.Errorf("%s = %s, want []", k, raw[k])
		}
	}
}

// TestPlan_DiagnosticsCLIShape: a plan whose copy does not check carries the copy's errors as
// `alder … --json` writes them (file, line, col, …).
func TestPlan_DiagnosticsCLIShape(t *testing.T) {
	c := start(t, blogCopy(t))
	openClean(t, c, "app/Posts/Index.alder")
	var plan struct {
		Diagnostics []map[string]any
		Error       string
	}
	// a msg whose field names a type that does not exist: the copy does not check
	c.call("alder/plan", req(c, "app/Posts/Index.alder", Position{}, map[string]any{"id": "new msg", "sets": map[string]string{"page": "Posts.Index", "name": "Zap", "fields": "x:NoSuchType"}}), &plan)
	if plan.Error == "" || strings.Contains(plan.Error, "(the errors above)") || len(plan.Diagnostics) == 0 {
		t.Fatalf("plan %+v", plan)
	}
	d := plan.Diagnostics[0]
	if d["file"] == nil || d["line"] == nil || d["col"] == nil || d["code"] == nil || d["range"] != nil {
		t.Fatalf("diagnostic %+v is not the CLI's shape", d)
	}
}

func TestGenerators_UnknownID(t *testing.T) {
	c := start(t, blogCopy(t))
	if e := c.call("alder/preview", map[string]any{"id": "new nothing"}, nil); e == nil || e.Code != codeInvalidParams {
		t.Fatalf("error %v", e)
	}
}

func TestPreview_LatestWins(t *testing.T) {
	c := start(t, blogCopy(t))
	openClean(t, c, "app/Posts/Index.alder")
	p := c.at("app/Posts/Index.alder", "load params", 0)
	var chans []<-chan *message
	for _, name := range []string{"A", "Ab", "Abc"} {
		chans = append(chans, c.callAsync("alder/preview", req(c, "app/Posts/Index.alder", p, map[string]any{"id": "new msg", "sets": map[string]string{"name": name}})))
	}
	last := recv(t, chans[2])
	if last.Error != nil || !strings.Contains(string(last.Result), "Abc") {
		t.Fatalf("latest preview: %+v", last)
	}
	for _, ch := range chans[:2] {
		m := recv(t, ch)
		if m.Error == nil && strings.Contains(string(m.Result), "Abc") {
			t.Fatal("an older request answered with the newest diff")
		}
	}
}
