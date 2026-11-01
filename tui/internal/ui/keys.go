package ui

import (
	"strings"

	"alder/tui/internal/look"
)

// Key is one key a screen answers, with its help, as the footer and the keys overlay show it.
type Key struct {
	Key, Help string
}

// globalKeys are the keys the frame answers on every screen.
var globalKeys = []Key{
	{"1–5", "switch tabs"},
	{"tab", "next tab"},
	{"shift-tab", "previous tab"},
	{"p", "problems: the last build's diagnostics, or the app's exit"},
	{"R", "restart the app (POST /_dev/restart)"},
	{"o", "open the app in the browser"},
	{"F", "focus the footer; then y copies its command"},
	{"?", "these keys"},
	{"q", "quit"},
}

// keysView is the keys overlay: the current screen's keys under its title, if it has any, then
// the global ones.
func keysView(s Screen) string {
	var b strings.Builder
	b.WriteString(look.Palette.Bold("Keys"))
	b.WriteString("\n")
	section := func(title string, keys []Key) {
		b.WriteString("\n")
		b.WriteString(look.Palette.Bold(title))
		b.WriteString("\n")
		rows := make([][]string, len(keys))
		for i, k := range keys {
			rows[i] = []string{k.Key, k.Help}
		}
		for _, l := range look.Columns(rows, 2) {
			b.WriteString("  " + l + "\n")
		}
	}
	if keys := s.Keys(); len(keys) > 0 {
		section(s.Title(), keys)
	}
	section("Everywhere", globalKeys)
	return b.String()
}
