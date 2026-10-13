package generate

import (
	"regexp"
	"slices"
	"strings"

	"alder/internal/diag"
	"alder/internal/names"
	"alder/internal/syntax"
)

// Hole is one typed hole (`?name`) of the app, from the checker's W0002: where it is and the
// type it must have.
type Hole struct {
	Module string `json:"module"`
	// Decl is the declaration holding it: a value's name ("update", "view"), a test's name in
	// quotes (`"signing adds an entry"`), or a fake's name ("Weather", whose entry is Arm).
	Decl string `json:"decl"`
	// Arm is the message of the `update` arm holding it ("Archive"), or the entry of the fake;
	// "" otherwise.
	Arm  string    `json:"arm,omitempty"`
	Name string    `json:"name"` // without the `?`
	Type string    `json:"type"`
	File string    `json:"file"`
	Line int       `json:"line"`
	Col  int       `json:"col"`
	Span diag.Span `json:"-"`
}

// Address is how `alder fill` names the hole's declaration: Module.decl[.arm].
func (h Hole) Address() string {
	s := h.Module + "." + h.Decl
	if h.Arm != "" {
		s += "." + h.Arm
	}
	return s
}

// DeclText is the declaration as `alder fill --list` prints it: decl[.arm].
func (h Hole) DeclText() string {
	if h.Arm != "" {
		return h.Decl + "." + h.Arm
	}
	return h.Decl
}

// holeMsg reads W0002's message, "hole ?name has type T", after any prefix saying where ("in
// `fake Weather`'s `current`: ").
var holeMsg = regexp.MustCompile(`hole \?(\w+) has type (.*)$`)

// HoleIndex lists the typed holes the diagnostics report (W0002), in the diagnostics' order
// (file, then position), each placed in its module and declaration.
func HoleIndex(prog *names.Program, diags []diag.Diagnostic) []Hole {
	byFile := map[string]*syntax.Module{}
	for _, m := range prog.Modules {
		byFile[m.File] = m
	}
	var out []Hole
	for _, d := range diags {
		if d.Code != "W0002" {
			continue
		}
		m := holeMsg.FindStringSubmatch(d.Message)
		mod := byFile[d.Span.File]
		if m == nil || mod == nil {
			continue
		}
		h := Hole{Module: mod.Name, Name: m[1], Type: m[2], File: d.Span.File, Line: d.Span.Start.Line, Col: d.Span.Start.Col, Span: d.Span}
		h.Decl, h.Arm = declAt(mod, d.Span.Start)
		out = append(out, h)
	}
	slices.SortStableFunc(out, func(a, b Hole) int {
		if a.File != b.File {
			return strings.Compare(a.File, b.File)
		}
		if a.Line != b.Line {
			return a.Line - b.Line
		}
		return a.Col - b.Col
	})
	return out
}

// declAt names the declaration of m at pos, and the arm (an `update` arm's message, a fake's
// entry) when there is one.
func declAt(m *syntax.Module, pos diag.Pos) (decl, arm string) {
	for _, d := range m.Decls {
		if !contains(d.GetSpan(), pos) {
			continue
		}
		switch d := d.(type) {
		case *syntax.ValueDecl:
			if mt, ok := d.Body.(*syntax.Match); ok {
				for _, a := range mt.Arms {
					if pc, ok := a.Pat.(*syntax.PCtor); ok && contains(a.Span, pos) {
						return d.Name, pc.Name
					}
				}
			}
			return d.Name, ""
		case *syntax.TestDecl:
			return quote(d.Name), ""
		case *syntax.FakeDecl:
			for _, e := range d.Entries {
				if contains(e.Span, pos) {
					return d.Name, e.Name
				}
			}
			return d.Name, ""
		}
	}
	return "", ""
}

// contains reports whether pos lies in sp (its end exclusive).
func contains(sp diag.Span, pos diag.Pos) bool {
	after := pos.Line > sp.Start.Line || pos.Line == sp.Start.Line && pos.Col >= sp.Start.Col
	before := pos.Line < sp.End.Line || pos.Line == sp.End.Line && pos.Col < sp.End.Col
	return after && before
}
