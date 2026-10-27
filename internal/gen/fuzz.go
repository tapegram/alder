package gen

import (
	"fmt"
	"maps"
	"slices"
	"strconv"
	"strings"
	"unicode"

	"alder/internal/ir"
	"alder/internal/types"
)

// Derived generators and literal printers (spec §4, §6). Each GenTree a program needs (a
// property's arguments, a `Fuzz.any`, the value of a `Fuzz.forAll` for printing) becomes, per
// distinct Key, a generator
//
//	func fuzz__Tree(c *fuzz.Choices, depth int) Todo__Tree
//
// that draws a value from the choices (depth is the recursion budget, fuzz.Depth at the root),
// and a literal printer
//
//	func lit__Tree(q fuzz.Quals, v Todo__Tree) string
//
// that prints a value as the Alder expression that builds it, in the module whose qualifiers
// q are (a constructor of another module is written `Todo.Leaf`). Builtin types and the stdlib
// brands are drawn and printed by rt/fuzz directly. They are emitted to fuzzFile, in the order
// first needed, in production builds too when a value of the program uses `Fuzz.any`.

// fuzzFile is the generated file of the derived generators and printers.
const fuzzFile = "fuzz__alder.go"

// fuzzSet is what the program needs of the derived generators: the trees whose generator
// (fuzz) or printer (lit) is used, in order, and each tree's Go name.
type fuzzSet struct {
	fuzz, lit  map[*ir.GenTree]bool
	fuzzOrder  []*ir.GenTree
	litOrder   []*ir.GenTree
	names      map[string]string // Key → mangled name
	namesTaken map[string]bool
}

func (g *gen) fuzzSet() *fuzzSet {
	if g.fuzzes == nil {
		g.fuzzes = &fuzzSet{fuzz: map[*ir.GenTree]bool{}, lit: map[*ir.GenTree]bool{}, names: map[string]string{}, namesTaken: map[string]bool{}}
	}
	return g.fuzzes
}

// needFuzz notes that t's generator is used, and so its parts'.
func (g *gen) needFuzz(t *ir.GenTree) {
	fs := g.fuzzSet()
	if t == nil || fs.fuzz[t] {
		return
	}
	fs.fuzz[t] = true
	if hasFunc(t) {
		fs.fuzzOrder = append(fs.fuzzOrder, t)
	}
	for _, e := range t.Elems {
		g.needFuzz(e)
	}
}

// needLit notes that t's printer is used, and so its parts'.
func (g *gen) needLit(t *ir.GenTree) {
	fs := g.fuzzSet()
	if t == nil || fs.lit[t] {
		return
	}
	fs.lit[t] = true
	if hasFunc(t) {
		fs.litOrder = append(fs.litOrder, t)
	}
	for _, e := range t.Elems {
		g.needLit(e)
	}
}

// hasFunc reports whether t gets a fuzz__ and lit__ function: every tree but a builtin type
// and a stdlib brand, which rt/fuzz draws and prints.
func hasFunc(t *ir.GenTree) bool { return t.Kind != ir.GenPrim && t.Kind != ir.GenStdBrand }

// fuzzName is the name of t's functions after "fuzz__" and "lit__": its Key with every run of
// characters that cannot be in a Go identifier made one '_' ("List Int" → "List_Int", a record
// "{ a: Int }" → "Rec_a_Int"), numbered "_2", "_3", … when another Key has the same.
func (g *gen) fuzzName(t *ir.GenTree) string {
	fs := g.fuzzSet()
	if n, ok := fs.names[t.Key]; ok {
		return n
	}
	var b strings.Builder
	under := false
	for _, r := range strings.ReplaceAll(t.Key, "{", " Rec ") {
		if r < unicode.MaxASCII && (unicode.IsLetter(r) || unicode.IsDigit(r)) {
			b.WriteRune(r)
			under = false
		} else if !under && b.Len() > 0 {
			b.WriteByte('_')
			under = true
		}
	}
	base := strings.TrimSuffix(b.String(), "_")
	n := base
	for i := 2; fs.namesTaken[n]; i++ {
		n = fmt.Sprintf("%s_%d", base, i)
	}
	fs.names[t.Key], fs.namesTaken[n] = n, true
	return n
}

// draw is the Go expression drawing a value of t from choices c with depth budget depth.
func (g *gen) draw(t *ir.GenTree, c, depth string) string {
	switch t.Kind {
	case ir.GenPrim:
		switch t.Prim {
		case "Int":
			return "fuzz.Int(" + c + ")"
		case "Float":
			return "fuzz.Float(" + c + ")"
		case "Bool":
			return "fuzz.Bool(" + c + ", 0.5)"
		case "Text", "Id":
			return "fuzz.Text(" + c + ")"
		case "Unit":
			return "struct{}{}"
		case "Bytes":
			return "fuzz.Bytes(" + c + ")"
		case "Date":
			return "fuzz.Date(" + c + ")"
		case "Time":
			return "fuzz.Time(" + c + ")"
		}
		panic("gen: no generator for builtin type " + t.Prim)
	case ir.GenStdBrand:
		return "fuzz." + stdBrandFunc[t.Brand] + "(" + c + ")"
	}
	return "fuzz__" + g.fuzzName(t) + "(" + c + ", " + depth + ")"
}

// stdBrandFunc is the rt/fuzz generator of each stdlib brand.
var stdBrandFunc = map[string]string{"Text.NonEmpty": "NonEmpty", "Text.Email": "Email", "Text.Url": "URL", "Text.Secret": "Secret", "Int.Positive": "Positive", "Int.NonNegative": "NonNegative"}

// lit is the Go expression printing the value v of t as an Alder literal.
func (g *gen) lit(t *ir.GenTree, v string) string {
	switch t.Kind {
	case ir.GenPrim:
		switch t.Prim {
		case "Int":
			return "fuzz.IntLit(" + v + ")"
		case "Float":
			return "fuzz.FloatLit(" + v + ")"
		case "Bool":
			return "fuzz.BoolLit(" + v + ")"
		case "Text", "Id":
			return "fuzz.TextLit(" + v + ")"
		case "Unit":
			return `"()"`
		case "Bytes":
			return "fuzz.BytesLit(" + v + ")"
		case "Date":
			return "fuzz.DateLit(fuzz.DaysOf(" + v + "))"
		case "Time":
			return "fuzz.TimeLit(" + v + ".UnixMilli())"
		}
		panic("gen: no printer for builtin type " + t.Prim)
	case ir.GenStdBrand:
		base := "fuzz.TextLit(" + v + ")"
		if strings.HasPrefix(t.Brand, "Int.") {
			base = "fuzz.IntLit(" + v + ")"
		}
		return "fuzz.Apply(" + strconv.Quote(t.Brand) + ", " + base + ")"
	}
	return "lit__" + g.fuzzName(t) + "(q, " + v + ")"
}

// litFunc is t's printer as a function value (for a type with a lit__ function, a closure over
// the qualifiers q in scope).
func (g *gen) litFunc(t *ir.GenTree) string {
	if hasFunc(t) {
		return "func(x " + GoType(t.Type) + ") string { return " + g.lit(t, "x") + " }"
	}
	if t.Kind == ir.GenPrim {
		switch t.Prim {
		case "Int", "Float", "Bool", "Text", "Bytes":
			return "fuzz." + t.Prim + "Lit"
		case "Id":
			return "fuzz.TextLit"
		}
	}
	return "func(x " + GoType(t.Type) + ") string { return " + g.lit(t, "x") + " }"
}

// drawFunc is t's generator as a fuzz.Fuzzer, its recursion budget depth.
func (g *gen) drawFunc(t *ir.GenTree, depth string) string {
	return "func(c *fuzz.Choices) " + GoType(t.Type) + " { return " + g.draw(t, "c", depth) + " }"
}

// fuzzDerive is the value of `Fuzz.any`: its generator as a fuzz.Fuzzer.
func (g *gen) fuzzDerive(x *ir.FuzzDerive) string {
	g.needFuzz(x.Gen)
	return "fuzz.Fuzzer[" + GoType(x.Gen.Type) + "](" + g.drawFunc(x.Gen, "fuzz.Depth") + ")"
}

// fuzzSource is fuzzFile's source, "" when the program needs no derived generator. Emitting
// one function can need no other: needFuzz and needLit noted every part.
func (g *gen) fuzzSource() string {
	fs := g.fuzzes
	if fs == nil || len(fs.fuzzOrder)+len(fs.litOrder) == 0 {
		return ""
	}
	var b strings.Builder
	for _, t := range fs.fuzzOrder {
		g.fuzzFunc(&b, t)
	}
	for _, t := range fs.litOrder {
		g.litFuncDecl(&b, t)
	}
	return b.String()
}

// fuzzFunc writes t's generator.
func (g *gen) fuzzFunc(w *strings.Builder, t *ir.GenTree) {
	typ := GoType(t.Type)
	name := "fuzz__" + g.fuzzName(t)
	fmt.Fprintf(w, "// %s draws a %s.\nfunc %s(c *fuzz.Choices, depth int) %s {\n", name, t.Key, name, typ)
	// A List, Maybe or Dict on a cycle (a record that holds a list of itself) is empty at depth
	// 0, holds at most depth elements, and draws them with one less depth.
	spend := func(empty string) string {
		if !t.Cyclic {
			return "depth"
		}
		fmt.Fprintf(w, "if depth <= 0 {\nreturn %s\n}\n", empty)
		return "depth - 1"
	}
	switch t.Kind {
	case ir.GenList:
		if d := spend(typ + "{}"); t.Cyclic {
			fmt.Fprintf(w, "return fuzz.ListOf(struct{ Min, Max int64 }{0, int64(depth)}, %s)(c)\n", g.drawFunc(t.Elems[0], d))
		} else {
			fmt.Fprintf(w, "return fuzz.List(%s)(c)\n", g.drawFunc(t.Elems[0], d))
		}
	case ir.GenMaybe:
		d := spend("rt.Nothing[" + GoType(t.Elems[0].Type) + "]()")
		fmt.Fprintf(w, "return fuzz.Maybe(%s)(c)\n", g.drawFunc(t.Elems[0], d))
	case ir.GenResult:
		fmt.Fprintf(w, "return fuzz.Result(%s, %s)(c)\n", g.drawFunc(t.Elems[0], "depth"), g.drawFunc(t.Elems[1], "depth"))
	case ir.GenDict:
		if d := spend(typ + "{}"); t.Cyclic {
			fmt.Fprintf(w, "return fuzz.DictOf(int64(depth), %s, %s)(c)\n", g.drawFunc(t.Elems[0], d), g.drawFunc(t.Elems[1], d))
		} else {
			fmt.Fprintf(w, "return fuzz.Dict(%s, %s)(c)\n", g.drawFunc(t.Elems[0], d), g.drawFunc(t.Elems[1], d))
		}
	case ir.GenRecord:
		var fields []string
		for i, n := range t.Names {
			v := g.draw(t.Elems[i], "c", "depth")
			if g.boxed.of(t.Type, n) {
				v = "new(" + v + ")"
			}
			fields = append(fields, Field(n)+": "+v)
		}
		fmt.Fprintf(w, "return %s{%s}\n", typ, strings.Join(fields, ", "))
	case ir.GenUnion:
		g.unionDraw(w, t)
	case ir.GenBrand:
		if !t.Check {
			fmt.Fprintf(w, "return %s\n", g.draw(t.Elems[0], "c", "depth"))
			break
		}
		fmt.Fprintf(w, "v := %s\nif !%s(v) {\npanic(fuzz.Reject{What: %s})\n}\nreturn v\n",
			g.draw(t.Elems[0], "c", "depth"), checkName(ir.BrandModule(t.Brand), ir.BrandName(t.Brand)), strconv.Quote(ir.BrandName(t.Brand)))
	default:
		panic(fmt.Sprintf("gen: no generator function for %s", t.Key))
	}
	w.WriteString("}\n\n")
}

// unionDraw writes the body of union t's generator: the constructor drawn by fuzz.Ctor, its
// payload drawn with one less depth when the constructor is recursive.
func (g *gen) unionDraw(w *strings.Builder, t *ir.GenTree) {
	u := t.Union
	var rec []string
	for _, r := range t.Rec {
		rec = append(rec, strconv.FormatBool(r))
	}
	fmt.Fprintf(w, "switch fuzz.Ctor(c, []bool{%s}, depth) {\n", strings.Join(rec, ", "))
	ta := ""
	if ut, ok := t.Type.(*types.Union); ok && len(u.Params) > 0 {
		ta = typeArgs(ut.Args)
	}
	for i, name := range t.Names {
		fmt.Fprintf(w, "case %d:\n", i)
		ct := ctorT(u, name)
		switch {
		case ct.Payload == nil && ta == "":
			fmt.Fprintf(w, "return %s\n", ctorName(u.Module, u.Name, name))
		case ct.Payload == nil:
			fmt.Fprintf(w, "return %s%s{Tag: %s}\n", GoName(u.Module, u.Name), ta, tagConst(u.Module, u.Name, name))
		default:
			depth := "depth"
			if t.Rec[i] {
				depth = "depth - 1"
			}
			fmt.Fprintf(w, "return %s%s(%s)\n", ctorName(u.Module, u.Name, name), ta, g.draw(t.Elems[i], "c", depth))
		}
	}
	w.WriteString("}\npanic(\"unreachable\")\n")
}

// litFuncDecl writes t's printer.
func (g *gen) litFuncDecl(w *strings.Builder, t *ir.GenTree) {
	name := "lit__" + g.fuzzName(t)
	fmt.Fprintf(w, "// %s prints a %s as an Alder literal, its constructors and brands named through q.\nfunc %s(q fuzz.Quals, v %s) string {\n", name, t.Key, name, GoType(t.Type))
	switch t.Kind {
	case ir.GenList:
		fmt.Fprintf(w, "return fuzz.ListLit(v, %s)\n", g.litFunc(t.Elems[0]))
	case ir.GenMaybe:
		fmt.Fprintf(w, "return fuzz.MaybeLit(v, %s)\n", g.litFunc(t.Elems[0]))
	case ir.GenResult:
		fmt.Fprintf(w, "return fuzz.ResultLit(v, %s, %s)\n", g.litFunc(t.Elems[0]), g.litFunc(t.Elems[1]))
	case ir.GenDict:
		fmt.Fprintf(w, "return fuzz.DictLit(v, %s, %s)\n", g.litFunc(t.Elems[0]), g.litFunc(t.Elems[1]))
	case ir.GenRecord:
		if len(t.Names) == 0 {
			w.WriteString("return \"{}\"\n")
			break
		}
		var names, vals []string
		for i, n := range t.Names {
			v := "v." + Field(n)
			if g.boxed.of(t.Type, n) {
				v = "*" + v
			}
			names = append(names, strconv.Quote(n))
			vals = append(vals, g.lit(t.Elems[i], v))
		}
		fmt.Fprintf(w, "return fuzz.RecordLit([]string{%s}, []string{\n%s,\n})\n", strings.Join(names, ", "), strings.Join(vals, ",\n"))
	case ir.GenUnion:
		u := t.Union
		w.WriteString("switch v.Tag {\n")
		for i, name := range t.Names {
			fmt.Fprintf(w, "case %s:\n", tagConst(u.Module, u.Name, name))
			ct := ctorT(u, name)
			qn := "q.Name(" + strconv.Quote(u.Module) + ", " + strconv.Quote(name) + ")"
			switch {
			case ct.Payload == nil:
				fmt.Fprintf(w, "return %s\n", qn)
			case wrapped(ct.Payload):
				fmt.Fprintf(w, "return fuzz.Apply(%s, %s)\n", qn, g.lit(t.Elems[i], "v."+ctorField(name)+".Value"))
			default:
				fmt.Fprintf(w, "return fuzz.Apply(%s, %s)\n", qn, g.lit(t.Elems[i], "*v."+ctorField(name)))
			}
		}
		w.WriteString("}\npanic(\"unreachable\")\n")
	case ir.GenBrand:
		fmt.Fprintf(w, "return fuzz.Apply(q.Name(%s, %s), %s)\n", strconv.Quote(ir.BrandModule(t.Brand)), strconv.Quote(ir.BrandName(t.Brand)), g.lit(t.Elems[0], "v"))
	default:
		panic(fmt.Sprintf("gen: no printer function for %s", t.Key))
	}
	w.WriteString("}\n\n")
}

// qualsDecl declares q, the fuzz.Quals the printers of trees name constructors and brands
// through (p.Quals: as p's module writes them), when one of them has a lit__ function to pass
// it to; "" otherwise.
func qualsDecl(p *ir.TestProp, trees ...*ir.GenTree) string {
	if !slices.ContainsFunc(trees, hasFunc) {
		return ""
	}
	var kv []string
	for _, m := range slices.Sorted(maps.Keys(p.Quals)) {
		kv = append(kv, strconv.Quote(m)+": "+strconv.Quote(p.Quals[m]))
	}
	return "q := fuzz.Quals{" + strings.Join(kv, ", ") + "}\n"
}

// pasteFunc is a Property.Case filling the template tmpl (ir.TestProp.Case) with the literals.
func pasteFunc(tmpl string) string {
	return "func(lits []string) string {\nreturn rtesting.Paste(" + strconv.Quote(tmpl) + ", lits)\n}"
}

// propTest emits property test t's function: the rtesting.Property it runs.
//
//	func test__Todo__1() *rtesting.Property {
//		return &rtesting.Property{
//			Args: []rtesting.Arg{{Name: "xs", Type: "List Int", Defaulted: true}},
//			Draw: func(c *fuzz.Choices) []any { return []any{fuzz__List_Int(c, fuzz.Depth)} },
//			Lit:  func(vs []any) []string { q := fuzz.Quals{"Todo": ""}; return []string{lit__List_Int(q, vs[0].([]int64))} },
//			Case: func(lits []string) string { return rtesting.Paste("test \"reverse twice (case)\" = …", lits) },
//			Pure: func(vs []any) bool { xs := vs[0].([]int64); return … },
//		}
//	}
//
// A Fuzz.forAll body is rtesting.ForAll (or ForAllTask) of its generator and function, with
// Args and Lit set after.
func (g *gen) propTest(w *strings.Builder, t *ir.Test, p *ir.TestProp, n int) {
	f := &fn{g: g}
	s := newScope()
	var args []string
	for i, name := range p.Params {
		a := "{Name: " + strconv.Quote(name) + ", Type: " + strconv.Quote(p.Types[i])
		if p.Defaulted[i] {
			a += ", Defaulted: true"
		}
		args = append(args, a+"}")
	}
	argList := "[]rtesting.Arg{" + strings.Join(args, ", ") + "}"
	var b strings.Builder
	if p.ForAllGen != nil {
		ctor := "rtesting.ForAll"
		if t.Effect {
			ctor = "rtesting.ForAllTask"
		}
		v := f.temp(s, "_p")
		fmt.Fprintf(&b, "%s := %s(%s, %s)\n%s.Args = %s\n", v, ctor, f.code(p.ForAllGen, s), f.code(p.Fn, s), v, argList)
		lit, quals := "rt.Show(vs[0])", ""
		if p.ForAllLit != nil {
			g.needLit(p.ForAllLit)
			lit = g.lit(p.ForAllLit, "vs[0].("+GoType(p.ForAllLit.Type)+")")
			quals = qualsDecl(p, p.ForAllLit)
		}
		fmt.Fprintf(&b, "%s.Lit = func(vs []any) []string {\n%sreturn []string{%s}\n}\n", v, quals, lit)
		if p.Case != "" {
			fmt.Fprintf(&b, "%s.Case = %s\n", v, pasteFunc(p.Case))
		}
		fmt.Fprintf(&b, "return %s\n", v)
	} else {
		var draws, lits []string
		for i, gt := range p.Gens {
			g.needFuzz(gt)
			g.needLit(gt)
			draws = append(draws, g.draw(gt, "c", "fuzz.Depth"))
			lits = append(lits, g.lit(gt, fmt.Sprintf("vs[%d].(%s)", i, GoType(gt.Type))))
		}
		lam := p.Fn.(*ir.Lambda)
		inner := s.child()
		vs := f.temp(inner, "_vs")
		var body strings.Builder
		for i, prm := range lam.Params {
			if f.uses(lam.Body, prm.Name) > 0 {
				fmt.Fprintf(&body, "%s := %s[%d].(%s)\n", inner.declare(prm.Name), vs, i, GoType(prm.Type))
			}
		}
		kind, result := "Pure", "bool"
		if t.Effect {
			kind, result = "Task", "eff.Task[bool]"
			f.retK(&body, lam.Body, inner, f.task)
		} else {
			f.ret(&body, lam.Body, inner)
		}
		paste := ""
		if p.Case != "" {
			paste = "Case: " + pasteFunc(p.Case) + ",\n"
		}
		fmt.Fprintf(&b, "return &rtesting.Property{\nArgs: %s,\nDraw: func(c *fuzz.Choices) []any {\nreturn []any{%s}\n},\n"+
			"Lit: func(vs []any) []string {\n%sreturn []string{%s}\n},\n%s: func(%s []any) %s {\n%s},\n%s}\n",
			argList, strings.Join(draws, ", "), qualsDecl(p, p.Gens...), strings.Join(lits, ", "), kind, vs, result, body.String(), paste)
	}
	fmt.Fprintf(w, "// %s is property test %s of module %s.\nfunc %s() *rtesting.Property {\n%s}\n\n", testFunc(t.Module, n), strconv.Quote(t.Name), t.Module,
		testFunc(t.Module, n), b.String())
}
