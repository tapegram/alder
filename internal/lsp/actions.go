package lsp

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	"alder/internal/diag"
	"alder/internal/generate"
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

type workspaceEdit struct {
	Changes map[string][]TextEdit `json:"changes"`
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
// range's lines (from the current build, so a parse error's fix counts too), then every
// generator that applies at range.start in the last good build. A generator's action carries
// the client command alder.generate (Ruling R8), which the server itself never runs.
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
	if a := s.current(); a != nil {
		uri := s.uriOf(rel)
		for _, d := range a.diags {
			if d.Span.File != rel || d.Fix == nil {
				continue
			}
			ld := lspDiagnostic(d)
			if !linesOverlap(ld.Range, p.Range) {
				continue
			}
			out = append(out, codeAction{
				Title: "Fix: " + d.Message, Kind: "quickfix", Diagnostics: []Diagnostic{ld},
				Edit: &workspaceEdit{Changes: map[string][]TextEdit{uri: {*ld.Data.Fix}}},
			})
		}
	}
	start := p.Range.Start
	if start.Line < 0 || start.Character < 0 {
		return out, nil
	}
	a := s.lastGood()
	if !s.fresh(a, rel, start.Line+1, false) {
		return out, nil // a line above has changed since the last good build
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
