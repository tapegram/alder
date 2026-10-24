package scratch

import (
	"regexp"
	"strings"

	"alder/internal/diag"
	"alder/internal/gen"
	"alder/internal/types"
)

// RunOutcome is how a scratch run ended (ClassifyRun).
type RunOutcome int

const (
	RunOK       RunOutcome = iota
	RunFailed              // the program reported a failure (a panic, a failed effect)
	RunCrashed             // it overflowed its stack or ran past its time limit
	RunInternal            // it could not be run at all
)

// ClassifyRun is what eval and the REPL report of a scratch run that ended with exit code exit
// and standard error errOut (RunTethered's; the caller has already told its own
// interruption apart): the message (without the tool's prefix) and how the run ended. A stack
// overflow is one line (Go's trace of it is hundreds about the generated code), a panic its
// first paragraph (the goroutine dump says nothing about the input), a timeout says so without
// the scratch program's path.
func ClassifyRun(errOut string, exit int) (string, RunOutcome) {
	switch {
	case exit == 0:
		return "", RunOK
	case exit == -1 && !timedOutRE.MatchString(errOut):
		return strings.TrimSpace(errOut), RunInternal
	case strings.Contains(errOut, "goroutine stack exceeds") || strings.Contains(errOut, "fatal error: stack overflow"):
		return "stack overflow (unbounded recursion?)", RunCrashed
	}
	if m := timedOutRE.FindStringSubmatch(errOut); m != nil && exit == -1 {
		return "the evaluation " + m[1], RunCrashed
	}
	if strings.HasPrefix(errOut, "panic: ") {
		errOut, _, _ = strings.Cut(errOut, "\n\n")
	}
	return strings.TrimSpace(errOut), RunFailed
}

// timedOutRE is the line RunStream adds when a program runs past its deadline.
var timedOutRE = regexp.MustCompile(`(?m)^scratch: .* (timed out.*)$`)

// GoValue is the Go expression for the scratch module mod's value name of type sig in the
// generated package: a polymorphic value is a generic thunk (`func Scratch__it[A any]() …`) and
// a polymorphic function a generic function, both instantiated here at `any` (an empty `[]`
// shows the same at any element type). With thunk set, a non-function value is a constant that
// gen emits as a func, which is called (`app.Scratch__it3()`, or `app.Scratch__it3[any]()` when
// generic); eval's `it` is not (gen gives it no other thunk, since nothing can refer to it).
func GoValue(mod, name string, sig types.Type, thunk bool) string {
	v := "app." + gen.GoName(mod, name)
	n := len(TypeParams(sig))
	if n > 0 {
		v += "[" + strings.TrimSuffix(strings.Repeat("any, ", n), ", ") + "]"
	}
	if _, isFun := sig.(*types.Fun); !isFun && (n > 0 || thunk) {
		v += "()"
	}
	return v
}

// TypeParams lists the distinct type parameters of t the way gen counts a declaration's Go
// type parameters: not those inside an `Html` (its message type is erased).
func TypeParams(t types.Type) []string {
	var out []string
	seen := map[string]bool{}
	var walk func(types.Type)
	walk = func(t types.Type) {
		switch t := t.(type) {
		case *types.Param:
			if !seen[t.Name] {
				seen[t.Name] = true
				out = append(out, t.Name)
			}
		case *types.Brand:
			walk(t.Base)
		case *types.App:
			if t.Con == "Html" {
				return
			}
			for _, a := range t.Args {
				walk(a)
			}
		case *types.Record:
			if t.Name != "" && !t.Anonymous {
				for _, a := range t.Args {
					walk(a)
				}
				return
			}
			for _, f := range t.Fields {
				walk(f.Type)
			}
		case *types.Union:
			for _, a := range t.Args {
				walk(a)
			}
		case *types.Fun:
			for _, p := range t.Params {
				walk(p)
			}
			walk(t.Result)
		}
	}
	walk(t)
	return out
}

// SourceMap maps positions in a scratch module's source back to the text it was assembled
// from: the input of eval and the REPL, or a scratch file.
type SourceMap struct {
	Lines []string // the original text's lines
	// For line i+1 of the module's source: InLine[i] is the line of the original it comes
	// from, and Shift[i] is what to add to a column on it to get the original's column.
	InLine, Shift []int
}

// Position maps a diagnostic of the scratch module into the original text, as file file: its
// lines through mapLine (Checked.MapDiag) and the map, its columns undoing the rewriting, and
// the line a duplicate's message names (E0009) likewise. One on a line Compile inserted (not
// mapped) is put at 1:1 with its message unchanged; one past the end of the source at the end of
// the text.
func (m SourceMap) Position(d diag.Diagnostic, mapLine func(diag.Diagnostic) (diag.Diagnostic, bool), file string) diag.Diagnostic {
	d, ok := mapLine(d)
	d.Span.File = file
	if !ok {
		d.Span.Start, d.Span.End, d.Fix = diag.Pos{Line: 1, Col: 1}, diag.Pos{Line: 1, Col: 1}, nil
		return d
	}
	pos := func(p diag.Pos) diag.Pos {
		if p.Line < 1 {
			return diag.Pos{Line: 1, Col: 1}
		}
		if p.Line > len(m.InLine) { // past the end of the text
			n := len(m.Lines)
			if n == 0 {
				return diag.Pos{Line: 1, Col: 1}
			}
			return diag.Pos{Line: n, Col: len(m.Lines[n-1]) + 1}
		}
		return diag.Pos{Line: m.InLine[p.Line-1], Col: max(1, p.Col+m.Shift[p.Line-1])}
	}
	d.Span.Start, d.Span.End = pos(d.Span.Start), pos(d.Span.End)
	d.Message = MapLineRef(d, func(l int) (int, bool) {
		if l < 1 || l > len(m.InLine) {
			return 0, false
		}
		return m.InLine[l-1], true
	})
	if d.Fix != nil {
		f := *d.Fix
		start, end := pos(diag.Pos{Line: f.Line, Col: f.Col}), pos(diag.Pos{Line: f.EndLine, Col: f.EndCol})
		f.Line, f.Col, f.EndLine, f.EndCol = start.Line, start.Col, end.Line, end.Col
		d.Fix = &f
	}
	return d
}
