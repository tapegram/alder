package gen

import (
	"bytes"
	"flag"
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"alder/internal/check"
	"alder/internal/diag"
	"alder/internal/ir"
	"alder/internal/mutate"
	"alder/internal/names"
	"alder/internal/syntax"
	"alder/internal/types"
	"alder/rt"
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

// writeRuntime writes the embedded runtime into dir/rt as buildkit.WriteRuntime does (gen's
// tests cannot import buildkit, which imports gen): the module files as rt/go.mod and rt/go.sum,
// the Go files under rt/rt/, and dir/go.sum. TestWriteRuntime (buildkit) tests the real one.
func writeRuntime(t *testing.T, dir string) {
	t.Helper()
	err := fs.WalkDir(rt.Files, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		b, err := rt.Files.ReadFile(path)
		if err != nil {
			return err
		}
		switch path {
		case "dist/go.mod.txt":
			path = "go.mod"
		case "dist/go.sum.txt":
			path = "go.sum"
			if err := os.WriteFile(filepath.Join(dir, "go.sum"), b, 0o644); err != nil {
				return err
			}
		default:
			path = "rt/" + path
		}
		dst := filepath.Join(dir, "rt", filepath.FromSlash(path))
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return err
		}
		return os.WriteFile(dst, b, 0o644)
	})
	if err != nil {
		t.Fatal(err)
	}
}

// emit lowers dir and emits it.
// An app with a migrations/ directory is emitted with Options.Migrations, and its *.sql files
// are added to the result as "migrations/<file>" (what buildkit.CopyMigrations copies), so the
// emitted module builds.
func emit(t *testing.T, dir string) map[string][]byte {
	t.Helper()
	return emitWith(t, dir, false)
}

// emitWith is emit, a test build (Options.Tests) when tests is set.
func emitWith(t *testing.T, dir string, tests bool) map[string][]byte {
	t.Helper()
	return emitOpts(t, dir, Options{Tests: tests})
}

// emitOpts is emit with o (Migrations filled in).
func emitOpts(t *testing.T, dir string, o Options) map[string][]byte {
	t.Helper()
	sqls, _ := filepath.Glob(filepath.Join(dir, "migrations", "*.sql"))
	o.Migrations = len(sqls) > 0
	files, err := Emit(lowerDir(t, dir), o)
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
	return emitSourceWith(t, srcs, false)
}

// emitSourceWith is emitSource, a test build (Options.Tests) when tests is set.
func emitSourceWith(t *testing.T, srcs map[string]string, tests bool) map[string][]byte {
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
	return emitWith(t, dir, tests)
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
	writeAdapterStubs(t, dir, files)
	writeRuntime(t, dir)
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

// capsCall is a line of the generated caps package calling an adapter's constructor, and
// capsImport one importing an adapter package.
var (
	capsCall   = regexp.MustCompile(`(?m)^\t+(\w+):\s+(a_\w+)\.(\w+)\(\),$`)
	capsImport = regexp.MustCompile(`(?m)^\t(a_\w+) "app/adapters/(\w+)"$`)
)

// writeAdapterStubs writes, into the emitted module in dir, a stub of every adapter package
// and constructor the generated caps package (CapsFile) calls, each returning a nil interface
// of the capability's type, so that `go vet ./...` compiles the caps package too.
func writeAdapterStubs(t *testing.T, dir string, files map[string][]byte) {
	t.Helper()
	src, ok := files[CapsFile]
	if !ok {
		return
	}
	pkgs := map[string]string{} // alias → directory
	for _, m := range capsImport.FindAllStringSubmatch(string(src), -1) {
		pkgs[m[1]] = m[2]
	}
	stubs := map[string]*strings.Builder{}
	for _, m := range capsCall.FindAllStringSubmatch(string(src), -1) {
		d, ok := pkgs[m[2]]
		if !ok {
			t.Fatalf("%s calls %s.%s, which it does not import:\n%s", CapsFile, m[2], m[3], src)
		}
		if stubs[d] == nil {
			stubs[d] = &strings.Builder{}
			fmt.Fprintf(stubs[d], "package %s\n\nimport \"app\"\n\n", d)
		}
		fmt.Fprintf(stubs[d], "func %s() app.Cap__%s { return nil }\n\n", m[3], m[1])
	}
	if len(stubs) != len(pkgs) {
		t.Fatalf("%s imports %d adapter packages but calls into %d:\n%s", CapsFile, len(pkgs), len(stubs), src)
	}
	for d, b := range stubs {
		if err := os.MkdirAll(filepath.Join(dir, "adapters", d), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "adapters", d, "stub.go"), []byte(b.String()), 0o644); err != nil {
			t.Fatal(err)
		}
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

// testBuildGoldens are the gen goldens whose want/ is a test build (Options.Tests) and whose
// want_prod/ is the production build of the same app.
var testBuildGoldens = map[string]bool{"gen_fakes": true, "gen_browser": true, "gen_fuzz": true, "gen_wire": true, "gen_helpers": true, "gen_route_handlers": true}

// mutateBuildGoldens are the gen goldens whose want/ is a mutate build (Options.Mutate, a test
// build) and whose want_prod/ is the production build of the same app.
var mutateBuildGoldens = map[string]bool{"gen_mutate": true, "gen_rules": true, "gen_silos": true}

func TestGenGolden(t *testing.T) {
	dirs, _ := filepath.Glob("../../testdata/golden/gen_*")
	n := 0
	for _, dir := range dirs {
		if st, err := os.Stat(filepath.Join(dir, "app")); err != nil || !st.IsDir() {
			continue // not an app (gen_vet holds a corpus of apps: TestGenVetCorpus)
		}
		n++
		type variant struct {
			want string
			opts Options
		}
		variants := []variant{{"want", Options{}}}
		if testBuildGoldens[filepath.Base(dir)] {
			variants = []variant{{"want", Options{Tests: true}}, {"want_prod", Options{}}}
		}
		if mutateBuildGoldens[filepath.Base(dir)] {
			variants = []variant{{"want", Options{Tests: true, Mutate: true}}, {"want_prod", Options{}}}
		}
		for _, v := range variants {
			t.Run(filepath.Base(dir)+"/"+v.want, func(t *testing.T) {
				goldenCompare(t, emitOpts(t, dir, v.opts), filepath.Join(dir, v.want))
			})
		}
	}
	if n == 0 {
		t.Fatal("no gen golden directories")
	}
}

// goldenCompare compares the emitted .go files with want/ (rewriting it with -update) and vets
// them.
func goldenCompare(t *testing.T, files map[string][]byte, want string) {
	t.Helper()
	if *update {
		os.RemoveAll(want)
		if err := os.MkdirAll(want, 0o755); err != nil {
			t.Fatal(err)
		}
		for name, b := range files {
			if strings.HasSuffix(name, ".go") {
				if err := os.MkdirAll(filepath.Dir(filepath.Join(want, name)), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(want, name), b, 0o644); err != nil {
					t.Fatal(err)
				}
			}
		}
	}
	var wantNames, gotNames []string
	filepath.WalkDir(want, func(path string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() && strings.HasSuffix(path, ".go") {
			rel, _ := filepath.Rel(want, path)
			wantNames = append(wantNames, filepath.ToSlash(rel))
		}
		return nil
	})
	slices.Sort(wantNames)
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
}

// The generated go.mod requires the runtime from rt/ next to it (buildkit.WriteRuntime): a
// relative path, so nothing generated names a checkout and the build directory can move.
func TestGen_GoMod(t *testing.T) {
	files, err := Emit(&ir.Program{}, Options{})
	if err != nil {
		t.Fatal(err)
	}
	want := "module app\n\ngo 1.27\n\nrequire alder v0.0.0\n\nreplace alder => ./rt\n"
	if got := string(files["go.mod"]); got != want {
		t.Fatalf("go.mod:\n%s\nwant:\n%s", got, want)
	}
}

// RF2: app__alder.go's header names the compiler's version when Emit is given one, and only
// app__alder.go's; without one (the goldens) it is the version-free header.
func TestGen_HeaderVersion(t *testing.T) {
	files, err := Emit(&ir.Program{}, Options{Version: "devel+71c46e8abcde"})
	if err != nil {
		t.Fatal(err)
	}
	if got, want := string(files[appFile]), "// Code generated by alder devel+71c46e8abcde; DO NOT EDIT.\n\npackage app\n"; !strings.HasPrefix(got, want) {
		t.Errorf("app__alder.go starts\n%.80s\nwant\n%s", got, want)
	}
	files, err = Emit(&ir.Program{}, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if got, want := string(files[appFile]), "// Code generated by alder; DO NOT EDIT.\n"; !strings.HasPrefix(got, want) {
		t.Errorf("app__alder.go starts\n%.80s\nwant\n%s", got, want)
	}
}

// A build directory whose path has a space builds: the replace is relative (it was an
// absolute, quoted path before M10).
func TestGen_BuildDirWithSpace(t *testing.T) {
	files := emit(t, "../../testdata/golden/gen_basic")
	dir := filepath.Join(t.TempDir(), "my build")
	for name, b := range files {
		if err := os.MkdirAll(filepath.Dir(filepath.Join(dir, name)), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, name), b, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	writeRuntime(t, dir)
	cmd := exec.Command("go", "vet", "-mod=mod", "./...")
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GOWORK=off", "GOFLAGS=-mod=mod", "GOPROXY=off", "GOTOOLCHAIN=local")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("go vet in %q: %v\n%s", dir, err, out)
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

// An `<al-button href>` writes the attributes that describe its link on the `<a>` it renders,
// next to `href`: the author's `data-alder-reload` (the `Edit` and disabled buttons) and the one
// the compiler adds for a link to a file page (`Export`). The `<al-button>` itself keeps only the
// rest, so no `data-alder-reload` is written on it.
func TestGen_AlButtonAnchorAttrs(t *testing.T) {
	files := emit(t, "../../testdata/corpus/al-button-anchor-attrs-ok")
	src := string(files["Index_alder.go"])
	if src == "" {
		t.Fatalf("no Index_alder.go among %v", mapsKeys(files))
	}
	const reload = `html.A("data-alder-reload", "true")`
	// Edit, Off, Both and Bare write their own; Export gets the compiler's. A link that has
	// its own is not given a second one, on an `<a>` or an `<al-button>`.
	if n := strings.Count(src, reload); n != 5 {
		t.Errorf("%d %s, want 5 (Edit, Off, Export, Both, Bare):\n%s", n, reload, src)
	}
	if strings.Contains(src, reload+", "+reload) {
		t.Errorf("a link has data-alder-reload twice:\n%s", src)
	}
	if n := strings.Count(src, `html.M("target", model.T)`); n != 2 {
		t.Errorf("%d html.M(\"target\", model.T), want 2 (both branches of the disabled `if`):\n%s", n, src)
	}
	contains(t, []byte(src),
		`html.El("a", []html.Attr{html.A("href", rt.PageURL(`,
		`html.A("target", "_blank"), html.A("rel", "noopener"), `+reload+`, html.A("download", "a.txt")`,
		`html.El("a", []html.Attr{html.A("aria-disabled", "true"), html.A("target", "_blank"), `+reload+`}`,
	)
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
		{types.Page, "rt.Route"}, // Page is Route until PR 9 of #160
		{types.Bytes, "[]byte"},
		{types.File, "rt.File"},
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
// routes and (stubbed) HTML: their emitted Go must vet.
func TestGen_VetIRGoldens(t *testing.T) {
	for _, dir := range []string{"ir_patterns", "ir_route_app", "../../examples/todos"} {
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

note : Html
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
			if key == "Fuzz.forAll" || key == "Fuzz.forAllTask" {
				continue // a property's body, emitted by gen's test (Fuzz.any is no declaration)
			}
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
	testing, err := filepath.Glob("../../rt/testing/*.go") // Test's queries: rtesting (test builds)
	if err != nil {
		t.Fatal(err)
	}
	fuzz, err := filepath.Glob("../../rt/fuzz/*.go") // Fuzz.*
	if err != nil {
		t.Fatal(err)
	}
	funcs := map[string]bool{}
	for _, file := range append(append(pkgs, testing...), fuzz...) {
		if strings.HasSuffix(file, "_test.go") {
			continue
		}
		f, err := parser.ParseFile(fset, file, nil, parser.SkipObjectResolution)
		if err != nil {
			t.Fatal(err)
		}
		for _, d := range f.Decls {
			if fd, ok := d.(*ast.FuncDecl); ok && fd.Recv == nil {
				pkg := "std."
				if strings.Contains(file, "rt/testing/") {
					pkg = "rtesting."
				}
				if strings.Contains(file, "rt/fuzz/") {
					pkg = "fuzz."
				}
				funcs[pkg+fd.Name.Name] = true
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
	fuzz, err := filepath.Glob("../../rt/fuzz/*.go")
	if err != nil {
		t.Fatal(err)
	}
	tparams := map[string][]string{}
	for _, file := range append(files, fuzz...) {
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
			pkg := "std."
			if strings.Contains(file, "rt/fuzz/") {
				pkg = "fuzz."
			}
			tparams[pkg+fd.Name.Name] = names
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

// A route passed as a value is clicked; a generic component takes its props at the instance's
// types; a Let around HTML is used.
func TestGen_HTMLDynamic(t *testing.T) {
	files := emitSource(t, map[string]string{
		"App.alder": `module App

session { }

error : Error -> Html
error err = <p>error</p>

notFound : Html
notFound = <p>not found</p>
`,
		"P.alder": `module P

route P     = GET  "/p"       { }
route Pick  = POST "/p/pick"  { name: Text, at: Maybe Int }
route Clear = POST "/p/clear" { }

type Model = { names: List Text }

P.load : P.Params -> Session -> Query Model
P.load params session = Query.succeed { names: [] }

Pick.update : Pick.Params -> Session -> Task Response
Pick.update params session = Task.succeed Response.refresh

Clear.update : Clear.Params -> Session -> Task Response
Clear.update params session = Task.succeed Response.refresh

Btn : { to: Route, label: Text } -> Html
Btn props = <button click={props.to}>{props.label}</button>

Each : { items: List a, render: a -> Html } -> Html
Each props = <ul>{List.map props.render props.items}</ul>

P.view : Model -> Session -> Html
P.view model session = {
  let first = List.head model.names ?? "none"
  <div>
    <Btn to={Clear} label={first} />
    <Each items={model.names} render={\n -> <li>{n}</li>} />
  </div>
}
`})
	contains(t, files["P_alder.go"],
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
		if err := os.MkdirAll(filepath.Dir(filepath.Join(dir, name)), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, name), b, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	writeAdapterStubs(t, dir, files)
	writeRuntime(t, dir)
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

// A route that `requires` narrows its Session as a page did, and widens it to App's the same way
// where shared code expects App's (#160: a page migrated to routes keeps its helpers).
func TestGen_RunRouteSessionWiden(t *testing.T) {
	files := emitSource(t, map[string]string{
		"App.alder": `module App

session { user: Maybe Text, n: Int = 0 }

login = Login.Login

error : Error -> Html
error err = <p>error</p>

notFound : Html
notFound = <p>nf</p>
`,
		"Login.alder": `module Login

route Login = GET "/login" { }

type Model = { }

Login.load : Login.Params -> Session -> Query Model
Login.load params session = Query.succeed { }

Login.view : Model -> Session -> Html
Login.view model session = <p>log in</p>
`,
		"Ui.alder": `module Ui

who : Session -> Text
who s = match s.user {
  Just u -> "user " ++ u,
  Nothing -> "anon",
}
`,
		"Home.alder": `module Home

import Ui

route Home = GET  "/home"      { } requires user
route Go   = POST "/home/_/Go" { } requires user

type Model = { }

Home.load : Home.Params -> Session -> Query Model
Home.load params session = Query.succeed { }

other : Home.Session -> Text
other session = Ui.who session

Go.update : Go.Params -> Session -> Task Response
Go.update params session = {
  let _ <- Log.info (Ui.who session)
  Task.succeed Response.refresh
}

Home.view : Model -> Session -> Html
Home.view model session = <p>{other session}</p>
`,
	})
	contains(t, files["Home_alder.go"],
		"func widen__Home__Home__Session(s Home__Home__Session) App__Session {\n\treturn App__Session{User: rt.Just(s.User), N: s.N}\n}",
		"return Ui__who(widen__Home__Home__Session(session))",
	)
	runGo(t, files, `package app

import "testing"

func TestWiden(t *testing.T) {
	if got := Home__other(Home__Home__Session{User: "u1", N: 1}); got != "user u1" {
		t.Errorf("other = %q", got)
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
	// nested patterns at any depth (#2)
	ints("sum3 [Just 1, Just 2, Just 3]", P__sum3([]rt.Maybe[int64]{just(1), just(2), just(3)}), 6)
	ints("sum3 [Just 1, Nothing, Just 3]", P__sum3([]rt.Maybe[int64]{just(1), none, just(3)}), -1)
	ints("sum3 []", P__sum3(nil), -1)
	ints("sum3 [Just 1]", P__sum3([]rt.Maybe[int64]{just(1)}), -1)
	ints("sum3 [Just 1, Just 2, Just 3, Just 4]", P__sum3([]rt.Maybe[int64]{just(1), just(2), just(3), just(4)}), -1)
	ints("inner Just (Just 4)", P__inner(rt.Just(just(4))), 4)
	ints("inner Just Nothing", P__inner(rt.Just(none)), 0)
	ints("inner Nothing", P__inner(rt.Nothing[rt.Maybe[int64]]()), -1)
	ints("heads Just [7, 8]", P__heads(rt.Just([]int64{7, 8})), 7)
	ints("heads Just []", P__heads(rt.Just([]int64{})), 0)
	ints("heads Nothing", P__heads(rt.Nothing[[]int64]()), -1)

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

// Nested patterns at any depth run first-match-wins (#2); parse is the issue's match, which the
// filterMap-and-length workaround used to spell.
func TestGen_RunNestedPatterns(t *testing.T) {
	runGo(t, emit(t, "../../testdata/golden/gen_vet/n1-nested-patterns"), `package app

import (
	"slices"
	"testing"
)

func TestNested(t *testing.T) {
	if want := []int64{6, 4, 5, 3, 2, 9, 2}; !slices.Equal(M__values, want) {
		t.Errorf("values = %v, want %v", M__values, want)
	}
	for in, want := range map[string]float64{"1 2 3": 6, "1 x 3": -1, "": -1, "1 2 3 4": -1} {
		if got := M__parse(in); got != want {
			t.Errorf("parse %q = %v, want %v", in, got, want)
		}
	}
	if M__texts[0] != "e" {
		t.Errorf("texts = %q", M__texts)
	}
}
`)
}

// emitApp emits the app in dir and writes the files into a temporary module, returning it.
func emitApp(t *testing.T, dir string) string {
	t.Helper()
	out := t.TempDir()
	files := emit(t, dir)
	writeAdapterStubs(t, out, files)
	writeRuntime(t, out)
	for name, b := range files {
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
const effectsLoad = `func Index__Index__load(params Index__Index__Params, session Index__Index__Session) eff.Query[Index__Model] {
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
		".Send(_e1.Ctx, struct {",
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
		return db.InsertRow(e.Ctx, e.Store.(db.Store), db.TxOf(e), table__Table__User__User,
			insertvals__Table__User__User(insert__Table__User__User{Email: "a@b.c", Name: "Ann", PasswordHash: "x"}), e.Now())
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
		p := Auth__Login__Submit__Params{Form: Auth__Login__Credentials{Email: "a@b.c", Password: "pw"}}
		eff.Run(env, eff.KTask, Auth__Login__Submit__update(p, env.Session.(App__Session)))
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
	login := func(pw string) rt.Response {
		t.Helper()
		p := Auth__Login__Submit__Params{Form: Auth__Login__Credentials{Email: "a@b.c", Password: pw}}
		r, err := eff.Run(env, eff.KTask, Auth__Login__Submit__update(p, env.Session.(App__Session)))
		if err != nil {
			t.Fatalf("login: %v", err)
		}
		return r
	}
	if r := login("nope"); r.Kind != rt.RespInvalid || r.Text != "Email or password is incorrect." {
		t.Fatalf("bad login: %+v", r)
	}
	if s := env.Session.(App__Session); s.User.Ok {
		t.Fatalf("bad login set the user: %+v", s)
	}
	if r := login("pw"); r.Kind != rt.RespRedirect || r.Target.Path != "/" {
		t.Fatalf("login: %+v", r)
	}
	calls("login", "Password.verify", "Password.verify", "Db.get User", "Session.put") // the login reads the row for its role
	app := env.Session.(App__Session)
	if !app.User.Ok || app.User.Val != uid {
		t.Fatalf("login did not set the session user: %+v", app)
	}
	session := Todos__Index__Index__Session{User: app.User.Val}

	run := func(task eff.Task[rt.Response]) (rt.Response, error) {
		t.Helper()
		return eff.Run(env, eff.KTask, task)
	}
	load := func(f Table__Todo__Filter) []Table__Todo__Todo {
		t.Helper()
		m, err := eff.RunQuery(env, Todos__Index__Index__load(Todos__Index__Index__Params{Filter: f}, session))
		if err != nil {
			t.Fatalf("load: %v", err)
		}
		return m.Todos
	}

	// add
	add := func() eff.Task[rt.Response] {
		return Todos__Index__Submit__update(Todos__Index__Submit__Params{Form: Todos__Index__NewTodo{Title: "Buy milk", Due: rt.Just(rt.Date{Y: 2026, M: 10, D: 31})}}, session)
	}
	r, err := run(add())
	if err != nil || r.Kind != rt.RespRedirect || r.Target.Path != "/" {
		t.Fatalf("add: %+v, %v", r, err)
	}
	calls("add", "Db.insertMine Todo") // the owner comes from the session (env.Session)
	todos := load(Table__Todo__Filter__All__)
	if len(todos) != 1 {
		t.Fatalf("after add: %d todos", len(todos))
	}
	td := todos[0]
	if td.Title != "Buy milk" || td.Done || td.Owner != uid || !td.Created.Equal(clock) || !td.Due.Ok || td.Due.Val != (rt.Date{Y: 2026, M: 10, D: 31}) {
		t.Fatalf("added todo: %+v", td)
	}
	if n := len(load(Table__Todo__Filter__Completed__)); n != 0 {
		t.Fatalf("completed before toggle: %d", n)
	}

	// toggle
	mark = len(env.Trace)
	r, err = run(Todos__Index__Toggle__update(Todos__Index__Toggle__Params{Id: td.Id}, session))
	if err != nil || r.Kind != rt.RespRefresh {
		t.Fatalf("toggle: %+v, %v", r, err)
	}
	calls("toggle", "Db.updateMine Todo") // one statement: WHERE id = ? AND owner = ?
	if todos = load(Table__Todo__Filter__All__); len(todos) != 1 || !todos[0].Done {
		t.Fatalf("after toggle: %+v", todos)
	}
	if n := len(load(Table__Todo__Filter__Active__)); n != 0 {
		t.Fatalf("active after toggle: %d", n)
	}

	// the show page loads the user's own todo, and not one that does not exist
	mark = len(env.Trace)
	shown, err := eff.RunQuery(env, Todos__Show__Show__load(Todos__Show__Show__Params{Id: td.Id}, Todos__Show__Show__Session{User: uid}))
	if err != nil || shown.Todo.Title != "Buy milk" {
		t.Fatalf("show: %+v, %v", shown, err)
	}
	calls("Show.load", "Db.getMine Todo")
	if _, err := eff.RunQuery(env, Todos__Show__Show__load(Todos__Show__Show__Params{Id: "nope"}, Todos__Show__Show__Session{User: uid})); err != eff.ErrNotFound {
		t.Fatalf("show missing: err = %v, want ErrNotFound", err)
	}

	// toggling a todo that does not exist is Task.notFound (a 404)
	if _, err := run(Todos__Index__Toggle__update(Todos__Index__Toggle__Params{Id: "nope"}, session)); err != eff.ErrNotFound {
		t.Fatalf("toggle missing: err = %v, want ErrNotFound", err)
	}
	// so is another user's: the scoped operations act for the ambient session (env.Session), not
	// for the session argument
	env.Session = App__Session{User: rt.Just("someone else")}
	if _, err := eff.RunQuery(env, Todos__Show__Show__load(Todos__Show__Show__Params{Id: td.Id}, Todos__Show__Show__Session{User: uid})); err != eff.ErrNotFound {
		t.Fatalf("show other's: err = %v, want ErrNotFound", err)
	}
	if _, err := run(Todos__Index__Delete__update(Todos__Index__Delete__Params{Id: td.Id}, session)); err != eff.ErrNotFound {
		t.Fatalf("delete other's: err = %v, want ErrNotFound", err)
	}
	env.Session = app

	// rollback: an add followed by a failure leaves no row
	failing := eff.Bind(add(), func(rt.Response) eff.Task[rt.Response] { return eff.Fail[rt.Response]("boom") })
	var fail *eff.ErrFail
	if _, err := eff.Run(env, eff.KTask, failing); !errors.As(err, &fail) || fail.Msg != "boom" {
		t.Fatalf("failing add: err = %v", err)
	}
	if n := len(load(Table__Todo__Filter__All__)); n != 1 {
		t.Fatalf("after rolled-back add: %d todos, want 1", n)
	}

	// delete
	mark = len(env.Trace)
	r, err = run(Todos__Index__Delete__update(Todos__Index__Delete__Params{Id: td.Id}, session))
	if err != nil || r.Kind != rt.RespRefresh {
		t.Fatalf("delete: %+v, %v", r, err)
	}
	calls("delete", "Db.deleteMine Todo")
	if n := len(load(Table__Todo__Filter__All__)); n != 0 {
		t.Fatalf("after delete: %d todos", n)
	}
	calls("load", "Db.mine Todo", "Db.all Todo")

	// a Task that fails after Session.put leaves the session as it was (I1); a successful one
	// that touches no table begins no transaction (I2: the in-memory store's single connection
	// would otherwise be held)
	before := env.Session.(App__Session)
	logoutThenFail := eff.Bind(App__Logout__update(App__Logout__Params{}, before), func(rt.Response) eff.Task[rt.Response] { return eff.Fail[rt.Response]("boom") })
	if _, err := eff.Run(env, eff.KTask, logoutThenFail); !errors.As(err, &fail) || fail.Msg != "boom" {
		t.Fatalf("logout then fail: err = %v", err)
	}
	if s := env.Session.(App__Session); s != before || !s.User.Ok {
		t.Fatalf("failed Task kept its session change: %+v, want %+v", s, before)
	}
	// App's Logout route: log out
	if r, err := run(App__Logout__update(App__Logout__Params{}, env.Session.(App__Session))); err != nil || r.Kind != rt.RespRedirect || r.Target.Path != "/login" {
		t.Fatalf("logout: %+v, %v", r, err)
	}
	if s := env.Session.(App__Session); s.User.Ok {
		t.Fatalf("logout kept the user: %+v", s)
	}
	t.Logf("trace: %v", env.Trace)
	t.Logf("log: %d lines", len(logs))
}

// Serve refuses a capability left unset, naming its adapter constructor; the session codec round-trips
// App's session as JSON and starts a cookie that lacks a field from the field's default.
func TestServe(t *testing.T) {
	if _, err := Serve(Caps{}, Options{DB: ":memory:", Dev: true}); err == nil || err.Error() != "capability Auth.Login.Password is not wired: adapters/auth/: func LoginPassword returned nil" {
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
	if r := Shop__after(rt.Invalid("no")); r.Kind != rt.RespInvalid || r.Text != "no" || len(r.Events) != 1 || r.Events[0].Toast.Text != "done" ||
		!Shop__isRefresh(rt.Refresh()) || Shop__isRefresh(rt.Invalid("no")) {
		t.Errorf("after = %+v", r)
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

// TestGen_Routes: the routes of the ir_route_app app, against a golden written by hand before the
// emitter (the spec of route emission): decoders, page__/update__ functions, the GET and POST
// routes and their helpers. The whole app vets.
func TestGen_Routes(t *testing.T) {
	files := emit(t, "../../testdata/golden/ir_route_app")
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

// The session cookie stores unions by name: an unknown or missing name decodes as the field's
// default.
func TestGen_SessionCodecUnions(t *testing.T) {
	files := emitSource(t, map[string]string{
		"App.alder": `module App

type Theme = Light | Dark

session { n: Int = 0, theme: Theme = Light, last: Maybe Theme }

error : Error -> Html
error err = <p>error</p>

notFound : Html
notFound = <p>not found</p>
`,
		"M.alder": `module M

route M = GET "/m" { }

M.load : M.Params -> Session -> Query Int
M.load params session = Query.succeed 1

M.view : Int -> Session -> Html
M.view n session = <p>m</p>
`,
	})
	runGo(t, files, `package app

import (
	"testing"

	"alder/rt"
)

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

error : Error -> Html
error err = <p>error</p>

notFound : Html
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

route M    = GET  "/m"      { }
route Save = POST "/m/save" { } form F

M.load : M.Params -> Session -> Query Int
M.load params session = Query.succeed 1

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

Save.update : Save.Params -> Session -> Task Response
Save.update params session = Task.succeed Response.refresh

M.view : Int -> Session -> Html
M.view n session = <form submit={Save}><Input field="pw" /><Input field="slug" /></form>
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
	page := html.RenderWith(M__M__view(1, App__Session{}), &html.FormState{Form: "M.F", Msg: "POST /m/save", Values: map[string]string{"slug": "A1"}, Errors: map[string][]string{}})
	if !strings.Contains(page, `+"`"+`<input name="pw" type="password" required />`+"`"+`) || !strings.Contains(page, `+"`"+`value="A1"`+"`"+`) {
		t.Fatalf("view: %s", page)
	}
}
`)
}

// A link to a route carries its path parameters in the path (each escaped as one segment) and
// the others as the query, in declaration order, rendered as text; a parameter at its default
// and a Maybe at Nothing are left out. The route's decoder reads the link back.
func TestGen_PageURLQuery(t *testing.T) {
	files := emitSource(t, map[string]string{
		"App.alder": appModule,
		"T.alder": `module T

type Filter = All | Active | Done

route T = GET "/t/{id}/x" { id: Text, filter: Filter = All, q: Maybe Text, n: Int, b: Bool = False, tags: List Text, m: Maybe Int = Just 3 }

T.load : T.Params -> Session -> Query Int
T.load params session = Query.succeed 1

T.view : Int -> Session -> Html
T.view n session = <p>t</p>

link1 : Route
link1 = T { id: "a/b c", filter: Active, q: Just "x&y", n: 2, b: True, tags: ["p", "q r"], m: Nothing }

link2 : Text -> Route
link2 id = T { id: id, q: Nothing, n: -1, tags: [] }
`,
		"I.alder": `module I

import T

route I = GET "/" { filter: T.Filter = All }

I.load : I.Params -> Session -> Query Int
I.load params session = Query.succeed 1

I.view : Int -> Session -> Html
I.view n session =
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
	p, errs := decode__T__T__Params(map[string]string{"id": "a/b c"}, u.Query())
	if errs != nil || p.Id != "a/b c" || p.Filter.Tag != is__T__Filter__Active || p.Q.Val != "x&y" || p.N != 2 || !p.B || strings.Join(p.Tags, ",") != "p,q r" || p.M.Ok {
		t.Fatalf("decoded link1: %+v %v", p, errs)
	}
	p, errs = decode__T__T__Params(map[string]string{"id": "c"}, url.Values{"n": {"-1"}})
	if errs != nil || p.Filter.Tag != is__T__Filter__All || p.Q.Ok || p.B || len(p.Tags) != 0 || p.M.Val != 3 {
		t.Fatalf("decoded link2: %+v %v", p, errs)
	}
	if _, errs := decode__T__T__Params(map[string]string{"id": "c"}, url.Values{"n": {"x"}, "filter": {"Nope"}}); len(errs) != 2 {
		t.Fatalf("bad params: %v", errs)
	}
	// a Bool URL parameter is strict (web.BoolParam): ?b=maybe is an error, not True
	if _, errs := decode__T__T__Params(map[string]string{"id": "c"}, url.Values{"n": {"1"}, "b": {"maybe"}}); len(errs) != 1 || errs[0].Field != "b" {
		t.Fatalf("b=maybe: %v", errs)
	}
	for raw, want := range map[string]bool{"True": true, "1": true, "false": false} {
		if p, errs := decode__T__T__Params(map[string]string{"id": "c"}, url.Values{"n": {"1"}, "b": {raw}}); errs != nil || p.B != want {
			t.Fatalf("b=%s: %+v %v", raw, p, errs)
		}
	}
	page := html.Render(I__I__view(0, App__Session{}))
	if !strings.Contains(page, `+"`"+`<a href="/">x</a><a href="/?filter=Active">x</a>`+"`"+`) {
		t.Fatalf("links: %s", page)
	}
}
`)
}

// Per-row forms of one form type (I1, the reviewer's adv3): a Response.invalid (or a form error)
// shows on the row whose form posted the write route, not on every form of that type; a plain GET shows the
// state nowhere.
func TestGen_FormStatePerRow(t *testing.T) {
	files := emitSource(t, map[string]string{
		"App.alder": `module App

session { user: Maybe Text }

login = P.P

error : Error -> Html
error err = <html><head><title>E</title></head><body><p>error</p></body></html>

notFound : Html
notFound = <html><head><title>NF</title></head><body><p>nf</p></body></html>
`,
		"P.alder": `module P

route P    = GET  "/"     { }
route Save = POST "/save" { row: Text } form Rename

form Rename = { title: Text.NonEmpty }

P.load : P.Params -> Session -> Query (List Text)
P.load params session = Query.succeed ["a", "b", "c"]

Save.update : Save.Params -> Session -> Task Response
Save.update p session = Task.succeed (Response.invalid ("bad " ++ p.row))

P.view : List Text -> Session -> Html
P.view rows session = <html><head><title>P</title></head><body>
  {for r in rows -> <form submit={Save { row: r }}><Errors form /><Input field="title" label={r} /><Errors field="title" /><button>s</button></form>}
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
		if w.Code != 422 { // Response.invalid, and a form that does not decode, re-render with 422
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
	body := post("/save?row=b", "title=typed")
	for i, row := range rows(body) {
		shows := strings.Contains(row, "<li>bad b</li>") && strings.Contains(row, `+"`"+`value="typed"`+"`"+`)
		if (i == 1) != shows || strings.Count(body, "typed") != 1 {
			t.Fatalf("Reject: row %d shows the state: %v\n%s", i, shows, body)
		}
	}
	body = post("/save?row=c", "title=")
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

// The re-review's fresh1 probe: a redirect to `Url.external` of user input cannot run script
// under Datastar, and an `href={Url.external …}` of it is "#".
func TestGen_UrlExternalIsSanitized(t *testing.T) {
	files := emitSource(t, map[string]string{
		"App.alder": appModule,
		"F.alder": `module F

route F  = GET  "/"   { q: Text = "" }
route Go = POST "/go" { to: Text }

F.load : F.Params -> Session -> Query Text
F.load params session = Query.succeed params.q

Go.update : Go.Params -> Session -> Task Response
Go.update params session = Task.succeed (Response.redirect (Url.external params.to))

F.view : Text -> Session -> Html
F.view q session = <p><a id="lnk" href={Url.external q}>x</a></p>
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
	// Url.external made the script "#", and a redirect to "#" is refused: nothing navigates
	w := do("POST", "/go?to="+evil, true)
	if !strings.Contains(w.Body.String(), "refusing to navigate") || strings.Contains(w.Body.String(), "javascript") {
		t.Fatalf("Datastar Go: %d %s", w.Code, w.Body.String())
	}
	if w := do("POST", "/go?to="+evil, false); w.Header().Get("Location") != "" || !strings.Contains(w.Body.String(), "refusing to navigate") || strings.Contains(w.Body.String(), "javascript") {
		t.Fatalf("Go: %d %q %s", w.Code, w.Header().Get("Location"), w.Body.String())
	}
	if w := do("POST", "/go?to="+url.QueryEscape("https://ex.com/x"), false); w.Code != 303 || w.Header().Get("Location") != "https://ex.com/x" {
		t.Fatalf("Go https: %d %q", w.Code, w.Header().Get("Location"))
	}
	if w := do("GET", "/?q="+url.QueryEscape("JavaScript:alert(1)"), false); !strings.Contains(w.Body.String(), `+"`"+`<a id="lnk" href="#">`+"`"+`) {
		t.Fatalf("href: %s", w.Body.String())
	}
}
`)
}

// Every union of the program, from every module file, registers its constructor names from
// app__alder.go's init, in declaration order, so rt.Show prints its values.
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
		`rt.RegisterUnion(reflect.TypeOf(tag__Table__Todo__Filter(0)), []string{"All", "Active", "Completed"})`,
		`rt.RegisterUnion(reflect.TypeOf(tag__Table__User__Role(0)), []string{"Member", "Admin"})`)
}

// runTodosShowTest prints todos values with rt.Show: unions from every module file (Todo's
// Filter, User's Role) print by their constructor names, and route parameters as records, never as Go.
const runTodosShowTest = `

func TestTodosShow(t *testing.T) {
	due := rt.Just(rt.Date{Y: 2026, M: 10, D: 9})
	for _, c := range []struct {
		v    any
		want string
	}{
		{Todos__Index__Toggle__Params{Id: "x"}, "{ id: \"x\" }"},
		{Todos__Index__Submit__Params{Filter: Table__Todo__Filter__Completed__, Form: Todos__Index__NewTodo{Title: "Milk", Due: due}},
			"{ filter: Completed, form: { title: \"Milk\", due: Just 2026-10-09 } }"},
		{App__Session{User: rt.Just("u1")}, "{ user: Just \"u1\", role: Member }"}, // a union's zero value is its first constructor
		{Todos__Index__Model{Filter: Table__Todo__Filter__Active__, Todos: []Table__Todo__Todo{{Id: "1", Title: "a", Created: time.Unix(0, 0)}}},
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
// cookie codec), a GET route's parameter, a write route's parameter, a form and a table row.
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

error : Error -> Html
error err = <p>error</p>

notFound : Html
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

route Home = GET  "/home"      { _q: Int = 0 }
route Add  = POST "/home/add"  { } form NewRow
route Pick = POST "/home/pick" { _k: Int }

form NewRow = { _note: Text }

type Model = { _rows: List Row.Row, _h: Html, _d: Dict Text Time }

Home.load : Home.Params -> Session -> Query Model
Home.load params session = {
  let rows <- Db.from Row |> Db.orderBy Row._at.asc |> Db.all
  Query.succeed { _rows: rows, _h: <b>{Text.fromInt params._q}</b>, _d: Dict.empty }
}

Home.view : Model -> Session -> Html
Home.view model session =
  <div>
    {model._h}
    <ul>{for r in model._rows -> <li>{r._note}</li>}</ul>
    <form submit={Add}>
      <Input field="_note" label="Note" />
      <Errors field="_note" />
    </form>
    <button click={Pick { _k: 7 }}>pick</button>
  </div>

Add.update : Add.Params -> Session -> Task Response
Add.update p session = {
  let _ <- Db.insert Row { _note: p.form._note }
  Task.succeed Response.refresh
}

Pick.update : Pick.Params -> Session -> Task Response
Pick.update p session = {
  let _ <- Session.put (\s -> { s | _n: s._n + p._k })
  Task.succeed (Response.redirect (Home { _q: p._k }))
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
	// a GET route's parameter
	if w := do("GET", "/home?_q=3", ""); w.Code != 200 || !strings.Contains(w.Body.String(), "<b>3</b>") {
		t.Fatalf("GET: %d %s", w.Code, w.Body.String())
	}
	// a form field, into a table row and back
	if w := do("POST", "/home/add", "_note=milk"); w.Code != http.StatusSeeOther {
		t.Fatalf("Add: %d %s", w.Code, w.Body.String())
	}
	if w := do("GET", "/home", ""); !strings.Contains(w.Body.String(), "<li>milk</li>") {
		t.Fatalf("the row: %s", w.Body.String())
	}
	// a write route's parameter, and the session
	if w := do("POST", "/home/pick?_k=7", ""); w.Code != http.StatusSeeOther || w.Header().Get("Location") != "/home?_q=7" {
		t.Fatalf("Pick: %d %q", w.Code, w.Header().Get("Location"))
	}
	if cookie == "" {
		t.Fatal("Pick stored no session")
	}
}
`)
}

// TestGen_Tests: in a test build an app's `test` declarations are emitted as test__M__N
// functions and the Tests table of app__alder.go (with TestSession and Registry), and run through rtesting.Run they
// compute the right outcomes: a pure test's recorded `==` sides, Test.run undoing a failed
// task's writes inside the test's transaction, its trace and Test.trace
// (testdata/e2e_tests_test.go.txt).
func TestGen_Tests(t *testing.T) {
	files := emitSourceWith(t, map[string]string{
		"App.alder": `module App

session { n: Int = 0 }

error : Error -> Html
error err = <html><body><p>error</p></body></html>

notFound : Html
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
	}, true)
	contains(t, files["Shop_alder.go"], "func test__Shop__1() bool {", "rtesting.Eq(", "func test__Shop__2() bool {")
	contains(t, files["Shop__Orders_alder.go"], "func test__Shop__Orders__1() eff.Task[bool] {", "func test__Shop__Orders__2() eff.Task[bool] {",
		`eff.Call("Test.run", true,`, "rtesting.Inner(", `eff.Call("Test.trace", false,`, "rtesting.Trace(")
	contains(t, files["app__alder.go"], "var Tests = []rtesting.Case{\n", `{Module: "Shop", Name: "total adds", File: "app/Shop.alder", Line: 6, Col: 1, Pure: test__Shop__1},`,
		`{Module: "Shop.Orders", Name: "a failed run undoes its writes", File: "app/Shop/Orders.alder", Line: 11, Col: 1, Task: test__Shop__Orders__1}`,
		"func TestSession() any {", "func (Caps) Registry() map[string]any {", "reg := caps.Registry()")
	prog, err := os.ReadFile("testdata/e2e_tests_test.go.txt")
	if err != nil {
		t.Fatal(err)
	}
	runGo(t, files, string(prog))
}

// TestGen_Fakes: a test build of the gen_fakes golden (corpus fake-ok with a charge that can
// fail) runs its tests through rtesting as `alder test` does: the Stripe-shaped fake's
// lifecycle (charge, refund, read the fake's table back), a failing entry under Test.run (Err,
// its insert rolled back) and outside it (the test fails), a Query entry, Test.fake twice, and
// no fake leaking into the next test (testdata/e2e_fakes_test.go.txt).
func TestGen_Fakes(t *testing.T) {
	files := emitWith(t, "../../testdata/golden/gen_fakes", true)
	contains(t, files["Payments_alder.go"], "type fake__Payments__Stripe struct{}", "func (fake__Payments__Stripe) Charge(ctx context.Context, args Args__Payments__Stripe__Charge)",
		"env := eff.EnvFrom(ctx)", "return eff.RunInner(env, fakeentry__Payments__Stripe__Charge(args))", "return eff.RunInner(env, eff.T(fakeentry__Payments__Stripe__Balance))",
		"type Payments__Charge struct {", "var table__Payments__Charge = &db.Table[Payments__Charge]{")
	contains(t, files["Checkout_alder.go"], `eff.Call("Test.fake Stripe", false,`, `_e1.Caps["Payments.Stripe"] = Fakes["Payments.Stripe"]()`)
	contains(t, files["app__alder.go"], "var FakeTables = []db.TableDesc{table__Payments__Charge.Desc()}", `"Payments.Stripe":   func() any { return fake__Payments__Stripe{} },`)
	if strings.Contains(string(files["app__alder.go"]), "var Tables = []db.TableDesc{table__User__User.Desc(), table__Payments") {
		t.Error("a fake's table is in Tables")
	}
	prog, err := os.ReadFile("testdata/e2e_fakes_test.go.txt")
	if err != nil {
		t.Fatal(err)
	}
	runGo(t, files, string(prog))
}

// TestGen_Browser: a test build of the gen_browser golden (corpus browser-ok) emits the browser
// steps as Calls into the test's browser, the forms' encoders and the selector literals parsed
// once, and runs its tests through rtesting with the app's Routes and Web() as `alder test`
// does (RF3, RF4, RF5); decode(encode(r)) == r for a form with a field of every kind
// (testdata/e2e_browser_test.go.txt).
func TestGen_Browser(t *testing.T) {
	files := emitWith(t, "../../testdata/golden/gen_browser", true)
	contains(t, files["Notes__Index_alder.go"], `eff.Call("Test.get Notes.Index.Index", true,`, `.GetPage(_u`, `, "user")`, `rtesting.ModelOf[Notes__Index__Model](_u`,
		`eff.Call("Test.click Notes.Index.Toggle", true,`, `.ClickRoute(rt.Route{Name: "Notes.Index.Toggle", Method: "POST", Path: "/_/Toggle", Query: []rt.KV{{Key: "id", Value: _m`,
		`rtesting.MsgName("Notes.Index.Toggle", _m`, `.SubmitRoute(rt.Route{Name: "Notes.Index.Submit", Method: "POST", Path: "/_/Submit"}, "NewNote", encode__Notes__Index__NewNote(`,
		`rtesting.RawFields("NewNote", []string{"title", "due", "urgent", "priority", "weight", "kind", "remark", "contact"},`,
		`rtesting.DocCountSel(selector__`, `rtesting.DocTexts(sel, `, `rtesting.DocCount(bad, `, `rtesting.ViewDoc(Notes__Index__Index__view(`, `case rtesting.RequestFailed:`)
	contains(t, files["Auth__Login_alder.go"], `eff.Call("Test.login", true,`, `eff.Call("Test.open Notes.Index.Index", true,`, `.Open(_u`,
		`rtesting.Answer{Tag: rtesting.Redirected, Page: rt.Route{Name: "Auth.Login.Login", Method: "GET", Path: "/login"}}`, `rtesting.FormRejectedAnswer([]struct {`,
		`eff.Call("Test.click App.Logout", true,`, `rtesting.MsgName("App.Logout", nil)`)
	contains(t, files["routes__alder.go"], "func encode__Notes__Index__NewNote(r Notes__Index__NewNote) rtesting.Fields {")
	contains(t, files["app__alder.go"], "func Web() web.Options {", `selector__1  = rtesting.Sel("h1")`)
	prog, err := os.ReadFile("testdata/e2e_browser_test.go.txt")
	if err != nil {
		t.Fatal(err)
	}
	runGo(t, files, string(prog))
}

// TestGen_Wire (#55): a test build of the gen_wire golden, whose page names its query keys
// (`electionId: Text = "" as "ElectionId"`), decodes the wire keys and emits them in every URL
// of the page: a link, the page's message URLs (_p.<key>), a redirect and a test's visit
// (testdata/e2e_wire_test.go.txt).
func TestGen_Wire(t *testing.T) {
	files := emitWith(t, "../../testdata/golden/gen_wire", true)
	contains(t, files["routes__alder.go"], `web.Text(q, "ElectionId", false)`, `web.Text(q, "ContestId", false)`)
	prog, err := os.ReadFile("testdata/e2e_wire_test.go.txt")
	if err != nil {
		t.Fatal(err)
	}
	runGo(t, files, string(prog))
}

// TestGen_ProdHasNoTests: a production build (Options.Tests unset) carries no test code: no
// Tests, TestSession or test bodies, no fake code, fake tables, FakeTables or Fakes, and no
// import of alder/rt/testing — for an app with fakes and tests that use them (fake-ok, the
// gen_fakes golden) and for the todos example. It still vets (TestGenGolden builds want_prod/).
func TestGen_ProdHasNoTests(t *testing.T) {
	for _, dir := range []string{"../../testdata/corpus/fake-ok", "../../testdata/golden/gen_fakes", "../../examples/todos", "../../testdata/corpus/browser-ok"} {
		files := emitWith(t, dir, false)
		for name, b := range files {
			for _, bad := range []string{"Tests", "TestSession", "rtesting", "alder/rt/testing", "test__", "Test.", "fake__", "fakeentry__",
				"Payments__Charge", "FakeTables", "Fakes[", "var Fakes", "EnvFrom", "RunInner", "encode__", "selector__", "BrowserOf"} {
				if strings.Contains(string(b), bad) {
					t.Errorf("%s: production %s contains %q", dir, name, bad)
				}
			}
		}
		if dir != "../../examples/todos" { // todos' production module is vetted and built with its real adapters by TestBuild_Todos (cmd/alder)
			vet(t, files)
		}
	}
	// the same app as a test build has them all
	files := emitWith(t, "../../testdata/corpus/fake-ok", true)
	contains(t, files["app__alder.go"], "var Tests = []rtesting.Case{", "func TestSession() any {", "var Fakes = map[string]func() any{")
	contains(t, files["Checkout_alder.go"], "func test__Checkout__1() eff.Task[bool] {", `eff.Call("Test.fake Stripe", false,`)
}

// TestGen_ProdTestNode: a Test node reaching the emitter in a production build (which emits no
// `test` bodies) is an internal error, not code: here a test's body moved into a value.
func TestGen_ProdTestNode(t *testing.T) {
	// fake-ok's first test starts with Test.fake; browser-ok's with Test.open (a browser step)
	for _, dir := range []string{"../../testdata/corpus/fake-ok", "../../testdata/corpus/browser-ok"} {
		p := lowerDir(t, dir)
		moved := false
		for _, m := range p.Modules {
			if len(m.Tests) > 0 && !moved {
				body := m.Tests[0].Body
				m.Values = append(m.Values, &ir.Value{Module: m.Name, Name: "leaked", Type: body.Type(), Body: body})
				moved = true
			}
		}
		if !moved {
			t.Fatalf("%s has no tests", dir)
		}
		_, err := Emit(p, Options{})
		if err == nil || !strings.Contains(err.Error(), "in a production build (test nodes are emitted only with Options.Tests)") {
			t.Fatalf("%s: Emit: %v", dir, err)
		}
	}
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

// A test build's missing adapters (Options.Unwired) are left nil in the caps package, which
// does not import a package none of the others needs; a production build refuses them.
func TestCapsPackage_Unwired(t *testing.T) {
	p := &ir.Program{Caps: []*ir.Capability{{Module: "Mail", Name: "Mailer"}, {Module: "Auth.Login", Name: "Password"}, {Module: "Auth.Login", Name: "Token"}}}
	src := string(CapsPackage(p, map[string]bool{"Mail.Mailer": true, "Auth.Login.Token": true}))
	for _, want := range []string{
		"a_auth \"app/adapters/auth\"",
		"Auth__Login__Password: a_auth.LoginPassword(),",
		"// Mail__Mailer: no adapter (adapters/mail/ has no func Mailer): unwired in tests",
		"// Auth__Login__Token: no adapter (adapters/auth/ has no func LoginToken): unwired in tests",
	} {
		if !strings.Contains(src, want) {
			t.Errorf("missing %q in:\n%s", want, src)
		}
	}
	if strings.Contains(src, "app/adapters/mail") {
		t.Errorf("imports the unwired package:\n%s", src)
	}
	if _, err := Emit(p, Options{Unwired: map[string]bool{"Mail.Mailer": true}}); err == nil || !strings.Contains(err.Error(), "production") {
		t.Errorf("production build with unwired: %v", err)
	}
}

// TestGen_Mutate: the mutate build (Options.Mutate) of every mutate fixture, of the examples
// and of the mutation-testing corpus apps vets — every mutant compiled into the one build is
// well-typed Go — and lists every mutant mutate.Collect finds; Mutate needs Tests; and the
// production build of each is the build without Mutate (no switch, no table).
func TestGen_Mutate(t *testing.T) {
	dirs, _ := filepath.Glob("../../testdata/mutate/*")
	dirs = append(dirs, "../../examples/todos", "../../examples/blog", "../../testdata/corpus/mutate-ok", "../../testdata/corpus/mutate-crash", "../../testdata/corpus/rules-ok")
	for _, dir := range dirs {
		t.Run(filepath.Base(dir), func(t *testing.T) {
			t.Parallel()
			files := emitOpts(t, dir, Options{Tests: true, Mutate: true})
			ms, _ := mutate.Collect(lowerDir(t, dir))
			app := string(files[appFile])
			if got := strings.Count(app, "{ID: "); got != len(ms) {
				t.Fatalf("Mutants has %d entries, want %d", got, len(ms))
			}
			for _, m := range ms {
				if m.Node == nil && m.Page == nil && m.Route == nil {
					continue
				}
				// a switch's case, or an If's or a Match's test of Mutant (mutatedIf, mutatedMatch)
				needles := []string{fmt.Sprintf("case %d:\n", m.ID), fmt.Sprintf("Mutant == %d ", m.ID), fmt.Sprintf("Mutant != %d ", m.ID), fmt.Sprintf("(Mutant == %d)", m.ID)}
				if m.Page != nil || m.Route != nil {
					needles = []string{fmt.Sprintf("Mutant == %d {", m.ID), fmt.Sprintf("Mutant != %d && ", m.ID)} // requires, role dropped
				}
				found := false
				for name, src := range files {
					for _, needle := range needles {
						if strings.HasSuffix(name, ".go") && strings.Contains(string(src), needle) {
							found = true
						}
					}
				}
				if !found {
					t.Errorf("mutant %d (%s %s:%d %s) has no switch", m.ID, m.Module, m.Decl, m.Line, m.Kind)
				}
			}
			vet(t, files)
			for name, src := range emitOpts(t, dir, Options{}) {
				if strings.Contains(string(src), "Mutant") {
					t.Errorf("the production build's %s mentions Mutant", name)
				}
			}
		})
	}
	if _, err := Emit(lowerDir(t, "../../testdata/corpus/mutate-ok"), Options{Mutate: true}); err == nil {
		t.Fatal("Mutate without Tests: no error")
	}
}

// TestGen_Nested: nested patterns with a shared fallback (the gen_nested golden: two matches of
// 12 arms, each arm testing two fields, so each arm's fallback — the arms after it — is shared
// by two branches) are emitted once per shared node (share.go): the plain build grows linearly,
// within 300 bytes per arm, and every arm, each fallback path and the final fallback compute the
// right value. (Each place used to emit its own copy: 2^12 copies of the last arms, 1.49 MB.)
// A named catch-all arm's body, shared under each Case's own `let name = subject` (a Maybe
// match, a union match, a name shadowing a parameter), is emitted per place: the fixture's
// `test`s, which pass with the emitter before share.go, pass through rtesting.Run.
func TestGen_Nested(t *testing.T) {
	const dir, depth = "../../testdata/golden/gen_nested", 12
	const arms = 2*depth + 12 // pick and route, then the named catch-all matches
	files := emit(t, dir)
	if size := len(files["Deep_alder.go"]); size > 300*arms {
		t.Errorf("Deep_alder.go is %d bytes: more than 300 bytes per arm (%d arms)", size, arms)
	}
	vet(t, emitOpts(t, dir, Options{Tests: true, Mutate: true}))
	var b strings.Builder
	b.WriteString(`package app

import (
	"reflect"
	"testing"

	"alder/rt"
	"alder/rt/eff"
)

// row is a Row whose fields js are Just their index, the others Nothing.
func row(js ...int) Deep__Row {
	var r Deep__Row
	v := reflect.ValueOf(&r).Elem()
	for i := 0; i < v.NumField(); i++ {
		v.Field(i).Set(reflect.ValueOf(rt.Nothing[int64]()))
	}
	for _, i := range js {
		v.Field(i).Set(reflect.ValueOf(rt.Just(int64(i))))
	}
	return r
}

// hops is a Hops whose fields oks are Ok their index, the others Err.
func hops(oks ...int) Deep__Hops {
	var h Deep__Hops
	v := reflect.ValueOf(&h).Elem()
	for i := 0; i < v.NumField(); i++ {
		v.Field(i).Set(reflect.ValueOf(rt.Err[string, int64]("no")))
	}
	for _, i := range oks {
		v.Field(i).Set(reflect.ValueOf(rt.Ok[string, int64](int64(i))))
	}
	return h
}

func check(t *testing.T, name string, set []int, want int64) {
	t.Helper()
	if got := Deep__pick(row(set...)); got != want {
		t.Errorf("pick %s %v = %d, want %d", name, set, got, want)
	}
	got, err := eff.Run(&eff.Env{}, eff.KTask, Deep__hop(hops(set...)))
	switch {
	case want == 0 && err != eff.ErrNotFound:
		t.Errorf("route %s %v = %d, %v, want not found", name, set, got, err)
	case want != 0 && (err != nil || got != want):
		t.Errorf("route %s %v = %d, %v, want %d", name, set, got, err, want)
	}
}

func TestNested(t *testing.T) {
`)
	for k := 0; k < depth; k++ {
		want := 2*k + 1 + (k+1)*100
		fmt.Fprintf(&b, "\tcheck(t, \"arm %d\", []int{%d, %d}, %d)\n", k, k, k+1, want)
		if k >= 2 { // arm k-2 fails at its second test, arm k-1 at its first
			fmt.Fprintf(&b, "\tcheck(t, \"arm %d after a second-test failure\", []int{%d, %d, %d}, %d)\n", k, k-2, k, k+1, want)
		}
	}
	b.WriteString("\tcheck(t, \"fallback\", nil, 0)\n\tcheck(t, \"fallback after second-test failures\", []int{0, 2, 4, 6, 8, 10, 12}, 0)\n}\n")
	runGo(t, files, b.String())
	runGo(t, emitWith(t, dir, true), `package app

import (
	"testing"

	rtesting "alder/rt/testing"
)

func TestAlderTests(t *testing.T) {
	rs := rtesting.Run(Tests, rtesting.Options{Tables: Tables, Session: TestSession(), Caps: Caps{}.Registry()})
	if len(rs) != 15 {
		t.Fatalf("%d results, want 15", len(rs))
	}
	for i, r := range rs {
		if !r.OK {
			t.Errorf("%s: %+v", Tests[i].Name, r)
		}
	}
}
`)
}

// TestGen_MutateNested: a mutant of an If, a Match or an operator is emitted in place, its
// subtrees once (mutatedIf, mutatedMatch, hoist), so the mutate build of nested mutated code
// grows with the number of mutants, not with the number of programs they make: a 12-deep
// else-if validation chain (two mutants per level), a 12-deep `&&` chain and a 12-deep chain
// of matches (two arm swaps per level) stay within a few hundred bytes per mutant of the plain
// test build, and type-check quickly. (Each replacement used to re-emit the whole subtree:
// 3^12 copies of the chain's tail.)
func TestGen_MutateNested(t *testing.T) {
	const depth = 12
	var b strings.Builder
	b.WriteString("module Form\n\ntype Row = {\n")
	for i := 1; i <= depth; i++ {
		fmt.Fprintf(&b, "  f%d: Text,\n", i)
	}
	b.WriteString("}\n\ntype Step = Go | Stop | Wait\n\nvalidate : Row -> Task Text\nvalidate r =\n")
	for i := 1; i <= depth; i++ {
		pre := "  else "
		if i == 1 {
			pre = "  "
		}
		fmt.Fprintf(&b, "%sif Text.isEmpty r.f%d then Task.fail \"f%d is empty\"\n", pre, i, i)
	}
	b.WriteString("  else Task.succeed r.f1\n\nfull : Row -> Bool\nfull r =\n  ")
	for i := 1; i <= depth; i++ {
		if i > 1 {
			b.WriteString(" && ")
		}
		fmt.Fprintf(&b, "r.f%d != \"\"", i)
	}
	b.WriteString("\n\nwalk : List Step -> Int\nwalk steps =\n")
	for i := 1; i <= depth; i++ {
		ind := strings.Repeat("  ", 2*i-1)
		fmt.Fprintf(&b, "%smatch List.head (List.drop %d steps) ?? Go {\n%s  Go -> %d,\n%s  Stop -> %d,\n%s  Wait ->\n", ind, i, ind, 10*i, ind, 10*i+1, ind)
	}
	fmt.Fprintf(&b, "%s0\n", strings.Repeat("  ", 2*depth+1))
	for i := depth; i >= 1; i-- {
		fmt.Fprintf(&b, "%s}\n", strings.Repeat("  ", 2*i-1))
	}
	b.WriteString("\ntest \"a full row\" = full { ")
	for i := 1; i <= depth; i++ {
		if i > 1 {
			b.WriteString(", ")
		}
		fmt.Fprintf(&b, "f%d: \"x\"", i)
	}
	b.WriteString(" }\n")
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "app"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "app", "Form.alder"), []byte(b.String()), 0o644); err != nil {
		t.Fatal(err)
	}
	ms, _ := mutate.Collect(lowerDir(t, dir))
	if len(ms) < 5*depth {
		t.Fatalf("%d mutants, want at least %d", len(ms), 5*depth)
	}
	plain := emitOpts(t, dir, Options{Tests: true})
	start := time.Now()
	files := emitOpts(t, dir, Options{Tests: true, Mutate: true})
	if d := time.Since(start); d > 5*time.Second {
		t.Errorf("the mutate build took %v to emit", d)
	}
	size, base := len(files["Form_alder.go"]), len(plain["Form_alder.go"])
	if size > base+300*len(ms) {
		t.Errorf("Form_alder.go is %d bytes in the mutate build, %d in the test build: more than 300 bytes per mutant (%d)", size, base, len(ms))
	}
	t.Logf("%d mutants; Form_alder.go: %d bytes (test build %d)", len(ms), size, base)
	start = time.Now()
	vet(t, files)
	if d := time.Since(start); d > 60*time.Second {
		t.Errorf("the mutate build took %v to vet", d)
	}
}

// renderGoldens are the values of testdata/golden/render_html's module R whose rendered HTML is
// pinned, each in want/<value>.html (go test -run RenderGolden -update rewrites them).
var renderGoldens = []string{"media", "inline", "external"}

// TestRenderGolden emits the render_html app, runs the generated code and compares each value's
// html.Render with its golden: what a browser receives, byte for byte.
func TestRenderGolden(t *testing.T) {
	dir := "../../testdata/golden/render_html"
	out := t.TempDir()
	var calls strings.Builder
	for _, v := range renderGoldens {
		fmt.Fprintf(&calls, "\twrite(%q, html.Render(R__%s))\n", filepath.Join(out, v+".html"), v)
	}
	runGo(t, emit(t, dir), `package app

import (
	"os"
	"testing"

	"alder/rt/html"
)

func TestRender(t *testing.T) {
	write := func(path, s string) {
		if err := os.WriteFile(path, []byte(s), 0o644); err != nil {
			t.Fatal(err)
		}
	}
`+calls.String()+`}
`)
	all := ""
	for _, v := range renderGoldens {
		got, err := os.ReadFile(filepath.Join(out, v+".html"))
		if err != nil {
			t.Fatal(err)
		}
		golden := filepath.Join(dir, "want", v+".html")
		if *update {
			if err := os.WriteFile(golden, got, 0o644); err != nil {
				t.Fatal(err)
			}
		}
		want, err := os.ReadFile(golden)
		if err != nil {
			t.Fatal(err)
		}
		if string(got) != string(want) {
			t.Errorf("%s renders\n%s\nwant\n%s", v, got, want)
		}
		all += string(got)
	}
	// every <script> and <style> body reaches the page byte for byte as the source wrote it
	src, err := os.ReadFile(filepath.Join(dir, "app", "R.alder"))
	if err != nil {
		t.Fatal(err)
	}
	bodies := regexp.MustCompile(`(?s)<(script|style)[^>]*>(.*?)</(?:script|style)>`).FindAllSubmatch(src, -1)
	if len(bodies) < 3 {
		t.Fatalf("only %d raw-text bodies in R.alder", len(bodies))
	}
	for _, b := range bodies {
		if len(b[2]) > 0 && !strings.Contains(all, string(b[0])) {
			t.Errorf("<%s> body not rendered byte for byte:\n%s", b[1], b[0])
		}
	}
}

// A <script> or <style> body is emitted as html.RawText of a Go string literal and nothing else:
// no Alder value can be spliced into script or style text (rt/html renders only RawText there).
func TestGen_RawTextIsLiteral(t *testing.T) {
	files := emit(t, "../../testdata/golden/render_html")
	src := string(files["R_alder.go"])
	uses := regexp.MustCompile(`html\.RawText\(`).FindAllStringIndex(src, -1)
	if len(uses) < 3 {
		t.Fatalf("only %d html.RawText in R_alder.go:\n%s", len(uses), src)
	}
	for _, u := range uses {
		rest := src[u[1]:]
		lit, err := strconv.QuotedPrefix(rest)
		if err != nil || !strings.HasPrefix(rest[len(lit):], ")") {
			t.Errorf("html.RawText takes something other than one string literal: %.60s", rest)
		}
	}
}

// Emitting a chain of lets takes time polynomial in its length: whether a let is emitted (its
// name used in its body) is decided once per let, not again by every count that reaches it
// (which was 2^k: 26 lets took 11s).
func TestGen_LetChainLinear(t *testing.T) {
	var b strings.Builder
	b.WriteString("module M\n\nf : Int -> Int\nf x = {\n  let a0 = x + 1\n")
	for i := 1; i < 40; i++ {
		fmt.Fprintf(&b, "  let a%d = a%d + 1\n", i, i-1)
	}
	b.WriteString("  a39\n}\n")
	done := make(chan map[string][]byte, 1)
	go func() { done <- emitSource(t, map[string]string{"M.alder": b.String()}) }()
	select {
	case files := <-done:
		contains(t, files["M_alder.go"], "a39 := a38 + 1")
	case <-time.After(3 * time.Second):
		t.Fatal("emitting a chain of 40 lets took over 3s")
	}
}

func TestGen_FilePage(t *testing.T) {
	files := emitSource(t, map[string]string{
		"App.alder": "module App\n\nerror : Error -> Html\nerror err = <h1>Error</h1>\n\nnotFound : Html\nnotFound = <h1>Not found</h1>\n",
		"Report.alder": `module Report

route Report = GET "/report.txt" { }

Report.load : Report.Params -> Session -> Query Int
Report.load params session = Query.succeed 1

Report.view : Int -> Session -> File
Report.view n session = File.text { contentType: "text/plain", body: "hi" } |> File.withStatus 503
`,
		"Home.alder": `module Home

route Home = GET "/" { }

Home.load : Home.Params -> Session -> Query Int
Home.load params session = Query.succeed 1

Home.view : Int -> Session -> Html
Home.view n session = <a href={Report.Report {}}>report</a>
`,
	})
	contains(t, files["routes__alder.go"],
		`return web.Route{Method: "GET", Pattern: "/report.txt", File: true, Handle: func(rq *web.Request) web.Response {`,
		"return web.FileResponse(Report__Report__view(model, session), rq.Session)")
	contains(t, files["Home_alder.go"], `html.A("data-alder-reload", "true")`)
	vet(t, files)
}

// Options.Thunks makes the named constants functions of no parameters (a scratch file's
// watches, so a panic in one is recovered at its call), called at each use; other constants
// stay package variables.
func TestGenThunksOption(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "app"), 0o755); err != nil {
		t.Fatal(err)
	}
	src := "module M\n\nx : Int\nx = 1\n\ny : Int\ny = x + 1\n"
	if err := os.WriteFile(filepath.Join(dir, "app", "M.alder"), []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	got := string(emitOpts(t, dir, Options{Thunks: map[string]bool{"M.x": true}})["M_alder.go"])
	if !strings.Contains(got, "func M__x() int64 {") {
		t.Errorf("x is not a thunk:\n%s", got)
	}
	if !regexp.MustCompile(`var M__y int64 = M__x\(\) \+`).MatchString(got) {
		t.Errorf("y is not a variable calling x:\n%s", got)
	}
}

// NoFormat leaves the module files unformatted but valid Go that gofmt turns into what the
// formatted emit holds; app__alder.go is the same either way.
func TestEmitNoFormat(t *testing.T) {
	dir := "../../testdata/golden/gen_basic"
	plain := emitOpts(t, dir, Options{})
	raw := emitOpts(t, dir, Options{NoFormat: true})
	if !bytes.Equal(plain[appFile], raw[appFile]) {
		t.Errorf("%s differs with NoFormat", appFile)
	}
	differs := false
	for name, src := range raw {
		if !strings.HasSuffix(name, "_alder.go") || name == appFile || name == "routes__alder.go" {
			continue
		}
		if _, err := parser.ParseFile(token.NewFileSet(), name, src, parser.SkipObjectResolution); err != nil {
			t.Errorf("%s does not parse: %v", name, err)
			continue
		}
		got, err := format.Source(src)
		if err != nil {
			t.Fatal(err)
		}
		got, err = format.Source(got)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(got, plain[name]) {
			t.Errorf("%s: formatting the NoFormat output is not the formatted emit", name)
		}
		differs = differs || !bytes.Equal(src, plain[name])
	}
	if !differs {
		t.Error("NoFormat changed no module file")
	}
}

// Query.redirect from a load (#8): a 303 to the page's URL as href renders it; under Datastar a
// page of this app is followed and pushed, another site navigated to; a file page's redirect is a
// plain 303; Url.external of an unsafe URL is "#", as for Redirect; Query.redirect as a value
// compiles.
func TestGen_QueryRedirect(t *testing.T) {
	files := emitSource(t, map[string]string{
		"App.alder": appModule,
		"Builder.alder": `module Builder

route Builder = GET "/builder/{seed}" { seed: Text, tab: Text = "main" }

Builder.load : Builder.Params -> Session -> Query Text
Builder.load params session = Query.succeed params.seed

Builder.view : Text -> Session -> Html
Builder.view seed session = <html><head><title>Builder</title></head><body><p>builder {seed}</p></body></html>
`,
		"Start.alder": `module Start

route Start = GET "/start" { go: Text = "", to: Text = "" }

Start.load : Start.Params -> Session -> Query Int
Start.load params session =
  if params.go == "builder" then Query.redirect (Builder.Builder { seed: "7", tab: "main" })
  else if params.go == "out" then Query.redirect (Url.external params.to)
  else if params.go == "value" then Maybe.withDefault (Query.succeed 1) (Maybe.map Query.redirect (Just (Builder.Builder { seed: "a b", tab: "x" })))
  else Query.succeed 1

Start.view : Int -> Session -> Html
Start.view n session = <p>start</p>
`,
		"Go.alder": `module Go

route Go = GET "/go.txt" { }

Go.load : Go.Params -> Session -> Query Int
Go.load params session = Query.redirect (Builder.Builder { seed: "9", tab: "main" })

Go.view : Int -> Session -> File
Go.view n session = File.text { contentType: "text/plain", body: "never" }
`,
	})
	contains(t, files["Start_alder.go"], `eff.Redirect[`)
	contains(t, files["Start_alder.go"], `rt.PageURL(`)
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
	do := func(target string, ds bool) *httptest.ResponseRecorder {
		r := httptest.NewRequest("GET", target, nil)
		if ds {
			r.Header.Set("Datastar-Request", "true")
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w
	}
	if w := do("/start?go=builder", false); w.Code != 303 || w.Header().Get("Location") != "/builder/7" || w.Body.Len() != 0 {
		t.Fatalf("plain: %d %q %q", w.Code, w.Header().Get("Location"), w.Body.String())
	}
	if w := do("/start?go=value", false); w.Code != 303 || w.Header().Get("Location") != "/builder/a%20b?tab=x" {
		t.Fatalf("as a value: %d %q", w.Code, w.Header().Get("Location"))
	}
	if w := do("/start", false); w.Code != 200 || !strings.Contains(w.Body.String(), "<p>start</p>") {
		t.Fatalf("no redirect: %d %s", w.Code, w.Body.String())
	}
	w := do("/start?go=builder", true)
	if w.Code != 200 || !strings.Contains(w.Body.String(), "builder 7") || !strings.Contains(w.Body.String(), "history.pushState") || !strings.Contains(w.Body.String(), `+"`"+`"/builder/7"`+"`"+`) {
		t.Fatalf("Datastar: %d %s", w.Code, w.Body.String())
	}
	if w := do("/start?go=out&to="+url.QueryEscape("https://ex.com/x"), false); w.Code != 303 || w.Header().Get("Location") != "https://ex.com/x" {
		t.Fatalf("external: %d %q", w.Code, w.Header().Get("Location"))
	}
	if w := do("/start?go=out&to="+url.QueryEscape("https://ex.com/x"), true); w.Code != 200 || !strings.Contains(w.Body.String(), `+"`"+`window.location.assign("https://ex.com/x")`+"`"+`) {
		t.Fatalf("external under Datastar: %d %s", w.Code, w.Body.String())
	}
	// "#" (an unsafe URL Url.external neutralised) or "#frag" would send a GET back to itself in
	// a loop: refused with a 500, under Datastar too
	for _, to := range []string{"javascript:alert(1)", "#frag"} {
		for _, ds := range []bool{false, true} {
			w := do("/start?go=out&to="+url.QueryEscape(to), ds)
			if w.Code != 500 && w.Header().Get("Alder-Status") != "500" || w.Header().Get("Location") != "" || !strings.Contains(w.Body.String(), "refusing to navigate") {
				t.Fatalf("unsafe %q (Datastar %v): %d %q %s", to, ds, w.Code, w.Header().Get("Location"), w.Body.String())
			}
		}
	}
	if w := do("/go.txt", false); w.Code != 303 || w.Header().Get("Location") != "/builder/9" {
		t.Fatalf("file page: %d %q", w.Code, w.Header().Get("Location"))
	}
	// under Datastar a file page is a real navigation before its load runs; that GET is the 303
	if w := do("/go.txt", true); !strings.Contains(w.Body.String(), `+"`"+`window.location.assign("/go.txt")`+"`"+`) {
		t.Fatalf("file page under Datastar: %d %s", w.Code, w.Body.String())
	}
}
`)
}

// The Response builders and Url.external emit calls of rt/std's wrappers, and the program vets
// (their Go signatures are the types gen emits): `Response.redirect (Url.external "/x") |>
// Response.success "ok"` is a Response with one success toast.
func TestGen_ResponseBuilders(t *testing.T) {
	files := emitSource(t, map[string]string{
		"App.alder": "module App\n\nerror : Error -> Html\nerror err = <html><body></body></html>\n\nnotFound : Html\nnotFound = <html><body></body></html>\n",
		"R.alder": "module R\n\n" +
			"done : Response\ndone = Response.redirect (Url.external \"/x\") |> Response.success \"ok\"\n\n" +
			"again : Response\nagain = Response.refresh |> Response.toast \"a\" |> Response.warning \"b\" |> Response.error \"c\"\n\n" +
			"shown : Route -> Response\nshown r = Response.render r\n\n" +
			"bad : Response\nbad = Response.invalid \"no\"\n",
	})
	contains(t, files["R_alder.go"],
		`std.ResponseSuccess("ok", std.ResponseRedirect(std.UrlExternal("/x")))`,
		`std.ResponseError("c", std.ResponseWarning("b", std.ResponseToast("a", std.ResponseRefresh())))`,
		"func R__shown(r rt.Route) rt.Response {",
		`std.ResponseInvalid("no")`,
	)
	vet(t, files)
}

// ---- GET route handlers (#160 PR 3)

// routeHandlersApp is the app of the route handlers' golden: GET routes with and without
// `requires`, a role, a file route, and write routes (Task 3.3).
const routeHandlersApp = "../../testdata/golden/gen_route_handlers"

// runRouteApp runs body, the statements of a Go test function, against the generated
// routeHandlersApp served with a database file: in scope are h (Serve's handler), uid (a user)
// and todo (a todo of uid's titled "Milk"), get (a GET of target with a cookie, "" for none, and
// headers as name, value pairs), send (a request of method with a url-encoded form body, the
// same way), ds (the headers of a Datastar post from the page "/": Datastar-Request, Alder-Page
// and a stale Alder-Shown), ran (how many times an effect step named so has run, counted from the
// app's log) and as (the signed session cookie of uid with a role).
func runRouteApp(t *testing.T, body string) {
	t.Helper()
	runRouteAppIn(t, routeHandlersApp, body)
}

// runRouteAppIn is runRouteApp for the app in dir (the golden's, or a copy of it).
func runRouteAppIn(t *testing.T, dir, body string) {
	t.Helper()
	runGo(t, emit(t, dir), `package app

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"alder/rt"
	"alder/rt/db"
	"alder/rt/eff"
	"alder/rt/web"
)

var _ = strings.Contains
var _ = http.StatusOK
var _ url.Values

func TestRoute(t *testing.T) {
	secret := []byte("0123456789abcdef0123456789abcdef")
	path := filepath.Join(t.TempDir(), "app.db")
	var mu sync.Mutex
	steps := map[string]int{}
	logged := func(level, msg string) {
		if level == "effect" {
			mu.Lock()
			steps[msg]++
			mu.Unlock()
		}
	}
	ran := func(step string) int {
		mu.Lock()
		defer mu.Unlock()
		return steps[step]
	}
	h, err := Serve(Caps{}, Options{DB: path, Secret: secret, Log: logged, LogID: func(_, level, msg string) { logged(level, msg) }})
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	store, err := db.OpenSQLite(path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	env := &eff.Env{Ctx: ctx, Store: store, Caps: map[string]any{}, Now: time.Now, Log: func(string, string) {}}
	uid, err := eff.Run(env, eff.KTask, eff.Call("Db.insert User", true, func(e *eff.Env) (string, error) {
		return db.InsertRow(e.Ctx, e.Store.(db.Store), db.TxOf(e), table__Table__User__User,
			insertvals__Table__User__User(insert__Table__User__User{Email: "a@b.c"}), e.Now())
	}))
	if err != nil {
		t.Fatal(err)
	}
	todo, err := eff.Run(env, eff.KTask, eff.Call("Db.insert Todo", true, func(e *eff.Env) (string, error) {
		return db.InsertRow(e.Ctx, e.Store.(db.Store), db.TxOf(e), table__Table__Todo__Todo,
			insertvals__Table__Todo__Todo(insert__Table__Todo__Todo{Title: "Milk", Due: rt.Nothing[rt.Date](), Owner: uid}), e.Now())
	}))
	if err != nil {
		t.Fatal(err)
	}
	as := func(role Table__User__Role) string {
		c, err := web.SessionCookie(secret, session__codec{}, App__Session{User: rt.Just(uid), Theme: "dark", Role: role}, time.Now())
		if err != nil {
			t.Fatal(err)
		}
		return web.CookieName + "=" + c
	}
	get := func(target, cookie string, headers ...string) *httptest.ResponseRecorder {
		r := httptest.NewRequest("GET", target, nil)
		if cookie != "" {
			r.Header.Set("Cookie", cookie)
		}
		for i := 0; i+1 < len(headers); i += 2 {
			r.Header.Set(headers[i], headers[i+1])
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w
	}
	send := func(method, target, cookie string, form url.Values, headers ...string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, target, strings.NewReader(form.Encode()))
		r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		if cookie != "" {
			r.Header.Set("Cookie", cookie)
		}
		for i := 0; i+1 < len(headers); i += 2 {
			r.Header.Set(headers[i], headers[i+1])
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w
	}
	ds := []string{"Datastar-Request", "true", "Alder-Page", "/", "Alder-Shown", "0123456789abcdef"}
	_, _, _, _, _, _ = as, get, todo, send, ds, ran
`+body+`
}
`)
}

// A GET route answers a plain request with its view as a document: the route's load and view
// with the narrowed session (requires user), a route without `requires` with App's session,
// links to routes rendered as their URLs, a parameter at its default left out of them.
func TestGenRoute_GetDocument(t *testing.T) {
	t.Parallel()
	runRouteApp(t, `
	me := as(Table__User__Role__Reader__)
	w := get("/todos/"+todo, me)
	body := w.Body.String()
	if w.Code != http.StatusOK || !strings.HasPrefix(body, "<!doctype html>") || !strings.Contains(body, "<li id=\"todo-"+todo+"\">") || !strings.Contains(body, "Milk") {
		t.Fatalf("GET /todos/id: %d %q", w.Code, body)
	}
	w = get("/", me)
	body = w.Body.String()
	for _, s := range []string{"<title>Todos</title>", "<body class=\"dark\">", "<li id=\"todo-"+todo+"\">", "href=\"/?filter=Done\"", "href=\"/about\"", "action=\"/todos\"", "<a href=\"/export.csv\" data-alder-reload=\"true\">CSV</a>"} {
		if !strings.Contains(body, s) {
			t.Errorf("GET /: no %q in %q", s, body)
		}
	}
	if w.Code != http.StatusOK || strings.Contains(body, "href=\"/?filter=All\"") {
		t.Fatalf("GET /: %d %q", w.Code, body)
	}
	if w := get("/?filter=Done", me); w.Code != http.StatusOK || strings.Contains(w.Body.String(), "Milk") {
		t.Fatalf("GET /?filter=Done: %d %q", w.Code, w.Body.String())
	}
	if w := get("/about?tab=jobs", ""); w.Code != http.StatusOK || !strings.Contains(w.Body.String(), "About: jobs (light)") {
		t.Fatalf("GET /about: %d %q", w.Code, w.Body.String())
	}
	`)
}

// An element fetch (Datastar, Alder-Fetch: 1) of a GET route is its view as one patch by its
// root's id: no document, no body morph, no history step.
func TestGenRoute_GetElement(t *testing.T) {
	t.Parallel()
	runRouteApp(t, `
	w := get("/todos/"+todo, as(Table__User__Role__Reader__), "Datastar-Request", "true", "Alder-Fetch", "1")
	body := w.Body.String()
	if w.Code != http.StatusOK || strings.Count(body, "event: datastar-patch-elements") != 1 || !strings.Contains(body, "data: selector #todo-"+todo+"\n") || !strings.Contains(body, "data: mode outer\n") {
		t.Fatalf("element fetch: %d %q", w.Code, body)
	}
	if !strings.Contains(body, "Milk") || strings.Contains(body, "<html") || strings.Contains(body, "<body") || strings.Contains(body, "history.") {
		t.Fatalf("element fetch: %q", body)
	}
	`)
}

// A GET route whose view is a File answers it raw: its content type and body, never a page.
func TestGenRoute_File(t *testing.T) {
	t.Parallel()
	runRouteApp(t, `
	w := get("/export.csv", as(Table__User__Role__Reader__))
	if w.Code != http.StatusOK || w.Header().Get("Content-Type") != "text/csv" || w.Body.String() != "Milk" {
		t.Fatalf("GET /export.csv: %d %q %q", w.Code, w.Header().Get("Content-Type"), w.Body.String())
	}
	`)
}

// A GET route's guard: without the session field it requires, a 303 to App.login; a role it
// does not name, a 404 (as a missing row); parameters that do not decode, a 404; a load that
// finds no row, a 404.
func TestGenRoute_Requires(t *testing.T) {
	t.Parallel()
	runRouteApp(t, `
	for _, target := range []string{"/", "/todos/"+todo, "/export.csv", "/admin/stats"} {
		if w := get(target, ""); w.Code != http.StatusSeeOther || w.Header().Get("Location") != "/login" {
			t.Errorf("GET %s without a user: %d %q", target, w.Code, w.Header().Get("Location"))
		}
	}
	if w := get("/admin/stats", as(Table__User__Role__Reader__)); w.Code != http.StatusNotFound {
		t.Errorf("GET /admin/stats as a reader: %d", w.Code)
	}
	if w := get("/admin/stats", as(Table__User__Role__Admin__)); w.Code != http.StatusOK || !strings.Contains(w.Body.String(), "42 todos") {
		t.Errorf("GET /admin/stats as an admin: %d %q", w.Code, w.Body.String())
	}
	if w := get("/?filter=Bogus", as(Table__User__Role__Reader__)); w.Code != http.StatusNotFound {
		t.Errorf("GET /?filter=Bogus: %d", w.Code)
	}
	if w := get("/todos/nope", as(Table__User__Role__Reader__)); w.Code != http.StatusNotFound || !strings.Contains(w.Body.String(), "Not found") {
		t.Errorf("GET /todos/nope: %d %q", w.Code, w.Body.String())
	}
	`)
}

// copyRouteApp copies the route handlers' golden app to a temp dir, with edit applied to the
// text of app/Todos.alder.
func copyRouteApp(t *testing.T, edit func(string) string) string {
	t.Helper()
	dst := t.TempDir()
	err := filepath.WalkDir(routeHandlersApp+"/app", func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(routeHandlersApp, p)
		if d.IsDir() {
			return os.MkdirAll(filepath.Join(dst, rel), 0o755)
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		if rel == filepath.Join("app", "Todos.alder") {
			b = []byte(edit(string(b)))
		}
		return os.WriteFile(filepath.Join(dst, rel), b, 0o644)
	})
	if err != nil {
		t.Fatal(err)
	}
	return dst
}

// A write route's guard runs before anything else (#160 final review, M1): without the session
// field it requires, a 303 to App.login; as a role it does not name, a 404; and in neither case
// does the update run (the row is not toggled, not deleted, nothing loaded).
func TestGenRoute_WriteRequires(t *testing.T) {
	t.Parallel()
	dir := copyRouteApp(t, func(s string) string {
		old := `route Toggle = PATCH  "/todos/{id}/toggle" { id: Todo.TodoId }           requires user`
		if !strings.Contains(s, old) {
			t.Fatal("Toggle route not found in the fixture")
		}
		s = strings.Replace(s, old, old+", role Admin", 1)
		old = `route Delete = DELETE "/todos/{id}"        { id: Todo.TodoId }           requires user`
		if !strings.Contains(s, old) {
			t.Fatal("Delete route not found in the fixture")
		}
		return strings.Replace(s, old, old+", role Admin", 1)
	})
	runRouteAppIn(t, dir, `
	toggle, del := "/todos/"+todo+"/toggle", "/todos/"+todo
	for _, c := range []struct{ method, target string }{{"PATCH", toggle}, {"DELETE", del}} {
		if w := send(c.method, c.target, "", nil); w.Code != http.StatusSeeOther || w.Header().Get("Location") != "/login" {
			t.Errorf("%s %s without a user: %d %q", c.method, c.target, w.Code, w.Header().Get("Location"))
		}
		if w := send(c.method, c.target, as(Table__User__Role__Reader__), nil); w.Code != http.StatusNotFound {
			t.Errorf("%s %s as a reader: %d %q", c.method, c.target, w.Code, w.Body.String())
		}
		// Under Datastar the same answers are delivered as a navigation and a not-found body.
		if w := send(c.method, c.target, "", nil, ds...); strings.Contains(w.Body.String(), "event: datastar-patch-elements") && !strings.Contains(w.Body.String(), "/login") {
			t.Errorf("%s %s without a user under Datastar: %d %q", c.method, c.target, w.Code, w.Body.String())
		}
		if w := send(c.method, c.target, as(Table__User__Role__Reader__), nil, ds...); !strings.Contains(w.Body.String(), "Not found") || strings.Contains(w.Body.String(), "Milk") {
			t.Errorf("%s %s as a reader under Datastar: %d %q", c.method, c.target, w.Code, w.Body.String())
		}
	}
	if w := send("POST", toggle, as(Table__User__Role__Reader__), url.Values{"_method": {"PATCH"}}); w.Code != http.StatusNotFound {
		t.Errorf("no-JS toggle as a reader: %d", w.Code)
	}
	if n := ran("Db.getMine Todo"); n != 0 {
		t.Errorf("the update ran (%d row loads)", n)
	}
	if w := get("/?filter=Done", as(Table__User__Role__Admin__)); strings.Contains(w.Body.String(), "Milk") {
		t.Errorf("the row was toggled: %q", w.Body.String())
	}
	if w := get("/todos/"+todo, as(Table__User__Role__Admin__)); w.Code != http.StatusOK || !strings.Contains(w.Body.String(), "Milk") {
		t.Errorf("the row was deleted: %d %q", w.Code, w.Body.String())
	}
	if w := send("PATCH", toggle, as(Table__User__Role__Admin__), nil, ds...); w.Code != http.StatusOK || ran("Db.getMine Todo") == 0 {
		t.Errorf("toggle as an admin: %d, row loads %d", w.Code, ran("Db.getMine Todo"))
	}
	`)
}

// An element fetch whose route answers 404 (its load finds no row, its parameters do not
// decode) patches nothing and toasts "Not found", as a `render` of a row that is gone does.
func TestGenRoute_ElementNotFound(t *testing.T) {
	t.Parallel()
	runRouteApp(t, `
	me := as(Table__User__Role__Reader__)
	for _, target := range []string{"/todos/nope", "/?filter=Bogus"} {
		w := get(target, me, "Datastar-Request", "true", "Alder-Fetch", "1")
		body := w.Body.String()
		if w.Code != http.StatusOK || strings.Count(body, "event: datastar-patch-elements") != 1 || !strings.Contains(body, "data: selector al-toasts\n") || !strings.Contains(body, ">Not found</al-toast>") {
			t.Errorf("element fetch of %s: %d %q", target, w.Code, body)
		}
	}
	`)
}

// The guard of GET routes in an app with silos (silo-routes-ok): `requires org` narrows the
// store to the session's org and, without one, redirects to the GET route `choose org` names;
// the admin route's `silo org` narrows to its parameter and checks the role. The app vets.
func TestGenRoute_Silos(t *testing.T) {
	t.Parallel()
	files := emit(t, "../../testdata/corpus/silo-routes-ok")
	contains(t, files["routes__alder.go"],
		`func Route__Projects__Index() web.Route {`,
		`r := web.RedirectResponse("/orgs", keep)`,
		`db.Narrow(rq.Env, "org", app.Org.Val)`,
		`func Route__Admin__Projects__Manage() web.Route {`,
		`if app.Role.Tag != is__User__Role__Admin {`,
		`db.Narrow(rq.Env, "org", params.Org)`,
	)
	vet(t, files)
}

// ---- write route handlers (#160 PR 3)

// A write route answering `render r` under Datastar: one patch of r's view by its root id, mode
// outer, then its toasts; the page the browser is on is neither loaded nor rendered (Index.load's
// Db.all never runs). Without JavaScript a POST with _method=PATCH is the route too, answered
// with a 303 back to the referring page.
func TestGenRoute_ToggleRender(t *testing.T) {
	t.Parallel()
	runRouteApp(t, `
	me := as(Table__User__Role__Reader__)
	w := send("PATCH", "/todos/"+todo+"/toggle", me, nil, ds...)
	body := w.Body.String()
	if w.Code != http.StatusOK || strings.Count(body, "event: datastar-patch-elements") != 1 || !strings.Contains(body, "data: selector #todo-"+todo+"\n") || !strings.Contains(body, "data: mode outer\n") || strings.Contains(body, "selector body") {
		t.Fatalf("PATCH toggle: %d %q", w.Code, body)
	}
	if n := ran("Db.all Todo"); n != 0 {
		t.Fatalf("the page's load ran %d times", n)
	}
	if w := get("/?filter=Done", me); !strings.Contains(w.Body.String(), "Milk") {
		t.Fatalf("not toggled: %q", w.Body.String())
	}
	w = send("POST", "/todos/"+todo+"/toggle", me, url.Values{"_method": {"PATCH"}}, "Referer", "http://example.com/?filter=Done")
	if w.Code != http.StatusSeeOther || w.Header().Get("Location") != "/?filter=Done" {
		t.Fatalf("no-JS toggle: %d %q", w.Code, w.Header().Get("Location"))
	}
	if w := get("/?filter=Done", me); strings.Contains(w.Body.String(), "Milk") {
		t.Fatalf("not toggled back: %q", w.Body.String())
	}
	`)
}

// A write route answering `refresh`: the page the browser is on rendered before the update and
// after it, and only the regions that changed sent (#todos), never the whole body.
func TestGenRoute_DeleteRefresh(t *testing.T) {
	t.Parallel()
	runRouteApp(t, `
	me := as(Table__User__Role__Reader__)
	w := send("DELETE", "/todos/"+todo, me, nil, ds...)
	body := w.Body.String()
	if w.Code != http.StatusOK || !strings.Contains(body, "data: selector #todos\n") || strings.Contains(body, "selector body") || strings.Contains(body, "Milk") {
		t.Fatalf("DELETE: %d %q", w.Code, body)
	}
	if n := ran("Db.all Todo"); n != 2 {
		t.Fatalf("the page's load ran %d times, want 2 (before and after)", n)
	}
	`)
}

// A write route whose update cannot answer refresh or invalid (Toggle: render only) renders no
// before-view: its handler never calls RenderShown, so the page the browser is on is not run.
// One that can (Delete: refresh) does.
func TestGenRoute_NoBeforeWhenUnreachable(t *testing.T) {
	t.Parallel()
	src := string(emit(t, routeHandlersApp)["routes__alder.go"])
	toggle, del := goFunc(t, src, "Route__Todos__Toggle"), goFunc(t, src, "Route__Todos__Delete")
	if strings.Contains(toggle, "RenderShown") || !strings.Contains(del, "RenderShown") {
		t.Fatalf("Toggle:\n%s\nDelete:\n%s", toggle, del)
	}
	runRouteApp(t, `
	w := send("PATCH", "/todos/"+todo+"/toggle", as(Table__User__Role__Reader__), nil, ds...)
	if w.Code != http.StatusOK || ran("Db.all Todo") != 0 || ran("Db.getMine Todo") != 1 {
		t.Fatalf("%d, page loads %d, row loads %d", w.Code, ran("Db.all Todo"), ran("Db.getMine Todo"))
	}
	`)
}

// A form that does not decode (a malformed date) is answered as `invalid` with the field errors:
// 422, the page the browser is on rendered again with the posted values and the errors, and the
// update not called (no row inserted). Without JavaScript the same page as a 422 document.
func TestGenRoute_FormDecodeFails(t *testing.T) {
	t.Parallel()
	runRouteApp(t, `
	me := as(Table__User__Role__Reader__)
	bad := url.Values{"title": {"Bread"}, "due": {"soon"}}
	w := send("POST", "/todos", me, bad, ds...)
	body := w.Body.String()
	if w.Code != http.StatusOK || w.Header().Get(web.StatusHeader) != "422" || !strings.Contains(body, web.MsgDate) || !strings.Contains(body, "value=\"Bread\"") {
		t.Fatalf("POST /todos: %d %q %q", w.Code, w.Header().Get(web.StatusHeader), body)
	}
	if ran("Db.insertMine Todo") != 0 {
		t.Fatal("update ran")
	}
	w = send("POST", "/todos", me, bad, "Referer", "http://example.com/")
	if w.Code != http.StatusUnprocessableEntity || !strings.Contains(w.Body.String(), web.MsgDate) || !strings.Contains(w.Body.String(), "<!doctype html>") {
		t.Fatalf("no-JS POST /todos: %d %q", w.Code, w.Body.String())
	}
	if w := get("/", me); strings.Contains(w.Body.String(), "Bread") || ran("Db.insertMine Todo") != 0 {
		t.Fatalf("a row was inserted: %q", w.Body.String())
	}
	`)
}

// The options check of a form posted from a page (plan call 11): a value the page's select did
// not offer is a 422 with the field error on the row's form, update not called; an offered one
// (the row's own title included) runs it. Posted from a page that shows no such select (or from
// another app's page), a 400.
func TestGenRoute_OptionsFromShownPage(t *testing.T) {
	t.Parallel()
	runRouteApp(t, `
	me := as(Table__User__Role__Reader__)
	move := "/todos/"+todo+"/move"
	w := send("POST", move, me, url.Values{"list": {"bogus"}}, ds...)
	body := w.Body.String()
	if w.Code != http.StatusOK || w.Header().Get(web.StatusHeader) != "422" || !strings.Contains(body, web.MsgOption) || strings.Contains(body, "Moved to") {
		t.Fatalf("forged option: %d %q %q", w.Code, w.Header().Get(web.StatusHeader), body)
	}
	for _, list := range []string{"home", "Milk"} {
		w = send("POST", move, me, url.Values{"list": {list}}, ds...)
		body = w.Body.String()
		if w.Code != http.StatusOK || w.Header().Get(web.StatusHeader) != "" || !strings.Contains(body, "data: selector #todo-"+todo+"\n") || !strings.Contains(body, "Moved to "+list) {
			t.Fatalf("option %s: %d %q", list, w.Code, body)
		}
	}
	w = send("POST", move, me, url.Values{"list": {"home"}}, "Datastar-Request", "true", "Alder-Page", "/about")
	if w.Header().Get(web.StatusHeader) != "400" || strings.Contains(w.Body.String(), "Moved to") {
		t.Fatalf("from /about: %d %q %q", w.Code, w.Header().Get(web.StatusHeader), w.Body.String())
	}
	w = send("POST", move, me, url.Values{"list": {"home"}}, "Referer", "http://example.com/about")
	if w.Code != http.StatusBadRequest {
		t.Fatalf("no-JS from /about: %d %q", w.Code, w.Body.String())
	}
	// Neither Alder-Page nor Referer: the page is "/", which shows the row's select, so a
	// forged option is a 422 and an offered one runs the update.
	w = send("POST", move, me, url.Values{"list": {"bogus"}}, "Datastar-Request", "true")
	if w.Header().Get(web.StatusHeader) != "422" || !strings.Contains(w.Body.String(), web.MsgOption) || strings.Contains(w.Body.String(), "Moved to") {
		t.Fatalf("no page header, forged option: %d %q %q", w.Code, w.Header().Get(web.StatusHeader), w.Body.String())
	}
	w = send("POST", move, me, url.Values{"list": {"home"}}, "Datastar-Request", "true")
	if w.Code != http.StatusOK || w.Header().Get(web.StatusHeader) != "" || !strings.Contains(w.Body.String(), "Moved to home") {
		t.Fatalf("no page header, offered option: %d %q %q", w.Code, w.Header().Get(web.StatusHeader), w.Body.String())
	}
	w = send("POST", move, me, url.Values{"list": {"bogus"}})
	if w.Code != http.StatusUnprocessableEntity || !strings.Contains(w.Body.String(), web.MsgOption) {
		t.Fatalf("no JS, no Referer, forged option: %d %q", w.Code, w.Body.String())
	}
	`)
}

// Response.invalid is a committed success: a row inserted before it is kept, and the page the
// browser is on is rendered again with the posted values and the text on the form (its state
// keyed to the route: FormState.Msg is the route's rt.RouteKey).
func TestGenRoute_InvalidCommits(t *testing.T) {
	t.Parallel()
	runRouteApp(t, `
	me := as(Table__User__Role__Reader__)
	w := send("POST", "/todos", me, url.Values{"title": {"Again"}}, ds...)
	body := w.Body.String()
	if w.Code != http.StatusOK || w.Header().Get(web.StatusHeader) != "422" || !strings.Contains(body, "Already there") || !strings.Contains(body, "value=\"Again\"") {
		t.Fatalf("POST /todos: %d %q %q", w.Code, w.Header().Get(web.StatusHeader), body)
	}
	if w := get("/", me); !strings.Contains(w.Body.String(), ">Again</button>") {
		t.Fatalf("the row is not kept: %q", w.Body.String())
	}
	w = send("POST", "/todos", me, url.Values{"title": {"Again"}}, "Referer", "http://example.com/")
	if w.Code != http.StatusUnprocessableEntity || !strings.Contains(w.Body.String(), "Already there") {
		t.Fatalf("no-JS: %d %q", w.Code, w.Body.String())
	}
	w = send("POST", "/todos", me, url.Values{"title": {"Bread"}}, "Referer", "http://example.com/")
	if w.Code != http.StatusSeeOther || w.Header().Get("Location") != "/" {
		t.Fatalf("redirect: %d %q", w.Code, w.Header().Get("Location"))
	}
	`)
}

// goFunc is the source of Go function name in src, up to the next top-level declaration.
func goFunc(t *testing.T, src, name string) string {
	t.Helper()
	i := strings.Index(src, "func "+name+"(")
	if i < 0 {
		t.Fatalf("no func %s in\n%s", name, src)
	}
	rest := src[i:]
	if j := strings.Index(rest, "\n}\n"); j >= 0 {
		return rest[:j+3]
	}
	return rest
}

// routeFormApp is a route app with a GET route whose parameter is named `form` (not a submitted
// form: it is part of the route's URL) and a PATCH route with `form Entry` sent from a bare
// `submit` attribute on an element that is not a <form>.
var routeFormApp = map[string]string{
	"App.alder": "module App\n\nsession { theme: Text = \"light\" }\n\nerror : Error -> Html\nerror err = <p>error</p>\n\nnotFound : Html\nnotFound = <p>not found</p>\n",
	"Search.alder": `module Search

route Find = GET   "/find"      { form: Text = "" }
route Save = PATCH "/save/{id}" { id: Text } form Entry

form Entry = { list: Text }

Find.load : Find.Params -> Session -> Query Text
Find.load params session = Query.succeed params.form

Find.view : Text -> Session -> Html
Find.view q session =
  <div id="find">
    <a href={Find { form: "x" }}>again</a>
    <form submit={Save { id: "a" }}><button>Save</button></form>
    <div submit={Save { id: "b" }}>bare</div>
  </div>

Save.update : Save.Params -> Session -> Task Response
Save.update params session = Task.succeed (Response.render (Find { form: params.form.list }))
`,
}

// A route's URL leaves out the `form` field only of a route with `form F` (its submitted form):
// a GET route's parameter named form is in its links (ruling R9). A <form> posting to a PATCH
// route carries the _method override (html.FormToRoute); a bare `submit` on another element is
// RouteAttr only, with no override (it never submits without JavaScript).
func TestGenRoute_FormParamAndOverride(t *testing.T) {
	t.Parallel()
	files := emitSource(t, routeFormApp)
	contains(t, files["Search_alder.go"],
		`rt.Route{Name: "Search.Find", Method: "GET", Path: "/find", Query: []rt.KV{rt.QueryKV("form", "x", "")}}`,
		`html.FormToRoute(rt.Route{Name: "Search.Save", Method: "PATCH", Path: rt.Concat("/save/", rt.PathSegment("a"))}`,
		`html.RouteAttr("data-on:submit", rt.Route{Name: "Search.Save", Method: "PATCH", Path: rt.Concat("/save/", rt.PathSegment("b"))})`,
	)
	vet(t, files)
}
