package generate

import (
	"fmt"
	"slices"
	"strings"

	"alder/internal/check"
	"alder/internal/names"
	"alder/internal/syntax"
	"alder/internal/types"
)

// newRouteForm is `alder new form` on a route module (plan call 13): `form <Name>`, the write
// route `<Name> = <METHOD> "<path>" { <page path parameters> } form <Name>` with the
// page's guard (named `Submit<Name>`, or --msg's name: a route cannot share its form's name), its
// `update` (an insert into the table the fields match by name and type, or --into's, then
// `Response.redirect` to the page and `Response.success "Saved"`; the hole `?answer` with
// --hole, and when no table fits), the form in the GET route's `view` (inside its last
// `al-card`, or in a card of its own), and two tests: a valid submission, and an empty required
// field (FormRejected, nothing inserted). The route's path is --path, else the collection of the
// table the form inserts into (`POST /entries`; formPath), else the page's own path; its method
// is --method, else POST. A new route that answers a method and path another route does is
// refused (routeClash), unless --msg attaches the form to a route that is there already.
func newRouteForm(p *Program, o FormOptions) ([]Edit, error) {
	r := p.RouteModule(o.Page)
	if r == nil {
		return nil, Refusef("`%s` has no GET route: a form is posted from a route's `view`; `alder new route %s /path` makes one", o.Page, o.Page)
	}
	m := p.Modules[o.Page]
	if r.IsFile {
		return nil, Refusef("a file route has no button: `view` returns `File`, so nothing can be posted from it; put the write route on an HTML route")
	}
	if r.Silo != "" {
		return nil, Refusef("`%s` is an admin route of a silo: its routes are written by `alder regen admin`", o.Page)
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
	if o.Edit != "" {
		if o.Msg == "" {
			route = "Save" + o.Name
		}
		return newEditForm(p, o, r, m, route)
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
	if attach != nil && (o.Path != "" || o.Method != "") {
		return nil, Refusef("`--path` and `--method` belong to a new route; `%s` has its path already", route)
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
	method := o.Method
	if method == "" {
		method = "POST"
	}
	path := o.Path
	if path == "" {
		path = formPath(r, into)
	} else if why := check.PagePathProblem(path); why != "" {
		return nil, Refusef("`%s` is not a route path: it %s; %s", path, why, check.PagePathRule)
	}
	if attach == nil {
		if have := routeClash(p, method, path, ""); have != nil {
			return nil, clashRefusal(have, o.Page, route, method, path, true)
		}
	}
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
	line := fmt.Sprintf("route %s = %s \"%s\" %s form %s%s", route, method, path, ps, o.Name, guardText(r))

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

// memberFormPath is the path an edit form's write route answers by default: the member of the
// table's collection (`/posts/{id}`), nested after the page's partition segment as formPath
// nests the collection.
func memberFormPath(r *Route, t *Table) string {
	return formPath(r, t) + "/{id}"
}

// formPath is the path a form's write route answers by default: the collection of the table the
// form inserts into (`/entries`), nested after the page's path segment `{k}` when the table is
// partitioned by a key k the path binds (`/a/{org}/tasks`), the page's own path when there is no
// table. A partition key that is not a path parameter comes from the session, so the collection
// is top level.
func formPath(r *Route, into *Table) string {
	if into == nil {
		return r.Path
	}
	coll := "/" + names.Collection(into.Name)
	if k := into.PartKey; k != "" {
		seg := "{" + k + "}"
		if i := strings.Index(r.Path, seg); i >= 0 {
			return r.Path[:i+len(seg)] + coll
		}
	}
	return coll
}

// newEditForm is `alder new form --edit T` (#194): on a page whose GET route takes `{id: T.TId}`
// in its path and whose Model holds the row (`row: T`, what `--show T` writes), `form <Name>` of
// the given fields (else T's editable columns: no ids, timestamps, secrets, owner, partition or
// references to users), the write route `Save<Name> = PUT "<member path>" { <page path
// parameters> } form <Name>` with the page's guard, its `update` writing the form over the row by
// the table's access (updateMine for an `@owner` table, updateScoped for a `@partition k` one, a
// keyless table's Db.update after Db.get, a missing row not found), the form in the view filled
// with the row's values (its path parameters the row's: the id, and a partition key the path
// binds), and its tests: as a resource's Edit page's. A reference is a select over the table it
// refers to, as on a resource's Edit page: its options join the page's Model and `load`, and a
// long table's search (a `find<Col>` parameter of the page) goes above the form.
func newEditForm(p *Program, o FormOptions, r *Route, m *Module, route string) ([]Edit, error) {
	if o.Into != "" || o.Hole {
		return nil, Refusef("`--edit` cannot be combined with `--into` or `--hole`")
	}
	t := p.TableNamed(o.Edit)
	if t == nil {
		return nil, Refusef("`--edit %s`: `%s` is not a table of this app (its tables: %s)", o.Edit, o.Edit, tableNames(p))
	}
	var idPar *Param
	for _, par := range r.Params {
		if par.Name == "id" && strings.Contains(r.Path, "{id}") {
			idPar = par
		}
	}
	if idPar != nil { // the page's Model is `{ row: T }`, and id is a T's
		if _, ok := rowKeyExpr(p, m, idPar); !ok || typeText(p, idPar.Type, o.Page) != typeText(p, t.IDType, o.Page) {
			idPar = nil
		}
	}
	if idPar == nil {
		return nil, Refusef("`%s` has no `%s` row to edit; make its page with `alder new route %s /… --show %s`", o.Page, t.Name, o.Page, t.Name)
	}
	if t.Silo != "" {
		return nil, Refusef("`%s` lives in the `%s` silo: `--edit` writes over the shared store only", t.Name, t.Silo)
	}
	if t.PartKey != "" && !slices.Contains(r.Keys, t.PartKey) {
		if t.Owner != "" {
			return nil, Refusef("`%s` is owned (`@owner`): an edit form on it needs a route that `requires user`", t.Name)
		}
		return nil, Refusef("`%s` is partitioned by `%s`: an edit form on it needs a route that `requires %s`", t.Name, t.PartKey, t.PartKey)
	}
	// the page's path parameters, as the view has them: the row's id, and a partition key the
	// path binds (`/a/{org}/…`), the row's partition column
	rowOf := map[*Param]*Column{idPar: nil}
	for _, par := range r.Params {
		if par == idPar || !strings.Contains(r.Path, "{"+par.Name+"}") {
			continue
		}
		c := t.Column(t.PartCol)
		if t.PartCol == "" || par.Name != t.PartKey || typeText(p, par.Type, o.Page) != typeText(p, c.Type, o.Page) {
			return nil, Refusef("`%s`'s path parameter `%s` is neither the row's id nor its partition key: an edit form fills its route's parameters from the row", o.Page, par.Name)
		}
		rowOf[par] = c
	}
	for _, n := range []string{o.Name, route} {
		if declared(m.Syntax, n) {
			return nil, Refusef("`%s` already declares `%s`: choose another name", o.Page, n)
		}
	}
	if viewIsHole(m.Syntax) {
		return nil, Refusef("`%s`'s `view` is a hole: fill `view` first (`alder fill %s.view -`), then add the form", o.Page, o.Page)
	}

	// the access of the table's writes, as a resource's: the scoped calls are the guard
	a := resourceAccess(p, t, ResourceOptions{Table: t.Name, Requires: strings.Join(r.Keys, ",")})
	a.keys, a.role, a.writeRequires = r.Keys, firstOrEmpty(r.Roles), guardText(r)
	g := &crudGen{p: p, a: a, files: map[string]string{}, names: map[string][]string{}}
	at := g.table(t, o.Page, "")
	at.form = o.Name
	// the owner and the partition are the session's (updateMine never writes them), and a form
	// never lists the users for another reference to them (as a resource's)
	userRef := func(c *Column) bool {
		return c.Name == t.PartCol || (p.Session != nil && c.Ref != "" && c.Ref == p.Session.UserTable)
	}
	at.editable = slices.DeleteFunc(at.editable, userRef)
	if len(o.Fields) > 0 {
		var names []string
		for _, c := range at.editable {
			names = append(names, c.Name)
		}
		var cols []*Column
		for _, f := range o.Fields {
			i := slices.IndexFunc(at.editable, func(c *Column) bool { return c.Name == f.Name })
			switch {
			case i < 0:
				return nil, Refusef("`%s` is not a column the edit form of `%s` can change (%s)", f.Name, t.Name, strings.Join(names, ", "))
			case slices.Contains(cols, at.editable[i]):
				return nil, Refusef("field `%s` is given twice", f.Name)
			case typeText(p, at.editable[i].Type, o.Page) != f.Type:
				return nil, Refusef("`%s` is `%s` in `%s`, not `%s`", f.Name, typeText(p, at.editable[i].Type, o.Page), t.Name, f.Type)
			}
			cols = append(cols, at.editable[i])
		}
		at.editable = cols
	}
	if len(at.editable) == 0 {
		return nil, Refusef("`%s` has no column an edit form can change", t.Name)
	}
	at.refs = slices.DeleteFunc(at.refs, func(c *Column) bool { return !slices.Contains(at.editable, c) })
	for _, c := range at.refs { // a select over another user's (key's) rows would list them all
		if rt := p.TableNamed(c.Ref); rt.PartKey != "" && !slices.Contains(r.Keys, rt.PartKey) {
			return nil, Refusef("`%s` refers to `%s`, whose rows are each %s's: an edit form with `%s` needs a route that `requires %s`", t.Name, rt.Name, rt.PartKey, c.Name, rt.PartKey)
		}
	}

	imports := newImports(o.Page, p.Std)
	imports.add(t.Module)
	model, loads, form, inputs := g.formParts(at, o.Page, imports, true)

	// the route: the page's path parameters, filled from the row; back to the page after
	var params, pageArgs, sent []string
	redirectable := true
	for _, par := range r.Params {
		switch {
		case strings.Contains(r.Path, "{"+par.Name+"}"):
			params = append(params, par.Name+": "+typeText(p, par.Type, o.Page))
			importTypes(imports, par.Type)
			pageArgs = append(pageArgs, par.Name+": params."+par.Name)
			col := t.IDField
			if c := rowOf[par]; c != nil {
				col = c.Name
			}
			sent = append(sent, par.Name+": model.row."+col)
		case !par.HasDefault:
			redirectable = false
		}
	}
	method := o.Method
	if method == "" {
		method = "PUT"
	}
	path := o.Path
	if path == "" {
		path = memberFormPath(r, t)
	} else if why := check.PagePathProblem(path); why != "" {
		return nil, Refusef("`%s` is not a route path: it %s; %s", path, why, check.PagePathRule)
	}
	if have := routeClash(p, method, path, ""); have != nil {
		return nil, clashRefusal(have, o.Page, route, method, path, true)
	}
	line := fmt.Sprintf("route %s = %s \"%s\" { %s } form %s%s", route, method, path, strings.Join(params, ", "), o.Name, guardText(r))

	// the update: getMine/getScoped need no load (the scoped update alone finds the row, another
	// user's being not found); Db.get's Maybe is matched, a missing row not found
	response := "Response.refresh"
	if redirectable {
		response = "Response.redirect (" + pageLit(r.Ref(o.Page), pageArgs) + ")"
	}
	direct := !a.maybeGet(a.writeGet)
	id := "row." + t.IDField
	if direct {
		id = "params.id"
	}
	var sets []string
	for _, c := range at.editable {
		sets = append(sets, c.Name+": params.form."+c.Name)
	}
	save := g.unique(at, id, []string{
		fmt.Sprintf("let _ <- %s %s %s (\\r -> { r | %s })", a.update, t.Name, id, strings.Join(sets, ", ")),
		"Task.succeed (" + response + " |> Response.success \"Saved\")",
	})
	if !direct {
		save = a.found(a.writeGet, t, "params.id", nil, save, "Task.notFound")
	}
	note := "-- " + route + " writes the form over the row and goes back to the page;\n-- a row deleted since is not found."
	if direct {
		note = "-- " + route + " writes the form over the row and goes back to the page;\n-- another " + a.whose() + "'s row, or one deleted since, is not found."
	}
	update := fmt.Sprintf("%s\n%s.update : %s.Params -> Session -> Task Response\n%s.update params session = {\n%s\n}", note, route, route, route, indent(strings.Join(save, "\n"), "  "))

	e := newSrcEdit(m.Source)
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
	// the selects' options: in the page's Model and its load (read with the row in hand, so a
	// long table's select keeps the row's value), a long table's search a parameter of the page
	var finder []string
	if len(model) > 0 {
		if err := addOptions(e, m, r, o.Page, model, loads, g.findParams(at), at); err != nil {
			return nil, err
		}
		vals := map[string]string{}
		for par, c := range rowOf {
			if c == nil {
				vals[par.Name] = textOf(at.idColumn(), "model.row."+t.IDField)
			} else {
				vals[par.Name] = textOf(c, "model.row."+c.Name)
			}
		}
		finder = g.finders(at, o.Page, pathAction(r.Path, vals))
	}
	d := FormData{Name: o.Name, Msg: route + " { " + strings.Join(sent, ", ") + " }", Inputs: inputs, Label: Humanize(o.Name)}
	markup := snippet("form_markup", d)
	if len(finder) > 0 {
		markup = strings.Join(finder, "\n") + "\n" + markup
	}
	if err := addToView(e, m, o.Page, markup, d.Label); err != nil {
		return nil, err
	}
	e.insertBefore(lastRoute.GetSpan().End.Line+1, line+"\n")
	decls := strings.Join(form, "\n") + "\n\n" + update + "\n"
	if firstTest != nil {
		e.insertBefore(declStart(e, firstTest), decls+"\n")
	} else {
		e.appendText("\n" + decls)
	}

	// the tests reach the page and its write route for a row target: its path parameters the
	// row's (a partition key the one the row lives under), any other the page needs a sample
	args := func(tb *testBuilder, target string, path bool) []string {
		var out []string
		for _, par := range r.Params {
			inPath := strings.Contains(r.Path, "{"+par.Name+"}")
			if path && !inPath || !inPath && par.HasDefault {
				continue
			}
			switch c, ok := rowOf[par]; {
			case ok && c == nil:
				out = append(out, par.Name+": "+target)
			case ok:
				out = append(out, par.Name+": "+tb.ownerOf(t, target))
			default:
				out = append(out, par.Name+": "+tb.sample(par.Type, par.Name, 1))
			}
		}
		return out
	}
	at.host = &editHost{
		open: func(tb *testBuilder, target string, extra ...string) string {
			return pageLit(r.Ref(o.Page), append(args(tb, target, false), extra...))
		},
		save: func(tb *testBuilder, target string) string {
			return "(" + route + " { " + strings.Join(args(tb, target, true), ", ") + " })"
		},
		back: func(tb *testBuilder, target string) string {
			if !redirectable {
				return "Refreshed"
			}
			return "Redirected (" + pageLit(r.Ref(o.Page), args(tb, target, true)) + ")"
		},
		run: func(tb *testBuilder, target, form string) string {
			return route + ".update { " + strings.Join(args(tb, target, true), ", ") + ", form: { " + form + " } }"
		},
	}
	var tests []TestData
	{ // every field changed, each to a value it did not have; the selects offer their rows
		tb := g.tb(o.Page, imports, true)
		target := tb.insertRow(t, "", nil, false)
		optLines := g.optionRows(tb, at)
		g.othersOptions(tb, at)
		k := tb.counts[t.Module]
		var values, got, want []string
		for _, c := range at.editable {
			v := g.changedValue(tb, at, c, k+1)
			values = append(values, c.Name+": "+v)
			got = append(got, c.Name+": row."+c.Name)
			want = append(want, c.Name+": "+v)
		}
		tb.let("let page <- Test.get (%s)", at.editPage(tb, target))
		tb.let("let r <- Test.submit %s { %s }", at.formRoute(tb, true, target), strings.Join(values, ", "))
		tb.let("let after <- Db.get %s %s", t.Name, target)
		lhs := append([]string{"answer: r.answer", "notices: r.notices", "row: Maybe.map (\\row -> { " + strings.Join(got, ", ") + " }) after"}, optLines[0]...)
		rhs := append([]string{"answer: " + at.editBack(tb, target), `notices: ["Saved"]`, "row: Just { " + strings.Join(want, ", ") + " }"}, optLines[1]...)
		tests = append(tests, TestData{Name: quote("saving " + o.Name + " changes every field"), Lines: tb.lines,
			Result: "{ " + strings.Join(lhs, ", ") + " } == { " + strings.Join(rhs, ", ") + " }"})
	}
	for _, c := range at.editable { // the first required field, empty: refused, the row unchanged
		msgText := emptyMessage(c.Type)
		if msgText == "" {
			continue
		}
		tb := g.tb(o.Page, imports, true)
		target := tb.insertRow(t, "", nil, false)
		tb.let("let before <- Db.get %s %s", t.Name, target)
		tb.let("let page <- Test.get (%s)", at.editPage(tb, target))
		var raw []string
		for _, f := range at.editable {
			v := quote(rawSample(f.Type))
			switch {
			case f == c:
				v = `""`
			case f.Ref != "" && !f.Maybe: // a select's value is one of its options
				v = f.Name + "Id"
				tb.let("let %s = Maybe.withDefault \"\" (Test.attr \"value\" \"select[name=%s] option\" page.doc)", v, f.Name)
			}
			raw = append(raw, fmt.Sprintf("{ name: %s, value: %s }", quote(f.Name), v))
		}
		tb.let("let r <- Test.submitRaw %s [%s]", at.formRoute(tb, true, target), strings.Join(raw, ", "))
		tb.let("let after <- Db.get %s %s", t.Name, target)
		tests = append(tests, TestData{Name: quote("a required field left empty in " + o.Name + " is rejected"), Lines: tb.lines,
			Result: fmt.Sprintf("{ answer: r.answer, row: after } == { answer: FormRejected [{ field: %s, message: %s }], row: before }", quote(c.Name), quote(msgText))})
		break
	}
	tests = append(tests, g.uniqueTests(at, o.Page, imports, true)...)
	tests = append(tests, g.notOffered(at, o.Page, imports, true)...)
	tests = append(tests, g.findTests(at, o.Page, imports, true)...)
	// saveAs posts the form, every field changed, for the row target: the row is unchanged after
	saveAs := func(tb *testBuilder, target string) (string, string, func() (string, string)) {
		g.optionRows(tb, at)
		k := tb.counts[t.Module]
		var values []string
		for _, c := range at.editable {
			values = append(values, c.Name+": "+g.changedValue(tb, at, c, k+1))
		}
		tb.let("let before <- Db.get %s %s", t.Name, target)
		return at.editPage(tb, target), fmt.Sprintf("Test.submit %s { %s }", at.formRoute(tb, true, target), strings.Join(values, ", ")), func() (string, string) {
			tb.let("let after <- Db.get %s %s", t.Name, target)
			return "row: after", "row: before"
		}
	}
	// the write route's own guard (a visitor, a user of another role), the row unchanged: posted
	// from the page, a visitor is sent to log in by the page's guard too, so the row is the proof
	tests = append(tests, g.writeGate(o.Page, imports, "submit "+o.Name, len(at.refs) > 0, func(tb *testBuilder) (string, string, func() (string, string)) {
		return saveAs(tb, tb.insertRow(t, "", nil, false))
	})...)
	if who := g.scopedTo(at); who != "" {
		tests = append(tests, g.othersWrite(at, o.Page, imports, o.Name+" on another "+who+"'s row is not found", true, saveAs))
		if len(at.refs) > 0 { // the browser's post is refused first (400): the update itself
			tests = append(tests, g.othersSave(at, o.Page, imports, who))
		}
	}
	if err := addTests(e, m, tests); err != nil {
		return nil, err
	}
	addImports(e, p, m.Syntax, imports.list())
	return []Edit{{Path: m.File, Body: e.result()}}, nil
}

// addOptions adds the options of an edit form's selects to page r of module m (what `--show`
// writes): model's fields to its `Model { row: T }`, loads before its load's `Query.succeed {
// row: row }` (in the load's block, or in the `Just row ->` arm of its match on `Db.get`) with
// those fields added to the record, and the long tables' search parameters find to its route.
func addOptions(e *srcEdit, m *Module, r *Route, page string, model, loads, find []string, at *crudTable) error {
	var fields, names []string
	for _, f := range model {
		f = strings.TrimSuffix(f, ",")
		fields = append(fields, f)
		name, _, _ := strings.Cut(f, ":")
		names = append(names, name+": "+name)
	}
	vd := valueDeclFull(m.Syntax, r.Name+".load")
	var td *syntax.TypeDecl
	for _, dc := range m.Syntax.Decls {
		if x, ok := dc.(*syntax.TypeDecl); ok && x.Name == "Model" && x.Record != nil {
			td = x
		}
	}
	const succeed = "Query.succeed { row: row }"
	at1 := 0
	if vd != nil {
		for n := vd.Span.Start.Line; n <= vd.Span.End.Line; n++ {
			if strings.Contains(e.line(n), succeed) {
				at1 = n
				break
			}
		}
	}
	if td == nil || at1 == 0 || r.Decl.Params == nil {
		return Refusef("`%s`'s `load` does not answer `%s` as `--show` writes it: a select needs its options loaded there; give the form's fields without references", page, succeed)
	}
	l := e.line(at1)
	ind := indentOf(l)
	body := append(slices.Clone(loads), "Query.succeed { row: row, "+strings.Join(names, ", ")+" }")
	switch strings.TrimSpace(l) {
	case succeed:
		e.replaceLine(at1, indent(strings.Join(body, "\n"), ind))
	case "Just row -> " + succeed + ",":
		e.replaceLine(at1, ind+"Just row -> {\n"+indent(strings.Join(body, "\n"), ind+"  ")+"\n"+ind+"},")
	default:
		return Refusef("`%s`'s `load` does not answer `%s` as `--show` writes it: a select needs its options loaded there; give the form's fields without references", page, succeed)
	}
	appendItem(e, td.Record.Span, fieldSpans(td.Record.Fields), strings.Join(fields, ", "))
	if len(find) > 0 {
		appendItem(e, r.Decl.Params.Span, fieldSpans(r.Decl.Params.Fields), strings.Join(find, ", "))
	}
	return nil
}
