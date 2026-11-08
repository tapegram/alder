// internal/names/routes_test.go
package names

import (
	"strings"
	"testing"

	"alder/internal/syntax"
)

const todosRoutes = `module Todos

route Index = GET "/" { }
route Row = GET "/todos/{id}" { id: Int }

Index.load : Index.Params -> Session -> Query Int
Index.load params session = Query.succeed 1

Index.view : Int -> Session -> Html
Index.view n session = <ul>{Row.view n session}</ul>

Row.load : Row.Params -> Session -> Query Int
Row.load params session = Query.succeed params.id

Row.view : Int -> Session -> Html
Row.view n session = <a href={Row { id: n }}>row</a>
`

// A route's constructor, its members and its Params resolve in its own module, written with the
// route's name.
func TestResolve_OwnRouteCtorAndMembers(t *testing.T) {
	prog, bag, mods := resolveMods(t, std(t), todosRoutes)
	if len(bag.Items) > 0 {
		t.Fatalf("unexpected: %s", codes(bag))
	}
	m := mods["Todos"]
	wantRef(t, m, "Row", syntax.Ref{Kind: syntax.RefRoute, Module: "Todos", Name: "Row"})
	wantRef(t, m, "Row.view", syntax.Ref{Kind: syntax.RefValue, Module: "Todos", Name: "Row.view"})
	wantRef(t, m, "Index.Params", syntax.Ref{Kind: syntax.RefType, Module: "Todos", Name: "Index.Params"})
	wantRef(t, m, "Row.Params", syntax.Ref{Kind: syntax.RefType, Module: "Todos", Name: "Row.Params"})
	if r := prog.Route("Todos", "Row"); r == nil || r.Path != "/todos/{id}" {
		t.Errorf("Route(Todos, Row) = %+v", r)
	}
	if prog.Route("Todos", "Nope") != nil || prog.Route("Nope", "Row") != nil {
		t.Error("Route of an unknown route or module should be nil")
	}
	if !prog.IsRouteModule("Todos") || prog.IsRouteModule("Nope") {
		t.Error("IsRouteModule")
	}
}

// From another module a route's constructor needs no import; its members do.
func TestResolve_CrossModuleRoute(t *testing.T) {
	home := "module Home\n\nlink : Html\nlink = <a href={Todos.Row { id: 1 }}>row</a>\n"
	list := "module List2\n\nimport Todos\n\nrow : Todos.Row.Params -> Html\nrow p = Todos.Row.view p.id {}\n"
	bare := "module Bare\n\nrow : Int -> Html\nrow n = Todos.Row.view n {}\n"
	_, bag, mods := resolveMods(t, std(t), todosRoutes, home, list, bare)
	wantRef(t, mods["Home"], "Todos.Row", syntax.Ref{Kind: syntax.RefRoute, Module: "Todos", Name: "Row"})
	wantRef(t, mods["List2"], "Todos.Row.view", syntax.Ref{Kind: syntax.RefValue, Module: "Todos", Name: "Row.view"})
	wantRef(t, mods["List2"], "Todos.Row.Params", syntax.Ref{Kind: syntax.RefType, Module: "Todos", Name: "Row.Params"})
	var got []string
	for _, d := range bag.Items {
		got = append(got, d.Span.File+" "+d.Code+":"+d.Message)
		if d.Code == "E0201" && (d.Fix == nil || d.Fix.Replace != "import Todos\n") {
			t.Errorf("E0201 fix: %+v", d.Fix)
		}
	}
	if want := "app/Bare.alder E0201:module `Todos` is not imported"; strings.Join(got, "\n") != want {
		t.Errorf("diagnostics:\n%s\nwant:\n%s", strings.Join(got, "\n"), want)
	}
}

// Inside its module a route is written unqualified; a member the route lacks is reported.
func TestResolve_RouteMisuses(t *testing.T) {
	src := todosRoutes + "\nx : Html\nx = <a href={Todos.Row { id: 1 }}>{Row.nope}</a>\n\ny : Row.Model -> Int\ny m = 1\n"
	_, bag, _ := resolveMods(t, std(t), src)
	got := codes(bag)
	for _, want := range []string{
		"E0002:inside `Todos`, its own names are written unqualified: `Row`",
		"E0002:route `Row` of `Todos` has no `nope`",
		"E0002:route `Row` of `Todos` has no type `Model`",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
}

// A qualified declaration names a route of its module.
func TestResolve_DeclNamesNoRoute(t *testing.T) {
	src := todosRoutes + "\nIndx.update : Int\nIndx.update = 1\n\nZzz.update : Int\nZzz.update = 1\n"
	_, bag, _ := resolveMods(t, std(t), src)
	got := codes(bag)
	for _, want := range []string{
		"E0044:module `Todos` has no route `Indx`; did you mean `Index`?",
		"E0044:module `Todos` has no route `Zzz`",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
	if strings.Contains(got, "`Zzz`; did you mean") {
		t.Errorf("no suggestion expected for Zzz:\n%s", got)
	}
}

// A route named like a standard-library module could not have its members named.
func TestResolve_RouteNamedLikeStdModule(t *testing.T) {
	src := "module Todos\n\nroute List = GET \"/\" { }\n"
	_, bag, _ := resolveMods(t, std(t), src)
	if got, want := codes(bag), "E0044:route `List` is named like the standard-library module `List`: `List.view` would name that module's `view`; rename the route"; got != want {
		t.Errorf("got:\n%s\nwant:\n%s", got, want)
	}
}

// A route constructor is written with its module's full path, like a page: through a shorter
// qualifier it gets the full path as a fix.
func TestResolve_RouteCtorThroughQualifier(t *testing.T) {
	admin := "module Admin.Users\n\nroute Edit = GET \"/users/{id}\" { id: Int }\n"
	home := "module Home\n\nimport Admin.Users\n\nlink : Html\nlink = <a href={Users.Edit { id: 1 }}>{Admin.Users.Edit { id: 2 }}</a>\n"
	_, bag, mods := resolveMods(t, std(t), admin, home)
	if got, want := codes(bag), "E0002:write `Admin.Users.Edit`: a route is written with its module's full path, like a page"; got != want {
		t.Fatalf("got:\n%s\nwant:\n%s", got, want)
	}
	if d := bag.Items[0]; d.Fix == nil || d.Fix.Replace != "Admin.Users.Edit" {
		t.Errorf("fix: %+v", d.Fix)
	}
	wantRef(t, mods["Home"], "Admin.Users.Edit", syntax.Ref{Kind: syntax.RefRoute, Module: "Admin.Users", Name: "Edit"})
}

// A route named like a table of its module, or a constructor of an imported one, is E0044.
func TestResolve_RouteNamedLikeTableOrImportedCtor(t *testing.T) {
	todo := "module Todo\n\ntype Priority = Low | High\n\ntable Todo {\n  id: Int @id,\n}\n\nroute Todo = GET \"/todo\" { }\n"
	other := "module Other\n\nimport Todo\n\nroute High = GET \"/high\" { }\n\nx : Todo.Priority\nx = Low\n"
	_, bag, _ := resolveMods(t, std(t), todo, other)
	got := codes(bag)
	for _, want := range []string{
		"E0044:route `Todo` is named like the table `Todo` (of `Todo`): `Todo { … }` cannot mean both; rename the route",
		"E0044:route `High` is named like the constructor `High` (of `Todo.Priority`): `High { … }` cannot mean both; rename the route",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
}
