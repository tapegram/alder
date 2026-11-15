package generate

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
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
		{"read all, scoped ref", "fixture", ResourceOptions{Table: "Tag", Read: "all"}, "", "`Tag` refers to `Post`, whose rows are each user's: pages a visitor reads cannot show them; use `--read signed-in`"},
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
	// --read signed-in is a read that requires a user: refused, not widened to every visitor
	_, _, _, err = resourceCheck(&cp, ResourceOptions{Table: "Tag", Read: "signed-in"})
	if !isRefusal(err) || err.Error() != "`--read signed-in` needs a session with a `user` field, and this app's session has none: `alder new auth` adds one, or use `--read all`" {
		t.Errorf("--read signed-in: %v", err)
	}
	// (without its reference to Post, whose rows are each user's: a visitor's page cannot name them)
	tag := cp.TableNamed("Tag")
	tag.Columns = slices.DeleteFunc(tag.Columns, func(c *Column) bool { return c.Name == "post" })
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

// TestResource_Command: the header's command records the options regen needs, `--read` and `--requires` only when given
// (regen derives the others from the table).
func TestResource_Command(t *testing.T) {
	p := loadResourceFixture(t, "fixture")
	for _, c := range []struct {
		in   ResourceOptions
		want string
	}{
		{ResourceOptions{Table: "Post"}, "alder new resource Post --module Posts --path /posts"},
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
	// an explicit --requires is recorded as its keys, joined by ",": spaced, reordered or
	// missing the user a membership needs, it reads back the same (parseResourceHeader)
	silo := loadResourceFixture(t, "fixture-silo")
	for _, req := range []string{"user, org", "org,user", "org"} {
		o, _, _, err := resourceCheck(silo, ResourceOptions{Table: "Activity", Requires: req})
		if err != nil {
			t.Fatal(err)
		}
		if got, want := o.command(), "alder new resource Activity --module Activities --path /activities --requires user,org"; got != want {
			t.Errorf("--requires %q: command = %q, want %q", req, got, want)
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

// TestNewResource_Partitioned: Activity (`@partition org`) is read and written within the
// session's org only (the Scoped family), every route requiring `user, org`; the partition
// column stays out of the form; no warning.
func TestNewResource_Partitioned(t *testing.T) {
	p := loadResourceFixture(t, "fixture-silo")
	edits, notes, err := NewResource(p, ResourceOptions{Table: "Activity"})
	if err != nil || len(notes) != 0 {
		t.Fatalf("NewResource: %v, %v", err, notes)
	}
	body := map[string]string{}
	for _, e := range edits {
		body[e.Path] = e.Body
	}
	for path, want := range map[string]string{
		"app/Activities/Index.alder":  "Db.scoped Activity",
		"app/Activities/Show.alder":   "let row <- Db.getScoped Activity params.id",
		"app/Activities/New.alder":    "let id <- Db.insertScoped Activity { text: params.form.text }",
		"app/Activities/Edit.alder":   "let _ <- Db.updateScoped Activity params.id",
		"app/Activities/Delete.alder": "let _ <- Db.deleteScoped Activity params.id",
	} {
		if !strings.Contains(body[path], want) || !strings.Contains(body[path], "requires user, org") {
			t.Errorf("%s has no %q or no `requires user, org`", path, want)
		}
	}
	if form, _, _ := RegionsFor("@resource").Body(body["app/Activities/New.alder"], "form"); strings.Contains(form, "org") {
		t.Errorf("the form edits the partition column:\n%s", form)
	}
}

// TestNewResource_Keyless: a table with neither `@owner` nor `@partition` generates, with one
// warning naming who can read and change its rows and the field that scopes it (spec §3). The
// variant of an app with no users is the golden `resource-keyless-no-user`'s.
func TestNewResource_Keyless(t *testing.T) {
	const remedy = " To scope it: alder edit table Tag add-field owner:User.UserId@ref=User@owner, then alder regen resource Tag"
	for _, c := range []struct {
		name, read, want string
	}{
		{"signed-in", "", "`Tag` has no `@owner`: every signed-in user can read and change every row." + remedy},
	} {
		p := loadResourceFixture(t, "fixture")
		edits, notes, err := NewResource(p, ResourceOptions{Table: "Tag", Read: c.read})
		if err != nil || len(edits) != 5 || !slices.Equal(notes, []string{c.want}) {
			t.Errorf("%s: %v, %d edits, notes %q", c.name, err, len(edits), notes)
		}
		for _, e := range edits {
			if strings.Contains(e.Body, "Mine ") || strings.Contains(e.Body, "Scoped ") {
				t.Errorf("%s: %s scopes a keyless table's rows", c.name, e.Path)
			}
		}
	}
}

// TestNewResource_Create: fields given for a table that does not exist make it first, as
// `alder new table` would, then its pages from the program that has it (Review Focus 2: a
// multi-word table); a non-owner user reference is left out of the form, with a notice, and
// its insert writes Nothing.
func TestNewResource_Create(t *testing.T) {
	p := loadResourceFixture(t, "fixture")
	var fields []FieldSpec
	for _, w := range []string{"title:Text", "owner:User.UserId@ref=User@owner", "reviewer:Maybe User.UserId@ref=User"} {
		f, err := ParseField(w, true)
		if err != nil {
			t.Fatal(err)
		}
		fields = append(fields, f)
	}
	edits, notes, err := NewResource(p, ResourceOptions{Table: "BlogPost", Fields: fields})
	if err != nil {
		t.Fatal(err)
	}
	var paths []string
	body := map[string]string{}
	for _, e := range edits {
		paths = append(paths, e.Path)
		body[e.Path] = e.Body
	}
	if want := []string{"app/Table/BlogPost.alder", "app/BlogPosts/Index.alder", "app/BlogPosts/Show.alder", "app/BlogPosts/New.alder", "app/BlogPosts/Edit.alder", "app/BlogPosts/Delete.alder"}; !slices.Equal(paths, want) {
		t.Fatalf("paths = %v", paths)
	}
	if want := []string{"`reviewer` refers to users and was left out of the form; add its input by hand"}; !slices.Equal(notes, want) {
		t.Errorf("notes = %q", notes)
	}
	if !strings.Contains(body["app/BlogPosts/Index.alder"], "`alder new resource BlogPost --module BlogPosts --path /blog-posts`") {
		t.Errorf("the header records an option that was not given")
	}
	if !strings.Contains(body["app/BlogPosts/New.alder"], "Db.insertMine BlogPost { title: params.form.title, reviewer: Nothing }") {
		t.Errorf("the insert does not leave the reviewer out:\n%s", body["app/BlogPosts/New.alder"])
	}
	if form, _, _ := RegionsFor("@resource").Body(body["app/BlogPosts/New.alder"], "form"); strings.Contains(form, "reviewer") || strings.Contains(form, "owner") {
		t.Errorf("the form edits a user reference:\n%s", form)
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

// TestResourceHeader_RoundTrip (Review Focus 4): the header records the options regen needs; a
// header written for o reads back as o, with --requires only when it was given.
func TestResourceHeader_RoundTrip(t *testing.T) {
	for _, o := range []ResourceOptions{
		{Table: "Post", Module: "Posts", Path: "/posts"}, // the defaults: no --read, no --requires
		{Table: "Post", Module: "Mine.Posts", Path: "/my/posts", Read: "all", Requires: "user", explicitRead: true, explicitRequires: true},
		{Table: "Activity", Module: "Activities", Path: "/activities", Requires: "user,org", explicitRequires: true},
	} {
		src := "-- generated by `" + o.command() + "`; regions between\n-- `-- @resource begin <name>` / `-- @resource end` are rewritten by `alder regen resource`\nmodule X\n"
		got, ok := parseResourceHeader(src)
		if !ok || !reflect.DeepEqual(got, o) {
			t.Errorf("parseResourceHeader(%q) = %+v, %v; want %+v", o.command(), got, ok, o)
		}
	}
	// a header written before --requires was canonical (with a space) still reads
	if got, ok := parseResourceHeader("-- generated by `alder new resource Activity --module Activities --path /activities --requires user, org`; regions between\n"); !ok || got.Requires != "user, org" {
		t.Errorf("spaced --requires: %+v, %v", got, ok)
	}
	for _, src := range []string{"module X\n", "-- generated by `alder new admin`; regions between\n", ""} {
		if _, ok := parseResourceHeader(src); ok {
			t.Errorf("parseResourceHeader(%q) read a header", src)
		}
	}
}

// TestEdit_RegionMarker: an Edit replaces the regions of its own marker; a stray region of the
// other kind is left alone (an `@admin` region in a resource file, and the reverse).
func TestEdit_RegionMarker(t *testing.T) {
	src := "module M\n-- @resource begin a\nold\n-- @resource end\n-- @admin begin b\nkeep\n-- @admin end\n"
	for _, c := range []struct{ marker, name, want string }{
		{"@resource", "a", "module M\n-- @resource begin a\nnew\n-- @resource end\n-- @admin begin b\nkeep\n-- @admin end\n"},
		{"", "b", "module M\n-- @resource begin a\nold\n-- @resource end\n-- @admin begin b\nnew\n-- @admin end\n"},
	} {
		got, err := replaceEdit(src, Edit{Marker: c.marker, Replace: []Region{{Name: c.name, Body: "new"}}})
		if err != nil || got != c.want {
			t.Errorf("marker %q: got %q, %v; want %q", c.marker, got, err, c.want)
		}
		// the other kind's region is not this edit's to replace
		other := "a"
		if c.name == "a" {
			other = "b"
		}
		if _, err := replaceEdit(src, Edit{Marker: c.marker, Replace: []Region{{Name: other, Body: "x"}}}); err == nil {
			t.Errorf("marker %q replaced region %q of the other kind", c.marker, other)
		}
	}
}
