package generate

import (
	"strings"
	"testing"
)

// The language server runs Specs for the form preview with no stdin: `-` must refuse, not read.
func TestThemeImportSpec_Stdin(t *testing.T) {
	run := SpecByID("theme import").Run
	_, err := run(Sets{"file": {"-"}}, App{Dir: t.TempDir()}, true)
	if _, ok := err.(*Refusal); !ok || !strings.Contains(err.Error(), "only the command line has") {
		t.Fatalf("nil Stdin: %T %v", err, err)
	}
	res, err := run(Sets{"file": {"-"}, "print": {""}}, App{Dir: t.TempDir(), Stdin: strings.NewReader(":root { --primary: oklch(0.5 0.1 200); }")}, true)
	if err != nil || res.Import == nil || !strings.Contains(res.Import.Print, "primary") {
		t.Fatalf("reader: %v %+v", err, res)
	}
}
