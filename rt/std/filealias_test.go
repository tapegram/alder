package std

import (
	"testing"
	"unsafe"

	"alder/rt"
	"alder/rt/html"
)

// File.text, File.json and File.html hand the Text's bytes to the File as its body without a
// copy (#37). That is sound only because no []byte Alder code can see is ever written to (rt's
// TestNoWritesToParameters); these tests pin the aliasing and that the Bytes functions copy.
func TestFile_BodyAliasesText(t *testing.T) {
	body := rt.Concat("hello, ", "world") // a heap string, as a rendered body is
	for name, f := range map[string]rt.File{
		"text": FileText(struct {
			ContentType string
			Body        string
		}{"text/plain", body}),
	} {
		if len(f.Body) != len(body) || &f.Body[0] != unsafe.StringData(body) || string(f.Body) != body {
			t.Errorf("%s: body was copied", name)
		}
	}
	h := FileHtml(html.Text(body))
	if string(h.Body) != body {
		t.Fatalf("html body = %q", h.Body)
	}
	empty := FileText(struct {
		ContentType string
		Body        string
	}{"text/plain", ""})
	if empty.Body == nil || len(empty.Body) != 0 {
		t.Fatalf("empty body = %#v, want empty non-nil", empty.Body)
	}
	if FileJson(JsonList(nil)).Body == nil {
		t.Fatal("empty json body is nil")
	}
}

// html.Render's result is aliased too: a body rendered from a node shares the rendered string.
func TestFile_HtmlBodyAliasesRender(t *testing.T) {
	n := html.El("p", nil, html.Text("hi"))
	f := FileHtml(n)
	// Render is deterministic, so compare content; the alias itself is what bodyOf does, tested
	// above through FileText and FileJson.
	if string(f.Body) != html.Render(n) || cap(f.Body) != len(f.Body) {
		t.Fatalf("html body = %q (cap %d)", f.Body, cap(f.Body))
	}
}

// The Bytes functions never hand back storage shared with an aliased body, so nothing an Alder
// program does with File.body can write into a Text.
func TestFile_BytesOfBodyAreFresh(t *testing.T) {
	body := rt.Concat("abc", "def")
	b := FileBody(FileText(struct {
		ContentType string
		Body        string
	}{"text/plain", body}))
	s := BytesSlice(0, 3, b)
	a := BytesAppend(b, []byte{})
	c := BytesConcat([][]byte{b})
	for name, out := range map[string][]byte{"slice": s, "append": a, "concat": c} {
		if &out[0] == &b[0] {
			t.Errorf("Bytes.%s shares storage with the body", name)
		}
	}
	if body != "abcdef" {
		t.Fatal("the Text changed")
	}
}
