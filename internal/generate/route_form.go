package generate

import (
	"fmt"
	"slices"
	"strings"

	"alder/internal/syntax"
	"alder/internal/types"
)

// newRouteForm is `alder new form` on a route module (plan call 13): `form <Name>`, the write
// route `<Name> = POST "<page path>/_/<Name>" { <page path parameters> } form <Name>` with the
// page's guard (named `Submit<Name>`, or --msg's name: a route cannot share its form's name), its
// `update` (an insert into the table the fields match by name and type, or --into's, then
// `Response.redirect` to the page and `Response.success "Saved"`; the hole `?answer` with
// --hole, and when no table fits), the form in the GET route's `view` (inside its last
// `al-card`, or in a card of its own), and two tests: a valid submission, and an empty required
// field (FormRejected, nothing inserted).
func newRouteForm(p *Program, o FormOptions) ([]Edit, error) {
	r := p.RouteModule(o.Page)
	if r == nil {
		return nil, Refusef("`%s` has no GET route: a form is posted from a route's `view`; `alder new route %s /path` makes one", o.Page, o.Page)
	}
	m := p.Modules[o.Page]
	if r.IsFile {
		return nil, Refusef("a file page has no messages: `view` returns `File`, so nothing can be posted from it; move the message to an HTML page")
	}
	if r.Silo != "" {
		return nil, Refusef("`%s` is an admin page of a silo: its routes are written by `alder regen admin`", o.Page)
	}
	if !IsUpperIdent(o.Name) {
		return nil, Refusef("`%s` is not a form name: a form is named by a capitalised identifier, such as `Contact`", o.Name)
	}
	if slices.ContainsFunc(p.Tables, func(t *Table) bool { return t.Name == o.Name }) {
		return nil, Refusef("`%s` names a table of this app: a form named like it hides the table's row type in `%s`; choose another form name, such as `%sForm`", o.Name, o.Page, o.Name)
	}
	route := "Submit" + o.Name // not the form's name: `Test.submit F` would name the form, not the route
	if o.Msg != "" {
		if !IsUpperIdent(o.Msg) {
			return nil, Refusef("`--msg %s` is not a route name: a route is named by a capitalised identifier, such as `Create`", o.Msg)
		}
		if o.Msg == o.Name {
			return nil, Refusef("`--msg %s` names the form itself: the form and its route need two names", o.Msg)
		}
		route = o.Msg
	}
	// --msg M naming a write route of the module without a form: the form is attached to it
	var attach *syntax.RouteDecl
	if o.Msg != "" {
		if rd := routeDecl(m.Syntax, o.Msg); rd != nil && rd.Method != "GET" {
			if rd.Form != "" {
				return nil, Refusef("`%s` is posted with the form `%s` already: a route has one form; choose another route name", o.Msg, rd.Form)
			}
			attach = rd
		} else if rd != nil {
			return nil, Refusef("`--msg %s` is a GET route of `%s`: a form is posted to a write route; choose another route name, or one of its write routes", o.Msg, o.Page)
		}
	}
	if attach != nil && (o.Into != "" || o.Hole) {
		return nil, Refusef("`%s` has its `update` already: --into and --hole belong to a new route", route)
	}
	for _, n := range []string{o.Name, route} {
		if n == route && attach != nil {
			continue
		}
		if declared(m.Syntax, n) {
			return nil, Refusef("`%s` already declares `%s`: choose another name", o.Page, n)
		}
	}
	if viewIsHole(m.Syntax) {
		return nil, Refusef("`%s`'s `view` is a hole: fill `view` first (`alder fill %s.view -`), then add the form", o.Page, o.Page)
	}
	if len(o.Fields) == 0 {
		return nil, Refusef("a form needs at least one field: `alder new form %s %s title:Text.NonEmpty`", o.Page, o.Name)
	}
	if o.Hole && o.Into != "" {
		return nil, Refusef("--hole leaves the `update` body a hole and --into %s fills it with an insert: give one of them", o.Into)
	}
	seen := map[string]bool{}
	d := FormData{Name: o.Name, Msg: route, Label: Humanize(o.Name)}
	for _, f := range o.Fields {
		if seen[f.Name] {
			return nil, Refusef("field `%s` is given twice", f.Name)
		}
		seen[f.Name] = true
		d.Fields = append(d.Fields, f.Source())
	}
	imports := newImports(o.Page, p.Std)
	for _, f := range o.Fields {
		if err := bareType(p, o.Page, f.Type); err != nil {
			return nil, err
		}
		typeImports(imports, f.Type, p)
	}

	// the probe: the form declared in the module, to read its fields' types
	e := newSrcEdit(m.Source)
	addImports(e, p, m.Syntax, imports.list())
	e.appendText("\n" + snippet("form_decl", d))
	probe, err := Probe(p.Dir, []Edit{{Path: m.File, Body: e.result()}})
	if err != nil {
		return nil, err
	}
	if rs := probe.errorsIn(m.File); rs != nil {
		return nil, rs
	}
	rec := probe.Res.Forms[o.Page+"."+o.Name]
	if rec == nil || len(rec.Fields) != len(o.Fields) {
		return nil, Refusef("internal error: the form %s was not checked", o.Name)
	}
	ftypes := map[string]types.Type{}
	for _, f := range rec.Fields {
		ftypes[f.Name] = f.Type
		d.Inputs = append(d.Inputs, input(f.Name, f.Type))
	}

	// the table the update inserts into
	var into *Table
	switch {
	case o.Hole, attach != nil:
	case o.Into != "":
		if into = p.TableNamed(o.Into); into == nil {
			return nil, Refusef("`--into %s`: `%s` is not a table of this app (its tables: %s)", o.Into, o.Into, tableNames(p))
		}
	default:
		for _, t := range p.Tables {
			if fieldsFit(t, rec.Fields, r.Keys) {
				into = t
				break
			}
		}
	}

	// the route's parameters are the page's path parameters; the page's others (query
	// parameters) are not the write route's, so the redirect back needs the page without them
	var params, pageArgs, holes []string
	redirectable := true
	for _, par := range r.Params {
		switch {
		case strings.Contains(r.Path, "{"+par.Name+"}"):
			params = append(params, par.Name+": "+typeText(p, par.Type, o.Page))
			importTypes(imports, par.Type)
			pageArgs = append(pageArgs, par.Name+": params."+par.Name)
			if v, ok := rowKeyExpr(p, m, par); ok {
				holes = append(holes, par.Name+": "+v)
			} else {
				holes = append(holes, par.Name+": ?"+LowerFirst(route)+UpperFirst(par.Name))
			}
		case !par.HasDefault:
			redirectable = false
		}
	}
	if len(holes) > 0 {
		d.Msg = route + " { " + strings.Join(holes, ", ") + " }"
	}
	path := strings.TrimSuffix(r.Path, "/") + "/_/" + route
	ps := "{ }"
	if len(params) > 0 {
		ps = "{ " + strings.Join(params, ", ") + " }"
	}
	if attach != nil { // the route's own parameters: its button's and the test's values
		holes, params = nil, nil
		if attach.Params != nil {
			for _, f := range attach.Params.Fields {
				holes = append(holes, f.Name+": ?"+LowerFirst(route)+UpperFirst(f.Name))
			}
		}
		d.Msg = route
		if len(holes) > 0 {
			d.Msg = route + " { " + strings.Join(holes, ", ") + " }"
		}
	}
	line := fmt.Sprintf("route %s = POST \"%s\" %s form %s%s", route, path, ps, o.Name, guardText(r))

	// the update
	response := "Response.refresh"
	answerBack := "Refreshed"
	if redirectable {
		response = "Response.redirect (" + pageLit(r.Ref(o.Page), pageArgs) + ")"
	}
	body := "?answer"
	if into != nil {
		imports.add(into.Module)
		var inits []string
		for _, f := range o.Fields {
			inits = append(inits, f.Name+": params.form."+f.Name)
		}
		op := "Db.insert"
		switch {
		case into.PartKey == "user" && slices.Contains(r.Keys, "user"):
			op = "Db.insertMine"
		case into.PartKey != "" && slices.Contains(r.Keys, into.PartKey): // M15: the page's key fills the column
			op = "Db.insertScoped"
		}
		body = fmt.Sprintf("{\n  let _ <- %s %s { %s }\n  Task.succeed (%s |> Response.success \"Saved\")\n}", op, recordName(into, o.Page), strings.Join(inits, ", "), response)
	}
	update := fmt.Sprintf("%s.update : %s.Params -> Session -> Task Response\n%s.update params session = %s", route, route, route, body)

	e = newSrcEdit(m.Source)
	var lastRoute, firstTest syntax.Decl
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
	if err := addToView(e, m, o.Page, snippet("form_markup", d), d.Label); err != nil {
		return nil, err
	}
	decls := snippet("form_decl", d) + "\n\n" + update + "\n"
	if attach != nil {
		if err := attachForm(e, attach, o.Name); err != nil {
			return nil, err
		}
		dropMsgControls(e, m, route)
		decls = "-- " + route + ".update reads the posted fields as params.form (params.form : " + o.Name + ", such as params.form." + o.Fields[0].Name + ")\n" + snippet("form_decl", d) + "\n"
	} else {
		e.insertBefore(lastRoute.GetSpan().End.Line+1, line+"\n")
	}
	if firstTest != nil {
		e.insertBefore(declStart(e, firstTest), decls+"\n")
	} else {
		e.appendText("\n" + decls)
	}

	// the tests: the page opened, the form submitted
	tb := newTestBuilder(p, o.Page, imports).keyedForKeys(r.Keys)
	if len(r.Keys) > 0 {
		tb.login(firstOrEmpty(r.Roles))
	}
	var openArgs []string
	pathValue := map[string]string{}
	for _, par := range r.Params {
		if par.HasDefault {
			continue
		}
		v := tb.sample(par.Type, par.Name, 1)
		openArgs = append(openArgs, par.Name+": "+v)
		pathValue[par.Name] = v
	}
	page := pageLit(r.Ref(o.Page), openArgs)
	var submitted, backArgs []string
	for _, par := range r.Params {
		if v, ok := pathValue[par.Name]; ok && strings.Contains(r.Path, "{"+par.Name+"}") {
			submitted = append(submitted, par.Name+": "+v)
			backArgs = append(backArgs, par.Name+": "+v)
		}
	}
	answer := answerBack
	if redirectable {
		answer = "Redirected (" + pageLit(r.Ref(o.Page), backArgs) + ")"
	}
	post := route
	if attach != nil && attach.Params != nil {
		var vals []string
		for _, f := range attach.Params.Fields {
			v := "?" + LowerFirst(route) + UpperFirst(f.Name)
			if pv, ok := pathValue[f.Name]; ok {
				v = pv
			}
			vals = append(vals, f.Name+": "+v)
		}
		submitted = vals
	}
	if len(submitted) > 0 {
		post = "(" + route + " { " + strings.Join(submitted, ", ") + " })"
	}
	notice := ""
	if into != nil {
		notice = "Saved" // the body's `Response.success "Saved"`
	}
	tests := formTests(formTestSpec{Form: o.Name, Page: o.Page, Open: page, Submit: post, Raw: post, Answer: answer,
		Fields: rec.Fields, Types: ftypes, Into: into, NoEmpty: attach != nil, Notice: notice}, tb, imports)
	if attach == nil {
		tests = append(tests, routeFormGate(p, r, o, tb, page, post, rec.Fields, into)...)
	}
	if err := addTests(e, m, tests); err != nil {
		return nil, err
	}
	addImports(e, p, m.Syntax, imports.list())
	return []Edit{{Path: m.File, Body: e.result()}}, nil
}

// attachForm adds ` form <name>` to the route line of r, after its parameters (or its path, when
// it has none) and before its `requires`.
func attachForm(e *srcEdit, r *syntax.RouteDecl, name string) error {
	if r.Params != nil {
		sp := r.Params.GetSpan()
		e.insertAt(sp.End.Line, sp.End.Col, " form "+name)
		return nil
	}
	l := e.line(r.Span.Start.Line)
	q := strings.Index(l, `"`+r.Path+`"`)
	if q < 0 {
		return Refusef("`%s`'s route line is not one `route %s = %s \"%s\"` line: add ` form %s` to it yourself", r.Name, r.Name, r.Method, r.Path, name)
	}
	at := q + len(r.Path) + 2
	e.insertAt(r.Span.Start.Line, at+1, " form "+name)
	return nil
}

// routeFormGate are the tests that the form's write route guards itself (#160: a write route runs
// no `load`, so its own `requires` is all that stops it), each posting the form from its page,
// opened by a signed-in user (tb's setup: the login and the rows the page's path names), after
// the session changed; nothing is written:
//   - the session ended (a visitor): the route's `requires user` sends them to log in;
//   - the cookie's role is another than the route's `role` ones: 404, before the form is decoded.
func routeFormGate(p *Program, r *Route, o FormOptions, setup *testBuilder, page, post string, fields []types.Field, into *Table) []TestData {
	if p.Session == nil || !slices.Contains(r.Keys, "user") {
		return nil
	}
	var tests []TestData
	refused := func(name, answer string, change func(tb *testBuilder)) {
		tb := *setup // its maps are shared: nothing below inserts a row
		tb.lines = slices.Clone(setup.lines)
		var values []string
		for _, f := range fields {
			values = append(values, f.Name+": "+tb.sample(f.Type, f.Name, 1))
		}
		rowsBefore := 0
		if into != nil {
			rowsBefore = tb.counts[into.Module]
		}
		tb.let("let _ <- Test.get (%s)", page)
		change(&tb)
		tb.let("let r <- Test.submit %s { %s }", post, strings.Join(values, ", "))
		res := "r.answer == " + answer
		if into != nil {
			tb.let("let rows <- Db.from %s |> Db.count", recordName(into, o.Page))
			res = fmt.Sprintf("{ answer: r.answer, rows: rows } == { answer: %s, rows: %d }", answer, rowsBefore)
		}
		tests = append(tests, TestData{Name: quote(name), Lines: tb.lines, Result: res})
	}
	if p.Login != "" {
		refused("a visitor cannot submit "+o.Name, "Redirected ("+pageLit(p.Login, nil)+")", func(tb *testBuilder) { tb.logout() })
	}
	if len(r.Roles) > 0 && p.Session.Role != nil {
		for _, c := range p.Session.Role.Ctors {
			if !slices.Contains(r.Roles, c) {
				refused(article(c)+" cannot submit "+o.Name, "PageMissing", func(tb *testBuilder) { tb.login(c) })
				break
			}
		}
	}
	return tests
}

// rowKeyExpr is the value of a path parameter the page already holds: on a page whose `Model` is
// `{ row: T }` (the shape `new route --show T` writes) for a table T, a parameter of the type of
// T's key is `model.row.<key>`, so the button sent from its `view` needs no hole. Otherwise it
// returns false and the parameter stays a hole.
func rowKeyExpr(p *Program, page *Module, par *Param) (string, bool) {
	for _, d := range page.Syntax.Decls {
		td, ok := d.(*syntax.TypeDecl)
		if !ok || td.Name != "Model" || td.Record == nil || len(td.Record.Fields) != 1 {
			continue
		}
		f := td.Record.Fields[0]
		tn, ok := f.Type.(*syntax.TypeName)
		if f.Name != "row" || !ok || tn.Module != "" || len(tn.Args) > 0 {
			return "", false
		}
		t := p.TableNamed(tn.Name)
		if t == nil || typeText(p, t.IDType, page.Name) != typeText(p, par.Type, page.Name) {
			return "", false
		}
		return "model.row." + t.IDField, true
	}
	return "", false
}
