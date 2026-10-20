package generate

import (
	"cmp"
	"embed"
	"fmt"
	"regexp"
	"slices"
	"strings"
	"text/template"

	"alder/internal/check"
	"alder/internal/syntax"
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
	Mine   bool      // --show reads with Db.getMine (or Db.getScoped: Get)
	Get    string    // Db.getMine or Db.getScoped
	Cols   []ColData // the columns shown
	Lists  []FromList
	// LoadHole: `load` is the typed hole ?load (a page with path parameters, whose load reads
	// what they name: the author's)
	LoadHole bool
	// ContentType is a file page's content type (--file --type).
	ContentType string
	// By is --show's --by column: the row is read by it (`Db.where (T.slug.eq params.slug)`)
	// rather than by its id
	By string
	// Example: page.tmpl renders the --example page instead of the empty one
	Example bool
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
	Requires string   // --requires user (or a key, `org`, or both: `user,org`)
	Role     string   // --role R
	// Blank: `Model` is `{ }` and `load` and `view` are the typed holes `?load` and `?view`,
	// for `alder fill` (or `alder fill --decl`, which also replaces `Model`); with --show T the
	// page keeps --show's path and parameter (`/{id}`, the row's id), and nothing else of it.
	Blank bool
	// Example (--example): the plain page carries a `count: Int = 0` parameter, `msg Bump { by: Int }`,
	// an update arm redirecting to itself with the new count and a view showing it, every part
	// marked `-- example:` so the reader deletes it once understood. Off on the CLI; the editor's
	// form starts with it on (Field.Default).
	Example bool
	// Params are --param's `name:Type`, one per `{name}` segment of Path (`/posts/{slug}
	// --param slug:Post.Slug`): the page's parameter record, in the path's order; `load` is
	// then the hole ?load (with --blank, as always).
	Params []string
	// By is --show T --by <column>: the row is looked up by that `@unique` column of T, whose
	// value the path's last segment `{<column>}` carries, instead of by its id.
	By string
	// File (--file): a file page, whose `view` returns `File` (File.text with a typed hole for
	// the body) instead of HTML; ContentType (--type) is the file's, text/plain when empty.
	File        bool
	ContentType string
}

// pathParam matches a `{name}` segment of a page path.
var pathParam = regexp.MustCompile(`\{([^/{}]*)\}`)

// NewPage generates `alder new page`: the module's file, with one browser test (two for
// --show). A path with parameters takes a --param for each (`/tags/{name} --param name:Text`),
// and its `load` is the hole ?load; --show T --by <column> reads the row by a `@unique`
// column, the path's last segment. It refuses an existing module, a path parameter without its
// --param (and a --param without its segment), a table that is not one of the app's, a --by
// column that is not unique, and a role the session does not have; a path another page has is
// the checker's E0016, surfaced by Apply.
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
	if o.File && (o.List != "" || o.Show != "" || len(o.From) > 0 || o.Blank || o.Example) {
		return nil, Refusef("--file makes a file page; --list and --show make HTML pages")
	}
	if o.ContentType != "" && !o.File {
		return nil, Refusef("--type goes with --file")
	}
	if o.By != "" && o.Show == "" {
		return nil, Refusef("--by names the column --show T reads its row by: give --show T too")
	}
	if o.Show != "" {
		last := "/{id}"
		if o.By != "" {
			last = "/{" + o.By + "}"
		}
		o.Path = strings.TrimSuffix(o.Path, last) // `--show T` adds it; given, it is the same page
		if o.Path == "" {
			o.Path = "/"
		}
	}
	var params []FieldSpec
	if len(o.Params) > 0 {
		if o.Show != "" || o.List != "" || len(o.From) > 0 {
			return nil, Refusef("--param goes with a page of its own or a --blank one: a --list, --show or --from page has its parameters made for it")
		}
		names := map[string]bool{}
		for _, a := range o.Params {
			f, err := ParseField(a, false)
			if err != nil {
				return nil, Refusef("--param %v", err)
			}
			if f.Default != "" {
				return nil, Refusef("--param %s: a path parameter has no default (it is always in the URL); a query parameter with one is `alder edit page %s add-param %s`", a, o.Module, a)
			}
			if names[f.Name] {
				return nil, Refusef("--param %s is given twice", f.Name)
			}
			names[f.Name] = true
			params = append(params, f)
		}
		var inPath []string
		for _, m := range pathParam.FindAllStringSubmatch(o.Path, -1) {
			inPath = append(inPath, m[1])
			if !names[m[1]] {
				return nil, Refusef("`%s` has the parameter `{%s}` but no `--param %s:<Type>`", o.Path, m[1], m[1])
			}
			delete(names, m[1])
		}
		for n := range names {
			return nil, Refusef("--param %s: `%s` has no `{%s}` segment (a query parameter is `alder edit page %s add-param`)", n, o.Path, n, o.Module)
		}
		// the record in the path's order
		ordered := make([]FieldSpec, 0, len(params))
		for _, n := range inPath {
			for _, f := range params {
				if f.Name == n {
					ordered = append(ordered, f)
				}
			}
		}
		params = ordered
	} else if strings.ContainsAny(o.Path, "{}:") {
		return nil, Refusef("`%s` has a parameter: name its type with `--param <name>:<Type>` (`/posts/{slug} --param slug:Post.Slug`), or give the path without it (`--show T` adds `/{id}`)", o.Path)
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
	if o.Example && (o.Blank || kinds > 0 || len(o.Params) > 0) {
		return nil, Refusef("--example goes with a plain page: a --list, --show or --from page already shows `load` reading a table, and --blank and --param leave `load` to you")
	}
	if o.Blank && (o.List != "" || len(o.From) > 0) {
		return nil, Refusef("--blank takes no --list or --from: a blank page's `load` and `view` are holes (with --show T it has the row's `/{id}` parameter)")
	}
	if kinds > 1 {
		return nil, Refusef("choose one of --list, --show and --from: a page generated with --list shows one table's rows, --show one row, --from several tables")
	}
	// --requires: `user`, a key of the session (`org`, M15), or both; a key with `via` needs the
	// user first (E0027), so it is written `requires user, org`
	var keys []string
	key := "" // the page's key other than user, "" for none
	for _, k := range strings.FieldsFunc(o.Requires, func(r rune) bool { return r == ',' || r == ' ' }) {
		if !slices.Contains(keys, k) {
			keys = append(keys, k)
		}
		if k != "user" && k != key {
			if key != "" {
				return nil, Refusef("`--requires %s`: a generated page requires at most one key besides `user`", o.Requires)
			}
			key = k
		}
	}
	if o.Role != "" {
		if p.Session == nil || p.Session.User == nil || p.Session.Role == nil {
			return nil, Refusef("`--role %s` needs a session with a `user` field and a `role` field whose type is a union (`session { user: Maybe User.UserId, role: User.Role = Reader }` in `App`): this app's session has %s", o.Role, missingSession(p))
		}
		if !slices.Contains(keys, "user") {
			keys = append([]string{"user"}, keys...)
		}
	}
	if key != "" {
		f := p.SessionField(key)
		if f == nil {
			return nil, Refusef("`--requires %s`: the session has no field `%s` (a key is a session field: `alder edit session add-field %s:<T>.<T>Id via <Membership>`)", key, key, key)
		}
		if _, isMaybe := unMaybe(f.Type); !isMaybe {
			return nil, Refusef("`--requires %s`: the session's `%s` is not a `Maybe`, so every session has one", key, key)
		}
		if p.Via[key] != "" && !slices.Contains(keys, "user") { // a membership is the user's
			keys = append([]string{"user"}, keys...)
		}
	}
	if slices.Contains(keys, "user") {
		if p.Session == nil || p.Session.User == nil {
			return nil, Refusef("`--requires user` needs a session with a `user` field (`session { user: Maybe User.UserId }` in `App`): this app's session has none")
		}
		keys = append([]string{"user"}, slices.DeleteFunc(slices.Clone(keys), func(k string) bool { return k == "user" })...)
	}
	o.Requires = strings.Join(keys, ", ")
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
	if o.Example {
		d.Example = true
		d.Params = "{ count: Int = 0 }"
	}
	if o.Requires != "" {
		d.Requires = " requires " + o.Requires
		if o.Role != "" {
			d.Requires += ", role " + o.Role
		}
	}
	var kt *Table // the key's table: a test signs in with one of its rows as the key
	if key != "" {
		kt = p.KeyTable(key)
	}
	d.Open, d.Close = rootTags(p, d.Title, imports)
	var paramPage *Page // the page with --param's parameters, checked: their types
	if len(params) > 0 {
		var fs []string
		for _, f := range params {
			fs = append(fs, f.Source())
			typeImports(imports, f.Type, p)
		}
		d.Params = "{ " + strings.Join(fs, ", ") + " }"
		d.LoadHole = true
		src := "module " + o.Module + "\n\n"
		for _, i := range imports.list() {
			src += "import " + i + "\n"
		}
		src += "\npage \"" + o.Path + "\" " + d.Params + "\n\ntype Model = { }\n\nload : Params -> Session -> Query Model\nload params session = Query.succeed { }\n\n" +
			"view : Model -> Session -> Html Msg\nview model session = <p></p>\n"
		probe, err := Probe(p.Dir, []Edit{{Path: file, Create: true, Body: src}})
		if err != nil {
			return nil, err
		}
		if r := probe.errorsIn(file); r != nil {
			return nil, r
		}
		if paramPage = probe.PageNamed(o.Module); paramPage == nil {
			return nil, Refusef("internal error: the page %s was not checked", o.Module)
		}
	}
	// newTest starts a test's builder, signed in as the page requires
	newTest := func() *testBuilder {
		tb := newTestBuilder(p, o.Module, imports)
		if kt != nil { // signed in with the key: a row of its table (and, with `via`, a member of it)
			tb.silo = &siloCtx{key: key, table: kt, member: true}
		}
		if o.Requires != "" {
			tb.login(o.Role)
		}
		return tb
	}
	scoped := func(t *Table) bool { return t.PartKey != "" && slices.Contains(keys, t.PartKey) }
	heading := fmt.Sprintf("Test.text \"h1\" r.doc } == { answer: Shown, heading: Just %s }", quote(d.Title))
	tmpl := "page.tmpl"
	var pageOf func(tb *testBuilder) string // the page value a test opens
	switch {
	case o.File:
		tmpl = "page_file.tmpl"
		d.ContentType = quote(cmp.Or(o.ContentType, "text/plain"))
		delete(imports.set, "Layout") // the view is a File: no layout
		pageOf = func(*testBuilder) string { return pageLit(o.Module, nil) }
		if paramPage != nil {
			pageOf = func(tb *testBuilder) string { return tb.pageValue(paramPage) }
		}
		tb := newTest()
		tb.let("let r <- Test.fetch (%s)", pageOf(tb))
		d.Tests = append(d.Tests, TestData{Name: quote("the file is served"), Lines: tb.lines, Result: "r.status == 200"})
	case o.Blank:
		tmpl = "page_blank.tmpl"
		delete(imports.set, "Layout") // the view is a hole: whatever fills it imports what it uses
		pageOf = func(*testBuilder) string { return pageLit(o.Module, nil) }
		if paramPage != nil {
			pageOf = func(tb *testBuilder) string { return tb.pageValue(paramPage) }
		}
		if o.Show != "" {
			t, err := table(o.Show)
			if err != nil {
				return nil, err
			}
			d.Path = strings.TrimSuffix(o.Path, "/") + "/{id}"
			importTypes(imports, t.IDType)
			d.Params = "{ id: " + types.StringIn(t.IDType, o.Module) + " }"
			pageOf = func(tb *testBuilder) string { return pageLit(o.Module, []string{"id: " + tb.insert(t, true)}) }
			if o.By != "" {
				by := t.Column(o.By)
				if by == nil || !by.Unique || by.Maybe || by.ID {
					return nil, Refusef("`--by %s`: `%s` is not a `@unique` column of `%s` (a row is looked up by one; its unique columns: %s)", o.By, o.By, t.Name, uniqueColumns(t))
				}
				importTypes(imports, by.Type)
				d.Path = strings.TrimSuffix(o.Path, "/") + "/{" + o.By + "}"
				d.Params = "{ " + o.By + ": " + by.TypeIn(o.Module) + " }"
				pageOf = func(tb *testBuilder) string {
					tb.insert(t, true)
					return pageLit(o.Module, []string{o.By + ": " + tb.sample(by.Type, by.Name, tb.counts[t.Module])})
				}
			}
		}
	case o.List != "":
		t, err := table(o.List)
		if err != nil {
			return nil, err
		}
		imports.add(t.Module)
		tmpl = "page_list.tmpl"
		d.Params = "{ page: Int = 1 }"
		d.T, d.Order = recordName(t, o.Module), t.OrderColumn()
		d.Select = selectIn(t, o.Module, keys)
		for _, c := range t.Columns {
			if !c.ID {
				d.Cols = append(d.Cols, ColData{Humanize(c.Name), cell(c, "row."+c.Name, imports)})
			}
		}
		mine := scoped(t)
		pageOf = func(*testBuilder) string { return pageLit(o.Module, nil) }

		// the rows, newest first (and, read with Db.mine, not another user's; with Db.scoped,
		// not another org's)
		tb := newTest()
		if mine && t.PartKey != "user" && kt != nil { // another org's row first: a test inserts it before signing in with one
			tb = newTestBuilder(p, o.Module, imports)
			tb.silo = &siloCtx{key: key, table: kt, member: true}
			tb.keyRow()
			other := tb.insertRow(kt, "other", nil, false)
			tb.inOtherKey(other, func() { tb.insertRow(t, "_", map[string]string{t.PartCol: other}, false) })
			tb.login(o.Role)
		}
		tb.insert(t, true)
		tb.insert(t, true)
		name := "the list shows the rows, newest first"
		switch {
		case mine && t.PartKey == "user":
			tb.insertRow(t, "", map[string]string{t.Owner: tb.otherUser()}, false)
			name = "the list shows my rows, newest first"
		case mine:
			name = "the list shows this " + key + "'s rows, newest first"
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
		if mine && t.PartKey == "user" { // another user's rows are not counted either
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
		d.T, d.Mine = recordName(t, o.Module), scoped(t)
		d.Get = "Db.getMine"
		if d.Mine && t.PartKey != "user" {
			d.Get = "Db.getScoped"
		}
		for _, c := range t.Columns {
			d.Cols = append(d.Cols, ColData{Humanize(c.Name), cell(c, "model.row."+c.Name, imports)})
		}
		// the page value of the row a test inserted last: by its id, or by its --by column
		param := func(tb *testBuilder, row string) string { return "id: " + row }
		var by *Column
		if o.By != "" {
			if by = t.Column(o.By); by == nil || !by.Unique || by.Maybe || by.ID {
				return nil, Refusef("`--by %s`: `%s` is not a `@unique` column of `%s` (a row is looked up by one; its unique columns: %s)", o.By, o.By, t.Name, uniqueColumns(t))
			}
			importTypes(imports, by.Type)
			d.By, d.Select = o.By, selectIn(t, o.Module, keys)
			d.Path = strings.TrimSuffix(o.Path, "/") + "/{" + o.By + "}"
			d.Params = "{ " + o.By + ": " + by.TypeIn(o.Module) + " }"
			param = func(tb *testBuilder, row string) string {
				return o.By + ": " + tb.sample(by.Type, by.Name, tb.counts[t.Module])
			}
		}
		pageOf = func(tb *testBuilder) string { return pageLit(o.Module, []string{param(tb, tb.insert(t, true))}) }

		tb := newTest()
		row := tb.insert(t, by == nil) // looked up by its --by column, the row needs no name
		tb.let("let r <- Test.open (%s)", pageLit(o.Module, []string{param(tb, row)}))
		label := t.Label()
		result := fmt.Sprintf("{ answer: r.answer, fields: Test.count \"dd\" r.doc } == { answer: Shown, fields: %d }", len(t.Columns))
		if !label.ID {
			result = fmt.Sprintf("{ answer: r.answer, %s: List.member %s (Test.texts \"dd\" r.doc) } == { answer: Shown, %s: True }",
				label.Name, quote(rawText(label, tb.counts[t.Module])), label.Name)
		}
		d.Tests = append(d.Tests, TestData{Name: quote("the page shows the row"), Lines: tb.lines, Result: result})

		tb = newTest()
		missing := "id: " + tb.brandCtor(t.IDType.(*types.Brand)) + " " + quote("missing")
		if by != nil {
			missing = o.By + ": " + tb.sample(by.Type, by.Name, 999)
		}
		tb.let("let r <- Test.open (%s)", pageLit(o.Module, []string{missing}))
		d.Tests = append(d.Tests, TestData{Name: quote("a missing row is not found"), Lines: tb.lines, Result: "r.answer == PageMissing"})

		if d.Mine && t.PartKey == "user" { // read with Db.getMine: another user's row is not found either
			tb = newTest()
			theirs := tb.insertRow(t, "", map[string]string{t.Owner: tb.otherUser()}, false)
			tb.let("let r <- Test.open (%s)", pageLit(o.Module, []string{param(tb, theirs)}))
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
			d.Lists = append(d.Lists, FromList{Field: Plural(LowerFirst(t.Name)), T: recordName(t, o.Module), Select: selectIn(t, o.Module, keys),
				Title: Humanize(Plural(LowerFirst(t.Name))), Cell: cell(label, "row."+label.Name, imports)})
		}
		pageOf = func(*testBuilder) string { return pageLit(o.Module, nil) }

		// each table's rows (read with Db.mine: mine, not another user's)
		tb := newTest()
		lhs, rhs := []string{"heading: Test.text \"h1\" page.doc"}, []string{"heading: Just " + quote(d.Title)}
		for i, t := range tables {
			field := d.Lists[i].Field
			tb.insert(t, true)
			if scoped(t) && t.PartKey == "user" {
				tb.insertRow(t, "", map[string]string{t.Owner: tb.otherUser()}, false)
			}
			lhs = append(lhs, fmt.Sprintf("%s: Test.count \"#%s li\" page.doc", field, field))
			rhs = append(rhs, fmt.Sprintf("%s: %d", field, len(tb.visible(t, scoped(t)))))
		}
		tb.let("let page <- Test.get (%s)", pageLit(o.Module, nil))
		d.Tests = append(d.Tests, TestData{Name: quote("the page shows each table's rows"), Lines: tb.lines,
			Result: "{ " + strings.Join(lhs, ", ") + " } == { " + strings.Join(rhs, ", ") + " }"})
	default:
		pageOf = func(*testBuilder) string { return pageLit(o.Module, nil) }
		if paramPage != nil {
			pageOf = func(tb *testBuilder) string { return tb.pageValue(paramPage) }
		}
		tb := newTest()
		tb.let("let r <- Test.open (%s)", pageOf(tb))
		d.Tests = append(d.Tests, TestData{Name: quote("the page is shown"), Lines: tb.lines, Result: "{ answer: r.answer, heading: " + heading})
		if o.Example { // the example's loop: press Bump, the redirect carries the new count
			tb = newTest()
			tb.let("let _ <- Test.get (%s)", pageOf(tb))
			tb.let("let r <- Test.click (Bump { by: 1 })")
			tb.let("let after <- Test.open (%s { count: 1 })", o.Module)
			d.Tests = append(d.Tests, TestData{Name: quote("pressing Bump counts one"), Lines: tb.lines,
				Result:  "{ answer: r.answer, count: Test.text \"#count\" after.doc } == { answer: Redirected (" + o.Module + " { count: 1 }), count: Just \"Count: 1\" }",
				Comment: []string{"example: delete this test with the rest of the example."}})
		}
	}

	// who may not see the page: a visitor is sent to log in; a user of another role gets a 404
	if o.Requires != "" && p.Login != "" {
		tb := newTestBuilder(p, o.Module, imports)
		tb.let("let r <- Test.open (%s)", pageOf(tb))
		d.Tests = append(d.Tests, TestData{Name: quote("a visitor is sent to log in"), Lines: tb.lines, Result: "r.answer == Redirected (" + pageLit(p.Login, nil) + ")"})
	}
	if choose := p.Choose[key]; key != "" && choose != "" && slices.Contains(keys, "user") { // signed in without the key: its picker
		tb := newTestBuilder(p, o.Module, imports)
		tb.login(o.Role)
		tb.let("let r <- Test.open (%s)", pageOf(tb))
		d.Tests = append(d.Tests, TestData{Name: quote("without " + article(key) + " the page sends you to choose one"), Lines: tb.lines, Result: "r.answer == Redirected (" + pageLit(choose, nil) + ")"})
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

// isFilePage reports whether m's `view` returns `File`: the signature walk of the checker's
// viewReturnsFile (unexported), repeated here.
func isFilePage(m *syntax.Module) bool {
	v := valueDecl(m, "view")
	if v == nil || v.Sig == nil {
		return false
	}
	t := v.Sig
	for {
		ft, ok := t.(*syntax.FunType)
		if !ok {
			break
		}
		t = ft.Result
	}
	tn, ok := t.(*syntax.TypeName)
	return ok && tn.Name == "File" && (tn.Module == "" || tn.Module == "Prelude") && len(tn.Args) == 0
}

// uniqueColumns lists t's `@unique` columns for a message, "none" when it has none.
func uniqueColumns(t *Table) string {
	var out []string
	for _, c := range t.Columns {
		if c.Unique && !c.ID {
			out = append(out, c.Name)
		}
	}
	if len(out) == 0 {
		return "none"
	}
	return strings.Join(out, ", ")
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

// selectIn is how a page that requires keys reads t's rows: `Db.mine T` for a table partitioned
// by `user`, `Db.scoped T` for one partitioned by another of them (chapter 6, "Partitions"),
// else `Db.from T`.
func selectIn(t *Table, home string, keys []string) string {
	switch {
	case t.PartKey == "" || !slices.Contains(keys, t.PartKey):
		return "Db.from " + recordName(t, home)
	case t.PartKey == "user":
		return "Db.mine " + recordName(t, home)
	}
	return "Db.scoped " + recordName(t, home)
}

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
