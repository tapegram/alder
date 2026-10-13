package html

import (
	"strings"

	"alder/rt"
)

// FormState is what a re-render after a failed form decode or a Reject carries. Form is the
// Alder form's name ("Todos.Index.NewTodo") and Msg the key of the message that posted it
// (rt.MsgKey: its path and its own parameters, as sent); Values are the submitted raw strings by
// field name (an unchecked checkbox is simply absent, as browsers do not submit it); Errors are
// the messages by field name; FormError is the form-level message (the Reject text, or "").
// rt/web must never put a Secret field's value in Values; the renderer ignores it anyway.
//
// A FormState applies only to the controls of form Form inside the <form> that posts message
// Msg (FormTo, whose MsgRef.Key equals it): every other form on the page — another form type, or
// a per-row form of the same type posting another row's message — renders its initial values
// and empty error lists, exactly as Render does. A state without Form or Msg matches no control.
type FormState struct {
	Form      string
	Msg       string
	Values    map[string]string
	Errors    map[string][]string
	FormError string
	// PageParams are the parameters of the page being rendered, as text in declaration order
	// (a parameter at its default left out): every message URL on the page carries them
	// (MsgAttr), so its route knows the page. A state with only PageParams (Form "") is what a
	// plain page render passes; it matches no control.
	PageParams []rt.KV
}

// Control is a form control bound to a form field: what `<Input>`, `<Textarea>`, `<Select>` and
// `<Errors>` render. The generated code derives it from the field's type and sets Form to the
// Alder form's name; the request state comes from RenderWith's FormState.
type Control struct {
	Kind     string   // "input", "textarea", "select" or "errors"
	Form     string   // the Alder form's name; matched against FormState.Form
	Name     string   // the field; "" for `<Errors form />`
	Type     string   // an input's type: "text", "email", "url", "number", "date" or "checkbox"
	Step     string   // a number input's step: "any" for a Float, "" (the browser's 1) for an Int
	Label    string   // "" for none
	Required bool     // the decoder refuses a blank value: not a Maybe, no default, not a checkbox, not plain Text
	Value    string   // the initial value as text ("True" checks a checkbox), "" for none
	Options  []string // a select's options: the union's constructor names
	// Choices are a select's options when it has `options={…}` (a field of Text or a brand of
	// it, such as a row's id): each one's value and label. Blank says whether the list starts
	// with an empty option: for a field that may be left empty (a Maybe, a field with a
	// default); otherwise the first choice is selected until one is chosen.
	Choices []Choice
	Blank   bool
	Secret  bool // a Text.Secret field: type="password", and no value is ever rendered
	// Wrap is the design system's input element written around the control ("al-input",
	// "al-textarea", "al-select", "al-checkbox"), "" for none; WrapAttrs are its attributes as
	// written (see Input).
	Wrap      string
	WrapAttrs []Attr
}

// Choice is one option of a select with `options={…}`: the value it submits and the label it
// shows.
type Choice struct{ Value, Label string }

// Choices are the options of a select from the list xs its `options={…}` gives, each made by f
// (the generated code's conversion of one `{ value, label }` record).
func Choices[T any](xs []T, f func(T) Choice) []Choice {
	out := make([]Choice, 0, len(xs))
	for _, x := range xs {
		out = append(out, f(x))
	}
	return out
}

// Input is a deferred form control: it is built when rendered. Without a matching FormState it
// renders a <label> holding the label text and the control with its initial Value, or for
// "errors" an empty <ul class="errors" data-errors="field|form">. With a FormState that matches
// it (FormState: its Form equals c.Form, its Msg the enclosing form's), the control's value is the submitted one (st.Values[Name]; for a checkbox,
// "True" checks it) and the error list holds one <li> per message (st.Errors[Name], or
// st.FormError for `<Errors form />`, then the errors of every field that has no `<Errors field>`
// in that form, prefixed with the field's name). Submitted values and messages are escaped like
// any text.
//
// A control with a Wrap renders inside that element, followed by the field's own error list
// (as `<Errors field>` renders it, so the field's errors are not repeated on the form's list):
//
//	<al-input label="Title" placeholder="…" help="…" invalid><label>Title<input name="title" … /></label><small class="help" id="help-title">…</small><ul class="errors" data-errors="title"><li>…</li></ul></al-input>
//
// The element carries `label` (the label text, when there is one), then WrapAttrs in order,
// then `invalid` exactly when the field has errors (or `invalid` is written and true). The
// control also gets the element's `placeholder` (not on a select or checkbox) and `disabled`,
// so they work without the design system's stylesheet, and `aria-invalid="true"` when the
// element is invalid. A `help` text renders as `<small class="help" id="help-<field>">` after
// the label (the stylesheet then does not draw the attribute again). When the field has errors,
// the list gets `id="errors-<field>"` (only then: two per-row forms with the same field would
// otherwise repeat the id). The control's `aria-describedby` names the help, then the errors,
// whichever are present, so a screen reader reads them with the control.
func Input(c Control) Node { return Node{kind: kCtrl, control: &c} }

func (c *Control) build(st *FormState, active bool, orphan []string) Node {
	if c.Kind == "errors" {
		scope := "form"
		var msgs []string
		if c.Name != "" {
			scope = c.Name
			if active {
				msgs = st.Errors[c.Name]
			}
		} else if active {
			if st.FormError != "" {
				msgs = []string{st.FormError}
			}
			msgs = append(msgs, orphan...)
		}
		items := make([]Node, len(msgs))
		for i, m := range msgs {
			items[i] = El("li", nil, Text(m))
		}
		return El("ul", []Attr{A("class", "errors"), A("data-errors", scope)}, items...)
	}
	value := c.Value
	if active {
		value = st.Values[c.Name]
	}
	if c.Secret {
		value = ""
	}
	var control Node
	switch c.Kind {
	case "textarea":
		control = El("textarea", []Attr{A("name", c.Name), B("required", c.Required)}, Text(value))
	case "select":
		var opts []Node
		if (c.Choices == nil && !c.Required) || (c.Choices != nil && c.Blank) {
			opts = append(opts, El("option", []Attr{A("value", "")}))
		}
		for _, o := range c.Options {
			opts = append(opts, El("option", []Attr{A("value", o), B("selected", o == value)}, Text(o)))
		}
		for _, o := range c.Choices {
			opts = append(opts, El("option", []Attr{A("value", o.Value), B("selected", o.Value == value)}, Text(o.Label)))
		}
		control = El("select", []Attr{A("name", c.Name), B("required", c.Required)}, opts...)
	default:
		typ := c.Type
		if c.Secret {
			typ = "password"
		} else if typ == "" {
			typ = "text"
		}
		attrs := []Attr{A("name", c.Name), A("type", typ), Attr{Name: "step", Value: c.Step, Omit: c.Step == ""}}
		if typ == "checkbox" {
			attrs = append(attrs, A("value", "True"), B("checked", value == "True"))
		} else {
			attrs = append(attrs, Attr{Name: "value", Value: value, Omit: value == ""}, B("required", c.Required))
		}
		control = El("input", attrs)
	}
	if c.Wrap == "" {
		return labelled(c.Label, control)
	}
	errs := (&Control{Kind: "errors", Form: c.Form, Name: c.Name}).build(st, active, nil)
	attrs := []Attr{{Name: "label", Value: c.Label, Omit: c.Label == ""}}
	hasErrs := len(errs.children) > 0
	invalid := hasErrs
	var described []string
	var help []Node
	for _, a := range c.WrapAttrs {
		if a.Name == "invalid" {
			invalid = invalid || !a.Omit
			continue
		}
		if a.Name == "help" && !a.Omit && a.Value != "" { // read with the control, not only seen
			id := "help-" + c.Name
			help = append(help, El("small", []Attr{A("class", "help"), A("id", id)}, Text(a.Value)))
			described = append(described, id)
		}
		attrs = append(attrs, a)
	}
	attrs = append(attrs, B("invalid", invalid))
	control.attrs = append(control.attrs, c.forwarded()...)
	control.attrs = append(control.attrs, Attr{Name: "aria-invalid", Value: "true", Omit: !invalid})
	if hasErrs { // the id only when there are errors: per-row forms share field names
		id := "errors-" + c.Name
		errs.attrs = append(errs.attrs, A("id", id))
		described = append(described, id)
	}
	if len(described) > 0 {
		control.attrs = append(control.attrs, A("aria-describedby", strings.Join(described, " ")))
	}
	children := append([]Node{labelled(c.Label, control)}, help...)
	return El(c.Wrap, attrs, append(children, errs)...)
}

// labelled is the <label> holding text (when there is any) and control.
func labelled(text string, control Node) Node {
	if text == "" {
		return El("label", nil, control)
	}
	return El("label", nil, Text(text), control)
}

// forwarded are the wrapping element's attributes its native control takes too: `disabled`,
// and `placeholder` on a text input or textarea.
func (c *Control) forwarded() []Attr {
	var out []Attr
	for _, a := range c.WrapAttrs {
		switch {
		case a.Name == "disabled":
			out = append(out, B("disabled", !a.Omit))
		case a.Name == "placeholder" && c.Kind != "select" && c.Type != "checkbox":
			out = append(out, a)
		}
	}
	return out
}
