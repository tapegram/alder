// Package watch tells when the files of an app change, by polling: it needs nothing from the
// platform, so it works the same on macOS, Linux and Windows.
package watch

import (
	"context"
	"io/fs"
	"path/filepath"
	"time"
)

// stamp is what a snapshot keeps of a file: its size and modification time.
type stamp struct {
	size int64
	mod  time.Time
}

// snapshot is the stamp of every file under root/<dir> for each of dirs, by path. A directory
// that does not exist, or a file that cannot be read, is left out.
func snapshot(root string, dirs []string) map[string]stamp {
	snap := map[string]stamp{}
	for _, d := range dirs {
		filepath.WalkDir(filepath.Join(root, d), func(path string, e fs.DirEntry, err error) error {
			if err != nil || e.IsDir() {
				return nil
			}
			if info, err := e.Info(); err == nil {
				snap[path] = stamp{info.Size(), info.ModTime()}
			}
			return nil
		})
	}
	return snap
}

// same is whether two snapshots hold the same files with the same stamps.
func same(a, b map[string]stamp) bool {
	if len(a) != len(b) {
		return false
	}
	for p, s := range a {
		if t, ok := b[p]; !ok || t.size != s.size || !t.mod.Equal(s.mod) {
			return false
		}
	}
	return true
}

// Poll watches the files under root/<dir> for each of dirs (`app`, `adapters` and `migrations`
// for an app), taking a snapshot of their sizes and modification times every `every`. A change
// signals once the files have stayed as they are for debounce, so a burst of writes gives one
// signal. The first snapshot is taken before Poll returns, so a write made after it returns is
// seen. A signal not yet read is not doubled: the channel holds at most one. The channel closes
// once ctx ends.
func Poll(ctx context.Context, root string, dirs []string, every, debounce time.Duration) <-chan struct{} {
	ch := make(chan struct{}, 1)
	last := snapshot(root, dirs)
	go func() {
		defer close(ch)
		tick := time.NewTicker(every)
		defer tick.Stop()
		var changed time.Time // when the files last changed, zero once that change has signalled
		for {
			select {
			case <-ctx.Done():
				return
			case now := <-tick.C:
				snap := snapshot(root, dirs)
				switch {
				case !same(snap, last):
					last, changed = snap, now
				case !changed.IsZero() && now.Sub(changed) >= debounce:
					changed = time.Time{}
					select {
					case ch <- struct{}{}:
					default:
					}
				}
			}
		}
	}()
	return ch
}
