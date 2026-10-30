package rtesting

import (
	"strconv"
	"strings"
	"testing"

	"alder/rt"
	"alder/rt/eff"
	"alder/rt/html"
	"alder/rt/web"
)

// The rows fixture: a page of n rows, each `<li id="row-N">` with a Toggle button, its state in
// a slice the routes share (no table: the fragment path is about markup). Toggle answers as the
// generated update route does: Before is the view of the model before update.

type rowsApp struct {
	done  []bool
	shown []string // the Alder-Shown header of each Toggle
}

func (a *rowsApp) view(done []bool) html.Node {
	var items []html.Node
	for i, d := range done {
		class := "open"
		if d {
			class = "done"
		}
		items = append(items, html.El("li", []html.Attr{html.A("id", "row-"+strconv.Itoa(i+1)), html.A("class", class)},
			html.El("span", nil, html.Text("Row "+strconv.Itoa(i+1))),
			html.El("button", []html.Attr{html.MsgAttr("data-on:click", msg("Rows", "Toggle", rt.KV{Key: "id", Value: strconv.Itoa(i + 1)}))}, html.Text("Toggle"))))
	}
	return html.El("html", nil, html.El("head", nil, html.El("title", nil, html.Text("Rows"))),
		html.El("body", nil, html.El("h1", nil, html.Text("Rows")), html.El("ul", []html.Attr{html.A("id", "rows")}, items...)))
}

func (a *rowsApp) routes() []web.Route {
	return []web.Route{
		{Method: "GET", Pattern: "/", Handle: func(rq *web.Request) web.Response {
			return web.PageResponse(a.view(a.done), &html.FormState{}, nil, false)
		}},
		{Method: "POST", Pattern: "/_m/Rows/Toggle", Handle: func(rq *web.Request) web.Response {
			before := append([]bool(nil), a.done...)
			a.shown = append(a.shown, rq.HTTP.Header.Get(html.ShownHeader))
			i, _ := strconv.Atoi(rq.Query.Get("id"))
			a.done[i-1] = !a.done[i-1]
			resp := web.PageResponse(a.view(a.done), &html.FormState{}, nil, true)
			resp.Before = func() html.Node { return a.view(before) }
			return resp
		}},
	}
}

// browseRows runs steps as one task test against a rowsApp of n rows; it returns the result
// without failing t, so a test can look at a failing step's error.
func browseRows(app *rowsApp, steps ...func(b *Browser, e *eff.Env) error) CaseResult {
	var task eff.Task[bool] = eff.Pure(true)
	for i := len(steps) - 1; i >= 0; i-- {
		f := steps[i]
		rest := task
		task = eff.Bind(eff.Call("step", true, func(e *eff.Env) (bool, error) {
			return true, f(BrowserOf(e), e)
		}), func(bool) eff.Task[bool] { return rest })
	}
	rs := Run([]Case{{Module: "M", Name: "rows", Task: func() eff.Task[bool] { return task }}},
		Options{Tables: tables, Routes: app.routes(), Web: web.Options{}})
	return rs[0]
}

// heldBody is doc's <body>, rendered.
func heldBody(t *testing.T, doc html.Node) string {
	t.Helper()
	bs := html.Select(doc, selBody)
	if len(bs) != 1 {
		t.Fatalf("no body in %s", html.Render(doc))
	}
	return html.Render(bs[0])
}

// TestBrowser_PatchedPageEqualsFullRender: a click on one row of a rows page leaves the held
// page showing the toggled row and equal to a fresh GET of the page.
func TestBrowser_PatchedPageEqualsFullRender(t *testing.T) {
	app := &rowsApp{done: make([]bool, 10)}
	var clicked, fresh string
	res := browseRows(app,
		func(b *Browser, e *eff.Env) error { _, err := b.Open("/"); return err },
		func(b *Browser, e *eff.Env) error {
			r, err := b.Click("/_m/Rows/Toggle?id=7")
			if err != nil {
				return err
			}
			if r.Answer.Tag != Rerendered {
				t.Errorf("answer = %+v", r.Answer)
			}
			clicked = heldBody(t, b.Doc())
			return nil
		},
		func(b *Browser, e *eff.Env) error {
			_, err := b.Get("/")
			fresh = heldBody(t, b.Doc())
			return err
		})
	if !res.OK {
		t.Fatalf("the test failed: %s", res.Error)
	}
	if !strings.Contains(clicked, `<li id="row-7" class="done">`) {
		t.Errorf("the toggled row is not shown:\n%s", clicked)
	}
	if clicked != fresh {
		t.Errorf("the patched page differs from a fresh GET:\n%s\n%s", clicked, fresh)
	}
	// the click carried the hash of the page the GET served, so the router sent fragments
	page := html.RenderWith(app.view(make([]bool, 10)), &html.FormState{})
	if want := html.Hash(html.BodyOf(page)); len(app.shown) != 1 || app.shown[0] != want {
		t.Errorf("Alder-Shown %q, want %s", app.shown, want)
	}
	if strings.Contains(clicked, "al-shown") {
		t.Error("the held page shows the router's al-shown")
	}
}

// TestBrowser_SendsShown: each post carries the hash of the page held, which a fragment answer
// moves on (its #_alder_shown patch); a page held without one (no al-shown) sends none.
func TestBrowser_SendsShown(t *testing.T) {
	app := &rowsApp{done: make([]bool, 3)}
	hashOf := func(done ...bool) string {
		return html.Hash(html.BodyOf(html.RenderWith(app.view(done), &html.FormState{})))
	}
	res := browseRows(app,
		func(b *Browser, e *eff.Env) error { _, err := b.Open("/"); return err },
		func(b *Browser, e *eff.Env) error { _, err := b.Click("/_m/Rows/Toggle?id=1"); return err },
		func(b *Browser, e *eff.Env) error { _, err := b.Click("/_m/Rows/Toggle?id=2"); return err })
	if !res.OK {
		t.Fatalf("the test failed: %s", res.Error)
	}
	if want := []string{hashOf(false, false, false), hashOf(true, false, false)}; strings.Join(app.shown, " ") != strings.Join(want, " ") {
		t.Errorf("Alder-Shown %q, want %q", app.shown, want)
	}
}

// TestBrowser_ReportsFragmentDrift: when the page the browser holds after applying an answer is
// not the page a full render shows — here the held page drifted from the one the router served
// while its hash stayed, as a client DOM changed by hand would — the step fails, naming where.
func TestBrowser_ReportsFragmentDrift(t *testing.T) {
	app := &rowsApp{done: make([]bool, 10)}
	res := browseRows(app,
		func(b *Browser, e *eff.Env) error {
			if _, err := b.Open("/"); err != nil {
				return err
			}
			li, _ := html.Parse(`<li id="row-3" class="open"><span>Row 3 (edited)</span></li>`)
			b.dom, _ = html.ReplaceByID(b.dom, "row-3", li)
			return nil
		},
		func(b *Browser, e *eff.Env) error { _, err := b.Click("/_m/Rows/Toggle?id=7"); return err })
	if res.OK {
		t.Fatal("the drift was not reported")
	}
	if !strings.Contains(res.Error, "POST /_m/Rows/Toggle: the patched page differs from a full render at body > ul#rows > li#row-3") {
		t.Fatalf("error: %s", res.Error)
	}
}

// msgPage is a page whose only region (#msg) holds a <div> in a <p>, which the browser's parser
// restructures: <p id="msg">n</p><div>x</div><p></p>.
func msgPage(n int) html.Node {
	return html.El("html", nil, html.El("head", nil, html.El("title", nil, html.Text("M"))), html.El("body", nil,
		html.El("p", []html.Attr{html.A("id", "msg")}, html.Text(strconv.Itoa(n)), html.El("div", nil, html.Text("x"))),
		html.El("button", []html.Attr{html.MsgAttr("data-on:click", msg("M", "Inc"))}, html.Text("Inc"))))
}

func msgRoutes(n *int) []web.Route {
	return []web.Route{
		{Method: "GET", Pattern: "/", Handle: func(rq *web.Request) web.Response {
			return web.PageResponse(msgPage(*n), &html.FormState{}, nil, false)
		}},
		{Method: "POST", Pattern: "/_m/M/Inc", Handle: func(rq *web.Request) web.Response {
			before := *n
			*n++
			return web.PageResponse(msgPage(*n), &html.FormState{}, nil, true).WithBefore(func() html.Node { return msgPage(before) })
		}},
	}
}

func browseMsg(n *int, steps ...func(b *Browser) error) CaseResult {
	var task eff.Task[bool] = eff.Pure(true)
	for i := len(steps) - 1; i >= 0; i-- {
		f := steps[i]
		rest := task
		task = eff.Bind(eff.Call("step", true, func(e *eff.Env) (bool, error) {
			return true, f(BrowserOf(e))
		}), func(bool) eff.Task[bool] { return rest })
	}
	return Run([]Case{{Module: "M", Name: "msg", Task: func() eff.Task[bool] { return task }}},
		Options{Tables: tables, Routes: msgRoutes(n), Web: web.Options{}})[0]
}

// TestBrowser_RepairedPageFallsBack: clicks on a page whose region the parser restructures pass
// the drift check: the router morphs the body. A #msg patch, as the markup alone would suggest,
// is caught: on the DOM the browser built it adds another <div> and <p>.
func TestBrowser_RepairedPageFallsBack(t *testing.T) {
	n := 1
	click := func(b *Browser) error { _, err := b.Click("/_m/M/Inc"); return err }
	res := browseMsg(&n, func(b *Browser) error { _, err := b.Open("/"); return err }, click, click)
	if !res.OK {
		t.Fatalf("the test failed: %s", res.Error)
	}

	n = 1
	res = browseMsg(&n, func(b *Browser) error { _, err := b.Open("/"); return err }, func(b *Browser) error {
		b.rendered = html.BodyOf(html.Render(msgPage(2)))
		ev, err := sseEvents("event: datastar-patch-elements\ndata: selector #msg\ndata: mode outer\ndata: elements <p id=\"msg\">2<div>x</div></p>\n\n")
		if err != nil {
			return err
		}
		if err := b.patch(ev[0]); err != nil {
			return err
		}
		return b.checkPatched()
	})
	if res.OK || !strings.Contains(res.Error, "the patched page differs from a full render") {
		t.Fatalf("the misplaced patch was not caught: %+v", res)
	}
}
