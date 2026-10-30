package check

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"alder/internal/diag"
	"alder/internal/names"
	"alder/internal/types"
)

// checkWith checks the app whose app/ holds files (name → source) with options o.
func checkWith(t *testing.T, files map[string]string, o Options) (*Result, *diag.Bag) {
	t.Helper()
	dir := t.TempDir()
	for name, src := range files {
		path := filepath.Join(dir, "app", name)
		os.MkdirAll(filepath.Dir(path), 0o755)
		if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	std, err := types.LoadStd()
	if err != nil {
		t.Fatal(err)
	}
	prog, bag, ioErrs := names.Load(dir, std)
	for _, e := range ioErrs {
		t.Fatal(e)
	}
	if prog.ParseFailed {
		t.Fatalf("parse failed: %v", bag.Items)
	}
	prog.Resolve(bag)
	prog.MarkFailed(bag)
	res := ProgramWith(prog, bag, o)
	bag.Sort()
	return res, bag
}

func TestInferTopLevel_ScratchOnly(t *testing.T) {
	files := map[string]string{
		"Lib.alder":     "module Lib\n\nlen = List.length [1]\n",
		"Scratch.alder": "module Scratch\n\nxs = [1, 2]\nnone = []\nid = \\x -> x\npair = { a: id 1, b: id \"x\", n: List.length xs }\nit = { l: none, d: Dict.empty }\n",
	}
	res, bag := checkWith(t, files, Options{InferModule: "Scratch"})
	var codes []string
	for _, d := range bag.Items {
		if d.Severity == "error" {
			codes = append(codes, d.Span.File+" "+d.Code)
		}
	}
	if len(codes) != 1 || codes[0] != "app/Lib.alder E0007" {
		t.Errorf("errors %v, want only Lib's E0007", codes)
	}
	want := map[string]string{
		"Scratch.xs":   "List Int",
		"Scratch.none": "List a",
		"Scratch.id":   "a -> a",
		"Scratch.pair": "{ a: Int, b: Text, n: Int }",
		"Scratch.it":   "{ l: List a, d: Dict b c }",
	}
	for k, w := range want {
		got, ok := res.Values[k]
		if !ok || types.String(res.U.Apply(got)) != w {
			t.Errorf("%s : %v, want %s", k, got, w)
		}
	}
	// without the option a sig-less Scratch value is E0007 as anywhere else
	_, bag = checkWith(t, files, Options{})
	n := 0
	for _, d := range bag.Items {
		if d.Code == diag.MissingSig {
			n++
		}
	}
	if n != 6 {
		t.Errorf("%d E0007 without InferModule, want 6:\n%s", n, render(bag))
	}
}

func TestInferTopLevel_NoCascade(t *testing.T) {
	files := map[string]string{
		"Scratch.alder": "module Scratch\n\na = 1 + \"x\"\nb = a\nf = \\n -> f n\ng = \\n -> g (n + h)\nh = 1\nk = \\x -> k\nit = b + 1\nj = g 1\nv = v + 1\n",
	}
	_, bag := checkWith(t, files, Options{InferModule: "Scratch"})
	var got []string
	for _, d := range bag.Items {
		if d.Severity == "error" {
			got = append(got, fmt.Sprintf("%d %s", d.Span.Start.Line, d.Code))
			if d.Fix != nil {
				t.Errorf("%s has a fix %q", d.Message, d.Fix.Replace)
			}
		}
	}
	// a's E0003; f uses itself (let-rec): fine; g's E0007 (it uses h, defined after it); k's
	// E0007 (it uses itself at another type); nothing for b, it and j, which only use a or g;
	// v's E0007 (only a function may use itself)
	if strings.Join(got, ", ") != "3 E0003, 6 E0007, 8 E0007, 11 E0007" {
		t.Errorf("errors %v\n%s", got, render(bag))
	}
}

// errorsOf lists bag's errors as "line code".
func errorsOf(bag *diag.Bag) string {
	var got []string
	for _, d := range bag.Items {
		if d.Severity == "error" {
			got = append(got, fmt.Sprintf("%d %s", d.Span.Start.Line, d.Code))
		}
	}
	return strings.Join(got, ", ")
}

func TestInferTopLevel_LetRec(t *testing.T) {
	files := map[string]string{
		"Scratch.alder": "module Scratch\n\nfact n = if n <= 1 then 1 else n * fact (n - 1)\nloop = \\n -> loop n\nit = fact 5\n",
	}
	res, bag := checkWith(t, files, Options{InferModule: "Scratch"})
	if got := errorsOf(bag); got != "" {
		t.Fatalf("errors %v\n%s", got, render(bag))
	}
	for k, w := range map[string]string{"Scratch.fact": "Int -> Int", "Scratch.loop": "a -> b", "Scratch.it": "Int"} {
		if got, ok := res.Values[k]; !ok || types.String(res.U.Apply(got)) != w {
			t.Errorf("%s : %v, want %s", k, got, w)
		}
	}
}

func TestInferTopLevel_ForwardReference(t *testing.T) {
	files := map[string]string{
		// g uses itself and h, defined after it: E0007 naming h; it uses g: no cascade
		"Scratch.alder": "module Scratch\n\ng n = if n then g n else h\nh = 2\nit = g True\n",
	}
	_, bag := checkWith(t, files, Options{InferModule: "Scratch"})
	if got := errorsOf(bag); got != "3 E0007" {
		t.Fatalf("errors %v\n%s", got, render(bag))
	}
	if m := bag.Items[0].Message; !strings.Contains(m, "`h`") || !strings.Contains(m, "defined after it") {
		t.Errorf("message %q", m)
	}
}

func TestInlineLambdaApplication(t *testing.T) {
	files := map[string]string{
		"Scratch.alder": "module Scratch\n\nit = (\\a b -> a + b) 1 2\nf = (\\s -> s ++ \"!\") \"x\"\n",
	}
	res, bag := checkWith(t, files, Options{InferModule: "Scratch"})
	if got := errorsOf(bag); got != "" {
		t.Fatalf("errors %v\n%s", got, render(bag))
	}
	for k, w := range map[string]string{"Scratch.it": "Int", "Scratch.f": "Text"} {
		if got, ok := res.Values[k]; !ok || types.String(res.U.Apply(got)) != w {
			t.Errorf("%s : %v, want %s", k, got, w)
		}
	}
}

func TestStdlibOpaqueTypeIsNamedQualified(t *testing.T) {
	files := map[string]string{
		"Lib.alder":     "module Lib\n\nfetch : Text -> Task (Result Http.HttpError Text)\nfetch u = Http.get { url: u }\n",
		"Scratch.alder": "module Scratch\n\nit = Http.get\n",
	}
	res, bag := checkWith(t, files, Options{InferModule: "Scratch"})
	if got := errorsOf(bag); got != "" {
		t.Fatalf("errors %v\n%s", got, render(bag))
	}
	// printed so that it can be pasted into a signature
	if got := types.String(res.U.Apply(res.Values["Scratch.it"])); got != "{ url: Text } -> Task (Result Http.HttpError Text)" {
		t.Errorf("it : %s", got)
	}
	if got := types.StringIn(res.U.Apply(res.Values["Scratch.it"]), "Http"); got != "{ url: Text } -> Task (Result HttpError Text)" {
		t.Errorf("in Http: %s", got)
	}
}

func TestInferModule_AnyNameAndBrands(t *testing.T) {
	files := map[string]string{
		"Lib.alder":   "module Lib\n\ntype Slug = brand Text\n",
		"Probe.alder": "module Probe\n\nimport Lib\n\nit = Slug \"x\"\nn = Text.NonEmpty \"y\"\n",
	}
	// the inferred module may have any name, and uses brand constructors as a test does
	res, bag := checkWith(t, files, Options{InferModule: "Probe"})
	if got := errorsOf(bag); got != "" {
		t.Fatalf("errors %v\n%s", got, render(bag))
	}
	if got := types.String(res.U.Apply(res.Values["Probe.it"])); got != "Lib.Slug" {
		t.Errorf("it : %s", got)
	}
	// no name is special: a module named Scratch is ordinary unless named
	files["Scratch.alder"] = "module Scratch\n\nx = 1\n"
	_, bag = checkWith(t, files, Options{InferModule: "Probe"})
	if got := errorsOf(bag); got != "3 E0007" {
		t.Errorf("Scratch without InferModule: %v\n%s", got, render(bag))
	}
}

// Only a lambda applied to exactly as many arguments as it has parameters takes the inline
// path; any other count is the ordinary application, whose arity error is E0008 (followed, for
// the curried lambda, by the E0003s it has always cascaded into: the arity design question
// of `(\a -> \b -> …) 1 2` is open, chapter 12).
func TestInlineLambdaArity(t *testing.T) {
	for _, src := range []string{"it = (\\a -> \\b -> a + b) 1 2\n", "it = (\\a -> a) 1 2\n"} {
		for _, c := range []struct {
			files map[string]string
			o     Options
			want  string
		}{
			{map[string]string{"Scratch.alder": "module Scratch\n\n" + src}, Options{InferModule: "Scratch"}, "3 E0008"},
			{map[string]string{"Lib.alder": "module Lib\n\nit : Int\n" + src}, Options{}, "4 E0008"}, // with a signature
		} {
			_, bag := checkWith(t, c.files, c.o)
			if got := errorsOf(bag); !strings.HasPrefix(got, c.want) {
				t.Errorf("%q: errors %v, want %s first\n%s", src, got, c.want, render(bag))
			}
		}
	}
}
