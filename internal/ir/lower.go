package ir

import (
	"fmt"
	"strconv"
	"strings"

	"alder/internal/check"
	"alder/internal/diag"
	"alder/internal/names"
	"alder/internal/syntax"
	"alder/internal/types"
)

// Lower builds the IR of a program the checker accepted without errors. A missing recorded type
// is a checker bug and panics with the node and its position.
func Lower(prog *names.Program, res *check.Result) *Program {
	l := &lowerer{prog: prog, res: res, out: &Program{},
		records: map[string]*RecordType{}, unions: map[string]*UnionType{}, values: map[string]*Value{},
		pages: map[string]*Page{}, forms: map[string]*Form{}, msgs: map[string]*Msg{}, erased: map[*types.Record]*types.Record{}, erasedU: map[*types.Union]*types.Union{}, wrappers: map[*RecordType]bool{}, msgPayloads: map[*RecordType]bool{}, pageSessions: map[string]*types.Record{}, anonDefaults: map[syntax.Expr]Expr{}}
	l.declareTypes()
	l.declareValues()
	for _, d := range l.defaults {
		l.enter(d.module)
		d.rt.Fields[d.index].Default = l.expr(d.field.Default)
	}
	for _, b := range l.bodies {
		l.enter(b.value.Module)
		b.value.Body = l.expr(b.decl.Body)
	}
	l.declareCaps()
	return l.out
}

type lowerer struct {
	prog *names.Program
	res  *check.Result
	out  *Program

	records map[string]*RecordType // "Module.Name" → nominal record
	unions  map[string]*UnionType  // "Module.Name" → union (Prelude unions created on first use)
	values  map[string]*Value      // "Module.name"
	pages   map[string]*Page       // page module name
	forms   map[string]*Form       // "Module.Form"
	msgs    map[string]*Msg        // "Owner.Msg": owner is the page module or App
	erased  map[*types.Record]*types.Record
	erasedU map[*types.Union]*types.Union
	// wrappers are the { value: T } payload records made for non-record constructor arguments
	wrappers map[*RecordType]bool
	// msgPayloads are the generated `<Msg>Params` records, whose `form` field a literal omits
	msgPayloads map[*RecordType]bool
	// pageSessions are the narrowed session records of `requires` pages, by page module
	pageSessions map[string]*types.Record
	// anonDefaults are the lowered defaults of anonymous record fields (anonDefault)
	anonDefaults map[syntax.Expr]Expr

	defaults []pendingDefault
	bodies   []pendingBody

	// per-module state while lowering bodies
	module string
	source string // the module's source text, so temporary names never shadow a user's name
	temps  int
	form   *Form // the form of the enclosing <form submit>, for <Input> and friends
}

type pendingDefault struct {
	module string
	rt     *RecordType
	index  int
	field  *syntax.Field
}

type pendingBody struct {
	value *Value
	decl  *syntax.ValueDecl
}

func key(module, name string) string { return module + "." + name }

// enter starts lowering code of module name.
func (l *lowerer) enter(name string) {
	if l.module != name {
		l.module = name
		if m := l.prog.Modules[name]; m != nil {
			l.source = strings.Join(m.Lines, "\n")
		}
	}
	l.temps = 0
	l.form = nil
}

// temp returns a fresh local name for a value the lowering needs to name (a match subject or an
// unnamed payload). It never occurs in the module's source, so it shadows no user binding.
func (l *lowerer) temp() string {
	for {
		l.temps++
		n := "_m" + strconv.Itoa(l.temps)
		if !strings.Contains(l.source, n) {
			return n
		}
	}
}

// ---- types

// erase applies the final substitution and replaces every brand with its base type.
func (l *lowerer) erase(t types.Type) types.Type {
	if t == nil {
		return nil
	}
	return l.eraseT(l.res.U.Apply(t))
}

func (l *lowerer) eraseT(t types.Type) types.Type {
	switch t := t.(type) {
	case nil:
		return nil
	case *types.Brand:
		return l.eraseT(t.Base)
	case *types.Record:
		if t.Module == "App" && t.Name == "Session" && t != l.res.Session && l.pageSessions[l.module] != nil {
			return l.pageSessions[l.module] // the checker's narrowed session of a `requires` page
		}
		if r, ok := l.erased[t]; ok {
			return r
		}
		r := &types.Record{Module: t.Module, Name: t.Name, Anonymous: t.Anonymous, Params: t.Params}
		l.erased[t] = r
		for _, f := range t.Fields {
			r.Fields = append(r.Fields, types.Field{Name: f.Name, Type: l.eraseT(f.Type), HasDefault: f.HasDefault, Default: f.Default})
		}
		if len(t.Args) > 0 {
			r.Args = l.eraseList(t.Args)
		}
		if t.Def != nil {
			r.Def = l.eraseT(t.Def).(*types.Record)
		}
		return r
	case *types.Union: // constructors too, so no brand is reachable from any type in the IR
		if u, ok := l.erasedU[t]; ok {
			return u
		}
		u := &types.Union{Module: t.Module, Name: t.Name, Params: t.Params}
		l.erasedU[t] = u
		u.Args = l.eraseList(t.Args)
		for _, c := range t.Ctors {
			u.Ctors = append(u.Ctors, types.Ctor{Name: c.Name, Arg: l.eraseT(c.Arg)})
		}
		if t.Def != nil {
			u.Def = l.eraseT(t.Def).(*types.Union)
		}
		return u
	case *types.App:
		return &types.App{Con: t.Con, Args: l.eraseList(t.Args)}
	case *types.Fun:
		return &types.Fun{Params: l.eraseList(t.Params), Result: l.eraseT(t.Result)}
	}
	return t
}

func (l *lowerer) eraseList(ts []types.Type) []types.Type {
	out := make([]types.Type, len(ts))
	for i, t := range ts {
		out[i] = l.eraseT(t)
	}
	return out
}

// effectful reports whether t mentions a type M3a does not lower: an effect or a database description.
func effectful(t types.Type) bool {
	switch t := t.(type) {
	case *types.App:
		switch t.Con {
		case "Query", "Task", "Table", "Select", "Condition", "Order", "Column", "Insert":
			return true
		}
		for _, a := range t.Args {
			if effectful(a) {
				return true
			}
		}
	case *types.Union:
		for _, a := range t.Args {
			if effectful(a) {
				return true
			}
		}
	case *types.Fun:
		for _, p := range t.Params {
			if effectful(p) {
				return true
			}
		}
		return effectful(t.Result)
	case *types.Record:
		if t.Anonymous || t.Name == "" {
			for _, f := range t.Fields {
				if effectful(f.Type) {
					return true
				}
			}
		}
	}
	return false
}

// recordType returns the RecordType for a record type: the declared one for a nominal record,
// a new anonymous one otherwise.
func (l *lowerer) recordType(t types.Type) *RecordType {
	rec, ok := t.(*types.Record)
	if !ok {
		panic(fmt.Sprintf("ir: record value of non-record type %s", types.String(t)))
	}
	if !rec.Anonymous && rec.Name != "" {
		if rt, ok := l.records[key(rec.Module, rec.Name)]; ok {
			return rt
		}
		panic(fmt.Sprintf("ir: record type %s is not declared", types.String(rec)))
	}
	return l.anonRecord(rec)
}

// anonRecord is the RecordType of an anonymous record type, with its fields' defaults lowered
// (a signature's `{ label: Text, kind: Text = "a" }`), so a literal that omits kind gets "a".
func (l *lowerer) anonRecord(rec *types.Record) *RecordType {
	rt := &RecordType{}
	for _, f := range rec.Fields {
		ft := FieldT{Name: f.Name, Type: l.erase(f.Type)}
		if f.Default != nil {
			ft.Default = l.anonDefault(f.Default)
		}
		rt.Fields = append(rt.Fields, ft)
	}
	return rt
}

// anonDefault lowers the default expression of an anonymous record's field once, in the module
// it is written in, keeping the state of the code being lowered meanwhile.
func (l *lowerer) anonDefault(e syntax.Expr) Expr {
	if x, ok := l.anonDefaults[e]; ok {
		return x
	}
	module, ok := l.res.DefaultModules[e]
	if !ok {
		panic(fmt.Sprintf("ir: default at %s was not checked", pos(e.GetSpan())))
	}
	saved := *l
	l.module = "" // so that enter switches to module and reads its source
	l.enter(module)
	x := l.expr(e)
	l.module, l.source, l.temps, l.form = saved.module, saved.source, saved.temps, saved.form
	l.anonDefaults[e] = x
	return x
}

// union returns the UnionType of u's declaration, creating it for a Prelude union on first use.
func (l *lowerer) union(u *types.Union) *UnionType {
	k := key(u.Module, u.Name)
	if ut, ok := l.unions[k]; ok {
		return ut
	}
	decl := u
	if u.Def != nil {
		decl = u.Def
	}
	ut := &UnionType{Module: u.Module, Name: u.Name, Params: decl.Params}
	l.unions[k] = ut
	l.fillUnion(ut, decl, false)
	return ut
}

// fillUnion sets ut's constructors from the declaration. A message union's constructor whose
// payload record has no fields has no payload.
func (l *lowerer) fillUnion(ut *UnionType, decl *types.Union, isMsg bool) {
	for _, c := range decl.Constructors() {
		ct := CtorT{Name: c.Name}
		if c.Arg != nil {
			if rec, ok := c.Arg.(*types.Record); ok {
				if !(isMsg && len(rec.Fields) == 0) {
					ct.Payload = l.recordType(l.erase(rec))
				}
			} else {
				ct.Payload = &RecordType{Fields: []FieldT{{Name: "value", Type: l.erase(c.Arg)}}}
				l.wrappers[ct.Payload] = true
			}
		}
		ut.Ctors = append(ut.Ctors, ct)
	}
}

// ctorT finds constructor name of ut.
func (ut *UnionType) ctorT(name string) CtorT {
	for _, c := range ut.Ctors {
		if c.Name == name {
			return c
		}
	}
	panic(fmt.Sprintf("ir: union %s.%s has no constructor %s", ut.Module, ut.Name, name))
}

// ---- declarations

// declareTypes creates every module's nominal types, tables, forms, pages and messages, so
// bodies can refer to any of them regardless of declaration order.
func (l *lowerer) declareTypes() {
	type pendingUnion struct {
		ut   *UnionType
		decl *types.Union
	}
	var unions []pendingUnion
	for _, name := range l.prog.Order {
		m := l.prog.Modules[name]
		mod := &Module{Name: name}
		l.out.Modules = append(l.out.Modules, mod)
		addRecord := func(n string, t types.Type, fields []*syntax.Field) *RecordType {
			rt := &RecordType{Module: name, Name: n}
			if r, ok := t.(*types.Record); ok {
				rt.Params = r.Params
			}
			l.records[key(name, n)] = rt
			mod.Types = append(mod.Types, &TypeDecl{Module: name, Name: n, Record: rt})
			l.recordFields(name, rt, t, fields)
			return rt
		}
		for _, d := range m.Decls {
			switch d := d.(type) {
			case *syntax.TypeDecl:
				switch {
				case d.Record != nil:
					addRecord(d.Name, l.res.Decls[key(name, d.Name)], d.Record.Fields)
				case d.Union != nil:
					ut := &UnionType{Module: name, Name: d.Name, Params: d.Params}
					l.unions[key(name, d.Name)] = ut
					mod.Types = append(mod.Types, &TypeDecl{Module: name, Name: d.Name, Union: ut})
					unions = append(unions, pendingUnion{ut, l.res.Decls[key(name, d.Name)].(*types.Union)})
				}
			case *syntax.TableDecl:
				rt := addRecord(d.Name, l.res.Decls[key(name, d.Name)], d.Fields)
				info := l.res.Tables[key(name, d.Name)]
				t := &Table{Module: name, Name: d.Name, Record: rt, IDField: info.IDField}
				for _, f := range info.Insert.Fields {
					t.InsertFields = append(t.InsertFields, f.Name)
				}
				l.out.Tables = append(l.out.Tables, t)
			case *syntax.FormDecl:
				f := &Form{Module: name, Name: d.Name, Record: addRecord(d.Name, l.res.Decls[key(name, d.Name)], d.Fields)}
				l.forms[key(name, d.Name)] = f
				l.out.Forms = append(l.out.Forms, f)
			case *syntax.SessionDecl:
				if name == "App" {
					l.out.Session = addRecord("Session", l.res.Session, d.Fields)
				}
			}
		}
		if name == "App" {
			if l.out.Session == nil { // pages still name the (empty) Session type
				l.out.Session = addRecord("Session", l.res.Session, nil)
			}
			l.out.AppMsgs = l.declareMsgs(mod, "App", l.res.AppMsg, m)
		}
		if info := l.res.Pages[name]; info != nil {
			pg := &Page{Module: name, Path: info.Decl.Path, Requires: info.Requires}
			pg.Params = addRecord("Params", info.Params, info.Decl.Params.Fields)
			pg.Msgs = l.declareMsgs(mod, name, info.Msg, m)
			pg.Model = l.records[key(name, "Model")]
			l.pages[name] = pg
			l.out.Pages = append(l.out.Pages, pg)
		}
	}
	l.declarePageSessions()
	// constructors last: a payload may name a record of any module
	for _, u := range unions {
		l.fillUnion(u.ut, u.decl, false)
	}
}

// declarePageSessions gives each page its Session: App's record, or for a `requires f` page a
// record `Session` of the page module with field f narrowed from `Maybe T` to `T`, exactly as the
// checker narrows it. The checker's narrowed copies of App.Session erase to that record (see
// eraseT), so the page's signatures and field accesses name it.
func (l *lowerer) declarePageSessions() {
	for i, name := range l.prog.Order {
		pg := l.pages[name]
		if pg == nil {
			continue
		}
		pg.Session = l.out.Session
		if pg.Requires == "" || l.out.Session == nil {
			continue
		}
		rec := &types.Record{Module: name, Name: "Session"}
		for _, f := range l.res.Session.Fields {
			if u, ok := f.Type.(*types.Union); ok && f.Name == pg.Requires && u.Name == "Maybe" && len(u.Args) == 1 {
				f = types.Field{Name: f.Name, Type: u.Args[0], HasDefault: f.HasDefault}
			}
			rec.Fields = append(rec.Fields, f)
		}
		rt := &RecordType{Module: name, Name: "Session"}
		l.recordFields(name, rt, rec, nil)
		l.records[key(name, "Session")] = rt
		mod := l.out.Modules[i]
		mod.Types = append(mod.Types, &TypeDecl{Module: name, Name: "Session", Record: rt})
		pg.Session = rt
		l.pageSessions[name] = l.erase(rec).(*types.Record)
	}
}

// recordFields fills rt's fields from the checker's record and queues their default expressions.
func (l *lowerer) recordFields(module string, rt *RecordType, t types.Type, fields []*syntax.Field) {
	rec, ok := t.(*types.Record)
	if !ok {
		panic(fmt.Sprintf("ir: %s.%s has no record type", rt.Module, rt.Name))
	}
	for i, f := range rec.Fields {
		rt.Fields = append(rt.Fields, FieldT{Name: f.Name, Type: l.erase(f.Type)})
		if i < len(fields) && fields[i].Name == f.Name && fields[i].Default != nil {
			l.defaults = append(l.defaults, pendingDefault{module, rt, i, fields[i]})
		}
	}
}

// declareMsgs declares the message payload records and the Msg union of a page module or App.
func (l *lowerer) declareMsgs(mod *Module, owner string, u *types.Union, m *syntax.Module) []*Msg {
	decls := map[string]*syntax.MsgDecl{}
	for _, d := range m.Decls {
		if md, ok := d.(*syntax.MsgDecl); ok {
			decls[md.Name] = md
		}
	}
	ut := &UnionType{Module: owner, Name: "Msg"}
	l.unions[key(owner, "Msg")] = ut
	var out []*Msg
	for _, c := range u.Constructors() {
		md := decls[c.Name]
		msg := &Msg{Name: c.Name}
		if md.Form != "" {
			msg.Form = l.forms[key(owner, md.Form)]
		}
		rec := c.Arg.(*types.Record)
		if len(rec.Fields) > 0 {
			var fields []*syntax.Field
			if md.Params != nil {
				fields = md.Params.Fields
			}
			msg.Params = &RecordType{Module: owner, Name: rec.Name}
			l.msgPayloads[msg.Params] = true
			l.records[key(owner, rec.Name)] = msg.Params
			mod.Types = append(mod.Types, &TypeDecl{Module: owner, Name: rec.Name, Record: msg.Params})
			l.recordFields(owner, msg.Params, rec, fields)
		}
		l.msgs[key(owner, c.Name)] = msg
		out = append(out, msg)
	}
	l.fillUnion(ut, u, true)
	mod.Types = append(mod.Types, &TypeDecl{Module: owner, Name: "Msg", Union: ut})
	return out
}

// declareValues creates every value with its signature and parameters; bodies are lowered once
// all values exist, since a body may use a component declared later.
func (l *lowerer) declareValues() {
	for i, name := range l.prog.Order {
		mod := l.out.Modules[i]
		l.enter(name) // signatures of a `requires` page name its narrowed Session
		for _, d := range l.prog.Modules[name].Decls {
			vd, ok := d.(*syntax.ValueDecl)
			if !ok || vd.Body == nil {
				continue
			}
			sig := l.erase(l.res.Values[key(name, vd.Name)])
			if sig == nil {
				panic(fmt.Sprintf("ir: no signature recorded for %s.%s at %s", name, vd.Name, pos(vd.Span)))
			}
			if effectful(sig) {
				mod.Skipped = append(mod.Skipped, vd.Name)
				continue
			}
			v := &Value{Module: name, Name: vd.Name, Type: sig, IsComponent: isUpper(vd.Name)}
			for _, p := range vd.Params {
				v.Params = append(v.Params, &Param{Name: p.Name, Type: l.bindType(p)})
			}
			l.values[key(name, vd.Name)] = v
			if v.IsComponent {
				mod.Components = append(mod.Components, v)
			} else {
				mod.Values = append(mod.Values, v)
			}
			l.bodies = append(l.bodies, pendingBody{v, vd})
			if pg := l.pages[name]; pg != nil && vd.Name == "view" {
				pg.View = v
			}
		}
	}
}

func (l *lowerer) declareCaps() {
	for _, name := range l.prog.Order {
		for _, d := range l.prog.Modules[name].Decls {
			cd, ok := d.(*syntax.CapDecl)
			if !ok {
				continue
			}
			c := &Capability{Module: name, Name: cd.Name}
			entries := l.res.Caps[key(name, cd.Name)]
			for _, e := range cd.Entries {
				c.Entries = append(c.Entries, FieldT{Name: e.Name, Type: l.erase(entries[e.Name])})
			}
			l.out.Caps = append(l.out.Caps, c)
		}
	}
}

// ---- recorded types

// typeOf is e's recorded type, brand-erased.
func (l *lowerer) typeOf(e syntax.Expr) types.Type {
	t, ok := l.res.Types[e]
	if !ok {
		panic(fmt.Sprintf("ir: no type recorded for %T at %s (checker bug)", e, pos(e.GetSpan())))
	}
	return l.erase(t)
}

// bindType is the recorded type of the variable a binding site introduces, brand-erased.
func (l *lowerer) bindType(site interface{ GetSpan() diag.Span }) types.Type {
	t, ok := l.res.Binds[site]
	if !ok {
		panic(fmt.Sprintf("ir: no binding type recorded for %T at %s (checker bug)", site, pos(site.GetSpan())))
	}
	return l.erase(t)
}

func pos(s diag.Span) string { return fmt.Sprintf("%s:%d:%d", s.File, s.Start.Line, s.Start.Col) }

func isUpper(s string) bool { return s != "" && s[0] >= 'A' && s[0] <= 'Z' }
