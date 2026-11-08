//go:build browser

package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"alder/rt"
	"alder/rt/datastar"
	"alder/rt/html"
)

// TestBrowser_RouteDeleteBody checks, in a real browser (testdata/browser/routedelete.cjs), what
// the requests rt/html renders for a DELETE route (#160) send, with JavaScript and without:
// Datastar v1.0.0 treats DELETE as it does GET (the payload in the URL, no body), so a DELETE
// whose request carries a form is rendered as a POST, and its _method=DELETE must be in the
// url-encoded body — from the hidden field, or from the form's own submit button (Datastar's
// form serialisation is `new FormData(form)`, which leaves the submitter out, plus the
// submitter's name and value appended when the form's own submit event sent it). A field of
// the form is never in the URL. A DELETE click on a button inside another form stays a Datastar
// DELETE without a form: its only query is `datastar={}`, which rt/web drops.
func TestBrowser_RouteDeleteBody(t *testing.T) {
	t.Parallel()
	node, env := playwrightEnv(t)
	script, err := filepath.Abs(filepath.Join("testdata", "browser", "routedelete.cjs"))
	if err != nil {
		t.Fatal(err)
	}
	del := func(n string) rt.Route { return rt.Route{Name: "D", Method: "DELETE", Path: "/d/" + n} }
	input := func(id, name, value string) html.Node {
		return html.El("input", []html.Attr{html.A("id", id), html.A("name", name), html.A("value", value)})
	}
	body := html.Nodes(
		// 1: a click button outside a form: a form of its own
		html.El("button", []html.Attr{html.A("id", "b1"), html.RouteAttr("data-on:click", del("1"))}, html.Text("1")),
		// 2: submit= with its own submit button (name="_method" on the button)
		html.FormToRoute(del("2"), nil, input("i2", "secret", "s2"), html.El("button", []html.Attr{html.A("id", "b2")}, html.Text("2"))),
		// 3: submit= without a button (Enter in the field): the hidden field
		html.FormToRoute(del("3"), nil, input("i3", "title", "t3")),
		// 4: submit= whose only submit button has its own name: the hidden field
		html.FormToRoute(del("4"), nil, input("i4", "title", "t4"),
			html.El("button", []html.Attr{html.A("id", "b4"), html.A("name", "action"), html.A("value", "save")}, html.Text("4"))),
		// 5: a DELETE click button inside another route's form (a POST: no _method of its own)
		html.FormToRoute(rt.Route{Name: "M.Save", Method: "POST", Path: "/save"}, nil, input("i5", "secret", "s5"),
			html.El("button", []html.Attr{html.A("id", "b5"), html.RouteAttr("data-on:click", del("5"))}, html.Text("5"))),
		// 6: as 2, submitted by Enter in the field (the submitter is the default button)
		html.FormToRoute(del("6"), nil, input("i6", "title", "t6"), html.El("button", []html.Attr{html.A("id", "b6")}, html.Text("6"))),
	)

	type hit struct {
		Path, Method, Query, Type, Body string
		JS                              bool
	}
	var mu sync.Mutex
	var hits []hit
	mux := http.NewServeMux()
	mux.Handle(datastar.ScriptPath, datastar.Handler())
	mux.HandleFunc("/d/", func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		h := hit{r.URL.Path, r.Method, r.URL.RawQuery, r.Header.Get("Content-Type"), string(b), datastar.IsDatastar(r)}
		mu.Lock()
		hits = append(hits, h)
		mu.Unlock()
		if h.JS {
			w.Header().Set("Content-Type", "text/event-stream")
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprint(w, `<!doctype html><title>done</title><p>done</p>`)
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprintf(w, `<!doctype html><html><head><title>delete</title>%s%s</head><body>%s</body></html>`,
			html.Render(datastar.ScriptTag()), datastar.BoostTag(), html.Render(body))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	cmd := exec.Command(node, script, srv.URL)
	cmd.Env = env
	var stderr strings.Builder
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("routedelete.cjs: %v\n%s\n%s", err, out, stderr.String())
	}
	var res struct{ Errors []string }
	if err := json.Unmarshal(out, &res); err != nil {
		t.Fatalf("routedelete.cjs printed %q (%v)\n%s", out, err, stderr.String())
	}
	if len(res.Errors) > 0 {
		t.Errorf("page errors: %v", res.Errors)
	}

	mu.Lock()
	defer mu.Unlock()
	for _, h := range hits {
		t.Logf("js=%v %s %s?%s [%s] %s", h.JS, h.Method, h.Path, h.Query, h.Type, h.Body)
	}
	find := func(path string, js bool) *hit {
		for i := range hits {
			if hits[i].Path == path && hits[i].JS == js {
				return &hits[i]
			}
		}
		t.Errorf("no request to %s (js=%v)", path, js)
		return nil
	}
	formBody := func(path string, js bool, want ...string) {
		h := find(path, js)
		if h == nil {
			return
		}
		if h.Method != "POST" || !strings.HasPrefix(h.Type, "application/x-www-form-urlencoded") || h.Query != "" {
			t.Errorf("%s (js=%v): %s ?%q [%s], want a url-encoded POST with no query", path, js, h.Method, h.Query, h.Type)
		}
		if strings.Count(h.Body, "_method=") != 1 || !strings.Contains(h.Body, "_method=DELETE") {
			t.Errorf("%s (js=%v): body %q, want exactly one _method=DELETE", path, js, h.Body)
		}
		for _, w := range want {
			if !strings.Contains(h.Body, w) {
				t.Errorf("%s (js=%v): body %q lacks %s", path, js, h.Body, w)
			}
		}
	}
	for _, js := range []bool{true, false} {
		formBody("/d/1", js)
		formBody("/d/2", js, "secret=s2")
		formBody("/d/3", js, "title=t3")
		formBody("/d/4", js, "title=t4", "action=save")
		formBody("/d/6", js, "title=t6")
	}
	if h := find("/d/5", true); h != nil {
		if h.Method != "DELETE" || h.Body != "" || (h.Query != "" && h.Query != "datastar=%7B%7D") {
			t.Errorf("/d/5: %s ?%q %q, want a DELETE with no body and at most datastar={}", h.Method, h.Query, h.Body)
		}
	}
}
