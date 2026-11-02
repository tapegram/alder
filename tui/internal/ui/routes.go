package ui

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"alder/tui/internal/look"
	"alder/tui/internal/routes"
)

// routesLoadedMsg is the outcome of the run of `alder routes --json` with the generation gen:
// the routes, or why there are none.
type routesLoadedMsg struct {
	gen    int
	routes []routes.Route
	err    string
}

// routesScreen is the Routes tab: a summary line, a rule, and a row per route (kind, path,
// module.name, file:line and the requests it answered this session), in the order alder lists
// them. The routes come from `alder routes --json`, run when the program starts and on every
// build that succeeds; they are kept in Ctx.Routes, which the Requests screen reads to filter by
// route. enter shows the requests of the route under the cursor, and o opens it in the browser
// when it is a page with no parameter.
type routesScreen struct {
	rs      []routes.Route // the routes, the same as Ctx.Routes
	loaded  bool           // a run has succeeded
	gen     int            // counts the runs started, so the answer of an older one is dropped
	err     string         // why the last run failed, or ""
	cursor  int
	top     int
	height  int    // the rows the body has, as of the last tea.WindowSizeMsg
	boot    string // the dev server boot hits were counted for
	hits    []int  // the requests each route of Ctx.Routes answered
	scanned int64  // the seq of the newest request counted in hits
}

func newRoutesScreen() *routesScreen { return &routesScreen{} }

func (*routesScreen) Title() string { return "Routes" }

// Start loads the routes when the program starts.
func (s *routesScreen) Start(c *Ctx) tea.Cmd { return s.load(c) }

// Open loads the routes when none have been loaded yet, such as after the load at start failed.
func (s *routesScreen) Open(c *Ctx) tea.Cmd {
	if s.loaded {
		return nil
	}
	return s.load(c)
}

// Claims takes o while the cursor is on a page that can be opened, so that it opens that page
// and not the app; otherwise o is the frame's.
func (s *routesScreen) Claims(msg tea.KeyMsg) bool {
	return msg.String() == "o" && s.cursor < len(s.rs) && s.rs[s.cursor].Openable()
}

// load starts a run of `alder routes --json` in the app's directory.
func (s *routesScreen) load(c *Ctx) tea.Cmd {
	s.gen++
	gen, r, dir := s.gen, c.Opts.Runner, c.Opts.Dir
	return func() tea.Msg {
		if r == nil {
			return routesLoadedMsg{gen: gen, err: "no alder to run"}
		}
		res, err := r.Run(context.Background(), dir, "routes", "--json")
		switch {
		case err != nil:
			return routesLoadedMsg{gen: gen, err: err.Error()}
		case res.Code != 0:
			msg := fmt.Sprintf("alder routes --json failed (exit %d)", res.Code)
			if l, _, _ := strings.Cut(strings.TrimSpace(string(res.Stderr)), "\n"); l != "" {
				msg += ": " + l
			}
			return routesLoadedMsg{gen: gen, err: msg}
		}
		rs, err := routes.Parse(res.Stdout)
		if err != nil {
			return routesLoadedMsg{gen: gen, err: "alder routes --json: " + err.Error()}
		}
		return routesLoadedMsg{gen: gen, routes: rs}
	}
}

// Update answers msg and then clamps the cursor and the scroll position View reads, so View never
// writes them.
func (s *routesScreen) Update(msg tea.Msg, c *Ctx) (Screen, tea.Cmd) {
	next, cmd := s.update(msg, c)
	s.clamp()
	return next, cmd
}

// headerRows is how many lines View puts above the routes: the summary, the rule and the error.
func (s *routesScreen) headerRows() int {
	if s.err != "" {
		return 3
	}
	return 2
}

// bodyRows is how many routes fit under the header.
func (s *routesScreen) bodyRows() int { return max(s.height-s.headerRows(), 1) }

// clamp keeps the cursor on a route and top where keeps the cursor in view.
func (s *routesScreen) clamp() {
	if len(s.rs) == 0 {
		s.cursor, s.top = 0, 0
		return
	}
	h := s.bodyRows()
	s.cursor = max(min(s.cursor, len(s.rs)-1), 0)
	s.top = max(min(s.top, s.cursor, len(s.rs)-h), 0)
	if s.cursor >= s.top+h {
		s.top = s.cursor - h + 1
	}
}

func (s *routesScreen) update(msg tea.Msg, c *Ctx) (Screen, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		s.height = max(msg.Height-3, 0) // the frame keeps the status bar, a rule and the footer
	case routesLoadedMsg:
		if msg.gen != s.gen {
			return s, nil
		}
		if msg.err != "" {
			s.err = msg.err
			return s, nil
		}
		s.err, s.loaded = "", true
		// The cursor stays on the route it was on, when the new routes still hold it.
		var was *routes.Route
		if s.cursor < len(s.rs) {
			was = &s.rs[s.cursor]
		}
		s.rs, s.hits, s.scanned, s.cursor = msg.routes, nil, 0, 0
		for i, r := range s.rs {
			if was != nil && r == *was {
				s.cursor = i
			}
		}
		c.Routes = s.rs
		s.count(c)
	case eventMsg:
		if msg.Type == "build" && msg.Build != nil && msg.Build.State == "ok" {
			return s, s.load(c)
		}
		s.count(c)
	case tea.KeyMsg:
		return s, s.key(msg.String(), c)
	}
	return s, nil
}

func (s *routesScreen) key(k string, c *Ctx) tea.Cmd {
	n := len(s.rs)
	switch k {
	case "k", "up":
		s.cursor = max(s.cursor-1, 0)
	case "j", "down":
		s.cursor = max(min(s.cursor+1, n-1), 0)
	case "enter":
		if s.cursor >= n {
			return nil
		}
		r := s.rs[s.cursor]
		return func() tea.Msg { return switchTabMsg{tab: 0, route: &r} }
	case "o":
		if s.cursor < n && s.rs[s.cursor].Openable() {
			return openURL(c, "http://"+c.Opts.Addr+s.rs[s.cursor].Path)
		}
	}
	return nil
}

// count adds to hits the requests that have arrived since the last count, each to the route the
// router would have answered it with. Requests are matched as they arrive, so a route's count is
// of the session; when the routes are loaded again the requests the table still holds are
// counted anew, and when the table starts over (another boot of the dev server) so do the counts.
func (s *routesScreen) count(c *Ctx) {
	n := c.Requests.Len()
	// Another boot of the dev server starts the seqs over, so a seq compare alone can miss it.
	if c.Hello != nil {
		if s.boot != "" && c.Hello.Boot != s.boot {
			s.hits, s.scanned = nil, 0
		}
		s.boot = c.Hello.Boot
	}
	if n == 0 || c.Requests.At(n-1).Seq < s.scanned {
		s.hits, s.scanned = nil, 0
	}
	if len(s.hits) != len(s.rs) {
		s.hits, s.scanned = make([]int, len(s.rs)), 0
	}
	if n == 0 {
		return
	}
	for i := n - 1; i >= 0; i-- {
		r := c.Requests.At(i)
		if r.Seq <= s.scanned {
			break
		}
		if j, ok := routes.MatchRequest(s.rs, r.Method, r.Path); ok {
			s.hits[j]++
		}
	}
	s.scanned = c.Requests.At(n - 1).Seq
}

// View draws the summary, a rule and the rows that fit, keeping the cursor in view.
func (s *routesScreen) View(width, height int, c *Ctx) string {
	total := 0
	for _, h := range s.hits {
		total += h
	}
	summary := strconv.Itoa(len(s.rs)) + " routes  " + strconv.Itoa(total) + " hits"
	if !s.loaded {
		summary = "loading routes"
		if s.err != "" {
			summary = "no routes"
		}
	}
	lines := []string{summary, strings.Repeat("─", width)}
	if s.err != "" {
		lines = append(lines, "  "+look.Palette.Yellow(ansi.Truncate(clean(s.err), max(width-2, 1), "…")))
	}
	if len(s.rs) == 0 {
		if s.loaded {
			lines = append(lines, "  "+look.Palette.Dim("the app has no routes"))
		}
		return strings.Join(lines, "\n")
	}
	h := max(height-len(lines), 1)
	top := max(min(s.top, len(s.rs)-1), 0) // clamp has already run; View never writes it
	// The columns are as wide as their widest cell over all the routes, so they do not move as
	// the list scrolls; the path takes what the others leave.
	cells := make([][]string, len(s.rs))
	w := make([]int, 5)
	for i, r := range s.rs {
		hits := 0
		if i < len(s.hits) {
			hits = s.hits[i]
		}
		cells[i] = []string{r.Kind, clean(r.Path), clean(r.Module + "." + r.Name), clean(r.File) + ":" + strconv.Itoa(r.Line),
			strconv.Itoa(hits)}
		for k, cl := range cells[i] {
			w[k] = max(w[k], ansi.StringWidth(cl))
		}
	}
	w[1] = max(min(w[1], width-(2+w[0]+w[2]+w[3]+w[4]+2*4)), 4)
	for i := top; i < min(top+h, len(s.rs)); i++ {
		mark := "  "
		if i == s.cursor {
			mark = "> "
		}
		cs := cells[i]
		hits := padLeft(cs[4], w[4])
		if cs[4] == "0" {
			hits = look.Palette.Dim(hits)
		}
		row := strings.Join([]string{padRight(cs[0], w[0]), padRight(ansi.Truncate(cs[1], w[1], "…"), w[1]),
			padRight(cs[2], w[2]), padRight(cs[3], w[3]), hits}, "  ")
		lines = append(lines, ansi.Truncate(mark+row, width, ""))
	}
	return strings.Join(lines, "\n")
}

func (s *routesScreen) Keys() []Key {
	if s.cursor < len(s.rs) && s.rs[s.cursor].Openable() {
		return []Key{{"enter", "requests"}, {"o", "open"}}
	}
	return []Key{{"enter", "requests"}}
}

func (*routesScreen) Command(*Ctx) string { return "alder routes --json" }
