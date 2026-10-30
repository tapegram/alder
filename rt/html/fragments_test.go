package html

import (
	"reflect"
	"strings"
	"testing"
)

func page(body ...Node) Node {
	return El("html", nil, El("head", nil, El("title", nil, Text("t"))), El("body", nil, body...))
}

func li(id, text string) Node { return El("li", []Attr{A("id", id)}, Text(text)) }

// TestRenderTracked_RegionsInDocumentOrder: every element with an id is recorded, outer before
// inner, with offsets of its markup in Page; ids in <head> are recorded as inert.
func TestRenderTracked_RegionsInDocumentOrder(t *testing.T) {
	n := El("html", nil,
		El("head", nil, El("meta", []Attr{A("id", "m")})),
		El("body", nil,
			El("ul", []Attr{A("id", "rows")}, li("a", "A"), El("li", nil, Text("x")), El("br", []Attr{A("id", "b")})),
			El("p", []Attr{A("id", "x"), A("class", "c")}, Text("p")),
			El("p", []Attr{{Name: "id", Value: "gone", Omit: true}}, Text("q"))))
	r := RenderTracked(n, nil)
	if r.Page != Render(n) {
		t.Fatalf("page differs from Render:\n%s\n%s", r.Page, Render(n))
	}
	var ids []string
	for _, g := range r.Regions {
		ids = append(ids, g.ID)
		m := r.Page[g.Start:g.End]
		if g.ID == "m" {
			if !g.Inert || m != `<meta id="m" />` {
				t.Errorf("head region %+v %q", g, m)
			}
			continue
		}
		if g.Inert || !strings.HasPrefix(m, "<") || !strings.Contains(m, `id="`+g.ID+`"`) {
			t.Errorf("region %+v: %q", g, m)
		}
	}
	if want := []string{"m", "rows", "a", "b", "x"}; !reflect.DeepEqual(ids, want) {
		t.Errorf("ids = %v, want %v", ids, want)
	}
	if got := r.Page[r.Regions[1].Start:r.Regions[1].End]; got != `<ul id="rows"><li id="a">A</li><li>x</li><br id="b" /></ul>` {
		t.Errorf("rows = %q", got)
	}
	if r.RawIDs {
		t.Error("RawIDs without Html.unsafe")
	}
	if !RenderTracked(page(Raw(`<p ID="z">raw</p>`)), nil).RawIDs {
		t.Error("an id in raw markup sets RawIDs")
	}
}

func selectors(ps []Patch) []string {
	out := []string{}
	for _, p := range ps {
		out = append(out, p.Selector)
	}
	return out
}

// fragmentCases are before/after pages and the selectors Fragments patches.
var fragmentCases = []struct {
	name          string
	before, after Node
	want          []string
}{
	{"unchanged", page(El("ul", []Attr{A("id", "rows")}, li("a", "A"))), page(El("ul", []Attr{A("id", "rows")}, li("a", "A"))), []string{}},
	{"one region", page(El("ul", []Attr{A("id", "rows")}, li("a", "A"), li("b", "B"))), page(El("ul", []Attr{A("id", "rows")}, li("a", "A"), li("b", "B!"))), []string{"#b"}},
	{"two regions", page(li("a", "A"), Text("mid"), li("b", "B")), page(li("a", "A2"), Text("mid"), li("b", "B2")), []string{"#a", "#b"}},
	{"nested child only", page(El("div", []Attr{A("id", "outer")}, El("h2", nil, Text("h")), El("section", []Attr{A("id", "inner")}, li("c", "C")))),
		page(El("div", []Attr{A("id", "outer")}, El("h2", nil, Text("h")), El("section", []Attr{A("id", "inner")}, li("c", "C2")))), []string{"#c"}},
	{"child added", page(El("ul", []Attr{A("id", "rows")}, li("a", "A"))), page(El("ul", []Attr{A("id", "rows")}, li("a", "A"), li("b", "B"))), []string{"#rows"}},
	{"child removed", page(El("ul", []Attr{A("id", "rows")}, li("a", "A"), li("b", "B"))), page(El("ul", []Attr{A("id", "rows")}, li("a", "A"))), []string{"#rows"}},
	{"children reordered", page(El("ul", []Attr{A("id", "rows")}, li("a", "A"), li("b", "B"))), page(El("ul", []Attr{A("id", "rows")}, li("b", "B"), li("a", "A"))), []string{"#rows"}},
	{"parent text changed", page(El("div", []Attr{A("id", "d")}, Text("x"), li("a", "A"))), page(El("div", []Attr{A("id", "d")}, Text("y"), li("a", "A2"))), []string{"#d"}},
	{"parent attribute changed", page(El("div", []Attr{A("id", "d"), A("class", "x")}, li("a", "A"))), page(El("div", []Attr{A("id", "d"), A("class", "y")}, li("a", "A"))), []string{"#d"}},
	{"text outside any region", page(El("h1", nil, Text("Count: 1")), li("a", "A")), page(El("h1", nil, Text("Count: 2")), li("a", "A")), []string{"body"}},
	{"duplicated id in after", page(El("div", nil, li("a", "A"), li("b", "B"))), page(El("div", nil, li("a", "A2"), li("a", "B"))), []string{"body"}},
	{"duplicated id in before", page(El("div", nil, li("a", "A"), li("a", "B"))), page(El("div", nil, li("a", "A2"), li("b", "B"))), []string{"body"}},
	{"duplicated child inside a region", page(El("ul", []Attr{A("id", "rows")}, li("a", "A"), li("a", "B"))), page(El("ul", []Attr{A("id", "rows")}, li("a", "A"), li("a", "C"))), []string{"#rows"}},
	{"id outside the pattern", page(El("ul", nil, li("a b", "A"))), page(El("ul", nil, li("a b", "B"))), []string{"body"}},
	{"id starting with a digit", page(El("ul", nil, li("7", "A"))), page(El("ul", nil, li("7", "B"))), []string{"body"}},
	{"id in head is ignored", El("html", nil, El("head", nil, El("meta", []Attr{A("id", "a"), A("content", "1")})), El("body", nil, li("b", "B"))),
		El("html", nil, El("head", nil, El("meta", []Attr{A("id", "a"), A("content", "1")})), El("body", nil, li("b", "B2"))), []string{"#b"}},
	{"id in head shadows a body id", El("html", nil, El("head", nil, El("meta", []Attr{A("id", "b")})), El("body", nil, li("b", "B"))),
		El("html", nil, El("head", nil, El("meta", []Attr{A("id", "b")})), El("body", nil, li("b", "B2"))), []string{"body"}},
	{"region only in after", page(El("div", nil, Text("x"))), page(El("div", nil, li("n", "x"))), []string{"body"}},
	{"raw markup with an id", page(li("a", "A"), Raw(`<p id="a">x</p>`)), page(li("a", "B"), Raw(`<p id="a">x</p>`)), []string{"body"}},
	{"raw markup with an id = spaced", page(li("a", "A"), Raw("<p ID\n= \"a\">x</p>")), page(li("a", "B"), Raw("<p ID\n= \"a\">x</p>")), []string{"body"}},
	{"raw markup in a region", page(El("div", []Attr{A("id", "d")}, Text("1"), Raw(`<b>x</b>`)), li("e", "E")), page(El("div", []Attr{A("id", "d")}, Text("2"), Raw(`<b>x</b>`)), li("e", "E")), []string{"body"}},
	{"div in a p", page(El("p", []Attr{A("id", "msg")}, Text("1"), El("div", nil, Text("x")))), page(El("p", []Attr{A("id", "msg")}, Text("2"), El("div", nil, Text("x")))), []string{"body"}},
	{"div in a p in a region", page(El("section", []Attr{A("id", "s")}, El("p", []Attr{A("id", "msg")}, Text("1"), El("div", nil, Text("x"))))),
		page(El("section", []Attr{A("id", "s")}, El("p", []Attr{A("id", "msg")}, Text("2"), El("div", nil, Text("x"))))), []string{"body"}},
	{"a region after a div in a p", page(El("p", []Attr{A("id", "m")}, Text("a"), El("div", nil, Text("x")), li("t", "1"))),
		page(El("p", []Attr{A("id", "m")}, Text("a"), El("div", nil, Text("x")), li("t", "2"))), []string{"body"}},
	{"a region inside a repaired one", page(El("p", []Attr{A("id", "m")}, El("span", []Attr{A("id", "s")}, Text("1")), El("div", nil, Text("x")))),
		page(El("p", []Attr{A("id", "m")}, El("span", []Attr{A("id", "s")}, Text("2")), El("div", nil, Text("x")))), []string{"#s"}},
	{"a link in a link", page(El("a", []Attr{A("id", "o"), A("href", "/a")}, Text("1"), El("a", []Attr{A("href", "/b")}, Text("b")))),
		page(El("a", []Attr{A("id", "o"), A("href", "/a")}, Text("2"), El("a", []Attr{A("href", "/b")}, Text("b")))), []string{"body"}},
	{"a form in a form", page(El("form", []Attr{A("id", "f")}, Text("1"), El("form", []Attr{A("id", "g")}, Text("g")))),
		page(El("form", []Attr{A("id", "f")}, Text("2"), El("form", []Attr{A("id", "g")}, Text("g")))), []string{"body"}},
	{"foster parenting spares the rows", page(El("table", []Attr{A("id", "t")}, El("tr", []Attr{A("id", "r")}, El("td", nil, Text("1"))), El("div", nil, Text("x")))),
		page(El("table", []Attr{A("id", "t")}, El("tr", []Attr{A("id", "r")}, El("td", nil, Text("2"))), El("div", nil, Text("x")))), []string{"#r"}},
	{"text in a table row", page(El("table", nil, El("tr", []Attr{A("id", "r")}, Text("loose"), El("td", []Attr{A("id", "c")}, Text("1"))))),
		page(El("table", nil, El("tr", []Attr{A("id", "r")}, Text("loose"), El("td", []Attr{A("id", "c")}, Text("2"))))), []string{"#c"}},
	{"an id in noscript", page(El("noscript", nil, El("p", []Attr{A("id", "n")}, Text("1")))), page(El("noscript", nil, El("p", []Attr{A("id", "n")}, Text("2")))), []string{"body"}},
	{"body attribute changed", El("html", nil, El("body", []Attr{A("class", "x")}, li("a", "A"))), El("html", nil, El("body", []Attr{A("class", "y")}, li("a", "A"))), []string{"body"}},
	{"a click button in a region", page(El("ul", []Attr{A("id", "rows")}, El("li", []Attr{A("id", "r1")}, Text("A"), El("button", []Attr{A("id", "b1")}, Text("x"))))),
		page(El("ul", []Attr{A("id", "rows")}, El("li", []Attr{A("id", "r1")}, Text("A2"), El("button", []Attr{A("id", "b1")}, Text("x"))))), []string{"#r1"}},
}

func TestFragments(t *testing.T) {
	for _, c := range fragmentCases {
		t.Run(c.name, func(t *testing.T) {
			before, after := RenderTracked(c.before, nil), RenderTracked(c.after, nil)
			ps := Fragments(before, after)
			if got := selectors(ps); !reflect.DeepEqual(got, c.want) {
				t.Fatalf("patches = %v, want %v", got, c.want)
			}
			// applying the patches to the before-DOM (the markup as the browser's parser builds
			// it, Repair) gives the after-DOM
			if doc, want := applyPatches(t, before.Page, ps), domOf(t, after.Page); !doc.Equal(want) {
				t.Fatalf("patched:\n%s\nwant:\n%s", Render(doc), Render(want))
			}
		})
	}
}

// domOf is markup as the browser's parser builds it.
func domOf(t *testing.T, markup string) Node {
	t.Helper()
	n, err := Parse(markup)
	if err != nil {
		t.Fatal(err)
	}
	return Repair(n)
}

// applyPatches is the DOM of page with ps applied as Datastar does (mode outer: the element
// replaced by the patch's nodes, as its parser builds them).
func applyPatches(t *testing.T, page string, ps []Patch) Node {
	t.Helper()
	doc := domOf(t, page)
	for _, p := range ps {
		el := domOf(t, p.Elements)
		var ok bool
		if p.Selector == "body" {
			doc, ok = ReplaceElement(doc, "body", el)
		} else {
			doc, ok = ReplaceByID(doc, strings.TrimPrefix(p.Selector, "#"), el)
		}
		if !ok {
			t.Fatalf("patch %s finds nothing", p.Selector)
		}
	}
	return doc
}

// TestFragments_PatchIsAfterMarkup: a patch carries the region's markup as the after page has it.
func TestFragments_PatchIsAfterMarkup(t *testing.T) {
	ps := Fragments(RenderTracked(page(li("a", "A")), nil), RenderTracked(page(li("a", "<B>")), nil))
	if len(ps) != 1 || ps[0].Elements != `<li id="a">&lt;B&gt;</li>` {
		t.Fatalf("%+v", ps)
	}
	ps = Fragments(RenderTracked(page(Text("1")), nil), RenderTracked(page(Text("2")), nil))
	if len(ps) != 1 || ps[0].Elements != `<body>2</body>` {
		t.Fatalf("%+v", ps)
	}
}

func TestHash(t *testing.T) {
	h := Hash("<body></body>")
	if len(h) != 16 || strings.Trim(h, "0123456789abcdef") != "" {
		t.Fatalf("Hash = %q", h)
	}
	if h == Hash("<body> </body>") || h != Hash("<body></body>") {
		t.Fatal("Hash is not a function of its input")
	}
	if Hash("") != "cbf29ce484222325" { // FNV-1a 64's offset basis
		t.Fatalf("Hash(\"\") = %s", Hash(""))
	}
}

// TestRepair: the parser's repairs Repair models.
func TestRepair(t *testing.T) {
	for _, c := range []struct{ in, want string }{
		{`<body><p id="msg">1<div>x</div></p></body>`, `<body><p id="msg">1</p><div>x</div><p></p></body>`},
		{`<body><p>a<span>b<div>x</div>c</span>d</p></body>`, `<body><p>a<span>b</span></p><div>x</div>cd<p></p></body>`},
		{`<body><a href="/a">1<a href="/b">2</a>3</a></body>`, `<body><a href="/a">1</a><a href="/b">2</a>3</body>`},
		{`<body><form id="f">1<form id="g">2</form>3</form></body>`, `<body><form id="f">123</form></body>`},
		{`<body><table><tr><td>1</td></tr><div>x</div></table></body>`, `<body><div>x</div><table><tr><td>1</td></tr></table></body>`},
		{`<body><table><tr>t<td>1</td></tr></table></body>`, `<body>t<table><tr><td>1</td></tr></table></body>`},
		{`<body><ul><li>a<li>b</li></li></ul></body>`, `<body><ul><li>a</li><li>b</li></ul></body>`},
		{`<body><ul><li>a<ul><li>b</li></ul></li></ul></body>`, `<body><ul><li>a<ul><li>b</li></ul></li></ul></body>`},
		{`<body><select><option>a</option><div>b</div></select></body>`, `<body><select><option>a</option>b</select></body>`},
		{`<body><textarea><b>x</b></textarea></body>`, `<body><textarea>&lt;b&gt;x&lt;/b&gt;</textarea></body>`},
		{`<body><button>a<button>b</button></button></body>`, `<body><button>a</button><button>b</button></body>`},
		{`<body><h1>a<h2>b</h2></h1></body>`, `<body><h1>a</h1><h2>b</h2></body>`},
		{`<html><head><title>t</title></head><body><div><p>ok</p></div></body></html>`, `<html><head><title>t</title></head><body><div><p>ok</p></div></body></html>`},
	} {
		n, err := Parse(c.in)
		if err != nil {
			t.Fatal(err)
		}
		want, _ := Parse(c.want)
		if got := Repair(n); !got.Equal(want) {
			t.Errorf("Repair(%s) = %s, want %s", c.in, Render(got), c.want)
		}
	}
}

// TestRepair_MisplacedPatchDrifts: the patch the renderer's markup suggests for a <p> holding a
// <div> (#msg, outer) does not give the after-page on the DOM the browser built — each click
// would add another <div> and <p> — which is why such a region is inert and the body is patched.
func TestRepair_MisplacedPatchDrifts(t *testing.T) {
	before := RenderTracked(page(El("p", []Attr{A("id", "msg")}, Text("1"), El("div", nil, Text("x")))), nil)
	after := RenderTracked(page(El("p", []Attr{A("id", "msg")}, Text("2"), El("div", nil, Text("x")))), nil)
	wrong := []Patch{{Selector: "#msg", Elements: after.Page[after.Regions[0].Start:after.Regions[0].End]}}
	if applyPatches(t, before.Page, wrong).Equal(domOf(t, after.Page)) {
		t.Fatal("the misplaced patch went unnoticed")
	}
	if !before.Regions[0].Inert || !after.Regions[0].Inert {
		t.Fatal("#msg is a region")
	}
}

func TestReplaceByID(t *testing.T) {
	doc, _ := Parse(`<body><ul id="rows"><li id="a">A</li><li id="b">B</li></ul><p id="b">late</p></body>`)
	with, _ := Parse(`<li id="b" class="x">B2</li>`)
	got, ok := ReplaceByID(doc, "b", with)
	if !ok || Render(got) != `<body><ul id="rows"><li id="a">A</li><li id="b" class="x">B2</li></ul><p id="b">late</p></body>` {
		t.Fatalf("%v %s", ok, Render(got))
	}
	if _, ok := ReplaceByID(doc, "zz", with); ok {
		t.Fatal("no element zz")
	}
}

func TestWithout(t *testing.T) {
	doc, _ := Parse(`<body><p>a</p><al-shown id="_alder_shown" data-h="1" hidden></al-shown><div><al-shown></al-shown>b</div></body>`)
	got := Without(doc, func(n Node) bool { return n.Tag() == "al-shown" })
	if Render(got) != `<body><p>a</p><div>b</div></body>` {
		t.Fatal(Render(got))
	}
}

// A click's and a form's post send the shown hash (Shown, read by datastar.BoostInit's helper).
func TestClickPost_SendsShownHeader(t *testing.T) {
	if got := ClickPost("/_m/M/T"); got != "(window._alderBusy||((e,p)=>p))(el, @post('/_m/M/T', window._alderO()))" {
		t.Fatal(got)
	}
}

func TestSubmitPost_SendsShownHeader(t *testing.T) {
	if got := SubmitPost("/_m/M/S"); got != "(window._alderBusy||((e,p)=>p))(el, @post('/_m/M/S', window._alderO('form')))" {
		t.Fatal(got)
	}
}
