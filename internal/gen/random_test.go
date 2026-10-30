package gen

import (
	"strings"
	"testing"
)

// A page whose load draws from a seeded Random (Text.hash, step, the generators and
// combinators) and whose tests use Fuzz.fromRandom: the production and test builds vet, so
// each stdlib function's Go signature matches the types gen emits for it.
func TestGen_RandomCorpus(t *testing.T) {
	for _, tests := range []bool{false, true} {
		files := emitWith(t, "../../testdata/corpus/random-ok", tests)
		src := string(files["Home_alder.go"])
		for _, want := range []string{"std.TextHash(", "std.RandomStep(", "std.Seed", "std.Generator[float64]"} {
			if !strings.Contains(src, want) {
				t.Errorf("tests=%v: no %s in Home_alder.go:\n%s", tests, want, src)
			}
		}
		vet(t, files)
	}
}
