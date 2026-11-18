package jobs

import (
	"context"
	"errors"
	"fmt"
	"os"
	"runtime/debug"
	"sync"
	"time"

	"alder/rt/eff"
)

const (
	// DefaultPoll is how often an idle worker looks for a due row (Options.Poll's default).
	DefaultPoll = time.Second
	// DefaultHousekeep is how often a process's workers keep the table (Options.Housekeep's
	// default): expired leases, pruning, unclaimable kinds.
	DefaultHousekeep = time.Minute
	// LeaseMargin is how much longer than its kind's timeout a claimed row's lease runs.
	LeaseMargin = 30 * time.Second
	// maxTimeout is the longest timeout a job may declare (spec §1), the lease of a kind
	// without one.
	maxTimeout = time.Hour
	// stopWait is how long stop waits, after it cancels the attempts still running, for them to
	// be re-queued; an attempt that ignores its cancellation is left to its lease.
	stopWait = time.Second
)

// Options configure Start.
type Options struct {
	Store eff.Store // the store whose jobs table the workers claim from (the shared one)
	Kinds []Kind    // the kinds this build knows (generated: app.Jobs()); rows of others are left
	// Env is the Env an attempt of a job of kind k runs with (generated: App.JobEnv); ctx is
	// cancelled when stop gives up waiting for the attempt.
	Env     func(ctx context.Context, k Kind) *eff.Env
	Workers int                     // goroutines claiming and running rows; 0 or less: none
	Now     func() time.Time        // the clock (nil: the wall clock)
	Log     func(level, msg string) // nil: no log; an Env without a Log logs here
	// OnAttempt, when set, is called with the Outcome of every attempt a worker ran.
	OnAttempt func(Outcome)
	// Poll is how long an idle worker waits for a wake before it looks again (0: DefaultPoll);
	// Housekeep the interval of the housekeeping (0: DefaultHousekeep).
	Poll, Housekeep time.Duration
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

// Start starts o.Workers workers. Each claims the oldest due row of a kind it knows (Claim),
// runs it (RunOne on o.Env's Env), and claims again; with none due it waits o.Poll or until
// Wake. One worker at a time keeps the table (housekeeping): at start and every o.Housekeep.
// With no workers or no store it starts nothing, and stop does nothing.
//
// stop(grace) stops claiming at once, waits up to grace for the running attempts (grace <= 0:
// none), then cancels them: a cancelled attempt is re-queued as it was before its claim (its
// attempts not counted, no error), so the next process runs it. It returns when every worker has
// returned, or stopWait after the cancel. stop may be called more than once; only the first call
// does anything. ctx is the workers' parent: the generated App passes context.Background(), and
// stop is how they end.
func Start(ctx context.Context, o Options) (stop func(grace time.Duration)) {
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
	p := &pool{o: o, kinds: map[string]Kind{}, hk: newHousekeeper(o.Store, o.Kinds, o.Log), due: make(chan struct{}, 1)}
	for _, k := range o.Kinds {
		p.kinds[k.Name] = k
		p.names = append(p.names, k.Name)
		t := k.Timeout
		if t <= 0 {
			t = maxTimeout
		}
		p.lease = max(p.lease, t+LeaseMargin)
	}
	p.due <- struct{}{} // housekeeping at start
	claimCtx, quit := context.WithCancel(ctx)
	runCtx, cancelRuns := context.WithCancelCause(ctx)
	ticker := time.NewTicker(o.Housekeep)
	host, err := os.Hostname()
	if err != nil {
		host = "localhost"
	}
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
			quit()
			ticker.Stop()
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
		})
	}
}

// pool is the shared state of one Start's workers.
type pool struct {
	o     Options
	kinds map[string]Kind
	names []string
	lease time.Duration // the longest of the kinds' timeouts, plus LeaseMargin
	hk    *housekeeper
	due   chan struct{} // the housekeeping at start
}

// work is one worker's loop: claim on claimCtx, run on runCtx, until claimCtx ends.
func (p *pool) work(claimCtx, runCtx context.Context, tick <-chan time.Time, worker string) {
	for claimCtx.Err() == nil {
		select {
		case <-p.due:
			p.hk.run(claimCtx, p.o.Now())
		case <-tick:
			p.hk.run(claimCtx, p.o.Now())
		default:
		}
		row, err := Claim(claimCtx, p.o.Store, p.names, p.o.Now(), p.lease, worker)
		if err != nil && claimCtx.Err() == nil {
			p.o.Log("warn", "jobs: claiming a job: "+err.Error())
		}
		if row != nil {
			Wake() // another idle worker looks too: more rows may be due
			p.run(runCtx, *row)
			continue
		}
		t := time.NewTimer(p.o.Poll)
		select {
		case <-claimCtx.Done():
		case <-wake:
		case <-t.C:
		case <-p.due:
			p.hk.run(claimCtx, p.o.Now())
		case <-tick:
			p.hk.run(claimCtx, p.o.Now())
		}
		t.Stop()
	}
}

// run runs one attempt of row, which the worker has claimed. A panic outside the job's own Task
// (RunOne recovers that one) is logged and leaves the row to its lease; the worker goes on.
func (p *pool) run(ctx context.Context, row Row) {
	defer func() {
		if v := recover(); v != nil {
			p.o.Log("error", fmt.Sprintf("job %s %s: worker: panic: %v\n%s", row.Kind, row.ID, v, debug.Stack()))
		}
	}()
	k := p.kinds[row.Kind]
	env := p.o.Env(ctx, k)
	if env.Log == nil {
		env.Log = p.o.Log
	}
	if env.Now == nil {
		env.Now = p.o.Now
	}
	out := RunOne(env, k, row)
	if p.o.OnAttempt != nil {
		p.o.OnAttempt(out)
	}
}
