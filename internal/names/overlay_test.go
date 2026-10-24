// internal/names/overlay_test.go
package names

import (
	"os"
	"path/filepath"
	"testing"

	"alder/internal/diag"
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

func TestLoadScratchInApp(t *testing.T) {
	std, err := types.LoadStd()
	if err != nil {
		t.Fatal(err)
	}
	const rel = "app/Todos.scratch.alder"
	check := func(name string, dir string, overlay map[string][]byte) {
		prog, bag, _ := LoadOverlay(dir, std, overlay)
		if len(bag.Items) != 1 {
			t.Fatalf("%s: want exactly one diagnostic, got %+v", name, bag.Items)
		}
		d := bag.Items[0]
		if d.Code != diag.ScratchInApp || d.Severity != "error" || d.Span.File != rel || d.Span.Start.Line != 1 || d.Span.Start.Col != 1 ||
			d.Message != "a scratch file does not belong under app/; move it to the project root" {
			t.Fatalf("%s: %+v", name, d)
		}
		if len(prog.Modules) != 0 {
			t.Fatalf("%s: scratch file became a module: %v", name, prog.Modules)
		}
	}
	check("disk", writeApp(t, map[string]string{rel: "> 1 +\n"}), nil)
	check("overlay", writeApp(t, nil), map[string][]byte{rel: []byte("> 1 +\n")})
}
