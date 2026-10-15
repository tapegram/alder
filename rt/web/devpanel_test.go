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
