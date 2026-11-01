package theme

import (
	"embed"
	"fmt"
	"io/fs"
	"maps"
	"sort"
	"strings"
	"sync"
)

//go:embed presets
var presetFS embed.FS

// Preset is a starting theme shipped with alder: Name is its directory (and the name
// `alder theme use` takes), Title and Purpose come from its resolver.
type Preset struct {
	Name, Title, Purpose string
	Theme                Theme
}

// Presets is the embedded starting themes: zinc first, then by Name. The result is cached and
// shared, so it is read-only: take a copy with PresetNamed before changing a Theme. It panics when a
// vendored preset fails to import or leaves a token unplaced, since that is a build defect.
var Presets = sync.OnceValue(func() []Preset {
	entries, err := fs.ReadDir(presetFS, "presets")
	if err != nil {
		panic(fmt.Sprintf("theme: reading presets: %v", err))
	}
	var out []Preset
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		sub, err := fs.Sub(presetFS, "presets/"+e.Name())
		if err != nil {
			panic(fmt.Sprintf("theme: preset %s: %v", e.Name(), err))
		}
		th, unplaced, _, err := ImportDTCG(sub, "theme.resolver.json")
		if err != nil {
			panic(fmt.Sprintf("theme: preset %s: %v", e.Name(), err))
		}
		if len(unplaced) > 0 {
			panic(fmt.Sprintf("theme: preset %s: %d unplaced tokens (first: %+v)", e.Name(), len(unplaced), unplaced[0]))
		}
		doc, err := loadDTCG(sub, "theme.resolver.json")
		if err != nil {
			panic(fmt.Sprintf("theme: preset %s: %v", e.Name(), err))
		}
		out = append(out, Preset{Name: e.Name(), Title: doc.Name, Purpose: doc.Description, Theme: th})
	}
	sort.SliceStable(out, func(i, j int) bool {
		if (out[i].Name == "zinc") != (out[j].Name == "zinc") {
			return out[i].Name == "zinc"
		}
		return out[i].Name < out[j].Name
	})
	return out
})

// PresetNamed is the preset called name. Its Theme's maps are copies, so the caller may change
// them without touching the cached presets.
func PresetNamed(name string) (Preset, bool) {
	for _, p := range Presets() {
		if p.Name == name {
			p.Theme = Theme{Root: maps.Clone(p.Theme.Root), Light: maps.Clone(p.Theme.Light), Dark: maps.Clone(p.Theme.Dark)}
			return p, true
		}
	}
	return Preset{}, false
}

// SuggestPreset is the preset name nearest to name by edit distance, if it is at most 2 away.
func SuggestPreset(name string) (string, bool) {
	best, bestD := "", 3
	for _, p := range Presets() {
		if d := distance(strings.ToLower(name), p.Name); d < bestD {
			best, bestD = p.Name, d
		}
	}
	return best, bestD <= 2
}

// PresetNames lists the embedded presets' names in Presets' order (zinc first, then by name)
// from the directory alone, without importing any of them: it is cheap enough for help text and
// cannot panic on a bad preset.
func PresetNames() []string {
	entries, err := fs.ReadDir(presetFS, "presets")
	if err != nil {
		panic(fmt.Sprintf("theme: reading presets: %v", err))
	}
	var out []string
	for _, e := range entries {
		if e.IsDir() {
			out = append(out, e.Name())
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if (out[i] == "zinc") != (out[j] == "zinc") {
			return out[i] == "zinc"
		}
		return out[i] < out[j]
	})
	return out
}
