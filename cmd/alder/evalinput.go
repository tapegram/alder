package main

import (
	"errors"
	"fmt"
	"strings"

	"alder/internal/snippets"
)

// evalSource is the scratch module `alder eval` (and the REPL) compiles for an input, and how
// to map its positions back to the input's.
type evalSource struct {
	src   string
	lines []string // the input's lines (set by the caller of assemble: evalModule sets them)
	// For line i+1 of src: inLine[i] is the input's line it comes from, and shift[i] is what to
	// add to a column on it to get the input's column (the expression's first line gains
	// `it = `, a let line loses its indentation and `let `, an annotated let's body line its
	// annotation).
	inLine, shift []int
}

// errNothing is an input with no expression.
var errNothing = errors.New("nothing to evaluate")

// item is one input item: a `let` (Let, Name) or the expression (Let false). Lines are the
// item's lines as they go into the scratch module (a let's `name = …`, preceded by `name : T`
// when annotated; the expression's `it = …`), followed by its continuation lines — the blank
// and `--` comment lines after it included, so the last item ends with the input's trailing
// ones — and preceded, for the input's first item, by the blank and `--` comment lines before
// it. InLine[i] is the line of the text parsed (1-based) that Lines[i] comes from — the
// input's line for eval; for the REPL, which parses each entry on its own, the line within
// that entry — and Shift[i] what to add to a column on it to get that line's column.
type item struct {
	Let   bool
	Name  string
	Lines []string

	InLine, Shift []int
}

// parseItems splits input (several lines) into its items: zero or more lets then at most one
// expression. Where each item starts is snippets.Starts: the first on the first line that is
// not blank or a `--` comment (its indentation is ignored), every later one on a line at column
// 1 that is not blank, a comment or a closer (`}`, `)`, `]`, `</tag>`, which continue the item
// before them, so a multi-line `match` or element pastes as written). Every item but the last
// must be `let name = expr` (or `let name : Type = expr`); the last is the expression unless it
// starts with `let `, in which case the input has none and parseItems returns its lets with
// errNothing (the REPL checks a `let` alone). An input with no item at all is errNothing too.
// who names the tool in the messages ("eval", "the REPL").
func parseItems(input, who string) ([]item, error) {
	lines := inputLines(input)
	starts := itemStarts(lines)
	if len(starts) == 0 {
		return nil, errNothing
	}
	last := strings.TrimLeft(lines[starts[len(starts)-1]], " \t")
	if !strings.HasPrefix(last, "let ") {
		return splitItems(lines, starts, true, who)
	}
	items, err := splitItems(lines, starts, false, who)
	if err != nil {
		return nil, err
	}
	return items, errNothing
}

// evalModule turns the input into the source of the scratch module: parseItems and assemble,
// with eval's one difference: the last item is always the expression, a `let` too (which then
// fails to parse, as a diagnostic positioned in the input). Each let item becomes the top-level
// `name = expr` (an annotated one also the line `name : Type` before it), the expression
// `it = expr`; unannotated values have no signature, and the checker infers them.
func evalModule(input string) (evalSource, error) {
	lines := inputLines(input)
	es := evalSource{lines: lines}
	starts := itemStarts(lines)
	if len(starts) == 0 {
		return es, errNothing
	}
	items, err := splitItems(lines, starts, true, "eval")
	if err != nil {
		return es, err
	}
	es = assemble(items)
	es.lines = lines
	return es, nil
}

// assemble joins items into the scratch module's source (no module header: scratch adds it)
// and the position map evalDiags uses (the input's lines, evalSource.lines, are the caller's
// to set). Items without an expression (the REPL's check of a `let` alone) give a module with
// no `it`.
func assemble(items []item) evalSource {
	var es evalSource
	var out []string
	for _, it := range items {
		out = append(out, it.Lines...)
		es.inLine = append(es.inLine, it.InLine...)
		es.shift = append(es.shift, it.Shift...)
	}
	es.src = strings.Join(out, "\n") + "\n"
	return es
}

// inputLines is the input's lines, CRLF as LF and trailing newlines dropped.
func inputLines(input string) []string {
	return strings.Split(strings.TrimRight(strings.ReplaceAll(input, "\r\n", "\n"), "\n"), "\n")
}

// itemStarts is the first line (0-based) of each item of lines (snippets.Starts).
func itemStarts(lines []string) []int {
	var starts []int
	for i, st := range snippets.Starts(lines) {
		if st {
			starts = append(starts, i)
		}
	}
	return starts
}

// splitItems makes the items of lines, which start at starts (non-empty); exprLast makes the
// last one the expression, else every one must be a let. A let's name starts with a lowercase
// ASCII letter (an uppercase one would name a module or type, which the name would be shadowed
// by) and is not `it`; who names the tool in the messages.
func splitItems(lines []string, starts []int, exprLast bool, who string) ([]item, error) {
	items := make([]item, 0, len(starts))
	var cur *item    // the item the next line continues
	var pending item // the lines before the first item, which go with it
	emit := func(src string, in, shift int) {
		c := cur
		if c == nil {
			c = &pending
		}
		c.Lines, c.InLine, c.Shift = append(c.Lines, src), append(c.InLine, in+1), append(c.Shift, shift)
	}
	next := 0 // the next item
	for i, l := range lines {
		if next >= len(starts) || i != starts[next] {
			emit(l, i, 0)
			continue
		}
		next++
		items = append(items, item{})
		cur = &items[len(items)-1]
		if next == 1 {
			cur.Lines, cur.InLine, cur.Shift = pending.Lines, pending.InLine, pending.Shift
		}
		body := strings.TrimLeft(l, " \t")
		indent := len(l) - len(body)
		if next == len(starts) && exprLast {
			emit("it = "+body, i, indent-len("it = "))
			continue
		}
		rest, ok := strings.CutPrefix(body, "let ")
		if !ok {
			return nil, fmt.Errorf("input line %d: only `let name = expr` lines may come before the expression", i+1)
		}
		def := strings.TrimLeft(rest, " \t")
		at := indent + len(body) - len(def) // the input column (0-based) where def starts
		name := def[:len(def)-len(strings.TrimLeft(def, "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789_'"))]
		if def == "" || def[0] < 'a' || def[0] > 'z' {
			return nil, fmt.Errorf("input line %d: a let name starts with a lowercase letter (`let todo = …`)", i+1)
		}
		if name == "it" {
			return nil, fmt.Errorf("input line %d: a let may not be named `it`, the name %s gives the expression; let another name", i+1, who)
		}
		cur.Let, cur.Name = true, name
		if after := strings.TrimLeft(def[len(name):], " \t"); name != "" && strings.HasPrefix(after, ":") {
			eq := strings.Index(def, "=")
			if eq < 0 {
				return nil, fmt.Errorf("input line %d: an annotated let is `let name : Type = expr`", i+1)
			}
			emit(strings.TrimRight(def[:eq], " \t"), i, at)
			emit(name+" "+def[eq:], i, at+eq-len(name)-1)
			continue
		}
		emit(def, i, at)
	}
	return items, nil
}
