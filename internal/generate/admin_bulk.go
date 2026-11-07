package generate

import (
	"fmt"
	"strings"

	"alder/internal/types"
)

// bulkInfo decides what the bulk page of at offers: a delete, unless a delete of a row can meet a
// @restrict below a @cascade, or a tree of the table's own (a stop node), which the page's query
// of the rows' direct referrers does not follow (a @cascade below a @cascade is the database's
// to follow, and is no reason); and a set for each Bool or union column its form edits.
func (g *adminGen) bulkInfo(at *adminTable) {
	at.bulkDel = true
	for _, n := range walk(g.closure(at)) {
		if n.stop || (n.parent != nil && n.col.OnDelete == "restrict") {
			at.bulkDel = false
		}
		if n.parent == nil && !n.stop && n.col.OnDelete == "restrict" {
			at.bulkHolds = append(at.bulkHolds, n)
		}
	}
	for _, c := range at.editable {
		if c.Ref == "" && !c.Maybe && c != g.role && (c.Kind == KindBool || (c.Kind == KindUnion && c.Union != nil)) {
			at.bulkSet = append(at.bulkSet, c)
		}
	}
}

func (at *adminTable) hasBulk() bool { return at.bulkDel || len(at.bulkSet) > 0 }

func (at *adminTable) idColumn() *Column {
	for _, c := range at.t.Columns {
		if c.ID {
			return c
		}
	}
	return nil
}

// bulk is the page of the rows chosen on the list (their ids its parameter): what a delete would
// do to each (the rows that keep one from being deleted are listed and left), and a form for
// each action, each a POST route taking the same ids, which answers the page again (refresh)
// with a toast.
func (g *adminGen) bulk(at *adminTable) {
	if !at.hasBulk() {
		return
	}
	t := at.t
	mod := at.page("Bulk")
	g.names[mod] = []string{"Bulk"}
	imports := newImports(mod, g.p.Std)
	imports.add(t.Module)
	importTypes(imports, t.IDType)
	open, close := g.layoutOpen("Selected "+at.plural, imports)
	idT := types.StringIn(t.IDType, mod)
	id := t.IDField
	own := g.isUserTable(at)
	// refused is called with the signed-in user's id for the user table (a helper's Session would
	// be App's, and a route's own does not widen to it in the generated Go)
	refused := "refused model row"
	if own {
		refused = "refused model session.user row"
	}

	loads := []string{"let taken = List.take maxRows ids", fmt.Sprintf("let rows <- Db.from %s |> Db.where (%s.%s.in taken) |> Db.all", t.Name, t.Name, id)}
	var held []string
	if at.bulkDel {
		for i, n := range at.bulkHolds {
			imports.add(n.t.Module)
			ids, pick := "taken", "List.map (\\r -> r."+n.col.Name+") "
			if n.col.Maybe {
				ids, pick = "List.map (\\id -> Just id) taken", "List.filterMap (\\r -> r."+n.col.Name+") "
			}
			v := fmt.Sprintf("held%d", i+1)
			loads = append(loads, fmt.Sprintf("let %s <- Db.from %s |> Db.where (%s.%s.in (%s)) |> Db.all", v, n.t.Name, n.t.Name, n.col.Name, ids))
			held = append(held, pick+v)
		}
	}
	heldExpr := "[]"
	if len(held) > 0 {
		heldExpr = "List.concat [" + strings.Join(held, ", ") + "]"
	}
	loads = append(loads, "Query.succeed { rows: rows, held: "+heldExpr+", chosen: List.length ids, ids: ids }")

	path := at.path + "/bulk"
	params := at.params(mod, imports, []string{"ids: List " + idT})
	routes := []string{routeLine("Bulk", "GET", path, g.keepWires(mod, params), "", at.requires())}
	var forms []string // the actions' forms and updates
	var views []string
	action := func(name, form, verb, ok string, body []string) {
		g.names[mod] = append(g.names[mod], name)
		routes = append(routes, routeLine(name, "POST", writePath(path, name), params, form, at.requires()))
		upd := append([]string{"let model <- selected params.ids", "if tooMany model then", "  Task.succeed (Response.invalid (" + bulkTooMany + "))", "else {"}, indentAll(body, "  ")...)
		upd = append(upd, "  Task.succeed (Response.refresh |> Response.success ("+quote(verb+" ")+" ++ "+ok+"))", "}")
		forms = append(forms, strings.Split(strings.TrimSuffix(g.update(name, strings.Join(indentAll(upd, "  "), "\n")), "\n"), "\n")...)
	}
	if at.bulkDel {
		forms = append(forms, "-- BulkDelete takes no input: the form is the button, and the place a refusal is shown.", "form Confirm = { }", "")
		action("BulkDelete", "Confirm", "Deleted", "Text.fromInt (List.length allowed) ++ \" of \" ++ Text.fromInt (List.length model.rows)", []string{
			"let allowed = List.filter (\\row -> " + refused + " == False) model.rows",
			"let _ <- Task.all (List.map (\\row -> Db.delete " + t.Name + " row." + id + ") allowed)",
		})
		views = append(views, `<form id="bulk-delete" submit={BulkDelete { ids: model.ids }}>`,
			`  <al-alert variant="destructive"><Errors form /></al-alert>`,
			`  <al-button variant="destructive" type="submit">Delete the rows that can be deleted</al-button>`,
			`</form>`)
	}
	for _, c := range at.bulkSet {
		importTypes(imports, c.Type)
		name := UpperFirst(c.Name)
		forms = append(forms, "form Set"+name+" = { value: "+c.TypeIn(mod)+" }", "")
		action("BulkSet"+name, "Set"+name, "Updated", "Text.fromInt (List.length model.rows)", []string{
			"let _ <- Task.all (List.map (\\row -> Db.update " + t.Name + " row." + id + " (\\r -> { r | " + c.Name + ": params.form.value })) model.rows)",
		})
		tag := "al-select"
		if c.Kind == KindBool {
			tag = "al-checkbox"
		}
		views = append(views, fmt.Sprintf(`<form id="set-%s" submit={BulkSet%s { ids: model.ids }}>`, c.Name, name),
			`  <al-alert variant="destructive"><Errors form /></al-alert>`,
			fmt.Sprintf(`  <%s field="value" label=%s />`, tag, quote(Humanize(c.Name))),
			fmt.Sprintf(`  <al-button type="submit">Set %s</al-button>`, strings.ToLower(Humanize(c.Name))),
			`</form>`)
	}

	var b strings.Builder
	b.WriteString(regionAt("", "routes", routes) + "\n\n")
	fmt.Fprintf(&b, "-- The rows chosen on the list (at most maxRows of them), the ids of those that other rows keep\n-- from being deleted, how many ids the page was given, and the ids, which each action takes.\ntype Model = { rows: List %s, held: List %s, chosen: Int, ids: List %s }\n\n", t.Name, idT, idT)
	fmt.Fprintf(&b, "-- maxRows is how many rows one action takes: a page of the list. More are refused.\nmaxRows : Int\nmaxRows = %d\n\n", bulkMax)
	b.WriteString("-- tooMany says whether the page was given more ids than an action takes.\ntooMany : Model -> Bool\ntooMany model = model.chosen > maxRows\n\n")
	b.WriteString("-- selected is the rows of ids: the page shows them, and each action reads them again.\n")
	fmt.Fprintf(&b, "selected : List %s -> Query Model\nselected ids = {\n%s\n}\n\n", idT, regionAt("  ", "load", wrapAll(loads)))
	b.WriteString(g.load("Bulk", "  selected params.ids"))
	cond := "List.member row." + id + " model.held"
	sig := "refused : Model -> " + t.Name + " -> Bool\nrefused model row = "
	kept := `{if ` + refused + ` then <small> (kept: other rows refer to it)</small> else <span></span>}`
	if own {
		cond += " || me == row." + id
		uid, _ := unMaybe(g.p.Session.User.Type)
		importTypes(imports, uid)
		sig = "refused : Model -> " + types.StringIn(uid, mod) + " -> " + t.Name + " -> Bool\nrefused model me row = "
		kept = `{if ` + g.isMe("row."+id) + ` then <small> (kept: your own account)</small> else if ` + refused + ` then <small> (kept: other rows refer to it)</small> else <span></span>}`
	}
	b.WriteString("-- refused says whether a row is kept from being deleted: rows refer to it with a `@restrict`" + map[bool]string{true: ",\n-- or it is the signed-in user's own (me).", false: "."}[own] + "\n" + sig + cond + "\n\n")
	for len(forms) > 0 && forms[len(forms)-1] == "" {
		forms = forms[:len(forms)-1]
	}
	upd := append([]string{"-- Each action reads the rows again and answers with the page again and a toast; none takes",
		"-- more than maxRows rows.", ""}, forms...)
	b.WriteString(regionAt("", "bulk", upd) + "\n\n")

	label := textOf(at.label, "row."+at.label.Name)
	b.WriteString(viewHead("Bulk"))
	fmt.Fprintf(&b, "  %s\n    <h1>{Text.fromInt (List.length model.rows)} selected</h1>\n\n    <al-card>\n      {if tooMany model then <al-alert id=\"too-many\" variant=\"warning\">{%s}</al-alert> else Html.none}\n      <ul id=\"selected\">\n        {for row in model.rows ->\n          <li>{%s}%s</li>}\n      </ul>\n%s\n    </al-card>\n    <a href={%s {}}>Back to the list</a>\n",
		open, bulkTooMany, label, kept, regionAt("      ", "actions", views), at.route("Index"))
	fmt.Fprintf(&b, "  %s\n", close)

	var tests []TestData
	ids := func(v ...string) string { return "ids: [" + strings.Join(v, ", ") + "]" }
	if at.bulkDel {
		tb := g.tb(mod, imports, true)
		var keep string
		if len(at.bulkHolds) > 0 {
			h := at.bulkHolds[0]
			keep = tb.insertRow(t, "", nil, true)
			tb.insertRow(h.t, "_", map[string]string{h.col.Name: maybeJust(h.col, keep)}, false)
		}
		free := tb.insertRow(t, "", nil, keep == "")
		unchosen := tb.insertRow(t, "", nil, false)
		sel := []string{free}
		lhs := "answer: r.answer, gone: Maybe.map (\\row -> row." + id + ") gone, left: Maybe.map (\\row -> row." + id + ") left"
		rhs := "answer: Refreshed, gone: Nothing, left: Just " + unchosen
		name := "deleting the chosen rows"
		if keep != "" {
			sel = []string{keep, free}
			name = "deleting the chosen rows leaves the ones other rows refer to"
		}
		tb.let("let _ <- Test.get (%s)", pageLit("Bulk", []string{ids(sel...)}))
		tb.let("let r <- Test.submit (%s) { }", pageLit("BulkDelete", []string{ids(sel...)}))
		tb.let("let gone <- Db.get %s %s", t.Name, free)
		tb.let("let left <- Db.get %s %s", t.Name, unchosen)
		if keep != "" { // the page again lists the row kept, and why
			tb.let("let kept <- Db.get %s %s", t.Name, keep)
			lhs += ", kept: Maybe.map (\\row -> row." + id + ") kept, listed: Test.count \"#selected li small\" r.doc"
			rhs += ", kept: Just " + keep + ", listed: 1"
		}
		tests = append(tests, TestData{Name: quote(name), Lines: tb.lines, Result: "{ " + lhs + " } == { " + rhs + " }"})
		if own {
			tb = g.tb(mod, imports, true)
			me := tb.user()
			tb.let("let page <- Test.get (%s)", pageLit("Bulk", []string{ids(me)}))
			tb.let("let r <- Test.submit (%s) { }", pageLit("BulkDelete", []string{ids(me)}))
			tb.let("let kept <- Db.get %s %s", t.Name, me)
			tests = append(tests, TestData{Name: quote("an Admin's own row is not deleted with the chosen rows"), Lines: tb.lines,
				Result: fmt.Sprintf("{ kept: Maybe.map (\\row -> row.%s) kept, says: Test.texts \"#selected small\" page.doc } == { kept: Just %s, says: [\"(kept: your own account)\"] }", id, me)})
		}
	}
	for _, c := range at.bulkSet {
		tb := g.tb(mod, imports, true)
		a := tb.insertRow(t, "", nil, true)
		o := tb.insertRow(t, "", nil, false)
		v := g.changedValue(tb, at, c, 1)
		tb.let("let _ <- Test.get (%s)", pageLit("Bulk", []string{ids(a)}))
		tb.let("let r <- Test.submit (%s) { value: %s }", pageLit("BulkSet"+UpperFirst(c.Name), []string{ids(a)}), v)
		tb.let("let one <- Db.get %s %s", t.Name, a)
		tb.let("let other <- Db.get %s %s", t.Name, o)
		tests = append(tests, TestData{Name: quote("setting " + strings.ToLower(Humanize(c.Name)) + " changes the chosen rows only"), Lines: tb.lines,
			Result: fmt.Sprintf("{ answer: r.answer, one: Maybe.map (\\row -> row.%s == %s) one, other: Maybe.map (\\row -> row.%s == %s) other } == { answer: Refreshed, one: Just True, other: Just False }",
				c.Name, v, c.Name, v)})
	}
	// at most maxRows ids: maxRows are taken, one more is refused (the others are ids of no row)
	for _, n := range []int{bulkMax, bulkMax + 1} {
		tb := g.tb(mod, imports, true)
		one := tb.insertRow(t, "", nil, false)
		tb.let("let chosen = [%s] ++ List.map (\\n -> %s (\"none-\" ++ Text.fromInt n)) (List.range 1 %d)", one, tb.brandCtor(t.IDType.(*types.Brand)), n-1)
		tb.let("let page <- Test.get (%s)", pageLit("Bulk", []string{"ids: chosen"}))
		var lhs, rhs string
		if at.bulkDel {
			tb.let("let r <- Test.submit (%s) { }", pageLit("BulkDelete", []string{"ids: chosen"}))
			tb.let("let after <- Db.get %s %s", t.Name, one)
			lhs, rhs = "kept: Maybe.map (\\row -> row."+id+") after", "kept: Nothing"
			if n > bulkMax {
				rhs = "kept: Just " + one
			}
		} else {
			c := at.bulkSet[0]
			v := g.changedValue(tb, at, c, 1)
			tb.let("let r <- Test.submit (%s) { value: %s }", pageLit("BulkSet"+UpperFirst(c.Name), []string{"ids: chosen"}), v)
			tb.let("let after <- Db.get %s %s", t.Name, one)
			lhs, rhs = fmt.Sprintf("set: Maybe.map (\\row -> row.%s == %s) after", c.Name, v), "set: Just True"
			if n > bulkMax {
				rhs = "set: Just False"
			}
		}
		answer, warned, name := "Refreshed", "0", fmt.Sprintf("%d chosen ids are taken", n)
		if n > bulkMax {
			answer, warned, name = "FormRejected [{ field: \"\", message: \"Choose at most "+fmt.Sprint(bulkMax)+" rows at a time.\" }]", "1", fmt.Sprintf("%d chosen ids are refused", n)
		}
		tests = append(tests, TestData{Name: quote(name), Lines: tb.lines,
			Result: fmt.Sprintf("{ answer: r.answer, %s, warned: Test.count \"#too-many\" page.doc } == { answer: %s, %s, warned: %s }", lhs, answer, rhs, warned)})
	}
	if at.silo != nil {
		tests = append(tests, g.siloBulkTest(at, mod, imports))
	}
	tests = append(tests, g.gateTests(mod, imports, func(tb *testBuilder) string {
		return pageLit("Bulk", []string{ids(tb.insertRow(t, "", nil, false))})
	})...)
	b.WriteString(g.testsIn(mod, tests))
	g.file(mod, imports, b.String())
}

// bulkMax is the most rows a bulk action takes (maxRows): a page of the list (perPage).
const bulkMax = 25

// bulkTooMany is the bulk page's refusal of more than maxRows ids, an Alder expression.
const bulkTooMany = `"Choose at most " ++ Text.fromInt maxRows ++ " rows at a time."`

// bulkForms are the list's two ways to the bulk page: the rows chosen (the checkboxes name this
// form), and every row on the page (a form of its own, no script needed).
func (g *adminGen) bulkForms(at *adminTable) string {
	if !at.hasBulk() {
		return ""
	}
	return fmt.Sprintf(`
    <al-stack direction="row" gap="sm">
      <form id="bulk" method="get" action=%s>
        <al-button type="submit" variant="secondary">Act on the chosen rows</al-button>
      </form>
      <form id="bulk-page" method="get" action=%s>
        {for row in model.rows -> <input type="hidden" name=%s value={%s} />}
        <al-button type="submit" variant="secondary">Act on all rows on this page</al-button>
      </form>
    </al-stack>
`, at.action("/bulk"), at.action("/bulk"), quote(g.wire(at.page("Bulk"), "ids")), textOf(at.idColumn(), "row."+at.t.IDField))
}

// findTests are the New (or Edit) page's tests of a long table behind a reference: at exactly
// 200 rows its select holds them all, past that it holds 50 and the search narrows them (Edit
// keeps the row's own value too: ownFirstTest), the search route answers the page at the text, and a value
// outside the rendered options (a crafted request) is refused with the field error.
func (g *adminGen) findTests(at *adminTable, mod string, imports *importSet, edit bool) []TestData {
	var tests []TestData
	get := "New"
	if edit {
		get = "Edit"
	}
	for _, c := range at.refs {
		if !g.finds(c) {
			continue
		}
		rt := g.p.TableNamed(c.Ref)
		sel := "select[name=" + c.Name + "] option"
		start := func() (*testBuilder, func(extra ...string) string, string) {
			tb := g.tb(mod, imports, true)
			target := ""
			if edit {
				var over map[string]string
				if c.Maybe { // the row refers to a row of the long table, as a required reference does
					over = map[string]string{c.Name: "Just " + tb.insertRow(rt, "", nil, true)}
				}
				target = tb.insertRow(at.t, "", over, false)
			}
			g.optionRows(tb, at)
			return tb, func(extra ...string) string {
				if edit {
					extra = append([]string{"id: " + target}, extra...)
				}
				return pageLit(get, extra)
			}, target
		}
		// exactly 200 rows are all offered
		tb, at200, _ := start()
		tb.bulkInsert(rt, 200-tb.counts[rt.Module], "")
		tb.let("let page <- Test.get (%s)", at200())
		tests = append(tests, TestData{Name: quote("a list of 200 " + pluralOf(rt) + " is offered whole"), Lines: tb.lines,
			Result: fmt.Sprintf("List.length page.model.%s == 200", optionsField(c))})

		tb, pageAt, target := start()
		tb.bulkInsert(rt, 201, "")
		last := rawText(rt.Label(), tb.counts[rt.Module])
		tb.let("let all <- Test.get (%s)", pageAt())
		tb.let("let found <- Test.get (%s)", pageAt(findParam(c)+": "+quote(last)))
		search := at.own("Search"+UpperFirst(c.Name), true)
		redirect := pageLit(get, []string{findParam(c) + ": \"zz\""})
		if edit {
			search = "(Search" + UpperFirst(c.Name) + " { id: " + target + " })"
			redirect = pageLit(get, []string{"id: " + target, findParam(c) + ": \"zz\""})
		}
		tb.let("let r <- Test.submit %s { q: \"zz\" }", search)
		found := fmt.Sprintf("Test.count %s found.doc <= 3", quote(sel))
		if !edit {
			want := "[" + quote(last) + "]"
			if c.Maybe {
				want = "[\"\", " + quote(last) + "]"
			}
			found = fmt.Sprintf("Test.texts %s found.doc == %s", quote(sel), want)
		}
		shown := 50
		if edit { // the row's own value, then 50
			shown = 51
		}
		tests = append(tests, TestData{Name: quote("a long list of " + pluralOf(rt) + " offers 50, and the search narrows it"), Lines: tb.lines,
			Result: fmt.Sprintf("{ shown: List.length all.model.%s, total: all.model.%s, found: %s, answer: r.answer } == { shown: %d, total: %d, found: True, answer: Redirected (%s) }",
				optionsField(c), totalField(c), found, shown, tb.counts[rt.Module], redirect)})
		if edit {
			tests = append(tests, g.ownFirstTest(at, mod, imports, c))
		}
		if c.Maybe || edit {
			continue
		}
		tb.let("let pick = Maybe.withDefault \"\" (Test.attr \"value\" %s found.doc)", quote(sel))
		tb.let("let page <- Test.get (%s)", pageAt())
		var raw []string
		for _, d := range at.editable {
			v := quote(rawSample(d.Type))
			switch {
			case d == c:
				v = "pick"
			case d.Ref != "" && g.p.TableNamed(d.Ref) != nil && !d.Maybe:
				v = d.Name + "Id"
				tb.let("let %s = Maybe.withDefault \"\" (Test.attr \"value\" \"select[name=%s] option\" page.doc)", v, d.Name)
			}
			raw = append(raw, fmt.Sprintf("{ name: %s, value: %s }", quote(d.Name), v))
		}
		tb.let("let sent <- Test.submitRaw %s [%s]", at.formRoute(false, ""), strings.Join(raw, ", "))
		tb.let("let rows <- Db.from %s |> Db.count", at.t.Name)
		tests = append(tests, TestData{Name: quote(article(strings.ToLower(Humanize(c.Name))) + " outside the narrowed list is refused"), Lines: tb.lines,
			Result: fmt.Sprintf("{ answer: sent.answer, rows: rows } == { answer: FormRejected [{ field: %s, message: \"must be one of the options\" }], rows: %d }", quote(c.Name), tb.counts[at.t.Module])})
	}
	return tests
}

// ownFirstTest is the Edit page's test of a long table behind reference c whose row's own value
// sorts after the 50 the select holds (its label is "zz-…", every other row's matches the empty
// search): the select offers it first and once, and saving the row as it is keeps it.
func (g *adminGen) ownFirstTest(at *adminTable, mod string, imports *importSet, c *Column) TestData {
	rt := g.p.TableNamed(c.Ref)
	tb := g.tb(mod, imports, true)
	l := rt.Label()
	last := tb.insertRow(rt, "", map[string]string{l.Name: tb.sample(l.Type, "zz", 1)}, false)
	target := tb.insertRow(at.t, "", map[string]string{c.Name: maybeJust(c, last)}, false)
	tb.bulkInsert(rt, 201, "")
	var same []string
	for _, d := range at.editable {
		same = append(same, d.Name+": page.model.row."+d.Name)
	}
	tb.let("let page <- Test.get (%s)", pageLit("Edit", []string{"id: " + target}))
	tb.let("let r <- Test.submit %s { %s }", at.formRoute(true, target), strings.Join(same, ", "))
	tb.let("let after <- Db.get %s %s", at.t.Name, target)
	n := optionsField(c)
	return TestData{Name: quote("the row's own " + strings.ToLower(Humanize(c.Name)) + " is offered first when it sorts past the 50"), Lines: tb.lines,
		Result: fmt.Sprintf("{ answer: r.answer, kept: Maybe.map (\\row -> row.%s) after, first: Maybe.map (\\o -> o.%s) (List.head page.model.%s), once: List.length (List.filter (\\o -> o.%s == %s) page.model.%s), shown: List.length page.model.%s } == { answer: Redirected (%s { id: %s }), kept: Just %s, first: Just %s, once: 1, shown: 51 }",
			c.Name, rt.IDField, n, rt.IDField, last, n, n, at.route("Show"), target, paren(maybeJust(c, last)), last)}
}
