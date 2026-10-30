package names

import (
	"fmt"
	"sort"
	"strings"

	"alder/internal/diag"
	"alder/internal/syntax"
)

// Name clashes a module's name makes with a capability or a constructor in scope (alder#11).
//
// A qualifier is looked up before a capability (design rule 5), so a module whose qualifier is a
// capability's name hides that capability's entries: `Clarity.final` is looked up in module
// Clarity. The capability stays reachable through its own module's qualifier
// (`DataClarity.final`), so the program can work: a warning at the import (W0009), and a hint on
// the "has no" error the hidden call gets. A standard-library module, always in scope, hides a
// capability of its name the same way: a warning at the capability.
//
// A constructor is looked up before a page (design rule 6), and a single-segment page has no
// other spelling, so where a constructor named like it is in scope the page cannot be written at
// all: `Status { cfg: "pa" }` builds the constructor. That is an error at the constructor's
// declaration (E0040).

// capEntryNames is the entries of capability name declared in module path, in declaration order.
func (p *Program) capEntryNames(path, name string) []string {
	m := p.Modules[path]
	if m == nil {
		return nil
	}
	for _, d := range m.Decls {
		if c, ok := d.(*syntax.CapDecl); ok && c.Name == name {
			var out []string
			for _, e := range c.Entries {
				out = append(out, e.Name)
			}
			return out
		}
	}
	return nil
}

// capSpelling is how this module writes entry of capability cap of the imported module owner:
// through the owner's qualifier, and the capability's name unless it is the owner's eponymous one.
func (r *resolver) capSpelling(owner, cap, entry string) string {
	if cap == lastSegment(owner) {
		return r.qualFor(owner) + "." + entry
	}
	return r.qualFor(owner) + "." + cap + "." + entry
}

// hiddenCapOwners is the imported modules other than path declaring a capability named q, which
// the import of path, whose qualifier is q, hides.
func (r *resolver) hiddenCapOwners(q, path string) []string {
	var out []string
	for _, o := range r.importedCapOwners(q) {
		if o != path {
			out = append(out, o)
		}
	}
	return out
}

// shadowedCapabilities warns (W0009) at each import whose qualifier is the name of a capability
// another import declares, and at each capability of this module named like a standard-library
// module.
func (r *resolver) shadowedCapabilities() {
	for _, im := range r.m.Imports {
		q := r.ms.quals[im.Module]
		if q == "" || r.ms.imports[q] != im || r.p.isStd(im.Module) {
			continue
		}
		owners := r.hiddenCapOwners(q, im.Module)
		if len(owners) == 0 {
			continue
		}
		o := owners[0]
		named := fmt.Sprintf("module `%s`", im.Module)
		if q != im.Module {
			named = fmt.Sprintf("`%s` (module `%s`)", q, im.Module)
		}
		remedy := fmt.Sprintf("; rename module `%s` (for example to `%sModule`)", im.Module, im.Module)
		if es := r.p.capEntryNames(o, q); len(es) > 0 {
			remedy = fmt.Sprintf(": `%s.%s` is looked up in the module, not the capability; write `%s`, or rename module `%s` (for example to `%sModule`)",
				q, es[0], r.capSpelling(o, q, es[0]), im.Module, im.Module)
		}
		r.bag.Warn(diag.ShadowedCapability, im.Span, "%s shadows `capability %s` of `%s` in this module%s", named, q, o, remedy)
	}
	for _, d := range r.m.Decls {
		if c, ok := d.(*syntax.CapDecl); ok && r.p.isStd(c.Name) {
			example := ""
			if len(c.Entries) > 0 {
				example = fmt.Sprintf(": `%s.%s` names the standard library's `%s`, here and in every module importing `%s`", c.Name, c.Entries[0].Name, c.Name, r.m.Name)
			}
			r.bag.Warn(diag.ShadowedCapability, c.Span, "the standard-library module `%s` shadows `capability %s`%s; rename the capability (for example `%sCapability`)",
				c.Name, c.Name, example, c.Name)
		}
	}
}

// shadowedEntryHint explains `Clarity.final` that module Clarity (path, through qualifier q)
// lacks when an imported capability Clarity has the entry: the hint, and the spelling that
// reaches the entry; "" when there is none.
func (r *resolver) shadowedEntryHint(q, path, entry string) (hint, spelling string) {
	for _, o := range r.hiddenCapOwners(q, path) {
		for _, e := range r.p.capEntryNames(o, q) {
			if e == entry {
				r.ms.used[o] = true // the use meant it: no W0001 besides
				spelling = r.capSpelling(o, q, entry)
				return fmt.Sprintf("; `%s` here is module `%s`, which shadows `capability %s` of `%s`: write `%s`", q, path, q, o, spelling), spelling
			}
		}
	}
	return "", ""
}

// pageShadows reports each constructor named like a single-segment page module: in the
// constructor's module and the modules importing it (everywhere, for a Prelude constructor) the
// page cannot be written. A page with a dot in its name is written with its full path, which no
// constructor can be. It is an error (E0040) when some module in that scope writes `Name { … }`
// and the constructor takes no record, so that the page value written there became a
// constructor that cannot take it (the use was already a type error); otherwise the program
// works and it is a warning (W0010). It runs after every body is resolved (pageUses).
func (p *Program) pageShadows(bag *diag.Bag) {
	var pages []string
	for name := range p.Pages {
		if !strings.Contains(name, ".") {
			pages = append(pages, name)
		}
	}
	sort.Strings(pages)
	report := func(misused bool, span diag.Span, format string, args ...any) {
		if misused {
			bag.Error(diag.PageShadowed, span, format, args...)
		} else {
			bag.Warn(diag.PageShadowedWarn, span, format, args...)
		}
	}
	for _, page := range pages {
		rename := page + "Page"
		if prel := p.Std["Prelude"]; prel != nil {
			if typ, arg, ok := ctorDecl(prel, page); ok {
				report(p.pageUses["Prelude."+page] && !takesRecord(arg), p.Modules[page].Span,
					"the Prelude constructor `%s` (of `%s`) shadows page `%s`: `%s { … }` is the constructor in every module, so the page cannot be linked or fetched; rename the page module (for example `module %s`)",
					page, typ, page, page, rename)
				continue
			}
		}
		for _, x := range p.Order {
			for _, d := range p.Modules[x].Decls {
				what, where, noun := "", x+"` and the modules importing it", "constructor"
				var span diag.Span
				record := false
				switch d := d.(type) {
				case *syntax.TypeDecl:
					if d.Brand != nil && d.Name == page {
						what, span = fmt.Sprintf("brand `%s`", page), d.Span
					}
					for _, c := range d.Union {
						if c.Name == page {
							what, span, record = fmt.Sprintf("constructor `%s` (of `%s.%s`)", page, x, d.Name), c.Span, takesRecord(c.Arg)
						}
					}
				case *syntax.MsgDecl:
					if d.Name == page {
						what, span, where, noun, record = fmt.Sprintf("message `%s`", page), d.Span, x+"`", "message", d.Params != nil
					}
				}
				if what != "" {
					report(p.pageUses[x+"."+page] && !record, span,
						"%s shadows page `%s` in `%s: there `%s { … }` is the %s, so the page cannot be linked or fetched; rename the %s or the page module (for example `module %s`)",
						what, page, where, page, noun, noun, rename)
				}
			}
		}
	}
}

// ctorDecl finds the union constructor name in module m: its type's name and its payload.
func ctorDecl(m *syntax.Module, name string) (typ string, arg syntax.Type, ok bool) {
	for _, d := range m.Decls {
		if t, isType := d.(*syntax.TypeDecl); isType {
			for _, c := range t.Union {
				if c.Name == name {
					return t.Name, c.Arg, true
				}
			}
		}
	}
	return "", nil, false
}

// notRecords are the Prelude types a record literal never has.
var notRecords = map[string]bool{"Int": true, "Float": true, "Bool": true, "Text": true, "Unit": true, "Date": true, "Time": true, "List": true, "Maybe": true, "Result": true, "Dict": true, "Html": true, "Never": true, "Query": true, "Task": true, "Nav": true, "Error": true, "Id": true, "File": true, "Bytes": true}

// takesRecord reports whether a constructor with payload arg may be applied to a record literal:
// a record type, a type variable or a type that may be a record; not no payload, a function or
// a Prelude type that is not a record.
func takesRecord(arg syntax.Type) bool {
	switch a := arg.(type) {
	case nil, *syntax.FunType:
		return false
	case *syntax.TypeName:
		return a.Module != "" || !notRecords[a.Name]
	}
	return true
}

// pageUse records `Name { … }`, ct applied to a record literal, when ct resolved to a constructor
// named like a single-segment page module: pageShadows tells by it whether the clash broke a use.
func (r *resolver) pageUse(ct *syntax.CtorRef) {
	if ct.Module != "" || ct.Ref.Kind != syntax.RefCtor || !r.p.IsPageModule(ct.Name) {
		return
	}
	if r.p.pageUses == nil {
		r.p.pageUses = map[string]bool{}
	}
	r.p.pageUses[ct.Ref.Module+"."+ct.Name] = true
}
