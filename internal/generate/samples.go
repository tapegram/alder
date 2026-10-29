package generate

import (
	"fmt"
	"strings"

	"alder/internal/types"
)

// testBuilder writes the setup of a generated test in module home: the rows it inserts (each
// with sample values, and the rows they refer to first), the login, the page value it opens.
// Its lines are the test block's statements, in order.
type testBuilder struct {
	p       *Program
	home    string
	imports *importSet
	lines   []string
	ids     map[string]string   // table module → the variable holding the id of a row of it
	counts  map[string]int      // rows of each table inserted so far
	rows    map[string][]rowRef // table module → its named rows, in insertion order
	bulk    map[string]int      // table module → rows inserted by bulkInsert for the signed-in user (or ownerless)
	me      string              // the signed-in user's id, "" before login
	other   string              // another user's id, "" until otherUser
	today   bool                // `let today <- Date.today` was written
	meRole  string              // the role the signed-in user's row is inserted with ("" for its default)
	taken   map[string]bool     // variable names in use
	silo    *siloCtx            // a test of an admin page in a silo: its key's row; nil otherwise
}

// siloCtx is a test of a page in `silo key`: the row of the key's table every row of the test
// lives under (v, inserted first), and whether the signed-in Admin is a member of it (logged in
// with the key set, so the test's own reads go to that silo) or not (the key unset: only the
// pages, which take it from their parameter, read the silo).
type siloCtx struct {
	key    string
	table  *Table
	member bool
	admin  bool // the key is set for an Admin only (the admin's pages; another role's tests are 404s)
	v      string
}

// keyedFor makes the builder's login set the key page pg requires besides `user` (an `org`):
// a row of the key's table, the signed-in user a member of it when the key is `via` a table.
func (b *testBuilder) keyedFor(pg *Page) *testBuilder {
	for _, k := range pg.Keys {
		if k == "user" {
			continue
		}
		if kt := b.p.KeyTable(k); kt != nil {
			b.silo = &siloCtx{key: k, table: kt, member: true}
		}
	}
	return b
}

// keyRow is the variable holding the silo's key row, inserted the first time.
func (b *testBuilder) keyRow() string {
	if b.silo.v == "" {
		b.silo.v = b.insertRow(b.silo.table, b.silo.key, nil, true)
	}
	return b.silo.v
}

// rowRef is a named row a test inserted, and its owner's variable (its partition column's:
// the user of an `@owner`, the org of an `@partition org`; "" for a table without one).
type rowRef struct{ v, owner string }

func newTestBuilder(p *Program, home string, imports *importSet) *testBuilder {
	return &testBuilder{p: p, home: home, imports: imports, ids: map[string]string{}, counts: map[string]int{},
		rows: map[string][]rowRef{}, bulk: map[string]int{}, taken: map[string]bool{}}
}

// let adds a statement.
func (b *testBuilder) let(format string, args ...any) {
	b.lines = append(b.lines, fmt.Sprintf(format, args...))
}

// fresh is a variable name based on base that is not yet taken.
func (b *testBuilder) fresh(base string) string {
	name := base
	for i := 2; b.taken[name]; i++ {
		name = fmt.Sprintf("%s%d", base, i)
	}
	b.taken[name] = true
	return name
}

// user is the id of a user row for the session's `user`: the signed-in user's when there is
// one, else a row inserted for it (or, when the session's user is not a table's id, a branded
// text).
func (b *testBuilder) user() string {
	if b.me != "" {
		return b.me
	}
	s := b.p.Session
	if s != nil && s.UserTable != "" {
		if t := b.p.TableNamed(s.UserTable); t != nil {
			var over map[string]string
			if rc := b.p.RoleColumn(); rc != nil && b.meRole != "" { // the row says the role the login gives
				over = map[string]string{rc.Name: b.meRole}
			}
			b.me = b.insertRow(t, "me", over, true)
			return b.me
		}
	}
	elem, _ := unMaybe(s.User.Type)
	b.me = "(" + b.sample(elem, "me", 1) + ")"
	return b.me
}

// otherUser is the id of a second user, a stranger to the signed-in one.
func (b *testBuilder) otherUser() string {
	if b.other != "" {
		return b.other
	}
	b.user()
	s := b.p.Session
	if t := b.p.TableNamed(s.UserTable); t != nil {
		b.other = b.insertRow(t, "other", nil, false)
		return b.other
	}
	elem, _ := unMaybe(s.User.Type)
	b.other = "(" + b.sample(elem, "other", 1) + ")"
	return b.other
}

// login signs the user in (Test.login), with role when it is not "". A user row inserted for it
// has that role too (the row a page may read again), unless meRole names another.
func (b *testBuilder) login(role string) {
	if b.me == "" && b.meRole == "" {
		b.meRole = role
	}
	me := b.user()
	fields := []string{"user: Just " + me}
	for _, f := range b.p.Session.Fields { // a `Maybe` without a default (an `org`) is written: Nothing
		if _, isMaybe := unMaybe(f.Type); !isMaybe || f.HasDefault || f.Name == "user" || f.Name == "role" {
			continue
		}
		if b.silo != nil && b.silo.member && f.Name == b.silo.key && (role == "Admin" || !b.silo.admin) {
			k := b.keyRow()
			if b.p.Via[f.Name] != "" { // `via`: the session's org must be one of the user's
				b.let("let _ <- Test.member %s %s", me, k)
			}
			fields = append(fields, f.Name+": Just "+k)
			continue
		}
		fields = append(fields, f.Name+": Nothing")
	}
	if role != "" {
		b.typeRef(b.p.Session.Role.Type)
		fields = append(fields, "role: "+role)
	}
	b.let("let _ <- Test.login { %s }", strings.Join(fields, ", "))
}

// insert inserts a row of t with sample values, after the rows it refers to, and returns the
// variable holding its id ("_" when not named). The owner column is the signed-in user.
func (b *testBuilder) insert(t *Table, named bool) string {
	if named {
		return b.insertRow(t, "", nil, true)
	}
	return b.insertRow(t, "_", nil, true)
}

// insertRow inserts a row of t into variable v ("" for a fresh name after the table, "_" for
// none), its columns over's values where over has one, else sample values (a reference: the
// row memo keeps for its table, else one inserted now). With memo, a named row becomes the one
// later references to t use, unless one was named before.
func (b *testBuilder) insertRow(t *Table, v string, over map[string]string, memo bool) string {
	b.imports.add(t.Module)
	b.counts[t.Module]++
	n := b.counts[t.Module]
	var fields []string
	owner := ""
	for _, c := range t.Columns {
		val, ok := over[c.Name]
		if !ok && !c.Required() {
			continue
		}
		if !ok {
			val = b.columnValue(t, c, n)
		}
		if c.Owner || c.Name == t.PartCol && t.PartCol != "" {
			owner = val
		}
		fields = append(fields, c.Name+": "+val)
	}
	if v == "" {
		v = LowerFirst(t.Name) + fmt.Sprint(n)
	}
	if v != "_" {
		v = b.fresh(v)
		if memo && b.ids[t.Module] == "" {
			b.ids[t.Module] = v
		}
		b.rows[t.Module] = append(b.rows[t.Module], rowRef{v, owner})
	}
	b.let("let %s <- Db.insert %s { %s }", v, recordName(b.p, t, b.home, b.imports), strings.Join(fields, ", "))
	return v
}

// bulkInsert inserts k rows of t in one step (`Task.all` over `List.range`), each with sample
// values numbered by the row, after the rows of t inserted before (so `@unique` columns
// differ), the owner owner when t has one (the signed-in user for ""), every reference the row
// memo keeps.
func (b *testBuilder) bulkInsert(t *Table, k int, owner string) {
	b.imports.add(t.Module)
	from, to := b.counts[t.Module]+1, b.counts[t.Module]+k
	var fields []string
	for _, c := range t.Columns {
		if !c.Required() {
			continue
		}
		var val string
		switch {
		case c.Owner && owner != "":
			val = owner
		case c.Ref != "":
			val = b.columnValue(t, c, from)
		default:
			val = b.sampleN(c.Type, c.Name)
		}
		fields = append(fields, c.Name+": "+val)
	}
	b.counts[t.Module] += k
	if owner == "" || owner == b.me {
		b.bulk[t.Module] += k
	}
	b.let("let _ <- Task.all (List.map (\\n -> Db.insert %s { %s }) (List.range %d %d))", recordName(b.p, t, b.home, b.imports), strings.Join(fields, ", "), from, to)
}

// visible are the named rows of t the signed-in user sees: all of them, or with mine (a page
// reading t with Db.mine) those the user owns; newest first.
func (b *testBuilder) visible(t *Table, mine bool) []string {
	var out []string
	rows := b.rows[t.Module]
	want := b.me
	if t.PartKey != "" && t.PartKey != "user" && b.silo != nil { // Db.scoped: the key's rows
		want = b.silo.v
	}
	for i := len(rows) - 1; i >= 0; i-- {
		if !mine || rows[i].owner == want {
			out = append(out, rows[i].v)
		}
	}
	return out
}

// sampleN is a sample value of type t for a field named field, an expression of the row number
// `n` (bulkInsert's), distinct for distinct rows where the type allows.
func (b *testBuilder) sampleN(t types.Type, field string) string {
	text := quote(strings.ToLower(field)+"-") + " ++ Text.fromInt n"
	switch x := t.(type) {
	case *types.Con:
		switch x.Name {
		case "Text":
			return text
		case "Int":
			return "n"
		case "Float":
			return "Int.toFloat n"
		}
	case *types.Brand:
		switch x.Module + "." + x.Name {
		case "Text.Email":
			return b.brandCtor(x) + " (" + text + " ++ \"@example.com\")"
		case "Text.Url":
			return b.brandCtor(x) + " (\"https://example.com/\" ++ " + text + ")"
		}
		if k, _, _ := kindOf(x); k == KindText || k == KindInt {
			if inner := b.sampleN(x.Base, field); inner != "" {
				return b.brandCtor(x) + " (" + inner + ")"
			}
		}
	}
	return b.sample(t, field, 1)
}

// columnValue is a sample value for column c of the n-th inserted row of t.
func (b *testBuilder) columnValue(t *Table, c *Column, n int) string {
	if c.Maybe {
		return "Nothing"
	}
	if c.Ref != "" {
		if c.Owner || (b.p.Session != nil && c.Ref == b.p.Session.UserTable) {
			return b.user()
		}
		if rt := b.p.TableNamed(c.Ref); rt != nil {
			if v := b.ids[rt.Module]; v != "" {
				return v
			}
			if rt != t {
				return b.insert(rt, true)
			}
		}
	}
	return b.sample(c.Type, c.Name, n)
}

// sample is a sample value of type t for a field named field, the n-th of its kind: distinct
// for distinct n, so a `@unique` column takes two rows. A table's id is a row's: the one
// inserted already, else one inserted now.
func (b *testBuilder) sample(t types.Type, field string, n int) string {
	if _, isMaybe := unMaybe(t); isMaybe {
		return "Nothing"
	}
	if br, ok := t.(*types.Brand); ok {
		for _, tbl := range b.p.Tables {
			if types.String(br) == types.String(tbl.IDType) { // a row's id: a row of that table
				if v := b.ids[tbl.Module]; v != "" {
					return v
				}
				return b.insert(tbl, true)
			}
		}
	}
	text := strings.ToLower(field) + "-" + fmt.Sprint(n)
	switch x := t.(type) {
	case *types.Con:
		switch x.Name {
		case "Text":
			return quote(text)
		case "Int":
			return fmt.Sprint(n)
		case "Float":
			return fmt.Sprintf("%d.5", n)
		case "Bool":
			return "True"
		case "Date":
			if !b.today {
				b.today = true
				b.taken["today"] = true
				b.lines = append([]string{"let today <- Date.today"}, b.lines...)
			}
			return "today"
		case "Time":
			return "Time.epoch"
		}
	case *types.Brand:
		var base string
		switch x.Module + "." + x.Name {
		case "Text.Email":
			base = quote(text + "@example.com")
		case "Text.Url":
			base = quote("https://example.com/" + text)
		default:
			base = b.sample(x.Base, field, n)
		}
		return b.brandCtor(x) + " " + base
	case *types.Union:
		if ctors := x.Constructors(); len(ctors) > 0 {
			b.imports.add(x.Module)
			return ctors[0].Name
		}
	}
	return "?" + field
}

// rawSample is valid form input for a field of type t, as text (Test.submitRaw).
func rawSample(t types.Type) string {
	if _, isMaybe := unMaybe(t); isMaybe {
		return ""
	}
	switch x := t.(type) {
	case *types.Con:
		switch x.Name {
		case "Int":
			return "1"
		case "Float":
			return "1.5"
		case "Bool":
			return "True"
		case "Date":
			return "2026-01-01"
		}
		return "x"
	case *types.Brand:
		switch x.Module + "." + x.Name {
		case "Text.Email":
			return "x@example.com"
		case "Text.Url":
			return "https://example.com"
		}
		return rawSample(x.Base)
	case *types.Union:
		if ctors := x.Constructors(); len(ctors) > 0 {
			return ctors[0].Name
		}
	}
	return "x"
}

// emptyMessage is the error a form shows for an empty input of a field of type t, or "" when
// an empty input is accepted (plain Text, a Maybe, a Bool) or its message is the app's own (a
// brand's `where`): the decoders' texts (rt/web).
func emptyMessage(t types.Type) string {
	if _, isMaybe := unMaybe(t); isMaybe {
		return ""
	}
	switch x := t.(type) {
	case *types.Con:
		switch x.Name {
		case "Int", "Float", "Date":
			return "is required"
		}
	case *types.Brand:
		switch x.Module + "." + x.Name {
		case "Text.NonEmpty", "Text.Secret":
			return "must not be empty"
		case "Text.Email":
			return "must be an email address"
		case "Text.Url":
			return "must be a URL"
		}
	case *types.Union:
		return "is required"
	}
	return ""
}

// brandCtor is brand x's constructor as written in the home module ("Text.NonEmpty",
// "Post.Slug", "Slug" in Post), importing its module.
func (b *testBuilder) brandCtor(x *types.Brand) string {
	b.typeRef(x)
	return types.StringIn(x, b.home)
}

// typeRef imports the app modules type t names.
func (b *testBuilder) typeRef(t types.Type) { importTypes(b.imports, t) }

// importTypes adds to s every app module type t names.
func importTypes(s *importSet, t types.Type) {
	switch x := t.(type) {
	case *types.Brand:
		s.add(x.Module)
	case *types.Union:
		s.add(x.Module)
		for _, a := range x.Args {
			importTypes(s, a)
		}
	case *types.Record:
		if !x.Anonymous {
			s.add(x.Module)
		}
		for _, a := range x.Args {
			importTypes(s, a)
		}
	case *types.App:
		for _, a := range x.Args {
			importTypes(s, a)
		}
	case *types.Fun:
		for _, a := range x.Params {
			importTypes(s, a)
		}
		importTypes(s, x.Result)
	}
}

// pageValue is the page value of pg for a test, its required parameters filled with sample
// values (the id of a row, inserted, for a table's id).
func (b *testBuilder) pageValue(pg *Page) string {
	var fields []string
	for _, par := range pg.Params {
		if par.HasDefault {
			continue
		}
		fields = append(fields, par.Name+": "+b.sample(par.Type, par.Name, 1))
	}
	return pageLit(pg.Module, fields)
}

// pageLit is a page value: `M {}` or `M { a: x, b: y }`.
func pageLit(module string, fields []string) string {
	if len(fields) == 0 {
		return module + " {}"
	}
	return module + " { " + strings.Join(fields, ", ") + " }"
}
