package rt_test

import (
	"math"
	"reflect"
	"strings"
	"testing"
	"time"

	"alder/rt"
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
		{"big float", 1e21, "1e+21"},
		{"NaN", math.NaN(), "NaN"},
		{"Inf", math.Inf(1), "+Inf"},
		{"string", "milk", `"milk"`},
		{"true", true, "True"},
		{"false", false, "False"},
		{"unit", struct{}{}, "()"},
		{"list", []int64{1, 2}, "[1, 2]"},
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
		{"Page", rt.Page{Path: "/todos", Query: []rt.KV{{Key: "f", Value: "a b"}}}, "/todos?f=a+b"},
		{"Rerender", rt.Rerender(), "Rerender"},
		{"Redirect", rt.Redirect(rt.Page{Path: "/x"}), "Redirect /x"},
		{"Reject", rt.Reject("no"), `Reject "no"`},
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
	type unexported struct{ _at time.Time }
	_ = rt.Show(unexported{})
	_ = rt.Show(map[string]int{"a": 1})
}
