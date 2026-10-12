package gen

import (
	"fmt"
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
// without the field is served with the placeholder instead of the redirect. Schema mutants
// carry their tables: the full Tables with the mutated table's descriptor rendered from the
// mutant's clone (mutantsTable), which rtesting creates with db.Ensure.

// mutation is a mutate build's state.
type mutation struct {
	all      []mutate.Mutant
	byNode   map[ir.Expr][]mutate.Mutant  // the mutants replacing a node, by pointer
	requires map[*ir.Page][]mutate.Mutant // requires dropped, by page
	// equivalent and skipped count the refused mutants by category
	equivalent, skipped int
	// bare are the nodes whose original is being emitted (inside their own switch)
	bare map[ir.Expr]bool
}

// indexMutants collects p's mutants for emission and makes every constant a thunk: a package
// variable is computed once, before any mutant is switched on, so a mutant in it (or in what it
// calls) would never be seen.
func (g *gen) indexMutants(p *ir.Program) {
	ms, refused := mutate.Collect(p)
	m := &mutation{all: ms, byNode: map[ir.Expr][]mutate.Mutant{}, requires: map[*ir.Page][]mutate.Mutant{}, bare: map[ir.Expr]bool{}}
	for _, x := range ms {
		switch {
		case x.Kind == mutate.RequiresDropped:
			m.requires[x.Page] = append(m.requires[x.Page], x)
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

// mutantsOf are the mutants to switch on in front of e: none outside a mutate build, and none
// while e's original is emitted.
func (f *fn) mutantsOf(e ir.Expr) []mutate.Mutant {
	m := f.g.mutation
	if m == nil || m.bare[e] {
		return nil
	}
	return m.byNode[e]
}

// switched writes e's mutant switch and then e itself, both returned as k renders them (see
// retK): the statements of a mutated node in ret mode.
func (f *fn) switched(w *strings.Builder, e ir.Expr, ms []mutate.Mutant, s *scope, k func(ir.Expr, *scope) string) {
	w.WriteString("switch Mutant {\n")
	for _, m := range ms {
		fmt.Fprintf(w, "case %d:\n", m.ID)
		f.retK(w, m.Replacement, s.child(), k)
	}
	w.WriteString("}\n")
	f.g.mutation.bare[e] = true
	f.retK(w, e, s, k)
	delete(f.g.mutation.bare, e)
}

// switchedExpr is a mutated node as a Go expression of type typ: switched in a closure.
func (f *fn) switchedExpr(e ir.Expr, ms []mutate.Mutant, typ string, s *scope, k func(ir.Expr, *scope) string) string {
	var w strings.Builder
	f.switched(&w, e, ms, s.child(), k)
	return "func() " + typ + " {\n" + w.String() + "}()"
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
		fmt.Fprintf(b, "\tif !%s.%s.Ok && Mutant == %d {\n\t\t%s.%s = rt.Just(%s)\n\t}\n", app, Field(pg.Requires), m.ID,
			app, Field(pg.Requires), f.code(m.Replacement, newScope()))
	}
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
		row := fmt.Sprintf("{ID: %d, Module: %s, Decl: %s, Kind: %s, Line: %d, Before: %s, After: %s", x.ID, strconv.Quote(x.Module),
			strconv.Quote(x.Decl), strconv.Quote(x.Kind), x.Line, strconv.Quote(x.Before), strconv.Quote(x.After))
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
