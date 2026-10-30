package lsp

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	"alder/internal/diag"
	"alder/internal/generate"
	"alder/internal/scratch"
	"alder/internal/syntax"
)

type codeActionParams struct {
	TextDocument struct {
		URI string `json:"uri"`
	} `json:"textDocument"`
	Range Range `json:"range"`
}

type codeAction struct {
	Title       string         `json:"title"`
	Kind        string         `json:"kind"`
	Diagnostics []Diagnostic   `json:"diagnostics,omitempty"`
	Edit        *workspaceEdit `json:"edit,omitempty"`
	Data        *generatorHere `json:"data,omitempty"`
	Command     *command       `json:"command,omitempty"`
}

// workspaceEdit carries its edits as documentChanges, each naming the document version it was
// computed from, so a client refuses an edit to a buffer that has changed since.
type workspaceEdit struct {
	DocumentChanges []textDocumentEdit `json:"documentChanges"`
}

type textDocumentEdit struct {
	TextDocument versionedDocument `json:"textDocument"`
	Edits        []TextEdit        `json:"edits"`
}

// versionedDocument is an OptionalVersionedTextDocumentIdentifier: Version is null for a file
// that was not open when the build read it.
type versionedDocument struct {
	URI     string `json:"uri"`
	Version *int   `json:"version"`
}

type command struct {
	Title     string `json:"title"`
	Command   string `json:"command"`
	Arguments []any  `json:"arguments"`
}

// generatorHere is a generator that applies at the cursor: its id and its prefill.
type generatorHere struct {
	ID   string            `json:"id"`
	Sets map[string]string `json:"sets"`
	Why  string            `json:"why"`
}

// codeActionReq answers textDocument/codeAction: the compiler's fixes for the diagnostics on the
// range's lines (from the current build, so a parse error's fix counts too) whose lines are
// unchanged since that build through the fix's end line, then every generator that applies at
// range.start in the last good build when range.start's line is unchanged too (as hereAt). A
// generator's action carries the client command alder.generate (Ruling R8), which the server
// itself never runs.
func codeActionReq(s *server, params json.RawMessage) (any, error) {
	var p codeActionParams
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, &rpcError{codeInvalidParams, err.Error()}
	}
	out := []codeAction{}
	rel, ok := s.rel(p.TextDocument.URI)
	if !ok {
		return out, nil
	}
	if scratch.IsScratchFile(rel) {
		return append(out, s.scratchActions(rel, p.Range)...), nil
	}
	now := strings.Split(s.text(rel), "\n")
	if a := s.current(); a != nil {
		doc := versionedDocument{URI: s.uriOf(rel)}
		if v, open := a.versions[rel]; open {
			doc.Version = &v
		}
		var fixes []codeAction
		for _, d := range a.diags {
			if d.Span.File != rel || d.Fix == nil {
				continue
			}
			ld := lspDiagnostic(d)
			if !linesOverlap(ld.Range, p.Range) || !freshLines(a, rel, now, d.Fix.EndLine, true) {
				continue
			}
			fixes = append(fixes, codeAction{
				Title: "Fix: " + d.Message, Kind: "quickfix", Diagnostics: []Diagnostic{ld},
				Edit: &workspaceEdit{DocumentChanges: []textDocumentEdit{{TextDocument: doc, Edits: []TextEdit{*ld.Data.Fix}}}},
			})
		}
		out = append(out, disjointFixes(fixes)...)
	}
	start := p.Range.Start
	if start.Line < 0 || start.Character < 0 {
		return out, nil
	}
	a := s.lastGood()
	if !freshLines(a, rel, now, start.Line+1, true) {
		return out, nil // the line or one above it has changed since the last good build
	}
	ctx := hereCtx(a, rel, diag.Pos{Line: start.Line + 1, Col: start.Character + 1})
	for _, spec := range generate.Specs() {
		if spec.Here == nil {
			continue
		}
		m := spec.Here(ctx)
		if m == nil {
			continue
		}
		title := hereTitle(spec.ID, m.Sets)
		g := &generatorHere{ID: spec.ID, Sets: m.Sets, Why: m.Why}
		out = append(out, codeAction{
			Title: title, Kind: "refactor", Data: g,
			Command: &command{Title: title, Command: "alder.generate", Arguments: []any{g}},
		})
	}
	return out, nil
}

// disjointFixes drops the quick fixes whose edit overlaps another's, so a client is never offered
// two rewrites of the same text that cannot both apply (a full-path constructor, `Table.Todo.Open`,
// is E0205 and E0203 on one span). Of an overlapping group the widest edit is kept; on equal
// ranges one other than E0205 (the first of those), which is the one that leaves checked code
// (`Open`, where E0205's `Todo.Open` is still E0203). The dropped fixes' diagnostics are
// added to the kept action, so the action is offered on each of them. Each action carries one
// edit, and the order of the rest is unchanged.
func disjointFixes(fixes []codeAction) []codeAction {
	edit := func(a codeAction) TextEdit { return a.Edit.DocumentChanges[0].Edits[0] }
	before := func(a, b Position) bool { return a.Line < b.Line || a.Line == b.Line && a.Character < b.Character }
	// Two zero-width insertions at one point count as overlapping (a == b): their order in the
	// text would be the client's choice. Today only E0201's import insertions do this, and the
	// resolver offers each module's once (importFix), so nothing is lost.
	overlap := func(a, b Range) bool {
		return before(a.Start, b.End) && before(b.Start, a.End) || a == b
	}
	// better: a's fix beats b's, an overlapping one. Wider wins; on equal ranges a fix other than
	// E0205 wins, since E0205 only shortens the module path and leaves what the other code fixes.
	better := func(a, b codeAction) bool {
		ra, rb := edit(a).Range, edit(b).Range
		if ra == rb {
			return a.Diagnostics[0].Code != diag.FullPath && b.Diagnostics[0].Code == diag.FullPath
		}
		return before(ra.Start, rb.Start) && !before(ra.End, rb.End) || !before(rb.Start, ra.Start) && before(rb.End, ra.End)
	}
	keep := make([]bool, len(fixes))
	for i := range keep {
		keep[i] = true
	}
	for i := range fixes {
		for j := range fixes {
			if i == j || !keep[i] || !keep[j] || !overlap(edit(fixes[i]).Range, edit(fixes[j]).Range) {
				continue
			}
			drop := j // i is kept unless j is better
			if better(fixes[j], fixes[i]) {
				drop = i
			}
			winner := i + j - drop
			fixes[winner].Diagnostics = append(fixes[winner].Diagnostics, fixes[drop].Diagnostics...)
			keep[drop] = false
		}
	}
	var out []codeAction
	for i, f := range fixes {
		if keep[i] {
			out = append(out, f)
		}
	}
	return out
}

// linesOverlap reports whether a and b share a line. An end at a line's first byte does not
// reach that line, unless the range starts there too.
func linesOverlap(a, b Range) bool {
	last := func(r Range) int {
		if r.End.Character == 0 && r.End.Line > r.Start.Line {
			return r.End.Line - 1
		}
		return r.End.Line
	}
	return a.Start.Line <= last(b) && b.Start.Line <= last(a)
}

// hereTitle is "id (k = v, …)", the sets in key order; just id without sets.
func hereTitle(id string, sets map[string]string) string {
	if len(sets) == 0 {
		return id
	}
	keys := make([]string, 0, len(sets))
	for k := range sets {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	parts := make([]string, len(keys))
	for i, k := range keys {
		parts[i] = k + " = " + sets[k]
	}
	return id + " (" + strings.Join(parts, ", ") + ")"
}

// hereCtx is where pos (a compiler position) is in rel, in a: its module, whether that module
// declares a page, the enclosing declaration's name and kind, and the hole under pos. It is the
// zero HereCtx when a has no module for rel.
func hereCtx(a *analysis, rel string, pos diag.Pos) generate.HereCtx {
	var c generate.HereCtx
	if a == nil {
		return c
	}
	var m *syntax.Module
	for _, x := range a.prog.Modules {
		if x.File == rel {
			m = x
			break
		}
	}
	if m == nil {
		return c
	}
	c.Module = m.Name
	c.IsPage = a.prog.Pages[m.Name] != nil
	if d := declAt(m, pos); d != nil {
		switch d := d.(type) {
		case *syntax.ValueDecl:
			c.Decl, c.DeclKind = d.Name, "value"
		case *syntax.TypeDecl:
			c.Decl, c.DeclKind = d.Name, "type"
		case *syntax.TableDecl:
			c.Decl, c.DeclKind = d.Name, "table"
		case *syntax.PageDecl:
			c.DeclKind = "page" // a page has no name of its own
		case *syntax.MsgDecl:
			c.Decl, c.DeclKind = d.Name, "msg"
		case *syntax.FormDecl:
			c.Decl, c.DeclKind = d.Name, "form"
		case *syntax.CapDecl:
			c.Decl, c.DeclKind = d.Name, "capability"
		case *syntax.TestDecl:
			c.Decl, c.DeclKind = d.Name, "test"
		}
	}
	if path := syntax.NodeAt(m, pos); len(path) > 0 {
		if h, ok := path[len(path)-1].(*syntax.HoleExpr); ok {
			c.Hole = h.Name
		}
	}
	return c
}

type executeCommandParams struct {
	Command   string            `json:"command"`
	Arguments []json.RawMessage `json:"arguments"`
}

// executeCommandReq answers workspace/executeCommand. The one command is alder.explain [code]:
// the code's explanation, as `alder explain` prints it, or its error for an unknown code.
func executeCommandReq(s *server, params json.RawMessage) (any, error) {
	var p executeCommandParams
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, &rpcError{codeInvalidParams, err.Error()}
	}
	if p.Command != "alder.explain" {
		return nil, &rpcError{codeInvalidParams, "unknown command " + p.Command}
	}
	var code string
	if len(p.Arguments) != 1 || json.Unmarshal(p.Arguments[0], &code) != nil {
		return nil, &rpcError{codeInvalidParams, "alder.explain takes one argument, a diagnostic code"}
	}
	text, ok := diag.Explain[code]
	if !ok {
		return nil, &rpcError{codeInvalidParams, fmt.Sprintf("unknown diagnostic code %s", code)}
	}
	return text, nil
}
