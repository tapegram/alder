package check

import (
	"slices"
	"strings"
	"testing"

	"alder/internal/types"
)

// TestRoutes_Info reads routes-ok's RouteInfo: each route's method and path in declaration
// order, its Params (a form route's with `form`), its narrowed Session (shared by the routes
// that narrow the same fields), a GET route's Model and File.
func TestRoutes_Info(t *testing.T) {
	res, bag := runDirResult(t, "../../testdata/corpus/routes-ok")
	if got := render(bag); got != "" {
		t.Fatalf("routes-ok should check clean:\n%s", got)
	}
	rs := res.Routes["Todos"]
	var got []string
	for _, r := range rs {
		got = append(got, r.Method+" "+r.Path+" "+r.Name)
	}
	want := []string{"GET / Index", "GET /todos/{id} Row", "PATCH /todos/{id}/toggle Toggle", "POST /todos Create", "DELETE /todos/{id} Delete", "GET /export.csv Export"}
	if !slices.Equal(got, want) {
		t.Fatalf("routes = %q, want %q", got, want)
	}
	byName := map[string]*RouteInfo{}
	for _, r := range rs {
		byName[r.Name] = r
	}
	index, row, create, export, toggle := byName["Index"], byName["Row"], byName["Create"], byName["Export"], byName["Toggle"]
	if m, ok := index.Model.(*types.Record); !ok || m.Name != "TodoList" {
		t.Errorf("Index.Model = %v, want TodoList", index.Model)
	}
	if m, ok := row.Model.(*types.Record); !ok || m.Name != "Todo" {
		t.Errorf("Row.Model = %v, want Todo", row.Model)
	}
	if !export.File || index.File || types.String(export.Model) != "List Table.Todo.Todo" {
		t.Errorf("File: Export %v (model %s), Index %v; want Export a file route of List Todo", export.File, types.String(export.Model), index.File)
	}
	if toggle.Model != nil || toggle.NeedsShown {
		t.Errorf("Toggle: Model %v, NeedsShown %v; want nil, false", toggle.Model, toggle.NeedsShown)
	}
	if index.Params.Module != "Todos" || index.Params.Name != "Index.Params" || len(index.Params.Fields) != 1 || index.Params.Fields[0].Name != "filter" {
		t.Errorf("Index.Params = %s %+v", index.Params.Name, index.Params.Fields)
	}
	if f := create.Params.Fields; len(f) != 1 || f[0].Name != "form" || f[0].Type.(*types.Record).Name != "NewTodo" {
		t.Errorf("Create.Params fields = %+v, want form: NewTodo", f)
	}
	if index.Session == nil || index.Session.Name != "Index.Session" || !slices.Equal(index.Requires, []string{"user"}) {
		t.Fatalf("Index.Session = %v, Requires %v", index.Session, index.Requires)
	}
	for _, r := range rs {
		if r.Session != index.Session {
			t.Errorf("%s.Session is not Index's: routes narrowing the same fields share one", r.Name)
		}
	}
	for _, f := range index.Session.Fields {
		if f.Name == "user" && types.String(f.Type) != "Table.User.UserId" {
			t.Errorf("Index.Session.user = %s, want it narrowed to UserId", types.String(f.Type))
		}
	}
	if res.Decls["Todos.Row.Params"] != row.Params || res.Decls["Todos.Row.Session"] != index.Session {
		t.Errorf("Row.Params and Row.Session are not the module's declared types")
	}
	var arms []string
	for _, a := range res.Arms["Todos"] {
		arms = append(arms, a.Msg)
	}
	if !slices.Equal(arms, []string{"Toggle", "Create", "Delete"}) {
		t.Errorf("Arms[Todos] = %q, want the write routes", arms)
	}
}

// routeApp is an app whose App has a session with a `Maybe` user, and the module Todos (src).
func routeApp(src string) map[string]string {
	return map[string]string{
		"App.alder": "module App\n\nimport Table.User\n\nsession { user: Maybe User.UserId }\n\nlogin = Login\n\n" +
			"error : Error -> Html Never\nerror err = <p>error</p>\n\nnotFound : Html Never\nnotFound = <p>not found</p>\n",
		"Login.alder": "module Login\n\npage \"/login\" { }\n\ntype Model = { }\n\nload : Params -> Session -> Query Model\nload params session = Query.succeed { }\n\n" +
			"view : Model -> Session -> Html Msg\nview model session = <p>log in</p>\n",
		"Table/User.alder": "module Table.User\n\ntype UserId = brand Text\n\ntable User {\n  id: UserId @id,\n}\n",
		"Table/Note.alder": "module Table.Note\n\nimport Table.User\n\ntype NoteId = brand Text\n\ntable Note {\n  id: NoteId @id,\n  owner: User.UserId @ref User @owner,\n}\n",
		"Todos.alder":      src,
	}
}

// getRoute is route name's `load` and `view` over an Int model, with body view for the view.
func getRoute(name, view string) string {
	return name + ".load : " + name + ".Params -> Session -> Query Int\n" + name + ".load params session = Query.succeed 1\n\n" +
		name + ".view : Int -> Session -> Html\n" + name + ".view n session = " + view + "\n\n"
}

func hasError(errs []string, code, text string) bool {
	for _, e := range errs {
		if strings.HasPrefix(e, code+" ") && strings.Contains(e, text) {
			return true
		}
	}
	return false
}

// TestRoutes_MembersChecked: route members are values under their full names, so two routes'
// `load`s do not clash, and every member's body is checked against its signature.
func TestRoutes_MembersChecked(t *testing.T) {
	_, errs := checkFiles(t, routeApp("module Todos\n\nroute A = GET \"/a\" { }\nroute B = GET \"/b\" { }\n\n"+
		getRoute("A", "<p>{n}</p>")+getRoute("B", "<p>b</p>")))
	if len(errs) != 1 || !hasError(errs, "E0003", "in `{…}`, got Int") {
		t.Fatalf("errors = %q, want only A.view's mismatch", errs)
	}
}

// TestRoutes_Session: `Session` in a member's signature is its route's, narrowed by its
// `requires` (plan call 4); a route without `requires` sees App's.
func TestRoutes_Session(t *testing.T) {
	src := "module Todos\n\nimport Table.User\n\nroute Mine = GET \"/mine\" { } requires user\nroute Open = GET \"/open\" { }\n\n" +
		"Mine.load : Mine.Params -> Session -> Query User.UserId\nMine.load params session = Query.succeed session.user\n\n" +
		"Mine.view : User.UserId -> Session -> Html\nMine.view u session = <p>mine</p>\n\n" +
		"Open.load : Open.Params -> Session -> Query User.UserId\nOpen.load params session = Query.succeed session.user\n\n" +
		"Open.view : User.UserId -> Session -> Html\nOpen.view u session = <p>open</p>\n\n" +
		"widened : Mine.Session -> Session\nwidened s = s\n"
	_, errs := checkFiles(t, routeApp(src))
	if len(errs) != 1 || !hasError(errs, "E0003", "Maybe") || !strings.Contains(errs[0], "Todos.alder:15:") {
		t.Fatalf("errors = %q, want one mismatch, in Open.load (App's session has a Maybe user)", errs)
	}
}

// TestRoutes_Scoped: a scoped operation is narrowed per route: in a member of a route that
// `requires user`, or in a helper of the module (or another route's member) that only such
// members use.
func TestRoutes_Scoped(t *testing.T) {
	src := "module Todos\n\nimport Table.Note\n\nroute Mine = GET \"/mine\" { } requires user\nroute Open = GET \"/open\" { }\n\n" +
		"count : Query Int\ncount = Db.mine Note |> Db.count\n\n" +
		"Mine.load : Mine.Params -> Session -> Query Int\nMine.load params session = count\n\n" +
		"Mine.view : Int -> Session -> Html\nMine.view n session = <p>mine</p>\n\n" +
		"Open.load : Open.Params -> Session -> Query Int\nOpen.load params session = count\n\n" +
		"Mine.total : Query Int\nMine.total = Db.mine Note |> Db.count\n\n" +
		"Open.view : Int -> Session -> Html\nOpen.view n session = <p>open</p>\n\n" +
		"Open.total : Query Int\nOpen.total = Mine.total\n\n" +
		"unused : Query Int\nunused = Db.mine Note |> Db.count\n"
	_, errs := checkFiles(t, routeApp(src))
	if !hasError(errs, "E0026", "`count` uses `Db.mine`, which needs a route that `requires user`: `Open` does not") {
		t.Errorf("errors = %q, want Open.load's use of count refused", errs)
	}
	if !hasError(errs, "E0026", "`Mine.total` uses `Db.mine`, which needs a route that `requires user`: `Open` does not") {
		t.Errorf("errors = %q, want Open.total's use of Mine.total refused", errs)
	}
	if len(errs) != 2 {
		t.Errorf("errors = %q, want only Open's two uses", errs)
	}
}

// TestRoutes_HtmlNoArgument: `Html` with no argument is `Html Route` in a route module (plan
// call 2), and still needs its argument elsewhere.
func TestRoutes_HtmlNoArgument(t *testing.T) {
	files := routeApp("module Todos\n\nimport Shared\n\nroute A = GET \"/a\" { }\n\n" + getRoute("A", "<button click={A {}}>{Shared.box}</button>"))
	files["Shared.alder"] = "module Shared\n\nbox : Html a\nbox = <p>box</p>\n\nbad : Html\nbad = <p>bad</p>\n"
	_, errs := checkFiles(t, files)
	if len(errs) != 1 || !hasError(errs, "E0008", "`Html` takes 1 type argument, got 0") || !strings.Contains(errs[0], "Shared.alder") {
		t.Fatalf("errors = %q, want only Shared's `Html` without its argument", errs)
	}
}

// TestRoutes_SelectOptions: `<Select options>` is written in a GET route's `view` (plan call 11),
// from its parameters only; in a helper of a route module it is refused, as in a page's.
func TestRoutes_SelectOptions(t *testing.T) {
	src := "module Todos\n\nroute A = GET \"/a\" { }\nroute Pick = POST \"/pick\" { } form Choice\n\nform Choice = { list: Text }\n\n" +
		"opts : List { value: Text, label: Text }\nopts = [{ value: \"a\", label: \"A\" }]\n\n" +
		"Picker : { n: Int } -> Html\nPicker props = <form submit={Pick}><Errors form /><Select field=\"list\" options={opts} /></form>\n\n" +
		getRoute("A", "<div><form submit={Pick}><Errors form /><Select field=\"list\" options={[{ value: Text.fromInt n, label: \"N\" }]} /></form>"+
			"{for x in [1] -> <form submit={Pick}><Errors form /><Select field=\"list\" options={[{ value: Text.fromInt x, label: \"X\" }]} /></form>}"+
			"<Picker n={n} /></div>") +
		"Pick.update : Pick.Params -> Session -> Task Response\nPick.update params session = Task.succeed Response.refresh\n"
	_, errs := checkFiles(t, routeApp(src))
	if !hasError(errs, "E0304", "`options` must be written in a GET route's `view`") {
		t.Errorf("errors = %q, want the helper's options refused", errs)
	}
	if !hasError(errs, "E0304", "`options` may use only the view's parameters (`model`, `session`): `x` is bound inside the view") {
		t.Errorf("errors = %q, want the `for` variable refused", errs)
	}
	if len(errs) != 2 {
		t.Errorf("errors = %q, want only those two: options from the view's parameters are fine", errs)
	}
}

// TestRoutes_Steps: the browser steps' rules on routes beyond the corpus's: a form route is
// submitted, not clicked; a file route is fetched; a route's form is posted by naming the route.
func TestRoutes_Steps(t *testing.T) {
	src := "module Todos\n\nroute A = GET \"/a\" { }\nroute Csv = GET \"/a.csv\" { }\nroute Add = POST \"/add\" { } form NewTodo\n\nform NewTodo = { title: Text }\n\n" +
		getRoute("A", "<p>a</p>") +
		"Csv.load : Csv.Params -> Session -> Query Int\nCsv.load params session = Query.succeed 1\n\n" +
		"Csv.view : Int -> Session -> File\nCsv.view n session = File.text { contentType: \"text/csv\", body: \"\" }\n\n" +
		"Add.update : Add.Params -> Session -> Task Response\nAdd.update params session = Task.succeed Response.refresh\n\n" +
		"test \"steps\" = {\n  let a <- Test.click Add\n  let b <- Test.get (Csv {})\n  let c <- Test.submit NewTodo { title: \"x\" }\n  let d <- Test.submit (A {}) { title: \"x\" }\n  Task.succeed True\n}\n"
	_, errs := checkFiles(t, routeApp(src))
	for _, want := range [][2]string{
		{"E0045", "`Test.click` sends a route without a form; `Add` posts form `NewTodo`: use `Test.submit Add { … }`"},
		{"E0003", "`Csv` is a file route: use `Test.fetch`"},
		{"E0003", "`Test.submit` names the route that posts a form: write `Test.submit Add …`"},
		{"E0045", "`Test.submit` posts a route's form; `A` is GET: use `Test.get`"},
	} {
		if !hasError(errs, want[0], want[1]) {
			t.Errorf("errors = %q, want %s %s", errs, want[0], want[1])
		}
	}
	if len(errs) != 4 {
		t.Errorf("errors = %q, want only those four", errs)
	}
}

// TestNeedsShown: a write route needs the page it was sent from rendered again (plan call 12)
// when its `update` mentions Response.refresh or Response.invalid, or calls an app function
// whose type's result contains Response; the function is judged by its type, not its name.
func TestNeedsShown(t *testing.T) {
	cases := []struct {
		name, body string
		want       bool
	}{
		{"render", "Task.succeed (Response.render (Row {}))", false},
		{"redirect", "Task.succeed (Response.redirect (Row {}))", false},
		{"refresh in one branch", "if params.flag then Task.succeed (Response.redirect (Row {})) else Task.succeed Response.refresh", true},
		{"invalid through toast", "Task.succeed (Response.invalid \"x\" |> Response.toast \"y\")", true},
		{"helper returning Response", "Task.succeed (answer params.flag)", true},
		{"helper returning Task Response", "later params.flag", true},
		{"helper with a Response in a Maybe", "Task.succeed (Maybe.withDefault (Response.redirect (Row {})) (pick params.flag))", true},
		{"Text.trim", "Task.succeed (if Text.trim \"x\" == \"\" then Response.redirect (Row {}) else Response.render (Row {}))", false},
		{"helper not returning Response", "Task.succeed (if same params.flag then Response.redirect (Row {}) else Response.redirect (Row {}))", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			src := "module Todos\n\nroute Row = GET \"/row\" { }\nroute Go = POST \"/go\" { flag: Bool }\n\n" + getRoute("Row", "<p>r</p>") +
				"answer : Bool -> Response\nanswer b = Response.refresh\n\n" +
				"later : Bool -> Task Response\nlater b = Task.succeed Response.refresh\n\n" +
				"pick : Bool -> Maybe Response\npick b = Nothing\n\n" +
				"same : Bool -> Bool\nsame b = b\n\n" +
				"Go.update : Go.Params -> Session -> Task Response\nGo.update params session = " + tc.body + "\n"
			res, errs := checkFiles(t, routeApp(src))
			if len(errs) != 0 {
				t.Fatalf("errors = %q", errs)
			}
			var got *RouteInfo
			for _, r := range res.Routes["Todos"] {
				if r.Name == "Go" {
					got = r
				}
			}
			if got == nil || got.NeedsShown != tc.want {
				t.Errorf("Go.NeedsShown = %v, want %v", got != nil && got.NeedsShown, tc.want)
			}
		})
	}
}
