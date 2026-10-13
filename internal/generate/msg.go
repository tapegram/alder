package generate

import (
	"fmt"
	"strings"

	"alder/internal/types"
)

// MsgData is the data of msg.tmpl: the message, its parameters as declared (`id: Post.PostId`),
// the `click` value of its button (`Archive`, `Archive { id: ?archiveId }`) and the button's
// text.
type MsgData struct {
	Name   string
	Fields []string
	Click  string
	Label  string
}

// NewMsg generates `alder new msg`: `msg <Name> { … }` in the page module, an `update` arm
// whose body is the typed hole `?<name>` : Task Nav, an `al-button` sending it at the end of
// the view's root element (its parameters' values are holes `?<name><Field>`), and a browser
// test that presses it (a parameter that is a table's id names a row the test inserts), reads
// again the row (or the page) the arm may change, and ends in the hole `?expected` : Bool, with
// a comment saying what to assert, so that the filled arm's mutants are killable.
func NewMsg(p *Program, page, name string, fields []FieldSpec) ([]Edit, error) {
	pg, m, err := pageModule(p, page)
	if err != nil {
		return nil, err
	}
	if !IsUpperIdent(name) {
		return nil, Refusef("`%s` is not a message name: a message is named by a capitalised identifier, such as `Archive`", name)
	}
	if declared(m.Syntax, name) {
		return nil, Refusef("`%s` already declares `%s`: choose another message name", page, name)
	}
	imports := newImports(page, p.Std)
	d := MsgData{Name: name, Click: name, Label: Humanize(name)}
	var holes []string
	seen := map[string]bool{}
	for _, f := range fields {
		if seen[f.Name] {
			return nil, Refusef("field `%s` is given twice", f.Name)
		}
		seen[f.Name] = true
		d.Fields = append(d.Fields, f.Source())
		typeImports(imports, f.Type, p)
		holes = append(holes, f.Name+": ?"+LowerFirst(name)+UpperFirst(f.Name))
	}
	if len(holes) > 0 {
		d.Click = name + " { " + strings.Join(holes, ", ") + " }"
	}
	e := newSrcEdit(m.Source)
	arm := fmt.Sprintf("%s p -> ?%s,", name, LowerFirst(name))
	if err := addDecls(e, m, page, snippet("msg_decl", d), arm); err != nil {
		return nil, err
	}
	if err := addToView(e, m, page, snippet("msg_button", d), ""); err != nil {
		return nil, err
	}
	// the test presses the button, then reads again what the arm may change: the row a
	// parameter names (inserted for the test), else the page; what to compare is the author's
	tb := newTestBuilder(p, page, imports)
	if pg.Requires != "" {
		tb.login(roleOf(pg))
	}
	pv := tb.pageValue(pg)
	click := name
	var reread string
	if len(fields) > 0 {
		var args []string
		for _, f := range fields {
			v := "?" + LowerFirst(name) + UpperFirst(f.Name)
			for _, t := range p.Tables {
				if f.Type == types.StringIn(t.IDType, page) {
					if v = tb.ids[t.Module]; v == "" { // the page's row, when it shows one
						v = tb.insert(t, true)
					}
					if reread == "" {
						reread = "let after <- Db.get " + t.Name + " " + v
					}
				}
			}
			args = append(args, f.Name+": "+v)
		}
		click = "(" + name + " { " + strings.Join(args, ", ") + " })"
	}
	if reread == "" {
		reread = "let after <- Test.get (" + pv + ")"
	}
	lines := append(tb.lines, "let _ <- Test.get ("+pv+")", "let r <- Test.click "+click, reread)
	test := TestData{Name: quote("pressing " + name), Lines: lines, Result: "?expected",
		Comment: []string{
			"Fill ?expected with what pressing " + name + " must do, so that a test fails when the arm",
			"does something else: its answer (`r.answer`: Rerendered, Redirected …) and what it",
			"changed (`after`, read again), as `{ answer: r.answer, … } == { answer: Rerendered, … }`.",
		}}
	if err := addTests(e, m, []TestData{test}); err != nil {
		return nil, err
	}
	addImports(e, m.Syntax, imports.list())
	return []Edit{{Path: m.File, Body: e.result()}}, nil
}
