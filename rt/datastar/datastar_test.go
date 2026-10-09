package datastar

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"alder/rt/html"
)

func TestPatchBody_EventFormat(t *testing.T) {
	rec := httptest.NewRecorder()
	if err := PatchBody(rec, "<body><p>a</p>\n<p>b</p></body>"); err != nil {
		t.Fatal(err)
	}
	want := "event: datastar-patch-elements\ndata: selector body\ndata: mode outer\ndata: elements <body><p>a</p>\ndata: elements <p>b</p></body>\n\n"
	if rec.Body.String() != want || rec.Header().Get("Content-Type") != "text/event-stream" {
		t.Fatalf("%q", rec.Body.String())
	}
	if !rec.Flushed {
		t.Fatal("not flushed")
	}
}

func TestHandler_ServesScript(t *testing.T) {
	file, err := os.ReadFile("datastar.js")
	if err != nil {
		t.Fatal(err)
	}
	rec := httptest.NewRecorder()
	Handler().ServeHTTP(rec, httptest.NewRequest("GET", ScriptPath, nil))
	res := rec.Result()
	body, _ := io.ReadAll(res.Body)
	if res.StatusCode != 200 {
		t.Fatal(res.StatusCode)
	}
	if ct := res.Header.Get("Content-Type"); !strings.HasPrefix(ct, "application/javascript") {
		t.Fatal(ct)
	}
	if res.Header.Get("Cache-Control") == "" {
		t.Fatal("no Cache-Control")
	}
	if len(body) < 64 || !bytes.Equal(body, file) {
		t.Fatalf("body is not the vendored file (%d bytes)", len(body))
	}
}

func TestVersionMatchesFile(t *testing.T) {
	v, err := os.ReadFile("VERSION")
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(string(v)) != Version {
		t.Fatalf("VERSION %q, Version %q", v, Version)
	}
	js, _ := os.ReadFile("datastar.js")
	if !strings.HasPrefix(string(js), "// Datastar "+Version) {
		t.Fatal("datastar.js header does not name Version")
	}
}

func TestScriptTag(t *testing.T) {
	if got := html.Render(ScriptTag()); got != `<script type="module" src="/_alder/datastar.js"></script>` {
		t.Fatal(got)
	}
}

func TestIsDatastar(t *testing.T) {
	r := httptest.NewRequest("POST", "/", nil)
	if IsDatastar(r) {
		t.Fatal("absent header")
	}
	r.Header.Set("Datastar-Request", "true")
	if !IsDatastar(r) {
		t.Fatal("present header")
	}
	var _ http.Handler = Handler()
}

// A lone "\r" (or "\r\n") in user text must not end a data line early: the reviewer's payload
// "pwn\rdata: selector head" would otherwise start an SSE field of its own.
func TestPatchBody_CarriageReturnCannotInjectFields(t *testing.T) {
	rec := httptest.NewRecorder()
	if err := PatchBody(rec, "<body><p>pwn\rdata: selector head</p>\r\n<p>b</p>\r</body>\r\n"); err != nil {
		t.Fatal(err)
	}
	got := rec.Body.String()
	want := "event: datastar-patch-elements\ndata: selector body\ndata: mode outer\n" +
		"data: elements <body><p>pwn\ndata: elements data: selector head</p>\ndata: elements <p>b</p>\ndata: elements </body>\n\n"
	if got != want {
		t.Fatalf("got %q\nwant %q", got, want)
	}
	if strings.Contains(got, "\r") {
		t.Fatalf("a carriage return reached the stream: %q", got)
	}
	for _, line := range strings.Split(strings.TrimSuffix(got, "\n\n"), "\n") {
		if line != "event: datastar-patch-elements" && !strings.HasPrefix(line, "data: ") {
			t.Fatalf("line %q is not a data line", line)
		}
	}
}

// Navigate is the SDKs' redirect: a self-removing script appended to <body> that assigns
// window.location; the URL is a JSON string with HTML-significant characters escaped.
func TestNavigate_EventFormat(t *testing.T) {
	rec := httptest.NewRecorder()
	if err := Navigate(rec, "/?filter=Active&q=</script>\u2028"); err != nil {
		t.Fatal(err)
	}
	want := "event: datastar-patch-elements\ndata: selector body\ndata: mode append\n" +
		`data: elements <script data-effect="el.remove()">window.location.assign("/?filter=Active\u0026q=\u003c/script\u003e\u2028")</script>` + "\n\n"
	if got := rec.Body.String(); got != want {
		t.Fatalf("got  %q\nwant %q", got, want)
	}
	if rec.Code != 200 || rec.Header().Get("Content-Type") != "text/event-stream" || !rec.Flushed {
		t.Fatalf("%d %q flushed=%v", rec.Code, rec.Header().Get("Content-Type"), rec.Flushed)
	}
}

// Navigate refuses a URL rt.SafeURL refuses and writes nothing.
func TestNavigate_RefusesScriptURLs(t *testing.T) {
	for _, u := range []string{"javascript:alert(document.cookie)", "JavaScript:x", "//evil.com", "data:text/html,x"} {
		rec := httptest.NewRecorder()
		err := Navigate(rec, u)
		if err == nil || !strings.HasPrefix(err.Error(), "refusing to navigate to ") || rec.Body.Len() != 0 || rec.Header().Get("Content-Type") != "" {
			t.Fatalf("Navigate(%q): %v, %q", u, err, rec.Body.String())
		}
	}
	rec := httptest.NewRecorder()
	if err := Navigate(rec, "#"); err != nil || !strings.Contains(rec.Body.String(), `window.location.assign("#")`) {
		t.Fatalf("Navigate(#): %v %q", err, rec.Body.String())
	}
}

// PatchEvent is the event patch writes; an empty elements is one empty data line (clears in
// mode inner).
func TestPatchEvent(t *testing.T) {
	if got, want := string(PatchEvent("#x", "inner", "a\r\nb")), "event: datastar-patch-elements\ndata: selector #x\ndata: mode inner\ndata: elements a\ndata: elements b\n\n"; got != want {
		t.Errorf("PatchEvent = %q, want %q", got, want)
	}
	if got, want := string(PatchEvent("#x", "inner", "")), "event: datastar-patch-elements\ndata: selector #x\ndata: mode inner\ndata: elements \n\n"; got != want {
		t.Errorf("PatchEvent empty = %q, want %q", got, want)
	}
}
