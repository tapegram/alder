package jobs

import (
	"context"
	"strings"
	"sync"
	"time"

	"alder/rt/applog"
	"alder/rt/db"
	"alder/rt/eff"
)

// StallAfter is how long a due row may wait, with no claim by this process's workers in that
// time, before Health says "stalled" (spec §9).
const StallAfter = 5 * time.Minute

// readTimeout bounds the statements Health and the queued gauge run for a check or a scrape.
const readTimeout = 5 * time.Second

// outcome is o's outcome label: "done", "failed", "dead" or "stopped".
func (o Outcome) outcome() string {
	switch {
	case o.Stopped:
		return "stopped"
	case o.Err == nil:
		return "done"
	case o.Dead:
		return "dead"
	}
	return "failed"
}

// Line is o's log line (spec §9): its kind, id, outcome, time and attempt; a failure's error
// (cut to MaxError, as the row keeps it) and the wait before its retry (Backoff); and how long
// it held the write transaction, which marks the line above applog.LockMark (§6a).
func (o Outcome) Line() *applog.Job {
	j := &applog.Job{ID: o.ID, Kind: o.Kind, Outcome: o.outcome(), Dur: o.Took, Attempt: o.Attempt, Attempts: o.Attempts, LockHeld: o.LockHeld}
	if o.Err != nil && !o.Stopped {
		j.Err = clip(o.Err.Error())
		if !o.Dead {
			j.Next = Backoff(o.Attempt)
		}
	}
	return j
}

// observe logs out's line and counts it in the metrics.
func (p *pool) observe(out Outcome) {
	j := out.Line()
	switch {
	case p.o.Line != nil:
		p.o.Line(j)
	case p.o.LogID != nil:
		p.o.LogID(out.ID, j.Level().String(), j.Text())
	default:
		p.o.Log(j.Level().String(), j.Text())
	}
	if m := p.o.Metrics; m != nil && !out.Stopped {
		m.Job(out.Kind, j.Outcome, out.Took)
	}
}

// queuedDue counts the rows of store queued and due at now(), of every kind: the
// alder_jobs_queued gauge.
func queuedDue(store eff.Store, now func() time.Time) func() (int64, error) {
	return func() (int64, error) {
		q, err := db.On(store, nil)
		if err != nil {
			return 0, err
		}
		ctx, cancel := context.WithTimeout(context.Background(), readTimeout)
		defer cancel()
		var n int64
		err = q.QueryRowContext(ctx, `SELECT COUNT(*) FROM "_alder_jobs" WHERE "state" = 'queued' AND "run_at" <= ?`, now().UnixMilli()).Scan(&n)
		return n, err
	}
}

// Health is the state of a process's job workers for /_alder/health (web.Options.Jobs: State).
// The zero Health is "off"; Start (Options.Health) turns it on and its workers record each
// claim. It is safe for concurrent use.
type Health struct {
	mu    sync.Mutex
	on    bool
	last  time.Time // the last claim, or the workers' start
	store eff.Store
	kinds []any // the kinds' names, the statement's arguments
	now   func() time.Time
}

func (h *Health) start(o Options) {
	names := make([]any, len(o.Kinds))
	for i, k := range o.Kinds {
		names[i] = k.Name
	}
	h.mu.Lock()
	h.on, h.last, h.store, h.kinds, h.now = true, o.Now(), o.Store, names, o.Now
	h.mu.Unlock()
}

func (h *Health) stop() {
	h.mu.Lock()
	h.on = false
	h.mu.Unlock()
}

func (h *Health) claimed(t time.Time) {
	h.mu.Lock()
	if t.After(h.last) {
		h.last = t
	}
	h.mu.Unlock()
}

// State is "off" when this process runs no workers (ALDER_JOBS=0, or they stopped); "stalled"
// when a row of a kind they run has been due more than StallAfter and they have claimed nothing
// in that time; else "ok". It reads the table only when the last claim is older than
// StallAfter; a read that fails (the database's state is health's "db") gives "ok".
func (h *Health) State(ctx context.Context) string {
	h.mu.Lock()
	on, last, store, kinds, now := h.on, h.last, h.store, h.kinds, h.now
	h.mu.Unlock()
	if !on {
		return "off"
	}
	t := now()
	if t.Sub(last) <= StallAfter || len(kinds) == 0 {
		return "ok"
	}
	q, err := db.On(store, nil)
	if err != nil {
		return "ok"
	}
	ctx, cancel := context.WithTimeout(ctx, readTimeout)
	defer cancel()
	stmt := `SELECT 1 FROM "_alder_jobs" WHERE "state" = 'queued' AND "run_at" < ? AND "kind" IN (` +
		strings.TrimSuffix(strings.Repeat("?, ", len(kinds)), ", ") + `) LIMIT 1`
	var one int
	err = q.QueryRowContext(ctx, stmt, append([]any{t.Add(-StallAfter).UnixMilli()}, kinds...)...).Scan(&one)
	if err == nil {
		return "stalled"
	}
	return "ok" // no such row (sql.ErrNoRows), or the read failed
}
