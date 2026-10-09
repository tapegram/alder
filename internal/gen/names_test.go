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
	// not Route, Cap or Args, nor under them: E0015 (names.ReservedModules)
	modules = append(modules, "A.B", "A_B", "M", "M.Shape", "Tag", "Tag.M", "Shape", "Std", "Home", "App", "Routes", "Caps", "Capx.Y", "Arg", "Page.Route")
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
		add(pageFunc(m), "page__ "+m)
		add(updateFunc(m), "update__ "+m)
		add(paramsFunc(m), "params__ "+m)
		add(routeFunc(m, ""), "the route of page "+m)
		for _, n := range uppers {
			add(tableName(m, n), "table "+m+"."+n)
			add(insertName(m, n), "insert "+m+"."+n)
			add(insertVals(m, n), "insertvals "+m+"."+n)
			add(capName(m, n), "capability "+m+"."+n)
			add(enumName(m, n), "enum "+m+"."+n)
			add(checkName(m, n), "brand check "+m+"."+n)
			add(decodeName(m, n), "decoder "+m+"."+n)
			add(routeFunc(m, n), "the route of message "+m+"."+n)
		}
		for _, n := range uppers[:12] {
			for _, e := range append(lowers[:20:20], "_x", "_X", "x_", "x", "x_x", "xx") {
				add(argsName(m, n, e), "args "+m+"."+n+"."+e)
			}
		}
	}
	for _, n := range []string{tablesVar, capsVar, "Caps", "Options", "Serve", "Routes", "route__fail", "route__values", "route__errors", "session__codec"} {
		add(n, "the fixed name "+n)
	}
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
	for _, n := range append(lowers, "type", "go", "rt", "std", "time", "math", "html", "len", "any", "eff", "db", "context", "fmt", "table", "cap", "name", "insert", "insertvals",
		"url", "web", "http", "json", "errors", "strings", "check", "decode", "page", "update", "route", "session", "params") {
		l := local(n)
		if _, ok := seen[l]; ok {
			t.Errorf("local %s is the package-level name of %s", l, seen[l])
		}
		if packages[l] != "" || reserved[l] {
			t.Errorf("local %s is reserved", l)
		}
	}
	// file names
	files := map[string]string{appFile: "the app file"}
	for _, m := range modules {
		f := fileName(m)
		if prev, ok := files[f]; ok && prev != m {
			t.Fatalf("modules %s and %s are both emitted to %s", prev, m, f)
		}
		files[f] = m
		// app__alder.go must differ from every module's file also on a case-insensitive file
		// system (macOS): App_alder.go and app_alder.go would be one file there
		if strings.EqualFold(f, appFile) {
			t.Fatalf("module %s's file %s is the app file %s on a case-insensitive file system", m, f, appFile)
		}
	}
	for _, m := range []string{"App", "APP", "App.App"} {
		if strings.EqualFold(fileName(m), appFile) {
			t.Fatalf("module %s's file is the app file", m)
		}
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
