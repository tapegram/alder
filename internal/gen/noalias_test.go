package gen

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestEmittedCodeWritesNoList is gen's half of the audit behind the runtime's shared storage
// (#37, see rt's TestNoWritesToParameters): List.take/drop/tail are views and File.text aliases
// the Text's bytes, which is sound only while generated code never writes into a slice it did
// not just make. Alder has no assignment, so gen emits no index store from user code; this test
// fails when a Go string literal in gen (the code gen emits) starts to contain an in-place slice
// operation that is not one of the known fresh-local uses below.
func TestEmittedCodeWritesNoList(t *testing.T) {
	// Each entry is a substring of an emitted literal (or, quoted, the whole literal) whose
	// append grows a slice the emitted function itself just made.
	allowed := map[string]bool{
		"absent = append(absent, ":             true, // browser.go: the form encoder's local `var absent []string`
		"errs = append(errs, ":                 true, // routes.go: the form decoder's local error list
		"out[e.Field] = append(out[e.Field], ": true, // routes.go: route__errors' local map
		"missing = append(missing, ":           true, // caps.go: main's local list of unwired capabilities
		"w.Migrations = append(w.Migrations, ": true, // caps.go: a Go-side migration report
		`" = append("`:                         true, // routes.go: a list field decoded into `target = T{}` first
		`"append("`:                            true, // routes.go: joining html.Choices results, each a fresh slice
	}
	ops := []string{"append(", "copy(", "clear(", "slices.Sort", "slices.Reverse", "sort.Slice"}
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range files {
		if strings.HasSuffix(path, "_test.go") {
			continue
		}
		src, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		fset := token.NewFileSet()
		f, err := parser.ParseFile(fset, path, src, 0)
		if err != nil {
			t.Fatal(err)
		}
		ast.Inspect(f, func(n ast.Node) bool {
			lit, ok := n.(*ast.BasicLit)
			if !ok || lit.Kind != token.STRING {
				return true
			}
			for _, op := range ops {
				if strings.Contains(lit.Value, op) && !allowedLit(lit.Value, allowed) {
					t.Errorf("%s: emitted code contains %q: %s", fset.Position(lit.Pos()), op, lit.Value)
				}
			}
			return true
		})
	}
}

func allowedLit(v string, allowed map[string]bool) bool {
	for a := range allowed {
		if v == a || (!strings.HasPrefix(a, `"`) && strings.Contains(v, a)) {
			return true
		}
	}
	return false
}
