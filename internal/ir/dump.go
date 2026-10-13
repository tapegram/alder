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
// then (table …), (form …), (page …) with (view|load|update M.name), (app-msg …),
// (app-update App.update), (session …), (capability …), (fake M.Cap (tables (record …)
// (table …)…) (entries (entry M.name : Type (params) body)…)). Expressions: literals bare (1, 3.14, "s", True, ()), (var x), (global M.f),
// (std List.map), (call f args…), (partial f given… (missing n)), (lambda (x : T) body),
// (record M.T? (field value)…), (update base (field value)…), (field name expr),
// (ctor M.Union.Ctor payload?), (list elems…), (page-ref M params?), (hole name),
// (match kind subject (case Tag (bind name field : T)… body)… (default body)?),
// (if c t e), (let x : T value body), (op l r) or (op : OperandT l r) for comparisons, `++` and
// `??`, (str parts…); HTML: (elem tag (attr name kind value)… children…), (comp M.C props children…),
// (htext "…"), (hexpr kind expr), (hfor x : T list body), (hif c t e), (hmatch …),
// (input Kind Form field=f label="…" [label-expr] [(value e)] [form]), (form-el M.Msg attrs… children…).
// Effects (kind is query or task): (pure kind v), (bind kind x : T effect body) or
// (bind kind _ effect body), (map kind fn effect), (all kind effects), (fail msg), (not-found),
// (cap M.Cap entry args…). Tests, after a module's values and components: (test "name" pure|task
// body); in their bodies (test-run task), (test-trace), (test-fake M.Cap), (test-eq l r). Database: (db-get M.T id), (db-select M.T [(base e)] (where c)…
// (order o)… [(limit n)] [(offset n)] select|all|first|count), (db-run all|first|count sel),
// (db-insert M.T row), (db-update M.T id fn), (db-delete M.T id), (col M.T field),
// (cond kind [col] [value] subs…) with (cond and|or (value e)) for a list that is no literal,
// (ord col asc|desc).
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
		for _, v := range m.Components {
			form(d.value("component", v))
		}
		for _, t := range m.Tests {
			kind := "pure"
			if t.Effect {
				kind = "task"
			}
			form(list(3, atom("test"), atom(strconv.Quote(t.Name)), atom(kind), d.expr(t.Body)))
		}
	}
	for _, t := range p.Tables {
		form(table(t))
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
		if len(pg.Roles) > 0 {
			x.items = append(x.items, atoms(append([]string{"roles"}, pg.Roles...)...))
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
		for _, v := range []struct {
			kind string
			v    *Value
		}{{"view", pg.View}, {"load", pg.Load}, {"update", pg.Update}} {
			if v.v != nil {
				x.items = append(x.items, atoms(v.kind, v.v.Module+"."+v.v.Name))
			}
		}
		form(x)
	}
	for _, m := range p.AppMsgs {
		form((&dumper{home: "App"}).msg("app-msg", m))
	}
	if p.AppUpdate != nil {
		form(atoms("app-update", p.AppUpdate.Module+"."+p.AppUpdate.Name))
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
	for _, f := range p.Fakes {
		d := &dumper{home: f.Module}
		tables := list(1, atom("tables"))
		for _, t := range f.Tables {
			tables.items = append(tables.items, d.typeDecl(&TypeDecl{Module: t.Module, Name: t.Name, Record: t.Record}), table(t))
		}
		entries := list(1, atom("entries"))
		for _, v := range f.Entries {
			entries.items = append(entries.items, d.value("entry", v))
		}
		form(list(2, atom("fake"), atom(f.Capability.Module+"."+f.Capability.Name), tables, entries))
	}
	return sb.String()
}

// table is a table's form: (table M.T (id f) (created f)? (updated f)? (unique f…)? (owner f)?
// (ref f M.T rule?)… (insert f…)).
func table(t *Table) *sx {
	items := []*sx{atom("table"), atom(t.Module + "." + t.Name), atoms("id", t.IDField)}
	if t.Created != "" {
		items = append(items, atoms("created", t.Created))
	}
	if t.Updated != "" {
		items = append(items, atoms("updated", t.Updated))
	}
	if len(t.Unique) > 0 {
		items = append(items, atoms(append([]string{"unique"}, t.Unique...)...))
	}
	if t.Owner != "" {
		items = append(items, atoms("owner", t.Owner))
	}
	for _, f := range t.Record.Fields {
		if r := t.Refs[f.Name]; r != "" {
			ref := []string{"ref", f.Name, r}
			if del := t.OnDelete[f.Name]; del != "" {
				ref = append(ref, del)
			}
			items = append(items, atoms(ref...))
		}
	}
	return list(2, append(items, atoms(append([]string{"insert"}, t.InsertFields...)...))...)
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
		if x.Wrap != "" {
			items = append(items, list(2, append([]*sx{atom("wrap"), atom(x.Wrap)}, d.attrs(x.WrapAttrs)...)...))
		}
		return list(head, items...)
	case *EffPure:
		return list(2, atom("pure"), atom(effKinds[x.Kind]), d.expr(x.Value))
	case *EffBind:
		name := "_"
		if x.Param != nil {
			name = x.Param.Name + " : " + d.ty(x.Param.Type)
		}
		return list(3, atom("bind"), atom(effKinds[x.Kind]), atom(name), d.expr(x.Effect), d.expr(x.Body))
	case *EffMap:
		return list(2, atom("map"), atom(effKinds[x.Kind]), d.expr(x.Fn), d.expr(x.Effect))
	case *EffAll:
		return list(2, atom("all"), atom(effKinds[x.Kind]), d.expr(x.Effects))
	case *EffFail:
		return list(1, atom("fail"), d.expr(x.Message))
	case *EffNotFound:
		return atoms("not-found")
	case *TestRun:
		return list(1, atom("test-run"), d.expr(x.Task))
	case *TestTrace:
		return atoms("test-trace")
	case *TestFake:
		return atoms("test-fake", x.Fake.Capability.Module+"."+x.Fake.Capability.Name)
	case *TestEq:
		return list(1, atom("test-eq"), d.expr(x.Left), d.expr(x.Right))
	case *TestLogin:
		return list(1, atom("test-login"), d.expr(x.Session))
	case *TestGet:
		name := "test-get"
		if x.Open {
			name = "test-open"
		}
		items := []*sx{atom(name), atom(x.Page.Module)}
		if x.Params != nil {
			items = append(items, d.expr(x.Params))
		}
		return list(2, items...)
	case *TestSubmit:
		return list(3, d.submitItems("test-submit", x.Form, x.Msg, x.Params, x.Record)...)
	case *TestSubmitRaw:
		return list(3, d.submitItems("test-submit-raw", x.Form, x.Msg, x.Params, x.Fields)...)
	case *TestClick:
		owner := "App"
		if x.Page != nil {
			owner = x.Page.Module
		}
		items := []*sx{atom("test-click"), atom(owner + "." + x.Msg.Name)}
		if x.Params != nil {
			items = append(items, d.expr(x.Params))
		}
		return list(2, items...)
	case *CapCall:
		items := []*sx{atom("cap"), atom(x.Cap.Module + "." + x.Cap.Name), atom(x.Entry)}
		return list(3, append(items, d.exprs(x.Args)...)...)
	case *DbGet:
		return list(2, atom("db-get"), atom(tableName(x.Table)), d.expr(x.ID))
	case *DbSelect:
		items := []*sx{atom("db-select"), atom(tableName(x.Table))}
		if x.Base != nil {
			items = append(items, list(1, atom("base"), d.expr(x.Base)))
		}
		for _, w := range x.Where {
			items = append(items, list(1, atom("where"), d.expr(w)))
		}
		for _, o := range x.Order {
			items = append(items, list(1, atom("order"), d.expr(o)))
		}
		if x.Limit != nil {
			items = append(items, list(1, atom("limit"), d.expr(x.Limit)))
		}
		if x.Offset != nil {
			items = append(items, list(1, atom("offset"), d.expr(x.Offset)))
		}
		return list(2, append(items, atom(selTerminals[x.Terminal]))...)
	case *DbRun:
		return list(2, atom("db-run"), atom(selTerminals[x.Terminal]), d.expr(x.Select))
	case *DbInsert:
		return list(2, atom("db-insert"), atom(tableName(x.Table)), d.expr(x.Row))
	case *DbUpdate:
		return list(2, atom("db-update"), atom(tableName(x.Table)), d.expr(x.ID), d.expr(x.Fn))
	case *DbDelete:
		return list(2, atom("db-delete"), atom(tableName(x.Table)), d.expr(x.ID))
	case *DbMine:
		return atoms("db-mine", tableName(x.Table))
	case *DbGetMine:
		return list(2, atom("db-get-mine"), atom(tableName(x.Table)), d.expr(x.ID))
	case *DbUpdateMine:
		return list(2, atom("db-update-mine"), atom(tableName(x.Table)), d.expr(x.ID), d.expr(x.Fn))
	case *DbDeleteMine:
		return list(2, atom("db-delete-mine"), atom(tableName(x.Table)), d.expr(x.ID))
	case *DbInsertMine:
		return list(2, atom("db-insert-mine"), atom(tableName(x.Table)), d.expr(x.Row))
	case *ColumnRef:
		return atoms("col", tableName(x.Table), x.Field)
	case *TableRef:
		return atoms("table", tableName(x.Table))
	case *Cond:
		items := []*sx{atom("cond"), atom(x.Kind)}
		if x.Col != nil {
			items = append(items, d.expr(x.Col))
		}
		if x.Value != nil {
			if x.Col == nil { // and/or of a list that is no literal
				items = append(items, list(1, atom("value"), d.expr(x.Value)))
			} else {
				items = append(items, d.expr(x.Value))
			}
		}
		return list(2, append(items, d.exprs(x.Subs)...)...)
	case *Ord:
		dir := "asc"
		if x.Desc {
			dir = "desc"
		}
		return list(1, atom("ord"), d.expr(x.Col), atom(dir))
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

var effKinds = []string{"query", "task"}

var selTerminals = []string{"select", "all", "first", "count"}

func tableName(t *Table) string { return t.Module + "." + t.Name }

// submitItems are the items of a test-submit or test-submit-raw: the form, the message (with
// its parameters when the test wrote them) and the value posted.
func (d *dumper) submitItems(name string, form *Form, msg *Msg, params, value Expr) []*sx {
	items := []*sx{atom(name), atom(form.Module + "." + form.Name), atom(msg.Name)}
	if params != nil {
		items = append(items, list(1, atom("params"), d.expr(params)))
	}
	return append(items, d.expr(value))
}
