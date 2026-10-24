package generate

import (
	"strings"
	"testing"

	"alder/internal/syntax"
)

// parseModule parses src, a module named module.
func parseModule(t *testing.T, module, src string) *syntax.Module {
	t.Helper()
	m, ds := syntax.ParseFile(module+".alder", []byte(src))
	for _, d := range ds {
		if d.Severity == "error" {
			t.Fatalf("parse: %s", d.Message)
		}
	}
	return m
}

// addImportsTo parses src as module `module` and runs addImports on it with want, returning
// the edited source and the qualifiers.
func addImportsTo(t *testing.T, p *Program, module, src string, want []string) (string, map[string]string) {
	t.Helper()
	m := parseModule(t, module, src)
	e := newSrcEdit(src)
	quals := addImports(e, p, m, want)
	return e.result(), quals
}

func stdProgram() *Program { return &Program{Std: map[string]bool{"Text": true, "List": true}} }

// TestAddImports_Canonical (design rule 9): adding Admin.Layout to a file that imports Layout
// writes the alias, and says which qualifier each import has.
func TestAddImports_Canonical(t *testing.T) {
	src := "module Home\n\nimport Layout\n\nview = <Layout.Page>x</Layout.Page>\n"
	got, quals := addImportsTo(t, stdProgram(), "Home", src, []string{"Admin.Layout"})
	if !strings.Contains(got, "import Admin.Layout as AdminLayout\n") || !strings.Contains(got, "import Layout\n") {
		t.Errorf("imports:\n%s", got)
	}
	if quals["Admin.Layout"] != "AdminLayout" || quals["Layout"] != "Layout" {
		t.Errorf("quals = %v", quals)
	}
}

// TestAddImports_RequalifiesExisting (rule 9): an addition that changes an existing import's
// qualifier rewrites its line and its references.
func TestAddImports_RequalifiesExisting(t *testing.T) {
	src := "module Home\n\nimport Admin.Posts.Index\n\nx = Index.x\n"
	got, quals := addImportsTo(t, stdProgram(), "Home", src, []string{"Posts.Index"})
	want := "module Home\n\nimport Admin.Posts.Index as AdminPostsIndex\nimport Posts.Index as PostsIndex\n\nx = AdminPostsIndex.x\n"
	if got != want {
		t.Errorf("got:\n%s\nwant:\n%s", got, want)
	}
	if quals["Admin.Posts.Index"] != "AdminPostsIndex" || quals["Posts.Index"] != "PostsIndex" {
		t.Errorf("quals = %v", quals)
	}
}

// TestAddImports_FullPathText: text written with a new import's full path (as the generators
// write it) comes out with its qualifier; a page value keeps its full path.
func TestAddImports_FullPathText(t *testing.T) {
	src := "module Home\n\nx = 1\n"
	m := parseModule(t, "Home", src)
	e := newSrcEdit(src)
	e.appendText("y = Admin.Layout.isAdmin z\nz = Admin.Layout {}\nw = \"Admin.Layout.q\" -- Admin.Layout.q")
	addImports(e, stdProgram(), m, []string{"Admin.Layout"})
	got := e.result()
	if !strings.Contains(got, "y = Layout.isAdmin z\n") || !strings.Contains(got, "z = Admin.Layout {}\n") ||
		!strings.Contains(got, "w = \"Admin.Layout.q\" -- Admin.Layout.q\n") {
		t.Errorf("got:\n%s", got)
	}
}

// TestAdmin_ShortQualifiers: the admin's pages write the layout through its qualifier, and
// page values with their full path.
func TestAdmin_ShortQualifiers(t *testing.T) {
	p, err := Load("../../testdata/generate/fixture")
	if err != nil {
		t.Fatal(err)
	}
	edits, err := NewAdmin(p, AdminOptions{Module: "Admin", Path: "/admin"})
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range edits {
		if e.Path != "app/Admin/Post/Index.alder" {
			continue
		}
		for _, w := range []string{"import Admin.Layout\n", "<Layout.Page ", "Layout.isAdmin", "Admin.Post.Show {"} {
			if !strings.Contains(e.Body, w) {
				t.Errorf("missing %q in:\n%s", w, e.Body)
			}
		}
		if strings.Contains(e.Body, "Admin.Layout.") {
			t.Errorf("full path left in:\n%s", e.Body)
		}
		return
	}
	t.Fatal("no Admin.Post.Index page generated")
}

// TestTypeImports_Qualifier: a type written with its module's qualifier imports the module.
func TestTypeImports_Qualifier(t *testing.T) {
	p := &Program{Std: map[string]bool{}, Modules: map[string]*Module{"Table.Todo": {}, "Post": {}}, Order: []string{"Post", "Table.Todo"}}
	s := newImports("Home", p.Std)
	typeImports(s, "Maybe (Todo.Status) Post.PostId", p)
	if got := strings.Join(s.list(), ","); got != "Post,Table.Todo" {
		t.Errorf("imports = %s", got)
	}
}

// TestAddImports_RequalifiesBare: a bare eponymous name follows its import's new qualifier.
func TestAddImports_RequalifiesBare(t *testing.T) {
	src := "module Home\n\nimport Table.Todo\n\nx : Todo\nx = Todo { a: 1 }\ny = Todo.f\n"
	got, _ := addImportsTo(t, stdProgram(), "Home", src, []string{"Admin.Todo"})
	want := "module Home\n\nimport Admin.Todo as AdminTodo\nimport Table.Todo as TableTodo\n\nx : TableTodo\nx = TableTodo { a: 1 }\ny = TableTodo.f\n"
	if got != want {
		t.Errorf("got:\n%s\nwant:\n%s", got, want)
	}
}
