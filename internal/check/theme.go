package check

import (
	"fmt"
	"strings"

	"alder/internal/diag"
	"alder/internal/syntax"
	"alder/internal/theme"
)

// The `theme` declaration (Appendix C, "Changing a token"): alder-ui tokens App sets, emitted as
// CSS after al.css (internal/theme). The checker holds every name to theme.Tokens and every
// value to its kind's shape (E0042), so nothing the build writes can break out of its
// declaration.

// checkTheme checks App's theme declarations (decls: every one App declares).
func (mc *modCtx) checkTheme(decls []*syntax.ThemeDecl) {
	if len(decls) == 0 {
		return
	}
	c := mc.c
	for _, extra := range decls[1:] {
		c.errorf(diag.ThemeDecl, extra.Span, "%s", "`theme` is declared once: merge this one into the first")
	}
	mc.themeFields(decls[0].Fields, "")
}

// themeFields checks one record of a theme: the top level (scheme "") or `light`/`dark`.
func (mc *modCtx) themeFields(fs []*syntax.DeployField, scheme string) {
	c := mc.c
	bad := func(sp diag.Span, format string, args ...any) {
		c.errorf(diag.ThemeDecl, sp, "%s", fmt.Sprintf(format, args...))
	}
	in := "`theme`"
	if scheme != "" {
		in = "`" + scheme + "`"
	}
	seen := map[string]bool{}
	for _, f := range fs {
		if seen[f.Name] {
			bad(f.NameSpan, "`%s` is declared twice in %s: declare it once", f.Name, in)
			continue
		}
		seen[f.Name] = true
		v := f.Value
		if f.Name == "light" || f.Name == "dark" {
			switch {
			case scheme != "":
				bad(f.NameSpan, "`%s` is set at the top of `theme`, not inside `%s`", f.Name, scheme)
			case v != nil && v.Kind != "record":
				bad(v.Span, "`%s` is a record of colours: `%s: { primary: \"…\" }`", f.Name, f.Name)
			case v != nil:
				mc.themeFields(v.Rec, f.Name)
			}
			continue
		}
		tok, ok := theme.Lookup(f.Name)
		if !ok {
			if r, removed := theme.Removed[f.Name]; removed {
				bad(f.NameSpan, "`%s` was removed in alder-ui v0.4: %s", f.Name, r)
			} else if s, near := theme.Suggest(f.Name); near {
				bad(f.NameSpan, "`%s` is not a token of alder-ui %s; did you mean `%s` (`%s`)?", f.Name, theme.Version(), s.Field, s.CSS)
			} else {
				bad(f.NameSpan, "`%s` is not a token of alder-ui %s; the tokens are listed in Appendix C", f.Name, theme.Version())
			}
			continue
		}
		switch {
		case tok.Kind == theme.Colour && scheme == "":
			bad(f.NameSpan, "`%s` is a colour: set it under `light` or `dark`", f.Name)
			continue
		case tok.Kind != theme.Colour && scheme != "":
			bad(f.NameSpan, "`%s` is theme-wide: set it outside `light` and `dark`", f.Name)
			continue
		}
		if v != nil {
			if msg := themeValue(tok, v); msg != "" {
				bad(v.Span, "%s", msg)
			}
		}
	}
}

// themeValue is the refusal of v as tok's value, "" when it is of tok's kind's shape.
func themeValue(tok theme.Token, v *syntax.DeployValue) string {
	text := strings.TrimSpace(v.Str)
	switch tok.Kind {
	case theme.Colour:
		shape := fmt.Sprintf("`%s` is a colour written as text: `\"oklch(0.55 0.22 285)\"`, `\"#4f46e5\"` or `\"var(--accent)\"`", tok.Field)
		if v.Kind != "string" {
			return shape
		}
		if name, ok := theme.Alias(text); ok {
			t, known := theme.ByCSS(name)
			switch {
			case !known:
				return fmt.Sprintf("`var(%s)` is not a token of alder-ui %s: a colour may alias a colour token, such as `\"var(--accent)\"`", name, theme.Version())
			case t.Kind != theme.Colour:
				return fmt.Sprintf("`var(%s)` is a %s, not a colour", name, t.Kind)
			}
			return ""
		}
		if !theme.ColourOK(text) {
			return shape
		}
	case theme.Length:
		shape := fmt.Sprintf("`%s` is a length written as text, such as `\"0.5rem\"`", tok.Field)
		switch {
		case v.Kind != "string":
			return shape
		case theme.NegativeLength(text):
			return fmt.Sprintf("`%s` is a length and cannot be negative: write one such as `\"0.5rem\"`", tok.Field)
		case !theme.LengthOK(text):
			return shape
		}
	case theme.Font:
		shape := fmt.Sprintf("`%s` is a list of family names, such as `[\"Inter\", \"sans-serif\"]`", tok.Field)
		if v.Kind != "list" || len(v.List) == 0 {
			return shape
		}
		for _, e := range v.List {
			if e.Kind != "string" || !theme.FamilyOK(e.Str) {
				return shape + ": each a non-empty text without `\"`, `;`, `{`, `}` or a line break"
			}
		}
	case theme.Shadow:
		if v.Kind != "string" || !theme.ShadowOK(text) {
			return fmt.Sprintf("`%s` is a shadow written as text, such as `\"0 1px 2px 0 rgb(0 0 0 / 0.05)\"` or `\"none\"`", tok.Field)
		}
	}
	return ""
}
