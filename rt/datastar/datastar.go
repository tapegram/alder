// Package datastar serves the vendored Datastar client script and writes the server-sent events
// the Alder web runtime answers Datastar requests with. The script is embedded, so an Alder
// binary needs no CDN; Version names the vendored release (see VERSION).
package datastar

import (
	_ "embed"
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
// i.e. whether it should be answered with PatchBody rather than a full page or a redirect.
func IsDatastar(r *http.Request) bool {
	return r.Header.Get("Datastar-Request") == "true"
}

// PatchBody answers a Datastar request with one `datastar-patch-elements` event that replaces
// the page's <body> (selector body, mode outer) with body, the rendered <body> element: one
// `elements` data line per line of body ("\r" stripped; a final newline adds no empty line).
// It sets Content-Type text/event-stream, writes the event and flushes if w can.
func PatchBody(w http.ResponseWriter, body string) error {
	h := w.Header()
	h.Set("Content-Type", "text/event-stream")
	h.Set("Cache-Control", "no-cache")
	var b strings.Builder
	b.WriteString("event: datastar-patch-elements\ndata: selector body\ndata: mode outer\n")
	for _, line := range strings.Split(strings.TrimSuffix(body, "\n"), "\n") {
		b.WriteString("data: elements ")
		b.WriteString(strings.TrimSuffix(line, "\r"))
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
