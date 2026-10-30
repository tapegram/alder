package web

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"alder/rt/applog"
	"alder/rt/datastar"
	"alder/rt/db"
	"alder/rt/eff"
	"alder/rt/metrics"
)

const metricsToken = "m3trics-t0ken-ABCDEFGH"

type note struct{ ID, Body string }

var noteTable = &db.Table[note]{
	Name:    "Note.Note",
	Columns: []db.Column{{Name: "id", Type: db.CText}, {Name: "body", Type: db.CText}},
	ID:      "id",
	Encode:  func(n note) []any { return []any{n.ID, n.Body} },
	Decode: func(v []any) (note, error) {
		var n note
		n.ID, _ = v[0].(string)
		n.Body, _ = v[1].(string)
		return n, nil
	},
}

func noteStore(t *testing.T) *db.SQLite {
	t.Helper()
	st := healthStore(t)
	if err := st.Ensure(context.Background(), []db.TableDesc{noteTable.Desc()}, true); err != nil {
		t.Fatal(err)
	}
	return st
}

func dbAll(rq *Request) {
	all := eff.Call("Db.all Note", false, func(e *eff.Env) ([]note, error) {
		return db.All(e.Ctx, e.Store.(db.Store), db.TxOf(e), db.From(noteTable))
	})
	if _, err := eff.Run(rq.Env, eff.KQuery, all); err != nil {
		panic(err)
	}
}

func dbInsert(rq *Request) {
	ins := eff.Call("Db.insert Note", true, func(e *eff.Env) (string, error) {
		return db.InsertRow(e.Ctx, e.Store.(db.Store), db.TxOf(e), noteTable, []any{nil, "x"}, e.Now())
	})
	if _, err := eff.Run(rq.Env, eff.KTask, ins); err != nil {
		panic(err)
	}
}

func page(rq *Request) Response { return PageResponse(doc("x"), nil, nil, false) }

// scrape is the exposition the router answers (GET MetricsPath with auth, when set).
func scrape(t *testing.T, h http.Handler, auth string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest("GET", MetricsPath, nil)
	if auth != "" {
		req.Header.Set("Authorization", auth)
	}
	return serve(h, req)
}

// value is the value of the series written exactly as series in body, or -1.
func value(t *testing.T, body, series string) float64 {
	t.Helper()
	for _, l := range strings.Split(body, "\n") {
		if v, ok := strings.CutPrefix(l, series+" "); ok {
			f, err := strconv.ParseFloat(v, 64)
			if err != nil {
				t.Fatalf("%s: %q", series, v)
			}
			return f
		}
	}
	return -1
}

// Without a Collector there is no endpoint: the path is a 404 like any other, access-logged.
func TestMetrics_Off(t *testing.T) {
	var s sink
	h := NewRouter(one("GET", "/", page), s.opts(nil))
	w := scrape(t, h, "")
	if w.Code != 404 || strings.Contains(w.Body.String(), "alder_") {
		t.Fatalf("%d %s", w.Code, w.Body.String())
	}
	if a := s.one(t); a.Status != 404 || a.Path != MetricsPath {
		t.Errorf("record %+v", a)
	}
}

func TestMetrics_Scrape(t *testing.T) {
	var s sink
	h := NewRouter(one("GET", "/", page), s.opts(func(o *Options) { o.Metrics, o.Build = metrics.New(""), "b-1" }))
	serve(h, httptest.NewRequest("GET", "/", nil))
	s.reset()
	w := scrape(t, h, "")
	if w.Code != 200 || w.Header().Get("Content-Type") != "text/plain; version=0.0.4; charset=utf-8" || w.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("%d %v", w.Code, w.Header())
	}
	b := w.Body.String()
	if !strings.Contains(b, "\nalder_http_requests_total{route=\"/\",method=\"GET\",status=\"2xx\"} 1\n") || !strings.Contains(b, `alder_build_info{build="b-1"} 1`) {
		t.Errorf("exposition:\n%s", b)
	}
	if strings.Contains(b, "_alder/metrics") {
		t.Errorf("the scrape counted itself:\n%s", b)
	}
	if len(s.access) != 0 {
		t.Errorf("a 200 scrape logged: %+v", s.access)
	}
	// a second scrape still counts only the page
	if v := value(t, scrape(t, h, "").Body.String(), `alder_http_requests_total{route="/",method="GET",status="2xx"}`); v != 1 {
		t.Errorf("after two scrapes: %v", v)
	}
	// HEAD: the headers, no body
	hw := serve(h, httptest.NewRequest("HEAD", MetricsPath, nil))
	if hw.Code != 200 || hw.Body.Len() != 0 || hw.Header().Get("Content-Type") != metrics.ContentType {
		t.Errorf("HEAD: %d %q %v", hw.Code, hw.Body.String(), hw.Header())
	}
	// a POST is not a scrape: routed (a 404 here)
	if pw := serve(h, httptest.NewRequest("POST", MetricsPath, nil)); pw.Code != 404 {
		t.Errorf("POST: %d", pw.Code)
	}
}

func TestMetrics_Token(t *testing.T) {
	var s sink
	h := NewRouter(one("GET", "/", page), s.opts(func(o *Options) { o.Metrics = metrics.New(metricsToken) }))
	for _, auth := range []string{"", "Bearer wrong-token-0123456789", "Basic " + metricsToken} {
		s.reset()
		w := scrape(t, h, auth)
		if w.Code != 401 || w.Header().Get("WWW-Authenticate") != `Bearer realm="alder"` || w.Body.String() != "unauthorized\n" {
			t.Errorf("%q: %d %v %q", auth, w.Code, w.Header(), w.Body.String())
		}
		if a := s.one(t); a.Status != 401 || a.Path != MetricsPath || a.Quiet {
			t.Errorf("%q: record %+v", auth, a)
		}
	}
	s.reset()
	if w := scrape(t, h, "Bearer "+metricsToken); w.Code != 200 || !strings.Contains(w.Body.String(), "alder_http_requests_in_flight 0") {
		t.Errorf("right token: %d\n%s", w.Code, w.Body.String())
	}
	if len(s.access) != 0 {
		t.Errorf("a 200 scrape logged: %+v", s.access)
	}
	// the refused probes are counted under the runtime's fixed route
	if v := value(t, scrape(t, h, "Bearer "+metricsToken).Body.String(), `alder_http_requests_total{route="/_alder/",method="GET",status="4xx"}`); v != 3 {
		t.Errorf("401s counted %v", v)
	}
}

// The token never reaches a log line, an access record, a response or a header the router
// writes, whether it is right or wrong.
func TestMetrics_TokenNotLogged(t *testing.T) {
	var b strings.Builder
	var mu sync.Mutex
	lg := applog.New(writerFunc(func(p []byte) (int, error) { mu.Lock(); defer mu.Unlock(); return b.Write(p) }), applog.Debug, applog.JSON)
	h := NewRouter(one("GET", "/", page), testOpts(func(o *Options) {
		o.Metrics, o.Log, o.Access = metrics.New(metricsToken), lg.Log, lg.Access
	}))
	var bodies strings.Builder
	for _, auth := range []string{"Bearer " + metricsToken, "Bearer " + metricsToken + "x", "Bearer " + metricsToken[:10]} {
		w := scrape(t, h, auth)
		bodies.WriteString(w.Body.String())
		for k, v := range w.Header() {
			bodies.WriteString(k + ": " + strings.Join(v, ",") + "\n")
		}
	}
	mu.Lock()
	defer mu.Unlock()
	for what, s := range map[string]string{"log": b.String(), "answers": bodies.String()} {
		if strings.Contains(s, metricsToken[:10]) {
			t.Errorf("the %s show the token:\n%s", what, s)
		}
	}
	if !strings.Contains(b.String(), `"status":401`) {
		t.Errorf("no 401 access line:\n%s", b.String())
	}
}

type writerFunc func([]byte) (int, error)

func (f writerFunc) Write(p []byte) (int, error) { return f(p) }

func TestMetrics_Route404(t *testing.T) {
	h := NewRouter(one("GET", "/todos/{id}", page), testOpts(func(o *Options) { o.Metrics = metrics.New("") }))
	serve(h, httptest.NewRequest("GET", "/todos/7", nil))
	serve(h, httptest.NewRequest("GET", "/todos/8", nil))
	serve(h, httptest.NewRequest("GET", "/nowhere", nil))
	serve(h, httptest.NewRequest("GET", "/todos", nil))
	b := scrape(t, h, "").Body.String()
	if v := value(t, b, `alder_http_requests_total{route="/todos/{id}",method="GET",status="2xx"}`); v != 2 {
		t.Errorf("pattern: %v\n%s", v, b)
	}
	if v := value(t, b, `alder_http_requests_total{route="(none)",method="GET",status="4xx"}`); v != 2 {
		t.Errorf("(none): %v\n%s", v, b)
	}
}

func TestMetrics_FileRoutes(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.css"), []byte("x{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	h := NewRouter(nil, testOpts(func(o *Options) { o.Metrics, o.StaticDir = metrics.New(""), dir }))
	serve(h, httptest.NewRequest("GET", "/static/a.css", nil))
	serve(h, httptest.NewRequest("GET", "/static/missing-"+strings.Repeat("x", 30)+".css", nil))
	serve(h, httptest.NewRequest("GET", datastar.ScriptPath, nil))
	b := scrape(t, h, "").Body.String()
	for series, n := range map[string]float64{
		`alder_http_requests_total{route="/static/",method="GET",status="2xx"}`: 1,
		`alder_http_requests_total{route="/static/",method="GET",status="4xx"}`: 1,
		`alder_http_requests_total{route="/_alder/",method="GET",status="2xx"}`: 1,
	} {
		if v := value(t, b, series); v != n {
			t.Errorf("%s: %v\n%s", series, v, b)
		}
	}
}

// labels are every route, method and status label value in an exposition.
var labelRE = regexp.MustCompile(`(route|method|status)="((?:[^"\\]|\\.)*)"`)

// Hostile paths, queries and methods never become label values: the route label stays a pattern
// or a fixed name, the method one of four, the status one of five.
func TestMetrics_HostileLabels(t *testing.T) {
	h := NewRouter([]Route{{Method: "GET", Pattern: "/u/{id}", Handle: page}, {Method: "POST", Pattern: "/u/{id}", Handle: page}},
		testOpts(func(o *Options) { o.Metrics = metrics.New("") }))
	paths := []string{
		"/u/" + strings.Repeat("a", 5000), "/u/%22%7D%20evil%0A", "/u/x?route=evil&method=EVIL", "/x%0aalder_build_info{build=%22pwn%22}%201",
		"/static/../../etc/passwd", "/_alder/evil", "/_dev?x=1", "/" + strings.Repeat("z/", 200), "/u/x/../../y", "/%2e%2e/%00",
	}
	methods := []string{"GET", "POST", "HEAD", "PUT", "DELETE", "PROPFIND", "EVIL-METHOD", "OPTIONS"}
	for _, p := range paths {
		for _, m := range methods {
			req := httptest.NewRequest("GET", "/", nil)
			req.Method = m
			u, err := req.URL.Parse(p)
			if err != nil {
				t.Fatal(err)
			}
			req.URL = u
			req.Header.Set("X-Request-Id", "evil-id")
			serve(h, req)
		}
	}
	b := scrape(t, h, "").Body.String()
	allowed := map[string]map[string]bool{
		"route":  {"/u/{id}": true, "(none)": true, "/static/": true, "/_alder/": true, "/_dev": true},
		"method": {"GET": true, "POST": true, "HEAD": true, "other": true},
		"status": {"1xx": true, "2xx": true, "3xx": true, "4xx": true, "5xx": true},
	}
	found := labelRE.FindAllStringSubmatch(b, -1)
	if len(found) < 20 {
		t.Fatalf("%d labels: the requests were not counted\n%s", len(found), b)
	}
	for _, m := range found {
		if !allowed[m[1]][m[2]] {
			t.Errorf("label %s=%q came from the request", m[1], m[2])
		}
	}
	for _, bad := range []string{"evil", "EVIL", "pwn", "passwd", "PROPFIND", "aaaa"} {
		if strings.Contains(b, bad) {
			t.Errorf("exposition holds %q:\n%s", bad, b)
		}
	}
	if n := strings.Count(b, "\nalder_http_requests_total{"); n > 5*4*5 {
		t.Errorf("%d request series", n)
	}
}

func TestMetrics_InFlight(t *testing.T) {
	entered, release := make(chan struct{}), make(chan struct{})
	h := NewRouter(one("GET", "/park", func(rq *Request) Response {
		close(entered)
		<-release
		return page(rq)
	}), testOpts(func(o *Options) { o.Metrics = metrics.New("") }))
	done := make(chan struct{})
	go func() {
		serve(h, httptest.NewRequest("GET", "/park", nil))
		close(done)
	}()
	<-entered
	if v := value(t, scrape(t, h, "").Body.String(), "alder_http_requests_in_flight"); v != 1 {
		t.Errorf("parked: %v", v)
	}
	close(release)
	<-done
	if v := value(t, scrape(t, h, "").Body.String(), "alder_http_requests_in_flight"); v != 0 {
		t.Errorf("after: %v", v)
	}
	// the health check is not in flight either
	serve(h, httptest.NewRequest("GET", HealthPath, nil))
	if v := value(t, scrape(t, h, "").Body.String(), "alder_http_requests_in_flight"); v != 0 {
		t.Errorf("after health: %v", v)
	}
}

func TestMetrics_Effects(t *testing.T) {
	st := noteStore(t)
	h := NewRouter(one("GET", "/", func(rq *Request) Response { dbAll(rq); dbAll(rq); return page(rq) }),
		testOpts(func(o *Options) { o.Metrics, o.Store = metrics.New(""), st }))
	serve(h, httptest.NewRequest("GET", "/", nil))
	b := scrape(t, h, "").Body.String()
	if v := value(t, b, `alder_effect_steps_total{name="Db.all Note"}`); v != 2 {
		t.Errorf("steps %v\n%s", v, b)
	}
}

func TestMetrics_Statements(t *testing.T) {
	st := noteStore(t)
	h := NewRouter(one("GET", "/", func(rq *Request) Response { dbInsert(rq); dbAll(rq); return page(rq) }),
		testOpts(func(o *Options) { o.Metrics, o.Store = metrics.New(""), st }))
	serve(h, httptest.NewRequest("GET", "/", nil))
	b := scrape(t, h, "").Body.String()
	for _, k := range []string{"exec", "query"} {
		if v := value(t, b, `alder_db_statements_total{kind="`+k+`"}`); v < 1 {
			t.Errorf("%s statements %v\n%s", k, v, b)
		}
		if v := value(t, b, `alder_db_statement_seconds_total{kind="`+k+`"}`); v <= 0 {
			t.Errorf("%s seconds %v\n%s", k, v, b)
		}
	}
}

// A scrape reads no session, sets no cookie and shows no dev panel, in dev mode too.
func TestMetrics_NoSessionNoPanel(t *testing.T) {
	t.Setenv("ALDER_DEV", "1")
	h := NewRouter(one("GET", "/", page), testOpts(func(o *Options) { o.Metrics, o.Dev = metrics.New(""), true }))
	req := httptest.NewRequest("GET", MetricsPath, nil)
	req.AddCookie(&http.Cookie{Name: CookieName, Value: "garbage"})
	w := serve(h, req)
	if w.Code != 200 || w.Header().Get("Set-Cookie") != "" || strings.Contains(w.Body.String(), "al-devpanel") || strings.Contains(w.Body.String(), "<") {
		t.Errorf("%d %v\n%s", w.Code, w.Header(), w.Body.String())
	}
}

// A request at or over SlowAfter is Slow in its record, with its route, statements and steps.
func TestSlow_Promoted(t *testing.T) {
	var s sink
	st := noteStore(t)
	h := NewRouter(one("GET", "/todos/{id}", func(rq *Request) Response {
		dbAll(rq)
		time.Sleep(5 * time.Millisecond)
		return page(rq)
	}), s.opts(func(o *Options) { o.SlowAfter, o.Store = time.Millisecond, st }))
	serve(h, httptest.NewRequest("GET", "/todos/3", nil))
	a := s.one(t)
	if !a.Slow || a.Route != "/todos/{id}" || a.Stmts < 1 || a.DBDur <= 0 || a.Steps != 1 {
		t.Errorf("record %+v", a)
	}
}

// SlowAfter 0 and no Collector: never Slow, Env.Stats nil.
func TestSlow_Off(t *testing.T) {
	var s sink
	var stats *eff.Stats
	h := NewRouter(one("GET", "/", func(rq *Request) Response {
		stats = rq.Env.Stats
		time.Sleep(2 * time.Millisecond)
		return page(rq)
	}), s.opts(nil))
	serve(h, httptest.NewRequest("GET", "/", nil))
	if a := s.one(t); a.Slow || a.Stmts != 0 {
		t.Errorf("record %+v", a)
	}
	if stats != nil {
		t.Error("Env.Stats set with metrics and slow off")
	}
	// either knob sets it
	for _, edit := range []func(*Options){func(o *Options) { o.SlowAfter = time.Hour }, func(o *Options) { o.Metrics = metrics.New("") }} {
		stats = nil
		serve(NewRouter(one("GET", "/", func(rq *Request) Response { stats = rq.Env.Stats; return page(rq) }), testOpts(edit)), httptest.NewRequest("GET", "/", nil))
		if stats == nil {
			t.Error("Env.Stats nil with a knob on")
		}
	}
}

// The health check is never Slow (it has no record) and never counted.
func TestSlow_HealthNever(t *testing.T) {
	var s sink
	st := &slowPing{SQLite: healthStore(t)}
	h := NewRouter(nil, s.opts(func(o *Options) { o.SlowAfter, o.Store, o.Metrics = time.Nanosecond, st, metrics.New("") }))
	if w := serve(h, httptest.NewRequest("GET", HealthPath, nil)); w.Code != 200 {
		t.Fatalf("%d", w.Code)
	}
	if len(s.access) != 0 {
		t.Errorf("records %+v", s.access)
	}
	if b := scrape(t, h, "").Body.String(); strings.Contains(b, "alder_http_requests_total{") {
		t.Errorf("health counted:\n%s", b)
	}
}

type slowPing struct{ *db.SQLite }

func (s *slowPing) Ping(ctx context.Context) error {
	time.Sleep(2 * time.Millisecond)
	return s.SQLite.Ping(ctx)
}
