package semtok

import (
	"regexp"
	"strings"

	"alder/internal/diag"
	"alder/internal/scratch"
	"alder/internal/syntax"
)

// ScratchDoc is a scratch file's text parsed as the run parses it: its module source
// (scratch.ParseFile) behind a `module Scratch` line. Module line L+1 is file line L; the columns
// differ only on a watch's first line, where the module has `itK = ` for the file's `>` and the
// spaces after it (File.Map.Shift, which is 0 on every other line).
type ScratchDoc struct {
	File   scratch.File
	Module *syntax.Module
	Trace  []syntax.Token // the parse's tokens (syntax.ParseTokens), in module positions
}

// ParseScratch parses text, the scratch file file's contents.
func ParseScratch(file, text string) ScratchDoc {
	f, _ := scratch.ParseFile(file, text)
	m, trace, _ := syntax.ParseTokens(file, []byte("module Scratch\n"+f.Source))
	return ScratchDoc{File: f, Module: m, Trace: trace}
}

// ToFile is p, a position in d.Module, in the file (both 1-based). One in a watch's generated
// `itK = ` is put at the start of its line; one on the `module Scratch` line on the file's first.
func (d ScratchDoc) ToFile(p diag.Pos) diag.Pos {
	line, col := p.Line-1, p.Col
	if line >= 1 && line <= len(d.File.Map.Shift) {
		col += d.File.Map.Shift[line-1]
	}
	return diag.Pos{Line: max(line, 1), Col: max(col, 1)}
}

// ToModule is p, a position in the file, in d.Module: ToFile's inverse. ToFile(ToModule(p)) is p
// for every p in the file.
func (d ScratchDoc) ToModule(p diag.Pos) diag.Pos {
	col := p.Col
	if p.Line >= 1 && p.Line <= len(d.File.Map.Shift) {
		col -= d.File.Map.Shift[p.Line-1]
	}
	return diag.Pos{Line: p.Line + 1, Col: col}
}

var segmentRE = regexp.MustCompile(`[A-Z][A-Za-z0-9_]*`)

// Scratch classifies a scratch file's text: its module's tokens (ParseScratch) mapped to the
// file's lines and columns, without the `module Scratch` line or the generated `itK =`, plus the
// `in` header and each watch's `>`.
func Scratch(file, text string) []Token {
	d := ParseScratch(file, text)
	gen := map[int]int{} // a watch's first line in the module → the width of its `itK = `
	for _, w := range d.File.Watches {
		gen[w.Line+1] = len(w.Name) + len(" = ")
	}
	var out []Token
	for _, k := range produce(d.Module, d.Trace) {
		if k.Line == 1 || k.Col <= gen[k.Line] { // the `module Scratch` line; `itK` and its `=`
			continue
		}
		p := d.ToFile(diag.Pos{Line: k.Line, Col: k.Col})
		k.Line, k.Col = p.Line, p.Col
		out = append(out, k)
	}
	for i, l := range d.File.Lines { // the header: the first line that is not blank or a comment
		s := strings.TrimSpace(l)
		if s == "" || strings.HasPrefix(s, "--") {
			continue
		}
		if l == "in" || strings.HasPrefix(l, "in ") || strings.HasPrefix(l, "in\t") {
			out = append(out, Token{Line: i + 1, Col: 1, Len: 2, Kind: TKeyword})
			for _, seg := range segmentRE.FindAllStringIndex(l[2:], -1) {
				out = append(out, Token{Line: i + 1, Col: 3 + seg[0], Len: seg[1] - seg[0], Kind: TNamespace})
			}
		}
		break
	}
	for _, w := range d.File.Watches {
		out = append(out, Token{Line: w.Line, Col: 1, Len: 1, Kind: TOperator})
	}
	sortOut(out)
	return out
}
