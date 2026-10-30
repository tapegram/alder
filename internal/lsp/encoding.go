package lsp

import (
	"slices"
	"strings"
)

// posEncoding is the unit of an LSP position's character offset, negotiated at initialize
// (LSP 3.17 general.positionEncodings). Everything inside the server counts UTF-8 bytes, as the
// compiler does. This file is the one place that converts, at the protocol boundary: positions
// coming in (server.byteAt, fromWire) and ranges going out (wireRange, toWire). With utf-8 both
// directions are the identity.
type posEncoding int

const (
	utf16Enc posEncoding = iota // the LSP default, when the client does not offer utf-8
	utf8Enc
)

func (e posEncoding) String() string {
	if e == utf8Enc {
		return "utf-8"
	}
	return "utf-16"
}

// negotiate picks utf-8 when the client lists it, else utf-16.
func negotiate(offered []string) posEncoding {
	if slices.Contains(offered, "utf-8") {
		return utf8Enc
	}
	return utf16Enc
}

func units(r rune) int {
	if r >= 0x10000 {
		return 2
	}
	return 1
}

// toWire is the character offset, in e's unit, of the byte column col in line. A column past the
// line's end keeps its excess.
func (e posEncoding) toWire(line string, col int) int {
	if e == utf8Enc || col <= 0 {
		return col
	}
	n := 0
	for i, r := range line {
		if i >= col {
			return n
		}
		n += units(r)
	}
	return n + col - len(line)
}

// fromWire is the byte column of the character offset ch (in e's unit) in line; an offset inside
// a character gives that character's start. An offset past the end keeps its excess.
func (e posEncoding) fromWire(line string, ch int) int {
	if e == utf8Enc || ch <= 0 {
		return ch
	}
	n := 0
	for i, r := range line {
		if n+units(r) > ch {
			return i
		}
		n += units(r)
	}
	return len(line) + ch - n
}

// wireRange is r, whose columns are bytes in lines, in e's unit. A line outside lines is left alone.
func (e posEncoding) wireRange(lines []string, r Range) Range {
	if e == utf8Enc {
		return r
	}
	conv := func(p Position) Position {
		if p.Line >= 0 && p.Line < len(lines) {
			p.Character = e.toWire(lines[p.Line], p.Character)
		}
		return p
	}
	return Range{Start: conv(r.Start), End: conv(r.End)}
}

// wireEdit is t with its range in e's unit.
func (e posEncoding) wireEdit(lines []string, t TextEdit) TextEdit {
	t.Range = e.wireRange(lines, t.Range)
	return t
}

// wireDiagnostic is d with its range and its fix's range in e's unit.
func (e posEncoding) wireDiagnostic(lines []string, d Diagnostic) Diagnostic {
	d.Range = e.wireRange(lines, d.Range)
	if d.Data != nil && d.Data.Fix != nil {
		f := e.wireEdit(lines, *d.Data.Fix)
		d.Data = &diagnosticData{Fix: &f}
	}
	return d
}

// byteAt is the client position p in rel as a byte position, read against rel's current text.
func (s *server) byteAt(rel string, p Position) Position {
	if s.enc == utf8Enc {
		return p
	}
	return s.byteAtLines(strings.Split(s.text(rel), "\n"), p)
}

// byteAtLines is byteAt against lines.
func (s *server) byteAtLines(lines []string, p Position) Position {
	if s.enc != utf8Enc && p.Line >= 0 && p.Line < len(lines) {
		p.Character = s.enc.fromWire(lines[p.Line], p.Character)
	}
	return p
}
