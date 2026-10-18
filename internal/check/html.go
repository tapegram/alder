// internal/check/html.go
package check

import (
	"reflect"
	"strings"

	"alder/internal/diag"
	"alder/internal/markup"
	"alder/internal/syntax"
	"alder/internal/types"
)

// htmlTags and boolAttrs are derived from the markup manifest. boolAttrs are the HTML standard's
// boolean attributes: written bare or `={Bool}`, rendered present or absent.
var htmlTags = map[string]bool{}
var boolAttrs = map[string]bool{}

func init() {
	for _, t := range markup.HTML.Tags {
		htmlTags[t.Name] = true
		for _, a := range t.Attrs {
			if a.Boolean {
				boolAttrs[a.Name] = true
			}
		}
	}
	for _, a := range markup.HTML.Global {
		if a.Boolean {
			boolAttrs[a.Name] = true
		}
	}
}

func (mc *modCtx) checkHTML(e syntax.Expr, expected types.Type, env *env, eff effect) {
	c := mc.c
	ex := c.u.Apply(expected)
	app, ok := ex.(*types.App)
	if !ok || app.Con != "Html" {
		if v, isVar := ex.(*types.Var); isVar {
			m := c.u.Fresh()
			c.u.Unify(v, &types.App{Con: "Html", Args: []types.Type{m}})
			mc.htmlNode(e, m, env, eff)
			return
		}
		if !types.IsErr(ex) {
			c.errorf(diag.TypeMismatch, e.GetSpan(), "expected %s, got Html", types.String(ex))
		}
		mc.htmlNode(e, c.u.Fresh(), env, eff)
		return
	}
	mc.htmlNode(e, app.Args[0], env, eff)
}

// htmlNode checks one HTML child against message type m.
func (mc *modCtx) htmlNode(e syntax.Expr, m types.Type, env *env, eff effect) {
	c := mc.c
	htmlT := c.record(e, &types.App{Con: "Html", Args: []types.Type{m}})
	switch x := e.(type) {
	case *syntax.HtmlText:
	case *syntax.Element:
		mc.element(x, m, env, eff)
	case *syntax.HtmlExpr:
		got := c.u.Apply(mc.infer(x.Expr, env, eff))
		if c.u.Check(got, types.Text) == nil || c.u.Check(got, htmlT) == nil || c.u.Check(got, &types.App{Con: "List", Args: []types.Type{htmlT}}) == nil {
			return
		}
		d := c.errorf(diag.TypeMismatch, x.Expr.GetSpan(), "expected Text, Html or List Html in `{…}`, got %s", types.String(got))
		mc.annotateMismatch(d, x.Expr, got, types.Text)
	case *syntax.HtmlFor:
		elem := c.u.Fresh()
		mc.check(x.List, &types.App{Con: "List", Args: []types.Type{elem}}, env, eff)
		child := env.child()
		c.bind(child, x, x.Var, elem)
		mc.htmlNode(x.Body, m, child, eff)
	case *syntax.HtmlIf:
		mc.check(x.Cond, types.Bool, env, eff)
		mc.htmlNode(x.Then, m, env, eff)
		mc.htmlNode(x.Else, m, env, eff)
	case *syntax.HtmlMatch:
		subj := c.u.Apply(mc.infer(x.Subject, env, eff))
		exh := newExhaustiveness(mc, subj)
		for _, a := range x.Arms {
			child := env.child()
			if exh.closed {
				c.errorf(diag.UnreachableArm, a.Span, "unreachable arm: every case is already handled")
			}
			mc.checkPattern(a.Pat, subj, child)
			exh.refresh(subj)
			exh.cover(a.Pat)
			mc.htmlNode(a.Body, m, child, eff)
		}
		exh.report(&syntax.Match{Node: x.Node, Subject: x.Subject, Arms: x.Arms})
	default:
		mc.check(e, htmlT, env, eff)
	}
}

func (mc *modCtx) element(x *syntax.Element, m types.Type, env *env, eff effect) {
	c := mc.c
	if x.Ref.Kind == syntax.RefComponent {
		if x.Ref.Module == "Prelude" {
			mc.formComponent(x, env, eff)
			return
		}
		mc.component(x, m, env, eff)
		return
	}
	if isUpperName(x.Tag) { // unresolved component: names already reported
		mc.walkUnchecked(x, m, env, eff)
		return
	}
	if strings.Contains(x.Tag, "-") {
		mc.customElement(x, m, env, eff)
		return
	}
	if !htmlTags[x.Tag] {
		c.errorf(diag.UnknownName, x.Span, "unknown element `<%s>`", x.Tag)
	}
	if !c.slotted[x] {
		mc.misplacedSlots(x)
	}
	if (x.Tag == "input" || x.Tag == "textarea" || x.Tag == "select") && mc.form != nil {
		c.errorf("E0304", x.Span, "plain `<%s>` inside `<form submit>`; use `<%s field=\"…\">`", x.Tag, map[string]string{"input": "Input", "textarea": "Textarea", "select": "Select"}[x.Tag])
	}
	var pushed *formCtx
	for _, a := range x.Attrs {
		mc.attribute(a, m, env, eff)
		if x.Tag == "form" && a.Name == "submit" && a.Value != nil {
			if f := mc.formOf(a.Value); f != nil {
				pushed = f
			} else {
				pushed = &formCtx{name: "", rec: &types.Record{Anonymous: true}}
			}
			pushed.submit = submitText(a.Value, mc.source(a.Value.GetSpan()))
			pushed.msg = msgCtor(a.Value)
		}
	}
	saved := mc.form
	if pushed != nil {
		mc.form = pushed
	}
	for _, ch := range x.Children {
		mc.htmlNode(ch, m, env, eff)
	}
	mc.form = saved
	if pushed != nil {
		for _, e := range pushed.errorsFor {
			if tag, ok := pushed.wrapped[e.field]; ok {
				c.errorf(diag.CustomElement, e.span, "`<%s field=\"%s\">` already shows the errors of `%s`; remove `<Errors field=\"%s\" />`", tag, e.field, e.field, e.field)
			}
		}
	}
	if pushed != nil && !pushed.formErrors && pushed.msg != nil && c.mayReject(pushed.msg.Ref.Module, pushed.msg.Ref.Name) {
		c.bag.Warn(diag.UnshownReject, x.Span, "`%s` can answer `Reject`, but this form has no `<Errors form />`: the user never sees the rejection text", pushed.msg.Ref.Name)
	}
	if pushed != nil && pushed.name != "" && !pushed.formErrors {
		for _, f := range pushed.rec.Fields {
			if !pushed.shown[f.Name] && !isCon(f.Type, "Bool") {
				c.bag.Warn(diag.UnshownErrors, x.Span, "form field `%s` has no `<Errors field=\"%s\">` and the form no `<Errors form />`: its decoding errors would not be shown", f.Name, f.Name)
			}
		}
	}
}

// msgCtor is the constructor of a message expression (`Save`, `Save { id: id }`) when it is a
// page's message, nil otherwise.
func msgCtor(e syntax.Expr) *syntax.CtorRef {
	if app, ok := e.(*syntax.App); ok {
		e = app.Fn
	}
	if ct, ok := e.(*syntax.CtorRef); ok && ct.Ref.Kind == syntax.RefCtor {
		return ct
	}
	return nil
}

// mayReject reports whether page module's `update` can answer `Reject` for its message msg (W0005):
// when `update` is a `match` on its message, whether the arms that can match msg mention
// `Reject`; otherwise whether `update` does anywhere. A `Reject` built by a function `update`
// calls is not seen.
func (c *Checker) mayReject(module, msg string) bool {
	m := c.prog.Modules[module]
	if m == nil || c.pages[module] == nil {
		return false
	}
	for _, d := range m.Decls {
		vd, ok := d.(*syntax.ValueDecl)
		if !ok || vd.Name != "update" || vd.Body == nil {
			continue
		}
		if mt, ok := vd.Body.(*syntax.Match); ok && len(vd.Params) > 0 {
			if v, ok := mt.Subject.(*syntax.Var); ok && v.Name == vd.Params[0].Name {
				for _, arm := range mt.Arms {
					if pc, ok := arm.Pat.(*syntax.PCtor); ok && pc.Name != msg {
						continue
					}
					if mentionsReject(reflect.ValueOf(arm.Body)) {
						return true
					}
				}
				return false
			}
		}
		return mentionsReject(reflect.ValueOf(vd.Body))
	}
	return false
}

// mentionsReject reports whether the syntax tree v holds the Prelude's `Reject`.
func mentionsReject(v reflect.Value) bool {
	switch v.Kind() {
	case reflect.Pointer, reflect.Interface:
		if v.IsNil() {
			return false
		}
		if ct, ok := v.Interface().(*syntax.CtorRef); ok {
			return ct.Ref.Kind == syntax.RefCtor && ct.Ref.Module == "Prelude" && ct.Ref.Name == "Reject"
		}
		return mentionsReject(v.Elem())
	case reflect.Struct:
		for i := range v.NumField() {
			if v.Type().Field(i).IsExported() && mentionsReject(v.Field(i)) {
				return true
			}
		}
	case reflect.Slice:
		for i := range v.Len() {
			if mentionsReject(v.Index(i)) {
				return true
			}
		}
	}
	return false
}

// submitText abbreviates a submit value for messages: `Toggle` or `Toggle …`.
func submitText(e syntax.Expr, src string) string {
	switch v := e.(type) {
	case *syntax.CtorRef:
		return v.Name
	case *syntax.App:
		if r, ok := v.Fn.(*syntax.CtorRef); ok {
			return r.Name + " …"
		}
	}
	return src
}

func isUpperName(s string) bool { return len(s) > 0 && s[0] >= 'A' && s[0] <= 'Z' }

// formOf finds the form record carried by a message expression (bare ctor or ctor applied to a record).
func (mc *modCtx) formOf(e syntax.Expr) *formCtx {
	c := mc.c
	var ct *syntax.CtorRef
	switch v := e.(type) {
	case *syntax.CtorRef:
		ct = v
	case *syntax.App:
		if r, ok := v.Fn.(*syntax.CtorRef); ok {
			ct = r
		}
	}
	if ct == nil || ct.Ref.Kind != syntax.RefCtor {
		return nil
	}
	info, ok := c.ctors[c.key(ct.Ref.Module, ct.Ref.Name)]
	if !ok {
		return nil
	}
	if rec, ok := info.Arg.(*types.Record); ok {
		for _, f := range rec.Fields {
			if f.Name == "form" {
				if fr, ok := f.Type.(*types.Record); ok {
					return &formCtx{name: fr.Name, rec: fr}
				}
			}
		}
	}
	return &formCtx{name: "", rec: &types.Record{Anonymous: true}}
}

func (mc *modCtx) attribute(a *syntax.HtmlAttr, m types.Type, env *env, eff effect) {
	c := mc.c
	if strings.HasPrefix(a.Name, "on") && len(a.Name) > 2 {
		c.errorf(diag.ParseError, a.Span, "`%s` is not Alder; use `click={Msg}`", a.Name)
		return
	}
	if a.Value == nil {
		if !boolAttrs[a.Name] && a.Name != "popover" { // a bare `popover` is `popover="auto"`
			c.errorf(diag.TypeMismatch, a.Span, "attribute `%s` needs a value", a.Name)
		}
		return
	}
	switch {
	case a.Name == "href":
		hrefText := func() {
			c.errorf("E0303", a.Span, "`href` takes a page value, not a string; write `href={Module.Page { … }}` or `Url.external \"…\"`")
		}
		if _, isStr := a.Value.(*syntax.StrLit); isStr {
			hrefText()
			return
		}
		got := c.u.Apply(mc.infer(a.Value, env, eff))
		if b, isBrand := got.(*types.Brand); isCon(got, "Text") || (isBrand && isCon(b.Base, "Text")) {
			hrefText() // a computed string such as `"/todos/" ++ x`
			return
		}
		if err := c.u.Check(got, types.Page); err != nil {
			mc.mismatch(a.Value.GetSpan(), got, types.Page)
		}
	case a.Name == "click" || a.Name == "submit":
		mc.checkMessage(a.Value, m, env, eff)
	case boolAttrs[a.Name]:
		mc.check(a.Value, types.Bool, env, eff)
	case a.Name == "src": // Text, or a page value: `Url.external "https://…"` (rendered as its URL)
		mc.textAttrOr(a, env, eff, types.Page)
	default:
		mc.textAttr(a, env, eff)
	}
}

// textAttr checks the value of an ordinary attribute: Text, or Maybe Text (omitted for Nothing).
func (mc *modCtx) textAttr(a *syntax.HtmlAttr, env *env, eff effect) {
	mc.textAttrOr(a, env, eff, nil)
}

// textAttrOr is textAttr that also accepts a value of type also (when not nil).
func (mc *modCtx) textAttrOr(a *syntax.HtmlAttr, env *env, eff effect, also types.Type) {
	c := mc.c
	got := c.u.Apply(mc.infer(a.Value, env, eff))
	if c.u.Check(got, types.Text) == nil {
		return
	}
	if c.u.Check(got, mc.maybeOf(types.Text)) == nil {
		return
	}
	if also != nil && c.u.Check(got, also) == nil {
		return
	}
	d := mc.mismatch(a.Value.GetSpan(), got, types.Text)
	mc.annotateMismatch(d, a.Value, got, types.Text)
}

// checkMessage checks a click/submit value: a page message (against m) or an app-level message.
func (mc *modCtx) checkMessage(e syntax.Expr, m types.Type, env *env, eff effect) {
	c := mc.c
	var ct *syntax.CtorRef
	applied := false
	switch v := e.(type) {
	case *syntax.CtorRef:
		ct = v
	case *syntax.App:
		if r, ok := v.Fn.(*syntax.CtorRef); ok {
			ct = r
			applied = true
		}
	}
	if ct == nil || ct.Ref.Kind != syntax.RefCtor || c.ctors[c.key(ct.Ref.Module, ct.Ref.Name)] == nil {
		mc.check(e, m, env, eff)
		return
	}
	target := m
	if ct.Ref.Module == "App" {
		target = c.appMsg
	}
	var got types.Type
	if applied {
		got = mc.infer(e, env, eff)
	} else {
		got = c.record(ct, mc.inferCtor(ct, false)) // reports E0301 for messages that need parameters
	}
	if err := c.u.Unify(got, target); err != nil {
		c.errorf(diag.TypeMismatch, e.GetSpan(), "`%s` is not a message of this page", ct.Name)
	}
}

// component checks <Mod.Name attrs>children</Mod.Name> as a call of Name with a props record.
func (mc *modCtx) component(x *syntax.Element, m types.Type, env *env, eff effect) {
	c := mc.c
	sig, ok := c.values[c.key(x.Ref.Module, x.Ref.Name)]
	if !ok {
		mc.walkUnchecked(x, m, env, eff)
		return
	}
	fn, isFn := c.u.Instantiate(sig).(*types.Fun)
	if types.IsErr(sig) || (isFn && len(fn.Params) == 1 && types.IsErr(c.u.Apply(fn.Params[0]))) {
		mc.walkUnchecked(x, m, env, eff) // its signature names an unresolved type, already reported
		return
	}
	if !isFn || len(fn.Params) != 1 {
		c.errorf(diag.TypeMismatch, x.Span, "component `%s` must take exactly one record argument", x.Tag)
		return
	}
	props, isRec := c.u.Apply(fn.Params[0]).(*types.Record)
	if !isRec {
		c.errorf(diag.TypeMismatch, x.Span, "component `%s` must take a record, got `%s`", x.Tag, types.String(fn.Params[0]))
		return
	}
	lit := &syntax.RecordLit{Node: x.Node}
	for _, a := range x.Attrs {
		val := a.Value
		if val == nil {
			val = &syntax.BoolLit{Node: syntax.Node{Span: a.Span}, Value: true}
		}
		lit.Fields = append(lit.Fields, &syntax.FieldInit{Node: syntax.Node{Span: a.Span}, Name: a.Name, Value: val})
	}
	if len(x.Children) > 0 {
		// children: List (Html m) — check them directly, then mark the field as supplied
		for _, ch := range x.Children {
			mc.htmlNode(ch, m, env, eff)
		}
		lit.Fields = append(lit.Fields, &syntax.FieldInit{Node: x.Node, Name: "children", Value: &syntax.ListLit{Node: x.Node}})
	}
	mc.checkRecordLit(lit, props, env, eff)
	// unify the children field's element type with Html m, if present
	for _, f := range props.Fields {
		if f.Name == "children" {
			c.u.Check(&types.App{Con: "List", Args: []types.Type{&types.App{Con: "Html", Args: []types.Type{m}}}}, f.Type)
		}
	}
	if err := c.u.Check(fn.Result, &types.App{Con: "Html", Args: []types.Type{m}}); err != nil {
		mc.mismatch(x.Span, fn.Result, &types.App{Con: "Html", Args: []types.Type{m}})
	}
}

// formComponent checks <Input>, <Textarea>, <Select>, <Errors>.
func (mc *modCtx) formComponent(x *syntax.Element, env *env, eff effect) {
	mc.formControl(x, x.Tag, x.Attrs, env, eff)
}

// formControl checks the form control x of kind "Input", "Textarea", "Select", "Errors" or
// "Checkbox" (an `<Input>` on a Bool field) with attributes attrs: a form component, or a
// built-in input element that wraps one (`<al-input>`, whose other attributes are its own).
// Messages name x's tag as written.
func (mc *modCtx) formControl(x *syntax.Element, kind string, attrs []*syntax.HtmlAttr, env *env, eff effect) {
	c := mc.c
	if mc.form == nil {
		c.errorf("E0304", x.Span, "`<%s>` must be inside `<form submit={…}>`", x.Tag)
		return
	}
	var fieldName string
	var value, options syntax.Expr
	bareForm := false
	for _, a := range attrs {
		if a.Value == nil && a.Name != "form" {
			c.errorf("E0304", a.Span, "`<%s>` attribute `%s` needs a value", x.Tag, a.Name)
			continue
		}
		switch a.Name {
		case "field":
			c.record(a.Value, types.Text)
			if s, ok := a.Value.(*syntax.StrLit); ok && len(s.Parts) == 1 && s.Parts[0].Expr == nil {
				fieldName = s.Parts[0].Text
			} else {
				c.errorf("E0304", a.Span, "`field` must be a literal field name")
			}
		case "label":
			mc.check(a.Value, types.Text, env, eff)
		case "value":
			value = a.Value
		case "options":
			if kind != "Select" {
				c.errorf("E0304", a.Span, "`options` belongs on `<Select>` or `<al-select>`, not on `<%s>`", x.Tag)
				mc.infer(a.Value, env, eff)
				continue
			}
			options = a.Value
		case "form":
			bareForm = true
		default:
			c.errorf("E0304", a.Span, "`<%s>` has no attribute `%s`", x.Tag, a.Name)
		}
	}
	if mc.form.name == "" {
		c.errorf("E0304", x.Span, "`<form submit={%s}>` has no form type; `<%s>` needs a message declared with `form F`", mc.form.submit, x.Tag)
		return
	}
	if kind == "Errors" && bareForm {
		mc.form.formErrors = true
		return
	}
	if kind == "Errors" && fieldName != "" {
		if mc.form.shown == nil {
			mc.form.shown = map[string]bool{}
		}
		mc.form.shown[fieldName] = true
		mc.form.errorsFor = append(mc.form.errorsFor, errorsAt{fieldName, x.Span})
	}
	if fieldName == "" {
		c.errorf("E0304", x.Span, "`<%s>` needs `field=\"…\"`", x.Tag)
		return
	}
	var ft types.Type
	var names []string
	for _, f := range mc.form.rec.Fields {
		names = append(names, f.Name)
		if f.Name == fieldName {
			ft = f.Type
		}
	}
	if ft == nil {
		c.errorf("E0304", x.Span, "form `%s` has no field `%s`; fields: %s", mc.form.name, fieldName, strings.Join(names, ", "))
		return
	}
	inner := ft
	if in, ok := maybeOf(ft); ok {
		inner = in
	}
	isUnion := func() bool { u, ok := inner.(*types.Union); return ok && isScalar(u) }()
	selectTag := "Select"
	if strings.HasPrefix(x.Tag, "al-") {
		selectTag = "al-select"
	}
	switch kind {
	case "Input", "Textarea":
		if isUnion {
			c.errorf("E0304", x.Span, "field `%s` is a union; use `<%s>` for `%s`", fieldName, selectTag, types.String(inner))
		}
	case "Select":
		switch {
		case options != nil && isUnion:
			c.errorf("E0304", options.GetSpan(), "field `%s` is a union, whose options are its constructors; `options` is for a field of `Text` or a brand of `Text`, such as a row's id", fieldName)
			mc.infer(options, env, eff)
		case options != nil && !isTextLike(inner):
			c.errorf("E0304", options.GetSpan(), "`options` needs a field of `Text` or a brand of `Text`, such as a row's id; `%s` has type `%s`", fieldName, types.String(ft))
			mc.infer(options, env, eff)
		case options != nil:
			// the options are the values the field may take, each with its label; a submitted value
			// that is not one of them is refused, so the runtime computes them again on submit
			if mc.page == nil || mc.value != "view" {
				c.errorf("E0304", options.GetSpan(), "`options` must be written in the page's `view`, which the runtime runs again from the model to check a submitted value; `<%s>` is in `%s`", x.Tag, map[bool]string{true: mc.value, false: "a component"}[mc.value != ""])
			}
			mc.options, mc.optionsBad = env, map[string]bool{}
			mc.check(options, &types.App{Con: "List", Args: []types.Type{&types.Record{Anonymous: true, Fields: []types.Field{{Name: "value", Type: inner}, {Name: "label", Type: types.Text}}}}}, env, eff)
			mc.options, mc.optionsBad = nil, nil
		case !isUnion:
			c.errorf("E0304", x.Span, "`<%s>` needs a union field, or `options={…}` for a field of `Text` or a brand of `Text`; `%s` has type `%s`", x.Tag, fieldName, types.String(ft))
		}
	case "Checkbox":
		if !isCon(inner, "Bool") {
			c.errorf("E0304", x.Span, "`<%s>` needs a Bool field; `%s` has type `%s`", x.Tag, fieldName, types.String(ft))
		}
	}
	if value != nil {
		got := c.u.Apply(mc.infer(value, env, eff))
		if c.u.Check(got, ft) != nil && c.u.Check(got, mc.maybeOf(ft)) != nil {
			mc.mismatch(value.GetSpan(), got, ft)
		}
	}
}

// optionsVar reports E0304 for local variable x of an options expression (checked in env) that
// is bound inside the view rather than by it (a `for`, `let` or `match` binding): the runtime
// computes the options again from the view's parameters alone. Names bound inside the
// expression itself (a lambda's) are fine.
func (mc *modCtx) optionsVar(x *syntax.Var, env *env) {
	inside := true // env frames below the options expression's own
	for f := env; f != nil; f = f.parent {
		if f == mc.options {
			inside = false
		}
		if _, ok := f.vars[x.Name]; ok {
			if !inside && f.parent != nil && !mc.optionsBad[x.Name] {
				mc.optionsBad[x.Name] = true
				mc.c.errorf("E0304", x.Span, "`options` may use only the view's parameters (`model`, `session`): `%s` is bound inside the view, and the runtime computes the options again from the model when the form is submitted; put the list in the model", x.Name)
			}
			return
		}
	}
}

// isTextLike reports whether t is Text or a brand of Text: a field `<Select options>` binds.
func isTextLike(t types.Type) bool {
	if b, ok := t.(*types.Brand); ok {
		t = b.Base
	}
	return isCon(t, "Text") || types.IsErr(t)
}

// walkUnchecked still visits the attribute values and children of an element whose own check cannot run.
func (mc *modCtx) walkUnchecked(x *syntax.Element, m types.Type, env *env, eff effect) {
	for _, a := range x.Attrs {
		if a.Value != nil {
			mc.infer(a.Value, env, eff)
		}
	}
	for _, ch := range x.Children {
		mc.htmlNode(ch, m, env, eff)
	}
}

// misplacedSlots reports a literal `slot="…"` on a child element of plain element x (looking
// through `if`, `for` and `match`): a slot names a slot of the enclosing custom element, so the
// child must be the custom element's own (E0028). A component's children are not looked at.
func (mc *modCtx) misplacedSlots(x *syntax.Element) {
	var visit func(e syntax.Expr)
	visit = func(e syntax.Expr) {
		switch ch := e.(type) {
		case *syntax.Element:
			if ch.Ref.Kind == syntax.RefComponent || isUpperName(ch.Tag) {
				return
			}
			for _, a := range ch.Attrs {
				if name, ok := literalText(a.Value); a.Name == "slot" && a.Value != nil && ok {
					mc.c.errorf(diag.CustomElement, a.Span, "`slot` names a slot of the enclosing custom element; `<%s slot=\"%s\">` is inside `<%s>`", ch.Tag, name, x.Tag)
				}
			}
		case *syntax.HtmlIf:
			visit(ch.Then)
			visit(ch.Else)
		case *syntax.HtmlFor:
			visit(ch.Body)
		case *syntax.HtmlMatch:
			for _, arm := range ch.Arms {
				visit(arm.Body)
			}
		}
	}
	for _, ch := range x.Children {
		visit(ch)
	}
}
