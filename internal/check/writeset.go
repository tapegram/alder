// The one-store-per-Task analysis (M15, E0031; book 08 "Silos", 13): a Task writes to the
// shared store or to one silo, never both and never two silos, since one transaction runs on one
// database. Reads may mix (a read of the other store runs outside the transaction).
//
// The analysis is syntactic, over the checked program, after every body is checked:
//
//  1. Every top-level value of every module (and every entry of a `fake`) gets its direct
//     writes — the tables of the Db writes written in its body, `Db.insert`, `Db.update`,
//     `Db.delete` and the scoped ones (`Db.insertMine`, `Db.updateScoped`, …), each applied to
//     its table (a table is always written in place: E0003 refuses `Table t`) — and the
//     top-level values it refers to, in any module.
//  2. A value's writes are its direct writes and, transitively, those of every value it refers
//     to (a fixed point, so recursion and cycles are fine). A reference counts whether or not
//     the value is called, and a whole body counts, lambdas and `Task.all`/`List.map`
//     arguments included: the analysis over-approximates, never under.
//  3. Each `update` (a page's, App's) is split into arms when its body is a `match` on its
//     message parameter (one arm per constructor; a `_` or variable arm stands for the others;
//     arms of one constructor are merged), else it is one arm. An arm's writes are those of its
//     body, as in 2. Each table written is in the store of its `silo` (siloOf), else shared. An
//     arm whose tables are in two stores is E0031, naming the arm, the tables and their stores;
//     otherwise its store is recorded (Result.Arms) for Lower's Arm.Store: the silo's, or shared
//     (also for an arm that writes nothing).
//  4. A fake's entry is checked the same way (as one arm).
//
// Session.put and capability calls are not writes to either store (the cookie; the adapter).
package check

import (
	"sort"
	"strings"

	"alder/internal/diag"
	"alder/internal/syntax"
)

// ArmStore is one arm of an `update` and the store its Task writes (Result.Arms): Msg is the
// constructor it matches ("" for the others, or for an update that is not a match on its
// message), Silo the silo key of the store ("" for the shared store), Span the arm's pattern
// (the update's declaration for Msg "").
type ArmStore struct {
	Msg  string
	Silo string
	Span diag.Span
}

// dbWrites are the Db functions that write, by name.
var dbWrites = map[string]bool{"insert": true, "update": true, "delete": true,
	"insertMine": true, "updateMine": true, "deleteMine": true,
	"insertScoped": true, "updateScoped": true, "deleteScoped": true}

// valueFacts are a body's direct writes (table keys) and the top-level values it refers to.
type valueFacts struct {
	writes map[string]bool
	refs   map[string]bool
}

// bodyFacts walks e for its direct writes and references.
func bodyFacts(e syntax.Expr) valueFacts {
	f := valueFacts{writes: map[string]bool{}, refs: map[string]bool{}}
	walkExpr(e, func(x syntax.Expr) {
		switch x := x.(type) {
		case *syntax.App:
			v, ok := x.Fn.(*syntax.Var)
			if !ok || v.Ref.Kind != syntax.RefStd || v.Ref.Module != "Db" || !dbWrites[v.Ref.Name] || len(x.Args) == 0 {
				return
			}
			if ct, ok := x.Args[0].(*syntax.CtorRef); ok {
				switch {
				case ct.Ref.Kind == syntax.RefTable:
					f.writes[ct.Ref.Module+"."+ct.Ref.Name] = true
				case ct.Table.Kind == syntax.RefTable:
					f.writes[ct.Table.Module+"."+ct.Table.Name] = true
				}
			}
		case *syntax.Var:
			if x.Ref.Kind == syntax.RefValue {
				f.refs[x.Ref.Module+"."+x.Ref.Name] = true
			}
		}
	})
	return f
}

// checkWriteSets runs the analysis (see the file comment) and records Result.Arms.
func (c *Checker) checkWriteSets() {
	direct := map[string]valueFacts{}
	type fakeEntry struct {
		fake string
		d    *syntax.ValueDecl
	}
	var fakes []fakeEntry
	for _, name := range c.prog.Order {
		m := c.prog.Modules[name]
		for _, d := range c.live(m) {
			switch d := d.(type) {
			case *syntax.ValueDecl:
				if d.Body != nil {
					direct[c.key(name, d.Name)] = bodyFacts(d.Body)
				}
			case *syntax.FakeDecl:
				if c.fakes[d.Ref.Name] != d {
					continue
				}
				for _, e := range d.Entries {
					if e.Body != nil && !c.losers[e] {
						fakes = append(fakes, fakeEntry{d.Name, e})
					}
				}
			}
		}
	}
	// closure: every value's writes through the values it refers to
	all := map[string]map[string]bool{}
	for k, f := range direct {
		all[k] = map[string]bool{}
		for t := range f.writes {
			all[k][t] = true
		}
	}
	for changed := true; changed; {
		changed = false
		for k, f := range direct {
			for r := range f.refs {
				for t := range all[r] {
					if !all[k][t] {
						all[k][t] = true
						changed = true
					}
				}
			}
		}
	}
	writesOf := func(e syntax.Expr) map[string]bool {
		f := bodyFacts(e)
		out := f.writes
		for r := range f.refs {
			for t := range all[r] {
				out[t] = true
			}
		}
		return out
	}
	c.res.Arms = map[string][]ArmStore{}
	for _, name := range c.prog.Order {
		if c.prog.Failed[name] {
			continue
		}
		if c.pages[name] == nil && name != "App" {
			continue
		}
		for _, d := range c.live(c.prog.Modules[name]) {
			vd, ok := d.(*syntax.ValueDecl)
			if !ok || vd.Name != "update" || vd.Body == nil {
				continue
			}
			c.res.Arms[name] = c.updateArms(vd, writesOf)
		}
	}
	for _, fe := range fakes {
		if groups := c.storeGroups(writesOf(fe.d.Body)); len(groups) > 1 {
			c.errorf(diag.WriteStores, fe.d.Span, "in `fake %s`'s `%s`: %s", fe.fake, fe.d.Name, c.mixMessage("`"+fe.d.Name+"`", groups))
		}
	}
}

// updateArms splits update vd into its arms (see the file comment), reports E0031 for an arm
// whose writes are in two stores, and returns each arm's store.
func (c *Checker) updateArms(vd *syntax.ValueDecl, writesOf func(syntax.Expr) map[string]bool) []ArmStore {
	type arm struct {
		msg    string
		span   diag.Span
		writes map[string]bool
	}
	var arms []*arm
	byMsg := map[string]*arm{}
	if m, ok := vd.Body.(*syntax.Match); ok && len(vd.Params) > 0 && isLocal(m.Subject, vd.Params[0].Name) {
		for _, a := range m.Arms {
			msg := ""
			if p, ok := a.Pat.(*syntax.PCtor); ok {
				msg = p.Name
			}
			ar := byMsg[msg]
			if ar == nil {
				ar = &arm{msg: msg, span: a.Pat.GetSpan(), writes: map[string]bool{}}
				byMsg[msg] = ar
				arms = append(arms, ar)
			}
			for t := range writesOf(a.Body) {
				ar.writes[t] = true
			}
		}
	} else {
		arms = append(arms, &arm{span: vd.Span, writes: writesOf(vd.Body)})
	}
	out := make([]ArmStore, 0, len(arms))
	for _, a := range arms {
		groups := c.storeGroups(a.writes)
		st := ArmStore{Msg: a.msg, Span: a.span}
		switch {
		case len(groups) > 1:
			name := "`" + a.msg + "`"
			switch {
			case a.msg == "" && len(arms) == 1:
				name = "`update`"
			case a.msg == "":
				name = "the `_` arm"
			}
			c.errorf(diag.WriteStores, a.span, "%s", c.mixMessage(name, groups))
		case len(groups) == 1:
			st.Silo = groups[0].silo
		}
		out = append(out, st)
	}
	return out
}

func isLocal(e syntax.Expr, name string) bool {
	v, ok := e.(*syntax.Var)
	return ok && v.Ref.Kind == syntax.RefLocal && v.Name == name && v.Module == ""
}

// storeGroup is the tables (bare names, sorted) an arm writes in one store (silo "" is shared).
type storeGroup struct {
	silo   string
	tables []string
}

// storeGroups groups the table keys written by store: silos by key, then the shared store.
func (c *Checker) storeGroups(writes map[string]bool) []storeGroup {
	by := map[string][]string{}
	for t := range writes {
		s := c.siloOf[t]
		by[s] = append(by[s], strings.TrimPrefix(t, tableModule(t)+"."))
	}
	keys := make([]string, 0, len(by))
	for k := range by {
		if k != "" {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	if _, ok := by[""]; ok {
		keys = append(keys, "")
	}
	out := make([]storeGroup, 0, len(keys))
	for _, k := range keys {
		ts := by[k]
		sort.Strings(ts)
		out = append(out, storeGroup{k, ts})
	}
	return out
}

// mixMessage is E0031's text for arm name writing groups (two or more): "`Create` writes
// `Project` (silo `org`) and `AuditLog` (shared): a Task writes to one store. …".
func (c *Checker) mixMessage(name string, groups []storeGroup) string {
	parts := make([]string, len(groups))
	shared := ""
	for i, g := range groups {
		ts := make([]string, len(g.tables))
		for j, t := range g.tables {
			ts[j] = "`" + t + "`"
		}
		where := "(shared)"
		if g.silo != "" {
			where = "(silo `" + g.silo + "`)"
		} else {
			shared = andList(ts)
		}
		parts[i] = andList(ts) + " " + where
	}
	hint := "Send one of the writes from its own message."
	if shared != "" {
		hint = "Write " + shared + " inside the silo, or send it from its own message."
	}
	return name + " writes " + andList(parts) + ": a Task writes to one store. " + hint
}

// andList joins items as "a", "a and b", "a, b and c".
func andList(items []string) string {
	switch len(items) {
	case 0:
		return ""
	case 1:
		return items[0]
	}
	return strings.Join(items[:len(items)-1], ", ") + " and " + items[len(items)-1]
}

// walkExpr calls f on e and every expression under it (lambdas, blocks, arms and HTML
// included), parents first.
func walkExpr(e syntax.Expr, f func(syntax.Expr)) {
	if e == nil {
		return
	}
	f(e)
	w := func(x syntax.Expr) { walkExpr(x, f) }
	switch x := e.(type) {
	case *syntax.StrLit:
		for _, p := range x.Parts {
			if p.Expr != nil {
				w(p.Expr)
			}
		}
	case *syntax.ListLit:
		for _, el := range x.Elems {
			w(el)
		}
	case *syntax.RecordLit:
		for _, fi := range x.Fields {
			w(fi.Value)
		}
	case *syntax.RecordUpdate:
		w(x.Base)
		for _, fi := range x.Fields {
			w(fi.Value)
		}
	case *syntax.Access:
		w(x.Expr)
	case *syntax.App:
		w(x.Fn)
		for _, a := range x.Args {
			w(a)
		}
	case *syntax.Lambda:
		w(x.Body)
	case *syntax.If:
		w(x.Cond)
		w(x.Then)
		w(x.Else)
	case *syntax.Match:
		w(x.Subject)
		for _, a := range x.Arms {
			w(a.Body)
		}
	case *syntax.Block:
		for _, s := range x.Stmts {
			w(s.Expr)
		}
		w(x.Result)
	case *syntax.BinOp:
		w(x.L)
		w(x.R)
	case *syntax.Element:
		for _, a := range x.Attrs {
			if a.Value != nil {
				w(a.Value)
			}
		}
		for _, ch := range x.Children {
			w(ch)
		}
	case *syntax.HtmlExpr:
		w(x.Expr)
	case *syntax.HtmlFor:
		w(x.List)
		w(x.Body)
	case *syntax.HtmlIf:
		w(x.Cond)
		w(x.Then)
		w(x.Else)
	case *syntax.HtmlMatch:
		w(x.Subject)
		for _, a := range x.Arms {
			w(a.Body)
		}
	}
}
