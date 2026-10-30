package theme

import (
	"strings"
	"testing"
)

// Re-vendoring alder-ui with a token added or dropped fails here until Tokens follows.
func TestTokens_MatchVendoredStylesheet(t *testing.T) {
	light, dark, root := Blocks()
	declared := map[string]bool{}
	for _, m := range []map[string]string{light, dark, root} {
		for name := range m {
			if name == "--color-white" || name == "--color-black" || strings.HasPrefix(name, "--_al-") {
				continue
			}
			declared[name] = true
		}
	}
	for name := range declared {
		if _, ok := ByCSS(name); !ok {
			t.Errorf("al.css declares %s, which theme.Tokens lacks", name)
		}
	}
	for _, tok := range Tokens {
		if !declared[tok.CSS] {
			t.Errorf("theme.Tokens has %s, which al.css's token blocks do not declare", tok.CSS)
		}
		if _, inDark := dark[tok.CSS]; inDark != (tok.Kind == Colour) {
			t.Errorf("%s: kind %s, but declared in .dark = %v", tok.CSS, tok.Kind, inDark)
		}
	}
}

func TestFieldName_Mechanical(t *testing.T) {
	for _, tok := range Tokens {
		if got := FieldName(tok.CSS); got != tok.Field {
			t.Errorf("FieldName(%s) = %s, want %s", tok.CSS, got, tok.Field)
		}
	}
	if got := FieldName("--al-font-mono"); got != "fontMono" {
		t.Errorf("FieldName(--al-font-mono) = %s", got)
	}
}

func TestDefaults_EveryToken(t *testing.T) {
	d := Defaults()
	for _, tok := range Tokens {
		v := d[tok.CSS]
		if v.Light == "" || v.Dark == "" {
			t.Errorf("%s: defaults %+v", tok.CSS, v)
		}
	}
	if p := d["--primary"]; p.Light != "oklch(0.205 0 0)" || p.Dark != "oklch(0.922 0 0)" {
		t.Errorf("--primary defaults %+v", p)
	}
	if r := d["--radius"]; r.Light != "0.625rem" || r.Dark != "0.625rem" {
		t.Errorf("--radius defaults %+v", r)
	}
}

func TestCSS_SpecExample(t *testing.T) {
	got := CSS(
		[]Setting{{"--font-sans", FontStack([]string{"Inter", "system-ui", "sans-serif"})}, {"--radius", "0.5rem"}},
		[]Setting{{"--primary", "oklch(0.55 0.22 285)"}, {"--primary-foreground", "#ffffff"}, {"--ring", "var(--primary)"}},
		[]Setting{{"--primary", "oklch(0.72 0.16 285)"}, {"--primary-foreground", "oklch(0.145 0 0)"}},
	)
	want := `/* theme: App */
:root { --radius: 0.5rem; --font-sans: "Inter", system-ui, sans-serif; }
:where(:root:not(.dark)), .light { --primary: oklch(0.55 0.22 285); --primary-foreground: #ffffff; --ring: var(--primary); }
.dark { --primary: oklch(0.72 0.16 285); --primary-foreground: oklch(0.145 0 0); }
`
	if got != want {
		t.Errorf("CSS:\n%s\nwant:\n%s", got, want)
	}
}

func TestCSS_Empty(t *testing.T) {
	if got := CSS(nil, nil, nil); got != "" {
		t.Errorf("CSS of an empty theme = %q", got)
	}
	if got := CSS(nil, nil, []Setting{{"--ring", "red"}}); strings.Contains(got, ":root") {
		t.Errorf("empty blocks are not left out: %q", got)
	}
}

func TestFontStack_QuotesNonGeneric(t *testing.T) {
	if got := FontStack([]string{"Geist Mono", " monospace ", "Serif"}); got != `"Geist Mono", monospace, Serif` {
		t.Errorf("FontStack = %s", got)
	}
}

func TestValueShapes(t *testing.T) {
	for _, c := range []struct {
		f    func(string) bool
		v    string
		want bool
	}{
		{ColourOK, "#fff", true}, {ColourOK, "#4f46e5", true}, {ColourOK, "#4f46e5cc", true}, {ColourOK, "#4f46e", false},
		{ColourOK, "transparent", true}, {ColourOK, "currentColor", true}, {ColourOK, "red", false},
		{ColourOK, "oklch(0.55 0.22 285)", true}, {ColourOK, "oklch(1 0 0 / 10%)", true},
		{ColourOK, "color-mix(in oklab, var(--ring) 50%, transparent)", true},
		{ColourOK, "oklch(0.5 0 0); } body { color: red", false}, {ColourOK, "oklch((0.5)", false},
		{ColourOK, "url(x)", false},
		{LengthOK, "0", true}, {LengthOK, "0.5rem", true}, {LengthOK, ".5rem", true}, {LengthOK, "12px", true},
		{LengthOK, "40%", true}, {LengthOK, "5", false}, {LengthOK, "-1rem", false}, {LengthOK, "1 rem", false},
		{NegativeLength, "-1rem", true}, {NegativeLength, "1rem", false},
		{FamilyOK, "Geist Mono", true}, {FamilyOK, "", false}, {FamilyOK, `a"b`, false}, {FamilyOK, "a;b", false},
		{FamilyOK, `a\b`, false}, {FamilyOK, "a\tb", false}, {FamilyOK, "a\x7fb", false}, {FamilyOK, "a\nb", false},
		{ShadowOK, "none", true}, {ShadowOK, "0 1px 2px 0 rgb(0 0 0 / 0.05)", true}, {ShadowOK, "0 0 #000", true},
		{ShadowOK, "", false}, {ShadowOK, "0 0 red; }", false},
	} {
		if got := c.f(c.v); got != c.want {
			t.Errorf("%q: %v, want %v", c.v, got, c.want)
		}
	}
	if n, ok := Alias("var(--accent)"); !ok || n != "--accent" {
		t.Errorf("Alias = %q %v", n, ok)
	}
}

func TestSuggest(t *testing.T) {
	if tok, ok := Suggest("primray"); !ok || tok.Field != "primary" {
		t.Errorf("Suggest(primray) = %v %v", tok, ok)
	}
	if _, ok := Suggest("zzzzzzzz"); ok {
		t.Error("Suggest(zzzzzzzz) found something")
	}
	if !strings.HasPrefix(Version(), "v0.") {
		t.Errorf("Version = %q", Version())
	}
}
