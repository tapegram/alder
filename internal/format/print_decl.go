package format

import (
	"cmp"
	"slices"
	"strings"

	"alder/internal/syntax"
)

// module prints the header, the imports (sorted, without repeats) and the declarations, one
// blank line between each part.
func (p *printer) module(m *syntax.Module) Doc {
	parts := []Doc{Text("module " + m.Name)}
	if len(m.Imports) > 0 {
		var ims []Doc
		for i, im := range sortedImports(m.Imports) {
			if i > 0 {
				ims = append(ims, HardLine)
			}
			ims = append(ims, Text("import "+im.Module))
		}
		parts = append(parts, Concat(ims...))
	}
	for _, d := range m.Decls {
		parts = append(parts, p.decl(d))
	}
	return Concat(join(parts, Concat(HardLine, HardLine)), HardLine)
}

func (p *printer) decl(d syntax.Decl) Doc {
	switch d := d.(type) {
	case *syntax.ValueDecl:
		return p.valueDecl(d)
	case *syntax.TypeDecl:
		return p.typeDecl(d)
	case *syntax.PageDecl:
		return p.pageDecl(d)
	case *syntax.MsgDecl:
		doc := []Doc{Text("msg " + d.Name)}
		if d.Params != nil {
			doc = append(doc, Text(" "), p.fieldRecord(d.Params.Fields))
		}
		if d.Form != "" {
			doc = append(doc, Text(" form "+d.Form))
		}
		return Concat(doc...)
	case *syntax.FormDecl:
		return Concat(Text("form "+d.Name+" = "), p.fieldRecord(d.Fields))
	case *syntax.TableDecl:
		return p.tableDecl(d)
	case *syntax.CapDecl:
		// always one entry per line, `name : Type` as a signature is written
		items := make([]Doc, len(d.Entries))
		for i, f := range d.Entries {
			items[i] = Concat(Text(f.Name+" : "), p.typ(f.Type))
		}
		return Concat(Text("capability "+d.Name+" "), braced(items, true))
	case *syntax.FakeDecl:
		return p.fakeDecl(d)
	case *syntax.SessionDecl:
		return Concat(Text("session "), p.fieldRecord(d.Fields))
	case *syntax.DeployDecl:
		return Concat(Text("deploy "), p.deployRecord(d.Fields))
	case *syntax.LoginDecl:
		return Text("login = " + d.Page)
	case *syntax.SiloDecl:
		items := make([]Doc, len(d.Tables))
		for i, t := range d.Tables {
			items[i] = Text(t)
		}
		return Concat(Text("silo "+d.Key+" "), braced(items, false))
	case *syntax.ChooseDecl:
		return Text("choose " + d.Key + " = " + d.Page)
	case *syntax.TestDecl:
		return Concat(Text("test \""+d.Name+"\" ="), p.testBody(d.Body))
	}
	panic("format: unknown declaration")
}

// valueDecl prints `name : Type` directly above `name params = body`.
func (p *printer) valueDecl(d *syntax.ValueDecl) Doc {
	var doc []Doc
	if d.Sig != nil {
		doc = append(doc, Text(d.Name+" : "), p.typ(d.Sig))
		if d.Body == nil {
			return Concat(doc...)
		}
		doc = append(doc, HardLine)
	}
	return Concat(append(doc, p.definition(d))...)
}

// definition prints `name params = body`.
func (p *printer) definition(d *syntax.ValueDecl) Doc {
	head := d.Name
	for _, x := range d.Params {
		head += " " + x.Name
	}
	return Concat(Text(head+" ="), p.after(d.Body))
}

// testBody prints a test's body after its `=`. A block there makes the test an effect test, so
// a block holding only a result stays a block.
func (p *printer) testBody(e syntax.Expr) Doc {
	if b, ok := e.(*syntax.Block); ok && len(b.Stmts) == 0 {
		return Concat(Text(" {"), Nest(Concat(HardLine, p.expr(b.Result))), HardLine, Text("}"))
	}
	return p.after(e)
}

func (p *printer) typeDecl(d *syntax.TypeDecl) Doc {
	head := "type " + d.Name
	for _, x := range d.Params {
		head += " " + x
	}
	switch {
	case d.Record != nil:
		return Concat(Text(head+" = "), p.fieldRecord(d.Record.Fields))
	case d.Brand != nil:
		doc := Text(head + " = brand " + d.Brand.Base)
		if d.Brand.Check != nil {
			// a lambda, `if` or `match` in parentheses, as the book writes them
			return Concat(doc, Text(" where "), p.operand(d.Brand.Check, p.level(d.Brand.Check) == levelOpen))
		}
		return doc
	case d.Union != nil:
		// one line if it fits, else `=` ends the line and each constructor starts one, `|` first
		var ctors []Doc
		for i, c := range d.Union {
			ctors = append(ctors, Line)
			if i > 0 {
				ctors = append(ctors, Text("| "))
			}
			ctors = append(ctors, Text(c.Name))
			if c.Arg != nil {
				ctors = append(ctors, Text(" "), p.atype(c.Arg))
			}
		}
		return Group(Concat(Text(head+" ="), Nest(Concat(ctors...))))
	}
	return Text(head) // opaque: stdlib signature files only
}

func (p *printer) pageDecl(d *syntax.PageDecl) Doc {
	doc := Concat(Text("page \""+d.Path+"\" "), p.fieldRecord(d.Params.Fields))
	var clauses []string
	if len(d.Requires) > 0 {
		clauses = append(clauses, strings.Join(d.Requires, ", "))
	}
	if len(d.Roles) > 0 {
		clauses = append(clauses, "role "+strings.Join(d.Roles, " | "))
	}
	if d.Silo != "" {
		clauses = append(clauses, "silo "+d.Silo)
	}
	if len(clauses) == 0 {
		return doc
	}
	return Concat(doc, Text(" requires "+strings.Join(clauses, ", ")))
}

func (p *printer) tableDecl(d *syntax.TableDecl) Doc {
	head := "table " + d.Name
	for _, a := range d.Attrs {
		head += " " + p.attr(a)
	}
	return Concat(Text(head+" "), p.fieldRecord(d.Fields))
}

// fakeDecl prints a fake's tables and entry definitions one per line, in source order, keeping
// one blank line where the source had any.
func (p *printer) fakeDecl(d *syntax.FakeDecl) Doc {
	var items []syntax.Decl
	for _, t := range d.Tables {
		items = append(items, t)
	}
	for _, e := range d.Entries {
		items = append(items, e)
	}
	if len(items) == 0 {
		return Text("fake " + d.Name + " { }")
	}
	slices.SortFunc(items, func(a, b syntax.Decl) int {
		sa, sb := a.GetSpan().Start, b.GetSpan().Start
		return cmp.Or(cmp.Compare(sa.Line, sb.Line), cmp.Compare(sa.Col, sb.Col))
	})
	var body []Doc
	for i, it := range items {
		if i > 0 {
			body = append(body, HardLine)
			if p.blankBetween(items[i-1].GetSpan().End.Line, it.GetSpan().Start.Line) {
				body = append(body, HardLine)
			}
		}
		switch it := it.(type) {
		case *syntax.TableDecl:
			body = append(body, p.tableDecl(it))
		case *syntax.ValueDecl:
			body = append(body, p.definition(it))
		}
	}
	return Concat(Text("fake "+d.Name+" {"), Nest(Concat(HardLine, Concat(body...))), HardLine, Text("}"))
}

// field prints `name: Type [via T] [= default] [@attr …]`.
func (p *printer) field(f *syntax.Field) Doc {
	doc := []Doc{Text(f.Name + ": "), p.typ(f.Type)}
	if f.Via != "" {
		doc = append(doc, Text(" via "+f.Via))
	}
	if f.Default != nil {
		doc = append(doc, Text(" = "), p.expr(f.Default))
	}
	for _, a := range f.Attrs {
		doc = append(doc, Text(" "+p.attr(a)))
	}
	return Concat(doc...)
}

// attr prints `@name`, `@ref Table`, `@partition field` or `@renamed "old"` (the string as
// written).
func (p *printer) attr(a *syntax.Attr) string {
	switch a.Name {
	case "renamed":
		s := p.spanText(a.Span)
		return "@renamed " + s[strings.IndexByte(s, '"'):]
	case "ref", "partition":
		return "@" + a.Name + " " + a.Arg
	}
	return "@" + a.Name
}

// fieldRecord prints a braced field list: 3 or more fields always one per line, fewer on one
// line when they fit.
func (p *printer) fieldRecord(fs []*syntax.Field) Doc {
	items := make([]Doc, len(fs))
	for i, f := range fs {
		items[i] = p.field(f)
	}
	return braced(items, len(items) >= 3)
}

// braced prints `{ a, b }`, or when it does not fit or vertical is set, one item per line with
// trailing commas and the closing brace on a line of its own. No items is `{ }`.
func braced(items []Doc, vertical bool) Doc {
	if len(items) == 0 {
		return Text("{ }")
	}
	first := Line
	if vertical {
		first = HardLine
	}
	return Group(Concat(Text("{"), Nest(Concat(first, join(items, Concat(Text(","), Line)), IfBreak(Text(","), Text("")))), Line, Text("}")))
}

func (p *printer) deployRecord(fs []*syntax.DeployField) Doc {
	items := make([]Doc, len(fs))
	for i, f := range fs {
		items[i] = Concat(Text(f.Name+": "), p.deployValue(f.Value))
	}
	return braced(items, len(items) >= 3)
}

func (p *printer) deployValue(v *syntax.DeployValue) Doc {
	switch v.Kind {
	case "string":
		return Text("\"" + v.Str + "\"")
	case "list":
		if len(v.List) == 0 {
			return Text("[]")
		}
		items := make([]Doc, len(v.List))
		for i, e := range v.List {
			items[i] = p.deployValue(e)
		}
		return Group(Concat(Text("["), Nest(Concat(SoftLine, join(items, Concat(Text(","), Line)), IfBreak(Text(","), Text("")))), SoftLine, Text("]")))
	case "ctor":
		doc := []Doc{Text(v.Ctor)}
		if v.Arg != nil {
			doc = append(doc, Text(" "), p.deployValue(v.Arg))
		}
		if v.Rec != nil {
			doc = append(doc, Text(" "), p.deployRecord(v.Rec))
		}
		return Concat(doc...)
	}
	return Text(v.Str) // a number as written
}

// ---- types

// typ prints a type. A function type that does not fit puts each further parameter and the
// result on a line of its own, `->` first, unless it holds a record type (see below). Record
// types inside a function type are width-based whatever their field count.
func (p *printer) typ(t syntax.Type) Doc {
	switch t := t.(type) {
	case *syntax.TypeName:
		if t.Module == "" && t.Name == "Unit" && len(t.Args) == 0 && strings.HasPrefix(p.spanText(t.Span), "(") {
			return Text("()")
		}
		name := t.Name
		if t.Module != "" {
			name = t.Module + "." + t.Name
		}
		doc := []Doc{Text(name)}
		for _, a := range t.Args {
			doc = append(doc, Text(" "), p.atype(a))
		}
		return Concat(doc...)
	case *syntax.TypeVar:
		return Text(t.Name)
	case *syntax.RecordType:
		if p.inFun > 0 { // width-based: the 3-field rule is for records that are a whole body
			items := make([]Doc, len(t.Fields))
			for i, f := range t.Fields {
				items[i] = p.field(f)
			}
			return braced(items, false)
		}
		return p.fieldRecord(t.Fields)
	case *syntax.FunType:
		p.inFun++
		defer func() { p.inFun-- }()
		// With a record in it, the record breaks (K&R, `} -> R` on its closing line) and the
		// arrows stay inline; otherwise the outermost function type's arrows break, `->` first.
		sep := Concat(Line, Text("-> "))
		if p.inFun > 1 || containsRecord(t) { // a function type inside another stays on its line
			sep = Text(" -> ")
		}
		var rest []Doc
		for _, x := range t.Params[1:] {
			rest = append(rest, sep, p.paramType(x))
		}
		rest = append(rest, sep, p.typ(t.Result))
		return Group(Concat(p.paramType(t.Params[0]), Nest(Concat(rest...))))
	}
	panic("format: unknown type")
}

// containsRecord reports whether a record type occurs anywhere in t.
func containsRecord(t syntax.Type) bool {
	switch t := t.(type) {
	case *syntax.RecordType:
		return true
	case *syntax.TypeName:
		return slices.ContainsFunc(t.Args, containsRecord)
	case *syntax.FunType:
		return containsRecord(t.Result) || slices.ContainsFunc(t.Params, containsRecord)
	}
	return false
}

// paramType prints a function type's parameter: a function type in parentheses.
func (p *printer) paramType(t syntax.Type) Doc {
	if _, ok := t.(*syntax.FunType); ok {
		return Concat(Text("("), p.typ(t), Text(")"))
	}
	return p.typ(t)
}

// atype prints a type in argument position: an applied type or a function type in parentheses.
func (p *printer) atype(t syntax.Type) Doc {
	switch t := t.(type) {
	case *syntax.TypeName:
		if len(t.Args) == 0 {
			return p.typ(t)
		}
	case *syntax.TypeVar, *syntax.RecordType:
		return p.typ(t)
	}
	return Concat(Text("("), p.typ(t), Text(")"))
}

func join(ds []Doc, sep Doc) Doc {
	var out []Doc
	for i, d := range ds {
		if i > 0 {
			out = append(out, sep)
		}
		out = append(out, d)
	}
	return Concat(out...)
}
