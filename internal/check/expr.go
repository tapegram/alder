// Expression checking: literals, operators, application, records, blocks and effect rules.
package check

import (
	"fmt"
	"sort"
	"strconv"
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
			if d.Sig == nil && d.Body != nil && mc.infersTopLevel() {
				mc.inferTopLevel(d)
				continue
			}
			if d.Sig == nil && d.Body != nil {
				mc.inferSig(d)
				continue
			}
			sig, ok := c.values[c.key(mc.m.Name, d.Name)]
			if !ok || d.Body == nil {
				continue
			}
			mc.inUpdate = (mc.page != nil && d.Name == "update") || (mc.isApp && d.Name == "update")
			mc.checkValue(d, sig)
			mc.inUpdate = false
		case *syntax.FakeDecl:
			mc.checkFakeEntries(d)
		case *syntax.TestDecl:
			mc.holes = nil
			mc.inTest, mc.testBody, mc.testName = true, d.Body, d.Name
			if _, isBlock := d.Body.(*syntax.Block); isBlock { // an effect test: a Task Bool
				mc.check(d.Body, &types.App{Con: "Task", Args: []types.Type{types.Bool}}, newEnv(), effect{kind: "Task"})
			} else {
				mc.check(d.Body, types.Bool, newEnv(), effect{})
			}
			mc.inTest, mc.testBody, mc.testName = false, nil, ""
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

// checkValue checks the body of value d against its signature sig.
func (mc *modCtx) checkValue(d *syntax.ValueDecl, sig types.Type) {
	c := mc.c
	mc.holes = nil
	env := newEnv()
	body := sig
	if fn, isFn := sig.(*types.Fun); isFn {
		if len(d.Params) > len(fn.Params) {
			c.errorf(diag.Arity, d.Span, "`%s` takes %d parameters but %d are named", d.Name, len(fn.Params), len(d.Params))
			return
		}
		for i, p := range d.Params {
			c.bind(env, p, p.Name, fn.Params[i])
		}
		if len(d.Params) < len(fn.Params) {
			body = &types.Fun{Params: fn.Params[len(d.Params):], Result: fn.Result}
		} else {
			body = fn.Result
		}
	} else if len(d.Params) > 0 {
		c.errorf(diag.Arity, d.Span, "`%s` has type `%s`, which takes no parameters", d.Name, types.String(sig))
		return
	}
	mc.check(d.Body, body, env, effect{})
	mc.reportHoles()
}

// inferSig checks the body of a value that has no signature and reports E0007 with the inferred
// signature as the fix, or `?type` when the type is not fully known (an unsolved variable or an error).
func (mc *modCtx) inferSig(d *syntax.ValueDecl) {
	c := mc.c
	mc.holes = nil
	env := newEnv()
	var params []types.Type
	for _, p := range d.Params {
		v := c.u.Fresh()
		c.bind(env, p, p.Name, v)
		params = append(params, v)
	}
	var sig types.Type = mc.infer(d.Body, env, effect{})
	if len(params) > 0 {
		sig = &types.Fun{Params: params, Result: sig}
	}
	mc.reportHoles()
	s := "?type"
	if t := c.u.Apply(sig); !unknownIn(t) {
		s = types.StringIn(t, mc.m.Name)
	}
	mc.missingSig(d, s)
}

// infersTopLevel reports whether this module's sig-less top-level values have their signatures
// inferred (it is Options.InferModule) rather than reported as E0007.
func (mc *modCtx) infersTopLevel() bool {
	return mc.c.opts.InferModule != "" && mc.m.Name == mc.c.opts.InferModule
}

// inferTopLevel infers the signature of a value written without one, generalizes the type
// variables left unsolved to type parameters `a`, `b`, … (binding each variable to its
// parameter, so the body's recorded types name them too), and records it as the value's
// signature. A body whose type is not known is remembered as poisoned; unless that comes from
// an error already reported (in it, or in a poisoned value it uses) it is E0007 without a fix
// (a value used before its own definition).
//
// A function may use itself (let-rec): while its body is inferred its name is bound to a fresh,
// monomorphic type variable, unified with the body's type before generalizing. Values without a
// signature defined after it are not known yet (there is no mutual recursion between them): such
// a forward reference poisons the value. A body that reports an error, uses a poisoned value or
// makes a forward reference is poisoned even when its final type is known, since the poison type
// inside it must not reach Lower.
func (mc *modCtx) inferTopLevel(d *syntax.ValueDecl) {
	c := mc.c
	key := c.key(mc.m.Name, d.Name)
	errs := errorCount(c.bag)
	c.usedPoisoned = false
	c.forwardRef = ""
	mc.holes = nil
	env := newEnv()
	var params []types.Type
	for _, p := range d.Params {
		v := c.u.Fresh()
		c.bind(env, p, p.Name, v)
		params = append(params, v)
	}
	_, isLambda := d.Body.(*syntax.Lambda)
	recursive := len(d.Params) > 0 || isLambda // only a function may use itself
	self := c.u.Fresh()
	if recursive {
		c.values[key] = self
	}
	var sig types.Type = mc.infer(d.Body, env, effect{})
	if len(params) > 0 {
		sig = &types.Fun{Params: params, Result: sig}
	}
	if c.u.Unify(self, sig) != nil {
		// the value uses itself at a type its body does not have (`f = \x -> f`)
		c.errorf(diag.MissingSig, d.Span, "the type of `%s` cannot be inferred: it uses itself at a different type; give it a type signature", d.Name)
	}
	t := c.u.Apply(sig)
	for i, v := range freeVars(t) {
		c.u.Subst[v.ID] = &types.Param{Name: paramName(i)}
	}
	mc.reportHoles()
	t = c.u.Apply(t)
	if poisoned(t) || errorCount(c.bag) > errs || c.usedPoisoned || c.forwardRef != "" {
		delete(c.values, key)
		if c.poisonedTop == nil {
			c.poisonedTop = map[string]bool{}
		}
		c.poisonedTop[key] = true
		if errorCount(c.bag) == errs && !c.usedPoisoned { // else the error is reported already
			if c.forwardRef == d.Name {
				c.errorf(diag.MissingSig, d.Span, "the type of `%s` cannot be inferred: it uses itself, and only a function may", d.Name)
			} else if c.forwardRef != "" {
				c.errorf(diag.MissingSig, d.Span, "the type of `%s` cannot be inferred: it uses `%s`, a value defined after it; move `%s` above it or give `%s` a type signature", d.Name, c.forwardRef, c.forwardRef, d.Name)
			} else {
				c.errorf(diag.MissingSig, d.Span, "the type of `%s` cannot be inferred; give it a type signature", d.Name)
			}
		}
		return
	}
	c.values[key] = t
}

// errorCount is the number of errors in bag.
func errorCount(bag *diag.Bag) int {
	n := 0
	for _, d := range bag.Items {
		if d.Severity == "error" {
			n++
		}
	}
	return n
}

// freeVars lists the unsolved variables of the applied type t, in order of first occurrence.
func freeVars(t types.Type) []*types.Var {
	var out []*types.Var
	seen := map[int]bool{}
	var walk func(types.Type)
	walk = func(t types.Type) {
		switch t := t.(type) {
		case *types.Var:
			if !seen[t.ID] {
				seen[t.ID] = true
				out = append(out, t)
			}
		case *types.App:
			for _, a := range t.Args {
				walk(a)
			}
		case *types.Union:
			for _, a := range t.Args {
				walk(a)
			}
		case *types.Fun:
			for _, p := range t.Params {
				walk(p)
			}
			walk(t.Result)
		case *types.Record:
			if t.Anonymous {
				for _, f := range t.Fields {
					walk(f.Type)
				}
			}
			for _, a := range t.Args {
				walk(a)
			}
		}
	}
	walk(t)
	return out
}

// paramName is the i-th generalized type parameter's name: a … z, then a1, b1, ….
func paramName(i int) string {
	s := string(rune('a' + i%26))
	if i >= 26 {
		s += fmt.Sprint(i / 26)
	}
	return s
}

// missingSig reports E0007 for d with a fix that inserts `name : sig` on the line before it.
func (mc *modCtx) missingSig(d *syntax.ValueDecl, sig string) {
	mc.c.errorf(diag.MissingSig, d.Span, "`%s` has no type signature; write `%s : %s` on the line before it", d.Name, d.Name, sig).Fix =
		diag.InsertAt(d.Span.Start.Line, 1, d.Name+" : "+sig+"\n")
}

// reportHoles ends a body: it reports its holes and the Dict keys it infers to hold a function.
func (mc *modCtx) reportHoles() {
	for _, h := range mc.holes {
		mc.c.res.Holes++
		mc.c.bag.Warn("W0002", h.span, "hole ?%s has type %s", h.name, types.String(mc.c.u.Apply(h.t)))
	}
	mc.holes = nil
	mc.c.checkDbTables()
	mc.c.checkScopedTables()
	mc.c.checkInferredDictKeys()
}

// checkDbTables reports E0003, once per body at its first such use, for a Db function taking a
// `Table t` or `Select t` whose t is not a table at that use (`countAny : Table t -> Query Int`
// calling `Db.from t`): the generated code names the table, so it must be known where the
// function is used — the checker accepts the polymorphic type, Lower cannot build it. It reads
// the expressions recorded since the last body ended (checkInferredDictKeys clears them).
func (c *Checker) checkDbTables() {
	var at *syntax.Var
	var what string
	for _, e := range c.bodyExprs {
		v, ok := e.(*syntax.Var)
		if !ok || v.Ref.Kind != syntax.RefStd || v.Ref.Module != "Db" {
			continue
		}
		fn, ok := c.u.Apply(c.res.Types[e]).(*types.Fun)
		if !ok {
			continue
		}
		for _, p := range fn.Params {
			app, ok := c.u.Apply(p).(*types.App)
			if !ok || (app.Con != "Table" && app.Con != "Select") || len(app.Args) != 1 {
				continue
			}
			var w string
			switch t := c.u.Apply(app.Args[0]).(type) {
			case *types.Param:
				w = fmt.Sprintf("`%s` is a type parameter", t.Name)
			case *types.Var:
				w = "its table is not known here"
			default:
				continue
			}
			if at == nil || before(v.Span, at.Span) {
				at, what = v, w
			}
		}
	}
	if at != nil {
		c.errorf(diag.TypeMismatch, at.Span, "`Db.%s` needs a concrete table; %s: write the table's own type (`Table Todo`, `Select Todo`) in the signature, one function per table", at.Ref.Name, what)
	}
}

// checkInferredDictKeys reports E0003 for a Dict whose key type holds a function when no written
// type says so (`Dict.insert inc "a" Dict.empty`): typeOf rejects such a Dict where its type is
// written, but an inferred one would only fail at run time (rt.Key cannot encode a function). It
// looks at the expressions recorded since the last body ended and reports once per body, at the
// first expression (the outermost when several start there) whose type holds such a Dict. A
// function type is not looked into: a Dict built by calling it has a type of its own. A body
// with a poisoned type is skipped: its error is reported already, and may be this one, written
// (`d : Dict (Int -> Int) Text` poisons the body checked against it).
func (c *Checker) checkInferredDictKeys() {
	exprs := c.bodyExprs
	c.bodyExprs = nil
	var at syntax.Expr
	var key types.Type
	for _, e := range exprs {
		t := c.u.Apply(c.res.Types[e])
		if poisoned(t) {
			return
		}
		k := funKeyDict(t, map[*types.Record]bool{})
		if k == nil {
			continue
		}
		if at == nil || before(e.GetSpan(), at.GetSpan()) {
			at, key = e, k
		}
	}
	if at != nil {
		c.errorf(diag.TypeMismatch, at.GetSpan(), "%s", dictKeyMessage(key, types.String(key)))
	}
}

// dictKeyMessage is the E0003 message for Dict key type key (written as shown), which holds a
// function or an effect value.
func dictKeyMessage(key types.Type, shown string) string {
	if containsEffect(key) {
		return fmt.Sprintf("a `Dict` key cannot be an effect value, got `%s`: keys are compared with `==`, and effect values cannot be compared", shown)
	}
	return fmt.Sprintf("a `Dict` key cannot be a function, got `%s`: keys are compared with `==`, and functions cannot be compared", shown)
}

// before orders spans by start, the longer first among equal starts.
func before(a, b diag.Span) bool {
	if a.Start != b.Start {
		return a.Start.Line < b.Start.Line || a.Start.Line == b.Start.Line && a.Start.Col < b.Start.Col
	}
	return a.End.Line > b.End.Line || a.End.Line == b.End.Line && a.End.Col > b.End.Col
}

// funKeyDict is the key type of a `Dict k v` in t, outside function types, whose k contains a
// function; nil when there is none.
func funKeyDict(t types.Type, seen map[*types.Record]bool) types.Type {
	switch t := t.(type) {
	case *types.App:
		if t.Con == "Dict" && len(t.Args) == 2 && containsFun(t.Args[0]) {
			return t.Args[0]
		}
		for _, a := range t.Args {
			if k := funKeyDict(a, seen); k != nil {
				return k
			}
		}
	case *types.Union:
		for _, a := range t.Args {
			if k := funKeyDict(a, seen); k != nil {
				return k
			}
		}
	case *types.Record:
		if seen[t] {
			return nil
		}
		seen[t] = true
		for _, f := range t.Fields {
			if k := funKeyDict(f.Type, seen); k != nil {
				return k
			}
		}
	case *types.Brand:
		return funKeyDict(t.Base, seen)
	}
	return nil
}

// maybeOf builds `Maybe inner` from the declared Prelude union.
func (mc *modCtx) maybeOf(inner types.Type) types.Type {
	return mc.c.types["Prelude.Maybe"].(*types.Union).WithArgs([]types.Type{inner})
}

// mismatch reports E0003. When either side involves the poison type the real error is already
// reported, so it reports nothing and returns a detached diagnostic the caller may still annotate.
func (mc *modCtx) mismatch(span diag.Span, got, expected types.Type) *diag.Diagnostic {
	c := mc.c
	g, e := c.u.Apply(got), c.u.Apply(expected)
	if poisoned(g) || poisoned(e) {
		return &diag.Diagnostic{}
	}
	msg := fmt.Sprintf("expected %s, got %s", types.String(e), types.String(g))
	if inner, ok := maybeOf(g); ok && c.u.Check(inner, e) == nil {
		msg += "; match it: match m { Just x -> …, Nothing -> … }"
	}
	if sameFields(g, e) {
		// a record literal bound without a signature (an eval or REPL let) is structural
		msg += fmt.Sprintf("; the record has %s's fields: annotate the let (`let x : %s = …`), or give the value a signature", types.String(e), types.String(e))
	}
	return c.errorf(diag.TypeMismatch, span, "%s", msg)
}

// sameFields reports whether got is a structural record with exactly the field names of the
// nominal record expected.
func sameFields(got, expected types.Type) bool {
	g, ok1 := got.(*types.Record)
	e, ok2 := expected.(*types.Record)
	if !ok1 || !ok2 || !g.Anonymous || e.Anonymous || len(g.Fields) != len(e.Fields) {
		return false
	}
	names := map[string]bool{}
	for _, f := range e.Fields {
		names[f.Name] = true
	}
	for _, f := range g.Fields {
		if !names[f.Name] {
			return false
		}
	}
	return true
}

// check verifies e against expected, pushing the expectation into literals, lambdas, branches and blocks.
// It records expected as e's type even on a mismatch, so on a program without errors every node has a type.
func (mc *modCtx) check(e syntax.Expr, expected types.Type, env *env, eff effect) {
	mc.checkExpr(e, expected, env, eff)
	if from, ok := mc.c.widenedFrom[e]; ok {
		mc.c.record(e, from) // the value is the narrowed session; Lower widens it
		return
	}
	mc.c.record(e, expected)
}

// narrowedSession reports whether t is the narrowed Session of a `requires` page.
func (c *Checker) narrowedSession(t types.Type) bool {
	r, ok := t.(*types.Record)
	if !ok || r.Anonymous || r.Name != "Session" {
		return false
	}
	p := c.pages[r.Module]
	return p != nil && p.Session != nil
}

// appSession reports whether t is App's Session record.
func appSession(t types.Type) bool {
	r, ok := t.(*types.Record)
	return ok && !r.Anonymous && r.Module == "App" && r.Name == "Session"
}

// widen accepts a `requires` page's narrowed Session where App's Session is expected (as a brand
// widens to its base), recording the place so the value is converted. It never narrows back.
func (mc *modCtx) widen(e syntax.Expr, got, expected types.Type) bool {
	c := mc.c
	g, ex := c.u.Apply(got), c.u.Apply(expected)
	if !c.narrowedSession(g) || !appSession(ex) {
		return false
	}
	c.res.Widened[e] = ex.(*types.Record)
	c.widenedFrom[e] = g
	return true
}

func (mc *modCtx) checkExpr(e syntax.Expr, expected types.Type, env *env, eff effect) {
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
				c.bind(child, p, p.Name, fn.Params[i])
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
	if mc.widen(e, got, expected) {
		return
	}
	if err := c.u.Check(got, expected); err != nil {
		if g, ex := c.u.Apply(got), c.u.Apply(expected); appSession(g) && c.narrowedSession(ex) {
			r := ex.(*types.Record)
			c.errorf(diag.TypeMismatch, e.GetSpan(), "expected this page's `Session`, got `App.Session`: the page `requires %s`, so its session has `%s` for certain, and App's session may not; a page's session widens to App's, never the other way", c.pages[r.Module].Requires, c.pages[r.Module].Requires)
			return
		}
		d := mc.mismatch(e.GetSpan(), got, expected)
		mc.annotateMismatch(d, e, got, expected)
	}
}

// annotateMismatch adds the fixes the spec lists for common mismatches. d is only valid until the next diagnostic.
func (mc *modCtx) annotateMismatch(d *diag.Diagnostic, e syntax.Expr, got, expected types.Type) {
	c := mc.c
	g, ex := c.u.Apply(got), c.u.Apply(expected)
	if k, _ := queryOrTask(g); mc.inTest && e == mc.testBody && k != "" && isCon(ex, "Bool") && d.Message != "" {
		d.Message += "; a test of a Task is written as a block: `test \"…\" = { … }`"
	} else if mc.inTest && e == mc.testBody && isCon(ex, "Bool") && d.Message != "" {
		// the body itself is neither a Bool nor a Task Bool: say which test
		d.Message = "test " + strconv.Quote(mc.testName) + ": " + d.Message
	}
	if k, _ := queryOrTask(g); k == "Query" && mc.runArgs[e] && d.Message != "" {
		if k, _ := queryOrTask(ex); k == "Task" {
			d.Message += "; a Query runs inside a test's block: `let x <- q`; `Test.run` takes a Task"
		}
	}
	if b, ok := mc.testBody.(*syntax.Block); ok && mc.inTest && e == b.Result && isCon(g, "Bool") {
		if k, arg := queryOrTask(ex); k == "Task" && isCon(arg, "Bool") && d.Message != "" {
			d.Message += "; a block test is a Task: end it with `Task.succeed (…)`"
		}
	}
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

// source returns the exact text of a span, across lines when it spans several (trimmed at both ends).
func (mc *modCtx) source(sp diag.Span) string {
	if sp.Start.Line < 1 || sp.End.Line > len(mc.m.Lines) || sp.End.Line < sp.Start.Line {
		return ""
	}
	if sp.Start.Line == sp.End.Line {
		line := lineOf(mc.m, sp.Start.Line)
		if sp.End.Col-1 > len(line) || sp.Start.Col-1 > len(line) || sp.End.Col < sp.Start.Col {
			return ""
		}
		return strings.TrimSpace(line[sp.Start.Col-1 : sp.End.Col-1])
	}
	first := lineOf(mc.m, sp.Start.Line)
	last := lineOf(mc.m, sp.End.Line)
	if sp.Start.Col-1 > len(first) || sp.End.Col-1 > len(last) {
		return ""
	}
	parts := []string{first[sp.Start.Col-1:]}
	parts = append(parts, mc.m.Lines[sp.Start.Line:sp.End.Line-1]...)
	parts = append(parts, last[:sp.End.Col-1])
	return strings.TrimSpace(strings.Join(parts, "\n"))
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
			if info, mine := c.insertRecordTable(rec); info != nil {
				if _, isCol := info.Columns[fi.Name]; isCol {
					switch {
					case mine && fi.Name == info.Owner:
						c.errorf(diag.TypeMismatch, fi.Span, "`%s` is set by `Db.insertMine`", fi.Name)
					case mine:
						c.errorf("E0501", fi.Span, "`Db.insertMine` never takes `%s`; it is generated", fi.Name)
					default:
						c.errorf("E0501", fi.Span, "`Db.insert` never takes `%s`; it is generated", fi.Name)
					}
					mc.infer(fi.Value, env, eff)
					continue
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
	// Defaults apply at boundaries: a session's to its cookie (and to the literal `Test.login`
	// stores in it), an insert's to the row it makes,
	// a parameter's to a link or URL without it, a form's to a request without it. A literal of
	// any other declared record (a table's row, a `type R = {…}`, App's Session or a page's
	// narrowed one) names every field. An anonymous record type's defaults (a component's
	// props, an options argument) apply to its literals.
	strict := !rec.Anonymous && !c.boundaryRecord(rec) && x != mc.cookieLit
	var missing, defaulted []string
	for _, f := range rec.Fields {
		if seen[f.Name] || f.Name == "form" {
			continue
		}
		switch {
		case !f.HasDefault:
			missing = append(missing, f.Name)
		case strict:
			missing = append(missing, f.Name)
			defaulted = append(defaulted, f.Name)
		}
	}
	const why = "a default applies only to an insert, the session cookie, a page or message parameter, or a form, not to a literal"
	switch {
	case len(missing) > 0 && len(missing) == len(defaulted):
		c.errorf(diag.TypeMismatch, x.Span, "record literal for `%s` is missing `%s` (%s)", rec.Name, strings.Join(missing, "`, `"), why)
	case len(defaulted) > 0:
		c.errorf(diag.TypeMismatch, x.Span, "record `%s` is missing field(s): %s (for `%s`, %s)", types.String(rec), strings.Join(missing, ", "), strings.Join(defaulted, "`, `"), why)
	case len(missing) > 0:
		c.errorf(diag.TypeMismatch, x.Span, "record `%s` is missing field(s): %s", types.String(rec), strings.Join(missing, ", "))
	}
}

// boundaryRecord reports whether rec's defaults apply to its literals: a table's `Insert T`, a
// form, or a page's or message's parameters.
func (c *Checker) boundaryRecord(rec *types.Record) bool {
	if info, _ := c.insertRecordTable(rec); info != nil {
		return true
	}
	return c.forms[c.key(rec.Module, rec.Name)] != nil || c.paramRecs[c.key(rec.Module, rec.Name)]
}

func (mc *modCtx) checkMatch(x *syntax.Match, expected types.Type, env *env, eff effect) {
	subj := mc.c.u.Apply(mc.infer(x.Subject, env, eff))
	ex := newExhaustiveness(mc, subj)
	for _, a := range x.Arms {
		child := env.child()
		if ex.closed && ex.kind != "error" {
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
	// poisoned: the bound expression that should decide the block's kind has the poison type,
	// so the kind is unknown because of an error already reported; its binds are not E0402.
	poisoned := false
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
			c.bind(child, s, s.Name, t)
			continue
		}
		var got types.Type
		if kind == "" && expectedUnknown {
			// no expectation: the first bound expression decides what kind of block this is
			got = c.u.Apply(mc.infer(s.Expr, child, sub()))
			if types.IsErr(got) {
				poisoned = true
			}
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
		if kind == "" && poisoned {
			if got == nil {
				mc.infer(s.Expr, child, sub())
			}
			c.bind(child, s, s.Name, types.Err)
			continue
		}
		if kind == "" {
			c.errorf("E0402", s.Span, "`let %s <-` is only allowed in a block that returns `Query` or `Task`", s.Name)
			if got == nil {
				mc.infer(s.Expr, child, sub())
			}
			c.bind(child, s, s.Name, inner)
			continue
		}
		if got == nil {
			got = c.u.Apply(mc.infer(s.Expr, child, sub()))
		}
		if types.IsErr(got) { // already reported: the name has its annotation, else is poisoned too, so its uses report nothing more
			if t != nil {
				c.bind(child, s, s.Name, t)
			} else {
				c.bind(child, s, s.Name, types.Err)
			}
			continue
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
		c.bind(child, s, s.Name, inner)
	}
	if x.Result != nil {
		mc.check(x.Result, expected, child, sub())
	}
}

// infer returns e's type, reporting errors inside e, and records it.
func (mc *modCtx) infer(e syntax.Expr, env *env, eff effect) types.Type {
	return mc.c.record(e, mc.inferExpr(e, env, eff))
}

func (mc *modCtx) inferExpr(e syntax.Expr, env *env, eff effect) types.Type {
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
		if !ok && types.IsErr(base) {
			for _, fi := range x.Fields {
				mc.infer(fi.Value, env, eff)
			}
			return base
		}
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
			c.bind(child, p, p.Name, v)
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
		if x.Ref.Kind == syntax.RefStd && x.Ref.Module == "Test" && !mc.inTest { // eval's module is not a test here: it cannot run Tasks
			c.errorf(diag.TypeMismatch, x.Span, "`Test.%s` is only available inside a `test`", x.Ref.Name)
			return types.Err
		}
		if x.Ref.Kind == syntax.RefStd && x.Ref.Module == "Test" && x.Ref.Name == "fake" { // applied, inferApp takes it
			c.errorf(diag.TypeMismatch, x.Span, "`Test.fake` wants a capability name, got an expression")
			return types.Err
		}
		if x.Ref.Kind == syntax.RefStd && x.Ref.Module == "Test" && IsTestStep(x.Ref.Name) { // applied, inferApp takes it
			c.errorf(diag.TypeMismatch, x.Span, "`Test.%s` is a step: apply it to its argument%s, as in `%s`", x.Ref.Name, map[bool]string{true: "s", false: ""}[testSteps[x.Ref.Name] > 1], stepExample[x.Ref.Name])
			return types.Err
		}
		if x.Ref.Kind == syntax.RefStd && x.Ref.Module == "Session" && mc.fake != "" { // the entry's prefix names the fake
			c.errorf(diag.TypeMismatch, x.Span, "a fake cannot use `Session`: an adapter has no session")
			return types.Err
		}
		if x.Ref.Module == "Session" && x.Ref.Name == "put" {
			c.usesSessionPut = true
		}
		if x.Ref.Kind == syntax.RefStd && x.Ref.Module == "Db" && scopedOps[x.Ref.Name] {
			return mc.inferScoped(x)
		}
		if t, ok := c.values[c.key(x.Ref.Module, x.Ref.Name)]; ok {
			return c.u.Instantiate(t)
		}
		if c.poisonedTop[c.key(x.Ref.Module, x.Ref.Name)] {
			c.usedPoisoned = true
		} else if x.Ref.Kind == syntax.RefValue && mc.infersTopLevel() && x.Ref.Module == mc.m.Name && c.forwardRef == "" {
			c.forwardRef = x.Ref.Name // a sig-less value of this module not inferred yet
		}
	case syntax.RefColumn:
		table := columnTable(x)
		mc.fakeTableUse(c.key(x.Ref.Module, table), table, x.Span)
		if info, ok := c.tables[c.key(x.Ref.Module, table)]; ok {
			return &types.App{Con: "Column", Args: []types.Type{info.Record, info.Columns[x.Name]}}
		}
	case syntax.RefCapability:
		if entries, ok := c.caps[c.key(x.Ref.Module, columnTable(x))]; ok { // `Clock.now`, or `A.Clock.now`
			if t, ok := entries[x.Name]; ok {
				return c.u.Instantiate(t)
			}
			c.errorf(diag.UnknownName, x.Span, "capability `%s` has no `%s`", x.Module, x.Name)
		}
	}
	return types.Err // unresolved, or a value whose signature is missing: already reported
}

// isMsgUnion reports whether u is App.Msg or a page's Msg.
func (mc *modCtx) isMsgUnion(u *types.Union) bool { return mc.c.isMessageUnion(u) }

// isMessageUnion reports whether u is the generated App.Msg or a page's generated Msg, by identity;
// a user-declared union that happens to be named Msg is not one.
func (c *Checker) isMessageUnion(u *types.Union) bool {
	if u == nil {
		return false
	}
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
		mc.fakeTableUse(c.key(x.Ref.Module, x.Ref.Name), x.Ref.Name, x.Span)
		if info, ok := c.tables[c.key(x.Ref.Module, x.Ref.Name)]; ok {
			return &types.App{Con: "Table", Args: []types.Type{info.Record}}
		}
	case syntax.RefCtor:
		if x.Ref.Module == "Prelude" && answerCtors[x.Ref.Name] && !mc.inTest {
			c.errorf(diag.TypeMismatch, x.Span, "`%s` is a constructor of the test type `Answer`: it exists only inside a `test`", x.Name)
			return types.Err
		}
		info, ok := c.ctors[c.key(x.Ref.Module, x.Ref.Name)]
		if !ok {
			if b, isBrand := c.types[c.key(x.Ref.Module, x.Ref.Name)].(*types.Brand); isBrand {
				if x.Ref.Module != mc.m.Name && !mc.inTest && !mc.infersTopLevel() { // eval's module is a test's
					c.errorf(diag.InvalidDecl, x.Span, "brand constructor `%s` can only be used in module `%s` or inside a `test`", x.Name, x.Ref.Module)
				}
				return &types.Fun{Params: []types.Type{b.Base}, Result: b}
			}
			return types.Err
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
	return types.Err // unresolved: already reported
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
	if types.IsErr(base) {
		return types.Err
	}
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
			// Lower resolves a column method's column from its receiver's syntax, so the
			// receiver must be the column itself, not a value holding it.
			if v, ok := x.Expr.(*syntax.Var); !ok || v.Ref.Kind != syntax.RefColumn {
				tbl := "Table"
				if r, ok := c.u.Apply(b.Args[0]).(*types.Record); ok && r.Name != "" {
					tbl = r.Name
				}
				c.errorf(diag.TypeMismatch, x.Span, "a column method (`.eq .lt .asc …`) needs its column written in place as `Table.field`; write `%s.field.%s …` instead of `%s.%s`: a column cannot be stored in a variable or passed as an argument",
					tbl, x.Field, mc.source(x.Expr.GetSpan()), x.Field)
				return c.u.Fresh()
			}
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
	case "contains", "startsWith":
		if !isTextLike {
			c.errorf(diag.TypeMismatch, x.Span, "`.%s` needs a Text column, got `%s`", x.Field, types.String(ft))
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
	c.errorf(diag.UnknownName, x.Span, "columns have no `.%s`; use .eq .ne .lt .le .gt .ge .contains .startsWith .in .isNothing .isJust .asc .desc", x.Field)
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
	expr syntax.Expr
}

// inferApp types an application; want, when non-nil, is the type the context expects and is pushed into the result.
func (mc *modCtx) inferApp(x *syntax.App, env *env, eff effect, pipe *pipeArg, want types.Type) types.Type {
	c := mc.c
	if v, ok := x.Fn.(*syntax.Var); ok && v.Ref.Kind == syntax.RefStd && v.Ref.Module == "Test" && v.Ref.Name == "fake" {
		return mc.inferTestFake(x, v, pipe)
	}
	if v, ok := x.Fn.(*syntax.Var); ok && v.Ref.Kind == syntax.RefStd && v.Ref.Module == "Test" && IsTestStep(v.Ref.Name) {
		return mc.inferTestStep(x, v, env, eff, pipe)
	}
	if v, ok := x.Fn.(*syntax.Var); ok && v.Ref.Kind == syntax.RefStd && v.Ref.Module == "Test" && mc.inTest {
		mc.checkSelectorArg(v.Ref.Name, x.Args)
	}
	// constructor applied to a record literal: check against the payload (form field implicit)
	if ct, ok := x.Fn.(*syntax.CtorRef); ok && len(x.Args) == 1 && pipe == nil {
		if lit, isLit := x.Args[0].(*syntax.RecordLit); isLit {
			fnT := c.u.Apply(c.record(ct, mc.inferCtor(ct, true)))
			if fn, isFn := fnT.(*types.Fun); isFn && len(fn.Params) == 1 {
				if rec, isRec := c.u.Apply(fn.Params[0]).(*types.Record); isRec {
					mc.checkRecordLit(lit, rec, env, eff)
					c.record(lit, rec)
					return fn.Result
				}
			}
		}
	}
	// E0301: a message constructor applied to anything but a record literal
	if ct, ok := x.Fn.(*syntax.CtorRef); ok && pipe == nil && ct.Ref.Kind == syntax.RefCtor && len(x.Args) > 0 {
		if info, found := c.ctors[c.key(ct.Ref.Module, ct.Ref.Name)]; found && c.isMessageUnion(info.Union) {
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
	// a lambda applied inline (`(\a b -> a + b) 1 2`): its parameters take the arguments' types,
	// so the body is checked knowing them (inferred alone, `a + b` would not know it adds Ints)
	if lam, ok := x.Fn.(*syntax.Lambda); ok && pipe == nil && len(x.Args) == len(lam.Params) {
		fn := &types.Fun{Result: c.u.Fresh()}
		for _, a := range x.Args {
			fn.Params = append(fn.Params, mc.infer(a, env, eff))
		}
		mc.check(lam, fn, env, eff)
		return fn.Result
	}
	var fnT types.Type
	if ct, ok := x.Fn.(*syntax.CtorRef); ok {
		fnT = c.record(ct, mc.inferCtor(ct, true))
	} else {
		fnT = mc.infer(x.Fn, env, eff)
	}
	return mc.applyFn(x.Fn, c.u.Apply(fnT), x.Args, pipe, want, x.Span, env, eff)
}

// pipedApp is the type of `f a…` on the right of `|>`: the callee's parameters after its explicit
// arguments (the piped one first) to the callee's own result, as one multi-parameter Fun.
func (mc *modCtx) pipedApp(app *syntax.App) types.Type {
	if fn, ok := mc.c.u.Apply(mc.c.res.Types[app.Fn]).(*types.Fun); ok && len(app.Args) < len(fn.Params) {
		return &types.Fun{Params: fn.Params[len(app.Args):], Result: fn.Result}
	}
	return types.Err // the callee is not a function taking the piped value, already reported
}

// applyFn checks args (and the piped value, which comes last) against fnT's parameters.
func (mc *modCtx) applyFn(fnExpr syntax.Expr, fnT types.Type, args []syntax.Expr, pipe *pipeArg, want types.Type, span diag.Span, env *env, eff effect) types.Type {
	c := mc.c
	fn, ok := fnT.(*types.Fun)
	if !ok && types.IsErr(fnT) {
		for _, a := range args {
			mc.infer(a, env, eff)
		}
		return types.Err
	}
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
	if v, isVar := fnExpr.(*syntax.Var); isVar && v.Module == "Test" && v.Name == "run" && len(args) > 0 {
		if mc.runArgs == nil {
			mc.runArgs = map[syntax.Expr]bool{}
		}
		mc.runArgs[args[0]] = true
	}
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
	if pipe != nil && len(args) < len(fn.Params) && !mc.widen(pipe.expr, pipe.t, fn.Params[len(args)]) {
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
		p := &pipeArg{t: l, span: x.L.GetSpan(), expr: x.L}
		if app, ok := x.R.(*syntax.App); ok {
			res := mc.inferApp(app, env, eff, p, nil)
			c.record(app, mc.pipedApp(app))
			return res
		}
		fnT := c.u.Apply(mc.infer(x.R, env, eff))
		if types.IsErr(fnT) {
			return types.Err
		}
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
		} else if lt := c.u.Apply(l); containsFun(lt) {
			if containsEffect(lt) {
				c.errorf(diag.TypeMismatch, x.Span, "effect values cannot be compared: a `Query` or `Task` is a description of work, not its result; run it with `let x <-` and compare the results")
			} else {
				c.errorf(diag.TypeMismatch, x.Span, "functions cannot be compared")
			}
		}
		return types.Bool
	case "<", "<=", ">", ">=":
		// symmetric: either side may widen to the other (`c < 500` and `500 > c` for a brand of Int)
		l := c.u.Apply(mc.infer(x.L, env, eff))
		r := c.u.Apply(mc.infer(x.R, env, eff))
		if c.u.Check(l, r) == nil {
			l = r
		} else if c.u.Check(r, l) == nil {
			// r widens to l
		} else if lb, rb := brandBase(l), brandBase(r); lb != nil && rb != nil && lb.Name == rb.Name && isNumeric(l) && isNumeric(r) {
			// two different brands of one numeric base (`Int.Positive < Int.NonNegative`):
			// both widen to the base and compare as it, as `arith` does (chapter 4, rule 1)
			l = lb
		} else {
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

// unknownIn reports whether the applied type t is not fully known: it contains an unsolved
// variable or the poison type, so it cannot be written as a signature.
func unknownIn(t types.Type) bool {
	switch t := t.(type) {
	case *types.Var:
		return true
	case *types.Con:
		return types.IsErr(t)
	case *types.App:
		for _, a := range t.Args {
			if unknownIn(a) {
				return true
			}
		}
	case *types.Union:
		for _, a := range t.Args {
			if unknownIn(a) {
				return true
			}
		}
	case *types.Fun:
		for _, p := range t.Params {
			if unknownIn(p) {
				return true
			}
		}
		return unknownIn(t.Result)
	case *types.Record:
		if t.Anonymous {
			for _, f := range t.Fields {
				if unknownIn(f.Type) {
					return true
				}
			}
		}
		for _, a := range t.Args {
			if unknownIn(a) {
				return true
			}
		}
	}
	return false
}

// poisoned reports whether t is or contains the poison type.
func poisoned(t types.Type) bool {
	switch t := t.(type) {
	case *types.Con:
		return types.IsErr(t)
	case *types.App:
		for _, a := range t.Args {
			if poisoned(a) {
				return true
			}
		}
	case *types.Union:
		for _, a := range t.Args {
			if poisoned(a) {
				return true
			}
		}
	case *types.Fun:
		for _, p := range t.Params {
			if poisoned(p) {
				return true
			}
		}
		return poisoned(t.Result)
	case *types.Record:
		if t.Anonymous {
			for _, f := range t.Fields {
				if poisoned(f.Type) {
					return true
				}
			}
		}
		for _, a := range t.Args {
			if poisoned(a) {
				return true
			}
		}
	}
	return false
}

// containsFun reports whether a value of type t can hold a function or an effect value (`Query
// a`, `Task a`): such values have no `==` and cannot be Dict keys. It looks through records,
// type arguments and union constructor payloads (`type Op = Op (Int -> Int)`).
func containsFun(t types.Type) bool { return containsFunIn(t, map[any]bool{}) }

// containsFunIn is containsFun that visits each record and union declaration once: they may be
// recursive.
func containsFunIn(t types.Type, seen map[any]bool) bool {
	switch t := t.(type) {
	case *types.Fun:
		return true
	case *types.App:
		if t.Con == "Query" || t.Con == "Task" {
			return true
		}
		for _, a := range t.Args {
			if containsFunIn(a, seen) {
				return true
			}
		}
	case *types.Union:
		for _, a := range t.Args {
			if containsFunIn(a, seen) {
				return true
			}
		}
		def := t
		if t.Def != nil {
			def = t.Def
		}
		if seen[def] {
			return false
		}
		seen[def] = true
		for _, ct := range def.Ctors { // a parameter in a payload is covered by Args above
			if ct.Arg != nil && containsFunIn(ct.Arg, seen) {
				return true
			}
		}
	case *types.Record:
		if seen[t] {
			return false
		}
		seen[t] = true
		for _, f := range t.Fields {
			if containsFunIn(f.Type, seen) {
				return true
			}
		}
	}
	return false
}

// containsEffect reports whether a value of type t can hold an effect value (`Query a`, `Task
// a`), looking where containsFun looks; it picks the wording of containsFun's errors.
func containsEffect(t types.Type) bool { return containsEffectIn(t, map[any]bool{}) }

func containsEffectIn(t types.Type, seen map[any]bool) bool {
	switch t := t.(type) {
	case *types.App:
		if t.Con == "Query" || t.Con == "Task" {
			return true
		}
		for _, a := range t.Args {
			if containsEffectIn(a, seen) {
				return true
			}
		}
	case *types.Union:
		for _, a := range t.Args {
			if containsEffectIn(a, seen) {
				return true
			}
		}
		def := t
		if t.Def != nil {
			def = t.Def
		}
		if seen[def] {
			return false
		}
		seen[def] = true
		for _, ct := range def.Ctors {
			if ct.Arg != nil && containsEffectIn(ct.Arg, seen) {
				return true
			}
		}
	case *types.Record:
		if seen[t] {
			return false
		}
		seen[t] = true
		for _, f := range t.Fields {
			if containsEffectIn(f.Type, seen) {
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
	if types.IsErr(l) || types.IsErr(r) {
		return types.Err
	}
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
		// two different brands of one base (`Int.NonNegative * Int.Positive`): both widen to
		// the base, and the operation returns it (chapter 4, rules 1 and 3)
		if lb, rb := brandBase(l), brandBase(r); lb != nil && rb != nil && lb.Name == rb.Name {
			return lb
		}
		c.errorf(diag.TypeMismatch, x.Span, "cannot apply `%s` to `%s` and `%s`", x.Op, types.String(l), types.String(r))
		return c.u.Fresh()
	}
	return lt
}

// brandBase is the built-in base of a numeric type: Int for `Int` and for `Cents = brand Int`.
func brandBase(t types.Type) *types.Con {
	switch t := t.(type) {
	case *types.Con:
		return t
	case *types.Brand:
		if b, ok := t.Base.(*types.Con); ok {
			return b
		}
	}
	return nil
}

func isNumeric(t types.Type) bool { _, ok := numericBase(t); return ok }

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
	tableT := c.record(tref, &types.App{Con: "Table", Args: []types.Type{rec}})
	c.record(x.Fn, &types.Fun{Params: []types.Type{tableT, info.IDType}, Result: res})
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
