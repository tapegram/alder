// Declared rules (M11): `@owner` columns (E0024), the scoped Db operations (E0025, E0026, the
// row of Db.insertMine) and a page's `requires user, role R` (E0027).
package check

import (
	"fmt"
	"strings"

	"alder/internal/diag"
	"alder/internal/syntax"
	"alder/internal/types"
)

// scopedOps are the Db functions scoped to the session's user through a table's `@owner`.
var scopedOps = map[string]bool{"mine": true, "getMine": true, "updateMine": true, "deleteMine": true, "insertMine": true}

// scopedUse is a scoped operation written in a body, with its instantiated type, for E0025
// once the body's types are known (checkScopedTables).
type scopedUse struct {
	v *syntax.Var
	t types.Type
}

// sessionUser is the type of App's session field `user` with its `Maybe` removed (what a page
// that `requires user` sees), and whether the session has the field.
func (c *Checker) sessionUser() (types.Type, bool) {
	for _, f := range c.session.Fields {
		if f.Name == "user" {
			if inner, ok := maybeOf(f.Type); ok {
				return inner, true
			}
			return f.Type, true
		}
	}
	return nil, false
}

// checkOwners validates every table's `@owner` (E0024) against App's session, once it is
// defined, and records the owner column and Db.insertMine's row record in the table's info.
// An `@owner` in error still names the column, so scoped uses of the table do not also report
// E0025.
func (c *Checker) checkOwners() {
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
	for _, f := range d.Fields {
		for _, a := range f.Attrs {
			if a.Name != "owner" {
				continue
			}
			switch {
			case first == f:
				c.errorf(diag.OwnerColumn, a.Span, "a field takes one `@owner`")
			case !hasUser:
				c.errorf(diag.OwnerColumn, a.Span, "`@owner` needs a session with a `user` field: this app's session has none")
			case first != nil && first != f:
				c.errorf(diag.OwnerColumn, a.Span, "a table has one `@owner` column: `%s` marks `%s` and `%s`", d.Name, first.Name, f.Name)
			case first == nil:
				ft := info.Columns[f.Name]
				const msg = "`@owner` must mark a `@ref` column of the session's user type `%s`; `%s` is `%s`"
				switch {
				case c.u.Unify(ft, userT) != nil:
					c.errorf(diag.OwnerColumn, a.Span, msg, types.String(userT), f.Name, types.String(ft))
				case !hasAttr(f, "ref"):
					c.errorf(diag.OwnerColumn, a.Span, msg+" without `@ref`", types.String(userT), f.Name, types.String(ft))
				}
			}
			if first == nil {
				first = f
			}
		}
	}
	if first == nil {
		return
	}
	info.Owner = first.Name
	info.InsertMine = &types.Record{Module: info.Module, Name: "InsertMine " + info.Name}
	for _, f := range info.Insert.Fields {
		if f.Name != first.Name {
			info.InsertMine.Fields = append(info.InsertMine.Fields, f)
		}
	}
}

// checkRoles validates the `role R | S` clause of page pd (E0027) and records its roles in
// info when it is well-formed: it follows `requires user`, and each role is a constructor of
// the union that App's session field `role` has.
func (mc *modCtx) checkRoles(pd *syntax.PageDecl, info *pageInfo) {
	c := mc.c
	if pd.Requires != "user" {
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
		c.errorf(diag.RoleClause, pd.RolesSpan, "this session's `role` field is `%s`: the role must be a union, not `Maybe`", types.String(role.Type))
		return
	case !ok:
		c.errorf(diag.RoleClause, pd.RolesSpan, "this session's `role` field is `%s`, not a union: write `role: Role = …` with `type Role = Reader | Author`", types.String(role.Type))
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
			c.errorf(diag.RoleClause, pd.RolesSpan, "`%s` is not a constructor of this session's `role` type `%s` (%s)", r, types.String(un), strings.Join(names, ", "))
		}
	}
	if !bad {
		info.Roles = pd.Roles
	}
}

// inferScoped types a reference to a scoped operation `Db.<name>`: its stdlib signature, with
// Db.insertMine's row `InsertMine t` (Insert t without the owner column). It reports E0026 when
// the context is not one where `session.user` is narrowed, marks the value being checked as
// using the operation (scopedRef), and notes the use for E0025.
func (mc *modCtx) inferScoped(x *syntax.Var) types.Type {
	c := mc.c
	op := "Db." + x.Ref.Name
	if msg := mc.scopedRefusal("`" + op + "`"); msg != "" {
		c.reportScoped(x.Span, msg)
	} else {
		mc.markScoped(op)
	}
	if x.Ref.Name == "insertMine" && !c.insertMineRows[x] {
		c.reportScoped(x.Span, "`Db.insertMine` takes its row as a record literal: write the record literal in the `Db.insertMine` call (its row is the table's `Insert` record without its `@owner` column)")
	}
	sig, ok := c.values[c.key("Db", x.Ref.Name)]
	if !ok {
		return types.Err
	}
	t := c.u.Instantiate(sig)
	if fn, ok := t.(*types.Fun); ok && x.Ref.Name == "insertMine" && len(fn.Params) == 2 {
		if app, ok := fn.Params[1].(*types.App); ok && app.Con == "Insert" {
			t = &types.Fun{Params: []types.Type{fn.Params[0], &types.App{Con: "InsertMine", Args: app.Args}}, Result: fn.Result}
		}
	}
	c.scopedUses = append(c.scopedUses, scopedUse{x, t})
	return t
}

// reportScoped reports E0026 (or the insertMine row error) once per place.
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

// scopedRefusal is E0026's message for subject (“`Db.mine`”, or “`Todos.Index.recent` uses
// `Db.mine`, which”) used where mc is checking, "" when it may be used there: in a page module
// whose page `requires user` (any of its values) or in a `test` (a test has a session, from
// `Test.login` or `Session.put`; a scoped operation without a signed-in user fails when it
// runs). A fake's entries may not: a fake has no session.
func (mc *modCtx) scopedRefusal(subject string) string {
	switch {
	case mc.fake != "":
		return subject + " cannot be used in a fake: a fake has no session"
	case mc.inTest:
		return ""
	case mc.page != nil && mc.page.Requires == "user":
		return ""
	case mc.page != nil:
		return subject + " needs a page that `requires user`: `" + mc.m.Name + "` does not"
	}
	return subject + " needs a page that `requires user`: `" + mc.m.Name + "` is not a page"
}

// markScoped notes that the value being checked (outside tests) uses scoped operation op.
func (mc *modCtx) markScoped(op string) {
	c := mc.c
	if mc.value == "" || mc.inTest || mc.fake != "" {
		return
	}
	k := c.key(mc.m.Name, mc.value)
	if c.scopedDirect == nil {
		c.scopedDirect = map[string]string{}
	}
	if c.scopedDirect[k] == "" {
		c.scopedDirect[k] = op
	}
}

// scopedRef handles a reference x to a top-level value: within its module it is an edge for
// propagateScoped; to another module's value that uses a scoped operation (scopedUsing), it is
// E0026 where the operation could not be written itself, and marks the value being checked
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
	op := c.scopedUsing[k]
	if op == "" {
		return
	}
	if msg := mc.scopedRefusal("`" + k + "` uses `" + op + "`, which"); msg != "" {
		c.reportScoped(x.Span, msg)
		return
	}
	mc.markScoped(op)
}

// propagateScoped recomputes scopedUsing, the values that use a scoped operation directly or
// through values of their module: run after each module's bodies, so a module's importers
// (checked after it) see its marks.
func (c *Checker) propagateScoped() {
	c.scopedUsing = map[string]string{}
	for k, op := range c.scopedDirect {
		c.scopedUsing[k] = op
	}
	for changed := true; changed; {
		changed = false
		for _, e := range c.scopedEdges {
			if c.scopedUsing[e[0]] == "" && c.scopedUsing[e[1]] != "" {
				c.scopedUsing[e[0]] = c.scopedUsing[e[1]]
				changed = true
			}
		}
	}
}

// insertMineCall handles `Db.insertMine T row`: a row written as a record literal in the call
// takes the ordinary path (nil); any other row (a variable, a piped value, the call partially
// applied) is reported, since the row's type is not one a program can write, and the call is
// typed without it.
func (mc *modCtx) insertMineCall(x *syntax.App, v *syntax.Var, pipe *pipeArg, env *env, eff effect) types.Type {
	c := mc.c
	if c.insertMineRows == nil {
		c.insertMineRows = map[*syntax.Var]bool{}
	}
	c.insertMineRows[v] = true
	if _, lit := argAt(x.Args, 1).(*syntax.RecordLit); lit && pipe == nil && len(x.Args) == 2 {
		return nil
	}
	table, owner := "T", "its `@owner` column"
	if ct, ok := argAt(x.Args, 0).(*syntax.CtorRef); ok && ct.Ref.Kind == syntax.RefTable {
		table = ct.Ref.Name
		if info := c.tables[c.key(ct.Ref.Module, ct.Ref.Name)]; info != nil && info.Owner != "" {
			owner = "`" + info.Owner + "`"
		}
	}
	at := x.Span
	switch {
	case len(x.Args) >= 2:
		at = x.Args[1].GetSpan()
	case pipe != nil:
		at = pipe.span
	}
	c.reportScoped(at, fmt.Sprintf("`Db.insertMine` takes its row as a record literal: write the record literal in the `Db.insertMine` call (its row is `Insert %s` without %s)", table, owner))
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

// checkScopedTables reports E0025 for each scoped operation of the body just checked whose
// table has no `@owner` column.
func (c *Checker) checkScopedTables() {
	uses := c.scopedUses
	c.scopedUses = nil
	for _, u := range uses {
		fn, ok := c.u.Apply(u.t).(*types.Fun)
		if !ok || len(fn.Params) == 0 {
			continue
		}
		app, ok := c.u.Apply(fn.Params[0]).(*types.App)
		if !ok || app.Con != "Table" || len(app.Args) != 1 {
			continue
		}
		rec, ok := app.Args[0].(*types.Record)
		if !ok {
			continue // a type parameter or unknown table: checkDbTables reports it
		}
		if info := c.u.Table(rec); info != nil && info.Owner == "" {
			c.errorf(diag.NoOwner, u.v.Span, "`%s` has no `@owner` column, so `Db.%s %s` has nothing to scope by", rec.Name, u.v.Ref.Name, rec.Name)
		}
	}
}

// insertRecordTable is the table whose insert record rec is — `Insert T`, or Db.insertMine's
// `InsertMine T` (mine set) — or nil.
func (c *Checker) insertRecordTable(rec *types.Record) (info *types.TableInfo, mine bool) {
	if rec.Anonymous {
		return nil, false
	}
	if name, ok := strings.CutPrefix(rec.Name, "InsertMine "); ok {
		return c.tables[c.key(rec.Module, name)], true
	}
	if name, ok := strings.CutPrefix(rec.Name, "Insert "); ok {
		return c.tables[c.key(rec.Module, name)], false
	}
	return nil, false
}

// recordLabel is how a message names record rec: its type, but Db.insertMine's row (whose type
// a program never writes) as "Insert T` without `owner" (the message's backticks around it).
func (c *Checker) recordLabel(rec *types.Record) string {
	if info, mine := c.insertRecordTable(rec); mine && info != nil {
		return "Insert " + info.Name + "` without `" + info.Owner
	}
	return types.String(rec)
}

// recordLabelName is recordLabel with a declared record named without its module.
func (c *Checker) recordLabelName(rec *types.Record) string {
	if info, mine := c.insertRecordTable(rec); mine && info != nil {
		return c.recordLabel(rec)
	}
	return rec.Name
}
