package rt

import (
	"math"
	"net/url"
	"reflect"
	"testing"
	"time"
)

func TestMaybe_OrElse(t *testing.T) {
	if got := OrElse(Just[int64](3), 0); got != 3 {
		t.Fatalf("OrElse(Just 3) = %d", got)
	}
	if got := OrElse(Nothing[int64](), 7); got != 7 {
		t.Fatalf("OrElse(Nothing) = %d", got)
	}
	var zero Maybe[string]
	if zero != Nothing[string]() {
		t.Fatal("zero Maybe is not Nothing")
	}
}

func TestResult_Constructors(t *testing.T) {
	ok := Ok[string, int64](1)
	if !ok.IsOk || ok.Val != 1 {
		t.Fatalf("Ok = %+v", ok)
	}
	e := Err[string, int64]("bad")
	if e.IsOk || e.Err != "bad" {
		t.Fatalf("Err = %+v", e)
	}
}

func TestDict_InsertionOrderAndGet(t *testing.T) {
	var d Dict[string, int64]
	d = d.Insert("b", 1).Insert("a", 2).Insert("c", 3)
	d2 := d.Insert("b", 10) // update keeps position, does not touch d
	if got := d2.Keys(); !reflect.DeepEqual(got, []string{"b", "a", "c"}) {
		t.Fatalf("Keys = %v", got)
	}
	if got := d2.Values(); !reflect.DeepEqual(got, []int64{10, 2, 3}) {
		t.Fatalf("Values = %v", got)
	}
	if v, ok := d.Get("b"); !ok || v != 1 {
		t.Fatalf("original dict mutated: b = %d, %v", v, ok)
	}
	if _, ok := d.Get("zz"); ok {
		t.Fatal("Get of a missing key reported ok")
	}
	d3 := d2.Remove("a")
	if got := d3.Keys(); !reflect.DeepEqual(got, []string{"b", "c"}) {
		t.Fatalf("Keys after Remove = %v", got)
	}
	if d2.Len() != 3 || d3.Len() != 2 {
		t.Fatalf("Len = %d, %d", d2.Len(), d3.Len())
	}
	if !d3.Has("c") || d3.Has("a") {
		t.Fatal("Has wrong after Remove")
	}
}

func TestConcatAppendUnit(t *testing.T) {
	if got := Concat("a", "", "bc", "é"); got != "abcé" {
		t.Fatalf("Concat = %q", got)
	}
	a := make([]int64, 1, 10)
	a[0] = 1
	x := Append(a, []int64{2})
	y := Append(a, []int64{3})
	if !reflect.DeepEqual(x, []int64{1, 2}) || !reflect.DeepEqual(y, []int64{1, 3}) {
		t.Fatalf("Append aliased: %v %v", x, y)
	}
	if Unit() != (struct{}{}) {
		t.Fatal("Unit")
	}
}

func TestDate_Format(t *testing.T) {
	if got := Format("Jan 2, 2006", Date{2026, 10, 9}); got != "Oct 9, 2026" {
		t.Fatalf("Format = %q", got)
	}
	if got := Format("2006-01-02", Date{2026, 1, 5}); got != "2026-01-05" {
		t.Fatalf("Format ISO = %q", got)
	}
}

func TestEq(t *testing.T) {
	type rec struct {
		Name string
		At   Maybe[time.Time]
		Tags []string
	}
	now := time.Now()
	stripped := now.Round(0) // same instant, no monotonic reading
	other := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	empty := Dict[string, int64]{}
	emptied := empty.Insert("a", 1).Remove("a")
	ab := empty.Insert("a", 1).Insert("b", 2)
	ba := empty.Insert("b", 2).Insert("a", 1)
	cases := []struct {
		name string
		got  bool
		want bool
	}{
		{"emptied dict == empty dict", Eq(emptied, empty), true},
		{"time monotonic stripped", Eq(now, stripped), true},
		{"time in another zone", Eq(other, other.In(time.FixedZone("X", 3600))), true},
		{"different times", Eq(now, other), false},
		{"dict order ignored", Eq(ab, ba), true},
		{"dict different value", Eq(ab, ba.Insert("a", 9)), false},
		{"dict different keys", Eq(ab, ab.Remove("b").Insert("c", 2)), false},
		{"nested record with Maybe Time", Eq(rec{"x", Just(now), nil}, rec{"x", Just(stripped), []string{}}), true},
		{"nested record differs", Eq(rec{"x", Just(now), nil}, rec{"x", Nothing[time.Time](), nil}), false},
		{"slices", Eq([]int64{1, 2}, []int64{1, 2}), true},
		{"slices differ", Eq([]int64{1, 2}, []int64{1}), false},
		{"dates", Eq(Date{2026, 1, 2}, Date{2026, 1, 2}), true},
		{"strings", Eq("a", "b"), false},
		{"results", Eq(Ok[string, int64](1), Ok[string, int64](1)), true},
		{"dict of dicts", Eq(Dict[string, Dict[string, int64]]{}.Insert("k", ab), Dict[string, Dict[string, int64]]{}.Insert("k", ba)), true},
		{"plain map nil vs empty", Eq(map[string]int64(nil), map[string]int64{}), true},
	}
	for _, c := range cases {
		if c.got != c.want {
			t.Errorf("%s: got %v", c.name, c.got)
		}
	}
	if !ab.Equal(ba) {
		t.Error("Dict.Equal")
	}
}

func TestDict_RemoveThenInsertMovesToEnd(t *testing.T) {
	d := Dict[string, int64]{}.Insert("a", 1).Insert("b", 2).Remove("a").Insert("a", 3)
	if got := d.Keys(); !reflect.DeepEqual(got, []string{"b", "a"}) {
		t.Fatalf("Keys = %v", got)
	}
}

func TestDateCmpTimeCmp(t *testing.T) {
	d := func(y, m, dd int) Date { return Date{Y: y, M: m, D: dd} }
	for _, tc := range []struct {
		a, b Date
		sign int
	}{
		{d(2026, 1, 1), d(2026, 1, 1), 0},
		{d(2025, 12, 31), d(2026, 1, 1), -1},
		{d(2026, 2, 1), d(2026, 1, 31), 1},
		{d(2026, 1, 2), d(2026, 1, 3), -1},
	} {
		if got := DateCmp(tc.a, tc.b); sign(got) != tc.sign {
			t.Errorf("DateCmp(%v, %v) = %d, want sign %d", tc.a, tc.b, got, tc.sign)
		}
	}
	t0 := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	if TimeCmp(t0, t0.Add(time.Second)) >= 0 || TimeCmp(t0.Add(time.Second), t0) <= 0 {
		t.Error("TimeCmp orders instants")
	}
	if TimeCmp(t0, t0.In(time.FixedZone("x", 3600))) != 0 {
		t.Error("TimeCmp compares instants, not zones")
	}
}

func sign(n int) int {
	switch {
	case n < 0:
		return -1
	case n > 0:
		return 1
	}
	return 0
}

func TestURLs(t *testing.T) {
	if got := PageURL(Page{Path: "/todos/3"}); got != "/todos/3" {
		t.Errorf("PageURL = %q", got)
	}
	if got := MsgURL("Todos.Index", "Ping"); got != "/_m/Todos.Index/Ping" {
		t.Errorf("MsgURL no params = %q", got)
	}
	got := MsgURL("M", "Pick", KV{Key: "name", Value: "a b&c"}, KV{Key: "n", Value: "2"}, KV{Key: "x", Omit: true})
	if want := "/_m/M/Pick?name=a+b%26c&n=2"; got != want {
		t.Errorf("MsgURL = %q, want %q", got, want)
	}
	page := Page{Path: "/t/" + PathSegment("a/b c"), Query: []KV{QueryKV("filter", "All", "All"), QueryKV("q", "x&y", ""), {Key: "n", Value: "1"}}}
	if got, want := PageURL(page), "/t/a%2Fb%20c?q=x%26y&n=1"; got != want {
		t.Errorf("PageURL = %q, want %q", got, want)
	}
	if got := PageURL(Page{Path: "/", Query: []KV{QueryKV("filter", "All", "All")}}); got != "/" {
		t.Errorf("PageURL at defaults = %q", got)
	}
	if got, want := PageURL(Page{Path: "/", Query: Queries([]KV{{Key: "a", Value: "1"}}, QueryKVs("t", []string{"x", "y"}))}), "/?a=1&t=x&t=y"; got != want {
		t.Errorf("PageURL with a list = %q, want %q", got, want)
	}
	if got := MaybeText(Just(int64(3)), func(n int64) string { return "n" }); got != "n" {
		t.Errorf("MaybeText Just = %q", got)
	}
	if got := MaybeText(Nothing[int64](), func(n int64) string { return "n" }); got != "" {
		t.Errorf("MaybeText Nothing = %q", got)
	}
}

// Any Alder type can be a Dict key: keys are compared by their canonical encoding (C2).
func TestDict_AnyKey(t *testing.T) {
	var lists Dict[[]int64, string]
	lists = lists.Insert([]int64{1, 2}, "a").Insert([]int64{1}, "b").Insert(nil, "nil")
	if v, ok := lists.Get([]int64{1, 2}); !ok || v != "a" {
		t.Fatalf("Get [1, 2] = %q, %v", v, ok)
	}
	if v, ok := lists.Get([]int64{}); !ok || v != "nil" {
		t.Fatalf("empty list key is not the nil list key: %q, %v", v, ok)
	}
	if lists.Len() != 3 {
		t.Fatalf("Len = %d", lists.Len())
	}

	utc := time.Date(2026, 10, 9, 12, 0, 0, 0, time.UTC)
	ny := utc.In(time.FixedZone("NY", -4*3600))
	times := Dict[time.Time, int64]{}.Insert(utc, 1)
	if v, ok := times.Get(ny); !ok || v != 1 {
		t.Fatalf("the same instant in another location is another key: %d, %v", v, ok)
	}
	if times.Insert(ny, 2).Len() != 1 {
		t.Fatal("Insert of the same instant added a key")
	}
	if !times.Equal(Dict[time.Time, int64]{}.Insert(ny, 1)) || !Eq(times, Dict[time.Time, int64]{}.Insert(ny, 1)) {
		t.Fatal("dicts keyed by the same instants in two locations are not equal")
	}

	type rec struct {
		A string
		B Maybe[int64]
	}
	recs := Dict[rec, int64]{}.Insert(rec{"x", Just[int64](1)}, 1).Insert(rec{"x", Nothing[int64]()}, 2)
	if v, _ := recs.Get(rec{"x", Just[int64](1)}); v != 1 || recs.Len() != 2 {
		t.Fatalf("record keys: %d, %d", v, recs.Len())
	}

	ab := Dict[string, int64]{}.Insert("a", 1).Insert("b", 2)
	ba := Dict[string, int64]{}.Insert("b", 2).Insert("a", 1)
	dicts := Dict[Dict[string, int64], int64]{}.Insert(ab, 1)
	if v, ok := dicts.Get(ba); !ok || v != 1 {
		t.Fatal("a Dict key ignores insertion order")
	}

	// length prefixes keep different values apart
	for _, pair := range [][2]any{
		{[]string{"a,b"}, []string{"a", "b"}},
		{[]string{"ab", ""}, []string{"a", "b"}},
		{rec{"1", Nothing[int64]()}, rec{"", Just[int64](1)}},
		{0.0, 0},
		{int64(1), "1"},
	} {
		if Key(pair[0]) == Key(pair[1]) {
			t.Errorf("Key(%#v) == Key(%#v)", pair[0], pair[1])
		}
	}
	if Key(0.0) != Key(math.Copysign(0, -1)) {
		t.Error("-0 and 0 are different keys")
	}
}

func TestPageParams(t *testing.T) {
	q := func(s string) url.Values { v, _ := url.ParseQuery(s); return v }
	for _, c := range []struct {
		pattern, referer, query string
		params                  map[string]string
		values                  string
	}{
		// the page's parameters come from the referer when the message has none of them
		{"/todos/{id}", "http://h/todos/a%2Fb?x=1", "", map[string]string{"id": "a/b"}, "x=1"},
		// the message's parameters win, also for a path parameter of the same name
		{"/todos/{id}", "http://h/todos/a", "id=p1&x=2", map[string]string{"id": "p1"}, "id=p1&x=2"},
		{"/", "http://h/?filter=Active", "id=7", map[string]string{}, "filter=Active&id=7"},
		// a referer that is another page, or none, gives nothing
		{"/todos/{id}", "http://h/other/a?x=1", "y=2", map[string]string{}, "y=2"},
		{"/todos/{id}", "", "", map[string]string{}, ""},
		{"/edit/{n}/x", "http://h/edit/3/x/", "", map[string]string{"n": "3"}, ""},
	} {
		params, values := PageParams(c.pattern, c.referer, q(c.query))
		if !reflect.DeepEqual(params, c.params) || values.Encode() != q(c.values).Encode() {
			t.Errorf("PageParams(%q, %q, %q) = %v, %q; want %v, %q", c.pattern, c.referer, c.query, params, values.Encode(), c.params, c.values)
		}
	}
}

func TestBackURL(t *testing.T) {
	for ref, want := range map[string]string{
		"":                           "/",
		"http://h/todos/a?filter=x":  "/todos/a?filter=x",
		"https://evil.example/":      "/",
		"http://h//evil.example/x":   "/",
		"http://h/a%2Fb":             "/a%2Fb",
		"/relative?q=1":              "/relative?q=1",
		"::not a url":                "/",
		"http://h/%5Cevil.example/x": "/%5Cevil.example/x",
	} {
		if got := BackURL(ref); got != want {
			t.Errorf("BackURL(%q) = %q, want %q", ref, got, want)
		}
	}
}
