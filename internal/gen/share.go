package gen

import (
	"fmt"
	"strings"

	"alder/internal/ir"
)

// Shared subtrees of a decision tree.
//
// Lower compiles a match to a tree whose fallback nodes are shared: a refutable arm that fails
// at any of its tests continues with the same node (the rest of the arms), so one node can be
// the default of several nested Matches (lower_match.go). Emitted at each place, a tree whose
// shared nodes nest — each arm's fallback holds the next arm's twice — doubles per arm. Instead,
// a shared compound node (a Match, If or Let) is emitted once, as a local closure declared at
// the innermost Match enclosing every place it is used (its anchor), and each place returns its
// call:
//
//	_fb3 := func() int64 {          the arms after the first, once
//		if r.F1.Ok { … }
//		return rt.Int(0) - 1
//	}
//	if r.F0.Ok {
//		…
//		if r.F1.Ok { … }
//		return _fb3()
//	}
//	return _fb3()
//
// A shared node is declared only where every name it uses is bound: a node that uses a name
// bound between the anchor and one of its places (a Case's part, or a `let` — a named catch-all
// arm's body is shared by every Case, each binding the name with its own Let) is emitted at
// each place instead, as is everything below such a node (analyseShared). A node used at one
// place, or a leaf (a variable, a call, …), is emitted where it is, as before.

// sharing is a fn's shared-node state.
type sharing struct {
	// analysed are the Matches whose tree has been analysed (analyseShared)
	analysed map[*ir.Match]bool
	// anchors are the shared nodes to declare in front of each Match, innermost first
	anchors map[*ir.Match][]ir.Expr
	// names are the closures declared for the shared nodes in the Match being emitted
	names map[ir.Expr]string
}

// treeChildren are the children of e that retK emits as statements (the edges a decision tree
// shares nodes along): a match's case bodies and default, a let's body, an if's branches.
func treeChildren(e ir.Expr) []ir.Expr {
	switch x := e.(type) {
	case *ir.Match:
		return matchBodies(x)
	case *ir.HMatch:
		return matchBodies((*ir.Match)(x))
	case *ir.Let:
		return []ir.Expr{x.Body}
	case *ir.If:
		return []ir.Expr{x.Then, x.Else}
	case *ir.HIf:
		return []ir.Expr{x.Then, x.Else}
	}
	return nil
}

func matchBodies(x *ir.Match) []ir.Expr {
	var out []ir.Expr
	for _, c := range x.Cases {
		out = append(out, c.Body)
	}
	if x.Default != nil {
		out = append(out, x.Default)
	}
	return out
}

// asMatch is e as the *ir.Match retK emits it with (an HMatch is one), or nil.
func asMatch(e ir.Expr) *ir.Match {
	switch x := e.(type) {
	case *ir.Match:
		return x
	case *ir.HMatch:
		return (*ir.Match)(x)
	}
	return nil
}

// step is one level of the path from a tree's root to a node: inside Match node (case -1), or
// in its case or default case (case i, the default being len(Cases)), or in Let node's body.
type step struct {
	node ir.Expr
	cas  int
}

// bound are the names step st binds for what is below it: a Case's parts, a Let's name.
func (st step) bound() []string {
	switch x := st.node.(type) {
	case *ir.Let:
		return []string{x.Name}
	case *ir.Match, *ir.HMatch:
		if m := asMatch(x); st.cas >= 0 && st.cas < len(m.Cases) {
			var out []string
			for _, b := range m.Cases[st.cas].Binds {
				out = append(out, b.Name)
			}
			return out
		}
	}
	return nil
}

// analyseShared finds the shared compound nodes of the tree rooted at Match root and the anchor
// of each. It walks the tree once (a shared node's subtree at its first use), recording every
// use of every node; then, parents before children, a node's path is the steps enclosing it
// where it is emitted — for a shared node, the longest path common to its uses, whose last
// Match is its anchor. A shared node is declared there only when no step between the anchor and
// one of its uses binds a name it uses (a named catch-all arm's body sits under its own
// `let name = subject` in every Case): otherwise, or below a shared node that is not declared,
// it is emitted at each place.
func (f *fn) analyseShared(root *ir.Match) {
	sh := &f.share
	if sh.analysed == nil {
		sh.analysed, sh.anchors, sh.names = map[*ir.Match]bool{}, map[*ir.Match][]ir.Expr{}, map[ir.Expr]string{}
	}
	parents := map[ir.Expr][]ir.Expr{}
	var post []ir.Expr // children before parents
	seen := map[ir.Expr]bool{}
	var walk func(e ir.Expr)
	walk = func(e ir.Expr) {
		seen[e] = true
		if m := asMatch(e); m != nil {
			sh.analysed[m] = true
		}
		for _, c := range treeChildren(e) {
			parents[c] = append(parents[c], e)
			if !seen[c] {
				walk(c)
			}
		}
		post = append(post, e)
	}
	walk(root)
	path := map[ir.Expr][]step{}
	// lim is, below a node, the least index in a path where an anchor may be: below a shared
	// node emitted at each place, a node is declared inside that node, in each copy
	lim := map[ir.Expr]int{}
	// below are the paths of child's places under parent p (one per edge p → child)
	below := func(p, child ir.Expr) [][]step {
		pp := path[p][:len(path[p]):len(path[p])]
		switch x := p.(type) {
		case *ir.Let:
			return [][]step{append(pp, step{p, 0})}
		case *ir.Match, *ir.HMatch:
			m := asMatch(x)
			var out [][]step
			for i, b := range matchBodies(m) {
				if b == child {
					out = append(out, append(append(pp, step{p, -1}), step{p, i}))
				}
			}
			return out
		}
		return [][]step{pp}
	}
	var shared []ir.Expr // parents before children
	for i := len(post) - 1; i >= 0; i-- {
		e := post[i]
		var places [][]step
		done := map[ir.Expr]bool{}
		for _, p := range parents[e] {
			if done[p] { // below gives every edge p → e
				continue
			}
			done[p] = true
			places = append(places, below(p, e)...)
			lim[e] = max(lim[e], lim[p])
		}
		if len(places) == 0 { // the root
			continue
		}
		c := places[0]
		for _, u := range places[1:] {
			c = commonPrefix(c, u)
		}
		path[e] = c
		if len(places) < 2 || treeChildren(e) == nil {
			continue
		}
		a := -1 // the anchor's step in c
		for k, st := range c {
			if st.cas == -1 {
				a = k
			}
		}
		ok := a >= lim[e]
		for _, u := range places {
			for _, st := range u[a+1:] {
				for _, name := range st.bound() {
					ok = ok && uses(e, name) == 0
				}
			}
		}
		if !ok {
			lim[e] = len(c)
			continue
		}
		path[e] = c[:a+1]
		shared = append(shared, e)
	}
	for i := len(shared) - 1; i >= 0; i-- { // a node's shared descendants are declared first
		c := path[shared[i]]
		a := asMatch(c[len(c)-1].node)
		sh.anchors[a] = append(sh.anchors[a], shared[i])
	}
}

func commonPrefix(a, b []step) []step {
	n := 0
	for n < len(a) && n < len(b) && a[n] == b[n] {
		n++
	}
	return a[:n:n]
}

// declareShared declares, in front of Match x, a closure for each shared node anchored at x,
// which retK then emits as a call (dropShared ends that with x). k is retK's: the closure
// returns what k renders, an eff.Task for f.task (the only k but nil). A Match whose arms a
// mutant moves (f.jumps) declares none: its shared nodes are emitted at each place.
func (f *fn) declareShared(w *strings.Builder, x *ir.Match, s *scope, k func(ir.Expr, *scope) string) {
	if !f.share.analysed[x] {
		f.analyseShared(x)
	}
	if f.jumps[x] != nil {
		return
	}
	for _, e := range f.share.anchors[x] {
		typ := GoType(e.Type())
		if k != nil {
			typ = "eff.Task[" + GoType(effResult(e.Type())) + "]"
		}
		name := f.temp(s, "_fb")
		var b strings.Builder
		f.retK(&b, e, s.child(), k)
		fmt.Fprintf(w, "%s := func() %s {\n%s}\n", name, typ, b.String())
		f.share.names[e] = name
	}
}

// dropShared ends the closures declareShared declared for x: past x, its shared nodes are
// emitted where they are.
func (f *fn) dropShared(x *ir.Match) {
	for _, e := range f.share.anchors[x] {
		delete(f.share.names, e)
	}
}
