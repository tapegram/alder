package book

import (
	"os"
	"strings"
	"testing"

	"alder/internal/elements"
	"alder/rt"
	"alder/rt/html"
)

// Chapter 5, "The input elements", quotes the markup of
// `<al-input field="title" label="Title" placeholder="A title" help="Short is good" />` (title
// a Text.NonEmpty) in an `html` block, as the page renders it and after a failed submission,
// and Appendix C does the same for its Settings page's name field. An `alder eval=` block
// cannot show the second (it needs a posted form), so this renders both with the runtime: each
// Control below is the one gen emits for that element (the gen_elements golden holds the
// same), rendered by rt/html as a page is, without and with a FormState carrying the field's
// error. The book's two lines must be those renders.
func TestBook_WrappedInputMarkup(t *testing.T) {
	for _, c := range []struct {
		file, field, label, placeholder, help string
	}{
		{"05-html.md", "title", "Title", "A title", "Short is good"},
		{"C-design-system.md", "name", "Name", "Ada Lovelace", "As it appears on your posts"},
	} {
		md, err := os.ReadFile(c.file)
		if err != nil {
			t.Fatal(err)
		}
		prefix := `<al-input label="` + c.label + `" placeholder="` + c.placeholder + `" help="` + c.help + `"`
		var quoted []string
		for _, l := range strings.Split(string(md), "\n") {
			if strings.HasPrefix(l, prefix) {
				quoted = append(quoted, l)
			}
		}
		if len(quoted) != 2 {
			t.Fatalf("%s should quote the al-input markup on two lines, found %d", c.file, len(quoted))
		}
		add := rt.MsgRef{Module: "Index", Msg: "Add"}
		control := html.Input(html.Control{Kind: "input", Form: "Index.F", Name: c.field, Type: "text", Label: c.label, Required: true,
			Wrap: "al-input", WrapAttrs: []html.Attr{html.A("placeholder", c.placeholder), html.A("help", c.help)}})
		page := html.FormTo(add, nil, control)
		inner := func(form string) string { // the control, without its form
			i := strings.Index(form, "<al-input")
			j := strings.LastIndex(form, "</al-input>")
			if i < 0 || j < 0 {
				t.Fatalf("no al-input in %s", form)
			}
			return form[i : j+len("</al-input>")]
		}
		if got := inner(html.Render(page)); got != quoted[0] {
			t.Errorf("%s: the first line is not the render:\n got %s\nbook %s", c.file, got, quoted[0])
		}
		st := &html.FormState{Form: "Index.F", Msg: add.Key(), Values: map[string]string{c.field: ""},
			Errors: map[string][]string{c.field: {"must not be empty"}}}
		if got := inner(html.RenderWith(page, st)); got != quoted[1] {
			t.Errorf("%s: the second line is not the failed render:\n got %s\nbook %s", c.file, got, quoted[1])
		}
	}
}

// Chapter 5's third-party example: its `json` block is a manifest the checker reads, declaring
// `wa-button` with the attributes and slots the prose names.
func TestChapter5_ThirdPartyManifest(t *testing.T) {
	md, err := os.ReadFile("05-html.md")
	if err != nil {
		t.Fatal(err)
	}
	_, rest, ok := strings.Cut(string(md), "to use Web Awesome's `<wa-button>`")
	if !ok {
		t.Fatal("chapter 5 lost its Web Awesome example")
	}
	_, rest, ok = strings.Cut(rest, "```json\n")
	block, _, ok2 := strings.Cut(rest, "```\n")
	if !ok || !ok2 {
		t.Fatal("no json block after the Web Awesome example")
	}
	m, err := elements.Parse([]byte(block))
	if err != nil {
		t.Fatal(err)
	}
	el := m.Lookup("wa-button")
	if el == nil || el.Attr("variant") == nil || !strings.Contains(strings.Join(el.Attr("variant").Values(), ","), "brand") ||
		el.Attr("size") == nil || !el.HasSlot("start") || el.HasSlot("icon") {
		t.Fatalf("the manifest does not declare wa-button as the prose says: %+v", el)
	}
}
