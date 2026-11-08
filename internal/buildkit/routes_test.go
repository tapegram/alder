package buildkit

import "testing"

// A route app lists its routes in program order, each with its method and kind "route".
func TestRoutes_ListsRoutes(t *testing.T) {
	t.Parallel()
	prog, res, bag, ioErrs := LoadChecked("../../testdata/corpus/routes-ok", nil)
	if len(ioErrs) > 0 || bag.HasErrors() || res == nil {
		t.Fatalf("does not check: %v %v", ioErrs, bag.Items)
	}
	type row struct{ Method, Kind, Path, Module, Name string }
	var got []row
	for _, r := range Routes(prog, res) {
		if r.File == "" || r.Line < 1 {
			t.Errorf("%s.%s has no place: %+v", r.Module, r.Name, r)
		}
		got = append(got, row{r.Method, r.Kind, r.Path, r.Module, r.Name})
	}
	want := map[row]bool{
		{"GET", "route", "/", "Todos", "Index"}:                     true,
		{"GET", "route", "/todos/{id}", "Todos", "Row"}:             true,
		{"GET", "route", "/export.csv", "Todos", "Export"}:          true,
		{"PATCH", "route", "/todos/{id}/toggle", "Todos", "Toggle"}: true,
		{"POST", "route", "/todos", "Todos", "Create"}:              true,
		{"DELETE", "route", "/todos/{id}", "Todos", "Delete"}:       true,
	}
	for _, r := range got {
		delete(want, r)
	}
	if len(want) > 0 {
		t.Errorf("missing %v in %v", want, got)
	}
}
