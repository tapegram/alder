package generate

import (
	"strings"
	"testing"
)

// TestNewRouteForm_VisitorAndToast (#160): a form on a route module whose GET route `requires
// user` writes a write route with the same guard; its tests assert the toast the update sends
// (`notices: ["Saved"]`) and that a visitor, the session ended on the page still open, is sent
// to log in with nothing written.
func TestNewRouteForm_VisitorAndToast(t *testing.T) {
	p, err := Load(fixture)
	if err != nil {
		t.Fatal(err)
	}
	var fs []FieldSpec
	for _, a := range []string{"title:Text.NonEmpty", "body:Text"} {
		f, err := ParseField(a, false)
		if err != nil {
			t.Fatal(err)
		}
		fs = append(fs, f)
	}
	edits, err := NewForm(p, FormOptions{Page: "Home", Name: "Quick", Fields: fs})
	if err != nil {
		t.Fatal(err)
	}
	e := editAt(edits, "app/Home.alder")
	if e == nil {
		t.Fatalf("no edit of Home: %+v", edits)
	}
	for _, want := range []string{
		`route SubmitQuick = POST "/_/SubmitQuick" { } form Quick requires user`,
		`Response.success "Saved"`,
		"notices: r.notices",
		`notices: ["Saved"]`,
		`test "a visitor cannot submit Quick" = {`,
		"let _ <- Test.login { user: Nothing }",
		"{ answer: r.answer, rows: rows } == { answer: Redirected (Auth.Login.Login {}), rows: 0 }",
	} {
		if !strings.Contains(e.Body, want) {
			t.Errorf("missing %q in:\n%s", want, e.Body)
		}
	}
	if _, err := Apply(fixture, edits, Options{DryRun: true}); err != nil {
		t.Fatalf("Apply: %v", err)
	}
}
