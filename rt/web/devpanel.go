package web

import (
	"bytes"
	"encoding/json"
	"fmt"
	stdhtml "html"
	"html/template"
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

// panelScript is the panel's own script, written once into a full document (a morph keeps the
// panel's shell and this script's work): it keeps the tab last chosen in the shell (data-tab) and
// shows that section again after each patch of the panel (which renders Request first, the other
// sections hidden); after each patch it also files the request in
// sessionStorage (the last 20 of this tab) and lists them in the History tab; a click on one shows
// its sections; "Copy as curl" copies the request's line. Dev mode only, like the panel.
const panelScript = `<script>(function(){if(window._alderHist)return;window._alderHist=1;var K="_alder_hist",ids=["request","trace","sql","answer","patches"];
function load(){try{return JSON.parse(sessionStorage.getItem(K)||"[]")}catch(e){return[]}}
function save(h){try{sessionStorage.setItem(K,JSON.stringify(h))}catch(e){}}
function pick(){var p=document.getElementById("_alder_panel"),id=p&&p.getAttribute("data-tab");if(!id)return;
p.querySelectorAll(":scope>section").forEach(function(s){var h=s.id!==id;if(s.hidden!==h)s.hidden=h});
p.querySelectorAll(":scope>nav>button").forEach(function(b){var v=String(b.getAttribute("aria-controls")===id);if(b.getAttribute("aria-selected")!==v)b.setAttribute("aria-selected",v)})}
function sync(){pick();var p=document.getElementById("_alder_panel"),nav=p&&p.querySelector(":scope>nav"),ol=p&&p.querySelector("ol[data-hist]");if(!nav||!ol||ol.children.length)return;
var h=load(),id=nav.getAttribute("data-rid");if(!h.some(function(e){return e.id===id})){var html="";ids.forEach(function(n){var s=document.getElementById("_alder_tab_"+n);if(s)html+=s.outerHTML.replace(/ id="_alder_tab_/g,' data-past="').replace(/ hidden(="[^"]*")?/g,"")});
h.push({id:id,bar:nav.querySelector("[data-bar]").textContent+" "+nav.children[1].textContent+" "+nav.children[2].textContent,html:html.slice(0,60000)});h=h.slice(-20);save(h)}
h.slice().reverse().forEach(function(e){var li=document.createElement("li"),b=document.createElement("button");b.type="button";b.setAttribute("data-id",e.id);b.textContent=e.bar;li.appendChild(b);ol.appendChild(li)})}
function watch(){var p=document.getElementById("_alder_panel");if(!p)return;new MutationObserver(sync).observe(p,{childList:true,subtree:true,attributes:true,attributeFilter:["data-rid"]});sync()}
document.addEventListener("click",function(ev){var t=ev.target.closest&&ev.target.closest("button");if(!t)return;
if(t.matches("#_alder_panel>nav>button[aria-controls]"))t.parentNode.parentNode.setAttribute("data-tab",t.getAttribute("aria-controls"));
if(t.hasAttribute("data-copy")){var c=t.getAttribute("data-copy"),done=function(){t.textContent="Copied"};
if(navigator.clipboard&&navigator.clipboard.writeText)navigator.clipboard.writeText(c).then(done,function(){fb()});else fb();
function fb(){var a=document.createElement("textarea");a.value=c;document.body.appendChild(a);a.select();try{document.execCommand("copy")}catch(e){}a.remove();done()}}
else if(t.hasAttribute("data-id")){var past=t.closest("section").querySelector("[data-past]"),e=load().filter(function(x){return x.id===t.getAttribute("data-id")})[0];past.innerHTML=e?e.html:""}});
if(document.readyState==="loading")document.addEventListener("DOMContentLoaded",watch);else watch()})()</script>`

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
	answer := tw.answer
	for _, h := range tw.hops {
		answer += ", then " + h.answer
	}
	reps := repeatMarks(segRepeats(tw.hops, rec))
	var b strings.Builder
	fmt.Fprintf(&b, `<nav data-rid="%d"><span data-bar>%s %s</span> <span>%s</span> <span data-ms>%.1f ms</span> <span>%d SQL</span>`,
		tw.start.UnixNano(), esc(r.Method), esc(r.URL.Path), esc(answer), ms, len(sqls))
	if len(reps) > 0 {
		b.WriteString(` <span data-nplus1>N+1</span>`)
	}
	tabs := []string{"Request", "Trace", "SQL", "Answer", "Patches", "History"}
	for i, t := range tabs {
		fmt.Fprintf(&b, ` <button type="button" aria-controls="_alder_tab_%s" aria-selected="%t">%s</button>`, strings.ToLower(t), i == 0, t)
	}
	b.WriteString(`</nav>`)
	// the hop markers: "then GET <url>" before the first step and statement of each redirect
	// follow served
	marker := func(i int, at func(h hopMark) int) {
		for _, h := range tw.hops {
			if at(h) == i {
				fmt.Fprintf(&b, `<li data-hop>then GET %s</li>`, esc(h.url))
			}
		}
	}

	// only the Request section is shown at first; the others are `hidden` until their tab is
	// chosen (the panel's script keeps the chosen tab across later answers)
	b.WriteString(`<section id="_alder_tab_request"><h3>Request</h3><pre>`)
	fmt.Fprintf(&b, "%s %s\n", esc(r.Method), esc(r.URL.RequestURI()))
	if tw.rq != nil {
		keys := make([]string, 0, len(tw.rq.Form))
		for k := range tw.rq.Form {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			fmt.Fprintf(&b, "form %s = %s\n", esc(k), esc(strings.Join(masked(k, tw.rq.Form[k]), ",")))
		}
		pk := make([]string, 0, len(tw.rq.Params))
		for k := range tw.rq.Params {
			pk = append(pk, k)
		}
		sort.Strings(pk)
		for _, k := range pk {
			fmt.Fprintf(&b, "param %s = %s\n", esc(k), esc(tw.rq.Params[k]))
		}
		fmt.Fprintf(&b, "session %s\n", esc(rt.sessionText(tw.rq.Session)))
	}
	curl := curlLine(tw)
	fmt.Fprintf(&b, "%s</pre><button type=\"button\" data-copy=\"%s\">Copy as curl</button></section>", esc(curl), esc(curl))

	b.WriteString(`<section id="_alder_tab_trace" hidden><h3>Trace</h3><ol>`)
	for i, s := range steps {
		marker(i, func(h hopMark) int { return h.steps })
		c := ""
		if s.Cap != "" {
			c = " [" + s.Cap + "]"
		}
		fmt.Fprintf(&b, `<li><code>%s</code>%s %.2f ms</li>`, esc(s.Name), c, s.Ms)
	}
	marker(len(steps), func(h hopMark) int { return h.steps })
	b.WriteString(`</ol></section>`)

	b.WriteString(`<section id="_alder_tab_sql" hidden><h3>SQL</h3>`)
	// a statement is counted per answer (the POST, then each redirect follow: the session's
	// user is selected by each); one run per id by a single Task.all, each time with other
	// arguments, is a bulk operation, not a repeated query: it is labelled, not flagged
	seg := func(i int) int { return segOf(tw.hops, i) }
	type key = segStmt
	count := map[key]int{}
	groups := map[key]map[int]bool{}
	args1 := map[key]map[string]bool{}
	var total float64
	for i, q := range sqls {
		k := key{seg(i), q.Stmt}
		count[k]++
		total += q.Ms
		if groups[k] == nil {
			groups[k], args1[k] = map[int]bool{}, map[string]bool{}
		}
		groups[k][q.Group] = true
		args1[k][fmt.Sprint(q.Args)] = true
	}
	bulk := func(k key) bool {
		return count[k] > 1 && len(groups[k]) == 1 && !groups[k][0] && len(args1[k]) == count[k]
	}
	fmt.Fprintf(&b, `<p>%d statements, %.2f ms</p><ol>`, len(sqls), total)
	for i, q := range sqls {
		marker(i, func(h hopMark) int { return h.sql })
		dup, k := "", key{seg(i), q.Stmt}
		if r, ok := reps[key{seg(i), eff.Shape(q.Stmt)}]; ok { // an N+1 (eff.RepeatsCut): five or more in one load, not a bulk write
			dup = fmt.Sprintf(` <strong data-nplus1>N+1 x%d</strong>`, r.Count)
		} else if bulk(k) {
			dup = fmt.Sprintf(` <span data-bulk>x%d (bulk)</span>`, count[k])
		} else if count[k] > 1 {
			dup = fmt.Sprintf(` <strong data-dup>duplicate x%d</strong>`, count[k])
		}
		rows := ""
		if q.Rows >= 0 {
			rows = fmt.Sprintf(", %d rows", q.Rows)
		}
		fmt.Fprintf(&b, `<li><code>%s</code> args %s (%.2f ms%s)%s</li>`, esc(q.Stmt), esc(fmt.Sprint(q.Args)), q.Ms, rows, dup)
	}
	marker(len(sqls), func(h hopMark) int { return h.sql })
	b.WriteString(`</ol></section>`)

	b.WriteString(`<section id="_alder_tab_answer" hidden><h3>Answer</h3>`)
	if len(tw.hops) == 0 {
		fmt.Fprintf(&b, `<p>%d %s</p>`, status, esc(tw.answer))
	} else {
		fmt.Fprintf(&b, `<p>%s</p>`, esc(tw.answer))
		for i, h := range tw.hops {
			if i == len(tw.hops)-1 {
				fmt.Fprintf(&b, `<p>then GET %s: %d %s</p>`, esc(h.url), status, esc(h.answer))
			} else {
				fmt.Fprintf(&b, `<p>then GET %s: %s</p>`, esc(h.url), esc(h.answer))
			}
		}
	}
	if tw.detail != "" {
		fmt.Fprintf(&b, `<pre>%s</pre>`, esc(tw.detail))
	}
	b.WriteString(`</section>`)

	b.WriteString(`<section id="_alder_tab_patches" hidden><h3>Patches</h3><ul>`)
	if tw.rq != nil && tw.rq.Datastar || len(tw.patches) > 0 {
		for _, p := range append(append([]string{}, tw.patches...), "panel") {
			fmt.Fprintf(&b, `<li>%s</li>`, esc(p))
		}
	} else {
		b.WriteString(`<li>none (a full document)</li>`)
	}
	b.WriteString(`</ul></section>`)
	b.WriteString(`<section id="_alder_tab_history" hidden><h3>History</h3><p>The last 20 requests of this tab; click one to see its sections.</p><ol data-hist></ol><div data-past></div></section>`)
	return b.String()
}

// segStmt is a statement's text in one answer segment (the request the client sent is 0, each
// redirect follow served after it the next).
type segStmt struct {
	seg  int
	stmt string
}

// segOf is the answer segment statement i belongs to.
func segOf(hops []hopMark, i int) int {
	n := 0
	for _, h := range hops {
		if h.sql <= i {
			n++
		}
	}
	return n
}

// segRepeats is the N+1s of each answer segment (the request, then each hop): eff.RepeatsCut of
// its statements, each load run counted on its own (rec.Cuts). By segment, in order.
func segRepeats(hops []hopMark, rec *eff.Recorder) [][]eff.Repeat {
	out := make([][]eff.Repeat, len(hops)+1)
	if rec == nil {
		return out
	}
	sqls := rec.SQL
	for s := range out {
		from, to := 0, len(sqls)
		if s > 0 {
			from = min(hops[s-1].sql, len(sqls))
		}
		if s < len(hops) {
			to = min(hops[s].sql, len(sqls))
		}
		var cuts []int
		for _, c := range rec.Cuts {
			if c > from && c < to {
				cuts = append(cuts, c-from)
			}
		}
		out[s] = eff.RepeatsCut(sqls[from:to], cuts, eff.RepeatThreshold)
	}
	return out
}

// repeatMarks is segRepeats by segment and shape (eff.Shape), for the SQL tab's marks.
func repeatMarks(reps [][]eff.Repeat) map[segStmt]eff.Repeat {
	out := map[segStmt]eff.Repeat{}
	for s, rs := range reps {
		for _, r := range rs {
			out[segStmt{s, r.Stmt}] = r
		}
	}
	return out
}

// warnRepeats logs at warn each N+1 of the answer tw tracked, with the request line of its
// segment: `GET /slow: N+1: …`. Dev mode only: a request has a Recorder only then.
func (rt *router) warnRepeats(tw *tracker) {
	if !rt.opts.Dev || tw.rq == nil || tw.rq.Env == nil || tw.rq.Env.Rec == nil {
		return // a test's browser records too (Options.EnvFor), and warns in its own report
	}
	for s, rs := range segRepeats(tw.hops, tw.rq.Env.Rec) {
		head := tw.r.Method + " " + tw.r.URL.Path
		if s > 0 {
			u, _, _ := strings.Cut(tw.hops[s-1].url, "?")
			head = "GET " + u
		}
		for _, r := range rs {
			rt.log("warn", head+": "+r.String())
		}
	}
}

// masked is the values of form field k as the panel shows them: a field whose name says it holds
// a password (or a secret, a token) is shown as "••••", so the panel, its History and the curl
// line never carry one.
func masked(k string, vs []string) []string {
	n := strings.ToLower(k)
	for _, w := range []string{"password", "passwd", "secret", "token"} {
		if strings.Contains(n, w) {
			out := make([]string, len(vs))
			for i := range out {
				out[i] = "••••"
			}
			return out
		}
	}
	return vs
}

// sessionText is the session as the panel shows it: the record the app's codec stores, as
// JSON ("none" without a session).
func (rt *router) sessionText(s any) string {
	if s == nil || rt.opts.Session == nil {
		return "none"
	}
	b, err := rt.opts.Session.Encode(s)
	if err != nil {
		return fmt.Sprintf("%+v", s)
	}
	var v any
	if json.Unmarshal(b, &v) == nil {
		if pretty, err := json.MarshalIndent(v, "", "  "); err == nil {
			return string(pretty)
		}
	}
	return string(b)
}

// shq quotes s for a POSIX shell, in single quotes.
func shq(s string) string { return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'" }

// curlLine is the request as a curl command: the method, the URL, the session cookie as a
// placeholder (the real one is a secret of the browser), the Datastar header of a partial
// update and the form fields.
func curlLine(tw *tracker) string {
	r := tw.r
	var b strings.Builder
	b.WriteString("curl -X " + r.Method + " " + shq("http://"+r.Host+r.URL.RequestURI()))
	b.WriteString(" -H " + shq("Cookie: "+CookieName+"=<your session cookie>"))
	if tw.rq != nil && tw.rq.Datastar {
		b.WriteString(" -H " + shq("Datastar-Request: true"))
	}
	if tw.rq != nil {
		keys := make([]string, 0, len(tw.rq.Form))
		for k := range tw.rq.Form {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			for _, v := range masked(k, tw.rq.Form[k]) {
				b.WriteString(" --data-urlencode " + shq(k+"="+v))
			}
		}
	}
	return b.String()
}

// inlinePanel puts the whole panel before the last </body> of a full document.
func (rt *router) inlinePanel(w http.ResponseWriter, page string, status int) string {
	tw, ok := w.(*tracker)
	if !ok || !rt.panelOn() {
		return page
	}
	return endOfBody(page, `<al-devpanel id="`+datastar.PanelID+`" data-ignore-morph>`+rt.panelInner(tw, status)+`</al-devpanel>`+panelScript+panelUI)
}

// shellBody adds to a body patch the panel's element (dev) and, when a Noticed answer has a toast
// and the page has no <al-toasts> region of its own, the implicit region (kept across morphs).
func (rt *router) shellBody(w http.ResponseWriter, body string) string {
	if tw, ok := w.(*tracker); ok && tw.notice != "" && !strings.Contains(body, "<al-toasts") {
		body = endOfBody(body, `<al-toasts popover="manual" data-ignore-morph></al-toasts>`)
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

// devPage is /_dev's template: the generator's map (Options.DevMap) with what only the runtime
// knows, the build, the routes, how each capability is wired and the migrations the database has.
var devPage = template.Must(template.New("dev").Parse(`<!doctype html><html><head><meta charset="utf-8"><title>Alder dev map</title></head><body>
<h1>Alder dev map</h1>
<p>build <code>{{.Build}}</code></p>
<h2>Pages</h2>
{{range .Map.Pages}}<h3>{{if .Link}}<a href="{{.Path}}">{{.Path}}</a>{{else}}<code>{{.Path}}</code>{{end}} <small>{{.Module}}</small></h3>
<ul>{{if .Requires}}<li>requires {{.Requires}}{{if .Roles}}, role {{range $i, $r := .Roles}}{{if $i}} | {{end}}{{$r}}{{end}}{{end}}</li>{{end}}
{{range .Params}}<li>param <code>{{.}}</code></li>{{end}}{{range .Msgs}}<li>message <code>{{.}}</code></li>{{end}}</ul>
{{end}}{{if .Map.AppMsgs}}<h3>App messages</h3><ul>{{range .Map.AppMsgs}}<li><code>{{.}}</code></li>{{end}}</ul>{{end}}
<h2>Tables</h2>
{{range .Map.Tables}}<h3>{{.Name}}</h3><ul>{{range .Columns}}<li><code>{{.}}</code></li>{{end}}
{{if .ID}}<li>id: {{.ID}}</li>{{end}}{{if .Created}}<li>created: {{.Created}}</li>{{end}}{{if .Updated}}<li>updated: {{.Updated}}</li>{{end}}{{if .Owner}}<li>owner: {{.Owner}}</li>{{end}}
{{range .Unique}}<li>unique: {{.}}</li>{{end}}{{range $f, $t := .Refs}}<li>ref {{$f}} to {{$t}}</li>{{end}}{{range $f, $a := .OnDelete}}<li>on delete {{$f}}: {{$a}}</li>{{end}}</ul>
{{end}}
<h2>Capabilities</h2>
<ul>{{range .Caps}}<li><code>{{.Name}}</code>: {{.Kind}}</li>{{end}}</ul>
<h2>Session</h2>
<ul>{{range .Map.Session}}<li><code>{{.}}</code></li>{{end}}</ul>
<h2>Migrations applied</h2>
<ul>{{range .Migrations}}<li><code>{{.}}</code></li>{{else}}<li>none</li>{{end}}</ul>
<h2>Routes</h2>
<ul>{{range .Routes}}<li>{{if .Link}}{{.Method}} <a href="{{.Pat}}">{{.Pat}}</a>{{else}}{{.Method}} <code>{{.Pat}}</code>{{end}}</li>{{end}}</ul>
</body></html>`))

type devMapData struct {
	Pages   []devMapPage  `json:"pages"`
	Tables  []devMapTable `json:"tables"`
	Session []string      `json:"session"`
	AppMsgs []string      `json:"appMsgs"`
}

type devMapPage struct {
	Module, Path, Requires string
	Roles, Params, Msgs    []string
	Link                   bool `json:"-"`
}

type devMapTable struct {
	Name                        string
	Columns                     []string
	ID, Created, Updated, Owner string
	Unique                      []string
	Refs, OnDelete              map[string]string
}

type devCap struct{ Name, Kind string }
type devRoute struct {
	Method, Pat string
	Link        bool
}

// devMap serves /_dev: the running app's pages, messages, tables, capabilities (adapter or fake
// wired), session shape, migrations applied, routes and build id.
func (rt *router) devMap(w http.ResponseWriter, r *http.Request) {
	data := struct {
		Build      string
		Map        devMapData
		Caps       []devCap
		Migrations []string
		Routes     []devRoute
	}{Build: BuildID(), Migrations: rt.opts.Migrations}
	if rt.opts.DevMap != "" {
		if err := json.Unmarshal([]byte(rt.opts.DevMap), &data.Map); err != nil {
			rt.log("error", "reading the dev map: "+err.Error())
		}
	}
	for i := range data.Map.Pages {
		data.Map.Pages[i].Link = !strings.Contains(data.Map.Pages[i].Path, "{")
	}
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
		data.Routes = append(data.Routes, devRoute{Method: rtn.method, Pat: pat, Link: rtn.method == http.MethodGet && !strings.Contains(pat, "{")})
	}
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
		data.Caps = append(data.Caps, devCap{k, kind})
	}
	var b bytes.Buffer
	if err := devPage.Execute(&b, data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write(b.Bytes())
}
