// Package mutate collects the mutants of a program: the curated, well-typed changes to its
// typed IR that `alder test --mutate` compiles into the test build and runs the suite against.
// Each operator is named by the behaviour it breaks; a mutant its tests do not notice is a test
// that is missing.
//
// Collect walks every non-test module in IR order: per module, a page's `requires` first, then
// its tables (field by field), then its values in declaration order, each body in pre-order.
// Mutant ids are 1-based in that order, so they are stable across runs of the same program; an
// edit renumbers the mutants after it. Module order is the program's (names.Program.Order):
// import-topological, dependencies first, roots alphabetical. So adding an import can move a
// module, renumbering the mutants of modules the edit never touched.
//
// Never mutated: test bodies, fake entries, `view`, components and any value whose result is
// Html, HTML nodes inside other values, brands' `where` checks, the stdlib. A node Lower shares
// between several places (a record field's default filled into a literal, a match's shared
// fallback) is mutated once, at its first occurrence; a default is mutated only as a schema
// mutant (`default changed`). A file page's `view` returns File, not Html, so it is mutated like
// any value.
package mutate

import (
	"strconv"
	"strings"
	"unicode/utf8"

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
	NotDropped         = "not dropped"
	SessionPutDropped  = "session put dropped"
	DefaultChanged     = "default changed"
	CascadeRestrict    = "cascade ↔ restrict"
	ScopeDropped       = "scope dropped"
	RoleDropped        = "role dropped"
	MembershipDropped  = "membership dropped"
)

// Mutant is one mutation point. Module and Decl name where it is (Decl is the value's name,
// "requires" for a page's requires, the table's name for a schema mutant), File is the module's
// source file and Line and Col the mutated code's position in it (for a schema mutant, the
// field's); Before and After are one-line renderings of the code and of the change (elided at 80
// columns; After describes the change where a rendering would mislead).
//
// What the mutant does, by kind:
//   - Node set (every kind but requires dropped, role dropped and cascade ↔ restrict): the expression Node, a
//     node of the program's IR (compare by pointer), is replaced by Replacement, an expression
//     of the same type built only from existing IR node kinds. Replacement never contains Node
//     itself; it may share Node's subexpressions (which may carry mutants of their own). A Node
//     may occur in several places of the IR (a shared default): every occurrence is replaced.
//   - requires dropped: Page is the page and Field its `requires` field (a brand over Text);
//     Node is nil and Replacement is the placeholder the field gets when the session lacks it
//     (a Text Lit "mutant", the brand being erased).
//   - role dropped: Page is the page whose `requires user, role R | …` is dropped to `requires
//     user` (any signed-in user is served); Node and Replacement are nil.
//   - scope dropped: Node is a scoped Db node and Replacement its unscoped twin: `Db.mine T` →
//     `Db.from T` (a DbSelect); `Db.getMine T id` → `Db.get T id` bound and matched, Nothing →
//     Query.notFound; `Db.updateMine`/`Db.deleteMine` → `Db.get T id`, then on Just the unscoped
//     write, on Nothing Task.notFound (not found only when the row is missing). `Db.insertMine`
//     is skipped: the owner comes from the session either way. A `Db.updateMine`/`Db.deleteMine`
//     in a page's update whose id is `model.f.id`, where the page's load filled f with a row of
//     the same table read by `Db.getMine`, is suppressed as equivalent ("load fetched it with
//     getMine"): every message runs load first, so the id is always the user's own row.
//   - default changed: Node is the table field's Default expression (the one node every record
//     literal that omits the field shares) and Replacement the changed literal; Table is a clone
//     of the table whose field Field has that Default (and DefaultLit).
//   - membership dropped (M15): Node is nil; Field is a session field with `via`, whose
//     membership check Session.put (and Test.login) skip under the mutant (gen's session__via).
//     A scoped node on a table in a `silo` has no scope dropped: equivalent, the silo's database
//     holding the key value's rows only; nor has a @ref out of a silo cascade ↔ restrict (the
//     silo's database holds no REFERENCES into another store).
//   - cascade ↔ restrict: Node is nil; Table is a clone of the table whose OnDelete rule for
//     @ref field Field is the other one. Gen renders the mutant's table descriptor from it.
type Mutant struct {
	ID                 int
	Module, Decl, Kind string
	File               string // the module's source file, "app/Posts/Index.alder"
	Line, Col          int
	Before, After      string
	Node               ir.Expr
	Replacement        ir.Expr
	Page               *ir.Page
	Table              *ir.Table
	Field              string
	// Arms, for a Match's arm swapped, are the indices of the swapped arms (i, i+1); for a
	// Match's guard dropped, the guard arm's and the other's (the cases in order, then the
	// default), the Replacement being the other arm's body lifted out of its case.
	Arms []int
}

// Suppressed is a mutant the mutator refused. Category is Equivalent for the spec's structural
// suppression rules (the mutant would provably be the same program: a literal condition, identical
// arms, …), Skipped where the operator does not apply (no placeholder, no paired rule, a Maybe
// default, an arm that cannot be scoped, another operator making the same program, …); the
// report counts the two apart. Why says which rule.
type Suppressed struct {
	Kind, Module, Decl string
	Line               int
	Category           string
	Why                string
}

// The categories of a Suppressed.
const (
	Equivalent = "equivalent"
	Skipped    = "skipped"
)

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
		if pg := pages[m.Name]; pg != nil && len(pg.Requires) > 0 {
			c.requires(pg)
			if pg.Roles != nil {
				c.roles(pg)
			}
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
			c.loaded, c.modelParam = nil, ""
			if pg := pages[m.Name]; pg != nil && pg.Update == v && len(v.Params) >= 2 && binds(v.Body, v.Params[1].Name) == 0 && !putsSession(v.Body) {
				c.loaded, c.modelParam = loadedMine(pg), v.Params[1].Name
			}
			c.walk(v.Body)
		}
	}
	c.memberships()
	for i := range c.out {
		c.out[i].ID = i + 1
		c.out[i].File = "app/" + strings.ReplaceAll(c.out[i].Module, ".", "/") + ".alder"
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
	// loaded and modelParam, while the walk is in a page's update: the model fields its load
	// fills with a row read by Db.getMine (loadedMine), and update's model parameter
	loaded     map[string]*ir.Table
	modelParam string
}

func (c *collector) add(kind string, at ir.Pos, node, repl ir.Expr, before, after string) {
	c.out = append(c.out, Mutant{Module: c.module, Decl: c.decl, Kind: kind, Line: at.Line, Col: at.Col,
		Before: before, After: after, Node: node, Replacement: repl})
}

// suppress refuses an equivalent mutant (the spec's suppression rules).
func (c *collector) suppress(kind string, line int, why string) {
	c.supp = append(c.supp, Suppressed{Kind: kind, Module: c.module, Decl: c.decl, Line: line, Category: Equivalent, Why: why})
}

// skip records that an operator does not apply here.
func (c *collector) skip(kind string, line int, why string) {
	c.supp = append(c.supp, Suppressed{Kind: kind, Module: c.module, Decl: c.decl, Line: line, Category: Skipped, Why: why})
}

func (c *collector) show(e ir.Expr) string { return line(c.module, e, id) }

func id(s string) string { return s }

func wrapped(prefix, suffix string) func(string) string {
	return func(s string) string { return prefix + s + suffix }
}

// ---- requires and the schema

// requires drops the first field of a page's `requires` (M15: Task 2 covers the others).
func (c *collector) requires(pg *ir.Page) {
	c.decl = "requires"
	line := pg.RequiresPos.Line
	req := pg.Requires[0]
	for _, k := range pg.Requires[1:] {
		if req == "user" && c.p.Via[k] != nil {
			c.suppress(RequiresDropped, line, "the page also requires "+k+", which is `via`: no session has it without a user")
			return
		}
	}
	var field *ir.FieldT
	if c.p.Session != nil {
		for i := range c.p.Session.Fields {
			if c.p.Session.Fields[i].Name == req {
				field = &c.p.Session.Fields[i]
			}
		}
	}
	if field == nil || field.Brand == nil || !isText(field.Brand.Base) {
		c.skip(RequiresDropped, line, "field is not a Text brand")
		return
	}
	placeholder := field.Brand.Name + ` "mutant"`
	if field.Brand.Module != c.module {
		placeholder = field.Brand.Module + "." + placeholder
	}
	c.out = append(c.out, Mutant{Module: c.module, Decl: c.decl, Kind: RequiresDropped, Line: line, Col: pg.RequiresPos.Col,
		Before: "requires " + req,
		After:  "served without " + req + " (" + req + " = " + placeholder + ")",
		Page:   pg, Field: req,
		Replacement: &ir.Lit{Typed: ir.Typed{Ty: types.Text}, Kind: ir.LitText, Text: "mutant"}})
}

// memberships drops each `via` field's membership check (M15): under the mutant Session.put
// (and Test.login) of the field to any value passes.
func (c *collector) memberships() {
	if c.p.Session == nil {
		return
	}
	for _, f := range c.p.Session.Fields {
		mt := c.p.Via[f.Name]
		if mt == nil {
			continue
		}
		c.module, c.decl = "App", "session"
		c.out = append(c.out, Mutant{Module: "App", Decl: "session", Kind: MembershipDropped, Line: f.Pos.Line, Col: f.Pos.Col,
			Before: "Session.put " + f.Name, After: "Session.put " + f.Name + " without the " + mt.Name + " check", Field: f.Name})
	}
}

// roles drops a page's `role R | …`: the page serves any signed-in user.
func (c *collector) roles(pg *ir.Page) {
	c.decl = "requires"
	c.out = append(c.out, Mutant{Module: c.module, Decl: c.decl, Kind: RoleDropped, Line: pg.RolesPos.Line, Col: pg.RolesPos.Col,
		Before: "requires " + strings.Join(pg.Requires, ", ") + ", role " + strings.Join(pg.Roles, " | "),
		After:  "requires " + strings.Join(pg.Requires, ", "), Page: pg})
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
			c.skip(DefaultChanged, line, "a Unit default")
			return
		}
		repl = &r
	case *ir.Ctor:
		if d.U.Module == "Prelude" && d.U.Name == "Maybe" {
			c.skip(DefaultChanged, line, "a Maybe default")
			return
		}
		// a union default: the next constructor in declaration order, the first after the last
		next := ""
		for j, ct := range d.U.Ctors {
			if ct.Name == d.Name {
				if n := d.U.Ctors[(j+1)%len(d.U.Ctors)]; n.Payload == nil {
					next = n.Name
				}
			}
		}
		if next == "" || next == d.Name || d.Payload != nil {
			c.skip(DefaultChanged, line, "no next constructor without a value")
			return
		}
		if c.defaultUses[f.Default] == 0 {
			c.suppress(DefaultChanged, line, "no Db.insert omits it: the default never applies")
			return
		}
		r := &ir.Ctor{Typed: d.Typed, Pos: d.Pos, U: d.U, Name: next}
		clone := cloneTable(t)
		clone.Record.Fields[i].Default = r
		litSQL := "'" + strings.ReplaceAll(next, "'", "''") + "'"
		clone.Record.Fields[i].DefaultLit = &litSQL
		c.out = append(c.out, Mutant{Module: c.module, Decl: c.decl, Kind: DefaultChanged, Line: line, Col: f.Pos.Col,
			Before: f.Name + " = " + d.Name, After: f.Name + " = " + next,
			Node: f.Default, Replacement: r, Table: clone, Field: f.Name})
		return
	default:
		c.skip(DefaultChanged, line, "not a literal default")
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
	c.out = append(c.out, Mutant{Module: c.module, Decl: c.decl, Kind: DefaultChanged, Line: line, Col: f.Pos.Col,
		Before: f.Name + " = " + lit(f.Default.(*ir.Lit)), After: f.Name + " = " + lit(repl),
		Node: f.Default, Replacement: repl, Table: clone, Field: f.Name})
}

func (c *collector) cascade(t *ir.Table, f ir.FieldT, ref string) {
	line := f.Pos.Line
	if k := c.siloOf(t); k != "" && c.siloOf(c.tableNamed(ref)) != k {
		c.suppress(CascadeRestrict, line, "a silo's database has no REFERENCES out of silo "+k)
		return
	}
	rule := t.OnDelete[f.Name]
	other := ""
	switch rule {
	case "":
		other = "cascade"
	case "cascade":
		other = ""
	default:
		c.skip(CascadeRestrict, line, "a @"+rule+" rule has no paired rule")
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
	c.out = append(c.out, Mutant{Module: c.module, Decl: c.decl, Kind: CascadeRestrict, Line: line, Col: f.Pos.Col,
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

// countDefaults counts, for every app table field's Default, its occurrences in the app's code
// (values and components; not tests, not fakes). Lower fills a field's default only into a
// record literal that omits the field (a Db.insert row, an `Insert T` a helper returns, …), so
// each occurrence is a literal that relies on it.
func countDefaults(p *ir.Program) map[ir.Expr]int {
	n := map[ir.Expr]int{}
	for _, t := range p.Tables {
		for _, f := range t.Record.Fields {
			if f.Default != nil {
				n[f.Default] = 0
			}
		}
	}
	seen := map[ir.Expr]bool{}
	var visit func(e ir.Expr)
	visit = func(e ir.Expr) {
		if isNil(e) {
			return
		}
		if _, isDefault := n[e]; isDefault {
			n[e]++
			return
		}
		if seen[e] {
			return
		}
		seen[e] = true
		for _, k := range ir.Children(e) {
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
			c.add(BoolFlipped, x.Pos, x, &r, lit(x), lit(&r))
		}
	case *ir.DbSelect:
		c.selectOps(x)
	case *ir.EffBind:
		c.write(x)
		c.sessionPut(x)
	case *ir.Call:
		c.notDropped(x)
	case *ir.Update:
		c.updateFields(x)
	case *ir.Ctor:
		c.nav(x)
	case *ir.DbScoped, *ir.DbGetScoped, *ir.DbUpdateScoped, *ir.DbDeleteScoped, *ir.DbInsertScoped:
		c.scope(e)
	}
	for _, k := range ir.Children(e) {
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
	c.add(GuardDropped, x.Pos, x, other, lineFirst(c.module, x, id, x.Then, x.Else), "the "+g+" arm dropped")
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
	other, g, arms := as[1], g0, []int{0, 1}
	if ok1 {
		other, g, arms = as[0], g1, []int{1, 0}
	}
	body, ok := other.lifted()
	if !ok {
		c.skip(GuardDropped, x.Pos.Line, "unscopable: the other arm uses what the match binds")
		return
	}
	c.add(GuardDropped, x.Pos, x, body, c.show(x), "the "+g+" arm dropped")
	c.out[len(c.out)-1].Arms = arms
}

// swapShown is the most columns an arm swap's Before and After take together before After names
// the swap instead of showing the whole match again.
const swapShown = 80

// swap makes one mutant per adjacent pair of arms (all arms of a match have its type).
func (c *collector) swap(x *ir.Match) {
	as := arms(x)
	for i := 0; i+1 < len(as); i++ {
		c.swapPair(x, as, i)
	}
}

func (c *collector) swapPair(x *ir.Match, as []arm, i int) {
	b0, ok0 := as[i].lifted()
	b1, ok1 := as[i+1].lifted()
	if !ok0 || !ok1 {
		c.skip(ArmSwapped, x.Pos.Line, "unscopable: an arm uses what the match binds")
		return
	}
	if text(b0) == text(b1) {
		c.suppress(ArmSwapped, x.Pos.Line, "identical bodies")
		return
	}
	r := *x
	r.Cases = append([]ir.Case(nil), x.Cases...)
	set := func(j int, body ir.Expr) {
		if j < len(r.Cases) {
			r.Cases[j].Body = body
		} else {
			r.Default = body
		}
	}
	set(i, b1)
	set(i+1, b0)
	before, after := c.show(x), c.show(&r)
	// the bodies were elided alike, or the two matches make too long a line: say what changed
	if before == after || utf8.RuneCountInString(before)+utf8.RuneCountInString(after) > swapShown {
		p := &printer{home: c.module, elided: map[ir.Expr]bool{}}
		tag := func(a arm) string {
			if a.c == nil {
				return "_"
			}
			pat, _ := p.pattern(x, *a.c)
			return pat
		}
		after = "the " + tag(as[i]) + " and " + tag(as[i+1]) + " bodies swapped"
	}
	c.add(ArmSwapped, x.Pos, x, &r, before, after)
	c.out[len(c.out)-1].Arms = []int{i, i + 1}
}

var notFn = &ir.Var{Typed: ir.Typed{Ty: &types.Fun{Params: []types.Type{types.Bool}, Result: types.Bool}},
	Kind: ir.VarStd, Module: "Prelude", Name: "not"}

func (c *collector) negate(x *ir.If) {
	if _, isLit := x.Cond.(*ir.Lit); isLit {
		c.suppress(ConditionNegated, x.Pos.Line, "a literal condition")
		return
	}
	if op, isOp := x.Cond.(*ir.Op); isOp && (op.Kind == "==" || op.Kind == "!=") {
		c.skip(ConditionNegated, x.Pos.Line, "operator flipped makes the same program")
		return
	}
	if call, isCall := x.Cond.(*ir.Call); isCall && isStd(call.Fn, "Prelude", "not") {
		c.skip(ConditionNegated, x.Pos.Line, "not dropped makes the same program")
		return
	}
	r := *x
	r.Cond = &ir.Call{Typed: ir.Typed{Ty: types.Bool}, Fn: notFn, Args: []ir.Expr{x.Cond}}
	c.add(ConditionNegated, x.Pos, x, &r,
		line(c.module, x.Cond, wrapped("if ", "")), line(c.module, x.Cond, argument("if not ", x.Cond, "")))
}

// Negating an `==` or `!=` condition is the program flipping its operator makes (`<` flips to
// `<=`, not to its negation `>=`, so an ordering condition is negated too).

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
	c.add(OperatorFlipped, x.Pos, x, &r, c.show(x), c.show(&r))
}

func (c *collector) selectOps(x *ir.DbSelect) {
	step := func(ps []ir.Pos, i int) ir.Pos {
		if i < len(ps) && ps[i].Line > 0 {
			return ps[i]
		}
		return x.Pos
	}
	at := func(p ir.Pos) ir.Pos {
		if p.Line > 0 {
			return p
		}
		return x.Pos
	}
	for i, w := range x.Where {
		r := *x
		r.Where = append(append([]ir.Expr(nil), x.Where[:i]...), x.Where[i+1:]...)
		c.add(WhereDropped, step(x.WherePos, i), x, &r, line(c.module, w, argument("Db.where ", w, "")), "the condition dropped")
	}
	for i, o := range x.Order {
		ord, ok := o.(*ir.Ord)
		if !ok {
			continue
		}
		if x.Terminal == ir.SelFirst && whereOnID(x) {
			c.suppress(OrderByReversed, step(x.OrderPos, i).Line, "the where pins one row by its id")
			continue
		}
		rev := *ord
		rev.Desc = !ord.Desc
		r := *x
		r.Order = append([]ir.Expr(nil), x.Order...)
		r.Order[i] = &rev
		c.add(OrderByReversed, step(x.OrderPos, i), x, &r, "Db.orderBy "+c.show(ord), "Db.orderBy "+c.show(&rev))
	}
	if x.Limit != nil {
		r := *x
		r.Limit = nil
		c.add(LimitDropped, at(x.LimitPos), x, &r, line(c.module, x.Limit, argument("Db.limit ", x.Limit, "")), "no limit")
	}
	if x.Offset != nil {
		r := *x
		r.Offset = nil
		c.add(OffsetDropped, at(x.OffsetPos), x, &r, line(c.module, x.Offset, argument("Db.offset ", x.Offset, "")), "no offset")
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
	case *ir.DbInsertScoped:
		tb = w.Table
	case *ir.DbUpdateScoped:
		tb = w.Table
	case *ir.DbDeleteScoped:
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
	_, isInsert := x.Effect.(*ir.DbInsert)
	_, isInsertMine := x.Effect.(*ir.DbInsertScoped)
	if isInsert || isInsertMine {
		if !isText(result) {
			c.skip(WriteReplaced, x.Pos.Line, "the table's id is not Text")
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
	c.add(WriteReplaced, x.Pos, x.Effect, repl,
		line(c.module, x.Effect, wrapped("let "+name+" <- ", "")), "let "+name+" <- Task.succeed "+shown)
}

// scope drops the owner predicate of a scoped Db node (see Mutant). On a table in a `silo` it
// is equivalent: the silo's database holds the rows of the request's key value only.
func (c *collector) scope(e ir.Expr) {
	if tb, at := scopedTable(e); tb != nil {
		if k := c.siloOf(tb); k != "" {
			c.suppress(ScopeDropped, at.Line, "the silo's database holds this "+k+"'s rows only")
			return
		}
	}
	switch x := e.(type) {
	case *ir.DbScoped:
		r := &ir.DbSelect{Typed: x.Typed, Pos: x.Pos, Table: x.Table}
		c.add(ScopeDropped, x.Pos, x, r, c.show(x), "Db.from "+x.Table.Name)
	case *ir.DbGetScoped:
		get, body := gotOrNotFound(x.Table, x.ID, x.Ty, func(row ir.Expr) ir.Expr {
			return &ir.EffPure{Typed: ir.Typed{Ty: x.Ty}, Kind: ir.EffQuery, Value: row}
		})
		c.add(ScopeDropped, x.Pos, x, body, c.show(x), line(c.module, get, wrapped("", ", not found when missing")))
	case *ir.DbUpdateScoped:
		if c.fetchedMine(x.Table, x.ID) {
			c.suppress(ScopeDropped, x.Pos.Line, "load fetched it with getMine")
			return
		}
		w := &ir.DbUpdate{Typed: x.Typed, Pos: x.Pos, Table: x.Table, ID: x.ID, Fn: x.Fn}
		_, body := gotOrNotFound(x.Table, x.ID, x.Ty, func(ir.Expr) ir.Expr { return w })
		c.add(ScopeDropped, x.Pos, x, body, c.show(x), line(c.module, w, wrapped("", ", not found when missing")))
	case *ir.DbDeleteScoped:
		if c.fetchedMine(x.Table, x.ID) {
			c.suppress(ScopeDropped, x.Pos.Line, "load fetched it with getMine")
			return
		}
		w := &ir.DbDelete{Typed: x.Typed, Pos: x.Pos, Table: x.Table, ID: x.ID}
		_, body := gotOrNotFound(x.Table, x.ID, x.Ty, func(ir.Expr) ir.Expr { return w })
		c.add(ScopeDropped, x.Pos, x, body, c.show(x), line(c.module, w, wrapped("", ", not found when missing")))
	case *ir.DbInsertScoped:
		c.skip(ScopeDropped, x.Pos.Line, "the owner comes from the session either way")
	}
}

// siloOf is the `silo` key of table tb, "" for a shared one (or nil).
func (c *collector) siloOf(tb *ir.Table) string {
	if tb == nil {
		return ""
	}
	for k, ts := range c.p.Silos {
		for _, t := range ts {
			if t.Module == tb.Module && t.Name == tb.Name {
				return k
			}
		}
	}
	return ""
}

// tableNamed is the table "Module.Name", or nil.
func (c *collector) tableNamed(name string) *ir.Table {
	for _, t := range c.p.Tables {
		if t.Module+"."+t.Name == name {
			return t
		}
	}
	return nil
}

// scopedTable is the table and position of a scoped Db node.
func scopedTable(e ir.Expr) (*ir.Table, ir.Pos) {
	switch x := e.(type) {
	case *ir.DbScoped:
		return x.Table, x.Pos
	case *ir.DbGetScoped:
		return x.Table, x.Pos
	case *ir.DbUpdateScoped:
		return x.Table, x.Pos
	case *ir.DbDeleteScoped:
		return x.Table, x.Pos
	case *ir.DbInsertScoped:
		return x.Table, x.Pos
	}
	return nil, ir.Pos{}
}

// fetchedMine reports whether id, the id of a scoped write to tb in a page's update, is
// `model.f.id` where the page's load filled f with a row of tb it read with Db.getMine. Every
// message runs load first, so that row is the signed-in user's: the unscoped write could reach
// no other row, and its `scope dropped` mutant is the same program (equivalent).
func (c *collector) fetchedMine(tb *ir.Table, id ir.Expr) bool {
	outer, ok := id.(*ir.Field)
	if !ok || c.loaded == nil || outer.Name != tb.IDField {
		return false
	}
	inner, ok := outer.Expr.(*ir.Field)
	if !ok {
		return false
	}
	v, ok := inner.Expr.(*ir.Var)
	if !ok || v.Kind != ir.VarLocal || v.Name != c.modelParam {
		return false
	}
	t := c.loaded[inner.Name]
	return t != nil && t.Module == tb.Module && t.Name == tb.Name
}

// loadedMine is, for page pg, the model fields its load fills with a row it read with
// Db.getMine, and that row's table: a field f such that every record of the page's Model type
// that load builds sets f to a local bound once, by `let x <- Db.getMine T …`. Load building no
// such record (a helper makes the model), or a record update of the model, gives none.
func loadedMine(pg *ir.Page) map[string]*ir.Table {
	if pg.Load == nil || pg.Model == nil {
		return nil
	}
	mine := map[string]*ir.Table{} // a local bound by Db.getMine → its table
	var models []*ir.Record
	updated := false
	var visit func(e ir.Expr)
	visit = func(e ir.Expr) {
		if isNil(e) {
			return
		}
		switch x := e.(type) {
		case *ir.EffBind:
			if g, ok := x.Effect.(*ir.DbGetScoped); ok && x.Param != nil {
				mine[x.Param.Name] = g.Table
			}
		case *ir.Record:
			if isModel(pg.Model, x.T) {
				models = append(models, x)
			}
		case *ir.Update:
			if r, ok := x.Ty.(*types.Record); ok && r.Module == pg.Model.Module && r.Name == pg.Model.Name {
				updated = true
			}
		}
		for _, k := range ir.Children(e) {
			visit(k)
		}
	}
	visit(pg.Load.Body)
	if updated || len(models) == 0 {
		return nil
	}
	out := map[string]*ir.Table{}
	for _, f := range pg.Model.Fields {
		var tb *ir.Table
		for i, r := range models {
			var t *ir.Table
			for _, fi := range r.Fields {
				if v, ok := fi.Value.(*ir.Var); ok && fi.Name == f.Name && v.Kind == ir.VarLocal && binds(pg.Load.Body, v.Name) == 1 {
					t = mine[v.Name]
				}
			}
			if t == nil || (i > 0 && t != tb) {
				tb = nil
				break
			}
			tb = t
		}
		if tb != nil {
			out[f.Name] = tb
		}
	}
	return out
}

// putsSession reports whether e may put the session: it calls Session.put, or uses a value of
// the app whose type is a Task (or a function to one), which may put it (conservatively, any such
// helper). A scoped write after a put acts for the session it put, which may not be the user load
// read the row for.
func putsSession(e ir.Expr) bool {
	if isNil(e) {
		return false
	}
	if isStd(e, "Session", "put") {
		return true
	}
	if v, ok := e.(*ir.Var); ok && v.Kind == ir.VarGlobal && isTaskType(v.Ty) {
		return true
	}
	for _, k := range ir.Children(e) {
		if putsSession(k) {
			return true
		}
	}
	return false
}

// isTaskType reports whether t is a Task or a function whose result (after all its parameters)
// is one.
func isTaskType(t types.Type) bool {
	for {
		switch x := t.(type) {
		case *types.Fun:
			t = x.Result
		case *types.App:
			return x.Con == "Task"
		default:
			return false
		}
	}
}

func isModel(model, t *ir.RecordType) bool {
	return t != nil && (t == model || (t.Module == model.Module && t.Name == model.Name))
}

// binds counts the bindings of local name in e: lets, lambda parameters, pattern bindings, for
// variables and `let … <-`.
func binds(e ir.Expr, name string) int {
	if isNil(e) {
		return 0
	}
	n := 0
	switch x := e.(type) {
	case *ir.Let:
		if x.Name == name {
			n++
		}
	case *ir.Lambda:
		for _, p := range x.Params {
			if p.Name == name {
				n++
			}
		}
	case *ir.Match:
		n += bindsCases(x.Cases, name)
	case *ir.HMatch:
		n += bindsCases(x.Cases, name)
	case *ir.HFor:
		if x.Var == name {
			n++
		}
	case *ir.EffBind:
		if x.Param != nil && x.Param.Name == name {
			n++
		}
	}
	for _, k := range ir.Children(e) {
		n += binds(k, name)
	}
	return n
}

func bindsCases(cs []ir.Case, name string) int {
	n := 0
	for _, c := range cs {
		for _, b := range c.Binds {
			if b.Name == name {
				n++
			}
		}
	}
	return n
}

// gotOrNotFound is `Db.get tb id` (get) and the effect of type ty running it: on `Just row` the
// effect then builds from row, on Nothing not found (Query.notFound or Task.notFound, as ty).
func gotOrNotFound(tb *ir.Table, id ir.Expr, ty types.Type, then func(row ir.Expr) ir.Expr) (*ir.DbGet, ir.Expr) {
	rec := &types.Record{Module: tb.Module, Name: tb.Name}
	for _, f := range tb.Record.Fields {
		rec.Fields = append(rec.Fields, types.Field{Name: f.Name, Type: f.Type})
	}
	maybe := &types.Union{Module: "Prelude", Name: "Maybe", Args: []types.Type{rec}}
	kind, query := ir.EffTask, &types.App{Con: "Query", Args: []types.Type{maybe}}
	if app, ok := ty.(*types.App); ok && app.Con == "Query" {
		kind = ir.EffQuery
	}
	get := &ir.DbGet{Typed: ir.Typed{Ty: query}, Table: tb, ID: id}
	row := &ir.Var{Typed: ir.Typed{Ty: rec}, Kind: ir.VarLocal}
	body := then(row)
	// the names bound around body capture none of its free variables (the id, the update's fn)
	fresh := func(base string) string {
		n := base
		for i := 2; uses(body, n) > 0; i++ {
			n = base + strconv.Itoa(i)
		}
		return n
	}
	row.Name = fresh("row")
	found := &ir.Var{Typed: ir.Typed{Ty: maybe}, Kind: ir.VarLocal, Name: fresh("found")}
	m := &ir.Match{Typed: ir.Typed{Ty: ty}, Kind: ir.MatchUnion, Subject: found, Cases: []ir.Case{
		{Tag: "Just", Binds: []ir.Bind{{Name: row.Name, Field: "value", Type: rec}}, Body: body},
		{Tag: "Nothing", Body: &ir.EffNotFound{Typed: ir.Typed{Ty: ty}}},
	}}
	return get, &ir.EffBind{Typed: ir.Typed{Ty: ty}, Kind: kind, Effect: get, Param: &ir.Param{Name: found.Name, Type: maybe}, Body: m}
}

// sessionPut replaces a `let _ <- Session.put f` step by Task.succeed ().
func (c *collector) sessionPut(x *ir.EffBind) {
	call, ok := x.Effect.(*ir.Call)
	if !ok || !isStd(call.Fn, "Session", "put") {
		return
	}
	result := types.Type(types.Unit)
	if app, ok := call.Type().(*types.App); ok && len(app.Args) == 1 {
		result = app.Args[0]
	}
	repl := &ir.EffPure{Typed: ir.Typed{Ty: call.Type()}, Kind: ir.EffTask, Value: &ir.Lit{Typed: ir.Typed{Ty: result}, Kind: ir.LitUnit}}
	name := "_"
	if x.Param != nil {
		name = x.Param.Name
	}
	c.add(SessionPutDropped, x.Pos, call, repl,
		line(c.module, call, wrapped("let "+name+" <- ", "")), "let "+name+" <- Task.succeed ()")
}

// notDropped replaces `not e` by e.
func (c *collector) notDropped(x *ir.Call) {
	if !isStd(x.Fn, "Prelude", "not") || len(x.Args) != 1 {
		return
	}
	c.add(NotDropped, x.Pos, x, x.Args[0], c.show(x), c.show(x.Args[0]))
}

func isStd(e ir.Expr, module, name string) bool {
	v, ok := e.(*ir.Var)
	return ok && v.Kind == ir.VarStd && v.Module == module && v.Name == name
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
		c.add(UpdateFieldDropped, x.Pos, x, &r, c.show(x), c.show(&r))
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
	c.add(kind, x.Pos, x, r, c.show(x), "Rerender")
}

// ---- helpers

func isText(t types.Type) bool {
	con, ok := t.(*types.Con)
	return ok && con.Name == "Text"
}

// isHtml reports whether a value's final result type contains Html anywhere (`Html m`,
// `List (Html m)`, a record with an Html field, …): such a value is view code.
func isHtml(t types.Type) bool {
	for {
		fn, ok := t.(*types.Fun)
		if !ok {
			break
		}
		t = fn.Result
	}
	return containsHtml(t, map[types.Type]bool{})
}

func containsHtml(t types.Type, seen map[types.Type]bool) bool {
	if t == nil || seen[t] {
		return false
	}
	seen[t] = true
	var parts []types.Type
	switch x := t.(type) {
	case *types.App:
		if x.Con == "Html" {
			return true
		}
		parts = x.Args
	case *types.Union:
		parts = x.Args
	case *types.Record:
		for _, f := range x.Fields {
			parts = append(parts, f.Type)
		}
		parts = append(parts, x.Args...)
	case *types.Fun:
		parts = append(append(parts, x.Params...), x.Result)
	}
	for _, p := range parts {
		if containsHtml(p, seen) {
			return true
		}
	}
	return false
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
	for _, k := range ir.Children(e) {
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
