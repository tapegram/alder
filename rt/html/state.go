package html

// FormState is what a re-render after a failed form decode or a Reject carries. Form is the
// Alder form's name ("NewTodo"); Values are the submitted raw strings by field name (an
// unchecked checkbox is simply absent, as browsers do not submit it); Errors are the messages by
// field name; FormError is the form-level message (the Reject text, or ""). rt/web must never
// put a Secret field's value in Values; the renderer ignores it anyway.
//
// A FormState applies only to controls whose Control.Form equals its Form: every other form on
// the page renders its initial values and empty error lists, exactly as Render does.
type FormState struct {
	Form      string
	Values    map[string]string
	Errors    map[string][]string
	FormError string
}

// Control is a form control bound to a form field: what `<Input>`, `<Textarea>`, `<Select>` and
// `<Errors>` render. The generated code derives it from the field's type and sets Form to the
// Alder form's name; the request state comes from RenderWith's FormState.
type Control struct {
	Kind     string   // "input", "textarea", "select" or "errors"
	Form     string   // the Alder form's name; matched against FormState.Form
	Name     string   // the field; "" for `<Errors form />`
	Type     string   // an input's type: "text", "number", "date" or "checkbox"
	Label    string   // "" for none
	Required bool     // the field is not a Maybe (never set for a checkbox)
	Value    string   // the initial value as text ("True" checks a checkbox), "" for none
	Options  []string // a select's options: the union's constructor names
	Secret   bool     // a Text.Secret field: type="password", and no value is ever rendered
}

// Input is a deferred form control: it is built when rendered. Without a matching FormState it
// renders a <label> holding the label text and the control with its initial Value, or for
// "errors" an empty <ul class="errors" data-errors="field|form">. With a FormState whose Form
// equals c.Form, the control's value is the submitted one (st.Values[Name]; for a checkbox,
// "True" checks it) and the error list holds one <li> per message (st.Errors[Name], or
// st.FormError for `<Errors form />`). Submitted values and messages are escaped like any text.
func Input(c Control) Node { return Node{kind: kCtrl, control: &c} }

func (c *Control) build(st *FormState) Node {
	active := st != nil && st.Form == c.Form
	if c.Kind == "errors" {
		scope := "form"
		var msgs []string
		if c.Name != "" {
			scope = c.Name
			if active {
				msgs = st.Errors[c.Name]
			}
		} else if active && st.FormError != "" {
			msgs = []string{st.FormError}
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
		if !c.Required {
			opts = append(opts, El("option", []Attr{A("value", "")}))
		}
		for _, o := range c.Options {
			opts = append(opts, El("option", []Attr{A("value", o), B("selected", o == value)}, Text(o)))
		}
		control = El("select", []Attr{A("name", c.Name), B("required", c.Required)}, opts...)
	default:
		typ := c.Type
		if c.Secret {
			typ = "password"
		} else if typ == "" {
			typ = "text"
		}
		attrs := []Attr{A("name", c.Name), A("type", typ)}
		if typ == "checkbox" {
			attrs = append(attrs, A("value", "True"), B("checked", value == "True"))
		} else {
			attrs = append(attrs, Attr{Name: "value", Value: value, Omit: value == ""}, B("required", c.Required))
		}
		control = El("input", attrs)
	}
	if c.Label == "" {
		return El("label", nil, control)
	}
	return El("label", nil, Text(c.Label), control)
}
