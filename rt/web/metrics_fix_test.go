package web

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"alder/rt/metrics"
)

// A refused scrape's access line is Info for the first in a minute, Debug (Quiet) for the rest
// of that minute: a scanner cannot flood the log.
func TestMetrics_ProbeLinesRateLimited(t *testing.T) {
	var s sink
	now := time.Date(2026, 10, 30, 12, 0, 0, 0, time.UTC)
	h := NewRouter(nil, s.opts(func(o *Options) {
		o.Metrics, o.Now = metrics.New(metricsToken), func() time.Time { return now }
	}))
	quiet := func() bool {
		s.reset()
		scrape(t, h, "Bearer nope-nope-nope-nope")
		return s.one(t).Quiet
	}
	if quiet() {
		t.Error("first probe quiet")
	}
	if !quiet() || !quiet() {
		t.Error("probes in the same minute at Info")
	}
	now = now.Add(59 * time.Second)
	if !quiet() {
		t.Error("59 s later at Info")
	}
	now = now.Add(time.Second)
	if quiet() {
		t.Error("a minute later still quiet")
	}
	if !quiet() {
		t.Error("second of the new minute at Info")
	}
}

// A request aborted (http.ErrAbortHandler) before writing anything is counted as 5xx, not as
// the 2xx an unwritten status otherwise is.
func TestMetrics_AbortCounted5xx(t *testing.T) {
	h := NewRouter(one("GET", "/abort", func(rq *Request) Response { panic(http.ErrAbortHandler) }),
		testOpts(func(o *Options) { o.Metrics = metrics.New("") }))
	func() {
		defer func() {
			if v := recover(); v != http.ErrAbortHandler {
				t.Errorf("recovered %v", v)
			}
		}()
		h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/abort", nil))
	}()
	b := scrape(t, h, "").Body.String()
	if v := value(t, b, `alder_http_requests_total{route="/abort",method="GET",status="5xx"}`); v != 1 {
		t.Errorf("5xx %v\n%s", v, b)
	}
	if v := value(t, b, `alder_http_requests_total{route="/abort",method="GET",status="2xx"}`); v != -1 {
		t.Errorf("2xx %v", v)
	}
	if v := value(t, b, "alder_http_requests_in_flight"); v != 0 {
		t.Errorf("in flight %v", v)
	}
}
