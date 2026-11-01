package lsp

import (
	"encoding/json"
	"strings"

	"alder/internal/diag"
	"alder/internal/syntax"
)

// SelectionRange is one step of an expand-selection chain: a range and the next larger one.
type SelectionRange struct {
	Range  Range           `json:"range"`
	Parent *SelectionRange `json:"parent,omitempty"`
}

type selectionRangeParams struct {
	textDocumentParams
	Positions []Position `json:"positions"`
}

// selectionRangeReq answers textDocument/selectionRange from the current text: for each
// position, the nodes from the innermost holding it out to the module (syntax.NodeAt), equal
// spans collapsed. LSP wants one result per position, so where no node holds the position, or
// the document cannot be parsed, or is not one of ours, the chain is a zero-width range there.
func selectionRangeReq(s *server, params json.RawMessage) (any, error) {
	var p selectionRangeParams
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, &rpcError{codeInvalidParams, err.Error()}
	}
	out := make([]SelectionRange, 0, len(p.Positions))
	rel, ok := s.alderDoc(p.TextDocument.URI)
	var d syntaxDoc
	if ok {
		d = parseSyntaxDoc(rel, s.text(rel))
	}
	for _, wp := range p.Positions {
		zero := SelectionRange{Range: Range{Start: wp, End: wp}}
		if !ok || d.m == nil || !d.hasModuleLine() {
			out = append(out, zero)
			continue
		}
		bp := s.byteAtLines(d.lines, wp)
		at := d.toModule(diag.Pos{Line: bp.Line + 1, Col: bp.Character + 1})
		var chain []Range // innermost first, in bytes
		path := syntax.NodeAt(d.m, at)
		for i := len(path) - 1; i >= 0; i-- {
			sp := path[i].GetSpan()
			if !syntax.Contains(sp, at) {
				continue
			}
			a, b := d.toFile(sp.Start), d.toFile(sp.End)
			r := Range{Start: Position{a.Line - 1, a.Col - 1}, End: Position{b.Line - 1, b.Col - 1}}
			if n := len(chain); n > 0 && chain[n-1] == r {
				continue
			}
			chain = append(chain, r)
		}
		if len(chain) == 0 {
			out = append(out, zero)
			continue
		}
		var parent *SelectionRange
		for i := len(chain) - 1; i >= 0; i-- {
			parent = &SelectionRange{Range: s.enc.wireRange(d.lines, chain[i]), Parent: parent}
		}
		out = append(out, *parent)
	}
	return out, nil
}

// hasModuleLine is whether the document's first token is the keyword `module` (the parser's
// own test, so a BOM, tabs and a bare `module` line count as the lexer counts them); without one
// the parser recovers a fragment whose nodes are not worth selecting. A scratch file always
// parses behind its own `module Scratch`.
func (d syntaxDoc) hasModuleLine() bool {
	if d.scratch != nil {
		return true
	}
	lx := syntax.NewLexer("", []byte(strings.Join(d.lines, "\n")))
	for {
		t := lx.Next()
		if t.Kind == syntax.NL {
			continue
		}
		return t.Kind == syntax.Keyword && t.Text == "module"
	}
}
