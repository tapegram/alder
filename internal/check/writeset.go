// The one-store-per-Task analysis (M15, E0031; book 08 "Silos", 13): a Task writes to the
// shared store or to one silo, never both and never two silos, since one transaction runs on one
// database. Reads may mix (a read of the other store runs outside the transaction).
//
// The analysis runs over the checked program, after every body is checked (it reads the
// instantiated types of the Db references, so it is by type, not by spelling):
//
//  1. Every top-level value of every module (and every entry of a `fake`) gets its direct
//     writes — the tables of the references in its body to a Db write function, `Db.insert`,
//     `Db.update`, `Db.delete` and the scoped ones (`Db.insertMine`, `Db.updateScoped`, …),
//     whether applied in place or not (`let ins = Db.insert`, a lambda's parameter applied to
//     `Db.insert`): each reference's table is read from its instantiated type (bodyFacts) — a
//     Db function always has a concrete table where it is used (E0003 refuses `Table t`), and a
//     `let` is not generalized, so one reference writes one table — and the top-level values
//     it refers to, in any module.
//  2. A value's writes are its direct writes and, transitively, those of every value it refers
//     to (a fixed point, so recursion and cycles are fine). A reference counts whether or not
//     the value is called, and a whole body counts, lambdas and `Task.all`/`List.map`
//     arguments included: the analysis over-approximates, never under.
//  3. Each write route's handler (`Toggle.patch`) is one arm, recorded under the route's module
//     with Msg the route's name. Its writes are those of its body, as in 2. Each table written
//     is in the store of its `silo` (siloOf), else shared. An update whose tables are in two
//     stores is E0031, naming it, the tables and their stores; otherwise its store is recorded
//     (Result.Arms) for Lower's Route.Store: the silo's, or shared (also for an update that
//     writes nothing).
//  4. A fake's entry is checked the same way (as one arm).
//  5. A job's `run` (#168) is one arm, checked the same way: writes in two stores are E0031.
//     Its store is its `silo k`'s, else shared: a `silo k` job that writes a shared table is
//     E0031 too (the worker binds it to its silo); a silo's table written from any other job is
//     E0026 (rules.go), where it is written.
//
// Session.put and capability calls are not writes to either store (the cookie; the adapter).
//
// What it proves: an `update` arm (and a fake's entry) that the checker accepts names no Db
// write function of a table of another store anywhere it can reach through its body and the
// top-level values it refers to. It does not see a `test`'s Tasks (Test.run of a Task built in a
// test is not an arm) nor code outside Alder (an adapter); there the runtime is the backstop: a
// request's Task is bound to its arm's store (db.Enter) and Test.run's to the first store it
// writes, and a write of another store fails the Task (an internal error: 500 in production, a
// failed test under `alder test`).
package check

import (
	"fmt"
	"slices"
	"sort"
	"strings"

	"alder/internal/diag"
	"alder/internal/syntax"
)

// ArmStore is a write route's `update` and the store its Task writes (Result.Arms): Msg is the
// route's name, Silo the silo key of the store ("" for the shared store), Span the update's
// declaration.
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

// bodyFacts walks e for its direct writes and references. A write is found by type, not by
// spelling: every reference to a Db write function (applied or not — `let ins = Db.insert`, a
// lambda that applies its parameter to `Db.insert`, `List.map (Db.deleteScoped Project)`) counts,
// with its table read from the reference's instantiated type (its `Table t` parameter), as
// checkScopedTables reads it. Lets are not generalized, so the reference's type is the one table
// it is used with; a reference whose table is not known is checkDbTables' E0003.
func (c *Checker) bodyFacts(e syntax.Expr) valueFacts {
	f := valueFacts{writes: map[string]bool{}, refs: map[string]bool{}}
	walkExpr(e, func(x syntax.Expr) {
		v, ok := x.(*syntax.Var)
		if !ok {
			return
		}
		switch {
		case v.Ref.Kind == syntax.RefValue:
			f.refs[v.Ref.Module+"."+v.Ref.Name] = true
		case v.Ref.Kind == syntax.RefStd && v.Ref.Module == "Db" && dbWrites[v.Ref.Name]:
			if rec := tableArg(c, c.res.Types[v]); rec != nil {
				f.writes[c.key(rec.Module, rec.Name)] = true
			}
		}
	})
	return f
}

// checkWriteSets runs the analysis (see the file comment) and records Result.Arms.
func (c *Checker) checkWriteSets() {
	direct := map[string]valueFacts{}
	type fakeEntry struct {
		fake, module string
		d            *syntax.ValueDecl
	}
	var fakes []fakeEntry
	for _, name := range c.prog.Order {
		m := c.prog.Modules[name]
		for _, d := range c.live(m) {
			switch d := d.(type) {
			case *syntax.ValueDecl:
				if d.Body != nil {
					direct[c.key(name, d.FullName())] = c.bodyFacts(d.Body)
				}
			case *syntax.FakeDecl:
				if c.fakes[d.Ref.Name] != d {
					continue
				}
				for _, e := range d.Entries {
					if e.Body != nil && !c.losers[e] {
						fakes = append(fakes, fakeEntry{d.Name, name, e})
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
	// writesOf maps each table body e writes to the helper ("Module.value") its write is first
	// reached through, "" for a write in e itself; of several helpers, the first by name
	writesOf := func(e syntax.Expr) map[string]string {
		f := c.bodyFacts(e)
		out := map[string]string{}
		for t := range f.writes {
			out[t] = ""
		}
		refs := make([]string, 0, len(f.refs))
		for r := range f.refs {
			refs = append(refs, r)
		}
		sort.Strings(refs)
		for _, r := range refs {
			for t := range all[r] {
				if _, ok := out[t]; !ok {
					out[t] = r
				}
			}
		}
		return out
	}
	c.res.Arms = map[string][]ArmStore{}
	for _, name := range c.prog.Order {
		if c.prog.Failed[name] {
			continue
		}
		for _, r := range c.routes[name] {
			vd := c.handler(r)
			if vd == nil || vd.Body == nil {
				continue
			}
			st := ArmStore{Msg: r.Name, Span: vd.Span}
			switch groups := c.storeGroups(writesOf(vd.Body)); {
			case len(groups) > 1:
				c.errorf(diag.WriteStores, vd.Span, "%s", c.mixMessage("`"+vd.FullName()+"`", "route", name, groups))
			case len(groups) == 1:
				st.Silo = groups[0].silo
			}
			c.res.Arms[name] = append(c.res.Arms[name], st)
		}
	}
	// a job's `run` (#168) is an arm too; a write of a silo other than its `silo k`'s (or of any
	// silo, without one) is E0026 where it is written
	for _, name := range c.prog.Order {
		if c.prog.Failed[name] {
			continue
		}
		for _, j := range c.jobs[name] {
			vd := c.routeMembers(c.prog.Modules[name], j.Name)["run"]
			if vd == nil || vd.Body == nil {
				continue
			}
			switch groups := c.storeGroups(writesOf(vd.Body)); {
			case len(groups) > 1:
				c.errorf(diag.WriteStores, vd.Span, "%s", c.mixMessage("`"+vd.FullName()+"`", "job", name, groups))
			case len(groups) == 1 && j.Silo != "" && groups[0].silo == "":
				// a `silo k` job runs in its silo (the worker binds it there): a shared write
				// is a second store, as it is for a route whose arm is the silo's
				c.errorf(diag.WriteStores, vd.Span, "%s", siloJobMessage("`"+vd.FullName()+"`", j.Silo, groups[0].tables))
			}
		}
	}
	for _, fe := range fakes {
		if groups := c.storeGroups(writesOf(fe.d.Body)); len(groups) > 1 {
			c.errorf(diag.WriteStores, fe.d.Span, "in `fake %s`'s `%s`: %s", fe.fake, fe.d.Name, c.mixMessage("`"+fe.d.Name+"`", "entry", fe.module, groups))
		}
	}
}

// storeGroup is the tables (bare names, sorted) an arm writes in one store (silo "" is shared),
// and the helpers ("Module.value", in the tables' order, each once) writes of them are reached
// through.
type storeGroup struct {
	silo   string
	tables []string
	vias   []string
}

// storeGroups groups the table keys written (each with the helper it is reached through, as
// writesOf gives them) by store: silos by key, then the shared store.
func (c *Checker) storeGroups(writes map[string]string) []storeGroup {
	by := map[string][]string{}
	viaOf := map[string]string{}
	for t, via := range writes {
		s := c.siloOf[t]
		name := strings.TrimPrefix(t, tableModule(t)+".")
		by[s] = append(by[s], name)
		viaOf[s+" "+name] = via
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
		var vias []string
		for _, t := range ts {
			if v := viaOf[k+" "+t]; v != "" && !slices.Contains(vias, v) {
				vias = append(vias, v)
			}
		}
		out = append(out, storeGroup{k, ts, vias})
	}
	return out
}

// mixMessage is E0031's text for arm name of module mod writing groups (two or more):
// "`Create.post` writes `Project` (silo `org`) and `AuditLog` (shared, through `Audit.record`):
// a Task writes to one store. …", a store's note naming the helpers its writes are reached
// through (#310); the hint sends a write from its own `from` ("entry" for a fake, "route" for a
// write route).
func (c *Checker) mixMessage(name, from, mod string, groups []storeGroup) string {
	parts := make([]string, len(groups))
	shared := ""
	for i, g := range groups {
		ts := make([]string, len(g.tables))
		for j, t := range g.tables {
			ts[j] = "`" + t + "`"
		}
		where := "shared"
		if g.silo != "" {
			where = "silo `" + g.silo + "`"
		} else {
			shared = andList(ts)
		}
		if len(g.vias) > 0 {
			vs := make([]string, len(g.vias))
			for j, v := range g.vias {
				vs[j] = "`" + helperName(v, mod) + "`"
			}
			where += ", through " + andList(vs)
		}
		where = "(" + where + ")"
		parts[i] = andList(ts) + " " + where
	}
	hint := "Send one of the writes from its own " + from + "."
	if shared != "" {
		hint = "Write " + shared + " inside the silo, or send it from its own " + from + "."
	}
	return name + " writes " + andList(parts) + ": a Task writes to one store. " + hint
}

// siloJobMessage is E0031's text for the `run` (name) of a `silo k` job that writes the shared
// tables tables: the job runs in its silo, so they are a second store.
func siloJobMessage(name, key string, tables []string) string {
	ts := make([]string, len(tables))
	for i, t := range tables {
		ts[i] = "`" + t + "`"
	}
	return fmt.Sprintf("%s writes %s (shared), but a job with `silo %s` runs in its %s's silo: a Task writes to one store. "+
		"Write %s from a job without `silo %s` (this job may enqueue one), or keep the write in the silo.", name, andList(ts), key, key, andList(ts), key)
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
