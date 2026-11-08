// Package routes holds the app's routes as `alder routes --json` lists them, and matches a
// request path to the route the router would pick for it.
package routes

import (
	"encoding/json"
	"net/url"
	"strings"
)

// Route is one route as `alder routes --json` prints it: a route (kind "route", answered to the
// method it declares), its path pattern, the module and declaration that
// handle it and where that is.
type Route struct {
	Kind   string `json:"kind"`
	Method string `json:"method"`
	Path   string `json:"path"`
	Module string `json:"module"`
	Name   string `json:"name"`
	File   string `json:"file"`
	Line   int    `json:"line"`
	Col    int    `json:"col"`
}

// Parse reads the output of `alder routes --json`.
func Parse(out []byte) ([]Route, error) {
	var rs []Route
	if err := json.Unmarshal(out, &rs); err != nil {
		return nil, err
	}
	return rs, nil
}

// Openable reports whether the route is a page with no parameter, which a browser can open
// without being given a value.
func (r Route) Openable() bool {
	return r.MethodOf() == "GET" && !strings.Contains(r.Path, "{")
}

// Match is the index in routes of the route that a request for path hits, as the router picks
// it: the query and fragment are dropped, one trailing slash is ignored, and the segments are
// compared; `{x}` matches any one non-empty segment. The most specific pattern wins, a literal
// segment before a parameter at the first position they differ, and equally specific patterns
// in the order of routes. It ignores the route's kind; MatchRequest does not.
func Match(routes []Route, path string) (int, bool) {
	return match(routes, path, func(Route) bool { return true })
}

// MethodOf is the HTTP method the route answers: its Method, or for a list from an older
// `alder routes` (kinds "page" and "msg", no method) GET for "page" and POST for "msg".
func (r Route) MethodOf() string {
	switch {
	case r.Method != "":
		return r.Method
	case r.Kind == "page":
		return "GET"
	case r.Kind == "msg":
		return "POST"
	}
	return ""
}

// MatchRequest is Match for a request with the given method: only a route of that method
// answers it, so DELETE /todos/7 hits the DELETE route, not the GET at the same path, and a
// method no route has hits none.
func MatchRequest(routes []Route, method, path string) (int, bool) {
	return match(routes, path, func(r Route) bool { return method != "" && r.MethodOf() == method })
}

func match(routes []Route, path string, ok func(Route) bool) (int, bool) {
	segs, good := split(path)
	if !good {
		return 0, false
	}
	best, bestShape := -1, ""
	for i, r := range routes {
		if !ok(r) {
			continue
		}
		shape, hit := fits(r.Path, segs)
		if hit && (best < 0 || shape < bestShape) {
			best, bestShape = i, shape
		}
	}
	return best, best >= 0
}

// split is the unescaped segments of a request path.
func split(path string) ([]string, bool) {
	if i := strings.IndexAny(path, "?#"); i >= 0 {
		path = path[:i]
	}
	p := strings.TrimSuffix(strings.TrimPrefix(path, "/"), "/")
	if p == "" {
		return nil, true
	}
	segs := strings.Split(p, "/")
	for i, s := range segs {
		u, err := url.PathUnescape(s)
		if err != nil {
			return nil, false
		}
		segs[i] = u
	}
	return segs, true
}

// fits reports whether the segments fit the pattern, and the pattern's shape, one byte per
// segment, 'a' for a literal and 'b' for a parameter, which orders patterns by specificity as
// the router does.
func fits(pattern string, segs []string) (string, bool) {
	p := strings.TrimSuffix(strings.TrimPrefix(pattern, "/"), "/")
	var ps []string
	if p != "" {
		ps = strings.Split(p, "/")
	}
	if len(ps) != len(segs) {
		return "", false
	}
	shape := make([]byte, len(ps))
	for i, s := range ps {
		shape[i] = 'a'
		switch {
		case strings.HasPrefix(s, "{") && strings.HasSuffix(s, "}") && len(s) > 2:
			if segs[i] == "" {
				return "", false
			}
			shape[i] = 'b'
		case s != segs[i]:
			return "", false
		}
	}
	return string(shape), true
}
