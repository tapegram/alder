package rtesting

import (
	"bytes"
	"crypto/sha1"
	"encoding/hex"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"alder/rt/eff"
	"alder/rt/fuzz"
	"alder/rt/web"
)

func TestForAll(t *testing.T) {
	p := ForAll(fuzz.IntF(), func(n int64) bool { return n >= 0 })
	vs := p.Draw(fuzz.Replay(make([]uint64, 8)))
	if len(vs) != 1 || vs[0].(int64) != 0 || !p.Pure(vs) || p.Task != nil {
		t.Fatalf("ForAll: %v", vs)
	}
	q := ForAllTask(fuzz.BoolF(), func(b bool) eff.Task[bool] { return eff.Pure(!b) })
	if vs := q.Draw(fuzz.Replay([]uint64{1})); vs[0] != true || q.Pure != nil || q.Task == nil {
		t.Fatalf("ForAllTask: %v", vs)
	}
}

// listProp is a property over one `xs : List Int` holding when holds says so.
func listProp(holds func(xs []int64) bool) *Property {
	p := ForAll(fuzz.List(fuzz.IntF()), holds)
	p.Args = []Arg{{Name: "xs", Type: "List Int", Defaulted: true}}
	p.Lit = func(vs []any) []string { return []string{fuzz.ListLit(vs[0].([]int64), fuzz.IntLit)} }
	return p
}

func runProp(p *Property, o Options) CaseResult {
	if o.Tables == nil {
		o.Tables = tables
	}
	rs := Run([]Case{{Module: "M", Name: "p", Prop: p}}, o)
	return rs[0]
}

func report(t *testing.T, r CaseResult, asJSON bool) string {
	t.Helper()
	var b bytes.Buffer
	if err := WriteResult(&b, r, asJSON); err != nil {
		t.Fatal(err)
	}
	return b.String()
}

func reversed(xs []int64) []int64 {
	ys := slices.Clone(xs)
	slices.Reverse(ys)
	return ys
}

func TestProp_Passes(t *testing.T) {
	calls := 0
	r := runProp(listProp(func(xs []int64) bool {
		calls++
		return slices.Equal(reversed(reversed(xs)), xs)
	}), Options{})
	if !r.OK || r.Runs != 100 || calls != 100 || r.Run != 0 || r.Args != nil {
		t.Fatalf("got %+v (calls %d)", r, calls)
	}
	if got := report(t, r, false); got != "ok   M \"p\"\n" {
		t.Errorf("report %q", got)
	}
	if got := report(t, r, true); !strings.Contains(got, `"property":{"runs":100,"seed":`) {
		t.Errorf("json %s", got)
	}
}

func TestProp_ShrinksAndReports(t *testing.T) {
	r := runProp(listProp(func(xs []int64) bool { return len(xs) < 2 }), Options{})
	if r.OK || len(r.Args) != 1 || r.Args[0].Value != "[0, 0]" || r.Args[0].Name != "xs" || r.Run < 1 || r.Shrinks == 0 {
		t.Fatalf("got %+v", r)
	}
	out := report(t, r, false)
	for _, want := range []string{"FAIL M \"p\"  (failed on run ", " of 100, shrunk in ", "  xs: [0, 0]  (xs : List Int)\n", "  seed: "} {
		if !strings.Contains(out, want) {
			t.Errorf("report lacks %q:\n%s", want, out)
		}
	}
	js := report(t, r, true)
	for _, want := range []string{`"property":{`, `"args":[{"name":"xs","type":"List Int","value":"[0, 0]","defaulted":true}]`} {
		if !strings.Contains(js, want) {
			t.Errorf("json lacks %q:\n%s", want, js)
		}
	}
}

// The sides of the `==` are those of the shrunk case: the runner re-runs it once after shrinking.
func TestProp_LeftRight(t *testing.T) {
	r := runProp(listProp(func(xs []int64) bool { return Eq(reversed(xs), xs) }), Options{})
	if r.OK || len(r.Args) != 1 {
		t.Fatalf("got %+v", r)
	}
	if r.Right != r.Args[0].Value || r.Left == r.Right || r.Args[0].Value != "[0, 1]" || r.Left != "[1, 0]" {
		t.Fatalf("left %q right %q args %q", r.Left, r.Right, r.Args[0].Value)
	}
	out := report(t, r, false)
	if !strings.Contains(out, "  xs: [0, 1]  (xs : List Int)\n  left:  [1, 0]\n  right: [0, 1]\n") {
		t.Errorf("report:\n%s", out)
	}
}

// Review Focus 1: a brand whose predicate rejects nearly every draw fails, quickly, naming it.
func TestProp_FilterTooTight(t *testing.T) {
	start := time.Now()
	p := ForAll(fuzz.Filter("Slug", func(s string) bool { return len(s) == 5 }, fuzz.TextF()), func(string) bool { return true })
	p.Args = []Arg{{Name: "s", Type: "Slug"}}
	r := runProp(p, Options{})
	if r.OK || !strings.Contains(r.Error, "`Slug`'s predicate rejects ") || !strings.Contains(r.Error, "write a `Fuzz.forAll`") {
		t.Fatalf("got %+v", r)
	}
	if d := time.Since(start); d > 2*time.Second {
		t.Errorf("took %s", d)
	}
}

func TestProp_TooSlow(t *testing.T) {
	defer func(d time.Duration) { attemptLimit = d }(attemptLimit)
	attemptLimit = 50 * time.Millisecond
	calls := 0
	p := ForAll(fuzz.IntF(), func(int64) bool {
		calls++
		if calls == 3 {
			time.Sleep(80 * time.Millisecond)
		}
		return true
	})
	p.Args = []Arg{{Name: "n", Type: "Int"}}
	r := runProp(p, Options{})
	if r.OK || r.Run != 3 || !strings.Contains(r.Error, "run 3 took ") || r.Shrinks != 0 || calls != 3 {
		t.Fatalf("got %+v (calls %d)", r, calls)
	}
}

func TestProp_DrawsTooMuch(t *testing.T) {
	big := fuzz.Fuzzer[int64](func(c *fuzz.Choices) int64 {
		for range 9000 {
			c.Choice(1)
		}
		return 0
	})
	p := ForAll(big, func(int64) bool { return true })
	r := runProp(p, Options{})
	if r.OK || !strings.Contains(r.Error, "generator draws too much") || r.Run != 1 {
		t.Fatalf("got %+v", r)
	}
}

func TestProp_SeedReproducible(t *testing.T) {
	fails := func(xs []int64) bool { return len(xs) < 3 || xs[0] < xs[1] }
	o := Options{Seed: 7, SeedSet: true}
	a, b := runProp(listProp(fails), o), runProp(listProp(fails), o)
	if a.OK || a.Seed != 7 || !slices.Equal(a.Args, b.Args) || a.Run != b.Run || a.Shrinks != b.Shrinks {
		t.Fatalf("not reproducible:\n%+v\n%+v", a, b)
	}
	// without a seed each run picks one
	c, d := runProp(listProp(fails), Options{}), runProp(listProp(fails), Options{})
	if c.Seed == d.Seed {
		t.Errorf("two runs drew the same seed %d", c.Seed)
	}
}

func TestProp_SavedFailureReplayedFirst(t *testing.T) {
	dbDir := t.TempDir()
	broken := true
	prop := func() *Property { return listProp(func(xs []int64) bool { return !broken || len(xs) < 2 }) }
	sum := sha1.Sum([]byte("p"))
	file := filepath.Join(dbDir, "M", hex.EncodeToString(sum[:])[:16])

	if r := runProp(prop(), Options{FuzzDB: dbDir, Seed: 1, SeedSet: true}); r.OK {
		t.Fatalf("want a failure: %+v", r)
	}
	if _, err := os.Stat(file); err != nil {
		t.Fatalf("no saved failure: %v", err)
	}
	// seed 2 alone does not fail on its first run; with the saved failure it does
	if r := runProp(prop(), Options{Seed: 2, SeedSet: true}); r.OK || r.Run == 1 {
		t.Fatalf("seed 2 without the saved failure: %+v", r)
	}
	r := runProp(prop(), Options{FuzzDB: dbDir, Seed: 2, SeedSet: true})
	if r.OK || r.Run != savedRun || r.Args[0].Value != "[0, 0]" {
		t.Fatalf("saved failure not replayed first: %+v", r)
	}
	out := report(t, r, false)
	for _, want := range []string{"(failed on the saved case", "  run: the saved failing case, replayed first (build/<app>/fuzz/M/)\n"} {
		if !strings.Contains(out, want) {
			t.Errorf("report lacks %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "failed on run") || strings.Contains(out, "seed:") {
		t.Errorf("report names a run or seed for the saved case:\n%s", out)
	}
	broken = false
	if r := runProp(prop(), Options{FuzzDB: dbDir}); !r.OK {
		t.Fatalf("fixed property fails: %+v", r)
	}
	if _, err := os.Stat(file); !os.IsNotExist(err) {
		t.Fatalf("the saved failure was not deleted: %v", err)
	}
}

func TestProp_Mutating(t *testing.T) {
	dbDir := t.TempDir()
	r := runProp(listProp(func(xs []int64) bool { return len(xs) < 2 }), Options{FuzzDB: dbDir, Mutating: true})
	if r.OK || r.Shrinks != 0 || r.Seed != 0 {
		t.Fatalf("got %+v", r)
	}
	if es, _ := os.ReadDir(dbDir); len(es) != 0 {
		t.Errorf("wrote %v", es)
	}
	if out := report(t, r, false); strings.Contains(out, "shrunk") {
		t.Errorf("report:\n%s", out)
	}
}

// A Go panic inside an attempt is a failure and shrinks like one.
func TestProp_Panic(t *testing.T) {
	r := runProp(listProp(func(xs []int64) bool { return xs[1] >= 0 || true }), Options{})
	if r.OK || len(r.Args) != 1 || r.Args[0].Value != "[]" || !strings.Contains(r.Error, "index out of range") || !r.panicked {
		t.Fatalf("got %+v", r)
	}
}

// Review Focus 4: every attempt of an effect property, shrink candidates included, starts from
// an empty store.
func TestProp_EffectIsolation(t *testing.T) {
	attempts, dirty := 0, 0
	prop := func(failAt int64) *Property {
		p := ForAllTask(fuzz.IntF(), func(n int64) eff.Task[bool] {
			attempts++
			return eff.Bind(countStep(), func(before int64) eff.Task[bool] {
				if before != 0 {
					dirty++
				}
				return eff.Bind(insertStep("Db.insert T", "x"), func(string) eff.Task[bool] {
					return eff.Pure(before == 0 && (failAt == 0 || n < failAt)) // failAt 0: never fails
				})
			})
		})
		p.Args = []Arg{{Name: "n", Type: "Int"}}
		p.Lit = func(vs []any) []string { return []string{fuzz.IntLit(vs[0].(int64))} }
		return p
	}
	start := time.Now()
	r := runProp(prop(0), Options{})
	if !r.OK || attempts != 100 {
		t.Fatalf("got %+v (attempts %d)", r, attempts)
	}
	t.Logf("%.2f ms per attempt (fresh store each)", float64(time.Since(start).Microseconds())/1000/float64(attempts))
	attempts = 0
	r = runProp(prop(5), Options{Seed: 3, SeedSet: true})
	if r.OK || r.Args[0].Value != "5" || r.Shrinks == 0 || dirty != 0 {
		t.Fatalf("got %+v (dirty %d)", r, dirty)
	}
	if !slices.Equal(r.Trace, []string{"Db.count T", "Db.insert T"}) {
		t.Errorf("trace %v", r.Trace)
	}
}

// Main reads the run count, the seed and the saved-failure directory from flags or the
// environment.
func TestMain_PropertyFlags(t *testing.T) {
	calls := 0
	cases := []Case{{Module: "M", Name: "p", Prop: listProp(func(xs []int64) bool { calls++; return len(xs) < 2 })}}
	var out, errb bytes.Buffer
	code := Main(cases, tables, nil, nil, nil, nil, nil, web.Options{}, []string{"-fuzz", "5", "-seed", "3"}, &out, &errb)
	if code != 1 || !strings.Contains(out.String(), "  seed: 3\n") {
		t.Fatalf("exit %d\n%s%s", code, out.String(), errb.String())
	}
	dbDir := t.TempDir()
	t.Setenv(EnvFuzz, "7")
	t.Setenv(EnvFuzzDB, dbDir)
	t.Setenv(EnvSeed, "12")
	out.Reset()
	calls = 0
	cases[0].Prop = listProp(func(xs []int64) bool { calls++; return true })
	if code := Main(cases, tables, nil, nil, nil, nil, nil, web.Options{}, nil, &out, &errb); code != 0 || calls != 7 {
		t.Fatalf("exit %d, %d calls\n%s%s", code, calls, out.String(), errb.String())
	}
	for _, bad := range [][]string{{"-fuzz", "0"}, {"-fuzz", "x"}, {"-seed", "-1"}} {
		if code := Main(cases, tables, nil, nil, nil, nil, nil, web.Options{}, bad, &out, &errb); code != 2 {
			t.Errorf("%v: exit %d", bad, code)
		}
	}
}
