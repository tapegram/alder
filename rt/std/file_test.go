package std

import (
	"testing"

	"alder/rt"
	"alder/rt/html"
)

func TestFile_Constructors(t *testing.T) {
	f := FileText(struct {
		ContentType string
		Body        string
	}{"text/csv", "a,b\n"})
	if FileStatus(f) != 200 || FileContentType(f) != "text/csv" || string(FileBody(f)) != "a,b\n" {
		t.Fatalf("text: %+v", f)
	}
	j := FileJson(`{"a":1}`)
	if FileContentType(j) != "application/json" || string(FileBody(j)) != `{"a":1}` {
		t.Fatalf("json: %+v", j)
	}
	b := FileBytes(struct {
		ContentType string
		Body        []byte
	}{"", []byte{0, 1}})
	if FileContentType(b) != "application/octet-stream" || BytesLength(FileBody(b)) != 2 {
		t.Fatalf("bytes: %+v", b)
	}
	h := FileHtml(html.Raw("<!doctype html>\n<p>x</p>"))
	if FileContentType(h) != "text/html; charset=utf-8" || string(FileBody(h)) != "<!doctype html>\n<p>x</p>" {
		t.Fatalf("html: %+v", h)
	}
}

func TestFile_StatusAndHeaders(t *testing.T) {
	f := FileWithHeader(struct {
		Name  string
		Value string
	}{"Retry-After", "20"},
		FileWithStatus(503, FileText(struct {
			ContentType string
			Body        string
		}{"text/plain", "busy"})))
	if FileStatus(f) != 503 || len(f.Headers) != 1 || f.Headers[0] != [2]string{"Retry-After", "20"} {
		t.Fatalf("%+v", f)
	}
	// headers are kept as given; the router filters them (rt/web)
	g := FileWithHeader(struct {
		Name  string
		Value string
	}{"Set-Cookie", "x=1"}, f)
	if len(g.Headers) != 2 || len(f.Headers) != 1 {
		t.Fatalf("withHeader must not alias: %+v %+v", f, g)
	}
}

func TestBytes(t *testing.T) {
	if BytesLength(TextToBytes("héllo")) != 6 {
		t.Fatal("length counts bytes")
	}
	if got := BytesToText(TextToBytes("héllo")); got != rt.Just("héllo") {
		t.Fatalf("%v", got)
	}
	if got := BytesToText([]byte{0xff, 0xfe}); got.Ok {
		t.Fatalf("invalid UTF-8 must be Nothing, got %v", got)
	}
	if !rt.Eq(TextToBytes("ab"), []byte("ab")) || rt.Eq(TextToBytes("ab"), []byte("ac")) {
		t.Fatal("Bytes compares by content")
	}
	if !rt.Eq(FileJson("1"), FileJson("1")) || rt.Eq(FileJson("1"), FileWithStatus(500, FileJson("1"))) {
		t.Fatal("File compares by status, type, headers, body")
	}
}

func TestFileHeaderAllowed(t *testing.T) {
	for _, n := range []string{"Content-Disposition", "content-disposition", "ETag", "Cache-Control", "Last-Modified", "Retry-After", "Content-Language", "X-Replay", "x-anything"} {
		if _, ok := rt.FileHeaderAllowed(n); !ok {
			t.Errorf("%s should be allowed", n)
		}
	}
	for _, n := range []string{"Set-Cookie", "Location", "Content-Length", "Transfer-Encoding", "Content-Type", "Connection", "X", "bad name", ""} {
		if _, ok := rt.FileHeaderAllowed(n); ok {
			t.Errorf("%s should be refused", n)
		}
	}
}
