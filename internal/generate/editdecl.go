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
// is not a record.
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
	e := newSrcEdit(m.Source)
	appendItem(e, rec.Span, fieldSpans(rec.Fields), f.Source())
	imports := newImports(m.Name, p.Std)
	typeImports(imports, f.Type, p)
	addImports(e, p, m.Syntax, imports.list())
	return []Edit{{Path: m.File, Body: e.result()}}, nil
}
