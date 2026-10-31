package generate

import (
	"errors"
	"fmt"
	"path/filepath"
	"slices"
	"strings"

	"alder/internal/style"
	"alder/internal/syntax"
	"alder/internal/theme"
)

// ThemeImport is what `alder theme import` did (#38 phase 2): the theme the export became, what
// it could not place, and how it compares with the theme App had.
type ThemeImport struct {
	Theme    theme.Theme
	Unplaced []theme.Unplaced
	Skipped  theme.Skipped
	Counts   struct{ Light, Dark, Wide int }
	Replaced bool     // App had a theme before
	Dropped  []string // fields the old theme set that the import does not (`radius`, `light.primary`)
	Print    string   // --print: the declaration; nothing written
}

// ImportTheme reads the CSS export css (named name, for messages; "-" is standard input) into a
// theme and makes it App's `theme` declaration (WriteTheme), replacing the one there. With
// print the declaration is returned in Import.Print and nothing is written. A result carrying
// Import comes back with a write's refusal too, so --json can still show the theme.
func ImportTheme(app App, name string, css []byte, print, dryRun bool) (*Result, error) {
	th, unplaced, skipped, err := theme.Import(string(css))
	if nt := (*theme.NoThemeError)(nil); errors.As(err, &nt) {
		nt.Name = filepath.Base(name)
		if name == "-" {
			nt.Name = "stdin"
		}
		return nil, Refusef("%s", nt.Error())
	} else if err != nil {
		return nil, err
	}
	imp := &ThemeImport{Theme: th, Unplaced: unplaced, Skipped: skipped}
	imp.Counts.Light, imp.Counts.Dark, imp.Counts.Wide = len(th.Light), len(th.Dark), len(th.Root)
	if print {
		text, err := ThemeText(th)
		if inv := (*ThemeInvalid)(nil); errors.As(err, &inv) {
			return &Result{Import: imp, Diagnostics: inv.Diags}, &Refusal{Msg: inv.Error(), Diags: inv.Diags}
		} else if err != nil {
			return nil, err
		}
		imp.Print = text
		return &Result{Import: imp}, nil
	}
	if p, err := LoadWithErrors(app.Dir); err == nil {
		if m := p.Modules["App"]; m != nil {
			if f, _ := syntax.ParseFile(m.File, []byte(m.Source)); f != nil {
				for _, d := range f.Decls {
					if td, ok := d.(*syntax.ThemeDecl); ok {
						imp.Replaced = true
						imp.Dropped = dropped(ThemeOf(td), th)
						break
					}
				}
			}
		}
	}
	res, err := WriteTheme(app, th, dryRun)
	if res == nil {
		res = &Result{}
	}
	if inv := (*ThemeInvalid)(nil); errors.As(err, &inv) {
		err = &Refusal{Msg: inv.Error(), Diags: inv.Diags}
		res.Diagnostics = inv.Diags // the renderers show the result's, not the refusal's
	}
	res.Import = imp
	return res, err
}

// dropped is the fields old sets that now does not: theme-wide ones by name, colours as
// `light.primary`, in sorted order within each group.
func dropped(old, now theme.Theme) []string {
	var out []string
	for _, g := range []struct {
		prefix   string
		old, now map[string]string
	}{{"", old.Root, now.Root}, {"light.", old.Light, now.Light}, {"dark.", old.Dark, now.Dark}} {
		var ks []string
		for k := range g.old {
			if _, ok := g.now[k]; !ok {
				ks = append(ks, g.prefix+k)
			}
		}
		slices.Sort(ks)
		out = append(out, ks...)
	}
	return out
}

// renderText is the import's report before the files list: the summary line, the dropped
// fields, the unplaced block (valid CSS, reasons as comments) and the skipped at-rules.
func (t *ThemeImport) renderText(files []FileChange, st style.Style) string {
	var b strings.Builder
	file := "app/App.alder"
	if len(files) > 0 {
		file = files[0].Path
	}
	note := ""
	if t.Replaced {
		note = "; replaced the theme that was there"
	}
	fmt.Fprintf(&b, "%s %d tokens → %s  (light %d, dark %d, theme-wide %d%s)\n", st.Bold("theme:"),
		t.Counts.Light+t.Counts.Dark+t.Counts.Wide, file, t.Counts.Light, t.Counts.Dark, t.Counts.Wide, note)
	if len(t.Dropped) > 0 {
		fmt.Fprintf(&b, "%s\n", st.Dim("dropped, the export does not set them: "+strings.Join(t.Dropped, ", ")))
	}
	b.WriteString(t.RenderUnplaced(st))
	if t.Skipped.N > 0 {
		blocks := "blocks"
		if t.Skipped.N == 1 {
			blocks = "block"
		}
		fmt.Fprintf(&b, "skipped %d %s (%s)\n", t.Skipped.N, blocks, strings.Join(t.Skipped.Kinds, ", "))
	}
	return b.String()
}

// RenderUnplaced is the "not placed" heading and the block under it, indented two spaces with
// the reasons dimmed; "" when everything was placed.
func (t *ThemeImport) RenderUnplaced(st style.Style) string {
	if len(t.Unplaced) == 0 {
		return ""
	}
	var b strings.Builder
	fmt.Fprintf(&b, "%d not placed — paste into static/app.css if you want them:\n", len(t.Unplaced))
	for _, l := range strings.Split(strings.TrimRight(theme.UnplacedCSS(t.Unplaced), "\n"), "\n") {
		if i := strings.Index(l, "/*"); i >= 0 {
			l = l[:i] + st.Dim(l[i:])
		}
		if l != "" {
			b.WriteString("  ")
		}
		b.WriteString(l + "\n")
	}
	return b.String()
}
