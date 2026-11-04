package web

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"alder/rt/datastar"
	"alder/rt/html"
)

// Delivery decided by the request (#160): a GET route's view goes out as a document (a plain
// request), as a body morph with its title and a history step (a boosted link: Datastar without
// Alder-Fetch), or as one element patch by its root's id (an element fetch: Alder-Fetch: 1).

// elementGet is an element fetch of target: a Datastar GET carrying Alder-Fetch: 1.
func elementGet(target string) *http.Request {
	req := dsGet(target)
	req.Header.Set(datastar.FetchHeader, "1")
	return req
}

// todoRoute is GET /todos/{id} answering view.
func todoRoute(view html.Node) []Route {
	return one("GET", "/todos/{id}", func(rq *Request) Response { return PageResponse(view, nil, nil, false) })
}

func todoItem(attrs ...html.Attr) html.Node {
	return html.El("li", attrs, html.Text("Buy milk"))
}

func TestDeliver_PlainDocument(t *testing.T) {
	t.Setenv("ALDER_DEV", "1")
	h := NewRouter(todoRoute(doc("Todo 7")), testOpts(func(o *Options) { o.Dev = true }))
	w := serve(h, httptest.NewRequest("GET", "/todos/7", nil))
	body := w.Body.String()
	if w.Code != http.StatusOK || !strings.HasPrefix(w.Header().Get("Content-Type"), "text/html") {
		t.Fatalf("got %d %q", w.Code, w.Header().Get("Content-Type"))
	}
	for _, s := range []string{"<!doctype html>\n<html>", "<title>Todo 7</title>", datastar.ScriptPath, "<p>Todo 7</p>", "<al-devpanel"} {
		if !strings.Contains(body, s) {
			t.Errorf("missing %q in %q", s, body)
		}
	}
}

func TestDeliver_BoostMorph(t *testing.T) {
	for _, fetch := range []string{"", "0"} { // no Alder-Fetch (a tab of an older build too), or not "1"
		h := NewRouter(todoRoute(doc("Todo 7")), testOpts(nil))
		req := dsGet("/todos/7?datastar=%7B%7D")
		if fetch != "" {
			req.Header.Set(datastar.FetchHeader, fetch)
		}
		w := serve(h, req)
		ev := events(w.Body.String())
		if w.Code != http.StatusOK || len(ev) != 3 {
			t.Fatalf("fetch %q: got %d %q", fetch, w.Code, w.Body.String())
		}
		if ev[0] != bodyEvent("<p>Todo 7</p>") || ev[1] != titleEvent("Todo 7") {
			t.Fatalf("fetch %q: events %q", fetch, ev)
		}
		historyEvent(t, ev[2], pushOpen, `"/todos/7"`)
	}
}

func TestDeliver_ElementPatch(t *testing.T) {
	for _, tc := range []struct {
		name string
		view html.Node
		want string // the elements line(s)
	}{
		{"one root", todoItem(html.A("id", "todo-7")), `<li id="todo-7">Buy milk</li>`},
		{"whitespace around it", html.Nodes(html.Text("\n  "), html.Nodes(todoItem(html.A("id", "todo-7"))), html.Text(" ")),
			"\n  " + `<li id="todo-7">Buy milk</li> `},
		// Datastar v1.0.0 morphs every top-level element of the event in (TestBrowser_DatastarMultiRoot):
		// the first root's id is the target, as "load more" wants (the next rows and a new sentinel).
		{"several roots", html.Nodes(todoItem(html.A("id", "todo-7")), html.El("li", []html.Attr{html.A("id", "more")})),
			`<li id="todo-7">Buy milk</li><li id="more"></li>`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var logs []string
			h := NewRouter(todoRoute(tc.view), testOpts(func(o *Options) { o.Log = func(level, msg string) { logs = append(logs, level+": "+msg) } }))
			w := serve(h, elementGet("/todos/7?datastar=%7B%7D"))
			want := string(datastar.PatchEvent("#todo-7", "outer", tc.want))
			if w.Code != http.StatusOK || w.Body.String() != want {
				t.Fatalf("got %d %q, want %q", w.Code, w.Body.String(), want)
			}
			if ct := w.Header().Get("Content-Type"); ct != "text/event-stream" {
				t.Fatalf("Content-Type %q", ct)
			}
			if strings.Contains(w.Body.String(), "<body") || strings.Contains(w.Body.String(), "history.") {
				t.Fatalf("an element patch with a body or a history step: %q", w.Body.String())
			}
			if len(logs) != 0 {
				t.Fatalf("logged %q", logs)
			}
		})
	}
}

// An element fetch whose route answers a 404 morphs the not-found page into the body, as a
// message's 404 does: no history step to the element's URL.
func TestDeliver_ElementNotFound(t *testing.T) {
	h := NewRouter(one("GET", "/todos/{id}", func(rq *Request) Response {
		return Response{Status: http.StatusNotFound, Node: doc("No such todo")}
	}), testOpts(nil))
	w := serve(h, elementGet("/todos/7"))
	ev := events(w.Body.String())
	if w.Code != http.StatusOK || len(ev) != 1 || ev[0] != bodyEvent("<p>No such todo</p>") {
		t.Fatalf("got %d %q", w.Code, w.Body.String())
	}
}

func TestDeliver_ElementNoRootID(t *testing.T) {
	for _, tc := range []struct {
		name string
		view html.Node
	}{
		{"a root without an id", todoItem()},
		{"a first root without an id", html.Nodes(todoItem(), todoItem(html.A("id", "todo-7")))},
		{"a text root", html.Nodes(html.Text("Buy milk"))},
		{"nothing", html.None()},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var logs []string
			h := NewRouter(todoRoute(tc.view), testOpts(func(o *Options) { o.Log = func(level, msg string) { logs = append(logs, level+": "+msg) } }))
			w := serve(h, elementGet("/todos/7"))
			if w.Code != http.StatusOK || w.Body.String() != "" || w.Header().Get("Content-Type") != "text/event-stream" {
				t.Fatalf("got %d %q %q, want an empty event stream", w.Code, w.Header().Get("Content-Type"), w.Body.String())
			}
			if len(logs) != 1 || !strings.HasPrefix(logs[0], "error: GET /todos/7: ") || !strings.Contains(logs[0], "route GET /todos/{id}") || !strings.Contains(logs[0], "id") {
				t.Fatalf("logs %q", logs)
			}
		})
	}
	t.Run("the dev panel says so", func(t *testing.T) {
		t.Setenv("ALDER_DEV", "1")
		h := NewRouter(todoRoute(todoItem()), testOpts(func(o *Options) { o.Dev = true; o.Log = func(string, string) {} }))
		w := serve(h, elementGet("/todos/7"))
		ev := events(w.Body.String())
		if len(ev) != 1 || !strings.Contains(ev[0], "data: selector #"+datastar.PanelID) {
			t.Fatalf("events %q, want the panel's alone", ev)
		}
		if !strings.Contains(ev[0], "no element patched") {
			t.Fatalf("the panel does not say no element was patched: %q", ev[0])
		}
		if !strings.Contains(ev[0], "-H &#39;"+datastar.FetchHeader+": 1&#39;") {
			t.Fatalf("the panel's curl is not an element fetch: %q", ev[0])
		}
	})
}

func TestDeliver_PartialAsDocument(t *testing.T) {
	h := NewRouter(todoRoute(todoItem(html.A("id", "todo-7"))), testOpts(nil))
	w := serve(h, httptest.NewRequest("GET", "/todos/7", nil))
	body := w.Body.String()
	if w.Code != http.StatusOK || !strings.HasPrefix(body, "<!doctype html>\n") || !strings.Contains(body, `<li id="todo-7">Buy milk</li>`) {
		t.Fatalf("got %d %q", w.Code, body)
	}
}

// An element fetch's handler sees the query without the signals parameter Datastar adds to
// every GET, as a boosted GET's does.
func TestDeliver_ElementDropsSignals(t *testing.T) {
	var got string
	h := NewRouter(one("GET", "/todos/{id}", func(rq *Request) Response {
		got = rq.Query.Encode()
		return PageResponse(todoItem(html.A("id", "todo-7")), nil, nil, false)
	}), testOpts(nil))
	serve(h, elementGet("/todos/7?b=2&datastar=%7B%7D&a=1"))
	if got != "a=1&b=2" {
		t.Fatalf("query %q", got)
	}
}

// An element fetch whose route redirects to a page of this app (a `requires` to the login page)
// is a navigation: the page is followed and morphed in, with a history push of its URL.
func TestDeliver_ElementRedirectNavigates(t *testing.T) {
	routes := append(one("GET", "/todos/{id}", func(rq *Request) Response {
		return Response{Status: http.StatusSeeOther, Location: "/login"}
	}), one("GET", "/login", func(rq *Request) Response { return PageResponse(doc("Log in"), nil, nil, false) })...)
	h := NewRouter(routes, testOpts(nil))
	w := serve(h, elementGet("/todos/7"))
	ev := events(w.Body.String())
	if w.Code != http.StatusOK || len(ev) != 3 || ev[0] != bodyEvent("<p>Log in</p>") || ev[1] != titleEvent("Log in") {
		t.Fatalf("got %d %q", w.Code, w.Body.String())
	}
	historyEvent(t, ev[2], pushOpen, `"/login"`)
}
