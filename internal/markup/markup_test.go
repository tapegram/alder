package markup

import (
	"sort"
	"testing"
)

func TestHTML_Shape(t *testing.T) {
	if len(HTML.Tags) < 100 {
		t.Fatalf("%d tags", len(HTML.Tags))
	}
	if !sort.SliceIsSorted(HTML.Tags, func(i, j int) bool { return HTML.Tags[i].Name < HTML.Tags[j].Name }) {
		t.Fatal("tags not sorted")
	}
	for _, tg := range HTML.Tags {
		if tg.Description == "" {
			t.Errorf("<%s> has no description", tg.Name)
		}
		for _, a := range tg.Attrs {
			if a.Description == "" {
				t.Errorf("<%s %s> has no description", tg.Name, a.Name)
			}
		}
	}
	if HTML.Attr("input", "disabled") == nil || !HTML.Attr("input", "disabled").Boolean {
		t.Fatal("input disabled is not a boolean attribute")
	}
	if HTML.Attr("div", "class") == nil {
		t.Fatal("class is not global")
	}
	if HTML.Attr("a", "href") == nil || HTML.Attr("div", "href") != nil {
		t.Fatal("href per tag wrong")
	}
	if got := HTML.AttrsOf("label"); got[0].Name != "for" || len(got) != 1+len(HTML.Global) {
		t.Fatalf("AttrsOf(label) = %v", got)
	}
}
