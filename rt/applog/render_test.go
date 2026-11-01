package applog

import (
	"bytes"
	"testing"
	"time"
)

var renderCalls = []struct {
	name string
	run  func(lg *Logger)
}{
	{"info", func(lg *Logger) { lg.Log("info", "hello") }},
	{"debug", func(lg *Logger) { lg.Log("debug", "detail") }},
	{"continuation", func(lg *Logger) { lg.Log("warn", "careful\nsecond line") }},
	{"control", func(lg *Logger) { lg.Log("error", "bad \x1b[31m") }},
	{"effect", func(lg *Logger) { lg.Log("effect", "Db.all Todo") }},
	{"custom", func(lg *Logger) { lg.Log("audit", "custom level") }},
	{"access", func(lg *Logger) {
		lg.Access(&Access{ID: "5f3a2c1e", Method: "GET", Path: "/start?key=••••", Status: 200, Dur: 12300 * time.Microsecond, Bytes: 4821, Steps: 3})
	}},
	{"quiet", func(lg *Logger) {
		lg.Access(&Access{ID: "a1", Method: "GET", Path: "/static/x.css", Status: 200, Quiet: true})
	}},
	{"slow", func(lg *Logger) {
		lg.Access(&Access{ID: "b2", Method: "GET", Path: "/todos/3", Status: 200, Dur: 812400 * time.Microsecond, Bytes: 4821, Steps: 12, Slow: true, Route: "/todos/{id}", Stmts: 9, DBDur: 640100 * time.Microsecond})
	}},
}

func TestRender_MatchesTextLogger(t *testing.T) {
	fixed := func() time.Time { return time.Date(2026, 11, 1, 14, 2, 11, 204000000, time.UTC) }
	for _, min := range []Level{Debug, Info, Warn, Error} {
		for _, c := range renderCalls {
			var txt, js bytes.Buffer
			tl, jl := New(&txt, min, Text), New(&js, Debug, JSON)
			tl.now, jl.now = fixed, fixed
			c.run(tl)
			c.run(jl)
			got, ok := Render(js.Bytes(), min)
			if !ok {
				t.Fatalf("%s at %s: not a record: %q", c.name, min, js.Bytes())
			}
			if txt.Len() == 0 {
				if got != nil {
					t.Errorf("%s at %s: want dropped, got %q", c.name, min, got)
				}
				continue
			}
			if string(got) != txt.String() {
				t.Errorf("%s at %s:\n got %q\nwant %q", c.name, min, got, txt.String())
			}
		}
	}
}

func TestRender_NotARecord(t *testing.T) {
	for _, s := range []string{"panic: boom", `{"a":1}`, `[1]`, `{"time":"2026-11-01T14:02:11Z","level":"INFO"}`, ""} {
		if text, ok := Render([]byte(s), Info); ok || text != nil {
			t.Errorf("Render(%q) = %q, %v; want not a record", s, text, ok)
		}
	}
}

func TestRender_IgnoresUnknownKeys(t *testing.T) {
	plain := `{"time":"2026-11-01T14:02:11.204Z","level":"INFO","msg":"hello"}`
	extra := `{"time":"2026-11-01T14:02:11.204Z","level":"INFO","msg":"hello","rid":"x"}`
	a, ok1 := Render([]byte(plain), Info)
	b, ok2 := Render([]byte(extra), Info)
	if !ok1 || !ok2 || string(a) != string(b) || string(a) != "2026/11/01 14:02:11 info: hello\n" {
		t.Errorf("got %q (%v) and %q (%v)", a, ok1, b, ok2)
	}
}
