package watch

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// eventually polls cond every few milliseconds until it holds, for up to d, and reports
// whether it did.
func eventually(t *testing.T, d time.Duration, cond func() bool) bool {
	t.Helper()
	for deadline := time.Now().Add(d); ; time.Sleep(5 * time.Millisecond) {
		if cond() {
			return true
		}
		if time.Now().After(deadline) {
			return false
		}
	}
}

// signalled reads a signal from ch without blocking, and reports whether there was one.
func signalled(ch <-chan struct{}) bool {
	select {
	case _, ok := <-ch:
		return ok
	default:
		return false
	}
}

// quiet reports whether ch gives no signal within d. It can only fail by a signal arriving, so a
// slow machine makes it pass, never fail.
func quiet(ch <-chan struct{}, d time.Duration) bool {
	select {
	case _, ok := <-ch:
		return !ok
	case <-time.After(d):
		return true
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
	const every, debounce = 10 * time.Millisecond, 100 * time.Millisecond
	root := t.TempDir()
	appendTo(t, filepath.Join(root, "app", "Books.al"))
	appendTo(t, filepath.Join(root, "static", "app.css"))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ch := Poll(ctx, root, []string{"app", "adapters", "migrations"}, every, debounce)
	// A burst of writes signals once, after it.
	for range 3 {
		appendTo(t, filepath.Join(root, "app", "Books.al"))
	}
	if !eventually(t, 2*time.Second, func() bool { return signalled(ch) }) {
		t.Fatal("a burst of writes under app/ gave no signal within 2s")
	}
	if !quiet(ch, 3*debounce) {
		t.Error("a burst of writes gave more than one signal")
	}
	appendTo(t, filepath.Join(root, "static", "app.css"))
	if !quiet(ch, 3*debounce) {
		t.Error("a write under static/ gave a signal")
	}
	cancel()
	if !eventually(t, 2*time.Second, func() bool {
		select {
		case _, ok := <-ch:
			return !ok
		default:
			return false
		}
	}) {
		t.Error("the channel is not closed once the context ends")
	}
}
