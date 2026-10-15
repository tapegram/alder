package generate

import (
	"strings"

	"alder/internal/syntax"
)

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
	e := newSrcEdit(app.Source)
	arm := name + " p -> ?" + LowerFirst(name) + ","
	line, ind, has, ok := updateArmSpot(e, app.Syntax)
	if !ok {
		return nil, Refusef("`App`'s `update` is not `match msg { … }` written over several lines: add the arm yourself")
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
	addImports(e, app.Syntax, imports.list())
	return []Edit{{Path: app.File, Body: e.result()}}, nil
}

// record is a record literal of fields: `{ a: 1 }`, or `{}` for none.
func record(fields []string) string {
	if len(fields) == 0 {
		return "{}"
	}
	return "{ " + strings.Join(fields, ", ") + " }"
}
