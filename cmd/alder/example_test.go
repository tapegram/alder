package main

import (
	"os"
	"path/filepath"
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
