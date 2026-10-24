package lsp

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"

	"alder/internal/check"
	"alder/internal/diag"
	"alder/internal/names"
	"alder/internal/scratch"
)

// fakeScratch replaces runScratchFile for the test (restored after the server has stopped):
// each call is recorded, and answered by result (given the call's 1-based number).
func fakeScratch(t *testing.T, result func(k int, f scratch.File) scratch.FileResult) *scratchCalls {
	t.Helper()
	calls := &scratchCalls{}
	real := runScratchFile
	runScratchFile = func(_ context.Context, _ string, f scratch.File, _ scratch.FileOptions) scratch.FileResult {
		calls.mu.Lock()
		calls.texts = append(calls.texts, strings.Join(f.Lines, "\n"))
		k := len(calls.texts)
		calls.mu.Unlock()
		return result(k, f)
	}
	t.Cleanup(func() { runScratchFile = real })
	return calls
}

type scratchCalls struct {
	mu    sync.Mutex
	texts []string // each call's file text, its lines joined by "\n"
}

func (c *scratchCalls) get() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]string(nil), c.texts...)
}

func (c *client) scratchNote() scratchParams {
	c.t.Helper()
	var p scratchParams
	if err := json.Unmarshal(c.note("alder/scratch"), &p); err != nil {
		c.t.Fatal(err)
	}
	return p
}

// TestScratchOpenSendsTypesThenValues runs a real build: the types first, then the values.
func TestScratchOpenSendsTypesThenValues(t *testing.T) {
	c := start(t, copyExample(t, "todos"))
	c.open("scratch.alder", "> 1 + 1\n")
	p := c.scratchNote()
	if p.Phase != "types" || len(p.Watches) != 1 || p.Watches[0].Type != "Int" || p.Watches[0].Value != "" {
		t.Fatalf("first alder/scratch: %+v", p)
	}
	p = c.scratchNote()
	if p.Phase != "values" || len(p.Watches) != 1 || p.Watches[0].Value != "2" || !p.Ran || p.Version != 1 || p.URI != c.uri("scratch.alder") {
		t.Fatalf("second alder/scratch: %+v", p)
	}
	if ds := c.diags("scratch.alder"); len(ds) != 0 {
		t.Fatalf("diagnostics %+v", ds)
	}
}

// TestScratchSaveReruns: a change alone does not run; a save of the scratch file runs it with the
// changed text, and a save of any other file under the root reruns it.
func TestScratchSaveReruns(t *testing.T) {
	calls := fakeScratch(t, func(k int, f scratch.File) scratch.FileResult {
		return scratch.FileResult{Ran: true, Watches: []scratch.WatchResult{{Line: 1, End: 1, Expr: f.Watches[0].Expr, Value: strconv.Itoa(k)}}}
	})
	c := start(t, copyExample(t, "todos"))
	c.open("scratch.alder", "> 1\n")
	if p := c.scratchNote(); p.Phase != "values" || p.Watches[0].Value != "1" || p.Version != 1 {
		t.Fatalf("open: %+v", p)
	}
	c.change("scratch.alder", "> 2\n")
	c.save("scratch.alder")
	if p := c.scratchNote(); p.Watches[0].Value != "2" || p.Version != 2 {
		t.Fatalf("save: %+v (a change must not run)", p)
	}
	if got := calls.get(); len(got) != 2 || got[1] != "> 2\n" {
		t.Fatalf("calls %q", got)
	}
	c.save("app/Todo.alder")
	if p := c.scratchNote(); p.Watches[0].Value != "3" || p.Version != 2 {
		t.Fatalf("save of another file: %+v", p)
	}
}

// TestScratchDiagnosticsPositioned: the scratch file's publish holds its own diagnostics and a
// warning at 1:1 for the app's errors, never the app's diagnostics themselves.
func TestScratchDiagnosticsPositioned(t *testing.T) {
	fakeScratch(t, func(int, scratch.File) scratch.FileResult {
		return scratch.FileResult{Watches: []scratch.WatchResult{}, Diags: []diag.Diagnostic{
			{Code: "E0003", Severity: "error", Message: "file error",
				Span: diag.Span{File: "scratch.alder", Start: diag.Pos{Line: 3, Col: 3}, End: diag.Pos{Line: 3, Col: 5}}},
			{Code: "E0003", Severity: "error", Message: "app error",
				Span: diag.Span{File: "app/Todo.alder", Start: diag.Pos{Line: 1, Col: 1}, End: diag.Pos{Line: 1, Col: 2}}},
		}}
	})
	c := start(t, copyExample(t, "todos"))
	c.open("scratch.alder", "> 1\n\n> x\n")
	c.scratchNote()
	ds := c.diags("scratch.alder")
	if len(ds) != 2 {
		t.Fatalf("diagnostics %+v", ds)
	}
	if d := ds[0]; d.Message != "file error" || d.Severity != 1 || d.Range.Start != (Position{2, 2}) || d.Range.End != (Position{2, 4}) {
		t.Fatalf("file error %+v", d)
	}
	if d := ds[1]; d.Message != "the app has errors; fix them and save" || d.Severity != 2 || d.Range.Start != (Position{0, 0}) {
		t.Fatalf("warning %+v", d)
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, p := range c.published["app/Todo.alder"] {
		for _, d := range p.Diagnostics {
			if d.Message == "app error" {
				t.Fatalf("the scratch run published the app's diagnostic: %+v", p)
			}
		}
	}
}

func TestScratchOut(t *testing.T) {
	c := start(t, copyExample(t, "todos"))
	real := c.srv.rootReal
	want := filepath.Join(real, "build", filepath.Base(real), "scratch", scratch.FilePurpose(filepath.Join(real, "scratch.alder")))
	if got := c.srv.scratchOut("scratch.alder"); got != want {
		t.Fatalf("scratchOut %q, want %q", got, want)
	}
}

// TestScratchNotOverlaid: an open scratch document never reaches the app's analysis.
func TestScratchNotOverlaid(t *testing.T) {
	fakeScratch(t, func(int, scratch.File) scratch.FileResult { return scratch.FileResult{} })
	var mu sync.Mutex
	var seen []string
	real := loadCheckedOverlay
	loadCheckedOverlay = func(root string, over map[string][]byte) (*names.Program, *check.Result, *diag.Bag, []error) {
		mu.Lock()
		for rel := range over {
			if scratch.IsScratchFile(rel) {
				seen = append(seen, rel)
			}
		}
		mu.Unlock()
		return real(root, over)
	}
	t.Cleanup(func() { loadCheckedOverlay = real })
	c := start(t, copyExample(t, "todos"))
	c.open("scratch.alder", "> 1\n")
	c.scratchNote()
	rel := "app/Todo.alder"
	c.open(rel, c.disk(rel))
	c.diags(rel)
	c.change(rel, c.disk(rel)+"\n") // rebuilds run one at a time: the one before this has finished
	c.diags(rel)
	mu.Lock()
	defer mu.Unlock()
	if len(seen) != 0 {
		t.Fatalf("overlaid %v", seen)
	}
}
