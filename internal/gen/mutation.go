package gen

import (
	"fmt"
	"reflect"
	"strconv"
	"strings"

	"alder/internal/ir"
	"alder/internal/mutate"
	"alder/internal/types"
)

// Mutant schemata (Options.Mutate, alder test --mutate).
//
// Every mutant of the program (mutate.Collect) is compiled into the one test build: the
// package variable Mutant names the mutant a run switches on (0: none), and each mutated node
// is emitted with its replacements in front of it, one case per mutant:
//
//	in ret mode (a statement that returns):     in expr mode (a Go expression):
//	switch Mutant {                             func() T {
//	case 17:                                        switch Mutant {
//		return <the replacement>                    case 17:
//	}                                                   return <the replacement>
//	return <the original>                           }
//	                                                return <the original>
//	                                            }()
//
// An effect node in a Task position (the effect of a `let … <-` step) is the expr form of type
// eff.Task[A]. A node the IR shares between several places (a table field's default, which
// every record literal omitting the field holds) is switched everywhere it is emitted, tests
// included. `requires dropped` is in the page's route (pageFunc): under its id, a session
// without the field is served with the placeholder instead of the redirect; `role dropped`
// too: under its id the role check lets every role through (roleDropped); `membership
// dropped` in session__via: under its id the field's membership check is skipped. Schema mutants
// carry their tables: the full Tables with the mutated table's descriptor rendered from the
// mutant's clone (mutantsTable), which rtesting creates with db.Ensure.

// mutation is a mutate build's state.
type mutation struct {
	all      []mutate.Mutant
	byNode   map[ir.Expr][]mutate.Mutant  // the mutants replacing a node, by pointer
	requires map[*ir.Page][]mutate.Mutant // requires dropped, by page
	roles    map[*ir.Page][]mutate.Mutant // role dropped, by page
	members  map[string]int               // membership dropped's id, by `via` field
	// equivalent and skipped count the refused mutants by category
	equivalent, skipped int
	// bare are the nodes whose original is being emitted (inside their own switch)
	bare map[ir.Expr]bool
	// subst are the hoisted children of the node being switched, as expr renders them (hoist)
	subst map[ir.Expr]string
}

// indexMutants collects p's mutants for emission and makes every constant a thunk: a package
// variable is computed once, before any mutant is switched on, so a mutant in it (or in what it
// calls) would never be seen.
func (g *gen) indexMutants(p *ir.Program) {
	ms, refused := mutate.Collect(p)
	m := &mutation{all: ms, byNode: map[ir.Expr][]mutate.Mutant{}, requires: map[*ir.Page][]mutate.Mutant{}, roles: map[*ir.Page][]mutate.Mutant{}, members: map[string]int{}, bare: map[ir.Expr]bool{}, subst: map[ir.Expr]string{}}
	for _, x := range ms {
		switch {
		case x.Kind == mutate.RequiresDropped:
			m.requires[x.Page] = append(m.requires[x.Page], x)
		case x.Kind == mutate.RoleDropped:
			m.roles[x.Page] = append(m.roles[x.Page], x)
		case x.Kind == mutate.MembershipDropped:
			m.members[x.Field] = x.ID
		case x.Node != nil:
			m.byNode[x.Node] = append(m.byNode[x.Node], x)
		}
	}
	for _, r := range refused {
		if r.Category == mutate.Equivalent {
			m.equivalent++
		} else {
			m.skipped++
		}
	}
	for _, v := range g.values {
		if _, isFun := v.Type.(*types.Fun); !isFun && len(v.Params) == 0 {
			g.thunks[v] = true
		}
	}
	g.mutation = m
}

// mutantsOf are the mutants to switch on in front of e: none outside a mutate build, none while
// e's original is emitted, and none of an If or a Match, whose mutants retK emits in place
// (mutatedIf, mutatedMatch).
func (f *fn) mutantsOf(e ir.Expr) []mutate.Mutant {
	m := f.g.mutation
	if m == nil || m.bare[e] {
		return nil
	}
	switch e.(type) {
	case *ir.If, *ir.Match:
		return nil
	}
	return m.byNode[e]
}

// localMutants are the mutants of If or Match e (nil outside a mutate build).
func (f *fn) localMutants(e ir.Expr) []mutate.Mutant {
	if f.g.mutation == nil {
		return nil
	}
	return f.g.mutation.byNode[e]
}

// switched writes e's mutant switch and then e itself, both returned as k renders them (see
// retK): the statements of a mutated node in ret mode. The children of e that a replacement
// shares (the operands of a flipped operator, the argument of a dropped `not`, the fields of an
// update, the steps of a select) are hoisted first (hoist): emitted once, each replacement and
// the original refer to them, so a node nested in another's operand is not emitted per mutant.
func (f *fn) switched(w *strings.Builder, e ir.Expr, ms []mutate.Mutant, s *scope, k func(ir.Expr, *scope) string) {
	hoisted := f.hoist(w, e, ms, s)
	w.WriteString("switch Mutant {\n")
	for _, m := range ms {
		fmt.Fprintf(w, "case %d:\n", m.ID)
		f.retK(w, m.Replacement, s.child(), k)
	}
	w.WriteString("}\n")
	f.g.mutation.bare[e] = true
	f.retK(w, e, s, k)
	delete(f.g.mutation.bare, e)
	for _, h := range hoisted {
		delete(f.g.mutation.subst, h)
	}
}

// hoist declares, in front of e's switch, a Go variable for each child of e that one of the
// replacements ms shares (the replacement itself, or one of its children) and has expr render
// the child as that variable (mutation.subst) until switched deletes it. The right operand of
// `&&` and `||` is a closure, called where it was: it is evaluated only when the operator needs
// it, flipped or not. It returns the hoisted children.
func (f *fn) hoist(w *strings.Builder, e ir.Expr, ms []mutate.Mutant, s *scope) []ir.Expr {
	m := f.g.mutation
	shared := map[ir.Expr]bool{}
	for _, x := range ms {
		shared[x.Replacement] = true
		for _, c := range ir.Children(x.Replacement) {
			shared[c] = true
		}
	}
	var lazy ir.Expr
	if op, ok := e.(*ir.Op); ok && (op.Kind == "&&" || op.Kind == "||") {
		lazy = op.R
	}
	var out []ir.Expr
	for _, c := range ir.Children(e) {
		if isNilExpr(c) || !shared[c] || m.subst[c] != "" || atom(c) && m.byNode[c] == nil {
			continue
		}
		name := f.temp(s, "_h")
		if c == lazy {
			var b strings.Builder
			f.ret(&b, c, s.child())
			fmt.Fprintf(w, "%s := func() %s {\n%s}\n", name, GoType(c.Type()), b.String())
			m.subst[c] = name + "()"
		} else {
			fmt.Fprintf(w, "%s := %s\n", name, f.typedExpr(c, s))
			m.subst[c] = name
		}
		fmt.Fprintf(w, "_ = %s\n", name) // the original may render the child without it
		out = append(out, c)
	}
	return out
}

// atom reports whether e is a variable, a literal or a field path (`t.done`): cheaper to repeat
// than to hoist.
func atom(e ir.Expr) bool {
	switch e.(type) {
	case *ir.Var, *ir.Lit:
		return true
	}
	return isPath(e)
}

// hoisted is e's hoisted variable (hoist), when e is a hoisted child of the node being switched.
func (f *fn) hoisted(e ir.Expr) (string, bool) {
	if m := f.g.mutation; m != nil && m.subst[e] != "" {
		return m.subst[e], true
	}
	return "", false
}

// isNilExpr reports whether e is nil or a typed nil pointer (a select's missing Limit).
func isNilExpr(e ir.Expr) bool {
	if e == nil {
		return true
	}
	v := reflect.ValueOf(e)
	return v.Kind() == reflect.Pointer && v.IsNil()
}

// switchedExpr is a mutated node as a Go expression of type typ: switched in a closure.
func (f *fn) switchedExpr(e ir.Expr, ms []mutate.Mutant, typ string, s *scope, k func(ir.Expr, *scope) string) string {
	var w strings.Builder
	f.switched(&w, e, ms, s.child(), k)
	return "func() " + typ + " {\n" + w.String() + "}()"
}

// mutatedIf is If x with its mutants, each switched in its condition, each branch emitted once:
//
//	if Mutant != 12 && ((Mutant == 13) != (cond)) {   guard dropped 12 (the Then is the guard),
//		<then>                                        condition negated 13
//	}
//	<else>
//
// (`Mutant == 12 || (…)` when the Else is the guard).
func (f *fn) mutatedIf(w *strings.Builder, x *ir.If, ms []mutate.Mutant, s *scope, k func(ir.Expr, *scope) string) {
	c := f.code(x.Cond, s)
	for _, m := range ms {
		if m.Kind == mutate.ConditionNegated {
			c = fmt.Sprintf("(Mutant == %d) != (%s)", m.ID, c)
		}
	}
	for _, m := range ms {
		if m.Kind != mutate.GuardDropped {
			continue
		}
		if m.Replacement == x.Else {
			c = fmt.Sprintf("Mutant != %d && (%s)", m.ID, c)
		} else {
			c = fmt.Sprintf("Mutant == %d || (%s)", m.ID, c)
		}
	}
	fmt.Fprintf(w, "if %s {\n", c)
	f.retK(w, x.Then, s.child(), k)
	w.WriteString("}\n")
	f.retK(w, x.Else, s, k)
}

// mutatedMatch is Match x with its mutants (arm swapped, guard dropped), each arm's body emitted
// once. The match picks an arm; an arm a mutant moves (its body lifted out of its case: it uses
// nothing the case binds) does not return its body but its number, 1-based, and leaves the
// labelled switch; the mutants then renumber the arm, and a switch on the number returns it:
//
//	_arm3 := 0
//	_arms3:
//	switch {
//	default:
//		if Mutant == 21 {             guard dropped 21: the other arm, whatever the subject
//			_arm3 = 2
//			break _arms3
//		}
//		<the match: arm 1 is `_arm3 = 1; break _arms3`, …; an arm no mutant moves returns>
//	}
//	switch Mutant {
//	case 22:                          arm swapped 22: arms 1 and 2
//		switch _arm3 {
//		case 1: _arm3 = 2
//		case 2: _arm3 = 1
//		}
//	}
//	switch _arm3 {
//	case 1: <arm 1's body>
//	case 2: <arm 2's body>
//	}
//	panic("unreachable")
func (f *fn) mutatedMatch(w *strings.Builder, x *ir.Match, ms []mutate.Mutant, s *scope, k func(ir.Expr, *scope) string) {
	body := func(r *ir.Match, i int) ir.Expr {
		if i < len(r.Cases) {
			return r.Cases[i].Body
		}
		return r.Default
	}
	moved := map[int]ir.Expr{} // arm index → its lifted body
	for _, m := range ms {
		switch m.Kind {
		case mutate.ArmSwapped:
			i, r := m.Arms[0], m.Replacement.(*ir.Match)
			moved[i], moved[i+1] = body(r, i+1), body(r, i)
		case mutate.GuardDropped:
			moved[m.Arms[1]] = m.Replacement
		}
	}
	v := f.temp(s, "_arm")
	label := fmt.Sprintf("_arms%d", f.temps)
	fmt.Fprintf(w, "%s := 0\n%s:\nswitch {\ndefault:\n", v, label)
	for _, m := range ms {
		if m.Kind == mutate.GuardDropped {
			fmt.Fprintf(w, "if Mutant == %d {\n%s = %d\nbreak %s\n}\n", m.ID, v, m.Arms[1]+1, label)
		}
	}
	jumps := map[int]string{}
	for i := range moved {
		jumps[i] = fmt.Sprintf("%s = %d\nbreak %s\n", v, i+1, label)
	}
	if f.jumps == nil {
		f.jumps = map[*ir.Match]map[int]string{}
	}
	f.jumps[x] = jumps
	f.match(w, x, s.child(), k)
	delete(f.jumps, x)
	w.WriteString("}\n")
	var swaps []mutate.Mutant
	for _, m := range ms {
		if m.Kind == mutate.ArmSwapped {
			swaps = append(swaps, m)
		}
	}
	if len(swaps) > 0 {
		w.WriteString("switch Mutant {\n")
		for _, m := range swaps {
			i := m.Arms[0] + 1
			fmt.Fprintf(w, "case %d:\nswitch %s {\ncase %d:\n%s = %d\ncase %d:\n%s = %d\n}\n", m.ID, v, i, v, i+1, i+1, v, i)
		}
		w.WriteString("}\n")
	}
	fmt.Fprintf(w, "switch %s {\n", v)
	for i := 0; i <= len(x.Cases); i++ {
		if b, ok := moved[i]; ok {
			fmt.Fprintf(w, "case %d:\n", i+1)
			f.retK(w, b, s.child(), k)
		}
	}
	w.WriteString("}\npanic(\"unreachable\")\n")
}

// requiresPlaceholder writes, into page pg's route, the `requires dropped` mutants of its
// `requires`: under the mutant's id a session without the field gets the placeholder, so the
// check that follows lets the request through. app is the route's App session variable.
func (g *gen) requiresPlaceholder(b *strings.Builder, pg *ir.Page, app string) {
	if g.mutation == nil {
		return
	}
	for _, m := range g.mutation.requires[pg] {
		f := &fn{g: g}
		fmt.Fprintf(b, "\tif !%s.%s.Ok && Mutant == %d {\n\t\t%s.%s = rt.Just(%s)\n\t}\n", app, Field(firstRequired(pg)), m.ID,
			app, Field(firstRequired(pg)), f.code(m.Replacement, newScope()))
	}
}

// membershipDropped is the id of the `membership dropped` mutant of session field k (its `via`
// check is skipped under it: session__via), 0 for none or outside a mutate build.
func (g *gen) membershipDropped(k string) int {
	if g.mutation == nil {
		return 0
	}
	return g.mutation.members[k]
}

// roleDropped is the prefix of page pg's role check (roleRefused) that lets every role through
// under the id of each of its `role dropped` mutants: "Mutant != 12 && ", "" outside a mutate
// build.
func (g *gen) roleDropped(pg *ir.Page) string {
	if g.mutation == nil {
		return ""
	}
	var b strings.Builder
	for _, m := range g.mutation.roles[pg] {
		fmt.Fprintf(&b, "Mutant != %d && ", m.ID)
	}
	return b.String()
}

// mutantsTable writes Mutant, Mutants (rtesting.Mutant, in id order; a schema mutant with the
// app's tables, its own table's descriptor rendered from the mutant's clone), and
// MutantsSuppressed and MutantsSkipped (the refused mutants by category).
func (g *gen) mutantsTable(b *strings.Builder, p *ir.Program) {
	m := g.mutation
	if m == nil {
		return
	}
	b.WriteString("// Mutant is the mutant the running suite switches on (alder test --mutate): an id of Mutants, 0 for none.\n" +
		"var Mutant int\n\n")
	var rows []string
	for _, x := range m.all {
		row := fmt.Sprintf("{ID: %d, Module: %s, Decl: %s, Kind: %s, File: %s, Line: %d, Col: %d, Before: %s, After: %s", x.ID, strconv.Quote(x.Module),
			strconv.Quote(x.Decl), strconv.Quote(x.Kind), strconv.Quote(x.File), x.Line, x.Col, strconv.Quote(x.Before), strconv.Quote(x.After))
		if x.Table != nil {
			var ds []string
			for _, tb := range p.Tables {
				if tb.Module == x.Table.Module && tb.Name == x.Table.Name {
					ds = append(ds, g.tableDescLit(x.Table))
				} else {
					ds = append(ds, tableName(tb.Module, tb.Name)+".Desc()")
				}
			}
			row += ", Tables: []db.TableDesc{" + strings.Join(ds, ", ") + "}"
		}
		rows = append(rows, row+"}")
	}
	list := "{}"
	if len(rows) > 0 {
		list = "{\n" + strings.Join(rows, ",\n") + ",\n}"
	}
	b.WriteString("// Mutants are the app's mutants in id order (mutate.Collect); a schema mutant's Tables are the app's tables with its change, created by db.Ensure.\n" +
		"var Mutants = []rtesting.Mutant" + list + "\n\n")
	fmt.Fprintf(b, "// MutantsSuppressed and MutantsSkipped count the mutants refused as equivalent and as not applicable.\n"+
		"var MutantsSuppressed, MutantsSkipped = %d, %d\n\n", m.equivalent, m.skipped)
}

// tableDescLit is table tb's descriptor as a db.TableDesc literal (the columns of tableDecl).
func (g *gen) tableDescLit(tb *ir.Table) string {
	var cols []string
	for _, fd := range tb.Record.Fields {
		cols = append(cols, columnLit(tb, fd))
	}
	s := "{Name: " + strconv.Quote(tb.Module+"."+tb.Name) + ", Columns: []db.Column{" + strings.Join(cols, ", ") + "}, ID: " + strconv.Quote(tb.IDField)
	if tb.Created != "" {
		s += ", Created: " + strconv.Quote(tb.Created)
	}
	if tb.Updated != "" {
		s += ", Updated: " + strconv.Quote(tb.Updated)
	}
	return s + "}"
}
