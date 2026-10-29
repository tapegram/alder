package gen

import (
	"fmt"
	"math"
	"strconv"
	"strings"

	"alder/internal/ir"
	"alder/internal/types"
)

// Expression emission.
//
// An expression is emitted in one of two modes:
//   - expr: a Go expression, returned with its precedence (see prec*) so that operands are
//     parenthesized only where Go needs it;
//   - ret: Go statements that end by returning the expression's value (a function or closure
//     body). Let, Match, If and Hole are naturally statements: in ret mode they become `x := …`,
//     `switch`/`if` with a return in each branch, and `panic`; in expr mode they are wrapped in an
//     immediately-invoked closure `func() T { … }()`. HIf and HMatch are If and Match in HTML.
//
// Go rejects unused variables, so a Let or match binding whose name the body never uses is not
// emitted (the language is pure, so nothing is lost).

// Go operator precedence, plus levels for unary and primary expressions.
const (
	precOr      = 1
	precAnd     = 2
	precCompare = 3
	precAdd     = 4
	precMul     = 5
	precUnary   = 6
	precAtom    = 7
)

var binPrec = map[string]int{
	"||": precOr, "&&": precAnd,
	"==": precCompare, "!=": precCompare, "<": precCompare, "<=": precCompare, ">": precCompare, ">=": precCompare,
	"+": precAdd, "-": precAdd, "++": precAdd,
	"*": precMul, "/": precMul, "%": precMul,
}

// fn emits one top-level value: it numbers the temporaries the emitter introduces.
type fn struct {
	g     *gen
	temps int
	// jumps are, while a mutated match is emitted (mutatedMatch), the statements its moved arms
	// are emitted as, by arm index (the cases, then the default)
	jumps map[*ir.Match]map[int]string
	// share is the decision-tree nodes a match shares between branches (share.go)
	share sharing
	// lets records which lets are emitted (letEmitted), for every count over this value's IR
	lets map[*ir.Let]bool
}

// temp declares a fresh temporary in s ("_a1", "_u2", …) and returns its Go name.
func (f *fn) temp(s *scope, prefix string) string {
	_, g := f.tempVar(s, prefix)
	return g
}

// tempVar declares a fresh temporary in s and returns its scope key, which no Alder local can
// have (an ir.Var of that name refers to it), and its Go name.
func (f *fn) tempVar(s *scope, prefix string) (key, g string) {
	f.temps++
	name := fmt.Sprintf("%s%d", prefix, f.temps)
	key = "\x00" + name
	return key, s.declareAs(key, name)
}

// scope maps Alder locals to their Go names and tracks the Go names declared in the current Go
// block.
//
// Alder identifiers are ASCII letters, digits and '_' (and may start with '_'), so the Go names
// the emitter derives — "n_2", "_a1" — are also Alder names a user could write (only the reserved
// escape "type__" is not: Alder names never contain "__"). A name
// is therefore never chosen by its spelling alone: declare picks the first candidate that is
// neither declared in the current Go block nor the Go name of another visible binding, so no
// declaration can shadow a variable the code still refers to.
type scope struct {
	vars  map[string]string // visible bindings: Alder name (or tempKey) → Go name
	block map[string]bool
}

func newScope() *scope { return &scope{vars: map[string]string{}, block: map[string]bool{}} }

// child is a scope for a nested Go block.
func (s *scope) child() *scope { return &scope{vars: s.vars, block: map[string]bool{}} }

// declare binds Alder name in the current block and returns its Go name.
func (s *scope) declare(name string) string {
	if name == "_" {
		return "_"
	}
	return s.declareAs(name, local(name))
}

// declareAs binds key to a Go name based on base, renamed base_2, base_3, … when base is taken.
func (s *scope) declareAs(key, base string) string {
	g := base
	for i := 2; s.taken(key, g); i++ {
		g = fmt.Sprintf("%s_%d", base, i)
	}
	s.block[g] = true
	vars := make(map[string]string, len(s.vars)+1)
	for k, v := range s.vars {
		vars[k] = v
	}
	vars[key] = g
	s.vars = vars
	return g
}

// taken reports whether Go name g is unavailable for key: declared in this block, or the Go name
// of another visible binding.
func (s *scope) taken(key, g string) bool {
	if s.block[g] {
		return true
	}
	for k, v := range s.vars {
		if v == g && k != key {
			return true
		}
	}
	return false
}

func (s *scope) lookup(name string) string {
	if g, ok := s.vars[name]; ok {
		return g
	}
	return local(name)
}

// ---- ret mode

// ret writes statements that return e's value.
func (f *fn) ret(w *strings.Builder, e ir.Expr, s *scope) { f.retK(w, e, s, nil) }

// retK writes statements that return e's value as k renders it: k is applied to each value
// returned (the body of a Let, each branch of a Match or If), nil meaning f.code. An effect
// Bind's continuation uses it to return its body as a Task (effects.go).
func (f *fn) retK(w *strings.Builder, e ir.Expr, s *scope, k func(ir.Expr, *scope) string) {
	if name, ok := f.share.names[e]; ok { // a shared subtree, declared once (share.go)
		fmt.Fprintf(w, "return %s()\n", name)
		return
	}
	if _, ok := f.hoisted(e); ok { // a mutate build (mutation.go): a value, rendered by k
		if k == nil {
			k = f.code
		}
		fmt.Fprintf(w, "return %s\n", k(e, s))
		return
	}
	if ms := f.mutantsOf(e); ms != nil { // a mutate build (mutation.go)
		f.switched(w, e, ms, s, k)
		return
	}
	switch x := e.(type) {
	case *ir.Let:
		if !f.letEmitted(x) {
			f.retK(w, x.Body, s, k)
			return
		}
		v := f.typedExpr(x.Value, s)
		fmt.Fprintf(w, "%s := %s\n", s.declare(x.Name), v)
		f.retK(w, x.Body, s, k)
	case *ir.Match:
		if ms := f.localMutants(x); ms != nil { // a mutate build (mutation.go)
			f.mutatedMatch(w, x, ms, s, k)
			return
		}
		f.match(w, x, s, k)
	case *ir.HMatch:
		f.match(w, (*ir.Match)(x), s, k)
	case *ir.If:
		if ms := f.localMutants(x); ms != nil { // a mutate build (mutation.go)
			f.mutatedIf(w, x, ms, s, k)
			return
		}
		f.ifElse(w, x.Cond, x.Then, x.Else, s, k)
	case *ir.HIf:
		f.ifElse(w, x.Cond, x.Then, x.Else, s, k)
	case *ir.Hole:
		fmt.Fprintf(w, "panic(%s)\n", strconv.Quote("alder: hole ?"+x.Name))
	default:
		if k == nil {
			fmt.Fprintf(w, "return %s\n", f.code(e, s))
		} else {
			fmt.Fprintf(w, "return %s\n", k(e, s))
		}
	}
}

func (f *fn) ifElse(w *strings.Builder, c, then, els ir.Expr, s *scope, k func(ir.Expr, *scope) string) {
	fmt.Fprintf(w, "if %s {\n", cond(f.code(c, s)))
	f.retK(w, then, s.child(), k)
	w.WriteString("}\n")
	f.retK(w, els, s, k)
}

// cond guards a Go `if`/`switch` header: a composite literal there must be parenthesized.
func cond(c string) string {
	if strings.Contains(c, "{") {
		return "(" + c + ")"
	}
	return c
}

// closure is e as an immediately-invoked closure returning its value.
func (f *fn) closure(e ir.Expr, s *scope) string {
	var w strings.Builder
	f.ret(&w, e, s.child())
	return "func() " + GoType(e.Type()) + " {\n" + w.String() + "}()"
}

// ---- expr mode

// code is e as a Go expression.
func (f *fn) code(e ir.Expr, s *scope) string {
	c, _ := f.expr(e, s)
	return c
}

// operand is e as a Go expression of at least precedence p, parenthesized when needed.
func (f *fn) operand(e ir.Expr, s *scope, p int) string {
	c, cp := f.expr(e, s)
	if cp < p {
		return "(" + c + ")"
	}
	return c
}

// typedExpr is e as a Go expression whose type is e's type even where Go infers the type from
// the expression (`x := …`, a generic argument): an untyped integer constant becomes int64.
func (f *fn) typedExpr(e ir.Expr, s *scope) string {
	c := f.code(e, s)
	if untypedInt(e) {
		return "int64(" + c + ")"
	}
	return c
}

// untypedInt reports whether e is emitted as an untyped Go integer constant: an Int literal.
// (An operator on two literals routes one through rt.Int, so it is typed: see binary.)
func untypedInt(e ir.Expr) bool {
	x, ok := e.(*ir.Lit)
	return ok && x.Kind == ir.LitInt
}

// expr is e as a Go expression and its precedence.
func (f *fn) expr(e ir.Expr, s *scope) (string, int) {
	if h, ok := f.hoisted(e); ok { // a mutate build (mutation.go)
		return h, precAtom
	}
	if ms := f.mutantsOf(e); ms != nil { // a mutate build (mutation.go)
		return f.switchedExpr(e, ms, GoType(e.Type()), s, nil), precAtom
	}
	switch x := e.(type) {
	case *ir.Lit:
		return lit(x)
	case *ir.Var:
		return f.variable(x, s), precAtom
	case *ir.Call:
		return f.call(x, s)
	case *ir.Partial:
		return f.partial(x, s), precAtom
	case *ir.Lambda:
		return f.lambda(x, s), precAtom
	case *ir.Record:
		return f.record(x, recordTypeName(x), s), precAtom
	case *ir.Update:
		return f.update(x, s), precAtom
	case *ir.Field:
		v := f.operand(x.Expr, s, precAtom) + "." + Field(x.Name)
		if f.g.boxed.of(x.Expr.Type(), x.Name) {
			return "(*" + v + ")", precAtom
		}
		return v, precAtom
	case *ir.Ctor:
		return f.ctor(x, s), precAtom
	case *ir.List:
		var es []string
		for _, el := range x.Elems {
			es = append(es, f.code(el, s))
		}
		return GoType(x.Ty) + "{" + strings.Join(es, ", ") + "}", precAtom
	case *ir.PageRef:
		return f.pageRef(x, s), precAtom
	case *ir.Op:
		return f.op(x, s)
	case *ir.Str:
		var ps []string
		for _, p := range x.Parts {
			ps = append(ps, f.code(p, s))
		}
		return "rt.Concat(" + strings.Join(ps, ", ") + ")", precAtom
	case *ir.Widen:
		return widenName(x.Page.Module) + "(" + f.code(x.Expr, s) + ")", precAtom
	case *ir.Let, *ir.Match, *ir.If, *ir.Hole:
		return f.closure(e, s), precAtom
	case *ir.Elem, *ir.Comp, *ir.HText, *ir.HExpr, *ir.HFor, *ir.HIf, *ir.HMatch, *ir.Input, *ir.FormEl:
		return f.html(e, s), precAtom // html.go
	case *ir.EffPure, *ir.EffBind, *ir.EffMap, *ir.EffAll, *ir.EffFail, *ir.EffNotFound, *ir.CapCall,
		*ir.DbGet, *ir.DbSelect, *ir.DbRun, *ir.DbInsert, *ir.DbUpdate, *ir.DbDelete,
		*ir.DbScoped, *ir.DbGetScoped, *ir.DbUpdateScoped, *ir.DbDeleteScoped, *ir.DbInsertScoped,
		*ir.ColumnRef, *ir.TableRef, *ir.Cond, *ir.Ord, *ir.TestRun, *ir.TestTrace, *ir.TestFake,
		*ir.TestLogin, *ir.TestGet, *ir.TestSubmit, *ir.TestSubmitRaw, *ir.TestClick:
		return f.effect(e, s), precAtom // effects.go
	case *ir.FuzzDerive:
		return f.g.fuzzDerive(x), precAtom
	case *ir.TestEq: // as rt.Eq for `==`, typed: an Int literal passed as `any` would be a Go int
		f.g.testNode(x)
		return "rtesting.Eq(" + f.typedExpr(x.Left, s) + ", " + f.typedExpr(x.Right, s) + ")", precAtom
	}
	panic(fmt.Sprintf("gen: cannot emit %T", e))
}

func lit(x *ir.Lit) (string, int) {
	switch x.Kind {
	case ir.LitInt:
		s := strconv.FormatInt(x.Int, 10)
		if x.Int < 0 {
			return s, precUnary
		}
		return s, precAtom
	case ir.LitFloat:
		s := floatLit(x.Float)
		if strings.HasPrefix(s, "-") && !strings.HasPrefix(s, "math.") {
			return s, precUnary
		}
		return s, precAtom
	case ir.LitBool:
		return strconv.FormatBool(x.Bool), precAtom
	case ir.LitText:
		return strconv.Quote(x.Text), precAtom
	}
	return "struct{}{}", precAtom
}

// floatLit formats f so that Go reads it as a float constant ("1.0", not "1"). Negative zero,
// which no Go constant is (Go folds -0.0 to 0), is math.Copysign(0, -1): the parser keeps the
// sign of `-0.0`, and so does the program.
func floatLit(f float64) string {
	if f == 0 && math.Signbit(f) {
		return "math.Copysign(0, -1)"
	}
	s := strconv.FormatFloat(f, 'g', -1, 64)
	if !strings.ContainsAny(s, ".eEIN") {
		s += ".0"
	}
	return s
}

// variable is a reference to a named value.
func (f *fn) variable(x *ir.Var, s *scope) string {
	switch x.Kind {
	case ir.VarLocal:
		return s.lookup(x.Name)
	case ir.VarStd:
		name := stdGoName(x.Module, x.Name)
		if _, isFun := x.Ty.(*types.Fun); isFun { // a function value, not called: instantiate it
			return name + stdInstance(x.Module, x.Name, x.Ty)
		}
		// a stdlib constant is a Go function; Dict.empty needs its type arguments
		if app, ok := x.Ty.(*types.App); ok && app.Con == "Dict" {
			return name + typeArgs(app.Args) + "()"
		}
		return name + "()"
	}
	name := GoName(x.Module, x.Name)
	v := f.g.values[x.Module+"."+x.Name]
	if v != nil {
		name += f.g.instantiate(v, x.Ty)
		if f.g.thunks[v] {
			name += "()"
		}
	}
	return name
}

// call emits a saturated call; arguments beyond the callee's parameters apply its result.
func (f *fn) call(x *ir.Call, s *scope) (string, int) {
	ft, _ := x.Fn.Type().(*types.Fun)
	n := len(x.Args)
	if ft != nil && len(ft.Params) < n {
		n = len(ft.Params)
	}
	if isNot(x.Fn) && n == 1 {
		return "!" + f.operand(x.Args[0], s, precUnary), precUnary
	}
	if q, ok := f.querySel(x.Fn, x.Args, s); ok && n == len(x.Args) { // browser.go
		return q, precAtom
	}
	if v, ok := x.Fn.(*ir.Var); ok && v.Kind == ir.VarStd && v.Module == "Test" && v.Name == "member" && len(x.Args) == 2 {
		return f.member(x, s), precAtom // browser.go
	}
	var args []string
	for i, a := range x.Args[:n] {
		args = append(args, f.argument(x.Fn, i, a, s))
	}
	c, p := f.apply(x.Fn, args, s)
	for rest := x.Args[n:]; len(rest) > 0; {
		rt, _ := ft.Result.(*types.Fun)
		k := len(rest)
		if rt != nil && len(rt.Params) < k {
			k = len(rt.Params)
		}
		var as []string
		for _, a := range rest[:k] {
			as = append(as, f.code(a, s))
		}
		if p < precAtom {
			c = "(" + c + ")"
		}
		c, p = c+"("+strings.Join(as, ", ")+")", precAtom
		rest, ft = rest[k:], rt
	}
	return c, p
}

// argument is argument i of a call of callee: typed where Go would otherwise infer a type
// parameter from an untyped constant as int — when the parameter of a stdlib callee is a bare
// type variable. (A generic value of the app is always called with explicit type arguments.)
func (f *fn) argument(callee ir.Expr, i int, a ir.Expr, s *scope) string {
	if v, ok := callee.(*ir.Var); ok && v.Kind == ir.VarStd && stdVarParam(v.Module, v.Name, i) {
		return f.typedExpr(a, s)
	}
	return f.code(a, s)
}

// isNot reports whether e is Prelude.not, emitted as Go's `!`.
func isNot(e ir.Expr) bool {
	v, ok := e.(*ir.Var)
	return ok && v.Kind == ir.VarStd && v.Module == "Prelude" && v.Name == "not"
}

// apply calls callee with already emitted arguments.
func (f *fn) apply(callee ir.Expr, args []string, s *scope) (string, int) {
	if isNot(callee) && len(args) == 1 { // only from a partial: the argument is a parameter
		return "!" + args[0], precUnary
	}
	var c string
	if v, ok := callee.(*ir.Var); ok && v.Kind == ir.VarStd {
		c = stdGoName(v.Module, v.Name) // called: Go infers its type arguments
	} else if ok {
		c = f.variable(v, s)
	} else {
		c = f.operand(callee, s, precAtom)
	}
	return c + "(" + strings.Join(args, ", ") + ")", precAtom
}

// partial is a closure over the missing parameters.
func (f *fn) partial(x *ir.Partial, s *scope) string {
	ft := x.Fn.Type().(*types.Fun)
	var args, params []string
	for i, g := range x.Given {
		args = append(args, f.argument(x.Fn, i, g, s))
	}
	c := s.child()
	for _, t := range ft.Params[len(x.Given):] {
		p := f.temp(c, "_a")
		params = append(params, p+" "+GoType(t))
		args = append(args, p)
	}
	body, _ := f.apply(x.Fn, args, s)
	return "func(" + strings.Join(params, ", ") + ") " + GoType(ft.Result) + " {\nreturn " + body + "\n}"
}

func (f *fn) lambda(x *ir.Lambda, s *scope) string {
	c := s.child()
	var params []string
	for _, p := range x.Params {
		params = append(params, c.declare(p.Name)+" "+GoType(p.Type))
	}
	var w strings.Builder
	f.ret(&w, x.Body, c)
	res := "any"
	if ft, ok := x.Ty.(*types.Fun); ok {
		res = GoType(ft.Result)
	}
	return "func(" + strings.Join(params, ", ") + ") " + res + " {\n" + w.String() + "}"
}

// recordTypeName is the Go type a record literal builds: its named record, else the anonymous
// struct of its type.
func recordTypeName(x *ir.Record) string {
	if x.T != nil && x.T.Name != "" {
		var args []types.Type
		if r, ok := x.Ty.(*types.Record); ok {
			args = r.Args
		}
		return GoName(x.T.Module, x.T.Name) + typeArgs(args)
	}
	return GoType(x.Ty)
}

func (f *fn) record(x *ir.Record, typ string, s *scope) string {
	var fs []string
	for _, fi := range x.Fields {
		v := f.code(fi.Value, s)
		if x.T != nil && f.g.boxed.has(x.T.Module, x.T.Name, fi.Name) {
			v = "new(" + v + ")"
		}
		fs = append(fs, Field(fi.Name)+": "+v)
	}
	return typ + "{" + strings.Join(fs, ", ") + "}"
}

// isPath reports whether e is a local variable or a field path from one: cheap to repeat.
func isPath(e ir.Expr) bool {
	switch x := e.(type) {
	case *ir.Var:
		return x.Kind == ir.VarLocal
	case *ir.Field:
		return isPath(x.Expr)
	}
	return false
}

// update copies the base with fields replaced: a full literal when the base is a path, else a
// closure that copies it into a temporary and assigns the new fields. A boxed field (types.go)
// left alone shares the base's pointer; a replaced one is boxed anew.
func (f *fn) update(x *ir.Update, s *scope) string {
	rec, _ := x.Ty.(*types.Record)
	typ := GoType(x.Ty)
	value := func(fi ir.FieldInit) string {
		v := f.code(fi.Value, s)
		if f.g.boxed.of(x.Ty, fi.Name) {
			v = "new(" + v + ")"
		}
		return v
	}
	if isPath(x.Base) && rec != nil {
		base := f.code(x.Base, s)
		var fs []string
		for _, fd := range rec.Fields {
			v := base + "." + Field(fd.Name)
			for _, fi := range x.Fields {
				if fi.Name == fd.Name {
					v = value(fi)
				}
			}
			fs = append(fs, Field(fd.Name)+": "+v)
		}
		return typ + "{" + strings.Join(fs, ", ") + "}"
	}
	base := f.code(x.Base, s)
	t := f.temp(s.child(), "_u")
	var w strings.Builder
	fmt.Fprintf(&w, "func() %s {\n%s := %s\n", typ, t, base)
	for _, fi := range x.Fields {
		fmt.Fprintf(&w, "%s.%s = %s\n", t, Field(fi.Name), value(fi))
	}
	fmt.Fprintf(&w, "return %s\n}()", t)
	return w.String()
}

func ctorT(u *ir.UnionType, name string) ir.CtorT {
	for _, c := range u.Ctors {
		if c.Name == name {
			return c
		}
	}
	panic(fmt.Sprintf("gen: union %s.%s has no constructor %s", u.Module, u.Name, name))
}

// ctor builds a union value.
func (f *fn) ctor(x *ir.Ctor, s *scope) string {
	ut, _ := x.Ty.(*types.Union)
	var args []types.Type
	if ut != nil {
		args = ut.Args
	}
	// inner is the payload a constructor function takes: the T of a wrapped { value: T }.
	inner := func() string {
		if r, ok := x.Payload.(*ir.Record); ok && len(r.Fields) == 1 {
			return f.code(r.Fields[0].Value, s)
		}
		return f.operand(x.Payload, s, precAtom) + ".Value"
	}
	if x.U.Module == "Prelude" {
		switch x.Name {
		case "Just":
			return "rt.Just" + typeArgs(args) + "(" + inner() + ")"
		case "Nothing":
			return "rt.Nothing" + typeArgs(args) + "()"
		case "Ok":
			return "rt.Ok" + typeArgs(args) + "(" + inner() + ")"
		case "Err":
			return "rt.Err" + typeArgs(args) + "(" + inner() + ")"
		case "Rerender":
			return "rt.Rerender()"
		case "Redirect":
			return "rt.Redirect(" + inner() + ")"
		case "Reject":
			return "rt.Reject(" + inner() + ")"
		case "Shown", "Rerendered", "PageMissing":
			return "rtesting.Answer{Tag: rtesting." + x.Name + "}"
		case "Redirected":
			return "rtesting.Answer{Tag: rtesting.Redirected, Page: " + inner() + "}"
		case "FormRejected":
			return "rtesting.FormRejectedAnswer(" + inner() + ")"
		case "RequestFailed":
			return "rtesting.Answer{Tag: rtesting.RequestFailed, Text: " + inner() + "}"
		}
		panic(fmt.Sprintf("gen: unknown Prelude constructor %s", x.Name))
	}
	u := x.U
	ta := ""
	if len(u.Params) > 0 {
		ta = typeArgs(args)
	}
	ct := ctorT(u, x.Name)
	switch {
	case ct.Payload == nil && ta == "":
		return ctorName(u.Module, u.Name, x.Name)
	case ct.Payload == nil:
		return GoName(u.Module, u.Name) + ta + "{Tag: " + tagConst(u.Module, u.Name, x.Name) + "}"
	case wrapped(ct.Payload):
		return ctorName(u.Module, u.Name, x.Name) + ta + "(" + inner() + ")"
	}
	var p string
	if r, ok := x.Payload.(*ir.Record); ok && r.T == ct.Payload {
		typ := payloadType(u, ct, ta)
		if ct.Payload.Name != "" { // a named record: at this use's type arguments
			typ = recordTypeName(r)
		}
		p = f.record(r, typ, s)
	} else {
		p = f.code(x.Payload, s)
	}
	return ctorName(u.Module, u.Name, x.Name) + ta + "(" + p + ")"
}

// pageRef is a link to a page: its path with the path parameters filled in from Params (each
// as text, escaped as one segment), and the other parameters as the query, in declaration
// order: rt.KV for a required one; rt.QueryKV, which leaves it out at its default (a Maybe
// without one: at Nothing), for the others; rt.QueryKVs for a List (one key per element).
func (f *fn) pageRef(x *ir.PageRef, s *scope) string {
	path := x.Page.Path
	inPath := map[string]bool{}
	var parts []string
	for path != "" {
		i := strings.Index(path, "{")
		j := strings.Index(path, "}")
		if i < 0 || j < i {
			parts = append(parts, strconv.Quote(path))
			break
		}
		if i > 0 {
			parts = append(parts, strconv.Quote(path[:i]))
		}
		name := path[i+1 : j]
		inPath[name] = true
		v := `""`
		if x.Params != nil {
			v = "rt.PathSegment(" + f.text(f.pageParam(x.Params, name), s) + ")"
		}
		parts = append(parts, v)
		path = path[j+1:]
	}
	p := strconv.Quote(x.Page.Path)
	if len(parts) > 1 || (len(parts) == 1 && parts[0] != p) {
		p = "rt.Concat(" + strings.Join(parts, ", ") + ")"
	}
	if x.Params == nil {
		return "rt.Page{Path: " + p + "}"
	}
	var items []kvItem
	for _, fd := range x.Page.Params.Fields {
		if !inPath[fd.Name] {
			items = append(items, kvItem{name: fd.Key(), t: fd.Type, v: f.pageParam(x.Params, fd.Name), def: fd.Default, omitBlank: true})
		}
	}
	if q := f.kvList(items, s); q != "nil" {
		return "rt.Page{Path: " + p + ", Query: " + q + "}"
	}
	return "rt.Page{Path: " + p + "}"
}

// pageParam is page parameter name of params: the field's value when params is a record
// literal, else the field of params.
func (f *fn) pageParam(params ir.Expr, name string) ir.Expr {
	if r, ok := params.(*ir.Record); ok {
		for _, fi := range r.Fields {
			if fi.Name == name {
				return fi.Value
			}
		}
	}
	if rec, ok := params.Type().(*types.Record); ok {
		for _, fd := range rec.Fields {
			if fd.Name == name {
				return &ir.Field{Typed: ir.Typed{Ty: fd.Type}, Expr: params, Name: name}
			}
		}
	}
	panic(fmt.Sprintf("gen: a page link has no parameter %s", name))
}

// texts is list (of element type elem) as a Go []string of the elements' text.
func (f *fn) texts(list ir.Expr, elem types.Type, s *scope) string {
	if GoType(elem) == "string" {
		return f.code(list, s)
	}
	c := s.child()
	key, v := f.tempVar(c, "_v")
	body := f.text(&ir.Var{Typed: ir.Typed{Ty: elem}, Kind: ir.VarLocal, Name: key}, c)
	return "std.ListMap(func(" + v + " " + GoType(elem) + ") string {\nreturn " + body + "\n}, " + f.code(list, s) + ")"
}

// op emits a binary operator.
func (f *fn) op(x *ir.Op, s *scope) (string, int) {
	call := func(fn string) (string, int) {
		return fn + "(" + f.code(x.L, s) + ", " + f.code(x.R, s) + ")", precAtom
	}
	switch x.Kind {
	case "??":
		return call("rt.OrElse")
	case "++":
		if app, ok := x.OperandT.(*types.App); ok && app.Con == "List" {
			return call("rt.Append")
		}
		return f.binary("+", x, s)
	case "==", "!=":
		switch eqKind(x.OperandT) {
		case eqPrim:
			return f.binary(x.Kind, x, s)
		case eqTag:
			return f.operand(x.L, s, precAtom) + ".Tag " + x.Kind + " " + f.operand(x.R, s, precAtom) + ".Tag", precCompare
		}
		c, _ := call("rt.Eq")
		if x.Kind == "!=" {
			return "!" + c, precUnary
		}
		return c, precAtom
	case "<", "<=", ">", ">=":
		switch t := x.OperandT.(type) {
		case *types.Con:
			if t.Name == "Date" || t.Name == "Time" {
				c, _ := call("rt." + t.Name + "Cmp")
				return c + " " + x.Kind + " 0", precCompare
			}
		case *types.Union: // a scalar union: constructors in declaration order
			return f.operand(x.L, s, precAtom) + ".Tag " + x.Kind + " " + f.operand(x.R, s, precAtom) + ".Tag", precCompare
		}
	case "%":
		if GoType(x.OperandT) == "float64" {
			return call("math.Mod")
		}
	}
	return f.binary(x.Kind, x, s)
}

// binary emits a Go binary operator.
//
// Go evaluates an operator on constants at compile time and rejects what Alder defines at run
// time: `9223372036854775807 + 1` overflows (Alder wraps) and `1 / 0` divides by zero (Alder
// fails when it runs). So the emitter keeps Go from folding: when both operands of an arithmetic
// operator are constant expressions, its left literal goes through the identity rt.Int (or
// rt.Float), which makes it a value; and the literal divisor of `/` or `%` always does.
func (f *fn) binary(op string, x *ir.Op, s *scope) (string, int) {
	p := binPrec[op]
	l, r := f.operand(x.L, s, p), f.operand(x.R, s, p+1)
	if arith[op] { // a switched operand (a mutate build) is no constant: it keeps its switch
		if constExpr(x.L) && constExpr(x.R) && f.mutantsOf(x.L) == nil {
			l = numValue(x.L, l)
		}
		if (op == "/" || op == "%") && f.mutantsOf(x.R) == nil {
			r = numValue(x.R, r)
		}
	}
	return l + " " + op + " " + r, p
}

var arith = map[string]bool{"+": true, "-": true, "*": true, "/": true, "%": true}

// constExpr reports whether e is an Alder constant expression: a number literal, or an arithmetic
// operator on two of them.
func constExpr(e ir.Expr) bool {
	switch x := e.(type) {
	case *ir.Lit:
		return x.Kind == ir.LitInt || x.Kind == ir.LitFloat
	case *ir.Op:
		return arith[x.Kind] && constExpr(x.L) && constExpr(x.R)
	}
	return false
}

// numValue is code c of e routed through rt.Int or rt.Float when e is a number literal.
func numValue(e ir.Expr, c string) string {
	if l, ok := e.(*ir.Lit); ok {
		v, _ := lit(l)
		switch l.Kind {
		case ir.LitInt:
			return "rt.Int(" + v + ")"
		case ir.LitFloat:
			return "rt.Float(" + v + ")"
		}
	}
	return c
}

const (
	eqPrim = iota // Go ==
	eqTag         // a union without payloads: compare tags
	eqDeep        // rt.Eq
)

func eqKind(t types.Type) int {
	switch t := t.(type) {
	case *types.Con:
		switch t.Name {
		case "Int", "Float", "Bool", "Text", "Unit":
			return eqPrim
		}
	case *types.Union:
		if t.Module == "Prelude" {
			return eqDeep
		}
		for _, c := range t.Constructors() {
			if c.Arg != nil {
				return eqDeep
			}
		}
		return eqTag
	}
	return eqDeep
}
