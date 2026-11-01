package generate

import (
	"fmt"
	"strings"

	"alder/internal/theme"
)

// PresetList is `alder theme use`'s list of the starting themes and what each is for.
func PresetList() string {
	var b strings.Builder
	fmt.Fprintf(&b, "starting themes (alder-ui %s):\n", theme.Version())
	for _, p := range theme.Presets() {
		fmt.Fprintf(&b, "  %-12s%s\n", p.Name, p.Purpose)
	}
	b.WriteString("alder theme use <name> writes one into app/App.alder; the app owns it from there.\n")
	return b.String()
}

// UsePreset makes the starting theme name App's `theme` declaration (the same replace-or-insert
// as ImportTheme). With print the declaration is returned in Import.Print and nothing is
// written. An unknown or empty name is refused with the list of presets.
func UsePreset(app App, name string, print, dryRun bool) (*Result, error) {
	p, ok := theme.PresetNamed(name)
	if !ok {
		msg := "name a starting theme"
		if name != "" {
			msg = fmt.Sprintf("no starting theme `%s`", name)
			if s, ok := theme.SuggestPreset(name); ok {
				msg += fmt.Sprintf("; did you mean `%s`?", s)
			}
		}
		return nil, Refusef("%s\n%s", msg, strings.TrimRight(PresetList(), "\n"))
	}
	res, err := themeResult(app, p.Theme, nil, theme.Skipped{}, print, dryRun)
	if res != nil && res.Import != nil {
		res.Import.Preset = p.Name
	}
	return res, err
}
