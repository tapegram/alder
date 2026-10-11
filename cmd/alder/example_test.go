package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
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

// TestExample_Blog: the second example checks clean, builds, its Markdown adapter passes its Go
// tests, and the app passes its own tests, every one of which chapter 14 quotes by name.
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
	// the Markdown adapter's Go tests, which compile only in the built module (they import "app")
	gotest := exec.Command("go", "test", "-count=1", "./adapters")
	gotest.Dir = filepath.Join("build", "blog")
	if b, err := gotest.CombinedOutput(); err != nil {
		t.Fatalf("go test ./adapters in build/blog: %v\n%s", err, b)
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
		`ok   Posts.Show "a draft is not found for a reader"`,
		`ok   Posts.Show "a draft is shown to its author"`,
		`ok   Posts.Show "a comment on a draft is refused"`,
		`ok   Posts.Show "a comment is listed oldest first"`,
		`ok   Admin "a Reader cannot open the admin page"`,
		`ok   Admin "creating a post redirects to its edit page"`,
		`ok   Admin.Edit "deleting a post deletes its comments"`,
		`ok   Admin.Edit "another author's post is not found"`,
		"14 passed, 0 failed\n",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("alder test examples/blog lacks %q:\n%s", want, out)
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
