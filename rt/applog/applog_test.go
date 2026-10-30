package applog

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http/httptest"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"
)

var fixed = time.Date(2026, 10, 29, 16, 0, 0, 123_000_000, time.FixedZone("EDT", -4*3600))

// newAt is New with the clock stopped at fixed.
func newAt(w io.Writer, min Level, f Format) *Logger {
	l := New(w, min, f)
	l.now = func() time.Time { return fixed }
	return l
}

func TestParseLevel(t *testing.T) {
	for in, want := range map[string]Level{"debug": Debug, "info": Info, "warn": Warn, "error": Error, "DEBUG": Debug, " Warn ": Warn, "Error\n": Error} {
		if got, err := ParseLevel(in); err != nil || got != want {
			t.Errorf("ParseLevel(%q) = %v, %v; want %v", in, got, err, want)
		}
	}
	for _, in := range []string{"verbose", "warning", "", "trace", "effect"} {
		if _, err := ParseLevel(in); err == nil {
			t.Errorf("ParseLevel(%q) accepted", in)
		}
	}
}

func env(vars map[string]string) func(string) string {
	return func(k string) string { return vars[k] }
}

func TestFromEnv_Defaults(t *testing.T) {
	for _, vars := range []map[string]string{{}, {"ALDER_LOG": "", "ALDER_LOG_FORMAT": "  "}} {
		l, err := fromEnv(io.Discard, env(vars))
		if err != nil || l.min != Info || l.format != Text {
			t.Errorf("%v: %+v, %v; want info, text", vars, l, err)
		}
	}
	l, err := fromEnv(io.Discard, env(map[string]string{"ALDER_LOG": "DEBUG", "ALDER_LOG_FORMAT": " Json "}))
	if err != nil || l.min != Debug || l.format != JSON {
		t.Errorf("DEBUG, Json: %+v, %v", l, err)
	}
}

func TestFromEnv_BadLevel(t *testing.T) {
	_, err := fromEnv(io.Discard, env(map[string]string{"ALDER_LOG": "verbose"}))
	if err == nil || err.Error() != "ALDER_LOG=verbose: want debug, info, warn or error" {
		t.Errorf("err = %v", err)
	}
}

func TestFromEnv_BadFormat(t *testing.T) {
	_, err := fromEnv(io.Discard, env(map[string]string{"ALDER_LOG_FORMAT": "yaml"}))
	if err == nil || err.Error() != "ALDER_LOG_FORMAT=yaml: want text or json" {
		t.Errorf("err = %v", err)
	}
}

// Each level's line under each minimum: written exactly when it is at or above it.
func TestLog_Filtered(t *testing.T) {
	for _, min := range []Level{Debug, Info, Warn, Error} {
		for _, f := range []Format{Text, JSON} {
			var b bytes.Buffer
			l := newAt(&b, min, f)
			for _, lv := range []Level{Debug, Info, Warn, Error} {
				b.Reset()
				l.Log(lv.String(), "m")
				if got, want := b.Len() > 0, lv >= min; got != want {
					t.Errorf("min %v format %v: a %v line written = %v, want %v (%q)", min, f, lv, got, want, b.String())
				}
				if l.Enabled(lv) != (lv >= min) {
					t.Errorf("Enabled(%v) under %v = %v", lv, min, l.Enabled(lv))
				}
			}
		}
	}
}

func TestLog_EffectIsDebug(t *testing.T) {
	var b bytes.Buffer
	newAt(&b, Info, Text).Log("effect", "Db.get Todo")
	if b.Len() != 0 {
		t.Errorf("an effect line under info: %q", b.String())
	}
	newAt(&b, Debug, Text).Log("effect", "Db.get Todo")
	if got := b.String(); got != "2026/10/29 20:00:00 debug: effect Db.get Todo\n" {
		t.Errorf("line %q", got)
	}
}

func TestLog_UnknownLevelKept(t *testing.T) {
	var b bytes.Buffer
	newAt(&b, Info, Text).Log("trace", "x happened")
	if got := b.String(); got != "2026/10/29 20:00:00 info: trace: x happened\n" {
		t.Errorf("line %q", got)
	}
	b.Reset()
	newAt(&b, Warn, Text).Log("trace", "x happened")
	if b.Len() != 0 {
		t.Errorf("an unknown level under warn: %q", b.String())
	}
}

// The text line is log.Printf's shape, stamped in UTC.
func TestText_Line(t *testing.T) {
	var b bytes.Buffer
	l := newAt(&b, Debug, Text)
	l.Log("info", "listening on :8080")
	l.Log("error", "GET /boom: panic: x\nstack line 1\nstack line 2\n")
	want := "2026/10/29 20:00:00 info: listening on :8080\n2026/10/29 20:00:00 error: GET /boom: panic: x\n\tstack line 1\n\tstack line 2\n"
	if b.String() != want {
		t.Errorf("lines\n%q\nwant\n%q", b.String(), want)
	}
	// the real clock: the stamp's shape
	b.Reset()
	New(&b, Info, Text).Log("warn", "w")
	if !regexp.MustCompile(`^\d{4}/\d\d/\d\d \d\d:\d\d:\d\d warn: w\n$`).MatchString(b.String()) {
		t.Errorf("line %q", b.String())
	}
}

var sample = Access{ID: "5f3a2c1e", Method: "GET", Path: "/start?key=••••&seed=7", Status: 200, Dur: 12_340 * time.Microsecond, Bytes: 4821, Steps: 4}

func TestText_Access(t *testing.T) {
	var b bytes.Buffer
	a := sample
	newAt(&b, Info, Text).Access(&a)
	if got, want := b.String(), "2026/10/29 20:00:00 info: GET /start?key=••••&seed=7 200 12.3ms 4821B rid=5f3a2c1e\n"; got != want {
		t.Errorf("info:\n%q\nwant\n%q", got, want)
	}
	b.Reset()
	newAt(&b, Debug, Text).Access(&a)
	if got, want := b.String(), "2026/10/29 20:00:00 info: GET /start?key=••••&seed=7 200 12.3ms 4821B rid=5f3a2c1e steps=4\n"; got != want {
		t.Errorf("debug:\n%q\nwant\n%q", got, want)
	}
	// ms keeps one decimal, rounded; a handler that wrote nothing is status 0
	b.Reset()
	newAt(&b, Info, Text).Access(&Access{ID: "a", Method: "POST", Path: "/x", Dur: 2 * time.Second, Bytes: 0})
	if got, want := b.String(), "2026/10/29 20:00:00 info: POST /x 0 2000.0ms 0B rid=a\n"; got != want {
		t.Errorf("got %q want %q", got, want)
	}
	b.Reset()
	newAt(&b, Info, Text).Access(&Access{ID: "a", Method: "GET", Path: "/", Status: 304, Dur: 50 * time.Microsecond})
	if got, want := b.String(), "2026/10/29 20:00:00 info: GET / 304 0.1ms 0B rid=a\n"; got != want {
		t.Errorf("got %q want %q", got, want)
	}
}

func TestJSON_Access(t *testing.T) {
	for _, min := range []Level{Info, Debug} {
		var b bytes.Buffer
		a := sample
		newAt(&b, min, JSON).Access(&a)
		if strings.Count(b.String(), "\n") != 1 {
			t.Fatalf("not one line: %q", b.String())
		}
		var got map[string]any
		dec := json.NewDecoder(&b)
		dec.UseNumber()
		if err := dec.Decode(&got); err != nil {
			t.Fatalf("%v: %q", err, b.String())
		}
		want := map[string]any{
			"time": "2026-10-29T20:00:00.123Z", "level": "INFO", "msg": "GET /start?key=••••&seed=7",
			"method": "GET", "path": "/start?key=••••&seed=7", "status": json.Number("200"), "ms": json.Number("12.3"),
			"bytes": json.Number("4821"), "rid": "5f3a2c1e",
		}
		if min == Debug {
			want["steps"] = json.Number("4")
		}
		if len(got) != len(want) {
			t.Errorf("%v: keys %v, want %v", min, got, want)
		}
		for k, v := range want {
			if got[k] != v {
				t.Errorf("%v: %s = %#v, want %#v", min, k, got[k], v)
			}
		}
	}
}

func TestJSON_StackInMsg(t *testing.T) {
	var b bytes.Buffer
	newAt(&b, Info, JSON).Log("error", "GET /boom: panic: x\ngoroutine 1 [running]:\n\tmain.go:1\n")
	if strings.Count(b.String(), "\n") != 1 {
		t.Fatalf("not one line: %q", b.String())
	}
	var got map[string]any
	if err := json.Unmarshal(b.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got["level"] != "ERROR" || got["msg"] != "GET /boom: panic: x\ngoroutine 1 [running]:\n\tmain.go:1\n" || got["time"] != "2026-10-29T20:00:00.123Z" || len(got) != 3 {
		t.Errorf("line %v", got)
	}
}

func TestAccess_QuietIsDebug(t *testing.T) {
	for _, f := range []Format{Text, JSON} {
		var b bytes.Buffer
		a := sample
		a.Quiet = true
		newAt(&b, Info, f).Access(&a)
		if b.Len() != 0 {
			t.Errorf("%v: a quiet line under info: %q", f, b.String())
		}
		newAt(&b, Debug, f).Access(&a)
		if !strings.Contains(strings.ToLower(b.String()), "debug") {
			t.Errorf("%v: a quiet line under debug: %q", f, b.String())
		}
	}
}

// A line nobody sees allocates nothing: a Debug access line and a Debug log line under Info.
func TestLogger_FilteredAllocs(t *testing.T) {
	for _, f := range []Format{Text, JSON} {
		l := New(io.Discard, Info, f)
		a := sample
		a.Quiet = true
		if n := testing.AllocsPerRun(1000, func() { l.Access(&a) }); n != 0 {
			t.Errorf("%v: a filtered access line: %v allocations", f, n)
		}
		if n := testing.AllocsPerRun(1000, func() { l.Log("effect", "Db.get Todo") }); n != 0 {
			t.Errorf("%v: a filtered effect line: %v allocations", f, n)
		}
		if n := testing.AllocsPerRun(1000, func() { l.Log("debug", "x") }); n != 0 {
			t.Errorf("%v: a filtered debug line: %v allocations", f, n)
		}
	}
}

// Lines written at once from many goroutines come out whole, one per Write.
func TestLogger_ConcurrentLinesWhole(t *testing.T) {
	for _, f := range []Format{Text, JSON} {
		w := &lineWriter{}
		l := New(w, Info, f)
		var wg sync.WaitGroup
		for i := 0; i < 50; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for j := 0; j < 20; j++ {
					a := sample
					l.Access(&a)
					l.Log("warn", "a warning")
				}
			}()
		}
		wg.Wait()
		if len(w.writes) != 2000 {
			t.Fatalf("%v: %d writes, want 2000", f, len(w.writes))
		}
		for _, s := range w.writes {
			if strings.Count(s, "\n") != 1 || !strings.HasSuffix(s, "\n") {
				t.Fatalf("%v: a write is not one line: %q", f, s)
			}
		}
	}
}

type lineWriter struct {
	mu     sync.Mutex
	writes []string
}

func (w *lineWriter) Write(b []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.writes = append(w.writes, string(b))
	return len(b), nil
}

func TestNewID_UniqueAndHex(t *testing.T) {
	hex8 := regexp.MustCompile(`^[0-9a-f]{8}$`)
	seen := map[string]bool{}
	for i := 0; i < 10000; i++ {
		id := NewID()
		if !hex8.MatchString(id) {
			t.Fatalf("NewID() = %q", id)
		}
		if seen[id] {
			t.Fatalf("NewID() repeated %q", id)
		}
		seen[id] = true
	}
}

func TestRequestID(t *testing.T) {
	for _, c := range []struct {
		header string
		kept   bool
	}{
		{"abc-1", true},
		{"01HF.fly_req-9", true},
		{strings.Repeat("a", 64), true},
		{strings.Repeat("a", 65), false},
		{"has space", false},
		{"a/b", false},
		{"<script>", false},
		{"rid=x", false},
		{"é", false},
		{"", false},
	} {
		r := httptest.NewRequest("GET", "/", nil)
		if c.header != "" {
			r.Header.Set(IDHeader, c.header)
		}
		got := RequestID(r)
		if c.kept && got != c.header {
			t.Errorf("%q: id %q, want it kept", c.header, got)
		}
		if !c.kept && (got == c.header || !regexp.MustCompile(`^[0-9a-f]{8}$`).MatchString(got)) {
			t.Errorf("%q: id %q, want a new one", c.header, got)
		}
	}
	if got := RequestID(httptest.NewRequest("GET", "/", nil)); len(got) != 8 {
		t.Errorf("no header: %q", got)
	}
}

func BenchmarkAccess_Text(b *testing.B) {
	l := New(io.Discard, Info, Text)
	a := sample
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		l.Access(&a)
	}
}

func BenchmarkAccess_JSON(b *testing.B) {
	l := New(io.Discard, Info, JSON)
	a := sample
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		l.Access(&a)
	}
}
