package std

import "alder/rt"

func DictEmpty[K, V any]() rt.Dict[K, V] { return rt.Dict[K, V]{} }

// DictFromList builds a Dict in list order; a repeated key keeps its first
// position and its last value.
func DictFromList[K, V any](kvs []struct {
	Key   K
	Value V
}) rt.Dict[K, V] {
	var d rt.Dict[K, V]
	for _, kv := range kvs {
		d.Put(kv.Key, kv.Value)
	}
	return d
}

// DictToList lists entries in key insertion order.
func DictToList[K, V any](d rt.Dict[K, V]) []struct {
	Key   K
	Value V
} {
	keys := d.Keys()
	out := make([]struct {
		Key   K
		Value V
	}, len(keys))
	for i, k := range keys {
		v, _ := d.Get(k)
		out[i].Key, out[i].Value = k, v
	}
	return out
}

func DictGet[K, V any](k K, d rt.Dict[K, V]) rt.Maybe[V] {
	if v, ok := d.Get(k); ok {
		return rt.Just(v)
	}
	return rt.Nothing[V]()
}

func DictInsert[K, V any](k K, v V, d rt.Dict[K, V]) rt.Dict[K, V] { return d.Insert(k, v) }

func DictRemove[K, V any](k K, d rt.Dict[K, V]) rt.Dict[K, V] { return d.Remove(k) }

func DictKeys[K, V any](d rt.Dict[K, V]) []K { return d.Keys() }

func DictValues[K, V any](d rt.Dict[K, V]) []V { return d.Values() }

func DictSize[K, V any](d rt.Dict[K, V]) int64 { return int64(d.Len()) }

func DictMember[K, V any](k K, d rt.Dict[K, V]) bool { return d.Has(k) }

func DictSingleton[K, V any](k K, v V) rt.Dict[K, V] {
	var out rt.Dict[K, V]
	out.Put(k, v)
	return out
}

func DictIsEmpty[K, V any](d rt.Dict[K, V]) bool { return d.Len() == 0 }

// DictUpdate applies f to the current value of k (Nothing when absent). Just
// binds k, keeping its position or appending it; Nothing removes it.
func DictUpdate[K, V any](k K, f func(rt.Maybe[V]) rt.Maybe[V], d rt.Dict[K, V]) rt.Dict[K, V] {
	r := f(DictGet(k, d))
	if r.Ok {
		return d.Insert(k, r.Val)
	}
	return d.Remove(k)
}

func DictMap[K, V, W any](f func(K, V) W, d rt.Dict[K, V]) rt.Dict[K, W] {
	var out rt.Dict[K, W]
	for _, k := range d.Keys() {
		v, _ := d.Get(k)
		out.Put(k, f(k, v))
	}
	return out
}

func DictFilter[K, V any](f func(K, V) bool, d rt.Dict[K, V]) rt.Dict[K, V] {
	var out rt.Dict[K, V]
	for _, k := range d.Keys() {
		v, _ := d.Get(k)
		if f(k, v) {
			out.Put(k, v)
		}
	}
	return out
}

func DictFoldl[K, V, B any](f func(K, V, B) B, acc B, d rt.Dict[K, V]) B {
	for _, k := range d.Keys() {
		v, _ := d.Get(k)
		acc = f(k, v, acc)
	}
	return acc
}

func DictPartition[K, V any](f func(K, V) bool, d rt.Dict[K, V]) struct {
	Yes rt.Dict[K, V]
	No  rt.Dict[K, V]
} {
	var r struct {
		Yes rt.Dict[K, V]
		No  rt.Dict[K, V]
	}
	for _, k := range d.Keys() {
		v, _ := d.Get(k)
		if f(k, v) {
			r.Yes.Put(k, v)
		} else {
			r.No.Put(k, v)
		}
	}
	return r
}

// DictUnion keeps a's entries and order, then appends b's new keys. It builds one
// fresh Dict with Put (linear), so neither input changes.
func DictUnion[K, V any](a, b rt.Dict[K, V]) rt.Dict[K, V] {
	var out rt.Dict[K, V]
	av := a.Values()
	for i, k := range a.Keys() {
		out.Put(k, av[i])
	}
	bv := b.Values()
	for i, k := range b.Keys() {
		if !out.Has(k) {
			out.Put(k, bv[i])
		}
	}
	return out
}

// DictIntersect keeps a's entries whose key is in b.
func DictIntersect[K, V any](a, b rt.Dict[K, V]) rt.Dict[K, V] {
	return DictFilter(func(k K, _ V) bool { return b.Has(k) }, a)
}

// DictDiff keeps a's entries whose key is not in b.
func DictDiff[K, V any](a, b rt.Dict[K, V]) rt.Dict[K, V] {
	return DictFilter(func(k K, _ V) bool { return !b.Has(k) }, a)
}
