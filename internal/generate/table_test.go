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
	p, err := Load(twoPostsDir(t))
	if err != nil {
		t.Fatal(err)
	}
	return p
}

// twoPostsDir is twoPostsApp's app on disk.
func twoPostsDir(t *testing.T) string {
	t.Helper()
	return scaffoldApp(t, map[string]string{
		"app/Table/Post.alder": "module Table.Post\n\ntype PostId = brand Text\n\ntable Post {\n  id: PostId @id,\n  title: Text,\n}\n",
		"app/Blog/Post.alder":  "module Blog.Post\n\ntype Key = brand Text\n",
		"app/Notes.alder": "module Notes\n\nimport Blog.Post as BlogPost\nimport Table.Post as TablePost\n\npage \"/notes\" { }\n\n" +
			"type Model = { n: Int }\n\nload : Params -> Session -> Query Model\nload params session = Query.succeed { n: 0 }\n\n" +
			"view : Model -> Session -> Html Msg\nview model session = <al-page><h1>Notes</h1></al-page>\n",
	})
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
// hole, and the field keeps its qualifier. The test writes the table through the alias too
// (#34): `Db.insert TablePost`, `Db.get TablePost`.
func TestNewMsgIdOfAliasedTable(t *testing.T) {
	dir := twoPostsDir(t)
	p, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	f, err := ParseField("id:TablePost.PostId", false)
	if err != nil {
		t.Fatal(err)
	}
	edits, err := NewMsg(p, "Notes", "Feature", []FieldSpec{f}, MsgOptions{})
	if err != nil {
		t.Fatal(err)
	}
	e := editAt(edits, "app/Notes.alder")
	if e == nil {
		t.Fatalf("no edit of Notes: %+v", edits)
	}
	for _, want := range []string{"Test.click (Feature { id: post1 })", "msg Feature { id: TablePost.PostId }", "Db.insert TablePost {", "Db.get TablePost post1"} {
		if !strings.Contains(e.Body, want) {
			t.Errorf("missing %q in:\n%s", want, e.Body)
		}
	}
	if strings.Contains(e.Body, "Db.insert Post ") || strings.Contains(e.Body, "Table.Post.Post") {
		t.Errorf("writes the table by its name, not its alias:\n%s", e.Body)
	}
	if res, err := Apply(dir, edits, Options{DryRun: true}); err != nil {
		t.Fatalf("Apply: %v\n%s", err, res.RenderText(true))
	}
}

// TestNewFormIntoAliasedTable (#34): a form inserting into a table the page imports under an
// alias writes the table through it, in the arm and in the tests.
func TestNewFormIntoAliasedTable(t *testing.T) {
	dir := twoPostsDir(t)
	p, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	f, err := ParseField("title:Text", false)
	if err != nil {
		t.Fatal(err)
	}
	edits, err := NewForm(p, FormOptions{Page: "Notes", Name: "NewPost", Fields: []FieldSpec{f}, Into: "Post"})
	if err != nil {
		t.Fatal(err)
	}
	e := editAt(edits, "app/Notes.alder")
	if e == nil {
		t.Fatalf("no edit of Notes: %+v", edits)
	}
	for _, want := range []string{"Db.insert TablePost {", "Db.from TablePost |>"} {
		if !strings.Contains(e.Body, want) {
			t.Errorf("missing %q in:\n%s", want, e.Body)
		}
	}
	if strings.Contains(e.Body, "Db.insert Post ") || strings.Contains(e.Body, "Db.from Post ") || strings.Contains(e.Body, "Table.Post.Post") {
		t.Errorf("writes the table by its name, not its alias:\n%s", e.Body)
	}
	if res, err := Apply(dir, edits, Options{DryRun: true}); err != nil {
		t.Fatalf("Apply: %v\n%s", err, res.RenderText(true))
	}
}

// TestNewPageAliasedTable (#34): a page whose module writes the table's module with an alias
// (its own last segment is `Post`, or it imports another module ending in `Post` for a column's
// union) writes the table through the alias: `Db.from TablePost`, `List TablePost`.
func TestNewPageAliasedTable(t *testing.T) {
	unionDir := func(t *testing.T) string {
		return scaffoldApp(t, map[string]string{
			"app/Blog/Post.alder":  "module Blog.Post\n\ntype Kind = Draft | Published\n",
			"app/Table/Post.alder": "module Table.Post\n\nimport Blog.Post as BlogPost\n\ntype PostId = brand Text\n\ntable Post {\n  id: PostId @id,\n  title: Text,\n  kind: BlogPost.Kind,\n}\n",
		})
	}
	refDir := func(t *testing.T) string {
		return scaffoldApp(t, map[string]string{
			"app/Blog/Post.alder":    "module Blog.Post\n\ntype Kind = Draft | Published\n",
			"app/Table/Author.alder": "module Table.Author\n\nimport Blog.Post\n\ntype AuthorId = brand Text\n\ntable Author {\n  id: AuthorId @id,\n  name: Text,\n  kind: Post.Kind,\n}\n",
			"app/Table/Post.alder":   "module Table.Post\n\nimport Table.Author\n\ntype PostId = brand Text\n\ntable Post {\n  id: PostId @id,\n  title: Text,\n  author: Author.AuthorId @ref Author,\n}\n",
		})
	}
	for _, c := range []struct {
		name string
		dir  func(*testing.T) string
		opts PageOptions
		file string
		want []string
	}{
		{"list in a module ending in Post", twoPostsDir, PageOptions{Module: "Admin.Post", Path: "/admin/posts", List: "Post"}, "app/Admin/Post.alder",
			[]string{"\nimport Table.Post as TablePost\n", "List TablePost", "Db.from TablePost", "TablePost.id.desc", "Db.insert TablePost {"}},
		{"show in a module ending in Post", twoPostsDir, PageOptions{Module: "Admin.Post", Path: "/admin/posts", Show: "Post"}, "app/Admin/Post.alder",
			[]string{"\nimport Table.Post as TablePost\n", "row: TablePost", "Db.get TablePost params.id", "Db.insert TablePost {"}},
		{"from in a module ending in Post", twoPostsDir, PageOptions{Module: "Admin.Post", Path: "/admin/posts", From: []string{"Post"}}, "app/Admin/Post.alder",
			[]string{"\nimport Table.Post as TablePost\n", "List TablePost", "Db.from TablePost", "Db.insert TablePost {"}},
		{"list with a union column from another Post module", unionDir, PageOptions{Module: "Posts.Index", Path: "/posts", List: "Post"}, "app/Posts/Index.alder",
			[]string{"\nimport Blog.Post as BlogPost\n", "\nimport Table.Post as TablePost\n", "List TablePost", "Db.from TablePost", "Db.insert TablePost {"}},
		// only the tests' rows (the referenced author's sample) import the other Post module, after the body was written
		{"list whose test rows import another Post module", refDir, PageOptions{Module: "Posts.Index", Path: "/posts", List: "Post"}, "app/Posts/Index.alder",
			[]string{"\nimport Blog.Post as BlogPost\n", "\nimport Table.Post as TablePost\n", "List TablePost", "Db.from TablePost", "TablePost.id.desc", "Db.insert TablePost {", "Db.insert Author {"}},
	} {
		t.Run(c.name, func(t *testing.T) {
			dir := c.dir(t)
			p, err := Load(dir)
			if err != nil {
				t.Fatal(err)
			}
			edits, err := NewPage(p, c.opts)
			if err != nil {
				t.Fatal(err)
			}
			e := editAt(edits, c.file)
			if e == nil {
				t.Fatalf("no edit of %s: %+v", c.file, edits)
			}
			for _, want := range c.want {
				if !strings.Contains(e.Body, want) {
					t.Errorf("missing %q in:\n%s", want, e.Body)
				}
			}
			for _, bad := range []string{"Db.from Post", "Db.get Post ", "Db.insert Post ", "List Post", "row: Post\n", "Table.Post.Post"} {
				if strings.Contains(e.Body, bad) {
					t.Errorf("writes the table by its name, not its alias (%q):\n%s", bad, e.Body)
				}
			}
			if res, err := Apply(dir, edits, Options{DryRun: true}); err != nil {
				t.Fatalf("Apply: %v\n%s", err, res.RenderText(true))
			}
		})
	}
}

// selfRefFields are a tree's columns as written on the command line: a reference to the table
// itself, qualified by its name as for any other table, and one to Post.
func selfRefFields(t *testing.T) []FieldSpec {
	t.Helper()
	var fs []FieldSpec
	for _, arg := range []string{"parent:Maybe Node.NodeId@ref=Node@cascade", "post:Maybe Post.PostId@ref=Post@cascade"} {
		f, err := ParseField(arg, true)
		if err != nil {
			t.Fatal(err)
		}
		fs = append(fs, f)
	}
	return fs
}

// checkSelfRef generates table Node with selfRefFields in the app at dir, checks that the
// edits apply, and returns the body written to path.
func checkSelfRef(t *testing.T, dir, path string) string {
	t.Helper()
	p, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	edits, err := NewTable(p, "Node", selfRefFields(t))
	if r, ok := err.(*Refusal); ok {
		t.Fatalf("NewTable: %v %+v", err, r.Diags)
	} else if err != nil {
		t.Fatalf("NewTable: %v", err)
	}
	e := editAt(edits, path)
	if e == nil {
		t.Fatalf("no edit of %s: %+v", path, edits)
	}
	if res, err := Apply(dir, edits, Options{DryRun: true}); err != nil {
		t.Fatalf("Apply: %v\n%s", err, res.RenderText(true))
	}
	return e.Body
}

// TestNewTableSelfRefUnqualified (#41): in module Table.Node the table's own name is not a
// module, so a reference to itself is written unqualified while one to Post keeps its qualifier.
func TestNewTableSelfRefUnqualified(t *testing.T) {
	dir := scaffoldApp(t, map[string]string{
		"app/Table/Post.alder": "module Table.Post\n\ntype PostId = brand Text\n\ntable Post {\n  id: PostId @id,\n}\n",
	})
	body := checkSelfRef(t, dir, "app/Table/Node.alder")
	for _, want := range []string{"parent: Maybe NodeId @ref Node @cascade", "post: Maybe Post.PostId @ref Post @cascade", "\nimport Table.Post\n"} {
		if !strings.Contains(body, want) {
			t.Errorf("missing %q in:\n%s", want, body)
		}
	}
	if strings.Contains(body, "import Table.Node") {
		t.Errorf("imports itself:\n%s", body)
	}
}

// TestNewTableSelfRefLegacyLayout (#41): beside a table of the layout before #20 (module Post
// at app/Post.alder) the new table still checks.
func TestNewTableSelfRefLegacyLayout(t *testing.T) {
	dir := scaffoldApp(t, map[string]string{
		"app/Post.alder": "module Post\n\ntype PostId = brand Text\n\ntable Post {\n  id: PostId @id,\n}\n",
	})
	body := checkSelfRef(t, dir, "app/Table/Node.alder")
	if !strings.Contains(body, "parent: Maybe NodeId @ref Node @cascade") || !strings.Contains(body, "\nimport Post\n") {
		t.Errorf("body:\n%s", body)
	}
}

// TestNewTableSelfRefTypesModule (#41): a types-only module Table.Node that takes the table
// gets the reference to itself unqualified too.
func TestNewTableSelfRefTypesModule(t *testing.T) {
	dir := scaffoldApp(t, map[string]string{
		"app/Table/Post.alder": "module Table.Post\n\ntype PostId = brand Text\n\ntable Post {\n  id: PostId @id,\n}\n",
		"app/Table/Node.alder": "module Table.Node\n\ntype Kind = A | B\n",
	})
	body := checkSelfRef(t, dir, "app/Table/Node.alder")
	for _, want := range []string{"type Kind = A | B", "parent: Maybe NodeId @ref Node @cascade", "\nimport Table.Post\n"} {
		if !strings.Contains(body, want) {
			t.Errorf("missing %q in:\n%s", want, body)
		}
	}
}
