package web

import (
	"errors"
	"net/http"
	"net/url"
	"strings"
)

// methods are the methods a Route may declare, in the order an Allow header lists them (HEAD,
// which no route declares, right after GET: a GET route answers it).
var methods = []string{http.MethodGet, http.MethodHead, http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete}

// declarable is whether a Route may declare method: GET, POST, PUT, PATCH or DELETE.
func declarable(method string) bool {
	switch method {
	case http.MethodGet, http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
		return true
	}
	return false
}

// hasBody is whether a request with method has its body read (readForm): every method but GET
// and HEAD. It is also the set a cross-site defence must cover: the session cookie is
// SameSite=Lax, which a browser withholds from every cross-site request of these methods.
func hasBody(method string) bool {
	return method != http.MethodGet && method != http.MethodHead
}

// methodField is the url-encoded form field that overrides a POST's method (the Rails
// convention): what a no-JS <form method="post"> sends for a PUT, PATCH or DELETE route.
const methodField = "_method"

// errOverride is effectiveMethod's refusal of a POST's _method: the request is a 400.
var errOverride = errors.New(`"_method" must be one value: PUT, PATCH or DELETE`)

// effectiveMethod is the method r is routed by: r.Method (HEAD as GET), except that a POST whose
// url-encoded body (form, as readForm parsed it; the query never counts) carries _method is
// routed by it. That _method must be exactly one value, "PUT", "PATCH" or "DELETE" (case
// matters); any other, an empty one or several, is errOverride. On any method but POST
// _method is ignored.
func effectiveMethod(r *http.Request, form url.Values) (string, error) {
	switch r.Method {
	case http.MethodHead:
		return http.MethodGet, nil
	case http.MethodPost:
	default:
		return r.Method, nil
	}
	vs, ok := form[methodField]
	if !ok {
		return http.MethodPost, nil
	}
	if len(vs) != 1 {
		return "", errOverride
	}
	switch vs[0] {
	case http.MethodPut, http.MethodPatch, http.MethodDelete:
		return vs[0], nil
	}
	return "", errOverride
}

// allowHeader is the Allow header of a 405: declared (the methods some route declares for the
// path) in the order of methods, HEAD with GET.
func allowHeader(declared map[string]bool) string {
	var b strings.Builder
	for _, m := range methods {
		if declared[m] || m == http.MethodHead && declared[http.MethodGet] {
			if b.Len() > 0 {
				b.WriteString(", ")
			}
			b.WriteString(m)
		}
	}
	return b.String()
}

// notAllowed answers a request whose path some route declares, but not for its method: 405
// with Allow (a page, or a body patch for a Datastar request); logged at warn.
func (rt *router) notAllowed(w http.ResponseWriter, r *http.Request, st hop, method string, declared map[string]bool) {
	allow := allowHeader(declared)
	rt.logReq(w, "warn", line(w, r, "405: "+method+" is not one of "+allow))
	if tw, ok := w.(*tracker); ok {
		tw.setAnswer("method not allowed")
	}
	w.Header().Set("Allow", allow)
	rt.failure(w, r, st, http.StatusMethodNotAllowed, "<p>405 method not allowed</p>")
}

// badOverride answers a POST whose _method effectiveMethod refused: 400 (a page, or a body
// patch for a Datastar request); logged at warn.
func (rt *router) badOverride(w http.ResponseWriter, r *http.Request, st hop, err error) {
	rt.logReq(w, "warn", line(w, r, "400: "+err.Error()))
	if tw, ok := w.(*tracker); ok {
		tw.setAnswer("bad request")
	}
	rt.failure(w, r, st, http.StatusBadRequest, "<p>400 bad request: "+esc(err.Error())+"</p>")
}
