package ui

import (
	"strconv"
	"strings"

	"github.com/charmbracelet/x/ansi"

	"alder/tui/internal/alderbin"
	"alder/tui/internal/look"
)

// statusBar is the frame's first line: ` <app>  ● <state>  <addr>  alder <version>`, then the
// tab names right-aligned with the current one bold. The dot and its state are yellow while
// building, green when ok and red when failed, with the count of diagnostics. A disconnected
// feed, or else a version mismatch between alder-tui and alder, replaces the version in yellow.
// When the tabs do not fit, only the current one is shown; when even that does not fit, the left
// part is cut.
func (m *root) statusBar() string {
	c := m.ctx
	left := " " + appName(c) + "  " + stateDot(c) + "  " + c.Opts.Addr
	if v := versionPart(c); v != "" {
		left += "  " + v
	}
	tabs := make([]string, len(m.screens))
	for i, s := range m.screens {
		tabs[i] = strconv.Itoa(i+1) + " " + s.Title()
		if i == m.tab {
			tabs[i] = look.Palette.Bold(tabs[i])
		}
	}
	right := strings.Join(tabs, " ")
	if ansi.StringWidth(left)+2+ansi.StringWidth(right)+1 > m.width {
		right = tabs[m.tab]
	}
	if room := m.width - 2 - ansi.StringWidth(right) - 1; ansi.StringWidth(left) > room {
		left = ansi.Truncate(left, max(room, 0), "…")
	}
	gap := max(m.width-ansi.StringWidth(left)-ansi.StringWidth(right)-1, 2)
	return left + strings.Repeat(" ", gap) + right
}

// stateDot is the build state with its coloured dot; while the TUI opens the dev server it is a
// dim `starting`, a red `failed to start` once that has failed, and before the first hello a dim
// `connecting`.
func stateDot(c *Ctx) string {
	switch {
	case c.StartErr != nil:
		return look.Palette.Red("● failed to start")
	case c.Starting:
		return look.Palette.Dim("● starting")
	}
	switch c.State {
	case "ok":
		return look.Palette.Green("● ok")
	case "building":
		return look.Palette.Yellow("● building")
	case "failed":
		if c.Build != nil && len(c.Build.Diagnostics) > 0 {
			return look.Palette.Red("● failed (" + strconv.Itoa(len(c.Build.Diagnostics)) + ")")
		}
		return look.Palette.Red("● failed")
	case "":
		return look.Palette.Dim("● connecting")
	}
	return "● " + c.State
}

// versionPart is `alder <version>`, the version being alder's own (from `alder help --json`),
// else the hello's; or `disconnected`, or the mismatch warning, in yellow.
func versionPart(c *Ctx) string {
	if c.Disconnected {
		return look.Palette.Yellow("disconnected")
	}
	if c.Opts.AlderVersion != "" {
		if w := alderbin.Mismatch(c.Opts.Version, c.Opts.AlderVersion); w != "" {
			return look.Palette.Yellow(w)
		}
	}
	v := c.Opts.AlderVersion
	if v == "" && c.Hello != nil {
		v = c.Hello.Alder
	}
	if v == "" {
		return ""
	}
	return "alder " + v
}
