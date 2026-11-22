package jobs

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"
	"time"

	"alder/rt/db"
	"alder/rt/eff"
)

// ActiveFor is how long a silo stays in the workers' active set with no row queued or running
// there (spec §7).
const ActiveFor = time.Hour

// ClaimLogEvery is how often, at most, a store whose claims keep failing is a warn line.
const ClaimLogEvery = time.Minute

// siloID names a silo: its key and value.
type siloID struct{ key, value string }

func (s siloID) String() string { return "silo " + s.key + "/" + s.value }

// member is a silo of the active set.
type member struct {
	// seen is the last time a row was known queued or running there: it joined (the start scan,
	// an enqueue's touch), a claim found a row, or housekeeping found one.
	seen time.Time
	// busy: a row may be queued or running there. Housekeeping clears it when it finds none, and
	// a silo not served (maintenance) is not busy.
	busy bool
	// due is when a poll may open the silo again once the idle sweep closed it: zero (at once)
	// when it joined, when a claim found a row, or when housekeeping found a row due or running;
	// the earliest queued row's run_at when housekeeping found only rows queued for later. A silo
	// not busy, or busy before its due, is not opened by a poll (Stores.Poll without open): it
	// closes at its idle time, as a silo no worker looks at does, and stays closed.
	due time.Time
}

// activeSet is the silos whose jobs tables one Start's workers poll beside the shared store's
// (spec §7): every silo with jobs the start scan finds, and every silo an enqueue of this process
// touches since; a silo leaves it after ActiveFor with no row queued or running there. A silo
// with a row queued for later (a retry's backoff, Job.enqueueAt) stays until it has run, but is
// opened for it only once the row is due. It is safe for concurrent use.
type activeSet struct {
	mu  sync.Mutex
	now func() time.Time
	m   map[siloID]*member
	// left, when set, is called with each silo that leaves the set (outside mu)
	left func(siloID)
}

func newActiveSet(now func() time.Time) *activeSet {
	return &activeSet{now: now, m: map[siloID]*member{}}
}

// add puts id in the set, busy, seen now, to be opened at once (a start scan's find, an
// enqueue's touch, a claim).
func (a *activeSet) add(id siloID) {
	now := a.now()
	a.mu.Lock()
	defer a.mu.Unlock()
	if m := a.m[id]; m != nil {
		m.seen, m.busy, m.due = now, true, time.Time{}
		return
	}
	a.m[id] = &member{seen: now, busy: true}
}

// drop takes id out of the set (under mu) and reports whether it was in it.
func (a *activeSet) drop(id siloID) bool {
	_, ok := a.m[id]
	delete(a.m, id)
	return ok
}

// leave tells left that id left the set.
func (a *activeSet) leave(id siloID) {
	if a.left != nil {
		a.left(id)
	}
}

func (a *activeSet) remove(id siloID) {
	a.mu.Lock()
	ok := a.drop(id)
	a.mu.Unlock()
	if ok {
		a.leave(id)
	}
}

// idle marks id not busy: its poll no longer opens it.
func (a *activeSet) idle(id siloID) {
	a.mu.Lock()
	if m := a.m[id]; m != nil {
		m.busy = false
	}
	a.mu.Unlock()
}

// opens reports whether a poll at now may open id when it is closed: it is busy and due.
func (a *activeSet) opens(id siloID, now time.Time) bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	m := a.m[id]
	return m != nil && m.busy && !now.Before(m.due)
}

// kept records housekeeping's look at id at now. next is when its earliest row comes due: a row
// running, or queued and due, gives a time not after now; nil, no row queued or running. A row
// keeps it, seen now, to be opened again once next has come; with none, it is not busy, and it
// leaves the set when none was seen for ActiveFor.
func (a *activeSet) kept(id siloID, next *time.Time, now time.Time) {
	a.mu.Lock()
	m := a.m[id]
	left := false
	switch {
	case m == nil:
	case next != nil:
		m.seen, m.busy, m.due = now, true, *next
	case now.Sub(m.seen) >= ActiveFor:
		left = a.drop(id)
	default:
		m.busy = false
	}
	a.mu.Unlock()
	if left {
		a.leave(id)
	}
}

// unseen is housekeeping's word on id when it did not look in it (closed, and not to be opened):
// a silo not busy leaves the set when no row was seen there for ActiveFor; a busy one waits for
// its due.
func (a *activeSet) unseen(id siloID, now time.Time) {
	a.mu.Lock()
	m := a.m[id]
	left := m != nil && !m.busy && now.Sub(m.seen) >= ActiveFor && a.drop(id)
	a.mu.Unlock()
	if left {
		a.leave(id)
	}
}

// list is the set's silos, by key then value.
func (a *activeSet) list() []siloID {
	a.mu.Lock()
	out := make([]siloID, 0, len(a.m))
	for id := range a.m {
		out = append(out, id)
	}
	a.mu.Unlock()
	slices.SortFunc(out, func(x, y siloID) int {
		if c := strings.Compare(x.key, y.key); c != 0 {
			return c
		}
		return strings.Compare(x.value, y.value)
	})
	return out
}

// actives are the active sets of the running Starts, by their Options.Stores, so that an
// enqueue reaches the set of the workers polling its app's silos (touch).
var actives = struct {
	mu sync.Mutex
	m  map[*db.Stores]*activeSet
}{m: map[*db.Stores]*activeSet{}}

func activeOf(s *db.Stores) *activeSet {
	actives.mu.Lock()
	defer actives.mu.Unlock()
	return actives.m[s]
}

// touch puts silo key/value of the stores s in the active set of the workers started on s, if
// any: Enqueue calls it once a row it inserted there has committed.
func touch(s *db.Stores, key, value string) {
	if a := activeOf(s); a != nil {
		a.add(siloID{key, value})
	}
}

// HintBatch is the most silo hints the workers read at once (Options.Hints).
const HintBatch = 100

// hints reads the silo hints of the shared store when the workers were started with Hints and
// none of them read them in the last Poll by the clock: each, oldest first and at most
// HintBatch, joins the active set and is deleted, so the claim that follows looks in its silo. A
// failure is a line of claimed under the name "the silo hints".
func (p *pool) hints(ctx context.Context, now time.Time) {
	if !p.o.Hints || p.active == nil {
		return
	}
	p.hintMu.Lock()
	if !p.hintAt.IsZero() && now.Sub(p.hintAt) < p.o.Poll {
		p.hintMu.Unlock()
		return
	}
	p.hintAt = now
	p.hintMu.Unlock()
	err := p.readHints(ctx)
	if ctx.Err() == nil {
		p.claimed("the silo hints", err, now)
	}
}

// readHints is one read of the hints (hints).
func (p *pool) readHints(ctx context.Context) error {
	q, err := db.On(p.o.Store, nil)
	if err != nil {
		return err
	}
	rows, err := q.QueryContext(ctx, `SELECT "key", "value" FROM "_alder_jobs_silos" ORDER BY "touched" LIMIT ?`, HintBatch)
	if err != nil {
		return err
	}
	var ids []siloID
	for rows.Next() {
		var id siloID
		if err := rows.Scan(&id.key, &id.value); err != nil {
			rows.Close()
			return err
		}
		ids = append(ids, id)
	}
	if err := errors.Join(rows.Err(), rows.Close()); err != nil {
		return err
	}
	for _, id := range ids {
		p.active.add(id)
		if _, err := q.ExecContext(ctx, `DELETE FROM "_alder_jobs_silos" WHERE "key" = ? AND "value" = ?`, id.key, id.value); err != nil {
			return err
		}
	}
	return nil
}

// at runs f on silo id's Place as Poll gives it — opened again, when closed, only while a row
// is due or running there (activeSet.opens) — and releases it after. ran is false when f did not
// run: the silo is closed, or it is not served (a *MaintenanceError: it is no longer busy), or
// its key row is gone (it leaves the set: nothing reaches it any more).
func (p *pool) at(ctx context.Context, id siloID, f func(db.Place) error) (ran bool, err error) {
	pl, release, err := p.o.Stores.Poll(ctx, id.key, id.value, p.active.opens(id, p.o.Now()))
	defer release()
	var me *db.MaintenanceError
	switch {
	case errors.Is(err, eff.ErrNotFound):
		p.active.remove(id)
		return false, nil
	case errors.As(err, &me):
		p.active.idle(id)
		return false, nil
	case err != nil:
		return false, err
	case pl.Store == nil:
		return false, nil
	}
	return true, f(pl)
}

// errEnough, returned by each's f, ends each: the answer is found.
var errEnough = errors.New("jobs: enough")

// each runs f on the jobs table of the shared store, then on those of the active set's silos
// (at: a closed silo is opened only when a row is due or running there), for the queued gauge
// and the health check, until f returns errEnough. The error is the shared store's: a silo's
// that fails is left out (the claims report it).
func (p *pool) each(ctx context.Context, f func(db.Execer) error) error {
	q, err := db.On(p.o.Store, nil)
	if err == nil {
		err = f(q)
	}
	if errors.Is(err, errEnough) {
		return nil
	}
	if err != nil || p.active == nil {
		return err
	}
	for _, id := range p.active.list() {
		_, err := p.at(ctx, id, func(pl db.Place) error {
			q, err := db.On(pl.Store, pl.Tx)
			if err != nil {
				return err
			}
			return f(q)
		})
		if errors.Is(err, errEnough) {
			break
		}
	}
	return nil
}

// scan is the start scan (spec §7): every silo database whose jobs table has a row queued or
// running, or one housekeeping prunes now (a done row older than KeepDone, a dead one older than
// KeepDead), joins the active set. It reads each database apart from the silos served
// (Stores.ScanSilos): a silo with nothing there is not opened. Once per Start.
func (p *pool) scan(ctx context.Context, now time.Time) {
	if p.active == nil || !p.scanned.CompareAndSwap(false, true) {
		return
	}
	n, found := 0, 0
	err := p.o.Stores.ScanSilos(ctx, func(key, value string, q db.Execer) error {
		n++
		var one int
		err := q.QueryRowContext(ctx, `SELECT 1 FROM sqlite_schema WHERE "type" = 'table' AND "name" = '_alder_jobs'`).Scan(&one)
		if err == nil {
			err = q.QueryRowContext(ctx, `SELECT 1 FROM "_alder_jobs" WHERE "state" IN ('queued', 'running') OR ("state" = 'done' AND COALESCE("finished", "created") < ?) OR ("state" = 'dead' AND COALESCE("finished", "created") < ?) LIMIT 1`,
				now.Add(-KeepDone).UnixMilli(), now.Add(-KeepDead).UnixMilli()).Scan(&one)
		}
		if errors.Is(err, sql.ErrNoRows) { // no such row, or no jobs table: not opened since it came
			return nil
		}
		if err != nil {
			return err
		}
		found++
		p.active.add(siloID{key, value})
		return nil
	})
	if err != nil && ctx.Err() == nil {
		p.o.Log("warn", "jobs: the start scan: "+err.Error())
	}
	p.o.Log("info", fmt.Sprintf("jobs: the start scan looked in %d silo(s): %d with jobs", n, found))
}

// keepSilos is the housekeeping of the active set's silos: in each (at), keep (its expired leases
// and its pruning), then a look for its earliest row queued (of a kind this build knows) or
// running, which keeps it in the set and says when a poll may open it again (kept). A silo closed
// and not to be opened is not looked in (unseen). An error is a warn line and leaves the silo as
// it was.
func (p *pool) keepSilos(ctx context.Context, now time.Time, keep func(db.Execer) error) {
	for _, id := range p.active.list() {
		var next *time.Time
		ran, err := p.at(ctx, id, func(pl db.Place) error {
			q, err := db.On(pl.Store, pl.Tx)
			if err == nil {
				err = keep(q)
			}
			if err != nil {
				return err
			}
			var ms sql.NullInt64
			if err := q.QueryRowContext(ctx, p.pendingStmt, p.pendingArgs...).Scan(&ms); err != nil {
				return err
			}
			if ms.Valid {
				t := time.UnixMilli(ms.Int64)
				next = &t
			}
			return nil
		})
		switch {
		case err != nil:
			if ctx.Err() == nil {
				p.o.Log("warn", fmt.Sprintf("jobs: housekeeping %s: %v", id, err))
			}
		case ran:
			p.active.kept(id, next, now)
		default:
			p.active.unseen(id, now)
		}
	}
}

// claimLog rate-limits the warn lines of the claims that fail: per store, the first failure, then
// at most one every ClaimLogEvery while they keep failing, and one line when a claim there works
// again.
type claimLog struct {
	mu   sync.Mutex
	last map[string]time.Time // a store failing → when its last warn line was written
}

// claimed records how a claim in store (its name in a line) went at now.
func (p *pool) claimed(store string, err error, now time.Time) {
	l := &p.claimLog
	var level, line string
	l.mu.Lock()
	last, failing := l.last[store]
	switch {
	case err != nil && (!failing || now.Sub(last) >= ClaimLogEvery):
		l.last[store] = now
		level, line = "warn", "jobs: claiming a job in "+store+": "+err.Error()
	case err == nil && failing:
		delete(l.last, store)
		level, line = "info", "jobs: claiming a job in "+store+" works again"
	}
	l.mu.Unlock()
	if line != "" {
		p.o.Log(level, line)
	}
}

// forget drops what claimLog knows of store (a silo that left the active set).
func (l *claimLog) forget(store string) {
	l.mu.Lock()
	delete(l.last, store)
	l.mu.Unlock()
}
