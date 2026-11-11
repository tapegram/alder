package web

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"alder/rt/html"
	"alder/rt/metrics"
)

// methodRoutes declares one handler per method; each records its name in *hit.
func methodRoutes(hit *string, decl map[string][]string) []Route {
	var routes []Route
	for path, methods := range decl {
		for _, m := range methods {
			m, path := m, path
			routes = append(routes, Route{Method: m, Pattern: path, Handle: func(rq *Request) Response {
				*hit = m + " " + path
				if m == http.MethodGet {
					return PageResponse(doc(m), nil, nil)
				}
				return RedirectResponse("/", nil)
			}})
		}
	}
	return routes
}

func formReq(method, target, body string) *http.Request {
	req := httptest.NewRequest(method, target, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	return req
}

func TestRouter_SamePathTwoMethods(t *testing.T) {
	var hit string
	h := NewRouter(methodRoutes(&hit, map[string][]string{"/todos/{id}": {"GET", "DELETE"}}), testOpts(nil))
	for _, m := range []string{"GET", "DELETE"} {
		hit = ""
		w := serve(h, httptest.NewRequest(m, "/todos/7", nil))
		if hit != m+" /todos/{id}" || w.Code >= 400 {
			t.Fatalf("%s: hit %q, status %d", m, hit, w.Code)
		}
	}
	// PUT, PATCH and DELETE routes are accepted alongside GET and POST
	h = NewRouter(methodRoutes(&hit, map[string][]string{"/x": {"GET", "POST", "PUT", "PATCH", "DELETE"}}), testOpts(nil))
	for _, m := range []string{"GET", "POST", "PUT", "PATCH", "DELETE"} {
		hit = ""
		serve(h, formReq(m, "/x", "a=1"))
		if hit != m+" /x" {
			t.Fatalf("%s: hit %q", m, hit)
		}
	}
	defer func() {
		if recover() == nil {
			t.Fatal("an OPTIONS route did not panic")
		}
	}()
	NewRouter([]Route{{Method: "OPTIONS", Pattern: "/x"}}, testOpts(nil))
}

func TestRouter_405Allow(t *testing.T) {
	var hit string
	h := NewRouter(methodRoutes(&hit, map[string][]string{
		"/page":        {"GET"},
		"/todos/{id}":  {"DELETE", "GET", "PATCH"},
		"/todos/new":   {"POST"},
		"/only-delete": {"DELETE"},
	}), testOpts(nil))
	cases := []struct {
		method, path, allow string
	}{
		{"PUT", "/page", "GET, HEAD"},
		{"OPTIONS", "/page", "GET, HEAD"},
		{"POST", "/page", "GET, HEAD"},
		{"DELETE", "/page/", "GET, HEAD"}, // a trailing slash tolerated, as for a match
		{"PUT", "/todos/3", "GET, HEAD, PATCH, DELETE"},
		// /todos/new matches both patterns: Allow is every method declared for the path
		{"PUT", "/todos/new", "GET, HEAD, POST, PATCH, DELETE"},
		{"GET", "/only-delete", "DELETE"},
		{"HEAD", "/only-delete", "DELETE"},
		{"BREW", "/page", "GET, HEAD"},
	}
	for _, c := range cases {
		hit = ""
		w := serve(h, httptest.NewRequest(c.method, c.path, nil))
		if w.Code != http.StatusMethodNotAllowed || w.Header().Get("Allow") != c.allow || hit != "" {
			t.Fatalf("%s %s: %d Allow %q hit %q, want 405 Allow %q", c.method, c.path, w.Code, w.Header().Get("Allow"), hit, c.allow)
		}
	}
	// an unknown path stays a 404, whatever the method
	for _, m := range []string{"GET", "PUT", "OPTIONS"} {
		w := serve(h, httptest.NewRequest(m, "/nope", nil))
		if w.Code != 404 || w.Header().Get("Allow") != "" {
			t.Fatalf("%s /nope: %d Allow %q", m, w.Code, w.Header().Get("Allow"))
		}
	}
	// HEAD on a GET path keeps working
	if w := serve(h, httptest.NewRequest("HEAD", "/page", nil)); w.Code != 200 || hit != "GET /page" {
		t.Fatalf("HEAD /page: %d hit %q", w.Code, hit)
	}
	// a Datastar request gets the 405 too, as a body patch naming it
	req := httptest.NewRequest("PUT", "/page", nil)
	req.Header.Set("Datastar-Request", "true")
	w := serve(h, req)
	if w.Header().Get(StatusHeader) != "405" || w.Header().Get("Allow") != "GET, HEAD" {
		t.Fatalf("datastar 405: %d %q Allow %q", w.Code, w.Header().Get(StatusHeader), w.Header().Get("Allow"))
	}
	// an element fetch of a path declared only for other methods patches nothing, as an
	// unmatched one does (#177): one "Not found" toast, never the 405 body patch; a boosted GET
	// of it still gets the 405
	hit = ""
	w = serve(h, elementGet("/only-delete?datastar=%7B%7D"))
	if ev := events(w.Body.String()); w.Code != http.StatusOK || w.Header().Get(StatusHeader) == "405" || len(ev) != 1 ||
		ev[0] != toastEvent("destructive", "10000", "Not found") || hit != "" {
		t.Fatalf("element fetch: %d %q hit %q", w.Code, w.Body.String(), hit)
	}
	if w := serve(h, dsGet("/only-delete")); w.Header().Get(StatusHeader) != "405" || w.Header().Get("Allow") != "DELETE" {
		t.Fatalf("boosted 405: %d %q Allow %q", w.Code, w.Header().Get(StatusHeader), w.Header().Get("Allow"))
	}
}

func TestRouter_MethodOverride(t *testing.T) {
	var hit string
	var got *Request
	routes := methodRoutes(&hit, map[string][]string{"/todos/{id}": {"POST", "PUT", "PATCH", "DELETE"}})
	for i := range routes {
		next := routes[i].Handle
		routes[i].Handle = func(rq *Request) Response { got = rq; return next(rq) }
	}
	h := NewRouter(routes, testOpts(nil))
	for _, m := range []string{"PUT", "PATCH", "DELETE"} {
		hit, got = "", nil
		w := serve(h, formReq("POST", "/todos/7", "_method="+m+"&title=Milk"))
		if hit != m+" /todos/{id}" || w.Code != 303 {
			t.Fatalf("_method=%s: hit %q, status %d", m, hit, w.Code)
		}
		if got.Params["id"] != "7" || got.Form.Get("title") != "Milk" {
			t.Fatalf("_method=%s: params %v form %v", m, got.Params, got.Form)
		}
	}
	// the override to a method the path does not declare is a 405
	h = NewRouter(methodRoutes(&hit, map[string][]string{"/x": {"POST", "GET"}}), testOpts(nil))
	hit = ""
	w := serve(h, formReq("POST", "/x", "_method=DELETE"))
	if w.Code != 405 || w.Header().Get("Allow") != "GET, HEAD, POST" || hit != "" {
		t.Fatalf("override to undeclared: %d Allow %q hit %q", w.Code, w.Header().Get("Allow"), hit)
	}
	// a PUT/PATCH/DELETE body is read like a POST's
	h = NewRouter(routes, testOpts(nil))
	for _, m := range []string{"PUT", "PATCH", "DELETE"} {
		got = nil
		serve(h, formReq(m, "/todos/1", "title=Eggs"))
		if got == nil || got.Form.Get("title") != "Eggs" {
			t.Fatalf("%s form: %+v", m, got)
		}
		got = nil
		big := formReq(m, "/todos/1", "title="+strings.Repeat("x", 1<<20))
		if w := serve(h, big); w.Code != 413 || got != nil {
			t.Fatalf("%s big body: %d", m, w.Code)
		}
	}
}

func TestRouter_MethodOverrideAbuse(t *testing.T) {
	var hit string
	h := NewRouter(methodRoutes(&hit, map[string][]string{"/r": {"GET", "POST", "PUT", "PATCH", "DELETE"}}), testOpts(nil))
	bad := []string{
		"_method=GET", "_method=HEAD", "_method=POST", "_method=post", "_method=put", "_method=Delete",
		"_method=OPTIONS", "_method=", "_method=PUT&_method=PUT", "_method=PUT&_method=DELETE",
		"_method=PUT+", "_method=%20PUT",
	}
	for _, body := range bad {
		hit = ""
		w := serve(h, formReq("POST", "/r", body))
		if w.Code != 400 || hit != "" {
			t.Fatalf("POST %q: %d hit %q, want 400", body, w.Code, hit)
		}
	}
	// a 400 for a Datastar POST too, as a body patch naming it
	req := formReq("POST", "/r", "_method=GET")
	req.Header.Set("Datastar-Request", "true")
	if w := serve(h, req); w.Header().Get(StatusHeader) != "400" || hit != "" {
		t.Fatalf("datastar bad _method: %d %q", w.Code, w.Header().Get(StatusHeader))
	}
	// _method is ignored on any method but POST
	for _, c := range []struct{ method, body string }{
		{"PUT", "_method=PATCH"}, {"PATCH", "_method=DELETE"}, {"PATCH", "_method=GET"},
		{"DELETE", "_method=PUT&_method=PATCH"}, {"PUT", "_method=bogus"},
	} {
		hit = ""
		serve(h, formReq(c.method, "/r", c.body))
		if hit != c.method+" /r" {
			t.Fatalf("%s %q: hit %q", c.method, c.body, hit)
		}
	}
	// and on a GET, in the query
	hit = ""
	serve(h, httptest.NewRequest("GET", "/r?_method=DELETE", nil))
	if hit != "GET /r" {
		t.Fatalf("GET ?_method: hit %q", hit)
	}
	// on a POST only the url-encoded body counts: a query _method is not honoured (nor refused)
	for _, target := range []string{"/r?_method=DELETE", "/r?_method=GET", "/r?" + url.Values{"_method": {"PUT", "PATCH"}}.Encode()} {
		hit = ""
		serve(h, formReq("POST", target, "a=1"))
		if hit != "POST /r" {
			t.Fatalf("POST %s: hit %q", target, hit)
		}
	}
	// a body that is not url-encoded (a Datastar JSON post) carries no _method
	hit = ""
	req = httptest.NewRequest("POST", "/r", strings.NewReader(`{"_method":"DELETE"}`))
	req.Header.Set("Content-Type", "application/json")
	serve(h, req)
	if hit != "POST /r" {
		t.Fatalf("POST json _method: hit %q", hit)
	}
}

// The router's cross-site defence is the session cookie's SameSite=Lax (a browser does not send
// it on a cross-site POST, nor on a PUT/PATCH/DELETE, which a cross-site page can only send by a
// CORS-preflighted fetch); the router itself has no Origin refusal. Every write method is
// treated exactly as a POST is: a foreign Origin gets the same answer, and a session set by any
// of them is the same Lax cookie.
func TestRouter_OriginChecksAllWrites(t *testing.T) {
	routes := []Route{}
	for _, m := range []string{"POST", "PUT", "PATCH", "DELETE"} {
		routes = append(routes, Route{Method: m, Pattern: "/w", Handle: func(rq *Request) Response {
			return RedirectResponse("/", testSession{User: "ada"})
		}})
	}
	h := NewRouter(routes, testOpts(nil))
	answer := func(m string) (int, string, *http.Cookie) {
		req := formReq(m, "/w", "a=1")
		req.Header.Set("Origin", "https://evil.example")
		req.Header.Set("Sec-Fetch-Site", "cross-site")
		w := serve(h, req)
		var c *http.Cookie
		for _, k := range w.Result().Cookies() {
			if k.Name == CookieName {
				c = k
			}
		}
		return w.Code, w.Header().Get("Location"), c
	}
	code, loc, cookie := answer("POST")
	if cookie == nil || cookie.SameSite != http.SameSiteLaxMode || !cookie.HttpOnly {
		t.Fatalf("POST session cookie %+v", cookie)
	}
	for _, m := range []string{"PUT", "PATCH", "DELETE"} {
		c, l, k := answer(m)
		if c != code || l != loc || k == nil || k.SameSite != cookie.SameSite || k.HttpOnly != cookie.HttpOnly || k.Path != cookie.Path {
			t.Fatalf("%s from a foreign origin: %d %q %+v, POST got %d %q %+v", m, c, l, k, code, loc, cookie)
		}
	}
}

// A Sensitive path parameter of the route a POST's _method reaches is masked in the access
// line, though no POST route declares it; so is one of a route the request's method misses (405).
func TestAccessLog_MasksOverriddenRoute(t *testing.T) {
	const tok = "Ab3_xY9-Qr7Lm2Np8Kt4"
	var s sink
	routes := []Route{
		{Method: "DELETE", Pattern: "/reset/{token}", Handle: func(rq *Request) Response { return RedirectResponse("/", nil) }},
		{Method: "POST", Pattern: "/s/{id}", Handle: func(rq *Request) Response { return RedirectResponse("/", nil) }},
		{Method: "DELETE", Pattern: "/s/{code}", Handle: func(rq *Request) Response { return RedirectResponse("/", nil) }},
	}
	h := NewRouter(routes, s.opts(nil))
	for _, c := range []struct{ method, path, body, want string }{
		{"POST", "/reset/" + tok, "_method=DELETE", "/reset/••••"},
		{"POST", "/reset/" + tok, "", "/reset/••••"}, // 405
		{"PUT", "/reset/" + tok, "", "/reset/••••"},  // 405
		{"POST", "/s/" + tok, "_method=DELETE", "/s/••••"},
	} {
		s.reset()
		serve(h, formReq(c.method, c.path, c.body))
		if a := s.one(t); a.Path != c.want {
			t.Errorf("%s %s %q: path %q, want %q", c.method, c.path, c.body, a.Path, c.want)
		}
	}
}

// Datastar adds its signals as a `datastar` query parameter to a GET and, alike, to a DELETE (a
// click on a DELETE route: `?datastar=%7B%7D`); the router drops it from every Datastar request,
// whatever its method, so the handler's query and the log line are the page's own. A
// `_method=DELETE` that Datastar puts in a DELETE's query is never honoured (R7).
func TestRouter_DatastarDropsSignalsEveryMethod(t *testing.T) {
	for _, m := range []string{"GET", "POST", "PUT", "PATCH", "DELETE"} {
		var query, raw string
		h := NewRouter([]Route{{Method: m, Pattern: "/x", Handle: func(rq *Request) Response {
			query, raw = rq.Query.Encode(), rq.HTTP.URL.RawQuery
			if m == http.MethodGet {
				return PageResponse(doc(m), nil, nil)
			}
			return RedirectResponse("/", nil)
		}}}, testOpts(nil))
		req := formReq(m, "/x?a=1&datastar=%7B%7D", "")
		req.Header.Set("Datastar-Request", "true")
		serve(h, req)
		if query != "a=1" || raw != "a=1" {
			t.Errorf("%s: query %q (raw %q), want a=1", m, query, raw)
		}
	}
	// a DELETE's query _method changes nothing; a POST's query _method neither
	var hit string
	h := NewRouter(methodRoutes(&hit, map[string][]string{"/r": {"POST", "PUT", "DELETE"}}), testOpts(nil))
	for _, c := range []struct{ method, target, want string }{
		{"DELETE", "/r?_method=PUT", "DELETE /r"}, {"POST", "/r?_method=DELETE", "POST /r"},
	} {
		hit = ""
		req := formReq(c.method, c.target, "")
		req.Header.Set("Datastar-Request", "true")
		serve(h, req)
		if hit != c.want {
			t.Errorf("%s %s: hit %q, want %q", c.method, c.target, hit, c.want)
		}
	}
}

// A POST carrying _method is logged by the method it is routed by, in the access line, the dev
// panel's request record, an error line and the metrics label, so a reader matching a logged
// request to a route (the TUI) finds the PUT/PATCH/DELETE route and not a POST one. A plain POST
// stays POST; a refused _method (400) never reached a route and is logged as the POST it was. A
// 405 after the override names the overriding method; a Datastar override whose redirect is
// followed is logged once, by the first request's method, while a line about the followed page
// names that page's GET. Sensitive path parameters stay masked throughout.
func TestRequestLog_EffectiveMethod(t *testing.T) {
	var s sink
	redirect := func(rq *Request) Response { return RedirectResponse("/", nil) }
	routes := []Route{
		{Method: "GET", Pattern: "/t/{id}", Handle: func(rq *Request) Response { return PageResponse(doc("t"), nil, nil) }},
		{Method: "DELETE", Pattern: "/t/{id}", Handle: redirect},
		{Method: "PATCH", Pattern: "/t/{id}", Handle: redirect},
		{Method: "POST", Pattern: "/t", Handle: redirect},
		{Method: "GET", Pattern: "/g/{token}", Handle: func(rq *Request) Response { return PageResponse(doc("g"), nil, nil) }},
		{Method: "DELETE", Pattern: "/g/{token}", Handle: redirect},
		{Method: "DELETE", Pattern: "/x/{token}", Handle: func(rq *Request) Response { panic("kaboom") }},
		{Method: "DELETE", Pattern: "/e/{token}", Handle: func(rq *Request) Response { return ErrorResponse(html.None(), errors.New("db is down")) }},
		{Method: "DELETE", Pattern: "/f/{token}", Handle: func(rq *Request) Response { return RedirectResponse("/t/7", nil) }},
		{Method: "DELETE", Pattern: "/fe/{token}", Handle: func(rq *Request) Response { return RedirectResponse("/bad/s3cret", nil) }},
		{Method: "GET", Pattern: "/bad/{token}", Handle: func(rq *Request) Response { return ErrorResponse(html.None(), errors.New("gone")) }},
	}
	t.Setenv("ALDER_DEV", "1")
	dev := NewRouter(routes, s.opts(func(o *Options) { o.Dev, o.Metrics = true, metrics.New("") })).(*router)
	for _, c := range []struct {
		path, body string
		datastar   bool
		want       string // the access line's and the dev record's method
		norec      bool   // no dev record: the request never reached a handler
		path2      string // the access line's path, when not path
		line       string // the error line's first line without " rid=…", when one is logged
	}{
		{path: "/t/7", body: "_method=DELETE", want: "DELETE"},
		{path: "/t/7", body: "_method=PATCH", want: "PATCH"},
		{path: "/t", want: "POST"},
		{path: "/t/7", body: "_method=GET", want: "POST", norec: true},                       // 400: refused, never routed
		{path: "/g/s3cret", body: "_method=PUT", want: "PUT", path2: "/g/••••", norec: true}, // 405
		{path: "/x/s3cret", body: "_method=DELETE", want: "DELETE", path2: "/x/••••", line: "error: DELETE /x/••••: panic: kaboom"},
		{path: "/e/s3cret", body: "_method=DELETE", want: "DELETE", path2: "/e/••••", line: "error: DELETE /e/••••: db is down"},
		{path: "/f/s3cret", body: "_method=DELETE", datastar: true, want: "DELETE", path2: "/f/••••"},
		{path: "/fe/s3cret", body: "_method=DELETE", datastar: true, want: "DELETE", path2: "/fe/••••", line: "error: GET /bad/••••: gone"},
	} {
		s.reset()
		req := formReq("POST", c.path, c.body)
		if c.datastar {
			req.Header.Set("Datastar-Request", "true")
		}
		w := serve(dev, req)
		want := c.path
		if c.path2 != "" {
			want = c.path2
		}
		if a := s.one(t); a.Method != c.want || a.Path != want {
			t.Errorf("access line of POST %s %q: %s %s, want %s %s", c.path, c.body, a.Method, a.Path, c.want, want)
		}
		rid := w.Header().Get("X-Request-Id")
		var rec *ReqSummary
		for _, g := range summaries(t, dev) {
			if g.RID == rid {
				rec = &g
			}
		}
		if c.norec && rec != nil || !c.norec && (rec == nil || rec.Method != c.want) {
			t.Errorf("dev record of POST %s %q: %+v, want method %q (none: %v)", c.path, c.body, rec, c.want, c.norec)
		}
		var errs []string
		for _, l := range s.lines {
			if strings.HasPrefix(l, "error: ") {
				first, _, _ := strings.Cut(l, "\n")
				errs = append(errs, first)
			}
		}
		if c.line == "" && len(errs) != 0 || c.line != "" && (len(errs) != 1 || errs[0] != c.line+" rid="+rid) {
			t.Errorf("error lines of POST %s %q: %q, want %q", c.path, c.body, errs, c.line)
		}
	}
	b := scrape(t, dev, "").Body.String()
	for _, series := range []string{
		`alder_http_requests_total{route="/t/{id}",method="DELETE",status="3xx"}`,
		`alder_http_requests_total{route="/x/{token}",method="DELETE",status="5xx"}`,
		`alder_http_requests_total{route="/e/{token}",method="DELETE",status="5xx"}`,
		`alder_http_requests_total{route="/f/{token}",method="DELETE",status="2xx"}`,
		`alder_http_requests_total{route="(none)",method="PUT",status="4xx"}`,
	} {
		if v := value(t, b, series); v != 1 {
			t.Errorf("%s: %v\n%s", series, v, b)
		}
	}
}
