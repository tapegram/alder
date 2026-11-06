package rt

import "net/url"

// Route is a request to one of the app's routes (#160): Name is the route's qualified name
// ("Todos.Row"), Method its HTTP method ("GET", "PATCH", …), Path its path with the path
// parameters filled in (each segment escaped), and Query its other parameters, in declaration
// order. Url.external is a Route with the URL as Path and nothing else.
type Route struct {
	Name, Method, Path string
	Query              []KV
}

// RouteURL is the URL r points at: `href={…}` and a redirect render it. It is r's Path, then
// the Query parameters that are not Omit as an URL-encoded query string ("/?filter=Active").
func RouteURL(r Route) string { return r.Path + queryString(r.Query) }

// RouteKey identifies the request r makes, as a form's FormState: its Method ("GET" when empty,
// as AlderEqual reads it), a space, and MsgKey of its Path and its own query (the parameters
// that are not Omit), so only the form that sent this very request (a per-row form's row id
// included) shows its values and errors.
func RouteKey(r Route) string {
	q := url.Values{}
	for _, kv := range r.Query {
		if !kv.Omit {
			q[kv.Key] = append(q[kv.Key], kv.Value)
		}
	}
	return routeMethod(r) + " " + MsgKey(r.Path, q)
}

// AlderEqual is Route's equality (Equaler): two routes are equal when they have the same Method (empty is GET)
// and link to the same URL (RouteURL), so a route built with a parameter at its default (left
// out of the URL) equals the route read back from that URL.
func (r Route) AlderEqual(other any) bool {
	o := other.(Route)
	return routeMethod(r) == routeMethod(o) && RouteURL(r) == RouteURL(o)
}

// AlderKey is Route's Dict key (Keyer): its Method and URL, so two routes equal under
// AlderEqual are one key. An empty Method is "GET", so a Page value and Url.external of one URL are one key.
func (r Route) AlderKey() string {
	return routeMethod(r) + " " + RouteURL(r)
}

// routeMethod is r's Method, "GET" when empty: a Page value carries no Method and is a GET.
func routeMethod(r Route) string {
	if r.Method == "" {
		return "GET"
	}
	return r.Method
}

// showRoute is r as Alder: its Name applied to its URL ("Todos.Index /todos"), or the bare URL
// for a route with no Name (a Page, Url.external), and whether that is an application.
func showRoute(r Route) (string, bool) {
	if r.Name == "" {
		return RouteURL(r), false
	}
	return r.Name + " " + RouteURL(r), true
}
