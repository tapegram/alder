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
)

// TestExample_SkeletonStandsAlone: the example's skeleton (App, Layout, User and the login
// page) checks clean without the rest of the app, so a new app that starts from a copy of
// these files needs no module it has not written. E7: models copied App.alder and never wrote
// the Theme module it imported. The one other module the skeleton names is the page the login
// sends the user to, which every app writes for itself; the test writes a bare one.
func TestExample_SkeletonStandsAlone(t *testing.T) {
	dir := t.TempDir()
	for _, rel := range []string{"App.alder", "Layout.alder", "User.alder", "Auth/Login.alder"} {
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
	needGoTool(t)
	code, out, errs := run(t, blogDir(t))
	if code != 0 || out != "" || errs != "" {
		t.Fatalf("alder check examples/blog: exit %d\nout: %s\nerr: %s", code, out, errs)
	}
	inTemp(t)
	copyDir(t, blogDir(t), "blog")
	code, out, errs = runBuild(t, "blog")
	if code != 0 || out != "wrote build/blog/blog\n" {
		t.Fatalf("alder build examples/blog: exit %d\nout: %s\nerr: %s", code, out, errs)
	}
	prodHasNoTests(t, "blog")
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
		`ok   Post "a slug is lowercase letters, digits and hyphens"`,
		`ok   Post "a draft is not visible"`,
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
		`ok   Comment "a name or comment has text and a bound"`,
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
		`ok   Post "bySlug finds the post with that slug"`,
		`ok   Post "an author with posts cannot be deleted"`,
		`ok   Write "the write page lists the author's own posts, newest first"`,
		`ok   Posts.Index "a page that is exactly full has no next page"`,
		`ok   Posts.Index "logging out from the front page goes to log in"`,
		`ok   Write.Edit "a visitor is sent to log in"`,
		`ok   Write.Edit "saving changes the title and the body"`,
		`ok   Posts.Show "commenting through the form"`,
		`ok   Auth.Login "an unknown email is rejected"`,
		`ok   Auth.Login "logging in through the form"`,
		// the generated admin (`alder new admin`, book 15), a few of its 82 tests
		`ok   Admin.Index "the dashboard counts the posts and lists the newest five"`,
		`ok   Admin.Index "a Reader gets no page, even one made Admin since logging in"`,
		`ok   Admin.Index "a demoted Admin is refused at once"`,
		`ok   Admin.Post.Index "searching finds the rows that contain the text"`,
		`ok   Admin.Post.New "an author not offered is refused"`,
		`ok   Admin.Post.Delete "the page counts the rows that go with it"`,
		`ok   Admin.User.Delete "a user that posts refer to is not deleted"`,
		"139 passed, 0 failed\n",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("alder test examples/blog lacks %q:\n%s", want, out)
		}
	}
	mutationAtLeast(t, "blog", blogMutationScore, blogMutants)
}

// The mutation scores examples/todos/README.md and examples/blog/README.md record (`alder test
// --mutate`, rounded down to two decimals: the blog's is 211 / 214 = 0.986) and their numbers of
// mutants: a change to an example that lets a mutant survive which its tests used to kill, or
// that changes the mutants made of it, fails TestExample_*.
const (
	todosMutationScore = 1.00
	blogMutationScore  = 0.98
	todosMutants       = 134
	blogMutants        = 214
)

// mutationAtLeast runs `alder test --mutate --json dir` and fails t when the score is below min
// or the number of mutants that ran is not mutants.
func mutationAtLeast(t *testing.T, dir string, min float64, mutants int) {
	t.Helper()
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
// mutant killed), measured on a copy so the build lands in a temporary directory.
func TestExample_TodosMutation(t *testing.T) {
	needGoTool(t)
	inTemp(t)
	copyDir(t, todosDir(t), "todos")
	mutationAtLeast(t, "todos", todosMutationScore, todosMutants)
}

// prodHasNoTests fails t when the production module build/<name> (alder build's) carries test
// code: Tests, TestSession, a test function or the rt/testing import.
func prodHasNoTests(t *testing.T, name string) {
	t.Helper()
	gos, _ := filepath.Glob(filepath.Join("build", name, "*.go"))
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
	dir := t.TempDir()
	for _, rel := range []string{"App.alder", "Layout.alder", "User.alder", "Auth/Login.alder"} {
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
