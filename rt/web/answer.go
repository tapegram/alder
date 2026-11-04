package web

import (
	"fmt"
	"net/http"
	"strconv"

	rtpkg "alder/rt"
	"alder/rt/datastar"
	"alder/rt/eff"
	"alder/rt/html"
)

// A write route's answer (#160), with JavaScript: the router reads the route's rt.Response
// (Response.Answer) and answers by its kind.
//   - redirect r (303): r's page served and morphed in with its title and a history push, as a
//     followed redirect is (follow); another site's URL is navigated to. Never diffed, even when
//     r is the page the browser is on; no before-view.
//   - render r (200): an in-process element GET of r with the session the write left, its view
//     patched in by its root id (deliverElement). No before-view, no page load, al-shown left
//     alone. A target that does not answer its view is never patched in (renderFailed).
//   - refresh (200): the page the browser is on (Request.ShownURL) run again and diffed against
//     the before-view the route rendered (Response.Shown): the regions the server changed.
//   - invalid t (422, in StatusHeader): the same, rendered with the route's form state (the
//     posted values, Response.State) and t as the form's error. Its update committed.
//
// The answer's toasts follow its patches. Without JavaScript only redirect is answered so far
// (a 303, its toast kept in the notice cookie for the next page).

// AnswerResponse is a write route's answer a, with before the page the browser was on, rendered
// before the update (RenderShown; nil when the route renders none), and the session to store.
// For `invalid`, the route sets the Response's State to its form's state (the posted values).
func AnswerResponse(a rtpkg.Response, before *Shown, session any) Response {
	return Response{Status: a.Code(), Answer: &a, Shown: before, Session: session}
}

// InvalidResponse answers a write whose form did not decode: as `invalid` with st, the form's
// values and field errors, and no text of its own; the route's update is not called. The route
// sets the Response's Shown to the before-view it rendered.
func InvalidResponse(st *html.FormState, session any) Response {
	a := rtpkg.Invalid("")
	return Response{Status: a.Code(), Answer: &a, State: st, Session: session}
}

// answer writes resp, a write route's answer (resp.Answer set), by its kind.
func (rt *router) answer(w http.ResponseWriter, r *http.Request, st hop, rq *Request, resp Response) {
	a := resp.Answer
	rt.setToasts(w, rq, a.Toasts())
	if a.Kind == rtpkg.RespRedirect {
		rt.answerRedirect(w, r, st, rq, resp)
		return
	}
	if !rq.Datastar {
		rt.fail(w, r, st, html.None(), fmt.Errorf("%w: a write's %s answer to a request without JavaScript is not supported yet", eff.ErrInternal, kindName(a.Kind)), nil)
		return
	}
	if err := rt.setSession(w, r, rq, resp.Session); err != nil {
		rt.fail(w, r, st, html.None(), err, nil)
		return
	}
	if a.Kind == rtpkg.RespRender {
		rt.answerRender(w, r, st, rq, resp)
		return
	}
	rt.answerShown(w, r, st, rq, resp)
}

// kindName is k as the Prelude names it, for a log line.
func kindName(k rtpkg.ResponseKind) string {
	switch k {
	case rtpkg.RespRedirect:
		return "redirect"
	case rtpkg.RespRender:
		return "render"
	case rtpkg.RespRefresh:
		return "refresh"
	}
	return "invalid"
}

// setToasts makes toasts the answer's: patched in after its patches (notify) under Datastar, or,
// without it, the first one kept for the page a redirect goes to (the notice cookie).
func (rt *router) setToasts(w http.ResponseWriter, rq *Request, toasts []rtpkg.Toast) {
	tw, ok := w.(*tracker)
	if !ok || len(toasts) == 0 {
		return
	}
	if !rq.Datastar {
		tw.alert, tw.kind = toasts[0].Text, toasts[0].Kind
		return
	}
	tw.notice, tw.kind, tw.more = toasts[0].Text, toasts[0].Kind, toasts[1:]
}

// addToast adds a toast after the answer's own.
func (t *tracker) addToast(kind rtpkg.NoticeKind, text string) {
	if t.notice == "" {
		t.notice, t.kind = text, kind
		return
	}
	t.more = append(t.more, rtpkg.Toast{Kind: kind, Text: text})
}

// answerRedirect answers `redirect r`: a 303 to r's URL, written as a handler's redirect is
// (write): followed and morphed in under Datastar, or a 303 with the toast stored.
func (rt *router) answerRedirect(w http.ResponseWriter, r *http.Request, st hop, rq *Request, resp Response) {
	rt.write(w, r, st, rq, RedirectResponse(rtpkg.RouteURL(resp.Answer.Target), resp.Session))
}

// answerRender answers `render r`: an element GET of r's URL served in-process (serve, with the
// session the write left) and patched in by its root id (deliverElement). A URL no GET route of
// this app matches (or whose query does not parse) is answered as a target that is not found
// (renderFailed): never the 404 page morphed into the body.
func (rt *router) answerRender(w http.ResponseWriter, r *http.Request, st hop, rq *Request, resp Response) {
	loc := rtpkg.RouteURL(resp.Answer.Target)
	u, err := r.URL.Parse(loc)
	if err != nil || !rt.followable(loc) {
		rt.fail(w, r, st, html.None(), fmt.Errorf("%w: render of %s, which is not a page of this app", eff.ErrInternal, strconv.Quote(loc)), nil)
		return
	}
	r2 := getOf(r, u, true)
	segs, ok := splitPath(u.EscapedPath())
	if ok && badQuery(u.RawQuery) == "" {
		hit, _, _ := rt.find(http.MethodGet, segs)
		ok = hit != nil
	}
	if !ok {
		rt.renderFailed(w, r2, rq, NotFoundResponse(html.None()))
		return
	}
	next := hop{origin: st.origin, gets: st.gets + 1, has: true, render: true}
	next.session, next.enc = rt.sessionLeft(rq, resp)
	rt.serve(w, r2, next)
}

// renderOK reports whether resp, the answer of a `render` target's GET, is delivered as an
// element fetch's is (deliverElement): its view (200), or a redirect, which navigates.
func renderOK(resp Response) bool {
	return resp.Raw == nil && (resp.Status == http.StatusOK || resp.Status == http.StatusSeeOther)
}

// renderFailed answers a `render` whose target (r, its GET) did not answer its view: no element
// is patched — the failure page never replaces the element — and an error toast follows the
// answer's own: "Not found" for a 404 (the row was deleted meanwhile), else "Something went
// wrong", with the target's error logged.
func (rt *router) renderFailed(w http.ResponseWriter, r *http.Request, rq *Request, resp Response) {
	text := "Not found"
	answer := "render: not found, no element patched"
	if resp.Raw != nil || resp.Status != http.StatusNotFound {
		text, answer = "Something went wrong", "render: error, no element patched"
		why := resp.Err
		if why == nil {
			why = fmt.Errorf("answered status %d", resp.Status)
		}
		if resp.Raw != nil {
			why = fmt.Errorf("answered a file, not a view")
		}
		rt.logReq(w, "error", line(w, r, fmt.Sprintf("render: %v (no element patched)", why)))
	}
	if tw, ok := w.(*tracker); ok {
		tw.setAnswer(answer)
		if tw.detail == "" && resp.Err != nil {
			tw.detail = resp.Err.Error()
		}
		tw.addToast(rtpkg.NoticeError, text)
	}
	h := w.Header()
	h.Set("Content-Type", "text/event-stream")
	h.Set("Cache-Control", "no-cache")
	w.WriteHeader(http.StatusOK)
	rt.notify(w)
	rt.patchPanel(w, http.StatusOK)
}

// answerShown answers `refresh` and `invalid`: the page the browser is on (ShownURL) run again
// with the session the write left — for invalid, rendered with the route's form state and the
// answer's text as the form's error — and diffed against the before-view (resp.Shown). A URL no
// GET route matches is answered with its plain HTTP, a 303 there: a navigation. A shown page
// that now answers something else than its view (a 404, a redirect) is written as a navigation
// to it would be.
func (rt *router) answerShown(w http.ResponseWriter, r *http.Request, st hop, rq *Request, resp Response) {
	a := resp.Answer
	var state *html.FormState
	if a.Kind == rtpkg.RespInvalid {
		state = &html.FormState{}
		if resp.State != nil {
			s := *resp.State
			state = &s
		}
		if a.Text != "" {
			state.FormError = a.Text
		}
	}
	u := rq.ShownURL()
	session, enc := rt.sessionLeft(rq, resp)
	tw, _ := w.(*tracker)
	n := 0
	if rq.Env != nil {
		n = len(rq.Env.Trace)
	}
	sub, got, ok := rt.runShown(rq, u, session, enc, state)
	if tw != nil && rq.Env != nil {
		tw.steps += len(rq.Env.Trace) - n
	}
	if !ok {
		if tw != nil {
			tw.setAnswer("navigate " + rt.maskRef(u))
		}
		if err := datastar.Navigate(w, u, datastar.Assign); err != nil {
			rt.logReq(w, "error", line(w, r, fmt.Sprintf("writing the navigation: %v", err)))
		}
		return
	}
	if got.Raw != nil || got.Status != http.StatusOK {
		rt.write(w, r, st, sub, got)
		return
	}
	var before *html.Rendered
	if sh := resp.Shown; sh != nil && sh.Status == http.StatusOK && sh.URL == u {
		before = &sh.Rendered
	}
	status := a.Code()
	if status != http.StatusOK {
		w.Header().Set(StatusHeader, strconv.Itoa(status))
	}
	if tw != nil {
		tw.setAnswer(kindName(a.Kind))
	}
	rt.patchRegions(w, r, rq, before, html.RenderTracked(got.Node, got.State), status)
}
