package generate

import (
	"strings"
	"testing"

	"alder/internal/theme"
)

func TestThemeUse_PresetsCheck(t *testing.T) {
	for _, p := range theme.Presets() {
		if _, err := ThemeText(p.Theme); err != nil {
			t.Errorf("preset %s: %v", p.Name, err)
		}
	}
}

func TestPresetList(t *testing.T) {
	got := PresetList()
	if !strings.HasPrefix(got, "starting themes (alder-ui "+theme.Version()+"):\n  zinc        Tools") {
		t.Errorf("list:\n%s", got)
	}
}
