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
	Name string // id | ref | created | updated | unique | cascade | restrict | setNull
	Arg  string // table name for ref
}
type PageDecl struct {
	Node
	Path         string
	Params       *RecordType
	Requires     string
	RequiresSpan diag.Span // the `requires field` clause; zero when absent
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
	Fields []*Field
}
type CapDecl struct {
	Node
	Name    string
	Entries []*Field // Type is a FunType or effect type
}
type SessionDecl struct {
	Node
	Fields []*Field
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
