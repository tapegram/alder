package generate

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"alder/internal/check"
	"alder/internal/types"
)

// The built-in admin (`alder new admin`, `alder regen admin`; book chapter 15): for every table
// T, five pages under the admin's module (`Admin.T.Index`, `.Show`, `.New`, `.Edit`,
// `.Delete`), a dashboard (`Admin.Index`) and a layout (`Admin.Layout.Page`, the app's layout
// around a nav of every table). Every page `requires user, role Admin` and reads and writes with
// the unscoped `Db.*`: an admin acts on everyone's rows, and the role on the `page` line is the
// rule (chapter 6, "When to use the unscoped forms"). Each page carries its browser tests, which
// kill the mutants of its code (`alder test --mutate`).
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
func adminNeeds(s *Session) string {
	switch {
	case s == nil || s.User == nil:
		return AdminNeeds + ": this app's session has no `user`; add a user table, a login that sets `user`, and a `role` (chapter 6, \"Rules\" and \"Roles\")"
	case s.Role != nil:
		return AdminNeeds + ": add `Admin` to the session's role type `" + types.String(s.Role.Type) + "`, and set the role at login (chapter 6)"
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
		return o, Refusef("%s", adminNeeds(s))
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
	tableOf := map[string]string{} // a table page's path: the expression of its list page
	for _, t := range p.Tables {
		for _, kind := range []string{"Index", "Show", "New", "Edit", "Delete"} {
			tableOf[FileOf(o.Module+"."+t.Module+"."+kind)] = o.Module + "." + t.Module + ".Index {}"
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
				if navOK {
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
	g := &adminGen{p: p, o: o, files: map[string]string{}, role: p.RoleColumn()}
	for _, t := range p.Tables {
		g.tables = append(g.tables, g.table(t))
	}
	g.layout()
	g.dashboard()
	for _, at := range g.tables {
		g.index(at)
		g.show(at)
		g.newPage(at)
		g.edit(at)
		g.delete(at)
	}
	return g.files, g.order
}

type adminGen struct {
	p      *Program
	o      AdminOptions
	role   *Column // the user table's role column, which every page's load reads again; nil for none
	tables []*adminTable
	files  map[string]string
	order  []string
}

// adminTable is a table as the admin shows it.
type adminTable struct {
	t        *Table
	mod      string // the module prefix of its pages: "Admin.Post"
	path     string // "/admin/post"
	plural   string // "posts": its rows in a model, an element's id
	title    string // "Posts"
	noun     string // "post"
	label    *Column
	editable []*Column   // the columns its forms edit
	refs     []*Column   // the editable columns that refer to a table: a select of its rows
	nowCols  []*Column   // Time columns an insert must name, which no form edits: the time of the insert
	secrets  []*Column   // secret columns an insert must name, which the admin never shows or edits
	referrer []*referrer // the columns of other tables (or its own) that refer to it
}

// referrer is a column that refers to a table: what deleting a row of that table does to the
// rows holding it.
type referrer struct {
	t     *Table
	col   *Column
	field string // the Delete page's model field counting them: "comments"
}

func (g *adminGen) table(t *Table) *adminTable {
	at := &adminTable{t: t, mod: g.o.Module + "." + t.Module, path: g.o.Path + "/" + strings.ToLower(t.Name),
		plural: Plural(LowerFirst(t.Name)), noun: strings.ToLower(Humanize(t.Name)), label: t.Label()}
	at.title = UpperFirst(Plural(at.noun))
	for _, c := range t.Columns {
		switch {
		case c.ID || c.Created || c.Updated:
		case c.Kind == KindTime:
			if c.Required() && !c.Maybe {
				at.nowCols = append(at.nowCols, c)
			}
		case c.Secret(): // never shown or edited: an insert writes it empty (secretValue)
			if c.Required() {
				at.secrets = append(at.secrets, c)
			}
		case c.Kind == KindOther:
		default:
			at.editable = append(at.editable, c)
			if c.Ref != "" && g.p.TableNamed(c.Ref) != nil {
				at.refs = append(at.refs, c)
			}
		}
	}
	at.referrer = g.referrersOf(t)
	return at
}

// referrersOf are the columns that refer to t, named after their table (and their column,
// when the table refers to t twice).
func (g *adminGen) referrersOf(t *Table) []*referrer {
	count := map[string]int{}
	var refs []*referrer
	for _, other := range g.p.Tables {
		for _, c := range other.Columns {
			if c.Ref == t.Name && g.p.TableNamed(c.Ref) == t {
				refs = append(refs, &referrer{t: other, col: c})
				count[other.Module]++
			}
		}
	}
	for _, r := range refs {
		r.field = Plural(LowerFirst(r.t.Name))
		if count[r.t.Module] > 1 {
			r.field += UpperFirst(r.col.Name)
		}
	}
	return refs
}

// node is a reference in the closure of what deleting a row does: a column that refers to the
// row (parent nil), or to the rows a `@cascade` node deletes with it (parent that node).
type node struct {
	t      *Table
	col    *Column
	field  string // the Delete page's model field counting its rows: "flagsOfComments"
	parent *node
	kids   []*node // for a @cascade: the @cascade and @restrict references down from it that lead to a @restrict
	// stop marks a @cascade from a table to itself (a tree: a row's children) whose deletes can
	// reach a @restrict (restrictReach): its rows are counted, not followed, since a query cannot
	// follow a tree to its depth. At the top (the row's own children) the page deletes only a row
	// without them ("Delete its children first."); further down, they keep the row like a
	// @restrict reference.
	stop bool
}

// selfStop reports whether a reference col from table t is a @cascade to t itself whose
// deletes can reach a @restrict: a node to stop at (node.stop). Only a table can refer to
// itself: a cycle through two tables would be an import cycle (E0006), as a `@ref` names a table
// its module imports.
func (g *adminGen) selfStop(t *Table, col *Column) bool {
	return col.OnDelete == "cascade" && g.p.TableNamed(col.Ref) == t && g.restrictReach(t)
}

// restrictReach reports whether deleting a row of t can meet a @restrict reference: one to t, or
// to a table whose rows a @cascade from t deletes, at any depth.
func (g *adminGen) restrictReach(t *Table) bool {
	seen := map[*Table]bool{t: true}
	for queue := []*Table{t}; len(queue) > 0; queue = queue[1:] {
		for _, r := range g.referrersOf(queue[0]) {
			switch r.col.OnDelete {
			case "restrict":
				return true
			case "cascade":
				if !seen[r.t] {
					seen[r.t] = true
					queue = append(queue, r.t)
				}
			}
		}
	}
	return false
}

// closure is the tree of the references to table at: each of at.referrer, and below each
// `@cascade` one the references to the rows it deletes, down the whole chain (chapter 8: a row
// that refuses the delete can be any number of steps down). A deeper reference is kept when it
// is a `@restrict` or leads to one. A column is not followed twice on one path, so a cycle (a
// table that refers to itself) is followed once round: the tree is bounded by the schema. A
// table's `@restrict` reference to itself, under a cascade of that table, is left out.
func (g *adminGen) closure(at *adminTable) []*node {
	var nodes []*node
	for _, r := range at.referrer {
		n := &node{t: r.t, col: r.col, field: r.field, stop: g.selfStop(r.t, r.col)}
		if !n.stop {
			g.expand(n, map[*Column]bool{r.col: true})
		}
		nodes = append(nodes, n)
	}
	return nodes
}

// expand adds node n's kids: when n is a @cascade, the references to its table not on path.
func (g *adminGen) expand(n *node, path map[*Column]bool) {
	if n.col.OnDelete != "cascade" {
		return
	}
	for _, c := range g.referrersOf(n.t) {
		if path[c.col] || (c.col.OnDelete != "cascade" && c.col.OnDelete != "restrict") || (c.col.OnDelete == "restrict" && c.t == n.t) {
			continue
		}
		k := &node{t: c.t, col: c.col, field: c.field + "Of" + UpperFirst(n.field), parent: n, stop: c.t == n.t && g.selfStop(c.t, c.col)}
		if !k.stop {
			path[c.col] = true
			g.expand(k, path)
			delete(path, c.col)
		}
		if c.col.OnDelete == "restrict" || len(k.kids) > 0 || k.stop {
			n.kids = append(n.kids, k)
		}
	}
}

// walk is nodes and their kids, each before its kids.
func walk(nodes []*node) []*node {
	var out []*node
	for _, n := range nodes {
		out = append(out, n)
		out = append(out, walk(n.kids)...)
	}
	return out
}

// secretValue is the value the admin's insert gives secret column c, which an insert must name
// and no form edits: Nothing, or the empty text (a hash or token that matches nothing, for the
// app to set); false for another type (adminCheck refuses it).
func secretValue(c *Column) (string, bool) {
	if c.Maybe {
		return "Nothing", true
	}
	if con, ok := c.Type.(*types.Con); ok && con.Name == "Text" {
		return `""`, true
	}
	return "", false
}

// shown are the columns of t the admin shows: all but its secrets (Column.Secret), and but the
// id unless id is set.
func shown(t *Table, id bool) []*Column {
	var out []*Column
	for _, c := range t.Columns {
		if !c.Secret() && (id || !c.ID) {
			out = append(out, c)
		}
	}
	return out
}

// pluralOf is table t's rows in a sentence: "comments".
func pluralOf(t *Table) string { return Plural(strings.ToLower(Humanize(t.Name))) }

// eqID is the condition that column c of table t holds the id in expr: `T.c.eq id`, or with a
// Maybe column `T.c.eq (Just id)`.
func eqID(t *Table, c *Column, expr string) string {
	if c.Maybe {
		return t.Name + "." + c.Name + ".eq (Just " + expr + ")"
	}
	return t.Name + "." + c.Name + ".eq " + expr
}

// file adds a generated file: the header, the module line, the imports region, then body.
func (g *adminGen) file(module string, imports *importSet, body string) {
	var b strings.Builder
	// no version: the same app gives the same files from every build, so an admin committed
	// with one build is replayed byte for byte by another
	b.WriteString("-- generated by `alder new admin`; regions between\n")
	b.WriteString("-- `-- @admin begin <name>` / `-- @admin end` are rewritten by `alder regen admin`\n")
	fmt.Fprintf(&b, "module %s\n\n", module)
	b.WriteString(regionAt("", "imports", strings.Split(strings.TrimSuffix(imports.lines(), "\n"), "\n")) + "\n\n")
	b.WriteString(body)
	path := FileOf(module)
	g.files[path] = Tidy(b.String())
	g.order = append(g.order, path)
}

// regionAt is an owned region: its markers at indentation ind, around lines (each indented by
// ind as well; none for an empty region).
func regionAt(ind, name string, lines []string) string {
	var b strings.Builder
	b.WriteString(ind + RegionBegin + name + "\n")
	for _, l := range lines {
		b.WriteString(indent(l, ind) + "\n")
	}
	b.WriteString(ind + RegionEnd)
	return b.String()
}

// load is a page's `load`, with body its statements (indented two spaces), run only for a user
// the layout's isAdmin lets in (its region `access`: when the user table holds the role, a user
// whose row says Admin now); any other gets a 404. The session's role is the one read at login,
// which lasts until logout or the cookie's 30 days (chapter 6, "Rules"); load runs before every
// message too.
func (g *adminGen) load(body string) string {
	return "load : Params -> Session -> Query Model\nload params session = {\n  let admin <- " + g.o.Module +
		".Layout.isAdmin session.user\n  if admin then {\n" + indent(body, "  ") + "\n  } else Query.notFound\n}\n\n"
}

// requires is the `page` line's clause of every admin page.
const adminRequires = " requires user, role Admin"

// page is the page module of table at's page kind ("Index", "Show", …).
func (at *adminTable) page(kind string) string { return at.mod + "." + kind }

// layoutOpen is the root element of an admin page's view.
func (g *adminGen) layoutOpen(title string, imports *importSet) (string, string) {
	imports.add(g.o.Module + ".Layout")
	return "<" + g.o.Module + ".Layout.Page title=" + quote(title) + ">", "</" + g.o.Module + ".Layout.Page>"
}

// ---- the layout and the dashboard

func (g *adminGen) layout() {
	mod := g.o.Module + ".Layout"
	imports := newImports(mod, g.p.Std)
	var nav []string
	for _, at := range g.tables {
		nav = append(nav, fmt.Sprintf("<a href={%s {}}>%s</a>", at.page("Index"), at.title))
	}
	var b strings.Builder
	b.WriteString("-- The admin's frame: the app's layout around a nav of every table's list.\n")
	b.WriteString("Page : { title: Text, children: List (Html a) } -> Html a\n")
	b.WriteString("Page props =\n")
	navBlock := "<nav aria-label=\"Admin\">\n  <al-stack direction=\"row\" gap=\"sm\">\n    <a href={" + g.o.Module + ".Index {}}>Dashboard</a>\n" +
		regionAt("    ", "nav", nav) + "\n  </al-stack>\n</nav>"
	if g.p.Layout {
		imports.add("Layout")
		b.WriteString("  <Layout.Page title={props.title ++ \" · Admin\"}>\n")
		b.WriteString(indent(navBlock, "    ") + "\n")
		b.WriteString("    {props.children}\n")
		b.WriteString("  </Layout.Page>\n")
	} else {
		b.WriteString("  <html>\n    <head>\n      <meta charset=\"utf-8\" />\n      <title>{props.title ++ \" · Admin\"}</title>\n")
		b.WriteString("      <link rel=\"stylesheet\" href={Url.external \"/static/al/al.css\"} />\n    </head>\n    <body>\n      <al-page>\n")
		b.WriteString(indent(navBlock, "        ") + "\n")
		b.WriteString("        <main>{props.children}</main>\n      </al-page>\n    </body>\n  </html>\n")
	}
	// isAdmin: every page's load asks it
	id, _ := unMaybe(g.p.Session.User.Type)
	importTypes(imports, id)
	access := []string{
		"-- isAdmin is whether the signed-in user may use the admin, which every page's load asks (it runs",
		"-- before every message too). The session's role is the one read at login, and lasts until",
		"-- logout or the cookie's 30 days (chapter 6, \"Rules\"): the user table holds the role, so it is",
		"-- read again, and a user demoted or deleted since gets a 404 at once.",
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
		access[2] = "-- logout or the cookie's 30 days (chapter 6, \"Rules\"): the user table has no role column to"
		access[3] = "-- read again, so the role on each page's line is the whole check."
		access = append(access, "isAdmin id = Query.succeed True")
	}
	b.WriteString("\n" + regionAt("", "access", access) + "\n")
	g.file(mod, imports, b.String())
}

func (g *adminGen) dashboard() {
	mod := g.o.Module + ".Index"
	imports := newImports(mod, g.p.Std)
	open, close := g.layoutOpen("Dashboard", imports)
	var loads, cards []string
	for _, at := range g.tables {
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
			quote(at.plural), quote(at.title), at.page("Index"), LowerFirst(t.Name), at.label.Name, at.page("Show"), t.IDField, at.plural))
	}
	body := append(loads, "Query.succeed { cards: [")
	body = append(body, cards...)
	body = append(body, "] }")

	var b strings.Builder
	fmt.Fprintf(&b, "page %s { }%s\n\n", quote(g.o.Path), adminRequires)
	b.WriteString("-- A card of the dashboard: a table's count and its newest rows, each linking to its page.\n")
	b.WriteString("type Card = { key: Text, title: Text, href: Page, count: Int, rows: List { label: Text, href: Page } }\n\n")
	b.WriteString("type Model = { cards: List Card }\n\n")
	b.WriteString("-- recent is how many of a table's newest rows its card lists.\nrecent : Int\nrecent = 5\n\n")
	b.WriteString(g.load(regionAt("  ", "cards", wrapAll(body))))
	b.WriteString("view : Model -> Session -> Html Msg\nview model session =\n")
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
	for _, at := range g.tables {
		t := at.t
		tb := g.tb(mod, imports, true)
		before := tb.counts[t.Module]
		tb.bulkInsert(t, recentRows+1, "")
		total := len(tb.visible(t, false)) + tb.bulk[t.Module]
		tb.let("let page <- Test.get (%s {})", mod)
		lhs := []string{fmt.Sprintf("count: Test.text \"#%s al-badge\" page.doc", at.plural)}
		rhs := []string{fmt.Sprintf("count: Just %s", quote(fmt.Sprint(total)))}
		if !at.label.ID && t.Created != "" {
			var labels []string
			for n := before + recentRows + 1; n > before+1; n-- {
				labels = append(labels, quote(rawText(at.label, n)))
			}
			lhs = append(lhs, fmt.Sprintf("rows: Test.texts \"#%s li\" page.doc", at.plural))
			rhs = append(rhs, "rows: ["+strings.Join(labels, ", ")+"]")
		} else if t.Created != "" { // the label is the id: the newest rows, by the pages they link to
			tb.let("let newest <- Db.from %s |> Db.orderBy %s.%s.desc |> Db.limit %d |> Db.all", t.Name, t.Name, t.Created, recentRows)
			lhs = append(lhs, fmt.Sprintf("rows: List.map (\\card -> List.map (\\row -> row.href) card.rows) (List.filter (\\card -> card.key == %s) page.model.cards)", quote(at.plural)))
			rhs = append(rhs, fmt.Sprintf("rows: [List.map (\\row -> %s { id: row.%s }) newest]", at.page("Show"), t.IDField))
		} else {
			lhs = append(lhs, fmt.Sprintf("rows: Test.count \"#%s li\" page.doc", at.plural))
			rhs = append(rhs, fmt.Sprintf("rows: %d", recentRows))
		}
		name := "the dashboard counts the " + Plural(at.noun) + " and lists the newest five"
		tests = append(tests, TestData{Name: quote(name), Lines: tb.lines, Result: "{ " + strings.Join(lhs, ", ") + " } == { " + strings.Join(rhs, ", ") + " }"})
	}
	if g.role == nil { // a role is read at login (chapter 6): the cookie, not a row, opens the admin
		tb := g.tb(mod, imports, false)
		tb.login("Admin")
		tb.let("let r <- Test.open (%s {})", mod)
		tests = append(tests, TestData{Name: quote("an Admin's role is the one the cookie got at login"), Lines: tb.lines, Result: "r.answer == Shown",
			Comment: []string{
				"`requires user, role Admin` reads the role in the session cookie, the one it got at login",
				"(chapter 6, \"Roles\"): with no role column in the user table to read again, an Admin",
				"demoted since keeps the admin until they log out, or for the cookie's 30 days.",
			}})
	}
	tests = append(tests, g.gateTests(mod, imports, func(tb *testBuilder) string { return mod + " {}" })...)
	b.WriteString(testsText(tests))
	g.file(mod, imports, b.String())
}

// recentRows is the dashboard's `recent`.
const recentRows = 5

// tb starts a test's builder in module mod, signed in as an Admin when admin is set.
func (g *adminGen) tb(mod string, imports *importSet, admin bool) *testBuilder {
	tb := newTestBuilder(g.p, mod, imports)
	if admin {
		tb.login("Admin")
	}
	return tb
}

// gateTests are the tests of who may not see an admin page: a visitor is sent to log in, and
// a signed-in user whose role is not Admin gets a 404. pageOf is the page value a test opens.
func (g *adminGen) gateTests(mod string, imports *importSet, pageOf func(tb *testBuilder) string) []TestData {
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

// testsText renders the generated tests, in the region `tests` at the end of the file: regen
// rewrites them as the schema changes, and a test the author adds after the region stays.
func testsText(tests []TestData) string {
	var parts []string
	for _, t := range tests {
		parts = append(parts, snippet("test", t))
	}
	return "\n" + RegionBegin + "tests\n" + wrap(strings.Join(parts, "\n\n")) + "\n" + RegionEnd + "\n"
}

// ---- a table's pages

// adminCell is the markup showing column c of row (an expression): a reference as a link to
// the referenced row's admin page, labelled by that row's label (model.labels, from the
// page's region `labels`) or, when its table has none, its id; the others as the list pages
// show them.
func (g *adminGen) adminCell(c *Column, row string, imports *importSet) string {
	if c.Ref != "" {
		if rt := g.p.TableNamed(c.Ref); rt != nil {
			show := g.o.Module + "." + rt.Module + ".Show"
			text := func(id string) string {
				if rt.Label().ID {
					return id
				}
				return labelFunc(c) + " model.labels " + id
			}
			if c.Maybe {
				return "{match " + row + "." + c.Name + " { Just v -> <a href={" + show + " { id: v }}>{" + text("v") + "}</a>, Nothing -> Html.none }}"
			}
			return "<a href={" + show + " { id: " + row + "." + c.Name + " }}>{" + text(row+"."+c.Name) + "}</a>"
		}
	}
	return cell(c, row+"."+c.Name, imports)
}

// labelFunc is the function naming the row reference column c refers to: "authorLabel".
func labelFunc(c *Column) string { return c.Name + "Label" }

// labelled are the columns of at's table whose cells show the label of the row they refer to:
// its references to a table whose rows have a label other than their id.
func (g *adminGen) labelled(at *adminTable) []*Column {
	var out []*Column
	for _, c := range at.t.Columns {
		if rt := g.p.TableNamed(c.Ref); c.Ref != "" && rt != nil && !rt.Label().ID && !c.Secret() {
			out = append(out, c)
		}
	}
	return out
}

// labels is the region `labels` of at's list and show pages: the type Labels, the rows its
// references name (a field per column, as optionsField), labelsOf reading them for the rows a
// page shows (`.in` their ids), and a function per column naming a row by its label.
func (g *adminGen) labels(at *adminTable, mod string, imports *importSet) []string {
	t := at.t
	cols := g.labelled(at)
	var fields, loads, inits, funcs []string
	for _, c := range cols {
		rt := g.p.TableNamed(c.Ref)
		imports.add(rt.Module)
		importTypes(imports, rt.IDType)
		fields = append(fields, optionsField(c)+": List "+rt.Name)
		ids := "List.map (\\row -> row." + c.Name + ") rows"
		if c.Maybe {
			ids = "List.filterMap (\\row -> row." + c.Name + ") rows"
		}
		loads = append(loads, fmt.Sprintf("  let %s <- Db.from %s |> Db.where (%s.%s.in (%s)) |> Db.all", optionsField(c), rt.Name, rt.Name, rt.IDField, ids))
		inits = append(inits, optionsField(c)+": "+optionsField(c))
		noun := strings.ToLower(Humanize(rt.Name))
		funcs = append(funcs, "",
			fmt.Sprintf("-- %s is the label of the %s id names (the id itself if it names none).", labelFunc(c), noun),
			fmt.Sprintf("%s : Labels -> %s -> Text", labelFunc(c), types.StringIn(rt.IDType, mod)),
			fmt.Sprintf("%s labels id = match List.find (\\r -> r.%s == id) labels.%s {", labelFunc(c), rt.IDField, optionsField(c)),
			"  Just r -> "+textOf(rt.Label(), "r."+rt.Label().Name)+",",
			"  Nothing -> id,",
			"}")
	}
	out := []string{"-- Labels are the rows the references of the rows shown name: their links show their labels."}
	if len(cols) == 0 {
		return append(out, "type Labels = { }", "", "labelsOf : List "+t.Name+" -> Query Labels", "labelsOf rows = Query.succeed { }")
	}
	out = append(out, "type Labels = { "+strings.Join(fields, ", ")+" }", "",
		"-- labelsOf reads the rows that rows refer to.",
		"labelsOf : List "+t.Name+" -> Query Labels", "labelsOf rows = {")
	out = append(out, loads...)
	out = append(out, "  Query.succeed { "+strings.Join(inits, ", ")+" }", "}")
	return append(out, funcs...)
}

// labelChecks are a test's comparisons that the page loaded the labels of the rows its
// references name, and only those, and names a row by its label (left and right sides'
// fields), with the rows they need: for each labelled column to another table, one more row of
// that table, which nothing shown refers to. model is the page's model expression. A reference
// of the table to itself (a tree) is checked when parents gives the parent the caller inserted
// for the rows shown (selfParents): its variable and label.
//
// selfParents inserts, for each labelled reference of at's table to itself (a `Maybe`, a tree),
// a parent row, and returns the overrides that make a row its child, with labelChecks' parents.
func (g *adminGen) selfParents(tb *testBuilder, at *adminTable) (map[string]string, map[*Column][2]string) {
	over, parents := map[string]string{}, map[*Column][2]string{}
	for _, c := range g.labelled(at) {
		if g.p.TableNamed(c.Ref) == at.t && c.Maybe {
			p := tb.insertRow(at.t, "parent", nil, false)
			parents[c] = [2]string{p, rawText(at.label, tb.counts[at.t.Module])}
			over[c.Name] = "Just " + p
		}
	}
	return over, parents
}
func (g *adminGen) labelChecks(tb *testBuilder, at *adminTable, model string, parents map[*Column][2]string) [2][]string {
	var out [2][]string
	for _, c := range g.labelled(at) {
		rt := g.p.TableNamed(c.Ref)
		if rt == at.t { // a tree: the parent the caller gave the rows shown, its label
			if p, ok := parents[c]; ok {
				out[0] = append(out[0], fmt.Sprintf("%s: List.length %s.labels.%s", optionsField(c), model, optionsField(c)),
					fmt.Sprintf("%s: %s %s.labels %s", labelFunc(c), labelFunc(c), model, p[0]))
				out[1] = append(out[1], optionsField(c)+": 1", labelFunc(c)+": "+quote(p[1]))
			}
			continue
		}
		if len(tb.rows[rt.Module]) == 0 && !c.Maybe {
			continue
		}
		first, firstID := "", ""
		if len(tb.rows[rt.Module]) > 0 {
			first, firstID = rawText(rt.Label(), 1), tb.rows[rt.Module][0].v
		}
		extra := tb.insertRow(rt, "", nil, false)
		n := 1
		if c.Maybe { // the rows shown refer to none: the label of the extra row, from labels holding it
			n = 0
			all := tb.fresh(optionsField(c) + "All")
			tb.let("let %s <- Db.from %s |> Db.all", all, rt.Name)
			out[0] = append(out[0], fmt.Sprintf("%s: %s { %s.labels | %s: %s } %s", labelFunc(c), labelFunc(c), model, optionsField(c), all, extra))
			out[1] = append(out[1], labelFunc(c)+": "+quote(rawText(rt.Label(), tb.counts[rt.Module])))
		}
		out[0] = append(out[0], fmt.Sprintf("%s: List.length %s.labels.%s", optionsField(c), model, optionsField(c)))
		out[1] = append(out[1], fmt.Sprintf("%s: %d", optionsField(c), n))
		if !c.Maybe { // the label of the row the rows shown refer to
			out[0] = append(out[0], fmt.Sprintf("%s: %s %s.labels %s", labelFunc(c), labelFunc(c), model, firstID))
			out[1] = append(out[1], labelFunc(c)+": "+quote(first))
		}
	}
	return out
}

func (g *adminGen) index(at *adminTable) {
	t := at.t
	mod := at.page("Index")
	imports := newImports(mod, g.p.Std)
	imports.add(t.Module)
	open, close := g.layoutOpen(at.title, imports)
	order := t.OrderColumn()

	// search: every Text column but ids and references
	var searchCols []*Column
	for _, c := range t.Columns {
		if c.Kind == KindText && !c.Maybe && !c.ID && c.Ref == "" && !c.Secret() {
			searchCols = append(searchCols, c)
		}
	}
	var terms []string
	for _, c := range searchCols {
		terms = append(terms, t.Name+"."+c.Name+".contains q")
	}
	search := []string{
		"-- search is the rows whose text contains q (every row for \"\").",
		"search : Text -> Condition " + t.Name,
		"search q = Db.or [" + strings.Join(terms, ", ") + "]",
	}
	if len(terms) == 0 {
		search[0] = "-- search is every row: the table has no text to search."
		search[2] = "search q = Db.and []"
	}
	// sort: every column but the id and the secrets
	cols := shown(t, false)
	sorts := []string{
		"-- sorts are the columns the list sorts by, by name.",
		"sorts : List { name: Text, asc: Order " + t.Name + ", desc: Order " + t.Name + " }",
		"sorts = [",
	}
	for _, c := range cols {
		sorts = append(sorts, fmt.Sprintf("  { name: %s, asc: %s.%s.asc, desc: %s.%s.desc },", quote(c.Name), t.Name, c.Name, t.Name, c.Name))
	}
	sorts = append(sorts, "]")

	var headers, cells []string
	shown := cols
	if at.label.ID { // no text names a row: its id does, as the link to it
		shown = append([]*Column{at.label}, cols...)
	}
	for _, c := range shown {
		headers = append(headers, fmt.Sprintf("<th><a href={listAt model %s (toggle model %s) 1}>%s</a></th>", quote(c.Name), quote(c.Name), Humanize(c.Name)))
		cellText := g.adminCell(c, "row", imports)
		if c == at.label {
			cellText = "<a href={" + at.page("Show") + " { id: row." + t.IDField + " }}>{" + textOf(c, "row."+c.Name) + "}</a>"
		}
		cells = append(cells, "<td>"+cellText+"</td>")
	}

	filterCols := filterColumns(t)
	params := []string{"q: Text = \"\""}
	for _, c := range filterCols {
		params = append(params, c.Name+": Text = \"\"")
	}
	params = append(params, "sort: Text = "+quote(order), "dir: Text = \"desc\"", "page: Int = 1")

	var b strings.Builder
	b.WriteString(regionAt("", "params", wrapAll([]string{fmt.Sprintf("page %s { %s }%s", quote(at.path), strings.Join(params, ", "), adminRequires)})) + "\n\n")
	fmt.Fprintf(&b, "type Model = {\n  rows: List %s,\n  labels: Labels,\n  q: Text,\n  filters: Filters,\n  sort: Text,\n  dir: Text,\n  page: Int,\n  pages: Int,\n  count: Int,\n}\n\n", t.Name)
	b.WriteString("perPage : Int\nperPage = 25\n\n")
	b.WriteString(regionAt("", "search", search) + "\n\n")
	fmt.Fprintf(&b, `-- A Filter is one of the list's selects: the column it filters, the option chosen ("" for any),
-- and each option's value in the address, its label, and the rows it selects.
type Filter = {
  name: Text,
  label: Text,
  value: Text,
  options: List { value: Text, label: Text, rows: Condition %s },
}

`, t.Name)
	b.WriteString(regionAt("", "filters", g.filters(at, mod, filterCols, imports)) + "\n\n")
	b.WriteString(regionAt("", "sorts", sorts) + "\n\n")
	newest := "newest first"
	if order == t.IDField {
		newest = "by id"
	}
	fmt.Fprintf(&b, `-- order is the ordering sort names, ascending when dir is "asc"; a name that is not a column's
-- sorts `+newest+`.
order : Text -> Text -> Order %s
order sort dir = match List.find (\s -> s.name == sort) sorts {
  Just s -> if dir == "asc" then s.asc else s.desc,
  Nothing -> %s.%s.desc,
}

-- toggle is the direction a column's header sorts by: the other way round when the list is
-- sorted by that column ascending.
toggle : Model -> Text -> Text
toggle model sort = if model.sort == sort && model.dir == "asc" then "desc" else "asc"

-- load reads how many rows match the search and the filters, and one page of them (the last, for
-- a page past it), in the order asked for. The search text is cut to 200 characters (Text.slice):
-- a text over 10,000 bytes would fail the query (chapter 8, "Search").
`, t.Name, t.Name, order)
	b.WriteString(g.load(fmt.Sprintf(`  let q = Text.slice 0 200 params.q
  let f = chosen params
  let count <- Db.from %s |> Db.where (Db.and [search q, filtered (filters f)]) |> Db.count
  let pages = Int.max 1 ((count + perPage - 1) / perPage)
  let page = Int.clamp { min: 1, max: pages } params.page
  let rows <-
    Db.from %s
      |> Db.where (Db.and [search q, filtered (filters f)])
      |> Db.orderBy (order params.sort params.dir)
      |> Db.limit perPage
      |> Db.offset ((page - 1) * perPage)
      |> Db.all
  let labels <- labelsOf rows
  Query.succeed {
    rows: rows,
    labels: labels,
    q: q,
    filters: f,
    sort: params.sort,
    dir: params.dir,
    page: page,
    pages: pages,
    count: count,
  }`, t.Name, t.Name)))
	b.WriteString(regionAt("", "labels", wrapAll(g.labels(at, mod, imports))) + "\n\n")
	b.WriteString("view : Model -> Session -> Html Msg\nview model session =\n")
	fmt.Fprintf(&b, "  %s\n    <h1>%s</h1>\n\n", open, at.title)
	fmt.Fprintf(&b, `    <al-stack direction="row" gap="sm">
      <form method="get" action=%s>
        <input type="search" name="q" value={model.q} aria-label="Search" />
        {for f in filters model.filters ->
          <al-select label={f.label}>
            <label>{f.label}
              <select name={f.name}>
                <option value="" selected={f.value == ""}>any</option>
                {for o in f.options -> <option value={o.value} selected={o.value == f.value}>{o.label}</option>}
              </select>
            </label>
          </al-select>}
        <input type="hidden" name="sort" value={model.sort} />
        <input type="hidden" name="dir" value={model.dir} />
        <al-button type="submit" variant="secondary">Search</al-button>
      </form>
      <al-button href={%s {}}>New %s</al-button>
    </al-stack>

    <p id="count">{if model.count == 1 then "1 row" else Text.fromInt model.count ++ " rows"}</p>

    <al-table>
      <table>
        <thead>
          <tr>
%s
          </tr>
        </thead>
        <tbody>
          {for row in model.rows ->
            <tr>
%s
            </tr>}
        </tbody>
      </table>
    </al-table>

    <al-pagination role="navigation" aria-label="Pages">
      {if model.page > 1 then <a slot="prev" href={listAt model model.sort model.dir (model.page - 1)} rel="prev">Previous</a> else <span slot="prev" aria-disabled="true">Previous</span>}
      <span>Page {Text.fromInt model.page} of {Text.fromInt model.pages}</span>
      {if model.page < model.pages then <a slot="next" href={listAt model model.sort model.dir (model.page + 1)} rel="next">Next</a> else <span slot="next" aria-disabled="true">Next</span>}
    </al-pagination>
`, quote(at.path), at.page("New"), at.noun, regionAt("            ", "headers", headers), regionAt("              ", "cells", cells))
	fmt.Fprintf(&b, "  %s\n", close)

	// the tests
	var tests []TestData
	ids := func(v string) string { return "List.map (\\row -> row." + t.IDField + ") " + v + ".model.rows" }

	tb := g.tb(mod, imports, true)
	tb.insert(t, true)
	over, parents := g.selfParents(tb, at)
	tb.insertRow(t, "", over, true)
	checks := g.labelChecks(tb, at, "page.model", parents)
	tb.let("let page <- Test.get (%s {})", mod)
	lhs, rhs := []string{"rows: " + ids("page")}, []string{"rows: [" + strings.Join(tb.visible(t, false), ", ") + "]"}
	name := "the list shows every row, newest first"
	if t.Created == "" {
		lhs, rhs = []string{"rows: List.length page.model.rows"}, []string{fmt.Sprintf("rows: %d", len(tb.visible(t, false)))}
		name = "the list shows every row"
	}
	if len(checks[0]) > 0 {
		name += ", and the labels of the rows they refer to"
	}
	lhs, rhs = append(lhs, checks[0]...), append(rhs, checks[1]...)
	tests = append(tests, TestData{Name: quote(name), Lines: tb.lines,
		Result: "{ " + strings.Join(lhs, ", ") + " } == { " + strings.Join(rhs, ", ") + " }"})

	// sorting: a column whose order is the reverse of the rows' (and of every other column's)
	if sc := sortColumn(t); sc != nil {
		tb = g.tb(mod, imports, true)
		k := tb.counts[t.Module]
		first := tb.insertRow(t, "", map[string]string{sc.Name: tb.sample(sc.Type, sc.Name, k+2)}, true)
		second := tb.insertRow(t, "", map[string]string{sc.Name: tb.sample(sc.Type, sc.Name, k+1)}, true)
		tb.let("let up <- Test.get (%s)", pageLit(mod, []string{"sort: " + quote(sc.Name), "dir: \"asc\""}))
		tb.let("let down <- Test.get (%s)", pageLit(mod, []string{"sort: " + quote(sc.Name)}))
		others := slices.DeleteFunc(slices.Clone(tb.visible(t, false)), func(v string) bool { return v == first || v == second })
		// ascending: second (sample k+1) then first (k+2), the rows inserted before around them
		up := []string{second, first}
		down := []string{first, second}
		if len(others) > 0 { // rows inserted before (the signed-in user, for the user table): sorted among them
			up, down = sortedSample(others, first, second)
		}
		// a header sorts the other way round when the list is sorted by its column ascending
		otherCol := ""
		for _, c := range cols {
			if c != sc && otherCol == "" {
				otherCol = c.Name
			}
		}
		lhs := fmt.Sprintf("up: %s, down: %s, toggles: [toggle up.model %s, toggle down.model %s", ids("up"), ids("down"), quote(sc.Name), quote(sc.Name))
		rhs := fmt.Sprintf("up: [%s], down: [%s], toggles: [\"desc\", \"asc\"", strings.Join(up, ", "), strings.Join(down, ", "))
		if otherCol != "" {
			lhs += ", toggle up.model " + quote(otherCol)
			rhs += ", \"asc\""
		}
		tests = append(tests, TestData{Name: quote("sorting by " + sc.Name + " goes both ways"), Lines: tb.lines,
			Result: "{ " + lhs + "] } == { " + rhs + "] }"})
	}

	// search
	if len(searchCols) > 0 {
		sc := searchCols[0]
		tb = g.tb(mod, imports, true)
		tb.insert(t, true)
		match := tb.insert(t, true)
		q := rawText(sc, tb.counts[t.Module])
		tb.let("let page <- Test.get (%s)", pageLit(mod, []string{"q: " + quote(q)}))
		tests = append(tests, TestData{Name: quote("searching finds the rows that contain the text"), Lines: tb.lines,
			Result: fmt.Sprintf("{ rows: %s, count: Test.text \"#count\" page.doc } == { rows: [%s], count: Just \"1 row\" }", ids("page"), match)})
	}

	tests = append(tests, g.filterTests(at, mod, imports, filterCols)...)

	// a search text too long for the database is cut, not a failure
	tb = g.tb(mod, imports, true)
	tb.let("let long = Text.join \"\" (List.map (\\n -> \"z\") (List.range 1 20000))")
	tb.let("let page <- Test.get (%s)", pageLit(mod, []string{"q: long"}))
	tests = append(tests, TestData{Name: quote("a search text of 20,000 characters is cut to 200"), Lines: tb.lines,
		Result: "{ q: Text.length page.model.q, rows: List.length page.model.rows } == { q: 200, rows: 0 }"})

	// 25 rows a page
	tb = g.tb(mod, imports, true)
	tb.bulkInsert(t, 26, "")
	total := len(tb.visible(t, false)) + tb.bulk[t.Module]
	tb.let("let first <- Test.get (%s {})", mod)
	tb.let("let second <- Test.get (%s)", pageLit(mod, []string{"page: 2"}))
	tb.let("let past <- Test.get (%s)", pageLit(mod, []string{"page: 9"}))
	tests = append(tests, TestData{Name: quote("the list has 25 rows a page, and a page past the last is the last"), Lines: tb.lines,
		Result: fmt.Sprintf("{ first: List.length first.model.rows, second: List.length second.model.rows, past: { page: past.model.page, rows: List.length past.model.rows }, count: Test.text \"#count\" first.doc, pages: Test.texts \"al-pagination span\" first.doc } == { first: 25, second: %d, past: { page: %d, rows: %d }, count: Just \"%d rows\", pages: [\"Previous\", \"Page 1 of %d\"] }",
			total-25, (total+24)/25, total-25, total, (total+24)/25)})

	tests = append(tests, g.gateTests(mod, imports, func(*testBuilder) string { return mod + " {}" })...)
	b.WriteString(testsText(tests))
	g.file(mod, imports, b.String())
}

// filterColumns are the columns of t the list filters by, a select each: its Bool and union
// columns (not a secret, nor one whose name is another of the list's parameters).
func filterColumns(t *Table) []*Column {
	var out []*Column
	for _, c := range t.Columns {
		if (c.Kind == KindBool || c.Union != nil) && !c.ID && c.Ref == "" && !c.Secret() && !slices.Contains([]string{"q", "sort", "dir", "page"}, c.Name) {
			out = append(out, c)
		}
	}
	return out
}

// filterOption is an option of column c's filter: its value in the address, its label, and the
// value of the column it selects.
type filterOption struct{ value, label, col string }

// filterOptions are the options of column c's filter (after "any", which the view adds): a
// union's constructors, a Bool's yes and no; for a Maybe, the values in `Just`, then "none".
func filterOptions(c *Column) []filterOption {
	var out []filterOption
	if c.Union != nil {
		for _, ct := range c.Ctors {
			out = append(out, filterOption{ct, ct, ct})
		}
	} else {
		out = []filterOption{{"yes", "yes", "True"}, {"no", "no", "False"}}
	}
	if c.Maybe {
		for i := range out {
			out[i].col = "(Just " + out[i].col + ")"
		}
		out = append(out, filterOption{"none", "none", "Nothing"})
	}
	return out
}

// filters is the region `filters` of at's list: the type Filters (the option each filter has
// chosen), chosen (them, from the page's parameters), filters (each filter with its options) and
// listAt (a link to the list that keeps them).
func (g *adminGen) filters(at *adminTable, mod string, cols []*Column, imports *importSet) []string {
	t := at.t
	var fields, from, keep []string
	for _, c := range cols {
		fields = append(fields, c.Name+": Text")
		from = append(from, c.Name+": params."+c.Name)
		keep = append(keep, c.Name+": model.filters."+c.Name)
	}
	rec := func(xs []string) string {
		if len(xs) == 0 {
			return "{ }"
		}
		return "{ " + strings.Join(xs, ", ") + " }"
	}
	none := "."
	if len(cols) == 0 {
		none = ": this table has none."
	}
	out := []string{
		"-- Filters are the option each filter has chosen in the address (\"\" for any): a select for each",
		"-- Bool and union column" + none,
		"type Filters = " + rec(fields),
		"",
		"-- chosen is the filters' options in params.",
		"chosen : Params -> Filters",
		"chosen params = " + rec(from),
		"",
		"-- filters are the list's filters, each with the option f has chosen.",
		"filters : Filters -> List Filter",
	}
	if len(cols) == 0 {
		out = append(out, "filters f = []")
	} else {
		out = append(out, "filters f = [")
		for _, c := range cols {
			if c.Union != nil {
				imports.add(c.Union.Module)
			}
			out = append(out, fmt.Sprintf("  { name: %s, label: %s, value: f.%s, options: [", quote(c.Name), quote(Humanize(c.Name)), c.Name))
			for _, o := range filterOptions(c) {
				out = append(out, fmt.Sprintf("    { value: %s, label: %s, rows: %s.%s.eq %s },", quote(o.value), quote(o.label), t.Name, c.Name, o.col))
			}
			out = append(out, "  ] },")
		}
		out = append(out, "]")
	}
	out = append(out, "",
		"-- filtered is the rows that the option each filter has chosen selects (every row for \"\", or for",
		"-- a value that is no option's).",
		"filtered : List Filter -> Condition "+t.Name)
	if len(cols) == 0 {
		out = append(out, "filtered fs = Db.and []")
	} else {
		out = append(out, "filtered fs = Db.and (List.map (\\f -> match List.find (\\o -> o.value == f.value) f.options {",
			"  Just o -> o.rows,",
			"  Nothing -> Db.and [],",
			"}) fs)")
	}
	keep = append([]string{"q: model.q"}, append(keep, "sort: sort", "dir: dir", "page: n")...)
	out = append(out, "",
		"-- listAt is this list with the model's search and filters, sorted by sort and dir, at page n.",
		"listAt : Model -> Text -> Text -> Int -> Page",
		"listAt model sort dir n = "+mod+" "+rec(keep))
	return wrapAll(out)
}

// filterTests are the tests of the list's filters: for its first Bool column and its first union
// column, a row the option chosen selects is listed and counted, another is not, and the select
// shows the option chosen.
func (g *adminGen) filterTests(at *adminTable, mod string, imports *importSet, cols []*Column) []TestData {
	var tests []TestData
	for _, union := range []bool{false, true} {
		for _, c := range cols {
			if (c.Union != nil) != union {
				continue
			}
			tests = append(tests, g.filterTest(at, mod, imports, c))
			break
		}
	}
	return tests
}

// filterTest is the test of column c's filter: two rows, the first with the value of the option
// the test chooses, the second with another's; the first is listed and the second not, and the
// select shows the option chosen. A Bool's filter is chosen both ways (yes, then no); a union's
// at its last constructor (the first is the other row's).
func (g *adminGen) filterTest(at *adminTable, mod string, imports *importSet, c *Column) TestData {
	t := at.t
	tb := g.tb(mod, imports, true)
	opts := filterOptions(c)
	pick, other := opts[0], opts[1]
	if c.Union != nil {
		tb.typeRef(c.Elem)
		pick, other = opts[len(c.Ctors)-1], opts[0]
	}
	col := func(o filterOption) string { return strings.TrimSuffix(strings.TrimPrefix(o.col, "("), ")") }
	first := tb.insertRow(t, "", map[string]string{c.Name: col(pick)}, false)
	second := tb.insertRow(t, "", map[string]string{c.Name: col(other)}, false)
	listed := func(page string) string {
		return fmt.Sprintf("List.map (\\id -> List.member id (List.map (\\row -> row.%s) %s.model.rows)) [%s, %s]", t.IDField, page, first, second)
	}
	page := LowerFirst(strings.ReplaceAll(pick.value, " ", ""))
	tb.let("let %s <- Test.get (%s)", page, pageLit(mod, []string{c.Name + ": " + quote(pick.value)}))
	lhs := []string{page + ": " + listed(page)}
	rhs := []string{page + ": [True, False]"}
	if c.Union == nil {
		tb.let("let %s <- Test.get (%s)", other.value, pageLit(mod, []string{c.Name + ": " + quote(other.value)}))
		lhs = append(lhs, other.value+": "+listed(other.value))
		rhs = append(rhs, other.value+": [False, True]")
	}
	lhs = append(lhs, fmt.Sprintf("count: %s.model.count == List.length %s.model.rows", page, page),
		fmt.Sprintf("chosen: Test.texts \"select[name=%s] option[selected]\" %s.doc", c.Name, page))
	rhs = append(rhs, "count: True", "chosen: ["+quote(pick.label)+"]")
	return TestData{Name: quote("filtering by " + strings.ToLower(Humanize(c.Name)) + " lists the rows with that value"), Lines: tb.lines,
		Result: "{ " + strings.Join(lhs, ", ") + " } == { " + strings.Join(rhs, ", ") + " }"}
}

// sortColumn is the column the sort test sorts by: the first text (else number or date) column
// that is not an id or a reference, nil for none.
func sortColumn(t *Table) *Column {
	for _, kinds := range [][]Kind{{KindText}, {KindInt, KindFloat, KindDate}} {
		for _, c := range t.Columns {
			if !c.ID && c.Ref == "" && !c.Maybe && slices.Contains(kinds, c.Kind) && !c.Created && !c.Updated && !c.Secret() {
				return c
			}
		}
	}
	return nil
}

// sortedSample orders the rows others (newest first, inserted with sample values 1…k), first
// (k+2) and second (k+1) by the sorted column's sample values: ascending and descending.
func sortedSample(others []string, first, second string) ([]string, []string) {
	// others are newest first: the oldest has sample 1
	var up []string
	for i := len(others) - 1; i >= 0; i-- {
		up = append(up, others[i])
	}
	up = append(up, second, first)
	down := slices.Clone(up)
	slices.Reverse(down)
	return up, down
}

func (g *adminGen) show(at *adminTable) {
	t := at.t
	mod := at.page("Show")
	imports := newImports(mod, g.p.Std)
	imports.add(t.Module)
	importTypes(imports, t.IDType)
	title := UpperFirst(at.noun)
	open, close := g.layoutOpen(title, imports)
	var fields []string
	for _, c := range shown(t, true) {
		fields = append(fields, "<dt>"+Humanize(c.Name)+"</dt>", "<dd>"+g.adminCell(c, "model.row", imports)+"</dd>")
	}
	var b strings.Builder
	fmt.Fprintf(&b, "page %s { id: %s }%s\n\n", pathLit(at.path+"/{id}"), types.StringIn(t.IDType, mod), adminRequires)
	fmt.Fprintf(&b, "type Model = { row: %s, labels: Labels }\n\n", t.Name)
	b.WriteString(g.load(fmt.Sprintf(`  let found <- Db.get %s params.id
  match found {
    Just row -> {
      let labels <- labelsOf [row]
      Query.succeed { row: row, labels: labels }
    },
    Nothing -> Query.notFound,
  }`, t.Name)))
	b.WriteString(regionAt("", "labels", wrapAll(g.labels(at, mod, imports))) + "\n\n")
	b.WriteString("view : Model -> Session -> Html Msg\nview model session =\n")
	fmt.Fprintf(&b, "  %s\n    <h1>%s</h1>\n\n    <al-card>\n      <dl>\n%s\n      </dl>\n", open, title, regionAt("        ", "fields", fields))
	fmt.Fprintf(&b, `      <div slot="actions">
        <al-button href={%s { id: model.row.%s }}>Edit</al-button>
        <al-button variant="destructive" href={%s { id: model.row.%s }}>Delete</al-button>
      </div>
    </al-card>
    <a href={%s {}}>All %s</a>
`, at.page("Edit"), t.IDField, at.page("Delete"), t.IDField, at.page("Index"), Plural(at.noun))
	fmt.Fprintf(&b, "  %s\n", close)

	var tests []TestData
	tb := g.tb(mod, imports, true)
	over, parents := g.selfParents(tb, at)
	row := tb.insertRow(t, "", over, false)
	k := tb.counts[t.Module]
	checks := g.labelChecks(tb, at, "page.model", parents)
	tb.let("let r <- Test.open (%s)", pageLit(mod, []string{"id: " + row}))
	lhs, rhs := []string{"answer: r.answer", "fields: Test.count \"dd\" r.doc"}, []string{"answer: Shown", fmt.Sprintf("fields: %d", len(shown(t, true)))}
	if !at.label.ID {
		lhs[1], rhs[1] = fmt.Sprintf("%s: List.member %s (Test.texts \"dd\" r.doc)", at.label.Name, quote(rawText(at.label, k))), at.label.Name+": True"
	}
	if len(checks[0]) > 0 { // the model, for the labels it loaded
		tb.let("let page <- Test.get (%s)", pageLit(mod, []string{"id: " + row}))
	}
	lhs, rhs = append(lhs, checks[0]...), append(rhs, checks[1]...)
	tests = append(tests, TestData{Name: quote("the page shows the row"), Lines: tb.lines, Result: "{ " + strings.Join(lhs, ", ") + " } == { " + strings.Join(rhs, ", ") + " }"})
	tests = append(tests, g.missingTest(at, mod, imports))
	tests = append(tests, g.gateTests(mod, imports, func(tb *testBuilder) string {
		return pageLit(mod, []string{"id: " + tb.insertRow(t, "", nil, false)})
	})...)
	b.WriteString(testsText(tests))
	g.file(mod, imports, b.String())
}

// missingTest is the test that a page of a row that does not exist is a 404.
func (g *adminGen) missingTest(at *adminTable, mod string, imports *importSet) TestData {
	tb := g.tb(mod, imports, true)
	missing := tb.brandCtor(at.t.IDType.(*types.Brand)) + " " + quote("missing")
	tb.let("let r <- Test.open (%s)", pageLit(mod, []string{"id: " + missing}))
	return TestData{Name: quote("a missing row is not found"), Lines: tb.lines, Result: "r.answer == PageMissing"}
}

// formName is the name of the form of a table's New and Edit pages: "PostForm".
func (at *adminTable) formName() string { return at.t.Name + "Form" }

// optionsField is the model field holding the rows a reference column's select offers.
func optionsField(c *Column) string { return Plural(c.Name) }

// formParts are the regions a New or Edit page shares: the model's option fields, their loads,
// the form's declaration, its inputs (edit: with the row's values).
func (g *adminGen) formParts(at *adminTable, mod string, imports *importSet, edit bool) (model, loads, form, inputs []string) {
	for _, c := range at.refs {
		rt := g.p.TableNamed(c.Ref)
		imports.add(rt.Module)
		model = append(model, optionsField(c)+": List "+rt.Name+",")
		sel := "Db.from " + rt.Name
		if l := rt.Label(); !l.ID {
			sel += " |> Db.orderBy " + rt.Name + "." + l.Name + ".asc"
		}
		loads = append(loads, fmt.Sprintf("let %s <- %s |> Db.all", optionsField(c), sel))
	}
	form = append(form, "form "+at.formName()+" = {")
	for _, c := range at.editable {
		importTypes(imports, c.Type)
		form = append(form, "  "+c.Name+": "+c.TypeIn(mod)+",")
	}
	form = append(form, "}")
	for _, c := range at.editable {
		attrs := fmt.Sprintf("field=%s label=%s", quote(c.Name), quote(Humanize(c.Name)))
		tag := "al-input"
		switch {
		case c.Ref != "" && g.p.TableNamed(c.Ref) != nil:
			rt := g.p.TableNamed(c.Ref)
			tag = "al-select"
			attrs += fmt.Sprintf(" options={List.map (\\r -> { value: r.%s, label: r.%s }) model.%s}", rt.IDField, rt.Label().Name, optionsField(c))
		case c.Kind == KindUnion:
			tag = "al-select"
		case c.Kind == KindBool:
			tag = "al-checkbox"
		case c.Kind == KindText && slices.Contains([]string{"body", "message", "description", "content"}, c.Name):
			tag = "al-textarea"
		}
		switch {
		case edit:
			attrs += " value={model.row." + c.Name + "}"
		case c.HasDefault:
			if c.Union != nil {
				imports.add(c.Union.Module)
			}
			attrs += " value={" + c.Default + "}"
		}
		inputs = append(inputs, "<"+tag+" "+attrs+" />")
	}
	return
}

func (g *adminGen) newPage(at *adminTable) {
	t := at.t
	mod := at.page("New")
	imports := newImports(mod, g.p.Std)
	imports.add(t.Module)
	title := "New " + at.noun
	open, close := g.layoutOpen(UpperFirst(title), imports)
	model, loads, form, inputs := g.formParts(at, mod, imports, false)
	var fields []string
	for _, c := range at.refs {
		fields = append(fields, optionsField(c)+": "+optionsField(c))
	}
	loads = append(loads, "Query.succeed { "+strings.Join(fields, ", ")+" }")
	if len(fields) == 0 {
		loads[len(loads)-1] = "Query.succeed { }"
	}
	var insert []string
	var inits []string
	if len(at.nowCols) > 0 {
		insert = append(insert, "let now <- Time.now")
	}
	for _, c := range at.editable {
		inits = append(inits, c.Name+": p.form."+c.Name)
	}
	for _, c := range at.nowCols {
		inits = append(inits, c.Name+": now")
	}
	for _, c := range at.secrets {
		v, _ := secretValue(c)
		inits = append(inits, c.Name+": "+v)
	}
	if len(at.secrets) > 0 {
		var names []string
		for _, c := range at.secrets {
			names = append(names, c.Name)
		}
		insert = append(insert, "-- the admin never shows or writes a secret ("+strings.Join(names, ", ")+"): a row starts without one")
	}
	insert = append(insert, g.unique(at, "", []string{
		fmt.Sprintf("let id <- Db.insert %s { %s }", t.Name, strings.Join(inits, ", ")),
		fmt.Sprintf("Task.succeed (Redirect (%s { id: id }))", at.page("Show")),
	})...)

	var b strings.Builder
	fmt.Fprintf(&b, "page %s { }%s\n\n", quote(at.path+"/new"), adminRequires)
	b.WriteString("type Model = {\n" + regionAt("  ", "model", model) + "\n}\n\n")
	b.WriteString(g.load(regionAt("  ", "load", wrapAll(loads))))
	b.WriteString(regionAt("", "form", form) + "\n\n")
	fmt.Fprintf(&b, "msg Create form %s\n\n", at.formName())
	b.WriteString("update : Msg -> Model -> Session -> Task Nav\nupdate msg model session = match msg {\n  Create p -> {\n")
	b.WriteString(regionAt("    ", "insert", wrapAll(insert)) + "\n  },\n}\n\n")
	b.WriteString("view : Model -> Session -> Html Msg\nview model session =\n")
	fmt.Fprintf(&b, "  %s\n    <h1>%s</h1>\n\n    <al-card>\n      <form submit={Create}>\n%s\n", open, UpperFirst(title), regionAt("        ", "inputs", inputs))
	b.WriteString("        <al-alert variant=\"destructive\"><Errors form /></al-alert>\n        <al-button type=\"submit\">Create</al-button>\n      </form>\n    </al-card>\n")
	fmt.Fprintf(&b, "  %s\n", close)

	// the tests: a valid form adds the row, with the values submitted, and goes to its page
	var tests []TestData
	tb := g.tb(mod, imports, true)
	optLines := g.optionRows(tb, at)
	if len(at.uniques()) > 0 && tb.counts[t.Module] == 0 { // a row whose values the new one's differ from: no false "exists"
		tb.insertRow(t, "_", nil, false)
	}
	k := tb.counts[t.Module]
	var values, got, want []string
	for _, c := range at.editable {
		v := g.submitValue(tb, at, c, k+1)
		values = append(values, c.Name+": "+v)
		got = append(got, c.Name+": row."+c.Name)
		want = append(want, c.Name+": "+v)
	}
	for _, c := range at.secrets { // written empty
		v, _ := secretValue(c)
		got = append(got, c.Name+": row."+c.Name)
		want = append(want, c.Name+": "+v)
	}
	tb.let("let page <- Test.get (%s {})", mod)
	tb.let("let r <- Test.submit %s { %s }", at.formName(), strings.Join(values, ", "))
	sel := "Db.from " + t.Name
	if k > 0 && !at.label.ID { // rows of the table were inserted before (the signed-in user): the new one by its label
		v := valueOf(values, at.label.Name)
		if strings.Contains(v, " ") {
			v = "(" + v + ")"
		}
		sel += fmt.Sprintf(" |> Db.where (%s.%s.eq %s)", t.Name, at.label.Name, v)
	}
	tb.let("let rows <- %s |> Db.all", sel)
	lhs := []string{fmt.Sprintf("answers: List.map (\\row -> Redirected (%s { id: row.%s })) rows", at.page("Show"), t.IDField),
		"rows: List.map (\\row -> { " + strings.Join(got, ", ") + " }) rows"}
	rhs := []string{"answers: [r.answer]", "rows: [{ " + strings.Join(want, ", ") + " }]"}
	lhs, rhs = append(lhs, optLines[0]...), append(rhs, optLines[1]...)
	if len(at.editable)+len(at.secrets) == 0 {
		lhs, rhs = lhs[:1], rhs[:1]
	}
	tests = append(tests, TestData{Name: quote("creating " + article(at.noun) + " adds it, with the values submitted"), Lines: tb.lines,
		Result: "{ " + strings.Join(lhs, ", ") + " } == { " + strings.Join(rhs, ", ") + " }"})
	tests = append(tests, g.uniqueTests(at, mod, imports, false)...)
	tests = append(tests, g.notOffered(at, mod, imports, false)...)
	tests = append(tests, g.gateTests(mod, imports, func(*testBuilder) string { return mod + " {}" })...)
	b.WriteString(testsText(tests))
	g.file(mod, imports, b.String())
}

// notOffered are the tests that a New (or Edit) page refuses, for each reference, a row id its
// select did not offer (a request crafted by hand): the field error "must be one of the
// options" (chapter 5), and nothing written. The other fields are sent valid: a reference the
// first option's id, read from the page.
func (g *adminGen) notOffered(at *adminTable, mod string, imports *importSet, edit bool) []TestData {
	var tests []TestData
	for _, c := range at.refs {
		tb := g.tb(mod, imports, true)
		page := mod + " {}"
		target := ""
		if edit {
			target = tb.insertRow(at.t, "", nil, false)
			page = pageLit(mod, []string{"id: " + target})
		}
		g.optionRows(tb, at)
		tb.let("let page <- Test.get (%s)", page)
		var raw []string
		for _, d := range at.editable {
			v := quote(rawSample(d.Type))
			switch {
			case d == c:
				v = quote("missing")
			case d.Ref != "" && g.p.TableNamed(d.Ref) != nil && !d.Maybe:
				v = d.Name + "Id"
				tb.let("let %s = Maybe.withDefault \"\" (Test.attr \"value\" \"select[name=%s] option\" page.doc)", v, d.Name)
			}
			raw = append(raw, fmt.Sprintf("{ name: %s, value: %s }", quote(d.Name), v))
		}
		tb.let("let r <- Test.submitRaw %s [%s]", at.formName(), strings.Join(raw, ", "))
		rejected := fmt.Sprintf("FormRejected [{ field: %s, message: \"must be one of the options\" }]", quote(c.Name))
		result := ""
		if edit {
			tb.let("let after <- Db.get %s %s", at.t.Name, target)
			was := "Nothing" // the row's value, as the test inserted it: a Maybe's Nothing, else the first row
			if !c.Maybe {
				was = tb.rows[g.p.TableNamed(c.Ref).Module][0].v
			}
			result = fmt.Sprintf("{ answer: r.answer, %s: Maybe.map (\\row -> row.%s) after } == { answer: %s, %s: Just %s }",
				c.Name, c.Name, rejected, c.Name, was)
		} else {
			tb.let("let rows <- Db.from %s |> Db.count", at.t.Name)
			result = fmt.Sprintf("{ answer: r.answer, rows: rows } == { answer: %s, rows: %d }", rejected, tb.counts[at.t.Module])
		}
		tests = append(tests, TestData{Name: quote(article(strings.ToLower(Humanize(c.Name))) + " not offered is refused"), Lines: tb.lines, Result: result})
	}
	return tests
}

// uniques are the columns of at's forms that are `@unique`.
func (at *adminTable) uniques() []*Column {
	var out []*Column
	for _, c := range at.editable {
		if c.Unique {
			out = append(out, c)
		}
	}
	return out
}

// takenText is the refusal of a form whose `@unique` column c holds a value another row has.
func takenText(at *adminTable, c *Column) string {
	return UpperFirst(article(at.noun)) + " with this " + strings.ToLower(Humanize(c.Name)) + " exists."
}

// unique is an arm's body, write (the write and the redirect), guarded for each `@unique`
// column of at's form by a count of the rows that already hold the submitted value (other than
// the row self, on an Edit page): such a value answers `Reject`, which the form shows, where the
// database would refuse the write with an error (a 500).
func (g *adminGen) unique(at *adminTable, self string, write []string) []string {
	cols := at.uniques()
	if len(cols) == 0 {
		return write
	}
	t := at.t
	var out []string
	for _, c := range cols {
		conds := []string{t.Name + "." + c.Name + ".eq p.form." + c.Name}
		if c.Maybe { // no value is never taken: `.ne Nothing` matches no row without one
			conds = append(conds, t.Name+"."+c.Name+".ne Nothing")
		}
		if self != "" {
			conds = append(conds, t.Name+"."+t.IDField+".ne "+self)
		}
		cond := conds[0]
		if len(conds) > 1 {
			cond = "Db.and [" + strings.Join(conds, ", ") + "]"
		}
		out = append(out, fmt.Sprintf("let %sTaken <- Db.from %s |> Db.where (%s) |> Db.count", c.Name, t.Name, cond))
	}
	for i, c := range cols {
		lead := "else if"
		if i == 0 {
			lead = "if"
		}
		out = append(out, fmt.Sprintf("%s %sTaken > 0 then", lead, c.Name), "  Task.succeed (Reject "+quote(takenText(at, c))+")")
	}
	out = append(out, "else {")
	out = append(out, indentAll(write, "  ")...)
	return append(out, "}")
}

// uniqueTests are the tests that a New (or Edit) page refuses a value of a `@unique` column
// that another row holds: the form's error, and nothing written.
func (g *adminGen) uniqueTests(at *adminTable, mod string, imports *importSet, edit bool) []TestData {
	var tests []TestData
	t := at.t
	for _, c := range at.uniques() {
		tb := g.tb(mod, imports, true)
		page := mod + " {}"
		target := ""
		if edit {
			target = tb.insertRow(t, "", nil, false)
			page = pageLit(mod, []string{"id: " + target})
		}
		g.optionRows(tb, at)
		taken := g.submitValue(tb, at, c, tb.counts[t.Module]+1)
		tb.insertRow(t, "_", map[string]string{c.Name: taken}, false)
		k := tb.counts[t.Module]
		var values []string
		for _, d := range at.editable {
			v := g.submitValue(tb, at, d, k+1)
			if edit {
				v = g.changedValue(tb, at, d, k+1)
			}
			if d == c {
				v = taken
			}
			values = append(values, d.Name+": "+v)
		}
		if edit {
			tb.let("let before <- Db.get %s %s", t.Name, target)
		}
		tb.let("let _ <- Test.get (%s)", page)
		tb.let("let r <- Test.submit %s { %s }", at.formName(), strings.Join(values, ", "))
		rejected := fmt.Sprintf("FormRejected [{ field: \"\", message: %s }]", quote(takenText(at, c)))
		col := strings.ToLower(Humanize(c.Name))
		if edit {
			tb.let("let after <- Db.get %s %s", t.Name, target)
			tests = append(tests, TestData{Name: quote("saving " + article(col) + " another " + at.noun + " has is refused"), Lines: tb.lines,
				Result: fmt.Sprintf("{ answer: r.answer, row: after } == { answer: %s, row: before }", rejected)})
			continue
		}
		tb.let("let rows <- Db.from %s |> Db.count", t.Name)
		tests = append(tests, TestData{Name: quote(article(at.noun) + " with " + article(col) + " that exists is refused"), Lines: tb.lines,
			Result: fmt.Sprintf("{ answer: r.answer, rows: rows } == { answer: %s, rows: %d }", rejected, k)})
	}
	return tests
}

// valueOf is the value given to field name among values ("name: value").
func valueOf(values []string, name string) string {
	for _, v := range values {
		if n, val, _ := strings.Cut(v, ": "); n == name {
			return val
		}
	}
	return ""
}

// optionRows inserts rows of each table at's forms refer to until there are two (the first the
// one the memo keeps, so the row a test inserted before refers to it), and returns the
// comparisons that each select offers them, in label order (after an empty option for a
// Maybe): the left and right sides' fields.
func (g *adminGen) optionRows(tb *testBuilder, at *adminTable) [2][]string {
	var out [2][]string
	for _, c := range at.refs {
		rt := g.p.TableNamed(c.Ref)
		for len(tb.rows[rt.Module]) < 2 {
			tb.insertRow(rt, "", nil, true)
		}
		var labels []string
		if c.Maybe {
			labels = append(labels, `""`)
		}
		for i := range tb.rows[rt.Module] {
			labels = append(labels, quote(rawText(rt.Label(), i+1)))
		}
		if rt.Label().ID {
			out[0] = append(out[0], fmt.Sprintf("%s: Test.count \"select[name=%s] option\" page.doc", optionsField(c), c.Name))
			out[1] = append(out[1], fmt.Sprintf("%s: %d", optionsField(c), len(labels)))
			continue
		}
		out[0] = append(out[0], fmt.Sprintf("%s: Test.texts \"select[name=%s] option\" page.doc", optionsField(c), c.Name))
		out[1] = append(out[1], fmt.Sprintf("%s: [%s]", optionsField(c), strings.Join(labels, ", ")))
	}
	return out
}

// submitValue is the value a test submits for column c of at's form: a sample (the n-th), a
// reference's newest row, a union's last constructor, True for a Bool.
func (g *adminGen) submitValue(tb *testBuilder, at *adminTable, c *Column, n int) string {
	switch {
	case c.Ref != "" && g.p.TableNamed(c.Ref) != nil:
		rows := tb.rows[g.p.TableNamed(c.Ref).Module]
		v := rows[len(rows)-1].v
		if c.Maybe {
			return "Just " + v
		}
		return v
	case c.Union != nil:
		tb.typeRef(c.Elem)
		v := c.Ctors[len(c.Ctors)-1]
		if c.Maybe {
			return "Just " + v
		}
		return v
	case c.Kind == KindBool && !c.Maybe:
		return "True"
	case c.Maybe:
		v := tb.sample(c.Elem, c.Name, n)
		if strings.Contains(v, " ") {
			v = "(" + v + ")"
		}
		return "Just " + v
	}
	return tb.sample(c.Type, c.Name, n)
}

func (g *adminGen) edit(at *adminTable) {
	t := at.t
	mod := at.page("Edit")
	imports := newImports(mod, g.p.Std)
	imports.add(t.Module)
	importTypes(imports, t.IDType)
	title := "Edit " + at.noun
	open, close := g.layoutOpen(UpperFirst(title), imports)
	model, loads, form, inputs := g.formParts(at, mod, imports, true)
	fields := []string{"row: row"}
	for _, c := range at.refs {
		fields = append(fields, optionsField(c)+": "+optionsField(c))
	}
	loads = append([]string{"let found <- Db.get " + t.Name + " params.id"}, loads...)
	loads = append(loads, "match found {", "  Just row -> Query.succeed { "+strings.Join(fields, ", ")+" },", "  Nothing -> Query.notFound,", "}")
	var sets []string
	for _, c := range at.editable {
		sets = append(sets, c.Name+": p.form."+c.Name)
	}
	upd := fmt.Sprintf("let _ <- Db.update %s model.row.%s (\\row -> { row | %s })", t.Name, t.IDField, strings.Join(sets, ", "))
	if len(sets) == 0 {
		upd = fmt.Sprintf("let _ <- Db.update %s model.row.%s (\\row -> row)", t.Name, t.IDField)
	}

	var b strings.Builder
	fmt.Fprintf(&b, "page %s { id: %s }%s\n\n", pathLit(at.path+"/{id}/edit"), types.StringIn(t.IDType, mod), adminRequires)
	b.WriteString("type Model = {\n  row: " + t.Name + ",\n" + regionAt("  ", "model", model) + "\n}\n\n")
	b.WriteString(g.load(regionAt("  ", "load", wrapAll(loads))))
	b.WriteString(regionAt("", "form", form) + "\n\n")
	fmt.Fprintf(&b, "msg Save form %s\n\n", at.formName())
	b.WriteString("update : Msg -> Model -> Session -> Task Nav\nupdate msg model session = match msg {\n  Save p -> {\n")
	save := g.unique(at, "model.row."+t.IDField, []string{upd, fmt.Sprintf("Task.succeed (Redirect (%s { id: model.row.%s }))", at.page("Show"), t.IDField)})
	role := g.ownRole(at)
	if role != nil { // the signed-in user's own role is not changed: an Admin would lock themselves out
		save = append([]string{
			fmt.Sprintf("if %s && p.form.%s != model.row.%s then", g.isMe("model.row."+t.IDField), role.Name, role.Name),
			"  Task.succeed (Reject " + quote(roleText) + ")",
			"else {",
		}, append(indentAll(save, "  "), "}")...)
	}
	b.WriteString(regionAt("    ", "update", wrapAll(save)) + "\n  },\n}\n\n")
	b.WriteString("view : Model -> Session -> Html Msg\nview model session =\n")
	fmt.Fprintf(&b, "  %s\n    <h1>%s</h1>\n\n    <al-card>\n      <form submit={Save}>\n%s\n", open, UpperFirst(title), regionAt("        ", "inputs", inputs))
	b.WriteString("        <al-alert variant=\"destructive\"><Errors form /></al-alert>\n        <al-button type=\"submit\">Save</al-button>\n      </form>\n    </al-card>\n")
	fmt.Fprintf(&b, "    <a href={%s { id: model.row.%s }}>Back</a>\n", at.page("Show"), t.IDField)
	fmt.Fprintf(&b, "  %s\n", close)

	// the tests: every field changed, each to a value it did not have
	var tests []TestData
	tb := g.tb(mod, imports, true)
	target := tb.insertRow(t, "", nil, false)
	optLines := g.optionRows(tb, at)
	k := tb.counts[t.Module]
	var values, got, want []string
	for _, c := range at.editable {
		v := g.changedValue(tb, at, c, k+1)
		values = append(values, c.Name+": "+v)
		got = append(got, c.Name+": row."+c.Name)
		want = append(want, c.Name+": "+v)
	}
	tb.let("let page <- Test.get (%s)", pageLit(mod, []string{"id: " + target}))
	tb.let("let r <- Test.submit %s { %s }", at.formName(), strings.Join(values, ", "))
	tb.let("let after <- Db.get %s %s", t.Name, target)
	lhs := []string{"answer: r.answer", "row: Maybe.map (\\row -> { " + strings.Join(got, ", ") + " }) after"}
	rhs := []string{fmt.Sprintf("answer: Redirected (%s { id: %s })", at.page("Show"), target), "row: Just { " + strings.Join(want, ", ") + " }"}
	if len(at.editable) == 0 {
		lhs, rhs = lhs[:1], rhs[:1]
	}
	lhs, rhs = append(lhs, optLines[0]...), append(rhs, optLines[1]...)
	tests = append(tests, TestData{Name: quote("saving changes every field"), Lines: tb.lines,
		Result: "{ " + strings.Join(lhs, ", ") + " } == { " + strings.Join(rhs, ", ") + " }"})
	tests = append(tests, g.uniqueTests(at, mod, imports, true)...)
	if role != nil {
		tests = append(tests, g.ownRoleTests(at, mod, imports, role)...)
	}
	tests = append(tests, g.notOffered(at, mod, imports, true)...)
	tests = append(tests, g.missingTest(at, mod, imports))
	tests = append(tests, g.gateTests(mod, imports, func(tb *testBuilder) string {
		return pageLit(mod, []string{"id: " + tb.insertRow(t, "", nil, false)})
	})...)
	b.WriteString(testsText(tests))
	g.file(mod, imports, b.String())
}

// ownRole is the role column of the user table at, which its Edit page refuses to change on the
// signed-in user's own row; nil for another table, or a role the form does not edit.
func (g *adminGen) ownRole(at *adminTable) *Column {
	if !g.isUserTable(at) || g.role == nil || !slices.Contains(at.editable, g.role) {
		return nil
	}
	return g.role
}

// ownRoleTests are the tests of the Edit page's guard on the signed-in user's own role (Review
// Focus 4 of M13): an Admin changing it is refused and nothing is written; an Admin saving their
// own row with the role unchanged, and an Admin demoting another Admin, are not refused.
func (g *adminGen) ownRoleTests(at *adminTable, mod string, imports *importSet, role *Column) []TestData {
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
	tb.let("let _ <- Test.get (%s)", pageLit(mod, []string{"id: " + me}))
	tb.let("let r <- Test.submit %s { %s }", at.formName(), values(tb, other))
	tb.let("let after <- Db.get %s %s", t.Name, me)
	tests = append(tests, TestData{Name: quote("an Admin cannot change their own role"), Lines: tb.lines,
		Result: fmt.Sprintf("{ answer: r.answer, row: after } == { answer: FormRejected [{ field: \"\", message: %s }], row: before }", quote(roleText))})

	tb = g.tb(mod, imports, true)
	me = tb.user()
	g.optionRows(tb, at)
	tb.let("let _ <- Test.get (%s)", pageLit(mod, []string{"id: " + me}))
	vs := values(tb, "Admin")
	tb.let("let r <- Test.submit %s { %s }", at.formName(), vs)
	tb.let("let after <- Db.get %s %s", t.Name, me)
	var got, want []string
	for _, c := range at.editable {
		got = append(got, c.Name+": row."+c.Name)
		want = append(want, c.Name+": "+valueOf(strings.Split(vs, ", "), c.Name))
	}
	tests = append(tests, TestData{Name: quote("an Admin saves their own account, the role unchanged"), Lines: tb.lines,
		Result: fmt.Sprintf("{ answer: r.answer, row: Maybe.map (\\row -> { %s }) after } == { answer: Redirected (%s { id: %s }), row: Just { %s } }",
			strings.Join(got, ", "), at.page("Show"), me, strings.Join(want, ", "))})

	tb = g.tb(mod, imports, true)
	tb.user()
	admin := tb.insertRow(t, "admin", map[string]string{role.Name: maybeJust(role, "Admin")}, false)
	g.optionRows(tb, at)
	tb.let("let _ <- Test.get (%s)", pageLit(mod, []string{"id: " + admin}))
	tb.let("let r <- Test.submit %s { %s }", at.formName(), values(tb, other))
	tb.let("let after <- Db.get %s %s", t.Name, admin)
	tests = append(tests, TestData{Name: quote("an Admin demotes another Admin"), Lines: tb.lines,
		Result: fmt.Sprintf("{ answer: r.answer, %s: Maybe.map (\\row -> row.%s) after } == { answer: Redirected (%s { id: %s }), %s: Just %s }",
			role.Name, role.Name, at.page("Show"), admin, role.Name, maybeJust(role, other))})
	return tests
}

// changedValue is a value for column c of the row a test inserted (the n-1-th sample) that
// differs from the one it has: the n-th sample, the newest row of a reference, another
// constructor, the other Bool, `Just` for a Maybe.
func (g *adminGen) changedValue(tb *testBuilder, at *adminTable, c *Column, n int) string {
	switch {
	case c.Kind == KindBool && !c.Maybe:
		if c.HasDefault && c.Default == "True" {
			return "False"
		}
		if c.HasDefault {
			return "True"
		}
		return "False" // a required Bool's sample is True
	case c.Kind == KindDate && !c.Maybe:
		tb.sample(c.Type, c.Name, n)
		return "Date.addDays 1 today"
	case c.HasDefault && !c.Maybe && c.Union == nil:
		// a column the test's insert left at its default: a sample that is not the default
		for v := g.submitValue(tb, at, c, n); ; n++ {
			if v != c.Default {
				return v
			}
			v = g.submitValue(tb, at, c, n+1)
		}
	case c.Union != nil && c.Ref == "":
		old := c.Ctors[0] // the sample
		if c.HasDefault && !c.Maybe {
			old = c.Default
		}
		tb.typeRef(c.Elem)
		for i := len(c.Ctors) - 1; i >= 0; i-- {
			if c.Ctors[i] != old || c.Maybe {
				if c.Maybe {
					return "Just " + c.Ctors[i]
				}
				return c.Ctors[i]
			}
		}
	}
	return g.submitValue(tb, at, c, n)
}

func (g *adminGen) delete(at *adminTable) {
	t := at.t
	mod := at.page("Delete")
	imports := newImports(mod, g.p.Std)
	imports.add(t.Module)
	importTypes(imports, t.IDType)
	title := "Delete " + at.noun
	open, close := g.layoutOpen(UpperFirst(title), imports)

	var model, loads, counts, blockers, children []string
	fields := []string{"row: row"}
	var chains []*node // the @restrict references down a @cascade, each counted
	nodes := g.closure(at)
	for _, n := range walk(nodes) {
		imports.add(n.t.Module)
		model = append(model, n.field+": Int,")
		cond := eqID(n.t, n.col, "params.id")
		if n.parent != nil { // the rows that refer to the rows its parent deletes
			ids := "List.map (\\row -> row." + n.parent.t.IDField + ") " + n.parent.field + "Rows"
			if n.col.Maybe {
				ids = "List.map (\\row -> Just row." + n.parent.t.IDField + ") " + n.parent.field + "Rows"
			}
			cond = fmt.Sprintf("%s.%s.in (%s)", n.t.Name, n.col.Name, ids)
		}
		sel := fmt.Sprintf("Db.from %s |> Db.where (%s)", n.t.Name, cond)
		if len(n.kids) == 0 {
			loads = append(loads, fmt.Sprintf("let %s <- %s |> Db.count", n.field, sel))
			fields = append(fields, n.field+": "+n.field)
		} else { // the rows themselves: the rows that refer to them are counted too
			loads = append(loads, fmt.Sprintf("let %sRows <- %s |> Db.all", n.field, sel))
			fields = append(fields, n.field+": List.length "+n.field+"Rows")
		}
		var what string
		switch {
		case n.stop && n.parent == nil:
			what = UpperFirst(pluralOf(n.t)) + " under it, to delete first"
		case n.stop:
			what = UpperFirst(pluralOf(n.t)) + " under those " + pluralOf(n.parent.t) + ", to delete first"
		case n.parent == nil:
			what = fmt.Sprintf(map[string]string{
				"cascade":  "%s deleted with it",
				"setNull":  "%s that lose their " + strings.ToLower(Humanize(n.col.Name)),
				"restrict": "%s that keep it from being deleted",
			}[n.col.OnDelete], UpperFirst(pluralOf(n.t)))
		default:
			what = UpperFirst(pluralOf(n.t)) + " of those " + pluralOf(n.parent.t) + map[string]string{
				"cascade":  " deleted with it",
				"restrict": " that keep it from being deleted",
			}[n.col.OnDelete]
		}
		counts = append(counts, "<dt>"+what+"</dt>", fmt.Sprintf("<dd id=%s>{Text.fromInt model.%s}</dd>", quote(n.field), n.field))
		switch {
		case n.stop && n.parent == nil:
			children = append(children, "model."+n.field)
		case n.col.OnDelete == "restrict" || n.stop:
			blockers = append(blockers, "model."+n.field)
			if n.parent != nil {
				chains = append(chains, n)
			}
		}
	}
	own := g.isUserTable(at)
	if own { // the signed-in user's own row is not deleted
		model = append([]string{"own: Bool,"}, model...)
		fields = append([]string{fields[0], "own: " + g.isMe("row."+t.IDField)}, fields[1:]...)
	}
	loads = append([]string{"let found <- Db.get " + t.Name + " params.id"}, loads...)
	loads = append(loads, "match found {", "  Just row -> Query.succeed { "+strings.Join(fields, ", ")+" },", "  Nothing -> Query.notFound,", "}")
	block := []string{
		"-- blockers counts the rows that keep this one from being deleted, which the database would",
		"-- refuse the delete for: the `@restrict` references to it, or to a row deleted with it.",
		"blockers : Model -> Int",
		"blockers model = 0",
	}
	if len(blockers) > 0 {
		block[3] = "blockers model = " + strings.Join(blockers, " + ")
	}
	if len(children) > 0 {
		block = append(block, "",
			"-- children counts the rows under this one, which refer to it with a `@cascade` to its own",
			"-- table: deleting it would delete them, theirs, and so on to any depth, past what the counts",
			"-- above can follow, to a row a `@restrict` reference may keep. So only a row without them is",
			"-- deleted.",
			"children : Model -> Int",
			"children model = "+strings.Join(children, " + "))
	}
	labelRow := "<p>{" + textOf(at.label, "model.row."+at.label.Name) + "}</p>"

	var b strings.Builder
	fmt.Fprintf(&b, "page %s { id: %s }%s\n\n", pathLit(at.path+"/{id}/delete"), types.StringIn(t.IDType, mod), adminRequires)
	b.WriteString("-- The row, and how many rows of each table refer to it.\n")
	b.WriteString("type Model = {\n  row: " + t.Name + ",\n" + regionAt("  ", "model", model) + "\n}\n\n")
	b.WriteString(g.load(regionAt("  ", "load", wrapAll(loads))))
	b.WriteString(regionAt("", "blockers", block) + "\n\n")
	b.WriteString("-- Delete takes no input: the form is the button, and the place a refusal is shown.\nform Confirm = { }\n\nmsg Remove form Confirm\n\n")
	remove := []string{
		"let _ <- Db.delete " + t.Name + " model.row." + t.IDField,
		"Task.succeed (Redirect (" + at.page("Index") + " {}))",
	}
	if len(blockers) > 0 || len(children) > 0 || own { // refused while rows refer to it: the database would refuse with an error
		var guard []string
		lead := func() string {
			if len(guard) > 0 {
				return "else if"
			}
			return "if"
		}
		if own {
			guard = append(guard, "if model.own then", "  Task.succeed (Reject "+quote(ownText)+")")
		}
		if len(children) > 0 {
			guard = append(guard, lead()+" children model > 0 then", "  Task.succeed (Reject "+quote(childrenText)+")")
		}
		if len(blockers) > 0 {
			guard = append(guard, lead()+" blockers model > 0 then", "  Task.succeed (Reject "+quote(rejectText(at))+")")
		}
		remove = append(append(guard, "else {"), append(indentAll(remove, "  "), "}")...)
	}
	b.WriteString(`-- Remove deletes the row, unless rows refer to it that keep it (load counted them, as it does
-- before every message): the database would refuse the delete with an error, and the page says
-- why instead.
update : Msg -> Model -> Session -> Task Nav
update msg model session = match msg {
  Remove p -> {
` + regionAt("    ", "remove", wrapAll(remove)) + `
  },
}

`)
	b.WriteString("view : Model -> Session -> Html Msg\nview model session =\n")
	fmt.Fprintf(&b, "  %s\n    <h1>Delete this %s?</h1>\n\n    <al-card>\n%s\n      <dl>\n%s\n      </dl>\n", open, at.noun, regionAt("      ", "row", []string{labelRow}), regionAt("        ", "counts", counts))
	button := `{if blockers model > 0 then <p>It cannot be deleted while other rows refer to it.</p> else <al-button variant="destructive" type="submit">Delete</al-button>}`
	if len(children) > 0 || own { // one line per reason
		var arms []string
		if own {
			arms = append(arms, "model.own then <p>"+ownText+"</p>")
		}
		if len(children) > 0 {
			arms = append(arms, "children model > 0 then <p>"+childrenText+"</p>")
		}
		arms = append(arms, "blockers model > 0 then <p>It cannot be deleted while other rows refer to it.</p>")
		button = "{if " + strings.Join(arms, "\n          else if ") + "\n          else <al-button variant=\"destructive\" type=\"submit\">Delete</al-button>}"
	}
	fmt.Fprintf(&b, `      <form id="delete" submit={Remove}>
        <al-alert variant="destructive"><Errors form /></al-alert>
        `+button+`
      </form>
    </al-card>
    <a href={%s { id: model.row.%s }}>Back</a>
`, at.page("Show"), t.IDField)
	fmt.Fprintf(&b, "  %s\n", close)

	// the tests
	var tests []TestData
	// a row nothing refers to is deleted; another row, with rows of every table referring to
	// it, is not
	tb := g.tb(mod, imports, true)
	target := tb.insertRow(t, "", nil, false)
	other := ""
	if len(at.referrer) > 0 {
		other = tb.insertRow(t, "", nil, true)
		g.referrers(tb, at, other)
	}
	tb.let("let _ <- Test.get (%s)", pageLit(mod, []string{"id: " + target}))
	tb.let("let r <- Test.submit Confirm { }")
	tb.let("let gone <- Db.get %s %s", t.Name, target)
	lhs := []string{"answer: r.answer", "gone: Maybe.map (\\row -> row." + t.IDField + ") gone"}
	rhs := []string{"answer: Redirected (" + at.page("Index") + " {})", "gone: Nothing"}
	if other != "" {
		tb.let("let kept <- Db.get %s %s", t.Name, other)
		lhs = append(lhs, "kept: Maybe.map (\\row -> row."+t.IDField+") kept")
		rhs = append(rhs, "kept: Just "+other)
	}
	tests = append(tests, TestData{Name: quote("deleting " + article(at.noun) + " nothing refers to"), Lines: tb.lines,
		Result: "{ " + strings.Join(lhs, ", ") + " } == { " + strings.Join(rhs, ", ") + " }"})

	// what goes with it: the cascade and setNull references counted, another row's not
	var going []*referrer
	for _, r := range at.referrer {
		if r.col.OnDelete != "restrict" {
			going = append(going, r)
		}
	}
	if len(going) > 0 {
		tb = g.tb(mod, imports, true)
		target := tb.insertRow(t, "", nil, true)
		for _, r := range going {
			tb.insertRow(r.t, "_", map[string]string{r.col.Name: maybeJust(r.col, target)}, false)
		}
		other := tb.insertRow(t, "", nil, false)
		for _, r := range going {
			tb.insertRow(r.t, "_", map[string]string{r.col.Name: maybeJust(r.col, other)}, false)
		}
		tb.let("let page <- Test.get (%s)", pageLit(mod, []string{"id: " + target}))
		var l, rr []string
		for _, r := range going {
			l = append(l, fmt.Sprintf("%s: Test.text \"#%s\" page.doc", r.field, r.field))
			rr = append(rr, r.field+": Just \"1\"")
		}
		tests = append(tests, TestData{Name: quote("the page counts the rows that go with it"), Lines: tb.lines,
			Result: "{ " + strings.Join(l, ", ") + " } == { " + strings.Join(rr, ", ") + " }"})
	}
	// each @restrict reference refuses: the count shown, no button, the message rejected
	for _, r := range at.referrer {
		if r.col.OnDelete != "restrict" {
			continue
		}
		tb = g.tb(mod, imports, true)
		target := tb.insertRow(t, "", nil, true)
		tb.insertRow(r.t, "_", map[string]string{r.col.Name: maybeJust(r.col, target)}, false)
		tb.let("let page <- Test.get (%s)", pageLit(mod, []string{"id: " + target}))
		tb.let("let r <- Test.submit Confirm { }")
		tb.let("let kept <- Db.get %s %s", t.Name, target)
		tests = append(tests, TestData{Name: quote(article(at.noun) + " that " + Plural(strings.ToLower(Humanize(r.t.Name))) + " refer to is not deleted"), Lines: tb.lines,
			Result: fmt.Sprintf("{ count: Test.text \"#%s\" page.doc, buttons: Test.count \"#delete button\" page.doc, answer: r.answer, kept: Maybe.map (\\row -> row.%s) kept } == { count: Just \"1\", buttons: 0, answer: FormRejected [{ field: \"\", message: %s }], kept: Just %s }",
				r.field, t.IDField, quote(rejectText(at)), target)})
	}
	// a @restrict reference down a @cascade, any number of steps, refuses too
	for _, c := range chains {
		tb = g.tb(mod, imports, true)
		target := tb.insertRow(t, "", nil, true)
		var path []*node
		for n := c.parent; n != nil; n = n.parent {
			path = append([]*node{n}, path...)
		}
		row := target
		name := article(at.noun)
		var levels, ones []string
		for _, n := range path {
			row = tb.insertRow(n.t, "", map[string]string{n.col.Name: maybeJust(n.col, row)}, false)
			name += " with " + pluralOf(n.t)
			if n.parent != nil {
				levels = append(levels, fmt.Sprintf("Test.text \"#%s\" page.doc", n.field))
				ones = append(ones, "Just \"1\"")
			}
		}
		tb.insertRow(c.t, "_", map[string]string{c.col.Name: maybeJust(c.col, row)}, false)
		tb.let("let page <- Test.get (%s)", pageLit(mod, []string{"id: " + target}))
		tb.let("let r <- Test.submit Confirm { }")
		tb.let("let kept <- Db.get %s %s", t.Name, target)
		lhs := fmt.Sprintf("count: Test.text \"#%s\" page.doc, ", c.field)
		rhs := "count: Just \"1\", "
		if len(levels) > 0 { // the rows deleted with it on the way down are counted as well
			lhs += "levels: [" + strings.Join(levels, ", ") + "], "
			rhs += "levels: [" + strings.Join(ones, ", ") + "], "
		}
		tests = append(tests, TestData{Name: quote(name + " that " + pluralOf(c.t) + " refer to is not deleted"), Lines: tb.lines,
			Result: fmt.Sprintf("{ %sbuttons: Test.count \"#delete button\" page.doc, answer: r.answer, kept: Maybe.map (\\row -> row.%s) kept } == { %sbuttons: 0, answer: FormRejected [{ field: \"\", message: %s }], kept: Just %s }",
				lhs, t.IDField, rhs, quote(rejectText(at)), target)})
	}
	// a row with children is refused, and so is a childless one a @restrict reference keeps: a
	// chain of three rows, the third held by a row of the first table that restricts it
	for _, n := range nodes {
		if !n.stop {
			continue
		}
		tb = g.tb(mod, imports, true)
		top := tb.insertRow(t, "", nil, true)
		mid := tb.insertRow(t, "", map[string]string{n.col.Name: maybeJust(n.col, top)}, false)
		leaf := tb.insertRow(t, "", map[string]string{n.col.Name: maybeJust(n.col, mid)}, false)
		var hold *node
		for _, r := range nodes {
			if r.col.OnDelete == "restrict" && hold == nil {
				hold = r
			}
		}
		if hold != nil {
			tb.insertRow(hold.t, "_", map[string]string{hold.col.Name: maybeJust(hold.col, leaf)}, false)
		}
		tb.let("let page <- Test.get (%s)", pageLit(mod, []string{"id: " + top}))
		tb.let("let r <- Test.submit Confirm { }")
		lhs := "says: Test.texts \"#delete p\" page.doc, buttons: Test.count \"#delete button\" page.doc, answer: r.answer"
		rhs := fmt.Sprintf("says: [%s], buttons: 0, answer: FormRejected [{ field: \"\", message: %s }]", quote(childrenText), quote(childrenText))
		name := "deleting " + article(at.noun) + " with children is refused"
		if hold != nil {
			tb.let("let _ <- Test.get (%s)", pageLit(mod, []string{"id: " + leaf}))
			tb.let("let leaf <- Test.submit Confirm { }")
			lhs += ", leaf: leaf.answer"
			rhs += fmt.Sprintf(", leaf: FormRejected [{ field: \"\", message: %s }]", quote(rejectText(at)))
			name += ", and a childless one " + Plural(strings.ToLower(Humanize(hold.t.Name))) + " refer to"
		}
		tb.let("let rows <- Db.from %s |> Db.count", t.Name)
		tests = append(tests, TestData{Name: quote(name), Lines: tb.lines,
			Result: fmt.Sprintf("{ %s, rows: rows } == { %s, rows: %d }", lhs, rhs, tb.counts[t.Module])})
		break
	}
	if own { // the signed-in Admin's own row: the page says why there is no button, Remove refuses
		tb = g.tb(mod, imports, true)
		me := tb.user()
		tb.let("let page <- Test.get (%s)", pageLit(mod, []string{"id: " + me}))
		tb.let("let r <- Test.submit Confirm { }")
		tb.let("let kept <- Db.get %s %s", t.Name, me)
		tests = append(tests, TestData{Name: quote("an Admin cannot delete their own account"), Lines: tb.lines,
			Result: fmt.Sprintf("{ says: Test.texts \"#delete p\" page.doc, buttons: Test.count \"#delete button\" page.doc, answer: r.answer, kept: Maybe.map (\\row -> row.%s) kept } == { says: [%s], buttons: 0, answer: FormRejected [{ field: \"\", message: %s }], kept: Just %s }",
				t.IDField, quote(ownText), quote(ownText), me)})
	}
	tests = append(tests, g.missingTest(at, mod, imports))
	tests = append(tests, g.gateTests(mod, imports, func(tb *testBuilder) string {
		return pageLit(mod, []string{"id: " + tb.insertRow(t, "", nil, false)})
	})...)
	b.WriteString(testsText(tests))
	g.file(mod, imports, b.String())
}

// ownText and roleText are the user table's refusals of the signed-in user's own row: its delete,
// and a change of its role (Review Focus 4 of M13: an Admin cannot lock themselves out).
const (
	ownText  = "You cannot delete your own account."
	roleText = "You cannot change your own role."
)

// isUserTable reports whether at is the table of the session's user: its admin guards the
// signed-in user's own row.
func (g *adminGen) isUserTable(at *adminTable) bool {
	s := g.p.Session
	return s != nil && s.User != nil && s.UserTable != "" && g.p.TableNamed(s.UserTable) == at.t
}

// isMe is the condition that the row whose id is expr is the signed-in user's (`requires user`
// makes session.user the user's id, not a Maybe).
func (g *adminGen) isMe(expr string) string { return "session.user == " + expr }

// childrenText is the Delete page's refusal of a row with children (node.stop).
const childrenText = "Delete its children first."

// indentAll is lines, each with prefix.
func indentAll(lines []string, prefix string) []string {
	out := make([]string, len(lines))
	for i, l := range lines {
		out[i] = prefix + l
	}
	return out
}

// article is noun after "a" or "an" ("an" before a, e, i, o; "a user", as most words in u
// sound).
func article(noun string) string {
	if noun != "" && strings.ContainsRune("aeioAEIO", rune(noun[0])) {
		return "an " + noun
	}
	return "a " + noun
}

// rejectText is the Delete page's refusal.
func rejectText(at *adminTable) string {
	return "This " + at.noun + " cannot be deleted while other rows refer to it."
}

// maybeJust is v as a value of column c: `Just v` for a Maybe column.
func maybeJust(c *Column, v string) string {
	if c.Maybe {
		return "Just " + v
	}
	return v
}

// referrers inserts one row of every table that refers to at's table, referring to row, and
// down the closure one row of every table that refers to those.
func (g *adminGen) referrers(tb *testBuilder, at *adminTable, row string) {
	var insert func(n *node, row string)
	insert = func(n *node, row string) {
		if len(n.kids) == 0 {
			tb.insertRow(n.t, "_", map[string]string{n.col.Name: maybeJust(n.col, row)}, false)
			return
		}
		mid := tb.insertRow(n.t, "", map[string]string{n.col.Name: maybeJust(n.col, row)}, false)
		for _, k := range n.kids {
			insert(k, mid)
		}
	}
	for _, n := range g.closure(at) {
		insert(n, row)
	}
}

// pathLit is a page's path as its `page` line writes it: in quotes, its `{id}` a parameter
// (quote would escape the brace, making it literal text).
func pathLit(path string) string { return `"` + path + `"` }

// wrapAll is lines, each wrapped (wrapLine).
func wrapAll(lines []string) []string {
	var out []string
	for _, l := range lines {
		out = append(out, wrapLine(l)...)
	}
	return out
}

// maxLine is the width the admin's code and tests are wrapped to: chapter 15 quotes them.
const maxLine = 100

// wrap is text with each line longer than maxLine broken (wrapLine).
func wrap(text string) string {
	var out []string
	for _, l := range strings.Split(text, "\n") {
		out = append(out, wrapLine(l)...)
	}
	return strings.Join(out, "\n")
}

// wrapLine breaks a line longer than maxLine at its outermost record and list literals: each
// one's elements on lines of their own, indented one step, with a trailing comma; then the
// lines that are still too long, the same way. A comment, markup, and a line with no literal
// of two or more elements stay as they are.
func wrapLine(line string) []string {
	body := strings.TrimLeft(line, " ")
	ind := line[:len(line)-len(body)]
	if len(line) <= maxLine || strings.HasPrefix(body, "--") || strings.HasPrefix(body, "<") {
		return []string{line}
	}
	type group struct {
		open, close int
		items       []string
	}
	var groups []group
	depth, outer, start := 0, -1, 0 // depth of brackets of any kind; outer: the open {/[ group's depth
	var items []string
	for i := 0; i < len(body); i++ {
		switch ch := body[i]; ch {
		case '"':
			for i++; i < len(body) && body[i] != '"'; i++ {
				if body[i] == '\\' {
					i++
				}
			}
		case '{', '[', '(':
			if outer < 0 && ch != '(' {
				outer, start, items = depth, i+1, nil
				groups = append(groups, group{open: i})
			}
			depth++
		case '}', ']', ')':
			depth--
			if outer >= 0 && depth == outer {
				items = append(items, strings.TrimSpace(body[start:i]))
				groups[len(groups)-1].close, groups[len(groups)-1].items = i, items
				outer = -1
			}
		case ',':
			if outer >= 0 && depth == outer+1 {
				items = append(items, strings.TrimSpace(body[start:i]))
				start = i + 1
			}
		}
	}
	var out []string
	from := 0
	for _, g := range groups {
		if g.close == 0 || len(g.items) < 2 {
			continue
		}
		open := body[from : g.open+1]
		if head, rest, ok := strings.Cut(g.items[0], " | "); ok && IsLowerIdent(head) { // { row | f: v, … }: `row |` opens it
			open += " " + head + " |"
			g.items[0] = rest
		}
		out = append(out, ind+open)
		for _, it := range g.items {
			if it != "" {
				out = append(out, wrapLine(ind+"  "+it+",")...)
			}
		}
		from = g.close
	}
	if from == 0 {
		return []string{line}
	}
	return append(out, ind+body[from:])
}
