package diag

import (
	"encoding/json"
	"fmt"
	"strings"
)

func RenderText(d Diagnostic, lines []string) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "%s:%d:%d: %s[%s]: %s\n", d.Span.File, d.Span.Start.Line, d.Span.Start.Col, d.Severity, d.Code, d.Message)
	if l := d.Span.Start.Line; !d.NoSource && l >= 1 && l <= len(lines) {
		src := lines[l-1]
		fmt.Fprintf(&sb, "  %s\n", src)
		width := 1
		if d.Span.End.Line == l && d.Span.End.Col > d.Span.Start.Col {
			width = d.Span.End.Col - d.Span.Start.Col
		}
		fmt.Fprintf(&sb, "  %s%s\n", strings.Repeat(" ", max(0, d.Span.Start.Col-1)), strings.Repeat("^", width))
	}
	if d.Fix != nil {
		fmt.Fprintf(&sb, "  fix: %s\n", d.Fix.Replace)
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
