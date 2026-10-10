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
	rec := &Record{Module: "Todo", Name: "Todo", Fields: []Field{{Name: "id", Type: id}, {Name: "title", Type: tText}, {Name: "done", Type: &Con{Name: "Bool"}, HasDefault: true}}}
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
	for _, m := range []string{"Prelude", "List", "Maybe", "Result", "Text", "Int", "Float", "Bool", "Dict", "Date", "Time", "Html", "Url", "Query", "Task", "Db", "Session", "Log", "Random", "Http"} {
		if std[m] == nil {
			t.Errorf("missing stdlib module %s", m)
		}
	}
	// Form is a prelude type, not a module: a decoded form is the message's form field.
	if std["Form"] != nil {
		t.Error("stdlib has a Form module")
	}
}

func TestCheck_NeverDoesNotBindVar(t *testing.T) {
	u := &Unifier{Subst: Subst{}}
	v := u.Fresh()
	if err := u.Check(Never, v); err != nil {
		t.Fatal(err)
	}
	if err := u.Unify(v, Int); err != nil {
		t.Fatalf("var should still be free: %v", err)
	}
}

func TestInstantiate_SeesLaterCtors(t *testing.T) {
	u := &Union{Module: "M", Name: "T", Params: []string{"a"}}
	cp := u.WithArgs([]Type{Int})
	u.Ctors = append(u.Ctors, Ctor{Name: "Box", Arg: &Param{Name: "a"}})
	if _, ok := CtorArg(cp, "Box"); !ok {
		t.Fatal("copy made before the ctor was appended should still find it")
	}
	un := &Unifier{Subst: Subst{}}
	if _, ok := CtorArg(un.Instantiate(cp).(*Union), "Box"); !ok {
		t.Fatal("instantiated copy should find it")
	}
}

// An unsolved inference variable never prints its internal id in a user-facing message.
func TestString_UnboundVarHidesID(t *testing.T) {
	u := &Unifier{Subst: Subst{}}
	got := String(&App{Con: "Id", Args: []Type{u.Fresh()}})
	if got != "Id _" {
		t.Fatalf("got %q, want %q", got, "Id _")
	}
}

func TestUnify_PoisonFitsWithoutBinding(t *testing.T) {
	u := &Unifier{Subst: Subst{}}
	if err := u.Unify(Err, tInt); err != nil {
		t.Fatal("Err must unify with Int:", err)
	}
	if err := u.Check(&App{Con: "List", Args: []Type{tText}}, Err); err != nil {
		t.Fatal("anything must check against Err:", err)
	}
	v := u.Fresh()
	if err := u.Unify(v, Err); err != nil {
		t.Fatal(err)
	}
	if _, bound := u.Subst[v.ID]; bound {
		t.Fatal("Err must not bind a variable")
	}
	if !IsErr(Err) || IsErr(&Con{Name: "<error>"}) {
		t.Fatal("IsErr compares by identity")
	}
	if String(Err) != "_" {
		t.Fatalf("String(Err) = %q", String(Err))
	}
}
