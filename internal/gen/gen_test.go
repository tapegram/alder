package gen

import (
	"flag"
	"fmt"
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
func emit(t *testing.T, dir string) map[string][]byte {
	t.Helper()
	files, err := Emit(lowerDir(t, dir), Options{RepoPath: repoPath(t)})
	if err != nil {
		t.Fatal(err)
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
		if err := os.WriteFile(filepath.Join(dir, name), b, 0o644); err != nil {
			t.Fatal(err)
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
		if st, err := os.Stat(dir); err != nil || !st.IsDir() {
			continue
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
	want := "module app\n\ngo 1.27\n\nrequire alder v0.0.0\n\nreplace alder => /r/alder\n"
	if got := string(files["go.mod"]); got != want {
		t.Fatalf("go.mod:\n%s\nwant:\n%s", got, want)
	}
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
	src := files["U.go"]
	contains(t, src,
		"type U__Shape_Rect struct {\n\tW float64\n\tH float64\n}",
		"type U__Shape_Circle struct {\n\tValue float64\n}",
		"Rect   U__Shape_Rect",
		"func U__Shape_Circle_(v float64) U__Shape {",
		"func U__Shape_Rect_(v U__Shape_Rect) U__Shape {",
		"var U__Shape_Empty = U__Shape{Tag: U__Shape_Tag_Empty}",
		"U__Shape_Rect_(U__Shape_Rect{W: 1.0, H: 2.0})",
		"type U__Tree[A any] struct {",
		"type U__Tree_Node[A any] struct {\n\tValue A\n\tSize  int64\n}",
		"func U__Tree_Node_[A any](v U__Tree_Node[A]) U__Tree[A] {",
		"U__Tree[string]{Tag: U__Tree_Tag_Leaf}",
		"func U__sizeOf[A any](t U__Tree[A]) int64 {",
		"U__Tree_Node_[string](U__Tree_Node[string]{Value: v, Size: 1})",
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
	src := files["K.go"]
	contains(t, src,
		"Tag_ K__Kind_Tag",
		"Type string",
		"func K__func(k K__Kind) string {",
		"case K__Kind_Tag_Tag:",
		"type_ := k.Func.Type",
		"return type_",
		"func K__go(range_ int64) int64 {",
		"var_ := range_ + 1",
		"rt_ := var_ * 2",
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
	contains(t, files["S.go"],
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
	src := string(files["T.go"])
	if strings.Contains(src, "import") {
		t.Errorf("types-only module imports something:\n%s", src)
	}
	if !strings.HasPrefix(src, "// Code generated by alder; DO NOT EDIT.\n\npackage app\n") {
		t.Errorf("bad header:\n%s", src)
	}
	contains(t, files["T.go"], "type T__Pair struct {", "var T__Color_Red = T__Color{Tag: T__Color_Tag_Red}")
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
		{&types.Record{Module: "Todos.Index", Name: "Model"}, "Todos_Index__Model"},
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
		{"Todos.Index", "view", "Todos_Index__view"},
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
	contains(t, files["E.go"],
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
		"func E__keysOf[K comparable, V any](d rt.Dict[K, V]) []K {",
		"n_2 := n + 1",
		"return n_2 * 2",
	)
	vet(t, files)
}
