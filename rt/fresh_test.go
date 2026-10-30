package rt

import (
	"testing"
	"time"
)

type freshRec struct {
	Name  string
	Tags  []string
	Inner Maybe[[]int64]
	Rows  [][]byte
	At    time.Time
}

type freshUnion struct {
	Tag  int
	Next *freshUnion
	Xs   []int64
}

func TestFresh_CopiesEverySlice(t *testing.T) {
	b := []byte("hello")
	fb := Fresh(b)
	fb[0] = 'H'
	if string(b) != "hello" {
		t.Fatal("[]byte shared")
	}
	r := freshRec{Name: "n", Tags: []string{"a"}, Inner: Just([]int64{1}), Rows: [][]byte{[]byte("x")}, At: time.Unix(5, 0)}
	fr := Fresh(r)
	fr.Tags[0], fr.Inner.Val[0], fr.Rows[0][0] = "z", 9, 'y'
	if r.Tags[0] != "a" || r.Inner.Val[0] != 1 || string(r.Rows[0]) != "x" || !fr.At.Equal(r.At) || fr.Name != "n" {
		t.Fatalf("record shared: %+v", r)
	}
	u := &freshUnion{Next: &freshUnion{Xs: []int64{1}}}
	fu := Fresh(u)
	fu.Next.Xs[0] = 7
	if u.Next.Xs[0] != 1 {
		t.Fatal("union payload shared")
	}
	var d Dict[string, []int64]
	d = d.Insert("k", []int64{1})
	fd := Fresh(d)
	v, _ := fd.Get("k")
	v[0] = 5
	if w, _ := d.Get("k"); w[0] != 1 {
		t.Fatal("Dict value shared")
	}
	if Fresh([]int64(nil)) != nil || Fresh(Nothing[[]int64]()).Val != nil {
		t.Fatal("nil stays nil")
	}
	var a any = []int64{3}
	fa := Fresh(a)
	fa.([]int64)[0] = 4
	if a.([]int64)[0] != 3 {
		t.Fatal("interface shared")
	}
}

func TestFresh_NoSliceNoCopy(t *testing.T) {
	type plain struct {
		A string
		B int64
	}
	if n := testing.AllocsPerRun(10, func() { _ = Fresh(plain{"a", 1}); _ = Fresh("s"); _ = Fresh(int64(1)) }); n != 0 {
		t.Fatalf("Fresh of a slice-free value allocated %v times", n)
	}
}

// BenchmarkFresh_Bytes1MB is the cost the capability boundary adds for a 1 MB Bytes argument.
func BenchmarkFresh_Bytes1MB(b *testing.B) {
	body := make([]byte, 1<<20)
	b.ReportAllocs()
	b.SetBytes(int64(len(body)))
	for b.Loop() {
		_ = Fresh(body)
	}
}

// BenchmarkFresh_Record is a record argument holding a 10k-element list.
func BenchmarkFresh_Record(b *testing.B) {
	r := freshRec{Name: "n", Tags: make([]string, 10_000)}
	b.ReportAllocs()
	for b.Loop() {
		_ = Fresh(r)
	}
}
