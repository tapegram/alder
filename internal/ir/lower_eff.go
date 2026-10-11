package ir

import (
	"fmt"
	"slices"
	"strings"

	"alder/internal/check"
	"alder/internal/syntax"
	"alder/internal/types"
)

// Lowering of effects and database descriptions: the Query.*, Task.* and Db.* functions,
// column methods and capability entries become the Eff*, Db*, Cond, Ord and CapCall nodes.
// Each such callee is a "head"; applied to all its parameters it is the node, applied to fewer
// (or not at all) it is a Lambda over the missing parameters whose body is the node.

// effHead is a callee lowered to a node rather than a Call.
type effHead struct {
	name  string     // "Query.succeed", "Db.where", …; "column" or "capability"
	ft    types.Type // the head's type: a Fun, or the node's own type when it takes nothing
	col   *ColumnRef // the column of a column method (name "column")
	meth  string     // the column method: eq, ne, …, asc, desc
	cap   *Capability
	entry string
	at    string
}

// effHead returns the head fn is, or nil when fn is an ordinary callee.
func (l *lowerer) effHead(fn syntax.Expr) *effHead {
	switch x := fn.(type) {
	case *syntax.Var:
		switch x.Ref.Kind {
		case syntax.RefStd:
			switch x.Ref.Module {
			case "Query", "Task", "Db":
				return &effHead{name: x.Ref.Module + "." + x.Ref.Name, ft: l.typeOf(x), at: pos(x.Span)}
			case "Test":
				if x.Ref.Name == "run" || x.Ref.Name == "trace" { // the queries are ordinary calls
					return &effHead{name: x.Ref.Module + "." + x.Ref.Name, ft: l.typeOf(x), at: pos(x.Span)}
				}
			}
		case syntax.RefCapability:
			capName := x.Module // `Clock.now`; with its module, `A.Clock.now`
			if i := strings.LastIndexByte(capName, '.'); i >= 0 {
				capName = capName[i+1:]
			}
			c := l.caps[key(x.Ref.Module, capName)]
			if c == nil {
				panic(fmt.Sprintf("ir: unknown capability %s.%s at %s", x.Ref.Module, x.Module, pos(x.Span)))
			}
			return &effHead{name: "capability", ft: l.typeOf(x), cap: c, entry: x.Name, at: pos(x.Span)}
		}
	case *syntax.Access:
		if app, ok := l.typeOf(x.Expr).(*types.App); ok && app.Con == "Column" {
			col, ok := l.expr(x.Expr).(*ColumnRef)
			if !ok {
				panic(fmt.Sprintf("ir: column method .%s at %s is not applied to a column written as Table.field (not supported)", x.Field, pos(x.Span)))
			}
			return &effHead{name: "column", ft: l.typeOf(x), col: col, meth: x.Field, at: pos(x.Span)}
		}
	}
	return nil
}

// testFake lowers `Test.fake Cap` (args: the capability name) of type t, or returns nil when fn
// is not Test.fake. The checker accepted it only applied to a capability with a fake.
func (l *lowerer) testFake(fn syntax.Expr, args []syntax.Expr, t types.Type) Expr {
	v, ok := fn.(*syntax.Var)
	if !ok || v.Ref.Kind != syntax.RefStd || v.Ref.Module != "Test" || v.Ref.Name != "fake" {
		return nil
	}
	var ct *syntax.CtorRef
	if len(args) == 1 {
		ct, _ = args[0].(*syntax.CtorRef)
	}
	if ct == nil || l.fakesByCap[ct.Ref.Name] == nil {
		panic(fmt.Sprintf("ir: Test.fake at %s is not applied to a capability with a fake (checker bug)", pos(v.Span)))
	}
	return &TestFake{Typed: Typed{t}, Fake: l.fakesByCap[ct.Ref.Name]}
}

// testStep lowers a browser step of module Test (`Test.login s`, `Test.get p`, `Test.open p`,
// `Test.submit F r`, `Test.submitRaw F fields`, `Test.click m`) of type t, or returns nil when
// fn is not one. The checker accepted each applied to all its arguments, in the shapes below.
func (l *lowerer) testStep(fn syntax.Expr, args []syntax.Expr, t types.Type) Expr {
	v, ok := fn.(*syntax.Var)
	if !ok || v.Ref.Kind != syntax.RefStd || v.Ref.Module != "Test" || !check.IsTestStep(v.Ref.Name) {
		return nil
	}
	bug := func(what string) Expr {
		panic(fmt.Sprintf("ir: Test.%s at %s: %s (checker bug)", v.Ref.Name, pos(v.Span), what))
	}
	switch v.Ref.Name {
	case "login":
		return &TestLogin{Typed: Typed{t}, Session: l.expr(args[0])}
	case "get", "open":
		ref, ok := l.expr(args[0]).(*PageRef)
		if !ok {
			return bug("not a page value")
		}
		return &TestGet{Typed: Typed{t}, Page: ref.Page, Params: ref.Params, Open: v.Ref.Name == "open"}
	case "submit", "submitRaw":
		var form *Form
		var msg *Msg
		var page *Page
		var params Expr
		if ct, _ := args[0].(*syntax.CtorRef); ct != nil && ct.Ref.Kind == syntax.RefType { // a form
			form = l.forms[key(ct.Ref.Module, ct.Ref.Name)]
			msgs := l.out.AppMsgs
			if ct.Ref.Module != "App" {
				page = l.pages[ct.Ref.Module]
				if page == nil {
					return bug("a form of no page")
				}
				msgs = page.Msgs
			}
			for _, m := range msgs {
				if m.Form == form && form != nil {
					msg = m
					break
				}
			}
		} else { // the message posting it, with its parameters
			c, ok := l.expr(args[0]).(*Ctor)
			if !ok {
				return bug("not a form or a message")
			}
			msg = l.msgs[key(c.U.Module, c.Name)]
			if msg != nil {
				form = msg.Form
				if c.U.Module != "App" {
					page = l.pages[c.U.Module]
				}
				if msgHasParams(msg) {
					params = c.Payload
				}
			}
		}
		if msg == nil || form == nil {
			return bug("no message posts the form")
		}
		if v.Ref.Name == "submit" {
			return &TestSubmit{Typed: Typed{t}, Form: form, Msg: msg, Page: page, Params: params, Record: l.expr(args[1])}
		}
		return &TestSubmitRaw{Typed: Typed{t}, Form: form, Msg: msg, Page: page, Params: params, Fields: l.expr(args[1])}
	case "click":
		c, ok := l.expr(args[0]).(*Ctor)
		if !ok {
			return bug("not a message")
		}
		owner := c.U.Module
		msg := l.msgs[key(owner, c.Name)]
		if msg == nil {
			return bug("not a message")
		}
		var page *Page
		if owner != "App" {
			page = l.pages[owner]
		}
		var params Expr
		if msgHasParams(msg) {
			params = c.Payload
		}
		return &TestClick{Typed: Typed{t}, Page: page, Msg: msg, Params: params}
	}
	return nil
}

// msgHasParams reports whether message m has parameters besides its form.
func msgHasParams(m *Msg) bool {
	if m.Params == nil {
		return false
	}
	for _, f := range m.Params.Fields {
		if f.Name != "form" {
			return true
		}
	}
	return false
}

// lowerArgs lowers a head's arguments, then piped. A table argument (`Todo` in `Db.get Todo`)
// becomes nil: the node takes its table from the checked type.
func (l *lowerer) lowerArgs(args []syntax.Expr, piped Expr) []Expr {
	var out []Expr
	for _, a := range args {
		if ct, ok := a.(*syntax.CtorRef); ok && ct.Ref.Kind == syntax.RefTable {
			out = append(out, nil)
			continue
		}
		out = append(out, l.expr(a))
	}
	if piped != nil {
		out = append(out, piped)
	}
	return out
}

// special lowers head h applied to args; t is the type of the whole application (h.ft when
// args is empty).
func (l *lowerer) special(h *effHead, args []Expr, t types.Type) Expr {
	fn, isFn := h.ft.(*types.Fun)
	if !isFn {
		if len(args) > 0 {
			panic(fmt.Sprintf("ir: %s at %s takes no arguments", h.name, h.at))
		}
		return l.build(h, nil, h.ft)
	}
	switch {
	case len(args) == len(fn.Params):
		return l.build(h, args, t)
	case len(args) > len(fn.Params):
		panic(fmt.Sprintf("ir: %s at %s applied to %d arguments, takes %d", h.name, h.at, len(args), len(fn.Params)))
	}
	missing := fn.Params[len(args):]
	lam := &Lambda{Typed: Typed{&types.Fun{Params: missing, Result: fn.Result}}}
	all := slices.Clone(args)
	for _, p := range missing {
		n := l.temp()
		lam.Params = append(lam.Params, &Param{Name: n, Type: p})
		all = append(all, &Var{Typed: Typed{p}, Kind: VarLocal, Name: n})
	}
	lam.Body = l.build(h, all, fn.Result)
	return lam
}

// build makes the node of head h applied to all its parameters; t is the node's type.
func (l *lowerer) build(h *effHead, args []Expr, t types.Type) Expr {
	var params []types.Type
	if fn, ok := h.ft.(*types.Fun); ok {
		params = fn.Params
	}
	table := func(i int) *Table { return l.tableOf(params[i], h.at) }
	switch h.name {
	case "column":
		switch h.meth {
		case "asc", "desc":
			return &Ord{Typed: Typed{t}, Col: h.col, Desc: h.meth == "desc"}
		case "isNothing", "isJust":
			return &Cond{Typed: Typed{t}, Kind: h.meth, Col: h.col}
		}
		return &Cond{Typed: Typed{t}, Kind: h.meth, Col: h.col, Value: args[0]}
	case "capability":
		return &CapCall{Typed: Typed{t}, Cap: h.cap, Entry: h.entry, Args: args}
	case "Query.succeed", "Task.succeed":
		return &EffPure{Typed: Typed{t}, Kind: l.effKind(t, h.at), Value: args[0]}
	case "Query.map", "Task.map":
		return &EffMap{Typed: Typed{t}, Kind: l.effKind(t, h.at), Fn: args[0], Effect: args[1]}
	case "Query.all", "Task.all":
		return &EffAll{Typed: Typed{t}, Kind: l.effKind(t, h.at), Effects: args[0]}
	case "Task.fail":
		return &EffFail{Typed: Typed{t}, Message: args[0]}
	case "Query.notFound", "Task.notFound":
		return &EffNotFound{Typed: Typed{t}}
	case "Test.run":
		return &TestRun{Typed: Typed{t}, Task: args[0]}
	case "Test.trace":
		return &TestTrace{Typed: Typed{t}}
	case "Db.get":
		return &DbGet{Typed: Typed{t}, Table: table(0), ID: args[1]}
	case "Db.from":
		return &DbSelect{Typed: Typed{t}, Table: table(0)}
	case "Db.where":
		s := l.extend(args[1], table(1), t)
		s.Where = append(s.Where, args[0])
		return s
	case "Db.orderBy":
		s := l.extend(args[1], table(1), t)
		s.Order = append(s.Order, args[0])
		return s
	case "Db.limit":
		s := l.extend(args[1], table(1), t)
		s.Limit = args[0]
		return s
	case "Db.offset":
		s := l.extend(args[1], table(1), t)
		s.Offset = args[0]
		return s
	case "Db.all", "Db.first", "Db.count":
		term := map[string]SelTerminal{"Db.all": SelAll, "Db.first": SelFirst, "Db.count": SelCount}[h.name]
		if s, ok := args[0].(*DbSelect); ok && s.Terminal == SelNone {
			s = copySelect(s, t)
			s.Terminal = term
			return s
		}
		return &DbRun{Typed: Typed{t}, Select: args[0], Terminal: term}
	case "Db.and", "Db.or":
		c := &Cond{Typed: Typed{t}, Kind: h.name[len("Db."):]}
		if list, ok := args[0].(*List); ok {
			c.Subs = append([]Expr{}, list.Elems...)
		} else {
			c.Value = args[0]
		}
		return c
	case "Db.not":
		return &Cond{Typed: Typed{t}, Kind: "not", Subs: []Expr{args[0]}}
	case "Db.insert":
		return &DbInsert{Typed: Typed{t}, Table: table(0), Row: args[1]}
	case "Db.update":
		return &DbUpdate{Typed: Typed{t}, Table: table(0), ID: args[1], Fn: args[2]}
	case "Db.delete":
		return &DbDelete{Typed: Typed{t}, Table: table(0), ID: args[1]}
	}
	panic(fmt.Sprintf("ir: stdlib function %s at %s has no lowering", h.name, h.at))
}

// extend is the select s extended by one more step: a copy of s when it is a pipeline written in
// place (a DbSelect not yet run), else a DbSelect of table tb whose Base is s.
func (l *lowerer) extend(s Expr, tb *Table, t types.Type) *DbSelect {
	if d, ok := s.(*DbSelect); ok && d.Terminal == SelNone {
		return copySelect(d, t)
	}
	return &DbSelect{Typed: Typed{t}, Table: tb, Base: s}
}

// copySelect copies d with type t; the copy's lists do not share d's arrays.
func copySelect(d *DbSelect, t types.Type) *DbSelect {
	c := *d
	c.Ty = t
	c.Where = slices.Clone(d.Where)
	c.Order = slices.Clone(d.Order)
	return &c
}

// tableOf is the table of a database type (`Table t`, `Select t`): its argument t must be a
// table's record.
func (l *lowerer) tableOf(t types.Type, at string) *Table {
	if app, ok := t.(*types.App); ok && len(app.Args) == 1 {
		if rec, ok := app.Args[0].(*types.Record); ok && !rec.Anonymous {
			if tb := l.tables[key(rec.Module, rec.Name)]; tb != nil {
				return tb
			}
		}
	}
	panic(fmt.Sprintf("ir: the table of %s at %s is not known (a database function over a type parameter is not supported)", types.String(t), at))
}

// effKind is the kind of effect type t.
func (l *lowerer) effKind(t types.Type, at string) EffKind {
	k, ok := effectKind(t)
	if !ok {
		panic(fmt.Sprintf("ir: effect at %s has type %s, not Query or Task", at, types.String(t)))
	}
	return k
}

// column lowers a column reference `Todo.owner`.
func (l *lowerer) column(x *syntax.Var) *ColumnRef {
	table := x.Module // `Charge.cents`; a fake's table from another module: `Payments.Charge.cents`
	if i := strings.LastIndexByte(table, '.'); i >= 0 {
		table = table[i+1:]
	}
	tb := l.tables[key(x.Ref.Module, table)]
	if tb == nil {
		panic(fmt.Sprintf("ir: unknown table %s.%s at %s", x.Ref.Module, x.Module, pos(x.Span)))
	}
	return &ColumnRef{Typed: Typed{l.typeOf(x)}, Table: tb, Field: x.Name}
}
