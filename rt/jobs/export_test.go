package jobs

import (
	"context"
	"strings"
	"time"

	"alder/rt/db"
	"alder/rt/eff"
)

// SetNewID replaces the job id source for a test; the returned func restores it.
func SetNewID(f func() string) (restore func()) {
	old := newID
	newID = f
	return func() { newID = old }
}

// Housekeeper is the housekeeping of Start's workers over s, for a test to run at a time of its
// choosing.
func Housekeeper(s eff.Store, kinds []Kind, log func(level, msg string)) func(ctx context.Context, now time.Time) {
	return newHousekeeper(s, kinds, log).run
}

// SiloHousekeeper is Housekeeper keeping the silos open in stores too, as Start's workers do
// with Options.Stores when those silos are in their active set; now is the active set's clock.
func SiloHousekeeper(s eff.Store, stores *db.Stores, kinds []Kind, now func() time.Time, log func(level, msg string)) func(ctx context.Context, now time.Time) {
	p := newPool(Options{Store: s, Stores: stores, Kinds: kinds, Log: log, Now: now})
	return func(ctx context.Context, now time.Time) {
		open, err := stores.OpenSilos(ctx)
		if err != nil {
			panic(err)
		}
		for _, pl := range open {
			p.active.add(siloID{pl.Key, pl.Value})
		}
		p.hk.run(ctx, now)
	}
}

// StartHousekept is Start calling housekept after each housekeeping of its workers.
func StartHousekept(ctx context.Context, o Options, housekept func()) (stop func(grace time.Duration)) {
	return start(ctx, o, housekept, nil)
}

// StartHintTicked is StartHousekept with the hint reads' ticks sent on tick instead of a ticker
// of o.Poll.
func StartHintTicked(ctx context.Context, o Options, housekept func(), tick <-chan time.Time) (stop func(grace time.Duration)) {
	return start(ctx, o, housekept, tick)
}

// Woken takes Wake's pending signal: whether there was one.
func Woken() bool {
	select {
	case <-wake:
		return true
	default:
		return false
	}
}

// Active is the active set of the workers started on s (Options.Stores), "key/value" sorted;
// nil when none are.
func Active(s *db.Stores) []string {
	a := activeOf(s)
	if a == nil {
		return nil
	}
	var out []string
	for _, m := range a.list() {
		out = append(out, m.key+"/"+m.value)
	}
	return out
}

// StoppedContext is a context the workers' stop has cancelled.
func StoppedContext() context.Context {
	ctx, cancel := context.WithCancelCause(context.Background())
	cancel(errStopped)
	return ctx
}

// PanelActive registers with panel a pool of no workers on s and stores whose active set holds
// the silos ids ("key/value"), busy and due, as Start's workers would: the panel then reads a silo
// not open through the workers' poll (pool.at), which opens it.
func PanelActive(panel *Panel, s eff.Store, stores *db.Stores, now func() time.Time, ids ...string) {
	p := newPool(Options{Store: s, Stores: stores, Now: now, Log: func(string, string) {}})
	for _, id := range ids {
		k, v, _ := strings.Cut(id, "/")
		p.active.add(siloID{k, v})
	}
	panel.started(p)
}

// PanelIdle marks silo id ("key/value") of panel's active set (PanelActive) not busy: its poll no
// longer opens it.
func PanelIdle(panel *Panel, id string) {
	k, v, _ := strings.Cut(id, "/")
	panel.pool.active.idle(siloID{k, v})
}
