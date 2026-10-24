package lsp

import (
	"context"
	"fmt"
	"path/filepath"
	"runtime/debug"
	"sync"

	"alder/internal/scratch"
	"alder/internal/watch"
)

// runScratchFile is scratch.RunFile; a test may replace it before a server starts.
var runScratchFile = scratch.RunFile

// scratchRunEnded is called as each scratch run's goroutine ends, its sends done; a test may
// replace it before a server starts.
var scratchRunEnded = func(rel string) {}

// scratchRuns are the runs of the open scratch documents: the current one of each, by rel. A run
// sends only under mu, and only while it is current, so a superseded or closed run sends nothing.
type scratchRuns struct {
	mu    sync.Mutex
	byRel map[string]*scratchRun
	seq   uint64 // the newest run's seq: every run's is unique, across all rels
	// sources are the stamps of the watched files (watch.Snapshot) when the open scratch
	// documents were last rerun for a change to them; nil before the first such rerun.
	sources map[string]watch.Stamp
}

// scratchRun is one run of a scratch document.
type scratchRun struct {
	seq    uint64
	cancel context.CancelFunc
}

// runScratch cancels the open scratch document rel's current run and starts another, returning
// at once. The run sends alder/scratch with phase "types" once the file has checked and "values"
// when it ends, each with the file's diagnostics published first. Nothing starts once the
// server is stopping.
func (s *server) runScratch(rel string) {
	ctx, cancel := context.WithCancel(context.Background())
	r := &s.scratch
	r.mu.Lock()
	defer r.mu.Unlock()
	select {
	case <-s.stop:
		cancel()
		return
	default:
	}
	if s.docs.get(rel) == nil {
		cancel()
		return // closed meanwhile (a close racing a poll rerun): nothing to run
	}
	if r.byRel == nil {
		r.byRel = map[string]*scratchRun{}
	}
	if old := r.byRel[rel]; old != nil {
		old.cancel()
	}
	r.seq++
	run := &scratchRun{seq: r.seq, cancel: cancel}
	r.byRel[rel] = run
	s.bg.Go(func() {
		defer scratchRunEnded(rel)
		defer cancel()
		s.scratchOnce(ctx, rel, run.seq)
	})
}

// rerunScratch runs every open scratch document again: after a change to the app they may use.
func (s *server) rerunScratch() {
	for _, rel := range s.docs.scratchRels() {
		s.runScratch(rel)
	}
}

// saveRerunScratch reruns every open scratch document after an editor save of another file, and
// records the watched files' stamps, which include that save's write: the poll that later sees
// the write then finds nothing new (scratchSourcesChanged), so one save is one run.
func (s *server) saveRerunScratch() {
	cur := watch.Snapshot(s.rootReal)
	s.scratch.mu.Lock()
	s.scratch.sources = cur
	s.scratch.mu.Unlock()
	s.rerunScratch()
}

// scratchSourcesChanged reports whether the watched files differ, in a source, from their stamps
// at the last rerun for a change to them, and records the stamps now. The poller calls it on a
// source change: false means an editor save already reran the scratch documents for it.
func (s *server) scratchSourcesChanged() bool {
	cur := watch.Snapshot(s.rootReal)
	s.scratch.mu.Lock()
	defer s.scratch.mu.Unlock()
	last := s.scratch.sources
	s.scratch.sources = cur
	return last == nil || watch.Diff(last, cur)&watch.SourceChange != 0
}

// stopScratch cancels the closed scratch document rel's run, forgets it, and clears its
// diagnostics at uri, the URI the document was opened by.
func (s *server) stopScratch(rel, uri string) {
	r := &s.scratch
	r.mu.Lock()
	defer r.mu.Unlock()
	if run := r.byRel[rel]; run != nil {
		run.cancel()
		delete(r.byRel, rel)
	}
	s.notify("textDocument/publishDiagnostics", publishDiagnosticsParams{URI: uri, Diagnostics: []Diagnostic{}})
}

// stopAllScratch cancels every run: the server is stopping (s.stop is closed, so none starts).
func (s *server) stopAllScratch() {
	r := &s.scratch
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, run := range r.byRel {
		run.cancel()
	}
}

// scratchOnce is run seq of the open scratch document rel, as it is now (nothing when it has
// closed). A panic is logged with its stack, so the server keeps serving.
func (s *server) scratchOnce(ctx context.Context, rel string, seq uint64) {
	defer func() {
		if r := recover(); r != nil {
			fmt.Fprintf(s.o.Log, "alder lsp: scratch %s: internal error: %v\n%s", rel, r, debug.Stack())
		}
	}()
	x := s.docs.get(rel)
	if x == nil {
		return
	}
	f, parsed := scratch.ParseFile(rel, x.Text)
	send := func(phase string, r scratch.FileResult) {
		s.scratch.mu.Lock()
		defer s.scratch.mu.Unlock()
		if run := s.scratch.byRel[rel]; run == nil || run.seq != seq || ctx.Err() != nil || s.docs.get(rel) == nil {
			return // superseded, closed or stopping
		}
		s.publishScratch(x, r)
		p := scratchParams{URI: x.URI, Version: x.Version, Phase: phase, Watches: r.Watches,
			Ran: r.Ran, Note: r.Note, Warning: r.Warning}
		if p.Watches == nil {
			p.Watches = []scratch.WatchResult{}
		}
		if r.Err != nil {
			p.Error = r.Err.Error()
		}
		s.notify("alder/scratch", p)
	}
	r := runScratchFile(ctx, s.rootReal, f, scratch.FileOptions{
		Out: s.scratchOut(rel), Parsed: parsed,
		OnTypes: func(r scratch.FileResult) { send("types", r) },
	})
	send("values", r)
}

// publishScratch publishes the scratch document x's diagnostics from r: the file's own, and a
// warning at 1:1 when the app has errors (the app's diagnostics are the analysis's to publish).
// Values are never diagnostics.
func (s *server) publishScratch(x *doc, r scratch.FileResult) {
	ds := []Diagnostic{}
	appErr := false
	for _, d := range r.Diags {
		switch {
		case d.Span.File == x.Rel:
			ds = append(ds, lspDiagnostic(d))
		case d.Severity == "error":
			appErr = true
		}
	}
	if appErr {
		ds = append(ds, Diagnostic{Range: lspRange(1, 1, 1, 1), Severity: 2, Source: "alder", Message: scratch.AppErrorsNote})
	}
	v := x.Version
	s.notify("textDocument/publishDiagnostics", publishDiagnosticsParams{URI: x.URI, Version: &v, Diagnostics: ds})
}

// scratchOut is the build directory of the scratch file rel, as `alder scratch` names it:
// build/<app>/scratch/<FilePurpose>.
func (s *server) scratchOut(rel string) string {
	abs := filepath.Join(s.rootReal, filepath.FromSlash(rel))
	return filepath.Join(s.rootReal, "build", filepath.Base(s.rootReal), "scratch", scratch.FilePurpose(abs))
}

// notify sends the notification method with params.
func (s *server) notify(method string, params any) {
	if err := s.c.send(map[string]any{"jsonrpc": "2.0", "method": method, "params": params}); err != nil {
		fmt.Fprintf(s.o.Log, "alder lsp: %v\n", err)
	}
}
