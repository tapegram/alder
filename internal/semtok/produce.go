// internal/semtok/produce.go
package semtok

import (
	"sort"
	"strings"

	"alder/internal/diag"
	"alder/internal/syntax"
)

// Module classifies src, a module's text. The tokens are sorted and do not overlap.
func Module(file string, src []byte) []Token {
	m, trace, _ := syntax.ParseTokens(file, src)
	return produce(m, trace)
}

// class is what the tree says a token is: its kind (none: no colour), its modifiers, and, when
// set, the end of the span it colours when that is longer than the token (`-1`, `()`).
type class struct {
	kind Kind
	mods int
	end  diag.Pos
}

// walker holds one parse: the tokens to colour, in source order, and the classes the tree gives
// them, keyed by each token's start.
type walker struct {
	m      *syntax.Module
	toks   []syntax.Token
	cls    map[diag.Pos]class
	interp map[diag.Pos][]syntax.Interpolation // the String tokens of literals with an interpolation
}

func pos(t syntax.Token) diag.Pos { return diag.Pos{Line: t.Line, Col: t.Col} }
func before(a, b diag.Pos) bool   { return a.Line < b.Line || a.Line == b.Line && a.Col < b.Col }

// sortOut orders ts by position.
func sortOut(ts []Token) {
	sort.SliceStable(ts, func(i, j int) bool {
		return ts[i].Line < ts[j].Line || ts[i].Line == ts[j].Line && ts[i].Col < ts[j].Col
	})
}

// sortTokens orders ts by position.
func sortTokens(ts []syntax.Token) {
	sort.SliceStable(ts, func(i, j int) bool { return before(pos(ts[i]), pos(ts[j])) })
}
func (w *walker) set(t syntax.Token, k Kind, mods int) { w.cls[pos(t)] = class{kind: k, mods: mods} }
func (w *walker) setAt(p diag.Pos, k Kind, mods int)   { w.cls[p] = class{kind: k, mods: mods} }

// produce classifies m's tokens: the trace of its parse, plus the tokens of each interpolation
// (which the parser reads with a lexer of its own, so the trace does not hold them).
func produce(m *syntax.Module, trace []syntax.Token) []Token {
	w := &walker{m: m, cls: map[diag.Pos]class{}, interp: map[diag.Pos][]syntax.Interpolation{}}
	interpolated := map[diag.Pos]bool{} // the literals the parser reads an interpolation in
	var strs func(n syntax.Spanned)
	strs = func(n syntax.Spanned) {
		if s, ok := n.(*syntax.StrLit); ok {
			for _, p := range s.Parts {
				if p.Expr != nil {
					interpolated[s.Span.Start] = true
					break
				}
			}
		}
		for _, c := range syntax.Children(n) {
			strs(c)
		}
	}
	strs(m)
	// The trace keeps the lexer's kinds; the parser reads a declaration keyword after column 1
	// as a name, so classify it as one.
	add := func(ts []syntax.Token) {
		for _, t := range ts {
			if t.Kind != syntax.NL && t.Kind != syntax.EOF {
				t.Kind = syntax.ParserKind(t)
				w.toks = append(w.toks, t)
			}
		}
	}
	add(trace)
	for i := 0; i < len(w.toks); i++ { // grows: a string inside an interpolation may have its own
		if t := w.toks[i]; t.Kind == syntax.String && interpolated[pos(t)] {
			ins := syntax.Interpolations(m.File, t)
			w.interp[pos(t)] = ins
			for _, in := range ins {
				add(in.Tokens)
			}
		}
	}
	sortTokens(w.toks)
	w.walk(m)
	return w.emit()
}

// in is the tokens that start inside sp, in order.
func (w *walker) in(sp diag.Span) []syntax.Token {
	i := sort.Search(len(w.toks), func(i int) bool { return !before(pos(w.toks[i]), sp.Start) })
	j := i
	for j < len(w.toks) && before(pos(w.toks[j]), sp.End) {
		j++
	}
	return w.toks[i:j]
}

// walk applies the rules after the children's, so the rule of the larger construct wins (an
// application's function field is a method, though an access makes its field a property).
func (w *walker) walk(n syntax.Spanned) {
	for _, c := range syntax.Children(n) {
		w.walk(c)
	}
	w.rule(n)
}

var operators = map[syntax.Kind]bool{syntax.Pipe: true, syntax.Arrow: true, syntax.LeftArrow: true,
	syntax.Equal: true, syntax.EqEq: true, syntax.NotEq: true, syntax.Lt: true, syntax.Le: true,
	syntax.Gt: true, syntax.Ge: true, syntax.Plus: true, syntax.Minus: true, syntax.Star: true,
	syntax.Slash: true, syntax.Percent: true, syntax.PlusPlus: true, syntax.AndAnd: true,
	syntax.OrOr: true, syntax.Question2: true, syntax.PipeGt: true, syntax.DotDot: true}

// lexical is a token's class when the tree says nothing of it: by its kind, and for a name,
// the default `(identifier) @variable` and `(constructor) @constructor` give.
func lexical(t syntax.Token) class {
	switch t.Kind {
	case syntax.Keyword, syntax.Backslash:
		return class{kind: TKeyword}
	case syntax.String:
		return class{kind: TString}
	case syntax.Int, syntax.Float:
		return class{kind: TNumber}
	case syntax.Hole:
		return class{kind: THole}
	case syntax.At:
		return class{kind: TDecorator}
	case syntax.LIdent:
		return class{kind: TVariable}
	case syntax.UIdent:
		return class{kind: TEnumMember}
	}
	if operators[t.Kind] {
		return class{kind: TOperator}
	}
	return class{kind: none}
}

// emit turns the classified tokens and the comments into the sorted list; a token overlapping
// the one before it is dropped (none should).
func (w *walker) emit() []Token {
	var out []Token
	for _, t := range w.toks {
		c, ok := w.cls[pos(t)]
		if !ok {
			c = lexical(t)
		}
		switch {
		case c.kind == none:
		case t.Kind == syntax.String && w.interp[pos(t)] != nil:
			out = append(out, pieces(t, w.interp[pos(t)])...)
		default:
			n := len(t.Text)
			if t.Kind == syntax.Hole {
				n++ // the token's text is the name, without its `?`
			}
			if c.end.Line == t.Line && c.end.Col > t.Col {
				n = c.end.Col - t.Col
			}
			out = append(out, Token{Line: t.Line, Col: t.Col, Len: n, Kind: c.kind, Mods: c.mods})
		}
	}
	out = append(out, w.comments()...)
	sortOut(out)
	kept := out[:0]
	for _, k := range out {
		if n := len(kept); k.Len <= 0 || n > 0 && kept[n-1].Line == k.Line && kept[n-1].Col+kept[n-1].Len > k.Col {
			continue
		}
		kept = append(kept, k)
	}
	return kept
}

// pieces are the string parts of t, a literal with interpolations: its text less each
// interpolation's expression (the braces stay string). The expressions' tokens are emitted on
// their own.
func pieces(t syntax.Token, ins []syntax.Interpolation) []Token {
	var out []Token
	at := 0
	for _, in := range ins {
		if in.From > at {
			out = append(out, Token{Line: t.Line, Col: t.Col + at, Len: in.From - at, Kind: TString})
		}
		at = in.To
	}
	if len(t.Text) > at {
		out = append(out, Token{Line: t.Line, Col: t.Col + at, Len: len(t.Text) - at, Kind: TString})
	}
	return out
}

// comments are the module's comments; one in the block of `--` lines ending on the line above a
// declaration (syntax.DocOf's rule) documents it.
func (w *walker) comments() []Token {
	doc := map[int]bool{}
	for _, d := range w.m.Decls {
		for l := d.GetSpan().Start.Line - 1; l >= 1 && l <= len(w.m.Lines) && strings.HasPrefix(strings.TrimSpace(w.m.Lines[l-1]), "--"); l-- {
			doc[l] = true
		}
	}
	var out []Token
	for _, c := range w.m.Comments {
		k := Token{Line: c.Line, Col: c.Col, Len: len(c.Text), Kind: TComment}
		if doc[c.Line] {
			k.Mods = Documentation
		}
		out = append(out, k)
	}
	return out
}
