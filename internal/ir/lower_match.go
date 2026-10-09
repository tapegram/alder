package ir

import (
	"fmt"
	"strconv"

	"alder/internal/syntax"
	"alder/internal/types"
)

// Lowering `match` to decision trees.
//
// The checker has proved every match exhaustive and patterns nest at most one level below a
// constructor (records and lists may nest further). Arms are compiled in order, first match wins:
//
//   - When the first arm is irrefutable (wildcard, variable, record of such) it is the result.
//     A refutable record pattern tests its fields left to right; when one fails, the next arm runs.
//   - Otherwise the arms switch on the subject's tag (Match.Kind from the subject's type): one
//     Case per tag, in order of first appearance. A tag's Case holds the arms with that tag plus
//     the wildcard and variable arms, in their original order ("specialization").
//   - A Case binds the parts of the matched value with Binds: a union payload (Field "value" when
//     wrapped, Field "" for the whole record, or one Bind per record field the arms look at) or a
//     list cons ("head", "rest"). Each part is always bound to a fresh temporary; a user's name
//     is only ever introduced by a Let (`Circle r` → Bind _m1 value, then Let r = _m1).
//   - The arms of a Case are tried one after another on those parts; a failing arm continues
//     with the next, then with the enclosing fallback. The fallback node may appear in more than
//     one place of the tree (shared).
//   - Variables an arm binds become Lets wrapped directly around that arm's body, after every
//     test of the arm has passed. No failure branch is ever inside a user's binding, so a later
//     arm or the fallback always sees the outer meaning of a name.
//   - Default is the compilation of the wildcard/variable arms (then the fallback). It is omitted
//     when the Cases cover every tag of the Kind (all constructors, True and False, nil and cons):
//     the checker proved it unreachable. A Default that is still nil after this is unreachable too,
//     by the checker's exhaustiveness proof of the enclosing match.
//
// The subject is bound to a temporary first when it is not a local variable and an arm needs to
// name it, or when a pattern variable has the subject variable's own name.

type row struct {
	pat  syntax.Pattern // nil is a wildcard
	body Expr
}

// listTail is the pattern for a list from element `from` of a list pattern on: `[a, b, ..r]`
// from 1 is `[b, ..r]`. It keeps the original node, whose Binds entry types `..r`.
type listTail struct {
	*syntax.PList
	from int
}

// binding is a user variable bound by an arm, waiting for the arm's tests to pass.
type binding struct {
	name  string
	t     types.Type
	value Expr
}

// cont builds the rest of an arm once its tests so far have passed, given its bindings.
type cont func(lets []binding) Expr

// specRow is one arm inside a Case: its pattern for the Case's parts (pat; for a list cons a
// listTail standing for head and tail), the bindings made so far, and what follows.
type specRow struct {
	pat  syntax.Pattern
	lets []binding
	k    cont
}

type matcher struct {
	l    *lowerer
	t    types.Type // the type of the whole match
	html bool       // build HMatch: the bodies are HTML
}

// match lowers `match subject { arms }` of type t.
func (l *lowerer) match(subject syntax.Expr, arms []*syntax.Arm, t types.Type, html bool) Expr {
	m := &matcher{l: l, t: t, html: html}
	subj := l.expr(subject)
	st := l.typeOf(subject)
	var rows []row
	named := false
	vars := map[string]bool{}
	for _, a := range arms {
		var body Expr
		if html {
			body = l.htmlNode(a.Body)
		} else {
			body = l.expr(a.Body)
		}
		rows = append(rows, row{a.Pat, body})
		switch a.Pat.(type) {
		case *syntax.PVar, *syntax.PRecord, *syntax.PList:
			named = true
		}
		patternVars(a.Pat, vars)
	}
	v, isLocal := subj.(*Var)
	isLocal = isLocal && v.Kind == VarLocal
	if (isLocal && !vars[v.Name]) || (!isLocal && !named) {
		return m.compile(subj, st, rows, nil)
	}
	tmp := l.temp()
	return &Let{Typed: Typed{t}, Name: tmp, T: st, Value: subj, Body: m.compile(local(tmp, st), st, rows, nil)}
}

// patternVars adds the names p binds to vars.
func patternVars(p syntax.Pattern, vars map[string]bool) {
	switch p := p.(type) {
	case *syntax.PVar:
		vars[p.Name] = true
	case *syntax.PCtor:
		patternVars(p.Arg, vars)
	case *syntax.PRecord:
		for _, f := range p.Fields {
			patternVars(f.Pat, vars)
		}
	case *syntax.PList:
		for _, e := range p.Elems {
			patternVars(e, vars)
		}
		if p.Rest != "" {
			vars[p.Rest] = true
		}
	}
}

func local(name string, t types.Type) *Var { return &Var{Typed: Typed{t}, Kind: VarLocal, Name: name} }

// leaf is the continuation that ends an arm: its bindings as Lets around its body.
func (m *matcher) leaf(body Expr) cont {
	return func(lets []binding) Expr {
		for i := len(lets) - 1; i >= 0; i-- {
			b := lets[i]
			if v, ok := b.value.(*Var); ok && v.Kind == VarLocal && v.Name == b.name {
				continue // the part was bound under the arm's own name already
			}
			body = &Let{Typed: Typed{m.t}, Name: b.name, T: b.t, Value: b.value, Body: body}
		}
		return body
	}
}

// with returns lets plus b, never sharing the backing array between branches.
func with(lets []binding, b binding) []binding {
	return append(lets[:len(lets):len(lets)], b)
}

// compile matches subj (of type st) against rows in order; fallback runs when none matches.
func (m *matcher) compile(subj Expr, st types.Type, rows []row, fallback Expr) Expr {
	if len(rows) == 0 {
		return fallback
	}
	r := rows[0]
	if irrefutable(r.pat) {
		return m.pattern(subj, st, r.pat, nil, m.leaf(r.body), nil)
	}
	if _, isRec := r.pat.(*syntax.PRecord); isRec {
		return m.pattern(subj, st, r.pat, nil, m.leaf(r.body), m.compile(subj, st, rows[1:], fallback))
	}
	return m.switchOn(subj, st, rows, fallback)
}

// pattern tests subj against one pattern: on success it continues with k (adding the
// pattern's variables to lets), on failure it is fail.
func (m *matcher) pattern(subj Expr, st types.Type, pat syntax.Pattern, lets []binding, k cont, fail Expr) Expr {
	switch p := pat.(type) {
	case nil, *syntax.PWild:
		return k(lets)
	case *syntax.PVar:
		return k(with(lets, binding{p.Name, m.l.bindType(p), subj}))
	case *syntax.PList:
		return m.pattern(subj, st, listTail{p, 0}, lets, k, fail)
	case listTail:
		if p.from == len(p.Elems) && p.Rest != "" { // `[..rest]`: any list
			return k(with(lets, binding{p.Rest, m.l.bindType(p.PList), subj}))
		}
	case *syntax.PRecord:
		rec := st.(*types.Record)
		var field func(i int, lets []binding) Expr
		field = func(i int, lets []binding) Expr {
			if i == len(p.Fields) {
				return k(lets)
			}
			f := p.Fields[i]
			ft := fieldType(rec, f.Name)
			return m.pattern(&Field{Typed: Typed{ft}, Expr: subj, Name: f.Name}, ft, f.Pat, lets,
				func(l2 []binding) Expr { return field(i+1, l2) }, fail)
		}
		return field(0, lets)
	}
	// one refutable tag: a Match with a single Case, failing to fail
	tag, _ := tagOf(pat)
	kind := matchKind(st)
	return m.build(subj, st, kind, []Case{m.caseFor(kind, tag, st, []specRow{{inner(pat), lets, k}}, fail)}, fail)
}

// build makes the Match (or HMatch) on subj with cases; dflt is used unless cases cover every tag.
func (m *matcher) build(subj Expr, st types.Type, kind MatchKind, cases []Case, dflt Expr) Expr {
	out := &Match{Typed: Typed{m.t}, Kind: kind, Subject: subj, Cases: cases}
	complete := false
	switch kind {
	case MatchUnion:
		complete = len(cases) == len(m.l.union(st.(*types.Union)).Ctors)
	case MatchBool, MatchList:
		complete = len(cases) == 2
	}
	if !complete {
		out.Default = dflt
	}
	if m.html {
		h := HMatch(*out)
		return &h
	}
	return out
}

func fieldType(rec *types.Record, name string) types.Type {
	for _, f := range rec.Fields {
		if f.Name == name {
			return f.Type
		}
	}
	panic(fmt.Sprintf("ir: record %s has no field %s", types.String(rec), name))
}

// irrefutable reports whether p matches every value of its type.
func irrefutable(p syntax.Pattern) bool {
	switch p := p.(type) {
	case nil, *syntax.PWild, *syntax.PVar:
		return true
	case *syntax.PRecord:
		for _, f := range p.Fields {
			if !irrefutable(f.Pat) {
				return false
			}
		}
		return true
	case *syntax.PList:
		return len(p.Elems) == 0 && p.Rest != ""
	case listTail:
		return p.from == len(p.Elems) && p.Rest != ""
	}
	return false
}

// tagOf is the tag a refutable top-level pattern tests; ok is false for an irrefutable one.
func tagOf(p syntax.Pattern) (tag string, ok bool) {
	switch p := p.(type) {
	case *syntax.PCtor:
		return p.Name, true
	case *syntax.PLit:
		switch lit := p.Lit.(type) {
		case *syntax.BoolLit:
			if lit.Value {
				return "True", true
			}
			return "False", true
		case *syntax.IntLit:
			return strconv.FormatInt(lit.Value, 10), true
		case *syntax.FloatLit:
			return strconv.FormatFloat(lit.Value, 'g', -1, 64), true
		case *syntax.StrLit:
			text, _ := literalText(lit)
			return text, true
		}
	case *syntax.PList:
		return tagOf(listTail{p, 0})
	case listTail:
		if p.from < len(p.Elems) {
			return "cons", true
		}
		if p.Rest == "" {
			return "nil", true
		}
	}
	return "", false
}

func matchKind(t types.Type) MatchKind {
	switch t := t.(type) {
	case *types.Union:
		return MatchUnion
	case *types.Con:
		if t.Name == "Bool" {
			return MatchBool
		}
	case *types.App:
		if t.Con == "List" {
			return MatchList
		}
	}
	return MatchLit
}

// switchOn builds one Match on subj's tag (see the comment at the top of this file).
func (m *matcher) switchOn(subj Expr, st types.Type, rows []row, fallback Expr) Expr {
	kind := matchKind(st)
	var tags []string
	seen := map[string]bool{}
	var defaults []row
	for _, r := range rows {
		if t, ok := tagOf(r.pat); !ok {
			defaults = append(defaults, r)
		} else if !seen[t] {
			seen[t] = true
			tags = append(tags, t)
		}
	}
	var cases []Case
	for _, tag := range tags {
		var spec []specRow // the rows that can match tag
		for _, r := range rows {
			switch t, ok := tagOf(r.pat); {
			case !ok:
				spec = append(spec, specRow{nil, nil, m.whole(subj, st, r)})
			case t == tag:
				spec = append(spec, specRow{inner(r.pat), nil, m.leaf(r.body)})
			}
		}
		cases = append(cases, m.caseFor(kind, tag, st, spec, fallback))
	}
	return m.build(subj, st, kind, cases, m.compile(subj, st, defaults, fallback))
}

// whole continues an irrefutable arm inside a Case: its pattern applies to the whole subject.
func (m *matcher) whole(subj Expr, st types.Type, r row) cont {
	return func(lets []binding) Expr { return m.pattern(subj, st, r.pat, lets, m.leaf(r.body), nil) }
}

// inner is what a tagged pattern says about the value's parts: a constructor's argument
// pattern, or for a list cons the listTail itself (head is Elems[from], the tail from+1).
func inner(p syntax.Pattern) syntax.Pattern {
	switch p := p.(type) {
	case *syntax.PCtor:
		return p.Arg
	case *syntax.PList:
		return inner(listTail{p, 0})
	case listTail:
		if p.from < len(p.Elems) {
			return p
		}
	}
	return nil
}

// part is one piece of the matched value a Case binds.
type part struct {
	field string
	t     types.Type
}

// caseFor builds the Case for tag from its rows.
func (m *matcher) caseFor(kind MatchKind, tag string, st types.Type, spec []specRow, fallback Expr) Case {
	var parts []part
	pats := make([][]syntax.Pattern, len(spec)) // per row, one pattern per part
	switch {
	case kind == MatchUnion:
		ct := m.l.union(st.(*types.Union)).ctorT(tag)
		if ct.Payload == nil {
			break
		}
		pt := m.l.payloadType(st, tag)
		rec, isRec := pt.(*types.Record)
		perField := isRec && !m.l.wrappers[ct.Payload]
		for _, r := range spec {
			if _, ok := r.pat.(*syntax.PRecord); !ok && r.pat != nil && !isWild(r.pat) {
				perField = false // an arm names the whole payload
			}
		}
		if !perField {
			field := ""
			if m.l.wrappers[ct.Payload] {
				field = "value"
			}
			parts = []part{{field, pt}}
			for i, r := range spec {
				pats[i] = []syntax.Pattern{r.pat}
			}
			break
		}
		for _, f := range rec.Fields {
			parts = append(parts, part{f.Name, f.Type})
			for i, r := range spec {
				var fp syntax.Pattern
				if pr, ok := r.pat.(*syntax.PRecord); ok {
					for _, pf := range pr.Fields {
						if pf.Name == f.Name {
							fp = pf.Pat
						}
					}
				}
				pats[i] = append(pats[i], fp)
			}
		}
	case kind == MatchList && tag == "cons":
		parts = []part{{"head", st.(*types.App).Args[0]}, {"rest", st}}
		for i, r := range spec {
			if lt, ok := r.pat.(listTail); ok {
				pats[i] = []syntax.Pattern{lt.Elems[lt.from], listTail{lt.PList, lt.from + 1}}
			} else {
				pats[i] = []syntax.Pattern{nil, nil}
			}
		}
	}
	// bind each part a pattern needs to a temporary; drop the others
	c := Case{Tag: tag}
	var subjs []Expr
	var used []int
	for j, p := range parts {
		needed := false
		for i := range spec {
			needed = needed || (pats[i][j] != nil && !isWild(pats[i][j]))
		}
		if !needed {
			continue
		}
		name := m.l.temp()
		c.Binds = append(c.Binds, Bind{Name: name, Field: p.field, Type: p.t})
		subjs = append(subjs, local(name, p.t))
		used = append(used, j)
	}
	c.Body = m.rows(subjs, used, pats, spec, fallback)
	return c
}

func isWild(p syntax.Pattern) bool { _, ok := p.(*syntax.PWild); return ok }

// rows tries spec's rows in order on the bound parts (subjs, at part indexes used): the first
// whose every part matches runs; a failing row continues with the next, then with fallback.
func (m *matcher) rows(subjs []Expr, used []int, pats [][]syntax.Pattern, spec []specRow, fallback Expr) Expr {
	if len(spec) == 0 {
		return fallback
	}
	var fail Expr
	for _, j := range used {
		if !irrefutable(pats[0][j]) {
			fail = m.rows(subjs, used, pats[1:], spec[1:], fallback)
			break
		}
	}
	var step func(i int, lets []binding) Expr
	step = func(i int, lets []binding) Expr {
		if i == len(used) {
			return spec[0].k(lets)
		}
		j := used[i]
		return m.pattern(subjs[i], subjs[i].Type(), pats[0][j], lets, func(l2 []binding) Expr { return step(i+1, l2) }, fail)
	}
	return step(0, spec[0].lets)
}
