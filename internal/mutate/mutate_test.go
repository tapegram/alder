package mutate

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"alder/internal/check"
	"alder/internal/diag"
	"alder/internal/ir"
	"alder/internal/names"
	"alder/internal/types"
)

var update = flag.Bool("update", false, "rewrite golden files")

// lower loads, resolves, checks and lowers the project dir (holding app/).
func lower(t *testing.T, dir string) *ir.Program {
	t.Helper()
	std, err := types.LoadStd()
	if err != nil {
		t.Fatal(err)
	}
	prog, bag, ioErrs := names.Load(dir, std)
	for _, e := range ioErrs {
		t.Fatal(e)
	}
	if prog.ParseFailed {
		t.Fatalf("%s does not parse:\n%s", dir, errs(bag))
	}
	prog.Resolve(bag)
	prog.MarkFailed(bag)
	res := check.Program(prog, bag)
	if bag.HasErrors() {
		t.Fatalf("%s does not check:\n%s", dir, errs(bag))
	}
	return ir.Lower(prog, res)
}

func errs(bag *diag.Bag) string {
	var sb strings.Builder
	for _, d := range bag.Items {
		if d.Severity == "error" {
			fmt.Fprintf(&sb, "%s:%d:%d %s %s\n", d.Span.File, d.Span.Start.Line, d.Span.Start.Col, d.Code, d.Message)
		}
	}
	return sb.String()
}

// table renders the mutant table: `id  module  decl:line  kind  before → after`, then each
// refused mutant as `-  module  decl:line  kind  equivalent|skipped: why`, in aligned columns.
func table(ms []Mutant, ss []Suppressed) string {
	var rows [][]string
	for _, m := range ms {
		rows = append(rows, []string{strconv.Itoa(m.ID), m.Module, m.Decl + ":" + strconv.Itoa(m.Line), m.Kind, m.Before + " → " + m.After})
	}
	for _, s := range ss {
		rows = append(rows, []string{"-", s.Module, s.Decl + ":" + strconv.Itoa(s.Line), s.Kind, s.Category + ": " + s.Why})
	}
	widths := make([]int, 4)
	for _, r := range rows {
		for i := range widths {
			widths[i] = max(widths[i], len([]rune(r[i])))
		}
	}
	var sb strings.Builder
	for _, r := range rows {
		for i := range widths {
			sb.WriteString(r[i] + strings.Repeat(" ", widths[i]-len([]rune(r[i]))+2))
		}
		sb.WriteString(r[4] + "\n")
	}
	return sb.String()
}

func compareGolden(t *testing.T, golden, got string) {
	t.Helper()
	if *update {
		if err := os.WriteFile(golden, []byte(got), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(golden)
	if err != nil {
		t.Fatalf("missing golden %s (run with -update)", golden)
	}
	if got != string(want) {
		t.Fatalf("golden mismatch for %s\n got:\n%s\nwant:\n%s", golden, got, want)
	}
}

// TestGolden: per operator (and the todos example under all/), the fixture's mutant table.
// Every operator's fixture has at least one mutant of its kind, and one refused mutant of its
// kind when the spec lists a suppression rule for it.
func TestGolden(t *testing.T) {
	dirs, _ := filepath.Glob("../../testdata/mutate/*")
	if len(dirs) == 0 {
		t.Fatal("no fixtures under testdata/mutate")
	}
	kinds := map[string]string{
		"requires-dropped": RequiresDropped, "guard-dropped": GuardDropped, "where-dropped": WhereDropped,
		"orderby-reversed": OrderByReversed, "limit-dropped": LimitDropped, "offset-dropped": OffsetDropped,
		"write-replaced": WriteReplaced, "update-field-dropped": UpdateFieldDropped,
		"redirect-rerender": RedirectRerender, "reject-rerender": RejectRerender, "arm-swapped": ArmSwapped,
		"condition-negated": ConditionNegated, "operator-flipped": OperatorFlipped, "bool-flipped": BoolFlipped,
		"default-changed": DefaultChanged, "cascade-restrict": CascadeRestrict,
		"not-dropped": NotDropped, "session-put-dropped": SessionPutDropped,
	}
	// the kinds the spec gives a suppression rule (plus the rulings': requires, cascade's setNull)
	refusable := map[string]bool{RequiresDropped: true, GuardDropped: true, ArmSwapped: true, OrderByReversed: true,
		ConditionNegated: true, OperatorFlipped: true, DefaultChanged: true, CascadeRestrict: true}
	covered := map[string]bool{}
	for _, dir := range dirs {
		name := filepath.Base(dir)
		t.Run(name, func(t *testing.T) {
			ms, ss := Collect(lower(t, dir))
			compareGolden(t, filepath.Join(dir, "want.txt"), table(ms, ss))
			kind, ok := kinds[name]
			if !ok {
				return
			}
			covered[kind] = true
			n, s := 0, 0
			for _, m := range ms {
				if m.Kind == kind {
					n++
				}
			}
			for _, x := range ss {
				if x.Kind == kind {
					s++
				}
			}
			if n == 0 {
				t.Errorf("no %q mutant", kind)
			}
			if refusable[kind] && s == 0 {
				t.Errorf("no refused %q mutant", kind)
			}
		})
	}
	for _, k := range kinds {
		if !covered[k] {
			t.Errorf("no fixture for %q", k)
		}
	}
}

// TestNeverInTestsFakesViews: no mutant's node is reachable from a test, a fake entry, a view,
// a component or an Html value, except a default's (shared by every literal omitting it).
func TestNeverInTestsFakesViews(t *testing.T) {
	for _, dir := range []string{"../../testdata/mutate/scope", "../../testdata/mutate/all", "../../examples/blog"} {
		p := lower(t, dir)
		ms, _ := Collect(p)
		forbidden := map[ir.Expr]bool{}
		var mark func(e ir.Expr)
		mark = func(e ir.Expr) {
			if isNil(e) || forbidden[e] {
				return
			}
			forbidden[e] = true
			for _, k := range ir.Children(e) {
				mark(k)
			}
		}
		for _, m := range p.Modules {
			for _, x := range m.Tests {
				mark(x.Body)
			}
			for _, v := range m.Components {
				mark(v.Body)
			}
			for _, v := range m.Values {
				if isHtml(v.Type) {
					mark(v.Body)
				}
			}
		}
		for _, pg := range p.Pages {
			if pg.View != nil {
				mark(pg.View.Body)
			}
		}
		for _, f := range p.Fakes {
			for _, v := range f.Entries {
				mark(v.Body)
			}
		}
		for _, m := range ms {
			if m.Node != nil && forbidden[m.Node] && m.Kind != DefaultChanged {
				t.Errorf("%s: mutant %d (%s %s) is inside a test, fake or view", dir, m.ID, m.Kind, m.Before)
			}
			if m.Replacement == nil && m.Kind != CascadeRestrict {
				t.Errorf("%s: mutant %d (%s) has no Replacement", dir, m.ID, m.Kind)
			}
			if m.Replacement != nil && m.Node != nil && m.Replacement.Type() != m.Node.Type() && types.String(m.Replacement.Type()) != types.String(m.Node.Type()) {
				t.Errorf("%s: mutant %d (%s): Replacement has type %s, Node %s", dir, m.ID, m.Kind, types.String(m.Replacement.Type()), types.String(m.Node.Type()))
			}
			if contains(m.Replacement, m.Node) {
				t.Errorf("%s: mutant %d (%s): Replacement contains Node", dir, m.ID, m.Kind)
			}
		}
		if len(ms) == 0 {
			t.Errorf("%s: no mutants", dir)
		}
	}
}

func contains(e, node ir.Expr) bool {
	if isNil(e) || node == nil {
		return false
	}
	if e == node {
		return true
	}
	for _, k := range ir.Children(e) {
		if contains(k, node) {
			return true
		}
	}
	return false
}

// TestIDsStable: the same program twice gives the same table; a declaration added above
// renumbers the mutants below it and changes nothing else about them.
func TestIDsStable(t *testing.T) {
	a, _ := Collect(lower(t, "../../testdata/mutate/all"))
	b, _ := Collect(lower(t, "../../testdata/mutate/all"))
	if table(a, nil) != table(b, nil) {
		t.Fatalf("two runs differ:\n%s\n%s", table(a, nil), table(b, nil))
	}

	// copy the stability fixture, then add a helper with one mutant above `later`
	dir := t.TempDir()
	src, err := os.ReadFile("../../testdata/mutate/stability/app/Calc.alder")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "app"), 0o755); err != nil {
		t.Fatal(err)
	}
	write := func(s string) {
		if err := os.WriteFile(filepath.Join(dir, "app", "Calc.alder"), []byte(s), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write(string(src))
	before, _ := Collect(lower(t, dir))
	edited := strings.Replace(string(src), "-- ADD HERE\n", "added : Int -> Bool\nadded n = n > 2\n\n", 1)
	if edited == string(src) {
		t.Fatal("the fixture has no `-- ADD HERE` marker")
	}
	write(edited)
	after, _ := Collect(lower(t, dir))
	if len(after) != len(before)+1 {
		t.Fatalf("after the edit: %d mutants, want %d", len(after), len(before)+1)
	}
	split := -1
	for i, m := range after {
		if m.Decl == "added" {
			split = i
		}
	}
	if split < 1 {
		t.Fatalf("the added declaration's mutant is not after the first: %s", table(after, nil))
	}
	for i, m := range before {
		got := after[i]
		lines := 0
		if i >= split {
			got, lines = after[i+1], 2
		}
		if i < split && got.ID != m.ID || i >= split && got.ID != m.ID+1 {
			t.Errorf("mutant %d (%s %s) is now %d", m.ID, m.Decl, m.Kind, got.ID)
		}
		if got.Decl != m.Decl || got.Kind != m.Kind || got.Before != m.Before || got.Line != m.Line+lines {
			t.Errorf("mutant %d changed: %+v → %+v", m.ID, m, got)
		}
	}
}

// TestLossyText: two renderings that leave HTML out never compare equal, so an arm swap or a
// flip is not refused for bodies that differ only inside HTML.
func TestLossyText(t *testing.T) {
	a := &ir.Elem{Tag: "p", Children: []ir.Expr{&ir.HText{Text: "yes"}}}
	b := &ir.Elem{Tag: "p", Children: []ir.Expr{&ir.HText{Text: "no"}}}
	if text(a) == text(b) || text(&ir.List{Elems: []ir.Expr{a}}) == text(&ir.List{Elems: []ir.Expr{b}}) {
		t.Fatal("lossy renderings compare equal")
	}
	if text(&ir.Lit{Kind: ir.LitInt, Int: 1}) != text(&ir.Lit{Kind: ir.LitInt, Int: 1}) {
		t.Fatal("equal literals compare unequal")
	}
}

// TestIsHtml: a result type containing Html anywhere is view code.
func TestIsHtml(t *testing.T) {
	html := &types.App{Con: "Html", Args: []types.Type{&types.Param{Name: "m"}}}
	for _, ty := range []types.Type{
		html,
		&types.Fun{Params: []types.Type{types.Int}, Result: &types.App{Con: "List", Args: []types.Type{html}}},
		&types.Record{Anonymous: true, Fields: []types.Field{{Name: "title", Type: types.Text}, {Name: "body", Type: html}}},
	} {
		if !isHtml(ty) {
			t.Errorf("isHtml(%s) = false", types.String(ty))
		}
	}
	if isHtml(&types.Fun{Params: []types.Type{html}, Result: types.Int}) {
		t.Error("a function taking Html and returning Int is not view code")
	}
}
