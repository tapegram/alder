package generate

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"alder/internal/types"
)

const fixture = "../../testdata/generate/fixture"

// TestLoad_Fixture: the facts a generator reads from the generators' fixture app.
func TestLoad_Fixture(t *testing.T) {
	p, err := Load(fixture)
	if err != nil {
		t.Fatal(err)
	}
	var tables []string
	for _, tb := range p.Tables {
		tables = append(tables, tb.Name)
	}
	if !slices.Equal(tables, []string{"Comment", "Post", "Tag", "User"}) {
		t.Errorf("tables %v", tables)
	}
	post := p.TableNamed("Post")
	if post.Owner != "author" || post.Created != "created" || post.Updated != "updated" || post.IDField != "id" || types.String(post.IDType) != "Post.PostId" {
		t.Errorf("Post: %+v", post)
	}
	kinds := map[string]Kind{}
	for _, c := range post.Columns {
		kinds[c.Name] = c.Kind
	}
	if kinds["title"] != KindText || kinds["status"] != KindUnion || kinds["featured"] != KindBool || kinds["created"] != KindTime || kinds["author"] != KindText {
		t.Errorf("Post's kinds: %v", kinds)
	}
	status := post.Column("status")
	if !slices.Equal(status.Ctors, []string{"Draft", "Published"}) || !status.HasDefault || status.Default != "Draft" || status.Required() {
		t.Errorf("status: %+v", status)
	}
	if a := post.Column("author"); a.Ref != "User" || a.OnDelete != "restrict" || !a.Owner || !a.Required() || a.TypeIn("Home") != "User.UserId" {
		t.Errorf("author: %+v", a)
	}
	if c := p.TableNamed("Comment").Column("post"); c.OnDelete != "cascade" {
		t.Errorf("Comment.post: %+v", c)
	}
	if c := p.TableNamed("Tag").Column("post"); c.OnDelete != "restrict" {
		t.Errorf("Tag.post: %+v", c)
	}
	if l := p.TableNamed("Post").Label(); l.Name != "title" {
		t.Errorf("Post's label: %s", l.Name)
	}
	s := p.Session
	if s == nil || s.User == nil || s.UserTable != "User" || s.Role == nil || !slices.Equal(s.Role.Ctors, []string{"Reader", "Admin"}) {
		t.Errorf("session: %+v", s)
	}
	home := p.PageNamed("Home")
	if home == nil || home.Path != "/" || home.Requires != "user" || !slices.Equal(home.Msgs, []string{"Submit"}) || !slices.Equal(home.Forms, []string{"Write"}) {
		t.Errorf("Home: %+v", home)
	}
	if !p.Layout || !p.Std["Text"] || p.Std["Post"] {
		t.Errorf("Layout %v, Std %v", p.Layout, p.Std)
	}
}

// TestHoleIndex: each W0002 placed in its module, declaration and arm, a test's name quoted, a
// fake's entry as its arm.
func TestHoleIndex(t *testing.T) {
	dir := t.TempDir()
	if err := copyApp(fixture, dir, nil); err != nil {
		t.Fatal(err)
	}
	home := filepath.Join(dir, "app", "Home.alder")
	b, _ := os.ReadFile(home)
	src := strings.Replace(string(b), "    Task.succeed (Redirect (Home {}))\n", "    ?done\n", 1)
	src += "\ntest \"a hole\" = ?verdict\n\ncapability Home {\n  stamp : Int -> Task Text,\n}\n\nfake Home {\n  stamp n = ?stamp\n}\n"
	os.WriteFile(home, []byte(src), 0o644)
	p, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, h := range HoleIndex(p.Prog, p.Diags) {
		got = append(got, h.Address()+" ?"+h.Name+" : "+h.Type)
	}
	want := []string{`Home.update.Submit ?done : Task Nav`, `Home."a hole" ?verdict : Bool`, `Home.Home.stamp ?stamp : Task Text`}
	if !slices.Equal(got, want) {
		t.Errorf("holes:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}

// TestApply_ChangedSinceCopy: a file of the app that changes after Apply copied it (another
// command, an editor) is not written over: the command is refused and writes nothing.
func TestApply_ChangedSinceCopy(t *testing.T) {
	dir := t.TempDir()
	if err := copyApp(fixture, dir, nil); err != nil {
		t.Fatal(err)
	}
	home := filepath.Join(dir, "app", "Home.alder")
	b, _ := os.ReadFile(home)
	edit := []Edit{{Path: "app/Home.alder", Body: string(b) + "\n-- an edit\n"}, {Path: "app/Note.alder", Create: true, Body: "module Note\n\nnote : Int\nnote = 1\n"}}
	defer func() { beforeWrite = nil }()
	for _, c := range []struct {
		name   string
		change func()
	}{
		{"an edited file", func() { os.WriteFile(home, append(b, "\n-- the author's\n"...), 0o644) }},
		{"a created file", func() { os.WriteFile(filepath.Join(dir, "app", "Note.alder"), []byte("module Note\n"), 0o644) }},
	} {
		os.WriteFile(home, b, 0o644)
		os.Remove(filepath.Join(dir, "app", "Note.alder"))
		beforeWrite = c.change
		_, err := Apply(dir, edit, Options{})
		var r *Refusal
		if !errors.As(err, &r) || !strings.Contains(r.Msg, "changed since the command read the app: nothing was written") {
			t.Errorf("%s: %v", c.name, err)
		}
		if now, _ := os.ReadFile(home); strings.Contains(string(now), "-- an edit") {
			t.Errorf("%s: Home was written", c.name)
		}
	}
	beforeWrite = nil
	os.WriteFile(home, b, 0o644)
	os.Remove(filepath.Join(dir, "app", "Note.alder"))
	if _, err := Apply(dir, edit, Options{}); err != nil {
		t.Errorf("unchanged: %v", err)
	}
}
