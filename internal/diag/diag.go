package diag

import (
	"fmt"
	"sort"
	"strings"
)

type Pos struct{ Line, Col int }
type Span struct {
	File       string
	Start, End Pos
}
type Fix struct {
	Line, Col       int
	EndLine, EndCol int
	Replace         string
}

// ReplaceSpan is a fix that replaces the half-open span [sp.Start, sp.End) with text.
func ReplaceSpan(sp Span, text string) *Fix {
	return &Fix{Line: sp.Start.Line, Col: sp.Start.Col, EndLine: sp.End.Line, EndCol: sp.End.Col, Replace: text}
}

// InsertAt is a zero-width fix: text is inserted before (line, col).
func InsertAt(line, col int, text string) *Fix {
	return &Fix{Line: line, Col: col, EndLine: line, EndCol: col, Replace: text}
}

// Apply applies fixes to src (lines joined by "\n"). Fixes must not overlap and must lie within src.
func Apply(src string, fixes []Fix) (string, error) {
	lines := strings.Split(src, "\n")
	offset := func(line, col int) (int, error) {
		if line < 1 || line > len(lines) || col < 1 || col > len(lines[line-1])+1 {
			return 0, fmt.Errorf("fix position %d:%d is outside the source", line, col)
		}
		n := 0
		for i := 0; i < line-1; i++ {
			n += len(lines[i]) + 1
		}
		return n + col - 1, nil
	}
	type span struct {
		s, e int
		text string
	}
	spans := make([]span, 0, len(fixes))
	for _, f := range fixes {
		s, err := offset(f.Line, f.Col)
		if err != nil {
			return "", err
		}
		e, err := offset(f.EndLine, f.EndCol)
		if err != nil {
			return "", err
		}
		if e < s {
			return "", fmt.Errorf("fix end before start at %d:%d", f.Line, f.Col)
		}
		spans = append(spans, span{s, e, f.Replace})
	}
	sort.SliceStable(spans, func(i, j int) bool {
		if spans[i].s != spans[j].s {
			return spans[i].s > spans[j].s
		}
		return spans[i].e > spans[j].e
	})
	for i := 1; i < len(spans); i++ {
		if spans[i].e > spans[i-1].s {
			return "", fmt.Errorf("overlapping fixes")
		}
	}
	out := src
	for _, sp := range spans {
		out = out[:sp.s] + sp.text + out[sp.e:]
	}
	return out, nil
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
	ParseError     = "E0001"
	UnknownName    = "E0002"
	TypeMismatch   = "E0003"
	NonExhaustive  = "E0004"
	ModulePath     = "E0005"
	ImportCycle    = "E0006"
	MissingSig     = "E0007"
	Arity          = "E0008"
	Duplicate      = "E0009"
	MissingDecl    = "E0010"
	InvalidDecl    = "E0011"
	ModuleCase     = "E0014"
	ReservedModule = "E0015"
	DuplicatePage  = "E0016"
	StdModule      = "E0017"
	TablePlacement = "E0018"
	SchemaChanged  = "E0019" // raised by buildkit.SchemaDrift, not the checker: the tables changed since the last migration
	FakePlacement  = "E0020" // a `fake` declared outside its capability's module
	FakeTable      = "E0021" // a fake's table used outside tests and the fake
	TableDefault   = "E0022"
	BadSelector    = "E0023" // a selector literal of a Test query that does not parse
	OwnerColumn    = "E0024" // an `@owner` that is not one `@ref` column of the session's user type
	NoOwner        = "E0025" // a scoped Db operation on a table without `@owner`
	ScopedOutside  = "E0026" // a scoped Db operation outside a module whose page `requires user`
	RoleClause     = "E0027" // `requires … role R` without `user`, or without a session `role` union having R
	UnreachableArm = "E0310"
	UnusedImport   = "W0001"
	UnshownErrors  = "W0003"
	PlainDefault   = "W0004"
	UnshownReject  = "W0005" // a message whose update may answer Reject, posted by a form without <Errors form />
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
