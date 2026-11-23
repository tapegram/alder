package store

import (
	"testing"
	"time"

	"alder/tui/internal/feed"
)

// A job attempt's record is a scrollback line marked Job, with the job's id as its Rid (so the rid
// filter finds it, as it finds a request's lines), at the record's level, and it is no request.
func TestLogs_JobLine(t *testing.T) {
	r := &feed.Record{Time: time.Date(2026, 11, 1, 14, 2, 20, 0, time.UTC), Level: "ERROR", Rid: "a1b2c3d4", HasJob: true,
		Job: "Signup.SendWelcome", Outcome: "dead", Attempt: 5,
		Msg: `job Signup.SendWelcome a1b2c3d4 failed attempt 5/5 "smtp: timeout", dead`}
	e := feed.Event{Type: "log", Seq: 7, Log: &feed.Log{Record: r}}
	lg := NewLogs(10)
	lg.Add(e)
	lg.Add(feed.Event{Type: "log", Seq: 8, Log: &feed.Log{Record: rec("INFO", "other")}})
	v := lg.View(Filter{Level: "info", Rid: "a1b2c3d4"})
	if len(v) != 1 || !v[0].Job || v[0].Access || v[0].Level != "error" || v[0].Rid != "a1b2c3d4" {
		t.Fatalf("job line = %+v", v)
	}
	if want := `2026/11/01 14:02:20 error: job Signup.SendWelcome a1b2c3d4 failed attempt 5/5 "smtp: timeout", dead`; v[0].Text != want {
		t.Errorf("text = %q, want %q", v[0].Text, want)
	}
	if other := lg.View(Filter{Rid: "zzz"}); len(other) != 0 {
		t.Errorf("rid filter found %q", texts(other))
	}
	q := NewRequests(10)
	q.Add(e)
	if q.Len() != 0 {
		t.Errorf("a job record is a request")
	}
}
