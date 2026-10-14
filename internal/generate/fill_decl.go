package generate

import (
	"strings"

	"alder/internal/diag"
	"alder/internal/syntax"
)

// FillDecl generates `alder fill --decl <Module>.<decl> -`: the whole body of one declaration
// replaced by src, read from standard input: a value's body (what follows `name params =`; its
// signature stays), a type declaration's right-hand side (what follows `type Name =`), or a
// test's body (`Module."test name"`). src must parse as one expression (one type for a type
// declaration); Apply then writes it only if the app checks, so a body that breaks anything
// (an `update` that drops an arm the view sends, a `Model` that `load` no longer builds) is
// refused. A module of the app src names is imported, as Fill does. It is the escape from "holes
// only" (chapter 15): still one declaration at a time, still typed and checked.
func FillDecl(p *Program, address, src string) (edits []Edit, imports []string, err error) {
	if err := controlChars(src); err != nil {
		return nil, nil, err
	}
	src = strings.TrimRight(src, " \t\r\n")
	for strings.HasPrefix(src, "\n") {
		src = src[1:]
	}
	if strings.TrimSpace(src) == "" {
		return nil, nil, Refusef("standard input is empty: write the declaration's new body")
	}
	mod, decl, arm, err := parseAddress(p, address)
	if err != nil {
		return nil, nil, err
	}
	if arm != "" {
		return nil, nil, Refusef("`%s` names an arm: --decl replaces a whole declaration, `%s.%s` (an arm's hole is `alder fill`'s)", address, mod, decl)
	}
	m := p.Modules[mod]
	var span diag.Span
	isType, isElement := false, false
	for _, d := range m.Syntax.Decls {
		switch d := d.(type) {
		case *syntax.ValueDecl:
			if d.Name == decl && d.Body != nil {
				span = d.Body.GetSpan()
				_, isElement = d.Body.(*syntax.Element)
			}
		case *syntax.TestDecl:
			if quote(d.Name) == decl {
				span = d.Body.GetSpan()
			}
		case *syntax.TypeDecl:
			if d.Name == decl {
				span, isType = typeRHS(d), true
			}
		}
	}
	if span.Start.Line == 0 {
		return nil, nil, Refusef("`%s` has no declaration `%s` with a body (a value, a type or a test)", mod, decl)
	}
	if isType {
		if _, err := parseTypeDecl(decl + " = " + src); err != nil {
			return nil, nil, Refusef("`alder fill --decl` takes one type for `type %s`: standard input is not one: %v", decl, err)
		}
	} else if _, ds, more, at := syntax.ParseExpr("stdin", []byte(src)); more != "" {
		return nil, nil, Refusef("`alder fill --decl` takes one expression; found more after it: %s at line %d, column %d of standard input", more, at.Line, at.Col)
	} else if len(ds) > 0 {
		return nil, nil, Refusef("`alder fill --decl` takes one expression; standard input does not parse as one: %s (line %d, column %d)", ds[0].Message, ds[0].Span.Start.Line, ds[0].Span.Start.Col)
	}
	lines := strings.Split(strings.TrimSuffix(m.Source, "\n"), "\n")
	if span.End.Line > len(lines) {
		return nil, nil, Refusef("internal error: the declaration `%s` is not where the parser says", decl)
	}
	first, last := lines[span.Start.Line-1], lines[span.End.Line-1]
	end := span.End.Col - 1
	if isElement && end < len(last) && last[end] == '>' { // an element's span ends on its last character
		end++
	}
	before, after := first[:min(len(first), span.Start.Col-1)], last[min(len(last), end):]
	// a multi-line body continues at the indentation of the line it starts on; markup that
	// follows `… =` starts on a line of its own, two spaces in, as a view is written
	body := strings.Split(src, "\n")
	ind := indentOf(first)
	if len(body) > 1 && strings.HasPrefix(src, "<") && strings.HasSuffix(strings.TrimRight(before, " "), "=") {
		before, ind = strings.TrimRight(before, " ")+"\n"+ind+"  ", ind+"  "
	}
	for i := 1; i < len(body); i++ {
		if body[i] != "" {
			body[i] = ind + body[i]
		}
	}
	e := newSrcEdit(m.Source)
	e.replaceLine(span.Start.Line, before+strings.Join(body, "\n")+after)
	for n := span.Start.Line + 1; n <= span.End.Line; n++ {
		e.deleteLine(n)
	}
	if !isType { // a test rewritten whole: the generator's comments on its holes go with them
		dropGuidance(e, m.Syntax, decl, !strings.Contains(src, "?expected"), !strings.Contains(src, "?"))
	}
	addImports(e, m.Syntax, fillImports(p, m, src, &imports))
	return []Edit{{Path: m.File, Body: e.result()}}, imports, nil
}

// typeRHS is the span of type declaration d's right-hand side: its record, its brand, or its
// constructors from the first to the last.
func typeRHS(d *syntax.TypeDecl) diag.Span {
	switch {
	case d.Record != nil:
		return d.Record.Span
	case d.Brand != nil:
		return d.Brand.Span
	case len(d.Union) > 0:
		return diag.Span{File: d.Span.File, Start: d.Union[0].Span.Start, End: d.Union[len(d.Union)-1].Span.End}
	}
	return diag.Span{}
}
