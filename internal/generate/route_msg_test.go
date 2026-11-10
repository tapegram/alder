package generate

import (
	"errors"
	"strings"
	"testing"

	"alder/internal/check"
)

// repliesProgram is the scaffold plus a page `Replies.Show` at /replies/{id} with an `id: Int`
// parameter (the scaffold's Home is at "/"); extra is more declarations for the page.
func repliesProgram(t *testing.T, extra string) *Program {
	t.Helper()
	p, _ := repliesApp(t, extra)
	return p
}

func repliesApp(t *testing.T, extra string) (*Program, string) {
	t.Helper()
	page := "module Replies.Show\n\nroute Show = GET \"/replies/{id}\" { id: Int }\n" + extra +
		"\nShow.load : Show.Params -> Session -> Query Int\nShow.load params session = Query.succeed params.id\n\n" +
		"Show.view : Int -> Session -> Html\nShow.view n session = <al-page><h1>Reply</h1></al-page>\n"
	_ = "module Root\n\nroute Root = GET \"/\" { }\n\nRoot.load : Root.Params -> Session -> Query Int\nRoot.load params session = Query.succeed 0\n\n" +
		"Root.view : Int -> Session -> Html\nRoot.view n session = <al-page><h1>Root</h1></al-page>\n"
	dir := scaffoldApp(t, map[string]string{"app/Replies/Show.alder": page})
	p, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	return p, dir
}

func msgBody(t *testing.T, edits []Edit, path string) string {
	t.Helper()
	e := editAt(edits, path)
	if e == nil {
		t.Fatalf("no edit of %s: %+v", path, edits)
	}
	return e.Body
}

func wantRefusal(t *testing.T, err error, want string) {
	t.Helper()
	var r *Refusal
	if !errors.As(err, &r) || !strings.Contains(r.Msg, want) {
		t.Fatalf("err = %v, want a refusal containing %q", err, want)
	}
}

func TestRouteMsg_PagePath(t *testing.T) {
	p := repliesProgram(t, "")
	edits, err := NewMsg(p, "Replies.Show", "Delete", nil, MsgOptions{Method: "DELETE"})
	if err != nil {
		t.Fatal(err)
	}
	if b := msgBody(t, edits, "app/Replies/Show.alder"); !strings.Contains(b, `route Delete = DELETE "/replies/{id}" { id: Int }`) {
		t.Errorf("got:\n%s", b)
	}
}

func TestRouteMsg_DefaultPost(t *testing.T) {
	edits, err := NewMsg(repliesProgram(t, ""), "Replies.Show", "Like", nil, MsgOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if b := msgBody(t, edits, "app/Replies/Show.alder"); !strings.Contains(b, `route Like = POST "/replies/{id}" { id: Int }`) {
		t.Errorf("got:\n%s", b)
	}
}

func TestRouteMsg_RootPagePath(t *testing.T) {
	edits, err := NewMsg(repliesProgram(t, ""), "Home", "Like", nil, MsgOptions{Method: "DELETE"})
	if err != nil {
		t.Fatal(err)
	}
	// the scaffold's Home already answers POST /, so the root path is pinned with another method
	if b := msgBody(t, edits, "app/Home.alder"); !strings.Contains(b, `route Like = DELETE "/" { }`) {
		t.Errorf("got:\n%s", b)
	}
}

func TestRouteMsg_ExplicitPath(t *testing.T) {
	p, dir := repliesApp(t, "")
	edits, err := NewMsg(p, "Replies.Show", "Pin", nil, MsgOptions{Method: "DELETE", Path: "/replies/{id}/pin"})
	if err != nil {
		t.Fatal(err)
	}
	if b := msgBody(t, edits, "app/Replies/Show.alder"); !strings.Contains(b, `route Pin = DELETE "/replies/{id}/pin" { id: Int }`) {
		t.Errorf("got:\n%s", b)
	}
	// a {param} that is neither a page path parameter nor a field is the checker's to refuse
	edits, err = NewMsg(p, "Replies.Show", "Pin", nil, MsgOptions{Path: "/replies/{rid}/pin"})
	if err == nil {
		_, err = Apply(dir, edits, Options{DryRun: true})
	}
	var r *Refusal
	if !errors.As(err, &r) || r.Result == nil || !strings.Contains(r.Result.RenderText(true), "has no matching parameter") {
		t.Errorf("err = %v\n%s", err, r.Result.RenderText(true))
	}
}

func TestRouteMsg_BadPath(t *testing.T) {
	_, err := NewMsg(repliesProgram(t, ""), "Replies.Show", "Pin", nil, MsgOptions{Path: "pin"})
	wantRefusal(t, err, check.PagePathRule)
}

func TestRouteMsg_SecondOnPageClashes(t *testing.T) {
	like := "\nroute Like = POST \"/replies/{id}\" { id: Int }\n\nLike.update : Like.Params -> Session -> Task Response\nLike.update p session = Task.succeed Response.refresh\n"
	p := repliesProgram(t, like)
	_, err := NewMsg(p, "Replies.Show", "Archive", nil, MsgOptions{})
	have := routeClash(p, "POST", "/replies/{id}", "")
	if have == nil {
		t.Fatal("fixture has no Like route")
	}
	want := clashRefusal(have, "Archive", "POST", "/replies/{id}")
	var r *Refusal
	if !errors.As(err, &r) || r.Msg != want.Error() {
		t.Fatalf("err = %v, want %v", err, want)
	}
}

func TestAppMsg_PathRequired(t *testing.T) {
	_, err := NewMsg(repliesProgram(t, ""), "App", "Ping", nil, MsgOptions{})
	wantRefusal(t, err, "`alder new msg App` needs `--path`: a route of App has no page to take a path from, such as `--path /theme`")
}

func TestAppMsg_Path(t *testing.T) {
	edits, err := NewMsg(repliesProgram(t, ""), "App", "ToggleTheme", nil, MsgOptions{Path: "/theme", Method: "PUT"})
	if err != nil {
		t.Fatal(err)
	}
	if b := msgBody(t, edits, "app/App.alder"); !strings.Contains(b, `route ToggleTheme = PUT "/theme" { }`) {
		t.Errorf("got:\n%s", b)
	}
}

func TestAppMsg_BadPath(t *testing.T) {
	_, err := NewMsg(repliesProgram(t, ""), "App", "ToggleTheme", nil, MsgOptions{Path: "theme"})
	wantRefusal(t, err, check.PagePathRule)
}
