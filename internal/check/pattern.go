// Pattern checking and exhaustiveness.
package check

import (
	"fmt"
	"strings"

	"alder/internal/diag"
	"alder/internal/syntax"
	"alder/internal/types"
)

// checkPattern checks p against t, binding variables in env.
func (mc *modCtx) checkPattern(p syntax.Pattern, t types.Type, env *env) {
	c := mc.c
	t = c.u.Apply(t)
	switch p := p.(type) {
	case *syntax.PWild:
	case *syntax.PVar:
		env.bind(p.Name, t)
	case *syntax.PLit:
		lt := mc.infer(p.Lit, env, effect{})
		if err := c.u.Check(t, lt); err != nil {
			mc.mismatch(p.Span, lt, t)
		}
	case *syntax.PCtor:
		un, ok := t.(*types.Union)
		if !ok {
			if v, isVar := t.(*types.Var); isVar {
				if info, found := c.ctors[c.key(p.Ref.Module, p.Ref.Name)]; found {
					inst := c.u.Instantiate(info.Union.WithArgs(paramsOf(info.Union.Params)))
					if len(info.Union.Params) == 0 {
						inst = info.Union
					}
					c.u.Unify(v, inst)
					mc.checkPattern(p, inst, env)
					return
				}
			}
			c.errorf(diag.TypeMismatch, p.Span, "pattern `%s` is a constructor, but the value has type `%s`", p.Name, types.String(t))
			return
		}
		arg, found := types.CtorArg(un, p.Name)
		if !found {
			c.errorf(diag.TypeMismatch, p.Span, "pattern `%s` is not a constructor of `%s`", p.Name, types.String(un))
			return
		}
		if p.Arg == nil {
			if arg != nil {
				c.errorf(diag.TypeMismatch, p.Span, "constructor `%s` carries a value; write `%s x`", p.Name, p.Name)
			}
			return
		}
		if arg == nil {
			c.errorf(diag.TypeMismatch, p.Span, "constructor `%s` carries no value", p.Name)
			return
		}
		if inner, isCtor := p.Arg.(*syntax.PCtor); isCtor && inner.Arg != nil {
			c.errorf(diag.ParseError, inner.Span, "patterns nest one level; match on `x` in a second `match`")
			return
		}
		mc.checkPattern(p.Arg, arg, env)
	case *syntax.PRecord:
		rec, ok := t.(*types.Record)
		if !ok {
			c.errorf(diag.TypeMismatch, p.Span, "record pattern against `%s`", types.String(t))
			return
		}
		for _, f := range p.Fields {
			var ft types.Type
			for _, rf := range rec.Fields {
				if rf.Name == f.Name {
					ft = rf.Type
				}
			}
			if ft == nil {
				c.errorf(diag.UnknownName, f.Span, "no field `%s` on `%s`", f.Name, types.String(rec))
				continue
			}
			mc.checkPattern(f.Pat, ft, env)
		}
	case *syntax.PList:
		elem := c.u.Fresh()
		list := &types.App{Con: "List", Args: []types.Type{elem}}
		if err := c.u.Unify(t, list); err != nil {
			mc.mismatch(p.Span, list, t)
			return
		}
		for _, e := range p.Elems {
			mc.checkPattern(e, elem, env)
		}
		if p.Rest != "" {
			env.bind(p.Rest, list)
		}
	}
}

// ---- exhaustiveness

type exhaustiveness struct {
	mc       *modCtx
	subject  types.Type
	ctors    map[string]bool // for unions: covered ctor names
	bools    map[bool]bool
	listNil  bool
	listCons bool
	closed   bool   // a wildcard/var arm has been seen, or every case is covered
	kind     string // "union" | "bool" | "list" | "record" | "other"
	all      []string
}

func newExhaustiveness(mc *modCtx, subject types.Type) *exhaustiveness {
	ex := &exhaustiveness{mc: mc, subject: subject, ctors: map[string]bool{}, bools: map[bool]bool{}, kind: "other"}
	switch t := subject.(type) {
	case *types.Union:
		ex.kind = "union"
		for _, ct := range t.Constructors() {
			ex.all = append(ex.all, ct.Name)
		}
	case *types.Con:
		if t.Name == "Bool" {
			ex.kind = "bool"
		}
	case *types.App:
		if t.Con == "List" {
			ex.kind = "list"
		}
	case *types.Record:
		ex.kind = "record"
	}
	return ex
}

// refresh re-derives the kind once pattern checking has refined an initially unbound subject type.
func (ex *exhaustiveness) refresh(subject types.Type) {
	if ex.kind != "other" || ex.closed {
		return
	}
	*ex = *newExhaustiveness(ex.mc, ex.mc.c.u.Apply(subject))
}

func (ex *exhaustiveness) cover(p syntax.Pattern) {
	switch p := p.(type) {
	case *syntax.PWild, *syntax.PVar:
		ex.closed = true
	case *syntax.PRecord:
		if irrefutable(p) {
			ex.closed = true
		}
	case *syntax.PCtor:
		if ex.kind == "union" {
			if !ex.closed && ex.ctors[p.Name] && irrefutable(p.Arg) {
				ex.mc.c.errorf(diag.UnreachableArm, p.Span, "unreachable arm: `%s` is already handled", p.Name)
			}
			if irrefutable(p.Arg) {
				ex.ctors[p.Name] = true
			}
			if len(ex.ctors) == len(ex.all) {
				ex.closed = true
			}
		}
	case *syntax.PLit:
		if b, ok := p.Lit.(*syntax.BoolLit); ok {
			ex.bools[b.Value] = true
			if ex.bools[true] && ex.bools[false] {
				ex.closed = true
			}
		}
	case *syntax.PList:
		if len(p.Elems) == 0 && p.Rest == "" {
			ex.listNil = true
		} else if len(p.Elems) == 1 && p.Rest != "" && irrefutable(p.Elems[0]) {
			ex.listCons = true
		}
		if ex.listNil && ex.listCons {
			ex.closed = true
		}
	}
}

func irrefutable(p syntax.Pattern) bool {
	switch p := p.(type) {
	case nil, *syntax.PWild, *syntax.PVar:
		return true
	case *syntax.PRecord:
		for _, f := range p.Fields {
			if !irrefutable(f.Pat) {
				return false
			}
		}
		return true
	}
	return false
}

func (ex *exhaustiveness) report(m *syntax.Match) {
	if ex.closed {
		return
	}
	var missing []string
	switch ex.kind {
	case "union":
		for _, n := range ex.all {
			if !ex.ctors[n] {
				missing = append(missing, n)
			}
		}
	case "bool":
		for _, b := range []bool{true, false} {
			if !ex.bools[b] {
				missing = append(missing, map[bool]string{true: "True", false: "False"}[b])
			}
		}
	case "list":
		if !ex.listNil {
			missing = append(missing, "[]")
		}
		if !ex.listCons {
			missing = append(missing, "[x, ..rest]")
		}
	case "record":
		return
	default:
		missing = append(missing, "_")
	}
	if len(missing) == 0 {
		return
	}
	quoted := make([]string, len(missing))
	for i, n := range missing {
		quoted[i] = "`" + n + "`"
	}
	var d *diag.Diagnostic
	isUpdate := false
	if un, ok := ex.subject.(*types.Union); ok && ex.mc.inUpdate && un.Name == "Msg" {
		isUpdate = true
		d = ex.mc.c.errorf("E0302", m.Span, "`update` does not handle %s", strings.Join(quoted, ", "))
	} else {
		d = ex.mc.c.errorf(diag.NonExhaustive, m.Span, "missing cases: %s", strings.Join(quoted, ", "))
	}
	var arms []string
	for _, name := range missing {
		if ex.kind == "union" {
			if arg, _ := types.CtorArg(ex.subject.(*types.Union), name); arg != nil {
				if isUpdate {
					name += " p"
				} else {
					name += " x"
				}
			}
		}
		arms = append(arms, fmt.Sprintf("  %s -> ?todo,", name))
	}
	brace := diag.Pos{Line: m.Span.End.Line, Col: m.Span.End.Col - 1} // the closing `}`
	trimmed := make([]string, len(arms))
	for i, a := range arms {
		trimmed[i] = strings.TrimSpace(a)
	}
	switch {
	case len(m.Arms) == 0 && m.Span.Start.Line == m.Span.End.Line:
		d.Fix = diag.InsertAt(brace.Line, brace.Col, " "+strings.Join(trimmed, " "))
	case len(m.Arms) == 0:
		d.Fix = diag.InsertAt(brace.Line, 1, strings.Join(arms, "\n")+"\n")
	default:
		// Replace everything from the end of the last arm to the `}`, so the separating comma is
		// always present whether or not the source had a trailing one.
		last := m.Arms[len(m.Arms)-1]
		text := ", " + strings.Join(trimmed, " ") + " "
		if last.Span.End.Line < brace.Line {
			indent := ""
			if line := lineOf(ex.mc.m, brace.Line); brace.Col-1 <= len(line) && strings.TrimSpace(line[:brace.Col-1]) == "" {
				indent = line[:brace.Col-1]
			}
			text = ",\n" + strings.Join(arms, "\n") + "\n" + indent
		}
		d.Fix = diag.ReplaceSpan(diag.Span{Start: last.Span.End, End: brace}, text)
	}
}
