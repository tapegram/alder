// internal/syntax/token.go
package syntax

import "alder/internal/diag"

type Kind int

const (
	EOF Kind = iota
	NL
	LIdent
	UIdent
	Int
	Float
	String
	Text
	LParen
	RParen
	LBrace
	RBrace
	LBracket
	RBracket
	Comma
	Colon
	Dot
	DotDot
	Pipe
	Arrow
	LeftArrow
	Equal
	EqEq
	NotEq
	Lt
	Le
	Gt
	Ge
	Plus
	Minus
	Star
	Slash
	Percent
	PlusPlus
	AndAnd
	OrOr
	Question2
	PipeGt
	Backslash
	Underscore
	At
	Hole
	Keyword
	Invalid
)

var kindNames = [...]string{"EOF", "NL", "LIdent", "UIdent", "Int", "Float", "String", "Text", "LParen", "RParen", "LBrace", "RBrace", "LBracket", "RBracket", "Comma", "Colon", "Dot", "DotDot", "Pipe", "Arrow", "LeftArrow", "Equal", "EqEq", "NotEq", "Lt", "Le", "Gt", "Ge", "Plus", "Minus", "Star", "Slash", "Percent", "PlusPlus", "AndAnd", "OrOr", "Question2", "PipeGt", "Backslash", "Underscore", "At", "Hole", "Keyword", "Invalid"}

func (k Kind) String() string { return kindNames[k] }

type Token struct {
	Kind    Kind
	Text    string
	Line    int
	Col     int
	Offset  int
	prevEnd diag.Pos // parser bookkeeping, set only by Parser.mark
}

var keywords = map[string]bool{"module": true, "import": true, "type": true, "brand": true, "where": true, "page": true, "msg": true, "form": true, "table": true, "capability": true, "fake": true, "session": true, "login": true, "test": true, "let": true, "match": true, "if": true, "then": true, "else": true, "for": true, "in": true, "requires": true}
