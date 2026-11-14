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
	IsPreferred bool           `json:"isPreferred,omitempty"`
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
// itself never runs. Fixes come first, the preferred ones (isPreferred) before the rest; the
// generators, which are not about the text at the cursor, follow as kind refactor, so a client
// that groups or filters by kind keeps them out of its quick-fix list (issue 45). On a hole of
// the build, `Fill ?h with AI (T)`: a quick fix on the hole's W0002 carrying alder.fill, which
// the server runs (#46); offered only to a client that declared workspace.applyEdit.
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
			title := "Fix: " + d.Message
			if d.Fix.Title != "" {
				title = d.Fix.Title
			}
			fixes = append(fixes, codeAction{
				Title: title, Kind: "quickfix", IsPreferred: d.Fix.Preferred, Diagnostics: []Diagnostic{ld},
				Edit: &workspaceEdit{DocumentChanges: []textDocumentEdit{{TextDocument: doc, Edits: []TextEdit{*ld.Data.Fix}}}},
			})
		}
		built := a.lines[rel]
		fixes = disjointFixes(fixes)
		// a preferred fix (the one obvious repair, as a hook's signature) is listed first
		slices.SortStableFunc(fixes, func(a, b codeAction) int {
			switch {
			case a.IsPreferred && !b.IsPreferred:
				return -1
			case b.IsPreferred && !a.IsPreferred:
				return 1
			}
			return 0
		})
		routeActs := routeSignatureActions(a, rel, now, p.Range, doc)
		for _, f := range fixes { // bytes until here; wired on the way out
			for i, dg := range f.Diagnostics {
				f.Diagnostics[i] = s.enc.wireDiagnostic(built, dg)
			}
			ch := &f.Edit.DocumentChanges[0]
			ch.Edits = []TextEdit{s.enc.wireEdit(built, ch.Edits[0])}
			out = append(out, f)
		}
		for _, f := range routeActs { // computed from the text as it is now: already in wire units
			f.Edit.DocumentChanges[0].Edits[0] = s.enc.wireEdit(now, f.Edit.DocumentChanges[0].Edits[0])
			out = append(out, f)
		}
	}
	start := s.byteAtLines(now, p.Range.Start)
	if start.Line < 0 || start.Character < 0 {
		return out, nil
	}
	a := s.lastGood()
	if !freshLines(a, rel, now, start.Line+1, true) {
		return out, nil // the line or one above it has changed since the last good build
	}
	pos := diag.Pos{Line: start.Line + 1, Col: start.Character + 1}
	ctx := hereCtx(a, rel, pos)
	if ctx.Hole != "" && s.applyEdit { // the edit comes back as workspace/applyEdit: nothing else carries it
		if act, ok := s.fillAction(a, rel, ctx, pos); ok {
			out = append(out, act)
		}
	}
	if ctx.Hole != "" && s.searchCmd { // the client runs alder.search itself
		if act, ok := s.searchAction(a, rel, ctx, pos); ok {
			out = append(out, act)
		}
	}
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

// routeSignatureActions offers, on the lines of a `route` of rel's module in the current build,
// to write the signatures of the members the route lacks (`Row.load` and `Row.view` for a GET
// route, `Row.update` for a write route; the checker's E0043 says they are needed), after the
// route's last member or else at the end of the file. The model of a GET route is `m`, unless
// its `load` or `view` is already written, which gives it. The build must still hold for the
// whole file (the edit's place and the route's module are read from it), else nothing is offered.
// Edits are in compiler (byte) positions of now.
func routeSignatureActions(a *analysis, rel string, now []string, rng Range, doc versionedDocument) []codeAction {
	var m *syntax.Module
	for _, x := range a.prog.Modules {
		if x.File == rel {
			m = x
		}
	}
	if m == nil || len(m.Lines) != len(now) || !freshLines(a, rel, now, len(now), true) {
		return nil
	}
	var out []codeAction
	for _, d := range m.Decls {
		r, ok := d.(*syntax.RouteDecl)
		if !ok || !linesOverlap(rng, spanRange(r.Span)) {
			continue
		}
		have := map[string]*syntax.ValueDecl{}
		last := 0
		for _, x := range m.Decls {
			if v, ok := x.(*syntax.ValueDecl); ok && v.Route == r.Name {
				have[v.Name] = v
				last = max(last, v.Span.End.Line)
			}
		}
		// each member is its signature then its definition, a hole for the body
		var sigs, blocks []string
		add := func(sig, params string) {
			sigs = append(sigs, sig)
			name, _, _ := strings.Cut(sig, " : ")
			blocks = append(blocks, sig+"\n"+name+" "+params+" = ?")
		}
		if r.Method == "GET" {
			model := routeModelText(m, have)
			if have["load"] == nil {
				add(r.Name+".load : "+r.Name+".Params -> Session -> Query "+argWrap(model), "params session")
			}
			if have["view"] == nil {
				add(r.Name+".view : "+model+" -> Session -> Html", "model session")
			}
		} else if have["update"] == nil {
			add(r.Name+".update : "+r.Name+".Params -> Session -> Task Response", "params session")
		}
		if len(sigs) == 0 {
			continue
		}
		body := strings.Join(blocks, "\n\n")
		// after the route's last member, else right after the route's own line; a blank line
		// before, and one after unless the next line is blank already or there is none
		line := r.Span.End.Line
		if last > 0 {
			line = last
		}
		text := "\n\n" + body
		if line < len(now) && now[line] != "" {
			text += "\n"
		}
		at := Position{Line: line - 1, Character: len(now[line-1])}
		title := "Insert " + strings.Join(missingNames(sigs), ", ") + " for route " + r.Name
		out = append(out, codeAction{
			Title: title, Kind: "quickfix",
			Edit: &workspaceEdit{DocumentChanges: []textDocumentEdit{{TextDocument: doc, Edits: []TextEdit{{Range: Range{Start: at, End: at}, NewText: text}}}}},
		})
	}
	return out
}

// missingNames is the member names of signatures sigs (`Row.load : …` gives `load`).
func missingNames(sigs []string) []string {
	var out []string
	for _, s := range sigs {
		head, _, _ := strings.Cut(s, " : ")
		_, name, _ := strings.Cut(head, ".")
		out = append(out, name)
	}
	return out
}

// argWrap is type text s as the argument of a type constructor (`Query (List Todo)`).
func argWrap(s string) string {
	if strings.Contains(s, " ") && !strings.HasPrefix(s, "(") && !strings.HasPrefix(s, "{") {
		return "(" + s + ")"
	}
	return s
}

// routeModelText is the model type of a GET route whose members are have, as a signature writes
// it: what its written `load` gives (the argument of `Query`), else what its written `view`
// takes, else `m`.
func routeModelText(m *syntax.Module, have map[string]*syntax.ValueDecl) string {
	// parenthesised only when a function type, which a signature needs as a parameter
	wrap := func(t syntax.Type) string {
		s := spanText(m.Lines, t.GetSpan())
		if _, ok := t.(*syntax.FunType); ok && !strings.HasPrefix(s, "(") {
			s = "(" + s + ")"
		}
		return s
	}
	if d := have["load"]; d != nil && d.Sig != nil {
		if ft, ok := d.Sig.(*syntax.FunType); ok {
			if q, ok := ft.Result.(*syntax.TypeName); ok && q.Name == "Query" && len(q.Args) == 1 {
				return wrap(q.Args[0])
			}
		}
	}
	if d := have["view"]; d != nil && d.Sig != nil {
		if ft, ok := d.Sig.(*syntax.FunType); ok && len(ft.Params) > 0 {
			return wrap(ft.Params[0])
		}
	}
	return "m"
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
// declares a route, the enclosing declaration's name and kind, and the hole under pos. It is the
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
	c.IsRoute = a.prog.IsRouteModule(m.Name)
	if d := declAt(m, pos); d != nil {
		switch d := d.(type) {
		case *syntax.ValueDecl:
			c.Decl, c.DeclKind = d.Name, "value"
		case *syntax.TypeDecl:
			c.Decl, c.DeclKind = d.Name, "type"
		case *syntax.TableDecl:
			c.Decl, c.DeclKind = d.Name, "table"
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

// executeCommandReq answers workspace/executeCommand: alder.explain [code], the code's
// explanation, as `alder explain` prints it, or its error for an unknown code; alder.fill
// [{uri, version, address, hole}], a fill started (fillCommand).
func executeCommandReq(s *server, params json.RawMessage) (any, error) {
	var p executeCommandParams
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, &rpcError{codeInvalidParams, err.Error()}
	}
	if p.Command == "alder.fill" {
		return s.fillCommand(p.Arguments)
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
