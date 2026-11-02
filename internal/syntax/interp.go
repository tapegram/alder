package syntax

// Interpolation is one `{…}` of a string literal token: the expression's bytes in the token's
// text (t.Text[From:To], between the braces) and the tokens a parse of it consumes, positioned
// in the file as parseString positions the expression's nodes. Their Offset fields count from
// the expression's start, not the file's.
type Interpolation struct {
	From, To int
	Tokens   []Token
}

// Interpolations are the interpolations of t, a String token, in order, found as parseString
// finds them: escapes skipped, nested braces and strings matched, and an unterminated `{` ends
// the search. A string inside an interpolation is one of its tokens; ask again for its own. A
// token that is not a String has none.
func Interpolations(file string, t Token) []Interpolation {
	if t.Kind != String {
		return nil
	}
	raw := t.Text
	if len(raw) >= 2 && raw[len(raw)-1] == '"' {
		raw = raw[1 : len(raw)-1]
	} else if len(raw) >= 1 {
		raw = raw[1:]
	}
	var out []Interpolation
	for i := 0; i < len(raw); i++ {
		switch {
		case raw[i] == '\\' && i+1 < len(raw):
			i++
		case raw[i] == '{':
			j, ok := scanInterp([]byte(raw), i+1) // j is just past the matching `}`
			if !ok {
				return out
			}
			var trace []Token
			interpParser(file, t, raw[i+1:j-1], i, false, nil, &trace).parseExpr()
			// raw[k] is t.Text[k+1]: the expression raw[i+1:j-1] is t.Text[i+2:j]
			out = append(out, Interpolation{From: i + 2, To: j, Tokens: trace})
			i = j - 1
		}
	}
	return out
}

// interpParser is a parser of inner, the expression of the interpolation whose `{` is raw[i] of
// the string token t (raw being t's text without its opening quote), positioned in the file.
// trace, when non-nil, records the tokens it consumes.
func interpParser(file string, t Token, inner string, i int, sigMode bool, ctx []string, trace *[]Token) *Parser {
	sub := &Parser{lx: NewLexer(file, []byte(inner)), file: file, sigMode: sigMode, ctx: ctx}
	sub.lx.trace = trace
	sub.lx.line, sub.lx.col = t.Line, t.Col+1+i+1
	sub.advance()
	return sub
}
