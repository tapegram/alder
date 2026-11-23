package jobs

import (
	"fmt"
	"testing"
)

// The ring keeps the last attempt of each of the last AttemptRing jobs; a job run again is
// filed as the newest, so it outlives jobs run before its last attempt.
func TestAttemptRing(t *testing.T) {
	var g attemptRing
	g.add(Attempt{Outcome: Outcome{ID: "keep", Attempt: 1}})
	for i := range AttemptRing - 1 {
		g.add(Attempt{Outcome: Outcome{ID: fmt.Sprint(i)}})
	}
	g.add(Attempt{Outcome: Outcome{ID: "keep", Attempt: 2}}) // run again: the newest
	g.add(Attempt{Outcome: Outcome{ID: "new"}})              // evicts "0", the oldest
	if a, ok := g.get("keep"); !ok || a.Attempt != 2 {
		t.Errorf("keep: %+v %v", a, ok)
	}
	if _, ok := g.get("0"); ok {
		t.Errorf("the oldest job was not evicted")
	}
	for _, id := range []string{"1", fmt.Sprint(AttemptRing - 2), "new"} {
		if _, ok := g.get(id); !ok {
			t.Errorf("%s evicted", id)
		}
	}
}

// The panel's merged listing sorts queued rows in claim order, as List does.
func TestList_QueuedInClaimOrderMerged(t *testing.T) {
	l := Listing{Queued: []Row{
		{ID: "neg", Priority: -5, RunAt: 0},
		{ID: "zero", Priority: 0, RunAt: 1},
		{ID: "ten-late", Priority: 10, RunAt: 9, Created: 1},
		{ID: "ten", Priority: 10, RunAt: 9},
	}}
	l.sort()
	var ids []string
	for _, r := range l.Queued {
		ids = append(ids, r.ID)
	}
	if got := fmt.Sprint(ids); got != "[ten ten-late zero neg]" {
		t.Fatalf("queued %s", got)
	}
}
