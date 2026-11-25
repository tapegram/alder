package check

import (
	"strings"
	"testing"
)

// svgPage is a module whose function chart is body.
func svgPage(body string) string {
	return "module Chart\n\nchart : Int -> Html\nchart n =\n  " + body + "\n"
}

// SVG elements are markup a view writes (#306): the tags and their attributes check, an unknown
// element inside <svg> is still unknown, and an attribute an SVG element does not take is an error.
func TestSVG(t *testing.T) {
	cases := []struct{ name, body, want string }{
		{"chart", `<svg viewBox="0 0 10 10" role="img"><title>t</title><polyline points="0,0 5,5" fill="none" stroke="currentColor" stroke-width="2" /><line x1="0" y1="0" x2="10" y2="0" /></svg>`, ""},
		{"every tag and attribute", `<svg viewBox="0 0 1 1" preserveAspectRatio="none" xmlns="http://www.w3.org/2000/svg" width="10" height="10" aria-hidden="true" data-k={Text.fromInt n}>
    <desc>d</desc>
    <g transform="translate(1 1)" opacity="0.5" class="axis">
      <path d="M0 0L1 1" stroke-dasharray="2 2" stroke-linecap="round" stroke-linejoin="round" stroke-opacity="0.5" />
      <polygon points="0,0 1,1 1,0" fill-opacity="0.2" />
      <rect x="0" y="0" width="1" height="1" rx="1" ry="1" />
      <circle cx="0" cy="0" r="1" />
      <ellipse cx="0" cy="0" rx="1" ry="1" />
      <text x="0" y="1" text-anchor="end" dominant-baseline="middle" font-size="12">a<tspan x="0" y="2">b</tspan></text>
    </g>
  </svg>`, ""},
		{"unknown element", `<svg><bogus /></svg>`, "E0002 unknown element `<bogus>`"},
		{"unknown attribute", `<svg><rect href="x" /></svg>`, "E0002 `<rect>` takes no attribute `href`"},
		{"an HTML attribute", `<svg><circle alt="x" /></svg>`, "E0002 `<circle>` takes no attribute `alt`"},
		{"an HTML element in an svg", `<svg><div>x</div></svg>`, "E0002 `<div>` cannot go inside `<svg>`: an SVG element holds SVG elements (and `<title>`)"},
		{"an HTML element in svg text", `<svg><text x="0" y="0">a<span>b</span></text></svg>`, "E0002 `<span>` cannot go inside `<text>`: an SVG element holds SVG elements (and `<title>`)"},
		{"an HTML element in a loop in an svg", `<svg>{for p in [1] -> <p>x</p>}</svg>`, "E0002 `<p>` cannot go inside `<svg>`: an SVG element holds SVG elements (and `<title>`)"},
		{"a custom element in an svg", `<svg><al-badge>x</al-badge></svg>`, "E0002 `<al-badge>` cannot go inside `<svg>`: an SVG element holds SVG elements (and `<title>`)"},
		{"an attribute in the wrong case", `<svg viewbox="0 0 1 1" />`, "E0002 `<svg>` takes no attribute `viewbox`; did you mean `viewBox`?"},
		{"a typed value", `<svg><circle r={n} /></svg>`, "E0003 expected Text, got Int"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var got []string
			for _, d := range checkModule(t, svgPage(c.body)) {
				got = append(got, d.Code+" "+d.Message)
			}
			if c.want == "" && len(got) > 0 || c.want != "" && (len(got) != 1 || got[0] != c.want) {
				t.Fatalf("got %q, want %q", strings.Join(got, "\n"), c.want)
			}
		})
	}
}
