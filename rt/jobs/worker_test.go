package jobs_test

import (
	"context"
	"fmt"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"alder/rt"
	"alder/rt/db"
	"alder/rt/eff"
	"alder/rt/jobs"
	rtesting "alder/rt/testing"
)

// openFile is a file-backed store (BEGIN IMMEDIATE, several connections, as in production)
// with noteTable and the jobs table.
func openFile(t *testing.T) *db.SQLite {
	t.Helper()
	st, err := db.OpenSQLite(filepath.Join(t.TempDir(), "app.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	if err := st.Ensure(context.Background(), []db.TableDesc{noteTable}, true); err != nil {
		t.Fatal(err)
	}
	return st
}

// lines collects log lines from several goroutines.
type lines struct {
	mu  sync.Mutex
	all []string // "level msg"
}

func (l *lines) log(level, msg string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.all = append(l.all, level+" "+msg)
}

// with is the lines at level that contain s.
func (l *lines) with(level, s string) []string {
	l.mu.Lock()
	defer l.mu.Unlock()
	var out []string
	for _, x := range l.all {
		if strings.HasPrefix(x, level+" ") && strings.Contains(x, s) {
			out = append(out, x)
		}
	}
	return out
}

// eventually fails the test when cond does not hold within 5 s.
func eventually(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.After(5 * time.Second)
	for !cond() {
		select {
		case <-deadline:
			t.Fatalf("timed out waiting for %s", what)
		case <-time.After(time.Millisecond):
		}
	}
}

// start starts workers on st whose Env reads clock c and logs to lg; the test's cleanup stops
// them (again: stop does nothing the second time).
func start(t *testing.T, st *db.SQLite, c *rtesting.Clock, lg *lines, workers int, poll time.Duration, kinds ...jobs.Kind) func(time.Duration) {
	t.Helper()
	stop := jobs.Start(context.Background(), jobs.Options{
		Store: st,
		Kinds: kinds,
		Env: func(ctx context.Context, k jobs.Kind) *eff.Env {
			return &eff.Env{Ctx: ctx, Store: st, Now: c.Now, Log: lg.log}
		},
		Workers:   workers,
		Now:       c.Now,
		Log:       lg.log,
		Poll:      poll,
		Housekeep: time.Hour,
	})
	t.Cleanup(func() { stop(0) })
	return stop
}

// insertRow writes a job row as another process would have left it.
func insertRow(t *testing.T, st *db.SQLite, id, kind, state string, runAt time.Time, attempts int, finished, leaseUntil *time.Time) {
	t.Helper()
	ms := func(p *time.Time) any {
		if p == nil {
			return nil
		}
		return p.UnixMilli()
	}
	if _, err := raw(t, st).ExecContext(context.Background(), `INSERT INTO "_alder_jobs" ("id", "kind", "params", "state", "run_at", "created", "attempts", "finished", "lease_until") VALUES (?, ?, '{}', ?, ?, ?, ?, ?, ?)`,
		id, kind, state, runAt.UnixMilli(), runAt.UnixMilli(), attempts, ms(finished), ms(leaseUntil)); err != nil {
		t.Fatal(err)
	}
}

func exists(t *testing.T, st *db.SQLite, id string) bool {
	return count(t, st, `SELECT COUNT(*) FROM "_alder_jobs" WHERE "id" = ?`, id) == 1
}

func TestClaim_Atomic(t *testing.T) {
	st := openFile(t)
	env, _ := newEnv(st)
	for range 50 {
		runEnqueue(t, env, welcome, nil)
	}
	var (
		mu      sync.Mutex
		claimed = map[string]int{}
		wg      sync.WaitGroup
		errs    = make(chan error, 100)
	)
	for w := range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range 50 {
				r, err := jobs.Claim(context.Background(), st, []string{welcome.Kind}, t0, time.Minute, fmt.Sprintf("test:%d", w))
				if err != nil {
					errs <- err
					return
				}
				if r != nil {
					mu.Lock()
					claimed[r.ID]++
					mu.Unlock()
				}
			}
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatal(err)
	}
	if len(claimed) != 50 {
		t.Fatalf("%d rows claimed, want 50", len(claimed))
	}
	for id, n := range claimed {
		if n != 1 {
			t.Fatalf("row %s claimed %d times", id, n)
		}
	}
	if n := count(t, st, `SELECT COUNT(*) FROM "_alder_jobs" WHERE "state" = 'running' AND "attempts" = 1`); n != 50 {
		t.Fatalf("%d running rows with one attempt, want 50", n)
	}
}

func TestClaim_SkipsUnknownKinds(t *testing.T) {
	st := openFile(t)
	env, _ := newEnv(st)
	other := runEnqueue(t, env, rt.Job{Kind: "Old.Gone", Params: "{}"}, nil)
	id := runEnqueue(t, env, welcome, nil)
	r, err := jobs.Claim(context.Background(), st, []string{welcome.Kind, "Other.Known"}, t0, time.Minute, "h:1:1")
	if err != nil || r == nil || r.ID != id {
		t.Fatalf("claim: %+v %v, want %s", r, err, id)
	}
	if r.State != "running" || r.Attempts != 1 || r.Worker != "h:1:1" || r.LeaseUntil == nil || *r.LeaseUntil != t0.Add(time.Minute).UnixMilli() ||
		r.Kind != welcome.Kind || r.Params != welcome.Params || r.RunAt != t0.UnixMilli() {
		t.Fatalf("claimed row: %+v", r)
	}
	if r, err := jobs.Claim(context.Background(), st, []string{welcome.Kind}, t0, time.Minute, "h:1:1"); r != nil || err != nil {
		t.Fatalf("second claim: %+v %v", r, err)
	}
	if s := read(t, st, other); s.state != "queued" || s.attempts != 0 {
		t.Fatalf("unknown kind's row: %+v", s)
	}
	if r, err := jobs.Claim(context.Background(), st, nil, t0, time.Minute, "h:1:1"); r != nil || err != nil {
		t.Fatalf("no kinds: %+v %v", r, err)
	}
}

func TestClaim_NotBeforeRunAt(t *testing.T) {
	st := openFile(t)
	env, _ := newEnv(st)
	later, err := eff.Run(env, eff.KTask, eff.Call("Job.enqueueAt", true, func(e *eff.Env) (string, error) {
		return jobs.Enqueue(e, welcome, t0.Add(time.Minute), nil)
	}))
	if err != nil {
		t.Fatal(err)
	}
	if r, err := jobs.Claim(context.Background(), st, []string{welcome.Kind}, t0.Add(time.Minute-time.Millisecond), time.Minute, "w"); r != nil || err != nil {
		t.Fatalf("before run_at: %+v %v", r, err)
	}
	now := runEnqueue(t, env, welcome, nil) // due at t0: claimed first, though enqueued last
	for _, want := range []string{now, later} {
		r, err := jobs.Claim(context.Background(), st, []string{welcome.Kind}, t0.Add(time.Minute), time.Minute, "w")
		if err != nil || r == nil || r.ID != want {
			t.Fatalf("claim: %+v %v, want %s", r, err, want)
		}
	}
}

// A committed enqueue wakes a worker: with a poll of an hour, the row is done at once.
func TestWorker_RunsEnqueued(t *testing.T) {
	st := openFile(t)
	env, c := newEnv(st)
	lg := &lines{}
	jobs.Woken() // a signal left by another test
	stop := start(t, st, c, lg, 2, time.Hour, kind(insertNote("hi")))
	time.Sleep(20 * time.Millisecond) // the workers find nothing at start and wait
	id := runEnqueue(t, env, welcome, nil)
	eventually(t, "the job done", func() bool { return read(t, st, id).state == "done" })
	stop(time.Second)
	if n := notes(t, st); n != 1 {
		t.Fatalf("notes: %d", n)
	}
}

// Enqueue wakes the workers after its Task commits, and not when the Task fails.
func TestEnqueue_WakesAfterCommit(t *testing.T) {
	st := open(t)
	env, _ := newEnv(st)
	jobs.Woken()
	failing := eff.Bind(enqueue(welcome, nil), func(string) eff.Task[struct{}] { return eff.Fail[struct{}]("no") })
	if _, err := eff.Run(env, eff.KTask, failing); err == nil {
		t.Fatal("want the Task's failure")
	}
	if jobs.Woken() {
		t.Fatal("a rolled-back enqueue woke the workers")
	}
	runEnqueue(t, env, welcome, nil)
	if !jobs.Woken() {
		t.Fatal("a committed enqueue did not wake the workers")
	}
}

// Review Focus 3: a job that panics on every attempt is logged with its stack each time,
// retried with the backoff, and dead after the fifth; the worker runs the next row meanwhile.
func TestWorker_PanicSurvives(t *testing.T) {
	st := openFile(t)
	env, c := newEnv(st)
	lg := &lines{}
	boom := kind(eff.Call("Boom", false, func(*eff.Env) (struct{}, error) { panic("boom") }))
	fine := kind(insertNote("fine"))
	fine.Name = "Other.Fine"
	start(t, st, c, lg, 1, 2*time.Millisecond, boom, fine)
	id := runEnqueue(t, env, welcome, nil)
	eventually(t, "the first attempt", func() bool { return read(t, st, id).attempts == 1 && read(t, st, id).state == "queued" })
	healthy := runEnqueue(t, env, rt.Job{Kind: "Other.Fine", Params: "{}"}, nil)
	eventually(t, "the healthy job done", func() bool { return read(t, st, healthy).state == "done" })
	for attempt := 2; attempt <= 5; attempt++ {
		c.Advance(time.Hour) // past every backoff
		eventually(t, fmt.Sprintf("attempt %d", attempt), func() bool {
			r := read(t, st, id)
			return r.attempts == attempt && r.state != "running"
		})
	}
	r := read(t, st, id)
	if r.state != "dead" || r.err == nil || *r.err != "boom" {
		t.Fatalf("after 5 attempts: %+v", r)
	}
	logged := lg.with("error", "panic: boom")
	if len(logged) != 5 {
		t.Fatalf("%d error lines with the panic, want 5: %q", len(logged), lg.all)
	}
	if !strings.Contains(logged[0], "runtime/debug.Stack") {
		t.Fatalf("no stack: %q", logged[0])
	}
}

// A running row past its lease is re-queued by housekeeping, its attempt counted; past its
// retries it is dead.
func TestWorker_LeaseExpired(t *testing.T) {
	st := openFile(t)
	c := rtesting.NewClock(t0)
	c.Set(t0)
	past := t0.Add(-time.Second)
	insertRow(t, st, "aaaa0001", welcome.Kind, "running", t0.Add(-time.Hour), 2, nil, &past)
	insertRow(t, st, "aaaa0002", welcome.Kind, "running", t0.Add(-time.Hour), 5, nil, &past)
	future := t0.Add(time.Second)
	insertRow(t, st, "aaaa0003", welcome.Kind, "running", t0.Add(-time.Hour), 1, nil, &future)
	lg := &lines{}
	stop := start(t, st, c, lg, 1, time.Hour, kind(eff.Pure(struct{}{})))
	eventually(t, "housekeeping", func() bool { return read(t, st, "aaaa0002").state != "running" })
	stop(0)
	if r := read(t, st, "aaaa0001"); r.state != "queued" || r.attempts != 2 || r.err == nil || *r.err != "lost: the process stopped" ||
		r.leaseUntil != nil || r.runAt != t0.Add(jobs.Backoff(2)).UnixMilli() {
		t.Fatalf("lost row: %+v", r)
	}
	if r := read(t, st, "aaaa0002"); r.state != "dead" || r.attempts != 5 || r.err == nil || *r.err != "lost: the process stopped" {
		t.Fatalf("lost row past its retries: %+v", r)
	}
	if r := read(t, st, "aaaa0003"); r.state != "running" {
		t.Fatalf("row within its lease: %+v", r)
	}
	if n := len(lg.with("warn", "lost: the process stopped")); n != 2 {
		t.Fatalf("%d warn lines, want 2: %q", n, lg.all)
	}
}

// stop cancels the attempts still running after its grace: the row is queued as before its
// claim, and nothing is claimed after stop.
func TestWorker_ShutdownRequeues(t *testing.T) {
	st := openFile(t)
	env, c := newEnv(st)
	started := make(chan struct{}, 1)
	blocked := kind(eff.Call("Block", false, func(e *eff.Env) (struct{}, error) {
		started <- struct{}{}
		<-e.Ctx.Done()
		return struct{}{}, e.Ctx.Err()
	}))
	id := runEnqueue(t, env, welcome, nil)
	if _, err := raw(t, st).ExecContext(context.Background(), `UPDATE "_alder_jobs" SET "attempts" = 1, "error" = 'earlier' WHERE "id" = ?`, id); err != nil {
		t.Fatal(err)
	}
	var outs []jobs.Outcome
	var mu sync.Mutex
	stop := jobs.Start(context.Background(), jobs.Options{
		Store: st, Kinds: []jobs.Kind{blocked}, Workers: 2, Now: c.Now, Poll: 2 * time.Millisecond,
		Env:       func(ctx context.Context, k jobs.Kind) *eff.Env { return &eff.Env{Ctx: ctx, Store: st, Now: c.Now} },
		OnAttempt: func(o jobs.Outcome) { mu.Lock(); outs = append(outs, o); mu.Unlock() },
	})
	t.Cleanup(func() { stop(0) })
	<-started
	stop(10 * time.Millisecond)
	if r := read(t, st, id); r.state != "queued" || r.attempts != 1 || r.err != nil || r.leaseUntil != nil || r.runAt != t0.UnixMilli() {
		t.Fatalf("after stop: %+v", r)
	}
	mu.Lock()
	if len(outs) != 1 || !outs[0].Stopped || outs[0].Dead {
		t.Fatalf("outcomes: %+v", outs)
	}
	mu.Unlock()
	after := runEnqueue(t, env, welcome, nil)
	time.Sleep(10 * time.Millisecond) // five polls
	if r := read(t, st, after); r.state != "queued" || r.attempts != 0 {
		t.Fatalf("claimed after stop: %+v", r)
	}
	if r := read(t, st, id); r.state != "queued" {
		t.Fatalf("re-claimed after stop: %+v", r)
	}
}

// stop waits up to its grace for an attempt, which then finishes; a grace of 0 or less cancels
// at once.
func TestWorker_StopGrace(t *testing.T) {
	st := openFile(t)
	env, c := newEnv(st)
	lg := &lines{}
	started := make(chan struct{}, 1)
	// waits is a kind whose attempt waits for release, or for its cancellation
	waits := func(release chan struct{}) jobs.Kind {
		return kind(then(eff.Call("Wait", false, func(e *eff.Env) (struct{}, error) {
			started <- struct{}{}
			select {
			case <-release:
				return struct{}{}, nil
			case <-e.Ctx.Done():
				return struct{}{}, e.Ctx.Err()
			}
		}), insertNote("finished")))
	}
	release := make(chan struct{})
	stop := start(t, st, c, lg, 1, 2*time.Millisecond, waits(release))
	id := runEnqueue(t, env, welcome, nil)
	<-started
	stopped := make(chan struct{})
	go func() { stop(5 * time.Second); close(stopped) }()
	time.Sleep(5 * time.Millisecond)
	close(release)
	<-stopped
	if r := read(t, st, id); r.state != "done" || notes(t, st) != 1 {
		t.Fatalf("within the grace: %+v", r)
	}

	stop = start(t, st, c, lg, 1, 2*time.Millisecond, waits(make(chan struct{})))
	id = runEnqueue(t, env, welcome, nil)
	<-started
	stopped = make(chan struct{})
	go func() { stop(-time.Second); close(stopped) }()
	select {
	case <-stopped:
	case <-time.After(time.Second):
		t.Fatal("stop(-1s) waited")
	}
	if r := read(t, st, id); r.state != "queued" || r.attempts != 0 {
		t.Fatalf("no grace: %+v", r)
	}
}

func TestHousekeep_Prune(t *testing.T) {
	st := openFile(t)
	day := 24 * time.Hour
	at := func(d time.Duration) *time.Time { x := t0.Add(-d); return &x }
	insertRow(t, st, "d0000008", welcome.Kind, "done", t0.Add(-9*day), 1, at(8*day), nil)
	insertRow(t, st, "d0000006", welcome.Kind, "done", t0.Add(-9*day), 1, at(6*day), nil)
	insertRow(t, st, "x0000031", welcome.Kind, "dead", t0.Add(-40*day), 5, at(31*day), nil)
	insertRow(t, st, "x0000029", welcome.Kind, "dead", t0.Add(-40*day), 5, at(29*day), nil)
	insertRow(t, st, "q0000100", welcome.Kind, "queued", t0.Add(-100*day), 0, nil, nil)
	lg := &lines{}
	jobs.Housekeeper(st, []jobs.Kind{kind(eff.Pure(struct{}{}))}, lg.log)(context.Background(), t0)
	for id, want := range map[string]bool{"d0000008": false, "d0000006": true, "x0000031": false, "x0000029": true, "q0000100": true} {
		if exists(t, st, id) != want {
			t.Errorf("row %s kept: %v, want %v", id, !want, want)
		}
	}
	if len(lg.all) != 0 {
		t.Fatalf("log: %q", lg.all)
	}
}

func TestHousekeep_Unclaimable(t *testing.T) {
	st := openFile(t)
	insertRow(t, st, "u0000001", "Gone.Job", "queued", t0.Add(-25*time.Hour), 0, nil, nil)
	insertRow(t, st, "u0000002", "Gone.Job", "queued", t0.Add(-30*time.Hour), 0, nil, nil)
	insertRow(t, st, "u0000003", "Recent.Job", "queued", t0.Add(-23*time.Hour), 0, nil, nil)
	insertRow(t, st, "u0000004", welcome.Kind, "queued", t0.Add(-25*time.Hour), 0, nil, nil)
	lg := &lines{}
	keep := jobs.Housekeeper(st, []jobs.Kind{kind(eff.Pure(struct{}{}))}, lg.log)
	keep(context.Background(), t0)
	keep(context.Background(), t0.Add(time.Minute))
	if len(lg.all) != 1 || len(lg.with("warn", "kind Gone.Job")) != 1 || !strings.Contains(lg.all[0], "2 job(s)") {
		t.Fatalf("log: %q, want one warn line naming Gone.Job", lg.all)
	}
}

func TestWorker_ZeroWorkers(t *testing.T) {
	st := openFile(t)
	env, c := newEnv(st)
	id := runEnqueue(t, env, welcome, nil)
	before := runtime.NumGoroutine()
	called := false
	stop := jobs.Start(context.Background(), jobs.Options{
		Store: st, Kinds: []jobs.Kind{kind(eff.Pure(struct{}{}))}, Workers: 0, Now: c.Now, Poll: time.Millisecond,
		Env: func(context.Context, jobs.Kind) *eff.Env { called = true; return nil },
	})
	if n := runtime.NumGoroutine(); n != before {
		t.Fatalf("goroutines: %d, before %d", n, before)
	}
	time.Sleep(5 * time.Millisecond)
	stop(time.Second)
	if r := read(t, st, id); r.state != "queued" || r.attempts != 0 || called {
		t.Fatalf("row %+v, Env called %v", r, called)
	}
}
