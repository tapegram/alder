// Package routes holds the app's routes as `alder routes --json` lists them, and matches a
// request path to the route the router would pick for it.
package routes

import (
	"encoding/json"
	"net/url"
	"strings"
)

// Route is one route as `alder routes --json` prints it: a page (kind "page", answered to GET)
// or a message (kind "msg", answered to POST), its path pattern, the module and declaration that
// handle it and where that is.
type Route struct {
	Kind   string `json:"kind"`
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
	return r.Kind == "page" && !strings.Contains(r.Path, "{")
}

// Match is the index in routes of the route that a request for path hits, as the router picks
// it: the query and fragment are dropped, one trailing slash is ignored, and the segments are
// compared; `{x}` matches any one non-empty segment. The most specific pattern wins, a literal
// segment before a parameter at the first position they differ, and equally specific patterns
// in the order of routes. It ignores the route's kind; MatchRequest does not.
func Match(routes []Route, path string) (int, bool) {
	return match(routes, path, func(Route) bool { return true })
}

// MatchRequest is Match for a request with the given method: only a page answers a GET and only
// a message answers a POST, so a request of any other method hits no route.
func MatchRequest(routes []Route, method, path string) (int, bool) {
	kind := ""
	switch method {
	case "GET":
		kind = "page"
	case "POST":
		kind = "msg"
	}
	return match(routes, path, func(r Route) bool { return kind != "" && r.Kind == kind })
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
