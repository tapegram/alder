package jobs

import (
	"context"
	"errors"
	"fmt"
	"os"
	"runtime/debug"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"alder/rt/applog"
	"alder/rt/db"
	"alder/rt/eff"
	"alder/rt/metrics"
)

const (
	// DefaultPoll is how often an idle worker looks for a due row (Options.Poll's default).
	DefaultPoll = time.Second
	// DefaultHousekeep is how often a process's workers keep the table (Options.Housekeep's
	// default): expired leases, pruning, unclaimable kinds, schedule ticks.
	DefaultHousekeep = time.Minute
	// LeaseMargin is how much longer than its kind's timeout a claimed row's lease runs.
	LeaseMargin = 30 * time.Second
	// maxTimeout is the longest timeout a job may declare (spec §1): Lease takes it for a kind
	// without one.
	maxTimeout = time.Hour
	// stopWait is how long stop waits, after it cancels the attempts still running, for them to
	// be re-queued; an attempt that ignores its cancellation is left to its lease.
	stopWait = time.Second
)

// Options configure Start.
type Options struct {
	Store eff.Store // the store whose jobs table the workers claim from (the shared one)
	// Stores, in an app with silos, are its stores: the workers claim from, and keep, the jobs
	// tables of the silos of their active set too (spec §7). nil: the shared store only.
	Stores *db.Stores
	Kinds  []Kind // the kinds this build knows (generated: app.Jobs()); rows of others are left
	// Env is the Env an attempt of a job of kind k runs with (generated: App.JobEnv); ctx is
	// cancelled when stop gives up waiting for the attempt.
	Env     func(ctx context.Context, k Kind) *eff.Env
	Workers int                     // goroutines claiming and running rows; 0 or less: none
	Now     func() time.Time        // the clock (nil: the wall clock)
	Log     func(level, msg string) // the workers' own lines; nil: no log
	// LogID, when set, is the hook for a line about one job, called with its id
	// (applog.Logger.LogID): an Env without a Log logs here under the attempt's job id (the
	// job's Log.*, its effect steps), and so does each attempt's line unless Line is set. nil:
	// Log serves both.
	LogID func(id, level, msg string)
	// Line, when set, is called with each attempt's line (Outcome.Line; applog.Logger.Job)
	// instead of writing it through LogID or Log.
	Line func(*applog.Job)
	// Metrics, when set (ALDER_METRICS), counts each attempt by kind and outcome and its time,
	// the attempts running, and reads the jobs queued and due at a scrape.
	Metrics *metrics.Collector
	// Health, when set, is told that the workers started and stopped and when they claim a row
	// (/_alder/health's "jobs": Health.State).
	Health *Health
	// OnAttempt, when set, is called with the Outcome of every attempt a worker ran.
	OnAttempt func(Outcome)
	// Poll is how long an idle worker waits for a wake before it looks again (0: DefaultPoll);
	// Housekeep the interval of the housekeeping (0: DefaultHousekeep).
	Poll, Housekeep time.Duration
	// Panel, when set (the dev server's), records each attempt with its Env's Recorder (an Env
	// without one is given one) and sees the silos of the active set while the workers run.
	Panel *Panel
	// Hints, with Stores, makes the workers read the silo hints of the shared store (written by
	// the Enqueues of a process with no workers, db.SiloHintsTable) before a claim: at the first
	// claim, then once per tick of a Poll ticker for them all (not by Now, so a frozen test clock
	// does not stop the reads; a tick also wakes an idle worker, which reads before its claim, so
	// a hint waits at most one Poll for an idle worker): each hint read, at most HintBatch of them oldest first, puts
	// its silo in the active set and is deleted. The generated --jobs-only main sets it.
	Hints bool
}

// wake carries Wake's signal to an idle worker: one pending signal is enough, since the worker
// it wakes passes it on when it finds a row.
var wake = make(chan struct{}, 1)

// Wake tells an idle worker of this process to look for a due row now instead of at its next
// poll. Enqueue calls it once the enqueueing Task's transaction has committed (eff's
// AfterCommit), so a job enqueued by a request starts within milliseconds.
func Wake() {
	select {
	case wake <- struct{}{}:
	default:
	}
}

// errStopped is the cause of the context of an attempt that stop cancelled: RunOne re-queues
// the row without counting the attempt.
var errStopped = errors.New("jobs: the workers stopped")

// Start starts o.Workers workers. Each claims the oldest due row of a kind it knows (Claim) in
// one of the stores it polls — the shared store and, with o.Stores, the silos of the active set,
// taken in turn from a start that moves on with every claim — runs it (RunOne on o.Env's Env,
// the row in the store it was found in), and claims again; with none due it waits o.Poll or
// until Wake. One worker at a time keeps the tables (housekeeping): at start and every
// o.Housekeep. The shared store's keeping also inserts each scheduled kind's tick when it has
// none queued (EnsureTicks): ticks are only ever in the shared store. A silo's keeping is its
// expired leases and its pruning.
//
// The active set (spec §7) is every silo the start scan finds with jobs (scan: the first
// housekeeping reads every silo database once, opening none), every silo an Enqueue of this
// process touches and, with o.Hints, every silo a hint in the shared store names (hints: read
// before a claim, once at the first and then at most once per o.Poll tick, which also wakes an idle worker); a silo leaves it after ActiveFor with no row queued
// or running there. A worker's look at a silo is not a use of it (db.Stores.Poll): a silo with
// nothing due or running closes at its idle time, as one no worker looks at, and is opened again
// only when a row there comes due (housekeeping records when). The queued gauge and Health read the active silos
// beside the shared store, by the same rule. A store whose claims keep failing is one warn line, then at most one every
// ClaimLogEvery, and one line when they work again.
// While attempts run, a watcher reads once every o.Poll which of their rows are cancelled (by
// another process, or alder jobs cancel) and cuts those attempts off (spec §3).
// With no workers or no store it starts nothing, and stop does nothing.
//
// stop(grace) stops claiming at once, waits up to grace for the running attempts (grace <= 0:
// none), then cancels them: a cancelled attempt is re-queued as it was before its claim (its
// attempts not counted, no error), so the next process runs it. It returns when every worker has
// returned, or stopWait after the cancel. stop may be called more than once; only the first call
// does anything. ctx is the workers' parent: the generated App passes context.Background(), and
// stop is how they end.
func Start(ctx context.Context, o Options) (stop func(grace time.Duration)) {
	return start(ctx, o, nil, nil)
}

// start is Start; housekept, when set, is called after each housekeeping (a test waits on it),
// and hintTick, when set, replaces the Poll ticker of the hint reads (a test sends the ticks).
func start(ctx context.Context, o Options, housekept func(), hintTick <-chan time.Time) (stop func(grace time.Duration)) {
	if o.Workers <= 0 || o.Store == nil {
		return func(time.Duration) {}
	}
	if o.Now == nil {
		o.Now = time.Now
	}
	if o.Log == nil {
		o.Log = func(string, string) {}
	}
	if o.Poll <= 0 {
		o.Poll = DefaultPoll
	}
	if o.Housekeep <= 0 {
		o.Housekeep = DefaultHousekeep
	}
	p := newPool(o)
	p.housekept = housekept
	hintStop := func() {}
	switch {
	case hintTick != nil:
		p.hintTick = hintTick
	case o.Hints && o.Stores != nil:
		t := time.NewTicker(o.Poll)
		p.hintTick, hintStop = t.C, t.Stop
	}
	if o.Stores != nil {
		actives.mu.Lock()
		actives.m[o.Stores] = p.active
		actives.mu.Unlock()
	}
	if o.Health != nil {
		o.Health.start(o, p.each)
	}
	if o.Metrics != nil {
		o.Metrics.JobsQueued(p.queuedDue)
	}
	if o.Panel != nil {
		o.Panel.started(p)
	}
	p.due <- struct{}{} // housekeeping at start
	claimCtx, quit := context.WithCancel(ctx)
	runCtx, cancelRuns := context.WithCancelCause(ctx)
	ticker := time.NewTicker(o.Housekeep)
	watchTicker := time.NewTicker(o.Poll)
	host, err := os.Hostname()
	if err != nil {
		host = "localhost"
	}
	watched := make(chan struct{})
	go func() {
		defer close(watched)
		p.watch(runCtx, watchTicker.C)
	}()
	var wg sync.WaitGroup
	for n := 1; n <= o.Workers; n++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			p.work(claimCtx, runCtx, ticker.C, fmt.Sprintf("%s:%d:%d", host, os.Getpid(), n))
		}()
	}
	var once sync.Once
	return func(grace time.Duration) {
		once.Do(func() {
			if o.Health != nil {
				o.Health.stop()
			}
			if o.Panel != nil {
				o.Panel.stopped(p)
			}
			if o.Stores != nil {
				actives.mu.Lock()
				if actives.m[o.Stores] == p.active {
					delete(actives.m, o.Stores)
				}
				actives.mu.Unlock()
			}
			quit()
			ticker.Stop()
			watchTicker.Stop()
			hintStop()
			done := make(chan struct{})
			go func() { wg.Wait(); close(done) }()
			if grace > 0 {
				t := time.NewTimer(grace)
				select {
				case <-done:
				case <-t.C:
				}
				t.Stop()
			}
			cancelRuns(errStopped)
			t := time.NewTimer(stopWait)
			select {
			case <-done:
			case <-t.C:
				o.Log("warn", "jobs: an attempt did not stop when cancelled; its lease re-queues it")
			}
			t.Stop()
			t = time.NewTimer(stopWait)
			select {
			case <-watched: // its read, if any, ends with runCtx
			case <-t.C:
			}
			t.Stop()
		})
	}
}

// pool is the shared state of one Start's workers.
type pool struct {
	o      Options
	kinds  map[string]Kind
	hk     *housekeeper
	due    chan struct{} // the housekeeping at start
	turn   atomic.Uint64 // moves the store a claim looks in first
	active *activeSet    // with o.Stores: the silos polled beside the shared store
	// scanned is set by the start scan (scan), which runs once
	scanned  atomic.Bool
	claimLog claimLog
	// pendingStmt and its arguments give the earliest run_at of the rows queued of a kind the
	// workers know, 0 when a row is running, NULL when there is none (keepSilos)
	pendingStmt string
	pendingArgs []any
	housekept   func() // see start
	// hintDue holds the first read of the hints (Options.Hints); hintTick, a ticker of o.Poll
	// set by start, each later one: whichever worker takes the token reads, the others skip
	hintDue  chan struct{}
	hintTick <-chan time.Time
	// attempts holds each attempt running (*live), by job id, for the watcher
	attempts sync.Map
}

// newPool is the pool of the workers of o (its Now and Log set).
func newPool(o Options) *pool {
	p := &pool{o: o, kinds: map[string]Kind{}, hk: newHousekeeper(o.Store, o.Kinds, o.Log), due: make(chan struct{}, 1)}
	p.claimLog.last = map[string]time.Time{}
	p.hintDue = make(chan struct{}, 1)
	p.hintDue <- struct{}{} // the first read, at the first claim
	// a running row counts as due at 0 (now): its lease is housekeeping's to watch
	p.pendingStmt = `SELECT MIN(CASE "state" WHEN 'running' THEN 0 ELSE "run_at" END) FROM "_alder_jobs" WHERE "state" = 'running'`
	if len(o.Kinds) > 0 {
		p.pendingStmt += ` OR ("state" = 'queued' AND "kind" IN (` + strings.TrimSuffix(strings.Repeat("?, ", len(o.Kinds)), ", ") + `))`
	}
	for _, k := range o.Kinds {
		p.kinds[k.Name] = k
		p.pendingArgs = append(p.pendingArgs, k.Name)
	}
	if o.Stores != nil {
		p.active = newActiveSet(o.Now)
		p.active.left = func(id siloID) { p.claimLog.forget(id.String()) }
		p.hk.silos = p.keepSilos
	}
	return p
}

// housekeep is the housekeeping at now: the start scan first, the first time.
func (p *pool) housekeep(ctx context.Context, now time.Time) {
	p.scan(ctx, now)
	p.hk.run(ctx, now)
	if p.housekept != nil {
		p.housekept()
	}
}

// claim claims the oldest due row of the first store that has one (Claim), the stores taken in
// turn from a start that moves on with every call, so a busy store keeps no other waiting; the
// row's In is the store it was found in. An error of one store does not keep the others from
// being looked in: it is a line (claimed), as is a tick claimed without its next tick (a
// *ScheduleError).
func (p *pool) claim(ctx context.Context, now time.Time, worker string) *Row {
	var silos []siloID
	if p.active != nil {
		silos = p.active.list()
	}
	n := len(silos) + 1
	start := int(p.turn.Add(1) % uint64(n))
	for i := range n {
		var (
			row  *Row
			err  error
			ran  = true // a claim was attempted: not in a silo that was closed or not served
			name = "the shared store"
		)
		if j := (start + i) % n; j == 0 {
			if row, err = Claim(ctx, p.o.Store, p.o.Kinds, now, worker); row != nil {
				row.In = db.Place{Store: p.o.Store}
			}
		} else {
			id := silos[j-1]
			name = id.String()
			ran, err = p.at(ctx, id, func(pl db.Place) error {
				var err error
				if row, err = Claim(ctx, pl.Store, p.o.Kinds, now, worker); row != nil {
					row.In = pl
				}
				return err
			})
			if row != nil {
				p.active.add(id)
			}
		}
		if ctx.Err() != nil {
			return row
		}
		var se *ScheduleError
		if errors.As(err, &se) {
			p.o.Log("warn", "jobs: claiming a job: "+err.Error())
			err = nil
		}
		if ran || err != nil {
			p.claimed(name, err, now)
		}
		if row != nil {
			return row
		}
	}
	return nil
}

// work is one worker's loop: claim on claimCtx, run on runCtx, until claimCtx ends.
func (p *pool) work(claimCtx, runCtx context.Context, tick <-chan time.Time, worker string) {
	for claimCtx.Err() == nil {
		select {
		case <-p.due:
			p.housekeep(claimCtx, p.o.Now())
		case <-tick:
			p.housekeep(claimCtx, p.o.Now())
		default:
		}
		now := p.o.Now()
		p.hints(claimCtx, now)
		row := p.claim(claimCtx, now, worker)
		if row != nil {
			if p.o.Health != nil {
				p.o.Health.claimed(now)
			}
			Wake() // another idle worker looks too: more rows may be due
			p.run(runCtx, *row)
			continue
		}
		t := time.NewTimer(p.o.Poll)
		select {
		case <-claimCtx.Done():
		case <-wake:
		case <-t.C:
		case <-p.hintTick: // a hint read is due: this worker does it (hints) before its claim
			select {
			case p.hintDue <- struct{}{}:
			default:
			}
		case <-p.due:
			p.housekeep(claimCtx, p.o.Now())
		case <-tick:
			p.housekeep(claimCtx, p.o.Now())
		}
		t.Stop()
	}
}

// run runs one attempt of row, which the worker has claimed, and logs and counts it (observe).
// An Env without a Log logs through LogID under the row's id. A panic outside the job's own Task
// (RunOne recovers that one) is logged and leaves the row to its lease; the worker goes on.
func (p *pool) run(ctx context.Context, row Row) {
	if m := p.o.Metrics; m != nil {
		m.JobRunning(1)
		defer m.JobRunning(-1)
	}
	defer func() {
		if v := recover(); v != nil {
			p.o.Log("error", fmt.Sprintf("job %s %s: worker: panic: %v\n%s", row.Kind, row.ID, v, debug.Stack()))
		}
	}()
	ctx, end := p.begin(ctx, row)
	defer end()
	k := p.kinds[row.Kind]
	env := p.o.Env(ctx, k)
	if env.Log == nil {
		env.Log = p.o.Log
		if logID := p.o.LogID; logID != nil {
			env.Log = func(level, msg string) { logID(row.ID, level, msg) }
		}
	}
	if env.Now == nil {
		env.Now = p.o.Now
	}
	if p.o.Panel != nil && env.Rec == nil {
		env.Rec = &eff.Recorder{}
	}
	start := env.Now()
	out := RunOne(env, k, row)
	p.observe(out)
	if p.o.Panel != nil {
		p.o.Panel.record(out, start, env.Rec)
	}
	if p.o.OnAttempt != nil {
		p.o.OnAttempt(out)
	}
}
