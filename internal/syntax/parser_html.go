// internal/syntax/parser_html.go
package syntax

import (
	"encoding/json"
	"strings"

	"alder/internal/diag"
)

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
	// attributes; line breaks are allowed between them and before `>` / `/>`
	for p.skipNL(); p.at(LIdent) || p.at(Keyword); p.skipNL() {
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
	if el.Module == "" && rawTextTags[el.Tag] {
		p.rawText(el)
	}
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
				if name != qual(el.Module, el.Tag) && !(rawTextTags[el.Tag] && el.Module == "" && strings.EqualFold(name, el.Tag)) { // `</Script>` closes a script, as in HTML
					msg := "closing tag </" + name + "> does not match <" + qual(el.Module, el.Tag) + ">"
					if rawTextTags[name] {
						// most likely a `</script>` inside a script's string ended the script early:
						// skip this tag, so the elements around it still close where they should
						p.errorAt(saveTok, msg+": "+rawCloseRule(name))
						continue
					}
					p.errorAt(saveTok, msg)
				}
				el.Span = p.spanFrom(start)
				return el
			}
			p.restore(save, saveTok)
			el.Children = append(el.Children, p.parseElement())
		}
	}
}

// rawTextTags are HTML's raw-text elements: their body is text, not markup or Alder.
var rawTextTags = map[string]bool{"script": true, "style": true}

// rawText reads the body of a `<script>` or `<style>` (p.tok on the start tag's `>`): the source's
// bytes up to the closing tag, as one raw HtmlText, kept byte for byte. Nothing in it is Alder, so
// no Alder value can reach script or style text; data reaches a script through `data-*`
// attributes. The body may not contain `</script` or `</style` in any case (the browser would
// end the element there), and a body that is wholly one `{ … }` and not JSON is refused as the
// interpolation it looks like (looksInterpolated). It returns with the lexer on the closing tag's
// `<`, where parseElement's loop reads the closing tag.
func (p *Parser) rawText(el *Element) {
	tx := p.lx.ScanRawText(el.Tag)
	if tx.Text == "" {
		return
	}
	t := &HtmlText{Text: tx.Text, Raw: true}
	t.Span = diag.Span{File: p.file, Start: p.posOf(tx), End: rawPos(tx, len(tx.Text))}
	el.Children = append(el.Children, t)
	low := strings.ToLower(tx.Text)
	for _, bad := range []string{"</script", "</style"} {
		if i := strings.Index(low, bad); i >= 0 {
			p.rawError(tx, i, len(bad), "the body contains `"+tx.Text[i:i+len(bad)]+"`: "+rawCloseRule(el.Tag))
			return
		}
	}
	if looksInterpolated(el, tx.Text) {
		i := strings.IndexByte(tx.Text, '{')
		p.rawError(tx, i, len(strings.TrimSpace(tx.Text)), "the body is raw text, not Alder: `{ … }` here is not an expression and would be sent as written; "+map[string]string{
			"script": "write the JavaScript itself, and pass Alder values through a `data-*` attribute (`data-clip={model.clip}`), which is escaped",
			"style":  "write the CSS itself, and set Alder values through an element's `class` or `style` attribute",
		}[el.Tag])
	}
}

// rawCloseRule explains why a raw-text body may not contain a closing tag.
func rawCloseRule(tag string) string {
	return "a <" + tag + "> body is raw text up to its first `</" + tag + "`, so it may not contain `</script` or `</style` in any case; split the text in a string (`\"<\\/" + tag + ">\"`), or move the " + map[string]string{"script": "script", "style": "stylesheet"}[tag] + " to a file in `static/`"
}

// rawError reports a parse error over n bytes at byte i of the raw text token tx.
func (p *Parser) rawError(tx Token, i, n int, msg string) {
	if len(p.ctx) > 0 {
		msg = "in " + p.ctx[len(p.ctx)-1] + ": " + msg
	}
	p.diags = append(p.diags, diag.Diagnostic{Code: diag.ParseError, Severity: "error", Message: msg,
		Span: diag.Span{File: p.file, Start: rawPos(tx, i), End: rawPos(tx, i+n)}})
}

// rawPos is the position of byte i of the raw text token tx.
func rawPos(tx Token, i int) diag.Pos {
	pos := diag.Pos{Line: tx.Line, Col: tx.Col}
	for _, c := range []byte(tx.Text[:i]) {
		if c == '\n' {
			pos.Line, pos.Col = pos.Line+1, 1
		} else {
			pos.Col++
		}
	}
	return pos
}

// looksInterpolated: the whole body, spaces aside, is one `{ … }` (it starts with `{` and ends
// with `}`) and is not JSON: Alder interpolation (`{Json.encode model.data}`, `{a ++ b}`), which a
// raw-text body does not have. As JavaScript it would be a block, and as CSS it is not a rule. A
// JSON body (`{"a": 1}`, a data island) and any body of a `type="application/json"` script are
// not refused.
func looksInterpolated(el *Element, body string) bool {
	b := strings.TrimSpace(body)
	if len(b) < 2 || b[0] != '{' || b[len(b)-1] != '}' || json.Valid([]byte(b)) {
		return false
	}
	for _, a := range el.Attrs {
		if lit, ok := a.Value.(*StrLit); ok && a.Name == "type" && len(lit.Parts) == 1 && lit.Parts[0].Expr == nil &&
			strings.EqualFold(strings.TrimSpace(lit.Parts[0].Text), "application/json") {
			return false
		}
	}
	return true
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
