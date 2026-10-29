// internal/syntax/parser.go
package syntax

import (
	"strings"

	"alder/internal/diag"
)

type Parser struct {
	lx      *Lexer
	file    string
	tok     Token
	diags   []diag.Diagnostic
	sigMode bool
	lines   []string
	// noBrace stops application arguments at `{`, so `match s { ... }` is not read as `s {...}`.
	noBrace bool
	// prevEnd is the end of the last consumed non-newline token; spans end there, not at the next token.
	prevEnd diag.Pos
	// ctx names what is being parsed, innermost last; errorAt prefixes messages with it.
	ctx []string
	// session is set inside App's `session { … }`: a field's type stops at `via`, which names
	// the field's membership table (`org: Maybe Org.OrgId via Membership`).
	session bool
	// pageParams is set inside a page's `{ … }`: a field's type and default stop at `as`, which
	// names the parameter's query key (`electionId: Text = "" as "ElectionId"`, #55).
	pageParams bool
	// fieldDefault is set while a field's default is parsed: application stops at `as "…"`, so
	// a wire name written where none is allowed is reported as such.
	fieldDefault bool
}

func newParser(file string, src []byte, sigMode bool, trace *[]Token) *Parser {
	p := &Parser{lx: NewLexer(file, src), file: file, sigMode: sigMode}
	p.lx.trace = trace
	p.lines = strings.Split(string(p.lx.src), "\n")
	p.advance()
	return p
}

// ParseFile parses a source file. It never panics and recovers at declaration boundaries.
func ParseFile(file string, src []byte) (*Module, []diag.Diagnostic) {
	p := newParser(file, src, false, nil)
	m := p.parseModule()
	return m, p.finish()
}

// Tokens parses a source file and returns the tokens the parser consumed, in source order: the
// lexer is driven by the parser, so HTML text runs appear as Text tokens and speculative
// lookahead is not repeated. String literals are single tokens (interpolations included).
func Tokens(file string, src []byte) ([]Token, []diag.Diagnostic) {
	var trace []Token
	p := newParser(file, src, false, &trace)
	p.parseModule()
	return trace, p.finish()
}

// ParseExpr parses src as one expression, as `alder fill` reads its standard input. more is
// what follows the expression, named as an error message names a token ("`,`", "`test`"), or ""
// when the expression is the whole of src (blank lines and comments aside).
func ParseExpr(file string, src []byte) (e Expr, diags []diag.Diagnostic, more string, at diag.Pos) {
	p := newParser(file, src, false, nil)
	p.skipNL()
	e = p.parseExpr()
	p.skipNL()
	if !p.at(EOF) {
		more, at = describe(p.tok), p.posOf(p.tok)
	}
	return e, p.finish(), more, at
}

// ParseSignatures parses stdlib signature files: bodies are optional and `type X a` may be opaque.
func ParseSignatures(file string, src []byte) (*Module, []diag.Diagnostic) {
	p := newParser(file, src, true, nil)
	m := p.parseModule()
	return m, p.finish()
}

func (p *Parser) finish() []diag.Diagnostic {
	for _, e := range p.lx.Errors() {
		p.diags = append(p.diags, diag.Diagnostic{Code: diag.ParseError, Severity: "error",
			Span: diag.Span{File: p.file, Start: diag.Pos{Line: e.Line, Col: e.Col}, End: diag.Pos{Line: e.Line, Col: e.Col + max(e.Len, 1)}}, Message: e.Msg})
	}
	return p.diags
}

// ---- token helpers

func (p *Parser) advance() Token {
	prev := p.tok
	// The lexer sits right after p.tok (one token of lookahead), so its position is p.tok's end.
	if prev.Kind != NL && prev.Kind != EOF {
		p.prevEnd = diag.Pos{Line: p.lx.line, Col: p.lx.col}
	}
	p.tok = p.lx.Next()
	// Declaration-head keywords are keywords only in column 1; elsewhere they are ordinary
	// names (`update msg model session = ...`, `{ form: 1 }`).
	if p.tok.Kind == Keyword && declKeywords[p.tok.Text] && p.tok.Col > 1 {
		p.tok.Kind = LIdent
	}
	return prev
}
func (p *Parser) at(k Kind) bool          { return p.tok.Kind == k }
func (p *Parser) atKeyword(s string) bool { return p.tok.Kind == Keyword && p.tok.Text == s }
func (p *Parser) skipNL() {
	for p.tok.Kind == NL {
		p.advance()
	}
}
func (p *Parser) posOf(t Token) diag.Pos { return diag.Pos{Line: t.Line, Col: t.Col} }
func (p *Parser) spanFrom(start Token) diag.Span {
	end := p.prevEnd
	if end.Line < start.Line || (end.Line == start.Line && end.Col < start.Col) {
		end = p.posOf(start) // nothing consumed since start
	}
	return diag.Span{File: p.file, Start: p.posOf(start), End: end}
}
func (p *Parser) spanTok(t Token) diag.Span {
	width := len(t.Text)
	if t.Kind == Hole { // the token's text is the name, without its `?`
		width++
	}
	return diag.Span{File: p.file, Start: p.posOf(t), End: diag.Pos{Line: t.Line, Col: t.Col + max(1, width)}}
}

// in pushes a parse context ("a form field", "the definition of `x`") and returns its pop.
func (p *Parser) in(what string) func() {
	p.ctx = append(p.ctx, what)
	return func() { p.ctx = p.ctx[:len(p.ctx)-1] }
}

// errorAt reports a parse error at t; a second error at the token of the previous one is a
// cascade of it and is dropped.
func (p *Parser) errorAt(t Token, msg string) {
	if n := len(p.diags); n > 0 && p.diags[n-1].Span.Start == p.posOf(t) {
		return
	}
	if n := len(p.ctx); n > 0 {
		msg = "in " + p.ctx[n-1] + ": " + msg
	}
	p.diags = append(p.diags, diag.Diagnostic{Code: diag.ParseError, Severity: "error", Span: p.spanTok(t), Message: msg})
}

// errorCodeAt reports an error with a specific diagnostic code. Its message already names the
// construct (`case ... of`, a tuple), so it carries no parse context.
func (p *Parser) errorCodeAt(t Token, code, msg string) {
	p.diags = append(p.diags, diag.Diagnostic{Code: code, Severity: "error", Span: p.spanTok(t), Message: msg})
}

// mark and restore save and rewind the parser (token and lexer) for speculative lookahead.
// peekIsColon reports whether the token after the current one is `:`.
func (p *Parser) peekIsColon() bool {
	st, tok := p.mark()
	defer p.restore(st, tok)
	p.advance()
	return p.at(Colon)
}

func (p *Parser) mark() (LexState, Token) {
	t := p.tok
	t.prevEnd = p.prevEnd
	return p.lx.Mark(), t
}
func (p *Parser) restore(st LexState, tok Token) {
	p.lx.Restore(st)
	p.tok = tok
	p.prevEnd = tok.prevEnd
}

var declKeywords = map[string]bool{"module": true, "import": true, "type": true, "page": true, "msg": true,
	"form": true, "table": true, "capability": true, "fake": true, "session": true, "login": true, "test": true}

func (p *Parser) expect(k Kind, what string) Token {
	if p.tok.Kind != k {
		p.errorAt(p.tok, "expected "+what+", got "+describe(p.tok))
		return p.tok
	}
	return p.advance()
}
func (p *Parser) expectKeyword(s string) Token {
	if !p.atKeyword(s) {
		p.errorAt(p.tok, "expected `"+s+"`, got "+describe(p.tok))
		return p.tok
	}
	return p.advance()
}
func describe(t Token) string {
	switch t.Kind {
	case EOF:
		return "end of file"
	case NL:
		return "end of line"
	default:
		return "`" + t.Text + "`"
	}
}

// skipToDecl recovers after a parse error: advance to the next line that starts a declaration.
// skipBeforeDecl skips like skipToDecl but stops at the newline before the next declaration,
// as a declaration that parsed ends (a value's continuation lines are skipped with it).
func (p *Parser) skipBeforeDecl() {
	for !p.at(EOF) {
		if p.at(NL) {
			st, tok := p.mark()
			p.skipNL()
			next := p.at(EOF) || p.tok.Col == 1 && (p.tok.Kind == Keyword || p.tok.Kind == LIdent || p.tok.Kind == UIdent)
			p.restore(st, tok)
			if next {
				return
			}
		}
		p.advance()
	}
}

func (p *Parser) skipToDecl() {
	for !p.at(EOF) {
		if p.at(NL) {
			p.advance()
			if p.tok.Col == 1 && (p.tok.Kind == Keyword || p.tok.Kind == LIdent || p.tok.Kind == UIdent) {
				return
			}
			continue
		}
		p.advance()
	}
}

// ---- module

func (p *Parser) parseModule() *Module {
	m := &Module{Lines: p.lines, File: p.file}
	// The module starts at `module`: comments and blank lines before it lex as newlines, and
	// the first of those would otherwise be the start (at the end of the first comment line).
	p.skipNL()
	start := p.tok
	p.expectKeyword("module")
	m.Name = p.parseModName()
	for !p.at(EOF) {
		p.skipNL()
		if p.at(EOF) {
			break
		}
		before := p.tok
		if p.atKeyword("import") {
			t := p.advance()
			im := &Import{Module: p.parseModName()}
			if p.at(LIdent) && p.tok.Text == "as" {
				p.advance()
				switch {
				case p.at(UIdent):
					a := p.advance()
					im.Alias, im.AliasSpan = a.Text, p.spanTok(a)
				case p.at(LIdent):
					p.errorAt(p.tok, "alias must be capitalized, got `"+p.tok.Text+"`")
					p.advance()
				default:
					p.errorAt(p.tok, "expected alias after `as`, got "+describe(p.tok))
				}
			}
			im.Span = p.spanFrom(t)
			m.Imports = append(m.Imports, im)
		} else if d := p.parseDecl(); d != nil {
			m.Decls = append(m.Decls, d)
		}
		if !p.at(NL) && !p.at(EOF) {
			p.errorAt(p.tok, "expected end of line after declaration, got "+describe(p.tok))
			p.skipToDecl()
		}
		if p.tok == before { // no progress: avoid an infinite loop
			p.advance()
		}
	}
	m.Span = p.spanFrom(start)
	m.Comments = p.lx.Comments()
	return m
}

func (p *Parser) parseModName() string {
	t := p.expect(UIdent, "module name")
	p.checkModSegment(t)
	name := t.Text
	for p.at(Dot) {
		p.advance()
		seg := p.expect(UIdent, "module name segment")
		p.checkModSegment(seg)
		name += "." + seg.Text
	}
	return name
}

// checkModSegment rejects a module name segment ending in '_': generated Go names join a module's
// segments and a declaration's name with "__", and a name may start with '_' (`A_` then `x`
// would read as `A` then `_x`; see internal/gen/names.go).
func (p *Parser) checkModSegment(t Token) {
	if t.Kind == UIdent && strings.HasSuffix(t.Text, "_") {
		p.errorAt(t, "module name segments may not end with an underscore: `"+t.Text+"`")
	}
}

// ---- declarations

func (p *Parser) parseDecl() Decl {
	start := p.tok
	switch {
	case p.atKeyword("type"):
		return p.parseTypeDecl()
	case p.atKeyword("page"):
		p.advance()
		d := &PageDecl{Path: p.parseStringText()}
		pop := p.in("page parameters")
		p.pageParams = true
		d.Params = p.parseRecordType()
		p.pageParams = false
		pop()
		if p.atKeyword("requires") {
			p.parseRequires(d)
		}
		d.Span = p.spanFrom(start)
		return d
	case p.atKeyword("msg"):
		p.advance()
		d := &MsgDecl{Name: p.expect(UIdent, "message name").Text}
		if p.at(LBrace) {
			pop := p.in("message parameters")
			d.Params = p.parseRecordType()
			pop()
		}
		if p.at(LIdent) && p.tok.Text == "form" {
			p.advance()
			d.Form = p.expect(UIdent, "form name").Text
		}
		d.Span = p.spanFrom(start)
		return d
	case p.atKeyword("form"):
		p.advance()
		d := &FormDecl{Name: p.expect(UIdent, "form name").Text}
		p.expect(Equal, "`=`")
		p.expect(LBrace, "`{`")
		pop := p.in("a form field")
		d.Fields = p.parseFieldList(RBrace, false)
		pop()
		d.Span = p.spanFrom(start)
		return d
	case p.atKeyword("table"):
		return p.parseTableDecl()
	case p.atKeyword("fake") && !(p.sigMode && p.peekIsColon()): // stdlib Test's `fake : …` is a value
		return p.parseFakeDecl()
	case p.atKeyword("capability"):
		p.advance()
		d := &CapDecl{Name: p.expect(UIdent, "capability name").Text}
		p.expect(LBrace, "`{`")
		pop := p.in("a capability entry")
		d.Entries = p.parseFieldList(RBrace, false)
		pop()
		d.Span = p.spanFrom(start)
		return d
	case p.atKeyword("session"):
		p.advance()
		d := &SessionDecl{}
		p.expect(LBrace, "`{`")
		pop := p.in("the session")
		p.session = true
		d.Fields = p.parseFieldList(RBrace, false)
		p.session = false
		pop()
		d.Span = p.spanFrom(start)
		return d
	case p.at(LIdent) && p.tok.Text == "silo" && p.peekSeq(LIdent, LBrace): // contextual, like `deploy`
		return p.parseSiloDecl()
	case p.at(LIdent) && p.tok.Text == "choose" && p.peekSeq(LIdent, Equal, UIdent): // contextual
		p.advance()
		kt := p.advance()
		d := &ChooseDecl{Key: kt.Text, KeySpan: p.spanTok(kt)}
		p.advance() // =
		d.Page = p.parseModName()
		d.Span = p.spanFrom(start)
		return d
	case p.at(LIdent) && p.tok.Text == "deploy" && p.peekIs(LBrace): // contextual: `deploy` stays a name elsewhere
		p.advance()
		p.advance()
		pop := p.in("the deploy declaration")
		d := &DeployDecl{Fields: p.parseDeployFields()}
		pop()
		d.Span = p.spanFrom(start)
		return d
	case p.atKeyword("login") && !(p.sigMode && p.peekIsColon()): // stdlib Test's `login : …` is a value
		p.advance()
		p.expect(Equal, "`=`")
		d := &LoginDecl{Page: p.parseModName()}
		d.Span = p.spanFrom(start)
		return d
	case p.atKeyword("test"):
		p.advance()
		d := &TestDecl{Name: p.parseStringText()}
		p.expect(Equal, "`=`")
		p.skipNL()
		d.Body = p.parseExpr()
		d.Span = p.spanFrom(start)
		return d
	case p.at(LIdent) || p.at(UIdent):
		return p.parseValueDecl()
	case p.sigMode && p.at(Keyword) && p.peekIsColon():
		// stdlib signature files may name a value after a keyword (`where : …`)
		p.tok.Kind = LIdent
		return p.parseValueDecl()
	}
	p.errorAt(p.tok, "expected a declaration, got "+describe(p.tok))
	p.skipToDecl()
	return nil
}

// parseTableDecl parses `table T [@renamed "Old"] { fields }` at `table` (a keyword at the top
// level, an ordinary name inside a `fake`).
func (p *Parser) parseTableDecl() *TableDecl {
	start := p.advance()
	d := &TableDecl{Name: p.expect(UIdent, "table name").Text}
	for p.at(At) {
		d.Attrs = append(d.Attrs, p.parseAttr())
	}
	p.expect(LBrace, "`{`")
	pop := p.in("a table field")
	d.Fields = p.parseFieldList(RBrace, true)
	pop()
	d.Span = p.spanFrom(start)
	return d
}

// fakeDeclWords are the declaration keywords that, followed by a name, a string or `{`, start a
// declaration a `fake` cannot hold (`type Card = …` inside `fake Stripe { … }`).
var fakeDeclWords = map[string]bool{"module": true, "import": true, "type": true, "page": true, "msg": true,
	"form": true, "capability": true, "fake": true, "session": true, "test": true}

// parseFakeDecl parses `fake Cap { … }`: inside the braces, in any order, `table T { … }`
// declarations and one definition per entry of the capability, `name args = body`, written
// without a signature (the capability's entry type is its signature).
func (p *Parser) parseFakeDecl() Decl {
	if p.peekIs(Colon) || p.peekIs(Equal) || p.peekIs(LIdent) { // `fake : Int`, `fake = 1`, `fake x = …`: a value
		p.errorAt(p.tok, "`fake` starts a fake declaration at column 1: rename the value")
		p.skipBeforeDecl()
		return nil
	}
	start := p.advance()
	nameTok := p.expect(UIdent, "capability name")
	d := &FakeDecl{Name: nameTok.Text, NameSpan: p.spanTok(nameTok)}
	if !p.at(LBrace) {
		p.errorAt(p.tok, "expected `{` after `fake "+d.Name+"`: a fake holds its tables and one definition per entry of the capability, got "+describe(p.tok))
		d.Span = p.spanFrom(start)
		return d
	}
	p.advance()
	defer p.in("`fake " + d.Name + "`")()
	for {
		p.skipNL()
		if p.at(RBrace) {
			p.advance()
			break
		}
		if p.at(EOF) {
			p.errorAt(p.tok, "unterminated `fake "+d.Name+"`: expected `}`")
			break
		}
		before := p.tok
		switch {
		case (p.at(LIdent) && p.tok.Text == "table" || p.atKeyword("table")) && p.peekIs(UIdent):
			d.Tables = append(d.Tables, p.parseTableDecl())
		case p.at(LIdent) && fakeDeclWords[p.tok.Text] && (p.peekIs(UIdent) || p.peekIs(String) || p.peekIs(LBrace)):
			p.errorAt(p.tok, "a fake holds only `table` declarations and entry definitions; `"+p.tok.Text+"` declarations belong at the top level of the module")
			p.skipFakeItem()
		case p.at(LIdent) && p.peekIsColon():
			p.errorAt(p.tok, "a fake's entry takes its type from the capability: write `"+p.tok.Text+" args = …` without a signature")
			p.skipFakeItem()
		case p.at(LIdent):
			d.Entries = append(d.Entries, p.parseFakeEntry())
		default:
			p.errorAt(p.tok, "expected a `table` declaration or an entry definition (`name args = …`), got "+describe(p.tok))
			p.skipFakeItem()
		}
		if !p.at(NL) && !p.at(RBrace) && !p.at(EOF) {
			p.errorAt(p.tok, "expected end of line, got "+describe(p.tok))
			p.skipFakeItem()
		}
		if p.tok == before { // no progress: avoid an infinite loop
			p.advance()
		}
	}
	d.Span = p.spanFrom(start)
	return d
}

// parseFakeEntry parses `name params = body` inside a fake.
func (p *Parser) parseFakeEntry() *ValueDecl {
	start := p.advance()
	d := &ValueDecl{Name: start.Text}
	defer p.in("the definition of `" + start.Text + "`")()
	for p.at(LIdent) {
		t := p.advance()
		id := &Ident{Name: t.Text}
		id.Span = p.spanTok(t)
		d.Params = append(d.Params, id)
	}
	p.expect(Equal, "`=`")
	p.skipNL()
	d.Body = p.parseExpr()
	d.Span = p.spanFrom(start)
	return d
}

// skipFakeItem recovers inside a fake: skip to the end of the line, past any braces opened on
// it, stopping before the fake's own closing brace.
func (p *Parser) skipFakeItem() {
	depth := 0
	for !p.at(EOF) {
		switch {
		case p.at(LBrace):
			depth++
		case p.at(RBrace):
			if depth == 0 {
				return
			}
			depth--
		case p.at(NL) && depth == 0:
			return
		}
		p.advance()
	}
}

// peekIs reports whether the token after the current one is of kind k.
func (p *Parser) peekIs(k Kind) bool {
	st, tok := p.mark()
	defer p.restore(st, tok)
	p.advance()
	return p.at(k)
}

// parseValueDecl handles `name : Type NL name params = expr`, and in sigMode a bare signature.
func (p *Parser) parseValueDecl() Decl {
	start := p.tok
	nameTok := p.advance()
	d := &ValueDecl{Name: nameTok.Text}
	defer p.in("the definition of `" + nameTok.Text + "`")()
	if p.at(Colon) {
		p.advance()
		p.skipNL() // the type may start on the line after `:`
		d.Sig = p.parseType()
		if p.sigMode && (p.at(NL) || p.at(EOF)) {
			// peek: is the next non-blank line this name's definition?
			save, saveTok := p.mark()
			p.skipNL()
			if !(p.tok.Kind == nameTok.Kind && p.tok.Text == nameTok.Text) {
				p.restore(save, saveTok)
				d.Span = p.spanFrom(start)
				return d
			}
			p.advance()
		} else {
			p.skipNL()
			if !(p.tok.Kind == nameTok.Kind && p.tok.Text == nameTok.Text) {
				p.errorAt(p.tok, "expected definition of `"+nameTok.Text+"` after its signature")
				d.Span = p.spanFrom(start)
				return d
			}
			p.advance()
		}
	}
	for p.at(LIdent) {
		t := p.advance()
		id := &Ident{Name: t.Text}
		id.Span = p.spanTok(t)
		d.Params = append(d.Params, id)
	}
	p.expect(Equal, "`=`")
	p.skipNL()
	d.Body = p.parseExpr()
	d.Span = p.spanFrom(start)
	return d
}

func (p *Parser) parseTypeDecl() Decl {
	start := p.tok
	p.advance()
	if p.at(LIdent) && p.tok.Text == "alias" {
		// `type alias X = …` (Elm): report it, then parse the rest as an ordinary type declaration
		p.advance()
		name := "Point"
		if p.at(UIdent) {
			name = p.tok.Text
		}
		p.errorCodeAt(start, "E0602", "`type alias` is not Alder: a new name is always a new type; write `type "+name+" = { … }`")
	}
	d := &TypeDecl{Name: p.expect(UIdent, "type name").Text}
	for p.at(LIdent) {
		d.Params = append(d.Params, p.advance().Text)
	}
	if p.at(NL) {
		// `type T` with `= …` on a following, indented line (Elm's layout): skip to it
		save, saveTok := p.mark()
		p.skipNL()
		if !p.at(Equal) || p.tok.Col == 1 {
			p.restore(save, saveTok)
		}
	}
	if p.sigMode && (p.at(NL) || p.at(EOF)) { // opaque
		d.Span = p.spanFrom(start)
		return d
	}
	p.expect(Equal, "`=`")
	p.skipNL()
	switch {
	case p.atKeyword("brand"):
		bt := p.tok
		p.advance()
		b := &BrandType{Base: p.expect(UIdent, "base type").Text}
		if p.atKeyword("where") {
			p.advance()
			b.Check = p.parseExpr()
		}
		b.Span = p.spanFrom(bt)
		d.Brand = b
	case p.at(LBrace):
		pop := p.in("a record type")
		d.Record = p.parseRecordType()
		pop()
	default:
		if p.at(Pipe) {
			p.advance()
		}
		for {
			ct := p.expect(UIdent, "constructor")
			if p.at(Dot) && len(d.Union) == 0 {
				// `type Model = Todo.Todo`: a qualified type name, i.e. an alias
				qual := ct.Text
				for p.at(Dot) {
					p.advance()
					qual += "." + p.expect(UIdent, "type name").Text
				}
				for p.at(UIdent) || p.at(LIdent) || p.at(LBrace) || p.at(LParen) {
					p.parseAType()
				}
				p.errorCodeAt(start, "E0602", "`type "+d.Name+" = "+qual+"` is an alias; a new name is always a new type; write `type "+d.Name+" = { }` or a union with its own constructors")
				d.Span = p.spanFrom(start)
				return d
			}
			c := &Ctor{Name: ct.Text}
			if p.at(UIdent) || p.at(LIdent) || p.at(LBrace) || p.at(LParen) {
				c.Arg = p.parseAType()
			}
			c.Span = p.spanFrom(ct)
			d.Union = append(d.Union, c)
			save, saveTok := p.mark()
			p.skipNL()
			if p.at(Pipe) {
				p.advance()
				continue
			}
			p.restore(save, saveTok)
			break
		}
	}
	d.Span = p.spanFrom(start)
	return d
}

// parseRequires parses a page's `requires` clause at `requires`: session fields (`requires
// user, org`), then an optional `role R | S`, then an optional `silo k` (the admin form,
// `requires user, role Admin, silo org`).
func (p *Parser) parseRequires(d *PageDecl) {
	rt := p.advance()
	for !p.atRole() && !p.atSilo() {
		ft := p.tok
		name := p.expect(LIdent, "session field").Text
		if len(d.Requires) == 0 {
			d.RequiresSpan = p.spanFrom(rt)
			d.FieldSpans = append(d.FieldSpans, d.RequiresSpan)
		} else {
			d.FieldSpans = append(d.FieldSpans, p.spanFrom(ft))
		}
		d.Requires = append(d.Requires, name)
		fields := strings.Join(d.Requires, ", ")
		if p.at(Comma) {
			p.advance()
			if p.atRole() || p.atSilo() || p.at(LIdent) {
				continue
			}
			p.errorAt(p.tok, "expected `role R` after `requires "+fields+",` (or another session field, or `silo k`), got "+describe(p.tok))
		} else if p.atRole() {
			p.errorAt(p.tok, "expected `,` between `requires "+fields+"` and `role`: write `requires "+fields+", role R`")
		}
		break
	}
	if p.atRole() {
		d.Roles, d.RolesSpan = p.parseRoles()
		if p.at(Comma) {
			switch {
			case p.peekSeq(LIdent, LIdent) && p.peekText("silo"):
				p.advance()
			case len(d.Requires) == 0: // `requires role Admin, user`
				p.errorAt(p.tok, "`user` goes first: write `requires user, role R` with unqualified constructors")
				p.skipLine()
				return
			default:
				p.errorAt(p.tok, "only `silo k` may follow the `role` clause: session fields go before it (`requires user, org, role R`)")
				p.skipLine()
				return
			}
		}
	}
	if p.atSilo() {
		st := p.advance()
		d.Silo = p.advance().Text
		d.SiloSpan = p.spanFrom(st)
	}
}

// skipLine skips to the end of the line.
func (p *Parser) skipLine() {
	for !p.at(NL) && !p.at(EOF) {
		p.advance()
	}
}

// atSilo reports whether the parser is at the `silo k` clause of a page's `requires`: the name
// `silo` followed by a name (`requires silo` alone names a session field `silo`).
func (p *Parser) atSilo() bool {
	return p.at(LIdent) && p.tok.Text == "silo" && p.peekIs(LIdent)
}

// peekSeq reports whether the tokens after the current one are of kinds ks, in order.
func (p *Parser) peekSeq(ks ...Kind) bool {
	st, tok := p.mark()
	defer p.restore(st, tok)
	for _, k := range ks {
		p.advance()
		if !p.at(k) {
			return false
		}
	}
	return true
}

// peekText reports whether the token after the current one has text s.
func (p *Parser) peekText(s string) bool {
	st, tok := p.mark()
	defer p.restore(st, tok)
	p.advance()
	return p.tok.Text == s
}

// parseSiloDecl parses `silo k { T, U }` at `silo`.
func (p *Parser) parseSiloDecl() *SiloDecl {
	start := p.advance()
	kt := p.advance()
	d := &SiloDecl{Key: kt.Text, KeySpan: p.spanTok(kt)}
	p.advance() // {
	pop := p.in("`silo " + d.Key + "`")
	defer pop()
	for {
		p.skipNL()
		if p.at(RBrace) {
			p.advance()
			break
		}
		if !p.at(UIdent) {
			p.errorAt(p.tok, "expected a table name, got "+describe(p.tok))
			for !p.at(RBrace) && !p.at(EOF) {
				p.advance()
			}
			if p.at(RBrace) {
				p.advance()
			}
			break
		}
		t := p.advance()
		d.Tables = append(d.Tables, t.Text)
		d.TableSpans = append(d.TableSpans, p.spanTok(t))
		p.skipNL()
		if p.at(Comma) {
			p.advance()
			continue
		}
		p.skipNL()
		if !p.at(RBrace) {
			p.errorAt(p.tok, "expected `,` or `}`, got "+describe(p.tok))
			for !p.at(RBrace) && !p.at(EOF) {
				p.advance()
			}
			if p.at(RBrace) {
				p.advance()
			}
			break
		}
	}
	d.Span = p.spanFrom(start)
	return d
}

// atRole reports whether the parser is at the `role R` clause of a page's `requires`: the name
// `role` followed by a constructor (`requires role` alone names a session field `role`).
func (p *Parser) atRole() bool {
	return p.at(LIdent) && p.tok.Text == "role" && p.peekIs(UIdent)
}

// parseRoles parses `role R` or `role R | S | …` at `role`.
func (p *Parser) parseRoles() ([]string, diag.Span) {
	start := p.advance()
	var roles []string
	for {
		roles = append(roles, p.expect(UIdent, "role constructor").Text)
		if p.at(Dot) { // `role User.Admin`
			p.errorAt(p.tok, "a role is a constructor of the session's `role` type, unqualified: write `requires user, role R` with unqualified constructors")
			for p.at(Dot) || p.at(UIdent) {
				p.advance()
			}
		}
		if !p.at(Pipe) {
			break
		}
		p.advance()
	}
	return roles, p.spanFrom(start)
}

// parseAttr parses `@name`, `@ref Table` or `@renamed "old"` (a plain string: no
// interpolation) at an `@`.
func (p *Parser) parseAttr() *Attr {
	at := p.advance()
	a := &Attr{Name: p.expect(LIdent, "attribute name").Text}
	switch a.Name {
	case "ref":
		a.Arg = p.expect(UIdent, "table name").Text
	case "partition":
		a.Arg = p.expect(LIdent, "the session field `@partition` names (`@partition org`)").Text
	case "renamed":
		if !p.at(String) {
			p.errorAt(p.tok, "expected the old name as a string (`@renamed \"old\"`), got "+describe(p.tok))
			break
		}
		t := p.advance()
		s := p.parseString(t).(*StrLit)
		for _, part := range s.Parts {
			if part.Expr != nil {
				p.errorAt(t, "`@renamed` takes a plain string, without `{…}`")
				break
			}
			a.Arg += part.Text
		}
	}
	a.Span = p.spanFrom(at)
	return a
}

// parseFieldList parses `name: Type [= default] [@attr…] ,` … up to `closing`; the opening brace is consumed.
func (p *Parser) parseFieldList(closing Kind, attrs bool) []*Field {
	var fields []*Field
	for {
		p.skipNL()
		if p.at(closing) {
			p.advance()
			return fields
		}
		if p.at(EOF) {
			p.errorAt(p.tok, "unterminated field list")
			return fields
		}
		ft := p.expect(LIdent, "field name")
		f := &Field{Name: ft.Text}
		p.expect(Colon, "`:`")
		f.Type = p.parseType()
		if p.session && p.at(LIdent) && p.tok.Text == "via" {
			vt := p.advance()
			f.Via = p.expect(UIdent, "the membership table after `via`").Text
			f.ViaSpan = p.spanFrom(vt)
		}
		if p.at(Equal) {
			p.advance()
			saved := p.fieldDefault
			p.fieldDefault = true
			f.Default = p.parseExpr()
			p.fieldDefault = saved
		}
		if p.pageParams && p.at(NL) && p.wireAfterNL() {
			p.skipNL() // `as "Key"` may start the next line: the list is brace-delimited (#63)
		}
		if p.atWire() {
			p.parseWire(f)
		}
		for !attrs && p.at(At) {
			// `title: Text @unique` in a form: report the rule, skip `@name [Table]`, and go on
			atTok := p.advance()
			name := "name"
			if p.at(LIdent) {
				name = p.advance().Text
			}
			if p.at(UIdent) || p.at(String) {
				p.advance()
			}
			p.errorAt(atTok, "attributes such as `@"+name+"` are only allowed on table fields")
		}
		for attrs && p.at(At) {
			f.Attrs = append(f.Attrs, p.parseAttr())
		}
		f.Span = p.spanFrom(ft)
		fields = append(fields, f)
		p.skipNL()
		if p.at(Comma) {
			p.advance()
			continue
		}
		p.skipNL()
		if !p.at(closing) {
			p.errorAt(p.tok, "expected `,` or closing brace")
			return fields
		}
	}
}

// atWire reports whether the parser is at a field's wire name `as "Key"` (#55): in a page's
// parameters any `as` (no type or default there names a value `as`), elsewhere `as` followed by
// a string, which is reported as misplaced.
func (p *Parser) atWire() bool {
	if !p.at(LIdent) || p.tok.Text != "as" {
		return false
	}
	return p.pageParams || p.peekIs(String)
}

// wireAfterNL reports whether the tokens after the current newlines start a wire name `as`.
func (p *Parser) wireAfterNL() bool {
	st, tok := p.mark()
	defer p.restore(st, tok)
	p.skipNL()
	return p.atWire()
}

// parseWire parses a field's `as "Key"` at `as` into f.Wire and f.WireSpan. Outside a page's
// parameters the clause is an error, as is a key that is not a plain string.
func (p *Parser) parseWire(f *Field) {
	at := p.advance()
	if !p.at(String) {
		p.errorAt(p.tok, "expected the query key after `as`, as a string (`as \"Key\"`), got "+describe(p.tok))
		return
	}
	t := p.advance()
	s := p.parseString(t).(*StrLit)
	key := ""
	for _, part := range s.Parts {
		if part.Expr != nil {
			p.errorAt(t, "a wire name is a plain string, without `{…}`")
			return
		}
		key += part.Text
	}
	span := p.spanFrom(at)
	if !p.pageParams {
		p.diags = append(p.diags, diag.Diagnostic{Code: diag.ParseError, Severity: "error", Span: span,
			Message: "a wire name (`as \"" + key + "\"`) is only allowed on a page's query parameters (`page \"/p\" { id: Text as \"Id\" }`), not on `" + f.Name + "` here"})
		return
	}
	f.Wire, f.WireSpan = key, span
}

func (p *Parser) parseStringText() string {
	t := p.expect(String, "string literal")
	s := t.Text
	if len(s) >= 2 && s[0] == '"' && s[len(s)-1] == '"' {
		return s[1 : len(s)-1]
	}
	return s
}

// ---- types

// parseType: atype {atype} ["->" type]
func (p *Parser) parseType() Type {
	defer p.in("a type")()
	start := p.tok
	first := p.parseAType()
	var args []Type
	for p.at(UIdent) || p.at(LIdent) && !(p.session && p.tok.Text == "via") && !p.atWire() || p.at(LBrace) || p.at(LParen) {
		args = append(args, p.parseAType())
	}
	var head Type = first
	if len(args) > 0 {
		tn, ok := first.(*TypeName)
		if !ok {
			p.errorAt(start, "only a named type can take type arguments")
		} else {
			tn.Args = args
			tn.Span = p.spanFrom(start)
		}
	}
	if p.at(Arrow) || (p.at(NL) && p.atOpAfterNL(Arrow)) { // a line starting with `->` continues the type
		p.advance()
		res := p.parseType()
		ft := &FunType{Params: []Type{head}, Result: res}
		if inner, ok := res.(*FunType); ok { // a -> b -> c flattens to params [a, b]
			ft.Params = append(ft.Params, inner.Params...)
			ft.Result = inner.Result
		}
		ft.Span = p.spanFrom(start)
		return ft
	}
	return head
}

func (p *Parser) parseAType() Type {
	start := p.tok
	switch {
	case p.at(UIdent):
		t := p.advance()
		tn := &TypeName{Name: t.Text}
		for p.at(Dot) {
			p.advance()
			tn.Module = qual(tn.Module, tn.Name)
			tn.Name = p.expect(UIdent, "type name").Text
		}
		tn.Span = p.spanFrom(start)
		return tn
	case p.at(LIdent):
		t := p.advance()
		tv := &TypeVar{Name: t.Text}
		tv.Span = p.spanTok(t)
		return tv
	case p.at(LBrace):
		return p.parseRecordType()
	case p.atKeyword("brand"):
		// `id: brand Text` inline: report it, then read the base type so parsing continues
		p.errorAt(p.tok, "`brand` is only allowed as the whole body of a `type` declaration (`type TodoId = brand Text`)")
		p.advance()
		return p.parseAType()
	case p.at(LParen):
		p.advance()
		if p.at(RParen) {
			p.advance()
			tn := &TypeName{Name: "Unit"}
			tn.Span = p.spanFrom(start)
			return tn
		}
		inner := p.parseType()
		p.expect(RParen, "`)`")
		return inner
	}
	p.errorAt(p.tok, "expected a type, got "+describe(p.tok))
	tn := &TypeName{Name: "?"}
	tn.Span = p.spanTok(p.tok)
	return tn
}

func (p *Parser) parseRecordType() *RecordType {
	start := p.tok
	p.expect(LBrace, "`{`")
	rt := &RecordType{Fields: p.parseFieldList(RBrace, false)}
	rt.Span = p.spanFrom(start)
	return rt
}

// parseDeployFields parses `name: value` fields up to and including the closing `}` (after the
// opening one): separated by commas or newlines, a trailing comma allowed. A field name may be
// a keyword (`requires`).
func (p *Parser) parseDeployFields() []*DeployField {
	var fields []*DeployField
	for {
		p.skipNL()
		if p.at(RBrace) {
			p.advance()
			return fields
		}
		if p.at(EOF) {
			p.errorAt(p.tok, "unterminated deploy declaration")
			return fields
		}
		if !p.at(LIdent) && !p.at(Keyword) {
			p.errorAt(p.tok, "expected a field name, got "+describe(p.tok))
			p.skipToDecl()
			return fields
		}
		nt := p.advance()
		f := &DeployField{Name: nt.Text, NameSpan: p.spanTok(nt)}
		p.expect(Colon, "`:`")
		p.skipNL()
		f.Value = p.parseDeployValue()
		f.Span = p.spanFrom(nt)
		fields = append(fields, f)
		if f.Value == nil {
			p.skipToDecl()
			return fields
		}
		p.skipNL()
		if p.at(Comma) {
			p.advance()
			continue
		}
		if !p.at(RBrace) {
			p.errorAt(p.tok, "expected `,` or closing brace")
			p.skipToDecl()
			return fields
		}
	}
}

// parseDeployValue parses a string, a number, `[value, …]`, or `Ctor ["text"] [{ fields }]`.
func (p *Parser) parseDeployValue() *DeployValue {
	start := p.tok
	switch {
	case p.at(String):
		return &DeployValue{Kind: "string", Str: p.parseStringText(), Span: p.spanFrom(start)}
	case p.at(Int), p.at(Float):
		kind := "int"
		if p.at(Float) {
			kind = "float"
		}
		t := p.advance()
		return &DeployValue{Kind: kind, Str: t.Text, Span: p.spanFrom(start)}
	case p.at(Minus):
		p.advance()
		if !p.at(Int) && !p.at(Float) {
			p.errorAt(p.tok, "expected a number after `-`, got "+describe(p.tok))
			return nil
		}
		kind := "int"
		if p.at(Float) {
			kind = "float"
		}
		t := p.advance()
		return &DeployValue{Kind: kind, Str: "-" + t.Text, Span: p.spanFrom(start)}
	case p.at(LBracket):
		p.advance()
		v := &DeployValue{Kind: "list"}
		for {
			p.skipNL()
			if p.at(RBracket) {
				p.advance()
				break
			}
			if p.at(EOF) {
				p.errorAt(p.tok, "unterminated list")
				return nil
			}
			e := p.parseDeployValue()
			if e == nil {
				return nil
			}
			v.List = append(v.List, e)
			p.skipNL()
			if p.at(Comma) {
				p.advance()
			} else if !p.at(RBracket) {
				p.errorAt(p.tok, "expected `,` or `]`")
				return nil
			}
		}
		v.Span = p.spanFrom(start)
		return v
	case p.at(UIdent):
		v := &DeployValue{Kind: "ctor", Ctor: p.advance().Text}
		if p.at(String) {
			a := p.tok
			v.Arg = &DeployValue{Kind: "string", Str: p.parseStringText(), Span: p.spanFrom(a)}
		}
		if p.at(LBrace) {
			p.advance()
			v.Rec = p.parseDeployFields()
			if v.Rec == nil {
				v.Rec = []*DeployField{}
			}
		}
		v.Span = p.spanFrom(start)
		return v
	}
	p.errorAt(p.tok, "expected a string, a number, a list or a constructor, got "+describe(p.tok))
	return nil
}
