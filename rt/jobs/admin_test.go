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

// Retry takes a dead row only, keeps a key another live row holds, and RunNow a queued row only.
func TestAdmin_RetryAndRunNow(t *testing.T) {
	st := open(t)
	ctx := context.Background()
	q, err := db.On(st, nil)
	if err != nil {
		t.Fatal(err)
	}
	t0 := time.Date(2026, 11, 16, 12, 0, 0, 0, time.UTC)
	for _, r := range []struct{ id, state, key string }{{"dead0001", "dead", "k1"}, {"live0001", "queued", "k1"}, {"dead0002", "dead", ""}, {"done0001", "done", ""}} {
		var key any
		if r.key != "" {
			key = r.key
		}
		if _, err := q.ExecContext(ctx, `INSERT INTO "_alder_jobs" ("id", "kind", "params", "key", "state", "run_at", "created", "attempts", "error", "finished", "worker") VALUES (?, 'K.J', '{}', ?, ?, ?, ?, 3, 'boom', ?, 'w:1:1')`,
			r.id, key, r.state, t0.UnixMilli(), t0.UnixMilli(), t0.UnixMilli()); err != nil {
			t.Fatal(err)
		}
	}
	now := t0.Add(time.Hour)
	if err := jobs.Retry(ctx, q, "dead0001", now); err == nil || err.Error() != `job dead0001 is not retried: job live0001 of K.J holds its key "k1" (queued or running)` {
		t.Errorf("retry of a key held: %v", err)
	}
	var se *jobs.StateError
	if err := jobs.Retry(ctx, q, "done0001", now); !errors.As(err, &se) || err.Error() != "job done0001 is done: only a dead job is retried" {
		t.Errorf("retry of a done row: %v", err)
	}
	if err := jobs.Retry(ctx, q, "nosuch01", now); !errors.Is(err, jobs.ErrNoJob) {
		t.Errorf("retry of no row: %v", err)
	}
	if err := jobs.Retry(ctx, q, "dead0002", now); err != nil {
		t.Fatal(err)
	}
	r, err := jobs.Get(ctx, q, "dead0002")
	if err != nil || r.State != "queued" || r.Attempts != 0 || r.Error != nil || r.RunAt != now.UnixMilli() || r.Finished != nil || r.Worker != "" {
		t.Errorf("retried: %+v %v", r, err)
	}
	if err := jobs.RunNow(ctx, q, "done0001", now); !errors.As(err, &se) || err.Error() != "job done0001 is done: only a queued job is run now" {
		t.Errorf("run now of a done row: %v", err)
	}
	if err := jobs.RunNow(ctx, q, "live0001", now); err != nil {
		t.Fatal(err)
	}
	if r, _ := jobs.Get(ctx, q, "live0001"); r.RunAt != now.UnixMilli() {
		t.Errorf("run now: %+v", r)
	}
	l, err := jobs.List(ctx, q)
	if err != nil || len(l.Queued) != 2 || len(l.Dead) != 1 || len(l.Done) != 1 || len(l.Running) != 0 {
		t.Errorf("list: %+v %v", l, err)
	}
}

func TestKind_ScheduleText(t *testing.T) {
	for k, want := range map[*jobs.Kind]string{
		{Every: time.Hour}:        "every 1h",
		{Every: 48 * time.Hour}:   "every 2d",
		{Every: 90 * time.Second}: "every 90s",
		{Cron: "0 3 * * *"}:       `cron "0 3 * * *"`,
		{}:                        "",
	} {
		if got := k.ScheduleText(); got != want {
			t.Errorf("%+v: %q, want %q", *k, got, want)
		}
	}
}

// The panel lists the rows of the silos open beside the shared store's, each with its silo, and
// retries a silo's row in its silo.
func TestPanel_Silos(t *testing.T) {
	ctx := context.Background()
	dir, shared, s := siloApp(t)
	inA := runEnqueue(t, orgEnv(t, shared, s, "A"), welcome, nil)
	inShared := runEnqueue(t, sharedEnv(shared, s), welcome, nil)
	p := &jobs.Panel{Store: shared, Stores: s, Kinds: []jobs.Kind{{Name: welcome.Kind}}, Now: func() time.Time { return t0.Add(time.Hour) }}
	l, err := p.List(ctx)
	if err != nil || len(l.Queued) != 2 || len(l.Missed) != 0 {
		t.Fatalf("list: %+v %v", l, err)
	}
	for _, r := range l.Queued {
		if want := map[string]string{inA: "A", inShared: ""}[r.ID]; r.In.Value != want || (want != "") != (r.In.Key == "org") {
			t.Errorf("row %s in %q/%q, want silo %q", r.ID, r.In.Key, r.In.Value, want)
		}
	}
	if _, err := raw(t, siloFile(t, dir, "A")).ExecContext(ctx, `UPDATE "_alder_jobs" SET "state" = 'dead', "attempts" = 5, "error" = 'boom' WHERE "id" = ?`, inA); err != nil {
		t.Fatal(err)
	}
	if err := p.Retry(ctx, inA); err != nil {
		t.Fatal(err)
	}
	r, err := p.Get(ctx, inA)
	if err != nil || r == nil || r.State != "queued" || r.Attempts != 0 || r.In.Value != "A" {
		t.Errorf("after retry: %+v %v", r, err)
	}
}

// A silo's row whose retry or run now is refused gives the refusal, whether the panel reads the
// silo open (OpenSilos) or through the workers' poll of their active set (a silo not open): a
// *StateError for a row in another state, the key held, and ErrNoJob for no row.
func TestPanel_SiloErrors(t *testing.T) {
	ctx := context.Background()
	now := func() time.Time { return t0.Add(time.Hour) }
	dir, shared, s1 := siloApp(t)
	// org A's silo: a done row, a dead one whose key a queued one holds
	runEnqueue(t, orgEnv(t, shared, s1, "A"), welcome, nil) // creates A's database
	a := raw(t, siloFile(t, dir, "A"))
	for _, r := range []struct{ id, state, key string }{{"doneA001", "done", ""}, {"deadA001", "dead", "k"}, {"liveA001", "queued", "k"}} {
		var key any
		if r.key != "" {
			key = r.key
		}
		if _, err := a.ExecContext(ctx, `INSERT INTO "_alder_jobs" ("id", "kind", "params", "key", "state", "run_at", "created", "attempts") VALUES (?, 'K.J', '{}', ?, ?, ?, ?, 1)`,
			r.id, key, r.state, t0.UnixMilli(), t0.UnixMilli()); err != nil {
			t.Fatal(err)
		}
	}
	s1.Close()
	cases := []struct {
		name string
		op   func(*jobs.Panel) error
		want func(error) bool
	}{
		{"retry of a done row", func(p *jobs.Panel) error { return p.Retry(ctx, "doneA001") }, func(err error) bool {
			var se *jobs.StateError
			return errors.As(err, &se) && se.State == jobs.Done
		}},
		{"run now of a done row", func(p *jobs.Panel) error { return p.RunNow(ctx, "doneA001") }, func(err error) bool {
			var se *jobs.StateError
			return errors.As(err, &se) && se.Want == jobs.Queued
		}},
		{"retry of a key held", func(p *jobs.Panel) error { return p.Retry(ctx, "deadA001") }, func(err error) bool {
			return err != nil && err.Error() == `job deadA001 is not retried: job liveA001 of K.J holds its key "k" (queued or running)`
		}},
		{"retry of no row", func(p *jobs.Panel) error { return p.Retry(ctx, "nosuch01") }, func(err error) bool { return errors.Is(err, jobs.ErrNoJob) }},
	}
	for _, via := range []string{"open silo", "active set"} {
		for _, c := range cases {
			s := reopened(t, dir, shared) // no silo open
			p := &jobs.Panel{Store: shared, Stores: s, Now: now}
			if via == "open silo" {
				if _, err := s.Place(ctx, "org", "A"); err != nil {
					t.Fatal(err)
				}
			} else {
				jobs.PanelActive(p, shared, s, now, "org/A")
			}
			if ps, err := s.OpenSilos(ctx); err != nil || (len(ps) == 1) != (via == "open silo") {
				t.Fatalf("%s: open silos %+v %v", via, ps, err)
			}
			if err := c.op(p); !c.want(err) {
				t.Errorf("%s, %s: %v", via, c.name, err)
			}
		}
	}
	if r := read(t, siloFile(t, dir, "A"), "deadA001"); r.state != "dead" {
		t.Errorf("the refused retry changed the row: %+v", r)
	}
}

// Counts counts a table's rows by state; RetryDead retries every dead row, as Retry does, and
// reports the ones it refuses; Manual queues a non-tick row of a kind with `{}`, due at now.
func TestAdmin_CountsRetryDeadManual(t *testing.T) {
	st := open(t)
	ctx := context.Background()
	q, err := db.On(st, nil)
	if err != nil {
		t.Fatal(err)
	}
	t0 := time.Date(2026, 11, 16, 12, 0, 0, 0, time.UTC)
	for _, r := range []struct{ id, state, key string }{{"dead0001", "dead", "k1"}, {"live0001", "queued", "k1"}, {"dead0002", "dead", ""}, {"dead0003", "dead", ""}, {"done0001", "done", ""}, {"run00001", "running", ""}} {
		var key any
		if r.key != "" {
			key = r.key
		}
		if _, err := q.ExecContext(ctx, `INSERT INTO "_alder_jobs" ("id", "kind", "params", "key", "state", "run_at", "created", "attempts", "error", "finished") VALUES (?, 'K.J', '{}', ?, ?, ?, ?, 3, 'boom', ?)`,
			r.id, key, r.state, t0.UnixMilli(), t0.UnixMilli(), t0.UnixMilli()); err != nil {
			t.Fatal(err)
		}
	}
	c, err := jobs.Counts(ctx, q)
	if err != nil || c[jobs.Queued] != 1 || c[jobs.Running] != 1 || c[jobs.Dead] != 3 || c[jobs.Done] != 1 {
		t.Fatalf("counts: %v %v", c, err)
	}
	now := t0.Add(time.Hour)
	retried, refused, err := jobs.RetryDead(ctx, q, now)
	if err != nil {
		t.Fatal(err)
	}
	var ids []string
	for _, r := range retried {
		ids = append(ids, r.ID)
	}
	if len(ids) != 2 || ids[0] != "dead0002" || ids[1] != "dead0003" || len(refused) != 1 || refused[0].Error() != `job dead0001 is not retried: job live0001 of K.J holds its key "k1" (queued or running)` {
		t.Fatalf("retried %v, refused %v", ids, refused)
	}
	for _, id := range ids {
		if r, _ := jobs.Get(ctx, q, id); r.State != "queued" || r.Attempts != 0 || r.Error != nil || r.RunAt != now.UnixMilli() {
			t.Errorf("retried %s: %+v", id, r)
		}
	}
	defer jobs.SetNewID(func() string { return "man00001" })()
	id, err := jobs.Manual(ctx, q, jobs.Kind{Name: "K.Nightly"}, now)
	if err != nil || id != "man00001" {
		t.Fatalf("manual: %q %v", id, err)
	}
	r, err := jobs.Get(ctx, q, id)
	if err != nil || r.Kind != "K.Nightly" || r.Params != "{}" || r.State != "queued" || r.Schedule || r.ScheduledFor != nil || r.RunAt != now.UnixMilli() || r.Created != now.UnixMilli() || r.Key != nil {
		t.Fatalf("manual row: %+v %v", r, err)
	}
}

// A silo of the workers' active set that is closed with nothing due is not read by the listing:
// it says so in Missed. A look for one row (Get, a retry) is not failed by it.
func TestPanel_ClosedIdleSilo(t *testing.T) {
	ctx := context.Background()
	now := func() time.Time { return t0.Add(time.Hour) }
	dir, shared, s1 := siloApp(t)
	runEnqueue(t, orgEnv(t, shared, s1, "A"), welcome, nil) // creates A's database
	s1.Close()
	s := reopened(t, dir, shared) // no silo open
	p := &jobs.Panel{Store: shared, Stores: s, Now: now}
	jobs.PanelActive(p, shared, s, now, "org/A")
	l, err := p.List(ctx)
	if err != nil || len(l.Queued) != 1 || len(l.Missed) != 0 {
		t.Fatalf("busy: %+v %v", l, err) // busy and due: opened and read
	}
	s.Close()
	s = reopened(t, dir, shared)
	p = &jobs.Panel{Store: shared, Stores: s, Now: now}
	jobs.PanelActive(p, shared, s, now, "org/A")
	jobs.PanelIdle(p, "org/A")
	l, err = p.List(ctx)
	want := "silo org/A: closed with nothing due, or under maintenance; its jobs are read again when one is due"
	if err != nil || len(l.Queued) != 0 || len(l.Missed) != 1 || l.Missed[0] != want {
		t.Fatalf("idle: %+v %v", l, err)
	}
	if err := p.Retry(ctx, "nosuch01"); !errors.Is(err, jobs.ErrNoJob) {
		t.Errorf("retry of no row: %v", err)
	}
}

// Manual writes the kind's priority; Retry keeps a dead row's.
func TestAdmin_ManualPriority(t *testing.T) {
	st := open(t)
	ctx := context.Background()
	q := raw(t, st)
	defer jobs.SetNewID(func() string { return "man00002" })()
	id, err := jobs.Manual(ctx, q, jobs.Kind{Name: "A.B", Priority: 2}, t0)
	if err != nil {
		t.Fatal(err)
	}
	if r, err := jobs.Get(ctx, q, id); err != nil || r.Kind != "A.B" || r.Priority != 2 {
		t.Fatalf("manual row: %+v %v", r, err)
	}
	if _, err := q.ExecContext(ctx, `INSERT INTO "_alder_jobs" ("id", "kind", "params", "state", "run_at", "created", "attempts", "finished", "priority") VALUES ('dead0009', 'A.B', '{}', 'dead', ?, ?, 3, ?, -4)`,
		t0.UnixMilli(), t0.UnixMilli(), t0.UnixMilli()); err != nil {
		t.Fatal(err)
	}
	if err := jobs.Retry(ctx, q, "dead0009", t0.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	if r, err := jobs.Get(ctx, q, "dead0009"); err != nil || r.State != "queued" || r.Priority != -4 {
		t.Fatalf("retried row: %+v %v", r, err)
	}
}

// List's queued rows are in claim order: priority, highest first, then run_at, then created.
func TestList_QueuedInClaimOrder(t *testing.T) {
	st := open(t)
	insertPri(t, st, "neg", -5, t0, t0)
	insertPri(t, st, "zero", 0, t0.Add(time.Second), t0)
	insertPri(t, st, "ten", 10, t0.Add(time.Minute), t0)
	l, err := jobs.List(context.Background(), raw(t, st))
	if err != nil {
		t.Fatal(err)
	}
	var ids []string
	for _, r := range l.Queued {
		ids = append(ids, r.ID)
	}
	if strings.Join(ids, " ") != "ten zero neg" {
		t.Fatalf("queued %v, want ten zero neg", ids)
	}
}
