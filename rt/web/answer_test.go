package web

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"

	"alder/rt"
	"alder/rt/html"
)

// ansApp is a page of rows at "/" (each `<li id="row-N">` in `<ul id="rows">`, a heading outside
// every region, a form `<form id="add">`), each row's own GET at /rows/{id} (its `<li>` alone),
// and write routes answering each Response builder as a generated write route does: a route that
// may answer refresh or invalid renders the shown page (RenderShown) before its update.
type ansApp struct {
	done    []bool
	heading string
	toasts  bool // the page has an <al-toasts> region of its own

	loads, rowLoads int    // runs of GET / and of GET /rows/{id}
	rowAnswer       string // how GET /rows/{id} answers: "" its row, "404", "303" (to /login), "500"
}

const ansForm = "R.New"

// ansAddKey is the key of the form #add posts (its FormState.Msg).
var ansAddKey = rt.MsgRef{Module: "R", Msg: "Add"}.Key()

func newAnsApp(n int) *ansApp { return &ansApp{done: make([]bool, n), heading: "Rows"} }

func (a *ansApp) row(i int) html.Node {
	class := "open"
	if a.done[i-1] {
		class = "done"
	}
	return html.El("li", []html.Attr{html.A("id", "row-"+strconv.Itoa(i)), html.A("class", class)}, html.Text("Row "+strconv.Itoa(i)))
}

func (a *ansApp) index() html.Node {
	var items []html.Node
	for i := range a.done {
		items = append(items, a.row(i+1))
	}
	body := []html.Node{
		html.El("h1", nil, html.Text(a.heading)),
		html.El("ul", []html.Attr{html.A("id", "rows")}, items...),
		html.FormTo(rt.MsgRef{Module: "R", Msg: "Add"}, []html.Attr{html.A("id", "add")},
			html.Input(html.Control{Kind: "errors", Form: ansForm}),
			html.Input(html.Control{Kind: "input", Form: ansForm, Name: "title", Label: "Title"}),
			html.Input(html.Control{Kind: "errors", Form: ansForm, Name: "title"})),
	}
	if a.toasts {
		body = append(body, html.El("al-toasts", []html.Attr{html.A("popover", "manual"), html.B("data-ignore-morph", true)}))
	}
	return html.El("html", nil, html.El("head", nil, html.El("title", nil, html.Text("R"))), html.El("body", nil, body...))
}

func titled(title string) html.Node {
	return html.El("html", nil, html.El("head", nil, html.El("title", nil, html.Text(title))), html.El("body", nil, html.El("h1", nil, html.Text(title))))
}

func rowRoute(id int) rt.Route {
	return rt.Route{Name: "R.Row", Method: "GET", Path: "/rows/" + strconv.Itoa(id)}
}

func (a *ansApp) routes() []Route {
	return []Route{
		{Method: "GET", Pattern: "/", Handle: func(rq *Request) Response {
			a.loads++
			st := rq.FormState
			if st == nil {
				st = &html.FormState{}
			}
			return PageResponse(a.index(), st, nil, false)
		}},
		{Method: "GET", Pattern: "/other", Handle: func(rq *Request) Response { return PageResponse(titled("Other"), nil, nil, false) }},
		{Method: "GET", Pattern: "/login", Handle: func(rq *Request) Response { return PageResponse(titled("Login"), nil, nil, false) }},
		{Method: "GET", Pattern: "/rows/{id}", Handle: func(rq *Request) Response {
			a.rowLoads++
			switch a.rowAnswer {
			case "404":
				return NotFoundResponse(html.None())
			case "303":
				return RedirectResponse("/login", nil)
			case "500":
				return ErrorResponse(html.None(), errors.New("boom"))
			}
			id, _ := strconv.Atoi(rq.Params["id"])
			return PageResponse(a.row(id), &html.FormState{}, nil, false)
		}},
		// POST /w/{kind}?id=N&toast=T&to=P&heading=H: toggles row N, sets the heading, answers kind
		{Method: "POST", Pattern: "/w/{kind}", Handle: func(rq *Request) Response {
			kind := rq.Params["kind"]
			var before *Shown
			if kind == "refresh" || kind == "invalid" {
				before, _ = rq.RenderShown(nil)
			}
			id, _ := strconv.Atoi(rq.Query.Get("id"))
			if id > 0 {
				a.done[id-1] = !a.done[id-1]
			}
			if h := rq.Query.Get("heading"); h != "" {
				a.heading = h
			}
			var ans rt.Response
			switch kind {
			case "redirect":
				to := rq.Query.Get("to")
				if to == "" {
					to = "/"
				}
				ans = rt.RedirectTo(rt.Route{Name: "R.Page", Method: "GET", Path: to})
			case "render":
				target := rowRoute(id)
				if p := rq.Query.Get("path"); p != "" {
					target.Path = p
				}
				ans = rt.Render(target)
			case "refresh":
				ans = rt.Refresh()
			case "invalid":
				ans = rt.Invalid("Title taken")
			}
			if t := rq.Query.Get("toast"); t != "" {
				ans = rt.WithToast(rt.NoticeSuccess, t, ans)
			}
			resp := AnswerResponse(ans, before, nil)
			if kind == "invalid" {
				resp.State = &html.FormState{Form: ansForm, Msg: ansAddKey, Values: map[string]string{"title": rq.Form.Get("title")}}
			}
			return resp
		}},
		// POST /add: a form that did not decode when its title is empty
		{Method: "POST", Pattern: "/add", Handle: func(rq *Request) Response {
			before, _ := rq.RenderShown(nil)
			if rq.Form.Get("title") == "" {
				resp := InvalidResponse(&html.FormState{Form: ansForm, Msg: ansAddKey, Values: map[string]string{"title": ""},
					Errors: map[string][]string{"title": {"must not be empty"}}}, nil)
				resp.Shown = before
				return resp
			}
			a.done = append(a.done, false)
			return AnswerResponse(rt.Refresh(), before, nil)
		}},
	}
}

// staleShown is an Alder-Shown hash no page has: the client's al-shown after an element patch.
const staleShown = "0123456789abcdef"

// ansPost is a Datastar post of target from the page at pageURL ("" sends no Alder-Page), with
// a stale shown hash.
func ansPost(h http.Handler, target, pageURL string, fields url.Values) *httptest.ResponseRecorder {
	req := httptest.NewRequest("POST", target, strings.NewReader(fields.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Datastar-Request", "true")
	req.Header.Set(html.ShownHeader, staleShown)
	if pageURL != "" {
		req.Header.Set(html.PageHeader, pageURL)
	}
	return serveRaw(h, req)
}

func toastEvent(variant, timeout, text string) string {
	return "event: datastar-patch-elements\ndata: selector al-toasts\ndata: mode append\ndata: elements <al-toast variant=\"" + variant + "\" timeout=\"" + timeout + "\">" + text + "</al-toast>"
}

// pushTo reports whether ev is the history push of a followed redirect to u.
func pushTo(ev, u string) bool {
	return strings.Contains(ev, "data: selector body\ndata: mode append\n") && strings.Contains(ev, `var u="`+u+`"`) && strings.Contains(ev, "history.pushState")
}

// redirect r: the target served and morphed in with its title and a history push; no
// before-view (the shown page is not loaded); to the shown route too, still a whole-body morph.
func TestAnswer_Redirect(t *testing.T) {
	a := newAnsApp(3)
	h := NewRouter(a.routes(), testOpts(nil))
	w := ansPost(h, "/w/redirect?to=/other&id=1", "/", nil)
	evs := events(w.Body.String())
	if len(evs) != 3 || !isBodyMorph(t, w) || !strings.Contains(evs[0], "<h1>Other</h1>") || !strings.Contains(evs[1], "<title>Other</title>") || !pushTo(evs[2], "/other") {
		t.Fatalf("%q", evs)
	}
	if a.loads != 0 {
		t.Fatalf("GET / ran %d times: a redirect renders no before-view", a.loads)
	}
	// to the page the browser is on: still a morph of the whole body, the page loaded once
	w = ansPost(h, "/w/redirect?to=/&id=2", "/", nil)
	evs = events(w.Body.String())
	if len(evs) != 3 || !isBodyMorph(t, w) || !strings.Contains(evs[0], `<li id="row-2" class="done">`) || !pushTo(evs[2], "/") {
		t.Fatalf("%q", evs)
	}
	if a.loads != 1 {
		t.Fatalf("GET / ran %d times, want 1 (the target's only)", a.loads)
	}
	if w.Header().Get(StatusHeader) != "" || w.Code != 200 {
		t.Fatalf("%d %q", w.Code, w.Header().Get(StatusHeader))
	}
}

// render r: one patch of r's view by its root id, mode outer; the page is never loaded; the
// answer's toasts follow the patch.
func TestAnswer_Render(t *testing.T) {
	a := newAnsApp(10)
	h := NewRouter(a.routes(), testOpts(nil))
	w := ansPost(h, "/w/render?id=7&toast=Saved", "/", nil)
	evs := events(w.Body.String())
	want := []string{regionEvent("row-7", `<li id="row-7" class="done">Row 7</li>`), toastEvent("success", "5000", "Saved")}
	if strings.Join(evs, "\n\n") != strings.Join(want, "\n\n") {
		t.Fatalf("events %q\nwant %q", evs, want)
	}
	if a.loads != 0 || a.rowLoads != 1 {
		t.Fatalf("loads %d, row loads %d", a.loads, a.rowLoads)
	}
	if w.Code != 200 || w.Header().Get(StatusHeader) != "" || w.Header().Get("Content-Type") != "text/event-stream" {
		t.Fatalf("%d %q %q", w.Code, w.Header().Get(StatusHeader), w.Header().Get("Content-Type"))
	}
}

// refresh: the shown page loaded again and diffed against the before-view; only the regions the
// server changed are sent, even though the client's al-shown is stale (an element patch made it
// so): no whole-body morph.
func TestAnswer_Refresh(t *testing.T) {
	a := newAnsApp(10)
	h := NewRouter(a.routes(), testOpts(nil))
	w := ansPost(h, "/w/refresh?id=7", "/", nil)
	evs := events(w.Body.String())
	newHash := html.Hash(bodyOf(html.RenderWith(a.index(), &html.FormState{})))
	want := []string{
		regionEvent("row-7", `<li id="row-7" class="done">Row 7</li>`),
		regionEvent("_alder_shown", `<al-shown id="_alder_shown" data-h="`+newHash+`" hidden></al-shown>`),
	}
	if strings.Join(evs, "\n\n") != strings.Join(want, "\n\n") {
		t.Fatalf("events %q\nwant %q", evs, want)
	}
	if a.loads != 2 {
		t.Fatalf("GET / ran %d times, want 2 (before and after)", a.loads)
	}
	// with its own <al-toasts>, regions then the toast
	a = newAnsApp(10)
	a.toasts = true
	h = NewRouter(a.routes(), testOpts(nil))
	w = ansPost(h, "/w/refresh?id=7&toast=Saved", "/", nil)
	evs = events(w.Body.String())
	if len(evs) != 3 || evs[0] != regionEvent("row-7", `<li id="row-7" class="done">Row 7</li>`) ||
		!strings.Contains(evs[1], "selector #_alder_shown") || evs[2] != toastEvent("success", "5000", "Saved") {
		t.Fatalf("events %q", evs)
	}
	for _, other := range []string{"#rows", "#add", "row-6", "row-8", "selector body"} {
		if strings.Contains(w.Body.String(), other) {
			t.Fatalf("%s sent: %q", other, evs)
		}
	}
	// nothing changed: no element event at all
	w = ansPost(h, "/w/refresh", "/", nil)
	if w.Body.String() != "" || w.Code != 200 || w.Header().Get("Content-Type") != "text/event-stream" {
		t.Fatalf("%d %q", w.Code, w.Body.String())
	}
	// a change outside every region still morphs the body
	w = ansPost(h, "/w/refresh?heading=Other", "/", nil)
	if !isBodyMorph(t, w) || !strings.Contains(w.Body.String(), "<h1>Other</h1>") {
		t.Fatalf("%q", w.Body.String())
	}
}

// invalid t: 422 in Alder-Status; the shown page with the posted form's values and t on that
// form, diffed against the before-view: only the form is patched. Its update committed.
func TestAnswer_Invalid(t *testing.T) {
	a := newAnsApp(3)
	h := NewRouter(a.routes(), testOpts(nil))
	w := ansPost(h, "/w/invalid?id=2", "/", url.Values{"title": {"dup"}})
	evs := events(w.Body.String())
	if w.Code != 200 || w.Header().Get(StatusHeader) != "422" {
		t.Fatalf("%d %q", w.Code, w.Header().Get(StatusHeader))
	}
	if len(evs) != 3 || evs[0] != regionEvent("row-2", `<li id="row-2" class="done">Row 2</li>`) ||
		!strings.HasPrefix(evs[1], "event: datastar-patch-elements\ndata: selector #add\ndata: mode outer\n") ||
		!strings.Contains(evs[1], `value="dup"`) || !strings.Contains(evs[1], "Title taken") || !strings.Contains(evs[2], "selector #_alder_shown") {
		t.Fatalf("events %q", evs)
	}
	if !a.done[1] || a.loads != 2 {
		t.Fatalf("done %v loads %d", a.done, a.loads)
	}
	// a form that does not decode: answered like invalid, with the field errors, update not called
	w = ansPost(h, "/add", "/", url.Values{"title": {""}})
	evs = events(w.Body.String())
	if w.Header().Get(StatusHeader) != "422" || len(evs) != 2 || !strings.HasPrefix(evs[0], "event: datastar-patch-elements\ndata: selector #add\n") ||
		!strings.Contains(evs[0], "must not be empty") || len(a.done) != 3 {
		t.Fatalf("%q %q", w.Header().Get(StatusHeader), evs)
	}
}

// render r whose r.load fails: never the failure page patched in place of the element. 404: no
// element patch, an error toast "Not found"; a redirect: the navigation; 500: logged, the error
// toast.
func TestAnswer_RenderLoadFails(t *testing.T) {
	a := newAnsApp(3)
	a.toasts = true
	a.rowAnswer = "404"
	var logged []string
	opts := testOpts(func(o *Options) { o.Log = func(level, msg string) { logged = append(logged, level+" "+msg) } })
	h := NewRouter(a.routes(), opts)
	w := ansPost(h, "/w/render?id=2&toast=Saved", "/", nil)
	evs := events(w.Body.String())
	want := []string{toastEvent("success", "5000", "Saved"), toastEvent("destructive", "10000", "Not found")}
	if strings.Join(evs, "\n\n") != strings.Join(want, "\n\n") || w.Code != 200 || w.Header().Get("Content-Type") != "text/event-stream" {
		t.Fatalf("404: %d %q", w.Code, evs)
	}

	// a target no GET route matches: the same, without running anything
	w = ansPost(h, "/w/render?id=2&path=/nowhere/at/all", "/", nil)
	if evs := events(w.Body.String()); len(evs) != 1 || evs[0] != toastEvent("destructive", "10000", "Not found") {
		t.Fatalf("unmatched: %q", evs)
	}

	a.rowAnswer = "303"
	w = ansPost(h, "/w/render?id=2", "/", nil)
	evs = events(w.Body.String())
	if len(evs) != 3 || !isBodyMorph(t, w) || !strings.Contains(evs[0], "<h1>Login</h1>") || !pushTo(evs[2], "/login") {
		t.Fatalf("303: %q", evs)
	}

	a.rowAnswer = "500"
	logged = nil
	w = ansPost(h, "/w/render?id=2", "/", nil)
	evs = events(w.Body.String())
	if len(evs) != 1 || evs[0] != toastEvent("destructive", "10000", "Something went wrong") || strings.Contains(w.Body.String(), "boom") {
		t.Fatalf("500: %q", evs)
	}
	if len(logged) != 1 || !strings.HasPrefix(logged[0], "error ") || !strings.Contains(logged[0], "boom") || !strings.Contains(logged[0], "/rows/2") {
		t.Fatalf("500 logged %q", logged)
	}
}

// A forged Alder-Page never makes refresh name another host: each resolves to "/", whose rows
// are patched; an oversized value is "/".
func TestAnswer_ForgedShownPage(t *testing.T) {
	for _, forged := range []string{"//evil.example/x", "https://evil.example/", `/\evil`, "/\t/evil.example", "javascript:alert(1)", "/" + strings.Repeat("a", 9*1024)} {
		a := newAnsApp(3)
		a.toasts = true
		h := NewRouter(a.routes(), testOpts(nil))
		w := ansPost(h, "/w/refresh?id=1", forged, nil)
		evs := events(w.Body.String())
		if strings.Contains(w.Body.String(), "evil") || strings.Contains(w.Body.String(), "aaaa") || len(evs) != 2 ||
			evs[0] != regionEvent("row-1", `<li id="row-1" class="done">Row 1</li>`) || a.loads != 2 {
			n := forged
			if len(n) > 30 {
				n = n[:30]
			}
			t.Fatalf("%q: loads %d %q", n, a.loads, evs)
		}
	}
}

// An Alder-Page no GET route matches (a stale build's path): refresh answers its plain HTTP, a
// 303 to that path, which under Datastar is a navigation.
func TestAnswer_ShownPageUnmatched(t *testing.T) {
	a := newAnsApp(3)
	h := NewRouter(a.routes(), testOpts(nil))
	w := ansPost(h, "/w/refresh?id=1", "/gone?x=1", nil)
	evs := events(w.Body.String())
	if len(evs) != 1 || !strings.Contains(evs[0], `window.location.assign("/gone?x=1")`) || a.loads != 0 || !a.done[0] {
		t.Fatalf("loads %d %q", a.loads, evs)
	}
}

// render leaves al-shown as it is; a refresh that morphs the whole body carries the new one.
func TestAnswer_RenderKeepsShown(t *testing.T) {
	a := newAnsApp(3)
	h := NewRouter(a.routes(), testOpts(nil))
	w := ansPost(h, "/w/render?id=1", "/", nil)
	if strings.Contains(w.Body.String(), "_alder_shown") {
		t.Fatalf("%q", w.Body.String())
	}
	w = ansPost(h, "/w/refresh?heading=New", "/", nil)
	want := html.Hash(bodyOf(html.RenderWith(a.index(), &html.FormState{})))
	if !isBodyMorph(t, w) || shownOf(w.Body.String()) != want {
		t.Fatalf("%q", w.Body.String())
	}
}

// A toast on a redirect answered to a request without JavaScript is stored in the notice cookie
// and shown by the next page, as Notice.with's is.
func TestAnswer_ToastOn303Survives(t *testing.T) {
	a := newAnsApp(3)
	h := NewRouter(a.routes(), testOpts(nil))
	req := httptest.NewRequest("POST", "/w/redirect?to=/other&toast=Added", strings.NewReader(""))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := serveRaw(h, req)
	if w.Code != http.StatusSeeOther || w.Header().Get("Location") != "/other" {
		t.Fatalf("%d %q", w.Code, w.Header().Get("Location"))
	}
	var notice *http.Cookie
	for _, c := range w.Result().Cookies() {
		if c.Name == NoticeCookie {
			notice = c
		}
	}
	if notice == nil {
		t.Fatalf("no notice cookie: %v", w.Header())
	}
	next := httptest.NewRequest("GET", "/other", nil)
	next.AddCookie(notice)
	w = serveRaw(h, next)
	if !strings.Contains(w.Body.String(), `<al-alert variant="success" role="status">Added</al-alert>`) {
		t.Fatalf("%s", w.Body.String())
	}
}

// The shown page's GET runs with an Env of its own, as a GET request's (serve): never the write's
// narrowing, store binding, SessionPut or trace state, only the session the write left. Both the
// before-view and the after-view run so.
func TestAnswer_ShownRunsWithFreshEnv(t *testing.T) {
	type seen struct {
		keys       map[string]string
		siloKey    string
		bound, put bool
		session    any
	}
	var runs []seen
	routes := []Route{
		{Method: "GET", Pattern: "/", Handle: func(rq *Request) Response {
			e := rq.Env
			runs = append(runs, seen{keys: e.Keys, siloKey: e.SiloKey, bound: e.Bound, put: e.SessionPut, session: e.Session})
			return PageResponse(doc("p"), &html.FormState{}, nil, false)
		}},
		{Method: "POST", Pattern: "/w", Handle: func(rq *Request) Response {
			// what db.Narrow and db.Enter leave on a write's Env, before its before-view
			rq.Env.Keys = map[string]string{"org": "A"}
			rq.Env.SiloKey, rq.Env.SiloValue, rq.Env.Bound = "org", "A", true
			before, _ := rq.RenderShown(nil)
			// the update: Session.put switches to user B
			rq.Env.SessionPut = true
			rq.Env.Session = testSession{User: "B"}
			return AnswerResponse(rt.Refresh(), before, testSession{User: "B"})
		}},
	}
	h := NewRouter(routes, testOpts(nil))
	ansPost(h, "/w", "/", nil)
	if len(runs) != 2 {
		t.Fatalf("runs %+v", runs)
	}
	for i, s := range runs {
		if len(s.keys) != 0 || s.siloKey != "" || s.bound || s.put {
			t.Errorf("run %d saw the write's Env: %+v", i, s)
		}
	}
	if runs[0].session != (testSession{}) || runs[1].session != (testSession{User: "B"}) {
		t.Fatalf("sessions %v, %v", runs[0].session, runs[1].session)
	}
}
