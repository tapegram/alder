package generate

import (
	"embed"
	"fmt"
	"strings"
	"text/template"

	"alder/internal/check"
	"alder/internal/types"
)

//go:embed templates/*.tmpl
var templateFS embed.FS

// templates are the generators' templates (templates/*.tmpl), with `[[` `]]` as delimiters:
// Alder's own braces would otherwise read as template actions.
var templates = template.Must(template.New("").Delims("[[", "]]").ParseFS(templateFS, "templates/*.tmpl"))

// render executes template name with data and tidies the result (Tidy).
func render(name string, data any) string {
	var b strings.Builder
	if err := templates.ExecuteTemplate(&b, name, data); err != nil {
		panic(fmt.Sprintf("generate: template %s: %v", name, err))
	}
	return Tidy(b.String())
}

// PageData is the data every page template takes (page.tmpl, page_list.tmpl, page_show.tmpl,
// page_from.tmpl; the header and tests of common.tmpl).
type PageData struct {
	Module   string   // "Posts.Index"
	Imports  []string // sorted
	Path     string   // "/posts", "/posts/{id}"
	Params   string   // the parameter record: "{ }", "{ page: Int = 1 }", "{ id: Post.PostId }"
	Requires string   // "", " requires user", " requires user, role Admin"
	Title    string   // the heading and the layout's title: "Posts"
	Open     string   // the view's root element: `<Layout.Page title="Posts">` or `<al-page>`
	Close    string   // its closing tag
	Tests    []TestData
	// the table of --list and --show
	T      string    // the table's record type as written here: "Post"
	Select string    // the rows: "Db.from Post", or "Db.mine Post" on a `requires user` page of an `@owner` table
	Order  string    // the column a list is ordered by, newest first
	Mine   bool      // --show reads with Db.getMine
	Cols   []ColData // the columns shown
	Lists  []FromList
}

// ColData is one column a page shows: its label and the markup of its cell, for `row` (a list)
// or `model.row` (a show page).
type ColData struct{ Label, Cell string }

// FromList is one table of a `--from` page: the Model field holding its rows, the table, how
// they are read, the card's heading, the markup of a row.
type FromList struct{ Field, T, Select, Title, Cell string }

// TestData is a generated test: its name (quoted), the statements of its block, the expression
// its `Task.succeed` gives (a `==`, written in parentheses), and a comment above it.
type TestData struct {
	Name    string
	Lines   []string
	Result  string
	Comment []string // lines of a comment above the test, without `-- `
}

// Succeed is the test's last line: `Task.succeed (<Result>)`, or without the parentheses when
// Result is one word.
func (t TestData) Succeed() string {
	if strings.ContainsAny(t.Result, " ") {
		return "Task.succeed (" + t.Result + ")"
	}
	return "Task.succeed " + t.Result
}

// PageOptions are the options of `alder new page`.
type PageOptions struct {
	Module   string
	Path     string
	List     string   // --list T
	Show     string   // --show T
	From     []string // --from T …
	Requires string   // --requires user
	Role     string   // --role R
}

// NewPage generates `alder new page`: the module's file, with one browser test (two for
// --show). It refuses an existing module, a path with parameters, a table that is not one of
// the app's, and a role the session does not have; a path another page has is the checker's
// E0016, surfaced by Apply.
func NewPage(p *Program, o PageOptions) ([]Edit, error) {
	if !IsModuleName(o.Module) {
		return nil, Refusef("`%s` is not a module name: write capitalised names joined by dots, such as `Posts.Index`", o.Module)
	}
	file := FileOf(o.Module)
	if m := p.Modules[o.Module]; m != nil {
		return nil, Refusef("module `%s` exists (`%s`): a generator never overwrites a file; choose another module name", o.Module, m.File)
	}
	if !strings.HasPrefix(o.Path, "/") {
		return nil, Refusef("`%s` is not a path: a page's path starts with `/`, such as `/posts`", o.Path)
	}
	if o.Show != "" {
		o.Path = strings.TrimSuffix(o.Path, "/{id}") // `--show T` adds it; given, it is the same page
		if o.Path == "" {
			o.Path = "/"
		}
	}
	if strings.ContainsAny(o.Path, "{}:") {
		return nil, Refusef("`%s` has a parameter: give the path without it (`--show T` adds `/{id}`), or write the page's `page` line yourself", o.Path)
	}
	if why := check.PagePathProblem(o.Path); why != "" {
		return nil, Refusef("`%s` is not a page path: it %s; %s", o.Path, why, check.PagePathRule)
	}
	kinds := 0
	for _, set := range []bool{o.List != "", o.Show != "", len(o.From) > 0} {
		if set {
			kinds++
		}
	}
	if kinds > 1 {
		return nil, Refusef("choose one of --list, --show and --from: a page generated with --list shows one table's rows, --show one row, --from several tables")
	}
	if o.Role != "" {
		if p.Session == nil || p.Session.User == nil || p.Session.Role == nil {
			return nil, Refusef("`--role %s` needs a session with a `user` field and a `role` field whose type is a union (`session { user: Maybe User.UserId, role: User.Role = Reader }` in `App`): this app's session has %s", o.Role, missingSession(p))
		}
		o.Requires = "user"
	}
	if o.Requires != "" && o.Requires != "user" {
		return nil, Refusef("`--requires %s`: a generated page can require `user`, the session's signed-in user", o.Requires)
	}
	if o.Requires == "user" && (p.Session == nil || p.Session.User == nil) {
		return nil, Refusef("`--requires user` needs a session with a `user` field (`session { user: Maybe User.UserId }` in `App`): this app's session has none")
	}
	if o.Role != "" {
		r := p.Session.Role
		if !contains1(r.Ctors, o.Role) {
			return nil, Refusef("`--role %s`: `%s` is not a constructor of the session's `role` type `%s` (%s)", o.Role, o.Role, types.String(r.Type), strings.Join(r.Ctors, ", "))
		}
	}
	table := func(name string) (*Table, error) {
		if t := p.TableNamed(name); t != nil {
			return t, nil
		}
		return nil, Refusef("`%s` is not a table of this app (its tables: %s)", name, tableNames(p))
	}

	imports := newImports(o.Module, p.Std)
	d := PageData{Module: o.Module, Path: o.Path, Params: "{ }", Title: Title(o.Module)}
	if o.Requires != "" {
		d.Requires = " requires user"
		if o.Role != "" {
			d.Requires += ", role " + o.Role
		}
	}
	d.Open, d.Close = rootTags(p, d.Title, imports)
	// newTest starts a test's builder, signed in as the page requires
	newTest := func() *testBuilder {
		tb := newTestBuilder(p, o.Module, imports)
		if o.Requires != "" {
			tb.login(o.Role)
		}
		return tb
	}
	heading := fmt.Sprintf("Test.text \"h1\" r.doc } == { answer: Shown, heading: Just %s }", quote(d.Title))
	tmpl := "page.tmpl"
	var pageOf func(tb *testBuilder) string // the page value a test opens
	switch {
	case o.List != "":
		t, err := table(o.List)
		if err != nil {
			return nil, err
		}
		imports.add(t.Module)
		tmpl = "page_list.tmpl"
		d.Params = "{ page: Int = 1 }"
		d.T, d.Order = recordName(t, o.Module), t.OrderColumn()
		d.Select = selectOf(t, o.Module, o.Requires != "")
		for _, c := range t.Columns {
			if !c.ID {
				d.Cols = append(d.Cols, ColData{Humanize(c.Name), cell(c, "row."+c.Name, imports)})
			}
		}
		mine := o.Requires != "" && t.Owner != ""
		pageOf = func(*testBuilder) string { return pageLit(o.Module, nil) }

		// the rows, newest first (and, read with Db.mine, not another user's)
		tb := newTest()
		tb.insert(t, true)
		tb.insert(t, true)
		name := "the list shows the rows, newest first"
		if mine {
			tb.insertRow(t, "", map[string]string{t.Owner: tb.otherUser()}, false)
			name = "the list shows my rows, newest first"
		}
		tb.let("let page <- Test.get (%s)", pageLit(o.Module, nil))
		result := fmt.Sprintf("List.map (\\row -> row.%s) page.model.rows == [%s]", t.IDField, strings.Join(tb.visible(t, mine), ", "))
		if t.Created == "" { // ordered by id, which is random: the rows, in any order
			result = fmt.Sprintf("List.length page.model.rows == %d", len(tb.visible(t, mine)))
		}
		d.Tests = append(d.Tests, TestData{Name: quote(name), Lines: tb.lines, Result: result})

		// 20 rows a page
		tb = newTest()
		tb.bulkInsert(t, 21, "")
		if mine { // another user's rows are not counted either
			tb.bulkInsert(t, 20, tb.otherUser())
		}
		total := len(tb.visible(t, mine)) + tb.bulk[t.Module]
		tb.let("let first <- Test.get (%s)", pageLit(o.Module, nil))
		tb.let("let second <- Test.get (%s)", pageLit(o.Module, []string{"page: 2"}))
		pages := (total + 19) / 20
		d.Tests = append(d.Tests, TestData{Name: quote("the list has 20 rows a page"), Lines: tb.lines,
			Result: fmt.Sprintf("{ first: List.length first.model.rows, second: List.length second.model.rows, pages: Test.texts \"al-pagination span\" first.doc } == { first: 20, second: %d, pages: [\"Previous\", \"Page 1 of %d\"] }", min(20, total-20), pages)})
	case o.Show != "":
		t, err := table(o.Show)
		if err != nil {
			return nil, err
		}
		imports.add(t.Module)
		tmpl = "page_show.tmpl"
		d.Title = Humanize(t.Name)
		heading = fmt.Sprintf("Test.text \"h1\" r.doc } == { answer: Shown, heading: Just %s }", quote(d.Title))
		d.Open, d.Close = rootTags(p, d.Title, imports)
		d.Path = strings.TrimSuffix(o.Path, "/") + "/{id}"
		importTypes(imports, t.IDType)
		d.Params = "{ id: " + types.StringIn(t.IDType, o.Module) + " }"
		d.T, d.Mine = recordName(t, o.Module), o.Requires != "" && t.Owner != ""
		for _, c := range t.Columns {
			d.Cols = append(d.Cols, ColData{Humanize(c.Name), cell(c, "model.row."+c.Name, imports)})
		}
		pageOf = func(tb *testBuilder) string { return pageLit(o.Module, []string{"id: " + tb.insert(t, true)}) }

		tb := newTest()
		row := tb.insert(t, true)
		tb.let("let r <- Test.open (%s)", pageLit(o.Module, []string{"id: " + row}))
		label := t.Label()
		result := fmt.Sprintf("{ answer: r.answer, fields: Test.count \"dd\" r.doc } == { answer: Shown, fields: %d }", len(t.Columns))
		if !label.ID {
			result = fmt.Sprintf("{ answer: r.answer, %s: List.member %s (Test.texts \"dd\" r.doc) } == { answer: Shown, %s: True }",
				label.Name, quote(rawText(label, tb.counts[t.Module])), label.Name)
		}
		d.Tests = append(d.Tests, TestData{Name: quote("the page shows the row"), Lines: tb.lines, Result: result})

		tb = newTest()
		missing := tb.brandCtor(t.IDType.(*types.Brand)) + " " + quote("missing")
		tb.let("let r <- Test.open (%s)", pageLit(o.Module, []string{"id: " + missing}))
		d.Tests = append(d.Tests, TestData{Name: quote("a missing row is not found"), Lines: tb.lines, Result: "r.answer == PageMissing"})

		if d.Mine { // read with Db.getMine: another user's row is not found either
			tb = newTest()
			theirs := tb.insertRow(t, "", map[string]string{t.Owner: tb.otherUser()}, false)
			tb.let("let r <- Test.open (%s)", pageLit(o.Module, []string{"id: " + theirs}))
			d.Tests = append(d.Tests, TestData{Name: quote("another user's row is not found"), Lines: tb.lines, Result: "r.answer == PageMissing"})
		}
	case len(o.From) > 0:
		tmpl = "page_from.tmpl"
		seen := map[string]bool{}
		var tables []*Table
		for _, name := range o.From {
			t, err := table(name)
			if err != nil {
				return nil, err
			}
			if seen[name] {
				return nil, Refusef("`--from %s` is given twice", name)
			}
			seen[name] = true
			tables = append(tables, t)
			imports.add(t.Module)
			label := t.Label()
			d.Lists = append(d.Lists, FromList{Field: Plural(LowerFirst(t.Name)), T: recordName(t, o.Module), Select: selectOf(t, o.Module, o.Requires != ""),
				Title: Humanize(Plural(LowerFirst(t.Name))), Cell: cell(label, "row."+label.Name, imports)})
		}
		pageOf = func(*testBuilder) string { return pageLit(o.Module, nil) }

		// each table's rows (read with Db.mine: mine, not another user's)
		tb := newTest()
		lhs, rhs := []string{"heading: Test.text \"h1\" page.doc"}, []string{"heading: Just " + quote(d.Title)}
		for i, t := range tables {
			field := d.Lists[i].Field
			tb.insert(t, true)
			if o.Requires != "" && t.Owner != "" {
				tb.insertRow(t, "", map[string]string{t.Owner: tb.otherUser()}, false)
			}
			lhs = append(lhs, fmt.Sprintf("%s: Test.count \"#%s li\" page.doc", field, field))
			rhs = append(rhs, fmt.Sprintf("%s: %d", field, len(tb.visible(t, o.Requires != "" && t.Owner != ""))))
		}
		tb.let("let page <- Test.get (%s)", pageLit(o.Module, nil))
		d.Tests = append(d.Tests, TestData{Name: quote("the page shows each table's rows"), Lines: tb.lines,
			Result: "{ " + strings.Join(lhs, ", ") + " } == { " + strings.Join(rhs, ", ") + " }"})
	default:
		pageOf = func(*testBuilder) string { return pageLit(o.Module, nil) }
		tb := newTest()
		tb.let("let r <- Test.open (%s)", pageLit(o.Module, nil))
		d.Tests = append(d.Tests, TestData{Name: quote("the page is shown"), Lines: tb.lines, Result: "{ answer: r.answer, heading: " + heading})
	}

	// who may not see the page: a visitor is sent to log in; a user of another role gets a 404
	if o.Requires != "" && p.Login != "" {
		tb := newTestBuilder(p, o.Module, imports)
		tb.let("let r <- Test.open (%s)", pageOf(tb))
		d.Tests = append(d.Tests, TestData{Name: quote("a visitor is sent to log in"), Lines: tb.lines, Result: "r.answer == Redirected (" + pageLit(p.Login, nil) + ")"})
	}
	if o.Role != "" {
		if other := otherRole(p, o.Role); other != "" {
			tb := newTestBuilder(p, o.Module, imports)
			tb.login(other)
			tb.let("let r <- Test.open (%s)", pageOf(tb))
			d.Tests = append(d.Tests, TestData{Name: quote("a user who is not " + o.Role + " gets no page"), Lines: tb.lines, Result: "r.answer == PageMissing"})
		}
	}
	d.Imports = imports.list()
	return []Edit{{Path: file, Create: true, Body: render(tmpl, d)}}, nil
}

// otherRole is a constructor of the session's role other than role, "" when there is none.
func otherRole(p *Program, role string) string {
	for _, c := range p.Session.Role.Ctors {
		if c != role {
			return c
		}
	}
	return ""
}

// missingSession says what the session lacks for a role: "no `user` field", "no `role` field".
func missingSession(p *Program) string {
	switch {
	case p.Session == nil || p.Session.User == nil:
		return "no `user` field"
	default:
		return "no `role` field whose type is a union"
	}
}

// rawText is the text a sample value of column c shows on a page (testBuilder.sample's).
func rawText(c *Column, n int) string {
	text := strings.ToLower(c.Name) + "-" + fmt.Sprint(n)
	if c.Brand != nil {
		switch c.Brand.Module + "." + c.Brand.Name {
		case "Text.Email":
			return text + "@example.com"
		case "Text.Url":
			return "https://example.com/" + text
		}
	}
	return text
}

// rootTags are the view's root element: the app's layout, `<Layout.Page title="…">`, when
// module Layout has a component Page, else a bare `<al-page>`.
func rootTags(p *Program, title string, imports *importSet) (string, string) {
	if p.Layout {
		imports.add("Layout")
		return "<Layout.Page title=" + quote(title) + ">", "</Layout.Page>"
	}
	return "<al-page>", "</al-page>"
}

// recordName is table t's record type as written in module home: its name, there or in any
// module importing its module, which is named after it.
func recordName(t *Table, home string) string { return t.Name }

// selectOf is how a page reads t's rows: `Db.mine T` on a `requires user` page when t has an
// `@owner` (chapter 6, "Rules"), else `Db.from T`.
func selectOf(t *Table, home string, requiresUser bool) string {
	if requiresUser && t.Owner != "" {
		return "Db.mine " + recordName(t, home)
	}
	return "Db.from " + recordName(t, home)
}

// cell is the markup showing column c's value expr: Bool and unions as an `al-badge`, a Time
// or Date formatted, a number as text, a Maybe as its value or nothing.
func cell(c *Column, expr string, imports *importSet) string {
	if c.Union != nil {
		imports.add(c.Union.Module)
	}
	switch {
	case c.Maybe:
		return "{match " + expr + " { Just v -> " + textOf(c, "v") + ", Nothing -> \"\" }}"
	case c.Kind == KindBool:
		return "{if " + expr + " then <al-badge>Yes</al-badge> else <al-badge variant=\"secondary\">No</al-badge>}"
	case c.Kind == KindUnion:
		return "<al-badge>{" + textOf(c, expr) + "}</al-badge>"
	}
	return "{" + textOf(c, expr) + "}"
}

// textOf is a Text expression for the (non-Maybe) value expr of column c's type.
func textOf(c *Column, expr string) string {
	switch c.Kind {
	case KindInt:
		return "Text.fromInt " + expr
	case KindFloat:
		return "Text.fromFloat " + expr
	case KindDate:
		return "Date.format \"Jan 2, 2006\" " + expr
	case KindTime:
		return "Time.format \"Jan 2, 2006\" " + expr
	case KindBool:
		return "if " + expr + " then \"Yes\" else \"No\""
	case KindUnion:
		var arms []string
		for _, ct := range c.Ctors {
			arms = append(arms, ct+" -> "+quote(ct))
		}
		return "match " + expr + " { " + strings.Join(arms, ", ") + " }"
	}
	return expr
}

// tableNames lists the app's tables for a message.
func tableNames(p *Program) string {
	if len(p.Tables) == 0 {
		return "none"
	}
	var names []string
	for _, t := range p.Tables {
		names = append(names, t.Name)
	}
	return strings.Join(names, ", ")
}

func contains1(xs []string, x string) bool {
	for _, y := range xs {
		if y == x {
			return true
		}
	}
	return false
}
