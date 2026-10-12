package gen

import "alder/internal/ir"

// Walking the IR: free-variable counts and subexpressions, over every node kind (HTML included).

// uses counts the free occurrences of local name in e.
func uses(e ir.Expr, name string) int { return (&usesOf{name: name}).count(e) }

// usesOf counts the free occurrences of name. A decision tree shares nodes (share.go): a
// shared node's count is computed once and counted at each place (memo), so counting takes
// time linear in the tree's nodes, not in its paths.
type usesOf struct {
	name string
	memo map[ir.Expr]int
}

func (u *usesOf) count(e ir.Expr) int {
	if treeChildren(e) == nil {
		return u.node(e)
	}
	if n, ok := u.memo[e]; ok {
		return n
	}
	n := u.node(e)
	if u.memo == nil {
		u.memo = map[ir.Expr]int{}
	}
	u.memo[e] = n
	return n
}

func (u *usesOf) node(e ir.Expr) int {
	name := u.name
	switch x := e.(type) {
	case nil:
		return 0
	case *ir.Var:
		if x.Kind == ir.VarLocal && x.Name == name {
			return 1
		}
		return 0
	case *ir.Let:
		n := u.count(x.Value)
		if x.Name != name {
			n += u.count(x.Body)
		}
		return n
	case *ir.Lambda:
		for _, p := range x.Params {
			if p.Name == name {
				return 0
			}
		}
		return u.count(x.Body)
	case *ir.Match:
		return u.match(x)
	case *ir.HMatch:
		return u.match((*ir.Match)(x))
	case *ir.HFor:
		n := u.count(x.List)
		if x.Var != name {
			n += u.count(x.Body)
		}
		return n
	case *ir.EffBind:
		n := u.count(x.Effect)
		if x.Param == nil || x.Param.Name != name {
			n += u.count(x.Body)
		}
		return n
	}
	n := 0
	for _, c := range ir.Children(e) {
		n += u.count(c)
	}
	return n
}

func (u *usesOf) match(x *ir.Match) int {
	name := u.name
	n := u.count(x.Subject) + u.count(x.Default)
cases:
	for _, c := range x.Cases {
		for _, b := range c.Binds {
			if b.Name == name {
				continue cases
			}
		}
		n += u.count(c.Body)
	}
	return n
}

// walk calls visit on e and every expression below it.
func walk(e ir.Expr, visit func(ir.Expr)) { walkOnce(e, visit, map[ir.Expr]bool{}) }

// walkOnce is walk visiting a node a decision tree shares (share.go) once.
func walkOnce(e ir.Expr, visit func(ir.Expr), seen map[ir.Expr]bool) {
	if treeChildren(e) != nil {
		if seen[e] {
			return
		}
		seen[e] = true
	}
	if e == nil {
		return
	}
	if p, ok := e.(*ir.Record); ok && p == nil {
		return
	}
	visit(e)
	for _, c := range ir.Children(e) {
		walkOnce(c, visit, seen)
	}
}
