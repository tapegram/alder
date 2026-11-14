package generate

import (
	"os"
	"path/filepath"
	"slices"
	"strings"

	"alder/internal/names"
	"alder/internal/types"
)

// ResourceOptions are the options of `alder new resource` and `alder regen resource`.
type ResourceOptions struct {
	Table    string
	Fields   []FieldSpec // create the table first when given
	Read     string      // "owner" | "signed-in" | "all"; "" = default
	Requires string      // as given; "" = default
	Module   string      // "" = PluralConcern(Table)
	Path     string      // "" = "/" + names.Collection(Table)

	explicitRequires bool // set when --requires was given: the header records it
}

// ResourceDir is the directory of the modules of the resource in module: "app/Posts".
func ResourceDir(module string) string {
	return "app/" + strings.ReplaceAll(module, ".", "/")
}

// resourceCheck fills the defaults and refuses what spec §1 refuses; it returns the table and
// the route keys (routeGuard's). A table that does not exist yet (its fields are given) comes
// back nil, with the module and path filled: Read and Requires wait for the table, so the check
// is made again on the program that has it.
func resourceCheck(p *Program, o ResourceOptions) (ResourceOptions, *Table, []string, error) {
	if o.Module == "" {
		o.Module = PluralConcern(o.Table)
	}
	if o.Path == "" {
		o.Path = "/" + names.Collection(o.Table)
	}
	var err error
	if o.Module, o.Path, err = pageOptionsCheck(o.Module, o.Path, "Posts", "/posts"); err != nil {
		return o, nil, nil, err
	}
	if o.Read != "" && !slices.Contains([]string{"owner", "signed-in", "all"}, o.Read) {
		return o, nil, nil, Refusef("`--read %s` is not `owner`, `signed-in` or `all`", o.Read)
	}
	t := p.TableNamed(o.Table)
	switch {
	case t == nil && len(o.Fields) == 0:
		return o, nil, nil, Refusef("there is no table `%s`: give its fields to create it (`alder new resource %s title:Text`) or make it first with `alder new table`", o.Table, o.Table)
	case t == nil:
		return o, nil, nil, nil
	case len(o.Fields) > 0:
		return o, nil, nil, Refusef("`%s` exists: change it with `alder edit table %s add-field …`, then `alder new resource %s`", t.Name, t.Name, t.Name)
	case t.Silo != "":
		return o, nil, nil, Refusef("`%s` lives in the `%s` silo: `alder new resource` writes pages over the shared store only (the admin shows siloed tables)", t.Name, t.Silo)
	}
	dir := ResourceDir(o.Module)
	if _, err := os.Stat(filepath.Join(p.Dir, filepath.FromSlash(dir))); err == nil {
		return o, nil, nil, Refusef("`%s/` exists: `alder regen resource %s` updates it; to generate another, choose another `--module`", dir, t.Name)
	}
	if err := secretRefusal(t); err != nil {
		return o, nil, nil, err
	}
	hasUser := p.Session != nil && p.Session.User != nil
	if hasUser {
		for _, c := range t.Columns {
			if c.Ref != "" && c.Ref == p.Session.UserTable && !c.Owner && c.Name != t.PartCol && c.Required() && !c.Maybe {
				return o, nil, nil, Refusef("`%s` is required and refers to users: give it a default or make it `Maybe`", c.Name)
			}
		}
	}

	owned := t.Owner != ""
	partitioned := t.PartKey != "" && !owned
	switch {
	case o.Read == "":
		switch {
		case owned || partitioned:
			o.Read = "owner"
		case hasUser:
			o.Read = "signed-in"
		default:
			o.Read = "all"
		}
	case o.Read == "owner" && !owned && !partitioned:
		return o, nil, nil, Refusef("`%s` has no `@owner` column to read by; add one (`alder edit table %s add-field %s`) or use `--read signed-in`", t.Name, t.Name, ownerField(p))
	case o.Read != "owner" && partitioned:
		return o, nil, nil, Refusef("rows of `%s` are partitioned by `%s` and read within it; `--read` applies to `@owner` tables", t.Name, t.PartKey)
	case o.Read == "all" && owned && !hasUser:
		return o, nil, nil, Refusef("`%s` is owned (`@owner`): its pages need `--requires user`", t.Name)
	}

	if o.Read == "all" { // labelsOf reads the rows referred to with Db.mine (Db.scoped), which a visitor's route cannot
		for _, c := range t.Columns {
			if rt := p.TableNamed(c.Ref); c.Ref != "" && rt != nil && rt.PartKey != "" {
				return o, nil, nil, Refusef("`%s` refers to `%s`, whose rows are each %s's: pages a visitor reads cannot show them; use `--read signed-in`", t.Name, rt.Name, rt.PartKey)
			}
		}
	}
	o.explicitRequires = o.Requires != ""
	ro := RouteOptions{Requires: o.Requires}
	if !o.explicitRequires {
		switch {
		case t.PartKey != "":
			ro.Requires = t.PartKey
		case hasUser:
			ro.Requires = "user"
		}
	}
	keys, _, requires, err := routeGuard(p, ro)
	if err != nil {
		return o, nil, nil, err
	}
	if t.PartKey != "" && !slices.Contains(keys, t.PartKey) {
		if owned {
			return o, nil, nil, Refusef("`%s` is owned (`@owner`): its pages need `--requires user`", t.Name)
		}
		return o, nil, nil, Refusef("`%s` is partitioned by `%s`: its pages need `--requires %s`", t.Name, t.PartKey, t.PartKey)
	}
	if !o.explicitRequires {
		o.Requires = requires
	}
	return o, t, keys, nil
}

// command is the `alder new resource` line that makes the resource of o (its options filled by
// resourceCheck): the header of its files records it, for `alder regen resource`. `--read` is
// always written; `--requires` only when it was given.
func (o ResourceOptions) command() string {
	c := "alder new resource " + o.Table + " --module " + o.Module + " --path " + o.Path + " --read " + o.Read
	if o.explicitRequires {
		c += " --requires " + o.Requires
	}
	return c
}

// resourceAccess is the access of the pages of table t (spec §3): for an `@owner` table read by
// its owner, every read and write scoped to the signed-in user (Db.mine, getMine, insertMine,
// updateMine, deleteMine), every route requiring o's keys. Another user's row is a missing row
// to every route, without a line of ownership code. Under `--read signed-in` or `all`, Index and
// Show read every user's rows (Db.from, Db.get), with no `requires` under `all`, and show the
// links to the writes to the owner only; the writes stay the owner's, so another user's row is
// still a missing one to them. A `@partition k` table's pages are scoped to the session's k the
// same way (Db.scoped, getScoped, insertScoped, updateScoped, deleteScoped): another k's row is
// a missing one. A keyless table's pages read and write every row (Db.from, Db.get, Db.insert,
// Db.update, Db.delete), a missing row matched to a 404; NewResource warns of it.
func resourceAccess(p *Program, t *Table, o ResourceOptions) access {
	req := ""
	if o.Requires != "" {
		req = " requires " + o.Requires
	}
	keys, _, _, _ := routeGuard(p, RouteOptions{Requires: o.Requires})
	a := access{
		marker: "@resource",
		header: "-- generated by `" + o.command() + "`; regions between\n" +
			"-- `-- @resource begin <name>` / `-- @resource end` are rewritten by `alder regen resource`",
		layout:        "Layout.Page",
		layoutImport:  "Layout",
		readRequires:  req,
		writeRequires: req,
		sel:           "Db.mine",
		get:           "Db.getMine",
		writeGet:      "Db.getMine",
		insert:        "Db.insertMine",
		update:        "Db.updateMine",
		remove:        "Db.deleteMine",
		perPage:       20,
		keys:          keys,
	}
	switch {
	case t.Owner == "" && t.PartKey != "":
		a.sel, a.get, a.writeGet = "Db.scoped", "Db.getScoped", "Db.getScoped"
		a.insert, a.update, a.remove = "Db.insertScoped", "Db.updateScoped", "Db.deleteScoped"
		a.scope = t.PartKey
		return a
	case t.Owner == "":
		a.sel, a.get, a.writeGet = "Db.from", "Db.get", "Db.get"
		a.insert, a.update, a.remove = "Db.insert", "Db.update", "Db.delete"
		if o.Read == "all" {
			a.readRequires = ""
		}
		return a
	}
	if o.Read == "signed-in" || o.Read == "all" {
		a.sel, a.get = "Db.from", "Db.get"
		a.ownerOnly = "session.user == row." + t.Owner
		a.readNote = "-- unscoped: `--read " + o.Read + "` shows every user's rows (the writes are the owner's)"
		if o.Read == "all" { // Index and Show require nothing: session.user is a Maybe there
			a.readRequires = ""
			a.ownerOnly = "session.user == Just row." + t.Owner
		}
	}
	return a
}

// ResourceFiles is every file of the resource of o (its options filled by resourceCheck), by
// path, and the paths in the order they are written: its Index, Show, New, Edit and Delete.
func ResourceFiles(p *Program, o ResourceOptions) (map[string]string, []string) {
	t := p.TableNamed(o.Table)
	g := &crudGen{p: p, a: resourceAccess(p, t, o), files: map[string]string{}, names: map[string][]string{}}
	at := g.table(t, o.Module, o.Path)
	// the owner is the signed-in user (insertMine fills it, updateMine never writes it), and a
	// form never lists the users for another reference to them
	userRef := func(c *Column) bool {
		return c.Name == t.PartCol || (p.Session != nil && c.Ref != "" && c.Ref == p.Session.UserTable)
	}
	for _, c := range at.editable {
		if userRef(c) && !c.Owner && c.Name != t.PartCol && c.Maybe && !c.HasDefault { // resourceCheck refused it required
			at.unset = append(at.unset, c)
		}
	}
	at.editable = slices.DeleteFunc(at.editable, userRef)
	at.refs = slices.DeleteFunc(at.refs, userRef)
	g.tables = []*crudTable{at}
	g.index(at)
	g.show(at)
	g.newPage(at)
	g.edit(at)
	g.delete(at)
	return g.files, g.order
}

// NewResource generates `alder new resource`: the five files of the resource, created; and the
// warnings and notices for standard error. It refuses what resourceCheck refuses, and a route
// of the program that answers one of the resource's methods and paths.
func NewResource(p *Program, o ResourceOptions) ([]Edit, []string, error) {
	o, t, _, err := resourceCheck(p, o)
	if err != nil {
		return nil, nil, err
	}
	if t == nil { // the table first, as `alder new table` makes it; the resource from the program that has it
		table, err := NewTableWith(p, o.Table, o.Fields, TableOptions{})
		if err != nil {
			return nil, nil, err
		}
		p2, err := Probe(p.Dir, table)
		if err != nil {
			return nil, nil, err
		}
		o.Fields = nil
		pages, notes, err := NewResource(p2, o)
		if err != nil {
			return nil, nil, err
		}
		return append(table, pages...), notes, nil
	}
	for _, r := range []struct{ kind, name, method, path string }{
		{"Index", "Index", "GET", o.Path},
		{"Show", "Show", "GET", o.Path + "/{id}"},
		{"New", "New", "GET", o.Path + "/new"},
		{"New", "Create", "POST", o.Path},
		{"Edit", "Edit", "GET", o.Path + "/{id}/edit"},
		{"Edit", "Save", "PUT", o.Path + "/{id}"},
		{"Delete", "Delete", "GET", o.Path + "/{id}/delete"},
		{"Delete", "Destroy", "DELETE", o.Path + "/{id}"},
	} {
		if have := routeClash(p, r.method, r.path, ""); have != nil {
			return nil, nil, resourceClashRefusal(have, r.method, r.path)
		}
	}
	files, order := ResourceFiles(p, o)
	var edits []Edit
	for _, path := range order {
		edits = append(edits, Edit{Path: path, Create: true, Body: files[path]})
	}
	var notes []string
	if t.Owner == "" && t.PartKey == "" {
		notes = append(notes, keylessWarning(p, t, o))
	}
	ut := userTable(p)
	for _, c := range t.Columns {
		if ut == nil || c.Ref != ut.Name {
			continue
		}
		// the form never lists the users: a user other than the owner is set by hand
		if !c.Owner && c.Name != t.PartCol {
			notes = append(notes, "`"+c.Name+"` refers to users and was left out of the form; add its input by hand")
		}
		// the pages never name a user by an email (crudGen.labelOf): with nothing else to name
		// one by, a column referring to users is left off them
		if userLabel(ut) == nil {
			notes = append(notes, "`"+c.Name+"` refers to users, whom only an email names: it was left off the list and the page; add a column such as `name: Text` to `"+ut.Name+"` to show it")
		}
	}
	return edits, notes, nil
}

// keylessWarning is the warning of the pages of t, a table with no `@owner` and no
// `@partition` (spec §3): who can read and change every row, and how to scope it.
func keylessWarning(p *Program, t *Table, o ResourceOptions) string {
	who, remedy := "every signed-in user can read and change every row", ""
	switch {
	case userTable(p) == nil:
		who, remedy = "any visitor can read and change every row", "alder new auth, then "
	case o.Read == "all":
		who = "any visitor can read every row and every signed-in user can change it"
	}
	return "`" + t.Name + "` has no `@owner`: " + who + ". To scope it: " + remedy +
		"alder edit table " + t.Name + " add-field " + ownerField(p) + ", then alder regen resource " + t.Name
}

// ownerField is the `alder edit table … add-field` word of an `@owner` column referring to the
// session's user table: `owner:User.UserId@ref=User@owner` (what `alder new auth` makes, when
// the app has no users yet).
func ownerField(p *Program) string {
	users, id := "User", "UserId"
	if ut := userTable(p); ut != nil {
		users = ut.Name
		if b, ok := ut.IDType.(*types.Brand); ok {
			id = b.Name
		}
	}
	return "owner:" + users + "." + id + "@ref=" + users + "@owner"
}

// userTable is the session's user table, nil when the app has none.
func userTable(p *Program) *Table {
	if p.Session == nil || p.Session.User == nil {
		return nil
	}
	return p.TableNamed(p.Session.UserTable)
}
