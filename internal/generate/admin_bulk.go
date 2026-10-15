package generate

import (
	"fmt"
	"strings"

	"alder/internal/types"
)

// bulkInfo decides what the bulk page of at offers: a delete, unless a delete of a row can meet a
// @restrict below a @cascade (or a tree of the table's own: the closure's deeper nodes), which a
// query over many rows does not follow; and a set for each Bool or union column its form edits.
func (g *adminGen) bulkInfo(at *adminTable) {
	at.bulkDel = true
	for _, n := range walk(g.closure(at)) {
		if n.parent != nil || n.stop {
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

// bulk is the page of the rows chosen on the list: what a delete would do to each (the rows that
// keep one from being deleted are listed and left), and a form for each action.
func (g *adminGen) bulk(at *adminTable) {
	if !at.hasBulk() {
		return
	}
	t := at.t
	mod := at.page("Bulk")
	imports := newImports(mod, g.p.Std)
	imports.add(t.Module)
	importTypes(imports, t.IDType)
	open, close := g.layoutOpen("Selected "+at.plural, imports)
	idT := types.StringIn(t.IDType, mod)
	id := t.IDField
	own := g.isUserTable(at)

	loads := []string{fmt.Sprintf("let rows <- Db.from %s |> Db.where (%s.%s.in params.ids) |> Db.all", t.Name, t.Name, id)}
	var held []string
	if at.bulkDel {
		for i, n := range at.bulkHolds {
			imports.add(n.t.Module)
			ids, pick := "params.ids", "List.map (\\r -> r."+n.col.Name+") "
			if n.col.Maybe {
				ids, pick = "List.map (\\id -> Just id) params.ids", "List.filterMap (\\r -> r."+n.col.Name+") "
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
	loads = append(loads, "Query.succeed { rows: rows, held: "+heldExpr+" }")

	var b strings.Builder
	fmt.Fprintf(&b, "page %s { ids: List %s }%s\n\n", pathLit(at.path+"/bulk"), idT, adminRequires)
	fmt.Fprintf(&b, "-- The rows chosen on the list, and the ids of those that other rows keep from being deleted.\ntype Model = { rows: List %s, held: List %s }\n\n", t.Name, idT)
	b.WriteString(g.load(regionAt("  ", "load", wrapAll(loads))))
	cond := "List.member row." + id + " model.held"
	if own {
		cond += " || " + g.isMe("row."+id)
	}
	b.WriteString("-- refused says whether a row is kept from being deleted: rows refer to it with a `@restrict`" + map[bool]string{true: ",\n-- or it is the signed-in user's own.", false: "."}[own] + "\nrefused : Model -> Session -> " + t.Name + " -> Bool\nrefused model session row = " + cond + "\n\n")

	var forms, arms []string // the actions' forms; the update's arms
	var views []string
	imports.add(t.Module)
	if at.bulkDel {
		forms = append(forms, "form Confirm = { }", "", "msg BulkDelete form Confirm", "")
		arms = append(arms,
			"BulkDelete p -> {",
			"  let allowed = List.filter (\\row -> refused model session row == False) model.rows",
			"  let _ <- Task.all (List.map (\\row -> Db.delete "+t.Name+" row."+id+") allowed)",
			"  Task.succeed (Notice.with (\"Deleted \" ++ Text.fromInt (List.length allowed) ++ \" of \" ++ Text.fromInt (List.length model.rows)) (Redirect ("+at.page("Index")+" {})))",
			"},")
		views = append(views, `<form id="bulk-delete" submit={BulkDelete}>
        <al-alert variant="destructive"><Errors form /></al-alert>
        <al-button variant="destructive" type="submit">Delete the rows that can be deleted</al-button>
      </form>`)
	}
	for _, c := range at.bulkSet {
		importTypes(imports, c.Type)
		name := UpperFirst(c.Name)
		forms = append(forms, "form Set"+name+" = { value: "+c.TypeIn(mod)+" }", "", "msg BulkSet"+name+" form Set"+name, "")
		arms = append(arms,
			"BulkSet"+name+" p -> {",
			"  let _ <- Task.all (List.map (\\row -> Db.update "+t.Name+" row."+id+" (\\r -> { r | "+c.Name+": p.form.value })) model.rows)",
			"  Task.succeed (Notice.with (\"Updated \" ++ Text.fromInt (List.length model.rows)) (Redirect ("+at.page("Index")+" {})))",
			"},")
		tag := "al-select"
		if c.Kind == KindBool {
			tag = "al-checkbox"
		}
		views = append(views, fmt.Sprintf(`<form id="set-%s" submit={BulkSet%s}>
        <%s field="value" label=%s />
        <al-button type="submit">Set %s</al-button>
      </form>`, c.Name, name, tag, quote(Humanize(c.Name)), strings.ToLower(Humanize(c.Name))))
	}
	upd := []string{"-- Each action reads the rows again (load runs before every message) and answers with a notice.",
		"update : Msg -> Model -> Session -> Task Nav", "update msg model session = match msg {"}
	upd = append(append(upd, indentAll(arms, "  ")...), "}")
	b.WriteString(regionAt("", "bulk", append(forms, upd...)) + "\n\n")

	label := textOf(at.label, "row."+at.label.Name)
	b.WriteString("view : Model -> Session -> Html Msg\nview model session =\n")
	fmt.Fprintf(&b, "  %s\n    <h1>{Text.fromInt (List.length model.rows)} selected</h1>\n\n    <al-card>\n      <ul id=\"selected\">\n        {for row in model.rows ->\n          <li>{%s}{if refused model session row then <small> (kept: other rows refer to it)</small> else <span></span>}</li>}\n      </ul>\n%s\n    </al-card>\n    <a href={%s {}}>Back to the list</a>\n",
		open, label, regionAt("      ", "actions", views), at.page("Index"))
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
		sel := []string{free}
		lhs := "answer: r.answer, gone: Maybe.map (\\row -> row." + id + ") gone"
		rhs := "answer: Redirected (" + at.page("Index") + " {}), gone: Nothing"
		name := "deleting the chosen rows"
		if keep != "" {
			sel = []string{keep, free}
			name = "deleting the chosen rows leaves the ones other rows refer to"
		}
		tb.let("let _ <- Test.get (%s)", pageLit(mod, []string{ids(sel...)}))
		tb.let("let r <- Test.submit Confirm { }")
		tb.let("let gone <- Db.get %s %s", t.Name, free)
		if keep != "" {
			tb.let("let kept <- Db.get %s %s", t.Name, keep)
			tb.let("let page <- Test.get (%s)", pageLit(mod, []string{ids(sel...)}))
			lhs += ", kept: Maybe.map (\\row -> row." + id + ") kept, listed: Test.count \"#selected li small\" page.doc"
			rhs += ", kept: Just " + keep + ", listed: 1"
		}
		tests = append(tests, TestData{Name: quote(name), Lines: tb.lines, Result: "{ " + lhs + " } == { " + rhs + " }"})
		if own {
			tb = g.tb(mod, imports, true)
			me := tb.user()
			tb.let("let _ <- Test.get (%s)", pageLit(mod, []string{ids(me)}))
			tb.let("let r <- Test.submit Confirm { }")
			tb.let("let kept <- Db.get %s %s", t.Name, me)
			tests = append(tests, TestData{Name: quote("an Admin's own row is not deleted with the chosen rows"), Lines: tb.lines,
				Result: fmt.Sprintf("Maybe.map (\\row -> row.%s) kept == Just %s", id, me)})
		}
	}
	for i, c := range at.bulkSet {
		tb := g.tb(mod, imports, true)
		a := tb.insertRow(t, "", nil, true)
		o := tb.insertRow(t, "", nil, false)
		v := g.changedValue(tb, at, c, 1)
		tb.let("let _ <- Test.get (%s)", pageLit(mod, []string{ids(a)}))
		tb.let("let r <- Test.submit Set%s { value: %s }", UpperFirst(c.Name), v)
		tb.let("let one <- Db.get %s %s", t.Name, a)
		tb.let("let other <- Db.get %s %s", t.Name, o)
		tests = append(tests, TestData{Name: quote("setting " + strings.ToLower(Humanize(c.Name)) + " changes the chosen rows only"), Lines: tb.lines,
			Result: fmt.Sprintf("{ answer: r.answer, one: Maybe.map (\\row -> row.%s == %s) one, other: Maybe.map (\\row -> row.%s == %s) other } == { answer: Redirected (%s {}), one: Just True, other: Just False }",
				c.Name, v, c.Name, v, at.page("Index"))})
		_ = i
	}
	tests = append(tests, g.gateTests(mod, imports, func(tb *testBuilder) string {
		return pageLit(mod, []string{ids(tb.insertRow(t, "", nil, false))})
	})...)
	b.WriteString(testsText(tests))
	g.file(mod, imports, b.String())
}

// bulkForms are the list's two ways to the bulk page: the rows chosen (the checkboxes name this
// form), and every row on the page (a form of its own, no script needed).
func bulkForms(at *adminTable) string {
	if !at.hasBulk() {
		return ""
	}
	return fmt.Sprintf(`
    <al-stack direction="row" gap="sm">
      <form id="bulk" method="get" action=%s>
        <al-button type="submit" variant="secondary">Act on the chosen rows</al-button>
      </form>
      <form id="bulk-page" method="get" action=%s>
        {for row in model.rows -> <input type="hidden" name="ids" value={%s} />}
        <al-button type="submit" variant="secondary">Act on all rows on this page</al-button>
      </form>
    </al-stack>
`, quote(at.path+"/bulk"), quote(at.path+"/bulk"), textOf(at.idColumn(), "row."+at.t.IDField))
}
