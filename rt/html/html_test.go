package html

import (
	"testing"

	"alder/rt"
)

func TestHTML_EscapeOnce(t *testing.T) {
	n := El("p", []Attr{A("title", `a"b<c`)}, Text("x < y & z é"), Raw("<b>raw</b>"))
	got := Render(n)
	want := `<p title="a&#34;b&lt;c">x &lt; y &amp; z é<b>raw</b></p>`
	if got != want {
		t.Fatalf("got %s", got)
	}
}

func TestHTML_BoolAndMaybeAttrs(t *testing.T) {
	got := Render(El("input", []Attr{B("disabled", true), B("checked", false), M("aria-current", rt.Nothing[string]()), M("class", rt.Just("x"))}))
	want := `<input disabled class="x" />`
	if got != want {
		t.Fatalf("got %s", got)
	}
}

func TestHTML_Void(t *testing.T) {
	if got := Render(El("br", nil)); got != "<br />" {
		t.Fatal(got)
	}
	if got := Render(El("div", nil)); got != "<div></div>" {
		t.Fatal(got)
	}
}

func TestHTML_FragmentsAndNone(t *testing.T) {
	if got := Render(None()); got != "" {
		t.Fatalf("None = %q", got)
	}
	n := El("ul", nil, Nodes(El("li", nil, Text("a")), None(), El("li", nil, Text("b'"))), Nodes())
	want := `<ul><li>a</li><li>b&#39;</li></ul>`
	if got := Render(n); got != want {
		t.Fatalf("got %s", got)
	}
	if got := Render(Nodes(Text("x"), Text("y"))); got != "xy" {
		t.Fatalf("top-level fragment = %q", got)
	}
}

func TestHTML_ZeroNodeRendersNothing(t *testing.T) {
	var n Node
	if got := Render(n); got != "" {
		t.Fatalf("zero Node = %q", got)
	}
}

func TestHTML_Controls(t *testing.T) {
	for _, tc := range []struct {
		c    Control
		want string
	}{
		{Control{Kind: "input", Name: "title", Type: "text", Label: "Title", Required: true},
			`<label>Title<input name="title" type="text" required /></label>`},
		{Control{Kind: "input", Name: "due", Type: "date", Value: "2026-01-02"},
			`<label><input name="due" type="date" value="2026-01-02" /></label>`},
		{Control{Kind: "input", Name: "done", Type: "checkbox", Label: "Done", Value: "True"},
			`<label>Done<input name="done" type="checkbox" value="True" checked /></label>`},
		{Control{Kind: "textarea", Name: "body", Label: "a<b", Value: "x&y"},
			`<label>a&lt;b<textarea name="body">x&amp;y</textarea></label>`},
		{Control{Kind: "select", Name: "color", Required: true, Value: "Green", Options: []string{"Red", "Green"}},
			`<label><select name="color" required><option value="Red">Red</option><option value="Green" selected>Green</option></select></label>`},
		{Control{Kind: "select", Name: "color", Options: []string{"Red"}},
			`<label><select name="color"><option value=""></option><option value="Red">Red</option></select></label>`},
		// options={…}: each choice's value and label; an empty choice first only when Blank
		{Control{Kind: "select", Name: "author", Value: "u2", Choices: Choices([]string{"u1", "u2"}, func(id string) Choice { return Choice{Value: id, Label: "<" + id + ">"} })},
			`<label><select name="author"><option value="u1">&lt;u1&gt;</option><option value="u2" selected>&lt;u2&gt;</option></select></label>`},
		{Control{Kind: "select", Name: "reviewer", Blank: true, Choices: []Choice{{"u1", "Ann"}}},
			`<label><select name="reviewer"><option value=""></option><option value="u1">Ann</option></select></label>`},
		{Control{Kind: "select", Name: "author", Choices: Choices([]string{}, func(id string) Choice { return Choice{id, id} })},
			`<label><select name="author"></select></label>`},
		{Control{Kind: "errors", Name: "title"}, `<ul class="errors" data-errors="title"></ul>`},
		{Control{Kind: "errors"}, `<ul class="errors" data-errors="form"></ul>`},
	} {
		if got := Render(Input(tc.c)); got != tc.want {
			t.Errorf("Input(%+v)\n got %s\nwant %s", tc.c, got, tc.want)
		}
	}
}

func TestHTML_Form(t *testing.T) {
	got := Render(Form("/_m/M/Save?id=1", []Attr{A("class", "f")}, El("button", nil, Text("Go"))))
	want := `<form method="post" action="/_m/M/Save?id=1" data-on:submit="@post(&#39;/_m/M/Save?id=1&#39;, {contentType: &#39;form&#39;})" class="f"><button>Go</button></form>`
	if got != want {
		t.Fatalf("got %s", got)
	}
}

// A message attribute resolves to its URL at render time, carrying the rendered page's
// parameters under the _p. prefix; without a state it has none. (A <button> with a click
// renders as a click button: TestRender_ClickButtons.)
func TestMsgAttr(t *testing.T) {
	m := rt.MsgRef{Module: "M", Msg: "Save", Params: []rt.KV{{Key: "id", Value: "1"}}}
	n := FormTo(m, []Attr{A("class", "f")}, El("span", []Attr{MsgAttr("data-on:click", rt.MsgRef{Module: "App", Msg: "Logout"})}))
	want := `<form method="post" action="/_m/M/Save?id=1" data-on:submit="@post(&#39;/_m/M/Save?id=1&#39;, {contentType: &#39;form&#39;})" class="f"><span data-on:click="@post(&#39;/_m/App/Logout&#39;)"></span></form>`
	if got := Render(n); got != want {
		t.Errorf("Render =\n%s\nwant\n%s", got, want)
	}
	st := &FormState{PageParams: []rt.KV{{Key: "filter", Value: "Active"}, rt.QueryKV("q", "", "")}}
	want = `<form method="post" action="/_m/M/Save?id=1&amp;_p.filter=Active" data-on:submit="@post(&#39;/_m/M/Save?id=1&amp;_p.filter=Active&#39;, {contentType: &#39;form&#39;})" class="f"><span data-on:click="@post(&#39;/_m/App/Logout?_p.filter=Active&#39;)"></span></form>`
	if got := RenderWith(n, st); got != want {
		t.Errorf("RenderWith =\n%s\nwant\n%s", got, want)
	}
}

// A carriage return in text or an attribute value renders as &#13;: no raw "\r" reaches the
// page, or a Datastar patch of it (the reviewer's payload).
func TestHTML_CarriageReturnEscaped(t *testing.T) {
	got := Render(El("p", []Attr{A("title", "a\rb")}, Text("pwn\rdata: selector head\r\n")))
	want := `<p title="a&#13;b">pwn&#13;data: selector head&#13;` + "\n" + `</p>`
	if got != want {
		t.Fatalf("got %q", got)
	}
}

// An href that rt.SafeURL refuses renders as "#".
func TestHTML_UnsafeHrefIsHash(t *testing.T) {
	got := Render(Nodes(El("a", []Attr{A("href", "javascript:alert(1)")}), El("a", []Attr{A("href", "https://ex.com/?a=1&b")}), El("a", []Attr{A("href", "//evil")})))
	if got != `<a href="#"></a><a href="https://ex.com/?a=1&amp;b"></a><a href="#"></a>` {
		t.Fatal(got)
	}
}

// Two Html values are Eq when they render to the same markup (rt.Equaler).
func TestHTML_EqIsRenderEq(t *testing.T) {
	cases := []struct {
		name string
		a, b Node
		want bool
	}{
		{"adjacent text nodes", El("p", nil, Text("a"), Text("b")), El("p", nil, Text("ab")), true},
		{"a fragment vs its children", El("p", nil, Nodes(Text("a"), El("b", nil, Text("c")))), El("p", nil, Text("a"), El("b", nil, Text("c"))), true},
		{"text vs the same raw markup", Text("a & b"), Raw("a &amp; b"), true},
		{"attributes in the same order", El("a", []Attr{A("href", "/x"), A("class", "c")}, Text("x")), El("a", []Attr{A("href", "/x"), A("class", "c")}, Text("x")), true},
		{"attributes in another order", El("a", []Attr{A("class", "c"), A("href", "/x")}), El("a", []Attr{A("href", "/x"), A("class", "c")}), false},
		{"different text", Text("a"), Text("b"), false},
		{"none vs empty text", None(), Text(""), true},
	}
	for _, c := range cases {
		if got := rt.Eq(c.a, c.b); got != c.want {
			t.Errorf("%s: Eq = %v, want %v", c.name, got, c.want)
		}
	}
	if !rt.Eq([]Node{Text("a"), Text("b")}, []Node{Text("a"), Nodes(Text("b"))}) {
		t.Error("a List Html compares its elements by markup")
	}
}

// An Html Dict key is its markup, as `==` is: two nodes that render alike are one key.
func TestHTML_DictKeyIsMarkup(t *testing.T) {
	ab := El("p", nil, Text("ab"))
	split := El("p", nil, Text("a"), Text("b"))
	d := rt.Dict[Node, int64]{}.Insert(ab, 1).Insert(split, 2)
	if d.Len() != 1 {
		t.Fatalf("two entries for one markup: %d", d.Len())
	}
	if v, ok := d.Get(ab); !ok || v != 2 {
		t.Fatalf("Get = %v %v", v, ok)
	}
	if !rt.Eq(rt.Dict[Node, int64]{}.Insert(ab, 1), rt.Dict[Node, int64]{}.Insert(split, 1)) {
		t.Error("one-entry dicts keyed by equal markup are not ==")
	}
	if rt.Key(ab) == rt.Key(El("p", nil, Text("b"))) {
		t.Error("different markup, same key")
	}
}

// TestHTML_Offered: a posted select value is accepted only when one of the options has it, or
// when it is blank and blank is allowed.
func TestHTML_Offered(t *testing.T) {
	cs := []Choice{{"u1", "Ann"}, {"u2", "Bob"}}
	for _, c := range []struct {
		value string
		blank bool
		want  bool
	}{
		{"u1", false, true}, {"u2", true, true}, {"", true, true},
		{"", false, false}, {"u3", true, false}, {"Ann", false, false}, {" u1", false, false}, {"';--", false, false},
	} {
		if got := Offered(c.value, cs, c.blank); got != c.want {
			t.Errorf("Offered(%q, blank %v) = %v, want %v", c.value, c.blank, got, c.want)
		}
	}
}
