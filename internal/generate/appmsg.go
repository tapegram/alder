package generate

import (
	"fmt"
	"strings"

	"alder/internal/check"
	"alder/internal/syntax"
)

// newAppRouteMsg is `alder new action App <Name> [fields]` on an App written with routes (plan call
// 13): `route <Name> = <METHOD> "<path>" { fields }` (POST unless method says otherwise; the path
// is --path, which App needs, as it has no page to take one from) after App's last route (else after its
// `login`, else after its `session`), and `<Name>.update` answering `Response.refresh`, with a
// comment saying how to test it. It writes no test: a route of App is sent by a button on any
// page (`click={App.<Name>}`), so no page's view is touched, and which page has the button is the
// author's to say.
func newAppRouteMsg(p *Program, name string, fields []FieldSpec, path, method string) ([]Edit, error) {
	app := p.Modules["App"]
	if !IsUpperIdent(name) {
		return nil, Refusef("`%s` is not a route name: a route is named by a capitalised identifier, such as `ToggleTheme`", name)
	}
	if declared(app.Syntax, name) {
		return nil, Refusef("`App` already declares `%s`: choose another route name", name)
	}
	if why := check.PagePathProblem(path); why != "" {
		return nil, Refusef("`%s` is not a route path: it %s; %s", path, why, check.PagePathRule)
	}
	if method == "" {
		method = "POST"
	}
	if have := routeClash(p, method, path, ""); have != nil {
		return nil, clashRefusal(have, "App", name, method, path, false)
	}
	imports := newImports("App", p.Std)
	var decl []string
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
	}
	line := fmt.Sprintf("route %s = %s \"%s\" { }", name, method, path)
	if len(decl) > 0 {
		line = fmt.Sprintf("route %s = %s \"%s\" { %s }", name, method, path, strings.Join(decl, ", "))
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
	// no test: the route is pressed from a page that has its button, which the app may not have yet
	press := "Test.click App." + name
	if len(fields) > 0 {
		press = "Test.click (App." + name + " { … })"
	}
	comment := fmt.Sprintf("-- Press %s from a page that has its button to test it: `Test.get` that page, then\n-- `%s`, and `r.answer` is Refreshed until `update` says otherwise.\n", name, press)
	update := comment + fmt.Sprintf("%s.update : %s.Params -> Session -> Task Response\n%s.update params session = Task.succeed Response.refresh", name, name, name)
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

	addImports(e, p, app.Syntax, imports.list())
	return []Edit{{Path: app.File, Body: e.result()}}, nil
}
