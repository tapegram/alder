package gen

import (
	"strings"
	"testing"
)

// A test helper (#4) is emitted in a test build and omitted from a production build, where
// nothing may reference it; an ordinary value of the same module is in both.
func TestGenTestHelpers(t *testing.T) {
	dir := "../../testdata/golden/gen_helpers"
	const file = "Helpers_alder.go"
	for _, c := range []struct {
		name  string
		opts  Options
		helps bool
	}{{"test build", Options{Tests: true}, true}, {"production build", Options{}, false}} {
		t.Run(c.name, func(t *testing.T) {
			src := string(emitOpts(t, dir, c.opts)[file])
			for _, h := range []string{"fetchText", "at"} {
				if got := strings.Contains(src, "func "+GoName("Helpers", h)+"("); got != c.helps {
					t.Errorf("%s: helper %s emitted = %v, want %v\n%s", file, h, got, c.helps, src)
				}
			}
			if !strings.Contains(src, GoName("Helpers", "final")) {
				t.Errorf("%s lacks the ordinary value final:\n%s", file, src)
			}
		})
	}
}
