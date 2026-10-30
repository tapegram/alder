// Package theme is the table of alder-ui's tokens an app may set in App's `theme { … }`
// declaration (book Appendix C, "Changing a token"): each token's field name, CSS name, kind and
// where it may appear, the tokens alder-ui v0.4 removed, alder-ui's own values (Defaults, read
// from the vendored rt/al/al.css), the shapes a value may take, and the CSS a checked theme
// becomes (css.go). The checker, the IR lowering, the LSP and the build all read it.
package theme

import (
	"regexp"
	"strings"
	"sync"

	"alder/rt"
)

// Kind is what a token holds, and so where it may be set and what its value looks like.
type Kind int

const (
	Colour Kind = iota // under `light` or `dark`: text such as "oklch(0.55 0.22 285)"
	Length             // theme-wide: text such as "0.5rem"
	Font               // theme-wide: a list of family names
	Shadow             // theme-wide: text such as "0 1px 2px 0 rgb(0 0 0 / 0.05)" or "none"
)

func (k Kind) String() string {
	switch k {
	case Colour:
		return "colour"
	case Length:
		return "length"
	case Font:
		return "font stack"
	}
	return "shadow"
}

// Token is one settable token: its field name in `theme` (lowerCamel), its CSS name (with the
// leading `--`) and its kind. A colour is set under `light` or `dark`; every other kind at the
// top level of the declaration.
type Token struct {
	Field string
	CSS   string
	Kind  Kind
}

// Tokens are alder-ui's settable tokens, in the order the emitted CSS lists them. A test holds
// them to the custom properties rt/al/al.css declares in its token blocks.
var Tokens = []Token{
	{"radius", "--radius", Length},
	{"radiusSm", "--radius-sm", Length},
	{"radiusMd", "--radius-md", Length},
	{"radiusLg", "--radius-lg", Length},
	{"radiusXl", "--radius-xl", Length},
	{"widthNarrow", "--al-width-narrow", Length},
	{"widthNormal", "--al-width-normal", Length},
	{"widthWide", "--al-width-wide", Length},
	{"fontSans", "--font-sans", Font},
	{"fontMono", "--font-mono", Font},
	{"shadowXs", "--shadow-xs", Shadow},
	{"shadowSm", "--shadow-sm", Shadow},
	{"shadowMd", "--shadow-md", Shadow},
	{"shadowLg", "--shadow-lg", Shadow},
	{"background", "--background", Colour},
	{"foreground", "--foreground", Colour},
	{"card", "--card", Colour},
	{"cardForeground", "--card-foreground", Colour},
	{"popover", "--popover", Colour},
	{"popoverForeground", "--popover-foreground", Colour},
	{"primary", "--primary", Colour},
	{"primaryForeground", "--primary-foreground", Colour},
	{"secondary", "--secondary", Colour},
	{"secondaryForeground", "--secondary-foreground", Colour},
	{"muted", "--muted", Colour},
	{"mutedForeground", "--muted-foreground", Colour},
	{"accent", "--accent", Colour},
	{"accentForeground", "--accent-foreground", Colour},
	{"destructive", "--destructive", Colour},
	{"destructiveForeground", "--destructive-foreground", Colour},
	{"border", "--border", Colour},
	{"input", "--input", Colour},
	{"ring", "--ring", Colour},
	{"chart1", "--chart-1", Colour},
	{"chart2", "--chart-2", Colour},
	{"chart3", "--chart-3", Colour},
	{"chart4", "--chart-4", Colour},
	{"chart5", "--chart-5", Colour},
	{"sidebar", "--sidebar", Colour},
	{"sidebarForeground", "--sidebar-foreground", Colour},
	{"sidebarPrimary", "--sidebar-primary", Colour},
	{"sidebarPrimaryForeground", "--sidebar-primary-foreground", Colour},
	{"sidebarAccent", "--sidebar-accent", Colour},
	{"sidebarAccentForeground", "--sidebar-accent-foreground", Colour},
	{"sidebarBorder", "--sidebar-border", Colour},
	{"sidebarRing", "--sidebar-ring", Colour},
}

// Lookup is the token whose field name is field.
func Lookup(field string) (Token, bool) {
	for _, t := range Tokens {
		if t.Field == field {
			return t, true
		}
	}
	return Token{}, false
}

// ByCSS is the token whose CSS name (with `--`) is name.
func ByCSS(name string) (Token, bool) {
	for _, t := range Tokens {
		if t.CSS == name {
			return t, true
		}
	}
	return Token{}, false
}

// FieldName is a CSS custom property's field name: without `--` (and alder-ui's own `al-`
// prefix), in lowerCamel, a digit segment appended as it is (`--chart-1` is `chart1`).
func FieldName(css string) string {
	s := strings.TrimPrefix(css, "--")
	s = strings.TrimPrefix(s, "al-")
	parts := strings.Split(s, "-")
	var b strings.Builder
	for i, p := range parts {
		if i > 0 && p != "" {
			p = strings.ToUpper(p[:1]) + p[1:]
		}
		b.WriteString(p)
	}
	return b.String()
}

// Removed are the tokens alder-ui v0.4 removed (Appendix C, "Removed tokens"), by their
// lowerCamel name as a theme field would spell them, each with what to write instead (or why
// there is nothing).
var Removed = map[string]string{
	"alRadiusSm":                  "write `radiusSm`",
	"alRadiusFull":                "it has none: write `9999px` in your own CSS",
	"alRadiusMd":                  "write `radiusMd`",
	"alRadiusLg":                  "write `radiusLg`",
	"alFontSans":                  "write `fontSans`",
	"alFontMono":                  "write `fontMono`",
	"alDestructiveText":           "write `destructive` (under `light` or `dark`)",
	"alShadowSm":                  "write `shadowSm`",
	"alTextXs":                    "it has none: text sizes are literal rem values",
	"alTextSm":                    "it has none: text sizes are literal rem values",
	"alTextBase":                  "it has none: text sizes are literal rem values",
	"alTextLg":                    "it has none: text sizes are literal rem values",
	"alLeading":                   "it has none: line heights are literal",
	"alLeadingTight":              "it has none: line heights are literal",
	"alWeightNormal":              "it has none: weights are literal",
	"alWeightMedium":              "it has none: weights are literal",
	"alWeightSemibold":            "it has none: weights are literal",
	"alControlHSm":                "it has none: control heights are literal rem values",
	"alControlHMd":                "it has none: control heights are literal rem values",
	"alControlHLg":                "it has none: control heights are literal rem values",
	"alCheckboxSize":              "it has none: the size is a literal `1rem`",
	"alBorderWidth":               "it has none: borders are a literal `1px`",
	"alRingWidth":                 "it has none: the focus ring is drawn from `ring`",
	"alRingOffset":                "it has none: the focus ring is drawn from `ring`",
	"alDisabledOpacity":           "it has none: the opacity is a literal `0.5`",
	"alDuration":                  "it has none: the duration is a literal `150ms`",
	"alInfoBorder":                "it has none: alerts no longer use it",
	"alSuccessBorder":             "it has none: alerts no longer use it",
	"alWarningBorder":             "it has none: alerts no longer use it",
	"alDestructiveBorder":         "it has none: alerts no longer use it",
	"alInfoSoft":                  "it has none: alerts no longer use it",
	"alSuccessSoft":               "it has none: alerts no longer use it",
	"alWarningSoft":               "it has none: alerts no longer use it",
	"alDestructiveSoft":           "it has none: alerts no longer use it",
	"alInfoSoftForeground":        "it has none: alerts no longer use it",
	"alSuccessSoftForeground":     "it has none: alerts no longer use it",
	"alWarningSoftForeground":     "it has none: alerts no longer use it",
	"alDestructiveSoftForeground": "it has none: alerts no longer use it",
}

func init() {
	for _, n := range []string{"05", "1", "15", "2", "3", "4", "6", "8", "10"} {
		Removed["alSpace"+n] = "it has none: spacing is literal rem values"
	}
}

// Version is the vendored alder-ui's version (rt/al/VERSION), as the messages name it.
var Version = sync.OnceValue(func() string {
	b, _ := rt.Files.ReadFile("al/VERSION")
	return strings.TrimSpace(string(b))
})

// Default is alder-ui's value of a token in the light and the dark scheme (the same for a
// theme-wide token).
type Default struct{ Light, Dark string }

// Defaults are alder-ui's values by CSS name, read once from the vendored stylesheet's token
// blocks: `:root, .light`, `.dark` and the theme-independent `:root`.
var Defaults = sync.OnceValue(func() map[string]Default {
	light, dark, root := Blocks()
	out := map[string]Default{}
	for name, v := range root {
		out[name] = Default{v, v}
	}
	for name, v := range light {
		d := out[name]
		d.Light = v
		if _, ok := dark[name]; !ok {
			d.Dark = v // --radius: in the light block, theme-wide
		}
		out[name] = d
	}
	for name, v := range dark {
		d := out[name]
		d.Dark = v
		out[name] = d
	}
	return out
})

var reDecl = regexp.MustCompile(`(?m)^\s*(--[A-Za-z0-9_-]+)\s*:\s*([^;]*);`)

// Blocks are the custom properties the vendored al.css declares in its light (`:root,
// .light`), dark (`.dark`) and theme-independent (`:root`) token blocks.
func Blocks() (light, dark, root map[string]string) {
	b, err := rt.Files.ReadFile("al/al.css")
	if err != nil {
		panic("theme: " + err.Error())
	}
	css := string(b)
	block := func(head string) map[string]string {
		i := strings.Index(css, head)
		if i < 0 {
			panic("theme: al.css has no " + head + " block")
		}
		body := css[i+len(head):]
		body = body[:strings.Index(body, "}")]
		out := map[string]string{}
		for _, m := range reDecl.FindAllStringSubmatch(body, -1) {
			out[m[1]] = strings.TrimSpace(m[2])
		}
		return out
	}
	return block("\n:root,\n.light {"), block("\n.dark {"), block("\n:root {")
}

// Suggest is the field name nearest to name by edit distance, if it is at most 2 away.
func Suggest(name string) (Token, bool) {
	best, bestD := Token{}, 3
	for _, t := range Tokens {
		if d := distance(strings.ToLower(name), strings.ToLower(t.Field)); d < bestD {
			best, bestD = t, d
		}
	}
	return best, bestD <= 2
}

func distance(a, b string) int {
	prev := make([]int, len(b)+1)
	cur := make([]int, len(b)+1)
	for j := range prev {
		prev[j] = j
	}
	for i := 1; i <= len(a); i++ {
		cur[0] = i
		for j := 1; j <= len(b); j++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}
			cur[j] = min(prev[j]+1, cur[j-1]+1, prev[j-1]+cost)
		}
		prev, cur = cur, prev
	}
	return prev[len(b)]
}

// ---- value shapes

var (
	reHex    = regexp.MustCompile(`^#(?:[0-9A-Fa-f]{3,4}|[0-9A-Fa-f]{6}|[0-9A-Fa-f]{8})$`)
	reFn     = regexp.MustCompile(`^(oklch|oklab|lch|lab|hsl|hwb|rgb|rgba|hsla|color|color-mix|light-dark)\(([A-Za-z0-9 .%/,()-]*)\)$`)
	reVar    = regexp.MustCompile(`^var\(\s*(--[A-Za-z0-9-]+)\s*\)$`)
	reLength = regexp.MustCompile(`^(?:0|(?:\d+(?:\.\d+)?|\.\d+)(?:rem|em|px|%|vw|vh|ch|lh))$`)
	reShadow = regexp.MustCompile(`^[A-Za-z0-9 .%/,()#-]+$`)
)

// ColourOK reports whether v (trimmed) is a colour value of the spec's shapes other than
// `var(…)`: hex, `transparent`, `currentColor`, or a colour function with balanced parentheses.
func ColourOK(v string) bool {
	switch {
	case reHex.MatchString(v), v == "transparent", v == "currentColor":
		return true
	}
	m := reFn.FindStringSubmatch(v)
	return m != nil && balanced(m[2])
}

// Alias is the CSS name a `var(--x)` value names; ok is false when v is not of that shape.
func Alias(v string) (name string, ok bool) {
	m := reVar.FindStringSubmatch(v)
	if m == nil {
		return "", false
	}
	return m[1], true
}

// LengthOK reports whether v is `0` or a non-negative decimal with a unit.
func LengthOK(v string) bool { return reLength.MatchString(v) }

// NegativeLength reports whether v is a length but for a leading `-`.
func NegativeLength(v string) bool {
	return strings.HasPrefix(v, "-") && LengthOK(strings.TrimSpace(v[1:]))
}

// FamilyOK reports whether v is a font family name the CSS can carry quoted: not blank, and
// without `"`, `\`, `;`, `{`, `}` or a control character.
func FamilyOK(v string) bool {
	if strings.TrimSpace(v) == "" || strings.ContainsAny(v, "\"\\;{}") {
		return false
	}
	for _, r := range v {
		if r < 0x20 || r == 0x7f {
			return false
		}
	}
	return true
}

// ShadowOK reports whether v is `none` or text of the shadow characters with balanced
// parentheses.
func ShadowOK(v string) bool {
	return v == "none" || reShadow.MatchString(v) && balanced(v)
}

func balanced(s string) bool {
	depth := 0
	for _, r := range s {
		switch r {
		case '(':
			depth++
		case ')':
			depth--
			if depth < 0 {
				return false
			}
		}
	}
	return depth == 0
}
