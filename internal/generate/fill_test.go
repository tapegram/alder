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

// fillApp is the fixture with Home.Submit.update's hole `?done` and the given extra modules.
func fillApp(t *testing.T, extra map[string]string) *Program {
	t.Helper()
	dir := t.TempDir()
	if err := copyApp(fixture, dir, nil); err != nil {
		t.Fatal(err)
	}
	home := filepath.Join(dir, "app", "Home.alder")
	b, _ := os.ReadFile(home)
	os.WriteFile(home, []byte(strings.Replace(string(b), "  Task.succeed (Response.redirect (Home {}))\n", "  ?done\n", 1)), 0o644)
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
	const expr = "Task.succeed (if Index.helper == 1 then Response.refresh else Response.refresh)"
	helper := func(m string) string { return "module " + m + "\n\nhelper : Int\nhelper = 1\n" }
	p := fillApp(t, map[string]string{"app/Todos/Index.alder": helper("Todos.Index")})
	edits, _, imports, err := Fill(p, "Home.Submit.update", "", expr)
	if err != nil || !slices.Equal(imports, []string{"Todos.Index"}) || len(edits) != 1 || !strings.Contains(edits[0].Body, "import Todos.Index\n") || !strings.Contains(edits[0].Body, "Index.helper") {
		t.Fatalf("unique: imports %v err %v edits %+v", imports, err, edits)
	}
	p = fillApp(t, map[string]string{"app/Todos/Index.alder": helper("Todos.Index"), "app/Reports/Index.alder": helper("Reports.Index")})
	_, _, _, err = Fill(p, "Home.Submit.update", "", expr)
	if err == nil || !strings.Contains(err.Error(), "`Reports.Index`, `Todos.Index`") || !strings.Contains(err.Error(), "`Index`") {
		t.Fatalf("ambiguous: %v", err)
	}
	// a qualifier no module would get is the check's to report
	p = fillApp(t, nil)
	if _, _, imports, err = Fill(p, "Home.Submit.update", "", expr); err != nil || len(imports) != 0 {
		t.Fatalf("none: imports %v err %v", imports, err)
	}
}

// A page is not a candidate for a short qualifier: `Show.x` is not an import of Teams.Show.
func TestFill_ShortQualifierSkipsPages(t *testing.T) {
	p := fillApp(t, map[string]string{"app/Teams/Show.alder": "module Teams.Show\n\npage \"/teams\" { }\n\ntype Model = { n: Int }\n\nload : Params -> Session -> Query Model\nload params session = Query.succeed { n: 1 }\n\nview : Model -> Session -> Html Msg\nview model session = <p></p>\n\nhelper : Int\nhelper = 1\n"})
	_, _, imports, err := Fill(p, "Home.Submit.update", "", "Task.succeed (if Show.helper == 1 then Response.refresh else Response.refresh)")
	if err != nil || len(imports) != 0 {
		t.Fatalf("imports %v err %v", imports, err)
	}
}

// twoRoutes is a route module with two write routes, each `update` a hole, and a GET route whose
// view is one: `Module.Route.member` names one route's member (#160).
const twoRoutes = `module Notes

route Notes = GET  "/notes"         { }
route Clear = POST "/notes/_/Clear" { }
route Pin   = POST "/notes/_/Pin"   { }

Notes.load : Notes.Params -> Session -> Query Int
Notes.load params session = Query.succeed 1

Notes.view : Int -> Session -> Html
Notes.view n session = ?view

Clear.update : Clear.Params -> Session -> Task Response
Clear.update params session = ?clear

Pin.update : Pin.Params -> Session -> Task Response
Pin.update params session = ?pin
`

// TestFill_RouteMember (#160): a hole in a route member is addressed `Module.Route.member`, which
// `alder fill --list` prints; the bare member (`Notes.update`) is refused when several routes
// have it, naming them, and taken when one does (`Notes.view`). --decl takes the same address.
func TestFill_RouteMember(t *testing.T) {
	p := fillApp(t, map[string]string{"app/Notes.alder": twoRoutes})
	var got []string
	for _, h := range HoleIndex(p.Prog, p.Diags) {
		if h.Module == "Notes" {
			got = append(got, h.Address())
		}
	}
	if want := []string{"Notes.Notes.view", "Notes.Clear.update", "Notes.Pin.update"}; !slices.Equal(got, want) {
		t.Errorf("addresses %q, want %q", got, want)
	}
	edits, h, _, err := Fill(p, "Notes.Pin.update", "", "Task.succeed Response.refresh")
	if err != nil || h.Name != "pin" || !strings.Contains(edits[0].Body, "Pin.update params session = (Task.succeed Response.refresh)\n") || !strings.Contains(edits[0].Body, "= ?clear\n") {
		t.Fatalf("Fill Notes.Pin.update: %v\n%v", err, edits)
	}
	if _, _, _, err := Fill(p, "Notes.update", "", "Task.succeed Response.refresh"); err == nil || !strings.Contains(err.Error(), "`Notes.Clear.update`, `Notes.Pin.update`") {
		t.Errorf("Fill Notes.update: %v, want the routes named", err)
	}
	if _, h, _, err := Fill(p, "Notes.view", "", "<p>n</p>"); err != nil || h.Name != "view" {
		t.Errorf("Fill Notes.view: %v", err)
	}
	edits, _, err = FillDecl(p, "Notes.Clear.update", "Task.succeed Response.refresh")
	if err != nil || !strings.Contains(edits[0].Body, "Clear.update params session = Task.succeed Response.refresh\n") || !strings.Contains(edits[0].Body, "= ?pin\n") {
		t.Fatalf("FillDecl Notes.Clear.update: %v\n%v", err, edits)
	}
}
