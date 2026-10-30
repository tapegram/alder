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
	done []bool
	lie  bool // Before renders a page the client never saw (TestBrowser_ReportsFragmentDrift)
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
			if a.lie {
				before[0] = !before[0]
			}
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
}
