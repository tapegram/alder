// internal/types/unify_test.go
package types

import "testing"

var (
	tInt  = &Con{Name: "Int"}
	tText = &Con{Name: "Text"}
)

func TestUnify_Basics(t *testing.T) {
	u := &Unifier{Subst: Subst{}}
	a := u.Fresh()
	if err := u.Unify(&App{Con: "List", Args: []Type{a}}, &App{Con: "List", Args: []Type{tInt}}); err != nil {
		t.Fatal(err)
	}
	if String(u.Apply(a)) != "Int" {
		t.Fatalf("got %s", String(u.Apply(a)))
	}
	if err := u.Unify(tInt, tText); err == nil {
		t.Fatal("Int ~ Text must fail")
	}
}

func TestCheck_BrandWidening(t *testing.T) {
	u := &Unifier{Subst: Subst{}}
	id := &Brand{Module: "Todo", Name: "TodoId", Base: tText}
	if err := u.Check(id, tText); err != nil {
		t.Fatal("brand must widen to base in check mode:", err)
	}
	if err := u.Check(tText, id); err == nil {
		t.Fatal("base must not narrow to brand")
	}
	if err := u.Unify(id, tText); err == nil {
		t.Fatal("unify (exact) must not widen")
	}
	if err := u.Check(&Con{Name: "Never"}, tInt); err != nil {
		t.Fatal("Never fits anything")
	}
}

func TestNormalize_IdAndInsert(t *testing.T) {
	u := &Unifier{Subst: Subst{}}
	id := &Brand{Module: "Todo", Name: "TodoId", Base: tText}
	rec := &Record{Module: "Todo", Name: "Todo", Fields: []Field{{"id", id, false}, {"title", tText, false}, {"done", &Con{"Bool"}, true}}}
	u.RegisterTable(&TableInfo{Module: "Todo", Name: "Todo", Record: rec, IDField: "id", IDType: id,
		Insert: &Record{Module: "Todo", Name: "Insert Todo", Fields: rec.Fields[1:]}})
	got := u.Apply(&App{Con: "Id", Args: []Type{rec}})
	if String(got) != "Todo.TodoId" {
		t.Fatalf("Id Todo → %s", String(got))
	}
	ins := u.Apply(&App{Con: "Insert", Args: []Type{rec}}).(*Record)
	if len(ins.Fields) != 2 || ins.Fields[0].Name != "title" {
		t.Fatalf("Insert Todo → %s", String(ins))
	}
	// Id applied to a type variable stays symbolic until the variable is bound
	v := u.Fresh()
	sym := &App{Con: "Id", Args: []Type{v}}
	if _, ok := u.Apply(sym).(*App); !ok {
		t.Fatal("Id a must stay an App while a is unbound")
	}
	u.Unify(v, rec)
	if String(u.Apply(sym)) != "Todo.TodoId" {
		t.Fatalf("after binding: %s", String(u.Apply(sym)))
	}
}

func TestInstantiate(t *testing.T) {
	u := &Unifier{Subst: Subst{}}
	sig := &Fun{Params: []Type{&Fun{Params: []Type{&Param{"a"}}, Result: &Param{"b"}}, &App{"List", []Type{&Param{"a"}}}}, Result: &App{"List", []Type{&Param{"b"}}}}
	inst := u.Instantiate(sig).(*Fun)
	if _, ok := inst.Params[1].(*App).Args[0].(*Var); !ok {
		t.Fatal("params must become fresh vars")
	}
	if inst.Params[0].(*Fun).Params[0] != inst.Params[1].(*App).Args[0] {
		t.Fatal("same param must map to the same var")
	}
}

func TestString(t *testing.T) {
	got := String(&Fun{Params: []Type{tInt, &App{"Maybe", []Type{tText}}}, Result: &App{"List", []Type{&App{"Maybe", []Type{tInt}}}}})
	if got != "Int -> Maybe Text -> List (Maybe Int)" {
		t.Fatalf("got %s", got)
	}
}

func TestLoadStd(t *testing.T) {
	std, err := LoadStd()
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range []string{"Prelude", "List", "Maybe", "Result", "Text", "Int", "Float", "Bool", "Dict", "Date", "Time", "Html", "Url", "Query", "Task", "Db", "Session", "Log", "Random", "Http", "Form"} {
		if std[m] == nil {
			t.Errorf("missing stdlib module %s", m)
		}
	}
}
