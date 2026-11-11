package migrate

import (
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"testing"
)

func sqlMust(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

func TestSQLFiles(t *testing.T) {
	for _, parent := range []string{"plain", "e2e-[x]-abc", "a*b?c", "open[bracket"} {
		t.Run(parent, func(t *testing.T) {
			if parent == "a*b?c" && runtime.GOOS == "windows" {
				t.Skip("* and ? are not allowed in Windows paths")
			}
			dir := filepath.Join(t.TempDir(), parent, "migrations")
			sqlMust(t, os.MkdirAll(filepath.Join(dir, "dir.sql"), 0o755)) // a directory: not a migration
			for _, n := range []string{"0002_b.sql", "0001_a.sql", "schema.json", ".sql", "notes.sql.bak"} {
				sqlMust(t, os.WriteFile(filepath.Join(dir, n), nil, 0o644))
			}
			got, err := SQLFiles(dir)
			want := []string{filepath.Join(dir, "0001_a.sql"), filepath.Join(dir, "0002_b.sql")}
			if err != nil || !slices.Equal(got, want) {
				t.Fatalf("SQLFiles = %v, %v; want %v", got, err, want)
			}
		})
	}
}

func TestSQLFiles_MissingDir(t *testing.T) {
	got, err := SQLFiles(filepath.Join(t.TempDir(), "none"))
	if got != nil || err != nil {
		t.Fatalf("SQLFiles = %v, %v; want nil, nil", got, err)
	}
}
