// internal/syntax/dump_test.go
package syntax

import "testing"

func TestDump(t *testing.T) {
	e := &App{Fn: &Var{Name: "add"}, Args: []Expr{&IntLit{Value: 1}, &Var{Module: "M", Name: "x"}}}
	got := Dump(e)
	want := "(app (var add) (int 1) (var M.x))"
	if got != want {
		t.Fatalf("got %s", got)
	}
}
