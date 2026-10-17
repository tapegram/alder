package names

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// EnvRead is one environment variable an adapter package reads: Name is the string literal given
// to os.Getenv or os.LookupEnv in a non-test Go file of adapters/<Package>/ (or below it).
type EnvRead struct{ Package, Name string }

// ScanAdapterEnv lists, sorted and without repeats, the environment variables the Go packages
// under dir/adapters read through os.Getenv and os.LookupEnv with a literal name (a name built at
// run time cannot be seen). A directory without adapters, or an unparsable file, yields nothing
// for it: the build reports those. `alder deploy` uses the same list to name what the target
// must provide.
func ScanAdapterEnv(dir string) []EnvRead {
	root := filepath.Join(dir, "adapters")
	seen := map[EnvRead]bool{}
	filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		rel, _ := filepath.Rel(root, path)
		pkg := strings.SplitN(filepath.ToSlash(rel), "/", 2)[0]
		if !strings.Contains(filepath.ToSlash(rel), "/") { // adapters/x.go: the flat form belongs to no package
			pkg = "adapters"
		}
		src, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		f, err := parser.ParseFile(token.NewFileSet(), path, src, 0)
		if err != nil {
			return nil
		}
		osName := ""
		for _, im := range f.Imports {
			if p, _ := strconv.Unquote(im.Path.Value); p == "os" {
				osName = "os"
				if im.Name != nil {
					osName = im.Name.Name
				}
			}
		}
		if osName == "" || osName == "_" || osName == "." {
			return nil
		}
		ast.Inspect(f, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok || len(call.Args) != 1 {
				return true
			}
			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok || (sel.Sel.Name != "Getenv" && sel.Sel.Name != "LookupEnv") {
				return true
			}
			if id, ok := sel.X.(*ast.Ident); !ok || id.Name != osName {
				return true
			}
			if lit, ok := call.Args[0].(*ast.BasicLit); ok && lit.Kind == token.STRING {
				if name, err := strconv.Unquote(lit.Value); err == nil && name != "" {
					seen[EnvRead{pkg, name}] = true
				}
			}
			return true
		})
		return nil
	})
	out := make([]EnvRead, 0, len(seen))
	for r := range seen {
		out = append(out, r)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Package != out[j].Package {
			return out[i].Package < out[j].Package
		}
		return out[i].Name < out[j].Name
	})
	return out
}
