package html

import (
	"strings"
	"testing"

	"alder/rt"
)

// newTodo is the message the tests' forms post; posted is the state of a re-render after it.
var newTodo = rt.MsgRef{Module: "Todos", Msg: "Add"}

func posted(form string, m rt.MsgRef, st FormState) *FormState {
	st.Form, st.Msg = form, m.Key()
	return &st
}

func TestRenderWith_FillsValuesAndErrors(t *testing.T) {
	n := FormTo(newTodo, nil,
		Input(Control{Kind: "input", Form: "NewTodo", Name: "title", Label: "T", Required: true}),
		Input(Control{Kind: "errors", Form: "NewTodo", Name: "title"}),
		Input(Control{Kind: "errors", Form: "NewTodo"}),
		Input(Control{Kind: "input", Form: "NewTodo", Name: "pw", Secret: true, Required: true}),
	)
	st := posted("NewTodo", newTodo, FormState{Values: map[string]string{"title": "a<b", "pw": "hunter2"},
		Errors: map[string][]string{"title": {"must not be empty"}}, FormError: "nope"})
	got := RenderWith(n, st)
	for _, want := range []string{
		`<input name="title" type="text" value="a&lt;b" required />`,
		`<ul class="errors" data-errors="title"><li>must not be empty</li></ul>`,
		`<ul class="errors" data-errors="form"><li>nope</li></ul>`,
		`<input name="pw" type="password" required />`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %s in %s", want, got)
		}
	}
	if strings.Contains(got, "hunter2") {
		t.Fatal("secret echoed")
	}
}

func TestRenderWith_OtherFormUntouched(t *testing.T) {
	n := FormTo(newTodo, nil, Input(Control{Kind: "input", Form: "Other", Name: "title", Value: "init"}))
	st := posted("NewTodo", newTodo, FormState{Values: map[string]string{"title": "x"}})
	if got := RenderWith(n, st); !strings.Contains(got, `value="init"`) {
		t.Fatal(got)
	}
}

func TestRender_WithoutStateUnchanged(t *testing.T) {
	n := El("div", []Attr{A("class", "x")},
		Text("a<b"), Raw("<i>r</i>"), None(),
		Form("/_m/M/Go", nil,
			Input(Control{Kind: "input", Form: "F", Name: "title", Label: "Title", Required: true, Value: "v&"}),
			Input(Control{Kind: "input", Form: "F", Name: "done", Type: "checkbox", Value: "True"}),
			Input(Control{Kind: "textarea", Form: "F", Name: "body", Value: "x<y"}),
			Input(Control{Kind: "select", Form: "F", Name: "c", Value: "B", Options: []string{"A", "B"}}),
			Input(Control{Kind: "errors", Form: "F", Name: "title"}),
			Input(Control{Kind: "errors", Form: "F"}),
		))
	if a, b := Render(n), RenderWith(n, nil); a != b {
		t.Fatalf("Render %s\nRenderWith(nil) %s", a, b)
	}
	if a, b := Render(n), RenderWith(n, &FormState{Form: "Other", Values: map[string]string{"title": "zz"}, FormError: "e"}); a != b {
		t.Fatalf("other-form state changed output:\n%s\n%s", a, b)
	}
}

func TestRenderWith_CheckboxSelectTextarea(t *testing.T) {
	n := FormTo(newTodo, nil,
		Input(Control{Kind: "input", Form: "F", Name: "done", Type: "checkbox"}),
		Input(Control{Kind: "input", Form: "F", Name: "off", Type: "checkbox", Value: "True"}),
		Input(Control{Kind: "select", Form: "F", Name: "c", Required: true, Value: "A", Options: []string{"A", "B"}}),
		Input(Control{Kind: "textarea", Form: "F", Name: "body", Value: "init"}),
	)
	st := posted("F", newTodo, FormState{Values: map[string]string{"done": "True", "c": "B", "body": "</textarea><b>"}})
	got := RenderWith(n, st)
	for _, want := range []string{
		`<input name="done" type="checkbox" value="True" checked />`,
		`<input name="off" type="checkbox" value="True" />`, // unchecked boxes are not submitted
		`<option value="A">A</option><option value="B" selected>B</option>`,
		`<textarea name="body">&lt;/textarea&gt;&lt;b&gt;</textarea>`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %s in %s", want, got)
		}
	}
}

func TestRenderWith_ErrorsEscapedAndSecretNeverEchoed(t *testing.T) {
	n := FormTo(newTodo, nil,
		Input(Control{Kind: "errors", Form: "F", Name: "a"}),
		Input(Control{Kind: "input", Form: "F", Name: "pw", Secret: true, Value: "initial"}),
	)
	st := posted("F", newTodo, FormState{Errors: map[string][]string{"a": {"x<y", "two"}}})
	got := RenderWith(n, st)
	if !strings.Contains(got, `<ul class="errors" data-errors="a"><li>x&lt;y</li><li>two</li></ul>`) {
		t.Fatal(got)
	}
	if strings.Contains(got, "initial") || strings.Contains(Render(n), "initial") {
		t.Fatal("secret value rendered")
	}
	if !strings.Contains(got, `<input name="pw" type="password" />`) {
		t.Fatal(got)
	}
}

// Per-row forms of one form type (I1, the reviewer's adv3): only the form posting the very
// message the state is for — same module, message and parameters — shows the submitted values
// and errors; the other rows render their initial values. A state without Form or Msg (a plain
// page render) matches nothing.
func TestRenderWith_StateOnlyOnThePostingForm(t *testing.T) {
	row := func(id string) Node {
		return FormTo(rt.MsgRef{Module: "Items", Msg: "Rename", Params: []rt.KV{{Key: "id", Value: id}}}, nil,
			Input(Control{Kind: "input", Form: "Items.Name", Name: "name", Value: "init-" + id}),
			Input(Control{Kind: "errors", Form: "Items.Name", Name: "name"}),
			Input(Control{Kind: "errors", Form: "Items.Name"}))
	}
	page := El("ul", nil, row("1"), row("2"), row("3"))
	q := map[string][]string{"id": {"2"}, "_p.filter": {"All"}} // as the route sees the request
	st := &FormState{Form: "Items.Name", Msg: rt.MsgKey("/_m/Items/Rename", q), Values: map[string]string{"name": "typed"},
		Errors: map[string][]string{"name": {"must not be empty"}}, FormError: "taken"}
	got := RenderWith(page, st)
	if strings.Count(got, `value="typed"`) != 1 || strings.Count(got, "<li>must not be empty</li>") != 1 || strings.Count(got, "<li>taken</li>") != 1 {
		t.Fatalf("the state shows on other rows too:\n%s", got)
	}
	for _, init := range []string{`value="init-1"`, `value="init-3"`} {
		if !strings.Contains(got, init) {
			t.Fatalf("missing %s:\n%s", init, got)
		}
	}
	if i, j := strings.Index(got, "Rename?id=2"), strings.Index(got, `value="typed"`); i < 0 || j < i || strings.Contains(got[i:j], "Rename?id=3") {
		t.Fatalf("the state is not on row 2:\n%s", got)
	}
	// no Msg, or no Form: nothing matches
	for _, st := range []*FormState{
		{Form: "Items.Name", Values: map[string]string{"name": "typed"}},
		{Msg: rt.MsgKey("/_m/Items/Rename", q), Values: map[string]string{"name": "typed"}},
	} {
		if got := RenderWith(page, st); strings.Contains(got, "typed") {
			t.Fatalf("state %+v matched:\n%s", st, got)
		}
	}
}

// A click button works without JavaScript (I3): outside a form it is wrapped in a form of its
// own posting the message (Datastar posts it on submit); inside a form it is a submit button
// with its own formaction, which Datastar posts on click, preventing the form's submission. A
// `type` the view wrote is replaced. Other elements keep data-on:click.
func TestRender_ClickButtons(t *testing.T) {
	m := rt.MsgRef{Module: "Todos", Msg: "Toggle", Params: []rt.KV{{Key: "id", Value: "a b"}}}
	st := &FormState{PageParams: []rt.KV{{Key: "filter", Value: "Active"}}}
	u := "/_m/Todos/Toggle?id=a+b&amp;_p.filter=Active"
	got := RenderWith(El("div", nil, El("button", []Attr{A("type", "button"), MsgAttr("data-on:click", m), A("class", "x")}, Text("Done"))), st)
	want := `<div><form method="post" action="` + u + `" class="_alder-click" data-on:submit="@post(&#39;` + u + `&#39;, {contentType: &#39;form&#39;})">` +
		`<button type="submit" class="x">Done</button></form></div>`
	if got != want {
		t.Fatalf("outside a form:\n got %s\nwant %s", got, want)
	}
	got = RenderWith(FormTo(newTodo, nil, El("button", []Attr{MsgAttr("data-on:click", m)}, Text("Delete")), El("button", nil, Text("Save"))), st)
	want = `<button type="submit" formaction="` + u + `" formmethod="post" formnovalidate data-on:click__prevent="@post(&#39;` + u + `&#39;)">Delete</button><button>Save</button></form>`
	if !strings.HasSuffix(got, want) || strings.Count(got, "<form") != 1 {
		t.Fatalf("inside a form:\n got %s\nwant …%s", got, want)
	}
	got = RenderWith(El("li", []Attr{MsgAttr("data-on:click", m)}, Text("row")), st)
	if got != `<li data-on:click="@post(&#39;`+u+`&#39;)">row</li>` {
		t.Fatalf("a non-button: %s", got)
	}
}
