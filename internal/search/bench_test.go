package search

import (
	"fmt"
	"os"
	"sort"
	"testing"
	"time"
)

var budgetQueries = []string{"split", ": List Text", "List a -> Int -> List a", "Text -> _ -> _ -> _"}

// bigIndex is the stdlib plus 2,000 app declarations: copies of the stdlib's entries in modules
// App0, App1, ..., sharing the stdlib's declared types and base unifier.
func bigIndex(t testing.TB) *Index {
	t.Helper()
	std, err := Std()
	if err != nil {
		t.Fatal(err)
	}
	const appEntries = 2000
	ix := &Index{decls: std.decls, base: std.base}
	ix.Entries = append(ix.Entries, std.Entries...)
	for n := 0; n < appEntries; {
		for _, e := range std.Entries {
			if n == appEntries {
				break
			}
			e.Source = "app"
			e.Module = fmt.Sprintf("App%d", n/len(std.Entries))
			ix.Entries = append(ix.Entries, e)
			n++
		}
	}
	return ix
}

func BenchmarkSearch(b *testing.B) {
	ix := bigIndex(b)
	for _, q := range budgetQueries {
		b.Run(q, func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				ix.Search(q, Options{})
			}
		})
	}
}

func TestSearch_Budget(t *testing.T) {
	if os.Getenv("ALDER_SPEED_TESTS") != "1" {
		t.Skip("set ALDER_SPEED_TESTS=1 to run")
	}
	ix := bigIndex(t)
	for _, q := range budgetQueries {
		runs := make([]time.Duration, 5)
		for i := range runs {
			start := time.Now()
			ix.Search(q, Options{})
			runs[i] = time.Since(start)
		}
		sort.Slice(runs, func(i, j int) bool { return runs[i] < runs[j] })
		t.Logf("%q: median %v", q, runs[2])
		if runs[2] >= 50*time.Millisecond {
			t.Errorf("%q: median %v, budget 50ms", q, runs[2])
		}
	}
}
