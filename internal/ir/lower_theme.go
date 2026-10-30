package ir

import (
	"slices"
	"strconv"
	"strings"

	"alder/internal/syntax"
	"alder/internal/theme"
)

// Theme is App's `theme { … }` (Appendix C): the alder-ui tokens it sets, by block: the
// theme-wide ones (`:root`), the light colours (`:root, .light`) and the dark (`.dark`), each in
// theme.Tokens order. The generated Web() carries its CSS (web.Options.Theme).
type Theme struct {
	Root, Light, Dark []Setting
}

// Setting is one token a theme sets: its CSS name and its value as the CSS gets it.
type Setting = theme.Setting

// CSS is the stylesheet the theme becomes (theme.CSS); "" for nil or a theme that sets nothing.
func (t *Theme) CSS() string {
	if t == nil {
		return ""
	}
	return theme.CSS(t.Root, t.Light, t.Dark)
}

// lowerTheme turns App's checked theme into the IR. The checker has refused a malformed one
// (E0042); a field that does not fit is left out.
func lowerTheme(d *syntax.ThemeDecl) *Theme {
	t := &Theme{}
	t.Root = themeSettings(d.Fields, false)
	for _, f := range d.Fields {
		if f.Value == nil || f.Value.Kind != "record" {
			continue
		}
		switch f.Name {
		case "light":
			t.Light = themeSettings(f.Value.Rec, true)
		case "dark":
			t.Dark = themeSettings(f.Value.Rec, true)
		}
	}
	return t
}

func themeSettings(fs []*syntax.DeployField, colours bool) []Setting {
	var out []Setting
	for _, f := range fs {
		tok, ok := theme.Lookup(f.Name)
		if !ok || f.Value == nil || (tok.Kind == theme.Colour) != colours {
			continue
		}
		v := f.Value
		switch {
		case tok.Kind == theme.Font && v.Kind == "list":
			fams := make([]string, len(v.List))
			for i, e := range v.List {
				fams[i] = e.Str
			}
			out = append(out, Setting{Name: tok.CSS, Value: theme.FontStack(fams)})
		case v.Kind == "string":
			out = append(out, Setting{Name: tok.CSS, Value: strings.TrimSpace(v.Str)})
		}
	}
	slices.SortStableFunc(out, func(a, b Setting) int { return theme.Order(a.Name) - theme.Order(b.Name) })
	return out
}

// themeForm is (theme (root (--name "value")…)? (light …)? (dark …)?).
func themeForm(t *Theme) *sx {
	x := list(1, atom("theme"))
	for _, b := range []struct {
		head string
		ss   []Setting
	}{{"root", t.Root}, {"light", t.Light}, {"dark", t.Dark}} {
		if len(b.ss) == 0 {
			continue
		}
		l := list(1, atom(b.head))
		for _, s := range b.ss {
			l.items = append(l.items, list(1, atom(s.Name), atom(strconv.Quote(s.Value))))
		}
		x.items = append(x.items, l)
	}
	return x
}
