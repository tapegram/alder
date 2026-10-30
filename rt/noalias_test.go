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
		// FloatRound's helper: its only caller passes []byte(whole + frac), a fresh conversion.
		"std/num.go: incDecimal index d": "called only on a fresh []byte",
	}
	var hits []string
	// The packages whose functions take and return Alder values. rt/db, rt/eff, rt/testing and
	// the rest handle Go-side state (rows, effect logs, browsers) that never reaches Alder code
	// as a List or Bytes without passing through one of these.
	dirs := map[string]bool{".": true, "std": true, "html": true}
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

// paramWrites finds writes through fd's parameters and receiver, and through the parameters of
// the function literals inside it.
func paramWrites(fd *ast.FuncDecl) []paramWrite {
	params := map[string]bool{}
	addFields := func(fl *ast.FieldList) {
		if fl == nil {
			return
		}
		for _, f := range fl.List {
			for _, n := range f.Names {
				if n.Name != "_" {
					params[n.Name] = true
				}
			}
		}
	}
	addFields(fd.Recv)
	addFields(fd.Type.Params)
	ast.Inspect(fd.Body, func(n ast.Node) bool {
		if fl, ok := n.(*ast.FuncLit); ok {
			addFields(fl.Type.Params)
		}
		return true
	})
	// A local that shadows a parameter (`xs := append([]A{}, xs...)`) is fresh: drop it.
	ast.Inspect(fd.Body, func(n ast.Node) bool {
		if as, ok := n.(*ast.AssignStmt); ok && as.Tok == token.DEFINE {
			for _, l := range as.Lhs {
				if id, ok := l.(*ast.Ident); ok {
					delete(params, id.Name)
				}
			}
		}
		return true
	})
	var out []paramWrite
	isParam := func(e ast.Expr) (string, bool) {
		b := base(e)
		if b == nil || !params[b.Name] {
			return "", false
		}
		return b.Name, true
	}
	ast.Inspect(fd.Body, func(n ast.Node) bool {
		switch x := n.(type) {
		case *ast.AssignStmt:
			for _, l := range x.Lhs {
				if ix, ok := l.(*ast.IndexExpr); ok {
					if p, ok := isParam(ix.X); ok {
						out = append(out, paramWrite{"index " + p, x.Pos()})
					}
				}
			}
		case *ast.IncDecStmt:
			if ix, ok := x.X.(*ast.IndexExpr); ok {
				if p, ok := isParam(ix.X); ok {
					out = append(out, paramWrite{"index " + p, x.Pos()})
				}
			}
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
				if p, ok := isParam(x.Args[0]); ok {
					out = append(out, paramWrite{strings.TrimPrefix(strings.TrimPrefix(fn, "slices."), "sort.") + " " + p, x.Pos()})
				}
			}
		}
		return true
	})
	return out
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
	// f and g append onto a fresh slice, which is fine.
	want := "a index xs|b append xs|c copy xs|d Sort xs|e index r"
	if strings.Join(got, "|") != want {
		t.Fatalf("audit found %q, want %q", strings.Join(got, "|"), want)
	}
}
