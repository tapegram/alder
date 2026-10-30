package main

import (
	"bytes"
	"regexp"
	"strings"
	"testing"
	"time"

	"alder/internal/renderfake"
)

// `alder logs` against the Render fake (internal/renderfake): -n and --grep are cmd/alder's own,
// the same for every target.

// logsApp is renderApp with the service already at Render and msgs in its log: no deploy.
func logsApp(t *testing.T, msgs ...string) (string, *renderFakes, *deployCmds) {
	t.Helper()
	dir, f, c := renderApp(t, rdDecl)
	f.render.AddService("blog", "ghcr.io/tavish/blog:abc", 1)
	var lines []renderfake.LogLine
	t0 := time.Date(2026, 10, 29, 16, 0, 0, 0, time.UTC)
	for i, m := range msgs {
		lines = append(lines, renderfake.LogLine{Time: t0.Add(time.Duration(i) * time.Second), Instance: "i-1", Message: m})
	}
	f.render.Logs(lines...)
	return dir, f, c
}

// lastLimit is the limit of the last GET /logs Render was sent.
func lastLimit(f *renderFakes) string {
	limit := ""
	for _, call := range f.render.Calls() {
		if call.Path == "/logs" {
			limit = call.Query.Get("limit")
		}
	}
	return limit
}

// `alder logs -n N` asks the target for the last N lines; the default is 100.
func TestLogs_N(t *testing.T) {
	t.Parallel()
	dir, f, c := logsApp(t, "one", "two", "three")
	for _, args := range [][]string{{"production", dir, "-n", "2"}, {"production", dir, "-n=2"}, {"-n", "2", "production", dir}} {
		code, out := runCmd(t, c.logs, args...)
		if code != 0 || out != "[i-1] two\n[i-1] three\n" {
			t.Errorf("%v: exit %d\n%s", args, code, out)
		}
		if l := lastLimit(f); l != "2" {
			t.Errorf("%v: limit %q", args, l)
		}
	}
	if code, out := runCmd(t, c.logs, "production", dir); code != 0 || out != "[i-1] one\n[i-1] two\n[i-1] three\n" {
		t.Errorf("default: exit %d\n%s", code, out)
	}
	if l := lastLimit(f); l != "100" {
		t.Errorf("default limit %q", l)
	}
	// more than Render gives at once: the last 100, and a note on stderr saying so
	var so, se bytes.Buffer
	if code := c.logs([]string{"production", dir, "-n", "500"}, &so, &se); code != 0 || so.String() != "[i-1] one\n[i-1] two\n[i-1] three\n" ||
		se.String() != "alder logs: Render returns at most 100 lines at once: these are the last 100\n" || lastLimit(f) != "100" {
		t.Errorf("-n 500: exit %d\nstdout %q\nstderr %q", code, so.String(), se.String())
	}
	se.Reset()
	if c.logs([]string{"production", dir, "-n", "100"}, &so, &se); se.Len() != 0 {
		t.Errorf("-n 100 noted: %q", se.String())
	}
}

// `alder logs --grep PATTERN` prints only the lines the regexp matches: a request id's lines.
func TestLogs_Grep(t *testing.T) {
	t.Parallel()
	dir, _, c := logsApp(t,
		"2026/10/29 16:00:00 info: GET /start?key=•••• 200 1.2ms 10B rid=5f3a2c1e",
		"2026/10/29 16:00:00 error: GET /boom: kaboom rid=0a0b0c0d",
		"2026/10/29 16:00:01 info: GET /boom 500 0.4ms 30B rid=0a0b0c0d",
		"2026/10/29 16:00:02 warn: the session cookie is 5000 bytes")
	code, out := runCmd(t, c.logs, "production", dir, "--grep", "rid=0a0b0c0d")
	if code != 0 || out != "[i-1] 2026/10/29 16:00:00 error: GET /boom: kaboom rid=0a0b0c0d\n[i-1] 2026/10/29 16:00:01 info: GET /boom 500 0.4ms 30B rid=0a0b0c0d\n" {
		t.Errorf("--grep rid: exit %d\n%s", code, out)
	}
	code, out = runCmd(t, c.logs, "production", dir, "--grep=(warn|error):", "-n", "4")
	if code != 0 || strings.Count(out, "\n") != 2 || !strings.Contains(out, "kaboom") || !strings.Contains(out, "session cookie") {
		t.Errorf("--grep levels: exit %d\n%s", code, out)
	}
	if code, out = runCmd(t, c.logs, "production", dir, "--grep", "nothing-matches"); code != 0 || out != "" {
		t.Errorf("no match: exit %d\n%q", code, out)
	}
}

// Bad -n and --grep values are usage errors, refused before the target is reached.
func TestLogs_BadFlags(t *testing.T) {
	t.Parallel()
	dir, f, c := logsApp(t, "one")
	for _, args := range [][]string{
		{"production", dir, "-n", "0"}, {"production", dir, "-n", "x"}, {"production", dir, "-n"},
		{"production", dir, "--grep"}, {"production", dir, "--grep", "("},
	} {
		if code, out := runCmd(t, c.logs, args...); code != 2 || !strings.Contains(out, "alder logs") {
			t.Errorf("%v: exit %d\n%s", args, code, out)
		}
	}
	if n := len(f.render.Requests()); n != 0 {
		t.Errorf("a refused flag reached Render: %v", f.render.Requests())
	}
}

// grepWriter passes the whole lines that match, however the writes split them; a last line
// without a newline is judged at Flush.
func TestGrepWriter(t *testing.T) {
	var out bytes.Buffer
	g := &grepWriter{w: &out, re: regexp.MustCompile(`rid=ab`)}
	for _, p := range []string{"x rid=ab 1\ny", " rid=a", "b 2\nz\n", "no\nlast rid=ab"} {
		if n, err := g.Write([]byte(p)); n != len(p) || err != nil {
			t.Fatalf("Write(%q) = %d, %v", p, n, err)
		}
	}
	if out.String() != "x rid=ab 1\ny rid=ab 2\n" {
		t.Errorf("before Flush: %q", out.String())
	}
	if err := g.Flush(); err != nil {
		t.Fatal(err)
	}
	if out.String() != "x rid=ab 1\ny rid=ab 2\nlast rid=ab\n" {
		t.Errorf("after Flush: %q", out.String())
	}
}
