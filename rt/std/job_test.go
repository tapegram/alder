package std

import (
	"context"
	"strings"
	"testing"
	"time"

	"alder/rt"
	"alder/rt/db"
	"alder/rt/eff"
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

// A cancelled row is the stdlib's Cancelled, with no due time.
func TestJobStatusOf_Cancelled(t *testing.T) {
	s, err := JobStatusOf(jobs.Status{State: "cancelled"})
	if err != nil || s.State.Tag != JobCancelled || s.RunAt != rt.Nothing[time.Time]() {
		t.Fatalf("%+v, %v", s, err)
	}
	if got := rt.Show(s.State); got != "Cancelled" {
		t.Errorf("Show = %q", got)
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

// Job.enqueueIn is the Env's clock plus its milliseconds; zero and less is now.
func TestJobEnqueueIn_Clock(t *testing.T) {
	st, err := db.OpenSQLite(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	if err := st.Ensure(context.Background(), nil, true); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 11, 22, 12, 0, 0, 0, time.UTC)
	env := &eff.Env{Ctx: context.Background(), Store: st, Now: func() time.Time { return now }}
	j := rt.Job{Kind: "Signup.SendWelcome", Params: `{"user":"u1"}`}
	for _, c := range []struct {
		ms   int64
		want time.Time
	}{{60000, now.Add(time.Minute)}, {0, now}, {-5, now}} {
		id, err := eff.Run(env, eff.KTask, JobEnqueueIn(c.ms, j))
		if err != nil {
			t.Fatal(err)
		}
		s, err := jobs.StatusOf(env, id)
		if err != nil || s == nil || s.RunAt == nil {
			t.Fatalf("enqueueIn %d: %+v, %v", c.ms, s, err)
		}
		if !s.RunAt.Equal(c.want) {
			t.Errorf("enqueueIn %d: run_at %v, want %v", c.ms, s.RunAt, c.want)
		}
	}
	if last := env.Trace[len(env.Trace)-1]; last != "Job.enqueueIn SendWelcome" {
		t.Errorf("trace %q", env.Trace)
	}
}
