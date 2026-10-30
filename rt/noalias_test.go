package rt

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"strings"
	"testing"
)

// TestNoWritesToParameters is the audit behind every storage-sharing shortcut in the runtime
// (#37): List.take/drop/tail return views of their input, `++` with an empty side returns the
// other side, and File.text/json/html alias the Text's bytes as the File's body. All are sound
// only while no slice (or []byte) that Alder code can see is ever written to. Alder has no
// assignment; this test pins the runtime's half: no function under rt/ writes through one of its
// own parameters or its receiver (an index store, append, copy, clear, or an in-place sort or
// reverse). A function that legitimately does (it only ever receives a value no Alder code has
// seen) is listed below with the reason; a new hit means either add it here with a reason that
// holds, or copy first.
func TestNoWritesToParameters(t *testing.T) {
	allowed := map[string]string{
		// Put is documented for a Dict no Alder code has seen yet (inside a stdlib function).
		"core.go: (*Dict).Put append d": "Put builds a fresh Dict",
		"core.go: (*Dict).Put index d":  "Put builds a fresh Dict",
		// dictEnc's helper fills its caller's stack array, never an Alder value.
		"core.go: putInt index buf": "fills a local [8]byte",
		// Fresh's type walk marks the types it is visiting in its own scratch map.
		"fresh.go: needsFreshIn index visiting": "a local scratch map",
		// FloatRound's helper: its only caller passes []byte(whole + frac), a fresh conversion.
		"std/num.go: incDecimal index d": "called only on a fresh []byte",
	}
	var hits []string
	// The packages whose functions take and return Alder values, and the router that writes a
	// File's body (only ever with Write). rt/db, rt/eff, rt/testing and
	// the rest handle Go-side state (rows, effect logs, browsers) that never reaches Alder code
	// as a List or Bytes without passing through one of these.
	dirs := map[string]bool{".": true, "std": true, "html": true, "web": true}
	err := filepath.WalkDir(".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() && !dirs[path] {
			return filepath.SkipDir
		}
		if d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		fset := token.NewFileSet()
		f, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			return err
		}
		for _, decl := range f.Decls {
			fd, ok := decl.(*ast.FuncDecl)
			if !ok || fd.Body == nil {
				continue
			}
			name := fd.Name.Name
			if fd.Recv != nil {
				name = "(" + recvType(fd.Recv.List[0].Type) + ")." + name
			}
			for _, h := range paramWrites(fd) {
				key := filepath.ToSlash(path) + ": " + name + " " + h.what
				if _, ok := allowed[key]; ok {
					continue
				}
				hits = append(hits, fmt.Sprintf("%s (%s)", key, fset.Position(h.pos)))
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, h := range hits {
		t.Errorf("writes through a parameter: %s", h)
	}
}

func recvType(e ast.Expr) string {
	switch x := e.(type) {
	case *ast.StarExpr:
		return "*" + recvType(x.X)
	case *ast.IndexExpr:
		return recvType(x.X)
	case *ast.IndexListExpr:
		return recvType(x.X)
	case *ast.Ident:
		return x.Name
	}
	return "?"
}

type paramWrite struct {
	what string // "index p", "append p", …
	pos  token.Pos
}

// paramWrites finds writes through fd's parameters and receiver, through the parameters of the
// function literals inside it, and through every local that aliases one of them: `ys := xs`,
// `ys := xs[1:]`, `row := rows[i]`, the value of `for _, row := range rows`, the variable of
// `switch v := p.(type)`. A local is tracked in its own scope, so `xs := fresh()` in an inner
// block shadows the parameter only inside that block; `xs = …` (plain assignment) re-taints or
// clears xs from that point on, in the scope that declared it.
func paramWrites(fd *ast.FuncDecl) []paramWrite {
	a := &auditor{}
	a.push()
	a.declareFields(fd.Recv)
	a.declareFields(fd.Type.Params)
	a.stmt(fd.Body)
	return a.out
}

type auditor struct {
	scopes []map[string]bool // name -> aliases a parameter
	out    []paramWrite
}

func (a *auditor) push() { a.scopes = append(a.scopes, map[string]bool{}) }
func (a *auditor) pop()  { a.scopes = a.scopes[:len(a.scopes)-1] }

func (a *auditor) declare(name string, tainted bool) {
	if name != "_" {
		a.scopes[len(a.scopes)-1][name] = tainted
	}
}

func (a *auditor) declareFields(fl *ast.FieldList) {
	if fl == nil {
		return
	}
	for _, f := range fl.List {
		for _, n := range f.Names {
			a.declare(n.Name, true)
		}
	}
}

// set re-binds an existing variable (plain `=`) in the scope that declared it.
func (a *auditor) set(name string, tainted bool) {
	for i := len(a.scopes) - 1; i >= 0; i-- {
		if _, ok := a.scopes[i][name]; ok {
			a.scopes[i][name] = tainted
			return
		}
	}
}

// tainted reports whether e reaches a parameter's storage: its base variable aliases one.
func (a *auditor) tainted(e ast.Expr) (string, bool) {
	b := base(e)
	if b == nil {
		return "", false
	}
	for i := len(a.scopes) - 1; i >= 0; i-- {
		if t, ok := a.scopes[i][b.Name]; ok {
			return b.Name, t
		}
	}
	return "", false
}

func (a *auditor) isTainted(e ast.Expr) bool { _, t := a.tainted(e); return t }

func (a *auditor) hit(what, p string, pos token.Pos) {
	a.out = append(a.out, paramWrite{what + " " + p, pos})
}

func (a *auditor) checkIndexStore(l ast.Expr, pos token.Pos) {
	if ix, ok := l.(*ast.IndexExpr); ok {
		if p, ok := a.tainted(ix.X); ok {
			a.hit("index", p, pos)
		}
	}
}

func (a *auditor) stmts(list []ast.Stmt) {
	for _, s := range list {
		a.stmt(s)
	}
}

func (a *auditor) stmt(s ast.Stmt) {
	switch x := s.(type) {
	case nil:
	case *ast.BlockStmt:
		if x == nil {
			return
		}
		a.push()
		a.stmts(x.List)
		a.pop()
	case *ast.IfStmt:
		a.push()
		a.stmt(x.Init)
		a.expr(x.Cond)
		a.stmt(x.Body)
		a.stmt(x.Else)
		a.pop()
	case *ast.ForStmt:
		a.push()
		a.stmt(x.Init)
		a.expr(x.Cond)
		a.stmt(x.Post)
		a.stmt(x.Body)
		a.pop()
	case *ast.RangeStmt:
		a.expr(x.X)
		a.push()
		t := a.isTainted(x.X)
		if id, ok := x.Value.(*ast.Ident); ok {
			if x.Tok == token.DEFINE {
				a.declare(id.Name, t)
			} else {
				a.set(id.Name, t)
			}
		}
		if id, ok := x.Key.(*ast.Ident); ok && x.Tok == token.DEFINE {
			a.declare(id.Name, false)
		}
		a.stmt(x.Body)
		a.pop()
	case *ast.SwitchStmt:
		a.push()
		a.stmt(x.Init)
		a.expr(x.Tag)
		a.stmt(x.Body)
		a.pop()
	case *ast.TypeSwitchStmt:
		a.push()
		a.stmt(x.Init)
		name, t := "", false
		switch as := x.Assign.(type) {
		case *ast.AssignStmt: // v := e.(type)
			if ta, ok := as.Rhs[0].(*ast.TypeAssertExpr); ok {
				a.expr(ta.X)
				t = a.isTainted(ta.X)
			}
			name = as.Lhs[0].(*ast.Ident).Name
		case *ast.ExprStmt:
			a.expr(as.X)
		}
		for _, c := range x.Body.List {
			cc := c.(*ast.CaseClause)
			a.push()
			if name != "" {
				a.declare(name, t)
			}
			a.stmts(cc.Body)
			a.pop()
		}
		a.pop()
	case *ast.CaseClause:
		a.push()
		for _, e := range x.List {
			a.expr(e)
		}
		a.stmts(x.Body)
		a.pop()
	case *ast.CommClause:
		a.push()
		a.stmt(x.Comm)
		a.stmts(x.Body)
		a.pop()
	case *ast.SelectStmt:
		a.stmt(x.Body)
	case *ast.LabeledStmt:
		a.stmt(x.Stmt)
	case *ast.AssignStmt:
		for _, e := range x.Rhs {
			a.expr(e)
		}
		for _, l := range x.Lhs {
			a.checkIndexStore(l, x.Pos())
			if _, ok := l.(*ast.Ident); !ok {
				a.expr(l)
			}
		}
		for i, l := range x.Lhs {
			id, ok := l.(*ast.Ident)
			if !ok {
				continue
			}
			t := len(x.Rhs) == len(x.Lhs) && a.isTainted(x.Rhs[i])
			switch x.Tok {
			case token.DEFINE:
				a.declare(id.Name, t)
			case token.ASSIGN:
				a.set(id.Name, t)
			}
		}
	case *ast.DeclStmt:
		gd, ok := x.Decl.(*ast.GenDecl)
		if !ok {
			return
		}
		for _, sp := range gd.Specs {
			vs, ok := sp.(*ast.ValueSpec)
			if !ok {
				continue
			}
			for _, v := range vs.Values {
				a.expr(v)
			}
			for i, n := range vs.Names {
				a.declare(n.Name, len(vs.Values) == len(vs.Names) && a.isTainted(vs.Values[i]))
			}
		}
	case *ast.IncDecStmt:
		a.checkIndexStore(x.X, x.Pos())
	case *ast.ExprStmt:
		a.expr(x.X)
	case *ast.ReturnStmt:
		for _, e := range x.Results {
			a.expr(e)
		}
	case *ast.GoStmt:
		a.expr(x.Call)
	case *ast.DeferStmt:
		a.expr(x.Call)
	case *ast.SendStmt:
		a.expr(x.Chan)
		a.expr(x.Value)
	}
}

// expr checks the calls inside e, and walks the bodies of its function literals in a new scope
// (their parameters count as parameters; the enclosing locals stay visible).
func (a *auditor) expr(e ast.Expr) {
	if e == nil {
		return
	}
	ast.Inspect(e, func(n ast.Node) bool {
		switch x := n.(type) {
		case *ast.FuncLit:
			a.push()
			a.declareFields(x.Type.Params)
			a.stmt(x.Body)
			a.pop()
			return false
		case *ast.CallExpr:
			if len(x.Args) == 0 {
				return true
			}
			var fn string
			switch f := x.Fun.(type) {
			case *ast.Ident:
				fn = f.Name
			case *ast.SelectorExpr:
				if pkg, ok := f.X.(*ast.Ident); ok {
					fn = pkg.Name + "." + f.Sel.Name
				}
			}
			switch fn {
			case "append", "copy", "clear", "slices.Sort", "slices.SortFunc", "slices.SortStableFunc",
				"slices.Reverse", "sort.Slice", "sort.SliceStable", "sort.Strings", "sort.Ints":
				if p, ok := a.tainted(x.Args[0]); ok {
					a.hit(strings.TrimPrefix(strings.TrimPrefix(fn, "slices."), "sort."), p, x.Pos())
				}
			}
		}
		return true
	})
}

// base is the variable an expression writes through: xs for xs, xs[i:j], xs.f, (*xs), p.f[i].
func base(e ast.Expr) *ast.Ident {
	for {
		switch x := e.(type) {
		case *ast.Ident:
			return x
		case *ast.SliceExpr:
			e = x.X
		case *ast.IndexExpr:
			e = x.X
		case *ast.SelectorExpr:
			e = x.X
		case *ast.StarExpr:
			e = x.X
		case *ast.ParenExpr:
			e = x.X
		default:
			return nil
		}
	}
}

// The audit itself must catch a write: a fixture with one of each kind.
func TestNoWritesToParameters_Catches(t *testing.T) {
	src := `package p
func a(xs []int) { xs[0] = 1 }
func b(xs []int) []int { return append(xs[:1], 2) }
func c(xs []int) { copy(xs, []int{1}) }
func d(xs []string) { slices.Sort(xs) }
func e(r struct{ B []byte }) { r.B[0]++ }
func f(xs []int) []int { xs = append([]int{}, xs...); return xs }
func g(xs []int) []int { ys := append([]int{}, xs...); ys[0] = 1; return ys }
func h(xs []int) { ys := xs; ys[0] = 1 }
func i(xs []int) { var ys = xs[1:]; ys[0] = 1 }
func j(rows [][]int) { for _, row := range rows { row[0] = 1 } }
func k(rows [][]int) { row := rows[0]; slices.Reverse(row) }
func l(xs []int) { if true { xs := []int{1}; xs[0] = 2 }; xs[0] = 3 }
func m(xs []int) { ys := []int{}; ys = xs; ys[0] = 1 }
func n(xs []int) { ys := xs; ys = []int{0}; ys[0] = 1 }
func o(v any) { switch x := v.(type) { case []int: x[0] = 1 } }
func p(xs []int) { func() { ys := xs; ys[0] = 1 }() }
`
	f, err := parser.ParseFile(token.NewFileSet(), "x.go", src, 0)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, d := range f.Decls {
		fd := d.(*ast.FuncDecl)
		for _, h := range paramWrites(fd) {
			got = append(got, fd.Name.Name+" "+h.what)
		}
	}
	// f and g append onto a fresh slice, and n writes a fresh slice, which is fine; l's inner xs
	// shadows the parameter only inside its block, so only the outer write is reported.
	want := "a index xs|b append xs|c copy xs|d Sort xs|e index r|h index ys|i index ys|j index row|" +
		"k Reverse row|l index xs|m index ys|o index x|p index ys"
	if strings.Join(got, "|") != want {
		t.Fatalf("audit found %q, want %q", strings.Join(got, "|"), want)
	}
}
