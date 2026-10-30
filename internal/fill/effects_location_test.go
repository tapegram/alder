package fill

import (
	"context"
	"strings"
	"testing"
)

// TestEffects_LocationRobust: the flagged lines and the placed text survive comments the model
// wrote and an import the fill added above.
func TestEffects_LocationRobust(t *testing.T) {
	dir := effectsApp(t)
	expr := "{\n  -- a comment the model wrote\n  let _ <- Mail.notify \"hi\" -- and another\n  Task.succeed Rerender\n}"
	r, err := Run(context.Background(), &script{answers: []string{expr}}, dir, "Home.update.Submit", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("imports %v effects %q lines %v placed %q", r.Imports, r.Effects, r.EffectLines, r.Placed)
	if len(r.Effects) != 1 || !strings.HasPrefix(r.Effects[0], "Mail.notify") || strings.Contains(r.Placed, "comment") {
		t.Errorf("effects %q placed %q", r.Effects, r.Placed)
	}
}
