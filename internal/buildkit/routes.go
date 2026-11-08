package buildkit

import (
	"alder/internal/check"
	"alder/internal/ir"
	"alder/internal/names"
)

// Route is one endpoint of a checked app, for `alder routes` and the language server's
// alder/routes: a `route` declaration. Kind is "route" (older editor plugins read "page" or
// "msg"; Method tells a GET from a write). Method is the HTTP method it answers, Path its path
// as declared, Module and Name the declaring module and the route's name. File is the declaring
// module's file as the loader names it (relative to the project root, e.g.
// app/Posts/Show.alder), and Line, Col are the declaration's place, 1-based.
type Route struct {
	Kind, Method, Path, Module, Name, File string
	Line, Col                              int
	EndLine, EndCol                        int // the declaration's end (1-based, as the span gives it)
}

// Routes lists the routes of a checked program in program order (module order, then
// declaration order): exactly the set gen.routesSource builds the web layer's route table
// from. res must be the result of a clean check (no errors, no holes).
func Routes(prog *names.Program, res *check.Result) []Route {
	p := ir.Lower(prog, res)
	var out []Route
	for _, rt := range p.Routes {
		r := Route{Kind: "route", Method: rt.Method, Path: rt.Path, Module: rt.Module, Name: rt.Name}
		if m := prog.Modules[rt.Module]; m != nil {
			r.File = m.File
			r.Line, r.Col, r.EndLine, r.EndCol = 1, 1, 1, 1
			if d := prog.Route(rt.Module, rt.Name); d != nil {
				r.Line, r.Col, r.EndLine, r.EndCol = d.Span.Start.Line, d.Span.Start.Col, d.Span.End.Line, d.Span.End.Col
			}
		}
		out = append(out, r)
	}
	return out
}
