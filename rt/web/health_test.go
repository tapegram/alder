package web

import (
	"context"
	"encoding/json"
	"errors"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"alder/rt/db"
	"alder/rt/eff"
	"alder/rt/jobs"
)

// healthBody decodes /_alder/health's answer.
func healthBody(t *testing.T, w *httptest.ResponseRecorder) map[string]string {
	t.Helper()
	var m map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &m); err != nil {
		t.Fatalf("body %q: %v", w.Body.String(), err)
	}
	return m
}

func healthStore(t *testing.T) *db.SQLite {
	t.Helper()
	st, err := db.OpenSQLite(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	return st
}

// /_alder/health answers 200 with the build and "ok" after SELECT 1 on the store.
func TestHealth_OK(t *testing.T) {
	st := healthStore(t)
	h := NewRouter(nil, testOpts(func(o *Options) { o.Store, o.Build = st, "b-123" }))
	w := serve(h, httptest.NewRequest("GET", "/_alder/health", nil))
	if w.Code != 200 || w.Header().Get("Content-Type") != "application/json" {
		t.Fatalf("%d %q %s", w.Code, w.Header().Get("Content-Type"), w.Body.String())
	}
	if m := healthBody(t, w); m["build"] != "b-123" || m["db"] != "ok" || len(m) != 2 {
		t.Fatalf("body %v", m)
	}
	// HEAD too (a load balancer's probe)
	if w := serve(h, httptest.NewRequest("HEAD", "/_alder/health", nil)); w.Code != 200 {
		t.Fatalf("HEAD: %d", w.Code)
	}
}

// flakyStore is a store whose SELECT 1 fails with err while err is set: a driver error naming
// the database's host, as Turso's does.
type flakyStore struct {
	*db.SQLite
	err error
}

func (f *flakyStore) Ping(ctx context.Context) error {
	if f.err != nil {
		return f.err
	}
	return f.SQLite.Ping(ctx)
}

// A store that cannot answer SELECT 1: 503 with the fixed category "unreachable", never the
// driver's error (the route is public; the error names the host). The error is logged at warn
// once when the checks start failing, and once more when they pass again — not on every check.
func TestHealth_DBDown(t *testing.T) {
	const host = "app-acme.turso.io"
	st := &flakyStore{SQLite: healthStore(t), err: errors.New(`Post "https://` + host + `/v2/pipeline": dial tcp: connection refused`)}
	var logs []string
	h := NewRouter(nil, testOpts(func(o *Options) {
		o.Store, o.Build = st, "b-1"
		o.Log = func(level, msg string) { logs = append(logs, level+": "+msg) }
	}))
	for range 3 {
		w := serve(h, httptest.NewRequest("GET", "/_alder/health", nil))
		if m := healthBody(t, w); w.Code != 503 || m["build"] != "b-1" || m["db"] != "unreachable" {
			t.Fatalf("%d %v", w.Code, m)
		}
		if b := w.Body.String(); strings.Contains(b, host) || strings.Contains(b, "https://") || strings.Contains(b, "refused") {
			t.Fatalf("the body carries the driver's error: %s", b)
		}
	}
	if len(logs) != 1 || !strings.HasPrefix(logs[0], "warn: ") || !strings.Contains(logs[0], host) {
		t.Fatalf("logs while failing %q", logs)
	}
	st.err = nil
	for range 2 {
		if w := serve(h, httptest.NewRequest("GET", "/_alder/health", nil)); w.Code != 200 {
			t.Fatalf("recovered: %d %s", w.Code, w.Body.String())
		}
	}
	if len(logs) != 2 || logs[1] != "warn: health: the database answers again" {
		t.Fatalf("logs after the recovery %q", logs)
	}
	// a closed connection is unreachable too
	closed := healthStore(t)
	closed.Close()
	h = NewRouter(nil, testOpts(func(o *Options) { o.Store = closed }))
	if m := healthBody(t, serve(h, httptest.NewRequest("GET", "/_alder/health", nil))); m["db"] != "unreachable" {
		t.Fatalf("closed: %v", m)
	}
}

// SharedMaintenance (the shared database is ahead of this build): health is 503 with its text,
// and every other route answers the maintenance page with 503.
func TestHealth_Ahead(t *testing.T) {
	st := healthStore(t)
	called := false
	route := one("GET", "/", func(rq *Request) Response { called = true; return PageResponse(doc("home"), nil, nil) })
	h := NewRouter(route, testOpts(func(o *Options) {
		o.Store, o.Build = st, "b-2"
		o.SharedMaintenance = errors.New("ahead: the database has 0002_x, which this build does not have")
	}))
	w := serve(h, httptest.NewRequest("GET", "/_alder/health", nil))
	if m := healthBody(t, w); w.Code != 503 || m["build"] != "b-2" || m["db"] != "ahead: the database has 0002_x, which this build does not have" {
		t.Fatalf("health: %d %v", w.Code, m)
	}
	for _, path := range []string{"/", "/nowhere"} {
		w = serve(h, httptest.NewRequest("GET", path, nil))
		if w.Code != 503 || w.Header().Get(StatusHeader) != "503" || !strings.Contains(w.Body.String(), "Down for maintenance") {
			t.Fatalf("%s: %d %s", path, w.Code, w.Body.String())
		}
	}
	w = serve(h, httptest.NewRequest("POST", "/", strings.NewReader("")))
	if w.Code != 503 {
		t.Fatalf("POST: %d", w.Code)
	}
	if called {
		t.Fatal("the page's handler ran in maintenance")
	}
}

// Health is outside the session: no cookie is read or set, it is never cached, and it is not
// logged.
func TestHealth_NoSession(t *testing.T) {
	st := healthStore(t)
	var logs []string
	h := NewRouter(nil, testOpts(func(o *Options) {
		o.Store = st
		o.Log = func(level, msg string) { logs = append(logs, level+": "+msg) }
	}))
	r := httptest.NewRequest("GET", "/_alder/health", nil)
	r.Header.Set("Cookie", CookieName+"=garbage")
	w := serve(h, r)
	if w.Code != 200 || w.Header().Get("Set-Cookie") != "" || w.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("%d set-cookie %q cache %q", w.Code, w.Header().Get("Set-Cookie"), w.Header().Get("Cache-Control"))
	}
	if len(logs) != 0 {
		t.Fatalf("logged %q", logs)
	}
	// a POST is not a health check: the router's usual 404
	if w := serve(h, httptest.NewRequest("POST", "/_alder/health", strings.NewReader(""))); w.Code != 404 {
		t.Fatalf("POST: %d", w.Code)
	}
}

// stepClock is a clock a test sets.
type stepClock struct {
	mu sync.Mutex
	t  time.Time
}

func (c *stepClock) Now() time.Time  { c.mu.Lock(); defer c.mu.Unlock(); return c.t }
func (c *stepClock) Set(t time.Time) { c.mu.Lock(); c.t = t; c.mu.Unlock() }

// Spec §9: /_alder/health carries "jobs": "off" when this instance runs no workers
// (ALDER_JOBS=0), "stalled" when a row has been due more than five minutes and no worker claimed
// in that time, else "ok" — and the status code does not change.
func TestHealth_Jobs(t *testing.T) {
	st, err := db.OpenSQLite(filepath.Join(t.TempDir(), "app.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	if err := st.Ensure(context.Background(), nil, true); err != nil { // the jobs table
		t.Fatal(err)
	}
	t0 := time.Date(2026, 11, 16, 12, 0, 0, 0, time.UTC)
	c := &stepClock{t: t0}
	k := jobs.Kind{Name: "Signup.SendWelcome", Run: func(*eff.Env, string) (eff.Task[struct{}], error) { return eff.Pure(struct{}{}), nil }}
	start := func(workers int) *jobs.Health {
		h := &jobs.Health{}
		stop := jobs.Start(context.Background(), jobs.Options{
			Store: st, Kinds: []jobs.Kind{k}, Workers: workers, Now: c.Now, Health: h,
			Env:  func(ctx context.Context, k jobs.Kind) *eff.Env { return &eff.Env{Ctx: ctx, Store: st, Now: c.Now} },
			Poll: time.Hour, Housekeep: time.Hour,
		})
		t.Cleanup(func() { stop(0) })
		return h
	}
	check := func(h *jobs.Health, want string) {
		t.Helper()
		r := NewRouter(nil, testOpts(func(o *Options) { o.Store, o.Build, o.Jobs = st, "b-1", h.State }))
		w := serve(r, httptest.NewRequest("GET", "/_alder/health", nil))
		if m := healthBody(t, w); w.Code != 200 || m["db"] != "ok" || m["jobs"] != want {
			t.Fatalf("%d %v, want jobs %q", w.Code, m, want)
		}
	}
	check(start(0), "off") // ALDER_JOBS=0
	check(&jobs.Health{}, "off")
	// a row due at t0 + 1m, which the worker, idle until its hour's poll, never claims
	q, err := db.On(st, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := q.ExecContext(context.Background(), `INSERT INTO "_alder_jobs" ("id", "kind", "params", "state", "run_at", "created", "attempts") VALUES ('aaaa0001', ?, '{}', 'queued', ?, ?, 0)`,
		k.Name, t0.Add(time.Minute).UnixMilli(), t0.UnixMilli()); err != nil {
		t.Fatal(err)
	}
	h := start(1)
	check(h, "ok")
	c.Set(t0.Add(6 * time.Minute)) // due 5 min: not more than five
	check(h, "ok")
	c.Set(t0.Add(7 * time.Minute)) // due 6 min, the workers' start (no claim since) 7 min ago
	check(h, "stalled")
	// HEAD answers the same status, without a body
	r := NewRouter(nil, testOpts(func(o *Options) { o.Store, o.Jobs = st, h.State }))
	if w := serve(r, httptest.NewRequest("HEAD", "/_alder/health", nil)); w.Code != 200 {
		t.Fatalf("HEAD: %d", w.Code)
	}
}
