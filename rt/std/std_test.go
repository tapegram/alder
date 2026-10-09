package std

import (
	"math"
	"reflect"
	"testing"
	"time"

	"alder/rt"
	"alder/rt/html"
)

type item struct {
	Name string
	N    int64
}

func eq(t *testing.T, name string, got, want any) {
	t.Helper()
	if !reflect.DeepEqual(got, want) {
		t.Errorf("%s: got %#v, want %#v", name, got, want)
	}
}

func TestList_Basics(t *testing.T) {
	xs := []int64{1, 2, 3, 4}
	eq(t, "length", ListLength(xs), int64(4))
	eq(t, "isEmpty", ListIsEmpty([]int64{}), true)
	eq(t, "isEmpty nil", ListIsEmpty[int64](nil), true)
	eq(t, "head", ListHead(xs), rt.Just[int64](1))
	eq(t, "head empty", ListHead([]int64{}), rt.Nothing[int64]())
	eq(t, "last", ListLast(xs), rt.Just[int64](4))
	eq(t, "last empty", ListLast[int64](nil), rt.Nothing[int64]())
	eq(t, "reverse", ListReverse(xs), []int64{4, 3, 2, 1})
	eq(t, "reverse leaves input", xs, []int64{1, 2, 3, 4})
	eq(t, "sum", ListSum(xs), int64(10))
	eq(t, "sumFloat", ListSumFloat([]float64{0.5, 1.25}), 1.75)
	eq(t, "member", ListMember(int64(3), xs), true)
	eq(t, "member missing", ListMember(int64(9), xs), false)
	eq(t, "member struct", ListMember(item{"a", 1}, []item{{"b", 2}, {"a", 1}}), true)
	eq(t, "concat", ListConcat([][]int64{{1}, {}, {2, 3}}), []int64{1, 2, 3})
	eq(t, "append", ListAppend([]int64{1}, []int64{2}), []int64{1, 2})
}

func TestList_TakeDrop(t *testing.T) {
	xs := []int64{1, 2, 3}
	cases := []struct {
		n          int64
		take, drop []int64
	}{
		{-1, []int64{}, []int64{1, 2, 3}},
		{0, []int64{}, []int64{1, 2, 3}},
		{2, []int64{1, 2}, []int64{3}},
		{3, []int64{1, 2, 3}, []int64{}},
		{9, []int64{1, 2, 3}, []int64{}},
	}
	for _, c := range cases {
		eq(t, "take", ListTake(c.n, xs), c.take)
		eq(t, "drop", ListDrop(c.n, xs), c.drop)
	}
	tk := ListTake(1, xs)
	_ = append(tk, 99)
	eq(t, "take does not alias", xs, []int64{1, 2, 3})
}

func TestList_HigherOrder(t *testing.T) {
	xs := []int64{1, 2, 3, 4, 5}
	even := func(x int64) bool { return x%2 == 0 }
	eq(t, "filter", ListFilter(even, xs), []int64{2, 4})
	eq(t, "filter none", ListFilter(func(int64) bool { return false }, xs), []int64{})
	eq(t, "map", ListMap(func(x int64) string { return TextFromInt(x * 10) }, []int64{1, 2}), []string{"10", "20"})
	eq(t, "filterMap", ListFilterMap(TextToInt, []string{"1", "x", "3"}), []int64{1, 3})
	eq(t, "find", ListFind(even, xs), rt.Just[int64](2))
	eq(t, "find none", ListFind(func(x int64) bool { return x > 9 }, xs), rt.Nothing[int64]())
	eq(t, "any", ListAny(even, xs), true)
	eq(t, "any empty", ListAny(even, nil), false)
	eq(t, "all", ListAll(even, xs), false)
	eq(t, "all empty", ListAll(even, nil), true)
	// foldl : (a -> b -> b) -> b -> List a -> b, left to right
	eq(t, "foldl", ListFoldl(func(x int64, acc string) string { return acc + TextFromInt(x) }, ">", xs), ">12345")
	eq(t, "foldl sum", ListFoldl(func(x, acc int64) int64 { return acc + x }, 0, xs), int64(15))
}

func TestList_SortByStable(t *testing.T) {
	in := []item{{"c", 2}, {"a", 1}, {"d", 2}, {"b", 1}, {"e", 0}}
	got := ListSortBy(func(i item) int64 { return i.N }, in)
	want := []item{{"e", 0}, {"a", 1}, {"b", 1}, {"c", 2}, {"d", 2}}
	eq(t, "sortBy", got, want)
	eq(t, "sortBy leaves input", in[0], item{"c", 2})

	in2 := []item{{"b", 1}, {"a", 2}, {"b", 3}, {"a", 4}}
	got2 := ListSortByText(func(i item) string { return i.Name }, in2)
	eq(t, "sortByText", got2, []item{{"a", 2}, {"a", 4}, {"b", 1}, {"b", 3}})
}

func TestList_IndexedRangeUnique(t *testing.T) {
	eq(t, "indexed", ListIndexed([]string{"x", "y"}), []struct {
		Index int64
		Item  string
	}{{0, "x"}, {1, "y"}})
	eq(t, "range", ListRange(2, 5), []int64{2, 3, 4, 5})
	eq(t, "range single", ListRange(3, 3), []int64{3})
	eq(t, "range empty", ListRange(5, 2), []int64{})
	eq(t, "unique", ListUnique([]int64{3, 1, 3, 2, 1}), []int64{3, 1, 2})
	eq(t, "unique lists", ListUnique([][]int64{{1}, {1}, {2}}), [][]int64{{1}, {2}})
}

func TestList_GroupBy(t *testing.T) {
	in := []item{{"b", 1}, {"a", 2}, {"b", 3}}
	d := ListGroupBy(func(i item) string { return i.Name }, in)
	eq(t, "groupBy keys", d.Keys(), []string{"b", "a"})
	eq(t, "groupBy b", DictGet("b", d), rt.Just([]item{{"b", 1}, {"b", 3}}))
	eq(t, "groupBy a", DictGet("a", d), rt.Just([]item{{"a", 2}}))
}

func TestMaybe(t *testing.T) {
	j, n := rt.Just[int64](2), rt.Nothing[int64]()
	dbl := func(x int64) int64 { return x * 2 }
	eq(t, "map", MaybeMap(dbl, j), rt.Just[int64](4))
	eq(t, "map nothing", MaybeMap(dbl, n), rt.Nothing[int64]())
	half := func(x int64) rt.Maybe[int64] {
		if x%2 != 0 {
			return rt.Nothing[int64]()
		}
		return rt.Just(x / 2)
	}
	eq(t, "andThen", MaybeAndThen(half, j), rt.Just[int64](1))
	eq(t, "andThen odd", MaybeAndThen(half, rt.Just[int64](3)), rt.Nothing[int64]())
	eq(t, "withDefault", MaybeWithDefault(9, n), int64(9))
	eq(t, "isJust", MaybeIsJust(j), true)
	eq(t, "toList", MaybeToList(j), []int64{2})
	eq(t, "toList nothing", MaybeToList(n), []int64{})
}

func TestResult(t *testing.T) {
	ok, bad := rt.Ok[string, int64](2), rt.Err[string, int64]("e")
	dbl := func(x int64) int64 { return x * 2 }
	eq(t, "map", ResultMap(dbl, ok), rt.Ok[string, int64](4))
	eq(t, "map err", ResultMap(dbl, bad), rt.Err[string, int64]("e"))
	ln := func(s string) int64 { return TextLength(s) }
	eq(t, "mapErr", ResultMapErr(ln, bad), rt.Err[int64, int64](1))
	eq(t, "mapErr ok", ResultMapErr(ln, ok), rt.Ok[int64, int64](2))
	pos := func(x int64) rt.Result[string, string] {
		if x > 0 {
			return rt.Ok[string](TextFromInt(x))
		}
		return rt.Err[string, string]("neg")
	}
	eq(t, "andThen", ResultAndThen(pos, ok), rt.Ok[string, string]("2"))
	eq(t, "andThen err", ResultAndThen(pos, bad), rt.Err[string, string]("e"))
	eq(t, "withDefault", ResultWithDefault(0, bad), int64(0))
	eq(t, "toMaybe", ResultToMaybe(ok), rt.Just[int64](2))
	eq(t, "toMaybe err", ResultToMaybe(bad), rt.Nothing[int64]())
}

func TestText(t *testing.T) {
	eq(t, "length runes", TextLength("héllo"), int64(5))
	eq(t, "isEmpty", TextIsEmpty(""), true)
	eq(t, "trim", TextTrim("  a b \n"), "a b")
	eq(t, "upper", TextToUpper("abé"), "ABÉ")
	eq(t, "lower", TextToLower("ABÉ"), "abé")
	eq(t, "contains needle haystack", TextContains("ell", "hello"), true)
	eq(t, "contains reversed", TextContains("hello", "ell"), false)
	eq(t, "startsWith", TextStartsWith("he", "hello"), true)
	eq(t, "endsWith", TextEndsWith("lo", "hello"), true)
	eq(t, "fromFloat", TextFromFloat(3.5), "3.5")
	eq(t, "fromFloat whole", TextFromFloat(2), "2")
	eq(t, "toFloat", TextToFloat("2.5"), rt.Just(2.5))
	eq(t, "toFloat bad", TextToFloat("x"), rt.Nothing[float64]())
	eq(t, "toFloat NaN", TextToFloat("NaN"), rt.Nothing[float64]())
}

func TestText_SplitJoinReplace(t *testing.T) {
	cases := []struct {
		sep, s string
		want   []string
	}{
		{",", "a,b,,c", []string{"a", "b", "", "c"}},
		{",", "abc", []string{"abc"}},
		{", ", "x, y", []string{"x", "y"}},
	}
	for _, c := range cases {
		eq(t, "split "+c.s, TextSplit(c.sep, c.s), c.want)
	}
	eq(t, "join", TextJoin(", ", []string{"a", "b", "c"}), "a, b, c")
	eq(t, "join empty", TextJoin(", ", nil), "")
	// replace old new text
	eq(t, "replace", TextReplace("a", "o", "banana"), "bonono")
	eq(t, "replace none", TextReplace("z", "o", "banana"), "banana")
}

func TestText_Matches(t *testing.T) {
	cases := []struct {
		re, s string
		want  bool
	}{
		{"^[a-z0-9-]+$", "hello-world-2", true},
		{"^[a-z0-9-]+$", "Hello", false},
		{"ell", "hello", true}, // unanchored: finds a match anywhere
		{"(", "(", false},      // invalid regex never matches
	}
	for _, c := range cases {
		eq(t, "matches "+c.re+" "+c.s, TextMatches(c.re, c.s), c.want)
		eq(t, "matches cached "+c.re+" "+c.s, TextMatches(c.re, c.s), c.want)
	}
}

func TestText_Slice(t *testing.T) {
	cases := []struct {
		start, end int64
		s, want    string
	}{
		{0, 3, "héllo", "hél"},
		{1, 4, "héllo", "éll"},
		{2, 99, "héllo", "llo"},
		{-3, -1, "héllo", "ll"},
		{0, -1, "héllo", "héll"},
		{3, 1, "héllo", ""},
		{-99, 2, "héllo", "hé"},
		{0, 0, "", ""},
	}
	for _, c := range cases {
		eq(t, "slice "+c.s, TextSlice(c.start, c.end, c.s), c.want)
	}
}

func TestText_IntConversions(t *testing.T) {
	cases := []struct {
		s    string
		want rt.Maybe[int64]
	}{
		{"42", rt.Just[int64](42)},
		{"-7", rt.Just[int64](-7)},
		{"", rt.Nothing[int64]()},
		{"4.2", rt.Nothing[int64]()},
		{" 1", rt.Nothing[int64]()},
		{"abc", rt.Nothing[int64]()},
		{"99999999999999999999", rt.Nothing[int64]()},
	}
	for _, c := range cases {
		eq(t, "toInt "+c.s, TextToInt(c.s), c.want)
	}
	eq(t, "fromInt", TextFromInt(-12), "-12")
}

func TestIntFloat(t *testing.T) {
	r := struct {
		Min int64
		Max int64
	}{Min: 1, Max: 10}
	for _, c := range []struct{ in, want int64 }{{-5, 1}, {1, 1}, {5, 5}, {10, 10}, {11, 10}} {
		eq(t, "clamp", IntClamp(r, c.in), c.want)
	}
	eq(t, "toFloat", IntToFloat(3), 3.0)
	eq(t, "min", IntMin(3, 2), int64(2))
	eq(t, "max", IntMax(3, 2), int64(3))
	eq(t, "abs", IntAbs(-3), int64(3))

	eq(t, "round", FloatRound(2.5), int64(3))
	eq(t, "round neg", FloatRound(-2.4), int64(-2))
	eq(t, "floor", FloatFloor(-1.5), int64(-2))
	eq(t, "ceil", FloatCeil(1.1), int64(2))
	eq(t, "fmin", FloatMin(1.5, 2), 1.5)
	eq(t, "fmax", FloatMax(1.5, 2), 2.0)
	eq(t, "fabs", FloatAbs(-0.5), 0.5)
	if !math.IsInf(FloatAbs(math.Inf(-1)), 1) {
		t.Error("fabs inf")
	}
	cases := []struct {
		dec  int64
		f    float64
		want string
	}{
		{2, 3.14159, "3.14"},
		{0, 2.5, "2"}, // round-half-even, as strconv does
		{1, 2, "2.0"},
		{3, -0.5, "-0.500"},
		{-1, 7.6, "8"}, // negative decimals treated as 0
	}
	for _, c := range cases {
		eq(t, "format", FloatFormat(struct{ Decimals int64 }{c.dec}, c.f), c.want)
	}
	eq(t, "bool toText", BoolToText(true)+BoolToText(false), "TrueFalse")
}

func TestDate(t *testing.T) {
	d := rt.Date{Y: 2026, M: 10, D: 9}
	eq(t, "addDays", DateAddDays(30, d), rt.Date{Y: 2026, M: 11, D: 8})
	eq(t, "addDays neg", DateAddDays(-9, d), rt.Date{Y: 2026, M: 9, D: 30})
	eq(t, "addDays leap", DateAddDays(1, rt.Date{Y: 2028, M: 2, D: 28}), rt.Date{Y: 2028, M: 2, D: 29})
	eq(t, "diffDays", DateDiffDays(d, rt.Date{Y: 2026, M: 12, D: 25}), int64(77))
	eq(t, "diffDays neg", DateDiffDays(rt.Date{Y: 2026, M: 12, D: 25}, d), int64(-77))
	eq(t, "diffDays dst-free", DateDiffDays(rt.Date{Y: 2026, M: 3, D: 1}, rt.Date{Y: 2026, M: 4, D: 1}), int64(31))
	eq(t, "toText", DateToText(rt.Date{Y: 2026, M: 1, D: 5}), "2026-01-05")
	eq(t, "fromText", DateFromText("2026-10-08"), rt.Just(rt.Date{Y: 2026, M: 10, D: 8}))
	for _, bad := range []string{"2026-02-30", "2026-13-01", "26-10-08", "2026-10-8", "x", ""} {
		eq(t, "fromText "+bad, DateFromText(bad), rt.Nothing[rt.Date]())
	}
	eq(t, "format", DateFormat("Jan 2, 2006", d), "Oct 9, 2026")
	eq(t, "year", DateYear(d), int64(2026))
	eq(t, "month", DateMonth(d), int64(10))
	eq(t, "day", DateDay(d), int64(9))
}

func TestTime(t *testing.T) {
	eq(t, "epoch", TimeEpoch().Equal(time.Unix(0, 0)), true)
	ts := time.Date(2026, 10, 9, 23, 30, 0, 0, time.FixedZone("X", -5*3600))
	eq(t, "toDate is UTC", TimeToDate(ts), rt.Date{Y: 2026, M: 10, D: 10})
	eq(t, "format UTC", TimeFormat("2006-01-02 15:04", ts), "2026-10-10 04:30")
}

func TestTime_Ago(t *testing.T) {
	now := time.Date(2026, 10, 9, 12, 0, 0, 0, time.UTC)
	cases := []struct {
		d    time.Duration
		want string
	}{
		{0, "just now"},
		{30 * time.Second, "just now"},
		{time.Minute, "1 minute ago"},
		{5*time.Minute + 59*time.Second, "5 minutes ago"},
		{time.Hour, "1 hour ago"},
		{3 * time.Hour, "3 hours ago"},
		{24 * time.Hour, "1 day ago"},
		{6 * 24 * time.Hour, "6 days ago"},
		{30 * 24 * time.Hour, "1 month ago"},
		{90 * 24 * time.Hour, "3 months ago"},
		{365 * 24 * time.Hour, "1 year ago"},
		{800 * 24 * time.Hour, "2 years ago"},
		{-2 * time.Hour, "in 2 hours"},
		{-30 * time.Second, "just now"},
	}
	for _, c := range cases {
		eq(t, "ago "+c.d.String(), TimeAgo(now, now.Add(-c.d)), c.want)
	}
}

func TestDict(t *testing.T) {
	type kv = struct {
		Key   string
		Value int64
	}
	d := DictFromList([]kv{{"b", 1}, {"a", 2}, {"b", 3}})
	eq(t, "fromList keys", DictKeys(d), []string{"b", "a"})
	eq(t, "fromList last wins", DictGet("b", d), rt.Just[int64](3))
	eq(t, "toList", DictToList(d), []kv{{"b", 3}, {"a", 2}})
	eq(t, "size", DictSize(d), int64(2))
	d2 := DictInsert("c", 4, d)
	eq(t, "insert", DictValues(d2), []int64{3, 2, 4})
	eq(t, "insert persistent", DictSize(d), int64(2))
	d3 := DictRemove("b", d2)
	eq(t, "remove", DictKeys(d3), []string{"a", "c"})
	eq(t, "remove missing", DictKeys(DictRemove("zz", d3)), []string{"a", "c"})
	eq(t, "member", DictMember("a", d3), true)
	eq(t, "member removed", DictMember("b", d3), false)
	e := DictEmpty[string, int64]()
	eq(t, "empty size", DictSize(e), int64(0))
	eq(t, "empty get", DictGet("a", e), rt.Nothing[int64]())
	eq(t, "empty toList", DictToList(e), []kv{})
}

func TestHtml(t *testing.T) {
	eq(t, "none", html.Render(HtmlNone()), "")
	eq(t, "text escapes", html.Render(HtmlText("<a>")), "&lt;a&gt;")
	eq(t, "unsafe raw", html.Render(HtmlUnsafe("<a>")), "<a>")
}
