package gen

import (
	"flag"
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"alder/internal/check"
	"alder/internal/diag"
	"alder/internal/ir"
	"alder/internal/names"
	"alder/internal/syntax"
	"alder/internal/types"
)

var update = flag.Bool("update", false, "rewrite golden files")

// lowerDir loads, resolves and checks dir (which holds app/*.alder), requires it to be free of
// errors, and lowers it.
func lowerDir(t *testing.T, dir string) *ir.Program {
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
		t.Fatalf("%s does not parse:\n%s", dir, errorsOf(bag))
	}
	prog.Resolve(bag)
	prog.MarkFailed(bag)
	res := check.Program(prog, bag)
	if bag.HasErrors() {
		t.Fatalf("%s does not check:\n%s", dir, errorsOf(bag))
	}
	return ir.Lower(prog, res)
}

func errorsOf(bag *diag.Bag) string {
	var sb strings.Builder
	for _, d := range bag.Items {
		if d.Severity == "error" {
			fmt.Fprintf(&sb, "%s:%d:%d %s %s\n", d.Span.File, d.Span.Start.Line, d.Span.Start.Col, d.Code, d.Message)
		}
	}
	return sb.String()
}

func repoPath(t *testing.T) string {
	t.Helper()
	p, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	return p
}

// emit lowers dir and emits it.
// An app with a migrations/ directory is emitted with Options.Migrations, and its *.sql files
// are added to the result as "migrations/<file>" (what buildkit.CopyMigrations copies), so the
// emitted module builds.
func emit(t *testing.T, dir string) map[string][]byte {
	t.Helper()
	sqls, _ := filepath.Glob(filepath.Join(dir, "migrations", "*.sql"))
	files, err := Emit(lowerDir(t, dir), Options{RepoPath: repoPath(t), Migrations: len(sqls) > 0})
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range sqls {
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		files["migrations/"+filepath.Base(f)] = b
	}
	return files
}

// emitSource writes the given Alder modules (file name → source) into a temporary app and
// emits it.
func emitSource(t *testing.T, srcs map[string]string) map[string][]byte {
	t.Helper()
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "app"), 0o755); err != nil {
		t.Fatal(err)
	}
	for name, src := range srcs {
		if err := os.MkdirAll(filepath.Dir(filepath.Join(dir, "app", name)), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "app", name), []byte(src), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return emit(t, dir)
}

// vet writes the emitted files into a temporary module and runs `go vet` on it.
func vet(t *testing.T, files map[string][]byte) {
	t.Helper()
	if _, ok := files["go.mod"]; !ok {
		t.Fatal("Emit produced no go.mod")
	}
	dir := t.TempDir()
	for name, b := range files {
		if err := os.MkdirAll(filepath.Dir(filepath.Join(dir, name)), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, name), b, 0o644); err != nil {
			t.Fatal(err)
		}
		if strings.HasSuffix(name, ".go") {
			if f, err := format.Source(b); err != nil || string(f) != string(b) {
				t.Errorf("%s is not gofmt-formatted (%v)", name, err)
			}
		}
	}
	cmd := exec.Command("go", "vet", "-mod=mod", "./...")
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GOWORK=off", "GOFLAGS=-mod=mod", "GOPROXY=off", "GOTOOLCHAIN=local")
	if out, err := cmd.CombinedOutput(); err != nil {
		var src strings.Builder
		for _, name := range mapsKeys(files) {
			fmt.Fprintf(&src, "==== %s\n%s\n", name, files[name])
		}
		t.Fatalf("go vet failed: %v\n%s\n%s", err, out, src.String())
	}
}

func mapsKeys(m map[string][]byte) []string {
	var ks []string
	for k := range m {
		ks = append(ks, k)
	}
	slices.Sort(ks)
	return ks
}

func TestGenGolden(t *testing.T) {
	dirs, _ := filepath.Glob("../../testdata/golden/gen_*")
	n := 0
	for _, dir := range dirs {
		if st, err := os.Stat(filepath.Join(dir, "app")); err != nil || !st.IsDir() {
			continue // not an app (gen_vet holds a corpus of apps: TestGenVetCorpus)
		}
		n++
		t.Run(filepath.Base(dir), func(t *testing.T) {
			files := emit(t, dir)
			want := filepath.Join(dir, "want")
			if *update {
				os.RemoveAll(want)
				if err := os.MkdirAll(want, 0o755); err != nil {
					t.Fatal(err)
				}
				for name, b := range files {
					if strings.HasSuffix(name, ".go") {
						if err := os.WriteFile(filepath.Join(want, name), b, 0o644); err != nil {
							t.Fatal(err)
						}
					}
				}
			}
			wantFiles, _ := filepath.Glob(filepath.Join(want, "*.go"))
			var wantNames, gotNames []string
			for _, w := range wantFiles {
				wantNames = append(wantNames, filepath.Base(w))
			}
			for name := range files {
				if strings.HasSuffix(name, ".go") {
					gotNames = append(gotNames, name)
				}
			}
			slices.Sort(gotNames)
			if !slices.Equal(wantNames, gotNames) {
				t.Fatalf("emitted files %v, want %v", gotNames, wantNames)
			}
			for _, name := range wantNames {
				w, err := os.ReadFile(filepath.Join(want, name))
				if err != nil {
					t.Fatal(err)
				}
				if got := string(files[name]); got != string(w) {
					t.Errorf("golden mismatch for %s\n got:\n%s\nwant:\n%s", name, got, w)
				}
			}
			vet(t, files)
		})
	}
	if n == 0 {
		t.Fatal("no gen golden directories")
	}
}

func TestGen_GoMod(t *testing.T) {
	files, err := Emit(&ir.Program{}, Options{RepoPath: "/r/alder"})
	if err != nil {
		t.Fatal(err)
	}
	want := "module app\n\ngo 1.27\n\nrequire alder v0.0.0\n\nreplace alder => \"/r/alder\"\n"
	if got := string(files["go.mod"]); got != want {
		t.Fatalf("go.mod:\n%s\nwant:\n%s", got, want)
	}
}

// The replace path is quoted (M2): a repository path with a space still builds.
func TestGen_GoModPathWithSpace(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "my repo")
	// the runtime is all the generated code needs of the repository
	if err := os.CopyFS(filepath.Join(dir, "rt"), os.DirFS(filepath.Join(repoPath(t), "rt"))); err != nil {
		t.Fatal(err)
	}
	gomod, err := os.ReadFile(filepath.Join(repoPath(t), "go.mod"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), gomod, 0o644); err != nil {
		t.Fatal(err)
	}
	files, err := Emit(lowerDir(t, "../../testdata/golden/gen_basic"), Options{RepoPath: dir})
	if err != nil {
		t.Fatal(err)
	}
	vet(t, files)
}

func contains(t *testing.T, src []byte, wants ...string) {
	t.Helper()
	for _, w := range wants {
		if !strings.Contains(string(src), w) {
			t.Errorf("emitted Go lacks %q:\n%s", w, src)
		}
	}
}

// A constructor's anonymous record payload gets its own named payload struct, and a non-record
// payload is wrapped as { Value T }; generic unions carry their parameters into both.
func TestGen_UnionAnonymousPayload(t *testing.T) {
	files := emitSource(t, map[string]string{"U.alder": `module U

type Shape = Circle Float | Rect { w: Float, h: Float } | Empty
type Tree a = Leaf | Node { value: a, size: Int }

rect : Shape
rect = Rect { w: 1.0, h: 2.0 }

wide : Shape -> Float
wide s = match s {
  Rect r -> r.w,
  Circle d -> d,
  Empty -> 0.0,
}

leaf : Tree Text
leaf = Leaf

sizeOf : Tree a -> Int
sizeOf t = match t {
  Node n -> n.size,
  Leaf -> 0,
}

node : Text -> Tree Text
node v = Node { value: v, size: 1 }
`})
	src := files["U_alder.go"]
	contains(t, src,
		"type payload__U__Shape__Rect struct {\n\tW float64\n\tH float64\n}",
		"type payload__U__Shape__Circle struct {\n\tValue float64\n}",
		"Rect   *payload__U__Shape__Rect",
		"func U__Shape__Circle__(v float64) U__Shape {",
		"func U__Shape__Rect__(v payload__U__Shape__Rect) U__Shape {",
		"var U__Shape__Empty__ = U__Shape{Tag: is__U__Shape__Empty}",
		"U__Shape__Rect__(payload__U__Shape__Rect{W: 1.0, H: 2.0})",
		"type U__Tree[A any] struct {",
		"type payload__U__Tree__Node[A any] struct {\n\tValue A\n\tSize  int64\n}",
		"func U__Tree__Node__[A any](v payload__U__Tree__Node[A]) U__Tree[A] {",
		"U__Tree[string]{Tag: is__U__Tree__Leaf}",
		"func U__sizeOf[A any](t U__Tree[A]) int64 {",
		"U__Tree__Node__[string](payload__U__Tree__Node[string]{Value: v, Size: 1})",
	)
	vet(t, files)
}

// Alder names that are Go keywords or clash with the generated shape are renamed.
func TestGen_KeywordNames(t *testing.T) {
	files := emitSource(t, map[string]string{"K.alder": `module K

type Kind = Tag Int | Func { type: Text }

func : Kind -> Text
func k = match k {
  Tag n -> Text.fromInt n,
  Func { type: type } -> type,
}

go : Int -> Int
go range = {
  let var = range + 1
  let rt = var * 2
  rt
}
`})
	src := files["K_alder.go"]
	contains(t, src,
		"Tag__ *payload__K__Kind__Tag",
		"Type string",
		"func K__func(k K__Kind) string {",
		"case is__K__Kind__Tag:",
		"type__ := k.Func.Type",
		"return type__",
		"func K__go(range__ int64) int64 {",
		"var__ := range__ + 1",
		"rt__ := var__ * 2",
	)
	vet(t, files)
}

func TestGen_StringEscapes(t *testing.T) {
	files := emitSource(t, map[string]string{"S.alder": `module S

s : Text
s = "a \{b} \"c\""

t : Text -> Text
t x = "<{x}> \\ é"
`})
	contains(t, files["S_alder.go"],
		`var S__s string = "a {b} \"c\""`,
		`rt.Concat("<", x, "> \\ é")`,
	)
	vet(t, files)
}

// A module of only types emits a file with no imports that still compiles.
func TestGen_TypesOnlyModule(t *testing.T) {
	files := emitSource(t, map[string]string{"T.alder": `module T

type Color = Red | Green
type Pair = { left: Int, right: Text }
`})
	src := string(files["T_alder.go"])
	if strings.Contains(src, "import") {
		t.Errorf("types-only module imports something:\n%s", src)
	}
	if !strings.HasPrefix(src, "// Code generated by alder; DO NOT EDIT.\n\npackage app\n") {
		t.Errorf("bad header:\n%s", src)
	}
	contains(t, files["T_alder.go"], "type T__Pair struct {", "var T__Color__Red__ = T__Color{Tag: is__T__Color__Red}")
	vet(t, files)
}

func TestGoType(t *testing.T) {
	maybe := func(a types.Type) types.Type {
		return &types.Union{Module: "Prelude", Name: "Maybe", Args: []types.Type{a}}
	}
	list := func(a types.Type) types.Type { return &types.App{Con: "List", Args: []types.Type{a}} }
	for _, tc := range []struct {
		t    types.Type
		want string
	}{
		{types.Int, "int64"},
		{types.Float, "float64"},
		{types.Bool, "bool"},
		{types.Text, "string"},
		{types.Unit, "struct{}"},
		{types.Date, "rt.Date"},
		{types.Time, "time.Time"},
		{types.Page, "rt.Page"},
		{list(maybe(types.Int)), "[]rt.Maybe[int64]"},
		{&types.Union{Module: "Prelude", Name: "Result", Args: []types.Type{types.Text, types.Int}}, "rt.Result[string, int64]"},
		{&types.App{Con: "Dict", Args: []types.Type{types.Text, list(types.Float)}}, "rt.Dict[string, []float64]"},
		{&types.App{Con: "Html", Args: []types.Type{&types.Param{Name: "a"}}}, "html.Node"},
		{&types.Record{Anonymous: true, Fields: []types.Field{{Name: "w", Type: types.Float}, {Name: "h", Type: types.Float}}}, "struct{ W float64; H float64 }"},
		{&types.Record{Anonymous: true}, "struct{}"},
		{&types.Record{Module: "Todos.Index", Name: "Model"}, "Todos__Index__Model"},
		{&types.Union{Module: "P", Name: "Tree", Args: []types.Type{types.Text}}, "P__Tree[string]"},
		{&types.Union{Module: "Theme", Name: "Theme"}, "Theme__Theme"},
		{&types.Param{Name: "a"}, "A"},
		{&types.Fun{Params: []types.Type{types.Int, types.Text}, Result: types.Bool}, "func(int64, string) bool"},
	} {
		if got := GoType(tc.t); got != tc.want {
			t.Errorf("GoType(%s) = %q, want %q", types.String(tc.t), got, tc.want)
		}
	}
}

func TestGoName(t *testing.T) {
	for _, tc := range [][3]string{
		{"Todos.Index", "view", "Todos__Index__view"},
		{"M", "area", "M__area"},
		{"M", "type", "M__type"},
	} {
		if got := GoName(tc[0], tc[1]); got != tc[2] {
			t.Errorf("GoName(%q, %q) = %q, want %q", tc[0], tc[1], got, tc[2])
		}
	}
}

// The IR goldens and the todos example exercise every match kind, partial application, holes,
// pages and (stubbed) HTML: their emitted Go must vet.
func TestGen_VetIRGoldens(t *testing.T) {
	for _, dir := range []string{"ir_patterns", "ir_page", "../../examples/todos"} {
		if !strings.Contains(dir, "/") {
			dir = "../../testdata/golden/" + dir
		}
		t.Run(filepath.Base(dir), func(t *testing.T) { vet(t, emit(t, dir)) })
	}
}

// Expression forms beyond the goldens: generics, thunks, closures, updates of computed records,
// equality kinds, operators, nested ifs and shadowing lets.
func TestGen_Expressions(t *testing.T) {
	files := emitSource(t, map[string]string{"E.alder": `module E

type Color = Red | Green
type Box = { w: Int, label: Text }

identity : a -> a
identity x = x

none : Maybe a
none = Nothing

empty : Dict Text Int
empty = Dict.empty

ticks : List Int
ticks = List.map (\n -> n + base) [1, 2]

base : Int
base = identity 1

same : Color -> Color -> Bool
same a b = a == b

sameBox : Box -> Box -> Bool
sameBox a b = a != b

grow : Int -> Box
grow n = { (mk n) | w: n * 2 }

mk : Int -> Box
mk n = { w: n, label: "x" }

rem : Float -> Float -> Float
rem a b = a % b

joinAll : List Int -> List Int -> List Int
joinAll a b = a ++ b ++ [3]

sign : Int -> Text
sign n = if n < 0 then "neg" else if n == 0 then "zero" else "pos"

labels : List Int -> List Text
labels xs = List.map (\n -> if n > 0 then "p" else "n") xs

again : Int -> Int
again n = {
  let m = n + 1
  let k = 2
  m * k
}

check : Result Text Int -> Int
check r = match r {
  Ok n -> n,
  Err e -> Text.length e,
}

twice : (Int -> Int) -> Int -> Int
twice f x = f (f x)

addTo : Int -> Int -> Int
addTo = \a b -> a + b

inc : Int -> Int
inc = addTo 1

wrapped : Maybe Int -> Maybe Int
wrapped m = Maybe.map (\n -> n * 2) m

total : Int
total = twice inc 3 + List.length ticks

loopy : Int
loopy = bump 1

bump : Int -> Int
bump n = if n > 100 then n else n + loopy

keysOf : Dict k v -> List k
keysOf d = Dict.keys d

shadow : Int -> Int
shadow n = {
  let n = n + 1
  n * 2
}
`})
	contains(t, files["E_alder.go"],
		"func E__identity[A any](x A) A {",
		"func E__none[A any]() rt.Maybe[A] {",
		"var E__empty rt.Dict[string, int64] = std.DictEmpty[string, int64]()",
		"var E__base int64 = E__identity[int64](1)",
		"return a.Tag == b.Tag",
		"return !rt.Eq(a, b)",
		"math.Mod(a, b)",
		"rt.Append(rt.Append(a, b), []int64{3})",
		"func E__addTo(a int64, b int64) int64 {",
		"func E__inc(_a1 int64) int64 {\n\treturn E__addTo(1, _a1)",
		"k := int64(2)",
		"func E__loopy() int64 {",
		"return n + E__loopy()",
		"func E__keysOf[K any, V any](d rt.Dict[K, V]) []K {",
		"n_2 := n + 1",
		"return n_2 * 2",
	)
	vet(t, files)
}

// Ordered comparisons on Date, Time and payload-less unions compile, with every operator.
func TestGen_OrderedComparisons(t *testing.T) {
	files := emitSource(t, map[string]string{"O.alder": `module O

type Prio = Low | High

dates : Date -> Date -> List Bool
dates a b = [a < b, a <= b, a > b, a >= b]

times : Time -> Time -> List Bool
times a b = [a < b, a <= b, a > b, a >= b]

prios : Prio -> Prio -> List Bool
prios a b = [a < b, a <= b, a > b, a >= b]
`})
	contains(t, files["O_alder.go"],
		"[]bool{rt.DateCmp(a, b) < 0, rt.DateCmp(a, b) <= 0, rt.DateCmp(a, b) > 0, rt.DateCmp(a, b) >= 0}",
		"[]bool{rt.TimeCmp(a, b) < 0, rt.TimeCmp(a, b) <= 0, rt.TimeCmp(a, b) > 0, rt.TimeCmp(a, b) >= 0}",
		"[]bool{a.Tag < b.Tag, a.Tag <= b.Tag, a.Tag > b.Tag, a.Tag >= b.Tag}",
	)
	vet(t, files)
}

// A union's tag type cannot collide with a user type named after it.
func TestGen_TagTypeNoCollision(t *testing.T) {
	files := emitSource(t, map[string]string{"N.alder": `module N

type Shape = Dot | Tag
type ShapeTag = { label: Text }

isDot : Shape -> ShapeTag -> Bool
isDot s t = match s {
  Dot -> True,
  Tag -> t.label == "tag",
}
`})
	contains(t, files["N_alder.go"], "type tag__N__Shape uint8", "type N__ShapeTag struct {", "is__N__Shape__Tag tag__N__Shape = 1")
	vet(t, files)
}

// Go names the emitter derives (renamings, temporaries, keyword suffixes) are also names an
// Alder program may use: no emitted declaration may capture one of them.
func TestGen_DerivedNamesDoNotCapture(t *testing.T) {
	files := emitSource(t, map[string]string{"C.alder": `module C

type Box = { w: Int }

add : Int -> Int -> Int
add a b = a + b

partial : Int -> List Int -> List Int
partial _a1 xs = List.map (add _a1) xs

renamed : Int -> Int -> Int
renamed n n_2 = {
  let n = n + 1
  n + n_2
}

keyword : Int -> Int -> Int
keyword type_ x = {
  let type = x * 2
  type + type_
}

upd : Int -> Box -> Box
upd _u1 b = { (grow b) | w: _u1 }

grow : Box -> Box
grow b = b

adder : Int -> Int -> Int
adder _a1 = add _a1
`})
	contains(t, files["C_alder.go"],
		"func(_a1_2 int64) int64 {\n\t\treturn C__add(_a1, _a1_2)", // the closure's parameter is not the user's _a1
		"_u1_2 := C__grow(b)\n\t\t_u1_2.W = _u1",
		"func C__adder(_a1 int64, _a1_2 int64) int64 {",
		"n_3 := n + 1",
		"return n_3 + n_2",
		"type__ := x * 2",
		"return type__ + type_",
	)
	vet(t, files)
}

// HTML text is emitted raw: rt/html escapes it exactly once, when rendering.
func TestGen_HTMLTextNotPreEscaped(t *testing.T) {
	files := emitSource(t, map[string]string{"H.alder": `module H

note : Html a
note = <p title="x & y">{"a < b"} & é</p>
`})
	src := string(files["H_alder.go"])
	contains(t, files["H_alder.go"], `html.Text("a < b")`, `html.Text(" & é")`, `html.A("title", "x & y")`)
	for _, bad := range []string{"&lt;", "&amp;"} {
		if strings.Contains(src, bad) {
			t.Errorf("emitted Go pre-escapes text (%s):\n%s", bad, src)
		}
	}
	vet(t, files)
}

// Every stdlib signature has an entry in StdCalls or (Query.*, Task.*, Db.*) in Effectful.
func TestStdCallsCoverStdlib(t *testing.T) {
	std, err := types.LoadStd()
	if err != nil {
		t.Fatal(err)
	}
	n := 0
	for _, m := range std {
		for _, d := range m.Decls {
			v, ok := d.(*syntax.ValueDecl)
			if !ok {
				continue
			}
			n++
			key := m.Name + "." + v.Name
			_, call := StdCalls[key]
			eff := isEffectful(m.Name, v.Name)
			switch {
			case call && eff:
				t.Errorf("%s is both in StdCalls and Effectful", key)
			case !call && !eff:
				t.Errorf("pure stdlib function %s has no StdCalls entry", key)
			}
		}
	}
	if n < 100 {
		t.Fatalf("only %d stdlib signatures found", n)
	}
	// every entry names a real stdlib function
	for key := range StdCalls {
		mod, name, _ := strings.Cut(key, ".")
		found := false
		if m := std[mod]; m != nil {
			for _, d := range m.Decls {
				if v, ok := d.(*syntax.ValueDecl); ok && v.Name == name {
					found = true
				}
			}
		}
		if !found {
			t.Errorf("StdCalls entry %s is not a stdlib function", key)
		}
	}
}

// Each StdCalls target is a function of rt/std.
func TestStdCallsExist(t *testing.T) {
	fset := token.NewFileSet()
	pkgs, err := filepath.Glob("../../rt/std/*.go")
	if err != nil {
		t.Fatal(err)
	}
	funcs := map[string]bool{}
	for _, file := range pkgs {
		if strings.HasSuffix(file, "_test.go") {
			continue
		}
		f, err := parser.ParseFile(fset, file, nil, parser.SkipObjectResolution)
		if err != nil {
			t.Fatal(err)
		}
		for _, d := range f.Decls {
			if fd, ok := d.(*ast.FuncDecl); ok && fd.Recv == nil {
				funcs["std."+fd.Name.Name] = true
			}
		}
	}
	for key, target := range StdCalls {
		if !funcs[target] {
			t.Errorf("StdCalls[%q] = %s, which rt/std does not define", key, target)
		}
	}
}

// The Go type parameters of each generic StdCalls target, read from rt/std's source, are the
// Alder type variables stdTypeParams lists, in the same order: the Go names are the variables
// upper-cased (A ↔ a), and Session.put's S is the app's Session (C3).
func TestStdTypeParamOrder(t *testing.T) {
	fset := token.NewFileSet()
	files, err := filepath.Glob("../../rt/std/*.go")
	if err != nil {
		t.Fatal(err)
	}
	tparams := map[string][]string{}
	for _, file := range files {
		if strings.HasSuffix(file, "_test.go") {
			continue
		}
		f, err := parser.ParseFile(fset, file, nil, parser.SkipObjectResolution)
		if err != nil {
			t.Fatal(err)
		}
		for _, d := range f.Decls {
			fd, ok := d.(*ast.FuncDecl)
			if !ok || fd.Recv != nil || fd.Type.TypeParams == nil {
				continue
			}
			var names []string
			for _, fl := range fd.Type.TypeParams.List {
				for _, n := range fl.Names {
					names = append(names, n.Name)
				}
			}
			tparams["std."+fd.Name.Name] = names
		}
	}
	for key, target := range StdCalls {
		mod, name, _ := strings.Cut(key, ".")
		var want []string
		for _, n := range tparams[target] {
			if key == "Session.put" && n == "S" {
				want = append(want, "Session")
			} else {
				want = append(want, strings.ToLower(n))
			}
		}
		if got := stdTypeParams(mod, name); !slices.Equal(got, want) {
			t.Errorf("%s (%s): stdTypeParams = %v, Go type parameters %v", key, target, got, tparams[target])
		}
	}
}

// Generic stdlib functions used as values run (C3): see gen_vet/p1-std-values.
func TestGen_RunStdValues(t *testing.T) {
	files := emit(t, "../../testdata/golden/gen_vet/p1-std-values")
	contains(t, files["M_alder.go"], "std.ListReverse[int64]", "std.ListMap[string, int64]", "std.SessionPut[App__Session]",
		"std.ResultAndThen[string, int64, int64]", "std.ResultMap[string, int64, bool]", "std.ResultWithDefault[string, bool]",
		"std.ListReverse[any]")
	runGo(t, files, `package app

import (
	"slices"
	"testing"

	"alder/rt"
	"alder/rt/eff"
)

func TestStdValues(t *testing.T) {
	if got := M__revd([]int64{1, 2, 3}); !slices.Equal(got, []int64{3, 2, 1}) {
		t.Errorf("revd = %v", got)
	}
	if got := M__lengths([]string{"a", "bcd"}); !slices.Equal(got, []int64{1, 3}) {
		t.Errorf("lengths = %v", got)
	}
	if !M__chain(rt.Ok[string](int64(5))) || M__chain(rt.Ok[string](int64(-1))) || M__chain(rt.Err[string, int64]("x")) {
		t.Error("chain")
	}
	if M__firstOr(rt.Nothing[string]()) != "none" || M__firstOr(rt.Just("a")) != "a" {
		t.Error("firstOr")
	}
	if got := M__counts(rt.Dict[string, int64]{}.Insert("a", 1)); !slices.Equal(got, []int64{1}) {
		t.Errorf("counts = %v", got)
	}
	if M__unsolved != 1 {
		t.Error("unsolved")
	}
	if fs := M__applyAll; len(fs) != 2 || !slices.Equal(fs[0]([]int64{1, 2}), []int64{2, 1}) {
		t.Error("applyAll")
	}
	env := &eff.Env{Session: App__Session{N: 1}}
	if _, err := eff.Run(env, eff.KTask, M__bump); err != nil || env.Session.(App__Session).N != 2 {
		t.Errorf("bump: %v %+v", err, env.Session)
	}
}
`)
}

// A message that is not a constructor written in place is switched on at run time for its URL;
// a generic component takes its props at the instance's types; a Let around HTML is used.
func TestGen_HTMLDynamic(t *testing.T) {
	files := emitSource(t, map[string]string{
		"App.alder": `module App

session { }

error : Error -> Html Never
error err = <p>error</p>

notFound : Html Never
notFound = <p>not found</p>
`,
		"P.alder": `module P

page "/p" { }

type Model = { names: List Text }

msg Pick { name: Text, at: Maybe Int }
msg Clear

load : Params -> Session -> Query Model
load params session = Query.succeed { names: [] }

update : Msg -> Model -> Session -> Task Nav
update msg model session = Task.succeed Rerender

Btn : { msg: Msg, label: Text } -> Html Msg
Btn props = <button click={props.msg}>{props.label}</button>

Each : { items: List a, render: a -> Html m } -> Html m
Each props = <ul>{List.map props.render props.items}</ul>

view : Model -> Session -> Html Msg
view model session = {
  let first = List.head model.names ?? "none"
  <div>
    <Btn msg={Clear} label={first} />
    <Each items={model.names} render={\n -> <li>{n}</li>} />
  </div>
}
`})
	contains(t, files["P_alder.go"],
		"switch _m1.Tag {",
		`return rt.MsgRef{Module: "P", Msg: "Pick", Params: []rt.KV{{Key: "name", Value: _m1.Pick.Name}, {Key: "at", Value: rt.MaybeText(_m1.Pick.At, std.TextFromInt)}}}`,
		`return rt.MsgRef{Module: "P", Msg: "Clear"}`,
		"P__Btn(struct {",
		"P__Each(struct {",
		"Items  []string",
		"first := rt.OrElse(",
	)
	vet(t, files)
}

// A type argument the checker leaves unsolved is unconstrained: it is instantiated as any (C7).
func TestGen_UnsolvedTypeArgs(t *testing.T) {
	files := emitSource(t, map[string]string{"M.alder": `module M

empty : List a
empty = []

nada : a -> Maybe b
nada x = Nothing

n : Int
n = List.length empty

m : Bool
m = Maybe.isJust (nada 1)
`})
	contains(t, files["M_alder.go"],
		"var M__n int64 = std.ListLength(M__empty[any]())",
		"var M__m bool = std.MaybeIsJust(M__nada[int64, any](1))",
	)
	vet(t, files)
}

// runGo writes the emitted files and a Go test file (package app) into a temporary module and
// runs `go test` there: the generated code must not only vet but compute the right values.
func runGo(t *testing.T, files map[string][]byte, test string) {
	t.Helper()
	dir := t.TempDir()
	for name, b := range files {
		if err := os.WriteFile(filepath.Join(dir, name), b, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(dir, "run_test.go"), []byte(test), 0o644); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("go", "test", "-count=1", "./...")
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GOWORK=off", "GOFLAGS=-mod=mod", "GOPROXY=off", "GOTOOLCHAIN=local")
	if out, err := cmd.CombinedOutput(); err != nil {
		var src strings.Builder
		for _, name := range mapsKeys(files) {
			fmt.Fprintf(&src, "==== %s\n%s\n", name, files[name])
		}
		t.Fatalf("go test of the generated code failed: %v\n%s\n%s", err, out, src.String())
	}
}

// Arithmetic on constants is evaluated at run time with Alder's semantics (C8): Int wraps on
// overflow, `/` truncates toward zero, `%` takes the dividend's sign, Int division by zero
// panics, Float division by zero is infinite.
func TestGen_RunArithmetic(t *testing.T) {
	files := emitSource(t, map[string]string{"M.alder": `module M

big : Int
big = 9223372036854775807 + 1

prod : Int
prod = 4611686018427387904 * 2 + 0

quot : Int
quot = -7 / 2

rem : Int
rem = -7 % 2

nested : Int
nested = 1 + 2 * 3

crash : Int -> Int
crash n = n + 1 / 0

crashMod : Int -> Int
crashMod n = n % 0

inf : Float
inf = 1.0 / 0.0
`})
	contains(t, files["M_alder.go"],
		"var M__big int64 = rt.Int(9223372036854775807) + 1",
		"return n + rt.Int(1)/rt.Int(0)",
		"return n % rt.Int(0)",
		"var M__inf float64 = rt.Float(1.0) / rt.Float(0.0)",
	)
	runGo(t, files, `package app

import (
	"math"
	"testing"
)

func panics(f func()) (p bool) {
	defer func() { p = recover() != nil }()
	f()
	return false
}

func TestArithmetic(t *testing.T) {
	if M__big != math.MinInt64 {
		t.Errorf("big = %d", M__big)
	}
	if M__prod != math.MinInt64 {
		t.Errorf("prod = %d", M__prod)
	}
	if M__quot != -3 || M__rem != -1 || M__nested != 7 {
		t.Errorf("quot, rem, nested = %d, %d, %d", M__quot, M__rem, M__nested)
	}
	if !math.IsInf(M__inf, 1) {
		t.Errorf("inf = %v", M__inf)
	}
	if !panics(func() { M__crash(1) }) || !panics(func() { M__crashMod(1) }) {
		t.Error("Int division by zero did not panic")
	}
}
`)
}

// Dict keys of any type compile and work (C2): a List key, a type parameter used as a key only
// inside a body, and a parameter reached through a generic union with a Dict payload.
func TestGen_RunDictKeys(t *testing.T) {
	files := emitSource(t, map[string]string{"D.alder": `module D

type Index k = Index (Dict k Int) | NoIndex

byPath : Dict (List Int) Text
byPath = Dict.fromList [{ key: [1, 2], value: "a" }, { key: [], value: "root" }]

find : List Int -> Text
find p = Dict.get p byPath ?? "none"

single : k -> Int -> Dict k Int
single k n = Dict.insert k n Dict.empty

count : Index k -> Int
count i = match i {
  Index d -> Dict.size d,
  NoIndex -> 0,
}

two : Int
two = count (Index (single [1] 1)) + Dict.size (single { a: 1 } 2)
`})
	src := string(files["D_alder.go"])
	if strings.Contains(src, "comparable") {
		t.Errorf("generated code constrains a type parameter:\n%s", src)
	}
	contains(t, files["D_alder.go"], "var D__byPath rt.Dict[[]int64, string] =", "func D__single[K any](k K, n int64) rt.Dict[K, int64] {")
	runGo(t, files, `package app

import "testing"

func TestDictKeys(t *testing.T) {
	if got := D__find([]int64{1, 2}); got != "a" {
		t.Errorf("find [1, 2] = %q", got)
	}
	if got := D__find(nil); got != "root" {
		t.Errorf("find [] = %q", got)
	}
	if got := D__find([]int64{2}); got != "none" {
		t.Errorf("find [2] = %q", got)
	}
	if D__two != 2 {
		t.Errorf("two = %d", D__two)
	}
}
`)
}

// A union may be recursive: every constructor payload is held by pointer, so the Go struct is
// finite (C1). Matching, equality and the whole-payload binding read through the pointer.
func TestGen_RunRecursiveUnion(t *testing.T) {
	files := emitSource(t, map[string]string{"T.alder": `module T

type Tree a = Leaf | Node { left: Tree a, value: a, right: Tree a }

type Expr = Num Int | Add { l: Expr, r: Expr } | Neg Expr

insert : Int -> Tree Int -> Tree Int
insert x t = match t {
  Leaf -> Node { left: Leaf, value: x, right: Leaf },
  Node n -> if x < n.value then Node { n | left: insert x n.left } else Node { n | right: insert x n.right },
}

toList : Tree a -> List a
toList t = match t {
  Leaf -> [],
  Node { left: l, value: v, right: r } -> toList l ++ [v] ++ toList r,
}

eval : Expr -> Int
eval e = match e {
  Num n -> n,
  Add { l: l, r: r } -> eval l + eval r,
  Neg x -> 0 - eval x,
}

tree : Tree Int
tree = List.foldl insert Leaf [3, 1, 2]

sorted : List Int
sorted = toList tree

same : Bool
same = tree == List.foldl insert Leaf [3, 1, 2]

differ : Bool
differ = tree == List.foldl insert Leaf [1, 2, 3]

value : Int
value = eval (Add { l: Num 2, r: Neg (Num 5) })

type Dir = { name: Text, entries: List Dir, link: Link }
type Link = None | To Dir

depth : Dir -> Int
depth d = match d.link {
  None -> 1,
  To e -> 1 + depth e,
}

deep : Int
deep = depth { name: "a", entries: [], link: To { name: "b", entries: [], link: None } }
`})
	contains(t, files["T_alder.go"], "Node *payload__T__Tree__Node[A]", "Add *payload__T__Expr__Add")
	runGo(t, files, `package app

import (
	"slices"
	"testing"
)

func TestTree(t *testing.T) {
	if !slices.Equal(T__sorted, []int64{1, 2, 3}) {
		t.Errorf("sorted = %v", T__sorted)
	}
	if !T__same || T__differ {
		t.Errorf("same, differ = %v, %v", T__same, T__differ)
	}
	if T__value != -3 || T__deep != 2 {
		t.Errorf("value, deep = %d, %d", T__value, T__deep)
	}
}
`)
}

// Generic records (C6): declared with Go type parameters, instantiated at each use; the
// gen_generic golden also runs.
func TestGen_RunGenericRecords(t *testing.T) {
	files := emit(t, "../../testdata/golden/gen_generic")
	runGo(t, files, `package app

import "testing"

func TestGeneric(t *testing.T) {
	if B__n != 3 {
		t.Errorf("n = %d", B__n)
	}
	if B__s != "xttwo" {
		t.Errorf("s = %q", B__s)
	}
	if B__same { // labels "i" and "made" differ
		t.Error("same is true")
	}
	if B__byMatch(B__Box[int64]{Item: 0}) != 0 || B__byMatch(B__Box[int64]{Item: 5}) != 5 {
		t.Error("byMatch")
	}
	if got := B__relabel(B__mk("v")); got.Label != "re" || got.Item != "v" {
		t.Errorf("relabel = %+v", got)
	}
}
`)
}

// An omitted field of a signature's anonymous record takes its default (C5).
func TestGen_RunSignatureDefaults(t *testing.T) {
	files := emit(t, "../../testdata/golden/gen_defaults")
	runGo(t, files, `package app

import (
	"testing"

	"alder/rt/html"
)

func TestDefaults(t *testing.T) {
	if Home__quiet != "ann" || Home__loud != "BOB" {
		t.Errorf("quiet, loud = %q, %q", Home__quiet, Home__loud)
	}
	want := "<div><button class=\"plain\" title=\"3\">save</button><button class=\"primary\" title=\"9\">go</button></div>"
	if got := html.Render(Home__bar); got != want {
		t.Errorf("bar = %s, want %s", got, want)
	}
}
`)
}

// A `requires` page's narrowed Session widens to App's where shared code expects it (C4): the
// required field is wrapped back in Just.
func TestGen_RunSessionWiden(t *testing.T) {
	files := emitSource(t, map[string]string{
		"App.alder": `module App

session { user: Maybe Text, n: Int = 0 }

login = Login

error : Error -> Html Never
error err = <p>error</p>

notFound : Html Never
notFound = <p>nf</p>
`,
		"Login.alder": `module Login

import Ui

page "/login" { }

type Model = { }

load : Params -> Session -> Query Model
load params session = Query.succeed { }

view : Model -> Session -> Html Msg
view model session = <p>{Ui.who session}</p>
`,
		"Ui.alder": `module Ui

who : Session -> Text
who s = match s.user {
  Just u -> "user " ++ u,
  Nothing -> "anon",
}

count : Session -> Int
count s = s.n
`,
		"Home.alder": `module Home

import Ui

page "/home" { } requires user

type Model = { }

load : Params -> Session -> Query Model
load params session = Query.succeed { }

name : Session -> Text
name session = session.user

other : Session -> Text
other session = Ui.who session

piped : Session -> Text
piped session = session |> Ui.who

both : Session -> Text
both session = "{Ui.who (if True then session else session)}{Text.fromInt (Ui.count { session | n: 2 })}"

view : Model -> Session -> Html Msg
view model session = <p>{name session}{other session}</p>
`,
	})
	contains(t, files["Home_alder.go"],
		"func widen__Home__Session(s Home__Session) App__Session {\n\treturn App__Session{User: rt.Just(s.User), N: s.N}\n}",
		"return Ui__who(widen__Home__Session(session))",
	)
	runGo(t, files, `package app

import "testing"

func TestWiden(t *testing.T) {
	s := Home__Session{User: "u1", N: 1}
	if got := Home__other(s); got != "user u1" {
		t.Errorf("other = %q", got)
	}
	if got := Home__both(s); got != "user u12" {
		t.Errorf("both = %q", got)
	}
	if got := Home__piped(s); got != "user u1" {
		t.Errorf("piped = %q", got)
	}
	if got := Home__name(s); got != "u1" {
		t.Errorf("name = %q", got)
	}
}
`)
}

// TestGenVetCorpus: every program in testdata/golden/gen_vet checks clean, so its emitted Go must
// vet. The corpus starts with the final M3a review's adversarial programs (adjusted where the
// language changed, see the comments in them) and grows with M3b and M3c.
func TestGenVetCorpus(t *testing.T) {
	dirs, _ := filepath.Glob("../../testdata/golden/gen_vet/*")
	if len(dirs) < 16 {
		t.Fatalf("only %d programs in the gen_vet corpus", len(dirs))
	}
	for _, dir := range dirs {
		t.Run(filepath.Base(dir), func(t *testing.T) {
			t.Parallel()
			vet(t, emit(t, dir))
		})
	}
}

// TestGen_Run runs the code emitted for the ir_patterns golden and the match cases of the
// gen_vet corpus's adv8 (one app): every match kind, nested patterns, captures, partial
// application, defaults and HTML compute the right values, not just compile (I1).
func TestGen_Run(t *testing.T) {
	srcs := map[string]string{}
	for _, f := range []string{"ir_patterns/app/P.alder", "gen_vet/adv8/app/M.alder"} {
		b, err := os.ReadFile("../../testdata/golden/" + f)
		if err != nil {
			t.Fatal(err)
		}
		srcs[filepath.Base(f)] = string(b)
	}
	runGo(t, emitSource(t, srcs), `package app

import (
	"slices"
	"testing"

	"alder/rt"
	"alder/rt/html"
)

func just(n int64) rt.Maybe[int64] { return rt.Just(n) }

var none = rt.Nothing[int64]()

func rect(w, h int64) P__Shape { return P__Shape__Rect__(payload__P__Shape__Rect{W: w, H: h}) }

func pair(a, b rt.Maybe[int64]) M__Pair { return M__Pair__P__(payload__M__Pair__P{A: a, B: b}) }

func col(c M__Color, n int64) int64 {
	return M__col(struct {
		C M__Color
		N int64
	}{C: c, N: n})
}

func TestPatterns(t *testing.T) {
	ints := func(name string, got, want int64) {
		t.Helper()
		if got != want {
			t.Errorf("%s = %d, want %d", name, got, want)
		}
	}
	texts := func(name, got, want string) {
		t.Helper()
		if got != want {
			t.Errorf("%s = %q, want %q", name, got, want)
		}
	}
	texts("describe Just 0", P__describe(just(0)), "zero")
	texts("describe Just 5", P__describe(just(5)), "n = 5")
	texts("describe Nothing", P__describe(none), "none")
	ints("firstTwo []", P__firstTwo(nil), 0)
	ints("firstTwo [7]", P__firstTwo([]int64{7}), 7)
	ints("firstTwo [1 2 3 4]", P__firstTwo([]int64{1, 2, 3, 4}), 5)
	texts("word 1", P__word(1), "one")
	texts("word 2", P__word(2), "two")
	texts("word 9", P__word(9), "many")
	texts("greet empty", P__greet(""), "nobody")
	texts("greet bo", P__greet("bo"), "hi bo")
	ints("flag", P__flag(true)*10+P__flag(false), 10)
	ints("clamp -1", P__clamp(-1), 0)
	ints("clamp 3", P__clamp(3), 8)
	ints("width Rect 0 5", P__width(rect(0, 5)), 5)
	ints("width Rect 3 5", P__width(rect(3, 5)), 3)
	ints("width Circle", P__width(P__Shape__Circle__(1.0)), 1)
	ints("point", P__point(struct {
		X int64
		Y int64
	}{X: 2, Y: 3}), 5)
	ints("size Node", P__size(P__Tree__Node__(payload__P__Tree__Node[string]{Value: "a", Size: 4})), 4)
	ints("size Leaf", P__size(P__Tree[string]{Tag: is__P__Tree__Leaf}), 0)
	if cs := P__circles([]float64{1.5}); len(cs) != 1 || cs[0].Tag != is__P__Shape__Circle || cs[0].Circle.Value != 1.5 {
		t.Errorf("circles = %+v", cs)
	}
	if got := P__addAll([]int64{1, 2}); !slices.Equal(got, []int64{0, 2, 3}) {
		t.Errorf("addAll = %v", got)
	}
	texts("joiner", P__joiner([]string{"a", "b"}), "a, b")
	if P__opts.Name != "x" || P__opts.Loud || !P__shout(P__opts).Loud {
		t.Errorf("opts = %+v", P__opts)
	}
	ints("compute 0", P__compute(0), 1)
	ints("compute 3", P__compute(3), 6)
	if got := P__both([]string{"a"}, []string{"b"}); !slices.Equal(got, []string{"a", "b"}) {
		t.Errorf("both = %v", got)
	}
	func() {
		defer func() {
			if r := recover(); r != "alder: hole ?todo" {
				t.Errorf("later: recovered %v", r)
			}
		}()
		P__later(1)
	}()
	if n := P__nodeOf("v"); n.Tag != is__P__Tree__Node || n.Node.Value != "v" || n.Node.Size != 1 {
		t.Errorf("nodeOf = %+v", n)
	}
	texts("View", html.Render(P__View(struct{ Items []rt.Maybe[int64] }{Items: []rt.Maybe[int64]{just(0), just(5), none}})),
		"<ul><li><b>zero</b></li><li>5</li><li></li></ul>")
	ints("capture Rect 4 0", P__capture(9, rect(4, 0)), 4)
	ints("capture Rect 4 1", P__capture(9, rect(4, 1)), 9)
	ints("captureList [5]", P__captureList(1, []int64{5}), 5)
	ints("captureList [5 6]", P__captureList(1, []int64{5, 6}), 1)
	if P__boxed.Tag != is__P__Shape__Rect || P__boxed.Rect.W != 1 || P__boxed.Rect.H != 2 {
		t.Errorf("boxed = %+v", P__boxed)
	}
	capRec := func(a rt.Maybe[int64], b int64) int64 {
		return P__capRec(1, struct {
			A rt.Maybe[int64]
			B int64
		}{A: a, B: b})
	}
	ints("capRec Just 7, 0", capRec(just(7), 0), 7)
	ints("capRec Just 7, 1", capRec(just(7), 1), 1)
	ints("capRec Nothing, 0", capRec(none, 0), 1)
	ints("capList [Just 4, Nothing]", P__capList(1, []rt.Maybe[int64]{just(4), none}), 4)
	ints("capList [Just 4]", P__capList(1, []rt.Maybe[int64]{just(4)}), 1)

	// adv8
	ints("both Just 1, Nothing", M__both(just(1), none), 1)
	ints("both Nothing, Nothing", M__both(none, none), 7)
	ints("both Nothing, Just 3", M__both(none, just(3)), 3)
	ints("both Just 1, Just 3", M__both(just(1), just(3)), 4)
	ints("lits a\"b", M__lits("a\"b", 0), 1)
	ints("lits é{", M__lits("é{", 0), 2)
	ints("lits z -1", M__lits("z", -1), 5)
	ints("lits z 0", M__lits("z", 0), 6)
	ints("lits z 9", M__lits("z", 9), 9)
	ints("col Red 0", col(M__Color__Red__, 0), 0)
	ints("col Red 4", col(M__Color__Red__, 4), 4)
	ints("col Green 1", col(M__Color__Green__, 1), 1)
	ints("col Blue 1", col(M__Color__Blue__, 1), 1)
	ints("col Green 5", col(M__Color__Green__, 5), 7)
	ints("col Blue 5", col(M__Color__Blue__, 5), 9)
	ints("lst [1 2 3]", M__lst([]int64{1, 2, 3}), 3)
	ints("lst [5]", M__lst([]int64{5}), 5)
	ints("lst []", M__lst(nil), 0)
	texts("res Ok 0", M__res(rt.Ok[string](int64(0))), "zero")
	texts("res Ok 7", M__res(rt.Ok[string](int64(7))), "7")
	texts("res Err empty", M__res(rt.Err[string, int64]("")), "empty")
	texts("res Err e", M__res(rt.Err[string, int64]("bad")), "bad")
	ints("bools", M__bools(true, true)*1000+M__bools(false, true)*100+M__bools(false, false)*10+M__bools(true, false), 1234)
	ints("subj 4", M__subj(4), 5)
	ints("pp Nothing, Just 2", M__pp(pair(none, just(2))), 2)
	ints("pp Nothing, Nothing", M__pp(pair(none, none)), 0)
	ints("pp Just 5, Nothing", M__pp(pair(just(5), none)), 5)
	ints("pp Just 5, Just 6", M__pp(pair(just(5), just(6))), 3)
	ints("swap Just 1, Just 5", M__swap(pair(just(1), just(5))), 4)
}
`)
}

// A generic record as a whole constructor payload keeps its type arguments: at a concrete
// instance (`B (Box Int)`), at the union's parameter (`Boxed (Box a)`), and through a generic
// record and union that refer to each other (N1).
func TestGen_RunGenericPayload(t *testing.T) {
	files := emit(t, "../../testdata/golden/gen_vet/n1-generic-payload")
	contains(t, files["M_alder.go"], "B   *M__Box[int64]", "Boxed *M__Box[A]", "Link *M__Node[A]")
	runGo(t, files, `package app

import "testing"

func TestPayload(t *testing.T) {
	if M__got != 7 || M__boxedText != "t" || M__plain != 4 || M__chain != 2 {
		t.Errorf("got, boxedText, plain, chain = %d, %q, %d, %d", M__got, M__boxedText, M__plain, M__chain)
	}
}
`)
}

// emitApp emits the app in dir and writes the files into a temporary module, returning it.
func emitApp(t *testing.T, dir string) string {
	t.Helper()
	out := t.TempDir()
	for name, b := range emit(t, dir) {
		if err := os.MkdirAll(filepath.Dir(filepath.Join(out, name)), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(out, name), b, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return out
}

// goRun runs the go command with args in dir (a generated module), failing the test on error.
func goRun(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("go", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GOWORK=off", "GOFLAGS=-mod=mod", "GOPROXY=off", "GOTOOLCHAIN=local")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("go %s failed: %v\n%s", strings.Join(args, " "), err, out)
	}
}

// Every module is emitted to "<Module>_alder.go", so a module named like a Go test file
// (Foo_test) or a build-constrained file (Bar_linux) is still an ordinary file of the package:
// `go build` sees its declarations (O1).
func TestGen_BuildSuffixModules(t *testing.T) {
	dir := emitApp(t, "../../testdata/golden/gen_vet/o1-build-suffix")
	for _, f := range []string{"Foo_test_alder.go", "Bar_linux_alder.go", "Zed_darwin_alder.go", "Qux_windows_amd64_test_alder.go", "Use_alder.go", "App_alder.go"} {
		if _, err := os.Stat(filepath.Join(dir, f)); err != nil {
			t.Fatal(err)
		}
	}
	goRun(t, dir, "build", "./...")
}

// A record field that reaches its own record through Maybe (or Result, or an anonymous record),
// directly or through other records, is held by pointer, so the Go type is not infinite; reads,
// construction, update, record and payload patterns, Eq and Dict keys all see through it (O2).
func TestGen_RunMaybeRecursion(t *testing.T) {
	files := emit(t, "../../testdata/golden/gen_vet/o2-maybe-recursion")
	contains(t, files["M_alder.go"],
		"Next *rt.Maybe[M__Node]",
		"Pong *rt.Maybe[M__Pong]",
		"Ping *rt.Maybe[M__Ping]",
		"R *rt.Result[string, M__Res]",
		"More *struct{ Next rt.Maybe[M__Deep] }",
		"Rest *rt.Maybe[M__Chain[A]]",
		"Parent *rt.Maybe[M__Crumb]",
		"_u1.Next = new(rt.Nothing[M__Node]())", "Parent: new((*p.Parent))",
	)
	runGo(t, files, `package app

import (
	"testing"

	"alder/rt/html"
)

func TestMaybeRecursion(t *testing.T) {
	if got := M__sum(M__cut(M__list)); got != 0 {
		t.Errorf("sum (cut list) = %d, want 0", got)
	}
	if got, want := html.Render(M__trail), "<span>c<span>b<span>a<b>root</b></span></span></span>"; got != want {
		t.Errorf("trail = %s, want %s", got, want)
	}
	if got := M__sum(M__list); got != 3 {
		t.Errorf("sum list = %d, want 3", got)
	}
	if got := M__sum(M__push(5, M__list)); got != 8 {
		t.Errorf("sum (push 5 list) = %d, want 8", got)
	}
	if got := M__sum(M__relink(M__list)); got != 1 {
		t.Errorf("sum (relink list) = %d, want 1", got)
	}
	if got := M__sum(M__renum(M__list)); got != 12 {
		t.Errorf("sum (renum list) = %d, want 12", got)
	}
	if got := M__second(M__list); got != 2 {
		t.Errorf("second list = %d, want 2", got)
	}
	if got := M__second(M__relink(M__list)); got != 1 {
		t.Errorf("second (relink list) = %d, want 1", got)
	}
	if M__pings != 6 || M__res != 2 || M__deeps != 11 || M__chain != 3 || M__trees != 3 {
		t.Errorf("pings, res, deeps, chain, trees = %d, %d, %d, %d, %d, want 6, 2, 11, 3, 3", M__pings, M__res, M__deeps, M__chain, M__trees)
	}
	if !M__same {
		t.Error("same is false")
	}
	if M__keyed != 2 {
		t.Errorf("keyed = %d, want 2", M__keyed)
	}
	// the original is unchanged by an update
	if l := M__relink(M__list); !M__list.Next.Ok || l.Next.Ok {
		t.Error("relink changed its argument")
	}
}
`)
}

// The Go for ir_effects' Index.load, written by hand from the emission rules before the emitter
// (the spec of effect emission): a Query block is Binds on Tasks wrapped in eff.Q once, each Db
// operation an eff.Call over rt/db with the transaction of the running effect, a Query run as
// a Task step (Time.now) converted with eff.T, and a Bool column's value encoded.
const effectsLoad = `func Index__load(params Index__Params, session Index__Session) eff.Query[Index__Model] {
	return eff.Q(eff.Bind(eff.Call("Db.all Todo", false, func(_e1 *eff.Env) ([]Todo__Todo, error) {
		return db.All(_e1.Ctx, _e1.Store.(db.Store), db.TxOf(_e1), db.Limit(10, db.OrderBy(db.Desc("created"), db.Where(db.CEq("owner", session.User), db.From(table__Todo__Todo)))))
	}), func(todos []Todo__Todo) eff.Task[Index__Model] {
		return eff.Bind(eff.All([]eff.Task[rt.Maybe[Todo__Todo]]{eff.Call("Db.get Todo", false, func(_e2 *eff.Env) (rt.Maybe[Todo__Todo], error) {
			return db.GetRow(_e2.Ctx, _e2.Store.(db.Store), db.TxOf(_e2), table__Todo__Todo, params.A)
		}), eff.Call("Db.get Todo", false, func(_e3 *eff.Env) (rt.Maybe[Todo__Todo], error) {
			return db.GetRow(_e3.Ctx, _e3.Store.(db.Store), db.TxOf(_e3), table__Todo__Todo, params.B)
		})}), func(pair []rt.Maybe[Todo__Todo]) eff.Task[Index__Model] {
			return eff.Bind(eff.T(std.TimeNow()), func(now time.Time) eff.Task[Index__Model] {
				q := db.Offset(5, db.Where(db.COr([]db.Cond{db.CContains("title", "x"), db.CEq("done", db.EncodeBool(true))}), db.From(table__Todo__Todo)))
				return eff.Bind(eff.Map(func(n int64) int64 {
					return n + 1
				}, eff.Call("Db.count Todo", false, func(_e4 *eff.Env) (int64, error) {
					return db.CountRows(_e4.Ctx, _e4.Store.(db.Store), db.TxOf(_e4), q)
				})), func(count int64) eff.Task[Index__Model] {
					return eff.Bind(eff.Call("Db.first Todo", false, func(_e5 *eff.Env) (rt.Maybe[Todo__Todo], error) {
						return db.First(_e5.Ctx, _e5.Store.(db.Store), db.TxOf(_e5), Todo__pending)
					}), func(first rt.Maybe[Todo__Todo]) eff.Task[Index__Model] {
						return eff.Pure(Index__Model{Todos: todos, Pair: pair, Now: now, Count: count, First: first})
					})
				})
			})
		})
	}))
}
`

func TestGen_EffectsLoad(t *testing.T) {
	files := emit(t, "../../testdata/golden/gen_effects")
	contains(t, files["Index_alder.go"], effectsLoad)
	contains(t, files["Todo_alder.go"],
		"var table__Todo__Todo = &db.Table[Todo__Todo]{",
		"type Cap__Todo__Stats interface {",
		"Total(ctx context.Context) (int64, error)",
		"ForOwner(ctx context.Context, a1 string, a2 bool) (int64, error)",
	)
	contains(t, files["Index_alder.go"],
		"Send(ctx context.Context, args Args__Index__Mailer__Send) (bool, error)",
		"type Args__Index__Mailer__Send struct {\n\tTo   string\n\tBody string\n}",
		`.Caps["Index.Mailer"].(Cap__Index__Mailer)`,
		`panic("alder: capability \"Index.Mailer\" is not wired")`,
		".Send(_e5.Ctx, struct {",
	)
	contains(t, files["app__alder.go"], "var Tables = []db.TableDesc{table__Todo__Todo.Desc()}",
		`var Capabilities = []string{"Todo.Stats", "Index.Mailer"}`)
}

// TestGen_ServeMigrates: the gen_migrations golden app embeds its migrations/*.sql, and Serve
// applies them to a new database (logging each), then finds nothing to apply on the next start.
func TestGen_ServeMigrates(t *testing.T) {
	dir := emitApp(t, "../../testdata/golden/gen_migrations")
	const test = `package app

import (
	"slices"
	"testing"
)

func TestServeMigrates(t *testing.T) {
	if len(Migrations) != 1 || Migrations[0].Name != "0001_initial" {
		t.Fatalf("Migrations = %v", Migrations)
	}
	path := t.TempDir() + "/app.db"
	for i, want := range [][]string{{"info: migrated 0001_initial"}, nil} {
		var logs []string
		o := Options{DB: path, Secret: []byte("0123456789abcdef0123"), Log: func(level, msg string) { logs = append(logs, level+": "+msg) }}
		if _, err := Serve(Caps{}, o); err != nil {
			t.Fatalf("start %d: %v", i, err)
		}
		if !slices.Equal(logs, want) {
			t.Fatalf("start %d logged %q, want %q", i, logs, want)
		}
	}
}
`
	if err := os.WriteFile(filepath.Join(dir, "zz_serve_test.go"), []byte(test), 0o644); err != nil {
		t.Fatal(err)
	}
	goRun(t, dir, "test", "-count=1", "-run", "TestServeMigrates", ".")
}

// TestGen_RunTodos runs the todos example end to end: its generated load and update functions,
// built as effect trees, run with eff.Run against an in-memory SQLite store created from the
// generated table descriptors, with a stub for the Auth.Login.Password capability.
func TestGen_RunTodos(t *testing.T) {
	dir := emitApp(t, "../../examples/todos")
	if err := os.WriteFile(filepath.Join(dir, "zz_run_test.go"), []byte(runTodosTest+runTodosShowTest), 0o644); err != nil {
		t.Fatal(err)
	}
	goRun(t, dir, "test", "-count=1", "-v", "-run", "TestTodos|TestServe", "./...")
}

const runTodosTest = `package app

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"alder/rt"
	"alder/rt/db"
	"alder/rt/eff"
)

// password is a stub Auth.Login.Password: a@b.c with "pw" is the user id it was made with.
type password struct{ user string }

func (p password) Verify(ctx context.Context, args Args__Auth__Login__Password__Verify) (rt.Maybe[string], error) {
	if args.Email == "a@b.c" && args.Password == "pw" {
		return rt.Just(p.user), nil
	}
	return rt.Nothing[string](), nil
}

var _ Cap__Auth__Login__Password = password{}

func TestTodos(t *testing.T) {
	ctx := context.Background()
	store, err := db.OpenSQLite(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if err := store.Ensure(ctx, Tables, true); err != nil {
		t.Fatal(err)
	}
	clock := time.Date(2026, 10, 9, 12, 0, 0, 0, time.UTC)
	var logs []string
	env := &eff.Env{Ctx: ctx, Store: store, Caps: map[string]any{}, Now: func() time.Time { return clock },
		Log: func(level, msg string) { logs = append(logs, level+": "+msg) }}

	// a user to own the todos (Todo.owner is @ref User)
	uid, err := eff.Run(env, eff.KTask, eff.Call("Db.insert User", true, func(e *eff.Env) (string, error) {
		return db.InsertRow(e.Ctx, e.Store.(db.Store), db.TxOf(e), table__User__User,
			insertvals__User__User(insert__User__User{Email: "a@b.c", Name: "Ann", PasswordHash: "x"}), e.Now())
	}))
	if err != nil || uid == "" {
		t.Fatalf("insert user: %q, %v", uid, err)
	}
	env.Session = App__Session{User: rt.Nothing[string]()}
	if len(Capabilities) != 1 || Capabilities[0] != "Auth.Login.Password" {
		t.Fatalf("Capabilities = %v", Capabilities)
	}

	// a capability that is not wired panics with its registry key (and the Task rolls back)
	func() {
		defer func() {
			if r := recover(); r != "alder: capability \"Auth.Login.Password\" is not wired" {
				t.Errorf("unwired capability: recovered %v", r)
			}
		}()
		msg := Auth__Login__Msg__Submit__(Auth__Login__SubmitParams{Form: Auth__Login__Credentials{Email: "a@b.c", Password: "pw"}})
		eff.Run(env, eff.KTask, Auth__Login__update(msg, Auth__Login__Model{}, env.Session.(App__Session)))
	}()
	env.Caps["Auth.Login.Password"] = password{user: uid}

	// calls asserts the Call steps run since mark
	mark := 0
	calls := func(what string, want ...string) {
		t.Helper()
		got := env.Trace[mark:]
		mark = len(env.Trace)
		if strings.Join(got, ", ") != strings.Join(want, ", ") {
			t.Errorf("%s ran %v, want %v", what, got, want)
		}
	}
	calls("setup", "Db.insert User", "Password.verify")

	// log in: a wrong password is rejected, the right one redirects to / and sets the session
	login := func(pw string) rt.Nav {
		t.Helper()
		msg := Auth__Login__Msg__Submit__(Auth__Login__SubmitParams{Form: Auth__Login__Credentials{Email: "a@b.c", Password: pw}})
		nav, err := eff.Run(env, eff.KTask, Auth__Login__update(msg, Auth__Login__Model{}, env.Session.(App__Session)))
		if err != nil {
			t.Fatalf("login: %v", err)
		}
		return nav
	}
	if nav := login("nope"); nav.Tag != rt.NavReject || nav.Text != "Email or password is incorrect." {
		t.Fatalf("bad login: %+v", nav)
	}
	if s := env.Session.(App__Session); s.User.Ok {
		t.Fatalf("bad login set the user: %+v", s)
	}
	if nav := login("pw"); nav.Tag != rt.NavRedirect || nav.Page.Path != "/" {
		t.Fatalf("login: %+v", nav)
	}
	calls("login", "Password.verify", "Password.verify", "Session.put")
	app := env.Session.(App__Session)
	if !app.User.Ok || app.User.Val != uid {
		t.Fatalf("login did not set the session user: %+v", app)
	}
	session := Todos__Index__Session{User: app.User.Val}
	model := Todos__Index__Model{Filter: Todo__Filter__All__}

	update := func(msg Todos__Index__Msg) (rt.Nav, error) {
		t.Helper()
		return eff.Run(env, eff.KTask, Todos__Index__update(msg, model, session))
	}
	load := func(f Todo__Filter) []Todo__Todo {
		t.Helper()
		m, err := eff.RunQuery(env, Todos__Index__load(Todos__Index__Params{Filter: f}, session))
		if err != nil {
			t.Fatalf("load: %v", err)
		}
		return m.Todos
	}

	// add
	add := Todos__Index__Msg__Submit__(Todos__Index__SubmitParams{Form: Todos__Index__NewTodo{Title: "Buy milk", Due: rt.Just(rt.Date{Y: 2026, M: 10, D: 31})}})
	nav, err := update(add)
	if err != nil || nav.Tag != rt.NavRedirect || nav.Page.Path != "/" {
		t.Fatalf("add: %+v, %v", nav, err)
	}
	calls("add", "Db.insert Todo")
	todos := load(Todo__Filter__All__)
	if len(todos) != 1 {
		t.Fatalf("after add: %d todos", len(todos))
	}
	td := todos[0]
	if td.Title != "Buy milk" || td.Done || td.Owner != uid || !td.Created.Equal(clock) || !td.Due.Ok || td.Due.Val != (rt.Date{Y: 2026, M: 10, D: 31}) {
		t.Fatalf("added todo: %+v", td)
	}
	if n := len(load(Todo__Filter__Completed__)); n != 0 {
		t.Fatalf("completed before toggle: %d", n)
	}

	// toggle
	mark = len(env.Trace)
	nav, err = update(Todos__Index__Msg__Toggle__(Todos__Index__ToggleParams{Id: td.Id}))
	if err != nil || nav.Tag != rt.NavRerender {
		t.Fatalf("toggle: %+v, %v", nav, err)
	}
	calls("toggle", "Db.get Todo", "Db.update Todo") // the ownership check, then the write
	if todos = load(Todo__Filter__All__); len(todos) != 1 || !todos[0].Done {
		t.Fatalf("after toggle: %+v", todos)
	}
	if n := len(load(Todo__Filter__Active__)); n != 0 {
		t.Fatalf("active after toggle: %d", n)
	}

	// the show page loads the user's own todo, and not one that does not exist
	mark = len(env.Trace)
	shown, err := eff.RunQuery(env, Todos__Show__load(Todos__Show__Params{Id: td.Id}, Todos__Show__Session{User: uid}))
	if err != nil || shown.Todo.Title != "Buy milk" {
		t.Fatalf("show: %+v, %v", shown, err)
	}
	calls("Show.load", "Db.get Todo")
	if _, err := eff.RunQuery(env, Todos__Show__load(Todos__Show__Params{Id: "nope"}, Todos__Show__Session{User: uid})); err != eff.ErrNotFound {
		t.Fatalf("show missing: err = %v, want ErrNotFound", err)
	}
	if _, err := eff.RunQuery(env, Todos__Show__load(Todos__Show__Params{Id: td.Id}, Todos__Show__Session{User: "someone else"})); err != eff.ErrNotFound {
		t.Fatalf("show other's: err = %v, want ErrNotFound", err)
	}

	// toggling a todo that does not exist, or another user's, is Task.notFound (a 404)
	if _, err := update(Todos__Index__Msg__Toggle__(Todos__Index__ToggleParams{Id: "nope"})); err != eff.ErrNotFound {
		t.Fatalf("toggle missing: err = %v, want ErrNotFound", err)
	}
	other := Todos__Index__Session{User: "someone else"}
	if _, err := eff.Run(env, eff.KTask, Todos__Index__update(Todos__Index__Msg__Delete__(Todos__Index__DeleteParams{Id: td.Id}), model, other)); err != eff.ErrNotFound {
		t.Fatalf("delete other's: err = %v, want ErrNotFound", err)
	}

	// rollback: an add followed by a failure leaves no row
	failing := eff.Bind(Todos__Index__update(add, model, session), func(rt.Nav) eff.Task[rt.Nav] { return eff.Fail[rt.Nav]("boom") })
	var fail *eff.ErrFail
	if _, err := eff.Run(env, eff.KTask, failing); !errors.As(err, &fail) || fail.Msg != "boom" {
		t.Fatalf("failing add: err = %v", err)
	}
	if n := len(load(Todo__Filter__All__)); n != 1 {
		t.Fatalf("after rolled-back add: %d todos, want 1", n)
	}

	// delete
	mark = len(env.Trace)
	nav, err = update(Todos__Index__Msg__Delete__(Todos__Index__DeleteParams{Id: td.Id}))
	if err != nil || nav.Tag != rt.NavRerender {
		t.Fatalf("delete: %+v, %v", nav, err)
	}
	calls("delete", "Db.get Todo", "Db.delete Todo")
	if n := len(load(Todo__Filter__All__)); n != 0 {
		t.Fatalf("after delete: %d todos", n)
	}
	calls("load", "Db.all Todo")

	// a Task that fails after Session.put leaves the session as it was (I1); a successful one
	// that touches no table begins no transaction (I2: the in-memory store's single connection
	// would otherwise be held)
	before := env.Session.(App__Session)
	logoutThenFail := eff.Bind(App__update(App__Msg__Logout__, before), func(rt.Nav) eff.Task[rt.Nav] { return eff.Fail[rt.Nav]("boom") })
	if _, err := eff.Run(env, eff.KTask, logoutThenFail); !errors.As(err, &fail) || fail.Msg != "boom" {
		t.Fatalf("logout then fail: err = %v", err)
	}
	if s := env.Session.(App__Session); s != before || !s.User.Ok {
		t.Fatalf("failed Task kept its session change: %+v, want %+v", s, before)
	}
	// App's update: log out
	if nav, err := eff.Run(env, eff.KTask, App__update(App__Msg__Logout__, env.Session.(App__Session))); err != nil || nav.Tag != rt.NavRedirect || nav.Page.Path != "/login" {
		t.Fatalf("logout: %+v, %v", nav, err)
	}
	if s := env.Session.(App__Session); s.User.Ok {
		t.Fatalf("logout kept the user: %+v", s)
	}
	t.Logf("trace: %v", env.Trace)
	t.Logf("log: %d lines", len(logs))
}

// Serve refuses a capability left unset, naming the Caps field; the session codec round-trips
// App's session as JSON and starts a cookie that lacks a field from the field's default.
func TestServe(t *testing.T) {
	if _, err := Serve(Caps{}, Options{DB: ":memory:", Dev: true}); err == nil || err.Error() != "capability Auth.Login.Password is not wired: set Caps.Auth__Login__Password" {
		t.Fatalf("Serve without the capability: %v", err)
	}
	if h, err := Serve(Caps{Auth__Login__Password: password{}}, Options{Dev: true}); err != nil || h == nil {
		t.Fatalf("Serve: %v", err)
	}
	// outside dev mode the cookie key must be a real secret
	for _, secret := range []string{"", "fifteen-bytes!!"} {
		if _, err := Serve(Caps{Auth__Login__Password: password{}}, Options{Secret: []byte(secret)}); err == nil || err.Error() != "ALDER_SECRET must be set to at least 16 bytes outside dev mode (ALDER_DEV=1)" {
			t.Fatalf("Serve in production with secret %q: %v", secret, err)
		}
	}
	if h, err := Serve(Caps{Auth__Login__Password: password{}}, Options{Secret: []byte("sixteen-bytes!!!")}); err != nil || h == nil {
		t.Fatalf("Serve in production with a secret: %v", err)
	}
	c := session__codec{}
	if d := c.Default().(App__Session); d.User.Ok {
		t.Fatalf("Default = %+v", d)
	}
	s := App__Session{User: rt.Just("u")}
	b, err := c.Encode(s)
	if err != nil {
		t.Fatal(err)
	}
	if got, err := c.Decode(b); err != nil || got.(App__Session) != s {
		t.Fatalf("Decode(Encode(s)) = %+v, %v", got, err)
	}
	if got, err := c.Decode([]byte(` + "`" + `{"User":{"Ok":true,"Val":"v"}}` + "`" + `)); err != nil || got.(App__Session).User.Val != "v" {
		t.Fatalf("Decode of a partial session = %+v, %v", got, err)
	}
	if _, err := c.Decode([]byte("nope")); err == nil {
		t.Fatal("Decode of garbage: no error")
	}
}
`

// The conditions of the M3b corpus program run against SQLite: every column kind is encoded
// with its column's encoder (enum, Maybe enum, Date, Maybe Date, Time, Bool, Int, Text, Maybe
// Text), an enum compares by declaration order, `in` takes encoded lists, Db.and of none is
// true and Db.or of none false, a table passed as a value, and Query capability entries.
func TestGen_RunEffectsCorpus(t *testing.T) {
	dir := emitApp(t, "../../testdata/golden/gen_vet/m3b-effects")
	if err := os.WriteFile(filepath.Join(dir, "zz_run_test.go"), []byte(runShopTest), 0o644); err != nil {
		t.Fatal(err)
	}
	goRun(t, dir, "test", "-count=1", "-run", "TestShop", "./...")
}

const runShopTest = `package app

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"alder/rt"
	"alder/rt/db"
	"alder/rt/eff"
)

type pay struct{}

// Charge writes a row through the running Task's transaction (I3): it commits or rolls back
// with the Task.
func (pay) Charge(ctx context.Context, args Args__Shop__Pay__Charge) (rt.Maybe[string], error) {
	row := insertvals__Customer__Customer(insert__Customer__Customer{Email: args.Card + "@pay", Seen: rt.Nothing[time.Time]()})
	_, err := db.InsertRow(ctx, db.StoreFrom(ctx), db.TxFrom(ctx), table__Customer__Customer, row, time.Now())
	return rt.Just(args.Card), err
}
func (pay) Balance(ctx context.Context) (float64, error) { return 2.5, nil }

// Limit checks that a capability's ctx carries the running effect (I3): in a Query there is
// no transaction to reach.
func (pay) Limit(ctx context.Context, a1 string, a2 int64) (int64, error) {
	if eff.EnvFrom(ctx) == nil || db.StoreFrom(ctx) == nil || db.TxFrom(ctx) != nil {
		return 0, errors.New("capability ctx does not carry the Query's Env")
	}
	return a2 * 10, nil
}

var _ Cap__Shop__Pay = pay{}

func TestShop(t *testing.T) {
	ctx := context.Background()
	store, err := db.OpenSQLite(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if err := store.Ensure(ctx, Tables, true); err != nil {
		t.Fatal(err)
	}
	clock := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	env := &eff.Env{Ctx: ctx, Store: store, Caps: map[string]any{"Shop.Pay": pay{}}, Now: func() time.Time { return clock }}
	uid, err := eff.Run(env, eff.KTask, Shop__register("a@b.c"))
	if err != nil {
		t.Fatal(err)
	}
	day := rt.Date{Y: 2026, M: 1, D: 1}
	open, paid, shipped := Order__Status__Open__, Order__Status__Paid__, Order__Status__Shipped__
	add := func(code string, st Order__Status, late rt.Maybe[Order__Status], qty int64, gift bool, note rt.Maybe[string]) {
		t.Helper()
		_, err := eff.Run(env, eff.KTask, eff.Call("add", true, func(e *eff.Env) (string, error) {
			return db.InsertRow(e.Ctx, e.Store.(db.Store), db.TxOf(e), table__Order__Order, insertvals__Order__Order(insert__Order__Order{
				Buyer: uid, Status: st, Late: late, Total: 2, Qty: qty, Gift: gift, Day: day, ShipDay: rt.Nothing[rt.Date](), Code: code, Note: note}), e.Now())
		}))
		if err != nil {
			t.Fatalf("add %s: %v", code, err)
		}
	}
	add("x1", open, rt.Just(open), 1, false, rt.Just("n"))     // matches search open
	add("x2", paid, rt.Just(paid), 2, false, rt.Just("n"))     // matches search paid
	add("x3", shipped, rt.Just(shipped), 3, false, rt.Just("n")) // shipped: not < Shipped
	add("y4", open, rt.Just(open), 1, false, rt.Just("n"))     // code lacks x
	add("x5", open, rt.Nothing[Order__Status](), 1, true, rt.Nothing[string]())

	all := func(s db.Select[Order__Order]) []Order__Order {
		t.Helper()
		rows, err := eff.RunQuery(env, eff.Q(eff.Call("all", false, func(e *eff.Env) ([]Order__Order, error) {
			return db.All(e.Ctx, e.Store.(db.Store), db.TxOf(e), s)
		})))
		if err != nil {
			t.Fatal(err)
		}
		return rows
	}
	later := clock.Add(time.Hour)
	if got := all(Shop__search(open, day, later, []Order__Status{open, paid})); len(got) != 1 || got[0].Code != "x1" || got[0].Late.Val.Tag != is__Order__Status__Open {
		t.Errorf("search open: %+v", got)
	}
	if got := all(Shop__search(paid, day, later, []Order__Status{paid})); len(got) != 1 || got[0].Code != "x2" {
		t.Errorf("search paid: %+v", got)
	}
	if got := all(Shop__search(shipped, day, later, []Order__Status{shipped})); len(got) != 0 {
		t.Errorf("search shipped (not < Shipped): %+v", got)
	}
	if got := all(Shop__search(open, day, clock, []Order__Status{open})); len(got) != 0 {
		t.Errorf("search created before the clock: %+v", got)
	}
	if got := all(Shop__either(nil)); len(got) != 0 {
		t.Errorf("Db.or [] matched %d rows", len(got))
	}
	if got := all(Shop__either([]db.Cond{Shop__conds(nil)})); len(got) != 0 {
		t.Errorf("Db.not (Db.and []) matched %d rows", len(got))
	}
	if got := all(db.Where(Shop__conds(nil), db.From(table__Order__Order))); len(got) != 5 {
		t.Errorf("Db.or [Db.and [], Db.not (Db.or [])] matched %d rows, want 5", len(got))
	}
	if n, err := eff.RunQuery(env, Shop__allOrders); err != nil || n != 5 {
		t.Errorf("allOrders = %d, %v", n, err)
	}
	if ls, err := eff.RunQuery(env, Shop__allLimits(uid)); err != nil || len(ls) != 2 || ls[1] != 20 {
		t.Errorf("allLimits = %v, %v", ls, err)
	}
	if cs, err := eff.RunQuery(env, Shop__seenBefore(clock)); err != nil || len(cs) != 0 {
		t.Errorf("seenBefore = %v, %v", cs, err)
	}
	if k, err := eff.Run(env, eff.KTask, Shop__runJob(Shop__job)); err != nil || k != 1 {
		t.Errorf("runJob = %d, %v", k, err)
	}
	if nav := Shop__after(rt.Reject("no")); nav.Tag != rt.NavReject || nav.Text != "no!" || !Shop__isRedirect(rt.Redirect(rt.Page{Path: "/"})) {
		t.Errorf("after = %+v", nav)
	}

	// a capability's write is part of the Task: rolled back when the Task fails, kept when it
	// succeeds (I3)
	customers := func() int64 {
		t.Helper()
		n, err := eff.RunQuery(env, eff.Q(eff.Call("count", false, func(e *eff.Env) (int64, error) {
			return db.CountRows(e.Ctx, e.Store.(db.Store), db.TxOf(e), db.From(table__Customer__Customer))
		})))
		if err != nil {
			t.Fatal(err)
		}
		return n
	}
	before := customers()
	chargeThenFail := eff.Bind(Shop__charges[0], func(rt.Maybe[string]) eff.Task[int64] { return eff.Fail[int64]("declined") })
	if _, err := eff.Run(env, eff.KTask, chargeThenFail); err == nil || err.Error() != "declined" {
		t.Fatalf("charge then fail: %v", err)
	}
	if n := customers(); n != before {
		t.Errorf("customers after a rolled-back charge: %d, want %d", n, before)
	}
	if card, err := eff.Run(env, eff.KTask, Shop__charges[0]); err != nil || card.Val != "a" {
		t.Fatalf("charge: %+v %v", card, err)
	}
	if n := customers(); n != before+1 {
		t.Errorf("customers after a charge: %d, want %d", n, before+1)
	}

	// conditions on Maybe columns are two-valued (C1)
	codes := func(rows []Order__Order) string {
		var out []string
		for _, r := range rows {
			out = append(out, r.Code)
		}
		return strings.Join(out, " ")
	}
	if got := codes(all(Shop__lateIn(open))); got != "x1 x5 y4" {
		t.Errorf("late in [Just Open, Nothing] = %s", got)
	}
	if got := codes(all(Shop__lateNothing)); got != "x5" {
		t.Errorf("late in [Nothing] = %s", got)
	}
	if got := codes(all(Shop__notLateIn([]rt.Maybe[Order__Status]{rt.Just(paid)}))); got != "x1 x3 x5 y4" {
		t.Errorf("not (late in [Just Paid]) = %s", got)
	}
	if got := codes(all(Shop__notLateIn([]rt.Maybe[Order__Status]{rt.Just(paid), rt.Nothing[Order__Status]()}))); got != "x1 x3 y4" {
		t.Errorf("not (late in [Just Paid, Nothing]) = %s", got)
	}
	if got := codes(all(Shop__notNote("n"))); got != "x5" {
		t.Errorf("not (note eq (Just n)) = %s", got)
	}

	// Time columns compare and order correctly below a second: x6 is created half a second
	// after the others (whole-second clock)
	clock = clock.Add(500 * time.Millisecond)
	add("x6", open, rt.Just(open), 1, false, rt.Just("n"))
	if got := all(Shop__createdBefore(clock)); len(got) != 5 {
		t.Errorf("created < clock+0.5s: %d rows, want 5", len(got))
	}
	if got := all(Shop__createdBefore(clock.Add(time.Nanosecond))); len(got) != 6 || got[0].Code != "x6" {
		t.Errorf("created < clock+0.5s+1ns, newest first: %d rows, first %+v", len(got), got)
	}
	if got := all(Shop__newestFirst); len(got) != 6 || got[0].Code != "x6" || !got[0].Created.Equal(clock) {
		t.Errorf("newest first: %+v", got[:1])
	}
}
`

// TestGen_Routes: the routes of the ir_page app, against a golden written by hand before the
// emitter (the spec of route emission): decoders, page__/update__ functions, the GET and POST
// routes and their helpers. The whole app vets.
func TestGen_Routes(t *testing.T) {
	files := emit(t, "../../testdata/golden/ir_page")
	want := "../../testdata/golden/gen_routes/want/routes__alder.go"
	if *update {
		if err := os.WriteFile(want, files["routes__alder.go"], 0o644); err != nil {
			t.Fatal(err)
		}
	}
	w, err := os.ReadFile(want)
	if err != nil {
		t.Fatal(err)
	}
	if got := string(files["routes__alder.go"]); got != string(w) {
		t.Errorf("routes__alder.go mismatch\n got:\n%s\nwant:\n%s", got, w)
	}
	vet(t, files)
}

// A message's parameters, the page's and the form's never mix: the page renders every message
// URL with its own parameters under "_p." keys (html.FormState.PageParams), the route decodes
// the page's from those only, the message's from the other keys, the form from the body — so
// a page id, a message id and a form field id decode independently, a re-render keeps the
// page's query parameters, and a URL without the page's path parameter is a 404. A List
// message parameter is one key per element and round-trips. Without Datastar a Rerender is a
// 303 to the page (PRG). A Rerender whose re-load fails still stores the session the Task left. The session cookie stores unions by name.
func TestDecode_ParamsAndFormDontMix(t *testing.T) {
	files := emitSource(t, map[string]string{
		"App.alder": `module App

type Theme = Light | Dark

session { n: Int = 0, theme: Theme = Light, last: Maybe Theme }

error : Error -> Html Never
error err = <p>error</p>

notFound : Html Never
notFound = <p>not found</p>
`,
		"M.alder": `module M

page "/m/{id}" { id: Text, filter: Text = "all" }

type Model = { id: Text, filter: Text }

load : Params -> Session -> Query Model
load params session = if session.n > 0 then Query.notFound else Query.succeed { id: params.id, filter: params.filter }

form F = { id: Text, title: Text }

msg Save { id: Text } form F
msg Ping
msg Pick { ids: List Int }
msg Vanish

update : Msg -> Model -> Session -> Task Nav
update msg model session = match msg {
  Save p -> Task.succeed (Redirect (O { a: p.id, b: p.form.id ++ "/" ++ p.form.title ++ "/" ++ model.id })),
  Ping p -> Task.succeed Rerender,
  Pick p -> Task.succeed (Redirect (O { a: Text.join "," (List.map Text.fromInt p.ids), b: model.id })),
  Vanish p -> {
    let _ <- Session.put (\s -> { s | n: s.n + 1 })
    Task.succeed Rerender
  },
}

view : Model -> Session -> Html Msg
view model session =
  <div>
    <p>{model.id ++ " " ++ model.filter}</p>
    <button click={Ping}>ping</button>
    <button click={Pick { ids: [1, 2] }}>pick</button>
    <form submit={Save { id: "m1" }}><Input field="id" /></form>
  </div>
`,
		"O.alder": `module O

page "/o" { a: Text, b: Text }

type Model = { }

load : Params -> Session -> Query Model
load params session = Query.succeed { }

view : Model -> Session -> Html Msg
view model session = <p>o</p>
`,
	})
	runGo(t, files, `package app

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"alder/rt"
	"alder/rt/web"
)

func TestMix(t *testing.T) {
	h, err := Serve(Caps{}, Options{DB: ":memory:", Dev: true})
	if err != nil {
		t.Fatal(err)
	}
	do := func(method, target, body string, hdr ...string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, target, strings.NewReader(body))
		r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		for _, h := range hdr {
			k, v, _ := strings.Cut(h, ": ")
			r.Header.Set(k, v)
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w
	}
	// the page renders its parameters into every message URL
	w := do("GET", "/m/p0?filter=x", "")
	for _, want := range []string{
		"<p>p0 x</p>",
		`+"`"+`<form method="post" action="/_m/M/Ping?_p.id=p0&amp;_p.filter=x" class="_alder-click"`+"`"+`,
		`+"`"+`<form method="post" action="/_m/M/Pick?ids=1&amp;ids=2&amp;_p.id=p0&amp;_p.filter=x" class="_alder-click"`+"`"+`,
		`+"`"+`action="/_m/M/Save?id=m1&amp;_p.id=p0&amp;_p.filter=x"`+"`"+`,
	} {
		if w.Code != 200 || !strings.Contains(w.Body.String(), want) {
			t.Fatalf("GET lacks %s: %d %s", want, w.Code, w.Body.String())
		}
	}
	// at its default a page parameter is left out
	if w := do("GET", "/m/p0", ""); !strings.Contains(w.Body.String(), `+"`"+`action="/_m/M/Ping?_p.id=p0"`+"`"+`) {
		t.Fatalf("GET at the default filter: %s", w.Body.String())
	}
	// page id, message id and form id are three values
	w = do("POST", "/_m/M/Save?id=m1&_p.id=p0", "id=f1&title=x")
	if w.Code != http.StatusSeeOther || w.Header().Get("Location") != "/o?a=m1&b=f1%2Fx%2Fp0" {
		t.Fatalf("Save: %d %q", w.Code, w.Header().Get("Location"))
	}
	// a form error re-renders the page the message came from
	if w := do("POST", "/_m/M/Save?id=m1&_p.id=p0&_p.filter=x", "title=x"); w.Code != 200 || !strings.Contains(w.Body.String(), "<p>p0 x</p>") {
		t.Fatalf("Save without the form's id: %d %s", w.Code, w.Body.String())
	}
	// a Rerender under Datastar keeps the page's query parameter, in the patch's URLs too
	w = do("POST", "/_m/M/Ping?_p.id=p0&_p.filter=x", "", "Datastar-Request: true")
	if w.Code != 200 || !strings.Contains(w.Body.String(), "event: datastar-patch-elements") || !strings.Contains(w.Body.String(), "<p>p0 x</p>") ||
		!strings.Contains(w.Body.String(), "/_m/M/Ping?_p.id=p0&amp;_p.filter=x") {
		t.Fatalf("Ping: %d %s", w.Code, w.Body.String())
	}
	// without Datastar a Rerender is a 303 back to the page, its parameters kept (PRG)
	w = do("POST", "/_m/M/Ping?_p.id=p%2F0&_p.filter=x", "")
	if w.Code != http.StatusSeeOther || w.Header().Get("Location") != "/m/p%2F0?filter=x" {
		t.Fatalf("Ping without Datastar: %d %q", w.Code, w.Header().Get("Location"))
	}
	// no page path parameter: 404 (the Referer is not consulted)
	if w := do("POST", "/_m/M/Ping?id=p0", "", "Referer: http://h/m/p0"); w.Code != http.StatusNotFound {
		t.Fatalf("Ping without _p.id: %d", w.Code)
	}
	// a List message parameter round-trips
	if w := do("POST", "/_m/M/Pick?ids=1&ids=2&_p.id=p0", ""); w.Code != http.StatusSeeOther || w.Header().Get("Location") != "/o?a=1%2C2&b=p0" {
		t.Fatalf("Pick: %d %q", w.Code, w.Header().Get("Location"))
	}
	// without Datastar the Task stores n = 1 on its 303 (the page then 404s)
	w = do("POST", "/_m/M/Vanish?_p.id=p0", "")
	if w.Code != http.StatusSeeOther || w.Header().Get("Location") != "/m/p0" || !strings.HasPrefix(w.Header().Get("Set-Cookie"), web.CookieName+"=") {
		t.Fatalf("Vanish without Datastar: %d %q %q", w.Code, w.Header().Get("Location"), w.Header().Get("Set-Cookie"))
	}
	// under Datastar the Task stored n = 1, then the re-load is Query.notFound: 404 (patched),
	// and the session is stored
	w = do("POST", "/_m/M/Vanish?_p.id=p0", "", "Datastar-Request: true")
	sc := w.Header().Get("Set-Cookie")
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), "not found") || !strings.HasPrefix(sc, web.CookieName+"=") {
		t.Fatalf("Vanish: %d, Set-Cookie %q", w.Code, sc)
	}
	v, _, _ := strings.Cut(strings.TrimPrefix(sc, web.CookieName+"="), ";")
	payload, _, _ := strings.Cut(v, ".")
	raw, _ := base64.RawURLEncoding.DecodeString(payload)
	var env struct{ Session json.RawMessage }
	_ = json.Unmarshal(raw, &env) // the signed payload: {"exp": …, "session": …}
	if s, err := (session__codec{}).Decode(env.Session); err != nil || s.(App__Session).N != 1 {
		t.Fatalf("session after Vanish: %s %v", raw, err)
	}
}

func TestSessionCodec(t *testing.T) {
	c := session__codec{}
	b, err := c.Encode(App__Session{N: 2, Theme: App__Theme__Dark__, Last: rt.Just(App__Theme__Light__)})
	if err != nil || string(b) != `+"`"+`{"Last":"Light","N":2,"Theme":"Dark"}`+"`"+` {
		t.Fatalf("Encode = %s, %v", b, err)
	}
	for raw, want := range map[string]App__Session{
		`+"`"+`{"Last":"Light","N":2,"Theme":"Dark"}`+"`"+`: {N: 2, Theme: App__Theme__Dark__, Last: rt.Just(App__Theme__Light__)},
		`+"`"+`{"Theme":"Gone","Last":"Nope","N":3}`+"`"+`:  {N: 3, Theme: App__Theme__Light__},
		`+"`"+`{"Last":null,"N":"x"}`+"`"+`:                    {Theme: App__Theme__Light__},
		`+"`"+`{}`+"`"+`:                                        {Theme: App__Theme__Light__},
	} {
		got, err := c.Decode([]byte(raw))
		if err != nil || got.(App__Session) != want {
			t.Errorf("Decode(%s) = %+v, %v; want %+v", raw, got, err, want)
		}
	}
	if _, err := c.Decode([]byte("nope")); err == nil {
		t.Error("Decode of garbage: no error")
	}
}
`)
}

// appModule is a minimal App for the emitted-app tests.
const appModule = `module App

error : Error -> Html Never
error err = <p>error</p>

notFound : Html Never
notFound = <p>not found</p>
`

// A brand's `where` check runs where a form or URL decodes it: an app brand's by its check__
// function ("is not valid"), the stdlib refinements by rt/web's helpers; inside a Maybe only
// when present. A Text.Secret field renders as a password input bound to its form.
func TestGen_BrandChecks(t *testing.T) {
	files := emitSource(t, map[string]string{
		"App.alder": appModule,
		"M.alder": `module M

type Slug = brand Text where Text.matches "^[a-z]+$"
type Age = brand Int where (\n -> n >= 0 && n < 150)
type Plain = brand Text

page "/m" { }

type Model = { }

load : Params -> Session -> Query Model
load params session = Query.succeed { }

form F = {
  slug: Slug,
  alias: Maybe Slug,
  age: Age,
  email: Text.Email,
  name: Text.NonEmpty,
  pw: Text.Secret,
  site: Maybe Text.Url,
  n: Int.Positive,
  plain: Plain,
}

msg Save form F

update : Msg -> Model -> Session -> Task Nav
update msg model session = Task.succeed Rerender

view : Model -> Session -> Html Msg
view model session = <form submit={Save}><Input field="pw" /><Input field="slug" /></form>
`,
	})
	contains(t, files["M_alder.go"], "func check__M__Slug(v string) bool {\n\treturn std.TextMatches(\"^[a-z]+$\", v)\n}",
		`html.Input(html.Control{Kind: "input", Form: "M.F", Name: "pw", Type: "text", Required: true, Secret: true})`)
	runGo(t, files, `package app

import (
	"net/url"
	"reflect"
	"strings"
	"testing"

	"alder/rt/html"
	"alder/rt/web"
)

func TestBrands(t *testing.T) {
	good := url.Values{"slug": {"abc"}, "alias": {""}, "age": {"30"}, "email": {"a@b.c"}, "name": {"x"}, "pw": {" "}, "site": {""}, "n": {"1"}, "plain": {""}}
	f, errs := decode__M__F(good)
	if errs != nil || f.Slug != "abc" || f.Alias.Ok || f.Age != 30 || f.Email != "a@b.c" || f.Pw != " " || f.Site.Ok || f.N != 1 {
		t.Fatalf("good: %+v %v", f, errs)
	}
	if f, errs := decode__M__F(url.Values{"slug": {"abc"}, "alias": {"de"}, "age": {"0"}, "email": {"a@b.c"}, "name": {"x"}, "pw": {"p"}, "site": {"http://x.y"}, "n": {"1"}, "plain": {"p"}}); errs != nil || f.Alias.Val != "de" || f.Site.Val != "http://x.y" {
		t.Fatalf("present Maybes: %+v %v", f, errs)
	}
	bad := url.Values{"slug": {"A1"}, "alias": {"B"}, "age": {"200"}, "email": {"x"}, "name": {"  "}, "pw": {""}, "site": {"nope"}, "n": {"0"}, "plain": {""}}
	_, errs = decode__M__F(bad)
	want := []web.FieldError{{Field: "slug", Message: "is not valid"}, {Field: "alias", Message: "is not valid"}, {Field: "age", Message: "is not valid"},
		{Field: "email", Message: web.MsgEmail}, {Field: "name", Message: web.MsgNonEmpty}, {Field: "pw", Message: web.MsgSecret},
		{Field: "site", Message: web.MsgURL}, {Field: "n", Message: "must be positive"}}
	if !reflect.DeepEqual(errs, want) {
		t.Fatalf("bad:\n got %v\nwant %v", errs, want)
	}
	_, errs = decode__M__F(url.Values{"age": {" "}, "n": {""}})
	want = []web.FieldError{web.Missing("slug"), web.Missing("age"), web.Missing("email"), web.Missing("name"), web.Missing("pw"), web.Missing("n"), web.Missing("plain")}
	if !reflect.DeepEqual(errs, want) {
		t.Fatalf("missing:\n got %v\nwant %v", errs, want)
	}
	if _, errs := decode__M__F(url.Values{"slug": {"abc"}, "age": {"x"}, "email": {"a@b.c"}, "name": {"x"}, "pw": {"p"}, "n": {"1"}, "plain": {""}}); !reflect.DeepEqual(errs, []web.FieldError{{Field: "age", Message: web.MsgInt}}) {
		t.Fatalf("malformed Int: %v", errs)
	}
	page := html.RenderWith(M__view(M__Model{}, App__Session{}), &html.FormState{Form: "M.F", Msg: "/_m/M/Save", Values: map[string]string{"slug": "A1"}, Errors: map[string][]string{}})
	if !strings.Contains(page, `+"`"+`<input name="pw" type="password" required />`+"`"+`) || !strings.Contains(page, `+"`"+`value="A1"`+"`"+`) {
		t.Fatalf("view: %s", page)
	}
}
`)
}

// A link to a page carries its path parameters in the path (each escaped as one segment) and
// the others as the query, in declaration order, rendered as text; a parameter at its default
// and a Maybe at Nothing are left out. The page's decoder reads the link back.
func TestGen_PageURLQuery(t *testing.T) {
	files := emitSource(t, map[string]string{
		"App.alder": appModule,
		"T.alder": `module T

type Filter = All | Active | Done

page "/t/{id}/x" { id: Text, filter: Filter = All, q: Maybe Text, n: Int, b: Bool = False, tags: List Text, m: Maybe Int = Just 3 }

type Model = { }

load : Params -> Session -> Query Model
load params session = Query.succeed { }

view : Model -> Session -> Html Msg
view model session = <p>t</p>

link1 : Page
link1 = T { id: "a/b c", filter: Active, q: Just "x&y", n: 2, b: True, tags: ["p", "q r"], m: Nothing }

link2 : Text -> Page
link2 id = T { id: id, q: Nothing, n: -1, tags: [] }
`,
		"I.alder": `module I

import T

page "/" { filter: T.Filter = All }

type Model = { }

load : Params -> Session -> Query Model
load params session = Query.succeed { }

view : Model -> Session -> Html Msg
view model session =
  <nav>
    {for f in [All, Active] -> <a href={I { filter: f }}>x</a>}
  </nav>
`,
	})
	runGo(t, files, `package app

import (
	"net/url"
	"strings"
	"testing"

	"alder/rt"
	"alder/rt/html"
)

func TestLinks(t *testing.T) {
	for got, want := range map[string]string{
		rt.PageURL(T__link1):   "/t/a%2Fb%20c/x?filter=Active&q=x%26y&n=2&b=True&tags=p&tags=q+r&m=",
		rt.PageURL(T__link2("c")): "/t/c/x?n=-1",
	} {
		if got != want {
			t.Errorf("PageURL = %q, want %q", got, want)
		}
	}
	u, _ := url.Parse(rt.PageURL(T__link1))
	p, errs := decode__T__Params(map[string]string{"id": "a/b c"}, u.Query())
	if errs != nil || p.Id != "a/b c" || p.Filter.Tag != is__T__Filter__Active || p.Q.Val != "x&y" || p.N != 2 || !p.B || strings.Join(p.Tags, ",") != "p,q r" || p.M.Ok {
		t.Fatalf("decoded link1: %+v %v", p, errs)
	}
	p, errs = decode__T__Params(map[string]string{"id": "c"}, url.Values{"n": {"-1"}})
	if errs != nil || p.Filter.Tag != is__T__Filter__All || p.Q.Ok || p.B || len(p.Tags) != 0 || p.M.Val != 3 {
		t.Fatalf("decoded link2: %+v %v", p, errs)
	}
	if _, errs := decode__T__Params(map[string]string{"id": "c"}, url.Values{"n": {"x"}, "filter": {"Nope"}}); len(errs) != 2 {
		t.Fatalf("bad params: %v", errs)
	}
	// a Bool URL parameter is strict (web.BoolParam): ?b=maybe is an error, not True
	if _, errs := decode__T__Params(map[string]string{"id": "c"}, url.Values{"n": {"1"}, "b": {"maybe"}}); len(errs) != 1 || errs[0].Field != "b" {
		t.Fatalf("b=maybe: %v", errs)
	}
	for raw, want := range map[string]bool{"True": true, "1": true, "false": false} {
		if p, errs := decode__T__Params(map[string]string{"id": "c"}, url.Values{"n": {"1"}, "b": {raw}}); errs != nil || p.B != want {
			t.Fatalf("b=%s: %+v %v", raw, p, errs)
		}
	}
	page := html.Render(I__view(I__Model{}, App__Session{}))
	if !strings.Contains(page, `+"`"+`<a href="/">x</a><a href="/?filter=Active">x</a>`+"`"+`) {
		t.Fatalf("links: %s", page)
	}
}
`)
}

// Per-row forms of one form type (I1, the reviewer's adv3): a Reject (or a form error) shows on
// the row whose form posted the message, not on every form of that type; a plain GET shows the
// state nowhere.
func TestGen_FormStatePerRow(t *testing.T) {
	files := emitSource(t, map[string]string{
		"App.alder": `module App

session { user: Maybe Text }

login = P

error : Error -> Html Never
error err = <html><head><title>E</title></head><body><p>error</p></body></html>

notFound : Html Never
notFound = <html><head><title>NF</title></head><body><p>nf</p></body></html>
`,
		"P.alder": `module P

page "/" { }

type Model = { rows: List Text }

load : Params -> Session -> Query Model
load params session = Query.succeed { rows: ["a", "b", "c"] }

form Rename = { title: Text.NonEmpty }

msg Save { row: Text } form Rename

update : Msg -> Model -> Session -> Task Nav
update msg model session = match msg {
  Save p -> Task.succeed (Reject ("bad " ++ p.row))
}

view : Model -> Session -> Html Msg
view model session = <html><head><title>P</title></head><body>
  {for r in model.rows -> <form submit={Save { row: r }}><Errors form /><Input field="title" label={r} /><Errors field="title" /><button>s</button></form>}
</body></html>
`,
	})
	runGo(t, files, `package app

import (
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRows(t *testing.T) {
	h, err := Serve(Caps{}, Options{DB: ":memory:", Dev: true})
	if err != nil {
		t.Fatal(err)
	}
	post := func(target, body string) string {
		r := httptest.NewRequest("POST", target, strings.NewReader(body))
		r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != 200 {
			t.Fatalf("POST %s: %d", target, w.Code)
		}
		return w.Body.String()
	}
	// the row of each form, then what follows it up to the next form
	rows := func(body string) []string {
		parts := strings.Split(body, "<form ")[1:]
		if len(parts) != 3 {
			t.Fatalf("%d forms:\n%s", len(parts), body)
		}
		return parts
	}
	body := post("/_m/P/Save?row=b", "title=typed")
	for i, row := range rows(body) {
		shows := strings.Contains(row, "<li>bad b</li>") && strings.Contains(row, `+"`"+`value="typed"`+"`"+`)
		if (i == 1) != shows || strings.Count(body, "typed") != 1 {
			t.Fatalf("Reject: row %d shows the state: %v\n%s", i, shows, body)
		}
	}
	body = post("/_m/P/Save?row=c", "title=")
	for i, row := range rows(body) {
		if (i == 2) != strings.Contains(row, "<li>must not be empty</li>") || strings.Count(body, "must not be empty") != 1 {
			t.Fatalf("form error: row %d\n%s", i, body)
		}
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/", nil))
	if strings.Contains(w.Body.String(), "<li>") {
		t.Fatalf("GET shows a state:\n%s", w.Body.String())
	}
}
`)
}

// The re-review's fresh1 probe: a Redirect to `Url.external` of user input cannot run script
// under Datastar, and an `href={Url.external …}` of it is "#".
func TestGen_UrlExternalIsSanitized(t *testing.T) {
	files := emitSource(t, map[string]string{
		"App.alder": appModule,
		"F.alder": `module F

page "/" { q: Text = "" }

type Model = { q: Text }

load : Params -> Session -> Query Model
load params session = Query.succeed { q: params.q }

msg Go { to: Text }

update : Msg -> Model -> Session -> Task Nav
update msg model session = match msg {
  Go p -> Task.succeed (Redirect (Url.external p.to)),
}

view : Model -> Session -> Html Msg
view model session = <p><a id="lnk" href={Url.external model.q}>x</a></p>
`,
	})
	runGo(t, files, `package app

import (
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestProbe(t *testing.T) {
	h, err := Serve(Caps{}, Options{DB: ":memory:", Dev: true})
	if err != nil {
		t.Fatal(err)
	}
	do := func(method, target string, ds bool) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, target, nil)
		if ds {
			r.Header.Set("Datastar-Request", "true")
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w
	}
	evil := url.QueryEscape("javascript:alert(document.cookie)")
	w := do("POST", "/_m/F/Go?to="+evil, true)
	if w.Code != 200 || !strings.Contains(w.Body.String(), `+"`"+`window.location.assign("#")`+"`"+`) || strings.Contains(w.Body.String(), "javascript") {
		t.Fatalf("Datastar Go: %d %s", w.Code, w.Body.String())
	}
	if w := do("POST", "/_m/F/Go?to="+evil, false); w.Code != 303 || w.Header().Get("Location") != "#" {
		t.Fatalf("Go: %d %q", w.Code, w.Header().Get("Location"))
	}
	if w := do("POST", "/_m/F/Go?to="+url.QueryEscape("https://ex.com/x"), false); w.Code != 303 || w.Header().Get("Location") != "https://ex.com/x" {
		t.Fatalf("Go https: %d %q", w.Code, w.Header().Get("Location"))
	}
	if w := do("GET", "/?q="+url.QueryEscape("JavaScript:alert(1)"), false); !strings.Contains(w.Body.String(), `+"`"+`<a id="lnk" href="#">`+"`"+`) {
		t.Fatalf("href: %s", w.Body.String())
	}
}
`)
}

// Every union of the program, page and App Msg unions included, registers its constructor
// names from app__alder.go's init, in declaration order, so rt.Show prints its values.
func TestGen_RegistersUnions(t *testing.T) {
	files := emit(t, "../../testdata/golden/gen_basic")
	contains(t, files[appFile], `rt.RegisterUnion(reflect.TypeOf(tag__M__Shape(0)), []string{"Circle", "Rect"})`)
	runGo(t, files, `package app

import (
	"testing"

	"alder/rt"
)

func TestShow(t *testing.T) {
	for _, c := range []struct {
		v    any
		want string
	}{
		{M__Shape__Rect__(payload__M__Shape__Rect{W: 1, H: 2}), "Rect { w: 1.0, h: 2.0 }"},
		{M__Shape__Circle__(-1.5), "Circle (-1.5)"},
		{rt.Just(M__Shape__Circle__(2)), "Just (Circle 2.0)"},
		{M__Point{X: 1, Y: 0.5}, "{ x: 1.0, y: 0.5 }"},
	} {
		if got := rt.Show(c.v); got != c.want {
			t.Errorf("Show = %s, want %s", got, c.want)
		}
	}
}
`)
	todos := emit(t, "../../examples/todos")
	contains(t, todos[appFile],
		`rt.RegisterUnion(reflect.TypeOf(tag__Todos__Index__Msg(0)), []string{"Submit", "Toggle", "Delete"})`,
		`rt.RegisterUnion(reflect.TypeOf(tag__App__Msg(0)), []string{`)
}

// runTodosShowTest prints todos values with rt.Show: unions from every module file (Todo,
// the page's and App's Msg) print by their constructor names, never as Go.
const runTodosShowTest = `

func TestTodosShow(t *testing.T) {
	due := rt.Just(rt.Date{Y: 2026, M: 10, D: 9})
	for _, c := range []struct {
		v    any
		want string
	}{
		{Todos__Index__Msg__Toggle__(Todos__Index__ToggleParams{Id: "x"}), "Toggle { id: \"x\" }"},
		{Todos__Index__Msg__Submit__(Todos__Index__SubmitParams{Form: Todos__Index__NewTodo{Title: "Milk", Due: due}}),
			"Submit { form: { title: \"Milk\", due: Just 2026-10-09 } }"},
		{App__Msg__Logout__, "Logout"},
		{App__Session{User: rt.Just("u1")}, "{ user: Just \"u1\" }"},
		{Todos__Index__Model{Filter: Todo__Filter__Active__, Todos: []Todo__Todo{{Id: "1", Title: "a", Created: time.Unix(0, 0)}}},
			"{ filter: Active, todos: [{ id: \"1\", title: \"a\", done: False, due: Nothing, owner: \"\", created: 1970-01-01T00:00:00Z }] }"},
	} {
		got := rt.Show(c.v)
		if got != c.want {
			t.Errorf("Show = %s, want %s", got, c.want)
		}
		for _, bad := range []string{"{Ok:", "*rt.", "0x", "Tag:"} {
			if strings.Contains(got, bad) {
				t.Errorf("Show = %s: Go syntax %q", got, bad)
			}
		}
	}
}
`

// A record field whose Alder name starts with '_' is the exported Go field U__<rest> (Field), in
// every place a field may be: a record (rt.Show, rt.Eq and rt.Key see it), App's session (its
// cookie codec), a page parameter, a message parameter, a form and a table row.
func TestGen_RunUnderscoreFields(t *testing.T) {
	if got := Field("_at"); got != "U__at" {
		t.Errorf("Field(_at) = %s", got)
	}
	if got := Field("title"); got != "Title" {
		t.Errorf("Field(title) = %s", got)
	}
	files := emitSource(t, map[string]string{
		"App.alder": `module App

session { _n: Int = 0, _seen: Maybe Time }

error : Error -> Html Never
error err = <p>error</p>

notFound : Html Never
notFound = <p>not found</p>
`,
		"Row.alder": `module Row

type RowId = brand Text

table Row {
  id: RowId @id,
  _note: Text,
  _at: Time @created,
}
`,
		"Home.alder": `module Home

import Row

page "/home" { _q: Int = 0 }

form NewRow = { _note: Text }

msg Add form NewRow
msg Pick { _k: Int }

type Model = { _rows: List Row.Row, _h: Html Never, _d: Dict Text Time }

load : Params -> Session -> Query Model
load params session = {
  let rows <- Db.from Row |> Db.orderBy Row._at.asc |> Db.all
  Query.succeed { _rows: rows, _h: <b>{Text.fromInt params._q}</b>, _d: Dict.empty }
}

view : Model -> Session -> Html Msg
view model session =
  <div>
    {model._h}
    <ul>{for r in model._rows -> <li>{r._note}</li>}</ul>
    <form submit={Add}>
      <Input field="_note" label="Note" />
      <Errors field="_note" />
    </form>
    <button click={Pick { _k: 7 }}>pick</button>
  </div>

update : Msg -> Model -> Session -> Task Nav
update msg model session = match msg {
  Add p -> {
    let _ <- Db.insert Row { _note: p.form._note }
    Task.succeed Rerender
  },
  Pick p -> {
    let _ <- Session.put (\s -> { s | _n: s._n + p._k })
    Task.succeed (Redirect (Home { _q: p._k }))
  },
}
`,
	})
	contains(t, files["Home_alder.go"], "U__note string", "U__q int64")
	contains(t, files["Row_alder.go"], "U__note ", "U__at ", "r.U__note, err = db.DecodeText")
	runGo(t, files, `package app

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"alder/rt"
	"alder/rt/html"
)

func TestUnderscore(t *testing.T) {
	tm := time.Date(2026, 10, 9, 0, 0, 0, 0, time.UTC)
	m := Home__Model{U__h: html.El("b", nil, html.Text("x")), U__d: rt.Dict[string, time.Time]{}.Insert("k", tm)}
	if got, want := rt.Show(m), "{ _rows: [], _h: <b>x</b>, _d: Dict.fromList [{ key: \"k\", value: 2026-10-09T00:00:00Z }] }"; got != want {
		t.Errorf("Show = %s, want %s", got, want)
	}
	other := Home__Model{U__d: rt.Dict[string, time.Time]{}.Insert("k", tm.In(time.FixedZone("x", 3600)))}
	if !rt.Eq(Home__Model{U__d: m.U__d}, other) || rt.Key(Home__Model{U__d: m.U__d}) != rt.Key(other) {
		t.Error("Eq/Key do not see the _d field's value")
	}
	// the session cookie keeps _n
	b, err := (session__codec{}).Encode(App__Session{U__n: 3})
	if err != nil {
		t.Fatal(err)
	}
	if s, err := (session__codec{}).Decode(b); err != nil || s.(App__Session).U__n != 3 {
		t.Fatalf("session codec: %s → %+v, %v", b, s, err)
	}

	h, err := Serve(Caps{}, Options{DB: ":memory:", Dev: true})
	if err != nil {
		t.Fatal(err)
	}
	var cookie string
	do := func(method, target, body string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, target, strings.NewReader(body))
		r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		if cookie != "" {
			r.Header.Set("Cookie", cookie)
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if c := w.Header().Get("Set-Cookie"); c != "" {
			cookie, _, _ = strings.Cut(c, ";")
		}
		return w
	}
	// a page parameter
	if w := do("GET", "/home?_q=3", ""); w.Code != 200 || !strings.Contains(w.Body.String(), "<b>3</b>") {
		t.Fatalf("GET: %d %s", w.Code, w.Body.String())
	}
	// a form field, into a table row and back
	if w := do("POST", "/_m/Home/Add", "_note=milk"); w.Code != http.StatusSeeOther {
		t.Fatalf("Add: %d %s", w.Code, w.Body.String())
	}
	if w := do("GET", "/home", ""); !strings.Contains(w.Body.String(), "<li>milk</li>") {
		t.Fatalf("the row: %s", w.Body.String())
	}
	// a message parameter, and the session
	if w := do("POST", "/_m/Home/Pick?_k=7", ""); w.Code != http.StatusSeeOther || w.Header().Get("Location") != "/home?_q=7" {
		t.Fatalf("Pick: %d %q", w.Code, w.Header().Get("Location"))
	}
	if cookie == "" {
		t.Fatal("Pick stored no session")
	}
}
`)
}

// TestGen_Tests: an app's `test` declarations are emitted as test__M__N functions and the Tests
// table of app__alder.go (with TestSession and Registry), and run through rtesting.Run they
// compute the right outcomes: a pure test's recorded `==` sides, Test.run undoing a failed
// task's writes inside the test's transaction, its trace and Test.trace
// (testdata/e2e_tests_test.go.txt).
func TestGen_Tests(t *testing.T) {
	files := emitSource(t, map[string]string{
		"App.alder": `module App

session { n: Int = 0 }

error : Error -> Html Never
error err = <html><body><p>error</p></body></html>

notFound : Html Never
notFound = <html><body><p>nf</p></body></html>
`,
		"Shop.alder": `module Shop

total : List Int -> Int
total xs = List.sum xs

test "total adds" = total [1, 2] == 3

test "total is wrong" = total [1, 2] == 4
`,
		"Item.alder": `module Item

type ItemId = brand Text

table Item {
  id: ItemId @id,
  name: Text,
}
`,
		"Shop/Orders.alder": `module Shop.Orders

import Item

addThenFail : Task Int
addThenFail = {
  let _ <- Db.insert Item { name: "gone" }
  Task.fail "boom"
}

test "a failed run undoes its writes" = {
  let _ <- Db.insert Item { name: "kept" }
  let r <- Test.run addThenFail
  let tr <- Test.trace
  let n <- Db.from Item |> Db.count
  Task.succeed ({ value: r.value, trace: r.trace, steps: tr, n: n } == { value: Err "boom", trace: ["Db.insert Item"], steps: ["Db.insert Item", "Test.run", "Db.insert Item"], n: 1 })
}

test "a successful run keeps its insert" = {
  let r <- Test.run (Db.insert Item { name: "x" })
  let n <- Db.from Item |> Db.count
  Task.succeed (n == 1)
}
`,
	})
	contains(t, files["Shop_alder.go"], "func test__Shop__1() bool {", "rtesting.Eq(", "func test__Shop__2() bool {")
	contains(t, files["Shop__Orders_alder.go"], "func test__Shop__Orders__1() eff.Task[bool] {", "func test__Shop__Orders__2() eff.Task[bool] {",
		`eff.Call("Test.run", true,`, "rtesting.Inner(", `eff.Call("Test.trace", false,`, "rtesting.Trace(")
	contains(t, files["app__alder.go"], "var Tests = []rtesting.Case{\n", `{Module: "Shop", Name: "total adds", Pure: test__Shop__1},`,
		`{Module: "Shop.Orders", Name: "a failed run undoes its writes", Task: test__Shop__Orders__1}`,
		"func TestSession() any {", "func (Caps) Registry() map[string]any {", "reg := caps.Registry()")
	prog, err := os.ReadFile("testdata/e2e_tests_test.go.txt")
	if err != nil {
		t.Fatal(err)
	}
	runGo(t, files, string(prog))
}

// A @ref column's delete rule reaches its db.Column (@restrict, the default, writes nothing), and
// `.startsWith` becomes db.CStartsWith.
func TestGen_DeleteRulesAndStartsWith(t *testing.T) {
	files := emitSource(t, map[string]string{
		"Post.alder": `module Post

type PostId = brand Text

table Post {
  id: PostId @id,
  title: Text,
}

titled : Text -> Query (List Post)
titled p = Db.from Post |> Db.where (Post.title.startsWith p) |> Db.all
`,
		"Comment.alder": `module Comment

import Post

type CommentId = brand Text

table Comment {
  id: CommentId @id,
  post: Post.PostId @ref Post @cascade,
  reply: Maybe Post.PostId @ref Post @setNull,
  quote: Post.PostId @ref Post @restrict,
}
`})
	contains(t, files["Comment_alder.go"],
		`{Name: "post", Type: db.CText, Ref: "Post.Post", OnDelete: db.Cascade},`,
		`{Name: "reply", Type: db.CText, Nullable: true, Ref: "Post.Post", OnDelete: db.SetNull},`,
		`{Name: "quote", Type: db.CText, Ref: "Post.Post"},`,
	)
	contains(t, files["Post_alder.go"], `db.CStartsWith("title", p)`)
	vet(t, files)
}
