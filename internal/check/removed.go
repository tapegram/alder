package check

import (
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
			for _, d := range m.Decls {
				if vd, ok := d.(*syntax.ValueDecl); ok && vd.Route == "" && pageMembers[vd.Name] {
					c.errorf(diag.RemovedSyntax, vd.Span, "an unqualified `%s` is a page's; a route's is written `Name.%s` for its `route Name`; %s", vd.Name, vd.Name, removedHint)
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

// reportRemoved reports the `page` and `msg` declarations rejectRemoved found.
func (c *Checker) reportRemoved(ds []removedDecl) {
	for _, d := range ds {
		c.errorf(diag.RemovedSyntax, d.span, "%s", d.msg)
	}
}
