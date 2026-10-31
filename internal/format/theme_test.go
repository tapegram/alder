package format

import (
	"os"
	"strings"
	"testing"

	"alder/internal/syntax"
)

func themeFields(t *testing.T, src []byte) []*syntax.DeployField {
	t.Helper()
	m, diags := syntax.ParseFile("App.alder", src)
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

func TestTheme_BlogByteForByte(t *testing.T) {
	src, err := os.ReadFile("../../examples/blog/app/App.alder")
	if err != nil {
		t.Fatal(err)
	}
	want := strings.Join(strings.Split(string(src), "\n")[37:42], "\n")
	if got := Theme(themeFields(t, src)); got != want {
		t.Errorf("got\n%s\nwant\n%s", got, want)
	}
}

func TestTheme_Empty(t *testing.T) {
	if got := Theme(nil); got != "" {
		t.Errorf("got %q, want \"\"", got)
	}
}

func TestTheme_OneFieldOneLine(t *testing.T) {
	fs := themeFields(t, []byte("module App\n\ntheme {\n  radius: \"0.5rem\",\n}\n"))
	if got, want := Theme(fs), `theme { radius: "0.5rem" }`; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}
