package lsp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"alder/internal/diag"
	"alder/internal/fill"
	"alder/internal/generate"
)

// fillArgs is the argument of the alder.fill command: the hole's document, its version when the
// action was offered, the declaration's address and the hole's name.
type fillArgs struct {
	URI     string `json:"uri"`
	Version *int   `json:"version"`
	Address string `json:"address"`
	Hole    string `json:"hole"`
	Type    string `json:"type,omitempty"` // the hole's type, for the progress title
}

// fillAction is the `Fill ?h with AI (T)` quick fix for the hole ctx names at pos in rel, from
// analysis a (the last good build, whose lines down to pos are unchanged): attached to the
// hole's W0002, carrying the alder.fill command. ok is false when a has no such hole.
func (s *server) fillAction(a *analysis, rel string, ctx generate.HereCtx, pos diag.Pos) (codeAction, bool) {
	for _, h := range generate.HoleIndex(a.prog, a.diags) {
		if h.File != rel || h.Name != ctx.Hole || !within(h.Span, pos) {
			continue
		}
		var ds []Diagnostic
		for _, d := range a.diags {
			if d.Code == "W0002" && d.Span == h.Span {
				ds = append(ds, lspDiagnostic(d))
			}
		}
		arg := fillArgs{URI: s.uriOf(rel), Address: h.Address(), Hole: h.Name, Type: h.Type}
		if v, open := a.versions[rel]; open {
			arg.Version = &v
		}
		title := fmt.Sprintf("Fill ?%s with AI (%s)", h.Name, h.Type)
		return codeAction{Title: title, Kind: "quickfix", Diagnostics: ds,
			Command: &command{Title: title, Command: "alder.fill", Arguments: []any{arg}}}, true
	}
	return codeAction{}, false
}

// fillCommand runs alder.fill: it validates the argument, refuses a second fill while one runs
// (one at a time per server) and a server with no backend (a warning each), then starts the
// fill under s.bg and answers {"started": true} at once, so previews and plans are not held up
// while the model answers.
func (s *server) fillCommand(args []json.RawMessage) (any, error) {
	var arg fillArgs
	if len(args) != 1 || json.Unmarshal(args[0], &arg) != nil || arg.Address == "" || arg.Hole == "" {
		return nil, &rpcError{codeInvalidParams, "alder.fill takes one argument, {uri, version, address, hole}"}
	}
	rel, ok := s.rel(arg.URI)
	if !ok {
		return nil, &rpcError{codeInvalidParams, "alder.fill: not a file of the app: " + arg.URI}
	}
	var backend fill.Backend
	var model string
	if s.o.Fill != nil {
		backend, model = s.o.Fill()
	}
	if backend == nil {
		s.showMessage(2, "alder fill: "+fill.NoBackend.Error())
		return map[string]any{"started": false}, nil
	}
	if !s.filling.CompareAndSwap(false, true) {
		s.showMessage(2, "alder fill: a fill is already running; wait for it to end")
		return map[string]any{"started": false}, nil
	}
	s.bg.Go(func() {
		defer s.filling.Store(false)
		s.runFill(rel, arg, backend, model)
	})
	return map[string]any{"started": true}, nil
}

// runFill is the fill itself, on its own goroutine: the app with the open documents overlaid
// (as a dry run sees it), fill.Run with progress, then the edit, versioned at the version of
// the hole's document the fill read; not sent when that document has changed since. A
// changed or refused edit, a failure or an error is a warning.
func (s *server) runFill(rel string, arg fillArgs, backend fill.Backend, model string) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		select {
		case <-s.stop:
			cancel()
		case <-ctx.Done():
		}
	}()
	over, vers := s.docs.snapshot()
	tmp, cleanup, err := generate.Materialise(s.rootReal, over)
	if err != nil {
		s.showMessage(2, "alder fill: "+err.Error())
		return
	}
	defer cleanup()
	title := "alder: filling ?" + arg.Hole
	if arg.Type != "" {
		title += " : " + arg.Type
	}
	progress := s.progress(title)
	progress.begin(model)
	r, err := fill.Run(ctx, backend, tmp, arg.Address, arg.Hole, func(a fill.Attempt) {
		progress.report(fmt.Sprintf("attempt %d of %d: %s", a.N, fill.Attempts, a.Why))
	})
	progress.end()
	var f *fill.Failure
	switch {
	case errors.As(err, &f):
		s.showMessage(2, f.Text())
		return
	case errors.Is(err, context.Canceled):
		return
	case err != nil:
		s.showMessage(2, "alder fill: "+err.Error())
		return
	}
	var changes []textDocumentEdit
	for _, fc := range r.Files {
		if fc.Action != "edited" || fc.Before == nil || fc.After == nil {
			continue
		}
		doc := versionedDocument{URI: s.uriOf(fc.Path)}
		if v, open := vers[fc.Path]; open {
			doc.Version = &v
		}
		var edits []TextEdit
		for _, e := range generate.LineEdits(*fc.Before, *fc.After) {
			edits = append(edits, TextEdit{Range: Range{Start: Position{Line: e.Start}, End: Position{Line: e.End}}, NewText: e.Text})
		}
		changes = append(changes, textDocumentEdit{TextDocument: doc, Edits: edits})
	}
	if len(changes) == 0 {
		return
	}
	// A buffer edited since the snapshot gets no edit. Neovim does not refuse a versioned edit
	// to a newer buffer (it prints "Buffer … newer than edits" and answers applied: true), so the
	// server compares the versions itself before it sends.
	for _, fc := range r.Files {
		if v, open := vers[fc.Path]; open {
			if now := s.docs.get(fc.Path); now == nil || now.Version != v {
				s.showMessage(2, fmt.Sprintf("alder fill: %s changed while filling; the expression was:\n%s", fc.Path, r.Expression))
				return
			}
		}
	}
	res, err := s.request("workspace/applyEdit", map[string]any{"label": "alder fill ?" + arg.Hole, "edit": workspaceEdit{DocumentChanges: changes}})
	if err != nil {
		select {
		case <-s.stop:
		default:
			s.showMessage(2, "alder fill: "+err.Error())
		}
		return
	}
	var applied struct {
		Applied bool `json:"applied"`
	}
	json.Unmarshal(res, &applied)
	if !applied.Applied { // a client that does refuse it
		s.showMessage(2, fmt.Sprintf("alder fill: %s changed while filling; the expression was:\n%s", rel, r.Expression))
	}
}

// showMessage sends window/showMessage (typ 1 error, 2 warning, 3 info).
func (s *server) showMessage(typ int, msg string) {
	s.notify("window/showMessage", map[string]any{"type": typ, "message": msg})
}

// workProgress is one `$/progress` run, shown in the client's status line; every method does
// nothing when the client did not declare window.workDoneProgress, or refused the token.
type workProgress struct {
	s     *server
	token string
	title string
	on    bool
}

// progress creates a progress token (window/workDoneProgress/create) when the client takes
// them.
func (s *server) progress(title string) *workProgress {
	p := &workProgress{s: s, title: title, token: fmt.Sprintf("alder-fill-%d", s.reqSeq.Add(1))}
	if s.workDoneProgress {
		_, err := s.request("window/workDoneProgress/create", map[string]any{"token": p.token})
		p.on = err == nil
	}
	return p
}

func (p *workProgress) send(value map[string]any) {
	if p.on {
		p.s.notify("$/progress", map[string]any{"token": p.token, "value": value})
	}
}

func (p *workProgress) begin(message string) {
	p.send(map[string]any{"kind": "begin", "title": p.title, "message": message, "cancellable": false})
}

func (p *workProgress) report(message string) {
	p.send(map[string]any{"kind": "report", "message": strings.TrimSpace(message)})
}

func (p *workProgress) end() { p.send(map[string]any{"kind": "end"}) }
