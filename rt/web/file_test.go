package web

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	rtpkg "alder/rt"
	"alder/rt/datastar"
)

// File pages (M17): a page whose view returns File is answered with the file as given — its
// status, Content-Type, allow-listed headers and body — and never morphed or followed into.

func fileRoute(f rtpkg.File, calls *int) []Route {
	return []Route{{Method: "GET", Pattern: "/r.csv", File: true, Handle: func(rq *Request) Response {
		*calls++
		return FileResponse(f, rq.Session)
	}}}
}

// navigateTo is the exact SSE answer of a real navigation to loc.
func navigateTo(t *testing.T, loc string) string {
	t.Helper()
	w := httptest.NewRecorder()
	if err := datastar.Navigate(w, loc, datastar.Assign); err != nil {
		t.Fatal(err)
	}
	return w.Body.String()
}

func TestFile_WrittenVerbatim(t *testing.T) {
	var n int
	f := rtpkg.File{Status: 200, ContentType: "text/csv", Body: []byte("a,b\n"),
		Headers: [][2]string{{"Content-Disposition", "attachment; filename=r.csv"}, {"Set-Cookie", "x=1"}}}
	var logs bytes.Buffer
	h := NewRouter(fileRoute(f, &n), testOpts(func(o *Options) {
		o.Log = func(level, msg string) { logs.WriteString(level + " " + msg + "\n") }
	}))
	w := serve(h, httptest.NewRequest("GET", "/r.csv", nil))
	if w.Code != 200 || w.Header().Get("Content-Type") != "text/csv" || w.Body.String() != "a,b\n" {
		t.Fatalf("%d %q %q", w.Code, w.Header().Get("Content-Type"), w.Body.String())
	}
	if w.Header().Get("Content-Disposition") != "attachment; filename=r.csv" {
		t.Fatal("allowed header sent")
	}
	if w.Header().Get("Set-Cookie") != "" || w.Header().Get(StatusHeader) != "" {
		t.Fatal("refused header dropped; no Alder-Status; no session cookie")
	}
	if !strings.Contains(logs.String(), `warn /r.csv: header "Set-Cookie" cannot be set from a page; dropped`) {
		t.Fatalf("a dropped header is a warn line: %q", logs.String())
	}
	// HEAD: the same headers, no body
	w = serve(h, httptest.NewRequest("HEAD", "/r.csv", nil))
	if w.Code != 200 || w.Header().Get("Content-Type") != "text/csv" || w.Body.Len() != 0 {
		t.Fatalf("HEAD: %d %q %q", w.Code, w.Header().Get("Content-Type"), w.Body.String())
	}
}

func TestFile_StatusAndDefaults(t *testing.T) {
	var n int
	h := NewRouter(fileRoute(rtpkg.File{Status: 503, Body: []byte("busy"), Headers: [][2]string{{"Retry-After", "20"}}}, &n), testOpts(nil))
	w := serve(h, httptest.NewRequest("GET", "/r.csv", nil))
	if w.Code != 503 || w.Body.String() != "busy" || w.Header().Get("Retry-After") != "20" ||
		w.Header().Get("Content-Type") != "application/octet-stream" {
		t.Fatalf("%d %q %v", w.Code, w.Body.String(), w.Header())
	}
	if strings.Contains(w.Body.String(), "<!doctype") {
		t.Fatal("a file's 503 is its own body, never the maintenance page")
	}
	// Status 0 is 200
	n = 0
	h = NewRouter(fileRoute(rtpkg.File{Body: []byte("x")}, &n), testOpts(nil))
	if w := serve(h, httptest.NewRequest("GET", "/r.csv", nil)); w.Code != 200 || w.Body.String() != "x" {
		t.Fatalf("status 0: %d %q", w.Code, w.Body.String())
	}
}

func TestFile_StatusOutOfRangeIs500(t *testing.T) {
	for _, code := range []int{101, 199, 600} {
		var n int
		h := NewRouter(fileRoute(rtpkg.File{Status: code, Body: []byte("x")}, &n), testOpts(nil))
		w := serve(h, httptest.NewRequest("GET", "/r.csv", nil))
		if w.Code != 500 || !strings.HasPrefix(w.Body.String(), "<!doctype html>") {
			t.Fatalf("%d: %d %q", code, w.Code, w.Body.String())
		}
	}
}

func TestFile_NoBodyStatuses(t *testing.T) {
	for _, code := range []int{204, 304} {
		var n int
		var logs bytes.Buffer
		h := NewRouter(fileRoute(rtpkg.File{Status: code, Body: []byte("ignored")}, &n), testOpts(func(o *Options) {
			o.Log = func(level, msg string) { logs.WriteString(level + " " + msg + "\n") }
		}))
		w := serve(h, httptest.NewRequest("GET", "/r.csv", nil))
		if w.Code != code || w.Body.Len() != 0 || strings.Contains(logs.String(), "error") {
			t.Fatalf("%d: got %d %q, logs %q", code, w.Code, w.Body.String(), logs.String())
		}
	}
}

func TestFile_HeaderCaseAndNewlines(t *testing.T) {
	var n int
	f := rtpkg.File{Body: []byte("x"), Headers: [][2]string{
		{"content-disposition", "inline"}, {"X-Note", "a\r\nSet-Cookie: x=1"}, {"X-Bad\r\nSet-Cookie", "y=2"}}}
	h := NewRouter(fileRoute(f, &n), testOpts(nil))
	w := serve(h, httptest.NewRequest("GET", "/r.csv", nil))
	if w.Header().Get("Content-Disposition") != "inline" {
		t.Fatal("a lowercase allowed name is sent, canonicalised")
	}
	if w.Header().Get("Set-Cookie") != "" || w.Header().Get("X-Note") != "a  Set-Cookie: x=1" {
		t.Fatalf("a newline in a value must not inject a header: %q", w.Header())
	}
	for name := range w.Header() {
		if strings.ContainsAny(name, "\r\n") {
			t.Fatalf("a newline in a name must not inject a header: %q", w.Header())
		}
	}
}

func TestFile_DatastarGetNavigates(t *testing.T) {
	var n int
	h := NewRouter(fileRoute(rtpkg.File{Body: []byte("x")}, &n), testOpts(nil))
	req := httptest.NewRequest("GET", "/r.csv?datastar=%7B%7D", nil)
	req.Header.Set("Datastar-Request", "true")
	w := serve(h, req)
	if n != 0 {
		t.Fatalf("load ran %d times for a Datastar GET of a file", n)
	}
	if w.Body.String() != navigateTo(t, "/r.csv") || w.Header().Get("Content-Type") != "text/event-stream" {
		t.Fatalf("want the navigate event: %q", w.Body.String())
	}
}

func TestFile_RedirectIntoFileNavigates(t *testing.T) {
	var n int
	routes := append(fileRoute(rtpkg.File{Body: []byte("x")}, &n), Route{Method: "POST", Pattern: "/go", Handle: func(rq *Request) Response {
		return RedirectResponse("/r.csv?y=1", rq.Session)
	}})
	h := NewRouter(routes, testOpts(nil))
	req := httptest.NewRequest("POST", "/go", nil)
	req.Header.Set("Datastar-Request", "true")
	w := serve(h, req)
	if n != 0 || w.Body.String() != navigateTo(t, "/r.csv?y=1") {
		t.Fatalf("a redirect into a file is the navigate event, never a morph: n=%d %q", n, w.Body.String())
	}
}

// A file route is "followable"-refused only when it is the route serve would pick: a more
// specific HTML page on a path a file pattern also matches is still followed.
func TestFile_FollowableMirrorsServe(t *testing.T) {
	page := func(rq *Request) Response { return PageResponse(doc("page"), nil, nil, false) }
	file := func(rq *Request) Response { return FileResponse(rtpkg.File{Body: []byte("f")}, rq.Session) }
	h := NewRouter([]Route{
		{Method: "GET", Pattern: "/r/{id}", File: true, Handle: file},
		{Method: "GET", Pattern: "/r/new", Handle: page},
		{Method: "POST", Pattern: "/r/{id}", Handle: page},
	}, testOpts(nil)).(*router)
	for loc, want := range map[string]bool{"/r/7": false, "/r/7/?a#b": false, "/r/new": true, "/x": true, "/r/%zz": true} {
		if got := h.followable(loc); got != want {
			t.Errorf("followable(%q) = %v", loc, got)
		}
	}
}

// A plain GET of a file leaves the notice a redirect stored for the next HTML page.
func TestFile_KeepsNotice(t *testing.T) {
	var n int
	h := NewRouter(fileRoute(rtpkg.File{Body: []byte("x")}, &n), testOpts(nil))
	req := httptest.NewRequest("GET", "/r.csv", nil)
	req.AddCookie(&http.Cookie{Name: NoticeCookie, Value: "v"})
	if w := serve(h, req); len(w.Result().Cookies()) != 0 || w.Body.String() != "x" {
		t.Fatalf("cookies %v body %q", w.Result().Cookies(), w.Body.String())
	}
}

func TestFile_RepeatedHeaders(t *testing.T) {
	var n int
	f := rtpkg.File{Status: 200, ContentType: "text/csv", Body: []byte("a"),
		Headers: [][2]string{
			{"Content-Disposition", "inline"}, {"content-disposition", "attachment; filename=r.csv"},
			{"X-Tag", "one"}, {"x-tag", "two"}}}
	h := NewRouter(fileRoute(f, &n), testOpts(nil))
	w := serve(h, httptest.NewRequest("GET", "/r.csv", nil))
	if got := w.Header().Values("Content-Disposition"); len(got) != 1 || got[0] != "attachment; filename=r.csv" {
		t.Fatalf("a single-valued header is sent once, the last value: %q", got)
	}
	if got := w.Header().Values("X-Tag"); len(got) != 2 || got[0] != "one" || got[1] != "two" {
		t.Fatalf("an X- header repeats: %q", got)
	}
}
