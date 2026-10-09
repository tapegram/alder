// internal/names/resolve.go
package names

import (
	"sort"

	"alder/internal/diag"
	"alder/internal/syntax"
)

var preludeTypes = map[string]bool{"Int": true, "Float": true, "Bool": true, "Text": true, "Unit": true, "Date": true, "Time": true, "List": true, "Maybe": true, "Result": true, "Dict": true, "Html": true, "Never": true, "Query": true, "Task": true, "Nav": true, "Error": true, "Form": true, "Table": true, "Id": true, "Insert": true, "Select": true, "Condition": true, "Order": true, "Page": true}
var generatedTypes = map[string]bool{"Params": true, "Msg": true, "Session": true}

// moduleScope indexes one module's declarations.
type moduleScope struct {
	name    string
	values  map[string]bool
	types   map[string]bool   // type, table, form, brand names
	ctors   map[string]string // ctor → owning type name
	brands  map[string]bool   // brand names; the constructor is usable only in the declaring module
	tables  map[string]bool
	caps    map[string]bool
	comps   map[string]bool            // capitalized value names
	columns map[string]map[string]bool // table → field set
	imports map[string]*syntax.Import
	used    map[string]bool
}

func (p *Program) index(m *syntax.Module) *moduleScope {
	s := &moduleScope{name: m.Name, values: map[string]bool{}, types: map[string]bool{}, ctors: map[string]string{}, brands: map[string]bool{}, tables: map[string]bool{}, caps: map[string]bool{}, comps: map[string]bool{}, columns: map[string]map[string]bool{}, imports: map[string]*syntax.Import{}, used: map[string]bool{}}
	for _, im := range m.Imports {
		s.imports[im.Module] = im
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
			s.types[d.Name] = true
			s.tables[d.Name] = true
			cols := map[string]bool{}
			for _, f := range d.Fields {
				cols[f.Name] = true
			}
			s.columns[d.Name] = cols
		case *syntax.FormDecl:
			s.types[d.Name] = true
		case *syntax.CapDecl:
			s.caps[d.Name] = true
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
}

func (p *Program) Resolve(bag *diag.Bag) {
	for _, name := range p.Order {
		p.scopes[name] = p.index(p.Modules[name])
	}
	for _, name := range p.Std {
		p.scopes[name.Name] = p.index(name)
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

// importNames lists the valid imported module names in declaration order.
func (r *resolver) importNames() []string {
	var out []string
	seen := map[string]bool{}
	for _, im := range r.m.Imports {
		if _, ok := r.ms.imports[im.Module]; ok && !seen[im.Module] {
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
			r.bag.Error(diag.UnknownName, im.Span, "unknown module `%s`", im.Module)
			delete(r.ms.imports, im.Module)
		}
	}
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
		case *syntax.SessionDecl:
			r.fields(d.Fields)
		case *syntax.LoginDecl:
			if !r.p.IsPageModule(d.Page) {
				r.bag.Error(diag.UnknownName, d.Span, "unknown page `%s`", d.Page)
			}
		case *syntax.TestDecl:
			r.expr(d.Body, &Scope{names: map[string]bool{}})
		}
	}
	for _, name := range r.importNames() {
		if !r.ms.used[name] {
			r.bag.Warn(diag.UnusedImport, r.ms.imports[name].Span, "import `%s` is unused", name)
		}
	}
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

// importedScope returns the scope of X if X is imported or std, marking it used.
func (r *resolver) importedScope(x string) (*moduleScope, bool) {
	if _, ok := r.ms.imports[x]; ok {
		if s := r.p.scopes[x]; s != nil {
			r.ms.used[x] = true
			return s, true
		}
		return nil, false
	}
	if _, ok := r.p.Std[x]; ok {
		if s := r.p.scopes[x]; s != nil {
			return s, true
		}
	}
	return nil, false
}

func (r *resolver) notImported(span diag.Span, x string) {
	d := r.bag.Error("E0201", span, "module `%s` is not imported", x)
	line := 2
	if n := len(r.m.Imports); n > 0 {
		line = r.m.Imports[n-1].Span.End.Line + 1
	}
	d.Fix = &diag.Fix{Line: line, Col: 1, Replace: "import " + x}
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
			if s, ok := r.importedScope(t.Module); ok {
				if s.types[t.Name] {
					t.Ref = syntax.Ref{Kind: syntax.RefType, Module: t.Module, Name: t.Name}
					return
				}
				r.bag.Error(diag.UnknownName, t.Span, "module `%s` has no type `%s`", t.Module, t.Name)
				return
			}
			if r.p.IsPageModule(t.Module+"."+t.Name) || r.p.IsPageModule(t.Module) {
				t.Ref = syntax.Ref{Kind: syntax.RefPage, Module: t.Module + "." + t.Name}
				return
			}
			if t.Module == "App" { // App needs no import; only App.Msg is a usable type
				switch t.Name {
				case "Msg": // the type of app-level messages
					t.Ref = syntax.Ref{Kind: syntax.RefType, Module: "App", Name: "Msg"}
				case "Session":
					d := r.bag.Error(diag.UnknownName, t.Span, "`Session` is not qualified; write `Session`")
					d.Fix = &diag.Fix{Line: t.Span.Start.Line, Col: t.Span.Start.Col, Replace: "Session"}
				default:
					r.bag.Error(diag.UnknownName, t.Span, "`App` has no type `%s` other modules can use; only `App.Msg` is global", t.Name)
				}
				return
			}
			r.notImported(t.Span, t.Module)
			return
		}
		if r.ms.types[t.Name] {
			t.Ref = syntax.Ref{Kind: syntax.RefType, Module: r.m.Name, Name: t.Name}
			return
		}
		// module-name rule: imported module X declaring type X
		if s, ok := r.importedScope(t.Name); ok && s.types[t.Name] {
			t.Ref = syntax.Ref{Kind: syntax.RefType, Module: t.Name, Name: t.Name}
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
					d := r.bag.Error("E0202", t.Span, "type `%s` belongs to module `%s`; write `%s.%s`", t.Name, x, x, t.Name)
					d.Fix = &diag.Fix{Line: t.Span.Start.Line, Col: t.Span.Start.Col, Replace: x + "." + t.Name}
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
			r.bag.Error(diag.UnknownName, t.Span, "unknown type `%s`", t.Name)
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
		for _, a := range e.Args {
			r.expr(a, sc)
		}
	case *syntax.Lambda:
		c := sc.child()
		for _, p := range e.Params {
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
	// Module.value
	if s, ok := r.importedScope(v.Module); ok {
		switch {
		case s.values[v.Name]:
			kind := syntax.RefValue
			if _, std := r.p.Std[v.Module]; std {
				kind = syntax.RefStd
			}
			v.Ref = syntax.Ref{Kind: kind, Module: v.Module, Name: v.Name}
		case s.tables[v.Module] && s.columns[v.Module][v.Name]:
			v.Ref = syntax.Ref{Kind: syntax.RefColumn, Module: v.Module, Name: v.Name}
		default:
			r.bag.Error(diag.UnknownName, v.Span, "module `%s` has no `%s`", v.Module, v.Name)
		}
		return
	}
	// capability declared in this or an imported module: Password.verify
	if r.ms.caps[v.Module] {
		v.Ref = syntax.Ref{Kind: syntax.RefCapability, Module: r.m.Name, Name: v.Module + "." + v.Name}
		return
	}
	for _, x := range r.importNames() {
		if r.p.scopes[x].caps[v.Module] {
			r.ms.used[x] = true
			v.Ref = syntax.Ref{Kind: syntax.RefCapability, Module: x, Name: v.Module + "." + v.Name}
			return
		}
	}
	// this module's own table columns: Todo.done inside module Todo
	if r.ms.tables[v.Module] && r.ms.columns[v.Module][v.Name] {
		v.Ref = syntax.Ref{Kind: syntax.RefColumn, Module: r.m.Name, Name: v.Name}
		return
	}
	if v.Module == "App" { // App needs no import; only its messages are usable values
		if v.Name == "login" {
			r.bag.Error(diag.UnknownName, v.Span, "`App` has no `login`; `login` is a declaration, not a value — link to the page with `href={%s {}}`", r.loginPage())
			return
		}
		r.bag.Error(diag.UnknownName, v.Span, "`App` has no `%s`; only app-level messages such as `App.Logout` can be used from other modules", v.Name)
		return
	}
	r.notImported(v.Span, v.Module)
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

func (r *resolver) ctor(c *syntax.CtorRef) {
	full := c.Name
	if c.Module != "" {
		full = c.Module + "." + c.Name
	}
	if c.Module == "" {
		if _, ok := r.ms.ctors[c.Name]; ok || r.ms.brands[c.Name] {
			c.Ref = syntax.Ref{Kind: syntax.RefCtor, Module: r.m.Name, Name: c.Name}
			return
		}
		if r.p.scopes["Prelude"] != nil {
			if _, ok := r.p.scopes["Prelude"].ctors[c.Name]; ok {
				c.Ref = syntax.Ref{Kind: syntax.RefCtor, Module: "Prelude", Name: c.Name}
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
		if s, ok := r.importedScope(c.Name); ok && s.tables[c.Name] {
			c.Ref = syntax.Ref{Kind: syntax.RefTable, Module: c.Name, Name: c.Name}
			return
		}
		if s := r.p.scopes[c.Name]; s != nil && s.tables[c.Name] {
			r.notImported(c.Span, c.Name) // `Db.from Todo` without `import Todo`
			return
		}
		r.bag.Error(diag.UnknownName, c.Span, "unknown constructor `%s`", c.Name)
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
	if s, ok := r.importedScope(c.Module); ok {
		if s.brands[c.Name] {
			c.Ref = syntax.Ref{Kind: syntax.RefCtor, Module: c.Module, Name: c.Name}
			return
		}
		if _, isCtor := s.ctors[c.Name]; isCtor {
			d := r.bag.Error("E0203", c.Span, "constructors are not qualified; write `%s`", c.Name)
			d.Fix = &diag.Fix{Line: c.Span.Start.Line, Col: c.Span.Start.Col, Replace: c.Name}
			c.Ref = syntax.Ref{Kind: syntax.RefCtor, Module: c.Module, Name: c.Name}
			return
		}
		if s.tables[c.Name] {
			c.Ref = syntax.Ref{Kind: syntax.RefTable, Module: c.Module, Name: c.Name}
			return
		}
		r.bag.Error(diag.UnknownName, c.Span, "module `%s` has no `%s`", c.Module, c.Name)
		return
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
			if s, ok := r.importedScope(e.Tag); ok && s.comps[e.Tag] {
				e.Ref = syntax.Ref{Kind: syntax.RefComponent, Module: e.Tag, Name: e.Tag}
				return
			}
			r.bag.Error(diag.UnknownName, e.Span, "unknown component `%s`", e.Tag)
		}
		return
	}
	if s, ok := r.importedScope(e.Module); ok {
		if s.comps[e.Tag] {
			e.Ref = syntax.Ref{Kind: syntax.RefComponent, Module: e.Module, Name: e.Tag}
			return
		}
		r.bag.Error(diag.UnknownName, e.Span, "module `%s` has no component `%s`", e.Module, e.Tag)
		return
	}
	r.notImported(e.Span, e.Module)
}
