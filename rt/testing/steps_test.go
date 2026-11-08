package rtesting

import (
	"net/url"
	"reflect"
	"strings"
	"testing"

	"alder/rt"
	"alder/rt/html"
	"alder/rt/web"
)

func mustDoc(t *testing.T, s string) Doc {
	t.Helper()
	n, err := html.Parse(s)
	if err != nil {
		t.Fatal(err)
	}
	return DocOf(n)
}

// Doc's `==` compares the trees as their markup shows them (text joined, a route's URL as
// rendered): a page held and the view rendered by Test.doc are one document; Show prints the
// markup.
func TestDoc_Equal(t *testing.T) {
	held := mustDoc(t, `<main><button class="x" data-on:click="@post('/p/a/t')">t</button><p>1 shown</p></main>`)
	view := ViewDoc(html.El("main", nil,
		html.El("button", []html.Attr{html.A("class", "x"), html.A("data-on:click", "@post('/p/a/t')")}, html.Text("t")),
		html.El("p", nil, html.Text("1"), html.Text(" shown"))))
	if !rt.Eq(held, view) {
		t.Fatalf("held %s\nview %s", held, view)
	}
	other := mustDoc(t, `<main><button class="x" data-on:click="@post('/p/b/t')">t</button><p>1 shown</p></main>`)
	if rt.Eq(held, other) {
		t.Fatal("a route's URL is not compared")
	}
	if rt.Key(held) != rt.Key(view) || rt.Key(held) == rt.Key(other) {
		t.Fatal("keys")
	}
	if got := rt.Show(other); !strings.HasPrefix(got, "<main><button") {
		t.Fatalf("Show = %q", got)
	}
}

// The queries: by text (a bad selector matches nothing) and parsed once (Sel).
func TestDoc_Queries(t *testing.T) {
	d := mustDoc(t, `<main><h1> Notes </h1><ul id="n"><li class="done">a <b>x</b></li><li>b</li></ul><a href="/x">l</a></main>`)
	if got := DocText("h1", d); got != rt.Just("Notes") {
		t.Errorf("text %v", got)
	}
	if got := DocTexts("#n li", d); !reflect.DeepEqual(got, []string{"a x", "b"}) {
		t.Errorf("texts %q", got)
	}
	if !DocHas("li.done", d) || DocHas("li.gone", d) || DocCount("ul > li", d) != 2 {
		t.Error("has/count")
	}
	if got := DocAttr("href", "a", d); got != rt.Just("/x") {
		t.Errorf("attr %v", got)
	}
	if DocAttr("title", "a", d).Ok || DocText("p", d).Ok || len(DocTexts("p", d)) != 0 || DocTexts("p", d) == nil {
		t.Error("no match")
	}
	if DocCount("ul >", d) != 0 || DocHas("", d) || DocText("#x y", d).Ok || len(DocTexts("li >", d)) != 0 || DocAttr("href", ">", d).Ok {
		t.Error("a selector that does not parse matches nothing")
	}
	if DocCountSel(Sel("li"), d) != 2 || DocTextSel(Sel("b"), d) != rt.Just("x") {
		t.Error("Sel")
	}
}

// Answer's `==` compares the payload its constructor names, a route as its URL; Show prints it
// as Alder; a FormRejected's list converts both ways.
func TestAnswer(t *testing.T) {
	built := Answer{Tag: Redirected, Page: rt.Route{Path: "/", Query: []rt.KV{{Key: "filter", Value: "All", Omit: true}}}}
	read := Answer{Tag: Redirected, Page: rt.Route{Path: "/"}, Text: "ignored"}
	if !rt.Eq(built, read) || rt.Eq(built, Answer{Tag: Redirected, Page: rt.Route{Path: "/login"}}) || rt.Eq(built, Answer{Tag: Shown}) {
		t.Error("Redirected")
	}
	rej := FormRejectedAnswer([]FieldError{{Field: "title", Message: "must not be empty"}})
	served := Answer{Tag: FormRejected, Errors: []web.FieldError{{Field: "title", Message: "must not be empty"}}}
	if !rt.Eq(rej, served) || rt.Eq(rej, FormRejectedAnswer(nil)) {
		t.Error("FormRejected")
	}
	if !rt.Eq(FormRejectedAnswer(nil), Answer{Tag: FormRejected}) {
		t.Error("an empty FormRejected")
	}
	if rt.Eq(Answer{Tag: RequestFailed, Text: "a"}, Answer{Tag: RequestFailed, Text: "b"}) || !rt.Eq(Answer{Tag: PageMissing}, Answer{Tag: PageMissing, Text: "x"}) {
		t.Error("RequestFailed/PageMissing")
	}
	for a, want := range map[*Answer]string{
		{Tag: Shown}: "Shown", &built: "Redirected /", &served: `FormRejected [{ field: "title", message: "must not be empty" }]`,
		{Tag: RequestFailed, Text: "boom"}: `RequestFailed "boom"`, {Tag: Refreshed}: "Refreshed", {Tag: PageMissing}: "PageMissing",
	} {
		if got := rt.Show(*a); got != want {
			t.Errorf("Show = %q, want %q", got, want)
		}
	}
	if got := rt.Show(rt.Just(Answer{Tag: RequestFailed, Text: "x"})); got != `Just (RequestFailed "x")` {
		t.Errorf("as an argument: %q", got)
	}
	if rt.Key(built) != rt.Key(read) {
		t.Error("keys")
	}
}

func TestRawFieldsAndMsgName(t *testing.T) {
	v, err := RawFields("NewTodo", []string{"title", "due"}, []struct {
		Name  string
		Value string
	}{{"title", ""}, {"due", "x"}, {"title", "b"}})
	if err != nil || !reflect.DeepEqual(v, Fields{Values: url.Values{"title": {"", "b"}, "due": {"x"}}}) {
		t.Fatalf("%v %v", v, err)
	}
	_, err = RawFields("NewTodo", []string{"title"}, []struct {
		Name  string
		Value string
	}{{"colour", "red"}})
	if err == nil || err.Error() != "form NewTodo has no field `colour`" {
		t.Fatalf("%v", err)
	}
	type params struct{ Id string }
	if got := MsgName("Todos.Index.Toggle", params{"a"}); got != `Todos.Index.Toggle { id: "a" }` {
		t.Errorf("MsgName = %q", got)
	}
	if got := MsgName("App.Logout", nil); got != "App.Logout" {
		t.Errorf("MsgName = %q", got)
	}
	if SessionOf[struct{ N int64 }](nil) != (struct{ N int64 }{}) || SessionOf[int64](int64(3)) != 3 {
		t.Error("SessionOf")
	}
	if FormatFloat(1.5) != "1.5" || FormatFloat(1e21) != "1e+21" {
		t.Error("FormatFloat")
	}
}

// TestFieldsFor: a submit sends only the fields the form has an input for; a field it has none
// for is left out when its value is what its absence decodes to (Absent), else the step fails
// as a user could not send it (C1).
func TestFieldsFor(t *testing.T) {
	form, err := html.Parse(`<form action="/save"><input name="title"><select name="kind"></select><textarea name="note"></textarea><input type="checkbox" name="done" value="True"><button>Save</button></form>`)
	if err != nil {
		t.Fatal(err)
	}
	fs := Fields{Values: url.Values{"title": {"a"}, "kind": {"K"}, "note": {""}, "done": {"True"}, "due": {""}}, Absent: []string{"due"}}
	got, err := fs.For(form, "Everything")
	if err != nil || !reflect.DeepEqual(got, url.Values{"title": {"a"}, "kind": {"K"}, "note": {""}, "done": {"True"}}) {
		t.Errorf("For = %v, %v", got, err)
	}
	fs.Values["n"] = []string{"-3"}
	if _, err := fs.For(form, "Everything"); err == nil || err.Error() != "the page's form Everything has no input `n`" {
		t.Errorf("a field without an input: %v", err)
	}
	if _, err := (Fields{Values: url.Values{"n": {"1"}}}).For(form, ""); err == nil || err.Error() != "the page's form has no input `n`" {
		t.Errorf("without the form's name: %v", err)
	}
}

// offered names the parameters the held page requests a route with, matched by the app's route
// pattern of the route's method; routeLabel writes a route with its parameters.
func TestOffered(t *testing.T) {
	route := func(name, method, path string, query ...rt.KV) rt.Route {
		return rt.Route{Name: "M." + name, Method: method, Path: path, Query: query}
	}
	toggle := func(id string) rt.Route { return route("Toggle", "PATCH", "/m/"+id+"/toggle") }
	rename := func(id, x string) rt.Route {
		return route("Rename", "POST", "/m/"+id+"/rename", rt.KV{Key: "x", Value: x})
	}
	var buttons []html.Node
	for _, r := range []rt.Route{toggle("a"), toggle("b"), toggle("a"), route("Other", "PATCH", "/m/c/other"), rename("a", "1"), rename("b", "2")} {
		buttons = append(buttons, html.El("button", []html.Attr{html.RouteAttr("data-on:click", r)}))
	}
	doc, err := html.Parse(html.Render(html.El("main", nil, buttons...)))
	if err != nil {
		t.Fatal(err)
	}
	noop := func(*web.Request) web.Response { return web.Response{} }
	b := &Browser{doc: doc, hasDoc: true, routes: []web.Route{
		{Method: "PATCH", Pattern: "/m/{id}/toggle", Handle: noop},
		{Method: "PATCH", Pattern: "/m/{id}/other", Handle: noop},
		{Method: "POST", Pattern: "/m/{id}/rename", Handle: noop},
		{Method: "DELETE", Pattern: "/m/{id}", Handle: noop},
	}}
	for _, c := range []struct {
		r    rt.Route
		want string
	}{
		{toggle("z"), "Toggle for id: a, b"},
		{rename("z", "0"), "Rename for id: a, x: 1; id: b, x: 2"},
		{route("Delete", "DELETE", "/m/z"), ""},
		{route("Nowhere", "PATCH", "/n/z"), ""},
	} {
		if got := b.offered(c.r, false); got != c.want {
			t.Errorf("offered(%s) = %q, want %q", rt.RouteURL(c.r), got, c.want)
		}
	}
	if got := b.offered(toggle("z"), true); got != "" {
		t.Errorf("a submit counts click buttons: %q", got)
	}
	if got := b.routeLabel(rename("a b", "0")); got != `M.Rename { id: "a b", x: "0" }` {
		t.Errorf("routeLabel = %s", got)
	}
	if got := b.routeLabel(route("Nowhere", "PATCH", "/n/z")); got != "M.Nowhere" {
		t.Errorf("routeLabel without a route = %s", got)
	}
}
