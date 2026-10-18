// internal/syntax/lexer.go
package syntax

import (
	"bytes"
	"strings"
)

type LexError struct {
	Line, Col, Offset int
	Msg               string
	Len               int // length of the offending text; 0 reports one column
}

type Lexer struct {
	file    string
	src     []byte
	pos     int
	line    int
	col     int
	htmlTag bool
	errs    []LexError
	// trace, when non-nil, records every token produced, in order; Restore and Reset drop the
	// tokens they rewind past, so it ends up as the stream the parser consumed.
	trace *[]Token
}

func NewLexer(file string, src []byte) *Lexer {
	src = bytes.TrimPrefix(src, []byte("\xEF\xBB\xBF"))
	src = bytes.ReplaceAll(src, []byte("\r\n"), []byte("\n"))
	return &Lexer{file: file, src: src, line: 1, col: 1}
}

func (l *Lexer) Errors() []LexError { return l.errs }
func (l *Lexer) Pos() int           { return l.pos }
func (l *Lexer) SetHTMLTag(on bool) { l.htmlTag = on }

// LexState is a cheap snapshot of the lexer position, for speculative lookahead.
type LexState struct{ pos, line, col, nerrs int }

func (l *Lexer) Mark() LexState { return LexState{l.pos, l.line, l.col, len(l.errs)} }

// Restore rewinds to a state from Mark and drops errors recorded since.
func (l *Lexer) Restore(s LexState) {
	l.pos, l.line, l.col = s.pos, s.line, s.col
	l.errs = l.errs[:s.nerrs]
	l.dropTrace(s.pos)
}

// dropTrace forgets traced tokens that start at or after offset.
func (l *Lexer) dropTrace(offset int) {
	if l.trace == nil {
		return
	}
	t := *l.trace
	for len(t) > 0 && t[len(t)-1].Offset >= offset {
		t = t[:len(t)-1]
	}
	*l.trace = t
}

func (l *Lexer) record(t Token) Token {
	if l.trace != nil {
		*l.trace = append(*l.trace, t)
	}
	return t
}

func (l *Lexer) Reset(offset int) {
	// Clamp offset to source length
	if offset > len(l.src) {
		offset = len(l.src)
	}
	// Drop errors at or after the offset
	keep := 0
	for _, e := range l.errs {
		if e.Offset < offset {
			l.errs[keep] = e
			keep++
		}
	}
	l.errs = l.errs[:keep]
	l.dropTrace(offset)
	// Rewind to the offset
	l.pos, l.line, l.col = 0, 1, 1
	for l.pos < offset {
		l.advance()
	}
}

func (l *Lexer) advance() byte {
	c := l.src[l.pos]
	l.pos++
	if c == '\n' {
		l.line++
		l.col = 1
	} else {
		l.col++
	}
	return c
}
func (l *Lexer) peek() byte {
	if l.pos >= len(l.src) {
		return 0
	}
	return l.src[l.pos]
}
func (l *Lexer) peek2() byte {
	if l.pos+1 >= len(l.src) {
		return 0
	}
	return l.src[l.pos+1]
}
func (l *Lexer) tok(k Kind, text string, line, col, off int) Token {
	return Token{Kind: k, Text: text, Line: line, Col: col, Offset: off}
}

func isLower(c byte) bool { return c >= 'a' && c <= 'z' }
func isUpper(c byte) bool { return c >= 'A' && c <= 'Z' }
func isDigit(c byte) bool { return c >= '0' && c <= '9' }
func (l *Lexer) isIdent(c byte) bool {
	return isLower(c) || isUpper(c) || isDigit(c) || c == '_' || (l.htmlTag && c == '-')
}

// checkIdent reports an identifier with "__" in it. The compiler joins the parts of a generated
// Go name with "__", so no Alder name may contain it (internal/gen/names.go). Inside an HTML tag
// (tag and attribute names, `data-x__y`) it does not apply: those never become Go names, and a
// component named there with "__" cannot have been declared.
func (l *Lexer) checkIdent(text string, line, col, off int) {
	if !l.htmlTag && strings.Contains(text, "__") {
		l.errs = append(l.errs, LexError{line, col, off, "identifiers may not contain two underscores in a row: `" + text + "`", len(text)})
	}
}

// Next returns the next code-mode token.
func (l *Lexer) Next() Token { return l.record(l.next()) }

func (l *Lexer) next() Token {
	for l.pos < len(l.src) {
		c := l.peek()
		if c == ' ' || c == '\t' {
			l.advance()
			continue
		}
		if c == '-' && l.peek2() == '-' {
			for l.pos < len(l.src) && l.peek() != '\n' {
				l.advance()
			}
			continue
		}
		break
	}
	line, col, off := l.line, l.col, l.pos
	if l.pos >= len(l.src) {
		return l.tok(EOF, "", line, col, off)
	}
	c := l.advance()
	switch {
	case c == '\n':
		return l.tok(NL, "\n", line, col, off)
	case isLower(c) || isUpper(c):
		for l.pos < len(l.src) && l.isIdent(l.peek()) {
			l.advance()
		}
		text := string(l.src[off:l.pos])
		l.checkIdent(text, line, col, off)
		// The name after a `.` (Db.where) is never a keyword. Stateless, so Mark/Restore stay valid.
		afterDot := off >= 2 && l.src[off-1] == '.' && l.src[off-2] != '.'
		if keywords[text] && !afterDot {
			return l.tok(Keyword, text, line, col, off)
		}
		if isUpper(c) {
			return l.tok(UIdent, text, line, col, off)
		}
		return l.tok(LIdent, text, line, col, off)
	case c == '_':
		if l.isIdent(l.peek()) {
			for l.pos < len(l.src) && l.isIdent(l.peek()) {
				l.advance()
			}
			text := string(l.src[off:l.pos])
			l.checkIdent(text, line, col, off)
			return l.tok(LIdent, text, line, col, off)
		}
		return l.tok(Underscore, "_", line, col, off)
	case isDigit(c):
		for isDigit(l.peek()) {
			l.advance()
		}
		if l.peek() == '.' && isDigit(l.peek2()) {
			l.advance()
			for isDigit(l.peek()) {
				l.advance()
			}
			return l.tok(Float, string(l.src[off:l.pos]), line, col, off)
		}
		return l.tok(Int, string(l.src[off:l.pos]), line, col, off)
	case c == '"':
		end, ok := scanString(l.src, l.pos)
		for l.pos < end {
			l.advance()
		}
		if !ok {
			l.errs = append(l.errs, LexError{Line: line, Col: col, Offset: off, Msg: "unterminated string"})
		}
		return l.tok(String, string(l.src[off:l.pos]), line, col, off)
	case c == '?':
		if l.peek() == '?' {
			l.advance()
			return l.tok(Question2, "??", line, col, off)
		}
		for l.pos < len(l.src) && l.isIdent(l.peek()) {
			l.advance()
		}
		return l.tok(Hole, string(l.src[off+1:l.pos]), line, col, off)
	}
	two := func(k Kind, s string) Token { l.advance(); return l.tok(k, s, line, col, off) }
	switch c {
	case '(':
		return l.tok(LParen, "(", line, col, off)
	case ')':
		return l.tok(RParen, ")", line, col, off)
	case '{':
		return l.tok(LBrace, "{", line, col, off)
	case '}':
		return l.tok(RBrace, "}", line, col, off)
	case '[':
		return l.tok(LBracket, "[", line, col, off)
	case ']':
		return l.tok(RBracket, "]", line, col, off)
	case ',':
		return l.tok(Comma, ",", line, col, off)
	case ':':
		return l.tok(Colon, ":", line, col, off)
	case '@':
		return l.tok(At, "@", line, col, off)
	case '\\':
		return l.tok(Backslash, "\\", line, col, off)
	case '.':
		if l.peek() == '.' {
			return two(DotDot, "..")
		}
		return l.tok(Dot, ".", line, col, off)
	case '|':
		if l.peek() == '>' {
			return two(PipeGt, "|>")
		}
		if l.peek() == '|' {
			return two(OrOr, "||")
		}
		return l.tok(Pipe, "|", line, col, off)
	case '-':
		if l.peek() == '>' {
			return two(Arrow, "->")
		}
		return l.tok(Minus, "-", line, col, off)
	case '<':
		if l.peek() == '-' {
			return two(LeftArrow, "<-")
		}
		if l.peek() == '=' {
			return two(Le, "<=")
		}
		return l.tok(Lt, "<", line, col, off)
	case '>':
		if l.peek() == '=' {
			return two(Ge, ">=")
		}
		return l.tok(Gt, ">", line, col, off)
	case '=':
		if l.peek() == '=' {
			return two(EqEq, "==")
		}
		return l.tok(Equal, "=", line, col, off)
	case '!':
		if l.peek() == '=' {
			return two(NotEq, "!=")
		}
	case '+':
		if l.peek() == '+' {
			return two(PlusPlus, "++")
		}
		return l.tok(Plus, "+", line, col, off)
	case '*':
		return l.tok(Star, "*", line, col, off)
	case '/':
		return l.tok(Slash, "/", line, col, off)
	case '%':
		return l.tok(Percent, "%", line, col, off)
	case '&':
		if l.peek() == '&' {
			return two(AndAnd, "&&")
		}
	}
	l.errs = append(l.errs, LexError{Line: line, Col: col, Offset: off, Msg: "unexpected character " + string(c)})
	return l.tok(Invalid, string(c), line, col, off)
}

// ScanText reads raw HTML text from the current position up to '{', or to a '<' that starts
// a tag (followed by a letter, '/' or '!'). Any other '<' is ordinary text.
func (l *Lexer) ScanText() Token {
	line, col, off := l.line, l.col, l.pos
	for l.pos < len(l.src) && l.peek() != '{' && !(l.peek() == '<' && isTagStart(l.peek2())) {
		l.advance()
	}
	return l.record(l.tok(Text, string(l.src[off:l.pos]), line, col, off))
}

// ScanRawText reads the body of a raw-text element (`<script>`, `<style>`): every byte up to its
// closing tag, written `</script` (the tag in lower case) then `>`, a space or `/`, or to the end
// of the source. Nothing in it is Alder: no `{…}`, no tags, no comments. A `</SCRIPT` (or a
// `</style` inside a script) is kept in the body, which the parser then refuses.
func (l *Lexer) ScanRawText(tag string) Token {
	line, col, off := l.line, l.col, l.pos
	end := len(l.src)
	for i := off; i < len(l.src); i++ {
		if l.src[i] == '<' && strings.HasPrefix(string(l.src[i:min(len(l.src), i+len(tag)+3)]), "</"+tag) {
			if j := i + 2 + len(tag); j == len(l.src) || strings.IndexByte("> \t\r\n/", l.src[j]) >= 0 {
				end = i
				break
			}
		}
	}
	for l.pos < end {
		l.advance()
	}
	return l.record(l.tok(Text, string(l.src[off:end]), line, col, off))
}

func isTagStart(c byte) bool { return isLower(c) || isUpper(c) || c == '/' || c == '!' }

// scanString scans a string literal's body starting at i, just after the opening quote. It returns
// the index just past the closing quote, or, when the line or source ends first, the index of
// that newline (or len(s)) and false. Inside an interpolation `{…}` a `"` opens a nested string
// scanned by this same rule, so `"a {f "-" x} b"` is one literal. A `{` with no matching `}`
// before the line ends is ordinary text here; the parser reports it.
func scanString(s []byte, i int) (int, bool) {
	for i < len(s) {
		switch s[i] {
		case '\n':
			return i, false
		case '\\':
			if i+1 < len(s) && s[i+1] != '\n' {
				i += 2
			} else {
				i++
			}
		case '"':
			return i + 1, true
		case '{':
			if j, ok := scanInterp(s, i+1); ok {
				i = j
			} else {
				i++
			}
		default:
			i++
		}
	}
	return i, false
}

// scanInterp scans an interpolation body starting at i, just after its `{`, tracking nested braces
// and skipping nested string literals. It returns the index just past the matching `}`, or false
// when the line or source ends first.
func scanInterp(s []byte, i int) (int, bool) {
	depth := 1
	for i < len(s) {
		switch s[i] {
		case '\n':
			return i, false
		case '"':
			j, ok := scanString(s, i+1)
			if !ok {
				return j, false
			}
			i = j
			continue
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return i + 1, true
			}
		}
		i++
	}
	return i, false
}
