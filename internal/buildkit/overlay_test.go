// internal/buildkit/overlay_test.go
package buildkit

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadCheckedOverlay_TypeErrorFromOverlay(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "app"), 0o755)
	os.WriteFile(filepath.Join(dir, "app", "A.alder"), []byte("module A\n\nx : Int\nx = 1\n"), 0o644)
	_, res, bag, ioErrs := LoadCheckedOverlay(dir, map[string][]byte{"app/A.alder": []byte("module A\n\nx : Int\nx = \"no\"\n")})
	if len(ioErrs) > 0 || res == nil {
		t.Fatalf("not checked: %v", ioErrs)
	}
	var found bool
	for _, d := range bag.Items {
		if d.Code == "E0003" && strings.HasPrefix(d.Span.File, "app/A.alder") {
			found = true
		}
	}
	if !found {
		t.Fatalf("want E0003 from the overlay text, got %+v", bag.Items)
	}
}
