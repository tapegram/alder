package std

import (
	"testing"

	"alder/rt"
)

// `Response.redirect (Url.external "/x") |> Response.success "ok"`: a 303 to /x with one success toast.
func TestResponse_Builders(t *testing.T) {
	r := ResponseSuccess("ok", ResponseRedirect(UrlExternal("/x")))
	if r.Kind != rt.RespRedirect || r.Code() != 303 || rt.RouteURL(r.Target) != "/x" {
		t.Fatalf("redirect: %+v", r)
	}
	if got := r.Toasts(); len(got) != 1 || got[0] != (rt.Toast{Kind: rt.NoticeSuccess, Text: "ok"}) {
		t.Fatalf("toasts: %+v", got)
	}
	if k := ResponseWarning("w", ResponseError("e", ResponseToast("i", ResponseRefresh()))).Toasts(); len(k) != 3 ||
		k[0].Kind != rt.NoticeInfo || k[1].Kind != rt.NoticeError || k[2].Kind != rt.NoticeWarning {
		t.Fatalf("toast kinds: %+v", k)
	}
	if c := ResponseInvalid("no").Code(); c != 422 {
		t.Fatalf("invalid: %d", c)
	}
	if c := ResponseRender(UrlExternal("/y")).Code(); c != 200 {
		t.Fatalf("render: %d", c)
	}
}
