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
// Every name a shared node uses is bound above its anchor: the parts a Case binds are visible
// to the nodes compiled inside that Case only, so a node used both inside and outside a Case
// uses none of them (and a user's `let` never encloses a fallback). A node used at one place, or
// a leaf (a variable, a call, …), is emitted where it is, as before.

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

// analyseShared finds the shared compound nodes of the tree rooted at Match root and the anchor
// of each: walking the tree once (a shared node's subtree is walked at its first use), it
// records every use of every node; then, parents before children, a node's chain is the Matches
// enclosing it where it is emitted — for a shared node, the longest chain common to its uses,
// whose last Match is its anchor.
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
	chain := map[ir.Expr][]*ir.Match{}
	inner := func(p ir.Expr) []*ir.Match { // the chain p's children are emitted in
		if m := asMatch(p); m != nil {
			return append(chain[p][:len(chain[p]):len(chain[p])], m)
		}
		return chain[p]
	}
	var shared []ir.Expr // parents before children
	for i := len(post) - 1; i >= 0; i-- {
		e := post[i]
		ps := parents[e]
		if len(ps) == 0 { // the root
			continue
		}
		c := inner(ps[0])
		for _, p := range ps[1:] {
			c = commonPrefix(c, inner(p))
		}
		chain[e] = c
		if len(ps) > 1 && treeChildren(e) != nil {
			shared = append(shared, e)
		}
	}
	for i := len(shared) - 1; i >= 0; i-- { // a node's shared descendants are declared first
		a := chain[shared[i]][len(chain[shared[i]])-1]
		sh.anchors[a] = append(sh.anchors[a], shared[i])
	}
}

func commonPrefix(a, b []*ir.Match) []*ir.Match {
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
