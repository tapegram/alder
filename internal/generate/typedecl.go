package generate

import (
	"strings"

	"alder/internal/syntax"
)

// NewType generates `alder new type <Module> "<Name> = …"`: one type declaration, written as
// chapter 4 writes them after `type ` (a union `Status = Pending | Done`, a brand `Slug = brand
// Text where …`, or a record `Row = { … }`), in the module after its existing type
// declarations (before its first other declaration when it has none), the module created when
// it does not exist. A module of the app the declaration names (qualified, or a table's record
// type by its name) is imported. It refuses text that is not one type declaration, and a name
// (the type's or a constructor's) the module declares already.
func NewType(p *Program, module, decl string) ([]Edit, error) {
	if !IsModuleName(module) {
		return nil, Refusef("`%s` is not a module name: write capitalised names joined by dots, such as `Invite`", module)
	}
	decl = strings.Join(strings.Fields(decl), " ")
	td, err := parseTypeDecl(decl)
	if err != nil {
		return nil, err
	}
	text := "type " + decl
	imports := newImports(module, p.Std)
	for _, q := range qualifiers(p, decl) {
		imports.add(q)
	}
	m := p.Modules[module]
	if m == nil {
		body := "module " + module + "\n\n" + imports.lines() + "\n" + text + "\n"
		body, err := canonical(p, FileOf(module), body)
		if err != nil {
			return nil, err
		}
		return []Edit{{Path: FileOf(module), Create: true, Body: body}}, nil
	}
	names := []string{td.Name}
	for _, c := range td.Union {
		names = append(names, c.Name)
	}
	for _, n := range names {
		if declared(m.Syntax, n) {
			return nil, Refusef("`%s` already declares `%s`: choose another name", module, n)
		}
	}
	e := newSrcEdit(m.Source)
	var last *syntax.TypeDecl
	for _, d := range m.Syntax.Decls {
		if t, ok := d.(*syntax.TypeDecl); ok {
			last = t
		}
	}
	switch {
	case last != nil:
		e.insertBefore(last.Span.End.Line+1, "\n"+text)
	case len(m.Syntax.Decls) > 0:
		e.insertBefore(declStart(e, m.Syntax.Decls[0]), text+"\n")
	default:
		e.appendText("\n" + text)
	}
	addImports(e, p, m.Syntax, imports.list())
	return []Edit{{Path: m.File, Body: e.result()}}, nil
}

// parseTypeDecl parses decl (`Name = …`) as the one type declaration `type <decl>`.
func parseTypeDecl(decl string) (*syntax.TypeDecl, error) {
	if strings.Contains(decl, "--") {
		return nil, Refusef("`%s` holds `--`, which begins a comment in Alder source: write the type without it", decl)
	}
	if name, rhs, ok := strings.Cut(decl, " = "); ok {
		for _, base := range []string{"Text", "Int", "Float", "Bool"} {
			if strings.HasPrefix(rhs, base+" where ") || rhs == base {
				return nil, Refusef("`type %s` is not a type declaration: a brand is written `%s = brand %s where …` (chapter 4), or `%s = brand %s` without a check", decl, strings.TrimSpace(name), base, strings.TrimSpace(name), base)
			}
		}
	}
	m, ds := syntax.ParseFile("X.alder", []byte("module X\n\ntype "+decl+"\n"))
	for _, d := range ds {
		if d.Severity == "error" {
			return nil, Refusef("`type %s` is not a type declaration: %s", decl, d.Message)
		}
	}
	if len(m.Decls) != 1 {
		return nil, Refusef("`type %s` is not one type declaration", decl)
	}
	td, ok := m.Decls[0].(*syntax.TypeDecl)
	if !ok {
		return nil, Refusef("`type %s` is not one type declaration", decl)
	}
	return td, nil
}

// NewDecl generates `alder new decl <Module> <name> "<Type>"`: the declaration `name : Type`
// with the body `name = ?name`, a typed hole for `alder fill`, before the module's first test
// (at its end when it has none). It refuses a name the module declares and a type that does
// not parse.
func NewDecl(p *Program, module, name, typ string) ([]Edit, error) {
	module = p.ResolveModule(module)
	m := p.Modules[module]
	if m == nil {
		return nil, Refusef("there is no module `%s` (`alder new type` or `alder new table` makes one)", module)
	}
	if !IsLowerIdent(name) {
		return nil, Refusef("`%s` is not a value name: a value is named by a lower-case identifier, such as `waiting`", name)
	}
	if valueDecl(m.Syntax, name) != nil {
		return nil, Refusef("`%s` already declares `%s`: choose another name, or `alder fill --decl %s.%s` to replace its body", module, name, module, name)
	}
	typ = strings.Join(strings.Fields(typ), " ")
	if strings.Contains(typ, "--") {
		return nil, Refusef("`%s` holds `--`, which begins a comment in Alder source: write the type without it", typ)
	}
	src := "module X\n\n" + name + " : " + typ + "\n" + name + " = ?" + name + "\n"
	pm, ds := syntax.ParseFile("X.alder", []byte(src))
	for _, d := range ds {
		if d.Severity == "error" {
			return nil, Refusef("`%s` is not a type: %s", typ, d.Message)
		}
	}
	if len(pm.Decls) != 1 {
		return nil, Refusef("`%s` is not a type", typ)
	}
	text := name + " : " + typ + "\n" + name + " = ?" + name
	e := newSrcEdit(m.Source)
	insertBeforeTests(e, m.Syntax, text)
	imports := newImports(module, p.Std)
	for _, q := range qualifiers(p, typ) {
		imports.add(q)
	}
	addImports(e, p, m.Syntax, imports.list())
	return []Edit{{Path: m.File, Body: e.result()}}, nil
}

// insertBeforeTests inserts the declaration text before module m's first test (and its
// comment), else at its end.
func insertBeforeTests(e *srcEdit, m *syntax.Module, text string) {
	for _, d := range m.Decls {
		if _, ok := d.(*syntax.TestDecl); ok {
			e.insertBefore(declStart(e, d), text+"\n")
			return
		}
	}
	e.appendText("\n" + text)
}

// NewTest generates `alder new test <Module> "<name>"`: `test "<name>" = { ?test }`, whose
// hole is the test's whole body, a `Task Bool` (a block filled by `alder fill` becomes the
// test's block), at the end of the module. It refuses a name the module's tests have.
func NewTest(p *Program, module, name string) ([]Edit, error) {
	module = p.ResolveModule(module)
	m := p.Modules[module]
	if m == nil {
		return nil, Refusef("there is no module `%s`: a test is written in the module it tests", module)
	}
	if strings.TrimSpace(name) == "" || strings.ContainsAny(name, "\"\n\\{}") {
		return nil, Refusef("`%s` is not a test name: write words, without quotes, braces or backslashes", name)
	}
	if testNamed(m.Syntax, name) {
		return nil, Refusef("`%s` already has a test %s: choose another name", module, quote(name))
	}
	e := newSrcEdit(m.Source)
	e.appendText("\ntest " + quote(name) + " = {\n  ?test\n}")
	return []Edit{{Path: m.File, Body: e.result()}}, nil
}
