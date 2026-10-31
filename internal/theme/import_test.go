package theme

import (
	"errors"
	"os"
	"reflect"
	"strings"
	"testing"
)

func fixture(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile("testdata/import/" + name + ".css")
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func mustImport(t *testing.T, css string) (Theme, []Unplaced, Skipped) {
	t.Helper()
	th, un, sk, err := Import(css)
	if err != nil {
		t.Fatalf("Import: %v", err)
	}
	return th, un, sk
}

func findUnplaced(us []Unplaced, block, name string) (Unplaced, bool) {
	for _, u := range us {
		if u.Name == name && (block == "" || u.Block == block) {
			return u, true
		}
	}
	return Unplaced{}, false
}

func TestFamilies(t *testing.T) {
	cases := map[string][]string{
		"'Inter', ui-sans-serif, system-ui, sans-serif": {"Inter", "ui-sans-serif", "system-ui", "sans-serif"},
		"\"Geist Mono\", monospace":                     {"Geist Mono", "monospace"},
		"Lora, , Georgia,":                              {"Lora", "Georgia"},
		"'A, B', serif":                                 {"A, B", "serif"},
	}
	for in, want := range cases {
		if got := Families(in); !reflect.DeepEqual(got, want) {
			t.Errorf("Families(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestThemeEmpty(t *testing.T) {
	if !(Theme{}).Empty() {
		t.Error("zero Theme should be empty")
	}
	if (Theme{Light: map[string]string{"primary": "red"}}).Empty() {
		t.Error("Theme with a light value is not empty")
	}
}

func TestImport_ShadcnV4(t *testing.T) {
	th, un, sk := mustImport(t, fixture(t, "shadcn-v4"))
	for _, tok := range Tokens {
		if tok.Kind != Colour {
			continue
		}
		if th.Light[tok.Field] == "" {
			t.Errorf("light %s missing", tok.Field)
		}
		if th.Dark[tok.Field] == "" {
			t.Errorf("dark %s missing", tok.Field)
		}
	}
	if th.Root["radius"] != "0.625rem" {
		t.Errorf("radius = %q", th.Root["radius"])
	}
	if th.Dark["border"] != "oklch(1 0 0 / 10%)" {
		t.Errorf("dark border = %q", th.Dark["border"])
	}
	if sk.N != 2 || !reflect.DeepEqual(sk.Kinds, []string{"@custom-variant", "@theme inline"}) {
		t.Errorf("skipped = %+v", sk)
	}
	if len(un) != 0 {
		t.Errorf("unplaced = %+v", un)
	}
}

func TestImport_ShadcnV3WrapsHSL(t *testing.T) {
	th, un, sk := mustImport(t, fixture(t, "shadcn-v3"))
	if got := th.Light["primary"]; got != "hsl(222.2 47.4% 11.2%)" {
		t.Errorf("light primary = %q", got)
	}
	if got := th.Dark["sidebarBorder"]; got != "hsl(217.2 32.6% 17.5% / 0.5)" {
		t.Errorf("dark sidebarBorder = %q", got)
	}
	if th.Root["radius"] != "0.5rem" {
		t.Errorf("radius = %q", th.Root["radius"])
	}
	if sk.N != 3 { // three @tailwind statements
		t.Errorf("skipped = %+v", sk)
	}
	if u, ok := findUnplaced(un, ":root", "--sidebar-background"); !ok || u.Reason != "not a token of alder-ui "+Version() {
		t.Errorf("--sidebar-background unplaced = %+v, %v", u, ok)
	}
}

func TestImport_TweakcnUnplaced(t *testing.T) {
	th, un, _ := mustImport(t, fixture(t, "tweakcn"))
	want := "not a token of alder-ui " + Version()
	for _, n := range []string{"--font-serif", "--tracking-normal", "--spacing", "--shadow-2xs", "--shadow-xl",
		"--shadow-2xl", "--shadow-color", "--shadow-opacity", "--shadow-blur", "--shadow-spread", "--shadow-x", "--shadow-y"} {
		u, ok := findUnplaced(un, ":root", n)
		if !ok || u.Reason != want {
			t.Errorf("%s: %+v, %v", n, u, ok)
		}
	}
	u, ok := findUnplaced(un, ".dark", "--shadow-sm")
	if !ok || u.Reason != "theme-wide in alder-ui; the dark value differs" {
		t.Errorf("dark --shadow-sm: %+v, %v", u, ok)
	}
	for _, n := range []string{"--shadow-xs", "--shadow-md"} {
		if _, ok := findUnplaced(un, ".dark", n); ok {
			t.Errorf("dark %s equals root's and should be accepted", n)
		}
	}
	if th.Root["fontSans"] != "Inter, ui-sans-serif, system-ui, sans-serif" {
		t.Errorf("fontSans = %q", th.Root["fontSans"])
	}
	if th.Root["fontMono"] != "Geist Mono, monospace" {
		t.Errorf("fontMono = %q", th.Root["fontMono"])
	}
	if th.Root["shadowSm"] == "" || th.Root["shadowLg"] == "" {
		t.Errorf("shadows not placed: %v", th.Root)
	}
	if th.Light["primary"] == "" || th.Dark["primary"] == "" {
		t.Error("colours not placed")
	}
}

func TestImport_RemovedName(t *testing.T) {
	_, un, _ := mustImport(t, ":root { --primary: #f00; --al-font-mono: monospace; }")
	u, ok := findUnplaced(un, ":root", "--al-font-mono")
	want := "`--al-font-mono` was removed in alder-ui v0.4: " + Removed["alFontMono"]
	if !ok || u.Reason != want {
		t.Errorf("got %+v, %v; want reason %q", u, ok, want)
	}
}

func TestImport_BadValues(t *testing.T) {
	th, un, _ := mustImport(t, ":root { --primary: #f00; --sidebar-accent: var(--sidebar-accent-2); --radius: calc(0.5rem + 2px); --radius-sm: -2px; --font-sans: var(--x), serif; --shadow-sm: 0 1px @ 2px; }")
	u, _ := findUnplaced(un, "", "--sidebar-accent")
	if u.Reason != "`var(--sidebar-accent-2)` is not a colour token" {
		t.Errorf("alias reason = %q", u.Reason)
	}
	u, _ = findUnplaced(un, "", "--radius")
	if !strings.HasPrefix(u.Reason, "`--radius: calc(0.5rem + 2px)` is not a length alder-ui accepts") {
		t.Errorf("length reason = %q", u.Reason)
	}
	for _, n := range []string{"--radius-sm", "--font-sans", "--shadow-sm"} {
		if _, ok := findUnplaced(un, "", n); !ok {
			t.Errorf("%s should be unplaced", n)
		}
	}
	if th.Root["radius"] != "" || th.Light["sidebarAccent"] != "" {
		t.Errorf("bad values placed: %+v", th)
	}
}

func TestImport_ColourAlias(t *testing.T) {
	th, un, _ := mustImport(t, ":root { --primary: #fff; --ring: var(--primary); --foreground: rgb(0 0 0) !important; }")
	if th.Light["ring"] != "var(--primary)" || len(un) != 0 {
		t.Errorf("th=%+v un=%+v", th, un)
	}
}

func TestImport_SelectorUnplaced(t *testing.T) {
	th, un, _ := mustImport(t, ":root { --primary: #00f; }\n.sidebar { --primary: red; color: red; }")
	if th.Light["primary"] != "#00f" {
		t.Errorf("primary = %q", th.Light["primary"])
	}
	if len(un) != 1 {
		t.Fatalf("unplaced = %+v", un)
	}
	if un[0].Block != ".sidebar" || un[0].Reason != "`.sidebar` is not a scheme selector" || un[0].Name != "--primary" || un[0].Value != "red" {
		t.Errorf("unplaced = %+v", un[0])
	}
}

func TestImport_SelectorForms(t *testing.T) {
	css := `html { --primary: #111; }
.light { --secondary: #222; }
[data-theme="light"] { --muted: #333; }
:root.light { --accent: #444; }
html.dark { --primary: #aaa; }
[data-theme='dark'] { --secondary: #bbb; }
:root.dark { --muted: #ccc; }
.dark:root { --accent: #ddd; }`
	th, un, _ := mustImport(t, css)
	if len(un) != 0 {
		t.Errorf("unplaced = %+v", un)
	}
	wantL := map[string]string{"primary": "#111", "secondary": "#222", "muted": "#333", "accent": "#444"}
	wantD := map[string]string{"primary": "#aaa", "secondary": "#bbb", "muted": "#ccc", "accent": "#ddd"}
	if !reflect.DeepEqual(th.Light, wantL) || !reflect.DeepEqual(th.Dark, wantD) {
		t.Errorf("light=%v dark=%v", th.Light, th.Dark)
	}
}

func TestImport_ImportantDropped(t *testing.T) {
	th, un, _ := mustImport(t, ":root { --primary: #123456 !important; --radius: 1rem ! important; }")
	if th.Light["primary"] != "#123456" || th.Root["radius"] != "1rem" || len(un) != 0 {
		t.Errorf("th=%+v un=%+v", th, un)
	}
}

func TestImport_LastWins(t *testing.T) {
	th, _, _ := mustImport(t, ":root { --primary: #111; --primary: #222; }\n:root { --secondary: #333; }\n:root { --secondary: #444; }")
	if th.Light["primary"] != "#222" || th.Light["secondary"] != "#444" {
		t.Errorf("light = %v", th.Light)
	}
}

func TestImport_NoTheme(t *testing.T) {
	_, _, _, err := Import(fixture(t, "garbage"))
	want := "no theme in the input: expected a `:root { --primary: … }` or `.dark { … }` block"
	if err == nil || err.Error() != want {
		t.Fatalf("err = %v", err)
	}
	var nt *NoThemeError
	if !errors.As(err, &nt) {
		t.Fatalf("not a *NoThemeError: %T", err)
	}
	nt.Name = "theme.css"
	if got := nt.Error(); got != "no theme in `theme.css`: expected a `:root { --primary: … }` or `.dark { … }` block" {
		t.Errorf("named = %q", got)
	}
}

func TestImport_SidebarOnly(t *testing.T) {
	th, un, sk := mustImport(t, fixture(t, "sidebar-only"))
	if len(th.Light) != 8 || len(th.Dark) != 0 || len(th.Root) != 0 || len(un) != 0 || sk.N != 0 {
		t.Errorf("th=%+v un=%+v sk=%+v", th, un, sk)
	}
}

func TestImport_MediaDarkRoot(t *testing.T) {
	th, un, _ := mustImport(t, fixture(t, "media-dark"))
	if th.Light["background"] != "#ffffff" || th.Dark["background"] != "#0b0b0f" || th.Dark["primary"] != "#818cf8" {
		t.Errorf("th = %+v", th)
	}
	if th.Root["radius"] != "0.5rem" || len(un) != 0 {
		t.Errorf("root=%v un=%+v", th.Root, un)
	}
}

func TestImport_MediaDarkUnplacedKeepsContext(t *testing.T) {
	css := ":root { --radius: 0.5rem; }\n@media (prefers-color-scheme: dark) { :root { --radius: 1rem; --font-serif: serif; } }"
	_, un, _ := mustImport(t, css)
	if u, ok := findUnplaced(un, "", "--radius"); !ok || u.Reason != "theme-wide in alder-ui; the dark value differs" {
		t.Errorf("radius: %+v %v", u, ok)
	}
	out := UnplacedCSS(un)
	if !strings.Contains(out, "@media (prefers-color-scheme: dark) {") {
		t.Errorf("media context lost:\n%s", out)
	}
}

func TestImport_CommentsAndStrings(t *testing.T) {
	css := "/* :root { --primary: red; } */\n:root { /* c */ --primary: #abc; --font-sans: \"A B\", serif; }"
	th, un, _ := mustImport(t, css)
	if th.Light["primary"] != "#abc" || th.Root["fontSans"] != "A B, serif" || len(un) != 0 {
		t.Errorf("th=%+v un=%+v", th, un)
	}
}

func TestUnplacedCSS_Valid(t *testing.T) {
	us := []Unplaced{
		{Block: ":root, .light", Name: "--font-serif", Value: "Lora, Georgia, serif", Reason: "not a token of alder-ui v0.4.2"},
		{Block: ".dark", Name: "--shadow-sm", Value: "0 1px 3px 0 hsl(0 0% 0% / 0.4)", Reason: "theme-wide in alder-ui; the dark value differs"},
		{Block: ":root, .light", Name: "--tracking-normal", Value: "0em", Reason: "not a token of alder-ui v0.4.2"},
	}
	want := `:root, .light {
  --font-serif: Lora, Georgia, serif; /* not a token of alder-ui v0.4.2 */
  --tracking-normal: 0em; /* not a token of alder-ui v0.4.2 */
}

.dark {
  --shadow-sm: 0 1px 3px 0 hsl(0 0% 0% / 0.4); /* theme-wide in alder-ui; the dark value differs */
}
`
	if got := UnplacedCSS(us); got != want {
		t.Errorf("got:\n%s\nwant:\n%s", got, want)
	}
	if UnplacedCSS(nil) != "" {
		t.Error("no unplaced, no CSS")
	}
}

func TestUnplacedCSS_ReasonCannotCloseComment(t *testing.T) {
	out := UnplacedCSS([]Unplaced{{Block: ".x", Name: "--a", Value: "1", Reason: "has */ in it"}})
	if strings.Count(out, "*/") != 1 {
		t.Errorf("comment closed early:\n%s", out)
	}
}

func TestImport_NotDarkIsLight(t *testing.T) {
	th, un, _ := mustImport(t, LightSelector+" { --primary: #111; --radius: 1rem }")
	if th.Light["primary"] == "" || len(th.Dark) != 0 || th.Root["radius"] != "1rem" || len(un) != 0 {
		t.Errorf("light %v dark %v root %v unplaced %v", th.Light, th.Dark, th.Root, un)
	}
	th, _, _ = mustImport(t, ":root:not(.dark) { --primary: #111 }")
	if th.Light["primary"] == "" || len(th.Dark) != 0 {
		t.Errorf("light %v dark %v", th.Light, th.Dark)
	}
}
