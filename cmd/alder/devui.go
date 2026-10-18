package main

import (
	"encoding/json"
	"fmt"
	"html"
	"os"
	"path/filepath"
	"strings"
)

// errState is what the dev server shows for a failure: structured diagnostics from the checker
// (kind "check"), or one block of text for a build failure ("build") or a crash of the app
// ("crash"). The zero value is "no error". "building" is the 502 page's placeholder while the
// first build runs; it is not an error.
type errState struct {
	kind  string
	text  string
	diags []diagItem
}

// diagItem is one checker error as `alder check --json` prints it, with the source excerpt and
// caret the text form shows.
type diagItem struct {
	Code     string `json:"code"`
	Severity string `json:"severity"`
	File     string `json:"file"`
	Line     int    `json:"line"`
	Col      int    `json:"col"`
	EndLine  int    `json:"endLine"`
	EndCol   int    `json:"endCol"`
	Message  string `json:"message"`
	Excerpt  string `json:"-"`
}

// count is the number of errors the strip announces: one per diagnostic, or one for a block of
// text.
func (e errState) count() int {
	if len(e.diags) > 0 {
		return len(e.diags)
	}
	if e.kind == "" || e.kind == "building" {
		return 0
	}
	return 1
}

// parseDiags reads the lines of `alder check --json` (out) and keeps the errors, each with the
// excerpt of its source line under dir. Lines that are not diagnostics (a text failure) are
// skipped, so an empty result means "fall back to the text".
func parseDiags(dir, out string) []diagItem {
	var ds []diagItem
	for _, line := range strings.Split(out, "\n") {
		var d diagItem
		if !strings.HasPrefix(line, "{") || json.Unmarshal([]byte(line), &d) != nil || d.Severity != "error" || d.Code == "" {
			continue
		}
		d.Excerpt = excerpt(dir, d)
		ds = append(ds, d)
	}
	return ds
}

// excerpt is the source line of d and a line of carets under its span ("" when the file or the
// line cannot be read). Tabs of the line are kept in the caret line's indent, so the carets
// stay under their characters.
func excerpt(dir string, d diagItem) string {
	b, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(d.File)))
	if err != nil {
		return ""
	}
	lines := strings.Split(string(b), "\n")
	if d.Line < 1 || d.Line > len(lines) {
		return ""
	}
	src := strings.TrimRight(lines[d.Line-1], "\r")
	pad := make([]byte, 0, d.Col)
	for i := 0; i < d.Col-1 && i < len(src); i++ {
		if src[i] == '\t' {
			pad = append(pad, '\t')
		} else {
			pad = append(pad, ' ')
		}
	}
	width := 1
	if d.EndLine == d.Line && d.EndCol > d.Col {
		width = d.EndCol - d.Col
	}
	return src + "\n" + string(pad) + strings.Repeat("^", width)
}

// stripText is the sentence on the strip at the top of the page.
func (e errState) stripText() string {
	if e.kind == "crash" {
		return "The app exited; there is no running app. Save a source file to rebuild it."
	}
	n := e.count()
	return fmt.Sprintf("Build failed: %d error%s. The page is the last good build.", n, map[bool]string{true: "", false: "s"}[n == 1])
}

// errorsHTML is the content patched into #_alder_errors (the container the router renders into
// every dev page): the strip, fixed to the top of the viewport (no layout shift), and the list
// of errors, which the dev panel copies into its Errors tab. "" for no error, which clears the
// container.
func errorsHTML(e errState) string {
	if e.kind == "" {
		return ""
	}
	var b strings.Builder
	b.WriteString("<style>" + errorsCSS + "</style>")
	if e.kind != "building" {
		fmt.Fprintf(&b, `<div class="_alder-strip" role="alert"><span>%s</span><button type="button" data-open-errors onclick="if(!document.getElementById('_alder_panel'))this.closest('#_alder_errors').toggleAttribute('data-show')">Show errors</button></div>`, html.EscapeString(e.stripText()))
	}
	fmt.Fprintf(&b, `<div class="_alder-errs" data-count="%d" data-kind="%s">`, e.count(), e.kind)
	if len(e.diags) > 0 {
		for _, d := range e.diags {
			fmt.Fprintf(&b, `<div class="_alder-diag"><div class="_alder-loc"><code class="_alder-at">%s:%d:%d</code> <b class="_alder-code">%s</b></div><p class="_alder-msg">%s</p>`,
				html.EscapeString(d.File), d.Line, d.Col, html.EscapeString(d.Code), html.EscapeString(d.Message))
			if d.Excerpt != "" {
				b.WriteString("<pre>" + html.EscapeString(d.Excerpt) + "</pre>")
			}
			b.WriteString("</div>")
		}
	} else {
		fmt.Fprintf(&b, `<div class="_alder-diag"><pre>%s</pre></div>`, html.EscapeString(strings.TrimRight(e.text, "\n")))
	}
	b.WriteString("</div>")
	return b.String()
}

// errorsCSS styles the strip and the list with the design system's tokens (fallbacks when
// al.css is not loaded: the 502 page, an app without it); dark and light follow the tokens.
// The list is hidden in the page itself: the panel shows it in its Errors tab; without a panel
// the strip's button shows it as a sheet (data-show); on the 502 page (data-page) it is the page.
const errorsCSS = `._alder-strip{position:fixed;inset:0 0 auto 0;z-index:2147483000;box-sizing:border-box;display:flex;gap:.75rem;align-items:center;justify-content:center;padding:.3rem .75rem;background:var(--destructive,#b91c1c);color:var(--destructive-foreground,#fff);font:12px/1.4 var(--al-font-mono,ui-monospace,SFMono-Regular,Menlo,monospace)}` +
	`._alder-strip button{margin:0;padding:0 .5rem;border:1px solid currentColor;border-radius:4px;background:transparent;color:inherit;font:inherit;cursor:pointer}` +
	`._alder-errs{display:block;font:12px/1.5 var(--al-font-mono,ui-monospace,SFMono-Regular,Menlo,monospace);color:var(--foreground,inherit)}` +
	`._alder-errs ._alder-diag{margin:0 0 .75rem;padding:.5rem .75rem;border-left:3px solid var(--destructive,#b91c1c);border-radius:0 4px 4px 0;background:var(--muted,rgba(127,127,127,.12))}` +
	`._alder-errs ._alder-loc{display:flex;gap:.75rem;align-items:baseline}` +
	`._alder-errs ._alder-at{font-weight:600}` +
	`._alder-errs ._alder-code{color:var(--destructive,#b91c1c)}` +
	`._alder-errs ._alder-msg{margin:.25rem 0}` +
	`._alder-errs pre{margin:.25rem 0 0;padding:0;background:transparent;white-space:pre;overflow:auto;font:inherit}` +
	`#_alder_errors ._alder-errs{display:none}` +
	`#_alder_errors[data-show] ._alder-errs{display:block;position:fixed;inset:auto 0 0 0;z-index:2147482999;box-sizing:border-box;max-block-size:50dvh;overflow:auto;padding:2.2rem .75rem .75rem;border-top:1px solid var(--border,#888);background:var(--background,#fff);color:var(--foreground,#111)}` +
	`#_alder_errors[data-page] ._alder-errs{display:block;position:static;max-block-size:none;padding:0;border:0;background:transparent}` +
	`#_alder_errors[data-page] ._alder-strip{display:none}` +
	`body:has(#_alder_panel) #_alder_errors ._alder-errs{display:none}`
