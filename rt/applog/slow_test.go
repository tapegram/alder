package applog

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

var slowSample = Access{ID: "5f3a2c1e", Method: "GET", Path: "/todos/3", Status: 200, Dur: 812_420 * time.Microsecond, Bytes: 4821,
	Steps: 12, Route: "/todos/{id}", Stmts: 9, DBDur: 640_140 * time.Microsecond, Slow: true}

// A slow request's line is a warn line with its breakdown: route, steps (whatever the level) and
// the database's statements and time.
func TestText_AccessSlow(t *testing.T) {
	for _, min := range []Level{Debug, Info, Warn} {
		var b bytes.Buffer
		a := slowSample
		newAt(&b, min, Text).Access(&a)
		want := "2026/10/29 20:00:00 warn: GET /todos/3 200 812.4ms 4821B rid=5f3a2c1e slow route=/todos/{id} steps=12 db=9/640.1ms\n"
		if b.String() != want {
			t.Errorf("%v:\n got %q\nwant %q", min, b.String(), want)
		}
	}
	// a record that is not Slow is written as before, without a route
	var b bytes.Buffer
	a := slowSample
	a.Slow = false
	newAt(&b, Info, Text).Access(&a)
	if b.String() != "2026/10/29 20:00:00 info: GET /todos/3 200 812.4ms 4821B rid=5f3a2c1e\n" {
		t.Errorf("not slow: %q", b.String())
	}
}

func TestJSON_AccessSlow(t *testing.T) {
	var b bytes.Buffer
	a := slowSample
	newAt(&b, Info, JSON).Access(&a)
	var got map[string]any
	dec := json.NewDecoder(&b)
	dec.UseNumber()
	if err := dec.Decode(&got); err != nil {
		t.Fatal(err)
	}
	want := map[string]any{
		"time": "2026-10-29T20:00:00.123Z", "level": "WARN", "msg": "GET /todos/3",
		"method": "GET", "path": "/todos/3", "status": json.Number("200"), "ms": json.Number("812.4"),
		"bytes": json.Number("4821"), "rid": "5f3a2c1e",
		"slow": true, "route": "/todos/{id}", "steps": json.Number("12"), "db_stmts": json.Number("9"), "db_ms": json.Number("640.1"),
	}
	if len(got) != len(want) {
		t.Errorf("keys %v, want %v", got, want)
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("%s = %#v, want %#v", k, got[k], v)
		}
	}
	// not slow: none of the five keys
	b.Reset()
	a.Slow = false
	newAt(&b, Info, JSON).Access(&a)
	for _, k := range []string{`"slow"`, `"route"`, `"steps"`, `"db_stmts"`, `"db_ms"`} {
		if strings.Contains(b.String(), k) {
			t.Errorf("not slow has %s: %s", k, b.String())
		}
	}
}

// A slow line passes a warn filter, Quiet or not; an error filter drops it.
func TestAccess_SlowAtWarn(t *testing.T) {
	for _, quiet := range []bool{false, true} {
		var b bytes.Buffer
		a := slowSample
		a.Quiet = quiet
		newAt(&b, Warn, Text).Access(&a)
		if !strings.Contains(b.String(), " warn: GET /todos/3 ") {
			t.Errorf("quiet %v: %q", quiet, b.String())
		}
		b.Reset()
		newAt(&b, Error, Text).Access(&a)
		if b.Len() != 0 {
			t.Errorf("under error: %q", b.String())
		}
	}
}

func TestSlowFromEnv(t *testing.T) {
	for v, want := range map[string]time.Duration{"": 0, " ": 0, "0": 0, "250": 250 * time.Millisecond, " 1 ": time.Millisecond} {
		if d, err := slowFromEnv(env(map[string]string{"ALDER_SLOW_MS": v})); err != nil || d != want {
			t.Errorf("%q: %v, %v", v, d, err)
		}
	}
	if d, err := slowFromEnv(env(nil)); d != 0 || err != nil {
		t.Errorf("unset: %v, %v", d, err)
	}
	for _, v := range []string{"abc", "-1", "1.5", "250ms", "99999999999999999999"} {
		_, err := slowFromEnv(env(map[string]string{"ALDER_SLOW_MS": v}))
		if err == nil || err.Error() != "ALDER_SLOW_MS="+v+": want a whole number of milliseconds (0: off)" {
			t.Errorf("%q: %v", v, err)
		}
	}
}
