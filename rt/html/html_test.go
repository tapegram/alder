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
