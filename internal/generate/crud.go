package generate

import (
	"fmt"
	"regexp"
	"slices"
	"strings"

	"alder/internal/types"
)

type crudGen struct {
	p      *Program
	o      AdminOptions
	a      access  // whose rows the pages reach and how their routes are guarded
	role   *Column // the user table's role column, which every route's adminOnly reads again; nil for none
	tables []*crudTable
	silo   []*adminSilo        // the app's silos (admin_silo.go)
	names  map[string][]string // each module's routes, by name: its code writes them bare
	files  map[string]string
	order  []string
}

// crudTable is a table as its generated pages show it: the names its modules, routes and
// models take, and the columns its list, forms and delete reach.
type crudTable struct {
	t         *Table
	mod       string // the module prefix of its pages: "Admin.Posts"
	path      string // "/admin/post"
	plural    string // "posts": its rows in a model, an element's id
	title     string // "Posts"
	noun      string // "post"
	label     *Column
	editable  []*Column   // the columns its forms edit
	refs      []*Column   // the editable columns that refer to a table: a select of its rows
	nowCols   []*Column   // Time columns an insert must name, which no form edits: the time of the insert
	secrets   []*Column   // secret columns an insert must name, which the admin never shows or edits
	referrer  []*referrer // the columns of other tables (or its own) that refer to it
	bulkDel   bool        // the bulk page offers a delete
	bulkHolds []*node     // the @restrict references that keep a row from a bulk delete
	bulkSet   []*Column   // the columns the bulk page sets
	silo      *adminSilo  // the silo it lives in, nil for a table of the shared store
	parent    *Column     // unused: the --under seam
}

// access says whose rows a set of pages reach and how its routes are guarded.
type access struct {
	marker       string // "@admin" | "@resource": region markers
	header       string // the file's two header lines, without the trailing newline
	gate         string // first statement of every load/update: "let _ <- Admin.Layout.adminOnly session.user" | ""
	layout       string // the view's root component: "Admin.Layout.Page" | "Layout.Page"
	layoutImport string // "Admin.Layout" | "Layout"

	readRequires  string // Index and Show: " requires user, role Admin" | " requires user" | " requires user, org" | ""
	writeRequires string // New/Create, Edit/Save, Delete/Destroy
	sel           string // Index's rows and count: "Db.from" | "Db.mine" | "Db.scoped"
	get           string // Show's load: "Db.get" (Maybe) | "Db.getMine" | "Db.getScoped"
	writeGet      string // the load on Edit and Delete and before Save/Destroy: as get, for writes

	insert, update, remove string // "Db.insert"|"Db.insertMine"|"Db.insertScoped", likewise update/delete
	ownerOnly              string // "" | "session.user == row.<owner>" | "session.user == Just row.<owner>"
	readNote               string // "" | the `-- unscoped:` comment above Index's and Show's reads of every user's rows of an owned table
	perPage                int    // 25 admin, 20 resource

	role string   // the role the tests sign in with ("Admin"; "" for none): the gate tests try the session's others
	keys []string // the session fields the routes require ("user", "org"): a reference's select is scoped by them
}

// scoped is whether the pages write with the scoped calls (insertMine, insertScoped): their
// unscoped reads, which may reach rows out of the user's scope, each carry a comment saying why.
func (a access) scoped() bool { return a.insert != "Db.insert" }

// ownerTest is ownerOnly of the row expression row ("model.row"): the test of the links to the
// writes, which render for the row's owner only.
func (a access) ownerTest(row string) string { return strings.Replace(a.ownerOnly, "row.", row+".", 1) }

// visitorReads is whether a visitor reads the pages (`--read all` on an owned table): Index and
// Show require nothing, and show the links to the writes to the owner only.
func (a access) visitorReads() bool { return a.ownerOnly != "" && a.readRequires == "" }

// hidingNote is the comment above an ownerOnly test in a view.
const hidingNote = "-- hiding is not guarding: Save and Destroy require the owner (updateMine, deleteMine)"

// maybeGet is whether get loads a Maybe (Db.get), which the code matches on, rather than the
// row itself (getMine, getScoped: a row out of reach stops the route with a 404).
func (a access) maybeGet(get string) bool { return get == "Db.get" }

// found is the statements loading row `row` of t by idExpr with get: for Db.get,
// `let found <- Db.get T id`, then between, then `match found { Just row -> { body }, Nothing ->
// notFound, }`; for getMine/getScoped, `let row <- <get> T id`, then between, then body.
func (a access) found(get string, t *Table, idExpr string, between, body []string, notFound string) []string {
	if !a.maybeGet(get) {
		out := append([]string{"let row <- " + get + " " + t.Name + " " + idExpr}, between...)
		return append(out, body...)
	}
	out := append([]string{"let found <- " + get + " " + t.Name + " " + idExpr}, between...)
	out = append(out, "match found {", "  Just row -> {")
	for _, l := range body {
		out = append(out, indent(l, "    "))
	}
	return append(out, "  },", "  Nothing -> "+notFound+",", "}")
}

// load is GET route r's `load`, with body its statements (indented two spaces), after the
// access's gate (the admin's adminOnly: a user it does not let in gets a 404; gated).
func (g *crudGen) load(r, body string) string {
	return r + ".load : " + r + ".Params -> Session -> Query Model\n" + r + ".load params session = {\n" + g.gated(body) + "\n}\n\n"
}

// update is write route r's `update`, with body its statements (indented two spaces), after the
// access's gate, as load.
func (g *crudGen) update(r, body string) string {
	return r + ".update : " + r + ".Params -> Session -> Task Response\n" + r + ".update params session = {\n" + g.gated(body) + "\n}\n\n"
}

// gated is body (statements indented two spaces) after the access's gate, when it has one: the
// admin's `let _ <- Admin.Layout.adminOnly session.user` (isAdmin, the layout's region `access`:
// when the user table holds the role, a user whose row says Admin now; the session's role is the
// one read at login, which lasts until logout or the cookie's 30 days, chapter 6, "Rules"). A
// resource has none: its routes' `requires` and the scoped calls are its guard.
func (g *crudGen) gated(body string) string {
	if g.a.gate == "" {
		return body
	}
	return "  " + g.a.gate + "\n" + body
}

// referrer is a column that refers to a table: what deleting a row of that table does to the
// rows holding it.
type referrer struct {
	t     *Table
	col   *Column
	field string // the Delete page's model field counting them: "comments"
}

// filterOption is an option of column c's filter: its value in the address, its label, and the
// value of the column it selects.
type filterOption struct{ value, label, col string }

// table is t as its pages show it, their modules under mod ("Admin.Posts", "Posts") and their
// paths under path ("/admin/post", "/posts").
func (g *crudGen) table(t *Table, mod, path string) *crudTable {
	at := &crudTable{t: t, mod: mod, path: path,
		plural: Plural(LowerFirst(t.Name)), noun: strings.ToLower(Humanize(t.Name)), label: t.Label()}
	at.title = UpperFirst(Plural(at.noun))
	for _, c := range t.Columns {
		switch {
		case c.ID || c.Created || c.Updated:
		case t.Silo != "" && c.Name == t.PartCol: // the page's parameter, never a form's
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
func (g *crudGen) referrersOf(t *Table) []*referrer {
	count := map[string]int{}
	var refs []*referrer
	for _, other := range g.p.Tables {
		if other.Silo != "" && other.Silo != t.Silo { // another database: a delete there is not this one's
			continue
		}
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
		if r.field == "labels" || r.field == "rows" || r.field == "owns" { // a field of the page's own
			r.field += "Of" + t.Name
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
func (g *crudGen) selfStop(t *Table, col *Column) bool {
	return col.OnDelete == "cascade" && g.p.TableNamed(col.Ref) == t && g.restrictReach(t)
}

// restrictReach reports whether deleting a row of t can meet a @restrict reference: one to t, or
// to a table whose rows a @cascade from t deletes, at any depth.
func (g *crudGen) restrictReach(t *Table) bool {
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
func (g *crudGen) closure(at *crudTable) []*node {
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
func (g *crudGen) expand(n *node, path map[*Column]bool) {
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

// labelOf is the column naming a row of rt on these pages: rt.Label(), but a resource's pages
// (no role) never name a user by an email (spec anti-goal: no list of every user's email on a
// user-facing page): the user table's userLabel, else its id, and the columns referring to users
// are then left off the pages (hidden).
func (g *crudGen) labelOf(rt *Table) *Column {
	if !g.usersPrivate(rt) {
		return rt.Label()
	}
	if l := userLabel(rt); l != nil {
		return l
	}
	return rt.Column(rt.IDField)
}

// usersPrivate is whether rt is the session's user table on pages that are not the admin's.
func (g *crudGen) usersPrivate(rt *Table) bool {
	return g.a.role == "" && rt != nil && g.p.Session != nil && rt.Name == g.p.Session.UserTable
}

// hidden is whether column c is left off these pages: a reference to users, on a resource's
// pages, when no column but an email names a user (userLabel).
func (g *crudGen) hidden(c *Column) bool {
	rt := g.p.TableNamed(c.Ref)
	return c.Ref != "" && g.usersPrivate(rt) && userLabel(rt) == nil
}

// userLabel is the column a resource's pages name a user by: the user table's first Text column
// that is not an id, a reference, a secret, nor an email (`Text.Email`, or a name holding
// "email"); nil when it has none.
func userLabel(ut *Table) *Column {
	for _, c := range ut.Columns {
		if c.Kind != KindText || c.Maybe || c.ID || c.Ref != "" || c.Secret() {
			continue
		}
		t, _ := unMaybe(c.Type)
		if b, ok := t.(*types.Brand); ok && b.Module == "Text" && b.Name == "Email" || strings.Contains(strings.ToLower(c.Name), "email") {
			continue
		}
		return c
	}
	return nil
}

// shownOf are the columns of t these pages show: shown's, but the hidden ones.
func (g *crudGen) shownOf(t *Table, id bool) []*Column {
	return slices.DeleteFunc(shown(t, id), g.hidden)
}

// shown are the columns of t the admin shows: all but its secrets (Column.Secret), and but the
// id unless id is set.
func shown(t *Table, id bool) []*Column {
	var out []*Column
	for _, c := range t.Columns {
		if !c.Secret() && (id || !c.ID) && !(t.Silo != "" && c.Name == t.PartCol) {
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
func (g *crudGen) file(module string, imports *importSet, body string) {
	var b strings.Builder
	b.WriteString(g.a.header + "\n")
	fmt.Fprintf(&b, "module %s\n\n", module)
	b.WriteString(g.regionAt("", "imports", strings.Split(strings.TrimSuffix(imports.lines(), "\n"), "\n")) + "\n\n")
	b.WriteString(g.siloCode(module, ownBare(module, g.names[module], body)))
	path := FileOf(module)
	// formatted here, not only by Apply: regen compares these regions with the file's, formatted
	// a text that does not parse is kept as it is: Apply formats it again, warns and the tests catch it
	text, _ := canonical(g.p, path, b.String())
	g.files[path], _ = Formatted(path, text)
	g.order = append(g.order, path)
}

// ownBare is body with the routes names of module written bare, as its own code must write them
// (a table that refers to itself links its Show page to another row: `Show { id: v }`).
func ownBare(module string, names []string, body string) string {
	if len(names) == 0 {
		return body
	}
	var alts []string
	for _, n := range names {
		alts = append(alts, regexp.QuoteMeta(n))
	}
	re := regexp.MustCompile(`(^|[^A-Za-z0-9_.])` + regexp.QuoteMeta(module) + `\.(` + strings.Join(alts, "|") + `)\b`)
	return re.ReplaceAllString(body, "$1$2")
}

// regionAt is an owned region: its markers at indentation ind, around lines (each indented by
// ind as well; none for an empty region).
func (g *crudGen) regionAt(ind, name string, lines []string) string {
	var b strings.Builder
	b.WriteString(ind + regionBegin(g.a.marker) + name + "\n")
	for _, l := range lines {
		b.WriteString(indent(l, ind) + "\n")
	}
	b.WriteString(ind + regionEnd(g.a.marker))
	return b.String()
}

// viewHead is the signature and head of GET route r's `view`.
func viewHead(r string) string {
	return r + ".view : Model -> Session -> Html\n" + r + ".view model session =\n"
}

// routeOf is the GET route of route module mod as another module writes it: the module and the
// route, which takes its last segment (`Admin.Posts.Show.Show`, Plan call 14).
func routeOf(mod string) string { return mod + "." + mod[strings.LastIndex(mod, ".")+1:] }

// routeLine is a `route` line: name = method "path" { params } [form F], and the requires
// clause req (" requires …").
func routeLine(name, method, path string, params []string, form, req string) string {
	ps := "{ }"
	if len(params) > 0 {
		ps = "{ " + strings.Join(params, ", ") + " }"
	}
	if form != "" {
		ps += " form " + form
	}
	return "route " + name + " = " + method + " " + pathLit(path) + " " + ps + req
}

// wire is the query key the GET route of module page reads parameter name from: its wire name
// when the route, as the app has it now, declares one (`q: Text = "" as "Q"`, #55), else the
// name. A GET form to the route names its inputs by it.
func (g *crudGen) wire(page, name string) string {
	var params []*Param
	if r := g.p.RouteModule(page); r != nil {
		params = r.Params
	}
	for _, par := range params {
		if par.Name == name && par.Wire != "" {
			return par.Wire
		}
	}
	return name
}

// wireSel is a selector's value for an input named by g.wire: the name, or the wire name quoted
// (a wire name may hold `.` or `~`, which a bare selector value may not).
func (g *crudGen) wireSel(page, name string) string {
	if w := g.wire(page, name); w != name {
		return "'" + w + "'"
	}
	return name
}

// keepWires adds to each of page's parameters ("q: Text = \"\"") the wire name its GET route, as
// the app has it now, gives it: a regenerated `route` line keeps an `as "Key"` the author added.
func (g *crudGen) keepWires(page string, params []string) []string {
	out := make([]string, len(params))
	for i, par := range params {
		out[i] = par
		if name, _, ok := strings.Cut(par, ":"); ok {
			if w := g.wire(page, name); w != name {
				out[i] = par + " as " + quote(w)
			}
		}
	}
	return out
}

// page is the route module of table at's page kind ("Index", "Show", …).
func (at *crudTable) page(kind string) string { return at.mod + "." + kind }

// route is the GET route of table at's page kind as another module writes it:
// `Admin.Posts.Show.Show`.
func (at *crudTable) route(kind string) string { return routeOf(at.page(kind)) }

// own is a route of the module being written, with no parameters but a siloed table's key: its
// name, or `name {}` (siloLits gives it the key), in parentheses when paren is set.
func (at *crudTable) own(name string, paren bool) string {
	if at.silo == nil {
		return name
	}
	if paren {
		return "(" + name + " {})"
	}
	return name + " {}"
}

// testsText renders the generated tests, in the region `tests` at the end of the file: regen
// rewrites them as the schema changes, and a test the author adds after the region stays.
func (g *crudGen) testsText(tests []TestData) string {
	var parts []string
	for _, t := range tests {
		parts = append(parts, snippet("test", t))
	}
	return "\n" + regionBegin(g.a.marker) + "tests\n" + wrap(strings.Join(parts, "\n\n")) + "\n" + regionEnd(g.a.marker) + "\n"
}

// refCell is the markup showing column c of row (an expression): a reference by the referenced
// row's label (model.labels, from the page's region `labels`) or, when its table has none, its
// id, as a link to the row's page when these pages show its table (showPage); the others as the
// list pages show them.
func (g *crudGen) refCell(c *Column, row string, imports *importSet) string {
	if c.Ref != "" {
		if rt := g.p.TableNamed(c.Ref); rt != nil {
			show := g.showPage(rt)
			text := func(id string) string {
				if g.labelOf(rt).ID {
					return id
				}
				return labelFunc(c) + " model.labels " + id
			}
			if show == "" { // no page of these shows the row: its label, unlinked
				if c.Maybe {
					return "{match " + row + "." + c.Name + " { Just v -> " + text("v") + ", Nothing -> \"\" }}"
				}
				return "{" + text(row+"."+c.Name) + "}"
			}
			if c.Maybe {
				return "{match " + row + "." + c.Name + " { Just v -> <a href={" + show + " { id: v }}>{" + text("v") + "}</a>, Nothing -> Html.none }}"
			}
			return "<a href={" + show + " { id: " + row + "." + c.Name + " }}>{" + text(row+"."+c.Name) + "}</a>"
		}
	}
	return cell(c, row+"."+c.Name, imports)
}

// showPage is the GET route of the page showing a row of t, as another module writes it: the
// Show page of t among the tables these pages show (every table, for the admin:
// `Admin.Users.Show.Show`), "" for a table they do not show.
func (g *crudGen) showPage(t *Table) string {
	for _, at := range g.tables {
		if at.t == t {
			return at.route("Show")
		}
	}
	return ""
}

// refSelect is how these pages read the rows of rt, a table one of their references refers to:
// `Db.mine Rt` when rt is owned and the routes require `user`, `Db.scoped Rt` when it is
// partitioned by another key they require, else `Db.from Rt`, with (on pages that write with the
// scoped calls) the comment saying why that read is unscoped.
func (g *crudGen) refSelect(rt *Table) (sel, comment string) {
	switch {
	case rt.PartKey == "user" && slices.Contains(g.a.keys, "user"):
		return "Db.mine " + rt.Name, ""
	case rt.PartKey != "" && slices.Contains(g.a.keys, rt.PartKey):
		return "Db.scoped " + rt.Name, ""
	case !g.a.scoped():
		return "Db.from " + rt.Name, ""
	case rt.PartKey != "":
		return "Db.from " + rt.Name, "-- every " + rt.PartKey + "'s rows: these routes do not require `" + rt.PartKey + "`"
	}
	return "Db.from " + rt.Name, "-- every user's rows: " + rt.Name + " has no owner"
}

// paren is expr in parentheses when it is more than a name.
func paren(expr string) string {
	if strings.ContainsAny(expr, " (") {
		return "(" + expr + ")"
	}
	return expr
}

// labelFunc is the function naming the row reference column c refers to: "authorLabel".
func labelFunc(c *Column) string { return c.Name + "Label" }

// labelled are the columns of at's table whose cells show the label of the row they refer to:
// its references to a table whose rows have a label other than their id.
func (g *crudGen) labelled(at *crudTable) []*Column {
	var out []*Column
	for _, c := range at.t.Columns {
		if rt := g.p.TableNamed(c.Ref); c.Ref != "" && rt != nil && !g.labelOf(rt).ID && !c.Secret() && !(at.t.Silo != "" && c.Name == at.t.PartCol) {
			out = append(out, c)
		}
	}
	return out
}

// labels is the region `labels` of at's list and show pages: the type Labels, the rows its
// references name (a field per column, as optionsField), labelsOf reading them for the rows a
// page shows (`.in` their ids), and a function per column naming a row by its label.
func (g *crudGen) labels(at *crudTable, mod string, imports *importSet) []string {
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
		sel, why := g.refSelect(rt)
		if why != "" {
			loads = append(loads, "  "+why)
		}
		loads = append(loads, fmt.Sprintf("  let %s <- %s |> Db.where (%s.%s.in (%s)) |> Db.all", optionsField(c), sel, rt.Name, rt.IDField, ids))
		inits = append(inits, optionsField(c)+": "+optionsField(c))
		noun := strings.ToLower(Humanize(rt.Name))
		funcs = append(funcs, "",
			fmt.Sprintf("-- %s is the label of the %s id names (the id itself if it names none).", labelFunc(c), noun),
			fmt.Sprintf("%s : Labels -> %s -> Text", labelFunc(c), types.StringIn(rt.IDType, mod)),
			fmt.Sprintf("%s labels id = match List.find (\\r -> r.%s == id) labels.%s {", labelFunc(c), rt.IDField, optionsField(c)),
			"  Just r -> "+textOf(g.labelOf(rt), "r."+g.labelOf(rt).Name)+",",
			"  Nothing -> id,",
			"}")
	}
	out := []string{"-- Labels are the rows the references of the rows shown name: their links show their labels."}
	if len(cols) == 0 {
		return append(out, "type Labels = { }", "", "labelsOf : List "+t.Name+" -> Query Labels", "labelsOf rows = Query.succeed {}")
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
func (g *crudGen) selfParents(tb *testBuilder, at *crudTable) (map[string]string, map[*Column][2]string) {
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

func (g *crudGen) labelChecks(tb *testBuilder, at *crudTable, model string, parents map[*Column][2]string) [2][]string {
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
			first, firstID = rawText(g.labelOf(rt), 1), tb.rows[rt.Module][0].v
		}
		extra := tb.insertRow(rt, "", nil, false)
		n := 1
		if c.Maybe { // the rows shown refer to none: the label of the extra row, from labels holding it
			n = 0
			all := tb.fresh(optionsField(c) + "All")
			tb.let("let %s <- Db.from %s |> Db.all", all, rt.Name)
			out[0] = append(out[0], fmt.Sprintf("%s: %s { %s.labels | %s: %s } %s", labelFunc(c), labelFunc(c), model, optionsField(c), all, extra))
			out[1] = append(out[1], labelFunc(c)+": "+quote(rawText(g.labelOf(rt), tb.counts[rt.Module])))
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

func (g *crudGen) index(at *crudTable) {
	t := at.t
	mod := at.page("Index")
	g.names[mod] = []string{"Index"}
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
	cols := g.shownOf(t, false)
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
		cellText := g.refCell(c, "row", imports)
		if c == at.label {
			cellText = "<a href={" + at.route("Show") + " { id: row." + t.IDField + " }}>{" + textOf(c, "row."+c.Name) + "}</a>"
		}
		cells = append(cells, "<td>"+cellText+"</td>")
	}

	if at.hasBulk() { // a checkbox a row, in the bulk form (below the table)
		headers = append([]string{`<th><span class="sr-only">Choose</span></th>`}, headers...)
		cells = append([]string{fmt.Sprintf(`<td><input type="checkbox" name=%s form="bulk" value={%s} aria-label={"Choose " ++ %s} /></td>`, quote(g.wire(at.page("Bulk"), "ids")), textOf(at.idColumn(), "row."+t.IDField), paren(textOf(at.label, "row."+at.label.Name)))}, cells...)
	}
	filterCols := filterColumns(t)
	params := []string{"q: Text = \"\""}
	for _, c := range filterCols {
		params = append(params, c.Name+": Text = \"\"")
	}
	params = append(params, "sort: Text = "+quote(order), "dir: Text = \"desc\"", "page: Int = 1")

	var b strings.Builder
	b.WriteString(g.regionAt("", "routes", []string{routeLine("Index", "GET", at.path, g.keepWires(mod, at.params(mod, imports, params)), "", g.requires(at, false))}) + "\n\n")
	keyField, keyInit := "", ""
	if at.silo != nil { // the key, for the links listAt makes (it has the model, not the session)
		keyField = "  " + at.silo.key + ": " + at.silo.keyType(mod, imports) + ",\n"
		keyInit = "\n    " + at.silo.key + ": session." + at.silo.key + ","
	}
	fmt.Fprintf(&b, "type Model = {\n%s  rows: List %s,\n  labels: Labels,\n  q: Text,\n  filters: Filters,\n  sort: Text,\n  dir: Text,\n  page: Int,\n  pages: Int,\n  count: Int,\n}\n\n", keyField, t.Name)
	b.WriteString(fmt.Sprintf("perPage : Int\nperPage = %d\n\n", g.a.perPage))
	b.WriteString(g.regionAt("", "search", search) + "\n\n")
	fmt.Fprintf(&b, `-- A Filter is one of the list's selects: the column it filters, the option chosen ("" for any),
-- and each option's value in the address, its label, and the rows it selects.
type Filter = {
  name: Text,
  label: Text,
  value: Text,
  options: List { value: Text, label: Text, rows: Condition %s },
}

`, t.Name)
	b.WriteString(g.regionAt("", "filters", g.filters(at, mod, filterCols, imports)) + "\n\n")
	b.WriteString(g.regionAt("", "sorts", sorts) + "\n\n")
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

-- Index.load reads how many rows match the search and the filters, and one page of them (the last,
-- for a page past it), in the order asked for. The search text is cut to 200 characters (Text.slice):
-- a text over 10,000 bytes would fail the query (chapter 8, "Search").
`, t.Name, t.Name, order)
	readNote := ""
	if g.a.readNote != "" {
		readNote = "  " + g.a.readNote + "\n"
	}
	b.WriteString(g.load("Index", fmt.Sprintf(`  let q = Text.slice 0 200 params.q
  let f = chosen params
%s  let count <- %s %s |> Db.where (Db.and [search q, filtered (filters f)]) |> Db.count
  let pages = Int.max 1 ((count + perPage - 1) / perPage)
  let page = Int.clamp { min: 1, max: pages } params.page
  let rows <-
    %s %s
      |> Db.where (Db.and [search q, filtered (filters f)])
      |> Db.orderBy (order params.sort params.dir)
      |> Db.limit perPage
      |> Db.offset ((page - 1) * perPage)
      |> Db.all
  let labels <- labelsOf rows
  Query.succeed {%s
    rows: rows,
    labels: labels,
    q: q,
    filters: f,
    sort: params.sort,
    dir: params.dir,
    page: page,
    pages: pages,
    count: count,
  }`, readNote, g.a.sel, t.Name, g.a.sel, t.Name, keyInit)))
	b.WriteString(g.regionAt("", "labels", wrapAll(g.labels(at, mod, imports))) + "\n\n")
	newLink := fmt.Sprintf("<al-button href={%s {}}>New %s</al-button>", at.route("New"), at.noun)
	if g.a.visitorReads() { // a visitor sees the list: the link is for a signed-in user
		newLink = "-- hiding is not guarding: Create requires a signed-in user (insertMine)\n      {if session.user != Nothing then " + newLink + " else Html.none}"
	}
	b.WriteString(viewHead("Index"))
	fmt.Fprintf(&b, "  %s\n    <h1>%s</h1>\n\n", open, at.title)
	fmt.Fprintf(&b, `    <al-stack direction="row" gap="sm">
      <form method="get" action=%s>
        <al-stack direction="row" gap="sm">
          <al-input>
            <label>Search
              <input type="search" name=%s value={model.q} />
            </label>
          </al-input>
          {for f in filters model.filters ->
            <al-select label={f.label}>
              <label>{f.label}
                <select name={f.name}>
                  <option value="" selected={f.value == ""}>any</option>
                  {for o in f.options -> <option value={o.value} selected={o.value == f.value}>{o.label}</option>}
                </select>
              </label>
            </al-select>}
          <input type="hidden" name=%s value={model.sort} />
          <input type="hidden" name=%s value={model.dir} />
          <al-button type="submit" variant="secondary">Search</al-button>
        </al-stack>
      </form>
      %s
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
%s
    <al-pagination role="navigation" aria-label="Pages">
      {if model.page > 1 then <a slot="prev" href={listAt model model.sort model.dir (model.page - 1)} rel="prev">Previous</a> else <span slot="prev" aria-disabled="true">Previous</span>}
      <span>Page {Text.fromInt model.page} of {Text.fromInt model.pages}</span>
      {if model.page < model.pages then <a slot="next" href={listAt model model.sort model.dir (model.page + 1)} rel="next">Next</a> else <span slot="next" aria-disabled="true">Next</span>}
    </al-pagination>
`, at.action(""), quote(g.wire(mod, "q")), quote(g.wire(mod, "sort")), quote(g.wire(mod, "dir")), newLink, g.regionAt("            ", "headers", headers), g.regionAt("              ", "cells", cells), g.bulkForms(at))
	fmt.Fprintf(&b, "  %s\n", close)

	// the tests
	var tests []TestData
	ids := func(v string) string { return "List.map (\\row -> row." + t.IDField + ") " + v + ".model.rows" }

	mine := g.a.sel != "Db.from" // the list reads the signed-in user's rows only
	tb := g.tb(mod, imports, true)
	tb.insert(t, true)
	over, parents := g.selfParents(tb, at)
	tb.insertRow(t, "", over, true)
	checks := g.labelChecks(tb, at, "page.model", parents)
	tb.let("let page <- Test.get (Index {})")
	lhs, rhs := []string{"rows: " + ids("page")}, []string{"rows: [" + strings.Join(tb.visible(t, mine), ", ") + "]"}
	every, newest := "the list shows every row", ", newest first"
	if mine {
		every, newest = "shows my rows", " newest first"
	}
	name := every + newest
	if t.Created == "" {
		lhs, rhs = []string{"rows: List.length page.model.rows"}, []string{fmt.Sprintf("rows: %d", len(tb.visible(t, mine)))}
		name = every
	}
	if len(checks[0]) > 0 {
		name += ", and the labels of the rows they refer to"
	}
	lhs, rhs = append(lhs, checks[0]...), append(rhs, checks[1]...)
	tests = append(tests, TestData{Name: quote(name), Lines: tb.lines,
		Result: "{ " + strings.Join(lhs, ", ") + " } == { " + strings.Join(rhs, ", ") + " }"})
	if g.owned(at) && mine { // another user's row is not listed, nor counted
		tb = g.tb(mod, imports, true)
		row := tb.insertRow(t, "", nil, false)
		g.othersRow(tb, at)
		tb.let("let page <- Test.get (Index {})")
		tests = append(tests, TestData{Name: quote("does not show another user's rows"), Lines: tb.lines,
			Result: fmt.Sprintf("{ rows: %s, count: Test.text \"#count\" page.doc } == { rows: [%s], count: Just \"1 row\" }", ids("page"), row)})
	}
	if g.owned(at) && !mine { // --read signed-in|all: another user's row is listed and counted too
		tb = g.tb(mod, imports, true)
		tb.insertRow(t, "", nil, false)
		other := g.othersRow(tb, at)
		tb.let("let page <- Test.get (Index {})")
		tests = append(tests, TestData{Name: quote("shows every user's rows"), Lines: tb.lines,
			Result: fmt.Sprintf("{ other: List.member %s (%s), count: Test.text \"#count\" page.doc } == { other: True, count: Just \"%d rows\" }", other, ids("page"), len(tb.visible(t, false)))})
	}
	if g.owned(at) && g.a.visitorReads() { // --read all: a visitor sees the list, and no link to a write
		tb = g.tb(mod, imports, false)
		tb.insertRow(t, "", nil, false)
		tb.let("let r <- Test.open (Index {})")
		count := "1 row"
		if n := len(tb.visible(t, false)); n != 1 {
			count = fmt.Sprintf("%d rows", n)
		}
		tests = append(tests, TestData{Name: quote("a visitor sees the list"), Lines: tb.lines,
			Result: fmt.Sprintf("{ answer: r.answer, count: Test.text \"#count\" r.doc } == { answer: Shown, count: Just %s }", quote(count))})
		tb = g.tb(mod, imports, false)
		tb.let("let visitor <- Test.open (Index {})")
		tb.login(g.a.role)
		tb.let("let user <- Test.open (Index {})")
		newText := quote("New " + at.noun)
		tests = append(tests, TestData{Name: quote("a visitor sees no New, Edit or Delete"), Lines: tb.lines,
			Result: fmt.Sprintf("{ visitor: List.member %s (Test.texts \"al-button\" visitor.doc), user: List.member %s (Test.texts \"al-button\" user.doc) } == { visitor: False, user: True }", newText, newText)})
	}

	// sorting: a column whose order is the reverse of the rows' (and of every other column's)
	if sc := sortColumn(t); sc != nil {
		tb = g.tb(mod, imports, true)
		k := tb.counts[t.Module]
		first := tb.insertRow(t, "", map[string]string{sc.Name: tb.sample(sc.Type, sc.Name, k+2)}, true)
		second := tb.insertRow(t, "", map[string]string{sc.Name: tb.sample(sc.Type, sc.Name, k+1)}, true)
		tb.let("let up <- Test.get (%s)", pageLit("Index", []string{"sort: " + quote(sc.Name), "dir: \"asc\""}))
		tb.let("let down <- Test.get (%s)", pageLit("Index", []string{"sort: " + quote(sc.Name)}))
		others := slices.DeleteFunc(slices.Clone(tb.visible(t, mine)), func(v string) bool { return v == first || v == second })
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
		tb.let("let page <- Test.get (%s)", pageLit("Index", []string{"q: " + quote(q)}))
		tests = append(tests, TestData{Name: quote("searching finds the rows that contain the text"), Lines: tb.lines,
			Result: fmt.Sprintf("{ rows: %s, count: Test.text \"#count\" page.doc } == { rows: [%s], count: Just \"1 row\" }", ids("page"), match)})
	}

	tests = append(tests, g.filterTests(at, mod, imports, filterCols)...)

	// a search text too long for the database is cut, not a failure
	tb = g.tb(mod, imports, true)
	tb.let("let long = Text.join \"\" (List.map (\\n -> \"z\") (List.range 1 20000))")
	tb.let("let page <- Test.get (%s)", pageLit("Index", []string{"q: long"}))
	tests = append(tests, TestData{Name: quote("a search text of 20,000 characters is cut to 200"), Lines: tb.lines,
		Result: "{ q: Text.length page.model.q, rows: List.length page.model.rows } == { q: 200, rows: 0 }"})

	// perPage rows a page
	per := g.a.perPage
	tb = g.tb(mod, imports, true)
	tb.bulkInsert(t, per+1, "")
	total := len(tb.visible(t, mine)) + tb.bulk[t.Module]
	tb.let("let first <- Test.get (Index {})")
	tb.let("let second <- Test.get (%s)", pageLit("Index", []string{"page: 2"}))
	tb.let("let past <- Test.get (%s)", pageLit("Index", []string{"page: 9"}))
	pages := (total + per - 1) / per
	tests = append(tests, TestData{Name: quote(fmt.Sprintf("the list has %d rows a page, and a page past the last is the last", per)), Lines: tb.lines,
		Result: fmt.Sprintf("{ first: List.length first.model.rows, second: List.length second.model.rows, past: { page: past.model.page, rows: List.length past.model.rows }, count: Test.text \"#count\" first.doc, pages: Test.texts \"al-pagination span\" first.doc } == { first: %d, second: %d, past: { page: %d, rows: %d }, count: Just \"%d rows\", pages: [\"Previous\", \"Page 1 of %d\"] }",
			per, total-per, pages, total-per, total, pages)})

	if at.silo != nil {
		tests = append(tests, g.siloListTest(at, mod, imports))
	}
	tests = append(tests, g.pageGates(at, mod, imports, false, func(*testBuilder) string { return "Index {}" })...)
	b.WriteString(g.testsIn(mod, tests))
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
func (g *crudGen) filters(at *crudTable, mod string, cols []*Column, imports *importSet) []string {
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
		"chosen : Index.Params -> Filters",
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
			out = append(out, fmt.Sprintf("  { name: %s, label: %s, value: f.%s, options: [", quote(g.wire(mod, c.Name)), quote(Humanize(c.Name)), c.Name))
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
	if at.silo != nil {
		keep = append([]string{at.silo.key + ": model." + at.silo.key}, keep...)
	}
	out = append(out, "",
		"-- listAt is this list with the model's search and filters, sorted by sort and dir, at page n.",
		"listAt : Model -> Text -> Text -> Int -> Route",
		"listAt model sort dir n = Index "+rec(keep))
	return wrapAll(out)
}

// filterTests are the tests of the list's filters: for its first Bool column and its first union
// column, a row the option chosen selects is listed and counted, another is not, and the select
// shows the option chosen.
func (g *crudGen) filterTests(at *crudTable, mod string, imports *importSet, cols []*Column) []TestData {
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
func (g *crudGen) filterTest(at *crudTable, mod string, imports *importSet, c *Column) TestData {
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
	tb.let("let %s <- Test.get (%s)", page, pageLit("Index", []string{c.Name + ": " + quote(pick.value)}))
	lhs := []string{page + ": " + listed(page)}
	rhs := []string{page + ": [True, False]"}
	if c.Union == nil {
		tb.let("let %s <- Test.get (%s)", other.value, pageLit("Index", []string{c.Name + ": " + quote(other.value)}))
		lhs = append(lhs, other.value+": "+listed(other.value))
		rhs = append(rhs, other.value+": [False, True]")
	}
	lhs = append(lhs, fmt.Sprintf("count: %s.model.count == List.length %s.model.rows", page, page),
		fmt.Sprintf("chosen: Test.texts \"select[name=%s] option[selected]\" %s.doc", g.wireSel(mod, c.Name), page))
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

func (g *crudGen) show(at *crudTable) {
	t := at.t
	mod := at.page("Show")
	g.names[mod] = []string{"Show"}
	imports := newImports(mod, g.p.Std)
	imports.add(t.Module)
	importTypes(imports, t.IDType)
	title := UpperFirst(at.noun)
	open, close := g.layoutOpen(title, imports)
	var fields []string
	for _, c := range g.shownOf(t, true) {
		fields = append(fields, "<dt>"+Humanize(c.Name)+"</dt>", "<dd>"+g.refCell(c, "model.row", imports)+"</dd>")
	}
	// the delete dialog: the Delete page's counts, and a form posting its Destroy route; the
	// Delete page stays, for the details and for a browser that cannot open the dialog
	pl := g.deletePlan(at, imports, "params.id")
	loads := g.a.found(g.a.get, t, "params.id", pl.loads, []string{"let labels <- labelsOf [row]",
		"Query.succeed { " + strings.Join(append([]string{"row: row", "labels: labels"}, pl.fields...), ", ") + " }"}, "Query.notFound")
	if g.a.readNote != "" {
		loads = append([]string{g.a.readNote}, loads...)
	}
	var b strings.Builder
	b.WriteString(g.regionAt("", "routes", []string{routeLine("Show", "GET", at.path+"/{id}", g.keepWires(mod, at.params(mod, imports, []string{"id: " + types.StringIn(t.IDType, mod)})), "", g.requires(at, false))}) + "\n\n")
	b.WriteString("-- The row, the labels of the rows it refers to, and (for the delete dialog) how many rows of\n-- each table refer to it.\n")
	b.WriteString("type Model = {\n  row: " + t.Name + ",\n  labels: Labels,\n" + g.regionAt("  ", "model", pl.model) + "\n}\n\n")
	b.WriteString(g.load("Show", g.regionAt("  ", "load", wrapAll(loads))))
	b.WriteString(g.regionAt("", "labels", wrapAll(g.labels(at, mod, imports))) + "\n\n")
	b.WriteString(g.regionAt("", "blockers", pl.block) + "\n\n")
	b.WriteString(viewHead("Show"))
	fmt.Fprintf(&b, "  %s\n    <h1>%s</h1>\n\n    <al-card>\n      <dl id=\"fields\">\n%s\n      </dl>\n", open, title, g.regionAt("        ", "fields", fields))
	actions := fmt.Sprintf(`<div slot="actions">
  <al-button href={%s { id: model.row.%s }}>Edit</al-button>
  <al-button variant="destructive"><button type="button" commandfor="delete-dialog" command="show-modal">Delete</button></al-button>
</div>`, at.route("Edit"), t.IDField)
	dialog := fmt.Sprintf(`<al-dialog id="delete-dialog" heading="Delete this %s?" dismissible>
  <dl>
%s
  </dl>
  <form id="delete" submit={%s { id: model.row.%s }}>
    <al-alert variant="destructive"><Errors form /></al-alert>
    `+pl.button+`
  </form>
  <div slot="actions">
    <a href={%s { id: model.row.%s }}>See the details</a>
  </div>
</al-dialog>`, at.noun, g.regionAt("    ", "counts", pl.counts), at.destroy(), t.IDField, at.route("Delete"), t.IDField)
	if g.a.ownerOnly != "" { // the links to the writes, for the row's owner only
		test := g.a.ownerTest("model.row")
		actions = hidingNote + "\n{if " + test + " then\n" + indent(actions, "  ") + "\nelse\n  Html.none}"
		dialog = hidingNote + "\n{if " + test + " then\n" + indent(dialog, "  ") + "\nelse\n  Html.none}"
	}
	fmt.Fprintf(&b, "%s\n    </al-card>\n%s\n    <a href={%s {}}>All %s</a>\n", indent(actions, "      "), indent(dialog, "    "), at.route("Index"), Plural(at.noun))
	fmt.Fprintf(&b, "  %s\n", close)

	var tests []TestData
	tb := g.tb(mod, imports, true)
	over, parents := g.selfParents(tb, at)
	row := tb.insertRow(t, "", over, false)
	k := tb.counts[t.Module]
	checks := g.labelChecks(tb, at, "page.model", parents)
	tb.let("let r <- Test.open (%s)", pageLit("Show", []string{"id: " + row}))
	lhs, rhs := []string{"answer: r.answer", "fields: Test.count \"#fields dd\" r.doc"}, []string{"answer: Shown", fmt.Sprintf("fields: %d", len(g.shownOf(t, true)))}
	if !at.label.ID {
		lhs[1], rhs[1] = fmt.Sprintf("%s: List.member %s (Test.texts \"#fields dd\" r.doc)", at.label.Name, quote(rawText(at.label, k))), at.label.Name+": True"
	}
	if len(checks[0]) > 0 { // the model, for the labels it loaded
		tb.let("let page <- Test.get (%s)", pageLit("Show", []string{"id: " + row}))
	}
	lhs, rhs = append(lhs, checks[0]...), append(rhs, checks[1]...)
	tests = append(tests, TestData{Name: quote("the page shows the row"), Lines: tb.lines, Result: "{ " + strings.Join(lhs, ", ") + " } == { " + strings.Join(rhs, ", ") + " }"})
	tests = append(tests, g.deleteTests(at, mod, "Show", imports, pl)...)
	tests = append(tests, g.missingTest(at, mod, "Show", imports))
	if g.owned(at) && g.a.ownerOnly == "" {
		tests = append(tests, g.othersMissing(at, mod, "Show", "another user's row is not found", imports))
	}
	// the links to the writes: for the row's owner only (Edit, Save, Delete and Destroy are a 404
	// to anyone else, as their own tests assert)
	buttons := func(tb *testBuilder, row, name, want string, dialog int) TestData {
		tb.let("let r <- Test.open (%s)", pageLit("Show", []string{"id: " + row}))
		return TestData{Name: quote(name), Lines: tb.lines,
			Result: fmt.Sprintf("{ answer: r.answer, buttons: Test.texts \"al-card al-button\" r.doc, dialog: Test.count \"#delete-dialog\" r.doc } == { answer: Shown, buttons: [%s], dialog: %d }", want, dialog)}
	}
	if g.owned(at) && g.a.ownerOnly != "" {
		tb = g.tb(mod, imports, true)
		tests = append(tests, buttons(tb, g.othersRow(tb, at), "shows another user's row without Edit or Delete", "", 0))
		tb = g.tb(mod, imports, true)
		tests = append(tests, buttons(tb, tb.insertRow(t, "", nil, false), "shows my row with Edit and Delete", `"Edit", "Delete"`, 1))
	}
	if g.owned(at) && g.a.visitorReads() { // --read all
		tb = g.tb(mod, imports, false)
		row := tb.insertRow(t, "", nil, false)
		tb.let("let r <- Test.open (%s)", pageLit("Show", []string{"id: " + row}))
		tests = append(tests, TestData{Name: quote("a visitor sees the row"), Lines: tb.lines,
			Result: fmt.Sprintf("{ answer: r.answer, fields: Test.count \"#fields dd\" r.doc } == { answer: Shown, fields: %d }", len(g.shownOf(t, true)))})
		tb = g.tb(mod, imports, false)
		tests = append(tests, buttons(tb, tb.insertRow(t, "", nil, false), "a visitor sees no New, Edit or Delete", "", 0))
	}
	tests = append(tests, g.pageGates(at, mod, imports, false, func(tb *testBuilder) string {
		return pageLit("Show", []string{"id: " + tb.insertRow(t, "", nil, false)})
	})...)
	b.WriteString(g.testsIn(mod, tests))
	g.file(mod, imports, b.String())
}

// pageGates are the tests of who may not open a page of at (write: a page of its writes): the
// admin's gateTests; for a page without a role, a visitor is sent to log in when its route
// requires `user`. pageOf is the route value a test opens.
func (g *crudGen) pageGates(at *crudTable, mod string, imports *importSet, write bool, pageOf func(tb *testBuilder) string) []TestData {
	if g.a.role != "" {
		return g.gateTests(mod, imports, pageOf)
	}
	if g.p.Login == "" || !requiresUser(g.requires(at, write)) {
		return nil
	}
	return []TestData{g.visitorTest(mod, imports, pageOf)}
}

// visitorTest is the test that a visitor opening the page pageOf gives is sent to log in.
func (g *crudGen) visitorTest(mod string, imports *importSet, pageOf func(tb *testBuilder) string) TestData {
	tb := g.tb(mod, imports, false)
	page := pageOf(tb)
	tb.let("let r <- Test.open (%s)", page)
	return TestData{Name: quote("a visitor is sent to log in"), Lines: tb.lines, Result: "r.answer == Redirected (" + pageLit(g.p.Login, nil) + ")"}
}

// requiresUser reports whether a `route` line's clause req (" requires user, role Admin")
// requires a signed-in user.
func requiresUser(req string) bool {
	keys, ok := strings.CutPrefix(strings.TrimSpace(req), "requires ")
	return ok && slices.Contains(strings.Split(keys, ", "), "user")
}

// destroy is the Delete page's Destroy route, as another module writes it.
func (at *crudTable) destroy() string { return at.page("Delete") + ".Destroy" }

// missingTest is the test that GET route r of module mod, for a row that does not exist, is a 404.
func (g *crudGen) missingTest(at *crudTable, mod, r string, imports *importSet) TestData {
	tb := g.tb(mod, imports, true)
	missing := tb.brandCtor(at.t.IDType.(*types.Brand)) + " " + quote("missing")
	tb.let("let r <- Test.open (%s)", pageLit(r, []string{"id: " + missing}))
	return TestData{Name: quote("a missing row is not found"), Lines: tb.lines, Result: "r.answer == PageMissing"}
}

// owned is whether at's pages reach the signed-in user's rows of an `@owner` table only
// (getMine): another user's row is a missing one, which their tests assert.
func (g *crudGen) owned(at *crudTable) bool { return at.t.Owner != "" && g.a.writeGet == "Db.getMine" }

// othersRow inserts a row of at's table that another user owns, and returns its variable.
func (g *crudGen) othersRow(tb *testBuilder, at *crudTable) string {
	return tb.insertRow(at.t, "", map[string]string{at.t.Owner: tb.otherUser()}, false)
}

// othersMissing is the test, named name, that GET route r of module mod is a 404 for another
// user's row.
func (g *crudGen) othersMissing(at *crudTable, mod, r, name string, imports *importSet) TestData {
	tb := g.tb(mod, imports, true)
	row := g.othersRow(tb, at)
	tb.let("let r <- Test.open (%s)", pageLit(r, []string{"id: " + row}))
	return TestData{Name: quote(name), Lines: tb.lines, Result: "r.answer == PageMissing"}
}

// othersWrite is the test, named name, that a write to another user's row is refused and
// changes nothing: the row's owner opens the page holding the form, then the signed-in user
// sends it (a write route runs no load: its own scoped call is the guard, a 404). build, as
// writeGate's, for the row target, gives the page, the step that sends the write and the check.
// A form whose selects are of at's references (refs) is refused before that, 400, when one
// offers an owned table's rows (Db.mine): the options the owner's page offered are not the
// sender's (Plan call 11).
func (g *crudGen) othersWrite(at *crudTable, mod string, imports *importSet, name string, refs bool, build func(tb *testBuilder, target string) (get, send string, check func() (string, string))) TestData {
	tb := g.tb(mod, imports, true)
	other := tb.otherUser()
	target := g.othersRow(tb, at)
	get, send, check := build(tb, target)
	tb.loginAs(other, g.a.role)
	tb.let("let _ <- Test.get (%s)", get)
	tb.login(g.a.role)
	tb.let("let r <- %s", send)
	l, r := check()
	answer := "PageMissing"
	for _, c := range at.refs {
		if sel, _ := g.refSelect(g.p.TableNamed(c.Ref)); refs && !strings.HasPrefix(sel, "Db.from ") {
			answer = `RequestFailed "400 bad request"`
		}
	}
	return TestData{Name: quote(name), Lines: tb.lines, Result: "{ answer: r.answer, " + l + " } == { answer: " + answer + ", " + r + " }"}
}

// formName is the name of the form of a table's New and Edit pages: "PostForm".
func (at *crudTable) formName() string { return at.t.Name + "Form" }

// formRoute is the write route a test of at's New page posts the form to (Create), or of its Edit
// page for the row target (Save), as the test's Test.submit names it.
func (at *crudTable) formRoute(edit bool, target string) string {
	if edit {
		return "(Save { id: " + target + " })"
	}
	return at.own("Create", true)
}

// optionsField is the model field holding the rows a reference column's select offers.
func optionsField(c *Column) string { return Plural(c.Name) }

// totalField is the model field counting the rows of the table a reference refers to, and
// findParam the route parameter narrowing a long list of them (the search's text).
func totalField(c *Column) string { return optionsField(c) + "Total" }

func findParam(c *Column) string { return "find" + UpperFirst(c.Name) }

// finds reports whether the select of reference c can be a search: the label of the table it
// refers to is text the database can search.
func (g *crudGen) finds(c *Column) bool {
	rt := g.p.TableNamed(c.Ref)
	l := g.labelOf(rt)
	return !l.ID && l.Kind == KindText && !l.Maybe && !l.Secret()
}

// findParams are the route parameters that narrow at's long selects.
func (g *crudGen) findParams(at *crudTable) []string {
	var out []string
	for _, c := range at.refs {
		if g.finds(c) {
			out = append(out, findParam(c)+": Text = \"\"")
		}
	}
	return out
}

// finders are the searches of at's New or Edit page (module mod, its address action, a form's
// `action`): forms above the page's form, shown while a list is long, each a GET form to the
// page itself. The narrowed list is the page's parameter, so the browser sends the text to the
// page's address and the page loads again at it. The action is written out: after a refused
// write without JavaScript the address bar holds the write's path (`/admin/post`), and a form
// with no action would go there.
func (g *crudGen) finders(at *crudTable, mod, action string) (view []string) {
	for _, c := range at.refs {
		if !g.finds(c) {
			continue
		}
		view = append(view, fmt.Sprintf(`{if model.%s > 200 then <form method="get" action=%s><al-input><label>%s <input type="search" name=%s value={model.%s} /></label></al-input><al-button type="submit" variant="secondary">Find</al-button></form> else Html.none}`,
			totalField(c), action, "Find "+strings.ToLower(Humanize(c.Name)), quote(g.wire(mod, findParam(c))), findParam(c)))
	}
	return
}

// formParts are the regions a New or Edit page shares: the model's option fields, their loads,
// the form's declaration, its inputs (edit: with the row's values).
func (g *crudGen) formParts(at *crudTable, mod string, imports *importSet, edit bool) (model, loads, form, inputs []string) {
	for _, c := range at.refs {
		rt := g.p.TableNamed(c.Ref)
		imports.add(rt.Module)
		model = append(model, optionsField(c)+": List "+rt.Name+",")
		from, why := g.refSelect(rt)
		if why != "" {
			loads = append(loads, why)
		}
		sel := from
		if l := g.labelOf(rt); !l.ID {
			sel += " |> Db.orderBy " + rt.Name + "." + l.Name + ".asc"
		}
		if !g.finds(c) {
			loads = append(loads, fmt.Sprintf("let %s <- %s |> Db.all", optionsField(c), sel))
			continue
		}
		// a long table is not offered whole: past 200 rows the select holds 50, those whose label
		// contains the search; on Edit the row's own value comes first, read by itself (it may sort
		// past the 50, and a select without it would save another), and the search leaves it out;
		// the rows a submitted value is checked against are the ones rendered
		model = append(model, totalField(c)+": Int,", findParam(c)+": Text,")
		q, n := findParam(c), optionsField(c)
		where := rt.Name + "." + g.labelOf(rt).Name + ".contains " + q
		if !edit {
			loads = append(loads,
				fmt.Sprintf("let %s = Text.slice 0 200 params.%s", q, q),
				fmt.Sprintf("let %s <- %s |> Db.count", totalField(c), from),
				fmt.Sprintf("let %s <- %s |> Db.where (if %s > 200 then %s else Db.and []) |> Db.limit (if %s > 200 then 50 else 200) |> Db.all", n, sel, totalField(c), where, totalField(c)))
			continue
		}
		cur := rt.Name + "." + rt.IDField + ".eq row." + c.Name
		other := rt.Name + "." + rt.IDField + ".ne row." + c.Name
		if c.Maybe {
			cur = rt.Name + "." + rt.IDField + ".in (List.filterMap (\\i -> i) [row." + c.Name + "])"
			other = "Db.not (" + cur + ")"
		}
		own, found := n+"Own", n+"Found"
		loads = append(loads,
			fmt.Sprintf("let %s = Text.slice 0 200 params.%s", q, q),
			fmt.Sprintf("let %s <- %s |> Db.count", totalField(c), from),
			fmt.Sprintf("let %s <- if %s > 200 then %s |> Db.where (%s) |> Db.all else Query.succeed []", own, totalField(c), from, cur),
			fmt.Sprintf("let %s <- %s |> Db.where (if %s > 200 then Db.and [%s, %s] else Db.and []) |> Db.limit (if %s > 200 then 50 else 200) |> Db.all", found, sel, totalField(c), where, other, totalField(c)),
			fmt.Sprintf("let %s = %s ++ %s", n, own, found))
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
			attrs += fmt.Sprintf(" options={List.map (\\r -> { value: r.%s, label: r.%s }) model.%s}", rt.IDField, g.labelOf(rt).Name, optionsField(c))
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

func (g *crudGen) newPage(at *crudTable) {
	t := at.t
	mod := at.page("New")
	g.names[mod] = []string{"New", "Create"}
	imports := newImports(mod, g.p.Std)
	imports.add(t.Module)
	title := "New " + at.noun
	open, close := g.layoutOpen(UpperFirst(title), imports)
	model, loads, form, inputs := g.formParts(at, mod, imports, false)
	var fields []string
	for _, c := range at.refs {
		fields = append(fields, optionsField(c)+": "+optionsField(c))
		if g.finds(c) {
			fields = append(fields, totalField(c)+": "+totalField(c), findParam(c)+": "+findParam(c))
		}
	}
	loads = append(loads, "Query.succeed { "+strings.Join(fields, ", ")+" }")
	if len(fields) == 0 {
		loads[len(loads)-1] = "Query.succeed {}"
	}
	var insert []string
	var inits []string
	if len(at.nowCols) > 0 {
		insert = append(insert, "let now <- Time.now")
	}
	for _, c := range at.editable {
		inits = append(inits, c.Name+": params.form."+c.Name)
	}
	for _, c := range at.nowCols {
		inits = append(inits, c.Name+": now")
	}
	if at.silo != nil { // the page's key: the database the row goes in
		inits = append(inits, t.PartCol+": session."+at.silo.key)
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
		fmt.Sprintf("let id <- %s %s { %s }", g.a.insert, t.Name, strings.Join(inits, ", ")),
		fmt.Sprintf("Task.succeed (Response.redirect (%s { id: id }))", at.route("Show")),
	})...)

	var b strings.Builder
	path := at.path + "/new"
	finder := g.finders(at, mod, at.actionAt("/new", ""))
	routes := []string{
		routeLine("New", "GET", path, g.keepWires(mod, at.params(mod, imports, g.findParams(at))), "", g.requires(at, true)),
		routeLine("Create", "POST", at.path, at.params(mod, imports, nil), at.formName(), g.requires(at, true)),
	}
	b.WriteString(g.regionAt("", "routes", routes) + "\n\n")
	b.WriteString("type Model = {\n" + g.regionAt("  ", "model", model) + "\n}\n\n")
	b.WriteString(g.load("New", g.regionAt("  ", "load", wrapAll(loads))))
	b.WriteString(g.regionAt("", "form", form) + "\n\n")
	// the searches are GET forms with nothing to declare; the region stays empty so that regen
	// clears an older admin's search routes' forms and updates
	b.WriteString(g.regionAt("", "search", nil) + "\n\n")
	b.WriteString("-- Create adds the row and goes to its page.\n")
	b.WriteString(g.update("Create", g.regionAt("  ", "insert", wrapAll(insert))))
	b.WriteString(viewHead("New"))
	fmt.Fprintf(&b, "  %s\n    <h1>%s</h1>\n\n    <al-card>\n%s\n      <form submit={%s}>\n%s\n", open, UpperFirst(title), g.regionAt("      ", "finder", finder), at.own("Create", false), g.regionAt("        ", "inputs", inputs))
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
	if g.owned(at) { // insertMine: the signed-in user's
		got = append(got, t.Owner+": row."+t.Owner)
		want = append(want, t.Owner+": "+tb.user())
	}
	tb.let("let page <- Test.get (New {})")
	tb.let("let r <- Test.submit %s { %s }", at.own("Create", true), strings.Join(values, ", "))
	sel := "Db.from " + t.Name
	if k > 0 && !at.label.ID { // rows of the table were inserted before (the signed-in user): the new one by its label
		v := valueOf(values, at.label.Name)
		if strings.Contains(v, " ") {
			v = "(" + v + ")"
		}
		sel += fmt.Sprintf(" |> Db.where (%s.%s.eq %s)", t.Name, at.label.Name, v)
	}
	tb.let("let rows <- %s |> Db.all", sel)
	lhs := []string{fmt.Sprintf("answers: List.map (\\row -> Redirected (%s { id: row.%s })) rows", at.route("Show"), t.IDField),
		"rows: List.map (\\row -> { " + strings.Join(got, ", ") + " }) rows"}
	rhs := []string{"answers: [r.answer]", "rows: [{ " + strings.Join(want, ", ") + " }]"}
	lhs, rhs = append(lhs, optLines[0]...), append(rhs, optLines[1]...)
	if len(got) == 0 {
		lhs, rhs = lhs[:1], rhs[:1]
	}
	name := "creating " + article(at.noun) + " adds it, with the values submitted"
	if g.owned(at) {
		name = "creating adds a row that is mine"
	}
	tests = append(tests, TestData{Name: quote(name), Lines: tb.lines,
		Result: "{ " + strings.Join(lhs, ", ") + " } == { " + strings.Join(rhs, ", ") + " }"})
	if g.a.role == "" { // not the admin's (its goldens predate it)
		tests = append(tests, g.emptyTest(at, mod, imports)...)
	}
	tests = append(tests, g.uniqueTests(at, mod, imports, false)...)
	tests = append(tests, g.notOffered(at, mod, imports, false)...)
	tests = append(tests, g.findTests(at, mod, imports, false)...)
	tests = append(tests, g.writeGate(mod, imports, "create "+article(at.noun), len(at.refs) > 0, func(tb *testBuilder) (string, string, func() (string, string)) {
		g.optionRows(tb, at)
		if len(at.uniques()) > 0 && tb.counts[t.Module] == 0 {
			tb.insertRow(t, "_", nil, false)
		}
		k := tb.counts[t.Module]
		var values []string
		for _, c := range at.editable {
			values = append(values, c.Name+": "+g.submitValue(tb, at, c, k+1))
		}
		return "New {}", fmt.Sprintf("Test.submit %s { %s }", at.own("Create", true), strings.Join(values, ", ")), func() (string, string) {
			tb.let("let rows <- Db.from %s |> Db.count", t.Name)
			return "rows: rows", fmt.Sprintf("rows: %d", k)
		}
	})...)
	tests = append(tests, g.pageGates(at, mod, imports, true, func(*testBuilder) string { return "New {}" })...)
	b.WriteString(g.testsIn(mod, tests))
	g.file(mod, imports, b.String())
}

// emptyTest is the test that the New page refuses its first required field that may not be
// empty (emptyMessage), sent empty with the other fields valid: the field's error, and nothing
// written (the rows the list reads, counted). None when no field may not be empty.
func (g *crudGen) emptyTest(at *crudTable, mod string, imports *importSet) []TestData {
	var c *Column
	for _, d := range at.editable {
		if msg := emptyMessage(d.Type); msg != "" && c == nil {
			c = d
		}
	}
	if c == nil {
		return nil
	}
	tb := g.tb(mod, imports, true)
	g.optionRows(tb, at)
	tb.let("let page <- Test.get (New {})")
	var raw []string
	for _, d := range at.editable {
		v := quote(rawSample(d.Type))
		switch {
		case d == c:
			v = `""`
		case d.Ref != "" && g.p.TableNamed(d.Ref) != nil && !d.Maybe:
			v = d.Name + "Id"
			tb.let("let %s = Maybe.withDefault \"\" (Test.attr \"value\" \"select[name=%s] option\" page.doc)", v, d.Name)
		}
		raw = append(raw, fmt.Sprintf("{ name: %s, value: %s }", quote(d.Name), v))
	}
	tb.let("let r <- Test.submitRaw %s [%s]", at.own("Create", true), strings.Join(raw, ", "))
	tb.let("let rows <- %s %s |> Db.count", g.a.sel, at.t.Name)
	return []TestData{{Name: quote("an empty " + strings.ToLower(Humanize(c.Name)) + " is rejected"), Lines: tb.lines,
		Result: fmt.Sprintf("{ answer: r.answer, rows: rows } == { answer: FormRejected [{ field: %s, message: %s }], rows: %d }",
			quote(c.Name), quote(emptyMessage(c.Type)), tb.counts[at.t.Module])}}
}

// notOffered are the tests that a New (or Edit) page refuses, for each reference, a row id its
// select did not offer (a request crafted by hand): the field error "must be one of the
// options" (chapter 5), and nothing written. The other fields are sent valid: a reference the
// first option's id, read from the page.
func (g *crudGen) notOffered(at *crudTable, mod string, imports *importSet, edit bool) []TestData {
	var tests []TestData
	for _, c := range at.refs {
		tb := g.tb(mod, imports, true)
		page := "New {}"
		target := ""
		if edit {
			target = tb.insertRow(at.t, "", nil, false)
			page = pageLit("Edit", []string{"id: " + target})
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
		tb.let("let r <- Test.submitRaw %s [%s]", at.formRoute(edit, target), strings.Join(raw, ", "))
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
func (at *crudTable) uniques() []*Column {
	var out []*Column
	for _, c := range at.editable {
		if c.Unique {
			out = append(out, c)
		}
	}
	return out
}

// takenText is the refusal of a form whose `@unique` column c holds a value another row has.
func takenText(at *crudTable, c *Column) string {
	return UpperFirst(article(at.noun)) + " with this " + strings.ToLower(Humanize(c.Name)) + " exists."
}

// unique is an arm's body, write (the write and the redirect), guarded for each `@unique`
// column of at's form by a count of the rows that already hold the submitted value (other than
// the row self, on an Edit page): such a value answers `Response.invalid`, which the form shows, where the
// database would refuse the write with an error (a 500).
func (g *crudGen) unique(at *crudTable, self string, write []string) []string {
	// answers Response.invalid, which the form shows
	cols := at.uniques()
	if len(cols) == 0 {
		return write
	}
	t := at.t
	var out []string
	for _, c := range cols {
		conds := []string{t.Name + "." + c.Name + ".eq params.form." + c.Name}
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
		if g.a.scoped() {
			out = append(out, "-- unscoped: @unique holds across every user's rows")
		}
		out = append(out, fmt.Sprintf("let %sTaken <- Db.from %s |> Db.where (%s) |> Db.count", c.Name, t.Name, cond))
	}
	for i, c := range cols {
		lead := "else if"
		if i == 0 {
			lead = "if"
		}
		out = append(out, fmt.Sprintf("%s %sTaken > 0 then", lead, c.Name), "  Task.succeed (Response.invalid "+quote(takenText(at, c))+")")
	}
	out = append(out, "else {")
	out = append(out, indentAll(write, "  ")...)
	return append(out, "}")
}

// uniqueTests are the tests that a New (or Edit) page refuses a value of a `@unique` column
// that another row holds: the form's error, and nothing written.
func (g *crudGen) uniqueTests(at *crudTable, mod string, imports *importSet, edit bool) []TestData {
	var tests []TestData
	t := at.t
	for _, c := range at.uniques() {
		tb := g.tb(mod, imports, true)
		page := "New {}"
		target := ""
		if edit {
			target = tb.insertRow(t, "", nil, false)
			page = pageLit("Edit", []string{"id: " + target})
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
		tb.let("let r <- Test.submit %s { %s }", at.formRoute(edit, target), strings.Join(values, ", "))
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
func (g *crudGen) optionRows(tb *testBuilder, at *crudTable) [2][]string {
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
			labels = append(labels, quote(rawText(g.labelOf(rt), i+1)))
		}
		if g.labelOf(rt).ID {
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
func (g *crudGen) submitValue(tb *testBuilder, at *crudTable, c *Column, n int) string {
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

func (g *crudGen) edit(at *crudTable) {
	t := at.t
	mod := at.page("Edit")
	g.names[mod] = []string{"Edit", "Save"}
	imports := newImports(mod, g.p.Std)
	imports.add(t.Module)
	importTypes(imports, t.IDType)
	title := "Edit " + at.noun
	open, close := g.layoutOpen(UpperFirst(title), imports)
	model, loads, form, inputs := g.formParts(at, mod, imports, true)
	fields := []string{"row: row"}
	for _, c := range at.refs {
		fields = append(fields, optionsField(c)+": "+optionsField(c))
		if g.finds(c) {
			fields = append(fields, totalField(c)+": "+totalField(c), findParam(c)+": "+findParam(c))
		}
	}
	// the options are read with the row in hand: the select of a long table keeps its value
	loads = g.a.found(g.a.writeGet, t, "params.id", nil, append(loads, "Query.succeed { "+strings.Join(fields, ", ")+" }"), "Query.notFound")
	var sets []string
	for _, c := range at.editable {
		sets = append(sets, c.Name+": params.form."+c.Name)
	}
	// Save loads the row first when it must match on it (Db.get) or read it (the user table's own
	// role); else the scoped update alone finds it, a row out of reach being not found
	role := g.ownRole(at)
	direct := !g.a.maybeGet(g.a.writeGet) && role == nil
	id := "row." + t.IDField
	if direct {
		id = "params.id"
	}
	upd := fmt.Sprintf("let _ <- %s %s %s (\\r -> { r | %s })", g.a.update, t.Name, id, strings.Join(sets, ", "))
	if len(sets) == 0 {
		upd = fmt.Sprintf("let _ <- %s %s %s (\\r -> r)", g.a.update, t.Name, id)
	}

	var b strings.Builder
	path := at.path + "/{id}/edit"
	idParam := "id: " + types.StringIn(t.IDType, mod)
	finder := g.finders(at, mod, at.actionAt("/{id}/edit", textOf(at.idColumn(), "model.row."+t.IDField)))
	routes := []string{
		routeLine("Edit", "GET", path, g.keepWires(mod, at.params(mod, imports, append([]string{idParam}, g.findParams(at)...))), "", g.requires(at, true)),
		routeLine("Save", "PUT", at.path+"/{id}", at.params(mod, imports, []string{idParam}), at.formName(), g.requires(at, true)),
	}
	b.WriteString(g.regionAt("", "routes", routes) + "\n\n")
	b.WriteString("type Model = {\n  row: " + t.Name + ",\n" + g.regionAt("  ", "model", model) + "\n}\n\n")
	b.WriteString(g.load("Edit", g.regionAt("  ", "load", wrapAll(loads))))
	b.WriteString(g.regionAt("", "form", form) + "\n\n")
	// the searches are GET forms with nothing to declare; the region stays empty so that regen
	// clears an older admin's search routes' forms and updates
	b.WriteString(g.regionAt("", "search", nil) + "\n\n")
	save := g.unique(at, id, []string{upd, fmt.Sprintf("Task.succeed (Response.redirect (%s { id: %s }))", at.route("Show"), id)})
	if role != nil { // the signed-in user's own role is not changed: an Admin would lock themselves out
		save = append([]string{
			fmt.Sprintf("if %s && params.form.%s != row.%s then", g.isMe("row."+t.IDField), role.Name, role.Name),
			"  Task.succeed (Response.invalid " + quote(roleText) + ")",
			"else {",
		}, append(indentAll(save, "  "), "}")...)
	}
	if direct {
		b.WriteString("-- Save writes the form over the row (another user's, or one deleted since, is not found) and goes\n-- to its page.\n")
	} else {
		b.WriteString("-- Save writes the form over the row (a row deleted since is not found) and goes to its page.\n")
	}
	saved := []string{g.regionAt("", "update", wrapAll(save))}
	if !direct {
		saved = g.a.found(g.a.writeGet, t, "params.id", nil, saved, "Task.notFound")
	}
	b.WriteString(g.update("Save", indent(strings.Join(saved, "\n"), "  ")))
	b.WriteString(viewHead("Edit"))
	fmt.Fprintf(&b, "  %s\n    <h1>%s</h1>\n\n    <al-card>\n%s\n      <form submit={Save { id: model.row.%s }}>\n%s\n", open, UpperFirst(title), g.regionAt("      ", "finder", finder), t.IDField, g.regionAt("        ", "inputs", inputs))
	b.WriteString("        <al-alert variant=\"destructive\"><Errors form /></al-alert>\n        <al-button type=\"submit\">Save</al-button>\n      </form>\n    </al-card>\n")
	fmt.Fprintf(&b, "    <a href={%s { id: model.row.%s }}>Back</a>\n", at.route("Show"), t.IDField)
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
	tb.let("let page <- Test.get (%s)", pageLit("Edit", []string{"id: " + target}))
	tb.let("let r <- Test.submit %s { %s }", at.formRoute(true, target), strings.Join(values, ", "))
	tb.let("let after <- Db.get %s %s", t.Name, target)
	lhs := []string{"answer: r.answer", "row: Maybe.map (\\row -> { " + strings.Join(got, ", ") + " }) after"}
	rhs := []string{fmt.Sprintf("answer: Redirected (%s { id: %s })", at.route("Show"), target), "row: Just { " + strings.Join(want, ", ") + " }"}
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
	tests = append(tests, g.findTests(at, mod, imports, true)...)
	saveAs := func(tb *testBuilder, target string) (string, string, func() (string, string)) {
		g.optionRows(tb, at)
		k := tb.counts[t.Module]
		var values []string
		for _, c := range at.editable {
			values = append(values, c.Name+": "+g.changedValue(tb, at, c, k+1))
		}
		tb.let("let before <- Db.get %s %s", t.Name, target)
		return pageLit("Edit", []string{"id: " + target}), fmt.Sprintf("Test.submit %s { %s }", at.formRoute(true, target), strings.Join(values, ", ")), func() (string, string) {
			tb.let("let after <- Db.get %s %s", t.Name, target)
			return "row: after", "row: before"
		}
	}
	tests = append(tests, g.writeGate(mod, imports, "save "+article(at.noun), len(at.refs) > 0, func(tb *testBuilder) (string, string, func() (string, string)) {
		return saveAs(tb, tb.insertRow(t, "", nil, false))
	})...)
	if g.owned(at) {
		tests = append(tests, g.othersWrite(at, mod, imports, "another user cannot save it", true, saveAs))
	}
	tests = append(tests, g.missingTest(at, mod, "Edit", imports))
	if g.owned(at) {
		name := "another user's row: the edit page is not found"
		if g.a.ownerOnly != "" { // --read signed-in|all: Show shows it, Edit does not
			name = "another user's edit page is not found"
		}
		tests = append(tests, g.othersMissing(at, mod, "Edit", name, imports))
	}
	tests = append(tests, g.pageGates(at, mod, imports, true, func(tb *testBuilder) string {
		return pageLit("Edit", []string{"id: " + tb.insertRow(t, "", nil, false)})
	})...)
	b.WriteString(g.testsIn(mod, tests))
	g.file(mod, imports, b.String())
}

// changedValue is a value for column c of the row a test inserted (the n-1-th sample) that
// differs from the one it has: the n-th sample, the newest row of a reference, another
// constructor, the other Bool, `Just` for a Maybe.
func (g *crudGen) changedValue(tb *testBuilder, at *crudTable, c *Column, n int) string {
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

// delPlan is what deleting a row of at involves, which the Delete page and the Show page's
// dialog both show, and the Delete page's Destroy does: the model's counts of the rows that refer
// to it, the loads that count them, the functions that sum the rows keeping it (region
// `blockers`), Destroy's body (region `remove`), the counts' markup and the button (or the
// reasons there is none; for the user table, also the signed-in user's own row).
type delPlan struct {
	model, loads, fields, counts, block, remove []string
	button                                      string
	nodes, chains                               []*node
	blockers, children                          []string
	own                                         bool
}

func (g *crudGen) deletePlan(at *crudTable, imports *importSet, id string) *delPlan {
	t := at.t
	var model, loads, counts, blockers, children []string
	var fields []string
	var chains []*node // the @restrict references down a @cascade, each counted
	nodes := g.closure(at)
	for _, n := range walk(nodes) {
		imports.add(n.t.Module)
		model = append(model, n.field+": Int,")
		cond := eqID(n.t, n.col, id)
		if n.parent != nil { // the rows that refer to the rows its parent deletes
			ids := "List.map (\\row -> row." + n.parent.t.IDField + ") " + n.parent.field + "Rows"
			if n.col.Maybe {
				ids = "List.map (\\row -> Just row." + n.parent.t.IDField + ") " + n.parent.field + "Rows"
			}
			cond = fmt.Sprintf("%s.%s.in (%s)", n.t.Name, n.col.Name, ids)
		}
		sel := fmt.Sprintf("Db.from %s |> Db.where (%s)", n.t.Name, cond)
		if g.a.scoped() {
			loads = append(loads, "-- unscoped: a row of "+n.t.Name+" that refers to it may be another user's")
		}
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
	own := g.isUserTable(at) // the signed-in user's own row is not deleted
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
	remove := []string{
		"let _ <- " + g.a.remove + " " + t.Name + " params.id",
		"Task.succeed (Response.redirect (" + at.route("Index") + " {}) |> Response.success " + quote(deletedText) + ")",
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
			guard = append(guard, "if "+g.isMe("params.id")+" then", "  Task.succeed (Response.invalid "+quote(ownText)+")")
		}
		if len(children) > 0 {
			guard = append(guard, lead()+" children model > 0 then", "  Task.succeed (Response.invalid "+quote(childrenText)+")")
		}
		if len(blockers) > 0 {
			guard = append(guard, lead()+" blockers model > 0 then", "  Task.succeed (Response.invalid "+quote(rejectText(at))+")")
		}
		remove = append(append(guard, "else {"), append(indentAll(remove, "  "), "}")...)
	}
	button := `{if blockers model > 0 then <p>It cannot be deleted while other rows refer to it.</p> else <al-button variant="destructive" type="submit">Delete</al-button>}`
	if len(children) > 0 || own { // one line per reason
		var arms []string
		if own {
			arms = append(arms, g.isMe("model.row."+t.IDField)+" then <p>"+ownText+"</p>")
		}
		if len(children) > 0 {
			arms = append(arms, "children model > 0 then <p>"+childrenText+"</p>")
		}
		arms = append(arms, "blockers model > 0 then <p>It cannot be deleted while other rows refer to it.</p>")
		button = "{if " + strings.Join(arms, "\n          else if ") + "\n          else <al-button variant=\"destructive\" type=\"submit\">Delete</al-button>}"
	}
	return &delPlan{model: model, loads: loads, fields: fields, counts: counts, block: block, remove: remove,
		button: button, nodes: nodes, chains: chains, blockers: blockers, children: children, own: own}
}

func (g *crudGen) delete(at *crudTable) {
	t := at.t
	mod := at.page("Delete")
	g.names[mod] = []string{"Delete", "Destroy"}
	imports := newImports(mod, g.p.Std)
	imports.add(t.Module)
	importTypes(imports, t.IDType)
	title := "Delete " + at.noun
	open, close := g.layoutOpen(UpperFirst(title), imports)
	idT := types.StringIn(t.IDType, mod)

	pl := g.deletePlan(at, imports, "id")
	succeed := "Query.succeed { " + strings.Join(append([]string{"row: row"}, pl.fields...), ", ") + " }"
	loads := g.a.found(g.a.writeGet, t, "id", pl.loads, []string{succeed}, "Query.notFound")
	if g.a.maybeGet(g.a.writeGet) { // found's lines up to `match found {`, then the arm on one line, as the admin has always written it
		m := slices.Index(loads, "match found {")
		loads = append(loads[:m+1], "  Just row -> "+succeed+",", "  Nothing -> Query.notFound,", "}")
	}
	labelRow := "<p>{" + textOf(at.label, "model.row."+at.label.Name) + "}</p>"

	var b strings.Builder
	path := at.path + "/{id}/delete"
	params := at.params(mod, imports, []string{"id: " + idT})
	b.WriteString(g.regionAt("", "routes", []string{
		routeLine("Delete", "GET", path, g.keepWires(mod, params), "", g.requires(at, true)),
		routeLine("Destroy", "DELETE", at.path+"/{id}", params, "Confirm", g.requires(at, true)),
	}) + "\n\n")
	b.WriteString("-- The row, and how many rows of each table refer to it.\n")
	b.WriteString("type Model = {\n  row: " + t.Name + ",\n" + g.regionAt("  ", "model", pl.model) + "\n}\n\n")
	b.WriteString("-- counted is the row id names (none is not found) and how many rows of each table refer to it:\n")
	b.WriteString("-- the page shows them, and Destroy counts them again before it deletes.\n")
	fmt.Fprintf(&b, "counted : %s -> Query Model\ncounted id = {\n%s\n}\n\n", idT, g.regionAt("  ", "load", wrapAll(loads)))
	b.WriteString(g.load("Delete", "  counted params.id"))
	b.WriteString(g.regionAt("", "blockers", pl.block) + "\n\n")
	b.WriteString("-- Destroy takes no input: the form is the button, and the place a refusal is shown.\nform Confirm = { }\n\n")
	b.WriteString(`-- Destroy deletes the row, unless rows refer to it that keep it (counted again, as the page
-- counted them): the database would refuse the delete with an error, and the page says why
-- instead. The Show page's dialog posts it too.
`)
	b.WriteString(g.update("Destroy", "  let model <- counted params.id\n"+g.regionAt("  ", "remove", wrapAll(pl.remove))))
	b.WriteString(viewHead("Delete"))
	fmt.Fprintf(&b, "  %s\n    <h1>Delete this %s?</h1>\n\n    <al-card>\n%s\n      <dl>\n%s\n      </dl>\n", open, at.noun, g.regionAt("      ", "row", []string{labelRow}), g.regionAt("        ", "counts", pl.counts))
	fmt.Fprintf(&b, `      <form id="delete" submit={Destroy { id: model.row.%s }}>
        <al-alert variant="destructive"><Errors form /></al-alert>
        `+pl.button+`
      </form>
    </al-card>
    <a href={%s { id: model.row.%s }}>Back</a>
`, t.IDField, at.route("Show"), t.IDField)
	fmt.Fprintf(&b, "  %s\n", close)

	tests := g.deleteTests(at, mod, "Delete", imports, pl)
	destroy := func(tb *testBuilder, target string) (string, string, func() (string, string)) {
		return pageLit("Delete", []string{"id: " + target}), fmt.Sprintf("Test.submit (Destroy { id: %s }) { }", target), func() (string, string) {
			tb.let("let kept <- Db.get %s %s", t.Name, target)
			return "kept: Maybe.map (\\row -> row." + t.IDField + ") kept", "kept: Just " + target
		}
	}
	tests = append(tests, g.writeGate(mod, imports, "delete "+article(at.noun), false, func(tb *testBuilder) (string, string, func() (string, string)) {
		return destroy(tb, tb.insertRow(t, "", nil, false))
	})...)
	if g.owned(at) {
		tests = append(tests, g.othersWrite(at, mod, imports, "another user cannot delete it", false, destroy))
	}
	tests = append(tests, g.missingTest(at, mod, "Delete", imports))
	if g.owned(at) {
		tests = append(tests, g.othersMissing(at, mod, "Delete", "another user's row is not found", imports))
	}
	tests = append(tests, g.pageGates(at, mod, imports, true, func(tb *testBuilder) string {
		return pageLit("Delete", []string{"id: " + tb.insertRow(t, "", nil, false)})
	})...)
	b.WriteString(g.testsIn(mod, tests))
	g.file(mod, imports, b.String())
}

// deleteTests are the tests of Destroy pressed on the page of GET route get of module mod (the
// Delete page, or the Show page's dialog): a row nothing refers to is deleted, the rows that go
// with it are counted, each @restrict reference (and one down a @cascade) refuses, a row with
// children is refused, and the signed-in user's own row is.
func (g *crudGen) deleteTests(at *crudTable, mod, get string, imports *importSet, pl *delPlan) []TestData {
	t := at.t
	destroy := at.destroy()
	if get == "Delete" {
		destroy = "Destroy"
	}
	press := func(tb *testBuilder, v, row string) {
		tb.let("let %s <- Test.submit (%s { id: %s }) { }", v, destroy, row)
	}
	nodes, chains, own := pl.nodes, pl.chains, pl.own
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
	tb.let("let page <- Test.get (%s)", pageLit(get, []string{"id: " + target}))
	press(tb, "r", target)
	tb.let("let gone <- Db.get %s %s", t.Name, target)
	// the page offers the button: the rows that refer to the other row are not counted for this one
	lhs := []string{"button: Test.count \"#delete al-button\" page.doc", "answer: r.answer", "notices: r.notices", "gone: Maybe.map (\\row -> row." + t.IDField + ") gone"}
	rhs := []string{"button: 1", "answer: Redirected (" + at.route("Index") + " {})", "notices: [" + quote(deletedText) + "]", "gone: Nothing"}
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
		tb.let("let page <- Test.get (%s)", pageLit(get, []string{"id: " + target}))
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
		tb.let("let page <- Test.get (%s)", pageLit(get, []string{"id: " + target}))
		press(tb, "r", target)
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
		tb.let("let page <- Test.get (%s)", pageLit(get, []string{"id: " + target}))
		press(tb, "r", target)
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
		tb.let("let page <- Test.get (%s)", pageLit(get, []string{"id: " + top}))
		press(tb, "r", top)
		lhs := "says: Test.texts \"#delete p\" page.doc, buttons: Test.count \"#delete button\" page.doc, answer: r.answer"
		rhs := fmt.Sprintf("says: [%s], buttons: 0, answer: FormRejected [{ field: \"\", message: %s }]", quote(childrenText), quote(childrenText))
		name := "deleting " + article(at.noun) + " with children is refused"
		if hold != nil {
			tb.let("let _ <- Test.get (%s)", pageLit(get, []string{"id: " + leaf}))
			press(tb, "leaf", leaf)
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
		tb.let("let page <- Test.get (%s)", pageLit(get, []string{"id: " + me}))
		press(tb, "r", me)
		tb.let("let kept <- Db.get %s %s", t.Name, me)
		tests = append(tests, TestData{Name: quote("an Admin cannot delete their own account"), Lines: tb.lines,
			Result: fmt.Sprintf("{ says: Test.texts \"#delete p\" page.doc, buttons: Test.count \"#delete button\" page.doc, answer: r.answer, kept: Maybe.map (\\row -> row.%s) kept } == { says: [%s], buttons: 0, answer: FormRejected [{ field: \"\", message: %s }], kept: Just %s }",
				t.IDField, quote(ownText), quote(ownText), me)})
	}
	return tests
}

// childrenText is the Delete page's refusal of a row with children (node.stop).
const childrenText = "Delete its children first."

// deletedText is Destroy's toast.
const deletedText = "Deleted"

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
func rejectText(at *crudTable) string {
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
func (g *crudGen) referrers(tb *testBuilder, at *crudTable, row string) {
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
