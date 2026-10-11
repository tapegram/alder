package html

import "testing"

// WithoutPageParams drops the `_p.` keys of message URLs (action, formaction, a data-on:*
// @post) and nothing else: a page rendered with its parameters equals the view rendered without.
func TestWithoutPageParams(t *testing.T) {
	with := `<div><form action="/_m/P/Save?id=1&amp;_p.filter=Active" data-on:submit="@post('/_m/P/Save?id=1&amp;_p.filter=Active', {contentType: 'form'})"><button formaction="/_m/P/Del?_p.filter=Active">x</button></form>` +
		`<a href="/?_p.filter=Active">keep</a><span data-on:click="@post('/_m/P/T?_p.a=1&amp;_p.b=2')">t</span></div>`
	without := `<div><form action="/_m/P/Save?id=1" data-on:submit="@post('/_m/P/Save?id=1', {contentType: 'form'})"><button formaction="/_m/P/Del">x</button></form>` +
		`<a href="/?_p.filter=Active">keep</a><span data-on:click="@post('/_m/P/T')">t</span></div>`
	a, err := Parse(with)
	if err != nil {
		t.Fatal(err)
	}
	b, err := Parse(without)
	if err != nil {
		t.Fatal(err)
	}
	if a.Equal(b) {
		t.Fatal("the trees should differ before")
	}
	if !WithoutPageParams(a).Equal(WithoutPageParams(b)) {
		t.Fatalf("after:\n%s\n%s", Render(WithoutPageParams(a)), Render(WithoutPageParams(b)))
	}
	if got := Render(WithoutPageParams(a)); got != Render(b) {
		t.Fatalf("got %s\nwant %s", got, Render(b))
	}
	other, _ := Parse(`<div><a href="/?_p.filter=Done">keep</a></div>`)
	if WithoutPageParams(other).Equal(WithoutPageParams(b)) {
		t.Fatal("an href is not a message URL")
	}
}
