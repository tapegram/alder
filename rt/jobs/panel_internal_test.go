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
