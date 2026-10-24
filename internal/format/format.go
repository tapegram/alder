package format

import (
	"bytes"
	"errors"
	"fmt"
	"reflect"
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

// programDump is the module's Dump with its imports sorted and de-duplicated (the formatter
// sorts them, and their order and repetition mean nothing) and every block that holds only a
// result replaced by the result (the formatter prints `{ x }` as `x`), except a test's whole
// body. It rewrites m's blocks in place.
func programDump(m *syntax.Module) string {
	unwrapResultBlocks(m)
	c := *m
	c.Imports = sortedImports(m.Imports)
	return syntax.Dump(&c)
}

var exprType = reflect.TypeFor[syntax.Expr]()

// unwrapResultBlocks replaces, in place, every expression that is a block holding only a result
// by that result. A test's body is left a block: there a block makes the test an effect test.
// The checker and the lowering treat such a block as its result everywhere else.
func unwrapResultBlocks(m *syntax.Module) {
	var visit func(v reflect.Value)
	visit = func(v reflect.Value) {
		switch v.Kind() {
		case reflect.Pointer, reflect.Interface:
			if !v.IsNil() {
				visit(v.Elem())
			}
		case reflect.Slice:
			for i := 0; i < v.Len(); i++ {
				if v.Type().Elem() == exprType {
					unwrapAt(v.Index(i))
				}
				visit(v.Index(i))
			}
		case reflect.Struct:
			_, isTest := v.Interface().(syntax.TestDecl)
			for i := 0; i < v.NumField(); i++ {
				f, ft := v.Field(i), v.Type().Field(i)
				if !ft.IsExported() {
					continue
				}
				if ft.Type == exprType && !(isTest && ft.Name == "Body") {
					unwrapAt(f)
				}
				visit(f)
			}
		}
	}
	visit(reflect.ValueOf(m))
}

// unwrapAt replaces the expression in v, while it is a block holding only a result, by the result.
func unwrapAt(v reflect.Value) {
	for {
		b, ok := v.Interface().(*syntax.Block)
		if !ok || b == nil || len(b.Stmts) > 0 || b.Result == nil {
			return
		}
		v.Set(reflect.ValueOf(&b.Result).Elem())
	}
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

// printer turns a parsed module into a document. It reads literals, strings and (until markup is
// printed) markup back from the source through their spans.
type printer struct {
	src    []byte   // the source as the lexer reads it: no BOM, LF line ends
	lines  []string // src split into lines
	starts []int    // byte offset of the start of each line
	// comments are the module's comments; attachment and printing come later, and a module
	// whose comments the output drops fails the safety check.
	comments []syntax.Comment
	// inFun counts the function types being printed around the current type: a record type
	// inside one is laid out by width alone.
	inFun int
	// noBrace is set while a match subject is printed: there an application argument cannot
	// start with `{`.
	noBrace bool
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
