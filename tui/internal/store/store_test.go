package store

import (
	"bufio"
	"os"
	"regexp"
	"strings"
	"testing"
	"time"

	"alder/tui/internal/feed"
)

// fixture decodes testdata/feeds/basic.ndjson.
func fixture(t *testing.T) []feed.Event {
	t.Helper()
	f, err := os.Open("../../testdata/feeds/basic.ndjson")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	var evs []feed.Event
	sc := bufio.NewScanner(f)
	sc.Buffer(nil, 1<<20)
	for sc.Scan() {
		e, err := feed.Decode(sc.Bytes())
		if err != nil {
			t.Fatal(err)
		}
		evs = append(evs, e)
	}
	return evs
}

func texts(ls []Line) []string {
	var out []string
	for _, l := range ls {
		out = append(out, l.Text)
	}
	return out
}

func TestLogs_FiltersStack(t *testing.T) {
	lg := NewLogs(100)
	for _, e := range fixture(t) {
		lg.Add(e)
	}
	all := lg.View(Filter{})
	info := lg.View(Filter{Level: "info"})
	if len(info) != len(all)-2 {
		// the effect line and the debug static access line are below info
		t.Fatalf("info hides %d lines, want 2: %q", len(all)-len(info), texts(info))
	}
	for _, l := range info {
		if l.Effect {
			t.Errorf("effect line shown at info: %q", l.Text)
		}
	}
	rid := lg.View(Filter{Rid: "5f3a2c1f"})
	if len(rid) != 4 { // the access line and three Log lines (one is debug)
		t.Fatalf("rid lines: %q", texts(rid))
	}
	both := lg.View(Filter{Level: "info", Rid: "5f3a2c1f"})
	if len(both) != 3 {
		t.Fatalf("rid at info: %q", texts(both))
	}
	got := lg.View(Filter{Level: "info", Re: regexp.MustCompile("lend")})
	if len(got) != 1 || !got[0].Access {
		t.Fatalf("regexp lend: %q", texts(got))
	}
	raw := lg.View(Filter{Level: "info", Re: regexp.MustCompile("^panic")})
	if len(raw) != 1 || raw[0].Level != "info" {
		t.Fatalf("raw line: %+v", raw)
	}
}

func TestLogs_Cap(t *testing.T) {
	lg := NewLogs(3)
	for i := int64(1); i <= 5; i++ {
		lg.Add(feed.Event{Type: "run", Seq: i, Run: &feed.Run{Text: "r"}})
	}
	v := lg.View(Filter{})
	if len(v) != 3 || v[0].Seq != 3 || v[2].Seq != 5 {
		t.Fatalf("view: %+v", v)
	}
}

func rec(level, msg string) *feed.Record {
	return &feed.Record{Time: time.Date(2026, 11, 1, 14, 2, 11, 204000000, time.UTC), Level: level, Msg: msg}
}

func TestLogs_TextMatchesAlderRun(t *testing.T) {
	access := func(r feed.Record) feed.Event {
		r.Time = time.Date(2026, 11, 1, 14, 2, 11, 204000000, time.UTC)
		r.HasAccess = true
		return feed.Event{Type: "log", Seq: 1, Log: &feed.Log{Record: &r}}
	}
	cases := []struct {
		name        string
		ev          feed.Event
		info, debug string
	}{
		{"access", access(feed.Record{Level: "INFO", Rid: "5f3a2c1e", Method: "GET", Path: "/start?key=••••", Status: 200, Ms: 12.3, Bytes: 4821, Steps: 3}),
			"2026/11/01 14:02:11 info: GET /start?key=•••• 200 12.3ms 4821B rid=5f3a2c1e",
			"2026/11/01 14:02:11 info: GET /start?key=•••• 200 12.3ms 4821B rid=5f3a2c1e steps=3"},
		{"slow", access(feed.Record{Level: "WARN", Rid: "b2", Method: "GET", Path: "/todos/3", Status: 200, Ms: 812.4, Bytes: 4821, Steps: 12, Slow: true, Route: "/todos/{id}", DBStmts: 9, DBMs: 640.1}),
			"2026/11/01 14:02:11 warn: GET /todos/3 200 812.4ms 4821B rid=b2 slow route=/todos/{id} steps=12 db=9/640.1ms",
			"2026/11/01 14:02:11 warn: GET /todos/3 200 812.4ms 4821B rid=b2 slow route=/todos/{id} steps=12 db=9/640.1ms"},
		{"info", feed.Event{Type: "log", Seq: 1, Log: &feed.Log{Record: rec("INFO", "hello")}},
			"2026/11/01 14:02:11 info: hello", "2026/11/01 14:02:11 info: hello"},
		{"continuation", feed.Event{Type: "log", Seq: 1, Log: &feed.Log{Record: rec("WARN", "careful\nsecond line")}},
			"2026/11/01 14:02:11 warn: careful\n\tsecond line", "2026/11/01 14:02:11 warn: careful\n\tsecond line"},
		{"control", feed.Event{Type: "log", Seq: 1, Log: &feed.Log{Record: rec("ERROR", "bad \x1b[31m")}},
			`2026/11/01 14:02:11 error: bad \x1b[31m`, `2026/11/01 14:02:11 error: bad \x1b[31m`},
		{"effect", feed.Event{Type: "log", Seq: 1, Log: &feed.Log{Record: rec("DEBUG", "effect Db.all Todo")}},
			"", "2026/11/01 14:02:11 debug: effect Db.all Todo"},
	}
	for _, c := range cases {
		lg := NewLogs(10)
		lg.Add(c.ev)
		for _, lv := range []struct{ level, want string }{{"info", c.info}, {"debug", c.debug}} {
			v := lg.View(Filter{Level: lv.level})
			if lv.want == "" {
				if len(v) != 0 {
					t.Errorf("%s at %s: want hidden, got %q", c.name, lv.level, texts(v))
				}
				continue
			}
			if len(v) != 1 || v[0].Text != lv.want {
				t.Errorf("%s at %s:\n got %q\nwant %q", c.name, lv.level, texts(v), lv.want)
			}
		}
	}
}

func TestRequests_LeavesOutQuiet(t *testing.T) {
	rq := NewRequests(5000)
	for _, e := range fixture(t) {
		rq.Add(e)
	}
	all := rq.All()
	if len(all) != 5 {
		t.Fatalf("%d requests, want 5", len(all))
	}
	if r, ok := rq.ByRid("5f3a2c20"); !ok || r.Path != "/books/42/lend" {
		t.Errorf("ByRid: %+v %v", r, ok)
	}
	if _, ok := rq.ByRid("5f3a2c22"); ok {
		t.Error("the quiet static request is in the table")
	}
	if all[0].Seq > all[len(all)-1].Seq {
		t.Error("not oldest first")
	}
}

func TestStats(t *testing.T) {
	now := time.Date(2026, 11, 1, 14, 3, 0, 0, time.UTC)
	mk := func(ago time.Duration, ms float64, status int) Request {
		return Request{Record: feed.Record{Time: now.Add(-ago), Ms: ms, Status: status}}
	}
	rs := []Request{mk(90*time.Second, 999, 200), mk(50*time.Second, 10, 200), mk(40*time.Second, 20, 200),
		mk(10*time.Second, 30, 500), mk(0, 40, 200)}
	s := Stats(rs, now)
	if s.ReqPerSec != 4.0/60 || s.P50 != 20 || s.P95 != 40 || s.ErrPct != 25 {
		t.Errorf("strip: %+v", s)
	}
	if z := Stats(nil, now); z != (Strip{}) {
		t.Errorf("empty: %+v", z)
	}
}

func TestReset(t *testing.T) {
	lg, rq := NewLogs(10), NewRequests(10)
	for _, e := range fixture(t) {
		lg.Add(e)
		rq.Add(e)
	}
	lg.Reset()
	rq.Reset()
	if len(lg.View(Filter{})) != 0 || len(rq.All()) != 0 {
		t.Error("not empty after Reset")
	}
	if _, ok := rq.ByRid("5f3a2c1e"); ok {
		t.Error("ByRid finds a request after Reset")
	}
}

func TestRequests_At(t *testing.T) {
	rq := NewRequests(3)
	for _, e := range fixture(t) {
		rq.Add(e)
	}
	if rq.Len() != 3 {
		t.Fatalf("Len = %d, want 3", rq.Len())
	}
	all := rq.All()
	for i := range rq.Len() {
		if rq.At(i).Rid != all[i].Rid {
			t.Errorf("At(%d) = %s, want %s", i, rq.At(i).Rid, all[i].Rid)
		}
	}
}

func TestLogs_Positions(t *testing.T) {
	lg := NewLogs(3)
	for i := int64(1); i <= 5; i++ {
		lg.Add(feed.Event{Type: "run", Seq: i, Run: &feed.Run{Text: "r"}})
	}
	if lg.Start() != 2 || lg.End() != 5 {
		t.Fatalf("Start, End = %d, %d, want 2, 5", lg.Start(), lg.End())
	}
	if l, ok := lg.Get(2, Filter{}); !ok || l.Seq != 3 {
		t.Errorf("Get(2) = %+v, %v, want the line of seq 3", l, ok)
	}
	if _, ok := lg.Get(4, Filter{Level: "warn"}); ok {
		t.Error("Get passes an info line through a warn filter")
	}
	lg.Reset()
	if lg.Start() != 5 || lg.End() != 5 {
		t.Errorf("after Reset Start, End = %d, %d, want 5, 5", lg.Start(), lg.End())
	}
	lg.Add(feed.Event{Type: "run", Seq: 1, Run: &feed.Run{Text: "again"}})
	if l, ok := lg.Get(5, Filter{}); !ok || l.Text != "again" {
		t.Errorf("Get(5) after Reset = %+v, %v", l, ok)
	}
	// An access line read at debug carries its steps, as in View.
	lg = NewLogs(10)
	for _, e := range fixture(t) {
		lg.Add(e)
	}
	for i := lg.Start(); i < lg.End(); i++ {
		if l, ok := lg.Get(i, Filter{Level: "debug", Re: regexp.MustCompile("lend")}); ok && l.Access {
			if want := " steps=3"; len(l.Text) < len(want) || l.Text[len(l.Text)-len(want):] != want {
				t.Errorf("the access line at debug = %q, want it to end with %q", l.Text, want)
			}
			return
		}
	}
	t.Error("no access line matched lend at debug")
}

// A gap in the feed is one dim marker line with the count of the events missed; every level
// filter shows it.
func TestLogs_GapMarker(t *testing.T) {
	lg := NewLogs(10)
	lg.Add(feed.Event{Type: "run", Seq: 3, Run: &feed.Run{Text: "before"}})
	lg.Add(feed.Event{Type: "gap", T: time.Unix(0, 0), Gap: &feed.Gap{Missed: 4}})
	ls := lg.View(Filter{})
	if len(ls) != 2 {
		t.Fatalf("lines = %q, want 2", texts(ls))
	}
	if g := ls[1]; !g.Gap || g.Text != "… 4 events missed (ring overflow)" {
		t.Errorf("marker = %+v", g)
	}
	if ls := lg.View(Filter{Level: "error"}); len(ls) != 1 || !ls[0].Gap {
		t.Errorf("at error = %q, want the marker alone", texts(ls))
	}
}

// The rid, the regexp and the debug rendering hold together: only the request's own access line
// passes, with its steps.
func TestLogs_RidRegexDebug(t *testing.T) {
	lg := NewLogs(50)
	for _, e := range fixture(t) {
		lg.Add(e)
	}
	f := Filter{Level: "debug", Rid: "5f3a2c1f", Re: regexp.MustCompile(`steps=\d+`)}
	ls := lg.View(f)
	if len(ls) != 1 || !ls[0].Access || ls[0].Rid != "5f3a2c1f" {
		t.Fatalf("lines = %q, want the one access line of 5f3a2c1f", texts(ls))
	}
	if !strings.Contains(ls[0].Text, " steps=") {
		t.Errorf("the line has no steps at debug: %q", ls[0].Text)
	}
	f.Rid = "5f3a2c20"
	for _, l := range lg.View(f) {
		if l.Rid != "5f3a2c20" || !strings.Contains(l.Text, "steps=") {
			t.Errorf("line %q does not hold the rid and the regexp", l.Text)
		}
	}
}

// recLine is a log event of one request's line at level lv.
func recLine(seq int64, lv, rid string) feed.Event {
	return feed.Event{Type: "log", Seq: seq, Log: &feed.Log{Record: &feed.Record{Level: lv, Rid: rid, Msg: "m"}}}
}

func addAll(lg *Logs, evs ...feed.Event) {
	for _, e := range evs {
		lg.Add(e)
	}
}

func TestHiddenNewer_Counts(t *testing.T) {
	lg := NewLogs(50)
	addAll(lg, recLine(1, "INFO", "a"), recLine(2, "INFO", "b"), recLine(3, "WARN", "b"), recLine(4, "INFO", "c"))
	if n := lg.HiddenNewer(Filter{Level: "info", Rid: "a"}); n != 3 {
		t.Errorf("HiddenNewer = %d, want 3", n)
	}
	// With no line of the rid shown, every line the rid alone hides counts.
	if n := lg.HiddenNewer(Filter{Level: "info", Rid: "zz"}); n != 4 {
		t.Errorf("HiddenNewer with no match = %d, want 4", n)
	}
}

func TestHiddenNewer_OlderNotCounted(t *testing.T) {
	lg := NewLogs(50)
	addAll(lg, recLine(1, "INFO", "b"), recLine(2, "INFO", "c"), recLine(3, "INFO", "a"))
	if n := lg.HiddenNewer(Filter{Level: "info", Rid: "a"}); n != 0 {
		t.Errorf("HiddenNewer = %d, want 0", n)
	}
}

func TestHiddenNewer_LevelExcluded(t *testing.T) {
	lg := NewLogs(50)
	addAll(lg, recLine(1, "INFO", "a"), recLine(2, "DEBUG", "b"))
	if n := lg.HiddenNewer(Filter{Level: "info", Rid: "a"}); n != 0 {
		t.Errorf("HiddenNewer = %d, want 0", n)
	}
}

func TestHiddenNewer_NoRid(t *testing.T) {
	lg := NewLogs(50)
	addAll(lg, recLine(1, "INFO", "a"), recLine(2, "INFO", "b"))
	if n := lg.HiddenNewer(Filter{Level: "info"}); n != 0 {
		t.Errorf("HiddenNewer = %d, want 0", n)
	}
}
