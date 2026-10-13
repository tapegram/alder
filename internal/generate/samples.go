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
	ids     map[string]string // table module → the variable holding the id of a row of it
	counts  map[string]int    // rows of each table inserted so far
	me      string            // the signed-in user's id, "" before login
	today   bool              // `let today <- Date.today` was written
	taken   map[string]bool   // variable names in use
}

func newTestBuilder(p *Program, home string, imports *importSet) *testBuilder {
	return &testBuilder{p: p, home: home, imports: imports, ids: map[string]string{}, counts: map[string]int{}, taken: map[string]bool{}}
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
			b.me = b.insertAs(t, "me")
			return b.me
		}
	}
	elem, _ := unMaybe(s.User.Type)
	b.me = "(" + b.sample(elem, "me", 1) + ")"
	return b.me
}

// login signs the user in (Test.login), with role when it is not "".
func (b *testBuilder) login(role string) {
	me := b.user()
	if role != "" {
		b.typeRef(b.p.Session.Role.Type)
		b.let("let _ <- Test.login { user: Just %s, role: %s }", me, role)
		return
	}
	b.let("let _ <- Test.login { user: Just %s }", me)
}

// insert inserts a row of t with sample values, after the rows it refers to, and returns the
// variable holding its id ("_" when not named). The owner column is the signed-in user.
func (b *testBuilder) insert(t *Table, named bool) string {
	if named {
		return b.insertAs(t, "")
	}
	return b.insertAs(t, "_")
}

// insertAs is insert, the id in variable v ("" for a fresh name, "_" for none).
func (b *testBuilder) insertAs(t *Table, v string) string {
	b.imports.add(t.Module)
	b.counts[t.Module]++
	n := b.counts[t.Module]
	var fields []string
	for _, c := range t.Columns {
		if !c.Required() {
			continue
		}
		fields = append(fields, c.Name+": "+b.columnValue(t, c, n))
	}
	if v == "" {
		v = LowerFirst(t.Name) + fmt.Sprint(n)
	}
	if v != "_" {
		v = b.fresh(v)
		if b.ids[t.Module] == "" {
			b.ids[t.Module] = v
		}
	}
	b.let("let %s <- Db.insert %s { %s }", v, t.Name, strings.Join(fields, ", "))
	return v
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
		if v := b.ids[c.Ref]; v != "" {
			return v
		}
		if rt := b.p.TableNamed(c.Ref); rt != nil && rt != t {
			return b.insert(rt, true)
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
