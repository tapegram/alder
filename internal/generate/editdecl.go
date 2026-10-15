package generate

import (
	"strings"

	"alder/internal/syntax"
)

// EditDecl generates `alder edit decl <Module>.<Name> add-prop <name>:<Type>=<default>`: a
// property added at the end of the props record of a component (`Page : { title: Text,
// children: List (Html a) } -> Html a`), with its default, so that every use of the component
// keeps checking (an anonymous record type's defaults apply to the component's literals). It
// refuses a property without a default, one the record has, a declaration whose first argument
// is not a record, and a record written over several lines.
func EditDecl(p *Program, address, op string, args []string) ([]Edit, error) {
	if op != "add-prop" || len(args) != 1 {
		return nil, Refusef("`alder edit decl` has one operation: `add-prop <name>:<Type>=<default>`")
	}
	i := strings.LastIndex(address, ".")
	if i < 0 {
		return nil, Refusef("`%s` is not an address: write `<Module>.<Name>`, such as `Layout.Page`", address)
	}
	m := p.Modules[address[:i]]
	if m == nil || p.Std[address[:i]] {
		return nil, Refusef("there is no module `%s` in this app", address[:i])
	}
	v := valueDecl(m.Syntax, address[i+1:])
	if v == nil {
		return nil, Refusef("`%s` has no declaration `%s`", m.Name, address[i+1:])
	}
	f, err := ParseField(args[0], false)
	if err != nil {
		return nil, Refusef("%v", err)
	}
	if f.Default == "" {
		return nil, Refusef("a property added to `%s` needs a default, so that every use keeps checking: `add-prop %s:%s=<default>`", address, f.Name, f.Type)
	}
	var rec *syntax.RecordType
	if ft, ok := v.Sig.(*syntax.FunType); ok && len(ft.Params) > 0 {
		rec, _ = ft.Params[0].(*syntax.RecordType)
	}
	if rec == nil {
		return nil, Refusef("`%s`'s first argument is not a record type `{ … }`: add the property yourself", address)
	}
	for _, g := range rec.Fields {
		if g.Name == f.Name {
			return nil, Refusef("`%s` already has a property `%s`", address, f.Name)
		}
	}
	if rec.Span.Start.Line != rec.Span.End.Line {
		return nil, Refusef("`%s`'s props record is not on one line: add the property yourself", address)
	}
	e := newSrcEdit(m.Source)
	l := e.line(rec.Span.Start.Line)
	open, close := rec.Span.Start.Col-1, rec.Span.End.Col-1
	if close > len(l) || l[open] != '{' || l[close-1] != '}' {
		return nil, Refusef("internal error: `%s`'s props record is not where the parser says", address)
	}
	inner := strings.TrimSuffix(strings.TrimSpace(l[open+1:close-1]), ",")
	if inner != "" {
		inner += ", "
	}
	e.replaceLine(rec.Span.Start.Line, l[:open]+"{ "+inner+f.Source()+" }"+l[close:])
	imports := newImports(m.Name, p.Std)
	typeImports(imports, f.Type, p)
	addImports(e, m.Syntax, imports.list())
	return []Edit{{Path: m.File, Body: e.result()}}, nil
}
