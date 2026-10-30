package fill

import (
	"context"
	"testing"
)

// A cycle of values must not hide an effect: a value walked while an ancestor of the cycle was
// still busy is not remembered as pure (the security review of #46). Scc.a calls Scc.b before
// its own Task.sleep; Scc.b calls Scc.a; Scc.c calls Scc.b only.
func sccApp(t *testing.T) string {
	t.Helper()
	return holeApp(t, map[string]string{
		"app/Scc.alder": "module Scc\n\na : Int -> Task Int\na n = {\n  let m <- b n\n  let _ <- Task.sleep 1\n  Task.succeed m\n}\n\nb : Int -> Task Int\nb n = if n > 0 then a (n - 1) else Task.succeed n\n\nc : Int -> Task Int\nc n = b n\n",
	})
}

func TestEffects_SCC(t *testing.T) {
	for _, c := range []struct {
		name, expr string
		want       int
	}{
		{"a then c in one expression", "{\n  let _ <- Scc.a 1\n  let _ <- Scc.c 2\n  Task.succeed Rerender\n}", 2},
		{"c alone", "{\n  let _ <- Scc.c 2\n  Task.succeed Rerender\n}", 1},
		{"b alone", "{\n  let _ <- Scc.b 2\n  Task.succeed Rerender\n}", 1},
		{"a then b", "{\n  let _ <- Scc.a 1\n  let _ <- Scc.b 2\n  Task.succeed Rerender\n}", 2},
	} {
		dir := sccApp(t)
		r, err := Run(context.Background(), &script{answers: []string{c.expr}}, dir, "Home.update.Submit", "", nil)
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		t.Logf("%s: effects %q lines %v", c.name, r.Effects, r.EffectLines)
		if len(r.Effects) != c.want {
			t.Errorf("%s: %d effects %q, want %d", c.name, len(r.Effects), r.Effects, c.want)
		}
	}
}
