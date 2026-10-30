package web

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	rtpkg "alder/rt"
	"alder/rt/applog"
	"alder/rt/eff"
	"alder/rt/html"
)

// sink records what a router logs: its access records and its lines ("<level>: <msg>").
type sink struct {
	mu     sync.Mutex
	access []applog.Access
	lines  []string
}

func (s *sink) opts(edit func(*Options)) Options {
	return testOpts(func(o *Options) {
		o.Access = func(a *applog.Access) {
			s.mu.Lock()
			defer s.mu.Unlock()
			c := *a
			c.Path, c.URL = a.MaskedPath(), nil
			s.access = append(s.access, c)
		}
		o.Log = func(level, msg string) {
			s.mu.Lock()
			defer s.mu.Unlock()
			s.lines = append(s.lines, level+": "+msg)
		}
		if edit != nil {
			edit(o)
		}
	})
}

func (s *sink) reset() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.access, s.lines = nil, nil
}

// one is the only access record, failing the test otherwise.
func (s *sink) one(t *testing.T) applog.Access {
	t.Helper()
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.access) != 1 {
		t.Fatalf("%d access records, want 1: %+v", len(s.access), s.access)
	}
	return s.access[0]
}

var ridSuffix = regexp.MustCompile(` rid=[0-9a-f]{8}$`)

func TestAccessLog_PageGET(t *testing.T) {
	var s sink
	h := NewRouter(one("GET", "/start", func(rq *Request) Response {
		return PageResponse(doc("start"), nil, nil, false)
	}), s.opts(nil))
	w := serveRaw(h, httptest.NewRequest("GET", "/start?key=s3cret&seed=7", nil))
	a := s.one(t)
	id := w.Header().Get("X-Request-Id")
	if a.Method != "GET" || a.Path != "/start?key=••••&seed=7" || a.Status != 200 || a.Dur <= 0 || a.Bytes != int64(w.Body.Len()) || a.ID != id || a.Quiet {
		t.Errorf("record %+v (header id %q, body %d bytes)", a, id, w.Body.Len())
	}
	if !regexp.MustCompile(`^[0-9a-f]{8}$`).MatchString(id) {
		t.Errorf("X-Request-Id %q", id)
	}
	if len(s.lines) != 0 {
		t.Errorf("a healthy page wrote lines: %q", s.lines)
	}
	// a second request has another id
	s.reset()
	w2 := serve(h, httptest.NewRequest("GET", "/start", nil))
	if id2 := w2.Header().Get("X-Request-Id"); id2 == id || s.one(t).ID != id2 {
		t.Errorf("ids %q, %q", id, id2)
	}
}

func TestAccessLog_POSTRedirect(t *testing.T) {
	var s sink
	h := NewRouter(one("POST", "/_m/A/Save", func(rq *Request) Response { return RedirectResponse("/list", nil) }), s.opts(nil))
	req := httptest.NewRequest("POST", "/_m/A/Save", strings.NewReader("password=hunter2"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := serve(h, req)
	a := s.one(t)
	if w.Code != 303 || a.Method != "POST" || a.Path != "/_m/A/Save" || a.Status != 303 || a.Bytes != 0 || a.Quiet {
		t.Errorf("%d: %+v", w.Code, a)
	}
}

func TestAccessLog_NotFound(t *testing.T) {
	var s sink
	h := NewRouter(nil, s.opts(nil))
	serve(h, httptest.NewRequest("GET", "/nowhere?token=abc", nil))
	if a := s.one(t); a.Status != 404 || a.Quiet || a.Path != "/nowhere?token=••••" {
		t.Errorf("%+v", a)
	}
}

func TestAccessLog_HealthSilent(t *testing.T) {
	var s sink
	h := NewRouter(nil, s.opts(nil))
	for _, m := range []string{"GET", "HEAD"} {
		w := serve(h, httptest.NewRequest(m, HealthPath, nil))
		if w.Code != 200 {
			t.Fatalf("%s: %d", m, w.Code)
		}
	}
	if len(s.access) != 0 || len(s.lines) != 0 {
		t.Errorf("health logged: %+v %q", s.access, s.lines)
	}
}

func TestAccessLog_StaticQuiet(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "app.css"), []byte("body{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	var s sink
	t.Setenv("ALDER_DEV", "1")
	h := NewRouter(one("GET", "/", func(rq *Request) Response { return PageResponse(doc("x"), nil, nil, false) }),
		s.opts(func(o *Options) { o.StaticDir = dir; o.Dev = true }))
	for _, c := range []struct {
		path   string
		status int
		quiet  bool
	}{
		{"/static/app.css", 200, true},
		{"/static/missing.css", 404, false},
		{"/static/al/al.css", 200, true},
		{"/_alder/datastar-nope.js", 404, false},
		{"/_dev", 200, true},
		{"/", 200, false},
	} {
		s.reset()
		w := serve(h, httptest.NewRequest("GET", c.path, nil))
		if a := s.one(t); w.Code != c.status || a.Status != c.status || a.Quiet != c.quiet {
			t.Errorf("%s: %d %+v, want %d quiet=%v", c.path, w.Code, a, c.status, c.quiet)
		}
	}
	// a revalidation (304) is quiet too
	s.reset()
	w := serve(h, httptest.NewRequest("GET", "/static/al/al.css", nil))
	req := httptest.NewRequest("GET", "/static/al/al.css", nil)
	req.Header.Set("If-None-Match", w.Header().Get("ETag"))
	s.reset()
	w = serve(h, req)
	if a := s.one(t); w.Code != 304 || a.Status != 304 || !a.Quiet {
		t.Errorf("304: %d %+v", w.Code, a)
	}
}

func TestAccessLog_Panic(t *testing.T) {
	var s sink
	h := NewRouter(one("GET", "/boom", func(rq *Request) Response { panic("kaboom") }), s.opts(nil))
	w := serve(h, httptest.NewRequest("GET", "/boom?key=s3cret", nil))
	a := s.one(t)
	if w.Code != 500 || a.Status != 500 || a.ID != w.Header().Get("X-Request-Id") {
		t.Fatalf("%d %+v", w.Code, a)
	}
	if len(s.lines) != 1 {
		t.Fatalf("lines %q", s.lines)
	}
	first, stack, _ := strings.Cut(s.lines[0], "\n")
	if first != "error: GET /boom?key=••••: panic: kaboom rid="+a.ID || !strings.Contains(stack, "goroutine") {
		t.Errorf("error line %q", s.lines[0])
	}

	// after the response started: the status as written, and the error line, share the id
	s.reset()
	h = NewRouter(one("POST", "/_m/A/B", func(rq *Request) Response {
		return PageResponse(doc("patched"), nil, nil, true)
	}), s.opts(nil))
	req := httptest.NewRequest("POST", "/_m/A/B", nil)
	req.Header.Set("Datastar-Request", "true")
	rec := httptest.NewRecorder()
	h.ServeHTTP(flushPanics{rec}, req)
	a = s.one(t)
	if a.Status != 200 || a.Bytes == 0 {
		t.Errorf("after start: %+v", a)
	}
	if len(s.lines) != 1 || !strings.Contains(strings.SplitN(s.lines[0], "\n", 2)[0], "(after the response started) rid="+a.ID) {
		t.Errorf("lines %q (id %s)", s.lines, a.ID)
	}
}

func TestAccessLog_IncomingID(t *testing.T) {
	var s sink
	h := NewRouter(one("GET", "/", func(rq *Request) Response { return PageResponse(doc("x"), nil, nil, false) }), s.opts(nil))
	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("X-Request-Id", "abc-1")
	if w := serve(h, req); w.Header().Get("X-Request-Id") != "abc-1" || s.one(t).ID != "abc-1" {
		t.Errorf("abc-1: header %q record %+v", w.Header().Get("X-Request-Id"), s.access)
	}
	s.reset()
	long := strings.Repeat("a", 65)
	req = httptest.NewRequest("GET", "/", nil)
	req.Header.Set("X-Request-Id", long)
	w := serve(h, req)
	if got := w.Header().Get("X-Request-Id"); got == long || len(got) != 8 || s.one(t).ID != got {
		t.Errorf("65 chars: header %q record %+v", got, s.access)
	}
}

func TestAccessLog_NilHook(t *testing.T) {
	var logs []string
	h := NewRouter(one("GET", "/", func(rq *Request) Response { return PageResponse(doc("x"), nil, nil, false) }), testOpts(func(o *Options) {
		o.Log = func(level, msg string) { logs = append(logs, level+": "+msg) }
	}))
	w := serve(h, httptest.NewRequest("GET", "/", nil))
	if w.Code != 200 || len(w.Header().Get("X-Request-Id")) != 8 || len(logs) != 0 {
		t.Errorf("%d %v %q", w.Code, w.Header(), logs)
	}
}

func TestAccessLog_Steps(t *testing.T) {
	var s sink
	h := NewRouter(one("GET", "/", func(rq *Request) Response {
		rq.Env.Trace = append(rq.Env.Trace, "Db.all Todo", "Time.now")
		return PageResponse(doc("x"), nil, nil, false)
	}), s.opts(nil))
	serve(h, httptest.NewRequest("GET", "/", nil))
	if a := s.one(t); a.Steps != 2 {
		t.Errorf("steps %d", a.Steps)
	}
}

// fail on GET /start?key=s3cret: the line names the URL masked and ends its first line with the
// request's id.
func TestErrorLine_Masked(t *testing.T) {
	var s sink
	h := NewRouter(one("GET", "/start", func(rq *Request) Response {
		return ErrorResponse(html.None(), errors.New("db is down"))
	}), s.opts(nil))
	w := serveRaw(h, httptest.NewRequest("GET", "/start?key=s3cret&seed=7", nil))
	want := "error: GET /start?key=••••&seed=7: db is down rid=" + w.Header().Get("X-Request-Id")
	if len(s.lines) != 1 || s.lines[0] != want {
		t.Errorf("lines %q, want %q", s.lines, want)
	}
}

func TestBadRequest_Masked(t *testing.T) {
	var s sink
	h := NewRouter(one("GET", "/start", func(rq *Request) Response { return PageResponse(doc("x"), nil, nil, false) }), s.opts(nil))
	hex40 := "0123456789abcdef0123456789abcdef01234567"
	w := serve(h, httptest.NewRequest("GET", "/start?key=s3cret&faults=a;b&"+hex40+"=%zz", nil))
	if w.Code != 400 || len(s.lines) != 1 {
		t.Fatalf("%d %q", w.Code, s.lines)
	}
	l := s.lines[0]
	if !strings.HasPrefix(l, "warn: GET /start?key=••••&faults=••••&••••=••••: ") || !strings.Contains(l, `"faults"`) || !ridSuffix.MatchString(l) {
		t.Errorf("line %q", l)
	}
	if strings.Contains(l, "s3cret") || strings.Contains(l, hex40) {
		t.Errorf("a secret in the line: %q", l)
	}
	if a := s.one(t); a.Status != 400 {
		t.Errorf("%+v", a)
	}
}

// 413 and 415 are warn lines, each with the request's id.
func TestBodyRefusals_Warn(t *testing.T) {
	var s sink
	h := NewRouter(one("POST", "/p", func(rq *Request) Response { return PageResponse(doc("x"), nil, nil, false) }), s.opts(nil))
	big := httptest.NewRequest("POST", "/p", strings.NewReader("a="+strings.Repeat("x", maxBody+10)))
	big.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	multi := httptest.NewRequest("POST", "/p", strings.NewReader("--x--"))
	multi.Header.Set("Content-Type", "multipart/form-data; boundary=x")
	for _, c := range []struct {
		req    *http.Request
		status int
	}{{big, 413}, {multi, 415}} {
		s.reset()
		w := serve(h, c.req)
		if w.Code != c.status || len(s.lines) != 1 || !strings.HasPrefix(s.lines[0], "warn: POST /p: ") || !strings.HasSuffix(s.lines[0], " rid="+w.Header().Get("X-Request-Id")) || s.one(t).Status != c.status {
			t.Errorf("%d: %d %q %+v", c.status, w.Code, s.lines, s.access)
		}
	}
}

// The runtime never hands a header, a cookie, a body, a form value or a session to the log:
// secrets planted in each, on requests answered every way, are in no line and no access record.
func TestLogLines_NoHeadersCookiesBodies(t *testing.T) {
	const secret = "PLANTEDsecretVALUE"
	var s sink
	routes := []Route{
		{Method: "GET", Pattern: "/page", Handle: func(rq *Request) Response {
			return PageResponse(doc("x"), nil, testSession{User: secret}, false)
		}},
		{Method: "GET", Pattern: "/boom", Handle: func(rq *Request) Response { panic("kaboom") }},
		{Method: "GET", Pattern: "/err", Handle: func(rq *Request) Response {
			return ErrorResponse(html.None(), errors.New("failed"))
		}},
		{Method: "POST", Pattern: "/_m/A/Save", Handle: func(rq *Request) Response {
			return RedirectResponse("/page", testSession{User: secret})
		}},
		{Method: "POST", Pattern: "/_m/A/Boom", Handle: func(rq *Request) Response { panic("kaboom") }},
		{Method: "GET", Pattern: "/file", File: true, Handle: func(rq *Request) Response {
			return FileResponse(rtpkg.File{Status: 200, ContentType: "text/plain", Body: []byte(secret), Headers: [][2]string{{"Set-Cookie", secret}}}, nil)
		}},
		{Method: "GET", Pattern: "/maint", Handle: func(rq *Request) Response { return MaintenanceResponse(errors.New("silo x")) }},
	}
	t.Setenv("ALDER_DEV", "1")
	for _, dev := range []bool{false, true} {
		h := NewRouter(routes, s.opts(func(o *Options) { o.Dev = dev }))
		cookie := SignedCookie(t, testSession{User: secret})
		for _, path := range []string{"/page", "/boom", "/err", "/_m/A/Save", "/_m/A/Boom", "/file", "/maint", "/nowhere", "/page?q=a;b", HealthPath} {
			for _, ds := range []bool{false, true} {
				method := "GET"
				if strings.HasPrefix(path, "/_m/") {
					method = "POST"
				}
				req := httptest.NewRequest(method, path, strings.NewReader("password="+secret+"&note="+secret))
				req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
				req.Header.Set("Authorization", "Bearer "+secret)
				req.Header.Set("X-Api-Key", secret)
				req.Header.Set("User-Agent", secret)
				req.Header.Set("Referer", "https://x.example/?"+secret)
				req.AddCookie(&http.Cookie{Name: CookieName, Value: cookie})
				req.AddCookie(&http.Cookie{Name: "other", Value: secret})
				if ds {
					req.Header.Set("Datastar-Request", "true")
				}
				serve(h, req)
			}
		}
	}
	if len(s.access) == 0 || len(s.lines) == 0 {
		t.Fatalf("nothing logged: %d records, %d lines", len(s.access), len(s.lines))
	}
	for _, l := range s.lines {
		if strings.Contains(l, secret) {
			t.Errorf("a line carries a planted secret: %q", l)
		}
	}
	for _, a := range s.access {
		if strings.Contains(a.Path, secret) || strings.Contains(a.Method, secret) || strings.Contains(a.ID, secret) {
			t.Errorf("a record carries a planted secret: %+v", a)
		}
	}
}

// SignedCookie is the session cookie value carrying s, as the test router's key signs it.
func SignedCookie(t *testing.T, s testSession) string {
	t.Helper()
	v, err := SessionCookie(testOpts(nil).Secret, jsonCodec{}, s, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	return v
}

func TestDevPanel_ShowsID(t *testing.T) {
	t.Setenv("ALDER_DEV", "1")
	h := NewRouter(one("GET", "/x", func(rq *Request) Response { return PageResponse(doc("hi"), nil, nil, false) }), testOpts(func(o *Options) { o.Dev = true }))
	w := serve(h, httptest.NewRequest("GET", "/x", nil))
	if body := w.Body.String(); !strings.Contains(body, `<section id="_alder_tab_request"><h3>Request</h3><pre>id `+w.Header().Get("X-Request-Id")+"\nGET /x\n") {
		t.Errorf("no id line first in Request: %s", body)
	}
}

func TestDevPanel_MasksQuery(t *testing.T) {
	t.Setenv("ALDER_DEV", "1")
	h := NewRouter(one("GET", "/x", func(rq *Request) Response {
		rq.Env.Rec.AddStep(eff.StepRec{Name: "Db.all"})
		return PageResponse(doc("hi"), nil, nil, false)
	}), testOpts(func(o *Options) { o.Dev = true }))
	body := serve(h, httptest.NewRequest("GET", "/x?key=s3cret&seed=7", nil)).Body.String()
	panel := body[strings.Index(body, "<al-devpanel"):]
	if strings.Contains(panel, "s3cret") || !strings.Contains(panel, "GET /x?key=••••&amp;seed=7\n") {
		t.Errorf("panel %s", panel)
	}
}
