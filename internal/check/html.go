// internal/check/html.go
package check

import (
	"cmp"
	"reflect"
	"slices"
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
			c.errorf(diag.TypeMismatch, e.GetSpan(), "expected %s, got Html", mc.ts(ex))
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
		d := c.errorf(diag.TypeMismatch, x.Expr.GetSpan(), "expected Text, Html or List Html in `{…}`, got %s", mc.ts(got))
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
		for _, a := range x.Arms {
			child := env.child()
			mc.checkPattern(a.Pat, subj, child)
			mc.htmlNode(a.Body, m, child, eff)
		}
		mc.checkArms(subj, &syntax.Match{Node: x.Node, Subject: x.Subject, Arms: x.Arms})
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
		mc.helperRef(x.Ref.Module, x.Ref.Name, x.Tag, x.Span)
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
	if x.Tag == "form" {
		mc.formWireNames(x)
	}
	mc.triggerAttrs(x)
	var pushed *formCtx
	for _, a := range x.Attrs {
		mc.attribute(x, a, m, env, eff)
		if x.Tag == "form" && a.Name == "submit" && a.Value != nil {
			if f := mc.formOf(a.Value); f != nil {
				pushed = f
			} else {
				pushed = &formCtx{name: "", rec: &types.Record{Anonymous: true}}
			}
			pushed.submit = submitText(a.Value, mc.source(a.Value.GetSpan()))
			if _, r := mc.routeTarget(a.Value); r != nil && isWrite(r.Method) {
				pushed.route = r
			}
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
	if pushed != nil && !pushed.formErrors && pushed.route != nil && c.mayInvalid(pushed.route) {
		c.bag.Warn(diag.UnshownReject, x.Span, "`%s` can answer `Response.invalid`, but this form has no `<Errors form />`: the user never sees its text", pushed.route.Name)
	}
	if pushed != nil && pushed.name != "" && !pushed.formErrors {
		for _, f := range pushed.rec.Fields {
			if !pushed.shown[f.Name] && !isCon(f.Type, "Bool") {
				c.bag.Warn(diag.UnshownErrors, x.Span, "form field `%s` has no `<Errors field=\"%s\">` and the form no `<Errors form />`: its decoding errors would not be shown", f.Name, f.Name)
			}
		}
	}
}

// mayInvalid reports whether write route r's `update` can answer `Response.invalid` (W0005):
// whether its body mentions it. A `Response.invalid` built by a function `update` calls is not
// seen.
func (c *Checker) mayInvalid(r *RouteInfo) bool {
	vd := c.routeMembers(c.prog.Modules[r.Module], r.Name)["update"]
	return vd != nil && vd.Body != nil && mentions(reflect.ValueOf(vd.Body), isInvalid)
}

// isInvalid reports whether syntax node n is `Response.invalid`.
func isInvalid(n any) bool {
	v, ok := n.(*syntax.Var)
	return ok && v.Ref.Kind == syntax.RefStd && v.Ref.Module == "Response" && v.Ref.Name == "invalid"
}

// mentions reports whether the syntax tree v holds a node that is reports.
func mentions(v reflect.Value, is func(any) bool) bool {
	switch v.Kind() {
	case reflect.Pointer, reflect.Interface:
		if v.IsNil() {
			return false
		}
		if is(v.Interface()) {
			return true
		}
		return mentions(v.Elem(), is)
	case reflect.Struct:
		for i := range v.NumField() {
			if v.Type().Field(i).IsExported() && mentions(v.Field(i), is) {
				return true
			}
		}
	case reflect.Slice:
		for i := range v.Len() {
			if mentions(v.Index(i), is) {
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

// formOf finds the form record carried by a submit value: a write route with `form F`, bare or
// applied to its parameters.
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
	if ct == nil {
		return nil
	}
	if ct.Ref.Kind != syntax.RefRoute {
		return nil
	}
	info := c.route(ct.Ref.Module, ct.Ref.Name) // a write route's `form F` is its parameters' field `form`
	if info == nil {
		return nil
	}
	if rec := info.Params; rec != nil {
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

// attribute checks attribute a of element x.
func (mc *modCtx) attribute(x *syntax.Element, a *syntax.HtmlAttr, m types.Type, env *env, eff effect) {
	c := mc.c
	if strings.HasPrefix(a.Name, "on") && len(a.Name) > 2 {
		c.errorf(diag.ParseError, a.Span, "`%s` is not Alder; use `click={Route}`", a.Name)
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
			c.errorf("E0303", a.Span, "`href` takes a GET route applied to its parameters, not a string; write `href={Index { … }}` or `Url.external \"…\"`")
		}
		if _, isStr := a.Value.(*syntax.StrLit); isStr {
			hrefText()
			return
		}
		if mc.bareRoute(a.Value, false) {
			return
		}
		if ct, r := mc.routeTarget(a.Value); r != nil && isWrite(r.Method) {
			c.errorf(diag.RouteTarget, a.Value.GetSpan(), "`href` goes to a GET route; `%s` is %s: use `click`", ct.Name, r.Method)
			mc.infer(a.Value, env, eff)
			return
		}
		got := c.u.Apply(mc.infer(a.Value, env, eff))
		if b, isBrand := got.(*types.Brand); isCon(got, "Text") || (isBrand && isCon(b.Base, "Text")) {
			hrefText() // a computed string such as `"/todos/" ++ x`
			return
		}
		if err := c.u.Check(got, types.Route); err != nil {
			mc.mismatch(a.Value.GetSpan(), got, types.Route)
		}
	case a.Name == "click" || a.Name == "submit":
		if mc.bareRoute(a.Value, false) {
			return
		}
		ct, r := mc.routeTarget(a.Value)
		if a.Name == "submit" && r != nil && !isWrite(r.Method) {
			c.errorf(diag.RouteTarget, a.Value.GetSpan(), "`submit` posts to a write route; `%s` is GET: link to it with `href`", ct.Name)
			mc.infer(a.Value, env, eff)
			return
		}
		if a.Name == "click" && r != nil && isWrite(r.Method) && r.Decl.Form != "" {
			c.errorf(diag.RouteTarget, a.Value.GetSpan(), "`click` sends a route without a form; `%s` posts form `%s`: use a submit button in a form for that route (`<form submit={%s}>`)", ct.Name, r.Decl.Form, ct.Name)
			mc.infer(a.Value, env, eff)
			return
		}
		mc.check(a.Value, m, env, eff)
	case a.Name == "load" || a.Name == "intersect":
		mc.triggerRoute(x, a, env, eff)
	case a.Name == "input":
		mc.inputAttr(x, a, env, eff)
	case a.Name == "every":
		if attrNamed(x, "load") != nil { // without one, triggerAttrs reported it
			mc.everyAttr(a, env, eff)
		} else {
			mc.infer(a.Value, env, eff)
		}
	case boolAttrs[a.Name]:
		mc.check(a.Value, types.Bool, env, eff)
	case a.Name == "src": // Text, or a route value: `Url.external "https://…"` (rendered as its URL)
		mc.textAttrOr(a, env, eff, types.Route)
	default:
		mc.textAttr(a, env, eff)
	}
}

// triggerNames are the route trigger attributes an element takes one of (#160).
var triggerNames = []string{"load", "intersect", "input"}

// triggerAttrs reports (E0047) the route trigger attributes of plain element x that cannot go
// together: two of `load`, `intersect` and `input`, or an `every` without a `load`. It runs once
// per element, before its attributes are checked.
func (mc *modCtx) triggerAttrs(x *syntax.Element) {
	c := mc.c
	var first *syntax.HtmlAttr
	for _, a := range x.Attrs {
		if !slices.Contains(triggerNames, a.Name) {
			continue
		}
		if first == nil {
			first = a
			continue
		}
		c.errorf(diag.RouteTrigger, a.Span, "an element takes one of `load`, `intersect`, `input`; this one has `%s` and `%s`", first.Name, a.Name)
	}
	if every := attrNamed(x, "every"); every != nil && attrNamed(x, "load") == nil {
		c.errorf(diag.RouteTrigger, every.Span, "`every` repeats a `load`; add `load={Route}` beside it")
	}
}

// attrNamed is x's first attribute called name, nil when it has none.
func attrNamed(x *syntax.Element, name string) *syntax.HtmlAttr {
	for _, a := range x.Attrs {
		if a.Name == name {
			return a
		}
	}
	return nil
}

// triggerRoute checks the value of route trigger attribute a of element x (`load`, `intersect`,
// `input`): a GET route, written applied to its parameters. A write route is E0045. It returns
// the route's constructor and route when the value is written as one and is a GET route, else
// nil, nil (also for a computed Route value, whose route is not known).
func (mc *modCtx) triggerRoute(x *syntax.Element, a *syntax.HtmlAttr, env *env, eff effect) (*syntax.CtorRef, *RouteInfo) {
	c := mc.c
	if mc.bareRoute(a.Value, false) {
		return nil, nil
	}
	ct, r := mc.routeTarget(a.Value)
	if r != nil && isWrite(r.Method) {
		switch {
		case a.Name == "load" && attrNamed(x, "every") != nil:
			c.errorf(diag.RouteTarget, a.Value.GetSpan(), "`load … every` fetches a GET route; `%s` is %s: a timer must not write", ct.Name, r.Method)
		case a.Name == "input":
			c.errorf(diag.RouteTarget, a.Value.GetSpan(), "`input` fetches a GET route; `%s` is %s: use `submit`", ct.Name, r.Method)
		default:
			c.errorf(diag.RouteTarget, a.Value.GetSpan(), "`%s` fetches a GET route; `%s` is %s: use `click`", a.Name, ct.Name, r.Method)
		}
		mc.infer(a.Value, env, eff)
		return nil, nil
	}
	got := c.u.Apply(mc.infer(a.Value, env, eff))
	if err := c.u.Check(got, types.Route); err != nil {
		mc.mismatch(a.Value.GetSpan(), got, types.Route)
		return nil, nil
	}
	return ct, r
}

// inputAttr checks `input={R}` on plain element x: a control (`<input>`, `<textarea>`,
// `<select>`) with a literal `name`, the query key it sets on R.
func (mc *modCtx) inputAttr(x *syntax.Element, a *syntax.HtmlAttr, env *env, eff effect) {
	c := mc.c
	if x.Tag != "input" && x.Tag != "textarea" && x.Tag != "select" {
		c.errorf(diag.RouteTrigger, a.Span, "`input` fires as a field changes; put it on `<input>`, `<textarea>`, `<select>` or `<Input field=…>`")
		mc.triggerRoute(x, a, env, eff)
		return
	}
	var key string
	ok := false
	if n := attrNamed(x, "name"); n != nil && n.Value != nil {
		key, ok = literalText(n.Value)
	}
	if !ok || key == "" {
		c.errorf(diag.RouteTrigger, a.Span, "`input` sends the field's value under its name; give `<%s>` a literal `name`", x.Tag)
		mc.triggerRoute(x, a, env, eff)
		return
	}
	mc.inputKey(x, a, key, env, eff)
}

// inputKey checks the route of `input` attribute a of control x, which sends its value as query
// key key: a GET route with a `Text` query parameter of that key (E0047).
func (mc *modCtx) inputKey(x *syntax.Element, a *syntax.HtmlAttr, key string, env *env, eff effect) {
	c := mc.c
	ct, r := mc.triggerRoute(x, a, env, eff)
	if r == nil {
		if mc.computedRoute(a.Value) {
			c.errorf(diag.RouteTrigger, a.Value.GetSpan(), "`input` needs a route written in place, like `input={Results { q: model.q }}`, so the compiler can check its query key")
		}
		return
	}
	f, ok := queryKey(r, key)
	switch {
	case !ok && slices.Contains(pathParams(r.Path), key):
		c.errorf(diag.RouteTrigger, a.Value.GetSpan(), "`input` sends field `%s`; `%s` has `%s` in its path, and the browser can only set a query parameter", key, ct.Name, key)
	case !ok:
		c.errorf(diag.RouteTrigger, a.Value.GetSpan(), "`input` sends field `%s`; `%s` has no query parameter `%s`", key, ct.Name, key)
	default:
		for _, p := range r.Params.Fields {
			if p.Name == f.Name && !isCon(c.u.Apply(p.Type), "Text") {
				c.errorf(diag.RouteTrigger, a.Value.GetSpan(), "`input` sends field `%s` as text; `%s.%s` is %s, so make it `Text`", key, ct.Name, f.Name, mc.ts(p.Type))
			}
		}
	}
}

// computedRoute reports whether e, already checked, is a Route value not written as a route
// constructor (`model.r`, `Url.external "…"`), whose route the checker does not know.
func (mc *modCtx) computedRoute(e syntax.Expr) bool {
	head := e
	if app, ok := e.(*syntax.App); ok {
		head = app.Fn
	}
	if ct, ok := head.(*syntax.CtorRef); ok && ct.Ref.Kind == syntax.RefRoute {
		return false
	}
	t, ok := mc.c.res.Types[e]
	return ok && mc.c.u.Check(mc.c.u.Apply(t), types.Route) == nil
}

// queryKey is route r's query parameter read from query key key: its `as "Key"` wire name, or
// its name when it has none. Path parameters and the form are not query parameters.
func queryKey(r *RouteInfo, key string) (*syntax.Field, bool) {
	if r.Decl == nil || r.Decl.Params == nil {
		return nil, false
	}
	inPath := pathParams(r.Path)
	for _, f := range r.Decl.Params.Fields {
		if f.Name == "form" || slices.Contains(inPath, f.Name) {
			continue
		}
		if k := cmp.Or(f.Wire, f.Name); k == key {
			return f, true
		}
	}
	return nil, false
}

// everyAttr checks the value of `every`: a literal duration (syntax.EveryDuration), E0047
// otherwise.
func (mc *modCtx) everyAttr(a *syntax.HtmlAttr, env *env, eff effect) {
	lit, ok := literalText(a.Value)
	if _, valid := syntax.EveryDuration(lit); ok && valid {
		mc.c.record(a.Value, types.Text)
		return
	}
	mc.c.errorf(diag.RouteTrigger, a.Value.GetSpan(), "`every` takes a literal like \"5s\": a whole number above 0 then `ms`, `s` or `m`")
	mc.infer(a.Value, env, eff)
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
		c.errorf(diag.TypeMismatch, x.Span, "component `%s` must take a record, got `%s`", x.Tag, mc.ts(fn.Params[0]))
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
	var trigger *syntax.HtmlAttr
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
		case "input":
			trigger = a
		default:
			c.errorf("E0304", a.Span, "`<%s>` has no attribute `%s`", x.Tag, a.Name)
		}
	}
	if trigger != nil {
		switch {
		case kind != "Input" && kind != "Textarea" && kind != "Select":
			c.errorf(diag.RouteTrigger, trigger.Span, "`input` fires as a field changes; put it on `<input>`, `<textarea>`, `<select>` or `<Input field=…>`")
			mc.infer(trigger.Value, env, eff)
		case fieldName == "": // E0304 above or below: `field` missing or not literal
			mc.triggerRoute(x, trigger, env, eff)
		default:
			mc.inputKey(x, trigger, fieldName, env, eff)
		}
	}
	if mc.form.name == "" {
		what := "a message"
		if mc.form.route != nil {
			what = "a route"
		}
		c.errorf("E0304", x.Span, "`<form submit={%s}>` has no form type; `<%s>` needs %s declared with `form F`", mc.form.submit, x.Tag, what)
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
			c.errorf("E0304", x.Span, "field `%s` is a union; use `<%s>` for `%s`", fieldName, selectTag, mc.ts(inner))
		}
	case "Select":
		switch {
		case options != nil && isUnion:
			c.errorf("E0304", options.GetSpan(), "field `%s` is a union, whose options are its constructors; `options` is for a field of `Text` or a brand of `Text`, such as a row's id", fieldName)
			mc.infer(options, env, eff)
		case options != nil && !isTextLike(inner):
			c.errorf("E0304", options.GetSpan(), "`options` needs a field of `Text` or a brand of `Text`, such as a row's id; `%s` has type `%s`", fieldName, mc.ts(ft))
			mc.infer(options, env, eff)
		case options != nil:
			// the options are the values the field may take, each with its label; a submitted value
			// that is not one of them is refused, so the runtime computes them again on submit
			if !mc.inShownView() {
				c.errorf("E0304", options.GetSpan(), "`options` must be written in a GET route's `view`, which the runtime renders again from the model to check a submitted value; `<%s>` is in `%s`", x.Tag, map[bool]string{true: mc.value, false: "a component"}[mc.value != ""])
			} else {
				mc.recordOptions(mc.form.route)
			}
			mc.options, mc.optionsBad = env, map[string]bool{}
			mc.check(options, &types.App{Con: "List", Args: []types.Type{&types.Record{Anonymous: true, Fields: []types.Field{{Name: "value", Type: inner}, {Name: "label", Type: types.Text}}}}}, env, eff)
			mc.options, mc.optionsBad = nil, nil
		case !isUnion:
			c.errorf("E0304", x.Span, "`<%s>` needs a union field, or `options={…}` for a field of `Text` or a brand of `Text`; `%s` has type `%s`", x.Tag, fieldName, mc.ts(ft))
		}
	case "Checkbox":
		if !isCon(inner, "Bool") {
			c.errorf("E0304", x.Span, "`<%s>` needs a Bool field; `%s` has type `%s`", x.Tag, fieldName, mc.ts(ft))
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
