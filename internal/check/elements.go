// internal/check/elements.go
package check

import (
	"errors"
	"strings"

	"alder/internal/diag"
	"alder/internal/elements"
	"alder/internal/syntax"
	"alder/internal/types"
)

// Custom elements (chapter 5, "Custom elements"): a tag with a hyphen is checked against the
// Custom Elements Manifests, the embedded `alder-ui` one and the app's
// elements/custom-elements.json (E0028).

// loadElements unites the embedded manifest with the app's, reporting an app file that does
// not parse (E0028, at the file).
func (c *Checker) loadElements() {
	var app *elements.Manifest
	if data := c.prog.ElementsManifest; data != nil {
		m, err := elements.Parse(data)
		if err != nil {
			line, col := 1, 1
			var e *elements.Error
			if errors.As(err, &e) {
				line, col = e.Line, e.Col
			}
			c.errorf(diag.CustomElement, diag.Span{File: elements.Path, Start: diag.Pos{Line: line, Col: col}, End: diag.Pos{Line: line, Col: col + 1}},
				"`%s`: %s", elements.Path, err.Error())
			c.appElementsBad = true
		}
		app = m
	}
	c.elements = elements.Merge(elements.Builtin(), app)
}

// customElement checks <x-y …>…</x-y>: the tag, attributes and children's slots against its
// manifest entry. A built-in input element inside a form, or given `field`, wraps a form
// control (wrappedInput).
func (mc *modCtx) customElement(x *syntax.Element, m types.Type, env *env, eff effect) {
	c := mc.c
	el := c.elements.Lookup(x.Tag)
	if el == nil {
		if !c.appElementsBad { // the app's file may have declared it: its error is reported
			c.errorf(diag.CustomElement, x.Span, "unknown element `<%s>`: declare custom elements in `%s` (chapter 5)", x.Tag, elements.Path)
		}
		mc.walkUnchecked(x, m, env, eff)
		return
	}
	if kind, ok := elements.WrappedInputs[x.Tag]; ok && el.Builtin && (mc.form != nil || hasHTMLAttr(x, "field")) {
		mc.wrappedInput(x, el, kind, m, env, eff)
		return
	}
	for _, a := range x.Attrs {
		mc.customAttr(x, el, a, m, env, eff)
	}
	mc.checkSlots(x, el)
	if el.Builtin {
		if r := rendered(x); r != "" {
			c.res.Rendered[x] = r
		}
	}
	for _, ch := range x.Children {
		mc.htmlNode(ch, m, env, eff)
	}
}

// wrappedInput checks a built-in input element that is the form control kind (Input, Textarea,
// Select, Checkbox): `field`, `label` and `value` exactly as on that form component, its other
// attributes as the manifest declares them. The renderer writes the control and the field's
// errors list inside it, so the field's errors are shown (W0003) and it takes no children.
func (mc *modCtx) wrappedInput(x *syntax.Element, el *elements.Element, kind string, m types.Type, env *env, eff effect) {
	c := mc.c
	c.res.Wrapped[x] = kind
	var formAttrs []*syntax.HtmlAttr
	for _, a := range x.Attrs {
		switch a.Name {
		case "field", "label", "value":
			formAttrs = append(formAttrs, a)
		default:
			mc.customAttr(x, el, a, m, env, eff)
		}
	}
	mc.formControl(x, kind, formAttrs, env, eff)
	if mc.form != nil {
		for _, a := range formAttrs {
			if name, ok := literalText(a.Value); ok && a.Name == "field" {
				if mc.form.shown == nil {
					mc.form.shown = map[string]bool{}
				}
				mc.form.shown[name] = true
			}
		}
	}
	if len(x.Children) > 0 {
		c.errorf("E0304", x.Span, "`<%s field=…>` takes no children: Alder renders its label, control and errors list", x.Tag)
		for _, ch := range x.Children {
			mc.htmlNode(ch, m, env, eff)
		}
	}
}

// customAttr checks attribute a of custom element x (entry el): declared by the manifest (a
// boolean one bare or `{Bool}`, any other Text as usual), or on the escape list (checked as on
// any element); anything else is E0028.
func (mc *modCtx) customAttr(x *syntax.Element, el *elements.Element, a *syntax.HtmlAttr, m types.Type, env *env, eff effect) {
	c := mc.c
	if d := el.Attr(a.Name); d != nil {
		switch {
		case d.Boolean() && a.Value == nil:
		case d.Boolean():
			if _, isStr := a.Value.(*syntax.StrLit); isStr {
				c.errorf(diag.CustomElement, a.Span, "`<%s>` attribute `%s` is boolean: write `%s` or `%s={expr}`", x.Tag, a.Name, a.Name, a.Name)
				return
			}
			mc.check(a.Value, types.Bool, env, eff)
		case a.Value == nil:
			c.errorf(diag.TypeMismatch, a.Span, "attribute `%s` needs a value", a.Name)
		default:
			mc.textAttr(a, env, eff)
		}
		return
	}
	if elements.Escaped(a.Name) || (strings.HasPrefix(a.Name, "on") && len(a.Name) > 2) {
		mc.attribute(a, m, env, eff) // `onclick` gets its own message there
		return
	}
	has := "it declares none"
	if len(el.Attrs) > 0 {
		var names []string
		for _, d := range el.Attrs {
			names = append(names, "`"+d.Name+"`")
		}
		has = "it has " + strings.Join(names, ", ")
	}
	c.errorf(diag.CustomElement, a.Span, "`<%s>` has no attribute `%s`; %s", x.Tag, a.Name, has)
	if a.Value != nil {
		mc.infer(a.Value, env, eff)
	}
}

// checkSlots checks each `slot="name"` on x's child elements (through `if`, `for` and `match`
// in the markup) against the slots el declares.
func (mc *modCtx) checkSlots(x *syntax.Element, el *elements.Element) {
	c := mc.c
	var visit func(e syntax.Expr)
	visit = func(e syntax.Expr) {
		switch ch := e.(type) {
		case *syntax.Element:
			if ch.Ref.Kind == syntax.RefComponent || isUpperName(ch.Tag) {
				return
			}
			for _, a := range ch.Attrs {
				if a.Name != "slot" || a.Value == nil {
					continue
				}
				name, ok := literalText(a.Value)
				if !ok || name == "" || el.HasSlot(name) {
					continue
				}
				has := "it declares no slots"
				if named := el.NamedSlots(); len(named) > 0 {
					has = "it has `" + strings.Join(named, "`, `") + "`"
				} else if el.HasSlot("") {
					has = "it has only the default slot"
				}
				c.errorf(diag.CustomElement, a.Span, "`<%s>` has no slot `%s`; %s", x.Tag, name, has)
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

// rendered is what the renderer adds to built-in element x (Result.Rendered), "" for nothing.
//
// An `<al-button>` given `click`, `submit`, `type` or `href` gets a real control inside it,
// unless the author wrote one (a `<button>` or `<a>` child): "a" for `href`, else "button". A
// custom element is not focusable and a form does not submit through it, so without this the
// button would need JavaScript and could not be reached from the keyboard.
//
// An `<al-alert>` without `role` gets the live-region role its variant implies: "status" for
// info (the default) and success, "alert" for warning and destructive. A computed variant gets
// none: the author writes `role` then.
func rendered(x *syntax.Element) string {
	switch x.Tag {
	case "al-button":
		for _, ch := range x.Children {
			if e, ok := ch.(*syntax.Element); ok && (e.Tag == "button" || e.Tag == "a") {
				return ""
			}
		}
		if hasHTMLAttr(x, "href") {
			return "a"
		}
		for _, n := range []string{"click", "submit", "type"} {
			if hasHTMLAttr(x, n) {
				return "button"
			}
		}
	case "al-alert":
		if hasHTMLAttr(x, "role") {
			return ""
		}
		variant := "info"
		for _, a := range x.Attrs {
			if a.Name == "variant" {
				v, ok := literalText(a.Value)
				if !ok {
					return ""
				}
				variant = v
			}
		}
		switch variant {
		case "info", "success":
			return "status"
		case "warning", "destructive":
			return "alert"
		}
	}
	return ""
}

// hasHTMLAttr reports whether x has an attribute name.
func hasHTMLAttr(x *syntax.Element, name string) bool {
	for _, a := range x.Attrs {
		if a.Name == name {
			return true
		}
	}
	return false
}

// literalText is the text of a string literal without interpolation.
func literalText(e syntax.Expr) (string, bool) {
	s, ok := e.(*syntax.StrLit)
	if !ok {
		return "", false
	}
	text := ""
	for _, p := range s.Parts {
		if p.Expr != nil {
			return "", false
		}
		text += p.Text
	}
	return text, true
}
