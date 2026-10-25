package web

import (
	"context"
	"encoding/json"
	"io"
	"mime"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"alder/rt/al"
	"alder/rt/datastar"
	"alder/rt/eff"
	"alder/rt/html"
)

// testSession is the App session type of the tests; jsonCodec is what the generated
// SessionCodec does for it.
type testSession struct{ User string }

type jsonCodec struct{}

func (jsonCodec) Default() any { return testSession{} }
func (jsonCodec) Encode(v any) ([]byte, error) {
	return json.Marshal(v.(testSession))
}
func (jsonCodec) Decode(b []byte) (any, error) {
	var s testSession
	err := json.Unmarshal(b, &s)
	return s, err
}

// doc is a small document: <html><head><title>t</title></head><body><p>t</p></body></html>.
func doc(t string) html.Node {
	return html.El("html", nil,
		html.El("head", nil, html.El("title", nil, html.Text(t))),
		html.El("body", nil, html.El("p", nil, html.Text(t))))
}

func testOpts(edit func(*Options)) Options {
	o := Options{
		Secret:      []byte("test-secret-test-secret"),
		Session:     jsonCodec{},
		NotFoundDoc: func() html.Node { return doc("Not found") },
		ErrorDoc:    func(error) html.Node { return doc("Something went wrong") },
		Login:       "/login",
	}
	if edit != nil {
		edit(&o)
	}
	return o
}

func serve(h http.Handler, req *http.Request) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	return w
}

func TestRouter_PathParams(t *testing.T) {
	var got *Request
	routes := []Route{
		{Method: "GET", Pattern: "/", Handle: func(rq *Request) Response {
			got = rq
			return PageResponse(doc("home"), nil, nil, false)
		}},
		{Method: "GET", Pattern: "/todos/{id}", Handle: func(rq *Request) Response {
			got = rq
			return PageResponse(doc("todo "+rq.Params["id"]), nil, nil, false)
		}},
		{Method: "GET", Pattern: "/todos/{id}/edit/{field}", Handle: func(rq *Request) Response {
			got = rq
			return PageResponse(doc("edit"), nil, nil, false)
		}},
	}
	h := NewRouter(routes, testOpts(nil))

	for _, path := range []string{"/todos/abc", "/todos/abc/"} {
		got = nil
		w := serve(h, httptest.NewRequest("GET", path+"?q=1", nil))
		if w.Code != 200 || got == nil || got.Params["id"] != "abc" || got.Query.Get("q") != "1" {
			t.Fatalf("%s: status %d, rq %+v", path, w.Code, got)
		}
		if got.Form != nil || got.Login != "/login" || got.Dev || got.Datastar || got.Env == nil || got.Ctx == nil || got.HTTP == nil {
			t.Fatalf("%s: request fields %+v", path, got)
		}
	}
	got = nil
	serve(h, httptest.NewRequest("GET", "/todos/a%2Fb%20c", nil))
	if got == nil || got.Params["id"] != "a/b c" {
		t.Fatalf("escaped param: %+v", got)
	}
	got = nil
	serve(h, httptest.NewRequest("GET", "/todos/1/edit/title", nil))
	if got == nil || got.Params["id"] != "1" || got.Params["field"] != "title" {
		t.Fatalf("two params: %+v", got)
	}
	got = nil
	if w := serve(h, httptest.NewRequest("GET", "/", nil)); w.Code != 200 || got == nil || len(got.Params) != 0 {
		t.Fatalf("root: %d %+v", w.Code, got)
	}
	for _, path := range []string{"/todos/", "/todos", "/todos/a/b", "/todos//", "/nope"} {
		got = nil
		w := serve(h, httptest.NewRequest("GET", path, nil))
		if w.Code != 404 || got != nil || !strings.Contains(w.Body.String(), "<p>Not found</p>") {
			t.Fatalf("%s: status %d, body %q", path, w.Code, w.Body.String())
		}
		if ct := w.Header().Get("Content-Type"); ct != "text/html; charset=utf-8" {
			t.Fatalf("%s: content type %q", path, ct)
		}
	}
}

func TestRouter_MethodDispatch(t *testing.T) {
	var hits []string
	routes := []Route{
		{Method: "GET", Pattern: "/todos", Handle: func(rq *Request) Response {
			hits = append(hits, "GET")
			return PageResponse(doc("list"), nil, nil, false)
		}},
		{Method: "POST", Pattern: "/_m/Todos/Add", Handle: func(rq *Request) Response {
			hits = append(hits, "POST")
			return RedirectResponse("/todos", nil)
		}},
	}
	h := NewRouter(routes, testOpts(nil))
	cases := []struct {
		method, path string
		code         int
	}{
		{"GET", "/_m/Todos/Add", 404},
		{"POST", "/todos", 404},
		{"POST", "/_m/Todos/Remove", 404},
		{"PUT", "/todos", 404},
		{"GET", "/todos", 200},
		{"POST", "/_m/Todos/Add", 303},
	}
	for _, c := range cases {
		w := serve(h, httptest.NewRequest(c.method, c.path, nil))
		if w.Code != c.code {
			t.Fatalf("%s %s: %d, want %d", c.method, c.path, w.Code, c.code)
		}
		if c.code == 404 && !strings.Contains(w.Body.String(), "<p>Not found</p>") {
			t.Fatalf("%s %s: body %q", c.method, c.path, w.Body.String())
		}
	}
	if strings.Join(hits, ",") != "GET,POST" {
		t.Fatalf("hits %v", hits)
	}
}

func TestRouter_StaticAndScript(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "app.css"), []byte("body{color:red}"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(dir, "img"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "img", "a.txt"), []byte("a"), 0o644); err != nil {
		t.Fatal(err)
	}
	// A secret next to the static dir must never be reachable.
	if err := os.WriteFile(filepath.Join(filepath.Dir(dir), "secret.txt"), []byte("s"), 0o644); err != nil {
		t.Fatal(err)
	}
	h := NewRouter(nil, testOpts(func(o *Options) { o.StaticDir = dir }))

	w := serve(h, httptest.NewRequest("GET", datastar.ScriptPath, nil))
	if w.Code != 200 || !strings.HasPrefix(w.Header().Get("Content-Type"), "application/javascript") || w.Body.Len() == 0 ||
		w.Header().Get("Cache-Control") != "public, max-age=31536000, immutable" || !strings.Contains(datastar.ScriptPath, datastar.Version) {
		t.Fatalf("script %s: %d %q %q", datastar.ScriptPath, w.Code, w.Header().Get("Content-Type"), w.Header().Get("Cache-Control"))
	}
	// The unversioned path of earlier releases is gone: an ordinary unknown path.
	if w := serve(h, httptest.NewRequest("GET", "/_alder/datastar.js", nil)); w.Code != 404 {
		t.Fatalf("old script path: %d", w.Code)
	}
	w = serve(h, httptest.NewRequest("GET", "/static/app.css", nil))
	if w.Code != 200 || w.Body.String() != "body{color:red}" || !strings.HasPrefix(w.Header().Get("Content-Type"), "text/css") {
		t.Fatalf("static: %d %q %q", w.Code, w.Header().Get("Content-Type"), w.Body.String())
	}
	if w.Header().Get("Cache-Control") != "" {
		t.Fatalf("the app's own static file is not immutable: %q", w.Header().Get("Cache-Control"))
	}
	if w := serve(h, httptest.NewRequest("GET", "/static/img/a.txt", nil)); w.Code != 200 || w.Body.String() != "a" {
		t.Fatalf("nested static: %d", w.Code)
	}
	// The design system's assets (static/al/, written by every build) keep their URL across
	// compiler upgrades: served from the runtime's embedded copy, fresh for five minutes and
	// then stale-while-revalidate (M13), with a strong ETag, so a revalidation gets a 304 while
	// nothing changed. An app's own file
	// under static/al/ is served like any static file (not immutable); a missing one is a 404.
	if err := os.MkdirAll(filepath.Join(dir, "al"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "al", "mine.css"), []byte(".mine{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	css, etag, _ := al.File("al.css")
	for _, name := range al.Names {
		w = serve(h, httptest.NewRequest("GET", "/static/al/"+name, nil))
		if w.Code != 200 || w.Header().Get("Cache-Control") != "public, max-age=300, stale-while-revalidate=86400" || !strings.HasPrefix(w.Header().Get("ETag"), `"`) {
			t.Fatalf("%s: %d %q %q", name, w.Code, w.Header().Get("Cache-Control"), w.Header().Get("ETag"))
		}
	}
	w = serve(h, httptest.NewRequest("GET", "/static/al/al.css", nil))
	if w.Header().Get("ETag") != etag || w.Body.String() != string(css) || !strings.HasPrefix(w.Header().Get("Content-Type"), "text/css") {
		t.Fatalf("al.css: %q %q (%d bytes)", w.Header().Get("ETag"), w.Header().Get("Content-Type"), w.Body.Len())
	}
	req := httptest.NewRequest("GET", "/static/al/al.css", nil)
	req.Header.Set("If-None-Match", etag)
	if w := serve(h, req); w.Code != http.StatusNotModified || w.Body.Len() != 0 {
		t.Fatalf("If-None-Match: %d (%d bytes)", w.Code, w.Body.Len())
	}
	req = httptest.NewRequest("GET", "/static/al/al.css", nil)
	req.Header.Set("If-None-Match", `"stale"`)
	if w := serve(h, req); w.Code != 200 || w.Body.Len() != len(css) {
		t.Fatalf("stale If-None-Match: %d", w.Code)
	}
	w = serve(h, httptest.NewRequest("GET", "/static/al/mine.css", nil))
	if w.Code != 200 || w.Body.String() != ".mine{}" || w.Header().Get("Cache-Control") != "" || w.Header().Get("ETag") != "" {
		t.Fatalf("app file under al/: %d %q %q", w.Code, w.Header().Get("Cache-Control"), w.Header().Get("ETag"))
	}
	if w := serve(h, httptest.NewRequest("GET", "/static/al/missing.css", nil)); w.Code != 404 || w.Header().Get("Cache-Control") != "" {
		t.Fatalf("missing al file: %d %q", w.Code, w.Header().Get("Cache-Control"))
	}
	for _, path := range []string{
		"/static/../go.mod", "/static/../secret.txt", "/static/%2e%2e/secret.txt",
		"/static/img/../../secret.txt", "/static/", "/static/img", "/static/img/", "/static/missing.css",
	} {
		req := httptest.NewRequest("GET", "/", nil)
		req.URL = &url.URL{Path: mustUnescape(t, path), RawPath: path}
		w := serve(h, req)
		if w.Code != 404 {
			t.Fatalf("%s: status %d body %q", path, w.Code, w.Body.String())
		}
	}
	// No StaticDir: /static/ is just an unknown path.
	h = NewRouter(nil, testOpts(nil))
	if w := serve(h, httptest.NewRequest("GET", "/static/app.css", nil)); w.Code != 404 {
		t.Fatalf("no static dir: %d", w.Code)
	}
}

func mustUnescape(t *testing.T, s string) string {
	u, err := url.PathUnescape(s)
	if err != nil {
		t.Fatal(err)
	}
	return u
}

func TestRouter_PanicRecovered(t *testing.T) {
	var logs []string
	var docErr error
	routes := []Route{{Method: "GET", Pattern: "/boom", Handle: func(rq *Request) Response {
		panic("kaboom")
	}}}
	h := NewRouter(routes, testOpts(func(o *Options) {
		o.Log = func(level, msg string) { logs = append(logs, level+": "+msg) }
		o.ErrorDoc = func(err error) html.Node { docErr = err; return doc("Something went wrong") }
	}))
	w := serve(h, httptest.NewRequest("GET", "/boom", nil))
	if w.Code != 500 || !strings.Contains(w.Body.String(), "<p>Something went wrong</p>") || strings.Contains(w.Body.String(), "kaboom") {
		t.Fatalf("status %d body %q", w.Code, w.Body.String())
	}
	if docErr == nil || !strings.Contains(docErr.Error(), "kaboom") {
		t.Fatalf("ErrorDoc got %v", docErr)
	}
	if len(logs) != 1 || !strings.HasPrefix(logs[0], "error: GET /boom: ") || !strings.Contains(logs[0], "kaboom") || !strings.Contains(logs[0], "goroutine") {
		t.Fatalf("logs %q", logs)
	}
	if strings.Contains(w.Body.String(), "goroutine") {
		t.Fatal("stack reached the page")
	}

	// Under Datastar: a 200 patch of the error document's body.
	req := httptest.NewRequest("GET", "/boom", nil)
	req.Header.Set("Datastar-Request", "true")
	w = serve(h, req)
	if w.Code != 200 || w.Header().Get("Content-Type") != "text/event-stream" || !strings.Contains(w.Body.String(), "data: elements <body><p>Something went wrong</p></body>") {
		t.Fatalf("datastar panic: %d %q", w.Code, w.Body.String())
	}
	// The router keeps serving.
	if w := serve(h, httptest.NewRequest("GET", "/boom", nil)); w.Code != 500 {
		t.Fatalf("second request %d", w.Code)
	}
}

func TestRouter_FormParsedForPost(t *testing.T) {
	var got *Request
	now := time.Date(2026, 10, 9, 0, 0, 0, 0, time.UTC)
	caps := map[string]any{"Cap": 1}
	routes := []Route{{Method: "POST", Pattern: "/_m/Todos/Add", Handle: func(rq *Request) Response {
		got = rq
		return RedirectResponse("/todos", nil)
	}}}
	h := NewRouter(routes, testOpts(func(o *Options) {
		o.Now = func() time.Time { return now }
		o.Caps = caps
		o.Dev = true
	}))
	post := func(body, ct string) *httptest.ResponseRecorder {
		req := httptest.NewRequest("POST", "/_m/Todos/Add?from=list", strings.NewReader(body))
		if ct != "" {
			req.Header.Set("Content-Type", ct)
		}
		return serve(h, req)
	}

	got = nil
	w := post("title=Buy+milk&tag=a&tag=b", "application/x-www-form-urlencoded")
	if w.Code != 303 || got == nil {
		t.Fatalf("status %d", w.Code)
	}
	if got.Form.Get("title") != "Buy milk" || len(got.Form["tag"]) != 2 || got.Form.Get("from") != "" || got.Query.Get("from") != "list" {
		t.Fatalf("form %v query %v", got.Form, got.Query)
	}
	env := got.Env
	if env.Ctx == nil || env.Now() != now || env.Caps["Cap"] != 1 || env.Session != (testSession{}) || env.Log == nil || env.Tx != nil || !got.Dev {
		t.Fatalf("env %+v", env)
	}

	// A body that is not a form (a Datastar JSON post of a button) gives an empty, non-nil Form.
	got = nil
	if w := post(`{"signal":1}`, "application/json"); w.Code != 303 || got == nil || got.Form == nil || len(got.Form) != 0 {
		t.Fatalf("json body: %d %+v", w.Code, got)
	}

	got = nil
	big := "title=" + strings.Repeat("x", 1<<20)
	w = post(big, "application/x-www-form-urlencoded")
	if w.Code != 413 || got != nil || !strings.HasPrefix(w.Header().Get("Content-Type"), "text/plain") {
		t.Fatalf("big body: %d %q", w.Code, w.Header().Get("Content-Type"))
	}

	got = nil
	w = post("--x\r\n", "multipart/form-data; boundary=x")
	if w.Code != 415 || got != nil {
		t.Fatalf("multipart: %d", w.Code)
	}

	got = nil
	w = post("%zz", "application/x-www-form-urlencoded")
	if w.Code != 400 || got != nil {
		t.Fatalf("malformed form: %d", w.Code)
	}

	// Defaults: nil Now is time.Now, nil Log is a no-op.
	h = NewRouter(routes, testOpts(nil))
	got = nil
	req := httptest.NewRequest("POST", "/_m/Todos/Add", strings.NewReader("a=1"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	serve(h, req)
	if got == nil || got.Env.Now == nil || got.Env.Log == nil || time.Since(got.Env.Now()) > time.Minute {
		t.Fatalf("defaults: %+v", got)
	}
	got.Env.Log("info", "no-op")
}

func TestRouter_ConcurrentRequests(t *testing.T) {
	routes := []Route{{Method: "GET", Pattern: "/n/{id}", Handle: func(rq *Request) Response {
		return PageResponse(doc(rq.Params["id"]), nil, testSession{User: rq.Params["id"]}, false)
	}}}
	h := NewRouter(routes, testOpts(nil))
	done := make(chan string)
	for i := 0; i < 20; i++ {
		id := strings.Repeat("x", i+1)
		go func() {
			w := serve(h, httptest.NewRequest("GET", "/n/"+id, nil))
			if !strings.Contains(w.Body.String(), "<p>"+id+"</p>") {
				done <- "wrong body for " + id
				return
			}
			done <- ""
		}()
	}
	for i := 0; i < 20; i++ {
		if msg := <-done; msg != "" {
			t.Error(msg)
		}
	}
}

// flushPanics is a ResponseWriter whose Flush panics, i.e. a panic after the response started.
type flushPanics struct{ *httptest.ResponseRecorder }

func (f flushPanics) Flush() { panic("flush failed") }

func TestRouter_PanicAfterWriteOnlyLogged(t *testing.T) {
	var logs []string
	h := NewRouter(one("POST", "/_m/A/B", func(rq *Request) Response {
		return PageResponse(doc("patched"), nil, nil, true)
	}), testOpts(func(o *Options) {
		o.Log = func(level, msg string) { logs = append(logs, level+": "+msg) }
	}))
	req := httptest.NewRequest("POST", "/_m/A/B", nil)
	req.Header.Set("Datastar-Request", "true")
	rec := httptest.NewRecorder()
	h.ServeHTTP(flushPanics{rec}, req)
	body := rec.Body.String()
	if strings.Count(body, "event: ") != 1 || strings.Contains(body, "went wrong") {
		t.Fatalf("written twice: %q", body)
	}
	if len(logs) != 1 || !strings.Contains(logs[0], "flush failed") || !strings.Contains(logs[0], "after the response started") {
		t.Fatalf("logs %q", logs)
	}
}

// Routes are matched most specific first (I4): a literal segment before a parameter, left to
// right, whatever the declaration order; equally specific patterns keep it.
func TestRouter_Specificity(t *testing.T) {
	var got string
	route := func(pattern, name string) Route {
		return Route{Method: "GET", Pattern: pattern, Handle: func(rq *Request) Response {
			got = name
			return PageResponse(doc(name), nil, nil, false)
		}}
	}
	h := NewRouter([]Route{
		route("/x/{id}", "show"),
		route("/x/{id}/edit", "edit"),
		route("/x/new", "new"),
		route("/{a}/{b}", "any2"),
		route("/x/{other}", "shadowed"),
		route("/{a}/new/{c}", "anynew"),
		route("/y/{b}/z", "yz"),
	}, testOpts(nil))
	for path, want := range map[string]string{
		"/x/new": "new", "/x/new/": "new", "/x/1": "show", "/x/1/edit": "edit", "/x/new/edit": "edit",
		"/q/r": "any2", "/y/new/z": "yz", "/q/new/z": "anynew",
	} {
		got = ""
		if w := serve(h, httptest.NewRequest("GET", path, nil)); w.Code != 200 || got != want {
			t.Errorf("GET %s → %d %q, want %q", path, w.Code, got, want)
		}
	}
}

// The color-scheme meta and the Datastar script go right after <head>, the boost just before
// </head>; without a <head>, all three after <html>, else before <body>, else first: where the
// parser puts them in the (implied) head, never in the body a patch replaces (M5, M13).
func TestWithScript(t *testing.T) {
	first, boost := HeadTags()
	tag := first + boost
	for page, want := range map[string]string{
		`<html><head><title>t</title></head><body></body></html>`: `<html><head>` + first + `<title>t</title>` + boost + `</head><body></body></html>`,
		`<html><head><title>t</title><body></body></html>`:        `<html><head>` + tag + `<title>t</title><body></body></html>`,
		`<html lang="en"><body><p>x</p></body></html>`:            `<html lang="en">` + tag + `<body><p>x</p></body></html>`,
		`<body class="x"><p>x</p></body>`:                         tag + `<body class="x"><p>x</p></body>`,
		`<p>bare</p>`:                                             tag + `<p>bare</p>`,
		`<header>h</header><p>x</p>`:                              tag + `<header>h</header><p>x</p>`,
	} {
		if got := withScript(page); got != want {
			t.Errorf("withScript(%s)\n got %s\nwant %s", page, got, want)
		}
	}
}

// HEAD is served like GET, without a body (I7): through a real server, which drops the body.
func TestRouter_Head(t *testing.T) {
	h := NewRouter([]Route{{Method: "GET", Pattern: "/", Handle: func(rq *Request) Response {
		return PageResponse(doc("home"), nil, testSession{User: "ada"}, false)
	}}}, testOpts(nil))
	srv := httptest.NewServer(h)
	defer srv.Close()
	for path, want := range map[string]int{"/": 200, "/nope": 404} {
		req, _ := http.NewRequest("HEAD", srv.URL+path, nil)
		res, err := srv.Client().Do(req)
		if err != nil {
			t.Fatal(err)
		}
		body, _ := io.ReadAll(res.Body)
		res.Body.Close()
		if res.StatusCode != want || len(body) != 0 || res.Header.Get("Content-Type") != "text/html; charset=utf-8" {
			t.Fatalf("HEAD %s: %d %q, %d body bytes", path, res.StatusCode, res.Header.Get("Content-Type"), len(body))
		}
	}
}

// With Options.Reload (alder run) every page, full or patched, carries the dev server's overlay
// target (kept by Datastar's morph: data-ignore-morph) and reload subscriber (naming the build
// generation, open while the tab is hidden) right after its <body> start tag; without it,
// neither.
func TestRouter_Reload(t *testing.T) {
	const dev = `<div id="_alder_errors" data-ignore-morph></div><div data-init="@get('/_dev/reload?b=a1b2-3', {openWhenHidden: true})"></div>`
	routes := []Route{{Method: "GET", Pattern: "/", Handle: func(rq *Request) Response {
		return PageResponse(doc("home"), nil, testSession{}, true)
	}}}
	for _, reload := range []bool{false, true} {
		h := NewRouter(routes, testOpts(func(o *Options) {
			if reload {
				o.Reload = "a1b2-3"
			}
		}))
		page := serve(h, httptest.NewRequest("GET", "/", nil)).Body.String()
		req := httptest.NewRequest("GET", "/", nil)
		req.Header.Set("Datastar-Request", "true")
		patch := serve(h, req).Body.String()
		missing := serve(h, httptest.NewRequest("GET", "/nope", nil)).Body.String()
		for name, got := range map[string]string{"page": page, "patch": patch, "404": missing} {
			if want := "<body>" + dev + "<p>"; strings.Contains(got, want) != reload || strings.Count(got, "_alder_errors") != map[bool]int{false: 0, true: 1}[reload] {
				t.Errorf("Reload=%v: %s:\n%s", reload, name, got)
			}
		}
	}
	if got := withReload(`<p>bare</p>`, "a1b2-3"); got != dev+`<p>bare</p>` {
		t.Errorf("withReload without <body>: %s", got)
	}
	// the generation is query-escaped: it cannot end the JavaScript string or the attribute
	if got := reloadTags(`x'" &y`); !strings.Contains(got, `?b=x%27%22+%26y'`) {
		t.Errorf("reloadTags with a quote: %s", got)
	}
}

// TestRouter_EnvFor: with Options.EnvFor the router uses the Env it makes (with the request's
// context and decoded session, then sets its Session); without, a fresh Env from the options.
func TestRouter_EnvFor(t *testing.T) {
	var got *Request
	routes := []Route{{Method: "GET", Pattern: "/", Handle: func(rq *Request) Response {
		got = rq
		return PageResponse(doc("home"), nil, nil, false)
	}}}
	made := &eff.Env{Caps: map[string]any{"X": 1}}
	var calledWith any
	var ctxOK bool
	h := NewRouter(routes, testOpts(func(o *Options) {
		o.Caps = map[string]any{"Y": 2}
		o.EnvFor = func(ctx context.Context, session any) *eff.Env {
			calledWith, ctxOK = session, ctx != nil
			return made
		}
	}))
	value, err := SessionCookie([]byte("test-secret-test-secret"), jsonCodec{}, testSession{User: "ann"}, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest("GET", "/", nil)
	req.AddCookie(&http.Cookie{Name: CookieName, Value: value})
	serve(h, req)
	if got.Env != made || !ctxOK || calledWith != (testSession{User: "ann"}) || made.Session != (testSession{User: "ann"}) {
		t.Fatalf("EnvFor not used: env %p (made %p), session %v / %v", got.Env, made, calledWith, made.Session)
	}
	// nil EnvFor: today's Env
	h = NewRouter(routes, testOpts(func(o *Options) { o.Caps = map[string]any{"Y": 2} }))
	serve(h, httptest.NewRequest("GET", "/", nil))
	if got.Env == made || got.Env.Caps["Y"] != 2 || got.Env.Session != (testSession{}) {
		t.Fatalf("default Env = %+v", got.Env)
	}
}

// TestRouter_Served: Options.Served sees each request after its handler answered, with the
// model a page route put in Request.Model; nil Served changes nothing.
func TestRouter_Served(t *testing.T) {
	routes := []Route{{Method: "GET", Pattern: "/", Handle: func(rq *Request) Response {
		rq.Model = 42
		return PageResponse(doc("home"), nil, nil, false)
	}}}
	var seen []any
	h := NewRouter(routes, testOpts(func(o *Options) { o.Served = func(rq *Request) { seen = append(seen, rq.Model) } }))
	serve(h, httptest.NewRequest("GET", "/", nil))
	serve(h, httptest.NewRequest("GET", "/nothing-here", nil))
	if len(seen) != 1 || seen[0] != 42 {
		t.Fatalf("Served saw %v", seen)
	}
	h = NewRouter(routes, testOpts(nil))
	if w := serve(h, httptest.NewRequest("GET", "/", nil)); w.Code != 200 {
		t.Fatalf("status %d", w.Code)
	}
}

// TestRouter_DatastarFailureStatusHeader: a Datastar answer to a 404 or 500 is a 200 patch
// that names the status in StatusHeader; a re-rendered page carries none.
func TestRouter_DatastarFailureStatusHeader(t *testing.T) {
	routes := []Route{
		{Method: "POST", Pattern: "/ok", Handle: func(rq *Request) Response { return PageResponse(doc("ok"), nil, nil, true) }},
		{Method: "POST", Pattern: "/missing", Handle: func(rq *Request) Response { return NotFoundResponse(html.None()) }},
		{Method: "POST", Pattern: "/fail", Handle: func(rq *Request) Response { return ErrorResponse(html.None(), io.EOF) }},
		{Method: "POST", Pattern: "/panic", Handle: func(rq *Request) Response { panic("boom") }},
	}
	h := NewRouter(routes, testOpts(func(o *Options) { o.Log = func(string, string) {} }))
	for path, want := range map[string]string{"/ok": "", "/missing": "404", "/nowhere": "404", "/fail": "500", "/panic": "500"} {
		req := httptest.NewRequest("POST", path, nil)
		req.Header.Set("Datastar-Request", "true")
		w := serve(h, req)
		if w.Code != 200 || w.Header().Get(StatusHeader) != want {
			t.Errorf("%s: %d, %s %q, want %q", path, w.Code, StatusHeader, w.Header().Get(StatusHeader), want)
		}
		// without Datastar: the status itself, no header
		w = serve(h, httptest.NewRequest("POST", path, nil))
		if w.Header().Get(StatusHeader) != "" {
			t.Errorf("%s without Datastar carries %s", path, StatusHeader)
		}
	}
}

func TestSessionCookie_RoundTrip(t *testing.T) {
	key := []byte("test-secret-test-secret")
	now := time.Unix(1_800_000_000, 0)
	v, err := SessionCookie(key, jsonCodec{}, testSession{User: "u"}, now)
	if err != nil {
		t.Fatal(err)
	}
	if s, ok := SessionFromCookie(key, jsonCodec{}, v, now); !ok || s != (testSession{User: "u"}) {
		t.Fatalf("= %v %v", s, ok)
	}
	if _, ok := SessionFromCookie(key, jsonCodec{}, v, now.Add(31*24*time.Hour)); ok {
		t.Fatal("expired cookie accepted")
	}
	if _, ok := SessionFromCookie([]byte("another-key-another-key"), jsonCodec{}, v, now); ok {
		t.Fatal("wrong key accepted")
	}
}

func TestDesignSystemFontServed(t *testing.T) {
	// Hosts' mime tables (macOS apache, Debian /etc/mime.types) already know .woff2, which
	// would hide a handler that stopped setting the type. Make the table wrong so only
	// serveStatic's own Content-Type can pass. Process-global: this test must not be parallel.
	if err := mime.AddExtensionType(".woff2", "application/octet-stream"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = mime.AddExtensionType(".woff2", "font/woff2") })
	h := NewRouter(nil, testOpts(func(o *Options) { o.StaticDir = t.TempDir() }))
	w := serve(h, httptest.NewRequest("GET", "/static/al/fonts/Geist-Variable.woff2", nil))
	etag := w.Header().Get("ETag")
	if w.Code != 200 || w.Header().Get("Content-Type") != "font/woff2" || w.Header().Get("Cache-Control") != DesignSystemCache || etag == "" {
		t.Fatalf("font: %d %q %q %q", w.Code, w.Header().Get("Content-Type"), w.Header().Get("Cache-Control"), etag)
	}
	req := httptest.NewRequest("GET", "/static/al/fonts/Geist-Variable.woff2", nil)
	req.Header.Set("If-None-Match", etag)
	if w := serve(h, req); w.Code != http.StatusNotModified {
		t.Fatalf("If-None-Match: %d", w.Code)
	}
	w = serve(h, httptest.NewRequest("GET", "/static/al/fonts/OFL.txt", nil))
	if w.Code != 200 || !strings.HasPrefix(w.Header().Get("Content-Type"), "text/plain") {
		t.Fatalf("OFL.txt: %d %q", w.Code, w.Header().Get("Content-Type"))
	}
}
