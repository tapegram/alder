// Package ir is the typed, resolved, desugared form of a checked Alder program: the input of
// the code generator. Lower builds it from a program the checker accepted without errors.
//
// What a consumer may assume everywhere:
//   - Every type is the checker's types.Type with brands erased to their base type (also inside
//     App, Fun, Record and Union arguments). Type parameters stay types.Param.
//   - Every name is resolved: a Var says whether it is a local, a top-level value or a stdlib
//     value; a Ctor points at its UnionType; a Comp at its component Value.
//   - There is no `|>`, no block, no pattern: pipes are calls, blocks are Lets, and matches are
//     decision trees over one tag at a time (see Match).
//   - Nothing here is Go-specific.
package ir

import "alder/internal/types"

// Program is a whole lowered app.
type Program struct {
	Modules   []*Module // in names.Program.Order (dependencies first)
	Tables    []*Table
	Forms     []*Form
	Pages     []*Page
	AppMsgs   []*Msg      // App's `msg` declarations, in order
	AppUpdate *Value      // nil in M3a: App.update is an effect
	Session   *RecordType // App's `session` record (empty when App declares none); nil without an App module
	Caps      []*Capability
}

// Module holds a module's declarations.
//
// Types lists every nominal type the module declares: its records and unions, the records of its
// tables and forms, App's Session record and App's Msg union (with the Msg payload records before
// it), and for a page module its generated Params record, Msg payload records and Msg union.
// Program.Tables, Forms, Pages and Session point at these same RecordTypes, so a consumer
// declares each type once by walking Types. Brands have no TypeDecl: they are erased.
type Module struct {
	Name       string
	Types      []*TypeDecl
	Values     []*Value // lowercase top-level values, in declaration order
	Components []*Value // capitalized top-level values (used as elements), in declaration order
	// Skipped names the values not lowered because their type involves an effect (Query, Task)
	// or a database description (Table, Select, Condition, Order, Column); M3b lowers them.
	Skipped []string
}

// TypeDecl is one nominal type: exactly one of Record and Union is set.
type TypeDecl struct {
	Module, Name string
	Record       *RecordType
	Union        *UnionType
}

// RecordType is a record's shape. Name == "" means an anonymous (structural) record. Params are
// a nominal record's type parameter names (`type Box a = { item: a }` → ["a"]); its fields
// mention them as types.Param. A use of the record at arguments (`Box Int`) is the types.Record
// instance with Args, whose fields have the parameters substituted.
type RecordType struct {
	Module, Name string
	Params       []string
	Fields       []FieldT // in declaration order
}

// FieldT is a record field. Default is the lowered default expression (`done: Bool = False`), nil
// when the field has none; Lower already fills defaults into record literals that omit them.
type FieldT struct {
	Name    string
	Type    types.Type
	Default Expr
}

// UnionType is a union's declaration. Params are its type parameter names (`Maybe a` → ["a"]).
// Unions of the Prelude (Maybe, Result, Nav) are runtime-provided: they appear as Ctor.U but in
// no Module's Types.
type UnionType struct {
	Module, Name string
	Params       []string
	Ctors        []CtorT // in declaration order; a ctor's index is its tag number
}

// CtorT is a constructor. Payload is nil for none; a record payload is that record (anonymous, or
// named for a message's generated `<Msg>Params`); any other payload T is wrapped as the anonymous
// record { value: T }.
type CtorT struct {
	Name    string
	Payload *RecordType
}

// Value is a top-level value or function. Params are the named parameters (fewer than the
// signature's when the body is itself a function value). Type is the declared signature.
type Value struct {
	Module, Name string
	Type         types.Type
	Params       []*Param
	Body         Expr
	IsComponent  bool
}

// Param is a named parameter of a Value or Lambda.
type Param struct {
	Name string
	Type types.Type
}

// Table is a `table` declaration; Record is also in its module's Types.
type Table struct {
	Module, Name string
	Record       *RecordType
	IDField      string
	InsertFields []string // the fields `Db.insert` takes: all but @id, @created, @updated
}

// Form is a `form` declaration; Record is also in its module's Types.
type Form struct {
	Module, Name string
	Record       *RecordType
}

// Page is a page module. Params and the Msg union are in the module's Types. Model is the
// module's `Model` record (nil when Model is a union). Load and Update are nil in M3a.
//
// Session is the session the page's code sees: App's Session record, or for a `requires f`
// page a record named Session in the page module (also in its Types, without field defaults)
// whose field f is narrowed from `Maybe T` to `T`; every type in the page's code that is the
// session names that record. The runtime builds it after the requires check.
type Page struct {
	Module, Path string
	Params       *RecordType
	Requires     string // session field the page requires, "" for none
	Session      *RecordType
	Model        *RecordType
	Msgs         []*Msg
	View         *Value
	Load, Update *Value
}

// Msg is a `msg` declaration. Params is the payload record of the message's constructor in its
// Msg union (named "<Msg>Params"): the declared parameters, then a field `form` holding the form
// when Form is set. Params is nil when the message has neither parameters nor a form.
type Msg struct {
	Name   string
	Params *RecordType
	Form   *Form
}

// Capability is a `capability` declaration: Entries are its functions with their types.
type Capability struct {
	Module, Name string
	Entries      []FieldT
}

// ---- Expressions

// Expr is an expression; Type is its (brand-erased) type.
type Expr interface{ Type() types.Type }

// Typed carries an expression's type; every expression node embeds it.
type Typed struct{ Ty types.Type }

func (t Typed) Type() types.Type { return t.Ty }

type LitKind int

const (
	LitInt LitKind = iota
	LitFloat
	LitBool
	LitText
	LitUnit
)

// Lit is a literal; the field matching Kind holds its value.
type Lit struct {
	Typed
	Kind  LitKind
	Int   int64
	Float float64
	Bool  bool
	Text  string
}

type VarKind int

const (
	VarLocal  VarKind = iota // parameter, let, lambda parameter, pattern binding; Module is ""
	VarGlobal                // top-level value Module.Name of the app
	VarStd                   // stdlib value, named as in its stdlib file: Module "List", Name "map"; `not` is Prelude.not
)

// Var is a reference to a named value.
type Var struct {
	Typed
	Kind         VarKind
	Module, Name string
}

// Call is a saturated call: len(Args) equals the callee's parameter count.
type Call struct {
	Typed
	Fn   Expr
	Args []Expr
}

// Partial is an unsaturated application: Fn applied to Given, still waiting for Missing more
// arguments (it is a function of those). Given are the first parameters, in order.
type Partial struct {
	Typed
	Fn      Expr
	Given   []Expr
	Missing int
}

// Lambda is an anonymous function.
type Lambda struct {
	Typed
	Params []*Param
	Body   Expr
}

// Record is a record value. Fields are in T's declaration order and complete, except that a
// message payload omits its `form` field: the runtime supplies the submitted form.
type Record struct {
	Typed
	T      *RecordType
	Fields []FieldInit
}

// FieldInit is one field of a Record or Update.
type FieldInit struct {
	Name  string
	Value Expr
}

// Update is `{ base | f: v }`: a copy of Base with Fields replaced.
type Update struct {
	Typed
	Base   Expr
	Fields []FieldInit
}

// Field is a record field access.
type Field struct {
	Typed
	Expr Expr
	Name string
}

// Ctor builds a union value. Payload is nil when the constructor has none; otherwise it has the
// type of the ctor's payload record (a Record literal when written as one, and always a Record
// { value: … } for a wrapped non-record payload).
type Ctor struct {
	Typed
	U       *UnionType
	Name    string
	Payload Expr
}

// List is a list literal.
type List struct {
	Typed
	Elems []Expr
}

// PageRef is a link to a page (`Todos.Show { id: x }`), of type Page. Params is the page's
// Params record, nil when the page has no parameters and is written bare.
type PageRef struct {
	Typed
	Page   *Page
	Params Expr
}

// Hole is a typed hole `?name`, which `alder build` rejects before code generation.
type Hole struct {
	Typed
	Name string
}

type MatchKind int

const (
	MatchUnion MatchKind = iota // Case.Tag is a constructor name; Binds name payload fields
	MatchBool                   // Case.Tag is "True" or "False"; no Binds
	MatchList                   // Case.Tag is "nil" or "cons"; cons Binds name "head" and "rest"
	MatchLit                    // Case.Tag is an Int literal in decimal, a Text literal's raw text, or a Float literal formatted with strconv 'g' (shortest round-trip)
)

// Match is one level of a decision tree: Subject is tested once, the first Case whose Tag matches
// runs, else Default. Default is nil when the Cases cover every tag of Kind (every constructor,
// both Bools, nil and cons); a Lit match always has one. Nested patterns were flattened by Lower:
// see lowerMatch in lower_match.go for the rules.
type Match struct {
	Typed
	Kind    MatchKind
	Subject Expr
	Cases   []Case
	Default Expr
}

// Case is one tag of a Match. Binds are bound from the matched value before Body runs.
type Case struct {
	Tag   string
	Binds []Bind
	Body  Expr
}

// Bind binds Name to a part of the matched value: for a union, payload field Field (Field "value"
// for a wrapped payload, Field "" for the whole payload record); for a list cons, Field "head" or
// "rest".
type Bind struct {
	Name  string
	Field string
	Type  types.Type
}

// If is a conditional expression.
type If struct {
	Typed
	Cond, Then, Else Expr
}

// Let binds Name (of type T) to Value in Body. Name may be "_".
type Let struct {
	Typed
	Name  string
	T     types.Type
	Value Expr
	Body  Expr
}

// Op is a binary operator: Kind is its token (`+ - * / % ++ == != < <= > >= && || ??`; never
// `|>`). OperandT is the left operand's type, which decides `++` (Text or List), `==` (structural
// equality) and `??` (it is the Maybe).
type Op struct {
	Typed
	Kind     string
	L, R     Expr
	OperandT types.Type
}

// Widen converts Expr, a `requires` page's Session (of the page's narrowed record Page.Session),
// to App's Session (its Type): the field Page.Requires becomes `Just` of its value, every other
// field is copied. The checker accepts a page's session where App's is expected, never the
// reverse.
type Widen struct {
	Typed
	Expr Expr
	Page *Page
}

// Str is string interpolation: the concatenation of Parts, each of type Text.
type Str struct {
	Typed
	Parts []Expr
}

// ---- HTML (every node has type Html m)
//
// An HTML child, branch or arm body is one of the nodes below, or a Let whose Body is one (a
// variable arm of an HMatch binds its name with a Let around the arm's HTML).

type AttrKind int

const (
	AttrText   AttrKind = iota // Value is Text
	AttrBool                   // Value is Bool: present when True (a bare attribute is Lit True)
	AttrMaybe                  // Value is Maybe Text: present when Just
	AttrHref                   // Value is a Page (PageRef or Url.external)
	AttrClick                  // Value is a message: a Ctor of a page's Msg union or App's
	AttrSubmit                 // as AttrClick, on a <form>
)

// HAttr is an attribute of an Elem or FormEl.
type HAttr struct {
	Name  string
	Kind  AttrKind
	Value Expr
}

// Elem is a plain HTML element.
type Elem struct {
	Typed
	Tag      string
	Attrs    []HAttr
	Children []Expr
}

// Comp is a component used as an element: a call of Fn with Props plus Children (the `children`
// field of the props record, which Props does not include; nil when the element has none).
type Comp struct {
	Typed
	Fn       *Value
	Props    *Record
	Children []Expr
}

// HText is literal text in HTML, not escaped yet.
type HText struct {
	Typed
	Text string
}

type HExprKind int

const (
	HExprText HExprKind = iota // Expr is Text: rendered as escaped text
	HExprHtml                  // Expr is Html m
	HExprList                  // Expr is List (Html m)
)

// HExpr is an expression `{…}` in HTML (also an arm or branch body that is not itself HTML
// syntax, such as `Html.none`).
type HExpr struct {
	Typed
	Expr Expr
	Kind HExprKind
}

// HFor is `{for Var in List -> Body}`; T is the element type.
type HFor struct {
	Typed
	Var  string
	T    types.Type
	List Expr
	Body Expr
}

// HIf is `{if Cond then Then else Else}` in HTML.
type HIf struct {
	Typed
	Cond, Then, Else Expr
}

// HMatch is `{match …}` in HTML: a Match whose bodies are HTML (inner matches produced by
// flattening are HMatch too).
type HMatch Match

type InputKind int

const (
	InputInput InputKind = iota
	InputTextarea
	InputSelect
	InputErrors
)

// Input is a form control bound to the enclosing FormEl's form: `<Input field="title" …>`.
// Field is "" only for `<Errors form />` (BareForm). Label is the literal label ("" when absent);
// LabelExpr is set instead when the label is computed. Value may be nil.
type Input struct {
	Typed
	Kind      InputKind
	Form      *Form
	Field     string
	Label     string
	LabelExpr Expr
	Value     Expr
	BareForm  bool
}

// FormEl is `<form submit={…}>`. Attrs include the AttrSubmit attribute holding the message
// value. Msg is the message it submits and Page the page declaring it (nil for an App message);
// both are nil when the submit value is not a constructor written in place.
type FormEl struct {
	Typed
	Msg      *Msg
	Page     *Page
	Attrs    []HAttr
	Children []Expr
}
