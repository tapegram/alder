package rt

import (
	"reflect"
	"testing"
)

func TestResponse_Codes(t *testing.T) {
	r := Route{Name: "Todos.Index", Method: "GET", Path: "/todos"}
	for _, c := range []struct {
		name string
		resp Response
		want int
	}{
		{"redirect", RedirectTo(r), 303},
		{"render", Render(r), 200},
		{"refresh", Refresh(), 200},
		{"invalid", Invalid("no"), 422},
	} {
		if got := c.resp.Code(); got != c.want {
			t.Errorf("%s: Code() = %d, want %d", c.name, got, c.want)
		}
	}
	if got := (Response{Kind: RespRedirect, Status: 302}).Code(); got != 302 {
		t.Errorf("an explicit Status wins: Code() = %d, want 302", got)
	}
}

func TestResponse_ToastAppends(t *testing.T) {
	base := Refresh()
	resp := WithToast(NoticeSuccess, "Saved", WithToast(NoticeInfo, "Added", base))
	want := []Toast{{Kind: NoticeInfo, Text: "Added"}, {Kind: NoticeSuccess, Text: "Saved"}}
	if got := resp.Toasts(); !reflect.DeepEqual(got, want) {
		t.Fatalf("Toasts() = %+v, want %+v", got, want)
	}
	if len(base.Toasts()) != 0 {
		t.Fatalf("WithToast changed its argument: %+v", base.Toasts())
	}
	// Two toasts on one base do not share a backing array.
	one := WithToast(NoticeInfo, "a", base)
	x, y := WithToast(NoticeInfo, "x", one), WithToast(NoticeInfo, "y", one)
	if x.Toasts()[1].Text != "x" || y.Toasts()[1].Text != "y" {
		t.Fatalf("toasts alias: %+v %+v", x.Toasts(), y.Toasts())
	}
}

func TestResponse_Equal(t *testing.T) {
	r := Route{Name: "Todos.Index", Method: "GET", Path: "/todos"}
	built := Route{Name: "Todos.Index", Method: "GET", Path: "/todos", Query: []KV{{Key: "f", Value: "All", Omit: true}}}
	a := WithToast(NoticeInfo, "Added", RedirectTo(r))
	b := WithToast(NoticeInfo, "Added", RedirectTo(built))
	if !Eq(a, b) {
		t.Fatal("same target URL and toasts should be equal")
	}
	if c := WithToast(NoticeError, "Added", RedirectTo(r)); Eq(a, c) {
		t.Fatal("a different toast kind should be unequal")
	}
	if Eq(RedirectTo(r), Render(r)) {
		t.Fatal("different kinds should be unequal")
	}
	if Eq(Invalid("a"), Invalid("b")) {
		t.Fatal("different invalid texts should be unequal")
	}
	if Eq(RedirectTo(r), RedirectTo(Route{Name: "Todos.Index", Method: "POST", Path: "/todos"})) {
		t.Fatal("different methods should be unequal")
	}
}

func TestResponse_Show(t *testing.T) {
	r := Route{Name: "Todos.Index", Method: "GET", Path: "/todos", Query: []KV{{Key: "f", Value: "a b"}}}
	for _, c := range []struct {
		resp Response
		want string
	}{
		{WithToast(NoticeInfo, "Added", RedirectTo(r)), `Response.redirect (Todos.Index /todos?f=a+b) |> Response.toast "Added"`},
		{Render(r), `Response.render (Todos.Index /todos?f=a+b)`},
		{Refresh(), `Response.refresh`},
		{WithToast(NoticeWarning, "Hm", WithToast(NoticeSuccess, "Ok", Invalid("bad"))), `Response.invalid "bad" |> Response.success "Ok" |> Response.warning "Hm"`},
		{WithToast(NoticeError, "x", RedirectTo(Route{Path: "https://example.com/"})), `Response.redirect https://example.com/ |> Response.error "x"`},
	} {
		if got := Show(c.resp); got != c.want {
			t.Errorf("Show = %s\nwant   %s", got, c.want)
		}
	}
	if got, want := Show(r), "Todos.Index /todos?f=a+b"; got != want {
		t.Errorf("Show(route) = %s, want %s", got, want)
	}
}

func TestRoute_URLAndKey(t *testing.T) {
	r := Route{Path: "/todos/7/toggle", Method: "PATCH"}
	if got := RouteURL(r); got != "/todos/7/toggle" {
		t.Errorf("RouteURL = %q", got)
	}
	if got := RouteKey(r); got != "PATCH /todos/7/toggle" {
		t.Errorf("RouteKey = %q", got)
	}
	q := Route{Path: "/todos", Method: "GET", Query: []KV{{Key: "f", Value: "a b"}, {Key: "d", Value: "x", Omit: true}}}
	if got := RouteURL(q); got != "/todos?f=a+b" {
		t.Errorf("RouteURL with query = %q", got)
	}
	if got := RouteKey(q); got != "GET /todos?f=a+b" {
		t.Errorf("RouteKey with query = %q", got)
	}
	if PageURL(q) != RouteURL(q) {
		t.Error("PageURL should delegate to RouteURL")
	}
}
