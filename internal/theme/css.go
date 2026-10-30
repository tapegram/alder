package theme

import (
	"slices"
	"strconv"
	"strings"
)

// Setting is one token a theme sets: its CSS name (with `--`) and its value as the CSS gets it.
type Setting struct {
	Name, Value string
}

// generic are the CSS generic font families, written unquoted.
var generic = map[string]bool{"serif": true, "sans-serif": true, "monospace": true, "system-ui": true,
	"ui-sans-serif": true, "ui-serif": true, "ui-monospace": true, "ui-rounded": true, "cursive": true,
	"fantasy": true, "math": true, "emoji": true, "fangsong": true}

// FontStack is the value of a font token: the families trimmed and joined by ", ", each quoted
// unless it is a generic family.
func FontStack(families []string) string {
	out := make([]string, len(families))
	for i, f := range families {
		f = strings.TrimSpace(f)
		if generic[f] {
			out[i] = f
		} else {
			out[i] = strconv.Quote(f)
		}
	}
	return strings.Join(out, ", ")
}

// CSS is the stylesheet a checked theme becomes, appended to the served al.css: the theme-wide
// settings on `:root`, the light colours on `:root, .light`, the dark on `.dark`, each block on
// one line with its settings in Tokens order; an empty block is left out, and a theme that sets
// nothing is "". The blocks are unlayered, so they win over alder-ui's `@layer al` values.
func CSS(root, light, dark []Setting) string {
	if len(root)+len(light)+len(dark) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("/* theme: App */\n")
	block := func(sel string, ss []Setting) {
		if len(ss) == 0 {
			return
		}
		ss = slices.Clone(ss)
		slices.SortStableFunc(ss, func(x, y Setting) int { return order(x.Name) - order(y.Name) })
		b.WriteString(sel + " {")
		for _, s := range ss {
			b.WriteString(" " + s.Name + ": " + s.Value + ";")
		}
		b.WriteString(" }\n")
	}
	block(":root", root)
	block(":root, .light", light)
	block(".dark", dark)
	return b.String()
}

func order(css string) int {
	for i, t := range Tokens {
		if t.CSS == css {
			return i
		}
	}
	return len(Tokens)
}
