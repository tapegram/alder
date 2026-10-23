package main

import (
	"io"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The dev server keeps check diagnostics structured: file, line, col, code, message and the
// source excerpt with its caret.
func TestParseDiags(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "app"), 0o755); err != nil {
		t.Fatal(err)
	}
	src := "module X\n\tx = a <b> + 1\n"
	if err := os.WriteFile(filepath.Join(dir, "app", "X.alder"), []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	json := `{"code":"E0003","severity":"error","file":"app/X.alder","line":2,"col":6,"endLine":2,"endCol":11,"message":"expected Text, got Int"}` + "\n" +
		`{"code":"W0002","severity":"warning","file":"app/X.alder","line":1,"col":1,"endLine":1,"endCol":2,"message":"hole"}` + "\n" +
		`{"code":"E0001","severity":"error","file":"app/Gone.alder","line":9,"col":1,"endLine":9,"endCol":2,"message":"no such file"}` + "\n"
	ds := parseDiags(dir, json)
	if len(ds) != 2 {
		t.Fatalf("want the 2 errors (the warning is dropped), got %+v", ds)
	}
	d := ds[0]
	if d.Code != "E0003" || d.File != "app/X.alder" || d.Line != 2 || d.Col != 6 || d.Message != "expected Text, got Int" {
		t.Fatalf("diagnostic: %+v", d)
	}
	if want := "\tx = a <b> + 1\n\t    ^^^^^"; d.Excerpt != want {
		t.Fatalf("excerpt %q, want %q", d.Excerpt, want)
	}
	if ds[1].Excerpt != "" {
		t.Fatalf("a missing file has no excerpt: %q", ds[1].Excerpt)
	}
	if got := parseDiags(dir, "alder check: internal error\n"); len(got) != 0 {
		t.Fatalf("text is not diagnostics: %+v", got)
	}
}

func TestErrorsHTML(t *testing.T) {
	t.Parallel()
	st := errState{kind: "check", diags: []diagItem{
		{Code: "E0003", File: "app/X.alder", Line: 2, Col: 6, Message: "expected <Text>, got Int", Excerpt: "x = a\n    ^"},
		{Code: "E0007", File: "app/Y.alder", Line: 1, Col: 1, Message: "second"},
	}}
	h := errorsHTML(st)
	for _, want := range []string{
		`<div class="_alder-strip" role="alert">`, "Build failed: 2 errors. The page is the last good build.", "data-open-errors",
		`<div class="_alder-errs" data-count="2" data-kind="check">`,
		`<code class="_alder-at">app/X.alder:2:6</code>`, `<b class="_alder-code">E0003</b>`, `expected &lt;Text&gt;, got Int`,
		"<pre>x = a\n    ^</pre>", "app/Y.alder:1:1",
	} {
		if !strings.Contains(h, want) {
			t.Errorf("missing %q in:\n%s", want, h)
		}
	}
	if one := errorsHTML(errState{kind: "check", diags: st.diags[:1]}); !strings.Contains(one, "Build failed: 1 error. ") {
		t.Errorf("singular: %s", one)
	}
}

// A build failure or a crash has no structure: its text is one block, escaped.
func TestErrorsHTML_TextFallback(t *testing.T) {
	t.Parallel()
	h := errorsHTML(errState{kind: "build", text: "alder run: <boom>\nline 2\n"})
	for _, want := range []string{"Build failed: 1 error. The page is the last good build.", `data-count="1"`, "<pre>alder run: &lt;boom&gt;\nline 2</pre>"} {
		if !strings.Contains(h, want) {
			t.Errorf("missing %q in:\n%s", want, h)
		}
	}
	c := errorsHTML(errState{kind: "crash", text: "alder run: the app exited"})
	if strings.Contains(c, "last good build") || !strings.Contains(c, "The app exited") {
		t.Errorf("crash strip: %s", c)
	}
	if errorsHTML(errState{}) != "" {
		t.Errorf("no error, no content")
	}
}

// A diagnostic's file, message and excerpt come from the user's files: the strip, the list (which
// the panel's Errors tab copies) and the 502 page show them escaped, never as markup.
func TestErrorsHTML_Escapes(t *testing.T) {
	t.Parallel()
	st := errState{kind: "check", diags: []diagItem{{
		Code: "E0003", File: "app/<img src=x>.alder", Line: 1, Col: 1,
		Message: `bad <b onclick="x">`, Excerpt: "<script>alert(1)</script>\n^",
	}}}
	check := func(where, h string) {
		t.Helper()
		for _, bad := range []string{"<script>alert", "<img src=x>", `<b onclick`} {
			if strings.Contains(h, bad) {
				t.Errorf("%s: unescaped %q in:\n%s", where, bad, h)
			}
		}
		for _, want := range []string{"&lt;script&gt;alert(1)&lt;/script&gt;", "app/&lt;img src=x&gt;.alder:1:1", "bad &lt;b onclick=&#34;x&#34;&gt;"} {
			if !strings.Contains(h, want) {
				t.Errorf("%s: missing %q in:\n%s", where, want, h)
			}
		}
	}
	check("strip and panel list", errorsHTML(st))
	check("patch event", string(overlayEvent(st)))
	w := httptest.NewRecorder()
	newDevServer(".", "app", "localhost:0", io.Discard, io.Discard).unavailable(w, st, "g-1")
	check("502 page", w.Body.String())
	// the text fallback is escaped too
	if h := errorsHTML(errState{kind: "build", text: "<script>alert(1)</script>"}); strings.Contains(h, "<script>alert") {
		t.Errorf("text fallback unescaped: %s", h)
	}
}
