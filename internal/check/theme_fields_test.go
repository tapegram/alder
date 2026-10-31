package check

import (
	"testing"

	"alder/internal/syntax"
	"alder/internal/theme"
)

// themeOf parses `theme { body }` in a module of its own and returns the declaration's fields.
func themeOf(t *testing.T, body string) []*syntax.DeployField {
	t.Helper()
	m, diags := syntax.ParseFile("App.alder", []byte("module App\n\ntheme {\n"+body+"\n}\n"))
	if len(diags) > 0 {
		t.Fatal(diags)
	}
	for _, d := range m.Decls {
		if td, ok := d.(*syntax.ThemeDecl); ok {
			return td.Fields
		}
	}
	t.Fatal("no theme declaration")
	return nil
}

// The messages are those of testdata/corpus/E0042-*/expected.diag for the same cases.
func TestThemeFields_SameMessagesAsChecker(t *testing.T) {
	cases := []struct{ name, body, want string }{
		{"colour-top-level", `primary: "#4f46e5",`, "`primary` is a colour: set it under `light` or `dark`"},
		{"negative-length", `radius: "-1px",`, "`radius` is a length and cannot be negative: write one such as `\"0.5rem\"`"},
		{"var-unknown", `light: { primary: "var(--nope)" },`, "`var(--nope)` is not a token of alder-ui " + theme.Version() + ": a colour may alias a colour token, such as `\"var(--accent)\"`"},
		{"bad-colour", `light: { primary: "a\"b" },`, "`primary` is a colour written as text: `\"oklch(0.55 0.22 285)\"`, `\"#4f46e5\"` or `\"var(--accent)\"`"},
		// E0042-alias-self's message, for `primary` instead of `ring`
		{"alias-self", `light: { primary: "var(--primary)" },`, "`primary` is `var(--primary)` in `light`: a colour cannot be defined by itself; give it a colour value"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := ThemeFields(themeOf(t, tc.body))
			if len(got) != 1 {
				t.Fatalf("got %d diagnostics, want 1: %v", len(got), got)
			}
			if got[0].Code != "E0042" || got[0].Message != tc.want {
				t.Errorf("got %s %q\nwant E0042 %q", got[0].Code, got[0].Message, tc.want)
			}
		})
	}
}

func TestThemeFields_OK(t *testing.T) {
	fs := themeOf(t, `  radius: "0.5rem",
  light: { primary: "oklch(0.51 0.23 277)", primaryForeground: "oklch(0.98 0 0)" },
  dark: { primary: "oklch(0.68 0.16 277)", primaryForeground: "oklch(0.15 0 0)" },`)
	if got := ThemeFields(fs); len(got) != 0 {
		t.Errorf("got %v, want no diagnostics", got)
	}
}

// str is a text value built in memory, as the configurator builds them (no spans).
func str(s string) *syntax.DeployValue { return &syntax.DeployValue{Kind: "string", Str: s} }

func field(name string, v *syntax.DeployValue) *syntax.DeployField {
	return &syntax.DeployField{Name: name, Value: v}
}

// Fields built in memory: a value the printer would write back as other source is refused.
func TestThemeFields_InMemoryRaw(t *testing.T) {
	rec := func(fs ...*syntax.DeployField) *syntax.DeployValue {
		return &syntax.DeployValue{Kind: "record", Rec: fs}
	}
	cases := []struct {
		name string
		fs   []*syntax.DeployField
		want string
	}{
		{"colour", []*syntax.DeployField{field("light", rec(field("primary", str("#fff\n"))))},
			"`primary` has a line break or surrounding spaces: write the value alone, such as `\"#4f46e5\"`"},
		{"length", []*syntax.DeployField{field("radius", str(" 0.5rem\n"))},
			"`radius` has a line break or surrounding spaces: write the value alone, such as `\"0.5rem\"`"},
		{"shadow", []*syntax.DeployField{field("shadowSm", str("none\n"))},
			"`shadowSm` has a line break or surrounding spaces: write the value alone, such as `\"none\"`"},
		{"font element", []*syntax.DeployField{field("fontSans", &syntax.DeployValue{Kind: "list", List: []*syntax.DeployValue{str("Inter"), str("sans\tserif")}})},
			"`fontSans` has a line break or surrounding spaces: write the value alone, such as `[\"Inter\", \"sans-serif\"]`"},
		{"nil value", []*syntax.DeployField{field("radius", nil)}, "`radius` has no value"},
		{"nil value in dark", []*syntax.DeployField{field("dark", rec(field("primary", nil)))}, "`primary` has no value"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := ThemeFields(tc.fs)
			if len(got) != 1 {
				t.Fatalf("got %d diagnostics, want 1: %v", len(got), got)
			}
			if got[0].Code != "E0042" || got[0].Message != tc.want {
				t.Errorf("got %s %q\nwant E0042 %q", got[0].Code, got[0].Message, tc.want)
			}
		})
	}
	ok := []*syntax.DeployField{field("radius", str("0.5rem")), field("light", rec(field("primary", str("#fff"))))}
	if got := ThemeFields(ok); len(got) != 0 {
		t.Errorf("clean in-memory fields: got %v", got)
	}
}
