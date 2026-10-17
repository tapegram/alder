// internal/syntax/ast.go
package syntax

import "alder/internal/diag"

type Node struct{ Span diag.Span }

func (n *Node) GetSpan() diag.Span { return n.Span }

// Ref is filled in by the names pass.
type RefKind int

const (
	RefNone       RefKind = iota
	RefValue              // top-level value/function
	RefLocal              // parameter, let, lambda param, pattern var
	RefType               // type/table/form/brand declaration
	RefCtor               // union constructor (incl. Msg ctors)
	RefPage               // page module used as a value
	RefTable              // table used as a value (Db.get Todo)
	RefColumn             // Todo.done
	RefComponent          // capitalized function used as element
	RefCapability         // capability module (Password.verify)
	RefStd                // standard library value
)

type Ref struct {
	Kind   RefKind
	Module string // declaring module ("" for locals)
	Name   string // declaration name
}

// ---- Module and declarations

type Module struct {
	Node
	Name    string
	File    string
	Imports []*Import
	Decls   []Decl
	Lines   []string // source lines, for rendering
}
type Import struct {
	Node
	Module string
}

type Decl interface{ GetSpan() diag.Span }

type ValueDecl struct {
	Node
	Name   string
	Sig    Type // nil only in stdlib signature files (body nil) or when missing (E0007)
	Params []*Ident
	Body   Expr // nil in stdlib signature files
}
type Ident struct {
	Node
	Name string
}
type TypeDecl struct {
	Node
	Name   string
	Params []string
	Union  []*Ctor // one of Union/Record/Brand set; all nil = opaque (stdlib only)
	Record *RecordType
	Brand  *BrandType
}
type Ctor struct {
	Node
	Name string
	Arg  Type // nil for no payload
}
type BrandType struct {
	Node
	Base  string // Int | Float | Bool | Text
	Check Expr   // nil if no `where`
}
type Field struct {
	Node
	Name    string
	Type    Type
	Default Expr // nil if none
	Attrs   []*Attr
}
type Attr struct {
	Node
	Name string // id | ref | created | updated | unique | cascade | restrict | setNull | renamed | owner
	Arg  string // table name for ref; the old name for renamed (`@renamed "heading"` → heading)
}
type PageDecl struct {
	Node
	Path         string
	Params       *RecordType
	Requires     string
	RequiresSpan diag.Span // the `requires field` clause; zero when absent
	// Roles are the constructors of `requires user, role R | S` (["R", "S"]), nil for none;
	// RolesSpan is the `role R | S` clause. `requires role R` (no field) leaves Requires "".
	Roles     []string
	RolesSpan diag.Span
}
type MsgDecl struct {
	Node
	Name   string
	Params *RecordType // nil = no params
	Form   string      // "" = no form
}
type FormDecl struct {
	Node
	Name   string
	Fields []*Field
}
type TableDecl struct {
	Node
	Name   string
	Attrs  []*Attr // after the name: `table Post @renamed "Article" {`
	Fields []*Field
}
type CapDecl struct {
	Node
	Name    string
	Entries []*Field // Type is a FunType or effect type
}

// FakeDecl is `fake Cap { … }`: an in-memory implementation of capability Cap for tests, with
// tables of its own (test-only) and one definition per entry (Sig nil: the entry's type is its
// signature). Ref is the capability the name resolves to (RefCapability, Name "Module.Cap").
type FakeDecl struct {
	Node
	Name     string
	NameSpan diag.Span
	Tables   []*TableDecl
	Entries  []*ValueDecl
	Ref      Ref
}
type SessionDecl struct {
	Node
	Fields []*Field
}

// DeployDecl is `deploy { name: value, … }` in App (chapter 16): data the `alder deploy`
// command reads. A field is an environment (its value a target: `Ssh "host" { … }`, `Fly
// "region" { … }`, `Script "path"`) or one of floor, checks, requires, backup, keep, proxy.
type DeployDecl struct {
	Node
	Fields []*DeployField
}

// DeployField is `name: value` of a deploy declaration or of a target's record.
type DeployField struct {
	Span     diag.Span
	Name     string
	NameSpan diag.Span
	Value    *DeployValue
}

// DeployValue is a value of a deploy declaration: a string, a number, a list, or a constructor
// with an optional string argument and an optional record (`Ssh "h" { domain: "d" }`).
type DeployValue struct {
	Span diag.Span
	Kind string // "string", "int", "float", "list", "ctor"
	Str  string // a string's text; a number's source text
	List []*DeployValue
	Ctor string
	Arg  *DeployValue   // a ctor's string argument, or nil
	Rec  []*DeployField // a ctor's record, or nil
}

type LoginDecl struct {
	Node
	Page string
}
type TestDecl struct {
	Node
	Name string
	Body Expr
}

// ---- Types

type Type interface{ GetSpan() diag.Span }

type TypeName struct {
	Node
	Module string // "" if unqualified
	Name   string
	Args   []Type
	Ref    Ref
}
type TypeVar struct {
	Node
	Name string
}
type RecordType struct {
	Node
	Fields []*Field
}
type FunType struct {
	Node
	Params []Type
	Result Type
}

// ---- Expressions

type Expr interface{ GetSpan() diag.Span }

type IntLit struct {
	Node
	Value int64
}
type FloatLit struct {
	Node
	Value float64
}
type BoolLit struct {
	Node
	Value bool
}
type UnitLit struct{ Node }
type StrLit struct {
	Node
	Parts []StrPart // literal text and interpolated expressions, in order
}
type StrPart struct {
	Text string
	Expr Expr // nil for text parts
}
type ListLit struct {
	Node
	Elems []Expr
}
type RecordLit struct {
	Node
	Fields []*FieldInit
}
type FieldInit struct {
	Node
	Name  string
	Value Expr
}
type RecordUpdate struct {
	Node
	Base   Expr
	Fields []*FieldInit
}
type Var struct {
	Node
	Module string
	Name   string
	Ref    Ref
}
type CtorRef struct { // capitalized name in expression position: Just, Normal, Todos.Show, App.Logout
	Node
	Module string
	Name   string
	Ref    Ref
	// Table is set (Kind RefTable) when a bare name the resolver made a constructor also names a
	// table this module can see (`Member` of `type Role = Reader | Member` and `table Member`):
	// in table position (the first argument of a `Db.` operation) the resolver picks the table;
	// elsewhere the checker picks by the type expected there, and reports E0002 when it is
	// neither (or unknown).
	Table Ref
}
type Access struct {
	Node
	Expr  Expr
	Field string
}
type App struct {
	Node
	Fn   Expr
	Args []Expr
}
type Lambda struct {
	Node
	Params []*Ident
	Body   Expr
}
type If struct {
	Node
	Cond, Then, Else Expr
}
type Match struct {
	Node
	Subject Expr
	Arms    []*Arm
}
type Arm struct {
	Node
	Pat  Pattern
	Body Expr
}
type Block struct {
	Node
	Stmts  []*LetStmt
	Result Expr
}
type LetStmt struct {
	Node
	Name string
	Type Type // optional annotation
	Bind bool // true for `let x <- e`
	Expr Expr
}
type BinOp struct {
	Node
	Op   string // "+", "++", "==", "|>", "??", ...
	L, R Expr
}
type HoleExpr struct {
	Node
	Name string
}

// ---- HTML

type Element struct {
	Node
	Module   string // for <Layout.Page>
	Tag      string // lowercase html tag or capitalized component name
	Attrs    []*HtmlAttr
	Children []Expr // Element, HtmlText, HtmlExpr, HtmlFor, HtmlIf, HtmlMatch
	Ref      Ref    // component resolution
}
type HtmlAttr struct {
	Node
	Name  string
	Value Expr // StrLit for literal, any Expr for {…}, nil for bare (`<Errors form />`)
}
type HtmlText struct {
	Node
	Text string
}
type HtmlExpr struct {
	Node
	Expr Expr
}
type HtmlFor struct {
	Node
	Var  string
	List Expr
	Body Expr
}
type HtmlIf struct {
	Node
	Cond, Then, Else Expr
}
type HtmlMatch struct {
	Node
	Subject Expr
	Arms    []*Arm
}

// ---- Patterns

type Pattern interface{ GetSpan() diag.Span }

type PWild struct{ Node }
type PVar struct {
	Node
	Name string
}
type PLit struct {
	Node
	Lit Expr // IntLit, StrLit (no interpolation), BoolLit
}
type PCtor struct {
	Node
	Name string
	Arg  Pattern // nil if none
	Ref  Ref
}
type PRecord struct {
	Node
	Fields []*PField
}
type PField struct {
	Node
	Name string
	Pat  Pattern
}
type PList struct {
	Node
	Elems []Pattern
	Rest  string // "" if none
}
