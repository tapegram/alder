package generate

import (
	"os"
	"path/filepath"
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

func TestDetectFormat(t *testing.T) {
	for src, want := range map[string]string{
		"\xef\xbb\xbf\n\n{ \"a\": 1 }": "dtcg",
		"{}":                           "dtcg",
		":root {":                      "css",
		"  /* c */ :root{":             "css",
		"":                             "css",
	} {
		if got := detectFormat([]byte(src)); got != want {
			t.Errorf("%q: %s, want %s", src, got, want)
		}
	}
}

// A DTCG file may be up to the reader's 1 MiB; the CSS cap does not apply to it.
func TestThemeImportSpec_SizeLimitsPerFormat(t *testing.T) {
	run := SpecByID("theme import").Run
	pad := func(n int) string { return strings.Repeat(" ", n) }
	dtcg := func(n int) string {
		return `{"color":{"$type":"color","primary":{"$value":{"colorSpace":"srgb","components":[0.2,0.4,0.8]}}}}` + pad(n)
	}
	app := func(s string) App { return App{Dir: t.TempDir(), Stdin: strings.NewReader(s)} }
	sets := Sets{"file": {"-"}, "print": {""}}

	if res, err := run(sets, app(dtcg(300<<10)), true); err != nil || res.Import == nil {
		t.Fatalf("300 KiB DTCG should import: %v", err)
	}
	_, err := run(sets, app(dtcg(2<<20)), true)
	if err == nil || !strings.Contains(err.Error(), "1 MiB") || !strings.Contains(err.Error(), "DTCG") {
		t.Fatalf("2 MiB DTCG: want a DTCG refusal naming 1 MiB, got %v", err)
	}
	_, err = run(sets, app(":root { --primary: red; }"+pad(300<<10)), true)
	if err == nil || !strings.Contains(err.Error(), "256 KiB") || !strings.Contains(err.Error(), "CSS") {
		t.Fatalf("300 KiB CSS: want the CSS refusal, got %v", err)
	}
	_, err = run(Sets{"file": {"-"}, "print": {""}, "format": {"css"}}, app(dtcg(300<<10)), true)
	if err == nil || !strings.Contains(err.Error(), "256 KiB") {
		t.Fatalf("--format css on 300 KiB: %v", err)
	}
}

// A DTCG file named on the command line is read once; a resolver's other files come from disk.
func TestThemeImportSpec_FileOverlay(t *testing.T) {
	dir := t.TempDir()
	doc := `{"color":{"$type":"color","primary":{"$value":{"colorSpace":"srgb","components":[0.2,0.4,0.8]}}}}` + strings.Repeat(" ", 300<<10)
	if err := os.WriteFile(filepath.Join(dir, "tokens.json"), []byte(doc), 0o644); err != nil {
		t.Fatal(err)
	}
	res, err := SpecByID("theme import").Run(Sets{"file": {filepath.Join(dir, "tokens.json")}, "print": {""}}, App{Dir: t.TempDir()}, true)
	if err != nil || res.Import == nil || !strings.Contains(res.Import.Print, "primary") {
		t.Fatalf("file: %v", err)
	}
}
