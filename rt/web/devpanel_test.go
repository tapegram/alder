package web

import (
	"net/http"
	"os"
	"regexp"

	"alder/internal/csscheck"
	"alder/rt/eff"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestDevPanel(t *testing.T) {
	t.Setenv("ALDER_DEV", "1")
	route := one("GET", "/x", func(rq *Request) Response { return PageResponse(doc("hi"), nil, nil, false) })
	for _, tc := range []struct {
		dev, ds, off bool
		want         []string
		not          []string
	}{
		{dev: true, want: []string{"<al-devpanel", "aria-controls=\"_alder_tab_trace\"", "_alder_tab_sql"}},
		{dev: true, want: []string{"aria-controls=\"_alder_tab_history\"", "data-copy=\"curl -X GET &#39;http://", "Cookie: alder_session=&lt;your session cookie&gt;", "data-rid=", "sessionStorage"}},
		{dev: true, want: []string{`id="_alder_ui"`, "_alder_tab_errors", "data-open-errors", "data-ms"}},
		{dev: true, ds: true, want: []string{"data: selector #_alder_panel", "data: mode inner", "data-ignore-morph"}},
		{dev: true, off: true, not: []string{"al-devpanel"}},
		{dev: false, not: []string{"al-devpanel", "_alder_tab", "sessionStorage", "_alder_hist", "_alder_ui", "_alder_errors", "data-ms", "localStorage"}},
	} {
		if tc.off {
			t.Setenv("ALDER_DEV_PANEL", "0")
		}
		h := NewRouter(route, testOpts(func(o *Options) { o.Dev = tc.dev }))
		req := httptest.NewRequest("GET", "/x", nil)
		if tc.ds {
			req.Header.Set("Datastar-Request", "true")
		}
		body := serve(h, req).Body.String()
		for _, s := range tc.want {
			if !strings.Contains(body, s) {
				t.Errorf("%+v: missing %q in %q", tc, s, body)
			}
		}
		for _, s := range tc.not {
			if strings.Contains(body, s) {
				t.Errorf("%+v: has %q", tc, s)
			}
		}
	}
}

func TestDevMap(t *testing.T) {
	route := one("GET", "/x", func(rq *Request) Response { return PageResponse(doc("hi"), nil, nil, false) })
	for _, dev := range []bool{true, false} {
		t.Setenv("ALDER_DEV", "1")
		h := NewRouter(route, testOpts(func(o *Options) { o.Dev = dev }))
		w := serve(h, httptest.NewRequest("GET", "/_dev", nil))
		if dev && (w.Code != 200 || !strings.Contains(w.Body.String(), `href="/x"`)) {
			t.Fatalf("dev: %d %q", w.Code, w.Body.String())
		}
		if !dev && w.Code != 404 {
			t.Fatalf("prod: %d", w.Code)
		}
	}
}

func TestNoticedToast(t *testing.T) {
	h := NewRouter(one("POST", "/_m/A/B", func(rq *Request) Response {
		rq.Notice = "Saved <ok>"
		return PageResponse(doc("hi"), nil, nil, true)
	}), testOpts(nil))
	req := httptest.NewRequest("POST", "/_m/A/B", nil)
	req.Header.Set("Datastar-Request", "true")
	body := serve(h, req).Body.String()
	for _, s := range []string{
		"data: selector al-toasts\ndata: mode append\ndata: elements <al-toast variant=\"info\" timeout=\"5000\">Saved &lt;ok&gt;</al-toast>",
		"<al-toasts popover=\"manual\" data-ignore-morph></al-toasts></body>",
	} {
		if !strings.Contains(body, s) {
			t.Errorf("missing %q in %q", s, body)
		}
	}
	// the toast comes after the body patch, so a morph does not show it again
	if strings.Index(body, "selector body") > strings.Index(body, "selector al-toasts") {
		t.Errorf("order: %q", body)
	}
}

func TestDevMapContents(t *testing.T) {
	t.Setenv("ALDER_DEV", "1")
	route := one("GET", "/x", func(rq *Request) Response { return PageResponse(doc("hi"), nil, nil, false) })
	h := NewRouter(route, testOpts(func(o *Options) {
		o.Dev = true
		o.Migrations = []string{"0001_init"}
		o.DevMap = `{"pages":[{"Module":"Posts.Show","Path":"/posts/{slug}","Requires":"user","Roles":["Admin"],"Params":["slug: Post.Slug"],"Msgs":["Remove"]}],` +
			`"tables":[{"Name":"Post.Post","Columns":["title: Text"],"Unique":["slug"]}],"session":["user: Maybe User.UserId"]}`
	}))
	body := serve(h, httptest.NewRequest("GET", "/_dev", nil)).Body.String()
	for _, want := range []string{"Posts.Show", "requires user, role Admin", "slug: Post.Slug", "message <code>Remove", "title: Text", "unique: slug", "user: Maybe User.UserId", "0001_init", "build <code>"} {
		if !strings.Contains(body, want) {
			t.Errorf("missing %q in %s", want, body)
		}
	}
}

// A message answering a redirect: the panel the followed page carries is the message's (its
// form fields, password masked, in Request and curl), with the GET's steps and statements after
// a "then GET …" marker, both answers, every patch, and only the Request section shown.
func TestDevPanel_Redirect(t *testing.T) {
	t.Setenv("ALDER_DEV", "1")
	routes := []Route{
		{Method: "POST", Pattern: "/_m/A/Save", Handle: func(rq *Request) Response {
			rq.Env.Rec.AddStep(eff.StepRec{Name: "Db.delete"})
			rq.Env.Rec.AddSQL(eff.SQLRec{Stmt: "DELETE FROM x", Rows: 2})
			return RedirectResponse("/list", nil)
		}},
		{Method: "GET", Pattern: "/list", Handle: func(rq *Request) Response {
			rq.Env.Rec.AddStep(eff.StepRec{Name: "Db.query"})
			rq.Env.Rec.AddSQL(eff.SQLRec{Stmt: "SELECT y", Rows: 3})
			return PageResponse(doc("list"), nil, nil, false)
		}},
	}
	h := NewRouter(routes, testOpts(func(o *Options) { o.Dev = true }))
	req := httptest.NewRequest("POST", "/_m/A/Save", strings.NewReader("q=ann&password=hunter2"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Datastar-Request", "true")
	body := serve(h, req).Body.String()
	panel := body[strings.Index(body, "selector #_alder_panel"):]
	order := []string{
		"redirect /list, then page",
		"form password = ••••", "form q = ann", "session {", "&#34;User&#34;: &#34;&#34;",
		"--data-urlencode &#39;password=••••&#39; --data-urlencode &#39;q=ann&#39;",
		`<section id="_alder_tab_trace" hidden>`, "Db.delete", "<li data-hop>then GET /list</li>", "Db.query",
		"DELETE FROM x", "<li data-hop>then GET /list</li>", "SELECT y", ", 3 rows",
		"<p>redirect /list</p><p>then GET /list: 200 page</p>",
		"<li>body</li><li>title</li><li>history /list</li><li>panel</li>",
	}
	at := 0
	for _, s := range order {
		i := strings.Index(panel[at:], s)
		if i < 0 {
			t.Fatalf("missing %q (in order) in %s", s, panel)
		}
		at += i + len(s)
	}
	if strings.Contains(panel, "hunter2") {
		t.Errorf("the password is shown: %s", panel)
	}
	if !strings.Contains(panel, `<section id="_alder_tab_request"><h3>`) || !strings.Contains(panel, `aria-controls="_alder_tab_request" aria-selected="true"`) {
		t.Errorf("Request is not the section shown: %s", panel)
	}
}

// A Noticed answer to a form posted without Datastar: a redirect carries it in a signed cookie
// to the next page, which shows it as an <al-alert> once; a Rerender shows it in place.
func TestNoticedWithoutScript(t *testing.T) {
	routes := []Route{
		{Method: "POST", Pattern: "/_m/A/Go", Handle: func(rq *Request) Response {
			rq.Notice = "Deleted 3 of 4 <rows>"
			return RedirectResponse("/list", nil)
		}},
		{Method: "POST", Pattern: "/_m/A/Stay", Handle: func(rq *Request) Response {
			rq.Notice = "Saved"
			return PageResponse(doc("here"), nil, nil, true)
		}},
		{Method: "GET", Pattern: "/list", Handle: func(rq *Request) Response { return PageResponse(doc("list"), nil, nil, false) }},
	}
	h := NewRouter(routes, testOpts(nil))
	w := serve(h, httptest.NewRequest("POST", "/_m/A/Go", nil))
	var c *http.Cookie
	for _, k := range w.Result().Cookies() {
		if k.Name == NoticeCookie {
			c = k
		}
	}
	if w.Code != 303 || c == nil {
		t.Fatalf("%d, cookie %v", w.Code, c)
	}
	get := httptest.NewRequest("GET", "/list", nil)
	get.AddCookie(c)
	w = serve(h, get)
	alert := `<body><al-alert variant="info" role="status">Deleted 3 of 4 &lt;rows&gt;</al-alert>`
	if !strings.Contains(w.Body.String(), alert) {
		t.Errorf("no alert: %s", w.Body.String())
	}
	if !strings.Contains(w.Header().Get("Set-Cookie"), NoticeCookie+"=;") {
		t.Errorf("the cookie is not cleared: %q", w.Header().Get("Set-Cookie"))
	}
	forged := httptest.NewRequest("GET", "/list", nil)
	forged.AddCookie(&http.Cookie{Name: NoticeCookie, Value: Sign([]byte("another key, another app"), []byte("notice:hi"))})
	if b := serve(h, forged).Body.String(); strings.Contains(b, "al-alert") {
		t.Errorf("a forged notice is shown: %s", b)
	}
	if b := serve(h, httptest.NewRequest("POST", "/_m/A/Stay", nil)).Body.String(); !strings.Contains(b, `<al-alert variant="info" role="status">Saved</al-alert>`) {
		t.Errorf("no alert on the re-rendered page: %s", b)
	}
}

// A statement run per id by one Task.all (each in its own step, as the admin's bulk delete
// does) is labelled bulk, not flagged as a duplicate; a repeated query outside a group, or with
// the same arguments, still is, and the session's user selected by both a POST and its
// redirect follow is not one.
func TestDevPanel_BulkNotDuplicate(t *testing.T) {
	t.Setenv("ALDER_DEV", "1")
	stmt := func(name, q string, arg any) eff.Task[int] {
		return eff.Call(name, false, func(e *eff.Env) (int, error) {
			e.Rec.AddSQL(eff.SQLRec{Stmt: q, Args: []any{arg}, Rows: 1})
			return 0, nil
		})
	}
	routes := []Route{
		{Method: "POST", Pattern: "/_m/A/Bulk", Handle: func(rq *Request) Response {
			var ds, same []eff.Task[int]
			for _, id := range []int{1, 2, 3} {
				ds = append(ds, stmt("Db.delete", "DELETE FROM p WHERE id = ?", id))
			}
			for range 2 {
				same = append(same, stmt("Db.get", "SELECT same", 1))
			}
			for _, t := range []eff.Task[[]int]{eff.All(ds), eff.All(same)} {
				if _, err := eff.Run(rq.Env, eff.KTask, t); err != nil {
					panic(err)
				}
			}
			for _, id := range []int{7, 8} { // distinct arguments, but not under a Task.all: N+1
				if _, err := eff.Run(rq.Env, eff.KTask, stmt("Db.get", "SELECT n1", id)); err != nil {
					panic(err)
				}
			}
			rq.Env.Rec.AddSQL(eff.SQLRec{Stmt: "SELECT user", Args: []any{"u"}, Rows: 1})
			return RedirectResponse("/list", nil)
		}},
		{Method: "GET", Pattern: "/list", Handle: func(rq *Request) Response {
			rq.Env.Rec.AddSQL(eff.SQLRec{Stmt: "SELECT user", Args: []any{"u"}, Rows: 1})
			return PageResponse(doc("list"), nil, nil, false)
		}},
	}
	h := NewRouter(routes, testOpts(func(o *Options) { o.Dev = true }))
	req := httptest.NewRequest("POST", "/_m/A/Bulk", nil)
	req.Header.Set("Datastar-Request", "true")
	b := serve(h, req).Body.String()
	for _, want := range []string{"x3 (bulk)", "duplicate x2", "duplicate x2"} {
		if !strings.Contains(b, want) {
			t.Errorf("missing %q in %s", want, b)
		}
	}
	if strings.Contains(b, "duplicate x3") {
		t.Errorf("the bulk statement is flagged: %s", b)
	}
	if strings.Count(b, "data-dup") != 4 { // two lines each: SELECT same (same args), SELECT n1 (not grouped)
		t.Errorf("%d duplicate flags, want 4 (the session select of both hops is not one): %s", strings.Count(b, "data-dup"), b)
	}
}

// A page whose load runs one select per row is an N+1: its statements are marked `N+1 x100` in
// the SQL tab, the bar says `N+1`, and the router logs one warn line naming the request; the same
// 100 statements as DELETEs under one Task.all are a bulk action, which neither marks nor logs.
func TestDevPanel_NPlusOne(t *testing.T) {
	t.Setenv("ALDER_DEV", "1")
	stmt := func(name, q string, arg any) eff.Task[int] {
		return eff.Call(name, false, func(e *eff.Env) (int, error) {
			e.Rec.AddSQL(eff.SQLRec{Stmt: q, Args: []any{arg}, Rows: 1})
			return 0, nil
		})
	}
	const get = `SELECT "id", "title", "n" FROM "Table__Row" WHERE "id" = ?`
	routes := []Route{
		{Method: "GET", Pattern: "/slow", Handle: func(rq *Request) Response {
			for id := range 100 {
				if _, err := eff.Run(rq.Env, eff.KQuery, stmt("Db.get", get, id)); err != nil {
					panic(err)
				}
			}
			return PageResponse(doc("slow"), nil, nil, false)
		}},
		{Method: "GET", Pattern: "/bulk", Handle: func(rq *Request) Response {
			var ds []eff.Task[int]
			for id := range 100 {
				ds = append(ds, stmt("Db.delete", `DELETE FROM "Table__Row" WHERE "id" = ?`, id))
			}
			if _, err := eff.Run(rq.Env, eff.KTask, eff.All(ds)); err != nil {
				panic(err)
			}
			return PageResponse(doc("bulk"), nil, nil, false)
		}},
	}
	var warns []string
	h := NewRouter(routes, testOpts(func(o *Options) {
		o.Dev = true
		o.Log = func(level, msg string) {
			if level == "warn" {
				warns = append(warns, msg)
			}
		}
	}))
	b := serve(h, httptest.NewRequest("GET", "/slow", nil)).Body.String()
	if !strings.Contains(b, "N+1 x100") || strings.Contains(b, "duplicate x100") {
		t.Errorf("the SQL tab does not mark the N+1: %s", b)
	}
	if !regexp.MustCompile(`<span>100 SQL</span> <span data-nplus1>N\+1</span>`).MatchString(b) {
		t.Errorf("the bar does not say N+1: %s", b)
	}
	if len(warns) != 1 || !strings.HasPrefix(warns[0], "GET /slow: N+1: SELECT \"id\", \"title\", … FROM \"Table__Row\" WHERE \"id\" = ? ran 100 times (100 argument sets") {
		t.Errorf("warn lines %q", warns)
	}
	warns = nil
	b = serve(h, httptest.NewRequest("GET", "/bulk", nil)).Body.String()
	if !strings.Contains(b, "x100 (bulk)") || strings.Contains(b, "N+1") {
		t.Errorf("the bulk delete is marked: %s", b)
	}
	if len(warns) != 0 {
		t.Errorf("warn lines for a bulk delete: %q", warns)
	}
}

// A Rerender runs the page's load twice: a load that runs one select 3 times is not an N+1 on
// the GET nor on the message (6 in the answer, but 3 per load run).
func TestDevPanel_RerenderNotNPlusOne(t *testing.T) {
	t.Setenv("ALDER_DEV", "1")
	load := eff.Q(eff.Call("Db.all", false, func(e *eff.Env) (int, error) {
		for range 3 {
			e.Rec.AddSQL(eff.SQLRec{Stmt: "SELECT all", Args: []any{}, Rows: 1})
		}
		return 0, nil
	}))
	page := func(n int) func(rq *Request) Response {
		return func(rq *Request) Response {
			for range n {
				if _, err := eff.RunQuery(rq.Env, load); err != nil {
					panic(err)
				}
			}
			return PageResponse(doc("p"), nil, nil, true)
		}
	}
	var warns []string
	h := NewRouter([]Route{{Method: "GET", Pattern: "/", Handle: page(1)}, {Method: "POST", Pattern: "/_m/P/Toggle", Handle: page(2)}},
		testOpts(func(o *Options) {
			o.Dev = true
			o.Log = func(level, msg string) {
				if level == "warn" {
					warns = append(warns, msg)
				}
			}
		}))
	for _, req := range []*http.Request{httptest.NewRequest("GET", "/", nil), httptest.NewRequest("POST", "/_m/P/Toggle", nil)} {
		req.Header.Set("Datastar-Request", "true")
		if b := serve(h, req).Body.String(); strings.Contains(b, "N+1") {
			t.Errorf("%s %s: marked N+1: %s", req.Method, req.URL.Path, b)
		}
	}
	if len(warns) != 0 {
		t.Errorf("warns %q", warns)
	}
}

// The notice cookie is consumed by a page GET only: not by a 404 (a favicon), a /_alder/ or
// /static/ path, or a HEAD.
func TestNoticeOnlyForPageGET(t *testing.T) {
	routes := []Route{
		{Method: "POST", Pattern: "/_m/A/Go", Handle: func(rq *Request) Response {
			rq.Notice = "hello"
			return RedirectResponse("/list", nil)
		}},
		{Method: "GET", Pattern: "/list", Handle: func(rq *Request) Response { return PageResponse(doc("list"), nil, nil, false) }},
	}
	h := NewRouter(routes, testOpts(nil))
	var nc *http.Cookie
	for _, k := range serve(h, httptest.NewRequest("POST", "/_m/A/Go", nil)).Result().Cookies() {
		if k.Name == NoticeCookie {
			nc = k
		}
	}
	if nc == nil {
		t.Fatal("no notice cookie")
	}
	cookie := func(path, method string, ds bool) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, path, nil)
		r.AddCookie(nc)
		if ds {
			r.Header.Set("Datastar-Request", "true")
		}
		return serve(h, r)
	}
	for _, c := range []struct {
		path, method string
		ds           bool
	}{{"/favicon.ico", "GET", false}, {"/_alder/datastar.js", "GET", false}, {"/static/app.css", "GET", false}, {"/static/app.css", "GET", true}, {"/list", "HEAD", false}} {
		w := cookie(c.path, c.method, c.ds)
		if strings.Contains(w.Header().Get("Set-Cookie"), NoticeCookie+"=;") {
			t.Errorf("%s %s cleared the notice", c.method, c.path)
		}
	}
	w := cookie("/list", "GET", false)
	if !strings.Contains(w.Body.String(), "hello") || !strings.Contains(w.Header().Get("Set-Cookie"), NoticeCookie+"=;") {
		t.Errorf("the page GET did not show and clear the notice: %s", w.Body.String())
	}
}

// prodPages renders, with dev mode off, a full page and a Datastar body patch of the same route,
// with the build id (the one nondeterministic part: the alder-build meta and the Alder-Build
// value) replaced by BUILD.
func prodPages(t *testing.T) string {
	t.Helper()
	t.Setenv("ALDER_DEV", "")
	route := one("GET", "/x", func(rq *Request) Response { return PageResponse(doc("hi"), nil, nil, false) })
	h := NewRouter(route, testOpts(func(o *Options) { o.Dev = false }))
	full := serveRaw(h, httptest.NewRequest("GET", "/x", nil)).Body.String()
	req := httptest.NewRequest("GET", "/x", nil)
	req.Header.Set("Datastar-Request", "true")
	patch := serveRaw(h, req).Body.String()
	out := "== full page\n" + full + "\n== datastar request\n" + patch + "\n"
	return regexp.MustCompile(`name="alder-build" content="[0-9a-f]+"`).ReplaceAllString(out, `name="alder-build" content="BUILD"`)
}

// Production output is byte for byte what it was before the dev panel's Errors tab, strip,
// resizing and minimising: testdata/production_page.golden was generated from the commit before
// them (168ad442), with the build id normalised to BUILD; the changes since are the pending
// bar's token, now alder-ui v0.4's --primary (#21), and the shown hash (#49): the boost's
// window._alderShown and the <al-shown> at the end of every body.
func TestProductionPageHasNoDevUI(t *testing.T) {
	if os.Getenv("UPDATE_GOLDEN") != "" {
		if err := os.WriteFile("testdata/production_page.golden", []byte(prodPages(t)), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile("testdata/production_page.golden")
	if err != nil {
		t.Fatal(err)
	}
	if got := prodPages(t); got != string(want) {
		t.Fatalf("production output changed:\ngot:  %q\nwant: %q", got, want)
	}
}

// The CSS the runtime injects (the dev panel's own rules and the pending bar) names only the
// design system's current tokens, each with a literal fallback, so it renders the same with
// alder-ui v0.4 loaded and on a page without it (spec §9).
func TestInjectedCSSTokens(t *testing.T) {
	for name, css := range map[string]string{"panelUICSS": panelUICSS, "pendingStyle": pendingStyle} {
		if got := csscheck.VarsWithoutLiteralFallback(css); len(got) > 0 {
			t.Errorf("%s: var() without a literal fallback: %q", name, got)
		}
		if got := csscheck.RemovedALTokens(css); len(got) > 0 {
			t.Errorf("%s: removed --al-* tokens: %q", name, got)
		}
	}
	if !strings.Contains(pendingStyle, "var(--primary,currentColor)") {
		t.Errorf("pendingStyle lacks var(--primary,currentColor): %s", pendingStyle)
	}
	for _, want := range []string{"var(--radius-sm,4px)", "var(--radius-md,6px)"} {
		if !strings.Contains(panelUICSS, want) {
			t.Errorf("panelUICSS lacks %s", want)
		}
	}
}
