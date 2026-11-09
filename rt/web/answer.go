package web

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"

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
// The answer's toasts follow its patches.
//
// Without JavaScript (answerPlain) each is plain HTTP: redirect r a 303 to r; refresh and render
// r a 303 to the page the browser is on (Request.ShownURL: Referer, else "/") — a partial has
// nowhere to go without a page; invalid t a 422 with that page rendered, the form kept and t
// shown on it. A 303's toast is kept in the notice cookie for the next page; a 422 page shows it
// as an alert.
//
// From a tab of another build (hop.stale: its Alder-Build is not BuildID) any write's answer is a
// reload instead (staleReload), never a patch.

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
	if err := rt.setSession(w, r, rq, resp.Session); err != nil {
		rt.fail(w, r, st, html.None(), err, nil)
		return
	}
	if !rq.Datastar {
		rt.answerPlain(w, r, st, rq, resp)
		return
	}
	if a.Kind == rtpkg.RespRender {
		rt.answerRender(w, r, st, rq, resp)
		return
	}
	rt.answerShown(w, r, st, rq, resp)
}

// targetProblem is why a, a write's answer, cannot be delivered, "" when it can (#160 ruling
// R10). The checker refuses a write route written in place (E0045), but not one held in a
// variable or made by a helper:
//   - `redirect` goes to a GET: a named GET route or a Url.external URL (no name, Method "GET":
//     isExternal, a full navigation); a route of another method is refused (an empty Method
//     is a GET);
//   - `render` patches in a named GET route's view only: an external URL has no element to
//     patch, and is refused, as a route of another method is.
func targetProblem(a rtpkg.Response) string {
	t := a.Target
	switch a.Kind {
	case rtpkg.RespRedirect:
		if t.Method != "" && t.Method != http.MethodGet {
			return targetName(t) + " is a " + t.Method + " route, not a GET route of this app"
		}
	case rtpkg.RespRender:
		switch {
		case t.Name == "":
			return "a Route with no name (Url.external makes one) is not a GET route of this app"
		case t.Method != "" && t.Method != http.MethodGet:
			return t.Name + " is a " + t.Method + " route, not a GET route of this app"
		}
	}
	return ""
}

// viaName is how the Patches tab names the answer `kind` to target: "render Row", the route's
// own name (the part after its module); just the kind for a route with no name.
func viaName(kind string, target rtpkg.Route) string {
	if target.Name == "" {
		return kind
	}
	return kind + " " + target.Name[strings.LastIndex(target.Name, ".")+1:]
}

// targetName is r's name for a log line: its Name, else its URL.
func targetName(r rtpkg.Route) string {
	if r.Name != "" {
		return r.Name
	}
	return strconv.Quote(rtpkg.RouteURL(r))
}

// isExternal reports whether r is a Url.external URL (no name, Method "GET"), not a route of
// this app.
func isExternal(r rtpkg.Route) bool { return r.Name == "" && r.Method == http.MethodGet }

// badTarget answers a write whose answer's target is not a GET route (targetProblem: why): never
// followed or patched in. It is logged and answered as an error: without JavaScript the 500 page
// (App.error); under Datastar no patch, the error toast and Alder-Status 500, as a `render`
// whose target failed (renderFailed). The update committed, so the session it left is stored.
func (rt *router) badTarget(w http.ResponseWriter, r *http.Request, st hop, rq *Request, resp Response, why string) {
	a := resp.Answer
	err := fmt.Errorf("%w: %s to %s: %s (nothing delivered)", eff.ErrInternal, kindName(a.Kind), strconv.Quote(rtpkg.RouteURL(a.Target)), why)
	if serr := rt.setSession(w, r, rq, resp.Session); serr != nil {
		rt.logReq(w, "error", line(w, r, serr.Error()))
	}
	if !rq.Datastar {
		rt.fail(w, r, st, html.None(), err, nil)
		return
	}
	rt.logReq(w, "error", line(w, r, err.Error()))
	if tw, ok := w.(*tracker); ok {
		tw.setAnswer("error")
		tw.detail = err.Error()
		tw.addToast(rtpkg.NoticeError, "Something went wrong")
	}
	h := w.Header()
	h.Set("Content-Type", "text/event-stream")
	h.Set("Cache-Control", "no-cache")
	h.Set(StatusHeader, strconv.Itoa(http.StatusInternalServerError))
	w.WriteHeader(http.StatusOK)
	rt.notify(w)
	rt.patchPanel(w, http.StatusInternalServerError)
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
// without it, kept for the page a redirect goes to (the notice cookie).
func (rt *router) setToasts(w http.ResponseWriter, rq *Request, toasts []rtpkg.Toast) {
	tw, ok := w.(*tracker)
	if !ok || len(toasts) == 0 {
		return
	}
	if !rq.Datastar {
		tw.alerts = toasts
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
// (write): followed and morphed in under Datastar, or a 303 with the toast stored. A Url.external
// target (isExternal) is a real navigation under Datastar (navigateAway: the toast kept in the
// notice cookie), even to a path of this app, never a morph (ruling R10); without JavaScript the
// same 303. An unsafe URL (Url.external's "#") is refused as a handler's redirect is.
func (rt *router) answerRedirect(w http.ResponseWriter, r *http.Request, st hop, rq *Request, resp Response) {
	loc, ext := rtpkg.RouteURL(resp.Answer.Target), isExternal(resp.Answer.Target)
	if ext && (strings.HasPrefix(loc, "#") || !rtpkg.SafeURL(loc)) {
		rt.write(w, r, st, rq, QueryRedirectResponse(loc)) // a 500: never a navigation in place
		return
	}
	if tw, ok := w.(*tracker); ok && !ext {
		tw.via = viaName("redirect", resp.Answer.Target)
	}
	if !ext || !rq.Datastar {
		rt.write(w, r, st, rq, RedirectResponse(loc, resp.Session))
		return
	}
	if err := rt.setSession(w, r, rq, resp.Session); err != nil {
		rt.fail(w, r, st, html.None(), err, nil)
		return
	}
	rt.navigateAway(w, r, "redirect", loc)
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
		rt.renderFailed(w, r2, rq, NotFoundResponse(html.None()), "render")
		return
	}
	if tw, ok := w.(*tracker); ok {
		tw.via = viaName("render", resp.Answer.Target)
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
// wrong", with the target's error logged. It also answers an element fetch whose route answered
// 404 (deliverElement): what names the request in the dev panel, "render" or "element fetch".
func (rt *router) renderFailed(w http.ResponseWriter, r *http.Request, rq *Request, resp Response, what string) {
	text := "Not found"
	answer := what + ": not found, no element patched"
	if resp.Raw != nil || resp.Status != http.StatusNotFound {
		text, answer = "Something went wrong", what+": error, no element patched"
		why := resp.Err
		if why == nil {
			why = fmt.Errorf("answered status %d", resp.Status)
		}
		if resp.Raw != nil {
			why = fmt.Errorf("answered a file, not a view")
		}
		rt.logReq(w, "error", line(w, r, fmt.Sprintf("%s: %v (no element patched)", what, why)))
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
	u, sub, got, ok := rt.shownAfter(w, rq, resp)
	if !ok {
		rt.navigateAway(w, r, "navigate", u)
		return
	}
	if got.Raw != nil || got.Status != http.StatusOK {
		rt.write(w, r, st, sub, got)
		return
	}
	tw, _ := w.(*tracker)
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

// invalidState is the form state the page of an `invalid` answer (resp.Answer) is rendered with:
// the route's (resp.State, the posted values), with the answer's text as the form's error; nil
// for any other answer.
func invalidState(resp Response) *html.FormState {
	a := resp.Answer
	if a.Kind != rtpkg.RespInvalid {
		return nil
	}
	state := &html.FormState{}
	if resp.State != nil {
		s := *resp.State
		state = &s
	}
	if a.Text != "" {
		state.FormError = a.Text
	}
	return state
}

// shownAfter runs the page the browser is on (u, Request.ShownURL) again for the answer resp,
// with the session the write left and invalidState, its steps counted as the write's; ok is
// false when no GET route matches u (runShown).
func (rt *router) shownAfter(w http.ResponseWriter, rq *Request, resp Response) (u string, sub *Request, got Response, ok bool) {
	u = rq.ShownURL()
	session, enc := rt.sessionLeft(rq, resp)
	n := 0
	if rq.Env != nil {
		n = len(rq.Env.Trace)
	}
	sub, got, ok = rt.runShown(rq, u, session, enc, invalidState(resp))
	if tw, isT := w.(*tracker); isT && rq.Env != nil {
		tw.steps += len(rq.Env.Trace) - n
	}
	return u, sub, got, ok
}

// answerPlain answers refresh, render and invalid to a request without JavaScript (the session
// already set): refresh and render r a 303 to the page the browser is on (its Referer, through
// rt.BackURL; render's partial has nowhere to go without a page); invalid t a 422 with that page
// rendered as a document, the form's values kept and t shown on it. A page no GET route matches is a 303 there; one that now answers something else
// than its view (a 404, a redirect) is written as a GET of it would be.
func (rt *router) answerPlain(w http.ResponseWriter, r *http.Request, st hop, rq *Request, resp Response) {
	if resp.Answer.Kind != rtpkg.RespInvalid {
		rt.write(w, r, st, rq, RedirectResponse(rq.ShownURL(), nil))
		return
	}
	u, sub, got, ok := rt.shownAfter(w, rq, resp)
	if !ok {
		rt.write(w, r, st, rq, RedirectResponse(u, nil))
		return
	}
	if got.Raw != nil || got.Status != http.StatusOK {
		rt.write(w, r, st, sub, got)
		return
	}
	if tw, isT := w.(*tracker); isT {
		tw.setAnswer("invalid")
	}
	rt.document(w, http.StatusUnprocessableEntity, html.RenderWith(got.Node, got.State))
}

// staleReload answers a Datastar write from a tab of another build (plan call 9): the write ran
// (its decoders guard it), and its answer resp is a reload, never a patch — the tab's <head> is
// that build's and a morph would keep it. A redirect (`redirect r`, a handler's 303) navigates
// to its target; anything else that succeeded reloads the page the browser is on
// (Request.ShownURL: Alder-Page through rt.BackURL). The session is stored and the toasts kept
// in the notice cookie, so the page loaded shows them. A failure (404, 500, a file) is
// answered as usual.
func (rt *router) staleReload(w http.ResponseWriter, r *http.Request, st hop, rq *Request, resp Response) bool {
	loc := rq.ShownURL()
	switch {
	case resp.Answer != nil:
		rt.setToasts(w, rq, resp.Answer.Toasts())
		if resp.Answer.Kind == rtpkg.RespRedirect {
			loc = rtpkg.RouteURL(resp.Answer.Target)
		}
	case resp.Raw != nil || resp.Status != http.StatusOK && resp.Status != http.StatusSeeOther:
		return false
	case resp.Status == http.StatusSeeOther:
		loc = resp.Location
	}
	if err := rt.setSession(w, r, rq, resp.Session); err != nil {
		rt.fail(w, r, st, html.None(), err, nil)
		return true
	}
	rt.navigateAway(w, r, "reload (another build)", loc)
	return true
}

// navigateAway answers a Datastar request with a real navigation to loc (datastar.Assign), the
// answer's toasts kept in the notice cookie for the page loaded (a toast patched into the page
// being left would be lost with it). what names the answer in the access line.
func (rt *router) navigateAway(w http.ResponseWriter, r *http.Request, what, loc string) {
	if tw, isT := w.(*tracker); isT {
		tw.setAnswer(what + " " + rt.maskRef(loc))
		if tw.notice != "" {
			rt.storeNotices(w, r, append([]rtpkg.Toast{{Kind: tw.kind, Text: tw.notice}}, tw.more...))
			tw.notice, tw.more = "", nil
		}
	}
	if err := datastar.Navigate(w, loc, datastar.Assign); err != nil {
		rt.logReq(w, "error", line(w, r, fmt.Sprintf("writing the navigation: %v", err)))
	}
}
