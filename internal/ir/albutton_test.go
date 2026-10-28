package ir

import (
	"strings"
	"testing"
)

// An `<al-button href>` renders an `<a>`, and the attributes that describe a link (check.AnchorAttrs)
// move onto it, next to `href`; the rest stay on the element. A disabled one renders an `<a
// aria-disabled>` without `href`, which takes them too. testdata/corpus/al-button-anchor-attrs-ok
// holds the buttons.
func TestIR_AlButtonAnchorAttrs(t *testing.T) {
	d := spaces(Dump(lowerDir(t, "../../testdata/corpus/al-button-anchor-attrs-ok")))
	for _, want := range []string{
		`(elem al-button (attr id text "edit") (attr variant text "outline") (elem a (attr href href (page-ref Index (record Params))) (attr target text "_blank") (attr rel text "noopener") (attr data-alder-reload text "true") (attr download text "a.txt") (attr hreflang text "en") (attr referrerpolicy text "no-referrer") (attr ping text "/p") (htext "Edit")))`,
		`(elem al-button (attr id text "off") (attr disabled bool True) (elem a (attr aria-disabled text "true") (attr target text "_blank") (attr data-alder-reload text "true") (htext "Off")))`,
		`(elem al-button (attr id text "file") (elem a (attr href href (page-ref Export (record Export.Params))) (htext "Export")))`,
		`(elem al-button (attr id text "own") (elem a (attr href href (page-ref Index (record Params))) (attr target text "_blank") (htext "Own")))`,
	} {
		if !strings.Contains(d, want) {
			t.Errorf("dump lacks\n%s\nin\n%s", want, d)
		}
	}
}
