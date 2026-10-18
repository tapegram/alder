package lsp

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestDiagnostics_OverlayTypeError(t *testing.T) {
	c := start(t, blogCopy(t))
	text := c.disk("app/Posts/Index.alder")
	c.open("app/Posts/Index.alder", text)
	if ds := c.diags("app/Posts/Index.alder"); len(ds) != 0 {
		t.Fatalf("clean file has %v", ds)
	}
	c.change("app/Posts/Index.alder", strings.Replace(text, "perPage = 10", `perPage = "ten"`, 1))
	ds := c.diags("app/Posts/Index.alder")
	if len(ds) == 0 || ds[0].Code != "E0003" || ds[0].Source != "alder" || ds[0].Severity != 1 {
		t.Fatalf("want E0003, got %+v", ds)
	}
	if want := c.at("app/Posts/Index.alder", `"ten"`, 0); ds[0].Range.Start.Line != want.Line {
		t.Fatalf("range %+v, want line %d", ds[0].Range, want.Line)
	}
}

func TestDiagnostics_HoleIsHint(t *testing.T) {
	c := start(t, blogCopy(t))
	text := strings.Replace(c.disk("app/Posts/Index.alder"), "Db.limit (perPage + 1)", "Db.limit ?n", 1)
	c.open("app/Posts/Index.alder", text)
	var hole *Diagnostic
	for _, d := range c.diags("app/Posts/Index.alder") {
		if d.Code == "W0002" {
			d := d
			hole = &d
		}
	}
	if hole == nil || hole.Severity != 4 || !strings.Contains(hole.Message, "?n") || !strings.Contains(hole.Message, "Int") {
		t.Fatalf("hole diagnostic %+v", hole)
	}
	if want := c.at("app/Posts/Index.alder", "?n", 0); hole.Range.Start != want {
		t.Fatalf("hole range %+v, want start %+v", hole.Range, want)
	}
}

func TestDiagnostics_NotOpenFile(t *testing.T) {
	c := start(t, blogCopy(t))
	// Posts/Index uses Post.title; renaming the field in the open Post.alder breaks the closed file
	text := c.disk("app/Post.alder")
	c.open("app/Post.alder", strings.Replace(text, "title:", "heading:", 1))
	if ds := c.waitPublished("app/Posts/Index.alder"); len(ds) == 0 {
		t.Fatal("no diagnostics published for the file that is not open")
	}
	c.change("app/Post.alder", text)
	if ds := c.waitPublished("app/Posts/Index.alder"); len(ds) != 0 {
		t.Fatalf("the closed file kept %v after the fix", ds)
	}
}

func TestDiagnostics_FixedFileClears(t *testing.T) {
	c := start(t, blogCopy(t))
	text := c.disk("app/Post.alder")
	c.open("app/Post.alder", strings.Replace(text, "module Post", "module Post\n\nbroken : Int\nbroken = \"x\"", 1))
	c.diags("app/Post.alder")
	c.change("app/Post.alder", text)
	if ds := c.diags("app/Post.alder"); len(ds) != 0 {
		t.Fatalf("fixed file kept %v", ds)
	}
}

// TestDiagnostics_Close: a closed buffer's text no longer applies; the disk text does again.
func TestDiagnostics_Close(t *testing.T) {
	c := start(t, blogCopy(t))
	text := c.disk("app/Post.alder")
	c.open("app/Post.alder", strings.Replace(text, "module Post", "module Post\n\nbroken : Int\nbroken = \"x\"", 1))
	if ds := c.diags("app/Post.alder"); len(ds) == 0 {
		t.Fatal("no diagnostics for the broken buffer")
	}
	c.mu.Lock()
	delete(c.vers, "app/Post.alder")
	c.mark = c.pubs
	c.mu.Unlock()
	c.notify("textDocument/didClose", map[string]any{"textDocument": map[string]any{"uri": c.uri("app/Post.alder")}})
	if ds := c.waitPublished("app/Post.alder"); len(ds) != 0 {
		t.Fatalf("the closed buffer's diagnostics stayed: %v", ds)
	}
}

func TestLastGood(t *testing.T) {
	c := start(t, blogCopy(t))
	text := c.disk("app/Posts/Index.alder")
	c.open("app/Posts/Index.alder", text)
	c.diags("app/Posts/Index.alder")
	broken := text + "\nhalf = \"unterminated\n"
	c.change("app/Posts/Index.alder", broken)
	ds := c.diags("app/Posts/Index.alder")
	if len(ds) == 0 || ds[0].Code != "E0001" {
		t.Fatalf("want the parse error at once, got %+v", ds)
	}
	s := c.srv
	if s.current() == nil || s.current().res != nil {
		t.Fatal("the current analysis is not the failed parse")
	}
	if s.lastGood() == nil || s.lastGood().res == nil {
		t.Fatal("no last good build kept")
	}
	if !s.fresh(s.lastGood(), "app/Posts/Index.alder", 3, true) {
		t.Fatal("a line above the edit is not fresh")
	}
	if s.fresh(s.lastGood(), "app/Posts/Index.alder", len(strings.Split(broken, "\n"))-1, true) {
		t.Fatal("the edited line is fresh")
	}
	if s.fresh(s.lastGood(), "app/Posts/Index.alder", 1_000_000, true) {
		t.Fatal("a line past the end is fresh")
	}
}

// TestFresh_TypingBreaksTheLine: while the cursor's line is being typed (and does not parse),
// the lines above it are fresh for completion (exclusive) but the line itself is not for hover
// (inclusive).
func TestFresh_TypingBreaksTheLine(t *testing.T) {
	c := start(t, blogCopy(t))
	text := c.disk("app/Posts/Index.alder")
	c.open("app/Posts/Index.alder", text)
	c.diags("app/Posts/Index.alder")
	lines := strings.Split(text, "\n")
	lines[17] = "perPage = List." // line 18, "perPage = 10", half typed
	c.change("app/Posts/Index.alder", strings.Join(lines, "\n"))
	if ds := c.diags("app/Posts/Index.alder"); len(ds) == 0 || ds[0].Code != "E0001" {
		t.Fatalf("want a parse error, got %+v", ds)
	}
	s := c.srv
	good := s.lastGood()
	if good == nil || good.versions["app/Posts/Index.alder"] != 1 {
		t.Fatalf("the last good build is not version 1: %+v", good)
	}
	if !s.fresh(good, "app/Posts/Index.alder", 18, false) {
		t.Fatal("line 18 is not fresh for completion")
	}
	if s.fresh(good, "app/Posts/Index.alder", 18, true) {
		t.Fatal("line 18 is fresh for hover")
	}
	if s.fresh(good, "app/Posts/Index.alder", 19, false) {
		t.Fatal("line 19, below the edit, is fresh")
	}
	if s.fresh(good, "app/Missing.alder", 1, false) {
		t.Fatal("a module the build does not have is fresh")
	}
}

func TestDiagnostics_SymlinkRoot(t *testing.T) {
	real := blogCopy(t)
	link := filepath.Join(t.TempDir(), "via-link")
	if err := os.Symlink(real, link); err != nil {
		t.Skip(err)
	}
	c := start(t, link) // the client names the root through the link
	text := c.disk("app/Posts/Index.alder")
	c.openURI("file://"+filepath.Join(real, "app/Posts/Index.alder"), "app/Posts/Index.alder", strings.Replace(text, "perPage = 10", `perPage = "x"`, 1))
	if ds := c.diags("app/Posts/Index.alder"); len(ds) == 0 {
		t.Fatal("no diagnostics for a buffer opened through the real path")
	}
}

func TestDiskChangeRebuilds(t *testing.T) {
	root := blogCopy(t)
	c := startWith(t, root, Options{Poll: 20 * time.Millisecond})
	c.open("app/Posts/Index.alder", c.disk("app/Posts/Index.alder"))
	c.diags("app/Posts/Index.alder")
	os.WriteFile(filepath.Join(root, "app", "Post.alder"), []byte("module Post\n\nx = \n"), 0o644)
	if ds := c.waitPublished("app/Post.alder"); len(ds) == 0 {
		t.Fatal("a broken file on disk was not reported")
	}
}

// TestRel: a URI maps to a slash-separated key relative to the real root, through symbolic
// links and for files that do not exist yet; nothing outside the root has a key.
func TestRel(t *testing.T) {
	real := blogCopy(t)
	link := filepath.Join(t.TempDir(), "via-link")
	if err := os.Symlink(real, link); err != nil {
		t.Skip(err)
	}
	outside := t.TempDir()
	c := start(t, link)
	for _, tc := range []struct {
		uri, want string
		ok        bool
	}{
		{"file://" + filepath.Join(link, "app/Posts/Index.alder"), "app/Posts/Index.alder", true},
		{"file://" + filepath.Join(real, "app/Posts/Index.alder"), "app/Posts/Index.alder", true},
		{"file://" + filepath.Join(link, "app/New/Dir/Page.alder"), "app/New/Dir/Page.alder", true},
		{"file://" + filepath.Join(link, "app/../../escape.alder"), "", false},
		{"file://" + filepath.Join(outside, "x.alder"), "", false},
		{"file://" + link, "", false},
		{"untitled:Untitled-1", "", false},
	} {
		got, ok := c.srv.rel(tc.uri)
		if got != tc.want || ok != tc.ok {
			t.Errorf("rel(%s) = %q, %v; want %q, %v", tc.uri, got, ok, tc.want, tc.ok)
		}
	}
	if got := c.srv.uriOf("app/Post.alder"); got != "file://"+filepath.ToSlash(filepath.Join(link, "app/Post.alder")) {
		t.Errorf("uriOf a closed file = %s", got)
	}
}
