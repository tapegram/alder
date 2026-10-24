package generate

import (
	"strings"
	"testing"
)

// userTableModule is a user table in app/Table/User.alder (#20's layout).
const userTableModule = "module Table.User\n\ntype UserId = brand Text\n\ntable User {\n  id: UserId @id,\n  email: Text.Email @unique,\n}\n"

func editAt(edits []Edit, path string) *Edit {
	for i := range edits {
		if edits[i].Path == path {
			return &edits[i]
		}
	}
	return nil
}

// TestNewTableWritesTableFolder (#20): a new table is app/Table/<Name>.alder, module
// Table.<Name>, and the command's text ends with the page that lists it.
func TestNewTableWritesTableFolder(t *testing.T) {
	dir := scaffoldApp(t, nil)
	p, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	edits, err := NewTable(p, "Todo", nil)
	if err != nil {
		t.Fatal(err)
	}
	e := editAt(edits, "app/Table/Todo.alder")
	if e == nil || !e.Create || !strings.HasPrefix(e.Body, "module Table.Todo\n") {
		t.Fatalf("edits: %+v", edits)
	}
	if editAt(edits, "app/Todo.alder") != nil {
		t.Errorf("wrote app/Todo.alder")
	}
	res, err := SpecByID("new table").Run(Sets{"name": {"Todo"}}, App{Dir: dir, Migrate: func(string, string) ([]string, error) { return nil, nil }}, true)
	if err != nil {
		t.Fatal(err)
	}
	if txt := res.RenderText(false); !strings.HasSuffix(txt, "next: alder new page /todos --list Todo\n") {
		t.Errorf("text:\n%s", txt)
	}
}

// TestNewTableRefImportsModule: `@ref=User` imports the user table's module, Table.User.
func TestNewTableRefImportsModule(t *testing.T) {
	dir := scaffoldApp(t, map[string]string{"app/Table/User.alder": userTableModule})
	p, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	f, err := ParseField("owner:User.UserId@ref=User", true)
	if err != nil {
		t.Fatal(err)
	}
	edits, err := NewTable(p, "Todo", []FieldSpec{f})
	if err != nil {
		t.Fatal(err)
	}
	e := editAt(edits, "app/Table/Todo.alder")
	if e == nil || !strings.Contains(e.Body, "\nimport Table.User\n") || strings.Contains(e.Body, "\nimport User\n") {
		t.Fatalf("edits: %+v", edits)
	}
	if res, err := Apply(dir, edits, Options{DryRun: true}); err != nil {
		t.Fatalf("Apply: %v\n%s", err, res.RenderText(true))
	}
}

// TestNewTableRefusesRootModule: a root module of the table's name (types only) is refused:
// the table goes in app/Table/, and a second module ending in the name would be ambiguous.
func TestNewTableRefusesRootModule(t *testing.T) {
	dir := scaffoldApp(t, map[string]string{"app/Todo.alder": "module Todo\n\ntype Status = Open | Done\n"})
	p, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	_, err = NewTable(p, "Todo", nil)
	if err == nil || !strings.Contains(err.Error(), "app/Todo.alder") || !strings.Contains(err.Error(), "app/Table/Todo.alder") {
		t.Fatalf("NewTable: %v", err)
	}
}

// TestTableNamedByName: TableNamed finds a table by its name, in Table.<Name> or (legacy) at
// the root.
func TestTableNamedByName(t *testing.T) {
	dir := scaffoldApp(t, map[string]string{
		"app/Table/Todo.alder": "module Table.Todo\n\ntype TodoId = brand Text\n\ntable Todo {\n  id: TodoId @id,\n}\n",
		"app/Tag.alder":        "module Tag\n\ntype TagId = brand Text\n\ntable Tag {\n  id: TagId @id,\n}\n",
	})
	p, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if tb := p.TableNamed("Todo"); tb == nil || tb.Module != "Table.Todo" {
		t.Errorf("TableNamed(Todo) = %+v", tb)
	}
	if tb := p.TableNamed("Tag"); tb == nil || tb.Module != "Tag" {
		t.Errorf("TableNamed(Tag) = %+v", tb)
	}
	if tb := p.TableNamed("Table.Todo"); tb != nil {
		t.Errorf("TableNamed(Table.Todo) = %+v, want nil (a table is named by its name)", tb)
	}
	if TableModule("Todo") != "Table.Todo" {
		t.Errorf("TableModule(Todo) = %q", TableModule("Todo"))
	}
}

// twoPostsApp has two modules ending in `Post`, Table.Post (a table) and Blog.Post, and a page
// that imports both, so each is written with its canonical alias (`TablePost`, `BlogPost`).
func twoPostsApp(t *testing.T) *Program {
	t.Helper()
	dir := scaffoldApp(t, map[string]string{
		"app/Table/Post.alder": "module Table.Post\n\ntype PostId = brand Text\n\ntable Post {\n  id: PostId @id,\n  title: Text,\n}\n",
		"app/Blog/Post.alder":  "module Blog.Post\n\ntype Key = brand Text\n",
		"app/Notes.alder": "module Notes\n\nimport Blog.Post as BlogPost\nimport Table.Post as TablePost\n\npage \"/notes\" { }\n\n" +
			"type Model = { n: Int }\n\nload : Params -> Session -> Query Model\nload params session = Query.succeed { n: 0 }\n\n" +
			"view : Model -> Session -> Html Msg\nview model session = <al-page><h1>Notes</h1></al-page>\n",
	})
	p, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	return p
}

// TestTypeTextCanonicalQualifiers (#23): a type is written through the qualifier the module
// writes its module with: the alias on a clash of last segments, the last segment otherwise.
func TestTypeTextCanonicalQualifiers(t *testing.T) {
	p := twoPostsApp(t)
	post := p.TableNamed("Post")
	if got := typeText(p, post.IDType, "Notes"); got != "TablePost.PostId" {
		t.Errorf("in Notes: %q, want TablePost.PostId", got)
	}
	if got := typeText(p, post.IDType, "Home"); got != "Post.PostId" {
		t.Errorf("in Home: %q, want Post.PostId", got)
	}
	if got := typeText(p, post.IDType, "Table.Post"); got != "PostId" {
		t.Errorf("in Table.Post: %q, want PostId", got)
	}
}

// TestNewMsgIdOfAliasedTable: a message field typed with the aliased qualifier
// (`TablePost.PostId`) is the table's id, so the test clicks with a row it inserts, not a
// hole, and the field keeps its qualifier. (The test's `Db.insert Post` still writes the
// table by its name, not `TablePost`: generated code does not yet alias a table's name.)
func TestNewMsgIdOfAliasedTable(t *testing.T) {
	p := twoPostsApp(t)
	f, err := ParseField("id:TablePost.PostId", false)
	if err != nil {
		t.Fatal(err)
	}
	edits, err := NewMsg(p, "Notes", "Feature", []FieldSpec{f}, MsgOptions{})
	if err != nil {
		t.Fatal(err)
	}
	e := editAt(edits, "app/Notes.alder")
	if e == nil || !strings.Contains(e.Body, "Test.click (Feature { id: post1 })") || !strings.Contains(e.Body, "msg Feature { id: TablePost.PostId }") {
		t.Fatalf("Notes:\n%+v", e)
	}
}
