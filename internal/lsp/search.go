package lsp

import (
	"encoding/json"
	"strings"

	"alder/internal/search"
	"alder/internal/types"
)

type searchParams struct {
	Query    string    `json:"query"`
	URI      string    `json:"uri,omitempty"`
	Position *Position `json:"position,omitempty"`
	Limit    int       `json:"limit,omitempty"` // 0 is the engine's default, negative no limit
}

// searchResult is alder/search's answer: `alder search --json`'s object.
type searchResult struct {
	Query   string       `json:"query"`
	Mode    string       `json:"mode"`
	Note    string       `json:"note"`
	Results []searchItem `json:"results"`
}

// searchItem has searchRowJSON's fields (`alder search --json`) with the declaration's place as a
// file URI and an LSP range in place of file, line and col; stdlib entries have neither.
type searchItem struct {
	Name      string `json:"name"`
	Module    string `json:"module"`
	Kind      string `json:"kind"`
	Signature string `json:"signature"`
	Doc       string `json:"doc"`
	Source    string `json:"source"`
	Effect    string `json:"effect,omitempty"`
	Match     string `json:"match,omitempty"`
	Args      *int   `json:"args,omitempty"` // set in produces mode, including 0
	URI       string `json:"uri,omitempty"`
	Range     *Range `json:"range,omitempty"`
}

// searchReq answers alder/search (params searchParams): the standard library's and the app's
// declarations found by name or by type, from the last build without errors (searchSnapshot).
// With no query and a position on a typed hole, the query is the hole's (holeQuery), and the
// answer's query is the text searched. A query never fails: malformed params are the only error,
// and a panic gives the empty result.
func searchReq(s *server, params json.RawMessage) (out any, err error) {
	var p searchParams
	if len(params) > 0 {
		if err := json.Unmarshal(params, &p); err != nil {
			return nil, &rpcError{codeInvalidParams, err.Error()}
		}
	}
	q := strings.TrimSpace(p.Query)
	empty := searchResult{Query: q, Mode: "name", Results: []searchItem{}}
	defer func() {
		if r := recover(); r != nil {
			out, err = empty, nil
		}
	}()
	if q == "" && p.URI != "" && p.Position != nil {
		if rel, ok := s.rel(p.URI); ok {
			if hq, ok := s.holeQuery(rel, *p.Position); ok {
				q = hq
				empty.Query = q
			}
		}
	}
	a := s.searchSnapshot()
	if a == nil {
		return empty, nil
	}
	ix := a.searchIndex()
	if ix == nil {
		return empty, nil
	}
	resp := ix.Search(q, search.Options{Limit: p.Limit})
	res := searchResult{Query: resp.Query, Mode: resp.Mode, Note: resp.Note, Results: make([]searchItem, 0, len(resp.Results))}
	for _, h := range resp.Results {
		e := h.Entry
		it := searchItem{Name: e.Name, Module: e.Module, Kind: string(e.Kind), Signature: e.Signature, Doc: e.Doc,
			Source: e.Source, Effect: e.Effect, Match: h.Match}
		if resp.Mode == "produces" {
			n := h.Args
			it.Args = &n
		}
		if e.File != "" {
			r := s.enc.wireRange(a.lines[e.File], lspRange(e.Line, e.Col, e.EndLine, e.EndCol))
			it.URI, it.Range = s.uriOf(e.File), &r
		}
		res.Results = append(res.Results, it)
	}
	return res, nil
}

// searchSnapshot is the analysis alder/search reads: the last build without errors, else (before
// there is one) the last good build, whose index is then the standard library's alone
// (searchIndex); nil before the first build.
func (s *server) searchSnapshot() *analysis {
	if a := s.lastClean(); a != nil {
		return a
	}
	return s.lastGood()
}

// holeQuery is the query text for the hole under a position: ": " + the hole's type, as
// types.String prints it (whole module paths, so the text resolves from any module; variables
// the checker left unresolved print as `_`). ok is false off a typed hole of the last good
// build, or when the hole's line has changed since.
func (s *server) holeQuery(rel string, p Position) (query string, ok bool) {
	cx := s.completionAt(rel, p)
	if cx == nil || cx.m == nil {
		return "", false
	}
	h := cx.holeAt()
	if h == nil {
		return "", false
	}
	t := cx.a.res.Types[h]
	if t == nil {
		return "", false
	}
	return ": " + types.String(cx.a.res.U.Apply(t)), true
}
