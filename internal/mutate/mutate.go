// Package mutate collects the mutants of a program: the curated, well-typed changes to its
// typed IR that `alder test --mutate` compiles into the test build and runs the suite against.
// Each operator is named by the behaviour it breaks; a mutant its tests do not notice is a test
// that is missing.
//
// Collect walks every non-test module in IR order: per module, a page's `requires` first, then
// its tables (field by field), then its values in declaration order, each body in pre-order.
// Mutant ids are 1-based in that order, so they are stable across runs of the same program; an
// edit renumbers the mutants after it.
//
// Never mutated: test bodies, fake entries, `view`, components and any value whose result is
// Html, HTML nodes inside other values, brands' `where` checks, the stdlib. A node Lower shares
// between several places (a record field's default filled into a literal, a match's shared
// fallback) is mutated once, at its first occurrence; a default is mutated only as a schema
// mutant (`default changed`).
package mutate

import (
	"strconv"
	"strings"

	"alder/internal/ir"
	"alder/internal/types"
)

// The operator kinds.
const (
	RequiresDropped    = "requires dropped"
	GuardDropped       = "guard dropped"
	WhereDropped       = "where dropped"
	OrderByReversed    = "orderBy reversed"
	LimitDropped       = "limit dropped"
	OffsetDropped      = "offset dropped"
	WriteReplaced      = "write replaced"
	UpdateFieldDropped = "update field dropped"
	RedirectRerender   = "redirect → rerender"
	RejectRerender     = "reject → rerender"
	ArmSwapped         = "arm swapped"
	ConditionNegated   = "condition negated"
	OperatorFlipped    = "operator flipped"
	BoolFlipped        = "bool flipped"
	DefaultChanged     = "default changed"
	CascadeRestrict    = "cascade ↔ restrict"
)

// Mutant is one mutation point. Module and Decl name where it is (Decl is the value's name,
// "requires" for a page's requires, the table's name for a schema mutant) and Line its source
// line; Before and After are one-line renderings of the code and of the change (elided at 80
// columns; After describes the change where a rendering would mislead).
//
// What the mutant does, by kind:
//   - Node set (every kind but requires dropped and cascade ↔ restrict): the expression Node, a
//     node of the program's IR (compare by pointer), is replaced by Replacement, an expression
//     of the same type built only from existing IR node kinds. Replacement never contains Node
//     itself; it may share Node's subexpressions (which may carry mutants of their own). A Node
//     may occur in several places of the IR (a shared default): every occurrence is replaced.
//   - requires dropped: Page is the page and Field its `requires` field (a brand over Text);
//     Node is nil and Replacement is the placeholder the field gets when the session lacks it
//     (a Text Lit "mutant", the brand being erased).
//   - default changed: Node is the table field's Default expression (the one node every record
//     literal that omits the field shares) and Replacement the changed literal; Table is a clone
//     of the table whose field Field has that Default (and DefaultLit).
//   - cascade ↔ restrict: Node is nil; Table is a clone of the table whose OnDelete rule for
//     @ref field Field is the other one. Gen renders the mutant's table descriptor from it.
type Mutant struct {
	ID                 int
	Module, Decl, Kind string
	Line               int
	Before, After      string
	Node               ir.Expr
	Replacement        ir.Expr
	Page               *ir.Page
	Table              *ir.Table
	Field              string
}

// Suppressed is a mutant the mutator refused: it is provably equivalent, or cannot be made
// well-typed (Why says which).
type Suppressed struct {
	Kind, Module, Decl string
	Line               int
	Why                string
}

// Collect returns the program's mutants, ids assigned, and the refused ones.
func Collect(p *ir.Program) ([]Mutant, []Suppressed) {
	c := &collector{p: p, seen: map[ir.Expr]bool{}}
	c.defaultUses = countDefaults(p)
	pages := map[string]*ir.Page{}
	for _, pg := range p.Pages {
		pages[pg.Module] = pg
	}
	for _, m := range p.Modules {
		c.module = m.Name
		if pg := pages[m.Name]; pg != nil && pg.Requires != "" {
			c.requires(pg)
		}
		for _, t := range p.Tables {
			if t.Module == m.Name {
				c.table(t)
			}
		}
		for _, v := range m.Values {
			if v.IsComponent || isHtml(v.Type) || (pages[m.Name] != nil && pages[m.Name].View == v) {
				continue
			}
			c.decl = v.Name
			c.walk(v.Body)
		}
	}
	for i := range c.out {
		c.out[i].ID = i + 1
	}
	return c.out, c.supp
}

type collector struct {
	p            *ir.Program
	module, decl string
	seen         map[ir.Expr]bool
	defaultUses  map[ir.Expr]int // a table field's Default → its occurrences in expressions
	out          []Mutant
	supp         []Suppressed
}

func (c *collector) add(kind string, line int, node, repl ir.Expr, before, after string) {
	c.out = append(c.out, Mutant{Module: c.module, Decl: c.decl, Kind: kind, Line: line,
		Before: before, After: after, Node: node, Replacement: repl})
}

func (c *collector) suppress(kind string, line int, why string) {
	c.supp = append(c.supp, Suppressed{Kind: kind, Module: c.module, Decl: c.decl, Line: line, Why: why})
}

func (c *collector) show(e ir.Expr) string { return line(c.module, e, id) }

func id(s string) string { return s }

func wrapped(prefix, suffix string) func(string) string {
	return func(s string) string { return prefix + s + suffix }
}

// ---- requires and the schema

func (c *collector) requires(pg *ir.Page) {
	c.decl = "requires"
	line := pg.RequiresPos.Line
	var field *ir.FieldT
	if c.p.Session != nil {
		for i := range c.p.Session.Fields {
			if c.p.Session.Fields[i].Name == pg.Requires {
				field = &c.p.Session.Fields[i]
			}
		}
	}
	if field == nil || field.Brand == nil || !isText(field.Brand.Base) {
		c.suppress(RequiresDropped, line, "field is not a Text brand")
		return
	}
	placeholder := field.Brand.Name + ` "mutant"`
	if field.Brand.Module != c.module {
		placeholder = field.Brand.Module + "." + placeholder
	}
	c.out = append(c.out, Mutant{Module: c.module, Decl: c.decl, Kind: RequiresDropped, Line: line,
		Before: "requires " + pg.Requires,
		After:  "served without " + pg.Requires + " (" + pg.Requires + " = " + placeholder + ")",
		Page:   pg, Field: pg.Requires,
		Replacement: &ir.Lit{Typed: ir.Typed{Ty: types.Text}, Kind: ir.LitText, Text: "mutant"}})
}

func (c *collector) table(t *ir.Table) {
	c.decl = t.Name
	for i, f := range t.Record.Fields {
		if f.Default != nil {
			c.defaultChanged(t, i)
		}
		if ref := t.Refs[f.Name]; ref != "" {
			c.cascade(t, f, ref)
		}
	}
}

func (c *collector) defaultChanged(t *ir.Table, i int) {
	f := t.Record.Fields[i]
	line := f.Pos.Line
	var repl *ir.Lit
	switch d := f.Default.(type) {
	case *ir.Lit:
		r := *d
		switch d.Kind {
		case ir.LitBool:
			r.Bool = !d.Bool
		case ir.LitInt:
			r.Int = d.Int + 1
		case ir.LitFloat:
			r.Float = d.Float + 1
		case ir.LitText:
			r.Text = "x"
			if d.Text == "x" {
				r.Text = "y"
			}
		default:
			c.suppress(DefaultChanged, line, "a Unit default")
			return
		}
		repl = &r
	case *ir.Ctor:
		if d.U.Module == "Prelude" && d.U.Name == "Maybe" {
			c.suppress(DefaultChanged, line, "a Maybe default")
		} else {
			c.suppress(DefaultChanged, line, "a constructor default")
		}
		return
	default:
		c.suppress(DefaultChanged, line, "not a literal default")
		return
	}
	if c.defaultUses[f.Default] == 0 {
		c.suppress(DefaultChanged, line, "no Db.insert omits it: the default never applies")
		return
	}
	clone := cloneTable(t)
	clone.Record.Fields[i].Default = repl
	litSQL := sqlLit(repl)
	clone.Record.Fields[i].DefaultLit = &litSQL
	c.out = append(c.out, Mutant{Module: c.module, Decl: c.decl, Kind: DefaultChanged, Line: line,
		Before: f.Name + " = " + lit(f.Default.(*ir.Lit)), After: f.Name + " = " + lit(repl),
		Node: f.Default, Replacement: repl, Table: clone, Field: f.Name})
}

func (c *collector) cascade(t *ir.Table, f ir.FieldT, ref string) {
	line := f.Pos.Line
	rule := t.OnDelete[f.Name]
	other := ""
	switch rule {
	case "":
		other = "cascade"
	case "cascade":
		other = ""
	default:
		c.suppress(CascadeRestrict, line, "a @"+rule+" rule has no paired rule")
		return
	}
	clone := cloneTable(t)
	clone.OnDelete = map[string]string{}
	for k, v := range t.OnDelete {
		if k != f.Name {
			clone.OnDelete[k] = v
		}
	}
	if other != "" {
		clone.OnDelete[f.Name] = other
	}
	if len(clone.OnDelete) == 0 {
		clone.OnDelete = nil
	}
	show := func(r string) string {
		if r == "" {
			r = "restrict"
		}
		return f.Name + " @ref " + ref[strings.LastIndexByte(ref, '.')+1:] + " @" + r
	}
	c.out = append(c.out, Mutant{Module: c.module, Decl: c.decl, Kind: CascadeRestrict, Line: line,
		Before: show(rule), After: show(other), Table: clone, Field: f.Name})
}

// cloneTable copies t with its own Record (and Fields), so a schema mutant can change them.
func cloneTable(t *ir.Table) *ir.Table {
	c := *t
	rec := *t.Record
	rec.Fields = append([]ir.FieldT(nil), t.Record.Fields...)
	c.Record = &rec
	return &c
}

// sqlLit is a changed default literal as SQL (ir.SQLLiteral's forms).
func sqlLit(l *ir.Lit) string {
	switch l.Kind {
	case ir.LitBool:
		if l.Bool {
			return "1"
		}
		return "0"
	case ir.LitInt:
		return strconv.FormatInt(l.Int, 10)
	case ir.LitFloat:
		s := strconv.FormatFloat(l.Float, 'g', -1, 64)
		if !strings.ContainsAny(s, ".eEn") {
			s += ".0"
		}
		return s
	}
	return "'" + strings.ReplaceAll(l.Text, "'", "''") + "'"
}

// countDefaults counts, for every app table field's Default, its occurrences in every
// expression of the program (values, components, tests, fakes): a record literal that omits the
// field holds that very node.
func countDefaults(p *ir.Program) map[ir.Expr]int {
	n := map[ir.Expr]int{}
	for _, t := range p.Tables {
		for _, f := range t.Record.Fields {
			if f.Default != nil {
				n[f.Default] = 0
			}
		}
	}
	var visit func(e ir.Expr)
	visit = func(e ir.Expr) {
		if isNil(e) {
			return
		}
		if _, ok := n[e]; ok {
			n[e]++
			return
		}
		for _, k := range kids(e) {
			visit(k)
		}
	}
	for _, m := range p.Modules {
		for _, v := range m.Values {
			visit(v.Body)
		}
		for _, v := range m.Components {
			visit(v.Body)
		}
		for _, t := range m.Tests {
			visit(t.Body)
		}
	}
	for _, f := range p.Fakes {
		for _, v := range f.Entries {
			visit(v.Body)
		}
	}
	return n
}

// ---- expressions

// walk visits e in pre-order: its mutants, then its subexpressions'. HTML is not entered, nor a
// record field holding its type's default.
func (c *collector) walk(e ir.Expr) {
	if isNil(e) || c.seen[e] || isHtmlNode(e) {
		return
	}
	c.seen[e] = true
	switch x := e.(type) {
	case *ir.If:
		c.guardIf(x)
		c.negate(x)
	case *ir.Match:
		c.guardMatch(x)
		c.swap(x)
	case *ir.Op:
		c.flip(x)
	case *ir.Lit:
		if x.Kind == ir.LitBool {
			r := *x
			r.Bool = !x.Bool
			c.add(BoolFlipped, x.Pos.Line, x, &r, lit(x), lit(&r))
		}
	case *ir.DbSelect:
		c.selectOps(x)
	case *ir.EffBind:
		c.write(x)
	case *ir.Update:
		c.updateFields(x)
	case *ir.Ctor:
		c.nav(x)
	}
	for _, k := range kids(e) {
		if r, ok := e.(*ir.Record); ok && isDefaultValue(r, k) {
			continue
		}
		c.walk(k)
	}
}

// isGuard reports whether e (under any Lets) is Task.notFound, Query.notFound or Task.fail,
// and which.
func isGuard(e ir.Expr) (string, bool) {
	for {
		l, ok := e.(*ir.Let)
		if !ok {
			break
		}
		e = l.Body
	}
	switch e.(type) {
	case *ir.EffNotFound:
		return "notFound", true
	case *ir.EffFail:
		return "fail", true
	}
	return "", false
}

func (c *collector) guardIf(x *ir.If) {
	gt, okT := isGuard(x.Then)
	ge, okE := isGuard(x.Else)
	if okT == okE {
		return
	}
	other, g := x.Else, gt
	if okE {
		other, g = x.Then, ge
	}
	c.add(GuardDropped, x.Pos.Line, x, other, c.show(x), "the "+g+" arm dropped")
}

// arm is one arm of a Match: a Case (with its Binds) or the Default.
type arm struct {
	c    *ir.Case
	body ir.Expr
}

func arms(m *ir.Match) []arm {
	var out []arm
	for i := range m.Cases {
		out = append(out, arm{&m.Cases[i], m.Cases[i].Body})
	}
	if !isNil(m.Default) {
		out = append(out, arm{nil, m.Default})
	}
	return out
}

// lifted is an arm's body moved out of its case: the Lets of names the body never uses peeled
// off; ok is false when it still uses a name the case binds (it cannot leave the case).
func (a arm) lifted() (ir.Expr, bool) {
	if a.c == nil {
		return a.body, true
	}
	body := a.body
	for {
		l, isLet := body.(*ir.Let)
		if !isLet || uses(l.Body, l.Name) > 0 {
			break
		}
		if v, isVar := l.Value.(*ir.Var); !isVar || v.Kind != ir.VarLocal {
			break
		}
		body = l.Body
	}
	for _, b := range a.c.Binds {
		if uses(body, b.Name) > 0 {
			return nil, false
		}
	}
	return body, true
}

func (c *collector) guardMatch(x *ir.Match) {
	as := arms(x)
	if len(as) != 2 {
		return
	}
	g0, ok0 := isGuard(as[0].body)
	g1, ok1 := isGuard(as[1].body)
	if ok0 == ok1 {
		return
	}
	other, g := as[1], g0
	if ok1 {
		other, g = as[0], g1
	}
	body, ok := other.lifted()
	if !ok {
		c.suppress(GuardDropped, x.Pos.Line, "unscopable: the other arm uses what the match binds")
		return
	}
	c.add(GuardDropped, x.Pos.Line, x, body, c.show(x), "the "+g+" arm dropped")
}

func (c *collector) swap(x *ir.Match) {
	as := arms(x)
	if len(as) != 2 {
		return
	}
	b0, ok0 := as[0].lifted()
	b1, ok1 := as[1].lifted()
	if !ok0 || !ok1 {
		c.suppress(ArmSwapped, x.Pos.Line, "unscopable: an arm uses what the match binds")
		return
	}
	if text(b0) == text(b1) {
		c.suppress(ArmSwapped, x.Pos.Line, "identical bodies")
		return
	}
	r := *x
	r.Cases = append([]ir.Case(nil), x.Cases...)
	set := func(i int, body ir.Expr) {
		if i < len(r.Cases) {
			r.Cases[i].Body = body
		} else {
			r.Default = body
		}
	}
	set(0, b1)
	set(1, b0)
	before, after := c.show(x), c.show(&r)
	if before == after { // the bodies were elided alike: say what changed
		p := &printer{home: c.module, elided: map[ir.Expr]bool{}}
		tag := func(a arm) string {
			if a.c == nil {
				return "_"
			}
			pat, _ := p.pattern(x, *a.c)
			return pat
		}
		after = "the " + tag(as[0]) + " and " + tag(as[1]) + " bodies swapped"
	}
	c.add(ArmSwapped, x.Pos.Line, x, &r, before, after)
}

var notFn = &ir.Var{Typed: ir.Typed{Ty: &types.Fun{Params: []types.Type{types.Bool}, Result: types.Bool}},
	Kind: ir.VarStd, Module: "Prelude", Name: "not"}

func (c *collector) negate(x *ir.If) {
	if _, isLit := x.Cond.(*ir.Lit); isLit {
		c.suppress(ConditionNegated, x.Pos.Line, "a literal condition")
		return
	}
	r := *x
	r.Cond = &ir.Call{Typed: ir.Typed{Ty: types.Bool}, Fn: notFn, Args: []ir.Expr{x.Cond}}
	c.add(ConditionNegated, x.Pos.Line, x, &r,
		line(c.module, x.Cond, wrapped("if ", "")), line(c.module, x.Cond, argument("if not ", x.Cond, "")))
}

var flips = map[string]string{"==": "!=", "!=": "==", "<": "<=", "<=": "<", ">": ">=", ">=": ">", "&&": "||", "||": "&&"}

func (c *collector) flip(x *ir.Op) {
	to, ok := flips[x.Kind]
	if !ok {
		return
	}
	if text(x.L) == text(x.R) {
		c.suppress(OperatorFlipped, x.Pos.Line, "both operands are the same expression")
		return
	}
	r := *x
	r.Kind = to
	c.add(OperatorFlipped, x.Pos.Line, x, &r, c.show(x), c.show(&r))
}

func (c *collector) selectOps(x *ir.DbSelect) {
	line0 := x.Pos.Line
	at := func(e ir.Expr) int {
		switch y := e.(type) {
		case *ir.Cond:
			if y.Pos.Line > 0 {
				return y.Pos.Line
			}
		case *ir.Ord:
			if y.Pos.Line > 0 {
				return y.Pos.Line
			}
		}
		return line0
	}
	for i, w := range x.Where {
		r := *x
		r.Where = append(append([]ir.Expr(nil), x.Where[:i]...), x.Where[i+1:]...)
		c.add(WhereDropped, at(w), x, &r, line(c.module, w, argument("Db.where ", w, "")), "the condition dropped")
	}
	for i, o := range x.Order {
		ord, ok := o.(*ir.Ord)
		if !ok {
			continue
		}
		if x.Terminal == ir.SelFirst && whereOnID(x) {
			c.suppress(OrderByReversed, at(o), "the where pins one row by its id")
			continue
		}
		rev := *ord
		rev.Desc = !ord.Desc
		r := *x
		r.Order = append([]ir.Expr(nil), x.Order...)
		r.Order[i] = &rev
		c.add(OrderByReversed, at(o), x, &r, "Db.orderBy "+c.show(ord), "Db.orderBy "+c.show(&rev))
	}
	if x.Limit != nil {
		r := *x
		r.Limit = nil
		c.add(LimitDropped, line0, x, &r, line(c.module, x.Limit, argument("Db.limit ", x.Limit, "")), "no limit")
	}
	if x.Offset != nil {
		r := *x
		r.Offset = nil
		c.add(OffsetDropped, line0, x, &r, line(c.module, x.Offset, argument("Db.offset ", x.Offset, "")), "no offset")
	}
}

// whereOnID reports whether one of x's own conditions is the equality of its table's id.
func whereOnID(x *ir.DbSelect) bool {
	for _, w := range x.Where {
		if cd, ok := w.(*ir.Cond); ok && cd.Kind == "eq" && cd.Col != nil && cd.Col.Field == x.Table.IDField {
			return true
		}
	}
	return false
}

func (c *collector) write(x *ir.EffBind) {
	var tb *ir.Table
	switch w := x.Effect.(type) {
	case *ir.DbInsert:
		tb = w.Table
	case *ir.DbUpdate:
		tb = w.Table
	case *ir.DbDelete:
		tb = w.Table
	default:
		return
	}
	result := types.Type(types.Unit)
	if app, ok := x.Effect.Type().(*types.App); ok && len(app.Args) == 1 {
		result = app.Args[0]
	}
	var value *ir.Lit
	shown := "()"
	if _, isInsert := x.Effect.(*ir.DbInsert); isInsert {
		if !isText(result) {
			c.suppress(WriteReplaced, x.Pos.Line, "the table's id is not Text")
			return
		}
		value = &ir.Lit{Typed: ir.Typed{Ty: result}, Kind: ir.LitText, Text: "mutant"}
		shown = `"mutant"`
		for _, f := range tb.Record.Fields {
			if f.Name == tb.IDField && f.Brand != nil {
				shown = "(" + qualify(c.module, f.Brand.Module, f.Brand.Name) + ` "mutant")`
			}
		}
	} else {
		value = &ir.Lit{Typed: ir.Typed{Ty: result}, Kind: ir.LitUnit}
	}
	repl := &ir.EffPure{Typed: ir.Typed{Ty: x.Effect.Type()}, Kind: ir.EffTask, Value: value}
	name := "_"
	if x.Param != nil {
		name = x.Param.Name
	}
	c.add(WriteReplaced, x.Pos.Line, x.Effect, repl,
		line(c.module, x.Effect, wrapped("let "+name+" <- ", "")), "let "+name+" <- Task.succeed "+shown)
}

func qualify(home, module, name string) string {
	if module == home {
		return name
	}
	return module + "." + name
}

func (c *collector) updateFields(x *ir.Update) {
	if len(x.Fields) < 2 {
		return
	}
	for i := range x.Fields {
		r := *x
		r.Fields = append(append([]ir.FieldInit(nil), x.Fields[:i]...), x.Fields[i+1:]...)
		c.add(UpdateFieldDropped, x.Pos.Line, x, &r, c.show(x), c.show(&r))
	}
}

func (c *collector) nav(x *ir.Ctor) {
	if x.U == nil || x.U.Module != "Prelude" || x.U.Name != "Nav" {
		return
	}
	kind := ""
	switch x.Name {
	case "Redirect":
		kind = RedirectRerender
	case "Reject":
		kind = RejectRerender
	default:
		return
	}
	r := &ir.Ctor{Typed: x.Typed, Pos: x.Pos, U: x.U, Name: "Rerender"}
	c.add(kind, x.Pos.Line, x, r, c.show(x), "Rerender")
}

// ---- helpers

func isText(t types.Type) bool {
	con, ok := t.(*types.Con)
	return ok && con.Name == "Text"
}

// isHtml reports whether a value's final result is Html.
func isHtml(t types.Type) bool {
	for {
		fn, ok := t.(*types.Fun)
		if !ok {
			break
		}
		t = fn.Result
	}
	app, ok := t.(*types.App)
	return ok && app.Con == "Html"
}

func isHtmlNode(e ir.Expr) bool {
	switch e.(type) {
	case *ir.Elem, *ir.Comp, *ir.HText, *ir.HExpr, *ir.HFor, *ir.HIf, *ir.HMatch, *ir.Input, *ir.FormEl:
		return true
	}
	return false
}

// isDefaultValue reports whether k is a field value of r that its record type's default filled.
func isDefaultValue(r *ir.Record, k ir.Expr) bool {
	for _, f := range r.Fields {
		if f.Value == k && isDefault(r.T, f) {
			return true
		}
	}
	return false
}

// isNil reports whether e is nil or a typed nil pointer (a Comp's nil Props).
func isNil(e ir.Expr) bool {
	if e == nil {
		return true
	}
	if r, ok := e.(*ir.Record); ok && r == nil {
		return true
	}
	return false
}

// uses counts the free occurrences of local name in e.
func uses(e ir.Expr, name string) int {
	switch x := e.(type) {
	case nil:
		return 0
	case *ir.Var:
		if x.Kind == ir.VarLocal && x.Name == name {
			return 1
		}
		return 0
	case *ir.Let:
		n := uses(x.Value, name)
		if x.Name != name {
			n += uses(x.Body, name)
		}
		return n
	case *ir.Lambda:
		for _, p := range x.Params {
			if p.Name == name {
				return 0
			}
		}
		return uses(x.Body, name)
	case *ir.Match:
		return usesMatch(x, name)
	case *ir.HMatch:
		return usesMatch((*ir.Match)(x), name)
	case *ir.HFor:
		n := uses(x.List, name)
		if x.Var != name {
			n += uses(x.Body, name)
		}
		return n
	case *ir.EffBind:
		n := uses(x.Effect, name)
		if x.Param == nil || x.Param.Name != name {
			n += uses(x.Body, name)
		}
		return n
	}
	n := 0
	for _, k := range kids(e) {
		if !isNil(k) {
			n += uses(k, name)
		}
	}
	return n
}

func usesMatch(x *ir.Match, name string) int {
	n := uses(x.Subject, name)
	if !isNil(x.Default) {
		n += uses(x.Default, name)
	}
cases:
	for _, c := range x.Cases {
		for _, b := range c.Binds {
			if b.Name == name {
				continue cases
			}
		}
		n += uses(c.Body, name)
	}
	return n
}

// kids lists the direct subexpressions of e in IR order (nil entries possible), as gen's
// children does.
func kids(e ir.Expr) []ir.Expr {
	switch x := e.(type) {
	case *ir.Call:
		return append([]ir.Expr{x.Fn}, x.Args...)
	case *ir.Partial:
		return append([]ir.Expr{x.Fn}, x.Given...)
	case *ir.Lambda:
		return []ir.Expr{x.Body}
	case *ir.Record:
		return fieldValues(x.Fields)
	case *ir.Update:
		return append([]ir.Expr{x.Base}, fieldValues(x.Fields)...)
	case *ir.Field:
		return []ir.Expr{x.Expr}
	case *ir.Ctor:
		return []ir.Expr{x.Payload}
	case *ir.List:
		return x.Elems
	case *ir.PageRef:
		return []ir.Expr{x.Params}
	case *ir.Match:
		return matchKids(x)
	case *ir.HMatch:
		return matchKids((*ir.Match)(x))
	case *ir.If:
		return []ir.Expr{x.Cond, x.Then, x.Else}
	case *ir.Let:
		return []ir.Expr{x.Value, x.Body}
	case *ir.Op:
		return []ir.Expr{x.L, x.R}
	case *ir.Str:
		return x.Parts
	case *ir.Widen:
		return []ir.Expr{x.Expr}
	case *ir.Elem:
		return append(attrValues(x.Attrs), x.Children...)
	case *ir.Comp:
		out := x.Children
		if x.Props != nil {
			out = append([]ir.Expr{x.Props}, out...)
		}
		return out
	case *ir.HExpr:
		return []ir.Expr{x.Expr}
	case *ir.HFor:
		return []ir.Expr{x.List, x.Body}
	case *ir.HIf:
		return []ir.Expr{x.Cond, x.Then, x.Else}
	case *ir.Input:
		return []ir.Expr{x.LabelExpr, x.Value}
	case *ir.FormEl:
		return append(attrValues(x.Attrs), x.Children...)
	case *ir.EffPure:
		return []ir.Expr{x.Value}
	case *ir.EffBind:
		return []ir.Expr{x.Effect, x.Body}
	case *ir.EffMap:
		return []ir.Expr{x.Fn, x.Effect}
	case *ir.EffAll:
		return []ir.Expr{x.Effects}
	case *ir.EffFail:
		return []ir.Expr{x.Message}
	case *ir.DbGet:
		return []ir.Expr{x.ID}
	case *ir.DbSelect:
		out := append([]ir.Expr{x.Base}, x.Where...)
		return append(append(out, x.Order...), x.Limit, x.Offset)
	case *ir.DbRun:
		return []ir.Expr{x.Select}
	case *ir.DbInsert:
		return []ir.Expr{x.Row}
	case *ir.DbUpdate:
		return []ir.Expr{x.ID, x.Fn}
	case *ir.DbDelete:
		return []ir.Expr{x.ID}
	case *ir.Cond:
		return append([]ir.Expr{x.Value}, x.Subs...)
	case *ir.CapCall:
		return x.Args
	case *ir.TestRun:
		return []ir.Expr{x.Task}
	case *ir.TestEq:
		return []ir.Expr{x.Left, x.Right}
	case *ir.TestLogin:
		return []ir.Expr{x.Session}
	case *ir.TestGet:
		return []ir.Expr{x.Params}
	case *ir.TestSubmit:
		return []ir.Expr{x.Params, x.Record}
	case *ir.TestSubmitRaw:
		return []ir.Expr{x.Params, x.Fields}
	case *ir.TestClick:
		return []ir.Expr{x.Params}
	}
	return nil
}

func matchKids(x *ir.Match) []ir.Expr {
	out := []ir.Expr{x.Subject}
	for _, c := range x.Cases {
		out = append(out, c.Body)
	}
	return append(out, x.Default)
}

func fieldValues(fs []ir.FieldInit) []ir.Expr {
	var out []ir.Expr
	for _, f := range fs {
		out = append(out, f.Value)
	}
	return out
}

func attrValues(as []ir.HAttr) []ir.Expr {
	var out []ir.Expr
	for _, a := range as {
		out = append(out, a.Value)
	}
	return out
}
