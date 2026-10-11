package check

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"alder/internal/diag"
	"alder/internal/names"
	"alder/internal/syntax"
	"alder/internal/types"
)

var update = flag.Bool("update", false, "rewrite golden files")

func runDir(t *testing.T, dir string) *diag.Bag {
	_, bag := runDirResult(t, dir)
	return bag
}

// runDirResult checks dir and returns the checker's Result (nil when parsing failed) with the diagnostics.
func runDirResult(t *testing.T, dir string) (*Result, *diag.Bag) {
	_, res, bag := runDirProg(t, dir)
	return res, bag
}

// runDirProg is runDirResult that also returns the loaded program.
func runDirProg(t *testing.T, dir string) (*names.Program, *Result, *diag.Bag) {
	std, err := types.LoadStd()
	if err != nil {
		t.Fatal(err)
	}
	prog, bag, ioErrs := names.Load(dir, std)
	for _, e := range ioErrs {
		t.Fatal(e)
	}
	var res *Result
	if !prog.ParseFailed { // Load's bag also holds module-graph errors (cycles), which only gate their modules
		prog.Resolve(bag)
		prog.MarkFailed(bag)
		res = Program(prog, bag)
	}
	bag.Sort()
	return prog, res, bag
}

func render(bag *diag.Bag) string {
	var sb strings.Builder
	for _, d := range bag.Items {
		if d.Severity != "error" {
			continue
		}
		fmt.Fprintf(&sb, "%s:%d:%d %s %s\n", d.Span.File, d.Span.Start.Line, d.Span.Start.Col, d.Code, d.Message)
		if d.Fix != nil {
			fmt.Fprintf(&sb, "  fix %d:%d-%d:%d %s\n", d.Fix.Line, d.Fix.Col, d.Fix.EndLine, d.Fix.EndCol, strings.ReplaceAll(d.Fix.Replace, "\n", "⏎"))
		}
	}
	return sb.String()
}

func TestCheckGolden(t *testing.T) {
	dirs, _ := filepath.Glob("../../testdata/golden/check_*")
	for _, dir := range dirs {
		if st, err := os.Stat(dir); err != nil || !st.IsDir() {
			continue
		}
		t.Run(filepath.Base(dir), func(t *testing.T) {
			got := render(runDir(t, dir))
			golden := dir + ".diag"
			if *update {
				os.WriteFile(golden, []byte(got), 0o644)
				return
			}
			want, err := os.ReadFile(golden)
			if err != nil {
				t.Fatalf("missing golden %s", golden)
			}
			if got != string(want) {
				t.Fatalf("golden mismatch\n got:\n%s\nwant:\n%s", got, want)
			}
		})
	}
}

func TestCorpus(t *testing.T) {
	dirs, _ := filepath.Glob("../../testdata/corpus/*")
	for _, dir := range dirs {
		t.Run(filepath.Base(dir), func(t *testing.T) {
			got := render(runDir(t, dir))
			if *update {
				if err := os.WriteFile(filepath.Join(dir, "expected.diag"), []byte(got), 0o644); err != nil {
					t.Fatal(err)
				}
				return
			}
			want, err := os.ReadFile(filepath.Join(dir, "expected.diag"))
			if err != nil {
				t.Fatal(err)
			}
			if got != string(want) {
				t.Fatalf("corpus mismatch\n got:\n%s\nwant:\n%s", got, want)
			}
		})
	}
}

func TestExamplesCheckClean(t *testing.T) {
	for _, ex := range []string{"../../examples/todos", "../../examples/blog"} {
		if got := render(runDir(t, ex)); got != "" {
			t.Fatalf("%s should check clean:\n%s", ex, got)
		}
	}
}

// TestFixesApply proves every corpus fix is well-formed: applying it to the source must not error
// and must change the file.
func TestFixesApply(t *testing.T) {
	dirs, _ := filepath.Glob("../../testdata/corpus/*")
	golden, _ := filepath.Glob("../../testdata/golden/check_*")
	for _, g := range golden {
		if st, err := os.Stat(g); err == nil && st.IsDir() {
			dirs = append(dirs, g)
		}
	}
	// outputs that must contain a given text after applying the case's fix
	want := map[string]string{
		"E0102-maybe-field/E0102":   "(match user { Just x -> x.name, Nothing -> ?default }) }",
		"check_fix_multiline/E0003": "|> List.length", // a multi-line span's fix must keep the whole expression
		"E0101-get-by-slug/E0101":   "Db.from Post |> Db.where (Post.slug.eq params.slug) |> Db.first\n",
	}
	for _, dir := range dirs {
		bag := runDir(t, dir)
		for _, d := range bag.Items {
			if d.Fix == nil {
				continue
			}
			src, err := os.ReadFile(filepath.Join(dir, d.Span.File))
			if err != nil {
				t.Fatal(err)
			}
			out, err := diag.Apply(string(src), []diag.Fix{*d.Fix})
			if err != nil {
				t.Errorf("%s %s: fix does not apply: %v", dir, d.Code, err)
			} else if out == string(src) {
				t.Errorf("%s %s: fix changes nothing", dir, d.Code)
			} else if _, pd := syntax.ParseFile(d.Span.File, []byte(out)); len(pd) > 0 && !strings.Contains(d.Fix.Replace, "?type") { // `?type` is a hole in type position, which the parser rejects by design
				t.Errorf("%s %s: fix must leave the file parseable: %v\n%s", dir, d.Code, pd[0].Message, out)
			} else if w, ok := want[filepath.Base(dir)+"/"+d.Code]; ok && !strings.Contains(out, w) {
				t.Errorf("%s %s: applied output lacks %q:\n%s", dir, d.Code, w, out)
			}
		}
	}
}

// W0003: a form field whose decoding errors no <Errors> of its form would show (M2). A field
// with <Errors field>, a form with <Errors form />, or a Bool field is not reported.
func TestUnshownErrorsWarning(t *testing.T) {
	dir := t.TempDir()
	files := map[string]string{
		"App.alder": "module App\n\nerror : Error -> Html Never\nerror err = <p>e</p>\n\nnotFound : Html Never\nnotFound = <p>nf</p>\n",
		"P.alder": `module P

page "/" { }

type Model = { }

form F = { title: Text, size: Int, done: Bool }

msg Add form F
msg Keep form F

load : Params -> Session -> Query Model
load params session = Query.succeed { }

update : Msg -> Model -> Session -> Task Nav
update msg model session = Task.succeed Rerender

view : Model -> Session -> Html Msg
view model session =
  <div>
    <form submit={Add}><Input field="title" /><Errors field="title" /><Input field="size" /><Input field="done" /></form>
    <form submit={Keep}><Errors form /><Input field="size" /></form>
  </div>
`,
	}
	if err := os.MkdirAll(filepath.Join(dir, "app"), 0o755); err != nil {
		t.Fatal(err)
	}
	for name, src := range files {
		if err := os.WriteFile(filepath.Join(dir, "app", name), []byte(src), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	var got []string
	for _, d := range runDir(t, dir).Items {
		got = append(got, fmt.Sprintf("%s %d:%d %s %s", d.Severity, d.Span.Start.Line, d.Span.Start.Col, d.Code, d.Message))
	}
	want := []string{"warning 21:5 W0003 form field `size` has no `<Errors field=\"size\">` and the form no `<Errors form />`: its decoding errors would not be shown"}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("got\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}

// checkModule checks src as the only module (its file named after its `module` line) of an app
// and returns the error diagnostics.
func checkModule(t *testing.T, src string) []diag.Diagnostic {
	t.Helper()
	dir := t.TempDir()
	name := strings.TrimSpace(strings.TrimPrefix(strings.SplitN(src, "\n", 2)[0], "module "))
	if err := os.MkdirAll(filepath.Join(dir, "app"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "app", name+".alder"), []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	var out []diag.Diagnostic
	for _, d := range runDir(t, dir).Items {
		if d.Severity == "error" {
			out = append(out, d)
		}
	}
	return out
}

// A `test` whose body is a block is a Task Bool; any other body is a Bool. Test.* is only
// available inside a test.
func TestTestBodies(t *testing.T) {
	cases := []struct{ name, src, want string }{
		{"bool", `test "t" = 1 == 1`, ""},
		{"task block", "test \"t\" = {\n  let r <- Test.run (Task.succeed 1)\n  Task.succeed (r.value == Ok 1)\n}", ""},
		{"trace", "test \"t\" = {\n  let tr <- Test.trace\n  Task.succeed (tr == [])\n}", ""},
		{"int", `test "t" = 1`, "E0003 test \"t\": expected Bool, got Int"},
		{"int call", "f : Int -> Int\nf n = n\n\ntest \"t\" = f 1", "E0003 test \"t\": expected Bool, got Int"},
		{"Test.run of a Query", "test \"t\" = {\n  let r <- Test.run (Query.succeed 1)\n  Task.succeed (r.value == Ok 1)\n}", "E0003 expected Task _, got Query Int; a Query runs inside a test's block: `let x <- q`; `Test.run` takes a Task"},
		{"query block", `test "t" = { Query.succeed True }`, "E0003"},
		{"task not block", `test "t" = Task.succeed True`, "E0003 expected Bool, got Task Bool; a test of a Task is written as a block"},
		{"pure block", "test \"t\" = {\n  let x = 1\n  x == 1\n}", "E0003 expected Task Bool, got Bool; a block test is a Task: end it with `Task.succeed (…)`"},
		{"nested task", `test "t" = if Task.succeed True then True else False`, "E0003 expected Bool, got Task Bool"},
		{"Test.run outside", "x : Task Int\nx = Task.map (\\r -> 1) (Test.run (Task.succeed 1))", "E0003"},
		{"Test.trace outside", "x : Task (List Text)\nx = Test.trace", "E0003"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			diags := checkModule(t, "module M\n\n"+c.src+"\n")
			var got []string
			for _, d := range diags {
				got = append(got, d.Code+" "+d.Message)
			}
			if c.want == "" && len(got) != 0 || c.want != "" && (len(got) != 1 || !strings.HasPrefix(got[0], c.want)) ||
				c.name == "nested task" && (strings.Contains(got[0], "written as a block") || strings.Contains(got[0], "test \"t\"")) ||
				c.name == "task not block" && strings.Contains(got[0], "test \"t\"") {
				t.Fatalf("got %q want one %q", got, c.want)
			}
		})
	}
}

// `let x : T <- e` whose e has the poison type binds x to its annotation T, so x's uses are
// still checked; without an annotation x is poisoned and its uses report nothing more.
func TestPoisonedBindKeepsAnnotation(t *testing.T) {
	src := func(ann string) string {
		return "module M\n\nx : Task Bool\nx = {\n  let r" + ann + " <- Test.run (Task.succeed 1)\n  Task.succeed (r == \"a\")\n}\n"
	}
	var got []string
	for _, d := range checkModule(t, src(" : Int")) {
		got = append(got, d.Code+" "+d.Message)
	}
	if len(got) != 2 || !strings.Contains(got[1], "cannot compare `Int` with `Text`") {
		t.Errorf("annotated: %q", got)
	}
	got = nil
	for _, d := range checkModule(t, src("")) {
		got = append(got, d.Code+" "+d.Message)
	}
	if len(got) != 1 {
		t.Errorf("unannotated: %q", got)
	}
}
