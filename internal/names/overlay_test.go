// internal/names/overlay_test.go
package names

import (
	"os"
	"path/filepath"
	"testing"

	"alder/internal/types"
)

func writeApp(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for p, s := range files {
		full := filepath.Join(dir, p)
		os.MkdirAll(filepath.Dir(full), 0o755)
		if err := os.WriteFile(full, []byte(s), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func TestLoadOverlay_ReplacesDiskFile(t *testing.T) {
	dir := writeApp(t, map[string]string{"app/A.alder": "module A\n\nx : Int\nx = 1\n"})
	std, err := types.LoadStd()
	if err != nil {
		t.Fatal(err)
	}
	prog, bag, ioErrs := LoadOverlay(dir, std, map[string][]byte{"app/A.alder": []byte("module A\n\ny : Int\ny = 2\n")})
	if len(ioErrs) > 0 || bag.HasErrors() {
		t.Fatalf("load: %v %v", ioErrs, bag.Items)
	}
	m := prog.Modules["A"]
	if m == nil || len(m.Decls) != 1 || m.Lines[2] != "y : Int" {
		t.Fatalf("overlay not used: %+v", m)
	}
	if len(prog.Modules) != 1 {
		t.Fatalf("overlay duplicated the module: %d modules", len(prog.Modules))
	}
}

func TestLoadOverlay_AddsNewFile(t *testing.T) {
	dir := writeApp(t, map[string]string{"app/A.alder": "module A\n"})
	std, _ := types.LoadStd()
	prog, _, _ := LoadOverlay(dir, std, map[string][]byte{"app/B/C.alder": []byte("module B.C\n")})
	if prog.Modules["B.C"] == nil || prog.Modules["B.C"].File != "app/B/C.alder" {
		t.Fatalf("new overlay file not loaded: %v", prog.Order)
	}
}

func TestLoadOverlay_IgnoresOtherPaths(t *testing.T) {
	dir := writeApp(t, map[string]string{"app/A.alder": "module A\n"})
	std, _ := types.LoadStd()
	prog, _, _ := LoadOverlay(dir, std, map[string][]byte{"migrations/schema.json": []byte("{}"), "app/notes.txt": []byte("x")})
	if len(prog.Modules) != 1 {
		t.Fatalf("non-.alder overlay loaded: %v", prog.Order)
	}
}

func TestLoadOverlay_ParseErrorInOverlay(t *testing.T) {
	dir := writeApp(t, map[string]string{"app/A.alder": "module A\n"})
	std, _ := types.LoadStd()
	prog, bag, _ := LoadOverlay(dir, std, map[string][]byte{"app/A.alder": []byte("module A\n\nx = \"unterminated\n")})
	if !prog.ParseFailed || !bag.HasErrors() || bag.Items[0].Span.File != "app/A.alder" {
		t.Fatalf("want a parse failure in app/A.alder, got %+v", bag.Items)
	}
}
