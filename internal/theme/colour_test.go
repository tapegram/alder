package theme

import (
	"math"
	"testing"
)

func TestParseColour(t *testing.T) {
	cases := []struct {
		in         string
		r, g, b, a float64
		tol        float64
	}{
		{"#fff", 1, 1, 1, 1, 0.002},
		{"#000", 0, 0, 0, 1, 0.002},
		{"#ff000080", 1, 0, 0, 0.502, 0.002},
		{"#abcd", 0.667, 0.733, 0.8, 0.867, 0.002},
		{"rgb(255 0 0)", 1, 0, 0, 1, 0.002},
		{"rgba(255, 0, 0, 0.5)", 1, 0, 0, 0.5, 0.002},
		{"rgb(100% 0% 0% / 50%)", 1, 0, 0, 0.5, 0.002},
		{"hsl(0 100% 50%)", 1, 0, 0, 1, 0.002},
		{"hsla(120, 100%, 50%, 1)", 0, 1, 0, 1, 0.002},
		{"hsl(0.5turn 100% 50%)", 0, 1, 1, 1, 0.002},
		{"hwb(0 0% 0%)", 1, 0, 0, 1, 0.002},
		{"hwb(0 60% 60%)", 0.5, 0.5, 0.5, 1, 0.002},
		{"oklch(1 0 0)", 1, 1, 1, 1, 0.002},
		{"oklch(0 0 0)", 0, 0, 0, 1, 0.002},
		{"oklch(0.577 0.245 27.325)", 0.906, 0, 0.043, 1, 0.01},
		{"oklch(0.205 0 0)", 23.0 / 255, 23.0 / 255, 23.0 / 255, 1, 0.003},
		{"oklch(50% 0 0deg / 25%)", 0.3889, 0.3889, 0.3889, 0.25, 0.003},
		// Red in each space (Ottosson's oklab red; the widely published D50 Lab/LCH red).
		{"oklab(0.62796 0.22486 0.12585)", 1, 0, 0, 1, 0.003},
		{"lab(54.2905 80.8049 69.8910)", 1, 0, 0, 1, 0.003},
		{"lch(54.2905 106.8390 40.8526)", 1, 0, 0, 1, 0.003},
		{"lab(100 0 0)", 1, 1, 1, 1, 0.002},
		{"color(srgb 1 0 0)", 1, 0, 0, 1, 0.002},
		{"color(display-p3 1 0 0)", 1, 0, 0, 1, 0.002},
		{"color(display-p3 0.5 0.5 0.5)", 0.5, 0.5, 0.5, 1, 0.002},
		{"red", 1, 0, 0, 1, 0.002},
		{"RebeccaPurple", 0.4, 0.2, 0.6, 1, 0.002},
		{"transparent", 0, 0, 0, 0, 0.002},
		{"oklch(none 0 0)", 0, 0, 0, 1, 0.002},
	}
	for _, c := range cases {
		got, ok := ParseColour(c.in)
		if !ok {
			t.Errorf("%q: not ok", c.in)
			continue
		}
		for i, p := range [][2]float64{{got.R, c.r}, {got.G, c.g}, {got.B, c.b}, {got.A, c.a}} {
			if math.Abs(p[0]-p[1]) > c.tol {
				t.Errorf("%q: channel %d = %.4f, want %.4f", c.in, i, p[0], p[1])
			}
		}
	}
}

func TestParseColourRejects(t *testing.T) {
	for _, in := range []string{"currentColor", "color-mix(in oklab, red 50%, blue)", "light-dark(red, blue)",
		"var(--x)", "", "oklch(1 0)", "#12", "#12345", "nonsense", "rgb(1 2)", "oklch(1 0 0 0 0)", "oklch(1 0 0"} {
		if got, ok := ParseColour(in); ok {
			t.Errorf("%q: ok, got %+v", in, got)
		}
	}
}

func TestNamedColourCount(t *testing.T) {
	if len(namedColours) != 148 {
		t.Errorf("named colours = %d, want 148", len(namedColours))
	}
}
