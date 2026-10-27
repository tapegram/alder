package ir

import (
	"slices"
	"strings"
	"testing"
)

// Property bodies lower to TestProp and Fuzz.any to FuzzDerive, with the derived GenTrees
// shared by Key across modules; recursive unions know their recursive constructors; brands
// with a check, the stdlib brands and Prelude unions get their own kinds; no brand type is
// left in the IR.
func TestLower_FuzzGenTrees(t *testing.T) {
	p := lowerDir(t, "../../testdata/golden/gen_fuzz")
	noBrands(t, p)
	props := map[string]*TestProp{}
	var slugs *Value
	for _, m := range p.Modules {
		for _, tc := range m.Tests {
			if tp, ok := tc.Body.(*TestProp); ok {
				props[tc.Name] = tp
			}
		}
		for _, v := range m.Values {
			if v.Name == "slugs" {
				slugs = v
			}
		}
	}
	if len(props) != 11 {
		t.Fatalf("%d properties, want 11", len(props))
	}

	tree := props["trees have a size"].Gens[0]
	if tree.Kind != GenUnion || tree.Key != "Todo.Tree" || !slices.Equal(tree.Names, []string{"Leaf", "Node"}) ||
		!slices.Equal(tree.Rec, []bool{false, true}) || !tree.Cyclic || tree.Elems[0] != nil || tree.Union == nil {
		t.Errorf("Tree: %+v", tree)
	}
	node := tree.Elems[1]
	if node.Kind != GenRecord || node.Elems[0] != tree || node.Elems[2] != tree || node.Elems[1].Prim != "Int" {
		t.Errorf("Node's payload: %+v", node)
	}

	todo := props["titles are not blank"].Gens[0]
	if todo.Kind != GenRecord || todo.Key != "Todo" || todo.Cyclic ||
		todo.Elems[0].Kind != GenStdBrand || todo.Elems[0].Brand != "Text.NonEmpty" ||
		todo.Elems[1].Kind != GenMaybe || todo.Elems[1].Elems[0].Prim != "Date" ||
		todo.Elems[2].Kind != GenList || todo.Elems[2].Elems[0].Prim != "Text" {
		t.Errorf("Todo: %+v", todo)
	}
	if props["the same types from another module"].Gens[0] != todo {
		t.Error("Report's Todo is not the same tree as Todo's")
	}

	slug := props["slugs are not empty"].Gens[0]
	if slug.Kind != GenBrand || slug.Brand != "Todo.Slug" || slug.Elems[0].Prim != "Text" || BrandName(slug.Brand) != "Slug" || BrandModule(slug.Brand) != "Todo" {
		t.Errorf("Slug: %+v", slug)
	}
	if fd, ok := slugs.Body.(*FuzzDerive); !ok || fd.Gen != slug {
		t.Errorf("slugs = %#v, want the FuzzDerive of Slug's tree", slugs.Body)
	}

	dir := props["everything else"].Gens[5]
	if dir.Key != "Todo.Dir" || !dir.Cyclic || !dir.Elems[1].Cyclic || dir.Elems[1].Elems[0] != dir {
		t.Errorf("Dir: %+v", dir)
	}
	shape := props["everything else"].Gens[0]
	if !slices.Equal(shape.Rec, []bool{false, false, false}) || shape.Cyclic || shape.Elems[1].Prim != "Float" ||
		shape.Elems[2].Elems[0].Brand != "Int.Positive" {
		t.Errorf("Shape: %+v", shape)
	}

	note := props["rows and generic unions"].Gens
	if note[0].Elems[0].Kind != GenBrand || note[0].Elems[0].Check || note[0].Elems[0].Brand != "Note.NoteId" || note[1] != note[0].Elems[0] ||
		note[2].Key != "Note.Pick Text" || note[3].Key != "Note.Pick Int" || note[2].Union != note[3].Union || note[3].Elems[1].Prim != "Int" {
		t.Errorf("rows and generic unions: %+v", note)
	}
	if !slug.Check {
		t.Error("Slug has a check")
	}

	rev := props["reverse twice"]
	if !slices.Equal(rev.Params, []string{"xs"}) || !slices.Equal(rev.Types, []string{"List Int"}) || !slices.Equal(rev.Defaulted, []bool{true}) || rev.ForAllGen != nil {
		t.Errorf("reverse twice: %+v", rev)
	}
	ex := props["explicit"]
	if ex.ForAllGen == nil || !slices.Equal(ex.Params, []string{"ns"}) || ex.ForAllLit == nil || ex.ForAllLit.Key != "List Int" || len(ex.Gens) != 0 {
		t.Errorf("explicit: %+v", ex)
	}
	if task := props["explicit task"]; !slices.Equal(task.Params, []string{"n"}) || task.ForAllGen == nil {
		t.Errorf("explicit task: %+v", task)
	}

	d := Dump(p)
	for _, want := range []string{`(test-prop`, `"xs : List Int (defaulted)"`, `(fuzz-derive Todo.Slug)`, `(for-all`} {
		if !strings.Contains(d, want) {
			t.Errorf("dump lacks %q", want)
		}
	}
}
