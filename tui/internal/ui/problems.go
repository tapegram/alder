package ui

import (
	"strconv"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"alder/tui/internal/feed"
	"alder/tui/internal/look"
	"alder/tui/internal/store"
)

// problems is the Problems overlay's state: the diagnostic under the cursor.
type problems struct {
	cursor int
}

// diags are the last build event's diagnostics (a failure's errors, or an ok build's warnings).
func diags(c *Ctx) []feed.Diag {
	if c.Build == nil {
		return nil
	}
	return append(append([]feed.Diag{}, c.Build.Diagnostics...), c.Build.Warnings...)
}

// location is a diagnostic's `file:line:col`.
func location(d feed.Diag) string {
	return d.File + ":" + strconv.Itoa(d.Line) + ":" + strconv.Itoa(d.Col)
}

// diagLine is a diagnostic in `alder check`'s layout, `file:line:col: error[CODE]: message`:
// the location bold, the severity and code bold red for an error and yellow for a warning. A
// diagnostic without a code drops the brackets.
func diagLine(d feed.Diag) string {
	sev := d.Severity
	if d.Code != "" {
		sev += "[" + d.Code + "]"
	}
	switch d.Severity {
	case "error":
		sev = look.Palette.BoldRed(sev)
	case "warning":
		sev = look.Palette.Yellow(sev)
	}
	return look.Palette.Bold(location(d)+":") + " " + sev + ": " + d.Message
}

// problemsKey answers a key while the overlay is open: j/k and the arrows move the cursor, enter
// copies the diagnostic's `file:line:col`, esc or p closes it, ? switches to the keys overlay
// and q quits.
func (m *root) problemsKey(k string) tea.Cmd {
	ds := diags(m.ctx)
	switch k {
	case "esc", "p":
		m.overlay = overlayNone
	case "?":
		m.overlay = overlayKeys
	case "q":
		return tea.Quit
	case "j", "down":
		m.probs.cursor = min(m.probs.cursor+1, max(len(ds)-1, 0))
	case "k", "up":
		m.probs.cursor = max(m.probs.cursor-1, 0)
	case "enter":
		if m.probs.cursor < len(ds) {
			return m.copy(location(ds[m.probs.cursor]))
		}
	}
	return nil
}

// view is the overlay in height lines: the heading, each diagnostic (the one under the cursor
// marked), a failure's text when the build failed without diagnostics, and after the app exited
// its exit and the last lines of its stderr. The lines scroll to keep the cursor in view.
func (p *problems) view(c *Ctx, height int) string {
	var lines []string
	at := 0 // the line of the cursor
	ds := diags(c)
	p.cursor = min(p.cursor, max(len(ds)-1, 0))
	for i, d := range ds {
		mark := "  "
		if i == p.cursor {
			mark, at = "> ", len(lines)
		}
		lines = append(lines, mark+diagLine(d))
	}
	if c.Build != nil && c.Build.Text != "" && len(ds) == 0 {
		for _, l := range strings.Split(strings.TrimRight(c.Build.Text, "\n"), "\n") {
			lines = append(lines, "  "+l)
		}
	}
	if e := c.Exited; e != nil {
		if len(lines) > 0 {
			lines = append(lines, "")
		}
		lines = append(lines, "  "+exitLine(e))
		for _, l := range e.Tail {
			lines = append(lines, "    "+tailLine(l))
		}
	}
	if len(lines) == 0 {
		lines = append(lines, "  no problems")
	}
	head := []string{look.Palette.Bold("Problems"), ""}
	room := max(height-len(head), 1)
	off := 0
	if at >= room {
		off = at - room + 1
	}
	lines = lines[off:min(len(lines), off+room)]
	return strings.Join(append(head, lines...), "\n")
}

// exitLine is `the app exited (code N)`, or `(signal NAME)` when a signal ended it.
func exitLine(a *feed.App) string {
	switch {
	case a.Exit == nil:
		return "the app exited"
	case a.Exit.Signal != "":
		return "the app exited (signal " + a.Exit.Signal + ")"
	}
	return "the app exited (code " + strconv.Itoa(a.Exit.Code) + ")"
}

// tailLine is a line of an exited app's stderr as `alder run` shows it: a JSON log record (the
// child logs JSON under the dev server) as its text line, anything else as it is.
func tailLine(raw string) string {
	ev, err := feed.Decode([]byte(`{"type":"log","record":` + raw + `}`))
	if err != nil || ev.Log == nil || ev.Log.Record == nil || ev.Log.Record.Level == "" {
		return raw
	}
	l := store.NewLogs(1)
	l.Add(ev)
	return l.View(store.Filter{})[0].Text
}
