package rt_test

import (
	"math"
	"reflect"
	"strings"
	"testing"
	"time"

	"alder/rt"
	"alder/rt/db"
	"alder/rt/eff"
	"alder/rt/html"
)

// Shapes mirroring the emitter's: a union struct with a Tag field and one payload pointer per
// constructor with a payload; an anonymous record payload is a payload__ struct, a scalar payload
// is wrapped as { Value T }.
type tagShape uint8

const (
	isCircle tagShape = iota
	isRect
	isEmpty
)

type payloadRect struct{ W, H float64 }

type shape struct {
	Tag    tagShape
	Circle *struct{ Value float64 }
	Rect   *payloadRect
}

type tagTagged uint8

// tagged has a constructor named Tag, whose payload field is Tag__ (names.go ctorField).
type tagged struct {
	Tag   tagTagged
	Tag__ *struct{ Value int64 }
}

type todo struct {
	Title   string
	Done    bool
	OwnerId int64
}

type node struct {
	V    int64
	Next *rt.Maybe[node]
}

type tagBox uint8

type box struct {
	Tag  tagBox
	Full *todo // a named record payload
}

func init() {
	rt.RegisterUnion(reflect.TypeOf(tagShape(0)), []string{"Circle", "Rect", "Empty"})
	rt.RegisterUnion(reflect.TypeOf(tagTagged(0)), []string{"Tag"})
	rt.RegisterUnion(reflect.TypeOf(tagBox(0)), []string{"Empty", "Full"})
}

func circle(r float64) shape  { return shape{Tag: isCircle, Circle: &struct{ Value float64 }{r}} }
func rect(w, h float64) shape { return shape{Tag: isRect, Rect: &payloadRect{w, h}} }

func TestShow(t *testing.T) {
	d := rt.Dict[string, int64]{}.Insert("b", 2).Insert("a", 1)
	tm := time.Date(2026, 10, 9, 12, 30, 0, 500_000_000, time.FixedZone("x", 3600))
	cases := []struct {
		name string
		v    any
		want string
	}{
		{"int", int64(3), "3"},
		{"negative int", int64(-3), "-3"},
		{"float", 1.5, "1.5"},
		{"whole float", 1.0, "1.0"},
		{"big float", 1e21, "1000000000000000000000.0"},
		{"million", 1e6, "1000000.0"},
		{"large fraction", 1234567.5, "1234567.5"},
		{"small float", 0.00001, "0.00001"},
		{"negative small", -0.00025, "-0.00025"},
		{"-Inf", math.Inf(-1), "-Inf"},
		{"NaN", math.NaN(), "NaN"},
		{"Inf", math.Inf(1), "+Inf"},
		{"string", "milk", `"milk"`},
		{"true", true, "True"},
		{"false", false, "False"},
		{"unit", struct{}{}, "()"},
		{"list", []int64{1, 2}, "[1, 2]"},
		{"bytes", []byte("hi"), `Text.toBytes "hi"`},
		{"binary bytes", []byte{0xff}, "<1 bytes>"},
		{"empty list", []string{}, "[]"},
		{"nil list", []string(nil), "[]"},
		{"Nothing", rt.Nothing[int64](), "Nothing"},
		{"Just", rt.Just[int64](3), "Just 3"},
		{"Just negative", rt.Just[int64](-1), "Just (-1)"},
		{"Just negative float", rt.Just(-1.5), "Just (-1.5)"},
		{"Just Just", rt.Just(rt.Just[int64](1)), "Just (Just 1)"},
		{"Just Nothing", rt.Just(rt.Nothing[int64]()), "Just Nothing"},
		{"Just record", rt.Just(todo{Title: "a"}), `Just { title: "a", done: False, ownerId: 0 }`},
		{"Just list", rt.Just([]int64{-1}), "Just [-1]"},
		{"Ok", rt.Ok[string](int64(1)), "Ok 1"},
		{"Err", rt.Err[string, int64]("x"), `Err "x"`},
		{"Dict", d, `Dict.fromList [{ key: "b", value: 2 }, { key: "a", value: 1 }]`},
		{"empty Dict", rt.Dict[string, int64]{}, "Dict.empty"},
		{"Just Dict", rt.Just(d.Remove("b")), `Just (Dict.fromList [{ key: "a", value: 1 }])`},
		{"Date", rt.Date{Y: 2026, M: 10, D: 9}, "2026-10-09"},
		{"Time", tm, "2026-10-09T11:30:00.5Z"},
		{"record", todo{Title: "Milk", OwnerId: 7}, `{ title: "Milk", done: False, ownerId: 7 }`},
		{"union scalar", circle(1), "Circle 1.0"},
		{"union record", rect(1, 2), "Rect { w: 1.0, h: 2.0 }"},
		{"union no payload", shape{Tag: isEmpty}, "Empty"},
		{"union named record", box{Tag: 1, Full: &todo{Title: "t"}}, `Full { title: "t", done: False, ownerId: 0 }`},
		{"ctor named Tag", tagged{Tag: 0, Tag__: &struct{ Value int64 }{4}}, "Tag 4"},
		{"Just union payload", rt.Just(rect(1, 2)), "Just (Rect { w: 1.0, h: 2.0 })"},
		{"Just union no payload", rt.Just(shape{Tag: isEmpty}), "Just Empty"},
		{"union negative payload", circle(-2), "Circle (-2.0)"},
		{"list of unions", []shape{circle(1), {Tag: isEmpty}}, "[Circle 1.0, Empty]"},
		{"pointer", &todo{Done: true}, `{ title: "", done: True, ownerId: 0 }`},
		{"nil Maybe pointer", (*rt.Maybe[int64])(nil), "Nothing"},
		{"nil pointer", (*todo)(nil), "<nil>"},
		{"nil", nil, "<nil>"},
		{"html", html.El("p", nil, html.Text("a<b")), "<p>a&lt;b</p>"},
		{"Query", eff.Q(eff.Pure[int64](1)), "<Query>"},
		{"Task", eff.Pure[int64](1), "<Task>"},
		{"Select", db.From(&db.Table[todo]{Name: "Todo.Todo"}), "<Select>"},
		{"Table", &db.Table[todo]{Name: "Todo.Todo"}, "<Table>"},
		{"Condition", db.Cond{Kind: db.Eq, Col: "done", Val: true}, "<Condition>"},
		{"Order", db.Order{Col: "title"}, "<Order>"},
		{"list of Conditions", []db.Cond{{Kind: db.IsNull, Col: "due"}}, "[<Condition>]"},
		{"Url.external", rt.Route{Path: "/todos", Query: []rt.KV{{Key: "f", Value: "a b"}}}, "/todos?f=a+b"},
		{"function", func() {}, "<function>"},
		{"HttpError", rt.HttpError{Status: 404, Message: "nf"}, `{ status: 404, message: "nf" }`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := rt.Show(c.v); got != c.want {
				t.Errorf("Show = %s, want %s", got, c.want)
			}
		})
	}
}

func TestShow_BoxedAndNested(t *testing.T) {
	n := node{V: 1, Next: &rt.Maybe[node]{Ok: true, Val: node{V: 2, Next: &rt.Maybe[node]{}}}}
	if got, want := rt.Show(n), "{ v: 1, next: Just { v: 2, next: Nothing } }"; got != want {
		t.Errorf("boxed: %s, want %s", got, want)
	}
	if got, want := rt.Show(node{V: 3}), "{ v: 3, next: Nothing }"; got != want {
		t.Errorf("nil boxed: %s, want %s", got, want)
	}
	type outer struct {
		Shapes []shape
		Best   rt.Maybe[shape]
		ByName rt.Dict[string, shape]
	}
	o := outer{Shapes: []shape{rect(1, 2)}, Best: rt.Just(circle(0.5)), ByName: rt.Dict[string, shape]{}.Insert("r", rect(3, 4))}
	want := `{ shapes: [Rect { w: 1.0, h: 2.0 }], best: Just (Circle 0.5), byName: Dict.fromList [{ key: "r", value: Rect { w: 3.0, h: 4.0 } }] }`
	if got := rt.Show(o); got != want {
		t.Errorf("nested:\n got %s\nwant %s", got, want)
	}
}

func TestShow_Escapes(t *testing.T) {
	for in, want := range map[string]string{
		`a"b`:    `"a\"b"`,
		`a\b`:    `"a\\b"`,
		"a\nb":   `"a\nb"`,
		"a\tb":   `"a\tb"`,
		"a\r\nb": `"a\r\nb"`,
		"{x}":    `"\{x}"`,
		"é ok }": `"é ok }"`,
		"":       `""`,
	} {
		if got := rt.Show(in); got != want {
			t.Errorf("Show(%q) = %s, want %s", in, got, want)
		}
	}
}

// Show never panics and never prints Go syntax for an unregistered union: it falls back to a
// record with its Tag as a number.
func TestShow_Unregistered(t *testing.T) {
	type tagX uint8
	type x struct {
		Tag tagX
		A   *struct{ Value int64 }
	}
	got := rt.Show(x{Tag: 0, A: &struct{ Value int64 }{1}})
	if strings.Contains(got, "0x") || got == "" {
		t.Errorf("unregistered union: %s", got)
	}
	// a registered tag out of range does not panic
	_ = rt.Show(shape{Tag: 9})
	type unexported struct{ at time.Time }
	_ = rt.Show(unexported{})
	_ = rt.Show(map[string]int{"a": 1})
}

// Dict keys and values print as they do anywhere else: a Time, Html, record or union key or value
// is not hidden behind the Dict's unexported fields.
func TestShow_DictEntries(t *testing.T) {
	tm := time.Date(2026, 10, 9, 0, 0, 0, 0, time.UTC)
	nested := rt.Dict[string, rt.Dict[string, time.Time]]{}.Insert("a", rt.Dict[string, time.Time]{}.Insert("b", tm))
	for _, c := range []struct {
		v    any
		want string
	}{
		{rt.Dict[string, html.Node]{}.Insert("p", html.El("p", nil, html.Text("hi"))), `Dict.fromList [{ key: "p", value: <p>hi</p> }]`},
		{rt.Dict[time.Time, int64]{}.Insert(tm, 1), `Dict.fromList [{ key: 2026-10-09T00:00:00Z, value: 1 }]`},
		{rt.Dict[string, time.Time]{}.Insert("t", tm), `Dict.fromList [{ key: "t", value: 2026-10-09T00:00:00Z }]`},
		{rt.Dict[todo, int64]{}.Insert(todo{Title: "a"}, 2), `Dict.fromList [{ key: { title: "a", done: False, ownerId: 0 }, value: 2 }]`},
		{rt.Dict[shape, int64]{}.Insert(rect(1, 2), 3).Insert(shape{Tag: isEmpty}, 4),
			`Dict.fromList [{ key: Rect { w: 1.0, h: 2.0 }, value: 3 }, { key: Empty, value: 4 }]`},
		{nested, `Dict.fromList [{ key: "a", value: Dict.fromList [{ key: "b", value: 2026-10-09T00:00:00Z }] }]`},
	} {
		if got := rt.Show(c.v); got != c.want {
			t.Errorf("Show = %s, want %s", got, c.want)
		}
	}
}

// underscored is gen's struct for `{ _h: Html a, _t: Time, _d: Dict Text Time, n: Int }`: a field
// whose Alder name starts with '_' is the exported Go field U__<rest> (gen.Field).
type underscored struct {
	U__h html.Node
	U__t time.Time
	U__d rt.Dict[string, time.Time]
	N    int64
}

func TestShow_UnderscoreFields(t *testing.T) {
	tm := time.Date(2026, 10, 9, 0, 0, 0, 0, time.UTC)
	v := underscored{U__h: html.El("b", nil, html.Text("x")), U__t: tm, U__d: rt.Dict[string, time.Time]{}.Insert("k", tm), N: 1}
	want := `{ _h: <b>x</b>, _t: 2026-10-09T00:00:00Z, _d: Dict.fromList [{ key: "k", value: 2026-10-09T00:00:00Z }], n: 1 }`
	if got := rt.Show(v); got != want {
		t.Errorf("Show:\n got %s\nwant %s", got, want)
	}
}

func TestEqKey_UnderscoreFields(t *testing.T) {
	tm := time.Date(2026, 10, 9, 0, 0, 0, 0, time.UTC)
	ab := rt.Dict[string, time.Time]{}.Insert("a", tm).Insert("b", tm)
	ba := rt.Dict[string, time.Time]{}.Insert("b", tm).Insert("a", tm.In(time.FixedZone("x", 3600)))
	type rec struct {
		U__t time.Time
		U__d rt.Dict[string, time.Time]
	}
	x, y := rec{tm, ab}, rec{tm.In(time.FixedZone("y", -7200)), ba}
	if !rt.Eq(x, y) {
		t.Error("Eq: the same instant in another zone, or the same entries in another order, differ")
	}
	if rt.Eq(x, rec{tm.Add(1), ab}) {
		t.Error("Eq: different instants are equal")
	}
	if rt.Key(x) != rt.Key(y) {
		t.Errorf("Key: %q != %q", rt.Key(x), rt.Key(y))
	}
}

// A Job prints as its kind and params; its priority, a function of its kind, is not shown.
func TestShow_JobIgnoresPriority(t *testing.T) {
	got := rt.Show(rt.Job{Kind: "A.B", Params: "{}", Priority: 10})
	if want := rt.Show(rt.Job{Kind: "A.B", Params: "{}"}); got != want || got != `{ kind: "A.B", params: "\{}" }` {
		t.Fatalf("Show: %s, want %s", got, want)
	}
}

// A replace prints its roots as markup (rt/html sets rt.ShowRoot) and its removals and toasts as
// the modifiers, in the order written (#309).
func TestShow_Replace(t *testing.T) {
	count := html.El("p", []html.Attr{html.A("id", "member-count")}, html.Text("1 members"))
	r := rt.WithRemove("member-3", rt.Replace([]any{count}))
	if got, want := rt.Show(r), `Response.replace [<p id="member-count">1 members</p>] |> Response.remove "member-3"`; got != want {
		t.Errorf("Show = %s, want %s", got, want)
	}
	r = rt.WithToast(rt.NoticeSuccess, "Gone", rt.WithRemove("b", rt.WithToast(rt.NoticeInfo, "x", rt.WithRemove("a", rt.Replace(nil)))))
	if got, want := rt.Show(r), `Response.replace [] |> Response.remove "a" |> Response.toast "x" |> Response.remove "b" |> Response.success "Gone"`; got != want {
		t.Errorf("Show = %s, want %s", got, want)
	}
}

// Two replaces are equal when their roots render alike, in order, with the same removals and
// toasts in the same order (#309).
func TestResponse_ReplaceEqual(t *testing.T) {
	p := func(s string) html.Node { return html.El("p", []html.Attr{html.A("id", "c")}, html.Text(s)) }
	mk := func(roots []any, removes ...string) rt.Response {
		r := rt.Replace(roots)
		for _, id := range removes {
			r = rt.WithRemove(id, r)
		}
		return rt.WithToast(rt.NoticeInfo, "Removed", r)
	}
	a := mk([]any{p("1"), p("2")}, "m-1", "m-2")
	if b := mk([]any{p("1"), html.El("p", []html.Attr{html.A("id", "c")}, html.Text("2"))}, "m-1", "m-2"); !rt.Eq(a, b) {
		t.Fatal("same rendered roots, removals and toasts should be equal")
	}
	if b := mk([]any{p("1"), p("2")}, "m-2", "m-1"); rt.Eq(a, b) {
		t.Fatal("removals in another order should be unequal")
	}
	if b := mk([]any{p("2"), p("1")}, "m-1", "m-2"); rt.Eq(a, b) {
		t.Fatal("roots in another order should be unequal")
	}
	if b := mk([]any{p("1")}, "m-1", "m-2"); rt.Eq(a, b) {
		t.Fatal("fewer roots should be unequal")
	}
	if rt.Eq(rt.Replace(nil), rt.Refresh()) {
		t.Fatal("a replace and a refresh should be unequal")
	}
}

// A replace's roots compare and print as they are sent (#329): flattened as web.ReplaceRoots
// flattens them, so one built from a fragment with whitespace text equals one built from the
// plain nodes, and both print alike in a failed comparison.
func TestResponse_ReplaceEqualFlattened(t *testing.T) {
	p := func(id string) html.Node { return html.El("p", []html.Attr{html.A("id", id)}, html.Text(id)) }
	wrapped := rt.Replace([]any{html.Nodes(html.Text("\n  "), p("a"), html.None(), html.Nodes(p("b"))), html.Text(" ")})
	plain := rt.Replace([]any{p("a"), p("b")})
	if !rt.Eq(wrapped, plain) {
		t.Errorf("a fragment of the roots should equal the roots: %s vs %s", rt.Show(wrapped), rt.Show(plain))
	}
	want := `Response.replace [<p id="a">a</p>, <p id="b">b</p>]`
	if got := rt.Show(wrapped); got != want {
		t.Errorf("Show = %s, want %s", got, want)
	}
	if rt.Eq(wrapped, rt.Replace([]any{html.Nodes(p("a"), p("b"), p("c"))})) {
		t.Error("a third root should be unequal")
	}
}

// A root that is not a view (only Go code can make one) prints by fmt instead of panicking.
func TestShow_ReplaceNonNodeRoot(t *testing.T) {
	if got, want := rt.Show(rt.Replace([]any{42})), "Response.replace [42]"; got != want {
		t.Errorf("Show = %s, want %s", got, want)
	}
}
