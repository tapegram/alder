package check

import (
	"strings"
	"testing"
)

// The checker's tag and boolean-attribute sets, derived from the markup manifest, equal the lists
// they were before the manifest existed.
func TestManifestDerivedSets(t *testing.T) {
	wantTags := strings.Fields(`a abbr address article aside audio b bdi bdo blockquote body br button canvas caption cite code col colgroup data datalist dd del details dfn dialog div dl dt em embed fieldset figcaption figure footer form h1 h2 h3 h4 h5 h6 head header hgroup hr html i iframe img input ins kbd label legend li link main map mark menu meta meter nav noscript object ol optgroup option output p picture pre progress q rp rt ruby s samp script section select slot small source span strong style sub summary sup table tbody td template textarea tfoot th thead time title tr track u ul var video wbr`)
	wantBool := strings.Fields(`allowfullscreen alpha async autofocus autoplay checked controls default defer disabled formnovalidate hidden inert ismap itemscope loop multiple muted nomodule novalidate open playsinline readonly required reversed selected shadowrootclonable shadowrootdelegatesfocus shadowrootserializable`)
	same := func(what string, got map[string]bool, want []string) {
		w := map[string]bool{}
		for _, n := range want {
			w[n] = true
			if !got[n] {
				t.Errorf("%s: missing %q", what, n)
			}
		}
		for n := range got {
			if !w[n] {
				t.Errorf("%s: extra %q", what, n)
			}
		}
	}
	same("htmlTags", htmlTags, wantTags)
	same("boolAttrs", boolAttrs, wantBool)
}
