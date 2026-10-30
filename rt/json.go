package rt

import (
	"math"
	"strconv"
	"strings"
	"unicode/utf8"
)

// JsonKind says which JSON value a JsonValue is.
type JsonKind uint8

const (
	JsonNull   JsonKind = iota // the zero JsonValue is null
	JsonBool                   // Bool
	JsonInt                    // Int
	JsonFloat                  // Float; NaN and infinities encode as null
	JsonString                 // Text
	JsonArray                  // Items
	JsonObject                 // Members, in order, keys distinct
)

// JsonValue is the stdlib `Json.Value`: a JSON tree. Only the field its Kind names is
// meaningful. Its `==`, Dict key and Show are those of its compact encoding (JsonEncode), so two
// values are equal exactly when they are the same JSON text.
type JsonValue struct {
	Kind    JsonKind
	Bool    bool
	Int     int64
	Float   float64
	Text    string
	Items   []JsonValue
	Members []JsonMember
}

// JsonMember is one key/value pair of a JSON object.
type JsonMember struct {
	Key   string
	Value JsonValue
}

// AlderEqual is Json.Value's equality (Equaler): the same compact encoding.
func (v JsonValue) AlderEqual(other any) bool { return JsonEncode(v) == JsonEncode(other.(JsonValue)) }

// AlderKey is Json.Value's Dict key (Keyer): its compact encoding, so equal values are one key.
func (v JsonValue) AlderKey() string { return JsonEncode(v) }

// AlderShow prints a Json.Value as its compact JSON text, unquoted (Shower).
func (v JsonValue) AlderShow() (string, bool) { return JsonEncode(v), false }

// JsonEncode is Json.encode: compact JSON (RFC 8259), no whitespace.
func JsonEncode(v JsonValue) string {
	var b strings.Builder
	writeJSON(&b, v, 0, 0)
	return b.String()
}

// JsonEncodeIndented is Json.encodeIndented: n spaces per level, one item or member per line;
// n <= 0 is JsonEncode.
func JsonEncodeIndented(n int64, v JsonValue) string {
	if n <= 0 {
		return JsonEncode(v)
	}
	var b strings.Builder
	writeJSON(&b, v, int(n), 0)
	return b.String()
}

// writeJSON writes v at nesting depth; indent 0 is compact.
func writeJSON(b *strings.Builder, v JsonValue, indent, depth int) {
	switch v.Kind {
	case JsonNull:
		b.WriteString("null")
	case JsonBool:
		b.WriteString(strconv.FormatBool(v.Bool))
	case JsonInt:
		b.WriteString(strconv.FormatInt(v.Int, 10))
	case JsonFloat:
		if math.IsNaN(v.Float) || math.IsInf(v.Float, 0) {
			b.WriteString("null")
		} else {
			b.WriteString(strconv.FormatFloat(v.Float, 'f', -1, 64))
		}
	case JsonString:
		writeJSONString(b, v.Text)
	case JsonArray:
		if len(v.Items) == 0 {
			b.WriteString("[]")
			return
		}
		b.WriteByte('[')
		for i, it := range v.Items {
			if i > 0 {
				b.WriteByte(',')
			}
			newline(b, indent, depth+1)
			writeJSON(b, it, indent, depth+1)
		}
		newline(b, indent, depth)
		b.WriteByte(']')
	case JsonObject:
		if len(v.Members) == 0 {
			b.WriteString("{}")
			return
		}
		b.WriteByte('{')
		for i, m := range v.Members {
			if i > 0 {
				b.WriteByte(',')
			}
			newline(b, indent, depth+1)
			writeJSONString(b, m.Key)
			b.WriteByte(':')
			if indent > 0 {
				b.WriteByte(' ')
			}
			writeJSON(b, m.Value, indent, depth+1)
		}
		newline(b, indent, depth)
		b.WriteByte('}')
	}
}

func newline(b *strings.Builder, indent, depth int) {
	if indent > 0 {
		b.WriteByte('\n')
		b.WriteString(strings.Repeat(" ", indent*depth))
	}
}

const hexDigits = "0123456789abcdef"

// writeJSONString writes s as a JSON string: `"` and `\` escaped, control characters as their
// short escape or \u00xx, and <, >, &, U+2028, U+2029 as \u escapes so the text is safe inside
// an HTML <script> and a JS string literal. An invalid UTF-8 byte is written as U+FFFD.
func writeJSONString(b *strings.Builder, s string) {
	b.WriteByte('"')
	for i := 0; i < len(s); {
		c := s[i]
		if c < utf8.RuneSelf {
			switch {
			case c == '"':
				b.WriteString(`\"`)
			case c == '\\':
				b.WriteString(`\\`)
			case c == '\b':
				b.WriteString(`\b`)
			case c == '\t':
				b.WriteString(`\t`)
			case c == '\n':
				b.WriteString(`\n`)
			case c == '\f':
				b.WriteString(`\f`)
			case c == '\r':
				b.WriteString(`\r`)
			case c < 0x20 || c == '<' || c == '>' || c == '&':
				b.WriteString(`\u00`)
				b.WriteByte(hexDigits[c>>4])
				b.WriteByte(hexDigits[c&0xf])
			default:
				b.WriteByte(c)
			}
			i++
			continue
		}
		r, size := utf8.DecodeRuneInString(s[i:])
		switch {
		case r == utf8.RuneError && size == 1:
			b.WriteRune(utf8.RuneError) // U+FFFD
		case r == 0x2028 || r == 0x2029: // LINE and PARAGRAPH SEPARATOR end a JS line
			b.WriteString(`\u`)
			b.WriteString(strconv.FormatInt(int64(r), 16))
		default:
			b.WriteString(s[i : i+size])
		}
		i += size
	}
	b.WriteByte('"')
}
