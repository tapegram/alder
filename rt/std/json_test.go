package std

import (
	"math"
	"testing"

	"alder/rt"
)

type jfield = struct {
	Key   string
	Value rt.JsonValue
}

func TestJson_Constructors(t *testing.T) {
	eq(t, "null", JsonEncode(JsonNull()), "null")
	eq(t, "bool", JsonEncode(JsonBool(true)), "true")
	eq(t, "int", JsonEncode(JsonInt(-42)), "-42")
	eq(t, "int large", JsonEncode(JsonInt(math.MaxInt64)), "9223372036854775807")
	eq(t, "float 1.0", JsonEncode(JsonFloat(1.0)), "1")
	eq(t, "float 0.1", JsonEncode(JsonFloat(0.1)), "0.1")
	eq(t, "float -0", JsonEncode(JsonFloat(math.Copysign(0, -1))), "-0")
	eq(t, "float NaN", JsonEncode(JsonFloat(math.NaN())), "null")
	eq(t, "float +Inf", JsonEncode(JsonFloat(math.Inf(1))), "null")
	eq(t, "float -Inf", JsonEncode(JsonFloat(math.Inf(-1))), "null")
	eq(t, "string", JsonEncode(JsonString("say \"hi\"\n")), `"say \"hi\"\n"`)
	eq(t, "list", JsonEncode(JsonList([]rt.JsonValue{JsonInt(1), JsonNull(), JsonString("x")})), `[1,null,"x"]`)
	eq(t, "list empty", JsonEncode(JsonList(nil)), "[]")
	eq(t, "object empty", JsonEncode(JsonObject(nil)), "{}")
	eq(t, "object order", JsonEncode(JsonObject([]jfield{{"b", JsonInt(1)}, {"a", JsonBool(false)}})), `{"b":1,"a":false}`)
}

func TestJson_ObjectDuplicateKeys(t *testing.T) {
	v := JsonObject([]jfield{{"a", JsonInt(1)}, {"b", JsonInt(2)}, {"a", JsonInt(3)}})
	eq(t, "encode", JsonEncode(v), `{"a":3,"b":2}`)
	eq(t, "show", rt.Show(v), `{"a":3,"b":2}`)
	eq(t, "==", rt.Eq(v, JsonObject([]jfield{{"a", JsonInt(3)}, {"b", JsonInt(2)}})), true)
}

func TestJson_DoesNotAliasInput(t *testing.T) {
	fs := []jfield{{"a", JsonInt(1)}}
	o := JsonObject(fs)
	fs[0].Value = JsonInt(9)
	xs := []rt.JsonValue{JsonInt(1)}
	l := JsonList(xs)
	xs[0] = JsonInt(9)
	eq(t, "object", JsonEncode(o), `{"a":1}`)
	eq(t, "list", JsonEncode(l), `[1]`)
}

func TestJson_EncodeIndented(t *testing.T) {
	v := JsonObject([]jfield{{"a", JsonList([]rt.JsonValue{JsonInt(1), JsonInt(2)})}, {"b", JsonObject(nil)}})
	eq(t, "2", JsonEncodeIndented(2, v), "{\n  \"a\": [\n    1,\n    2\n  ],\n  \"b\": {}\n}")
	eq(t, "0", JsonEncodeIndented(0, v), `{"a":[1,2],"b":{}}`)
	eq(t, "-3", JsonEncodeIndented(-3, v), `{"a":[1,2],"b":{}}`)
}

func TestJson_EqKeyShowAgree(t *testing.T) {
	eq(t, "int 1 == float 1.0", rt.Eq(JsonInt(1), JsonFloat(1.0)), true)
	eq(t, "NaN == null", rt.Eq(JsonFloat(math.NaN()), JsonNull()), true)
	eq(t, "1 /= 2", rt.Eq(JsonInt(1), JsonInt(2)), false)
	eq(t, "string 1 /= int 1", rt.Eq(JsonString("1"), JsonInt(1)), false)
	d := DictInsert(JsonFloat(1.0), int64(2), DictInsert(JsonInt(1), int64(1), DictEmpty[rt.JsonValue, int64]()))
	eq(t, "dict size", DictSize(d), int64(1))
	eq(t, "dict get", DictGet(JsonInt(1), d), rt.Just(int64(2)))
	eq(t, "dict member", DictMember(JsonNull(), d), false)
	eq(t, "show", rt.Show(JsonList([]rt.JsonValue{JsonString("a")})), `["a"]`)
}
