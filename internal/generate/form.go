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
	Msg    string // --msg M: the message that posts the form ("Submit<Name>" by default); on a route module, the route's name
	Hole   bool   // --hole: on a route module, the `update` body is the hole ?answer
}

// NewForm generates `alder new form`: newRouteForm's POST route on a route module, refused on
// any other.
func NewForm(p *Program, o FormOptions) ([]Edit, error) {
	if len(p.RoutesOf(o.Page)) == 0 {
		return nil, notRouteModule(p, o.Page)
	}
	return newRouteForm(p, o)
}

// notRouteModule is the refusal for a module name that is not one of the app's route modules.
func notRouteModule(p *Program, name string) error {
	if p.Modules[name] != nil {
		return Refusef("`%s` is not a route module (it declares no route): `alder new route %s /path` makes one", name, name)
	}
	return Refusef("there is no route module `%s`: `alder new route %s /path` makes one", name, name)
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

// noticesGot and noticesWant are a test's `notices` fields for the toast notice ("" for none).
func noticesGot(notice string) string {
	if notice == "" {
		return ""
	}
	return "notices: r.notices, "
}

func noticesWant(notice string) string {
	if notice == "" {
		return ""
	}
	return "notices: [" + quote(notice) + "], "
}

// formTestSpec is what formTests needs to know of the form it tests.
type formTestSpec struct {
	Form    string        // the form's name
	Page    string        // the module, for record names
	Open    string        // the page value the test opens
	Submit  string        // what `Test.submit` posts to: the form, or a route with its parameters
	Raw     string        // the same for `Test.submitRaw`
	Answer  string        // the answer of a valid submission
	Fields  []types.Field // the form's fields
	Types   map[string]types.Type
	Into    *Table // the table the update inserts into, or nil
	NoEmpty bool   // no test of an empty required field
	Notice  string // the toast a valid submission's update sends ("Saved"), asserted with the rows; "" for none
}

// formTests are the tests of a form: a valid submission (its answer, and the row inserted into
// s.Into, or the hole ?expected without a table) and an empty required field (FormRejected,
// nothing inserted). tb has logged in and made the setup of the page s.Open.
func formTests(s formTestSpec, tb *testBuilder, imports *importSet) []TestData {
	setup := tb.lines
	into, answer := s.Into, s.Answer
	rowsBefore := 0
	if into != nil {
		rowsBefore = tb.counts[into.Module]
	}
	var values []string
	for _, f := range s.Fields {
		values = append(values, f.Name+": "+tb.sample(f.Type, f.Name, 1))
	}
	valid := append(append([]string{}, tb.lines...), "let _ <- Test.get ("+s.Open+")", fmt.Sprintf("let r <- Test.submit %s { %s }", s.Submit, strings.Join(values, ", ")))
	tests := []TestData{}
	switch {
	case into != nil && rowsBefore == 0:
		// the row as inserted: each field's value, and the defaults of the columns the form leaves out
		var got, want []string
		for i, f := range s.Fields {
			if into.Column(f.Name) != nil {
				got = append(got, f.Name+": row."+f.Name)
				want = append(want, values[i])
			}
		}
		for _, c := range into.Columns {
			if c.HasDefault && !c.ID && !c.Created && !c.Updated && s.Types[c.Name] == nil {
				if c.Union != nil {
					imports.add(c.Union.Module)
				}
				got = append(got, c.Name+": row."+c.Name)
				want = append(want, c.Name+": "+c.Default)
			}
		}
		valid = append(valid, "let rows <- Db.from "+recordName(into, s.Page)+" |> Db.all")
		tests = append(tests, TestData{Name: quote("submitting " + s.Form + " adds a row"), Lines: valid,
			Result: fmt.Sprintf("{ answer: r.answer, %srows: List.map (\\row -> { %s }) rows } == { answer: %s, %srows: [{ %s }] }", noticesGot(s.Notice), strings.Join(got, ", "), answer, noticesWant(s.Notice), strings.Join(want, ", "))})
	case into != nil:
		valid = append(valid, "let rows <- Db.from "+recordName(into, s.Page)+" |> Db.count")
		tests = append(tests, TestData{Name: quote("submitting " + s.Form + " adds a row"), Lines: valid,
			Result: fmt.Sprintf("{ answer: r.answer, %srows: rows } == { answer: %s, %srows: %d }", noticesGot(s.Notice), answer, noticesWant(s.Notice), rowsBefore+1)})
	default:
		tests = append(tests, TestData{Name: quote("submitting " + s.Form), Lines: valid, Result: "r.answer == ?expected"})
	}
	if s.NoEmpty {
		return tests
	}
	for _, f := range s.Fields { // the first required field, empty
		msgText := emptyMessage(f.Type)
		if msgText == "" {
			continue
		}
		var raw []string
		for _, g := range s.Fields {
			v := rawSample(g.Type)
			if g.Name == f.Name {
				v = ""
			}
			raw = append(raw, fmt.Sprintf("{ name: %s, value: %s }", quote(g.Name), quote(v)))
		}
		lines := append(append([]string{}, setup...), "let _ <- Test.get ("+s.Open+")", fmt.Sprintf("let r <- Test.submitRaw %s [%s]", s.Raw, strings.Join(raw, ", ")))
		rejected := fmt.Sprintf("FormRejected [{ field: %s, message: %s }]", quote(f.Name), quote(msgText))
		res := "r.answer == " + rejected
		if into != nil {
			lines = append(lines, "let rows <- Db.from "+recordName(into, s.Page)+" |> Db.count")
			res = fmt.Sprintf("{ answer: r.answer, rows: rows } == { answer: %s, rows: %d }", rejected, rowsBefore)
		}
		tests = append(tests, TestData{Name: quote(s.Form + " refuses an empty " + f.Name), Lines: lines, Result: res})
		break
	}
	return tests
}

// dropMsgControls deletes msg's own button in the view of m and its "pressing <msg>" test from e:
// Test.click cannot post a form, so they give way to the form that replaces them.
func dropMsgControls(e *srcEdit, m *Module, msg string) {
	for n := 1; n <= len(m.Syntax.Lines); n++ { // on one line, or over several (`alder fmt` breaks a long one)
		if !strings.HasPrefix(strings.TrimSpace(m.Syntax.Lines[n-1]), "<al-button") {
			continue
		}
		end, text := n, strings.TrimSpace(m.Syntax.Lines[n-1])
		for !strings.HasSuffix(strings.TrimSpace(m.Syntax.Lines[end-1]), "</al-button>") && end < len(m.Syntax.Lines) {
			end++
			text += " " + strings.TrimSpace(m.Syntax.Lines[end-1])
		}
		if strings.HasSuffix(text, "</al-button>") && strings.Count(text, "<al-button") == 1 && (strings.Contains(text, "click={"+msg+" ") || strings.Contains(text, "click={"+msg+"}")) {
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
