package check

import (
	"alder/internal/elements"
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
	Routes  map[string][]*RouteInfo          // route module name → its routes, in declaration order
	Ctors   map[string]*CtorInfo             // "Module.Ctor" → {Union, Arg}
	Caps    map[string]map[string]types.Type // capability module → name → type
	Session *types.Record
	// Elements is the merged custom-elements manifest the checker used (the built-in
	// `alder-ui` one and the app's), for the editor's completion and hover.
	Elements *elements.Manifest
	// DefaultModules maps the default expression of every anonymous-record field written in
	// source (types.Field.Default) to the module it is written in, where it is lowered.
	DefaultModules map[syntax.Expr]string
	// Widened maps each expression whose value is a `requires` route's narrowed Session used
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
	// ("Project.Project"), each `choose` key's GET route as `Module.Route`. Nil when there are
	// none.
	Via    map[string]string
	Silos  map[string][]string
	Choose map[string]string
	// Arms are the write routes of each module with their stores, from the one-store analysis
	// (writeset.go): one arm per write route, named after it.
	Arms map[string][]ArmStore
	// Props are the property tests (a function or `Fuzz.forAll` body), by declaration; a plain
	// test has no entry.
	Props map[*syntax.TestDecl]PropInfo
	// FuzzAny is the type at each `Fuzz.any` use (the `a` of its `Fuzzer a`), from which the
	// generator is derived: concrete and derivable for every use without an error. A use that got
	// E0038 has an entry too (its type is not concrete or has no generator), but then the build
	// stops, so nothing derives from it.
	FuzzAny map[*syntax.Var]types.Type
	// TestOnly are the test helpers (#4), "Module.name" → the first thing that makes each one
	// (“`Test.fetch`”, or “`fetchText`, a test helper”): values only tests and other helpers use,
	// which a production build omits.
	TestOnly map[string]string
	Holes    int            // count of W0002 reported
	U        *types.Unifier // for Apply on stored types
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

type ctorInfo = CtorInfo

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
	r.Routes, r.Ctors, r.Session, r.U = c.routes, c.ctors, c.session, c.u
	r.Via, r.Silos, r.Choose = c.via, c.silos, c.choose
	r.Elements = c.elements
	return r
}
