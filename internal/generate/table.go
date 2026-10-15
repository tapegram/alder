package generate

import (
	"fmt"
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
	Tests   []TestData // one per reference with a delete rule (`@cascade`, `@restrict`)
	// Timestamps: the template writes `created: Time @created` and `updated: Time @updated`
	// (the default; --no-timestamps leaves them out, and a field may then be @created itself).
	Timestamps bool
}

// TableOptions are `alder new table`'s options.
type TableOptions struct {
	NoTimestamps bool // --no-timestamps: no `created`/`updated` pair
}

// NewTable generates `alder new table`: app/<Name>.alder with the table, its id brand, the
// fields given and `created`/`updated` (NewTableWith: without them), and a test of each reference's delete rule. The migration is the caller's (Options.Migrate). It
// refuses a name that is not a table name or whose module exists, a field given twice, and the
// fields the generator writes itself.
func NewTable(p *Program, name string, fields []FieldSpec) ([]Edit, error) {
	return NewTableWith(p, name, fields, TableOptions{})
}

// NewTableWith is NewTable with options: with NoTimestamps the table has no `created` and
// `updated` columns, unless the fields given declare them (`created:Time@created`).
func NewTableWith(p *Program, name string, fields []FieldSpec, o TableOptions) ([]Edit, error) {
	if !IsUpperIdent(name) {
		return nil, Refusef("`%s` is not a table name: a table is named by one capitalised identifier, such as `Tag`, and declared in the module of that name", name)
	}
	m := p.Modules[name]
	if m != nil { // a module of types only (`alder new type User "Role = …"`) takes its table
		for _, d := range m.Syntax.Decls {
			if _, ok := d.(*syntax.TableDecl); ok {
				return nil, Refusef("module `%s` exists (`%s`) and declares a table: a generator never overwrites a file; choose another table name", name, m.File)
			}
		}
		for _, n := range []string{name, name + "Id"} {
			if declared(m.Syntax, n) {
				return nil, Refusef("module `%s` already declares `%s`: the table needs the name", name, n)
			}
		}
	}
	imports := newImports(name, p.Std)
	d := TableData{Name: name, Timestamps: !o.NoTimestamps}
	seen := map[string]bool{}
	for _, f := range fields {
		switch {
		case f.Name == "id" || f.Has("id"):
			return nil, Refusef("field `%s`: the generator writes the id, `id: %sId @id`; leave it out", f.Name, name)
		case !o.NoTimestamps && (f.Name == "created" || f.Name == "updated" || f.Has("created") || f.Has("updated")):
			return nil, Refusef("field `%s`: the generator writes `created: Time @created` and `updated: Time @updated`; leave them out, or give --no-timestamps to write your own", f.Name)
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
	file := FileOf(name)
	edit := func() Edit {
		body := render("table.tmpl", d)
		if m == nil {
			return Edit{Path: file, Create: true, Body: body}
		}
		// in the module: what follows the template's `module` and imports, after its declarations
		_, rest, _ := strings.Cut(body, "type "+name+"Id = brand Text")
		e := newSrcEdit(m.Source)
		e.appendText("\ntype " + name + "Id = brand Text" + strings.TrimRight(rest, "\n"))
		addImports(e, m.Syntax, d.Imports)
		return Edit{Path: m.File, Body: e.result()}
	}

	// the tests of its delete rules, from the checked table
	probe, err := Probe(p.Dir, []Edit{edit()})
	if err != nil {
		return nil, err
	}
	if r := probe.errorsIn(edit().Path); r != nil {
		return nil, r
	}
	if t := probe.TableNamed(name); t != nil {
		d.Tests = deleteRuleTests(probe, t, imports)
	}
	d.Imports = imports.list()
	return []Edit{edit()}, nil
}

// deleteRuleTests are the tests of t's references' delete rules (chapter 8): with `@cascade`,
// deleting the row referred to deletes t's row; with `@restrict` (the default), the delete
// fails and the row stays. Each refers to a row of its own (target), so nothing else holds it.
// A reference of t to itself (a tree, `Maybe`: a first row has no parent) is tested on a parent
// and its child.
func deleteRuleTests(p *Program, t *Table, imports *importSet) []TestData {
	var tests []TestData
	for _, c := range t.Columns {
		r := p.TableNamed(c.Ref)
		if r == nil || r == t && !c.Maybe || (c.OnDelete != "cascade" && c.OnDelete != "restrict") {
			continue
		}
		tb := newTestBuilder(p, t.Module, imports)
		target := tb.insertRow(r, "target", nil, false)
		val := target
		if c.Maybe {
			val = "Just " + target
		}
		tb.insertRow(t, "_", map[string]string{c.Name: val}, false)
		if c.OnDelete == "cascade" {
			tb.let("let _ <- Db.delete %s %s", r.Name, target)
			tb.let("let left <- Db.from %s |> Db.count", t.Name)
			tests = append(tests, TestData{Name: quote("deleting its " + c.Name + " deletes a " + t.Name), Lines: tb.lines, Result: "left == 0"})
			continue
		}
		tb.let("let r <- Test.run (Db.delete %s %s)", r.Name, target)
		tb.let("let left <- Db.from %s |> Db.count", t.Name)
		tests = append(tests, TestData{Name: quote("a " + t.Name + " keeps its " + c.Name + " from being deleted"), Lines: tb.lines,
			Result: fmt.Sprintf("{ deleted: Result.toMaybe r.value, left: left } == { deleted: Nothing, left: %d }", tb.counts[t.Module])})
	}
	return tests
}

// typeImports imports the modules a type written as text names (`Post.PostId` → Post; `Maybe
// User.Role` → User); the standard library's (`Text.NonEmpty`) need no import.
// bareType refuses a generator argument that writes an app type without its module (`theme:Theme`
// where `Theme` declares the type `Theme`), as the checker would, but naming the fix: when
// exactly one other module of the app declares a type of that name, the refusal says
// "write `Theme.Theme`". A name the home module declares itself is its own.
func bareType(p *Program, home, typ string) error {
	for _, w := range strings.FieldsFunc(typ, func(r rune) bool {
		return !(r == '.' || r == '_' || r >= '0' && r <= '9' || r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z')
	}) {
		if strings.Contains(w, ".") || !IsUpperIdent(w) {
			continue
		}
		var in []string
		for _, m := range p.Modules {
			if p.Std[m.Name] || m.Syntax == nil {
				continue
			}
			for _, d := range m.Syntax.Decls {
				if td, ok := d.(*syntax.TypeDecl); ok && td.Name == w {
					if m.Name == home {
						in = nil
						goto next
					}
					in = append(in, m.Name)
				}
			}
		}
		if len(in) == 1 {
			return Refusef("`%s` is a type of module `%s`: write `%s.%s` in `%s`", w, in[0], in[0], w, typ)
		}
	next:
	}
	return nil
}

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
