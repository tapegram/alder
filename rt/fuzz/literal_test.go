package fuzz

import (
	"math"
	"strconv"
	"strings"
	"testing"
	"time"

	"alder/rt"
)

func TestLiterals(t *testing.T) {
	negZero := math.Copysign(0, -1)
	for _, c := range []struct{ got, want string }{
		{TextLit(""), `""`},
		{TextLit("a\"{\n"), `"a\"\{\n"`},
		{TextLit("tab\there\r\\}"), `"tab\there\r\\}"`},
		{TextLit("é ü"), `"é ü"`},
		{IntLit(0), "0"},
		{IntLit(-12), "-12"},
		{IntLit(math.MaxInt64), "9223372036854775807"},
		{IntLit(math.MinInt64), "(-9223372036854775807 - 1)"},
		{FloatLit(0), "0.0"},
		// Decided by parsing (R1): the Alder parser keeps the sign of `-0.0` (parser_expr.go
		// negates the parsed float64), and gen emits a negative zero literal as
		// math.Copysign(0, -1), so `-0.0` is negative zero when the program runs too. Fuzz.float
		// keeps drawing -0, and it prints as -0.0.
		{FloatLit(negZero), "-0.0"},
		{FloatLit(1.5), "1.5"},
		{FloatLit(-2), "-2.0"},
		{FloatLit(1e21), "1000000000000000000000.0"},
		{FloatLit(0.000001), "0.000001"},
		{FloatLit(math.SmallestNonzeroFloat64), "0." + strings.Repeat("0", 323) + "5"},
		{BoolLit(true), "True"},
		{BoolLit(false), "False"},
		{TimeLit(0), "Time.fromMillis 0"},
		{TimeLit(1767225600000), "Time.fromMillis 1767225600000"},
		{TimeLit(-5), "Time.fromMillis (-5)"},
		{DateLit(0), "Date.addDays 0 (Time.toDate Time.epoch)"},
		{DateLit(20454), "Date.addDays 20454 (Time.toDate Time.epoch)"},
		{DateLit(-3), "Date.addDays (-3) (Time.toDate Time.epoch)"},
		{BytesLit(nil), `Text.toBytes ""`},
		{BytesLit([]byte("hi\n")), `Text.toBytes "hi\n"`},
		{Apply("Just", "1"), "Just 1"},
		{Apply("Just", "-1"), "Just (-1)"},
		{Apply("Just", `"a b"`), `Just "a b"`},
		{Apply("Just", "Just 1"), "Just (Just 1)"},
		{Apply("Ok", "[1, 2]"), "Ok [1, 2]"},
		{Apply("Node", "{ a: 1 }"), "Node { a: 1 }"},
		{Apply("Just", IntLit(math.MinInt64)), "Just (-9223372036854775807 - 1)"},
		{ListLit([]int64{}, IntLit), "[]"},
		{ListLit([]int64{0, -1}, IntLit), "[0, -1]"},
		{MaybeLit(rt.Nothing[int64](), IntLit), "Nothing"},
		{MaybeLit(rt.Just[int64](-1), IntLit), "Just (-1)"},
		{ResultLit(rt.Ok[string, int64](2), TextLit, IntLit), "Ok 2"},
		{ResultLit(rt.Err[string, int64]("no"), TextLit, IntLit), `Err "no"`},
		{DictLit(rt.Dict[string, int64]{}, TextLit, IntLit), "Dict.empty"},
		{DictLit(rt.Dict[string, int64]{}.Insert("a", 1).Insert("b", -2), TextLit, IntLit),
			`Dict.fromList [{ key: "a", value: 1 }, { key: "b", value: -2 }]`},
		{RecordLit(nil, nil), "{}"},
		{RecordLit([]string{"a", "b"}, []string{"1", `"x"`}), `{ a: 1, b: "x" }`},
	} {
		if c.got != c.want {
			t.Errorf("got %s, want %s", c.got, c.want)
		}
	}
}

// Every Float the generator draws prints as a literal that reads back to the same value
// (strconv stands in for the Alder lexer, which reads digits '.' digits).
func TestFloatLitRoundTrips(t *testing.T) {
	c := Fresh(3)
	for i := range 2000 {
		if i%500 == 0 {
			c = Fresh(uint64(i) + 3)
		}
		f := Float(c)
		s := FloatLit(f)
		if strings.ContainsAny(s, "eE+") || !strings.Contains(s, ".") {
			t.Fatalf("FloatLit(%v) = %s: not an Alder float literal", f, s)
		}
		back, err := parseFloatLit(s)
		if err != nil || back != f || math.Signbit(back) != math.Signbit(f) {
			t.Fatalf("FloatLit(%v) = %s reads back as %v (%v)", f, s, back, err)
		}
	}
}

// The date and time literals name the value they print.
func TestDateTimeLitValues(t *testing.T) {
	ms := int64(1767225600123)
	if got := time.UnixMilli(ms).UTC(); !strings.HasSuffix(TimeLit(ms), " 1767225600123") || got.Year() != 2026 {
		t.Fatalf("TimeLit(%d) = %s", ms, TimeLit(ms))
	}
	d := rt.DateOf(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	if DateLit(DaysOf(d)) != "Date.addDays 20454 (Time.toDate Time.epoch)" {
		t.Fatalf("DateLit(DaysOf(2026-01-01)) = %s", DateLit(DaysOf(d)))
	}
}

func parseFloatLit(s string) (float64, error) { return strconv.ParseFloat(s, 64) }

// A user's Fuzz.forAll generator may make what derived ones never do: a non-finite Float prints
// as the Alder expression that evaluates to it (book chapter 4: `1.0 / 0.0` is infinity), and a
// Text with a code point Alder has no literal for (a control other than \n \t \r, invalid
// UTF-8) prints readably as `\u{…}`, which is no Alder: Pasteable says so.
func TestLiterals_OutsideDerived(t *testing.T) {
	for _, c := range []struct {
		got, want string
		ok        bool
	}{
		{FloatLit(math.Inf(1)), "(1.0 / 0.0)", true},
		{FloatLit(math.Inf(-1)), "(-1.0 / 0.0)", true},
		{FloatLit(math.NaN()), "(0.0 / 0.0)", true},
		{Apply("Just", FloatLit(math.Inf(1))), "Just (1.0 / 0.0)", true},
		{TextLit("a\x01b\x7f"), `"a\u{1}b\u{7F}"`, false},
		{TextLit("\u200b"), `"\u{200B}"`, false},
		{TextLit("x\xffy"), `"x\u{FFFD}y"`, false},
		{ListLit([]string{"ok", "\x00"}, TextLit), `["ok", "\u{0}"]`, false},
		{TextLit(`\u{1}`), `"\\u\{1}"`, true}, // the text itself, escaped: no marker
		{TextLit("é\n"), `"é\n"`, true},
	} {
		if c.got != c.want {
			t.Errorf("got %s, want %s", c.got, c.want)
		}
		if Pasteable(c.got) != c.ok {
			t.Errorf("Pasteable(%s) = %v", c.got, !c.ok)
		}
	}
}

// Quals writes a constructor or brand of another module through the module's qualifier in the
// module the literal is pasted into: its own names bare, an imported module's through its
// canonical qualifier, any other by the qualifier the map gives (or its path).
func TestQuals(t *testing.T) {
	q := Quals{"Report": "", "Todo": "Todo", "Data.Colors": "Colors"}
	for _, c := range []struct{ got, want string }{
		{q.Name("Report", "Slug"), "Slug"},
		{q.Name("Todo", "Leaf"), "Todo.Leaf"},
		{q.Name("Data.Colors", "Red"), "Colors.Red"},
		{q.Name("Other.Mod", "X"), "Other.Mod.X"},
		{Quals(nil).Name("Todo", "Leaf"), "Leaf"},
	} {
		if c.got != c.want {
			t.Errorf("got %s, want %s", c.got, c.want)
		}
	}
}
