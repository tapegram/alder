package syntax

import (
	"reflect"
	"sort"
	"strings"

	"alder/internal/diag"
)

// Spanned is any AST node: everything that embeds Node.
type Spanned interface{ GetSpan() diag.Span }

var spannedType = reflect.TypeOf((*Spanned)(nil)).Elem()

// Children returns n's direct child nodes in source order, found by reflection over n's fields:
// a field holding a node, a slice of nodes, or a plain struct (StrPart) holding nodes. Nil
// interfaces and pointers are skipped, and so is the embedded Node itself.
func Children(n Spanned) []Spanned {
	var out []Spanned
	var fields func(v reflect.Value)
	var value func(v reflect.Value)
	value = func(v reflect.Value) {
		switch v.Kind() {
		case reflect.Interface, reflect.Pointer:
			if v.IsNil() {
				return
			}
		}
		if v.Type().Implements(spannedType) && v.Kind() != reflect.Struct {
			out = append(out, v.Interface().(Spanned))
			return
		}
		switch v.Kind() {
		case reflect.Slice:
			for i := 0; i < v.Len(); i++ {
				value(v.Index(i))
			}
		case reflect.Struct:
			fields(v)
		}
	}
	fields = func(v reflect.Value) {
		t := v.Type()
		for i := 0; i < v.NumField(); i++ {
			f := t.Field(i)
			if f.Anonymous && f.Type == reflect.TypeOf(Node{}) || !f.IsExported() {
				continue
			}
			value(v.Field(i))
		}
	}
	rv := reflect.ValueOf(n)
	if rv.Kind() == reflect.Pointer {
		rv = rv.Elem()
	}
	if rv.Kind() == reflect.Struct {
		fields(rv)
	}
	sort.SliceStable(out, func(i, j int) bool { return less(out[i].GetSpan().Start, out[j].GetSpan().Start) })
	return out
}

func less(a, b diag.Pos) bool { return a.Line < b.Line || a.Line == b.Line && a.Col < b.Col }

// Contains reports whether pos is in the half-open span [Start, End).
func Contains(sp diag.Span, pos diag.Pos) bool {
	return !less(pos, sp.Start) && less(pos, sp.End)
}

// NodeAt is the path from m down to the innermost node whose span contains pos (index 0 is m).
func NodeAt(m *Module, pos diag.Pos) []Spanned {
	path := []Spanned{m}
	for {
		var next Spanned
		for _, c := range Children(path[len(path)-1]) {
			sp := c.GetSpan()
			if sp.Start == sp.End {
				continue
			}
			if Contains(sp, pos) {
				next = c
			}
		}
		if next == nil {
			return path
		}
		path = append(path, next)
	}
}

// DocOf is the comment block directly above the 1-based line: the run of lines starting with
// `--` ending at line-1, with `--` and one space stripped, joined by newlines.
func DocOf(lines []string, line int) string {
	var doc []string
	for i := line - 2; i >= 0 && i < len(lines); i-- {
		l := strings.TrimSpace(lines[i])
		if !strings.HasPrefix(l, "--") {
			break
		}
		doc = append([]string{strings.TrimPrefix(strings.TrimPrefix(l, "--"), " ")}, doc...)
	}
	return strings.Join(doc, "\n")
}
