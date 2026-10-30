package web

import (
	"bytes"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"alder/rt/applog"
	"alder/rt/html"
)

// An error line names its URL cut at applog.MaxPath, as an access line does.
func TestErrorLine_PathCapped(t *testing.T) {
	var s sink
	long := strings.Repeat("/abc", 1500)
	h := NewRouter(nil, s.opts(func(o *Options) { o.SharedMaintenance = errors.New("ahead") }))
	serve(h, httptest.NewRequest("GET", long, nil))
	if len(s.lines) != 1 || !strings.HasPrefix(s.lines[0], "warn: GET "+long[:applog.MaxPath]+"…: maintenance: ahead rid=") {
		t.Errorf("lines (%d): %.120q", len(s.lines), s.lines)
	}
}

// An adapter's error (or a panic's value) wrapping user text with a line break and a forged
// access line: the app's logger writes the error line as one entry, every continuation line
// indented, and the error text cut at 8 KB.
func TestErrorLine_NoForgedLines(t *testing.T) {
	forged := "smtp: bad recipient bob\n2026/10/30 10:00:00 info: GET /admin/export 200 1.0ms 0B rid=abc"
	var b bytes.Buffer
	lg := applog.New(&b, applog.Info, applog.Text)
	routes := []Route{
		{Method: "GET", Pattern: "/send", Handle: func(rq *Request) Response { return ErrorResponse(html.None(), errors.New(forged)) }},
		{Method: "GET", Pattern: "/boom", Handle: func(rq *Request) Response { panic(forged) }},
		{Method: "GET", Pattern: "/big", Handle: func(rq *Request) Response {
			return ErrorResponse(html.None(), errors.New(strings.Repeat("x", 20000)))
		}},
	}
	h := NewRouter(routes, testOpts(func(o *Options) { o.Log, o.Access = lg.Log, lg.Access }))
	for _, p := range []string{"/send", "/boom"} {
		b.Reset()
		serve(h, httptest.NewRequest("GET", p, nil))
		var entries int
		for _, l := range strings.Split(strings.TrimSuffix(b.String(), "\n"), "\n") {
			switch {
			case strings.HasPrefix(l, "\t"):
			case l != "" && l[0] >= '0' && l[0] <= '9' && !strings.Contains(l, "/admin/export"):
				entries++
			default:
				t.Errorf("%s: a forged line at column 0: %q", p, l)
			}
		}
		if entries != 2 { // the error line and the access line
			t.Errorf("%s: %d entries:\n%s", p, entries, b.String())
		}
	}
	b.Reset()
	serve(h, httptest.NewRequest("GET", "/big", nil))
	first, _, _ := strings.Cut(b.String(), "\n")
	if n := strings.Count(first, "x"); n != applog.MaxText || !strings.Contains(first, "x… rid=") {
		t.Errorf("%d x kept: %.60q…", n, first)
	}
}

// A Random.id token (20 base64url characters: too short to look like a secret) in a path
// parameter named `token` or `code` is masked in the access line and in error lines; in a query
// it is masked by its key's name. Other parameters keep their values.
func TestAccessLog_SensitivePathParams(t *testing.T) {
	const tok = "Ab3_xY9-Qr7Lm2Np8Kt4" // Random.id's shape
	if applog.LooksSecret(tok) {
		t.Fatal("the token looks secret on its own: the test proves nothing")
	}
	var s sink
	routes := []Route{
		{Method: "GET", Pattern: "/reset/{token}", Handle: func(rq *Request) Response { return PageResponse(doc("x"), nil, nil, false) }},
		{Method: "GET", Pattern: "/magic/{resetCode}/go", Handle: func(rq *Request) Response {
			return ErrorResponse(html.None(), errors.New("expired"))
		}},
		{Method: "GET", Pattern: "/u/{id}", Handle: func(rq *Request) Response { return PageResponse(doc("x"), nil, nil, false) }},
		{Method: "GET", Pattern: "/u/new", Handle: func(rq *Request) Response { return PageResponse(doc("x"), nil, nil, false) }},
	}
	h := NewRouter(routes, s.opts(nil))
	for _, c := range []struct{ path, want string }{
		{"/reset/" + tok, "/reset/••••"},
		{"/reset/" + tok + "/", "/reset/••••/"},
		{"/reset/" + tok + "?token=" + tok + "&seed=7", "/reset/••••?token=••••&seed=7"},
		{"/u/" + tok, "/u/" + tok},
		{"/u/new?token=" + tok, "/u/new?token=••••"},
		{"/nowhere/" + tok, "/nowhere/" + tok},
	} {
		s.reset()
		serve(h, httptest.NewRequest("GET", c.path, nil))
		if a := s.one(t); a.Path != c.want {
			t.Errorf("%s: path %q, want %q", c.path, a.Path, c.want)
		}
	}
	for _, m := range []string{"GET", "HEAD"} {
		s.reset()
		serve(h, httptest.NewRequest(m, "/magic/"+tok+"/go", nil))
		if len(s.lines) != 1 || !strings.HasPrefix(s.lines[0], "error: "+m+" /magic/••••/go: expired rid=") || strings.Contains(s.lines[0], tok) || s.one(t).Path != "/magic/••••/go" {
			t.Errorf("%s: lines %q, records %+v", m, s.lines, s.access)
		}
	}
}

// The dev panel shows a Sensitive path parameter masked, in the URL and in its params.
func TestDevPanel_MasksSensitiveParam(t *testing.T) {
	t.Setenv("ALDER_DEV", "1")
	const tok = "Ab3_xY9-Qr7Lm2Np8Kt4"
	h := NewRouter(one("GET", "/reset/{token}", func(rq *Request) Response { return PageResponse(doc("x"), nil, nil, false) }), testOpts(func(o *Options) { o.Dev = true }))
	body := serve(h, httptest.NewRequest("GET", "/reset/"+tok, nil)).Body.String()
	panel := body[strings.Index(body, "<al-devpanel"):]
	if strings.Contains(panel, tok) || !strings.Contains(panel, "GET /reset/••••\n") || !strings.Contains(panel, "param token = ••••") {
		t.Errorf("panel %s", panel)
	}
}

// fixedWriter is a ResponseWriter that allocates nothing per request (httptest's recorder does).
type fixedWriter struct{ h http.Header }

func (f *fixedWriter) Header() http.Header         { return f.h }
func (f *fixedWriter) Write(b []byte) (int, error) { return len(b), nil }
func (f *fixedWriter) WriteHeader(int)             {}

// Under ALDER_LOG=warn the access hook costs a request nothing: the record lives in the router's
// tracker and its URL is masked only for a line that is written. A request served with the
// logger's hook allocates exactly what one served without a hook does.
func TestAccessLog_FilteredNoCost(t *testing.T) {
	route := one("GET", "/p", func(rq *Request) Response { return PageResponse(html.None(), nil, nil, false) })
	lg := applog.New(io.Discard, applog.Warn, applog.Text)
	with := NewRouter(route, testOpts(func(o *Options) { o.Access = lg.Access }))
	without := NewRouter(route, testOpts(nil))
	req := httptest.NewRequest("GET", "/p?key=s3cret&seed=7&next=https%3A%2F%2Fh%2F%3Ftoken%3Dx", nil)
	w := &fixedWriter{h: http.Header{}}
	run := func(h http.Handler) float64 {
		return testing.AllocsPerRun(200, func() {
			clear(w.h)
			h.ServeHTTP(w, req)
		})
	}
	run(with) // warm the pools
	a, b := run(with), run(without)
	if a != b {
		t.Errorf("a request with the access hook at warn: %v allocations, without a hook %v: the hook costs %v", a, b, a-b)
	}
	// the measure sees the masking: at info, where the line is written, it costs
	info := NewRouter(route, testOpts(func(o *Options) { o.Access = applog.New(io.Discard, applog.Info, applog.Text).Access }))
	if c := run(info); c <= b {
		t.Errorf("a written access line: %v allocations, without a hook %v", c, b)
	}
}
