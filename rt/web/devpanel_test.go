package web

import (
	"net/http/httptest"
	"strings"
	"testing"
)

func TestDevPanel(t *testing.T) {
	t.Setenv("ALDER_DEV", "1")
	route := one("GET", "/x", func(rq *Request) Response { return PageResponse(doc("hi"), nil, nil, false) })
	for _, tc := range []struct {
		dev, ds, off bool
		want         []string
		not          []string
	}{
		{dev: true, want: []string{"<al-devpanel", "aria-controls=\"_alder_tab_trace\"", "_alder_tab_sql"}},
		{dev: true, ds: true, want: []string{"data: selector #_alder_panel", "data: mode inner", "data-ignore-morph"}},
		{dev: true, off: true, not: []string{"al-devpanel"}},
		{dev: false, not: []string{"al-devpanel", "_alder_tab"}},
	} {
		if tc.off {
			t.Setenv("ALDER_DEV_PANEL", "0")
		}
		h := NewRouter(route, testOpts(func(o *Options) { o.Dev = tc.dev }))
		req := httptest.NewRequest("GET", "/x", nil)
		if tc.ds {
			req.Header.Set("Datastar-Request", "true")
		}
		body := serve(h, req).Body.String()
		for _, s := range tc.want {
			if !strings.Contains(body, s) {
				t.Errorf("%+v: missing %q in %q", tc, s, body)
			}
		}
		for _, s := range tc.not {
			if strings.Contains(body, s) {
				t.Errorf("%+v: has %q", tc, s)
			}
		}
	}
}

func TestDevMap(t *testing.T) {
	route := one("GET", "/x", func(rq *Request) Response { return PageResponse(doc("hi"), nil, nil, false) })
	for _, dev := range []bool{true, false} {
		t.Setenv("ALDER_DEV", "1")
		h := NewRouter(route, testOpts(func(o *Options) { o.Dev = dev }))
		w := serve(h, httptest.NewRequest("GET", "/_dev", nil))
		if dev && (w.Code != 200 || !strings.Contains(w.Body.String(), `href="/x"`)) {
			t.Fatalf("dev: %d %q", w.Code, w.Body.String())
		}
		if !dev && w.Code != 404 {
			t.Fatalf("prod: %d", w.Code)
		}
	}
}

func TestNoticedToast(t *testing.T) {
	h := NewRouter(one("POST", "/_m/A/B", func(rq *Request) Response {
		rq.Notice = "Saved <ok>"
		return PageResponse(doc("hi"), nil, nil, true)
	}), testOpts(nil))
	req := httptest.NewRequest("POST", "/_m/A/B", nil)
	req.Header.Set("Datastar-Request", "true")
	body := serve(h, req).Body.String()
	for _, s := range []string{
		"data: selector al-toasts\ndata: mode append\ndata: elements <al-toast variant=\"info\" timeout=\"5000\">Saved &lt;ok&gt;</al-toast>",
		"<al-toasts data-ignore-morph></al-toasts></body>",
	} {
		if !strings.Contains(body, s) {
			t.Errorf("missing %q in %q", s, body)
		}
	}
	// the toast comes after the body patch, so a morph does not show it again
	if strings.Index(body, "selector body") > strings.Index(body, "selector al-toasts") {
		t.Errorf("order: %q", body)
	}
}
