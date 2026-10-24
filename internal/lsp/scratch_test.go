package lsp

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"alder/internal/check"
	"alder/internal/diag"
	"alder/internal/names"
	"alder/internal/scratch"
	"alder/internal/watch"
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
	seq := c.srv.lastSeq()
	c.change("scratch.alder", "> 2\n")
	c.sync()
	if got := c.srv.lastSeq(); got != seq {
		t.Fatalf("a change ran the file (seq %d, was %d)", got, seq)
	}
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
// warning (W0007) at 1:1 for the app's errors, never the app's diagnostics themselves; the
// warning is the one place the app's errors are said, so alder/scratch carries no note for them.
func TestScratchDiagnosticsPositioned(t *testing.T) {
	fakeScratch(t, func(int, scratch.File) scratch.FileResult {
		return scratch.FileResult{Watches: []scratch.WatchResult{}, Note: scratch.AppErrorsNote, Diags: []diag.Diagnostic{
			{Code: "E0003", Severity: "error", Message: "file error",
				Span: diag.Span{File: "scratch.alder", Start: diag.Pos{Line: 3, Col: 3}, End: diag.Pos{Line: 3, Col: 5}}},
			{Code: "E0003", Severity: "error", Message: "app error",
				Span: diag.Span{File: "app/Todo.alder", Start: diag.Pos{Line: 1, Col: 1}, End: diag.Pos{Line: 1, Col: 2}}},
		}}
	})
	c := start(t, copyExample(t, "todos"))
	c.open("scratch.alder", "> 1\n\n> x\n")
	if p := c.scratchNote(); p.Note != "" {
		t.Fatalf("alder/scratch note %q: the warning already says it", p.Note)
	}
	ds := c.diags("scratch.alder")
	if len(ds) != 2 {
		t.Fatalf("diagnostics %+v", ds)
	}
	if d := ds[0]; d.Message != "file error" || d.Severity != 1 || d.Range.Start != (Position{2, 2}) || d.Range.End != (Position{2, 4}) {
		t.Fatalf("file error %+v", d)
	}
	if d := ds[1]; d.Message != "the app has errors; fix them and save" || d.Code != diag.ScratchAppErrors || d.Severity != 2 || d.Range.Start != (Position{0, 0}) {
		t.Fatalf("warning %+v", d)
	}
	c.sync()
	c.mu.Lock()
	defer c.mu.Unlock()
	for rel, pubs := range c.published {
		for _, p := range pubs {
			for _, d := range p.Diagnostics {
				if d.Message == "app error" {
					t.Fatalf("the app's diagnostic was published, for %s: %+v", rel, p)
				}
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

// blockedScratch replaces runScratchFile with a fake that waits for release (or its ctx), records
// its ctx.Err(), then reports the types and returns, both with the watches' values set to the
// call's number. It also records each run's end (scratchRunEnded).
type blockedScratch struct {
	started chan int      // each call's number, as it starts
	release chan struct{} // closed to release every call
	ended   chan string   // the rel of each run that has ended, its sends done
	mu      sync.Mutex
	errs    map[int]error // each released call's ctx.Err()
	done    chan int      // each call's number, once it has recorded its ctx.Err()
}

func blockScratch(t *testing.T) *blockedScratch {
	t.Helper()
	b := &blockedScratch{started: make(chan int, 64), release: make(chan struct{}), ended: make(chan string, 64),
		errs: map[int]error{}, done: make(chan int, 64)}
	var n atomic.Int32
	realRun, realEnded := runScratchFile, scratchRunEnded
	runScratchFile = func(ctx context.Context, _ string, f scratch.File, o scratch.FileOptions) scratch.FileResult {
		k := int(n.Add(1))
		b.started <- k
		select {
		case <-b.release:
		case <-ctx.Done():
		}
		b.mu.Lock()
		b.errs[k] = ctx.Err()
		b.mu.Unlock()
		b.done <- k
		r := scratch.FileResult{Watches: []scratch.WatchResult{}}
		for _, w := range f.Watches {
			r.Watches = append(r.Watches, scratch.WatchResult{Line: w.Line, End: w.Line, Expr: w.Expr, Value: strconv.Itoa(k)})
		}
		if o.OnTypes != nil {
			o.OnTypes(r)
		}
		r.Ran = true
		return r
	}
	scratchRunEnded = func(rel string) { b.ended <- rel }
	t.Cleanup(func() { runScratchFile, scratchRunEnded = realRun, realEnded })
	return b
}

func (b *blockedScratch) err(k int) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.errs[k]
}

// waitStarted waits for call k to start.
func (b *blockedScratch) waitStarted(t *testing.T, k int) {
	t.Helper()
	select {
	case got := <-b.started:
		if got != k {
			t.Fatalf("call %d started, want %d", got, k)
		}
	case <-time.After(timeout):
		t.Fatalf("call %d did not start", k)
	}
}

// waitDone waits for call k to have recorded its ctx.Err().
func (b *blockedScratch) waitDone(t *testing.T, k int) {
	t.Helper()
	select {
	case got := <-b.done:
		if got != k {
			t.Fatalf("call %d done, want %d", got, k)
		}
	case <-time.After(timeout):
		t.Fatalf("call %d did not finish", k)
	}
}

// waitEnded waits for n runs to end.
func (b *blockedScratch) waitEnded(t *testing.T, n int) {
	t.Helper()
	for range n {
		select {
		case <-b.ended:
		case <-time.After(timeout):
			t.Fatal("a scratch run did not end")
		}
	}
}

// scratchNotes drains the alder/scratch notifications received so far.
func (c *client) scratchNotes() []scratchParams {
	c.t.Helper()
	var ps []scratchParams
	for {
		select {
		case m := <-c.notes:
			if m.Method != "alder/scratch" {
				continue
			}
			var p scratchParams
			if err := json.Unmarshal(m.Params, &p); err != nil {
				c.t.Fatal(err)
			}
			ps = append(ps, p)
		default:
			return ps
		}
	}
}

func (c *client) closeDoc(rel string) {
	c.notify("textDocument/didClose", map[string]any{"textDocument": map[string]any{"uri": c.uri(rel)}})
}

func (c *client) publishedFor(rel string) []publishDiagnosticsParams {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]publishDiagnosticsParams(nil), c.published[rel]...)
}

// lastSeq is the number of the newest scratch run started.
func (s *server) lastSeq() uint64 {
	s.scratch.mu.Lock()
	defer s.scratch.mu.Unlock()
	return s.scratch.seq
}

// TestScratchSupersededRunPublishesNothing: a save while a run blocks cancels it, and nothing the
// cancelled run reports reaches the client.
func TestScratchSupersededRunPublishesNothing(t *testing.T) {
	b := blockScratch(t)
	c := start(t, copyExample(t, "todos"))
	c.open("scratch.alder", "> 1\n")
	b.waitStarted(t, 1)
	c.change("scratch.alder", "> 2\n")
	c.save("scratch.alder")
	b.waitDone(t, 1) // cancelled: it does not wait for the release
	if err := b.err(1); err != context.Canceled {
		t.Fatalf("the first run's ctx: %v", err)
	}
	b.waitStarted(t, 2)
	close(b.release)
	b.waitEnded(t, 2)
	c.sync()
	ps := c.scratchNotes()
	if len(ps) != 2 || ps[0].Phase != "types" || ps[1].Phase != "values" {
		t.Fatalf("alder/scratch %+v", ps)
	}
	for _, p := range ps {
		if p.Version != 2 || p.Watches[0].Value != "2" {
			t.Fatalf("alder/scratch from the superseded run: %+v", p)
		}
	}
	pubs := c.publishedFor("scratch.alder")
	if len(pubs) != 2 {
		t.Fatalf("publishes %+v", pubs)
	}
	for _, p := range pubs {
		if p.Version == nil || *p.Version != 2 {
			t.Fatalf("a publish from the superseded run: %+v", p)
		}
	}
}

// TestScratchCloseCancelsAndClears: closing the scratch document cancels its run, clears its
// diagnostics, and nothing of the run follows.
func TestScratchCloseCancelsAndClears(t *testing.T) {
	b := blockScratch(t)
	c := start(t, copyExample(t, "todos"))
	c.open("scratch.alder", "> 1\n")
	b.waitStarted(t, 1)
	c.closeDoc("scratch.alder")
	b.waitDone(t, 1)
	if err := b.err(1); err != context.Canceled {
		t.Fatalf("the run's ctx: %v", err)
	}
	b.waitEnded(t, 1)
	c.sync()
	if ps := c.scratchNotes(); len(ps) != 0 {
		t.Fatalf("alder/scratch after the close: %+v", ps)
	}
	pubs := c.publishedFor("scratch.alder")
	if len(pubs) == 0 || len(pubs[len(pubs)-1].Diagnostics) != 0 || pubs[len(pubs)-1].Diagnostics == nil {
		t.Fatalf("publishes %+v", pubs)
	}
}

// TestScratchRename (#30): an editor renames a scratch file by closing the old name and opening
// the new one, here while the old one's run is in flight: the old one's diagnostics clear,
// nothing of its run reaches the client, and only the new one runs.
func TestScratchRename(t *testing.T) {
	b := blockScratch(t)
	c := start(t, copyExample(t, "todos"))
	c.open("a.scratch.alder", "> 1\n")
	b.waitStarted(t, 1)
	c.closeDoc("a.scratch.alder")
	c.open("b.scratch.alder", "> 1\n")
	b.waitStarted(t, 2)
	b.waitDone(t, 1) // cancelled by the close
	if err := b.err(1); err != context.Canceled {
		t.Fatalf("a's run's ctx: %v", err)
	}
	close(b.release)
	b.waitEnded(t, 2)
	c.sync()
	pubs := c.publishedFor("a.scratch.alder")
	if len(pubs) != 1 || pubs[0].Diagnostics == nil || len(pubs[0].Diagnostics) != 0 {
		t.Fatalf("a's publishes %+v, want only the close's empty list", pubs)
	}
	ps := c.scratchNotes()
	if len(ps) != 2 {
		t.Fatalf("alder/scratch %+v, want b's types and values", ps)
	}
	for _, p := range ps {
		if p.URI != c.uri("b.scratch.alder") || p.Watches[0].Value != "2" {
			t.Fatalf("alder/scratch %+v, want only b's run", p)
		}
	}
	c.srv.scratch.mu.Lock()
	defer c.srv.scratch.mu.Unlock()
	if len(c.srv.scratch.byRel) != 1 || c.srv.scratch.byRel["b.scratch.alder"] == nil {
		t.Fatalf("byRel %v", c.srv.scratch.byRel)
	}
}

// TestScratchRerunsOnAppSave: a save of an app file reruns the open scratch document.
func TestScratchRerunsOnAppSave(t *testing.T) {
	b := blockScratch(t)
	close(b.release)
	c := start(t, copyExample(t, "todos"))
	c.open("scratch.alder", "> 1\n")
	b.waitEnded(t, 1)
	c.save("app/Todo.alder")
	b.waitEnded(t, 1)
	if p := c.scratchNote(); p.Watches[0].Value != "1" {
		t.Fatalf("first run %+v", p)
	}
	for {
		if p := c.scratchNote(); p.Phase == "values" && p.Watches[0].Value == "2" {
			break
		}
	}
}

// TestScratchRerunsOnPoll: a change to an app file on disk reruns the open scratch document. The
// file is rewritten (growing) until a run ends, since a write before the poller's first look is
// not a change to it.
func TestScratchRerunsOnPoll(t *testing.T) {
	b := blockScratch(t)
	close(b.release)
	root := copyExample(t, "todos")
	c := startWith(t, root, Options{Poll: 20 * time.Millisecond})
	c.open("scratch.alder", "> 1\n")
	b.waitEnded(t, 1)
	file := filepath.Join(root, "app", "Todo.alder")
	text := c.disk("app/Todo.alder")
	deadline := time.After(timeout)
	for rerun := false; !rerun; {
		text += "\n"
		if err := os.WriteFile(file, []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
		select {
		case <-b.ended: // nothing else runs it: no save, no change to it
			rerun = true
		case <-time.After(100 * time.Millisecond):
		case <-deadline:
			t.Fatalf("no rerun in %v of rewriting app/Todo.alder", timeout)
		}
	}
	for {
		if p := c.scratchNote(); p.Phase == "values" && p.Watches[0].Value == "2" {
			break
		}
	}
}

// TestScratchShutdownWaits: the server stops with a run blocked: the run is cancelled and Serve
// returns.
func TestScratchShutdownWaits(t *testing.T) {
	b := blockScratch(t)
	c := start(t, copyExample(t, "todos"))
	c.open("scratch.alder", "> 1\n")
	b.waitStarted(t, 1)
	if e := c.call("shutdown", nil, nil); e != nil {
		t.Fatal(e)
	}
	c.notify("exit", nil)
	if err := c.waitDone(); err != nil {
		t.Fatalf("Serve returned %v", err)
	}
	if err := b.err(1); err != context.Canceled {
		t.Fatalf("the run's ctx: %v", err)
	}
}

// TestScratchAppSaveRunsOnce: an editor save of an app file writes it and sends didSave; the
// didSave reruns the open scratch document and the poll that then sees the write does not. The
// poller is a fake the test ticks (watchRoot), so the poll comes after the save, as it does
// when the save is handled first; scratchSourcesChanged still compares the files on disk.
func TestScratchAppSaveRunsOnce(t *testing.T) {
	b := blockScratch(t)
	close(b.release)
	polled := make(chan bool, 16)
	tick := make(chan watch.Change)
	realPolled, realWatch := scratchPolled, watchRoot
	scratchPolled = func(rerun bool) { polled <- rerun }
	watchRoot = func(_ string, _ time.Duration, stop <-chan struct{}, ch chan<- watch.Change) {
		for {
			select {
			case <-stop:
				return
			case k := <-tick:
				ch <- k
			}
		}
	}
	t.Cleanup(func() { scratchPolled, watchRoot = realPolled, realWatch })
	root := copyExample(t, "todos")
	c := startWith(t, root, Options{Poll: time.Hour}) // Poll > 0 starts the (fake) poller
	c.open("scratch.alder", "> 1\n")
	b.waitEnded(t, 1)
	if err := os.WriteFile(filepath.Join(root, "app", "Todo.alder"), []byte(c.disk("app/Todo.alder")+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	c.save("app/Todo.alder")
	b.waitEnded(t, 1) // the save's rerun: its stamps are recorded before it starts
	select {
	case tick <- watch.SourceChange: // the poll that sees the save's write
	case <-time.After(timeout):
		t.Fatal("the poller did not start")
	}
	select {
	case rerun := <-polled:
		if rerun {
			t.Fatal("the poll reran the scratch document after the save had")
		}
	case <-time.After(timeout):
		t.Fatal("the poll was not handled")
	}
	if n := c.srv.lastSeq(); n != 2 {
		t.Fatalf("%d runs, want 2 (the open and the save)", n)
	}
	// A write no save reported: the poll reruns it.
	if err := os.WriteFile(filepath.Join(root, "app", "Todo.alder"), []byte(c.disk("app/Todo.alder")+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	select {
	case tick <- watch.SourceChange:
	case <-time.After(timeout):
		t.Fatal("the poller did not take the second tick")
	}
	select {
	case rerun := <-polled:
		if !rerun {
			t.Fatal("the poll of an unsaved write did not rerun the scratch document")
		}
	case <-time.After(timeout):
		t.Fatal("the poll was not handled")
	}
	b.waitEnded(t, 1)
}

// TestScratchRunNotOpen: a run asked for a scratch document that is no longer open (a close
// racing a poll rerun) does not start, and leaves nothing behind.
func TestScratchRunNotOpen(t *testing.T) {
	b := blockScratch(t)
	close(b.release)
	c := start(t, copyExample(t, "todos"))
	c.sync()
	c.srv.runScratch("scratch.alder")
	c.srv.scratch.mu.Lock()
	defer c.srv.scratch.mu.Unlock()
	if len(c.srv.scratch.byRel) != 0 || c.srv.scratch.seq != 0 {
		t.Fatalf("byRel %v, seq %d", c.srv.scratch.byRel, c.srv.scratch.seq)
	}
}

// TestScratchCloseClearsItsURI: the close clears the diagnostics at the URI the document was
// opened by, though the root was given by another spelling (a symbolic link).
func TestScratchCloseClearsItsURI(t *testing.T) {
	b := blockScratch(t)
	close(b.release)
	real := copyExample(t, "todos")
	link := filepath.Join(t.TempDir(), "via-link")
	if err := os.Symlink(real, link); err != nil {
		t.Skip(err)
	}
	c := start(t, link)
	uri := "file://" + filepath.ToSlash(filepath.Join(real, "scratch.alder"))
	c.openURI(uri, "scratch.alder", "> 1\n")
	b.waitEnded(t, 1)
	c.notify("textDocument/didClose", map[string]any{"textDocument": map[string]any{"uri": uri}})
	c.sync()
	pubs := c.publishedFor("scratch.alder")
	if len(pubs) == 0 {
		t.Fatal("no publishes")
	}
	if last := pubs[len(pubs)-1]; last.URI != uri || last.Diagnostics == nil || len(last.Diagnostics) != 0 {
		t.Fatalf("the close published %+v, want an empty list at %s", last, uri)
	}
}

// TestScratchAppSaveWalksOnlyWhenNeeded: a save of an app file stamps the watched files (a walk
// of the tree) only when a scratch document is open and a poller would see the write.
func TestScratchAppSaveWalksOnlyWhenNeeded(t *testing.T) {
	b := blockScratch(t)
	close(b.release)
	var walks atomic.Int32
	realSnap := snapshotRoot
	snapshotRoot = func(dir string) map[string]watch.Stamp { walks.Add(1); return realSnap(dir) }
	t.Cleanup(func() { snapshotRoot = realSnap })

	c := start(t, copyExample(t, "todos")) // Poll 0: no poller
	c.open("scratch.alder", "> 1\n")
	b.waitEnded(t, 1)
	c.save("app/Todo.alder")
	b.waitEnded(t, 1) // the save still reruns it
	if n := walks.Load(); n != 0 {
		t.Fatalf("%d walks without a poller", n)
	}

	realWatch := watchRoot
	watchRoot = func(_ string, _ time.Duration, stop <-chan struct{}, _ chan<- watch.Change) { <-stop }
	t.Cleanup(func() { watchRoot = realWatch })
	c = startWith(t, copyExample(t, "todos"), Options{Poll: time.Hour})
	c.save("app/Todo.alder")
	c.sync()
	if n := walks.Load(); n != 0 {
		t.Fatalf("%d walks with no scratch document open", n)
	}
	c.open("scratch.alder", "> 1\n")
	b.waitEnded(t, 1)
	c.save("app/Todo.alder")
	b.waitEnded(t, 1)
	if n := walks.Load(); n != 1 {
		t.Fatalf("%d walks, want 1 (a scratch document open, a poller running)", n)
	}
}
