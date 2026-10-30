package web

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"alder/rt/datastar"
	"alder/rt/html"
)

// Boosted navigation (M13): a Datastar GET of a page is answered with the morph of its body,
// its title and a history step; a redirect under Datastar to a page of this app is that page's
// morph; another site is a real navigation.

// dsGet is a Datastar GET of target, as the boost's click handler sends it (Datastar adds the
// signals as ?datastar=…).
func dsGet(target string) *http.Request {
	req := httptest.NewRequest("GET", target, nil)
	req.Header.Set("Datastar-Request", "true")
	return req
}

// events splits an SSE body into its events (without their blank lines).
func events(body string) []string {
	return strings.Split(strings.TrimSuffix(body, "\n\n"), "\n\n")
}

const (
	pushOpen    = "push"
	replaceOpen = "replace"
)

func bodyEvent(inner string) string {
	return "event: datastar-patch-elements\ndata: selector body\ndata: mode outer\ndata: elements <body>" + inner + "</body>"
}

func titleEvent(t string) string {
	return "event: datastar-patch-elements\ndata: selector head > title\ndata: mode outer\ndata: elements <title>" + t + "</title>"
}

// historyEvent checks ev is the history step of mode to the JSON string u.
func historyEvent(t *testing.T, ev, open, u string) {
	t.Helper()
	prefix := "event: datastar-patch-elements\ndata: selector body\ndata: mode append\ndata: elements " +
		`<script data-effect="el.remove()">(function(){var u=` + u + ";"
	if !strings.HasPrefix(ev, prefix) || !strings.HasSuffix(ev, "})()</script>") {
		t.Fatalf("history event %q, want prefix %q", ev, prefix)
	}
	step := `history.pushState(null,"",u);`
	if open == replaceOpen {
		step = `history.replaceState(history.state,"",u);`
	}
	if !strings.Contains(ev, step) {
		t.Fatalf("history event %q lacks %q", ev, step)
	}
	for _, part := range []string{"window._alderAt=location.pathname+location.search", "document.body.tabIndex=-1", "document.body.focus({preventScroll:true})"} {
		if !strings.Contains(ev, part) {
			t.Fatalf("history event %q lacks %q", ev, part)
		}
	}
	if open == pushOpen && !strings.Contains(ev, "scrollTo(0,0)") {
		t.Fatalf("push does not scroll to the top: %q", ev)
	}
	if open == replaceOpen && !strings.Contains(ev, "scrollTo(0,history.state&&history.state.alderY||0)") {
		t.Fatalf("replace does not restore the scroll: %q", ev)
	}
}

// A boosted GET of a page: 200 SSE — the body patch, the title patch, the push of the URL the
// page was asked for (Datastar's ?datastar= dropped, the other parameters kept in order); the
// page's handler sees the query without it.
func TestBoost_GetMorphsTitlePush(t *testing.T) {
	var got url.Values
	h := NewRouter(one("GET", "/posts", func(rq *Request) Response {
		got = rq.Query
		return PageResponse(doc("Posts &amp; more"), nil, rq.Session, false)
	}), testOpts(nil))
	w := serve(h, dsGet("/posts?b=2&datastar=%7B%7D&a=1"))
	if w.Code != 200 || w.Header().Get("Content-Type") != "text/event-stream" || w.Header().Get(StatusHeader) != "" || sessionCookie(t, w) != nil {
		t.Fatalf("%d %q %q", w.Code, w.Header().Get("Content-Type"), w.Header().Get(StatusHeader))
	}
	evs := events(w.Body.String())
	if len(evs) != 3 || evs[0] != bodyEvent("<p>Posts &amp;amp; more</p>") || evs[1] != titleEvent("Posts &amp;amp; more") {
		t.Fatalf("events %q", evs)
	}
	historyEvent(t, evs[2], pushOpen, "\"/posts?b=2\\u0026a=1\"")
	if got.Has("datastar") || got.Get("a") != "1" || got.Get("b") != "2" {
		t.Fatalf("query %v", got)
	}
	// Not boosted: the whole document, as before.
	first, _ := HeadTags()
	w = serve(h, httptest.NewRequest("GET", "/posts", nil))
	if w.Code != 200 || !strings.HasPrefix(w.Body.String(), "<!doctype html>\n<html><head>"+first+"<title>") {
		t.Fatalf("plain GET %d %q", w.Code, w.Body.String())
	}
}

// Back/forward: the popstate handler's GET (Alder-History: replace) is answered alike, with
// replaceState and the scroll restored instead of a push; a page without a <title> gets an
// empty one, so the old page's title does not stay.
func TestBoost_PopstateReplaces(t *testing.T) {
	h := NewRouter(one("GET", "/a", func(rq *Request) Response {
		return PageResponse(html.El("html", nil, html.El("body", nil, html.Text("A"))), nil, nil, false)
	}), testOpts(nil))
	req := dsGet("/a?datastar=%7B%7D")
	req.Header.Set(datastar.HistoryHeader, "replace")
	evs := events(serve(h, req).Body.String())
	if len(evs) != 3 || evs[0] != bodyEvent("A") || evs[1] != titleEvent("") {
		t.Fatalf("events %q", evs)
	}
	historyEvent(t, evs[2], replaceOpen, `"/a"`)
	// two boosted navigations push twice; going back replaces (the entry exists already)
	for range 2 {
		if evs := events(serve(h, dsGet("/a")).Body.String()); !strings.Contains(evs[len(evs)-1], `history.pushState(null,"",u)`) {
			t.Fatalf("push %q", evs)
		}
	}
}

// RF2: a boosted link to a page that answers 404 (no route, or the handler's NotFound) or 500
// morphs the error document in, with Alder-Status, its title and the push (the address bar
// shows the URL, as after a load).
func TestBoost_GetFailures(t *testing.T) {
	h := NewRouter([]Route{
		{Method: "GET", Pattern: "/n", Handle: func(rq *Request) Response { return NotFoundResponse(doc("No such post")) }},
		{Method: "GET", Pattern: "/e", Handle: func(rq *Request) Response { return ErrorResponse(html.None(), errors.New("boom")) }},
		{Method: "GET", Pattern: "/p", Handle: func(rq *Request) Response { panic("kaput") }},
	}, testOpts(nil))
	for _, c := range []struct{ path, status, text string }{
		{"/missing", "404", "Not found"}, {"/n", "404", "No such post"}, {"/e", "500", "Something went wrong"}, {"/p", "500", "Something went wrong"},
	} {
		w := serve(h, dsGet(c.path))
		evs := events(w.Body.String())
		if w.Code != 200 || w.Header().Get(StatusHeader) != c.status || len(evs) != 3 || evs[0] != bodyEvent("<p>"+c.text+"</p>") || evs[1] != titleEvent(c.text) {
			t.Fatalf("%s: %d %q %q", c.path, w.Code, w.Header().Get(StatusHeader), evs)
		}
		historyEvent(t, evs[2], pushOpen, `"`+c.path+`"`)
	}
	// a message's failure under Datastar is the body patch alone, as before
	h = NewRouter(one("POST", "/_m/A/B", func(rq *Request) Response { return NotFoundResponse(doc("Gone")) }), testOpts(nil))
	req := httptest.NewRequest("POST", "/_m/A/B", nil)
	req.Header.Set("Datastar-Request", "true")
	if evs := events(serve(h, req).Body.String()); len(evs) != 1 || evs[0] != bodyEvent("<p>Gone</p>") {
		t.Fatalf("message 404 %q", evs)
	}
}

// RF2: a message that redirects to a page of this app, under Datastar: the router serves the
// page itself, with the session the redirect stored, and answers with its morph and the push
// of the Location; the cookie is set as for a 303, and Options.Served sees the page's request.
// Without Datastar it is the 303, unchanged.
func TestBoost_RedirectMorphsTarget(t *testing.T) {
	var served []string
	h := NewRouter([]Route{
		{Method: "POST", Pattern: "/_m/Auth/Login", Handle: func(rq *Request) Response {
			return RedirectResponse("/home?tab=1#top", testSession{User: "ada"})
		}},
		{Method: "GET", Pattern: "/home", Handle: func(rq *Request) Response {
			u := rq.Session.(testSession).User
			rq.Model = u
			if rq.Form != nil || rq.HTTP.Method != "GET" || rq.Query.Get("tab") != "1" || !rq.Datastar {
				t.Errorf("target request: form %v method %s query %v", rq.Form, rq.HTTP.Method, rq.Query)
			}
			return PageResponse(doc("Hello "+u), nil, rq.Session, false)
		}},
	}, testOpts(func(o *Options) {
		o.Served = func(rq *Request) { served = append(served, rq.HTTP.Method+" "+rq.HTTP.URL.Path) }
	}))
	req := httptest.NewRequest("POST", "/_m/Auth/Login", strings.NewReader("a=1"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Datastar-Request", "true")
	w := serve(h, req)
	evs := events(w.Body.String())
	if w.Code != 200 || w.Header().Get("Location") != "" || w.Header().Get(StatusHeader) != "" || len(evs) != 3 ||
		evs[0] != bodyEvent("<p>Hello ada</p>") || evs[1] != titleEvent("Hello ada") {
		t.Fatalf("%d %q %q", w.Code, w.Header().Get("Location"), evs)
	}
	historyEvent(t, evs[2], pushOpen, `"/home?tab=1#top"`)
	if c := sessionCookie(t, w); c == nil || len(w.Result().Cookies()) != 1 {
		t.Fatalf("cookies %v", w.Result().Cookies())
	}
	if strings.Join(served, ",") != "POST /_m/Auth/Login,GET /home" {
		t.Fatalf("served %q", served)
	}
	// without Datastar: the 303
	w = serve(h, httptest.NewRequest("POST", "/_m/Auth/Login", nil))
	if w.Code != 303 || w.Header().Get("Location") != "/home?tab=1#top" || w.Body.Len() != 0 {
		t.Fatalf("plain %d %q", w.Code, w.Header().Get("Location"))
	}
}

// A redirect chain is followed (a boosted link to a page whose `requires` sends to the login
// page: the push names the login page; a back/forward's chain replaces); a target that fails
// is morphed in with its status; more than redirectLimit in a row is a 500 logged for the
// request the client sent, without a history step.
func TestBoost_RedirectChains(t *testing.T) {
	var logs []string
	h := NewRouter([]Route{
		{Method: "GET", Pattern: "/secret", Handle: func(rq *Request) Response { return RedirectResponse(rq.Login, nil) }},
		{Method: "GET", Pattern: "/login", Handle: func(rq *Request) Response { return PageResponse(doc("Sign in"), nil, nil, false) }},
		{Method: "GET", Pattern: "/loop/{n}", Handle: func(rq *Request) Response { return RedirectResponse("/loop/"+rq.Params["n"]+"x", nil) }},
		{Method: "POST", Pattern: "/_m/A/Gone", Handle: func(rq *Request) Response { return RedirectResponse("/nowhere", nil) }},
		{Method: "POST", Pattern: "/_m/A/Loop", Handle: func(rq *Request) Response { return RedirectResponse("/loop/x", nil) }},
	}, testOpts(func(o *Options) { o.Log = func(level, msg string) { logs = append(logs, level+": "+msg) } }))
	evs := events(serve(h, dsGet("/secret")).Body.String())
	if len(evs) != 3 || evs[0] != bodyEvent("<p>Sign in</p>") {
		t.Fatalf("requires %q", evs)
	}
	historyEvent(t, evs[2], pushOpen, `"/login"`)
	req := dsGet("/secret")
	req.Header.Set(datastar.HistoryHeader, "replace")
	evs = events(serve(h, req).Body.String())
	historyEvent(t, evs[2], replaceOpen, `"/login"`)

	req = httptest.NewRequest("POST", "/_m/A/Gone", nil)
	req.Header.Set("Datastar-Request", "true")
	w := serve(h, req)
	if evs := events(w.Body.String()); w.Header().Get(StatusHeader) != "404" || evs[0] != bodyEvent("<p>Not found</p>") {
		t.Fatalf("gone %q %q", w.Header().Get(StatusHeader), evs)
	} else {
		historyEvent(t, evs[2], pushOpen, `"/nowhere"`)
	}

	req = httptest.NewRequest("POST", "/_m/A/Loop", nil)
	req.Header.Set("Datastar-Request", "true")
	w = serve(h, req)
	evs = events(w.Body.String())
	last := "/loop/x" + strings.Repeat("x", redirectLimit+1)
	want := "error: POST /_m/A/Loop: more than 10 redirects in a row (the last to " + last + ") rid=" + w.Header().Get("X-Request-Id")
	if w.Header().Get(StatusHeader) != "500" || len(evs) != 1 || evs[0] != bodyEvent("<p>Something went wrong</p>") || logs[len(logs)-1] != want {
		t.Fatalf("loop %q %q %q", w.Header().Get(StatusHeader), evs, logs)
	}
}

// RF2: a redirect to another site (Url.external), or to "#" (an unsafe URL neutralised), stays
// a real navigation under Datastar; a protocol-relative one is refused (500), as before.
func TestBoost_ExternalRedirectNavigates(t *testing.T) {
	for _, loc := range []string{"https://example.com/x?a=1", "#", "//evil.example/x"} {
		h := NewRouter(one("POST", "/_m/A/Go", func(rq *Request) Response { return RedirectResponse(loc, nil) }), testOpts(nil))
		req := httptest.NewRequest("POST", "/_m/A/Go", nil)
		req.Header.Set("Datastar-Request", "true")
		w := serve(h, req)
		body := w.Body.String()
		if strings.Contains(body, "pushState") || (loc != "//evil.example/x" && !strings.Contains(body, "window.location.assign(")) {
			t.Fatalf("%s: %q", loc, body)
		}
	}
}

func TestBoost_Followable(t *testing.T) {
	rt := NewRouter(nil, testOpts(nil)).(*router)
	for loc, want := range map[string]bool{
		"/": true, "/posts/1?x=y#c": true, "/_m/A/B": true,
		"#": false, "": false, "posts": false, "//evil.com/x": false, `/\evil.com`: false, "https://ex.com/": false,
		"/static/app.css": false, "/static/al/al.css": false, datastar.ScriptPath: false, datastar.ScriptPath + "?v": false,
	} {
		if got := rt.followable(loc); got != want {
			t.Errorf("followable(%q) = %v", loc, got)
		}
	}
}

func TestBoost_DropSignals(t *testing.T) {
	for in, want := range map[string]string{
		"":                        "",
		"datastar=%7B%7D":         "",
		"b=2&datastar=%7B%7D&a=1": "b=2&a=1",
		"q=a%20b&datastar=%7B%7D": "q=a%20b",
		"datastar=x&a=1":          "datastar=x&a=1", // not the boost's: the page's own
		"datastar=&a=1":           "datastar=&a=1",
		"datastarx=1&x=datastar":  "datastarx=1&x=datastar",
	} {
		u := &url.URL{Path: "/", RawQuery: in}
		if dropSignals(u); u.RawQuery != want {
			t.Errorf("dropSignals(%q) = %q, want %q", in, u.RawQuery, want)
		}
	}
}

func TestBoost_TitleOf(t *testing.T) {
	for page, want := range map[string]string{
		`<html><head><title>A &amp; B</title></head><body></body></html>`:       "A &amp; B",
		`<html><head></head><body><svg><title>icon</title></svg></body></html>`: "",
		`<html><head><title lang="en">T</title></head><body></body></html>`:     "T",
		`<p>bare</p>`: "",
	} {
		got, ok := titleOf(page)
		if got != want || ok != (want != "") {
			t.Errorf("titleOf(%s) = %q %v", page, got, ok)
		}
	}
}

// RF2, the click's exceptions: the boost's handler is Datastar attribute expressions
// (datastar.BoostTag) rather than Go, so the test reads them: each exclusion is in the click
// handler, exactly; the GET carries no signals; popstate refetches with the replace header only
// when the path or query changed. (Their behaviour in a real browser is M7b's smoke suite.)
func TestBoost_ClickHandlerExclusions(t *testing.T) {
	for _, part := range []string{
		"evt.defaultPrevented",
		"evt.button !== 0",
		"evt.metaKey || evt.ctrlKey || evt.shiftKey || evt.altKey",
		"closest('a[href]')",
		"a.hasAttribute('target')",
		"a.hasAttribute('download')",
		"a.hasAttribute('data-alder-reload')",
		"u.origin !== location.origin",
		"u.protocol !== 'http:' && u.protocol !== 'https:'", // mailto:, tel:, javascript:
		"if (u.hash && u.pathname === location.pathname && u.search === location.search) return;", // a jump on this page
		"window._alderHash = u.hash;", // another page's fragment, added back by the push
		"if (u.pathname.startsWith('/static/') || u.pathname.startsWith('/_alder/')) return;", // a file
		"evt.preventDefault(); window._alderHash = u.hash; history.scrollRestoration = 'manual'; history.replaceState({alderY: scrollY}, ''); window._alderAC?.abort(); (window._alderBusy||((e,p)=>p))(null, @get(u.pathname + u.search, {payload: {}, headers: {'Alder-Build': window._alderBuild}, requestCancellation: window._alderAC = new AbortController()}))",
	} {
		if !strings.Contains(datastar.BoostClick, part) {
			t.Errorf("BoostClick lacks %q", part)
		}
	}
	// the exclusions all return before the default is prevented
	if strings.Index(datastar.BoostClick, "evt.preventDefault()") < strings.LastIndex(datastar.BoostClick, "return;") {
		t.Error("BoostClick prevents the default before its last exclusion")
	}
	if want := "if (location.pathname + location.search !== window._alderAt) {window._alderAC?.abort(); (window._alderBusy||((e,p)=>p))(null, @get(location.pathname + location.search, {payload: {}, headers: {'Alder-History': 'replace', 'Alder-Build': window._alderBuild}, requestCancellation: window._alderAC = new AbortController()}))}"; datastar.BoostPopstate != want {
		t.Errorf("BoostPopstate = %q", datastar.BoostPopstate)
	}
	// Datastar rewrites $name as a signal: the expressions must not use one.
	for _, e := range []string{datastar.BoostClick, datastar.BoostPopstate, datastar.BoostInit} {
		if strings.Contains(e, "$") {
			t.Errorf("expression uses $: %q", e)
		}
	}
	// on window, so the listeners survive body patches; in <head>, which is never morphed
	tag := datastar.BoostTag()
	// written by hand (m7): the expressions' single quotes are not escaped, & and " are
	if strings.Contains(tag, "&#39;") || !strings.Contains(tag, "'a[href]'") || !strings.Contains(tag, "&amp;&amp;") {
		t.Errorf("BoostTag escaping: %s", tag)
	}
	// the scroll position is kept in the current entry's state as the page scrolls, so Back and
	// Forward both restore it
	if want := "history.replaceState(Object.assign({}, history.state, {alderY: scrollY}), '')"; datastar.BoostScroll != want {
		t.Errorf("BoostScroll = %q", datastar.BoostScroll)
	}
	for _, attr := range []string{`data-on:click__window="`, `data-on:popstate__window="`, `data-init="`, `data-on:scroll__window__throttle.200ms.trailing="`} {
		if !strings.Contains(tag, attr) {
			t.Errorf("BoostTag lacks %s: %s", attr, tag)
		}
	}
	if _, boost := HeadTags(); boost != tag {
		t.Error("HeadTags lacks the boost")
	}
}

// The flash (M13): every page served whole begins its <head> with the color-scheme meta, before
// the Datastar script, so the first frame is painted in the OS's scheme; the boost goes last,
// before </head>, so a layout's <meta charset> stays in the first 1024 bytes.
func TestBoost_ColorSchemeMeta(t *testing.T) {
	h := NewRouter(one("GET", "/", func(rq *Request) Response { return PageResponse(doc("Home"), nil, nil, false) }), testOpts(nil))
	body := serve(h, httptest.NewRequest("GET", "/", nil)).Body.String()
	want := "<!doctype html>\n<html><head>" + `<meta name="color-scheme" content="light dark" />` + `<meta name="alder-build" content="` + BuildID() + `" />` + pendingStyle + html.Render(datastar.ScriptTag()) + "<title>Home</title>" + datastar.BoostTag() + "</head>"
	if !strings.HasPrefix(body, want) {
		t.Fatalf("head %q", body)
	}
}

// I2: a file under /static/ or /_alder/ is not a page: a Datastar GET of one (the boost leaves
// such links to the browser; an older boost did not) is answered with a real navigation to it.
func TestBoost_FileLinkNavigates(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "x.pdf"), []byte("%PDF"), 0o644); err != nil {
		t.Fatal(err)
	}
	opts := testOpts(nil)
	opts.StaticDir = dir
	h := NewRouter(one("GET", "/", func(rq *Request) Response { return PageResponse(doc("Home"), nil, nil, false) }), opts)
	for _, path := range []string{"/static/x.pdf", "/static/al/al.css", datastar.ScriptPath} {
		r := httptest.NewRequest("GET", path+"?datastar=%7B%7D", nil)
		r.Header.Set("Datastar-Request", "true")
		w := serve(h, r)
		if got := w.Body.String(); !strings.Contains(got, "window.location.assign(\""+path+"\")") {
			t.Errorf("%s: %q", path, got)
		}
	}
	// without Datastar the file itself
	if w := serve(h, httptest.NewRequest("GET", "/static/x.pdf", nil)); w.Body.String() != "%PDF" {
		t.Errorf("plain GET: %q", w.Body.String())
	}
}

// I8: a boosted GET from a page of another build (its Alder-Build header) is answered with a
// real navigation to the URL, so the tab reloads once and gets this build's <head>; the same
// build, or no header (a hand-made request), is the usual morph.
func TestBoost_BuildMismatchReloads(t *testing.T) {
	h := NewRouter(one("GET", "/p", func(rq *Request) Response { return PageResponse(doc("P"), nil, nil, false) }), testOpts(nil))
	get := func(build string) string {
		r := httptest.NewRequest("GET", "/p?a=1&datastar=%7B%7D", nil)
		r.Header.Set("Datastar-Request", "true")
		if build != "" {
			r.Header.Set(datastar.BuildHeader, build)
		}
		return serve(h, r).Body.String()
	}
	if got := get("0123456789abcdef-old"); !strings.Contains(got, "window.location.assign(\"/p?a=1\")") || strings.Contains(got, "selector body\ndata: mode outer") {
		t.Errorf("old build: %q", got)
	}
	for _, b := range []string{BuildID(), ""} {
		if got := get(b); !strings.Contains(got, "data: selector body\ndata: mode outer") || !strings.Contains(got, "history.pushState") {
			t.Errorf("build %q: %q", b, got)
		}
	}
	if id := BuildID(); len(id) != 16 || id != BuildID() {
		t.Errorf("BuildID %q", id)
	}
	if !strings.Contains(BuildMeta(), `<meta name="alder-build" content="`+BuildID()+`" />`) {
		t.Errorf("BuildMeta %q", BuildMeta())
	}
}
