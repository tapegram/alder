package fill

import (
	_ "embed"
	"fmt"
	"regexp"
	"slices"
	"strings"

	"alder/internal/diag"
	"alder/internal/generate"
	"alder/internal/syntax"
	"alder/internal/types"
	"alder/stdlib"
)

// Card is the system prompt: what a hole is, the expression syntax, the idioms the holes of
// generated code need, and the rules of the answer.
//
//go:embed card.md
var Card string

// maxScopeLines caps the `Names in scope` section.
const maxScopeLines = 200

// Prompt is the first user turn for hole h of p: where the hole is and its type, the comment
// lines directly above its declaration, the module's source whole, the names in scope (the
// binders at the hole, the module's values, the values and types of the app modules it imports;
// those whose type fits the hole first), the standard library signature files the hole's type
// and the module name (Task and Prelude always, Test in a test), and the closing instruction.
// Nothing of another module but its names and types is in it.
func Prompt(p *generate.Program, h generate.Hole) string {
	m := p.Modules[h.Module]
	var b strings.Builder
	fmt.Fprintf(&b, "Fill the typed hole ?%s in %s of module %s (%s).\nIt must have type: %s\n", h.Name, h.DeclText(), h.Module, h.File, h.Type)
	if m == nil {
		b.WriteString("\nReply with the expression only.\n")
		return b.String()
	}
	if c := commentAbove(m, h); c != "" {
		fmt.Fprintf(&b, "\nThe comment above the declaration:\n%s\n", c)
	}
	fmt.Fprintf(&b, "\nThe module, %s:\n```alder\n%s\n```\n", m.File, strings.TrimRight(m.Source, "\n"))
	if lines := scopeLines(p, m, h); len(lines) > 0 {
		fmt.Fprintf(&b, "\nNames in scope (those whose type fits the hole first):\n%s\n", strings.Join(lines, "\n"))
	}
	if std := stdFiles(p, m, h); std != "" {
		fmt.Fprintf(&b, "\nStandard library:\n%s", std)
	}
	b.WriteString("\nReply with the expression only.\n")
	return b.String()
}

// RetryTurn is the user turn after an answer the checker refused: its errors, as `alder check`
// prints them.
func RetryTurn(errors string) string {
	return "The checker refused it:\n" + strings.TrimRight(errors, "\n") + "\nReply with a corrected expression only."
}

// StripFence is the answer without surrounding whitespace and one surrounding code fence
// (```alder … ```), which models write despite the card.
func StripFence(s string) string {
	s = strings.TrimSpace(s)
	if !strings.HasPrefix(s, "```") {
		return s
	}
	nl := strings.IndexByte(s, '\n')
	if nl < 0 {
		return strings.TrimSpace(strings.Trim(s, "`"))
	}
	body := s[nl+1:]
	if !strings.HasSuffix(body, "```") {
		return s // not one fence around the whole answer: the check says what is wrong
	}
	return strings.TrimSpace(body[:len(body)-3])
}

// declOf is the declaration of m holding the hole.
func declOf(m *generate.Module, h generate.Hole) syntax.Decl {
	for _, d := range m.Syntax.Decls {
		if syntax.Contains(d.GetSpan(), h.Span.Start) {
			return d
		}
	}
	return nil
}

// commentAbove is the run of comment lines directly above the hole's declaration, as written.
func commentAbove(m *generate.Module, h generate.Hole) string {
	d := declOf(m, h)
	if d == nil {
		return ""
	}
	lines := m.Syntax.Lines
	var out []string
	for i := d.GetSpan().Start.Line - 2; i >= 0 && i < len(lines); i-- {
		if !strings.HasPrefix(strings.TrimSpace(lines[i]), "--") {
			break
		}
		out = append([]string{lines[i]}, out...)
	}
	return strings.Join(out, "\n")
}

// scopeLines are the `name : Type` lines of the names in scope at the hole, the fitting ones
// first, at most maxScopeLines.
func scopeLines(p *generate.Program, m *generate.Module, h generate.Hole) []string {
	res := p.Res
	if res == nil {
		return nil
	}
	q := p.Prog.Qualify(m.Name)
	show := func(t types.Type) string { return types.StringQ(t, m.Name, q) }
	path := syntax.NodeAt(m.Syntax, h.Span.Start)
	var want types.Type
	if he, ok := path[len(path)-1].(*syntax.HoleExpr); ok {
		want = res.Types[he]
	}
	fits := func(t types.Type) bool {
		if want == nil || t == nil || res.U == nil {
			return false
		}
		u := res.U.Clone()
		return u.Check(u.Instantiate(t), want) == nil
	}
	var fit, rest []string
	seen := map[string]bool{}
	add := func(name string, t types.Type) {
		if t == nil || seen[name] || name == "_" {
			return
		}
		seen[name] = true
		line := name + " : " + show(t)
		if fits(t) {
			fit = append(fit, line)
		} else {
			rest = append(rest, line)
		}
	}
	bs := binders(path, h.Span.Start)
	for i := len(bs) - 1; i >= 0; i-- { // innermost first
		add(bs[i].name, res.Binds[bs[i].key])
	}
	for _, d := range m.Syntax.Decls {
		if v, ok := d.(*syntax.ValueDecl); ok {
			add(v.Name, res.Values[m.Name+"."+v.Name])
		}
	}
	var typeLines []string
	for _, im := range m.Imports {
		other := p.Modules[im]
		if other == nil || p.Std[im] {
			continue
		}
		qual := q(im)
		if qual == "" {
			qual = im
		}
		for _, d := range other.Syntax.Decls {
			switch d := d.(type) {
			case *syntax.ValueDecl:
				add(qual+"."+d.Name, res.Values[im+"."+d.Name])
			case *syntax.TypeDecl:
				line := "type " + qual + "." + d.Name
				if len(d.Union) > 0 {
					var cs []string
					for _, c := range d.Union {
						s := c.Name
						if ci := res.Ctors[im+"."+c.Name]; ci != nil && ci.Arg != nil {
							s += " " + show(ci.Arg)
						}
						cs = append(cs, s)
					}
					line += " = " + strings.Join(cs, " | ")
				}
				typeLines = append(typeLines, line)
			case *syntax.TableDecl:
				sp := d.GetSpan()
				if sp.Start.Line >= 1 && sp.End.Line <= len(other.Syntax.Lines) {
					typeLines = append(typeLines, "-- in "+im+" (imported as "+qual+"):\n"+strings.Join(other.Syntax.Lines[sp.Start.Line-1:sp.End.Line], "\n"))
				}
			}
		}
	}
	slices.Sort(rest)
	out := append(append(fit, rest...), typeLines...)
	var capped []string
	n := 0
	for _, l := range out {
		k := strings.Count(l, "\n") + 1
		if n+k > maxScopeLines {
			break
		}
		capped, n = append(capped, l), n+k
	}
	return capped
}

// binder is a local name in scope and its check.Result.Binds key.
type binder struct {
	name string
	key  any
}

// binders are the local names in scope at pos, given NodeAt's path to it, innermost last: value
// and lambda parameters, the lets of a block that end before pos, an arm's pattern variables
// inside its body, a `for`'s variable inside its body (internal/lsp's scopeAt, which this
// package cannot import).
func binders(path []syntax.Spanned, pos diag.Pos) []binder {
	var out []binder
	params := func(ps []*syntax.Ident) {
		for _, p := range ps {
			out = append(out, binder{p.Name, p})
		}
	}
	before := func(a, b diag.Pos) bool { return a.Line < b.Line || a.Line == b.Line && a.Col < b.Col }
	for _, n := range path {
		switch n := n.(type) {
		case *syntax.ValueDecl:
			params(n.Params)
		case *syntax.Lambda:
			params(n.Params)
		case *syntax.Block:
			for _, st := range n.Stmts {
				if !before(pos, st.Span.End) {
					out = append(out, binder{st.Name, st})
				}
			}
		case *syntax.Arm:
			if n.Body != nil && syntax.Contains(n.Body.GetSpan(), pos) {
				out = patternBinders(n.Pat, out)
			}
		case *syntax.HtmlFor:
			if n.Body != nil && syntax.Contains(n.Body.GetSpan(), pos) {
				out = append(out, binder{n.Var, n})
			}
		}
	}
	return out
}

func patternBinders(p syntax.Pattern, out []binder) []binder {
	switch p := p.(type) {
	case *syntax.PVar:
		out = append(out, binder{p.Name, p})
	case *syntax.PCtor:
		out = patternBinders(p.Arg, out)
	case *syntax.PRecord:
		for _, f := range p.Fields {
			out = patternBinders(f.Pat, out)
		}
	case *syntax.PList:
		for _, e := range p.Elems {
			out = patternBinders(e, out)
		}
		if p.Rest != "" {
			out = append(out, binder{p.Rest, p})
		}
	}
	return out
}

// qualifierRe finds `Name.` in source and types: a module named before a member.
var qualifierRe = regexp.MustCompile(`\b([A-Z][A-Za-z0-9]*)\.`)

// coreStd are the standard modules almost every expression reaches for, always in the prompt
// (together about 6 KB) whether or not the module has written `List.` or `Text.` yet: a model
// that cannot see them leaves a hole instead of folding a list or comparing text.
var coreStd = []string{"Task", "Prelude", "List", "Maybe", "Result", "Text", "Int", "Float", "Dict", "Bool"}

// stdFiles are the standard library signature files, comments kept, of the core modules
// (coreStd) always, of any other module named in the hole's type or in the module's source,
// and Test in a test, each in a fence, in name order.
func stdFiles(p *generate.Program, m *generate.Module, h generate.Hole) string {
	want := map[string]bool{}
	for _, n := range coreStd {
		want[n] = true
	}
	if strings.HasPrefix(h.Decl, `"`) {
		want["Test"] = true
	}
	for _, src := range []string{h.Type, m.Source} {
		for _, g := range qualifierRe.FindAllStringSubmatch(src, -1) {
			if p.Std[g[1]] {
				want[g[1]] = true
			}
		}
	}
	// a name of the hole's type that is a standard module's own type (`Nav`, `Query`) is in
	// Prelude, which is always there
	var names []string
	for n := range want {
		names = append(names, n)
	}
	slices.Sort(names)
	var b strings.Builder
	for _, n := range names {
		src, err := stdlib.FS.ReadFile(n + ".alder")
		if err != nil {
			continue
		}
		fmt.Fprintf(&b, "```alder\n%s\n```\n", strings.TrimRight(string(src), "\n"))
	}
	return b.String()
}
