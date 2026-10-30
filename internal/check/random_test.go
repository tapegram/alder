package check

import (
	"strings"
	"testing"

	"alder/internal/types"
)

// Random's opaque types are written qualified in signatures, like Fuzz.Fuzzer.
func TestRandomTypesCheck(t *testing.T) {
	files := map[string]string{
		"Scratch.alder": `module Scratch

dice : Random.Generator Int
dice = Random.intRange { min: 1, max: 6 }

start : Random.Seed
start = Random.seed (Text.hash "a/b")

it = Random.step dice start
pick = Fuzz.fromRandom dice
`,
	}
	res, bag := checkWith(t, files, Options{InferModule: "Scratch"})
	if got := errorsOf(bag); got != "" {
		t.Fatalf("errors %v\n%s", got, render(bag))
	}
	for k, w := range map[string]string{
		"Scratch.it":   "{ value: Int, seed: Random.Seed }",
		"Scratch.pick": "Fuzzer Int", // an App prints unqualified (as Fuzzer always has)
	} {
		if got, ok := res.Values[k]; !ok || types.String(res.U.Apply(got)) != w {
			t.Errorf("%s : %v, want %s", k, types.String(res.U.Apply(got)), w)
		}
	}
}

func TestRandomBareSeedIsE0202(t *testing.T) {
	ds := checkModule(t, "module Main\n\ns : Seed\ns = Random.seed 1\n")
	if len(ds) != 1 || ds[0].Code != "E0202" || !strings.Contains(ds[0].Message, "write `Random.Seed`") {
		t.Fatalf("got %v", ds)
	}
	if ds[0].Fix == nil {
		t.Fatal("no fix")
	}
}

// A generator is a function underneath: comparing two is refused, not silently False.
func TestGeneratorEqRefused(t *testing.T) {
	for _, src := range []string{
		"module Main\n\ng : Random.Generator Int\ng = Random.constant 1\n\nsame : Bool\nsame = g == g\n",
		"module Main\n\ng : Fuzz.Fuzzer Int\ng = Fuzz.constant 1\n\nsame : Bool\nsame = g == g\n",
	} {
		ds := checkModule(t, src)
		if len(ds) != 1 || ds[0].Message != "functions cannot be compared" {
			t.Errorf("%s: got %v", src, ds)
		}
	}
	// a seed is a value
	if ds := checkModule(t, "module Main\n\nsame : Bool\nsame = Random.seed 1 == Random.seed 1\n"); len(ds) != 0 {
		t.Errorf("seeds: %v", ds)
	}
}

func TestFuzzAnyAtSeedIsE0038(t *testing.T) {
	for _, ty := range []string{"Random.Seed", "Random.Generator Int"} {
		ds := checkModule(t, "module Main\n\ntest \"t\" = \\(s : "+ty+") -> True\n")
		if len(ds) != 1 || ds[0].Code != "E0038" {
			t.Errorf("%s: got %v", ty, ds)
		}
	}
}
