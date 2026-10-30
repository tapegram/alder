package buildkit

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"alder/rt/web"
)

// The build writes the al.css the router serves: with a theme, the same bytes as
// web.ThemedStylesheet; WrittenTheme reads the theme back (alder run's static-only refresh).
func TestWriteDesignSystem_Theme(t *testing.T) {
	const theme = "/* theme: App */\n:root { --radius: 0.5rem; }\n"
	dir := t.TempDir()
	if err := CopyStatic(filepath.Join(dir, "none"), filepath.Join(dir, "static"), theme); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(dir, "static", "al", "al.css"))
	if err != nil {
		t.Fatal(err)
	}
	served, _ := web.ThemedStylesheet(theme)
	if !bytes.Equal(got, served) {
		t.Fatalf("on-disk al.css (%d bytes) differs from the served one (%d bytes)", len(got), len(served))
	}
	if w := WrittenTheme(filepath.Join(dir, "static")); w != theme {
		t.Fatalf("WrittenTheme = %q", w)
	}
	if err := CopyStatic(filepath.Join(dir, "none"), filepath.Join(dir, "plain"), ""); err != nil {
		t.Fatal(err)
	}
	if w := WrittenTheme(filepath.Join(dir, "plain")); w != "" {
		t.Fatalf("WrittenTheme without a theme = %q", w)
	}
}
