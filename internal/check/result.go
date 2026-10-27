package check

import (
	"alder/internal/syntax"
	"alder/internal/types"
)

// Result is everything the checker learned about a program, for lowering to IR.
type Result struct {
	Types map[syntax.Expr]types.Type // every expression checked or inferred (after Apply)
	// Binds holds the type of every variable a program introduces, keyed by the AST node that
	// introduces it: *syntax.Ident for value and lambda parameters, *syntax.LetStmt for a block's
	// `let`, *syntax.PVar for a pattern variable, *syntax.PList for a list pattern's `...rest`
	// (the type of the whole list), and *syntax.HtmlFor for its loop variable (the element type).
	Binds   map[any]types.Type
	Values  map[string]types.Type            // "Module.name" → signature
	Decls   map[string]types.Type            // "Module.Type" → declared type (records, unions, brands, tables, forms)
	Tables  map[string]*types.TableInfo      // "Module.Table"
	Forms   map[string]*types.Record         // "Module.Form"
	Pages   map[string]*PageInfo             // page module name → its generated Params and Msg
	Ctors   map[string]*CtorInfo             // "Module.Ctor" → {Union, Arg}
	Caps    map[string]map[string]types.Type // capability module → name → type
	Session *types.Record
	AppMsg  *types.Union
	// DefaultModules maps the default expression of every anonymous-record field written in
	// source (types.Field.Default) to the module it is written in, where it is lowered.
	DefaultModules map[syntax.Expr]string
	// Widened maps each expression whose value is a `requires` page's narrowed Session used
	// where App's Session is expected to App's Session record: the expression's own recorded
	// type is the narrowed record, and Lower wraps it in a widening conversion.
	Widened map[syntax.Expr]*types.Record
	// Wrapped maps each built-in input element that wraps a form control (`<al-input
	// field="title">`, chapter 5) to the form component it is: "Input", "Textarea", "Select" or
	// "Checkbox" (an `<Input>` on a Bool field). An element absent from it is an ordinary custom
	// element.
	Wrapped map[*syntax.Element]string
	// Rendered maps each built-in element the renderer completes (chapter 13) to what it adds:
	// "button" or "a" for an `<al-button>` with `click`, `type` or `href` (the real control is
	// rendered inside it; the checker has refused a control among its children), "status" or "alert" for
	// an `<al-alert>` without `role` whose `variant` is a literal or absent (the role it gets).
	Rendered map[*syntax.Element]string
	// Via, Silos and Choose are App's valid M15 declarations: each `via` session field's
	// membership table ("Membership.Membership"), each silo key's tables in the order written
	// ("Project.Project"), each `choose` key's page module. Nil when there are none.
	Via    map[string]string
	Silos  map[string][]string
	Choose map[string]string
	// Arms are the arms of each `update` with their stores, from the one-store analysis
	// (writeset.go), by page module ("App" for App's update).
	Arms map[string][]ArmStore
	// Props are the property tests (a function or `Fuzz.forAll` body), by declaration; a plain
	// test has no entry.
	Props map[*syntax.TestDecl]PropInfo
	// FuzzAny is the concrete type at each `Fuzz.any` use (the `a` of its `Fuzzer a`), from which
	// the generator is derived. Every use checked without error has an entry.
	FuzzAny map[*syntax.Var]types.Type
	Holes   int            // count of W0002 reported
	U       *types.Unifier // for Apply on stored types
}

// PropKind is the form of a property test's body.
type PropKind int

const (
	PropFun    PropKind = iota + 1 // `\x y -> …`: each argument's generator is derived from its type
	PropForAll                     // `Fuzz.forAll gen f` or `Fuzz.forAllTask gen f`
)

// PropInfo is what the checker learned about a property test.
type PropInfo struct {
	Kind   PropKind
	Effect bool // the body's result is a Task Bool (a block-bodied lambda, or Fuzz.forAllTask)
	// Params are the lambda's parameters: the test body's for PropFun; for PropForAll the
	// parameters of forAll's function argument when it is a lambda written there, else nil.
	Params []*syntax.Ident
	// Types are the final, concrete argument types (after defaulting): one per parameter for
	// PropFun; for PropForAll one, the value type of forAll's generator.
	Types []types.Type
	// Defaulted[i] is set when a type variable left free in Types[i] was set to Int (spec §4.4).
	Defaulted []bool
}

// CtorInfo is a union constructor: the union it builds and its payload type (nil for none).
type CtorInfo struct {
	Union *types.Union
	Arg   types.Type
}

// PageInfo is a page module's generated types and declarations.
type PageInfo struct {
	Params *types.Record
	Msg    *types.Union
	// Requires are the session fields of the page's `requires` (["user", "org"]), nil for none.
	Requires []string
	// Roles are the page's `requires user, role R | S` constructors, nil for none (or when the
	// clause is in error: E0027).
	Roles []string
	// Silo is the key of the admin form `requires user, role R, silo k` (M15), "" for none (or
	// when the clause is in error).
	Silo string
	// File is true when the page's `view` returns File (chapter 6, "File pages"): a file page
	// has no messages, forms or `update` (E0034).
	File bool
	// Session is the narrowed session of a `requires` page (<page>.Session, see narrow), nil
	// for a page without `requires`, whose code sees App's session.
	Session *types.Record
	Decl    *syntax.PageDecl
	Msgs    map[string]*syntax.MsgDecl
	Module  string
}

type ctorInfo = CtorInfo
type pageInfo = PageInfo

// record notes e's type and returns it, so a walker can record on its return path.
func (c *Checker) record(e syntax.Expr, t types.Type) types.Type {
	c.res.Types[e] = t
	c.bodyExprs = append(c.bodyExprs, e)
	return t
}

// bind binds name to t in env and records t as the type of the binding site.
func (c *Checker) bind(env *env, site any, name string, t types.Type) {
	env.bind(name, t)
	c.res.Binds[site] = t
}

// result fills the declaration tables and applies the final substitution to every recorded type.
func (c *Checker) result() *Result {
	r := c.res
	for e, t := range r.Types {
		r.Types[e] = c.u.Apply(t)
	}
	for k, t := range r.Binds {
		r.Binds[k] = c.u.Apply(t)
	}
	for v, t := range r.FuzzAny {
		r.FuzzAny[v] = c.u.Apply(t)
	}
	r.Values, r.Decls, r.Tables, r.Forms, r.Caps = c.values, c.types, c.tables, c.forms, c.caps
	r.Pages, r.Ctors, r.Session, r.AppMsg, r.U = c.pages, c.ctors, c.session, c.appMsg, c.u
	r.Via, r.Silos, r.Choose = c.via, c.silos, c.choose
	return r
}
