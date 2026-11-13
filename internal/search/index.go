// Package search indexes the declarations of the stdlib and of an app, for name and type search.
package search

import (
	"sort"
	"strings"
	"sync"

	"alder/internal/check"
	"alder/internal/diag"
	"alder/internal/names"
	"alder/internal/syntax"
	"alder/internal/types"
)

// Kind is what an entry declares.
type Kind string

const (
	Function    Kind = "function"
	Value       Kind = "value"
	TypeDecl    Kind = "type"
	Constructor Kind = "constructor"
)

// Entry is one searchable declaration.
type Entry struct {
	Name, Module               string
	Kind                       Kind
	Type                       types.Type // nil for Kind TypeDecl
	Signature                  string
	Doc                        string
	Source                     string // "stdlib" or "app"
	Effect                     string // "", "query" or "task"
	File                       string // app entries only; root-relative, as syntax.Module.File
	Line, Col, EndLine, EndCol int    // 1-based, as diag.Span; 0 for stdlib
}

// Index is the searchable declarations of a checked program.
type Index struct {
	Entries []Entry

	decls map[string]types.Type // the declared types by "Module.Type" (res.Decls)
	base  *types.Unifier        // res.U's tables and variable counter, with an empty substitution
}

// Build indexes the stdlib and the app's modules of a checked program. res must not be nil.
func Build(prog *names.Program, res *check.Result) *Index {
	base := res.U.Clone()
	base.Subst = types.Subst{}
	ix := &Index{decls: res.Decls, base: base}

	stdNames := make([]string, 0, len(prog.Std))
	for name := range prog.Std {
		stdNames = append(stdNames, name)
	}
	sort.Strings(stdNames)
	for _, name := range stdNames {
		ix.addModule(prog.Std[name], "stdlib", res)
	}
	for _, name := range prog.Order {
		if m := prog.Modules[name]; m != nil {
			ix.addModule(m, "app", res)
		}
	}
	return ix
}

func (ix *Index) addModule(m *syntax.Module, source string, res *check.Result) {
	// where fills an entry's own position: app entries only.
	where := func(e *Entry, sp diag.Span) {
		e.Doc = syntax.DocOf(m.Lines, sp.Start.Line)
		if source == "app" {
			e.File = m.File
			e.Line, e.Col, e.EndLine, e.EndCol = sp.Start.Line, sp.Start.Col, sp.End.Line, sp.End.Col
		}
	}
	for _, d := range m.Decls {
		switch d := d.(type) {
		case *syntax.ValueDecl:
			if d.Route != "" {
				continue
			}
			if source == "stdlib" && m.Name == "Test" && (check.IsTestStep(d.Name) || d.Name == "fake") {
				continue // special forms the checker handles, not declarations
			}
			t, ok := res.Values[m.Name+"."+d.Name]
			if !ok {
				continue
			}
			t = res.U.Apply(t)
			e := Entry{Name: d.Name, Module: m.Name, Kind: Value, Type: t, Signature: types.String(t),
				Source: source, Effect: effectOf(t)}
			if _, fn := t.(*types.Fun); fn {
				e.Kind = Function
			}
			where(&e, d.GetSpan())
			ix.Entries = append(ix.Entries, e)
		case *syntax.TypeDecl:
			sig := "type " + d.Name
			if len(d.Params) > 0 {
				sig += " " + strings.Join(d.Params, " ")
			}
			e := Entry{Name: d.Name, Module: m.Name, Kind: TypeDecl, Signature: sig, Source: source}
			where(&e, d.GetSpan())
			ix.Entries = append(ix.Entries, e)
			for _, c := range d.Union {
				info := res.Ctors[m.Name+"."+c.Name]
				if info == nil {
					continue
				}
				union := info.Union
				if len(union.Args) == 0 && len(union.Params) > 0 { // the declaration: spell its parameters
					args := make([]types.Type, len(union.Params))
					for i, p := range union.Params {
						args[i] = &types.Param{Name: p}
					}
					union = union.WithArgs(args)
				}
				var t types.Type = union
				if info.Arg != nil {
					t = &types.Fun{Params: []types.Type{info.Arg}, Result: union}
				}
				t = res.U.Apply(t)
				ce := Entry{Name: c.Name, Module: m.Name, Kind: Constructor, Type: t, Signature: types.String(t),
					Source: source, Effect: effectOf(t)}
				where(&ce, c.GetSpan())
				ix.Entries = append(ix.Entries, ce)
			}
		}
	}
}

// effectOf is "query" or "task" when t, or a function's result, is a Query or Task.
func effectOf(t types.Type) string {
	if f, ok := t.(*types.Fun); ok {
		t = f.Result
	}
	if a, ok := t.(*types.App); ok {
		switch a.Con {
		case "Query":
			return "query"
		case "Task":
			return "task"
		}
	}
	return ""
}

var (
	stdOnce sync.Once
	stdIdx  *Index
	stdErr  error
)

// Std is the stdlib-only index, built once per process.
func Std() (*Index, error) {
	stdOnce.Do(func() { stdIdx, stdErr = buildStd() })
	return stdIdx, stdErr
}

func buildStd() (*Index, error) {
	std, err := types.LoadStd()
	if err != nil {
		return nil, err
	}
	prog, bag := names.Build(nil, std)
	prog.Resolve(bag)
	prog.MarkFailed(bag)
	res := check.Program(prog, bag)
	if bag.HasErrors() {
		return nil, &stdError{bag}
	}
	return Build(prog, res), nil
}

type stdError struct{ bag *diag.Bag }

func (e *stdError) Error() string {
	for _, d := range e.bag.Items {
		if d.Severity == "error" {
			return "search: checking the stdlib: " + d.Code + " " + d.Message
		}
	}
	return "search: checking the stdlib failed"
}
