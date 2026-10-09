// Package datastar serves the vendored Datastar client script and writes the server-sent events
// the Alder web runtime answers Datastar requests with. The script is embedded, so an Alder
// binary needs no CDN; Version names the vendored release (see VERSION).
package datastar

import (
	_ "embed"
	"encoding/json"
	"net/http"
	"strings"

	"alder/rt/html"
)

// Version is the vendored Datastar release; it must equal the contents of VERSION and the
// version in datastar.js's first line (checked by the tests).
const Version = "v1.0.0"

// ScriptPath is where Handler is mounted by rt/web.
const ScriptPath = "/_alder/datastar.js"

//go:embed datastar.js
var script []byte

// Handler serves the embedded datastar.js with a long-lived, immutable cache policy (the
// content changes only with Version). Mount it at ScriptPath.
func Handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("Content-Type", "application/javascript; charset=utf-8")
		h.Set("Cache-Control", "public, max-age=31536000, immutable")
		_, _ = w.Write(script)
	})
}

// ScriptTag is the <script type="module" src="/_alder/datastar.js"></script> element a page's
// <head> carries.
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

// Navigate answers a Datastar request with a navigation to url: one `datastar-patch-elements`
// event appending (selector body, mode append) a script that removes itself and calls
// window.location.assign(url), with url as a JSON string (so "<", ">", "&", U+2028 and U+2029
// are escaped and the URL cannot end the script). This is how the official Datastar SDKs
// redirect: a 303 would be followed inside the client's fetch, and the page at the new URL
// patched into the old one, leaving the address bar and history behind. It sets Content-Type
// text/event-stream, writes the event and flushes if w can.
func Navigate(w http.ResponseWriter, url string) error {
	u, err := json.Marshal(url)
	if err != nil {
		return err
	}
	return patch(w, "body", "append", `<script data-effect="el.remove()">window.location.assign(`+string(u)+`)</script>`)
}

// patch writes one `datastar-patch-elements` event patching elements at selector in mode.
func patch(w http.ResponseWriter, selector, mode, elements string) error {
	h := w.Header()
	h.Set("Content-Type", "text/event-stream")
	h.Set("Cache-Control", "no-cache")
	var b strings.Builder
	b.WriteString("event: datastar-patch-elements\ndata: selector " + selector + "\ndata: mode " + mode + "\n")
	for _, line := range lines(elements) {
		b.WriteString("data: elements ")
		b.WriteString(line)
		b.WriteByte('\n')
	}
	b.WriteByte('\n')
	if _, err := w.Write([]byte(b.String())); err != nil {
		return err
	}
	if f, ok := w.(http.Flusher); ok {
		f.Flush()
	}
	return nil
}

// lines splits s into the lines of SSE data lines: "\r\n", a lone "\r" and "\n" all end a line
// (the three line ends of the SSE format), and a final line end adds no empty line. No line
// contains "\r" or "\n".
func lines(s string) []string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")
	return strings.Split(strings.TrimSuffix(s, "\n"), "\n")
}
