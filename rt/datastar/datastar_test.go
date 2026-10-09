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
