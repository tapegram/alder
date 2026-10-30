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
		if generic[strings.ToLower(f)] { // CSS keywords are case-insensitive: `Serif` is generic
			out[i] = f
		} else {
			out[i] = strconv.Quote(f)
		}
	}
	return strings.Join(out, ", ")
}

// CSS is the stylesheet a checked theme becomes, appended to the served al.css: the theme-wide
// settings on `:root`, the light colours on LightSelector, the dark on `.dark`, each block on
// one line with its settings in Tokens order; an empty block is left out, and a theme that sets
// nothing is "". The blocks are unlayered, so they win over alder-ui's `@layer al` values.
//
// The light block must not match a dark root: an unlayered `:root` would beat alder-ui's layered
// `.dark` there, and a colour set only under `light` would show in dark mode. `:root:not(.dark)`
// excludes it, wrapped in `:where()` so its specificity stays that of the `:root` an app.css
// writes, which (linked later) keeps winning over the theme.
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
		slices.SortStableFunc(ss, func(x, y Setting) int { return Order(x.Name) - Order(y.Name) })
		b.WriteString(sel + " {")
		for _, s := range ss {
			b.WriteString(" " + s.Name + ": " + s.Value + ";")
		}
		b.WriteString(" }\n")
	}
	block(":root", root)
	block(LightSelector, light)
	block(".dark", dark)
	return b.String()
}

// LightSelector is the light block's selector list (see CSS).
const LightSelector = ":where(:root:not(.dark)), .light"

// Order is the CSS name's index in Tokens (len(Tokens) for a name not in it).
func Order(css string) int {
	for i, t := range Tokens {
		if t.CSS == css {
			return i
		}
	}
	return len(Tokens)
}
