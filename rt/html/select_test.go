package html

import (
	"strings"
	"testing"
)

const selectDoc = `<html><head><title>T</title><script>var s = "<li>no</li>"</script></head><body>
<main id="m">
  <h1 id="summary" class="title big">2   open
    todos</h1>
  <ul class="todos">
    <li class="done" data-id="1"><span>one</span></li>
    <li data-id="2"><span>two</span><ul class="sub"><li class="done">nested</li></ul></li>
  </ul>
  <form method="post" action="/_m/M/Add"><ul class="errors" data-errors="title"><li>must not be empty</li></ul>
  <input name="title" type="text" required /><input name="done" type="checkbox" value="True" checked /></form>
  <a href="/?page=2" data-on:click="@post('/x')">next</a>
</main></body></html>`

func texts(ns []Node) string {
	var out []string
	for _, n := range ns {
		out = append(out, TextContent(n))
	}
	return strings.Join(out, "|")
}

func TestSelect_EveryForm(t *testing.T) {
	doc, err := Parse(selectDoc)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		sel  string
		want string // the text of each match, joined by |
		n    int
	}{
		{"li", "one|twonested|nested|must not be empty", 4},
		{"#summary", "2 open todos", 1},
		{".done", "one|nested", 2},
		{"h1.title.big", "2 open todos", 1},
		{"h1.title.small", "", 0},
		{"[data-id]", "one|twonested", 2},
		{"[data-id=2]", "twonested", 1},
		{`[data-id="1"]`, "one", 1},
		{"li[data-id].done", "one", 1},
		{"ul li", "one|twonested|nested|must not be empty", 4},
		{"ul.todos > li", "one|twonested", 2},
		{"ul.todos li", "one|twonested|nested", 3},
		{"main > ul > li > span", "one|two", 2},
		{"main li.done", "one|nested", 2},
		{"body > li", "", 0},
		{"html main ul.sub > li", "nested", 1},
		{"ul.errors[data-errors=title] > li", "must not be empty", 1},
		{"[data-on:click]", "next", 1},
		{"input[checked]", "", 1},
		{"#m", "", 1},
		{"title", "T", 1},
		{"script", "", 1},
		{"p", "", 0},
	} {
		got := Select(doc, MustParseSelector(tc.sel))
		if len(got) != tc.n {
			t.Errorf("%s: %d matches, want %d", tc.sel, len(got), tc.n)
			continue
		}
		if tc.want != "" && texts(got) != tc.want {
			t.Errorf("%s: %q, want %q", tc.sel, texts(got), tc.want)
		}
	}
	// the matched node itself counts
	ul := Select(doc, MustParseSelector("ul.sub"))[0]
	if got := Select(ul, MustParseSelector("ul")); len(got) != 1 {
		t.Fatalf("self match = %d", len(got))
	}
	if Select(doc, Selector{}) != nil {
		t.Fatal("empty selector matched")
	}
}

func TestTextContentAndAttrOf(t *testing.T) {
	doc, _ := Parse(selectDoc)
	if got := TextContent(Select(doc, MustParseSelector("head"))[0]); got != "T" {
		t.Fatalf("script text leaked: %q", got)
	}
	a := Select(doc, MustParseSelector("a"))[0]
	if v, ok := AttrOf(a, "href"); !ok || v != "/?page=2" {
		t.Fatalf("href = %q %v", v, ok)
	}
	if _, ok := AttrOf(a, "title"); ok {
		t.Fatal("missing attr found")
	}
	in := Select(doc, MustParseSelector("input[checked]"))[0]
	if v, ok := AttrOf(in, "checked"); !ok || v != "" {
		t.Fatalf("checked = %q %v", v, ok)
	}
	if _, ok := AttrOf(Text("x"), "a"); ok {
		t.Fatal("text has attrs")
	}
	// on a built view (not parsed): omitted attrs are absent, controls are looked through
	v := El("div", []Attr{B("hidden", false), A("id", "d")}, Input(Control{Kind: "errors", Form: "F", Name: "x"}))
	if _, ok := AttrOf(v, "hidden"); ok {
		t.Fatal("omitted attr found")
	}
	if got := Select(v, MustParseSelector("#d > ul.errors[data-errors=x]")); len(got) != 1 {
		t.Fatalf("control not selected: %d", len(got))
	}
}
