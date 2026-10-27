package format

import (
	"bytes"
	"errors"
	"fmt"
	"slices"
	"strings"

	"alder/internal/diag"
	"alder/internal/syntax"
)

// ErrUnsafe reports that the printed program does not parse to the same program as the input;
// Source then returns no output.
var ErrUnsafe = errors.New("formatting changed the program")

// testMutate, when set by a test, rewrites the printed output before the safety check.
var testMutate func([]byte) []byte

// Source formats one Alder source file. A file that does not parse yields its diagnostics and no
// output. The output is re-parsed and must give the same program (Dump equality, imports taken
// as a set) and the same sequence of comments; otherwise Source returns an error wrapping
// ErrUnsafe and no output.
func Source(file string, src []byte) ([]byte, []diag.Diagnostic, error) {
	m, diags := syntax.ParseFile(file, src)
	if len(diags) > 0 {
		return nil, diags, nil
	}
	p := newPrinter(src)
	p.c = attach(m)
	out := []byte(strings.TrimRight(Render(p.module(m), Width), "\n") + "\n")
	if testMutate != nil {
		out = testMutate(out)
	}
	if !samePrograms(file, m, out) {
		return nil, nil, fmt.Errorf("%s: %w", file, ErrUnsafe)
	}
	return out, nil, nil
}

// samePrograms reports whether out parses, without errors, to the program m with the same
// comments.
func samePrograms(file string, m *syntax.Module, out []byte) bool {
	m2, diags := syntax.ParseFile(file, out)
	if len(diags) > 0 {
		return false
	}
	return programDump(m) == programDump(m2) && slices.Equal(commentTexts(m), commentTexts(m2))
}

// programDump is the module's Dump with its imports sorted and de-duplicated: the formatter
// sorts them, and their order and repetition mean nothing.
func programDump(m *syntax.Module) string {
	c := *m
	c.Imports = sortedImports(m.Imports)
	return syntax.Dump(&c)
}

func sortedImports(ims []*syntax.Import) []*syntax.Import {
	out := slices.Clone(ims)
	slices.SortStableFunc(out, func(a, b *syntax.Import) int { return strings.Compare(a.Module, b.Module) })
	return slices.CompactFunc(out, func(a, b *syntax.Import) bool { return a.Module == b.Module })
}

// commentTexts is the module's comments in order, except that those of the import block (after
// the module line, up to the last import's line) are sorted: they move with their imports, which
// the formatter sorts. A marker ends the block, so no comment can move in or out of it.
func commentTexts(m *syntax.Module) []string {
	var out []string
	if len(m.Imports) == 0 {
		for _, c := range m.Comments {
			out = append(out, c.Text)
		}
		return out
	}
	lo, hi := m.Span.Start.Line, m.Imports[len(m.Imports)-1].Span.End.Line
	block, ended := -1, false
	for _, c := range m.Comments {
		if c.Line > hi && !ended {
			out, ended = endImports(out, block), true
		}
		if c.Line > lo && block < 0 && !ended {
			block = len(out)
		}
		out = append(out, c.Text)
	}
	if !ended {
		out = endImports(out, block)
	}
	return out
}

// endImports sorts the import block's comments, out[block:] (none when block < 0), and ends the
// block with a marker.
func endImports(out []string, block int) []string {
	if block >= 0 {
		slices.Sort(out[block:])
	}
	return append(out, "\x00end of imports")
}

// printer turns a parsed module into a document. It reads literals, strings and markup text back
// from the source through their spans.
type printer struct {
	src    []byte   // the source as the lexer reads it: no BOM, LF line ends
	lines  []string // src split into lines
	starts []int    // byte offset of the start of each line
	// c places the module's comments (comments.go).
	c *attached
	// inFun counts the function types being printed around the current type: a record type
	// inside one is laid out by width alone.
	inFun int
	// noBrace is set while a match subject is printed: there an application argument cannot
	// start with `{`.
	noBrace bool
	// holes are the variables Paste writes as placeholders, with the text of each.
	holes map[*syntax.Var]string
}

func newPrinter(src []byte) *printer {
	src = bytes.TrimPrefix(src, []byte("\xEF\xBB\xBF"))
	src = bytes.ReplaceAll(src, []byte("\r\n"), []byte("\n"))
	p := &printer{src: src, lines: strings.Split(string(src), "\n"), starts: []int{0}}
	for i, c := range src {
		if c == '\n' {
			p.starts = append(p.starts, i+1)
		}
	}
	return p
}

// offset is the byte offset of a position (columns count bytes, as the lexer's do).
func (p *printer) offset(pos diag.Pos) int {
	if pos.Line < 1 {
		return 0
	}
	if pos.Line > len(p.starts) {
		return len(p.src)
	}
	return min(p.starts[pos.Line-1]+pos.Col-1, len(p.src))
}

// spanText returns the verbatim source of a span.
func (p *printer) spanText(s diag.Span) string {
	return string(p.src[p.offset(s.Start):p.offset(s.End)])
}

// blankBetween reports whether a blank line lies strictly between source lines a and b.
func (p *printer) blankBetween(a, b int) bool {
	for l := a + 1; l < b && l <= len(p.lines); l++ {
		if strings.TrimSpace(p.lines[l-1]) == "" {
			return true
		}
	}
	return false
}
