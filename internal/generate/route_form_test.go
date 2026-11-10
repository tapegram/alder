package generate

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"alder/internal/check"
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
		`route SubmitQuick = POST "/posts" { } form Quick requires user`,
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

// formsApp is the scaffold plus a table Reply (body) and a page Notes.Edit at /projects/{pid}/edit
// with a `pid: Int` parameter; extra is more declarations for Notes.Edit.
func formsApp(t *testing.T) (*Program, string) {
	t.Helper()
	page := func(mod, path, params string) string {
		return "module " + mod + "\n\nroute Page = GET \"" + path + "\" { " + params + " }\n\n" +
			"Page.load : Page.Params -> Session -> Query Int\nPage.load params session = Query.succeed 0\n\n" +
			"Page.view : Int -> Session -> Html\nPage.view n session = <al-page><h1>Page</h1></al-page>\n"
	}
	dir := scaffoldApp(t, map[string]string{
		"app/Table/Reply.alder":   "module Table.Reply\n\ntype ReplyId = brand Text\n\ntable Reply {\n  id: ReplyId @id,\n  body: Text.NonEmpty,\n}\n",
		"app/Table/Note.alder":    "module Table.Note\n\ntype NoteId = brand Text\n\ntable Note {\n  id: NoteId @id,\n  text: Text.NonEmpty,\n}\n",
		"app/Contact.alder":       page("Contact", "/contact", ""),
		"app/Projects/Edit.alder": page("Projects.Edit", "/projects/{pid}/edit", "pid: Int"),
		"app/Other.alder":         page("Other", "/other", ""),
	})
	p, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	return p, dir
}

func formFields(t *testing.T, args ...string) []FieldSpec {
	t.Helper()
	var fs []FieldSpec
	for _, a := range args {
		f, err := ParseField(a, false)
		if err != nil {
			t.Fatal(err)
		}
		fs = append(fs, f)
	}
	return fs
}

func TestRouteForm_IntoCollection(t *testing.T) {
	p, _ := formsApp(t)
	edits, err := NewForm(p, FormOptions{Page: "Home", Name: "Join", Msg: "SubmitEntry", Into: "Entry", Fields: formFields(t, "name:Text.NonEmpty", "message:Text.NonEmpty")})
	if err != nil {
		t.Fatal(err)
	}
	if b := msgBody(t, edits, "app/Home.alder"); !strings.Contains(b, `route SubmitEntry = POST "/entries" { } form Join`) {
		t.Errorf("got:\n%s", b)
	}
}

func TestRouteForm_InferredTableCollection(t *testing.T) {
	p, _ := formsApp(t)
	edits, err := NewForm(p, FormOptions{Page: "Contact", Name: "Say", Fields: formFields(t, "body:Text.NonEmpty")})
	if err != nil {
		t.Fatal(err)
	}
	if b := msgBody(t, edits, "app/Contact.alder"); !strings.Contains(b, `route SubmitSay = POST "/replies" { } form Say`) {
		t.Errorf("got:\n%s", b)
	}
}

func TestRouteForm_UnscopedFromNestedPage(t *testing.T) {
	p, dir := formsApp(t)
	edits, err := NewForm(p, FormOptions{Page: "Projects.Edit", Name: "Jot", Into: "Note", Fields: formFields(t, "text:Text.NonEmpty")})
	if err != nil {
		t.Fatal(err)
	}
	if b := msgBody(t, edits, "app/Projects/Edit.alder"); !strings.Contains(b, `route SubmitJot = POST "/notes" { pid: Int } form Jot`) {
		t.Errorf("got:\n%s", b)
	}
	if res, err := Apply(dir, edits, Options{DryRun: true}); err != nil {
		t.Fatalf("Apply: %v\n%s", err, res.RenderText(true))
	}
}

func TestRouteForm_NestedByPathKey(t *testing.T) {
	dir := t.TempDir()
	if err := os.CopyFS(dir, os.DirFS("../../testdata/generate/fixture-silo")); err != nil {
		t.Fatal(err)
	}
	page := "module Board\n\nimport Org\n\nroute Board = GET \"/a/{org}/b/{y}\" { org: Org.OrgId, y: Int } requires user, org\n\n" +
		"Board.load : Board.Params -> Session -> Query Int\nBoard.load params session = Query.succeed 0\n\n" +
		"Board.view : Int -> Session -> Html\nBoard.view n session = <al-page><h1>Board</h1></al-page>\n"
	if err := os.WriteFile(filepath.Join(dir, "app/Board.alder"), []byte(page), 0o644); err != nil {
		t.Fatal(err)
	}
	p, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	edits, err := NewForm(p, FormOptions{Page: "Board", Name: "Log", Into: "Activity", Fields: formFields(t, "text:Text.NonEmpty")})
	if err != nil {
		t.Fatal(err)
	}
	if b := msgBody(t, edits, "app/Board.alder"); !strings.Contains(b, `route SubmitLog = POST "/a/{org}/activities" { org: Org.OrgId, y: Int } form Log requires user, org`) {
		t.Errorf("got:\n%s", b)
	}
}

func TestRouteForm_NoTablePagePath(t *testing.T) {
	p, _ := formsApp(t)
	edits, err := NewForm(p, FormOptions{Page: "Contact", Name: "Message", Hole: true, Fields: formFields(t, "email:Text.Email")})
	if err != nil {
		t.Fatal(err)
	}
	if b := msgBody(t, edits, "app/Contact.alder"); !strings.Contains(b, `route SubmitMessage = POST "/contact" { } form Message`) {
		t.Errorf("got:\n%s", b)
	}
}

func TestRouteForm_PathAndMethodFlags(t *testing.T) {
	p, _ := formsApp(t)
	edits, err := NewForm(p, FormOptions{Page: "Contact", Name: "Message", Hole: true, Path: "/contact/messages", Method: "PUT", Fields: formFields(t, "email:Text.Email")})
	if err != nil {
		t.Fatal(err)
	}
	if b := msgBody(t, edits, "app/Contact.alder"); !strings.Contains(b, `route SubmitMessage = PUT "/contact/messages" { } form Message`) {
		t.Errorf("got:\n%s", b)
	}
	_, err = NewForm(p, FormOptions{Page: "Contact", Name: "Message", Hole: true, Path: "messages", Fields: formFields(t, "email:Text.Email")})
	wantRefusal(t, err, check.PagePathRule)
}

func TestRouteForm_TableClashRefused(t *testing.T) {
	p, dir := formsApp(t)
	edits, err := NewForm(p, FormOptions{Page: "Contact", Name: "Say", Fields: formFields(t, "body:Text.NonEmpty")})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Apply(dir, edits, Options{}); err != nil {
		t.Fatal(err)
	}
	p, err = Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	_, err = NewForm(p, FormOptions{Page: "Other", Name: "Reply2", Into: "Reply", Fields: formFields(t, "body:Text.NonEmpty")})
	// Contact.SubmitSay is another module's route: --msg cannot reach it, so the refusal names
	// its module instead
	wantRefusalIs(t, err, "routes `Contact.SubmitSay` and `SubmitReply2` both answer POST /replies; give the new route its own resource with `--path`; or, to send `Contact.SubmitSay` itself, write it into `Other`'s `view` by hand: it is a route of `Contact`")
}

// A clash with a route of the page that has a form already: --msg would be refused, so only
// --path is offered.
func TestRouteForm_ClashWithFormRouteOffersPathOnly(t *testing.T) {
	p, _ := formsApp(t)
	_, err := NewForm(p, FormOptions{Page: "Home", Name: "Comment", Hole: true, Fields: formFields(t, "body:Text.NonEmpty")})
	wantRefusalIs(t, err, "routes `Home.Submit` and `SubmitComment` both answer POST /; give the new route its own resource with `--path`")
}

// A clash with a form-less write route of the page: --msg attaches the form to it.
func TestRouteForm_ClashWithFormlessRouteOffersMsg(t *testing.T) {
	p, dir := formsApp(t)
	src := "\nroute Send = POST \"/contact\" { }\n\nSend.update : Send.Params -> Session -> Task Response\nSend.update params session = Task.succeed Response.refresh\n"
	f := filepath.Join(dir, "app/Contact.alder")
	b, _ := os.ReadFile(f)
	if err := os.WriteFile(f, append(b, src...), 0o644); err != nil {
		t.Fatal(err)
	}
	p, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	_, err = NewForm(p, FormOptions{Page: "Contact", Name: "Message", Hole: true, Fields: formFields(t, "email:Text.Email")})
	wantRefusalIs(t, err, "routes `Contact.Send` and `SubmitMessage` both answer POST /contact; give the new route its own resource with `--path`, or attach the form to `Contact.Send` with `--msg Send`")
}

func TestRouteForm_MsgAttachNoClash(t *testing.T) {
	p, dir := formsApp(t)
	// a form-less write route already answering POST /contact
	src := "\nroute Send = POST \"/contact\" { }\n\nSend.update : Send.Params -> Session -> Task Response\nSend.update params session = Task.succeed Response.refresh\n"
	f := filepath.Join(dir, "app/Contact.alder")
	b, _ := os.ReadFile(f)
	if err := os.WriteFile(f, append(b, src...), 0o644); err != nil {
		t.Fatal(err)
	}
	p, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	edits, err := NewForm(p, FormOptions{Page: "Contact", Name: "Message", Msg: "Send", Fields: formFields(t, "email:Text.Email")})
	if err != nil {
		t.Fatal(err)
	}
	if body := msgBody(t, edits, "app/Contact.alder"); strings.Count(body, "route Send") != 1 || !strings.Contains(body, "form Message") {
		t.Errorf("got:\n%s", body)
	}
}

// --msg attaching the form to a route that is there refuses --path and --method: the route has
// its own.
func TestRouteForm_MsgAttachRefusesPathAndMethod(t *testing.T) {
	p, dir := formsApp(t)
	src := "\nroute Send = POST \"/contact\" { }\n\nSend.update : Send.Params -> Session -> Task Response\nSend.update params session = Task.succeed Response.refresh\n"
	f := filepath.Join(dir, "app/Contact.alder")
	b, _ := os.ReadFile(f)
	if err := os.WriteFile(f, append(b, src...), 0o644); err != nil {
		t.Fatal(err)
	}
	p, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	want := "`--path` and `--method` belong to a new route; `Send` has its path already"
	for _, o := range []FormOptions{{Path: "/messages"}, {Method: "PUT"}} {
		o.Page, o.Name, o.Msg, o.Fields = "Contact", "Message", "Send", formFields(t, "email:Text.Email")
		_, err := NewForm(p, o)
		wantRefusalIs(t, err, want)
	}
}
