package lsp

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// helpersCopy copies the gen_helpers golden's app (test helpers in Helpers) to a temporary root.
func helpersCopy(t *testing.T) string {
	t.Helper()
	src, root := "../../testdata/golden/gen_helpers/app", t.TempDir()
	err := filepath.Walk(src, func(p string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return err
		}
		rel, _ := filepath.Rel(src, p)
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		dst := filepath.Join(root, "app", rel)
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return err
		}
		return os.WriteFile(dst, b, 0o644)
	})
	if err != nil {
		t.Fatal(err)
	}
	return root
}

// Hover on a test helper (#4), at its declaration or a reference, says it is one and why; an
// ordinary value's hover does not.
func TestHover_TestHelper(t *testing.T) {
	c := start(t, helpersCopy(t))
	openClean(t, c, "app/Helpers.alder")
	openClean(t, c, "app/Checks.alder")
	cases := []struct {
		file, at string
		off      int
		want     string
	}{
		{"app/Helpers.alder", "fetchText cfg = {", 0, "test helper (uses `Test.fake`)"},
		{"app/Helpers.alder", "fetchText (if", 0, "test helper (uses `Test.fake`)"},
		{"app/Checks.alder", "Helpers.at 400", len("Helpers."), "test helper (uses `fetchText`, a test helper)"},
		{"app/Checks.alder", "Helpers.final\n", len("Helpers."), ""},
	}
	for _, k := range cases {
		got := hover(t, c, k.file, c.at(k.file, k.at, k.off))
		if k.want == "" && strings.Contains(got, "test helper") || k.want != "" && !strings.Contains(got, k.want) {
			t.Errorf("hover at %q: %q, want it to contain %q", k.at, got, k.want)
		}
		if got == "" {
			t.Errorf("hover at %q: empty", k.at)
		}
	}
}
