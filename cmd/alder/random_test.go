package main

import (
	"path/filepath"
	"strings"
	"testing"
)

// The corpus app random-ok pins, through a real page load, the numbers Text.hash and the seeded
// Random draw in `load` (#7): they are fixed forever, on every platform (CI is amd64, dev
// machines arm64). It also runs a Fuzz.fromRandom property.
func TestTest_RandomPinned(t *testing.T) {
	t.Parallel()
	needGoTool(t)
	src, err := filepath.Abs("../../testdata/corpus/random-ok")
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(t.TempDir(), "random-ok")
	copyDir(t, src, dir)
	code, stdout, stderr := runTest(t, dir)
	if code != 0 || !strings.Contains(stdout, "3 passed, 0 failed\n") || !strings.Contains(stdout, `ok   Home "load's draws are pinned"`) {
		t.Fatalf("exit %d\nstdout:\n%s\nstderr:\n%s", code, stdout, stderr)
	}
}
