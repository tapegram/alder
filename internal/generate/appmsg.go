package generate

import (
	"fmt"
	"slices"
	"strings"

	"alder/internal/syntax"
)

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
