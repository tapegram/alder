package generate

import (
	"errors"
	"os"
	"path/filepath"
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
