package generate

// EditPage generates `alder edit route <Module> add-param <name>:<Type>[=default]`: a parameter
// added at the end of the page's parameter record (`page "/teams" { q: Text = "" }`), a query
// parameter unless the path names it. A parameter without a default must be given wherever the
// page is named (`Teams.Index {}`), so such an edit is refused by the check unless nothing names
// the page yet. It refuses a name the page's parameters have.
func EditPage(p *Program, page, op string, args []string) ([]Edit, error) {
	if len(p.RoutesOf(page)) > 0 {
		return editRoute(p, page, op, args)
	}
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
	if rec == nil {
		return nil, Refusef("`%s` has no parameter record `{ … }` on its `page` line: add the parameter yourself", page)
	}
	e := newSrcEdit(m.Source)
	appendItem(e, rec.Span, fieldSpans(rec.Fields), f.Source())
	imports := newImports(page, p.Std)
	typeImports(imports, f.Type, p)
	addImports(e, p, m.Syntax, imports.list())
	return []Edit{{Path: m.File, Body: e.result()}}, nil
}

// editRoute is `alder edit route <Module> add-param <name>:<Type>[=default]` on a route module:
// the parameter goes at the end of the parameter record of the module's route (the one named
// after the module, else its first GET route, else its first route).
func editRoute(p *Program, module, op string, args []string) ([]Edit, error) {
	r := p.RouteModule(module)
	if r == nil {
		r = p.RoutesOf(module)[0]
	}
	m := p.Modules[module]
	if op != "add-param" || len(args) != 1 {
		return nil, Refusef("`alder edit route` has one operation: `add-param <name>:<Type>[=default]`")
	}
	f, err := ParseField(args[0], false)
	if err != nil {
		return nil, Refusef("%v", err)
	}
	for _, par := range r.Params {
		if par.Name == f.Name {
			return nil, Refusef("route `%s` of `%s` already has a parameter `%s`", r.Name, module, f.Name)
		}
	}
	rec := r.Decl.Params
	if rec == nil {
		return nil, Refusef("route `%s` of `%s` has no parameter record `{ … }`: add the parameter yourself", r.Name, module)
	}
	e := newSrcEdit(m.Source)
	appendItem(e, rec.Span, fieldSpans(rec.Fields), f.Source())
	imports := newImports(module, p.Std)
	typeImports(imports, f.Type, p)
	addImports(e, p, m.Syntax, imports.list())
	return []Edit{{Path: m.File, Body: e.result()}}, nil
}
