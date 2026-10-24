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

func commentTexts(m *syntax.Module) []string {
	var out []string
	for _, c := range m.Comments {
		out = append(out, c.Text)
	}
	return out
}

// printer turns a parsed module into a document. It reads literals and (until expressions are
// printed) expression bodies back from the source through their spans.
type printer struct {
	src    []byte   // the source as the lexer reads it: no BOM, LF line ends
	lines  []string // src split into lines
	starts []int    // byte offset of the start of each line
	// comments are the module's comments; attachment and printing come later, and a module
	// whose comments the output drops fails the safety check.
	comments []syntax.Comment
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

// expr prints an expression. For now it is the expression's source, verbatim, with any
// parentheses around the whole of it (which its span leaves out): expressions are printed
// only as whole declaration bodies and field defaults, where verbatim source re-parses to the
// same thing.
func (p *printer) expr(e syntax.Expr) Doc {
	sp := e.GetSpan()
	from, to := p.offset(sp.Start), p.offset(sp.End)
	for {
		i := from - 1
		for i >= 0 && isBlank(p.src[i]) {
			i--
		}
		j := to
		for j < len(p.src) && isBlank(p.src[j]) {
			j++
		}
		if i < 0 || j >= len(p.src) || p.src[i] != '(' || p.src[j] != ')' {
			break
		}
		from, to = i, j+1
	}
	return Text(string(p.src[from:to]))
}

func isBlank(c byte) bool { return c == ' ' || c == '\t' || c == '\n' || c == '\r' }
