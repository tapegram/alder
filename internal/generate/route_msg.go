package generate

import (
	"fmt"
	"strings"

	"alder/internal/check"
	"alder/internal/syntax"
)

// guardText is the `requires …` clause of route r as the route line writes it, with its leading
// space: " requires user, role Admin | Editor, silo org", "" for a route that requires nothing.
func guardText(r *Route) string {
	var parts []string
	parts = append(parts, r.Keys...)
	if len(r.Roles) > 0 {
		parts = append(parts, "role "+strings.Join(r.Roles, " | "))
	}
	if r.Silo != "" {
		parts = append(parts, "silo "+r.Silo)
	}
	if len(parts) == 0 {
		return ""
	}
	return " requires " + strings.Join(parts, ", ")
}

// newRouteMsg is `alder new action` on a route module (plan call 13, aliased as `new route
// --method POST` from the page's side): a write route `M = <METHOD> "<path>" { <page path
// parameters>, <fields> }`, the method POST unless --method says otherwise and the path the
// page's own unless --path says otherwise (the method tells the router which one is meant), with the page's guard, its `M.update` answering `Response.refresh`,
// a button sending it at the end of the page's view (--in-for, --when and --id place it as for a
// page), and a test that presses it from the page. The fields' values in the button and the
// test are holes, as a page's are, except a table's id: the row the page shows.
func newRouteMsg(p *Program, page, name string, fields []FieldSpec, o ActionOptions) ([]Edit, error) {
	r := p.RouteModule(page)
	if r == nil {
		return nil, Refusef("`%s` has no GET route: a message is a button on a route's `view`; `alder new route %s /path --method POST` makes a write route without one", page, page)
	}
	m := p.Modules[page]
	if r.IsFile {
		return nil, Refusef("a file page has no messages: `view` returns `File`, so nothing can be posted from it; move the message to an HTML page")
	}
	if !IsUpperIdent(name) {
		return nil, Refusef("`%s` is not a route name: a route is named by a capitalised identifier, such as `Archive`", name)
	}
	if declared(m.Syntax, name) {
		return nil, Refusef("`%s` already declares `%s`: choose another route name", page, name)
	}
	if o.ID != "" && !validID(o.ID) {
		return nil, Refusef("`--id %s` is not an id: use letters, digits, `-` and `_`, starting with a letter", o.ID)
	}
	if viewIsHole(m.Syntax) {
		return nil, Refusef("`%s`'s `view` is a hole: fill `view` first (`alder fill %s.view -`), then add the route's button", page, page)
	}
	if r.Silo != "" {
		return nil, Refusef("`%s` is an admin page of a silo: its routes are written by `alder regen admin`", page)
	}
	method := o.Method
	if method == "" {
		method = "POST"
	}
	path := o.Path
	if path == "" {
		path = r.Path
	} else if why := check.PagePathProblem(path); why != "" {
		return nil, Refusef("`%s` is not a route path: it %s; %s", path, why, check.PagePathRule)
	}
	if have := routeClash(p, method, path, ""); have != nil {
		return nil, clashRefusal(have, page, name, method, path, false)
	}
	imports := newImports(page, p.Std)
	d := MsgData{Name: name, Click: name, Label: Humanize(name), ID: o.ID}
	var loop *syntax.HtmlFor
	if o.InFor != "" {
		n := 0
		if loop, n = findFor(m.Syntax, o.InFor); loop == nil {
			return nil, Refusef("`%s`'s `view` has no `{for %s in …}`: name the loop variable of one of its loops", page, o.InFor)
		} else if n > 1 {
			return nil, Refusef("`%s`'s `view` has %d loops `{for %s in …}`: rename one, so that --in-for names one loop", page, n, o.InFor)
		}
		d.Class = kebab(name)
	}
	if w := strings.TrimSpace(o.When); w != "" {
		if _, ds, more, _ := syntax.ParseExpr("--when", []byte(w)); more != "" || len(ds) > 0 || strings.Contains(w, "\n") {
			return nil, Refusef("`--when %s` is not one expression", w)
		}
		d.When = w
	}
	// the route's parameters: the page's path parameters (the URL names the same row), then the fields
	var params []string
	seen, pathType := map[string]bool{}, map[string]string{}
	for _, par := range r.Params {
		if strings.Contains(r.Path, "{"+par.Name+"}") {
			seen[par.Name], pathType[par.Name] = true, typeText(p, par.Type, page)
			params = append(params, par.Name+": "+pathType[par.Name])
			importTypes(imports, par.Type)
		}
	}
	var own []FieldSpec // the fields the route adds to the path's parameters
	for _, f := range fields {
		if seen[f.Name] {
			if pathType[f.Name] == f.Type { // `id:Post.PostId` on a page whose path has {id}: the route's own
				continue
			}
			return nil, Refusef("field `%s` is a parameter of the page's path already, with another type (`%s`): the route takes it from there", f.Name, pathType[f.Name])
		}
		seen[f.Name] = true
		if err := bareType(p, page, f.Type); err != nil {
			return nil, err
		}
		typeImports(imports, f.Type, p)
		params = append(params, f.Source())
		own = append(own, f)
	}
	fields = own
	line := fmt.Sprintf("route %s = %s \"%s\" { %s }%s", name, method, path, strings.Join(params, ", "), guardText(r))
	if len(params) == 0 {
		line = fmt.Sprintf("route %s = %s \"%s\" { }%s", name, method, path, guardText(r))
	}
	update := fmt.Sprintf("%s.update : %s.Params -> Session -> Task Response\n%s.update params session = Task.succeed Response.refresh", name, name, name)

	// the button: the path parameters' values are holes (the page's `params` are not in scope in
	// its `view`), the fields' as a page's message has them
	holeOf := func(field string) string { return "?" + LowerFirst(name) + UpperFirst(field) }
	var click []string
	for _, par := range r.Params {
		if strings.Contains(r.Path, "{"+par.Name+"}") {
			click = append(click, par.Name+": "+holeOf(par.Name))
		}
	}
	for _, f := range fields {
		v := ""
		if loop != nil {
			v = binderValue(p, o.InFor, p.Res.Binds[loop], f.Name, f.Type, page)
		}
		if v == "" {
			v = holeOf(f.Name)
		}
		click = append(click, f.Name+": "+v)
	}
	if len(click) > 0 {
		d.Click = name + " { " + strings.Join(click, ", ") + " }"
	}
	e := newSrcEdit(m.Source)
	var lastRoute syntax.Decl
	var firstTest syntax.Decl
	for _, dc := range m.Syntax.Decls {
		switch dc.(type) {
		case *syntax.RouteDecl:
			lastRoute = dc
		case *syntax.TestDecl:
			if firstTest == nil {
				firstTest = dc
			}
		}
	}
	if loop != nil {
		if !addToFor(e, loop, snippet("msg_button", d)) {
			return nil, Refusef("the body of `%s`'s `{for %s in …}` is not one element with a closing tag: add the button yourself", page, o.InFor)
		}
	} else if err := addToView(e, m, page, snippet("msg_button", d), ""); err != nil {
		return nil, err
	}

	// after the button: a one-line view that ends the file is split at its end, where the update goes
	e.insertBefore(lastRoute.GetSpan().End.Line+1, line+"\n")
	if firstTest != nil {
		e.insertBefore(declStart(e, firstTest), update+"\n")
	} else {
		e.appendText("\n" + update + "\n")
	}

	// the test opens the page and presses the button; the route answers Response.refresh
	tb := newTestBuilder(p, page, imports).keyedForKeys(r.Keys)
	if len(r.Keys) > 0 {
		tb.login(firstOrEmpty(r.Roles))
	}
	pageArgs, pathValue := []string{}, map[string]string{}
	for _, par := range r.Params {
		if par.HasDefault {
			continue
		}
		v := tb.sample(par.Type, par.Name, 1)
		pageArgs = append(pageArgs, par.Name+": "+v)
		pathValue[par.Name] = v
	}
	pv := pageLit(r.Ref(page), pageArgs)
	var pressed []string
	for _, par := range r.Params {
		if v, ok := pathValue[par.Name]; ok && strings.Contains(r.Path, "{"+par.Name+"}") {
			pressed = append(pressed, par.Name+": "+v)
		}
	}
	for _, f := range fields {
		v := holeOf(f.Name)
		for _, t := range p.Tables {
			if f.Type == typeText(p, t.IDType, page) {
				if v = tb.ids[t.Module]; v == "" { // the page's row, when it shows one
					v = tb.insert(t, true)
				}
			}
		}
		if loop != nil { // the row is inserted, but which row the loop shows is the page's logic: the author's to say
			v = holeOf(f.Name)
		}
		pressed = append(pressed, f.Name+": "+v)
	}
	press := name
	if len(pressed) > 0 {
		press = "(" + name + " { " + strings.Join(pressed, ", ") + " })"
	}
	var comment []string
	if loop != nil {
		var holes []string
		for _, f := range fields {
			holes = append(holes, holeOf(f.Name))
		}
		comment = []string{
			"The button is in the view's `{for " + o.InFor + " in " + sliceSpan(m.Syntax.Lines, loop.List.GetSpan()) + " …}`: insert rows so that the",
			"signed-in user sees the row this clicks, and fill " + strings.Join(holes, ", ") + " with that row's values.",
		}
		if len(holes) == 0 {
			comment[1] = "signed-in user sees a row to click."
		}
	}
	if d.When != "" { // what makes the condition hold is the page's logic: the author's to set up
		comment = append(comment, "The button is shown only when `"+d.When+"`: insert the rows that make it hold",
			"on the page this test opens, or the click finds no button.")
	}
	test := TestData{Name: quote("pressing " + name), Result: "r.answer == Refreshed", Comment: comment,
		Lines: append(tb.lines, "let _ <- Test.get ("+pv+")", "let r <- Test.click "+press)}
	if err := addTests(e, m, []TestData{test}); err != nil {
		return nil, err
	}
	addImports(e, p, m.Syntax, imports.list())
	return []Edit{{Path: m.File, Body: e.result()}}, nil
}

func firstOrEmpty(xs []string) string {
	if len(xs) > 0 {
		return xs[0]
	}
	return ""
}
