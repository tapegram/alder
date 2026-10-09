// internal/syntax/parser_expr.go
package syntax

import (
	"strconv"
	"strings"
)

func (p *Parser) parseExpr() Expr { return p.parsePipe() }

// withBraces parses with record-argument braces allowed (or not) and restores the previous setting.
func (p *Parser) withBraces(allowed bool, f func()) {
	saved := p.noBrace
	p.noBrace = !allowed
	f()
	p.noBrace = saved
}

func (p *Parser) binary(start Token, op string, l, r Expr) Expr {
	b := &BinOp{Op: op, L: l, R: r}
	b.Span = p.spanFrom(start)
	return b
}

// atOpAfterNL reports whether the next line begins with one of kinds (an operator continuing
// the expression). The parser position is left unchanged when it returns false.
func (p *Parser) atOpAfterNL(kinds ...Kind) bool {
	save, saveTok := p.mark()
	p.skipNL()
	for _, k := range kinds {
		if p.at(k) {
			return true
		}
	}
	p.restore(save, saveTok)
	return false
}

func (p *Parser) parsePipe() Expr {
	start := p.tok
	l := p.parseDefault()
	for p.at(PipeGt) || (p.at(NL) && p.atOpAfterNL(PipeGt)) {
		p.advance()
		p.skipNL()
		l = p.binary(start, "|>", l, p.parseDefault())
	}
	return l
}

func (p *Parser) parseDefault() Expr {
	start := p.tok
	l := p.parseOr()
	if p.at(Question2) {
		p.advance()
		p.skipNL()
		return p.binary(start, "??", l, p.parseDefault())
	}
	return l
}

func (p *Parser) parseOr() Expr {
	start := p.tok
	l := p.parseAnd()
	for p.at(OrOr) || (p.at(NL) && p.atOpAfterNL(OrOr)) {
		p.advance()
		p.skipNL()
		l = p.binary(start, "||", l, p.parseAnd())
	}
	return l
}

func (p *Parser) parseAnd() Expr {
	start := p.tok
	l := p.parseCmp()
	for p.at(AndAnd) || (p.at(NL) && p.atOpAfterNL(AndAnd)) {
		p.advance()
		p.skipNL()
		l = p.binary(start, "&&", l, p.parseCmp())
	}
	return l
}

var cmpOps = map[Kind]string{EqEq: "==", NotEq: "!=", Lt: "<", Le: "<=", Gt: ">", Ge: ">="}

func (p *Parser) parseCmp() Expr {
	start := p.tok
	l := p.parseAdd()
	if op, ok := cmpOps[p.tok.Kind]; ok {
		p.advance()
		p.skipNL()
		r := p.parseAdd()
		if _, again := cmpOps[p.tok.Kind]; again {
			p.errorAt(p.tok, "comparisons do not chain; use `&&`")
		}
		return p.binary(start, op, l, r)
	}
	return l
}

func (p *Parser) parseAdd() Expr {
	start := p.tok
	l := p.parseMul()
	for p.at(Plus) || p.at(Minus) || p.at(PlusPlus) || (p.at(NL) && p.atOpAfterNL(Plus, Minus, PlusPlus)) {
		op := p.advance().Text
		p.skipNL()
		l = p.binary(start, op, l, p.parseMul())
	}
	return l
}

func (p *Parser) parseMul() Expr {
	start := p.tok
	l := p.parseApp()
	for p.at(Star) || p.at(Slash) || p.at(Percent) {
		op := p.advance().Text
		p.skipNL()
		l = p.binary(start, op, l, p.parseApp())
	}
	return l
}

// parseApp: primary {primary}; arguments continue while the next token can start a primary on the same line.
func (p *Parser) parseApp() Expr {
	start := p.tok
	fn := p.parsePostfix()
	var args []Expr
	for p.canStartArg() {
		args = append(args, p.parsePostfix())
	}
	if len(args) == 0 {
		return fn
	}
	a := &App{Fn: fn, Args: args}
	a.Span = p.spanFrom(start)
	return p.checkCaseOf(a, start)
}

func (p *Parser) canStartArg() bool {
	switch p.tok.Kind {
	case LIdent, UIdent, Int, Float, String, LParen, LBracket, Hole, Backslash:
		return true
	case LBrace:
		return !p.noBrace
	}
	return false
}

func (p *Parser) parsePostfix() Expr {
	start := p.tok
	e := p.parsePrimary()
	for p.at(Dot) {
		p.advance()
		f := p.expect(LIdent, "field name")
		a := &Access{Expr: e, Field: f.Text}
		a.Span = p.spanFrom(start)
		e = a
	}
	return e
}

func (p *Parser) unit(start Token) Expr {
	e := &UnitLit{}
	e.Span = p.spanFrom(start)
	return e
}

func (p *Parser) parsePrimary() Expr {
	start := p.tok
	switch p.tok.Kind {
	case Int:
		t := p.advance()
		v, _ := strconv.ParseInt(t.Text, 10, 64)
		e := &IntLit{Value: v}
		e.Span = p.spanTok(t)
		return e
	case Float:
		t := p.advance()
		v, _ := strconv.ParseFloat(t.Text, 64)
		e := &FloatLit{Value: v}
		e.Span = p.spanTok(t)
		return e
	case Minus: // -1 only
		p.advance()
		if p.at(Int) || p.at(Float) {
			lit := p.parsePrimary()
			switch l := lit.(type) {
			case *IntLit:
				l.Value = -l.Value
			case *FloatLit:
				l.Value = -l.Value
			}
			return lit
		}
		p.errorAt(start, "negation is not an operator; use `0 - x`")
		return p.parsePrimary()
	case String:
		return p.parseString(p.advance())
	case Hole:
		t := p.advance()
		e := &HoleExpr{Name: t.Text}
		e.Span = p.spanTok(t)
		return e
	case LIdent:
		t := p.advance()
		e := &Var{Name: t.Text}
		e.Span = p.spanTok(t)
		return e
	case UIdent:
		return p.parseQualified()
	case LParen:
		p.advance()
		if p.at(RParen) {
			p.advance()
			return p.unit(start)
		}
		var inner Expr
		p.withBraces(true, func() {
			p.skipNL()
			inner = p.parseExpr()
			p.skipNL()
		})
		if p.at(Comma) {
			p.errorCodeAt(start, "E0602", "tuples are not Alder; use a record `{ a: 1, b: 2 }`")
			// skip to the matching `)`, tracking nested parens
			for depth := 1; depth > 0 && !p.at(EOF) && !p.at(NL); p.advance() {
				switch p.tok.Kind {
				case LParen:
					depth++
				case RParen:
					depth--
				}
			}
			return p.unit(start)
		}
		p.expect(RParen, "`)`")
		return inner
	case LBracket:
		p.advance()
		e := &ListLit{}
		p.withBraces(true, func() {
			p.skipNL()
			for !p.at(RBracket) && !p.at(EOF) {
				e.Elems = append(e.Elems, p.parseExpr())
				p.skipNL()
				if p.at(Comma) {
					p.advance()
					p.skipNL()
				} else {
					break
				}
			}
		})
		p.expect(RBracket, "`]`")
		e.Span = p.spanFrom(start)
		return e
	case LBrace:
		var e Expr
		p.withBraces(true, func() { e = p.parseBraces() })
		return e
	case Backslash:
		p.advance()
		l := &Lambda{}
		for p.at(LIdent) {
			t := p.advance()
			id := &Ident{Name: t.Text}
			id.Span = p.spanTok(t)
			l.Params = append(l.Params, id)
		}
		p.expect(Arrow, "`->`")
		p.skipNL()
		l.Body = p.parseExpr()
		l.Span = p.spanFrom(start)
		return l
	case Lt:
		return p.parseHTML()
	case Keyword:
		switch p.tok.Text {
		case "if":
			p.advance()
			e := &If{Cond: p.parseExpr()}
			p.skipNL()
			p.expectKeyword("then")
			p.skipNL()
			e.Then = p.parseExpr()
			p.skipNL()
			p.expectKeyword("else")
			p.skipNL()
			e.Else = p.parseExpr()
			e.Span = p.spanFrom(start)
			return e
		case "match":
			p.advance()
			e := &Match{}
			p.withBraces(false, func() { e.Subject = p.parseExpr() })
			p.skipNL()
			p.expect(LBrace, "`{`")
			p.withBraces(true, func() { e.Arms = p.parseArms() })
			e.Span = p.spanFrom(start)
			return e
		case "let":
			p.errorCodeAt(start, "E0602", "`let ... in` is not Alder; use a block: `{ let y = 1 … y }`")
			p.skipToDeclStayingOnLine()
			return p.unit(start)
		}
	}
	p.errorAt(p.tok, "expected an expression, got "+describe(p.tok))
	p.advance()
	return p.unit(start)
}

// checkCaseOf catches `case x of`. `case` and `of` are not keywords, so `case x of` lexes as
// an application of the variable `case` whose last argument is the variable `of`.
func (p *Parser) checkCaseOf(e Expr, start Token) Expr {
	if a, ok := e.(*App); ok {
		if v, ok := a.Fn.(*Var); ok && v.Module == "" && v.Name == "case" {
			if last, ok := a.Args[len(a.Args)-1].(*Var); ok && last.Name == "of" {
				p.errorCodeAt(start, "E0602", "`case ... of` is not Alder; use `match x { ... }`")
				p.skipToDeclStayingOnLine()
				return p.unit(start)
			}
		}
	}
	return e
}

// skipToDeclStayingOnLine consumes the rest of the current logical expression (to the next blank line or EOF).
func (p *Parser) skipToDeclStayingOnLine() {
	for !p.at(EOF) {
		if p.at(NL) {
			save, saveTok := p.mark()
			p.advance()
			if p.at(NL) || p.at(EOF) {
				p.restore(save, saveTok)
				return
			}
			continue
		}
		p.advance()
	}
}

func (p *Parser) parseQualified() Expr {
	start := p.tok
	t := p.advance()
	mod, name := "", t.Text
	for p.at(Dot) {
		save, saveTok := p.mark()
		p.advance()
		if p.at(UIdent) {
			mod = qual(mod, name)
			name = p.advance().Text
			continue
		}
		if p.at(LIdent) { // Module.value
			v := &Var{Module: qual(mod, name), Name: p.advance().Text}
			v.Span = p.spanFrom(start)
			return v
		}
		p.restore(save, saveTok)
		break
	}
	if mod == "" && (name == "True" || name == "False") {
		b := &BoolLit{Value: name == "True"}
		b.Span = p.spanFrom(start)
		return b
	}
	c := &CtorRef{Module: mod, Name: name}
	c.Span = p.spanFrom(start)
	return c
}

// parseBraces: record literal, record update, or block.
func (p *Parser) parseBraces() Expr {
	start := p.tok
	p.expect(LBrace, "`{`")
	p.skipNL()
	if p.at(RBrace) {
		p.advance()
		e := &RecordLit{}
		e.Span = p.spanFrom(start)
		return e
	}
	if p.atKeyword("let") {
		return p.parseBlockBody(start)
	}
	if p.at(LIdent) {
		save, saveTok := p.mark()
		p.advance()
		isField := p.at(Colon)
		p.restore(save, saveTok)
		if isField {
			e := &RecordLit{Fields: p.parseFieldInits()}
			e.Span = p.spanFrom(start)
			return e
		}
	}
	first := p.parseExpr()
	p.skipNL()
	if p.at(Pipe) {
		p.advance()
		p.skipNL()
		e := &RecordUpdate{Base: first, Fields: p.parseFieldInits()}
		e.Span = p.spanFrom(start)
		return e
	}
	// block with a single result expression
	p.expect(RBrace, "`}`")
	b := &Block{Result: first}
	b.Span = p.spanFrom(start)
	return b
}

// parseFieldInits parses `name: expr, …}` and consumes the closing brace.
func (p *Parser) parseFieldInits() []*FieldInit {
	var fs []*FieldInit
	for {
		p.skipNL()
		if p.at(RBrace) {
			p.advance()
			return fs
		}
		ft := p.expect(LIdent, "field name")
		if !p.at(Colon) {
			p.errorAt(p.tok, "record fields are written `name: value`; there is no punning")
		} else {
			p.advance()
		}
		p.skipNL()
		fi := &FieldInit{Name: ft.Text, Value: p.parseExpr()}
		fi.Span = p.spanFrom(ft)
		fs = append(fs, fi)
		p.skipNL()
		if p.at(Comma) {
			p.advance()
			continue
		}
		p.skipNL()
		p.expect(RBrace, "`}` or `,`")
		return fs
	}
}

func (p *Parser) parseBlockBody(start Token) Expr {
	b := &Block{}
	for p.atKeyword("let") {
		lt := p.advance()
		s := &LetStmt{Name: p.expectNameOrWild()}
		if p.at(Colon) {
			p.advance()
			s.Type = p.parseType()
		}
		if p.at(LeftArrow) {
			s.Bind = true
			p.advance()
		} else {
			p.expect(Equal, "`=` or `<-`")
		}
		p.skipNL()
		s.Expr = p.parseExpr()
		s.Span = p.spanFrom(lt)
		b.Stmts = append(b.Stmts, s)
		if !p.at(NL) && !p.at(RBrace) {
			p.errorAt(p.tok, "expected end of line after `let` statement")
		}
		p.skipNL()
	}
	if p.at(RBrace) {
		p.errorAt(p.tok, "a block must end with an expression")
	} else {
		b.Result = p.parseExpr()
	}
	p.skipNL()
	p.expect(RBrace, "`}`")
	b.Span = p.spanFrom(start)
	return b
}

func (p *Parser) expectNameOrWild() string {
	if p.at(Underscore) {
		p.advance()
		return "_"
	}
	return p.expect(LIdent, "name").Text
}

func (p *Parser) parseArms() []*Arm {
	return p.parseArmsWith(p.parseExpr)
}

// parseArmsWith parses `pattern -> body, …}` (the opening brace is consumed) and consumes the closing brace.
func (p *Parser) parseArmsWith(body func() Expr) []*Arm {
	var arms []*Arm
	for {
		p.skipNL()
		if p.at(RBrace) {
			p.advance()
			return arms
		}
		if p.at(EOF) {
			p.errorAt(p.tok, "unterminated match")
			return arms
		}
		start := p.tok
		a := &Arm{Pat: p.parsePattern()}
		p.expect(Arrow, "`->`")
		p.skipNL()
		a.Body = body()
		a.Span = p.spanFrom(start)
		arms = append(arms, a)
		p.skipNL()
		if p.at(Comma) {
			p.advance()
			continue
		}
		p.skipNL()
		p.expect(RBrace, "`,` or `}`")
		return arms
	}
}

// ---- patterns

func (p *Parser) parsePattern() Pattern {
	start := p.tok
	switch p.tok.Kind {
	case Underscore:
		p.advance()
		w := &PWild{}
		w.Span = p.spanTok(start)
		return w
	case LIdent:
		t := p.advance()
		v := &PVar{Name: t.Text}
		v.Span = p.spanTok(t)
		return v
	case Int, String, Minus:
		lit := p.parsePrimary()
		l := &PLit{Lit: lit}
		l.Span = p.spanFrom(start)
		return l
	case UIdent:
		t := p.advance()
		if t.Text == "True" || t.Text == "False" {
			l := &PLit{Lit: &BoolLit{Value: t.Text == "True"}}
			l.Span = p.spanTok(t)
			return l
		}
		c := &PCtor{Name: t.Text}
		if p.at(LIdent) || p.at(Underscore) || p.at(LBrace) || p.at(LParen) || p.at(UIdent) || p.at(Int) || p.at(String) {
			c.Arg = p.parsePattern1()
		}
		c.Span = p.spanFrom(start)
		return c
	case LBrace:
		return p.parseRecordPattern()
	case LBracket:
		p.advance()
		l := &PList{}
		for !p.at(RBracket) && !p.at(EOF) {
			if p.at(DotDot) {
				p.advance()
				l.Rest = p.expect(LIdent, "rest name").Text
				break
			}
			l.Elems = append(l.Elems, p.parsePattern1())
			if p.at(Comma) {
				p.advance()
			} else {
				break
			}
		}
		p.expect(RBracket, "`]`")
		l.Span = p.spanFrom(start)
		return l
	case LParen:
		p.advance()
		inner := p.parsePattern()
		p.expect(RParen, "`)`")
		return inner
	}
	p.errorAt(p.tok, "expected a pattern, got "+describe(p.tok))
	p.advance()
	w := &PWild{}
	w.Span = p.spanTok(start)
	return w
}

// parsePattern1: a constructor argument — no bare constructor application (needs parens).
func (p *Parser) parsePattern1() Pattern {
	if p.at(UIdent) {
		t := p.advance()
		if t.Text == "True" || t.Text == "False" {
			l := &PLit{Lit: &BoolLit{Value: t.Text == "True"}}
			l.Span = p.spanTok(t)
			return l
		}
		c := &PCtor{Name: t.Text}
		c.Span = p.spanTok(t)
		return c
	}
	return p.parsePattern()
}

func (p *Parser) parseRecordPattern() Pattern {
	start := p.tok
	p.expect(LBrace, "`{`")
	r := &PRecord{}
	for {
		p.skipNL()
		if p.at(RBrace) {
			p.advance()
			break
		}
		ft := p.expect(LIdent, "field name")
		f := &PField{Name: ft.Text}
		if p.at(Colon) {
			p.advance()
			f.Pat = p.parsePattern1()
		} else {
			p.errorAt(p.tok, "record patterns are written `{ field: pattern }`")
			f.Pat = &PWild{}
		}
		f.Span = p.spanFrom(ft)
		r.Fields = append(r.Fields, f)
		p.skipNL()
		if p.at(Comma) {
			p.advance()
			continue
		}
		p.skipNL()
		p.expect(RBrace, "`}`")
		break
	}
	r.Span = p.spanFrom(start)
	return r
}

// ---- strings

// parseString splits a raw literal (with quotes) into literal and interpolated parts.
func (p *Parser) parseString(t Token) Expr {
	raw := t.Text
	if len(raw) >= 2 && raw[len(raw)-1] == '"' {
		raw = raw[1 : len(raw)-1]
	} else if len(raw) >= 1 {
		raw = raw[1:]
	}
	s := &StrLit{}
	s.Span = p.spanTok(t)
	var buf strings.Builder
	flush := func() {
		if buf.Len() > 0 {
			s.Parts = append(s.Parts, StrPart{Text: buf.String()})
			buf.Reset()
		}
	}
	for i := 0; i < len(raw); i++ {
		c := raw[i]
		switch {
		case c == '\\' && i+1 < len(raw):
			i++
			switch raw[i] {
			case 'n':
				buf.WriteByte('\n')
			case 't':
				buf.WriteByte('\t')
			case '"', '\\', '{':
				buf.WriteByte(raw[i])
			default:
				p.errorAt(t, "unknown escape \\"+string(raw[i]))
			}
		case c == '{':
			j, ok := scanInterp([]byte(raw), i+1) // j is just past the matching `}`
			if !ok {
				p.errorAt(t, "unterminated `{` in string")
				return s
			}
			flush()
			inner := raw[i+1 : j-1]
			sub := &Parser{lx: NewLexer(p.file, []byte(inner)), file: p.file, sigMode: p.sigMode, ctx: append([]string(nil), p.ctx...)}
			// position the sub-lexer at the interpolation inside the string literal
			sub.lx.line, sub.lx.col = t.Line, t.Col+1+i+1
			sub.advance()
			e := sub.parseExpr()
			if !sub.at(EOF) {
				sub.errorAt(sub.tok, "unexpected token in interpolation")
			}
			p.diags = append(p.diags, sub.finish()...)
			s.Parts = append(s.Parts, StrPart{Expr: e})
			i = j - 1
		default:
			buf.WriteByte(c)
		}
	}
	flush()
	if len(s.Parts) == 0 {
		s.Parts = []StrPart{{Text: ""}}
	}
	return s
}
