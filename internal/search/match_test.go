package search

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"alder/internal/buildkit"
	"alder/internal/check"
	"alder/internal/types"
)

func TestMatch_FunctionHole(t *testing.T) {
	r := std(t).Search(": Int -> Text", Options{Limit: -1})
	if _, h := mustHit(t, r, "Text.fromInt"); h.Args != 0 {
		t.Errorf("Text.fromInt args %d, want 0", h.Args)
	}
	mustNotHit(t, r, "Text.padLeft")
	mustNotHit(t, r, "Maybe.withDefault")
	for _, h := range r.Results {
		if f, ok := h.Entry.Type.(*types.Fun); ok && h.Args != len(f.Params)-1 {
			t.Errorf("%s.%s : %s has args %d", h.Entry.Module, h.Entry.Name, h.Entry.Signature, h.Args)
		}
	}
}

func TestMatch_NonFunctionTypeQuery(t *testing.T) {
	r := std(t).Search("List (a -> b)", Options{Limit: -1})
	if r.Mode != "type" {
		t.Fatalf("mode %q", r.Mode)
	}
	for _, h := range r.Results {
		if _, ok := h.Entry.Type.(*types.Fun); ok {
			t.Errorf("found function %s.%s", h.Entry.Module, h.Entry.Name)
		}
	}
}

func TestMatch_NoReorderInProduces(t *testing.T) {
	mustNotHit(t, std(t).Search(": List a -> Int -> List a", Options{Limit: -1}), "List.take")
}

func TestMatch_TypesNeverMatch(t *testing.T) {
	for _, h := range std(t).Search(": Maybe Int", Options{Limit: -1}).Results {
		if h.Entry.Kind == TypeDecl {
			t.Errorf("found type %s.%s", h.Entry.Module, h.Entry.Name)
		}
	}
}

func TestMatch_DoesNotMutateIndex(t *testing.T) {
	ix := std(t)
	a := ix.Search(": List Text", Options{Limit: -1})
	b := ix.Search(": List Text", Options{Limit: -1})
	if len(a.Results) == 0 || !reflect.DeepEqual(a, b) {
		t.Errorf("first %v\nsecond %v", hitNames(a), hitNames(b))
	}
}

func TestStd_NoTestSteps(t *testing.T) {
	ix := std(t)
	for _, q := range []string{"Text -> _", ": Task Unit"} {
		for _, h := range ix.Search(q, Options{Limit: -1}).Results {
			if h.Entry.Module == "Test" && (check.IsTestStep(h.Entry.Name) || h.Entry.Name == "fake") {
				t.Errorf("%q found Test.%s", q, h.Entry.Name)
			}
		}
	}
	for _, n := range []string{"fake", "login", "get", "click", "submit"} {
		if find(ix, "Test", n, Function) != nil || find(ix, "Test", n, Value) != nil {
			t.Errorf("Test.%s is indexed", n)
		}
	}
}

func TestMatch_NeverRanksLast(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "app"), 0o755); err != nil {
		t.Fatal(err)
	}
	src := "module App\n\nerror : Error -> Html\nerror err = <h1>Something went wrong</h1>\n\nnotFound : Html\nnotFound = <h1>Not found</h1>\n\nboom : Int -> Never\nboom n = boom n\n"
	if err := os.WriteFile(filepath.Join(dir, "app", "App.alder"), []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	prog, res, bag, errs := buildkit.LoadChecked(dir, nil)
	if len(errs) > 0 || res == nil || bag.HasErrors() {
		t.Fatalf("load: %v %v", errs, bag)
	}
	ix := Build(prog, res)
	for _, q := range []string{": List Text", "Int -> Text"} {
		r := ix.Search(q, Options{Limit: -1})
		at, _ := hitOf(r, "App.boom")
		if at < 0 {
			t.Fatalf("%q: no App.boom", q)
		}
		for i, h := range r.Results[at+1:] { // only bare and Never results may follow boom
			f, ok := h.Entry.Type.(*types.Fun)
			if !ok || !(isNever(f.Result) || q[0] == ':' && isParam(f.Result)) {
				t.Errorf("%q: %s.%s : %s sorts after boom (at %d)", q, h.Entry.Module, h.Entry.Name, h.Entry.Signature, at+1+i)
			}
		}
	}
}

func isParam(t types.Type) bool { _, ok := t.(*types.Param); return ok }
