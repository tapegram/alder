// internal/names/resolve.go
package names

import (
	"fmt"
	"sort"
	"strings"

	"alder/internal/diag"
	"alder/internal/syntax"
)

var preludeTypes = map[string]bool{"Int": true, "Float": true, "Bool": true, "Text": true, "Unit": true, "Date": true, "Time": true, "List": true, "Maybe": true, "Result": true, "Dict": true, "Html": true, "Never": true, "Query": true, "Task": true, "Nav": true, "Error": true, "Form": true, "Table": true, "Id": true, "Insert": true, "Select": true, "Condition": true, "Order": true, "Page": true, "Doc": true, "Answer": true, "File": true, "Bytes": true}
var generatedTypes = map[string]bool{"Params": true, "Msg": true, "Session": true}

// moduleScope indexes one module's declarations.
type moduleScope struct {
	name   string
	values map[string]bool
	types  map[string]bool   // type, table, form, brand names
	forms  map[string]bool   // form names
	ctors  map[string]string // ctor → owning type name
	brands map[string]bool   // brand names; the constructor is usable only in the declaring module
	tables map[string]bool
	// fakeTables maps each table declared inside a `fake` to the fake's capability name; such a
	// table belongs to the fake, not to its module's name (E0018 does not apply to it)
	fakeTables map[string]string
	caps       map[string]bool
	// capEntries holds, for a capability named like its module's last segment (`capability
	// Markdown` in module Markdown or Lib.Markdown), its entry names: `Markdown.render` from
	// another module, through the module's qualifier, is that entry.
	capEntries map[string]bool
	// entryCaps maps each entry name of any of its capabilities to the first capability
	// declaring it: `Mail.send` for `capability Mailer { send … }` says to write `Mailer.send`.
	entryCaps map[string]string
	comps     map[string]bool            // capitalized value names
	columns   map[string]map[string]bool // table → field set
	// imports holds the module's imports by canonical qualifier (design rule 3), whatever alias
	// is written: the written alias is only checked (E0204), so references keep resolving while
	// it is wrong. An import with no usable qualifier (E0206) is not here.
	imports map[string]*syntax.Import
	quals   map[string]string // import path → canonical qualifier ("" for none)
	used    map[string]bool   // by import path
}

func (p *Program) index(m *syntax.Module) *moduleScope {
	s := &moduleScope{name: m.Name, values: map[string]bool{}, types: map[string]bool{}, forms: map[string]bool{}, ctors: map[string]string{}, brands: map[string]bool{}, tables: map[string]bool{}, fakeTables: map[string]string{}, caps: map[string]bool{}, capEntries: map[string]bool{}, entryCaps: map[string]string{}, comps: map[string]bool{}, columns: map[string]map[string]bool{}, imports: map[string]*syntax.Import{}, used: map[string]bool{}}
	s.quals = p.qualifiers(m)
	for _, im := range m.Imports {
		if q := s.quals[im.Module]; q != "" {
			s.imports[q] = im
		}
	}
	for _, d := range m.Decls {
		switch d := d.(type) {
		case *syntax.ValueDecl:
			if isUpper(d.Name) {
				s.comps[d.Name] = true
			}
			s.values[d.Name] = true
		case *syntax.TypeDecl:
			s.types[d.Name] = true
			if d.Brand != nil {
				s.brands[d.Name] = true
			}
			for _, c := range d.Union {
				s.ctors[c.Name] = d.Name
			}
		case *syntax.TableDecl:
			s.table(d)
		case *syntax.FakeDecl:
			for _, t := range d.Tables {
				s.table(t)
				s.fakeTables[t.Name] = d.Name
			}
		case *syntax.FormDecl:
			s.types[d.Name] = true
			s.forms[d.Name] = true
		case *syntax.CapDecl:
			s.caps[d.Name] = true
			for _, e := range d.Entries {
				if _, seen := s.entryCaps[e.Name]; !seen {
					s.entryCaps[e.Name] = d.Name
				}
			}
			if d.Name == lastSegment(m.Name) {
				for _, e := range d.Entries {
					s.capEntries[e.Name] = true
				}
			}
		case *syntax.MsgDecl:
			s.ctors[d.Name] = "Msg"
		case *syntax.PageDecl:
			s.types["Params"] = true
		}
	}
	if m.Name == "App" {
		s.types["Session"] = true
	}
	return s
}

// table indexes table declaration d: a type, a table and its columns.
func (s *moduleScope) table(d *syntax.TableDecl) {
	s.types[d.Name] = true
	s.tables[d.Name] = true
	cols := map[string]bool{}
	for _, f := range d.Fields {
		cols[f.Name] = true
	}
	s.columns[d.Name] = cols
}

func isUpper(s string) bool { return len(s) > 0 && s[0] >= 'A' && s[0] <= 'Z' }

type Scope struct {
	parent *Scope
	names  map[string]bool
}

func (s *Scope) child() *Scope { return &Scope{parent: s, names: map[string]bool{}} }
func (s *Scope) bind(n string) { s.names[n] = true }
func (s *Scope) has(n string) bool {
	for c := s; c != nil; c = c.parent {
		if c.names[n] {
			return true
		}
	}
	return false
}

type resolver struct {
	p   *Program
	bag *diag.Bag
	m   *syntax.Module
	ms  *moduleScope
	// reported holds the modules already reported as not imported (E0201), so each is reported once.
	reported map[string]bool
	// fixed holds the modules an `import X` fix was already offered for, so it is offered once.
	fixed map[string]bool
	// missing holds the imports of this module that name no module: their error at the `import`
	// stands for every use of the name (message 3), which is then not reported again.
	missing map[string]bool
	// unqual holds the imports of this module with no usable qualifier: their E0206 stands for
	// every use of the module, which is then not reported again.
	unqual map[string]bool
	// inTest is set while a `test` body is resolved.
	inTest bool
}

func (p *Program) Resolve(bag *diag.Bag) {
	for _, name := range p.Order {
		p.scopes[name] = p.index(p.Modules[name])
	}
	for _, name := range p.Std {
		p.scopes[name.Name] = p.index(name)
	}
	p.missingImports = map[string]string{}
	for _, name := range p.Order {
		for _, im := range p.Modules[name].Imports {
			_, inApp := p.Modules[im.Module]
			_, inStd := p.Std[im.Module]
			if _, seen := p.missingImports[im.Module]; !seen && !inApp && !inStd {
				p.missingImports[im.Module] = name
			}
		}
	}
	p.ctorIndex = map[string][]ctorOwner{}
	p.tableIndex = map[string][]string{}
	for _, name := range p.Order {
		s := p.scopes[name]
		for c, typ := range s.ctors {
			if typ != "Msg" { // a page's messages are its own; no import makes them usable elsewhere
				p.ctorIndex[c] = append(p.ctorIndex[c], ctorOwner{Module: name, Type: typ})
			}
		}
		for b := range s.brands {
			p.ctorIndex[b] = append(p.ctorIndex[b], ctorOwner{Module: name, Type: b})
		}
		for t := range s.tables {
			if _, fake := s.fakeTables[t]; t != lastSegment(name) && !fake {
				p.tableIndex[t] = append(p.tableIndex[t], name)
			}
		}
	}
	for c := range p.ctorIndex {
		sort.Slice(p.ctorIndex[c], func(i, j int) bool { return p.ctorIndex[c][i].Module < p.ctorIndex[c][j].Module })
	}
	for t := range p.tableIndex {
		sort.Strings(p.tableIndex[t])
	}
	for _, name := range p.Order {
		r := &resolver{p: p, bag: bag, m: p.Modules[name], ms: p.scopes[name]}
		r.module()
	}
	// stdlib signatures: resolve their type names so the checker can read them
	var std []string
	for name := range p.Std {
		std = append(std, name)
	}
	sort.Strings(std)
	for _, name := range std {
		r := &resolver{p: p, bag: bag, m: p.Std[name], ms: p.scopes[name]}
		r.module()
	}
}

// importNames lists the valid imported module names (paths) in declaration order: not an import
// that names no module or has no usable qualifier.
func (r *resolver) importNames() []string {
	var out []string
	seen := map[string]bool{}
	for _, im := range r.m.Imports {
		if q := r.ms.quals[im.Module]; q != "" && r.ms.imports[q] != nil && !seen[im.Module] {
			seen[im.Module] = true
			out = append(out, im.Module)
		}
	}
	return out
}

func (r *resolver) module() {
	for _, im := range r.m.Imports {
		_, inApp := r.p.Modules[im.Module]
		_, inStd := r.p.Std[im.Module]
		if !inApp && !inStd {
			if r.missing == nil {
				r.missing = map[string]bool{}
			}
			r.missing[im.Module] = true
			if q := r.ms.quals[im.Module]; q != "" { // a use through its qualifier is silent too
				r.missing[q] = true
				delete(r.ms.imports, q)
			}
			if owner, table, ok := r.tableInOtherModule(im.Module); ok {
				if s := r.p.scopes[owner]; s != nil && s.fakeTables[table] != "" { // `import Mail.Sent`
					d := r.bag.Error(diag.UnknownName, im.Span, "`%s` is a table, not a module: `%s`", im.Module, importText(owner, r.qualFor(owner)))
					if !r.imported(owner) && r.p.importPath(owner, r.m.Name) == nil {
						d.Fix = diag.ReplaceSpan(im.Span, importText(owner, r.qualFor(owner)))
						if r.fixed == nil {
							r.fixed = map[string]bool{}
						}
						r.fixed[owner] = true // a use of the table offers no second `import`
					}
					continue
				}
				r.tableMisplaced(im.Span, owner, table)
				continue
			}
			r.bag.Error(diag.UnknownName, im.Span, "there is no `%s` (or `%s/…`): write the module or remove the import", modulePath(im.Module)+".alder", modulePath(im.Module))
		}
	}
	r.aliases()
	for _, d := range r.m.Decls {
		switch d := d.(type) {
		case *syntax.ValueDecl:
			if d.Sig != nil {
				r.typ(d.Sig)
			}
			sc := (&Scope{names: map[string]bool{}}).child()
			for _, prm := range d.Params {
				sc.bind(prm.Name)
			}
			if d.Body != nil {
				r.expr(d.Body, sc)
			}
		case *syntax.TypeDecl:
			for _, c := range d.Union {
				if c.Arg != nil {
					r.typ(c.Arg)
				}
			}
			if d.Record != nil {
				r.recordType(d.Record)
			}
			if d.Brand != nil && d.Brand.Check != nil {
				r.expr(d.Brand.Check, &Scope{names: map[string]bool{}})
			}
		case *syntax.PageDecl:
			r.recordType(d.Params)
		case *syntax.MsgDecl:
			if d.Params != nil {
				r.recordType(d.Params)
			}
			if d.Form != "" && !r.ms.types[d.Form] {
				r.bag.Error(diag.UnknownName, d.Span, "unknown form `%s`", d.Form)
			}
		case *syntax.FormDecl:
			r.fields(d.Fields)
		case *syntax.TableDecl:
			r.fields(d.Fields)
		case *syntax.CapDecl:
			r.fields(d.Entries)
			if d.Name == lastSegment(r.m.Name) {
				for _, e := range d.Entries {
					if r.ms.values[e.Name] {
						r.bag.Error(diag.Duplicate, e.Span, "`%s` is both a value of `%s` and an entry of capability `%s`", e.Name, r.m.Name, d.Name)
					}
				}
			}
		case *syntax.FakeDecl:
			r.fake(d)
		case *syntax.SessionDecl:
			r.fields(d.Fields)
		case *syntax.LoginDecl:
			if !r.p.IsPageModule(d.Page) {
				r.bag.Error(diag.UnknownName, d.Span, "unknown page `%s`", d.Page)
			}
		case *syntax.ChooseDecl:
			if !r.p.IsPageModule(d.Page) {
				r.bag.Error(diag.UnknownName, d.Span, "unknown page `%s`", d.Page)
			}
		case *syntax.TestDecl:
			r.inTest = true
			r.expr(d.Body, &Scope{names: map[string]bool{}})
			r.inTest = false
		}
	}
	for _, name := range r.importNames() {
		if !r.ms.used[name] {
			r.bag.Warn(diag.UnusedImport, r.ms.imports[r.ms.quals[name]].Span, "import `%s` is unused", name)
		}
	}
}

// aliases checks that each import is written in its canonical form (design rule 4): with no
// `as` when its qualifier is its last segment, else `as` the qualifier. Any other alias is
// E0204, with a fix rewriting the line; an import with no usable qualifier is E0206. An import
// that names no module was reported above.
func (r *resolver) aliases() {
	for _, im := range r.m.Imports {
		if r.missing[im.Module] {
			continue
		}
		q := r.ms.quals[im.Module]
		if q == "" {
			if r.unqual == nil {
				r.unqual = map[string]bool{}
			}
			r.unqual[im.Module] = true
			r.bag.Error(diag.NoQualifier, im.Span, "import `%s` has no usable qualifier: %s", im.Module, r.whyNoQualifier(im.Module))
			continue
		}
		want := ""
		if NeedsAlias(im.Module, q) {
			want = q
		}
		if im.Alias == want {
			continue
		}
		line := importText(im.Module, q)
		var d *diag.Diagnostic
		switch {
		case im.Alias == "":
			d = r.bag.Error(diag.ImportAlias, im.Span, "import `%s` needs `as %s`: %s; write `%s`", im.Module, q, r.whyAlias(im.Module), line)
		case want == "":
			d = r.bag.Error(diag.ImportAlias, im.Span, "import `%s` needs no alias: it is already named `%s`; write `%s`", im.Module, q, line)
		default:
			d = r.bag.Error(diag.ImportAlias, im.Span, "import `%s` must be named `%s`, not `%s` (one spelling per module); write `%s`", im.Module, q, im.Alias, line)
		}
		d.Fix = diag.ReplaceSpan(im.Span, line)
	}
}

// whyNoQualifier says why path has no qualifier here and what to rename. The last candidate is
// the whole path joined: it is a declaration of this module, `App` or a standard-library module,
// or else it is another import's.
func (r *resolver) whyNoQualifier(path string) string {
	join := strings.ReplaceAll(path, ".", "")
	local := Taken(r.m, func(string) bool { return false })
	switch {
	case join == "App":
		return fmt.Sprintf("`%s` is the app module; rename the module", join)
	case r.p.isStd(join):
		return fmt.Sprintf("`%s` is a standard-library module; rename the module", join)
	case local(join):
		return fmt.Sprintf("this module declares `%s`; rename that declaration (or the module)", join)
	}
	return "its qualifier would be another import's; rename the module (or the one it collides with)"
}

// isUnqual reports whether x, a module as written (its full path or any trailing run of its
// segments joined, the spellings a qualifier takes), names an import with no usable qualifier,
// whose E0206 stands for the reference.
func (r *resolver) isUnqual(x string) bool {
	if r.unqual[x] {
		return true
	}
	if r.p.Modules[x] != nil {
		return false // a real module of that name is its own: it is simply not imported
	}
	for path := range r.unqual {
		segs := strings.Split(path, ".")
		for i := range segs {
			if strings.Join(segs[i:], "") == x {
				return true
			}
		}
	}
	return false
}

// whyAlias says why path's last segment cannot be its qualifier here.
func (r *resolver) whyAlias(path string) string {
	last := lastSegment(path)
	for _, im := range r.m.Imports {
		if im.Module != path && lastSegment(im.Module) == last {
			return fmt.Sprintf("import `%s` also ends in `%s`", im.Module, last)
		}
	}
	switch {
	case last == lastSegment(r.m.Name):
		return fmt.Sprintf("`%s` is this module's own name", last)
	case last == "App":
		return "`App` is the app module"
	case r.p.isStd(last):
		return fmt.Sprintf("`%s` is a standard-library module", last)
	}
	return fmt.Sprintf("this module declares `%s`", last)
}

// importText is the import line of module x with qualifier q: `import x`, or `import x as q`.
func importText(x, q string) string {
	if q != x && NeedsAlias(x, q) {
		return "import " + x + " as " + q
	}
	return "import " + x
}

// imported reports whether module x (a path) is imported here, with a usable qualifier.
func (r *resolver) imported(x string) bool {
	q := r.ms.quals[x]
	return q != "" && r.ms.imports[q] != nil
}

// qualFor is the qualifier module x has here: its import's, or, when x is not imported, the
// canonical qualifier it would have beside this module's imports (x itself when it would have
// none). Text that tells the reader to write a name of x uses it.
func (r *resolver) qualFor(x string) string {
	if q := r.ms.quals[x]; q != "" {
		return q
	}
	if r.p.isStd(x) {
		return x
	}
	paths := []string{x}
	for _, im := range r.m.Imports {
		if !r.p.isStd(im.Module) {
			paths = append(paths, im.Module)
		}
	}
	quals, _ := CanonicalQualifiers(r.m.Name, paths, Taken(r.m, r.p.isStd))
	if q := quals[x]; q != "" {
		return q
	}
	return x
}

// fake resolves `fake Cap { … }`: its name as a capability of this module or an imported one
// (the checker reports E0020 for one declared elsewhere), its tables as the module's tables,
// and its entries' bodies like values', each parameter in scope.
func (r *resolver) fake(d *syntax.FakeDecl) {
	if ref, ok := r.capability("", d.Name, d.NameSpan); ok {
		d.Ref = ref
	} else {
		r.bag.Error(diag.UnknownName, d.NameSpan, "`fake %s`: there is no capability `%s`; a fake implements a capability declared in its module (`capability %s { … }`)", d.Name, d.Name, d.Name)
	}
	for _, t := range d.Tables {
		r.fields(t.Fields)
	}
	for _, e := range d.Entries {
		sc := (&Scope{names: map[string]bool{}}).child()
		for _, prm := range e.Params {
			sc.bind(prm.Name)
		}
		r.expr(e.Body, sc)
	}
}

// capability finds capability name, written `name` or `mod.name` (mod this module's name or an
// import's qualifier): declared in this module or, unqualified, in an imported one. Its Ref is
// RefCapability with Name "Module.Cap". A name two imported modules declare is reported at span
// (found, with no Ref).
func (r *resolver) capability(mod, name string, span diag.Span) (syntax.Ref, bool) {
	ref := func(m string) syntax.Ref {
		return syntax.Ref{Kind: syntax.RefCapability, Module: m, Name: m + "." + name}
	}
	switch {
	case mod == "" || mod == r.m.Name:
		if r.ms.caps[name] {
			return ref(r.m.Name), true
		}
		if mod != "" {
			return syntax.Ref{}, false
		}
		if owners := r.importedCapOwners(name); len(owners) > 1 {
			r.ambiguousCapability(span, name, owners)
			return syntax.Ref{}, true
		} else if len(owners) == 1 {
			r.ms.used[owners[0]] = true
			return ref(owners[0]), true
		}
	default:
		if im, ok := r.ms.imports[mod]; ok {
			if s := r.p.scopes[im.Module]; s != nil && s.caps[name] {
				r.ms.used[im.Module] = true
				return ref(im.Module), true
			}
		}
	}
	return syntax.Ref{}, false
}

// importedCapOwners is the imported modules declaring capability name, in import order.
func (r *resolver) importedCapOwners(name string) []string {
	var out []string
	for _, x := range r.importNames() {
		if s := r.p.scopes[x]; s != nil && s.caps[name] {
			out = append(out, x)
		}
	}
	return out
}

// ambiguousCapability reports capability name, written unqualified, that several imported
// modules declare: the use must name the module. Each such module counts as used.
func (r *resolver) ambiguousCapability(span diag.Span, name string, owners []string) {
	owners = append([]string(nil), owners...)
	sort.Strings(owners)
	var of, alts []string
	for _, o := range owners {
		r.ms.used[o] = true
		of = append(of, "`"+o+"`")
		alts = append(alts, "`"+r.qualFor(o)+"."+name+"`")
	}
	both, or := "both ", alts[0]+" or "+alts[1]
	if len(owners) > 2 {
		both = ""
		or = strings.Join(alts[:len(alts)-1], ", ") + " or " + alts[len(alts)-1]
	}
	list := strings.Join(of[:len(of)-1], ", ") + " and " + of[len(of)-1]
	r.bag.Error(diag.UnknownName, span, "`%s` is a capability of %s%s: write %s", name, both, list, or)
}

// testFakeArg resolves the argument of `Test.fake`: a capability name, `Stripe` or, through its
// module's qualifier, `Payments.Stripe`. Any other argument is resolved as an expression (the
// checker refuses it).
func (r *resolver) testFakeArg(a syntax.Expr, sc *Scope) {
	ct, ok := a.(*syntax.CtorRef)
	if !ok {
		r.expr(a, sc)
		return
	}
	mod, name := ct.Module, ct.Name
	if mod != "" {
		if short, path, ok := r.fullPath(mod+"."+name, ct.Span.Start); ok { // `A.B.Clock`
			mod, name = r.splitShort(short, path)
		}
	}
	if ref, found := r.capability(mod, name, ct.Span); found {
		ct.Ref = ref
		return
	}
	full := ct.Name
	if ct.Module != "" {
		full = ct.Module + "." + ct.Name
		if _, imported := r.ms.imports[mod]; !imported && mod != r.m.Name && r.p.Modules[mod] != nil {
			r.notImported(ct.Span, mod)
			return
		}
	}
	r.bag.Error(diag.UnknownName, ct.Span, "`Test.fake %s`: there is no capability `%s` in scope", full, full)
}

// testFormArg resolves the first argument of `Test.submit` and `Test.submitRaw`: a form, `NewTodo`
// (of this module) or `Todos.Index.NewTodo` (of any module, imported or not, as a page needs no
// import), resolved as a type; or a message posting one, resolved as testClickArg does. A form
// of another module written unqualified, or through a page import's qualifier
// (`Login.Credentials`), gets the full name as a fix. Any other argument is resolved as an
// expression (the checker refuses it).
func (r *resolver) testFormArg(a syntax.Expr, step string, sc *Scope) {
	ct, ok := a.(*syntax.CtorRef)
	if !ok {
		r.testClickArg(a, sc) // a message with its parameters: `(Save { id: id })`
		return
	}
	if ct.Module == "" && r.ms.types[ct.Name] { // a form, or another type (the checker refuses it)
		ct.Ref = syntax.Ref{Kind: syntax.RefType, Module: r.m.Name, Name: ct.Name}
		return
	}
	if ct.Module != "" {
		if path, ok := r.pageByQualifier(ct.Module); ok {
			if s := r.p.scopes[path]; s != nil && s.forms[ct.Name] {
				r.pageMemberShort(ct, path, syntax.RefType, "form")
				return
			}
		}
		path := ct.Module // a page's full path, or the qualifier of an imported non-page module
		if im, ok := r.ms.imports[path]; ok {
			path = im.Module
		}
		if s := r.p.scopes[path]; s != nil && r.p.Modules[path] != nil && s.forms[ct.Name] {
			ct.Ref = syntax.Ref{Kind: syntax.RefType, Module: path, Name: ct.Name}
			return
		}
	} else {
		var owners []string
		for _, x := range r.p.Order {
			if s := r.p.scopes[x]; s != nil && s.forms[ct.Name] {
				owners = append(owners, x)
			}
		}
		if len(owners) > 0 {
			d := r.bag.Error(diag.UnknownName, ct.Span, "`%s` is a form of `%s`: write `%s.%s`", ct.Name, owners[0], owners[0], ct.Name)
			d.Fix = diag.ReplaceSpan(ct.Span, owners[0]+"."+ct.Name)
			return
		}
	}
	r.testClickArg(a, sc) // a message: `Save`, `App.Send`, `Todos.Index.Save`
}

// testClickArg resolves the argument of `Test.click`: a message, `Toggle { id: id }` or
// `App.Logout`, or from another module a page's message written with its page,
// `Todos.Index.Toggle { id: id }` (a page's messages are otherwise its own; through the page
// import's qualifier, `Index.Toggle`, it gets the full name as a fix). Anything else is
// resolved as an expression.
func (r *resolver) testClickArg(a syntax.Expr, sc *Scope) {
	head, rest := a, []syntax.Expr(nil)
	if app, ok := a.(*syntax.App); ok {
		head, rest = app.Fn, app.Args
	}
	ct, ok := head.(*syntax.CtorRef)
	if ok && ct.Module != "" {
		if path, isPage := r.pageByQualifier(ct.Module); isPage && path != r.m.Name {
			if s := r.p.scopes[path]; s != nil && s.ctors[ct.Name] == "Msg" {
				r.pageMemberShort(ct, path, syntax.RefCtor, "message")
				for _, x := range rest {
					r.expr(x, sc)
				}
				return
			}
		}
	}
	if !ok || ct.Module == "" || ct.Module == r.m.Name || !r.p.IsPageModule(ct.Module) {
		r.expr(a, sc)
		return
	}
	if s := r.p.scopes[ct.Module]; s != nil && s.ctors[ct.Name] == "Msg" {
		ct.Ref = syntax.Ref{Kind: syntax.RefCtor, Module: ct.Module, Name: ct.Name}
		for _, x := range rest {
			r.expr(x, sc)
		}
		return
	}
	r.expr(a, sc)
}

// pageByQualifier reports the path of the imported page module whose qualifier is q, when q is
// not that path itself (`Login` for `import Auth.Login`).
func (r *resolver) pageByQualifier(q string) (string, bool) {
	im, ok := r.ms.imports[q]
	if !ok || im.Module == q || !r.p.IsPageModule(im.Module) {
		return "", false
	}
	return im.Module, true
}

// pageMemberShort reports a page's form or message written through the page import's qualifier
// in a test step (`Test.submit Login.Credentials`): like page values, a page's forms and
// messages are written with its full path (design rule 6). The fix writes the full path, and
// the reference resolves to the member so nothing else is reported.
func (r *resolver) pageMemberShort(ct *syntax.CtorRef, path string, kind syntax.RefKind, what string) {
	r.ms.used[path] = true // one error, not a W0001 besides
	full := path + "." + ct.Name
	d := r.bag.Error(diag.UnknownName, ct.Span, "write `%s`: a page's %ss are written with its full path, like the page itself", full, what)
	d.Fix = diag.ReplaceSpan(ct.Span, full)
	ct.Ref = syntax.Ref{Kind: kind, Module: path, Name: ct.Name}
}

func (r *resolver) fields(fs []*syntax.Field) {
	for _, f := range fs {
		r.typ(f.Type)
		if f.Default != nil {
			r.expr(f.Default, &Scope{names: map[string]bool{}})
		}
	}
}
func (r *resolver) recordType(rt *syntax.RecordType) {
	if rt != nil {
		r.fields(rt.Fields)
	}
}

// importedScope returns the module qualifier q names, its full path and its scope: the import
// whose canonical qualifier is q (marked used), or the stdlib module q.
func (r *resolver) importedScope(q string) (string, *moduleScope, bool) {
	if im, ok := r.ms.imports[q]; ok {
		if s := r.p.scopes[im.Module]; s != nil {
			r.ms.used[im.Module] = true
			return im.Module, s, true
		}
		return "", nil, false
	}
	if r.p.isStd(q) {
		if s := r.p.scopes[q]; s != nil {
			return q, s, true
		}
	}
	return "", nil, false
}

// fullPath handles a name written with the full path of an imported module whose qualifier is
// not that path (design rule 5): `Table.Todo.find`, `A.B.Clock.now`, or `Table.Todo` for its
// eponymous declaration. It reports E0205 at each of starts (where the written name begins in
// the source; an element's closing tag is a second place), with a fix writing the qualifier in
// the path's place, and returns the name as the qualifier writes it and the module's path; the
// caller resolves that short name, so nothing else is reported. Only the longest prefix that is
// a module counts. A page value's full path is never E0205: callers resolve page values (design
// rule 6) before calling it.
func (r *resolver) fullPath(written string, starts ...diag.Pos) (short, path string, ok bool) {
	for _, im := range r.m.Imports {
		p, q := im.Module, r.ms.quals[im.Module]
		if q == "" || q == p || r.ms.imports[q] == nil || len(p) <= len(path) {
			continue // no qualifier, or a single-segment path: its qualifier is its name
		}
		if written == p || strings.HasPrefix(written, p+".") {
			path = p // the longest such path: `A.B.C` over `A.B`
		}
	}
	if path == "" {
		return "", "", false
	}
	// a longer prefix that is itself a module (`Admin.Users.Util` beside `import Admin.Users`,
	// or the page `Admin.Users.Edit`) is the module the name means: not a full path of path
	for i := len(path) + 1; i <= len(written); i++ {
		if i == len(written) || written[i] == '.' {
			if _, isMod := r.p.Modules[written[:i]]; isMod {
				return "", "", false
			}
		}
	}
	q := r.ms.quals[path]
	short = q + written[len(path):]
	for _, st := range starts {
		sp := diag.Span{File: r.m.File, Start: st, End: diag.Pos{Line: st.Line, Col: st.Col + len(written)}}
		d := r.bag.Error(diag.FullPath, sp, "write `%s`, not `%s`: an imported module is named by its qualifier, `%s` for `%s`", short, written, q, path)
		if r.textAt(st, written) {
			d.Fix = diag.ReplaceSpan(sp, short)
		}
	}
	return short, path, true
}

// splitShort splits a name fullPath shortened into its module part and name. The qualifier
// alone (`Todo` for `Table.Todo`) names the module's eponymous declaration (design rule 2): it
// splits as the qualifier and the path's last segment.
func (r *resolver) splitShort(short, path string) (mod, name string) {
	if i := strings.LastIndexByte(short, '.'); i >= 0 {
		return short[:i], short[i+1:]
	}
	return short, lastSegment(path)
}

// textAt reports whether the source has text at pos (on one line).
func (r *resolver) textAt(pos diag.Pos, text string) bool {
	if pos.Line < 1 || pos.Line > len(r.m.Lines) {
		return false
	}
	line, i := r.m.Lines[pos.Line-1], pos.Col-1
	return i >= 0 && i+len(text) <= len(line) && line[i:i+len(text)] == text
}

// stdShadowed reports whether x is the name of an app module refused by E0017 (the stdlib has
// a module named x). A name the stdlib's x lacks is then the app module's: rather than report
// it, stdShadowed marks the module using it failed (MarkFailed), so it is not checked either.
func (r *resolver) stdShadowed(x string) bool {
	if !r.p.stdNamed[x] {
		return false
	}
	r.p.quiet[r.m.Name] = true
	return true
}

// modulePath is the file path of module x without its extension: `Blog.Show` → `app/Blog/Show`.
func modulePath(x string) string { return "app/" + strings.ReplaceAll(x, ".", "/") }

// notImported reports x.name used without `import x`, once per module x. A name of an import
// that names no module was reported at the import and is not reported again; a module that
// does not exist but names a table of another module gets the table's message; and when x
// imports this module, directly or not, the import cannot be added, so none is offered.
func (r *resolver) notImported(span diag.Span, x string) {
	if r.missing[x] || r.isUnqual(x) || r.reported[x] {
		return
	}
	if r.reported == nil {
		r.reported = map[string]bool{}
	}
	r.reported[x] = true
	if owner, table, ok := r.tableInOtherModule(x); ok {
		r.tableMisplaced(span, owner, table)
		return
	}
	if by, ok := r.p.missingImports[x]; ok && r.p.Modules[x] == nil {
		r.bag.Error(diag.UnknownName, span, "there is no `%s.alder` (imported in `%s`)", modulePath(x), by)
		return
	}
	x = r.p.moduleByLastSegment(x)
	if path := r.p.importPath(x, r.m.Name); path != nil {
		msg := fmt.Sprintf("module `%s` is not imported, and it %s, so `%s` cannot import it", x, importsPhrase(path), r.m.Name)
		if r.inTest {
			msg += fmt.Sprintf("; a test of `%s` belongs in `%s.alder`", x, modulePath(x))
		}
		r.bag.Error("E0201", span, "%s", msg)
		return
	}
	d := r.bag.Error("E0201", span, "module `%s` is not imported", x)
	d.Fix = r.importFix(x)
}

// moduleByLastSegment is the full path of the one app module whose last segment is x
// (`Table.Todo` for `Todo`), when x itself names no module; x otherwise.
func (p *Program) moduleByLastSegment(x string) string {
	if p.Modules[x] != nil || strings.Contains(x, ".") {
		return x
	}
	found := ""
	for _, name := range p.Order {
		if lastSegment(name) == x {
			if found != "" {
				return x
			}
			found = name
		}
	}
	if found == "" {
		return x
	}
	return found
}

// importFix inserts `import x` after the last import, once per module x, in its canonical form
// (`import x as Q` when x needs an alias beside this module's imports); nil when it was already
// offered.
func (r *resolver) importFix(x string) *diag.Fix {
	if r.fixed[x] {
		return nil
	}
	if r.fixed == nil {
		r.fixed = map[string]bool{}
	}
	r.fixed[x] = true
	line := r.m.Span.Start.Line + 1 // after the module line (comments may come before it)
	if n := len(r.m.Imports); n > 0 {
		line = r.m.Imports[n-1].Span.End.Line + 1
	}
	return diag.InsertAt(line, 1, importText(x, r.qualFor(x))+"\n")
}

// importPath is the chain of imports from app module from to module to (from first, to
// last), or nil when from does not import to, directly or through other modules.
func (p *Program) importPath(from, to string) []string {
	seen := map[string]bool{}
	var walk func(string) []string
	walk = func(name string) []string {
		if seen[name] {
			return nil
		}
		seen[name] = true
		m := p.Modules[name]
		if m == nil {
			return nil
		}
		for _, im := range m.Imports {
			if im.Module == to {
				return []string{name, to}
			}
			if rest := walk(im.Module); rest != nil {
				return append([]string{name}, rest...)
			}
		}
		return nil
	}
	return walk(from)
}

// importsPhrase: "imports `M`", or "imports `M` through `B`" when the chain is longer.
func importsPhrase(path []string) string {
	out := "imports `" + path[len(path)-1] + "`"
	if mid := path[1 : len(path)-1]; len(mid) > 0 {
		out += " through `" + strings.Join(mid, "` and `") + "`"
	}
	return out
}

// tableInOtherModule: x names no module, but is a table declared in a module not named after
// it: `Post.Comment` for `table Comment` in module `Post`, or `Comment` when exactly one module
// declares it.
func (r *resolver) tableInOtherModule(x string) (owner, table string, ok bool) {
	if _, isMod := r.p.Modules[x]; isMod {
		return "", "", false
	}
	if _, isStd := r.p.Std[x]; isStd {
		return "", "", false
	}
	if i := strings.LastIndex(x, "."); i > 0 {
		owner, table = x[:i], x[i+1:]
		if s, isApp := r.p.Modules[owner]; isApp && s != nil && r.p.scopes[owner].tables[table] && lastSegment(owner) != table {
			return owner, table, true
		}
		return "", "", false
	}
	if owners := r.p.tableIndex[x]; len(owners) == 1 {
		return owners[0], x, true
	}
	return "", "", false
}

// tableMisplaced reports a use of a table declared in another table's module (E5 problem 2):
// such a table's columns cannot be named from anywhere, so no import can help.
func (r *resolver) tableMisplaced(span diag.Span, owner, table string) {
	if s := r.p.scopes[owner]; s != nil {
		if fake, ok := s.fakeTables[table]; ok { // a use from a module that does not import owner
			if path := r.p.importPath(owner, r.m.Name); path != nil {
				r.bag.Error(diag.UnknownName, span, "`%s` is a table of `fake %s` in module `%s`, and `%s` %s, so `%s` cannot import it; %s", table, fake, owner, owner, importsPhrase(path), r.m.Name, r.cycleRemedy(owner))
				return
			}
			q := r.qualFor(owner)
			d := r.bag.Error(diag.UnknownName, span, "`%s` is a table of `fake %s` in module `%s`: add `%s`, then write `%s.%s` (columns: `%s.%s.column`)", table, fake, owner, importText(owner, q), q, table, q, table)
			d.Fix = r.importFix(owner)
			return
		}
	}
	r.bag.Error(diag.UnknownName, span, "`%s` is a table of module `%s`, but a table is declared in a module named after it: move it to `app/Table/%s.alder`%s", table, owner, table, r.p.tableDeclSee(owner, table))
}

// tableDeclSee points at the E0018 declaration of table in module owner: "; see `app/Post.alder:11`".
func (p *Program) tableDeclSee(owner, table string) string {
	if m := p.Modules[owner]; m != nil {
		for _, d := range m.Decls {
			if t, ok := d.(*syntax.TableDecl); ok && t.Name == table {
				return fmt.Sprintf(" (see `%s:%d`)", t.Span.File, t.Span.Start.Line)
			}
		}
	}
	return ""
}

// ---- types

func (r *resolver) typ(t syntax.Type) {
	switch t := t.(type) {
	case *syntax.TypeName:
		for _, a := range t.Args {
			r.typ(a)
		}
		if t.Module != "" {
			if t.Module == r.m.Name && r.ms.types[t.Name] {
				t.Ref = syntax.Ref{Kind: syntax.RefType, Module: t.Module, Name: t.Name}
				return
			}
			if r.p.IsPageModule(t.Module + "." + t.Name) { // a page: always its full path (design rule 6)
				t.Ref = syntax.Ref{Kind: syntax.RefPage, Module: t.Module + "." + t.Name}
				return
			}
			mod, name := t.Module, t.Name
			if short, path, ok := r.fullPath(mod+"."+name, t.Span.Start); ok {
				mod, name = r.splitShort(short, path)
			}
			if path, s, ok := r.importedScope(mod); ok {
				if s.types[name] {
					t.Ref = syntax.Ref{Kind: syntax.RefType, Module: path, Name: name}
					return
				}
				if !r.stdShadowed(path) {
					r.bag.Error(diag.UnknownName, t.Span, "module `%s` has no type `%s`", path, name)
				}
				return
			}
			if r.p.IsPageModule(t.Module) {
				t.Ref = syntax.Ref{Kind: syntax.RefPage, Module: t.Module + "." + t.Name}
				return
			}
			if t.Module == "App" { // App needs no import; only App.Msg is a usable type
				switch t.Name {
				case "Msg": // the type of app-level messages
					t.Ref = syntax.Ref{Kind: syntax.RefType, Module: "App", Name: "Msg"}
				case "Session":
					d := r.bag.Error(diag.UnknownName, t.Span, "`Session` is not qualified; write `Session`")
					d.Fix = diag.ReplaceSpan(t.Span, "Session")
				default:
					r.bag.Error(diag.UnknownName, t.Span, "`App` has no type `%s` other modules can use; only `App.Msg` is global", t.Name)
				}
				return
			}
			if t.Module == r.m.Name { // not one of its types (those resolved above)
				r.ownQualified(t.Span, t.Name, false, false)
				return
			}
			r.notImported(t.Span, t.Module)
			return
		}
		if r.ms.types[t.Name] {
			t.Ref = syntax.Ref{Kind: syntax.RefType, Module: r.m.Name, Name: t.Name}
			return
		}
		if t.Name == "Capability" && r.m == r.p.Std["Test"] {
			// `fake : Capability -> Task Unit`: Test.fake is a special form the checker types
			// itself; `Capability` names no type, here or in any program
			return
		}
		// module-name rule (design rule 2): an import's qualifier alone names the module's type
		// named like its last segment (`Todo` for `import Table.Todo`)
		if path, s, ok := r.importedScope(t.Name); ok && s.types[lastSegment(path)] {
			t.Ref = syntax.Ref{Kind: syntax.RefType, Module: path, Name: lastSegment(path)}
			return
		}
		switch {
		case generatedTypes[t.Name]:
			mod := r.m.Name
			if t.Name == "Session" {
				mod = "App"
			}
			t.Ref = syntax.Ref{Kind: syntax.RefType, Module: mod, Name: t.Name}
		case preludeTypes[t.Name]:
			t.Ref = syntax.Ref{Kind: syntax.RefType, Module: "Prelude", Name: t.Name}
		case r.p.IsPageModule(t.Name):
			t.Ref = syntax.Ref{Kind: syntax.RefPage, Module: t.Name}
		default:
			// unqualified type that lives in an imported module → E0202 with fix
			for _, x := range r.importNames() {
				if r.p.scopes[x] != nil && r.p.scopes[x].types[t.Name] {
					r.ms.used[x] = true
					if _, fake := r.p.scopes[x].fakeTables[t.Name]; r.p.scopes[x].tables[t.Name] && lastSegment(x) != t.Name && r.p.Modules[x] != nil && !fake {
						r.bag.Error("E0202", t.Span, "type `%s` belongs to module `%s`, but `%s` is a table, and a table is declared in a module named after it: move it to `app/Table/%s.alder`%s", t.Name, x, t.Name, t.Name, r.p.tableDeclSee(x, t.Name))
						return
					}
					q := r.qualFor(x)
					write := q + "." + t.Name
					if t.Name == lastSegment(x) {
						write = q // the eponymous type is written by its qualifier alone
					}
					d := r.bag.Error("E0202", t.Span, "type `%s` belongs to module `%s`; write `%s`", t.Name, x, write)
					d.Fix = diag.ReplaceSpan(t.Span, write)
					return
				}
			}
			// any app module declaring it → not imported
			var cands []string
			for x, s := range r.p.scopes {
				if _, isStd := r.p.Std[x]; !isStd && s.types[t.Name] {
					cands = append(cands, x)
				}
			}
			sort.Strings(cands)
			if len(cands) > 0 {
				r.notImported(t.Span, cands[0])
				return
			}
			// a type of a stdlib module other than Prelude (`Http.HttpError`) is always qualified
			var stds []string
			for x, s := range r.p.scopes {
				if _, isStd := r.p.Std[x]; isStd && x != "Prelude" && s.types[t.Name] {
					stds = append(stds, x)
				}
			}
			sort.Strings(stds)
			if len(stds) > 0 {
				d := r.bag.Error("E0202", t.Span, "type `%s` belongs to module `%s`; write `%s.%s`", t.Name, stds[0], stds[0], t.Name)
				d.Fix = diag.ReplaceSpan(t.Span, stds[0]+"."+t.Name)
				return
			}
			if len(r.missing) > 0 { // the import that names no module stands for it
				return
			}
			if by, ok := r.p.missingImports[t.Name]; ok {
				r.bag.Error(diag.UnknownName, t.Span, "there is no `%s.alder` (imported in `%s`)", modulePath(t.Name), by)
				return
			}
			r.bag.Error(diag.UnknownName, t.Span, "unknown type `%s`%s", t.Name, r.missingCause(t.Name))
		}
	case *syntax.RecordType:
		r.recordType(t)
	case *syntax.FunType:
		for _, a := range t.Params {
			r.typ(a)
		}
		r.typ(t.Result)
	case *syntax.TypeVar:
	}
}

// ---- expressions

func (r *resolver) expr(e syntax.Expr, sc *Scope) {
	switch e := e.(type) {
	case nil, *syntax.IntLit, *syntax.FloatLit, *syntax.BoolLit, *syntax.UnitLit, *syntax.HoleExpr:
	case *syntax.StrLit:
		for _, p := range e.Parts {
			if p.Expr != nil {
				r.expr(p.Expr, sc)
			}
		}
	case *syntax.ListLit:
		for _, x := range e.Elems {
			r.expr(x, sc)
		}
	case *syntax.RecordLit:
		for _, f := range e.Fields {
			r.expr(f.Value, sc)
		}
	case *syntax.RecordUpdate:
		r.expr(e.Base, sc)
		for _, f := range e.Fields {
			r.expr(f.Value, sc)
		}
	case *syntax.Var:
		r.variable(e, sc)
	case *syntax.CtorRef:
		r.ctor(e)
	case *syntax.Access:
		r.expr(e.Expr, sc)
	case *syntax.App:
		r.expr(e.Fn, sc)
		for i, a := range e.Args {
			if v, ok := e.Fn.(*syntax.Var); ok && i == 0 && v.Ref.Kind == syntax.RefStd && v.Ref.Module == "Test" {
				switch v.Ref.Name {
				case "fake":
					r.testFakeArg(a, sc)
					continue
				case "submit", "submitRaw":
					r.testFormArg(a, v.Ref.Name, sc)
					continue
				case "click":
					r.testClickArg(a, sc)
					continue
				}
			}
			r.expr(a, sc)
			// table position: the first argument of a `Db.` operation is a table, and a table
			// wins over a constructor of the same name (`Db.from Member` with a `Member` role)
			if v, ok := e.Fn.(*syntax.Var); ok && i == 0 && v.Ref.Kind == syntax.RefStd && v.Ref.Module == "Db" {
				if ct, ok := a.(*syntax.CtorRef); ok && ct.Ref.Kind == syntax.RefCtor && ct.Table.Kind == syntax.RefTable {
					ct.Ref, ct.Table = ct.Table, syntax.Ref{}
				}
			}
		}
	case *syntax.Lambda:
		c := sc.child()
		for i, p := range e.Params {
			if i < len(e.Types) && e.Types[i] != nil {
				r.typ(e.Types[i])
			}
			c.bind(p.Name)
		}
		r.expr(e.Body, c)
	case *syntax.If:
		r.expr(e.Cond, sc)
		r.expr(e.Then, sc)
		r.expr(e.Else, sc)
	case *syntax.Match:
		r.expr(e.Subject, sc)
		for _, a := range e.Arms {
			c := sc.child()
			r.pattern(a.Pat, c)
			r.expr(a.Body, c)
		}
	case *syntax.Block:
		c := sc.child()
		for _, s := range e.Stmts {
			if s.Type != nil {
				r.typ(s.Type)
			}
			r.expr(s.Expr, c)
			c.bind(s.Name)
		}
		r.expr(e.Result, c)
	case *syntax.BinOp:
		r.expr(e.L, sc)
		r.expr(e.R, sc)
	case *syntax.Element:
		r.element(e, sc)
	case *syntax.HtmlText:
	case *syntax.HtmlExpr:
		r.expr(e.Expr, sc)
	case *syntax.HtmlFor:
		r.expr(e.List, sc)
		c := sc.child()
		c.bind(e.Var)
		r.expr(e.Body, c)
	case *syntax.HtmlIf:
		r.expr(e.Cond, sc)
		r.expr(e.Then, sc)
		r.expr(e.Else, sc)
	case *syntax.HtmlMatch:
		r.expr(e.Subject, sc)
		for _, a := range e.Arms {
			c := sc.child()
			r.pattern(a.Pat, c)
			r.expr(a.Body, c)
		}
	}
}

func (r *resolver) variable(v *syntax.Var, sc *Scope) {
	if v.Module == "" {
		switch {
		case sc.has(v.Name):
			v.Ref = syntax.Ref{Kind: syntax.RefLocal, Name: v.Name}
		case r.ms.values[v.Name]:
			v.Ref = syntax.Ref{Kind: syntax.RefValue, Module: r.m.Name, Name: v.Name}
		case r.p.scopes["Prelude"] != nil && r.p.scopes["Prelude"].values[v.Name]:
			v.Ref = syntax.Ref{Kind: syntax.RefStd, Module: "Prelude", Name: v.Name}
		default:
			r.bag.Error(diag.UnknownName, v.Span, "unknown name `%s`", v.Name)
		}
		return
	}
	// Module.value, in the order of design rule 5: an import's qualifier (or a full path, E0205,
	// resolved as its qualifier) or a stdlib module, then this module's capabilities and tables,
	// then `Q.Cap.entry` and `Q.Table.column`
	mod := v.Module
	if short, path, ok := r.fullPath(mod+"."+v.Name, v.Span.Start); ok {
		mod, _ = r.splitShort(short, path)
	}
	if path, s, ok := r.importedScope(mod); ok {
		ep := lastSegment(path) // the name of the module's eponymous table or capability
		switch {
		case s.values[v.Name]:
			kind := syntax.RefValue
			if r.p.isStd(path) {
				kind = syntax.RefStd
			}
			v.Ref = syntax.Ref{Kind: kind, Module: path, Name: v.Name}
		case path == "Fuzz" && v.Name == "any" && r.p.isStd(path):
			// `any : Fuzzer a` is a special form (stdlib/Fuzz.alder): the checker types it and
			// records the type at each use, from which the generator is derived
			v.Ref = syntax.Ref{Kind: syntax.RefStd, Module: path, Name: v.Name}
		case s.tables[ep] && s.columns[ep][v.Name]:
			v.Ref = syntax.Ref{Kind: syntax.RefColumn, Module: path, Name: v.Name}
		case s.capEntries[v.Name]: // `capability M` of module M: Markdown.render
			v.Ref = syntax.Ref{Kind: syntax.RefCapability, Module: path, Name: ep + "." + v.Name}
		default:
			if !r.stdShadowed(path) {
				d := r.bag.Error(diag.UnknownName, v.Span, "module `%s` has no `%s`%s", path, v.Name, entryHint(s, v.Name))
				if c := s.entryCaps[v.Name]; c != "" {
					d.Fix = diag.ReplaceSpan(v.Span, c+"."+v.Name)
				}
			}
		}
		return
	}
	// capability declared in this or an imported module: Password.verify
	if r.ms.caps[mod] {
		v.Ref = syntax.Ref{Kind: syntax.RefCapability, Module: r.m.Name, Name: mod + "." + v.Name}
		return
	}
	if owners := r.importedCapOwners(mod); len(owners) > 1 { // `Clock.now`, A and B declaring Clock
		r.ambiguousCapability(v.Span, mod, owners)
		return
	} else if len(owners) == 1 {
		r.ms.used[owners[0]] = true
		v.Ref = syntax.Ref{Kind: syntax.RefCapability, Module: owners[0], Name: mod + "." + v.Name}
		return
	}
	// this module's own table columns: Todo.done inside module Todo
	if r.ms.tables[mod] && r.ms.columns[mod][v.Name] {
		v.Ref = syntax.Ref{Kind: syntax.RefColumn, Module: r.m.Name, Name: v.Name}
		return
	}
	if mod == r.m.Name {
		r.ownQualified(v.Span, v.Name, r.ms.values[v.Name], true)
		return
	}
	if i := strings.LastIndexByte(mod, '.'); i > 0 {
		owner, table := mod[:i], mod[i+1:]
		// owner is an import's qualifier, or (not imported) a module's path
		path, imported := owner, false
		if im, ok := r.ms.imports[owner]; ok {
			path, imported = im.Module, true
		}
		// an entry of a capability named with its module: B.Clock.now
		if s := r.p.scopes[path]; s != nil && s.caps[table] && r.p.Modules[path] != nil {
			switch {
			case path == r.m.Name:
				r.ownQualified(v.Span, table+"."+v.Name, true, true)
			case imported:
				r.ms.used[path] = true
				v.Ref = syntax.Ref{Kind: syntax.RefCapability, Module: path, Name: table + "." + v.Name}
			default:
				r.notImported(v.Span, path)
			}
			return
		}
		// a fake's table's column from another module: Payments.Charge.cents
		if _, fake := r.p.scopes[path].fakeTablesOf()[table]; fake && imported {
			r.ms.used[path] = true
			if r.p.scopes[path].columns[table][v.Name] {
				v.Ref = syntax.Ref{Kind: syntax.RefColumn, Module: path, Name: v.Name}
				return
			}
			r.bag.Error(diag.UnknownName, v.Span, "table `%s` of `fake %s` has no column `%s`", table, r.p.scopes[path].fakeTables[table], v.Name)
			return
		}
	}
	// a fake's table of an imported module, unqualified: Charge.cents
	if owner, fake, ok := r.importedFakeTable(mod); ok {
		r.bag.Error(diag.UnknownName, v.Span, "`%s` is a table of `fake %s` in module `%s`: write `%s.%s.%s`", mod, fake, owner, r.qualFor(owner), mod, v.Name)
		return
	}
	if mod == "App" { // App needs no import; only its messages are usable values
		if app := r.p.scopes["App"]; v.Name == "update" && r.p.Modules["App"] != nil && app != nil && app.values["update"] {
			r.bag.Error(diag.UnknownName, v.Span, "`App.update` is called by the runtime, not by code; a test of it belongs in `app/App.alder`")
			return
		}
		if v.Name == "login" {
			r.bag.Error(diag.UnknownName, v.Span, "`App` has no `login`; `login` is a declaration, not a value — link to the page with `href={%s {}}`", r.loginPage())
			return
		}
		r.bag.Error(diag.UnknownName, v.Span, "`App` has no `%s`; only app-level messages such as `App.Logout` can be used from other modules", v.Name)
		return
	}
	r.notImported(v.Span, mod)
}

// fakeTablesOf is s's fake tables (none for a nil scope).
func (s *moduleScope) fakeTablesOf() map[string]string {
	if s == nil {
		return nil
	}
	return s.fakeTables
}

// importedFakeTable finds table as a fake's table of a module this one imports: the module
// and the fake's capability name.
func (r *resolver) importedFakeTable(table string) (owner, fake string, ok bool) {
	for _, x := range r.importNames() {
		if f, isFake := r.p.scopes[x].fakeTablesOf()[table]; isFake {
			return x, f, true
		}
	}
	return "", "", false
}

// entryHint is "; `send` is an entry of `capability Mailer`: write `Mailer.send`" when name is
// an entry of a capability of module s (written `Mail.send` for module Mail), else "".
func entryHint(s *moduleScope, name string) string {
	if s == nil || s.entryCaps[name] == "" {
		return ""
	}
	return fmt.Sprintf("; `%s` is an entry of `capability %s`: write `%s.%s`", name, s.entryCaps[name], s.entryCaps[name], name)
}

// ownQualified reports `M.x` inside module M (message 1): never "module `M` is not imported".
// When M declares x the fix drops the qualifier.
// fix is false where span covers more than the name (an element).
func (r *resolver) ownQualified(span diag.Span, name string, declared, fix bool) {
	if !declared {
		d := r.bag.Error(diag.UnknownName, span, "module `%s` has no `%s`%s", r.m.Name, name, entryHint(r.ms, name))
		if c := r.ms.entryCaps[name]; c != "" && fix {
			d.Fix = diag.ReplaceSpan(span, c+"."+name)
		}
		return
	}
	d := r.bag.Error(diag.UnknownName, span, "inside `%s`, its own names are written unqualified: `%s`", r.m.Name, name)
	if fix {
		d.Fix = diag.ReplaceSpan(span, name)
	}
}

// unresolvedCtor reports an unqualified constructor (or table) found nowhere this module can
// see. A table of another module gets the table's message (E5 problem 2); a constructor some
// app module declares names that module (message 4); a name no module declares is silent when
// an import of this module names no module, since that import's error stands for it (message 3).
func (r *resolver) unresolvedCtor(c *syntax.CtorRef) {
	if owner, table, ok := r.tableInOtherModule(c.Name); ok {
		r.tableMisplaced(c.Span, owner, table)
		return
	}
	if owners := r.p.ctorIndex[c.Name]; len(owners) > 0 {
		var of []string
		for _, o := range owners {
			of = append(of, "of `"+o.Module+"."+o.Type+"`")
		}
		if len(owners) == 1 {
			o := owners[0]
			if r.unqual[o.Module] { // its import's E0206 stands for it
				return
			}
			if o.Type == c.Name && r.p.scopes[o.Module].brands[c.Name] && !r.inTest {
				r.bag.Error(diag.InvalidDecl, c.Span, "brand constructor `%s` can only be used in module `%s` or inside a `test`", c.Name, o.Module)
				return
			}
			if path := r.p.importPath(o.Module, r.m.Name); path != nil {
				r.bag.Error(diag.UnknownName, c.Span, "`%s` is a constructor %s, and `%s` %s, so `%s` cannot import it; %s", c.Name, of[0], o.Module, importsPhrase(path), r.m.Name, r.cycleRemedy(o.Module))
				return
			}
			d := r.bag.Error(diag.UnknownName, c.Span, "`%s` is a constructor %s; import `%s`", c.Name, of[0], o.Module)
			d.Fix = r.importFix(o.Module)
			return
		}
		list := strings.Join(of[:len(of)-1], ", ") + " and " + of[len(of)-1]
		r.bag.Error(diag.UnknownName, c.Span, "`%s` is a constructor %s; import one of them", c.Name, list)
		return
	}
	if _, ok := r.p.AppMsgs[c.Name]; ok && r.m.Name != "App" { // `Logout` in a page
		d := r.bag.Error(diag.UnknownName, c.Span, "`%s` is an app-level message: write `App.%s`", c.Name, c.Name)
		d.Fix = diag.ReplaceSpan(c.Span, "App."+c.Name)
		return
	}
	if len(r.missing) > 0 {
		return
	}
	hint := r.missingCause(c.Name)
	if c.Name == "NotFound" && r.inTest && hint == "" {
		hint = "; the browser's 404 answer is `PageMissing`"
	}
	r.bag.Error(diag.UnknownName, c.Span, "unknown constructor `%s`%s", c.Name, hint)
}

// cycleRemedy says what to do instead of importing module x, which imports this module: inside
// a `test`, the test belongs in x's file; elsewhere the code using x's names moves into x.
func (r *resolver) cycleRemedy(x string) string {
	if r.inTest {
		return fmt.Sprintf("a test of `%s` belongs in `%s.alder`", x, modulePath(x))
	}
	return fmt.Sprintf("move the code that uses it into `%s`", x)
}

// missingCause explains a name no module declares when some module's import names no module
// (the E7 shape: App uses `Light`, Layout imports a `Theme` nobody wrote); "" when none does.
// With several such imports every one is named, since any of them may be the one meant.
func (r *resolver) missingCause(name string) string {
	var mods []string
	for x := range r.p.missingImports {
		mods = append(mods, x)
	}
	if len(mods) == 0 {
		return ""
	}
	for _, m := range r.p.Order { // a page's message, say, is declared: the cause is elsewhere
		if s := r.p.scopes[m]; s != nil {
			if _, ok := s.ctors[name]; ok || s.types[name] {
				return ""
			}
		}
	}
	sort.Strings(mods)
	if len(mods) == 1 { // the one missing import is the cause
		x := mods[0]
		return fmt.Sprintf("; no module declares `%s`, and `import %s` in `%s` names no file (`%s.alder`)", name, x, r.p.missingImports[x], modulePath(x))
	}
	// several: which one was meant to declare name is unknowable, so name them all
	var each []string
	for _, x := range mods {
		each = append(each, fmt.Sprintf("`import %s` in `%s`", x, r.p.missingImports[x]))
	}
	list := strings.Join(each[:len(each)-1], ", ") + " and " + each[len(each)-1]
	return fmt.Sprintf("; no module declares `%s`, and %s name no file", name, list)
}

// loginPage is the page named by App's `login` declaration (Auth.Login when there is none).
func (r *resolver) loginPage() string {
	if app := r.p.Modules["App"]; app != nil {
		for _, d := range app.Decls {
			if l, ok := d.(*syntax.LoginDecl); ok && l.Page != "" {
				return l.Page
			}
		}
	}
	return "Auth.Login"
}

// tableAlt is the table a bare name also names, as the resolver would resolve it were it no
// constructor (this module's table, or the table of an imported module of that name), or the
// zero Ref. A constructor of the same name stays the resolution; CtorRef.Table records the table.
func (r *resolver) tableAlt(name string) syntax.Ref {
	if r.ms.tables[name] {
		return syntax.Ref{Kind: syntax.RefTable, Module: r.m.Name, Name: name}
	}
	if path, s, ok := r.importedScope(name); ok && s.tables[lastSegment(path)] && r.p.Modules[path] != nil {
		return syntax.Ref{Kind: syntax.RefTable, Module: path, Name: lastSegment(path)}
	}
	return syntax.Ref{}
}

func (r *resolver) ctor(c *syntax.CtorRef) {
	full := c.Name
	if c.Module != "" {
		full = c.Module + "." + c.Name
	}
	if c.Module == "" {
		if _, ok := r.ms.ctors[c.Name]; ok || r.ms.brands[c.Name] {
			c.Ref = syntax.Ref{Kind: syntax.RefCtor, Module: r.m.Name, Name: c.Name}
			c.Table = r.tableAlt(c.Name)
			return
		}
		if r.p.scopes["Prelude"] != nil {
			if _, ok := r.p.scopes["Prelude"].ctors[c.Name]; ok {
				c.Ref = syntax.Ref{Kind: syntax.RefCtor, Module: "Prelude", Name: c.Name}
				c.Table = r.tableAlt(c.Name)
				return
			}
		}
		var found []string
		for _, x := range r.importNames() {
			if _, ok := r.p.scopes[x].ctors[c.Name]; ok || r.p.scopes[x].brands[c.Name] {
				found = append(found, x)
			}
		}
		if len(found) > 1 {
			r.bag.Error(diag.Duplicate, c.Span, "constructor `%s` is declared in both `%s` and `%s`", c.Name, found[0], found[1])
			return
		}
		if len(found) == 1 {
			r.ms.used[found[0]] = true
			c.Ref = syntax.Ref{Kind: syntax.RefCtor, Module: found[0], Name: c.Name}
			c.Table = r.tableAlt(c.Name)
			return
		}
		if r.p.IsPageModule(c.Name) {
			c.Ref = syntax.Ref{Kind: syntax.RefPage, Module: c.Name}
			return
		}
		if r.ms.tables[c.Name] {
			c.Ref = syntax.Ref{Kind: syntax.RefTable, Module: r.m.Name, Name: c.Name}
			return
		}
		if path, s, ok := r.importedScope(c.Name); ok && s.tables[lastSegment(path)] { // through its qualifier
			c.Ref = syntax.Ref{Kind: syntax.RefTable, Module: path, Name: lastSegment(path)}
			return
		}
		if s := r.p.scopes[c.Name]; s != nil && s.tables[c.Name] {
			r.notImported(c.Span, c.Name) // `Db.from Todo` without `import Todo`
			return
		}
		if owner, fake, ok := r.importedFakeTable(c.Name); ok { // `Db.from Charge` in another module
			r.bag.Error(diag.UnknownName, c.Span, "`%s` is a table of `fake %s` in module `%s`: write `%s.%s`", c.Name, fake, owner, r.qualFor(owner), c.Name)
			return
		}
		r.unresolvedCtor(c)
		return
	}

	if r.p.IsPageModule(full) {
		c.Ref = syntax.Ref{Kind: syntax.RefPage, Module: full}
		return
	}
	if c.Module == "App" {
		if _, ok := r.p.AppMsgs[c.Name]; ok {
			c.Ref = syntax.Ref{Kind: syntax.RefCtor, Module: "App", Name: c.Name}
			return
		}
	}
	// another page's message: `click={Auth.Login.Logout}`
	if r.p.IsPageModule(c.Module) && c.Module != r.m.Name {
		if s := r.p.scopes[c.Module]; s != nil && s.ctors[c.Name] == "Msg" {
			r.bag.Error(diag.UnknownName, c.Span, "`%s` belongs to page `%s`; a page can only send its own messages and `App` messages", c.Name, c.Module)
			return
		}
	}
	if c.Module == r.m.Name { // `Notes.Show.Save` inside Notes.Show
		_, isCtor := r.ms.ctors[c.Name]
		r.ownQualified(c.Span, c.Name, isCtor || r.ms.brands[c.Name] || r.ms.tables[c.Name], true)
		return
	}
	if r.missing[c.Module] || r.isUnqual(c.Module) { // reported at the import
		return
	}
	mod, name := c.Module, c.Name
	if short, path, ok := r.fullPath(full, c.Span.Start); ok {
		mod, name = r.splitShort(short, path)
	}
	if path, s, ok := r.importedScope(mod); ok {
		if s.brands[name] {
			c.Ref = syntax.Ref{Kind: syntax.RefCtor, Module: path, Name: name}
			return
		}
		if _, isCtor := s.ctors[name]; isCtor {
			if s.ctors[name] == "Msg" && r.p.IsPageModule(path) { // an imported page's message, through its qualifier
				r.bag.Error(diag.UnknownName, c.Span, "`%s` belongs to page `%s`; a page can only send its own messages and `App` messages", name, path)
				return
			}
			d := r.bag.Error("E0203", c.Span, "constructors are not qualified; write `%s`", name)
			d.Fix = diag.ReplaceSpan(c.Span, name)
			c.Ref = syntax.Ref{Kind: syntax.RefCtor, Module: path, Name: name}
			return
		}
		if s.tables[name] {
			c.Ref = syntax.Ref{Kind: syntax.RefTable, Module: path, Name: name}
			return
		}
		if !r.stdShadowed(path) {
			r.bag.Error(diag.UnknownName, c.Span, "module `%s` has no `%s`", path, name)
		}
		return
	}
	if owner, table, ok := r.tableInOtherModule(full); ok { // `Db.from Post.Comment` without `import Post`
		r.tableMisplaced(c.Span, owner, table)
		return
	}
	// `Theme.Light` where app module Theme declares Light but is not imported
	if s := r.p.scopes[c.Module]; s != nil && r.p.Modules[c.Module] != nil {
		if _, isCtor := s.ctors[c.Name]; isCtor && s.ctors[c.Name] != "Msg" {
			if path := r.p.importPath(c.Module, r.m.Name); path != nil {
				r.bag.Error("E0203", c.Span, "constructors are not qualified; write `%s`, but `%s` %s, so `%s` cannot import it; %s", c.Name, c.Module, importsPhrase(path), r.m.Name, r.cycleRemedy(c.Module))
				return
			}
			d := r.bag.Error("E0203", c.Span, "constructors are not qualified; write `%s` and import `%s`", c.Name, c.Module)
			d.Fix = r.importFix(c.Module)
			return
		}
	}
	r.bag.Error(diag.UnknownName, c.Span, "unknown `%s`", full)
}

func (r *resolver) pattern(p syntax.Pattern, sc *Scope) {
	switch p := p.(type) {
	case *syntax.PVar:
		sc.bind(p.Name)
	case *syntax.PCtor:
		c := &syntax.CtorRef{Name: p.Name}
		c.Span = p.Span
		r.ctor(c)
		p.Ref = c.Ref
		if p.Arg != nil {
			r.pattern(p.Arg, sc)
		}
	case *syntax.PRecord:
		for _, f := range p.Fields {
			r.pattern(f.Pat, sc)
		}
	case *syntax.PList:
		for _, e := range p.Elems {
			r.pattern(e, sc)
		}
		if p.Rest != "" {
			sc.bind(p.Rest)
		}
	case *syntax.PWild, *syntax.PLit:
	}
}

var builtinComponents = map[string]bool{"Input": true, "Textarea": true, "Select": true, "Errors": true}

func (r *resolver) element(e *syntax.Element, sc *Scope) {
	for _, a := range e.Attrs {
		if a.Value != nil {
			r.expr(a.Value, sc)
		}
	}
	for _, c := range e.Children {
		r.expr(c, sc)
	}
	if !isUpper(e.Tag) {
		return
	}
	if e.Module == "" {
		switch {
		case builtinComponents[e.Tag]:
			e.Ref = syntax.Ref{Kind: syntax.RefComponent, Module: "Prelude", Name: e.Tag}
		case r.ms.comps[e.Tag]:
			e.Ref = syntax.Ref{Kind: syntax.RefComponent, Module: r.m.Name, Name: e.Tag}
		default:
			// an import's qualifier alone: the module's component named like its last segment
			if path, s, ok := r.importedScope(e.Tag); ok && s.comps[lastSegment(path)] {
				e.Ref = syntax.Ref{Kind: syntax.RefComponent, Module: path, Name: lastSegment(path)}
				return
			}
			if r.missing[e.Tag] { // `<Layout>` whose `import Layout` names no module
				return
			}
			r.bag.Error(diag.UnknownName, e.Span, "unknown component `%s`", e.Tag)
		}
		return
	}
	mod, tag := e.Module, e.Tag
	written := mod + "." + tag
	if short, path, ok := r.fullPath(written, r.tagStarts(e, written)...); ok {
		mod, tag = r.splitShort(short, path)
	}
	if path, s, ok := r.importedScope(mod); ok {
		if s.comps[tag] {
			e.Ref = syntax.Ref{Kind: syntax.RefComponent, Module: path, Name: tag}
			return
		}
		r.bag.Error(diag.UnknownName, e.Span, "module `%s` has no component `%s`", path, tag)
		return
	}
	if e.Module == r.m.Name {
		r.ownQualified(e.Span, e.Tag, r.ms.comps[e.Tag], false)
		return
	}
	r.notImported(e.Span, e.Module)
}

// tagStarts is where element e's name, written, begins in the source: after its `<`, and in its
// closing tag when it has one.
func (r *resolver) tagStarts(e *syntax.Element, written string) []diag.Pos {
	out := []diag.Pos{{Line: e.Span.Start.Line, Col: e.Span.Start.Col + 1}}
	closing := "</" + written + ">"
	if end := e.Span.End; end.Line >= 1 && end.Line <= len(r.m.Lines) {
		if line := r.m.Lines[end.Line-1]; end.Col-1 <= len(line) && strings.HasSuffix(line[:end.Col-1], closing) {
			out = append(out, diag.Pos{Line: end.Line, Col: end.Col - len(closing) + 2})
		}
	}
	return out
}
