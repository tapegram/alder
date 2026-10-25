package scaffold_test

import (
	"os"
	"strings"
	"testing"

	"alder/internal/format"
	"alder/internal/scaffold"
)

// TestScaffoldLayoutThemeScript: the scaffold's Layout carries the theme script before the
// stylesheet, and the formatter (a raw <script> body is text, not interpolation) leaves the file,
// the script byte for byte included, as it is.
func TestScaffoldLayoutThemeScript(t *testing.T) {
	src, err := os.ReadFile("files/app/Layout.alder")
	if err != nil {
		t.Fatal(err)
	}
	s := string(src)
	i, j := strings.Index(s, scaffold.ThemeScript), strings.Index(s, "al.css")
	if i < 0 || j < 0 || i > j {
		t.Fatalf("theme script at %d, al.css at %d", i, j)
	}
	out, diags, err := format.Source("Layout.alder", src)
	if err != nil || len(diags) > 0 {
		t.Fatalf("fmt: %v %v", err, diags)
	}
	if string(out) != s {
		t.Errorf("fmt changed the Layout:\n%s", out)
	}
	if !strings.Contains(string(out), scaffold.ThemeScript) {
		t.Errorf("fmt altered the theme script:\n%s", out)
	}
}
