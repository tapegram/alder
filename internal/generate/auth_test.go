package generate

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"alder/internal/scaffold"
)

// scaffoldApp writes the scaffold into a temporary directory, with files (path → text) written
// over it, and returns the directory.
func scaffoldApp(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for _, f := range scaffold.Files("app", false) {
		if err := writeFile(filepath.Join(dir, filepath.FromSlash(f.Path)), string(f.Data)); err != nil {
			t.Fatal(err)
		}
	}
	for p, s := range files {
		if err := writeFile(filepath.Join(dir, filepath.FromSlash(p)), s); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

// appWithUpdate is an App with imports, a session with another field, an `update` with an arm
// and a comment between declarations: what `alder new auth` must keep (RF1).
const appWithUpdate = `module App

import Layout
import Theme

-- the theme is the visitor's choice
session { theme: Theme.Theme = Light }

msg Toggle

-- Toggle switches the theme.
update : Msg -> Session -> Task Nav
update msg session = match msg {
  Toggle p -> {
    let _ <- Session.put (\s -> { s | theme: Theme.flip s.theme })
    Task.succeed Rerender
  },
}

error : Error -> Html Never
error err =
  <Layout.Page title="Error">
    <h1>Something went wrong</h1>
  </Layout.Page>

notFound : Html Never
notFound =
  <Layout.Page title="Not found">
    <h1>Not found</h1>
  </Layout.Page>
`

const themeModule = `module Theme

type Theme = Light | Dark

flip : Theme -> Theme
flip t = match t {
  Light -> Dark,
  Dark -> Light,
}
`

// TestNewAuth_App (Review Focus 1): on an App with an `update`, its arm stays and Logout is
// added as a new arm before its closing brace; the session keeps its other field and gains
// `user`; `login` follows the session; the imports stay sorted; nothing else moves. On the
// scaffold's App (no `update`), one is created after `login`. Both apps check after it.
func TestNewAuth_App(t *testing.T) {
	dir := scaffoldApp(t, map[string]string{"app/App.alder": appWithUpdate, "app/Theme.alder": themeModule})
	p, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	edits, created, err := NewAuth(p, AuthOptions{})
	if err != nil || !created {
		t.Fatalf("NewAuth: %v (created %v)", err, created)
	}
	var app string
	for _, e := range edits {
		if e.Path == "app/App.alder" {
			app = e.Body
		}
	}
	want := `module App

import Layout
import Table.User
import Theme

-- the theme is the visitor's choice
session { theme: Theme.Theme = Light, user: Maybe User.UserId }

login = Auth.Login

msg Toggle

msg Logout

-- Toggle switches the theme.
update : Msg -> Session -> Task Nav
update msg session = match msg {
  Toggle p -> {
    let _ <- Session.put (\s -> { s | theme: Theme.flip s.theme })
    Task.succeed Rerender
  },
  Logout p -> {
    let _ <- Session.put (\s -> { s | user: Nothing })
    Task.succeed (Redirect (Auth.Login {}))
  },
}
`
	if !strings.HasPrefix(app, want) || !strings.Contains(app, "\nnotFound : Html Never\n") ||
		!strings.HasSuffix(app, "  Task.succeed ({ value: r.value, trace: r.trace } == { value: Ok (Redirect (Auth.Login {})), trace: [\"Session.put\"] })\n}\n") {
		t.Errorf("App:\n%s\nwant it to start:\n%s", app, want)
	}
	if res, err := Apply(dir, edits, Options{}); err != nil {
		t.Fatalf("Apply: %v\n%s", err, res.RenderText(true))
	}

	// a session over several lines, with a field without a comma
	multi := strings.Replace(appWithUpdate, "session { theme: Theme.Theme = Light }", "session {\n  theme: Theme.Theme = Light\n}", 1)
	dir = scaffoldApp(t, map[string]string{"app/App.alder": multi, "app/Theme.alder": themeModule})
	if p, err = Load(dir); err != nil {
		t.Fatal(err)
	}
	if edits, _, err = NewAuth(p, AuthOptions{}); err != nil {
		t.Fatal(err)
	}
	for _, e := range edits {
		if e.Path == "app/App.alder" && !strings.Contains(e.Body, "session {\n  theme: Theme.Theme = Light,\n  user: Maybe User.UserId,\n}\n\nlogin = Auth.Login\n\nmsg Toggle\n") {
			t.Errorf("multi-line session:\n%s", e.Body)
		}
	}
	if res, err := Apply(dir, edits, Options{}); err != nil {
		t.Fatalf("Apply: %v\n%s", err, res.RenderText(true))
	}

	// the scaffold: no update
	dir = scaffoldApp(t, nil)
	if p, err = Load(dir); err != nil {
		t.Fatal(err)
	}
	if edits, _, err = NewAuth(p, AuthOptions{}); err != nil {
		t.Fatal(err)
	}
	for _, e := range edits {
		if e.Path == "app/App.alder" && !strings.HasPrefix(e.Body, "module App\n\nimport Layout\nimport Table.User\n\nsession { user: Maybe User.UserId }\n\nlogin = Auth.Login\n\nmsg Logout\n\nupdate : Msg -> Session -> Task Nav\nupdate msg session = match msg {\n  Logout p -> {\n") {
			t.Errorf("scaffold's App:\n%s", e.Body)
		}
	}
	if res, err := Apply(dir, edits, Options{}); err != nil {
		t.Fatalf("Apply: %v\n%s", err, res.RenderText(true))
	}
}

// TestNewAuth_Refusals: a second login, an Auth module, a role that is not a union of the user
// module, a user table without a password column, a --to that is not a page.
func TestNewAuth_Refusals(t *testing.T) {
	refused := func(dir string, o AuthOptions, want string) {
		t.Helper()
		p, err := Load(dir)
		if err != nil {
			t.Fatal(err)
		}
		if _, _, err := NewAuth(p, o); err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("NewAuth(%+v): %v, want %q", o, err, want)
		}
	}
	refused(fixture, AuthOptions{}, "the session already has a `user` field")
	refused(scaffoldApp(t, nil), AuthOptions{Role: "Role"}, "`alder new type Table.User \"Role = Member | Admin\"` makes one")
	refused(scaffoldApp(t, map[string]string{"app/Auth/Other.alder": "module Auth.Other\n"}), AuthOptions{}, "module `Auth.Other` exists")
	refused(scaffoldApp(t, map[string]string{"app/User.alder": "module User\n\ntype UserId = brand Text\n\ntable User {\n  id: UserId @id,\n  email: Text.Email,\n}\n"}),
		AuthOptions{}, "`User` needs the columns `email: Text.Email` and `passwordHash")
	refused(scaffoldApp(t, nil), AuthOptions{To: "Nope"}, "`--to Nope` is not a page")
}

// An app with no page at `/` and no --to (a bare app: its pages come after the login, since
// they `requires user`): the login goes to the typed hole ?afterLogin, filled once the page
// exists, and the result checks.
func TestNewAuth_NoHomePage(t *testing.T) {
	dir := scaffoldApp(t, nil)
	os.Remove(filepath.Join(dir, "app", "Home.alder"))
	p, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	edits, _, err := NewAuth(p, AuthOptions{})
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range edits {
		if e.Path == "app/Auth/Login.alder" && (!strings.Contains(e.Body, "Task.succeed (Redirect (?afterLogin))") || !strings.Contains(e.Body, "answer: Redirected (?afterLogin)")) {
			t.Errorf("Auth.Login:\n%s", e.Body)
		}
	}
	res, err := Apply(dir, edits, Options{})
	if err != nil {
		t.Fatalf("Apply: %v\n%s", err, res.RenderText(true))
	}
	if len(res.Holes) != 2 {
		t.Errorf("holes %+v, want ?afterLogin twice", res.Holes)
	}
}

// TestNewAuthUserInTableFolder (#20): the user table `alder new auth` makes is
// app/Table/User.alder, module Table.User, and the login imports it.
func TestNewAuthUserInTableFolder(t *testing.T) {
	dir := scaffoldApp(t, nil)
	p, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	edits, created, err := NewAuth(p, AuthOptions{})
	if err != nil || !created {
		t.Fatalf("NewAuth: %v (created %v)", err, created)
	}
	if e := editAt(edits, "app/Table/User.alder"); e == nil || !strings.HasPrefix(e.Body, "module Table.User\n") {
		t.Fatalf("edits: %+v", edits)
	}
	if editAt(edits, "app/User.alder") != nil {
		t.Errorf("wrote app/User.alder")
	}
	if e := editAt(edits, "app/Auth/Login.alder"); e == nil || !strings.Contains(e.Body, "\nimport Table.User\n") {
		t.Errorf("Auth.Login: %+v", e)
	}
	if res, err := Apply(dir, edits, Options{DryRun: true}); err != nil {
		t.Fatalf("Apply: %v\n%s", err, res.RenderText(true))
	}
}
