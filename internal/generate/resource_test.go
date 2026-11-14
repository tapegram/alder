package generate

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func isRefusal(err error) bool {
	var r *Refusal
	return errors.As(err, &r)
}

func loadResourceFixture(t *testing.T, name string) *Program {
	t.Helper()
	p, err := Load("../../testdata/generate/" + name)
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func TestResourceCheck_Defaults(t *testing.T) {
	cases := []struct {
		app, table string
		want       ResourceOptions
	}{
		{"fixture", "Post", ResourceOptions{Read: "owner", Requires: "user", Module: "Posts", Path: "/posts"}},
		{"fixture", "Tag", ResourceOptions{Read: "signed-in", Requires: "user", Module: "Tags", Path: "/tags"}},
		{"fixture-silo", "Activity", ResourceOptions{Read: "owner", Requires: "user, org", Module: "Activities", Path: "/activities"}},
	}
	for _, c := range cases {
		p := loadResourceFixture(t, c.app)
		got, tbl, keys, err := resourceCheck(p, ResourceOptions{Table: c.table})
		if err != nil {
			t.Errorf("%s: %v", c.table, err)
			continue
		}
		c.want.Table = c.table
		if got.Read != c.want.Read || got.Requires != c.want.Requires || got.Module != c.want.Module || got.Path != c.want.Path || got.explicitRequires {
			t.Errorf("%s: got %+v, want %+v", c.table, got, c.want)
		}
		if tbl == nil || tbl.Name != c.table || len(keys) == 0 {
			t.Errorf("%s: table %v keys %v", c.table, tbl, keys)
		}
	}
}

func TestResourceCheck_ExplicitRequires(t *testing.T) {
	p := loadResourceFixture(t, "fixture")
	got, _, _, err := resourceCheck(p, ResourceOptions{Table: "Post", Requires: "user", Read: "all", Module: "Mine.Posts", Path: "/my/posts/"})
	if err != nil {
		t.Fatal(err)
	}
	if !got.explicitRequires || got.Requires != "user" || got.Read != "all" || got.Path != "/my/posts" {
		t.Errorf("got %+v", got)
	}
}

func TestResourceCheck_NewTable(t *testing.T) {
	p := loadResourceFixture(t, "fixture")
	got, tbl, _, err := resourceCheck(p, ResourceOptions{Table: "BlogPost", Fields: []FieldSpec{{Name: "title", Type: "Text"}}})
	if err != nil || tbl != nil || got.Module != "BlogPosts" || got.Path != "/blog-posts" {
		t.Errorf("got %+v %v %v", got, tbl, err)
	}
}

func TestResourceCheck_Refusals(t *testing.T) {
	existing := t.TempDir()
	if err := os.MkdirAll(filepath.Join(existing, "app/Posts"), 0o755); err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name, app string
		o         ResourceOptions
		dir       string
		want      string
	}{
		{"no table", "fixture", ResourceOptions{Table: "Nope"}, "", "there is no table `Nope`: give its fields to create it (`alder new resource Nope title:Text`) or make it first with `alder new table`"},
		{"fields on existing", "fixture", ResourceOptions{Table: "Post", Fields: []FieldSpec{{Name: "x", Type: "Text"}}}, "", "`Post` exists: change it with `alder edit table Post add-field …`, then `alder new resource Post`"},
		{"silo", "fixture-silo", ResourceOptions{Table: "Issue"}, "", "`Issue` lives in the `org` silo: `alder new resource` writes pages over the shared store only (the admin shows siloed tables)"},
		{"dir exists", "fixture", ResourceOptions{Table: "Post"}, existing, "`app/Posts/` exists: `alder regen resource Post` updates it; to generate another, choose another `--module`"},
		{"read owner keyless", "fixture", ResourceOptions{Table: "Tag", Read: "owner"}, "", "`Tag` has no `@owner` column to read by; add one (`alder edit table Tag add-field owner:User.UserId@ref=User@owner`) or use `--read signed-in`"},
		{"read all partitioned", "fixture-silo", ResourceOptions{Table: "Activity", Read: "all"}, "", "rows of `Activity` are partitioned by `org` and read within it; `--read` applies to `@owner` tables"},
		{"requires without key", "fixture-silo", ResourceOptions{Table: "Activity", Requires: "user"}, "", "`Activity` is partitioned by `org`: its pages need `--requires org`"},
		{"bad read", "fixture", ResourceOptions{Table: "Post", Read: "x"}, "", "`--read x` is not `owner`, `signed-in` or `all`"},
		{"bad module", "fixture", ResourceOptions{Table: "Post", Module: "posts"}, "", "`--module posts` is not a module name: write capitalised names joined by dots, such as `Posts`"},
		{"bad path", "fixture", ResourceOptions{Table: "Post", Path: "posts"}, "", "`--path posts` must begin with `/`, such as `/posts`"},
		{"author", "fixture", ResourceOptions{Table: "Comment"}, "", "`author` is required and refers to users: give it a default or make it `Maybe`"},
	}
	for _, c := range cases {
		p := loadResourceFixture(t, c.app)
		if c.dir != "" {
			cp := *p
			cp.Dir = c.dir
			p = &cp
		}
		_, _, _, err := resourceCheck(p, c.o)
		if !isRefusal(err) {
			t.Errorf("%s: want a refusal, got %v", c.name, err)
			continue
		}
		if err.Error() != c.want {
			t.Errorf("%s:\n got %q\nwant %q", c.name, err.Error(), c.want)
		}
	}
}

func TestResourceCheck_NoSessionUser(t *testing.T) {
	p := loadResourceFixture(t, "fixture")
	cp := *p
	cp.Session = nil
	_, _, _, err := resourceCheck(&cp, ResourceOptions{Table: "Post", Read: "all"})
	if !isRefusal(err) || err.Error() != "`Post` is owned (`@owner`): its pages need `--requires user`" {
		t.Errorf("owned: %v", err)
	}
	_, _, _, err = resourceCheck(&cp, ResourceOptions{Table: "Tag", Requires: "user"})
	if !isRefusal(err) {
		t.Errorf("--requires user: %v", err)
	}
	got, _, _, err := resourceCheck(&cp, ResourceOptions{Table: "Tag"})
	if err != nil || got.Read != "all" || got.Requires != "" {
		t.Errorf("keyless default: %+v %v", got, err)
	}
}

func TestResourceDir(t *testing.T) {
	if ResourceDir("Mine.Posts") != "app/Mine/Posts" {
		t.Error(ResourceDir("Mine.Posts"))
	}
}

// TestResource_Command: the header's command records the options regen needs, `--read` always
// and `--requires` only when it was given.
func TestResource_Command(t *testing.T) {
	p := loadResourceFixture(t, "fixture")
	for _, c := range []struct {
		in   ResourceOptions
		want string
	}{
		{ResourceOptions{Table: "Post"}, "alder new resource Post --module Posts --path /posts --read owner"},
		{ResourceOptions{Table: "Post", Requires: "user", Read: "all", Module: "Mine.Posts", Path: "/my/posts"}, "alder new resource Post --module Mine.Posts --path /my/posts --read all --requires user"},
	} {
		o, _, _, err := resourceCheck(p, c.in)
		if err != nil {
			t.Fatal(err)
		}
		if got := o.command(); got != c.want {
			t.Errorf("command = %q, want %q", got, c.want)
		}
	}
}

// TestNewResource_Owner: Post's five files, every write scoped to its owner (Review Focus 1),
// the owner left out of the form; and a route that answers one of their methods and paths is
// refused before anything is written.
func TestNewResource_Owner(t *testing.T) {
	p := loadResourceFixture(t, "fixture")
	edits, notes, err := NewResource(p, ResourceOptions{Table: "Post"})
	if err != nil || len(notes) != 0 {
		t.Fatalf("NewResource: %v, %v", err, notes)
	}
	var paths []string
	body := map[string]string{}
	for _, e := range edits {
		paths = append(paths, e.Path)
		body[e.Path] = e.Body
	}
	if want := []string{"app/Posts/Index.alder", "app/Posts/Show.alder", "app/Posts/New.alder", "app/Posts/Edit.alder", "app/Posts/Delete.alder"}; !slices.Equal(paths, want) {
		t.Fatalf("paths = %v", paths)
	}
	for path, want := range map[string]string{
		"app/Posts/Index.alder":  "Db.mine Post",
		"app/Posts/Show.alder":   "let row <- Db.getMine Post params.id",
		"app/Posts/New.alder":    "let id <- Db.insertMine Post {",
		"app/Posts/Edit.alder":   "let _ <- Db.updateMine Post params.id",
		"app/Posts/Delete.alder": "let _ <- Db.deleteMine Post params.id",
	} {
		if !strings.Contains(body[path], want) {
			t.Errorf("%s has no %q", path, want)
		}
	}
	if form, _, _ := RegionsFor("@resource").Body(body["app/Posts/New.alder"], "form"); strings.Contains(form, "author") {
		t.Errorf("the form edits the owner:\n%s", form)
	}

	p.Routes = append(p.Routes, &Route{Module: "Feed", Name: "Feed", Method: "PUT", Path: "/posts/{postId}"})
	_, _, err = NewResource(p, ResourceOptions{Table: "Post"})
	if !isRefusal(err) || !strings.Contains(err.Error(), "`PUT /posts/{id}` is already answered by `Feed.Feed`") {
		t.Errorf("a clash: %v", err)
	}
}

// TestNewResource_Interim: until the keyless, partitioned and create-from-fields resources land
// (#194 Task 8), a table with no `@owner` is refused, nothing written.
func TestNewResource_Interim(t *testing.T) {
	p := loadResourceFixture(t, "fixture")
	edits, _, err := NewResource(p, ResourceOptions{Table: "Tag"})
	if !isRefusal(err) || !strings.Contains(err.Error(), "writes the pages of an `@owner` table so far") || edits != nil {
		t.Errorf("a keyless table: %v, %d edits", err, len(edits))
	}
}

// TestNewResource_NoEmails: a resource's pages name a user by a column that is not an email
// (the fixture's `name`), under every `--read`; with none, the column referring to users is
// left off Index and Show, and a warning says so.
func TestNewResource_NoEmails(t *testing.T) {
	for _, read := range []string{"owner", "signed-in", "all"} {
		p := loadResourceFixture(t, "fixture")
		edits, notes, err := NewResource(p, ResourceOptions{Table: "Post", Read: read})
		if err != nil || len(notes) != 0 {
			t.Fatalf("--read %s: %v, %v", read, err, notes)
		}
		for _, e := range edits {
			if strings.Contains(e.Body, "r.email") {
				t.Errorf("--read %s: %s names a user by the email", read, e.Path)
			}
			if (strings.HasSuffix(e.Path, "Index.alder") || strings.HasSuffix(e.Path, "Show.alder")) && !strings.Contains(e.Body, "Just r -> r.name") {
				t.Errorf("--read %s: %s does not name the author by the name", read, e.Path)
			}
		}
	}

	p := loadResourceFixture(t, "fixture")
	u := p.TableNamed("User")
	u.Columns = slices.DeleteFunc(u.Columns, func(c *Column) bool { return c.Name == "name" })
	edits, notes, err := NewResource(p, ResourceOptions{Table: "Post", Read: "all"})
	want := "`author` refers to users, whom only an email names: it was left off the list and the page; add a column such as `name: Text` to `User` to show it"
	if err != nil || !slices.Equal(notes, []string{want}) {
		t.Fatalf("no name: %v, %q", err, notes)
	}
	for _, e := range edits {
		if strings.Contains(e.Body, "r.email") {
			t.Errorf("no name: %s names a user by the email", e.Path)
		}
		if (strings.HasSuffix(e.Path, "Index.alder") || strings.HasSuffix(e.Path, "Show.alder")) && (strings.Contains(e.Body, "authorLabel") || strings.Contains(e.Body, "<dt>Author</dt>") || strings.Contains(e.Body, `name: "author"`)) {
			t.Errorf("no name: %s shows the author", e.Path)
		}
	}
}
