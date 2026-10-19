package lsp

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"

	"alder/internal/buildkit"
	"alder/internal/ir"
	"alder/internal/migrate"
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

// TestDiffCounts_CommentLines: a removed `-- note` (`--- note` in the diff) and an added
// `++ x` are counted, not taken for the header.
func TestDiffCounts_CommentLines(t *testing.T) {
	before, after := "a\n-- note\nb\n", "a\nb\n++ x\n"
	if add, rem := diffCounts("app/A.alder", &before, &after); add != 1 || rem != 1 {
		t.Fatalf("added %d removed %d, want 1 and 1", add, rem)
	}
	if add, rem := diffCounts("app/A.alder", nil, &after); add != 3 || rem != 0 {
		t.Fatalf("created: added %d removed %d, want 3 and 0", add, rem)
	}
}

// tree is every file under root, by path relative to it.
func tree(t *testing.T, root string) map[string]string {
	t.Helper()
	out := map[string]string{}
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		b, err := os.ReadFile(p)
		rel, _ := filepath.Rel(root, p)
		out[rel] = string(b)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

// TestPlan_MigrationOnlyInTheCopy: a Spec that writes a migration (new table), previewed and
// planned, runs the injected Migrate on the copy alone: the app's files are as they were.
func TestPlan_MigrationOnlyInTheCopy(t *testing.T) {
	root := blogCopy(t)
	var mu sync.Mutex
	var dirs []string
	// the migration writer, as cmd/alder's migrateCopyAs does it (which lsp cannot import)
	mig := func(dir, slug string) ([]string, error) {
		mu.Lock()
		dirs = append(dirs, dir)
		mu.Unlock()
		prog, res, _, ioErrs := buildkit.LoadChecked(dir, nil)
		if len(ioErrs) > 0 {
			return nil, ioErrs[0]
		}
		p, err := ir.LowerTables(prog, res)
		if err != nil {
			return nil, err
		}
		mdir := filepath.Join(dir, "migrations")
		old, err := migrate.ReadSnapshot(filepath.Join(mdir, "schema.json"))
		if err != nil {
			return nil, err
		}
		cur := migrate.FromProgram(p).WithSilos(res.Silos)
		m, err := migrate.Plan(old, cur, "next")
		if err != nil {
			return nil, err
		}
		if m.Name, err = migrate.NextName(mdir, "tag"); err != nil {
			return nil, err
		}
		if err := os.WriteFile(filepath.Join(mdir, m.Name+".sql"), []byte(migrate.Render(m)), 0o644); err != nil {
			return nil, err
		}
		var notes []string
		for _, st := range m.Steps {
			notes = append(notes, fmt.Sprintf("  %s", st))
		}
		return notes, migrate.WriteSnapshot(filepath.Join(mdir, "schema.json"), cur)
	}
	c := startWith(t, root, Options{Migrate: mig})
	openClean(t, c, "app/Posts/Index.alder")
	before := tree(t, root)
	sets := map[string]any{"id": "new table", "sets": map[string]string{"name": "Tag", "fields": "label:Text"}}
	var pv preview
	if e := c.call("alder/preview", req(c, "app/Posts/Index.alder", Position{}, sets), &pv); e != nil || !pv.Ready || pv.Error != "" {
		t.Fatalf("preview %+v %v", pv, e)
	}
	var plan struct {
		Files     []struct{ Path string }
		Migration []string
		Error     string
	}
	if e := c.call("alder/plan", req(c, "app/Posts/Index.alder", Position{}, sets), &plan); e != nil || plan.Error != "" {
		t.Fatalf("plan %+v %v", plan, e)
	}
	if !slices.ContainsFunc(plan.Files, func(f struct{ Path string }) bool { return f.Path == "migrations/0002_tag.sql" }) || len(plan.Migration) == 0 || !strings.Contains(plan.Migration[0], "Tag") {
		t.Fatalf("plan %+v", plan)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(dirs) != 2 {
		t.Fatalf("Migrate ran %d times, want 2", len(dirs))
	}
	real, _ := filepath.EvalSymlinks(root)
	for _, d := range dirs {
		if d == root || d == real || strings.HasPrefix(d, real+string(filepath.Separator)) {
			t.Fatalf("Migrate ran in the app (%s)", d)
		}
	}
	if after := tree(t, root); !maps.Equal(before, after) {
		t.Fatalf("the app changed: %d files before, %d after", len(before), len(after))
	}
}
