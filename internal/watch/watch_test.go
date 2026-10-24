package watch

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestWatch_SourceChange(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "app"), 0o755)
	os.WriteFile(filepath.Join(dir, "app", "A.alder"), []byte("module A\n"), 0o644)
	stop := make(chan struct{})
	defer close(stop)
	ch := make(chan Change, 1)
	go Watch(dir, 10*time.Millisecond, stop, ch)
	time.Sleep(30 * time.Millisecond)
	os.WriteFile(filepath.Join(dir, "app", "B.alder"), []byte("module B\n"), 0o644)
	select {
	case c := <-ch:
		if c&SourceChange == 0 {
			t.Fatalf("change %v", c)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("no change seen")
	}
}

func TestWatchWithExtraFile(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "app"), 0o755)
	os.WriteFile(filepath.Join(dir, "app", "A.alder"), []byte("module A\n"), 0o644)
	extra := filepath.Join(dir, "scratch.alder")
	os.WriteFile(extra, []byte("> 1\n"), 0o644)
	stop := make(chan struct{})
	defer close(stop)
	ch := make(chan Change, 1)
	go WatchWith(dir, []string{extra}, 10*time.Millisecond, stop, ch)
	time.Sleep(30 * time.Millisecond)
	os.WriteFile(extra, []byte("> 22\n"), 0o644)
	select {
	case c := <-ch:
		if c&SourceChange == 0 {
			t.Fatalf("change %v", c)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("no change seen")
	}
}
