package std

import (
	"strings"
	"testing"

	"alder/rt"
	"alder/rt/jobs"
)

// A state the stdlib has no constructor for (a row written by a newer build, a hand edit) is an
// error, never reported as Dead.
func TestJobStatusOf_UnknownState(t *testing.T) {
	for _, st := range []jobs.State{jobs.Queued, jobs.Running, jobs.Done, jobs.Dead} {
		if _, err := JobStatusOf(jobs.Status{State: st}); err != nil {
			t.Errorf("%s: %v", st, err)
		}
	}
	s, err := JobStatusOf(jobs.Status{State: "paused"})
	if err == nil || !strings.Contains(err.Error(), `"paused"`) {
		t.Errorf("unknown state: %+v, %v; want an error naming it", s, err)
	}
}

// A decode error names its path; at the root (the parameters themselves) it names none.
func TestJobWant_Path(t *testing.T) {
	list := rt.JsonValue{Kind: rt.JsonArray}
	if got := JobWant("", "an object", list).Error(); got != "expected an object, got a list" {
		t.Errorf("root: %q", got)
	}
	if got := JobWant("user", "a Text", list).Error(); got != "user: expected a Text, got a list" {
		t.Errorf("field: %q", got)
	}
	if _, err := JobField(list, "", "user", JobDecText); err == nil || err.Error() != "expected an object, got a list" {
		t.Errorf("JobField at the root: %v", err)
	}
}
