package lsp

import (
	"strings"
	"testing"
)

// openThemeApp opens the blog's App with its theme's `light` written one field per line.
func openThemeApp(t *testing.T) *client {
	t.Helper()
	c := start(t, blogCopy(t))
	const one = `  light: { primary: "oklch(0.51 0.23 277)", primaryForeground: "oklch(0.98 0 0)" },`
	text := c.disk("app/App.alder")
	if !strings.Contains(text, one) {
		t.Fatalf("the blog's theme has no %q", one)
	}
	c.open("app/App.alder", strings.Replace(text, one, "  light: {\n    primary: \"oklch(0.51 0.23 277)\",\n  },", 1))
	c.diags("app/App.alder")
	return c
}

func TestComplete_ThemeLight(t *testing.T) {
	c := openThemeApp(t)
	p := typing(t, c, "app/App.alder", "    primary: \"oklch(0.51 0.23 277)\",\n", "    pri")
	got := labels(complete(t, c, "app/App.alder", p))
	it, ok := got["primaryForeground"]
	if !ok || got["radius"].Label != "" || got["light"].Label != "" {
		t.Fatalf("inside light: %v", got)
	}
	if it.Detail != "--primary-foreground (colour)" || it.TextEdit.NewText != "primaryForeground: " || it.TextEdit.Range.Start.Character != p.Character-3 {
		t.Fatalf("primaryForeground: %+v", it)
	}
}

func TestComplete_ThemeTop(t *testing.T) {
	c := openThemeApp(t)
	p := typing(t, c, "app/App.alder", "  radius: \"0.5rem\",\n", "  r")
	got := labels(complete(t, c, "app/App.alder", p))
	if _, ok := got["radius"]; !ok || got["light"].Label == "" || got["primary"].Label != "" {
		t.Fatalf("at the top of theme: %v", got)
	}
	if got["fontSans"].Detail != "--font-sans (font stack)" {
		t.Fatalf("fontSans: %+v", got["fontSans"])
	}
}

func TestHover_ThemeField(t *testing.T) {
	c := openThemeApp(t)
	got := hover(t, c, "app/App.alder", c.at("app/App.alder", "primary: \"oklch(0.51", 2))
	for _, want := range []string{"`--primary`", "colour", "light `oklch(0.205 0 0)`", "dark `oklch(0.922 0 0)`"} {
		if !strings.Contains(got, want) {
			t.Fatalf("hover %q lacks %q", got, want)
		}
	}
	if got := hover(t, c, "app/App.alder", c.at("app/App.alder", "radius: \"0.5rem\"", 1)); !strings.Contains(got, "`--radius`") || !strings.Contains(got, "light `0.625rem`") {
		t.Fatalf("radius hover %q", got)
	}
}
