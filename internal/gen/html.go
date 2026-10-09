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
//	Input    html.Input(html.Control{…}), derived from the bound form field's type
//	FormEl   html.Form(url, []html.Attr{…}, children...)
//
// Attributes:
//
//	Text     html.A("name", e)
//	Bool     html.B("name", e)
//	Maybe    html.M("name", e)
//	Href     html.A("href", rt.PageURL(e))
//	Click    html.A("data-on:click", "@post('"+url+"')")
//	Submit   method="post", action=url and data-on:submit (html.Form adds them itself)
//
// where url is rt.MsgURL("<Module>", "<Msg>", rt.KV{Key: "<param>", Value: <text>}, …): the
// message's parameters in declaration order, as text (see text), without the submitted form.

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
		url := `""`
		var rest []ir.HAttr
		for _, a := range x.Attrs {
			if a.Kind == ir.AttrSubmit {
				url = f.msgURL(a.Value, s)
			} else {
				rest = append(rest, a)
			}
		}
		return "html.Form(" + url + ", " + f.attrs(rest, s) + f.nodeArgs(x.Children, s)
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
			out = append(out, `html.A("data-on:click", "@post('"+`+f.msgURL(a.Value, s)+`+"')")`)
		case ir.AttrSubmit: // on an element Lower did not make a FormEl
			url := f.msgURL(a.Value, s)
			out = append(out, `html.A("method", "post")`, `html.A("action", `+url+`)`,
				`html.A("data-on:submit", "@post('"+`+url+`+"')")`)
		}
	}
	return "[]html.Attr{" + strings.Join(out, ", ") + "}"
}

// msgURL is the URL message value e posts to. A constructor written in place gives its module,
// name and parameters statically; any other message value is switched on at run time.
func (f *fn) msgURL(e ir.Expr, s *scope) string {
	if c, ok := e.(*ir.Ctor); ok {
		args := []string{strconv.Quote(c.U.Module), strconv.Quote(c.Name)}
		if c.Payload != nil {
			if r, ok := c.Payload.(*ir.Record); ok {
				for _, fi := range r.Fields {
					if fi.Name != "form" {
						args = append(args, kv(fi.Name, f.text(fi.Value, s)))
					}
				}
			} else if rec, ok := c.Payload.Type().(*types.Record); ok {
				for _, fd := range rec.Fields {
					if fd.Name != "form" {
						args = append(args, kv(fd.Name, f.text(&ir.Field{Typed: ir.Typed{Ty: fd.Type}, Expr: c.Payload, Name: fd.Name}, s)))
					}
				}
			}
		}
		return "rt.MsgURL(" + strings.Join(args, ", ") + ")"
	}
	u, ok := e.Type().(*types.Union)
	if !ok {
		panic("gen: a message value is not a union")
	}
	c := s.child()
	key, m := f.tempVar(c, "_m")
	mv := &ir.Var{Typed: ir.Typed{Ty: e.Type()}, Kind: ir.VarLocal, Name: key}
	var w strings.Builder
	w.WriteString("func() string {\n" + m + " := " + f.code(e, s) + "\nswitch " + m + ".Tag {\n")
	for _, ct := range u.Constructors() {
		args := []string{strconv.Quote(u.Module), strconv.Quote(ct.Name)}
		if rec, ok := ct.Arg.(*types.Record); ok {
			payload := &ir.Field{Typed: ir.Typed{Ty: rec}, Expr: mv, Name: ctorField(ct.Name)}
			for _, fd := range rec.Fields {
				if fd.Name != "form" {
					args = append(args, kv(fd.Name, f.text(&ir.Field{Typed: ir.Typed{Ty: fd.Type}, Expr: payload, Name: fd.Name}, c)))
				}
			}
		}
		w.WriteString("case " + tagConst(u.Module, u.Name, ct.Name) + ":\nreturn rt.MsgURL(" + strings.Join(args, ", ") + ")\n")
	}
	w.WriteString("}\npanic(\"unreachable\")\n}()")
	return w.String()
}

func kv(key, value string) string {
	return "rt.KV{Key: " + strconv.Quote(key) + ", Value: " + value + "}"
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
			inits = append(inits, Field(fd.Name)+": "+f.code(v, s))
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
	if x.Field != "" {
		out = append(out, "Name: "+strconv.Quote(x.Field))
	}
	if x.Kind == ir.InputErrors {
		return "html.Input(html.Control{" + strings.Join(out, ", ") + "})"
	}
	var ft types.Type
	if x.Form != nil {
		for _, fd := range x.Form.Record.Fields {
			if fd.Name == x.Field {
				ft = fd.Type
			}
		}
	}
	required := true
	if u, ok := ft.(*types.Union); ok && u.Module == "Prelude" && u.Name == "Maybe" {
		ft, required = u.Args[0], false
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
		}
		out = append(out, "Type: "+strconv.Quote(typ))
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
	return "html.Input(html.Control{" + strings.Join(out, ", ") + "})"
}
