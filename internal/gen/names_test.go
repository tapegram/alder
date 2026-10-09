package gen

import (
	"strings"
	"testing"

	"alder/internal/ir"
	"alder/internal/types"
)

// identifiers lists every identifier of length 1 to n over alphabet whose first character is in
// first, obeying the lexer's rule (no "__") and, for module segments, the parser's (no trailing
// '_').
func identifiers(first, alphabet string, n int, segment bool) []string {
	var out []string
	var grow func(s string)
	grow = func(s string) {
		if strings.Contains(s, "__") || s == "_" {
			return
		}
		if !(segment && strings.HasSuffix(s, "_")) {
			out = append(out, s)
		}
		if len(s) < n {
			for _, c := range alphabet {
				grow(s + string(c))
			}
		}
	}
	for _, c := range first {
		grow(string(c))
	}
	return out
}

// TestGoNameInjective checks the naming scheme's injectivity proof (names.go) exhaustively on
// small identifiers, and on a corpus of names that collided under earlier schemes: no two
// different Alder entities get the same Go name, whatever their kind.
func TestGoNameInjective(t *testing.T) {
	segs := identifiers("AB", "AB_x", 3, true)
	var modules []string
	for _, a := range segs {
		modules = append(modules, a)
		for _, b := range segs[:12] {
			modules = append(modules, a+"."+b)
		}
	}
	uppers := identifiers("A", "A_x", 3, false)  // types, constructors, components
	lowers := identifiers("x_", "A_x", 3, false) // values (and locals)
	modules = append(modules, "A.B", "A_B", "M", "M.Shape", "Tag", "Tag.M", "Shape", "Std", "Home")
	uppers = append(uppers, "Shape", "Shape_Rect", "ShapeTag", "Tag", "Rect", "Rect_", "Circle", "M", "Session", "Msg", "Params", "PickParams")
	lowers = append(lowers, "type", "x", "_x", "x_", "rt", "html", "tag", "is", "payload", "widen", "view")

	seen := map[string]string{} // Go name → the entity that has it
	add := func(goName, entity string) {
		if prev, ok := seen[goName]; ok && prev != entity {
			t.Fatalf("%s and %s are both %s", prev, entity, goName)
		}
		seen[goName] = entity
	}
	for _, m := range modules {
		for _, n := range append(append([]string{}, uppers...), lowers...) {
			add(GoName(m, n), "decl "+m+"."+n)
		}
		add(widenName(m), "widen "+m)
		for _, n := range uppers {
			add(tableName(m, n), "table "+m+"."+n)
			add(insertName(m, n), "insert "+m+"."+n)
			add(insertVals(m, n), "insertvals "+m+"."+n)
			add(capName(m, n), "capability "+m+"."+n)
			add(enumName(m, n), "enum "+m+"."+n)
		}
	}
	add(tablesVar, "the Tables variable")
	ctors := uppers
	if len(ctors) > 20 {
		ctors = append(ctors[:10:10], "Rect", "Rect_", "Tag", "Circle", "A_x", "Ax_")
	}
	for _, m := range modules {
		for _, u := range uppers[:12] {
			add(tagType(m, u), "tag type "+m+"."+u)
			for _, c := range ctors {
				e := m + "." + u + "." + c
				add(tagConst(m, u, c), "tag "+e)
				add(payloadName(m, u, c), "payload "+e)
				add(ctorName(m, u, c), "ctor "+e)
			}
		}
	}
	// locals: never a package-level name, nor an imported package's name
	for _, n := range append(lowers, "type", "go", "rt", "std", "time", "math", "html", "len", "any", "eff", "db", "context", "fmt", "table", "cap", "name", "insert", "insertvals") {
		l := local(n)
		if _, ok := seen[l]; ok {
			t.Errorf("local %s is the package-level name of %s", l, seen[l])
		}
		if packages[l] != "" || reserved[l] {
			t.Errorf("local %s is reserved", l)
		}
	}
	// file names
	files := map[string]string{tablesFile: "the tables file"}
	for _, m := range modules {
		f := fileName(m)
		if prev, ok := files[f]; ok && prev != m {
			t.Fatalf("modules %s and %s are both emitted to %s", prev, m, f)
		}
		files[f] = m
	}
	if len(seen) < 100000 {
		t.Fatalf("only %d names generated", len(seen))
	}
}

// Emit refuses a program whose Go names or files would collide, rather than overwrite a file or
// emit a redeclaration (defense in depth: the scheme is injective, so this needs a hand-made IR).
func TestEmit_RejectsDuplicates(t *testing.T) {
	val := func(module, name string) *ir.Value {
		return &ir.Value{Module: module, Name: name, Type: types.Int, Body: &ir.Lit{Typed: ir.Typed{Ty: types.Int}, Kind: ir.LitInt, Int: 1}}
	}
	_, err := Emit(&ir.Program{Modules: []*ir.Module{{Name: "A", Values: []*ir.Value{val("A", "x")}}, {Name: "A", Values: []*ir.Value{val("A", "y")}}}}, Options{RepoPath: "/r"})
	if err == nil || !strings.Contains(err.Error(), "two modules are emitted to A_alder.go") {
		t.Errorf("duplicate file: err = %v", err)
	}
	_, err = Emit(&ir.Program{Modules: []*ir.Module{{Name: "A", Values: []*ir.Value{val("A", "x"), val("A", "x")}}}}, Options{RepoPath: "/r"})
	if err == nil || !strings.Contains(err.Error(), "A__x is declared twice") {
		t.Errorf("duplicate name: err = %v", err)
	}
}
