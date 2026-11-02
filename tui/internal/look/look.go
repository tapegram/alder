// Package look is the TUI's palette. It uses the 16 ANSI colours only, so the terminal's theme
// applies, and means by each style what internal/style means in the alder CLI (that package is
// internal to the root module, so the meanings are copied here, not imported). The sequences are
// written directly rather than through a renderer, so the bytes are the same on a terminal, in
// a pipe and in a test.
package look

import (
	"strings"

	"github.com/charmbracelet/x/ansi"
)

// Style is the palette: each field wraps text in its sequence, and leaves empty text alone.
type Style struct {
	Bold, Dim, Red, Green, Yellow, Cyan, BoldRed, BoldGreen, BoldYellow func(string) string
}

func sgr(code string) func(string) string {
	return func(text string) string {
		if text == "" {
			return text
		}
		return "\x1b[" + code + "m" + text + "\x1b[0m"
	}
}

// Palette is the Style the screens use.
var Palette = Style{
	Bold:       sgr("1"),
	Dim:        sgr("2"),
	Red:        sgr("31"),
	Green:      sgr("32"),
	Yellow:     sgr("33"),
	Cyan:       sgr("36"),
	BoldRed:    sgr("1;31"),
	BoldGreen:  sgr("1;32"),
	BoldYellow: sgr("1;33"),
}

// Mark styles a status word: ok green; FAIL and error bold red; warning and SKIPPED yellow;
// anything else as it is.
func Mark(status string) string {
	switch status {
	case "ok":
		return Palette.Green(status)
	case "FAIL", "error":
		return Palette.BoldRed(status)
	case "warning", "SKIPPED":
		return Palette.Yellow(status)
	}
	return status
}

// StatusClass is the style of an HTTP status code: below 300 plain, redirects dim, client
// errors yellow, server errors bold red.
func StatusClass(code int) func(string) string {
	switch {
	case code >= 500:
		return Palette.BoldRed
	case code >= 400:
		return Palette.Yellow
	case code >= 300:
		return Palette.Dim
	}
	return func(s string) string { return s }
}

// Columns lays rows out left-aligned, each column as wide as its widest cell in terminal cells
// (escape sequences take none, wide runes take two),
// gutter spaces between, one string per row with no trailing spaces.
func Columns(rows [][]string, gutter int) []string {
	var widths []int
	for _, r := range rows {
		for i, c := range r {
			if i >= len(widths) {
				widths = append(widths, 0)
			}
			widths[i] = max(widths[i], ansi.StringWidth(c))
		}
	}
	lines := make([]string, 0, len(rows))
	for _, r := range rows {
		var sb strings.Builder
		for i, c := range r {
			sb.WriteString(c)
			if i < len(r)-1 {
				sb.WriteString(strings.Repeat(" ", widths[i]-ansi.StringWidth(c)+gutter))
			}
		}
		lines = append(lines, strings.TrimRight(sb.String(), " "))
	}
	return lines
}
