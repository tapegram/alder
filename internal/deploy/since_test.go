package deploy

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"alder/internal/ir"
	"alder/internal/renderfake"
)

// `alder logs --since` (#92): each target's reach back.

// Logs with a since reads Render's log API forward from it, page after full page, and stops on
// a short page; past RenderSincePages pages it stops with a note on Out.
func TestLogs_RenderSincePages(t *testing.T) {
	g := newRenderRig(t)
	g.mustDeploy(g.target())
	t0 := time.Date(2026, 10, 29, 16, 0, 0, 0, time.UTC)
	mk := func(n int) []renderfake.LogLine {
		var ls []renderfake.LogLine
		for i := range n {
			ls = append(ls, renderfake.LogLine{Time: t0.Add(time.Duration(i) * time.Second), Instance: "i-1", Message: fmt.Sprintf("line %d", i)})
		}
		return ls
	}
	g.render.Logs(mk(260)...)
	since := t0.Add(10 * time.Second)
	starts := func() []string {
		var s []string
		for _, c := range g.render.Calls() {
			if c.Path == "/logs" && c.Query.Get("direction") == "forward" {
				s = append(s, c.Query.Get("startTime"))
			}
		}
		return s
	}
	var w bytes.Buffer
	r := g.connected()
	var note bytes.Buffer
	r.Out = &note
	note.Reset()
	if err := r.Logs(context.Background(), false, 0, since, &w); err != nil {
		t.Fatal(err)
	}
	if got := strings.Count(w.String(), "\n"); got != 250 || !strings.HasPrefix(w.String(), "[i-1] line 10\n") || !strings.HasSuffix(w.String(), "[i-1] line 259\n") {
		t.Errorf("%d lines:\n%.200s", got, w.String())
	}
	if s := starts(); len(s) != 3 || s[0] != since.Format(time.RFC3339Nano) {
		t.Errorf("pages from %v", s)
	}
	if strings.Contains(note.String(), "cut") {
		t.Errorf("note without a cut: %s", note.String())
	}

	// the cap: 20 pages, then the note
	g.render.Logs(mk(RenderSincePages*RenderLogMax + 150)...)
	w.Reset()
	note.Reset()
	if err := r.Logs(context.Background(), false, 0, t0, &w); err != nil {
		t.Fatal(err)
	}
	if got := strings.Count(w.String(), "\n"); got != RenderSincePages*RenderLogMax {
		t.Errorf("%d lines past the cap", got)
	}
	if want := "alder logs: Render gave 2000 lines since 2026-10-29T16:00:00Z; the rest is cut: use a later --since\n"; note.String() != want {
		t.Errorf("note %q", note.String())
	}
}

// flyctl has no time flag: with a since Logs writes everything `flyctl logs --no-tail` prints,
// whatever n (alder logs keeps the lines since by their stamps).
func TestLogs_FlySince(t *testing.T) {
	ff := newFakeFly(t)
	var lines []string
	for i := 1; i <= 150; i++ {
		lines = append(lines, fmt.Sprintf("2026-10-29T16:%02d:%02dZ app[x] iad [info]line %d", i/60, i%60, i))
	}
	ff.logs = strings.Join(lines, "\n") + "\n"
	var w bytes.Buffer
	if err := ff.target(nil).Logs(context.Background(), false, 0, time.Date(2026, 10, 29, 16, 1, 0, 0, time.UTC), &w); err != nil {
		t.Fatal(err)
	}
	if w.String() != ff.logs {
		t.Errorf("%d bytes, want all %d", w.Len(), len(ff.logs))
	}
}

// With a since (or n 0) the script reads the whole log (cat), not its tail: every line reaches
// alder logs' filter.
func TestSsh_LogsSince(t *testing.T) {
	t.Parallel()
	ssh := filepath.Join(t.TempDir(), "ssh")
	if err := os.WriteFile(ssh, []byte(loopbackSSH), 0o755); err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "builds", "b1"), 0o700); err != nil {
		t.Fatal(err)
	}
	var lines []string
	for i := 1; i <= 300; i++ {
		lines = append(lines, fmt.Sprintf("2026/10/29 16:%02d:%02d info: line %d", i/60, i%60, i))
	}
	all := strings.Join(lines, "\n") + "\n"
	os.WriteFile(filepath.Join(root, "builds", "b1", "log"), []byte(all), 0o600)
	os.WriteFile(filepath.Join(root, "current"), []byte("b1\n"), 0o600)
	s := NewSsh("blog", "production", ir.Target{Host: "deploy@box", Path: root}, &ir.Deploy{Proxy: "None"})
	s.SSH = ssh
	for _, c := range []struct {
		n     int
		since time.Time
	}{{0, time.Date(2026, 10, 29, 16, 2, 0, 0, time.UTC)}, {0, time.Time{}}, {5, time.Unix(1, 0)}} {
		var w bytes.Buffer
		if err := s.Logs(context.Background(), false, c.n, c.since, &w); err != nil {
			t.Fatal(err)
		}
		if w.String() != all {
			t.Errorf("n %d since %v: %d bytes, want %d", c.n, c.since, w.Len(), len(all))
		}
	}
}

// --since -f at a box: the whole log first (cat), then what is appended, and the build that
// becomes current from its start; every line reaches alder logs' filter.
func TestSsh_LogsSinceFollow(t *testing.T) {
	t.Parallel()
	ssh := filepath.Join(t.TempDir(), "ssh")
	if err := os.WriteFile(ssh, []byte(loopbackSSH), 0o755); err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	for _, b := range []string{"b1", "b2"} {
		if err := os.MkdirAll(filepath.Join(root, "builds", b), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	var lines []string
	for i := 1; i <= 150; i++ {
		lines = append(lines, fmt.Sprintf("2026/10/29 16:%02d:%02d info: line %d", i/60, i%60, i))
	}
	log1 := filepath.Join(root, "builds", "b1", "log")
	os.WriteFile(log1, []byte(strings.Join(lines, "\n")+"\n"), 0o600)
	os.WriteFile(filepath.Join(root, "builds", "b2", "log"), []byte("2026/10/29 17:00:00 info: b2 first\n"), 0o600)
	os.WriteFile(filepath.Join(root, "current"), []byte("b1\n"), 0o600)
	s := NewSsh("blog", "production", ir.Target{Host: "deploy@box", Path: root}, &ir.Deploy{Proxy: "None"})
	s.SSH, s.Poll = ssh, 50*time.Millisecond
	var w syncBuffer
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- s.Logs(ctx, true, 0, time.Date(2026, 10, 29, 16, 2, 0, 0, time.UTC), &w) }()
	wait := func(what string) {
		t.Helper()
		deadline := time.Now().Add(10 * time.Second)
		for !strings.Contains(w.String(), what) {
			if time.Now().After(deadline) {
				t.Fatalf("no %q within 10 s:\n%s", what, w.String())
			}
			time.Sleep(20 * time.Millisecond)
		}
	}
	wait("info: line 150\n")
	if !strings.HasPrefix(w.String(), "2026/10/29 16:00:01 info: line 1\n") {
		t.Errorf("not the whole log first:\n%.200s", w.String())
	}
	f, err := os.OpenFile(log1, os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	f.WriteString("2026/10/29 16:03:00 info: appended\n")
	f.Close()
	wait("info: appended\n")
	os.WriteFile(filepath.Join(root, "current"), []byte("b2\n"), 0o600)
	wait("--- alder logs: build b2 is current\n2026/10/29 17:00:00 info: b2 first\n")
	cancel()
	<-done // the loopback's sh is killed: an error, which alder logs ignores once ctx has ended
}
