package web

import (
	"context"
	"net/http"
	"net/url"

	rtpkg "alder/rt"
	"alder/rt/eff"
	"alder/rt/html"
)

// Route is one page (GET) or message (POST) endpoint. Pattern is a path such as "/" or
// "/todos/{id}" ({name} matches one non-empty segment); messages use "/_m/<Module>/<Msg>".
// Handle gets the request with the matched path params and returns a Response.
type Route struct {
	Method  string // "GET" | "POST"
	Pattern string
	File    bool // a file page's GET: answered raw; never morphed or followed
	Handle  func(rq *Request) Response
}

// Request is what a route handler sees. The router builds a fresh one (and a fresh Env) for
// every HTTP request.
type Request struct {
	Ctx      context.Context
	HTTP     *http.Request
	Params   map[string]string // path params, unescaped
	Query    url.Values
	Form     url.Values // POST body (parsed when url-encoded, else empty); nil for GET
	Session  any        // decoded from the cookie via Options.Session; its Default when absent or invalid
	Env      *eff.Env   // Ctx = HTTP.Context(); Store, Caps, Now, Log and Session set
	Datastar bool       // sent by the Datastar client (header Datastar-Request: true)
	Dev      bool       // Options.Dev
	Login    string     // Options.Login: where a failed `requires` redirects
	Notice   string     // the toast text of a Noticed answer (set by the generated update route)
	// Model is the model a page's GET route rendered (set by the generated route, nil for any
	// other request): Options.Served hands it to an Alder test's browser, so Test.get gives the
	// model the page showed without running its load again.
	Model any

	sessionEnc []byte // the codec's encoding of Session: a response's session equal to it is not stored again
}

// Response is what a handler returns; build it with the constructors below. Status decides its
// kind: 200 Page, 303 Redirect, 404 NotFound, 500 Error, 503 Maintenance; Raw set: a file, with
// its own status.
type Response struct {
	Status   int
	Node     html.Node       // the document for Page/NotFound/Error
	State    *html.FormState // nil or the form state to render with
	Location string          // Redirect
	Patch    bool            // Page: answer a Datastar request with a body patch
	Session  any             // to store in the cookie (nil, or equal to the request's: unchanged, not stored)
	Err      error           // Error
	Raw      *rtpkg.File     // a file page's answer, written as given
	// Before, set by a message's route, renders the page as it was before the message: the view
	// of the model loaded before update (or, for a form that did not decode, the same model),
	// rendered with the page's plain state (FormState{PageParams}). Under Datastar the router
	// compares it with the answer and sends only the regions that differ (html.Fragments) when
	// the client shows exactly that page (html.ShownHeader). nil: the whole body is morphed.
	Before func() html.Node
}

// WithBefore is r with Before set to before.
func (r Response) WithBefore(before func() html.Node) Response {
	r.Before = before
	return r
}

// PageResponse renders doc with st (200). When patch is set and the request came from
// Datastar, only the document's <body> is sent, as an SSE patch. A non-nil session is stored.
func PageResponse(doc html.Node, st *html.FormState, session any, patch bool) Response {
	return Response{Status: http.StatusOK, Node: doc, State: st, Session: session, Patch: patch}
}

// FileResponse is a file page's answer: f written as given, with f's status.
func FileResponse(f rtpkg.File, session any) Response {
	st := f.Status
	if st == 0 {
		st = http.StatusOK
	}
	return Response{Status: st, Raw: &f, Session: session}
}

// RedirectResponse is a 303 See Other to location; for a Datastar request, a 200 SSE answer:
// the page at location served by the router and morphed in, with a history push, when it is a
// page of this app, else an event that navigates the browser there (datastar.Navigate). A
// non-nil session that differs from the request's is stored either way.
func RedirectResponse(location string, session any) Response {
	return Response{Status: http.StatusSeeOther, Location: location, Session: session}
}

// NotFoundResponse renders doc (App.notFound) with status 404; a zero doc means
// Options.NotFoundDoc. The session is left unchanged, unless the handler sets the Response's
// Session (a Task that succeeded before the page failed to re-load).
func NotFoundResponse(doc html.Node) Response {
	return Response{Status: http.StatusNotFound, Node: doc}
}

// MaintenanceResponse is the answer of a request on a silo that is not served (err, a
// *db.MaintenanceError): Options.MaintenanceDoc with status 503 and Alder-Status: 503.
func MaintenanceResponse(err error) Response {
	return Response{Status: http.StatusServiceUnavailable, Err: err}
}

// ErrorResponse renders doc (App.error) with status 500; a zero doc means Options.ErrorDoc(err).
// err is logged; in dev it is also appended to the document as a comment. The session is left
// unchanged (a failed Task does not change it), unless the handler sets the Response's Session
// (a Task that succeeded before the page failed to re-load).
func ErrorResponse(doc html.Node, err error) Response {
	return Response{Status: http.StatusInternalServerError, Node: doc, Err: err}
}
