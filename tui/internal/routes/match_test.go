package routes

import "testing"

func TestMatch(t *testing.T) {
	rs := []Route{
		{Kind: "page", Path: "/"},
		{Kind: "page", Path: "/books/{id}"},
		{Kind: "page", Path: "/books/new"},
		{Kind: "page", Path: "/a/{x}/c"},
		{Kind: "page", Path: "/{y}/b/c"},
		{Kind: "msg", Path: "/_m/Books/lend"},
	}
	for _, c := range []struct {
		path string
		want int // -1: none
	}{
		{"/", 0},
		{"", 0},
		{"/books/new", 2},
		{"/books/new/", 2},
		{"/books/42?x=1", 1},
		{"/books/42#top", 1},
		{"/books/a%2Fb", 1},
		{"/books/", -1},
		{"/books", -1},
		{"/books/42/lend", -1},
		{"/nope", -1},
		{"/a/b/c", 3}, // a literal first segment beats a parameter there, whatever the order
		{"/_m/Books/lend", 5},
	} {
		got, ok := Match(rs, c.path)
		if c.want < 0 && ok || c.want >= 0 && (!ok || got != c.want) {
			t.Errorf("Match(%q) = %d, %v; want %d", c.path, got, ok, c.want)
		}
	}
}

func TestMatch_DeclarationOrderBreaksTies(t *testing.T) {
	rs := []Route{{Path: "/x/{a}"}, {Path: "/x/{b}"}}
	if got, ok := Match(rs, "/x/1"); !ok || got != 0 {
		t.Errorf("Match = %d, %v; want 0", got, ok)
	}
}

func TestMatchRequest(t *testing.T) {
	rs := []Route{{Kind: "page", Path: "/books/{id}"}, {Kind: "msg", Path: "/_m/Books/lend"}}
	for _, c := range []struct {
		method, path string
		want         int
	}{
		{"GET", "/books/42", 0},
		{"POST", "/books/42", -1}, // a POST to a page is a 404
		{"POST", "/_m/Books/lend", 1},
		{"GET", "/_m/Books/lend", -1},
		{"PUT", "/books/42", -1},
	} {
		got, ok := MatchRequest(rs, c.method, c.path)
		if c.want < 0 && ok || c.want >= 0 && (!ok || got != c.want) {
			t.Errorf("MatchRequest(%s %q) = %d, %v; want %d", c.method, c.path, got, ok, c.want)
		}
	}
}

func TestParse(t *testing.T) {
	out := `[{"kind":"page","path":"/books/{id}","module":"Books","name":"page","file":"app/Books.al","line":3,"col":1}]`
	rs, err := Parse([]byte(out))
	if err != nil || len(rs) != 1 || rs[0].Module != "Books" || rs[0].Line != 3 {
		t.Errorf("Parse = %+v, %v", rs, err)
	}
	if _, err := Parse([]byte("not json")); err == nil {
		t.Error("Parse accepted text")
	}
}

func TestRoute_Openable(t *testing.T) {
	for r, want := range map[Route]bool{
		{Kind: "page", Path: "/"}:             true,
		{Kind: "page", Path: "/books"}:        true,
		{Kind: "page", Path: "/books/{id}"}:   false,
		{Kind: "msg", Path: "/_m/Books/lend"}: false,
	} {
		if r.Openable() != want {
			t.Errorf("%+v.Openable() = %v", r, !want)
		}
	}
}

func TestMatch_MethodAndPath(t *testing.T) {
	rs := []Route{
		{Method: "GET", Kind: "page", Path: "/todos/{id}", Name: "Row"},
		{Method: "DELETE", Kind: "msg", Path: "/todos/{id}", Name: "Delete"},
		{Method: "PATCH", Kind: "msg", Path: "/todos/{id}/toggle", Name: "Toggle"},
		{Method: "POST", Kind: "msg", Path: "/todos", Name: "Create"},
	}
	for _, c := range []struct {
		method, path string
		want         int
	}{
		{"GET", "/todos/7", 0},
		{"DELETE", "/todos/7", 1},
		{"PATCH", "/todos/7/toggle", 2},
		{"POST", "/todos", 3},
		{"PUT", "/todos/7", -1},
		{"GET", "/todos", -1},
	} {
		got, ok := MatchRequest(rs, c.method, c.path)
		if c.want < 0 && ok || c.want >= 0 && (!ok || got != c.want) {
			t.Errorf("MatchRequest(%s %q) = %d, %v; want %d", c.method, c.path, got, ok, c.want)
		}
	}
}
