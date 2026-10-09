// Package check is the type checker: a declaration pass followed by body checking.
package check

import (
	"alder/internal/diag"
	"alder/internal/names"
	"alder/internal/syntax"
	"alder/internal/types"
)

type Checker struct {
	prog     *names.Program
	bag      *diag.Bag
	u        *types.Unifier
	types    map[string]types.Type
	values   map[string]types.Type
	ctors    map[string]*ctorInfo
	tables   map[string]*types.TableInfo
	forms    map[string]*types.Record
	caps     map[string]map[string]types.Type
	pages    map[string]*pageInfo
	session  *types.Record
	appMsg   *types.Union
	losers   map[syntax.Decl]bool
	defaults []pendingDefault // field defaults, checked after value signatures // duplicate declarations (E0009), skipped after declareTypeNames

	usesSessionPut bool // some module calls Session.put
	res            *Result
}

// holeNote records a `?name` typed hole to be reported once its type is known.
type holeNote struct {
	name string
	t    types.Type
	span diag.Span
}

// formCtx is the form being built by the enclosing `Form.*` expression.
type formCtx struct {
	name   string // "" when the submit message declares no `form`
	rec    *types.Record
	submit string // the submit value, abbreviated, for messages
}

type modCtx struct {
	c        *Checker
	m        *syntax.Module
	page     *pageInfo
	session  *types.Record
	isApp    bool
	holes    []holeNote
	form     *formCtx
	inUpdate bool
	inTest   bool
}

// Program checks prog, reporting into bag, and returns the types it learned.
func Program(prog *names.Program, bag *diag.Bag) *Result {
	c := &Checker{prog: prog, bag: bag, u: &types.Unifier{Subst: types.Subst{}},
		types: map[string]types.Type{}, values: map[string]types.Type{}, ctors: map[string]*ctorInfo{},
		tables: map[string]*types.TableInfo{}, forms: map[string]*types.Record{}, caps: map[string]map[string]types.Type{}, pages: map[string]*pageInfo{}, losers: map[syntax.Decl]bool{},
		res: &Result{Types: map[syntax.Expr]types.Type{}, Binds: map[any]types.Type{}}}
	c.declareStd()
	// pass 1: placeholders for every declared type in every module (app + std)
	for _, m := range c.allModules() {
		c.declareTypeNames(m)
	}
	// pass 2: bodies of types, tables, forms
	for _, m := range c.allModules() {
		c.ctx(m).defineTypes()
	}
	// pass 2b: representability checks need every union's constructors
	for _, m := range c.allModules() {
		c.ctx(m).checkRepresentable()
	}
	// pass 3: App session + app msgs, then pages (generated Params/Msg), capabilities, value signatures
	if app := prog.Modules["App"]; app != nil {
		c.ctx(app).defineApp()
	}
	if c.session == nil {
		c.session = &types.Record{Module: "App", Name: "Session"}
	}
	if c.appMsg == nil {
		c.appMsg = &types.Union{Module: "App", Name: "Msg"}
	}
	for _, name := range prog.Order {
		mc := c.ctx(prog.Modules[name])
		if prog.IsPageModule(name) {
			mc.definePage()
		}
		mc.defineCapabilities()
	}
	for _, m := range c.allModules() {
		c.ctx(m).declareValues()
	}
	c.checkDefaults()
	// pass 4: bodies (Tasks 9-11)
	for _, name := range prog.Order {
		if prog.Failed[name] {
			continue // resolution failed: its bodies would only cascade
		}
		c.ctx(prog.Modules[name]).checkBodies()
	}
	// App rules need every module's bodies checked first (Session.put usage)
	if app := prog.Modules["App"]; app != nil && !prog.Failed["App"] {
		c.ctx(app).checkApp()
	}
	return c.result()
}

func (c *Checker) allModules() []*syntax.Module {
	var out []*syntax.Module
	for _, m := range c.prog.Std {
		out = append(out, m)
	}
	for _, name := range c.prog.Order {
		out = append(out, c.prog.Modules[name])
	}
	return out
}

func (c *Checker) ctx(m *syntax.Module) *modCtx {
	mc := &modCtx{c: c, m: m, session: c.session, isApp: m.Name == "App"}
	if p, ok := c.pages[m.Name]; ok {
		mc.page = p
		if p.Requires != "" && c.session != nil {
			mc.session = narrow(c.session, p.Requires)
		}
	}
	return mc
}

// narrow returns a copy of the session record with field f's `Maybe a` replaced by `a`.
func narrow(s *types.Record, f string) *types.Record {
	out := &types.Record{Module: s.Module, Name: s.Name}
	for _, fl := range s.Fields {
		if fl.Name == f {
			if un, ok := fl.Type.(*types.Union); ok && un.Name == "Maybe" && len(un.Args) == 1 {
				fl = types.Field{Name: fl.Name, Type: un.Args[0], HasDefault: fl.HasDefault}
			}
		}
		out.Fields = append(out.Fields, fl)
	}
	return out
}

func (c *Checker) key(mod, name string) string { return mod + "." + name }

func (c *Checker) errorf(code string, span diag.Span, format string, args ...any) *diag.Diagnostic {
	return c.bag.Error(code, span, format, args...)
}

// declareStd maps Prelude's opaque types to their canonical representation.
func (c *Checker) declareStd() {
	for _, t := range []*types.Con{types.Int, types.Float, types.Bool, types.Text, types.Unit, types.Date, types.Time, types.Never, types.Error, types.Page} {
		c.types[c.key("Prelude", t.Name)] = t // canonical singletons, so identity comparisons (e.g. `ft == types.Time`) hold
	}
}

// isCon reports whether t is the builtin type constructor called name (compared by name, not identity).
func isCon(t types.Type, name string) bool {
	c, ok := t.(*types.Con)
	return ok && c.Name == name
}
