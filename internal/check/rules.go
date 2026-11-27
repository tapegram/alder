// Declared rules (M11, generalised by M15): a table's partition — `@owner` (E0024) or
// `@partition k` (E0032) —, the scoped Db operations (E0025, E0026, the row of the scoped
// insert) and a route's `requires user, role R` (E0027). partitions.go holds M15's App
// declarations (via, silo, choose) and requires rules; writeset.go the one-store analysis.
package check

import (
	"fmt"
	"slices"
	"sort"
	"strings"

	"alder/internal/diag"
	"alder/internal/syntax"
	"alder/internal/types"
)

// scopedOps are the scoped Db functions: M11's `mine` family (legal only on a table partitioned
// by `user`, that is with `@owner` or `@partition user`) and M15's generic family (any
// partition key), mapped to the generic name (the stdlib function's role).
var scopedOps = map[string]string{
	"mine": "scoped", "getMine": "getScoped", "updateMine": "updateScoped", "deleteMine": "deleteScoped", "insertMine": "insertScoped",
	"scoped": "scoped", "getScoped": "getScoped", "updateScoped": "updateScoped", "deleteScoped": "deleteScoped", "insertScoped": "insertScoped",
}

// mineOp reports whether name is one of M11's `mine` family.
func mineOp(name string) bool { return scopedOps[name] != "" && scopedOps[name] != name }

// scopedUse is a scoped operation written in a body, with its instantiated type, for E0025 and
// (a generic one, whose key is its table's) E0026, once the body's types are known
// (checkScopedTables).
type scopedUse struct {
	v *syntax.Var
	t types.Type
}

// sessionUser is the type of App's session field `user` with its `Maybe` removed (what a page
// that `requires user` sees), and whether the session has the field.
func (c *Checker) sessionUser() (types.Type, bool) {
	return c.sessionKey("user")
}

// sessionKey is the type of App's session field k with its `Maybe` removed, and whether the
// session has the field.
func (c *Checker) sessionKey(k string) (types.Type, bool) {
	for _, f := range c.session.Fields {
		if f.Name == k {
			if inner, ok := maybeOf(f.Type); ok {
				return inner, true
			}
			return f.Type, true
		}
	}
	return nil, false
}

// checkOwners validates every table's partition (E0024 for `@owner`, E0032 for `@partition k`)
// against App's session, once it is defined, and records the partition and the scoped insert's
// row record in the table's info. A partition in error still names its column, so scoped uses
// of the table do not also report E0025.
func (c *Checker) checkOwners() {
	if app := c.prog.Modules["App"]; app != nil {
		for _, d := range c.live(app) {
			if _, ok := d.(*syntax.SessionDecl); ok {
				c.hasSession = true
			}
		}
	}
	for _, name := range c.prog.Order {
		m := c.prog.Modules[name]
		for _, d := range c.live(m) {
			if td, ok := d.(*syntax.TableDecl); ok { // live lists a fake's tables too
				c.checkOwner(m, td)
			}
		}
	}
}

func (c *Checker) checkOwner(m *syntax.Module, d *syntax.TableDecl) {
	info := c.tables[c.key(m.Name, d.Name)]
	if info == nil {
		return
	}
	userT, hasUser := c.sessionUser()
	var first *syntax.Field
	firstKey, firstOwner := "", false
	for _, f := range d.Fields {
		for _, a := range f.Attrs {
			if a.Name != "owner" && a.Name != "partition" {
				continue
			}
			owner, key := a.Name == "owner", a.Arg
			if owner {
				key = "user"
			}
			switch {
			case first == f && owner && firstOwner:
				c.errorf(diag.OwnerColumn, a.Span, "a field takes one `@owner`")
			case first == f:
				c.errorf(diag.PartitionDecl, a.Span, "a field takes one `@partition` (`@owner` is `@partition user`)")
			case owner && !hasUser:
				c.errorf(diag.OwnerColumn, a.Span, "`@owner` needs a session with a `user` field: this app's session has none")
			case !owner && !c.hasSession:
				c.errorf(diag.PartitionDecl, a.Span, "`@partition` needs a session: declare App's `session { … }` with the field `%s`", key)
			case first != nil && owner && firstOwner:
				c.errorf(diag.OwnerColumn, a.Span, "a table has one `@owner` column: `%s` marks `%s` and `%s`", d.Name, first.Name, f.Name)
			case first != nil:
				c.errorf(diag.PartitionDecl, a.Span, "a table has one partition column: `%s` marks `%s` and `%s`", d.Name, first.Name, f.Name)
			case owner:
				ft := info.Columns[f.Name]
				const msg = "`@owner` must mark a `@ref` column of the session's user type `%s`; `%s` is `%s`"
				switch {
				case c.u.Unify(ft, userT) != nil:
					c.errorf(diag.OwnerColumn, a.Span, msg, c.typeIn(userT, m.Name), f.Name, c.typeIn(ft, m.Name))
				case !hasAttr(f, "ref"):
					c.errorf(diag.OwnerColumn, a.Span, msg+" without `@ref`", c.typeIn(userT, m.Name), f.Name, c.typeIn(ft, m.Name))
				}
			default:
				ft := info.Columns[f.Name]
				kt, hasKey := c.sessionKey(key)
				_, isMaybe := c.fieldMaybe(key)
				switch {
				case types.IsErr(ft):
				case !hasAttr(f, "ref"):
					c.errorf(diag.PartitionDecl, a.Span, "`@partition %s` must mark a `@ref` column; `%s` is `%s` without `@ref`", key, f.Name, c.typeIn(ft, m.Name))
				case !hasKey || !isMaybe || c.u.Unify(ft, kt) != nil:
					c.errorf(diag.PartitionDecl, a.Span, "`@partition %s` needs a session field `%s` of type `Maybe %s`", key, key, c.typeIn(ft, m.Name))
				}
			}
			if first == nil {
				first, firstKey, firstOwner = f, key, owner
			}
		}
	}
	if first == nil {
		return
	}
	info.PartKey, info.PartCol = firstKey, first.Name
	if firstKey == "user" {
		info.Owner = first.Name
	}
	info.InsertScoped = &types.Record{Module: info.Module, Name: "InsertScoped " + info.Name}
	for _, f := range info.Insert.Fields {
		if f.Name != first.Name {
			info.InsertScoped.Fields = append(info.InsertScoped.Fields, f)
		}
	}
}

// fieldMaybe is App's session field k's type and whether it is a `Maybe`.
func (c *Checker) fieldMaybe(k string) (types.Type, bool) {
	for _, f := range c.session.Fields {
		if f.Name == k {
			_, ok := maybeOf(f.Type)
			return f.Type, ok
		}
	}
	return nil, false
}

// checkRoles validates the `role R | S` clause of a route's guard pd (E0027) and
// returns its roles when it is well-formed (nil otherwise): it follows `requires user`, and each
// role is a constructor of the union that App's session field `role` has.
func (mc *modCtx) checkRoles(pd *syntax.Guard) []string {
	c := mc.c
	if len(pd.Requires) == 0 || pd.Requires[0] != "user" {
		c.errorf(diag.RoleClause, pd.RolesSpan, "`role` needs `user` first")
		return nil
	}
	var role *types.Field
	for i, f := range c.session.Fields {
		if f.Name == "role" {
			role = &c.session.Fields[i]
		}
	}
	if role == nil {
		c.errorf(diag.RoleClause, pd.RolesSpan, "this session has no `role` field")
		return nil
	}
	un, ok := role.Type.(*types.Union)
	switch {
	case types.IsErr(role.Type):
		return nil
	case ok && un.Name == "Maybe" && un.Module == "Prelude":
		c.errorf(diag.RoleClause, pd.RolesSpan, "this session's `role` field is `%s`: the role must be a union, not `Maybe`", mc.ts(role.Type))
		return nil
	case !ok:
		c.errorf(diag.RoleClause, pd.RolesSpan, "this session's `role` field is `%s`, not a union: write `role: Role = …` with `type Role = Reader | Author`", mc.ts(role.Type))
		return nil
	}
	seen := map[string]bool{}
	for _, r := range pd.Roles {
		if seen[r] {
			c.errorf(diag.RoleClause, pd.RolesSpan, "`role` names `%s` twice", r)
		}
		seen[r] = true
	}
	ctors := map[string]bool{}
	for _, ct := range un.Constructors() {
		ctors[ct.Name] = true
	}
	bad := len(seen) != len(pd.Roles)
	for _, r := range pd.Roles {
		if !ctors[r] {
			bad = true
			var names []string
			for _, ct := range un.Constructors() {
				names = append(names, ct.Name)
			}
			c.errorf(diag.RoleClause, pd.RolesSpan, "`%s` is not a constructor of this session's `role` type `%s` (%s)", r, mc.ts(un), strings.Join(names, ", "))
		}
	}
	if bad {
		return nil
	}
	return pd.Roles
}

// inferScoped types a reference to a scoped operation `Db.<name>`: its stdlib signature, with
// the scoped insert's row `InsertScoped t` (Insert t without the partition column). For the
// `mine` family (key `user`) it reports E0026 when the context is not one where `session.user`
// is narrowed, and marks the value being checked as using the operation (scopedRef); a generic
// operation's key is its table's, known once the body's types are (checkScopedTables). It notes
// the use for E0025.
func (mc *modCtx) inferScoped(x *syntax.Var) types.Type {
	c := mc.c
	op := "Db." + x.Ref.Name
	if mineOp(x.Ref.Name) {
		if msg := mc.scopedRefusal("`"+op+"`", "user"); msg != "" {
			c.reportScoped(x.Span, msg)
		} else {
			mc.markScoped("user", op, "", x.Span)
		}
	}
	if scopedOps[x.Ref.Name] == "insertScoped" && !c.insertRows[x] {
		c.reportScoped(x.Span, "`"+op+"` takes its row as a record literal: write the record literal in the `"+op+"` call (its row is the table's `Insert` record without its "+partitionColumnWords(x.Ref.Name)+")")
	}
	sig, ok := c.values[c.key("Db", x.Ref.Name)]
	if !ok {
		return types.Err
	}
	t := c.u.Instantiate(sig)
	if fn, ok := t.(*types.Fun); ok && scopedOps[x.Ref.Name] == "insertScoped" && len(fn.Params) == 2 {
		if app, ok := fn.Params[1].(*types.App); ok && app.Con == "Insert" {
			t = &types.Fun{Params: []types.Type{fn.Params[0], &types.App{Con: "InsertScoped", Args: app.Args}}, Result: fn.Result}
		}
	}
	c.scopedUses = append(c.scopedUses, scopedUse{x, t})
	return t
}

// partitionColumnWords names a table's partition column in a message about op: M11's words for
// the `mine` family.
func partitionColumnWords(op string) string {
	if mineOp(op) {
		return "`@owner` column"
	}
	return "`@partition` column"
}

// reportScoped reports E0026 (or the scoped insert's row error) once per place.
func (c *Checker) reportScoped(at diag.Span, msg string) {
	if c.scopedReported == nil {
		c.scopedReported = map[diag.Span]bool{}
	}
	if c.scopedReported[at] {
		return
	}
	c.scopedReported[at] = true
	code := diag.ScopedOutside
	if strings.Contains(msg, "takes its row as a record literal") {
		code = diag.TypeMismatch
	}
	c.errorf(code, at, "%s", msg)
}

// narrowed reports whether the session's field key is certainly there where mc is checking: in
// a member of a route that `requires` key, or takes it from its parameter (`silo key`, the admin
// form), and in a `test` (a test has a
// session, from `Test.login` or `Session.put`; a scoped operation without the field fails when
// it runs). Never in a fake. In a top-level value that is no route's or job's member, in any
// module (#310), when its signature's `requires` clause names key.
func (mc *modCtx) narrowed(key string) bool {
	switch {
	case mc.fake != "":
		return false
	case mc.inTest:
		return true
	case mc.route != nil:
		return slices.Contains(mc.route.narrowed, key)
	}
	if j := mc.armJob(); j != nil { // a job's `run`: only its `silo k` (#168)
		return j.Silo != "" && j.Silo == key
	}
	if !mc.topValue() {
		return false
	}
	vd := mc.c.valueDecls[mc.c.key(mc.m.Name, mc.value)]
	return vd.d != nil && slices.Contains(mc.c.statedKeys(vd.d), key)
}

// topValue reports whether mc is checking a top-level value that is no route's or job's member.
func (mc *modCtx) topValue() bool {
	return mc.value != "" && !strings.Contains(mc.value, ".")
}

// need is what a value needs of one session key (scopedUsing, by key): Op the first operation
// on its path (“Db.scoped Reviewer”), Via the first helper on that path ("Module.value"), ""
// when the value uses Op itself; At the use in the value's body that needs it (the operation,
// or the reference to Via). Op is "" when the need comes from a `requires` clause alone (a
// clause ahead of its body, W0015).
type need struct {
	Op, Via string
	At      diag.Span
}

// scopedEdge is a reference, at at, from value from to value to of the same module
// ("Module.value"), for propagateScoped.
type scopedEdge struct {
	from, to string
	at       diag.Span
}

// valueDecl is a top-level value declaration and its module.
type valueDecl struct {
	mod string
	d   *syntax.ValueDecl
}

// needSubject is E0026's subject for a use of value name (as the message names it), which
// needs n of session key key, in module mod: “`Access.reviewers` uses `Db.scoped Reviewer`
// (through `Table.Reviewer.current`), which”; “`Reviewer.current` says `requires org`, which”
// for a need its clause states and its body does not have.
func needSubject(name string, n need, mod, key string) string {
	switch {
	case n.Op == "" && n.Via == "":
		return "`" + name + "` says `requires " + key + "`, which"
	case n.Op == "":
		return "`" + name + "` uses `" + helperName(n.Via, mod) + "`, which"
	}
	return "`" + name + "` uses `" + n.Op + "`" + throughNote(n.Via, mod) + ", which"
}

// throughNote is “ (through `X`)” for helper via ("Module.value") named in module mod: by its
// bare name in its own module, as the messages name a module's own values. "" for no helper.
func throughNote(via, mod string) string {
	if via == "" {
		return ""
	}
	return " (through `" + helperName(via, mod) + "`)"
}

// helperName is value key ("Module.value") as module mod names it.
func helperName(key, mod string) string {
	if rest, ok := strings.CutPrefix(key, mod+"."); ok && !strings.Contains(rest, ".") {
		return rest
	}
	return key
}

// scopedRefusal is E0026's message for subject (“`Db.mine`”, “`Db.scoped Project`”, or
// “`Todos.Index.recent` uses `Db.mine`, which”) used where mc is checking, "" when it may be
// used there (narrowed): a member of a route that `requires key`, a job with `silo key`, or a
// test; and in any top-level value that is no member, which then has the need, checked against
// its `requires` clause once every body is (checkStatedNeeds). A fake's entries may not: a fake
// has no session.
func (mc *modCtx) scopedRefusal(subject, key string) string {
	switch {
	case mc.fake != "":
		return subject + " cannot be used in a fake: a fake has no session"
	case mc.narrowed(key), mc.topValue():
		return ""
	case mc.armJob() != nil:
		return jobRefusal(mc.armJob(), subject, key)
	case mc.route != nil:
		return subject + " needs a route that `requires " + key + "`: `" + mc.route.Name + "` does not"
	}
	// a brand's check, a field's default: no session at all
	return subject + " needs a route that `requires " + key + "`: this has no session"
}

// markScoped notes that the value being checked (outside tests) uses operation op, at at, which
// needs the session's key narrowed, itself (via "") or through the helper via of another module.
func (mc *modCtx) markScoped(key, op, via string, at diag.Span) {
	c := mc.c
	if mc.value == "" || mc.inTest || mc.fake != "" {
		return
	}
	k := c.key(mc.m.Name, mc.value)
	if c.scopedDirect == nil {
		c.scopedDirect = map[string]map[string]need{}
	}
	if c.scopedDirect[k] == nil {
		c.scopedDirect[k] = map[string]need{}
	}
	if old, ok := c.scopedDirect[k][key]; !ok || old.Op == "" && op != "" {
		c.scopedDirect[k][key] = need{Op: op, Via: via, At: at}
	}
}

// scopedRef handles a reference x to a top-level value: within its module it is an edge for
// propagateScoped (and, from a member, a job's `run` or a fake, a routeRef checked once the
// module's bodies are); to another module's value that has a need (statedNeed: its signature's,
// #310), it is E0026 where that need's key is not narrowed, and marks the value being checked
// otherwise (with the referenced value as the helper its need comes through).
func (mc *modCtx) scopedRef(x *syntax.Var) {
	c := mc.c
	k := c.key(x.Ref.Module, x.Ref.Name)
	if x.Ref.Module == mc.m.Name {
		if mc.value != "" && !mc.inTest && mc.fake == "" {
			c.scopedEdges = append(c.scopedEdges, scopedEdge{from: c.key(mc.m.Name, mc.value), to: k, at: x.Span})
		}
		switch {
		case mc.inTest:
		case mc.fake != "":
			c.routeRefs = append(c.routeRefs, routeRef{x: x, fake: mc.fake, entry: mc.fakeEntry})
		case mc.route != nil:
			// what the value uses is known once the module's bodies are (checkRouteRefs)
			c.routeRefs = append(c.routeRefs, routeRef{x: x, route: mc.route})
		case mc.armJob() != nil:
			c.routeRefs = append(c.routeRefs, routeRef{x: x, job: mc.armJob()})
		case !mc.topValue(): // a brand's check, a field's default: no session, as scopedRefusal says
			c.routeRefs = append(c.routeRefs, routeRef{x: x})
		}
		return
	}
	uses := c.statedUses(k)
	for _, key := range sortedKeys(uses) {
		if msg := mc.scopedRefusal(needSubject(k, uses[key], mc.m.Name, key), key); msg != "" {
			c.reportScoped(x.Span, msg)
			return
		}
	}
	for _, key := range sortedKeys(uses) {
		mc.markScoped(key, uses[key].Op, k, x.Span)
	}
}

func sortedKeys(uses map[string]need) []string {
	keys := make([]string, 0, len(uses))
	for key := range uses {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

// propagateScoped recomputes scopedUsing, every value's inferred need: per key, the first
// operation and the first helper on its path, from its own uses (scopedDirect) and through the
// values of its module it refers to (scopedEdges: what each states, statedNeed; a fixed point,
// so recursion terminates). It runs after each module's bodies, so a module's importers
// (checked after it) see its needs.
func (c *Checker) propagateScoped() {
	c.scopedUsing = map[string]map[string]need{}
	add := func(v, key string, n need) bool {
		if c.scopedUsing[v] == nil {
			c.scopedUsing[v] = map[string]need{}
		}
		if old, ok := c.scopedUsing[v][key]; ok && (old.Op != "" || n.Op == "") {
			return false
		}
		c.scopedUsing[v][key] = n
		return true
	}
	for k, ops := range c.scopedDirect {
		for key, n := range ops {
			add(k, key, n)
		}
	}
	for changed := true; changed; {
		changed = false
		for _, e := range c.scopedEdges {
			for key, n := range c.statedUses(e.to) {
				if add(e.from, key, need{Op: n.Op, Via: e.to, At: e.at}) {
					changed = true
				}
			}
		}
	}
}

// insertScopedCall handles `Db.insertMine T row` and `Db.insertScoped T row`: a row written as
// a record literal in the call takes the ordinary path (nil); any other row (a variable, a piped
// value, the call partially applied) is reported, since the row's type is not one a program can
// write, and the call is typed without it.
func (mc *modCtx) insertScopedCall(x *syntax.App, v *syntax.Var, pipe *pipeArg, env *env, eff effect) types.Type {
	c := mc.c
	if c.insertRows == nil {
		c.insertRows = map[*syntax.Var]bool{}
	}
	c.insertRows[v] = true
	op := "Db." + v.Ref.Name
	if lit, ok := argAt(x.Args, 1).(*syntax.RecordLit); ok && pipe == nil && len(x.Args) == 2 {
		if c.insertOps == nil {
			c.insertOps = map[*syntax.RecordLit]string{}
		}
		c.insertOps[lit] = op
		return nil
	}
	table, col := "T", "its "+partitionColumnWords(v.Ref.Name)
	if ct, ok := argAt(x.Args, 0).(*syntax.CtorRef); ok && ct.Ref.Kind == syntax.RefTable {
		table = ct.Ref.Name
		if info := c.tables[c.key(ct.Ref.Module, ct.Ref.Name)]; info != nil && info.PartCol != "" {
			col = "`" + info.PartCol + "`"
		}
	}
	at := x.Span
	switch {
	case len(x.Args) >= 2:
		at = x.Args[1].GetSpan()
	case pipe != nil:
		at = pipe.span
	}
	c.reportScoped(at, fmt.Sprintf("`%s` takes its row as a record literal: write the record literal in the `%s` call (its row is `Insert %s` without %s)", op, op, table, col))
	fn, ok := c.u.Apply(mc.infer(v, env, eff)).(*types.Fun)
	if !ok {
		for _, a := range x.Args {
			mc.infer(a, env, eff)
		}
		return types.Err
	}
	for i, a := range x.Args {
		if i == 0 {
			mc.check(a, fn.Params[0], env, eff)
		} else {
			mc.infer(a, env, eff)
		}
	}
	total := len(x.Args)
	if pipe != nil {
		total++
	}
	if total < len(fn.Params) {
		return &types.Fun{Params: fn.Params[total:], Result: fn.Result}
	}
	return fn.Result
}

func argAt(args []syntax.Expr, i int) syntax.Expr {
	if i < len(args) {
		return args[i]
	}
	return nil
}

// checkScopedTables ends a body's scoped checks, once its types are known: E0025 for each
// scoped operation whose table has no partition (or, for the `mine` family, a partition by a key
// other than `user`); E0026 for a generic scoped operation, and for an unscoped Db operation on a
// siloed table, where the table's key is not narrowed (marking the value being checked
// otherwise).
func (mc *modCtx) checkScopedTables() {
	c := mc.c
	uses := c.scopedUses
	c.scopedUses = nil
	for _, u := range uses {
		rec := tableArg(c, u.t)
		if rec == nil {
			continue // a type parameter or unknown table: checkDbTables reports it
		}
		info := c.u.Table(rec)
		if info == nil {
			continue
		}
		name := u.v.Ref.Name
		switch {
		case info.PartKey == "" && mineOp(name):
			c.errorf(diag.NoOwner, u.v.Span, "`%s` has no `@owner` column, so `Db.%s %s` has nothing to scope by", rec.Name, name, rec.Name)
		case info.PartKey == "":
			c.errorf(diag.NoOwner, u.v.Span, "`%s` has no `@partition` column, so `Db.%s %s` has nothing to scope by", rec.Name, name, rec.Name)
		case mineOp(name) && info.PartKey != "user":
			c.errorf(diag.NoOwner, u.v.Span, "`%s` is partitioned by `%s`, not by the session's `user`: `Db.%s` is for `@owner` tables; write `Db.%s %s`", rec.Name, info.PartKey, name, scopedOps[name], rec.Name)
		case !mineOp(name):
			subject := "`Db." + name + " " + rec.Name + "`"
			if msg := mc.scopedRefusal(subject, info.PartKey); msg != "" {
				c.reportScoped(u.v.Span, msg)
			} else {
				mc.markScoped(info.PartKey, "Db."+name+" "+rec.Name, "", u.v.Span)
			}
		}
	}
	if len(c.siloOf) == 0 {
		return
	}
	for _, e := range c.bodyExprs {
		v, ok := e.(*syntax.Var)
		if !ok || v.Ref.Kind != syntax.RefStd || v.Ref.Module != "Db" || scopedOps[v.Ref.Name] != "" {
			continue
		}
		rec := tableArg(c, c.res.Types[e])
		if rec == nil {
			continue
		}
		key := c.siloOf[c.key(rec.Module, rec.Name)]
		if key == "" {
			continue
		}
		subject := "`Db." + v.Ref.Name + " " + rec.Name + "` (`" + rec.Name + "` lives in `silo " + key + "`)"
		if msg := mc.scopedRefusal(subject, key); msg != "" {
			c.reportScoped(v.Span, msg)
		} else {
			mc.markScoped(key, "Db."+v.Ref.Name+" "+rec.Name, "", v.Span)
		}
	}
}

// tableArg is the table record of a Db function type t's `Table t` parameter, nil when it has
// none or its table is not known.
func tableArg(c *Checker, t types.Type) *types.Record {
	fn, ok := c.u.Apply(t).(*types.Fun)
	if !ok {
		return nil
	}
	for _, p := range fn.Params {
		app, ok := c.u.Apply(p).(*types.App)
		if !ok || app.Con != "Table" || len(app.Args) != 1 {
			continue
		}
		rec, _ := c.u.Apply(app.Args[0]).(*types.Record)
		return rec
	}
	return nil
}

// insertRecordTable is the table whose insert record rec is — `Insert T`, or the scoped
// insert's `InsertScoped T` (mine set) — or nil.
func (c *Checker) insertRecordTable(rec *types.Record) (info *types.TableInfo, mine bool) {
	if rec.Anonymous {
		return nil, false
	}
	if name, ok := strings.CutPrefix(rec.Name, "InsertScoped "); ok {
		return c.tables[c.key(rec.Module, name)], true
	}
	if name, ok := strings.CutPrefix(rec.Name, "Insert "); ok {
		return c.tables[c.key(rec.Module, name)], false
	}
	return nil, false
}

// recordLabel is how a message for module mod names record rec: its type, but the scoped insert's row (whose
// type a program never writes) as "Insert T` without `owner" (the message's backticks around it).
func (c *Checker) recordLabel(rec *types.Record, mod string) string {
	if info, mine := c.insertRecordTable(rec); mine && info != nil {
		return "Insert " + info.Name + "` without `" + info.PartCol
	}
	return c.typeIn(rec, mod)
}

// recordLabelName is recordLabel with a declared record named without its module.
func (c *Checker) recordLabelName(rec *types.Record) string {
	if info, mine := c.insertRecordTable(rec); mine && info != nil {
		return c.recordLabel(rec, "")
	}
	return rec.Name
}

// statedKeys are the session keys d's `requires` clause names that are `Maybe` fields of App's
// session (another is already E0011, and is no need a caller could meet).
func (c *Checker) statedKeys(d *syntax.ValueDecl) []string {
	var out []string
	for _, k := range d.Needs {
		if _, ok := c.fieldMaybe(k); ok && k != "" {
			out = append(out, k)
		}
	}
	return out
}

// statedNeed is S for value key ("Module.value"), the session keys its callers see (#310 rule
// 3): its `requires` clause. A route-module or job-module value without a clause is still
// accepted for one release (W0014), and its callers see what its body needs, S ∪ I; so do a
// route's or job's member and a value with no declaration here.
func (c *Checker) statedNeed(key string) []string {
	vd, ok := c.valueDecls[key]
	switch {
	case !ok || vd.d.Route != "":
		return sortedKeys(c.scopedUsing[key])
	case len(vd.d.Needs) > 0:
		return c.statedKeys(vd.d)
	case c.unstatedAllowed(vd.mod):
		return sortedKeys(c.scopedUsing[key])
	}
	return nil
}

// statedUses is statedNeed with each key's need as the body has it (its operation and first
// helper, for the messages); a key the clause states and the body does not use has no Op.
func (c *Checker) statedUses(key string) map[string]need {
	keys := c.statedNeed(key)
	if len(keys) == 0 {
		return nil
	}
	out := map[string]need{}
	for _, k := range keys {
		out[k] = c.scopedUsing[key][k]
	}
	return out
}

// unstatedAllowed reports whether module mod declares a route or a job: there a helper whose
// signature does not state its need is W0014 for one release, elsewhere E0026 (ruling R2).
func (c *Checker) unstatedAllowed(mod string) bool {
	return c.prog.IsRouteModule(mod) || len(c.jobs[mod]) > 0
}

// sessionOrder is keys (a set) in the order of App's session fields.
func (c *Checker) sessionOrder(keys map[string]bool) []string {
	var out []string
	for _, f := range c.session.Fields {
		if keys[f.Name] {
			out = append(out, f.Name)
		}
	}
	return out
}

// checkStatedNeeds checks every top-level value's `requires` clause against its inferred need
// (#310), once every body is checked and test helpers are known (they neither state nor are
// checked for needs): a key the body needs and the clause lacks is E0026 at the use that needs
// it (W0014 for a route or job module's value with no clause), a key the clause names and the
// body does not need is W0015, and a clause on a route's member or a job's `run` is E0043.
func (c *Checker) checkStatedNeeds() {
	for _, name := range c.prog.Order {
		if c.prog.Failed[name] {
			continue
		}
		mc := c.ctx(c.prog.Modules[name])
		for _, d := range c.live(mc.m) {
			vd, ok := d.(*syntax.ValueDecl)
			if !ok || vd.Sig == nil || vd.Body == nil {
				continue
			}
			if vd.Route != "" {
				mc.memberClause(vd)
				continue
			}
			if k := c.key(name, vd.Name); c.res.TestOnly[k] == "" {
				mc.checkStated(vd, k)
			}
		}
	}
}

// memberClause reports E0043 for a `requires` clause on a route's member or a job's `run`: the
// key belongs on the route line, or in the job's `silo k`. (A clause on another qualified
// declaration is refused with its name, names.noSuchRoute.)
func (mc *modCtx) memberClause(vd *syntax.ValueDecl) {
	c := mc.c
	if vd.NeedsSpan == (diag.Span{}) || vd.NeedsErr {
		return // no clause, or one the parser gave up on (its error is reported)
	}
	if len(c.statedKeys(vd)) < len(vd.Needs) {
		return // a key that is no `Maybe` field of the session: that one error (E0011) is reported
	}
	if j := c.job(mc.m.Name, vd.Route); j != nil {
		key := ""
		for _, k := range vd.Needs {
			if k != "user" { // a job never has `user`: name the key a silo could hold
				key = k
				break
			}
		}
		if key == "" {
			c.errorf(diag.RouteDecl, vd.NeedsSpan, "`%s` takes its key from its job, and a job has no `user`: give the job the user as a parameter and use the unscoped operations", vd.FullName())
			return
		}
		c.errorf(diag.RouteDecl, vd.NeedsSpan, "`%s` takes its key from its job: write `silo %s` on `job %s`", vd.FullName(), key, j.Name)
		return
	}
	if c.route(mc.m.Name, vd.Route) != nil {
		c.errorf(diag.RouteDecl, vd.NeedsSpan, "`%s` takes what it requires from its route: write `requires %s` on `route %s`", vd.FullName(), strings.Join(vd.Needs, ", "), vd.Route)
	}
}

// checkStated checks value vd's (key k) `requires` clause, S, against what its body needs, I
// (scopedUsing): rule 1, a key of I that S lacks, at the use that needs it, with the clause
// inserted or rewritten (keys in the session's order) as the fix; rule 2, a key of S that I
// lacks (W0015).
func (mc *modCtx) checkStated(vd *syntax.ValueDecl, k string) {
	c := mc.c
	clause := vd.NeedsSpan != (diag.Span{})
	if vd.NeedsErr {
		return // a clause the parser gave up on (E0027, or a missing key): its error is reported
	}
	inferred := c.scopedUsing[k]
	stated := c.statedKeys(vd)
	all := map[string]bool{}
	for _, key := range stated {
		all[key] = true
	}
	var missing []string
	for _, key := range c.sessionOrder(keysOf(inferred)) {
		if !all[key] {
			missing = append(missing, key)
		}
		all[key] = true
	}
	if len(missing) > 0 {
		keys := strings.Join(c.sessionOrder(all), ", ")
		reported := map[diag.Span]bool{} // one diagnostic per use, whatever it needs
		held := mc.clauseHeld(inferred, missing)
		for _, key := range missing {
			n := inferred[key]
			at := n.At
			if at == (diag.Span{}) {
				at = vd.Span
			}
			if reported[at] {
				continue
			}
			reported[at] = true
			var subject string
			if n.Via != "" && c.states(n.Via, key) {
				subject = "`" + vd.Name + "` uses `" + helperName(n.Via, mc.m.Name) + "`, which `requires " + key + "`"
			} else {
				subject = needSubject(vd.Name, n, mc.m.Name, key) + " needs `" + key + "`"
			}
			switch {
			case clause:
				d := c.errorf(diag.ScopedOutside, at, "%s: its signature says `%s`; write `requires %s`", subject, mc.source(vd.NeedsSpan), keys)
				d.Fix = diag.ReplaceSpan(vd.NeedsSpan, "requires "+keys)
				d.Fix.Preferred = true
			default:
				sig := vd.FullName() + " : " + strings.Join(strings.Fields(mc.source(vd.Sig.GetSpan())), " ") + " requires " + keys
				var d *diag.Diagnostic
				if c.unstatedAllowed(mc.m.Name) {
					if c.res.Unstated == nil {
						c.res.Unstated = map[string][]string{}
					}
					c.res.Unstated[k] = c.sessionOrder(all)
					d = c.bag.Warn(diag.HelperNeedUnstated, at, "%s: say so in its signature (`%s`). `alder fmt` adds it.", subject, sig)
				} else {
					d = c.errorf(diag.ScopedOutside, at, "%s: say so in its signature, `%s`", subject, sig)
				}
				end := vd.Sig.GetSpan().End
				d.Fix = diag.InsertAt(end.Line, end.Col, " requires "+keys)
				d.Fix.Preferred = true
				d.Fix.Held = held && d.Code == diag.HelperNeedUnstated
			}
		}
	}
	for i, key := range vd.Needs {
		if slices.Index(vd.Needs, key) < i {
			continue // named twice: E0011 at the second (checkValueRequires)
		}
		if _, ok := inferred[key]; !ok && slices.Contains(stated, key) {
			c.bag.Warn(diag.NeedUnused, vd.NeedSpans[i], "`%s` says `requires %s` and uses nothing that needs `%s`", vd.Name, key, key)
		}
	}
}

// states reports whether value key's `requires` clause names session key k.
func (c *Checker) states(key, k string) bool {
	vd, ok := c.valueDecls[key]
	return ok && slices.Contains(c.statedKeys(vd.d), k)
}

func keysOf(m map[string]need) map[string]bool {
	out := map[string]bool{}
	for k := range m {
		out[k] = true
	}
	return out
}
