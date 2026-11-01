package theme

import "testing"

func TestEquivalent(t *testing.T) {
	cases := []struct {
		k    Kind
		a, b string
		want bool
	}{
		{Colour, "oklch(1 0 0 / 10%)", "rgb(255 255 255 / 0.1)", true},
		{Colour, "#ffffff", "white", true},
		{Colour, "#000000", "#020202", false}, // 2/255 apart
		{Colour, "#000000", "#010101", true},  // 1/255 apart
		{Colour, "rgb(0 0 0)", "rgb(0.5 0 0)", true},
		{Colour, "rgb(0 0 0 / 0.5)", "rgb(0 0 0 / 0.6)", false}, // alpha counts
		{Colour, "var(--primary)", "var(--primary)", true},
		{Colour, "var(--primary)", " var(--primary) ", true},
		{Colour, "var(--primary)", "var(--accent)", false},
		{Colour, "var(--primary)", "#000000", false},
		{Length, "0", "0px", true},
		{Length, "0px", "0rem", true},
		{Length, "0", "0rem", true},
		{Length, "0.5rem", "8px", false},
		{Length, "0.5rem", "0.50rem", true},
		{Length, "0.5rem", "0.5em", false},
		{Length, "1px", "2px", false},
		{Shadow, "0 1px 2px 0 rgb(0 0 0 / 0.05)", "0px 1px 2px 0px rgb(0 0 0 / 5%)", true},
		{Shadow, "0 1px 2px 0 rgb(0 0 0 / 0.05)", "0 1px 3px 0 rgb(0 0 0 / 0.05)", false},
		{Shadow, "0 1px 2px 0 rgb(0 0 0 / 0.05)", "0 1px 2px 0 rgb(0 0 0 / 0.5)", false},
		{Shadow, "0 1px 3px 0 rgb(0 0 0 / 0.1), 0 1px 2px -1px rgb(0 0 0 / 0.1)", "0 1px 3px 0px rgb(0 0 0 / 10%),0 1px 2px -1px #0000001a", true},
		{Shadow, "0 1px 3px 0 rgb(0 0 0 / 0.1), 0 1px 2px -1px rgb(0 0 0 / 0.1)", "0 1px 3px 0 rgb(0 0 0 / 0.1)", false},
		{Shadow, "inset 0 1px 2px black", "0 1px 2px black inset", true},
		{Shadow, "inset 0 1px 2px black", "0 1px 2px black", false},
		{Shadow, "none", "none", true},
		{Shadow, "none", "0 0 0 0 transparent", false},
		{Font, `"Geist", sans-serif`, "Geist, sans-serif", true},
		{Font, `'Geist Mono', monospace`, `"Geist Mono",monospace`, true},
		{Font, "Geist, sans-serif", "sans-serif, Geist", false},
		{Font, "Geist, sans-serif", "Geist", false},
	}
	for _, c := range cases {
		if got := Equivalent(c.k, c.a, c.b); got != c.want {
			t.Errorf("Equivalent(%v, %q, %q) = %v, want %v", c.k, c.a, c.b, got, c.want)
		}
		if got := Equivalent(c.k, c.b, c.a); got != c.want {
			t.Errorf("Equivalent(%v, %q, %q) = %v, want %v (swapped)", c.k, c.b, c.a, got, c.want)
		}
	}
}

func TestImport_DarkEqualsRootEquivalent(t *testing.T) {
	th, un, _ := mustImport(t, ":root { --radius: 0; }\n.dark { --radius: 0px; }")
	if th.Root["radius"] != "0" {
		t.Errorf("radius = %q, want 0", th.Root["radius"])
	}
	if len(un) != 0 {
		t.Errorf("unplaced = %+v, want none", un)
	}
}
