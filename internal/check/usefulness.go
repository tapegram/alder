package check

import (
	"fmt"
	"strings"

	"alder/internal/syntax"
	"alder/internal/types"
)

// Exhaustiveness and redundancy use the usefulness algorithm on pattern matrices (Maranget,
// "Warnings for pattern matching", #2). Every pattern is first normalised to one shape, a tag
// with arguments or a wildcard, driven by the type of the value it matches:
//
//	union   one tag per constructor, declaration order; arity 1 with a payload, else 0
//	Bool    True, False
//	List a  "[]" (arity 0) and "::" (head a, tail List a); [p1, p2, ..r] is p1 :: p2 :: r
//	record  one tag, the record itself; its fields in the type's order, unnamed ones wildcards
//	others  literals are tags of an infinite signature; any other type has no tags
//
// useful(P, q) says whether some value matches q but no row of P. Arm i is unreachable when its
// pattern is not useful against the arms above it; a match is exhaustive when a wildcard is not
// useful against all its arms, and missing builds a witness, a pattern of values no arm matches.

const (
	tagNil    = "[]"
	tagCons   = "::"
	tagRecord = "{}"
	tagBad    = "!" // a pattern that does not fit its type: an error is already reported
)

// pat is a normalised pattern: tag "" is a wildcard.
type pat struct {
	tag    string
	args   []*pat
	fields []string // a record's field names, parallel to args
	union  bool     // tag is a union constructor
}

var wildPat = &pat{}

func (p *pat) wild() bool { return p.tag == "" }

// bad reports whether p has a part that does not fit its type.
func (p *pat) bad() bool {
	if p.tag == tagBad {
		return true
	}
	for _, a := range p.args {
		if a.bad() {
			return true
		}
	}
	return false
}

type sigCtor struct {
	tag    string
	args   []types.Type
	fields []string
	union  bool
}

// signature is the set of tags a type's values can have; finite is false for literal types
// (their set is infinite) and for types patterns cannot take apart.
type signature struct {
	ctors  []sigCtor
	finite bool
}

func (s signature) args(tag string) []types.Type {
	for _, c := range s.ctors {
		if c.tag == tag {
			return c.args
		}
	}
	return nil
}

func (s signature) complete(h map[string]bool) bool {
	if !s.finite {
		return false
	}
	for _, c := range s.ctors {
		if !h[c.tag] {
			return false
		}
	}
	return true
}

// matcher runs usefulness; apply resolves a type through the checker's substitution.
type matcher struct {
	apply func(types.Type) types.Type
}

func (m *matcher) signature(t types.Type) signature {
	switch t := m.apply(t).(type) {
	case *types.Union:
		s := signature{finite: true}
		for _, c := range t.Constructors() {
			sc := sigCtor{tag: c.Name, union: true}
			if arg, _ := types.CtorArg(t, c.Name); arg != nil {
				sc.args = []types.Type{arg}
			}
			s.ctors = append(s.ctors, sc)
		}
		return s
	case *types.Con:
		if t.Name == "Bool" {
			return signature{finite: true, ctors: []sigCtor{{tag: "True"}, {tag: "False"}}}
		}
	case *types.App:
		if t.Con == "List" && len(t.Args) == 1 {
			return signature{finite: true, ctors: []sigCtor{{tag: tagNil}, {tag: tagCons, args: []types.Type{t.Args[0], t}}}}
		}
	case *types.Record:
		c := sigCtor{tag: tagRecord}
		for _, f := range t.Fields {
			c.args = append(c.args, f.Type)
			c.fields = append(c.fields, f.Name)
		}
		return signature{finite: true, ctors: []sigCtor{c}}
	}
	return signature{}
}

// normalise turns p, a pattern for a value of type t, into a pat. A part of the poison type is
// a wildcard; a part that does not fit its type (already reported) is a tag no type has.
func (m *matcher) normalise(p syntax.Pattern, t types.Type) *pat {
	t = m.apply(t)
	if types.IsErr(t) {
		return wildPat
	}
	bad := &pat{tag: tagBad}
	switch p := p.(type) {
	case nil, *syntax.PWild, *syntax.PVar:
		return wildPat
	case *syntax.PLit:
		if b, ok := p.Lit.(*syntax.BoolLit); ok {
			if c, isCon := t.(*types.Con); !isCon || c.Name != "Bool" {
				return bad
			}
			return &pat{tag: map[bool]string{true: "True", false: "False"}[b.Value]}
		}
		if m.signature(t).finite {
			return bad
		}
		return &pat{tag: litKey(p.Lit)}
	case *syntax.PCtor:
		un, ok := t.(*types.Union)
		if !ok {
			return bad
		}
		arg, found := types.CtorArg(un, p.Name)
		if !found || (arg == nil) != (p.Arg == nil) {
			return bad
		}
		if arg == nil {
			return &pat{tag: p.Name, union: true}
		}
		return &pat{tag: p.Name, union: true, args: []*pat{m.normalise(p.Arg, arg)}}
	case *syntax.PRecord:
		rec, ok := t.(*types.Record)
		if !ok {
			return bad
		}
		named := map[string]syntax.Pattern{}
		for _, pf := range p.Fields {
			named[pf.Name] = pf.Pat
		}
		r := &pat{tag: tagRecord}
		for _, f := range rec.Fields {
			sub := wildPat
			if fp, ok := named[f.Name]; ok {
				sub = m.normalise(fp, f.Type)
				delete(named, f.Name)
			}
			r.args = append(r.args, sub)
			r.fields = append(r.fields, f.Name)
		}
		if len(named) > 0 { // a field the type does not have
			return bad
		}
		return r
	case *syntax.PList:
		app, ok := t.(*types.App)
		if !ok || app.Con != "List" || len(app.Args) != 1 {
			return bad
		}
		tail := &pat{tag: tagNil}
		if p.Rest != "" {
			tail = wildPat
		}
		for i := len(p.Elems) - 1; i >= 0; i-- {
			tail = &pat{tag: tagCons, args: []*pat{m.normalise(p.Elems[i], app.Args[0]), tail}}
		}
		return tail
	}
	return bad
}

// litKey is a literal pattern's tag: equal literals have equal tags.
func litKey(e syntax.Expr) string {
	switch l := e.(type) {
	case *syntax.IntLit:
		return fmt.Sprintf("int %d", l.Value)
	case *syntax.FloatLit:
		return fmt.Sprintf("float %v", l.Value)
	case *syntax.StrLit:
		var b strings.Builder
		for _, part := range l.Parts {
			if part.Expr != nil {
				return fmt.Sprintf("expr %p", l) // equal to no other literal
			}
			b.WriteString(part.Text)
		}
		return fmt.Sprintf("text %q", b.String())
	}
	return fmt.Sprintf("expr %p", e)
}

func wilds(n int) []*pat {
	out := make([]*pat, n)
	for i := range out {
		out[i] = wildPat
	}
	return out
}

// fit returns p's arguments as exactly n patterns (a bad tag's arity is 0).
func fit(p *pat, n int) []*pat {
	if len(p.args) == n {
		return p.args
	}
	return wilds(n)
}

func concat[T any](a, b []T) []T {
	return append(append(make([]T, 0, len(a)+len(b)), a...), b...)
}

// specialise is S(tag, P): the rows whose first column matches tag, that column replaced by
// its n arguments.
func specialise(rows [][]*pat, tag string, n int) [][]*pat {
	var out [][]*pat
	for _, r := range rows {
		switch {
		case r[0].wild():
			out = append(out, concat(wilds(n), r[1:]))
		case r[0].tag == tag:
			out = append(out, concat(fit(r[0], n), r[1:]))
		}
	}
	return out
}

// defaults is D(P): the rows whose first column is a wildcard, without it.
func defaults(rows [][]*pat) [][]*pat {
	var out [][]*pat
	for _, r := range rows {
		if r[0].wild() {
			out = append(out, r[1:])
		}
	}
	return out
}

// heads is the set of tags in the first column of rows.
func heads(rows [][]*pat) map[string]bool {
	h := map[string]bool{}
	for _, r := range rows {
		if !r[0].wild() {
			h[r[0].tag] = true
		}
	}
	return h
}

// useful reports whether some value of types tys matches q but no row of rows.
func (m *matcher) useful(rows [][]*pat, q []*pat, tys []types.Type) bool {
	if len(q) == 0 {
		return len(rows) == 0
	}
	s := m.signature(tys[0])
	if !q[0].wild() {
		args := s.args(q[0].tag)
		return m.useful(specialise(rows, q[0].tag, len(args)), concat(fit(q[0], len(args)), q[1:]), concat(args, tys[1:]))
	}
	if s.complete(heads(rows)) {
		for _, c := range s.ctors {
			if m.useful(specialise(rows, c.tag, len(c.args)), concat(wilds(len(c.args)), q[1:]), concat(c.args, tys[1:])) {
				return true
			}
		}
		return false
	}
	return m.useful(defaults(rows), q[1:], tys[1:])
}

// missing returns a witness: values of types tys, as patterns, that no row matches. A column
// whose tags are incomplete names its first missing constructor in signature order when the
// rows use a tag there, or at the top of the subject (so a match without arms lists every
// constructor); otherwise it is a wildcard.
func (m *matcher) missing(rows [][]*pat, tys []types.Type, top bool) ([]*pat, bool) {
	if len(tys) == 0 {
		return nil, len(rows) == 0
	}
	s := m.signature(tys[0])
	h := heads(rows)
	if s.complete(h) {
		for _, c := range s.ctors {
			n := len(c.args)
			if w, ok := m.missing(specialise(rows, c.tag, n), concat(c.args, tys[1:]), false); ok {
				head := &pat{tag: c.tag, args: w[:n], fields: c.fields, union: c.union}
				return concat([]*pat{head}, w[n:]), true
			}
		}
		return nil, false
	}
	w, ok := m.missing(defaults(rows), tys[1:], false)
	if !ok {
		return nil, false
	}
	head := wildPat
	if s.finite && (len(h) > 0 || top) {
		for _, c := range s.ctors {
			if !h[c.tag] {
				head = &pat{tag: c.tag, args: wilds(len(c.args)), fields: c.fields, union: c.union}
				break
			}
		}
	}
	return concat([]*pat{head}, w), true
}

// witnesses lists up to max patterns of values of type t that no row matches, each covering
// what the earlier ones do not (so they are disjoint); more reports that there are others.
func (m *matcher) witnesses(rows [][]*pat, t types.Type, max int) (ws []*pat, more bool) {
	tys := []types.Type{t}
	rows = append([][]*pat(nil), rows...)
	for {
		w, ok := m.missing(rows, tys, true)
		if !ok {
			return ws, false
		}
		if len(ws) == max {
			return ws, true
		}
		ws = append(ws, w[0])
		rows = append(rows, w)
	}
}

// printWitness prints a witness as a pattern. At the top an unconstrained payload is named
// (`Name x`, `Name p` in an update) and an unconstrained cons is `[x, ..rest]`, the spellings
// flat matches always had; below the top an unconstrained part is `_` and a tail `..rest`.
func printWitness(p *pat, payload string) string {
	switch {
	case p.union && len(p.args) == 1 && p.args[0].wild():
		return p.tag + " " + payload
	case p.tag == tagCons && p.args[0].wild() && p.args[1].wild():
		return "[x, ..rest]"
	}
	return printPat(p)
}

func printPat(p *pat) string {
	switch {
	case p.wild():
		return "_"
	case p.tag == tagNil:
		return "[]"
	case p.tag == tagCons:
		var elems []string
		for p.tag == tagCons {
			elems = append(elems, printPat(p.args[0]))
			p = p.args[1]
		}
		if p.wild() {
			elems = append(elems, "..rest")
		}
		return "[" + strings.Join(elems, ", ") + "]"
	case p.tag == tagRecord:
		var fs []string
		for i, a := range p.args {
			if !a.wild() {
				fs = append(fs, p.fields[i]+": "+printPat(a))
			}
		}
		if len(fs) == 0 {
			return "_"
		}
		return "{ " + strings.Join(fs, ", ") + " }"
	case len(p.args) == 1:
		arg := printPat(p.args[0])
		if a := p.args[0]; a.union && len(a.args) == 1 {
			arg = "(" + arg + ")"
		}
		return p.tag + " " + arg
	}
	return p.tag
}
