package selector

import (
	"reflect"
	"testing"
)

func TestParse_EveryForm(t *testing.T) {
	for _, tc := range []struct {
		in   string
		want []Step
	}{
		{"li", []Step{{Comb: Descendant, Tag: "li"}}},
		{"#summary", []Step{{Comb: Descendant, ID: "summary"}}},
		{".done", []Step{{Comb: Descendant, Classes: []string{"done"}}}},
		{"[data-errors]", []Step{{Comb: Descendant, Attrs: []Attr{{Name: "data-errors"}}}}},
		{"[data-errors=title]", []Step{{Comb: Descendant, Attrs: []Attr{{Name: "data-errors", Value: "title", HasValue: true}}}}},
		{`[href="/a b"]`, []Step{{Comb: Descendant, Attrs: []Attr{{Name: "href", Value: "/a b", HasValue: true}}}}},
		{`[title='x']`, []Step{{Comb: Descendant, Attrs: []Attr{{Name: "title", Value: "x", HasValue: true}}}}},
		{"[data-on:click]", []Step{{Comb: Descendant, Attrs: []Attr{{Name: "data-on:click"}}}}},
		{"li.done.big#x[a][b=c]", []Step{{Comb: Descendant, Tag: "li", ID: "x", Classes: []string{"done", "big"}, Attrs: []Attr{{Name: "a"}, {Name: "b", Value: "c", HasValue: true}}}}},
		{"ul li", []Step{{Comb: Descendant, Tag: "ul"}, {Comb: Descendant, Tag: "li"}}},
		{"ul > li", []Step{{Comb: Descendant, Tag: "ul"}, {Comb: Child, Tag: "li"}}},
		{"ul>li", []Step{{Comb: Descendant, Tag: "ul"}, {Comb: Child, Tag: "li"}}},
		{"  main  ul.todos >  li  a ", []Step{{Comb: Descendant, Tag: "main"}, {Comb: Descendant, Tag: "ul", Classes: []string{"todos"}}, {Comb: Child, Tag: "li"}, {Comb: Descendant, Tag: "a"}}},
		{"my-widget", []Step{{Comb: Descendant, Tag: "my-widget"}}},
	} {
		got, err := Parse(tc.in)
		if err != nil {
			t.Errorf("Parse(%q): %v", tc.in, err)
			continue
		}
		if !reflect.DeepEqual(got.Steps, tc.want) {
			t.Errorf("Parse(%q) = %#v, want %#v", tc.in, got.Steps, tc.want)
		}
	}
}

func TestParse_ErrorsNameTheToken(t *testing.T) {
	const sub = " (a selector is a tag, `#id`, `.class`, `[attr]`, `[attr=value]`, combined with spaces or `>`)"
	for _, tc := range []struct{ in, want string }{
		{"#sumary x", "`#sumary x` is not a selector: unexpected `x` after `#sumary`" + sub},
		{"", "`` is not a selector: it is empty" + sub},
		{"   ", "`   ` is not a selector: it is empty" + sub},
		{"LI", "`LI` is not a selector: unexpected `LI` at the start" + sub},
		{"ul + li", "`ul + li` is not a selector: unexpected `+` after `ul`" + sub},
		{"ul ~ li", "`ul ~ li` is not a selector: unexpected `~` after `ul`" + sub},
		{"> li", "`> li` is not a selector: unexpected `>` at the start" + sub},
		{"ul >", "`ul >` is not a selector: unexpected end after `ul >`" + sub},
		{"ul >> li", "`ul >> li` is not a selector: unexpected `>` after `ul >`" + sub},
		{"li:first-child", "`li:first-child` is not a selector: unexpected `:` after `li`" + sub},
		{"*", "`*` is not a selector: unexpected `*` at the start" + sub},
		{"li,p", "`li,p` is not a selector: unexpected `,` after `li`" + sub},
		{"#", "`#` is not a selector: unexpected `#` at the start" + sub},
		{".a.", "`.a.` is not a selector: unexpected `.` after `.a`" + sub},
		{"#a#b", "`#a#b` is not a selector: unexpected `#b` after `#a`" + sub},
		{"[a", "`[a` is not a selector: unexpected end after `[a`" + sub},
		{"[a=]", "`[a=]` is not a selector: unexpected `]` after `[a=`" + sub},
		{`[a="x]`, "`[a=\"x]` is not a selector: unexpected `\"x]` after `[a=`" + sub},
		{"[a~=b]", "`[a~=b]` is not a selector: unexpected `~` after `[a`" + sub},
		{"[]", "`[]` is not a selector: unexpected `]` after `[`" + sub},
		{"lu", "`lu` is not a selector: unexpected `lu` at the start" + sub},
	} {
		_, err := Parse(tc.in)
		if err == nil {
			t.Errorf("Parse(%q) succeeded", tc.in)
			continue
		}
		if err.Error() != tc.want {
			t.Errorf("Parse(%q):\n got %s\nwant %s", tc.in, err, tc.want)
		}
		if _, ok := err.(*Error); !ok {
			t.Errorf("Parse(%q) error is %T", tc.in, err)
		}
	}
}

func TestSelector_String(t *testing.T) {
	for in, want := range map[string]string{
		"ul>li.done":        "ul > li.done",
		"  a  [href]":       `a [href]`,
		`p#x.y[a='b']`:      `p#x.y[a="b"]`,
		"main  ul.todos li": "main ul.todos li",
	} {
		if got := MustParse(in).String(); got != want {
			t.Errorf("String(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestMustParse_Panics(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("no panic")
		}
	}()
	MustParse("ul +")
}
