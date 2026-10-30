package scratch

import (
	"fmt"
	"strings"

	"alder/internal/diag"
	"alder/internal/format"
	"alder/internal/snippets"
)

// formatModule is the module line Format puts before a scratch file's source that has none, so the
// formatter reads it as a module; it is taken off again.
const formatModule = "module Scratch"

// Format formats the scratch file named name: its declarations as `alder fmt` formats a module,
// each watch's expression as the right-hand side of a definition (written back as `> expr`), the
// `in` header first after any comments above it. Watches that were on consecutive lines stay on
// consecutive lines; everything else is spaced as in a module. A file that does not parse yields
// its diagnostics, positioned in the file, and no output. The output must parse to the same
// watches and format to the same module; otherwise Format returns an error wrapping
// format.ErrUnsafe and no output.
func Format(name string, src []byte) ([]byte, []diag.Diagnostic, error) {
	f, pds := ParseFile(name, string(src))
	if len(pds) > 0 {
		return nil, pds, nil
	}
	modSrc, synth := formatSource(f)
	out, ds, err := format.Source(name, []byte(modSrc))
	if err != nil {
		return nil, nil, err
	}
	if out == nil {
		return nil, f.formatDiags(ds, synth), nil
	}
	text := f.unformat(string(out), synth)

	// the round trip: the output is a scratch file with the same watches and header, whose
	// module formats to what was just printed
	f2, pds2 := ParseFile(name, text)
	if len(pds2) > 0 || len(f2.Watches) != len(f.Watches) || f2.In != f.In {
		return nil, nil, fmt.Errorf("%s: %w", name, format.ErrUnsafe)
	}
	src2, _ := formatSource(f2)
	if out2, _, err := format.Source(name, []byte(src2)); err != nil || string(out2) != string(out) {
		return nil, nil, fmt.Errorf("%s: %w", name, format.ErrUnsafe)
	}
	return []byte(text), nil, nil
}

// formatSource is f's module source as the formatter reads it: with formatModule first when it
// has no `module` line (synth).
func formatSource(f File) (string, bool) {
	if modLine, _, _ := headerLines(f.Source); modLine >= 0 {
		return f.Source, false
	}
	return formatModule + "\n" + f.Source, true
}

// formatDiags positions the formatter's diagnostics of f's module source (with formatModule
// first when synth) in the file, as RunFile positions the checker's.
func (f File) formatDiags(ds []diag.Diagnostic, synth bool) []diag.Diagnostic {
	mapLine := func(d diag.Diagnostic) (diag.Diagnostic, bool) {
		if !synth {
			return d, true
		}
		if d.Span.Start.Line <= 1 || d.Span.End.Line <= 1 {
			return d, false // the synthesized module line
		}
		d.Span.Start.Line--
		d.Span.End.Line--
		if d.Fix != nil {
			fx := *d.Fix
			fx.Line, fx.EndLine = fx.Line-1, fx.EndLine-1
			d.Fix = &fx
		}
		return d, true
	}
	out := make([]diag.Diagnostic, 0, len(ds))
	for _, d := range ds {
		if inc, ok := f.incomplete(d, mapLine); ok {
			d = inc
		} else {
			d = f.Map.Position(d, mapLine, f.Name)
			d.Message = itPrefixRE.ReplaceAllString(d.Message, "")
		}
		out = append(out, d)
	}
	return out
}

// unformat turns the formatted module source of f back into a scratch file: the synthesized
// module line goes, each watch's `itK = ` becomes `> `, watches that were adjacent are again, and
// the `in` header returns after the comments that were above it.
func (f File) unformat(out string, synth bool) string {
	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	if synth {
		lines = lines[1:]
		for len(lines) > 0 && strings.TrimSpace(lines[0]) == "" {
			lines = lines[1:]
		}
	}
	// watches, in file order
	starts := make([]int, len(f.Watches)) // each watch's line index in lines
	next := 0
	for i := 0; i < len(lines) && next < len(f.Watches); i++ {
		name := f.Watches[next].Name
		switch {
		case strings.HasPrefix(lines[i], name+" = "):
			lines[i] = "> " + lines[i][len(name+" = "):]
		case lines[i] == name+" =" && i+1 < len(lines):
			lines[i] = "> " + strings.TrimLeft(lines[i+1], " \t")
			lines = append(lines[:i+1], lines[i+2:]...)
		default:
			continue
		}
		starts[next] = i
		next++
	}
	// adjacent watches: drop the blank lines the formatter put between them
	item := snippets.Starts(lines)
	drop := map[int]bool{}
	for k := 1; k < next; k++ {
		if f.Watches[k].Line != f.Watches[k-1].End+1 {
			continue
		}
		between := false
		for i := starts[k-1] + 1; i < starts[k]; i++ {
			if item[i] && strings.TrimSpace(lines[i]) != "" {
				between = true
			}
		}
		if between {
			continue
		}
		for i := starts[k] - 1; i > starts[k-1] && strings.TrimSpace(lines[i]) == ""; i-- {
			drop[i] = true
		}
	}
	kept := lines[:0:0]
	for i, l := range lines {
		if !drop[i] {
			kept = append(kept, l)
		}
	}
	lines = kept
	if f.In != "" {
		lines = withHeader(lines, "in "+f.In, f.commentsAboveHeader())
	}
	if len(lines) == 0 || len(lines) == 1 && lines[0] == "" {
		return ""
	}
	return strings.Join(lines, "\n") + "\n"
}

// commentsAboveHeader counts the comment lines above f's `in` header.
func (f File) commentsAboveHeader() int {
	n := 0
	for _, l := range f.Lines {
		t := strings.TrimSpace(l)
		switch {
		case t == "in" || strings.HasPrefix(t, "in ") || strings.HasPrefix(t, "in\t"):
			return n
		case strings.HasPrefix(t, "--"):
			n++
		}
	}
	return n
}

// withHeader is lines with header inserted after the first k comment lines (and the blank lines
// among them), followed by a blank line when more follows.
func withHeader(lines []string, header string, k int) []string {
	at := 0
	for seen := 0; at < len(lines) && seen < k; at++ {
		if strings.HasPrefix(strings.TrimSpace(lines[at]), "--") {
			seen++
		}
	}
	rest := lines[at:]
	for len(rest) > 0 && strings.TrimSpace(rest[0]) == "" {
		rest = rest[1:]
	}
	out := append(append([]string(nil), lines[:at]...), header)
	if len(rest) > 0 {
		out = append(append(out, ""), rest...)
	}
	return out
}
