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
// as Alder; a Rejected's list converts both ways.
func TestAnswer(t *testing.T) {
	built := Answer{Tag: Redirected, Page: rt.Page{Path: "/", Query: []rt.KV{{Key: "filter", Value: "All", Omit: true}}}}
	read := Answer{Tag: Redirected, Page: rt.Page{Path: "/"}, Text: "ignored"}
	if !rt.Eq(built, read) || rt.Eq(built, Answer{Tag: Redirected, Page: rt.Page{Path: "/login"}}) || rt.Eq(built, Answer{Tag: Shown}) {
		t.Error("Redirected")
	}
	rej := RejectedAnswer([]FieldError{{Field: "title", Message: "must not be empty"}})
	served := Answer{Tag: Rejected, Errors: []web.FieldError{{Field: "title", Message: "must not be empty"}}}
	if !rt.Eq(rej, served) || rt.Eq(rej, RejectedAnswer(nil)) {
		t.Error("Rejected")
	}
	if !rt.Eq(RejectedAnswer(nil), Answer{Tag: Rejected}) {
		t.Error("an empty Rejected")
	}
	if rt.Eq(Answer{Tag: Failed, Text: "a"}, Answer{Tag: Failed, Text: "b"}) || !rt.Eq(Answer{Tag: Missing}, Answer{Tag: Missing, Text: "x"}) {
		t.Error("Failed/Missing")
	}
	for a, want := range map[*Answer]string{
		{Tag: Shown}: "Shown", &built: "Redirected /", &served: `Rejected [{ field: "title", message: "must not be empty" }]`,
		{Tag: Failed, Text: "boom"}: `Failed "boom"`, {Tag: Rerendered}: "Rerendered", {Tag: Missing}: "Missing",
	} {
		if got := rt.Show(*a); got != want {
			t.Errorf("Show = %q, want %q", got, want)
		}
	}
	if got := rt.Show(rt.Just(Answer{Tag: Failed, Text: "x"})); got != `Just (Failed "x")` {
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
	if err != nil || !reflect.DeepEqual(v, url.Values{"title": {"", "b"}, "due": {"x"}}) {
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
