package generate

import (
	"strings"
)

// EditPage generates `alder edit page <Module> add-param <name>:<Type>[=default]`: a parameter
// added at the end of the page's parameter record (`page "/teams" { q: Text = "" }`), a query
// parameter unless the path names it. A parameter without a default must be given wherever the
// page is named (`Teams.Index {}`), so such an edit is refused by the check unless nothing names
// the page yet. It refuses a name the page's parameters have, and a record written over several
// lines.
func EditPage(p *Program, page, op string, args []string) ([]Edit, error) {
	pg, m, err := pageModule(p, page)
	if err != nil {
		return nil, err
	}
	if op != "add-param" || len(args) != 1 {
		return nil, Refusef("`alder edit page` has one operation: `add-param <name>:<Type>[=default]`")
	}
	f, err := ParseField(args[0], false)
	if err != nil {
		return nil, Refusef("%v", err)
	}
	for _, par := range pg.Params {
		if par.Name == f.Name {
			return nil, Refusef("`%s` already has a parameter `%s`", page, f.Name)
		}
	}
	rec := pg.Decl.Params
	if rec == nil || rec.Span.Start.Line != rec.Span.End.Line {
		return nil, Refusef("`%s`'s parameters are not one `{ … }` on its `page` line: add the parameter yourself", page)
	}
	e := newSrcEdit(m.Source)
	l := e.line(rec.Span.Start.Line)
	open, close := rec.Span.Start.Col-1, rec.Span.End.Col-1
	if close > len(l) || l[open] != '{' || l[close-1] != '}' {
		return nil, Refusef("internal error: `%s`'s parameter record is not where the parser says", page)
	}
	inner := strings.TrimSpace(l[open+1 : close-1])
	inner = strings.TrimSuffix(inner, ",")
	if inner != "" {
		inner += ", "
	}
	e.replaceLine(rec.Span.Start.Line, l[:open]+"{ "+inner+f.Source()+" }"+l[close:])
	imports := newImports(page, p.Std)
	typeImports(imports, f.Type, p)
	addImports(e, m.Syntax, imports.list())
	return []Edit{{Path: m.File, Body: e.result()}}, nil
}
