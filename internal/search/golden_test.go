package search

import (
	"testing"
)

// hitOf is the index and hit named "Module.Name" in r, or -1 and a zero Hit.
func hitOf(r Response, name string) (int, Hit) {
	for i, h := range r.Results {
		if h.Entry.Module+"."+h.Entry.Name == name {
			return i, h
		}
	}
	return -1, Hit{}
}

// mustHit is the index and hit named name, failing the test when r does not hold it.
func mustHit(t *testing.T, r Response, name string) (int, Hit) {
	t.Helper()
	i, h := hitOf(r, name)
	if i < 0 {
		t.Fatalf("%q: no %s in %v", r.Query, name, hitNames(r))
	}
	return i, h
}

func mustNotHit(t *testing.T, r Response, name string) {
	t.Helper()
	if i, _ := hitOf(r, name); i >= 0 {
		t.Errorf("%q: found %s at %d", r.Query, name, i)
	}
}

// mustPrecede checks that every one of first comes before every one of then.
func mustPrecede(t *testing.T, r Response, first, then []string) {
	t.Helper()
	for _, a := range first {
		ia, _ := mustHit(t, r, a)
		for _, b := range then {
			if ib, _ := mustHit(t, r, b); ia >= ib {
				t.Errorf("%q: %s (%d) is not before %s (%d)", r.Query, a, ia, b, ib)
			}
		}
	}
}

// TestGolden is the spec's "Golden queries" table, one subtest per row.
func TestGolden(t *testing.T) {
	ix := std(t)
	all := Options{Limit: -1}
	search := func(q string) Response { return ix.Search(q, all) }

	t.Run("split", func(t *testing.T) {
		if got := hitNames(search("split")); len(got) == 0 || got[0] != "Text.split" {
			t.Fatalf("got %v", got)
		}
	})

	t.Run("Text.", func(t *testing.T) {
		r := search("Text.")
		var want []string
		for _, e := range ix.Entries {
			if e.Module == "Text" {
				want = append(want, "Text."+e.Name)
			}
		}
		got := hitNames(r)
		if len(got) != len(want) {
			t.Fatalf("got %d hits, want %d: %v", len(got), len(want), got)
		}
		for i := range want {
			if got[i] != want[i] {
				t.Fatalf("hit %d is %s, want %s", i, got[i], want[i])
			}
		}
	})

	t.Run("Text -> Text -> List Text", func(t *testing.T) {
		r := search("Text -> Text -> List Text")
		if _, h := mustHit(t, r, "Text.split"); h.Match != "exact" {
			t.Errorf("Text.split is %q, want exact", h.Match)
		}
	})

	t.Run("List b -> Maybe b", func(t *testing.T) {
		r := search("List b -> Maybe b")
		mustHit(t, r, "List.head")
		mustHit(t, r, "List.last")
		mustNotHit(t, r, "List.tail")
	})

	t.Run("List Int -> Int", func(t *testing.T) {
		r := search("List Int -> Int")
		is, sum := mustHit(t, r, "List.sum")
		il, length := mustHit(t, r, "List.length")
		if sum.Match != "exact" || length.Match != "general" || is >= il {
			t.Errorf("List.sum %d %q, List.length %d %q", is, sum.Match, il, length.Match)
		}
	})

	t.Run("List a -> Int -> List a", func(t *testing.T) {
		r := search("List a -> Int -> List a")
		for _, n := range []string{"List.take", "List.drop"} {
			if _, h := mustHit(t, r, n); h.Match != "reordered" {
				t.Errorf("%s is %q, want reordered", n, h.Match)
			}
		}
	})

	t.Run("a -> b", func(t *testing.T) {
		r := search("a -> b")
		for _, h := range r.Results {
			if h.Entry.Module == "List" || h.Entry.Module == "Text" {
				t.Errorf("found %s.%s", h.Entry.Module, h.Entry.Name)
			}
		}
	})

	t.Run("Text -> _", func(t *testing.T) {
		r := search("Text -> _")
		for _, n := range []string{"Text.length", "Text.toInt", "Text.toBytes"} {
			mustHit(t, r, n)
		}
		mustNotHit(t, r, "Text.fromInt")
		mustNotHit(t, r, "Text.split")
	})

	t.Run(": Maybe Int", func(t *testing.T) {
		r := search(": Maybe Int")
		exact := []string{"Text.toInt", "List.maximum", "List.minimum"}
		general := []string{"Prelude.Nothing", "Prelude.Just", "List.head"}
		for _, n := range exact {
			if _, h := mustHit(t, r, n); h.Match != "exact" || h.Args != 1 {
				t.Errorf("%s is %q args %d, want exact args 1", n, h.Match, h.Args)
			}
		}
		mustPrecede(t, r, exact, general)
		mustPrecede(t, r, general, []string{"Maybe.withDefault"})
		if _, h := mustHit(t, r, "Prelude.Nothing"); h.Args != 0 {
			t.Errorf("Prelude.Nothing args %d, want 0", h.Args)
		}
	})

	t.Run(": List Text", func(t *testing.T) {
		r := search(": List Text")
		for _, n := range []string{"Text.lines", "Text.words"} {
			if _, h := mustHit(t, r, n); h.Args != 1 {
				t.Errorf("%s args %d, want 1", n, h.Args)
			}
		}
		if _, h := mustHit(t, r, "Text.split"); h.Args != 2 {
			t.Errorf("Text.split args %d, want 2", h.Args)
		}
		mustPrecede(t, r, []string{"Text.lines", "Text.words"}, []string{"Text.split"})
		mustPrecede(t, r, []string{"Text.split"}, []string{"List.reverse"})
		mustPrecede(t, r, []string{"List.reverse"}, []string{"List.foldl"})
	})
}
