package lsp

import (
	"encoding/json"

	"alder/internal/buildkit"
)

// routeItem is one route of alder/routes' list: the same fields as `alder routes --json`, with
// the declaration's place as a file URI and an LSP range (0-based) in place of file, line, col.
type routeItem struct {
	Kind   string `json:"kind"`
	Path   string `json:"path"`
	Module string `json:"module"`
	Name   string `json:"name"`
	URI    string `json:"uri"`
	Range  Range  `json:"range"`
}

// routesReq answers alder/routes (params {}): every page (GET) and message (POST) of the last
// build that type-checked, sorted by path. The list is empty before the first good build, and
// while the program has errors the last good build's routes stand (their places may have
// moved: a stale list is better than an empty picker).
func routesReq(s *server, _ json.RawMessage) (out any, err error) {
	items := []routeItem{}
	defer func() {
		if r := recover(); r != nil { // a program with typed holes may not lower
			out, err = items, nil
		}
	}()
	a := s.lastGood()
	if a == nil || a.res == nil || a.res.Holes > 0 {
		return items, nil
	}
	for _, r := range buildkit.Routes(a.prog, a.res) {
		items = append(items, routeItem{
			Kind: r.Kind, Path: r.Path, Module: r.Module, Name: r.Name,
			URI:   s.uriOf(r.File),
			Range: lspRange(r.Line, r.Col, r.EndLine, r.EndCol),
		})
	}
	return items, nil
}
