package mutate

import (
	"fmt"
	"strconv"
	"strings"
	"unicode/utf8"

	"alder/internal/ir"
	"alder/internal/types"
)

// Showing IR as one line of Alder-like source, for a mutant's Before and After.
//
// The rendering follows the surface syntax closely enough to be recognised (`Db.from Todo |>
// Db.where (Todo.owner.eq session.user) |> Db.all`, `{ t | done: not t.done }`), not to be
// parsed back: a match shows its arms with the names the user bound, a record literal leaves
// out the fields its type's defaults filled in, a block is `{ let x <- e; r }`. When a line is
// longer than width it is shortened by replacing its largest subexpression with `…` (a record
// with `{…}`), repeatedly, then cut.

const width = 80

// line renders e (in module home) as wrap(rendering), elided to width columns.
func line(home string, e ir.Expr, wrap func(string) string) string {
	return lineFirst(home, e, wrap)
}

// lineFirst is line, eliding the subexpressions first (in order) before any other: a guard's
// arms go before its condition.
func lineFirst(home string, e ir.Expr, wrap func(string) string, first ...ir.Expr) string {
	p := &printer{home: home, elided: map[ir.Expr]bool{}}
	for {
		s := wrap(p.expr(e))
		if utf8.RuneCountInString(s) <= width {
			return s
		}
		if len(first) > 0 {
			p.elided[first[0]] = true
			first = first[1:]
			continue
		}
		// elide the largest subexpression not elided yet (the first one, on a tie)
		var best ir.Expr
		bestLen := 1
		var visit func(x ir.Expr, top bool)
		visit = func(x ir.Expr, top bool) {
			if isNil(x) || p.elided[x] {
				return
			}
			if !top {
				if n := utf8.RuneCountInString(p.expr(x)); n > bestLen {
					best, bestLen = x, n
				}
			}
			for _, c := range ir.Children(x) {
				visit(c, false)
			}
		}
		visit(e, true)
		if best == nil {
			r := []rune(s)
			return string(r[:width-1]) + "…"
		}
		p.elided[best] = true
	}
}

// argument wraps an operand shown after prefix (`Db.limit `, `if not `): parenthesized unless
// it is atomic.
func argument(prefix string, e ir.Expr, suffix string) func(string) string {
	if atomic(e) {
		return func(s string) string { return prefix + s + suffix }
	}
	return func(s string) string { return prefix + "(" + s + ")" + suffix }
}

// text is the full rendering of e, fully qualified: two expressions with the same text are
// the same expression (for the suppression rules). A rendering that loses information (HTML,
// a node kind the printer does not spell out) is replaced by a text unique to e, so it never
// equals another's.
func text(e ir.Expr) string {
	p := &printer{elided: map[ir.Expr]bool{}}
	s := p.expr(e)
	if p.lossy {
		return fmt.Sprintf("\x00lossy %p", e)
	}
	return s
}

type printer struct {
	home   string
	elided map[ir.Expr]bool
	lossy  bool // set when a rendering left something out (HTML, an unknown kind)
}

func (p *printer) qual(module, name string) string {
	if module == "" || module == p.home || module == "Prelude" {
		return name
	}
	return module + "." + name
}

// atom renders e, parenthesized unless it is atomic.
func (p *printer) atom(e ir.Expr) string {
	s := p.expr(e)
	if p.elided[e] || atomic(e) {
		return s
	}
	return "(" + s + ")"
}

func atomic(e ir.Expr) bool {
	switch x := e.(type) {
	case *ir.Lit:
		return x.Kind != ir.LitInt && x.Kind != ir.LitFloat || x.Int >= 0 && x.Float >= 0
	case *ir.Var, *ir.Record, *ir.Update, *ir.List, *ir.Hole, *ir.ColumnRef, *ir.TableRef, *ir.Str,
		*ir.Field, *ir.Ord, *ir.EffNotFound, *ir.Let, *ir.EffBind:
		return true
	case *ir.Ctor:
		return x.Payload == nil
	case *ir.PageRef:
		return x.Params == nil
	case *ir.Widen:
		return atomic(x.Expr)
	case *ir.Cond:
		return x.Kind == "isNothing" || x.Kind == "isJust"
	}
	return false
}

// operand renders an operator's operand: parenthesized when it is itself an operator, a
// conditional, a match or a function.
func (p *printer) operand(e ir.Expr) string {
	switch e.(type) {
	case *ir.Op, *ir.If, *ir.Match, *ir.Lambda:
		if !p.elided[e] {
			return "(" + p.expr(e) + ")"
		}
	}
	return p.expr(e)
}

func (p *printer) args(es []ir.Expr) string {
	var sb strings.Builder
	for _, a := range es {
		sb.WriteByte(' ')
		sb.WriteString(p.atom(a))
	}
	return sb.String()
}

func (p *printer) expr(e ir.Expr) string {
	if isNil(e) {
		return "_"
	}
	if p.elided[e] {
		switch e.(type) {
		case *ir.Record, *ir.Update:
			return "{…}"
		}
		return "…"
	}
	switch x := e.(type) {
	case *ir.Lit:
		return lit(x)
	case *ir.Var:
		switch x.Kind {
		case ir.VarLocal:
			return x.Name
		default:
			return p.qual(x.Module, x.Name)
		}
	case *ir.Call:
		return p.atom(x.Fn) + p.args(x.Args)
	case *ir.Partial:
		return p.atom(x.Fn) + p.args(x.Given)
	case *ir.Lambda:
		var ps []string
		for _, a := range x.Params {
			ps = append(ps, a.Name)
		}
		return `\` + strings.Join(ps, " ") + " -> " + p.expr(x.Body)
	case *ir.Record:
		var fs []string
		for _, f := range x.Fields {
			if isDefault(x.T, f) {
				continue
			}
			fs = append(fs, f.Name+": "+p.expr(f.Value))
		}
		if len(fs) == 0 {
			return "{}"
		}
		return "{ " + strings.Join(fs, ", ") + " }"
	case *ir.Update:
		var fs []string
		for _, f := range x.Fields {
			fs = append(fs, f.Name+": "+p.expr(f.Value))
		}
		return "{ " + p.expr(x.Base) + " | " + strings.Join(fs, ", ") + " }"
	case *ir.Field:
		return p.atom(x.Expr) + "." + x.Name
	case *ir.Ctor:
		if x.Payload == nil {
			return x.Name
		}
		if r, ok := x.Payload.(*ir.Record); ok && !p.elided[r] && r.T != nil && r.T.Name == "" && len(r.Fields) == 1 && r.Fields[0].Name == "value" {
			return x.Name + " " + p.atom(r.Fields[0].Value)
		}
		return x.Name + " " + p.atom(x.Payload)
	case *ir.List:
		var es []string
		for _, el := range x.Elems {
			es = append(es, p.expr(el))
		}
		return "[" + strings.Join(es, ", ") + "]"
	case *ir.PageRef:
		s := p.qual("", x.Page.Module)
		if x.Params != nil {
			s += " " + p.atom(x.Params)
		}
		return s
	case *ir.Hole:
		return "?" + x.Name
	case *ir.Match:
		return p.match(x, nil)
	case *ir.If:
		return "if " + p.expr(x.Cond) + " then " + p.expr(x.Then) + " else " + p.expr(x.Else)
	case *ir.Let, *ir.EffBind:
		return p.block(e)
	case *ir.Op:
		return p.operand(x.L) + " " + x.Kind + " " + p.operand(x.R)
	case *ir.Widen:
		return p.expr(x.Expr)
	case *ir.Str:
		var sb strings.Builder
		sb.WriteByte('"')
		for _, part := range x.Parts {
			if l, ok := part.(*ir.Lit); ok && l.Kind == ir.LitText && !p.elided[l] {
				q := strconv.Quote(l.Text)
				sb.WriteString(q[1 : len(q)-1])
				continue
			}
			sb.WriteString("{" + p.expr(part) + "}")
		}
		sb.WriteByte('"')
		return sb.String()
	case *ir.EffPure:
		return effName(x.Kind) + ".succeed " + p.atom(x.Value)
	case *ir.EffMap:
		return effName(x.Kind) + ".map " + p.atom(x.Fn) + " " + p.atom(x.Effect)
	case *ir.EffAll:
		return effName(x.Kind) + ".all " + p.atom(x.Effects)
	case *ir.EffFail:
		return "Task.fail " + p.atom(x.Message)
	case *ir.EffNotFound:
		if app, ok := x.Ty.(*types.App); ok && app.Con == "Query" {
			return "Query.notFound"
		}
		return "Task.notFound"
	case *ir.DbGet:
		return "Db.get " + x.Table.Name + " " + p.atom(x.ID)
	case *ir.DbSelect:
		return p.selectPipe(x)
	case *ir.DbRun:
		return p.atom(x.Select) + " |> " + terminal(x.Terminal)
	case *ir.DbInsert:
		return "Db.insert " + x.Table.Name + " " + p.atom(x.Row)
	case *ir.DbUpdate:
		return "Db.update " + x.Table.Name + " " + p.atom(x.ID) + " " + p.atom(x.Fn)
	case *ir.DbDelete:
		return "Db.delete " + x.Table.Name + " " + p.atom(x.ID)
	case *ir.DbScoped:
		return x.Op + " " + x.Table.Name
	case *ir.DbGetScoped:
		return x.Op + " " + x.Table.Name + " " + p.atom(x.ID)
	case *ir.DbUpdateScoped:
		return x.Op + " " + x.Table.Name + " " + p.atom(x.ID) + " " + p.atom(x.Fn)
	case *ir.DbDeleteScoped:
		return x.Op + " " + x.Table.Name + " " + p.atom(x.ID)
	case *ir.DbInsertScoped:
		return x.Op + " " + x.Table.Name + " " + p.atom(x.Row)
	case *ir.TableRef:
		return x.Table.Name
	case *ir.ColumnRef:
		return x.Table.Name + "." + x.Field
	case *ir.Cond:
		switch x.Kind {
		case "and", "or":
			if x.Subs == nil {
				return "Db." + x.Kind + " " + p.atom(x.Value)
			}
			var ss []string
			for _, s := range x.Subs {
				ss = append(ss, p.expr(s))
			}
			return "Db." + x.Kind + " [" + strings.Join(ss, ", ") + "]"
		case "not":
			return "Db.not " + p.atom(x.Subs[0])
		case "isNothing", "isJust":
			return p.expr(x.Col) + "." + x.Kind
		}
		return p.expr(x.Col) + "." + x.Kind + " " + p.atom(x.Value)
	case *ir.Ord:
		dir := "asc"
		if x.Desc {
			dir = "desc"
		}
		return p.expr(x.Col) + "." + dir
	case *ir.CapCall:
		return x.Cap.Name + "." + x.Entry + p.args(x.Args)
	case *ir.Elem:
		p.lossy = true
		return "<" + x.Tag + ">"
	case *ir.HText, *ir.HExpr, *ir.HFor, *ir.HIf, *ir.HMatch, *ir.Comp, *ir.Input, *ir.FormEl:
		p.lossy = true
		return "<…>"
	}
	p.lossy = true
	return strings.TrimPrefix(fmt.Sprintf("%T", e), "*ir.")
}

func effName(k ir.EffKind) string {
	if k == ir.EffQuery {
		return "Query"
	}
	return "Task"
}

func terminal(t ir.SelTerminal) string {
	switch t {
	case ir.SelAll:
		return "Db.all"
	case ir.SelFirst:
		return "Db.first"
	case ir.SelCount:
		return "Db.count"
	}
	return ""
}

func lit(x *ir.Lit) string {
	switch x.Kind {
	case ir.LitInt:
		return strconv.FormatInt(x.Int, 10)
	case ir.LitFloat:
		s := strconv.FormatFloat(x.Float, 'g', -1, 64)
		if !strings.ContainsAny(s, ".eEnI") {
			s += ".0"
		}
		return s
	case ir.LitBool:
		if x.Bool {
			return "True"
		}
		return "False"
	case ir.LitText:
		return strconv.Quote(x.Text)
	}
	return "()"
}

// selectPipe renders a select pipeline step by step.
func (p *printer) selectPipe(x *ir.DbSelect) string {
	var steps []string
	if x.Base == nil {
		steps = append(steps, "Db.from "+x.Table.Name)
	} else {
		steps = append(steps, p.atom(x.Base))
	}
	for _, w := range x.Where {
		steps = append(steps, "Db.where "+p.atom(w))
	}
	for _, o := range x.Order {
		steps = append(steps, "Db.orderBy "+p.atom(o))
	}
	if x.Limit != nil {
		steps = append(steps, "Db.limit "+p.atom(x.Limit))
	}
	if x.Offset != nil {
		steps = append(steps, "Db.offset "+p.atom(x.Offset))
	}
	if x.Terminal != ir.SelNone {
		steps = append(steps, terminal(x.Terminal))
	}
	return strings.Join(steps, " |> ")
}

// block renders a chain of Lets and effect binds as `{ let x = v; let y <- e; r }`; a Let of a
// match subject's temporary is shown as the match on the value.
func (p *printer) block(e ir.Expr) string {
	var stmts []string
	for {
		if p.elided[e] {
			break
		}
		switch x := e.(type) {
		case *ir.Let:
			if m, ok := x.Body.(*ir.Match); ok && isTemp(x.Name) && !p.elided[m] {
				if v, ok := m.Subject.(*ir.Var); ok && v.Kind == ir.VarLocal && v.Name == x.Name {
					e = nil
					stmts = append(stmts, p.match(m, x.Value))
					continue
				}
			}
			stmts = append(stmts, "let "+x.Name+" = "+p.expr(x.Value))
			e = x.Body
			continue
		case *ir.EffBind:
			name := "_"
			if x.Param != nil {
				name = x.Param.Name
			}
			stmts = append(stmts, "let "+name+" <- "+p.expr(x.Effect))
			e = x.Body
			continue
		}
		break
	}
	if e != nil {
		stmts = append(stmts, p.expr(e))
	}
	if len(stmts) == 1 {
		return stmts[0]
	}
	return "{ " + strings.Join(stmts, "; ") + " }"
}

// match renders a Match; subject, when set, is shown in place of the temporary it was bound to.
func (p *printer) match(m *ir.Match, subject ir.Expr) string {
	subj := p.expr(m.Subject)
	if subject != nil {
		subj = p.expr(subject)
	}
	var arms []string
	for _, c := range m.Cases {
		pat, body := p.pattern(m, c)
		arms = append(arms, pat+" -> "+p.expr(body))
	}
	if !isNil(m.Default) {
		arms = append(arms, "_ -> "+p.expr(m.Default))
	}
	return "match " + subj + " { " + strings.Join(arms, ", ") + " }"
}

// pattern is a case's pattern as the user wrote it (as far as the Binds and the Lets around the
// body tell), and the body under those Lets.
func (p *printer) pattern(m *ir.Match, c ir.Case) (string, ir.Expr) {
	tag := c.Tag
	if m.Kind == ir.MatchLit {
		if con, ok := m.Subject.Type().(*types.Con); ok && con.Name == "Text" {
			tag = strconv.Quote(tag)
		}
	}
	if m.Kind == ir.MatchList {
		if tag == "nil" {
			return "[]", c.Body
		}
		tag = "cons"
	}
	if len(c.Binds) == 0 {
		if u, ok := m.Subject.Type().(*types.Union); ok && m.Kind == ir.MatchUnion {
			if arg, _ := types.CtorArg(u, c.Tag); arg != nil {
				return tag + " _", c.Body
			}
		}
		return tag, c.Body
	}
	names := map[string]string{}
	body := c.Body
	for {
		l, ok := body.(*ir.Let)
		if !ok || p.elided[l] {
			break
		}
		v, ok := l.Value.(*ir.Var)
		if !ok || v.Kind != ir.VarLocal || !bindsName(c, v.Name) {
			break
		}
		names[v.Name] = l.Name
		body = l.Body
	}
	var parts []string
	for _, b := range c.Binds {
		n := names[b.Name]
		if n == "" {
			n = "_" // unnamed, or a temporary a nested pattern reads
			if uses(body, b.Name) > 0 && !isTemp(b.Name) {
				n = b.Name
			}
		}
		if b.Field != "" && b.Field != "value" && m.Kind == ir.MatchUnion {
			n = b.Field + ": " + n
		}
		parts = append(parts, n)
	}
	switch {
	case m.Kind == ir.MatchList:
		return "[" + parts[0] + ", .." + parts[len(parts)-1] + "]", body
	case len(c.Binds) == 1 && (c.Binds[0].Field == "" || c.Binds[0].Field == "value"):
		return tag + " " + parts[0], body
	}
	return tag + " { " + strings.Join(parts, ", ") + " }", body
}

func bindsName(c ir.Case, name string) bool {
	for _, b := range c.Binds {
		if b.Name == name {
			return true
		}
	}
	return false
}

// isTemp reports whether name is one of Lower's temporaries (`_m1`).
func isTemp(name string) bool {
	if !strings.HasPrefix(name, "_m") || len(name) == 2 {
		return false
	}
	_, err := strconv.Atoi(name[2:])
	return err == nil
}

// isDefault reports whether f's value is the default its record type filled in.
func isDefault(rt *ir.RecordType, f ir.FieldInit) bool {
	if rt == nil {
		return false
	}
	for _, d := range rt.Fields {
		if d.Name == f.Name {
			return d.Default != nil && d.Default == f.Value
		}
	}
	return false
}
