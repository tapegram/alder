// Package semtok classifies the tokens of an Alder source for highlighting: the language
// server's semantic tokens (internal/lsp/tokens.go) and the parity test against the tree-sitter
// queries (tree-sitter-alder/highlights_test.go). It reads one parse (syntax.ParseTokens) and
// no analysis, so it can run on every keystroke's text. The legend is the transcription of
// tree-sitter-alder/queries/neovim/highlights.scm (spec 2026-11-01-vscode-extension-design.md).
package semtok

// Kind is a token type: its index in Types, the legend the server sends.
type Kind int

const (
	TNamespace Kind = iota
	TType
	TEnumMember
	TFunction
	TMethod
	TParameter
	TVariable
	TProperty
	TKeyword
	TOperator
	TString
	TNumber
	TComment
	TDecorator
	TTag
	TAttribute
	TBoolean
	THole
)

// none marks a token that gets no colour (a tag's `<`, the digits of a `-1` the `-` already covers).
const none Kind = -1

// Types are the legend's token types, in Kind order. The VS Code extension pins the same list.
var Types = []string{"namespace", "type", "enumMember", "function", "method", "parameter", "variable",
	"property", "keyword", "operator", "string", "number", "comment", "decorator", "tag", "attribute",
	"boolean", "hole"}

// The modifiers, as bits.
const (
	Declaration    = 1 << iota // a name where it is bound
	Documentation              // a comment block that ends on the line above a declaration
	DefaultLibrary             // reserved for phase 2 (standard-library references); never set yet
)

// Modifiers are the legend's token modifiers, in bit order.
var Modifiers = []string{"declaration", "documentation", "defaultLibrary"}

// Token is one coloured span: 1-based line, 1-based byte column and byte length, on one line.
type Token struct {
	Line, Col, Len int
	Kind           Kind
	Mods           int
}
