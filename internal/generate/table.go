package generate

import (
	"strings"

	"alder/internal/diag"
	"alder/internal/names"
	"alder/internal/syntax"
)

// TableData is the data of table.tmpl: the table's name (its module's), its imports, and its
// fields as declared (`title: Text.NonEmpty`, `post: Post.PostId @ref Post @cascade`), between
// the id and the `@created`/`@updated` pair the template writes.
type TableData struct {
	Name    string
	Imports []string
	Fields  []string
}

// NewTable generates `alder new table`: app/<Name>.alder with the table, its id brand, the
// fields given and `created`/`updated`. The migration is the caller's (Options.Migrate). It
// refuses a name that is not a table name or whose module exists, a field given twice, and the
// fields the generator writes itself.
func NewTable(p *Program, name string, fields []FieldSpec) ([]Edit, error) {
	if !IsUpperIdent(name) {
		return nil, Refusef("`%s` is not a table name: a table is named by one capitalised identifier, such as `Tag`, and declared in the module of that name", name)
	}
	if m := p.Modules[name]; m != nil {
		return nil, Refusef("module `%s` exists (`%s`): a generator never overwrites a file; choose another table name", name, m.File)
	}
	imports := newImports(name, p.Std)
	d := TableData{Name: name}
	seen := map[string]bool{}
	for _, f := range fields {
		switch {
		case f.Name == "id" || f.Has("id"):
			return nil, Refusef("field `%s`: the generator writes the id, `id: %sId @id`; leave it out", f.Name, name)
		case f.Name == "created" || f.Name == "updated" || f.Has("created") || f.Has("updated"):
			return nil, Refusef("field `%s`: the generator writes `created: Time @created` and `updated: Time @updated`; leave them out", f.Name)
		case seen[f.Name]:
			return nil, Refusef("field `%s` is given twice", f.Name)
		}
		seen[f.Name] = true
		typeImports(imports, f.Type, p)
		for _, a := range f.Attrs {
			if a.Name == "ref" {
				imports.add(a.Arg)
			}
		}
		d.Fields = append(d.Fields, f.Source())
	}
	d.Imports = imports.list()
	return []Edit{{Path: FileOf(name), Create: true, Body: render("table.tmpl", d)}}, nil
}

// typeImports imports the modules a type written as text names (`Post.PostId` → Post; `Maybe
// User.Role` → User); the standard library's (`Text.NonEmpty`) need no import.
func typeImports(s *importSet, typ string, p *Program) {
	for _, w := range strings.FieldsFunc(typ, func(r rune) bool { return r == ' ' || r == '(' || r == ')' }) {
		if i := strings.LastIndex(w, "."); i > 0 {
			s.add(w[:i])
		}
	}
}

// TableErrors are the errors of diags that lie in a table declaration of prog: a table that does
// not check cannot be migrated.
func TableErrors(prog *names.Program, diags []diag.Diagnostic) []diag.Diagnostic {
	var out []diag.Diagnostic
	for _, d := range diags {
		if d.Severity != "error" {
			continue
		}
		for _, m := range prog.Modules {
			if m.File != d.Span.File {
				continue
			}
			for _, decl := range m.Decls {
				if td, ok := decl.(*syntax.TableDecl); ok && contains(td.Span, d.Span.Start) {
					out = append(out, d)
				}
			}
		}
	}
	return out
}
