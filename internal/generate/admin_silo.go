package generate

import (
	"fmt"
	"regexp"
	"slices"
	"strings"

	"alder/internal/types"
)

// The admin of the tables in a `silo` (M15, book chapter 15): each value of the silo's key has a
// database of its own, so there is no page of every org's projects. The admin has a picker
// instead (`Admin.Orgs.Pick`, `/admin/orgs`: the key's table, a link per row) and a dashboard
// per value (`Admin.Orgs.Dashboard`, `/admin/orgs/{org}`: each siloed table's count and newest rows,
// read when it is opened, not for every org at once); the siloed tables' pages are the usual
// ones, under the dashboard at `/admin/orgs/{org}/project…`, declared `requires user, role Admin, silo org` with the
// key in the page's parameter `org`. The role check guards them, not `via`: an Admin need not be
// a member of the org (chapter 6, "Partitions"). Every page of one org reads and writes that
// org's database only, so a bulk action stays inside one silo. A table partitioned by a key but
// in no silo keeps the ordinary (unscoped) pages.

// adminSilo is a `silo key { … }` as the admin shows it.
type adminSilo struct {
	key    string
	kt     *Table        // the key's table (Org)
	tables []*adminTable // its tables, in the admin's order
	picker string        // the picker's module: "Admin.Orgs.Pick"
	dash   string        // the dashboard of one value: "Admin.Orgs.Dashboard"
	path   string        // the picker's path: "/admin/orgs"
	title  string        // its tables in a sentence: "Projects and tasks"
	mods   map[string]bool
}

// silos are the app's silos whose key is a table's id, by key, sorted; each siloed adminTable
// gets its silo and its path under the key.
func (g *adminGen) silos() {
	var keys []string
	for k := range g.p.Silos {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	for _, k := range keys {
		kt := g.p.KeyTable(k)
		if kt == nil {
			continue
		}
		mod := g.o.Module + "." + UpperFirst(Plural(k))
		s := &adminSilo{key: k, kt: kt, picker: mod + ".Pick", dash: mod + ".Dashboard", path: g.o.Path + "/" + Plural(k), mods: map[string]bool{}}
		var titles []string
		for _, at := range g.tables {
			if at.t.Silo != k {
				continue
			}
			at.silo = s
			at.path = s.path + "/{" + k + "}/" + strings.ToLower(at.t.Name) // under the dashboard: /admin/orgs/{org}/project
			s.tables = append(s.tables, at)
			for _, kind := range []string{"Index", "Show", "New", "Edit", "Delete", "Bulk"} {
				s.mods[at.page(kind)] = true
			}
			titles = append(titles, Plural(at.noun))
		}
		if len(s.tables) == 0 {
			continue
		}
		s.title = UpperFirst(joinAnd(titles))
		g.silo = append(g.silo, s)
	}
}

// joinAnd is words in a sentence: "a", "a and b", "a, b and c".
func joinAnd(words []string) string {
	if len(words) < 2 {
		return strings.Join(words, "")
	}
	return strings.Join(words[:len(words)-1], ", ") + " and " + words[len(words)-1]
}

// siloOf is the silo whose table's page module mod is, nil for another module.
func (g *adminGen) siloOf(mod string) *adminSilo {
	for _, s := range g.silo {
		if s.mods[mod] {
			return s
		}
	}
	return nil
}

// keyType is the type of the silo's key as written in module mod (`Org.OrgId`), imported.
func (s *adminSilo) keyType(mod string, imports *importSet) string {
	importTypes(imports, s.kt.IDType)
	return types.StringIn(s.kt.IDType, mod)
}

// params are a page's parameters: the silo's key first, for a siloed table's page.
func (at *adminTable) params(mod string, imports *importSet, params []string) []string {
	if at.silo == nil {
		return params
	}
	return append([]string{at.silo.key + ": " + at.silo.keyType(mod, imports)}, params...)
}

// requires is the `page` line's clause of the table's pages: the role, and for a siloed table the
// key from the parameter.
func (at *adminTable) requires() string {
	if at.silo == nil {
		return adminRequires
	}
	return adminRequires + ", silo " + at.silo.key
}

// action is a GET form's `action` attribute to the table's path with suffix: the text, or for a
// siloed table the text built around the key's value (a form's action is text, not a page value).
func (at *adminTable) action(suffix string) string {
	if at.silo == nil {
		return quote(at.path + suffix)
	}
	before, after, _ := strings.Cut(at.path+suffix, "{"+at.silo.key+"}")
	return `{"` + before + `" ++ session.` + at.silo.key + ` ++ "` + after + `"}`
}

// siloLits adds the key, expr, to every page value of a table in silo s in text: `Admin.Projects.Show
// { id: x }` becomes `Admin.Projects.Show { org: expr, id: x }` (one that names the key is left).
// The key's own pages, the picker `Admin.Orgs.Pick` and the dashboard `Admin.Orgs.Dashboard`,
// are not a table's and are left as written.
func (s *adminSilo) siloLits(text, expr string) string {
	var alts []string
	for m := range s.mods {
		alts = append(alts, regexp.QuoteMeta(m))
	}
	slices.Sort(alts)
	re := regexp.MustCompile(`(^|[^A-Za-z0-9_.])(` + strings.Join(alts, "|") + `) \{`)
	var b strings.Builder
	last := 0
	for _, m := range re.FindAllStringSubmatchIndex(text, -1) {
		end := m[1] // after the `{`
		b.WriteString(text[last:end])
		rest := text[end:]
		trimmed := strings.TrimLeft(rest, " ")
		switch {
		case strings.HasPrefix(trimmed, "}"):
			b.WriteString(" " + s.key + ": " + expr + " ")
			last = end + len(rest) - len(trimmed)
		case strings.HasPrefix(strings.TrimLeft(rest, " \n"), s.key+":"):
			last = end
		case strings.HasPrefix(rest, "\n"):
			nl := strings.Index(rest[1:], "\n")
			line := rest[1:]
			if nl >= 0 {
				line = rest[1 : nl+1]
			}
			ind := line[:len(line)-len(strings.TrimLeft(line, " "))]
			b.WriteString("\n" + ind + s.key + ": " + expr + ",")
			last = end
		default:
			b.WriteString(" " + s.key + ": " + expr + ", ")
			last = end + len(rest) - len(trimmed)
		}
	}
	b.WriteString(text[last:])
	return b.String()
}

// testsIn renders the generated tests of module mod (testsText): in a siloed table's module,
// every page value of the silo's tables takes the test's key row.
func (g *adminGen) testsIn(mod string, tests []TestData) string {
	if s := g.siloOf(mod); s != nil {
		for i := range tests {
			for j := range tests[i].Lines {
				tests[i].Lines[j] = s.siloLits(tests[i].Lines[j], s.key)
			}
			tests[i].Result = s.siloLits(tests[i].Result, s.key)
		}
	}
	return testsText(tests)
}

// siloCode is body (a file's code, before its tests) with the silo's page values taking the
// key from the narrowed session: the page's parameter.
func (g *adminGen) siloCode(mod, body string) string {
	s := g.siloOf(mod)
	if s == nil {
		return body
	}
	code, tests, ok := strings.Cut(body, RegionBegin+"tests\n")
	code = s.siloLits(code, "session."+s.key)
	if ok {
		return code + RegionBegin + "tests\n" + tests
	}
	return code
}

// tbIn starts a test's builder in module mod, as tb; in a siloed table's module (or the
// dashboard of one value), the key's row is inserted first, and an Admin is signed in as a
// member of it when member is set (the test's own reads go to its database), else with the key
// unset (only the pages, which take it from their parameter, read the silo).
func (g *adminGen) tbIn(mod string, imports *importSet, admin bool, s *adminSilo, member bool) *testBuilder {
	tb := newTestBuilder(g.p, mod, imports)
	if s != nil {
		tb.silo = &siloCtx{key: s.key, table: s.kt, member: member, admin: true}
		tb.keyRow()
	}
	if admin {
		tb.login("Admin")
	}
	return tb
}

// inOtherKey inserts, through f, rows under another row of the key's table (other): the rows
// they refer to in the key's silo are inserted again there, as a reference cannot cross
// databases. What the builder remembers of rows is restored after (its counts go on, so samples
// stay distinct).
func (tb *testBuilder) inOtherKey(other string, f func()) {
	ids, rows := map[string]string{}, map[string][]rowRef{}
	for k, v := range tb.ids {
		ids[k] = v
	}
	for k, v := range tb.rows {
		rows[k] = slices.Clone(v)
	}
	tb.ids[tb.silo.table.Module] = other
	for _, m := range tb.p.Silos[tb.silo.key] {
		delete(tb.ids, m)
	}
	f()
	tb.ids, tb.rows = ids, rows
}

// ---- the picker and the dashboard of one value

// picker is the page that lists the key's table, each row linking to its dashboard.
func (g *adminGen) picker(s *adminSilo) {
	mod := s.picker
	imports := newImports(mod, g.p.Std)
	kt := s.kt
	imports.add(kt.Module)
	noun := strings.ToLower(Humanize(kt.Name))
	title := "Choose " + article(noun)
	open, close := g.layoutOpen(title, imports)
	label := kt.Label()
	sel := "Db.from " + kt.Name + " |> Db.orderBy " + kt.Name + "." + label.Name + ".asc"
	var b strings.Builder
	fmt.Fprintf(&b, "page %s { }%s\n\n", quote(s.path), adminRequires)
	fmt.Fprintf(&b, "-- The %s: %s live in a database per %s (`silo %s`), so they are\n", Plural(noun), strings.ToLower(s.title), s.key, s.key)
	fmt.Fprintf(&b, "-- listed one %s at a time, from its dashboard.\n", s.key)
	fmt.Fprintf(&b, "type Model = { rows: List %s }\n\n", kt.Name)
	b.WriteString(g.load(regionAt("  ", "load", wrapAll([]string{"let rows <- " + sel + " |> Db.all", "Query.succeed { rows: rows }"}))))
	b.WriteString("view : Model -> Session -> Html Msg\nview model session =\n")
	fmt.Fprintf(&b, "  %s\n    <h1>%s</h1>\n\n", open, UpperFirst(title))
	fmt.Fprintf(&b, "    <p>%s live in a database per %s: choose one.</p>\n", s.title, s.key)
	fmt.Fprintf(&b, "    <ul id=%s>\n      {for row in model.rows -> <li><a href={%s { %s: row.%s }}>{%s}</a></li>}\n    </ul>\n",
		quote(Plural(s.key)), s.dash, s.key, kt.IDField, textOf(label, "row."+label.Name))
	fmt.Fprintf(&b, "  %s\n", close)

	var tests []TestData
	tb := g.tb(mod, imports, true)
	k := tb.counts[kt.Module]
	second := tb.insertRow(kt, "", map[string]string{label.Name: tb.sample(label.Type, label.Name, k+2)}, false)
	first := tb.insertRow(kt, "", map[string]string{label.Name: tb.sample(label.Type, label.Name, k+1)}, false)
	tb.let("let page <- Test.get (%s {})", mod)
	if label.ID {
		tests = append(tests, TestData{Name: quote("the picker lists every " + noun), Lines: tb.lines,
			Result: fmt.Sprintf("Test.count \"#%s li\" page.doc == %d", Plural(s.key), tb.counts[kt.Module])})
	} else {
		var want []string
		for _, r := range tb.rows[kt.Module] { // by label: the rows inserted before, then first, second
			if r.v != first && r.v != second {
				want = append(want, r.v)
			}
		}
		want = append(want, first, second)
		tests = append(tests, TestData{Name: quote("the picker lists every " + noun + ", by " + strings.ToLower(Humanize(label.Name)) + ", each opening its dashboard"), Lines: tb.lines,
			Result: fmt.Sprintf("{ rows: List.map (\\row -> row.%s) page.model.rows, first: Test.text \"#%s li\" page.doc } == { rows: [%s], first: Just %s }",
				kt.IDField, Plural(s.key), strings.Join(want, ", "), quote(rawText(label, k+1)))})
	}
	tests = append(tests, g.gateTests(mod, imports, func(*testBuilder) string { return mod + " {}" })...)
	b.WriteString(testsText(tests))
	g.file(mod, imports, b.String())
}

// siloDashboard is the dashboard of one value of the key: each siloed table's count and newest
// rows, read from that value's database when the page is opened.
func (g *adminGen) siloDashboard(s *adminSilo) {
	mod := s.dash
	imports := newImports(mod, g.p.Std)
	kt := s.kt
	imports.add(kt.Module)
	open, close := g.layoutOpen(UpperFirst(strings.ToLower(Humanize(kt.Name))), imports)
	keyT := s.keyType(mod, imports)
	var loads, cards []string
	for _, at := range s.tables {
		t := at.t
		imports.add(t.Module)
		sel := "Db.from " + t.Name
		if t.Created != "" {
			sel += " |> Db.orderBy " + t.Name + "." + t.Created + ".desc"
		}
		loads = append(loads,
			fmt.Sprintf("let %s <- %s |> Db.limit recent |> Db.all", at.plural, sel),
			fmt.Sprintf("let %sCount <- Db.from %s |> Db.count", LowerFirst(t.Name), t.Name))
		cards = append(cards, fmt.Sprintf("  { key: %s, title: %s, href: %s { %s: params.%s }, count: %sCount, rows: List.map (\\row -> { label: %s, href: %s { %s: params.%s, id: row.%s } }) %s },",
			quote(at.plural), quote(at.title), at.page("Index"), s.key, s.key, LowerFirst(t.Name), textOf(at.label, "row."+at.label.Name), at.page("Show"), s.key, s.key, t.IDField, at.plural))
	}
	label := kt.Label()
	body := []string{fmt.Sprintf("let found <- Db.get %s params.%s", kt.Name, s.key), "match found {", "  Just row -> {"}
	body = append(body, indentAll(loads, "    ")...)
	body = append(body, fmt.Sprintf("    Query.succeed { name: %s, cards: [", textOf(label, "row."+label.Name)))
	body = append(body, indentAll(cards, "    ")...)
	body = append(body, "    ] }", "  },", "  Nothing -> Query.notFound,", "}")

	var b strings.Builder
	fmt.Fprintf(&b, "page %s { %s: %s }%s, silo %s\n\n", pathLit(s.path+"/{"+s.key+"}"), s.key, keyT, adminRequires, s.key)
	fmt.Fprintf(&b, "-- The %s of the page's parameter: each table in `silo %s`, counted in its database. The role\n", strings.ToLower(Humanize(kt.Name)), s.key)
	fmt.Fprintf(&b, "-- check guards the page (`silo %s` takes the %s from the parameter): an Admin need not be a member.\n", s.key, s.key)
	b.WriteString("type Card = { key: Text, title: Text, href: Page, count: Int, rows: List { label: Text, href: Page } }\n\n")
	b.WriteString("type Model = { name: Text, cards: List Card }\n\n")
	b.WriteString("-- recent is how many of a table's newest rows its card lists.\nrecent : Int\nrecent = 5\n\n")
	b.WriteString(g.load(regionAt("  ", "cards", wrapAll(body))))
	b.WriteString("view : Model -> Session -> Html Msg\nview model session =\n")
	fmt.Fprintf(&b, "  %s\n    <h1>{model.name}</h1>\n\n", open)
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
	fmt.Fprintf(&b, "    <a href={%s {}}>Another %s</a>\n", s.picker, s.key)
	fmt.Fprintf(&b, "  %s\n", close)

	var tests []TestData
	pageOf := func(tb *testBuilder) string { return fmt.Sprintf("%s { %s: %s }", mod, s.key, tb.keyRow()) }
	for _, at := range s.tables {
		t := at.t
		tb := g.tbIn(mod, imports, true, s, false)
		before := tb.counts[t.Module]
		tb.bulkInsert(t, recentRows+1, "")
		total := len(tb.visible(t, false)) + tb.bulk[t.Module]
		other := tb.insertRow(kt, "other", nil, false)
		tb.inOtherKey(other, func() { tb.insertRow(t, "_", map[string]string{t.PartCol: other}, false) })
		tb.let("let page <- Test.get (%s)", pageOf(tb))
		lhs, rhs := g.cardChecks(tb, at, at.plural, before, total, func(row string) string {
			return fmt.Sprintf("%s { %s: %s, id: %s }", at.page("Show"), s.key, tb.keyRow(), row)
		})
		lhs = append([]string{"name: Test.text \"h1\" page.doc"}, lhs...)
		rhs = append([]string{"name: Just " + quote(rawText(label, 1))}, rhs...)
		if label.ID {
			lhs, rhs = lhs[1:], rhs[1:]
		}
		name := "the " + s.key + "'s dashboard counts its " + Plural(at.noun) + ", not another " + s.key + "'s, and lists the newest five"
		tests = append(tests, TestData{Name: quote(name), Lines: tb.lines, Result: "{ " + strings.Join(lhs, ", ") + " } == { " + strings.Join(rhs, ", ") + " }",
			Comment: []string{"The Admin is no member of the " + s.key + ": the page's parameter and the role check open its database."}})
	}
	tb := g.tbIn(mod, imports, true, s, false)
	missing := tb.brandCtor(kt.IDType.(*types.Brand)) + " " + quote("missing")
	tb.let("let r <- Test.open (%s { %s: %s })", mod, s.key, missing)
	tests = append(tests, TestData{Name: quote("a missing " + s.key + " is not found"), Lines: tb.lines, Result: "r.answer == PageMissing"})
	tests = append(tests, g.gateTests(mod, imports, pageOf)...)
	b.WriteString(testsText(tests))
	g.file(mod, imports, b.String())
}

// cardChecks are a dashboard test's comparisons of at's card (its count, total, and its newest
// rows, numbered after before) on page, the left and right sides' fields; show is the page
// value of a row's Show page.
func (g *adminGen) cardChecks(tb *testBuilder, at *adminTable, key string, before, total int, show func(row string) string) ([]string, []string) {
	t := at.t
	lhs := []string{fmt.Sprintf("count: Test.text \"#%s al-badge\" page.doc", key)}
	rhs := []string{fmt.Sprintf("count: Just %s", quote(fmt.Sprint(total)))}
	switch {
	case !at.label.ID && t.Created != "":
		var labels []string
		for n := before + recentRows + 1; n > before+1; n-- {
			labels = append(labels, quote(rawText(at.label, n)))
		}
		lhs = append(lhs, fmt.Sprintf("rows: Test.texts \"#%s li\" page.doc", key))
		rhs = append(rhs, "rows: ["+strings.Join(labels, ", ")+"]")
	case t.Created != "": // the label is the id: the newest rows, by the pages they link to
		tb.let("let newest <- Db.from %s |> Db.orderBy %s.%s.desc |> Db.limit %d |> Db.all", t.Name, t.Name, t.Created, recentRows)
		lhs = append(lhs, fmt.Sprintf("rows: List.map (\\card -> List.map (\\row -> row.href) card.rows) (List.filter (\\card -> card.key == %s) page.model.cards)", quote(key)))
		rhs = append(rhs, fmt.Sprintf("rows: [List.map (\\row -> %s) newest]", show("row."+t.IDField)))
	default:
		lhs = append(lhs, fmt.Sprintf("rows: Test.count \"#%s li\" page.doc", key))
		rhs = append(rhs, fmt.Sprintf("rows: %d", recentRows))
	}
	return lhs, rhs
}

// siloTests are the tests of a siloed table's list (and its bulk page) that the Admin need not be
// a member: signed in without the key, the page of one value lists that value's rows only, and a
// bulk action given another value's ids changes nothing there.
func (g *adminGen) siloListTest(at *adminTable, mod string, imports *importSet) TestData {
	s, t := at.silo, at.t
	tb := g.tbIn(mod, imports, true, s, false)
	mine := tb.insertRow(t, "", nil, false)
	other := tb.insertRow(s.kt, "other", nil, false)
	var theirs string
	tb.inOtherKey(other, func() { theirs = tb.insertRow(t, "", map[string]string{t.PartCol: other}, false) })
	tb.let("let page <- Test.get (%s { %s: %s })", mod, s.key, tb.keyRow())
	tb.let("let them <- Test.get (%s { %s: %s })", mod, s.key, other)
	return TestData{Name: quote("an Admin who is no member of the " + s.key + " lists its " + Plural(at.noun) + ", and only its"), Lines: tb.lines,
		Result:  fmt.Sprintf("{ mine: List.map (\\row -> row.%s) page.model.rows, theirs: List.map (\\row -> row.%s) them.model.rows } == { mine: [%s], theirs: [%s] }", t.IDField, t.IDField, mine, theirs),
		Comment: []string{"Each " + s.key + "'s rows live in its own database: the page's parameter and the role check open it."}}
}

// siloBulkTest is the test that the bulk page of one value acts on that value's rows only:
// another value's ids are not found there, and its rows are left.
func (g *adminGen) siloBulkTest(at *adminTable, mod string, imports *importSet) TestData {
	s, t := at.silo, at.t
	tb := g.tbIn(mod, imports, true, s, false)
	mine := tb.insertRow(t, "", nil, false)
	other := tb.insertRow(s.kt, "other", nil, false)
	var theirs string
	tb.inOtherKey(other, func() { theirs = tb.insertRow(t, "", map[string]string{t.PartCol: other}, false) })
	tb.let("let page <- Test.get (%s { %s: %s, ids: [%s, %s] })", mod, s.key, tb.keyRow(), mine, theirs)
	lhs := fmt.Sprintf("chosen: List.map (\\row -> row.%s) page.model.rows", t.IDField)
	rhs := "chosen: [" + mine + "]"
	if at.bulkDel {
		tb.let("let _ <- Test.submit Confirm { }")
		tb.let("let mineAfter <- Test.get (%s { %s: %s })", at.page("Index"), s.key, tb.keyRow())
		tb.let("let theirsAfter <- Test.get (%s { %s: %s })", at.page("Index"), s.key, other)
		lhs += fmt.Sprintf(", mine: List.length mineAfter.model.rows, theirs: List.map (\\row -> row.%s) theirsAfter.model.rows", t.IDField)
		rhs += ", mine: 0, theirs: [" + theirs + "]"
	}
	return TestData{Name: quote("a bulk action stays inside one " + s.key), Lines: tb.lines, Result: "{ " + lhs + " } == { " + rhs + " }"}
}
