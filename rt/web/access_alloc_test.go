package web

import (
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
