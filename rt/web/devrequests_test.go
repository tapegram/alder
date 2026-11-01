package web

import (
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"alder/rt/eff"
)

// devRouter is a dev router (ALDER_DEV=1, Options.Dev) with routes, and opts edited by edit.
func devRouter(t *testing.T, routes []Route, edit func(*Options)) *router {
	t.Helper()
	t.Setenv("ALDER_DEV", "1")
	return NewRouter(routes, testOpts(func(o *Options) {
		o.Dev = true
		if edit != nil {
			edit(o)
		}
	})).(*router)
}

// summaries is GET /_dev/requests on h, decoded.
func summaries(t *testing.T, h *router) []ReqSummary {
	t.Helper()
	w := serve(h, httptest.NewRequest("GET", "/_dev/requests", nil))
	if w.Code != 200 || !strings.HasPrefix(w.Header().Get("Content-Type"), "application/json") {
		t.Fatalf("/_dev/requests: %d %q %s", w.Code, w.Header().Get("Content-Type"), w.Body.String())
	}
	var out []ReqSummary
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
		t.Fatalf("%v: %s", err, w.Body.String())
	}
	return out
}

func pageRoute(pattern string) Route {
	return Route{Method: "GET", Pattern: pattern, Handle: func(rq *Request) Response { return PageResponse(doc("hi"), nil, nil, false) }}
}

// The ring holds the requests the dev panel recorded, newest first, each under the id its
// response carried in X-Request-Id.
func TestDevRequests_RingAndOrder(t *testing.T) {
	h := devRouter(t, []Route{pageRoute("/a"), pageRoute("/b"), pageRoute("/c")}, nil)
	var rids []string
	for _, p := range []string{"/a", "/b", "/c"} {
		rids = append(rids, serve(h, httptest.NewRequest("GET", p, nil)).Header().Get("X-Request-Id"))
	}
	got := summaries(t, h)
	if len(got) != 3 {
		t.Fatalf("%d summaries, want 3: %+v", len(got), got)
	}
	for i, want := range []string{"/c", "/b", "/a"} {
		if got[i].Path != want || got[i].RID != rids[2-i] || got[i].Method != "GET" || got[i].Status != 200 || got[i].Route != want {
			t.Errorf("summary %d = %+v, want %s rid %s", i, got[i], want, rids[2-i])
		}
	}
}

// A request's record carries the lines it logged (its own first, then each step's effect line),
// its steps in order, its statements with
// their N+1 group's count, and the N+1 warning the router logged.
func TestDevRequests_Detail(t *testing.T) {
	const sel = `SELECT "id" FROM "T" WHERE "id" = ?`
	call := func(name string, sqls int) eff.Task[int] {
		return eff.Call(name, false, func(e *eff.Env) (int, error) {
			for i := range sqls {
				e.Rec.AddSQL(eff.SQLRec{Stmt: sel, Args: []any{i, "x"}, Rows: 1})
			}
			return 0, nil
		})
	}
	h := devRouter(t, []Route{{Method: "GET", Pattern: "/d/{id}", Handle: func(rq *Request) Response {
		rq.Env.Log("info", "loading the page")
		if _, err := eff.Run(rq.Env, eff.KTask, call("Db.first", 0)); err != nil {
			panic(err)
		}
		if _, err := eff.RunQuery(rq.Env, eff.Q(call("Db.second", 5))); err != nil {
			panic(err)
		}
		return PageResponse(doc("d"), nil, nil, false)
	}}}, func(o *Options) { o.Log = func(string, string) {} })
	rid := serve(h, httptest.NewRequest("GET", "/d/7", nil)).Header().Get("X-Request-Id")

	w := serve(h, httptest.NewRequest("GET", "/_dev/requests/"+rid, nil))
	if w.Code != 200 {
		t.Fatalf("detail: %d %s", w.Code, w.Body.String())
	}
	var raw map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &raw); err != nil {
		t.Fatal(err)
	}
	for _, k := range []string{"rid", "t", "method", "path", "route", "status", "ms", "bytes", "steps", "sql", "loads", "cuts", "logs", "slow", "warnings"} {
		if _, ok := raw[k]; !ok {
			t.Errorf("no %q in %s", k, w.Body.String())
		}
	}
	var rec ReqRecord
	if err := json.Unmarshal(w.Body.Bytes(), &rec); err != nil {
		t.Fatal(err)
	}
	if rec.RID != rid || rec.Path != "/d/7" || rec.Route != "/d/{id}" || rec.Status != 200 || rec.Bytes == 0 {
		t.Errorf("record %+v", rec)
	}
	if len(rec.Logs) == 0 || rec.Logs[0].Msg != "loading the page" || rec.Logs[0].Level != "info" || rec.Logs[0].T.IsZero() {
		t.Errorf("logs %+v", rec.Logs)
	}
	if len(rec.Steps) != 2 || rec.Steps[0].Name != "Db.first" || rec.Steps[1].Name != "Db.second" {
		t.Errorf("steps %+v", rec.Steps)
	}
	if len(rec.SQL) != 5 {
		t.Fatalf("sql %+v", rec.SQL)
	}
	for i, q := range rec.SQL {
		if q.Stmt != sel || q.Repeats != 5 || q.Rows != 1 || len(q.Args) != 2 || q.Args[0] != fmt.Sprint(i) || q.Args[1] != "x" {
			t.Errorf("sql %d = %+v", i, q)
		}
	}
	if rec.Loads != 1 {
		t.Errorf("loads %d", rec.Loads)
	}
	if len(rec.Warnings) != 1 || !strings.HasPrefix(rec.Warnings[0], "N+1:") {
		t.Errorf("warnings %q", rec.Warnings)
	}
	s := summaries(t, h)
	if len(s) != 1 || s[0].Steps != 2 || s[0].Stmts != 5 || s[0].Repeats != 1 {
		t.Errorf("summary %+v", s)
	}
}

// The ring keeps the last 200 records: the 201st request evicts the first, whose detail then
// answers 404 with a JSON error.
func TestDevRequests_Evicts(t *testing.T) {
	h := devRouter(t, []Route{pageRoute("/a")}, nil)
	var rids []string
	for range requestRing + 1 {
		rids = append(rids, serve(h, httptest.NewRequest("GET", "/a", nil)).Header().Get("X-Request-Id"))
	}
	w := serve(h, httptest.NewRequest("GET", "/_dev/requests/"+rids[0], nil))
	if w.Code != 404 || !strings.HasPrefix(w.Header().Get("Content-Type"), "application/json") {
		t.Fatalf("evicted: %d %q", w.Code, w.Header().Get("Content-Type"))
	}
	var e map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &e); err != nil || e["error"] != "no request "+rids[0]+" in the last 200" {
		t.Errorf("evicted body %s (%v)", w.Body.String(), err)
	}
	if w := serve(h, httptest.NewRequest("GET", "/_dev/requests/"+rids[requestRing], nil)); w.Code != 200 {
		t.Errorf("the 201st: %d", w.Code)
	}
	if n := len(summaries(t, h)); n != requestRing {
		t.Errorf("%d summaries, want %d", n, requestRing)
	}
}

// Without ALDER_DEV the paths are the app's (its 404 here) and nothing is recorded.
func TestDevRequests_OffOutsideDev(t *testing.T) {
	t.Setenv("ALDER_DEV", "")
	h := NewRouter([]Route{pageRoute("/a")}, testOpts(func(o *Options) { o.Dev = true })).(*router)
	serve(h, httptest.NewRequest("GET", "/a", nil))
	w := serve(h, httptest.NewRequest("GET", "/_dev/requests", nil))
	if w.Code != 404 || strings.Contains(w.Header().Get("Content-Type"), "json") {
		t.Errorf("/_dev/requests outside dev: %d %q", w.Code, w.Header().Get("Content-Type"))
	}
	if n := h.reqs.len(); n != 0 {
		t.Errorf("%d records outside dev", n)
	}
}

// The dev endpoints are never recorded: neither the list, nor a detail lookup that misses.
func TestDevRequests_NotSelfRecorded(t *testing.T) {
	h := devRouter(t, []Route{pageRoute("/a")}, nil)
	serve(h, httptest.NewRequest("GET", "/_dev/requests/nope", nil))
	first := summaries(t, h)
	second := summaries(t, h)
	if len(first) != 0 || len(second) != 0 {
		t.Errorf("dev requests recorded: %+v then %+v", first, second)
	}
}

// A static file answered 200 runs no handler: it is not recorded.
func TestDevRequests_StaticNotRecorded(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "app.css"), []byte("body{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	h := devRouter(t, nil, func(o *Options) { o.StaticDir = dir })
	if w := serve(h, httptest.NewRequest("GET", "/static/app.css", nil)); w.Code != 200 {
		t.Fatalf("static: %d", w.Code)
	}
	if s := summaries(t, h); len(s) != 0 {
		t.Errorf("static recorded: %+v", s)
	}
}

// The dev endpoints' 2xx access lines are quiet, like /_dev's.
func TestDevRequests_Quiet(t *testing.T) {
	for _, c := range []struct {
		path   string
		status int
		want   bool
	}{
		{"/_dev/requests", 200, true},
		{"/_dev/requests/abc", 200, true},
		{"/_dev/requests/abc", 404, false},
		{"/_dev/map.json", 200, true},
		{"/_dev/requestsx", 200, false},
	} {
		if got := quiet(c.path, c.status); got != c.want {
			t.Errorf("quiet(%q, %d) = %t", c.path, c.status, got)
		}
	}
}

// The router's own line about a request (a handler's panic) goes through Options.LogID with the
// request's id and is in the record's logs, so the detail of a 500 says why.
func TestDevRequests_PanicLineCarriesRidAndIsRecorded(t *testing.T) {
	var mu sync.Mutex
	var got []string
	h := devRouter(t, []Route{{Method: "GET", Pattern: "/boom", Handle: func(rq *Request) Response { panic("kaboom") }}}, func(o *Options) {
		o.LogID = func(id, level, msg string) {
			mu.Lock()
			defer mu.Unlock()
			got = append(got, id+" "+level+" "+msg)
		}
	})
	rid := serve(h, httptest.NewRequest("GET", "/boom", nil)).Header().Get("X-Request-Id")
	mu.Lock()
	if len(got) != 1 || !strings.HasPrefix(got[0], rid+" error ") || !strings.Contains(got[0], "kaboom") {
		t.Errorf("LogID got %q, want one error line for %s", got, rid)
	}
	mu.Unlock()
	w := serve(h, httptest.NewRequest("GET", "/_dev/requests/"+rid, nil))
	var rec ReqRecord
	if err := json.Unmarshal(w.Body.Bytes(), &rec); err != nil {
		t.Fatalf("%v: %s", err, w.Body.String())
	}
	if rec.Status != 500 || len(rec.Logs) != 1 || rec.Logs[0].Level != "error" || !strings.Contains(rec.Logs[0].Msg, "kaboom") {
		t.Errorf("record %d logs %+v, want the panic line", rec.Status, rec.Logs)
	}
}

// A refusal (a router warning about the request) is in the record's logs and carries the rid too.
func TestDevRequests_RefusalLineCarriesRid(t *testing.T) {
	var got []string
	h := devRouter(t, []Route{pageRoute("/p")}, func(o *Options) {
		o.LogID = func(id, level, msg string) { got = append(got, id+" "+level) }
	})
	w := serve(h, httptest.NewRequest("GET", "/p?%zz", nil))
	rid := w.Header().Get("X-Request-Id")
	if len(got) == 0 || got[0] != rid+" warn" {
		t.Errorf("LogID got %q, want a warn line for %s (status %d)", got, rid, w.Code)
	}
}
