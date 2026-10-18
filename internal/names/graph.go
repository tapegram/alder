// internal/names/graph.go
package names

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"alder/internal/diag"
	"alder/internal/syntax"
)

type Program struct {
	Modules map[string]*syntax.Module
	Order   []string
	Pages   map[string]*syntax.PageDecl
	AppMsgs map[string]*syntax.MsgDecl
	Std     map[string]*syntax.Module
	scopes  map[string]*moduleScope
	// ctorIndex maps a constructor (or brand) name to the app modules declaring it, and
	// tableIndex a table name to the app modules declaring it that are not named after it;
	// both are built before any body is resolved, for the hints of names not imported.
	ctorIndex  map[string][]ctorOwner
	tableIndex map[string][]string
	// missingImports maps a module name that some `import` names but no module has to the first
	// module (in Order) importing it.
	missingImports map[string]string
	Failed         map[string]bool // module name -> resolution produced errors
	// stdNamed are the names of app modules refused because a standard-library module has the
	// name (E0017); uses of the name resolve to the stdlib module, and a name the stdlib module
	// lacks is not reported (it is the app module's), but quiets the module using it instead.
	stdNamed map[string]bool
	// Refused are the app modules left out of the program by E0017, kept for their source lines.
	Refused []*syntax.Module
	quiet   map[string]bool // modules that used a stdNamed module's missing name: Failed, silently
	// ParseFailed is set by Load when any file had a syntax error; such a program must not be checked.
	ParseFailed bool
	// ElementsManifest is the content of the app's elements/custom-elements.json (the custom
	// elements it declares, chapter 5), read by Load; nil when the file does not exist. The
	// checker parses it (internal/elements).
	ElementsManifest []byte
	// AdapterEnv are the environment variables the app's adapter packages read (ScanAdapterEnv),
	// read by Load; the checker compares them with `deploy.requires` (W0006).
	AdapterEnv []EnvRead
}

// Load parses every .alder file under dir/app and adds the modules extra, parsed by the caller
// from memory (their File is what the caller named them, e.g. "Scratch.alder", and their parse
// diagnostics are the caller's), after the directory's; duplicate, cycle and naming rules apply
// to them as to the rest. It also reads dir/elements/custom-elements.json, if there is one
// (Program.ElementsManifest). I/O failures (unreadable directory or file) are returned
// separately as ioErrs; the caller must not check a program that failed to load.
func Load(dir string, std map[string]*syntax.Module, extra ...*syntax.Module) (*Program, *diag.Bag, []error) {
	var ioErrs []error
	bag := &diag.Bag{}
	var mods []*syntax.Module
	root := filepath.Join(dir, "app")
	filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			ioErrs = append(ioErrs, err)
			return nil
		}
		if d.IsDir() || !strings.HasSuffix(path, ".alder") {
			return nil
		}
		src, err := os.ReadFile(path)
		if err != nil {
			ioErrs = append(ioErrs, err)
			return nil
		}
		rel, _ := filepath.Rel(dir, path)
		m, ds := syntax.ParseFile(filepath.ToSlash(rel), src)
		bag.Items = append(bag.Items, ds...)
		mods = append(mods, m)
		return nil
	})
	parseFailed := bag.HasErrors()
	mods = append(mods, extra...)
	prog, b2 := Build(mods, std)
	prog.ParseFailed = parseFailed
	if data, err := os.ReadFile(filepath.Join(dir, "elements", "custom-elements.json")); err == nil {
		prog.ElementsManifest = data
	} else if !errors.Is(err, fs.ErrNotExist) {
		ioErrs = append(ioErrs, err)
	}
	prog.AdapterEnv = ScanAdapterEnv(dir)
	bag.Items = append(bag.Items, b2.Items...)
	return prog, bag, ioErrs
}

// ReservedModules are the first module segments no app module may have (E0015): the compiled
// program's exported names Route__…, Cap__… and Args__… start with them (internal/gen, names.go
// rule 5).
var ReservedModules = []string{"Args", "Cap", "Route"}

func Build(mods []*syntax.Module, std map[string]*syntax.Module) (*Program, *diag.Bag) {
	bag := &diag.Bag{}
	p := &Program{Modules: map[string]*syntax.Module{}, Pages: map[string]*syntax.PageDecl{}, AppMsgs: map[string]*syntax.MsgDecl{}, Std: std, scopes: map[string]*moduleScope{},
		stdNamed: map[string]bool{}, quiet: map[string]bool{}}
	folded := map[string]*syntax.Module{} // by lower-cased name
	for _, m := range mods {
		if prev, dup := p.Modules[m.Name]; dup {
			bag.Error(diag.Duplicate, m.Span, "module `%s` is also defined in %s", m.Name, prev.File)
			continue
		}
		// A module named like a stdlib module would be shadowed by it: `Test.helper` would be
		// looked up in the stdlib's Test. It is refused and left out of the program.
		if _, isStd := std[m.Name]; isStd {
			bag.Error(diag.StdModule, m.Span, "module `%s` is a standard-library module; rename it", m.Name)
			p.stdNamed[m.Name] = true
			p.Refused = append(p.Refused, m)
			continue
		}
		// Each module becomes a file named after it (<Module>_alder.go); on a case-insensitive
		// file system two names that differ only in case would be one file.
		if prev, clash := folded[strings.ToLower(m.Name)]; clash {
			bag.Error(diag.ModuleCase, m.Span, "module names `%s` and `%s` differ only in case (%s, %s); rename one of them", prev.Name, m.Name, prev.File, m.File)
		} else {
			folded[strings.ToLower(m.Name)] = m
		}
		if first, _, _ := strings.Cut(m.Name, "."); slices.Contains(ReservedModules, first) {
			bag.Error(diag.ReservedModule, m.Span, "module name `%s` is reserved: the compiled program's names `%s__…` are taken; rename the module", m.Name, first)
		}
		p.Modules[m.Name] = m
		if want := expectedName(m.File); want != "" && want != m.Name {
			if msg := badSegment(m.File, want); msg != "" {
				bag.Error(diag.ModulePath, m.Span, "module `%s` is in file %s: %s", m.Name, m.File, msg)
			} else {
				bag.Error(diag.ModulePath, m.Span, "module `%s` is in file %s; expected `module %s`", m.Name, m.File, want)
			}
		}
		for _, d := range m.Decls {
			switch d := d.(type) {
			case *syntax.PageDecl:
				p.Pages[m.Name] = d
			case *syntax.MsgDecl:
				if m.Name == "App" {
					p.AppMsgs[d.Name] = d
				}
			}
		}
	}
	p.Order = p.topoSort(bag)
	return p, bag
}

// expectedName: "app/Todos/Index.alder" → "Todos.Index"; "" if the file is not under app/.
func expectedName(file string) string {
	file = filepath.ToSlash(file)
	i := strings.Index(file, "app/")
	if i < 0 {
		return ""
	}
	rest := strings.TrimSuffix(file[i+4:], ".alder")
	return strings.ReplaceAll(rest, "/", ".")
}

// badSegment explains a path under app/ that no module line could match: folders (or the file's
// name) that are not module name segments, such as `app/pages/About.alder`, whose expected
// `module pages.About` would itself be a parse error. It names every such segment, the rename
// of each, and the module line to write after them; when some segment has no rename it can
// suggest (`2024`), it only says they must be renamed, so it never suggests an impossible
// module. "" when every segment is valid.
func badSegment(file, want string) string {
	segs := strings.Split(want, ".")
	var names, renames []string
	folders, upperOnly, suggest := 0, true, true
	fixed := slices.Clone(segs)
	for i, seg := range segs {
		if seg == "" || isModuleSegment(seg) {
			continue
		}
		last := i == len(segs)-1
		if !last {
			folders++
		}
		names = append(names, "`"+seg+"`")
		upperOnly = upperOnly && isModuleSegment(strings.ToUpper(seg[:1])+seg[1:])
		f := segmentFor(seg)
		if f == "" {
			suggest = false
			continue
		}
		fixed[i] = f
		if last {
			f += ".alder"
		}
		renames = append(renames, "`"+f+"`")
	}
	if len(names) == 0 {
		return ""
	}
	n := len(names)
	var what string
	switch {
	case folders == n:
		what = map[bool]string{true: "the folder ", false: "the folders "}[n == 1] + list(names)
	case folders == 0:
		what = "the file name " + names[0]
	default:
		what = map[bool]string{true: "the folder ", false: "the folders "}[folders == 1] + list(names[:folders]) + " and the file name " + names[folders]
	}
	be, it := "must be a module name", "it"
	if n > 1 {
		be, it = "must be module names", "them"
	}
	rule := be + ", starting with an uppercase letter"
	if !upperOnly {
		rule = be + ": an uppercase letter, then letters, digits and `_`, not ending in `_`"
	}
	if !suggest {
		return fmt.Sprintf("%s %s: rename %s", what, rule, it)
	}
	return fmt.Sprintf("%s %s: rename %s %s and write `module %s`", what, rule, it, list(renames), strings.Join(fixed, "."))
}

// list joins items as prose: "a", "a and b", "a, b and c".
func list(items []string) string {
	if len(items) == 1 {
		return items[0]
	}
	return strings.Join(items[:len(items)-1], ", ") + " and " + items[len(items)-1]
}

// isModuleSegment: an uppercase letter, then letters, digits and single underscores, not ending
// in `_` (the lexer's upper-case identifier, as a module name segment: A-grammar).
func isModuleSegment(s string) bool {
	if s == "" || s[0] < 'A' || s[0] > 'Z' || strings.Contains(s, "__") || strings.HasSuffix(s, "_") {
		return false
	}
	for _, c := range []byte(s) {
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '_') {
			return false
		}
	}
	return true
}

// segmentFor suggests a module segment for a folder or file name: its runs of letters and digits,
// each capitalised, joined (`pages` → `Pages`, `blog-posts` → `BlogPosts`, `Foo_` → `Foo`); ""
// when there is none that starts with a letter.
func segmentFor(s string) string {
	if isModuleSegment(s) {
		return s
	}
	var b strings.Builder
	for _, part := range strings.FieldsFunc(s, func(r rune) bool {
		return !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9')
	}) {
		b.WriteString(strings.ToUpper(part[:1]) + part[1:])
	}
	if out := b.String(); isModuleSegment(out) {
		return out
	}
	return ""
}

// ctorOwner is a module declaring a constructor, and the type it constructs.
type ctorOwner struct{ Module, Type string }

func (p *Program) IsPageModule(name string) bool { _, ok := p.Pages[name]; return ok }

// topoSort orders modules dependencies-first; reports one E0006 per cycle and still returns every module.
func (p *Program) topoSort(bag *diag.Bag) []string {
	const (
		white, grey, black = 0, 1, 2
	)
	color := map[string]int{}
	var order []string
	var stack []string
	reported := map[string]bool{}
	var visit func(name string)
	visit = func(name string) {
		switch color[name] {
		case black:
			return
		case grey:
			// cycle: stack from name to end
			start := 0
			for i, s := range stack {
				if s == name {
					start = i
				}
			}
			cyc := append(append([]string{}, stack[start:]...), name)
			key := strings.Join(cyc, ">")
			if !reported[key] {
				reported[key] = true
				m := p.Modules[cyc[0]]
				var sp diag.Span = m.Span
				for _, im := range m.Imports {
					if im.Module == cyc[1] {
						sp = im.Span
					}
				}
				bag.Error(diag.ImportCycle, sp, "import cycle: %s", strings.Join(cyc, " -> "))
			}
			return
		}
		color[name] = grey
		stack = append(stack, name)
		if m := p.Modules[name]; m != nil {
			for _, im := range m.Imports {
				if _, ok := p.Modules[im.Module]; ok {
					visit(im.Module)
				}
			}
		}
		stack = stack[:len(stack)-1]
		color[name] = black
		order = append(order, name)
	}
	names := make([]string, 0, len(p.Modules))
	for n := range p.Modules {
		names = append(names, n)
	}
	sortStrings(names)
	for _, n := range names {
		visit(n)
	}
	return order
}

func sortStrings(s []string) {
	for i := 1; i < len(s); i++ {
		for j := i; j > 0 && s[j] < s[j-1]; j-- {
			s[j], s[j-1] = s[j-1], s[j]
		}
	}
}

// MarkFailed records which modules produced resolution errors, by diagnostic file, and the
// modules that used a name of an app module refused by E0017 (whose error is that E0017).
func (p *Program) MarkFailed(bag *diag.Bag) {
	if p.Failed == nil {
		p.Failed = map[string]bool{}
	}
	byFile := map[string]string{}
	for name, m := range p.Modules {
		byFile[m.File] = name
	}
	for _, d := range bag.Items {
		if d.Severity != "error" {
			continue
		}
		if name, ok := byFile[d.Span.File]; ok {
			p.Failed[name] = true
		}
	}
	for name := range p.quiet {
		p.Failed[name] = true
	}
}
