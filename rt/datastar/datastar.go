// Package datastar serves the vendored Datastar client script and writes the server-sent events
// the Alder web runtime answers Datastar requests with. The script is embedded, so an Alder
// binary needs no CDN; Version names the vendored release (see VERSION).
package datastar

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"alder/rt"
	"alder/rt/html"
)

// Version is the vendored Datastar release; it must equal the contents of VERSION and the
// version in datastar.js's first line (checked by the tests).
const Version = "v1.0.0"

// ScriptPath is where Handler is mounted by rt/web: versioned, so the immutable cache policy is
// safe (a new Datastar is a new URL). The unversioned /_alder/datastar.js of earlier releases is
// gone: a request for it is an ordinary unknown path (404).
const ScriptPath = "/_alder/datastar-" + Version + ".js"

//go:embed datastar.js
var script []byte

// Handler serves the embedded datastar.js with a long-lived, immutable cache policy (the
// content changes only with Version, which ScriptPath carries). Mount it at ScriptPath.
func Handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("Content-Type", "application/javascript; charset=utf-8")
		h.Set("Cache-Control", "public, max-age=31536000, immutable")
		_, _ = w.Write(script)
	})
}

// ScriptTag is the <script type="module" src="/_alder/datastar-v1.0.0.js"></script> element (the
// path is ScriptPath) a page's <head> carries.
func ScriptTag() html.Node {
	return html.El("script", []html.Attr{html.A("type", "module"), html.A("src", ScriptPath)})
}

// IsDatastar reports whether r was sent by the Datastar client (header Datastar-Request: true),
// i.e. whether it should be answered with PatchBody or Navigate rather than a full page or a
// redirect.
func IsDatastar(r *http.Request) bool {
	return r.Header.Get("Datastar-Request") == "true"
}

// PatchBody answers a Datastar request with one `datastar-patch-elements` event that replaces
// the page's <body> (selector body, mode outer) with body, the rendered <body> element: one
// `elements` data line per line of body. Line ends are normalised first ("\r\n" and a lone "\r"
// become "\n"), so no text in body can end a data line early and start an SSE field of its own;
// a final newline adds no empty line. It sets Content-Type text/event-stream, writes the event
// and flushes if w can.
func PatchBody(w http.ResponseWriter, body string) error {
	return patch(w, "body", "outer", body)
}

// History is how Navigate moves the browser: Assign loads the URL (a real navigation, the
// browser's own history entry); Push and Replace leave the page loaded — the body was just
// morphed in — and only record the URL, as a new history entry (a boosted link, a redirect
// followed by the router) or in place of the current one (the answer to a back/forward, whose
// entry already exists).
type History int

const (
	Assign History = iota
	Push
	Replace
)

// HistoryHeader is the request header the boost's popstate handler sends ("replace"): the GET
// refetches a URL the history already holds, so its answer replaces the entry instead of
// pushing another (BoostTag).
const HistoryHeader = "Alder-History"

// Navigate answers a Datastar request with a navigation to url: one `datastar-patch-elements`
// event appending (selector body, mode append) a script that removes itself and runs the
// history step mode names, with url as a JSON string (so "<", ">", "&", U+2028 and U+2029
// are escaped and the URL cannot end the script). It sets Content-Type text/event-stream,
// writes the event and flushes if w can.
//
// Assign calls window.location.assign(url): how the official Datastar SDKs redirect (a 303
// would be followed inside the client's fetch, and the page at the new URL patched into the old
// one, leaving the address bar and history behind); rt/web uses it for a redirect to another
// site. Push and Replace follow a body patch of the page at url (the boosted navigation): Push
// calls history.pushState and scrolls to the top (or to the element the URL's fragment names);
// Replace calls history.replaceState and scrolls to where the entry was left (alderY, which the
// boost's click handler records in the state of the entry it leaves). Both then make <body>
// focusable (tabindex -1) and focus it, so a screen reader starts at the new page, and set
// window._alderAt, the path and query the page shows (the popstate handler compares it, so a
// fragment-only change refetches nothing).
//
// A url that rt.SafeURL refuses (javascript:, data:, a protocol-relative URL, …) is not written:
// Navigate returns an error and writes nothing, so a script URL can never run.
func Navigate(w http.ResponseWriter, url string, mode History) error {
	if !rt.SafeURL(url) {
		return fmt.Errorf("refusing to navigate to %s", rt.URLScheme(url))
	}
	b, err := json.Marshal(url)
	if err != nil {
		return err
	}
	u := string(b)
	var js string
	switch mode {
	case Push:
		js = `history.pushState(null,"",` + u + `);` + afterMorph +
			`var h=location.hash&&document.getElementById(decodeURIComponent(location.hash.slice(1)));h?h.scrollIntoView():scrollTo(0,0)`
	case Replace:
		js = `history.replaceState(history.state,"",` + u + `);` + afterMorph +
			`scrollTo(0,history.state&&history.state.alderY||0)`
	default:
		js = `window.location.assign(` + u + `)`
	}
	if mode != Assign {
		js = `(function(){` + js + `})()`
	}
	return patch(w, "body", "append", `<script data-effect="el.remove()">`+js+`</script>`)
}

// afterMorph is the part of a Push or Replace script common to both: the page's path and query
// for the popstate handler, and the focus moved to the new <body>.
const afterMorph = `window._alderAt=location.pathname+location.search;` +
	`document.body.tabIndex=-1;document.body.focus({preventScroll:true});`

// PatchTitle answers (as part of a boosted navigation, after the body patch) with one
// `datastar-patch-elements` event replacing the document's <title> (selector `head > title`,
// mode outer: a <title> inside an SVG in the body is left alone) with <title>text</title>.
// text is the title's content as a rendered page holds it (already escaped, RCDATA): it is
// written as is.
func PatchTitle(w http.ResponseWriter, text string) error {
	return patch(w, "head > title", "outer", "<title>"+text+"</title>")
}

// BoostClick is the expression of the boost's data-on:click__window attribute (BoostTag): a
// click on a link of this site — same origin, http(s), no fragment — that nothing else handled
// (not defaultPrevented), with the main button and no modifier key, on an <a href> without
// target, download or data-alder-reload, is run as a Datastar GET of the link's path and query
// instead of a page load: the scroll position is recorded in the state of the entry being left
// (alderY, which a later Replace restores) and history.scrollRestoration set to manual (the
// page comes back through a fetch, after the browser would have scrolled). Everything else —
// another site, mailto:, tel:, javascript:, an in-page #fragment, a new tab, a download, an
// opted-out link — is left to the browser. The GET carries an empty signal payload (Datastar
// adds it as ?datastar={}; rt/web drops it).
const BoostClick = `if (evt.defaultPrevented || evt.button !== 0 || evt.metaKey || evt.ctrlKey || evt.shiftKey || evt.altKey) return;` +
	` const a = evt.target.closest ? evt.target.closest('a[href]') : null;` +
	` if (!a || a.hasAttribute('target') || a.hasAttribute('download') || a.hasAttribute('data-alder-reload')) return;` +
	` const u = new URL(a.getAttribute('href'), document.baseURI);` +
	` if (u.origin !== location.origin || (u.protocol !== 'http:' && u.protocol !== 'https:') || u.hash) return;` +
	` evt.preventDefault();` +
	` history.scrollRestoration = 'manual';` +
	` history.replaceState({alderY: scrollY}, '');` +
	` @get(u.pathname + u.search, {payload: {}})`

// BoostPopstate is the expression of the boost's data-on:popstate__window attribute: on back
// or forward to an entry whose path or query differs from the page shown (window._alderAt), a
// Datastar GET of it with HistoryHeader "replace", so its answer morphs the page in and
// replaces the entry rather than pushing a new one.
const BoostPopstate = `if (location.pathname + location.search !== window._alderAt)` +
	` @get(location.pathname + location.search, {payload: {}, headers: {'` + HistoryHeader + `': 'replace'}})`

// BoostInit is the expression of the boost's data-init attribute: the page loaded is the one
// shown.
const BoostInit = `window._alderAt = location.pathname + location.search`

// BoostTag is the element in a page's <head> that boosts its links: a <meta name="alder-boost">
// whose Datastar attributes listen on window, so the listeners survive every body patch (the
// <head> is never morphed) and see every click, in the bubble phase after the page's own
// handlers (BoostClick, BoostPopstate, BoostInit).
func BoostTag() html.Node {
	return html.El("meta", []html.Attr{
		html.A("name", "alder-boost"),
		html.A("content", "links"),
		html.A("data-init", BoostInit),
		html.A("data-on:click__window", BoostClick),
		html.A("data-on:popstate__window", BoostPopstate),
	})
}

// patch writes one `datastar-patch-elements` event (PatchEvent) patching elements at selector
// in mode.
func patch(w http.ResponseWriter, selector, mode, elements string) error {
	h := w.Header()
	h.Set("Content-Type", "text/event-stream")
	h.Set("Cache-Control", "no-cache")
	if _, err := w.Write(PatchEvent(selector, mode, elements)); err != nil {
		return err
	}
	if f, ok := w.(http.Flusher); ok {
		f.Flush()
	}
	return nil
}

// PatchEvent is one `datastar-patch-elements` event, ending in its blank line, that patches
// elements at selector in mode: one `elements` data line per line of elements, line ends
// normalised as in PatchBody (an empty elements is one empty data line, which patches nothing
// in: in mode inner it clears the target). The dev server (alder run) streams these to its
// reload subscribers.
func PatchEvent(selector, mode, elements string) []byte {
	var b strings.Builder
	b.WriteString("event: datastar-patch-elements\ndata: selector " + selector + "\ndata: mode " + mode + "\n")
	for _, line := range lines(elements) {
		b.WriteString("data: elements ")
		b.WriteString(line)
		b.WriteByte('\n')
	}
	b.WriteByte('\n')
	return []byte(b.String())
}

// lines splits s into the lines of SSE data lines: "\r\n", a lone "\r" and "\n" all end a line
// (the three line ends of the SSE format), and a final line end adds no empty line. No line
// contains "\r" or "\n".
func lines(s string) []string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")
	return strings.Split(strings.TrimSuffix(s, "\n"), "\n")
}
