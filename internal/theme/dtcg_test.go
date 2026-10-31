package theme

import (
	"fmt"
	"os"
	"reflect"
	"strings"
	"testing"
	"testing/fstest"
)

func mustLoadDTCG(t *testing.T, dir, name string) *dtcgDoc {
	t.Helper()
	doc, err := loadDTCG(os.DirFS("testdata/dtcg/"+dir), name)
	if err != nil {
		t.Fatalf("loadDTCG(%s/%s): %v", dir, name, err)
	}
	return doc
}

// paths renders each token as "file path=value" so a test reads the order, origin and
// value at once.
func paths(ts []dtcgToken) []string {
	out := make([]string, len(ts))
	for i, tk := range ts {
		out[i] = tk.File + " " + strings.Join(tk.Path, ".") + "=" + string(tk.Value)
	}
	return out
}

func tokenAt(t *testing.T, ts []dtcgToken, path string) dtcgToken {
	t.Helper()
	for _, tk := range ts {
		if strings.Join(tk.Path, ".") == path {
			return tk
		}
	}
	t.Fatalf("no token %s in %q", path, paths(ts))
	return dtcgToken{}
}

func TestLoadDTCG_Resolver(t *testing.T) {
	doc := mustLoadDTCG(t, "resolver", "theme.resolver.json")
	if doc.Name != "Fixture" || doc.Description != "A resolver fixture: two sets and a scheme modifier." {
		t.Errorf("Name, Description = %q, %q", doc.Name, doc.Description)
	}
	wantRoot := []string{
		`base.tokens.json radius.radius={ "value": 0.625, "unit": "rem" }`,
		// overrides.tokens.json redefines radius-sm: the later set wins, in the first one's place.
		`overrides.tokens.json radius.radius-sm={ "value": 0.25, "unit": "rem" }`,
		`base.tokens.json font.sans=["Inter", "sans-serif"]`,
		`base.tokens.json color.brand={ "colorSpace": "oklch", "components": [0.5, 0.2, 260] }`,
		`overrides.tokens.json shadow.shadow-sm={ "offsetX": { "value": 0, "unit": "px" }, "offsetY": { "value": 1, "unit": "px" }, "blur": { "value": 2, "unit": "px" }, "spread": { "value": 0, "unit": "px" }, "color": "{color.brand}" }`,
	}
	if got := paths(doc.Root); !reflect.DeepEqual(got, wantRoot) {
		t.Errorf("Root =\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(wantRoot, "\n"))
	}
	wantLight := []string{
		`light.tokens.json color.background={ "colorSpace": "oklch", "components": [1, 0, 0] }`,
		`light.tokens.json color.foreground={ "colorSpace": "oklch", "components": [0.145, 0, 0] }`,
		`light.tokens.json color.primary="{color.brand}"`,
		`light.tokens.json color.ring="{color.primary}"`,
	}
	if got := paths(doc.Light); !reflect.DeepEqual(got, wantLight) {
		t.Errorf("Light =\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(wantLight, "\n"))
	}
	wantDark := []string{
		`dark.tokens.json color.background={ "colorSpace": "oklch", "components": [0.145, 0, 0] }`,
		`dark.tokens.json color.foreground={ "colorSpace": "oklch", "components": [0.985, 0, 0] }`,
		`dark.tokens.json color.primary={ "colorSpace": "oklch", "components": [0.7, 0.15, 260] }`,
	}
	if got := paths(doc.Dark); !reflect.DeepEqual(got, wantDark) {
		t.Errorf("Dark =\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(wantDark, "\n"))
	}
	if bg := tokenAt(t, doc.Light, "color.background"); bg.Description != "The page background." || bg.Type != "color" {
		t.Errorf("light background = %+v", bg)
	}
	if r := tokenAt(t, doc.Root, "radius.radius"); r.Description != "The base radius." {
		t.Errorf("radius description = %q", r.Description)
	}
	if doc.Skipped.N != 0 {
		t.Errorf("Skipped = %+v, want none", doc.Skipped)
	}

	// Alias lookup sees the scheme first, then the root.
	for _, c := range []struct {
		path  string
		dark  bool
		file  string
		found bool
	}{
		{"color.primary", false, "light.tokens.json", true},
		{"color.primary", true, "dark.tokens.json", true},
		{"color.brand", false, "base.tokens.json", true},
		{"color.brand", true, "base.tokens.json", true},
		{"radius.radius-sm", true, "overrides.tokens.json", true},
		{"color.missing", false, "", false},
	} {
		tk, ok := doc.lookup(c.path, c.dark)
		if ok != c.found || tk.File != c.file {
			t.Errorf("lookup(%s, dark=%v) = %s, %v; want %s, %v", c.path, c.dark, tk.File, ok, c.file, c.found)
		}
	}
}

func TestLoadDTCG_SchemeModifierNames(t *testing.T) {
	doc := mustLoadDTCG(t, "resolver", "mode.resolver.json")
	wantRoot := []string{
		`mode.resolver.json color.brand="#336699"`,
		`mode.resolver.json spacing.gap={ "value": 0.5, "unit": "rem" }`, // density at its default
		`mode.resolver.json color.border="#cccccc"`,                      // contrast at its first context
	}
	if got := paths(doc.Root); !reflect.DeepEqual(got, wantRoot) {
		t.Errorf("Root = %q, want %q", got, wantRoot)
	}
	if got, want := paths(doc.Light), []string{`mode.resolver.json color.background="#ffffff"`}; !reflect.DeepEqual(got, want) {
		t.Errorf("Light = %q, want %q", got, want)
	}
	if got, want := paths(doc.Dark), []string{`mode.resolver.json color.background="#000000"`}; !reflect.DeepEqual(got, want) {
		t.Errorf("Dark = %q, want %q", got, want)
	}
	if want := (Skipped{N: 2, Kinds: []string{"density", "contrast"}}); !reflect.DeepEqual(doc.Skipped, want) {
		t.Errorf("Skipped = %+v, want %+v", doc.Skipped, want)
	}

	doc = mustLoadDTCG(t, "resolver", "colorscheme.resolver.json")
	if got, want := paths(doc.Light), []string{`colorscheme.resolver.json color.background="#fafafa"`}; !reflect.DeepEqual(got, want) {
		t.Errorf("colorScheme Light = %q, want %q", got, want)
	}
	if got, want := paths(doc.Dark), []string{`colorscheme.resolver.json color.background="#0a0a0a"`}; !reflect.DeepEqual(got, want) {
		t.Errorf("colorScheme Dark = %q, want %q", got, want)
	}
	if len(doc.Root) != 0 || doc.Skipped.N != 0 {
		t.Errorf("colorScheme Root = %q, Skipped = %+v; want both empty", paths(doc.Root), doc.Skipped)
	}
}

// A modifier with a scheme name but no light/dark contexts is not the scheme: it is resolved
// like any other and counted as skipped, and a later one that has them is picked.
func TestLoadDTCG_SchemeNeedsLightAndDark(t *testing.T) {
	fsys := fstest.MapFS{"t.resolver.json": {Data: []byte(`{
		"modifiers": {
			"theme": {"contexts": {"brand-a": [{"x": {"$type": "color", "$value": "#aaaaaa"}}], "brand-b": []}},
			"scheme": {"contexts": {"dark": [{"bg": {"$type": "color", "$value": "#000000"}}], "light": [{"bg": {"$type": "color", "$value": "#ffffff"}}]}}
		},
		"resolutionOrder": [{"$ref": "#/modifiers/theme"}, {"$ref": "#/modifiers/scheme"}]
	}`)}}
	doc, err := loadDTCG(fsys, "t.resolver.json")
	if err != nil {
		t.Fatal(err)
	}
	if got := paths(doc.Root); !reflect.DeepEqual(got, []string{`t.resolver.json x="#aaaaaa"`}) {
		t.Errorf("Root = %q", got)
	}
	if got := paths(doc.Light); !reflect.DeepEqual(got, []string{`t.resolver.json bg="#ffffff"`}) {
		t.Errorf("Light = %q", got)
	}
	if want := (Skipped{N: 1, Kinds: []string{"theme"}}); !reflect.DeepEqual(doc.Skipped, want) {
		t.Errorf("Skipped = %+v, want %+v", doc.Skipped, want)
	}
}

func TestLoadDTCG_SingleFile(t *testing.T) {
	doc := mustLoadDTCG(t, "single-file", "tokens.json")
	if doc.Name != "" || doc.Description != "" {
		t.Errorf("a tokens file has no Name/Description, got %q, %q", doc.Name, doc.Description)
	}
	wantRoot := []string{
		`tokens.json radius.corners.radius={ "value": 0.5, "unit": "rem" }`,
		`tokens.json brand="#336699"`,
	}
	if got := paths(doc.Root); !reflect.DeepEqual(got, wantRoot) {
		t.Errorf("Root = %q, want %q", got, wantRoot)
	}
	// The scheme group's own name is not part of the path, so `{color.background}` finds it.
	wantLight := []string{`tokens.json color.background="#ffffff"`, `tokens.json color.foreground="#111111"`}
	if got := paths(doc.Light); !reflect.DeepEqual(got, wantLight) {
		t.Errorf("Light = %q, want %q", got, wantLight)
	}
	wantDark := []string{`tokens.json color.background="#111111"`, `tokens.json color.foreground="#ffffff"`}
	if got := paths(doc.Dark); !reflect.DeepEqual(got, wantDark) {
		t.Errorf("Dark = %q, want %q", got, wantDark)
	}
	if fg := tokenAt(t, doc.Light, "color.foreground"); fg.Description != "Text." {
		t.Errorf("foreground description = %q", fg.Description)
	}
	if tk, ok := doc.lookup("color.background", true); !ok || string(tk.Value) != `"#111111"` {
		t.Errorf("dark lookup = %+v, %v", tk, ok)
	}
}

func TestLoadDTCG_TypeInherited(t *testing.T) {
	doc := mustLoadDTCG(t, "single-file", "tokens.json")
	// Two groups down from the `$type`.
	if tk := tokenAt(t, doc.Root, "radius.corners.radius"); tk.Type != "dimension" {
		t.Errorf("radius type = %q, want dimension", tk.Type)
	}
	if tk := tokenAt(t, doc.Dark, "color.foreground"); tk.Type != "color" {
		t.Errorf("dark foreground type = %q, want color", tk.Type)
	}
	// A token's own `$type` beats its group's.
	fsys := fstest.MapFS{"t.json": {Data: []byte(`{"g": {"$type": "color", "a": {"$value": 1, "$type": "number"}, "b": {"$value": "#fff"}}}`)}}
	d, err := loadDTCG(fsys, "t.json")
	if err != nil {
		t.Fatal(err)
	}
	if a, b := tokenAt(t, d.Root, "g.a"), tokenAt(t, d.Root, "g.b"); a.Type != "number" || b.Type != "color" {
		t.Errorf("types = %q, %q; want number, color", a.Type, b.Type)
	}
}

func TestLoadDTCG_NoType(t *testing.T) {
	doc := mustLoadDTCG(t, "single-file", "tokens.json")
	if tk := tokenAt(t, doc.Root, "brand"); tk.Type != "" {
		t.Errorf("brand type = %q, want none", tk.Type)
	}
}

func TestLoadDTCG_Limits(t *testing.T) {
	for _, c := range []struct{ name, want string }{
		{"escape.resolver.json", "../x.json"},
		{"absolute.resolver.json", "/x.json"},
		{"missing.resolver.json", "nowhere.tokens.json"},
	} {
		_, err := loadDTCG(os.DirFS("testdata/dtcg/limits"), c.name)
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: err = %v, want one naming %s", c.name, err, c.want)
		}
	}

	// n files, each referenced by its own set.
	resolver := func(n int) fstest.MapFS {
		fsys := fstest.MapFS{}
		var sets, order []string
		for i := range n {
			f := fmt.Sprintf("f%d.tokens.json", i)
			fsys[f] = &fstest.MapFile{Data: []byte(fmt.Sprintf(`{"t%d": {"$type": "number", "$value": %d}}`, i, i))}
			sets = append(sets, fmt.Sprintf(`"s%d": {"sources": [{"$ref": %q}]}`, i, f))
			order = append(order, fmt.Sprintf(`{"$ref": "#/sets/s%d"}`, i))
		}
		fsys["theme.resolver.json"] = &fstest.MapFile{Data: []byte(`{"sets": {` + strings.Join(sets, ",") +
			`}, "resolutionOrder": [` + strings.Join(order, ",") + `]}`)}
		return fsys
	}
	if doc, err := loadDTCG(resolver(32), "theme.resolver.json"); err != nil || len(doc.Root) != 32 {
		t.Errorf("32 files: err = %v, want them read", err)
	}
	if _, err := loadDTCG(resolver(33), "theme.resolver.json"); err == nil || !strings.Contains(err.Error(), "32") {
		t.Errorf("33 files: err = %v, want the 32-file limit", err)
	}

	big := `{"pad": {"$description": "` + strings.Repeat("x", 1<<20) + `", "t": {"$type": "number", "$value": 1}}}`
	if _, err := loadDTCG(fstest.MapFS{"big.json": {Data: []byte(big)}}, "big.json"); err == nil || !strings.Contains(err.Error(), "1 MiB") {
		t.Errorf("big tokens file: err = %v, want the 1 MiB limit", err)
	}
	big2 := fstest.MapFS{
		"theme.resolver.json": {Data: []byte(`{"sets": {"b": {"sources": [{"$ref": "big.json"}]}}, "resolutionOrder": [{"$ref": "#/sets/b"}]}`)},
		"big.json":            {Data: []byte(big)},
	}
	if _, err := loadDTCG(big2, "theme.resolver.json"); err == nil || !strings.Contains(err.Error(), "big.json") || !strings.Contains(err.Error(), "1 MiB") {
		t.Errorf("big referenced file: err = %v, want the 1 MiB limit naming big.json", err)
	}
}

func TestLoadDTCG_Malformed(t *testing.T) {
	for name, src := range map[string]string{
		"not json":       `{"a": `,
		"not an object":  `[1, 2]`,
		"stray member":   `{"color": {"primary": "#fff"}}`,
		"unknown set":    `{"resolutionOrder": [{"$ref": "#/sets/nope"}]}`,
		"bad default":    `{"modifiers": {"m": {"default": "x", "contexts": {"a": []}}}, "resolutionOrder": [{"$ref": "#/modifiers/m"}]}`,
		"file fragment":  `{"sets": {"s": {"sources": [{"$ref": "a.json#/x"}]}}, "resolutionOrder": [{"$ref": "#/sets/s"}]}`,
		"trailing value": `{} {}`,
	} {
		if _, err := loadDTCG(fstest.MapFS{"t.json": {Data: []byte(src)}}, "t.json"); err == nil {
			t.Errorf("%s: loadDTCG accepted %s", name, src)
		}
	}
}

// The presets alder ships load with every token placed in a scheme or the root.
func TestLoadDTCG_Presets(t *testing.T) {
	for _, name := range []string{"zinc", "brutal", "catppuccin", "paper", "pebble"} {
		doc, err := loadDTCG(os.DirFS("presets/"+name), "theme.resolver.json")
		if err != nil {
			t.Errorf("%s: %v", name, err)
			continue
		}
		if doc.Name == "" || len(doc.Root) == 0 || len(doc.Light) == 0 || len(doc.Light) != len(doc.Dark) {
			t.Errorf("%s: Name %q, %d root, %d light, %d dark", name, doc.Name, len(doc.Root), len(doc.Light), len(doc.Dark))
		}
	}
}

// A leading BOM is not part of the document; a source may name another set, and a
// `resolutionOrder` entry may be an inline set.
func TestLoadDTCG_BOMAndSetSources(t *testing.T) {
	fsys := fstest.MapFS{
		"theme.resolver.json": {Data: []byte("\uFEFF" + `{
			"sets": {"a": {"sources": [{"$ref": "a.json"}]}, "b": {"sources": [{"$ref": "#/sets/a"}, {"$ref": "b.json"}]}},
			"resolutionOrder": [{"$ref": "#/sets/b"}, {"type": "set", "name": "inline", "sources": [{"z": {"$type": "number", "$value": 3}}]}]
		}`)},
		"a.json": {Data: []byte("\uFEFF" + `{"x": {"$type": "number", "$value": 1}}`)},
		"b.json": {Data: []byte(`{"y": {"$type": "number", "$value": 2}}`)},
	}
	doc, err := loadDTCG(fsys, "theme.resolver.json")
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"a.json x=1", "b.json y=2", "theme.resolver.json z=3"}
	if got := paths(doc.Root); !reflect.DeepEqual(got, want) {
		t.Errorf("Root = %q, want %q", got, want)
	}

	cycle := fstest.MapFS{"t.json": {Data: []byte(`{"sets": {"a": {"sources": [{"$ref": "#/sets/b"}]}, "b": {"sources": [{"$ref": "#/sets/a"}]}}, "resolutionOrder": [{"$ref": "#/sets/a"}]}`)}}
	if _, err := loadDTCG(cycle, "t.json"); err == nil || !strings.Contains(err.Error(), "includes itself") {
		t.Errorf("set cycle: err = %v", err)
	}
}

// $ref paths resolve against the resolver's own directory.
func TestLoadDTCG_Subdirectory(t *testing.T) {
	fsys := fstest.MapFS{
		"themes/x/theme.resolver.json": {Data: []byte(`{"sets": {"a": {"sources": [{"$ref": "a.json"}]}}, "resolutionOrder": [{"$ref": "#/sets/a"}]}`)},
		"themes/x/a.json":              {Data: []byte(`{"x": {"$type": "number", "$value": 1}}`)},
	}
	doc, err := loadDTCG(fsys, "themes/x/theme.resolver.json")
	if err != nil {
		t.Fatal(err)
	}
	if got := paths(doc.Root); !reflect.DeepEqual(got, []string{"themes/x/a.json x=1"}) {
		t.Errorf("Root = %q", got)
	}
}

// Sets that each refer to the next twice expand exponentially: past dtcgMaxSources the loader
// stops with an error instead of walking them all.
func TestLoadDTCG_SourceCap(t *testing.T) {
	chain := func(n int) fstest.MapFS {
		var sets []string
		for i := range n {
			sets = append(sets, fmt.Sprintf(`"s%d": {"sources": [{"$ref": "#/sets/s%d"}, {"$ref": "#/sets/s%d"}]}`, i, i+1, i+1))
		}
		sets = append(sets, fmt.Sprintf(`"s%d": {"sources": [{"$ref": "a.json"}]}`, n))
		return fstest.MapFS{
			"theme.resolver.json": {Data: []byte(`{"sets": {` + strings.Join(sets, ",") + `}, "resolutionOrder": [{"$ref": "#/sets/s0"}]}`)},
			"a.json":              {Data: []byte(`{"x": {"$type": "number", "$value": 1}}`)},
		}
	}
	// 4 levels: 31 sets and 16 file refs, well inside the cap; the file is read once.
	if doc, err := loadDTCG(chain(4), "theme.resolver.json"); err != nil || len(doc.Root) != 1 {
		t.Errorf("4 levels: err = %v", err)
	}
	if _, err := loadDTCG(chain(10), "theme.resolver.json"); err == nil || !strings.Contains(err.Error(), "1024") {
		t.Errorf("10 levels: err = %v, want the 1024-source cap", err)
	}
}
