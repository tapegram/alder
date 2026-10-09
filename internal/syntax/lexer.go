// internal/syntax/lexer.go
package syntax

import "bytes"

type LexError struct {
	Line, Col int
	Msg       string
}

type Lexer struct {
	file    string
	src     []byte
	pos     int
	line    int
	col     int
	htmlTag bool
	errs    []LexError
}

func NewLexer(file string, src []byte) *Lexer {
	src = bytes.TrimPrefix(src, []byte("\xEF\xBB\xBF"))
	src = bytes.ReplaceAll(src, []byte("\r\n"), []byte("\n"))
	return &Lexer{file: file, src: src, line: 1, col: 1}
}

func (l *Lexer) Errors() []LexError   { return l.errs }
func (l *Lexer) Pos() int             { return l.pos }
func (l *Lexer) SetHTMLTag(on bool)   { l.htmlTag = on }
func (l *Lexer) Reset(offset int) {
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

// Next returns the next code-mode token.
func (l *Lexer) Next() Token {
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
		if keywords[text] {
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
			return l.tok(LIdent, string(l.src[off:l.pos]), line, col, off)
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
		for {
			if l.pos >= len(l.src) || l.peek() == '\n' {
				l.errs = append(l.errs, LexError{line, col, "unterminated string"})
				return l.tok(String, string(l.src[off:l.pos]), line, col, off)
			}
			d := l.advance()
			if d == '\\' && l.pos < len(l.src) {
				l.advance()
				continue
			}
			if d == '"' {
				return l.tok(String, string(l.src[off:l.pos]), line, col, off)
			}
		}
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
	l.errs = append(l.errs, LexError{line, col, "unexpected character " + string(c)})
	return l.tok(Invalid, string(c), line, col, off)
}

// ScanText reads raw HTML text from the current position up to '<' or '{'.
func (l *Lexer) ScanText() Token {
	line, col, off := l.line, l.col, l.pos
	for l.pos < len(l.src) && l.peek() != '<' && l.peek() != '{' {
		l.advance()
	}
	return l.tok(Text, string(l.src[off:l.pos]), line, col, off)
}
