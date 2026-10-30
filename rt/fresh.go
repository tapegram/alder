package rt

import (
	"reflect"
	"slices"
	"sync"
)

// Fresh is v with every slice an adapter could write through copied: the value itself when it is
// a List or Bytes, and the lists inside its record fields, Maybe, Result, union payloads and Dict
// keys and values, at every level that is a slice. Generated code passes each argument of a
// capability call through it (#37): the runtime shares storage freely (List.take is a view,
// File.text's body is the Text's own bytes, a literal's bytes are read-only memory), which is
// sound only because nothing writes to an Alder value, and a Go adapter is code the compiler does
// not check. With Fresh, an adapter may do as it likes with its arguments.
//
// Strings need no copy (Go cannot write one); a value with no slice in it is returned as is.
func Fresh[T any](v T) T {
	switch p := any(&v).(type) { // on *T, so the common cases box nothing
	case *string, *int64, *float64, *bool:
		return v
	case *[]byte:
		*p = slices.Clone(*p)
		return v
	case *[]string:
		*p = slices.Clone(*p)
		return v
	case *[]int64:
		*p = slices.Clone(*p)
		return v
	}
	if !needsFresh(reflect.TypeFor[T]()) {
		return v
	}
	return freshSlow(v)
}

// freshSlow is Fresh's reflect walk, in a function of its own so that only its copy of v escapes.
func freshSlow[T any](v T) T {
	var out T
	reflect.ValueOf(&out).Elem().Set(freshValue(reflect.ValueOf(&v).Elem()))
	return out
}

// dictFresh is implemented by every Dict instantiation: the Dict with Fresh keys and values, and
// whether its key or value type needs Fresh at all.
type dictFresh interface {
	freshAny() any
	needsFreshAny() bool
}

func (d Dict[K, V]) freshAny() any {
	c := d.clone()
	for i, k := range c.keys {
		c.keys[i] = Fresh(k)
	}
	for e, v := range c.m {
		c.m[e] = Fresh(v)
	}
	return c
}

func (d Dict[K, V]) needsFreshAny() bool {
	return needsFresh(reflect.TypeFor[K]()) || needsFresh(reflect.TypeFor[V]())
}

var freshTypes sync.Map // reflect.Type -> bool

var dictFreshType = reflect.TypeFor[dictFresh]()

// needsFresh reports whether a value of type t can hold a slice that Fresh copies.
func needsFresh(t reflect.Type) bool {
	if b, ok := freshTypes.Load(t); ok {
		return b.(bool)
	}
	b := needsFreshIn(t, map[reflect.Type]bool{})
	freshTypes.Store(t, b)
	return b
}

// needsFreshIn is needsFresh, with the types being visited (a recursive union reaches itself
// through a pointer; a slice on the cycle is found where it is).
func needsFreshIn(t reflect.Type, visiting map[reflect.Type]bool) bool {
	if visiting[t] {
		return false
	}
	visiting[t] = true
	defer delete(visiting, t)
	switch t.Kind() {
	case reflect.Slice, reflect.Interface:
		return true
	case reflect.Pointer, reflect.Array:
		return needsFreshIn(t.Elem(), visiting)
	case reflect.Struct:
		if t.Implements(dictFreshType) {
			return reflect.Zero(t).Interface().(dictFresh).needsFreshAny()
		}
		for i := range t.NumField() {
			if f := t.Field(i); f.IsExported() && needsFreshIn(f.Type, visiting) {
				return true
			}
		}
	}
	return false
}

func freshValue(v reflect.Value) reflect.Value {
	t := v.Type()
	if !needsFresh(t) {
		return v
	}
	switch t.Kind() {
	case reflect.Slice:
		if v.IsNil() {
			return v
		}
		n := reflect.MakeSlice(t, v.Len(), v.Len())
		if needsFresh(t.Elem()) {
			for i := range v.Len() {
				n.Index(i).Set(freshValue(v.Index(i)))
			}
		} else {
			reflect.Copy(n, v)
		}
		return n
	case reflect.Pointer:
		if v.IsNil() {
			return v
		}
		p := reflect.New(t.Elem())
		p.Elem().Set(freshValue(v.Elem()))
		return p
	case reflect.Interface:
		if v.IsNil() {
			return v
		}
		n := reflect.New(t).Elem()
		n.Set(freshValue(v.Elem()))
		return n
	case reflect.Array:
		n := reflect.New(t).Elem()
		for i := range v.Len() {
			n.Index(i).Set(freshValue(v.Index(i)))
		}
		return n
	case reflect.Struct:
		if v.CanInterface() {
			if d, ok := v.Interface().(dictFresh); ok {
				return reflect.ValueOf(d.freshAny())
			}
		}
		n := reflect.New(t).Elem()
		n.Set(v)
		for i := range t.NumField() {
			if f := t.Field(i); f.IsExported() && needsFresh(f.Type) {
				n.Field(i).Set(freshValue(v.Field(i)))
			}
		}
		return n
	}
	return v
}
