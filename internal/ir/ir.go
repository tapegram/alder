// Package ir is the typed, resolved, desugared form of a checked Alder program: the input of
// the code generator. Lower builds it from a program the checker accepted without errors.
//
// What a consumer may assume everywhere:
//   - Every type is the checker's types.Type with brands erased to their base type (also inside
//     App, Fun, Record and Union arguments). Type parameters stay types.Param. No type is or
//     contains the poison type (types.Err): Lower panics with an internal error if one reaches it.
//   - Every name is resolved: a Var says whether it is a local, a top-level value or a stdlib
//     value; a Ctor points at its UnionType; a Comp at its component Value.
//   - There is no `|>`, no block, no pattern: pipes are calls, blocks are Lets and effect Binds
//     (see EffBind), and matches are decision trees over one tag at a time (see Match).
//   - Effects and database descriptions are data: the Query.*/Task.* combinators are the Eff*
//     nodes, the Db.* functions the Db* nodes (with Cond, Ord and ColumnRef for conditions and
//     orders), capability entries CapCall. The other effectful stdlib values (Session.put,
//     Log.*, Random.*, Time.now, Date.today, Http.*) stay ordinary Var/Call/Partial of VarStd.
//     The Prelude union Nav (Rerender | Redirect Page | Reject Text) is built with Ctor.
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
	AppUpdate *Value      // App's `update` (a Task effect); nil when App declares none
	Session   *RecordType // App's `session` record (empty when App declares none); nil without an App module
	Caps      []*Capability
	Brands    []*Brand // the app's brands, in module order then declaration order
	Login     string   // the path of the page App's `login` names; "" when App has none
}

// Brand is a brand type (`type Slug = brand Text where …`). Types in the IR are brand-erased;
// a brand survives where the runtime must check it: Brands lists the app's, and a record field
// declared with a brand (directly, or as the element of a Maybe or List) points at it
// (FieldT.Brand), for the decoders of forms and URL parameters. Check is the lowered `where`
// function (of type Base -> Bool), nil when the brand has none. The stdlib's refinement brands
// (Text.NonEmpty, Text.Email, Text.Url, Text.Secret, Int.Positive, Int.NonNegative) are Brands of
// module Text or Int with no Check: the runtime implements them; they are not in
// Program.Brands.
type Brand struct {
	Module, Name string
	Base         types.Type
	Check        Expr
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
// Brand is the brand the field's declared type is, or the brand of its Maybe's or List's
// element (`email: Maybe Text.Email`), nil for none; it is set on the fields of nominal records
// (forms, page and message parameters, the session, tables, records).
type FieldT struct {
	Name    string
	Type    types.Type
	Default Expr
	Brand   *Brand
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
//
// PayloadType is the payload's type as declared in the union, in terms of the union's type
// parameters: for a named record payload it is the record instance (`Boxed (Box a)` → Box a),
// whose type arguments Payload, the declaration, does not carry. Nil when Payload is nil.
type CtorT struct {
	Name        string
	Payload     *RecordType
	PayloadType types.Type
}

// Value is a top-level value or function. Params are the named parameters (fewer than the
// signature's when the body is itself a function value). Type is the declared signature.
//
// IsEffect is set when the signature's final result (after every parameter) is a Query or a
// Task; Effect then says which. A value that only mentions effects or database descriptions
// elsewhere in its type (`Select Todo`, `List (Task Unit)`) is not IsEffect.
type Value struct {
	Module, Name string
	Type         types.Type
	Params       []*Param
	Body         Expr
	IsComponent  bool
	IsEffect     bool
	Effect       EffKind
}

// Param is a named parameter of a Value or Lambda.
type Param struct {
	Name string
	Type types.Type
}

// Table is a `table` declaration; Record is also in its module's Types. Its fields' attributes:
// IDField is the @id field; Created and Updated the @created and @updated fields ("" when
// absent); Unique the @unique fields in declaration order; Refs maps each @ref field to the
// referenced table, module-qualified ("User.User").
type Table struct {
	Module, Name     string
	Record           *RecordType
	IDField          string
	Created, Updated string
	Unique           []string
	Refs             map[string]string
	InsertFields     []string // the fields `Db.insert` takes: all but @id, @created, @updated
}

// Form is a `form` declaration; Record is also in its module's Types.
type Form struct {
	Module, Name string
	Record       *RecordType
}

// Page is a page module. Params and the Msg union are in the module's Types. Model is the
// module's `Model` record (nil when Model is a union). Load is the page's `load` (a Query
// effect); Update its `update` (a Task effect), nil when the page declares none.
//
// Session is the session the page's code sees: App's Session record, or for a `requires f`
// page a record named Session in the page module (also in its Types, without field defaults)
// whose field f is narrowed from `Maybe T` to `T`; every type in the page's code that is the
// session names that record, and where App's Session is expected a Widen converts it. The
// runtime builds it after the requires check.
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

// ---- Effects
//
// A Query or Task is a value describing effects; the emitter builds it with the runtime's effect
// constructors and never runs anything while building. Kind fields come from the checked type of
// the node (an EffBind's from its block's type): EffQuery for `Query a`, EffTask for `Task a`.
// Every effect expression's type agrees with where it is used, with one exception: the Effect
// of a Task EffBind may be a Query (a Query run inside a Task block), which the emitter
// converts. Nowhere is a Task used where a Query is expected.

type EffKind int

const (
	EffQuery EffKind = iota
	EffTask
)

// EffPure is Query.succeed / Task.succeed: an effect producing Value and doing nothing.
type EffPure struct {
	Typed
	Kind  EffKind
	Value Expr
}

// EffBind runs Effect, binds its result to Param and continues with Body, an effect of the same
// Kind (the type of the whole node). It is the lowering of a block's `let x <- e`: the block
// `{ let a <- e1; let b = v; let c <- e2; r }` is EffBind(e1, a, Let(b, v, EffBind(e2, c, r))).
// Param is nil for `let _ <- e` (the result is discarded); otherwise Param.Type is the bound
// type (the annotation of `let x : T <- e` when written, which is the effect's result type).
// Effect's type is `Query T` or `Task T` for Param's T; it may be a Query in a Task bind.
type EffBind struct {
	Typed
	Kind   EffKind
	Effect Expr
	Param  *Param
	Body   Expr
}

// EffMap is Query.map / Task.map: Fn (a function a -> b) applied to the result of Effect.
type EffMap struct {
	Typed
	Kind       EffKind
	Fn, Effect Expr
}

// EffAll is Query.all / Task.all: Effects is an expression of type List (Query a) or
// List (Task a) (a List literal when written as one); the effects run in order.
type EffAll struct {
	Typed
	Kind    EffKind
	Effects Expr
}

// EffFail is Task.fail: a Task that fails with Message (of type Text).
type EffFail struct {
	Typed
	Message Expr
}

// EffNotFound is Query.notFound or Task.notFound: the effect fails with eff.ErrNotFound, which a
// route answers with 404 (App.notFound).
type EffNotFound struct{ Typed }

// ---- Database operations: the Db.* functions, resolved to the table they act on.
//
// The table always comes from the checked type (Table t, Select t, …), so it is known
// statically: the table argument itself (`Todo` in `Db.get Todo id`) is not kept (a table
// elsewhere is a TableRef). A Db function
// used as a value or partially applied is wrapped in a Lambda whose body is the node.

// DbGet is `Db.get Table ID`: a Query (Maybe t).
type DbGet struct {
	Typed
	Table *Table
	ID    Expr
}

// SelTerminal says what a DbSelect is: a Select value, or the Query running it.
type SelTerminal int

const (
	SelNone  SelTerminal = iota // a Select t value, not run (`Db.from T |> Db.where c`)
	SelAll                      // Db.all: Query (List t)
	SelFirst                    // Db.first: Query (Maybe t)
	SelCount                    // Db.count: Query Int
)

// DbSelect is a pipeline `Db.from T |> Db.where c… |> Db.orderBy o… |> Db.limit n |> Db.offset m`
// folded into one node, optionally run by Db.all/first/count (Terminal).
//
// Base is nil when the pipeline starts at `Db.from Table`; otherwise it is an expression of type
// Select t (a variable, a call) that this node extends: Base's own conditions and orders come
// first, then these. Where are expressions of type Condition t (Cond nodes, or any expression
// producing a condition) combined with and, in application order; Order are expressions of
// type Order t (usually Ord nodes) in application order (the first is the primary sort key).
// Limit and Offset are Int expressions, nil when not applied; when applied several times the
// last application is kept.
type DbSelect struct {
	Typed
	Table         *Table
	Base          Expr
	Where         []Expr
	Order         []Expr
	Limit, Offset Expr
	Terminal      SelTerminal
}

// DbRun is Db.all/first/count applied to a Select value that is not a pipeline written in place
// (a variable, a call): Select has type Select t; Terminal is never SelNone.
type DbRun struct {
	Typed
	Select   Expr
	Terminal SelTerminal
}

// DbInsert is `Db.insert Table Row`, a Task of the new row's id. Row is a record of the table's
// InsertFields (an anonymous record type; a record literal has every field, defaults filled).
type DbInsert struct {
	Typed
	Table *Table
	Row   Expr
}

// DbUpdate is `Db.update Table ID Fn`, a Task Unit; Fn has type t -> t.
type DbUpdate struct {
	Typed
	Table *Table
	ID    Expr
	Fn    Expr
}

// DbDelete is `Db.delete Table ID`, a Task Unit.
type DbDelete struct {
	Typed
	Table *Table
	ID    Expr
}

// TableRef is a table used as a value other than as the table argument of a Db function
// (`countIn Todo`, passed to a function taking `Table Todo`). Its type is Table t.
type TableRef struct {
	Typed
	Table *Table
}

// ColumnRef is a column `Todo.owner` of Table (Field is one of its record's fields), of type
// Column t a.
type ColumnRef struct {
	Typed
	Table *Table
	Field string
}

// Cond is a condition (type Condition t). Kind is one of:
//
//	eq ne lt le gt ge contains in   Col compared with Value (for in, Value is a List of the
//	                                column's type; for contains, Text)
//	isNothing isJust                Col is a Maybe column; Value is nil
//	and or                          Subs are the conditions when written as a list literal;
//	                                otherwise Subs is nil and Value is the List (Condition t)
//	not                             Subs has exactly one condition
//
// Col is nil exactly for and, or and not. A column method used as a value or partially applied
// (`List.map Todo.owner.eq ids`) is wrapped in a Lambda whose body is the Cond.
type Cond struct {
	Typed
	Kind  string
	Col   *ColumnRef
	Value Expr
	Subs  []Expr
}

// Ord is `Todo.created.asc` / `.desc` (type Order t).
type Ord struct {
	Typed
	Col  *ColumnRef
	Desc bool
}

// CapCall is a call of a capability entry: Entry of Cap applied to Args, the entry's parameters
// in order (none for an entry that is itself a Query or Task). Its type is the entry's result,
// a Query or Task. An entry used as a value or partially applied is wrapped in a Lambda.
type CapCall struct {
	Typed
	Cap   *Capability
	Entry string
	Args  []Expr
}
