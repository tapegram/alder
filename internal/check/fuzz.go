package check

import (
	"alder/internal/diag"
	"alder/internal/syntax"
	"alder/internal/types"
)

// fuzzProperty is `Fuzz.Property`, the type of a property test's body.
var fuzzProperty = &types.Con{Name: "Property", Module: "Fuzz"}

// fuzzForAllHead returns the `Fuzz.forAll` or `Fuzz.forAllTask` reference at the head of e: e is an
// application of it, or a `|>` pipeline whose last stage is one. nil when e is neither.
func fuzzForAllHead(e syntax.Expr) *syntax.Var {
	switch x := e.(type) {
	case *syntax.Var:
		if isFuzzProperty(x) {
			return x
		}
	case *syntax.App:
		return fuzzForAllHead(x.Fn)
	case *syntax.BinOp:
		if x.Op == "|>" {
			return fuzzForAllHead(x.R)
		}
	}
	return nil
}

func isFuzzProperty(v *syntax.Var) bool {
	return v.Ref.Kind == syntax.RefStd && v.Ref.Module == "Fuzz" && (v.Ref.Name == "forAll" || v.Ref.Name == "forAllTask")
}

// isFuzzForAll reports whether e is an App (or the last `|>` stage) whose head resolves to
// `Fuzz.forAll` or `Fuzz.forAllTask`.
func isFuzzForAll(e syntax.Expr) bool { return fuzzForAllHead(e) != nil }

// isFuzzAny reports whether v is `Fuzz.any`, the special form whose generator is derived from
// the type it is used at.
func isFuzzAny(v *syntax.Var) bool {
	return v.Ref.Kind == syntax.RefStd && v.Ref.Module == "Fuzz" && v.Ref.Name == "any"
}

// fuzzUse is one `Fuzz.any`: the reference and the `a` of its `Fuzzer a`.
type fuzzUse struct {
	v    *syntax.Var
	elem types.Type
}

// inferFuzzAny types `Fuzz.any` as a `Fuzzer a` for a fresh a, remembered until the end of the
// declaration (resolveFuzzAny), when a must be concrete.
func (c *Checker) inferFuzzAny(v *syntax.Var) types.Type {
	elem := c.u.Fresh()
	c.fuzzAny = append(c.fuzzAny, fuzzUse{v, elem})
	return &types.App{Con: "Fuzzer", Args: []types.Type{elem}}
}

// resolveFuzzAny ends the declaration the pending `Fuzz.any` uses were checked in: inside a test
// (inTest) a type variable left free defaults to Int, as a property's argument does; anywhere
// else a type that is not concrete is E0038. A concrete type that has no generator is E0038 too.
// Each use's type is recorded in Result.FuzzAny. show prints a type for the message.
func (c *Checker) resolveFuzzAny(show func(types.Type) string, inTest bool) {
	for _, use := range c.fuzzAny {
		t := c.u.Apply(use.elem)
		if inTest {
			t = c.defaultInt(t)
		}
		c.res.FuzzAny[use.v] = t
		switch {
		case poisoned(t):
		case !concrete(t):
			c.errorf(diag.NotDerivable, use.v.Span, "`Fuzz.any` needs a concrete type here")
		default:
			if path, ok := derivableIn(t, show); !ok {
				c.notDerivable(use.v.Span, path)
			}
		}
	}
	c.fuzzAny = nil
}

// notDerivable reports E0038 for the type path names, which has no generator.
func (c *Checker) notDerivable(span diag.Span, path string) {
	c.errorf(diag.NotDerivable, span, "%s cannot be generated; use `Fuzz.forAll` with a generator you write", path)
}

// defaultInt sets every type variable free in the applied type t to Int (spec §4.4) and returns
// t with them set.
func (c *Checker) defaultInt(t types.Type) types.Type {
	for _, v := range freeVars(t) {
		c.u.Subst[v.ID] = types.Int
	}
	return c.u.Apply(t)
}

// checkPropLambda checks a function-bodied test: the lambda takes one argument per parameter
// (its annotation, or a fresh variable) and returns a Bool, or a Task Bool when its body is a
// block. It returns the property's kind and parameters, and the parameters' types.
func (mc *modCtx) checkPropLambda(lam *syntax.Lambda) (PropInfo, []types.Type) {
	c := mc.c
	fn := &types.Fun{Result: types.Bool}
	for range lam.Params {
		fn.Params = append(fn.Params, c.u.Fresh()) // the lambda's annotation, if any, is unified with it
	}
	_, isTask := lam.Body.(*syntax.Block)
	if isTask {
		fn.Result = &types.App{Con: "Task", Args: []types.Type{types.Bool}}
	}
	mc.propBody = lam.Body
	mc.check(lam, fn, newEnv(), effect{}) // a block body takes its kind from its expected type
	return PropInfo{Kind: PropFun, Effect: isTask, Params: lam.Params}, fn.Params
}

// forAllInfo is the PropInfo of a `Fuzz.forAll` body before its types are known.
func (mc *modCtx) forAllInfo(body syntax.Expr) PropInfo {
	info := PropInfo{Kind: PropForAll, Effect: mc.propHead.Ref.Name == "forAllTask"}
	if lam, ok := forAllFn(body).(*syntax.Lambda); ok {
		info.Params = lam.Params
	}
	return info
}

// forAllValue is the `a` of the test's `Fuzz.forAll : Fuzzer a -> …` as instantiated there.
func (mc *modCtx) forAllValue() types.Type {
	if fn, ok := mc.c.u.Apply(mc.propType).(*types.Fun); ok && len(fn.Params) > 0 {
		if gen, ok := fn.Params[0].(*types.App); ok && gen.Con == "Fuzzer" && len(gen.Args) == 1 {
			return gen.Args[0]
		}
	}
	return types.Err
}

// forAllFn is the function argument of the `Fuzz.forAll` body e (`Fuzz.forAll gen f`, or
// `f |> Fuzz.forAll gen`), nil when it has none.
func forAllFn(e syntax.Expr) syntax.Expr {
	switch x := e.(type) {
	case *syntax.App:
		if args := appArgs(x); len(args) >= 2 {
			return args[1]
		}
	case *syntax.BinOp:
		if a, ok := x.R.(*syntax.App); ok && x.Op == "|>" && len(appArgs(a)) == 1 {
			return x.L
		}
	}
	return nil
}

// appArgs are the arguments of an application, its head's first when the head is one too.
func appArgs(a *syntax.App) []syntax.Expr {
	if inner, ok := a.Fn.(*syntax.App); ok {
		return append(appArgs(inner), a.Args...)
	}
	return a.Args
}

// endProperty finishes property test d once its body is checked: the type variables left free
// in its argument types default to Int (spec §4.4), and a function-bodied test's arguments must
// each have a generator (E0038 at the parameter). It records d in Result.Props.
func (mc *modCtx) endProperty(d *syntax.TestDecl, info PropInfo, params []types.Type) {
	c := mc.c
	var free [][]*types.Var
	for _, t := range params {
		free = append(free, freeVars(c.u.Apply(t))) // all before any is set: two may share one
	}
	for i, t := range params {
		info.Types = append(info.Types, c.defaultInt(c.u.Apply(t)))
		info.Defaulted = append(info.Defaulted, len(free[i]) > 0)
	}
	if info.Kind == PropFun {
		for i, t := range info.Types {
			if poisoned(t) {
				continue
			}
			if path, ok := derivableIn(t, mc.ts); !ok {
				c.notDerivable(info.Params[i].Span, path)
			}
		}
	}
	c.res.Props[d] = info
}

// concrete reports whether the applied type t has no type variable or parameter in it.
func concrete(t types.Type) bool {
	switch t := t.(type) {
	case *types.Var, *types.Param:
		return false
	case *types.App:
		return allConcrete(t.Args)
	case *types.Union:
		return allConcrete(t.Args)
	case *types.Fun:
		return allConcrete(t.Params) && concrete(t.Result)
	case *types.Record:
		if t.Anonymous {
			for _, f := range t.Fields {
				if !concrete(f.Type) {
					return false
				}
			}
		}
		return allConcrete(t.Args)
	}
	return true
}

func allConcrete(ts []types.Type) bool {
	for _, t := range ts {
		if !concrete(t) {
			return false
		}
	}
	return true
}

// derivableCons are the builtin types with a generator of their own (spec §4).
var derivableCons = map[string]bool{"Int": true, "Float": true, "Bool": true, "Text": true, "Unit": true, "Bytes": true, "Date": true, "Time": true}

// derivable reports whether `alder test` can derive a generator for the concrete type t (spec
// §4). When it cannot, path names the offending part, as in "`Html Msg` in field `view` of `Card`".
func derivable(t types.Type) (path string, ok bool) { return derivableIn(t, types.String) }

// derivableIn is derivable with the types in path printed by show.
func derivableIn(t types.Type, show func(types.Type) string) (string, bool) {
	bad, where := derive(t, show, map[string]bool{})
	if bad == nil {
		return "", true
	}
	return "`" + show(bad) + "`" + where, false
}

// derive returns the part of t that has no generator (nil when t has one) and, when that part
// is inside a record or a constructor, where: " in field `f` of `R`".
func derive(t types.Type, show func(types.Type) string, seen map[string]bool) (types.Type, string) {
	switch x := t.(type) {
	case *types.Con:
		if types.IsErr(x) || x.Module == "" && derivableCons[x.Name] {
			return nil, ""
		}
	case *types.Brand: // draw the base, keep the values the brand's check accepts
		return derive(x.Base, show, seen)
	case *types.App:
		switch x.Con {
		case "Id": // a row id: any Int is one
			return nil, ""
		case "List", "Dict":
			for _, a := range x.Args {
				if bad, where := derive(a, show, seen); bad != nil {
					return bad, where
				}
			}
			return nil, ""
		}
	case *types.Record:
		key := "record " + show(x)
		if !x.Anonymous {
			if seen[key] { // a record that holds itself (through a list): its fields are being derived
				return nil, ""
			}
			seen[key] = true
		}
		for _, f := range x.Fields {
			if bad, where := derive(f.Type, show, seen); bad != nil {
				if where == "" {
					where = " in field `" + f.Name + "`"
					if !x.Anonymous {
						where += " of `" + show(x) + "`"
					}
				}
				return bad, where
			}
		}
		return nil, ""
	case *types.Union:
		if x.Module == "Prelude" && (x.Name == "Nav" || x.Name == "Answer") {
			break // results a page or a browser step produces, not data
		}
		key := "union " + show(x)
		if seen[key] { // a recursive union: generated within a depth budget
			return nil, ""
		}
		seen[key] = true
		for _, ct := range x.Constructors() {
			arg, _ := types.CtorArg(x, ct.Name)
			if arg == nil {
				continue
			}
			if bad, where := derive(arg, show, seen); bad != nil {
				if where == "" {
					where = " in constructor `" + ct.Name + "` of `" + show(x) + "`"
				}
				return bad, where
			}
		}
		return nil, ""
	}
	return t, ""
}
