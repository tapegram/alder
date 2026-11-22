package ui

import (
	"fmt"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/exp/teatest"

	"alder/tui/internal/feed"
)

// shelfJobs is /_dev/jobs.json of the shelf app at the fixed clock: a schedule with its tick
// queued in 38 minutes, a schedule with none, two queued rows (one is the tick), one running, one
// dead (in a silo, with a two-line error) and one done.
func shelfJobs() string {
	at := func(d time.Duration) int64 { return clock.Add(d).UnixMilli() }
	return fmt.Sprintf(`{"workers":true,"schedules":[`+
		`{"kind":"Reports.Nightly","rule":"every 1h","next":%d,"id":"t1"},`+
		`{"kind":"Cleanup.Sweep","rule":"cron \"0 3 * * *\"","next":null}],`+
		`"queued":[`+
		`{"id":"q1","kind":"Signup.SendWelcome","state":"queued","params":"{}","runAt":%d,"created":1,"attempts":0},`+
		`{"id":"t1","kind":"Reports.Nightly","state":"queued","params":"{}","runAt":%d,"created":1,"attempts":0,"tick":true}],`+
		`"running":[{"id":"r1","kind":"Signup.SendWelcome","state":"running","params":"{}","runAt":1,"created":1,"attempts":1,"worker":"w1"}],`+
		`"dead":[{"id":"d1","kind":"Signup.SendWelcome","silo":"org/42","state":"dead","params":"{}","runAt":1,"created":1,"attempts":5,"error":"smtp: timeout\nsecond line"}],`+
		`"done":[{"id":"x1","kind":"Reports.Nightly","state":"done","params":"{}","runAt":1,"created":1,"attempts":1}],`+
		`"missed":[]}`, at(38*time.Minute), at(-5*time.Second), at(38*time.Minute))
}

// jobsServer is a dev server's jobs endpoints for the tests: it answers the listing from body and
// records each GET of it and each POST (its path).
type jobsServer struct {
	mu    sync.Mutex
	body  string
	gets  int
	posts []string
	code  int // the status of the POSTs; 0 is 303
	text  string
}

func (j *jobsServer) client() *http.Client {
	return &http.Client{Transport: handlerTransport(func(w http.ResponseWriter, r *http.Request) {
		j.mu.Lock()
		defer j.mu.Unlock()
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/_dev/jobs.json":
			j.gets++
			_, _ = w.Write([]byte(j.body))
		case r.Method == http.MethodPost && strings.HasPrefix(r.URL.Path, "/_dev/jobs/"):
			j.posts = append(j.posts, r.URL.Path)
			if j.code != 0 {
				http.Error(w, j.text, j.code)
				return
			}
			http.Redirect(w, r, strings.TrimSuffix(strings.TrimSuffix(r.URL.Path, "/retry"), "/run"), http.StatusSeeOther)
		default:
			http.NotFound(w, r)
		}
	})}
}

func (j *jobsServer) counts() (int, []string) {
	j.mu.Lock()
	defer j.mu.Unlock()
	return j.gets, append([]string{}, j.posts...)
}

// jobsRoot is a root model whose dev server is srv, with the Jobs screen's clock stopped: the
// ticks it asks for are collected in ticks.
func jobsRoot(srv *jobsServer, evs ...feed.Event) (*root, *[]tea.Msg) {
	o := options(nil)
	o.HTTP = srv.client()
	return jobsRootOf(o, evs...)
}

// jobsRootOf is jobsRoot for the options o.
func jobsRootOf(o Options, evs ...feed.Event) (*root, *[]tea.Msg) {
	m := newRoot(o, evs...)
	var ticks []tea.Msg
	m.screens[tabJobs].(*jobsScreen).after = func(_ time.Duration, msg tea.Msg) tea.Cmd {
		ticks = append(ticks, msg)
		return nil
	}
	return m, &ticks
}

func jobLine(seq int64, level, outcome string) feed.Event {
	return feed.Event{Type: "log", Seq: seq, T: clock, Log: &feed.Log{Record: &feed.Record{
		Time: clock, Level: level, Rid: "d1", Job: "Signup.SendWelcome", Outcome: outcome, Attempt: 1, Ms: 12.5, HasJob: true,
		Msg: "job Signup.SendWelcome d1 " + outcome + " 12ms attempt 1"}}}
}

func TestJobs_Screen(t *testing.T) {
	srv := &jobsServer{body: shelfJobs()}
	m, _ := jobsRoot(srv, fixture(t, "basic")...)
	runAll(m, key("3"))
	if m.tab != tabJobs {
		t.Fatalf("3 went to tab %d", m.tab+1)
	}
	frame := m.View()
	lines := strings.Split(frame, "\n")
	if want := " 2 queued  1 running  \x1b[31m1 dead\x1b[0m  1 done  workers on"; lines[1] != want {
		t.Errorf("summary = %q, want %q", lines[1], want)
	}
	for _, want := range []string{
		"Reports.Nightly", "every 1h", "next in 38m",
		"Cleanup.Sweep", "no tick queued",
		"Signup.SendWelcome  q1", "due now",
		"Signup.SendWelcome", "r1", "attempts 1",
		"[org/42]", "d1", "attempts 5", "smtp: timeout",
	} {
		if !strings.Contains(frame, want) {
			t.Errorf("no %q in\n%s", want, frame)
		}
	}
	if strings.Contains(frame, "second line") {
		t.Errorf("more than the error's first line:\n%s", frame)
	}
	if !strings.HasSuffix(lines[len(lines)-1], "\x1b[2malder jobs\x1b[0m") {
		t.Errorf("footer = %q", lines[len(lines)-1])
	}
	for i, l := range lines {
		if w := ansi.StringWidth(l); w > 100 {
			t.Errorf("line %d is %d wide", i, w)
		}
	}
	teatest.RequireEqualOutput(t, []byte(frame))
}

func TestJobs_Empty(t *testing.T) {
	srv := &jobsServer{body: `{"workers":false,"schedules":[],"queued":[],"running":[],"dead":[],"done":[],"missed":["silo org/7: unreadable"]}`}
	m, _ := jobsRoot(srv)
	runAll(m, key("3"))
	frame := m.View()
	for _, want := range []string{"0 queued", "\x1b[33mworkers off\x1b[0m", "no jobs yet", "\x1b[33msilo org/7: unreadable\x1b[0m"} {
		if !strings.Contains(frame, want) {
			t.Errorf("no %q in\n%s", want, frame)
		}
	}
}

// With no dev server, or none with a Jobs page, the screen says why in yellow and keeps what it
// last read.
func TestJobs_Errors(t *testing.T) {
	m, _ := jobsRootOf(options(nil))
	runAll(m, key("3"))
	if f := m.View(); !strings.Contains(f, "\x1b[33m") || !strings.Contains(f, "no dev server in the tests") {
		t.Errorf("offline:\n%s", f)
	}
	srv := &jobsServer{body: shelfJobs()}
	m, _ = jobsRoot(srv)
	runAll(m, key("3"))
	srv.mu.Lock()
	srv.body = "<html>"
	srv.mu.Unlock()
	runAll(m, key("tab"))
	runAll(m, key("shift+tab"))
	f := m.View()
	if !strings.Contains(f, "Reports.Nightly") || !strings.Contains(f, "reading the jobs") {
		t.Errorf("a bad answer dropped the last listing or hid the error:\n%s", f)
	}
}

// A job's line on the feed refreshes the screen while it is shown; an access line, and a job line
// while another tab is shown, do not.
func TestJobs_RefreshOnJobEvent(t *testing.T) {
	srv := &jobsServer{body: shelfJobs()}
	m, _ := jobsRoot(srv)
	press(m, "2")
	runAll(m, eventMsg(jobLine(1, "INFO", "done")))
	if n, _ := srv.counts(); n != 0 {
		t.Fatalf("a job line on Logs fetched %d times", n)
	}
	runAll(m, key("3"))
	if n, _ := srv.counts(); n != 1 {
		t.Fatalf("opening fetched %d times, want 1", n)
	}
	runAll(m, eventMsg(access(2, clock, "5f3a2c30", "/")))
	if n, _ := srv.counts(); n != 1 {
		t.Errorf("an access line fetched: %d", n)
	}
	runAll(m, eventMsg(jobLine(3, "INFO", "done")))
	if n, _ := srv.counts(); n != 2 {
		t.Errorf("a job line fetched %d times in all, want 2", n)
	}
	// The new listing replaces the old one.
	srv.mu.Lock()
	srv.body = `{"workers":true,"schedules":[],"queued":[],"running":[],"dead":[],"done":[],"missed":[]}`
	srv.mu.Unlock()
	runAll(m, eventMsg(jobLine(4, "INFO", "done")))
	if f := m.View(); !strings.Contains(f, "no jobs yet") {
		t.Errorf("the listing was not replaced:\n%s", f)
	}
}

// While the screen is shown it asks for a tick, which refreshes it and asks for the next; once
// another tab is shown the chain ends, and opening again starts one chain, not two.
func TestJobs_Tick(t *testing.T) {
	srv := &jobsServer{body: shelfJobs()}
	m, ticks := jobsRoot(srv)
	runAll(m, key("3"))
	if len(*ticks) != 1 {
		t.Fatalf("opening asked for %d ticks, want 1", len(*ticks))
	}
	tick := (*ticks)[0]
	runAll(m, tick)
	if n, _ := srv.counts(); n != 2 || len(*ticks) != 2 {
		t.Errorf("a tick: %d fetches, %d ticks asked, want 2 and 2", n, len(*ticks))
	}
	press(m, "2")
	runAll(m, (*ticks)[1])
	if n, _ := srv.counts(); n != 2 || len(*ticks) != 2 {
		t.Errorf("a tick while hidden: %d fetches, %d ticks asked, want 2 and 2", n, len(*ticks))
	}
	runAll(m, key("3"))
	runAll(m, (*ticks)[1]) // the old chain's tick
	if n, _ := srv.counts(); n != 3 {
		t.Errorf("an old chain's tick fetched: %d", n)
	}
}

// moveTo moves the Jobs cursor to the row whose text holds s.
func moveTo(t *testing.T, m *root, s string) {
	t.Helper()
	for range 20 {
		press(m, "k")
	}
	for range 20 {
		if r := ansi.Strip(cursorRow(m.View())); strings.Contains(r, s) {
			return
		}
		press(m, "j")
	}
	t.Fatalf("no row %q to move to:\n%s", s, m.View())
}

func footerOf(m *root) string {
	lines := strings.Split(ansi.Strip(m.View()), "\n")
	return lines[len(lines)-1]
}

func TestJobs_Retry(t *testing.T) {
	srv := &jobsServer{body: shelfJobs()}
	m, _ := jobsRoot(srv)
	runAll(m, key("3"))
	moveTo(t, m, " d1 ")
	runAll(m, key("r"))
	if _, posts := srv.counts(); len(posts) != 1 || posts[0] != "/_dev/jobs/d1/retry" {
		t.Fatalf("posts = %q", posts)
	}
	if n, _ := srv.counts(); n != 2 {
		t.Errorf("the listing was fetched %d times, want 2 (the action refreshes it)", n)
	}
	if f := footerOf(m); !strings.Contains(f, "retried d1") {
		t.Errorf("footer = %q", f)
	}
	// Not a dead job: nothing is sent.
	moveTo(t, m, " r1 ")
	runAll(m, key("r"))
	if _, posts := srv.counts(); len(posts) != 1 {
		t.Errorf("r on a running job posted: %q", posts)
	}
	if f := footerOf(m); !strings.Contains(f, "retry: r1 is running, not dead") {
		t.Errorf("footer = %q", f)
	}
}

func TestJobs_RunNow(t *testing.T) {
	srv := &jobsServer{body: shelfJobs()}
	m, _ := jobsRoot(srv)
	runAll(m, key("3"))
	// A schedule with a tick runs that tick; a queued row runs itself.
	moveTo(t, m, "every 1h")
	runAll(m, key("n"))
	moveTo(t, m, " q1 ")
	runAll(m, key("n"))
	if _, posts := srv.counts(); len(posts) != 2 || posts[0] != "/_dev/jobs/t1/run" || posts[1] != "/_dev/jobs/q1/run" {
		t.Fatalf("posts = %q", posts)
	}
	if f := footerOf(m); !strings.Contains(f, "run now requested: Signup.SendWelcome") {
		t.Errorf("footer = %q", f)
	}
	// A schedule with no tick queued, and a dead job: nothing is sent.
	moveTo(t, m, "no tick queued")
	runAll(m, key("n"))
	if f := footerOf(m); !strings.Contains(f, "run now: Cleanup.Sweep has no tick queued") {
		t.Errorf("footer = %q", f)
	}
	moveTo(t, m, " d1 ")
	runAll(m, key("n"))
	if _, posts := srv.counts(); len(posts) != 2 {
		t.Errorf("n on a dead job posted: %q", posts)
	}
}

// The dev server's refusal (a job that has moved on, the cross-origin check) is shown as it
// says it, in yellow.
func TestJobs_ActionRefused(t *testing.T) {
	srv := &jobsServer{body: shelfJobs(), code: http.StatusConflict, text: "job d1 is queued, not dead"}
	m, _ := jobsRoot(srv)
	runAll(m, key("3"))
	moveTo(t, m, " d1 ")
	runAll(m, key("r"))
	if f := m.View(); !strings.Contains(f, "\x1b[33mretry: job d1 is queued, not dead\x1b[0m") {
		t.Errorf("no refusal in\n%s", f)
	}
}

func TestJobs_Keys(t *testing.T) {
	srv := &jobsServer{body: shelfJobs()}
	m, _ := jobsRoot(srv)
	runAll(m, key("3"))
	if f := footerOf(m); !strings.Contains(f, "r retry  n run now  o browser") {
		t.Errorf("footer = %q", f)
	}
	press(m, "?")
	f := ansi.Strip(m.View())
	for _, want := range []string{"Jobs", "r  retry", "n  run now"} {
		if !strings.Contains(f, want) {
			t.Errorf("no %q in the keys overlay:\n%s", want, f)
		}
	}
}

// The cursor stays on its job when the listing changes under it, and the list scrolls to keep it
// in view.
func TestJobs_CursorFollowsJob(t *testing.T) {
	srv := &jobsServer{body: shelfJobs()}
	m, _ := jobsRoot(srv)
	runAll(m, key("3"))
	moveTo(t, m, " d1 ")
	srv.mu.Lock()
	srv.body = strings.Replace(srv.body, `"queued":[`, `"queued":[{"id":"q0","kind":"A.B","state":"queued","params":"{}","runAt":1,"created":1,"attempts":0},`, 1)
	srv.mu.Unlock()
	runAll(m, eventMsg(jobLine(9, "INFO", "done")))
	if r := cursorRow(m.View()); !strings.Contains(ansi.Strip(r), " d1 ") {
		t.Errorf("the cursor left d1: %q", r)
	}
	// A short terminal scrolls.
	run(m, tea.WindowSizeMsg{Width: 100, Height: 9})
	if r := cursorRow(m.View()); !strings.Contains(ansi.Strip(r), " d1 ") {
		t.Errorf("the cursor is out of view: %q\n%s", r, m.View())
	}
}

// A job's line on Logs is set apart from a request's: its `job <Kind>` is cyan. Enter on it opens
// no request (a job has none), and filtering by its id finds its lines.
func TestLogs_JobLine(t *testing.T) {
	m := newRoot(options(nil), fixture(t, "basic")...)
	m.Update(eventMsg(jobLine(20, "INFO", "done")))
	press(m, "2")
	frame := m.View()
	if want := "\x1b[32minfo:\x1b[0m \x1b[36mjob Signup.SendWelcome\x1b[0m d1 done 12ms attempt 1"; !strings.Contains(frame, want) {
		t.Errorf("no %q in\n%s", want, frame)
	}
	if strings.Contains(rowOf(frame, "POST /books/42/lend"), "\x1b[36m") {
		t.Errorf("a request line is cyan")
	}
	press(m, "enter")
	if m.screens[1].(*logsScreen).detail != nil {
		t.Errorf("enter on a job line opened a request")
	}
	m.Update(eventMsg(jobLine(21, "ERROR", "dead")))
	if r := rowOf(m.View(), "dead"); !strings.Contains(r, "\x1b[31merror:\x1b[0m \x1b[36mjob ") {
		t.Errorf("a dead job's line = %q", r)
	}
}
