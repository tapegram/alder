package theme

import (
	"fmt"
	"os"
	"strings"
	"testing"
	"testing/fstest"
)

func mustImportDTCG(t *testing.T, dir, name string) (Theme, []Unplaced, Skipped) {
	t.Helper()
	th, un, sk, err := ImportDTCG(os.DirFS("testdata/dtcg/"+dir), name)
	if err != nil {
		t.Fatalf("ImportDTCG(%s/%s): %v", dir, name, err)
	}
	return th, un, sk
}

// importDoc imports one tokens document given as text.
func importDoc(t *testing.T, doc string) (Theme, []Unplaced) {
	t.Helper()
	th, un, _, err := ImportDTCG(fstest.MapFS{"t.json": {Data: []byte(doc)}}, "t.json")
	if err != nil {
		t.Fatalf("ImportDTCG(%s): %v", doc, err)
	}
	return th, un
}

func unplacedAt(t *testing.T, un []Unplaced, block string) Unplaced {
	t.Helper()
	for _, u := range un {
		if u.Block == block {
			return u
		}
	}
	t.Fatalf("no unplaced %q in %+v", block, un)
	return Unplaced{}
}

func TestImportDTCG_Resolver(t *testing.T) {
	th, un, sk := mustImportDTCG(t, "resolver", "theme.resolver.json")
	if th.Root["radius"] != "0.625rem" || th.Root["radiusSm"] != "0.25rem" {
		t.Errorf("Root = %v", th.Root)
	}
	if th.Light["primary"] != "oklch(0.5 0.2 260)" || th.Dark["primary"] != "oklch(0.7 0.15 260)" {
		t.Errorf("primary light %q dark %q", th.Light["primary"], th.Dark["primary"])
	}
	if th.Light["ring"] != "var(--primary)" {
		t.Errorf("ring = %q, want var(--primary)", th.Light["ring"])
	}
	// The shadow's colour aliases a colour that is no token of alder-ui: its value is substituted.
	if got := th.Root["shadowSm"]; got != "0 1px 2px 0 oklch(0.5 0.2 260)" {
		t.Errorf("shadowSm = %q", got)
	}
	if sk.N != 0 {
		t.Errorf("Skipped = %+v", sk)
	}
	u := unplacedAt(t, un, "base.tokens.json color.brand")
	if u.Name != "--brand" || u.Value != "oklch(0.5 0.2 260)" || u.Reason != "not a token of alder-ui "+Version() {
		t.Errorf("brand = %+v", u)
	}
}

func TestImportDTCG_SingleFileColourToBoth(t *testing.T) {
	th, un := importDoc(t, `{
		"color": {"$type": "color", "primary": {"$value": {"colorSpace": "oklch", "components": [0.5, 0.2, 260]}}},
		"light": {"color": {"$type": "color", "background": {"$value": {"colorSpace": "oklch", "components": [1, 0, 0]}}}},
		"dark": {"color": {"$type": "color", "primary": {"$value": {"colorSpace": "oklch", "components": [0.7, 0.1, 260]}}}}
	}`)
	if th.Light["primary"] != "oklch(0.5 0.2 260)" {
		t.Errorf("light primary = %q", th.Light["primary"])
	}
	// The dark group's own value beats the root's.
	if th.Dark["primary"] != "oklch(0.7 0.1 260)" {
		t.Errorf("dark primary = %q", th.Dark["primary"])
	}
	if th.Light["background"] != "oklch(1 0 0)" || th.Dark["background"] != "" || len(un) != 0 {
		t.Errorf("light %v dark %v unplaced %v", th.Light, th.Dark, un)
	}
	// A root colour alone lands in both schemes.
	th, _ = importDoc(t, `{"color": {"$type": "color", "card": {"$value": {"colorSpace": "oklch", "components": [0.9, 0, 0]}}}}`)
	if th.Light["card"] != "oklch(0.9 0 0)" || th.Dark["card"] != "oklch(0.9 0 0)" {
		t.Errorf("card light %q dark %q", th.Light["card"], th.Dark["card"])
	}
}

func TestImportDTCG_Names(t *testing.T) {
	th, un, _ := mustImportDTCG(t, "foreign-names", "theme.resolver.json")
	for _, block := range []string{"light.tokens.json color.brand-500", "dark.tokens.json color.brand-500"} {
		u := unplacedAt(t, un, block)
		if u.Name != "--brand-500" || u.Reason != "not a token of alder-ui "+Version() {
			t.Errorf("%s = %+v", block, u)
		}
	}
	if th.Light["primaryForeground"] != "oklch(0.98 0 0)" || th.Dark["primaryForeground"] != "oklch(0.2 0 0)" {
		t.Errorf("primaryForeground light %q dark %q", th.Light["primaryForeground"], th.Dark["primaryForeground"])
	}
	if th.Root["widthNarrow"] != "36rem" || th.Root["borderWidth"] != "2px" {
		t.Errorf("Root = %v", th.Root)
	}
	u := unplacedAt(t, un, "base.tokens.json radius.al-radius-sm")
	if u.Name != "--al-radius-sm" || u.Value != "0.25rem" || u.Reason != "`--al-radius-sm` was removed in alder-ui v0.4: write `radiusSm`" {
		t.Errorf("removed = %+v", u)
	}
	if len(un) != 3 {
		t.Errorf("unplaced = %+v, want 3", un)
	}
}

func TestImportDTCG_Aliases(t *testing.T) {
	th, un, _ := mustImportDTCG(t, "aliases", "tokens.json")
	for field, want := range map[string]string{
		"primary":                  "oklch(0.5 0.2 260)", // brand is no token of alder-ui: substituted
		"ring":                     "var(--primary)",
		"accent":                   "var(--primary)", // no $type: the alias's target gives it
		"sidebarPrimaryForeground": "var(--primary-foreground)",
	} {
		if th.Light[field] != want || th.Dark[field] != want {
			t.Errorf("%s = light %q dark %q, want %q", field, th.Light[field], th.Dark[field], want)
		}
	}
	if th.Root["radiusSm"] != "0.5rem" || th.Root["radiusMd"] != "0.5rem" {
		t.Errorf("Root = %v", th.Root)
	}
	if len(un) != 1 || un[0].Name != "--brand" {
		t.Errorf("unplaced = %+v", un)
	}

	// cssText knows no alder names: any colour-typed target is a var() of its last segment.
	doc, err := loadDTCG(os.DirFS("testdata/dtcg/aliases"), "tokens.json")
	if err != nil {
		t.Fatal(err)
	}
	for path, want := range map[string]string{
		"color.primary":                    "var(--brand)",
		"color.sidebar-primary-foreground": "var(--primary-foreground)",
		"radius.radius-md":                 "0.5rem",
	} {
		got, err := doc.cssText(tokenAt(t, doc.Root, path), "")
		if err != nil || got != want {
			t.Errorf("cssText(%s) = %q, %v; want %q", path, got, err, want)
		}
	}
}

func TestImportDTCG_Colours(t *testing.T) {
	for _, c := range []struct{ value, want string }{
		{`{"colorSpace": "oklch", "components": [0.3, 0.03, 50]}`, "oklch(0.3 0.03 50)"},
		{`{"colorSpace": "hsl", "components": [210, 40, 50]}`, "hsl(210 40% 50%)"},
		{`{"colorSpace": "srgb", "components": [0.2, 0.4, 0.6], "alpha": 1, "hex": "#336699"}`, "#336699"},
		{`{"colorSpace": "srgb", "components": [0, 0, 0], "alpha": 0.05}`, "rgb(0 0 0 / 0.05)"},
		{`{"colorSpace": "srgb", "components": [1, 0.5, 0]}`, "rgb(255 127.5 0)"},
		{`{"colorSpace": "srgb", "components": [0.298039, 0.309804, 0.411765]}`, "rgb(76 79 105)"},
		{`{"colorSpace": "display-p3", "components": [1, 0, 0]}`, "color(display-p3 1 0 0)"},
		{`{"colorSpace": "oklch", "components": [0.5, 0.1, 30], "alpha": 0.5}`, "oklch(0.5 0.1 30 / 0.5)"},
		{`{"colorSpace": "oklch", "components": [0.5, "none", 30]}`, "oklch(0.5 none 30)"},
	} {
		th, un := importDoc(t, `{"color": {"$type": "color", "primary": {"$value": `+c.value+`}}}`)
		if th.Light["primary"] != c.want || th.Dark["primary"] != c.want || len(un) != 0 {
			t.Errorf("%s → light %q dark %q (unplaced %+v), want %q", c.value, th.Light["primary"], th.Dark["primary"], un, c.want)
		}
	}
	for _, bad := range []string{
		`"#ffffff"`, // a bare string where the format wants an object
		`{"colorSpace": "cmyk", "components": [0, 0, 0]}`,
		`{"colorSpace": "oklch", "components": [0.5, 0.1]}`,
	} {
		_, un := importDoc(t, `{"color": {"$type": "color", "primary": {"$value": `+bad+`}}, "radius": {"$type": "dimension", "$value": {"value": 1, "unit": "rem"}}}`)
		if len(un) != 1 || un[0].Name != "--primary" || un[0].Reason == "" {
			t.Errorf("%s: unplaced = %+v, want one", bad, un)
		}
	}
}

func TestImportDTCG_Dimensions(t *testing.T) {
	for _, c := range []struct{ value, want string }{
		{`{"value": 0, "unit": "rem"}`, "0"},
		{`{"value": 0.5, "unit": "rem"}`, "0.5rem"},
		{`{"value": 12, "unit": "px"}`, "12px"},
	} {
		th, un := importDoc(t, `{"radius": {"$type": "dimension", "$value": `+c.value+`}}`)
		if th.Root["radius"] != c.want || len(un) != 0 {
			t.Errorf("%s → %q (unplaced %+v), want %q", c.value, th.Root["radius"], un, c.want)
		}
	}
	_, un := importDoc(t, `{"radius": {"$type": "dimension", "$value": {"value": -0.5, "unit": "rem"}},
		"color": {"$type": "color", "primary": {"$value": {"colorSpace": "oklch", "components": [0.5, 0, 0]}}}}`)
	if len(un) != 1 || un[0].Value != "-0.5rem" || un[0].Reason != NegativeLengthReason("radius") {
		t.Errorf("negative radius: unplaced = %+v", un)
	}
}

func TestImportDTCG_Shadow(t *testing.T) {
	th, un := importDoc(t, `{"shadow": {"$type": "shadow", "shadow-sm": {"$value": [
		{"color": {"colorSpace": "srgb", "components": [0, 0, 0], "alpha": 0.05},
		 "offsetX": {"value": 0, "unit": "px"}, "offsetY": {"value": 1, "unit": "px"},
		 "blur": {"value": 2, "unit": "px"}, "spread": {"value": 0, "unit": "px"}},
		{"color": {"colorSpace": "srgb", "components": [0, 0, 0], "hex": "#000000"},
		 "offsetX": {"value": 0, "unit": "px"}, "offsetY": {"value": 0, "unit": "px"},
		 "blur": {"value": 0, "unit": "px"}, "spread": {"value": 1, "unit": "px"}, "inset": true}
	]}}}`)
	if got, want := th.Root["shadowSm"], "0 1px 2px 0 rgb(0 0 0 / 0.05), inset 0 0 0 1px #000000"; got != want || len(un) != 0 {
		t.Errorf("shadowSm = %q (unplaced %+v), want %q", got, un, want)
	}

	th, un = importDoc(t, `{
		"light": {"color": {"$type": "color", "border": {"$value": {"colorSpace": "oklch", "components": [0.9, 0, 0]}}}},
		"dark": {"color": {"$type": "color", "border": {"$value": {"colorSpace": "oklch", "components": [0.3, 0, 0]}}}},
		"shadow": {"$type": "shadow", "shadow-xs": {"$value": {"color": "{color.border}",
			"offsetX": {"value": 2, "unit": "px"}, "offsetY": {"value": 2, "unit": "px"},
			"blur": {"value": 0, "unit": "px"}, "spread": {"value": 0, "unit": "px"}}}}
	}`)
	if got, want := th.Root["shadowXs"], "2px 2px 0 0 var(--border)"; got != want || len(un) != 0 {
		t.Errorf("shadowXs = %q (unplaced %+v), want %q", got, un, want)
	}
}

func TestImportDTCG_Fonts(t *testing.T) {
	th, un := importDoc(t, `{"font": {"$type": "fontFamily", "font-sans": {"$value": ["Geist", "ui-sans-serif"]}, "font-mono": {"$value": "Geist Mono"}}}`)
	if th.Root["fontSans"] != "Geist, ui-sans-serif" || th.Root["fontMono"] != "Geist Mono" || len(un) != 0 {
		t.Errorf("Root = %v unplaced %+v", th.Root, un)
	}
	_, un = importDoc(t, `{"font": {"$type": "fontFamily", "font-sans": {"$value": ["Bad;Font", "serif"]}},
		"radius": {"$type": "dimension", "$value": {"value": 1, "unit": "rem"}}}`)
	if len(un) != 1 || !strings.Contains(un[0].Reason, "is not a font stack alder-ui accepts") {
		t.Errorf("bad family: unplaced = %+v", un)
	}
}

func TestImportDTCG_BadTypes(t *testing.T) {
	th, un, _ := mustImportDTCG(t, "bad-types", "tokens.json")
	if th.Light["primary"] == "" {
		t.Errorf("primary not placed: %v", th.Light)
	}
	for block, want := range map[string]string{
		"tokens.json radius":          "`radius` is a length; the file says `color`",
		"tokens.json misc.radius-sm":  "no $type",
		"tokens.json color.ring":      "`{color.nope}` is not a token",
		"tokens.json typography.body": "alder-ui has no `typography` tokens",
		"tokens.json motion.fast":     "alder-ui has no `duration` tokens",
	} {
		if u := unplacedAt(t, un, block); u.Reason != want {
			t.Errorf("%s: reason %q, want %q", block, u.Reason, want)
		}
	}
	// No CSS text was produced: the raw JSON stands in, as a comment.
	if u := unplacedAt(t, un, "tokens.json motion.fast"); u.Value != `/* {"value":150,"unit":"ms"} */` {
		t.Errorf("motion.fast value = %q", u.Value)
	}
	if len(un) != 5 {
		t.Errorf("unplaced = %+v, want 5", un)
	}

	for name, doc := range map[string]string{
		"dimension cycle": `{"radius": {"$type": "dimension", "radius": {"$value": "{radius.radius-sm}"}, "radius-sm": {"$value": "{radius.radius}"}}}`,
		"colour cycle":    `{"color": {"$type": "color", "primary": {"$value": "{color.ring}"}, "ring": {"$value": "{color.primary}"}}}`,
		"self":            `{"radius": {"$type": "dimension", "$value": "{radius}"}}`,
	} {
		if _, _, _, err := ImportDTCG(fstest.MapFS{"t.json": {Data: []byte(doc)}}, "t.json"); err == nil || !strings.Contains(err.Error(), "cycle") {
			t.Errorf("%s: err = %v, want an alias cycle", name, err)
		}
	}
}

func TestImportDTCG_ThemeWideInScheme(t *testing.T) {
	th, un := importDoc(t, `{
		"light": {"radius": {"$type": "dimension", "$value": {"value": 1, "unit": "rem"}}, "radius-sm": {"$type": "dimension", "$value": {"value": 2, "unit": "px"}}},
		"dark": {"radius": {"$type": "dimension", "$value": {"value": 1, "unit": "rem"}}, "radius-sm": {"$type": "dimension", "$value": {"value": 4, "unit": "px"}}}
	}`)
	if th.Root["radius"] != "1rem" || th.Root["radiusSm"] != "2px" {
		t.Errorf("Root = %v", th.Root)
	}
	if len(un) != 1 || un[0].Block != "t.json radius-sm" || un[0].Reason != "theme-wide in alder-ui; the dark value differs" {
		t.Errorf("unplaced = %+v", un)
	}
}

func TestImportDTCG_NoTheme(t *testing.T) {
	_, _, _, err := ImportDTCG(fstest.MapFS{"t.json": {Data: []byte(`{"color": {"$type": "color", "brand": {"$value": {"colorSpace": "oklch", "components": [0.5, 0, 0]}}}}`)}}, "t.json")
	if err == nil || !strings.Contains(err.Error(), "t.json") {
		t.Errorf("err = %v, want no theme in t.json", err)
	}
}

// Task B1 builds the presets from ImportDTCG: each must place every token.
func TestImportDTCG_Presets(t *testing.T) {
	for _, name := range []string{"zinc", "brutal", "catppuccin", "paper", "pebble"} {
		th, un, _, err := ImportDTCG(os.DirFS("presets/"+name), "theme.resolver.json")
		if err != nil {
			t.Errorf("%s: %v", name, err)
			continue
		}
		if len(un) != 0 {
			t.Errorf("%s: unplaced %+v", name, un)
		}
		if len(th.Light) != 32 || len(th.Dark) != 32 || len(th.Root) != 15 {
			t.Errorf("%s: light %d, dark %d, theme-wide %d; want 32, 32, 15", name, len(th.Light), len(th.Dark), len(th.Root))
		}
	}
}

func TestUnplacedCSS_DTCG(t *testing.T) {
	_, un, _ := mustImportDTCG(t, "foreign-names", "theme.resolver.json")
	v := Version()
	want := fmt.Sprintf(`/* base.tokens.json */
:root {
  --al-radius-sm: 0.25rem; /* radius.al-radius-sm: `+"`--al-radius-sm`"+` was removed in alder-ui v0.4: write `+"`radiusSm`"+` */
}

/* light.tokens.json */
:root {
  --brand-500: oklch(0.6 0.2 30); /* color.brand-500: not a token of alder-ui %s */
}

/* dark.tokens.json */
:root {
  --brand-500: oklch(0.7 0.2 30); /* color.brand-500: not a token of alder-ui %s */
}
`, v, v)
	if got := UnplacedCSS(un); got != want {
		t.Errorf("got:\n%s\nwant:\n%s", got, want)
	}
}
