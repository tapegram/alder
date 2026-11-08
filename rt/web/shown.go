package web

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"

	rtpkg "alder/rt"
	"alder/rt/datastar"
	"alder/rt/html"
)

// The page the browser is on (#160). A write route does not know which page sent it (a Toggle
// may be used from the index and from a detail page), and `refresh`, `invalid` and the
// before-view need it: the page's post helper sends its path and query as html.PageHeader, and
// a request without JavaScript has Referer. Both are read through rt.BackURL, so the page is
// always a path of this app. It is resolved to a GET route with the matcher a GET uses, and that
// route's Handle is run in-process (runShown).
//
// The diff (patchRegions) sends the regions the server changed between the before-view and the
// after-view. Since #160 the shown hash the client sends (html.ShownHeader) no longer gates it:
// an element fetch or a `render` changes the DOM without the server rendering the page, so the
// client's al-shown is stale by design after one, and the regions the server did not change are
// left as the browser has them. The hash keeps one job: before equal to after sends no element
// event. A request without the header (a post made before the boost ran) is answered with the
// whole body.

// Shown is the page the browser is on, rendered: its URL (Request.ShownURL), the Pattern of the
// GET route that matched it, the status that route answered and, for a 200, its render with the
// regions (html.RenderTracked). A write route that may answer `refresh` or `invalid` renders it
// before its update (Request.RenderShown) and hands it to the router as Response.Shown, which
// diffs the after-view against it; a write route whose form has selects with `options={…}`
// checks the posted values against it (CheckOptions).
type Shown struct {
	URL      string
	Pattern  string
	Status   int
	Rendered html.Rendered
	node     html.Node // the view rendered, for a 200 (CheckOptions)
}

// ShownURL is the path and query of the page the browser is on: html.PageHeader, else Referer,
// through rt.BackURL ("/" when neither is a path of this app).
func (rq *Request) ShownURL() string {
	if rq.HTTP == nil {
		return "/"
	}
	return shownURLOf(rq.HTTP)
}

// shownURLOf is the page r was sent from: html.PageHeader, else Referer, through rt.BackURL.
func shownURLOf(r *http.Request) string {
	raw := r.Header.Get(html.PageHeader)
	if raw == "" {
		raw = r.Referer()
	}
	return rtpkg.BackURL(raw)
}

// RenderShown runs the GET route of the page the browser is on (ShownURL) in-process, with the
// request's session and its view rendered with st (Request.FormState; nil: the plain page), and
// renders its answer with the regions. The route's answer is returned as it is; Shown is nil
// when no GET route matches the URL (a stale build's path, a file page's), and its Rendered is
// empty when the route did not answer its view (a 404, a redirect). A 500 (a panic in the route
// included) is logged: the write goes on, and its answer morphs the whole body.
func (rq *Request) RenderShown(st *html.FormState) (*Shown, Response) {
	if rq.rt == nil {
		return nil, NotFoundResponse(html.None())
	}
	u := rq.ShownURL()
	sub, resp, ok := rq.rt.runShown(rq, u, rq.Session, rq.sessionEnc, st)
	if !ok {
		return nil, NotFoundResponse(html.None())
	}
	if resp.Status == http.StatusInternalServerError {
		// the before-view is lost, not the write: its answer morphs the whole body
		rq.rt.logReq(rq.w, "error", line(rq.w, sub.HTTP, fmt.Sprintf("rendering the page before the write: %v", resp.Err)))
	}
	sh := &Shown{URL: u, Pattern: sub.pattern, Status: resp.Status}
	if resp.Raw == nil && resp.Status == http.StatusOK {
		sh.Rendered, sh.node = html.RenderTracked(resp.Node, resp.State), resp.Node
	}
	return sh, resp
}

// runShown runs the GET route that matches raw (a path and query, rt.BackURL's) for parent, a
// write's request, with session (enc its encoding) and st as its FormState; ok is false when no
// GET route that is not a file page's matches. The sub-request shares parent's context and
// headers; its Env is a fresh one, as a GET request's (newEnv), so no write-scoped state (a
// db.Narrow or db.Enter, SessionPut, a transaction) reaches the page and none of the page's
// reaches the write. Only the request's log, recorder and statement counts are shared, and its
// steps are appended to parent's trace.
// A panic in the route is answered as a 500 (Err "panic: …").
func (rt *router) runShown(parent *Request, raw string, session any, enc []byte, st *html.FormState) (sub *Request, resp Response, ok bool) {
	u, err := url.Parse(raw)
	if err != nil {
		return nil, Response{}, false
	}
	segs, ok := splitPath(u.EscapedPath())
	if !ok {
		return nil, Response{}, false
	}
	hit, params, _ := rt.find(http.MethodGet, segs)
	if hit == nil || hit.file {
		return nil, Response{}, false
	}
	tw, _ := parent.w.(*tracker)
	env := rt.newEnv(parent.HTTP.Context(), session, tw)
	if parent.Env != nil {
		env.Rec = parent.Env.Rec // the dev panel's trace of the request goes on
	}
	sub = &Request{
		Ctx: parent.Ctx, HTTP: getOf(parent.HTTP, u, false), Params: params, Query: u.Query(),
		Session: session, Env: env, Datastar: parent.Datastar, Dev: parent.Dev, Login: parent.Login,
		FormState: st, sessionEnc: enc, rt: rt, w: parent.w, pattern: hit.pattern,
	}
	defer func() {
		if parent.Env != nil {
			parent.Env.Trace = append(parent.Env.Trace, env.Trace...)
		}
		if p := recover(); p != nil {
			resp, ok = ErrorResponse(html.None(), fmt.Errorf("panic: %v", p)), true
		}
	}()
	return sub, hit.handle(sub), true
}

// getOf is a GET of u made from r, as the router serves it in-process (a followed redirect, the
// shown page, a `render` target): r's context and headers, no body, and datastar.FetchHeader "1"
// when fetch is set (an element fetch), else none (a navigation).
func getOf(r *http.Request, u *url.URL, fetch bool) *http.Request {
	r2 := r.Clone(r.Context())
	r2.Method, r2.URL, r2.RequestURI = http.MethodGet, u, u.RequestURI()
	r2.Body, r2.ContentLength, r2.Form, r2.PostForm = http.NoBody, 0, nil, nil
	r2.Header.Del("Content-Type")
	r2.Header.Del("Content-Length")
	r2.Header.Del(datastar.FetchHeader)
	if fetch {
		r2.Header.Set(datastar.FetchHeader, "1")
	}
	return r2
}

// sessionLeft is the session a write left (resp's, else the request's) and its encoding: the one
// a GET served after it in the same answer runs with.
func (rt *router) sessionLeft(rq *Request, resp Response) (any, []byte) {
	if resp.Session == nil {
		return rq.Session, rq.sessionEnc
	}
	var enc []byte
	if rt.opts.Session != nil {
		enc, _ = rt.opts.Session.Encode(resp.Session)
	}
	return resp.Session, enc
}

// patchRegions answers a Datastar request with a re-rendered page, after (rendered with its
// regions when before is set): the regions that differ from before (regions), or the whole body
// morphed in when that is not safe. Options.Rendered gets after's body either way. status is the
// panel's; the caller sets StatusHeader.
func (rt *router) patchRegions(w http.ResponseWriter, r *http.Request, rq *Request, before *html.Rendered, after html.Rendered, status int) {
	body := bodyOf(after.Page)
	if rt.opts.Rendered != nil {
		rt.opts.Rendered(rq, body)
	}
	tw, _ := w.(*tracker)
	note := func(p string) {
		if tw != nil {
			tw.patches = append(tw.patches, tw.patchName(p))
		}
	}
	patches, ok := rt.regions(w, r, before, after, body)
	var err error
	switch {
	case !ok:
		err = datastar.PatchBody(w, rt.shellBody(w, rt.dev(withShown(body))))
		note("body")
	case len(patches) == 0:
		// nothing changed: no element event, and the shown hash stays what it was
		h := w.Header()
		h.Set("Content-Type", "text/event-stream")
		h.Set("Cache-Control", "no-cache")
		note("none")
	default:
		for _, p := range patches {
			if err == nil {
				err = datastar.PatchRegion(w, strings.TrimPrefix(p.Selector, "#"), p.Elements)
			}
			note(p.Selector)
		}
		// the shown hash of the server's render, last: the page's al-shown says which render it
		// last received whole or in part (a full-body delivery refreshes it too)
		if err == nil {
			err = datastar.PatchShown(w, html.Hash(body))
		}
		note("shown")
	}
	if err != nil {
		rt.logReq(w, "error", line(w, r, fmt.Sprintf("writing the patch: %v", err)))
	}
	rt.notify(w)
	rt.patchPanel(w, status)
}

// regions is the region patches from before to after (html.Fragments), body after's <body>; ok
// is false when only a body morph is safe: there is no before-view (none rendered, or it
// panicked), the request carries no shown hash (html.ShownHeader: a post made before the boost
// ran), a toast needs the implicit <al-toasts> region the page does not have, or a change lies
// outside every region. The client's shown hash is not compared with before's (see above).
func (rt *router) regions(w http.ResponseWriter, r *http.Request, before *html.Rendered, after html.Rendered, body string) (patches []html.Patch, ok bool) {
	if before == nil || r.Header.Get(html.ShownHeader) == "" {
		return nil, false
	}
	if tw, isT := w.(*tracker); isT && tw.notice != "" && !after.Toasts {
		return nil, false
	}
	if bodyOf(before.Page) == body {
		return []html.Patch{}, true
	}
	ps := html.Fragments(*before, after)
	for _, p := range ps {
		if p.Selector == "body" {
			return nil, false
		}
	}
	return ps, true
}
