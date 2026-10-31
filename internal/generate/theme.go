package generate

import (
	"errors"
	"fmt"
	"slices"
	"strings"

	"alder/internal/check"
	"alder/internal/diag"
	"alder/internal/format"
	"alder/internal/syntax"
	"alder/internal/theme"
)

// The theme write (#38 phase 2, spec "The write"): the configurator's Write and `alder theme
// import` turn a theme.Theme into App's `theme` declaration. The fields are checked first
// (check.ThemeFields, E0042), so no text reaches the printer that the checker's closed sets did
// not admit; the print replaces the lines of the declaration App has, or goes in after its
// `deploy`, `session`, last import or `module` line; nothing else in the file changes.

// ThemeOf reads a declaration into the Theme shape (fonts joined by ", "); zero Theme for nil.
func ThemeOf(d *syntax.ThemeDecl) theme.Theme {
	var th theme.Theme
	if d == nil {
		return th
	}
	put := func(m *map[string]string, k, v string) {
		if *m == nil {
			*m = map[string]string{}
		}
		(*m)[k] = v
	}
	text := func(v *syntax.DeployValue) string {
		if v.Kind != "list" {
			return v.Str
		}
		var fams []string
		for _, e := range v.List {
			fams = append(fams, e.Str)
		}
		return strings.Join(fams, ", ")
	}
	for _, f := range d.Fields {
		if f.Value == nil {
			continue
		}
		if (f.Name == "light" || f.Name == "dark") && f.Value.Kind == "record" {
			scheme := &th.Light
			if f.Name == "dark" {
				scheme = &th.Dark
			}
			for _, c := range f.Value.Rec {
				if c.Value != nil {
					put(scheme, c.Name, text(c.Value))
				}
			}
			continue
		}
		put(&th.Root, f.Name, text(f.Value))
	}
	return th
}

// ThemeInvalid is ThemeEdit's refusal: the E0042 diagnostics, located on the declaration
// (its span, or the insertion line when App has none). Nothing was printed or written.
type ThemeInvalid struct{ Diags []diag.Diagnostic }

// Error is the first message, "; +N more" when several.
func (e *ThemeInvalid) Error() string {
	if len(e.Diags) == 0 {
		return "the theme does not check"
	}
	msg := e.Diags[0].Message
	if n := len(e.Diags) - 1; n > 0 {
		msg += fmt.Sprintf("; +%d more", n)
	}
	return msg
}

// ThemeText is th's `theme { … }` declaration as ThemeEdit writes it (no trailing newline; ""
// for an empty theme), or a *ThemeInvalid (its diagnostics not located) when a field would not
// check.
func ThemeText(th theme.Theme) (string, error) {
	fs := themeFields(th)
	if ds := check.ThemeFields(fs); len(ds) > 0 {
		return "", &ThemeInvalid{Diags: ds}
	}
	return format.Theme(fs), nil
}

// themeFields are th's fields in theme.Tokens order: the top-level ones, then `light`, then
// `dark` (an empty record left out). A name that is not a token follows the tokens, sorted,
// for check.ThemeFields to refuse. Values are trimmed; a Font token's value is a list of its
// families.
func themeFields(th theme.Theme) []*syntax.DeployField {
	record := func(m map[string]string) []*syntax.DeployField {
		var fs []*syntax.DeployField
		add := func(name string, font bool) {
			v := strings.TrimSpace(m[name])
			val := &syntax.DeployValue{Kind: "string", Str: v}
			if font {
				val = &syntax.DeployValue{Kind: "list", List: []*syntax.DeployValue{}}
				for _, f := range theme.Families(v) {
					val.List = append(val.List, &syntax.DeployValue{Kind: "string", Str: strings.TrimSpace(f)})
				}
			}
			fs = append(fs, &syntax.DeployField{Name: name, Value: val})
		}
		known := map[string]bool{}
		for _, t := range theme.Tokens {
			known[t.Field] = true
			if _, ok := m[t.Field]; ok {
				add(t.Field, t.Kind == theme.Font)
			}
		}
		var rest []string
		for k := range m {
			if !known[k] {
				rest = append(rest, k)
			}
		}
		slices.Sort(rest)
		for _, k := range rest {
			add(k, false)
		}
		return fs
	}
	fs := record(th.Root)
	for _, s := range []struct {
		name string
		m    map[string]string
	}{{"light", th.Light}, {"dark", th.Dark}} {
		if len(s.m) > 0 {
			fs = append(fs, &syntax.DeployField{Name: s.name, Value: &syntax.DeployValue{Kind: "record", Rec: record(s.m)}})
		}
	}
	return fs
}

// isThemeDecl is a theme edit's Guard.
func isThemeDecl(d syntax.Decl) bool { _, ok := d.(*syntax.ThemeDecl); return ok }

// ThemeEdit is the edit of App.alder that makes its `theme` declaration th: the declaration's
// lines replaced by th's print, or the print inserted with a blank line each side after the
// `deploy` declaration, else after `session`, else after the last import, else after the
// `module App` line; an empty th deletes the declaration and one blank line next to it (none
// when App has none: no edits). App is parsed from the source p loaded (WriteTheme loads it
// fresh), and a value that would not check is a *ThemeInvalid.
func ThemeEdit(p *Program, th theme.Theme) ([]Edit, error) {
	app := p.Modules["App"]
	if app == nil {
		return nil, Refusef("there is no module `App`: `alder new <app>` writes one")
	}
	m, diags := syntax.ParseFile(app.File, []byte(app.Source))
	if m == nil || len(diags) > 0 {
		lines := strings.Split(app.Source, "\n")
		if m != nil {
			lines = m.Lines
		}
		return nil, &Refusal{Msg: "`" + app.File + "` does not parse (the errors above): fix it first, then write the theme", Diags: diags, Sources: map[string][]string{app.File: lines}}
	}
	var decl *syntax.ThemeDecl
	var deploy, session syntax.Decl
	for _, d := range m.Decls {
		switch d := d.(type) {
		case *syntax.ThemeDecl:
			if decl == nil {
				decl = d
			}
		case *syntax.DeployDecl:
			deploy = d
		case *syntax.SessionDecl:
			session = d
		}
	}
	e := newSrcEdit(app.Source)
	// after is the line the print goes after when App has no declaration
	after := 0
	switch {
	case deploy != nil:
		after = deploy.GetSpan().End.Line
	case session != nil:
		after = session.GetSpan().End.Line
	case len(m.Imports) > 0:
		for _, im := range m.Imports {
			after = max(after, im.Span.End.Line)
		}
	default:
		for n := 1; n <= len(e.lines); n++ {
			if strings.HasPrefix(e.line(n), "module ") {
				after = n
				break
			}
		}
	}

	text, err := ThemeText(th)
	if inv := (*ThemeInvalid)(nil); errors.As(err, &inv) {
		at := diag.Span{File: app.File, Start: diag.Pos{Line: after + 1, Col: 1}, End: diag.Pos{Line: after + 1, Col: 1}}
		if decl != nil {
			at = decl.Span
			at.File = app.File
		}
		located := make([]diag.Diagnostic, len(inv.Diags))
		for i, d := range inv.Diags {
			d.Span = at
			located[i] = d
		}
		return nil, &ThemeInvalid{Diags: located}
	} else if err != nil {
		return nil, err
	}

	if decl == nil {
		if text == "" {
			return nil, nil
		}
		ins := "\n" + text
		if l := e.line(after + 1); after < len(e.lines) && strings.TrimSpace(l) != "" {
			ins += "\n"
		}
		e.insertBefore(after+1, ins)
		return []Edit{{Path: app.File, Body: e.result(), Guard: isThemeDecl}}, nil
	}

	// the text around the declaration on its first and last lines (a trailing comment) stays
	sp := decl.Span
	first, last := e.line(sp.Start.Line), e.line(sp.End.Line)
	head := first[:min(max(sp.Start.Col-1, 0), len(first))]
	tail := last[min(max(sp.End.Col-1, 0), len(last)):]
	for n := sp.Start.Line + 1; n <= sp.End.Line; n++ {
		e.deleteLine(n)
	}
	if text != "" {
		e.replaceLine(sp.Start.Line, head+text+tail)
		return []Edit{{Path: app.File, Body: e.result(), Guard: isThemeDecl}}, nil
	}
	// deleted, with one blank line next to it: the one after, else the one right above
	if rest := strings.TrimSpace(head + tail); rest != "" {
		e.replaceLine(sp.Start.Line, strings.TrimRight(head+tail, " "))
	} else {
		e.deleteLine(sp.Start.Line)
		blank := func(n int) bool { return n >= 1 && n <= len(e.lines) && strings.TrimSpace(e.line(n)) == "" }
		switch {
		case blank(sp.End.Line + 1):
			e.deleteLine(sp.End.Line + 1)
		case blank(sp.Start.Line - 1):
			e.deleteLine(sp.Start.Line - 1)
		}
	}
	// no Guard: the declaration is gone, and Apply refuses a Guard that matches nothing
	return []Edit{{Path: app.File, Body: e.result()}}, nil
}

// WriteTheme is ThemeEdit + Apply for App (LoadWithErrors, WriteDespiteErrors): the app is
// loaded, and App.alder read, at each call, so a change made since in an editor is kept.
// Errors elsewhere in the app do not stop the write; one in the declaration refuses it.
func WriteTheme(app App, th theme.Theme, dryRun bool) (*Result, error) {
	return runEdits(app, dryRun, Options{WriteDespiteErrors: true}, LoadWithErrors, func(p *Program) ([]Edit, error) {
		return ThemeEdit(p, th)
	})
}
