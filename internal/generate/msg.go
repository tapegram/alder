package generate

import (
	"fmt"
	"strings"

	"alder/internal/syntax"
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
	Class  string // the button's class, "" for none (a button in a loop has its message's name)
	ID     string // the button's id (--id), "" for none
	When   string // the condition the button is shown on, "" for always
}

// MsgOptions are `alder new msg`'s placement options.
type MsgOptions struct {
	InFor string // --in-for: the binder of the view's loop the button goes in
	When  string // --when: the Bool expression the button is shown on
	ID    string // --id: the button's id attribute
}

// NewMsg generates `alder new msg`: `msg <Name> { … }` in the page module, an `update` arm
// whose body is the typed hole `?<name>` : Task Nav, an `al-button` sending it at the end of
// the view's root element (its parameters' values are holes `?<name><Field>`), and a browser
// test that presses it (a parameter that is a table's id names a row the test inserts), reads
// again the row the arm may change (or opens the page again, whatever its answer: the arm may
// delete the page's own row), and ends in the hole `?expected` : Bool, with
// a comment saying what to assert, so that the filled arm's mutants are killable.
//
// With o.InFor (`--in-for <binder>`) the button goes at the end of the body element of the
// view's `{for <binder> in …}` instead, with the class of the message's name (`class="remove"`,
// so a test can count the rows' buttons), and each parameter's value is taken from the binder
// where it can be (binderValue: `user: m.user`, `user: u.id`), a hole otherwise. With o.When
// (`--when <Bool expression>`) the button is shown only when the expression holds: `{if <expr>
// then <al-button …> else Html.none}`.
func NewMsg(p *Program, page, name string, fields []FieldSpec, o MsgOptions) ([]Edit, error) {
	if page == "App" {
		if o.InFor != "" || o.When != "" || o.ID != "" {
			return nil, Refusef("`alder new msg App` adds a message without a button: --in-for, --when and --id belong to a page")
		}
		return newAppMsg(p, name, fields)
	}
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
	d := MsgData{Name: name, Click: name, Label: Humanize(name), ID: o.ID}
	if o.ID != "" && !validID(o.ID) {
		return nil, Refusef("`--id %s` is not an id: use letters, digits, `-` and `_`, starting with a letter", o.ID)
	}
	if viewIsHole(m.Syntax) {
		return nil, Refusef("`%s`'s `view` is a hole: fill `view` first (`alder fill %s.view -`), then add the message's button", page, page)
	}
	var loop *syntax.HtmlFor
	if o.InFor != "" {
		n := 0
		if loop, n = findFor(m.Syntax, o.InFor); loop == nil {
			return nil, Refusef("`%s`'s `view` has no `{for %s in …}`: name the loop variable of one of its loops", page, o.InFor)
		} else if n > 1 {
			return nil, Refusef("`%s`'s `view` has %d loops `{for %s in …}`: rename one, so that --in-for names one loop", page, n, o.InFor)
		}
		d.Class = kebab(name)
	}
	if w := strings.TrimSpace(o.When); w != "" {
		if _, ds, more, _ := syntax.ParseExpr("--when", []byte(w)); more != "" || len(ds) > 0 || strings.Contains(w, "\n") {
			return nil, Refusef("`--when %s` is not one expression", w)
		}
		d.When = w
	}
	var holes []string
	seen := map[string]bool{}
	for _, f := range fields {
		if seen[f.Name] {
			return nil, Refusef("field `%s` is given twice", f.Name)
		}
		seen[f.Name] = true
		d.Fields = append(d.Fields, f.Source())
		if err := bareType(p, page, f.Type); err != nil {
			return nil, err
		}
		typeImports(imports, f.Type, p)
		v := ""
		if loop != nil {
			v = binderValue(p, o.InFor, p.Res.Binds[loop], f.Name, f.Type, page)
		}
		if v == "" {
			v = "?" + LowerFirst(name) + UpperFirst(f.Name)
		}
		holes = append(holes, f.Name+": "+v)
	}
	if len(holes) > 0 {
		d.Click = name + " { " + strings.Join(holes, ", ") + " }"
	}
	e := newSrcEdit(m.Source)
	arm := fmt.Sprintf("%s p -> ?%s,", name, LowerFirst(name))
	if err := addDecls(e, m, page, snippet("msg_decl", d), arm); err != nil {
		return nil, err
	}
	if loop != nil {
		if !addToFor(e, loop, snippet("msg_button", d)) {
			return nil, Refusef("the body of `%s`'s `{for %s in …}` is not one element with a closing tag: add the button yourself", page, o.InFor)
		}
	} else if err := addToView(e, m, page, snippet("msg_button", d), ""); err != nil {
		return nil, err
	}
	// the test presses the button, then reads again what the arm may change: the row a
	// parameter names (inserted for the test), else the page; what to compare is the author's
	tb := newTestBuilder(p, page, imports).keyedFor(pg)
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
			if loop != nil { // which row the loop shows is the page's logic: the author's to say
				v = "?" + LowerFirst(name) + UpperFirst(f.Name)
			}
			args = append(args, f.Name+": "+v)
		}
		click = "(" + name + " { " + strings.Join(args, ", ") + " })"
	}
	var loopNote []string
	if loop != nil {
		var holes []string
		for _, f := range fields {
			holes = append(holes, "?"+LowerFirst(name)+UpperFirst(f.Name))
		}
		loopNote = []string{
			"The button is in the view's `{for " + o.InFor + " in " + sliceSpan(m.Syntax.Lines, loop.List.GetSpan()) + " …}`: insert rows so that the",
			"signed-in user sees the row this clicks, and fill " + strings.Join(holes, ", ") + " with that row's values.",
		}
		if len(holes) == 0 {
			loopNote[1] = "signed-in user sees a row to click."
		}
	}
	changed := "changed (`after`, the row read again), as `{ answer: r.answer, … } == { answer: Rerendered, … }`."
	if reread == "" { // the page opened again, whatever its answer: an arm may delete the page's row
		reread = "let after <- Test.open (" + pv + ")"
		changed = "changed (`after`: the page opened again, `after.answer` Shown, or PageMissing when the arm\n" +
			"deleted the page's row, and `after.doc` its HTML), as\n" +
			"`{ answer: r.answer, … } == { answer: Rerendered, … }`."
	}
	lines := append(tb.lines, "let _ <- Test.get ("+pv+")", "let r <- Test.click "+click, reread)
	if d.When != "" { // what makes the condition hold is the page's logic: the author's to set up
		loopNote = append(loopNote, "The button is shown only when `"+d.When+"`: insert the rows that make it hold",
			"on the page this test opens, or the click finds no button.")
	}
	comment := append(loopNote, []string{
		"Fill ?expected with what pressing " + name + " must do, so that a test fails when the arm",
		"does something else: its answer (`r.answer`: Rerendered, Redirected …) and what it",
	}...)
	comment = append(comment, strings.Split(changed, "\n")...)
	test := TestData{Name: quote("pressing " + name), Lines: lines, Result: "?expected", Comment: comment}
	if err := addTests(e, m, []TestData{test}); err != nil {
		return nil, err
	}
	addImports(e, m.Syntax, imports.list())
	return []Edit{{Path: m.File, Body: e.result()}}, nil
}

// validID reports whether s can be an element's id written by a generator: a letter, then
// letters, digits, `-` and `_`.
func validID(s string) bool {
	for i, r := range s {
		ok := r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || i > 0 && (r >= '0' && r <= '9' || r == '-' || r == '_')
		if !ok {
			return false
		}
	}
	return s != ""
}
