package ir

import (
	"fmt"
	"maps"
	"slices"
	"strings"

	"alder/internal/check"
	"alder/internal/format"
	"alder/internal/syntax"
	"alder/internal/types"
)

// Property tests and Fuzz.any (spec §4): Lower still sees brands, so it is here that each
// argument type of a property and each `Fuzz.any` becomes a GenTree, from which gen emits one
// generator and one literal printer per type.

// isFuzzAny reports whether v is `Fuzz.any`.
func isFuzzAny(v *syntax.Var) bool {
	return v.Ref.Kind == syntax.RefStd && v.Ref.Module == "Fuzz" && v.Ref.Name == "any"
}

// fuzzDerive lowers `Fuzz.any` to the generator of the type the checker recorded for it.
func (l *lowerer) fuzzDerive(v *syntax.Var) Expr {
	t, ok := l.res.FuzzAny[v]
	if !ok {
		panic(fmt.Sprintf("ir: no type recorded for Fuzz.any at %s (checker bug)", pos(v.Span)))
	}
	g, ok := l.genTree(t)
	if !ok {
		panic(fmt.Sprintf("ir: Fuzz.any at %s: %s has no generator (the checker's E0038)", pos(v.Span), types.String(t)))
	}
	return &FuzzDerive{Typed: Typed{l.typeOf(v)}, Gen: g}
}

// propTest lowers property test d's body to a TestProp.
func (l *lowerer) propTest(d *syntax.TestDecl, info check.PropInfo) *TestProp {
	p := &TestProp{Typed: Typed{l.typeOf(d.Body)}, Defaulted: info.Defaulted}
	qual := func(m string) string { return l.prog.QualifierFor(l.module, m) }
	for _, t := range info.Types {
		p.Types = append(p.Types, types.StringQ(t, l.module, qual))
	}
	defer func() {
		p.Quals = l.literalQuals(p)
		// a forAll value with no printer is shown with rt.Show, which is no Alder: no case
		if info.Kind == check.PropFun || p.ForAllLit != nil {
			p.Case = l.pasteCase(d, info)
		}
	}()
	body := l.expr(d.Body)
	if info.Kind == check.PropFun {
		p.Fn = body
		for i, t := range info.Types {
			p.Params = append(p.Params, info.Params[i].Name)
			g, ok := l.genTree(t)
			if !ok {
				panic(fmt.Sprintf("ir: test %q: %s has no generator (the checker's E0038)", d.Name, types.String(t)))
			}
			p.Gens = append(p.Gens, g)
		}
		return p
	}
	p.ForAllGen, p.Fn = forAllArgs(body)
	if p.ForAllGen == nil {
		panic(fmt.Sprintf("ir: test %q: a Fuzz.forAll body that is no application of it", d.Name))
	}
	name := "value"
	if len(info.Params) == 1 {
		name = info.Params[0].Name
	}
	p.Params = []string{name}
	if g, ok := l.genTree(info.Types[0]); ok {
		p.ForAllLit = g
	}
	return p
}

// pasteCase is the template of property test d's paste-ready plain test (spec §6): `test
// "<name> (case)" = ` and the property's body with the lambda removed (for a Fuzz.forAll body,
// forAll and its generator too; a function written there that is no lambda is applied to its
// argument), each use of an argument a placeholder format.Paste writes and rtesting.Paste
// fills. "" when there is none: an argument used inside a string literal or markup, whose text
// the formatter does not rebuild.
func (l *lowerer) pasteCase(d *syntax.TestDecl, info check.PropInfo) string {
	var body syntax.Expr
	var params []string
	switch {
	case info.Kind == check.PropFun:
		lam, ok := d.Body.(*syntax.Lambda)
		if !ok {
			return ""
		}
		body = lam.Body
		for _, prm := range lam.Params {
			params = append(params, prm.Name)
		}
	default:
		fn := forAllFn(d.Body)
		if fn == nil {
			return ""
		}
		if lam, ok := fn.(*syntax.Lambda); ok && len(lam.Params) == 1 {
			body, params = lam.Body, []string{lam.Params[0].Name}
			break
		}
		arg := &syntax.Var{Name: "value", Ref: syntax.Ref{Kind: syntax.RefLocal, Name: "value"}}
		return l.pasteTemplate(d, &syntax.App{Fn: fn, Args: []syntax.Expr{arg}}, map[*syntax.Var]int{arg: 0})
	}
	holes := map[*syntax.Var]int{}
	scope := map[string]int{}
	for i, n := range params {
		scope[n] = i
	}
	paramUses(body, scope, holes)
	return l.pasteTemplate(d, body, holes)
}

// pasteTemplate is `test "<d's name> (case)" = ` and body printed with holes as placeholders,
// or "" when format.Paste cannot print it.
func (l *lowerer) pasteTemplate(d *syntax.TestDecl, body syntax.Expr, holes map[*syntax.Var]int) string {
	text, ok := format.Paste(l.prog.Modules[l.module], body, holes)
	if !ok {
		return ""
	}
	return "test " + alderString(d.Name+" (case)") + " = " + text
}

// forAllFn is the function of a `Fuzz.forAll gen f` (or forAllTask) body as written, also as
// `f |> Fuzz.forAll gen`; nil for anything else.
func forAllFn(e syntax.Expr) syntax.Expr {
	isForAll := func(e syntax.Expr) bool {
		v, ok := e.(*syntax.Var)
		return ok && v.Ref.Kind == syntax.RefStd && v.Ref.Module == "Fuzz" && (v.Ref.Name == "forAll" || v.Ref.Name == "forAllTask")
	}
	switch x := e.(type) {
	case *syntax.App:
		if isForAll(x.Fn) && len(x.Args) == 2 {
			return x.Args[1]
		}
	case *syntax.BinOp:
		if app, ok := x.R.(*syntax.App); ok && x.Op == "|>" && isForAll(app.Fn) && len(app.Args) == 1 {
			return x.L
		}
	}
	return nil
}

// paramUses adds to holes each variable under e that names a property argument: scope maps a
// name to its argument's index while it is one, -1 once a lambda parameter, `let`, pattern
// variable or `for` variable of e shadows it.
func paramUses(e syntax.Spanned, scope map[string]int, holes map[*syntax.Var]int) {
	shadow := func(names ...string) map[string]int {
		inner := maps.Clone(scope)
		for _, n := range names {
			if _, ok := inner[n]; ok {
				inner[n] = -1
			}
		}
		return inner
	}
	switch x := e.(type) {
	case *syntax.Var:
		if i, ok := scope[x.Name]; ok && i >= 0 && x.Module == "" && x.Ref.Kind == syntax.RefLocal {
			holes[x] = i
		}
		return
	case *syntax.Lambda:
		var names []string
		for _, p := range x.Params {
			names = append(names, p.Name)
		}
		paramUses(x.Body, shadow(names...), holes)
		return
	case *syntax.Block:
		inner := scope
		for _, s := range x.Stmts {
			paramUses(s.Expr, inner, holes)
			inner = shadowIn(inner, s.Name)
		}
		paramUses(x.Result, inner, holes)
		return
	case *syntax.Arm:
		bound := map[string]bool{}
		patternVars(x.Pat, bound)
		paramUses(x.Body, shadow(slices.Collect(maps.Keys(bound))...), holes)
		return
	case *syntax.HtmlFor:
		paramUses(x.List, scope, holes)
		paramUses(x.Body, shadow(x.Var), holes)
		return
	}
	for _, c := range syntax.Children(e) {
		paramUses(c, scope, holes)
	}
}

// shadowIn is scope with name, when it is an argument's, no longer one.
func shadowIn(scope map[string]int, name string) map[string]int {
	if _, ok := scope[name]; !ok {
		return scope
	}
	inner := maps.Clone(scope)
	inner[name] = -1
	return inner
}

// alderString is s as an Alder string literal.
func alderString(s string) string {
	r := strings.NewReplacer(`\`, `\\`, `"`, `\"`, "{", `\{`, "\n", `\n`, "\r", `\r`, "\t", `\t`)
	return `"` + r.Replace(s) + `"`
}

// literalQuals maps each module whose constructors or brands p's literals print (by path) to
// how the test's module writes them ("" for its own names): the generated printers name them
// through it (fuzz.Quals).
func (l *lowerer) literalQuals(p *TestProp) map[string]string {
	out := map[string]string{l.module: ""}
	seen := map[*GenTree]bool{}
	var walk func(*GenTree)
	walk = func(g *GenTree) {
		if g == nil || seen[g] {
			return
		}
		seen[g] = true
		mod := ""
		switch g.Kind {
		case GenUnion:
			mod = g.Union.Module
		case GenBrand:
			mod = BrandModule(g.Brand)
		}
		if _, ok := out[mod]; mod != "" && !ok {
			out[mod] = l.prog.QualifierFor(l.module, mod)
		}
		for _, e := range g.Elems {
			walk(e)
		}
	}
	for _, g := range p.Gens {
		walk(g)
	}
	walk(p.ForAllLit)
	return out
}

// forAllArgs are the generator and function of the lowered `Fuzz.forAll gen f` (or forAllTask,
// or `f |> Fuzz.forAll gen`): a Call of it with both arguments, or a Call of its Partial with
// the generator.
func forAllArgs(e Expr) (gen, fn Expr) {
	call, ok := e.(*Call)
	if !ok {
		return nil, nil
	}
	args := call.Args
	head := call.Fn
	if pa, ok := head.(*Partial); ok {
		args, head = append(append([]Expr{}, pa.Given...), args...), pa.Fn
	}
	if v, ok := head.(*Var); !ok || v.Kind != VarStd || v.Module != "Fuzz" || len(args) != 2 {
		return nil, nil
	}
	return args[0], args[1]
}

// derivedPrims are the builtin types with a generator of their own (spec §4).
var derivedPrims = map[string]bool{"Int": true, "Float": true, "Bool": true, "Text": true, "Unit": true, "Bytes": true, "Date": true, "Time": true}

// StdBrandFunc is the stdlib refinement brands, which have generators of their own (spec
// §0a), each with the name of its rt/fuzz generator. It is the one list: gen draws with it and
// the lowerer tells a stdlib brand from a user's by it.
var StdBrandFunc = map[string]string{"Text.NonEmpty": "NonEmpty", "Text.Email": "Email", "Text.Url": "URL", "Text.Secret": "Secret", "Int.Positive": "Positive", "Int.NonNegative": "NonNegative"}

// genTree is the derived generator of the concrete type t (brands included), shared by Key with
// every tree made before; ok is false when t has none (a function, Html, a type variable…).
func (l *lowerer) genTree(t types.Type) (g *GenTree, ok bool) {
	if l.gens == nil {
		l.gens = map[string]*GenTree{}
	}
	b := &genBuild{l: l}
	defer func() {
		if r := recover(); r != nil {
			if _, bad := r.(noGen); !bad {
				panic(r)
			}
			for _, k := range b.added { // a partial tree must not be found later
				delete(l.gens, k)
			}
			g, ok = nil, false
		}
	}()
	g = b.tree(l.res.U.Apply(t))
	for _, n := range b.made {
		n.Cyclic = reaches(n.Elems, n)
		if n.Kind == GenResult {
			n.Rec = []bool{reaches(n.Elems[:1], n), reaches(n.Elems[1:], n)}
		}
		if n.Kind == GenUnion {
			for i, e := range n.Elems {
				n.Rec[i] = e != nil && reaches([]*GenTree{e}, n)
			}
		}
	}
	return g, true
}

// noGen is genBuild's panic value for a type with no generator.
type noGen struct{}

// genBuild is one genTree call: the trees it made, and the keys it added to the lowerer's.
type genBuild struct {
	l     *lowerer
	made  []*GenTree
	added []string
}

func (b *genBuild) tree(t types.Type) *GenTree {
	key := types.String(t)
	if g, ok := b.l.gens[key]; ok {
		return g
	}
	g := &GenTree{Key: key, Type: b.l.eraseT(t)}
	remember := func() {
		b.l.gens[key] = g
		b.added = append(b.added, key)
		b.made = append(b.made, g)
	}
	switch x := t.(type) {
	case *types.Con:
		if x.Module != "" || !derivedPrims[x.Name] {
			panic(noGen{})
		}
		g.Kind, g.Prim = GenPrim, x.Name
	case *types.Brand:
		k := x.Module + "." + x.Name
		if _, std := StdBrandFunc[k]; std {
			g.Kind, g.Brand = GenStdBrand, k
		} else { // its base, kept when it passes the check; printed with its constructor
			g.Kind, g.Brand, g.Check = GenBrand, k, x.HasCheck
			g.Elems = []*GenTree{b.tree(x.Base)}
		}
	case *types.App:
		switch x.Con {
		case "Id": // a row id: Text
			g.Kind, g.Prim = GenPrim, "Id"
			remember()
			return g
		case "List":
			g.Kind = GenList
		case "Dict":
			g.Kind = GenDict
		default:
			panic(noGen{})
		}
		remember()
		for _, a := range x.Args {
			g.Elems = append(g.Elems, b.tree(a))
		}
		return g
	case *types.Record:
		if _, insert := b.l.insertTable(x); insert {
			panic(noGen{})
		}
		g.Kind = GenRecord
		remember() // before the fields: a record may reach itself through a List
		for _, f := range x.Fields {
			g.Names = append(g.Names, f.Name)
			g.Elems = append(g.Elems, b.tree(f.Type))
		}
		return g
	case *types.Union:
		if x.Module == "Prelude" {
			switch x.Name {
			case "Maybe":
				g.Kind = GenMaybe
			case "Result":
				g.Kind = GenResult
			default: // Nav, Answer: what a page or a browser step produces, not data
				panic(noGen{})
			}
			remember()
			for _, a := range x.Args {
				g.Elems = append(g.Elems, b.tree(a))
			}
			return g
		}
		g.Kind, g.Union = GenUnion, b.l.union(x)
		remember() // before the payloads: a recursive union reaches itself
		for _, ct := range x.Constructors() {
			g.Names = append(g.Names, ct.Name)
			g.Rec = append(g.Rec, false)
			var e *GenTree
			if arg, _ := types.CtorArg(x, ct.Name); arg != nil {
				e = b.tree(arg)
			}
			g.Elems = append(g.Elems, e)
		}
		return g
	default:
		panic(noGen{})
	}
	remember()
	return g
}

// reaches reports whether target is among the trees from (or below them).
func reaches(from []*GenTree, target *GenTree) bool {
	seen := map[*GenTree]bool{}
	stack := append([]*GenTree{}, from...)
	for len(stack) > 0 {
		n := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if n == nil || seen[n] {
			continue
		}
		if n == target {
			return true
		}
		seen[n] = true
		stack = append(stack, n.Elems...)
	}
	return false
}

// BrandName is the name of brand "Module.Name" as a program writes it without its module:
// "Slug" of "Todo.Slug".
func BrandName(brand string) string { return brand[strings.LastIndex(brand, ".")+1:] }

// BrandModule is the module of brand "Module.Name".
func BrandModule(brand string) string { return brand[:max(strings.LastIndex(brand, "."), 0)] }
