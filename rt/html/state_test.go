package html

import (
	"strings"
	"testing"
)

func TestRenderWith_FillsValuesAndErrors(t *testing.T) {
	n := Nodes(
		Input(Control{Kind: "input", Form: "NewTodo", Name: "title", Label: "T", Required: true}),
		Input(Control{Kind: "errors", Form: "NewTodo", Name: "title"}),
		Input(Control{Kind: "errors", Form: "NewTodo"}),
		Input(Control{Kind: "input", Form: "NewTodo", Name: "pw", Secret: true, Required: true}),
	)
	st := &FormState{Form: "NewTodo", Values: map[string]string{"title": "a<b", "pw": "hunter2"},
		Errors: map[string][]string{"title": {"must not be empty"}}, FormError: "nope"}
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
	n := Input(Control{Kind: "input", Form: "Other", Name: "title", Value: "init"})
	st := &FormState{Form: "NewTodo", Values: map[string]string{"title": "x"}}
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
	n := Nodes(
		Input(Control{Kind: "input", Form: "F", Name: "done", Type: "checkbox"}),
		Input(Control{Kind: "input", Form: "F", Name: "off", Type: "checkbox", Value: "True"}),
		Input(Control{Kind: "select", Form: "F", Name: "c", Required: true, Value: "A", Options: []string{"A", "B"}}),
		Input(Control{Kind: "textarea", Form: "F", Name: "body", Value: "init"}),
	)
	st := &FormState{Form: "F", Values: map[string]string{"done": "True", "c": "B", "body": "</textarea><b>"}}
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
	n := Nodes(
		Input(Control{Kind: "errors", Form: "F", Name: "a"}),
		Input(Control{Kind: "input", Form: "F", Name: "pw", Secret: true, Value: "initial"}),
	)
	st := &FormState{Form: "F", Errors: map[string][]string{"a": {"x<y", "two"}}}
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
