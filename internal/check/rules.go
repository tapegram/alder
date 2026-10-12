// Declared rules (M11): `@owner` columns (E0024), the scoped Db operations (E0025, E0026, the
// row of Db.insertMine) and a page's `requires user, role R` (E0027).
package check

import (
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
			switch d := d.(type) {
			case *syntax.TableDecl:
				c.checkOwner(m, d)
			case *syntax.FakeDecl:
				for _, td := range d.Tables {
					c.checkOwner(m, td)
				}
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
	if !ok || un.Name == "Maybe" && un.Module == "Prelude" {
		if !types.IsErr(role.Type) {
			c.errorf(diag.RoleClause, pd.RolesSpan, "this session's `role` field is `%s`, not a union: write `role: Role = …` with `type Role = Reader | Author`", types.String(role.Type))
		}
		return
	}
	ctors := map[string]bool{}
	for _, ct := range un.Constructors() {
		ctors[ct.Name] = true
	}
	bad := false
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
// the module is not one where `session.user` is narrowed, and notes the use for E0025.
func (mc *modCtx) inferScoped(x *syntax.Var) types.Type {
	c := mc.c
	if why := mc.scopedOutside(); why != "" && !c.scopedReported[x.Span] {
		if c.scopedReported == nil {
			c.scopedReported = map[diag.Span]bool{}
		}
		c.scopedReported[x.Span] = true
		c.errorf(diag.ScopedOutside, x.Span, "`Db.%s` needs a page that `requires user`: %s", x.Ref.Name, why)
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

// scopedOutside says why a scoped operation may not be used in mc's module, "" when it may: a
// page module whose page `requires user` (any of its values; not a test).
func (mc *modCtx) scopedOutside() string {
	switch {
	case mc.inTest:
		return "a `test` has no session; reach the page through `Test.open` or `Test.click`"
	case mc.page != nil && mc.page.Requires == "user":
		return ""
	case mc.page != nil:
		return "`" + mc.m.Name + "` does not"
	}
	return "`" + mc.m.Name + "` is not a page"
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
