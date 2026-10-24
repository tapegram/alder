// Package generate holds the code generators behind `alder new page|table|form|msg|capability`,
// `alder edit table` and `alder fill` (book chapter 15): the facts a generator reads from the
// checked app (Program), the field syntax of their command lines (ParseField), the templates
// they write from (templates/), and the copy-check-write pipeline every command ends in (Apply):
// the edits are applied to a copy of the app, the copy is checked exactly as `alder check`
// checks the app, and only a copy that checks is written back. A generator never overwrites a
// file: it refuses, with one sentence, and writes nothing.
package generate

import (
	"fmt"
	"slices"
	"strings"

	"alder/internal/buildkit"
	"alder/internal/check"
	"alder/internal/diag"
	"alder/internal/names"
	"alder/internal/syntax"
	"alder/internal/types"
)

// Program is what the generators know about an app: its modules, tables, pages and session,
// read from the checked program (buildkit.LoadChecked, the load path of `alder check`).
type Program struct {
	Dir     string             // the app's directory, as given
	Modules map[string]*Module // by module name
	Order   []string           // module names, sorted
	Tables  []*Table           // sorted by module name
	Pages   []*Page            // sorted by module name
	Session *Session           // App's session; nil without an App module
	Login   string             // the page App's `login` names ("Auth.Login"), "" for none
	// Layout is whether module Layout has a component Page (`Layout.Page`): generated pages
	// wrap their view in it, else in a bare `<al-page>`.
	Layout bool
	Std    map[string]bool // the standard library's module names (Text, Db, …)
	// Silos are App's `silo k { T, … }` (M15): each key's table modules; Via each `via`
	// session field's membership table name; Choose each `choose k = Page`'s page.
	Silos  map[string][]string
	Via    map[string]string
	Choose map[string]string

	Prog  *names.Program
	Res   *check.Result
	Diags []diag.Diagnostic // the app's diagnostics (warnings only, unless LoadWithErrors)
}

// Module is one module of the app.
type Module struct {
	Name    string
	File    string // relative to the app, slash-separated: "app/Posts/Index.alder"
	Imports []string
	Syntax  *syntax.Module
	Source  string // the file's text
}

// Table is one table declaration.
type Table struct {
	Module  string // the module it is declared in, which is named after it ("Post")
	Name    string
	File    string
	IDField string     // "id"
	IDType  types.Type // the id's brand (Post.PostId)
	Columns []*Column  // in declaration order, the id included
	Owner   string     // the `@owner` column, "" for none
	// PartKey and PartCol are the table's partition (M15): the session field its `@partition k`
	// (or `@owner`, "user") names and the column; "" for none. Silo is the key of the `silo`
	// that lists it, "" for a table in the shared store.
	PartKey, PartCol, Silo string
	Created                string // the `@created` column, "" for none
	Updated                string // the `@updated` column, "" for none
	Decl                   *syntax.TableDecl
}

// Column is one field of a table.
type Column struct {
	Name  string
	Type  types.Type // the field's type
	Maybe bool       // Type is `Maybe Elem`
	Elem  types.Type // Type without its Maybe
	Kind  Kind       // of Elem (a brand's is its base's)
	// Brand is Elem when it is a brand (Text.NonEmpty, Post.Slug, Post.PostId), else nil.
	Brand *types.Brand
	// Union is Elem when it is a union of constructors without payloads, else nil; Ctors are its
	// constructors.
	Union      *types.Union
	Ctors      []string
	Default    string // the default's source text, "" for none
	HasDefault bool
	// the attributes: @id, @created, @updated, @owner, @unique
	ID, Created, Updated, Owner, Unique bool
	// Ref is the table `@ref` names ("User"), "" for none; OnDelete its rule, "restrict",
	// "cascade" or "setNull" ("" without a ref); Renamed the old name of `@renamed "old"`.
	Ref, OnDelete, Renamed string
	Decl                   *syntax.Field
}

// Kind is a column's (or a form field's) storage class.
type Kind string

const (
	KindText  Kind = "Text"
	KindInt   Kind = "Int"
	KindFloat Kind = "Float"
	KindBool  Kind = "Bool"
	KindDate  Kind = "Date"
	KindTime  Kind = "Time"
	KindUnion Kind = "union"
	KindOther Kind = "other"
)

// TypeIn is the column's type as written in module mod.
func (c *Column) TypeIn(mod string) string { return types.StringIn(c.Type, mod) }

// typeText is t as written in module home: each module through the qualifier home writes it
// with (#23, names.CanonicalQualifiers over home's imports and t's modules: `Post.PostId` for
// Table.Post's brand, `TablePost.PostId` when home also imports a Blog.Post), a standard
// library module by its name. Messages, and comparisons with a type the person typed, use it.
func typeText(p *Program, t types.Type, home string) string {
	var mods []string
	types.StringQ(t, home, func(m string) string { mods = append(mods, m); return "" })
	q := p.qualifiersFor(home, mods)
	return types.StringQ(t, home, func(m string) string { return q[m] })
}

// qualifierIn is the qualifier module home writes module with (qualifiersFor).
func (p *Program) qualifierIn(home, module string) string {
	if q := p.qualifiersFor(home, []string{module})[module]; q != "" {
		return q
	}
	return module
}

// qualifiersFor are the canonical qualifiers of home's imports with mods imported too; a
// standard-library module, and one with no usable qualifier, map to "" (written by its path).
func (p *Program) qualifiersFor(home string, mods []string) map[string]string {
	var paths []string
	taken := func(n string) bool { return n == "App" || p.Std[n] }
	if m := p.Modules[home]; m != nil && m.Syntax != nil {
		taken = names.Taken(m.Syntax, func(n string) bool { return p.Std[n] })
		for _, im := range m.Syntax.Imports {
			paths = append(paths, im.Module)
		}
	}
	for _, m := range mods {
		if m != home && m != "Prelude" {
			paths = append(paths, m)
		}
	}
	paths = slices.DeleteFunc(paths, func(m string) bool { return p.Std[m] })
	q, _ := names.CanonicalQualifiers(home, paths, taken)
	return q
}

// Required is whether an insert must name the column: not the id, @created or @updated, no
// default (a Maybe without a default is required too, as `Nothing`).
func (c *Column) Required() bool { return !c.ID && !c.Created && !c.Updated && !c.HasDefault }

// Page is one page module.
type Page struct {
	Module   string
	File     string
	Path     string
	Params   []*Param
	Requires string // "user", or "" for none
	// Keys are every field of the page's `requires` (["user", "org"]); Silo the key of the admin
	// form `requires user, role R, silo k`, "" for none (M15).
	Keys  []string
	Silo  string
	Roles []string // `role R | S`, nil for none
	Msgs  []string // the page's messages, in declaration order
	Forms []string // the page's forms, in declaration order
	Decl  *syntax.PageDecl
}

// Param is one page parameter.
type Param struct {
	Name       string
	Type       types.Type
	HasDefault bool
}

// Session is App's session.
type Session struct {
	Fields []*SessionField
	// User is the `user` field (`user: Maybe User.UserId`), nil for none; UserTable is the
	// table whose id it is ("User"), "" when it is not a table's id.
	User      *SessionField
	UserTable string
	// Role is the `role` field when its type is a union of constructors without payloads,
	// nil otherwise.
	Role *SessionField
}

// SessionField is one field of the session.
type SessionField struct {
	Name       string
	Type       types.Type
	HasDefault bool
	Union      *types.Union // the field's union (its Maybe's for `user`), nil for none
	Ctors      []string
}

// Load reads the app in dir as `alder check` does. An app that does not load, or has errors,
// is refused: a generator works on an app that checks.
func Load(dir string) (*Program, error) { return load(dir, false) }

// LoadWithErrors is Load for `alder edit table`, which may change a table of an app whose code
// a previous edit broke (the errors are its to-do list): only an app that does not parse is
// refused. The errors are among the Program's Diags.
func LoadWithErrors(dir string) (*Program, error) { return load(dir, true) }

// LoadExcept is Load for `alder regen admin`: errors in the files under the directory under
// (the admin's, "app/Admin") do not refuse the app. Regen rewrites those files' regions, and a
// schema edit (a field renamed or dropped, a page deleted that another links to) breaks them
// there; what regen writes is then checked as usual (Apply), so an error it does not fix still
// refuses. Errors anywhere else refuse, as Load's do.
func LoadExcept(dir, under string) (*Program, error) {
	p, err := load(dir, true)
	if err != nil {
		return nil, err
	}
	var errs []diag.Diagnostic
	for _, d := range p.Diags {
		if d.Severity == "error" && !strings.HasPrefix(d.Span.File, under+"/") {
			errs = append(errs, d)
		}
	}
	if len(errs) > 0 {
		return nil, &Refusal{Msg: "the app has errors (`alder check` lists them): fix them first, so that what is generated can be checked", Diags: errs, Sources: buildkit.Sources(p.Prog)}
	}
	return p, nil
}

func load(dir string, errorsOK bool) (*Program, error) {
	prog, res, bag, ioErrs := buildkit.LoadChecked(dir, nil)
	if len(ioErrs) > 0 {
		return nil, ioErrs[0]
	}
	if len(prog.Modules) == 0 {
		return nil, Refusef("there are no .alder files under %s/app: run it in an app's directory, or name one with --dir", dir)
	}
	if (bag.HasErrors() && !errorsOK) || res == nil {
		return nil, &Refusal{Msg: "the app has errors (`alder check` lists them): fix them first, so that what is generated can be checked", Diags: bag.Items, Sources: buildkit.Sources(prog)}
	}
	return newProgram(dir, prog, res, bag.Items), nil
}

func newProgram(dir string, prog *names.Program, res *check.Result, diags []diag.Diagnostic) *Program {
	p := &Program{Dir: dir, Modules: map[string]*Module{}, Std: map[string]bool{}, Prog: prog, Res: res, Diags: diags}
	for name := range prog.Std {
		p.Std[name] = true
	}
	for name, m := range prog.Modules {
		mod := &Module{Name: name, File: m.File, Syntax: m, Source: strings.Join(m.Lines, "\n")}
		for _, im := range m.Imports {
			mod.Imports = append(mod.Imports, im.Module)
		}
		p.Modules[name] = mod
		p.Order = append(p.Order, name)
	}
	slices.Sort(p.Order)
	for _, name := range p.Order {
		m := p.Modules[name]
		for _, d := range m.Syntax.Decls {
			switch d := d.(type) {
			case *syntax.TableDecl:
				if t := p.table(m, d); t != nil {
					p.Tables = append(p.Tables, t)
				}
			case *syntax.ValueDecl:
				if name == "Layout" && d.Name == "Page" {
					p.Layout = true
				}
			}
		}
		if pd := prog.Pages[name]; pd != nil {
			p.Pages = append(p.Pages, p.page(m, pd))
		}
	}
	p.Session = p.session()
	p.Silos, p.Via, p.Choose = map[string][]string{}, map[string]string{}, map[string]string{}
	for k, ts := range res.Silos {
		for _, tk := range ts {
			if i := strings.LastIndex(tk, "."); i > 0 { // "Table.Post.Post": the module before the name
				p.Silos[k] = append(p.Silos[k], tk[:i])
			}
		}
	}
	for k, tk := range res.Via {
		p.Via[k] = tk[strings.LastIndex(tk, ".")+1:] // the table's name, after its module (all of tk without one)
	}
	for k, pg := range res.Choose {
		p.Choose[k] = pg
	}
	if app := p.Modules["App"]; app != nil {
		for _, d := range app.Syntax.Decls {
			if ld, ok := d.(*syntax.LoginDecl); ok {
				p.Login = ld.Page
			}
		}
	}
	return p
}

// table reads table d of module m from the checker's result.
func (p *Program) table(m *Module, d *syntax.TableDecl) *Table {
	info := p.Res.Tables[m.Name+"."+d.Name]
	if info == nil || info.Record == nil {
		return nil
	}
	t := &Table{Module: m.Name, Name: d.Name, File: m.File, IDField: info.IDField, IDType: info.IDType, Owner: info.Owner, Decl: d,
		PartKey: info.PartKey, PartCol: info.PartCol}
	for k, ts := range p.Res.Silos {
		if slices.Contains(ts, m.Name+"."+d.Name) {
			t.Silo = k
		}
	}
	for i, f := range d.Fields {
		if i >= len(info.Record.Fields) {
			break
		}
		c := &Column{Name: f.Name, Type: info.Record.Fields[i].Type, Decl: f}
		c.Elem, c.Maybe = unMaybe(c.Type)
		c.Kind, c.Brand, c.Union = kindOf(c.Elem)
		if c.Union != nil {
			for _, ct := range c.Union.Constructors() {
				c.Ctors = append(c.Ctors, ct.Name)
			}
		}
		if f.Default != nil {
			c.HasDefault, c.Default = true, sliceSpan(m.Syntax.Lines, f.Default.GetSpan())
		}
		for _, a := range f.Attrs {
			switch a.Name {
			case "id":
				c.ID = true
			case "created":
				c.Created, t.Created = true, f.Name
			case "updated":
				c.Updated, t.Updated = true, f.Name
			case "unique":
				c.Unique = true
			case "owner":
				c.Owner = true
			case "ref":
				c.Ref, c.OnDelete = a.Arg, "restrict"
			case "cascade", "restrict", "setNull":
				c.OnDelete = a.Name
			case "renamed":
				c.Renamed = a.Arg
			}
		}
		t.Columns = append(t.Columns, c)
	}
	return t
}

// page reads page module m.
func (p *Program) page(m *Module, d *syntax.PageDecl) *Page {
	pg := &Page{Module: m.Name, File: m.File, Path: d.Path, Roles: d.Roles, Decl: d}
	if len(d.Requires) > 0 { // the generators know `requires user` (M15's other keys: Task 4)
		pg.Requires = d.Requires[0]
	}
	pg.Keys, pg.Silo = d.Requires, d.Silo
	if info := p.Res.Pages[m.Name]; info != nil && info.Params != nil {
		for i, f := range info.Params.Fields {
			par := &Param{Name: f.Name, Type: f.Type, HasDefault: f.HasDefault}
			if d.Params != nil && i < len(d.Params.Fields) && d.Params.Fields[i].Default != nil {
				par.HasDefault = true
			}
			pg.Params = append(pg.Params, par)
		}
	}
	for _, d := range m.Syntax.Decls {
		switch d := d.(type) {
		case *syntax.MsgDecl:
			pg.Msgs = append(pg.Msgs, d.Name)
		case *syntax.FormDecl:
			pg.Forms = append(pg.Forms, d.Name)
		}
	}
	return pg
}

// session reads App's session.
func (p *Program) session() *Session {
	app := p.Modules["App"]
	if app == nil || p.Res.Session == nil {
		return nil
	}
	s := &Session{}
	var decl *syntax.SessionDecl
	for _, d := range app.Syntax.Decls {
		if sd, ok := d.(*syntax.SessionDecl); ok {
			decl = sd
		}
	}
	for i, f := range p.Res.Session.Fields {
		sf := &SessionField{Name: f.Name, Type: f.Type, HasDefault: f.HasDefault}
		if decl != nil && i < len(decl.Fields) && decl.Fields[i].Default != nil {
			sf.HasDefault = true
		}
		elem, _ := unMaybe(f.Type)
		if _, _, u := kindOf(elem); u != nil {
			sf.Union = u
			for _, c := range u.Constructors() {
				sf.Ctors = append(sf.Ctors, c.Name)
			}
		}
		s.Fields = append(s.Fields, sf)
		switch f.Name {
		case "user":
			if _, isMaybe := unMaybe(f.Type); isMaybe {
				s.User = sf
				for _, t := range p.Tables {
					if types.String(elem) == types.String(t.IDType) {
						s.UserTable = t.Name
					}
				}
			}
		case "role":
			if _, isMaybe := unMaybe(f.Type); !isMaybe && sf.Union != nil {
				s.Role = sf
			}
		}
	}
	return s
}

// TableNamed is the table of that name (`Todo`, the name `--list T` and `@ref T` use), or
// nil. Its module is Table.<Name> (TableModule), or, in an app written before that layout,
// the root module <Name>: table names are unique in an app (E0037).
func (p *Program) TableNamed(name string) *Table {
	for _, t := range p.Tables {
		if t.Name == name {
			return t
		}
	}
	return nil
}

// TableModule is the module a new table of that name is declared in: Table.<Name>, the file
// app/Table/<Name>.alder.
func TableModule(name string) string { return "Table." + name }

// PageNamed is page module name, or nil.
func (p *Program) PageNamed(name string) *Page {
	for _, pg := range p.Pages {
		if pg.Module == name {
			return pg
		}
	}
	return nil
}

// Column is the table's column name, or nil.
func (t *Table) Column(name string) *Column {
	for _, c := range t.Columns {
		if c.Name == name {
			return c
		}
	}
	return nil
}

// RoleColumn is the user table's column holding the session's role (`role: Role = Reader` in
// `User`, of the session role's type), nil when the session has no role or user table, or the
// table no such column: the role a login copies, which a page can read again (chapter 6,
// "Rules").
func (p *Program) RoleColumn() *Column {
	s := p.Session
	if s == nil || s.Role == nil || s.UserTable == "" {
		return nil
	}
	t := p.TableNamed(s.UserTable)
	if t == nil {
		return nil
	}
	for _, c := range t.Columns {
		if !c.Maybe && types.String(c.Type) == types.String(s.Role.Type) {
			return c
		}
	}
	return nil
}

// Label is the column a row is named by: its first Text column that is not an id or a
// reference, nor a secret (Secret), else its id.
func (t *Table) Label() *Column {
	for _, c := range t.Columns {
		if c.Kind == KindText && !c.Maybe && !c.ID && c.Ref == "" && !c.Secret() {
			return c
		}
	}
	return t.Column(t.IDField)
}

// Secret reports whether column c holds a secret the admin never shows or edits: its type is
// `Text.Secret` (or a Maybe of it), or its name, in any case, begins with `password`, ends in
// `hash`, or holds `token` or `secret` (`passwordHash`, `apiToken`, `clientSecret`).
func (c *Column) Secret() bool {
	t, _ := unMaybe(c.Type)
	if b, ok := t.(*types.Brand); ok && b.Module == "Text" && b.Name == "Secret" {
		return true
	}
	n := strings.ToLower(c.Name)
	return strings.HasPrefix(n, "password") || strings.HasSuffix(n, "hash") || strings.Contains(n, "token") || strings.Contains(n, "secret")
}

// OrderColumn is the column a list is ordered by, newest first: the `@created` one, else the id.
func (t *Table) OrderColumn() string {
	if t.Created != "" {
		return t.Created
	}
	return t.IDField
}

// FileOf is the file of module name: "app/A/B.alder" for "A.B".
func FileOf(module string) string { return "app/" + strings.ReplaceAll(module, ".", "/") + ".alder" }

// unMaybe is t without a `Maybe`, and whether it had one.
func unMaybe(t types.Type) (types.Type, bool) {
	if u, ok := t.(*types.Union); ok && u.Module == "Prelude" && u.Name == "Maybe" && len(u.Args) == 1 {
		return u.Args[0], true
	}
	return t, false
}

// kindOf classifies a column's (non-Maybe) type: its storage kind, its brand, its union.
func kindOf(t types.Type) (Kind, *types.Brand, *types.Union) {
	switch x := t.(type) {
	case *types.Con:
		switch x.Name {
		case "Text":
			return KindText, nil, nil
		case "Int":
			return KindInt, nil, nil
		case "Float":
			return KindFloat, nil, nil
		case "Bool":
			return KindBool, nil, nil
		case "Date":
			return KindDate, nil, nil
		case "Time":
			return KindTime, nil, nil
		}
	case *types.Brand:
		k, _, _ := kindOf(x.Base)
		return k, x, nil
	case *types.Union:
		if x.Module == "Prelude" {
			return KindOther, nil, nil
		}
		for _, c := range x.Constructors() {
			if c.Arg != nil {
				return KindOther, nil, nil
			}
		}
		return KindUnion, nil, x
	}
	return KindOther, nil, nil
}

// sliceSpan is the source text of span sp in lines (1-based lines and columns, the end
// exclusive).
func sliceSpan(lines []string, sp diag.Span) string {
	if sp.Start.Line < 1 || sp.End.Line > len(lines) || sp.End.Line < sp.Start.Line {
		return ""
	}
	if sp.Start.Line == sp.End.Line {
		l := lines[sp.Start.Line-1]
		return l[min(len(l), sp.Start.Col-1):min(len(l), sp.End.Col-1)]
	}
	var b strings.Builder
	first := lines[sp.Start.Line-1]
	b.WriteString(first[min(len(first), sp.Start.Col-1):])
	for l := sp.Start.Line + 1; l < sp.End.Line; l++ {
		b.WriteString("\n" + lines[l-1])
	}
	last := lines[sp.End.Line-1]
	b.WriteString("\n" + last[:min(len(last), sp.End.Col-1)])
	return b.String()
}

// Refusal is a command's refusal: one sentence (Msg), and the diagnostics that are its reason
// when the copy did not check (with the sources to render them).
type Refusal struct {
	Msg     string
	Diags   []diag.Diagnostic
	Sources map[string][]string
	Result  *Result // the copy's result, when the refusal comes from Apply
}

func (r *Refusal) Error() string { return r.Msg }

// Refusef is a Refusal without diagnostics.
func Refusef(format string, args ...any) *Refusal { return &Refusal{Msg: fmt.Sprintf(format, args...)} }

// KeyTable is the table whose id session field key holds (`org: Maybe Org.OrgId` → Org), nil
// when there is none.
func (p *Program) KeyTable(key string) *Table {
	if p.Session == nil {
		return nil
	}
	for _, f := range p.Session.Fields {
		if f.Name != key {
			continue
		}
		elem, _ := unMaybe(f.Type)
		for _, t := range p.Tables {
			if types.String(elem) == types.String(t.IDType) {
				return t
			}
		}
	}
	return nil
}

// SessionField is the session's field name, nil for none.
func (p *Program) SessionField(name string) *SessionField {
	if p.Session == nil {
		return nil
	}
	for _, f := range p.Session.Fields {
		if f.Name == name {
			return f
		}
	}
	return nil
}
