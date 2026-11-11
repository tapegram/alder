package rtesting

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"testing"

	"alder/rt"
	"alder/rt/db"
	"alder/rt/eff"
	"alder/rt/html"
	"alder/rt/web"
)

// The fixture app: what the generated code of a small route app would be, written by hand. Its
// session is { user: Text }; GET / (Index, `requires user`, parameter filter = "All") lists T's
// rows with a click of Index.Delete (DELETE /rows/{id}) each, a NewRow form (title, note) sending
// Index.Add (POST /rows), and clicks of Index.Fail, Index.Boom, App.Logout, Index.Away and
// Index.Hash; GET /login has a form sending Login.Submit (POST /login), which logs the email in
// and redirects to "/".

type session struct{ User string }

type codec struct{}

func (codec) Default() any                 { return session{} }
func (codec) Encode(v any) ([]byte, error) { return json.Marshal(v.(session)) }
func (codec) Decode(b []byte) (any, error) {
	var s session
	err := json.Unmarshal(b, &s)
	return s, err
}

func layout(title string, body ...html.Node) html.Node {
	return html.El("html", nil,
		html.El("head", nil, html.El("meta", []html.Attr{html.A("charset", "utf-8")}), html.El("title", nil, html.Text(title))),
		html.El("body", nil, html.El("main", nil, body...)))
}

const newRow = "Index.NewRow"

// The fixture's write routes.
var (
	iAdd     = rt.Route{Name: "Index.Add", Method: "POST", Path: "/rows"}
	iFail    = rt.Route{Name: "Index.Fail", Method: "POST", Path: "/fail"}
	iBoom    = rt.Route{Name: "Index.Boom", Method: "POST", Path: "/boom"}
	iAway    = rt.Route{Name: "Index.Away", Method: "POST", Path: "/go-away"}
	iHash    = rt.Route{Name: "Index.Hash", Method: "POST", Path: "/to-hash"}
	iLogout  = rt.Route{Name: "App.Logout", Method: "POST", Path: "/logout"}
	iLogin   = rt.Route{Name: "Login.Submit", Method: "POST", Path: "/login"}
	iToIndex = rt.Route{Name: "Index", Method: "GET", Path: "/"}
	iToLogin = rt.Route{Name: "Login", Method: "GET", Path: "/login"}
)

func iDelete(id string) rt.Route {
	return rt.Route{Name: "Index.Delete", Method: "DELETE", Path: "/rows/" + rt.PathSegment(id)}
}

func clickOf(r rt.Route, label string) html.Node {
	return html.El("button", []html.Attr{html.RouteAttr("data-on:click", r)}, html.Text(label))
}

func indexView(filter string, s session, rows []row) html.Node {
	var items []html.Node
	for _, r := range rows {
		items = append(items, html.El("li", []html.Attr{html.A("data-id", r.ID)}, html.Text(r.Title), clickOf(iDelete(r.ID), "Delete")))
	}
	return layout("Index",
		html.El("p", []html.Attr{html.A("id", "user")}, html.Text(s.User)),
		html.El("p", []html.Attr{html.A("id", "filter")}, html.Text(filter)),
		html.El("p", []html.Attr{html.A("id", "summary")}, html.Text(strconv.Itoa(len(rows))+" rows")),
		html.El("ul", []html.Attr{html.A("id", "rows")}, items...),
		html.FormToRoute(iAdd, nil,
			html.Input(html.Control{Kind: "errors", Form: newRow}),
			html.Input(html.Control{Kind: "input", Form: newRow, Name: "title", Label: "Title", Required: true}),
			html.Input(html.Control{Kind: "errors", Form: newRow, Name: "title"}),
			html.Input(html.Control{Kind: "input", Form: newRow, Name: "note", Label: "Note"}),
			html.El("button", nil, html.Text("Add"))),
		clickOf(iFail, "Fail"),
		clickOf(iBoom, "Boom"),
		clickOf(iLogout, "Log out"),
		clickOf(iAway, "Away"),
		clickOf(iHash, "Hash"),
		html.El("a", []html.Attr{html.A("href", "/?filter=Done")}, html.Text("Done")),
	)
}

func loginView() html.Node {
	return layout("Log in", html.FormToRoute(iLogin, nil,
		html.Input(html.Control{Kind: "errors", Form: "Login.Form"}),
		html.Input(html.Control{Kind: "input", Form: "Login.Form", Name: "email", Type: "email", Label: "Email"}),
		html.Input(html.Control{Kind: "errors", Form: "Login.Form", Name: "email"})))
}

// filterOf decodes the page's parameter from a GET's query.
func filterOf(q url.Values) string {
	if f := q.Get("filter"); f != "" {
		return f
	}
	return "All"
}

// loadIndex is the page's load, a Query (Db.all T).
func loadIndex(rq *web.Request) ([]row, error) {
	return eff.RunQuery(rq.Env, eff.Q(eff.Call("Db.all T", false, func(e *eff.Env) ([]row, error) {
		return db.All(e.Ctx, e.Store.(db.Store), db.TxOf(e), db.OrderBy(db.Asc("title"), db.From(tb)))
	})))
}

func fail(err error) web.Response {
	if errors.Is(err, eff.ErrNotFound) {
		return web.NotFoundResponse(html.None())
	}
	return web.ErrorResponse(html.None(), err)
}

// update runs task, a write route's update, and answers as a generated write route does: the
// page the browser is on rendered before it (RenderShown, under Datastar), then `refresh`, or
// `invalid t` with the form's state st when the task gives a text t.
func update(rq *web.Request, task eff.Task[string], st *html.FormState) web.Response {
	var before *web.Shown
	if rq.Datastar {
		before, _ = rq.RenderShown(nil)
	}
	text, err := eff.Run(rq.Env, eff.KTask, task)
	if err != nil {
		return fail(err)
	}
	if text != "" {
		resp := web.AnswerResponse(rt.Invalid(text), before, rq.Env.Session)
		resp.State = st
		return resp
	}
	return web.AnswerResponse(rt.Refresh(), before, rq.Env.Session)
}

// invalidForm is a write route's answer to a form that did not decode: st re-rendered on the
// page the browser is on.
func invalidForm(rq *web.Request, st *html.FormState) web.Response {
	resp := web.InvalidResponse(st, nil)
	if rq.Datastar {
		resp.Shown, _ = rq.RenderShown(nil)
	}
	return resp
}

func insertTask(title string) eff.Task[string] {
	return eff.Call("Db.insert T", true, func(e *eff.Env) (string, error) {
		return db.InsertRow(e.Ctx, e.Store.(db.Store), db.TxOf(e), tb, []any{nil, title}, e.Now())
	})
}

func fixtureRoutes() []web.Route {
	return []web.Route{
		{Method: "GET", Pattern: "/", Handle: func(rq *web.Request) web.Response {
			s := rq.Session.(session)
			if s.User == "" {
				return web.RedirectResponse("/login", nil)
			}
			rows, err := loadIndex(rq)
			if err != nil {
				return fail(err)
			}
			return web.PageResponse(indexView(filterOf(rq.Query), s, rows), rq.FormState, nil)
		}},
		{Method: "GET", Pattern: "/login", Handle: func(rq *web.Request) web.Response {
			return web.PageResponse(loginView(), rq.FormState, nil)
		}},
		{Method: "GET", Pattern: "/old", Handle: func(rq *web.Request) web.Response { return web.RedirectResponse("/?filter=Old", nil) }},
		{Method: "GET", Pattern: "/loop", Handle: func(rq *web.Request) web.Response { return web.RedirectResponse("/loop", nil) }},
		{Method: "GET", Pattern: "/gone", Handle: func(rq *web.Request) web.Response { return web.RedirectResponse("/nope", nil) }},
		{Method: "GET", Pattern: "/away", Handle: func(rq *web.Request) web.Response {
			return web.RedirectResponse("https://other.example/login?next=%2F", nil)
		}},
		{Method: "GET", Pattern: "/p/{slug}", Handle: func(rq *web.Request) web.Response {
			return web.ErrorResponse(html.None(), errors.New("bad slug"))
		}},
		{Method: "GET", Pattern: "/hash", Handle: func(rq *web.Request) web.Response { return web.RedirectResponse("#top", nil) }},
		{Method: "POST", Pattern: "/to-hash", Handle: func(rq *web.Request) web.Response {
			return web.RedirectResponse("#", rq.Env.Session) // a handler's redirect to Url.external "javascript:…"
		}},
		{Method: "POST", Pattern: "/go-away", Handle: func(rq *web.Request) web.Response {
			return web.AnswerResponse(rt.Redirect(rt.Route{Method: "GET", Path: "https://other.example/login"}), nil, rq.Env.Session)
		}},
		{Method: "GET", Pattern: "/broken", Handle: func(rq *web.Request) web.Response {
			_, err := eff.Run(rq.Env, eff.KTask, then(insertTask("broken"), eff.Fail[int]("kaput")))
			return fail(err)
		}},
		{Method: "POST", Pattern: "/login", Handle: func(rq *web.Request) web.Response {
			email := rq.Form.Get("email")
			if email == "" {
				return invalidForm(rq, &html.FormState{Form: "Login.Form", Msg: rt.RouteKey(iLogin),
					Values: map[string]string{"email": ""}, Errors: map[string][]string{"email": {"is required"}}})
			}
			return web.AnswerResponse(rt.Redirect(iToIndex), nil, session{User: email})
		}},
		{Method: "POST", Pattern: "/rows", Handle: func(rq *web.Request) web.Response {
			values := map[string]string{"title": rq.Form.Get("title"), "note": rq.Form.Get("note")}
			key := rt.RouteKey(iAdd)
			if values["title"] == "" {
				return invalidForm(rq, &html.FormState{Form: newRow, Msg: key, Values: values,
					Errors: map[string][]string{"title": {"must not be empty"}}})
			}
			st := &html.FormState{Form: newRow, Msg: key, Values: values}
			if values["title"] == "dup" {
				return update(rq, eff.Pure("that title is taken"), st)
			}
			return update(rq, eff.Map(func(string) string { return "" }, insertTask(values["title"])), st)
		}},
		{Method: "DELETE", Pattern: "/rows/{id}", Handle: func(rq *web.Request) web.Response {
			id := rq.Params["id"]
			return update(rq, eff.Call("Db.delete T", true, func(e *eff.Env) (string, error) {
				m, err := db.GetRow(e.Ctx, e.Store.(db.Store), db.TxOf(e), tb, id)
				if err != nil {
					return "", err
				}
				if !m.Ok {
					return "", eff.ErrNotFound
				}
				return "", db.DeleteRow(e.Ctx, e.Store.(db.Store), db.TxOf(e), tb, id)
			}), nil)
		}},
		{Method: "POST", Pattern: "/fail", Handle: func(rq *web.Request) web.Response {
			return update(rq, then(insertTask("failed"), eff.Fail[string]("nope")), nil)
		}},
		{Method: "POST", Pattern: "/boom", Handle: func(rq *web.Request) web.Response {
			return update(rq, then(insertTask("boom"), eff.Call("boom", false, func(*eff.Env) (string, error) { panic("boom") })), nil)
		}},
		{Method: "POST", Pattern: "/logout", Handle: func(rq *web.Request) web.Response {
			return web.AnswerResponse(rt.Redirect(iToLogin), nil, session{})
		}},
		{Method: "POST", Pattern: "/to", Handle: func(rq *web.Request) web.Response {
			return web.RedirectResponse(rq.Query.Get("to"), nil)
		}},
		{Method: "GET", Pattern: "/r.csv", File: true, Handle: func(rq *web.Request) web.Response {
			return web.FileResponse(rt.File{ContentType: "text/csv", Headers: [][2]string{{"Content-Disposition", "inline"}}, Body: []byte("a,b\n")}, nil)
		}},
	}
}

// post sends a write to target as a click with no form of the page's script would (a
// Datastar POST with the script's empty JSON payload and headers), whether or not the held page
// offers a control for it, and applies its answer.
func post(b *Browser, target string) (Answer, error) {
	return b.sendRoute(routeCtl{method: http.MethodPost, route: http.MethodPost, url: target}, rt.Route{}, nil)
}

func fixtureWeb() web.Options {
	return web.Options{
		Session:     codec{},
		Login:       "/login",
		NotFoundDoc: func() html.Node { return layout("Not found", html.El("h1", nil, html.Text("Not found"))) },
		ErrorDoc:    func(error) html.Node { return layout("Error", html.El("h1", nil, html.Text("Something went wrong"))) },
	}
}

// browse runs steps as one task test against the fixture (through Run, so BrowserOf is the
// case's browser) and returns its result. Each step gets the case's browser and its Env.
func browse(t *testing.T, steps ...func(b *Browser, e *eff.Env)) CaseResult {
	t.Helper()
	var task eff.Task[bool] = eff.Pure(true)
	for i := len(steps) - 1; i >= 0; i-- {
		f := steps[i]
		rest := task
		task = eff.Bind(eff.Call("step", true, func(e *eff.Env) (bool, error) {
			f(BrowserOf(e), e)
			return true, nil
		}), func(bool) eff.Task[bool] { return rest })
	}
	rs := Run([]Case{{Module: "M", Name: "browse", Task: func() eff.Task[bool] { return task }}},
		Options{Tables: tables, Routes: fixtureRoutes(), Web: fixtureWeb(), Session: session{}})
	if !rs[0].OK {
		t.Fatalf("the test failed: %s", rs[0].Error)
	}
	return rs[0]
}

func text(t *testing.T, doc html.Node, sel string) string {
	t.Helper()
	ns := html.Select(doc, html.MustParseSelector(sel))
	if len(ns) == 0 {
		t.Errorf("no %s in %s", sel, html.Render(doc))
		return ""
	}
	return html.TextContent(ns[0])
}

// mustOf is a step's result, failing t when the step had an error.
func mustOf(t *testing.T) func(Result, error) Result {
	return func(r Result, err error) Result {
		t.Helper()
		if err != nil {
			t.Errorf("step: %v", err)
		}
		return r
	}
}

func wantAnswer(t *testing.T, r Result, want Answer) {
	t.Helper()
	if !reflect.DeepEqual(r.Answer, want) {
		t.Errorf("answer = %+v, want %+v\n%s", r.Answer, want, html.Render(r.Doc))
	}
}

func insertDirect(t *testing.T, e *eff.Env, title string) string {
	t.Helper()
	tx, err := e.Transaction()
	if err != nil {
		t.Fatal(err)
	}
	id, err := db.InsertRow(e.Ctx, e.Store.(db.Store), tx.(db.Tx), tb, []any{nil, title}, e.Now())
	if err != nil {
		t.Error(err)
	}
	return id
}

func titles(t *testing.T, e *eff.Env) []string {
	t.Helper()
	tx, _ := e.Transaction()
	rs, err := db.All(e.Ctx, e.Store.(db.Store), tx.(db.Tx), db.OrderBy(db.Asc("title"), db.From(tb)))
	if err != nil {
		t.Error(err)
	}
	out := []string{}
	for _, r := range rs {
		out = append(out, r.Title)
	}
	return out
}

// TestBrowser_GetLoginAndCookie: a page that requires a user redirects to the login page
// (Get's ruled failure, Open's Redirected); Test.login's cookie is sent back; a submit of the
// login form stores the router's cookie, which the next request carries; logout clears it.
func TestBrowser_GetLoginAndCookie(t *testing.T) {
	browse(t, func(b *Browser, e *eff.Env) {
		must := mustOf(t)
		r, err := b.Get("/")
		var ns *NotShownError
		if !errors.As(err, &ns) || err.Error() != "GET /: redirected to /login" {
			t.Errorf("Get / without login: %v", err)
		}
		ns.Requires = "user"
		if err.Error() != "GET /: redirected to /login (requires user)" {
			t.Errorf("with Requires: %v", err)
		}
		wantAnswer(t, r, Answer{Tag: Redirected, Page: rt.Route{Path: "/login"}})
		if text(t, r.Doc, "title") != "Log in" || r.Session != (session{}) || b.URL() != "/login" {
			t.Errorf("held %s, session %v", b.URL(), r.Session)
		}

		if err := b.Login(session{User: "ann"}); err != nil {
			t.Error(err)
		}
		r = must(b.Get("/"))
		wantAnswer(t, r, Answer{Tag: Shown})
		if text(t, r.Doc, "#user") != "ann" || r.Session != (session{User: "ann"}) {
			t.Errorf("after login: user %q session %v", text(t, r.Doc, "#user"), r.Session)
		}
		// the held page is the page as the view renders it: no doctype, no injected script
		if html.Select(r.Doc, html.MustParseSelector("script")) != nil || strings.Contains(html.Render(r.Doc), "doctype") {
			t.Errorf("held page keeps the router's additions: %s", html.Render(r.Doc))
		}
		if !r.Doc.Equal(b.Doc()) {
			t.Error("Result.Doc is not the held page")
		}

		// through the real login form: the router's Set-Cookie is kept and sent back
		b.Open("/login")
		r = must(b.SubmitRoute(iLogin, "Login.Form", Fields{Values: url.Values{"email": {"bob@example.com"}}}))
		wantAnswer(t, r, Answer{Tag: Redirected, Page: rt.Route{Path: "/"}})
		if r.Session != (session{User: "bob@example.com"}) || text(t, r.Doc, "#user") != "bob@example.com" {
			t.Errorf("after the login form: session %v, page user %q", r.Session, text(t, r.Doc, "#user"))
		}

		// App.Logout through its button
		r = must(b.ClickRoute(iLogout, "App.Logout"))
		wantAnswer(t, r, Answer{Tag: Redirected, Page: rt.Route{Path: "/login"}})
		if r.Session != (session{}) {
			t.Errorf("after logout: %v", r.Session)
		}
	})
}

// TestBrowser_Rejected (Review Focus 1): a submit whose form does not decode holds the
// re-rendered form with the submitted values and the errors; the answer lists them, "" for the
// form-level list; the session is unchanged.
func TestBrowser_Rejected(t *testing.T) {
	browse(t, func(b *Browser, e *eff.Env) {
		must := mustOf(t)
		b.Login(session{User: "ann"})
		must(b.Get("/"))
		r := must(b.SubmitRoute(iAdd, newRow, Fields{Values: url.Values{"title": {""}, "note": {"keep <me>"}}}))
		wantAnswer(t, r, Answer{Tag: FormRejected, Errors: []web.FieldError{{Field: "title", Message: "must not be empty"}}})
		note := html.Select(r.Doc, html.MustParseSelector("input[name=note]"))
		if v, _ := html.AttrOf(note[0], "value"); v != "keep <me>" {
			t.Errorf("the note's value = %q", v)
		}
		if got := text(t, r.Doc, `ul[data-errors=title] > li`); got != "must not be empty" {
			t.Errorf("title errors = %q", got)
		}
		if r.Session != (session{User: "ann"}) || text(t, r.Doc, "#user") != "ann" {
			t.Errorf("session %v", r.Session)
		}
		// the form-level invalid text
		r = must(b.SubmitRoute(iAdd, newRow, Fields{Values: url.Values{"title": {"dup"}}}))
		wantAnswer(t, r, Answer{Tag: FormRejected, Errors: []web.FieldError{{Field: "", Message: "that title is taken"}}})
		if v, _ := html.AttrOf(html.Select(r.Doc, html.MustParseSelector("input[name=title]"))[0], "value"); v != "dup" {
			t.Errorf("title value = %q", v)
		}
		// a good one: Refreshed, the errors gone, the row listed
		r = must(b.SubmitRoute(iAdd, newRow, Fields{Values: url.Values{"title": {"milk"}}}))
		wantAnswer(t, r, Answer{Tag: Refreshed})
		if text(t, r.Doc, "#rows li") != "milkDelete" || text(t, r.Doc, "#summary") != "1 rows" {
			t.Errorf("after add: %s", html.Render(r.Doc))
		}
		if !reflect.DeepEqual(titles(t, e), []string{"milk"}) {
			t.Errorf("rows = %v", titles(t, e))
		}
	})
}

// TestBrowser_ShownPageQueryKept (Review Focus 3): on a page opened with a parameter, a write's
// `refresh` renders the page the browser is on (its URL, sent as Alder-Page), parameter
// included, and the held URL keeps it.
func TestBrowser_ShownPageQueryKept(t *testing.T) {
	browse(t, func(b *Browser, e *eff.Env) {
		must := mustOf(t)
		a := insertDirect(t, e, "a")
		insertDirect(t, e, "b")
		b.Login(session{User: "ann"})
		r := must(b.Get("/?filter=Active"))
		if text(t, r.Doc, "#filter") != "Active" {
			t.Errorf("filter = %q", text(t, r.Doc, "#filter"))
		}
		start := len(e.Trace)
		r = must(b.ClickRoute(iDelete(a), "Index.Delete"))
		wantAnswer(t, r, Answer{Tag: Refreshed})
		if text(t, r.Doc, "#filter") != "Active" || text(t, r.Doc, "#summary") != "1 rows" || b.URL() != "/?filter=Active" {
			t.Errorf("after the click: %s %s", b.URL(), html.Render(r.Doc))
		}
		// the trace: the request's effects (the page before, update, then the page again)
		if got := e.Trace[start:]; !reflect.DeepEqual(got, []string{"Db.all T", "Db.delete T", "Db.all T"}) {
			t.Errorf("trace = %v", got)
		}
		// and a submit on that page too
		r = must(b.SubmitRoute(iAdd, newRow, Fields{Values: url.Values{"title": {"c"}}}))
		wantAnswer(t, r, Answer{Tag: Refreshed})
		if text(t, r.Doc, "#filter") != "Active" {
			t.Errorf("after the submit: filter %q", text(t, r.Doc, "#filter"))
		}
	})
}

// TestBrowser_FailuresKeepTheTestsRows (Review Focus 5): a request whose update fails or
// panics is RequestFailed with its message; its own write is undone, the test's rows stay; steps work
// inside Test.run (Inner), whose failure undoes the steps' writes.
func TestBrowser_FailuresKeepTheTestsRows(t *testing.T) {
	res := browse(t, func(b *Browser, e *eff.Env) {
		must := mustOf(t)
		insertDirect(t, e, "mine")
		b.Login(session{User: "ann"})
		must(b.Get("/"))
		r := must(b.ClickRoute(iFail, "Index.Fail"))
		wantAnswer(t, r, Answer{Tag: RequestFailed, Text: "nope"})
		if text(t, r.Doc, "h1") != "Something went wrong" {
			t.Errorf("held %s", html.Render(r.Doc))
		}
		if r.Session != (session{User: "ann"}) {
			t.Errorf("session %v", r.Session)
		}
		must(b.Get("/"))
		r = must(b.ClickRoute(iBoom, "Index.Boom"))
		wantAnswer(t, r, Answer{Tag: RequestFailed, Text: "panic: boom"})
		if got := titles(t, e); !reflect.DeepEqual(got, []string{"mine"}) {
			t.Errorf("rows after the failures = %v", got)
		}

		// within Test.run: the steps run (nested savepoints) …
		_, _, ok, trace, err := Inner(e, eff.Call("Test.get Index", true, func(ie *eff.Env) (int, error) {
			r := must(BrowserOf(ie).Get("/"))
			must(BrowserOf(ie).SubmitRoute(iAdd, newRow, Fields{Values: url.Values{"title": {"inner"}}}))
			return len(html.Select(r.Doc, html.MustParseSelector("#rows li"))), nil
		}))
		if err != nil || !ok || !reflect.DeepEqual(trace, []string{"Test.get Index", "Db.all T", "Db.all T", "Db.insert T", "Db.all T"}) {
			t.Errorf("Inner: ok %v trace %v err %v", ok, trace, err)
		}
		if got := titles(t, e); !reflect.DeepEqual(got, []string{"inner", "mine"}) {
			t.Errorf("rows after Test.run = %v", got)
		}
		// … and a Test.run that fails after a step undoes the step's write
		_, msg, ok, _, err := Inner(e, then(eff.Call("Test.submit", true, func(ie *eff.Env) (int, error) {
			must(BrowserOf(ie).SubmitRoute(iAdd, newRow, Fields{Values: url.Values{"title": {"undone"}}}))
			return 0, nil
		}), eff.Fail[int]("stop")))
		if err != nil || ok || msg != "stop" {
			t.Errorf("failing Inner: %v %q %v", ok, msg, err)
		}
		if got := titles(t, e); !reflect.DeepEqual(got, []string{"inner", "mine"}) {
			t.Errorf("rows after the failed Test.run = %v", got)
		}
	})
	if len(res.Trace) == 0 || res.Trace[0] != "step" {
		t.Errorf("case trace = %v", res.Trace)
	}
}

// TestBrowser_NoControl: SubmitRoute and ClickRoute act only on controls the held page renders;
// the error names the route as the test wrote it and the parameters the page offers it with.
func TestBrowser_NoControl(t *testing.T) {
	browse(t, func(b *Browser, e *eff.Env) {
		must := mustOf(t)
		_, err := b.ClickRoute(iDelete("zzz"), `Index.Delete { id: "zzz" }`)
		if err == nil || err.Error() != "no page is open: `Test.get` one first" {
			t.Errorf("before any page: %v", err)
		}
		if _, err := b.SubmitRoute(iAdd, "NewRow", Fields{}); err == nil || err.Error() != "no page is open: `Test.get` one first" {
			t.Errorf("submit before any page: %v", err)
		}
		b.Login(session{User: "ann"})
		a := insertDirect(t, e, "a")
		must(b.Get("/"))
		start := len(e.Trace)
		_, err = b.ClickRoute(iDelete("zzz"), `Index.Delete { id: "zzz" }`)
		var nc *NoControlError
		if !errors.As(err, &nc) || nc.Page != "/" || nc.URL != "/rows/zzz" ||
			err.Error() != `the page / has no button or form for Index.Delete { id: "zzz" } (the page has Delete for id: `+a+")" {
			t.Errorf("click: %v", err)
		}
		_, err = b.SubmitRoute(iLogin, "", Fields{})
		if err == nil || err.Error() != "the page / has no form for Login.Submit" {
			t.Errorf("submit: %v", err)
		}
		_, err = b.SubmitRoute(iLogin, "LoginForm", Fields{})
		if err == nil || err.Error() != "the page / has no form LoginForm (Login.Submit)" {
			t.Errorf("submit with Form: %v", err)
		}
		// a click button is not a form to submit
		if _, err := b.SubmitRoute(iFail, "", Fields{}); err == nil {
			t.Error("submitted a click button")
		}
		// a field the form renders no input for is not sent by a browser: the step fails (C1)
		_, err = b.SubmitRoute(iAdd, "NewRow", Fields{Values: url.Values{"title": {"x"}, "colour": {"red"}}})
		if err == nil || err.Error() != "the page's form NewRow has no input `colour`" {
			t.Errorf("a field without an input: %v", err)
		}
		if len(e.Trace) != start {
			t.Errorf("a refused step sent a request: %v", e.Trace[start:])
		}
		// the route with the row's own parameters matches
		r := must(b.ClickRoute(iDelete(a), "Index.Delete"))
		wantAnswer(t, r, Answer{Tag: Refreshed})
	})
}

// TestBrowser_MissingAndFailed: 404 and 500 for a GET hold the served document; a Datastar 404
// (a click on a row gone meanwhile) is PageMissing; redirects are followed, a redirect to a 404 is
// PageMissing, a loop gives up.
func TestBrowser_MissingAndFailed(t *testing.T) {
	browse(t, func(b *Browser, e *eff.Env) {
		must := mustOf(t)
		r := must(b.Open("/nope"))
		wantAnswer(t, r, Answer{Tag: PageMissing})
		if r.Status != 404 || r.URL != "/nope" { // #160: the step's last response and the URL held
			t.Errorf("Status %d, URL %q; want 404, /nope", r.Status, r.URL)
		}
		if text(t, r.Doc, "h1") != "Not found" || b.URL() != "/nope" {
			t.Errorf("held %s", html.Render(r.Doc))
		}
		if _, err := b.Get("/nope"); err == nil || err.Error() != "GET /nope: not found" {
			t.Errorf("Get: %v", err)
		}
		r = must(b.Open("/broken"))
		wantAnswer(t, r, Answer{Tag: RequestFailed, Text: "kaput"})
		if r.Status != 500 {
			t.Errorf("Status %d, want 500", r.Status)
		}
		if text(t, r.Doc, "h1") != "Something went wrong" {
			t.Errorf("held %s", html.Render(r.Doc))
		}
		if _, err := b.Get("/broken"); err == nil || err.Error() != "GET /broken: failed: kaput" {
			t.Errorf("Get: %v", err)
		}
		if got := titles(t, e); len(got) != 0 {
			t.Errorf("the failed GET's write stayed: %v", got)
		}
		r = must(b.Open("/gone"))
		wantAnswer(t, r, Answer{Tag: PageMissing})
		r = must(b.Open("/loop"))
		if r.Answer.Tag != RequestFailed || !strings.HasPrefix(r.Answer.Text, "more than 10 redirects in a row") {
			t.Errorf("loop = %+v", r.Answer)
		}
		b.Login(session{User: "ann"})
		r = must(b.Open("/old"))
		wantAnswer(t, r, Answer{Tag: Redirected, Page: rt.Route{Path: "/", Query: []rt.KV{{Key: "filter", Value: "Old"}}}})
		if text(t, r.Doc, "#filter") != "Old" {
			t.Errorf("held %s", html.Render(r.Doc))
		}
		// a Datastar 404: the row is gone by the time the button is clicked
		id := insertDirect(t, e, "x")
		must(b.Get("/"))
		tx, _ := e.Transaction()
		if err := db.DeleteRow(e.Ctx, e.Store.(db.Store), tx.(db.Tx), tb, id); err != nil {
			t.Error(err)
		}
		r = must(b.ClickRoute(iDelete(id), "Index.Delete"))
		wantAnswer(t, r, Answer{Tag: PageMissing})
		if r.Status != 404 { // a Datastar answer's code is its Alder-Status
			t.Errorf("Status %d after a Datastar 404, want 404", r.Status)
		}
		if text(t, r.Doc, "h1") != "Not found" {
			t.Errorf("held after a Datastar 404: %s", html.Render(r.Doc))
		}
	})
}

// TestBrowser_BadQuery: a query pair the router cannot parse (a raw `;`) is RequestFailed with
// the router's 400 text naming the parameter, for a plain GET and for a Datastar POST alike.
func TestBrowser_BadQuery(t *testing.T) {
	browse(t, func(b *Browser, e *eff.Env) {
		must := mustOf(t)
		want := Answer{Tag: RequestFailed, Text: "400 bad request: query parameter \"filter\" holds a raw \";\": encode it as %3B"}
		wantAnswer(t, must(b.Open("/?filter=a;b")), want)
		if _, err := b.Get("/?filter=a;b"); err == nil || !strings.Contains(err.Error(), `"filter"`) {
			t.Errorf("Get: %v", err)
		}
		// a Datastar POST (sent directly: no page offers a control with that query): the 200 body
		// patch's Alder-Status 400 is the same RequestFailed
		b.Login(session{User: "ann"})
		must(b.Get("/"))
		ans, err := post(b, "/to?to=a;b")
		if err != nil {
			t.Fatal(err)
		}
		want.Text = "400 bad request: query parameter \"to\" holds a raw \";\": encode it as %3B"
		wantAnswer(t, b.result(ans), want)
		if got := titles(t, e); len(got) != 0 {
			t.Errorf("rows %v", got)
		}
	})
}

// TestBrowser_AnswerParsing exercises the answer parsing directly: an SSE patch replacing the
// body, a navigate event and pageOf.
func TestBrowser_AnswerParsing(t *testing.T) {
	events, err := sseEvents("event: datastar-patch-elements\ndata: selector body\ndata: mode outer\ndata: elements <body><p>a\ndata: elements b</p></body>\n\n" +
		"event: datastar-patch-elements\ndata: selector body\ndata: mode append\ndata: elements <script data-effect=\"el.remove()\">window.location.assign(\"/x?a=1\\u0026b=2\")</script>\n\n")
	if err != nil || len(events) != 2 {
		t.Fatalf("events = %v %v", events, err)
	}
	if got := events[0].field("elements"); got != "<body><p>a\nb</p></body>" {
		t.Fatalf("elements = %q", got)
	}
	if u, ok := navigation(events[1]); !ok || u != "/x?a=1&b=2" {
		t.Fatalf("navigation = %q %v", u, ok)
	}
	if _, ok := navigation(events[0]); ok {
		t.Fatal("a body patch is not a navigation")
	}
	b := &Browser{}
	b.doc, _ = html.Parse(`<html><head><title>t</title></head><body><p>old</p></body></html>`)
	b.hasDoc = true
	if err := b.patch(events[0]); err != nil {
		t.Fatal(err)
	}
	if got := html.Render(b.doc); got != "<html><head><title>t</title></head><body><p>a\nb</p></body></html>" {
		t.Fatalf("patched = %s", got)
	}
	if err := b.patch(sseEvent{typ: "datastar-patch-elements", data: [][2]string{{"selector", "#x"}, {"mode", "outer"}}}); err == nil {
		t.Fatal("patched a selector other than body")
	}
	if got := pageOf("/todos/a%2Fb?filter=Active&q=x+y"); !reflect.DeepEqual(got, rt.Route{Path: "/todos/a%2Fb", Query: []rt.KV{{Key: "filter", Value: "Active"}, {Key: "q", Value: "x y"}}}) || rt.RouteURL(got) != "/todos/a%2Fb?filter=Active&q=x+y" {
		t.Fatalf("pageOf = %+v", got)
	}
}

// TestBrowser_Plain303: a POST answered with a plain 303 (the router sends a navigate event
// to a Datastar request, but a handler behind another http.Handler might not) is followed.
func TestBrowser_Plain303(t *testing.T) {
	browse(t, func(b *Browser, e *eff.Env) {
		must := mustOf(t)
		router := b.handler
		b.handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method == http.MethodPost && r.URL.Path == "/logout" {
				http.Redirect(w, r, "/login", http.StatusSeeOther)
				return
			}
			router.ServeHTTP(w, r)
		})
		b.Login(session{User: "ann"})
		must(b.Get("/"))
		r := must(b.ClickRoute(iLogout, "App.Logout"))
		wantAnswer(t, r, Answer{Tag: Redirected, Page: rt.Route{Path: "/login"}})
		if text(t, r.Doc, "title") != "Log in" {
			t.Errorf("held %s", html.Render(r.Doc))
		}
	})
}

// TestBrowser_PerCase: every case has its own browser (a login does not reach the next case),
// and a test program built without routes has none.
func TestBrowser_PerCase(t *testing.T) {
	var second Answer
	cases := []Case{
		{Module: "M", Name: "one", Task: func() eff.Task[bool] {
			return eff.Call("login", true, func(e *eff.Env) (bool, error) {
				return BrowserOf(e).Login(session{User: "ann"}) == nil, nil
			})
		}},
		{Module: "M", Name: "two", Task: func() eff.Task[bool] {
			return eff.Call("open", true, func(e *eff.Env) (bool, error) {
				r, err := BrowserOf(e).Open("/")
				second = r.Answer
				return err == nil, nil
			})
		}},
	}
	rs := Run(cases, Options{Tables: tables, Routes: fixtureRoutes(), Web: fixtureWeb(), Session: session{}})
	if !rs[0].OK || !rs[1].OK {
		t.Fatalf("results %+v", rs)
	}
	if second.Tag != Redirected {
		t.Fatalf("the second case's browser was logged in: %+v", second)
	}
	rs = Run(cases[:1], Options{Tables: tables})
	if rs[0].OK || !strings.Contains(rs[0].Error, "this test has no browser") {
		t.Fatalf("without routes: %+v", rs[0])
	}
	defer func() {
		if recover() == nil {
			t.Fatal("BrowserOf of a bare Env did not panic")
		}
	}()
	BrowserOf(&eff.Env{Ctx: context.Background()})
}

func TestStep(t *testing.T) {
	var logged []string
	e := &eff.Env{Log: func(level, msg string) { logged = append(logged, level+" "+msg) }}
	Step(e, "Test.login")
	if !reflect.DeepEqual(e.Trace, []string{"Test.login"}) || !reflect.DeepEqual(logged, []string{"effect Test.login"}) {
		t.Fatalf("%v %v", e.Trace, logged)
	}
}

// TestBrowser_ExternalRedirect: a redirect to another site (Url.external) is Redirected with
// the URL as the page's path; nothing is served, and the held page and URL stay — for a GET's
// 303 and for a Datastar navigate alike.
func TestBrowser_ExternalRedirect(t *testing.T) {
	browse(t, func(b *Browser, e *eff.Env) {
		must := mustOf(t)
		b.Login(session{User: "ann"})
		must(b.Get("/"))
		before := html.Render(b.Doc())
		r := must(b.Open("/away"))
		wantAnswer(t, r, Answer{Tag: Redirected, Page: rt.Route{Path: "https://other.example/login?next=%2F"}})
		if b.URL() != "/" || html.Render(r.Doc) != before {
			t.Errorf("held %s %s", b.URL(), html.Render(r.Doc))
		}
		r = must(b.ClickRoute(iAway, "Index.Away"))
		wantAnswer(t, r, Answer{Tag: Redirected, Page: rt.Route{Path: "https://other.example/login"}})
		if b.URL() != "/" || html.Render(r.Doc) != before {
			t.Errorf("held after the navigate: %s %s", b.URL(), html.Render(r.Doc))
		}
	})
}

// TestBrowser_FragmentRedirect: a redirect to "#" (an unsafe Url.external the runtime
// neutralised) or "#frag" stays on the page, as a browser does: Redirected with that target,
// nothing requested, the held page and URL kept — for a GET's 303 and a write's alike.
func TestBrowser_FragmentRedirect(t *testing.T) {
	browse(t, func(b *Browser, e *eff.Env) {
		must := mustOf(t)
		b.Login(session{User: "ann"})
		must(b.Get("/"))
		before := html.Render(b.Doc())
		r := must(b.Open("/hash"))
		wantAnswer(t, r, Answer{Tag: Redirected, Page: rt.Route{Path: "#top"}})
		if b.URL() != "/" || html.Render(r.Doc) != before {
			t.Errorf("held %s %s", b.URL(), html.Render(r.Doc))
		}
		r = must(b.ClickRoute(iHash, "Index.Hash"))
		wantAnswer(t, r, Answer{Tag: Redirected, Page: rt.Route{Path: "#"}})
		if b.URL() != "/" || html.Render(r.Doc) != before {
			t.Errorf("held after the write: %s %s", b.URL(), html.Render(r.Doc))
		}
	})
}

// TestBrowser_FailTextWithColonInPath: the router's log line is stripped of exactly its
// "METHOD /decoded/path: " head, though the path holds ": ".
func TestBrowser_FailTextWithColonInPath(t *testing.T) {
	browse(t, func(b *Browser, e *eff.Env) {
		r := mustOf(t)(b.Open("/p/a%3A%20b"))
		wantAnswer(t, r, Answer{Tag: RequestFailed, Text: "bad slug"})
	})
}

// beginFails is a store whose transactions cannot be begun.
type beginFails struct{ *db.SQLite }

func (beginFails) Begin(context.Context) (eff.Tx, error) {
	return nil, errors.New("database is locked")
}

// TestBrowser_TransactionErrorFailsTheStep: when the test's transaction cannot be begun, the
// step fails with that error and sends no request.
func TestBrowser_TransactionErrorFailsTheStep(t *testing.T) {
	s, err := db.OpenSQLite(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	env := &eff.Env{Ctx: context.Background(), Store: beginFails{s}, Now: cookieNow}
	var stepErr error
	_, err = eff.Run(env, eff.KTask, eff.Call("Test.get", true, func(e *eff.Env) (int, error) {
		_, stepErr = NewBrowser(e, fixtureRoutes(), fixtureWeb()).Open("/login")
		return 0, nil
	}))
	if err != nil {
		t.Fatal(err)
	}
	if stepErr == nil || stepErr.Error() != "beginning the test's transaction: database is locked" {
		t.Fatalf("step error = %v", stepErr)
	}
	if len(env.Trace) != 1 {
		t.Fatalf("a request was served: %v", env.Trace)
	}
}

// TestBrowser_FollowedRedirect (M13): a write's redirect to a page of this app is followed
// by the router (its morph, title and history push), and the browser answers as when it
// fetched the target itself: Redirected with the target held and its URL (title included),
// PageMissing, RequestFailed with the GET's error, and the redirect loop's text.
func TestBrowser_FollowedRedirect(t *testing.T) {
	browse(t, func(b *Browser, e *eff.Env) {
		b.Login(session{User: "ann"})
		mustOf(t)(b.Get("/"))
		to := func(u string) Answer {
			t.Helper()
			ans, err := post(b, "/to?to="+url.QueryEscape(u))
			if err != nil {
				t.Fatal(err)
			}
			return ans
		}
		b.Login(session{})
		if ans := to("/"); ans.Tag != Redirected || ans.Page.Path != "/login" || b.URL() != "/login" || text(t, b.Doc(), "title") != "Log in" {
			t.Errorf("to / signed out: %+v %s %s", ans, b.URL(), html.Render(b.Doc()))
		}
		if ans := to("/gone"); ans.Tag != PageMissing || b.URL() != "/nope" {
			t.Errorf("to /gone: %+v %s", ans, b.URL())
		}
		if ans := to("/broken"); ans.Tag != RequestFailed || ans.Text != "kaput" || b.URL() != "/broken" {
			t.Errorf("to /broken: %+v %s", ans, b.URL())
		}
		if ans := to("/loop"); ans.Tag != RequestFailed || ans.Text != "more than 10 redirects in a row (the last to /loop)" {
			t.Errorf("to /loop: %+v", ans)
		}
	})
}

// TestBrowser_Fetch: Test.fetch answers a file raw and leaves the held page alone; a redirect is
// not followed; Open of a file page is RequestFailed (the run-time guard).
func TestBrowser_Fetch(t *testing.T) {
	browse(t, func(b *Browser, e *eff.Env) {
		b.Login(session{User: "ann"})
		mustOf(t)(b.Get("/"))
		held := html.Render(b.Doc())
		r, err := b.Fetch("/r.csv")
		if err != nil {
			t.Fatal(err)
		}
		if r.Status != 200 || r.ContentType != "text/csv" || string(r.Body) != "a,b\n" {
			t.Errorf("fetch: %+v", r)
		}
		var cd []string
		for _, h := range r.Headers {
			if h.Name == "Content-Disposition" {
				cd = append(cd, h.Value)
			}
		}
		if !slices.Equal(cd, []string{"inline"}) || !slices.IsSortedFunc(r.Headers, func(a, b struct {
			Name  string
			Value string
		}) int {
			return strings.Compare(a.Name, b.Name)
		}) {
			t.Errorf("headers: %+v", r.Headers)
		}
		if b.URL() != "/" || html.Render(b.Doc()) != held {
			t.Errorf("fetch changed the held page: %s", b.URL())
		}
		b.Login(session{})
		r, err = b.Fetch("/")
		if err != nil {
			t.Fatal(err)
		}
		loc := ""
		for _, h := range r.Headers {
			if h.Name == "Location" {
				loc = h.Value
			}
		}
		if r.Status != 303 || loc != "/login" || b.URL() != "/" {
			t.Errorf("fetch of a redirect: %+v (held %s)", r, b.URL())
		}
		o, err := b.Open("/r.csv")
		if err != nil {
			t.Fatal(err)
		}
		wantAnswer(t, o, Answer{Tag: RequestFailed, Text: "GET /r.csv: a file page (text/csv): use `Test.fetch`"})
	})
}

// A destructive toast (the markup of Response.error) reads back as kind "error"; the other
// variants read back as they are written.
func TestToastOf_DestructiveReadsBackAsError(t *testing.T) {
	for variant, want := range map[string]string{
		"destructive": string(rt.NoticeError),
		"success":     "success",
		"info":        "info",
	} {
		els, err := html.Parse(`<al-toast variant="` + variant + `" timeout="5000"> Saved </al-toast>`)
		if err != nil {
			t.Fatal(err)
		}
		if got := toastOf(els); got.Kind != want || got.Text != "Saved" {
			t.Errorf("%s: toastOf = %+v, want kind %q text %q", variant, got, want, "Saved")
		}
	}
}

// The route fixture (#160): what a generated route app answers, written by hand, under /r so
// it sits beside the fixture app. GET /r (R.Index) lists rows, each `<li id="row-N">` with
// click buttons for R.Toggle (PATCH, render), R.Delete (DELETE, redirect), R.Row (GET, an
// element fetch) and R.Rename (POST with `form RenameF`), and a form #add posting R.Add (POST
// `form NewRow`: refresh, invalid) with a click of R.Purge (DELETE, no form) and of R.Nudge
// (PATCH, no form) inside it: a click sends a route without a form, whatever form it is in.
// seen records what each write route received, as the page's script sends it.
type routeApp struct {
	rows []string // the rows' titles; "" a deleted row
	seen map[string]seenReq
	// lie, when set, is the title the next load of R.Row shows instead of the row's own, once: a
	// route whose view the element check catches (Task 4.2)
	lie string
	// summary, when set, adds to the index a count of the rows toggled (`<p id="toggled">`), which
	// R.Toggle's `render` does not patch: a render that lies about the page (Task 4.3)
	summary bool
}

type seenReq struct {
	method, query, ctype, body                string
	form                                      url.Values
	page, build, fetch, datastar, shownHeader string
}

const (
	newRowRoute  = "R.NewRow"
	renameFRoute = "R.RenameF"
)

func newRouteApp(titles ...string) *routeApp {
	return &routeApp{rows: titles, seen: map[string]seenReq{}}
}

func rIndex() rt.Route { return rt.Route{Name: "R.Index", Method: "GET", Path: "/r"} }
func rRow(i int) rt.Route {
	return rt.Route{Name: "R.Row", Method: "GET", Path: "/r/rows/" + strconv.Itoa(i)}
}
func rToggle(i int) rt.Route {
	return rt.Route{Name: "R.Toggle", Method: "PATCH", Path: "/r/rows/" + strconv.Itoa(i)}
}
func rDelete(i int) rt.Route {
	return rt.Route{Name: "R.Delete", Method: "DELETE", Path: "/r/rows/" + strconv.Itoa(i)}
}
func rRename(i int) rt.Route {
	return rt.Route{Name: "R.Rename", Method: "POST", Path: "/r/rows/" + strconv.Itoa(i) + "/rename"}
}
func rAdd() rt.Route   { return rt.Route{Name: "R.Add", Method: "POST", Path: "/r/rows"} }
func rPurge() rt.Route { return rt.Route{Name: "R.Purge", Method: "DELETE", Path: "/r/purge"} }
func rNudge() rt.Route { return rt.Route{Name: "R.Nudge", Method: "PATCH", Path: "/r/nudge"} }

// rGone is a GET route of no app: the page /r/few renders a control for it.
func rGone() rt.Route { return rt.Route{Name: "R.Gone", Method: "GET", Path: "/r/gone/1"} }

func (a *routeApp) row(i int) html.Node {
	return html.El("li", []html.Attr{html.A("id", "row-"+strconv.Itoa(i))}, html.Text(a.rows[i-1]),
		html.El("button", []html.Attr{html.RouteAttr("data-on:click", rToggle(i))}, html.Text("Toggle")),
		html.El("button", []html.Attr{html.RouteAttr("data-on:click", rDelete(i))}, html.Text("Delete")),
		html.El("button", []html.Attr{html.RouteAttr("data-on:click", rRow(i))}, html.Text("Show")),
		html.El("button", []html.Attr{html.RouteAttr("data-on:click", rRename(i))}, html.Text("Rename")))
}

func (a *routeApp) index() html.Node {
	var items []html.Node
	for i, t := range a.rows {
		if t != "" {
			items = append(items, a.row(i+1))
		}
	}
	var summary []html.Node
	if a.summary {
		n := 0
		for _, t := range a.rows {
			if strings.HasSuffix(t, "!") {
				n++
			}
		}
		summary = append(summary, html.El("p", []html.Attr{html.A("id", "toggled")}, html.Text(strconv.Itoa(n)+" toggled")))
	}
	return layout("Rows", append([]html.Node{
		html.El("h1", nil, html.Text("Rows")),
		html.El("ul", []html.Attr{html.A("id", "rows")}, items...)}, append(summary,
		html.FormToRoute(rAdd(), []html.Attr{html.A("id", "add")},
			html.Input(html.Control{Kind: "errors", Form: newRowRoute}),
			html.Input(html.Control{Kind: "input", Form: newRowRoute, Name: "title", Label: "Title"}),
			html.Input(html.Control{Kind: "errors", Form: newRowRoute, Name: "title"}),
			html.El("button", nil, html.Text("Add")),
			html.El("button", []html.Attr{html.RouteAttr("data-on:click", rPurge())}, html.Text("Purge")),
			html.El("button", []html.Attr{html.RouteAttr("data-on:click", rNudge())}, html.Text("Nudge"))))...)...)
}

func (a *routeApp) see(rq *web.Request, name string) {
	r := rq.HTTP
	body := ""
	if r.Body != nil {
		b, _ := io.ReadAll(r.Body) // a url-encoded body was read by the router (rq.Form)
		body = string(b)
	}
	a.seen[name] = seenReq{method: r.Method, query: r.URL.RawQuery, ctype: r.Header.Get("Content-Type"), body: body, form: rq.Form,
		page: r.Header.Get("Alder-Page"), build: r.Header.Get("Alder-Build"), fetch: r.Header.Get("Alder-Fetch"),
		datastar: r.Header.Get("Datastar-Request"), shownHeader: r.Header.Get(html.ShownHeader)}
}

func (a *routeApp) id(rq *web.Request) (int, bool) {
	i, err := strconv.Atoi(rq.Params["id"])
	return i, err == nil && i >= 1 && i <= len(a.rows) && a.rows[i-1] != ""
}

func (a *routeApp) routes() []web.Route {
	return []web.Route{
		{Method: "GET", Pattern: "/r", Handle: func(rq *web.Request) web.Response {
			st := rq.FormState
			if st == nil {
				st = &html.FormState{}
			}
			return web.PageResponse(a.index(), st, nil)
		}},
		{Method: "GET", Pattern: "/r/rows/{id}", Handle: func(rq *web.Request) web.Response {
			a.see(rq, "Row")
			i, ok := a.id(rq)
			if !ok {
				return web.NotFoundResponse(html.None())
			}
			if a.lie != "" {
				title := a.rows[i-1]
				a.rows[i-1], a.lie = a.lie, ""
				defer func() { a.rows[i-1] = title }()
			}
			return web.PageResponse(a.row(i), &html.FormState{}, nil)
		}},
		// a page of row 1 alone, with controls for row 2 (which it does not show) and R.Gone
		{Method: "GET", Pattern: "/r/few", Handle: func(rq *web.Request) web.Response {
			return web.PageResponse(layout("Few",
				html.El("ul", []html.Attr{html.A("id", "rows")}, a.row(1)),
				html.El("button", []html.Attr{html.RouteAttr("data-on:click", rToggle(2))}, html.Text("Toggle 2")),
				html.El("button", []html.Attr{html.RouteAttr("data-on:click", rGone())}, html.Text("Gone"))), &html.FormState{}, nil)
		}},
		{Method: "PATCH", Pattern: "/r/rows/{id}", Handle: func(rq *web.Request) web.Response {
			a.see(rq, "Toggle")
			i, ok := a.id(rq)
			if !ok {
				return web.NotFoundResponse(html.None())
			}
			a.rows[i-1] += "!"
			return web.AnswerResponse(rt.Render(rRow(i)), nil, nil)
		}},
		{Method: "DELETE", Pattern: "/r/rows/{id}", Handle: func(rq *web.Request) web.Response {
			a.see(rq, "Delete")
			i, ok := a.id(rq)
			if !ok {
				return web.NotFoundResponse(html.None())
			}
			a.rows[i-1] = ""
			to := rIndex()
			to.Query = []rt.KV{{Key: "after", Value: "delete"}}
			return web.AnswerResponse(rt.Redirect(to), nil, nil)
		}},
		{Method: "POST", Pattern: "/r/rows/{id}/rename", Handle: func(rq *web.Request) web.Response {
			a.see(rq, "Rename")
			i, ok := a.id(rq)
			if !ok {
				return web.NotFoundResponse(html.None())
			}
			before, _ := rq.RenderShown(nil)
			name := rq.Form.Get("name")
			if name == "" { // RenameF { name: Text } did not decode
				resp := web.InvalidResponse(&html.FormState{Form: renameFRoute, Msg: rt.RouteKey(rRename(i)), Values: map[string]string{"name": ""},
					Errors: map[string][]string{"name": {"is required"}}}, nil)
				resp.Shown = before
				return resp
			}
			a.rows[i-1] = name
			return web.AnswerResponse(rt.Render(rRow(i)), nil, nil)
		}},
		{Method: "POST", Pattern: "/r/rows", Handle: func(rq *web.Request) web.Response {
			a.see(rq, "Add")
			before, _ := rq.RenderShown(nil)
			title := rq.Form.Get("title")
			st := &html.FormState{Form: newRowRoute, Msg: rt.RouteKey(rAdd()), Values: map[string]string{"title": title}}
			switch title {
			case "": // NewRow { title: Text } did not decode
				st.Errors = map[string][]string{"title": {"must not be empty"}}
				resp := web.InvalidResponse(st, nil)
				resp.Shown = before
				return resp
			case "dup":
				resp := web.AnswerResponse(rt.Invalid("that title is taken"), before, nil)
				resp.State = st
				return resp
			}
			a.rows = append(a.rows, title)
			return web.AnswerResponse(rt.WithToast(rt.NoticeSuccess, "Added", rt.Refresh()), before, nil)
		}},
		{Method: "DELETE", Pattern: "/r/purge", Handle: func(rq *web.Request) web.Response {
			a.see(rq, "Purge")
			before, _ := rq.RenderShown(nil)
			resp := web.AnswerResponse(rt.Invalid("nothing to purge"), before, nil)
			resp.State = &html.FormState{Form: newRowRoute, Msg: rt.RouteKey(rPurge())}
			return resp
		}},
		{Method: "PATCH", Pattern: "/r/nudge", Handle: func(rq *web.Request) web.Response {
			a.see(rq, "Nudge")
			before, _ := rq.RenderShown(nil)
			return web.AnswerResponse(rt.Refresh(), before, nil)
		}},
	}
}

// browseRoutes runs steps against app's routes, beside the fixture app's.
func browseRoutes(t *testing.T, app *routeApp, steps ...func(b *Browser, e *eff.Env)) CaseResult {
	t.Helper()
	return browseOpts(t, Options{}, app.routes(), steps...)
}

// A click of a GET route is an element fetch, as the page's script makes it (Alder-Fetch,
// Alder-Page the held URL, Alder-Build): its answer, the route's view, replaces the element
// with its root's id in the held page, and the URL held stays. A route that answers 404
// patches nothing: PageMissing, the "Not found" toast.
func TestBrowser_ClickGetRoutePatches(t *testing.T) {
	app := newRouteApp("one", "two")
	browseRoutes(t, app, func(b *Browser, e *eff.Env) {
		must := mustOf(t)
		must(b.Get("/r"))
		app.rows[1] = "TWO" // changed behind the page's back: only the fetched element shows it
		r := must(b.ClickRoute(rRow(2), `R.Row { id: "2" }`))
		wantAnswer(t, r, Answer{Tag: Rendered, Page: rRow(2)})
		if r.Status != 200 || r.URL != "/r" || b.URL() != "/r" {
			t.Errorf("status %d, url %q", r.Status, r.URL)
		}
		if got := text(t, r.Doc, "#row-2"); !strings.HasPrefix(got, "TWO") {
			t.Errorf("#row-2 = %q, want the fetched row", got)
		}
		s := app.seen["Row"]
		if s.method != "GET" || s.fetch != "1" || s.page != "/r" || s.build != web.BuildID() || s.datastar != "true" || s.query != "" {
			t.Errorf("the fetch sent %+v", s)
		}
		app.rows[0] = "" // gone: the route answers 404, nothing is patched and a toast says so
		before := html.Render(b.Doc())
		r = must(b.ClickRoute(rRow(1), `R.Row { id: "1" }`))
		wantAnswer(t, r, Answer{Tag: PageMissing})
		if html.Render(r.Doc) != before || len(r.Toasts) != 1 || r.Toasts[0] != (Toast{Kind: "error", Text: "Not found"}) {
			t.Errorf("toasts %v, doc changed: %v", r.Toasts, html.Render(r.Doc) != before)
		}
	})
}

// A click of a write route answering `render r`: Rendered r, status 200, r's view patched in by
// its id. A PATCH outside any form is its own form's @patch, the form's _method in the body.
func TestBrowser_ClickWriteRender(t *testing.T) {
	app := newRouteApp("one", "two")
	browseRoutes(t, app, func(b *Browser, e *eff.Env) {
		must := mustOf(t)
		must(b.Get("/r"))
		r := must(b.ClickRoute(rToggle(1), `R.Toggle { id: "1" }`))
		wantAnswer(t, r, Answer{Tag: Rendered, Page: rRow(1)})
		if r.Status != 200 || r.URL != "/r" {
			t.Errorf("status %d, url %q", r.Status, r.URL)
		}
		if got := text(t, r.Doc, "#row-1"); !strings.HasPrefix(got, "one!") {
			t.Errorf("#row-1 = %q, want the rendered row", got)
		}
		s := app.seen["Toggle"]
		if s.method != "PATCH" || s.ctype != "application/x-www-form-urlencoded" || s.form.Encode() != "_method=PATCH" ||
			s.page != "/r" || s.build != web.BuildID() || s.fetch != "" || s.datastar != "true" {
			t.Errorf("the click sent %+v", s)
		}
		if !(Answer{Tag: Rendered, Page: rRow(1)}).AlderEqual(r.Answer) {
			t.Error("Rendered (Row 1) is not equal to the answer")
		}
	})
}

// A write route answering `redirect r`: the target is fetched and held, Redirected r, and the
// result's URL is the target's. A DELETE outside any form is a POST, _method=DELETE in its body.
func TestBrowser_RedirectURL(t *testing.T) {
	app := newRouteApp("one", "two")
	browseRoutes(t, app, func(b *Browser, e *eff.Env) {
		must := mustOf(t)
		must(b.Get("/r"))
		r := must(b.ClickRoute(rDelete(1), `R.Delete { id: "1" }`))
		wantAnswer(t, r, Answer{Tag: Redirected, Page: pageOf("/r?after=delete")})
		if r.URL != "/r?after=delete" || b.URL() != "/r?after=delete" || r.Status != 200 {
			t.Errorf("url %q (held %q), status %d", r.URL, b.URL(), r.Status)
		}
		if html.Select(r.Doc, html.MustParseSelector("#row-1")) != nil {
			t.Errorf("the deleted row is still held: %s", html.Render(r.Doc))
		}
		s := app.seen["Delete"]
		if s.method != "POST" || s.form.Encode() != "_method=DELETE" || s.page != "/r" || s.build != web.BuildID() {
			t.Errorf("the click sent %+v", s)
		}
	})
}

// `invalid t` and a form that does not decode: status 422, FormRejected with what the page
// shows; a refresh is Refreshed, 200, the page re-rendered with the new row.
func TestBrowser_InvalidStatus(t *testing.T) {
	app := newRouteApp("one")
	browseRoutes(t, app, func(b *Browser, e *eff.Env) {
		must := mustOf(t)
		must(b.Get("/r"))
		r := must(b.SubmitRoute(rAdd(), "NewRow", Fields{Values: url.Values{"title": {"dup"}}}))
		wantAnswer(t, r, Answer{Tag: FormRejected, Errors: []web.FieldError{{Field: "", Message: "that title is taken"}}})
		if r.Status != 422 || r.URL != "/r" {
			t.Errorf("status %d, url %q", r.Status, r.URL)
		}
		s := app.seen["Add"]
		if s.method != "POST" || s.form.Encode() != "title=dup" || s.page != "/r" || s.build != web.BuildID() || s.datastar != "true" {
			t.Errorf("the submit sent %+v", s)
		}
		r = must(b.SubmitRoute(rAdd(), "NewRow", Fields{Values: url.Values{"title": {""}}}))
		wantAnswer(t, r, Answer{Tag: FormRejected, Errors: []web.FieldError{{Field: "title", Message: "must not be empty"}}})
		if r.Status != 422 {
			t.Errorf("status %d", r.Status)
		}
		r = must(b.SubmitRoute(rAdd(), "NewRow", Fields{Values: url.Values{"title": {"two"}}}))
		wantAnswer(t, r, Answer{Tag: Refreshed})
		if r.Status != 200 || r.URL != "/r" || len(r.Notices) != 1 || r.Notices[0] != "Added" {
			t.Errorf("status %d, url %q, notices %q", r.Status, r.URL, r.Notices)
		}
		if got := text(t, r.Doc, "#row-2"); !strings.HasPrefix(got, "two") {
			t.Errorf("#row-2 = %q after the refresh", got)
		}
		_, err := b.SubmitRoute(rAdd(), "NewRow", Fields{Values: url.Values{"colour": {"red"}}})
		var ni *NoInputError
		if !errors.As(err, &ni) || err.Error() != "the page's form NewRow has no input `colour`" {
			t.Errorf("a field the form has no input for: %v", err)
		}
	})
}

// A route the held page renders no control for is a NoControlError naming the route as the
// test wrote it (a submit's with its parameters) and the parameters the page offers it with;
// nothing is sent.
func TestBrowser_NoControlRoute(t *testing.T) {
	app := newRouteApp("one")
	browseRoutes(t, app, func(b *Browser, e *eff.Env) {
		must := mustOf(t)
		must(b.Get("/r"))
		_, err := b.ClickRoute(rToggle(9), `R.Toggle { id: "9" }`)
		var nc *NoControlError
		if !errors.As(err, &nc) || err.Error() != `the page /r has no button or form for R.Toggle { id: "9" } (the page has Toggle for id: 1)` {
			t.Errorf("click: %v", err)
		}
		_, err = b.SubmitRoute(rRename(1), "RenameF", Fields{Values: url.Values{"name": {"x"}}})
		if !errors.As(err, &nc) || err.Error() != `the page /r has no form RenameF (R.Rename { id: "1" })` {
			t.Errorf("submit: %v", err)
		}
		if _, ok := app.seen["Toggle"]; ok {
			t.Error("a request was sent")
		}
	})
}

// Ruling R-P2: a click sends what the page's control sends, never a body of the test's: outside
// any form a route's own form's fields (a POST: none; the checker refuses such a click in markup,
// #177, but a generated page may still carry one), inside another form the script's empty
// payload (a DELETE: `datastar={}` in the query, which rt/web drops, and no body; a PATCH: the
// JSON `{}`). A form that decodes empty answers the form's invalid page, 422.
func TestBrowser_InFormClickSendsNoFormBody(t *testing.T) {
	app := newRouteApp("one")
	browseRoutes(t, app, func(b *Browser, e *eff.Env) {
		must := mustOf(t)
		must(b.Get("/r"))
		r := must(b.ClickRoute(rRename(1), `R.Rename { id: "1" }`))
		if r.Answer.Tag != FormRejected || r.Status != 422 {
			t.Errorf("answer %v, status %d", r.Answer, r.Status)
		}
		s := app.seen["Rename"]
		if s.method != "POST" || s.ctype != "application/x-www-form-urlencoded" || len(s.form) != 0 || s.page != "/r" || s.build != web.BuildID() {
			t.Errorf("the click sent %+v", s)
		}
		r = must(b.ClickRoute(rPurge(), "R.Purge"))
		if r.Answer.Tag != FormRejected || r.Status != 422 {
			t.Errorf("purge: answer %v, status %d", r.Answer, r.Status)
		}
		s = app.seen["Purge"]
		if s.method != "DELETE" || s.query != "" || s.body != "" || len(s.form) != 0 || s.page != "/r" || s.build != web.BuildID() {
			t.Errorf("the in-form click sent %+v", s)
		}
		// a click of a PATCH route without a form, inside a form: the empty JSON payload, not the
		// form's fields
		r = must(b.ClickRoute(rNudge(), "R.Nudge"))
		if r.Answer.Tag != Refreshed {
			t.Errorf("nudge: answer %v", r.Answer)
		}
		s = app.seen["Nudge"]
		if s.method != "PATCH" || s.ctype != "application/json" || s.body != "{}" || len(s.form) != 0 {
			t.Errorf("the in-form PATCH click sent %+v", s)
		}
	})
}

// tamper makes b's router answer as it does, with old replaced by new in each answer's body: a
// router whose patches are wrong, which the checks must catch.
func tamper(b *Browser, old, new string) {
	inner := b.handler
	b.handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rec := httptest.NewRecorder()
		inner.ServeHTTP(rec, r)
		for k, vs := range rec.Header() {
			w.Header()[k] = vs
		}
		w.WriteHeader(rec.Code)
		_, _ = io.WriteString(w, strings.Replace(rec.Body.String(), old, new, 1))
	})
}

// The region check (#160) runs after `refresh`, `invalid` and `redirect`: every region the
// answer patched must equal that region of the full render the server made; a step whose patch
// differs fails, naming the region. Untampered, each passes.
func TestCheck_RegionsAfterRefresh(t *testing.T) {
	for _, tc := range []struct {
		name     string
		old, new string
		step     func(b *Browser) (Result, error)
		want     string
	}{
		{"refresh", `<li id="row-2">two`, `<li id="row-2">TWO`,
			func(b *Browser) (Result, error) {
				return b.SubmitRoute(rAdd(), "NewRow", Fields{Values: url.Values{"title": {"two"}}})
			},
			"POST /r/rows: the patched page differs from a full render at body > main > ul#rows > li#row-2"},
		{"invalid", `that title is taken`, `that title is free`,
			func(b *Browser) (Result, error) {
				return b.SubmitRoute(rAdd(), "NewRow", Fields{Values: url.Values{"title": {"dup"}}})
			},
			"POST /r/rows: the patched page differs from a full render at body > main > form#add"},
		{"redirect", `<h1>Rows</h1>`, `<h1>Rose</h1>`,
			func(b *Browser) (Result, error) { return b.ClickRoute(rDelete(1), `R.Delete { id: "1" }`) },
			"GET /r: the patched page differs from a full render at body > main > h1"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, lie := range []bool{false, true} {
				app := newRouteApp("one")
				browseRoutes(t, app, func(b *Browser, e *eff.Env) {
					if _, err := b.Get("/r"); err != nil {
						t.Fatal(err)
					}
					if lie {
						tamper(b, tc.old, tc.new)
					}
					_, err := tc.step(b)
					switch {
					case !lie && err != nil:
						t.Errorf("untampered: %v", err)
					case lie && (err == nil || !strings.Contains(err.Error(), tc.want)):
						t.Errorf("tampered: %v, want %q", err, tc.want)
					}
				})
			}
		})
	}
}

// The element check (#160): after `render r` and an element fetch, the element patched in must
// equal r's view of r's load rendered on its own (r's URL fetched again as an element). A route
// whose render lies fails the step, naming the element and the route; the check's own fetch
// changes nothing the step reports.
func TestCheck_ElementAfterRender(t *testing.T) {
	app := newRouteApp("one", "two")
	browseRoutes(t, app, func(b *Browser, e *eff.Env) {
		must := mustOf(t)
		must(b.Get("/r"))
		r := must(b.ClickRoute(rToggle(1), `R.Toggle { id: "1" }`))
		wantAnswer(t, r, Answer{Tag: Rendered, Page: rRow(1)})
		if r.Status != 200 || len(r.Toasts) != 0 {
			t.Errorf("status %d, toasts %v", r.Status, r.Toasts)
		}
		must(b.ClickRoute(rRow(2), `R.Row { id: "2" }`))

		app.lie = "fake"
		_, err := b.ClickRoute(rToggle(1), `R.Toggle { id: "1" }`)
		want := "PATCH /r/rows/1: the answer patched #row-1 unlike R.Row's view rendered on its own (GET /r/rows/1) at li#row-1"
		if err == nil || !strings.Contains(err.Error(), want) || !strings.Contains(err.Error(), "fake") {
			t.Errorf("render: %v, want %q", err, want)
		}
		app.lie = "fake"
		_, err = b.ClickRoute(rRow(2), `R.Row { id: "2" }`)
		want = "GET /r/rows/2: the answer patched #row-2 unlike R.Row's view rendered on its own (GET /r/rows/2) at li#row-2"
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("element fetch: %v, want %q", err, want)
		}
	})
}

// Review Focus: a `render` whose root id the held page lacks (the row was filtered out) is
// dropped silently by a real browser; a test fails the step naming the id.
func TestCheck_RenderTargetMissing(t *testing.T) {
	app := newRouteApp("one", "two")
	browseRoutes(t, app, func(b *Browser, e *eff.Env) {
		if _, err := b.Get("/r/few"); err != nil {
			t.Fatal(err)
		}
		_, err := b.ClickRoute(rToggle(2), "R.Toggle")
		if err == nil || !strings.Contains(err.Error(), "the answer patched `#row-2`, which the page does not have") {
			t.Errorf("err = %v", err)
		}
	})
}

// Ruling R-P3: an element fetch of a URL no route matches patches nothing, as a route's 404
// does: PageMissing, the "Not found" toast, the held page unchanged.
func TestBrowser_ClickUnmatchedGetRoute(t *testing.T) {
	app := newRouteApp("one")
	browseRoutes(t, app, func(b *Browser, e *eff.Env) {
		must := mustOf(t)
		must(b.Get("/r/few"))
		before := html.Render(b.Doc())
		r := must(b.ClickRoute(rGone(), "R.Gone"))
		wantAnswer(t, r, Answer{Tag: PageMissing})
		if html.Render(r.Doc) != before || len(r.Toasts) != 1 || r.Toasts[0] != (Toast{Kind: "error", Text: "Not found"}) {
			t.Errorf("toasts %v, doc changed: %v\n%s", r.Toasts, html.Render(r.Doc) != before, html.Render(r.Doc))
		}
	})
}

// dbRenderRoutes is a page /d with one element #d and a button for PATCH /d/1, which inserts a
// row "written" and answers `render` of GET /d/1. GET /d/1's load reads how many "written" rows
// it sees, and — a load that writes, which a Query cannot, but a hand-made route can — inserts a
// row "loaded" each time it runs.
func dbRenderRoutes() []web.Route {
	dRow := rt.Route{Name: "D.Row", Method: "GET", Path: "/d/1"}
	dTouch := rt.Route{Name: "D.Touch", Method: "PATCH", Path: "/d/1"}
	item := func(rq *web.Request) (html.Node, error) {
		rows, err := loadIndex(rq)
		if err != nil {
			return html.None(), err
		}
		n := 0
		for _, r := range rows {
			if r.Title == "written" {
				n++
			}
		}
		return html.El("li", []html.Attr{html.A("id", "d")}, html.Text("written: "+strconv.Itoa(n))), nil
	}
	return []web.Route{
		{Method: "GET", Pattern: "/d", Handle: func(rq *web.Request) web.Response {
			li, err := item(rq)
			if err != nil {
				return fail(err)
			}
			return web.PageResponse(layout("D", html.El("ul", nil, li),
				html.El("button", []html.Attr{html.RouteAttr("data-on:click", dTouch)}, html.Text("Touch"))), &html.FormState{}, nil)
		}},
		{Method: "GET", Pattern: "/d/1", Handle: func(rq *web.Request) web.Response {
			li, err := item(rq)
			if err == nil {
				_, err = eff.Run(rq.Env, eff.KTask, insertTask("loaded"))
			}
			if err != nil {
				return fail(err)
			}
			return web.PageResponse(li, &html.FormState{}, nil)
		}},
		{Method: "PATCH", Pattern: "/d/1", Handle: func(rq *web.Request) web.Response {
			if _, err := eff.Run(rq.Env, eff.KTask, insertTask("written")); err != nil {
				return fail(err)
			}
			return web.AnswerResponse(rt.Render(dRow), nil, nil)
		}},
	}
}

// The element check's fetch runs over the test's real database in a savepoint that is rolled
// back: it sees the step's write (its view counts it, as the render's did, so the check passes)
// and leaves none of its own — the one "loaded" row is the render's, not the check's.
func TestCheck_ElementFetchRolledBack(t *testing.T) {
	res := browseOpts(t, Options{}, dbRenderRoutes(), func(b *Browser, e *eff.Env) {
		must := mustOf(t)
		must(b.Get("/d"))
		r := must(b.ClickRoute(rt.Route{Name: "D.Touch", Method: "PATCH", Path: "/d/1"}, "D.Touch"))
		wantAnswer(t, r, Answer{Tag: Rendered, Page: rt.Route{Name: "D.Row", Method: "GET", Path: "/d/1"}})
		if got := text(t, r.Doc, "#d"); got != "written: 1" {
			t.Errorf("#d = %q", got)
		}
		if got := strings.Join(titles(t, e), ","); got != "loaded,written" {
			t.Errorf("rows %q, want the render's load and the write only", got)
		}
	})
	if !res.OK {
		t.Fatalf("the test failed: %s", res.Error)
	}
}

// failingSavepoints is a Savepointer whose RollbackTo (and, when release is set, only Release)
// fails.
type failingSavepoints struct {
	eff.Savepointer
	release bool
}

func (f failingSavepoints) RollbackTo(ctx context.Context, tx eff.Tx, name string) error {
	if f.release {
		return f.Savepointer.RollbackTo(ctx, tx, name)
	}
	return errors.New("disk I/O error")
}

func (f failingSavepoints) Release(ctx context.Context, tx eff.Tx, name string) error {
	if f.release {
		return errors.New("disk I/O error")
	}
	return f.Savepointer.Release(ctx, tx, name)
}

// A savepoint the element check cannot roll back or release fails the step, never silently.
func TestCheck_ElementRollbackFails(t *testing.T) {
	for _, release := range []bool{false, true} {
		browseOpts(t, Options{}, dbRenderRoutes(), func(b *Browser, e *eff.Env) {
			must := mustOf(t)
			must(b.Get("/d"))
			cur, err := b.within()
			if err != nil || cur == nil || cur.Savepoints == nil {
				t.Fatalf("no savepoints: %v", err)
			}
			b.cur = &eff.Within{Tx: cur.Tx, Savepoints: failingSavepoints{cur.Savepoints, release}}
			_, err = b.quietly(context.Background(), httptest.NewRequest("GET", "/d/1", nil))
			if err == nil || !strings.Contains(err.Error(), "the element check: undoing its fetch: disk I/O error") {
				t.Errorf("release %v: err = %v", release, err)
			}
		})
	}
}

// Test.refresh (#160): a plain GET of the held URL whose body must equal the held one, compared
// as Doc equality compares. After a toggle whose `render` patched all the page shows of it, the
// refresh passes: Refreshed, the URL held, 200, the fresh page held. Before any page it fails.
func TestRefresh_PassesAfterToggle(t *testing.T) {
	app := newRouteApp("one", "two")
	browseRoutes(t, app, func(b *Browser, e *eff.Env) {
		if _, err := b.Refresh(); err == nil || err.Error() != "Test.refresh: the browser holds no page yet" {
			t.Errorf("before any page: %v", err)
		}
		must := mustOf(t)
		must(b.Get("/r"))
		must(b.ClickRoute(rToggle(1), `R.Toggle { id: "1" }`))
		r := must(b.Refresh())
		wantAnswer(t, r, Answer{Tag: Refreshed})
		if r.URL != "/r" || r.Status != 200 || b.URL() != "/r" {
			t.Errorf("url %q (held %q), status %d", r.URL, b.URL(), r.Status)
		}
		if got := text(t, r.Doc, "#row-1"); !strings.HasPrefix(got, "one!") {
			t.Errorf("#row-1 = %q after the refresh", got)
		}
		if s := app.seen["Row"]; s.method != "GET" {
			t.Errorf("the element check did not fetch R.Row: %+v", s)
		}
	})
}

// A route whose `render` lies about the page (R.Toggle patches the row, but the page's count of
// toggled rows changes too and stays stale) passes the element check and fails Test.refresh,
// the failure naming where the held body and the fresh one differ, with both sides.
func TestRefresh_FailsWhenRenderLies(t *testing.T) {
	app := newRouteApp("one", "two")
	app.summary = true
	browseRoutes(t, app, func(b *Browser, e *eff.Env) {
		must := mustOf(t)
		must(b.Get("/r"))
		must(b.ClickRoute(rToggle(1), `R.Toggle { id: "1" }`))
		_, err := b.Refresh()
		want := "Test.refresh: GET /r: the page held differs from the page loaded again at body > main > p#toggled: "
		if err == nil || !strings.HasPrefix(err.Error(), want) ||
			!strings.Contains(err.Error(), "\n  held:   <p id=\"toggled\">0 toggled</p>") ||
			!strings.Contains(err.Error(), "\n  loaded: <p id=\"toggled\">1 toggled</p>") {
			t.Errorf("err = %v, want %q and both sides", err, want)
		}
		must(b.Get("/r/rows/2"))
		app.rows[1] = "" // gone: the page is not shown again
		if _, err := b.Refresh(); err == nil || err.Error() != "Test.refresh: GET /r/rows/2: not found" {
			t.Errorf("a page gone: %v", err)
		}
	})
}
