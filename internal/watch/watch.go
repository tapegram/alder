package watch

import (
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Change is what the watcher saw change: sources (a rebuild), static files (a reload), or both.
type Change int

const (
	SourceChange Change = 1 << iota // app/**/*.alder, adapters/** or migrations/ (*.sql, schema.json)
	StaticChange                    // static/**
)

// DefaultInterval is how often `alder run`, the REPL and `alder scratch --watch` poll.
const DefaultInterval = 300 * time.Millisecond

// Stamp is what the watcher compares of a file.
type Stamp struct {
	mod  time.Time
	size int64
}

// Watch polls dir every interval until stop closes: the files app/**/*.alder, adapters/**,
// migrations/*.sql, migrations/schema.json and static/**. A file added, removed or modified is a change; the changes are coalesced and
// sent on changed once a poll finds nothing new after a poll that did, so a burst of saves is
// one change.
func Watch(dir string, interval time.Duration, stop <-chan struct{}, changed chan<- Change) {
	WatchWith(dir, nil, interval, stop, changed)
}

// WatchWith is Watch that also compares the stamps of the files in extra (absolute paths, such as
// a scratch file outside app/). A change to one, or its appearing or vanishing, is a SourceChange.
func WatchWith(dir string, extra []string, interval time.Duration, stop <-chan struct{}, changed chan<- Change) {
	snap := func() map[string]Stamp {
		m := Snapshot(dir)
		for _, f := range extra {
			if info, err := os.Stat(f); err == nil {
				m[f] = Stamp{info.ModTime(), info.Size()}
			}
		}
		return m
	}
	prev := snap()
	var pending Change
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-stop:
			return
		case <-t.C:
		}
		cur := snap()
		if k := Diff(prev, cur); k != 0 {
			pending |= k
			prev = cur
			continue
		}
		if pending != 0 {
			select {
			case changed <- pending:
			case <-stop:
				return
			}
			pending = 0
		}
	}
}

// EditorNoise reports whether name, a file or directory under adapters/, is an editor's or the
// system's, not the user's Go: a name starting with '.' (.x.go.swp, .DS_Store, .#x.go, .git),
// ending in '~', ".swp" or ".swx", or of the form #…#. A change to one does not rebuild.
func EditorNoise(name string) bool {
	return strings.HasPrefix(name, ".") || strings.HasSuffix(name, "~") || strings.HasSuffix(name, ".swp") ||
		strings.HasSuffix(name, ".swx") || (len(name) > 1 && strings.HasPrefix(name, "#") && strings.HasSuffix(name, "#"))
}

// Snapshot stamps the watched files of dir, keyed by path (relative to dir, slash-separated):
// app/**/*.alder, every file under static/, migrations/*.sql and schema.json, and the files
// under adapters/ but editor noise (EditorNoise) and symbolic links, which the build does not
// copy.
func Snapshot(dir string) map[string]Stamp {
	files := map[string]Stamp{}
	add := func(path string, d fs.DirEntry) {
		if info, err := d.Info(); err == nil {
			if rel, err := filepath.Rel(dir, path); err == nil {
				files[filepath.ToSlash(rel)] = Stamp{info.ModTime(), info.Size()}
			}
		}
	}
	for _, sub := range []string{"app", "adapters", "static"} {
		_ = filepath.WalkDir(filepath.Join(dir, sub), func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return nil // vanished meanwhile, or unreadable: skipped
			}
			if sub == "adapters" && path != filepath.Join(dir, sub) {
				if EditorNoise(d.Name()) {
					if d.IsDir() {
						return filepath.SkipDir
					}
					return nil
				}
				if d.Type()&fs.ModeSymlink != 0 {
					return nil
				}
			}
			if !d.IsDir() && (sub != "app" || strings.HasSuffix(path, ".alder")) {
				add(path, d)
			}
			return nil
		})
	}
	entries, _ := os.ReadDir(filepath.Join(dir, "migrations"))
	for _, e := range entries {
		if !e.IsDir() && (strings.HasSuffix(e.Name(), ".sql") || e.Name() == "schema.json") {
			add(filepath.Join(dir, "migrations", e.Name()), e)
		}
	}
	return files
}

// Diff is the kinds of the files that differ between two snapshots.
func Diff(a, b map[string]Stamp) Change {
	if maps.Equal(a, b) {
		return 0
	}
	var k Change
	kind := func(path string) Change {
		if strings.HasPrefix(path, "static/") {
			return StaticChange
		}
		return SourceChange
	}
	for p, s := range a {
		if t, ok := b[p]; !ok || t != s {
			k |= kind(p)
		}
	}
	for p := range b {
		if _, ok := a[p]; !ok {
			k |= kind(p)
		}
	}
	return k
}
