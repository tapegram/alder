package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The splicing generators accept every layout `alder fmt` writes (#18): a record, a page's
// parameters, a table, a session literal or a silo on one line or over several. Each runs
// repeatedly, on its own formatted output, and the app checks after every step.

// layoutStep runs the alder command args (with stdin) in dir and requires it to succeed and
// leave an app without errors.
func layoutStep(t *testing.T, dir, stdin string, args ...string) {
	t.Helper()
	if code, out, errs := alderIn(t, dir, stdin, args...); code != 0 || strings.Contains(out, "error[") {
		t.Fatalf("alder %s: exit %d\n%s%s", strings.Join(args, " "), code, out, errs)
	}
	if code, out, errs := alderIn(t, dir, "", "check"); code != 0 || strings.Contains(out, "error[") {
		t.Fatalf("alder check after alder %s: exit %d\n%s%s", strings.Join(args, " "), code, out, errs)
	}
}

// rewrite replaces old with new in the app's file rel, which must have it.
func rewrite(t *testing.T, dir, rel, old, new string) {
	t.Helper()
	p := filepath.Join(dir, filepath.FromSlash(rel))
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), old) {
		t.Fatalf("%s has no %q:\n%s", rel, old, b)
	}
	if err := os.WriteFile(p, []byte(strings.Replace(string(b), old, new, 1)), 0o644); err != nil {
		t.Fatal(err)
	}
}

func readApp(t *testing.T, dir, rel string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(rel)))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestLayout_EditDeclAddProp(t *testing.T) {
	t.Parallel()
	dir := genApp(t, "scaffold")
	for _, p := range []string{"a", "b", "c", "d"} { // one line, then over several
		layoutStep(t, dir, "", "edit", "decl", "Layout.Page", "add-prop", p+`:Text=""`)
	}
	if s := readApp(t, dir, "app/Layout.alder"); !strings.Contains(s, `d: Text = ""`) {
		t.Fatalf("no prop d:\n%s", s)
	}
	// written by hand over several lines, one field per line
	dir = genApp(t, "scaffold")
	rewrite(t, dir, "app/Layout.alder", "Page : { title: Text, children: List Html } -> Html", "Page : {\n  title: Text,\n  children: List Html,\n} -> Html")
	layoutStep(t, dir, "", "edit", "decl", "Layout.Page", "add-prop", `x:Text=""`)
}

func TestLayout_EditPageAddParam(t *testing.T) {
	t.Parallel()
	dir := genApp(t, "scaffold")
	layoutStep(t, dir, "", "new", "route", "Search", "/search")
	for _, p := range []string{"query:Text=\"\"", "pageNumber:Int=1", "sortOrder:Text=\"asc\"", "size:Int=20", "dense:Bool=False"} {
		layoutStep(t, dir, "", "edit", "route", "Search", "add-param", p)
	}
	if s := readApp(t, dir, "app/Search.alder"); !strings.Contains(s, "dense: Bool = False") {
		t.Fatalf("no param dense:\n%s", s)
	}
}

// TestLayout_EditSessionAddField: fields added one by one to the session of an app with a login
// (whose Logout test signs in with `Test.login`, no session literal since routes, #160): the
// session is laid out over several lines and the app checks after every step (it used to be
// skipped, leaving an app that does not check).
func TestLayout_EditSessionAddField(t *testing.T) {
	t.Parallel()
	dir := genApp(t, "scaffold")
	layoutStep(t, dir, "", "new", "auth")
	for _, f := range []string{`theme:Text="light"`, `lang:Text="en"`, `tz:Text="UTC"`, `dense:Bool=False`} {
		layoutStep(t, dir, "", "edit", "session", "add-field", f)
	}
	if s := readApp(t, dir, "app/App.alder"); !strings.Contains(s, "  dense: Bool = False,\n}") {
		t.Fatalf("no field dense:\n%s", s)
	}
}

func TestLayout_EditTable(t *testing.T) {
	t.Parallel()
	dir := genApp(t, "scaffold")
	// two fields: `table Tag { id: TagId @id, name: Text }` on one line
	layoutStep(t, dir, "", "new", "table", "Tag", "name:Text", "--no-timestamps")
	if s := readApp(t, dir, "app/Table/Tag.alder"); !strings.Contains(s, "table Tag { id: TagId @id, name: Text }") {
		t.Fatalf("the table is not on one line:\n%s", s)
	}
	layoutStep(t, dir, "", "edit", "table", "Tag", "rename-field", "name", "label")
	rewrite(t, dir, "app/Table/Tag.alder", ` @renamed "name"`, "")
	layoutStep(t, dir, "", "edit", "table", "Tag", "drop-field", "label")
	for _, f := range []string{"a:Int=0", "b:Int=0", "c:Text=\"\"", "d:Bool=False"} { // one line, then over several
		layoutStep(t, dir, "", "edit", "table", "Tag", "add-field", f)
	}
	layoutStep(t, dir, "", "edit", "table", "Tag", "rename-field", "b", "bee")
	rewrite(t, dir, "app/Table/Tag.alder", ` @renamed "b"`, "")
	layoutStep(t, dir, "", "edit", "table", "Tag", "drop-field", "c")
	s := readApp(t, dir, "app/Table/Tag.alder")
	for _, w := range []string{"  a: Int = 0,", "  bee: Int = 0,", "  d: Bool = False,"} {
		if !strings.Contains(s, w) {
			t.Errorf("Tag lacks %q:\n%s", w, s)
		}
	}
	if strings.Contains(s, "c: Text") {
		t.Errorf("c not dropped:\n%s", s)
	}
}

// TestLayout_FormOnMsg: `new form --msg` on a write route whose button `alder fmt` broke over
// several lines (a route line is never wrapped).
func TestLayout_FormOnMsg(t *testing.T) {
	t.Parallel()
	dir := genApp(t, "scaffold")
	layoutStep(t, dir, "", "new", "action", "Home", "Pick", "first:Entry.EntryId", "second:Entry.EntryId", "third:Entry.EntryId", "--method", "PATCH")
	if s := readApp(t, dir, "app/Home.alder"); !strings.Contains(s, "      click={Pick {\n") {
		t.Fatalf("the button is not over several lines:\n%s", s)
	}
	layoutStep(t, dir, "", "new", "form", "Home", "Reason", "why:Text.NonEmpty", "--msg", "Pick")
	s := readApp(t, dir, "app/Home.alder")
	if !strings.Contains(s, "third: Entry.EntryId } form Reason\n") || strings.Contains(s, "click={Pick") {
		t.Fatalf("the form is not on the route, or its button is left:\n%s", s)
	}
}

// TestLayout_SiloOverSeveralLines: a table added to a silo written over several lines.
func TestLayout_SiloOverSeveralLines(t *testing.T) {
	t.Parallel()
	dir := genApp(t, "fixture-silo")
	rewrite(t, dir, "app/App.alder", "silo org { Project, Issue }", "silo org {\n  Project,\n  Issue,\n}")
	if code, out, errs := alderIn(t, dir, "", "new", "silo", "org", "Activity", "--empty"); code != 0 {
		t.Fatalf("new silo: exit %d\n%s%s", code, out, errs)
	}
	if s := readApp(t, dir, "app/App.alder"); !strings.Contains(s, "silo org { Project, Issue, Activity }") {
		t.Fatalf("Activity not in the silo:\n%s", s)
	}
}

// A table in `app/Table/Todo.alder` (module `Table.Todo`, #20) checks and builds: a route module
// imports it, inserts into it and lists it.
func TestBuildTableFolder(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	writeFiles(t, dir, map[string]string{
		"app/App.alder": "module App\n\nsession { n: Int = 0 }\n\nerror : Error -> Html\nerror err = <p>Error</p>\n\nnotFound : Html\nnotFound = <p>Not found</p>\n",
		"app/Table/Todo.alder": `module Table.Todo

type TodoId = brand Text

table Todo {
  id: TodoId @id,
  title: Text,
}
`,
		"app/Home.alder": `module Home

import Table.Todo

route Home = GET  "/"    { }
route Add  = POST "/add" { }

Home.load : Home.Params -> Session -> Query (List Todo)
Home.load params session = Db.from Todo |> Db.orderBy Todo.title.asc |> Db.all

Add.update : Add.Params -> Session -> Task Response
Add.update params session = {
  let _ <- Db.insert Todo { title: "Milk" }
  Task.succeed Response.refresh
}

Home.view : List Todo -> Session -> Html
Home.view todos session = <ul>{List.map (\t -> <li>{t.title}</li>) todos}</ul>
`})
	if code, out, errs := alderIn(t, dir, "", "check"); code != 0 || strings.Contains(out, "error[") {
		t.Fatalf("alder check: exit %d\n%s%s", code, out, errs)
	}
	if code, out, errs := runBuild(t, dir); code != 0 || !strings.HasPrefix(out, "wrote ") {
		t.Fatalf("alder build: exit %d\nstdout: %s\nstderr: %s", code, out, errs)
	}
}
