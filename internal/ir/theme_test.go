package ir

import (
	"strings"
	"testing"
)

// App's theme lowers to its settings by CSS name, in alder-ui's token order whatever the source
// order, with values trimmed and a font list as its CSS stack.
func TestLower_Theme(t *testing.T) {
	p := lowerDir(t, "../../testdata/corpus/theme-ok")
	if p.Theme == nil {
		t.Fatal("no theme")
	}
	want := `(theme (root (--radius "0.5rem") (--radius-xl "0") (--al-width-wide "90%") (--font-sans "\"Inter\", system-ui, sans-serif") (--font-mono "\"Geist Mono\", monospace") (--shadow-sm "0 1px 3px 0 rgb(0 0 0 / 0.1), 0 1px 2px -1px rgb(0 0 0 / 0.1)") (--shadow-lg "none")) (light (--primary "oklch(0.55 0.22 285)") (--primary-foreground "#ffffff") (--border "color-mix(in oklab, var(--foreground) 20%, transparent)") (--input "oklch(0.65 0 0)") (--ring "var(--primary)") (--chart-1 "hsl(12 76% 61%)")) (dark (--primary "oklch(0.72 0.16 285)") (--primary-foreground "oklch(0.145 0 0)") (--sidebar-ring "currentColor")))`
	if got := themeForm(p.Theme).flat(); got != want {
		t.Errorf("theme form:\n%s\nwant:\n%s", got, want)
	}
	if !strings.Contains(Dump(p), "--primary-foreground \"#ffffff\"") {
		t.Error("Dump lacks the theme")
	}
	if css := p.Theme.CSS(); !strings.HasPrefix(css, "/* theme: App */\n:root { --radius: 0.5rem;") {
		t.Errorf("CSS = %q", css)
	}
}

func TestLower_NoTheme(t *testing.T) {
	if p := lowerDir(t, "../../testdata/corpus/deploy-ok"); p.Theme != nil || p.Theme.CSS() != "" {
		t.Errorf("theme without a declaration: %+v", p.Theme)
	}
	if p := lowerDir(t, "../../testdata/corpus/theme-empty-ok"); p.Theme == nil || p.Theme.CSS() != "" {
		t.Errorf("empty theme: %+v", p.Theme)
	}
}
