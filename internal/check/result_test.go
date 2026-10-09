package check

import (
	"testing"

	"alder/internal/types"
)

func TestResult_RecordsTypes(t *testing.T) {
	res, bag := runDirResult(t, "../../examples/todos")
	if bag.HasErrors() {
		t.Fatal(render(bag))
	}
	if res.Values["Todo.countOpen"] == nil || res.Decls["Todo.Todo"] == nil || res.Tables["Todo.Todo"] == nil {
		t.Fatalf("declarations missing from Result")
	}
	// every expression in every body has a type, and none is an unbound var
	n := 0
	for e, ty := range res.Types {
		n++
		if _, isVar := res.U.Apply(ty).(*types.Var); isVar {
			t.Errorf("unresolved type for %T at %v", e, e.GetSpan().Start)
		}
	}
	if n < 100 {
		t.Fatalf("too few recorded expressions: %d", n)
	}
	// every variable a body introduces has a resolved type
	if len(res.Binds) == 0 {
		t.Fatal("no bindings recorded")
	}
	for site, ty := range res.Binds {
		if _, isVar := res.U.Apply(ty).(*types.Var); isVar {
			t.Errorf("unresolved type for binding %T", site)
		}
	}
	if res.Pages["Todos.Index"] == nil || res.Session == nil || res.AppMsg == nil {
		t.Fatal("page/app info missing")
	}
}
