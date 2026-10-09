// Expression checking: literals, operators, application, records, blocks and effect rules.
package check

import (
	"fmt"
	"sort"
	"strings"

	"alder/internal/diag"
	"alder/internal/syntax"
	"alder/internal/types"
)

type env struct {
	parent *env
	vars   map[string]types.Type
}

func newEnv() *env         { return &env{vars: map[string]types.Type{}} }
func (e *env) child() *env { return &env{parent: e, vars: map[string]types.Type{}} }
func (e *env) bind(n string, t types.Type) {
	if n != "_" {
		e.vars[n] = t
	}
}
func (e *env) lookup(n string) (types.Type, bool) {
	for c := e; c != nil; c = c.parent {
		if t, ok := c.vars[n]; ok {
			return t, true
		}
	}
	return nil, false
}

type effect struct {
	kind string // "", "Query", "Task"
}

// checkBodies checks every value, test and brand check in the module (page/HTML rules hook in from Tasks 10-11).
func (mc *modCtx) checkBodies() {
	c := mc.c
	if c.prog.Std[mc.m.Name] != nil {
		return
	}
	for _, d := range c.live(mc.m) {
		switch d := d.(type) {
		case *syntax.ValueDecl:
			sig, ok := c.values[c.key(mc.m.Name, d.Name)]
			if !ok || d.Body == nil {
				continue
			}
			mc.holes = nil
			env := newEnv()
			eff := effect{}
			body := sig
			if fn, isFn := sig.(*types.Fun); isFn {
				if len(d.Params) > len(fn.Params) {
					c.errorf(diag.Arity, d.Span, "`%s` takes %d parameters but %d are named", d.Name, len(fn.Params), len(d.Params))
					continue
				}
				for i, p := range d.Params {
					env.bind(p.Name, fn.Params[i])
				}
				if len(d.Params) < len(fn.Params) {
					body = &types.Fun{Params: fn.Params[len(d.Params):], Result: fn.Result}
				} else {
					body = fn.Result
				}
			} else if len(d.Params) > 0 {
				c.errorf(diag.Arity, d.Span, "`%s` has type `%s`, which takes no parameters", d.Name, types.String(sig))
				continue
			}
			mc.inUpdate = (mc.page != nil && d.Name == "update") || (mc.isApp && d.Name == "update")
			mc.check(d.Body, body, env, eff)
			mc.inUpdate = false
			mc.reportHoles()
		case *syntax.TestDecl:
			mc.holes = nil
			mc.inTest = true
			mc.check(d.Body, types.Bool, newEnv(), effect{})
			mc.inTest = false
			mc.reportHoles()
		case *syntax.TypeDecl:
			if d.Brand != nil && d.Brand.Check != nil {
				mc.holes = nil
				mc.check(d.Brand.Check, &types.Fun{Params: []types.Type{baseType(d.Brand.Base)}, Result: types.Bool}, newEnv(), effect{})
				mc.reportHoles()
			}
		}
	}
	mc.checkPage() // Task 11; no-op for non-page modules
}

func (mc *modCtx) reportHoles() {
	for _, h := range mc.holes {
		mc.c.bag.Warn("W0002", h.span, "hole ?%s has type %s", h.name, types.String(mc.c.u.Apply(h.t)))
	}
	mc.holes = nil
}

// maybeOf builds `Maybe inner` from the declared Prelude union.
func (mc *modCtx) maybeOf(inner types.Type) types.Type {
	return mc.c.types["Prelude.Maybe"].(*types.Union).WithArgs([]types.Type{inner})
}

func (mc *modCtx) mismatch(span diag.Span, got, expected types.Type) *diag.Diagnostic {
	c := mc.c
	g, e := c.u.Apply(got), c.u.Apply(expected)
	msg := fmt.Sprintf("expected %s, got %s", types.String(e), types.String(g))
	if inner, ok := maybeOf(g); ok && c.u.Check(inner, e) == nil {
		msg += "; match it: match m { Just x -> …, Nothing -> … }"
	}
	return c.errorf(diag.TypeMismatch, span, "%s", msg)
}

// check verifies e against expected, pushing the expectation into literals, lambdas, branches and blocks.
func (mc *modCtx) check(e syntax.Expr, expected types.Type, env *env, eff effect) {
	c := mc.c
	expected = c.u.Apply(expected)
	switch x := e.(type) {
	case *syntax.RecordLit:
		if rec, ok := expected.(*types.Record); ok {
			mc.checkRecordLit(x, rec, env, eff)
			return
		}
	case *syntax.ListLit:
		if app, ok := expected.(*types.App); ok && app.Con == "List" {
			for _, el := range x.Elems {
				mc.check(el, app.Args[0], env, eff)
			}
			return
		}
	case *syntax.Lambda:
		if fn, ok := expected.(*types.Fun); ok {
			if len(x.Params) > len(fn.Params) {
				c.errorf(diag.Arity, x.Span, "lambda has %d parameters but %d are expected", len(x.Params), len(fn.Params))
				return
			}
			child := env.child()
			for i, p := range x.Params {
				child.bind(p.Name, fn.Params[i])
			}
			body := fn.Result
			if len(x.Params) < len(fn.Params) {
				body = &types.Fun{Params: fn.Params[len(x.Params):], Result: fn.Result}
			}
			mc.check(x.Body, body, child, eff)
			return
		}
	case *syntax.If:
		mc.check(x.Cond, types.Bool, env, eff)
		mc.check(x.Then, expected, env, eff)
		mc.check(x.Else, expected, env, eff)
		return
	case *syntax.Match:
		mc.checkMatch(x, expected, env, eff)
		return
	case *syntax.Block:
		mc.checkBlock(x, expected, env, eff)
		return
	case *syntax.Element, *syntax.HtmlFor, *syntax.HtmlIf, *syntax.HtmlMatch, *syntax.HtmlExpr, *syntax.HtmlText:
		mc.checkHTML(e, expected, env, eff)
		return
	case *syntax.HoleExpr:
		mc.holes = append(mc.holes, holeNote{x.Name, expected, x.Span})
		return
	}
	var got types.Type
	if app, ok := e.(*syntax.App); ok {
		got = mc.inferApp(app, env, eff, nil, expected)
	} else {
		got = mc.infer(e, env, eff)
	}
	if err := c.u.Check(got, expected); err != nil {
		d := mc.mismatch(e.GetSpan(), got, expected)
		mc.annotateMismatch(d, e, got, expected)
	}
}

// annotateMismatch adds the fixes the spec lists for common mismatches. d is only valid until the next diagnostic.
func (mc *modCtx) annotateMismatch(d *diag.Diagnostic, e syntax.Expr, got, expected types.Type) {
	c := mc.c
	g, ex := c.u.Apply(got), c.u.Apply(expected)
	if isCon(ex, "Text") {
		sp := e.GetSpan()
		src := mc.source(sp)
		switch {
		case isCon(g, "Int"):
			d.Fix = diag.ReplaceSpan(sp, "Text.fromInt ("+src+")")
		case isCon(g, "Float"):
			d.Fix = diag.ReplaceSpan(sp, "Text.fromFloat ("+src+")")
		}
	}
}

// source returns the text of a span (the rest of the first line when it spans lines).
func (mc *modCtx) source(sp diag.Span) string {
	line := lineOf(mc.m, sp.Start.Line)
	if sp.Start.Line != sp.End.Line {
		if sp.Start.Col-1 <= len(line) {
			return strings.TrimSpace(line[sp.Start.Col-1:])
		}
		return ""
	}
	if sp.End.Col-1 > len(line) || sp.Start.Col-1 > len(line) || sp.End.Col < sp.Start.Col {
		return ""
	}
	return strings.TrimSpace(line[sp.Start.Col-1 : sp.End.Col-1])
}

func (mc *modCtx) checkRecordLit(x *syntax.RecordLit, rec *types.Record, env *env, eff effect) {
	c := mc.c
	byName := map[string]types.Field{}
	for _, f := range rec.Fields {
		byName[f.Name] = f
	}
	seen := map[string]bool{}
	for _, fi := range x.Fields {
		f, ok := byName[fi.Name]
		if !ok {
			if strings.HasPrefix(rec.Name, "Insert ") {
				if info := c.tables[c.key(rec.Module, strings.TrimPrefix(rec.Name, "Insert "))]; info != nil {
					if _, isCol := info.Columns[fi.Name]; isCol {
						c.errorf("E0501", fi.Span, "`Db.insert` never takes `%s`; it is generated", fi.Name)
						mc.infer(fi.Value, env, eff)
						continue
					}
				}
			}
			c.errorf(diag.UnknownName, fi.Span, "no field `%s` on `%s`", fi.Name, types.String(rec))
			mc.infer(fi.Value, env, eff)
			continue
		}
		if seen[fi.Name] {
			c.errorf(diag.Duplicate, fi.Span, "field `%s` is given twice", fi.Name)
		}
		seen[fi.Name] = true
		mc.check(fi.Value, f.Type, env, eff)
	}
	var missing []string
	for _, f := range rec.Fields {
		if !seen[f.Name] && !f.HasDefault && f.Name != "form" {
			missing = append(missing, f.Name)
		}
	}
	if len(missing) > 0 {
		c.errorf(diag.TypeMismatch, x.Span, "record `%s` is missing field(s): %s", types.String(rec), strings.Join(missing, ", "))
	}
}

func (mc *modCtx) checkMatch(x *syntax.Match, expected types.Type, env *env, eff effect) {
	subj := mc.c.u.Apply(mc.infer(x.Subject, env, eff))
	ex := newExhaustiveness(mc, subj)
	for _, a := range x.Arms {
		child := env.child()
		if ex.closed {
			mc.c.errorf(diag.UnreachableArm, a.Span, "unreachable arm: every case is already handled")
		}
		mc.checkPattern(a.Pat, subj, child)
		ex.refresh(subj)
		ex.cover(a.Pat)
		mc.check(a.Body, expected, child, eff)
	}
	ex.report(x)
}

func queryOrTask(t types.Type) (string, types.Type) {
	if app, ok := t.(*types.App); ok && (app.Con == "Query" || app.Con == "Task") && len(app.Args) == 1 {
		return app.Con, app.Args[0]
	}
	return "", nil
}

func (mc *modCtx) checkBlock(x *syntax.Block, expected types.Type, env *env, eff effect) {
	c := mc.c
	expected = c.u.Apply(expected)
	kind, _ := queryOrTask(expected)
	_, expectedUnknown := expected.(*types.Var)
	child := env.child()
	sub := func() effect { return effect{kind: kind} }
	for _, s := range x.Stmts {
		var t types.Type
		if s.Type != nil {
			t = mc.typeOf(s.Type, nil)
		}
		if !s.Bind {
			if t != nil {
				mc.check(s.Expr, t, child, sub())
			} else {
				t = mc.infer(s.Expr, child, sub())
			}
			child.bind(s.Name, t)
			continue
		}
		var got types.Type
		if kind == "" && expectedUnknown {
			// no expectation: the first bound expression decides what kind of block this is
			got = c.u.Apply(mc.infer(s.Expr, child, sub()))
			if k, _ := queryOrTask(got); k != "" {
				kind = k
				c.u.Unify(expected, &types.App{Con: k, Args: []types.Type{c.u.Fresh()}})
				expected = c.u.Apply(expected)
				expectedUnknown = false
			}
		}
		inner := t
		if inner == nil {
			inner = c.u.Fresh()
		}
		if kind == "" {
			c.errorf("E0402", s.Span, "`let %s <-` is only allowed in a block that returns `Query` or `Task`", s.Name)
			if got == nil {
				mc.infer(s.Expr, child, sub())
			}
			child.bind(s.Name, inner)
			continue
		}
		if got == nil {
			got = c.u.Apply(mc.infer(s.Expr, child, sub()))
		}
		k, arg := queryOrTask(got)
		switch {
		case k == kind, k == "Query" && kind == "Task":
			if err := c.u.Check(arg, inner); err != nil {
				mc.mismatch(s.Expr.GetSpan(), arg, inner)
			}
		case k == "Task" && kind == "Query":
			c.errorf("E0401", s.Expr.GetSpan(), "`Task` cannot run inside a `Query`; `load` and `Query` blocks can only read")
		default:
			if _, isVar := got.(*types.Var); isVar {
				c.u.Unify(got, &types.App{Con: kind, Args: []types.Type{inner}})
			} else if err := c.u.Check(got, &types.App{Con: kind, Args: []types.Type{inner}}); err != nil {
				mc.mismatch(s.Expr.GetSpan(), got, &types.App{Con: kind, Args: []types.Type{inner}})
			}
		}
		child.bind(s.Name, inner)
	}
	if x.Result != nil {
		mc.check(x.Result, expected, child, sub())
	}
}

// infer returns e's type, reporting errors inside e.
func (mc *modCtx) infer(e syntax.Expr, env *env, eff effect) types.Type {
	c := mc.c
	switch x := e.(type) {
	case *syntax.IntLit:
		return types.Int
	case *syntax.FloatLit:
		return types.Float
	case *syntax.BoolLit:
		return types.Bool
	case *syntax.UnitLit:
		return types.Unit
	case *syntax.StrLit:
		for _, p := range x.Parts {
			if p.Expr != nil {
				mc.check(p.Expr, types.Text, env, eff)
			}
		}
		return types.Text
	case *syntax.ListLit:
		elem := c.u.Fresh()
		for _, el := range x.Elems {
			mc.check(el, elem, env, eff)
		}
		return &types.App{Con: "List", Args: []types.Type{elem}}
	case *syntax.RecordLit:
		rec := &types.Record{Anonymous: true}
		for _, f := range x.Fields {
			rec.Fields = append(rec.Fields, types.Field{Name: f.Name, Type: mc.infer(f.Value, env, eff)})
		}
		return rec
	case *syntax.RecordUpdate:
		base := c.u.Apply(mc.infer(x.Base, env, eff))
		rec, ok := base.(*types.Record)
		if !ok {
			c.errorf(diag.TypeMismatch, x.Span, "record update on a non-record `%s`", types.String(base))
			for _, fi := range x.Fields {
				mc.infer(fi.Value, env, eff)
			}
			return base
		}
		for _, fi := range x.Fields {
			found := false
			for _, f := range rec.Fields {
				if f.Name == fi.Name {
					found = true
					mc.check(fi.Value, f.Type, env, eff)
				}
			}
			if !found {
				c.errorf(diag.UnknownName, fi.Span, "no field `%s` on `%s`", fi.Name, types.String(rec))
				mc.infer(fi.Value, env, eff)
			}
		}
		return rec
	case *syntax.Var:
		return mc.inferVar(x, env)
	case *syntax.CtorRef:
		return mc.inferCtor(x, false)
	case *syntax.Access:
		return mc.inferAccess(x, env, eff)
	case *syntax.App:
		return mc.inferApp(x, env, eff, nil, nil)
	case *syntax.Lambda:
		child := env.child()
		fn := &types.Fun{}
		for _, p := range x.Params {
			v := c.u.Fresh()
			child.bind(p.Name, v)
			fn.Params = append(fn.Params, v)
		}
		fn.Result = mc.infer(x.Body, child, eff)
		return fn
	case *syntax.If:
		mc.check(x.Cond, types.Bool, env, eff)
		t := mc.infer(x.Then, env, eff)
		mc.check(x.Else, t, env, eff)
		return t
	case *syntax.Match:
		v := c.u.Fresh()
		mc.checkMatch(x, v, env, eff)
		return v
	case *syntax.Block:
		v := c.u.Fresh()
		mc.checkBlock(x, v, env, eff)
		return v
	case *syntax.BinOp:
		return mc.inferBinOp(x, env, eff)
	case *syntax.HoleExpr:
		v := c.u.Fresh()
		mc.holes = append(mc.holes, holeNote{x.Name, v, x.Span})
		return v
	case *syntax.Element, *syntax.HtmlFor, *syntax.HtmlIf, *syntax.HtmlMatch, *syntax.HtmlExpr, *syntax.HtmlText:
		v := &types.App{Con: "Html", Args: []types.Type{c.u.Fresh()}}
		mc.checkHTML(e, v, env, eff)
		return v
	}
	return c.u.Fresh()
}

func (mc *modCtx) inferVar(x *syntax.Var, env *env) types.Type {
	c := mc.c
	switch x.Ref.Kind {
	case syntax.RefLocal:
		if t, ok := env.lookup(x.Name); ok {
			return t
		}
	case syntax.RefValue, syntax.RefStd:
		if x.Ref.Module == "Session" && x.Ref.Name == "put" {
			c.usesSessionPut = true
		}
		if t, ok := c.values[c.key(x.Ref.Module, x.Ref.Name)]; ok {
			return c.u.Instantiate(t)
		}
	case syntax.RefColumn:
		if info, ok := c.tables[c.key(x.Ref.Module, x.Module)]; ok {
			return &types.App{Con: "Column", Args: []types.Type{info.Record, info.Columns[x.Name]}}
		}
	case syntax.RefCapability:
		if entries, ok := c.caps[c.key(x.Ref.Module, x.Module)]; ok {
			if t, ok := entries[x.Name]; ok {
				return c.u.Instantiate(t)
			}
			c.errorf(diag.UnknownName, x.Span, "capability `%s` has no `%s`", x.Module, x.Name)
		}
	}
	return c.u.Fresh()
}

// isMsgUnion reports whether u is App.Msg or a page's Msg.
func (mc *modCtx) isMsgUnion(u *types.Union) bool {
	c := mc.c
	if u == c.appMsg {
		return true
	}
	p, ok := c.pages[u.Module]
	return ok && p.Msg == u
}

// inferCtor types a constructor reference. applied=true means the reference is the head of an application.
func (mc *modCtx) inferCtor(x *syntax.CtorRef, applied bool) types.Type {
	c := mc.c
	switch x.Ref.Kind {
	case syntax.RefPage:
		info, ok := c.pages[x.Ref.Module]
		if !ok {
			return types.Page
		}
		if len(info.Params.Fields) == 0 && !applied {
			return types.Page
		}
		return &types.Fun{Params: []types.Type{info.Params}, Result: types.Page}
	case syntax.RefTable:
		if info, ok := c.tables[c.key(x.Ref.Module, x.Ref.Name)]; ok {
			return &types.App{Con: "Table", Args: []types.Type{info.Record}}
		}
	case syntax.RefCtor:
		info, ok := c.ctors[c.key(x.Ref.Module, x.Ref.Name)]
		if !ok {
			if b, isBrand := c.types[c.key(x.Ref.Module, x.Ref.Name)].(*types.Brand); isBrand {
				if x.Ref.Module != mc.m.Name && !mc.inTest {
					c.errorf(diag.InvalidDecl, x.Span, "brand constructor `%s` can only be used in module `%s` or inside a `test`", x.Name, x.Ref.Module)
				}
				return &types.Fun{Params: []types.Type{b.Base}, Result: b}
			}
			return c.u.Fresh()
		}
		un := info.Union.WithArgs(paramsOf(info.Union.Params))
		if len(info.Union.Params) == 0 {
			un = info.Union
		}
		if info.Arg == nil {
			return c.u.Instantiate(un)
		}
		if rec, isRec := info.Arg.(*types.Record); isRec && !applied && mc.isMsgUnion(info.Union) {
			if messageLike(rec) {
				return un
			}
			var parts []string
			for _, f := range rec.Fields {
				if f.Name != "form" {
					parts = append(parts, f.Name+": ?"+f.Name)
				}
			}
			full := fmt.Sprintf("%s { %s }", x.Name, strings.Join(parts, ", "))
			c.errorf("E0301", x.Span, "message `%s` needs its parameters: `%s`", x.Name, full).Fix =
				diag.ReplaceSpan(x.Span, full)
			return un
		}
		arg := info.Arg
		if len(info.Union.Params) > 0 {
			arg, _ = types.CtorArg(un, x.Ref.Name)
		}
		return c.u.Instantiate(&types.Fun{Params: []types.Type{arg}, Result: un})
	}
	return c.u.Fresh()
}

func paramsOf(names []string) []types.Type {
	var out []types.Type
	for _, n := range names {
		out = append(out, &types.Param{Name: n})
	}
	return out
}

// messageLike reports whether a ctor's payload record has no HTML-supplied fields (only `form` or none).
func messageLike(rec *types.Record) bool {
	for _, f := range rec.Fields {
		if f.Name != "form" {
			return false
		}
	}
	return true
}

func (mc *modCtx) inferAccess(x *syntax.Access, env *env, eff effect) types.Type {
	c := mc.c
	base := c.u.Apply(mc.infer(x.Expr, env, eff))
	switch b := base.(type) {
	case *types.Record:
		for _, f := range b.Fields {
			if f.Name == x.Field {
				return f.Type
			}
		}
		c.errorf(diag.UnknownName, x.Span, "no field `%s` on `%s`", x.Field, types.String(b))
	case *types.Union:
		if inner, ok := maybeOf(b); ok {
			src := mc.source(x.Expr.GetSpan())
			c.errorf("E0102", x.Span, "`.%s` on a `Maybe %s`; match it first", x.Field, types.String(inner)).Fix =
				diag.ReplaceSpan(x.Span, fmt.Sprintf("(match %s { Just x -> x.%s, Nothing -> ?default })", src, x.Field))
			return c.u.Fresh()
		}
		c.errorf(diag.TypeMismatch, x.Span, "`%s` has no fields; it is a union", types.String(b))
	case *types.App:
		if b.Con == "Column" && len(b.Args) == 2 {
			return mc.columnMethod(x, b.Args[0], b.Args[1])
		}
		c.errorf(diag.TypeMismatch, x.Span, "`%s` has no field `%s`", types.String(b), x.Field)
	case *types.Var:
		c.errorf(diag.TypeMismatch, x.Span, "cannot infer the record type of `%s`; add a type annotation", mc.source(x.Expr.GetSpan()))
	default:
		c.errorf(diag.TypeMismatch, x.Span, "`%s` has no field `%s`", types.String(b), x.Field)
	}
	return c.u.Fresh()
}

func (mc *modCtx) columnMethod(x *syntax.Access, table, field types.Type) types.Type {
	c := mc.c
	cond := &types.App{Con: "Condition", Args: []types.Type{table}}
	order := &types.App{Con: "Order", Args: []types.Type{table}}
	ft := c.u.Apply(field)
	isTextLike := isCon(ft, "Text") || func() bool { b, ok := ft.(*types.Brand); return ok && isCon(b.Base, "Text") }()
	switch x.Field {
	case "eq", "ne":
		return &types.Fun{Params: []types.Type{ft}, Result: cond}
	case "lt", "le", "gt", "ge":
		if !comparable(ft) {
			c.errorf(diag.TypeMismatch, x.Span, "`.%s` needs an ordered column, got `%s`", x.Field, types.String(ft))
		}
		return &types.Fun{Params: []types.Type{ft}, Result: cond}
	case "contains":
		if !isTextLike {
			c.errorf(diag.TypeMismatch, x.Span, "`.contains` needs a Text column, got `%s`", types.String(ft))
		}
		return &types.Fun{Params: []types.Type{types.Text}, Result: cond}
	case "in":
		return &types.Fun{Params: []types.Type{&types.App{Con: "List", Args: []types.Type{ft}}}, Result: cond}
	case "isNothing", "isJust":
		if _, ok := maybeOf(ft); !ok {
			c.errorf(diag.TypeMismatch, x.Span, "`.%s` needs a Maybe column, got `%s`", x.Field, types.String(ft))
		}
		return cond
	case "asc", "desc":
		return order
	}
	c.errorf(diag.UnknownName, x.Span, "columns have no `.%s`; use .eq .ne .lt .le .gt .ge .contains .in .isNothing .isJust .asc .desc", x.Field)
	return c.u.Fresh()
}

func comparable(t types.Type) bool {
	switch t := t.(type) {
	case *types.Con:
		return t.Name == "Int" || t.Name == "Float" || t.Name == "Text" || t.Name == "Date" || t.Name == "Time"
	case *types.Brand:
		return true
	case *types.Union:
		return isScalar(t)
	}
	return false
}

// pipeArg is the left side of `|>`, supplied as the final argument of the application on its right.
type pipeArg struct {
	t    types.Type
	span diag.Span
}

// inferApp types an application; want, when non-nil, is the type the context expects and is pushed into the result.
func (mc *modCtx) inferApp(x *syntax.App, env *env, eff effect, pipe *pipeArg, want types.Type) types.Type {
	c := mc.c
	// constructor applied to a record literal: check against the payload (form field implicit)
	if ct, ok := x.Fn.(*syntax.CtorRef); ok && len(x.Args) == 1 && pipe == nil {
		if lit, isLit := x.Args[0].(*syntax.RecordLit); isLit {
			fnT := c.u.Apply(mc.inferCtor(ct, true))
			if fn, isFn := fnT.(*types.Fun); isFn && len(fn.Params) == 1 {
				if rec, isRec := c.u.Apply(fn.Params[0]).(*types.Record); isRec {
					mc.checkRecordLit(lit, rec, env, eff)
					return fn.Result
				}
			}
		}
	}
	// E0301: a message constructor applied to anything but a record literal
	if ct, ok := x.Fn.(*syntax.CtorRef); ok && pipe == nil && ct.Ref.Kind == syntax.RefCtor && len(x.Args) > 0 {
		if info, found := c.ctors[c.key(ct.Ref.Module, ct.Ref.Name)]; found && info.Union.Name == "Msg" {
			if _, isLit := x.Args[0].(*syntax.RecordLit); !isLit {
				if rec, isRec := info.Arg.(*types.Record); isRec {
					var parts []string
					for _, f := range rec.Fields {
						if f.Name != "form" {
							parts = append(parts, f.Name+": ?"+f.Name)
						}
					}
					full := ct.Name + " { " + strings.Join(parts, ", ") + " }"
					c.errorf("E0301", x.Span, "message `%s` takes a record: `%s`", ct.Name, full).Fix = diag.ReplaceSpan(x.Span, full)
					for _, a := range x.Args {
						mc.infer(a, env, eff)
					}
					return info.Union
				}
			}
		}
	}
	// E0101: Db.get T x with x not an id
	if v, ok := x.Fn.(*syntax.Var); ok && v.Module == "Db" && v.Name == "get" && len(x.Args) == 2 && pipe == nil {
		if r := mc.checkDbGet(x, env, eff); r != nil {
			return r
		}
	}
	var fnT types.Type
	if ct, ok := x.Fn.(*syntax.CtorRef); ok {
		fnT = mc.inferCtor(ct, true)
	} else {
		fnT = mc.infer(x.Fn, env, eff)
	}
	return mc.applyFn(x.Fn, c.u.Apply(fnT), x.Args, pipe, want, x.Span, env, eff)
}

// applyFn checks args (and the piped value, which comes last) against fnT's parameters.
func (mc *modCtx) applyFn(fnExpr syntax.Expr, fnT types.Type, args []syntax.Expr, pipe *pipeArg, want types.Type, span diag.Span, env *env, eff effect) types.Type {
	c := mc.c
	fn, ok := fnT.(*types.Fun)
	if !ok {
		if v, isVar := fnT.(*types.Var); isVar {
			// unknown function type: build one from the args
			var ps []types.Type
			for _, a := range args {
				ps = append(ps, mc.infer(a, env, eff))
			}
			if pipe != nil {
				ps = append(ps, pipe.t)
			}
			res := c.u.Fresh()
			c.u.Unify(v, &types.Fun{Params: ps, Result: res})
			return res
		}
		if ct, isCtor := fnExpr.(*syntax.CtorRef); isCtor {
			c.errorf(diag.Arity, span, "`%s` takes no arguments", ct.Name)
		} else {
			c.errorf(diag.TypeMismatch, fnExpr.GetSpan(), "`%s` is not a function; it has type `%s`", mc.source(fnExpr.GetSpan()), types.String(fnT))
		}
		for _, a := range args {
			mc.infer(a, env, eff)
		}
		return c.u.Fresh()
	}
	total := len(args)
	if pipe != nil {
		total++
	}
	if total > len(fn.Params) {
		c.errorf(diag.Arity, span, "too many arguments: `%s` takes %d, got %d", mc.source(fnExpr.GetSpan()), len(fn.Params), total)
	}
	if want != nil && total == len(fn.Params) {
		if _, isVar := c.u.Apply(want).(*types.Var); !isVar {
			c.u.Check(fn.Result, want) // an error here is reported by the caller's own check
		}
	}
	// Order: explicit non-lambda arguments, then the piped value, then lambdas. The explicit
	// arguments fix type variables the piped value's parameter mentions (`p.id |> Db.delete Todo`
	// needs `Table t` bound before `Id t` can normalize), and lambdas go last so their parameter
	// types are known.
	checkArgs := func(lambdas bool) {
		for i, a := range args {
			if _, isLam := a.(*syntax.Lambda); isLam != lambdas {
				continue
			}
			if i < len(fn.Params) {
				mc.check(a, fn.Params[i], env, eff)
			} else {
				mc.infer(a, env, eff)
			}
		}
	}
	checkArgs(false)
	if pipe != nil && len(args) < len(fn.Params) {
		if err := c.u.Check(pipe.t, fn.Params[len(args)]); err != nil {
			mc.mismatch(pipe.span, pipe.t, fn.Params[len(args)])
		}
	}
	checkArgs(true)
	if total < len(fn.Params) {
		return &types.Fun{Params: fn.Params[total:], Result: fn.Result}
	}
	return fn.Result
}

func (mc *modCtx) inferBinOp(x *syntax.BinOp, env *env, eff effect) types.Type {
	c := mc.c
	switch x.Op {
	case "|>":
		l := c.u.Apply(mc.infer(x.L, env, eff))
		p := &pipeArg{t: l, span: x.L.GetSpan()}
		if app, ok := x.R.(*syntax.App); ok {
			return mc.inferApp(app, env, eff, p, nil)
		}
		fnT := c.u.Apply(mc.infer(x.R, env, eff))
		if _, ok := fnT.(*types.Fun); !ok {
			c.errorf(diag.TypeMismatch, x.R.GetSpan(), "right side of `|>` must be a function, got `%s`", types.String(fnT))
			return c.u.Fresh()
		}
		return mc.applyFn(x.R, fnT, nil, p, nil, x.Span, env, eff)
	case "??":
		inner := c.u.Fresh()
		mc.check(x.L, mc.maybeOf(inner), env, eff)
		mc.check(x.R, inner, env, eff)
		return inner
	case "&&", "||":
		mc.check(x.L, types.Bool, env, eff)
		mc.check(x.R, types.Bool, env, eff)
		return types.Bool
	case "==", "!=":
		l := c.u.Apply(mc.infer(x.L, env, eff))
		r := c.u.Apply(mc.infer(x.R, env, eff))
		if err := c.u.Unify(l, r); err != nil {
			c.errorf(diag.TypeMismatch, x.Span, "cannot compare `%s` with `%s`: `==` needs both sides to have the same type", types.String(l), types.String(r))
		} else if containsFun(c.u.Apply(l)) {
			c.errorf(diag.TypeMismatch, x.Span, "functions cannot be compared")
		}
		return types.Bool
	case "<", "<=", ">", ">=":
		// symmetric: either side may widen to the other (`c < 500` and `500 > c` for a brand of Int)
		l := c.u.Apply(mc.infer(x.L, env, eff))
		r := c.u.Apply(mc.infer(x.R, env, eff))
		if c.u.Check(l, r) == nil {
			l = r
		} else if c.u.Check(r, l) != nil {
			mc.mismatch(x.R.GetSpan(), r, l)
		}
		if l2 := c.u.Apply(l); !comparable(l2) {
			if _, isVar := l2.(*types.Var); !isVar {
				c.errorf(diag.TypeMismatch, x.Span, "`%s` is not ordered; comparison needs Int, Float, Text, Date, Time or a union without payloads", types.String(l2))
			}
		}
		return types.Bool
	case "++":
		l := c.u.Apply(mc.infer(x.L, env, eff))
		if app, ok := l.(*types.App); ok && app.Con == "List" {
			mc.check(x.R, l, env, eff)
			return l
		}
		if err := c.u.Check(l, types.Text); err != nil {
			mc.mismatch(x.L.GetSpan(), l, types.Text)
		}
		mc.check(x.R, types.Text, env, eff)
		return types.Text
	case "+", "-", "*", "/", "%":
		l := c.u.Apply(mc.infer(x.L, env, eff))
		r := c.u.Apply(mc.infer(x.R, env, eff))
		return mc.arith(x, l, r)
	}
	return c.u.Fresh()
}

func containsFun(t types.Type) bool {
	switch t := t.(type) {
	case *types.Fun:
		return true
	case *types.App:
		for _, a := range t.Args {
			if containsFun(a) {
				return true
			}
		}
	case *types.Union:
		for _, a := range t.Args {
			if containsFun(a) {
				return true
			}
		}
	case *types.Record:
		for _, f := range t.Fields {
			if containsFun(f.Type) {
				return true
			}
		}
	}
	return false
}

func numericBase(t types.Type) (types.Type, bool) {
	switch t := t.(type) {
	case *types.Con:
		if t.Name == "Int" || t.Name == "Float" {
			return t, true
		}
	case *types.Brand:
		if b, ok := t.Base.(*types.Con); ok && (b.Name == "Int" || b.Name == "Float") {
			return t, true
		}
	}
	return nil, false
}

func (mc *modCtx) arith(x *syntax.BinOp, l, r types.Type) types.Type {
	c := mc.c
	if v, ok := l.(*types.Var); ok {
		c.u.Unify(v, r)
		l = c.u.Apply(l)
	}
	if v, ok := r.(*types.Var); ok {
		c.u.Unify(v, l)
		r = c.u.Apply(r)
	}
	lt, lok := numericBase(l)
	_, rok := numericBase(r)
	if !lok || !rok {
		msg := fmt.Sprintf("expected Int, got %s", types.String(firstNonNumeric(l, r)))
		if isCon(l, "Text") || isCon(r, "Text") {
			msg += "; `++` joins Text, `+` is for numbers"
		}
		c.errorf(diag.TypeMismatch, x.Span, "%s", msg)
		return c.u.Fresh()
	}
	if err := c.u.Unify(l, r); err != nil {
		// brand with its base: allowed by widening, the result is the base
		if c.u.Check(l, r) == nil {
			return r
		}
		if c.u.Check(r, l) == nil {
			return l
		}
		c.errorf(diag.TypeMismatch, x.Span, "cannot apply `%s` to `%s` and `%s`", x.Op, types.String(l), types.String(r))
		return c.u.Fresh()
	}
	return lt
}

func firstNonNumeric(l, r types.Type) types.Type {
	if _, ok := numericBase(l); !ok {
		return l
	}
	return r
}

// checkDbGet handles `Db.get T x`; returns the result type when the call is well-formed or diagnosed, nil to fall through.
func (mc *modCtx) checkDbGet(x *syntax.App, env *env, eff effect) types.Type {
	c := mc.c
	tref, ok := x.Args[0].(*syntax.CtorRef)
	if !ok || tref.Ref.Kind != syntax.RefTable {
		return nil
	}
	info := c.tables[c.key(tref.Ref.Module, tref.Ref.Name)]
	if info == nil {
		return nil
	}
	rec := info.Record
	argT := c.u.Apply(mc.infer(x.Args[1], env, eff))
	res := &types.App{Con: "Query", Args: []types.Type{mc.maybeOf(rec)}}
	if err := c.u.Check(argT, info.IDType); err == nil {
		return res
	}
	d := c.errorf("E0101", x.Args[1].GetSpan(), "`Db.get %s` takes a `%s`, got `%s`", rec.Name, types.String(info.IDType), types.String(argT))
	// fix: a column of the argument's type, if exactly one exists
	var cols []string
	for name, ct := range info.Columns {
		if name != info.IDField && c.u.Check(argT, ct) == nil {
			cols = append(cols, name)
		}
	}
	sort.Strings(cols)
	if len(cols) == 1 {
		d.Fix = diag.ReplaceSpan(x.Span,
			fmt.Sprintf("Db.from %s |> Db.where (%s.%s.eq %s) |> Db.first", rec.Name, rec.Name, cols[0], mc.source(x.Args[1].GetSpan())))
	}
	return res
}
