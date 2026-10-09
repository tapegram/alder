package web

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"alder/rt/datastar"
	"alder/rt/html"
)

func one(method, pattern string, resp func(rq *Request) Response) []Route {
	return []Route{{Method: method, Pattern: pattern, Handle: resp}}
}

func sessionCookie(t *testing.T, w *httptest.ResponseRecorder) *http.Cookie {
	t.Helper()
	for _, c := range w.Result().Cookies() {
		if c.Name == CookieName {
			return c
		}
	}
	return nil
}

func TestResponse_PageFullHTML(t *testing.T) {
	key := []byte("test-secret-test-secret")
	st := &html.FormState{Form: "f", Values: map[string]string{"title": "kept"}}
	page := html.El("html", nil,
		html.El("head", []html.Attr{html.A("lang", "en")}, html.El("title", nil, html.Text("T"))),
		html.El("body", nil, html.Input(html.Control{Kind: "input", Form: "f", Name: "title", Type: "text"})))
	h := NewRouter(one("GET", "/", func(rq *Request) Response {
		return PageResponse(page, st, testSession{User: "ada"}, false)
	}), testOpts(nil))
	w := serve(h, httptest.NewRequest("GET", "/", nil))
	body := w.Body.String()
	if w.Code != 200 || w.Header().Get("Content-Type") != "text/html; charset=utf-8" {
		t.Fatalf("status %d ct %q", w.Code, w.Header().Get("Content-Type"))
	}
	script := html.Render(datastar.ScriptTag())
	want := "<!doctype html>\n<html><head lang=\"en\">" + script + "<title>T</title></head><body>"
	if !strings.HasPrefix(body, want) || !strings.HasSuffix(body, "</body></html>") {
		t.Fatalf("body %q", body)
	}
	if !strings.Contains(body, `value="kept"`) {
		t.Fatalf("form state not rendered: %q", body)
	}
	c := sessionCookie(t, w)
	if c == nil || !c.HttpOnly || c.Path != "/" || c.SameSite != http.SameSiteLaxMode {
		t.Fatalf("cookie %+v", c)
	}
	payload, ok := Verify(key, c.Value)
	if !ok || string(payload) != `{"User":"ada"}` {
		t.Fatalf("cookie payload %q %v", payload, ok)
	}

	// Session nil: no cookie. No <head>: the script goes right after the doctype.
	h = NewRouter(one("GET", "/", func(rq *Request) Response {
		return PageResponse(html.El("p", nil, html.Text("bare")), nil, nil, false)
	}), testOpts(nil))
	w = serve(h, httptest.NewRequest("GET", "/", nil))
	if sessionCookie(t, w) != nil {
		t.Fatal("cookie set for a nil session")
	}
	if got := w.Body.String(); got != "<!doctype html>\n"+script+"<p>bare</p>" {
		t.Fatalf("headless body %q", got)
	}
	// <header> is not <head>.
	h = NewRouter(one("GET", "/", func(rq *Request) Response {
		return PageResponse(html.El("body", nil, html.El("header", nil), html.El("head", nil)), nil, nil, false)
	}), testOpts(nil))
	if got := serve(h, httptest.NewRequest("GET", "/", nil)).Body.String(); got != "<!doctype html>\n<body><header></header><head>"+script+"</head></body>" {
		t.Fatalf("header body %q", got)
	}
}

func TestResponse_PagePatchUnderDatastar(t *testing.T) {
	h := NewRouter(one("POST", "/_m/Todos/Add", func(rq *Request) Response {
		if !rq.Datastar {
			t.Error("rq.Datastar false")
		}
		return PageResponse(doc("patched"), nil, testSession{User: "bo"}, true)
	}), testOpts(nil))
	req := httptest.NewRequest("POST", "/_m/Todos/Add", strings.NewReader("{}"))
	req.Header.Set("Datastar-Request", "true")
	w := serve(h, req)
	if w.Code != 200 || w.Header().Get("Content-Type") != "text/event-stream" {
		t.Fatalf("status %d ct %q", w.Code, w.Header().Get("Content-Type"))
	}
	want := "event: datastar-patch-elements\ndata: selector body\ndata: mode outer\ndata: elements <body><p>patched</p></body>\n\n"
	if w.Body.String() != want {
		t.Fatalf("body %q", w.Body.String())
	}
	if c := sessionCookie(t, w); c == nil {
		t.Fatal("no cookie on a patch")
	}

	// No <body> in the document: the whole document is wrapped in one.
	h = NewRouter(one("POST", "/_m/A/B", func(rq *Request) Response {
		return PageResponse(html.El("p", nil, html.Text("x")), nil, nil, true)
	}), testOpts(nil))
	req = httptest.NewRequest("POST", "/_m/A/B", nil)
	req.Header.Set("Datastar-Request", "true")
	if got := serve(h, req).Body.String(); !strings.Contains(got, "data: elements <body><p>x</p></body>\n") {
		t.Fatalf("bodiless patch %q", got)
	}

	// Patch without the Datastar header: a full page.
	req = httptest.NewRequest("POST", "/_m/A/B", nil)
	w = serve(h, req)
	if !strings.HasPrefix(w.Body.String(), "<!doctype html>\n") || w.Header().Get("Content-Type") != "text/html; charset=utf-8" {
		t.Fatalf("non-datastar patch %q", w.Body.String())
	}
}

func TestResponse_PageNoPatchWithoutFlag(t *testing.T) {
	h := NewRouter(one("GET", "/", func(rq *Request) Response {
		return PageResponse(doc("full"), nil, nil, false)
	}), testOpts(nil))
	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Datastar-Request", "true")
	w := serve(h, req)
	if w.Code != 200 || w.Header().Get("Content-Type") != "text/html; charset=utf-8" || !strings.HasPrefix(w.Body.String(), "<!doctype html>\n<html><head>") {
		t.Fatalf("status %d ct %q body %q", w.Code, w.Header().Get("Content-Type"), w.Body.String())
	}
}

func TestResponse_Redirect303(t *testing.T) {
	h := NewRouter(one("POST", "/_m/Auth/Login", func(rq *Request) Response {
		return RedirectResponse("/todos", testSession{User: "ada"})
	}), testOpts(nil))
	for _, ds := range []bool{false, true} {
		req := httptest.NewRequest("POST", "/_m/Auth/Login", nil)
		if ds {
			req.Header.Set("Datastar-Request", "true")
		}
		w := serve(h, req)
		if w.Code != 303 || w.Header().Get("Location") != "/todos" || sessionCookie(t, w) == nil {
			t.Fatalf("datastar=%v: %d %q", ds, w.Code, w.Header().Get("Location"))
		}
	}
	// requires: the handler redirects to rq.Login with the session unchanged.
	h = NewRouter(one("GET", "/secret", func(rq *Request) Response {
		return RedirectResponse(rq.Login, nil)
	}), testOpts(nil))
	w := serve(h, httptest.NewRequest("GET", "/secret", nil))
	if w.Code != 303 || w.Header().Get("Location") != "/login" || sessionCookie(t, w) != nil {
		t.Fatalf("login redirect %d %q", w.Code, w.Header().Get("Location"))
	}
}

func TestResponse_ErrorHidesTextInProd(t *testing.T) {
	for _, dev := range []bool{false, true} {
		var logs []string
		h := NewRouter(one("POST", "/_m/A/B", func(rq *Request) Response {
			return ErrorResponse(doc("Something went wrong"), errors.New("db is -- down"))
		}), testOpts(func(o *Options) {
			o.Dev = dev
			o.Log = func(level, msg string) { logs = append(logs, level+": "+msg) }
		}))
		req := httptest.NewRequest("POST", "/_m/A/B", nil)
		req.Header.Set("Datastar-Request", "true")
		w := serve(h, req)
		body := w.Body.String()
		if w.Code != 500 || w.Header().Get("Content-Type") != "text/html; charset=utf-8" || !strings.Contains(body, "<p>Something went wrong</p>") {
			t.Fatalf("dev=%v: %d %q", dev, w.Code, body)
		}
		if sessionCookie(t, w) != nil {
			t.Fatalf("dev=%v: error set the cookie", dev)
		}
		if dev {
			if !strings.HasSuffix(body, "</html><!-- error: db is - - down -->") {
				t.Fatalf("dev body %q", body)
			}
		} else if strings.Contains(body, "down") || strings.Contains(body, "<!--") {
			t.Fatalf("prod body leaks the error: %q", body)
		}
		if len(logs) != 1 || logs[0] != "error: POST /_m/A/B: db is -- down" {
			t.Fatalf("dev=%v: logs %q", dev, logs)
		}
	}

	// A zero Node falls back to Options.ErrorDoc; NotFoundResponse renders its doc with 404.
	h := NewRouter([]Route{
		{Method: "GET", Pattern: "/e", Handle: func(rq *Request) Response { return ErrorResponse(html.None(), errors.New("x")) }},
		{Method: "GET", Pattern: "/n", Handle: func(rq *Request) Response { return NotFoundResponse(doc("No such todo")) }},
	}, testOpts(nil))
	if w := serve(h, httptest.NewRequest("GET", "/e", nil)); w.Code != 500 || !strings.Contains(w.Body.String(), "<p>Something went wrong</p>") {
		t.Fatalf("fallback error doc: %d %q", w.Code, w.Body.String())
	}
	if w := serve(h, httptest.NewRequest("GET", "/n", nil)); w.Code != 404 || !strings.Contains(w.Body.String(), "<p>No such todo</p>") {
		t.Fatalf("not found response: %d %q", w.Code, w.Body.String())
	}
}
