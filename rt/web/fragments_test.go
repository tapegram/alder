package web

import (
	"net/http/httptest"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"alder/rt"
	"alder/rt/html"
)

// frApp is a page of rows, each `<li id="row-N">`, and a form `<form id="add">` with a title,
// answering its messages as the generated update route does (Before: the view of the model
// before update, or of the same model for a form that did not decode).
type frApp struct {
	done    []bool
	heading string
	toasts  bool // the page has an <al-toasts> region of its own

	noBefore, panicBefore bool
}

const frForm = "R.New"

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
		html.FormTo(rt.MsgRef{Module: "R", Msg: "Add"}, []html.Attr{html.A("id", "add")},
			html.Input(html.Control{Kind: "errors", Form: frForm}),
			html.Input(html.Control{Kind: "input", Form: frForm, Name: "title", Label: "Title"}),
			html.Input(html.Control{Kind: "errors", Form: frForm, Name: "title"})),
	}
	if a.toasts {
		body = append(body, html.El("al-toasts", []html.Attr{html.A("popover", "manual"), html.B("data-ignore-morph", true)}))
	}
	return html.El("html", nil, html.El("head", nil, html.El("title", nil, html.Text("R"))), html.El("body", nil, body...))
}

func (a *frApp) answer(rq *Request, st *html.FormState, before func() html.Node) Response {
	resp := PageResponse(a.view(a.done, a.heading), st, nil, true)
	switch {
	case a.panicBefore:
		resp.Before = func() html.Node { panic("before") }
	case !a.noBefore:
		resp.Before = before
	}
	return resp
}

func (a *frApp) routes() []Route {
	snapshot := func() func() html.Node {
		done, heading := append([]bool(nil), a.done...), a.heading
		return func() html.Node { return a.view(done, heading) }
	}
	return []Route{
		{Method: "GET", Pattern: "/", Handle: func(rq *Request) Response {
			return PageResponse(a.view(a.done, a.heading), &html.FormState{}, nil, false)
		}},
		{Method: "POST", Pattern: "/_m/R/Toggle", Handle: func(rq *Request) Response {
			before := snapshot()
			i, _ := strconv.Atoi(rq.Query.Get("id"))
			a.done[i-1] = !a.done[i-1]
			if rq.Query.Get("notice") != "" {
				rq.Notice = "Saved"
			}
			return a.answer(rq, &html.FormState{}, before)
		}},
		{Method: "POST", Pattern: "/_m/R/Same", Handle: func(rq *Request) Response {
			return a.answer(rq, &html.FormState{}, snapshot())
		}},
		{Method: "POST", Pattern: "/_m/R/Heading", Handle: func(rq *Request) Response {
			before := snapshot()
			a.heading = rq.Query.Get("to")
			return a.answer(rq, &html.FormState{}, before)
		}},
		{Method: "POST", Pattern: "/_m/R/Add", Handle: func(rq *Request) Response {
			before := snapshot()
			title := rq.Form.Get("title")
			if title == "" { // the form does not decode: the same model, with the errors
				return a.answer(rq, &html.FormState{Form: frForm, Msg: rt.MsgKey("/_m/R/Add", rq.Query), Values: map[string]string{"title": ""},
					Errors: map[string][]string{"title": {"must not be empty"}}}, before)
			}
			a.done = append(a.done, false)
			return a.answer(rq, &html.FormState{}, before)
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
	w = post(t, a, testOpts(nil), "/_m/R/Same", nil, "")
	if !isBodyMorph(t, w) || shownOf(w.Body.String()) != want {
		t.Fatalf("rerender %q", w.Body.String())
	}
}

// One row toggled: one #row patch of its after-markup, then the shown hash of the new body.
func TestRerender_PatchesChangedRegion(t *testing.T) {
	a := newFrApp(10)
	shown := shownNow(t, a, testOpts(nil))
	w := post(t, a, testOpts(nil), "/_m/R/Toggle?id=7", nil, shown)
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
	w = post(t, a, testOpts(nil), "/_m/R/Toggle?id=2", nil, newHash)
	if evs := events(w.Body.String()); len(evs) != 2 || evs[0] != regionEvent("row-2", `<li id="row-2" class="done">Row 2</li>`) {
		t.Fatalf("second click %q", evs)
	}
	// a row added changes #rows' skeleton: the list is patched, not the body
	w = post(t, a, testOpts(nil), "/_m/R/Add", url.Values{"title": {"x"}}, shownNow(t, a, testOpts(nil)))
	if evs := events(w.Body.String()); len(evs) != 2 || !strings.HasPrefix(evs[0], "event: datastar-patch-elements\ndata: selector #rows\n") || !strings.Contains(evs[0], `<li id="row-11" class="open">`) {
		t.Fatalf("add %q", evs)
	}
}

// Text outside every region changed: the body's skeleton differs, so the body is morphed.
func TestRerender_SkeletonChangeMorphsBody(t *testing.T) {
	a := newFrApp(3)
	w := post(t, a, testOpts(nil), "/_m/R/Heading?to=Other", nil, shownNow(t, a, testOpts(nil)))
	if !isBodyMorph(t, w) || len(events(w.Body.String())) != 1 || !strings.Contains(w.Body.String(), "<h1>Other</h1>") {
		t.Fatalf("%q", w.Body.String())
	}
}

// Without the header, or with a hash of another page, the body is morphed.
func TestRerender_ShownMismatchMorphsBody(t *testing.T) {
	for _, shown := range []string{"", "0123456789abcdef"} {
		a := newFrApp(3)
		w := post(t, a, testOpts(nil), "/_m/R/Toggle?id=1", nil, shown)
		if !isBodyMorph(t, w) || len(events(w.Body.String())) != 1 {
			t.Fatalf("shown %q: %q", shown, w.Body.String())
		}
	}
}

// A route that sets no Before (not a message's re-render) is morphed whole.
func TestRerender_NoBeforeMorphsBody(t *testing.T) {
	a := newFrApp(3)
	shown := shownNow(t, a, testOpts(nil))
	a.noBefore = true
	if w := post(t, a, testOpts(nil), "/_m/R/Toggle?id=1", nil, shown); !isBodyMorph(t, w) {
		t.Fatalf("%q", w.Body.String())
	}
}

// Nothing changed: no element event at all, not even the shown hash's.
func TestRerender_NoChangeSendsNoElements(t *testing.T) {
	a := newFrApp(3)
	w := post(t, a, testOpts(nil), "/_m/R/Same", nil, shownNow(t, a, testOpts(nil)))
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
	w := post(t, a, opts, "/_m/R/Toggle?id=1", nil, shown)
	if !isBodyMorph(t, w) || w.Header().Get(StatusHeader) != "" || !strings.Contains(w.Body.String(), `<li id="row-1" class="done">`) {
		t.Fatalf("%q", w.Body.String())
	}
	if len(logged) != 1 || !strings.Contains(logged[0], "error POST /_m/R/Toggle?id=1: rendering the page before the message: before") {
		t.Fatalf("logged %q", logged)
	}
}

// A notice's toast needs the <al-toasts> region: a page without its own is morphed whole (the
// implicit region with it); a page with one gets its patches and then the toast.
func TestRerender_NoticeWithoutToastsMorphsBody(t *testing.T) {
	a := newFrApp(3)
	w := post(t, a, testOpts(nil), "/_m/R/Toggle?id=1&notice=1", nil, shownNow(t, a, testOpts(nil)))
	evs := events(w.Body.String())
	if !isBodyMorph(t, w) || !strings.Contains(evs[0], `<al-toasts popover="manual" data-ignore-morph></al-toasts>`) || len(evs) != 2 ||
		!strings.Contains(evs[1], "data: selector al-toasts\ndata: mode append") {
		t.Fatalf("%q", evs)
	}
	a = newFrApp(3)
	a.toasts = true
	w = post(t, a, testOpts(nil), "/_m/R/Toggle?id=1&notice=1", nil, shownNow(t, a, testOpts(nil)))
	evs = events(w.Body.String())
	if len(evs) != 3 || evs[0] != regionEvent("row-1", `<li id="row-1" class="done">Row 1</li>`) || !strings.Contains(evs[2], "data: selector al-toasts\ndata: mode append") {
		t.Fatalf("%q", evs)
	}
}

// A form that does not decode re-renders with its errors: one patch of the form (it has an id);
// the next click's before-view (plain state) is not the page shown (with errors), so it morphs
// the body.
func TestRerender_RejectedFormPatchesItsRegion(t *testing.T) {
	a := newFrApp(3)
	w := post(t, a, testOpts(nil), "/_m/R/Add", url.Values{"title": {""}}, shownNow(t, a, testOpts(nil)))
	evs := events(w.Body.String())
	if len(evs) != 2 || !strings.HasPrefix(evs[0], "event: datastar-patch-elements\ndata: selector #add\ndata: mode outer\n") || !strings.Contains(evs[0], "must not be empty") {
		t.Fatalf("%q", evs)
	}
	shown := shownOf(evs[1])
	w = post(t, a, testOpts(nil), "/_m/R/Toggle?id=1", nil, shown)
	if !isBodyMorph(t, w) || strings.Contains(w.Body.String(), "must not be empty") {
		t.Fatalf("%q", w.Body.String())
	}
}

// Options.Rendered gets the full render's body on every re-render, patched or morphed.
func TestRerender_CallsRendered(t *testing.T) {
	a := newFrApp(3)
	var got []string
	opts := testOpts(func(o *Options) { o.Rendered = func(rq *Request, body string) { got = append(got, body) } })
	shown := shownNow(t, a, opts)
	post(t, a, opts, "/_m/R/Toggle?id=1", nil, shown)
	post(t, a, opts, "/_m/R/Toggle?id=1", nil, "")
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
	w := post(t, a, opts, "/_m/R/Toggle?id=2", nil, shownNow(t, a, opts))
	if body := w.Body.String(); !strings.Contains(body, "<h3>Patches</h3><ul><li>#row-2</li><li>shown</li><li>panel</li></ul>") {
		t.Fatalf("%s", body)
	}
	w = post(t, a, opts, "/_m/R/Same", nil, shownNow(t, a, opts))
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
		{Method: "GET", Pattern: "/", Handle: func(rq *Request) Response { return PageResponse(pageOf(n), &html.FormState{}, nil, false) }},
		{Method: "POST", Pattern: "/_m/R/Inc", Handle: func(rq *Request) Response {
			before := n
			n++
			return PageResponse(pageOf(n), &html.FormState{}, nil, true).WithBefore(func() html.Node { return pageOf(before) })
		}},
	}
	h := NewRouter(routes, testOpts(nil))
	shown := shownOf(serveRaw(h, httptest.NewRequest("GET", "/", nil)).Body.String())
	req := httptest.NewRequest("POST", "/_m/R/Inc", nil)
	req.Header.Set("Datastar-Request", "true")
	req.Header.Set(html.ShownHeader, shown)
	w := serveRaw(h, req)
	if !isBodyMorph(t, w) || !strings.Contains(w.Body.String(), `<p id="msg">2<div>x</div></p>`) {
		t.Fatalf("%q", w.Body.String())
	}
}

// The before-render panic's log line masks a Sensitive path parameter and carries the request id.
func TestRerender_BeforePanicLineIsMasked(t *testing.T) {
	a := newFrApp(3)
	routes := append(a.routes(), Route{Method: "POST", Pattern: "/_m/R/Reset/{token}", Handle: func(rq *Request) Response {
		resp := PageResponse(a.view(a.done, a.heading), &html.FormState{}, nil, true)
		resp.Before = func() html.Node { panic("before") }
		return resp
	}})
	var logged []string
	opts := testOpts(func(o *Options) { o.Log = func(level, msg string) { logged = append(logged, level+" "+msg) } })
	req := httptest.NewRequest("POST", "/_m/R/Reset/hunter2secret", nil)
	req.Header.Set("Datastar-Request", "true")
	req.Header.Set(html.ShownHeader, shownNow(t, a, testOpts(nil)))
	serveRaw(NewRouter(routes, opts), req)
	if len(logged) != 1 || strings.Contains(logged[0], "hunter2secret") || !strings.Contains(logged[0], "/_m/R/Reset/••••: rendering the page before the message: before") || !strings.Contains(logged[0], " rid=") {
		t.Fatalf("logged %q", logged)
	}
}
