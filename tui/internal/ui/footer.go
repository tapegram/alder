package ui

import (
	"strings"

	"github.com/charmbracelet/x/ansi"

	"alder/tui/internal/look"
)

// footerKeys are the global keys every screen's footer lists after its own.
var footerKeys = []Key{{"o", "browser"}, {"R", "restart"}, {"?", "keys"}}

// footer is the frame's last line: the keys as `key help` pairs, and right-aligned and dim the
// real command that prints the same data. While the footer is focused it offers `y copy` and the
// command is bold; a flash (the outcome of a copy, an open or a restart) replaces the keys until
// the next key. Keys that do not fit are left off from the end; a command that does not fit is
// cut.
func (m *root) footer() string {
	var keys []Key
	cmd := m.screens[m.tab].Command(m.ctx)
	style := look.Palette.Dim
	switch {
	case m.overlay == overlayProblems:
		keys, cmd = []Key{{"esc", "close"}}, "alder check"
		if len(diags(m.ctx)) > 0 {
			keys = append([]Key{{"j/k", "move"}, {"enter", "copy file:line:col"}}, keys...)
		}
	case m.overlay == overlayKeys:
		keys = []Key{{"esc", "close"}}
	case m.focus:
		keys, style = []Key{{"y", "copy"}, {"esc", "back"}}, look.Palette.Bold
	default:
		keys = append(append(keys, m.screens[m.tab].Keys()...), footerKeys...)
	}
	cmd = ansi.Truncate(cmd, max(m.width-2, 0), "…")
	right := style(cmd)
	room := m.width - 1 - ansi.StringWidth(cmd) - 2
	left := " "
	if m.flash != "" {
		left += ansi.Truncate(m.flash, max(room-1, 0), "…")
	} else {
		pairs := make([]string, 0, len(keys))
		for _, k := range keys {
			pairs = append(pairs, k.Key+" "+k.Help)
		}
		for len(pairs) > 0 && 1+ansi.StringWidth(strings.Join(pairs, "  ")) > room {
			pairs = pairs[:len(pairs)-1]
		}
		left += strings.Join(pairs, "  ")
	}
	gap := max(m.width-ansi.StringWidth(left)-ansi.StringWidth(cmd)-1, 2)
	return left + strings.Repeat(" ", gap) + right
}
