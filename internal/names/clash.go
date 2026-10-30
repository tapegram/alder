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
// (`DataClarity.final`), so the program can work. A call the module lacks is E0002 with a hint
// and the fix; a call the module has resolves to the module's value without a word, which is
// the warning W0010 at the import. A standard-library module, always in scope, hides a
// capability of its name the same way: W0010 at the capability, and the same E0002 hint.
//
// A constructor is looked up before a page (design rule 6), and a single-segment page has no
// other spelling, so where a constructor named like it is in scope the page cannot be written at
// all: `Status { cfg: "pa" }` builds the constructor. That is E0041 at the constructor's
// declaration when a use was broken by it, else the warning W0011 (pageShadows).

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

// hiddenEntryOwner is the first imported module other than path whose capability q, hidden by
// the module path that q names here, has entry; "" when there is none.
func (r *resolver) hiddenEntryOwner(q, path, entry string) string {
	for _, o := range r.importedCapOwners(q) {
		if o == path {
			continue
		}
		for _, e := range r.p.capEntryNames(o, q) {
			if e == entry {
				return o
			}
		}
	}
	return ""
}

// stdShadowedCapabilities warns (W0010) at each capability of this module named like a
// standard-library module: `Http.fetch` names the standard library's, here and in every module
// importing this one.
func (r *resolver) stdShadowedCapabilities() {
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

// shadowedValue notes `Clarity.latest` that resolved to module Clarity's value (path, through
// qualifier q) while an imported capability Clarity, which the module hides, has the entry
// `latest`: the first such use of each import is warned about at the import (shadowWarnings).
func (r *resolver) shadowedValue(q, path, entry string) {
	if r.p.isStd(path) || r.hiddenEntryOwner(q, path, entry) == "" {
		return
	}
	if r.shadowUses == nil {
		r.shadowUses = map[string]string{}
	}
	if _, seen := r.shadowUses[path]; !seen {
		r.shadowUses[path] = entry
	}
}

// shadowWarnings reports W0010 at each import a hidden capability's entry was called through.
func (r *resolver) shadowWarnings() {
	for _, im := range r.m.Imports {
		entry, ok := r.shadowUses[im.Module]
		q := r.ms.quals[im.Module]
		if !ok || q == "" || r.ms.imports[q] != im {
			continue
		}
		o := r.hiddenEntryOwner(q, im.Module, entry)
		named := fmt.Sprintf("module `%s`", im.Module)
		if q != im.Module {
			named = fmt.Sprintf("`%s` (module `%s`)", q, im.Module)
		}
		r.bag.Warn(diag.ShadowedCapability, im.Span, "%s shadows `capability %s` of `%s` in this module: `%s.%s` here is the module's `%s`, not the capability's entry; write `%s` for the capability, or rename module `%s` (for example to `%sModule`)",
			named, q, o, q, entry, entry, r.capSpelling(o, q, entry), im.Module, im.Module)
	}
}

// shadowedEntryHint explains `Clarity.final` that module Clarity (path, through qualifier q)
// lacks when a capability Clarity it hides has the entry: the hint, and the spelling that
// reaches the entry ("" when none does: the module's own capability hidden by a stdlib module).
func (r *resolver) shadowedEntryHint(q, path, entry string) (hint, spelling string) {
	if o := r.hiddenEntryOwner(q, path, entry); o != "" {
		r.ms.used[o] = true // the use meant it: no W0001 besides
		spelling = r.capSpelling(o, q, entry)
		mod := fmt.Sprintf("module `%s`", path)
		if r.p.isStd(path) {
			mod = "the standard-library module"
		}
		return fmt.Sprintf("; `%s` here is %s, which shadows `capability %s` of `%s`: write `%s`", q, mod, q, o, spelling), spelling
	}
	if r.p.isStd(path) && r.ms.caps[q] {
		for _, e := range r.p.capEntryNames(r.m.Name, q) {
			if e == entry {
				return fmt.Sprintf("; `%s` here is the standard-library module, which shadows `capability %s` of this module: rename the capability (for example `%sCapability`)", q, q, q), ""
			}
		}
	}
	return "", ""
}

// pageShadows reports each constructor named like a single-segment page module: in the
// constructor's module and the modules importing it (everywhere, for a Prelude constructor) the
// page cannot be written. A page with a dot in its name is written with its full path, which no
// constructor can be. It is an error (E0041) when some module in that scope writes `Name { … }`
// and the constructor takes no record (never a `msg`, which always takes one), so that the page value written there became a
// constructor that cannot take it (the use was already a type error); otherwise the program
// works and it is a warning (W0011). It runs after every body is resolved (pageUses).
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
						// a message always takes a record (its parameters, `{}`, or `{ form: … }`),
						// so `Go {}` for it type-checks: a message clash is only ever W0011
						what, span, where, noun, record = fmt.Sprintf("message `%s`", page), d.Span, x+"`", "message", true
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
