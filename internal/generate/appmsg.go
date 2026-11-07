package generate

import (
	"fmt"
	"slices"
	"strings"

	"alder/internal/syntax"
)

// appHasLegacyMsg reports whether App is written with the pre-routes message form: a `msg`
// declaration or an `update` of its own.
func appHasLegacyMsg(m *syntax.Module) bool {
	for _, d := range m.Decls {
		switch d := d.(type) {
		case *syntax.MsgDecl:
			return true
		case *syntax.ValueDecl:
			if d.Route == "" && d.Name == "update" {
				return true
			}
		}
	}
	return false
}

// newAppRouteMsg is `alder new msg App <Name> [fields]` on an App written with routes (plan call
// 13): `route <Name> = POST "/_/<Name>" { fields }` after App's last route (else after its
// `login`, else after its `session`), `<Name>.update` answering `Response.refresh`, and a test
// that opens the app's first plain GET page and presses the route's button. A route of App is
// sent by a button on any page (`click={App.<Name>}`), so no page's view is touched: the test
// fails, saying so, until a page has the button.
func newAppRouteMsg(p *Program, name string, fields []FieldSpec) ([]Edit, error) {
	app := p.Modules["App"]
	if !IsUpperIdent(name) {
		return nil, Refusef("`%s` is not a route name: a route is named by a capitalised identifier, such as `ToggleTheme`", name)
	}
	if declared(app.Syntax, name) {
		return nil, Refusef("`App` already declares `%s`: choose another route name", name)
	}
	imports := newImports("App", p.Std)
	var decl, args []string
	seen := map[string]bool{}
	for _, f := range fields {
		if seen[f.Name] {
			return nil, Refusef("field `%s` is given twice", f.Name)
		}
		seen[f.Name] = true
		decl = append(decl, f.Source())
		if err := bareType(p, "App", f.Type); err != nil {
			return nil, err
		}
		typeImports(imports, f.Type, p)
		args = append(args, f.Name+": ?"+LowerFirst(name)+UpperFirst(f.Name))
	}
	line := fmt.Sprintf("route %s = POST \"/_/%s\" { }", name, name)
	if len(decl) > 0 {
		line = fmt.Sprintf("route %s = POST \"/_/%s\" { %s }", name, name, strings.Join(decl, ", "))
	}
	e := newSrcEdit(app.Source)
	var after, firstTest syntax.Decl
	rank := 0 // a route beats a login beats a session as the place to go after
	for _, dc := range app.Syntax.Decls {
		switch dc.(type) {
		case *syntax.SessionDecl:
			if rank < 1 {
				after, rank = dc, 1
			}
		case *syntax.LoginDecl:
			if rank < 2 {
				after, rank = dc, 2
			}
		case *syntax.RouteDecl:
			after, rank = dc, 3
		case *syntax.TestDecl:
			if firstTest == nil {
				firstTest = dc
			}
		}
	}
	update := fmt.Sprintf("%s.update : %s.Params -> Session -> Task Response\n%s.update params session = Task.succeed Response.refresh", name, name, name)
	if after != nil {
		e.insertBefore(after.GetSpan().End.Line+1, line+"\n")
	} else {
		e.appendText("\n" + line + "\n")
	}
	if firstTest != nil {
		e.insertBefore(declStart(e, firstTest), update+"\n")
	} else {
		e.appendText("\n" + update + "\n")
	}

	// the test opens a page and presses the route's button; which page has it is the author's to say
	tb := newTestBuilder(p, "App", imports)
	open := "?" + LowerFirst(name) + "Page"
	for _, r := range p.Routes {
		if r.Write() || r.IsFile || r.Module == "App" || len(r.Keys) > 0 || len(r.Roles) > 0 || r.Silo != "" {
			continue
		}
		if slices.ContainsFunc(r.Params, func(par *Param) bool { return !par.HasDefault }) {
			continue
		}
		open = pageLit(r.Ref("App"), nil)
		break
	}
	press := name
	if len(args) > 0 {
		press = "(" + name + " { " + strings.Join(args, ", ") + " })"
	}
	lines := append(tb.lines, "let _ <- Test.get ("+open+")", "let r <- Test.click "+press)
	test := TestData{Name: quote("pressing " + name), Lines: lines, Result: "r.answer == Refreshed",
		Comment: []string{name + " is sent from any page (`click={App." + name + "}`): put an `al-button` with it in the view of",
			"the page this test opens, or open the page that has it. Until a page does, the click finds no control."}}
	if err := addTests(e, app, []TestData{test}); err != nil {
		return nil, err
	}
	addImports(e, p, app.Syntax, imports.list())
	return []Edit{{Path: app.File, Body: e.result()}}, nil
}

// newAppMsg is `alder new msg App <Name> [fields]`: `msg <Name> { … }` in App, an arm of its
// `update` whose body is the typed hole `?<name>` : Task Nav (the `update` created when App has
// none, as `new auth` does for Logout), and a test that calls `update` with the message and a
// session, ending in the hole `?expected`. A message of App is sent by a button on any page
// (`click={App.<Name>}`), so no page's view is touched.
func newAppMsg(p *Program, name string, fields []FieldSpec) ([]Edit, error) {
	app := p.Modules["App"]
	if app == nil {
		return nil, Refusef("the app has no `App` module")
	}
	if !IsUpperIdent(name) {
		return nil, Refusef("`%s` is not a message name: a message is named by a capitalised identifier, such as `ToggleTheme`", name)
	}
	if declared(app.Syntax, name) {
		return nil, Refusef("`App` already declares `%s`: choose another message name", name)
	}
	imports := newImports("App", p.Std)
	var decl, args []string
	seen := map[string]bool{}
	for _, f := range fields {
		if seen[f.Name] {
			return nil, Refusef("field `%s` is given twice", f.Name)
		}
		seen[f.Name] = true
		decl = append(decl, f.Source())
		if err := bareType(p, "App", f.Type); err != nil {
			return nil, err
		}
		typeImports(imports, f.Type, p)
		args = append(args, f.Name+": ?"+LowerFirst(name)+UpperFirst(f.Name))
	}
	d := MsgData{Name: name, Fields: decl}
	app, err := withUpdateMatch(app, name)
	if err != nil {
		return nil, err
	}
	e := newSrcEdit(app.Source)
	arm := name + " p -> ?" + LowerFirst(name) + ","
	line, ind, has, ok := updateArmSpot(e, app.Syntax)
	if !ok {
		return nil, Refusef("`App`'s `update` is not a `match` a new arm can go in: add the arm yourself")
	}
	if has {
		e.insertBefore(declStart(e, valueDecl(app.Syntax, "update")), snippet("msg_decl", d)+"\n")
		e.insertBefore(line, indent(arm, ind))
	} else {
		e.appendText("\n" + snippet("msg_decl", d) + "\n\nupdate : Msg -> Session -> Task Nav\nupdate msg session = match msg {\n" + indent(arm, "  ") + "\n}")
	}

	// the test calls update itself: a message of App has no button of its own to press
	var sd *syntax.SessionDecl
	for _, dc := range app.Syntax.Decls {
		if s, ok := dc.(*syntax.SessionDecl); ok {
			sd = s
		}
	}
	tb := newTestBuilder(p, "App", imports)
	var sess []string
	if p.Session != nil {
		for i, f := range p.Session.Fields {
			v := ""
			if sd != nil && i < len(sd.Fields) && sd.Fields[i].Default != nil {
				v = sliceSpan(app.Syntax.Lines, sd.Fields[i].Default.GetSpan())
			}
			if v == "" {
				v = tb.sample(f.Type, f.Name, 1)
			}
			sess = append(sess, f.Name+": "+v)
		}
	}
	msgValue := name
	if len(args) > 0 {
		msgValue = "(" + name + " { " + strings.Join(args, ", ") + " })"
	}
	lines := append(tb.lines, "let r <- Test.run (update "+msgValue+" "+record(sess)+")")
	test := TestData{Name: quote("pressing " + name), Lines: lines, Result: "?expected",
		Comment: []string{"A message of App is sent from any page (`click={App." + name + "}`), so the test calls update itself.",
			"Fill ?expected with what " + name + " must do, as `{ value: r.value, trace: r.trace } ==",
			"{ value: Ok Rerender, trace: [\"Session.put\"] }`: its answer and the effects it ran."}}
	if err := addTests(e, app, []TestData{test}); err != nil {
		return nil, err
	}
	addImports(e, p, app.Syntax, imports.list())
	return []Edit{{Path: app.File, Body: e.result()}}, nil
}

// record is a record literal of fields: `{ a: 1 }`, or `{}` for none.
func record(fields []string) string {
	if len(fields) == 0 {
		return "{}"
	}
	return "{ " + strings.Join(fields, ", ") + " }"
}
