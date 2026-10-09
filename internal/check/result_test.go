package check

import (
	"testing"

	"alder/internal/diag"
	"alder/internal/syntax"
	"alder/internal/types"
)

func TestResult_RecordsTypes(t *testing.T) {
	res, bag := runDirResult(t, "../../examples/todos")
	if bag.HasErrors() {
		t.Fatal(render(bag))
	}
	if res.Values["Todo.countOpen"] == nil || res.Decls["Todo.Todo"] == nil || res.Tables["Todo.Todo"] == nil {
		t.Fatalf("declarations missing from Result")
	}
	// every recorded type is fully resolved: no unbound variable anywhere inside it
	n := 0
	for e, ty := range res.Types {
		n++
		if hasVar(res.U.Apply(ty), map[*types.Record]bool{}) {
			t.Errorf("unresolved type %s for %T at %v", types.String(res.U.Apply(ty)), e, e.GetSpan().Start)
		}
	}
	if n < 100 {
		t.Fatalf("too few recorded expressions: %d", n)
	}
	if len(res.Binds) == 0 {
		t.Fatal("no bindings recorded")
	}
	for site, ty := range res.Binds {
		if hasVar(res.U.Apply(ty), map[*types.Record]bool{}) {
			t.Errorf("unresolved type %s for binding %T", types.String(res.U.Apply(ty)), site)
		}
	}
	if res.Pages["Todos.Index"] == nil || res.Session == nil || res.AppMsg == nil {
		t.Fatal("page/app info missing")
	}
}

// TestResult_Complete walks every body of each program and requires a recorded type for every
// expression, a recorded binding for every site that introduces a variable, and the documented
// shape for the application on the right of `|>`. testdata/result_pipe has partial and widening pipes.
func TestResult_Complete(t *testing.T) {
	for _, tc := range []struct {
		dir                          string
		minExprs, minBinds, minPipes int
	}{
		{"../../examples/todos", 100, 1, 1},
		{"testdata/result_pipe", 10, 1, 2},
	} {
		t.Run(tc.dir, func(t *testing.T) {
			prog, res, bag := runDirProg(t, tc.dir)
			if bag.HasErrors() {
				t.Fatal(render(bag))
			}
			cv := &coverage{t: t, res: res}
			for _, name := range prog.Order {
				for _, d := range prog.Modules[name].Decls {
					cv.decl(d)
				}
			}
			if cv.exprs < tc.minExprs || cv.binds < tc.minBinds || cv.pipes < tc.minPipes {
				t.Fatalf("walk saw too little: %d expressions, %d bindings, %d pipes into an application", cv.exprs, cv.binds, cv.pipes)
			}
		})
	}
}

// hasVar reports whether t contains an unbound type variable at any depth.
func hasVar(t types.Type, seen map[*types.Record]bool) bool {
	switch t := t.(type) {
	case *types.Var:
		return true
	case *types.Brand:
		return hasVar(t.Base, seen)
	case *types.Record:
		if seen[t] {
			return false
		}
		seen[t] = true
		for _, f := range t.Fields {
			if hasVar(f.Type, seen) {
				return true
			}
		}
	case *types.Union:
		for _, a := range t.Args {
			if hasVar(a, seen) {
				return true
			}
		}
	case *types.App:
		for _, a := range t.Args {
			if hasVar(a, seen) {
				return true
			}
		}
	case *types.Fun:
		for _, p := range t.Params {
			if hasVar(p, seen) {
				return true
			}
		}
		return hasVar(t.Result, seen)
	}
	return false
}

type coverage struct {
	t                   *testing.T
	res                 *Result
	exprs, binds, pipes int
}

func (cv *coverage) decl(d syntax.Decl) {
	switch d := d.(type) {
	case *syntax.ValueDecl:
		if d.Body == nil {
			return
		}
		for _, p := range d.Params {
			cv.bound(p)
		}
		cv.expr(d.Body)
	case *syntax.TestDecl:
		cv.expr(d.Body)
	case *syntax.TypeDecl:
		if d.Brand != nil && d.Brand.Check != nil {
			cv.expr(d.Brand.Check)
		}
		if d.Record != nil {
			cv.fields(d.Record.Fields)
		}
	case *syntax.TableDecl:
		cv.fields(d.Fields)
	case *syntax.FormDecl:
		cv.fields(d.Fields)
	case *syntax.SessionDecl:
		cv.fields(d.Fields)
	case *syntax.CapDecl:
		cv.fields(d.Entries)
	case *syntax.MsgDecl:
		if d.Params != nil {
			cv.fields(d.Params.Fields)
		}
	case *syntax.PageDecl:
		if d.Params != nil {
			cv.fields(d.Params.Fields)
		}
	}
}

func (cv *coverage) fields(fs []*syntax.Field) {
	for _, f := range fs {
		if f.Default != nil {
			cv.expr(f.Default)
		}
	}
}

func (cv *coverage) bound(site interface{ GetSpan() diag.Span }) {
	cv.binds++
	if _, ok := cv.res.Binds[site]; !ok {
		cv.t.Errorf("no binding recorded for %T at %v", site, site.GetSpan())
	}
}

func (cv *coverage) expr(e syntax.Expr) {
	if e == nil {
		return
	}
	cv.exprs++
	if _, ok := cv.res.Types[e]; !ok {
		cv.t.Errorf("no type recorded for %T at %v", e, e.GetSpan())
	}
	switch x := e.(type) {
	case *syntax.IntLit, *syntax.FloatLit, *syntax.BoolLit, *syntax.UnitLit, *syntax.Var, *syntax.CtorRef, *syntax.HoleExpr, *syntax.HtmlText:
	case *syntax.StrLit:
		for _, p := range x.Parts {
			cv.expr(p.Expr)
		}
	case *syntax.ListLit:
		for _, el := range x.Elems {
			cv.expr(el)
		}
	case *syntax.RecordLit:
		for _, f := range x.Fields {
			cv.expr(f.Value)
		}
	case *syntax.RecordUpdate:
		cv.expr(x.Base)
		for _, f := range x.Fields {
			cv.expr(f.Value)
		}
	case *syntax.Access:
		cv.expr(x.Expr)
	case *syntax.App:
		cv.expr(x.Fn)
		for _, a := range x.Args {
			cv.expr(a)
		}
	case *syntax.Lambda:
		for _, p := range x.Params {
			cv.bound(p)
		}
		cv.expr(x.Body)
	case *syntax.If:
		cv.expr(x.Cond)
		cv.expr(x.Then)
		cv.expr(x.Else)
	case *syntax.Match:
		cv.expr(x.Subject)
		cv.arms(x.Arms)
	case *syntax.Block:
		for _, s := range x.Stmts {
			cv.expr(s.Expr)
			cv.bound(s)
		}
		cv.expr(x.Result)
	case *syntax.BinOp:
		cv.expr(x.L)
		cv.expr(x.R)
		if app, ok := x.R.(*syntax.App); ok && x.Op == "|>" {
			cv.pipe(x, app)
		}
	case *syntax.Element:
		for _, a := range x.Attrs {
			cv.expr(a.Value)
		}
		for _, ch := range x.Children {
			cv.expr(ch)
		}
	case *syntax.HtmlExpr:
		cv.expr(x.Expr)
	case *syntax.HtmlFor:
		cv.expr(x.List)
		cv.bound(x)
		cv.expr(x.Body)
	case *syntax.HtmlIf:
		cv.expr(x.Cond)
		cv.expr(x.Then)
		cv.expr(x.Else)
	case *syntax.HtmlMatch:
		cv.expr(x.Subject)
		cv.arms(x.Arms)
	default:
		cv.t.Errorf("coverage walk does not know expression %T at %v", e, e.GetSpan())
	}
}

// pipe checks that `l |> f a…` records the application as the callee's remaining parameters
// (the piped one first) to the callee's own result.
func (cv *coverage) pipe(x *syntax.BinOp, app *syntax.App) {
	cv.pipes++
	callee, ok1 := cv.res.Types[app.Fn].(*types.Fun)
	rhs, ok2 := cv.res.Types[app].(*types.Fun)
	if !ok1 || !ok2 {
		cv.t.Errorf("pipe at %v: callee %T, application %T; want functions", x.GetSpan(), cv.res.Types[app.Fn], cv.res.Types[app])
		return
	}
	want := &types.Fun{Params: callee.Params[len(app.Args):], Result: callee.Result}
	same := len(rhs.Params) == len(want.Params) && types.String(rhs.Result) == types.String(want.Result) // String alone prints curried and multi-param alike
	for i := range rhs.Params {
		same = same && i < len(want.Params) && types.String(rhs.Params[i]) == types.String(want.Params[i])
	}
	if !same {
		cv.t.Errorf("pipe at %v: application recorded as %s (%d params), want %s (%d params)", x.GetSpan(), types.String(rhs), len(rhs.Params), types.String(want), len(want.Params))
	}
}

func (cv *coverage) arms(as []*syntax.Arm) {
	for _, a := range as {
		cv.pat(a.Pat)
		cv.expr(a.Body)
	}
}

func (cv *coverage) pat(p syntax.Pattern) {
	switch p := p.(type) {
	case nil, *syntax.PWild:
	case *syntax.PVar:
		cv.bound(p)
	case *syntax.PLit:
		cv.expr(p.Lit)
	case *syntax.PCtor:
		cv.pat(p.Arg)
	case *syntax.PRecord:
		for _, f := range p.Fields {
			cv.pat(f.Pat)
		}
	case *syntax.PList:
		for _, e := range p.Elems {
			cv.pat(e)
		}
		if p.Rest != "" {
			cv.bound(p)
		}
	default:
		cv.t.Errorf("coverage walk does not know pattern %T at %v", p, p.GetSpan())
	}
}
