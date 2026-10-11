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
// never wired. Before emitting, alder build checks with CheckAdapters that every needed package
// and constructor exists, so a missing one is named exactly instead of surfacing as a Go error
// in the generated caps package; the test builds (alder test, eval, the REPL) check with
// CheckTestAdapters, which leaves a missing one unwired, with a warning, so fakes come first.

// FlatAdaptersMessage is printed when a top-level adapters/*.go declares func Caps and
// adapters are missing (listed after it): the flat form (one package adapters whose Caps
// returns every capability) is no longer wired. When none is missing, flatStaleMessage is
// printed instead.
const FlatAdaptersMessage = "adapters/ no longer wires capabilities: move each adapter to adapters/<first module segment, lower-cased>/ (see the list below)"

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
// ("app.Cap__Auth__Login__Password"). Found, set by the scan on a missing need, is a near miss
// the package declares under the constructor's name ("func Markdown(style string): it must
// take no parameters"), or "".
type Need struct{ Module, Cap, Package, Func, Iface, Found string }

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
// needs `adapters/auth/` with `func LoginPassword() app.Cap__Auth__Login__Password`", followed,
// when the package directory is a Go keyword, by "(select is a Go keyword: name the package
// e.g. selectadapter)", and by the near miss the scan found, "(found func LoginPassword(email
// string): it must take no parameters)".
func (n Need) String() string {
	s := fmt.Sprintf("capability `%s.%s` needs `adapters/%s/` with `func %s() %s`", n.Module, n.Cap, n.Package, n.Func, n.Iface)
	if token.IsKeyword(n.Package) {
		s += fmt.Sprintf(" (%s is a Go keyword: name the package e.g. %sadapter)", n.Package, n.Package)
	}
	if n.Found != "" {
		s += " (found " + n.Found + ")"
	}
	return s
}

// symlinkNote ends the list of missing adapters when the scan skipped a symbolic link under
// adapters/: neither the scan nor CopyAdapters follows one.
const symlinkNote = "(symlinks under adapters/ are ignored)"

// ScanAdapters reports which of caps the adapters of the app in appDir lack, in caps' order:
// a need is missing when appDir/adapters has no directory named exactly Package (a symbolic
// link is no directory, and the name's letter case counts), or that directory holds no Go file
// (of the package proper: test files, files excluded by their build constraints and symbolic
// links do not count) or none of them declares an exported function named Func with no
// parameters (its result type is Go's to check). The files are parsed, not type-checked; a
// package with a file that does not parse is taken to have every constructor, so Go reports
// the syntax error. flat reports whether a top-level adapters/*.go declares func Caps (the
// removed flat form). err is an I/O failure.
func ScanAdapters(appDir string, caps []Need) (missing []Need, flat bool, err error) {
	s, err := scanAdapters(appDir, caps)
	return s.missing, s.flatFile != "", err
}

// scan is what scanAdapters found.
type scan struct {
	missing  []Need
	flatFile string      // the file declaring func Caps, slash-separated and relative to appDir ("adapters/caps.go"), or ""
	miscased [][2]string // directories named like a needed package but for letter case: {"Mail", "mail"}, in caps' order
	symlinks bool        // a symbolic link was skipped
}

// scanAdapters is ScanAdapters, with the misnamed directories and skipped links. A need whose
// package directory is mis-cased is in miscased, not missing.
func scanAdapters(appDir string, caps []Need) (s scan, err error) {
	root := filepath.Join(appDir, "adapters")
	var entries []fs.DirEntry
	rootLink := false
	if st, err := os.Lstat(root); err == nil && st.Mode()&fs.ModeSymlink != 0 {
		rootLink, s.symlinks = true, true // adapters/ itself: no adapters
	} else if entries, err = os.ReadDir(root); err != nil && !os.IsNotExist(err) {
		return scan{}, err
	}
	byName := map[string]fs.DirEntry{}
	for _, e := range entries {
		byName[e.Name()] = e
	}
	funcs := map[string]map[string]found{} // package → its exported top-level names (nil: does not parse)
	cased := map[string]bool{}
	for _, n := range caps {
		fns, ok := funcs[n.Package]
		if !ok {
			switch e := byName[n.Package]; {
			case e != nil && e.Type()&fs.ModeSymlink != 0:
				s.symlinks = true
				fns = map[string]found{}
			case e != nil && e.IsDir():
				var links bool
				if fns, links, err = packageFuncs(filepath.Join(root, n.Package)); err != nil {
					return scan{}, err
				}
				s.symlinks = s.symlinks || links
			default:
				fns = map[string]found{}
				for _, e := range entries {
					if e.IsDir() && e.Name() != n.Package && strings.EqualFold(e.Name(), n.Package) {
						fns = nil // mis-cased: not a missing need, a misnamed directory
						if !cased[n.Package] {
							cased[n.Package] = true
							s.miscased = append(s.miscased, [2]string{e.Name(), n.Package})
						}
						break
					}
				}
				if fns == nil {
					funcs[n.Package] = nil
					continue
				}
			}
			funcs[n.Package] = fns
		}
		if fns == nil {
			continue // does not parse (Go says so), or mis-cased
		}
		if f := fns[n.Func]; f.problem != "" || f.file == "" {
			n.Found = f.problem
			s.missing = append(s.missing, n)
		}
	}
	if rootLink {
		return s, nil
	}
	top, links, err := packageFuncs(root)
	if err != nil {
		return scan{}, err
	}
	s.symlinks = s.symlinks || links
	if f := top["Caps"]; f.file != "" && f.problem == "" {
		s.flatFile = "adapters/" + f.file
	}
	return s, nil
}

// found is a top-level exported name a package declares: the file declaring it and, when it
// is not an exported, parameterless, non-generic function, why it is no constructor ("func
// Mailer(host string): it must take no parameters", "var Mailer: it must be a func").
type found struct{ file, problem string }

// packageFuncs maps the exported top-level names (functions, not methods; variables,
// constants and types) the Go package in dir declares to where and how (found); empty when
// dir holds no Go file of the package or does not exist, nil when one of its files does not
// parse. A function that is a constructor wins over a near miss of the same name in another
// file. links reports a symbolic link among dir's entries (skipped, as CopyAdapters skips it).
func packageFuncs(dir string) (out map[string]found, links bool, err error) {
	out = map[string]found{}
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return out, false, nil
		}
		return nil, false, err
	}
	fset := token.NewFileSet()
	for _, e := range entries {
		name := e.Name()
		if e.Type()&fs.ModeSymlink != 0 {
			links = true
			continue
		}
		if !e.Type().IsRegular() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		if ok, err := build.Default.MatchFile(dir, name); err != nil || !ok {
			continue // excluded by its name or build constraints, or unreadable (Go says so)
		}
		src, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			return nil, false, err
		}
		f, err := parser.ParseFile(fset, filepath.Join(dir, name), src, parser.SkipObjectResolution)
		if err != nil {
			return nil, links, nil
		}
		text := func(n ast.Node) string {
			return string(src[fset.Position(n.Pos()).Offset:fset.Position(n.End()).Offset])
		}
		note := func(id string, fd found) {
			if prev, ok := out[id]; !ok || (prev.problem != "" && fd.problem == "") {
				out[id] = fd
			}
		}
		for _, d := range f.Decls {
			switch d := d.(type) {
			case *ast.FuncDecl:
				if d.Recv != nil || !d.Name.IsExported() {
					continue
				}
				sig := "func " + d.Name.Name
				if d.Type.TypeParams != nil {
					sig += text(d.Type.TypeParams)
				}
				sig += text(d.Type.Params)
				switch {
				case d.Type.TypeParams != nil:
					note(d.Name.Name, found{name, sig + ": it must not be generic"})
				case d.Type.Params.NumFields() > 0:
					note(d.Name.Name, found{name, sig + ": it must take no parameters"})
				default:
					note(d.Name.Name, found{file: name})
				}
			case *ast.GenDecl:
				kind := d.Tok.String() // var, const, type
				for _, sp := range d.Specs {
					var ids []*ast.Ident
					switch sp := sp.(type) {
					case *ast.ValueSpec:
						ids = sp.Names
					case *ast.TypeSpec:
						ids = []*ast.Ident{sp.Name}
					}
					for _, id := range ids {
						if id.IsExported() {
							note(id.Name, found{name, kind + " " + id.Name + ": it must be a func"})
						}
					}
				}
			}
		}
	}
	return out, links, nil
}

// CheckAdapters checks, before anything is emitted, that the app in appDir has an adapter for
// every capability of caps, and reports whether it does. Otherwise it prints to stderr, as
// applicable: the internal naming error of two capabilities that would share a constructor
// (gen.AdapterCollisionError); that a module's first segment cannot name an adapter package
// (Internal: Go keeps adapters/internal/ private to adapters/, and it holds helpers; Testdata:
// Go skips testdata/ directories); that adapters/go.mod must go (adapters/ is part of the
// generated module); "adapters/Mail/ must be adapters/mail/" for each directory named like a
// needed package but for letter case; FlatAdaptersMessage (or, when nothing is missing, the
// message to delete func Caps, naming its file) when a top-level adapters/*.go declares func
// Caps; and the heading "<app> is missing adapters for N capabilities:" (or "1 capability";
// <app> is appDir's base name) followed by one Need.String line per missing adapter, all of
// them, and symlinkNote when the scan skipped a symbolic link.
func CheckAdapters(appDir string, caps []*ir.Capability, stderr io.Writer) (ok bool) {
	_, _, ok = checkAdapters(appDir, caps, stderr, false)
	return ok
}

// CheckTestAdapters is CheckAdapters for a test build (alder test, eval and the REPL): every
// refusal is CheckAdapters' except missing adapters, which are not refused but returned, in
// caps' order, for the build to leave unwired (gen.Options.Unwired; a call then fails "not
// wired in tests"), with the warning the caller prints, so an app can be tested with fakes
// before its Go is written: "warning: 1 capability without an adapter runs unwired in tests:"
// (or "N capabilities without adapters run unwired in tests:"), one Need.String line each, and
// symlinkNote when the scan skipped a symbolic link; it ends in a newline, and is "" when
// nothing is missing. ok is false when it printed a refusal (then unwired is nil).
func CheckTestAdapters(appDir string, caps []*ir.Capability, stderr io.Writer) (unwired []Need, warning string, ok bool) {
	unwired, links, ok := checkAdapters(appDir, caps, stderr, true)
	if !ok || len(unwired) == 0 {
		return nil, "", ok
	}
	var b strings.Builder
	if len(unwired) == 1 {
		b.WriteString("warning: 1 capability without an adapter runs unwired in tests:\n")
	} else {
		fmt.Fprintf(&b, "warning: %d capabilities without adapters run unwired in tests:\n", len(unwired))
	}
	for _, n := range unwired {
		fmt.Fprintln(&b, n)
	}
	if links {
		fmt.Fprintln(&b, symlinkNote)
	}
	return unwired, b.String(), true
}

// GoModMessage refuses an adapters/go.mod: it would make adapters/ a module of its own, out of
// the generated one.
const GoModMessage = "adapters/go.mod: adapters/ is part of the generated module; delete it (your editor resolves `app` in build/<app>/)"

// checkAdapters is CheckAdapters, and with lenient CheckTestAdapters (links: the scan skipped
// a symbolic link).
func checkAdapters(appDir string, caps []*ir.Capability, stderr io.Writer, lenient bool) (unwired []Need, links, ok bool) {
	if err := gen.AdapterCollisionError(caps); err != nil {
		fmt.Fprintln(stderr, err)
		return nil, false, false
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
		return nil, false, false
	}
	ok = true
	if _, err := os.Lstat(filepath.Join(appDir, "adapters", "go.mod")); err == nil {
		fmt.Fprintln(stderr, GoModMessage)
		ok = false
	}
	s, err := scanAdapters(appDir, Needs(caps))
	if err != nil {
		fmt.Fprintf(stderr, "cannot read the adapters of %s: %v\n", appDir, err)
		return nil, false, false
	}
	for _, m := range s.miscased {
		fmt.Fprintf(stderr, "adapters/%s/ must be adapters/%s/ (an adapter package's directory is its module's first segment, lower-cased)\n", m[0], m[1])
		ok = false
	}
	if s.flatFile != "" {
		if len(s.missing) > 0 {
			fmt.Fprintln(stderr, FlatAdaptersMessage)
		} else {
			fmt.Fprintf(stderr, flatStaleMessage+"\n", s.flatFile)
		}
		ok = false
	}
	if len(s.missing) == 0 {
		return nil, false, ok
	}
	if lenient && s.flatFile == "" {
		return s.missing, s.symlinks, ok
	}
	noun := "capabilities"
	if len(s.missing) == 1 {
		noun = "capability"
	}
	fmt.Fprintf(stderr, "%s is missing adapters for %d %s:\n", AppName(appDir), len(s.missing), noun)
	for _, n := range s.missing {
		fmt.Fprintln(stderr, n)
	}
	if s.symlinks {
		fmt.Fprintln(stderr, symlinkNote)
	}
	return nil, false, false
}

// AdaptersCompileMessage is what alder build (and a test build: scratch.AdapterError) says when
// the generated package vets but the adapters, copied to dir, or the caps package wiring them do
// not: "the adapters of <app> (copied to <dir>) do not compile with the app: <err>" and Go's
// output out.
func AdaptersCompileMessage(app, dir string, err error, out []byte) string {
	return fmt.Sprintf("the adapters of %s (copied to %s) do not compile with the app: %v\n%s", app, dir, err, out)
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
// appDir/adapters (an adapters/ of notes only is none; a symbolic link is no file).
func HasAdapters(appDir string) bool {
	found := false
	filepath.WalkDir(filepath.Join(appDir, "adapters"), func(path string, d fs.DirEntry, err error) error {
		if err == nil && d.Type().IsRegular() && strings.HasSuffix(path, ".go") {
			found = true
			return filepath.SkipAll
		}
		return nil
	})
	return found
}

// CopyAdapters makes dst a copy of the directory tree src (when present; else it removes dst):
// every file, in every subdirectory (test files and testdata/ included, so `go test
// ./adapters/...` in the generated module runs the adapters' tests); a symbolic link is not
// copied (ScanAdapters ignores it too). First, a file or directory of dst that src no longer
// has, by its exact name, is removed (dst is alder's copy), so a directory renamed only in its
// letter case (Mail/ → mail/) is not kept, nor its new copy removed, on a case-insensitive file
// system; then a file whose content is unchanged is not rewritten.
func CopyAdapters(src, dst string, present bool) error {
	if !present {
		return os.RemoveAll(dst)
	}
	keep := map[string]bool{} // slash paths relative to src of its directories and regular files
	err := filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || d.Type().IsRegular() {
			rel, err := filepath.Rel(src, path)
			if err != nil {
				return err
			}
			keep[filepath.ToSlash(rel)] = true
		}
		return nil
	})
	if err != nil {
		return err
	}
	// remove what src no longer has (a removed directory goes whole)
	var stale []string
	err = filepath.WalkDir(dst, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			if os.IsNotExist(err) && path == dst {
				return filepath.SkipAll
			}
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
	return filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
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
}
