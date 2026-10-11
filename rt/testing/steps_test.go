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

// Doc's `==` ignores the page's parameters in message URLs on both sides; Test.doc renders a
// view and parses it back; Show prints the markup.
func TestDoc_Equal(t *testing.T) {
	held := mustDoc(t, `<main><button class="x" data-on:click="@post('/_m/P/T?id=a&amp;_p.filter=Open')">t</button><p>1 shown</p></main>`)
	view := ViewDoc(html.El("main", nil,
		html.El("button", []html.Attr{html.A("class", "x"), html.A("data-on:click", "@post('/_m/P/T?id=a')")}, html.Text("t")),
		html.El("p", nil, html.Text("1"), html.Text(" shown"))))
	if !rt.Eq(held, view) {
		t.Fatalf("held %s\nview %s", held, view)
	}
	other := mustDoc(t, `<main><button class="x" data-on:click="@post('/_m/P/T?id=b')">t</button><p>1 shown</p></main>`)
	if rt.Eq(held, other) {
		t.Fatal("a message's own parameter is compared")
	}
	if rt.Key(held) != rt.Key(view) {
		t.Fatal("keys differ")
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

// Answer's `==` compares the payload its constructor names, a page as its URL; Show prints it
// as Alder; a FormRejected's list converts both ways.
func TestAnswer(t *testing.T) {
	built := Answer{Tag: Redirected, Page: rt.Page{Path: "/", Query: []rt.KV{{Key: "filter", Value: "All", Omit: true}}}}
	read := Answer{Tag: Redirected, Page: rt.Page{Path: "/"}, Text: "ignored"}
	if !rt.Eq(built, read) || rt.Eq(built, Answer{Tag: Redirected, Page: rt.Page{Path: "/login"}}) || rt.Eq(built, Answer{Tag: Shown}) {
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
		{Tag: RequestFailed, Text: "boom"}: `RequestFailed "boom"`, {Tag: Rerendered}: "Rerendered", {Tag: PageMissing}: "PageMissing",
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
	form, err := html.Parse(`<form action="/_m/M/Save"><input name="title"><select name="kind"></select><textarea name="note"></textarea><input type="checkbox" name="done" value="True"><button>Save</button></form>`)
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

func TestOffered(t *testing.T) {
	urls := []string{"/_m/M/Toggle?id=a&_p.f=x", "/_m/M/Toggle?id=b", "/_m/M/Toggle?id=a", "/_m/M/Other?id=c", "/_m/M/Rename?id=a&x=1", "/_m/M/Rename?id=b&x=2"}
	for _, c := range []struct{ msg, want string }{
		{"/_m/M/Toggle?id=z", "Toggle for id: a, b"},
		{"/_m/M/Rename?id=z&x=0", "Rename for id: a, x: 1; id: b, x: 2"},
		{"/_m/M/Delete?id=z", ""},
	} {
		if got := Offered(c.msg, urls); got != c.want {
			t.Errorf("Offered(%s) = %q, want %q", c.msg, got, c.want)
		}
	}
}
