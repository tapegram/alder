package testgocache

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fill writes files of the given sizes into dir (nested one level, as the go cache is).
func fill(t *testing.T, dir string, sizes ...int) {
	t.Helper()
	for i, n := range sizes {
		p := filepath.Join(dir, string(rune('a'+i)), "entry-d")
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, make([]byte, n), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func entries(t *testing.T, dir string) []string {
	t.Helper()
	des, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, de := range des {
		names = append(names, de.Name())
	}
	return names
}

func TestPrepare_UnderCapKeepsCache(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "cache")
	fill(t, dir, 300, 300)
	release, err := prepare(dir, 1000)
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	if got := entries(t, dir); len(got) != 2 {
		t.Fatalf("cache under its cap: entries %v, want both kept", got)
	}
}

func TestPrepare_OverCapWipesCache(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "cache")
	fill(t, dir, 600, 600)
	release, err := prepare(dir, 1000)
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	if got := entries(t, dir); len(got) != 0 {
		t.Fatalf("cache over its cap: entries %v, want an empty directory", got)
	}
	for _, name := range entries(t, root) {
		if name != "cache" && name != "cache.lock" {
			t.Errorf("left beside the cache: %s", name)
		}
	}
}

func TestPrepare_CreatesMissingCache(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "a", "cache")
	release, err := prepare(dir, 1000)
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	if fi, err := os.Stat(dir); err != nil || !fi.IsDir() {
		t.Fatalf("cache directory: %v", err)
	}
}

// A wipe interrupted by a crash leaves its renamed copy behind; the next wipe removes it.
func TestPrepare_WipeRemovesLeftovers(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "cache")
	fill(t, dir, 600, 600)
	fill(t, dir+".wipe-123", 10)
	release, err := prepare(dir, 1000)
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	if _, err := os.Stat(dir + ".wipe-123"); !os.IsNotExist(err) {
		t.Fatalf("leftover of an earlier wipe: %v, want it removed", err)
	}
}

// While another test binary uses the cache (holds the lock shared), an over-cap cache is left
// alone; the first run after it wipes it.
func TestPrepare_NoWipeWhileInUse(t *testing.T) {
	if !flockWorks {
		t.Skip("no flock on this platform")
	}
	dir := filepath.Join(t.TempDir(), "cache")
	fill(t, dir, 600, 600)
	other, err := prepare(dir, 1<<40) // the other binary: a cap it is under
	if err != nil {
		t.Fatal(err)
	}
	release, err := prepare(dir, 1000)
	if err != nil {
		t.Fatal(err)
	}
	release()
	if got := entries(t, dir); len(got) != 2 {
		t.Fatalf("cache in use by another binary: entries %v, want both kept", got)
	}
	other()
	release, err = prepare(dir, 1000)
	if err != nil {
		t.Fatal(err)
	}
	release()
	if got := entries(t, dir); len(got) != 0 {
		t.Fatalf("cache over its cap, no longer in use: entries %v, want it wiped", got)
	}
}

func TestSetup_SetsGOCACHE(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "cache")
	t.Setenv("GOCACHE", "/somewhere/else")
	t.Setenv(DirEnv, dir)
	t.Setenv(MaxEnv, "0.000001") // about 1 KB
	fill(t, dir, 600, 600)
	release, err := Setup()
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	if got := os.Getenv("GOCACHE"); got != dir {
		t.Errorf("GOCACHE = %q, want %q", got, dir)
	}
	if got := entries(t, dir); len(got) != 0 {
		t.Errorf("cache over the cap %s sets: entries %v, want it wiped", MaxEnv, got)
	}
}

func TestSetup_DefaultDir(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("TMPDIR", tmp) // os.TempDir on unix
	t.Setenv("TMP", tmp)    // and on Windows
	t.Setenv("GOCACHE", "")
	t.Setenv(DirEnv, "")
	release, err := Setup()
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	if got, want := os.Getenv("GOCACHE"), filepath.Join(tmp, "alder-test-gocache"); got != want {
		t.Errorf("GOCACHE = %q, want %q", got, want)
	}
}

func TestSetup_BadSettings(t *testing.T) {
	for _, c := range []struct{ dir, max, want string }{
		{"relative/cache", "", "absolute"},
		{"", "lots", MaxEnv},
		{"", "0", MaxEnv},
		{"", "-1", MaxEnv},
	} {
		t.Setenv("GOCACHE", "")
		if c.dir == "" {
			c.dir = filepath.Join(t.TempDir(), "cache")
		}
		t.Setenv(DirEnv, c.dir)
		t.Setenv(MaxEnv, c.max)
		release, err := Setup()
		if err == nil {
			release()
			t.Errorf("%s=%q %s=%q: no error", DirEnv, c.dir, MaxEnv, c.max)
			continue
		}
		if !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s=%q %s=%q: error %q, want it to mention %q", DirEnv, c.dir, MaxEnv, c.max, err, c.want)
		}
	}
}

// A huge cap is clamped, not overflowed into a tiny one that wipes on every run.
func TestSetup_HugeCapKeepsCache(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "cache")
	t.Setenv("GOCACHE", "")
	t.Setenv(DirEnv, dir)
	t.Setenv(MaxEnv, "1e300")
	fill(t, dir, 600, 600)
	release, err := Setup()
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	if got := entries(t, dir); len(got) != 2 {
		t.Fatalf("%s=1e300: entries %v, want both kept", MaxEnv, got)
	}
}
