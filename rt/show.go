package rt

import (
	"fmt"
	"math"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"
)

// unions maps a generated union's tag type to its constructor names, in tag order.
var unions sync.Map // reflect.Type → []string

// RegisterUnion records a union's constructor names by tag so Show can print them; generated
// code calls it from init(). tagType is the tag's reflect.Type (tag__M__U); names[i] is the
// constructor whose tag constant is i.
func RegisterUnion(tagType reflect.Type, names []string) {
	unions.Store(tagType, append([]string(nil), names...))
}

// Show prints v as Alder syntax. Deterministic. Unknown Go values print as %v.
//
// Int 3, Float 1.5 / 1.0 / 1000000.0 / 0.00001 (always with a point, never an exponent) / NaN /
// +Inf, Text "a\"b" (escapes \\ \" \n \t \{), True/False, () for Unit, [1, 2], Nothing / Just 3,
// Ok 1 / Err "x", Dict.fromList [{ key: k, value: v }] in insertion order (Dict.empty), a Date
// 2026-10-09, a Time in RFC 3339 UTC, a record { title: "Milk", _at: … } (field names are the
// inverse of gen's Field: the Go field with its first letter lower-cased, U__at as _at), a
// registered union Name / Name 3 / Name { w: 1.0 }, Html as its markup, <Query> / <Task>, the
// Db builder's values as <Select> / <Table> / <Condition> / <Order>, a Page as its URL, a Nav
// as Rerender / Redirect <url> / Reject "text". NaN, +Inf and -Inf are not Alder literals; they
// print as is. A constructor's argument is parenthesized when it is itself an application or
// negative: Just (Rect { w: 1.0 }), Just (-1).
func Show(v any) (s string) {
	defer func() {
		if r := recover(); r != nil {
			s = fmt.Sprintf("%v", v)
		}
	}()
	s, _ = show(reflect.ValueOf(v))
	return s
}

var (
	pageType = reflect.TypeFor[Page]()
	navType  = reflect.TypeFor[Nav]()
	dateType = reflect.TypeFor[Date]()
)

// Shower is a runtime type that prints itself as Alder (the test types Doc and Answer of
// rt/testing, which rt cannot name): AlderShow is its text and whether it is an application
// (it needs parentheses as an argument).
type Shower interface{ AlderShow() (string, bool) }

// arg is v as a constructor's argument: parenthesized when it is an application or negative.
func arg(v reflect.Value) string {
	s, app := show(v)
	if app {
		return "(" + s + ")"
	}
	return s
}

// show is v as Alder, and whether it is an application or a negative number (it needs
// parentheses as an argument).
func show(v reflect.Value) (string, bool) {
	if !v.IsValid() {
		return "<nil>", false
	}
	t := v.Type()
	switch t {
	case timeType:
		if v.CanInterface() {
			return v.Interface().(time.Time).UTC().Format(time.RFC3339Nano), false
		}
		return "<time>", false
	case dateType:
		return fmt.Sprintf("%04d-%02d-%02d", v.Field(0).Int(), v.Field(1).Int(), v.Field(2).Int()), false
	case pageType:
		return pageURL(v), false
	case navType:
		switch NavTag(v.FieldByName("Tag").Uint()) {
		case NavRerender:
			return "Rerender", false
		case NavRedirect:
			return "Redirect " + pageURL(v.FieldByName("Page")), true
		case NavReject:
			return "Reject " + quote(v.FieldByName("Text").String()), true
		case NavNoticed:
			return "Noticed " + quote(v.FieldByName("Text").String()) + " " + Show(v.FieldByName("Next").Elem().Interface()), true
		}
	}
	if v.Kind() == reflect.Struct && v.CanInterface() {
		if sh, ok := v.Interface().(Shower); ok {
			return sh.AlderShow()
		}
	}
	switch t.PkgPath() {
	case "alder/rt":
		name := t.Name()
		switch {
		case strings.HasPrefix(name, "Maybe["):
			if !v.FieldByName("Ok").Bool() {
				return "Nothing", false
			}
			return "Just " + arg(v.FieldByName("Val")), true
		case strings.HasPrefix(name, "Result["):
			if v.FieldByName("IsOk").Bool() {
				return "Ok " + arg(v.FieldByName("Val")), true
			}
			return "Err " + arg(v.FieldByName("Err")), true
		case strings.HasPrefix(name, "Dict["):
			return showDict(v)
		}
	case "alder/rt/eff":
		name := t.Name()
		switch {
		case strings.HasPrefix(name, "Query["):
			return "<Query>", false
		case strings.HasPrefix(name, "Task["):
			return "<Task>", false
		}
	case "alder/rt/db": // the query builder's data is opaque in Alder
		name := t.Name()
		switch {
		case strings.HasPrefix(name, "Select["):
			return "<Select>", false
		case strings.HasPrefix(name, "Table["):
			return "<Table>", false
		case name == "Cond":
			return "<Condition>", false
		case name == "Order":
			return "<Order>", false
		}
	case "alder/rt/html":
		if t.Name() == "Node" && v.CanInterface() {
			if s, ok := v.Interface().(fmt.Stringer); ok {
				return s.String(), false
			}
		}
	}
	switch v.Kind() {
	case reflect.String:
		return quote(v.String()), false
	case reflect.Bool:
		if v.Bool() {
			return "True", false
		}
		return "False", false
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return strconv.FormatInt(v.Int(), 10), v.Int() < 0
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		return strconv.FormatUint(v.Uint(), 10), false
	case reflect.Float32, reflect.Float64:
		return showFloat(v.Float())
	case reflect.Slice, reflect.Array:
		parts := make([]string, v.Len())
		for i := range v.Len() {
			parts[i], _ = show(v.Index(i))
		}
		return "[" + strings.Join(parts, ", ") + "]", false
	case reflect.Pointer:
		if v.IsNil() {
			if t.Elem().PkgPath() == "alder/rt" && strings.HasPrefix(t.Elem().Name(), "Maybe[") {
				return "Nothing", false
			}
			return "<nil>", false
		}
		return show(v.Elem())
	case reflect.Interface:
		if v.IsNil() {
			return "<nil>", false
		}
		return show(v.Elem())
	case reflect.Func:
		return "<function>", false
	case reflect.Struct:
		if s, app, ok := showUnion(v); ok {
			return s, app
		}
		return showRecord(v), false
	}
	if v.CanInterface() {
		return fmt.Sprintf("%v", v.Interface()), false
	}
	return fmt.Sprintf("%v", v), false
}

func showFloat(f float64) (string, bool) {
	switch {
	case math.IsNaN(f):
		return "NaN", false
	case math.IsInf(f, 1):
		return "+Inf", false
	case math.IsInf(f, -1):
		return "-Inf", true
	}
	s := strconv.FormatFloat(f, 'f', -1, 64) // an Alder literal: no exponent
	if !strings.Contains(s, ".") {
		s += ".0"
	}
	return s, math.Signbit(f)
}

// pageURL is PageURL of a Page value, read field by field (so it works on a value reflect cannot
// hand out as an interface).
func pageURL(v reflect.Value) string {
	p := Page{Path: v.FieldByName("Path").String()}
	q := v.FieldByName("Query")
	for i := range q.Len() {
		kv := q.Index(i)
		p.Query = append(p.Query, KV{Key: kv.Field(0).String(), Value: kv.Field(1).String(), Omit: kv.Field(2).Bool()})
	}
	return PageURL(p)
}

// dictShow is implemented by every Dict instantiation.
type dictShow interface{ showEntries() (keys, vals []any) }

// showEntries are d's keys in insertion order and their values, as interfaces: Show reads them
// through this method rather than through reflect on d's unexported fields, whose values reflect
// marks read-only (a Time key or an Html value could then not be handed out).
func (d Dict[K, V]) showEntries() (keys, vals []any) {
	keys, vals = make([]any, len(d.keys)), make([]any, len(d.keys))
	for i, k := range d.keys {
		keys[i], vals[i] = k, d.m[d.encs[i]]
	}
	return keys, vals
}

func asDictShow(v reflect.Value) (dictShow, bool) {
	if !v.CanInterface() {
		return nil, false
	}
	d, ok := v.Interface().(dictShow)
	return d, ok
}

// showDict prints a Dict as Dict.fromList of its entries in insertion order. Generated records
// export every field, so reflect can always hand a Dict out; one that it could not (reached
// through unexported Go fields) is read field by field instead, as a fallback that never panics.
func showDict(v reflect.Value) (string, bool) {
	var keys, vals []reflect.Value
	if d, ok := asDictShow(v); ok {
		ks, vs := d.showEntries()
		for i := range ks {
			keys, vals = append(keys, reflect.ValueOf(ks[i])), append(vals, reflect.ValueOf(vs[i]))
		}
	} else {
		ks, encs, m := v.FieldByName("keys"), v.FieldByName("encs"), v.FieldByName("m")
		for i := range ks.Len() {
			keys, vals = append(keys, ks.Index(i)), append(vals, m.MapIndex(encs.Index(i)))
		}
	}
	if len(keys) == 0 {
		return "Dict.empty", false
	}
	parts := make([]string, len(keys))
	for i := range keys {
		k, _ := show(keys[i])
		val, _ := show(vals[i])
		parts[i] = "{ key: " + k + ", value: " + val + " }"
	}
	return "Dict.fromList [" + strings.Join(parts, ", ") + "]", true
}

// showUnion prints a generated union struct: one with a Tag field whose type is registered.
func showUnion(v reflect.Value) (string, bool, bool) {
	tag := v.FieldByName("Tag")
	if !tag.IsValid() {
		return "", false, false
	}
	n, ok := unions.Load(tag.Type())
	if !ok {
		return "", false, false
	}
	names := n.([]string)
	i := tag.Uint()
	if i >= uint64(len(names)) {
		return "", false, false
	}
	name := names[i]
	field := name
	if name == "Tag" {
		field = "Tag__"
	}
	p := v.FieldByName(field)
	if !p.IsValid() || p.Kind() != reflect.Pointer || p.IsNil() {
		return name, false, true
	}
	p = p.Elem()
	pt := p.Type()
	if p.Kind() == reflect.Struct && pt.NumField() == 1 && pt.Field(0).Name == "Value" &&
		(pt.Name() == "" || strings.HasPrefix(pt.Name(), "payload__")) {
		return name + " " + arg(p.Field(0)), true, true
	}
	return name + " " + arg(p), true, true
}

// showRecord prints a struct as a record, its Go field names lower-cased back to Alder's.
func showRecord(v reflect.Value) string {
	t := v.Type()
	if t.NumField() == 0 {
		return "()"
	}
	parts := make([]string, t.NumField())
	for i := range t.NumField() {
		s, _ := show(v.Field(i))
		parts[i] = fieldName(t.Field(i).Name) + ": " + s
	}
	return "{ " + strings.Join(parts, ", ") + " }"
}

// fieldName inverts gen's Field: U__x is the field _x; any other has its first letter
// lower-cased.
func fieldName(goName string) string {
	if rest, ok := strings.CutPrefix(goName, "U__"); ok {
		return "_" + rest
	}
	r, n := utf8.DecodeRuneInString(goName)
	return string(unicode.ToLower(r)) + goName[n:]
}

// quote is s as an Alder string literal.
func quote(s string) string {
	var b strings.Builder
	b.WriteByte('"')
	for _, r := range s {
		switch r {
		case '\\':
			b.WriteString(`\\`)
		case '"':
			b.WriteString(`\"`)
		case '\n':
			b.WriteString(`\n`)
		case '\t':
			b.WriteString(`\t`)
		case '{':
			b.WriteString(`\{`)
		default:
			b.WriteRune(r)
		}
	}
	b.WriteByte('"')
	return b.String()
}
