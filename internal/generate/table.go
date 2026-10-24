package generate

import (
	"fmt"
	"strings"

	"alder/internal/diag"
	"alder/internal/names"
	"alder/internal/syntax"
)

// TableData is the data of table.tmpl: the table's module and name, its imports, and its
// fields as declared (`title: Text.NonEmpty`, `post: Post.PostId @ref Post @cascade`), between
// the id and the `@created`/`@updated` pair the template writes.
type TableData struct {
	Module  string // Table.<Name>
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
	// Silo (--silo, M15): the table lives in the silo of its `@partition` key, which App's `silo
	// key { … }` gets it in (or is written with). A table that refers to a siloed one must.
	Silo bool
}

// NewTable generates `alder new table`: app/Table/<Name>.alder (module Table.<Name>, #20) with
// the table, its id brand, the fields given and `created`/`updated` (NewTableWith: without
// them), and a test of each reference's delete rule; a module Table.<Name> of types only takes
// the table. The migration is the caller's (Options.Migrate). It refuses a name that is not a
// table name or that names a table or a root module already, a field given twice, and the
// fields the generator writes itself.
func NewTable(p *Program, name string, fields []FieldSpec) ([]Edit, error) {
	return NewTableWith(p, name, fields, TableOptions{})
}

// TableNext is the command `alder new table` suggests next: a page listing the table's rows,
// at its name in the plural (`alder new page /todos --list Todo`).
func TableNext(name string) string {
	return "alder new page /" + Plural(strings.ToLower(name)) + " --list " + name
}

// NewTableWith is NewTable with options: with NoTimestamps the table has no `created` and
// `updated` columns, unless the fields given declare them (`created:Time@created`).
func NewTableWith(p *Program, name string, fields []FieldSpec, o TableOptions) ([]Edit, error) {
	if !IsUpperIdent(name) {
		return nil, Refusef("`%s` is not a table name: a table is named by one capitalised identifier, such as `Tag`, and declared in the module of that name", name)
	}
	mod := TableModule(name)
	if t := p.TableNamed(name); t != nil {
		return nil, Refusef("table `%s` exists (`%s`): a generator never overwrites a file, and table names are unique in an app; choose another table name", name, t.File)
	}
	if root := p.Modules[name]; root != nil {
		return nil, Refusef("module `%s` exists (`%s`): the table goes in `%s` (module `%s`), and `%s` would then name two modules; move `%s`'s declarations there, or choose another table name", name, root.File, FileOf(mod), mod, name, root.File)
	}
	m := p.Modules[mod]
	if m != nil { // a module of types only (`alder new type Table.User "Role = …"`) takes its table
		for _, d := range m.Syntax.Decls {
			if _, ok := d.(*syntax.TableDecl); ok {
				return nil, Refusef("module `%s` exists (`%s`) and declares a table: a generator never overwrites a file; choose another table name", mod, m.File)
			}
		}
		for _, n := range []string{name, name + "Id"} {
			if declared(m.Syntax, n) {
				return nil, Refusef("module `%s` already declares `%s`: the table needs the name", mod, n)
			}
		}
	}
	imports := newImports(mod, p.Std)
	d := TableData{Module: mod, Name: name, Timestamps: !o.NoTimestamps}
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
				imports.add(tableModuleOf(p, a.Arg))
			}
		}
		d.Fields = append(d.Fields, f.Source())
	}
	d.Imports = imports.list()
	var appEdit []Edit // --silo: App's `silo key { … }` with the table
	if o.Silo {
		key := ""
		for _, f := range fields {
			for _, a := range f.Attrs {
				if a.Name == "partition" {
					key = a.Arg
				}
			}
		}
		app := p.Modules["App"]
		switch {
		case key == "":
			return nil, Refusef("--silo puts the table in the silo of its `@partition` key: give a field one, such as `org:Org.OrgId@ref=Org@partition=org`")
		case app == nil:
			return nil, Refusef("--silo writes `silo %s { … }` in `App`, and there is no module `App`", key)
		}
		e, err := siloApp(app, key, []string{name}, []string{Plural(strings.ToLower(Humanize(name)))})
		if err != nil {
			return nil, err
		}
		appEdit = []Edit{{Path: app.File, Body: e.result()}}
	}
	file := FileOf(mod)
	edit := func() (Edit, error) {
		body := render("table.tmpl", d)
		if m == nil {
			body, err := canonical(p, file, body)
			return Edit{Path: file, Create: true, Body: body}, err
		}
		// in the module: what follows the template's `module` and imports, after its declarations
		_, rest, _ := strings.Cut(body, "type "+name+"Id = brand Text")
		e := newSrcEdit(m.Source)
		e.appendText("\ntype " + name + "Id = brand Text" + strings.TrimRight(rest, "\n"))
		addImports(e, p, m.Syntax, d.Imports)
		return Edit{Path: m.File, Body: e.result()}, nil
	}

	// the tests of its delete rules, from the checked table
	first, err := edit()
	if err != nil {
		return nil, err
	}
	probe, err := Probe(p.Dir, append([]Edit{first}, appEdit...))
	if err != nil {
		return nil, err
	}
	if r := probe.errorsIn(first.Path); r != nil {
		return nil, r
	}
	if t := probe.TableNamed(name); t != nil {
		d.Tests = deleteRuleTests(probe, t, imports)
	}
	d.Imports = imports.list()
	last, err := edit()
	if err != nil {
		return nil, err
	}
	return append([]Edit{last}, appEdit...), nil
}

// deleteRuleTests are the tests of t's references' delete rules (chapter 8): with `@cascade`,
// deleting the row referred to deletes t's row; with `@restrict` (the default), the delete
// fails and the row stays. Each refers to a row of its own (target), so nothing else holds it.
// A reference of t to itself (a tree, `Maybe`: a first row has no parent) is tested on a parent
// and its child.
func deleteRuleTests(p *Program, t *Table, imports *importSet) []TestData {
	var tests []TestData
	keyed := t.PartKey != "" && t.PartKey != "user" && p.Session != nil && p.Session.User != nil && p.KeyTable(t.PartKey) != nil
	for _, c := range t.Columns {
		r := p.TableNamed(c.Ref)
		if r == nil || r == t && !c.Maybe || (c.OnDelete != "cascade" && c.OnDelete != "restrict") {
			continue
		}
		if keyed && c.Name == t.PartCol { // the key's row: in a silo (`alder new silo`) its delete is the shared database's, not the table's (book 8)
			continue
		}
		tb := newTestBuilder(p, t.Module, imports)
		if keyed { // signed in with the key, so the test reads the rows wherever they live (a silo too)
			tb.silo = &siloCtx{key: t.PartKey, table: p.KeyTable(t.PartKey), member: true}
			tb.login("")
		}
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
			return Refusef("`%s` is a type of module `%s`: write `%s.%s` in `%s`", w, in[0], p.qualifierIn(home, in[0]), w, typ)
		}
	next:
	}
	return nil
}

// A qualifier that is the last segment of the home module (`Todo.TodoId` in the table Todo
// being written, module Table.Todo) is the home module's, which needs no import.
// A qualifier one of the home module's imports is written with (`TablePost`, an alias) is
// that import's module.
func typeImports(s *importSet, typ string, p *Program) {
	var have map[string]string // the home module's imports, by qualifier
	for _, w := range strings.FieldsFunc(typ, func(r rune) bool { return r == ' ' || r == '(' || r == ')' }) {
		if i := strings.LastIndex(w, "."); i > 0 {
			q := w[:i]
			if have == nil && p != nil {
				have = map[string]string{}
				for m, mq := range p.qualifiersFor(s.home, nil) {
					have[mq] = m
				}
			}
			if m, ok := have[q]; ok {
				s.add(m)
				continue
			}
			m := moduleOfQualifier(p, q)
			if m == q && strings.HasSuffix(s.home, "."+q) {
				m = s.home
			}
			s.add(m)
		}
	}
}

// tableModuleOf is the module of the table named name (`User` → Table.User, or User in an app
// of the layout before #20), or TableModule(name) for a table not declared yet (one the same
// command writes).
func tableModuleOf(p *Program, name string) string {
	if t := p.TableNamed(name); t != nil {
		return t.Module
	}
	return TableModule(name)
}

// moduleOfQualifier is the module of the app that q, written before a name, stands for: a
// module's full path, a table's name (`Todo` for the table of `Table.Todo`), or a qualifier
// when exactly one module of the app ends in it; q itself when it is none of these.
func moduleOfQualifier(p *Program, q string) string {
	if p == nil || p.Modules[q] != nil {
		return q
	}
	if t := p.TableNamed(q); t != nil {
		return t.Module
	}
	found := ""
	for _, m := range p.Order {
		if !p.Std[m] && strings.HasSuffix(m, "."+q) {
			if found != "" {
				return q
			}
			found = m
		}
	}
	if found == "" {
		return q
	}
	return found
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
