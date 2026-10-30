package generate

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

// A multi-byte character in markup text must not stall the qualifier scan (E11 #23).
func TestQualifiersMultiByte(t *testing.T) {
	done := make(chan struct{})
	go func() {
		for _, e := range []string{
			"<p>café · 3 × 4</p>",
			"<Layout.Page>Été Member.x · ×</Layout.Page>",
			"Ã©",
		} {
			qualifiers(&Program{}, e)
		}
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("qualifiers does not return on a multi-byte character")
	}
}

// fillApp is the fixture with Home.update.Submit's hole `?done` and the given extra modules.
func fillApp(t *testing.T, extra map[string]string) *Program {
	t.Helper()
	dir := t.TempDir()
	if err := copyApp(fixture, dir, nil); err != nil {
		t.Fatal(err)
	}
	home := filepath.Join(dir, "app", "Home.alder")
	b, _ := os.ReadFile(home)
	os.WriteFile(home, []byte(strings.Replace(string(b), "    Task.succeed (Redirect (Home {}))\n", "    ?done\n", 1)), 0o644)
	for rel, src := range extra {
		if err := os.MkdirAll(filepath.Dir(filepath.Join(dir, rel)), 0o755); err != nil {
			t.Fatal(err)
		}
		os.WriteFile(filepath.Join(dir, rel), []byte(src), 0o644)
	}
	p, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	return p
}

// A short qualifier of a module not yet imported is resolved when exactly one module of the app
// would get it, and refused, naming the candidates, when several would.
func TestFill_ShortQualifier(t *testing.T) {
	const expr = "Task.succeed (if Index.helper == 1 then Rerender else Rerender)"
	helper := func(m string) string { return "module " + m + "\n\nhelper : Int\nhelper = 1\n" }
	p := fillApp(t, map[string]string{"app/Todos/Index.alder": helper("Todos.Index")})
	edits, _, imports, err := Fill(p, "Home.update.Submit", "", expr)
	if err != nil || !slices.Equal(imports, []string{"Todos.Index"}) || len(edits) != 1 || !strings.Contains(edits[0].Body, "import Todos.Index\n") || !strings.Contains(edits[0].Body, "Index.helper") {
		t.Fatalf("unique: imports %v err %v edits %+v", imports, err, edits)
	}
	p = fillApp(t, map[string]string{"app/Todos/Index.alder": helper("Todos.Index"), "app/Reports/Index.alder": helper("Reports.Index")})
	_, _, _, err = Fill(p, "Home.update.Submit", "", expr)
	if err == nil || !strings.Contains(err.Error(), "`Reports.Index`, `Todos.Index`") || !strings.Contains(err.Error(), "`Index`") {
		t.Fatalf("ambiguous: %v", err)
	}
	// a qualifier no module would get is the check's to report
	p = fillApp(t, nil)
	if _, _, imports, err = Fill(p, "Home.update.Submit", "", expr); err != nil || len(imports) != 0 {
		t.Fatalf("none: imports %v err %v", imports, err)
	}
}

// A page is not a candidate for a short qualifier: `Show.x` is not an import of Teams.Show.
func TestFill_ShortQualifierSkipsPages(t *testing.T) {
	p := fillApp(t, map[string]string{"app/Teams/Show.alder": "module Teams.Show\n\npage \"/teams\" { }\n\ntype Model = { n: Int }\n\nload : Params -> Session -> Query Model\nload params session = Query.succeed { n: 1 }\n\nview : Model -> Session -> Html Msg\nview model session = <p></p>\n\nhelper : Int\nhelper = 1\n"})
	_, _, imports, err := Fill(p, "Home.update.Submit", "", "Task.succeed (if Show.helper == 1 then Rerender else Rerender)")
	if err != nil || len(imports) != 0 {
		t.Fatalf("imports %v err %v", imports, err)
	}
}
