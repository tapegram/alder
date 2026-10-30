package check

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"alder/internal/syntax"
	"alder/internal/types"
)

// Hand-built types and patterns for the usefulness matrix (#2): no source, no unifier.

var (
	tMaybeInt = &types.Union{Name: "MaybeInt", Ctors: []types.Ctor{{Name: "Nothing"}, {Name: "Just", Arg: types.Int}}}
	tFilter   = &types.Union{Name: "Filter", Ctors: []types.Ctor{{Name: "All"}, {Name: "Active"}, {Name: "Completed"}}}
	tBool     = &types.Con{Name: "Bool"}
)

func listOf(t types.Type) types.Type { return &types.App{Con: "List", Args: []types.Type{t}} }

func pv(n string) syntax.Pattern { return &syntax.PVar{Name: n} }
func pc(n string, arg syntax.Pattern) syntax.Pattern {
	return &syntax.PCtor{Name: n, Arg: arg}
}
func pl(rest string, elems ...syntax.Pattern) syntax.Pattern {
	return &syntax.PList{Elems: elems, Rest: rest}
}
func pi(n int64) syntax.Pattern { return &syntax.PLit{Lit: &syntax.IntLit{Value: n}} }
func pb(b bool) syntax.Pattern  { return &syntax.PLit{Lit: &syntax.BoolLit{Value: b}} }

func testMatcher() *matcher { return &matcher{apply: func(t types.Type) types.Type { return t }} }

func rowsOf(m *matcher, t types.Type, ps ...syntax.Pattern) [][]*pat {
	var rows [][]*pat
	for _, p := range ps {
		rows = append(rows, []*pat{m.normalise(p, t)})
	}
	return rows
}

// witAll lists the witnesses with a cap no test reaches.
func witAll(m *matcher, rows [][]*pat, t types.Type) []*pat {
	ws, _ := m.witnesses(rows, t, 1000)
	return ws
}

func printed(ws []*pat) string {
	var out []string
	for _, w := range ws {
		out = append(out, printWitness(w, "x"))
	}
	return strings.Join(out, ", ")
}

func TestUseful_ListOfMaybe(t *testing.T) {
	m := testMatcher()
	ty := listOf(tMaybeInt)
	rows := rowsOf(m, ty, pl("", pc("Just", pv("m")), pc("Just", pv("s")), pc("Just", pv("p"))))
	ws := witAll(m, rows, ty)
	want := "[], [Nothing, ..rest], [Just _], [Just _, Nothing, ..rest], [Just _, Just _], " +
		"[Just _, Just _, Nothing, ..rest], [Just _, Just _, Just _, _, ..rest]"
	if got := printed(ws); got != want {
		t.Fatalf("witnesses = %s\nwant %s", got, want)
	}
}

func TestUseful_FlatUnion(t *testing.T) {
	m := testMatcher()
	rows := rowsOf(m, tFilter, pc("All", nil))
	if ws := witAll(m, rows, tFilter); printed(ws) != "Active, Completed" {
		t.Fatalf("witnesses = %s", printed(ws))
	}
	if m.useful(rows, []*pat{m.normalise(pc("All", nil), tFilter)}, []types.Type{tFilter}) {
		t.Error("a second All is useful")
	}
	if !m.useful(rows, []*pat{m.normalise(pc("Active", nil), tFilter)}, []types.Type{tFilter}) {
		t.Error("Active is not useful")
	}
	// the top-level spellings of today's messages
	rows = rowsOf(m, tMaybeInt)
	if ws := witAll(m, rows, tMaybeInt); printed(ws) != "Nothing, Just x" {
		t.Fatalf("witnesses = %s", printed(ws))
	}
	rows = rowsOf(m, listOf(types.Int), pl(""))
	if ws := witAll(m, rows, listOf(types.Int)); printed(ws) != "[x, ..rest]" {
		t.Fatalf("witnesses = %s", printed(ws))
	}
	rows = rowsOf(m, tBool, pb(true))
	if ws := witAll(m, rows, tBool); printed(ws) != "False" {
		t.Fatalf("witnesses = %s", printed(ws))
	}
}

func TestUseful_Literals(t *testing.T) {
	m := testMatcher()
	rows := rowsOf(m, types.Int, pi(0), pi(1))
	if ws := witAll(m, rows, types.Int); printed(ws) != "_" {
		t.Fatalf("witnesses = %s", printed(ws))
	}
	tys := []types.Type{types.Int}
	if m.useful(rows, []*pat{m.normalise(pi(0), types.Int)}, tys) || !m.useful(rows, []*pat{m.normalise(pi(2), types.Int)}, tys) {
		t.Error("literal usefulness")
	}
}

func TestUseful_Record(t *testing.T) {
	m := testMatcher()
	rec := &types.Record{Fields: []types.Field{{Name: "a", Type: tMaybeInt}, {Name: "b", Type: tBool}}, Anonymous: true}
	rows := rowsOf(m, rec,
		&syntax.PRecord{Fields: []*syntax.PField{{Name: "a", Pat: pc("Just", pv("x"))}}},
		&syntax.PRecord{Fields: []*syntax.PField{{Name: "b", Pat: pb(true)}}})
	if ws := witAll(m, rows, rec); printed(ws) != "{ a: Nothing, b: False }" {
		t.Fatalf("witnesses = %s", printed(ws))
	}
}

func TestUseful_Redundant(t *testing.T) {
	m := testMatcher()
	ty := listOf(tMaybeInt)
	rows := rowsOf(m, ty, pl("", pc("Just", pv("x"))))
	tys := []types.Type{ty}
	if m.useful(rows, []*pat{m.normalise(pl("", pc("Just", pi(1))), ty)}, tys) {
		t.Error("[Just 1] after [Just x] is useful")
	}
	if !m.useful(rows, []*pat{m.normalise(pl("r", pc("Just", pi(1))), ty)}, tys) {
		t.Error("[Just 1, ..r] after [Just x] is not useful")
	}
	// Just (Just x) after Just Nothing: Just (Just _) is still missing, then nothing is
	mm := &types.Union{Name: "MM", Ctors: []types.Ctor{{Name: "Nothing"}, {Name: "Just", Arg: tMaybeInt}}}
	rows = rowsOf(m, mm, pc("Nothing", nil), pc("Just", pc("Nothing", nil)))
	if ws := witAll(m, rows, mm); printed(ws) != "Just (Just _)" {
		t.Fatalf("witnesses = %s", printed(ws))
	}
}

// Witnesses are never capped (the message caps nested lists, see checkArms): a ten-constructor
// union without arms has ten, all flat.
func TestWitnessesUncapped(t *testing.T) {
	m := testMatcher()
	u := &types.Union{Name: "Ten"}
	for i := range 10 {
		u.Ctors = append(u.Ctors, types.Ctor{Name: fmt.Sprintf("C%d", i)})
	}
	ws := witAll(m, nil, u)
	if len(ws) != 10 || printed(ws[:2]) != "C0, C1" {
		t.Fatalf("witnesses = %s", printed(ws))
	}
	for _, w := range ws {
		if !w.flat() {
			t.Errorf("%s is not flat", printWitness(w, "x"))
		}
	}
}

// Nested witnesses can be exponentially many: a record of 2m fields of `A | B | D` with the m
// arms `{ ai: A, bi: A }` has 4^m. The list stops at the cap, so checking stays fast (#2).
func TestWitnessesStopAtCapWhenNested(t *testing.T) {
	for _, m := range []int{7, 10} {
		src := "module M\n\ntype C = A | B | D\n\nq : {"
		var fields, arms []string
		for i := range m {
			fields = append(fields, fmt.Sprintf(" a%d: C, b%d: C", i, i))
			arms = append(arms, fmt.Sprintf("{ a%d: A, b%d: A } -> %d", i, i, i))
		}
		src += strings.Join(fields, ",") + " } -> Int\nq r = match r { " + strings.Join(arms, ", ") + " }\n"
		start := time.Now()
		diags := checkModule(t, src)
		if took := time.Since(start); took > 5*time.Second {
			t.Errorf("m=%d: checking took %v", m, took)
		}
		if len(diags) != 1 || diags[0].Code != "E0004" || !strings.HasSuffix(diags[0].Message, ", …") || diags[0].Fix == nil {
			t.Fatalf("m=%d: diagnostics %+v", m, diags)
		}
	}
}

// A witness with several list tails numbers them, so its fix arm binds each name once.
func TestWitnessRestsNumbered(t *testing.T) {
	m := testMatcher()
	rec := &types.Record{Fields: []types.Field{{Name: "a", Type: listOf(tMaybeInt)}, {Name: "b", Type: listOf(tMaybeInt)}}, Anonymous: true}
	rows := rowsOf(m, rec,
		&syntax.PRecord{Fields: []*syntax.PField{{Name: "a", Pat: pl("")}}},
		&syntax.PRecord{Fields: []*syntax.PField{{Name: "a", Pat: pl("r", pc("Just", pv("x")))}}},
		&syntax.PRecord{Fields: []*syntax.PField{{Name: "b", Pat: pl("")}}},
		&syntax.PRecord{Fields: []*syntax.PField{{Name: "b", Pat: pl("r", pc("Just", pv("x")))}}})
	if ws := witAll(m, rows, rec); printed(ws) != "{ a: [Nothing, ..rest], b: [Nothing, ..rest2] }" {
		t.Fatalf("witnesses = %s", printed(ws))
	}
}

// The issue's match and a 20-arm flat update: usefulness stays cheap (#2).
func BenchmarkUseful(b *testing.B) {
	m := testMatcher()
	ty := listOf(tMaybeInt)
	issue := rowsOf(m, ty, pl("", pc("Just", pv("m")), pc("Just", pv("s")), pc("Just", pv("p"))))
	msg := &types.Union{Name: "Msg"}
	var arms []syntax.Pattern
	for i := range 20 {
		msg.Ctors = append(msg.Ctors, types.Ctor{Name: fmt.Sprintf("M%d", i), Arg: types.Int})
		arms = append(arms, pc(fmt.Sprintf("M%d", i), pv("p")))
	}
	flat := rowsOf(m, msg, arms...)
	for b.Loop() {
		witAll(m, issue, ty)
		for i := range flat {
			m.useful(flat[:i], flat[i], []types.Type{msg})
		}
		witAll(m, flat, msg)
	}
}
