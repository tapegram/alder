package ui

import (
	"net/url"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"alder/tui/internal/look"
	"alder/tui/internal/routes"
	"alder/tui/internal/store"
)

// stripWindow is the span the stats strip covers.
const stripWindow = 60 * time.Second

// stripTick is how often the strip is redrawn while the Requests tab is open, so its window slides
// and its figures decay when no request arrives.
const stripTick = time.Second

// stripTickMsg is the strip's clock. gen is the generation of the chain that sent it; a chain
// ends when its tick finds another tab open or a newer chain started.
type stripTickMsg struct{ gen int }

// switchTabMsg asks the frame to switch to tab (0 is Requests), and when rid is set to filter
// the Logs screen to that request, and when route is set to filter the Requests screen to that
// route.
type switchTabMsg struct {
	tab   int
	rid   string
	route *routes.Route
}

// routeFilterMsg tells the screens that Ctx.RouteFilter was set, so the Requests screen closes
// its open request and follows the newest row of the filtered table.
type routeFilterMsg struct{}

// requestsScreen is the Requests tab: the strip of the last minute, a rule, and the table of
// answered requests, newest at the bottom. The cursor follows the newest row until it moves up;
// G follows again. `/` filters the rows by their text, enter opens the request under the cursor
// and l shows its lines in Logs.
type requestsScreen struct {
	follow bool         // the cursor stays on the newest row as rows arrive
	sel    int64        // the seq of the row under the cursor while not following
	top    int          // the position of the first row drawn
	filter string       // the text every row shown holds, ignoring case
	typing bool         // the `/` input is active and takes the keys
	routed bool         // Ctx.RouteFilter is set, so the footer offers esc to clear it
	detail *requestView // the open request, or nil
	height int          // the rows the body has, as of the last tea.WindowSizeMsg

	gen   int                                              // the generation of the live strip tick chain
	after func(d time.Duration, msg tea.Msg) tea.Cmd       // sends msg after d; a seam for tests
	match func([]routes.Route, string, string) (int, bool) // the router's match; a seam for tests
	memo  rowsMemo                                         // the rows last computed
}

// rowsKey is what the rows of the table depend on: the text filter, the route filter, the table
// (End moves with every request added) and the routes the router matches against.
type rowsKey struct {
	filter  string
	routed  bool
	route   routes.Route
	end     int64
	nroutes int
	first   *routes.Route
}

// rowsMemo is the rows computed for key, and once the table has been drawn the width of each of
// its columns over all rows.
type rowsMemo struct {
	ok   bool
	key  rowsKey
	rows rows
	w    []int
}

// newRequestsScreen is the Requests tab, following the newest row.
func newRequestsScreen() *requestsScreen {
	return &requestsScreen{follow: true, match: routes.MatchRequest,
		after: func(d time.Duration, msg tea.Msg) tea.Cmd {
			return tea.Tick(d, func(time.Time) tea.Msg { return msg })
		}}
}

// Start begins the strip's clock when the Requests tab is the one the program opens on.
func (s *requestsScreen) Start(c *Ctx) tea.Cmd {
	if c.Tab != 0 {
		return nil
	}
	return s.tick()
}

// Open begins the strip's clock each time the Requests tab becomes the current one.
func (s *requestsScreen) Open(*Ctx) tea.Cmd { return s.tick() }

// tick starts a new chain of strip ticks, ending the previous one.
func (s *requestsScreen) tick() tea.Cmd {
	s.gen++
	return s.after(stripTick, stripTickMsg{s.gen})
}

func (*requestsScreen) Title() string { return "Requests" }

// Claims takes every key while the filter input is active (ctrl+c aside, which the frame keeps),
// and while a request is open the keys the detail answers: tab and shift-tab move between its
// sections, enter unfolds, y copies the rid, and esc, left or backspace go back to the table.
func (s *requestsScreen) Claims(msg tea.KeyMsg) bool {
	return s.typing || s.detail != nil && detailClaims(msg)
}

// Update answers msg and then clamps the state View reads (the scroll position), so View never
// writes it.
func (s *requestsScreen) Update(msg tea.Msg, c *Ctx) (Screen, tea.Cmd) {
	next, cmd := s.update(msg, c)
	s.clamp(c)
	return next, cmd
}

func (s *requestsScreen) update(msg tea.Msg, c *Ctx) (Screen, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		s.height = max(msg.Height-3, 0) // the frame keeps the status bar, a rule and the footer
	case stripTickMsg:
		if msg.gen == s.gen && c.Tab == 0 {
			return s, s.after(stripTick, stripTickMsg{s.gen})
		}
	case routeFilterMsg:
		s.detail, s.typing, s.routed, s.follow, s.top = nil, false, true, true, 0
	case detailMsg:
		if s.detail != nil && s.detail.req.Rid == msg.rid {
			s.detail.set(msg)
		}
	case tea.KeyMsg:
		switch {
		case s.detail != nil:
			back, cmd := s.detail.key(msg, c)
			if back {
				s.detail = nil
			}
			return s, cmd
		case s.typing:
			s.input(msg)
		default:
			return s, s.tableKey(msg.String(), c)
		}
	}
	return s, nil
}

// input edits the filter: enter keeps it and closes the input, esc clears it, backspace drops the
// last character, and any other text is appended. The cursor follows the newest matching row
// again whenever the filter changes.
func (s *requestsScreen) input(msg tea.KeyMsg) {
	before := s.filter
	switch msg.Type {
	case tea.KeyEnter:
		s.typing = false
	case tea.KeyEsc:
		s.typing, s.filter = false, ""
	case tea.KeyBackspace:
		if r := []rune(s.filter); len(r) > 0 {
			s.filter = string(r[:len(r)-1])
		}
	case tea.KeyRunes, tea.KeySpace:
		s.filter += string(msg.Runes)
	}
	if s.filter != before {
		s.follow, s.top = true, 0
	}
}

// tableKey answers a key on the table: j/k and the arrows move the cursor (up stops following,
// down onto the newest row follows again),
// G follows again, `/` opens the filter input, esc clears the text filter and the route filter,
// enter opens the request under the cursor and l switches to Logs filtered to it.
func (s *requestsScreen) tableKey(k string, c *Ctx) tea.Cmd {
	rs := s.rows(c)
	n := rs.len()
	switch k {
	case "/":
		s.typing = true
		return nil
	case "esc":
		if s.filter != "" || c.RouteFilter != nil {
			s.filter, c.RouteFilter, s.routed, s.follow, s.top = "", nil, false, true, 0
		}
		return nil
	case "G":
		s.follow = true
		return nil
	}
	if n == 0 {
		return nil
	}
	cur := s.cursor(rs)
	switch k {
	case "k", "up":
		s.follow, s.sel = false, rs.at(max(cur-1, 0)).Seq
	case "j", "down":
		next := min(cur+1, n-1)
		s.follow, s.sel = next == n-1, rs.at(next).Seq
	case "enter":
		var cmd tea.Cmd
		s.detail, cmd = openRequest(c, rs.at(cur))
		return cmd
	case "l":
		rid := rs.at(cur).Rid // read here: the store is touched only in Update
		return func() tea.Msg { return switchTabMsg{tab: 1, rid: rid} }
	}
	return nil
}

// rows are the table's rows that hold the filter, oldest first.
type rows struct {
	q   *store.Requests
	idx []int // the positions in q of the rows that hold the filter; nil without one
}

func (r rows) len() int {
	if r.idx == nil {
		return r.q.Len()
	}
	return len(r.idx)
}

func (r rows) at(i int) store.Request {
	if r.idx == nil {
		return r.q.At(i)
	}
	return r.q.At(r.idx[i])
}

// rows are the requests that hold the text filter and are answered by the route filter's route.
// Without either they are the table itself, read in place. They are kept until the filters, the
// table or the routes change, so a key or a frame does not match every row again.
func (s *requestsScreen) rows(c *Ctx) rows {
	key := rowsKey{filter: s.filter, end: c.Requests.End(), nroutes: len(c.Routes)}
	if len(c.Routes) > 0 {
		key.first = &c.Routes[0]
	}
	if c.RouteFilter != nil {
		key.routed, key.route = true, *c.RouteFilter
	}
	if s.memo.ok && s.memo.key == key {
		return s.memo.rows
	}
	rs := rows{q: c.Requests}
	if s.filter != "" || c.RouteFilter != nil {
		needle := strings.ToLower(s.filter)
		rs.idx = []int{}
		for i := range c.Requests.Len() {
			r := c.Requests.At(i)
			if c.RouteFilter != nil && !s.answeredBy(c, r, *c.RouteFilter) {
				continue
			}
			if strings.Contains(strings.ToLower(strings.Join(cells(r), " ")), needle) {
				rs.idx = append(rs.idx, i)
			}
		}
	}
	s.memo = rowsMemo{ok: true, key: key, rows: rs}
	return rs
}

// answeredBy reports whether the router would have answered r with the route want, among the
// routes the Routes screen loaded. Routes are the same when their kind, path, module and name are:
// the position of the declaration moves with every edit above it, and the filter outlives the
// reload that follows each build.
func (s *requestsScreen) answeredBy(c *Ctx, r store.Request, want routes.Route) bool {
	i, ok := s.match(c.Routes, r.Method, r.Path)
	if !ok {
		return false
	}
	got := c.Routes[i]
	return got.Kind == want.Kind && got.Path == want.Path && got.Module == want.Module && got.Name == want.Name
}

// bodyRows is how many table rows fit under the strip and its rule.
func (s *requestsScreen) bodyRows() int { return max(s.height-2, 1) }

// clamp keeps top where the rows and the cursor put it: at the first row of the last page while
// following, else no lower than the cursor and no higher than keeps it in view. The frame runs it
// at the end of every Update (a key, a resize, a request arriving), so View only reads it.
func (s *requestsScreen) clamp(c *Ctx) {
	rs := s.rows(c)
	n, h := rs.len(), s.bodyRows()
	if n == 0 {
		s.top = 0
		return
	}
	cur := s.cursor(rs)
	if s.follow {
		s.top = n - h
	}
	s.top = max(min(s.top, cur, n-h), 0)
	if cur >= s.top+h {
		s.top = cur - h + 1
	}
}

// cursor is the position of the row under the cursor: the newest while following, else the row
// with the selected seq (seqs rise through the table), or the next one when it has gone.
func (s *requestsScreen) cursor(rs rows) int {
	n := rs.len()
	if s.follow || n == 0 {
		return n - 1
	}
	i := sort.Search(n, func(i int) bool { return rs.at(i).Seq >= s.sel })
	return min(i, n-1)
}

// The columns of a row's cells.
const (
	colTime = iota
	colMethod
	colPath
	colStatus
	colMs
	colBytes
	colSteps
	colFlags
)

// cells are a row's columns as plain text: the time (local, to the millisecond), method, path,
// status, milliseconds, size, steps and flags (slow, and n+1 when the request repeated a
// statement).
func cells(r store.Request) []string {
	var flags []string
	if r.Slow {
		flags = append(flags, "slow")
	}
	if r.Repeats > 0 {
		flags = append(flags, "n+1")
	}
	return []string{r.Time.Local().Format("15:04:05.000"), r.Method, clean(r.Path), strconv.Itoa(r.Status),
		fmtMs(r.Ms), fmtBytes(r.Bytes), strconv.Itoa(r.Steps), strings.Join(flags, " ")}
}

// View draws the strip, a rule and the rows that fit, keeping the cursor in view, or the open
// request.
func (s *requestsScreen) View(width, height int, c *Ctx) string {
	if s.detail != nil {
		return s.detail.view(width, height, c)
	}
	lines := []string{s.strip(c), strings.Repeat("─", width)}
	rs := s.rows(c)
	n, h := rs.len(), max(height-2, 1)
	if n == 0 {
		msg := "no requests yet"
		switch {
		case s.filter != "" && c.RouteFilter != nil:
			msg = "no requests match route " + clean(c.RouteFilter.Path) + " and /" + clean(s.filter)
		case s.filter != "":
			msg = "no requests match /" + clean(s.filter)
		case c.RouteFilter != nil:
			msg = "no requests match route " + clean(c.RouteFilter.Path)
		}
		return strings.Join(append(lines, "  "+look.Palette.Dim(msg)), "\n")
	}
	cur := s.cursor(rs)
	top := min(s.top, n-1) // clamp has already run; View never writes it
	vis := make([][]string, 0, h)
	for i := top; i < min(top+h, n); i++ {
		vis = append(vis, cells(rs.at(i)))
	}
	w := slices.Clone(s.widths(rs))
	// The path takes what the other columns and their gutters leave.
	rest := 2 + 2*colFlags
	for i, x := range w {
		if i != colPath {
			rest += x
		}
	}
	w[colPath] = max(min(w[colPath], width-rest), 4)
	for k, cs := range vis {
		i := top + k
		mark := "  "
		if i == cur {
			mark = "> "
		}
		lines = append(lines, mark+s.row(rs.at(i), cs, w))
	}
	return strings.Join(lines, "\n")
}

// widths are the width of each column over all rows, so the columns do not move as the table
// scrolls. They are kept with the rows.
func (s *requestsScreen) widths(rs rows) []int {
	if s.memo.w != nil {
		return s.memo.w
	}
	w := make([]int, colFlags+1)
	for i := range rs.len() {
		for k, cl := range cells(rs.at(i)) {
			w[k] = max(w[k], ansi.StringWidth(cl))
		}
	}
	s.memo.w = w
	return w
}

// row is one table row laid out in the column widths w: the cells are padded first, then styled,
// the status by its class, the milliseconds yellow when slow and the flags yellow.
func (s *requestsScreen) row(r store.Request, cs []string, w []int) string {
	ms := padLeft(cs[colMs], w[colMs])
	if r.Slow {
		ms = look.Palette.Yellow(ms)
	}
	out := []string{
		cs[colTime],
		padRight(cs[colMethod], w[colMethod]),
		padRight(ansi.Truncate(cs[colPath], w[colPath], "…"), w[colPath]),
		look.StatusClass(r.Status)(padLeft(cs[colStatus], w[colStatus])),
		ms,
		padLeft(cs[colBytes], w[colBytes]),
		padLeft(cs[colSteps], w[colSteps]),
		look.Palette.Yellow(cs[colFlags]),
	}
	return strings.TrimRight(strings.Join(out, "  "), " ")
}

// strip is the last minute as ` last 60s  12.4 req/s  p50 8ms  p95 41ms  errors 0.0%`, then the
// filter when one is set (bold while it is being typed). Only the requests of the window are
// counted.
func (s *requestsScreen) strip(c *Ctx) string {
	now := c.Opts.Now()
	var recent []store.Request
	for i := c.Requests.Len() - 1; i >= 0; i-- {
		r := c.Requests.At(i)
		if !r.Time.After(now.Add(-stripWindow)) {
			continue // arrival is not time order: a later row can still be inside the window
		}
		recent = append(recent, r)
	}
	st := store.Stats(recent, now)
	line := "last 60s  " + strconv.FormatFloat(st.ReqPerSec, 'f', 1, 64) + " req/s  p50 " + fmtStripMs(st.P50) +
		"  p95 " + fmtStripMs(st.P95) + "  errors " + strconv.FormatFloat(st.ErrPct, 'f', 1, 64) + "%"
	if c.RouteFilter != nil {
		line += "  route " + clean(c.RouteFilter.Path)
	}
	switch {
	case s.typing:
		line += "  " + look.Palette.Bold(clean("/"+s.filter))
	case s.filter != "":
		line += "  /" + clean(s.filter)
	}
	return line
}

func (s *requestsScreen) Keys() []Key {
	switch {
	case s.detail != nil:
		return detailKeys
	case s.typing:
		return []Key{{"enter", "apply"}, {"esc", "clear"}}
	}
	keys := []Key{{"enter", "open"}, {"l", "logs"}, {"/", "filter"}, {"G", "follow"}}
	if s.routed {
		keys = append(keys, Key{"esc", "clear filters"})
	}
	return keys
}

func (s *requestsScreen) Command(c *Ctx) string {
	if s.detail != nil {
		return "curl " + c.Opts.Addr + "/_dev/requests/" + url.PathEscape(s.detail.req.Rid)
	}
	return "alder run · ALDER_LOG=debug"
}

// fmtMs is milliseconds to a tenth, `38.9ms`.
func fmtMs(ms float64) string { return strconv.FormatFloat(ms, 'f', 1, 64) + "ms" }

// fmtStripMs is the strip's milliseconds: whole from 1 ms up (and at zero), `41ms`, and to a
// tenth below 1 ms.
func fmtStripMs(ms float64) string {
	if ms > 0 && ms < 1 {
		return fmtMs(ms)
	}
	return strconv.FormatFloat(ms, 'f', 0, 64) + "ms"
}

// fmtBytes is a size in bytes below 1 KiB, `512`, else in KiB or MiB to a tenth, `4.2K`.
func fmtBytes(n int64) string {
	switch {
	case n < 1<<10:
		return strconv.FormatInt(n, 10)
	case n < 1<<20:
		return strconv.FormatFloat(float64(n)/(1<<10), 'f', 1, 64) + "K"
	}
	return strconv.FormatFloat(float64(n)/(1<<20), 'f', 1, 64) + "M"
}

// padRight pads s with spaces to w cells; padLeft does so on the left.
func padRight(s string, w int) string { return s + strings.Repeat(" ", max(w-ansi.StringWidth(s), 0)) }
func padLeft(s string, w int) string  { return strings.Repeat(" ", max(w-ansi.StringWidth(s), 0)) + s }

// clean is s on one line with no control characters, each replaced by a space, so that text from
// the app cannot move the cursor or break a row.
func clean(s string) string {
	return strings.Map(func(r rune) rune {
		if r < 0x20 || r == 0x7f || r >= 0x80 && r <= 0x9f {
			return ' '
		}
		return r
	}, s)
}
