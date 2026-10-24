// Package format holds the document IR and width-aware renderer that the
// Alder pretty-printer is built on (a Wadler/Lindig style "prettier").
//
// Semantics:
//   - Line renders as a space when its enclosing group is flat, and as a
//     newline plus indentation when the group is broken.
//   - SoftLine renders as nothing when flat, a newline plus indentation when
//     broken.
//   - HardLine is always a newline and forces every enclosing group broken.
//     A Text containing "\n" behaves the same way.
//   - Nest adds IndentWidth to the indentation of newlines inside it.
//   - Flat prints its content flat whatever the width, unless it holds a hard break.
//   - IfBreak picks its first argument when the innermost enclosing group is
//     broken (the top level counts as broken), otherwise its second.
//   - A group is flat iff it contains no hard break and the "fits rest" rule
//     holds: its flat content, plus everything that follows it up to the next
//     possible line break, fits in the remaining width. Following content is
//     measured in the mode it will actually be printed in: a Line or SoftLine
//     that is in a broken enclosing group ends the measurement (it fits), and
//     a not-yet-decided group that follows is measured up to its first
//     possible break (as Prettier does).
//
// Hard-break containment and flat widths are computed once, when a node is
// built, so rendering is linear apart from the bounded lookahead in fits.
// Rendering uses an explicit stack, so document depth is not limited by the
// goroutine stack. Widths count runes. Indentation is written lazily, and
// blanks before a line break are trimmed (except those a text holding "\n"
// wrote, which is verbatim), so no line carries trailing whitespace.
package format

import (
	"strings"
	"unicode/utf8"
)

const (
	// Width is the target line width.
	Width = 100
	// IndentWidth is the number of spaces Nest adds.
	IndentWidth = 2
)

// Doc is a document node. Build values only with the constructors in this
// package.
type Doc interface{}

type text struct {
	s     string
	w     int  // rune width; for hard text, width of the part before the first newline
	hard  bool // contains "\n"
	lastW int  // rune width after the last newline (hard text only)
}

type concat struct {
	ds   []Doc
	flat int
	hard bool
}

type nest struct {
	d    Doc
	flat int
	hard bool
}

type group struct {
	d    Doc
	flat int
	hard bool
}

type flat struct {
	d    Doc
	w    int
	hard bool
}

type ifBreak struct {
	broken, flat Doc
	flatW        int
	hard         bool // of the flat branch: that is what a flat group prints
}

type lineKind int

const (
	kLine lineKind = iota
	kSoft
	kHard
)

type line struct{ kind lineKind }

var (
	// Line is a space when flat, a newline when broken.
	Line Doc = line{kLine}
	// SoftLine is nothing when flat, a newline when broken.
	SoftLine Doc = line{kSoft}
	// HardLine is always a newline and forces enclosing groups broken.
	HardLine Doc = line{kHard}
)

// Text is literal text. A "\n" inside it is a hard break.
func Text(s string) Doc {
	i := strings.IndexByte(s, '\n')
	if i < 0 {
		return text{s: s, w: utf8.RuneCountInString(s)}
	}
	j := strings.LastIndexByte(s, '\n')
	return text{s: s, w: utf8.RuneCountInString(s[:i]), hard: true, lastW: utf8.RuneCountInString(s[j+1:])}
}

// Concat joins documents.
func Concat(ds ...Doc) Doc {
	switch len(ds) {
	case 0:
		return text{}
	case 1:
		return ds[0]
	}
	c := concat{ds: append([]Doc(nil), ds...)}
	for _, d := range c.ds {
		w, h := measure(d)
		c.flat += w
		c.hard = c.hard || h
	}
	return c
}

// Nest indents newlines inside d by IndentWidth.
func Nest(d Doc) Doc {
	w, h := measure(d)
	return nest{d: d, flat: w, hard: h}
}

// Group prints d flat if it fits, otherwise broken.
func Group(d Doc) Doc {
	w, h := measure(d)
	return group{d: d, flat: w, hard: h}
}

// Flat prints d with every group in it flat, however long the line, unless d holds a hard
// break: then d is printed as it would be without Flat.
func Flat(d Doc) Doc {
	w, h := measure(d)
	return flat{d: d, w: w, hard: h}
}

// IfBreak prints broken when the enclosing group is broken, else flat.
func IfBreak(broken, flat Doc) Doc {
	w, h := measure(flat)
	return ifBreak{broken: broken, flat: flat, flatW: w, hard: h}
}

// measure returns the flat width of d and whether it contains a hard break.
func measure(d Doc) (int, bool) {
	switch n := d.(type) {
	case text:
		return n.w, n.hard
	case concat:
		return n.flat, n.hard
	case nest:
		return n.flat, n.hard
	case group:
		return n.flat, n.hard
	case ifBreak:
		return n.flatW, n.hard
	case flat:
		return n.w, n.hard
	case line:
		switch n.kind {
		case kLine:
			return 1, false
		case kSoft:
			return 0, false
		}
		return 0, true
	}
	return 0, false
}

type cmd struct {
	indent int
	flat   bool
	d      Doc
}

// Render prints d, breaking groups that do not fit within width columns.
func Render(d Doc, width int) string {
	var out []byte
	stack := []cmd{{0, false, d}}
	col := 0
	pending := 0  // indentation owed to the next non-empty text
	verbatim := 0 // out[:verbatim] ends with a text holding "\n": its blanks are kept
	for len(stack) > 0 {
		c := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		switch n := c.d.(type) {
		case text:
			if n.s == "" {
				continue
			}
			if pending > 0 {
				out = append(out, strings.Repeat(" ", pending)...)
				pending = 0
			}
			out = append(out, n.s...)
			if n.hard {
				col = n.lastW
				verbatim = len(out)
			} else {
				col += n.w
			}
		case concat:
			for i := len(n.ds) - 1; i >= 0; i-- {
				stack = append(stack, cmd{c.indent, c.flat, n.ds[i]})
			}
		case nest:
			stack = append(stack, cmd{c.indent + IndentWidth, c.flat, n.d})
		case group:
			flat := c.flat || (!n.hard && fits(n, width-col, stack))
			stack = append(stack, cmd{c.indent, flat, n.d})
		case flat:
			stack = append(stack, cmd{c.indent, c.flat || !n.hard, n.d})
		case ifBreak:
			if c.flat {
				stack = append(stack, cmd{c.indent, c.flat, n.flat})
			} else {
				stack = append(stack, cmd{c.indent, c.flat, n.broken})
			}
		case line:
			switch {
			case n.kind == kLine && c.flat:
				if pending > 0 {
					out = append(out, strings.Repeat(" ", pending)...)
					pending = 0
				}
				out = append(out, ' ')
				col++
			case n.kind == kSoft && c.flat:
			default:
				// no trailing blanks, except a verbatim text's own
				for len(out) > verbatim && (out[len(out)-1] == ' ' || out[len(out)-1] == '\t') {
					out = out[:len(out)-1]
				}
				out = append(out, '\n')
				pending = c.indent
				col = c.indent
			}
		}
	}
	return string(out)
}

// fits reports whether g printed flat, followed by the rest of the stack up to
// the first possible line break, fits in rem columns.
func fits(g group, rem int, rest []cmd) bool {
	if g.flat > rem {
		return false
	}
	local := []cmd{{0, true, g.d}}
	ri := len(rest) - 1
	for rem >= 0 {
		var c cmd
		if len(local) > 0 {
			c = local[len(local)-1]
			local = local[:len(local)-1]
		} else if ri >= 0 {
			c = rest[ri]
			ri--
		} else {
			return true
		}
		switch n := c.d.(type) {
		case text:
			rem -= n.w
			if n.hard {
				return rem >= 0
			}
		case concat:
			for i := len(n.ds) - 1; i >= 0; i-- {
				local = append(local, cmd{0, c.flat, n.ds[i]})
			}
		case nest:
			local = append(local, cmd{0, c.flat, n.d})
		case group:
			local = append(local, cmd{0, c.flat && !n.hard, n.d})
		case flat:
			local = append(local, cmd{0, c.flat || !n.hard, n.d})
		case ifBreak:
			if c.flat {
				local = append(local, cmd{0, true, n.flat})
			} else {
				local = append(local, cmd{0, false, n.broken})
			}
		case line:
			if n.kind == kHard || !c.flat {
				return true
			}
			if n.kind == kLine {
				rem--
			}
		}
	}
	return false
}
