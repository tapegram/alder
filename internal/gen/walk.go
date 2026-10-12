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
	case *ir.EffBind:
		n := uses(x.Effect, name)
		if x.Param == nil || x.Param.Name != name {
			n += uses(x.Body, name)
		}
		return n
	}
	n := 0
	for _, c := range ir.Children(e) {
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

// walk calls visit on e and every expression below it.
func walk(e ir.Expr, visit func(ir.Expr)) {
	if e == nil {
		return
	}
	if p, ok := e.(*ir.Record); ok && p == nil {
		return
	}
	visit(e)
	for _, c := range ir.Children(e) {
		walk(c, visit)
	}
}
