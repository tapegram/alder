package search

import (
	"reflect"
	"testing"

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
