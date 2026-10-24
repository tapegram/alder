package generate

import (
	"fmt"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"alder/internal/gen"
	"alder/internal/ir"
	"alder/internal/syntax"
)

// CapabilityData is the data of capability.tmpl.
type CapabilityData struct {
	Module  string
	Imports []string
	Name    string   // the capability: the module's last segment
	Entry   string   // its entry: "current"
	Type    string   // the entry's type as written: "{ city: Text } -> Query (Maybe Float)"
	Params  []string // the fake entry's parameters: "args" for one, "a1 a2" for several
	Package string   // the adapter's directory under adapters/
	Func    string   // its constructor
}

// NewCapability generates `alder new capability <Module> <entry> "<Type>"`: the capability
// named after the module's last segment, with its one entry, in the module (created when it
// does not exist); a `fake` of it whose entry is the typed hole `?<entry>`; a test that installs
// the fake; and the adapter, adapters/<segment>/<constructor>.go, with the exact constructor
// chapter 7's rule names, whose methods return the error "not implemented". It refuses a module
// that already declares something of the capability's name.
func NewCapability(p *Program, module, entry, typ string) ([]Edit, error) {
	if !IsModuleName(module) {
		return nil, Refusef("`%s` is not a module name: write capitalised names joined by dots, such as `Weather`", module)
	}
	if !IsLowerIdent(entry) {
		return nil, Refusef("`%s` is not an entry name: an entry is named by a lower-case identifier, such as `current`", entry)
	}
	segs := strings.Split(module, ".")
	d := CapabilityData{Module: module, Name: segs[len(segs)-1], Entry: entry, Type: strings.Join(strings.Fields(typ), " "),
		Package: gen.AdapterPkg(module), Func: gen.AdapterFunc(module, segs[len(segs)-1])}
	arity, err := entryArity(d.Type)
	if err != nil {
		return nil, err
	}
	switch {
	case arity == 1:
		d.Params = []string{"args"}
	case arity > 1:
		for i := 1; i <= arity; i++ {
			d.Params = append(d.Params, fmt.Sprintf("a%d", i))
		}
	}
	imports := newImports(module, p.Std)
	typeImports(imports, d.Type, p)
	install := TestData{Name: quote("the " + d.Name + " fake installs"), Comment: []string{"It proves the fake compiles and installs; tests of what the capability does use it."}, Lines: []string{"let _ <- Test.fake " + d.Name}, Result: "True"}

	var edit Edit
	if m := p.Modules[module]; m != nil {
		if declared(m.Syntax, d.Name) {
			return nil, Refusef("`%s` already declares `%s`: add the entry `%s` to it by hand (and to its fake and its adapter)", module, d.Name, entry)
		}
		e := newSrcEdit(m.Source)
		addImports(e, p, m.Syntax, imports.list())
		e.appendText("\n" + snippet("capability", d))
		if err := addTests(e, m, []TestData{install}); err != nil {
			return nil, err
		}
		edit = Edit{Path: m.File, Body: e.result()}
	} else {
		d.Imports = imports.list()
		edit = Edit{Path: FileOf(module), Create: true, Body: canonical(p, FileOf(module), snippet("capability_module", d)+"\n\n"+snippet("test", install))}
	}

	// the adapter, from the checked capability
	probe, err := Probe(p.Dir, []Edit{edit})
	if err != nil {
		return nil, err
	}
	if r := probe.errorsIn(edit.Path); r != nil {
		return nil, r
	}
	var c *ir.Capability
	for _, x := range ir.Lower(probe.Prog, probe.Res).Caps {
		if x.Module == module && x.Name == d.Name {
			c = x
		}
	}
	if c == nil {
		return nil, Refusef("internal error: the capability %s.%s was not checked", module, d.Name)
	}
	pkg, err := adapterPackage(p.Dir, d.Package)
	if err != nil {
		return nil, err
	}
	stub, err := gen.AdapterStub(c, pkg)
	if err != nil {
		return nil, fmt.Errorf("internal error: the adapter stub does not format: %v", err)
	}
	file := strings.ToLower(d.Name) + "_" + Snake(entry) + ".go" // weather_current.go
	return []Edit{edit, {Path: "adapters/" + d.Package + "/" + file, Create: true, Body: string(stub)}}, nil
}

// entryArity is the number of parameters of a capability entry of type typ (0 for an entry that
// is a Query or Task itself), parsed as the language parses it.
func entryArity(typ string) (int, error) {
	src := "module X\n\ncapability X {\n  e : " + typ + ",\n}\n"
	m, ds := syntax.ParseFile("X.alder", []byte(src))
	for _, d := range ds {
		if d.Severity == "error" {
			return 0, Refusef("`%s` is not a type: %s", typ, d.Message)
		}
	}
	for _, d := range m.Decls {
		if c, ok := d.(*syntax.CapDecl); ok && len(c.Entries) == 1 {
			if ft, ok := c.Entries[0].Type.(*syntax.FunType); ok {
				return len(ft.Params), nil
			}
			return 0, nil
		}
	}
	return 0, Refusef("`%s` is not a type", typ)
}

var packageClause = regexp.MustCompile(`(?m)^package\s+(\w+)`)

// adapterPackage is the Go package name of adapters/<dir>/: the name its Go files already use,
// else dir (dir + "adapter" when dir is a Go keyword, as chapter 7 says).
func adapterPackage(appDir, dir string) (string, error) {
	entries, err := os.ReadDir(filepath.Join(appDir, "adapters", dir))
	if err == nil {
		for _, e := range entries {
			if e.IsDir() || !strings.HasSuffix(e.Name(), ".go") || strings.HasSuffix(e.Name(), "_test.go") {
				continue
			}
			b, err := os.ReadFile(filepath.Join(appDir, "adapters", dir, e.Name()))
			if err != nil {
				return "", err
			}
			if m := packageClause.FindSubmatch(b); m != nil {
				return string(m[1]), nil
			}
		}
	}
	if token.IsKeyword(dir) {
		return dir + "adapter", nil
	}
	return dir, nil
}
