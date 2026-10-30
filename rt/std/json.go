package std

import "alder/rt"

// JsonNull is Json.null.
func JsonNull() rt.JsonValue { return rt.JsonValue{Kind: rt.JsonNull} }

// JsonBool is Json.bool.
func JsonBool(b bool) rt.JsonValue { return rt.JsonValue{Kind: rt.JsonBool, Bool: b} }

// JsonInt is Json.int: the decimal digits, however large.
func JsonInt(n int64) rt.JsonValue { return rt.JsonValue{Kind: rt.JsonInt, Int: n} }

// JsonFloat is Json.float: NaN and infinities encode as null.
func JsonFloat(f float64) rt.JsonValue { return rt.JsonValue{Kind: rt.JsonFloat, Float: f} }

// JsonString is Json.string.
func JsonString(s string) rt.JsonValue { return rt.JsonValue{Kind: rt.JsonString, Text: s} }

// JsonList is Json.list; the items are copied, never aliased.
func JsonList(xs []rt.JsonValue) rt.JsonValue {
	return rt.JsonValue{Kind: rt.JsonArray, Items: append([]rt.JsonValue(nil), xs...)}
}

// JsonObject is Json.object: keys in list order; a repeated key keeps its first position and
// takes its last value (Dict.insert's rule), so the value never holds a key twice.
func JsonObject(fs []struct {
	Key   string
	Value rt.JsonValue
}) rt.JsonValue {
	ms := make([]rt.JsonMember, 0, len(fs))
	at := make(map[string]int, len(fs))
	for _, f := range fs {
		if i, ok := at[f.Key]; ok {
			ms[i].Value = f.Value
			continue
		}
		at[f.Key] = len(ms)
		ms = append(ms, rt.JsonMember{Key: f.Key, Value: f.Value})
	}
	return rt.JsonValue{Kind: rt.JsonObject, Members: ms}
}

// JsonEncode is Json.encode: compact JSON.
func JsonEncode(v rt.JsonValue) string { return rt.JsonEncode(v) }

// JsonEncodeIndented is Json.encodeIndented: n spaces per level; n <= 0 is Json.encode.
func JsonEncodeIndented(n int64, v rt.JsonValue) string { return rt.JsonEncodeIndented(n, v) }
