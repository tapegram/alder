package fill

import (
	"slices"
	"strings"

	"alder/internal/diag"
	"alder/internal/generate"
	"alder/internal/syntax"
)

// stdEffects are the standard library's effects that reach beyond the page and its user's
// rows, by module: any HTTP request, a change to the session, raw HTML, a pause, and a database
// write not scoped to the session (the `…Mine` and `…Scoped` writes keep to its rows). "*" is
// every value of the module.
var stdEffects = map[string][]string{
	"Http":    {"*"},
	"Session": {"put"},
	"Html":    {"unsafe"},
	"Task":    {"sleep"},
	"Db":      {"insert", "update", "delete"},
}

func stdEffect(module, name string) bool {
	for _, n := range stdEffects[module] {
		if n == "*" || n == name {
			return true
		}
	}
	return false
}

// Effects are what the expression at span (in p, a checked program that contains it: the
// generate.Probe of the fill's edits) does that its hole's type does not limit, one line each,
// sorted: a capability entry it calls (`capability Email.send`; each runs host Go, Query or
// Task alike), a standard library effect of stdEffects (`Http.post`), and an app value it
// references whose body reaches one of those, directly or through other values, with the path
// (`Mail.notify → Email.send`). Names are the checker's resolution, so an alias, a helper in
// another module or a chain of helpers cannot hide one; a cycle of values is walked once.
func Effects(p *generate.Program, span diag.Span) []string {
	out, _ := effectsAt(p, span)
	return out
}

// effectsAt is Effects, with the lines of span (0 its first) where each flagged reference is.
func effectsAt(p *generate.Program, span diag.Span) (out []string, lines []int) {
	mod := moduleOfFile(p, span.File)
	if mod == nil {
		return nil, nil
	}
	w := &effectWalk{p: p, memo: map[string][]string{}, busy: map[string]int{}}
	seen := map[string]bool{}
	var visit func(n syntax.Spanned)
	visit = func(n syntax.Spanned) {
		if n == nil {
			return
		}
		sp := n.GetSpan()
		if inSpan(sp, span) {
			if path := w.ref(refOf(n)); path != nil {
				line := strings.Join(path, " → ")
				if len(path) == 1 && strings.HasPrefix(path[0], "capability ") {
					line = path[0]
				}
				if !seen[line] {
					seen[line] = true
					out = append(out, line)
				}
				lines = append(lines, sp.Start.Line-span.Start.Line)
			}
		}
		for _, c := range syntax.Children(n) {
			if overlaps(c.GetSpan(), span) {
				visit(c)
			}
		}
	}
	visit(mod)
	slices.Sort(out)
	return out, lines
}

// Excerpt is text (the expression as placed) for a question about its effects: whole when it has
// fewer than 40 lines, else the lines at (0-based) that hold a flagged reference, `…` between
// and around runs of lines left out, so no flagged call is cut away.
func Excerpt(text string, at []int) string {
	ls := strings.Split(text, "\n")
	if len(ls) < 40 {
		return text
	}
	keep := map[int]bool{}
	for _, i := range at {
		if i >= 0 && i < len(ls) {
			keep[i] = true
		}
	}
	var out []string
	gap := false
	for i, l := range ls {
		if keep[i] {
			if gap || (len(out) == 0 && i > 0) {
				out = append(out, "…")
			}
			out = append(out, l)
			gap = false
		} else if len(out) > 0 {
			gap = true
		}
	}
	if gap || len(out) == 0 {
		out = append(out, "…")
	}
	return strings.Join(out, "\n")
}

// effectWalk memoises, per app value ("Module.name"), the first path by which its body reaches
// an effect (nil: none). A value being walked (busy, at its depth on the walk) counts as none,
// so a cycle ends; but a "none" found while an ancestor was busy is only provisional (that
// ancestor's own effect, found later, may reach it), so it is remembered only by the value that
// opened the cycle, once its whole body is walked: each strongly connected group of values is
// settled at its root, as Tarjan's algorithm settles it.
type effectWalk struct {
	p    *generate.Program
	memo map[string][]string
	busy map[string]int // the values being walked, by their depth
}

// settled is the low mark of a walk that met no busy value.
const settled = int(^uint(0) >> 1)

// ref is the path from r to an effect: [capability X.y], [Http.get], or [Mod.v, …] through app
// values; nil when r reaches none.
func (w *effectWalk) ref(r syntax.Ref) []string {
	path, _ := w.refLow(r)
	return path
}

// refLow is ref, with the lowest depth of a busy value the walk met (settled for none): a nil
// path with a low mark under the caller's depth is provisional.
func (w *effectWalk) refLow(r syntax.Ref) ([]string, int) {
	switch r.Kind {
	case syntax.RefCapability:
		return []string{"capability " + r.Name}, settled
	case syntax.RefStd:
		if stdEffect(r.Module, r.Name) {
			return []string{r.Module + "." + r.Name}, settled
		}
	case syntax.RefValue, syntax.RefComponent:
		key := r.Module + "." + r.Name
		if path, ok := w.memo[key]; ok {
			return path, settled
		}
		if d, ok := w.busy[key]; ok {
			return nil, d
		}
		depth := len(w.busy)
		w.busy[key] = depth
		var path []string
		low := settled
		if body := w.body(r.Module, r.Name); body != nil {
			path, low = w.inside(body)
		}
		delete(w.busy, key)
		if path != nil {
			last := path[len(path)-1]
			last = strings.TrimPrefix(last, "capability ")
			path = append([]string{key}, append(path[:len(path)-1:len(path)-1], last)...)
			w.memo[key] = path // an effect found is an effect, whatever was busy
			return path, settled
		}
		if low >= depth { // only this value, or values under it, were busy: none, for good
			w.memo[key] = nil
			return nil, settled
		}
		return nil, low // provisional: an ancestor still being walked may yet reach an effect
	}
	return nil, settled
}

// inside is the first path to an effect from the references in n's subtree, and the lowest
// busy depth met on the way.
func (w *effectWalk) inside(n syntax.Spanned) ([]string, int) {
	path, low := w.refLow(refOf(n))
	if path != nil {
		return path, low
	}
	for _, c := range syntax.Children(n) {
		p, l := w.inside(c)
		low = min(low, l)
		if p != nil {
			return p, low
		}
	}
	return nil, low
}

// body is the body of the value name of module mod, nil when there is none (a stdlib value).
func (w *effectWalk) body(mod, name string) syntax.Spanned {
	m := w.p.Prog.Modules[mod]
	if m == nil {
		return nil
	}
	for _, d := range m.Decls {
		if v, ok := d.(*syntax.ValueDecl); ok && v.Name == name && v.Body != nil {
			return v.Body
		}
	}
	return nil
}

// refOf is the resolved reference a node carries: a name's, or a component element's.
func refOf(n syntax.Spanned) syntax.Ref {
	switch n := n.(type) {
	case *syntax.Var:
		return n.Ref
	case *syntax.Element:
		return n.Ref
	}
	return syntax.Ref{}
}

func moduleOfFile(p *generate.Program, file string) *syntax.Module {
	for _, m := range p.Prog.Modules {
		if m.File == file {
			return m
		}
	}
	return nil
}

func before(a, b diag.Pos) bool { return a.Line < b.Line || a.Line == b.Line && a.Col < b.Col }

// inSpan reports whether sp starts inside outer.
func inSpan(sp, outer diag.Span) bool {
	return !before(sp.Start, outer.Start) && before(sp.Start, outer.End)
}

// overlaps reports whether sp and outer share a position.
func overlaps(sp, outer diag.Span) bool {
	return before(sp.Start, outer.End) && before(outer.Start, sp.End)
}
