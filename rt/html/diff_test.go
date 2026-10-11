package html

import "testing"

func TestDiff(t *testing.T) {
	page := func(s string) Node {
		n, err := Parse("<html><head><title>T</title></head><body><main>" + s + "</main></body></html>")
		if err != nil {
			t.Fatal(err)
		}
		return n
	}
	for _, c := range []struct {
		a, b                  string
		at, what, left, right string
	}{
		{`<ul id="todos"><li>Milk</li></ul>`, `<ul id="todos"></ul>`,
			"body > main > ul#todos", "left 1 child <li>, right 0", `<ul id="todos"><li>Milk</li></ul>`, `<ul id="todos"></ul>`},
		{`<p class="a">x</p>`, `<p class="b">x</p>`, "body > main > p.a", `attribute class: left "a", right "b"`, `<p class="a">x</p>`, `<p class="b">x</p>`},
		{`<p>x</p><p>y</p>`, `<p>x</p><p>z</p>`, "body > main > p:nth-child(2)", `child 1: left text "y", right text "z"`, `<p>y</p>`, `<p>z</p>`},
		{`<p>x</p>`, `<span>x</span>`, "body > main", "child 1 is <p> on the left, <span> on the right", `<main><p>x</p></main>`, `<main><span>x</span></main>`},
	} {
		d, ok := Diff(page(c.a), page(c.b))
		if !ok || d.At != c.at || d.What != c.what || d.Left != c.left || d.Right != c.right {
			t.Errorf("Diff(%s, %s) = %+v, %v", c.a, c.b, d, ok)
		}
	}
	if d, ok := Diff(page("<p>x</p>"), page("<p>x</p>")); ok {
		t.Errorf("equal pages differ: %+v", d)
	}
	long := page("<p>aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa</p>")
	if d, _ := Diff(long, page("<p>b</p>")); len([]rune(d.Left)) != contextLen {
		t.Errorf("context not cut: %q", d.Left)
	}
}
