package theme

import "testing"

func TestPresets_AllPlaced(t *testing.T) {
	if got := len(Presets()); got != 5 {
		t.Fatalf("got %d presets, want 5", got)
	}
}

func TestPresets_Order(t *testing.T) {
	want := []string{"zinc", "brutal", "catppuccin", "paper", "pebble"}
	for i, p := range Presets() {
		if p.Name != want[i] {
			t.Errorf("preset %d = %q, want %q", i, p.Name, want[i])
		}
	}
}

func TestPresets_Purposes(t *testing.T) {
	want := map[string]string{
		"zinc":       "Tools, dashboards, admin — the default, the safe choice.",
		"paper":      "Reading: blogs, docs, long-form.",
		"catppuccin": "Developer-facing apps.",
		"brutal":     "Launch pages, portfolios, anything that should shout.",
		"pebble":     "Consumer, mobile-feel apps.",
	}
	for _, p := range Presets() {
		if p.Purpose != want[p.Name] {
			t.Errorf("%s purpose = %q, want %q", p.Name, p.Purpose, want[p.Name])
		}
		if p.Title == "" {
			t.Errorf("%s has no title", p.Name)
		}
	}
}

func TestPresetNamed_Unknown(t *testing.T) {
	if _, ok := PresetNamed("papr"); ok {
		t.Error("papr should not be found")
	}
	if p, ok := PresetNamed("paper"); !ok || p.Name != "paper" {
		t.Errorf("paper not found: %v", ok)
	}
	if s, ok := SuggestPreset("papr"); !ok || s != "paper" {
		t.Errorf("SuggestPreset(papr) = %q, %v", s, ok)
	}
	if s, ok := SuggestPreset("nothingclose"); ok {
		t.Errorf("unexpected suggestion %q", s)
	}
}

func TestPresets_Counts(t *testing.T) {
	for _, p := range Presets() {
		if len(p.Theme.Light) != 32 || len(p.Theme.Dark) != 32 || len(p.Theme.Root) != 15 {
			t.Errorf("%s: %d/%d/%d, want 32/32/15", p.Name, len(p.Theme.Light), len(p.Theme.Dark), len(p.Theme.Root))
		}
		if p.Name == "brutal" && p.Theme.Root["borderWidth"] != "2px" {
			t.Errorf("brutal borderWidth = %q", p.Theme.Root["borderWidth"])
		}
	}
}
