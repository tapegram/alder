package mutate

import (
	"os"
	"path/filepath"
	"testing"
)

// Fuzzer-typed values get no mutants (spec §0a): a generator is test code, wherever it is
// declared, as is a function returning one. Other values of the module still get theirs.
func TestFuzzerValuesNotMutated(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "app"), 0o755); err != nil {
		t.Fatal(err)
	}
	src := `module Gens

small : Fuzz.Fuzzer Int
small = Fuzz.filter (\n -> n > 2 && n < 10) Fuzz.int

pick : Bool -> Fuzz.Fuzzer Int
pick b = if b then Fuzz.int else Fuzz.constant 1

plain : Int -> Bool
plain n = n > 2
`
	if err := os.WriteFile(filepath.Join(dir, "app", "Gens.alder"), []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	ms, _ := Collect(lower(t, dir))
	plain := 0
	for _, m := range ms {
		if m.Decl != "plain" {
			t.Errorf("mutant %d (%s %s) in %s, a Fuzzer value", m.ID, m.Kind, m.Before, m.Decl)
		} else {
			plain++
		}
	}
	if plain == 0 {
		t.Error("plain has no mutants")
	}
}
