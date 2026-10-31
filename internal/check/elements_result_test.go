package check

import "testing"

// The Result carries the merged manifest the checker used, for the editor's completion and hover.
func TestResult_Elements(t *testing.T) {
	res, _ := runDirResult(t, "../../examples/blog")
	if res == nil || res.Elements == nil || res.Elements.Lookup("al-button") == nil {
		t.Fatalf("Result.Elements %+v, want a manifest with al-button", res)
	}
}
