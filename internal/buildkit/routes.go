package buildkit

import (
	"sort"

	"alder/internal/check"
	"alder/internal/ir"
	"alder/internal/names"
	"alder/internal/syntax"
)

// Route is one endpoint of a checked app, for `alder routes` and the language server's
// alder/routes. Kind is "page" (GET Path) or "msg" (POST Path). A page's Name is "page"; a
// message's is its declaration's. Path is the page's path as declared, or for a message the URL
// it is posted to (/_m/<Module>/<Msg>, as gen/routes.go routes it). File is the declaring module's
// file as the loader names it (relative to the project root, e.g. app/Posts/Show.alder),
// and Line, Col are the declaration's place, 1-based.
type Route struct {
	Kind, Path, Module, Name, File string
	Line, Col                      int
	EndLine, EndCol                int // the declaration's end (1-based, as the span gives it)
}

// Routes lists the routes of a checked program: the pages (GET), then the messages of the pages
// that have an update and of App (POST), exactly the set gen.routesSource builds the web
// layer's route table from, sorted by path (pages before messages at a path, then module and
// name). res must be the result of a clean check (no errors, no holes).
func Routes(prog *names.Program, res *check.Result) []Route {
	p := ir.Lower(prog, res)
	var out []Route
	add := func(kind, path, module, name string) {
		r := Route{Kind: kind, Path: path, Module: module, Name: name}
		if m := prog.Modules[module]; m != nil {
			r.File = m.File
			r.Line, r.Col, r.EndLine, r.EndCol = 1, 1, 1, 1
			for _, d := range m.Decls {
				switch d := d.(type) {
				case *syntax.PageDecl:
					if kind == "page" {
						r.Line, r.Col, r.EndLine, r.EndCol = d.Span.Start.Line, d.Span.Start.Col, d.Span.End.Line, d.Span.End.Col
					}
				case *syntax.MsgDecl:
					if kind == "msg" && d.Name == name {
						r.Line, r.Col, r.EndLine, r.EndCol = d.Span.Start.Line, d.Span.Start.Col, d.Span.End.Line, d.Span.End.Col
					}
				}
			}
		}
		out = append(out, r)
	}
	for _, pg := range p.Pages {
		add("page", pg.Path, pg.Module, "page")
		if pg.Update != nil {
			for _, m := range pg.Msgs {
				add("msg", "/_m/"+pg.Module+"/"+m.Name, pg.Module, m.Name)
			}
		}
	}
	if p.AppUpdate != nil {
		for _, m := range p.AppMsgs {
			add("msg", "/_m/App/"+m.Name, "App", m.Name)
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if a.Path != b.Path {
			return a.Path < b.Path
		}
		if a.Kind != b.Kind {
			return a.Kind == "page"
		}
		if a.Module != b.Module {
			return a.Module < b.Module
		}
		return a.Name < b.Name
	})
	return out
}
