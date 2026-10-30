// Pattern checking, exhaustiveness and redundancy.
package check

import (
	"fmt"
	"strings"

	"alder/internal/diag"
	"alder/internal/format"
	"alder/internal/syntax"
	"alder/internal/types"
)

// checkPattern checks p against t, binding variables in env.
func (mc *modCtx) checkPattern(p syntax.Pattern, t types.Type, env *env) {
	c := mc.c
	t = c.u.Apply(t)
	if types.IsErr(t) {
		mc.bindPoison(p, env)
		return
	}
	switch p := p.(type) {
	case *syntax.PWild:
	case *syntax.PVar:
		c.bind(env, p, p.Name, t)
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
			c.errorf(diag.TypeMismatch, p.Span, "pattern `%s` is a constructor, but the value has type `%s`", p.Name, mc.ts(t))
			return
		}
		arg, found := types.CtorArg(un, p.Name)
		if !found {
			c.errorf(diag.TypeMismatch, p.Span, "pattern `%s` is not a constructor of `%s`", p.Name, mc.ts(un))
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
		mc.checkPattern(p.Arg, arg, env)
	case *syntax.PRecord:
		rec, ok := t.(*types.Record)
		if !ok {
			c.errorf(diag.TypeMismatch, p.Span, "record pattern against `%s`", mc.ts(t))
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
				c.errorf(diag.UnknownName, f.Span, "no field `%s` on `%s`", f.Name, mc.ts(rec))
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
			c.bind(env, p, p.Rest, list)
		}
	}
}

// bindPoison binds every variable of p to the poison type: the value's type is unknown
// because of an error already reported, so nothing about the pattern can be checked.
func (mc *modCtx) bindPoison(p syntax.Pattern, env *env) {
	switch p := p.(type) {
	case *syntax.PVar:
		mc.c.bind(env, p, p.Name, types.Err)
	case *syntax.PCtor:
		if p.Arg != nil {
			mc.bindPoison(p.Arg, env)
		}
	case *syntax.PRecord:
		for _, f := range p.Fields {
			mc.bindPoison(f.Pat, env)
		}
	case *syntax.PList:
		for _, e := range p.Elems {
			mc.bindPoison(e, env)
		}
		if p.Rest != "" {
			mc.c.bind(env, p, p.Rest, types.Err)
		}
	}
}

// ---- exhaustiveness and redundancy

// maxWitnesses caps the missing cases an E0004 or E0302 message lists when some case is nested;
// past it the message ends `, …`. A flat match's message lists every case, as it always did, and
// the fix always inserts an arm for every case, so applying it makes the match exhaustive.
const maxWitnesses = 8

// checkArms reports, once every arm's pattern is checked against the subject's type subj, the
// arms no value can reach (E0310) and the values no arm matches (E0004, or E0302 for a page's
// `update` on its `Msg`), by usefulness (usefulness.go). A poison subject reports nothing.
func (mc *modCtx) checkArms(subj types.Type, m *syntax.Match) {
	c := mc.c
	subj = c.u.Apply(subj)
	if types.IsErr(subj) {
		return
	}
	mt := &matcher{apply: c.u.Apply}
	tys := []types.Type{subj}
	var rows [][]*pat
	for _, a := range m.Arms {
		q := mt.normalise(a.Pat, subj)
		if !q.bad() && !mt.useful(rows, []*pat{q}, tys) {
			if _, open := mt.missing(rows, tys, true); !open {
				c.errorf(diag.UnreachableArm, a.Span, "unreachable arm: every case is already handled")
			} else {
				c.errorf(diag.UnreachableArm, a.Pat.GetSpan(), "unreachable arm: `%s` is already handled", format.Pattern(mc.m, a.Pat))
			}
		}
		rows = append(rows, []*pat{q})
	}
	ws := mt.witnesses(rows, subj)
	if len(ws) == 0 {
		return
	}
	shown, more := ws, false
	for _, w := range ws {
		if !w.flat() && len(ws) > maxWitnesses {
			shown, more = ws[:maxWitnesses], true
			break
		}
	}
	un, _ := subj.(*types.Union)
	isUpdate := un != nil && mc.inUpdate && un.Name == "Msg"
	payload := "x"
	if isUpdate {
		payload = "p"
	}
	missing := make([]string, len(ws))
	for i, w := range ws {
		missing[i] = printWitness(w, payload)
	}
	quoted := make([]string, len(shown))
	for i, w := range shown {
		quoted[i] = "`" + printWitness(w, "") + "`"
	}
	list := strings.Join(quoted, ", ")
	if more {
		list += ", …"
	}
	var d *diag.Diagnostic
	if isUpdate {
		d = c.errorf("E0302", m.Span, "`update` does not handle %s", list)
	} else {
		d = c.errorf(diag.NonExhaustive, m.Span, "missing cases: %s", list)
	}
	var arms []string
	for _, name := range missing {
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
			if line := lineOf(mc.m, brace.Line); brace.Col-1 <= len(line) && strings.TrimSpace(line[:brace.Col-1]) == "" {
				indent = line[:brace.Col-1]
			}
			text = ",\n" + strings.Join(arms, "\n") + "\n" + indent
		}
		d.Fix = diag.ReplaceSpan(diag.Span{Start: last.Span.End, End: brace}, text)
	}
}
