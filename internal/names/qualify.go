// internal/names/qualify.go
package names

import (
	"sort"
	"strings"

	"alder/internal/syntax"
)

// CanonicalQualifiers maps each import path to its canonical qualifier (design rule 3).
// self is the importing module's name; taken reports names declared in it or reserved
// (stdlib modules, App). Unqualifiable paths map to "" and are listed in bad, sorted.
func CanonicalQualifiers(self string, paths []string, taken func(string) bool) (quals map[string]string, bad []string) {
	selfLast := lastSegment(self)
	segs := map[string][]string{}
	k := map[string]int{}
	var live []string // unique paths not yet found unqualifiable
	for _, p := range paths {
		if _, dup := segs[p]; dup {
			continue
		}
		segs[p] = strings.Split(p, ".")
		k[p] = 1
		live = append(live, p)
	}
	sort.Strings(live)
	cand := func(p string) string {
		s := segs[p]
		return strings.Join(s[len(s)-k[p]:], "")
	}
	isBad := map[string]bool{}
	// Synchronous rounds, so the outcome does not depend on input order: bump every
	// path that collides or is taken and can still grow; one that cannot grow is
	// unqualifiable if it shares its name with another such path or the name is taken.
	for changed := true; changed; {
		changed = false
		groups := map[string][]string{}
		for _, p := range live {
			if !isBad[p] {
				groups[cand(p)] = append(groups[cand(p)], p)
			}
		}
		for c, g := range groups {
			blocked := taken(c) || c == selfLast
			if len(g) == 1 && !blocked {
				continue
			}
			canGrow := false
			for _, p := range g {
				if k[p] < len(segs[p]) {
					canGrow = true
				}
			}
			for _, p := range g {
				switch {
				case k[p] < len(segs[p]):
					k[p]++
					changed = true
				case blocked || !canGrow:
					// Cannot grow: a taken name, or a tie only with other whole joins.
					if !isBad[p] {
						isBad[p] = true
						changed = true
					}
				}
			}
		}
	}
	quals = map[string]string{}
	for _, p := range live {
		if isBad[p] {
			quals[p] = ""
			bad = append(bad, p)
		} else {
			quals[p] = cand(p)
		}
	}
	sort.Strings(bad)
	return quals, bad
}

// NeedsAlias reports whether path's import must be written with `as q`.
func NeedsAlias(path, q string) bool { return q != lastSegment(path) }

// Taken returns the "taken" predicate of design rule 3 for module m: App, any name isStd
// accepts (stdlib modules), and every type, table, capability or fake table declared in m.
// The module's own last segment is handled by CanonicalQualifiers through its self parameter.
func Taken(m *syntax.Module, isStd func(string) bool) func(string) bool {
	decl := map[string]bool{"App": true}
	for _, d := range m.Decls {
		switch d := d.(type) {
		case *syntax.TypeDecl:
			decl[d.Name] = true
		case *syntax.TableDecl:
			decl[d.Name] = true
		case *syntax.CapDecl:
			decl[d.Name] = true
		case *syntax.FakeDecl:
			decl[d.Name] = true
			for _, t := range d.Tables {
				decl[t.Name] = true
			}
		}
	}
	return func(n string) bool { return decl[n] || isStd(n) }
}

func lastSegment(path string) string {
	return path[strings.LastIndex(path, ".")+1:]
}

// Qualifiers maps each import path of module to its canonical qualifier ("" for an import with
// no usable qualifier, E0206); nil when there is no such module. A stdlib module imported keeps
// its name. The editor (completion, hover) and `alder fmt` write references with it.
func (p *Program) Qualifiers(module string) map[string]string {
	m := p.Modules[module]
	if m == nil {
		return nil
	}
	return p.qualifiers(m)
}

// qualifiers computes module m's import qualifiers (Qualifiers).
func (p *Program) qualifiers(m *syntax.Module) map[string]string {
	var paths []string
	for _, im := range m.Imports {
		if !p.isStd(im.Module) {
			paths = append(paths, im.Module)
		}
	}
	quals, _ := CanonicalQualifiers(m.Name, paths, Taken(m, p.isStd))
	for _, im := range m.Imports {
		if p.isStd(im.Module) {
			quals[im.Module] = im.Module
		}
	}
	return quals
}

// isStd reports whether name is a standard-library module.
func (p *Program) isStd(name string) bool { _, ok := p.Std[name]; return ok }
