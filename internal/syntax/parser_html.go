// internal/syntax/parser_html.go
package syntax

import "strings"

// parseHTML is entered with p.tok == Lt at expression start. It consumes the whole element
// and leaves p.tok on the first token after it.
func (p *Parser) parseHTML() Expr {
	el := p.parseElement()
	p.lx.SetHTMLTag(false)
	p.advance()
	return el
}

// parseElement parses one element starting at p.tok == Lt. It returns with p.tok on the element's
// final `>` (or EOF) and without lexing past it, so the caller can continue in text mode.
func (p *Parser) parseElement() *Element {
	start := p.tok
	p.lx.SetHTMLTag(true)
	defer p.lx.SetHTMLTag(false)
	p.expect(Lt, "`<`")
	el := &Element{}
	tagTok := p.tok
	if p.at(UIdent) {
		p.advance()
		el.Tag = tagTok.Text
		for p.at(Dot) {
			p.advance()
			el.Module = qual(el.Module, el.Tag)
			el.Tag = p.expect(UIdent, "component name").Text
		}
	} else if p.at(LIdent) || p.at(Keyword) { // keywords such as `form` are ordinary tag names
		el.Tag = p.advance().Text
	} else {
		p.errorAt(p.tok, "expected tag name, got "+describe(p.tok))
	}
	defer p.in("<" + qual(el.Module, el.Tag) + ">")()
	// attributes
	for p.at(LIdent) || p.at(Keyword) {
		at := p.advance()
		a := &HtmlAttr{Name: at.Text}
		if p.at(Equal) {
			p.advance()
			if p.at(String) {
				a.Value = p.parseString(p.advance())
			} else {
				p.lx.SetHTMLTag(false)
				p.expect(LBrace, "`{` or a string")
				p.withBraces(true, func() {
					p.skipNL()
					a.Value = p.parseExpr()
					p.skipNL()
				})
				p.lx.SetHTMLTag(true)
				p.expect(RBrace, "`}`")
			}
		}
		a.Span = p.spanFrom(at)
		el.Attrs = append(el.Attrs, a)
	}
	if p.at(Slash) { // self-closing
		p.advance()
		if !p.at(Gt) {
			p.errorAt(p.tok, "expected `>`, got "+describe(p.tok))
		}
		el.Span = p.spanFrom(start)
		return el
	}
	if !p.at(Gt) {
		p.errorAt(p.tok, "expected `>`, got "+describe(p.tok))
		el.Span = p.spanFrom(start)
		return el
	}
	p.lx.SetHTMLTag(false)
	// children: raw text until `<` or `{`. p.tok is the last consumed delimiter, with no lookahead lexed.
	for {
		p.addText(el, p.lx.ScanText())
		p.advance() // now p.tok is `<`, `{` or EOF
		switch {
		case p.at(EOF):
			p.errorAt(start, "unclosed element <"+qual(el.Module, el.Tag)+">")
			el.Span = p.spanFrom(start)
			return el
		case p.at(LBrace):
			ltok := p.tok
			p.advance()
			p.skipNL()
			var child Expr
			p.withBraces(true, func() { child = p.parseHTMLBrace(ltok) })
			el.Children = append(el.Children, child)
			p.skipNL()
			if !p.at(RBrace) {
				p.errorAt(p.tok, "expected `}`, got "+describe(p.tok))
			}
		case p.at(Lt):
			save, saveTok := p.mark()
			p.lx.SetHTMLTag(true)
			p.advance()
			if p.at(Slash) { // closing tag
				p.advance()
				name := p.tok.Text
				p.advance()
				for p.at(Dot) {
					p.advance()
					name += "." + p.tok.Text
					p.advance()
				}
				p.lx.SetHTMLTag(false)
				if !p.at(Gt) {
					p.errorAt(p.tok, "expected `>`, got "+describe(p.tok))
				}
				if name != qual(el.Module, el.Tag) {
					p.errorAt(saveTok, "closing tag </"+name+"> does not match <"+qual(el.Module, el.Tag)+">")
				}
				el.Span = p.spanFrom(start)
				return el
			}
			p.restore(save, saveTok)
			el.Children = append(el.Children, p.parseElement())
		}
	}
}

// addText applies the JSX whitespace rule to one raw text run (the text between two nodes) and
// appends an HtmlText if anything is left:
//   - a run with no newline is kept as-is, so `<b>a</b> <i>b</i>` keeps its single space;
//   - a run containing a newline is split at its newlines; each line loses the whitespace that
//     touches a newline, empty lines and lines that begin with `--` (comments) are dropped,
//     and the rest are joined with one space. Leading/trailing same-line whitespace stays, so
//     the run "\n  hello " before `{name}` becomes "hello ".
//
// The rule is book chapter 5, "Whitespace".
func (p *Parser) addText(el *Element, tx Token) {
	text := tx.Text
	if strings.Contains(text, "\n") {
		lines := strings.Split(text, "\n")
		var kept []string
		for i, ln := range lines {
			if i > 0 {
				ln = strings.TrimLeft(ln, " \t")
			}
			if i < len(lines)-1 {
				ln = strings.TrimRight(ln, " \t")
			}
			if ln == "" || i > 0 && strings.HasPrefix(ln, "--") { // a line that begins with `--` is a comment
				continue
			}
			kept = append(kept, ln)
		}
		text = strings.Join(kept, " ")
	}
	if text == "" {
		return
	}
	t := &HtmlText{Text: text}
	t.Span = p.spanTok(tx)
	el.Children = append(el.Children, t)
}

// parseHTMLBrace is entered after `{`; p.tok is the first token inside. It returns with p.tok on the closing `}`.
func (p *Parser) parseHTMLBrace(ltok Token) Expr {
	switch {
	case p.atKeyword("for"):
		p.advance()
		f := &HtmlFor{Var: p.expect(LIdent, "loop variable").Text}
		p.expectKeyword("in")
		f.List = p.parseExpr()
		p.expect(Arrow, "`->`")
		p.skipNL()
		f.Body = p.parseHTMLChild()
		f.Span = p.spanFrom(ltok)
		return f
	case p.atKeyword("if"):
		p.advance()
		h := &HtmlIf{Cond: p.parseExpr()}
		p.skipNL()
		p.expectKeyword("then")
		p.skipNL()
		h.Then = p.parseHTMLChild()
		p.skipNL()
		if !p.atKeyword("else") {
			p.errorAt(p.tok, "`if` always has an `else`; in markup write `else Html.none`")
			h.Span = p.spanFrom(ltok)
			return h
		}
		p.advance()
		p.skipNL()
		h.Else = p.parseHTMLChild()
		h.Span = p.spanFrom(ltok)
		return h
	case p.atKeyword("match"):
		p.advance()
		m := &HtmlMatch{}
		p.withBraces(false, func() { m.Subject = p.parseExpr() })
		p.skipNL()
		p.expect(LBrace, "`{`")
		m.Arms = p.parseArmsWith(p.parseHTMLChild)
		m.Span = p.spanFrom(ltok)
		return m
	default:
		e := &HtmlExpr{Expr: p.parseExpr()}
		e.Span = p.spanFrom(ltok)
		return e
	}
}

// parseHTMLChild: an element, or an expression (wrapped in HtmlExpr).
func (p *Parser) parseHTMLChild() Expr {
	if p.at(Lt) {
		return p.parseHTML()
	}
	start := p.tok
	e := &HtmlExpr{Expr: p.parseExpr()}
	e.Span = p.spanFrom(start)
	return e
}
