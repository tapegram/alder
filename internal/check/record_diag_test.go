package check

import (
	"strings"
	"testing"
)

// Each record mismatch a literal can hit says what differs (#3): E0003 with a recordDiff suffix.
func TestRecordLitInference_Diagnostics(t *testing.T) {
	cases := []struct{ name, body, want string }{
		{"field type", `x : Float
x = {
  let wide = { median: 1, sigma: 2.0, perDoubling: 0.5 }
  reportTime wide 1.0
}
`, "expected Curve, got { median: Int, sigma: Float, perDoubling: Float }; field `median` is Int, Curve's is Float"},
		{"missing", `x : Float
x = {
  let wide = { median: 1.0, sigma: 2.0 }
  reportTime wide 1.0
}
`, "; the record is missing Curve's `perDoubling`"},
		{"extra", `x : Float
x = {
  let wide = { median: 1.0, sigma: 2.0, perDoubling: 0.5, extra: 1, alpha: 2 }
  reportTime wide 1.0
}
`, "; `alpha`, `extra` are not fields of Curve"},
		{"missing and extra", `x : Float
x = {
  let wide = { median: 1.0, sigma: 2.0, perDubling: 0.5 }
  reportTime wide 1.0
}
`, "; the record is missing Curve's `perDoubling`; `perDubling` is not a field of Curve"},
		{"structural", `spread : { median: Float, sigma: Float, perDoubling: Float } -> Float
spread c = c.sigma

x : Float
x = {
  let wide = { median: 1.0, sigma: 2.0, perDoubling: 0.5 }
  spread wide + reportTime wide 1.0
}
`, "; the record has Curve's fields: declare it as Curve where it is made (a signature's inline record type, or a let that already met a structural type)"},
		{"compare", `ok : Summary -> Bool
ok s = s == { t0: 1.0, label: "a" }
`, "cannot compare `Summary` with `{ t0: Float, label: Text }`: `==` needs both sides to have the same type; field `t0` is Float, Summary's is Int"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := errorTexts(t, curveModule(c.body))
			if len(got) != 1 || !strings.Contains(got[0], c.want) || strings.Contains(got[0], "annotate the let") {
				t.Fatalf("got %q, want one E0003 containing %q", got, c.want)
			}
		})
	}
}
