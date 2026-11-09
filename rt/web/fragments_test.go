package web

import (
	"net/http/httptest"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"alder/rt"
	"alder/rt/datastar"
	"alder/rt/html"
)

// frApp is a page of rows, each `<li id="row-N">`, and a form `<form id="add">` with a title,
// with write routes answering `refresh` (or `invalid` for a form that did not decode) as the
// generated write route does: the page the browser is on rendered before the update
// (RenderShown), the router diffing the after-view against it.
type frApp struct {
	done    []bool
	heading string
	toasts  bool   // the page has an <al-toasts> region of its own
	class   string // the <html> element's class, none when empty

	noBefore    bool // the write renders no before-view
	panicBefore bool // the next GET / (the before-view) panics
}

const frForm = "R.New"

// frAdd is the route the form #add sends.
var frAdd = rt.Route{Name: "R.Add", Method: "POST", Path: "/r/add"}

func (a *frApp) view(done []bool, heading string) html.Node {
	var items []html.Node
	for i, d := range done {
		class := "open"
		if d {
			class = "done"
		}
		items = append(items, html.El("li", []html.Attr{html.A("id", "row-"+strconv.Itoa(i+1)), html.A("class", class)},
			html.Text("Row "+strconv.Itoa(i+1))))
	}
	body := []html.Node{
		html.El("h1", nil, html.Text(heading)),
		html.El("ul", []html.Attr{html.A("id", "rows")}, items...),
		html.FormToRoute(frAdd, []html.Attr{html.A("id", "add")},
			html.Input(html.Control{Kind: "errors", Form: frForm}),
			html.Input(html.Control{Kind: "input", Form: frForm, Name: "title", Label: "Title"}),
			html.Input(html.Control{Kind: "errors", Form: frForm, Name: "title"})),
	}
	if a.toasts {
		body = append(body, html.El("al-toasts", []html.Attr{html.A("popover", "manual"), html.B("data-ignore-morph", true)}))
	}
	var root []html.Attr
	if a.class != "" {
		root = []html.Attr{html.A("class", a.class)}
	}
	return html.El("html", root, html.El("head", nil, html.El("title", nil, html.Text("R"))), html.El("body", nil, body...))
}

// before is the page the browser is on, rendered before the write's update (nil with noBefore).
func (a *frApp) before(rq *Request) *Shown {
	if a.noBefore {
		return nil
	}
	sh, _ := rq.RenderShown(nil)
	return sh
}

// refresh is the write's `refresh` answer, with an info toast when the request asks for one.
func refresh(rq *Request, before *Shown) Response {
	ans := rt.Refresh()
	if rq.Query.Get("notice") != "" {
		ans = rt.WithToast(rt.NoticeInfo, "Saved", ans)
	}
	return AnswerResponse(ans, before, nil)
}

func (a *frApp) routes() []Route {
	return []Route{
		{Method: "GET", Pattern: "/", Handle: func(rq *Request) Response {
			if a.panicBefore {
				a.panicBefore = false
				panic("before")
			}
			return PageResponse(a.view(a.done, a.heading), rq.FormState, nil)
		}},
		{Method: "POST", Pattern: "/r/toggle", Handle: func(rq *Request) Response {
			before := a.before(rq)
			i, _ := strconv.Atoi(rq.Query.Get("id"))
			a.done[i-1] = !a.done[i-1]
			return refresh(rq, before)
		}},
		{Method: "POST", Pattern: "/r/same", Handle: func(rq *Request) Response {
			return refresh(rq, a.before(rq))
		}},
		{Method: "POST", Pattern: "/r/heading", Handle: func(rq *Request) Response {
			before := a.before(rq)
			a.heading = rq.Query.Get("to")
			return refresh(rq, before)
		}},
		{Method: "POST", Pattern: "/r/theme", Handle: func(rq *Request) Response {
			before := a.before(rq)
			a.class = rq.Query.Get("to")
			return refresh(rq, before)
		}},
		{Method: "POST", Pattern: "/r/add", Handle: func(rq *Request) Response {
			before := a.before(rq)
			if rq.Form.Get("title") == "" { // the form does not decode: the same model, with the errors
				resp := InvalidResponse(&html.FormState{Form: frForm, Msg: rt.RouteKey(frAdd), Values: map[string]string{"title": ""},
					Errors: map[string][]string{"title": {"must not be empty"}}}, nil)
				resp.Shown = before
				return resp
			}
			a.done = append(a.done, false)
			return refresh(rq, before)
		}},
	}
}

var shownRe = regexp.MustCompile(`<al-shown id="_alder_shown" data-h="([0-9a-f]{16})" hidden></al-shown>`)

// shownOf is the hash of the last al-shown in an answer, "" for none.
func shownOf(body string) string {
	m := shownRe.FindAllStringSubmatch(body, -1)
	if len(m) == 0 {
		return ""
	}
	return m[len(m)-1][1]
}

// post is a Datastar post of target (with form fields, when given) carrying shown.
func post(t *testing.T, a *frApp, opts Options, target string, fields url.Values, shown string) *httptest.ResponseRecorder {
	t.Helper()
	h := NewRouter(a.routes(), opts)
	req := httptest.NewRequest("POST", target, strings.NewReader(fields.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Datastar-Request", "true")
	if shown != "" {
		req.Header.Set(html.ShownHeader, shown)
	}
	return serveRaw(h, req)
}

// shownNow is the hash the document the app serves now carries.
func shownNow(t *testing.T, a *frApp, opts Options) string {
	t.Helper()
	w := serveRaw(NewRouter(a.routes(), opts), httptest.NewRequest("GET", "/", nil))
	h := shownOf(w.Body.String())
	if h == "" {
		t.Fatalf("no al-shown in %s", w.Body.String())
	}
	return h
}

func newFrApp(n int) *frApp { return &frApp{done: make([]bool, n), heading: "Rows"} }

func regionEvent(id, elements string) string {
	return "event: datastar-patch-elements\ndata: selector #" + id + "\ndata: mode outer\ndata: elements " + elements
}

// isBodyMorph reports whether the answer is one body patch ending with the al-shown of the body.
func isBodyMorph(t *testing.T, w *httptest.ResponseRecorder) bool {
	t.Helper()
	evs := events(w.Body.String())
	return len(evs) >= 1 && strings.HasPrefix(evs[0], "event: datastar-patch-elements\ndata: selector body\ndata: mode outer\n") &&
		strings.Contains(evs[0], `<al-shown id="_alder_shown"`)
}

// TestDocument_CarriesShown: a document and a body morph end their body with the al-shown of
// the body's hash, as the view rendered it.
func TestDocument_CarriesShown(t *testing.T) {
	a := newFrApp(3)
	w := serveRaw(NewRouter(a.routes(), testOpts(nil)), httptest.NewRequest("GET", "/", nil))
	body := html.RenderWith(a.view(a.done, a.heading), &html.FormState{})
	want := html.Hash(bodyOf(body))
	if got := w.Body.String(); !strings.HasSuffix(got, `<al-shown id="_alder_shown" data-h="`+want+`" hidden></al-shown></body></html>`) {
		t.Fatalf("document %s, want hash %s", got, want)
	}
	// a boosted GET's morph
	w = serveRaw(NewRouter(a.routes(), testOpts(nil)), dsGet("/"))
	if evs := events(w.Body.String()); !strings.HasSuffix(evs[0], `<al-shown id="_alder_shown" data-h="`+want+`" hidden></al-shown></body>`) {
		t.Fatalf("morph %q", evs[0])
	}
	// a re-render without a shown hash
	w = post(t, a, testOpts(nil), "/r/same", nil, "")
	if !isBodyMorph(t, w) || shownOf(w.Body.String()) != want {
		t.Fatalf("rerender %q", w.Body.String())
	}
}

// One row toggled: one #row patch of its after-markup, then the shown hash of the new body.
func TestRerender_PatchesChangedRegion(t *testing.T) {
	a := newFrApp(10)
	shown := shownNow(t, a, testOpts(nil))
	w := post(t, a, testOpts(nil), "/r/toggle?id=7", nil, shown)
	evs := events(w.Body.String())
	newHash := html.Hash(bodyOf(html.RenderWith(a.view(a.done, a.heading), &html.FormState{})))
	want := []string{regionEvent("row-7", `<li id="row-7" class="done">Row 7</li>`), regionEvent("_alder_shown", `<al-shown id="_alder_shown" data-h="`+newHash+`" hidden></al-shown>`)}
	if strings.Join(evs, "\n\n") != strings.Join(want, "\n\n") {
		t.Fatalf("events %q\nwant %q", evs, want)
	}
	if w.Header().Get("Content-Type") != "text/event-stream" || w.Code != 200 {
		t.Fatalf("%d %q", w.Code, w.Header().Get("Content-Type"))
	}
	// the next click starts from the new hash
	w = post(t, a, testOpts(nil), "/r/toggle?id=2", nil, newHash)
	if evs := events(w.Body.String()); len(evs) != 2 || evs[0] != regionEvent("row-2", `<li id="row-2" class="done">Row 2</li>`) {
		t.Fatalf("second click %q", evs)
	}
	// a row added changes #rows' skeleton: the list is patched, not the body
	w = post(t, a, testOpts(nil), "/r/add", url.Values{"title": {"x"}}, shownNow(t, a, testOpts(nil)))
	if evs := events(w.Body.String()); len(evs) != 2 || !strings.HasPrefix(evs[0], "event: datastar-patch-elements\ndata: selector #rows\n") || !strings.Contains(evs[0], `<li id="row-11" class="open">`) {
		t.Fatalf("add %q", evs)
	}
}

// Text outside every region changed: the body's skeleton differs, so the body is morphed.
func TestRerender_SkeletonChangeMorphsBody(t *testing.T) {
	a := newFrApp(3)
	w := post(t, a, testOpts(nil), "/r/heading?to=Other", nil, shownNow(t, a, testOpts(nil)))
	if !isBodyMorph(t, w) || len(events(w.Body.String())) != 1 || !strings.Contains(w.Body.String(), "<h1>Other</h1>") {
		t.Fatalf("%q", w.Body.String())
	}
}

// Without the header (a post made before the boost ran) the body is morphed.
func TestRerender_NoShownMorphsBody(t *testing.T) {
	a := newFrApp(3)
	w := post(t, a, testOpts(nil), "/r/toggle?id=1", nil, "")
	if !isBodyMorph(t, w) || len(events(w.Body.String())) != 1 {
		t.Fatalf("%q", w.Body.String())
	}
}

// A stale hash (the client's al-shown after an element patch) no longer forces a body morph
// (#160): the regions the server changed are patched, the others left as the browser has them.
func TestRerender_StaleShownPatchesRegions(t *testing.T) {
	a := newFrApp(3)
	w := post(t, a, testOpts(nil), "/r/toggle?id=1", nil, "0123456789abcdef")
	evs := events(w.Body.String())
	if len(evs) != 2 || evs[0] != regionEvent("row-1", `<li id="row-1" class="done">Row 1</li>`) || !strings.Contains(evs[1], "selector #_alder_shown") {
		t.Fatalf("%q", evs)
	}
}

// A write that renders no before-view is morphed whole.
func TestRerender_NoBeforeMorphsBody(t *testing.T) {
	a := newFrApp(3)
	shown := shownNow(t, a, testOpts(nil))
	a.noBefore = true
	if w := post(t, a, testOpts(nil), "/r/toggle?id=1", nil, shown); !isBodyMorph(t, w) {
		t.Fatalf("%q", w.Body.String())
	}
}

// Nothing changed: no element event at all, not even the shown hash's.
func TestRerender_NoChangeSendsNoElements(t *testing.T) {
	a := newFrApp(3)
	w := post(t, a, testOpts(nil), "/r/same", nil, shownNow(t, a, testOpts(nil)))
	if w.Body.String() != "" || w.Code != 200 || w.Header().Get("Content-Type") != "text/event-stream" {
		t.Fatalf("%d %q %q", w.Code, w.Header().Get("Content-Type"), w.Body.String())
	}
}

// A before-view that panics is logged and the body is morphed.
func TestRerender_BeforePanicMorphsBody(t *testing.T) {
	a := newFrApp(3)
	shown := shownNow(t, a, testOpts(nil))
	a.panicBefore = true
	var logged []string
	opts := testOpts(func(o *Options) { o.Log = func(level, msg string) { logged = append(logged, level+" "+msg) } })
	w := post(t, a, opts, "/r/toggle?id=1", nil, shown)
	if !isBodyMorph(t, w) || w.Header().Get(StatusHeader) != "" || !strings.Contains(w.Body.String(), `<li id="row-1" class="done">`) {
		t.Fatalf("%q", w.Body.String())
	}
	if len(logged) != 1 || !strings.Contains(logged[0], "error GET /: rendering the page before the write: panic: before") {
		t.Fatalf("logged %q", logged)
	}
}

// A toast needs the <al-toasts> region: a page without its own is morphed whole (the
// implicit region with it); a page with one gets its patches and then the toast.
func TestRerender_NoticeWithoutToastsMorphsBody(t *testing.T) {
	a := newFrApp(3)
	w := post(t, a, testOpts(nil), "/r/toggle?id=1&notice=1", nil, shownNow(t, a, testOpts(nil)))
	evs := events(w.Body.String())
	if !isBodyMorph(t, w) || !strings.Contains(evs[0], `<al-toasts popover="manual" data-ignore-morph></al-toasts>`) || len(evs) != 2 ||
		!strings.Contains(evs[1], "data: selector al-toasts\ndata: mode append") {
		t.Fatalf("%q", evs)
	}
	a = newFrApp(3)
	a.toasts = true
	w = post(t, a, testOpts(nil), "/r/toggle?id=1&notice=1", nil, shownNow(t, a, testOpts(nil)))
	evs = events(w.Body.String())
	if len(evs) != 3 || evs[0] != regionEvent("row-1", `<li id="row-1" class="done">Row 1</li>`) || !strings.Contains(evs[2], "data: selector al-toasts\ndata: mode append") {
		t.Fatalf("%q", evs)
	}
}

// A form that does not decode re-renders with its errors: one patch of the form (it has an id).
// The next click patches only what it changed (#160): the form, which the server renders plain
// before and after, is left as the browser has it.
func TestRerender_RejectedFormPatchesItsRegion(t *testing.T) {
	a := newFrApp(3)
	w := post(t, a, testOpts(nil), "/r/add", url.Values{"title": {""}}, shownNow(t, a, testOpts(nil)))
	evs := events(w.Body.String())
	if len(evs) != 2 || !strings.HasPrefix(evs[0], "event: datastar-patch-elements\ndata: selector #add\ndata: mode outer\n") || !strings.Contains(evs[0], "must not be empty") {
		t.Fatalf("%q", evs)
	}
	shown := shownOf(evs[1])
	w = post(t, a, testOpts(nil), "/r/toggle?id=1", nil, shown)
	evs = events(w.Body.String())
	if len(evs) != 2 || evs[0] != regionEvent("row-1", `<li id="row-1" class="done">Row 1</li>`) || strings.Contains(w.Body.String(), "#add") {
		t.Fatalf("%q", evs)
	}
}

// Options.Rendered gets the full render's body on every re-render, patched or morphed.
func TestRerender_CallsRendered(t *testing.T) {
	a := newFrApp(3)
	var got []string
	opts := testOpts(func(o *Options) { o.Rendered = func(rq *Request, body string) { got = append(got, body) } })
	shown := shownNow(t, a, opts)
	post(t, a, opts, "/r/toggle?id=1", nil, shown)
	post(t, a, opts, "/r/toggle?id=1", nil, "")
	full := bodyOf(html.RenderWith(a.view(a.done, a.heading), &html.FormState{}))
	if len(got) != 2 || got[1] != full || !strings.Contains(got[0], `<li id="row-1" class="done">`) || strings.Contains(got[0], "al-shown") {
		t.Fatalf("%q", got)
	}
}

// The panel's Patches tab lists the regions sent, the shown hash, or none.
func TestPanel_ListsRegionPatches(t *testing.T) {
	t.Setenv("ALDER_DEV", "1")
	a := newFrApp(3)
	opts := testOpts(func(o *Options) { o.Dev = true })
	w := post(t, a, opts, "/r/toggle?id=2", nil, shownNow(t, a, opts))
	if body := w.Body.String(); !strings.Contains(body, "<h3>Patches</h3><ul><li>#row-2</li><li>shown</li><li>panel</li></ul>") {
		t.Fatalf("%s", body)
	}
	w = post(t, a, opts, "/r/same", nil, shownNow(t, a, opts))
	if body := w.Body.String(); !strings.Contains(body, "<h3>Patches</h3><ul><li>none</li><li>panel</li></ul>") {
		t.Fatalf("%s", body)
	}
}

// A region around markup the browser's parser restructures (a <div> in a <p>) is not patched:
// here the only region is one, so the body is morphed.
func TestRerender_RepairedMarkupMorphsBody(t *testing.T) {
	n := 1
	pageOf := func(n int) html.Node {
		return html.El("html", nil, html.El("body", nil,
			html.El("p", []html.Attr{html.A("id", "msg")}, html.Text(strconv.Itoa(n)), html.El("div", nil, html.Text("x")))))
	}
	routes := []Route{
		{Method: "GET", Pattern: "/", Handle: func(rq *Request) Response { return PageResponse(pageOf(n), &html.FormState{}, nil) }},
		{Method: "POST", Pattern: "/r/inc", Handle: func(rq *Request) Response {
			before, _ := rq.RenderShown(nil)
			n++
			return AnswerResponse(rt.Refresh(), before, nil)
		}},
	}
	h := NewRouter(routes, testOpts(nil))
	shown := shownOf(serveRaw(h, httptest.NewRequest("GET", "/", nil)).Body.String())
	req := httptest.NewRequest("POST", "/r/inc", nil)
	req.Header.Set("Datastar-Request", "true")
	req.Header.Set(html.ShownHeader, shown)
	w := serveRaw(h, req)
	if !isBodyMorph(t, w) || !strings.Contains(w.Body.String(), `<p id="msg">2<div>x</div></p>`) {
		t.Fatalf("%q", w.Body.String())
	}
}

// The before-render panic's log line masks a Sensitive path parameter of the page the browser
// is on and carries the request id.
func TestRerender_BeforePanicLineIsMasked(t *testing.T) {
	a := newFrApp(3)
	routes := append(a.routes(), Route{Method: "GET", Pattern: "/reset/{token}", Handle: func(rq *Request) Response { panic("before") }},
		Route{Method: "POST", Pattern: "/r/reset", Handle: func(rq *Request) Response {
			before, _ := rq.RenderShown(nil)
			return AnswerResponse(rt.Refresh(), before, nil)
		}})
	var logged []string
	opts := testOpts(func(o *Options) { o.Log = func(level, msg string) { logged = append(logged, level+" "+msg) } })
	req := httptest.NewRequest("POST", "/r/reset", nil)
	req.Header.Set("Datastar-Request", "true")
	req.Header.Set(html.PageHeader, "/reset/hunter2secret")
	req.Header.Set(html.ShownHeader, shownNow(t, a, testOpts(nil)))
	serveRaw(NewRouter(routes, opts), req)
	if len(logged) < 1 || strings.Contains(strings.Join(logged, "\n"), "hunter2secret") || !strings.Contains(logged[0], "/reset/••••: rendering the page before the write: panic: before") || !strings.Contains(logged[0], " rid=") {
		t.Fatalf("logged %q", logged)
	}
}

// htmlAttrsEvent is the event that sets the <html> element's attributes to the JSON attrs.
func htmlAttrsEvent(t *testing.T, ev, attrs string) {
	t.Helper()
	prefix := "event: datastar-patch-elements\ndata: selector body\ndata: mode append\ndata: elements <script data-effect=\"el.remove()\">"
	if !strings.HasPrefix(ev, prefix) || !strings.Contains(ev, "var a="+attrs+";") {
		t.Fatalf("attrs event %q, want attrs %s", ev, attrs)
	}
}

// Only the <html> element's class changed (#189): the body is the same, so no region is
// patched, but its attributes are set; the Patches tab names the patch html, not none.
func TestRerender_HtmlAttrsOnly(t *testing.T) {
	t.Setenv("ALDER_DEV", "1")
	a := newFrApp(3)
	opts := testOpts(func(o *Options) { o.Dev = true })
	w := post(t, a, opts, "/r/theme?to=dark", nil, shownNow(t, a, opts))
	body := w.Body.String()
	var evs []string
	for _, ev := range events(body) {
		if !strings.Contains(ev, "data: selector #"+datastar.PanelID+"\n") {
			evs = append(evs, ev)
		}
	}
	if len(evs) != 1 || w.Header().Get("Content-Type") != "text/event-stream" {
		t.Fatalf("%q", evs)
	}
	htmlAttrsEvent(t, evs[0], `[["class","dark"]]`)
	tab := body[strings.Index(body, "_alder_tab_patches"):]
	if !strings.Contains(tab, "<li>html</li>") || strings.Contains(tab, "<li>none</li>") {
		t.Fatalf("patches tab %s", tab)
	}
	// back to no class: the attribute is removed
	w = post(t, a, testOpts(nil), "/r/theme?to=", nil, shownNow(t, a, testOpts(nil)))
	if evs := events(w.Body.String()); len(evs) != 1 {
		t.Fatalf("%q", evs)
	} else {
		htmlAttrsEvent(t, evs[0], `[]`)
	}
}

// The <html> element's attributes are the same: no attrs event.
func TestRerender_HtmlAttrsUnchanged(t *testing.T) {
	a := newFrApp(3)
	a.class = "dark"
	w := post(t, a, testOpts(nil), "/r/theme?to=dark", nil, shownNow(t, a, testOpts(nil)))
	if w.Body.String() != "" {
		t.Fatalf("same class %q", w.Body.String())
	}
	w = post(t, a, testOpts(nil), "/r/toggle?id=1", nil, shownNow(t, a, testOpts(nil)))
	if evs := events(w.Body.String()); len(evs) != 2 || strings.Contains(w.Body.String(), "var a=") {
		t.Fatalf("toggle %q", evs)
	}
}
