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
	Fakes     []*Fake  // the `fake` declarations, in module order; test builds only use them
	Brands    []*Brand // the app's brands, in module order then declaration order
	Login     string   // the path of the page App's `login` names; "" when App has none
	Deploy    *Deploy  // App's `deploy` declaration (chapter 16); nil when App has none. Data only: nothing in the generated program reads it
	// AppArms are the arms of App's `update` with the store each one's Task writes (M15); nil
	// when App declares no update.
	AppArms []*Arm
	// Partitions and silos (M15, book 06 "Partitions", 08). Via maps each session field
	// declared `via T` to its membership table T (one `@ref` to the user table, one to the
	// key's table). Silos maps each `silo k { … }` key to its tables, in the order written;
	// a table in no silo is in the shared store. Choose maps each `choose k = Page` key to that
	// page. Each is nil when the app declares none.
	Via    map[string]*Table
	Silos  map[string][]*Table
	Choose map[string]*Page
}

// Store is where a Task writes (M15): the shared store (StoreShared, Silo "") or the store of
// one silo key's value (StoreSilo(k)), chosen at run time by the session's k.
type Store struct{ Silo string }

// StoreShared is the shared store (the app's database).
var StoreShared = Store{}

// StoreSilo is the store of silo key k: one database per value of the session's k.
func StoreSilo(k string) Store { return Store{Silo: k} }

// String is "shared" or "silo k".
func (s Store) String() string {
	if s.Silo == "" {
		return "shared"
	}
	return "silo " + s.Silo
}

// Arm is one arm of an `update` (a page's or App's) and the store its Task writes, from the
// checker's one-store-per-Task analysis (E0031): every table the arm writes (insert, update,
// delete, scoped or not, through helpers too) is in Store. An arm that writes nothing is
// StoreShared. Msg is the message constructor the arm matches; "" for an arm that matches every
// other message (`_ ->`, a variable), and for an update whose body is not a `match` on its
// message (one Arm for all). Several arms of one constructor are one Arm. Pos is the arm's
// pattern (the update's body for Msg "").
type Arm struct {
	Msg   string
	Store Store
	Pos   Pos
}

// Partition is a table's partition (M15): the session field Key whose value the table's
// `@ref` column Column holds in every row (`org: Org.OrgId @ref Org @partition org`; `@owner`
// is Key "user").
type Partition struct{ Key, Column string }

// Deploy is App's `deploy { … }`: where the app ships and what the gate demands (chapter 16).
// Envs are the fields whose value is a target, by name. Floor is the minimum mutation score
// (0..1), meaningful only when FloorSet (the default is the score of the last successful deploy).
// Checks are commands run from the app root; Requires the environment-variable names the target
// must provide; Backup the S3 location of the database's backup (nil: none); Keep how many builds
// to keep (0: the default); Proxy "Caddy" (the default) or "None"; Image the registry path
// Render environments push to ("" when not written: the command defaults it from the git remote).
type Deploy struct {
	Envs     map[string]Target
	Floor    float64
	FloorSet bool
	Checks   []string
	Requires []string
	Backup   *Backup
	Keep     int
	Proxy    string
	Image    string
}

// Target is one environment's destination. Kind is "ssh" (Host, Domain, Path, User, Port),
// "fly" (Region, App), "render" (Region, Service, Plan, Instances) or "script" (ScriptPath). Port is the decimal text of the SSH port, ""
// for the default (22); User "" takes the user in Host (`user@host`) or ssh's own default.
type Target struct {
	Kind                                                    string
	Host, Domain, Path, User, Port, Region, App, ScriptPath string
	// Service and Plan are a Render environment's service name and plan ("" when unset: Render's
	// default); Instances its numInstances (0 when unset: means 1).
	Service, Plan string
	Instances     int
	// Database is an Ssh or Render environment's `database: Turso "org" { … }` (M15): the app's data on
	// Turso instead of a file on the box; nil for the file.
	Database *Database
}

// Database is `database: Turso "org" { group?, app, sample? }`: the organization, the group the
// databases are created in ("default" when not given), the app's name on Turso (the shared
// database's name and the prefix of its silos' databases), and how many of the largest silo
// databases a deploy tries its migrations on first (Sample; 0: the default, 5).
type Database struct {
	Kind            string // "turso"
	Org, Group, App string
	Sample          int
}

// Backup is `backup: S3 "s3://…"`: Kind is "s3", URL the location.
type Backup struct{ Kind, URL string }

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
	Tests      []*Test  // `test` declarations, in declaration order
}

// Test is a `test` declaration. Effect says its Body is a Task Bool (a block, run as a Task);
// otherwise Body is a Bool. The test's recorded `==` is a TestEq: a pure test's Body when it is
// an `==`, or the Value of the EffPure that ends an effect test's straight-line block (reached
// through EffBind and Let bodies) when that is an `==`.
type Test struct {
	Module, Name string
	File         string // the test's file (as the span names it, "app/Todo.alder") and
	Line, Col    int    // the position of its `test` keyword, for alder test --json
	Body         Expr
	Effect       bool
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
//
// On a table's fields only: DefaultLit is the default as an SQL literal ("0", "1", "'Reader'",
// "NULL"; see SQLLiteral), nil when the field has no default; RenamedFrom is the field's old
// name from `@renamed "old"`, "" for none.
type FieldT struct {
	Name        string
	Type        types.Type
	Default     Expr
	Brand       *Brand
	DefaultLit  *string
	RenamedFrom string
	Pos         Pos // the field's declaration; zero for a field Lower made up
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
// referenced table, module-qualified ("User.User"), and OnDelete each @ref field with a delete
// rule other than the default @restrict to it ("cascade" or "setNull").
type Table struct {
	Module, Name     string
	Record           *RecordType
	IDField          string
	Created, Updated string
	Unique           []string
	Refs             map[string]string
	OnDelete         map[string]string
	InsertFields     []string // the fields `Db.insert` takes: all but @id, @created, @updated
	RenamedFrom      string   // the table's old name from `table Post @renamed "Article"`, "" for none
	// Partition is the table's `@partition k` column (`@owner` is `@partition user`), nil for
	// none: a `@ref` whose type is the session field k's without its Maybe (never Maybe itself).
	// The column is in InsertFields (Db.insert takes it), and the scoped insert's row is
	// InsertFields without it. It changes nothing in the stored schema (migrate ignores it).
	Partition *Partition
}

// Owner is the column of a table partitioned by the session's `user` (M11's `@owner`), "" for
// any other table.
func (t *Table) Owner() string {
	if t.Partition != nil && t.Partition.Key == "user" {
		return t.Partition.Column
	}
	return ""
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
	Requires     []string // session fields the page requires (`requires user, org`), nil for none
	File         bool     // view returns File: a raw GET answer, no messages
	RequiresPos  Pos      // the `requires f` clause; zero when Requires is nil
	// Silo is the key of the admin form `requires user, role Admin, silo k` (M15), "" for none:
	// the page's session field k is its parameter k (narrowed in Session, like a required
	// field), not the cookie's, and no `via` check applies (the role check gates the page).
	// SiloPos is the clause.
	Silo    string
	SiloPos Pos
	// Arms are the page update's arms with their stores (see Arm), nil without an update.
	Arms []*Arm
	// Roles are the constructors of `requires user, role R | S` (["R", "S"]), nil for none:
	// constructors of the union of App's session field `role` (never Maybe; the page session's
	// `role` is that union as App's is). The route answers 404 when the session's role is none
	// of them, after the `requires user` redirect and before load. RolesPos is the `role R…`
	// clause; zero when Roles is nil. Requires starts with "user" whenever Roles is set.
	Roles        []string
	RolesPos     Pos
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

// Fake is `fake Cap { tables; entries }`: an in-memory implementation of capability Cap that a
// test installs with `Test.fake Cap` (TestFake). It belongs to test builds only.
//
// Tables are the fake's own tables, lowered like an app table but test-only: they are NOT in
// Program.Tables (so migrations, the schema snapshot and E0019 never see them), and their
// records are NOT in their Module's Types: a consumer that emits a fake declares each table's
// Record itself. A fake's table may @ref an app table (Refs names it as usual).
//
// Entries has one Value per Capability.Entries[i], in the same order: Name is the entry's name,
// Type the entry's (brand-erased) type, Params the parameters the definition names, Body the
// lowered definition; IsEffect is always set. Entries are in no Module's Values.
type Fake struct {
	Module     string
	Capability *Capability // the capability it implements (same module)
	Tables     []*Table
	Entries    []*Value
}

// ---- Expressions

// Pos is a source position (1-based line and column) of the expression a node was lowered from.
// The node kinds a mutant can target carry one (If, Match, Op, Lit, Update, Ctor, Call, EffBind,
// DbSelect, DbInsert, DbUpdate, DbDelete, the scoped Db*Scoped nodes, Cond, Ord); it is the zero Pos for a node Lower made
// up (a default filled into a literal keeps its declaration's). Dump never prints it.
type Pos struct{ Line, Col int }

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
	Pos   Pos
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
	Pos  Pos // the callee's
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
	Pos    Pos
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
	Pos     Pos
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
	Pos     Pos
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
	Pos              Pos
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
	Pos      Pos
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
	AttrHref                   // Value is a Page (PageRef or Url.external): an href, or a src given a page
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
	Raw  bool // the body of a <script> or <style>: rendered unescaped (html.RawText)
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
	// Options is a select's `options={…}`, a `List { value: …, label: Text }` of the values a
	// field of Text (or a brand of it) may take; nil for a union's select, whose options are
	// its constructors.
	Options  Expr
	BareForm bool
	// Wrap is the built-in input element written around the control (`al-input` for
	// `<al-input field="title">`), "" for a form component; WrapAttrs are that element's own
	// attributes, as written (not `field`, `label`, `value`).
	Wrap      string
	WrapAttrs []HAttr
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
	Pos    Pos // the `let … <-` statement's
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
	Pos           Pos // the pipeline's first step (`Db.from`, or the step extending Base)
	Table         *Table
	Base          Expr
	Where         []Expr
	Order         []Expr
	Limit, Offset Expr
	Terminal      SelTerminal
	// the position of each step: WherePos[i] is the `Db.where` applying Where[i], OrderPos[i]
	// the `Db.orderBy` applying Order[i], LimitPos and OffsetPos the kept `Db.limit`/`Db.offset`
	WherePos, OrderPos  []Pos
	LimitPos, OffsetPos Pos
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
	Pos   Pos
	Table *Table
	Row   Expr
}

// DbUpdate is `Db.update Table ID Fn`, a Task Unit; Fn has type t -> t.
type DbUpdate struct {
	Typed
	Pos   Pos
	Table *Table
	ID    Expr
	Fn    Expr
}

// DbDelete is `Db.delete Table ID`, a Task Unit.
type DbDelete struct {
	Typed
	Pos   Pos
	Table *Table
	ID    Expr
}

// ---- Scoped operations (M11, generalised in M15): the scoped Db functions of a table with a
// partition (Table.Partition, Key the session field it is partitioned by). The checker accepts
// them only where the session's Key is narrowed: in a page module whose page `requires` Key
// (load, update, view and helper values), or the admin form's `silo Key`, and in tests. Each is
// the unscoped operation with the predicate `Column = session.Key`. A row of another partition
// answers as a missing one (not found), never an error. Key is the table's partition key; Op is
// the function as written, for traces and messages: "Db.scoped" (any key) or M11's "Db.mine"
// (the family legal only on a table partitioned by `user`), and likewise for the others.

// DbScoped is `Db.scoped Table` (or `Db.mine Table`): a Select t value (Db.from Table filtered
// on the partition). It is never run itself: Db.where/orderBy/limit/offset over it make a
// DbSelect whose Base is the DbScoped, and Db.all/first/count over it (or over such a DbSelect)
// make a DbSelect or DbRun as for any Select value.
type DbScoped struct {
	Typed
	Pos   Pos
	Op    string
	Key   string
	Table *Table
}

// DbGetScoped is `Db.getScoped Table ID` (`Db.getMine`): a Query t, the row when it exists and
// is in the session's partition, else Query.notFound (the page answers 404, as for a missing
// row).
type DbGetScoped struct {
	Typed
	Pos   Pos
	Op    string
	Key   string
	Table *Table
	ID    Expr
}

// DbUpdateScoped is `Db.updateScoped Table ID Fn` (`Db.updateMine`), a Task Unit; Fn has type
// t -> t. A predicated read then a predicated write (`id = ? AND col = ?` in both), in the
// Task's transaction: no row found or changed is Task.notFound. It never writes the partition
// column.
type DbUpdateScoped struct {
	Typed
	Pos   Pos
	Op    string
	Key   string
	Table *Table
	ID    Expr
	Fn    Expr
}

// DbDeleteScoped is `Db.deleteScoped Table ID` (`Db.deleteMine`), a Task Unit: one DELETE with
// `id = ? AND col = ?`; no row deleted is Task.notFound.
type DbDeleteScoped struct {
	Typed
	Pos   Pos
	Op    string
	Key   string
	Table *Table
	ID    Expr
}

// DbInsertScoped is `Db.insertScoped Table Row` (`Db.insertMine`), a Task of the new row's id.
// Row is a record of the table's InsertFields without the partition column (an anonymous record
// type, defaults filled, as DbInsert's); the column is the session's Key.
type DbInsertScoped struct {
	Typed
	Pos   Pos
	Op    string
	Key   string
	Table *Table
	Row   Expr
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
//	eq ne lt le gt ge contains      Col compared with Value (for in, Value is a List of the
//	startsWith in                   column's type; for contains and startsWith, Text)
//	isNothing isJust                Col is a Maybe column; Value is nil
//	and or                          Subs are the conditions when written as a list literal;
//	                                otherwise Subs is nil and Value is the List (Condition t)
//	not                             Subs has exactly one condition
//
// Col is nil exactly for and, or and not. A column method used as a value or partially applied
// (`List.map Todo.owner.eq ids`) is wrapped in a Lambda whose body is the Cond.
type Cond struct {
	Typed
	Pos   Pos
	Kind  string
	Col   *ColumnRef
	Value Expr
	Subs  []Expr
}

// Ord is `Todo.created.asc` / `.desc` (type Order t).
type Ord struct {
	Typed
	Pos  Pos
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

// ---- Tests

// TestRun is `Test.run task`: a Task of the record { value: Result Text a, trace: List Text }
// (its type): it runs Task (a Task a) inside the enclosing test's Task, undoing its writes when
// it fails, and gives its outcome and the steps it performed.
type TestRun struct {
	Typed
	Task Expr
}

// TestFake is `Test.fake Cap`: a Task Unit (its type) that installs Fake in place of the
// capability's implementation for the rest of the enclosing test.
type TestFake struct {
	Typed
	Fake *Fake
}

// TestTrace is `Test.trace`: a Task (List Text) of the enclosing test's steps so far.
type TestTrace struct{ Typed }

// The browser steps of a test (module Test's special forms): each is a Task of the step's result
// record (its type), made by the test's browser (rtesting.BrowserOf) against the app's routes.

// TestLogin is `Test.login s`: a Task Unit storing Session (App's session record) in the
// browser's cookie.
type TestLogin struct {
	Typed
	Session Expr
}

// TestGet is `Test.get p` (Open unset: a Task { model: M, session, doc }, failing unless the
// page is shown; M is Page's Model), `Test.open p` (Open set: { answer, session, doc }) or
// `Test.fetch p` (Fetch set: the raw answer { status, contentType, body, headers }), for the
// page value Page with Params (nil for a page without parameters).
type TestGet struct {
	Typed
	Page   *Page
	Params Expr
	Open   bool
	Fetch  bool
}

// TestSubmit is `Test.submit F r`: Record (F's record) posted to Msg, the message that posts
// form Form, of Page (nil for App's form): a Task { answer, session, doc }. Params is the
// message's payload when the test wrote the message (`Test.submit (Save { id: id }) r`), nil
// when it wrote the form: the form is then found by the message alone.
type TestSubmit struct {
	Typed
	Form   *Form
	Msg    *Msg
	Page   *Page
	Params Expr
	Record Expr
}

// TestSubmitRaw is `Test.submitRaw F fields`: Fields (a List { name: Text, value: Text })
// posted as text to Msg, the message that posts form Form, of Page (nil for App's form);
// Params as for TestSubmit.
type TestSubmitRaw struct {
	Typed
	Form   *Form
	Msg    *Msg
	Page   *Page
	Params Expr
	Fields Expr
}

// TestClick is `Test.click m`: message Msg of Page (nil for App's) with its parameters Params
// (Msg's payload record; nil for a message without parameters) posted as the held page's
// control for it would.
type TestClick struct {
	Typed
	Page   *Page
	Msg    *Msg
	Params Expr
}

// TestEq is the `==` a test records (see Test): Left == Right, of type Bool, whose sides the
// test runner shows when the test fails.
type TestEq struct {
	Typed
	Left, Right Expr
}
