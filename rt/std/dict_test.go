package std

import (
	"testing"

	"alder/rt"
)

type kvS = struct {
	Key   string
	Value int64
}

func kv(ps ...any) []kvS {
	out := []kvS{}
	for i := 0; i < len(ps); i += 2 {
		out = append(out, kvS{ps[i].(string), int64(ps[i+1].(int))})
	}
	return out
}

func TestDict_Ops(t *testing.T) {
	d := DictFromList(kv("a", 1, "b", 2, "c", 3))
	orig := kv("a", 1, "b", 2, "c", 3)
	always := func(m rt.Maybe[int64]) func(rt.Maybe[int64]) rt.Maybe[int64] {
		return func(rt.Maybe[int64]) rt.Maybe[int64] { return m }
	}

	eq(t, "singleton", DictToList(DictSingleton("a", int64(1))), kv("a", 1))
	eq(t, "isEmpty empty", DictIsEmpty(DictEmpty[string, int64]()), true)
	eq(t, "isEmpty d", DictIsEmpty(d), false)
	eq(t, "member b", DictMember("b", d), true)
	eq(t, "member z", DictMember("z", d), false)

	eq(t, "update b", DictToList(DictUpdate("b", always(rt.Just(int64(20))), d)), kv("a", 1, "b", 20, "c", 3))
	eq(t, "update z", DictToList(DictUpdate("z", always(rt.Just(int64(9))), d)), kv("a", 1, "b", 2, "c", 3, "z", 9))
	eq(t, "update b nothing", DictToList(DictUpdate("b", always(rt.Nothing[int64]()), d)), kv("a", 1, "c", 3))
	eq(t, "update z nothing", DictToList(DictUpdate("z", always(rt.Nothing[int64]()), d)), orig)
	var seen []rt.Maybe[int64]
	rec := func(m rt.Maybe[int64]) rt.Maybe[int64] { seen = append(seen, m); return m }
	DictUpdate("b", rec, d)
	DictUpdate("z", rec, d)
	eq(t, "update arg", seen, []rt.Maybe[int64]{rt.Just(int64(2)), rt.Nothing[int64]()})

	m := DictMap(func(k string, v int64) string { return k + string(rune('0'+v)) }, d)
	eq(t, "map", DictToList(m), []struct {
		Key   string
		Value string
	}{{"a", "a1"}, {"b", "b2"}, {"c", "c3"}})
	odd := func(_ string, v int64) bool { return v%2 == 1 }
	eq(t, "filter", DictToList(DictFilter(odd, d)), kv("a", 1, "c", 3))
	eq(t, "foldl", DictFoldl(func(k string, _ int64, acc string) string { return acc + k }, "", d), "abc")
	p := DictPartition(odd, d)
	eq(t, "partition yes", DictToList(p.Yes), kv("a", 1, "c", 3))
	eq(t, "partition no", DictToList(p.No), kv("b", 2))

	a := DictFromList(kv("x", 1, "y", 2))
	b := DictFromList(kv("y", 20, "z", 30))
	eq(t, "union", DictToList(DictUnion(a, b)), kv("x", 1, "y", 2, "z", 30))
	eq(t, "intersect", DictToList(DictIntersect(a, b)), kv("y", 2))
	eq(t, "diff", DictToList(DictDiff(a, b)), kv("x", 1))
	e := DictEmpty[string, int64]()
	eq(t, "union empties", DictIsEmpty(DictUnion(e, e)), true)

	eq(t, "d unchanged", DictToList(d), orig)
	eq(t, "a unchanged", DictToList(a), kv("x", 1, "y", 2))
	eq(t, "b unchanged", DictToList(b), kv("y", 20, "z", 30))
}
