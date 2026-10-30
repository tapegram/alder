package diag

import (
	"io"
	"strings"
	"testing"

	"alder/internal/style"
)

func styledForTest() style.Style {
	return style.ForEnv(io.Discard, func(k string) string {
		if k == "ALDER_COLOR" {
			return "always"
		}
		return ""
	})
}

func renderCases() []Diagnostic {
	sp := Span{File: "app/X.alder", Start: Pos{2, 5}, End: Pos{2, 8}}
	return []Diagnostic{
		{Code: "E0003", Severity: "error", Span: sp, Message: "expected Text, got Int", Fix: ReplaceSpan(sp, "Text.fromInt (1)")},
		{Code: "W0002", Severity: "warning", Span: sp, Message: "a hole", NoSource: true},
		{Severity: "error", Span: sp, Message: "scratch's own, no code"},
	}
}

func TestRenderTextStyled_StripEqualsPlain(t *testing.T) {
	lines := []string{"module X", "x = 123"}
	st := styledForTest()
	for _, d := range renderCases() {
		styled := RenderTextStyled(d, lines, st)
		if style.Strip(styled) != RenderText(d, lines) {
			t.Errorf("Strip(styled) != plain for %s:\n%q\n%q", d.Code, style.Strip(styled), RenderText(d, lines))
		}
		if RenderTextStyled(d, lines, style.Plain) != RenderText(d, lines) {
			t.Errorf("Plain differs from RenderText for %s", d.Code)
		}
	}
}

func TestRenderTextStyled_Colours(t *testing.T) {
	lines := []string{"module X", "x = 123"}
	st := styledForTest()
	d := renderCases()[0]
	out := RenderTextStyled(d, lines, st)
	for _, want := range []string{
		st.Bold("app/X.alder:2:5:"),
		st.BoldRed("error[E0003]"),
		st.BoldRed("^^^"),
		st.Cyan("fix:"),
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in %q", want, out)
		}
	}
	if !strings.Contains(out, ": expected Text, got Int\n") || !strings.Contains(out, "  x = 123\n") {
		t.Errorf("message and source must stay plain: %q", out)
	}
	w := RenderTextStyled(renderCases()[1], lines, st)
	if !strings.Contains(w, st.Yellow("warning[W0002]")) {
		t.Errorf("warning not yellow: %q", w)
	}
}
