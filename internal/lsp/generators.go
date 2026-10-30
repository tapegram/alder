package lsp

import (
	"encoding/json"
	"errors"
	"strings"

	"alder/internal/diag"
	"alder/internal/generate"
)

// generatorParams are the params of alder/generators, alder/preview and alder/plan: the cursor,
// and for preview and plan the generator and the values the user typed, by field name.
type generatorParams struct {
	TextDocument struct {
		URI string `json:"uri"`
	} `json:"textDocument"`
	Position Position          `json:"position"`
	ID       string            `json:"id"`
	Sets     map[string]string `json:"sets"`
}

// generatorItem is one generator of alder/generators' list.
type generatorItem struct {
	ID      string            `json:"id"`
	Summary string            `json:"summary"`
	Here    bool              `json:"here"`
	Why     string            `json:"why,omitempty"`
	Sets    map[string]string `json:"sets,omitempty"`
}

// previewFile is a file a preview would change, with its count of added and removed lines.
type previewFile struct {
	Path    string `json:"path"`
	Action  string `json:"action"`
	Added   int    `json:"added"`
	Removed int    `json:"removed"`
}

type previewResult struct {
	Fields      []generate.FieldState `json:"fields"`
	Files       []previewFile         `json:"files"`
	Diff        string                `json:"diff"`
	Warnings    []string              `json:"warnings"` // Result.Warnings: a file written unformatted (never null)
	Diagnostics []json.RawMessage     `json:"diagnostics"`
	Holes       []generate.Hole       `json:"holes"`
	Ready       bool                  `json:"ready"`
	Error       string                `json:"error"`
}

// planFile is a file a plan changes: its text before (null: created) and after (null: removed).
type planFile struct {
	Path   string  `json:"path"`
	Action string  `json:"action"`
	Before *string `json:"before"`
	After  *string `json:"after"`
}

type planResult struct {
	Files       []planFile        `json:"files"`
	Diagnostics []json.RawMessage `json:"diagnostics"`
	Holes       []generate.Hole   `json:"holes"`
	Imports     []string          `json:"imports"`
	Migration   []string          `json:"migration"`
	Error       string            `json:"error"`
}

func decodeGeneratorParams(params json.RawMessage) (generatorParams, error) {
	var p generatorParams
	if err := json.Unmarshal(params, &p); err != nil {
		return p, &rpcError{codeInvalidParams, err.Error()}
	}
	return p, nil
}

// cursor is the params' document (rel, "" when it is not a file under the root) and position
// as the compiler counts.
func (s *server) cursor(p generatorParams) (string, diag.Pos) {
	rel, _ := s.rel(p.TextDocument.URI)
	return rel, diag.Pos{Line: p.Position.Line + 1, Col: p.Position.Character + 1}
}

// hereAt is spec's prefill at pos in rel from the last good build, or nil: when spec is never
// "here", when rel is not in that build, or when the lines down to pos's have changed since.
func (s *server) hereAt(spec *generate.Spec, rel string, pos diag.Pos) *generate.Match {
	if spec.Here == nil || rel == "" || pos.Line < 1 || pos.Col < 1 {
		return nil
	}
	a := s.lastGood()
	if a == nil || !s.fresh(a, rel, pos.Line, true) {
		return nil
	}
	return spec.Here(hereCtx(a, rel, pos))
}

// generatorsReq answers alder/generators: every generator, the ones that apply at the cursor
// first (with their prefill and its reason), each group in Spec order.
func generatorsReq(s *server, params json.RawMessage) (any, error) {
	p, err := decodeGeneratorParams(params)
	if err != nil {
		return nil, err
	}
	rel, pos := s.cursor(p)
	var here, rest []generatorItem
	for _, spec := range generate.Specs() {
		g := generatorItem{ID: spec.ID, Summary: spec.Summary}
		if m := s.hereAt(spec, rel, pos); m != nil {
			g.Here, g.Why, g.Sets = true, m.Why, m.Sets
			here = append(here, g)
		} else {
			rest = append(rest, g)
		}
	}
	return map[string]any{"generators": append(append([]generatorItem{}, here...), rest...)}, nil
}

// dryRun runs generator id dry on a copy of the app with the open documents overlaid: the form
// (here's prefill at pos, then you over it), and, when every field is valid, the generator's
// result. A refusal is err, a *generate.Refusal.
func (s *server) dryRun(id string, you map[string]string, rel string, pos diag.Pos) (spec *generate.Spec, fields []generate.FieldState, res *generate.Result, ready bool, err error) {
	spec = generate.SpecByID(id)
	if spec == nil {
		return nil, nil, nil, false, &rpcError{codeInvalidParams, "no generator " + id}
	}
	here := s.hereAt(spec, rel, pos)
	// the overlay's keys are s.rel's, which never leave the root: Materialise writes under tmp
	tmp, cleanup, err := generate.Materialise(s.rootReal, s.docs.overlay())
	if err != nil {
		return spec, nil, nil, false, err
	}
	defer cleanup()
	p, _ := generate.Load(tmp) // nil when the app does not check: fields still validate syntactically
	fields, sets, ready := generate.Form(spec, here, you, p)
	if !ready {
		return spec, fields, nil, false, nil
	}
	res, err = spec.Run(sets, generate.App{Dir: tmp, Migrate: s.o.Migrate}, true)
	return spec, fields, res, ready, err
}

// outcome is a dry run's result as both answers report it: the result (the refusal's own when
// it carries one), the diagnostics in `alder … --json`'s shape, the holes, and the refusal's
// text with " (the errors above)" cut, as the CLI's --json does (they are in diagnostics).
// Any error but a refusal is returned as is.
type outcome struct {
	res     *generate.Result // nil when refused
	diags   []json.RawMessage
	holes   []generate.Hole
	refusal string
}

func outcomeOf(res *generate.Result, err error) (outcome, error) {
	o := outcome{diags: []json.RawMessage{}, holes: []generate.Hole{}}
	var ref *generate.Refusal
	if err != nil && !errors.As(err, &ref) {
		return o, err
	}
	shown := res
	if shown == nil && ref != nil {
		shown = ref.Result
	}
	var ds []diag.Diagnostic
	if shown != nil {
		ds = shown.Diagnostics
		if shown.Holes != nil {
			o.holes = shown.Holes
		}
	}
	if len(ds) == 0 && ref != nil {
		ds = ref.Diags
	}
	for _, d := range ds {
		o.diags = append(o.diags, json.RawMessage(diag.RenderJSON(d)))
	}
	if ref != nil {
		o.refusal = strings.Replace(ref.Msg, " (the errors above)", "", 1)
		return o, nil
	}
	o.res = res
	return o, nil
}

// text is a FileChange side's text, "" for nil.
func text(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

// diffCounts are the added and removed lines of the unified diff from before to after. Only its
// first two lines are the `---`/`+++` header: a removed Alder comment (`-- note`) is `--- note`.
func diffCounts(path string, before, after *string) (added, removed int) {
	d := generate.UnifiedDiff(path, text(before), text(after))
	if d == "" {
		return 0, 0
	}
	lines := strings.Split(d, "\n")
	for _, l := range lines[min(2, len(lines)):] {
		switch {
		case strings.HasPrefix(l, "+"):
			added++
		case strings.HasPrefix(l, "-"):
			removed++
		}
	}
	return added, removed
}

// previewReq answers alder/preview: the generator's form at the cursor and, when it is ready,
// the files it would change and their diff. Nothing is written.
func previewReq(s *server, params json.RawMessage) (any, error) {
	p, err := decodeGeneratorParams(params)
	if err != nil {
		return nil, err
	}
	rel, pos := s.cursor(p)
	_, fields, res, ready, err := s.dryRun(p.ID, p.Sets, rel, pos)
	var re *rpcError
	if errors.As(err, &re) {
		return nil, err
	}
	o, err := outcomeOf(res, err)
	if err != nil {
		return nil, err
	}
	out := previewResult{Fields: fields, Files: []previewFile{}, Diagnostics: o.diags, Holes: o.holes, Ready: ready && o.refusal == "", Error: o.refusal, Warnings: []string{}}
	if out.Fields == nil {
		out.Fields = []generate.FieldState{}
	}
	if o.res != nil {
		out.Diff = o.res.Diff
		out.Warnings = append(out.Warnings, o.res.Warnings...)
		for _, f := range o.res.Files {
			add, rem := diffCounts(f.Path, f.Before, f.After)
			out.Files = append(out.Files, previewFile{Path: f.Path, Action: f.Action, Added: add, Removed: rem})
		}
	}
	return out, nil
}

// planReq answers alder/plan: the files the generator changes, each with its text before and
// after, for the client to apply to its buffers. Nothing is written. The diagnostics are the
// copy's, published nowhere; imports is always empty (only `fill` reports imports, and it has
// no Spec). A form that is not ready is refused with its first invalid field ("name: required"):
// the client plans only what its preview showed ready.
func planReq(s *server, params json.RawMessage) (any, error) {
	p, err := decodeGeneratorParams(params)
	if err != nil {
		return nil, err
	}
	rel, pos := s.cursor(p)
	_, fields, res, ready, err := s.dryRun(p.ID, p.Sets, rel, pos)
	var re *rpcError
	if errors.As(err, &re) {
		return nil, err
	}
	o, err := outcomeOf(res, err)
	if err != nil {
		return nil, err
	}
	out := planResult{Files: []planFile{}, Diagnostics: o.diags, Holes: o.holes, Imports: []string{}, Migration: []string{}, Error: o.refusal}
	if !ready {
		for _, f := range fields {
			if !f.Valid {
				out.Error = f.Name + ": " + f.Why
				break
			}
		}
	}
	if o.res != nil {
		for _, f := range o.res.Files {
			out.Files = append(out.Files, planFile{Path: f.Path, Action: f.Action, Before: f.Before, After: f.After})
		}
		for _, n := range o.res.Notes {
			out.Migration = append(out.Migration, strings.TrimSpace(n))
		}
	}
	return out, nil
}
