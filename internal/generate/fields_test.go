package generate

import (
	"reflect"
	"strings"
	"testing"
)

// TestParseField: the field syntax of the generators' command lines, and the source each field
// is declared as.
func TestParseField(t *testing.T) {
	for _, c := range []struct {
		arg    string
		table  bool
		want   FieldSpec
		source string
	}{
		{"title:Text.NonEmpty", true, FieldSpec{Name: "title", Type: "Text.NonEmpty"}, "title: Text.NonEmpty"},
		{"done:Bool=False", true, FieldSpec{Name: "done", Type: "Bool", Default: "False"}, "done: Bool = False"},
		{"post:Post.PostId@ref=Post@cascade", true, FieldSpec{Name: "post", Type: "Post.PostId", Attrs: []AttrSpec{{"ref", "Post"}, {"cascade", ""}}}, "post: Post.PostId @ref Post @cascade"},
		{"owner:User.UserId@ref=User@owner", true, FieldSpec{Name: "owner", Type: "User.UserId", Attrs: []AttrSpec{{"ref", "User"}, {"owner", ""}}}, "owner: User.UserId @ref User @owner"},
		{"note:Maybe  Text", true, FieldSpec{Name: "note", Type: "Maybe Text"}, "note: Maybe Text"},
		{"text:Text@renamed=body", true, FieldSpec{Name: "text", Type: "Text", Attrs: []AttrSpec{{"renamed", "body"}}}, `text: Text @renamed "body"`},
		{`kind:Text="a@b"@unique`, true, FieldSpec{Name: "kind", Type: "Text", Default: `"a@b"`, Attrs: []AttrSpec{{"unique", ""}}}, `kind: Text = "a@b" @unique`},
		{"priority:Priority=Normal", false, FieldSpec{Name: "priority", Type: "Priority", Default: "Normal"}, "priority: Priority = Normal"},
	} {
		got, err := ParseField(c.arg, c.table)
		if err != nil {
			t.Errorf("ParseField(%q): %v", c.arg, err)
			continue
		}
		if !reflect.DeepEqual(got, c.want) {
			t.Errorf("ParseField(%q) = %+v, want %+v", c.arg, got, c.want)
		}
		if got.Source() != c.source {
			t.Errorf("ParseField(%q).Source() = %q, want %q", c.arg, got.Source(), c.source)
		}
	}
	for _, c := range []struct {
		arg   string
		table bool
		want  string
	}{
		{"title", true, "`title` is not a field: write `name:Type`"},
		{"Title:Text", true, "`Title` is not a field name"},
		{"type:Text", true, "`type` is not a field name"},
		{"title:", true, "field `title` has no type"},
		{"title:Text=", true, "field `title` has an empty default"},
		{"title:Text;rm", true, "field `title`: `Text;rm` is not a type"},
		{"post:Post.PostId@ref", true, "`@ref` takes an argument: `@ref=Table`"},
		{"title:Text@unique=yes", true, "`@unique` takes no argument"},
		{"title:Text@index", true, "`@index` is not an attribute"},
		{"title:Text@unique", false, "a form field takes no attributes"},
		{"n:Int=1, evil: Text", true, "field `n`: the default `1, evil: Text` is not one expression"},
		{"n:Int=(1", true, "field `n`: the default `(1` is not one expression"},
		{"p:Post.PostId@ref=Post }", true, "`@ref=Post }`: a table is named by a capitalised identifier"},
	} {
		_, err := ParseField(c.arg, c.table)
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("ParseField(%q): %v, want an error containing %q", c.arg, err, c.want)
		}
	}
}

func TestNames(t *testing.T) {
	for in, want := range map[string]string{"passwordHash": "Password hash", "NewPost": "New post", "title": "Title", "Posts.Index": "Posts index", "URLs": "Urls"} {
		if got := Humanize(in); got != want {
			t.Errorf("Humanize(%q) = %q, want %q", in, got, want)
		}
	}
	for in, want := range map[string]string{"post": "posts", "category": "categories", "day": "days", "box": "boxes", "status": "statuses"} {
		if got := Plural(in); got != want {
			t.Errorf("Plural(%q) = %q, want %q", in, got, want)
		}
	}
	for in, want := range map[string]string{"Posts.Index": "Posts", "About": "About", "Admin.UserList": "User list", "Index": "Index"} {
		if got := Title(in); got != want {
			t.Errorf("Title(%q) = %q, want %q", in, got, want)
		}
	}
	for _, s := range []string{"A", "Posts.Index", "A1.B2"} {
		if !IsModuleName(s) {
			t.Errorf("IsModuleName(%q) = false", s)
		}
	}
	for _, s := range []string{"", "a", "Posts.", "Posts.index", "Po-sts"} {
		if IsModuleName(s) {
			t.Errorf("IsModuleName(%q) = true", s)
		}
	}
	if got := Tidy("module A\n\n\n\nx = 1   \n\n"); got != "module A\n\nx = 1\n" {
		t.Errorf("Tidy = %q", got)
	}
	if got := quote("a\r\nb"); got != `"a\r\nb"` {
		t.Errorf("quote CRLF = %s", got)
	}
	if got := quote(`a "b" {c} \d`); got != `"a \"b\" \{c} \\d"` {
		t.Errorf("quote = %s", got)
	}
}
