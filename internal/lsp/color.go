package lsp

import (
	"encoding/json"
	"fmt"
	"math"
	"strings"

	"alder/internal/syntax"
	"alder/internal/theme"
)

// ColorInformation is a swatch: a colour and the range of the text that spells it.
type ColorInformation struct {
	Range Range `json:"range"`
	Color Color `json:"color"`
}

// Color is an LSP colour: gamma-encoded sRGB channels, each 0–1.
type Color struct {
	Red   float64 `json:"red"`
	Green float64 `json:"green"`
	Blue  float64 `json:"blue"`
	Alpha float64 `json:"alpha"`
}

// ColorPresentation is one way to write a picked colour.
type ColorPresentation struct {
	Label    string    `json:"label"`
	TextEdit *TextEdit `json:"textEdit,omitempty"`
}

type documentColorParams struct {
	TextDocument struct {
		URI string `json:"uri"`
	} `json:"textDocument"`
}

type colorPresentationParams struct {
	TextDocument struct {
		URI string `json:"uri"`
	} `json:"textDocument"`
	Color Color `json:"color"`
	Range Range `json:"range"`
}

// documentColorReq answers textDocument/documentColor: a swatch for each colour string of the
// `light` and `dark` records of App's theme declaration, read from the document's current text.
// Anything else (another module, no theme, a syntax error, text that is not a colour) gives an
// empty list: the checker owns errors.
func documentColorReq(s *server, params json.RawMessage) (any, error) {
	var p documentColorParams
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, &rpcError{codeInvalidParams, err.Error()}
	}
	out := []ColorInformation{}
	rel, ok := s.rel(p.TextDocument.URI)
	if !ok || !s.isApp(rel) {
		return out, nil
	}
	m, ds := syntax.ParseFile(rel, []byte(s.text(rel)))
	for _, d := range ds {
		if d.Severity == "error" {
			return out, nil
		}
	}
	for _, d := range m.Decls {
		if td, ok := d.(*syntax.ThemeDecl); ok {
			for _, f := range td.Fields {
				if (f.Name == "light" || f.Name == "dark") && f.Value != nil && f.Value.Kind == "record" {
					out = append(out, s.schemeColours(m.Lines, f.Name, f.Value.Rec)...)
				}
			}
		}
	}
	return out, nil
}

// isApp reports whether rel is the app's App module, in the latest analysis or the last good one.
func (s *server) isApp(rel string) bool {
	for _, a := range []*analysis{s.current(), s.lastGood()} {
		if a == nil || a.prog == nil {
			continue
		}
		if app := a.prog.Modules["App"]; app != nil {
			return app.File == rel
		}
	}
	return false
}

// schemeColours are the swatches of one scheme record (scheme is "light" or "dark").
func (s *server) schemeColours(lines []string, scheme string, rec []*syntax.DeployField) []ColorInformation {
	own := map[string]string{} // field → its string value, for var()
	for _, f := range rec {
		if f.Value != nil && f.Value.Kind == "string" {
			own[f.Name] = f.Value.Str
		}
	}
	var out []ColorInformation
	for _, f := range rec {
		tok, ok := theme.Lookup(f.Name)
		if !ok || tok.Kind != theme.Colour || f.Value == nil || f.Value.Kind != "string" {
			continue
		}
		c, ok := resolveColour(f.Value.Str, scheme, own, map[string]bool{tok.CSS: true})
		if !ok {
			continue
		}
		r := spanRange(f.Value.Span) // over the quotes; the contents are one byte in from each
		r.Start.Character++
		r.End.Character--
		out = append(out, ColorInformation{Range: s.enc.wireRange(lines, r), Color: Color{c.R, c.G, c.B, c.A}})
	}
	return out
}

// resolveColour parses text, following a `var(--x)` to the record's own value for x, else
// alder-ui's default for the scheme. seen holds the CSS names already followed: a cycle gives
// no colour.
func resolveColour(text, scheme string, own map[string]string, seen map[string]bool) (theme.RGBA, bool) {
	t := strings.TrimSpace(text)
	if len(t) < len("var()") || !strings.EqualFold(t[:4], "var(") || t[len(t)-1] != ')' {
		return theme.ParseColour(t)
	}
	name := strings.TrimSpace(t[4 : len(t)-1])
	if !strings.HasPrefix(name, "--") || seen[name] {
		return theme.RGBA{}, false
	}
	seen[name] = true
	if tok, ok := theme.ByCSS(name); ok {
		if v, ok := own[tok.Field]; ok {
			return resolveColour(v, scheme, own, seen)
		}
	}
	d, ok := theme.Defaults()[name]
	if !ok {
		return theme.RGBA{}, false
	}
	v := d.Light
	if scheme == "dark" {
		v = d.Dark
	}
	return resolveColour(v, scheme, own, seen)
}

// colorPresentationReq answers textDocument/colorPresentation: the picked colour as hex
// (`#rrggbb`, or `#rrggbbaa` when it is not opaque), replacing the given range.
func colorPresentationReq(s *server, params json.RawMessage) (any, error) {
	var p colorPresentationParams
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, &rpcError{codeInvalidParams, err.Error()}
	}
	hex := colourHex(p.Color)
	return []ColorPresentation{{Label: hex, TextEdit: &TextEdit{Range: p.Range, NewText: hex}}}, nil
}

// colourHex is c as lower-case hex, each channel rounded to the nearest of 0–255.
func colourHex(c Color) string {
	b := func(v float64) int { return int(math.Round(math.Min(math.Max(v, 0), 1) * 255)) }
	hex := fmt.Sprintf("#%02x%02x%02x", b(c.Red), b(c.Green), b(c.Blue))
	if c.Alpha < 1 {
		hex += fmt.Sprintf("%02x", b(c.Alpha))
	}
	return hex
}
