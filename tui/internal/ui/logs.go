package ui

import (
	"regexp"
	"sort"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"alder/tui/internal/feed"
	"alder/tui/internal/look"
	"alder/tui/internal/store"
)

// levels are the levels L cycles through, in order.
var levels = []string{"debug", "info", "warn", "error"}

// logLinePat is a line of the app's log as `alder run` writes it: the stamp, then the level word
// (none for debug), then the text. It is alder run's own pattern (cmd/alder/run.go).
var logLinePat = regexp.MustCompile(`^(\d{4}/\d{2}/\d{2} \d{2}:\d{2}:\d{2}(?:\.\d+)? )(info:|warn:|error:)?(.*)$`)

// logsScreen is the Logs tab: a strip of the active filters, a rule, and the scrollback that
// passes them, newest at the bottom. The cursor follows the newest line until it moves up; G
// follows again. L cycles the level, `/` sets the regexp, x clears the rid and the regexp, and
// enter opens the request of the line under the cursor.
//
// The filters live in the Ctx, since Requests' l sets the rid. The screen keeps the positions of
// the lines that pass them and reads only the lines added since it last looked, starting over
// when the filters change; it draws only the lines that fit.
type logsScreen struct {
	follow bool         // the cursor stays on the newest line as lines arrive
	sel    int64        // the position of the line under the cursor while not following
	top    int          // the index in idx of the first line drawn
	typing bool         // the `/` input is active and takes the keys
	input  string       // the regexp being typed
	reErr  string       // why the last regexp typed was refused
	detail *requestView // the open request, or nil

	idx     []int64      // the positions of the lines that pass key, oldest first
	scanned int64        // the position up to which the scrollback has been read
	key     store.Filter // the filters idx was read for
}

// newLogsScreen is the Logs tab, following the newest line.
func newLogsScreen() *logsScreen { return &logsScreen{follow: true} }

func (*logsScreen) Title() string { return "Logs" }

// Claims takes every key while the regexp input is active (ctrl+c aside, which the frame keeps),
// and while a request is open the keys the detail answers.
func (s *logsScreen) Claims(msg tea.KeyMsg) bool {
	return s.typing || s.detail != nil && detailClaims(msg)
}

func (s *logsScreen) Update(msg tea.Msg, c *Ctx) (Screen, tea.Cmd) {
	switch msg := msg.(type) {
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
			s.inputKey(msg, c)
		default:
			return s, s.listKey(msg.String(), c)
		}
	}
	return s, nil
}

// inputKey edits the regexp: enter compiles it and, when it is valid, filters by it (an empty one
// filters by none), else keeps the old one and records why; esc leaves the old one; backspace
// drops the last character, and any other text is appended.
func (s *logsScreen) inputKey(msg tea.KeyMsg, c *Ctx) {
	switch msg.Type {
	case tea.KeyEnter:
		s.typing = false
		if s.input == "" {
			c.LogFilter.Re = nil
			return
		}
		re, err := regexp.Compile(s.input)
		if err != nil {
			s.reErr = err.Error()
			return
		}
		c.LogFilter.Re = re
	case tea.KeyEsc:
		s.typing = false
	case tea.KeyBackspace:
		if r := []rune(s.input); len(r) > 0 {
			s.input = string(r[:len(r)-1])
		}
	case tea.KeyRunes, tea.KeySpace:
		s.input += string(msg.Runes)
	}
}

// listKey answers a key on the scrollback: `/` opens the regexp input with the current regexp in
// it, L moves to the next level, x clears the rid and the regexp, G follows again, j/k and the
// arrows move the cursor (up stops following, down onto the newest line follows again), and enter
// opens the request of the line under the cursor, if it has one.
func (s *logsScreen) listKey(k string, c *Ctx) tea.Cmd {
	switch k {
	case "/":
		s.typing, s.input, s.reErr = true, "", ""
		if c.LogFilter.Re != nil {
			s.input = c.LogFilter.Re.String()
		}
		return nil
	case "L":
		i := 0
		for j, l := range levels {
			if l == c.LogFilter.Level {
				i = j
			}
		}
		c.LogFilter.Level = levels[(i+1)%len(levels)]
		return nil
	case "x":
		c.LogFilter.Rid, c.LogFilter.Re, s.reErr = "", nil, ""
		return nil
	case "G":
		s.follow = true
		return nil
	}
	s.refresh(c)
	n := len(s.idx)
	if n == 0 {
		return nil
	}
	cur := s.cursor()
	switch k {
	case "k", "up":
		s.follow, s.sel = false, s.idx[max(cur-1, 0)]
	case "j", "down":
		next := min(cur+1, n-1)
		s.follow, s.sel = next == n-1, s.idx[next]
	case "enter":
		ln, _ := c.Logs.Get(s.idx[cur], c.LogFilter)
		if ln.Rid == "" {
			return nil
		}
		r, ok := c.Requests.ByRid(ln.Rid)
		if !ok {
			r = store.Request{Record: feed.Record{Rid: ln.Rid}}
		}
		var cmd tea.Cmd
		s.detail, cmd = openRequest(c, r)
		return cmd
	}
	return nil
}

// refresh brings idx up to date with the scrollback: it starts over when the filters have
// changed (and the cursor follows again), drops the positions of the lines the scrollback no
// longer keeps, and reads the lines added since it last looked.
func (s *logsScreen) refresh(c *Ctx) {
	f, lg := c.LogFilter, c.Logs
	if f != s.key {
		s.key, s.idx, s.scanned = f, s.idx[:0], lg.Start()
		s.follow, s.top = true, 0
	}
	start := lg.Start()
	if k := sort.Search(len(s.idx), func(i int) bool { return s.idx[i] >= start }); k > 0 {
		s.idx, s.top = s.idx[k:], max(s.top-k, 0)
	}
	for i := max(s.scanned, start); i < lg.End(); i++ {
		if _, ok := lg.Get(i, f); ok {
			s.idx = append(s.idx, i)
		}
	}
	s.scanned = lg.End()
}

// cursor is the index in idx of the line under the cursor: the newest while following, else the
// line at the selected position, or the next one when it has gone.
func (s *logsScreen) cursor() int {
	n := len(s.idx)
	if s.follow || n == 0 {
		return n - 1
	}
	i := sort.Search(n, func(i int) bool { return s.idx[i] >= s.sel })
	return min(i, n-1)
}

// View draws the strip, a rule and the lines that fit, keeping the cursor in view, or the open
// request.
func (s *logsScreen) View(width, height int, c *Ctx) string {
	if s.detail != nil {
		return s.detail.view(width, height, c)
	}
	s.refresh(c)
	lines := []string{s.strip(c), strings.Repeat("─", width)}
	n, h := len(s.idx), max(height-2, 1)
	if n == 0 {
		msg := "no lines yet"
		if c.LogFilter.Rid != "" || c.LogFilter.Re != nil {
			msg = "no lines match the filters"
		}
		return strings.Join(append(lines, "  "+look.Palette.Dim(msg)), "\n")
	}
	cur := s.cursor()
	if s.follow {
		s.top = n - h
	}
	s.top = max(min(s.top, cur, n-h), 0)
	if cur >= s.top+h {
		s.top = cur - h + 1
	}
	for i := s.top; i < min(s.top+h, n); i++ {
		ln, _ := c.Logs.Get(s.idx[i], c.LogFilter)
		mark := "  "
		if i == cur {
			mark = "> "
		}
		lines = append(lines, ansi.Truncate(mark+renderLine(ln, width), width, "…"))
	}
	return strings.Join(lines, "\n")
}

// renderLine is a line as `alder run` shows it on a terminal, cut to about width cells first so
// that a huge line costs no more than a short one: a run line dim, after `alder run: ` when it
// does not start with it; an effect line dim; any other line with its stamp dim and its level
// word coloured, `info:` green, `warn:` yellow, `error:` red. The app's text is cleaned of
// control characters before it is styled.
func renderLine(ln store.Line, width int) string {
	text := ln.Text
	if limit := 4*width + 64; len(text) > limit {
		text = text[:limit]
	}
	text = clean(text)
	switch {
	case ln.Run:
		if !strings.HasPrefix(text, "alder run:") {
			text = "alder run: " + text
		}
		return look.Palette.Dim(text)
	case ln.Effect:
		return look.Palette.Dim(text)
	}
	m := logLinePat.FindStringSubmatch(text)
	if m == nil {
		return text
	}
	level := m[2]
	switch level {
	case "info:":
		level = look.Palette.Green(level)
	case "warn:":
		level = look.Palette.Yellow(level)
	case "error:":
		level = look.Palette.Red(level)
	}
	return look.Palette.Dim(m[1]) + level + m[3]
}

// strip is the active filters, ` level info  rid 5f3a2c1e  /lend/`: the level always, the rid
// and the regexp when set; while the input is active the regexp being typed, bold; and the error
// of a regexp just refused, yellow.
func (s *logsScreen) strip(c *Ctx) string {
	f := c.LogFilter
	parts := []string{"level " + f.Level}
	if f.Rid != "" {
		parts = append(parts, "rid "+clean(f.Rid))
	}
	switch {
	case s.typing:
		parts = append(parts, look.Palette.Bold("/"+s.input))
	case f.Re != nil:
		parts = append(parts, "/"+f.Re.String()+"/")
	}
	if s.reErr != "" {
		parts = append(parts, look.Palette.Yellow(s.reErr))
	}
	return strings.Join(parts, "  ")
}

func (s *logsScreen) Keys() []Key {
	switch {
	case s.detail != nil:
		return detailKeys
	case s.typing:
		return []Key{{"enter", "apply"}, {"esc", "cancel"}}
	}
	return []Key{{"enter", "open"}, {"L", "level"}, {"/", "regexp"}, {"x", "clear"}, {"G", "follow"}}
}

// Command is `ALDER_LOG=<level> alder run`, with ALDER_LOG_FORMAT=json when the level is debug and
// a rid filter is on, since only alder run's JSON lines carry the rid of every line a request
// logs; or the open request's curl.
func (s *logsScreen) Command(c *Ctx) string {
	if s.detail != nil {
		return "curl " + c.Opts.Addr + "/_dev/requests/" + s.detail.req.Rid
	}
	cmd := "ALDER_LOG=" + c.LogFilter.Level
	if c.LogFilter.Level == "debug" && c.LogFilter.Rid != "" {
		cmd += " ALDER_LOG_FORMAT=json"
	}
	return cmd + " alder run"
}
