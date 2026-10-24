package check

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"alder/internal/diag"
)

// Tables live in `app/Table/<Name>.alder` (module `Table.<Name>`, #20): a table's module ends in
// its name, and table names are unique app-wide.

// checkApp writes files (slash path under a fresh app dir → source) and checks the app.
func checkApp(t *testing.T, files map[string]string) *diag.Bag {
	t.Helper()
	dir := t.TempDir()
	for rel, src := range files {
		p := filepath.Join(dir, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(src), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return runDir(t, dir)
}

// codes is the codes of bag's errors, in order.
func codes(bag *diag.Bag) []string {
	var out []string
	for _, d := range bag.Items {
		if d.Severity == "error" {
			out = append(out, d.Code)
		}
	}
	return out
}

func wantClean(t *testing.T, bag *diag.Bag) {
	t.Helper()
	if bag.HasErrors() {
		t.Fatalf("unexpected:\n%s", render(bag))
	}
}

// wantOne requires exactly one error, with code and a message containing frag.
func wantOne(t *testing.T, bag *diag.Bag, code, frag string) {
	t.Helper()
	if got := codes(bag); len(got) != 1 || got[0] != code || !strings.Contains(render(bag), frag) {
		t.Fatalf("want one %s containing %q, got:\n%s", code, frag, render(bag))
	}
}

const todoTable = "module Table.Todo\n\ntype TodoId = brand Text\n\ntable Todo {\n  id: TodoId @id,\n  title: Text,\n}\n"

const todoHome = `module Home

import Table.Todo

page "/" { }

type Model = { titles: List Text }

load : Params -> Session -> Query Model
load params session = {
  let todos <- Db.from Todo |> Db.orderBy Todo.title.asc |> Db.all
  Query.succeed { titles: List.map (\t -> t.title) todos }
}

msg Add

update : Msg -> Model -> Session -> Task Nav
update msg model session = match msg {
  Add p -> {
    let _ <- Db.insert Todo { title: "x" }
    Task.succeed Rerender
  },
}

view : Model -> Session -> Html Msg
view model session = <p>{Text.fromInt (List.length model.titles)}</p>
`

// appModule is an App module with session fields session.
func appModule(imports, session string) string {
	return "module App\n\n" + imports + "\nsession { " + session + " }\n\nerror : Error -> Html Never\nerror err = <p>Error</p>\n\nnotFound : Html Never\nnotFound = <p>Not found</p>\n"
}

func TestTableInTableFolder(t *testing.T) {
	wantClean(t, checkApp(t, map[string]string{"app/Table/Todo.alder": todoTable, "app/Home.alder": todoHome, "app/App.alder": appModule("", "n: Int = 0")}))
}

func TestTableNested(t *testing.T) {
	wantClean(t, checkApp(t, map[string]string{
		"app/Table/Billing/Invoice.alder":  "module Table.Billing.Invoice\n\ntype InvoiceId = brand Text\n\ntable Invoice {\n  id: InvoiceId @id,\n  total: Int,\n}\n",
		"app/Table/Billing/LineItem.alder": "module Table.Billing.LineItem\n\nimport Table.Billing.Invoice\n\ntype LineItemId = brand Text\n\ntable LineItem {\n  id: LineItemId @id,\n  invoice: Invoice.InvoiceId @ref Invoice,\n}\n",
	}))
}

func TestTableWrongLastSegment(t *testing.T) {
	src := strings.Replace(todoTable, "module Table.Todo", "module Table.Todos", 1)
	wantOne(t, checkApp(t, map[string]string{"app/Table/Todos.alder": src}), diag.TablePlacement, "app/Table/Todo.alder")
}

func TestTableRootStillLegal(t *testing.T) {
	src := strings.Replace(todoTable, "module Table.Todo", "module Todo", 1)
	wantClean(t, checkApp(t, map[string]string{"app/Todo.alder": src}))
}

func TestDuplicateTableName(t *testing.T) {
	inv := "type InvoiceId = brand Text\n\ntable Invoice {\n  id: InvoiceId @id,\n  total: Int,\n}\n"
	wantOne(t, checkApp(t, map[string]string{
		"app/Table/Invoice.alder":         "module Table.Invoice\n\n" + inv,
		"app/Table/Billing/Invoice.alder": "module Table.Billing.Invoice\n\n" + inv,
	}), diag.DuplicateTable, "two tables named Invoice: `Table.Invoice` and `Table.Billing.Invoice`; table names are app-wide")
}

func TestSessionViaTableFolder(t *testing.T) {
	wantClean(t, checkApp(t, map[string]string{
		"app/Table/User.alder": "module Table.User\n\ntype UserId = brand Text\n\ntable User {\n  id: UserId @id,\n  name: Text,\n}\n",
		"app/Table/Org.alder":  "module Table.Org\n\ntype OrgId = brand Text\n\ntable Org {\n  id: OrgId @id,\n  name: Text,\n}\n",
		"app/Table/Membership.alder": `module Table.Membership

import Table.User
import Table.Org

type MembershipId = brand Text

table Membership {
  id: MembershipId @id,
  user: User.UserId @ref User @cascade,
  org: Org.OrgId @ref Org @cascade,
}
`,
		"app/App.alder": appModule("import Table.User\nimport Table.Org\n", "user: Maybe User.UserId, org: Maybe Org.OrgId via Membership"),
	}))
}

// A table's type written bare when its import needs an alias (another import ends in `Todo`)
// is the eponymous type of an imported module, not a misplaced table: E0202 says to write the
// qualifier.
func TestTableTypeThroughAlias(t *testing.T) {
	use := "module Todos\n\nimport Admin.Todo as AdminTodo\nimport Table.Todo as TableTodo\n\nfirst : List Todo -> AdminTodo.Mode -> Int\nfirst todos m = List.length todos\n"
	wantOne(t, checkApp(t, map[string]string{
		"app/Table/Todo.alder": todoTable,
		"app/Admin/Todo.alder": "module Admin.Todo\n\ntype Mode = A | B\n",
		"app/Todos.alder":      use,
	}), "E0202", "write `TableTodo`")
}

// A table declared in another module's file: E0202 at the bare use names the table's own file.
func TestMisplacedTableTypeNamesTableFolder(t *testing.T) {
	post := "module Post\n\ntype CommentId = brand Text\n\ntable Comment {\n  id: CommentId @id,\n  body: Text,\n}\n"
	use := "module Feed\n\nimport Post\n\nfirst : List Comment -> Int\nfirst cs = List.length cs\n"
	bag := checkApp(t, map[string]string{"app/Post.alder": post, "app/Feed.alder": use})
	for _, d := range bag.Items {
		if d.Code == "E0202" && strings.Contains(d.Message, "move it to `app/Table/Comment.alder`") {
			return
		}
	}
	t.Fatalf("want E0202 naming app/Table/Comment.alder, got:\n%s", render(bag))
}

// A table in `Table.*` used without its import is a missing import (E0201), not a misplaced
// table.
func TestTableFolderNotImported(t *testing.T) {
	use := "module Todos\n\ncount : List Todo.TodoId -> Int\ncount ids = List.length ids\n"
	got := render(checkApp(t, map[string]string{"app/Table/Todo.alder": todoTable, "app/Todos.alder": use}))
	if strings.Contains(got, "move it") || !strings.Contains(got, "E0201") {
		t.Fatalf("want a missing-import error, got:\n%s", got)
	}
}

// `import Table.Todo.Todo` names no module; it is not a misplaced table either (as `import
// Todo.Todo` is not, for a table at the root).
func TestTableFolderImportOfTableNotMisplaced(t *testing.T) {
	use := "module Todos\n\nimport Table.Todo.Todo\n\nn : Int\nn = 1\n"
	got := render(checkApp(t, map[string]string{"app/Table/Todo.alder": todoTable, "app/Todos.alder": use}))
	if strings.Contains(got, "move it") || !strings.Contains(got, "there is no `app/Table/Todo/Todo.alder`") {
		t.Fatalf("want no such module, got:\n%s", got)
	}
}
