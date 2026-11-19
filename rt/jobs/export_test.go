package jobs

import (
	"context"
	"time"

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

// Woken takes Wake's pending signal: whether there was one.
func Woken() bool {
	select {
	case <-wake:
		return true
	default:
		return false
	}
}
