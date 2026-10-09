// Package gen emits Go source for a lowered Alder program (internal/ir). Every Alder module
// becomes one file of package app; names follow GoName, types follow GoType, and the code calls
// the runtime in alder/rt, alder/rt/html and alder/rt/std.
package gen

import (
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"slices"
	"strconv"
	"strings"

	"alder/internal/ir"
	"alder/internal/types"
)

// Options configure Emit.
type Options struct {
	RepoPath string // absolute path of the alder repository, for go.mod's replace directive
	Dev      bool
}

// Emit returns the generated files: one "<Module>.go" per module ('.' → '_') and "go.mod".
// Its error is only for output go/format cannot parse, which is a bug in gen.
func Emit(p *ir.Program, opts Options) (map[string][]byte, error) {
	g := newGen(p)
	files := map[string][]byte{"go.mod": []byte(goMod(opts.RepoPath))}
	for _, m := range p.Modules {
		src, err := g.module(m)
		if err != nil {
			return nil, err
		}
		files[fileName(m.Name)] = src
	}
	return files, nil
}

func goMod(repo string) string {
	return "module app\n\ngo 1.27\n\nrequire alder v0.0.0\n\nreplace alder => " + repo + "\n"
}

// gen holds what emission needs across modules.
type gen struct {
	values  map[string]*ir.Value // "Module.name" → value or component
	thunks  map[*ir.Value]bool   // constants emitted as functions (see newGen)
	tparams map[*ir.Value][]string
	stubbed map[string]bool // HTML node kinds stubbed in the current file
}

// newGen indexes the program's values. A constant (a value with no parameters and a non-function
// type) is a Go package variable, except that it is a function called at each use (a thunk) when
// it is generic (Go has no generic variables) or when its initializer refers back to itself
// through other values (Go rejects initialization cycles).
func newGen(p *ir.Program) *gen {
	g := &gen{values: map[string]*ir.Value{}, thunks: map[*ir.Value]bool{}, tparams: map[*ir.Value][]string{}}
	var all []*ir.Value
	for _, m := range p.Modules {
		all = append(append(all, m.Values...), m.Components...)
	}
	refs := map[*ir.Value][]*ir.Value{}
	for _, v := range all {
		g.values[v.Module+"."+v.Name] = v
	}
	for _, v := range all {
		walk(v.Body, func(e ir.Expr) {
			switch x := e.(type) {
			case *ir.Var:
				if r := g.values[x.Module+"."+x.Name]; x.Kind == ir.VarGlobal && r != nil {
					refs[v] = append(refs[v], r)
				}
			case *ir.Comp:
				if x.Fn != nil {
					refs[v] = append(refs[v], x.Fn)
				}
			}
		})
	}
	for _, v := range all {
		if _, isFun := v.Type.(*types.Fun); isFun || len(v.Params) > 0 {
			continue
		}
		if len(g.params(v)) > 0 || reaches(refs, v) {
			g.thunks[v] = true
		}
	}
	return g
}

// reaches reports whether v refers to itself through the reference graph.
func reaches(refs map[*ir.Value][]*ir.Value, v *ir.Value) bool {
	seen := map[*ir.Value]bool{}
	stack := slices.Clone(refs[v])
	for len(stack) > 0 {
		r := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if r == v {
			return true
		}
		if !seen[r] {
			seen[r] = true
			stack = append(stack, refs[r]...)
		}
	}
	return false
}

// params are the Alder type parameters of v's Go declaration.
func (g *gen) params(v *ir.Value) []string {
	ps, ok := g.tparams[v]
	if !ok {
		ps, _ = typeParams(v.Type)
		g.tparams[v] = ps
	}
	return ps
}

// instantiate is the explicit type argument list for a reference to generic value v at type t,
// "" when v is not generic or when some argument is not fully known (Go then infers it).
func (g *gen) instantiate(v *ir.Value, t types.Type) string {
	ps := g.params(v)
	if len(ps) == 0 {
		return ""
	}
	m := map[string]types.Type{}
	bindParams(v.Type, t, m)
	var args []types.Type
	for _, p := range ps {
		a := m[p]
		if a == nil || !solved(a) {
			return ""
		}
		args = append(args, a)
	}
	return typeArgs(args)
}

// bindParams matches declared type decl against its instance inst, recording what each type
// parameter stands for.
func bindParams(decl, inst types.Type, m map[string]types.Type) {
	switch d := decl.(type) {
	case *types.Param:
		if m[d.Name] == nil {
			m[d.Name] = inst
		}
	case *types.App:
		if i, ok := inst.(*types.App); ok && i.Con == d.Con && len(i.Args) == len(d.Args) {
			for k := range d.Args {
				bindParams(d.Args[k], i.Args[k], m)
			}
		}
	case *types.Union:
		if i, ok := inst.(*types.Union); ok && len(i.Args) == len(d.Args) {
			for k := range d.Args {
				bindParams(d.Args[k], i.Args[k], m)
			}
		}
	case *types.Record:
		if i, ok := inst.(*types.Record); ok && len(i.Fields) == len(d.Fields) {
			for k := range d.Fields {
				bindParams(d.Fields[k].Type, i.Fields[k].Type, m)
			}
		}
	case *types.Fun:
		if i, ok := inst.(*types.Fun); ok && len(i.Params) == len(d.Params) {
			for k := range d.Params {
				bindParams(d.Params[k], i.Params[k], m)
			}
			bindParams(d.Result, i.Result, m)
		}
	}
}

// solved reports whether t has no unsolved inference variable.
func solved(t types.Type) bool {
	switch t := t.(type) {
	case *types.Var:
		return false
	case *types.Con:
		return !types.IsErr(t)
	case *types.App:
		return all(t.Args)
	case *types.Union:
		return all(t.Args)
	case *types.Record:
		for _, f := range t.Fields {
			if !solved(f.Type) {
				return false
			}
		}
	case *types.Fun:
		return all(t.Params) && solved(t.Result)
	}
	return true
}

func all(ts []types.Type) bool {
	for _, t := range ts {
		if !solved(t) {
			return false
		}
	}
	return true
}

// module emits one module's file.
func (g *gen) module(m *ir.Module) ([]byte, error) {
	g.stubbed = map[string]bool{}
	var body strings.Builder
	for _, d := range m.Types {
		typeDecl(&body, d)
	}
	for _, v := range m.Values {
		g.value(&body, v)
	}
	for _, v := range m.Components {
		g.value(&body, v)
	}
	var head strings.Builder
	head.WriteString("// Code generated by alder; DO NOT EDIT.\n\npackage app\n\n")
	imports, err := importsOf(body.String())
	if err != nil {
		return nil, fmt.Errorf("gen: module %s: %v\n%s", m.Name, err, body.String())
	}
	if len(imports) == 1 {
		fmt.Fprintf(&head, "import %s\n\n", strconv.Quote(imports[0]))
	} else if len(imports) > 1 {
		head.WriteString("import (\n")
		for _, imp := range imports {
			head.WriteString(strconv.Quote(imp) + "\n")
		}
		head.WriteString(")\n\n")
	}
	if len(g.stubbed) > 0 {
		kinds := slices.Sorted(func(yield func(string) bool) {
			for k := range g.stubbed {
				if !yield(k) {
					return
				}
			}
		})
		fmt.Fprintf(&head, "// TODO: HTML is not emitted yet; these nodes are stubbed as html.None(): %s.\n\n", strings.Join(kinds, ", "))
	}
	src := head.String() + body.String()
	out, err := format.Source([]byte(src))
	if err != nil {
		return nil, fmt.Errorf("gen: module %s: %v\n%s", m.Name, err, src)
	}
	return out, nil
}

// packages are the imports generated code may need, by package name.
var packages = map[string]string{
	"rt": "alder/rt", "html": "alder/rt/html", "std": "alder/rt/std", "time": "time", "math": "math",
}

// importsOf lists the import paths body refers to. Locals never take a package's name (see
// local), so a selector on such a name is a package reference.
func importsOf(body string) ([]string, error) {
	f, err := parser.ParseFile(token.NewFileSet(), "", "package app\n"+body, parser.SkipObjectResolution)
	if err != nil {
		return nil, err
	}
	used := map[string]bool{}
	ast.Inspect(f, func(n ast.Node) bool {
		if sel, ok := n.(*ast.SelectorExpr); ok {
			if id, ok := sel.X.(*ast.Ident); ok && packages[id.Name] != "" {
				used[packages[id.Name]] = true
			}
		}
		return true
	})
	var out []string
	for p := range used {
		out = append(out, p)
	}
	slices.Sort(out) // format.Source would sort them anyway
	return out, nil
}

// value emits a top-level value or component: a function when its type is a function (with
// all the signature's parameters, also those its definition leaves to a function-valued body),
// else a package variable or, for a thunk, a function of no parameters.
func (g *gen) value(w *strings.Builder, v *ir.Value) {
	f := &fn{g: g}
	s := newScope()
	ps, cmp := typeParams(v.Type)
	tps := typeParamDecl(ps, cmp)
	name := GoName(v.Module, v.Name)
	ft, isFun := v.Type.(*types.Fun)
	if !isFun {
		if g.thunks[v] {
			var b strings.Builder
			f.ret(&b, v.Body, s)
			fmt.Fprintf(w, "func %s%s() %s {\n%s}\n\n", name, tps, GoType(v.Type), b.String())
			return
		}
		fmt.Fprintf(w, "var %s %s = %s\n\n", name, GoType(v.Type), f.code(v.Body, s))
		return
	}
	var params []string
	for _, p := range v.Params {
		params = append(params, s.declare(p.Name)+" "+GoType(p.Type))
	}
	body := v.Body
	rest := ft.Params[min(len(v.Params), len(ft.Params)):]
	// a function-valued body: its lambda's parameters become the function's own
	for len(rest) > 0 {
		l, ok := body.(*ir.Lambda)
		if !ok || len(l.Params) > len(rest) {
			break
		}
		for _, p := range l.Params {
			params = append(params, s.declare(p.Name)+" "+GoType(p.Type))
		}
		rest, body = rest[len(l.Params):], l.Body
	}
	if len(rest) > 0 { // otherwise apply the body to the remaining parameters
		var extra []ir.Expr
		for _, t := range rest {
			key, p := f.tempVar(s, "_a")
			params = append(params, p+" "+GoType(t))
			extra = append(extra, &ir.Var{Typed: ir.Typed{Ty: t}, Kind: ir.VarLocal, Name: key})
		}
		if pa, ok := body.(*ir.Partial); ok && pa.Missing == len(extra) {
			body = &ir.Call{Typed: ir.Typed{Ty: ft.Result}, Fn: pa.Fn, Args: append(slices.Clone(pa.Given), extra...)}
		} else {
			body = &ir.Call{Typed: ir.Typed{Ty: ft.Result}, Fn: body, Args: extra}
		}
	}
	var b strings.Builder
	f.ret(&b, body, s)
	fmt.Fprintf(w, "func %s%s(%s) %s {\n%s}\n\n", name, tps, strings.Join(params, ", "), GoType(ft.Result), b.String())
}
