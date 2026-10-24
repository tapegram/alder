// Declared rules (M11, generalised by M15): a table's partition — `@owner` (E0024) or
// `@partition k` (E0032) —, the scoped Db operations (E0025, E0026, the row of the scoped
// insert) and a page's `requires user, role R` (E0027). partitions.go holds M15's App
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

// checkRoles validates the `role R | S` clause of page pd (E0027) and records its roles in
// info when it is well-formed: it follows `requires user`, and each role is a constructor of
// the union that App's session field `role` has.
func (mc *modCtx) checkRoles(pd *syntax.PageDecl, info *pageInfo) {
	c := mc.c
	if len(pd.Requires) == 0 || pd.Requires[0] != "user" {
		c.errorf(diag.RoleClause, pd.RolesSpan, "`role` needs `user` first")
		return
	}
	var role *types.Field
	for i, f := range c.session.Fields {
		if f.Name == "role" {
			role = &c.session.Fields[i]
		}
	}
	if role == nil {
		c.errorf(diag.RoleClause, pd.RolesSpan, "this session has no `role` field")
		return
	}
	un, ok := role.Type.(*types.Union)
	switch {
	case types.IsErr(role.Type):
		return
	case ok && un.Name == "Maybe" && un.Module == "Prelude":
		c.errorf(diag.RoleClause, pd.RolesSpan, "this session's `role` field is `%s`: the role must be a union, not `Maybe`", mc.ts(role.Type))
		return
	case !ok:
		c.errorf(diag.RoleClause, pd.RolesSpan, "this session's `role` field is `%s`, not a union: write `role: Role = …` with `type Role = Reader | Author`", mc.ts(role.Type))
		return
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
	if !bad {
		info.Roles = pd.Roles
	}
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
			mc.markScoped("user", op)
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
// a page module whose page `requires` key (any of its values), or takes it from its parameter
// (`silo key`, the admin form), and in a `test` (a test has a session, from `Test.login` or
// `Session.put`; a scoped operation without the field fails when it runs). Never in a fake.
func (mc *modCtx) narrowed(key string) bool {
	switch {
	case mc.fake != "":
		return false
	case mc.inTest:
		return true
	}
	return mc.page != nil && (slices.Contains(mc.page.Requires, key) || mc.page.Decl != nil && mc.page.Decl.Silo == key)
}

// scopedRefusal is E0026's message for subject (“`Db.mine`”, “`Db.scoped Project`”, or
// “`Todos.Index.recent` uses `Db.mine`, which”) used where mc is checking, "" when it may be
// used there (narrowed): a page module whose page `requires key`, or a test. A fake's entries may
// not: a fake has no session.
func (mc *modCtx) scopedRefusal(subject, key string) string {
	switch {
	case mc.fake != "":
		return subject + " cannot be used in a fake: a fake has no session"
	case mc.narrowed(key):
		return ""
	case mc.page != nil:
		return subject + " needs a page that `requires " + key + "`: `" + mc.m.Name + "` does not"
	}
	return subject + " needs a page that `requires " + key + "`: `" + mc.m.Name + "` is not a page"
}

// markScoped notes that the value being checked (outside tests) uses operation op, which needs
// the session's key narrowed.
func (mc *modCtx) markScoped(key, op string) {
	c := mc.c
	if mc.value == "" || mc.inTest || mc.fake != "" {
		return
	}
	k := c.key(mc.m.Name, mc.value)
	if c.scopedDirect == nil {
		c.scopedDirect = map[string]map[string]string{}
	}
	if c.scopedDirect[k] == nil {
		c.scopedDirect[k] = map[string]string{}
	}
	if c.scopedDirect[k][key] == "" {
		c.scopedDirect[k][key] = op
	}
}

// scopedRef handles a reference x to a top-level value: within its module it is an edge for
// propagateScoped; to another module's value that uses a scoped operation (scopedUsing), it is
// E0026 where that operation's key is not narrowed, and marks the value being checked
// otherwise (a `requires user` page module's value using another's).
func (mc *modCtx) scopedRef(x *syntax.Var) {
	c := mc.c
	k := c.key(x.Ref.Module, x.Ref.Name)
	if x.Ref.Module == mc.m.Name {
		if mc.value != "" && !mc.inTest && mc.fake == "" {
			c.scopedEdges = append(c.scopedEdges, [2]string{c.key(mc.m.Name, mc.value), k})
		}
		return
	}
	uses := c.scopedUsing[k]
	keys := make([]string, 0, len(uses))
	for key := range uses {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		op := uses[key]
		if msg := mc.scopedRefusal("`"+k+"` uses `"+op+"`, which", key); msg != "" {
			c.reportScoped(x.Span, msg)
			return
		}
	}
	for _, key := range keys {
		mc.markScoped(key, uses[key])
	}
}

// propagateScoped recomputes scopedUsing, the values that use a scoped operation directly or
// through values of their module (per key, the first operation): run after each module's
// bodies, so a module's importers (checked after it) see its marks.
func (c *Checker) propagateScoped() {
	c.scopedUsing = map[string]map[string]string{}
	add := func(v, key, op string) bool {
		if c.scopedUsing[v] == nil {
			c.scopedUsing[v] = map[string]string{}
		}
		if c.scopedUsing[v][key] != "" {
			return false
		}
		c.scopedUsing[v][key] = op
		return true
	}
	for k, ops := range c.scopedDirect {
		for key, op := range ops {
			add(k, key, op)
		}
	}
	for changed := true; changed; {
		changed = false
		for _, e := range c.scopedEdges {
			for key, op := range c.scopedUsing[e[1]] {
				if add(e[0], key, op) {
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
				mc.markScoped(info.PartKey, "Db."+name+" "+rec.Name)
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
			mc.markScoped(key, "Db."+v.Ref.Name+" "+rec.Name)
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
