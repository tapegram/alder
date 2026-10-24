package generate

import (
	"fmt"
	"slices"
	"strings"

	"alder/internal/syntax"
	"alder/internal/types"
)

// FormData is the data of form.tmpl.
type FormData struct {
	Name   string   // the form: "Contact"
	Msg    string   // the message that posts it: "SubmitContact"
	Fields []string // as declared: "name: Text.NonEmpty"
	Inputs []string // one wrapped input per field: `<al-input field="name" label="Name" />`
	Label  string   // the card's heading and the button's text: "Contact"
}

// snippet executes template name with data, untidied and without a final newline.
func snippet(name string, data any) string {
	var b strings.Builder
	if err := templates.ExecuteTemplate(&b, name, data); err != nil {
		panic(fmt.Sprintf("generate: template %s: %v", name, err))
	}
	return strings.TrimRight(b.String(), "\n")
}

// FormOptions are the options of `alder new form`.
type FormOptions struct {
	Page   string // the page module
	Name   string
	Fields []FieldSpec
	Into   string // --into T: the table the arm inserts into
	Msg    string // --msg M: the message that posts the form ("Submit<Name>" by default)
}

// NewForm generates `alder new form`: in the page module, `form <Name>`, `msg Submit<Name> form
// <Name>` (or --msg's name), an `update` arm (an insert into the table the fields match by name and type, or
// --into's; else the typed hole `?submit<Name>` : Task Nav), the form in `view` (inside its last
// `al-card`, or in a card of its own at the end of the root element), and two browser tests: a
// valid submission (its answer, and the row inserted), and an empty required field
// (FormRejected, nothing inserted). A page whose `view` is still the hole ?view is refused: fill
// it first.
func NewForm(p *Program, o FormOptions) ([]Edit, error) {
	pg, m, err := pageModule(p, o.Page)
	if err != nil {
		return nil, err
	}
	if isFilePage(m.Syntax) {
		return nil, Refusef("a file page has no messages: `view` returns `File`, so nothing can be posted from it; move the message to an HTML page")
	}
	if !IsUpperIdent(o.Name) {
		return nil, Refusef("`%s` is not a form name: a form is named by a capitalised identifier, such as `Contact`", o.Name)
	}
	if slices.ContainsFunc(p.Tables, func(t *Table) bool { return t.Name == o.Name }) {
		return nil, Refusef("`%s` names a table of this app: a form named like it hides the table's row type in `%s`; choose another form name, such as `%sForm`", o.Name, o.Page, o.Name)
	}
	msg := "Submit" + o.Name
	if o.Msg != "" {
		if !IsUpperIdent(o.Msg) {
			return nil, Refusef("`--msg %s` is not a message name: a message is named by a capitalised identifier, such as `Create`", o.Msg)
		}
		if o.Msg == o.Name {
			return nil, Refusef("`--msg %s` names the form itself: the form and its message need two names", o.Msg)
		}
		msg = o.Msg
	}
	// --msg M naming a message that exists without a form: the form is attached to it
	var attach *syntax.MsgDecl
	if o.Msg != "" {
		for _, dc := range m.Syntax.Decls {
			if md, ok := dc.(*syntax.MsgDecl); ok && md.Name == o.Msg {
				attach = md
			}
		}
	}
	if attach != nil && attach.Form != "" {
		attach = nil // a message has one form: the check below refuses it
	}
	for _, n := range []string{o.Name, msg} {
		if n == msg && attach != nil {
			continue
		}
		if declared(m.Syntax, n) {
			return nil, Refusef("`%s` already declares `%s`: choose another name", o.Page, n)
		}
	}
	if viewIsHole(m.Syntax) {
		return nil, Refusef("`%s`'s `view` is a hole: fill `view` first (`alder fill %s.view -`), then add the form", o.Page, o.Page)
	}
	if len(o.Fields) == 0 {
		return nil, Refusef("a form needs at least one field: `alder new form %s %s title:Text.NonEmpty`", o.Page, o.Name)
	}
	seen := map[string]bool{}
	d := FormData{Name: o.Name, Msg: msg, Label: Humanize(o.Name)}
	for _, f := range o.Fields {
		if seen[f.Name] {
			return nil, Refusef("field `%s` is given twice", f.Name)
		}
		seen[f.Name] = true
		d.Fields = append(d.Fields, f.Source())
	}
	imports := newImports(o.Page, p.Std)
	for _, f := range o.Fields {
		if err := bareType(p, o.Page, f.Type); err != nil {
			return nil, err
		}
		typeImports(imports, f.Type, p)
	}

	// the probe: the form declared in the page, to read its fields' types
	e := newSrcEdit(m.Source)
	addImports(e, m.Syntax, imports.list())
	e.appendText("\n" + snippet("form_decl", d))
	probe, err := Probe(p.Dir, []Edit{{Path: m.File, Body: e.result()}})
	if err != nil {
		return nil, err
	}
	if r := probe.errorsIn(m.File); r != nil {
		return nil, r
	}
	rec := probe.Res.Forms[o.Page+"."+o.Name]
	if rec == nil || len(rec.Fields) != len(o.Fields) {
		return nil, Refusef("internal error: the form %s was not checked", o.Name)
	}
	ftypes := map[string]types.Type{}
	for _, f := range rec.Fields {
		ftypes[f.Name] = f.Type
		d.Inputs = append(d.Inputs, input(f.Name, f.Type))
	}

	// the table the arm inserts into
	var into *Table
	requiresUser := pg.Requires != ""
	if attach != nil {
		into = nil // the message's arm exists already
	} else if o.Into != "" {
		if into = p.TableNamed(o.Into); into == nil {
			return nil, Refusef("`--into %s`: `%s` is not a table of this app (its tables: %s)", o.Into, o.Into, tableNames(p))
		}
	} else {
		for _, t := range p.Tables {
			if fieldsFit(t, rec.Fields, pg.Keys) {
				into = t
				break
			}
		}
	}

	e = newSrcEdit(m.Source)
	redirect, answer := "Rerender", "Rerendered"
	if !hasRequiredParams(pg) {
		redirect, answer = "Redirect ("+pageLit(o.Page, nil)+")", "Redirected ("+pageLit(o.Page, nil)+")"
	}
	var arm string
	if into != nil {
		imports.add(into.Module)
		var inits []string
		for _, f := range o.Fields {
			inits = append(inits, f.Name+": p.form."+f.Name)
		}
		op := "Db.insert"
		switch {
		case into.PartKey == "user" && slices.Contains(pg.Keys, "user"):
			op = "Db.insertMine"
		case into.PartKey != "" && slices.Contains(pg.Keys, into.PartKey): // M15: the page's key fills the column
			op = "Db.insertScoped"
		}
		arm = fmt.Sprintf("%s p -> {\n  let _ <- %s %s { %s }\n  Task.succeed (%s)\n},", msg, op, into.Name, strings.Join(inits, ", "), redirect)
		if redirect == "Rerender" {
			arm = strings.Replace(arm, "Task.succeed (Rerender)", "Task.succeed Rerender", 1)
		}
	} else {
		arm = fmt.Sprintf("%s p -> ?%s,", msg, LowerFirst(msg))
	}
	if attach != nil {
		e.insertAt(attach.Span.End.Line, attach.Span.End.Col, " form "+o.Name) // after its parameters, on one line or several
		e.insertBefore(attach.Span.Start.Line, "-- "+msg+"'s arm reads the posted fields as p.form (p.form : "+o.Name+", such as p.form."+o.Fields[0].Name+")\n"+snippet("form_decl", d)+"\n")
		// the message's own button and its "pressing" test (Test.click cannot post a form) give way to the form
		for n := 1; n <= len(m.Syntax.Lines); n++ { // on one line, or over several (`alder fmt` breaks a long one)
			if !strings.HasPrefix(strings.TrimSpace(m.Syntax.Lines[n-1]), "<al-button") {
				continue
			}
			end, text := n, strings.TrimSpace(m.Syntax.Lines[n-1])
			for !strings.HasSuffix(strings.TrimSpace(m.Syntax.Lines[end-1]), "</al-button>") && end < len(m.Syntax.Lines) {
				end++
				text += " " + strings.TrimSpace(m.Syntax.Lines[end-1])
			}
			if strings.HasSuffix(text, "</al-button>") && strings.Count(text, "<al-button") == 1 && strings.Contains(text, "click={"+msg+" ") {
				for k := n; k <= end; k++ {
					e.deleteLine(k)
				}
				n = end
			}
		}
		for _, dc := range m.Syntax.Decls {
			if t, ok := dc.(*syntax.TestDecl); ok && t.Name == "pressing "+msg {
				first := declStart(e, t)
				if first > 1 && strings.TrimSpace(e.line(first-1)) == "" {
					first--
				}
				for n := first; n <= t.Span.End.Line; n++ {
					e.deleteLine(n)
				}
			}
		}
		if attach.Params != nil {
			var holes []string
			for _, f := range attach.Params.Fields {
				holes = append(holes, f.Name+": ?"+LowerFirst(msg)+UpperFirst(f.Name))
			}
			d.Msg = msg + " { " + strings.Join(holes, ", ") + " }"
		}
	} else if err := addDecls(e, m, o.Page, snippet("form_decls", d), arm); err != nil {
		return nil, err
	}
	markup := snippet("form_markup", d)
	if err := addToView(e, m, o.Page, markup, d.Label); err != nil {
		return nil, err
	}

	// the tests
	tb := newTestBuilder(p, o.Page, imports).keyedFor(pg)
	if requiresUser {
		tb.login(roleOf(pg))
	}
	page := tb.pageValue(pg)
	setup := tb.lines
	rowsBefore := 0
	if into != nil {
		rowsBefore = tb.counts[into.Module]
	}
	var values []string
	for _, f := range rec.Fields {
		values = append(values, f.Name+": "+tb.sample(f.Type, f.Name, 1))
	}
	submitted := o.Name
	if attach != nil && attach.Params != nil {
		var holes []string
		for _, f := range attach.Params.Fields {
			holes = append(holes, f.Name+": ?"+LowerFirst(msg)+UpperFirst(f.Name))
		}
		submitted = "(" + msg + " { " + strings.Join(holes, ", ") + " })"
	}
	valid := append(append([]string{}, tb.lines...), "let _ <- Test.get ("+page+")", fmt.Sprintf("let r <- Test.submit %s { %s }", submitted, strings.Join(values, ", ")))
	tests := []TestData{}
	switch {
	case into != nil && rowsBefore == 0:
		// the row as inserted: each field's value, and the defaults of the columns the form leaves out
		var got, want []string
		for i, f := range rec.Fields {
			if into.Column(f.Name) != nil {
				got = append(got, f.Name+": row."+f.Name)
				want = append(want, values[i])
			}
		}
		for _, c := range into.Columns {
			if c.HasDefault && !c.ID && !c.Created && !c.Updated && ftypes[c.Name] == nil {
				if c.Union != nil {
					imports.add(c.Union.Module)
				}
				got = append(got, c.Name+": row."+c.Name)
				want = append(want, c.Name+": "+c.Default)
			}
		}
		valid = append(valid, "let rows <- Db.from "+into.Name+" |> Db.all")
		tests = append(tests, TestData{Name: quote("submitting " + o.Name + " adds a row"), Lines: valid,
			Result: fmt.Sprintf("{ answer: r.answer, rows: List.map (\\row -> { %s }) rows } == { answer: %s, rows: [{ %s }] }", strings.Join(got, ", "), answer, strings.Join(want, ", "))})
	case into != nil:
		valid = append(valid, "let rows <- Db.from "+into.Name+" |> Db.count")
		tests = append(tests, TestData{Name: quote("submitting " + o.Name + " adds a row"), Lines: valid,
			Result: fmt.Sprintf("{ answer: r.answer, rows: rows } == { answer: %s, rows: %d }", answer, rowsBefore+1)})
	default:
		tests = append(tests, TestData{Name: quote("submitting " + o.Name), Lines: valid, Result: "r.answer == ?expected"})
	}
	// an empty required field
	for _, f := range rec.Fields {
		if attach != nil {
			break
		}
		msgText := emptyMessage(f.Type)
		if msgText == "" {
			continue
		}
		var raw []string
		for _, g := range rec.Fields {
			v := rawSample(g.Type)
			if g.Name == f.Name {
				v = ""
			}
			raw = append(raw, fmt.Sprintf("{ name: %s, value: %s }", quote(g.Name), quote(v)))
		}
		lines := append(append([]string{}, setup...), "let _ <- Test.get ("+page+")", fmt.Sprintf("let r <- Test.submitRaw %s [%s]", o.Name, strings.Join(raw, ", ")))
		rejected := fmt.Sprintf("FormRejected [{ field: %s, message: %s }]", quote(f.Name), quote(msgText))
		res := "r.answer == " + rejected
		if into != nil {
			lines = append(lines, "let rows <- Db.from "+into.Name+" |> Db.count")
			res = fmt.Sprintf("{ answer: r.answer, rows: rows } == { answer: %s, rows: %d }", rejected, rowsBefore)
		}
		tests = append(tests, TestData{Name: quote(o.Name + " refuses an empty " + f.Name), Lines: lines, Result: res})
		break
	}
	if err := addTests(e, m, tests); err != nil {
		return nil, err
	}
	addImports(e, m.Syntax, imports.list())
	return []Edit{{Path: m.File, Body: e.result()}}, nil
}

// pageModule is page module name, refused when it is not one of the app's pages.
func pageModule(p *Program, name string) (*Page, *Module, error) {
	pg := p.PageNamed(name)
	if pg == nil {
		if p.Modules[name] != nil {
			return nil, nil, Refusef("`%s` is not a page (it has no `page` declaration): `alder new page %s /path` makes one", name, name)
		}
		return nil, nil, Refusef("there is no page `%s`: `alder new page %s /path` makes one", name, name)
	}
	return pg, p.Modules[name], nil
}

// roleOf is the role a test signs in with to see page pg: its first, or "".
func roleOf(pg *Page) string {
	if len(pg.Roles) > 0 {
		return pg.Roles[0]
	}
	return ""
}

// hasRequiredParams reports whether page pg has a parameter without a default.
func hasRequiredParams(pg *Page) bool {
	for _, par := range pg.Params {
		if !par.HasDefault {
			return true
		}
	}
	return false
}

// fieldsFit reports whether a form with fields inserts a row of t: every field is a column of
// t of the same type, and every column an insert must name is a field (but the partition one,
// `@owner` or `@partition k`, when the page requires its key: Db.insertMine and Db.insertScoped
// name it).
func fieldsFit(t *Table, fields []types.Field, keys []string) bool {
	byName := map[string]types.Type{}
	for _, f := range fields {
		c := t.Column(f.Name)
		if c == nil || c.ID || c.Created || c.Updated || types.String(c.Type) != types.String(f.Type) {
			return false
		}
		byName[f.Name] = f.Type
	}
	for _, c := range t.Columns {
		if c.Required() && byName[c.Name] == nil && !(c.Name == t.PartCol && slices.Contains(keys, t.PartKey)) {
			return false
		}
	}
	return true
}

// input is the wrapped input of a form field: `al-select` for a union, `al-checkbox` for a
// Bool, `al-textarea` for a Text field named body, message or description, `al-input` else.
func input(name string, t types.Type) string {
	elem, _ := unMaybe(t)
	k, _, u := kindOf(elem)
	tag := "al-input"
	switch {
	case u != nil:
		tag = "al-select"
	case k == KindBool:
		tag = "al-checkbox"
	case k == KindText && (name == "body" || name == "message" || name == "description"):
		tag = "al-textarea"
	}
	return fmt.Sprintf("<%s field=%s label=%s />", tag, quote(name), quote(Humanize(name)))
}

// viewIsHole reports whether module m's `view` is a typed hole (`view model session = ?view`).
func viewIsHole(m *syntax.Module) bool {
	v := valueDecl(m, "view")
	if v == nil {
		return false
	}
	_, ok := v.Body.(*syntax.HoleExpr)
	return ok
}

// addDecls inserts decls (a page's new `form`/`msg` declarations) and arm (a new arm of its
// `update`): before `update` with the arm added to its match, or, for a page without
// messages yet, before `view` with a new `update` holding the arm.
func addDecls(e *srcEdit, m *Module, page, decls, arm string) error {
	line, ind, has, ok := updateArmSpot(e, m.Syntax)
	if !ok {
		return Refusef("`%s`'s `update` is not `match msg { … }` written over several lines: add the arm yourself", page)
	}
	if has {
		if last := lastArmLine(e, m.Syntax, line); last > 0 {
			if fixed, changed := withTrailingComma(e.line(last)); changed {
				e.replaceLine(last, fixed)
			}
		}
		e.insertBefore(declStart(e, valueDecl(m.Syntax, "update")), decls+"\n")
		e.insertBefore(line, indent(arm, ind))
		return nil
	}
	view := valueDecl(m.Syntax, "view")
	if view == nil {
		return Refusef("`%s` has no `view`", page)
	}
	e.insertBefore(declStart(e, view), decls+"\n\n"+snippet("update", indent(arm, "  "))+"\n")
	return nil
}

// lastArmLine is the line of the last code in `update`'s match before its closing `}` at close:
// not blank, not a comment; 0 when the match has no arms yet. Alder allows the last arm without
// a trailing comma, so a new arm after it needs one added there.
func lastArmLine(e *srcEdit, m *syntax.Module, close int) int {
	v := valueDecl(m, "update")
	if v == nil {
		return 0
	}
	mt, ok := v.Body.(*syntax.Match)
	if !ok {
		return 0
	}
	for n := close - 1; n > mt.Span.Start.Line; n-- {
		t := strings.TrimSpace(e.line(n))
		if t != "" && !strings.HasPrefix(t, "--") {
			return n
		}
	}
	return 0
}

// withTrailingComma is line with a `,` after its code when it has none, before any trailing
// `-- comment`; changed is false when the comma is already there. A `--` inside a string
// literal is not a comment.
func withTrailingComma(line string) (string, bool) {
	code, comment := line, ""
	inStr := false
	for i := 0; i < len(line); i++ {
		switch {
		case inStr && line[i] == '\\':
			i++
		case line[i] == '"':
			inStr = !inStr
		case !inStr && strings.HasPrefix(line[i:], "--"):
			code, comment = line[:i], line[i:]
			i = len(line)
		}
	}
	trimmed := strings.TrimRight(code, " \t")
	if strings.HasSuffix(trimmed, ",") {
		return line, false
	}
	if comment != "" {
		comment = " " + comment
	}
	return trimmed + "," + comment, true
}

// addToView inserts markup into module m's view: a form (label is its card's heading) inside
// the view's last `al-card`, or in a card of its own at the end of the root element; a button
// (label "") at the end of the root element.
func addToView(e *srcEdit, m *Module, page, markup, label string) error {
	spot, ok := findViewSpot(e, m.Syntax, label == "")
	if !ok {
		return Refusef("`%s`'s `view` is not one element (write it as `<Layout.Page …>` … `</Layout.Page>`, with `alder fill --decl %s.view -`), or add the markup yourself", page, page)
	}
	switch {
	case label == "": // a button, after a blank line, as the root's sections are
		spot.insert(e, "\n"+indent(markup, spot.Indent))
		return nil
	case spot.InCard:
		spot.insert(e, indent(markup, spot.Indent))
		return nil
	}
	card := snippet("card", map[string]string{"Label": label, "Body": indent(markup, "  ")})
	spot.insert(e, "\n"+indent(card, spot.Indent))
	return nil
}

// addTests appends tests to module m, refusing a test name m already has.
func addTests(e *srcEdit, m *Module, tests []TestData) error {
	for _, t := range tests {
		name := strings.Trim(t.Name, `"`)
		if testNamed(m.Syntax, name) {
			return Refusef("`%s` already has a test %s: rename it first", m.Name, t.Name)
		}
		e.appendText("\n" + snippet("test", t))
	}
	return nil
}
