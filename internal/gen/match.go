package gen

import (
	"fmt"
	"strconv"
	"strings"

	"alder/internal/ir"
	"alder/internal/types"
)

// Match emission (ret mode; a match in expression position is wrapped in a closure first).
//
//	union of the app  switch s.Tag { case M__T_Tag_C: … }, panic("unreachable") after it when
//	                  there is no default
//	Maybe, Result     if s.Ok { … } / if s.IsOk { … } then the other case or the default
//	Bool              if s { … } (or if !s) then the other case or the default
//	List              if len(s) == 0 { … } (or len(s) > 0) then the other case or the default
//	literal           switch s { case 1: … default: … }
//
// A case binds the parts it needs to Go variables (`_m1 := s.Circle.Value`, `_m2 := s[1:]`)
// before its body. When the body starts by renaming such a part to a user name (Lower's
// `let r = _m1`, the only use of _m1), the part is bound to the user name directly
// (`r := s.Circle.Value`).

func (f *fn) match(w *strings.Builder, x *ir.Match, s *scope) {
	subj, p := f.expr(x.Subject, s)
	if !isPath(x.Subject) && hasBinds(x) { // binds would re-evaluate the subject
		t := f.temp("_s")
		fmt.Fprintf(w, "%s := %s\n", t, subj)
		subj, p = t, precAtom
	}
	if p < precAtom {
		subj = "(" + subj + ")"
	}
	switch x.Kind {
	case ir.MatchUnion:
		u, _ := x.Subject.Type().(*types.Union)
		if u != nil && u.Module == "Prelude" {
			field := "Ok"
			if u.Name == "Result" {
				field = "IsOk"
			}
			f.twoWay(w, x, s, subj, func(tag string) string {
				if tag == "Just" || tag == "Ok" {
					return subj + "." + field
				}
				return "!" + subj + "." + field
			})
			return
		}
		fmt.Fprintf(w, "switch %s.Tag {\n", subj)
		for _, c := range x.Cases {
			fmt.Fprintf(w, "case %s:\n", tagConst(u.Module, u.Name, c.Tag))
			f.caseBody(w, x, c, subj, s.child())
		}
		if x.Default != nil {
			w.WriteString("default:\n")
			f.ret(w, x.Default, s.child())
		}
		w.WriteString("}\n")
		if x.Default == nil {
			w.WriteString("panic(\"unreachable\")\n")
		}
	case ir.MatchBool:
		f.twoWay(w, x, s, subj, func(tag string) string {
			if tag == "True" {
				return subj
			}
			return "!" + subj
		})
	case ir.MatchList:
		f.twoWay(w, x, s, subj, func(tag string) string {
			if tag == "nil" {
				return "len(" + subj + ") == 0"
			}
			return "len(" + subj + ") > 0"
		})
	case ir.MatchLit:
		fmt.Fprintf(w, "switch %s {\n", cond(subj))
		for _, c := range x.Cases {
			fmt.Fprintf(w, "case %s:\n", litTag(x.Subject.Type(), c.Tag))
			f.caseBody(w, x, c, subj, s.child())
		}
		w.WriteString("default:\n")
		f.ret(w, x.Default, s.child())
		w.WriteString("}\n")
	}
}

// twoWay emits a match over a two-tag kind: an `if` for the first case, then the second case (the
// tags are then covered) or the default.
func (f *fn) twoWay(w *strings.Builder, x *ir.Match, s *scope, subj string, test func(tag string) string) {
	if len(x.Cases) == 0 {
		f.ret(w, x.Default, s)
		return
	}
	fmt.Fprintf(w, "if %s {\n", cond(test(x.Cases[0].Tag)))
	f.caseBody(w, x, x.Cases[0], subj, s.child())
	w.WriteString("}\n")
	switch {
	case len(x.Cases) > 1:
		f.caseBody(w, x, x.Cases[1], subj, s)
	case x.Default != nil:
		f.ret(w, x.Default, s)
	default:
		w.WriteString("panic(\"unreachable\")\n")
	}
}

// litTag is a literal match tag as a Go constant of the subject's type.
func litTag(t types.Type, tag string) string {
	switch GoType(t) {
	case "string":
		return strconv.Quote(tag)
	case "float64":
		if v, err := strconv.ParseFloat(tag, 64); err == nil {
			return floatLit(v)
		}
	}
	return tag
}

// caseBody binds the case's parts and returns its body.
func (f *fn) caseBody(w *strings.Builder, x *ir.Match, c ir.Case, subj string, s *scope) {
	access := map[string]string{}
	for _, b := range c.Binds {
		access[b.Name] = partAccess(x, c.Tag, b.Field, subj)
	}
	// The body starts with Lower's renamings `let user = part`: a part used only by its renaming
	// is bound to the user name directly.
	type rename struct{ user, from string }
	var renames []rename
	folded := map[string]bool{}
	body := c.Body
	for {
		l, ok := body.(*ir.Let)
		if !ok {
			break
		}
		v, ok := l.Value.(*ir.Var)
		if !ok || v.Kind != ir.VarLocal || uses(x.Subject, l.Name) > 0 {
			break
		}
		if access[v.Name] != "" && uses(c.Body, v.Name) == 1 {
			folded[v.Name] = true
		}
		if l.Name != "_" && uses(l.Body, l.Name) > 0 {
			renames = append(renames, rename{l.Name, v.Name})
		}
		body = l.Body
	}
	for _, b := range c.Binds {
		if !folded[b.Name] && uses(c.Body, b.Name) > 0 {
			fmt.Fprintf(w, "%s := %s\n", s.declare(b.Name), access[b.Name])
		}
	}
	for _, r := range renames {
		from := s.lookup(r.from)
		if folded[r.from] {
			from = access[r.from]
		}
		fmt.Fprintf(w, "%s := %s\n", s.declare(r.user), from)
	}
	f.ret(w, body, s)
}

// partAccess is the Go expression for part field of the value subj matched by tag.
func partAccess(x *ir.Match, tag, field, subj string) string {
	switch x.Kind {
	case ir.MatchList:
		if field == "head" {
			return subj + "[0]"
		}
		return subj + "[1:]"
	case ir.MatchUnion:
		if u, ok := x.Subject.Type().(*types.Union); ok && u.Module == "Prelude" {
			if tag == "Err" {
				return subj + ".Err"
			}
			return subj + ".Val"
		}
		if field == "" {
			return subj + "." + ctorField(tag)
		}
		return subj + "." + ctorField(tag) + "." + Field(field)
	}
	panic(fmt.Sprintf("gen: a %v match binds no parts", x.Kind))
}

func hasBinds(x *ir.Match) bool {
	for _, c := range x.Cases {
		if len(c.Binds) > 0 {
			return true
		}
	}
	return false
}

// ---- walking the IR

// uses counts the free occurrences of local name in e.
func uses(e ir.Expr, name string) int {
	switch x := e.(type) {
	case nil:
		return 0
	case *ir.Var:
		if x.Kind == ir.VarLocal && x.Name == name {
			return 1
		}
		return 0
	case *ir.Let:
		n := uses(x.Value, name)
		if x.Name != name {
			n += uses(x.Body, name)
		}
		return n
	case *ir.Lambda:
		for _, p := range x.Params {
			if p.Name == name {
				return 0
			}
		}
		return uses(x.Body, name)
	case *ir.Match:
		return usesMatch(x, name)
	case *ir.HMatch:
		return usesMatch((*ir.Match)(x), name)
	case *ir.HFor:
		n := uses(x.List, name)
		if x.Var != name {
			n += uses(x.Body, name)
		}
		return n
	}
	n := 0
	for _, c := range children(e) {
		n += uses(c, name)
	}
	return n
}

func usesMatch(x *ir.Match, name string) int {
	n := uses(x.Subject, name) + uses(x.Default, name)
cases:
	for _, c := range x.Cases {
		for _, b := range c.Binds {
			if b.Name == name {
				continue cases
			}
		}
		n += uses(c.Body, name)
	}
	return n
}

// children lists the direct subexpressions of e (nil entries possible).
func children(e ir.Expr) []ir.Expr {
	switch x := e.(type) {
	case *ir.Call:
		return append([]ir.Expr{x.Fn}, x.Args...)
	case *ir.Partial:
		return append([]ir.Expr{x.Fn}, x.Given...)
	case *ir.Lambda:
		return []ir.Expr{x.Body}
	case *ir.Record:
		return fieldValues(x.Fields)
	case *ir.Update:
		return append([]ir.Expr{x.Base}, fieldValues(x.Fields)...)
	case *ir.Field:
		return []ir.Expr{x.Expr}
	case *ir.Ctor:
		return []ir.Expr{x.Payload}
	case *ir.List:
		return x.Elems
	case *ir.PageRef:
		return []ir.Expr{x.Params}
	case *ir.Match:
		return matchChildren(x)
	case *ir.HMatch:
		return matchChildren((*ir.Match)(x))
	case *ir.If:
		return []ir.Expr{x.Cond, x.Then, x.Else}
	case *ir.Let:
		return []ir.Expr{x.Value, x.Body}
	case *ir.Op:
		return []ir.Expr{x.L, x.R}
	case *ir.Str:
		return x.Parts
	case *ir.Elem:
		return append(attrValues(x.Attrs), x.Children...)
	case *ir.Comp:
		out := x.Children
		if x.Props != nil {
			out = append([]ir.Expr{x.Props}, out...)
		}
		return out
	case *ir.HExpr:
		return []ir.Expr{x.Expr}
	case *ir.HFor:
		return []ir.Expr{x.List, x.Body}
	case *ir.HIf:
		return []ir.Expr{x.Cond, x.Then, x.Else}
	case *ir.Input:
		return []ir.Expr{x.LabelExpr, x.Value}
	case *ir.FormEl:
		return append(attrValues(x.Attrs), x.Children...)
	}
	return nil // Lit, Var, Hole, HText
}

func matchChildren(x *ir.Match) []ir.Expr {
	out := []ir.Expr{x.Subject}
	for _, c := range x.Cases {
		out = append(out, c.Body)
	}
	return append(out, x.Default)
}

func fieldValues(fs []ir.FieldInit) []ir.Expr {
	var out []ir.Expr
	for _, f := range fs {
		out = append(out, f.Value)
	}
	return out
}

func attrValues(as []ir.HAttr) []ir.Expr {
	var out []ir.Expr
	for _, a := range as {
		out = append(out, a.Value)
	}
	return out
}

// walk calls visit on e and every expression below it.
func walk(e ir.Expr, visit func(ir.Expr)) {
	if e == nil {
		return
	}
	if p, ok := e.(*ir.Record); ok && p == nil {
		return
	}
	visit(e)
	for _, c := range children(e) {
		walk(c, visit)
	}
}
