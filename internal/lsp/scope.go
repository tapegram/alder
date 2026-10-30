package lsp

import (
	"strings"

	"alder/internal/diag"
	"alder/internal/syntax"
)

// binder is a local name in scope at a position.
type binder struct {
	Name string
	Key  any       // the check.Result.Binds key: *syntax.Ident, *syntax.LetStmt, *syntax.PVar, *syntax.PList, *syntax.HtmlFor
	Span diag.Span // where it is bound: the name itself
}

// scopeAt is the local names in scope at pos, given NodeAt's path to it (path[0] the module):
// value and lambda parameters, the lets of a block that end before pos, an arm's pattern
// variables inside its body and a `for`'s variable inside its body. Innermost last.
func scopeAt(path []syntax.Spanned, pos diag.Pos) []binder {
	var m *syntax.Module
	if len(path) > 0 {
		m, _ = path[0].(*syntax.Module)
	}
	var out []binder
	params := func(ps []*syntax.Ident) {
		for _, p := range ps {
			out = append(out, binder{p.Name, p, p.Span})
		}
	}
	for _, n := range path {
		switch n := n.(type) {
		case *syntax.ValueDecl:
			params(n.Params)
		case *syntax.Lambda:
			params(n.Params)
		case *syntax.Block:
			for _, st := range n.Stmts {
				if !posLess(pos, st.Span.End) {
					out = append(out, binder{st.Name, st, wordSpan(m, st.Span, len("let"), st.Name)})
				}
			}
		case *syntax.Arm:
			if n.Body != nil && syntax.Contains(n.Body.GetSpan(), pos) {
				out = patternBinders(m, n.Pat, out)
			}
		case *syntax.HtmlFor:
			if n.Body != nil && syntax.Contains(n.Body.GetSpan(), pos) {
				out = append(out, binder{n.Var, n, wordSpan(m, n.Span, 0, n.Var)})
			}
		}
	}
	return out
}

// patternBinders appends the variables p binds, in source order.
func patternBinders(m *syntax.Module, p syntax.Pattern, out []binder) []binder {
	switch p := p.(type) {
	case *syntax.PVar:
		out = append(out, binder{p.Name, p, p.Span})
	case *syntax.PCtor:
		out = patternBinders(m, p.Arg, out)
	case *syntax.PRecord:
		for _, f := range p.Fields {
			out = patternBinders(m, f.Pat, out)
		}
	case *syntax.PList:
		for _, e := range p.Elems {
			out = patternBinders(m, e, out)
		}
		if p.Rest != "" {
			out = append(out, binder{p.Rest, p, restSpan(m, p)})
		}
	}
	return out
}

// restSpan is the span of a list pattern's `...rest` name: its last whole-word occurrence in
// the pattern's text (the rest comes last), or the pattern's span when it is not found.
func restSpan(m *syntax.Module, p *syntax.PList) diag.Span {
	sp := p.Span
	if m == nil || sp.Start.Line != sp.End.Line || sp.Start.Line < 1 || sp.Start.Line > len(m.Lines) {
		return sp
	}
	line := m.Lines[sp.Start.Line-1]
	lo, hi := sp.Start.Col-1, min(sp.End.Col-1, len(line))
	if lo < 0 || lo > hi {
		return sp
	}
	for i := hi - len(p.Rest); i >= lo; i-- {
		if isWordAt(line, i, p.Rest) {
			return lineSpan(sp.File, sp.Start.Line, i+1, len(p.Rest))
		}
	}
	return sp
}

// innermost is the last binder named name, or nil.
func innermost(bs []binder, name string) *binder {
	for i := len(bs) - 1; i >= 0; i-- {
		if bs[i].Name == name {
			return &bs[i]
		}
	}
	return nil
}

// at is the last good analysis and the place in it that p names in rel: the module, NodeAt's
// path and the compiler position. ok is false when there is no good analysis, when rel's lines
// up to p's (with it when inclusive) have changed since it was built (Ruling R11), or when the
// analysis has no module for rel.
func (s *server) at(rel string, p Position, inclusive bool) (a *analysis, m *syntax.Module, path []syntax.Spanned, pos diag.Pos, ok bool) {
	if p.Line < 0 || p.Character < 0 {
		return nil, nil, nil, pos, false
	}
	p = s.byteAt(rel, p)
	a = s.lastGood()
	if !s.fresh(a, rel, p.Line+1, inclusive) {
		return nil, nil, nil, pos, false
	}
	for _, x := range a.prog.Modules {
		if x.File == rel {
			m = x
			break
		}
	}
	if m == nil {
		return nil, nil, nil, pos, false
	}
	pos = diag.Pos{Line: p.Line + 1, Col: p.Character + 1}
	return a, m, syntax.NodeAt(m, pos), pos, true
}

// declAt is m's top-level declaration whose span contains pos, or nil.
func declAt(m *syntax.Module, pos diag.Pos) syntax.Decl {
	for _, d := range m.Decls {
		if syntax.Contains(d.GetSpan(), pos) {
			return d
		}
	}
	return nil
}

// declName is the name a declaration (or a union's constructor, or a field) declares, and how
// far into its first line the name may start (past its keyword); ok is false for one without a
// name (a page, a session, …).
func declName(d syntax.Spanned) (name string, skip int, ok bool) {
	switch d := d.(type) {
	case *syntax.ValueDecl:
		return d.Name, 0, true
	case *syntax.TypeDecl:
		return d.Name, len("type"), true
	case *syntax.TableDecl:
		return d.Name, len("table"), true
	case *syntax.FormDecl:
		return d.Name, len("form"), true
	case *syntax.MsgDecl:
		return d.Name, len("msg"), true
	case *syntax.CapDecl:
		return d.Name, len("capability"), true
	case *syntax.FakeDecl:
		return d.Name, len("fake"), true
	case *syntax.Ctor:
		return d.Name, 0, true
	case *syntax.Field:
		return d.Name, 0, true
	}
	return "", 0, false
}

// nameSpan is the span of the name d declares: from the declaration's start (a value's starts
// at the name of its signature) for a value, a constructor or a field, else the name's first
// whole-word occurrence on the declaration's first line past its keyword. A declaration
// without a name gives its first line's start to the end of its first word.
func nameSpan(d syntax.Spanned, m *syntax.Module) diag.Span {
	sp := d.GetSpan()
	if fd, ok := d.(*syntax.FakeDecl); ok && fd.NameSpan.Start.Line > 0 {
		return fd.NameSpan
	}
	name, skip, ok := declName(d)
	if !ok {
		n := 0
		if m != nil && sp.Start.Line >= 1 && sp.Start.Line <= len(m.Lines) {
			line := m.Lines[sp.Start.Line-1]
			for i := sp.Start.Col - 1; i >= 0 && i < len(line) && isIdentByte(line[i]); i++ {
				n++
			}
		}
		return lineSpan(sp.File, sp.Start.Line, sp.Start.Col, n)
	}
	if skip == 0 {
		return lineSpan(sp.File, sp.Start.Line, sp.Start.Col, len(name))
	}
	return wordSpan(m, sp, skip, name)
}

// wordSpan is the span of name's first whole-word occurrence on sp's first line, at least skip
// bytes past sp's start; sp's start (name's length long) when it is not found.
func wordSpan(m *syntax.Module, sp diag.Span, skip int, name string) diag.Span {
	if m != nil && sp.Start.Line >= 1 && sp.Start.Line <= len(m.Lines) {
		line := m.Lines[sp.Start.Line-1]
		for i := max(sp.Start.Col-1+skip, 0); i+len(name) <= len(line); i++ {
			if isWordAt(line, i, name) {
				return lineSpan(sp.File, sp.Start.Line, i+1, len(name))
			}
		}
	}
	return lineSpan(sp.File, sp.Start.Line, sp.Start.Col, len(name))
}

// lineSpan is n bytes from line:col.
func lineSpan(file string, line, col, n int) diag.Span {
	return diag.Span{File: file, Start: diag.Pos{Line: line, Col: col}, End: diag.Pos{Line: line, Col: col + n}}
}

// isWordAt reports whether name occurs in line at byte i as a whole word.
func isWordAt(line string, i int, name string) bool {
	if i < 0 || !strings.HasPrefix(line[i:], name) {
		return false
	}
	if i > 0 && isIdentByte(line[i-1]) {
		return false
	}
	end := i + len(name)
	return end >= len(line) || !isIdentByte(line[end])
}

func isIdentByte(b byte) bool {
	return b == '_' || '0' <= b && b <= '9' || 'a' <= b && b <= 'z' || 'A' <= b && b <= 'Z'
}

func posLess(a, b diag.Pos) bool { return a.Line < b.Line || a.Line == b.Line && a.Col < b.Col }

// target is what the cursor names: a declaration (ref), a local (local, with ref its RefLocal),
// or a module (ref of Kind RefNone with only Module set). ok is false when it names nothing.
//   - a Var, CtorRef, TypeName, PCtor or Element with a resolved Ref gives it; on a qualifier
//     that names a module (`Post.` of `Post.title`), the module;
//   - a RefLocal gives the innermost binder of that name in scope;
//   - the cursor on a local's binding site (a parameter, `let x`, a pattern variable, `for x`)
//     gives that binder;
//   - the cursor on a declaration's own name (or a union constructor's, or a value's on its
//     definition line) gives the declaration;
//   - an import gives the module.
func target(a *analysis, m *syntax.Module, path []syntax.Spanned, pos diag.Pos) (ref syntax.Ref, local *binder, ok bool) {
	if len(path) == 0 {
		return syntax.Ref{}, nil, false
	}
	resolved := func(r syntax.Ref, name string) (syntax.Ref, *binder, bool) {
		switch r.Kind {
		case syntax.RefLocal:
			if b := innermost(scopeAt(path, pos), name); b != nil {
				return r, b, true
			}
			return syntax.Ref{}, nil, false
		case syntax.RefNone:
			return syntax.Ref{}, nil, false
		}
		return r, nil, true
	}
	switch n := path[len(path)-1].(type) {
	case *syntax.Import:
		return syntax.Ref{Kind: syntax.RefNone, Module: n.Module}, nil, true
	case *syntax.Var:
		if onQualifier(a, m, n.Module, n.Span, pos) {
			return syntax.Ref{Kind: syntax.RefNone, Module: qualifiedModule(a, m, n.Module)}, nil, true
		}
		return resolved(n.Ref, n.Name)
	case *syntax.CtorRef:
		if onQualifier(a, m, n.Module, n.Span, pos) {
			return syntax.Ref{Kind: syntax.RefNone, Module: qualifiedModule(a, m, n.Module)}, nil, true
		}
		return resolved(n.Ref, n.Name)
	case *syntax.TypeName:
		if onQualifier(a, m, n.Module, n.Span, pos) {
			return syntax.Ref{Kind: syntax.RefNone, Module: qualifiedModule(a, m, n.Module)}, nil, true
		}
		return resolved(n.Ref, n.Name)
	case *syntax.PCtor:
		return resolved(n.Ref, n.Name)
	case *syntax.Element:
		if n.Ref.Kind != syntax.RefNone {
			return n.Ref, nil, true
		}
	case *syntax.Ctor:
		if syntax.Contains(nameSpan(n, m), pos) {
			return syntax.Ref{Kind: syntax.RefCtor, Module: m.Name, Name: n.Name}, nil, true
		}
	default:
		if b := binderAt(m, n, pos); b != nil {
			return syntax.Ref{Kind: syntax.RefLocal, Name: b.Name}, b, true
		}
	}
	if d := declAt(m, pos); d != nil && (syntax.Contains(nameSpan(d, m), pos) || syntax.Contains(defNameSpan(d, m), pos)) {
		if ref, ok := declRef(m.Name, d); ok {
			return ref, nil, true
		}
	}
	return syntax.Ref{}, nil, false
}

// binderAt is the binder that node n introduces when pos is on its name: a parameter, a let, a
// pattern variable, a list pattern's rest or a `for`'s variable; nil otherwise.
func binderAt(m *syntax.Module, n syntax.Spanned, pos diag.Pos) *binder {
	var b binder
	switch n := n.(type) {
	case *syntax.Ident:
		b = binder{n.Name, n, n.Span}
	case *syntax.LetStmt:
		b = binder{n.Name, n, wordSpan(m, n.Span, len("let"), n.Name)}
	case *syntax.PVar:
		b = binder{n.Name, n, n.Span}
	case *syntax.PList:
		if n.Rest == "" {
			return nil
		}
		b = binder{n.Rest, n, restSpan(m, n)}
	case *syntax.HtmlFor:
		b = binder{n.Var, n, wordSpan(m, n.Span, 0, n.Var)}
	default:
		return nil
	}
	if !syntax.Contains(b.Span, pos) {
		return nil
	}
	return &b
}

// defNameSpan is, for a value with a signature, the name starting its definition line
// (`load params = …` under `load : …`); a zero span otherwise.
func defNameSpan(d syntax.Decl, m *syntax.Module) diag.Span {
	vd, ok := d.(*syntax.ValueDecl)
	if !ok || vd.Sig == nil || m == nil {
		return diag.Span{}
	}
	for l := vd.Sig.GetSpan().End.Line + 1; l <= vd.Span.End.Line && l <= len(m.Lines); l++ {
		if isWordAt(m.Lines[l-1], 0, vd.Name) {
			return lineSpan(vd.Span.File, l, 1, len(vd.Name))
		}
	}
	return diag.Span{}
}

// declRef is the Ref the resolver gives a use of declaration d of module mod.
func declRef(mod string, d syntax.Decl) (syntax.Ref, bool) {
	switch d := d.(type) {
	case *syntax.ValueDecl:
		return syntax.Ref{Kind: syntax.RefValue, Module: mod, Name: d.Name}, true
	case *syntax.TypeDecl:
		return syntax.Ref{Kind: syntax.RefType, Module: mod, Name: d.Name}, true
	case *syntax.TableDecl:
		return syntax.Ref{Kind: syntax.RefType, Module: mod, Name: d.Name}, true
	case *syntax.FormDecl:
		return syntax.Ref{Kind: syntax.RefType, Module: mod, Name: d.Name}, true
	case *syntax.MsgDecl:
		return syntax.Ref{Kind: syntax.RefCtor, Module: mod, Name: d.Name}, true
	case *syntax.CapDecl:
		return syntax.Ref{Kind: syntax.RefCapability, Module: mod, Name: mod + "." + d.Name}, true
	}
	return syntax.Ref{}, false
}

// onQualifier reports whether pos is on the module qualifier of a name spanning sp (`Post` of
// `Post.title`), and the qualifier names a module of the program or the standard library, as m
// imports it. The qualifier's length is the written text's, whatever module it names.
func onQualifier(a *analysis, m *syntax.Module, module string, sp diag.Span, pos diag.Pos) bool {
	if module == "" || pos.Line != sp.Start.Line || pos.Col < sp.Start.Col || pos.Col >= sp.Start.Col+len(module) {
		return false
	}
	return moduleNamed(a, m, module) != nil
}

// qualifiedModule is the full path of the module that qualifier q names in m (q itself when it
// names none), so a Ref made of it matches the resolver's.
func qualifiedModule(a *analysis, m *syntax.Module, q string) string {
	if mod := moduleNamed(a, m, q); mod != nil {
		return mod.Name
	}
	return q
}

// moduleNamed is the module a written name q means in m: an import's qualifier (design rule 1),
// else a module's full path (a page value's, or a full path the resolver reports as E0205), else
// a standard-library module; nil when it is none. m may be nil, which leaves the qualifiers out.
func moduleNamed(a *analysis, m *syntax.Module, q string) *syntax.Module {
	if m != nil {
		for path, qual := range a.prog.Qualifiers(m.Name) {
			if qual == q && qual != "" {
				if mod := a.prog.Modules[path]; mod != nil {
					return mod
				}
				return a.prog.Std[path]
			}
		}
	}
	return modulePath(a, q)
}

// modulePath is the app or standard-library module with full path name, or nil.
func modulePath(a *analysis, name string) *syntax.Module {
	if m := a.prog.Modules[name]; m != nil {
		return m
	}
	return a.prog.Std[name]
}

// findDecl is the node that declares ref in its module (app or standard library): a value, a
// type, table, form or message, a union constructor, a page's `page` line, or a capability or
// its entry. nil when there is none.
func findDecl(a *analysis, ref syntax.Ref) (*syntax.Module, syntax.Spanned) {
	mod := modulePath(a, ref.Module)
	if mod == nil {
		return nil, nil
	}
	for _, d := range mod.Decls {
		switch d := d.(type) {
		case *syntax.ValueDecl:
			if (ref.Kind == syntax.RefValue || ref.Kind == syntax.RefStd || ref.Kind == syntax.RefComponent) && d.Name == ref.Name {
				return mod, d
			}
		case *syntax.TypeDecl:
			if ref.Kind == syntax.RefType && d.Name == ref.Name {
				return mod, d
			}
			if ref.Kind == syntax.RefCtor {
				for _, c := range d.Union {
					if c.Name == ref.Name {
						return mod, c
					}
				}
			}
		case *syntax.TableDecl:
			if (ref.Kind == syntax.RefType || ref.Kind == syntax.RefTable) && d.Name == ref.Name {
				return mod, d
			}
		case *syntax.FormDecl:
			if ref.Kind == syntax.RefType && d.Name == ref.Name {
				return mod, d
			}
		case *syntax.MsgDecl:
			if ref.Kind == syntax.RefCtor && d.Name == ref.Name {
				return mod, d
			}
		case *syntax.PageDecl:
			if ref.Kind == syntax.RefPage {
				return mod, d
			}
		}
	}
	if ref.Kind == syntax.RefCapability {
		return mod, findCapability(mod, ref.Name)
	}
	return mod, nil
}

// findCapability finds a capability Ref's Name in mod: "M.Cap" names capability Cap, and
// "Cap.entry" (or "M.entry" for `capability M`) an entry of one.
func findCapability(mod *syntax.Module, name string) syntax.Spanned {
	pre, last := "", name
	if i := strings.LastIndex(name, "."); i >= 0 {
		pre, last = name[:i], name[i+1:]
	}
	var entry syntax.Spanned
	for _, d := range mod.Decls {
		c, ok := d.(*syntax.CapDecl)
		if !ok {
			continue
		}
		if c.Name == last {
			return c
		}
		for _, f := range c.Entries {
			if f.Name == last && (entry == nil || c.Name == pre) {
				entry = f
			}
		}
	}
	return entry
}
