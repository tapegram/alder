package check

import (
	"fmt"
	"strings"

	"alder/internal/diag"
	"alder/internal/syntax"
)

// The page-era syntax (#160, PR 9) is gone: `page`, `msg`, `Nav` and its constructors, `Page`,
// `Html` with a type argument and the unqualified `load`/`view`/`update` of a page module are
// E0046, each with the hint to run the migration.

// removedHint ends every E0046 message.
const removedHint = diag.RemovedHint

// pageMembers are the declarations of a page module that a route writes qualified (`Index.load`).
var pageMembers = map[string]bool{"load": true, "view": true, "update": true}

// removedDecl is an E0046 to report once the checker is done (reportRemoved).
type removedDecl struct {
	span diag.Span
	msg  string
}

// rejectRemoved finds the app modules that declare a `page` or a `msg`. Each such declaration is
// E0046 (reported by reportRemoved, after quietErrors); the rest of the module is old syntax
// through and through, so its other errors (names' too, already in bag) are dropped, its bodies
// are not checked, and its files join quiet. In the other modules an unqualified `load`, `view`
// or `update` is E0046 at once.
func (c *Checker) rejectRemoved(quiet map[string]bool) []removedDecl {
	var out []removedDecl
	rejected := map[string]bool{}
	for _, name := range c.prog.Order {
		m := c.prog.Modules[name]
		var found []removedDecl
		for _, d := range m.Decls {
			switch d := d.(type) {
			case *syntax.PageDecl:
				found = append(found, removedDecl{d.Span, "`page` declarations are gone: a page is a GET route now (`route Name = GET \"/path\" { … }`); " + removedHint})
			case *syntax.MsgDecl:
				found = append(found, removedDecl{d.Span, "`msg` declarations are gone: a message is a write route now (`route " + d.Name + " = POST \"/path\" { … }`); " + removedHint})
			}
		}
		if len(found) == 0 {
			if c.hasError(ReservedName, m.File) { // the leftover is a cascade of that module's E0012
				continue
			}
			for _, d := range m.Decls {
				if vd, ok := d.(*syntax.ValueDecl); ok && vd.Route == "" && pageMembers[vd.Name] {
					cands := routeCandidates(m, vd.Name)
					if len(cands) == 0 {
						if vd.Name == legacyHandler {
							c.errorf(diag.RemovedSyntax, vd.Span, "an unqualified `update` is a page's; a write route's handler is named after its method and written qualified (`Name.post` for `route Name = POST`); %s", removedHint)
							continue
						}
						c.errorf(diag.RemovedSyntax, vd.Span, "an unqualified `%s` is a page's; a route's is written `Name.%s` for its `route Name`; %s", vd.Name, vd.Name, removedHint)
						continue
					}
					// the route's own E0043 says the same thing: one error per member (#185)
					if c.unqualified == nil {
						c.unqualified = map[string]map[string]bool{}
					}
					if c.unqualified[m.Name] == nil {
						c.unqualified[m.Name] = map[string]bool{}
					}
					c.unqualified[m.Name][vd.Name] = true
					c.errorf(diag.RemovedSyntax, vd.Span, "%s", qualifyMessage(m.Name, vd.Name, cands))
				}
			}
			continue
		}
		out = append(out, found...)
		rejected[m.File] = true
		quiet[m.File] = true
		if c.prog.Failed == nil {
			c.prog.Failed = map[string]bool{}
		}
		c.prog.Failed[name] = true
	}
	if len(rejected) > 0 { // what names reported about these modules is a cascade of the old syntax
		keep := c.bag.Items[:0]
		for _, d := range c.bag.Items {
			if d.Severity != "error" || !rejected[d.Span.File] {
				keep = append(keep, d)
			}
		}
		c.bag.Items = keep
	}
	return out
}

// routeCandidate is a route that needs an unqualified member, and the member's name for it: the
// member itself, or for `update` the route's handler (`post` for a POST route).
type routeCandidate struct{ route, member string }

// routeCandidates are the routes of m that need the member (`update`: write routes, which need
// a handler; `load` and `view`: GET routes) and do not declare it, in declaration order. A write
// route has its handler when it declares it under its method's name or, for one release,
// `update`.
func routeCandidates(m *syntax.Module, member string) []routeCandidate {
	have := map[string]bool{}
	methods := map[string]string{}
	for _, d := range m.Decls {
		if r, ok := d.(*syntax.RouteDecl); ok {
			methods[r.Name] = r.Method
		}
	}
	for _, d := range m.Decls {
		vd, ok := d.(*syntax.ValueDecl)
		if !ok || vd.Route == "" {
			continue
		}
		if vd.Name == member || member == legacyHandler && isHandlerName(vd.Name, methods[vd.Route]) {
			have[vd.Route] = true
		}
	}
	var out []routeCandidate
	for _, d := range m.Decls {
		if r, ok := d.(*syntax.RouteDecl); ok && isWrite(r.Method) == (member == legacyHandler) && !have[r.Name] {
			name := member
			if member == legacyHandler {
				name = HandlerName(r.Method)
			}
			out = append(out, routeCandidate{r.Name, name})
		}
	}
	return out
}

// qualifyMessage is the E0046 for an unqualified member in a module with routes that lack it.
func qualifyMessage(module, member string, cands []routeCandidate) string {
	kind := "GET"
	if member == legacyHandler {
		kind = "write"
	}
	if len(cands) == 1 {
		return fmt.Sprintf("`%s` must be qualified with the route it belongs to: `%s.%s` (route `%s` is the only %s route in `%s`); %s", member, cands[0].route, cands[0].member, cands[0].route, kind, module, removedHint)
	}
	qs := make([]string, len(cands))
	for i, r := range cands {
		qs[i] = "`" + r.route + "." + r.member + "`"
	}
	return fmt.Sprintf("`%s` must be qualified with the route it belongs to: one of %s (the %s routes in `%s`); %s", member, strings.Join(qs, ", "), kind, module, removedHint)
}

// reportRemoved reports the `page` and `msg` declarations rejectRemoved found.
func (c *Checker) reportRemoved(ds []removedDecl) {
	for _, d := range ds {
		c.errorf(diag.RemovedSyntax, d.span, "%s", d.msg)
	}
}

// hasError reports whether the bag already holds an error of code in file.
func (c *Checker) hasError(code, file string) bool {
	for _, d := range c.bag.Items {
		if d.Severity == "error" && d.Code == code && d.Span.File == file {
			return true
		}
	}
	return false
}
