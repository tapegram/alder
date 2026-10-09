package ir

import (
	"fmt"
	"strings"

	"alder/internal/syntax"
	"alder/internal/types"
)

// expr lowers an expression.
func (l *lowerer) expr(e syntax.Expr) Expr {
	switch x := e.(type) {
	case *syntax.IntLit:
		return &Lit{Typed: Typed{l.typeOf(x)}, Kind: LitInt, Int: x.Value}
	case *syntax.FloatLit:
		return &Lit{Typed: Typed{l.typeOf(x)}, Kind: LitFloat, Float: x.Value}
	case *syntax.BoolLit:
		return &Lit{Typed: Typed{l.typeOf(x)}, Kind: LitBool, Bool: x.Value}
	case *syntax.UnitLit:
		return &Lit{Typed: Typed{l.typeOf(x)}, Kind: LitUnit}
	case *syntax.StrLit:
		return l.str(x)
	case *syntax.ListLit:
		out := &List{Typed: Typed{l.typeOf(x)}}
		for _, el := range x.Elems {
			out.Elems = append(out.Elems, l.expr(el))
		}
		return out
	case *syntax.RecordLit:
		t := l.typeOf(x)
		return l.record(x, l.recordType(t), t)
	case *syntax.RecordUpdate:
		out := &Update{Typed: Typed{l.typeOf(x)}, Base: l.expr(x.Base)}
		for _, f := range x.Fields {
			out.Fields = append(out.Fields, FieldInit{Name: f.Name, Value: l.expr(f.Value)})
		}
		return out
	case *syntax.Var:
		return l.variable(x)
	case *syntax.CtorRef:
		return l.ctorRef(x)
	case *syntax.Access:
		return &Field{Typed: Typed{l.typeOf(x)}, Expr: l.expr(x.Expr), Name: x.Field}
	case *syntax.App:
		return l.app(x.Fn, x.Args, nil, l.typeOf(x))
	case *syntax.Lambda:
		out := &Lambda{Typed: Typed{l.typeOf(x)}}
		for _, p := range x.Params {
			out.Params = append(out.Params, &Param{Name: p.Name, Type: l.bindType(p)})
		}
		out.Body = l.expr(x.Body)
		return out
	case *syntax.If:
		return &If{Typed: Typed{l.typeOf(x)}, Cond: l.expr(x.Cond), Then: l.expr(x.Then), Else: l.expr(x.Else)}
	case *syntax.Match:
		return l.match(x.Subject, x.Arms, l.typeOf(x), false)
	case *syntax.Block:
		return l.block(x.Stmts, x.Result, l.typeOf(x))
	case *syntax.BinOp:
		if x.Op == "|>" {
			return l.pipe(x)
		}
		return &Op{Typed: Typed{l.typeOf(x)}, Kind: x.Op, L: l.expr(x.L), R: l.expr(x.R), OperandT: l.typeOf(x.L)}
	case *syntax.HoleExpr:
		return &Hole{Typed: Typed{l.typeOf(x)}, Name: x.Name}
	case *syntax.Element, *syntax.HtmlText, *syntax.HtmlExpr, *syntax.HtmlFor, *syntax.HtmlIf, *syntax.HtmlMatch:
		return l.htmlNode(e)
	}
	panic(fmt.Sprintf("ir: cannot lower expression %T at %s", e, pos(e.GetSpan())))
}

// str lowers a string literal: a Text Lit when it has no interpolation, else a Str.
func (l *lowerer) str(x *syntax.StrLit) Expr {
	t := l.typeOf(x)
	interpolated := false
	var text strings.Builder
	for _, p := range x.Parts {
		interpolated = interpolated || p.Expr != nil
		text.WriteString(p.Text)
	}
	if !interpolated {
		return &Lit{Typed: Typed{t}, Kind: LitText, Text: text.String()}
	}
	out := &Str{Typed: Typed{t}}
	for _, p := range x.Parts {
		if p.Expr != nil {
			out.Parts = append(out.Parts, l.expr(p.Expr))
		} else if p.Text != "" {
			out.Parts = append(out.Parts, &Lit{Typed: Typed{types.Text}, Kind: LitText, Text: p.Text})
		}
	}
	return out
}

// record lowers a record literal of type rt: fields in rt's order, omitted fields filled with
// their defaults; a message payload's `form` is left to the runtime.
func (l *lowerer) record(x *syntax.RecordLit, rt *RecordType, t types.Type) *Record {
	given := map[string]syntax.Expr{}
	for _, f := range x.Fields {
		given[f.Name] = f.Value
	}
	return l.recordOf(rt, t, given, pos(x.Span))
}

// recordOf builds a Record of rt from the given field expressions (nil entries mean literal True,
// for bare component attributes).
func (l *lowerer) recordOf(rt *RecordType, t types.Type, given map[string]syntax.Expr, at string) *Record {
	out := &Record{Typed: Typed{t}, T: rt}
	for _, f := range rt.Fields {
		v, ok := given[f.Name]
		switch {
		case ok && v == nil:
			out.Fields = append(out.Fields, FieldInit{Name: f.Name, Value: &Lit{Typed: Typed{types.Bool}, Kind: LitBool, Bool: true}})
		case ok:
			out.Fields = append(out.Fields, FieldInit{Name: f.Name, Value: l.expr(v)})
		case f.Default != nil:
			out.Fields = append(out.Fields, FieldInit{Name: f.Name, Value: f.Default})
		case f.Name == "form" && l.msgPayloads[rt]: // the runtime supplies the submitted form
		default:
			panic(fmt.Sprintf("ir: record at %s has no field %s and no default for it", at, f.Name))
		}
	}
	return out
}

func (l *lowerer) variable(x *syntax.Var) Expr {
	t := l.typeOf(x)
	switch x.Ref.Kind {
	case syntax.RefLocal:
		return &Var{Typed: Typed{t}, Kind: VarLocal, Name: x.Name}
	case syntax.RefValue:
		return &Var{Typed: Typed{t}, Kind: VarGlobal, Module: x.Ref.Module, Name: x.Ref.Name}
	case syntax.RefStd:
		return &Var{Typed: Typed{t}, Kind: VarStd, Module: x.Ref.Module, Name: x.Ref.Name}
	}
	panic(fmt.Sprintf("ir: %s.%s at %s is a database or capability reference, not lowered in M3a", x.Module, x.Name, pos(x.Span)))
}

// ctorRef lowers a constructor, page or brand name used as a value (not applied).
func (l *lowerer) ctorRef(x *syntax.CtorRef) Expr {
	t := l.typeOf(x)
	fn, isFn := t.(*types.Fun)
	switch x.Ref.Kind {
	case syntax.RefPage:
		if !isFn {
			return &PageRef{Typed: Typed{t}, Page: l.page(x.Ref.Module)}
		}
		return l.eta(fn, func(arg Expr) Expr { return &PageRef{Typed: Typed{fn.Result}, Page: l.page(x.Ref.Module), Params: arg} })
	case syntax.RefCtor:
		info := l.res.Ctors[key(x.Ref.Module, x.Ref.Name)]
		if info == nil { // a brand constructor: erased to the identity
			return l.eta(fn, func(arg Expr) Expr { return arg })
		}
		if isFn {
			return l.eta(fn, func(arg Expr) Expr { return l.ctor(fn.Result, x.Ref.Name, arg, nil) })
		}
		// no payload, or a message whose payload the runtime supplies (only a `form`)
		u := l.union(t.(*types.Union))
		ct := u.ctorT(x.Ref.Name)
		out := &Ctor{Typed: Typed{t}, U: u, Name: x.Ref.Name}
		if ct.Payload != nil {
			out.Payload = &Record{Typed: Typed{l.payloadType(t, x.Ref.Name)}, T: ct.Payload}
		}
		return out
	}
	panic(fmt.Sprintf("ir: %s at %s is a table reference, not lowered in M3a", x.Name, pos(x.Span)))
}

// eta wraps a one-parameter operation as a function value: \v -> body(v).
func (l *lowerer) eta(fn *types.Fun, body func(Expr) Expr) Expr {
	name := l.temp()
	arg := &Var{Typed: Typed{fn.Params[0]}, Kind: VarLocal, Name: name}
	return &Lambda{Typed: Typed{fn}, Params: []*Param{{Name: name, Type: fn.Params[0]}}, Body: body(arg)}
}

func (l *lowerer) page(module string) *Page {
	if pg := l.pages[module]; pg != nil {
		return pg
	}
	panic(fmt.Sprintf("ir: unknown page %s", module))
}

// payloadType is the type of constructor name's payload in union type t (with its arguments).
func (l *lowerer) payloadType(t types.Type, name string) types.Type {
	arg, _ := types.CtorArg(t.(*types.Union), name)
	return l.erase(arg)
}

// ctor builds constructor name of union type t. lit, when non-nil, is the record literal the
// payload was written as (lowered against the payload's RecordType, so it shares it); otherwise
// arg is the lowered payload (nil for none).
func (l *lowerer) ctor(t types.Type, name string, arg Expr, lit *syntax.RecordLit) Expr {
	u := l.union(t.(*types.Union))
	ct := u.ctorT(name)
	pt := l.payloadType(t, name)
	out := &Ctor{Typed: Typed{t}, U: u, Name: name}
	if lit != nil {
		arg = l.record(lit, ct.Payload, pt)
	}
	switch {
	case ct.Payload == nil: // no payload, or a message payload with no fields
	case l.wrappers[ct.Payload]:
		wrap := &types.Record{Anonymous: true, Fields: []types.Field{{Name: "value", Type: pt}}}
		out.Payload = &Record{Typed: Typed{wrap}, T: ct.Payload, Fields: []FieldInit{{Name: "value", Value: arg}}}
	default:
		out.Payload = arg
	}
	return out
}

// app lowers fn applied to args, with piped (the left side of `|>`, already lowered) as the
// argument right after them. t is the type of the whole application.
func (l *lowerer) app(fn syntax.Expr, args []syntax.Expr, piped Expr, t types.Type) Expr {
	lower := func() []Expr {
		var all []Expr
		for _, a := range args {
			all = append(all, l.expr(a))
		}
		if piped != nil {
			all = append(all, piped)
		}
		return all
	}
	if ct, ok := fn.(*syntax.CtorRef); ok {
		switch ct.Ref.Kind {
		case syntax.RefPage:
			return &PageRef{Typed: Typed{t}, Page: l.page(ct.Ref.Module), Params: lower()[0]}
		case syntax.RefCtor:
			if l.res.Ctors[key(ct.Ref.Module, ct.Ref.Name)] == nil {
				return lower()[0] // a brand constructor is erased
			}
			u := l.union(t.(*types.Union))
			if piped == nil && len(args) == 1 && !l.wrappers[u.ctorT(ct.Ref.Name).Payload] {
				if lit, isLit := args[0].(*syntax.RecordLit); isLit {
					return l.ctor(t, ct.Ref.Name, nil, lit)
				}
			}
			return l.ctor(t, ct.Ref.Name, lower()[0], nil)
		}
		panic(fmt.Sprintf("ir: %s at %s is a table reference, not lowered in M3a", ct.Name, pos(ct.Span)))
	}
	callee := l.expr(fn)
	all := lower()
	ft, ok := callee.Type().(*types.Fun)
	if !ok {
		panic(fmt.Sprintf("ir: application of non-function %s at %s", types.String(callee.Type()), pos(fn.GetSpan())))
	}
	if len(all) < len(ft.Params) {
		return &Partial{Typed: Typed{t}, Fn: callee, Given: all, Missing: len(ft.Params) - len(all)}
	}
	return &Call{Typed: Typed{t}, Fn: callee, Args: all}
}

// pipe lowers `l |> f a…` to the application of f to a… and then l.
func (l *lowerer) pipe(x *syntax.BinOp) Expr {
	// `{ … } |> C`: the literal is the constructor's payload, sharing its RecordType
	if ct, ok := x.R.(*syntax.CtorRef); ok && ct.Ref.Kind == syntax.RefCtor && l.res.Ctors[key(ct.Ref.Module, ct.Ref.Name)] != nil {
		if lit, isLit := x.L.(*syntax.RecordLit); isLit {
			t := l.typeOf(x)
			if payload := l.union(t.(*types.Union)).ctorT(ct.Ref.Name).Payload; payload != nil && !l.wrappers[payload] {
				return l.ctor(t, ct.Ref.Name, nil, lit)
			}
		}
	}
	arg := l.expr(x.L)
	if app, ok := x.R.(*syntax.App); ok {
		return l.app(app.Fn, app.Args, arg, l.typeOf(x))
	}
	return l.app(x.R, nil, arg, l.typeOf(x))
}

// block lowers a block's `let`s to nested Lets around its result.
func (l *lowerer) block(stmts []*syntax.LetStmt, result syntax.Expr, t types.Type) Expr {
	if len(stmts) == 0 {
		return l.expr(result)
	}
	s := stmts[0]
	if s.Bind {
		panic(fmt.Sprintf("ir: `let %s <-` at %s is an effect, not lowered in M3a", s.Name, pos(s.Span)))
	}
	return &Let{Typed: Typed{t}, Name: s.Name, T: l.bindType(s), Value: l.expr(s.Expr), Body: l.block(stmts[1:], result, t)}
}
