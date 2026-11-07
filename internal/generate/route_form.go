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
	for _, n := range []string{o.Name, route} {
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
	case o.Hole:
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
			holes = append(holes, par.Name+": ?"+LowerFirst(route)+UpperFirst(par.Name))
		case !par.HasDefault:
			redirectable = false
		}
	}
	if len(holes) > 0 {
		d.Msg = route + " { " + strings.Join(holes, ", ") + " }"
	}
	path := strings.TrimSuffix(r.Path, "/") + "/_/" + route
	line := fmt.Sprintf("route %s = POST \"%s\" { %s } form %s%s", route, path, strings.Join(params, ", "), o.Name, guardText(r))
	if len(params) == 0 {
		line = fmt.Sprintf("route %s = POST \"%s\" { } form %s%s", route, path, o.Name, guardText(r))
	}

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
	e.insertBefore(lastRoute.GetSpan().End.Line+1, line+"\n")
	decls := snippet("form_decl", d) + "\n\n" + update + "\n"
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
	var openArgs, submitted []string
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
	setup := tb.lines
	rowsBefore := 0
	if into != nil {
		rowsBefore = tb.counts[into.Module]
	}
	var values []string
	for _, f := range rec.Fields {
		values = append(values, f.Name+": "+tb.sample(f.Type, f.Name, 1))
	}
	var backArgs []string
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
	if len(submitted) > 0 {
		post = "(" + route + " { " + strings.Join(submitted, ", ") + " })"
	}
	valid := append(append([]string{}, tb.lines...), "let _ <- Test.get ("+page+")", fmt.Sprintf("let r <- Test.submit %s { %s }", post, strings.Join(values, ", ")))
	tests := []TestData{}
	switch {
	case into != nil && rowsBefore == 0:
		var got, want []string
		for i, f := range rec.Fields {
			if into.Column(f.Name) != nil {
				got = append(got, f.Name+": row."+f.Name)
				want = append(want, values[i])
			}
		}
		for _, c := range into.Columns {
			if c.HasDefault && !c.ID && !c.Created && !c.Updated && ftypes[c.Name] == nil {
				if c.Union != nil {
					imports.add(c.Union.Module)
				}
				got = append(got, c.Name+": row."+c.Name)
				want = append(want, c.Name+": "+c.Default)
			}
		}
		valid = append(valid, "let rows <- Db.from "+recordName(into, o.Page)+" |> Db.all")
		tests = append(tests, TestData{Name: quote("submitting " + o.Name + " adds a row"), Lines: valid,
			Result: fmt.Sprintf("{ answer: r.answer, rows: List.map (\\row -> { %s }) rows } == { answer: %s, rows: [{ %s }] }", strings.Join(got, ", "), answer, strings.Join(want, ", "))})
	case into != nil:
		valid = append(valid, "let rows <- Db.from "+recordName(into, o.Page)+" |> Db.count")
		tests = append(tests, TestData{Name: quote("submitting " + o.Name + " adds a row"), Lines: valid,
			Result: fmt.Sprintf("{ answer: r.answer, rows: rows } == { answer: %s, rows: %d }", answer, rowsBefore+1)})
	default:
		tests = append(tests, TestData{Name: quote("submitting " + o.Name), Lines: valid, Result: "r.answer == ?expected"})
	}
	for _, f := range rec.Fields { // an empty required field
		msgText := emptyMessage(f.Type)
		if msgText == "" {
			continue
		}
		var raw []string
		for _, g := range rec.Fields {
			v := rawSample(g.Type)
			if g.Name == f.Name {
				v = ""
			}
			raw = append(raw, fmt.Sprintf("{ name: %s, value: %s }", quote(g.Name), quote(v)))
		}
		lines := append(append([]string{}, setup...), "let _ <- Test.get ("+page+")", fmt.Sprintf("let r <- Test.submitRaw %s [%s]", post, strings.Join(raw, ", ")))
		rejected := fmt.Sprintf("FormRejected [{ field: %s, message: %s }]", quote(f.Name), quote(msgText))
		res := "r.answer == " + rejected
		if into != nil {
			lines = append(lines, "let rows <- Db.from "+recordName(into, o.Page)+" |> Db.count")
			res = fmt.Sprintf("{ answer: r.answer, rows: rows } == { answer: %s, rows: %d }", rejected, rowsBefore)
		}
		tests = append(tests, TestData{Name: quote(o.Name + " refuses an empty " + f.Name), Lines: lines, Result: res})
		break
	}
	if err := addTests(e, m, tests); err != nil {
		return nil, err
	}
	addImports(e, p, m.Syntax, imports.list())
	return []Edit{{Path: m.File, Body: e.result()}}, nil
}
