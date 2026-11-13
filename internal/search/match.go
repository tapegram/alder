package search

import (
	"sort"

	"alder/internal/types"
)

// typeHit is a candidate that matched a type query, with its ranking keys.
type typeHit struct {
	e         *Entry
	bound     int  // the candidate's own variables bound to something other than a variable
	args      int  // produces mode: leading arguments still to supply
	reordered bool // type mode: matched with its parameters permuted
	bare      bool // the declared result is a bare type variable (produces mode) or Never (either mode)
}

// searchType answers a type query: q is the query's type and u the unifier holding its
// wildcards. produces is the `:` form. u is never written: every attempt runs on a clone.
func (ix *Index) searchType(q types.Type, u *types.Unifier, produces bool, source string) []Hit {
	qf, qFun := q.(*types.Fun)
	m := 0
	if qFun {
		m = len(qf.Params)
	}
	var found []typeHit
	for i := range ix.Entries {
		e := &ix.Entries[i]
		if e.Kind == TypeDecl || e.Type == nil || (source != "" && e.Source != source) {
			continue
		}
		cf, cFun := e.Type.(*types.Fun)
		var h typeHit
		var ok bool
		switch {
		case produces && cFun:
			if len(cf.Params) < m {
				continue
			}
			h.args = len(cf.Params) - m
			h.bound, ok = attempt(u, e.Type, q, func(t types.Type) types.Type {
				f := t.(*types.Fun)
				if m == 0 {
					return f.Result
				}
				return &types.Fun{Params: f.Params[h.args:], Result: f.Result}
			})
			_, h.bare = cf.Result.(*types.Param)
			h.bare = h.bare || isNever(cf.Result)
		case produces:
			h.bound, ok = attempt(u, e.Type, q, nil)
			_, h.bare = e.Type.(*types.Param)
			h.bare = h.bare || isNever(e.Type)
		case !qFun: // a non-function type query is answered by non-functions only
			if cFun {
				continue
			}
			h.bound, ok = attempt(u, e.Type, q, nil)
			h.bare = isNever(e.Type)
		default: // type mode, function query
			if !cFun || len(cf.Params) != m {
				continue
			}
			h.bare = isNever(cf.Result)
			h.bound, ok = attempt(u, e.Type, q, nil)
			if !ok {
				for _, p := range perms[m] {
					if h.bound, ok = attempt(u, e.Type, q, permuted(p)); ok {
						h.reordered = true
						break
					}
				}
			}
		}
		if ok {
			h.e = e
			found = append(found, h)
		}
	}
	sort.SliceStable(found, func(i, j int) bool {
		a, b := found[i], found[j]
		switch {
		case a.bare != b.bare:
			return b.bare
		case a.bound != b.bound:
			return a.bound < b.bound
		case a.args != b.args:
			return a.args < b.args
		case a.reordered != b.reordered:
			return b.reordered
		}
		if sa, sb := sourceRank(a.e.Source), sourceRank(b.e.Source); sa != sb {
			return sa < sb
		}
		return a.e.Module+"."+a.e.Name < b.e.Module+"."+b.e.Name
	})
	hits := make([]Hit, len(found))
	for i, h := range found {
		match := "general"
		switch {
		case h.reordered:
			match = "reordered"
		case h.bound == 0:
			match = "exact"
		}
		hits[i] = Hit{Entry: h.e, Match: match, Args: h.args}
	}
	return hits
}

// isNever reports whether t is Never, which Check lets fit anywhere, so it matches every query.
func isNever(t types.Type) bool {
	c, ok := t.(*types.Con)
	return ok && c.Name == "Never"
}

// attempt checks the candidate type cand, instantiated and then shaped by shape (when not nil),
// against q on a clone of u. It reports whether it matched and, when it did, how many of the
// candidate's own variables are bound to something other than a variable.
func attempt(u *types.Unifier, cand, q types.Type, shape func(types.Type) types.Type) (int, bool) {
	c := u.Clone()
	lo := c.Fresh().ID // the instance's variables are numbered between lo and hi
	t := c.Instantiate(cand)
	hi := c.Fresh().ID
	if shape != nil {
		t = shape(t)
	}
	if c.Check(t, q) != nil {
		return 0, false
	}
	bound := 0
	for id := lo + 1; id < hi; id++ {
		switch c.Apply(&types.Var{ID: id}).(type) {
		case *types.Var, *types.Param:
		default:
			bound++
		}
	}
	return bound, true
}

// permuted is the shape that reorders an instantiated function's parameters by p.
func permuted(p []int) func(types.Type) types.Type {
	return func(t types.Type) types.Type {
		f := t.(*types.Fun)
		ps := make([]types.Type, len(p))
		for i, k := range p {
			ps[i] = f.Params[k]
		}
		return &types.Fun{Params: ps, Result: f.Result}
	}
}

// perms is the reorderings tried for a function of n parameters: for 2 to 4, every order but
// the declared one, in lexicographic order; none for any other n.
var perms = map[int][][]int{2: allPermutations(2), 3: allPermutations(3), 4: allPermutations(4)}

func allPermutations(n int) [][]int {
	var out [][]int
	var rec func(prefix []int, used []bool)
	rec = func(prefix []int, used []bool) {
		if len(prefix) == n {
			out = append(out, append([]int(nil), prefix...))
			return
		}
		for i := 0; i < n; i++ {
			if !used[i] {
				used[i] = true
				rec(append(prefix, i), used)
				used[i] = false
			}
		}
	}
	rec(nil, make([]bool, n))
	return out[1:] // the first is the declared order
}
