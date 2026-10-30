package rt

import (
	"math"
	"reflect"
	"strconv"
	"testing"
)

func TestDictIntKeys(t *testing.T) {
	ks := []int64{0, -1, 1, math.MinInt64, math.MaxInt64}
	var d Dict[int64, string]
	for _, k := range ks {
		d = d.Insert(k, strconv.FormatInt(k, 10))
	}
	if d.Len() != 5 || !reflect.DeepEqual(d.Keys(), ks) {
		t.Fatalf("keys = %v", d.Keys())
	}
	for _, k := range ks {
		if v, ok := d.Get(k); !ok || v != strconv.FormatInt(k, 10) || !d.Has(k) {
			t.Fatalf("Get(%d) = %q, %v", k, v, ok)
		}
	}
	if d.Has(2) || d.Has(-2) {
		t.Fatal("Has of an absent key")
	}
	d = d.Insert(-1, "again")
	if v, _ := d.Get(-1); v != "again" || !reflect.DeepEqual(d.Keys(), ks) {
		t.Fatalf("re-insert moved the key or kept the value: %v %q", d.Keys(), v)
	}
	if v, _ := d.Get(1); v != "1" {
		t.Fatal("-1 and 1 are one key")
	}
	r := d.Remove(math.MinInt64)
	if r.Has(math.MinInt64) || r.Len() != 4 || !reflect.DeepEqual(r.Keys(), []int64{0, -1, 1, math.MaxInt64}) {
		t.Fatalf("Remove: %v", r.Keys())
	}
	if !d.Has(math.MinInt64) {
		t.Fatal("Remove changed the receiver")
	}
	if d.Remove(42).Len() != 5 {
		t.Fatal("Remove of an absent key")
	}
}

func TestDictTextKeys(t *testing.T) {
	var d Dict[string, int64]
	d = d.Insert("b", 1).Insert("", 2).Insert("a", 3)
	if v, ok := d.Get(""); !ok || v != 2 || !reflect.DeepEqual(d.Keys(), []string{"b", "", "a"}) {
		t.Fatalf("empty key: %v %v %v", v, ok, d.Keys())
	}
	if d.Remove("").Has("") || d.Remove("").Len() != 2 {
		t.Fatal("Remove \"\"")
	}
}

// The fast path is chosen by the Dict's key type, not by a key's dynamic type: in a Dict whose
// key type is an interface, a Text and an Int with the same 8 bytes stay distinct.
func TestDictAnyKeyType(t *testing.T) {
	var d Dict[any, int64]
	d = d.Insert("\x00\x00\x00\x00\x00\x00\x00\x05", 1).Insert(int64(5), 2)
	if d.Len() != 2 {
		t.Fatalf("Len = %d, want 2", d.Len())
	}
	if v, _ := d.Get(int64(5)); v != 2 {
		t.Fatal("Get Int")
	}
}

func TestDictAsKey_OrderInsensitive(t *testing.T) {
	var ab, ba Dict[string, int64]
	ab = ab.Insert("a", 1).Insert("b", 2)
	ba = ba.Insert("b", 2).Insert("a", 1)
	if Key(ab) != Key(ba) {
		t.Fatalf("%q != %q", Key(ab), Key(ba))
	}
	var outer Dict[Dict[string, int64], string]
	outer = outer.Insert(ab, "x").Insert(ba, "y")
	if outer.Len() != 1 {
		t.Fatal("two equal dicts are two keys")
	}
	var ia, ib Dict[int64, int64]
	ia, ib = ia.Insert(-1, 0).Insert(1, 0), ib.Insert(1, 0).Insert(-1, 0)
	if Key(ia) != Key(ib) || Key(ia) == Key(ia.Insert(2, 0)) {
		t.Fatal("Int-keyed dict as a key")
	}
}

// Key's encoding is unchanged by the fast path.
func TestKey_Unchanged(t *testing.T) {
	for k, want := range map[any]string{"a": "s1:a", "": "s0:", int64(5): "i5;", int64(-3): "i-3;"} {
		if got := Key(k); got != want {
			t.Errorf("Key(%#v) = %q, want %q", k, got, want)
		}
	}
	if got := Key([]string{"a"}); got != "[1:s1:a]" {
		t.Errorf("Key list = %q", got)
	}
}

func TestDictGet_Allocs(t *testing.T) {
	var ds Dict[string, int64]
	var di Dict[int64, int64]
	for i := range 100 {
		ds.Put(strconv.Itoa(i), int64(i))
		di.Put(int64(i), int64(i))
	}
	k := "42"
	n := testing.AllocsPerRun(100, func() {
		_, _ = ds.Get(k)
		_ = ds.Has(k)
		_, _ = di.Get(42)
		_ = di.Has(-7)
	})
	if n != 0 {
		t.Fatalf("Get/Has allocated %v times", n)
	}
}

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
