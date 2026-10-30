package std

import (
	"testing"

	"alder/rt/html"
)

func TestHtml_ToText(t *testing.T) {
	if got := HtmlToText(HtmlText("a<b")); got != "a&lt;b" {
		t.Errorf("text: %q", got)
	}
	if got := HtmlToText(HtmlFragment([]html.Node{HtmlUnsafe("<p>"), HtmlText("x")})); got != "<p>x" {
		t.Errorf("fragment: %q", got)
	}
	n := html.Raw("<!doctype html>\n<p>x</p>")
	if got := HtmlToText(n); got != string(FileHtml(n).Body) {
		t.Errorf("File.html: %q", got)
	}
}
