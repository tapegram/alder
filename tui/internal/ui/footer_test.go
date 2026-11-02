package ui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
)

// With a long command the footer cuts the command first and drops keys from the end; `? keys`
// and `esc back` stay, and no line of the frame is wider than the terminal.
func TestFooter_Priority(t *testing.T) {
	for _, w := range []int{79, 80, 100} {
		for _, name := range []string{"logs", "detail"} {
			m := newRoot(options(nil), fixture(t, "basic")...)
			run(m, tea.WindowSizeMsg{Width: w, Height: 24})
			press(m, "2")
			if name == "detail" {
				press(m, "1", "k", "k", "k", "enter")
			}
			frame := m.View()
			lines := strings.Split(frame, "\n")
			foot := ansi.Strip(lines[len(lines)-1])
			if !strings.Contains(foot, "? keys") {
				t.Errorf("%s at %d: no `? keys` in %q", name, w, foot)
			}
			if name == "detail" && !strings.Contains(foot, "esc back") {
				t.Errorf("%s at %d: no `esc back` in %q", name, w, foot)
			}
			for i, l := range lines {
				if cw := ansi.StringWidth(l); cw > w {
					t.Errorf("%s at %d: line %d is %d wide: %q", name, w, i, cw, l)
				}
			}
		}
	}
}

// fitFooter on its own: a long command is cut before any key is dropped, and keys drop from the
// end but never `?` or `esc`.
func TestFooter_Fit(t *testing.T) {
	keys := []Key{{"enter", "open"}, {"esc", "back"}, {"o", "browser"}, {"R", "restart"}, {"?", "keys"}}
	long := strings.Repeat("c", 80)
	left, cmd := fitFooter(keys, long, 100)
	if !strings.Contains(left, "R restart") || !strings.HasSuffix(cmd, "…") || len(cmd) < footerCmdFloor {
		t.Errorf("wide enough for the keys: left %q, cmd %q", left, cmd)
	}
	left, cmd = fitFooter(keys, long, 50)
	if strings.Contains(left, "restart") || !strings.Contains(left, "? keys") || !strings.Contains(left, "esc back") {
		t.Errorf("narrow: left %q", left)
	}
	if ansi.StringWidth(left)+2+ansi.StringWidth(cmd) > 50 {
		t.Errorf("narrow: %q + %q is wider than 50", left, cmd)
	}
	left, cmd = fitFooter(keys, long, 30)
	if !strings.Contains(left, "? keys") || !strings.Contains(left, "esc back") || ansi.StringWidth(left)+2+ansi.StringWidth(cmd) > 30 {
		t.Errorf("tiny: left %q, cmd %q", left, cmd)
	}
}

// joinFooter never makes a line wider than the terminal, however narrow it is.
func TestFooter_VeryNarrow(t *testing.T) {
	keys := []Key{{"enter", "open"}, {"esc", "back"}, {"o", "browser"}, {"R", "restart"}, {"?", "keys"}}
	left, cmd := fitFooter(keys, strings.Repeat("c", 80), 20)
	line := joinFooter(left, cmd, func(s string) string { return "\x1b[2m" + s + "\x1b[0m" }, 20)
	if w := ansi.StringWidth(line); w > 20 {
		t.Errorf("footer is %d cells at width 20: %q", w, ansi.Strip(line))
	}
}
