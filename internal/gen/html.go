package gen

import (
	"strconv"
	"strings"

	"alder/internal/ir"
	"alder/internal/types"
)

// HTML emission. Every HTML node is a Go expression of type html.Node (alder/rt/html):
//
//	Elem     html.El("tag", []html.Attr{…}, children...)  (nil for no attributes)
//	HText    html.Text("…")              raw: rt/html escapes once, when rendering
//	HExpr    html.Text(e) for Text, e for Html, html.Nodes(e...) for List (Html a)
//	HFor     html.Nodes(std.ListMap(func(x T) html.Node { … }, list)...)
//	HIf      an immediately-invoked closure with a Go if (ret mode)
//	HMatch   an immediately-invoked closure with the match's switch (ret mode)
//	Comp     M__Card(struct{…}{Title: …, Children: []html.Node{…}})
//	Input    html.Input(html.Control{…}), derived from the bound form field's type, with Form
//	         "<Module>.<Form>" (matched by the request's html.FormState) and Secret for a
//	         Text.Secret field
//	FormEl   html.FormTo(ref, []html.Attr{…}, children...)
//
// Attributes:
//
//	Text     html.A("name", e)
//	Bool     html.B("name", e)
//	Maybe    html.M("name", e)
//	Href     html.A("href", rt.PageURL(e))
//	Click    html.MsgAttr("data-on:click", ref)
//	Submit   method="post", html.MsgAttr("action", ref) and html.MsgAttr("data-on:submit", ref)
//	         (html.FormTo adds them itself)
//
// where ref is rt.MsgRef{Module: "<Module>", Msg: "<Msg>", Params: …}: the message's parameters
// in declaration order, as text, without the submitted form (kvList). rt/html resolves it when
// rendering, appending the rendered page's parameters as "_p." keys (FormState.PageParams), so
// the message's route knows the page; Datastar posts a submit url-encoded (html.SubmitPost).

// html is HTML node e as a Go expression.
func (f *fn) html(e ir.Expr, s *scope) string {
	switch x := e.(type) {
	case *ir.Elem:
		return "html.El(" + strconv.Quote(x.Tag) + ", " + f.attrs(x.Attrs, s) + f.nodeArgs(x.Children, s)
	case *ir.HText:
		return "html.Text(" + strconv.Quote(x.Text) + ")"
	case *ir.HExpr:
		switch x.Kind {
		case ir.HExprText:
			return "html.Text(" + f.code(x.Expr, s) + ")"
		case ir.HExprList:
			return "html.Nodes(" + f.code(x.Expr, s) + "...)"
		}
		return f.code(x.Expr, s)
	case *ir.HFor:
		c := s.child()
		v := c.declare(x.Var)
		var w strings.Builder
		f.ret(&w, x.Body, c)
		return "html.Nodes(std.ListMap(func(" + v + " " + GoType(x.T) + ") html.Node {\n" + w.String() + "}, " + f.code(x.List, s) + ")...)"
	case *ir.HIf, *ir.HMatch:
		return f.closure(e, s)
	case *ir.Comp:
		return f.comp(x, s)
	case *ir.Input:
		return f.input(x, s)
	case *ir.FormEl:
		ref := "rt.MsgRef{}"
		var rest []ir.HAttr
		for _, a := range x.Attrs {
			if a.Kind == ir.AttrSubmit {
				ref = f.msgRef(a.Value, s)
			} else {
				rest = append(rest, a)
			}
		}
		return "html.FormTo(" + ref + ", " + f.attrs(rest, s) + f.nodeArgs(x.Children, s)
	}
	panic("gen: not an HTML node")
}

// nodeArgs ends a call whose last arguments are child nodes: on the same line for at most one
// child, else one child per line.
func (f *fn) nodeArgs(cs []ir.Expr, s *scope) string {
	switch len(cs) {
	case 0:
		return ")"
	case 1:
		return ", " + f.code(cs[0], s) + ")"
	}
	var b strings.Builder
	b.WriteString(",\n")
	for _, c := range cs {
		b.WriteString(f.code(c, s) + ",\n")
	}
	b.WriteString(")")
	return b.String()
}

// nodeList is a []html.Node literal of cs, laid out like nodeArgs.
func (f *fn) nodeList(cs []ir.Expr, s *scope) string {
	if len(cs) == 1 {
		return "[]html.Node{" + f.code(cs[0], s) + "}"
	}
	var b strings.Builder
	b.WriteString("[]html.Node{\n")
	for _, c := range cs {
		b.WriteString(f.code(c, s) + ",\n")
	}
	b.WriteString("}")
	return b.String()
}

// attrs is an element's attribute list: nil for none.
func (f *fn) attrs(as []ir.HAttr, s *scope) string {
	if len(as) == 0 {
		return "nil"
	}
	var out []string
	for _, a := range as {
		name := strconv.Quote(a.Name)
		switch a.Kind {
		case ir.AttrText:
			out = append(out, "html.A("+name+", "+f.code(a.Value, s)+")")
		case ir.AttrBool:
			out = append(out, "html.B("+name+", "+f.code(a.Value, s)+")")
		case ir.AttrMaybe:
			out = append(out, "html.M("+name+", "+f.code(a.Value, s)+")")
		case ir.AttrHref:
			out = append(out, "html.A("+name+", rt.PageURL("+f.code(a.Value, s)+"))")
		case ir.AttrClick:
			out = append(out, `html.MsgAttr("data-on:click", `+f.msgRef(a.Value, s)+`)`)
		case ir.AttrSubmit: // on an element Lower did not make a FormEl
			ref := f.msgRef(a.Value, s)
			out = append(out, `html.A("method", "post")`, `html.MsgAttr("action", `+ref+`)`,
				`html.MsgAttr("data-on:submit", `+ref+`)`)
		}
	}
	return "[]html.Attr{" + strings.Join(out, ", ") + "}"
}

// msgRef is message value e as an rt.MsgRef, which rt/html resolves to the URL it posts to
// (html.MsgAttr). A constructor written in place gives its module, name and parameters
// statically; any other message value is switched on at run time. The parameters are the
// message's (without the submitted form), in declaration order, as text (kvList: a List one
// key per element).
func (f *fn) msgRef(e ir.Expr, s *scope) string {
	ref := func(module, msg string, rec *types.Record, field func(name string, t types.Type) ir.Expr, c *scope) string {
		var items []kvItem
		if rec != nil {
			for _, fd := range rec.Fields {
				if fd.Name != "form" {
					items = append(items, kvItem{name: fd.Name, t: fd.Type, v: field(fd.Name, fd.Type)})
				}
			}
		}
		out := "rt.MsgRef{Module: " + strconv.Quote(module) + ", Msg: " + strconv.Quote(msg)
		if len(items) > 0 {
			out += ", Params: " + f.kvList(items, c)
		}
		return out + "}"
	}
	if c, ok := e.(*ir.Ctor); ok {
		var rec *types.Record
		field := func(string, types.Type) ir.Expr { return nil }
		if c.Payload != nil {
			rec, _ = c.Payload.Type().(*types.Record)
			field = func(name string, t types.Type) ir.Expr {
				if r, ok := c.Payload.(*ir.Record); ok {
					for _, fi := range r.Fields {
						if fi.Name == name {
							return fi.Value
						}
					}
				}
				return &ir.Field{Typed: ir.Typed{Ty: t}, Expr: c.Payload, Name: name}
			}
		}
		return ref(c.U.Module, c.Name, rec, field, s)
	}
	u, ok := e.Type().(*types.Union)
	if !ok {
		panic("gen: a message value is not a union")
	}
	c := s.child()
	key, m := f.tempVar(c, "_m")
	mv := &ir.Var{Typed: ir.Typed{Ty: e.Type()}, Kind: ir.VarLocal, Name: key}
	var w strings.Builder
	w.WriteString("func() rt.MsgRef {\n" + m + " := " + f.code(e, s) + "\nswitch " + m + ".Tag {\n")
	for _, ct := range u.Constructors() {
		rec, _ := ct.Arg.(*types.Record)
		payload := &ir.Field{Typed: ir.Typed{Ty: rec}, Expr: mv, Name: ctorField(ct.Name)}
		field := func(name string, t types.Type) ir.Expr {
			return &ir.Field{Typed: ir.Typed{Ty: t}, Expr: payload, Name: name}
		}
		w.WriteString("case " + tagConst(u.Module, u.Name, ct.Name) + ":\nreturn " + ref(u.Module, ct.Name, rec, field, c) + "\n")
	}
	w.WriteString("}\npanic(\"unreachable\")\n}()")
	return w.String()
}

// kvItem is one URL parameter: its name, its declared type and value, and for a page link its
// default (left out at it) and whether Nothing is left out.
type kvItem struct {
	name      string
	t         types.Type
	v         ir.Expr
	def       ir.Expr
	omitBlank bool
}

// kvList is items as a Go []rt.KV: rt.KV for a parameter always given, rt.QueryKV for one left
// out at its default (or at Nothing), rt.QueryKVs for a List (one key per element); groups
// joined with rt.Queries when there is a List.
func (f *fn) kvList(items []kvItem, s *scope) string {
	var groups, kvs []string
	lists := false
	flush := func() {
		if len(kvs) > 0 {
			groups = append(groups, "[]rt.KV{"+strings.Join(kvs, ", ")+"}")
			kvs = nil
		}
	}
	for _, it := range items {
		key := strconv.Quote(it.name)
		switch {
		case listArg(it.t) != nil:
			flush()
			groups = append(groups, "rt.QueryKVs("+key+", "+f.texts(it.v, listArg(it.t), s)+")")
			lists = true
		case it.def != nil:
			kvs = append(kvs, "rt.QueryKV("+key+", "+f.text(it.v, s)+", "+f.text(it.def, s)+")")
		case it.omitBlank && maybeArg(it.t) != nil:
			kvs = append(kvs, "rt.QueryKV("+key+", "+f.text(it.v, s)+`, "")`)
		default:
			kvs = append(kvs, "rt.KV{Key: "+key+", Value: "+f.text(it.v, s)+"}")
		}
	}
	flush()
	switch {
	case len(groups) == 0:
		return "nil"
	case !lists && len(groups) == 1:
		return groups[0]
	}
	return "rt.Queries(" + strings.Join(groups, ", ") + ")"
}

// text is e as a Go string: a message parameter or a form control's value. Literals and
// constructors written in place are converted statically.
func (f *fn) text(e ir.Expr, s *scope) string {
	switch x := e.(type) {
	case *ir.Lit:
		switch x.Kind {
		case ir.LitInt:
			return strconv.Quote(strconv.FormatInt(x.Int, 10))
		case ir.LitBool:
			if x.Bool {
				return `"True"`
			}
			return `"False"`
		}
	case *ir.Ctor:
		switch {
		case x.U.Module == "Prelude" && x.Name == "Nothing":
			return `""`
		case x.U.Module == "Prelude" && x.Name == "Just":
			if r, ok := x.Payload.(*ir.Record); ok && len(r.Fields) == 1 {
				return f.text(r.Fields[0].Value, s)
			}
		case x.U.Module != "Prelude" && x.Payload == nil:
			return strconv.Quote(x.Name)
		}
	}
	t := e.Type()
	if fn := textFunc(t); fn != "" {
		if fn == "string" {
			return f.code(e, s)
		}
		return fn + "(" + f.code(e, s) + ")"
	}
	if u, ok := t.(*types.Union); ok {
		if u.Module == "Prelude" && u.Name == "Maybe" {
			inner := u.Args[0]
			switch fn := textFunc(inner); fn {
			case "string":
				return `rt.OrElse(` + f.code(e, s) + `, "")`
			case "":
				c := s.child()
				key, v := f.tempVar(c, "_v")
				body := f.text(&ir.Var{Typed: ir.Typed{Ty: inner}, Kind: ir.VarLocal, Name: key}, c)
				return "rt.MaybeText(" + f.code(e, s) + ", func(" + v + " " + GoType(inner) + ") string {\nreturn " + body + "\n})"
			default:
				return "rt.MaybeText(" + f.code(e, s) + ", " + fn + ")"
			}
		}
		if names := ctorNames(u); names != nil {
			return "[]string{" + strings.Join(names, ", ") + "}[" + f.operand(e, s, precAtom) + ".Tag]"
		}
	}
	return "rt.TextOf(" + f.code(e, s) + ")"
}

// textFunc is the Go function converting a value of type t to text ("string" when it is text
// already), "" when there is none.
func textFunc(t types.Type) string {
	switch GoType(t) {
	case "string":
		return "string"
	case "int64":
		return "std.TextFromInt"
	case "float64":
		return "std.TextFromFloat"
	case "bool":
		return "std.BoolToText"
	case "rt.Date":
		return "std.DateToText"
	}
	return ""
}

// ctorNames are the quoted constructor names of a union of the app whose constructors have no
// payload, nil for any other type.
func ctorNames(t types.Type) []string {
	u, ok := t.(*types.Union)
	if !ok || u.Module == "Prelude" {
		return nil
	}
	var names []string
	for _, c := range u.Constructors() {
		if c.Arg != nil {
			return nil
		}
		names = append(names, strconv.Quote(c.Name))
	}
	return names
}

// comp calls a component with its props record, children included.
func (f *fn) comp(x *ir.Comp, s *scope) string {
	param := x.Fn.Type.(*types.Fun).Params[0]
	rec, _ := param.(*types.Record)
	given := map[string]ir.Expr{}
	if x.Props != nil {
		for _, fi := range x.Props.Fields {
			given[fi.Name] = fi.Value
		}
	}
	var fields, inits []string
	for _, fd := range rec.Fields {
		t := GoType(fd.Type)
		if fd.Name == "children" {
			t = "[]html.Node"
			if len(x.Children) > 0 {
				inits = append(inits, "Children: "+f.nodeList(x.Children, s))
			}
		} else if v := given[fd.Name]; v != nil {
			// a field whose declared type has type parameters takes the instance's type
			if ps := typeParams(fd.Type); len(ps) > 0 {
				t = GoType(v.Type())
			}
			val := f.code(v, s)
			if f.g.boxed.of(rec, fd.Name) { // a nominal props record's boxed field (types.go)
				val = "new(" + val + ")"
			}
			inits = append(inits, Field(fd.Name)+": "+val)
		}
		fields = append(fields, Field(fd.Name)+" "+t)
	}
	typ := "struct{ " + strings.Join(fields, "; ") + " }"
	if len(fields) == 0 {
		typ = "struct{}"
	}
	if rec.Name != "" && !rec.Anonymous {
		typ = GoName(rec.Module, rec.Name)
	}
	return GoName(x.Fn.Module, x.Fn.Name) + "(" + typ + "{" + strings.Join(inits, ", ") + "})"
}

// input renders <Input>, <Textarea>, <Select> or <Errors> from the bound form field's type.
func (f *fn) input(x *ir.Input, s *scope) string {
	kind := map[ir.InputKind]string{ir.InputInput: "input", ir.InputTextarea: "textarea", ir.InputSelect: "select", ir.InputErrors: "errors"}[x.Kind]
	out := []string{"Kind: " + strconv.Quote(kind)}
	if x.Form != nil {
		out = append(out, "Form: "+strconv.Quote(x.Form.Module+"."+x.Form.Name))
	}
	if x.Field != "" {
		out = append(out, "Name: "+strconv.Quote(x.Field))
	}
	if x.Kind == ir.InputErrors {
		return "html.Input(html.Control{" + strings.Join(out, ", ") + "})"
	}
	var ft types.Type
	var brand *ir.Brand
	hasDefault := false
	if x.Form != nil {
		for _, fd := range x.Form.Record.Fields {
			if fd.Name == x.Field {
				ft, brand, hasDefault = fd.Type, fd.Brand, fd.Default != nil
			}
		}
	}
	secret := isSecret(brand)
	// required only where the decoder refuses a blank value (book 05): not for a Maybe, a field
	// with a default (blank is the default), a checkbox, or Text that accepts "" (plain Text and
	// a brand of it other than the stdlib's NonEmpty, Email, Url and Secret)
	required := !hasDefault
	if u, ok := ft.(*types.Union); ok && u.Module == "Prelude" && u.Name == "Maybe" {
		ft, required = u.Args[0], false
	}
	if GoType(ft) == "string" && !blankRefused(brand) {
		required = false
	}
	if x.Kind == ir.InputInput {
		typ := "text"
		switch GoType(ft) {
		case "int64", "float64":
			typ = "number"
		case "rt.Date":
			typ = "date"
		case "bool":
			typ, required = "checkbox", false
		case "string":
			switch brandName(brand) {
			case "Text.Email":
				typ = "email"
			case "Text.Url":
				typ = "url"
			}
		}
		out = append(out, "Type: "+strconv.Quote(typ))
		if GoType(ft) == "float64" {
			out = append(out, `Step: "any"`) // a number input accepts only integers without it
		}
	}
	switch {
	case x.LabelExpr != nil:
		out = append(out, "Label: "+f.code(x.LabelExpr, s))
	case x.Label != "":
		out = append(out, "Label: "+strconv.Quote(x.Label))
	}
	if required {
		out = append(out, "Required: true")
	}
	if x.Value != nil {
		out = append(out, "Value: "+f.text(x.Value, s))
	}
	if x.Kind == ir.InputSelect {
		if names := ctorNames(ft); names != nil {
			out = append(out, "Options: []string{"+strings.Join(names, ", ")+"}")
		}
	}
	if secret {
		out = append(out, "Secret: true")
	}
	return "html.Input(html.Control{" + strings.Join(out, ", ") + "})"
}

// brandName is "<Module>.<Name>" of brand b, "" for none.
func brandName(b *ir.Brand) string {
	if b == nil {
		return ""
	}
	return b.Module + "." + b.Name
}

// blankRefused reports whether a Text field of brand b refuses "" when decoded: the stdlib's
// Text.NonEmpty, Email, Url and Secret do; plain Text and an app's brand need not.
func blankRefused(b *ir.Brand) bool {
	switch brandName(b) {
	case "Text.NonEmpty", "Text.Email", "Text.Url", "Text.Secret":
		return true
	}
	return false
}
