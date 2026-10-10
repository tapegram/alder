//go:build !unix

package scratch

import (
	"os"
	"path/filepath"
)

// lockFile: without flock, Compiles into one directory are not serialized; the lock file is
// still created so its location is the same everywhere.
func lockFile(path string) (func(), error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0o644)
	if err != nil {
		return nil, err
	}
	return func() { f.Close() }, nil
}
