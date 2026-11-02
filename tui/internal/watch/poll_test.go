package watch

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// count is how many signals ch gives within d.
func count(ch <-chan struct{}, d time.Duration) int {
	n := 0
	deadline := time.After(d)
	for {
		select {
		case <-ch:
			n++
		case <-deadline:
			return n
		}
	}
}

// appendTo appends a line to the file at path, making it and its directory when missing.
func appendTo(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString("x\n"); err != nil {
		t.Fatal(err)
	}
	f.Close()
}

func TestPoll_Debounced(t *testing.T) {
	root := t.TempDir()
	appendTo(t, filepath.Join(root, "app", "Books.al"))
	appendTo(t, filepath.Join(root, "static", "app.css"))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ch := Poll(ctx, root, []string{"app", "adapters", "migrations"}, 20*time.Millisecond, 150*time.Millisecond)
	for range 3 {
		appendTo(t, filepath.Join(root, "app", "Books.al"))
		time.Sleep(50 * time.Millisecond)
	}
	if n := count(ch, 700*time.Millisecond); n != 1 {
		t.Errorf("three writes 50 ms apart gave %d signals, want 1", n)
	}
	appendTo(t, filepath.Join(root, "static", "app.css"))
	if n := count(ch, 400*time.Millisecond); n != 0 {
		t.Errorf("a write under static/ gave %d signals, want 0", n)
	}
	cancel()
	select {
	case _, ok := <-ch:
		if ok {
			t.Error("a signal after the context ended")
		}
	case <-time.After(time.Second):
		t.Error("the channel is not closed once the context ends")
	}
}
