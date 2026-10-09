package diag

import (
	"fmt"
	"sort"
)

type Pos struct{ Line, Col int }
type Span struct {
	File       string
	Start, End Pos
}
type Fix struct {
	Line, Col int
	Replace   string
}
type Diagnostic struct {
	Code     string
	Severity string // "error" | "warning"
	Span     Span
	Message  string
	Fix      *Fix
}

// General codes. Spec codes (E0101..E0602) are declared where they are raised.
const (
	ParseError      = "E0001"
	UnknownName     = "E0002"
	TypeMismatch    = "E0003"
	NonExhaustive   = "E0004"
	ModulePath      = "E0005"
	ImportCycle     = "E0006"
	MissingSig      = "E0007"
	Arity           = "E0008"
	Duplicate       = "E0009"
	MissingDecl     = "E0010"
	InvalidDecl     = "E0011"
	UnreachableArm  = "E0310"
	UnusedImport    = "W0001"
)

type Bag struct{ Items []Diagnostic }

// add is an internal method that appends a diagnostic and returns a pointer to it.
// Callers must set .Fix immediately (if needed) before the next append, as append may reallocate.
func (b *Bag) add(sev, code string, span Span, format string, args ...any) *Diagnostic {
	b.Items = append(b.Items, Diagnostic{Code: code, Severity: sev, Span: span, Message: fmt.Sprintf(format, args...)})
	return &b.Items[len(b.Items)-1]
}

// Error adds an error diagnostic and returns a pointer to it.
// Callers must set .Fix immediately (if needed) before adding another diagnostic.
func (b *Bag) Error(code string, span Span, format string, args ...any) *Diagnostic {
	return b.add("error", code, span, format, args...)
}

// Warn adds a warning diagnostic and returns a pointer to it.
// Callers must set .Fix immediately (if needed) before adding another diagnostic.
func (b *Bag) Warn(code string, span Span, format string, args ...any) *Diagnostic {
	return b.add("warning", code, span, format, args...)
}

func (b *Bag) HasErrors() bool {
	for _, d := range b.Items {
		if d.Severity == "error" {
			return true
		}
	}
	return false
}

func (b *Bag) Sort() {
	sort.SliceStable(b.Items, func(i, j int) bool {
		a, c := b.Items[i].Span, b.Items[j].Span
		if a.File != c.File {
			return a.File < c.File
		}
		if a.Start.Line != c.Start.Line {
			return a.Start.Line < c.Start.Line
		}
		return a.Start.Col < c.Start.Col
	})
}
