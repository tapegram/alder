package jobs_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"alder/rt/db"
	"alder/rt/jobs"
)

// `app --jobs-only` (spec §7a): ALDER_JOBS defaults to 8 workers in that mode, and a shared
// database ahead of the build is waited out, looked at again every 10 s, before any claim.

func TestJobsOnly_DefaultWorkers(t *testing.T) {
	for v, want := range map[string]int{"": 8, "0": 0, "1": 1, " 12 ": 12} {
		t.Setenv(jobs.WorkersEnv, v)
		if n, err := jobs.JobsOnlyWorkersFromEnv(); err != nil || n != want {
			t.Errorf("ALDER_JOBS=%q: %d, %v; want %d", v, n, err, want)
		}
		if v == "" {
			if n, err := jobs.WorkersFromEnv(); err != nil || n != 4 {
				t.Errorf("serving, ALDER_JOBS unset: %d, %v; want 4", n, err)
			}
		}
	}
	for _, v := range []string{"x", "-1", "65536"} {
		t.Setenv(jobs.WorkersEnv, v)
		if _, err := jobs.JobsOnlyWorkersFromEnv(); err == nil || err.Error() != "ALDER_JOBS="+v+": want a whole number of job workers below 65536 (0: none)" {
			t.Errorf("ALDER_JOBS=%q: %v", v, err)
		}
	}
}

// fakeTimer is AwaitBuild's injected clock: each wait asked for is recorded, and the test fires it.
type fakeTimer struct {
	asked chan time.Duration
	fire  chan time.Time
}

func newFakeTimer() *fakeTimer {
	return &fakeTimer{asked: make(chan time.Duration, 16), fire: make(chan time.Time)}
}

func (f *fakeTimer) after(d time.Duration) <-chan time.Time {
	f.asked <- d
	return f.fire
}

// tick waits for the next wait to be asked for, checks it is 10 s, and ends it.
func (f *fakeTimer) tick(t *testing.T) {
	t.Helper()
	select {
	case d := <-f.asked:
		if d != 10*time.Second {
			t.Fatalf("waits %v, want 10s", d)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("no wait asked for")
	}
	f.fire <- time.Time{}
}

type answer struct {
	state  db.SharedState
	detail string
	err    error
}

// TestJobsOnly_Ahead: ahead, the schema is read again every 10 s (and not before the first 10 s:
// Open just read it); a failed read is a warn line and the wait goes on; equal ends the wait.
func TestJobsOnly_Ahead(t *testing.T) {
	answers := []answer{
		{db.SharedAhead, "a newer build migrated it (to 0003_x, which this build does not have)", nil},
		{"", "", errors.New("unreachable")},
		{db.SharedOK, "", nil},
	}
	checks := make(chan int, 8)
	n := 0
	check := func(context.Context) (db.SharedState, string, error) {
		a := answers[n]
		n++
		checks <- n
		return a.state, a.detail, a.err
	}
	ft := newFakeTimer()
	lg := &lines{}
	done := make(chan error, 1)
	go func() { done <- jobs.AwaitBuild(context.Background(), check, ft.after, lg.log) }()
	for i := 1; i <= 3; i++ {
		select {
		case <-checks:
			t.Fatalf("check %d before its 10 s", i)
		default:
		}
		ft.tick(t)
		if got := <-checks; got != i {
			t.Fatalf("check %d, want %d", got, i)
		}
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("still waiting once the database is this build's")
	}
	if got := lg.with("warn", "jobs: reading the database's schema: unreachable"); len(got) != 1 {
		t.Errorf("lines %q", lg.all)
	}
	if got := lg.with("info", "jobs: the database is this build's now: the job workers start"); len(got) != 1 {
		t.Errorf("lines %q", lg.all)
	}
}

// Behind once a wait looked again (a database restored to an older one): the web build's error;
// a cancelled context ends the wait with its error.
func TestJobsOnly_AheadThenBehind(t *testing.T) {
	ft := newFakeTimer()
	lg := &lines{}
	done := make(chan error, 1)
	check := func(context.Context) (db.SharedState, string, error) {
		return db.SharedBehind, "0002_x is not applied", nil
	}
	go func() { done <- jobs.AwaitBuild(context.Background(), check, ft.after, lg.log) }()
	ft.tick(t)
	if err := <-done; err == nil || err.Error() != "the database is behind this build: run `alder deploy`" {
		t.Fatalf("%v", err)
	}
	if got := lg.with("error", "the database: 0002_x is not applied"); len(got) != 1 {
		t.Errorf("lines %q", lg.all)
	}

	ctx, cancel := context.WithCancel(context.Background())
	ft = newFakeTimer()
	go func() {
		done <- jobs.AwaitBuild(ctx, func(context.Context) (db.SharedState, string, error) { return db.SharedAhead, "", nil }, ft.after, lg.log)
	}()
	<-ft.asked
	cancel()
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatalf("%v", err)
	}
	if strings.Contains(strings.Join(lg.all, "\n"), "start") {
		t.Errorf("lines %q", lg.all)
	}
}
