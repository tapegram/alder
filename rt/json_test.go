package rt_test

import (
	"math"
	"testing"

	"alder/rt"
)

func jint(n int64) rt.JsonValue     { return rt.JsonValue{Kind: rt.JsonInt, Int: n} }
func jfloat(f float64) rt.JsonValue { return rt.JsonValue{Kind: rt.JsonFloat, Float: f} }
func jstr(s string) rt.JsonValue    { return rt.JsonValue{Kind: rt.JsonString, Text: s} }
func jbool(b bool) rt.JsonValue     { return rt.JsonValue{Kind: rt.JsonBool, Bool: b} }
func jlist(xs ...rt.JsonValue) rt.JsonValue {
	return rt.JsonValue{Kind: rt.JsonArray, Items: xs}
}
func jobj(ms ...rt.JsonMember) rt.JsonValue {
	return rt.JsonValue{Kind: rt.JsonObject, Members: ms}
}
func jm(k string, v rt.JsonValue) rt.JsonMember { return rt.JsonMember{Key: k, Value: v} }

// u is the JSON escape backslash-u followed by hex.
func u(hex string) string { return `\` + "u" + hex }

func checkJSON(t *testing.T, name, got, want string) {
	t.Helper()
	if got != want {
		t.Errorf("%s: got %q, want %q", name, got, want)
	}
}

func TestJsonEncode_Scalars(t *testing.T) {
	for _, c := range []struct {
		name string
		v    rt.JsonValue
		want string
	}{
		{"null", rt.JsonValue{}, "null"},
		{"true", jbool(true), "true"},
		{"false", jbool(false), "false"},
		{"int 0", jint(0), "0"},
		{"int -7", jint(-7), "-7"},
		{"int max", jint(math.MaxInt64), "9223372036854775807"},
		{"int min", jint(math.MinInt64), "-9223372036854775808"},
		{"float 1.0", jfloat(1.0), "1"},
		{"float 0.1", jfloat(0.1), "0.1"},
		{"float -2.5", jfloat(-2.5), "-2.5"},
		{"float -0", jfloat(math.Copysign(0, -1)), "-0"},
		{"float 1e21", jfloat(1e21), "1000000000000000000000"},
		{"float 1e-7", jfloat(1e-7), "0.0000001"},
		{"float NaN", jfloat(math.NaN()), "null"},
		{"float +Inf", jfloat(math.Inf(1)), "null"},
		{"float -Inf", jfloat(math.Inf(-1)), "null"},
	} {
		checkJSON(t, c.name, rt.JsonEncode(c.v), c.want)
	}
}

func TestJsonEncode_Escapes(t *testing.T) {
	for _, c := range []struct{ in, want string }{
		{`"`, `"\""`},
		{`\`, `"\\"`},
		{"\b", `"\b"`},
		{"\t", `"\t"`},
		{"\n", `"\n"`},
		{"\f", `"\f"`},
		{"\r", `"\r"`},
		{"\x00", `"\u0000"`},
		{"\x1f", `"\u001f"`},
		{"\x0b", `"\u000b"`},
		{"<", `"` + u("003c") + `"`},
		{">", `"` + u("003e") + `"`},
		{"&", `"` + u("0026") + `"`},
		{string(rune(0x2028)), `"` + u("2028") + `"`},
		{string(rune(0x2029)), `"` + u("2029") + `"`},
		{"</script>", `"` + u("003c") + `/script` + u("003e") + `"`},
		{"a\xffb", `"a` + string(rune(0xFFFD)) + `b"`},
		{"/", `"/"`},
		{"\x7f", "\"\x7f\""},
		{"é", `"é"`},
		{"😀", `"😀"`},
		{"", `""`},
	} {
		checkJSON(t, "escape "+c.in, rt.JsonEncode(jstr(c.in)), c.want)
	}
}

func TestJsonEncode_Containers(t *testing.T) {
	checkJSON(t, "empty list", rt.JsonEncode(jlist()), "[]")
	checkJSON(t, "empty object", rt.JsonEncode(jobj()), "{}")
	v := jobj(jm("b", jint(1)), jm("a", jlist(jbool(true), rt.JsonValue{}, jstr("x"))), jm("c\"", jobj()))
	checkJSON(t, "nested", rt.JsonEncode(v), `{"b":1,"a":[true,null,"x"],"c\"":{}}`)
}

func TestJsonEncodeIndented(t *testing.T) {
	v := jobj(jm("a", jint(1)), jm("b", jlist(jbool(true), jlist(), jobj(jm("c", jstr("x"))))), jm("d", jobj()))
	want := "{\n" +
		"  \"a\": 1,\n" +
		"  \"b\": [\n" +
		"    true,\n" +
		"    [],\n" +
		"    {\n" +
		"      \"c\": \"x\"\n" +
		"    }\n" +
		"  ],\n" +
		"  \"d\": {}\n" +
		"}"
	checkJSON(t, "indented 2", rt.JsonEncodeIndented(2, v), want)
	checkJSON(t, "indented 0", rt.JsonEncodeIndented(0, v), rt.JsonEncode(v))
	checkJSON(t, "indented -1", rt.JsonEncodeIndented(-1, v), rt.JsonEncode(v))
	checkJSON(t, "indented scalar", rt.JsonEncodeIndented(4, jint(3)), "3")
	checkJSON(t, "indented 1 list", rt.JsonEncodeIndented(1, jlist(jint(1))), "[\n 1\n]")
}

func TestJsonValue_EqKeyShow(t *testing.T) {
	pairs := []struct {
		name  string
		a, b  rt.JsonValue
		equal bool
	}{
		{"int 1 vs float 1.0", jint(1), jfloat(1.0), true},
		{"NaN vs null", jfloat(math.NaN()), rt.JsonValue{}, true},
		{"1 vs 2", jint(1), jint(2), false},
		{"text 1 vs int 1", jstr("1"), jint(1), false},
		{"objects", jobj(jm("a", jint(1))), jobj(jm("a", jfloat(1))), true},
		{"key order", jobj(jm("a", jint(1)), jm("b", jint(2))), jobj(jm("b", jint(2)), jm("a", jint(1))), false},
	}
	for _, p := range pairs {
		if got := rt.Eq(p.a, p.b); got != p.equal {
			t.Errorf("Eq %s: got %v", p.name, got)
		}
		if got := rt.Key(p.a) == rt.Key(p.b); got != p.equal {
			t.Errorf("Key %s: got %v", p.name, got)
		}
	}
	checkJSON(t, "show", rt.Show(jobj(jm("a", jint(1)))), `{"a":1}`)
	checkJSON(t, "show string", rt.Show(jstr("x")), `"x"`)
	checkJSON(t, "show in Just", rt.Show(rt.Just(jlist(jint(1)))), `Just [1]`)
}
