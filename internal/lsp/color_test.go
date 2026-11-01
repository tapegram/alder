package lsp

import (
	"math"
	"strings"
	"testing"

	"alder/internal/check"
	"alder/internal/diag"
	"alder/internal/names"
	"alder/internal/theme"
)

// documentColors asks for rel's colours.
func documentColors(t *testing.T, c *client, rel string) []ColorInformation {
	t.Helper()
	var out []ColorInformation
	if e := c.call("textDocument/documentColor", map[string]any{"textDocument": map[string]any{"uri": c.uri(rel)}}, &out); e != nil {
		t.Fatalf("documentColor: %v", e)
	}
	return out
}

// withTheme opens the blog's App with its theme replaced by decl (a whole `theme { … }`, or "" for none).
func withTheme(t *testing.T, c *client, decl string) {
	t.Helper()
	text := c.disk("app/App.alder")
	i := strings.Index(text, "theme {")
	j := i + strings.Index(text[i:], "\n}\n") + 3
	c.open("app/App.alder", text[:i]+decl+text[j:])
	c.diags("app/App.alder")
}

func TestInitialize_ColorProvider(t *testing.T) {
	c := start(t, blogCopy(t))
	if caps, _ := c.initResult["capabilities"].(map[string]any); caps["colorProvider"] != true {
		t.Fatalf("capabilities %v", caps)
	}
}

// TestDocumentColor_Blog: one swatch per colour literal of the blog's theme, each over the
// literal's contents, in the client's unit (a line with é and an astral character before one).
func TestDocumentColor_Blog(t *testing.T) {
	for _, e := range encodings {
		t.Run(e.name, func(t *testing.T) {
			c := startOffering(t, blogCopy(t), Options{}, e.offered)
			text := c.disk("app/App.alder")
			const odd = `light: { primary: "oklch(0.51 0.23 277)"`
			c.open("app/App.alder", strings.Replace(text, odd, `light: { foreground: "é😀", primary: "oklch(0.51 0.23 277)"`, 1))
			c.diags("app/App.alder")
			got := documentColors(t, c, "app/App.alder")
			lits := []string{"oklch(0.51 0.23 277)", "oklch(0.98 0 0)", "oklch(0.68 0.16 277)", "oklch(0.15 0 0)"}
			if len(got) != len(lits) {
				t.Fatalf("got %d colours, want %d: %+v", len(got), len(lits), got)
			}
			lines := strings.Split(c.texts["app/App.alder"], "\n")
			for k, lit := range lits {
				ln := 0
				for ln < len(lines) && !strings.Contains(lines[ln], `"`+lit+`"`) {
					ln++
				}
				start := atCol(c, e.enc, "app/App.alder", ln, `"`+lit+`"`, 1)
				want := Range{Start: start, End: Position{ln, start.Character + len(lit)}}
				if got[k].Range != want {
					t.Errorf("%s: range %+v, want %+v", lit, got[k].Range, want)
				}
				if k == 0 && e.enc == utf16Enc && start.Character == strings.Index(lines[ln], lit) {
					t.Fatal("the test line does not tell the encodings apart")
				}
			}
		})
	}
}

func near(a, b float64) bool { return math.Abs(a-b) < 0.01 }

func TestDocumentColor_Values(t *testing.T) {
	c := start(t, blogCopy(t))
	withTheme(t, c, `theme {
  light: { primary: "#ff0000", destructive: "oklch(0.577 0.245 27.325)" },
  dark: { primary: "#00ff00", ring: "var(--primary)", border: "var(--muted)" },
}
`)
	got := documentColors(t, c, "app/App.alder")
	mutedDark, _ := theme.ParseColour(theme.Defaults()["--muted"].Dark)
	want := []Color{{1, 0, 0, 1}, {0.906, 0, 0.043, 1}, {0, 1, 0, 1}, {0, 1, 0, 1}, {mutedDark.R, mutedDark.G, mutedDark.B, mutedDark.A}}
	if len(got) != len(want) {
		t.Fatalf("got %+v", got)
	}
	for k, w := range want {
		g := got[k].Color
		if !near(g.Red, w.Red) || !near(g.Green, w.Green) || !near(g.Blue, w.Blue) || !near(g.Alpha, w.Alpha) {
			t.Errorf("colour %d: %+v, want %+v", k, g, w)
		}
	}
}

func TestDocumentColor_VarDefault(t *testing.T) {
	c := start(t, blogCopy(t))
	withTheme(t, c, `theme {
  dark: { ring: "var(--primary)" },
}
`)
	got := documentColors(t, c, "app/App.alder")
	d, ok := theme.ParseColour(theme.Defaults()["--primary"].Dark)
	if !ok || len(got) != 1 {
		t.Fatalf("got %+v", got)
	}
	if g := got[0].Color; !near(g.Red, d.R) || !near(g.Green, d.G) || !near(g.Blue, d.B) {
		t.Fatalf("ring %+v, want the default dark primary %+v", g, d)
	}
}

// TestDocumentColor_VarFallback: `var(--x, fallback)` is --x when it resolves, else the fallback.
func TestDocumentColor_VarFallback(t *testing.T) {
	c := start(t, blogCopy(t))
	withTheme(t, c, `theme {
  light: { primary: "#ff0000", ring: "var(--nope, #00ff00)", border: "var(--primary, #0000ff)", accent: "var(--accent, #0000ff)" },
}
`)
	got := documentColors(t, c, "app/App.alder")
	want := []Color{{1, 0, 0, 1}, {0, 1, 0, 1}, {1, 0, 0, 1}, {0, 0, 1, 1}}
	if len(got) != len(want) {
		t.Fatalf("got %+v", got)
	}
	for k, w := range want {
		if got[k].Color != w {
			t.Errorf("colour %d: %+v, want %+v", k, got[k].Color, w)
		}
	}
}

// TestDocumentColor_BeforeAnalysis: a request while the first build runs still finds App by its path.
func TestDocumentColor_BeforeAnalysis(t *testing.T) {
	release := make(chan struct{})
	real := loadCheckedOverlay
	loadCheckedOverlay = func(root string, over map[string][]byte) (*names.Program, *check.Result, *diag.Bag, []error) {
		<-release
		return real(root, over)
	}
	t.Cleanup(func() { loadCheckedOverlay = real })
	c := start(t, blogCopy(t))
	defer close(release) // before the cleanups, so the server can stop
	c.open("app/App.alder", c.disk("app/App.alder"))
	if got := documentColors(t, c, "app/App.alder"); len(got) != 4 {
		t.Fatalf("before the first analysis: %+v", got)
	}
	c.open("app/Layout.alder", c.disk("app/Layout.alder"))
	if got := documentColors(t, c, "app/Layout.alder"); got == nil || len(got) != 0 {
		t.Fatalf("Layout before the first analysis: %+v", got)
	}
}

func TestDocumentColor_Skips(t *testing.T) {
	c := start(t, blogCopy(t))
	withTheme(t, c, `theme {
  radius: "0.5rem",
  fontSans: "Inter, sans-serif",
  light: { primary: "not a colour", ring: "var(--ring)", accent: "var(--border)", border: "var(--accent)", radius: "#ff0000" },
}
`)
	if got := documentColors(t, c, "app/App.alder"); got == nil || len(got) != 0 {
		t.Fatalf("radius, fontSans, an unparsable colour and var cycles: %+v", got)
	}

	c.open("app/Layout.alder", c.disk("app/Layout.alder"))
	if got := documentColors(t, c, "app/Layout.alder"); got == nil || len(got) != 0 {
		t.Fatalf("a module other than App: %+v", got)
	}

	withTheme(t, c, "")
	if got := documentColors(t, c, "app/App.alder"); got == nil || len(got) != 0 {
		t.Fatalf("no theme: %+v", got)
	}

	withTheme(t, c, "theme {\n  light: { primary: \"#ff0000\" },\n}\n\nnope nope (\n")
	if got := documentColors(t, c, "app/App.alder"); got == nil || len(got) != 0 {
		t.Fatalf("a syntax error: %+v", got)
	}
}

func TestColorPresentation(t *testing.T) {
	c := start(t, blogCopy(t))
	rg := Range{Start: Position{39, 21}, End: Position{39, 41}}
	for _, tc := range []struct {
		col  Color
		want string
	}{
		{Color{0, 0.5, 1, 1}, "#0080ff"},
		{Color{0, 0.5, 1, 0.5}, "#0080ff80"},
	} {
		var got []ColorPresentation
		params := map[string]any{"textDocument": map[string]any{"uri": c.uri("app/App.alder")}, "color": tc.col, "range": rg}
		if e := c.call("textDocument/colorPresentation", params, &got); e != nil {
			t.Fatalf("colorPresentation: %v", e)
		}
		if len(got) != 1 || got[0].Label != tc.want || got[0].TextEdit == nil || *got[0].TextEdit != (TextEdit{Range: rg, NewText: tc.want}) {
			t.Errorf("%+v: %+v, want %s", tc.col, got, tc.want)
		}
	}
}
