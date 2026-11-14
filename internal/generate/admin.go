package generate

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"alder/internal/check"
	"alder/internal/scaffold"
	"alder/internal/types"
)

// The built-in admin (`alder new admin`, `alder regen admin`; book chapter 15): for every table
// T, route modules under the admin's module (`Admin.Ts.Index`, `.Show`, `.New`, `.Edit`,
// `.Delete`, `.Bulk`), a dashboard (`Admin.Index`) and a layout (`Admin.Layout.Page`, the app's
// layout around a nav of every table). Each module's GET route takes the module's last segment
// (`Admin.Posts.Show.Show { id }`, #160); its writes are routes of their own, named for what
// they do: New's `Create` (POST with the form, redirecting to the row), Edit's `Save` (PUT,
// redirecting), Delete's `Destroy` (DELETE, to the list with a success toast; Show's dialog posts
// it too), the bulk page's `BulkDelete` and `BulkSet…` (POST, answering refresh). Every route
// `requires user, role Admin` and reads and writes with the unscoped `Db.*`: an admin acts on
// everyone's rows, and the role on the `route` line is the rule (chapter 6, "When to use the
// unscoped forms"); every `load` and `update` asks the layout's isAdmin too. Each module carries
// its browser tests, which kill the mutants of its code (`alder test --mutate`).
//
// The parts of a file that follow the schema are owned regions (regions.go): `alder regen
// admin` regenerates every file and writes back only their regions, so what the author changed
// outside them stays.

// AdminOptions are the options of `alder new admin` and `alder regen admin`.
type AdminOptions struct {
	Module string // the admin's module prefix: "Admin"
	Path   string // its path: "/admin"
}

// AdminNeeds begins the refusal of `alder new admin` (and `regen`) in an app whose session has
// no role with an `Admin` constructor; adminNeeds adds what to do, naming the app's user table.
const AdminNeeds = "`alder new admin` needs a session with a `role` whose type has an `Admin` constructor"

// adminNeeds is the refusal for session s: AdminNeeds, then what this app lacks and what to
// add: a user first (the guestbook), a role (named after the user table, when there is one),
// or the constructor.
func adminNeeds(p *Program, s *Session) string {
	switch {
	case s == nil || s.User == nil:
		return AdminNeeds + ": this app's session has no `user`; add a user table, a login that sets `user`, and a `role` (chapter 6, \"Rules\" and \"Roles\")"
	case s.Role != nil:
		return AdminNeeds + ": add `Admin` to the session's role type `" + typeText(p, s.Role.Type, "App") + "`, and set the role at login (chapter 6)"
	case s.UserTable != "":
		u := s.UserTable
		return AdminNeeds + ": add `type Role = Reader | Admin` and a column `role: Role = Reader` to `" + u + "`, `role: " + u + ".Role = Reader` to `session` in `App`, and set it at login (chapter 6)"
	}
	return AdminNeeds + ": add a union with an `Admin` constructor and `role: Role = Reader` of it to `session` in `App`, and set it at login (chapter 6)"
}

// adminCheck refuses an app without the admin's role, and options that are not a module name
// and a path; it returns the options with their defaults.
func adminCheck(p *Program, o AdminOptions) (AdminOptions, error) {
	s := p.Session
	if s == nil || s.User == nil || s.Role == nil || !slices.Contains(s.Role.Ctors, "Admin") {
		return o, Refusef("%s", adminNeeds(p, s))
	}
	if o.Module == "" {
		o.Module = "Admin"
	}
	if o.Path == "" {
		o.Path = "/admin"
	}
	if !IsModuleName(o.Module) {
		return o, Refusef("`--module %s` is not a module name: write capitalised names joined by dots, such as `Admin`", o.Module)
	}
	if !strings.HasPrefix(o.Path, "/") {
		return o, Refusef("`--path %s` must begin with `/`, such as `/admin`", o.Path)
	}
	o.Path = strings.TrimSuffix(o.Path, "/")
	if o.Path == "" || strings.ContainsAny(o.Path, "{}: ?#") {
		return o, Refusef("`--path %s` is not a path without parameters, such as `/admin`", o.Path)
	}
	if why := check.PagePathProblem(o.Path); why != "" {
		return o, Refusef("`--path %s` is not a page path: it %s; %s", o.Path, why, check.PagePathRule)
	}
	if len(p.Tables) == 0 {
		return o, Refusef("this app has no tables: the admin has nothing to show (`alder new table` makes one)")
	}
	for _, t := range p.Tables {
		for _, c := range t.Columns {
			if _, ok := secretValue(c); c.Secret() && c.Required() && c.Kind != KindTime && !ok {
				return o, Refusef("`%s.%s` is a secret, which the admin never shows or writes, and an insert must name it: give it a default (`= …`) or make it `Maybe`", t.Name, c.Name)
			}
		}
	}
	return o, nil
}

// adminDir is the directory of the admin's modules: "app/Admin".
func adminDir(o AdminOptions) string { return AdminDir(o.Module) }

// PluralConcern is the folder module of table name's pages: "Todos" for Todo, "Activities" for
// Activity. Generated pages of a table live in it (`Todos.Index`); the admin's too (`Admin.Todos`).
func PluralConcern(name string) string { return UpperFirst(Plural(LowerFirst(name))) }

// adminTableModule is the module prefix of table t's admin pages: "Admin.Todos", the plural of
// the table's name (not its module, which may be "Table.Todo").
func adminTableModule(o AdminOptions, t *Table) string {
	return o.Module + "." + PluralConcern(t.Name)
}

// AdminDir is the directory of the modules of the admin named module ("" is the default,
// "Admin"): "app/Admin".
func AdminDir(module string) string {
	if module == "" {
		module = "Admin"
	}
	return "app/" + strings.ReplaceAll(module, ".", "/")
}

// NewAdmin generates `alder new admin`: every file of the admin, created. It refuses an app
// without the role (AdminNeeds) and an admin directory that exists (regen updates it).
func NewAdmin(p *Program, o AdminOptions) ([]Edit, error) {
	o, err := adminCheck(p, o)
	if err != nil {
		return nil, err
	}
	dir := adminDir(o)
	if _, err := os.Stat(filepath.Join(p.Dir, filepath.FromSlash(dir))); err == nil {
		return nil, Refusef("`%s/` exists: `alder regen admin` updates the admin in it; to generate another, choose another `--module`", dir)
	}
	files, order := AdminFiles(p, o)
	var edits []Edit
	for _, path := range order {
		edits = append(edits, Edit{Path: path, Create: true, Body: files[path]})
	}
	return edits, nil
}

// RegenAdmin generates `alder regen admin`: every file of the admin computed afresh, and for
// each one that exists, its owned regions written back where they changed (Edit.Replace); a
// file that does not exist is created when all is set, or when its table is new (the layout's
// nav region has no link to its list: a table added since), and is otherwise left out (the
// author deleted it): skipped names those. Nothing to change is no edit at all. A file whose markers are broken is refused.
func RegenAdmin(p *Program, o AdminOptions, all bool) (edits []Edit, skipped []string, err error) {
	o, err = adminCheck(p, o)
	if err != nil {
		return nil, nil, err
	}
	dir := adminDir(o)
	if _, err := os.Stat(filepath.Join(p.Dir, filepath.FromSlash(dir))); err != nil {
		return nil, nil, Refusef("there is no admin in `%s/`: `alder new admin` generates one (name another with --module)", dir)
	}
	files, order := AdminFiles(p, o)
	// a table is new when the nav does not link to its list: its pages are all written
	nav, navOK := "", false
	if b, err := os.ReadFile(filepath.Join(p.Dir, filepath.FromSlash(FileOf(o.Module+".Layout")))); err == nil {
		nav, navOK, _ = Regions.Body(string(b), "nav")
	}
	// an admin generated with a folder per table name (Admin/Todo/) is not updated in place
	var oldDirs, newDirs []string
	for _, t := range p.Tables {
		oldMod := o.Module + "." + t.Name
		newMod := adminTableModule(o, t)
		if oldMod == newMod {
			continue
		}
		_, statErr := os.Stat(filepath.Join(p.Dir, filepath.FromSlash(AdminDir(oldMod))))
		if statErr == nil || strings.Contains(nav, oldMod+".Index") {
			oldDirs = append(oldDirs, strings.TrimPrefix(AdminDir(oldMod), "app/")+"/")
			newDirs = append(newDirs, strings.TrimPrefix(AdminDir(newMod), "app/")+"/")
		}
	}
	if len(oldDirs) > 0 {
		return nil, nil, Refusef("admin is in the old layout (%s): folders are plural now (%s). Delete %s and run `alder new admin` again; move any code you added outside `-- @admin` regions first",
			strings.Join(oldDirs, ", "), strings.Join(newDirs, ", "), dir)
	}
	// an admin still in the old `page` syntax (#160): its files have `page` lines, which no
	// region holds
	for _, path := range order {
		if b, err := os.ReadFile(filepath.Join(p.Dir, filepath.FromSlash(path))); err == nil && pageLine.Match(b) {
			return nil, nil, Refusef("admin is written with pages (`%s` has a `page` line): it is written with routes now. Delete %s and run `alder new admin` again; move any code you added outside `-- @admin` regions first", path, dir)
		}
	}
	tableOf := map[string]string{} // a table page's path: the expression of its list route
	siloOf := map[string]string{}  // a siloed table page's path: its silo's dashboard
	for _, t := range p.Tables {
		tmod := adminTableModule(o, t)
		for _, kind := range []string{"Index", "Show", "New", "Edit", "Delete", "Bulk"} {
			if t.Silo != "" { // new when the dashboard of one value does not link to its list
				siloOf[FileOf(tmod+"."+kind)] = FileOf(o.Module + "." + UpperFirst(Plural(t.Silo)) + ".Dashboard")
				tableOf[FileOf(tmod+"."+kind)] = routeOf(tmod+".Index") + " {"
				continue
			}
			tableOf[FileOf(tmod+"."+kind)] = routeOf(tmod+".Index") + " {}"
		}
	}
	exists := func(path string) bool {
		_, err := os.Stat(filepath.Join(p.Dir, filepath.FromSlash(path)))
		return err == nil
	}
	for _, path := range order {
		fresh := files[path]
		if !exists(path) {
			newTable := false
			if list, ok := tableOf[path]; ok { // a table's page: new when the nav has no link to the table
				if dash, ok := siloOf[path]; ok {
					b, err := os.ReadFile(filepath.Join(p.Dir, filepath.FromSlash(dash)))
					newTable = err != nil || !strings.Contains(string(b), list)
				} else if navOK {
					newTable = !strings.Contains(nav, "{"+list+"}")
				} else { // no nav to read (the layout deleted): new when none of its pages exists
					entries, _ := os.ReadDir(filepath.Join(p.Dir, filepath.Dir(filepath.FromSlash(path))))
					newTable = len(entries) == 0
				}
			}
			if all || newTable {
				edits = append(edits, Edit{Path: path, Create: true, Body: fresh})
			} else {
				skipped = append(skipped, path)
			}
			continue
		}
		b, err := os.ReadFile(filepath.Join(p.Dir, filepath.FromSlash(path)))
		if err != nil {
			return nil, nil, err
		}
		cur := string(b)
		names, err := Regions.Names(cur)
		if err != nil {
			return nil, nil, Refusef("`%s`: %v", path, err)
		}
		if _, ok, _ := Regions.Body(fresh, "filters"); ok && !slices.Contains(names, "filters") && !strings.Contains(cur, "\nlistAt :") {
			// a list from before the filters: its other regions now use what the region defines
			return nil, nil, Refusef("`%s` has no region `filters`, which its list now needs (it was generated before the filters): delete it and run `alder regen admin --all` to write it again, then carry over your changes", path)
		}
		var replace []Region
		for _, name := range names {
			body, ok, _ := Regions.Body(fresh, name)
			if !ok { // a region the generator no longer writes: the author's now
				continue
			}
			if old, _, _ := Regions.Body(cur, name); old != body {
				replace = append(replace, Region{Name: name, Body: body})
			}
		}
		if len(replace) > 0 {
			edits = append(edits, Edit{Path: path, Replace: replace})
		}
	}
	return edits, skipped, nil
}

// AdminFiles is every file of the admin, by path, and the paths in the order they are written.
func AdminFiles(p *Program, o AdminOptions) (map[string]string, []string) {
	g := &crudGen{p: p, o: o, files: map[string]string{}, names: map[string][]string{}, role: p.RoleColumn()}
	for _, t := range p.Tables {
		g.tables = append(g.tables, g.table(t))
	}
	for _, at := range g.tables {
		g.bulkInfo(at)
	}
	g.silos()
	g.layout()
	g.dashboard()
	for _, s := range g.silo {
		g.picker(s)
		g.siloDashboard(s)
	}
	for _, at := range g.tables {
		g.index(at)
		g.show(at)
		g.newPage(at)
		g.edit(at)
		g.delete(at)
		g.bulk(at)
	}
	return g.files, g.order
}

// referrer is a column that refers to a table: what deleting a row of that table does to the
// rows holding it.
type referrer struct {
	t     *Table
	col   *Column
	field string // the Delete page's model field counting them: "comments"
}

// load is GET route r's `load`, with body its statements (indented two spaces), run only for a
// user the layout's adminOnly lets in (isAdmin, its region `access`: when the user table holds
// the role, a user whose row says Admin now); any other gets a 404. The session's role is the one
// read at login, which lasts until logout or the cookie's 30 days (chapter 6, "Rules"); every
// write route's `update` begins the same way (update).
func (g *crudGen) load(r, body string) string {
	return r + ".load : " + r + ".Params -> Session -> Query Model\n" + r + ".load params session = {\n  let _ <- " + g.o.Module +
		".Layout.adminOnly session.user\n" + body + "\n}\n\n"
}

// update is write route r's `update`, with body its statements (indented two spaces), run only
// for a user the layout's adminOnly lets in, as load; any other gets a 404.
func (g *crudGen) update(r, body string) string {
	return r + ".update : " + r + ".Params -> Session -> Task Response\n" + r + ".update params session = {\n  let _ <- " + g.o.Module +
		".Layout.adminOnly session.user\n" + body + "\n}\n\n"
}

// requires is the `route` line's clause of every admin route.
const adminRequires = " requires user, role Admin"

// pageLine is a `page` line: a module still in the old syntax (#160).
var pageLine = regexp.MustCompile(`(?m)^page "`)

// layoutOpen is the root element of an admin page's view.
func (g *crudGen) layoutOpen(title string, imports *importSet) (string, string) {
	imports.add(g.o.Module + ".Layout")
	return "<" + g.o.Module + ".Layout.Page title=" + quote(title) + ">", "</" + g.o.Module + ".Layout.Page>"
}

// ---- the layout and the dashboard

func (g *crudGen) layout() {
	mod := g.o.Module + ".Layout"
	imports := newImports(mod, g.p.Std)
	var nav []string
	for _, at := range g.tables {
		if at.silo == nil {
			nav = append(nav, fmt.Sprintf("<a href={%s {}}>%s</a>", at.route("Index"), at.title))
		}
	}
	for _, s := range g.silo { // a database per value: the picker, then a value's pages
		nav = append(nav, fmt.Sprintf("<a href={%s {}}>%s, by %s</a>", routeOf(s.picker), s.title, s.key))
	}
	var b strings.Builder
	b.WriteString("-- The admin's frame: the app's layout around a nav of every table's list.\n")
	b.WriteString("Page : { title: Text, children: List Html } -> Html\n")
	b.WriteString("Page props =\n")
	navBlock := "<nav aria-label=\"Admin\">\n  <al-stack direction=\"row\" gap=\"sm\">\n    <a href={" + routeOf(g.o.Module+".Index") + " {}}>Dashboard</a>\n" +
		regionAt("    ", "nav", nav) + "\n  </al-stack>\n</nav>"
	if g.p.Layout {
		imports.add("Layout")
		b.WriteString("  <Layout.Page title={props.title ++ \" · Admin\"}>\n")
		b.WriteString(indent(navBlock, "    ") + "\n")
		b.WriteString("    {props.children}\n")
		b.WriteString("  </Layout.Page>\n")
	} else {
		b.WriteString("  <html>\n    <head>\n      <meta charset=\"utf-8\" />\n      " + scaffold.ThemeScript + "\n      <title>{props.title ++ \" · Admin\"}</title>\n")
		b.WriteString("      <link rel=\"stylesheet\" href={Url.external \"/static/al/al.css\"} />\n    </head>\n    <body>\n      <al-page>\n")
		b.WriteString(indent(navBlock, "        ") + "\n")
		b.WriteString("        <main><al-stack gap=\"lg\">{props.children}</al-stack></main>\n      </al-page>\n    </body>\n  </html>\n")
	}
	// isAdmin: every page's load asks it
	id, _ := unMaybe(g.p.Session.User.Type)
	importTypes(imports, id)
	access := []string{
		"-- isAdmin is whether the signed-in user may use the admin, which every route's load and update",
		"-- asks (adminOnly). The session's role is the one read at login, and lasts until logout or the",
		"-- cookie's 30 days (chapter 6, \"Rules\"): the user table holds the role, so it is read again,",
		"-- and a user demoted or deleted since gets a 404 at once.",
		"isAdmin : " + types.StringIn(id, mod) + " -> Query Bool",
	}
	if g.role != nil {
		ut := g.p.TableNamed(g.p.Session.UserTable)
		imports.add(ut.Module)
		access = append(access, "isAdmin id = {",
			"  let me <- Db.get "+ut.Name+" id",
			"  Query.succeed (Maybe.map (\\u -> u."+g.role.Name+") me == Just Admin)",
			"}")
	} else {
		access[2] = "-- cookie's 30 days (chapter 6, \"Rules\"): the user table has no role column to read again, so"
		access[3] = "-- the role on each route's line is the whole check."
		access = append(access, "isAdmin id = Query.succeed True")
	}
	b.WriteString("\n" + regionAt("", "access", access) + "\n")
	b.WriteString(`
-- adminOnly stops a route with a 404 unless isAdmin: every route's load and update begins with it.
adminOnly : ` + types.StringIn(id, mod) + ` -> Query Unit
adminOnly id = {
  let admin <- isAdmin id
  if admin then Query.succeed () else Query.notFound
}
`)
	g.file(mod, imports, b.String())
}

func (g *crudGen) dashboard() {
	mod := g.o.Module + ".Index"
	g.names[mod] = []string{"Index"}
	imports := newImports(mod, g.p.Std)
	open, close := g.layoutOpen("Dashboard", imports)
	var loads, cards []string
	for _, at := range g.tables {
		if at.silo != nil { // counted for one value at a time: the silo's card
			continue
		}
		t := at.t
		imports.add(t.Module)
		sel := "Db.from " + t.Name
		if t.Created != "" {
			sel += " |> Db.orderBy " + t.Name + "." + t.Created + ".desc"
		}
		loads = append(loads,
			fmt.Sprintf("let %s <- %s |> Db.limit recent |> Db.all", at.plural, sel),
			fmt.Sprintf("let %sCount <- Db.from %s |> Db.count", LowerFirst(t.Name), t.Name))
		cards = append(cards, fmt.Sprintf("  { key: %s, title: %s, href: %s {}, count: %sCount, rows: List.map (\\row -> { label: row.%s, href: %s { id: row.%s } }) %s },",
			quote(at.plural), quote(at.title), at.route("Index"), LowerFirst(t.Name), at.label.Name, at.route("Show"), t.IDField, at.plural))
	}
	for _, s := range g.silo { // the key's newest rows (its own card's), each opening its dashboard
		kat := g.adminOf(s.kt)
		cards = append(cards, fmt.Sprintf("  { key: %s, title: %s, href: %s {}, count: %sCount, rows: List.map (\\row -> { label: %s, href: %s { %s: row.%s } }) %s },",
			quote("silo-"+s.key), quote(s.title+", by "+s.key), routeOf(s.picker), LowerFirst(s.kt.Name), textOf(kat.label, "row."+kat.label.Name), routeOf(s.dash), s.key, s.kt.IDField, kat.plural))
	}
	body := append(loads, "Query.succeed { cards: [")
	body = append(body, cards...)
	body = append(body, "] }")

	var b strings.Builder
	b.WriteString(regionAt("", "routes", []string{routeLine("Index", "GET", g.o.Path, nil, "", adminRequires)}) + "\n\n")
	b.WriteString("-- A card of the dashboard: a table's count and its newest rows, each linking to its page.\n")
	b.WriteString("type Card = { key: Text, title: Text, href: Route, count: Int, rows: List { label: Text, href: Route } }\n\n")
	b.WriteString("type Model = { cards: List Card }\n\n")
	b.WriteString("-- recent is how many of a table's newest rows its card lists.\nrecent : Int\nrecent = 5\n\n")
	b.WriteString(g.load("Index", regionAt("  ", "cards", wrapAll(body))))
	b.WriteString(viewHead("Index"))
	fmt.Fprintf(&b, "  %s\n    <h1>Admin</h1>\n\n", open)
	b.WriteString(`    <al-stack gap="md">
      {for card in model.cards ->
        <al-card id={card.key}>
          <h2 slot="header"><a href={card.href}>{card.title}</a> <al-badge variant="secondary">{Text.fromInt card.count}</al-badge></h2>
          <ul>
            {for row in card.rows -> <li><a href={row.href}>{row.label}</a></li>}
          </ul>
        </al-card>}
    </al-stack>
`)
	fmt.Fprintf(&b, "  %s\n", close)

	// the tests
	var tests []TestData
	card := func(at *crudTable, key, name string, show func(row string) string) {
		t := at.t
		tb := g.tb(mod, imports, true)
		before := tb.counts[t.Module]
		tb.bulkInsert(t, recentRows+1, "")
		total := len(tb.visible(t, false)) + tb.bulk[t.Module]
		tb.let("let page <- Test.get (Index {})")
		lhs, rhs := g.cardChecks(tb, at, key, before, total, show)
		tests = append(tests, TestData{Name: quote(name), Lines: tb.lines, Result: "{ " + strings.Join(lhs, ", ") + " } == { " + strings.Join(rhs, ", ") + " }"})
	}
	for _, at := range g.tables {
		if at.silo == nil {
			card(at, at.plural, "the dashboard counts the "+Plural(at.noun)+" and lists the newest five", func(row string) string {
				return at.route("Show") + " { id: " + row + " }"
			})
		}
	}
	for _, s := range g.silo {
		card(g.adminOf(s.kt), "silo-"+s.key, "the dashboard lists the newest "+Plural(s.key)+", each opening its "+strings.ToLower(s.title), func(row string) string {
			return routeOf(s.dash) + " { " + s.key + ": " + row + " }"
		})
	}
	if g.role == nil { // a role is read at login (chapter 6): the cookie, not a row, opens the admin
		tb := g.tb(mod, imports, false)
		tb.login("Admin")
		tb.let("let r <- Test.open (Index {})")
		tests = append(tests, TestData{Name: quote("an Admin's role is the one the cookie got at login"), Lines: tb.lines, Result: "r.answer == Shown",
			Comment: []string{
				"`requires user, role Admin` reads the role in the session cookie, the one it got at login",
				"(chapter 6, \"Roles\"): with no role column in the user table to read again, an Admin",
				"demoted since keeps the admin until they log out, or for the cookie's 30 days.",
			}})
	}
	tests = append(tests, g.gateTests(mod, imports, func(tb *testBuilder) string { return "Index {}" })...)
	b.WriteString(g.testsIn(mod, tests))
	g.file(mod, imports, b.String())
}

// adminOf is table t as the admin shows it.
func (g *crudGen) adminOf(t *Table) *crudTable {
	for _, at := range g.tables {
		if at.t == t {
			return at
		}
	}
	return nil
}

// recentRows is the dashboard's `recent`.
const recentRows = 5

// tb starts a test's builder in module mod, signed in as an Admin when admin is set.
func (g *crudGen) tb(mod string, imports *importSet, admin bool) *testBuilder {
	if s := g.siloOf(mod); s != nil { // a siloed table's page: the Admin a member, for the test's own reads
		return g.tbIn(mod, imports, admin, s, true)
	}
	for _, s := range g.silo {
		if mod == s.dash { // the dashboard of one value: an Admin who is no member
			return g.tbIn(mod, imports, admin, s, false)
		}
	}
	tb := newTestBuilder(g.p, mod, imports)
	if admin {
		tb.login("Admin")
	}
	return tb
}

// gateTests are the tests of who may not see an admin page: a visitor is sent to log in, and
// a signed-in user whose role is not Admin gets a 404. pageOf is the route value a test opens.
func (g *crudGen) gateTests(mod string, imports *importSet, pageOf func(tb *testBuilder) string) []TestData {
	var tests []TestData
	if g.p.Login != "" {
		tb := g.tb(mod, imports, false)
		page := pageOf(tb)
		tb.let("let r <- Test.open (%s)", page)
		tests = append(tests, TestData{Name: quote("a visitor is sent to log in"), Lines: tb.lines, Result: "r.answer == Redirected (" + pageLit(g.p.Login, nil) + ")"})
	}
	for _, role := range g.p.Session.Role.Ctors {
		if role == "Admin" {
			continue
		}
		tb := g.tb(mod, imports, false)
		name := article(role) + " gets no page"
		if g.role != nil { // the row says Admin, the cookie does not: the page line's role refuses
			tb.meRole = "Admin"
			name += ", even one made Admin since logging in"
		}
		tb.login(role)
		page := pageOf(tb)
		tb.let("let r <- Test.open (%s)", page)
		tests = append(tests, TestData{Name: quote(name), Lines: tb.lines, Result: "r.answer == PageMissing"})
		if g.role != nil { // the cookie says Admin, the row no longer does
			tb := g.tb(mod, imports, false)
			tb.meRole = role
			tb.login("Admin")
			page := pageOf(tb)
			tb.let("let r <- Test.open (%s)", page)
			tests = append(tests, TestData{Name: quote("a demoted Admin is refused at once"), Lines: tb.lines, Result: "r.answer == PageMissing"})
		}
		break
	}
	return tests
}

// writeGate are the tests that a write route guards itself, each sending the write from the page
// holding its form, opened by an Admin, after the session changed (#187: a write route runs no
// `load`, so its own `requires` and adminOnly are all that stop it):
//   - the session ended (a visitor): its `requires user` sends them to log in;
//   - the cookie's role is no longer Admin (with a role column, the row still says Admin): its
//     `role Admin` answers 404, before the form is decoded;
//   - an Admin demoted since (the cookie still says Admin, the row no longer does; with a role
//     column only): its update's adminOnly answers 404. A form with a select of a table's rows
//     (refs) is refused before that, 400: the runtime checks the options against the page it was
//     posted from (Plan call 11), whose load adminOnly refuses too.
//
// Nothing is written. build inserts the rows and returns the page to open, the step that sends
// the write, and check, which reads what must not have changed (the left and right sides'
// fields).
func (g *crudGen) writeGate(mod string, imports *importSet, what string, refs bool, build func(tb *testBuilder) (get, send string, check func() (string, string))) []TestData {
	var tests []TestData
	refused := func(name, answer string, change func(tb *testBuilder)) {
		tb := g.tb(mod, imports, true)
		get, send, check := build(tb)
		tb.let("let _ <- Test.get (%s)", get)
		admin := tb.lastLogin
		change(tb)
		relogged := tb.lastLogin != admin
		tb.let("let r <- %s", send)
		if tb.silo != nil && tb.silo.member && relogged {
			tb.let("%s", admin) // the Admin's session again: the test's own reads go to the silo by its key
		}
		l, r := check()
		tests = append(tests, TestData{Name: quote(name), Lines: tb.lines,
			Result: "{ answer: r.answer, " + l + " } == { answer: " + answer + ", " + r + " }"})
	}
	if g.p.Login != "" {
		refused("a visitor cannot "+what, "Redirected ("+pageLit(g.p.Login, nil)+")", func(tb *testBuilder) { tb.logout() })
	}
	if other := g.otherRole(); other != "" {
		name := article(other) + " cannot " + what
		if g.role != nil { // the row says Admin (the login inserted it so), the cookie does not
			name += ", even one made Admin since logging in"
		}
		refused(name, "PageMissing", func(tb *testBuilder) { tb.login(other) })
	}
	if g.role == nil {
		return tests
	}
	answer := "PageMissing"
	if refs {
		answer = `RequestFailed "400 bad request"`
	}
	refused("a demoted Admin cannot "+what, answer, func(tb *testBuilder) {
		ut := g.p.TableNamed(g.p.Session.UserTable)
		demoted := ""
		for _, c := range g.role.Ctors {
			if c != "Admin" && demoted == "" {
				demoted = c
			}
		}
		tb.let("let _ <- Db.update %s %s (\\u -> { u | %s: %s })", ut.Name, tb.user(), g.role.Name, maybeJust(g.role, demoted))
	})
	return tests
}

// otherRole is the session's first role but Admin, "" for none.
func (g *crudGen) otherRole() string {
	for _, c := range g.p.Session.Role.Ctors {
		if c != "Admin" {
			return c
		}
	}
	return ""
}

// ---- a table's pages

// filterOption is an option of column c's filter: its value in the address, its label, and the
// value of the column it selects.
type filterOption struct{ value, label, col string }

// ownRole is the role column of the user table at, which its Edit page refuses to change on the
// signed-in user's own row; nil for another table, or a role the form does not edit.
func (g *crudGen) ownRole(at *crudTable) *Column {
	if !g.isUserTable(at) || g.role == nil || !slices.Contains(at.editable, g.role) {
		return nil
	}
	return g.role
}

// ownRoleTests are the tests of the Edit page's guard on the signed-in user's own role (Review
// Focus 4 of M13): an Admin changing it is refused and nothing is written; an Admin saving their
// own row with the role unchanged, and an Admin demoting another Admin, are not refused.
func (g *crudGen) ownRoleTests(at *crudTable, mod string, imports *importSet, role *Column) []TestData {
	t := at.t
	other := ""
	for _, ct := range role.Ctors {
		if ct != "Admin" && other == "" {
			other = ct
		}
	}
	// values is a submission changing every field of target but the role, which is given r
	values := func(tb *testBuilder, r string) string {
		k := tb.counts[t.Module]
		var vs []string
		for _, c := range at.editable {
			v := g.changedValue(tb, at, c, k+1)
			if c == role {
				v = maybeJust(c, r)
			}
			vs = append(vs, c.Name+": "+v)
		}
		return strings.Join(vs, ", ")
	}
	var tests []TestData

	tb := g.tb(mod, imports, true)
	me := tb.user()
	g.optionRows(tb, at)
	tb.let("let before <- Db.get %s %s", t.Name, me)
	tb.let("let _ <- Test.get (%s)", pageLit("Edit", []string{"id: " + me}))
	tb.let("let r <- Test.submit %s { %s }", at.formRoute(true, me), values(tb, other))
	tb.let("let after <- Db.get %s %s", t.Name, me)
	tests = append(tests, TestData{Name: quote("an Admin cannot change their own role"), Lines: tb.lines,
		Result: fmt.Sprintf("{ answer: r.answer, row: after } == { answer: FormRejected [{ field: \"\", message: %s }], row: before }", quote(roleText))})

	tb = g.tb(mod, imports, true)
	me = tb.user()
	g.optionRows(tb, at)
	tb.let("let _ <- Test.get (%s)", pageLit("Edit", []string{"id: " + me}))
	vs := values(tb, "Admin")
	tb.let("let r <- Test.submit %s { %s }", at.formRoute(true, me), vs)
	tb.let("let after <- Db.get %s %s", t.Name, me)
	var got, want []string
	for _, c := range at.editable {
		got = append(got, c.Name+": row."+c.Name)
		want = append(want, c.Name+": "+valueOf(strings.Split(vs, ", "), c.Name))
	}
	tests = append(tests, TestData{Name: quote("an Admin saves their own account, the role unchanged"), Lines: tb.lines,
		Result: fmt.Sprintf("{ answer: r.answer, row: Maybe.map (\\row -> { %s }) after } == { answer: Redirected (%s { id: %s }), row: Just { %s } }",
			strings.Join(got, ", "), at.route("Show"), me, strings.Join(want, ", "))})

	tb = g.tb(mod, imports, true)
	tb.user()
	admin := tb.insertRow(t, "admin", map[string]string{role.Name: maybeJust(role, "Admin")}, false)
	g.optionRows(tb, at)
	tb.let("let _ <- Test.get (%s)", pageLit("Edit", []string{"id: " + admin}))
	tb.let("let r <- Test.submit %s { %s }", at.formRoute(true, admin), values(tb, other))
	tb.let("let after <- Db.get %s %s", t.Name, admin)
	tests = append(tests, TestData{Name: quote("an Admin demotes another Admin"), Lines: tb.lines,
		Result: fmt.Sprintf("{ answer: r.answer, %s: Maybe.map (\\row -> row.%s) after } == { answer: Redirected (%s { id: %s }), %s: Just %s }",
			role.Name, role.Name, at.route("Show"), admin, role.Name, maybeJust(role, other))})
	return tests
}

// ownText and roleText are the user table's refusals of the signed-in user's own row: its delete,
// and a change of its role (Review Focus 4 of M13: an Admin cannot lock themselves out).
const (
	ownText  = "You cannot delete your own account."
	roleText = "You cannot change your own role."
)

// isUserTable reports whether at is the table of the session's user: its admin guards the
// signed-in user's own row.
func (g *crudGen) isUserTable(at *crudTable) bool {
	s := g.p.Session
	return s != nil && s.User != nil && s.UserTable != "" && g.p.TableNamed(s.UserTable) == at.t
}

// isMe is the condition that the row whose id is expr is the signed-in user's (`requires user`
// makes session.user the user's id, not a Maybe).
func (g *crudGen) isMe(expr string) string { return "session.user == " + expr }

// The bulk actions' toasts: the verb, then the counts (bulkNotice).
const (
	bulkDeletedText = "Deleted"
	bulkUpdatedText = "Updated"
)

// bulkNotice is a bulk action's toast: the verb and its counts, "Deleted 1 of 2" for two counts,
// "Updated 1" for one.
func bulkNotice(verb string, counts ...int) string {
	s := verb
	for i, n := range counts {
		if i > 0 {
			s += " of"
		}
		s += " " + strconv.Itoa(n)
	}
	return s
}
