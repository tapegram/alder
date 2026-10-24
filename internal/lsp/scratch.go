package lsp

import (
	"context"
	"fmt"
	"path/filepath"
	"runtime/debug"
	"sync"

	"alder/internal/scratch"
)

// runScratchFile is scratch.RunFile; a test may replace it before a server starts.
var runScratchFile = scratch.RunFile

// appErrorsWarning is the scratch file's warning at 1:1 when the app it runs against has errors.
const appErrorsWarning = "the app has errors; fix them and save"

// scratchRuns are the runs of the open scratch documents, by rel: one at a time for each.
type scratchRuns struct {
	mu    sync.Mutex
	byRel map[string]*scratchRun
}

// scratchRun is one scratch document's run slot: held for the length of a run, so a second run
// of the document waits for the first and then takes its newest text.
type scratchRun struct {
	mu sync.Mutex
}

// run is rel's run slot, made on first use.
func (r *scratchRuns) run(rel string) *scratchRun {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.byRel == nil {
		r.byRel = map[string]*scratchRun{}
	}
	x := r.byRel[rel]
	if x == nil {
		x = &scratchRun{}
		r.byRel[rel] = x
	}
	return x
}

// runScratch starts a run of the open scratch document rel and returns at once. The run sends
// alder/scratch with phase "types" once the file has checked and "values" when it ends, each
// with the file's diagnostics published first.
func (s *server) runScratch(rel string) {
	slot := s.scratch.run(rel)
	s.bg.Go(func() {
		slot.mu.Lock()
		defer slot.mu.Unlock()
		select {
		case <-s.stop:
			return
		default:
		}
		s.scratchOnce(rel)
	})
}

// scratchOnce runs the open scratch document rel as it is now (nothing when it has closed). A
// panic is logged with its stack, so the server keeps serving.
func (s *server) scratchOnce(rel string) {
	defer func() {
		if r := recover(); r != nil {
			fmt.Fprintf(s.o.Log, "alder lsp: scratch %s: internal error: %v\n%s", rel, r, debug.Stack())
		}
	}()
	x := s.docs.get(rel)
	if x == nil {
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { // the server stopping cancels the run
		select {
		case <-s.stop:
			cancel()
		case <-ctx.Done():
		}
	}()
	f, parsed := scratch.ParseFile(rel, x.Text)
	send := func(phase string, r scratch.FileResult) {
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
	if ctx.Err() != nil {
		return // the server is stopping
	}
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
		ds = append(ds, Diagnostic{Range: lspRange(1, 1, 1, 1), Severity: 2, Source: "alder", Message: appErrorsWarning})
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
