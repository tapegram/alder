// Package check is the type checker: a declaration pass followed by body checking.
package check

import (
	"slices"

	"alder/internal/diag"
	"alder/internal/elements"
	"alder/internal/names"
	"alder/internal/syntax"
	"alder/internal/types"
)

type Checker struct {
	prog   *names.Program
	bag    *diag.Bag
	u      *types.Unifier
	types  map[string]types.Type
	values map[string]types.Type
	ctors  map[string]*ctorInfo
	tables map[string]*types.TableInfo
	// tableKeys maps each app table's name to its key (indexTables, after pass 2): table names
	// are app-wide
	tableKeys map[string]string
	forms     map[string]*types.Record
	// paramRecs are the records of page and message parameters (by key): their defaults apply
	// to a literal too, since a parameter left out of a link is the URL left without it.
	paramRecs map[string]bool
	caps      map[string]map[string]types.Type
	// fakes are the accepted `fake` declarations by capability key ("Module.Cap");
	// fakeTables maps each fake's table ("Module.Table") to the fake's capability name, and
	// fakeUses holds the places E0021 was reported at
	fakes      map[string]*syntax.FakeDecl
	fakeTables map[string]string
	fakeUses   map[diag.Span]bool
	pages      map[string]*pageInfo
	session    *types.Record
	appMsg     *types.Union
	losers     map[syntax.Decl]bool
	// elements are the custom elements a program may use: the embedded `alder-ui` manifest's
	// and the app's elements/custom-elements.json's (loadElements); appElementsBad is set when
	// the app's file does not parse, so elements only it could declare are not reported again
	elements       *elements.Manifest
	appElementsBad bool
	// slotted are the `<dialog>`s an author wrote directly in an `<al-dialog>`: their children's
	// slots are the `<al-dialog>`'s (customElement checks them), not misplaced (misplacedSlots)
	slotted  map[*syntax.Element]bool
	defaults []pendingDefault // field defaults, checked after value signatures // duplicate declarations (E0009), skipped after declareTypeNames

	// typesDefined is set once every record has its fields (after pass 2); early are the generic
	// record instances made before, refreshed then.
	typesDefined bool
	early        []*types.Record
	// anonDefaults are the anonymous-record field defaults already registered (anonDefault);
	// defaultsChecked is set once checkDefaults has run.
	anonDefaults    map[*syntax.Field]bool
	defaultsChecked bool
	// widenedFrom is the narrowed session type of each expression in Result.Widened
	widenedFrom map[syntax.Expr]types.Type
	// ambiguousErr holds the constructor references reported as both a constructor and a table
	// (E0002, resolveByType): their type is Err, so nothing is reported about them again
	ambiguousErr map[*syntax.CtorRef]bool

	// scopedUses are the scoped Db operations of the body being checked (checkScopedTables);
	// scopedReported the places E0026 was reported at
	scopedUses     []scopedUse
	scopedReported map[diag.Span]bool
	// scopedDirect maps each value ("Module.name") of a page module that uses a scoped
	// operation itself (or another module's value that does, or an unscoped operation on a
	// siloed table) to the operation per partition key it needs narrowed ("user" → "Db.mine");
	// scopedEdges are its module-internal references (from, to); scopedUsing is their closure
	// (propagateScoped). insertRows are the scoped inserts insertScopedCall saw, by reference,
	// and insertOps the op (“Db.insertMine”) of each row literal written in one.
	scopedDirect map[string]map[string]string
	scopedEdges  [][2]string
	scopedUsing  map[string]map[string]string
	insertRows   map[*syntax.Var]bool
	insertOps    map[*syntax.RecordLit]string
	// M15: hasSession says App declares `session`; via maps each valid `via` session field to
	// its membership table ("Membership.Membership"), silos each silo key to its tables
	// ("Project.Project"), siloOf each siloed table to its key, choose each `choose` key to
	// its page.
	hasSession bool
	via        map[string]string
	silos      map[string][]string
	siloOf     map[string]string
	choose     map[string]string

	usesSessionPut bool // some module calls Session.put
	opts           Options
	// poisonedTop are the InferModule values whose inferred type is poisoned, and
	// usedPoisoned is set when a body being inferred uses one of them
	poisonedTop  map[string]bool
	usedPoisoned bool
	// forwardRef is the first sig-less value of the inferred module used, by the body being
	// inferred, before its own definition
	forwardRef string
	res        *Result
	// bodyExprs are the expressions recorded since the last body ended (reportHoles), for
	// checkInferredDictKeys
	bodyExprs []syntax.Expr
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
	// shown are the fields with an <Errors field>; formErrors is set by an <Errors form />
	shown      map[string]bool
	formErrors bool
	// wrapped maps each field a wrapped input element shows to its tag; errorsFor are the
	// `<Errors field>` written in the form (m1: one of a wrapped field is redundant, E0028)
	wrapped   map[string]string
	errorsFor []errorsAt
	msg       *syntax.CtorRef // the submit message's constructor (W0005), nil when not a message
}

// errorsAt is an `<Errors field="…">` in a form: its field and where it is.
type errorsAt struct {
	field string
	span  diag.Span
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
	value    string               // the top-level value whose body is being checked, "" for none
	testBody syntax.Expr          // the body of the `test` being checked (inTest)
	testName string               // the name of the `test` being checked (inTest)
	runArgs  map[syntax.Expr]bool // the arguments of `Test.run` calls (their mismatches get a hint)
	fake     string               // the capability whose `fake` entries are being checked
	// cookieLit is the record literal `Test.login` is applied to: the cookie's value, so the
	// session's defaults apply to it (a boundary, like an insert or a form).
	cookieLit *syntax.RecordLit
	// options is the env a `<Select options={…}>` expression is checked in, while it is: the
	// expression may use only the page view's parameters (and what it binds itself), since the
	// runtime computes it again, from the model, when the form is submitted.
	options *env
	// optionsBad are the names an options expression used that it may not, reported once each.
	optionsBad map[string]bool
}

// Options adjust what Program accepts.
type Options struct {
	// InferModule names the module whose top-level values written without a signature have it
	// inferred and recorded in Result.Values, instead of reporting E0007 (the scratch module
	// of `alder eval` and the REPL); unsolved type variables become type parameters (`[]` is
	// `List a`). That module is also treated like a `test` body for brand constructors (E0011):
	// it may use any module's. "" names none; every other module still needs its signatures.
	InferModule string
}

// Program checks prog, reporting into bag, and returns the types it learned.
func Program(prog *names.Program, bag *diag.Bag) *Result {
	return ProgramWith(prog, bag, Options{})
}

// ProgramWith is Program with options.
func ProgramWith(prog *names.Program, bag *diag.Bag, o Options) *Result {
	c := &Checker{prog: prog, bag: bag, u: &types.Unifier{Subst: types.Subst{}}, opts: o,
		types: map[string]types.Type{}, values: map[string]types.Type{}, ctors: map[string]*ctorInfo{},
		tables: map[string]*types.TableInfo{}, forms: map[string]*types.Record{}, paramRecs: map[string]bool{}, caps: map[string]map[string]types.Type{}, pages: map[string]*pageInfo{}, losers: map[syntax.Decl]bool{},
		anonDefaults: map[*syntax.Field]bool{}, fakes: map[string]*syntax.FakeDecl{}, fakeTables: map[string]string{}, fakeUses: map[diag.Span]bool{},
		res:         &Result{Types: map[syntax.Expr]types.Type{}, Binds: map[any]types.Type{}, DefaultModules: map[syntax.Expr]string{}, Widened: map[syntax.Expr]*types.Record{}, Wrapped: map[*syntax.Element]string{}, Rendered: map[*syntax.Element]string{}},
		widenedFrom: map[syntax.Expr]types.Type{}, ambiguousErr: map[*syntax.CtorRef]bool{}}
	c.declareStd()
	// pass 1: placeholders for every declared type in every module (app + std)
	for _, m := range c.allModules() {
		c.declareTypeNames(m)
	}
	quiet := c.reservePrelude()
	checkStart := len(bag.Items)
	c.loadElements()
	// App's Session exists from here on, so a type may name it (`{ s: Session }`); defineApp
	// fills its fields in pass 3.
	c.session = &types.Record{Module: "App", Name: "Session"}
	// pass 2: bodies of types, tables, forms
	for _, m := range c.allModules() {
		c.ctx(m).defineTypes()
	}
	c.typesDefined = true
	c.indexTables()
	for _, r := range c.early {
		r.Refresh()
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
	c.checkOwners()         // `@owner` and `@partition` columns need App's session
	c.checkPartitionDecls() // App's via and silo need every table's partition
	for _, name := range prog.Order {
		mc := c.ctx(prog.Modules[name])
		if prog.IsPageModule(name) {
			mc.definePage()
		}
		mc.defineCapabilities()
	}
	for _, name := range prog.Order {
		c.ctx(prog.Modules[name]).defineFakes()
	}
	c.checkChoose() // App's choose needs the pages
	c.checkPagePaths()
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
		c.propagateScoped()
	}
	c.checkWriteSets() // one store per Task (E0031) needs every body checked
	// App rules need every module's bodies checked first (Session.put usage)
	if app := prog.Modules["App"]; app != nil && !prog.Failed["App"] {
		c.ctx(app).checkApp()
	}
	c.checkInferredDictKeys() // anything recorded after the last body
	quietErrors(bag, checkStart, quiet)
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
		if p.Session != nil {
			mc.session = p.Session
		}
	}
	return mc
}

// narrow returns the session of `requires f, g` page module page: a record of its own,
// <page>.Session, with App's session fields but each of fs's `Maybe a` replaced by `a`. It is a
// distinct nominal type that widens implicitly to App.Session (see widen) and never narrows back.
func narrow(s *types.Record, fs []string, page string) *types.Record {
	out := &types.Record{Module: page, Name: "Session"}
	for _, fl := range s.Fields {
		if slices.Contains(fs, fl.Name) {
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
	for _, t := range []*types.Con{types.Int, types.Float, types.Bool, types.Text, types.Unit, types.Date, types.Time, types.Never, types.Error, types.Page, types.File, types.Bytes} {
		c.types[c.key("Prelude", t.Name)] = t // canonical singletons, so identity comparisons (e.g. `ft == types.Time`) hold
	}
}

// isCon reports whether t is the builtin type constructor called name (compared by name, not identity).
func isCon(t types.Type, name string) bool {
	c, ok := t.(*types.Con)
	return ok && c.Name == name
}
