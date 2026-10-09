package web

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"alder/rt/datastar"
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
	if w.Code != 200 || !strings.HasPrefix(w.Header().Get("Content-Type"), "application/javascript") || w.Body.Len() == 0 {
		t.Fatalf("script: %d %q", w.Code, w.Header().Get("Content-Type"))
	}
	w = serve(h, httptest.NewRequest("GET", "/static/app.css", nil))
	if w.Code != 200 || w.Body.String() != "body{color:red}" || !strings.HasPrefix(w.Header().Get("Content-Type"), "text/css") {
		t.Fatalf("static: %d %q %q", w.Code, w.Header().Get("Content-Type"), w.Body.String())
	}
	if w := serve(h, httptest.NewRequest("GET", "/static/img/a.txt", nil)); w.Code != 200 || w.Body.String() != "a" {
		t.Fatalf("nested static: %d", w.Code)
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
