package web

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"alder/rt"
	"alder/rt/html"
)

// A Query.redirect to a page is a 303 with the session left alone; to a fragment ("#", an
// unsafe Url.external, or "#frag") a 500, since the GET would load itself again in a loop.
func TestQueryRedirectResponse(t *testing.T) {
	if r := QueryRedirectResponse("/builder/7"); r.Status != http.StatusSeeOther || r.Location != "/builder/7" || r.Session != nil {
		t.Fatalf("%+v", r)
	}
	for _, loc := range []string{"#", "#frag"} {
		r := QueryRedirectResponse(loc)
		if r.Status != http.StatusInternalServerError || r.Location != "" || r.Err == nil || !strings.Contains(r.Err.Error(), "refusing to navigate to") {
			t.Fatalf("%q: %+v", loc, r)
		}
	}
}

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
	add := rt.Route{Name: "M.Add", Method: "POST", Path: "/add"}
	st := &html.FormState{Form: "f", Msg: rt.RouteKey(add), Values: map[string]string{"title": "kept"}}
	page := html.El("html", nil,
		html.El("head", []html.Attr{html.A("lang", "en")}, html.El("title", nil, html.Text("T"))),
		html.El("body", nil, html.FormToRoute(add, nil, html.Input(html.Control{Kind: "input", Form: "f", Name: "title", Type: "text"}))))
	h := NewRouter(one("GET", "/", func(rq *Request) Response {
		return PageResponse(page, st, testSession{User: "ada"})
	}), testOpts(func(o *Options) { o.Now = func() time.Time { return time.Unix(1000, 0) } }))
	w := serve(h, httptest.NewRequest("GET", "/", nil))
	body := w.Body.String()
	if w.Code != 200 || w.Header().Get("Content-Type") != "text/html; charset=utf-8" {
		t.Fatalf("status %d ct %q", w.Code, w.Header().Get("Content-Type"))
	}
	first, boost := HeadTags()
	script := first + boost
	want := "<!doctype html>\n<html><head lang=\"en\">" + first + "<title>T</title>" + boost + "</head><body>"
	if !strings.HasPrefix(body, want) || !strings.HasSuffix(body, "</body></html>") {
		t.Fatalf("body %q", body)
	}
	if !strings.Contains(body, `value="kept"`) {
		t.Fatalf("form state not rendered: %q", body)
	}
	c := sessionCookie(t, w)
	if c == nil || !c.HttpOnly || c.Path != "/" || c.SameSite != http.SameSiteLaxMode || c.MaxAge != 30*24*60*60 || c.Expires.IsZero() || c.Secure {
		t.Fatalf("cookie %+v", c)
	}
	payload, ok := Verify(key, c.Value)
	if !ok || string(payload) != `{"iss":1000,"session":{"User":"ada"}}` {
		t.Fatalf("cookie payload %q %v", payload, ok)
	}

	// Session nil: no cookie. No <head>: the script goes right after the doctype.
	h = NewRouter(one("GET", "/", func(rq *Request) Response {
		return PageResponse(html.El("p", nil, html.Text("bare")), nil, nil)
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
		return PageResponse(html.El("body", nil, html.El("header", nil), html.El("head", nil)), nil, nil)
	}), testOpts(nil))
	if got := serve(h, httptest.NewRequest("GET", "/", nil)).Body.String(); got != "<!doctype html>\n<body><header></header><head>"+script+"</head></body>" {
		t.Fatalf("header body %q", got)
	}
}

// A page a POST handler answers (PageResponse) is a whole document even under Datastar (a
// Datastar GET is a boosted navigation instead: boost_test.go).
func TestResponse_PageNoPatchWithoutFlag(t *testing.T) {
	h := NewRouter(one("POST", "/a/b", func(rq *Request) Response {
		return PageResponse(doc("full"), nil, nil)
	}), testOpts(nil))
	req := httptest.NewRequest("POST", "/a/b", nil)
	req.Header.Set("Datastar-Request", "true")
	w := serve(h, req)
	if w.Code != 200 || w.Header().Get("Content-Type") != "text/html; charset=utf-8" || !strings.HasPrefix(w.Body.String(), "<!doctype html>\n<html><head>") {
		t.Fatalf("status %d ct %q body %q", w.Code, w.Header().Get("Content-Type"), w.Body.String())
	}
}

func TestResponse_Redirect303(t *testing.T) {
	h := NewRouter(one("POST", "/auth/login", func(rq *Request) Response {
		return RedirectResponse("/todos?f=a&b", testSession{User: "ada"})
	}), testOpts(nil))
	w := serve(h, httptest.NewRequest("POST", "/auth/login", nil))
	if w.Code != 303 || w.Header().Get("Location") != "/todos?f=a&b" || sessionCookie(t, w) == nil || w.Body.Len() != 0 {
		t.Fatalf("%d %q %q", w.Code, w.Header().Get("Location"), w.Body.String())
	}
	// Under Datastar (C1, M13): a 200 SSE answer, the session stored all the same; no Location
	// (the client's fetch would follow it and patch the new page in place). The target is a
	// page of this app, so the router serves it itself and answers with its morph and a history
	// push (boost_test.go); here it has no route: the not-found page, Alder-Status 404.
	req := httptest.NewRequest("POST", "/auth/login", nil)
	req.Header.Set("Datastar-Request", "true")
	w = serve(h, req)
	if w.Code != 200 || w.Header().Get("Content-Type") != "text/event-stream" || w.Header().Get("Location") != "" || sessionCookie(t, w) == nil ||
		w.Header().Get(StatusHeader) != "404" || !strings.Contains(w.Body.String(), `(function(){var u="/todos?f=a\u0026b";`) {
		t.Fatalf("datastar: %d %q %q\n%q", w.Code, w.Header().Get("Content-Type"), w.Header().Get("Location"), w.Body.String())
	}
	// requires: the handler redirects to rq.Login with the session unchanged.
	h = NewRouter(one("GET", "/secret", func(rq *Request) Response {
		return RedirectResponse(rq.Login, nil)
	}), testOpts(nil))
	w = serve(h, httptest.NewRequest("GET", "/secret", nil))
	if w.Code != 303 || w.Header().Get("Location") != "/login" || sessionCookie(t, w) != nil {
		t.Fatalf("login redirect %d %q", w.Code, w.Header().Get("Location"))
	}
	req = httptest.NewRequest("GET", "/secret", nil)
	req.Header.Set("Datastar-Request", "true")
	w = serve(h, req)
	if w.Code != 200 || !strings.Contains(w.Body.String(), `(function(){var u="/login";`) || sessionCookie(t, w) != nil {
		t.Fatalf("login redirect under Datastar %d %q", w.Code, w.Body.String())
	}
}

func TestResponse_ErrorHidesTextInProd(t *testing.T) {
	for _, ds := range []bool{false, true} {
		for _, dev := range []bool{false, true} {
			var logs []string
			h := NewRouter(one("POST", "/a/b", func(rq *Request) Response {
				return ErrorResponse(doc("Something went wrong"), errors.New("db is -- down"))
			}), testOpts(func(o *Options) {
				o.Dev = dev
				o.Log = func(level, msg string) { logs = append(logs, level+": "+msg) }
			}))
			req := httptest.NewRequest("POST", "/a/b", nil)
			if ds {
				req.Header.Set("Datastar-Request", "true")
			}
			w := serve(h, req)
			body := w.Body.String()
			if ds {
				// Under Datastar: a 200 SSE patch of the error document's body.
				if w.Code != 200 || w.Header().Get("Content-Type") != "text/event-stream" || !strings.Contains(body, "data: elements <body><p>Something went wrong</p>") {
					t.Fatalf("ds dev=%v: %d %q", dev, w.Code, body)
				}
			} else if w.Code != 500 || w.Header().Get("Content-Type") != "text/html; charset=utf-8" || !strings.Contains(body, "<p>Something went wrong</p>") {
				t.Fatalf("dev=%v: %d %q", dev, w.Code, body)
			}
			if sessionCookie(t, w) != nil {
				t.Fatalf("dev=%v: error set the cookie", dev)
			}
			if dev {
				// The comment goes at the end of the <body>, for pages and patches alike.
				if !strings.Contains(body, "<p>Something went wrong</p><!-- error: db is - - down -->") {
					t.Fatalf("ds=%v dev body %q", ds, body)
				}
			} else if strings.Contains(body, "db is") || strings.Contains(body, "<!--") {
				t.Fatalf("ds=%v prod body leaks the error: %q", ds, body)
			}
			if len(logs) != 1 || logs[0] != "error: POST /a/b: db is -- down rid="+w.Header().Get("X-Request-Id") {
				t.Fatalf("ds=%v dev=%v: logs %q", ds, dev, logs)
			}
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

func TestResponse_NotFoundPatchedUnderDatastar(t *testing.T) {
	h := NewRouter(one("POST", "/todos/remove", func(rq *Request) Response {
		return NotFoundResponse(doc("No such todo"))
	}), testOpts(nil))
	for path, want := range map[string]string{
		"/todos/remove": "data: elements <body><p>No such todo</p></body>\n", // handler-returned
		"/todos/nope":   "data: elements <body><p>Not found</p></body>\n",    // no such message
	} {
		req := httptest.NewRequest("POST", path, nil)
		req.Header.Set("Datastar-Request", "true")
		w := serve(h, req)
		if w.Code != 200 || w.Header().Get("Content-Type") != "text/event-stream" || !strings.Contains(w.Body.String(), want) {
			t.Fatalf("%s: %d %q %q", path, w.Code, w.Header().Get("Content-Type"), w.Body.String())
		}
		// Without Datastar: the 404 page.
		w = serve(h, httptest.NewRequest("POST", path, nil))
		if w.Code != 404 || w.Header().Get("Content-Type") != "text/html; charset=utf-8" {
			t.Fatalf("%s without datastar: %d", path, w.Code)
		}
	}
}

// A 404 or 500 response stores a session the handler sets (a committed Task whose page then
// failed to re-load); without one the cookie is left alone.
func TestResponse_FailureKeepsSetSession(t *testing.T) {
	h := NewRouter([]Route{
		{Method: "POST", Pattern: "/n", Handle: func(rq *Request) Response {
			r := NotFoundResponse(html.None())
			r.Session = testSession{User: "u"}
			return r
		}},
		{Method: "POST", Pattern: "/e", Handle: func(rq *Request) Response {
			r := ErrorResponse(html.None(), errors.New("x"))
			r.Session = testSession{User: "v"}
			return r
		}},
		{Method: "POST", Pattern: "/plain", Handle: func(rq *Request) Response { return NotFoundResponse(html.None()) }},
	}, testOpts(nil))
	for path, want := range map[string]int{"/n": 404, "/e": 500} {
		w := serve(h, httptest.NewRequest("POST", path, nil))
		if w.Code != want || !strings.HasPrefix(w.Header().Get("Set-Cookie"), CookieName+"=") {
			t.Errorf("%s: %d, Set-Cookie %q", path, w.Code, w.Header().Get("Set-Cookie"))
		}
	}
	if w := serve(h, httptest.NewRequest("POST", "/plain", nil)); w.Code != 404 || w.Header().Get("Set-Cookie") != "" {
		t.Errorf("/plain: %d, Set-Cookie %q", w.Code, w.Header().Get("Set-Cookie"))
	}
}

// A redirect to a URL rt.SafeURL refuses is a 500, with and without Datastar (defense in depth:
// Url.external already makes such a URL "#").
func TestResponse_RedirectRefusesScriptURL(t *testing.T) {
	var logs []string
	h := NewRouter(one("POST", "/f/go", func(rq *Request) Response {
		return RedirectResponse("javascript:alert(document.cookie)", nil)
	}), testOpts(func(o *Options) { o.Log = func(level, msg string) { logs = append(logs, level+": "+msg) } }))
	for _, ds := range []bool{false, true} {
		req := httptest.NewRequest("POST", "/f/go", nil)
		if ds {
			req.Header.Set("Datastar-Request", "true")
		}
		w := serve(h, req)
		body := w.Body.String()
		if strings.Contains(body, "javascript:") || w.Header().Get("Location") != "" {
			t.Fatalf("datastar=%v: %d %q", ds, w.Code, body)
		}
		if ds && (w.Code != 200 || !strings.Contains(body, "Something went wrong")) || !ds && w.Code != 500 {
			t.Fatalf("datastar=%v: %d %q", ds, w.Code, body)
		}
	}
	if len(logs) != 2 || !strings.Contains(logs[0], "internal error: refusing to navigate to javascript:") {
		t.Fatalf("logs %q", logs)
	}
}

// A request on a silo that is not served: 503 with Alder-Status: 503, App.maintenance (or the
// default page), a Datastar request patched as for a 404/500.
func TestResponse_Maintenance(t *testing.T) {
	route := one("GET", "/", func(rq *Request) Response { return MaintenanceResponse(errors.New("silo org/x: ahead: …")) })
	w := serve(NewRouter(route, testOpts(nil)), httptest.NewRequest("GET", "/", nil))
	if w.Code != 503 || w.Header().Get(StatusHeader) != "503" || !strings.Contains(w.Body.String(), "Down for maintenance") {
		t.Fatalf("default: %d %q %s", w.Code, w.Header().Get(StatusHeader), w.Body.String())
	}
	doc := html.El("html", nil, html.El("body", nil, html.Text("back soon")))
	h := NewRouter(route, testOpts(func(o *Options) { o.MaintenanceDoc = func() html.Node { return doc } }))
	w = serve(h, httptest.NewRequest("GET", "/", nil))
	if w.Code != 503 || !strings.Contains(w.Body.String(), "back soon") {
		t.Fatalf("App.maintenance: %d %s", w.Code, w.Body.String())
	}
	r := httptest.NewRequest("GET", "/", nil)
	r.Header.Set("Datastar-Request", "true")
	w = serve(h, r)
	if w.Code != 200 || w.Header().Get(StatusHeader) != "503" || !strings.Contains(w.Body.String(), "back soon") {
		t.Fatalf("Datastar: %d %q %s", w.Code, w.Header().Get(StatusHeader), w.Body.String())
	}
}
