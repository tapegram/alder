package main

import (
	"bytes"
	"encoding/json"
	"slices"
	"strings"
	"testing"

	"alder/internal/style"
	"alder/rt/applog"
)

const (
	infoRecord  = `{"time":"2026-11-01T12:00:00.123Z","level":"INFO","msg":"hello"}`
	debugRecord = `{"time":"2026-11-01T12:00:01Z","level":"DEBUG","msg":"step 1"}`
)

// logEvents is every event h holds, decoded.
func logEvents(t *testing.T, h *eventHub) []map[string]any {
	t.Helper()
	backlog, _, cancel := h.subscribe(0)
	defer cancel()
	var es []map[string]any
	for _, b := range backlog {
		var e map[string]any
		if err := json.Unmarshal(b, &e); err != nil {
			t.Fatalf("event %q: %v", b, err)
		}
		es = append(es, e)
	}
	return es
}

// rendered is applog.Render of line at min, failing unless it is a record kept at min.
func rendered(t *testing.T, line string, min applog.Level) string {
	t.Helper()
	text, ok := applog.Render([]byte(line), min)
	if !ok || text == nil {
		t.Fatalf("Render(%q) = %q, %v", line, text, ok)
	}
	return string(text)
}

func TestUserLogging(t *testing.T) {
	for _, c := range []struct {
		log, format string
		level       applog.Level
		json        bool
	}{
		{"", "", applog.Info, false},
		{"debug", "", applog.Debug, false},
		{" WARN ", "text", applog.Warn, false},
		{"", "json", applog.Info, true},
	} {
		env := map[string]string{"ALDER_LOG": c.log, "ALDER_LOG_FORMAT": c.format}
		level, js, err := userLogging(func(k string) string { return env[k] })
		if err != nil || level != c.level || js != c.json {
			t.Errorf("userLogging(ALDER_LOG=%q ALDER_LOG_FORMAT=%q) = %v, %v, %v; want %v, %v", c.log, c.format, level, js, err, c.level, c.json)
		}
	}
	if _, _, err := userLogging(func(k string) string { return map[string]string{"ALDER_LOG": "loud"}[k] }); err == nil || !strings.Contains(err.Error(), "ALDER_LOG=loud") {
		t.Errorf("ALDER_LOG=loud: err = %v, want one naming ALDER_LOG=loud", err)
	}
	if _, _, err := userLogging(func(k string) string { return map[string]string{"ALDER_LOG_FORMAT": "xml"}[k] }); err == nil || !strings.Contains(err.Error(), "ALDER_LOG_FORMAT=xml") {
		t.Errorf("ALDER_LOG_FORMAT=xml: err = %v, want one naming ALDER_LOG_FORMAT=xml", err)
	}
}

func TestChildEnv_DebugJSON(t *testing.T) {
	env := childEnv([]string{"ALDER_LOG=warn", "ALDER_LOG_FORMAT=text", "HOME=/h"}, "dev.db", "static", "g-1", "localhost:8080")
	lastOf := func(key string) string {
		v := ""
		for _, kv := range env {
			if s, ok := strings.CutPrefix(kv, key+"="); ok {
				v = s
			}
		}
		return v
	}
	if got := lastOf("ALDER_LOG"); got != "debug" {
		t.Errorf("last ALDER_LOG = %q, want debug (%q)", got, env)
	}
	if got := lastOf("ALDER_LOG_FORMAT"); got != "json" {
		t.Errorf("last ALDER_LOG_FORMAT = %q, want json (%q)", got, env)
	}
}

func TestLogPipe_RendersAtUserLevel(t *testing.T) {
	var term bytes.Buffer
	h := newEventHub("b00t", eventRing)
	p := newLogPipe(&term, style.Plain, applog.Info, false, h)
	if n, err := p.Write([]byte(infoRecord + "\n" + debugRecord + "\n")); err != nil || n != len(infoRecord)+len(debugRecord)+2 {
		t.Fatalf("Write = %d, %v", n, err)
	}
	if want := rendered(t, infoRecord, applog.Info); term.String() != want {
		t.Errorf("terminal = %q, want %q", term.String(), want)
	}
	es := logEvents(t, h)
	if len(es) != 2 {
		t.Fatalf("%d events, want 2: %v", len(es), es)
	}
	for i, want := range []string{"hello", "step 1"} {
		rec, ok := es[i]["record"].(map[string]any)
		if es[i]["type"] != "log" || !ok || rec["msg"] != want || es[i]["raw"] != nil {
			t.Errorf("event %d = %v, want a log event with the record of %q", i, es[i], want)
		}
	}
}

func TestLogPipe_Styled(t *testing.T) {
	var term bytes.Buffer
	st := style.ForEnv(&term, func(k string) string { return map[string]string{"ALDER_COLOR": "always"}[k] })
	if !st.On() {
		t.Fatal("ALDER_COLOR=always: not styled")
	}
	p := newLogPipe(&term, st, applog.Info, false, newEventHub("b00t", eventRing))
	p.Write([]byte(infoRecord + "\n"))
	want := logLine(st)(strings.TrimSuffix(rendered(t, infoRecord, applog.Info), "\n")) + "\n"
	if term.String() != want {
		t.Errorf("terminal = %q, want %q", term.String(), want)
	}
}

func TestLogPipe_PassesJSONWhenAsked(t *testing.T) {
	var term bytes.Buffer
	h := newEventHub("b00t", eventRing)
	p := newLogPipe(&term, style.Plain, applog.Info, true, h)
	p.Write([]byte(infoRecord + "\n" + debugRecord + "\n"))
	if want := infoRecord + "\n"; term.String() != want {
		t.Errorf("terminal = %q, want the info record unchanged and the debug one dropped: %q", term.String(), want)
	}
	if es := logEvents(t, h); len(es) != 2 {
		t.Errorf("%d events, want 2: %v", len(es), es)
	}
}

func TestLogPipe_RawLines(t *testing.T) {
	var term bytes.Buffer
	h := newEventHub("b00t", eventRing)
	p := newLogPipe(&term, style.Plain, applog.Info, false, h)
	p.Write([]byte("panic: boom\n"))
	if term.String() != "panic: boom\n" {
		t.Errorf("terminal = %q, want %q", term.String(), "panic: boom\n")
	}
	es := logEvents(t, h)
	if len(es) != 1 || es[0]["type"] != "log" || es[0]["raw"] != "panic: boom" || es[0]["record"] != nil {
		t.Errorf("events = %v, want one log event with raw %q and no record", es, "panic: boom")
	}
}

func TestLogPipe_PartialWrites(t *testing.T) {
	var term bytes.Buffer
	h := newEventHub("b00t", eventRing)
	p := newLogPipe(&term, style.Plain, applog.Info, false, h)
	line := infoRecord + "\n"
	for _, part := range []string{line[:10], line[10:30], line[30:]} {
		p.Write([]byte(part))
	}
	if want := rendered(t, infoRecord, applog.Info); term.String() != want {
		t.Errorf("terminal = %q, want %q", term.String(), want)
	}
	if es := logEvents(t, h); len(es) != 1 {
		t.Errorf("%d events, want 1: %v", len(es), es)
	}
	term.Reset()
	p.Write([]byte("exit status"))
	if term.Len() != 0 || len(logEvents(t, h)) != 1 {
		t.Errorf("a partial line went out before Flush: %q", term.String())
	}
	if err := p.Flush(); err != nil {
		t.Fatal(err)
	}
	if term.String() != "exit status" {
		t.Errorf("terminal after Flush = %q, want %q", term.String(), "exit status")
	}
	es := logEvents(t, h)
	if len(es) != 2 || es[1]["raw"] != "exit status" {
		t.Errorf("events after Flush = %v, want a second with raw %q", es, "exit status")
	}
	term.Reset()
	if err := p.Flush(); err != nil || term.Len() != 0 || len(logEvents(t, h)) != 2 {
		t.Errorf("a second Flush wrote %q (%v)", term.String(), err)
	}
}

func TestLogPipe_HugeLine(t *testing.T) {
	msg := strings.Repeat("x", 1<<20)
	line := `{"time":"2026-11-01T12:00:00Z","level":"INFO","msg":"` + msg + `"}`
	var term, tail bytes.Buffer
	h := newEventHub("b00t", eventRing)
	p := newLogPipe(&term, style.Plain, applog.Info, false, h)
	p.tail = &tail
	b := []byte(line + "\n")
	for len(b) > 0 { // the pipe's reads: 32 KiB at a time
		n := min(len(b), 32<<10)
		p.Write(b[:n])
		b = b[n:]
	}
	want := rendered(t, line, applog.Info)
	if term.String() != want {
		t.Errorf("terminal: %d bytes, %d lines; want Render's %d bytes", term.Len(), strings.Count(term.String(), "\n"), len(want))
	}
	if tail.String() != want {
		t.Errorf("tail: %d bytes, want Render's %d", tail.Len(), len(want))
	}
	es := logEvents(t, h)
	if len(es) != 1 {
		t.Fatalf("%d events, want 1", len(es))
	}
	if rec, _ := es[0]["record"].(map[string]any); rec["msg"] != msg {
		t.Errorf("the event's record does not hold the whole message")
	}
	if got := slices.Collect(strings.Lines(term.String())); len(got) != 1 {
		t.Errorf("%d terminal lines, want 1", len(got))
	}
}
