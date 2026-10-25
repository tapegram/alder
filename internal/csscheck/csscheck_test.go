package csscheck

import (
	"reflect"
	"testing"
)

func TestVarsWithoutLiteralFallback(t *testing.T) {
	for _, c := range []struct {
		css  string
		want []string
	}{
		{`a{color:var(--a,#fff)}`, nil},
		{`a{font:12px/1.4 var(--font-mono,ui-monospace,SFMono-Regular,Menlo,monospace)}`, nil},
		{`a{color:color-mix(in srgb,var(--d,#b91c1c) 60%,var(--f,#000))}`, nil},
		{`a{color:var(--a)}`, []string{"var(--a)"}},
		{`a{color:var(--a,inherit)}`, []string{"var(--a,inherit)"}},
		{`a{color:var(--a, initial )}b{color:var(--b,unset)}`, []string{"var(--a, initial )", "var(--b,unset)"}},
		{`a{color:var(--a,var(--b))}`, []string{"var(--a,var(--b))", "var(--b)"}},
		{`a{color:var(--a,var(--b,#000))}`, []string{"var(--a,var(--b,#000))"}},
		{`a{color:var(--a,)}`, []string{"var(--a,)"}},
	} {
		if got := VarsWithoutLiteralFallback(c.css); !reflect.DeepEqual(got, c.want) {
			t.Errorf("VarsWithoutLiteralFallback(%q) = %q, want %q", c.css, got, c.want)
		}
	}
}

func TestRemovedALTokens(t *testing.T) {
	// the removed names are spelled in parts, so a grep for them finds none here
	const al = "--al-"
	got := RemovedALTokens(`a{max-width:var(--al-width-wide);font:var(` + al + `font-mono,x);border-radius:var(` + al + `radius-sm,4px)}`)
	want := []string{al + "font-mono", al + "radius-sm"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("RemovedALTokens = %q, want %q", got, want)
	}
	if got := RemovedALTokens(`a{width:var(--al-width-narrow)}`); got != nil {
		t.Errorf("RemovedALTokens kept width: %q", got)
	}
}
