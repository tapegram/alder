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
// A route named like a constructor in scope cannot be told apart from it: E0044 (routeClashes).

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

// routeClashes reports (E0044) each route of this module whose name cannot be told apart: a
// second route of a name; a module named by this module's path and the route (`Todos.Row`
// beside route Row in `Todos`); an import's qualifier or a standard-library module, which a
// dotted name reaches first; a constructor, brand or table in scope, which `Row { … }` would
// also be. One error per route, the first that applies.
func (r *resolver) routeClashes() {
	seen := map[string]int{}
	for _, d := range r.m.Decls {
		rd, ok := d.(*syntax.RouteDecl)
		if !ok {
			continue
		}
		name, sp := rd.Name, rd.NameSpan
		if line, dup := seen[name]; dup {
			r.bag.Error(diag.RouteName, sp, "route `%s` is already declared at line %d", name, line)
			continue
		}
		seen[name] = rd.Span.Start.Line
		if mod := r.m.Name + "." + name; r.p.Modules[mod] != nil {
			r.bag.Error(diag.RouteName, sp, "route `%s` of `%s` and the module `%s` have one name: `%s.view` could name either; rename the route or the module", name, r.m.Name, mod, mod)
			continue
		}
		member := r.firstMember(name)
		if r.p.isStd(name) {
			r.bag.Error(diag.RouteName, sp, "route `%s` is named like the standard-library module `%s`: `%s.%s` would name that module's `%s`; rename the route", name, name, name, member, member)
			continue
		}
		if im, ok := r.ms.imports[name]; ok {
			r.bag.Error(diag.RouteName, sp, "route `%s` is named like the qualifier of `import %s`: `%s.%s` would name that module's `%s`; rename the route", name, im.Module, name, member, member)
			continue
		}
		if what := r.ctorInScope(name); what != "" {
			r.bag.Error(diag.RouteName, sp, "route `%s` is named like %s: `%s { … }` cannot mean both; rename the route", name, what, name)
		}
	}
}

// firstMember is the name of the first declaration of route `route` in this module (`load` for
// `Row.load`), or "view" when it has none: an example for a message.
func (r *resolver) firstMember(route string) string {
	for _, d := range r.m.Decls {
		if v, ok := d.(*syntax.ValueDecl); ok && v.Route == route {
			return v.Name
		}
	}
	return "view"
}

// ctorInScope describes the constructor, brand or table written `name` that this module sees
// unqualified ("the constructor `Active` (of `Todos.Filter`)"), or "" when there is none.
func (r *resolver) ctorInScope(name string) string {
	switch {
	case r.ms.ctors[name] != "":
		return fmt.Sprintf("the constructor `%s` (of `%s.%s`)", name, r.m.Name, r.ms.ctors[name])
	case r.ms.brands[name]:
		return fmt.Sprintf("the brand `%s` (of `%s`)", name, r.m.Name)
	case r.ms.tables[name]:
		return fmt.Sprintf("the table `%s` (of `%s`)", name, r.m.Name)
	}
	if prel := r.p.scopes["Prelude"]; prel != nil && prel.ctors[name] != "" {
		return fmt.Sprintf("the Prelude constructor `%s` (of `%s`)", name, prel.ctors[name])
	}
	for _, x := range r.importNames() {
		s := r.p.scopes[x]
		switch {
		case s == nil:
		case s.ctors[name] != "":
			return fmt.Sprintf("the constructor `%s` (of `%s.%s`)", name, x, s.ctors[name])
		case s.brands[name]:
			return fmt.Sprintf("the brand `%s` (of `%s`)", name, x)
		}
	}
	return ""
}

// noSuchRoute reports (E0044) declaration d, written `Route.name`, whose route this module does
// not declare, with the nearest route's name when one is close.
func (r *resolver) noSuchRoute(d *syntax.ValueDecl) {
	msg := fmt.Sprintf("module `%s` has no route `%s`", r.m.Name, d.Route)
	best, bestD := "", 3
	var names []string
	for n := range r.ms.routes {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, n := range names {
		if dist := editDistance(strings.ToLower(d.Route), strings.ToLower(n)); dist < bestD {
			best, bestD = n, dist
		}
	}
	if best != "" {
		msg += fmt.Sprintf("; did you mean `%s`?", best)
	}
	r.bag.Error(diag.RouteName, d.RouteSpan, "%s", msg)
}

// editDistance is the Levenshtein distance between a and b.
func editDistance(a, b string) int {
	prev, cur := make([]int, len(b)+1), make([]int, len(b)+1)
	for j := range prev {
		prev[j] = j
	}
	for i := 1; i <= len(a); i++ {
		cur[0] = i
		for j := 1; j <= len(b); j++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}
			cur[j] = min(prev[j]+1, cur[j-1]+1, prev[j-1]+cost)
		}
		prev, cur = cur, prev
	}
	return prev[len(b)]
}
