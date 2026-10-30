package gen

import "testing"

// Text.Builder in a user annotation and through Text.builder/add/build compiles to rt.TextBuilder
// and the three std functions, and the program vets (#37).
func TestGen_TextBuilder(t *testing.T) {
	files := emit(t, "../../testdata/corpus/text-builder-ok")
	contains(t, files["Export_alder.go"],
		"rt.TextBuilder",
		"std.TextBuilder()",
		"std.TextAdd",
		"std.TextBuild",
	)
	vet(t, files)
}
