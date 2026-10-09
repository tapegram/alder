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
	src := files["U.go"]
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
	src := files["K.go"]
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
	contains(t, files["T.go"], "type T__Pair struct {", "var T__Color__Red__ = T__Color{Tag: is__T__Color__Red}")
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
	contains(t, files["O.go"],
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
	contains(t, files["N.go"], "type tag__N__Shape uint8", "type N__ShapeTag struct {", "is__N__Shape__Tag tag__N__Shape = 1")
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
	contains(t, files["C.go"],
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
	src := string(files["H.go"])
	contains(t, files["H.go"], `html.Text("a < b")`, `html.Text(" & é")`, `html.A("title", "x & y")`)
	for _, bad := range []string{"&lt;", "&amp;"} {
		if strings.Contains(src, bad) {
			t.Errorf("emitted Go pre-escapes text (%s):\n%s", bad, src)
		}
	}
	vet(t, files)
}

// Every pure stdlib signature has an entry in StdCalls; every effectful one is in Effectful.
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
	contains(t, files["P.go"],
		"switch _m1.Tag {",
		`return rt.MsgURL("P", "Pick", rt.KV{Key: "name", Value: _m1.Pick.Name}, rt.KV{Key: "at", Value: rt.MaybeText(_m1.Pick.At, std.TextFromInt)})`,
		`return rt.MsgURL("P", "Clear")`,
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
	contains(t, files["M.go"],
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
	contains(t, files["M.go"],
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
	src := string(files["D.go"])
	if strings.Contains(src, "comparable") {
		t.Errorf("generated code constrains a type parameter:\n%s", src)
	}
	contains(t, files["D.go"], "var D__byPath rt.Dict[[]int64, string] =", "func D__single[K any](k K, n int64) rt.Dict[K, int64] {")
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
	contains(t, files["T.go"], "Node *payload__T__Tree__Node[A]", "Add *payload__T__Expr__Add")
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
	contains(t, files["Home.go"],
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
	contains(t, files["M.go"], "B   *M__Box[int64]", "Boxed *M__Box[A]", "Link *M__Node[A]")
	runGo(t, files, `package app

import "testing"

func TestPayload(t *testing.T) {
	if M__got != 7 || M__boxedText != "t" || M__plain != 4 || M__chain != 2 {
		t.Errorf("got, boxedText, plain, chain = %d, %q, %d, %d", M__got, M__boxedText, M__plain, M__chain)
	}
}
`)
}
