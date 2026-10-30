// internal/syntax/intlit_test.go
package syntax

import (
	"math"
	"strings"
	"testing"
)

func TestIntLiteralRange(t *testing.T) {
	parse := func(body string) (Expr, []string) {
		m, diags := ParseFile("T.alder", []byte("module T\n\nx : Int\nx = "+body+"\n"))
		var msgs []string
		for _, d := range diags {
			msgs = append(msgs, d.Code+" "+d.Message)
		}
		for _, d := range m.Decls {
			if f, ok := d.(*ValueDecl); ok && f.Name == "x" {
				return f.Body, msgs
			}
		}
		t.Fatalf("no x in %q (%v)", body, msgs)
		return nil, nil
	}
	for _, c := range []struct {
		src  string
		want int64
	}{
		{"9223372036854775807", 9223372036854775807},
		{"-9223372036854775807", -9223372036854775807},
		{"-9223372036854775808", -9223372036854775808},
		{"-09223372036854775808", -9223372036854775808},
	} {
		e, msgs := parse(c.src)
		l, ok := e.(*IntLit)
		if len(msgs) != 0 || !ok || l.Value != c.want {
			t.Errorf("%s: got %#v %v, want %d", c.src, e, msgs, c.want)
		}
	}
	// Oversized, and MinInt64 only as `-` directly on the literal (not parenthesised).
	for _, src := range []string{"9223372036854775808", "99999999999999999999", "-9223372036854775809", "-(9223372036854775808)"} {
		if _, msgs := parse(src); len(msgs) == 0 {
			t.Errorf("%s: want a diagnostic", src)
		}
	}
	_, msgs := parse("99999999999999999999")
	if len(msgs) != 1 || !strings.HasSuffix(msgs[0], "`99999999999999999999` does not fit in an Int (64 bits, up to 9223372036854775807)") {
		t.Errorf("message: %v", msgs)
	}
	// Float literals that overflow to infinity are errors too.
	for _, src := range []string{"1e999", "-1e999", "1" + strings.Repeat("0", 400) + ".0"} {
		if _, msgs := parse(src); len(msgs) == 0 {
			t.Errorf("%s: want a diagnostic", src)
		}
	}
	if e, msgs := parse("1.7976931348623157e308"); len(msgs) != 0 || e.(*FloatLit).Value != math.MaxFloat64 {
		t.Errorf("MaxFloat64: %v %v", e, msgs)
	}
	// Pattern literals go through the same path.
	_, diags := ParseFile("T.alder", []byte("module T\n\nf : Int -> Int\nf n = match n {\n  -9223372036854775808 -> 1,\n  9223372036854775808 -> 2,\n  _ -> 3\n}\n"))
	if len(diags) != 1 || !strings.Contains(diags[0].Message, "`9223372036854775808` does not fit") || diags[0].Span.Start.Line != 6 {
		t.Errorf("pattern: %v", diags)
	}
}
