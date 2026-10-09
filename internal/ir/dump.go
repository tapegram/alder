package ir

import (
	"fmt"
	"strconv"
	"strings"

	"alder/internal/types"
)

// Dump renders p as S-expressions, one top-level form per declaration, for golden tests. A form
// that does not fit in 80 columns is broken after its head, one item per indented line. Types are
// printed with types.StringIn relative to the module being printed.
//
// Forms: (module M); (record M.T fields…) with fields (name Type) or (name Type = default);
// (union M.T params… (Ctor payload?)…); (value|component M.name : Type (params) body);
// (skipped-effect M.name); then (table …), (form …), (page …), (app-msg …), (session …),
// (capability …). Expressions: literals bare (1, 3.14, "s", True, ()), (var x), (global M.f),
// (std List.map), (call f args…), (partial f given… (missing n)), (lambda (x : T) body),
// (record M.T? (field value)…), (update base (field value)…), (field name expr),
// (ctor M.Union.Ctor payload?), (list elems…), (page-ref M params?), (hole name),
// (match kind subject (case Tag (bind name field : T)… body)… (default body)?),
// (if c t e), (let x : T value body), (op l r) or (op : OperandT l r) for comparisons, `++` and
// `??`, (str parts…); HTML: (elem tag (attr name kind value)… children…), (comp M.C props children…),
// (htext "…"), (hexpr kind expr), (hfor x : T list body), (hif c t e), (hmatch …),
// (input Kind Form field=f label="…" [label-expr] [(value e)] [form]), (form-el M.Msg attrs… children…).
func Dump(p *Program) string {
	var sb strings.Builder
	form := func(x *sx) {
		x.render(&sb, 0)
		sb.WriteByte('\n')
	}
	for _, m := range p.Modules {
		d := &dumper{home: m.Name}
		form(atoms("module", m.Name))
		for _, t := range m.Types {
			form(d.typeDecl(t))
		}
		for _, v := range m.Values {
			form(d.value("value", v))
		}
		for _, n := range m.Skipped {
			form(atoms("skipped-effect", m.Name+"."+n))
		}
		for _, v := range m.Components {
			form(d.value("component", v))
		}
	}
	for _, t := range p.Tables {
		form(list(2, atom("table"), atom(t.Module+"."+t.Name), atoms("id", t.IDField), atoms(append([]string{"insert"}, t.InsertFields...)...)))
	}
	for _, f := range p.Forms {
		form(atoms("form", f.Module+"."+f.Name))
	}
	for _, pg := range p.Pages {
		d := &dumper{home: pg.Module}
		x := list(3, atom("page"), atom(pg.Module), atom(strconv.Quote(pg.Path)), atoms("params", d.rt(pg.Params)))
		if pg.Requires != "" {
			x.items = append(x.items, atoms("requires", pg.Requires))
		}
		if pg.Session != nil && pg.Session != p.Session {
			x.items = append(x.items, atoms("session", d.rt(pg.Session)))
		}
		if pg.Model != nil {
			x.items = append(x.items, atoms("model", d.rt(pg.Model)))
		}
		for _, m := range pg.Msgs {
			x.items = append(x.items, d.msg("msg", m))
		}
		if pg.View != nil {
			x.items = append(x.items, atoms("view", pg.View.Module+"."+pg.View.Name))
		}
		form(x)
	}
	for _, m := range p.AppMsgs {
		form((&dumper{home: "App"}).msg("app-msg", m))
	}
	if p.Session != nil {
		form(atoms("session", p.Session.Module+"."+p.Session.Name))
	}
	for _, c := range p.Caps {
		d := &dumper{home: c.Module}
		x := list(2, atom("capability"), atom(c.Module+"."+c.Name))
		for _, e := range c.Entries {
			x.items = append(x.items, list(1, atom(e.Name), atom(d.ty(e.Type))))
		}
		form(x)
	}
	return sb.String()
}

// ---- S-expression layout

const width = 80

// sx is an atom or a list; when a list is broken over lines, its first head items stay on the
// first line, or only the first minHead items when the head line itself would not fit.
type sx struct {
	atom    string
	items   []*sx
	head    int
	minHead int
	list    bool
}

func atom(s string) *sx { return &sx{atom: s} }

func atoms(ss ...string) *sx {
	x := &sx{list: true, head: len(ss), minHead: len(ss)}
	for _, s := range ss {
		x.items = append(x.items, atom(s))
	}
	return x
}

func list(head int, items ...*sx) *sx {
	return &sx{list: true, head: head, minHead: head, items: items}
}

func (x *sx) flat() string {
	if !x.list {
		return x.atom
	}
	parts := make([]string, len(x.items))
	for i, it := range x.items {
		parts[i] = it.flat()
	}
	return "(" + strings.Join(parts, " ") + ")"
}

func (x *sx) render(sb *strings.Builder, indent int) {
	flat := x.flat()
	if !x.list || indent+len(flat) <= width {
		sb.WriteString(flat)
		return
	}
	head := max(x.head, 1)
	if x.minHead < head {
		n := indent + 1
		for _, it := range x.items[:min(head, len(x.items))] {
			n += len(it.flat()) + 1
		}
		if n > width {
			head = max(x.minHead, 1)
		}
	}
	sb.WriteByte('(')
	for i, it := range x.items {
		switch {
		case i == 0:
			sb.WriteString(it.flat())
		case i < head:
			sb.WriteString(" " + it.flat())
		default:
			sb.WriteString("\n" + strings.Repeat(" ", indent+2))
			it.render(sb, indent+2)
		}
	}
	sb.WriteByte(')')
}

// ---- declarations

type dumper struct{ home string } // the module whose names print unqualified

func (d *dumper) ty(t types.Type) string { return types.StringIn(t, d.home) }

// rt names a record type: its name, or its fields when anonymous.
func (d *dumper) rt(r *RecordType) string {
	if r == nil {
		return "nil"
	}
	if r.Name != "" {
		return d.ty(&types.Record{Module: r.Module, Name: r.Name})
	}
	var parts []string
	for _, f := range r.Fields {
		parts = append(parts, f.Name+": "+d.ty(f.Type))
	}
	return "{ " + strings.Join(parts, ", ") + " }"
}

func (d *dumper) typeDecl(t *TypeDecl) *sx {
	name := t.Module + "." + t.Name
	if t.Record != nil {
		x := list(2+len(t.Record.Params), atom("record"), atom(name))
		for _, p := range t.Record.Params {
			x.items = append(x.items, atom(p))
		}
		for _, f := range t.Record.Fields {
			fx := list(2, atom(f.Name), atom(d.ty(f.Type)))
			if f.Default != nil {
				fx.items = append(fx.items, atom("="), d.expr(f.Default))
				fx.head = 3
			}
			x.items = append(x.items, fx)
		}
		return x
	}
	x := list(2+len(t.Union.Params), atom("union"), atom(name))
	for _, p := range t.Union.Params {
		x.items = append(x.items, atom(p))
	}
	for _, c := range t.Union.Ctors {
		if c.Payload == nil {
			x.items = append(x.items, atoms(c.Name))
		} else {
			x.items = append(x.items, atoms(c.Name, d.rt(c.Payload)))
		}
	}
	return x
}

func (d *dumper) value(kind string, v *Value) *sx {
	var ps []string
	for _, p := range v.Params {
		ps = append(ps, p.Name)
	}
	return list(5, atom(kind), atom(v.Module+"."+v.Name), atom(":"), atom(d.ty(v.Type)), atom("("+strings.Join(ps, " ")+")"), d.expr(v.Body))
}

func (d *dumper) msg(kind string, m *Msg) *sx {
	x := list(2, atom(kind), atom(m.Name))
	if m.Params != nil {
		x.items = append(x.items, atoms("params", d.rt(m.Params)))
	}
	if m.Form != nil {
		x.items = append(x.items, atoms("form", d.ty(&types.Record{Module: m.Form.Module, Name: m.Form.Name})))
	}
	return x
}

// ---- expressions

func (d *dumper) exprs(es []Expr) []*sx {
	out := make([]*sx, len(es))
	for i, e := range es {
		out[i] = d.expr(e)
	}
	return out
}

func (d *dumper) fields(fs []FieldInit) []*sx {
	out := make([]*sx, len(fs))
	for i, f := range fs {
		out[i] = list(1, atom(f.Name), d.expr(f.Value))
	}
	return out
}

// typedOps print their operand type: the emitter's choice depends on it.
var typedOps = map[string]bool{"==": true, "!=": true, "<": true, "<=": true, ">": true, ">=": true, "++": true, "??": true}

func (d *dumper) expr(e Expr) *sx {
	switch x := e.(type) {
	case nil:
		return atom("nil")
	case *Lit:
		switch x.Kind {
		case LitInt:
			return atom(strconv.FormatInt(x.Int, 10))
		case LitFloat:
			s := strconv.FormatFloat(x.Float, 'g', -1, 64)
			if !strings.ContainsAny(s, ".eEnN") {
				s += ".0"
			}
			return atom(s)
		case LitBool:
			if x.Bool {
				return atom("True")
			}
			return atom("False")
		case LitText:
			return atom(strconv.Quote(x.Text))
		}
		return atom("()")
	case *Var:
		switch x.Kind {
		case VarGlobal:
			return atoms("global", x.Module+"."+x.Name)
		case VarStd:
			return atoms("std", x.Module+"."+x.Name)
		}
		return atoms("var", x.Name)
	case *Call:
		return list(2, append([]*sx{atom("call"), d.expr(x.Fn)}, d.exprs(x.Args)...)...)
	case *Partial:
		items := append([]*sx{atom("partial"), d.expr(x.Fn)}, d.exprs(x.Given)...)
		return list(2, append(items, atoms("missing", strconv.Itoa(x.Missing)))...)
	case *Lambda:
		var ps []string
		for _, p := range x.Params {
			ps = append(ps, p.Name+" : "+d.ty(p.Type))
		}
		return list(2, atom("lambda"), atom("("+strings.Join(ps, ", ")+")"), d.expr(x.Body))
	case *Record:
		items := []*sx{atom("record")}
		if x.T != nil && x.T.Name != "" {
			items = append(items, atom(d.rt(x.T)))
		}
		return list(len(items), append(items, d.fields(x.Fields)...)...)
	case *Update:
		return list(2, append([]*sx{atom("update"), d.expr(x.Base)}, d.fields(x.Fields)...)...)
	case *Field:
		return list(2, atom("field"), atom(x.Name), d.expr(x.Expr))
	case *Ctor:
		name := x.U.Module + "." + x.U.Name + "." + x.Name
		if x.Payload == nil {
			return atoms("ctor", name)
		}
		return list(2, atom("ctor"), atom(name), d.expr(x.Payload))
	case *List:
		return list(1, append([]*sx{atom("list")}, d.exprs(x.Elems)...)...)
	case *PageRef:
		if x.Params == nil {
			return atoms("page-ref", x.Page.Module)
		}
		return list(2, atom("page-ref"), atom(x.Page.Module), d.expr(x.Params))
	case *Hole:
		return atoms("hole", x.Name)
	case *Match:
		return d.match("match", x)
	case *HMatch:
		return d.match("hmatch", (*Match)(x))
	case *If:
		return list(2, atom("if"), d.expr(x.Cond), d.expr(x.Then), d.expr(x.Else))
	case *Let:
		return list(2, atom("let"), atom(x.Name+" : "+d.ty(x.T)), d.expr(x.Value), d.expr(x.Body))
	case *Op:
		if typedOps[x.Kind] {
			return list(3, atom(x.Kind), atom(": "+d.ty(x.OperandT)), d.expr(x.L), d.expr(x.R))
		}
		return list(1, atom(x.Kind), d.expr(x.L), d.expr(x.R))
	case *Str:
		return list(1, append([]*sx{atom("str")}, d.exprs(x.Parts)...)...)
	case *Widen:
		return list(2, atom("widen"), atom(d.ty(x.Ty)), d.expr(x.Expr))
	case *Elem:
		items := []*sx{atom("elem"), atom(x.Tag)}
		items = append(items, d.attrs(x.Attrs)...)
		el := list(len(items), append(items, d.exprs(x.Children)...)...)
		el.minHead = 2
		return el
	case *Comp:
		items := []*sx{atom("comp"), atom(x.Fn.Module + "." + x.Fn.Name), d.expr(x.Props)}
		return list(2, append(items, d.exprs(x.Children)...)...)
	case *HText:
		return atoms("htext", strconv.Quote(x.Text))
	case *HExpr:
		return list(2, atom("hexpr"), atom([]string{"text", "html", "list"}[x.Kind]), d.expr(x.Expr))
	case *HFor:
		return list(2, atom("hfor"), atom(x.Var+" : "+d.ty(x.T)), d.expr(x.List), d.expr(x.Body))
	case *HIf:
		return list(2, atom("hif"), d.expr(x.Cond), d.expr(x.Then), d.expr(x.Else))
	case *Input:
		form := "nil"
		if x.Form != nil {
			form = d.ty(&types.Record{Module: x.Form.Module, Name: x.Form.Name})
		}
		items := []*sx{atom("input"), atom([]string{"Input", "Textarea", "Select", "Errors"}[x.Kind]), atom(form)}
		if x.Field != "" {
			items = append(items, atom("field="+x.Field))
		}
		if x.Label != "" {
			items = append(items, atom("label="+strconv.Quote(x.Label)))
		}
		head := len(items)
		if x.BareForm {
			items = append(items, atom("form"))
			head++
		}
		if x.LabelExpr != nil {
			items = append(items, list(1, atom("label"), d.expr(x.LabelExpr)))
		}
		if x.Value != nil {
			items = append(items, list(1, atom("value"), d.expr(x.Value)))
		}
		return list(head, items...)
	case *FormEl:
		msg := "nil"
		if x.Msg != nil {
			owner := "App"
			if x.Page != nil {
				owner = x.Page.Module
			}
			msg = owner + "." + x.Msg.Name
		}
		items := []*sx{atom("form-el"), atom(msg)}
		items = append(items, d.attrs(x.Attrs)...)
		el := list(len(items), append(items, d.exprs(x.Children)...)...)
		el.minHead = 2
		return el
	}
	return atom(fmt.Sprintf("<%T>", e))
}

func (d *dumper) attrs(as []HAttr) []*sx {
	kinds := []string{"text", "bool", "maybe", "href", "click", "submit"}
	out := make([]*sx, len(as))
	for i, a := range as {
		out[i] = list(3, atom("attr"), atom(a.Name), atom(kinds[a.Kind]), d.expr(a.Value))
	}
	return out
}

func (d *dumper) match(head string, m *Match) *sx {
	kind := []string{"union", "bool", "list", "lit"}[m.Kind]
	x := list(3, atom(head), atom(kind), d.expr(m.Subject))
	for _, c := range m.Cases {
		tag := c.Tag
		if t, ok := m.Subject.Type().(*types.Con); ok && m.Kind == MatchLit && t.Name == "Text" {
			tag = strconv.Quote(tag)
		}
		cx := list(2+len(c.Binds), atom("case"), atom(tag))
		for _, b := range c.Binds {
			field := b.Field
			if field != "" {
				field += " "
			}
			cx.items = append(cx.items, atom("(bind "+b.Name+" "+field+": "+d.ty(b.Type)+")"))
		}
		cx.items = append(cx.items, d.expr(c.Body))
		x.items = append(x.items, cx)
	}
	if m.Default != nil {
		x.items = append(x.items, list(1, atom("default"), d.expr(m.Default)))
	}
	return x
}
