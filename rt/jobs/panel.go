package jobs

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"
	"time"

	"alder/rt/db"
	"alder/rt/eff"
)

// AttemptRing is how many attempts' records a Panel keeps: the last attempt of each of the last
// AttemptRing jobs run in its process.
const AttemptRing = 100

// Panel is the dev panel's view of the jobs (web.Options.DevJobs; the generated Open makes one in
// development mode only): the rows of the shared store and of the silos this process sees
// (List, Get), a retry and a run now by hand (Retry, RunNow), and the record of the last attempt
// of each recent job (Last). Start, given it (Options.Panel), records each attempt its workers
// run, with the Recorder of its steps and statements, and lends it the silos of its active set.
// It is safe for concurrent use.
type Panel struct {
	Store  eff.Store        // the shared store
	Stores *db.Stores       // in an app with silos, its stores; nil: the shared store only
	Kinds  []Kind           // the kinds this build knows (generated: app.Jobs())
	Now    func() time.Time // the clock of a retry and a run now (required)

	mu   sync.Mutex
	pool *pool // the running Start's workers; nil while none run in this process
	ring attemptRing
}

// Attempt is the record of one attempt a worker of this process ran: how it ended and the steps
// and statements its Env's Recorder kept.
type Attempt struct {
	Outcome
	Start time.Time
	Steps []eff.StepRec
	SQL   []eff.SQLRec
}

// Workers reports whether job workers run in this process now: without them (ALDER_JOBS=0) a
// queued row waits for another process's.
func (p *Panel) Workers() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.pool != nil
}

// Last is the record of the last attempt of job id that a worker of this process ran, if it is
// still among the last AttemptRing jobs run.
func (p *Panel) Last(id string) (Attempt, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.ring.get(id)
}

// List is the rows of every store the panel sees (List of each, merged in its orders, each row's
// In its store): the shared store; the silos open and served now; and, while workers run here,
// the silos of their active set, read as their claims read them (a closed silo is opened only
// when a row there is due or running). A silo that cannot be read is in Missed; an error is the
// shared store's.
func (p *Panel) List(ctx context.Context) (Listing, error) {
	var out Listing
	err := p.each(ctx, &out.Missed, &out.Missed, func(pl db.Place, q db.Execer) error {
		l, err := List(ctx, q)
		if err == nil {
			out.merge(l, pl)
		}
		return err
	})
	out.sort()
	return out, err
}

// Get is job id's row in the first store List reads that has it, its In that store; nil, nil
// when none does and every store was read. A store that could not be read while no other had
// the row is an error (notRead): the row may be there.
func (p *Panel) Get(ctx context.Context, id string) (*Row, error) {
	var found *Row
	var missed []string
	err := p.each(ctx, &missed, nil, func(pl db.Place, q db.Execer) error {
		r, err := Get(ctx, q, id)
		if err != nil || r == nil {
			return err
		}
		r.In, found = pl, r
		return errEnough
	})
	if err == nil && found == nil && len(missed) > 0 {
		err = notRead(id, missed)
	}
	return found, err
}

// notRead is the error of a look for job id that found it nowhere while the stores missed
// could not be read.
func notRead(id string, missed []string) error {
	return fmt.Errorf("job %s is in none of the stores read, and these could not be read: %s", id, strings.Join(missed, "; "))
}

// Retry is Retry of job id in its store (Get), at the panel's clock; then, for a silo's row,
// the silo joins the active set of this process's workers, and they are woken.
func (p *Panel) Retry(ctx context.Context, id string) error {
	return p.change(ctx, id, Retry)
}

// RunNow is RunNow of job id in its store (Get), at the panel's clock, as Retry is.
func (p *Panel) RunNow(ctx context.Context, id string) error {
	return p.change(ctx, id, RunNow)
}

// change runs op on job id in the first store that has it: op's error (a *StateError, a key
// held, a failed statement) is change's, whichever store the row is in. ErrNoJob when no store
// has it and every store was read; notRead's error when one could not be read.
func (p *Panel) change(ctx context.Context, id string, op func(context.Context, db.Execer, string, time.Time) error) error {
	var (
		at     *db.Place
		opErr  error
		missed []string
	)
	err := p.each(ctx, &missed, nil, func(pl db.Place, q db.Execer) error {
		r, err := Get(ctx, q, id)
		if err != nil || r == nil {
			return err
		}
		at, opErr = &pl, op(ctx, q, id, p.Now())
		return errEnough // the row is found: no other store is looked in, and opErr is kept
	})
	switch {
	case err != nil:
		return err
	case at == nil && len(missed) > 0:
		return notRead(id, missed)
	case at == nil:
		return fmt.Errorf("%w %s", ErrNoJob, id)
	case opErr != nil:
		return opErr
	}
	if at.Key != "" && p.Stores != nil {
		touch(p.Stores, at.Key, at.Value)
	}
	Wake()
	return nil
}

// each runs f on the jobs table of each store the panel sees, in List's order, until f returns
// errEnough. The shared store's error is the result; a silo's is added to missed (when set) and
// the others are still read. A silo of the active set that stays closed (nothing due there, or
// under maintenance) is added to skipped (when set), not to missed: a look for one row (Get, a
// retry) is not failed by it, a listing says it was not read.
func (p *Panel) each(ctx context.Context, missed, skipped *[]string, f func(db.Place, db.Execer) error) error {
	miss := func(name string, err error) {
		if missed != nil {
			*missed = append(*missed, name+": "+err.Error())
		}
	}
	q, err := db.On(p.Store, nil)
	if err == nil {
		err = f(db.Place{Store: p.Store}, q)
	}
	if errors.Is(err, errEnough) {
		return nil
	}
	if err != nil || p.Stores == nil {
		return err
	}
	seen := map[siloID]bool{}
	open, err := p.Stores.OpenSilos(ctx)
	if err != nil {
		miss("the open silos", err)
	}
	for _, pl := range open {
		id := siloID{pl.Key, pl.Value}
		seen[id] = true
		q, err := db.On(pl.Store, pl.Tx)
		if err == nil {
			err = f(pl, q)
		}
		if errors.Is(err, errEnough) {
			return nil
		}
		if err != nil {
			miss(id.String(), err)
		}
	}
	p.mu.Lock()
	pool := p.pool
	p.mu.Unlock()
	if pool == nil || pool.active == nil {
		return nil
	}
	for _, id := range pool.active.list() {
		if seen[id] {
			continue
		}
		ran, err := pool.at(ctx, id, func(pl db.Place) error {
			q, err := db.On(pl.Store, pl.Tx)
			if err != nil {
				return err
			}
			return f(pl, q)
		})
		if errors.Is(err, errEnough) {
			return nil
		}
		if err != nil {
			miss(id.String(), err)
		}
		if err == nil && !ran && skipped != nil && slices.Contains(pool.active.list(), id) {
			*skipped = append(*skipped, id.String()+": closed with nothing due, or under maintenance; its jobs are read again when one is due")
		}
	}
	return nil
}

// started tells the panel that workers p run here; stopped, that they stopped.
func (p *Panel) started(pl *pool) { p.mu.Lock(); p.pool = pl; p.mu.Unlock() }

func (p *Panel) stopped(pl *pool) {
	p.mu.Lock()
	if p.pool == pl {
		p.pool = nil
	}
	p.mu.Unlock()
}

// record files the attempt that started at start, ended out, and whose Env kept rec.
func (p *Panel) record(out Outcome, start time.Time, rec *eff.Recorder) {
	a := Attempt{Outcome: out, Start: start}
	if rec != nil {
		a.Steps = append([]eff.StepRec{}, rec.Steps...)
		a.SQL = append([]eff.SQLRec{}, rec.SQL...)
	}
	p.mu.Lock()
	p.ring.add(a)
	p.mu.Unlock()
}

// attemptRing is the last attempt of each of the last AttemptRing jobs run, the one run longest
// ago overwritten first; a job run again leaves its earlier record's slot empty and is filed as
// the newest. Its Panel's mu guards it.
type attemptRing struct {
	recs  [AttemptRing]Attempt
	next  int            // the slot the next record goes in
	byID  map[string]int // a job id's slot
	inUse [AttemptRing]bool
}

func (g *attemptRing) add(a Attempt) {
	if g.byID == nil {
		g.byID = make(map[string]int, AttemptRing)
	}
	if i, ok := g.byID[a.ID]; ok { // its last attempt only, filed as the newest
		g.recs[i], g.inUse[i] = Attempt{}, false
	}
	if g.inUse[g.next] {
		delete(g.byID, g.recs[g.next].ID)
	}
	g.recs[g.next], g.inUse[g.next] = a, true
	g.byID[a.ID] = g.next
	g.next = (g.next + 1) % AttemptRing
}

func (g *attemptRing) get(id string) (Attempt, bool) {
	i, ok := g.byID[id]
	if !ok {
		return Attempt{}, false
	}
	return g.recs[i], true
}
