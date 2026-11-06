package rt

import "testing"

// A Page value carries no Method and is a GET: Url.external (Method "GET") of one URL equals it,
// and the two are one Dict key.
func TestRoute_EmptyMethodIsGET(t *testing.T) {
	page, ext := Route{Path: "/x"}, Route{Method: "GET", Path: "/x"}
	if !Eq(page, ext) || !Eq(ext, page) {
		t.Errorf("Route{Path} != Route{GET, Path}")
	}
	if page.AlderKey() != ext.AlderKey() {
		t.Errorf("keys differ: %q vs %q", page.AlderKey(), ext.AlderKey())
	}
	if Eq(page, Route{Method: "POST", Path: "/x"}) {
		t.Errorf("a POST route equals a page")
	}
	if RouteKey(page) != RouteKey(ext) || RouteKey(page) != "GET /x" {
		t.Errorf("RouteKey: %q vs %q", RouteKey(page), RouteKey(ext))
	}
}
