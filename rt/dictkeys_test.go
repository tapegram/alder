package rt

import (
	"strconv"
	"testing"
)

type recKey struct {
	A string
	B int64
}

func benchDictGet[K any](b *testing.B, key func(i int) K) {
	var d Dict[K, int64]
	ks := make([]K, 1000)
	for i := range ks {
		ks[i] = key(i)
		d.Put(ks[i], int64(i))
	}
	b.ReportAllocs()
	var sum int64
	i := 0
	for b.Loop() {
		v, _ := d.Get(ks[i])
		sum += v
		i++
		if i == len(ks) {
			i = 0
		}
	}
	_ = sum
}

// BenchmarkDictGet_* look up a 1000-key Dict, cycling keys (#37: Text and Int keys skip reflection).
func BenchmarkDictGet_Text(b *testing.B) {
	benchDictGet(b, func(i int) string { return "key-" + strconv.Itoa(i) })
}

func BenchmarkDictGet_Int(b *testing.B) {
	benchDictGet(b, func(i int) int64 { return int64(i) * 7919 })
}

func BenchmarkDictGet_Record(b *testing.B) {
	benchDictGet(b, func(i int) recKey { return recKey{"k" + strconv.Itoa(i%10), int64(i)} })
}
