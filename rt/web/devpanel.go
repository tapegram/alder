package web

import (
	"fmt"
	stdhtml "html"
	"net/http"
	"os"
	"sort"
	"strings"
	"time"

	"alder/rt/datastar"
	"alder/rt/eff"
)

// panelOn reports whether the dev panel is rendered: dev mode, unless ALDER_DEV_PANEL=0.
func (rt *router) panelOn() bool {
	return rt.opts.Dev && os.Getenv("ALDER_DEV") == "1" && os.Getenv("ALDER_DEV_PANEL") != "0"
}

func esc(s string) string { return stdhtml.EscapeString(s) }

// shell is the panel element kept in every morphed body: data-ignore-morph on old and new keeps
// the existing one, with its open state and selected tab.
const panelShell = `<al-devpanel id="` + datastar.PanelID + `" data-ignore-morph></al-devpanel>`

// panelInner is the panel's content for the request tw tracks: the bar summary, a nav of tab
// buttons and the sections (Request, Trace, SQL, Answer, Patches).
func (rt *router) panelInner(tw *tracker, status int) string {
	r := tw.r
	var rec *eff.Recorder
	if tw.rq != nil && tw.rq.Env != nil {
		rec = tw.rq.Env.Rec
	}
	ms := eff.Ms(time.Since(tw.start))
	var steps []eff.StepRec
	var sqls []eff.SQLRec
	if rec != nil {
		steps, sqls = rec.Steps, rec.SQL
	}
	var b strings.Builder
	fmt.Fprintf(&b, `<nav><span data-bar>%s %s</span> <span>%s</span> <span>%.1f ms</span> <span>%d SQL</span>`,
		esc(r.Method), esc(r.URL.Path), esc(tw.answer), ms, len(sqls))
	tabs := []string{"Request", "Trace", "SQL", "Answer", "Patches"}
	for _, t := range tabs {
		fmt.Fprintf(&b, ` <button type="button" aria-controls="_alder_tab_%s">%s</button>`, strings.ToLower(t), t)
	}
	b.WriteString(`</nav>`)

	b.WriteString(`<section id="_alder_tab_request"><h3>Request</h3><pre>`)
	fmt.Fprintf(&b, "%s %s\n", esc(r.Method), esc(r.URL.RequestURI()))
	if tw.rq != nil {
		keys := make([]string, 0, len(tw.rq.Form))
		for k := range tw.rq.Form {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			fmt.Fprintf(&b, "form %s = %s\n", esc(k), esc(strings.Join(tw.rq.Form[k], ",")))
		}
		pk := make([]string, 0, len(tw.rq.Params))
		for k := range tw.rq.Params {
			pk = append(pk, k)
		}
		sort.Strings(pk)
		for _, k := range pk {
			fmt.Fprintf(&b, "param %s = %s\n", esc(k), esc(tw.rq.Params[k]))
		}
		fmt.Fprintf(&b, "session %s\n", esc(fmt.Sprintf("%+v", tw.rq.Session)))
	}
	fmt.Fprintf(&b, "curl -X %s 'http://%s%s'", esc(r.Method), esc(r.Host), esc(r.URL.RequestURI()))
	b.WriteString(`</pre></section>`)

	b.WriteString(`<section id="_alder_tab_trace"><h3>Trace</h3><ol>`)
	for _, s := range steps {
		c := ""
		if s.Cap != "" {
			c = " [" + s.Cap + "]"
		}
		fmt.Fprintf(&b, `<li><code>%s</code>%s %.2f ms</li>`, esc(s.Name), c, s.Ms)
	}
	b.WriteString(`</ol></section>`)

	b.WriteString(`<section id="_alder_tab_sql"><h3>SQL</h3>`)
	count := map[string]int{}
	var total float64
	for _, q := range sqls {
		count[q.Stmt]++
		total += q.Ms
	}
	fmt.Fprintf(&b, `<p>%d statements, %.2f ms</p><ol>`, len(sqls), total)
	for _, q := range sqls {
		dup := ""
		if count[q.Stmt] > 1 {
			dup = fmt.Sprintf(` <strong data-dup>duplicate x%d</strong>`, count[q.Stmt])
		}
		rows := ""
		if q.Rows >= 0 {
			rows = fmt.Sprintf(", %d rows", q.Rows)
		}
		fmt.Fprintf(&b, `<li><code>%s</code> args %s (%.2f ms%s)%s</li>`, esc(q.Stmt), esc(fmt.Sprint(q.Args)), q.Ms, rows, dup)
	}
	b.WriteString(`</ol></section>`)

	fmt.Fprintf(&b, `<section id="_alder_tab_answer"><h3>Answer</h3><p>%d %s</p>`, status, esc(tw.answer))
	if tw.detail != "" {
		fmt.Fprintf(&b, `<pre>%s</pre>`, esc(tw.detail))
	}
	b.WriteString(`</section>`)

	b.WriteString(`<section id="_alder_tab_patches"><h3>Patches</h3><ul>`)
	if tw.rq != nil && tw.rq.Datastar || len(tw.patches) > 0 {
		for _, p := range append(append([]string{}, tw.patches...), "panel") {
			fmt.Fprintf(&b, `<li>%s</li>`, esc(p))
		}
	} else {
		b.WriteString(`<li>none (a full document)</li>`)
	}
	b.WriteString(`</ul></section>`)
	return b.String()
}

// inlinePanel puts the whole panel before the last </body> of a full document.
func (rt *router) inlinePanel(w http.ResponseWriter, page string, status int) string {
	tw, ok := w.(*tracker)
	if !ok || !rt.panelOn() {
		return page
	}
	return endOfBody(page, `<al-devpanel id="`+datastar.PanelID+`" data-ignore-morph>`+rt.panelInner(tw, status)+`</al-devpanel>`)
}

// shellBody adds to a body patch the panel's element (dev) and, when a Noticed answer has a toast
// and the page has no <al-toasts> region of its own, the implicit region (kept across morphs).
func (rt *router) shellBody(w http.ResponseWriter, body string) string {
	if tw, ok := w.(*tracker); ok && tw.notice != "" && !strings.Contains(body, "<al-toasts") {
		body = endOfBody(body, `<al-toasts data-ignore-morph></al-toasts>`)
	}
	if !rt.panelOn() {
		return body
	}
	return endOfBody(body, panelShell)
}

// patchPanel writes the panel's own event, after the body's.
func (rt *router) patchPanel(w http.ResponseWriter, status int) {
	tw, ok := w.(*tracker)
	if !ok || !rt.panelOn() {
		return
	}
	if err := datastar.PatchPanel(w, rt.panelInner(tw, status)); err != nil {
		rt.log("error", "writing the panel patch: "+err.Error())
	}
}

// notify patches the toast of a Noticed answer into the page's <al-toasts> region, as its own
// event after the body's, so a later morph does not show it again.
func (rt *router) notify(w http.ResponseWriter) {
	tw, ok := w.(*tracker)
	if !ok || tw.notice == "" {
		return
	}
	toast := `<al-toast variant="info" timeout="5000">` + esc(tw.notice) + `</al-toast>`
	tw.patches = append(tw.patches, "toast")
	tw.notice = ""
	if err := datastar.PatchToast(w, toast); err != nil {
		rt.log("error", "writing the toast patch: "+err.Error())
	}
}

// devOn is true for the dev server only (Options.Dev and ALDER_DEV=1): the /_dev map is served
// then and never from a production binary.
func (rt *router) devOn() bool { return rt.opts.Dev && os.Getenv("ALDER_DEV") == "1" }

// devMap serves /_dev: the running app's routes, capabilities (adapter or fake wired) and build.
func (rt *router) devMap(w http.ResponseWriter, r *http.Request) {
	var b strings.Builder
	b.WriteString(`<!doctype html><html><head><meta charset="utf-8"><title>Alder dev map</title></head><body><h1>Alder dev map</h1>`)
	fmt.Fprintf(&b, `<p>build <code>%s</code></p><h2>Routes</h2><ul>`, esc(BuildID()))
	for _, rtn := range rt.routes {
		pat := ""
		for _, s := range rtn.segs {
			if s.param != "" {
				pat += "/{" + s.param + "}"
			} else {
				pat += "/" + s.lit
			}
		}
		if pat == "" {
			pat = "/"
		}
		if rtn.method == http.MethodGet && !strings.Contains(pat, "{") {
			fmt.Fprintf(&b, `<li>GET <a href="%s">%s</a></li>`, esc(pat), esc(pat))
		} else {
			fmt.Fprintf(&b, `<li>%s <code>%s</code></li>`, esc(rtn.method), esc(pat))
		}
	}
	b.WriteString(`</ul><h2>Capabilities</h2><ul>`)
	caps := make([]string, 0, len(rt.opts.Caps))
	for k := range rt.opts.Caps {
		caps = append(caps, k)
	}
	sort.Strings(caps)
	for _, k := range caps {
		kind := "adapter"
		if strings.Contains(strings.ToLower(fmt.Sprintf("%T", rt.opts.Caps[k])), "fake") {
			kind = "fake"
		}
		fmt.Fprintf(&b, `<li><code>%s</code>: %s</li>`, esc(k), kind)
	}
	b.WriteString(`</ul></body></html>`)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte(b.String()))
}
