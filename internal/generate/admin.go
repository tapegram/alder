package generate

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

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
	Module  string // the admin's module prefix: "Admin"
	Path    string // its path: "/admin"
	Version string // the alder version written in each file's header
}

// AdminNeeds is the refusal of `alder new admin` (and `regen`) in an app whose session has no
// role with an `Admin` constructor.
const AdminNeeds = "`alder new admin` needs a session with a `role` whose type has an `Admin` constructor: add `role: User.Role = Reader` to `session` in `App` and `Admin` to `User.Role`, and set it at login (chapter 6)"

// adminCheck refuses an app without the admin's role, and options that are not a module name
// and a path; it returns the options with their defaults.
func adminCheck(p *Program, o AdminOptions) (AdminOptions, error) {
	s := p.Session
	if s == nil || s.User == nil || s.Role == nil || !slices.Contains(s.Role.Ctors, "Admin") {
		return o, Refusef("%s", AdminNeeds)
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
	o.Path = strings.TrimSuffix(o.Path, "/")
	if !strings.HasPrefix(o.Path, "/") || strings.ContainsAny(o.Path, "{}: ?#") {
		return o, Refusef("`--path %s` is not a path without parameters, such as `/admin`", o.Path)
	}
	if len(p.Tables) == 0 {
		return o, Refusef("this app has no tables: the admin has nothing to show (`alder new table` makes one)")
	}
	return o, nil
}

// adminDir is the directory of the admin's modules: "app/Admin".
func adminDir(o AdminOptions) string { return "app/" + strings.ReplaceAll(o.Module, ".", "/") }

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
// file that does not exist is created when all is set, or when no file of its table's pages
// exists yet (a table added since), and is otherwise left out (the author deleted it): skipped
// names those. Nothing to change is no edit at all. A file whose markers are broken is refused.
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
	exists := func(path string) bool {
		_, err := os.Stat(filepath.Join(p.Dir, filepath.FromSlash(path)))
		return err == nil
	}
	for _, path := range order {
		fresh := files[path]
		if !exists(path) {
			tdir := filepath.Dir(filepath.FromSlash(path))
			newTable := false
			if tdir != filepath.FromSlash(dir) { // a table's page: new when none of its pages exists
				entries, _ := os.ReadDir(filepath.Join(p.Dir, tdir))
				newTable = len(entries) == 0
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
	g := &adminGen{p: p, o: o, files: map[string]string{}}
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
	referrer []*referrer // the columns of other tables (or its own) that refer to it
}

// referrer is a column that refers to a table: what deleting a row of that table does to the
// rows holding it.
type referrer struct {
	t     *Table
	col   *Column
	field string    // the Delete page's model field counting them: "comments"
	via   *referrer // for a reference down a chain, the @cascade reference whose rows it refers to
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

// chained are the `@restrict` references to the rows a `@cascade` reference r deletes with
// its row: they refuse that row's delete as well (chapter 8: the blocking row can be further
// down a chain). One step down the chain; r's own table referring to itself is left out.
func (g *adminGen) chained(r *referrer, top *Table) []*referrer {
	if r.col.OnDelete != "cascade" || r.t == top {
		return nil
	}
	var out []*referrer
	for _, c := range g.referrersOf(r.t) {
		if c.col.OnDelete == "restrict" && c.t != r.t {
			out = append(out, &referrer{t: c.t, col: c.col, field: c.field + "Of" + UpperFirst(r.field)})
		}
	}
	return out
}

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
	fmt.Fprintf(&b, "-- generated by `alder new admin` (alder %s); regions between\n", g.o.Version)
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
	b.WriteString("load : Params -> Session -> Query Model\nload params session = {\n")
	b.WriteString(regionAt("  ", "cards", body) + "\n}\n\n")
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
		} else {
			lhs = append(lhs, fmt.Sprintf("rows: Test.count \"#%s li\" page.doc", at.plural))
			rhs = append(rhs, fmt.Sprintf("rows: %d", recentRows))
		}
		name := "the dashboard counts the " + Plural(at.noun) + " and lists the newest five"
		tests = append(tests, TestData{Name: quote(name), Lines: tb.lines, Result: "{ " + strings.Join(lhs, ", ") + " } == { " + strings.Join(rhs, ", ") + " }"})
	}
	// a role is read at login (chapter 6): the cookie, not the user's row, opens the admin
	tb := g.tb(mod, imports, false)
	tb.login("Admin")
	tb.let("let r <- Test.open (%s {})", mod)
	tests = append(tests, TestData{Name: quote("an Admin's role is the one the cookie got at login"), Lines: tb.lines, Result: "r.answer == Shown",
		Comment: []string{
			"`requires user, role Admin` reads the role in the session cookie, the one it got at login",
			"(chapter 6, \"Roles\"), not the user's row: an Admin demoted since keeps the admin until they",
			"log out. A page that must turn them away at once re-reads the row in its `load`.",
		}})
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
		tb.login(role)
		page := pageOf(tb)
		tb.let("let r <- Test.open (%s)", page)
		tests = append(tests, TestData{Name: quote(article(role) + " gets no page"), Lines: tb.lines, Result: "r.answer == PageMissing"})
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
	return "\n" + RegionBegin + "tests\n" + strings.Join(parts, "\n\n") + "\n" + RegionEnd + "\n"
}

// ---- a table's pages

// adminCell is the markup showing column c of row (an expression): a reference as a link to
// the referenced row's admin page, the others as the list pages show them.
func (g *adminGen) adminCell(c *Column, row string, imports *importSet) string {
	if c.Ref != "" {
		if rt := g.p.TableNamed(c.Ref); rt != nil {
			show := g.o.Module + "." + rt.Module + ".Show"
			if c.Maybe {
				return "{match " + row + "." + c.Name + " { Just v -> <a href={" + show + " { id: v }}>{v}</a>, Nothing -> Html.none }}"
			}
			return "<a href={" + show + " { id: " + row + "." + c.Name + " }}>{" + row + "." + c.Name + "}</a>"
		}
	}
	return cell(c, row+"."+c.Name, imports)
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
		if c.Kind == KindText && !c.Maybe && !c.ID && c.Ref == "" {
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
	// sort: every column but the id
	var cols []*Column
	for _, c := range t.Columns {
		if !c.ID {
			cols = append(cols, c)
		}
	}
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
		headers = append(headers, fmt.Sprintf("<th><a href={%s { q: model.q, sort: %s, dir: if model.sort == %s && model.dir == \"asc\" then \"desc\" else \"asc\" }}>%s</a></th>",
			mod, quote(c.Name), quote(c.Name), Humanize(c.Name)))
		cellText := g.adminCell(c, "row", imports)
		if c == at.label {
			cellText = "<a href={" + at.page("Show") + " { id: row." + t.IDField + " }}>{" + textOf(c, "row."+c.Name) + "}</a>"
		}
		cells = append(cells, "<td>"+cellText+"</td>")
	}

	var b strings.Builder
	fmt.Fprintf(&b, "page %s { q: Text = \"\", sort: Text = %s, dir: Text = \"desc\", page: Int = 1 }%s\n\n", quote(at.path), quote(order), adminRequires)
	fmt.Fprintf(&b, "type Model = { rows: List %s, q: Text, sort: Text, dir: Text, page: Int, pages: Int, count: Int }\n\n", t.Name)
	b.WriteString("perPage : Int\nperPage = 25\n\n")
	b.WriteString("-- lastPage bounds `page`, so `(page - 1) * perPage` cannot overflow.\nlastPage : Int\nlastPage = 1000000\n\n")
	b.WriteString(regionAt("", "search", search) + "\n\n")
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

-- load reads one page of the rows that match the search, in the order asked for, and how many
-- match.
load : Params -> Session -> Query Model
load params session = {
  let page = Int.clamp { min: 1, max: lastPage } params.page
  let rows <-
    Db.from %s
      |> Db.where (search params.q)
      |> Db.orderBy (order params.sort params.dir)
      |> Db.limit perPage
      |> Db.offset ((page - 1) * perPage)
      |> Db.all
  let count <- Db.from %s |> Db.where (search params.q) |> Db.count
  Query.succeed { rows: rows, q: params.q, sort: params.sort, dir: params.dir, page: page, pages: Int.max 1 ((count + perPage - 1) / perPage), count: count }
}

`, t.Name, t.Name, order, t.Name, t.Name)
	b.WriteString("view : Model -> Session -> Html Msg\nview model session =\n")
	fmt.Fprintf(&b, "  %s\n    <h1>%s</h1>\n\n", open, at.title)
	fmt.Fprintf(&b, `    <al-stack direction="row" gap="sm">
      <form method="get" action=%s>
        <input type="search" name="q" value={model.q} aria-label="Search" />
        <input type="hidden" name="sort" value={model.sort} />
        <input type="hidden" name="dir" value={model.dir} />
      </form>
      <al-button href={%s {}}>New %s</al-button>
    </al-stack>

    <p id="count">{Text.fromInt model.count} rows</p>

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
      {if model.page > 1 then <a slot="prev" href={%s { q: model.q, sort: model.sort, dir: model.dir, page: model.page - 1 }} rel="prev">Previous</a> else <span slot="prev" aria-disabled="true">Previous</span>}
      <span>Page {Text.fromInt model.page} of {Text.fromInt model.pages}</span>
      {if model.page < model.pages then <a slot="next" href={%s { q: model.q, sort: model.sort, dir: model.dir, page: model.page + 1 }} rel="next">Next</a> else <span slot="next" aria-disabled="true">Next</span>}
    </al-pagination>
`, quote(at.path), at.page("New"), at.noun, regionAt("            ", "headers", headers), regionAt("              ", "cells", cells), mod, mod)
	fmt.Fprintf(&b, "  %s\n", close)

	// the tests
	var tests []TestData
	ids := func(v string) string { return "List.map (\\row -> row." + t.IDField + ") " + v + ".model.rows" }

	tb := g.tb(mod, imports, true)
	tb.insert(t, true)
	tb.insert(t, true)
	tb.let("let page <- Test.get (%s {})", mod)
	if t.Created != "" {
		tests = append(tests, TestData{Name: quote("the list shows every row, newest first"), Lines: tb.lines,
			Result: ids("page") + " == [" + strings.Join(tb.visible(t, false), ", ") + "]"})
	} else {
		tests = append(tests, TestData{Name: quote("the list shows every row"), Lines: tb.lines,
			Result: fmt.Sprintf("List.length page.model.rows == %d", len(tb.visible(t, false)))})
	}

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
		tests = append(tests, TestData{Name: quote("sorting by " + sc.Name + " goes both ways"), Lines: tb.lines,
			Result: fmt.Sprintf("{ up: %s, down: %s } == { up: [%s], down: [%s] }", ids("up"), ids("down"), strings.Join(up, ", "), strings.Join(down, ", "))})
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
			Result: fmt.Sprintf("{ rows: %s, count: page.model.count } == { rows: [%s], count: 1 }", ids("page"), match)})
	}

	// 25 rows a page
	tb = g.tb(mod, imports, true)
	tb.bulkInsert(t, 26, "")
	total := len(tb.visible(t, false)) + tb.bulk[t.Module]
	tb.let("let first <- Test.get (%s {})", mod)
	tb.let("let second <- Test.get (%s)", pageLit(mod, []string{"page: 2"}))
	tests = append(tests, TestData{Name: quote("the list has 25 rows a page"), Lines: tb.lines,
		Result: fmt.Sprintf("{ first: List.length first.model.rows, second: List.length second.model.rows, pages: Test.texts \"al-pagination span\" first.doc } == { first: 25, second: %d, pages: [\"Previous\", \"Page 1 of %d\"] }", total-25, (total+24)/25)})

	tests = append(tests, g.gateTests(mod, imports, func(*testBuilder) string { return mod + " {}" })...)
	b.WriteString(testsText(tests))
	g.file(mod, imports, b.String())
}

// sortColumn is the column the sort test sorts by: the first text (else number or date) column
// that is not an id or a reference, nil for none.
func sortColumn(t *Table) *Column {
	for _, kinds := range [][]Kind{{KindText}, {KindInt, KindFloat, KindDate}} {
		for _, c := range t.Columns {
			if !c.ID && c.Ref == "" && !c.Maybe && slices.Contains(kinds, c.Kind) && !c.Created && !c.Updated {
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
	for _, c := range t.Columns {
		fields = append(fields, "<dt>"+Humanize(c.Name)+"</dt>", "<dd>"+g.adminCell(c, "model.row", imports)+"</dd>")
	}
	var b strings.Builder
	fmt.Fprintf(&b, "page %s { id: %s }%s\n\n", pathLit(at.path+"/{id}"), types.StringIn(t.IDType, mod), adminRequires)
	fmt.Fprintf(&b, "type Model = { row: %s }\n\n", t.Name)
	fmt.Fprintf(&b, `load : Params -> Session -> Query Model
load params session = {
  let found <- Db.get %s params.id
  match found {
    Just row -> Query.succeed { row: row },
    Nothing -> Query.notFound,
  }
}

`, t.Name)
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
	row := tb.insertRow(t, "", nil, false)
	tb.let("let r <- Test.open (%s)", pageLit(mod, []string{"id: " + row}))
	result := fmt.Sprintf("{ answer: r.answer, fields: Test.count \"dd\" r.doc } == { answer: Shown, fields: %d }", len(t.Columns))
	if !at.label.ID {
		result = fmt.Sprintf("{ answer: r.answer, %s: List.member %s (Test.texts \"dd\" r.doc) } == { answer: Shown, %s: True }",
			at.label.Name, quote(rawText(at.label, tb.counts[t.Module])), at.label.Name)
	}
	tests = append(tests, TestData{Name: quote("the page shows the row"), Lines: tb.lines, Result: result})
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
	insert = append(insert, fmt.Sprintf("let id <- Db.insert %s { %s }", t.Name, strings.Join(inits, ", ")))

	var b strings.Builder
	fmt.Fprintf(&b, "page %s { }%s\n\n", quote(at.path+"/new"), adminRequires)
	b.WriteString("type Model = {\n" + regionAt("  ", "model", model) + "\n}\n\n")
	b.WriteString("load : Params -> Session -> Query Model\nload params session = {\n" + regionAt("  ", "load", loads) + "\n}\n\n")
	b.WriteString(regionAt("", "form", form) + "\n\n")
	fmt.Fprintf(&b, "msg Create form %s\n\n", at.formName())
	b.WriteString("update : Msg -> Model -> Session -> Task Nav\nupdate msg model session = match msg {\n  Create p -> {\n")
	b.WriteString(regionAt("    ", "insert", insert) + "\n")
	fmt.Fprintf(&b, "    Task.succeed (Redirect (%s { id: id }))\n  },\n}\n\n", at.page("Show"))
	b.WriteString("view : Model -> Session -> Html Msg\nview model session =\n")
	fmt.Fprintf(&b, "  %s\n    <h1>%s</h1>\n\n    <al-card>\n      <form submit={Create}>\n%s\n", open, UpperFirst(title), regionAt("        ", "inputs", inputs))
	b.WriteString("        <al-alert variant=\"destructive\"><Errors form /></al-alert>\n        <al-button type=\"submit\">Create</al-button>\n      </form>\n    </al-card>\n")
	fmt.Fprintf(&b, "  %s\n", close)

	// the tests: a valid form adds the row, with the values submitted, and goes to its page
	var tests []TestData
	tb := g.tb(mod, imports, true)
	optLines := g.optionRows(tb, at)
	k := tb.counts[t.Module]
	var values, got, want []string
	for _, c := range at.editable {
		v := g.submitValue(tb, at, c, k+1)
		values = append(values, c.Name+": "+v)
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
	if len(at.editable) == 0 {
		lhs, rhs = lhs[:1], rhs[:1]
	}
	tests = append(tests, TestData{Name: quote("creating " + article(at.noun) + " adds it, with the values submitted"), Lines: tb.lines,
		Result: "{ " + strings.Join(lhs, ", ") + " } == { " + strings.Join(rhs, ", ") + " }"})
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
	b.WriteString("load : Params -> Session -> Query Model\nload params session = {\n" + regionAt("  ", "load", loads) + "\n}\n\n")
	b.WriteString(regionAt("", "form", form) + "\n\n")
	fmt.Fprintf(&b, "msg Save form %s\n\n", at.formName())
	b.WriteString("update : Msg -> Model -> Session -> Task Nav\nupdate msg model session = match msg {\n  Save p -> {\n")
	b.WriteString(regionAt("    ", "update", []string{upd}) + "\n")
	fmt.Fprintf(&b, "    Task.succeed (Redirect (%s { id: model.row.%s }))\n  },\n}\n\n", at.page("Show"), t.IDField)
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
	tests = append(tests, g.notOffered(at, mod, imports, true)...)
	tests = append(tests, g.missingTest(at, mod, imports))
	tests = append(tests, g.gateTests(mod, imports, func(tb *testBuilder) string {
		return pageLit(mod, []string{"id: " + tb.insertRow(t, "", nil, false)})
	})...)
	b.WriteString(testsText(tests))
	g.file(mod, imports, b.String())
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

	var model, loads, counts, blockers []string
	fields := []string{"row: row"}
	var chains []*referrer // the @restrict references down a @cascade, each counted
	for _, r := range at.referrer {
		imports.add(r.t.Module)
		model = append(model, r.field+": Int,")
		sel := fmt.Sprintf("Db.from %s |> Db.where (%s)", r.t.Name, eqID(r.t, r.col, "params.id"))
		chained := g.chained(r, t)
		if len(chained) == 0 {
			loads = append(loads, fmt.Sprintf("let %s <- %s |> Db.count", r.field, sel))
			fields = append(fields, r.field+": "+r.field)
		} else { // the rows themselves: the rows that refer to them are counted too
			loads = append(loads, fmt.Sprintf("let %sRows <- %s |> Db.all", r.field, sel))
			fields = append(fields, r.field+": List.length "+r.field+"Rows")
		}
		what := map[string]string{
			"cascade":  "%s deleted with it",
			"setNull":  "%s that lose their " + strings.ToLower(Humanize(r.col.Name)),
			"restrict": "%s that keep it from being deleted",
		}[r.col.OnDelete]
		counts = append(counts, "<dt>"+fmt.Sprintf(what, UpperFirst(Plural(strings.ToLower(Humanize(r.t.Name)))))+"</dt>", fmt.Sprintf("<dd id=%s>{Text.fromInt model.%s}</dd>", quote(r.field), r.field))
		if r.col.OnDelete == "restrict" {
			blockers = append(blockers, "model."+r.field)
		}
		for _, c := range chained {
			imports.add(c.t.Module)
			ids := "List.map (\\row -> row." + r.t.IDField + ") " + r.field + "Rows"
			if c.col.Maybe {
				ids = "List.map (\\row -> Just row." + r.t.IDField + ") " + r.field + "Rows"
			}
			model = append(model, c.field+": Int,")
			loads = append(loads, fmt.Sprintf("let %s <- Db.from %s |> Db.where (%s.%s.in (%s)) |> Db.count", c.field, c.t.Name, c.t.Name, c.col.Name, ids))
			fields = append(fields, c.field+": "+c.field)
			counts = append(counts, "<dt>"+UpperFirst(Plural(strings.ToLower(Humanize(c.t.Name))))+" of those "+Plural(strings.ToLower(Humanize(r.t.Name)))+" that keep it from being deleted</dt>",
				fmt.Sprintf("<dd id=%s>{Text.fromInt model.%s}</dd>", quote(c.field), c.field))
			blockers = append(blockers, "model."+c.field)
			chains = append(chains, &referrer{t: c.t, col: c.col, field: c.field})
			chains[len(chains)-1].via = r
		}
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
	labelRow := "<p>{" + textOf(at.label, "model.row."+at.label.Name) + "}</p>"

	var b strings.Builder
	fmt.Fprintf(&b, "page %s { id: %s }%s\n\n", pathLit(at.path+"/{id}/delete"), types.StringIn(t.IDType, mod), adminRequires)
	b.WriteString("-- The row, and how many rows of each table refer to it.\n")
	b.WriteString("type Model = {\n  row: " + t.Name + ",\n" + regionAt("  ", "model", model) + "\n}\n\n")
	b.WriteString("load : Params -> Session -> Query Model\nload params session = {\n" + regionAt("  ", "load", loads) + "\n}\n\n")
	b.WriteString(regionAt("", "blockers", block) + "\n\n")
	b.WriteString("-- Delete takes no input: the form is the button, and the place a refusal is shown.\nform Confirm = { }\n\nmsg Remove form Confirm\n\n")
	remove := []string{
		"let _ <- Db.delete " + t.Name + " model.row." + t.IDField,
		"Task.succeed (Redirect (" + at.page("Index") + " {}))",
	}
	if len(blockers) > 0 { // refused while rows refer to it: the database would refuse with an error
		remove = append([]string{
			"if blockers model > 0 then",
			"  Task.succeed (Reject " + quote(rejectText(at)) + ")",
			"else {",
		}, append(indentAll(remove, "  "), "}")...)
	}
	b.WriteString(`-- Remove deletes the row, unless rows refer to it that keep it (load counted them, as it does
-- before every message): the database would refuse the delete with an error, and the page says
-- why instead.
update : Msg -> Model -> Session -> Task Nav
update msg model session = match msg {
  Remove p -> {
` + regionAt("    ", "remove", remove) + `
  },
}

`)
	b.WriteString("view : Model -> Session -> Html Msg\nview model session =\n")
	fmt.Fprintf(&b, "  %s\n    <h1>Delete this %s?</h1>\n\n    <al-card>\n%s\n      <dl>\n%s\n      </dl>\n", open, at.noun, regionAt("      ", "row", []string{labelRow}), regionAt("        ", "counts", counts))
	fmt.Fprintf(&b, `      <form id="delete" submit={Remove}>
        <al-alert variant="destructive"><Errors form /></al-alert>
        {if blockers model > 0 then <p>It cannot be deleted while other rows refer to it.</p> else <al-button variant="destructive" type="submit">Delete</al-button>}
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
	// a @restrict reference down a @cascade refuses too
	for _, c := range chains {
		tb = g.tb(mod, imports, true)
		target := tb.insertRow(t, "", nil, true)
		mid := tb.insertRow(c.via.t, "", map[string]string{c.via.col.Name: maybeJust(c.via.col, target)}, false)
		tb.insertRow(c.t, "_", map[string]string{c.col.Name: maybeJust(c.col, mid)}, false)
		tb.let("let page <- Test.get (%s)", pageLit(mod, []string{"id: " + target}))
		tb.let("let r <- Test.submit Confirm { }")
		tb.let("let kept <- Db.get %s %s", t.Name, target)
		tests = append(tests, TestData{Name: quote(article(at.noun) + " with " + Plural(strings.ToLower(Humanize(c.via.t.Name))) + " that " + Plural(strings.ToLower(Humanize(c.t.Name))) + " refer to is not deleted"), Lines: tb.lines,
			Result: fmt.Sprintf("{ count: Test.text \"#%s\" page.doc, buttons: Test.count \"#delete button\" page.doc, answer: r.answer, kept: Maybe.map (\\row -> row.%s) kept } == { count: Just \"1\", buttons: 0, answer: FormRejected [{ field: \"\", message: %s }], kept: Just %s }",
				c.field, t.IDField, quote(rejectText(at)), target)})
	}
	tests = append(tests, g.missingTest(at, mod, imports))
	tests = append(tests, g.gateTests(mod, imports, func(tb *testBuilder) string {
		return pageLit(mod, []string{"id: " + tb.insertRow(t, "", nil, false)})
	})...)
	b.WriteString(testsText(tests))
	g.file(mod, imports, b.String())
}

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
// one of every table down a chain that refers to those.
func (g *adminGen) referrers(tb *testBuilder, at *adminTable, row string) {
	for _, r := range at.referrer {
		chained := g.chained(r, at.t)
		if len(chained) == 0 {
			tb.insertRow(r.t, "_", map[string]string{r.col.Name: maybeJust(r.col, row)}, false)
			continue
		}
		mid := tb.insertRow(r.t, "", map[string]string{r.col.Name: maybeJust(r.col, row)}, false)
		for _, c := range chained {
			tb.insertRow(c.t, "_", map[string]string{c.col.Name: maybeJust(c.col, mid)}, false)
		}
	}
}

// pathLit is a page's path as its `page` line writes it: in quotes, its `{id}` a parameter
// (quote would escape the brace, making it literal text).
func pathLit(path string) string { return `"` + path + `"` }
