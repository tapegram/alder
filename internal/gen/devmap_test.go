package gen

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

// decodeDevMap lowers the app in srcs (file name to source) and decodes its dev map.
func decodeDevMap(t *testing.T, srcs map[string]string) devMap {
	t.Helper()
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "app"), 0o755); err != nil {
		t.Fatal(err)
	}
	for name, src := range srcs {
		if err := os.WriteFile(filepath.Join(dir, "app", name), []byte(src), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	var m devMap
	if err := json.Unmarshal([]byte(devMapJSON(lowerDir(t, dir))), &m); err != nil {
		t.Fatal(err)
	}
	return m
}

func TestDevMapJSON_SQLNames(t *testing.T) {
	m := decodeDevMap(t, map[string]string{"Book.alder": `module Book

type BookId = brand Text

table Book {
  id: BookId @id,
  title: Text,
  pages: Int,
  rating: Maybe Float,
}
`})
	if len(m.Tables) != 1 {
		t.Fatalf("tables = %+v", m.Tables)
	}
	tb := m.Tables[0]
	if tb.Name != "Book.Book" || tb.SQL != "Book__Book" || tb.Silo != "" {
		t.Errorf("table = %+v", tb)
	}
	want := []devCol{
		{Name: "id", Type: "Text", SQL: "id", SQLType: "TEXT"},
		{Name: "title", Type: "Text", SQL: "title", SQLType: "TEXT"},
		{Name: "pages", Type: "Int", SQL: "pages", SQLType: "INTEGER"},
		{Name: "rating", Type: "Maybe Float", SQL: "rating", SQLType: "REAL"},
	}
	if len(tb.Cols) != len(want) {
		t.Fatalf("cols = %+v", tb.Cols)
	}
	for i, w := range want {
		if tb.Cols[i] != w {
			t.Errorf("cols[%d] = %+v, want %+v", i, tb.Cols[i], w)
		}
	}
}

func TestDevMapJSON_Silos(t *testing.T) {
	dir := "../../testdata/golden/gen_silos"
	var m devMap
	if err := json.Unmarshal([]byte(devMapJSON(lowerDir(t, dir))), &m); err != nil {
		t.Fatal(err)
	}
	if len(m.Silos) != 1 {
		t.Fatalf("silos = %+v", m.Silos)
	}
	s := m.Silos[0]
	if s.Key != "org" || s.Ref != "Org.Org" || len(s.Tables) != 1 || s.Tables[0] != "Project.Project" || s.Path != "silos/org/{value}.db" {
		t.Errorf("silo = %+v", s)
	}
	for _, tb := range m.Tables {
		want := ""
		if tb.Name == "Project.Project" {
			want = "org"
		}
		if tb.Silo != want {
			t.Errorf("%s: silo = %q, want %q", tb.Name, tb.Silo, want)
		}
	}
}

// The map lists every route once, GET and write, with its method, module, path and requires.
func TestDevMapJSON_Routes(t *testing.T) {
	m := decodeDevMap(t, map[string]string{
		"App.alder": `module App

import User

session { user: Maybe User.UserId }

login = Login.Login

error : Error -> Html
error err = <html><body><h1>error</h1></body></html>

notFound : Html
notFound = <html><body><h1>not found</h1></body></html>
`,
		"Login.alder": `module Login

route Login = GET "/login" { }

Login.load : Login.Params -> Session -> Query {}
Login.load params session = Query.succeed {}

Login.view : {} -> Session -> Html
Login.view model session = <html><body><h1>log in</h1></body></html>
`,
		"User.alder": `module User

type UserId = brand Text

table User { id: UserId @id, name: Text }
`,
		"Items.alder": `module Items

route Index = GET  "/items" { }
route Add   = POST "/items" { name: Text } requires user

Index.load : Index.Params -> Session -> Query {}
Index.load params session = Query.succeed {}

Index.view : {} -> Session -> Html
Index.view model session = <html><body><h1>items</h1></body></html>

Add.update : Add.Params -> Session -> Task Response
Add.update params session = Task.succeed Response.refresh
`})
	var got []devRoute
	for _, r := range m.Routes {
		if r.Module == "Items.Index" || r.Module == "Items.Add" {
			got = append(got, r)
		}
	}
	want := []devRoute{
		{Method: "GET", Module: "Items.Index", Path: "/items"},
		{Method: "POST", Module: "Items.Add", Path: "/items", Requires: "user", Params: []string{"name: Text"}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("routes = %+v, want %+v (all: %+v)", got, want, m.Routes)
	}
}
