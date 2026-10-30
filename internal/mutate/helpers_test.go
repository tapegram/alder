package mutate

import "testing"

// A test helper (#4) gets no mutants: it is test code, wherever it is declared. The golden app's
// ordinary values still get theirs.
func TestTestHelpersNotMutated(t *testing.T) {
	ms, _ := Collect(lower(t, "../../testdata/golden/gen_helpers"))
	ordinary := 0
	for _, m := range ms {
		switch m.Decl {
		case "fetchText", "at":
			t.Errorf("mutant %d (%s %s) in %s, a test helper", m.ID, m.Kind, m.Before, m.Decl)
		case "label":
			ordinary++
		}
	}
	if ordinary == 0 {
		t.Error("label (an ordinary value) has no mutants")
	}
}
