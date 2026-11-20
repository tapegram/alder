package jobs_test

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"alder/rt"
	"alder/rt/db"
	"alder/rt/eff"
	"alder/rt/jobs"
)

// utc is the time the layout "2006-01-02 15:04:05" reads from s, in UTC.
func utc(t *testing.T, s string) time.Time {
	t.Helper()
	v, err := time.Parse("2006-01-02 15:04:05", s)
	if err != nil {
		t.Fatal(err)
	}
	return v
}

// scheduled is a kind named name scheduled every `every` or by cron expr, whose run is task.
func scheduled(name string, every time.Duration, expr string, task eff.Task[struct{}]) jobs.Kind {
	k := kind(task)
	k.Name, k.Every, k.Cron = name, every, expr
	return k
}

var nightly = scheduled("Jobs.Nightly", 0, "0 3 * * *", eff.Pure(struct{}{}))

func TestNext_Every(t *testing.T) {
	cases := []struct {
		every       time.Duration
		after, want string
	}{
		{time.Hour, "2026-11-16 10:17:00", "2026-11-16 11:00:00"},
		{7 * time.Minute, "2026-11-16 10:17:00", "2026-11-16 10:24:00"}, // does not divide a day
		{24 * time.Hour, "2026-11-16 10:17:00", "2026-11-17 00:00:00"},
		{90 * time.Second, "2026-11-16 10:17:10", "2026-11-16 10:18:00"}, // divides a day
		{time.Hour, "2026-11-16 11:00:00", "2026-11-16 12:00:00"},        // strictly after
		{48 * time.Hour, "2026-11-16 10:17:00", "2026-11-18 10:17:00"},   // longer than a day
	}
	for _, tc := range cases {
		got, err := jobs.Next(scheduled("A.B", tc.every, "", eff.Pure(struct{}{})), utc(t, tc.after))
		if err != nil || !got.Equal(utc(t, tc.want)) || got.Location() != time.UTC {
			t.Errorf("every %v after %s: %v %v, want %s", tc.every, tc.after, got, err, tc.want)
		}
	}
}

func TestNext_Cron(t *testing.T) {
	got, err := jobs.Next(nightly, utc(t, "2026-11-16 03:00:00"))
	if err != nil || !got.Equal(utc(t, "2026-11-17 03:00:00")) {
		t.Fatalf("nightly after 03:00: %v %v", got, err)
	}
	// an expression the checker refuses, in a build that bypassed it: an error, not a panic
	for _, k := range []jobs.Kind{
		scheduled("A.Bad", 0, "0 3 * *", eff.Pure(struct{}{})),
		scheduled("A.None", 0, "", eff.Pure(struct{}{})),
	} {
		got, err := jobs.Next(k, t0)
		var se *jobs.ScheduleError
		if !errors.As(err, &se) || se.Kind != k.Name || !got.IsZero() {
			t.Errorf("%s: %v %v, want a ScheduleError", k.Name, got, err)
		}
	}
}

// tickRows is the rows of kind that are or were ticks: id, state, run_at, scheduled_for and
// schedule, oldest first.
type tickRow struct {
	id, state      string
	runAt, schedAt int64
	schedule       bool
}

func tickRows(t *testing.T, st *db.SQLite, kind string) []tickRow {
	t.Helper()
	rows, err := raw(t, st).QueryContext(context.Background(), `SELECT "id", "state", "run_at", "scheduled_for", "schedule" FROM "_alder_jobs" WHERE "kind" = ? AND "scheduled_for" IS NOT NULL ORDER BY "scheduled_for", "created"`, kind)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var out []tickRow
	for rows.Next() {
		var r tickRow
		if err := rows.Scan(&r.id, &r.state, &r.runAt, &r.schedAt, &r.schedule); err != nil {
			t.Fatal(err)
		}
		out = append(out, r)
	}
	return out
}

// queuedTicks is the number of rows of kind that are its queued tick.
func queuedTicks(t *testing.T, st *db.SQLite, kind string) int {
	return count(t, st, `SELECT COUNT(*) FROM "_alder_jobs" WHERE "kind" = ? AND "schedule" = 1 AND "state" = 'queued'`, kind)
}

// ensure runs EnsureTicks on st at now and fails the test on an error.
func ensure(t *testing.T, st *db.SQLite, kinds []jobs.Kind, now time.Time) {
	t.Helper()
	if err := jobs.EnsureTicks(context.Background(), st, kinds, now); err != nil {
		t.Fatal(err)
	}
}

func TestEnsureTicks_OnePerKind(t *testing.T) {
	st := openFile(t)
	hourly := scheduled("A.Hourly", time.Hour, "", eff.Pure(struct{}{}))
	plain := kind(eff.Pure(struct{}{}))
	kinds := []jobs.Kind{nightly, hourly, plain}
	ensure(t, st, kinds, t0)
	ensure(t, st, kinds, t0.Add(time.Minute))
	var wg sync.WaitGroup
	errs := make(chan error, 2)
	for range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			errs <- jobs.EnsureTicks(context.Background(), st, kinds, t0.Add(2*time.Minute))
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	for name, at := range map[string]string{nightly.Name: "2026-11-17 03:00:00", hourly.Name: "2026-11-16 13:00:00"} {
		rows := tickRows(t, st, name)
		if len(rows) != 1 || rows[0].state != "queued" || !rows[0].schedule || rows[0].runAt != utc(t, at).UnixMilli() || rows[0].schedAt != rows[0].runAt {
			t.Errorf("%s: %+v, want one queued tick at %s", name, rows, at)
		}
	}
	if n := count(t, st, `SELECT COUNT(*) FROM "_alder_jobs"`); n != 2 {
		t.Fatalf("%d rows, want a tick of each scheduled kind", n)
	}
	var params string
	if err := raw(t, st).QueryRowContext(context.Background(), `SELECT "params" FROM "_alder_jobs" WHERE "kind" = ?`, nightly.Name).Scan(&params); err != nil || params != "{}" {
		t.Fatalf("params %q %v", params, err)
	}
}

// claimAt claims the oldest due row of kinds at now and fails the test unless one is claimed.
func claimAt(t *testing.T, st *db.SQLite, kinds []jobs.Kind, now time.Time) jobs.Row {
	t.Helper()
	r, err := jobs.Claim(context.Background(), st, kinds, now, "w")
	if err != nil || r == nil {
		t.Fatalf("claim at %v: %+v %v", now, r, err)
	}
	return *r
}

// Claiming a tick inserts the next, at Next(max(scheduled_for, now)), in the claim's
// transaction; the claimed row stops being its kind's queued tick and keeps its scheduled_for.
func TestClaimTick_InsertsNext(t *testing.T) {
	st := openFile(t)
	kinds := []jobs.Kind{nightly}
	ensure(t, st, kinds, utc(t, "2026-11-16 02:00:00"))
	// claimed late, but before the following tick: the next is on the grid after scheduled_for
	r := claimAt(t, st, kinds, utc(t, "2026-11-16 03:00:30"))
	if !r.Schedule || r.ScheduledFor == nil || *r.ScheduledFor != utc(t, "2026-11-16 03:00:00").UnixMilli() {
		t.Fatalf("claimed %+v", r)
	}
	rows := tickRows(t, st, nightly.Name)
	if len(rows) != 2 || rows[0].id != r.ID || rows[0].state != "running" || rows[0].schedule ||
		rows[1].state != "queued" || !rows[1].schedule || rows[1].schedAt != utc(t, "2026-11-17 03:00:00").UnixMilli() || rows[1].runAt != rows[1].schedAt {
		t.Fatalf("rows %+v", rows)
	}
	// claimed on time: the next is one interval on
	hourly := scheduled("A.Hourly", time.Hour, "", eff.Pure(struct{}{}))
	ensure(t, st, []jobs.Kind{hourly}, utc(t, "2026-11-16 10:17:00"))
	claimAt(t, st, []jobs.Kind{hourly}, utc(t, "2026-11-16 11:00:00"))
	if rows := tickRows(t, st, hourly.Name); len(rows) != 2 || rows[1].schedAt != utc(t, "2026-11-16 12:00:00").UnixMilli() {
		t.Fatalf("hourly rows %+v", rows)
	}
	// a non-tick row of a scheduled kind inserts nothing
	env, _ := newEnv(st)
	runEnqueue(t, env, nightlyJob, nil)
	claimAt(t, st, kinds, utc(t, "2026-11-16 12:00:00"))
	if n := count(t, st, `SELECT COUNT(*) FROM "_alder_jobs" WHERE "kind" = ?`, nightly.Name); n != 3 {
		t.Fatalf("%d nightly rows, want 3", n)
	}
}

var nightlyJob = rt.Job{Kind: nightly.Name, Params: "{}"}

// A nightly tick left queued across a three-day outage runs once, late, and the next is on the
// grid.
func TestTicks_Outage(t *testing.T) {
	st := openFile(t)
	kinds := []jobs.Kind{nightly}
	ensure(t, st, kinds, utc(t, "2026-11-16 02:00:00"))
	back := utc(t, "2026-11-19 10:00:00")
	env, _ := newEnv(st)
	if out := jobs.RunOne(env, nightly, claimAt(t, st, kinds, back)); out.Err != nil {
		t.Fatal(out.Err)
	}
	if r, err := jobs.Claim(context.Background(), st, kinds, back, "w"); r != nil || err != nil {
		t.Fatalf("a second claim: %+v %v", r, err)
	}
	rows := tickRows(t, st, nightly.Name)
	if len(rows) != 2 || rows[0].state != "done" || rows[1].state != "queued" || rows[1].schedAt != utc(t, "2026-11-20 03:00:00").UnixMilli() {
		t.Fatalf("rows %+v", rows)
	}
}

// A failed tick is queued for its retry beside the next tick (no unique conflict), with its
// scheduled_for kept; claiming the retry inserts no tick.
func TestTick_RetryKeepsScheduledFor(t *testing.T) {
	st := openFile(t)
	flaky := scheduled(nightly.Name, 0, nightly.Cron, eff.Fail[struct{}]("smtp down"))
	kinds := []jobs.Kind{flaky}
	ensure(t, st, kinds, utc(t, "2026-11-16 02:00:00"))
	at := utc(t, "2026-11-16 03:00:00")
	env, c := newEnv(st)
	c.Set(at)
	out := jobs.RunOne(env, flaky, claimAt(t, st, kinds, at))
	if out.Err == nil || out.Dead || strings.Contains(out.Err.Error(), "recording") {
		t.Fatalf("outcome %+v", out)
	}
	rows := tickRows(t, st, nightly.Name)
	if len(rows) != 2 || rows[0].state != "queued" || rows[0].schedAt != at.UnixMilli() || rows[0].runAt != at.Add(10*time.Second).UnixMilli() ||
		rows[1].state != "queued" || !rows[1].schedule || rows[1].schedAt != at.Add(24*time.Hour).UnixMilli() {
		t.Fatalf("rows %+v", rows)
	}
	r := claimAt(t, st, kinds, at.Add(10*time.Second))
	if r.ID != rows[0].id {
		t.Fatalf("claimed %s, want the retry %s", r.ID, rows[0].id)
	}
	if got := tickRows(t, st, nightly.Name); len(got) != 2 || got[1] != rows[1] {
		t.Fatalf("after the retry's claim %+v, want the next tick as it was %+v", got, rows[1])
	}
}

// A tick that dies leaves the next tick alone, and so does a retry by hand of the dead one
// (queued again, attempts 0).
func TestTick_DeadRetryLeavesNextAlone(t *testing.T) {
	st := openFile(t)
	once := scheduled(nightly.Name, 0, nightly.Cron, eff.Fail[struct{}]("smtp down"))
	once.Retries = 0
	kinds := []jobs.Kind{once}
	ensure(t, st, kinds, utc(t, "2026-11-16 02:00:00"))
	at := utc(t, "2026-11-16 03:00:00")
	env, c := newEnv(st)
	c.Set(at)
	if out := jobs.RunOne(env, once, claimAt(t, st, kinds, at)); !out.Dead {
		t.Fatalf("outcome %+v", out)
	}
	rows := tickRows(t, st, nightly.Name)
	if len(rows) != 2 || rows[0].state != "dead" || rows[1].state != "queued" || !rows[1].schedule {
		t.Fatalf("rows %+v", rows)
	}
	if _, err := raw(t, st).ExecContext(context.Background(), `UPDATE "_alder_jobs" SET "state" = 'queued', "attempts" = 0, "run_at" = ? WHERE "id" = ?`, at.Add(time.Hour).UnixMilli(), rows[0].id); err != nil {
		t.Fatalf("retry by hand: %v", err)
	}
	claimAt(t, st, kinds, at.Add(time.Hour))
	if got := tickRows(t, st, nightly.Name); len(got) != 2 || got[1] != rows[1] {
		t.Fatalf("after the retry %+v, want the next tick as it was %+v", got, rows[1])
	}
}

// A kind whose cron the checker would refuse (a build that bypassed it): the claim stands, the
// tick runs, and the error says why no next tick was inserted.
func TestClaimTick_BadCron(t *testing.T) {
	st := openFile(t)
	ensure(t, st, []jobs.Kind{nightly}, utc(t, "2026-11-16 02:00:00"))
	bad := scheduled(nightly.Name, 0, "0 3 * *", eff.Pure(struct{}{}))
	r, err := jobs.Claim(context.Background(), st, []jobs.Kind{bad}, utc(t, "2026-11-16 03:00:00"), "w")
	var se *jobs.ScheduleError
	if r == nil || r.State != "running" || !errors.As(err, &se) {
		t.Fatalf("claim %+v %v", r, err)
	}
	if rows := tickRows(t, st, nightly.Name); len(rows) != 1 || rows[0].state != "running" {
		t.Fatalf("rows %+v", rows)
	}
	if err := jobs.EnsureTicks(context.Background(), st, []jobs.Kind{bad}, t0); !errors.As(err, &se) {
		t.Fatalf("EnsureTicks: %v", err)
	}
	// a kind no longer scheduled runs its last tick and inserts none
	ensure(t, st, []jobs.Kind{scheduled("A.Was", time.Hour, "", eff.Pure(struct{}{}))}, t0)
	r, err = jobs.Claim(context.Background(), st, []jobs.Kind{kind(eff.Pure(struct{}{}))}, t0.Add(time.Hour), "w")
	if r != nil || err != nil {
		t.Fatalf("claim of another kind: %+v %v", r, err)
	}
	was := kind(eff.Pure(struct{}{}))
	was.Name = "A.Was"
	if r, err = jobs.Claim(context.Background(), st, []jobs.Kind{was}, t0.Add(time.Hour), "w"); r == nil || err != nil {
		t.Fatalf("claim of the unscheduled kind's tick: %+v %v", r, err)
	}
	if n := count(t, st, `SELECT COUNT(*) FROM "_alder_jobs" WHERE "kind" = 'A.Was'`); n != 1 {
		t.Fatalf("%d rows of A.Was, want 1", n)
	}
}

// Housekeeping deletes a queued tick of a kind this build does not know once it is a day old,
// a tick queued for its retry too (schedule 0 since its claim), and nothing else of that kind;
// it creates the ticks of the kinds it knows.
func TestHousekeep_UnknownTick(t *testing.T) {
	st := openFile(t)
	old, recent := t0.Add(-25*time.Hour), t0.Add(-23*time.Hour)
	for _, r := range []struct {
		id, kind      string
		created       time.Time
		tick, retried bool // a tick: scheduled_for set; retried: schedule 0, claimed once
	}{
		{"t0000001", "Gone.Tick", old, true, false},
		{"t0000002", "Fresh.Tick", recent, true, false},
		{"t0000003", "Gone.Plain", old, false, false},
		{"t0000004", nightly.Name, old, true, false},
		{"t0000005", "Gone.Retry", old, true, true},
	} {
		var sched any
		if r.tick {
			sched = t0.Add(time.Hour).UnixMilli()
		}
		if _, err := raw(t, st).ExecContext(context.Background(), `INSERT INTO "_alder_jobs" ("id", "kind", "params", "state", "run_at", "created", "schedule", "scheduled_for") VALUES (?, ?, '{}', 'queued', ?, ?, ?, ?)`,
			r.id, r.kind, t0.Add(time.Hour).UnixMilli(), r.created.UnixMilli(), r.tick && !r.retried, sched); err != nil {
			t.Fatal(err)
		}
	}
	lg := &lines{}
	hourly := scheduled("A.Hourly", time.Hour, "", eff.Pure(struct{}{}))
	jobs.Housekeeper(st, []jobs.Kind{nightly, hourly}, lg.log)(context.Background(), t0)
	for id, want := range map[string]bool{"t0000001": false, "t0000002": true, "t0000003": true, "t0000004": true, "t0000005": false} {
		if exists(t, st, id) != want {
			t.Errorf("row %s kept: %v, want %v", id, !want, want)
		}
	}
	if queuedTicks(t, st, nightly.Name) != 1 || queuedTicks(t, st, hourly.Name) != 1 {
		t.Fatalf("ticks: nightly %d, hourly %d", queuedTicks(t, st, nightly.Name), queuedTicks(t, st, hourly.Name))
	}
	if len(lg.with("warn", "Gone.Tick")) != 1 || len(lg.with("warn", "Gone.Retry")) != 1 || len(lg.all) != 2 {
		t.Fatalf("log %q", lg.all)
	}
}

// The workers create the ticks when they start (housekeeping at start).
func TestWorker_TicksAtStart(t *testing.T) {
	st := openFile(t)
	_, c := newEnv(st)
	lg := &lines{}
	start(t, st, c, lg, 1, time.Hour, nightly)
	eventually(t, "the nightly tick", func() bool { return queuedTicks(t, st, nightly.Name) == 1 })
	var runAt int64
	if err := raw(t, st).QueryRowContext(context.Background(), `SELECT "run_at" FROM "_alder_jobs" WHERE "kind" = ?`, nightly.Name).Scan(&runAt); err != nil && !errors.Is(err, sql.ErrNoRows) {
		t.Fatal(err)
	}
	if runAt != utc(t, "2026-11-17 03:00:00").UnixMilli() {
		t.Fatalf("run_at %v", time.UnixMilli(runAt).UTC())
	}
}

// A changed schedule takes effect at the next EnsureTicks: the queued tick moves to the new
// schedule's next time when that comes sooner, and a kind no longer scheduled loses its tick.
func TestEnsureTicks_ScheduleChanged(t *testing.T) {
	st := openFile(t)
	at := utc(t, "2026-11-16 10:17:00")
	daily := scheduled("A.Job", 24*time.Hour, "", eff.Pure(struct{}{}))
	ensure(t, st, []jobs.Kind{daily}, at)
	hourly := scheduled("A.Job", time.Hour, "", eff.Pure(struct{}{}))
	ensure(t, st, []jobs.Kind{hourly}, at.Add(time.Minute))
	if rows := tickRows(t, st, "A.Job"); len(rows) != 1 || rows[0].state != "queued" ||
		rows[0].schedAt != utc(t, "2026-11-16 11:00:00").UnixMilli() || rows[0].runAt != rows[0].schedAt {
		t.Fatalf("every 1d → 1h: %+v", rows)
	}
	// back to daily: the sooner tick stays, and runs once on the old schedule's time
	ensure(t, st, []jobs.Kind{daily}, at.Add(2*time.Minute))
	if rows := tickRows(t, st, "A.Job"); len(rows) != 1 || rows[0].schedAt != utc(t, "2026-11-16 11:00:00").UnixMilli() {
		t.Fatalf("every 1h → 1d: %+v", rows)
	}

	monthly := scheduled(nightly.Name, 0, "0 3 1 * *", eff.Pure(struct{}{}))
	ensure(t, st, []jobs.Kind{monthly}, at)
	ensure(t, st, []jobs.Kind{nightly}, at)
	if rows := tickRows(t, st, nightly.Name); len(rows) != 1 || rows[0].schedAt != utc(t, "2026-11-17 03:00:00").UnixMilli() || rows[0].runAt != rows[0].schedAt {
		t.Fatalf("monthly → nightly: %+v", rows)
	}

	// the schedule removed: the queued tick is deleted; a tick queued for its retry stays
	if _, err := raw(t, st).ExecContext(context.Background(), `INSERT INTO "_alder_jobs" ("id", "kind", "params", "state", "run_at", "created", "schedule", "scheduled_for") VALUES ('r0000001', ?, '{}', 'queued', ?, ?, 0, ?)`,
		nightly.Name, at.UnixMilli(), at.UnixMilli(), at.UnixMilli()); err != nil {
		t.Fatal(err)
	}
	plain := kind(eff.Pure(struct{}{}))
	plain.Name = nightly.Name
	ensure(t, st, []jobs.Kind{plain}, at)
	if rows := tickRows(t, st, nightly.Name); len(rows) != 1 || rows[0].id != "r0000001" {
		t.Fatalf("schedule removed: %+v", rows)
	}
}

// A housekeeping whose now was read just before a claim inserted the next tick (or an instance
// whose clock is behind) does not pull that next tick back onto the slot that just ran: only a
// tick more than one period ahead of Next(k, now) is moved.
func TestEnsureTicks_StaleNowLeavesNextAlone(t *testing.T) {
	st := openFile(t)
	hourly := scheduled("A.Hourly", time.Hour, "", eff.Pure(struct{}{}))
	kinds := []jobs.Kind{hourly}
	ensure(t, st, kinds, utc(t, "2026-11-16 10:17:00"))
	claimAt(t, st, kinds, utc(t, "2026-11-16 11:00:00"))
	ensure(t, st, kinds, utc(t, "2026-11-16 10:59:59"))
	rows := tickRows(t, st, hourly.Name)
	if len(rows) != 2 || rows[1].state != "queued" || rows[1].schedAt != utc(t, "2026-11-16 12:00:00").UnixMilli() || rows[1].runAt != rows[1].schedAt {
		t.Fatalf("rows %+v, want the next tick still at 12:00", rows)
	}
	// the same with cron
	ensure(t, st, []jobs.Kind{nightly}, utc(t, "2026-11-16 02:00:00"))
	claimAt(t, st, []jobs.Kind{nightly}, utc(t, "2026-11-16 03:00:00"))
	ensure(t, st, []jobs.Kind{nightly}, utc(t, "2026-11-16 02:59:59"))
	if rows := tickRows(t, st, nightly.Name); len(rows) != 2 || rows[1].schedAt != utc(t, "2026-11-17 03:00:00").UnixMilli() {
		t.Fatalf("nightly rows %+v, want the next tick still on 11-17", rows)
	}
}
