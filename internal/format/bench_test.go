package format

import "testing"

func BenchmarkSource_1000Lines(b *testing.B) {
	src := bigModule(b, 1000)
	if _, diags, err := Source("Big.alder", src); err != nil || len(diags) > 0 {
		b.Fatalf("generated module does not format: diags=%v err=%v", diags, err)
	}
	b.SetBytes(int64(len(src)))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, _, err := Source("Big.alder", src); err != nil {
			b.Fatal(err)
		}
	}
}
