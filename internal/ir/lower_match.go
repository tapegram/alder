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
//   - A wildcard arm is its body; a variable arm is `Let x = subject` around its body; a record
//     pattern tests its fields left to right (a variable field is a Let of the Field access).
//   - Otherwise the arms switch on the subject's tag (Match.Kind from the subject's type): one
//     Case per tag, in order of first appearance. A tag's Case holds the arms with that tag plus
//     the wildcard and variable arms, in their original order ("specialization").
//   - The payload of a union Case is bound by Binds and the inner patterns are matched against
//     those names: an irrefutable record payload pattern binds each named field directly
//     (`Rect { w: w, h: h }` → Bind w w, Bind h h); a variable binds the whole payload (Field "",
//     or "value" for a wrapped non-record payload); anything else binds the payload or each field
//     to a name (the arm's own variable if it has one, else a temporary) and continues with an
//     inner Match on it. A list `cons` Case binds "head" and "rest" the same way.
//   - When an arm's inner pattern fails, matching continues with the next arm of the same tag,
//     then with the enclosing fallback. The fallback expression may therefore appear in more than
//     one place of the tree (the same node, shared).
//   - Default is the compilation of the wildcard/variable arms (then the fallback). It is omitted
//     when the Cases cover every tag of the Kind (all constructors, True and False, nil and cons):
//     the checker proved it unreachable. A Default that is still nil after this is unreachable too,
//     by the checker's exhaustiveness proof of the enclosing match.
//
// A subject that is not already a local variable is bound to a temporary first when an arm needs
// to name it (a variable or record pattern at the top).

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
	}
	if v, ok := subj.(*Var); (ok && v.Kind == VarLocal) || !named {
		return m.compile(subj, st, rows, nil)
	}
	tmp := l.temp()
	return &Let{Typed: Typed{t}, Name: tmp, T: st, Value: subj, Body: m.compile(local(tmp, st), st, rows, nil)}
}

func local(name string, t types.Type) *Var { return &Var{Typed: Typed{t}, Kind: VarLocal, Name: name} }

// compile matches subj (of type st) against rows in order; fallback runs when none matches.
func (m *matcher) compile(subj Expr, st types.Type, rows []row, fallback Expr) Expr {
	if len(rows) == 0 {
		return fallback
	}
	r := rows[0]
	switch p := r.pat.(type) {
	case nil, *syntax.PWild:
		return r.body
	case *syntax.PVar:
		return m.let(subj, p.Name, m.l.bindType(p), r.body)
	case *syntax.PList:
		return m.compile(subj, st, append([]row{{listTail{p, 0}, r.body}}, rows[1:]...), fallback)
	case listTail:
		if p.from == len(p.Elems) && p.Rest != "" { // `[..rest]`: any list
			return m.let(subj, p.Rest, m.l.bindType(p.PList), r.body)
		}
	case *syntax.PRecord:
		var rest Expr
		if !irrefutable(p) {
			rest = m.compile(subj, st, rows[1:], fallback)
		}
		rec := st.(*types.Record)
		e := r.body
		for i := len(p.Fields) - 1; i >= 0; i-- {
			f := p.Fields[i]
			ft := fieldType(rec, f.Name)
			e = m.compile(&Field{Typed: Typed{ft}, Expr: subj, Name: f.Name}, ft, []row{{f.Pat, e}}, rest)
		}
		return e
	}
	return m.switchOn(subj, st, rows, fallback)
}

// let binds name to subj around body, unless subj already is that variable.
func (m *matcher) let(subj Expr, name string, t types.Type, body Expr) Expr {
	if v, ok := subj.(*Var); ok && v.Kind == VarLocal && v.Name == name {
		return body
	}
	return &Let{Typed: Typed{m.t}, Name: name, T: t, Value: subj, Body: body}
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

// switchOn builds one Match on subj's tag (see the package comment above).
func (m *matcher) switchOn(subj Expr, st types.Type, rows []row, fallback Expr) Expr {
	for i, r := range rows {
		if p, ok := r.pat.(*syntax.PList); ok {
			rows[i].pat = listTail{p, 0}
		}
	}
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
	out := &Match{Typed: Typed{m.t}, Kind: kind, Subject: subj}
	for _, tag := range tags {
		var spec []row // the rows that can match tag: inner pattern + body
		for _, r := range rows {
			switch t, ok := tagOf(r.pat); {
			case !ok:
				spec = append(spec, row{nil, m.compile(subj, st, []row{r}, nil)})
			case t == tag:
				spec = append(spec, row{inner(r.pat), r.body})
			}
		}
		out.Cases = append(out.Cases, m.caseFor(kind, tag, st, spec, fallback))
	}
	complete := false
	switch kind {
	case MatchUnion:
		complete = len(tags) == len(m.l.union(st.(*types.Union)).Ctors)
	case MatchBool, MatchList:
		complete = len(tags) == 2
	}
	if !complete {
		out.Default = m.compile(subj, st, defaults, fallback)
	}
	if m.html {
		h := HMatch(*out)
		return &h
	}
	return out
}

// inner is what a tagged pattern says about the value's parts: a constructor's argument pattern,
// or for a list cons the pair (head, tail) as a listTail with from+1 (head is p.Elems[from]).
func inner(p syntax.Pattern) syntax.Pattern {
	switch p := p.(type) {
	case *syntax.PCtor:
		return p.Arg
	case listTail:
		if p.from < len(p.Elems) {
			return p
		}
	}
	return nil
}

// caseFor builds the Case for tag from its specialized rows.
func (m *matcher) caseFor(kind MatchKind, tag string, st types.Type, spec []row, fallback Expr) Case {
	c := Case{Tag: tag}
	switch {
	case kind == MatchUnion:
		u := m.l.union(st.(*types.Union))
		ct := u.ctorT(tag)
		if ct.Payload == nil {
			c.Body = spec[0].body
			return c
		}
		pt := m.l.payloadType(st, tag)
		if m.l.wrappers[ct.Payload] {
			c.Binds, c.Body = m.bindWhole("value", pt, spec, fallback)
		} else {
			c.Binds, c.Body = m.bindRecord(pt.(*types.Record), spec, fallback)
		}
	case kind == MatchList && tag == "cons":
		elem := st.(*types.App).Args[0]
		var heads, tails []syntax.Pattern
		for _, r := range spec {
			if lt, ok := r.pat.(listTail); ok {
				heads = append(heads, lt.Elems[lt.from])
				tails = append(tails, listTail{lt.PList, lt.from + 1})
			} else {
				heads, tails = append(heads, nil), append(tails, nil)
			}
		}
		head, hb := m.binder(heads, elem, "head")
		rest, rb := m.binder(tails, st, "rest")
		c.Binds = append(hb, rb...)
		var rows [][]syntax.Pattern
		for i := range spec {
			rows = append(rows, []syntax.Pattern{heads[i], tails[i]})
		}
		c.Body = m.columns([]column{{head, elem}, {rest, st}}, rows, spec, fallback)
	default: // Bool, Lit, list nil: no parts to bind
		c.Body = spec[0].body
	}
	return c
}

// bindWhole binds the whole payload (Field "value" for a wrapped one, "" for a record) and
// matches the inner patterns against it.
func (m *matcher) bindWhole(field string, pt types.Type, spec []row, fallback Expr) ([]Bind, Expr) {
	pats := make([]syntax.Pattern, len(spec))
	for i, r := range spec {
		pats[i] = r.pat
	}
	v, binds := m.binder(pats, pt, field)
	if v == nil {
		return nil, spec[0].body
	}
	return binds, m.compile(v, pt, spec, fallback)
}

// bindRecord binds a record payload: field by field when the arms only use record patterns,
// else as a whole.
func (m *matcher) bindRecord(rec *types.Record, spec []row, fallback Expr) ([]Bind, Expr) {
	for _, r := range spec {
		if _, isRec := r.pat.(*syntax.PRecord); !isRec && r.pat != nil && !isWild(r.pat) {
			return m.bindWhole("", rec, spec, fallback)
		}
	}
	var cols []column
	var binds []Bind
	rows := make([][]syntax.Pattern, len(spec))
	for _, f := range rec.Fields {
		pats := make([]syntax.Pattern, len(spec))
		for i, r := range spec {
			if pr, ok := r.pat.(*syntax.PRecord); ok {
				for _, pf := range pr.Fields {
					if pf.Name == f.Name {
						pats[i] = pf.Pat
					}
				}
			}
		}
		v, b := m.binder(pats, f.Type, f.Name)
		if v == nil {
			continue
		}
		binds = append(binds, b...)
		cols = append(cols, column{v, f.Type})
		for i := range spec {
			rows[i] = append(rows[i], pats[i])
		}
	}
	return binds, m.columns(cols, rows, spec, fallback)
}

func isWild(p syntax.Pattern) bool { _, ok := p.(*syntax.PWild); return ok }

// binder names the part `field` that pats are matched against: the first arm's variable when it
// is one, else a temporary; nil when no pattern needs the part at all.
func (m *matcher) binder(pats []syntax.Pattern, t types.Type, field string) (Expr, []Bind) {
	needed := false
	for _, p := range pats {
		if p != nil && !isWild(p) {
			needed = true
		}
	}
	if !needed {
		return nil, nil
	}
	var name string
	switch p := pats[0].(type) {
	case *syntax.PVar:
		name = p.Name
	case listTail:
		if p.from == len(p.Elems) && p.Rest != "" {
			name = p.Rest
		}
	}
	if name == "" {
		name = m.l.temp()
	}
	return local(name, t), []Bind{{Name: name, Field: field, Type: t}}
}

type column struct {
	subj Expr
	t    types.Type
}

// columns matches rows of patterns (one per column) in order: the first row whose every
// column matches runs its body; a failing row continues with the next, then fallback.
func (m *matcher) columns(cols []column, rows [][]syntax.Pattern, spec []row, fallback Expr) Expr {
	if len(spec) == 0 {
		return fallback
	}
	var fail Expr
	for _, p := range rows[0] {
		if !irrefutable(p) {
			fail = m.columns(cols, rows[1:], spec[1:], fallback)
			break
		}
	}
	e := spec[0].body
	for j := len(cols) - 1; j >= 0; j-- {
		e = m.compile(cols[j].subj, cols[j].t, []row{{rows[0][j], e}}, fail)
	}
	return e
}
