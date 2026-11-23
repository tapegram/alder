package applog

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

// A job attempt's line: its text and level by outcome (spec §9), the write-lock mark (§6a).
func TestJob_Text(t *testing.T) {
	for _, c := range []struct {
		j     Job
		level Level
		text  string
	}{
		{Job{ID: "a1b2c3d4", Kind: "Signup.SendWelcome", Outcome: "done", Dur: 120 * time.Millisecond, Attempt: 1, Attempts: 5},
			Info, "job Signup.SendWelcome a1b2c3d4 done 120ms attempt 1"},
		{Job{ID: "a1b2c3d4", Kind: "Signup.SendWelcome", Outcome: "failed", Dur: time.Second, Attempt: 2, Attempts: 5, Err: "smtp: timeout", Next: time.Minute},
			Warn, `job Signup.SendWelcome a1b2c3d4 failed attempt 2/5 "smtp: timeout", next in 1m`},
		{Job{ID: "a1b2c3d4", Kind: "Signup.SendWelcome", Outcome: "dead", Attempt: 5, Attempts: 5, Err: "smtp: timeout\nat line 2"},
			Error, `job Signup.SendWelcome a1b2c3d4 failed attempt 5/5 "smtp: timeout\nat line 2", dead`},
		{Job{ID: "a1b2c3d4", Kind: "Signup.SendWelcome", Outcome: "stopped", Attempt: 1, Attempts: 5},
			Info, "job Signup.SendWelcome a1b2c3d4 stopped, queued again"},
		{Job{ID: "a1b2c3d4", Kind: "Signup.SendWelcome", Outcome: "cancelled", Dur: 120 * time.Millisecond, Attempt: 1, Attempts: 5},
			Info, "job Signup.SendWelcome a1b2c3d4 cancelled 120ms attempt 1"},
		{Job{ID: "a1b2c3d4", Kind: "Digest.Send", Outcome: "done", Dur: 41500 * time.Millisecond, Attempt: 1, Attempts: 5, LockHeld: 41 * time.Second},
			Warn, "job Digest.Send a1b2c3d4 done 41500ms attempt 1, held the write lock 41 s: write last, or fan out — book 17"},
		{Job{ID: "a1b2c3d4", Kind: "Digest.Send", Outcome: "done", Dur: 2 * time.Second, Attempt: 1, Attempts: 5, LockHeld: LockMark},
			Info, "job Digest.Send a1b2c3d4 done 2000ms attempt 1"},
	} {
		if got := c.j.Text(); got != c.text {
			t.Errorf("text:\n%s\nwant:\n%s", got, c.text)
		}
		if got := c.j.Level(); got != c.level {
			t.Errorf("%s: level %v, want %v", c.text, got, c.level)
		}
	}
}

// As text, the line is the stamp, the level and Text; as JSON, the message is Text and the
// record carries the job's id as rid (the key a request's lines use), its kind, outcome, ms and
// attempt. Render shows the JSON record as the text line, so `alder run` and its event feed
// show it like any line.
func TestJob_Logger(t *testing.T) {
	j := &Job{ID: "a1b2c3d4", Kind: "Signup.SendWelcome", Outcome: "failed", Dur: 1500 * time.Millisecond, Attempt: 2, Attempts: 5, Err: "smtp: timeout", Next: time.Minute}
	var text bytes.Buffer
	newAt(&text, Info, Text).Job(j)
	want := "2026/10/29 20:00:00 warn: " + j.Text() + "\n"
	if text.String() != want {
		t.Fatalf("text %q, want %q", text.String(), want)
	}
	var js bytes.Buffer
	newAt(&js, Info, JSON).Job(j)
	var m map[string]any
	if err := json.Unmarshal(js.Bytes(), &m); err != nil {
		t.Fatal(err)
	}
	if m["level"] != "WARN" || m["msg"] != j.Text() || m["rid"] != "a1b2c3d4" || m["job"] != "Signup.SendWelcome" ||
		m["outcome"] != "failed" || m["ms"] != 1500.0 || m["attempt"] != 2.0 {
		t.Fatalf("record %v", m)
	}
	if _, ok := m["lock_ms"]; ok {
		t.Fatalf("lock_ms without the mark: %v", m)
	}
	if got, ok := Render(js.Bytes(), Info); !ok || string(got) != want {
		t.Fatalf("Render %q %v, want %q", got, ok, want)
	}
	// the mark's record carries the time the lock was held
	js.Reset()
	newAt(&js, Info, JSON).Job(&Job{ID: "a1b2c3d4", Kind: "K.J", Outcome: "done", Attempt: 1, Attempts: 1, LockHeld: 2100 * time.Millisecond})
	if !strings.Contains(js.String(), `"lock_ms":2100`) {
		t.Fatalf("no lock_ms: %s", js.String())
	}
	// below the level: nothing
	js.Reset()
	newAt(&js, Error, JSON).Job(j)
	if js.Len() != 0 {
		t.Fatalf("written below the level: %s", js.String())
	}
}
