package jobs

import (
	"context"
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
	return start(ctx, o, housekept)
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
