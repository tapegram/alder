package jobs_test

import (
	"context"
	"fmt"
	"path/filepath"
	"sort"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"alder/rt/db"
	"alder/rt/eff"
	"alder/rt/jobs"
)

// BenchmarkJobs measures the worker against the real thing (#168, spec §6a, §7): a file-backed
// WAL SQLite store opened the way the app opens it (db.OpenSQLite: busy_timeout 5 s, BEGIN
// IMMEDIATE), real time, the default one-second poll. Throughput of the claim-and-finish loop
// alone, then the same drain while request-shaped writers hold the write lock, against the
// writers alone.
//
//	go test ./rt/jobs -run '^$' -bench BenchmarkJobs -benchtime=2000x -count 5 -timeout 20m
//
// (`-benchtime=Nx` sets b.N, the jobs drained; the requests-only runs last a fixed window.)
func BenchmarkJobs(b *testing.B) {
	for _, w := range []int{1, 4, 8} {
		b.Run(fmt.Sprintf("claim-finish/workers=%d", w), func(b *testing.B) {
			runBench(b, w, 0)
		})
	}
	for _, r := range []int{4, 16} {
		b.Run(fmt.Sprintf("with-requests/workers=4/writers=%d", r), func(b *testing.B) {
			runBench(b, 4, r)
		})
	}
	for _, r := range []int{4, 16} {
		b.Run(fmt.Sprintf("requests-only/writers=%d", r), func(b *testing.B) {
			runBench(b, 0, r)
		})
	}
}

// requestsOnlyWindow is how long the writers run with no workers.
const requestsOnlyWindow = 2 * time.Second

// epoch anchors the benchmark's own readings of the wall clock (time.Since); rt/jobs reads the
// clock nowhere else but Start's default.
var epoch = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

func since() time.Duration { return time.Since(epoch) }

// benchStore is a file-backed store as the app opens it, with noteTable and the jobs table.
func benchStore(b *testing.B) *db.SQLite {
	b.Helper()
	st, err := db.OpenSQLite(filepath.Join(b.TempDir(), "app.db"))
	if err != nil {
		b.Fatal(err)
	}
	b.Cleanup(func() { st.Close() })
	if err := st.Ensure(context.Background(), []db.TableDesc{noteTable}, true); err != nil {
		b.Fatal(err)
	}
	return st
}

// requestWrite is one request-shaped write: BEGIN IMMEDIATE, one insert, commit.
func requestWrite(ctx context.Context, st *db.SQLite) error {
	tx, err := st.Begin(ctx)
	if err != nil {
		return err
	}
	if _, err := st.Insert(ctx, tx, noteTable.Name, noteTable.Columns, "id", "", "", []any{nil, "request"}, epoch); err != nil {
		tx.Rollback()
		return err
	}
	return tx.Commit()
}

// queueJobs puts n due rows of kind name in the table in one transaction.
func queueJobs(b *testing.B, st *db.SQLite, name string, n int) {
	b.Helper()
	ctx := context.Background()
	tx, err := st.Begin(ctx)
	if err != nil {
		b.Fatal(err)
	}
	q, err := db.On(st, tx)
	if err != nil {
		b.Fatal(err)
	}
	for i := range n {
		if _, err := q.ExecContext(ctx, `INSERT INTO "_alder_jobs" ("id", "kind", "params", "state", "run_at", "created", "attempts") VALUES (?, ?, '{}', 'queued', 0, 0, 0)`,
			fmt.Sprintf("%08x", i), name); err != nil {
			b.Fatal(err)
		}
	}
	if err := tx.Commit(); err != nil {
		b.Fatal(err)
	}
}

func percentile(sorted []time.Duration, p float64) float64 {
	if len(sorted) == 0 {
		return 0
	}
	return float64(sorted[int(float64(len(sorted)-1)*p)]) / float64(time.Millisecond)
}

// runBench drains b.N jobs with workers workers (0: none) while writers goroutines (0: none) run
// request-shaped writes, and reports what it measured. With no workers the writers run for
// requestsOnlyWindow instead.
func runBench(b *testing.B, workers, writers int) {
	st := benchStore(b)
	const name = "Bench.Insert"
	kind := jobs.Kind{
		Name:    name,
		Run:     func(*eff.Env, string) (eff.Task[struct{}], error) { return insertNote("job"), nil },
		Retries: 4,
		Timeout: 5 * time.Minute,
	}
	jobsN := b.N
	if workers == 0 {
		jobsN = 0
	} else {
		queueJobs(b, st, name, jobsN)
	}

	var done atomic.Int64
	drained := make(chan struct{})
	var once sync.Once
	stop := func(time.Duration) {}
	if workers > 0 {
		stop = jobs.Start(context.Background(), jobs.Options{
			Store: st,
			Kinds: []jobs.Kind{kind},
			Env: func(ctx context.Context, k jobs.Kind) *eff.Env {
				return &eff.Env{Ctx: ctx, Store: st}
			},
			Workers: workers,
			OnAttempt: func(o jobs.Outcome) {
				if o.Err != nil {
					b.Errorf("attempt of %s failed: %v", o.ID, o.Err)
				}
				if done.Add(1) == int64(jobsN) {
					once.Do(func() { close(drained) })
				}
			},
		})
	}

	var (
		quit  = make(chan struct{})
		wg    sync.WaitGroup
		mu    sync.Mutex
		lats  []time.Duration
		fails atomic.Int64
	)
	ctx := context.Background()
	b.ResetTimer()
	begin := since()
	for range writers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			var mine []time.Duration
			for {
				select {
				case <-quit:
					mu.Lock()
					lats = append(lats, mine...)
					mu.Unlock()
					return
				default:
				}
				t := since()
				if err := requestWrite(ctx, st); err != nil {
					fails.Add(1)
					b.Logf("request write: %v", err)
					continue
				}
				mine = append(mine, since()-t)
			}
		}()
	}
	if workers > 0 {
		select {
		case <-drained:
		case <-time.After(10 * time.Minute):
			b.Errorf("only %d of %d jobs ran", done.Load(), jobsN)
		}
	} else {
		time.Sleep(requestsOnlyWindow)
	}
	elapsed := since() - begin
	close(quit)
	wg.Wait()
	b.StopTimer()
	stop(0)

	secs := elapsed.Seconds()
	if workers > 0 {
		b.ReportMetric(float64(done.Load())/secs, "jobs/s")
	}
	if writers > 0 {
		sort.Slice(lats, func(i, j int) bool { return lats[i] < lats[j] })
		b.ReportMetric(float64(len(lats))/secs, "req-writes/s")
		b.ReportMetric(percentile(lats, 0.50), "req-p50-ms")
		b.ReportMetric(percentile(lats, 0.99), "req-p99-ms")
		// the tail beyond p99: a writer that waits out busy_timeout while others go on shows here
		b.ReportMetric(percentile(lats, 0.999), "req-p99.9-ms")
		b.ReportMetric(percentile(lats, 1), "req-max-ms")
		b.ReportMetric(float64(fails.Load()), "req-failures")
	}
}
