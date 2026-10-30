package types

import "testing"

// A record literal typed with nothing expected carries a handle (Flex): the first record type it
// is unified with that has exactly its field names decides it (#3).

var tFloat = &Con{Name: "Float"}

func curve() *Record {
	return &Record{Module: "Curve", Name: "Curve", Fields: []Field{
		{Name: "median", Type: tFloat}, {Name: "sigma", Type: tFloat}, {Name: "perDoubling", Type: tFloat}}}
}

func lit(u *Unifier, fs ...Field) *Record {
	return &Record{Anonymous: true, Fields: fs, Flex: u.Fresh()}
}

func curveLit(u *Unifier) *Record {
	return lit(u, Field{Name: "median", Type: tFloat}, Field{Name: "sigma", Type: tFloat}, Field{Name: "perDoubling", Type: tFloat})
}

func isCurve(t Type) bool {
	r, ok := t.(*Record)
	return ok && !r.Anonymous && r.Name == "Curve"
}

func TestRecordFlex_BindsNominal(t *testing.T) {
	u := &Unifier{Subst: Subst{}}
	l := curveLit(u)
	if err := u.Unify(l, curve()); err != nil {
		t.Fatal(err)
	}
	if !isCurve(u.Apply(l)) {
		t.Fatalf("literal is %s, want Curve", String(u.Apply(l)))
	}
}

func TestRecordFlex_Symmetric(t *testing.T) {
	u := &Unifier{Subst: Subst{}}
	l := curveLit(u)
	if err := u.Unify(curve(), l); err != nil {
		t.Fatal(err)
	}
	if !isCurve(u.Apply(l)) {
		t.Fatalf("literal is %s, want Curve", String(u.Apply(l)))
	}
}

func TestRecordFlex_FieldOrderIrrelevant(t *testing.T) {
	u := &Unifier{Subst: Subst{}}
	l := lit(u, Field{Name: "sigma", Type: tFloat}, Field{Name: "perDoubling", Type: tFloat}, Field{Name: "median", Type: tFloat})
	if err := u.Check(l, curve()); err != nil {
		t.Fatal(err)
	}
	if !isCurve(u.Apply(l)) {
		t.Fatalf("literal is %s, want Curve", String(u.Apply(l)))
	}
}

func TestRecordFlex_DeclinesOnNames(t *testing.T) {
	u := &Unifier{Subst: Subst{}}
	missing := lit(u, Field{Name: "median", Type: tFloat}, Field{Name: "sigma", Type: tFloat})
	extra := lit(u, Field{Name: "median", Type: tFloat}, Field{Name: "sigma", Type: tFloat}, Field{Name: "perDoubling", Type: tFloat}, Field{Name: "extra", Type: tInt})
	for _, l := range []*Record{missing, extra} {
		if err := u.Unify(l, curve()); err == nil {
			t.Fatal("different names must not unify")
		}
		if r := u.Apply(l).(*Record); !r.Anonymous || r.Flex == nil {
			t.Fatalf("handle must stay unbound, got %s", String(r))
		}
	}
}

func TestRecordFlex_DeclinesOnFieldType(t *testing.T) {
	u := &Unifier{Subst: Subst{}}
	l := lit(u, Field{Name: "median", Type: tInt}, Field{Name: "sigma", Type: tFloat}, Field{Name: "perDoubling", Type: tFloat})
	err := u.Unify(l, curve())
	if err == nil || err.Error() != "expected Float, got Int" {
		t.Fatalf("err = %v, want the field's mismatch", err)
	}
	if r := u.Apply(l).(*Record); !r.Anonymous {
		t.Fatalf("handle must stay unbound, got %s", String(r))
	}
}

func TestRecordFlex_CheckAndUnify(t *testing.T) {
	for name, f := range map[string]func(u *Unifier, a, b Type) error{
		"Unify": func(u *Unifier, a, b Type) error { return u.Unify(a, b) },
		"Check": func(u *Unifier, a, b Type) error { return u.Check(a, b) },
	} {
		u := &Unifier{Subst: Subst{}}
		l := curveLit(u)
		if err := f(u, l, curve()); err != nil || !isCurve(u.Apply(l)) {
			t.Fatalf("%s: err %v, literal %s", name, err, String(u.Apply(l)))
		}
	}
}

func TestRecordFlex_LinksTwoLiterals(t *testing.T) {
	u := &Unifier{Subst: Subst{}}
	a, b := curveLit(u), curveLit(u)
	if err := u.Unify(a, b); err != nil {
		t.Fatal(err)
	}
	if err := u.Unify(b, curve()); err != nil {
		t.Fatal(err)
	}
	if !isCurve(u.Apply(a)) || !isCurve(u.Apply(b)) {
		t.Fatalf("a %s, b %s: both must be Curve", String(u.Apply(a)), String(u.Apply(b)))
	}
}

func TestRecordFlex_DecidesStructural(t *testing.T) {
	u := &Unifier{Subst: Subst{}}
	l := curveLit(u)
	declared := &Record{Anonymous: true, Fields: curve().Fields}
	if err := u.Unify(l, declared); err != nil {
		t.Fatal(err)
	}
	if err := u.Unify(l, curve()); err == nil {
		t.Fatal("a literal decided structural must not become Curve")
	}
	if r := u.Apply(l).(*Record); !r.Anonymous || r.Flex != nil {
		t.Fatalf("literal must be the declared structural type, got %s (flex %v)", String(r), r.Flex)
	}
}

func TestRecordFlex_GenericNominal(t *testing.T) {
	u := &Unifier{Subst: Subst{}}
	box := &Record{Module: "M", Name: "Box", Params: []string{"a"}, Fields: []Field{{Name: "item", Type: &Param{Name: "a"}}}}
	inst := box.WithArgs([]Type{u.Fresh()})
	l := lit(u, Field{Name: "item", Type: tInt})
	if err := u.Unify(l, inst); err != nil {
		t.Fatal(err)
	}
	if got := String(u.Apply(l)); got != "M.Box Int" {
		t.Fatalf("literal is %s, want M.Box Int", got)
	}
}

func TestRecordFlex_CarriedBySubstAndInstantiate(t *testing.T) {
	u := &Unifier{Subst: Subst{}}
	l := lit(u, Field{Name: "median", Type: &Param{Name: "a"}}, Field{Name: "sigma", Type: tFloat}, Field{Name: "perDoubling", Type: tFloat})
	if r := u.Apply(l).(*Record); r.Flex == nil || r.Flex.ID != l.Flex.ID {
		t.Fatal("Apply must carry the handle")
	}
	if r := u.Instantiate(l).(*Record); r.Flex == nil || r.Flex.ID != l.Flex.ID {
		t.Fatal("Instantiate must carry the handle")
	}
	if r := substParams(l, []string{"a"}, []Type{tFloat}).(*Record); r.Flex == nil || r.Flex.ID != l.Flex.ID {
		t.Fatal("subst must carry the handle")
	}
	inst := u.Instantiate(l)
	if err := u.Unify(inst, curve()); err != nil {
		t.Fatal(err)
	}
	if !isCurve(u.Apply(l)) {
		t.Fatal("deciding an instance decides the literal")
	}
}

func TestRecordFlex_SelfUnify(t *testing.T) {
	u := &Unifier{Subst: Subst{}}
	l := curveLit(u)
	if err := u.Unify(l, l); err != nil {
		t.Fatal(err)
	}
	if _, bound := u.Subst[l.Flex.ID]; bound {
		t.Fatal("a literal unified with itself must stay undecided")
	}
	if err := u.Unify(l, curve()); err != nil || !isCurve(u.Apply(l)) {
		t.Fatalf("err %v, literal %s", err, String(u.Apply(l)))
	}
}
