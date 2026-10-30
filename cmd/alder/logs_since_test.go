package main

import (
	"bytes"
	"strings"
	"testing"
	"time"
)

// `alder logs --since T` (#92): T is parsed here and the target is asked for the lines since;
// Render reads the range forward from it.

// lastStart is the startTime of the last forward GET /logs Render was sent.
func lastStart(f *renderFakes) string {
	s := ""
	for _, call := range f.render.Calls() {
		if call.Path == "/logs" && call.Query.Get("direction") == "forward" {
			s = call.Query.Get("startTime")
		}
	}
	return s
}

// Not parallel: it sets logsNow (parallel tests wait for the sequential ones to finish).
func TestLogs_Since(t *testing.T) {
	now := time.Date(2026, 10, 29, 18, 0, 0, 0, time.UTC)
	logsNow = func() time.Time { return now }
	t.Cleanup(func() { logsNow = time.Now })
	dir, f, c := logsApp(t, "one", "two", "three") // stamped 16:00:00, :01, :02
	for _, cse := range []struct {
		arg, start, out string
	}{
		{"2h", "2026-10-29T16:00:00Z", "[i-1] one\n[i-1] two\n[i-1] three\n"},
		{"119m59s", "2026-10-29T16:00:01Z", "[i-1] two\n[i-1] three\n"},
		{"2026-10-29T16:00:02Z", "2026-10-29T16:00:02Z", "[i-1] three\n"},
		{"2026-10-29T12:00:02-04:00", "2026-10-29T16:00:02Z", "[i-1] three\n"},
		{"2026-10-29", "2026-10-29T00:00:00Z", "[i-1] one\n[i-1] two\n[i-1] three\n"},
	} {
		for _, args := range [][]string{{"production", dir, "--since", cse.arg}, {"--since=" + cse.arg, "production", dir}} {
			code, out := runCmd(t, c.logs, args...)
			if code != 0 || out != cse.out {
				t.Errorf("%v: exit %d\n%s", args, code, out)
			}
			if s := lastStart(f); s != cse.start {
				t.Errorf("%v: startTime %q, want %q", args, s, cse.start)
			}
		}
	}
	// --grep applies after
	if code, out := runCmd(t, c.logs, "production", dir, "--since", "2h", "--grep", "t"); code != 0 || out != "[i-1] two\n[i-1] three\n" {
		t.Errorf("--since --grep: exit %d\n%s", code, out)
	}
}

func TestLogs_SinceBad(t *testing.T) {
	t.Parallel()
	dir, f, c := logsApp(t, "one")
	for _, v := range []string{"yesterday", "-2h", "0s", "2026-13-01", "10/29/2026", ""} {
		var so, se bytes.Buffer
		code := c.logs([]string{"production", dir, "--since", v}, &so, &se)
		if code != 2 || !strings.HasPrefix(se.String(), "alder logs: --since: want a duration (2h), an RFC 3339 time or a date\nusage: alder logs ") {
			t.Errorf("%q: exit %d, stderr %q", v, code, se.String())
		}
	}
	var so, se bytes.Buffer
	if code := c.logs([]string{"production", dir, "--since"}, &so, &se); code != 2 {
		t.Errorf("no value: exit %d", code)
	}
	if n := len(f.render.Requests()); n != 0 {
		t.Errorf("a refused --since reached Render: %v", f.render.Requests())
	}
}

func TestLogs_SinceAndN(t *testing.T) {
	t.Parallel()
	dir, f, c := logsApp(t, "one")
	for _, args := range [][]string{{"production", dir, "-n", "5", "--since", "2h"}, {"--since=2h", "production", dir, "-n=5"}} {
		var so, se bytes.Buffer
		code := c.logs(args, &so, &se)
		if code != 2 || !strings.HasPrefix(se.String(), "alder logs: -n and --since do not combine: --since prints every line since T\n") {
			t.Errorf("%v: exit %d, stderr %q", args, code, se.String())
		}
	}
	if n := len(f.render.Requests()); n != 0 {
		t.Errorf("reached Render: %v", f.render.Requests())
	}
}

// sinceWriter keeps the lines stamped at or after since (text, JSON and flyctl stamps, after a
// Render prefix too); an unstamped line goes with the line before it, and before any stamped
// line it is dropped; the writes' boundaries do not matter.
func TestSinceWriter(t *testing.T) {
	since := time.Date(2026, 10, 29, 16, 0, 5, 500_000_000, time.UTC)
	in := strings.Join([]string{
		"listening on :8080", // before any stamp: dropped
		"2026/10/29 16:00:04 info: GET /old 200 1.0ms 1B rid=a",
		"\tcontinuation of old",
		"2026/10/29 16:00:05 error: same second kept rid=b", // the text stamp's second
		"\tstack line",
		`{"time":"2026-10-29T16:00:04.9Z","level":"INFO","msg":"GET /json-old"}`,
		`{"time":"2026-10-29T16:00:06Z","level":"INFO","msg":"GET /json-new"}`,
		"2026-10-29T16:00:03Z app[x] iad [info]fly old",
		"2026-10-29T16:00:07.123Z app[x] iad [info]2026/10/29 16:00:07 info: fly new",
		"[i-1] 2026/10/29 16:00:01 info: render old",
		"[i-1] 2026/10/29 16:00:09 info: render new",
		"--- alder logs: build b2 is current",
		"2026/10/29 15:00:00 info: older again",
		"no stamp after an old line",
		"2026/10/29 16:00:10 info: last without newline",
	}, "\n")
	want := strings.Join([]string{
		"2026/10/29 16:00:05 error: same second kept rid=b",
		"\tstack line",
		`{"time":"2026-10-29T16:00:06Z","level":"INFO","msg":"GET /json-new"}`,
		"2026-10-29T16:00:07.123Z app[x] iad [info]2026/10/29 16:00:07 info: fly new",
		"[i-1] 2026/10/29 16:00:09 info: render new",
		"--- alder logs: build b2 is current",
		"2026/10/29 16:00:10 info: last without newline",
	}, "\n") + "\n"
	for _, chunk := range []int{1, 7, 64, len(in)} {
		var out bytes.Buffer
		s := &sinceWriter{w: &out, since: since}
		for i := 0; i < len(in); i += chunk {
			p := in[i:min(i+chunk, len(in))]
			if n, err := s.Write([]byte(p)); n != len(p) || err != nil {
				t.Fatalf("Write: %d %v", n, err)
			}
		}
		if err := s.Flush(); err != nil {
			t.Fatal(err)
		}
		if out.String() != want {
			t.Errorf("chunk %d:\n%s\nwant:\n%s", chunk, out.String(), want)
		}
	}
}
