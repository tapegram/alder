package format

import (
	"strings"
	"testing"
	"time"
)

func braces() Doc {
	return Group(Concat(Text("{"), Nest(Concat(Line, Text("a: 1"))), Line, Text("}")))
}

func eq(t *testing.T, got, want string) {
	t.Helper()
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestRender_GroupFits(t *testing.T) {
	eq(t, Render(braces(), 100), "{ a: 1 }")
}

func TestRender_GroupBreaks(t *testing.T) {
	eq(t, Render(braces(), 5), "{\n  a: 1\n}")
}

func TestRender_ExactWidthFits(t *testing.T) {
	eq(t, Render(braces(), 8), "{ a: 1 }")
	eq(t, Render(braces(), 7), "{\n  a: 1\n}")
}

func TestRender_SoftLine(t *testing.T) {
	d := Group(Concat(Text("f("), Nest(Concat(SoftLine, Text("x"))), SoftLine, Text(")")))
	eq(t, Render(d, 100), "f(x)")
	eq(t, Render(d, 3), "f(\n  x\n)")
}

func TestRender_HardLinePropagates(t *testing.T) {
	inner := Group(Concat(Text("b"), HardLine, Text("c")))
	outer := Group(Concat(Text("a"), Line, inner, Line, Text("d")))
	eq(t, Render(outer, 100), "a\nb\nc\nd")
}

func TestRender_HardLineDoesNotBreakSiblingGroup(t *testing.T) {
	d := Concat(Group(Concat(Text("a"), HardLine, Text("b"))), Group(Concat(Text("x"), Line, Text("y"))))
	eq(t, Render(d, 100), "a\nbx y")
}

func TestRender_IfBreak(t *testing.T) {
	d := Group(Concat(Text("["), Nest(Concat(SoftLine, Text("1"), IfBreak(Text(","), Text("")))), SoftLine, Text("]")))
	eq(t, Render(d, 100), "[1]")
	eq(t, Render(d, 2), "[\n  1,\n]")
}

func TestRender_IfBreakTopLevelIsBroken(t *testing.T) {
	eq(t, Render(IfBreak(Text("b"), Text("f")), 100), "b")
}

func TestRender_RuneWidth(t *testing.T) {
	d := Group(Concat(Text("ééééé"), Line, Text("x")))
	eq(t, Render(d, 7), "ééééé x")
	eq(t, Render(d, 6), "ééééé\nx")
}

func TestRender_NoTrailingSpaces(t *testing.T) {
	d := Nest(Concat(Text("a"), HardLine, HardLine, Text("b")))
	eq(t, Render(d, 100), "a\n\n  b")
	out := Render(Nest(Concat(Text("a"), HardLine, HardLine)), 100)
	for _, l := range strings.Split(out, "\n") {
		if l != strings.TrimRight(l, " ") {
			t.Errorf("trailing space in %q", l)
		}
	}
}

func TestRender_TextNewlineIsHardBreak(t *testing.T) {
	d := Group(Concat(Text("a\nbc"), Line, Text("d")))
	eq(t, Render(d, 100), "a\nbc\nd")
	// column resets to the length after the last newline
	d2 := Concat(Text("aaaa\nb"), Group(Concat(Text("c"), Line, Text("d"))))
	eq(t, Render(d2, 5), "aaaa\nbc d")
}

func TestRender_FitsConsidersTrailingText(t *testing.T) {
	// the group itself fits, but the text after it up to the next break does not
	d := Concat(Group(Concat(Text("a"), Line, Text("b"))), Text("cccc"), Line, Text("e"))
	eq(t, Render(d, 5), "a\nbcccc\ne")
	eq(t, Render(d, 7), "a bcccc\ne")
}

func TestRender_NestedIndent(t *testing.T) {
	d := Nest(Concat(Text("a"), HardLine, Nest(Concat(Text("b"), HardLine, Text("c"))), HardLine, Text("d")))
	eq(t, Render(d, 100), "a\n  b\n    c\n  d")
}

func TestRender_Empty(t *testing.T) {
	eq(t, Render(Concat(), 100), "")
	eq(t, Render(Text(""), 100), "")
}

func TestRender_DeepNestingNoRecursion(t *testing.T) {
	var d Doc = Text("x")
	for i := 0; i < 200000; i++ {
		d = Group(Concat(Text("("), d, Text(")")))
	}
	if got := Render(d, 100); len(got) != 400001 {
		t.Errorf("len = %d", len(got))
	}
}

func TestRender_Performance(t *testing.T) {
	skipTiming(t)
	items := make([]Doc, 0, 4000)
	for i := 0; i < 1000; i++ {
		items = append(items, Group(Concat(Text("let x ="), Nest(Concat(Line, Text("foo(bar, baz)"))), IfBreak(Text(";"), Text("")))), HardLine)
	}
	start := time.Now()
	out := Render(Concat(items...), Width)
	if el := time.Since(start); el > 20*time.Millisecond {
		t.Errorf("rendering 1000 groups took %v", el)
	}
	if !strings.HasPrefix(out, "let x = foo(bar, baz)\n") {
		t.Errorf("unexpected output %q", out[:30])
	}
}

func TestRender_TrimsTrailingSpacesBeforeNewline(t *testing.T) {
	// a flat Line or a text's trailing space before a hard break leaves nothing behind
	d := Group(Concat(Text("x ="), Line, Concat(Text("y"), Text(" ")), HardLine, Text("z")))
	eq(t, Render(Concat(Text("a = "), HardLine, Text("b")), 100), "a =\nb")
	eq(t, Render(d, 100), "x =\ny\nz")
	// bytes of a text with an embedded newline are verbatim, trailing blanks included
	eq(t, Render(Concat(Text("s \n t \t"), HardLine, Text("u")), 100), "s \n t \t\nu")
}

func TestRender_FlatNeverBreaks(t *testing.T) {
	eq(t, Render(Concat(Text("f "), Flat(braces())), 5), "f { a: 1 }")
}

func TestRender_FlatWithHardBreakPrintsAsUsual(t *testing.T) {
	d := Flat(Group(Concat(Text("{"), Nest(Concat(HardLine, Text("a: 1"))), Line, Text("}"))))
	eq(t, Render(d, 100), "{\n  a: 1\n}")
}

// A trailing comment breaks its groups and has no width: the code before it keeps its layout.
func TestRender_Trailing(t *testing.T) {
	d := Group(Concat(Text("{"), Nest(Concat(Line, Text("a: 1,"), Trailing(" -- c"), Line, Text("b: 2"))), Line, Text("}")))
	eq(t, Render(d, 100), "{\n  a: 1, -- c\n  b: 2\n}")
	body := Concat(Text("x ="), Group(Nest(Concat(Line, Text("long_value")))), Trailing(" -- a comment far wider than the line"))
	eq(t, Render(body, 20), "x = long_value -- a comment far wider than the line")
}
