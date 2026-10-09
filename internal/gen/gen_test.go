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

// Every stdlib signature has an entry in StdCalls or (Query.*, Task.*, Db.*, Form.values) in Effectful.
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
	contains(t, files["P_alder.go"],
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
		"type cap__Todo__Stats interface {",
		"Total(ctx context.Context) (int64, error)",
		"ForOwner(ctx context.Context, a1 string, a2 bool) (int64, error)",
	)
	contains(t, files["Index_alder.go"],
		"Send(ctx context.Context, args struct {",
		`.Caps["Index.Mailer"].(cap__Index__Mailer)`,
		`panic("alder: capability \"Index.Mailer\" is not wired")`,
		".Send(_e5.Ctx, struct {",
	)
	contains(t, files["app__alder.go"], "var Tables = []db.TableDesc{table__Todo__Todo.Desc()}",
		`var Capabilities = []string{"Todo.Stats", "Index.Mailer"}`)
}

// TestGen_RunTodos runs the todos example end to end: its generated load and update functions,
// built as effect trees, run with eff.Run against an in-memory SQLite store created from the
// generated table descriptors, with a stub for the Auth.Login.Password capability.
func TestGen_RunTodos(t *testing.T) {
	dir := emitApp(t, "../../examples/todos")
	if err := os.WriteFile(filepath.Join(dir, "zz_run_test.go"), []byte(runTodosTest), 0o644); err != nil {
		t.Fatal(err)
	}
	goRun(t, dir, "test", "-count=1", "-v", "-run", "TestTodos", "./...")
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

func (p password) Verify(ctx context.Context, args struct {
	Email    string
	Password string
}) (rt.Maybe[string], error) {
	if args.Email == "a@b.c" && args.Password == "pw" {
		return rt.Just(p.user), nil
	}
	return rt.Nothing[string](), nil
}

var _ cap__Auth__Login__Password = password{}

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
	env.Session = App__Session{User: rt.Nothing[string](), Theme: Theme__Theme__Light__}
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
	session := Todos__Index__Session{User: app.User.Val, Theme: app.Theme}
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
	calls("toggle", "Db.update Todo")
	if todos = load(Todo__Filter__All__); len(todos) != 1 || !todos[0].Done {
		t.Fatalf("after toggle: %+v", todos)
	}
	if n := len(load(Todo__Filter__Active__)); n != 0 {
		t.Fatalf("active after toggle: %d", n)
	}

	// the show page loads the user's own todo, and not one that does not exist
	mark = len(env.Trace)
	shown, err := eff.RunQuery(env, Todos__Show__load(Todos__Show__Params{Id: td.Id}, Todos__Show__Session{User: uid, Theme: app.Theme}))
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

	// toggling a todo that does not exist fails the Task
	if _, err := update(Todos__Index__Msg__Toggle__(Todos__Index__ToggleParams{Id: "nope"})); err == nil {
		t.Fatal("toggle missing: no error")
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
	calls("delete", "Db.delete Todo")
	if n := len(load(Todo__Filter__All__)); n != 0 {
		t.Fatalf("after delete: %d todos", n)
	}
	calls("load", "Db.all Todo")

	// App's update: switch the theme, then log out
	if nav, err := eff.Run(env, eff.KTask, App__update(App__Msg__SwitchTheme__, env.Session.(App__Session))); err != nil || nav.Tag != rt.NavRerender {
		t.Fatalf("switch theme: %+v, %v", nav, err)
	}
	if s := env.Session.(App__Session); s.Theme.Tag != is__Theme__Theme__Dark {
		t.Fatalf("theme not switched: %+v", s)
	}
	if nav, err := eff.Run(env, eff.KTask, App__update(App__Msg__Logout__, env.Session.(App__Session))); err != nil || nav.Tag != rt.NavRedirect || nav.Page.Path != "/login" {
		t.Fatalf("logout: %+v, %v", nav, err)
	}
	if s := env.Session.(App__Session); s.User.Ok {
		t.Fatalf("logout kept the user: %+v", s)
	}
	t.Logf("trace: %v", env.Trace)
	t.Logf("log: %d lines", len(logs))
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
	"testing"
	"time"

	"alder/rt"
	"alder/rt/db"
	"alder/rt/eff"
)

type pay struct{}

func (pay) Charge(ctx context.Context, args struct {
	Amount float64
	Card   string
}) (rt.Maybe[string], error) {
	return rt.Just(args.Card), nil
}
func (pay) Balance(ctx context.Context) (float64, error)                { return 2.5, nil }
func (pay) Limit(ctx context.Context, a1 string, a2 int64) (int64, error) { return a2 * 10, nil }

var _ cap__Shop__Pay = pay{}

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
	open, paid, shipped := Shop__Status__Open__, Shop__Status__Paid__, Shop__Status__Shipped__
	add := func(code string, st Shop__Status, late rt.Maybe[Shop__Status], qty int64, gift bool, note rt.Maybe[string]) {
		t.Helper()
		_, err := eff.Run(env, eff.KTask, eff.Call("add", true, func(e *eff.Env) (string, error) {
			return db.InsertRow(e.Ctx, e.Store.(db.Store), db.TxOf(e), table__Shop__Order, insertvals__Shop__Order(insert__Shop__Order{
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
	add("x5", open, rt.Nothing[Shop__Status](), 1, true, rt.Nothing[string]())

	all := func(s db.Select[Shop__Order]) []Shop__Order {
		t.Helper()
		rows, err := eff.RunQuery(env, eff.Q(eff.Call("all", false, func(e *eff.Env) ([]Shop__Order, error) {
			return db.All(e.Ctx, e.Store.(db.Store), db.TxOf(e), s)
		})))
		if err != nil {
			t.Fatal(err)
		}
		return rows
	}
	later := clock.Add(time.Hour)
	if got := all(Shop__search(open, day, later, []Shop__Status{open, paid})); len(got) != 1 || got[0].Code != "x1" || got[0].Late.Val.Tag != is__Shop__Status__Open {
		t.Errorf("search open: %+v", got)
	}
	if got := all(Shop__search(paid, day, later, []Shop__Status{paid})); len(got) != 1 || got[0].Code != "x2" {
		t.Errorf("search paid: %+v", got)
	}
	if got := all(Shop__search(shipped, day, later, []Shop__Status{shipped})); len(got) != 0 {
		t.Errorf("search shipped (not < Shipped): %+v", got)
	}
	if got := all(Shop__search(open, day, clock, []Shop__Status{open})); len(got) != 0 {
		t.Errorf("search created before the clock: %+v", got)
	}
	if got := all(Shop__either(nil)); len(got) != 0 {
		t.Errorf("Db.or [] matched %d rows", len(got))
	}
	if got := all(Shop__either([]db.Cond{Shop__conds(nil)})); len(got) != 0 {
		t.Errorf("Db.not (Db.and []) matched %d rows", len(got))
	}
	if got := all(db.Where(Shop__conds(nil), db.From(table__Shop__Order))); len(got) != 5 {
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
