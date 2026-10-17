// Partitions and silos (M15, book 06 "Partitions", 08): App's `via`, `silo` and `choose`
// declarations (E0032, E0033) and the M15 rules of a page's `requires` (a key with `via` after
// `user`, E0027; the admin form `silo k`, E0033). The table side (`@partition k`) is in
// rules.go; the one-store analysis (E0031) in writeset.go.
package check

import (
	"slices"
	"sort"
	"strings"

	"alder/internal/diag"
	"alder/internal/syntax"
	"alder/internal/types"
)

// sessionDecl is App's `session` declaration, nil when it has none.
func (c *Checker) sessionDecl() *syntax.SessionDecl {
	if app := c.prog.Modules["App"]; app != nil {
		for _, d := range c.live(app) {
			if sd, ok := d.(*syntax.SessionDecl); ok {
				return sd
			}
		}
	}
	return nil
}

// sessionVia is the `via` table named by App's session field k ("" for none, or no field).
func (c *Checker) sessionVia(k string) string {
	if sd := c.sessionDecl(); sd != nil {
		for _, f := range sd.Fields {
			if f.Name == k {
				return f.Via
			}
		}
	}
	return ""
}

// appTable is the key ("Org.Org") of the app (not fake) table called name: a table lives in the
// module named after it (E0018). "" when there is none.
func (c *Checker) appTable(name string) string {
	k := c.key(name, name)
	if c.tables[k] == nil {
		return ""
	}
	if _, fake := c.fakeTables[k]; fake {
		return ""
	}
	return k
}

// tableRef is one `@ref` of an app table: the field, its attribute, and the referenced table's
// key ("" when it names no table in scope: checkRef reports it).
type tableRef struct {
	table, field, target string
	attr                 *syntax.Attr
}

// appTableRefs lists every `@ref` of every app table (not a fake's), in module and declaration
// order.
func (c *Checker) appTableRefs() []tableRef {
	var out []tableRef
	for _, name := range c.prog.Order {
		m := c.prog.Modules[name]
		for _, d := range c.live(m) {
			td, ok := d.(*syntax.TableDecl)
			if !ok {
				continue
			}
			tk := c.key(m.Name, td.Name)
			if _, fake := c.fakeTables[tk]; fake || c.tables[tk] == nil {
				continue
			}
			for _, f := range td.Fields {
				for _, a := range f.Attrs {
					if a.Name != "ref" {
						continue
					}
					target := ""
					for _, cand := range append([]string{m.Name}, importNames(m)...) {
						if k := c.key(cand, a.Arg); c.tables[k] != nil {
							target = k
							break
						}
					}
					out = append(out, tableRef{table: tk, field: f.Name, target: target, attr: a})
				}
			}
		}
	}
	return out
}

func importNames(m *syntax.Module) []string {
	var out []string
	for _, im := range m.Imports {
		out = append(out, im.Module)
	}
	return out
}

// tableOfID is the key of the app table whose id type is t, "" for none.
func (c *Checker) tableOfID(t types.Type) string {
	keys := make([]string, 0, len(c.tables))
	for k := range c.tables {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		info := c.tables[k]
		if _, fake := c.fakeTables[k]; fake || info.IDType == nil || types.IsErr(info.IDType) {
			continue
		}
		if types.String(info.IDType) == types.String(t) && c.u.Unify(info.IDType, t) == nil {
			return k
		}
	}
	return ""
}

// checkPartitionDecls validates App's session fields' `via` and App's `silo` and `choose`
// declarations, once every table's partition is known (checkOwners), and records what is valid
// (c.via, c.silos, c.siloOf, c.choose; choose's page is checked again by checkChoose once pages
// are defined).
func (c *Checker) checkPartitionDecls() {
	app := c.prog.Modules["App"]
	if app == nil {
		return
	}
	c.checkVia()
	refs := c.appTableRefs()
	var silos []*syntax.SiloDecl
	for _, d := range c.live(app) {
		if sd, ok := d.(*syntax.SiloDecl); ok {
			silos = append(silos, sd)
		}
	}
	c.silos, c.siloOf = map[string][]string{}, map[string]string{}
	declared := map[string]bool{}
	for _, sd := range silos {
		if declared[sd.Key] {
			c.errorf(diag.SiloDecl, sd.KeySpan, "`silo %s` is declared twice: list all its tables in one", sd.Key)
			continue
		}
		declared[sd.Key] = true
		if _, ok := c.fieldMaybe(sd.Key); !ok {
			if _, has := c.sessionKey(sd.Key); !has {
				c.errorf(diag.SiloDecl, sd.KeySpan, "`silo %s`: the session has no field `%s`", sd.Key, sd.Key)
			} else {
				c.errorf(diag.SiloDecl, sd.KeySpan, "`silo %s`: session field `%s` must be a `Maybe`", sd.Key, sd.Key)
			}
			continue
		}
		listed := map[string]bool{}
		for i, name := range sd.Tables {
			at := sd.TableSpans[i]
			tk := c.appTable(name)
			switch {
			case tk == "":
				c.errorf(diag.SiloDecl, at, "`silo %s`: there is no table `%s`", sd.Key, name)
				continue
			case listed[tk]:
				c.errorf(diag.SiloDecl, at, "`%s` is listed twice in `silo %s`", name, sd.Key)
				continue
			case c.siloOf[tk] != "":
				c.errorf(diag.SiloDecl, at, "`%s` is in `silo %s` and `silo %s`: a table lives in one store", name, c.siloOf[tk], sd.Key)
				continue
			case c.tables[tk].PartKey != sd.Key:
				c.errorf(diag.SiloDecl, at, "`%s` is in `silo %s` but is not partitioned by `%s`", name, sd.Key, sd.Key)
				continue
			}
			listed[tk] = true
			c.siloOf[tk] = sd.Key
			c.silos[sd.Key] = append(c.silos[sd.Key], tk)
		}
	}
	for _, r := range refs {
		into := c.siloOf[r.target]
		if into == "" {
			continue
		}
		from := c.siloOf[r.table]
		field := strings.TrimPrefix(r.table, tableModule(r.table)+".") + "." + r.field
		target := strings.TrimPrefix(r.target, tableModule(r.target)+".")
		switch {
		case from == "":
			c.errorf(diag.SiloDecl, r.attr.Span, "`%s` refers to `%s`, which lives in `silo %s`: a shared table cannot point into a silo", field, target, into)
		case from != into:
			c.errorf(diag.SiloDecl, r.attr.Span, "`%s` refers to `%s`, which lives in `silo %s`: `%s` lives in `silo %s`, and a table cannot point into another silo",
				field, target, into, strings.TrimPrefix(r.table, tableModule(r.table)+"."), from)
		}
	}
	if len(c.silos) == 0 {
		c.silos, c.siloOf = nil, nil
	}
}

func tableModule(k string) string {
	if i := strings.LastIndex(k, "."); i >= 0 {
		return k[:i]
	}
	return k
}

// checkVia validates each session field's `via T` (E0032): a key other than `user`, a session
// with `user`, a field of type `Maybe` of a table's id, and a table T with exactly one `@ref` to
// the user's table and one to the key's.
func (c *Checker) checkVia() {
	sd := c.sessionDecl()
	if sd == nil {
		return
	}
	userT, hasUser := c.sessionUser()
	for _, f := range sd.Fields {
		if f.Via == "" {
			continue
		}
		at := f.ViaSpan
		if f.Name == "user" {
			c.errorf(diag.PartitionDecl, at, "`user` cannot be `via` a table: membership is checked against the signed-in user")
			continue
		}
		if !hasUser {
			c.errorf(diag.PartitionDecl, at, "`%s` is `via %s`, which needs a session field `user`: a membership is a user's", f.Name, f.Via)
			continue
		}
		mk := c.appTable(f.Via)
		if mk == "" {
			c.errorf(diag.PartitionDecl, at, "`via %s`: there is no table `%s`", f.Via, f.Via)
			continue
		}
		ft, isMaybe := c.fieldMaybe(f.Name)
		kt, _ := c.sessionKey(f.Name)
		keyTable := ""
		if isMaybe {
			keyTable = c.tableOfID(kt)
		}
		if keyTable == "" {
			c.errorf(diag.PartitionDecl, at, "`via %s` needs `%s` to be `Maybe` of a table's id (such as `Maybe Org.OrgId`), got `%s`", f.Via, f.Name, types.String(ft))
			continue
		}
		userTable := c.tableOfID(userT)
		nUser, nKey := 0, 0
		for _, r := range c.appTableRefs() {
			if r.table != mk {
				continue
			}
			if r.target == userTable {
				nUser++
			}
			if r.target == keyTable {
				nKey++
			}
		}
		if nUser != 1 || nKey != 1 || userTable == "" {
			un := strings.TrimPrefix(userTable, tableModule(userTable)+".")
			if un == "" {
				un = "User"
			}
			kn := strings.TrimPrefix(keyTable, tableModule(keyTable)+".")
			c.errorf(diag.PartitionDecl, at, "`via %s`: `%s` needs exactly one `@ref %s` column and one `@ref %s` column; it has %d and %d", f.Via, f.Via, un, kn, nUser, nKey)
			continue
		}
		if c.via == nil {
			c.via = map[string]string{}
		}
		c.via[f.Name] = mk
	}
}

// checkChoose validates App's `choose k = Page` declarations (E0032) once pages are defined, and
// that every key with `via` that a page `requires` has one.
func (c *Checker) checkChoose() {
	app := c.prog.Modules["App"]
	if app == nil {
		return
	}
	seen := map[string]bool{}
	for _, d := range c.live(app) {
		cd, ok := d.(*syntax.ChooseDecl)
		if !ok {
			continue
		}
		_, has := c.sessionKey(cd.Key)
		pg := c.pages[cd.Page]
		twice := seen[cd.Key]
		seen[cd.Key] = true
		switch {
		case twice:
			c.errorf(diag.PartitionDecl, cd.KeySpan, "`choose %s` is declared twice", cd.Key)
		case cd.Key == "user":
			c.errorf(diag.PartitionDecl, cd.KeySpan, "`choose user`: the signed-in user is chosen by `login`; `choose` names the page for another key")
		case !has:
			c.errorf(diag.PartitionDecl, cd.KeySpan, "`choose %s`: the session has no field `%s`", cd.Key, cd.Key)
		case pg == nil: // names reported the unknown page
		case slices.Contains(pg.Requires, cd.Key):
			c.errorf(diag.PartitionDecl, cd.Span, "`choose %s = %s`: `%s` `requires %s` itself, so a request without `%s` would be sent back to it", cd.Key, cd.Page, cd.Page, cd.Key, cd.Key)
		default:
			if c.choose == nil {
				c.choose = map[string]string{}
			}
			c.choose[cd.Key] = cd.Page
		}
	}
	sd := c.sessionDecl()
	if sd == nil {
		return
	}
	pages := make([]string, 0, len(c.pages))
	for name := range c.pages {
		pages = append(pages, name)
	}
	sort.Strings(pages)
	for _, f := range sd.Fields {
		if c.via[f.Name] == "" || c.choose[f.Name] != "" || c.chooseDeclared(f.Name) {
			continue
		}
		for _, name := range pages {
			if c.prog.Failed[name] || !slices.Contains(c.pages[name].Requires, f.Name) {
				continue
			}
			c.errorf(diag.PartitionDecl, f.ViaSpan, "`%s` is `via %s` and `%s` `requires %s`: `App` needs `choose %s = Page`, the page a request without `%s` is sent to (it picks one)", f.Name, f.Via, name, f.Name, f.Name, f.Name)
			break
		}
	}
}

// chooseDeclared reports whether App declares a `choose k` (valid or not).
func (c *Checker) chooseDeclared(k string) bool {
	for _, d := range c.live(c.prog.Modules["App"]) {
		if cd, ok := d.(*syntax.ChooseDecl); ok && cd.Key == k {
			return true
		}
	}
	return false
}

// checkRequiresM15 checks the M15 rules of page pd's `requires` once its fields are checked:
// a field named twice, a key with `via` before `user` (E0027), and the admin form `silo k`
// (E0033). It returns the fields the page's session narrows: the required ones and the silo key.
func (mc *modCtx) checkRequiresM15(pd *syntax.PageDecl, info *pageInfo) []string {
	c := mc.c
	seen := map[string]bool{}
	for i, req := range pd.Requires {
		at := pd.RequiresSpan
		if i > 0 && i < len(pd.FieldSpans) {
			at = pd.FieldSpans[i]
		}
		if seen[req] {
			c.errorf(diag.InvalidDecl, at, "`requires` names `%s` twice", req)
		}
		seen[req] = true
		if via := c.sessionVia(req); via != "" && req != "user" && !slices.Contains(pd.Requires[:i], "user") {
			c.errorf(diag.RoleClause, at, "`%s` needs `user` first: `%s` is `via %s`, a membership of the signed-in user; write `requires user, %s`", req, req, via, req)
		}
	}
	narrowed := slices.Clone(pd.Requires)
	if pd.Silo == "" {
		return narrowed
	}
	k, at := pd.Silo, pd.SiloSpan
	kt, has := c.sessionKey(k)
	_, isMaybe := c.fieldMaybe(k)
	var param *types.Field
	for i := range info.Params.Fields {
		if info.Params.Fields[i].Name == k {
			param = &info.Params.Fields[i]
		}
	}
	switch {
	case len(pd.Roles) == 0:
		c.errorf(diag.SiloDecl, at, "`silo %s` needs a role check: the page takes `%s` from its parameter, which only the role check guards; write `requires user, role Admin, silo %s`", k, k, k)
	case !has:
		c.errorf(diag.SiloDecl, at, "`silo %s`: the session has no field `%s`", k, k)
	case !isMaybe:
		c.errorf(diag.SiloDecl, at, "`silo %s`: session field `%s` must be a `Maybe`", k, k)
	case seen[k]:
		c.errorf(diag.SiloDecl, at, "`silo %s` and `requires %s` both name `%s`: the admin form takes `%s` from the page's parameter, not the session", k, k, k, k)
	case param == nil:
		c.errorf(diag.SiloDecl, at, "`silo %s` takes `%s` from the page's parameter: add `%s: %s` to the page's parameters", k, k, k, types.StringIn(kt, mc.m.Name))
	case types.IsErr(param.Type):
	case c.u.Unify(param.Type, kt) != nil:
		c.errorf(diag.SiloDecl, at, "the page's parameter `%s` is `%s`; `silo %s` needs `%s`, the type of the session's `%s`", k, types.StringIn(param.Type, mc.m.Name), k, types.StringIn(kt, mc.m.Name), k)
	default:
		info.Silo = k
		return append(narrowed, k)
	}
	return narrowed
}

// m15Placement reports `silo` and `choose` outside App (InvalidDecl, like `login`).
func (c *Checker) m15Placement(d syntax.Decl) {
	switch d := d.(type) {
	case *syntax.SiloDecl:
		c.errorf(diag.InvalidDecl, d.Span, "`silo` is only allowed in `App`")
	case *syntax.ChooseDecl:
		c.errorf(diag.InvalidDecl, d.Span, "`choose` is only allowed in `App`")
	}
}
