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
//	union of the app  switch s.Tag { case is__M__T__C: … }, panic("unreachable") after it when
//	                  there is no default
//	Maybe, Result     if s.Ok { … } / if s.IsOk { … } then the other case or the default
//	Nav               switch s.Tag { case rt.NavRedirect: … } (payloads s.Page, s.Text)
//	Answer            switch s.Tag { case rtesting.Redirected: … } (payloads s.Page,
//	                  s.FieldErrors(), s.Text; test builds only)
//	Bool              if s { … } (or if !s) then the other case or the default
//	List              if len(s) == 0 { … } (or len(s) > 0) then the other case or the default
//	literal           switch s { case 1: … default: … }
//
// A case binds the parts it needs to Go variables (`_m1 := s.Circle.Value`, `_m2 := s[1:]`)
// before its body. When the body starts by renaming such a part to a user name (Lower's
// `let r = _m1`, the only use of _m1), the part is bound to the user name directly
// (`r := s.Circle.Value`).

// k renders each returned value (see retK; nil is f.code).
func (f *fn) match(w *strings.Builder, x *ir.Match, s *scope, k func(ir.Expr, *scope) string) {
	subj, p := f.expr(x.Subject, s)
	if !isPath(x.Subject) && hasBinds(x) { // binds would re-evaluate the subject
		t := f.temp(s, "_s")
		fmt.Fprintf(w, "%s := %s\n", t, subj)
		subj, p = t, precAtom
	}
	if p < precAtom {
		subj = "(" + subj + ")"
	}
	switch x.Kind {
	case ir.MatchUnion:
		u, _ := x.Subject.Type().(*types.Union)
		if u != nil && u.Module == "Prelude" && (u.Name == "Nav" || u.Name == "Answer") {
			fmt.Fprintf(w, "switch %s.Tag {\n", subj)
			for _, c := range x.Cases {
				if u.Name == "Answer" {
					fmt.Fprintf(w, "case rtesting.%s:\n", c.Tag)
				} else {
					fmt.Fprintf(w, "case rt.Nav%s:\n", c.Tag)
				}
				f.caseBody(w, x, c, subj, s.child(), k)
			}
			if x.Default != nil {
				w.WriteString("default:\n")
				f.retK(w, x.Default, s.child(), k)
			}
			w.WriteString("}\n")
			if x.Default == nil {
				w.WriteString("panic(\"unreachable\")\n")
			}
			return
		}
		if u != nil && u.Module == "Prelude" {
			field := "Ok"
			if u.Name == "Result" {
				field = "IsOk"
			}
			f.twoWay(w, x, s, k, subj, func(tag string) string {
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
			f.caseBody(w, x, c, subj, s.child(), k)
		}
		if x.Default != nil {
			w.WriteString("default:\n")
			f.retK(w, x.Default, s.child(), k)
		}
		w.WriteString("}\n")
		if x.Default == nil {
			w.WriteString("panic(\"unreachable\")\n")
		}
	case ir.MatchBool:
		f.twoWay(w, x, s, k, subj, func(tag string) string {
			if tag == "True" {
				return subj
			}
			return "!" + subj
		})
	case ir.MatchList:
		f.twoWay(w, x, s, k, subj, func(tag string) string {
			if tag == "nil" {
				return "len(" + subj + ") == 0"
			}
			return "len(" + subj + ") > 0"
		})
	case ir.MatchLit:
		fmt.Fprintf(w, "switch %s {\n", cond(subj))
		for _, c := range x.Cases {
			fmt.Fprintf(w, "case %s:\n", litTag(x.Subject.Type(), c.Tag))
			f.caseBody(w, x, c, subj, s.child(), k)
		}
		w.WriteString("default:\n")
		f.retK(w, x.Default, s.child(), k)
		w.WriteString("}\n")
	}
}

// twoWay emits a match over a two-tag kind: an `if` for the first case, then the second case (the
// tags are then covered) or the default.
func (f *fn) twoWay(w *strings.Builder, x *ir.Match, s *scope, k func(ir.Expr, *scope) string, subj string, test func(tag string) string) {
	if len(x.Cases) == 0 {
		f.retK(w, x.Default, s, k)
		return
	}
	fmt.Fprintf(w, "if %s {\n", cond(test(x.Cases[0].Tag)))
	f.caseBody(w, x, x.Cases[0], subj, s.child(), k)
	w.WriteString("}\n")
	switch {
	case len(x.Cases) > 1:
		f.caseBody(w, x, x.Cases[1], subj, s, k)
	case x.Default != nil:
		f.retK(w, x.Default, s, k)
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
func (f *fn) caseBody(w *strings.Builder, x *ir.Match, c ir.Case, subj string, s *scope, k func(ir.Expr, *scope) string) {
	access := map[string]string{}
	for _, b := range c.Binds {
		access[b.Name] = f.partAccess(x, c.Tag, b.Field, subj)
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
	f.retK(w, body, s, k)
}

// partAccess is the Go expression for part field of the value subj matched by tag.
func (f *fn) partAccess(x *ir.Match, tag, field, subj string) string {
	switch x.Kind {
	case ir.MatchList:
		if field == "head" {
			return subj + "[0]"
		}
		return subj + "[1:]"
	case ir.MatchUnion:
		if u, ok := x.Subject.Type().(*types.Union); ok && u.Module == "Prelude" {
			switch tag {
			case "Redirect", "Redirected":
				return subj + ".Page"
			case "Reject", "Failed":
				return subj + ".Text"
			case "Rejected":
				return subj + ".FieldErrors()"
			}
			if tag == "Err" {
				return subj + ".Err"
			}
			return subj + ".Val"
		}
		if field == "" { // the whole payload record: the union holds it by pointer
			return "*" + subj + "." + ctorField(tag)
		}
		v := subj + "." + ctorField(tag) + "." + Field(field)
		if u, ok := x.Subject.Type().(*types.Union); ok {
			if p := f.g.payloads[u.Module+"."+u.Name+"."+tag]; p != nil && f.g.boxed.has(p.Module, p.Name, field) {
				return "(*" + v + ")"
			}
		}
		return v
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
