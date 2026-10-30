package diag

import (
	"encoding/json"
	"fmt"
	"strings"

	"alder/internal/style"
)

// RenderText is d as text, `file:line:col: severity[CODE]: message`, with its source line and
// a caret under the span; a diagnostic with no code (scratch's own) drops the brackets.
func RenderText(d Diagnostic, lines []string) string { return RenderTextStyled(d, lines, style.Plain) }

// RenderTextStyled is RenderText with colour and weight from st: the location bold, the
// severity word bold red (error) or yellow (warning), the caret line in the severity's colour,
// `fix:` cyan; the message and the source line stay plain. With style.Plain it is RenderText.
func RenderTextStyled(d Diagnostic, lines []string, st style.Style) string {
	var sb strings.Builder
	sev := d.Severity
	if d.Code != "" {
		sev += "[" + d.Code + "]"
	}
	loc := fmt.Sprintf("%s:%d:%d:", d.Span.File, d.Span.Start.Line, d.Span.Start.Col)
	fmt.Fprintf(&sb, "%s %s: %s\n", st.Bold(loc), st.Severity(d.Severity, sev), d.Message)
	if l := d.Span.Start.Line; !d.NoSource && l >= 1 && l <= len(lines) {
		src := lines[l-1]
		fmt.Fprintf(&sb, "  %s\n", src)
		width := 1
		if d.Span.End.Line == l && d.Span.End.Col > d.Span.Start.Col {
			width = d.Span.End.Col - d.Span.Start.Col
		}
		fmt.Fprintf(&sb, "  %s%s\n", strings.Repeat(" ", max(0, d.Span.Start.Col-1)), st.Severity(d.Severity, strings.Repeat("^", width)))
	}
	if d.Fix != nil {
		fmt.Fprintf(&sb, "  %s %s\n", st.Cyan("fix:"), d.Fix.Replace)
	}
	return sb.String()
}

type jsonFix struct {
	Line    int    `json:"line"`
	Col     int    `json:"col"`
	EndLine int    `json:"endLine"`
	EndCol  int    `json:"endCol"`
	Replace string `json:"replace"`
}
type jsonDiag struct {
	Code     string   `json:"code"`
	Severity string   `json:"severity"`
	File     string   `json:"file"`
	Line     int      `json:"line"`
	Col      int      `json:"col"`
	EndLine  int      `json:"endLine"`
	EndCol   int      `json:"endCol"`
	Message  string   `json:"message"`
	Fix      *jsonFix `json:"fix,omitempty"`
}

func RenderJSON(d Diagnostic) string {
	j := jsonDiag{d.Code, d.Severity, d.Span.File, d.Span.Start.Line, d.Span.Start.Col, d.Span.End.Line, d.Span.End.Col, d.Message, nil}
	if d.Fix != nil {
		j.Fix = &jsonFix{d.Fix.Line, d.Fix.Col, d.Fix.EndLine, d.Fix.EndCol, d.Fix.Replace}
	}
	// SetEscapeHTML(false): messages quote code such as `|>` and `<Input>`, which must stay readable.
	var sb strings.Builder
	enc := json.NewEncoder(&sb)
	enc.SetEscapeHTML(false)
	enc.Encode(j)
	return strings.TrimSuffix(sb.String(), "\n")
}
