package ui

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/exp/teatest"

	"alder/tui/internal/feed"
)

// rowOf is the line of frame that holds s, or "".
func rowOf(frame, s string) string {
	for _, l := range strings.Split(frame, "\n") {
		if strings.Contains(l, s) {
			return l
		}
	}
	return ""
}

// cursorRow is the table row under the cursor in frame, or "".
func cursorRow(frame string) string { return rowOf(frame, " > ") }

func TestRequests_Table(t *testing.T) {
	m := newRoot(options(nil), fixture(t, "basic")...)
	frame := m.View()
	lines := strings.Split(frame, "\n")
	if want := " last 60s  0.1 req/s  p50 6ms  p95 812ms  errors 20.0%"; lines[1] != want {
		t.Errorf("strip = %q, want %q", lines[1], want)
	}
	if !strings.HasPrefix(lines[2], " ─") {
		t.Errorf("no rule under the strip: %q", lines[2])
	}
	n := 0
	for _, l := range lines[3:] {
		if strings.Contains(l, "14:02:") {
			n++
		}
	}
	if n != 5 {
		t.Errorf("%d rows, want 5:\n%s", n, frame)
	}
	search := rowOf(frame, "/search")
	for _, want := range []string{"\x1b[1;31m500\x1b[0m", "\x1b[33m812.4ms\x1b[0m", "slow", " > "} {
		if !strings.Contains(search, want) {
			t.Errorf("the /search row has no %q: %q", want, search)
		}
	}
	if !strings.Contains(rowOf(frame, "/books/42 "), "n+1") {
		t.Errorf("the /books/42 row has no n+1: %q", rowOf(frame, "/books/42 "))
	}
	if !strings.Contains(rowOf(frame, "/books/42/lend"), "\x1b[2m303\x1b[0m") {
		t.Errorf("the 303 is not dim: %q", rowOf(frame, "/books/42/lend"))
	}
	if !strings.Contains(rowOf(frame, "14:02:11.204"), "4.7K") {
		t.Errorf("the / row has no 4.7K: %q", rowOf(frame, "14:02:11.204"))
	}
	teatest.RequireEqualOutput(t, []byte(frame))
}

func TestRequests_Filter(t *testing.T) {
	m := newRoot(options(nil), fixture(t, "basic")...)
	press(m, "/", "l", "e", "n", "d")
	if m.tab != 0 {
		t.Fatalf("typing the filter switched to tab %d", m.tab+1)
	}
	press(m, "enter")
	frame := m.View()
	n := 0
	for _, l := range strings.Split(frame, "\n") {
		if strings.Contains(l, "14:02:") {
			n++
		}
	}
	if n != 1 || rowOf(frame, "/books/42/lend") == "" {
		t.Errorf("%d rows after /lend, want the lend row only:\n%s", n, frame)
	}
	teatest.RequireEqualOutput(t, []byte(frame))
}

// access is a log event carrying the access record of a request to path at time at.
func access(seq int64, at time.Time, rid, path string) feed.Event {
	return feed.Event{Type: "log", Seq: seq, T: at, Log: &feed.Log{Record: &feed.Record{
		Time: at, Level: "INFO", Msg: "GET " + path, Method: "GET", Path: path, Status: 200, Ms: 2.5,
		Bytes: 300, Rid: rid, Steps: 1, HasAccess: true}}}
}

func TestRequests_Follow(t *testing.T) {
	m := newRoot(options(nil), fixture(t, "basic")...)
	if c := cursorRow(m.View()); !strings.Contains(c, "/search") {
		t.Fatalf("the cursor does not follow the newest row: %q", c)
	}
	press(m, "k")
	m.Update(eventMsg(access(14, clock, "5f3a2c24", "/authors")))
	if c := cursorRow(m.View()); !strings.Contains(c, "/books/999") {
		t.Errorf("after k and a new row the cursor is on %q, want /books/999", c)
	}
	press(m, "G")
	frame := m.View()
	if c := cursorRow(frame); !strings.Contains(c, "/authors") {
		t.Errorf("after G the cursor is on %q, want /authors", c)
	}
	m.Update(eventMsg(access(15, clock, "5f3a2c25", "/about")))
	if c := cursorRow(m.View()); !strings.Contains(c, "/about") {
		t.Errorf("G does not keep following: the cursor is on %q", c)
	}
	press(m, "k", "j")
	m.Update(eventMsg(access(16, clock, "5f3a2c26", "/contact")))
	if c := cursorRow(m.View()); !strings.Contains(c, "/contact") {
		t.Errorf("j back onto the newest row does not follow: the cursor is on %q", c)
	}
	teatest.RequireEqualOutput(t, []byte(frame))
}

// books42 is the detail the fake dev server answers for /books/42: five steps, a select of the
// book, nine selects of its loans (one N+1 group), a count, and two lines logged.
func books42() reqDetail {
	at := time.Date(2026, 11, 1, 14, 2, 12, 516000000, time.UTC)
	d := reqDetail{RID: "5f3a2c1f", T: at, Method: "GET", Path: "/books/42", Route: "/books/{id}", Status: 200,
		Ms: 12.8, Bytes: 6377,
		Steps: []reqStep{{"load", 0.4, "db"}, {"Books.find", 0.9, "db"}, {"Loans.for", 8.1, "db"},
			{"Reviews.count", 0.6, "db"}, {"render", 2.4, ""}},
		Logs: []reqLog{
			{T: at.Add(1210 * time.Microsecond), Level: "INFO", Msg: "showing book 42"},
			{T: at.Add(7861 * time.Microsecond), Level: "WARN", Msg: "book 42 has no cover image"}},
	}
	d.SQL = append(d.SQL, reqSQL{Stmt: "select * from books where id=?", Args: []string{"42"}, Ms: 0.9, Rows: 1})
	for i := range 9 {
		d.SQL = append(d.SQL, reqSQL{Stmt: "select * from loans where book=?", Args: []string{"42"}, Ms: 0.9,
			Rows: 1, Group: i + 1, Repeats: 9})
	}
	d.SQL = append(d.SQL, reqSQL{Stmt: "select count(*) from reviews where book=?", Args: []string{"42"}, Ms: 0.6, Rows: 1})
	return d
}

// devServer is a fake dev server answering GET /_dev/requests/<rid> with the details it holds,
// 404 for any other rid; it records the paths it was asked for. The options keep the address
// localhost:8080, so the frames are the same on every run, and their client dials the fake.
func devServer(t *testing.T, details ...reqDetail) (Options, *[]string) {
	t.Helper()
	var asked []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		asked = append(asked, r.Method+" "+r.URL.Path)
		for _, d := range details {
			if r.URL.Path == "/_dev/requests/"+d.RID {
				json.NewEncoder(w).Encode(d)
				return
			}
		}
		http.NotFound(w, r)
	}))
	t.Cleanup(srv.Close)
	o := options(nil)
	o.HTTP = &http.Client{Transport: rewriter{addr: srv.Listener.Addr().String(), next: http.DefaultTransport}}
	return o, &asked
}

// openBooks42 is the root model of o with the basic feed, the cursor on /books/42 and its detail
// opened.
func openBooks42(t *testing.T, o Options) *root {
	t.Helper()
	m := newRoot(o, fixture(t, "basic")...)
	press(m, "k", "k", "k", "enter")
	return m
}

func TestRequest_Detail(t *testing.T) {
	o, asked := devServer(t, books42())
	m := openBooks42(t, o)
	frame := m.View()
	if strings.Join(*asked, "|") != "GET /_dev/requests/5f3a2c1f" {
		t.Errorf("the dev server was asked %q", *asked)
	}
	for _, want := range []string{
		"← GET /books/42  200  12.8ms  rid 5f3a2c1f  route /books/{id}  steps 5  db 11/9.6ms",
		"\x1b[1mSteps\x1b[0m",
		"\x1b[1mSQL (11, 1 repeat group)\x1b[0m",
		"\x1b[1mLogs (2)\x1b[0m",
		"select * from loa…",
		"info: showing book 42",
		"\x1b[33mwarn\x1b[0m: book 42 has no",
	} {
		if !strings.Contains(frame, want) {
			t.Errorf("the detail has no %q:\n%s", want, frame)
		}
	}
	if !strings.Contains(m.footer(), "curl localhost:8080/_dev/requests/5f3a2c1f") {
		t.Errorf("footer = %q", m.footer())
	}
	if strings.Count(frame, "×9 \x1b[33mn+1\x1b[0m") != 1 || strings.Count(frame, "×1") != 2 {
		t.Errorf("the nine loans selects are not folded into one row:\n%s", frame)
	}
	teatest.RequireEqualOutput(t, []byte(frame))
}

// rewriter sends every request to the fake dev server at addr, whatever its host.
type rewriter struct {
	addr string
	next http.RoundTripper
}

func (r rewriter) RoundTrip(req *http.Request) (*http.Response, error) {
	req = req.Clone(req.Context())
	req.URL.Host = r.addr
	return r.next.RoundTrip(req)
}

func TestRequest_Unfold(t *testing.T) {
	o, _ := devServer(t, books42())
	m := openBooks42(t, o)
	press(m, "tab", "j", "enter")
	frame := m.View()
	if m.tab != 0 {
		t.Fatalf("tab in the detail switched to tab %d", m.tab+1)
	}
	if !strings.Contains(frame, "args [42] [42] [42]") {
		t.Errorf("the group's args are not listed:\n%s", frame)
	}
	if !strings.Contains(frame, "▾") {
		t.Errorf("the group is not marked unfolded:\n%s", frame)
	}
	press(m, "enter")
	if strings.Contains(m.View(), "args [42] [42]") {
		t.Errorf("enter again does not fold the group:\n%s", m.View())
	}
	press(m, "esc")
	if !strings.Contains(m.View(), "last 60s") {
		t.Errorf("esc does not return to the table:\n%s", m.View())
	}
	teatest.RequireEqualOutput(t, []byte(frame))
}

func TestRequest_Narrow(t *testing.T) {
	o, _ := devServer(t, books42())
	m := openBooks42(t, o)
	m.Update(tea.WindowSizeMsg{Width: 79, Height: 30})
	frame := m.View()
	lines := strings.Split(frame, "\n")
	steps, sql, logs := -1, -1, -1
	for i, l := range lines {
		switch {
		case strings.Contains(l, "Steps"):
			steps = i
		case strings.Contains(l, "SQL ("):
			sql = i
		case strings.Contains(l, "Logs ("):
			logs = i
		}
	}
	if !(steps >= 0 && steps < sql && sql < logs) {
		t.Errorf("the sections are not stacked (Steps %d, SQL %d, Logs %d):\n%s", steps, sql, logs, frame)
	}
	for _, want := range []string{"select * from loans where book=?", "select count(*) from reviews where book=?",
		"\x1b[33mwarn\x1b[0m: book 42 has no cover image"} {
		if !strings.Contains(frame, want) {
			t.Errorf("the stacked detail has no %q in full:\n%s", want, frame)
		}
	}
	teatest.RequireEqualOutput(t, []byte(frame))
}

func TestRequest_Gone(t *testing.T) {
	o, _ := devServer(t)
	m := openBooks42(t, o)
	frame := m.View()
	for _, want := range []string{
		"← GET /books/42  200  12.8ms  rid 5f3a2c1f",
		"the dev server no longer has this request (the last 200 are kept, and a restart clears them)",
	} {
		if !strings.Contains(frame, want) {
			t.Errorf("no %q in\n%s", want, frame)
		}
	}
	teatest.RequireEqualOutput(t, []byte(frame))
}

func TestRequest_CopyRid(t *testing.T) {
	var copied []string
	o, _ := devServer(t, books42())
	o.Copy = func(s string) error { copied = append(copied, s); return nil }
	m := openBooks42(t, o)
	press(m, "y")
	if strings.Join(copied, "|") != "5f3a2c1f" {
		t.Errorf("copied %q", copied)
	}
}

func TestRequests_LogsJump(t *testing.T) {
	m := newRoot(options(nil), fixture(t, "basic")...)
	press(m, "k", "k", "k", "l")
	if m.tab != 1 {
		t.Fatalf("l switched to tab %d, want 2 Logs", m.tab+1)
	}
	frame := m.View()
	if !strings.Contains(strings.Split(frame, "\n")[1], "rid 5f3a2c1f") {
		t.Errorf("the Logs strip does not show the rid filter:\n%s", frame)
	}
	teatest.RequireEqualOutput(t, []byte(frame))
}
