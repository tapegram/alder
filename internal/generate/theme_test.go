package generate

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"alder/internal/syntax"
	"alder/internal/theme"
)

const appFile = "app/App.alder"

// blogCopy is a copy of examples/blog in a temporary directory.
func blogCopy(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := copyApp("../../examples/blog", dir, nil); err != nil {
		t.Fatal(err)
	}
	return dir
}

// themeEdits loads the app at dir (errors allowed) and runs ThemeEdit.
func themeEdits(t *testing.T, dir string, th theme.Theme) ([]Edit, error) {
	t.Helper()
	p, err := LoadWithErrors(dir)
	if err != nil {
		t.Fatal(err)
	}
	return ThemeEdit(p, th)
}

func readApp(t *testing.T, dir string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(appFile)))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// writeTheme writes th into the app at dir and returns App.alder after the write.
func writeTheme(t *testing.T, dir string, th theme.Theme) string {
	t.Helper()
	res, err := WriteTheme(App{Dir: dir}, th, false)
	if err != nil {
		t.Fatal(err)
	}
	if !res.Written {
		t.Fatalf("not written: %v", res.Diagnostics)
	}
	return readApp(t, dir)
}

var radiusTheme = theme.Theme{Root: map[string]string{"radius": "0.5rem"}}

const radiusPrint = "theme { radius: \"0.5rem\" }"

// lineDiff is the lines of a and b that differ, a and b of the same length; ok is false when
// the lengths differ.
func lineDiff(a, b string) (diff []string, ok bool) {
	al, bl := strings.Split(a, "\n"), strings.Split(b, "\n")
	if len(al) != len(bl) {
		return nil, false
	}
	for i := range al {
		if al[i] != bl[i] {
			diff = append(diff, al[i]+" → "+bl[i])
		}
	}
	return diff, true
}

func TestThemeEdit_InsertAfterSession(t *testing.T) {
	dir := scaffoldApp(t, nil)
	edits, err := themeEdits(t, dir, radiusTheme)
	if err != nil {
		t.Fatal(err)
	}
	e := editAt(edits, appFile)
	if e == nil || len(edits) != 1 {
		t.Fatalf("edits = %+v", edits)
	}
	want := "session { }\n\n" + radiusPrint + "\n\nerror : Error -> Html Never\n"
	if !strings.Contains(e.Body, want) {
		t.Fatalf("body:\n%s\nwant it to contain:\n%s", e.Body, want)
	}
	if got := writeTheme(t, dir, radiusTheme); !strings.Contains(got, want) {
		t.Fatalf("written:\n%s", got)
	}
}

// insertCase writes App as src (with a Layout page so that the scaffold's other modules check)
// and returns App.alder after writing radiusTheme.
func insertCase(t *testing.T, src string) string {
	t.Helper()
	dir := scaffoldApp(t, map[string]string{appFile: src})
	return writeTheme(t, dir, radiusTheme)
}

func TestThemeEdit_InsertAfterDeploy(t *testing.T) {
	src := "module App\n\nimport Layout\n\nsession { }\n\ndeploy { local: Script \"./d.sh\" }\n\nnotFound : Html Never\nnotFound = <Layout.Page title=\"Not found\"><h1>Not found</h1></Layout.Page>\n"
	got := insertCase(t, src)
	want := "deploy { local: Script \"./d.sh\" }\n\n" + radiusPrint + "\n\nnotFound : Html Never\n"
	if !strings.Contains(got, want) {
		t.Fatalf("written:\n%s", got)
	}
}

func TestThemeEdit_InsertAfterImports(t *testing.T) {
	src := "module App\n\nimport Layout\n\nnotFound : Html Never\nnotFound = <Layout.Page title=\"Not found\"><h1>Not found</h1></Layout.Page>\n"
	got := insertCase(t, src)
	want := "module App\n\nimport Layout\n\n" + radiusPrint + "\n\nnotFound : Html Never\n"
	if !strings.HasPrefix(got, want) {
		t.Fatalf("written:\n%s", got)
	}
}

func TestThemeEdit_InsertAfterModule(t *testing.T) {
	src := "module App\n\nnotFound : Html Never\nnotFound = <h1>Not found</h1>\n"
	got := insertCase(t, src)
	want := "module App\n\n" + radiusPrint + "\n\nnotFound : Html Never\n"
	if !strings.HasPrefix(got, want) {
		t.Fatalf("written:\n%s", got)
	}
}

// blogTheme is the blog's theme with light's primary changed.
func blogTheme(t *testing.T, dir string) theme.Theme {
	t.Helper()
	p, err := LoadWithErrors(dir)
	if err != nil {
		t.Fatal(err)
	}
	var d *syntax.ThemeDecl
	for _, x := range p.Modules["App"].Syntax.Decls {
		if td, ok := x.(*syntax.ThemeDecl); ok {
			d = td
		}
	}
	if d == nil {
		t.Fatal("the blog has no theme")
	}
	th := ThemeOf(d)
	if th.Light["primary"] != "oklch(0.51 0.23 277)" || th.Root["radius"] != "0.5rem" {
		t.Fatalf("ThemeOf = %+v", th)
	}
	th.Light["primary"] = "oklch(0.6 0.2 30)"
	return th
}

func TestThemeEdit_ReplaceOnlyDeclLines(t *testing.T) {
	dir := blogCopy(t)
	before := readApp(t, dir)
	after := writeTheme(t, dir, blogTheme(t, dir))
	diff, ok := lineDiff(before, after)
	if !ok || len(diff) != 1 || !strings.Contains(diff[0], `primary: "oklch(0.6 0.2 30)"`) {
		t.Fatalf("diff = %q (same length %v)\nafter:\n%s", diff, ok, after)
	}
}

// TestThemeEdit_ReplaceKeepsLeadingComment (Review Focus 1): the comment above the declaration,
// and every line outside its span, come through byte-identical.
func TestThemeEdit_ReplaceKeepsLeadingComment(t *testing.T) {
	dir := blogCopy(t)
	before := readApp(t, dir)
	edits, err := themeEdits(t, dir, blogTheme(t, dir))
	if err != nil {
		t.Fatal(err)
	}
	body := editAt(edits, appFile).Body
	bl, al := strings.Split(before, "\n"), strings.Split(body, "\n")
	for i := 0; i < 37; i++ { // lines 1–37: everything above `theme`, its comment among them
		if bl[i] != al[i] {
			t.Fatalf("line %d: %q → %q", i+1, bl[i], al[i])
		}
	}
	if !strings.HasPrefix(bl[35], "-- The look") || al[37] != "theme {" {
		t.Fatalf("lines 36, 38: %q, %q", al[35], al[37])
	}
}

func TestThemeEdit_TokensOrder(t *testing.T) {
	th := theme.Theme{
		Root:  map[string]string{"fontSans": " Inter , 'Noto Sans' ", "radius": "1rem"},
		Light: map[string]string{"ring": "oklch(0.5 0.1 200)", "primary": "oklch(0.5 0.2 277)"},
		Dark:  map[string]string{"primary": "oklch(0.7 0.2 277)"},
	}
	got, err := ThemeText(th)
	if err != nil {
		t.Fatal(err)
	}
	want := `theme {
  radius: "1rem",
  fontSans: ["Inter", "Noto Sans"],
  light: { primary: "oklch(0.5 0.2 277)", ring: "oklch(0.5 0.1 200)" },
  dark: { primary: "oklch(0.7 0.2 277)" },
}`
	if got != want {
		t.Fatalf("got:\n%s\nwant:\n%s", got, want)
	}
	again, _ := ThemeText(th)
	if again != got {
		t.Fatal("two prints of one theme differ")
	}
	if s, err := ThemeText(theme.Theme{}); s != "" || err != nil {
		t.Fatalf("empty: %q, %v", s, err)
	}
}

func TestThemeEdit_EmptyDeletes(t *testing.T) {
	dir := blogCopy(t)
	before := strings.Split(readApp(t, dir), "\n")
	after := writeTheme(t, dir, theme.Theme{})
	// lines 38–42 go; with no blank line after them (the end of the file), none before the
	// declaration either (the comment's line 37 is right above it): the comment stays
	want := strings.Join(before[:37], "\n") + "\n"
	if after != want {
		t.Fatalf("after:\n%s\nwant:\n%s", after, want)
	}

	// a blank line after the declaration goes with it
	src := "module App\n\nimport Layout\n\nsession { }\n\n" + radiusPrint + "\n\nnotFound : Html Never\nnotFound = <Layout.Page title=\"Not found\"><h1>Not found</h1></Layout.Page>\n"
	dir = scaffoldApp(t, map[string]string{appFile: src})
	after = writeTheme(t, dir, theme.Theme{})
	want = "module App\n\nimport Layout\n\nsession { }\n\nnotFound : Html Never\n"
	if !strings.HasPrefix(after, want) {
		t.Fatalf("after:\n%s", after)
	}

	edits, err := themeEdits(t, scaffoldApp(t, nil), theme.Theme{})
	if err != nil || len(edits) != 0 {
		t.Fatalf("scaffold + empty: %+v, %v", edits, err)
	}
}

func TestThemeEdit_Refused(t *testing.T) {
	for _, th := range []theme.Theme{
		{Light: map[string]string{"primary": "a\"b"}},
		{Light: map[string]string{"primary": "red\n}"}},
		{Root: map[string]string{"primary": "red"}},
		{Root: map[string]string{"radius": "-1px"}},
		{Light: map[string]string{"primary": "var(--nope)"}},
		{Root: map[string]string{"bogus": "1"}},
		{Root: map[string]string{"fontSans": " , "}}, // no family
	} {
		for _, dir := range []string{blogCopy(t), scaffoldApp(t, nil)} {
			before := readApp(t, dir)
			_, err := WriteTheme(App{Dir: dir}, th, false)
			var inv *ThemeInvalid
			if !errors.As(err, &inv) || len(inv.Diags) == 0 {
				t.Fatalf("%+v: err = %v", th, err)
			}
			for _, d := range inv.Diags {
				if d.Code != "E0042" || d.Span.File != appFile || d.Span.Start.Line == 0 {
					t.Fatalf("%+v: diagnostic %+v", th, d)
				}
			}
			if err.Error() != inv.Diags[0].Message {
				t.Fatalf("Error() = %q", err.Error())
			}
			if readApp(t, dir) != before {
				t.Fatalf("%+v: App.alder changed", th)
			}
		}
	}
	// on the blog, located on its declaration
	_, err := themeEdits(t, blogCopy(t), theme.Theme{Root: map[string]string{"radius": "-1px"}})
	var inv *ThemeInvalid
	if !errors.As(err, &inv) || inv.Diags[0].Span.Start.Line != 38 || inv.Diags[0].Span.End.Line != 42 {
		t.Fatalf("err = %v", err)
	}
	// several: the first message and how many more
	_, err = ThemeText(theme.Theme{Root: map[string]string{"radius": "-1px", "radiusSm": "-2px"}})
	if !errors.As(err, &inv) || len(inv.Diags) != 2 || !strings.HasSuffix(err.Error(), "; +1 more") {
		t.Fatalf("err = %v", err)
	}
}

func TestWriteTheme_DespiteUnrelatedError(t *testing.T) {
	dir := scaffoldApp(t, map[string]string{"app/Broken.alder": "module Broken\n\nx : Int\nx = \"not an int\"\n"})
	res, err := WriteTheme(App{Dir: dir}, radiusTheme, false)
	if err != nil {
		t.Fatal(err)
	}
	if !res.Written || !strings.Contains(readApp(t, dir), radiusPrint) {
		t.Fatalf("written %v, diagnostics %v", res.Written, res.Diagnostics)
	}
}

func TestWriteTheme_DryRun(t *testing.T) {
	dir := scaffoldApp(t, nil)
	before := readApp(t, dir)
	res, err := WriteTheme(App{Dir: dir}, radiusTheme, true)
	if err != nil || res.Written || !strings.Contains(res.Diff, radiusPrint) {
		t.Fatalf("%+v, %v", res, err)
	}
	if readApp(t, dir) != before {
		t.Fatal("dry run wrote")
	}
}

func TestThemeEdit_ParseErrorRefused(t *testing.T) {
	dir := scaffoldApp(t, map[string]string{appFile: "module App\n\nsession { \n"})
	before := readApp(t, dir)
	_, err := WriteTheme(App{Dir: dir}, radiusTheme, false)
	var r *Refusal
	if !errors.As(err, &r) || len(r.Diags) == 0 {
		t.Fatalf("err = %v", err)
	}
	if readApp(t, dir) != before {
		t.Fatal("App.alder changed")
	}
}

func TestThemeEdit_GuardSet(t *testing.T) {
	edits, err := themeEdits(t, scaffoldApp(t, nil), radiusTheme)
	if err != nil {
		t.Fatal(err)
	}
	g := editAt(edits, appFile).Guard
	if g == nil || !g(&syntax.ThemeDecl{}) || g(&syntax.SessionDecl{}) {
		t.Fatal("a theme's edit is guarded by its declaration")
	}
	edits, err = themeEdits(t, blogCopy(t), theme.Theme{})
	if err != nil || len(edits) != 1 || edits[0].Guard != nil {
		t.Fatalf("the delete: %+v, %v", edits, err)
	}
}
