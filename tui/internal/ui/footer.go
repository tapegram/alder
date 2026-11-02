package ui

import (
	"strings"

	"github.com/charmbracelet/x/ansi"

	"alder/tui/internal/look"
)

// footerKeys are the global keys every screen's footer lists after its own.
var footerKeys = []Key{{"o", "browser"}, {"R", "restart"}, {"?", "keys"}}

// footerCmdFloor is how many cells of the command the footer keeps before it starts dropping keys.
const footerCmdFloor = 16

// footer is the frame's last line: the keys as `key help` pairs, and right-aligned and dim the
// real command that prints the same data. While the footer is focused it offers `y copy` and the
// command is bold; a flash (the outcome of a copy, an open or a restart) replaces the keys until
// the next key. When the line is too narrow the command is cut with `…` first, down to
// footerCmdFloor cells; then keys are dropped from the end, and `? keys` and any `esc` key never
// are; only then is the command cut further.
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
	if m.flash != "" {
		cmd = ansi.Truncate(cmd, max(m.width-2, 0), "…")
		room := m.width - 1 - ansi.StringWidth(cmd) - 2
		left := " " + ansi.Truncate(m.flash, max(room-1, 0), "…")
		return joinFooter(left, cmd, style, m.width)
	}
	left, cmd := fitFooter(keys, cmd, m.width)
	return joinFooter(left, cmd, style, m.width)
}

// fitFooter is the left side of the footer (a space, then the keys that fit) and the command,
// cut if need be, for a line width cells wide.
func fitFooter(keys []Key, cmd string, width int) (string, string) {
	pairs := func(ks []Key) string {
		s := make([]string, len(ks))
		for i, k := range ks {
			s[i] = k.Key + " " + k.Help
		}
		return strings.Join(s, "  ")
	}
	cmdW := ansi.StringWidth(cmd)
	want := min(cmdW, footerCmdFloor)
	// avail is what the command may take beside the keys: the line less the leading space, the
	// keys and a gap of two.
	avail := func() int {
		w := 0
		if len(keys) > 0 {
			w = ansi.StringWidth(pairs(keys))
		}
		return width - 1 - w - 2
	}
	for avail() < want {
		drop := -1
		for i := len(keys) - 1; i >= 0; i-- {
			if keys[i].Key != "?" && keys[i].Key != "esc" {
				drop = i
				break
			}
		}
		if drop < 0 {
			break
		}
		keys = append(keys[:drop:drop], keys[drop+1:]...)
	}
	cmd = ansi.Truncate(cmd, max(avail(), 0), "…")
	return " " + pairs(keys), cmd
}

// joinFooter puts the command at the right of left, at least two cells away.
func joinFooter(left, cmd string, style func(string) string, width int) string {
	gap := max(width-ansi.StringWidth(left)-ansi.StringWidth(cmd)-1, 2)
	return left + strings.Repeat(" ", gap) + style(cmd)
}
