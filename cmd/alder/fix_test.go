package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"
)

func runFix(args ...string) (int, string, string) {
	var out, errb bytes.Buffer
	code := runFixTo(args, &out, &errb)
	return code, out.String(), errb.String()
}

// todosOnPages copies testdata/fix/todos (examples/todos as it was before #160 moved it to
// routes) into a temporary directory and returns the copy's root.
func todosOnPages(t *testing.T) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "todos")
	copyDir(t, "../../testdata/fix/todos", dir)
	return dir
}

// TestFix_Routes: `alder fix --routes` rewrites the todos app on pages to routes, prints a line
// per file changed and per hole, exits 1 for the holes, and `alder check` afterwards lists exactly
// those holes (W0002) and nothing else.
func TestFix_Routes(t *testing.T) {
	t.Parallel()
	dir := todosOnPages(t)
	code, out, errs := runFix("--routes", "--dir", dir)
	if code != 1 {
		t.Fatalf("code %d (want 1: holes remain)\nstdout:\n%s\nstderr:\n%s", code, out, errs)
	}
	for _, f := range []string{"app/App.alder", "app/Todos/Index.alder", "app/Todos/Show.alder", "app/Auth/Login.alder"} {
		if !strings.Contains(out, "rewrote "+f+"\n") {
			t.Errorf("no line for %s:\n%s", f, out)
		}
	}
	holeLine := regexp.MustCompile(`(?m)^(app/[^:]+):(\d+): hole: (.+)$`)
	var holes []string
	for _, m := range holeLine.FindAllStringSubmatch(out, -1) {
		holes = append(holes, m[1]+":"+m[2])
	}
	if len(holes) == 0 {
		t.Fatalf("no holes reported:\n%s", out)
	}
	if !strings.Contains(errs, "typed holes left") {
		t.Errorf("stderr does not count the holes:\n%s", errs)
	}
	for _, f := range []string{"app/Todos/Index.alder", "app/Todos/Show.alder", "app/App.alder"} {
		b, err := os.ReadFile(filepath.Join(dir, f))
		if err != nil {
			t.Fatal(err)
		}
		if regexp.MustCompile(`(?m)^(page|msg) `).Match(b) {
			t.Errorf("%s still has a page or msg:\n%s", f, b)
		}
	}

	var cout, cerr bytes.Buffer
	runCheckTo([]string{dir, "--json"}, &cout, &cerr)
	var got []string
	for _, line := range strings.Split(strings.TrimSpace(cout.String()), "\n") {
		if line == "" {
			continue
		}
		var d struct {
			Code, File, Message string
			Line                int
		}
		if err := json.Unmarshal([]byte(line), &d); err != nil {
			t.Fatalf("%v: %s", err, line)
		}
		if d.Code != "W0002" {
			t.Errorf("check lists more than the holes: %s", line)
			continue
		}
		got = append(got, d.File+":"+strconv.Itoa(d.Line))
	}
	slices.Sort(got)
	slices.Sort(holes)
	if !slices.Equal(got, holes) {
		t.Errorf("check's holes %v, fix reported %v\nfix:\n%s", got, holes, out)
	}

	// a second run has nothing left to rewrite, but lists the holes still unfilled and exits 1,
	// so a CI step that runs it keeps failing until they are filled
	code, out, errs = runFix("--routes", "--dir", dir)
	var again []string
	for _, m := range holeLine.FindAllStringSubmatch(out, -1) {
		again = append(again, m[1]+":"+m[2])
	}
	slices.Sort(again)
	if code != 1 || strings.Contains(out, "rewrote") || !strings.Contains(out, "nothing to rewrite") || !slices.Equal(again, holes) {
		t.Errorf("second run: code %d, holes %v, want %v\n%s%s", code, again, holes, out, errs)
	}
}

// TestFix_RoutesDryRun prints the changes as a diff and writes nothing.
func TestFix_RoutesDryRun(t *testing.T) {
	t.Parallel()
	dir := todosOnPages(t)
	before, _ := os.ReadFile(filepath.Join(dir, "app/Todos/Index.alder"))
	code, out, errs := runFix("--routes", "--dry-run", "--dir", dir)
	if code != 1 {
		t.Fatalf("code %d\n%s%s", code, out, errs)
	}
	for _, w := range []string{"would rewrite app/Todos/Index.alder", "+route Index", "-page "} {
		if !strings.Contains(out, w) {
			t.Errorf("dry run lacks %q:\n%s", w, out)
		}
	}
	after, _ := os.ReadFile(filepath.Join(dir, "app/Todos/Index.alder"))
	if !bytes.Equal(before, after) {
		t.Error("--dry-run wrote the file")
	}
}

// TestFix_RoutesFiles: files named are rewritten only when the app still checks with just them
// rewritten (holes aside), the old syntax of the files not named aside (E0046: they are left on
// pages for a later run). Todos/Index alone does not: the route module Links names its page,
// which would be a route. A page nothing else names migrates alone; the rest of the app is left
// as it was, and `alder check` lists only the holes and the old syntax left.
func TestFix_RoutesFiles(t *testing.T) {
	t.Parallel()
	dir := todosOnPages(t)
	links := "module Links\n\nroute Links = GET \"/links\" { }\n\nLinks.load : Links.Params -> Session -> Query Int\nLinks.load params session = Query.succeed 1\n\n" +
		"Links.view : Int -> Session -> Html\nLinks.view n session = <a href={Todos.Index {}}>todos</a>\n"
	if err := os.WriteFile(filepath.Join(dir, "app/Links.alder"), []byte(links), 0o644); err != nil {
		t.Fatal(err)
	}
	index := filepath.Join(dir, "app/Todos/Index.alder")
	before, _ := os.ReadFile(index)
	code, out, errs := runFix("--routes", "--dir", dir, index)
	if code != 2 || strings.Contains(out, "rewrote") || !strings.Contains(errs, "E0002") ||
		!strings.Contains(errs, "rewriting only the files named leaves the app with") {
		t.Fatalf("Todos/Index alone: code %d\n%s%s", code, out, errs)
	}
	if after, _ := os.ReadFile(index); !bytes.Equal(before, after) {
		t.Error("a refused run wrote the file")
	}

	about := `module About

import Table.User

page "/about/{n}" { n: Int } requires user

type Model = { n: Int, seen: Int }

load : Params -> Session -> Query Model
load params session = {
  let users <- Db.from User |> Db.count
  Query.succeed { n: params.n, seen: users }
}

msg Bump

update : Msg -> Model -> Session -> Task Nav
update msg model session = match msg {
  Bump _ -> Task.succeed (Redirect (About { n: model.n + 1 })),
}

view : Model -> Session -> Html Msg
view model session = <p>{Text.fromInt model.n} <button click={Bump}>bump</button></p>
`
	if err := os.WriteFile(filepath.Join(dir, "app/About.alder"), []byte(about), 0o644); err != nil {
		t.Fatal(err)
	}
	app, _ := os.ReadFile(filepath.Join(dir, "app/App.alder"))
	code, out, errs = runFix("--routes", "--dir", dir, filepath.Join(dir, "app/About.alder"))
	if code != 1 || out == "" || !strings.HasPrefix(out, "rewrote app/About.alder\n") || strings.Contains(out, "rewrote app/App.alder") {
		t.Fatalf("About: code %d\n%s%s", code, out, errs)
	}
	if again, _ := os.ReadFile(filepath.Join(dir, "app/App.alder")); !bytes.Equal(app, again) {
		t.Error("a file not named was written")
	}
	var cout, cerr bytes.Buffer
	runCheckTo([]string{dir, "--json"}, &cout, &cerr)
	n := 0
	for _, line := range strings.Split(strings.TrimSpace(cout.String()), "\n") {
		if line == "" {
			continue
		}
		if strings.Contains(line, `"code":"E0046"`) && !strings.Contains(line, `"file":"app/About.alder"`) {
			continue // the rest of the app, left on pages
		}
		n++
		if !strings.Contains(line, `"code":"W0002"`) || !strings.Contains(line, `"file":"app/About.alder"`) {
			t.Errorf("after a file-scoped run, check lists more than its holes and the old syntax left: %s", line)
		}
	}
	if holes := strings.Count(out, ": hole: "); n == 0 || n != holes {
		t.Errorf("check lists %d holes, fix reported %d\n%s", n, holes, out)
	}
}

// TestFix_RefusesBrokenApp: the rewrite reads every file's syntax, so a file that does not parse
// is refused and nothing is written. A type error is not: the page syntax the rewrite reads is
// itself an `alder check` error (E0046), and the check after the rewrite reports the rest.
func TestFix_RefusesBrokenApp(t *testing.T) {
	t.Parallel()
	dir := todosOnPages(t)
	f := filepath.Join(dir, "app/Todos/Index.alder")
	src, _ := os.ReadFile(f)
	broken := append(src, []byte("\nbroken : Int\nbroken = (1\n")...)
	if err := os.WriteFile(f, broken, 0o644); err != nil {
		t.Fatal(err)
	}
	code, out, errs := runFix("--routes", "--dir", dir)
	if code != 2 || !strings.Contains(errs, "does not parse") || !strings.Contains(errs, "E0001") {
		t.Fatalf("code %d\nstdout:\n%s\nstderr:\n%s", code, out, errs)
	}
	if again, _ := os.ReadFile(f); !bytes.Equal(broken, again) {
		t.Error("the broken app was rewritten")
	}
}

// TestFix_Usage: --routes is required.
func TestFix_Usage(t *testing.T) {
	t.Parallel()
	if code, _, errs := runFix(); code != 2 || !strings.Contains(errs, "usage: alder fix --routes") {
		t.Errorf("no args: code %d %s", code, errs)
	}
	if code, _, errs := runFix("--bogus"); code != 2 || !strings.Contains(errs, "usage:") {
		t.Errorf("--bogus: code %d %s", code, errs)
	}
}

// TestFix_RoutesOldAdmin: a page that `alder new admin` wrote (with `@admin` regions) is not
// rewritten: its writes are gated only by its `load`, which a route would not run, and its
// regions would not line up with the route lines. It stays a page, the command says to
// regenerate the admin, and exits 1.
func TestFix_RoutesOldAdmin(t *testing.T) {
	t.Parallel()
	dir := todosOnPages(t)
	old := "-- generated by `alder new admin`; regions between\n" +
		"-- `-- @admin begin <name>` / `-- @admin end` are rewritten by `alder regen admin`\n" +
		`module Admin.Old

page "/admin/old" { } requires user, role Admin

type Model = { }

load : Params -> Session -> Query Model
load params session = Query.succeed { }

-- @admin begin go
msg Go
-- @admin end

update : Msg -> Model -> Session -> Task Nav
update msg model session = match msg {
  Go _ -> Task.succeed Rerender,
}

view : Model -> Session -> Html Msg
view model session = <button click={Go}>Go</button>
`
	f := filepath.Join(dir, "app/Admin/Old.alder")
	if err := os.WriteFile(f, []byte(old), 0o644); err != nil {
		t.Fatal(err)
	}
	code, out, errs := runFix("--routes", "--dir", dir)
	if code != 1 || strings.Contains(out, "rewrote app/Admin/Old.alder") || !strings.Contains(out, "rewrote app/Todos/Index.alder") {
		t.Fatalf("code %d\n%s%s", code, out, errs)
	}
	if b, _ := os.ReadFile(f); string(b) != old {
		t.Errorf("the admin page was rewritten:\n%s", b)
	}
	if !strings.Contains(errs, "1 page (app/Admin/Old.alder…) written by alder new admin") || !strings.Contains(errs, "left on pages") ||
		!strings.Contains(errs, "regenerate the admin") {
		t.Errorf("no advice to regenerate the admin:\n%s", errs)
	}

	// named alone, it is left too
	code, out, errs = runFix("--routes", "--dir", dir, f)
	if code != 1 || strings.Contains(out, "rewrote") || !strings.Contains(errs, "written by alder new admin") {
		t.Errorf("named alone: code %d\n%s%s", code, out, errs)
	}
	if b, _ := os.ReadFile(f); string(b) != old {
		t.Errorf("the admin page was rewritten:\n%s", b)
	}
}

// TestFix_RoutesReportsErrorsAfter: a rewrite that leaves an error besides the holes says so and
// exits 1. Here App's login page `requires user`, which a page could and a route cannot (E0011).
func TestFix_RoutesReportsErrorsAfter(t *testing.T) {
	t.Parallel()
	dir := todosOnPages(t)
	f := filepath.Join(dir, "app/App.alder")
	src, _ := os.ReadFile(f)
	src = bytes.Replace(src, []byte("login = Auth.Login\n"), []byte("login = Todos.Index\n"), 1)
	if err := os.WriteFile(f, src, 0o644); err != nil {
		t.Fatal(err)
	}
	code, out, errs := runFix("--routes", "--dir", dir)
	if code != 1 || !strings.Contains(out, "rewrote app/App.alder") ||
		!strings.Contains(errs, "E0011") || !strings.Contains(errs, "alder fix: alder check finds 1 error after the rewrite") {
		t.Fatalf("code %d\nstdout:\n%s\nstderr:\n%s", code, out, errs)
	}
}

// TestFix_RoutesParamClash: a message whose payload has a field named like one of its page's
// path parameters is refused (its route would carry the field where the path parameter was):
// the command names the page, the message and the field, says to rename the field, exits 2 and
// writes nothing. testdata/fix/workspace's Projects.Show has `msg Toggle { id: Todo.TodoId }`.
func TestFix_RoutesParamClash(t *testing.T) {
	t.Parallel()
	dir := filepath.Join(t.TempDir(), "workspace")
	copyDir(t, "../../testdata/fix/workspace", dir)
	show := filepath.Join(dir, "app/Projects/Show.alder")
	before, _ := os.ReadFile(show)
	code, out, errs := runFix("--routes", "--dir", dir)
	if code != 2 || strings.Contains(out, "rewrote") {
		t.Fatalf("code %d\nstdout:\n%s\nstderr:\n%s", code, out, errs)
	}
	for _, w := range []string{"Projects.Show", "`Toggle`", "`id`", "rename"} {
		if !strings.Contains(errs, w) {
			t.Errorf("stderr lacks %q:\n%s", w, errs)
		}
	}
	if after, _ := os.ReadFile(show); !bytes.Equal(before, after) {
		t.Error("a refused run wrote the file")
	}
}

// TestFix_RoutesLoadCheck: before routes every message ran its page's `load` first, so a `load`
// that checked access (here `Db.getMine`: anyone else's todo is a 404) gated every write. A route
// does not run it: the rewrite puts a comment above each write route's `update` saying so, prints
// a line per such route and exits 1, though `Wipe`'s arm does not read the model and leaves no
// typed hole. The note is the first run's: a second run has nothing to rewrite and exits 0.
// TestFix_RoutesFileAppMsg: a page whose button sends one of App's old messages
// (`click={App.Logout}`) is rewritten on its own: App, still on messages, is E0046 alone, and the
// rewritten route's reference to `App.Logout` resolves quietly until App is rewritten too.
func TestFix_RoutesFileAppMsg(t *testing.T) {
	t.Parallel()
	dir := todosOnPages(t)
	bye := "module Bye\n\npage \"/bye\" { }\n\ntype Model = { n: Int }\n\nload : Params -> Session -> Query Model\nload params session = Query.succeed { n: 1 }\n\n" +
		"view : Model -> Session -> Html Msg\nview model session = <button click={App.Logout}>log out</button>\n"
	f := filepath.Join(dir, "app/Bye.alder")
	if err := os.WriteFile(f, []byte(bye), 0o644); err != nil {
		t.Fatal(err)
	}
	code, out, errs := runFix("--routes", "--dir", dir, f)
	if code > 1 || !strings.Contains(out, "rewrote app/Bye.alder\n") || strings.Contains(errs, "E0002") {
		t.Fatalf("Bye alone: code %d\nstdout:\n%s\nstderr:\n%s", code, out, errs)
	}
}

func TestFix_RoutesLoadCheck(t *testing.T) {
	t.Parallel()
	dir := todosOnPages(t)
	tidy := `module Todos.Tidy

import Table.Todo

page "/tidy/{id}" { id: Todo.TodoId } requires user

type Model = { todo: Todo }

-- only the todo's owner may open this page: anyone else's is a 404
load : Params -> Session -> Query Model
load params session = {
  let todo <- Db.getMine Todo params.id
  Query.succeed { todo: todo }
}

msg Wipe { other: Todo.TodoId }

update : Msg -> Model -> Session -> Task Nav
update msg model session = match msg {
  Wipe p -> {
    let _ <- Db.delete Todo p.other
    Task.succeed Rerender
  },
}

view : Model -> Session -> Html Msg
view model session = <h1>{model.todo.title}</h1>
`
	f := filepath.Join(dir, "app/Todos/Tidy.alder")
	if err := os.WriteFile(f, []byte(tidy), 0o644); err != nil {
		t.Fatal(err)
	}
	code, out, errs := runFix("--routes", "--dir", dir, f)
	if code != 1 || !strings.Contains(out, "rewrote app/Todos/Tidy.alder\n") || strings.Contains(out, ": hole: ") {
		t.Fatalf("code %d (want 1, no typed hole)\nstdout:\n%s\nstderr:\n%s", code, out, errs)
	}
	m := regexp.MustCompile(`(?m)^app/Todos/Tidy\.alder:(\d+): access: (.+)$`).FindAllStringSubmatch(out, -1)
	if len(m) != 1 || !strings.Contains(m[0][2], "Wipe") || !strings.Contains(m[0][2], "Tidy.load") {
		t.Fatalf("want one access line for Wipe:\n%s", out)
	}
	for _, bad := range []string{"p.id", "path", "payload"} {
		if strings.Contains(m[0][2], bad) {
			t.Errorf("the access line steers to %q: %s", bad, m[0][2])
		}
	}
	b, _ := os.ReadFile(f)
	lines := strings.Split(string(b), "\n")
	n, _ := strconv.Atoi(m[0][1])
	if n < 1 || n >= len(lines) || !strings.HasPrefix(lines[n-1], "-- ") || !strings.Contains(lines[n-1], "Tidy.load") ||
		!strings.Contains(lines[n-1], "#160") || !strings.HasPrefix(lines[n], "Wipe.update :") {
		t.Errorf("line %d is not the comment directly above Wipe.update:\n%s", n, b)
	}
	if !strings.Contains(errs, "1 write route") {
		t.Errorf("stderr does not count the write routes:\n%s", errs)
	}
	if code, out, errs := runFix("--routes", "--dir", dir, f); code != 0 || !strings.Contains(out, "nothing to rewrite") {
		t.Errorf("second run: code %d\n%s%s", code, out, errs)
	}
}
