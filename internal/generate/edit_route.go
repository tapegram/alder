package generate

// EditRoute generates `alder edit route <Module> add-param <name>:<Type>[=default]`: the
// parameter goes at the end of the parameter record of the module's route (the one named after
// the module, else its first GET route, else its first route).
func EditRoute(p *Program, module, op string, args []string) ([]Edit, error) {
	if module == "App" {
		return nil, Refusef("`App` is not a route module: its routes (such as `Logout`) are write routes with no GET route, so there is no route to add a parameter to; edit `app/App.alder` by hand")
	}
	if len(p.RoutesOf(module)) == 0 {
		return nil, notRouteModule(p, module)
	}
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
