package theme

import (
	"regexp"
	"slices"
	"strings"
	"testing"
)

// A model of the CSS cascade for the custom properties of the token blocks: enough to show
// which value an element computes when alder-ui's layered blocks, the theme's unlayered ones
// and an app.css meet. A rule is one selector list; an element is the root or a child of the
// one before it, with its classes.

type rule struct {
	sels    []string
	decls   map[string]string
	layered bool
}

type elem struct {
	root    bool
	classes []string
}

// matches reports whether the simple selector s (`:root`, `.x`, `:root:not(.x)`, `:where(s)`)
// matches e, and its specificity in class units.
func matches(s string, e elem) (bool, int) {
	if inner, ok := strings.CutPrefix(s, ":where("); ok {
		m, _ := matches(strings.TrimSuffix(inner, ")"), e)
		return m, 0
	}
	if rest, ok := strings.CutPrefix(s, ":root"); ok {
		if !e.root {
			return false, 0
		}
		if rest == "" {
			return true, 1
		}
		not := strings.TrimSuffix(strings.TrimPrefix(rest, ":not(."), ")")
		return !slices.Contains(e.classes, not), 2
	}
	if c, ok := strings.CutPrefix(s, "."); ok {
		return slices.Contains(e.classes, c), 1
	}
	panic("cascade model: selector " + s)
}

// computed is the value of prop on the last element of path: the winning declaration on it
// (unlayered over layered, then specificity, then order), else its parent's value.
func computed(rules []rule, path []elem, prop string) string {
	e := path[len(path)-1]
	best, bestKey := "", [3]int{-1}
	for i, r := range rules {
		v, ok := r.decls[prop]
		if !ok {
			continue
		}
		spec := -1
		for _, s := range r.sels {
			if m, sp := matches(s, e); m && sp > spec {
				spec = sp
			}
		}
		if spec < 0 {
			continue
		}
		layer := 1
		if r.layered {
			layer = 0
		}
		if key := [3]int{layer, spec, i}; key[0] > bestKey[0] || key[0] == bestKey[0] && (key[1] > bestKey[1] || key[1] == bestKey[1] && key[2] > bestKey[2]) {
			best, bestKey = v, key
		}
	}
	if best == "" && len(path) > 1 {
		return computed(rules, path[:len(path)-1], prop)
	}
	return best
}

var reRule = regexp.MustCompile(`(?m)^([^{/]+)\{([^}]*)\}`)

// parseRules reads one-line rules as CSS writes them.
func parseRules(css string, layered bool) []rule {
	var out []rule
	for _, m := range reRule.FindAllStringSubmatch(css, -1) {
		r := rule{decls: map[string]string{}, layered: layered}
		for _, s := range strings.Split(m[1], ",") {
			r.sels = append(r.sels, strings.TrimSpace(s))
		}
		for _, d := range strings.Split(m[2], ";") {
			if k, v, ok := strings.Cut(d, ":"); ok {
				r.decls[strings.TrimSpace(k)] = strings.TrimSpace(v)
			}
		}
		out = append(out, r)
	}
	return out
}

// sheet is alder-ui's token blocks (layered), then the theme's CSS, then app.css.
func sheet(themeCSS, appCSS string) []rule {
	light, dark, root := Blocks()
	rules := []rule{{sels: []string{":root", ".light"}, decls: light, layered: true}, {sels: []string{".dark"}, decls: dark, layered: true}, {sels: []string{":root"}, decls: root, layered: true}}
	return append(append(rules, parseRules(themeCSS, false)...), parseRules(appCSS, false)...)
}

var (
	lightPage = elem{root: true}
	darkPage  = elem{root: true, classes: []string{"dark"}}
	darkDiv   = elem{classes: []string{"dark"}}
	lightDiv  = elem{classes: []string{"light"}}
	plainDiv  = elem{}
)

func TestCSS_Cascade(t *testing.T) {
	d := Defaults()
	lightOnly := CSS(nil, []Setting{{"--background", "#ffffff"}}, nil)
	darkOnly := CSS(nil, nil, []Setting{{"--background", "#000000"}})
	both := CSS([]Setting{{"--radius", "0.5rem"}}, []Setting{{"--background", "#ffffff"}}, []Setting{{"--background", "#000000"}})
	for _, c := range []struct {
		name, theme, app string
		path             []elem
		prop, want       string
	}{
		{"a light-only theme under html.dark keeps alder-ui's dark value", lightOnly, "", []elem{darkPage, plainDiv}, "--background", d["--background"].Dark},
		{"a light-only theme on a light page", lightOnly, "", []elem{lightPage, plainDiv}, "--background", "#ffffff"},
		{"a light-only theme in a .light subtree of a dark page", lightOnly, "", []elem{darkPage, lightDiv}, "--background", "#ffffff"},
		{"a light-only theme in a .dark subtree of a light page", lightOnly, "", []elem{lightPage, darkDiv}, "--background", d["--background"].Dark},
		{"a dark-only theme under light keeps alder-ui's light value", darkOnly, "", []elem{lightPage, plainDiv}, "--background", d["--background"].Light},
		{"a dark-only theme under html.dark", darkOnly, "", []elem{darkPage}, "--background", "#000000"},
		{"a dark-only theme in a .dark subtree of a light page", darkOnly, "", []elem{lightPage, darkDiv}, "--background", "#000000"},
		{"both, a .dark subtree of a light page", both, "", []elem{lightPage, darkDiv, plainDiv}, "--background", "#000000"},
		{"both, a .light subtree of a dark page", both, "", []elem{darkPage, lightDiv}, "--background", "#ffffff"},
		{"a theme-wide token in dark", both, "", []elem{darkPage}, "--radius", "0.5rem"},
		{"app.css's :root, .light beats the theme's light", both, ":root, .light { --background: red; }", []elem{lightPage}, "--background", "red"},
		{"app.css's .dark beats the theme's dark", both, ".dark { --background: blue; }", []elem{darkPage}, "--background", "blue"},
		{"app.css's :root beats the theme-wide block", both, ":root { --radius: 1rem; }", []elem{lightPage}, "--radius", "1rem"},
	} {
		if got := computed(sheet(c.theme, c.app), c.path, c.prop); got != c.want {
			t.Errorf("%s: %s = %q, want %q\n%s", c.name, c.prop, got, c.want, c.theme)
		}
	}
}
