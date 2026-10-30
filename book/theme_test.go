package book

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"alder/internal/theme"
)

// TestAppendixC_ThemeFragments: every `theme { … }` fragment of Appendix C is a declaration
// `alder check` accepts in App (appended to a copy of the scaffold, which has none).
func TestAppendixC_ThemeFragments(t *testing.T) {
	bin := alder(t)
	infos, sources := fences(t, "C-design-system.md", "alder")
	n := 0
	for i, src := range sources {
		if infos[i] != "alder fragment" || !strings.HasPrefix(src, "theme {") {
			continue
		}
		n++
		dir := filepath.Join(t.TempDir(), "app")
		if err := copyApp(scaffoldApp, dir); err != nil {
			t.Fatal(err)
		}
		app := filepath.Join(dir, "app", "App.alder")
		b, err := os.ReadFile(app)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(app, append(b, []byte("\n"+src)...), 0o644); err != nil {
			t.Fatal(err)
		}
		if out, err := exec.Command(bin, "check", dir).CombinedOutput(); err != nil {
			t.Errorf("book/C-design-system.md: theme fragment %d does not check in App: %v\n%s\n%s", n, err, src, out)
		}
	}
	if n < 2 {
		t.Errorf("Appendix C has %d theme fragments, want the example and the AA-strict borders", n)
	}
}

// TestAppendixC_ThemeTable: "Changing a token" names every field of theme.Tokens, and its CSS
// block is the CSS the generator emits for the same theme (testdata/golden/gen_theme).
func TestAppendixC_ThemeTable(t *testing.T) {
	b, err := os.ReadFile("C-design-system.md")
	if err != nil {
		t.Fatal(err)
	}
	_, sec, _ := strings.Cut(string(b), "### Changing a token\n")
	sec, _, _ = strings.Cut(sec, "\n## ")
	for _, tok := range theme.Tokens {
		if !strings.Contains(sec, "`"+tok.Field+"`") {
			t.Errorf("Changing a token does not name `%s` (%s)", tok.Field, tok.CSS)
		}
	}
	_, css, ok := strings.Cut(sec, "```css\n/* theme: App */\n")
	if !ok {
		t.Fatal("Changing a token has no ```css block of the emitted theme")
	}
	css, _, _ = strings.Cut(css, "```")
	gen, err := os.ReadFile("../testdata/golden/gen_theme/want/app__alder.go")
	if err != nil {
		t.Fatal(err)
	}
	if want := strconv.Quote("/* theme: App */\n" + css); !strings.Contains(string(gen), want) {
		t.Errorf("the chapter's CSS block is not what gen_theme emits:\n%s", css)
	}
}
