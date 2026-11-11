package migrate

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// SQLFiles are the paths of the migration files (*.sql) directly in dir, sorted; nil when dir
// does not exist. It reads the directory rather than globbing it: filepath.Glob takes the
// metacharacters ([ * ? \) of the app's own path as a pattern and finds nothing (#172).
func SQLFiles(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var files []string
	for _, e := range entries { // sorted by name
		if name := e.Name(); e.Type().IsRegular() && len(name) > len(".sql") && strings.HasSuffix(name, ".sql") {
			files = append(files, filepath.Join(dir, name))
		}
	}
	return files, nil
}
