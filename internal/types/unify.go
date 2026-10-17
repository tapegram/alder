// internal/types/unify.go
package types

import "fmt"

type Subst map[int]Type

type TableInfo struct {
	Module, Name string
	Record       *Record
	IDField      string
	IDType       Type
	Insert       *Record
	Columns      map[string]Type // field → field type
	// PartKey and PartCol are the table's partition (M15): the session field `@partition k`
	// names and its column ("" when none; `@owner` is PartKey "user"). Owner is PartCol when
	// PartKey is "user" (M11's `@owner` column), "" otherwise. InsertScoped, for a partitioned
	// table, is the record the scoped insert (`Db.insertScoped`, `Db.insertMine`) takes:
	// Insert's fields without PartCol (named "InsertScoped T").
	PartKey, PartCol string
	Owner            string
	InsertScoped     *Record
}

type Unifier struct {
	Subst  Subst
	next   int
	tables map[string]*TableInfo // key: Module+"."+Name
}

func (u *Unifier) RegisterTable(info *TableInfo) {
	if u.tables == nil {
		u.tables = map[string]*TableInfo{}
	}
	u.tables[info.Module+"."+info.Name] = info
}

func (u *Unifier) Table(r *Record) *TableInfo {
	if u.tables == nil {
		return nil
	}
	return u.tables[r.Module+"."+r.Name]
}

func (u *Unifier) Fresh() *Var {
	u.next++
	return &Var{ID: u.next}
}

func (u *Unifier) prune(t Type) Type {
	for {
		v, ok := t.(*Var)
		if !ok {
			return t
		}
		b, bound := u.Subst[v.ID]
		if !bound {
			return t
		}
		t = b
	}
}

// Apply substitutes bound variables and normalizes Id/Insert/InsertScoped over known tables.
// `InsertScoped t` is the checker's type of the scoped insert's row (never written in source).
func (u *Unifier) Apply(t Type) Type {
	t = u.prune(t)
	switch t := t.(type) {
	case *App:
		args := make([]Type, len(t.Args))
		for i, a := range t.Args {
			args[i] = u.Apply(a)
		}
		if (t.Con == "Id" || t.Con == "Insert" || t.Con == "InsertScoped") && len(args) == 1 {
			if r, ok := args[0].(*Record); ok {
				if info := u.Table(r); info != nil {
					switch {
					case t.Con == "Id":
						return info.IDType
					case t.Con == "InsertScoped" && info.InsertScoped != nil:
						return info.InsertScoped
					}
					return info.Insert // `InsertScoped t` of a table without a partition: E0025 says so
				}
			}
		}
		return &App{Con: t.Con, Args: args}
	case *Fun:
		ps := make([]Type, len(t.Params))
		for i, p := range t.Params {
			ps[i] = u.Apply(p)
		}
		return &Fun{Params: ps, Result: u.Apply(t.Result)}
	case *Union:
		if len(t.Args) == 0 {
			return t
		}
		args := make([]Type, len(t.Args))
		for i, a := range t.Args {
			args[i] = u.Apply(a)
		}
		return t.WithArgs(args)
	case *Record:
		if !t.Anonymous {
			if len(t.Args) == 0 {
				return t
			}
			args := make([]Type, len(t.Args))
			for i, a := range t.Args {
				args[i] = u.Apply(a)
			}
			return t.WithArgs(args)
		}
		fs := make([]Field, len(t.Fields))
		for i, f := range t.Fields {
			fs[i] = Field{Name: f.Name, Type: u.Apply(f.Type), HasDefault: f.HasDefault, Default: f.Default}
		}
		return &Record{Fields: fs, Anonymous: true}
	}
	return t
}

func (u *Unifier) occurs(v *Var, t Type) bool {
	t = u.prune(t)
	switch t := t.(type) {
	case *Var:
		return t.ID == v.ID
	case *App:
		for _, a := range t.Args {
			if u.occurs(v, a) {
				return true
			}
		}
	case *Fun:
		for _, p := range t.Params {
			if u.occurs(v, p) {
				return true
			}
		}
		return u.occurs(v, t.Result)
	case *Union:
		for _, a := range t.Args {
			if u.occurs(v, a) {
				return true
			}
		}
	case *Record:
		if !t.Anonymous { // a nominal record's variables are in its arguments; it may be recursive
			for _, a := range t.Args {
				if u.occurs(v, a) {
					return true
				}
			}
			return false
		}
		for _, f := range t.Fields {
			if u.occurs(v, f.Type) {
				return true
			}
		}
	}
	return false
}

func mismatch(a, b Type) error { return fmt.Errorf("expected %s, got %s", String(b), String(a)) }

// Unify: exact structural unification with nominal records/unions/brands.
func (u *Unifier) Unify(a, b Type) error { return u.unify(a, b, false) }

// Check: like Unify, but `actual` may be a brand where its base is expected, or Never anywhere.
func (u *Unifier) Check(actual, expected Type) error { return u.unify(actual, expected, true) }

func (u *Unifier) unify(a, b Type, widen bool) error {
	a, b = u.Apply(a), u.Apply(b)
	// The poison type fits anything without binding: its error is already reported.
	if IsErr(a) || IsErr(b) {
		return nil
	}
	// Never fits anywhere; return before the variable cases so it never binds a variable.
	if widen {
		if c, ok := a.(*Con); ok && c.Name == "Never" {
			return nil
		}
	}
	if va, ok := a.(*Var); ok {
		if vb, ok := b.(*Var); ok && vb.ID == va.ID {
			return nil
		}
		if u.occurs(va, b) {
			return fmt.Errorf("infinite type")
		}
		u.Subst[va.ID] = b
		return nil
	}
	if vb, ok := b.(*Var); ok {
		if u.occurs(vb, a) {
			return fmt.Errorf("infinite type")
		}
		u.Subst[vb.ID] = a
		return nil
	}
	if widen {
		if br, ok := a.(*Brand); ok {
			if _, bIsBrand := b.(*Brand); !bIsBrand {
				return u.unify(br.Base, b, true)
			}
		}
	}
	switch x := a.(type) {
	case *Con:
		if y, ok := b.(*Con); ok && x.Name == y.Name {
			return nil
		}
	case *Param:
		if y, ok := b.(*Param); ok && x.Name == y.Name {
			return nil
		}
	case *Brand:
		if y, ok := b.(*Brand); ok && x.Module == y.Module && x.Name == y.Name {
			return nil
		}
	case *Record:
		y, ok := b.(*Record)
		if !ok {
			break
		}
		if x.Anonymous && y.Anonymous {
			if len(x.Fields) != len(y.Fields) {
				return mismatch(a, b)
			}
			for i := range x.Fields {
				if x.Fields[i].Name != y.Fields[i].Name {
					return mismatch(a, b)
				}
				if err := u.unify(x.Fields[i].Type, y.Fields[i].Type, widen); err != nil {
					return err
				}
			}
			return nil
		}
		if x.Module == y.Module && x.Name == y.Name && !x.Anonymous && !y.Anonymous && len(x.Args) == len(y.Args) {
			for i := range x.Args {
				if err := u.unify(x.Args[i], y.Args[i], false); err != nil {
					return err
				}
			}
			return nil
		}
	case *Union:
		y, ok := b.(*Union)
		if !ok || x.Module != y.Module || x.Name != y.Name || len(x.Args) != len(y.Args) {
			break
		}
		for i := range x.Args {
			if err := u.unify(x.Args[i], y.Args[i], false); err != nil {
				return err
			}
		}
		return nil
	case *App:
		y, ok := b.(*App)
		if !ok || x.Con != y.Con || len(x.Args) != len(y.Args) {
			break
		}
		for i := range x.Args {
			// Html Never fits Html a: widen inside Html only
			w := widen && x.Con == "Html"
			if err := u.unify(x.Args[i], y.Args[i], w); err != nil {
				return err
			}
		}
		return nil
	case *Fun:
		y, ok := b.(*Fun)
		if !ok || len(x.Params) != len(y.Params) {
			break
		}
		for i := range x.Params {
			if err := u.unify(y.Params[i], x.Params[i], widen); err != nil {
				return err
			}
		}
		return u.unify(x.Result, y.Result, widen)
	}
	return mismatch(a, b)
}

// Instantiate replaces every Param with a fresh Var, consistently.
func (u *Unifier) Instantiate(t Type) Type {
	m := map[string]*Var{}
	var inst func(Type) Type
	inst = func(t Type) Type {
		switch t := t.(type) {
		case *Param:
			v, ok := m[t.Name]
			if !ok {
				v = u.Fresh()
				v.Name = ""
				m[t.Name] = v
			}
			return v
		case *App:
			args := make([]Type, len(t.Args))
			for i, a := range t.Args {
				args[i] = inst(a)
			}
			return &App{Con: t.Con, Args: args}
		case *Fun:
			ps := make([]Type, len(t.Params))
			for i, p := range t.Params {
				ps[i] = inst(p)
			}
			return &Fun{Params: ps, Result: inst(t.Result)}
		case *Union:
			if len(t.Args) == 0 {
				return t
			}
			args := make([]Type, len(t.Args))
			for i, a := range t.Args {
				args[i] = inst(a)
			}
			return t.WithArgs(args)
		case *Record:
			if !t.Anonymous {
				if len(t.Args) == 0 {
					return t
				}
				args := make([]Type, len(t.Args))
				for i, a := range t.Args {
					args[i] = inst(a)
				}
				return t.WithArgs(args)
			}
			fs := make([]Field, len(t.Fields))
			for i, f := range t.Fields {
				fs[i] = Field{Name: f.Name, Type: inst(f.Type), HasDefault: f.HasDefault, Default: f.Default}
			}
			return &Record{Fields: fs, Anonymous: true}
		}
		return t
	}
	return inst(t)
}

// CtorArg returns the payload type of constructor `name` of union `un`, with the union's parameters substituted by un.Args.
func CtorArg(un *Union, name string) (Type, bool) {
	for _, c := range un.Constructors() {
		if c.Name == name {
			if c.Arg == nil {
				return nil, true
			}
			return substParams(c.Arg, un.Params, un.Args), true
		}
	}
	return nil, false
}

func substParams(t Type, names []string, args []Type) Type {
	idx := map[string]Type{}
	for i, n := range names {
		if i < len(args) {
			idx[n] = args[i]
		}
	}
	return subst(t, idx, map[string]*Record{})
}

// subst replaces the parameters in idx. inProgress are the generic record instances being built
// (see instantiateRecord), so a recursive record does not recurse forever.
func subst(t Type, idx map[string]Type, inProgress map[string]*Record) Type {
	switch t := t.(type) {
	case *Param:
		if r, ok := idx[t.Name]; ok {
			return r
		}
		return t
	case *App:
		args := make([]Type, len(t.Args))
		for i, a := range t.Args {
			args[i] = subst(a, idx, inProgress)
		}
		return &App{Con: t.Con, Args: args}
	case *Fun:
		ps := make([]Type, len(t.Params))
		for i, p := range t.Params {
			ps[i] = subst(p, idx, inProgress)
		}
		return &Fun{Params: ps, Result: subst(t.Result, idx, inProgress)}
	case *Union:
		if len(t.Args) == 0 {
			return t
		}
		as := make([]Type, len(t.Args))
		for i, a := range t.Args {
			as[i] = subst(a, idx, inProgress)
		}
		return t.WithArgs(as)
	case *Record:
		if !t.Anonymous { // a nominal record mentions parameters only in its arguments, and may be recursive
			if len(t.Args) == 0 {
				return t
			}
			as := make([]Type, len(t.Args))
			for i, a := range t.Args {
				as[i] = subst(a, idx, inProgress)
			}
			return instantiateRecord(t, as, inProgress)
		}
		fs := make([]Field, len(t.Fields))
		for i, f := range t.Fields {
			fs[i] = Field{Name: f.Name, Type: subst(f.Type, idx, inProgress), HasDefault: f.HasDefault, Default: f.Default}
		}
		return &Record{Module: t.Module, Name: t.Name, Fields: fs, Anonymous: t.Anonymous}
	}
	return t
}
