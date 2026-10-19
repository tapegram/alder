package generate

import (
	"os"
	"path/filepath"
)

// Materialise copies the app in dir to a temporary directory (as Apply's copy: no build/, .git
// or databases) and writes overlay over it: an editor's unsaved buffers, keyed by path relative
// to dir. A dry run on the copy then sees what the editor sees. cleanup removes the copy.
func Materialise(dir string, overlay map[string][]byte) (string, func(), error) {
	tmp, err := os.MkdirTemp("", "alder-overlay-")
	if err != nil {
		return "", nil, err
	}
	cleanup := func() { os.RemoveAll(tmp) }
	if err := copyApp(dir, tmp, nil); err != nil {
		cleanup()
		return "", nil, err
	}
	for rel, b := range overlay {
		p := filepath.Join(tmp, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			cleanup()
			return "", nil, err
		}
		if err := os.WriteFile(p, b, 0o644); err != nil {
			cleanup()
			return "", nil, err
		}
	}
	return tmp, cleanup, nil
}
