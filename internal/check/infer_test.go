package check

import (
	"os"
	"path/filepath"
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
	res, bag := checkWith(t, files, Options{InferTopLevel: true})
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
		t.Errorf("%d E0007 without InferTopLevel, want 6:\n%s", n, render(bag))
	}
}
