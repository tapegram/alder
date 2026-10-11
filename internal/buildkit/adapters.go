package buildkit

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/build"
	"go/parser"
	"go/token"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"alder/internal/gen"
	"alder/internal/ir"
)

// Adapters. A capability's Go implementation lives in the package named after its module's
// first segment: capability X of module A.B is returned by `func BX() app.Cap__A__B__X` of
// package adapters/a/ (gen.AdapterPkg, gen.AdapterFunc). The generated package caps
// (gen.CapsPackage) imports every such package and calls every constructor; packages under
// adapters/ that match no module (helpers, adapters/internal/…) are copied and compiled but
// never wired. Before emitting, alder build (and the test, eval and REPL builds) check with
// CheckAdapters that every needed package and constructor exists, so a missing one is named
// exactly instead of surfacing as a Go error in the generated caps package.

// FlatAdaptersMessage is printed when a top-level adapters/*.go declares func Caps and
// adapters are missing (listed after it): the flat form (one package adapters whose Caps
// returns every capability) is no longer wired. When none is missing, flatStaleMessage is
// printed instead.
const FlatAdaptersMessage = "adapters/ no longer wires capabilities: move each adapter to adapters/<module>/ (see the list below)"

// flatStaleMessage is the flat-form message when no adapter is missing: %s is the file
// declaring func Caps ("adapters/caps.go").
const flatStaleMessage = "adapters/ no longer wires capabilities: delete func Caps (%s): the generated caps package wires the adapters"

// AdapterPackage is the directory under adapters/ holding module's capabilities' adapters:
// its first segment, lower-cased ("Auth.Login" → "auth").
func AdapterPackage(module string) (dir string) { return gen.AdapterPkg(module) }

// Constructor is the exported function of AdapterPackage(module) returning capability's
// adapter: module's segments after the first, then the capability (("Auth.Login",
// "Password") → "LoginPassword"; ("Markdown", "Markdown") → "Markdown").
func Constructor(module, capability string) string { return gen.AdapterFunc(module, capability) }

// Need is one capability's adapter as the app needs it: capability Cap of Module, implemented
// by func Func of package adapters/Package/, returning the interface Iface
// ("app.Cap__Auth__Login__Password").
type Need struct{ Module, Cap, Package, Func, Iface string }

// Needs are the adapters caps need, in order.
func Needs(caps []*ir.Capability) []Need {
	out := make([]Need, 0, len(caps))
	for _, c := range caps {
		out = append(out, Need{Module: c.Module, Cap: c.Name, Package: AdapterPackage(c.Module), Func: Constructor(c.Module, c.Name),
			Iface: "app.Cap__" + gen.GoName(c.Module, c.Name)})
	}
	return out
}

// String is the line CheckAdapters prints for a missing need: "capability `Auth.Login.Password`
// needs `adapters/auth/` with `func LoginPassword() app.Cap__Auth__Login__Password`".
func (n Need) String() string {
	return fmt.Sprintf("capability `%s.%s` needs `adapters/%s/` with `func %s() %s`", n.Module, n.Cap, n.Package, n.Func, n.Iface)
}

// ScanAdapters reports which of caps the adapters of the app in appDir lack, in caps' order:
// a need is missing when appDir/adapters/<Package> holds no Go file (of the package proper:
// test files and files excluded by their build constraints do not count) or none of them
// declares an exported function named Func with no parameters (its result type is Go's to
// check). The files are parsed, not type-checked; a package with a file that does not parse is
// taken to have every constructor, so Go reports the syntax error. flat reports whether a
// top-level adapters/*.go declares func Caps (the removed flat form). err is an I/O failure.
func ScanAdapters(appDir string, caps []Need) (missing []Need, flat bool, err error) {
	missing, flatFile, err := scanAdapters(appDir, caps)
	return missing, flatFile != "", err
}

// scanAdapters is ScanAdapters, flat as the file declaring func Caps, slash-separated and
// relative to appDir ("adapters/caps.go"), or "".
func scanAdapters(appDir string, caps []Need) (missing []Need, flatFile string, err error) {
	root := filepath.Join(appDir, "adapters")
	funcs := map[string]map[string]string{} // package → its exported parameterless functions' files (nil: does not parse)
	for _, n := range caps {
		fns, ok := funcs[n.Package]
		if !ok {
			if fns, err = packageFuncs(filepath.Join(root, n.Package)); err != nil {
				return nil, "", err
			}
			funcs[n.Package] = fns
		}
		if _, found := fns[n.Func]; fns != nil && !found {
			missing = append(missing, n)
		}
	}
	top, err := packageFuncs(root)
	if err != nil {
		return nil, "", err
	}
	if f := top["Caps"]; f != "" {
		flatFile = "adapters/" + f
	}
	return missing, flatFile, nil
}

// packageFuncs maps the exported, parameterless, non-generic functions (not methods) the Go
// package in dir declares to the file declaring each; empty when dir holds no Go file of the
// package or does not exist, nil when one of its files does not parse.
func packageFuncs(dir string) (map[string]string, error) {
	out := map[string]string{}
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return out, nil
		}
		return nil, err
	}
	fset := token.NewFileSet()
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		if ok, err := build.Default.MatchFile(dir, name); err != nil || !ok {
			continue // excluded by its name or build constraints, or unreadable (Go says so)
		}
		f, err := parser.ParseFile(fset, filepath.Join(dir, name), nil, parser.SkipObjectResolution)
		if err != nil {
			return nil, nil
		}
		for _, d := range f.Decls {
			fd, ok := d.(*ast.FuncDecl)
			if ok && fd.Recv == nil && fd.Name.IsExported() && fd.Type.TypeParams == nil && fd.Type.Params.NumFields() == 0 {
				out[fd.Name.Name] = name
			}
		}
	}
	return out, nil
}

// CheckAdapters checks, before anything is emitted, that the app in appDir has an adapter for
// every capability of caps, and reports whether it does. Otherwise it prints to stderr, as
// applicable: the internal naming error of two capabilities that would share a constructor
// (gen.AdapterCollisionError); that a module's first segment cannot name an adapter package
// (Internal: Go keeps adapters/internal/ private to adapters/, and it holds helpers; Testdata:
// Go skips testdata/ directories); FlatAdaptersMessage (or, when nothing is missing, the
// message to delete func Caps, naming its file) when a top-level adapters/*.go declares func
// Caps; and the heading "<app> is missing adapters for N capabilities:" (or "1 capability";
// <app> is appDir's base name) followed by one Need.String line per missing adapter, all of
// them.
func CheckAdapters(appDir string, caps []*ir.Capability, stderr io.Writer) (ok bool) {
	if err := gen.AdapterCollisionError(caps); err != nil {
		fmt.Fprintln(stderr, err)
		return false
	}
	refused := map[string]bool{}
	for _, c := range caps {
		first, _, _ := strings.Cut(c.Module, ".")
		if refused[first] {
			continue
		}
		switch AdapterPackage(c.Module) {
		case "internal":
			fmt.Fprintf(stderr, "module `%s` cannot name an adapter package: Go keeps `adapters/internal/` private to adapters/ (it holds helpers); rename the module\n", first)
		case "testdata":
			fmt.Fprintf(stderr, "module `%s` cannot name an adapter package: Go skips `testdata/` directories; rename the module\n", first)
		default:
			continue
		}
		refused[first] = true
	}
	if len(refused) > 0 {
		return false
	}
	missing, flatFile, err := scanAdapters(appDir, Needs(caps))
	if err != nil {
		fmt.Fprintf(stderr, "cannot read the adapters of %s: %v\n", appDir, err)
		return false
	}
	if flatFile != "" {
		if len(missing) > 0 {
			fmt.Fprintln(stderr, FlatAdaptersMessage)
		} else {
			fmt.Fprintf(stderr, flatStaleMessage+"\n", flatFile)
		}
	}
	if len(missing) > 0 {
		noun := "capabilities"
		if len(missing) == 1 {
			noun = "capability"
		}
		fmt.Fprintf(stderr, "%s is missing adapters for %d %s:\n", AppName(appDir), len(missing), noun)
		for _, n := range missing {
			fmt.Fprintln(stderr, n)
		}
	}
	return flatFile == "" && len(missing) == 0
}

// AppName is the app's name: the base name of appDir made absolute ("." in examples/blog is
// "blog"); appDir's own base name when it cannot be made absolute.
func AppName(appDir string) string {
	if abs, err := filepath.Abs(appDir); err == nil {
		return filepath.Base(abs)
	}
	return filepath.Base(appDir)
}

// HasAdapters reports whether the app in appDir has adapters: a .go file anywhere under
// appDir/adapters (an adapters/ of notes only is none).
func HasAdapters(appDir string) bool {
	found := false
	filepath.WalkDir(filepath.Join(appDir, "adapters"), func(path string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() && strings.HasSuffix(path, ".go") {
			found = true
			return filepath.SkipAll
		}
		return nil
	})
	return found
}

// CopyAdapters makes dst a copy of the directory tree src (when present; else it removes dst):
// every file, in every subdirectory (test files and testdata/ included, so `go test
// ./adapters/...` in the generated module runs the adapters' tests). A file whose content is
// unchanged is not rewritten; a file or directory of dst that src no longer has is removed
// (dst is alder's copy).
func CopyAdapters(src, dst string, present bool) error {
	if !present {
		return os.RemoveAll(dst)
	}
	keep := map[string]bool{} // slash paths relative to dst that src has
	err := filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		keep[filepath.ToSlash(rel)] = true
		to := filepath.Join(dst, rel)
		if d.IsDir() {
			if st, err := os.Lstat(to); err == nil && !st.IsDir() {
				if err := os.Remove(to); err != nil {
					return err
				}
			}
			return os.MkdirAll(to, 0o755)
		}
		if !d.Type().IsRegular() {
			return nil // a symbolic link or other special file is not copied
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if old, err := os.ReadFile(to); err == nil && bytes.Equal(old, b) {
			return nil
		}
		if st, err := os.Lstat(to); err == nil && st.IsDir() {
			if err := os.RemoveAll(to); err != nil {
				return err
			}
		}
		return os.WriteFile(to, b, 0o644)
	})
	if err != nil {
		return err
	}
	// remove what src no longer has (a removed directory goes whole)
	var stale []string
	err = filepath.WalkDir(dst, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(dst, path)
		if err != nil {
			return err
		}
		if !keep[filepath.ToSlash(rel)] {
			stale = append(stale, path)
			if d.IsDir() {
				return filepath.SkipDir
			}
		}
		return nil
	})
	if err != nil {
		return err
	}
	for _, p := range stale {
		if err := os.RemoveAll(p); err != nil {
			return err
		}
	}
	return nil
}
