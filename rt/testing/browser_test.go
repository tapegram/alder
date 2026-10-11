package rtesting

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"alder/rt"
	"alder/rt/db"
	"alder/rt/eff"
	"alder/rt/html"
	"alder/rt/web"
)

// The fixture app: what the generated code of a small app would be, written by hand. Its
// session is { user: Text }; page Index ("/", `requires user`, parameter filter = "All") lists
// T's rows with a Delete button each, a NewRow form (title, note) posting Add, buttons posting
// Fail, Boom and App.Logout; page Login ("/login") has a form posting Login.Submit, which logs
// the email in and redirects to "/".

type session struct{ User string }

type codec struct{}

func (codec) Default() any                 { return session{} }
func (codec) Encode(v any) ([]byte, error) { return json.Marshal(v.(session)) }
func (codec) Decode(b []byte) (any, error) {
	var s session
	err := json.Unmarshal(b, &s)
	return s, err
}

func msg(module, name string, params ...rt.KV) rt.MsgRef {
	return rt.MsgRef{Module: module, Msg: name, Params: params}
}

func layout(title string, body ...html.Node) html.Node {
	return html.El("html", nil,
		html.El("head", nil, html.El("meta", []html.Attr{html.A("charset", "utf-8")}), html.El("title", nil, html.Text(title))),
		html.El("body", nil, html.El("main", nil, body...)))
}

const newRow = "Index.NewRow"

func indexView(filter string, s session, rows []row) html.Node {
	var items []html.Node
	for _, r := range rows {
		items = append(items, html.El("li", []html.Attr{html.A("data-id", r.ID)}, html.Text(r.Title),
			html.El("button", []html.Attr{html.MsgAttr("data-on:click", msg("Index", "Delete", rt.KV{Key: "id", Value: r.ID}))}, html.Text("Delete"))))
	}
	return layout("Index",
		html.El("p", []html.Attr{html.A("id", "user")}, html.Text(s.User)),
		html.El("p", []html.Attr{html.A("id", "filter")}, html.Text(filter)),
		html.El("p", []html.Attr{html.A("id", "summary")}, html.Text(strconv.Itoa(len(rows))+" rows")),
		html.El("ul", []html.Attr{html.A("id", "rows")}, items...),
		html.FormTo(msg("Index", "Add"), nil,
			html.Input(html.Control{Kind: "errors", Form: newRow}),
			html.Input(html.Control{Kind: "input", Form: newRow, Name: "title", Label: "Title", Required: true}),
			html.Input(html.Control{Kind: "errors", Form: newRow, Name: "title"}),
			html.Input(html.Control{Kind: "input", Form: newRow, Name: "note", Label: "Note"}),
			html.El("button", nil, html.Text("Add"))),
		html.El("button", []html.Attr{html.MsgAttr("data-on:click", msg("Index", "Fail"))}, html.Text("Fail")),
		html.El("button", []html.Attr{html.MsgAttr("data-on:click", msg("Index", "Boom"))}, html.Text("Boom")),
		html.El("button", []html.Attr{html.MsgAttr("data-on:click", msg("App", "Logout"))}, html.Text("Log out")),
		html.El("button", []html.Attr{html.MsgAttr("data-on:click", msg("Index", "Away"))}, html.Text("Away")),
		html.El("a", []html.Attr{html.A("href", "/?filter=Done")}, html.Text("Done")),
	)
}

func loginView() html.Node {
	return layout("Log in", html.FormTo(msg("Login", "Submit"), nil,
		html.Input(html.Control{Kind: "errors", Form: "Login.Form"}),
		html.Input(html.Control{Kind: "input", Form: "Login.Form", Name: "email", Type: "email", Label: "Email"}),
		html.Input(html.Control{Kind: "errors", Form: "Login.Form", Name: "email"})))
}

func pageParams(filter string) []rt.KV {
	return []rt.KV{rt.QueryKV("filter", filter, "All")}
}

// filterOf decodes the page's parameter from a GET's query or a message's _p. keys.
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

// update runs task and answers as a generated message route does (Rerender: a body patch under
// Datastar, else a 303 to the page; Reject with a form state: the form re-rendered).
func update(rq *web.Request, filter string, task eff.Task[rt.Nav], st *html.FormState) web.Response {
	nav, err := eff.Run(rq.Env, eff.KTask, task)
	if err != nil {
		return fail(err)
	}
	switch {
	case nav.Tag == rt.NavRedirect:
		return web.RedirectResponse(rt.PageURL(nav.Page), rq.Env.Session)
	case nav.Tag == rt.NavReject && st != nil:
		st.FormError = nav.Text
	case !rq.Datastar:
		return web.RedirectResponse(rt.PageAt("/", pageParams(filter)), rq.Env.Session)
	default:
		st = &html.FormState{}
	}
	st.PageParams = pageParams(filter)
	rows, err := loadIndex(rq)
	if err != nil {
		return fail(err)
	}
	return web.PageResponse(indexView(filter, rq.Env.Session.(session), rows), st, rq.Env.Session, true)
}

func insertTask(title string) eff.Task[string] {
	return eff.Call("Db.insert T", true, func(e *eff.Env) (string, error) {
		return db.InsertRow(e.Ctx, e.Store.(db.Store), db.TxOf(e), tb, []any{nil, title}, e.Now())
	})
}

func fixtureRoutes() []web.Route {
	pageFilter := func(rq *web.Request) string { _, q := rt.PageParams(rq.Query); return filterOf(q) }
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
			f := filterOf(rq.Query)
			return web.PageResponse(indexView(f, s, rows), &html.FormState{PageParams: pageParams(f)}, nil, false)
		}},
		{Method: "GET", Pattern: "/login", Handle: func(rq *web.Request) web.Response {
			return web.PageResponse(loginView(), nil, nil, false)
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
		{Method: "POST", Pattern: "/_m/Index/Away", Handle: func(rq *web.Request) web.Response {
			return web.RedirectResponse("https://other.example/login", rq.Env.Session)
		}},
		{Method: "GET", Pattern: "/broken", Handle: func(rq *web.Request) web.Response {
			_, err := eff.Run(rq.Env, eff.KTask, then(insertTask("broken"), eff.Fail[int]("kaput")))
			return fail(err)
		}},
		{Method: "POST", Pattern: "/_m/Login/Submit", Handle: func(rq *web.Request) web.Response {
			email := rq.Form.Get("email")
			if email == "" {
				return web.PageResponse(loginView(), &html.FormState{Form: "Login.Form", Msg: rt.MsgKey("/_m/Login/Submit", rq.Query),
					Values: map[string]string{"email": ""}, Errors: map[string][]string{"email": {"is required"}}}, nil, true)
			}
			return web.RedirectResponse("/", session{User: email})
		}},
		{Method: "POST", Pattern: "/_m/Index/Add", Handle: func(rq *web.Request) web.Response {
			f := pageFilter(rq)
			values := map[string]string{"title": rq.Form.Get("title"), "note": rq.Form.Get("note")}
			key := rt.MsgKey("/_m/Index/Add", rq.Query)
			if values["title"] == "" {
				rows, _ := loadIndex(rq)
				return web.PageResponse(indexView(f, rq.Session.(session), rows), &html.FormState{Form: newRow, Msg: key, Values: values,
					Errors: map[string][]string{"title": {"must not be empty"}}, PageParams: pageParams(f)}, nil, true)
			}
			st := &html.FormState{Form: newRow, Msg: key, Values: values}
			if values["title"] == "dup" {
				return update(rq, f, eff.Pure(rt.Reject("that title is taken")), st)
			}
			return update(rq, f, eff.Map(func(string) rt.Nav { return rt.Rerender() }, insertTask(values["title"])), st)
		}},
		{Method: "POST", Pattern: "/_m/Index/Delete", Handle: func(rq *web.Request) web.Response {
			id := rq.Query.Get("id")
			return update(rq, pageFilter(rq), eff.Call("Db.delete T", true, func(e *eff.Env) (rt.Nav, error) {
				m, err := db.GetRow(e.Ctx, e.Store.(db.Store), db.TxOf(e), tb, id)
				if err != nil {
					return rt.Nav{}, err
				}
				if !m.Ok {
					return rt.Nav{}, eff.ErrNotFound
				}
				return rt.Rerender(), db.DeleteRow(e.Ctx, e.Store.(db.Store), db.TxOf(e), tb, id)
			}), nil)
		}},
		{Method: "POST", Pattern: "/_m/Index/Fail", Handle: func(rq *web.Request) web.Response {
			return update(rq, pageFilter(rq), then(insertTask("failed"), eff.Fail[rt.Nav]("nope")), nil)
		}},
		{Method: "POST", Pattern: "/_m/Index/Boom", Handle: func(rq *web.Request) web.Response {
			return update(rq, pageFilter(rq), then(insertTask("boom"), eff.Call("boom", false, func(*eff.Env) (rt.Nav, error) { panic("boom") })), nil)
		}},
		{Method: "POST", Pattern: "/_m/App/Logout", Handle: func(rq *web.Request) web.Response {
			return web.RedirectResponse("/login", session{})
		}},
	}
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
		wantAnswer(t, r, Answer{Tag: Redirected, Page: rt.Page{Path: "/login"}})
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
		r = must(b.Submit("/_m/Login/Submit", url.Values{"email": {"bob@example.com"}}))
		wantAnswer(t, r, Answer{Tag: Redirected, Page: rt.Page{Path: "/"}})
		if r.Session != (session{User: "bob@example.com"}) || text(t, r.Doc, "#user") != "bob@example.com" {
			t.Errorf("after the login form: session %v, page user %q", r.Session, text(t, r.Doc, "#user"))
		}

		// App.Logout through its button
		r = must(b.Click("/_m/App/Logout"))
		wantAnswer(t, r, Answer{Tag: Redirected, Page: rt.Page{Path: "/login"}})
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
		r := must(b.Submit("/_m/Index/Add", url.Values{"title": {""}, "note": {"keep <me>"}}))
		wantAnswer(t, r, Answer{Tag: Rejected, Errors: []web.FieldError{{Field: "title", Message: "must not be empty"}}})
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
		// the form-level Reject text
		r = must(b.Submit("/_m/Index/Add", url.Values{"title": {"dup"}}))
		wantAnswer(t, r, Answer{Tag: Rejected, Errors: []web.FieldError{{Field: "", Message: "that title is taken"}}})
		if v, _ := html.AttrOf(html.Select(r.Doc, html.MustParseSelector("input[name=title]"))[0], "value"); v != "dup" {
			t.Errorf("title value = %q", v)
		}
		// a good one: Rerendered, the errors gone, the row listed
		r = must(b.Submit("/_m/Index/Add", url.Values{"title": {"milk"}}))
		wantAnswer(t, r, Answer{Tag: Rerendered})
		if text(t, r.Doc, "#rows li") != "milkDelete" || text(t, r.Doc, "#summary") != "1 rows" {
			t.Errorf("after add: %s", html.Render(r.Doc))
		}
		if !reflect.DeepEqual(titles(t, e), []string{"milk"}) {
			t.Errorf("rows = %v", titles(t, e))
		}
	})
}

// TestBrowser_PageParamsRideAlong (Review Focus 3): on a page opened with a parameter, a click
// posts the control's URL with `_p.filter`, and the Rerendered page keeps the parameter.
func TestBrowser_PageParamsRideAlong(t *testing.T) {
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
		r = must(b.Click("/_m/Index/Delete?id=" + url.QueryEscape(a)))
		wantAnswer(t, r, Answer{Tag: Rerendered})
		if text(t, r.Doc, "#filter") != "Active" || text(t, r.Doc, "#summary") != "1 rows" {
			t.Errorf("after the click: %s", html.Render(r.Doc))
		}
		// the trace: the request's effects (update, then the re-load)
		if got := e.Trace[start:]; !reflect.DeepEqual(got, []string{"Db.delete T", "Db.all T"}) {
			t.Errorf("trace = %v", got)
		}
		// and a submit on that page too
		r = must(b.Submit("/_m/Index/Add", url.Values{"title": {"c"}}))
		wantAnswer(t, r, Answer{Tag: Rerendered})
		if text(t, r.Doc, "#filter") != "Active" {
			t.Errorf("after the submit: filter %q", text(t, r.Doc, "#filter"))
		}
		// the held page's message URLs still carry the parameter
		if f := html.Select(r.Doc, html.MustParseSelector("form[action]")); len(f) == 0 {
			t.Error("no form")
		} else if a, _ := html.AttrOf(f[0], "action"); !strings.Contains(a, "_p.filter=Active") {
			t.Errorf("action = %q", a)
		}
	})
}

// TestBrowser_FailuresKeepTheTestsRows (Review Focus 5): a request whose update fails or
// panics is Failed with its message; its own write is undone, the test's rows stay; steps work
// inside Test.run (Inner), whose failure undoes the steps' writes.
func TestBrowser_FailuresKeepTheTestsRows(t *testing.T) {
	res := browse(t, func(b *Browser, e *eff.Env) {
		must := mustOf(t)
		insertDirect(t, e, "mine")
		b.Login(session{User: "ann"})
		must(b.Get("/"))
		r := must(b.Click("/_m/Index/Fail"))
		wantAnswer(t, r, Answer{Tag: Failed, Text: "nope"})
		if text(t, r.Doc, "h1") != "Something went wrong" {
			t.Errorf("held %s", html.Render(r.Doc))
		}
		if r.Session != (session{User: "ann"}) {
			t.Errorf("session %v", r.Session)
		}
		must(b.Get("/"))
		r = must(b.Click("/_m/Index/Boom"))
		wantAnswer(t, r, Answer{Tag: Failed, Text: "panic: boom"})
		if got := titles(t, e); !reflect.DeepEqual(got, []string{"mine"}) {
			t.Errorf("rows after the failures = %v", got)
		}

		// within Test.run: the steps run (nested savepoints) …
		_, _, ok, trace, err := Inner(e, eff.Call("Test.get Index", true, func(ie *eff.Env) (int, error) {
			r := must(BrowserOf(ie).Get("/"))
			must(BrowserOf(ie).Submit("/_m/Index/Add", url.Values{"title": {"inner"}}))
			return len(html.Select(r.Doc, html.MustParseSelector("#rows li"))), nil
		}))
		if err != nil || !ok || !reflect.DeepEqual(trace, []string{"Test.get Index", "Db.all T", "Db.insert T", "Db.all T"}) {
			t.Errorf("Inner: ok %v trace %v err %v", ok, trace, err)
		}
		if got := titles(t, e); !reflect.DeepEqual(got, []string{"inner", "mine"}) {
			t.Errorf("rows after Test.run = %v", got)
		}
		// … and a Test.run that fails after a step undoes the step's write
		_, msg, ok, _, err := Inner(e, then(eff.Call("Test.submit", true, func(ie *eff.Env) (int, error) {
			must(BrowserOf(ie).Submit("/_m/Index/Add", url.Values{"title": {"undone"}}))
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

// TestBrowser_NoControl: Submit and Click act only on controls the held page renders.
func TestBrowser_NoControl(t *testing.T) {
	browse(t, func(b *Browser, e *eff.Env) {
		must := mustOf(t)
		_, err := b.Click("/_m/Index/Delete?id=zzz")
		if err == nil || err.Error() != "no page is open: `Test.get` one first" {
			t.Errorf("before any page: %v", err)
		}
		for _, sub := range []func() (Result, error){
			func() (Result, error) { return b.Submit("/_m/Index/NewRow", nil) },
			func() (Result, error) { return b.SubmitForm("/_m/Index/NewRow", "NewRow", Fields{}) },
		} {
			if _, err := sub(); err == nil || err.Error() != "no page is open: `Test.get` one first" {
				t.Errorf("submit before any page: %v", err)
			}
		}
		b.Login(session{User: "ann"})
		a := insertDirect(t, e, "a")
		must(b.Get("/"))
		start := len(e.Trace)
		_, err = b.Click("/_m/Index/Delete?id=zzz")
		var nc *NoControlError
		if !errors.As(err, &nc) || nc.Page != "/" || nc.URL != "/_m/Index/Delete?id=zzz" ||
			err.Error() != "the page / has no button or form for Index.Delete { id: \"zzz\" } (the page has Delete for id: "+a+")" {
			t.Errorf("click: %v", err)
		}
		_, err = b.Submit("/_m/Login/Submit", url.Values{})
		if err == nil || err.Error() != "the page / has no form for Login.Submit" {
			t.Errorf("submit: %v", err)
		}
		errors.As(err, &nc)
		nc.Form, nc.Msg = "LoginForm", "Login.Submit"
		if err.Error() != "the page / has no form LoginForm (Login.Submit)" {
			t.Errorf("submit with Form: %v", err)
		}
		// a click button is not a form to submit; a form posting it is a control to click
		if _, err := b.Submit("/_m/Index/Fail", nil); err == nil {
			t.Error("submitted a click button")
		}
		// a field the form renders no input for is not sent by a browser: the step fails (C1)
		_, err = b.SubmitForm("/_m/Index/Add", "NewRow", Fields{Values: url.Values{"title": {"x"}, "colour": {"red"}}})
		if err == nil || err.Error() != "the page's form NewRow has no input `colour`" {
			t.Errorf("a field without an input: %v", err)
		}
		if len(e.Trace) != start {
			t.Errorf("a refused step sent a request: %v", e.Trace[start:])
		}
		// the same message with its own parameters matches; the page's are not compared
		r := must(b.Click("/_m/Index/Delete?id=" + url.QueryEscape(a) + "&_p.filter=Other"))
		wantAnswer(t, r, Answer{Tag: Rerendered})
	})
}

// TestBrowser_MissingAndFailed: 404 and 500 for a GET hold the served document; a Datastar 404
// (a click on a row gone meanwhile) is Missing; redirects are followed, a redirect to a 404 is
// Missing, a loop gives up.
func TestBrowser_MissingAndFailed(t *testing.T) {
	browse(t, func(b *Browser, e *eff.Env) {
		must := mustOf(t)
		r := must(b.Open("/nope"))
		wantAnswer(t, r, Answer{Tag: Missing})
		if text(t, r.Doc, "h1") != "Not found" || b.URL() != "/nope" {
			t.Errorf("held %s", html.Render(r.Doc))
		}
		if _, err := b.Get("/nope"); err == nil || err.Error() != "GET /nope: not found" {
			t.Errorf("Get: %v", err)
		}
		r = must(b.Open("/broken"))
		wantAnswer(t, r, Answer{Tag: Failed, Text: "kaput"})
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
		wantAnswer(t, r, Answer{Tag: Missing})
		r = must(b.Open("/loop"))
		if r.Answer.Tag != Failed || !strings.HasPrefix(r.Answer.Text, "more than 10 redirects in a row") {
			t.Errorf("loop = %+v", r.Answer)
		}
		b.Login(session{User: "ann"})
		r = must(b.Open("/old"))
		wantAnswer(t, r, Answer{Tag: Redirected, Page: rt.Page{Path: "/", Query: []rt.KV{{Key: "filter", Value: "Old"}}}})
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
		r = must(b.Click("/_m/Index/Delete?id=" + url.QueryEscape(id)))
		wantAnswer(t, r, Answer{Tag: Missing})
		if text(t, r.Doc, "h1") != "Not found" {
			t.Errorf("held after a Datastar 404: %s", html.Render(r.Doc))
		}
	})
}

// TestBrowser_AnswerParsing exercises the answer parsing directly: an SSE patch replacing the
// body, a navigate event, MsgLabel and pageOf.
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
	if got := MsgLabel("/_m/Todos.Index/Toggle?id=a%20b&_p.filter=Active"); got != `Todos.Index.Toggle { id: "a b" }` {
		t.Fatalf("MsgLabel = %s", got)
	}
	if got := MsgLabel("/_m/App/Logout"); got != "App.Logout" {
		t.Fatalf("MsgLabel = %s", got)
	}
	if got := pageOf("/todos/a%2Fb?filter=Active&q=x+y"); !reflect.DeepEqual(got, rt.Page{Path: "/todos/a%2Fb", Query: []rt.KV{{Key: "filter", Value: "Active"}, {Key: "q", Value: "x y"}}}) || rt.PageURL(got) != "/todos/a%2Fb?filter=Active&q=x+y" {
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
			if r.Method == http.MethodPost && r.URL.Path == "/_m/App/Logout" {
				http.Redirect(w, r, "/login", http.StatusSeeOther)
				return
			}
			router.ServeHTTP(w, r)
		})
		b.Login(session{User: "ann"})
		must(b.Get("/"))
		r := must(b.Click("/_m/App/Logout"))
		wantAnswer(t, r, Answer{Tag: Redirected, Page: rt.Page{Path: "/login"}})
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
		wantAnswer(t, r, Answer{Tag: Redirected, Page: rt.Page{Path: "https://other.example/login?next=%2F"}})
		if b.URL() != "/" || html.Render(r.Doc) != before {
			t.Errorf("held %s %s", b.URL(), html.Render(r.Doc))
		}
		r = must(b.Click("/_m/Index/Away"))
		wantAnswer(t, r, Answer{Tag: Redirected, Page: rt.Page{Path: "https://other.example/login"}})
		if b.URL() != "/" || html.Render(r.Doc) != before {
			t.Errorf("held after the navigate: %s %s", b.URL(), html.Render(r.Doc))
		}
	})
}

// TestBrowser_FailTextWithColonInPath: the router's log line is stripped of exactly its
// "METHOD /decoded/path: " head, though the path holds ": ".
func TestBrowser_FailTextWithColonInPath(t *testing.T) {
	browse(t, func(b *Browser, e *eff.Env) {
		r := mustOf(t)(b.Open("/p/a%3A%20b"))
		wantAnswer(t, r, Answer{Tag: Failed, Text: "bad slug"})
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
