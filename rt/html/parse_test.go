package html

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"alder/rt"
)

// goldenNodes are the trees whose rendering the html tests pin (html_test.go, state_test.go),
// rebuilt here: every one must survive Render → Parse → Render unchanged.
func goldenNodes() map[string]Node {
	form := FormTo(rt.MsgRef{Module: "Todos.Index", Msg: "Submit"}, []Attr{A("class", "new")},
		Input(Control{Kind: "errors", Form: "Todos.Index.NewTodo"}),
		Input(Control{Kind: "input", Form: "Todos.Index.NewTodo", Name: "title", Label: "Title", Required: true}),
		Input(Control{Kind: "errors", Form: "Todos.Index.NewTodo", Name: "title"}),
		Input(Control{Kind: "input", Form: "Todos.Index.NewTodo", Name: "done", Type: "checkbox"}),
		Input(Control{Kind: "input", Form: "Todos.Index.NewTodo", Name: "n", Type: "number", Step: "any", Value: "1.5"}),
		Input(Control{Kind: "input", Form: "Todos.Index.NewTodo", Name: "pw", Secret: true, Value: "x"}),
		Input(Control{Kind: "textarea", Form: "Todos.Index.NewTodo", Name: "body", Value: "line 1\nline 2\r\n<tag>"}),
		Input(Control{Kind: "select", Form: "Todos.Index.NewTodo", Name: "colour", Options: []string{"Red", "Green"}}),
		Input(Control{Kind: "select", Form: "Todos.Index.NewTodo", Name: "size", Required: true, Options: []string{"S", "M"}, Value: "M"}),
		El("button", []Attr{MsgAttr("data-on:click", rt.MsgRef{Module: "Todos.Index", Msg: "Clear"})}, Text("Clear")),
		El("button", []Attr{A("type", "submit")}, Text("Add")),
	)
	page := El("html", []Attr{A("lang", "en")},
		El("head", nil, El("meta", []Attr{A("charset", "utf-8")}), El("title", nil, Text("To-dos & more")),
			El("style", nil, RawText("a > b { color: red } p:before { content: \"<x>\" }")),
			El("script", []Attr{A("type", "module"), A("src", "/_alder/datastar.js")})),
		El("body", nil,
			El("h1", []Attr{A("id", "top"), A("title", `a"b<c'd`)}, Text("x < y & z é — ok")),
			Raw("<!-- error: boom -->"),
			El("ul", []Attr{A("class", "todos")},
				El("li", []Attr{A("class", "done")}, Text("one"),
					El("button", []Attr{MsgAttr("data-on:click", rt.MsgRef{Module: "Todos.Index", Msg: "Toggle", Params: []rt.KV{{Key: "id", Value: "a b&c"}}}), A("class", "toggle")}, Text("Toggle"))),
				El("li", nil, Nodes(Text("tw"), Text("o")), None())),
			El("a", []Attr{A("href", "javascript:alert(1)")}, Text("bad")),
			El("a", []Attr{A("href", "/?filter=Active&page=2")}, Text("next")),
			El("input", []Attr{B("disabled", true), B("checked", false), M("aria-current", rt.Nothing[string]()), M("class", rt.Just("x"))}),
			El("br", nil), El("hr", nil), El("img", []Attr{A("src", "/static/a.png"), A("alt", "")}),
			El("div", []Attr{A("data-on:click", "@post('/x')")}),
			form,
		))
	return map[string]Node{
		"escape":   El("p", []Attr{A("title", `a"b<c`)}, Text("x < y & z é"), Raw("<b>raw</b>")),
		"void":     El("br", nil),
		"empty":    El("div", nil),
		"fragment": El("ul", nil, Nodes(El("li", nil, Text("a")), None(), El("li", nil, Text("b'"))), Nodes()),
		"text":     Nodes(Text("x"), Text("y")),
		"cr":       El("p", nil, Text("a\rb")),
		"page":     page,
		"form":     El("main", nil, form),
		"state":    El("main", nil, form), // rendered with state below
	}
}

func TestParse_RoundTripsGoldenPages(t *testing.T) {
	for name, n := range goldenNodes() {
		var st *FormState
		if name == "state" {
			st = &FormState{
				Form: "Todos.Index.NewTodo", Msg: "/_m/Todos.Index/Submit",
				Values:     map[string]string{"title": `<b>"x" & y</b>`, "done": "True", "colour": "Red", "body": "a\r\nb"},
				Errors:     map[string][]string{"title": {"must not be empty", "is < 3 & > 1"}, "due": {"is not a date"}},
				FormError:  "that's \"taken\"",
				PageParams: []rt.KV{{Key: "filter", Value: "Active"}, {Key: "q", Value: "a&b c"}},
			}
		}
		want := RenderWith(n, st)
		p, err := Parse(want)
		if err != nil {
			t.Errorf("%s: Parse: %v", name, err)
			continue
		}
		if got := Render(p); got != want {
			t.Errorf("%s: round trip\n got %s\nwant %s", name, got, want)
		}
	}
}

// TestParse_RoundTripsExamplePages: pages the two examples served (testdata/pages, captured
// from `alder build` binaries of examples/todos and examples/blog, with the router's doctype
// and script tag) parse and render back byte for byte.
func TestParse_RoundTripsExamplePages(t *testing.T) {
	files, _ := filepath.Glob(filepath.Join("testdata", "pages", "*.html"))
	if len(files) == 0 {
		t.Fatal("no captured example pages in testdata/pages")
	}
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		want := string(b)
		p, err := Parse(want)
		if err != nil {
			t.Errorf("%s: %v", f, err)
			continue
		}
		if got := Render(p); got != want {
			t.Errorf("%s: round trip\n got %s\nwant %s", f, got, want)
		}
	}
}

func TestParse_Shapes(t *testing.T) {
	p, err := Parse(`<p class="a">x &amp; <b>y</b></p>`)
	if err != nil {
		t.Fatal(err)
	}
	if p.Tag() != "p" || len(p.Children()) != 2 || TextContent(p) != "x & y" {
		t.Fatalf("parsed %#v", p)
	}
	if v, ok := AttrOf(p, "class"); !ok || v != "a" {
		t.Fatalf("class = %q %v", v, ok)
	}
	if n, _ := Parse(""); Render(n) != "" || n.kind != kNone {
		t.Fatalf("empty = %#v", n)
	}
	if n, _ := Parse("a<b>c</b>"); n.kind != kFrag || len(n.children) != 2 {
		t.Fatalf("two top nodes = %#v", n)
	}
	// lenient: upper case, unquoted and single-quoted values, <br> without a slash, a stray end
	// tag, an unclosed element, a lone '<'
	n, err := Parse(`<DIV Class=x id='y'>a<br>b</span>1 < 2<p>open`)
	if err != nil {
		t.Fatal(err)
	}
	if got := Render(n); got != `<div class="x" id="y">a<br />b1 &lt; 2<p>open</p></div>` {
		t.Fatalf("lenient = %s", got)
	}
	// an end tag closes the elements inside the one it names
	if n, _ := Parse(`<ul><li>a<li>b</ul><p>c</p>`); Render(n) != `<ul><li>a<li>b</li></li></ul><p>c</p>` {
		t.Fatalf("implied close = %s", Render(n))
	}
	for _, bad := range []string{`<p class="x>`, `<p`, `<!-- x`, `<p a=">`} {
		if _, err := Parse(bad); err == nil {
			t.Errorf("Parse(%q) succeeded", bad)
		} else if !strings.HasPrefix(err.Error(), "html: line 1: ") {
			t.Errorf("Parse(%q): %v", bad, err)
		}
	}
	// a doctype and the script survive
	doc := "<!doctype html>\n<html><head><script>if (a < b && c) {}</script></head></html>"
	if n, err := Parse(doc); err != nil || Render(n) != doc {
		t.Fatalf("doctype = %q %v", Render(n), err)
	}
}

func TestNode_Equal(t *testing.T) {
	view := El("ul", []Attr{A("class", "x"), B("hidden", false)}, Nodes(El("li", nil, Text("a"), Text("b"))), None(), Text(""))
	parsed, err := Parse(`<ul class="x"><li>ab</li></ul>`)
	if err != nil {
		t.Fatal(err)
	}
	if !view.Equal(parsed) || !parsed.Equal(view) {
		t.Fatal("a view and its parsed markup differ")
	}
	for _, other := range []string{`<ul class="y"><li>ab</li></ul>`, `<ul class="x"><li>a</li></ul>`, `<ul class="x" id="z"><li>ab</li></ul>`, `<ol class="x"><li>ab</li></ol>`, `<ul class="x"><li>ab</li><li></li></ul>`} {
		o, _ := Parse(other)
		if view.Equal(o) {
			t.Errorf("equal to %s", other)
		}
	}
	// a control and a click button compare as rendered
	c := Input(Control{Kind: "errors", Form: "F", Name: "title"})
	if pc, _ := Parse(Render(c)); !c.Equal(pc) {
		t.Fatal("control")
	}
	b := El("button", []Attr{MsgAttr("data-on:click", rt.MsgRef{Module: "M", Msg: "Go"})}, Text("Go"))
	if pb, _ := Parse(Render(b)); !b.Equal(pb) {
		t.Fatal("click button")
	}
	if !(Node{}).Equal(None()) || !None().Equal(Nodes()) {
		t.Fatal("empty nodes")
	}
}

func TestReplaceAndAppend(t *testing.T) {
	doc, _ := Parse(`<html><head></head><body><p>old</p></body></html>`)
	body, _ := Parse(`<body><p>new</p></body>`)
	got, ok := ReplaceElement(doc, "body", body)
	if !ok || Render(got) != `<html><head></head><body><p>new</p></body></html>` {
		t.Fatalf("replace = %s", Render(got))
	}
	if Render(doc) != `<html><head></head><body><p>old</p></body></html>` {
		t.Fatal("ReplaceElement changed its argument")
	}
	got, ok = AppendTo(got, "body", El("i", nil))
	if !ok || Render(got) != `<html><head></head><body><p>new</p><i></i></body></html>` {
		t.Fatalf("append = %s", Render(got))
	}
	if _, ok := ReplaceElement(got, "main", body); ok {
		t.Fatal("replaced a missing element")
	}
}
