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

// DictMember is not in stdlib/Dict.alder; it is provided for the emitter
// and for a future Dict.member.
func DictMember[K, V any](k K, d rt.Dict[K, V]) bool { return d.Has(k) }
