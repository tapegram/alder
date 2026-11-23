package ui

import (
	"context"
	"strconv"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"alder/tui/internal/jobs"
	"alder/tui/internal/look"
)

// tabJobs is the index of the Jobs tab.
const tabJobs = 5

const (
	// jobsEvery is how often the shown Jobs screen reads the listing again, besides when a job's
	// line arrives on the feed.
	jobsEvery = 2 * time.Second
	// jobsTimeout bounds each GET /_dev/jobs.json and each POST of an action.
	jobsTimeout = 5 * time.Second
)

// The messages the Jobs screen sends itself: a listing read (or why it was not), the clock's
// tick, and the outcome of an action.
type (
	jobsLoadedMsg struct {
		v   jobs.View
		err error
	}
	jobsTickMsg  struct{ gen int }
	jobsActedMsg struct{ flash string }
)

// The sections of the screen, in the order it lists them.
var jobsSections = []string{"Schedules", "Queued", "Running", "Dead", "Done"}

const (
	secSchedules = iota
	secQueued
	secRunning
	secDead
	secDone
)

// jobItem is a row the cursor can be on: a schedule or a job, in section sec.
type jobItem struct {
	sec   int
	sched *jobs.Schedule
	row   *jobs.Row
}

// key identifies the item across listings, so that the cursor stays on it.
func (it jobItem) key() string {
	if it.sched != nil {
		return "s " + it.sched.Kind
	}
	return strconv.Itoa(it.sec) + " " + it.row.Silo + " " + it.row.ID
}

// jobsScreen is the Jobs tab: a summary line (the count of jobs in each state, whether workers
// run), a rule, and the app's schedules with their next tick and its queued, running, dead and
// last done jobs, as GET /_dev/jobs.json lists them. The listing is read when the tab is opened,
// when a job's line arrives on the feed while the tab is shown, and every jobsEvery while it is
// shown. r retries the dead job under the cursor, and n runs the queued job, or the schedule's
// queued tick, under it now.
type jobsScreen struct {
	view     jobs.View
	loaded   bool   // a read has succeeded
	err      string // why the last read failed, or ""
	inflight bool   // a read is on its way; another is asked for when it ends (again)
	again    bool
	gen      int // counts the chains of ticks started, so that an old chain ends
	cursor   int
	curKey   string // the key of the item under the cursor, to find it again
	top      int
	height   int                                        // the rows the body has, as of the last tea.WindowSizeMsg
	after    func(d time.Duration, msg tea.Msg) tea.Cmd // sends msg after d; a seam for tests
}

func newJobsScreen() *jobsScreen {
	return &jobsScreen{after: func(d time.Duration, msg tea.Msg) tea.Cmd {
		return tea.Tick(d, func(time.Time) tea.Msg { return msg })
	}}
}

func (*jobsScreen) Title() string { return "Jobs" }

// Open reads the listing and starts a chain of ticks, ending the previous one, each time the tab
// becomes the current one.
func (s *jobsScreen) Open(c *Ctx) tea.Cmd {
	s.gen++
	return tea.Batch(s.fetch(c), s.after(jobsEvery, jobsTickMsg{s.gen}))
}

// fetch reads the listing, GET http://<addr>/_dev/jobs.json. While a read is on its way it asks
// for none more, but remembers to read again when it ends, so a burst of job lines makes two
// reads, not one for each.
func (s *jobsScreen) fetch(c *Ctx) tea.Cmd {
	if s.inflight {
		s.again = true
		return nil
	}
	s.inflight = true
	client, addr := c.Opts.HTTP, c.Opts.Addr
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), jobsTimeout)
		defer cancel()
		v, err := jobs.Fetch(ctx, client, addr)
		return jobsLoadedMsg{v: v, err: err}
	}
}

// items are the rows the cursor can be on, section by section.
func (s *jobsScreen) items() []jobItem {
	var out []jobItem
	for i := range s.view.Schedules {
		out = append(out, jobItem{sec: secSchedules, sched: &s.view.Schedules[i]})
	}
	for sec, rows := range [][]jobs.Row{s.view.Queued, s.view.Running, s.view.Dead, s.view.Done} {
		for i := range rows {
			out = append(out, jobItem{sec: sec + secQueued, row: &rows[i]})
		}
	}
	return out
}

// lineOf is the index in the body of the line of item i: each section's heading takes a line, and
// an empty section has none.
func (s *jobsScreen) lineOf(items []jobItem, i int) int {
	line := i + 1
	for k := 1; k <= i; k++ {
		if items[k].sec != items[k-1].sec {
			line++
		}
	}
	return line
}

// headerRows is how many lines View puts above the body: the summary, the rule, the stores that
// were missed and the error.
func (s *jobsScreen) headerRows() int {
	n := 2 + len(s.view.Missed)
	if s.err != "" {
		n++
	}
	return n
}

// bodyRows is how many body lines fit under the header.
func (s *jobsScreen) bodyRows() int { return max(s.height-s.headerRows(), 1) }

// clamp keeps the cursor on an item and top where keeps the cursor, and its section's heading when
// it is the section's first row, in view.
func (s *jobsScreen) clamp() {
	items := s.items()
	if len(items) == 0 {
		s.cursor, s.top = 0, 0
		return
	}
	s.cursor = max(min(s.cursor, len(items)-1), 0)
	s.curKey = items[s.cursor].key()
	h := s.bodyRows()
	line := s.lineOf(items, s.cursor)
	first := line
	if s.cursor == 0 || items[s.cursor-1].sec != items[s.cursor].sec {
		first = line - 1
	}
	total := s.lineOf(items, len(items)-1) + 1
	s.top = max(min(s.top, first, total-h), 0)
	if line >= s.top+h {
		s.top = line - h + 1
	}
}

// Update answers msg and then clamps the cursor and the scroll position View reads, so View
// never writes them.
func (s *jobsScreen) Update(msg tea.Msg, c *Ctx) (Screen, tea.Cmd) {
	next, cmd := s.update(msg, c)
	s.clamp()
	return next, cmd
}

func (s *jobsScreen) update(msg tea.Msg, c *Ctx) (Screen, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		s.height = max(msg.Height-3, 0) // the frame keeps the status bar, a rule and the footer
	case jobsLoadedMsg:
		s.inflight = false
		if msg.err != nil {
			s.err = msg.err.Error()
		} else {
			s.err, s.loaded, s.view = "", true, msg.v
			// The cursor stays on the item it was on, when the new listing still holds it.
			for i, it := range s.items() {
				if it.key() == s.curKey {
					s.cursor = i
				}
			}
		}
		if s.again {
			s.again = false
			return s, s.fetch(c)
		}
	case jobsTickMsg:
		if msg.gen == s.gen && c.Tab == tabJobs {
			return s, tea.Batch(s.fetch(c), s.after(jobsEvery, jobsTickMsg{s.gen}))
		}
	case jobsActedMsg:
		text := msg.flash
		return s, tea.Batch(func() tea.Msg { return flashMsg(text) }, s.fetch(c))
	case eventMsg:
		if r := msg.Log; msg.Type == "log" && r != nil && r.Record != nil && r.Record.HasJob && c.Tab == tabJobs {
			return s, s.fetch(c)
		}
	case tea.KeyMsg:
		return s, s.key(msg.String(), c)
	}
	return s, nil
}

func (s *jobsScreen) key(k string, c *Ctx) tea.Cmd {
	items := s.items()
	switch k {
	case "k", "up":
		s.cursor = max(s.cursor-1, 0)
	case "j", "down":
		s.cursor = max(min(s.cursor+1, len(items)-1), 0)
	case "r", "n":
		if s.cursor >= len(items) {
			return nil
		}
		return s.act(k, items[s.cursor], c)
	}
	return nil
}

// act is r (retry) or n (run now) on it: the POST when the item is of the state the action needs,
// else a line in the footer that says why nothing was sent.
func (s *jobsScreen) act(k string, it jobItem, c *Ctx) tea.Cmd {
	refuse := func(text string) tea.Cmd {
		return func() tea.Msg { return flashMsg(look.Palette.Yellow(text)) }
	}
	var (
		action jobs.Action
		id     string // the job the POST names
		label  string // what the footer names on a failure
		done   string // what the footer says on success
	)
	switch {
	case k == "r":
		action = jobs.Retry
		if it.row == nil {
			return refuse("retry: " + clean(it.sched.Kind) + " is a schedule, not a dead job")
		}
		if it.row.State != "dead" {
			return refuse("retry: " + clean(it.row.ID) + " is " + it.row.State + ", not dead")
		}
		id, label = it.row.ID, "retry"
		done = "retried " + clean(it.row.ID) + " (" + clean(it.row.Kind) + ")"
	case it.sched != nil:
		action = jobs.Run
		if it.sched.ID == "" {
			return refuse("run now: " + clean(it.sched.Kind) + " has no tick queued")
		}
		id, label = it.sched.ID, "run now"
		done = "run now requested: " + clean(it.sched.Kind)
	default:
		action = jobs.Run
		if it.row.State != "queued" {
			return refuse("run now: " + clean(it.row.ID) + " is " + it.row.State + ", not queued")
		}
		id, label = it.row.ID, "run now"
		done = "run now requested: " + clean(it.row.Kind) + " " + clean(it.row.ID)
	}
	client, addr := c.Opts.HTTP, c.Opts.Addr
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), jobsTimeout)
		defer cancel()
		if err := jobs.Act(ctx, client, addr, action, id); err != nil {
			return jobsActedMsg{flash: look.Palette.Yellow(label + ": " + clean(err.Error()))}
		}
		return jobsActedMsg{flash: done}
	}
}

// View draws the summary, a rule, the stores that were missed and the last error, and the rows
// that fit, each section under its heading, keeping the cursor in view.
func (s *jobsScreen) View(width, height int, c *Ctx) string {
	v := s.view
	summary := "loading jobs"
	switch {
	case s.loaded:
		dead := strconv.Itoa(len(v.Dead)) + " dead"
		if len(v.Dead) > 0 {
			dead = look.Palette.Red(dead)
		}
		workers := "workers on"
		if !v.Workers {
			workers = look.Palette.Yellow("workers off")
		}
		summary = strings.Join([]string{strconv.Itoa(len(v.Queued)) + " queued", strconv.Itoa(len(v.Running)) + " running",
			dead, strconv.Itoa(len(v.Done)) + " done", workers}, "  ")
	case s.err != "":
		summary = "no jobs"
	}
	lines := []string{summary, strings.Repeat("─", width)}
	for _, m := range v.Missed {
		lines = append(lines, look.Palette.Yellow(ansi.Truncate(clean(m), width, "…")))
	}
	if s.err != "" {
		lines = append(lines, look.Palette.Yellow(ansi.Truncate(clean(s.err), width, "…")))
	}
	items := s.items()
	if len(items) == 0 {
		if s.loaded {
			lines = append(lines, "  "+look.Palette.Dim("no jobs yet"))
		}
		return strings.Join(lines, "\n")
	}
	body := s.body(items, c.Opts.Now())
	h := max(height-len(lines), 1)
	top := max(min(s.top, len(body)-1), 0) // clamp has already run; View never writes it
	for i := top; i < min(top+h, len(body)); i++ {
		lines = append(lines, ansi.Truncate(body[i], width, "…"))
	}
	return strings.Join(lines, "\n")
}

// body is every line under the header: each section's heading and its rows, the cell columns
// aligned within the section, the item under the cursor marked.
func (s *jobsScreen) body(items []jobItem, now time.Time) []string {
	var out []string
	for i := 0; i < len(items); {
		sec := items[i].sec
		j := i
		for j < len(items) && items[j].sec == sec {
			j++
		}
		cells := make([][]string, 0, j-i)
		for _, it := range items[i:j] {
			cells = append(cells, jobCells(it, now))
		}
		out = append(out, look.Palette.Bold(jobsSections[sec]))
		for k, l := range look.Columns(cells, 2) {
			mark := "  "
			if i+k == s.cursor {
				mark = "> "
			}
			out = append(out, mark+l)
		}
		i = j
	}
	return out
}

// jobCells are an item's columns. A schedule: its kind, its rule and its next tick. A job: its
// kind (with its silo), id and attempts, then what its state adds: when a queued job is due (and
// that it is a tick), the worker of a running one, the first line of a dead one's error.
func jobCells(it jobItem, now time.Time) []string {
	if s := it.sched; s != nil {
		next := look.Palette.Dim("no tick queued")
		if s.Next != nil {
			next = "next " + dueText(*s.Next, now)
		}
		return []string{clean(s.Kind), clean(s.Rule), next}
	}
	r := it.row
	kind := clean(r.Kind)
	if r.Silo != "" {
		kind += " " + look.Palette.Dim("["+clean(r.Silo)+"]")
	}
	cells := []string{kind, clean(r.ID), look.Palette.Dim("attempts " + strconv.Itoa(r.Attempts))}
	switch it.sec {
	case secQueued:
		cells = []string{kind, clean(r.ID), "due " + dueText(r.RunAt, now)}
		if r.Tick {
			cells = append(cells, look.Palette.Dim("tick"))
		}
	case secDead:
		cells = append(cells, look.Palette.Red(clean(r.ErrorLine())))
	}
	return cells
}

// dueText is when ms (milliseconds since the epoch) is, from now: `now` when it has passed, else
// `in 45s`, `in 12m`, `in 3h` or `in 2d`.
func dueText(ms int64, now time.Time) string {
	d := time.UnixMilli(ms).Sub(now)
	if d <= 0 {
		return "now"
	}
	switch {
	case d < time.Minute:
		return "in " + strconv.Itoa(int(d.Seconds())) + "s"
	case d < time.Hour:
		return "in " + strconv.Itoa(int(d.Minutes())) + "m"
	case d < 24*time.Hour:
		return "in " + strconv.Itoa(int(d.Hours())) + "h"
	}
	return "in " + strconv.Itoa(int(d.Hours()/24)) + "d"
}

func (*jobsScreen) Keys() []Key { return []Key{{"r", "retry"}, {"n", "run now"}} }

func (*jobsScreen) Command(*Ctx) string { return "alder jobs" }
