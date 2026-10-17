package web

import (
	"encoding/json"
	"errors"
	"net/http/httptest"
	"strings"
	"testing"

	"alder/rt/db"
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

// A store that cannot answer SELECT 1 (its connection closed): 503 with the error.
func TestHealth_DBDown(t *testing.T) {
	st := healthStore(t)
	st.Close()
	h := NewRouter(nil, testOpts(func(o *Options) { o.Store, o.Build = st, "b-1" }))
	w := serve(h, httptest.NewRequest("GET", "/_alder/health", nil))
	m := healthBody(t, w)
	if w.Code != 503 || m["build"] != "b-1" || !strings.Contains(m["db"], "closed") {
		t.Fatalf("%d %v", w.Code, m)
	}
}

// SharedMaintenance (the shared database is ahead of this build): health is 503 with its text,
// and every other route answers the maintenance page with 503.
func TestHealth_Ahead(t *testing.T) {
	st := healthStore(t)
	called := false
	route := one("GET", "/", func(rq *Request) Response { called = true; return PageResponse(doc("home"), nil, nil, false) })
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
