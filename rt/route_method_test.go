package rt

import "testing"

// A Route with no Method is a GET: Url.external (Method "GET") of one URL equals it,
// and the two are one Dict key.
func TestRoute_EmptyMethodIsGET(t *testing.T) {
	bare, ext := Route{Path: "/x"}, Route{Method: "GET", Path: "/x"}
	if !Eq(bare, ext) || !Eq(ext, bare) {
		t.Errorf("Route{Path} != Route{GET, Path}")
	}
	if bare.AlderKey() != ext.AlderKey() {
		t.Errorf("keys differ: %q vs %q", bare.AlderKey(), ext.AlderKey())
	}
	if Eq(bare, Route{Method: "POST", Path: "/x"}) {
		t.Errorf("a POST route equals a GET")
	}
	if RouteKey(bare) != RouteKey(ext) || RouteKey(bare) != "GET /x" {
		t.Errorf("RouteKey: %q vs %q", RouteKey(bare), RouteKey(ext))
	}
}
