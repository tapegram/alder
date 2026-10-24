package lsp

import (
	"strings"
	"testing"
)

func TestRoutes_List(t *testing.T) {
	root := blogCopy(t)
	c := start(t, root)
	openClean(t, c, "app/Posts/Show.alder")
	var out []routeItem
	if e := c.call("alder/routes", map[string]any{}, &out); e != nil {
		t.Fatalf("rpc error %+v", e)
	}
	var page, msg bool
	for _, r := range out {
		if r.Kind == "page" && r.Path == "/posts/{slug}" && r.Module == "Posts.Show" && r.Name == "page" &&
			r.URI == c.uri("app/Posts/Show.alder") && r.Range.Start == (Position{Line: 8, Character: 0}) {
			page = true
		}
		if r.Kind == "msg" && r.Path == "/_m/Posts.Show/AddComment" && r.Name == "AddComment" &&
			strings.HasSuffix(r.URI, "app/Posts/Show.alder") && r.Range.Start == (Position{Line: 45, Character: 0}) {
			msg = true
		}
	}
	if !page || !msg {
		t.Fatalf("page %v msg %v in %+v", page, msg, out)
	}
}
