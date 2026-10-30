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
			// applying the patches to the before-markup gives the after-markup
			doc, err := Parse(before.Page)
			if err != nil {
				t.Fatal(err)
			}
			for _, p := range ps {
				el, err := Parse(p.Elements)
				if err != nil {
					t.Fatal(err)
				}
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
			want, _ := Parse(after.Page)
			if !doc.Equal(want) {
				t.Fatalf("patched:\n%s\nwant:\n%s", Render(doc), after.Page)
			}
		})
	}
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
	if got := ClickPost("/_m/M/T"); !strings.Contains(got, "@post('/_m/M/T', {headers: window._alderShown?.(), requestCancellation:") {
		t.Fatal(got)
	}
}

func TestSubmitPost_SendsShownHeader(t *testing.T) {
	if got := SubmitPost("/_m/M/S"); !strings.Contains(got, "@post('/_m/M/S', {contentType: 'form', headers: window._alderShown?.(), requestCancellation:") {
		t.Fatal(got)
	}
}
