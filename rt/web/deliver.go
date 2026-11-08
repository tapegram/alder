package web

import (
	"fmt"
	"net/http"

	"alder/rt/datastar"
	"alder/rt/html"
)

// Delivery decided by the request (#160). A GET route's view is answered to:
//   - a plain request (the address bar, no JavaScript) as a document: <html>, the head tags, the
//     dev panel (write, document); a partial's URL so is a bare document, status 200;
//   - a boosted link (a Datastar GET without datastar.FetchHeader) as the morph of the body, its
//     title and a history step, as before (ServeHTTP sets the hop's URL; write, morph);
//   - an element fetch (a Datastar GET with datastar.FetchHeader "1") as one
//     `datastar-patch-elements` event replacing the element with its root's id (mode outer):
//     deliverElement. No body, no title, no history step.

// elementFetch reports whether r is an element fetch: a Datastar GET (or HEAD) carrying
// datastar.FetchHeader "1".
func elementFetch(r *http.Request) bool {
	return (r.Method == http.MethodGet || r.Method == http.MethodHead) && datastar.IsDatastar(r) &&
		r.Header.Get(datastar.FetchHeader) == "1"
}

// deliverElement answers an element fetch with resp, the GET route's answer. A 200 view is
// patched in by its root's id (html.RootID: several roots morph in together, the first one's id
// the target). A view without one is a runtime error: logged at error naming the route, noted
// in the dev panel, and answered with an empty event stream, so the page is left as it was. A 404
// patches nothing and toasts "Not found" (renderFailed). Any other answer (a redirect, a 500, a
// file) is written as write writes it for a Datastar request that is not a navigation (st.url
// ""): a redirect to a page of this app is followed and morphed in with a history push, a
// failure's page is morphed into the body.
//
// It is also how a write's `render r` is delivered (an in-process element GET of r, with the
// session the write left in st, st.render set): w is the write's own answer, and a target that
// answers neither its view nor a redirect is never patched in (renderFailed).
func (rt *router) deliverElement(w http.ResponseWriter, r *http.Request, st hop, rq *Request, resp Response) {
	if st.render && !renderOK(resp) {
		rt.renderFailed(w, r, rq, resp, "render")
		return
	}
	if resp.Raw == nil && resp.Status == http.StatusNotFound {
		// the route's parameters did not decode or its load found no row: the not-found page
		// never replaces the element, a toast says so, as for a `render` of a row that is gone
		rt.renderFailed(w, r, rq, resp, "element fetch")
		return
	}
	if resp.Raw != nil || resp.Status != http.StatusOK {
		rt.write(w, r, st, rq, resp)
		return
	}
	if err := rt.setSession(w, r, rq, resp.Session); err != nil {
		rt.fail(w, r, st, html.None(), err, nil)
		return
	}
	tw, _ := w.(*tracker)
	id, ok := html.RootID(resp.Node)
	if !ok {
		rt.logReq(w, "error", line(w, r, fmt.Sprintf("route %s answered an element fetch with a view whose first root element has no id: "+
			"no element patched (an element fetch replaces the element with its view's root id)", rt.routeOf(r))))
		if tw != nil {
			tw.setAnswer("no element patched: the view's root has no id")
		}
		h := w.Header()
		h.Set("Content-Type", "text/event-stream")
		h.Set("Cache-Control", "no-cache")
		w.WriteHeader(http.StatusOK)
		rt.patchPanel(w, http.StatusOK)
		return
	}
	if tw != nil {
		tw.setAnswer("element #" + id)
		tw.patches = append(tw.patches, tw.patchName("#"+id))
	}
	if err := datastar.PatchRegion(w, id, html.RenderWith(resp.Node, resp.State)); err != nil {
		rt.logReq(w, "error", line(w, r, fmt.Sprintf("writing the patch: %v", err)))
	}
	rt.notify(w)
	rt.patchPanel(w, http.StatusOK)
}

// routeOf names the GET route r is served by, "GET <pattern>", for a log line; its path when none
// matches.
func (rt *router) routeOf(r *http.Request) string {
	if segs, ok := pathSegments(r); ok {
		if hit, _, _ := rt.find(http.MethodGet, segs); hit != nil {
			return http.MethodGet + " " + hit.pattern
		}
	}
	return http.MethodGet + " " + r.URL.Path
}
