package main

import (
	"encoding/json"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"alder/internal/csscheck"
)

// TestExample_SkeletonStandsAlone: the example's skeleton (App, Layout, User and the login
// page) checks clean without the rest of the app, so a new app that starts from a copy of
// these files needs no module it has not written. E7: models copied App.alder and never wrote
// the Theme module it imported. The one other module the skeleton names is the page the login
// sends the user to, which every app writes for itself; the test writes a bare one.
func TestExample_SkeletonStandsAlone(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	for _, rel := range []string{"App.alder", "Layout.alder", "Table/User.alder", "Auth/Login.alder"} {
		b, err := os.ReadFile(filepath.Join(todosDir(t), "app", filepath.FromSlash(rel)))
		if err != nil {
			t.Fatal(err)
		}
		dst := filepath.Join(dir, "app", filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(dst, b, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	home := `module Todos.Index

import Layout

page "/" { } requires user

type Model = { }

load : Params -> Session -> Query Model
load params session = Query.succeed { }

view : Model -> Session -> Html Msg
view model session =
  <Layout.Page title="Home">
    <button click={App.Logout}>Log out</button>
  </Layout.Page>
`
	if err := os.MkdirAll(filepath.Join(dir, "app", "Todos"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "app", "Todos", "Index.alder"), []byte(home), 0o644); err != nil {
		t.Fatal(err)
	}
	code, out, errs := run(t, dir)
	if code != 0 || out != "" || errs != "" {
		t.Fatalf("alder check of the skeleton alone: exit %d\nout: %s\nerr: %s", code, out, errs)
	}
}

// blogAbs is examples/blog, made absolute before any test changes directory.
var blogAbs, blogAbsErr = filepath.Abs("../../examples/blog")

// blogDir is examples/blog, absolute, so a test may run from a temporary directory.
func blogDir(t *testing.T) string {
	t.Helper()
	if blogAbsErr != nil {
		t.Fatal(blogAbsErr)
	}
	return blogAbs
}

// TestExample_Blog: the second example checks clean, builds — a production module with no test
// code (no Tests, no rt/testing import) whose binary serves the front page —, its adapters
// (adapters/auth/ and adapters/markdown/) pass their Go tests, and the app passes its own tests,
// every one of which chapter 14 quotes by name.
func TestExample_Blog(t *testing.T) {
	// serial: inTemp (t.Chdir) into a fresh dir; the test names its app by a relative path
	needGoTool(t)
	code, out, errs := run(t, blogDir(t))
	if code != 0 || out != "" || errs != "" {
		t.Fatalf("alder check examples/blog: exit %d\nout: %s\nerr: %s", code, out, errs)
	}
	inTemp(t)
	copyDir(t, blogDir(t), "blog")
	code, out, errs = runBuild(t, "--build-dir", "build", "blog")
	if code != 0 || out != "wrote build/blog/blog\n" {
		t.Fatalf("alder build examples/blog: exit %d\nout: %s\nerr: %s", code, out, errs)
	}
	prodHasNoTests(t, "build", "blog")
	tmp, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	app := startAppNamed(t, tmp, "blog", "ALDER_DEV=1", "ALDER_SECRET=")
	res, err := (&http.Client{Timeout: 10 * time.Second}).Get("http://" + app.addr + "/")
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(res.Body)
	res.Body.Close()
	if res.StatusCode != http.StatusOK || !strings.Contains(string(body), "<html") {
		t.Fatalf("GET / of the blog = %d:\n%s", res.StatusCode, body)
	}
	if code := app.stop(t, os.Interrupt); code != 0 {
		t.Fatalf("the blog after SIGINT: exit %d; stderr:\n%s", code, app.stderr.String())
	}
	// the adapters' Go tests, which compile only in the built module (they import "app")
	gotest := exec.Command("go", "test", "-count=1", "./adapters/...")
	gotest.Dir = filepath.Join("build", "blog")
	b, err := gotest.CombinedOutput()
	if err != nil {
		t.Fatalf("go test ./adapters/... in build/blog: %v\n%s", err, b)
	}
	for _, pkg := range []string{"app/adapters/auth", "app/adapters/markdown"} {
		if !strings.Contains(string(b), "ok  \t"+pkg+"\t") {
			t.Errorf("go test ./adapters/... in build/blog did not pass %s:\n%s", pkg, b)
		}
	}
	code, out, errs = runTest(t, "blog")
	if code != 0 {
		t.Fatalf("alder test examples/blog: exit %d\nout: %s\nerr: %s", code, out, errs)
	}
	for _, want := range []string{
		`ok   Table.Post "a slug is lowercase letters, digits and hyphens"`,
		`ok   Table.Post "a draft is not visible"`,
		`ok   Posts.Index "the front page lists only published posts, newest first"`,
		`ok   Posts.Index "search matches title or body"`,
		`ok   Posts.Index "a ` + "`%`" + ` in the query is literal"`,
		`ok   Posts.Index "page 99 is empty"`,
		`ok   Posts.Index "a negative page is page 1"`,
		`ok   Posts.Index "a ` + "`_`" + ` in the query is literal"`,
		`ok   Posts.Index "a long query is cut"`,
		`ok   Posts.Index "a search answers with the first page of its results"`,
		`ok   Posts.Index "only an Author sees the Write link"`,
		`ok   Posts.Index "the second page links back to the first"`,
		`ok   Posts.Index "a search through the form"`,
		`ok   Table.Comment "a name or comment has text and a bound"`,
		`ok   Posts.Show "a draft is not found for a reader"`,
		`ok   Posts.Show "a draft is shown to its author"`,
		`ok   Posts.Show "a post's body is rendered through the Markdown capability"`,
		`ok   Posts.Show "a comment on a draft is refused"`,
		`ok   Posts.Show "a comment is listed oldest first"`,
		`ok   Posts.Show "an empty comment is rejected"`,
		`ok   Write "a Reader cannot open the write page"`,
		`ok   Write "creating a post redirects to its edit page"`,
		`ok   Write "a taken slug is refused"`,
		`ok   Write "the write page needs an Author"`,
		`ok   Write "an Author logs out from the write page"`,
		`ok   Write.Edit "deleting a post deletes its comments"`,
		`ok   Write.Edit "another author's post is not found"`,
		`ok   Write.Edit "a demoted author is turned away at once"`,
		`ok   Write.Edit "a promoted reader logs in again before editing"`,
		`ok   Write "a demoted author cannot open the write page"`,
		`ok   Write "a promoted reader logs in again first"`,
		`ok   Write.Edit "Publish shows the post on the front page; Unpublish hides it"`,
		`ok   Table.Post "bySlug finds the post with that slug"`,
		`ok   Table.Post "an author with posts cannot be deleted"`,
		`ok   Write "the write page lists the author's own posts, newest first"`,
		`ok   Posts.Index "a page that is exactly full has no next page"`,
		`ok   Posts.Index "logging out from the front page goes to log in"`,
		`ok   Write.Edit "a visitor is sent to log in"`,
		`ok   Write.Edit "saving changes the title and the body"`,
		`ok   Posts.Show "commenting through the form"`,
		`ok   Auth.Login "an unknown email is rejected"`,
		`ok   Auth.Login "logging in through the form"`,
		// the generated admin (`alder new admin`, book 15), a few of its 103 tests
		`ok   Admin.Index "the dashboard counts the posts and lists the newest five"`,
		`ok   Admin.Index "a Reader gets no page, even one made Admin since logging in"`,
		`ok   Admin.Index "a demoted Admin is refused at once"`,
		`ok   Admin.Posts.Index "searching finds the rows that contain the text"`,
		`ok   Admin.Posts.New "an author not offered is refused"`,
		`ok   Admin.Posts.Delete "the page counts the rows that go with it"`,
		`ok   Admin.Users.Delete "a user that posts refer to is not deleted"`,
		`ok   Admin.Posts.Index "filtering by published lists the rows with that value"`,
		`ok   Admin.Users.Index "filtering by role lists the rows with that value"`,
		`ok   Admin.Users.Edit "an Admin cannot change their own role"`,
		`ok   Admin.Users.Edit "an Admin demotes another Admin"`,
		"183 passed, 0 failed\n",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("alder test examples/blog lacks %q:\n%s", want, out)
		}
	}
	mutationAtLeast(t, "blog", blogMutationScore, blogMutants)
}

// The mutation scores examples/todos/README.md and examples/blog/README.md record (`alder test
// --mutate`, rounded down to two decimals: the blog's is 317 / 323 = 0.981) and their numbers of
// mutants: a change to an example that lets a mutant survive which its tests used to kill, or
// that changes the mutants made of it, fails TestExample_*, when the mutation quality gates run
// (nightly in CI, or with ALDER_MUTATION_GATES=1: see gates_test.go).
const (
	todosMutationScore     = 0.99
	blogMutationScore      = 0.98
	workspaceMutationScore = 0.97
	todosMutants           = 208
	blogMutants            = 323
	workspaceMutants       = 522
)

// mutationAtLeast runs `alder test --mutate --json dir` and fails t when the score is below min
// or the number of mutants that ran is not mutants. It is a mutation quality gate, so it only
// logs unless the gates run (the tests that call it test more than the mutation).
func mutationAtLeast(t *testing.T, dir string, min float64, mutants int) {
	t.Helper()
	if !mutationGates() {
		t.Logf("alder test --mutate %s: skipped, a mutation quality gate (%s=1 runs it)", dir, mutationGatesEnv)
		return
	}
	code, out, errs := runTest(t, dir, "--mutate", "--json")
	if code != 0 {
		t.Fatalf("alder test --mutate %s: exit %d\nout: %s\nerr: %s", dir, code, out, errs)
	}
	lines := strings.Split(strings.TrimSpace(out), "\n")
	var score struct {
		Score            *float64
		Killed, Survived int
	}
	if err := json.Unmarshal([]byte(lines[len(lines)-1]), &score); err != nil || score.Score == nil {
		t.Fatalf("alder test --mutate --json %s: no score on the last line:\n%s", dir, out)
	}
	if *score.Score < min {
		t.Errorf("alder test --mutate %s: score %.4f, below the recorded %.2f:\n%s", dir, *score.Score, min, out)
	}
	if n := score.Killed + score.Survived; n != mutants {
		t.Errorf("alder test --mutate %s: %d mutants, the README records %d", dir, n, mutants)
	}
}

// TestExample_TodosMutation: examples/todos keeps the mutation score its README records (every
// mutant killed), measured on a copy so the build lands in a temporary directory. A mutation
// quality gate: it runs nightly in CI, or with ALDER_MUTATION_GATES=1.
func TestExample_TodosMutation(t *testing.T) {
	// serial: inTemp (t.Chdir) into a fresh dir; the test names its app by a relative path
	mutationGate(t)
	needGoTool(t)
	inTemp(t)
	copyDir(t, todosDir(t), "todos")
	mutationAtLeast(t, "todos", todosMutationScore, todosMutants)
}

// prodHasNoTests fails t when the production module build/<name> (alder build's) carries test
// code: Tests, TestSession, a test function or the rt/testing import.
func prodHasNoTests(t *testing.T, buildDir, name string) {
	t.Helper()
	gos, _ := filepath.Glob(filepath.Join(buildDir, name, "*.go"))
	if len(gos) == 0 {
		t.Fatalf("no generated Go in build/%s", name)
	}
	for _, f := range gos {
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		for _, bad := range []string{"rtesting", "alder/rt/testing", "var Tests", "TestSession", "func test__"} {
			if strings.Contains(string(b), bad) {
				t.Errorf("production %s contains %q", f, bad)
			}
		}
	}
}

// TestExample_BlogSkeletonStandsAlone: the blog's skeleton (App, Layout, User and the login
// page) checks clean without the rest of the app, as the to-do example's does
// (TestExample_SkeletonStandsAlone); the login sends the user to Posts.Index, which the test
// writes bare.
func TestExample_BlogSkeletonStandsAlone(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	for _, rel := range []string{"App.alder", "Layout.alder", "Table/User.alder", "Auth/Login.alder"} {
		b, err := os.ReadFile(filepath.Join(blogDir(t), "app", filepath.FromSlash(rel)))
		if err != nil {
			t.Fatal(err)
		}
		dst := filepath.Join(dir, "app", filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(dst, b, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	home := `module Posts.Index

import Layout

page "/" { }

type Model = { }

load : Params -> Session -> Query Model
load params session = Query.succeed { }

view : Model -> Session -> Html Msg
view model session =
  <Layout.Page title="Home">
    <button click={App.Logout}>Log out</button>
  </Layout.Page>
`
	if err := os.MkdirAll(filepath.Join(dir, "app", "Posts"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "app", "Posts", "Index.alder"), []byte(home), 0o644); err != nil {
		t.Fatal(err)
	}
	code, out, errs := run(t, dir)
	if code != 0 || out != "" || errs != "" {
		t.Fatalf("alder check of the blog's skeleton alone: exit %d\nout: %s\nerr: %s", code, out, errs)
	}
}

// workspaceAbs is examples/workspace, made absolute before any test changes directory.
var workspaceAbs, workspaceAbsErr = filepath.Abs("../../examples/workspace")

// TestExample_Workspace (M15): the third example — orgs, memberships (`org … via Membership`,
// `choose org`), projects and to-dos in `silo org`, an Activity log partitioned by org in the
// shared database, and the admin of the silo — checks clean, builds, serves its login page, its
// adapter passes its Go tests, and the app passes its tests (chapter 14 quotes them by name):
// two orgs, a non-member's switch refused, a member seeing only their org's projects, a read of
// each store in one load, the re-read idiom, and the admin's picker. Its committed admin is
// exactly what `alder new admin` writes (README, "Replaying the admin"), and it keeps the
// mutation score its README records.
func TestExample_Workspace(t *testing.T) {
	// serial: inTemp (t.Chdir) into a fresh dir; the test names its app by a relative path
	needGoTool(t)
	if workspaceAbsErr != nil {
		t.Fatal(workspaceAbsErr)
	}
	code, out, errs := run(t, workspaceAbs)
	if code != 0 || out != "" || errs != "" {
		t.Fatalf("alder check examples/workspace: exit %d\nout: %s\nerr: %s", code, out, errs)
	}
	// the admin replays byte for byte
	replay := filepath.Join(t.TempDir(), "workspace")
	copyDir(t, workspaceAbs, replay)
	os.RemoveAll(filepath.Join(replay, "build"))
	os.RemoveAll(filepath.Join(replay, "app", "Admin"))
	if code, out, errs := alderIn(t, replay, "", "new", "admin"); code != 0 {
		t.Fatalf("alder new admin in a copy: exit %d\n%s%s", code, out, errs)
	}
	committed, replayed := appFiles(t, workspaceAbs), appFiles(t, replay)
	for p, s := range replayed {
		if strings.HasPrefix(p, "app/Admin/") && committed[p] != s {
			t.Errorf("%s: the committed admin is not what `alder new admin` writes", p)
		}
	}
	for p := range committed {
		if _, ok := replayed[p]; strings.HasPrefix(p, "app/Admin/") && !ok {
			t.Errorf("%s: committed, but `alder new admin` does not write it", p)
		}
	}

	inTemp(t)
	copyDir(t, workspaceAbs, "workspace")
	code, out, errs = runBuild(t, "--build-dir", "build", "workspace")
	if code != 0 || out != "wrote build/workspace/workspace\n" {
		t.Fatalf("alder build examples/workspace: exit %d\nout: %s\nerr: %s", code, out, errs)
	}
	prodHasNoTests(t, "build", "workspace")
	tmp, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	app := startAppNamed(t, tmp, "workspace", "ALDER_DEV=1", "ALDER_SECRET=")
	res, err := (&http.Client{Timeout: 10 * time.Second}).Get("http://" + app.addr + "/login")
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(res.Body)
	res.Body.Close()
	if res.StatusCode != http.StatusOK || !strings.Contains(string(body), "<h1>Log in</h1>") {
		t.Fatalf("GET /login of the workspace = %d:\n%s", res.StatusCode, body)
	}
	if code := app.stop(t, os.Interrupt); code != 0 {
		t.Fatalf("the workspace after SIGINT: exit %d; stderr:\n%s", code, app.stderr.String())
	}
	gotest := exec.Command("go", "test", "-count=1", "./adapters/...")
	gotest.Dir = filepath.Join("build", "workspace")
	b, err := gotest.CombinedOutput()
	if err != nil || !strings.Contains(string(b), "ok  \tapp/adapters/auth\t") {
		t.Fatalf("go test ./adapters/... in build/workspace: %v\n%s", err, b)
	}
	code, out, errs = runTest(t, "workspace")
	if code != 0 {
		t.Fatalf("alder test examples/workspace: exit %d\nout: %s\nerr: %s", code, out, errs)
	}
	for _, want := range []string{
		`ok   Orgs.Index "a member opens one of their orgs"`,
		`ok   Orgs.Index "a non-member's switch is refused"`,
		`ok   Orgs.Index "a membership deleted earlier in the same Task counts as gone"`,
		`ok   Orgs.Index "creating an org makes the user its member, logs it, and opens it"`,
		`ok   Projects.Index "a member sees the org's name and only its projects, by name"`,
		`ok   Projects.Index "a project is created in the org's database, and another org does not see it"`,
		`ok   Projects.Index "another org's project cannot be removed"`,
		`ok   Projects.Index "a member removed from the org loses access at once"`,
		`ok   Projects.Index "without an org the page sends you to choose one"`,
		`ok   Projects.Show "another org's project is not found"`,
		`ok   Projects.Show "deleting a project deletes its to-dos, in the org's database"`,
		`ok   Members.Index "the members, by name, and the activity of this org only"`,
		`ok   Members.Index "removing a member logs it, and a removed member loses access at once"`,
		`ok   Auth.Login "logging in opens the org the user joined first, with their role"`,
		// the generated admin (`alder new admin`, book 15): the picker and the pages of one org
		`ok   Admin.Orgs.Pick "the picker lists every org, by name, each opening its dashboard"`,
		`ok   Admin.Orgs.Dashboard "the org's dashboard counts its projects, not another org's, and lists the newest five"`,
		`ok   Admin.Projects.Index "an Admin who is no member of the org lists its projects, and only its"`,
		`ok   Admin.Projects.Index "a Member gets no page, even one made Admin since logging in"`,
		`ok   Admin.Projects.Bulk "a bulk action stays inside one org"`,
		"307 passed, 0 failed\n",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("alder test examples/workspace lacks %q:\n%s", want, out)
		}
	}
	mutationAtLeast(t, "workspace", workspaceMutationScore, workspaceMutants)
}

// The examples' stylesheets use alder-ui v0.4's tokens: no removed --al-* token (the
// --al-width-* layout widths are kept).
func TestExampleCSSTokens(t *testing.T) {
	files, err := filepath.Glob("../../examples/*/static/*.css")
	if err != nil || len(files) == 0 {
		t.Fatalf("no example stylesheets (%v)", err)
	}
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		if got := csscheck.RemovedALTokens(string(b)); len(got) > 0 {
			t.Errorf("%s: removed --al-* tokens: %q", f, got)
		}
	}
	for f, want := range map[string]string{
		"../../examples/blog/static/app.css":  "var(--font-mono)",
		"../../examples/todos/static/app.css": "var(--destructive)",
	} {
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(b), want) {
			t.Errorf("%s lacks %s", f, want)
		}
	}
}
