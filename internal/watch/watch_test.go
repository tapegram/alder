package watch

import (
	"os"
	"path/filepath"
	"strings"
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

// expectChange runs step every 20 ms (with its count) until a change arrives or the deadline
// passes, so it does not depend on when the watcher took its first snapshot, then waits for the
// channel to go quiet.
func expectChange(t *testing.T, what string, ch <-chan Change, step func(i int)) Change {
	t.Helper()
	deadline := time.After(5 * time.Second)
	tick := time.NewTicker(20 * time.Millisecond)
	defer tick.Stop()
	for i := 0; ; i++ {
		step(i)
		select {
		case c := <-ch:
			for quiet := false; !quiet; {
				select {
				case <-ch:
				case <-time.After(100 * time.Millisecond):
					quiet = true
				}
			}
			return c
		case <-deadline:
			t.Fatalf("%s: no change seen", what)
		case <-tick.C:
		}
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

	c := expectChange(t, "edit", ch, func(i int) {
		os.WriteFile(extra, []byte("> 1"+strings.Repeat("0", i+1)+"\n"), 0o644)
	})
	if c&SourceChange == 0 {
		t.Fatalf("edit: change %v", c)
	}
	c = expectChange(t, "remove", ch, func(int) { os.Remove(extra) })
	if c&SourceChange == 0 {
		t.Fatalf("remove: change %v", c)
	}
	c = expectChange(t, "recreate", ch, func(i int) {
		os.WriteFile(extra, []byte("> 2"+strings.Repeat("0", i)+"\n"), 0o644)
	})
	if c&SourceChange == 0 {
		t.Fatalf("recreate: change %v", c)
	}
}
