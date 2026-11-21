package jobs

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"alder/rt/db"
)

// A silo that leaves the active set is forgotten by the claim log too: whether it left at
// housekeeping (nothing for ActiveFor) or at once (its key row gone).
func TestClaimLog_ForgetsSiloThatLeft(t *testing.T) {
	now := time.Date(2026, 11, 16, 12, 0, 0, 0, time.UTC)
	p := newPool(Options{Stores: &db.Stores{}, Now: func() time.Time { return now }, Log: func(string, string) {}})
	a, b := siloID{"org", "A"}, siloID{"org", "B"}
	for _, id := range []siloID{a, b} {
		p.active.add(id)
		p.claimed(id.String(), errors.New("down"), now)
	}
	p.active.kept(a, nil, now.Add(ActiveFor))
	p.active.remove(b)
	if n := len(p.claimLog.last); n != 0 || len(p.active.list()) != 0 {
		t.Fatalf("claim log %v, active %v", p.claimLog.last, p.active.list())
	}
}

// A silo the pool did not look in (it is closed and has no row due) is no claim: it neither
// logs "works again" for a failure it had nor resets that failure's rate limit.
func TestClaim_ClosedSiloIsNoClaim(t *testing.T) {
	now := time.Date(2026, 11, 16, 12, 0, 0, 0, time.UTC)
	st, err := db.OpenSQLite(filepath.Join(t.TempDir(), "app.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	var mu sync.Mutex
	var got []string
	log := func(level, msg string) { mu.Lock(); got = append(got, level+" "+msg); mu.Unlock() }
	p := newPool(Options{Store: st, Stores: &db.Stores{}, Now: func() time.Time { return now }, Log: log})
	id := siloID{"org", "A"}
	p.active.add(id)
	p.active.idle(id) // not busy: a poll does not open it
	failed := now.Add(-10 * time.Second)
	p.claimLog.last[id.String()] = failed
	p.claim(context.Background(), now, "w")
	mu.Lock()
	defer mu.Unlock()
	for _, l := range got {
		if strings.Contains(l, "works again") {
			t.Errorf("a closed silo logged %q", l)
		}
	}
	if at, ok := p.claimLog.last[id.String()]; !ok || !at.Equal(failed) {
		t.Errorf("claim log entry %v (%v), want it kept at %v", at, ok, failed)
	}
}
