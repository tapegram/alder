package gen

import "alder/internal/ir"

// Walking the IR: free-variable counts and subexpressions, over every node kind (HTML included).

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
