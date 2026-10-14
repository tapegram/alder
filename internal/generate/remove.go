package generate

import (
	"strings"

	"alder/internal/buildkit"
	"alder/internal/syntax"
)

// RemovePage generates `alder remove page <Module>`: the page module's file removed. It refuses
// a module that is not a page, and a page something else still names (a link, a Redirect, a
// test, `login = …`): the check of the app without it lists those uses, and nothing is removed
// until they are gone.
func RemovePage(p *Program, module string) ([]Edit, error) {
	pg, m, err := pageModule(p, module)
	if err != nil {
		return nil, err
	}
	edits := []Edit{{Path: pg.File, Delete: true}}
	return edits, stillNamed(p, edits, "`"+module+"`", m.File)
}

// RemoveTable generates `alder remove table <T>`: the table's declaration removed with its id
// brand (`type TId = brand Text`) and the module's tests, or, when the module holds nothing
// else, the module's file; the migration that drops the table (its DROPS DATA line: the rows
// are lost) is the caller's (Options.Migrate). It refuses a name that is not a table of the app,
// and a table something else still names (a reference `@ref T`, a page, a query, a test, the
// admin's pages: remove those first — for the admin, `alder remove page` its pages of the table,
// then `alder regen admin`): the check of the app without it lists those uses.
func RemoveTable(p *Program, name string) ([]Edit, error) {
	t := p.TableNamed(name)
	if t == nil {
		return nil, Refusef("`%s` is not a table of this app (its tables: %s)", name, tableNames(p))
	}
	m := p.Modules[t.Module]
	gone := func(d syntax.Decl) bool {
		switch d := d.(type) {
		case *syntax.TableDecl:
			return d.Name == name
		case *syntax.TypeDecl:
			return d.Name == name+"Id"
		case *syntax.TestDecl:
			return true
		}
		return false
	}
	rest := false
	for _, d := range m.Syntax.Decls {
		if !gone(d) {
			rest = true
		}
	}
	var edit Edit
	if !rest {
		edit = Edit{Path: m.File, Delete: true}
	} else {
		e := newSrcEdit(m.Source)
		for _, d := range m.Syntax.Decls {
			if !gone(d) {
				continue
			}
			end := d.GetSpan().End.Line
			for n := declStart(e, d); n <= end; n++ {
				e.deleteLine(n)
			}
			if end+1 <= len(e.lines) && strings.TrimSpace(e.line(end+1)) == "" {
				e.deleteLine(end + 1) // the blank line after it
			}
		}
		edit = Edit{Path: m.File, Body: e.result()}
	}
	edits := []Edit{edit}
	return edits, stillNamed(p, edits, "table `"+name+"`", m.File)
}

// stillNamed refuses edits (a removal) when the app without what they remove does not check:
// what (`Posts.Show`, table `Tag`) is still named elsewhere, and the errors say where. Errors
// in the file being removed itself (own) are not uses.
func stillNamed(p *Program, edits []Edit, what, own string) error {
	probe, err := Probe(p.Dir, edits)
	if err != nil {
		return err
	}
	r := &Refusal{Sources: buildkit.Sources(probe.Prog)}
	for _, d := range probe.Diags {
		if d.Severity == "error" && (d.Span.File != own || !edits[0].Delete) {
			r.Diags = append(r.Diags, d)
		}
	}
	if len(r.Diags) == 0 {
		return nil
	}
	r.Msg = what + " is still used (the errors above): remove those uses first; nothing was removed"
	return r
}
