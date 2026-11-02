package lsp

import (
	"encoding/json"
	"strings"

	"alder/internal/scratch"
	"alder/internal/semtok"
)

// textDocumentParams is the `{"textDocument": {"uri": …}}` shape the document-wide requests
// (semantic tokens, folding, selection) share.
type textDocumentParams struct {
	TextDocument struct {
		URI string `json:"uri"`
	} `json:"textDocument"`
}

// alderDoc is the guard those requests share: rel is the root-relative path of uri, ok is false
// when uri is not a file under the root or not an .alder file (they then answer an empty result).
func (s *server) alderDoc(uri string) (rel string, ok bool) {
	rel, ok = s.rel(uri)
	if !ok || !strings.HasSuffix(rel, ".alder") {
		return "", false
	}
	return rel, true
}

// semanticTokensProvider is the capability: the legend (internal/semtok, which the VS Code
// extension pins too), full and range answers, no deltas (a module's token pass costs about
// what a hover costs; spec 2026-11-01).
func semanticTokensProvider() map[string]any {
	return map[string]any{
		"legend": map[string]any{"tokenTypes": semtok.Types, "tokenModifiers": semtok.Modifiers},
		"full":   true,
		"range":  true,
	}
}

type semanticTokensRangeParams struct {
	textDocumentParams
	Range *Range `json:"range"`
}

// SemanticTokens answers both requests: five integers per token (LSP 3.17), never null.
type SemanticTokens struct {
	Data []int `json:"data"`
}

// semanticTokensFullReq answers textDocument/semanticTokens/full from the document's current
// text: the tokens must match what is on screen, so no analysis is read.
func semanticTokensFullReq(s *server, params json.RawMessage) (any, error) {
	var p textDocumentParams
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, &rpcError{codeInvalidParams, err.Error()}
	}
	return s.semanticTokens(p.TextDocument.URI, nil), nil
}

// semanticTokensRangeReq answers textDocument/semanticTokens/range: the full answer cut to the
// range's lines, encoded from the first kept token.
func semanticTokensRangeReq(s *server, params json.RawMessage) (any, error) {
	var p semanticTokensRangeParams
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, &rpcError{codeInvalidParams, err.Error()}
	}
	return s.semanticTokens(p.TextDocument.URI, p.Range), nil
}

func (s *server) semanticTokens(uri string, rng *Range) SemanticTokens {
	out := SemanticTokens{Data: []int{}}
	rel, ok := s.alderDoc(uri)
	if !ok {
		return out
	}
	text := s.text(rel)
	var toks []semtok.Token
	if scratch.IsScratchFile(rel) {
		toks = semtok.Scratch(rel, text)
	} else {
		toks = semtok.Module(rel, []byte(text))
	}
	if rng != nil {
		lo, hi := rng.Start.Line+1, rng.End.Line+1
		kept := toks[:0]
		for _, t := range toks {
			if t.Line >= lo && t.Line <= hi {
				kept = append(kept, t)
			}
		}
		toks = kept
	}
	out.Data = encodeTokens(s.enc, strings.Split(text, "\n"), toks)
	return out
}

// encodeTokens is toks (sorted, one line each, byte columns) as the LSP's relative integers: line
// delta, start delta (from the previous token's start when on the same line), length, type,
// modifiers, with starts and lengths in enc's unit.
func encodeTokens(enc posEncoding, lines []string, toks []semtok.Token) []int {
	data := make([]int, 0, 5*len(toks))
	prevLine, prevChar := 0, 0
	for _, t := range toks {
		line := ""
		if t.Line-1 < len(lines) {
			line = lines[t.Line-1]
		}
		start := enc.toWire(line, t.Col-1)
		length := enc.toWire(line, t.Col-1+t.Len) - start
		dl, ds := t.Line-1-prevLine, start
		if dl == 0 {
			ds = start - prevChar
		}
		data = append(data, dl, ds, length, int(t.Kind), t.Mods)
		prevLine, prevChar = t.Line-1, start
	}
	return data
}
