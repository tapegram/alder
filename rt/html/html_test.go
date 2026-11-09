package html

import (
	"strings"
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

// The body of a <script> or <style> is the source's literal text (RawText, which only the
// generator writes, from the parser's raw body), rendered unescaped. Nothing else is ever spliced
// into script or style text: an Alder value (Text, or Html.unsafe's Raw), an element, a fragment
// or a RawText holding a closing tag is dropped. Outside <script> and <style>, RawText is text.
func TestHTML_RawTextElements(t *testing.T) {
	body := "\n  if (1 < 2 && s != \"&amp;\") { x('<b>') }\n"
	cases := []struct {
		n    Node
		want string
	}{
		{El("script", nil, RawText(body)), "<script>" + body + "</script>"},
		{El("style", nil, RawText("a > b { content: \"<x>\" }")), `<style>a > b { content: "<x>" }</style>`},
		{El("script", []Attr{A("data-clip", `"</script><b>`)}, RawText("go()")), `<script data-clip="&#34;&lt;/script&gt;&lt;b&gt;">go()</script>`},
		{El("script", nil, Text("alert(1)")), "<script></script>"},
		{El("script", nil, Raw("alert(1)")), "<script></script>"},
		{El("style", nil, Nodes(RawText("a{}")), El("b", nil)), "<style></style>"},
		{El("script", nil, RawText("x = '</script><b>'")), "<script></script>"},
		{El("script", nil, RawText("x = '</STYLE '")), "<script></script>"},
		{El("style", nil, RawText("p{}</Style>")), "<style></style>"},
		{El("p", nil, RawText("a < b")), "<p>a &lt; b</p>"},
	}
	for _, c := range cases {
		if got := Render(c.n); got != c.want {
			t.Errorf("got  %s\nwant %s", got, c.want)
		}
	}
}

// RootID is the id an element delivery patches by (#160): its first root element's, whitespace
// text ignored. Datastar v1.0.0 morphs several top-level elements onto one selector, so several
// roots are allowed and the first one's id is the target.
func TestRootID(t *testing.T) {
	li := func(attrs ...Attr) Node { return El("li", attrs, Text("x")) }
	for _, c := range []struct {
		name string
		n    Node
		id   string
		ok   bool
	}{
		{"one root", li(A("id", "todo-7")), "todo-7", true},
		{"whitespace and fragments around it", Nodes(Text("\n "), Nodes(Nodes(li(A("id", "a")))), Text("\t")), "a", true},
		{"several roots", Nodes(li(A("id", "a")), li(), li(A("id", "b"))), "a", true},
		{"no id", li(), "", false},
		{"an omitted id", li(Attr{Name: "id", Value: "a", Omit: true}), "", false},
		{"a first root without an id", Nodes(li(), li(A("id", "b"))), "", false},
		{"text before the root", Nodes(Text("hi"), li(A("id", "a"))), "", false},
		{"text after the root", Nodes(li(A("id", "a")), Text("hi")), "", false},
		{"raw markup", Raw(`<li id="a">x</li>`), "", false},
		{"nothing", None(), "", false},
		{"an empty id", li(A("id", "")), "", false},
		{"a click button (rendered in a form of its own)", El("button", []Attr{A("id", "b"), RouteAttr("data-on:click", rowRoute("7"))}, Text("Go")), "", false},
		{"a plain button", El("button", []Attr{A("id", "b")}, Text("Go")), "b", true},
	} {
		id, ok := RootID(c.n)
		if id != c.id || ok != c.ok {
			t.Errorf("%s: got %q %v, want %q %v", c.name, id, ok, c.id, c.ok)
		}
	}
}

// busyQ is busy(el, req) as rendered in an attribute: its quotes and arrow escaped.
func busyQ(req string) string {
	return `(window._alderBusy||((e,p)=&gt;p))(el, ` + strings.ReplaceAll(req, "'", "&#39;") + `)`
}

var (
	rowRoute = func(id string) rt.Route {
		return rt.Route{Name: "Todos.Row", Method: "PATCH", Path: "/todos/" + rt.PathSegment(id), Query: []rt.KV{{Key: "tab", Value: "a b"}}}
	}
	saveRoute = rt.Route{Name: "M.Save", Method: "POST", Path: "/save"}
	listRoute = rt.Route{Name: "Todos.Index", Method: "GET", Path: "/todos", Query: []rt.KV{{Key: "filter", Value: "Active"}, rt.QueryKV("q", "", "")}}
)

// `click` on a write route (#160): a form of its own posting to the route's URL, the method in a
// hidden _method field for PUT, PATCH and DELETE (the router honours it on a POST), and with
// JavaScript the route's own method. A path segment's quote cannot end the Datastar string.
func TestRender_ClickWriteRoute(t *testing.T) {
	r := rowRoute("7")
	u := "/todos/7?tab=a+b"
	got := Render(El("button", []Attr{A("type", "button"), RouteAttr("data-on:click", r), A("class", "x")}, Text("Done")))
	want := `<form method="post" action="` + u + `" class="_alder-click" data-on:submit="` + busyQ("@patch('"+u+"', window._alderO?.('form'))") + `">` +
		`<input type="hidden" name="_method" value="PATCH" /><button type="submit" class="x">Done</button></form>`
	if got != want {
		t.Fatalf("PATCH:\n got %s\nwant %s", got, want)
	}
	for _, m := range []string{"PUT", "DELETE"} {
		r := r
		r.Method = m
		got := Render(El("button", []Attr{RouteAttr("data-on:click", r)}, Text("Go")))
		// Datastar sends a DELETE's form in the URL (as a GET's): a DELETE posts, its _method
		// in the body
		verb := strings.ToLower(m)
		if m == "DELETE" {
			verb = "post"
		}
		if !strings.Contains(got, `<input type="hidden" name="_method" value="`+m+`" />`) || !strings.Contains(got, "@"+verb+"(&#39;"+u) {
			t.Errorf("%s: %s", m, got)
		}
	}
	r.Method = "POST"
	got = Render(El("button", []Attr{RouteAttr("data-on:click", r)}, Text("Go")))
	want = `<form method="post" action="` + u + `" class="_alder-click" data-on:submit="` + busyQ("@post('"+u+"', window._alderO?.('form'))") + `"><button type="submit">Go</button></form>`
	if got != want {
		t.Fatalf("POST:\n got %s\nwant %s", got, want)
	}
	// the design system's button: the form goes around the <al-button>
	got = Render(El("al-button", []Attr{A("variant", "ghost")}, El("button", []Attr{RouteAttr("data-on:click", rowRoute("7"))}, Text("Done"))))
	want = `<form method="post" action="` + u + `" class="_alder-click" data-on:submit="` + busyQ("@patch('"+u+"', window._alderO?.('form'))") + `">` +
		`<input type="hidden" name="_method" value="PATCH" /><al-button variant="ghost"><button type="submit">Done</button></al-button></form>`
	if got != want {
		t.Fatalf("al-button:\n got %s\nwant %s", got, want)
	}
	// inside a form: the button names the method itself (name="_method"), so no field of the
	// enclosing form is needed
	got = Render(FormToRoute(saveRoute, nil, El("button", []Attr{RouteAttr("data-on:click", rowRoute("7"))}, Text("Done"))))
	want = `<button type="submit" formaction="` + u + `" formmethod="post" formnovalidate name="_method" value="PATCH" data-on:click__prevent="` + busyQ("@patch('"+u+"', window._alderO?.())") + `">Done</button></form>`
	if !strings.HasSuffix(got, want) || strings.Count(got, "<form") != 1 {
		t.Fatalf("inside a form:\n got %s\nwant …%s", got, want)
	}
	// inside a form, a button with a name of its own (R16): the override comes first and the
	// author's name and value are dropped (keeping them would lose _method without JavaScript)
	got = Render(FormToRoute(saveRoute, nil, El("button", []Attr{A("name", "action"), A("value", "done"), RouteAttr("data-on:click", rowRoute("7"))}, Text("Done"))))
	if strings.Count(got, ` name="`) != 1 || strings.Count(got, ` value="`) != 1 || !strings.Contains(got, `formnovalidate name="_method" value="PATCH"`) {
		t.Fatalf("named, inside a form: %s", got)
	}
	// a POST route's button has no override: its own name and value stay
	post := rowRoute("7")
	post.Method = "POST"
	got = Render(FormToRoute(saveRoute, nil, El("button", []Attr{A("name", "action"), A("value", "done"), RouteAttr("data-on:click", post)}, Text("Done"))))
	if strings.Contains(got, "_method") || !strings.Contains(got, `name="action" value="done"`) {
		t.Fatalf("named POST, inside a form: %s", got)
	}
	// a non-button
	got = Render(El("li", []Attr{RouteAttr("data-on:click", rowRoute("7"))}, Text("row")))
	if want := `<li data-on:click="` + busyQ("@patch('"+u+"', window._alderO?.())") + `">row</li>`; got != want {
		t.Fatalf("a non-button:\n got %s\nwant %s", got, want)
	}
	// a quote in a path segment stays inside the Datastar string
	got = Render(El("button", []Attr{RouteAttr("data-on:click", rowRoute("x');alert(1);('"))}, Text("Go")))
	if strings.Contains(got, "@patch(&#39;/todos/x&#39;") || !strings.Contains(got, "/todos/x%27") {
		t.Fatalf("a quote in the path: %s", got)
	}
	// RootID: a route click button renders in a form of its own too
	if _, ok := RootID(El("button", []Attr{A("id", "b"), RouteAttr("data-on:click", rowRoute("7"))}, Text("Go"))); ok {
		t.Fatal("RootID accepted a route click button")
	}
}

// `click` on a GET route: a form of its own with method="get", so without JavaScript the button
// navigates to the route (its query in hidden fields, as a GET form replaces the action's
// query); with it, an element fetch (window._alderF).
func TestRender_ClickGetRoute(t *testing.T) {
	u := "/todos?filter=Active"
	got := Render(El("button", []Attr{RouteAttr("data-on:click", listRoute)}, Text("More")))
	want := `<form method="get" action="` + u + `" class="_alder-click" data-on:submit="` + busyQ("@get('"+u+"', window._alderF?.())") + `">` +
		`<input type="hidden" name="filter" value="Active" /><button type="submit">More</button></form>`
	if got != want {
		t.Fatalf("outside a form:\n got %s\nwant %s", got, want)
	}
	// inside a form: JavaScript only (type="button", no formaction or formmethod), so without it
	// the button is inert and no field of the enclosing form can reach a URL
	inner := []Node{Input(Control{Kind: "input", Form: "F", Name: "pw", Type: "text", Secret: true}),
		El("button", []Attr{RouteAttr("data-on:click", listRoute), A("class", "x")}, Text("More")),
		El("al-button", nil, El("button", []Attr{RouteAttr("data-on:click", listRoute)}, Text("All")))}
	for _, form := range []Node{FormToRoute(saveRoute, nil, inner...), FormToRoute(rowRoute("7"), nil, inner...)} {
		got = Render(form)
		want = `<button type="button" class="x" data-on:click="` + busyQ("@get('"+u+"', window._alderF?.())") + `">More</button>` +
			`<al-button><button type="button" data-on:click="` + busyQ("@get('"+u+"', window._alderF?.())") + `">All</button></al-button></form>`
		if !strings.HasSuffix(got, want) || strings.Count(got, "<form") != 1 {
			t.Fatalf("inside a form:\n got %s\nwant …%s", got, want)
		}
		for _, bad := range []string{`formmethod="get"`, `method="get"`, "formaction", `type="submit"`} {
			if strings.Contains(got, bad) {
				t.Errorf("inside a form: %s in %s", bad, got)
			}
		}
	}
	got = Render(El("div", []Attr{RouteAttr("data-on:click", listRoute)}))
	if want := `<div data-on:click="` + busyQ("@get('"+u+"', window._alderF?.())") + `"></div>`; got != want {
		t.Fatalf("a non-button:\n got %s\nwant %s", got, want)
	}
	// href to a GET route: its URL (rt.RouteURL)
	if got := Render(El("a", []Attr{RouteAttr("href", listRoute)}, Text("All"))); got != `<a href="/todos?filter=Active">All</a>` {
		t.Fatalf("href: %s", got)
	}
}

// `submit` on <form>: posted to the route's URL, the _method field for PUT/PATCH/DELETE, and
// with JavaScript the route's own method with the fields url-encoded.
func TestRender_SubmitRoute(t *testing.T) {
	u := "/todos/7?tab=a+b"
	// the form's own submit buttons (no type, or type="submit", no click; at any depth) name the
	// method; a type="button" or a click button does not
	got := Render(FormToRoute(rowRoute("7"), []Attr{A("class", "f")}, El("button", nil, Text("Save")),
		El("p", nil, El("al-button", nil, El("button", []Attr{A("type", "submit")}, Text("Go")))), El("button", []Attr{A("type", "button")}, Text("No"))))
	want := `<form method="post" action="` + u + `" data-on:submit="` + busyQ("@patch('"+u+"', window._alderO?.('form')||{contentType: 'form'})") + `" class="f">` +
		`<button name="_method" value="PATCH">Save</button><p><al-button><button type="submit" name="_method" value="PATCH">Go</button></al-button></p><button type="button">No</button></form>`
	if got != want {
		t.Fatalf("PATCH:\n got %s\nwant %s", got, want)
	}
	// no own submit button (Enter in a field submits it): the hidden field, which a click button
	// inside the form would send too (the documented limitation)
	got = Render(FormToRoute(rowRoute("7"), nil, El("input", []Attr{A("name", "title")})))
	want = `<form method="post" action="` + u + `" data-on:submit="` + busyQ("@patch('"+u+"', window._alderO?.('form')||{contentType: 'form'})") + `">` +
		`<input type="hidden" name="_method" value="PATCH" /><input name="title" /></form>`
	if got != want {
		t.Fatalf("fallback:\n got %s\nwant %s", got, want)
	}
	// DELETE: a POST (Datastar would put a DELETE's fields in the URL), the method in the body
	// from the own submit button; PUT keeps its own verb
	del := rowRoute("7")
	del.Method = "DELETE"
	got = Render(FormToRoute(del, nil, El("input", []Attr{A("name", "title")}), El("button", nil, Text("Delete"))))
	want = `<form method="post" action="` + u + `" data-on:submit="` + busyQ("@post('"+u+"', window._alderO?.('form')||{contentType: 'form'})") + `">` +
		`<input name="title" /><button name="_method" value="DELETE">Delete</button></form>`
	if got != want {
		t.Fatalf("DELETE:\n got %s\nwant %s", got, want)
	}
	put := rowRoute("7")
	put.Method = "PUT"
	if got := Render(FormToRoute(put, nil)); !strings.Contains(got, "@put(&#39;"+u) {
		t.Fatalf("PUT: %s", got)
	}
	// a submit button with a name of its own keeps it, and is not an own submit: the hidden
	// field carries the method (two name attributes would drop _method without JavaScript)
	got = Render(FormToRoute(rowRoute("7"), nil, El("button", []Attr{A("name", "action"), A("value", "save")}, Text("Save")),
		El("button", []Attr{A("name", "action"), A("value", "close")}, Text("Close"))))
	want = `<form method="post" action="` + u + `" data-on:submit="` + busyQ("@patch('"+u+"', window._alderO?.('form')||{contentType: 'form'})") + `">` +
		`<input type="hidden" name="_method" value="PATCH" /><button name="action" value="save">Save</button><button name="action" value="close">Close</button></form>`
	if got != want {
		t.Fatalf("named button:\n got %s\nwant %s", got, want)
	}
	// a named button beside an unnamed one: the hidden field alone (the unnamed one naming the
	// method too would send a second _method), both buttons untouched
	got = Render(FormToRoute(rowRoute("7"), nil, El("button", []Attr{A("name", "action"), A("value", "save")}, Text("Save")), El("button", nil, Text("Go"))))
	if strings.Count(got, `name="_method"`) != 1 || !strings.Contains(got, `<input type="hidden" name="_method" value="PATCH" />`) ||
		!strings.Contains(got, `<button name="action" value="save">Save</button><button>Go</button>`) {
		t.Fatalf("named and unnamed buttons: %s", got)
	}
	r := rowRoute("7")
	r.Method = "POST"
	got = Render(FormToRoute(r, nil))
	want = `<form method="post" action="` + u + `" data-on:submit="` + busyQ("@post('"+u+"', window._alderO?.('form')||{contentType: 'form'})") + `"></form>`
	if got != want {
		t.Fatalf("POST:\n got %s\nwant %s", got, want)
	}
}

// Two per-row forms of one PATCH route: the state (Msg = rt.RouteKey of the posted request)
// shows only on the form that sent it.
func TestRender_FormStateByRouteKey(t *testing.T) {
	ctl := Input(Control{Kind: "errors", Form: "Todos.Edit", Name: "title"})
	n := Nodes(FormToRoute(rowRoute("1"), nil, ctl), FormToRoute(rowRoute("2"), nil, ctl))
	st := &FormState{Form: "Todos.Edit", Msg: rt.RouteKey(rowRoute("2")), Errors: map[string][]string{"title": {"is required"}}}
	got := RenderWith(n, st)
	if strings.Count(got, "is required") != 1 {
		t.Fatalf("errors shown %d times: %s", strings.Count(got, "is required"), got)
	}
	if i, j := strings.Index(got, "is required"), strings.Index(got, `action="/todos/2`); j < 0 || i < j {
		t.Fatalf("errors not on row 2's form: %s", got)
	}
}

// Inside a PATCH FormToRoute with its own submit button, each click button sends exactly its own
// _method without JavaScript: a POST route's none, a DELETE route's DELETE.
func TestRender_ClickInRouteFormOwnMethod(t *testing.T) {
	post := rowRoute("7")
	post.Method = "POST"
	del := rowRoute("7")
	del.Method = "DELETE"
	got := Render(FormToRoute(rowRoute("7"), nil,
		El("button", []Attr{RouteAttr("data-on:click", post)}, Text("P")),
		El("button", []Attr{RouteAttr("data-on:click", del)}, Text("D")),
		El("button", nil, Text("Save"))))
	if strings.Contains(got, `type="hidden"`) {
		t.Fatalf("a hidden _method field: %s", got)
	}
	for label, want := range map[string]int{"P": 0, "D": 1, "Save": 1} {
		i := strings.Index(got, ">"+label+"</button>")
		start := strings.LastIndex(got[:i], "<button")
		tag := got[start:i]
		if n := strings.Count(tag, `name="_method"`); n != want {
			t.Errorf("%s: %d _method, want %d: %s", label, n, want, tag)
		}
	}
	if !strings.Contains(got, `name="_method" value="DELETE"`) || !strings.Contains(got, `<button name="_method" value="PATCH">Save`) {
		t.Fatalf("own methods: %s", got)
	}
}

// A literal quote in Route.Path (not built by rt.PathSegment, which escapes it) still cannot end
// the Datastar string.
func TestRender_RouteRawQuoteInPath(t *testing.T) {
	r := rt.Route{Method: "DELETE", Path: "/it's"}
	for _, n := range []Node{El("li", []Attr{RouteAttr("data-on:click", r)}), FormToRoute(r, nil)} {
		got := Render(n)
		// a click @delete, a form @post (a DELETE that sends a form posts)
		if !strings.Contains(got, "@delete(&#39;/it%27s&#39;, ") && !strings.Contains(got, "@post(&#39;/it%27s&#39;, ") {
			t.Errorf("quote not escaped: %s", got)
		}
	}
}

// load, intersect, every and input on a GET route (#160): a Datastar trigger fetching the route
// as an element fetch, with no busy wrapper and no attribute besides the requested ones.
func TestRender_RouteLoad(t *testing.T) {
	got := Render(El("div", []Attr{RouteAttr(LoadAttr, listRoute)}))
	want := `<div data-init="@get(&#39;/todos?filter=Active&#39;, window._alderF?.())"></div>`
	if got != want {
		t.Fatalf("got %s\nwant %s", got, want)
	}
}

func TestRender_RouteLoadEvery(t *testing.T) {
	got := Render(El("div", []Attr{RouteAttr(LoadAttr, listRoute), RouteAttr(IntervalPrefix+"5s", listRoute)}))
	v := `@get(&#39;/todos?filter=Active&#39;, window._alderF?.())`
	want := `<div data-init="` + v + `" data-on-interval__duration.5s="` + v + `"></div>`
	if got != want {
		t.Fatalf("got %s\nwant %s", got, want)
	}
}

func TestRender_RouteIntersect(t *testing.T) {
	got := Render(El("div", []Attr{RouteAttr(IntersectAttr, listRoute)}))
	want := `<div data-on-intersect__once="@get(&#39;/todos?filter=Active&#39;, window._alderF?.())"></div>`
	if got != want {
		t.Fatalf("got %s\nwant %s", got, want)
	}
}

func TestRender_RouteInput(t *testing.T) {
	got := Render(El("input", []Attr{RouteInput("filter", listRoute)}))
	want := `<input data-on:input__debounce.300ms="@get(window._alderQ(&#39;/todos?filter=Active&#39;, &#39;filter&#39;, el.value), window._alderF?.())" />`
	if got != want {
		t.Fatalf("got %s\nwant %s", got, want)
	}
}

func TestRender_RouteInputQuote(t *testing.T) {
	r := rt.Route{Name: "Todos.Index", Method: "GET", Path: "/todos", Query: []rt.KV{{Key: "q", Value: "o'neil"}}}
	got := Render(El("input", []Attr{RouteInput("a'b", r)}))
	if !strings.Contains(got, "q=o%27neil") || strings.Contains(got, "o'neil") {
		t.Errorf("the URL's quote: %s", got)
	}
	if !strings.Contains(got, `a\&#39;b`) {
		t.Errorf("the key's quote: %s", got)
	}
	if v := RouteInputValue(r, `a\b`); !strings.Contains(v, `'a\\b'`) {
		t.Errorf("the key's backslash: %s", v)
	}
}

func TestRender_RouteTriggersNoBusy(t *testing.T) {
	for _, a := range []Attr{RouteAttr(LoadAttr, listRoute), RouteAttr(IntersectAttr, listRoute), RouteAttr(IntervalPrefix+"2s", listRoute), RouteInput("filter", listRoute)} {
		if v := a.value(); strings.Contains(v, "_alderBusy") {
			t.Errorf("%s: busy wrapper in %s", a.Name, v)
		}
		got := Render(El("div", []Attr{a}))
		if n := strings.Count(got, `="`); n != 1 {
			t.Errorf("%s: %d attributes in %s", a.Name, n, got)
		}
	}
}
