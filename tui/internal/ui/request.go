package ui

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"alder/tui/internal/look"
	"alder/tui/internal/store"
)

// reqDetail is a request as the dev server serves it at GET /_dev/requests/<rid>: the fields of
// PR 2's ReqRecord (rt/web/devrequests.go).
type reqDetail struct {
	RID      string    `json:"rid"`
	T        time.Time `json:"t"`
	Method   string    `json:"method"`
	Path     string    `json:"path"`
	Route    string    `json:"route"`
	Status   int       `json:"status"`
	Ms       float64   `json:"ms"`
	Bytes    int64     `json:"bytes"`
	Steps    []reqStep `json:"steps"`
	SQL      []reqSQL  `json:"sql"`
	Loads    int       `json:"loads"`
	Cuts     []int     `json:"cuts"`
	Logs     []reqLog  `json:"logs"`
	Slow     bool      `json:"slow"`
	Warnings []string  `json:"warnings"`
}

// reqStep is one effect step the request ran.
type reqStep struct {
	Name string  `json:"name"`
	Ms   float64 `json:"ms"`
	Cap  string  `json:"cap"`
}

// reqSQL is one statement the request ran: its arguments (each as fmt.Sprint shows it), its
// milliseconds and rows, its group, and the count of the N+1 group it belongs to (zero when it
// belongs to none).
type reqSQL struct {
	Stmt    string   `json:"stmt"`
	Args    []string `json:"args"`
	Ms      float64  `json:"ms"`
	Rows    int64    `json:"rows"`
	Group   int      `json:"group"`
	Repeats int      `json:"repeats,omitempty"`
}

// reqLog is one line the request logged.
type reqLog struct {
	T     time.Time `json:"t"`
	Level string    `json:"level"`
	Msg   string    `json:"msg"`
}

// detailTimeout bounds the GET of a request's detail.
const detailTimeout = 5 * time.Second

// goneNote is what the detail says when the dev server no longer has the request.
const goneNote = "the dev server no longer has this request (the last 200 are kept, and a restart clears them)"

// detailMsg is the answer to the GET of request rid's detail: the detail, or gone when the dev
// server answered 404, or the error.
type detailMsg struct {
	rid  string
	d    *reqDetail
	gone bool
	err  error
}

// fetchDetail gets request rid's detail, GET http://<addr>/_dev/requests/<rid>, off the update
// loop.
func fetchDetail(c *Ctx, rid string) tea.Cmd {
	client, url := c.Opts.HTTP, "http://"+c.Opts.Addr+"/_dev/requests/"+rid
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), detailTimeout)
		defer cancel()
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			return detailMsg{rid: rid, err: err}
		}
		resp, err := client.Do(req)
		if err != nil {
			return detailMsg{rid: rid, err: err}
		}
		defer resp.Body.Close()
		switch {
		case resp.StatusCode == http.StatusNotFound:
			return detailMsg{rid: rid, gone: true}
		case resp.StatusCode/100 != 2:
			return detailMsg{rid: rid, err: errors.New(resp.Status)}
		}
		var d reqDetail
		if err := json.NewDecoder(resp.Body).Decode(&d); err != nil {
			return detailMsg{rid: rid, err: err}
		}
		return detailMsg{rid: rid, d: &d}
	}
}

// openRequest opens request r, whose row the detail shows until the dev server answers, and asks
// the dev server for its detail.
func openRequest(c *Ctx, r store.Request) (*requestView, tea.Cmd) {
	return &requestView{req: r}, fetchDetail(c, r.Rid)
}

// detailClaims reports whether msg is a key an open request answers before the frame: tab and
// shift-tab move between its sections, enter unfolds, y copies the rid, and esc, left or
// backspace go back.
func detailClaims(msg tea.KeyMsg) bool {
	switch msg.String() {
	case "tab", "shift+tab", "y", "enter", "esc", "left", "backspace":
		return true
	}
	return false
}

// The detail's sections, in the order tab moves through them.
const (
	secSteps = iota
	secSQL
	secLogs
	sections
)

// logsFloor is the narrowest the Logs section gets side by side: a quarter of the room, but never
// under 8 cells. SQL, whose statements the detail is mostly read for, takes its natural width
// first, up to 55% of the room; an unfolded row shows its statement in full below.
func logsFloor(room int) int { return max(8, room/4) }

// detailKeys are the keys the footer lists while a request is open.
var detailKeys = []Key{{"tab", "section"}, {"enter", "unfold"}, {"y", "copy rid"}, {"esc", "back"}}

// sqlRow is a row of the SQL section: one statement, or the statements of an N+1 group that share
// its text, folded, with their total milliseconds and rows and each one's arguments.
type sqlRow struct {
	stmt   string
	n      int
	repeat bool
	ms     float64
	rows   int64
	args   [][]string
}

// requestView is the open request: its row in the table, then its detail once the dev server
// answers (or that it is gone, or the error); the focused section, each section's cursor and the
// unfolded SQL rows.
type requestView struct {
	req     store.Request
	d       *reqDetail
	sql     []sqlRow
	groups  int
	gone    bool
	err     string
	section int
	cursor  [sections]int
	open    map[int]bool
}

// set records the dev server's answer, folding each N+1 group's statements that share a text
// into the row of the first of them.
func (v *requestView) set(msg detailMsg) {
	switch {
	case msg.gone:
		v.gone = true
		return
	case msg.err != nil:
		v.err = msg.err.Error()
		return
	}
	v.d = msg.d
	type key struct {
		repeats int
		stmt    string
	}
	at := map[key]int{}
	for _, q := range v.d.SQL {
		k := key{q.Repeats, q.Stmt}
		if i, ok := at[k]; ok && q.Repeats > 0 {
			r := &v.sql[i]
			r.n, r.ms, r.rows, r.args = r.n+1, r.ms+q.Ms, r.rows+q.Rows, append(r.args, q.Args)
			continue
		}
		if q.Repeats > 0 {
			at[k] = len(v.sql)
			v.groups++
		}
		v.sql = append(v.sql, sqlRow{stmt: clean(q.Stmt), n: 1, repeat: q.Repeats > 0, ms: q.Ms, rows: q.Rows,
			args: [][]string{q.Args}})
	}
}

// key answers a key: esc, left or backspace go back to the table (back is true); tab and
// shift-tab move between the sections; j/k and the arrows move the section's cursor; enter
// unfolds or folds the SQL row under it; y copies the rid.
func (v *requestView) key(msg tea.KeyMsg, c *Ctx) (back bool, cmd tea.Cmd) {
	switch msg.String() {
	case "esc", "left", "backspace":
		return true, nil
	case "tab":
		v.section = (v.section + 1) % sections
	case "shift+tab":
		v.section = (v.section + sections - 1) % sections
	case "j", "down":
		v.cursor[v.section] = min(v.cursor[v.section]+1, max(v.count(v.section)-1, 0))
	case "k", "up":
		v.cursor[v.section] = max(v.cursor[v.section]-1, 0)
	case "enter":
		if v.section == secSQL && v.cursor[secSQL] < len(v.sql) {
			if v.open == nil {
				v.open = map[int]bool{}
			}
			v.open[v.cursor[secSQL]] = !v.open[v.cursor[secSQL]]
		}
	case "y":
		return false, copyText(c, v.req.Rid)
	}
	return false, nil
}

// count is the number of rows section s holds.
func (v *requestView) count(s int) int {
	if v.d == nil {
		return 0
	}
	switch s {
	case secSteps:
		return len(v.d.Steps)
	case secSQL:
		return len(v.sql)
	}
	return len(v.d.Logs)
}

// header is ` ← METHOD path  status  ms  rid R  route P  steps N  db S/Tms`, from the detail once
// it has arrived and from the table's row before (or when the dev server no longer has it). A
// request without a route leaves it out.
func (v *requestView) header() string {
	r := v.req
	method, path, route, status, ms := r.Method, r.Path, r.Route, r.Status, r.Ms
	steps, stmts, dbms := r.Steps, r.DBStmts, r.DBMs
	if d := v.d; d != nil {
		method, path, route, status, ms = d.Method, d.Path, d.Route, d.Status, d.Ms
		steps, stmts, dbms = len(d.Steps), len(d.SQL), 0
		for _, q := range d.SQL {
			dbms += q.Ms
		}
	}
	h := "← " + method + " " + clean(path) + "  " + look.StatusClass(status)(strconv.Itoa(status)) + "  " + fmtMs(ms) +
		"  rid " + r.Rid
	if route != "" {
		h += "  route " + clean(route)
	}
	return h + "  steps " + strconv.Itoa(steps) + "  db " + strconv.Itoa(stmts) + "/" + fmtMs(dbms)
}

// view draws the header, then the three sections: side by side, with a border between them, when
// the terminal is at least 80 columns (the frame keeps a cell on each side), stacked below that.
// Before the detail arrives it says so; when the dev server no longer has the request, it says
// that instead.
func (v *requestView) view(width, height int, c *Ctx) string {
	lines := []string{v.header()}
	switch {
	case v.gone:
		return strings.Join(append(lines, "", goneNote), "\n")
	case v.err != "":
		return strings.Join(append(lines, "", look.Palette.Yellow("GET /_dev/requests/"+v.req.Rid+": "+v.err)), "\n")
	case v.d == nil:
		return strings.Join(append(lines, "", look.Palette.Dim("loading /_dev/requests/"+v.req.Rid+" …")), "\n")
	}
	h := max(height-1, 3)
	if width+2 >= 80 {
		// Steps is as wide as its rows (up to a third of the room), SQL as wide as its rows within
		// what Steps and the floor of Logs leave, and Logs takes the rest.
		room := width - 6
		sw := min(v.stepsWidth(), room/3)
		qw := max(min(v.sqlWidth(), room*55/100, room-sw-logsFloor(room)), 0)
		lw := room - sw - qw
		cols := [sections][]string{v.sectionLines(secSteps, sw, h), v.sectionLines(secSQL, qw, h), v.sectionLines(secLogs, lw, h)}
		bar := " " + look.Palette.Dim("│") + " "
		for i := range h {
			lines = append(lines, strings.TrimRight(cols[0][i]+bar+cols[1][i]+bar+cols[2][i], " "))
		}
		return strings.Join(lines, "\n")
	}
	each := h / sections
	for s := range sections {
		for _, l := range v.sectionLines(s, width, each) {
			lines = append(lines, strings.TrimRight(l, " "))
		}
	}
	return strings.Join(lines, "\n")
}

// stepsWidth is the width of the Steps section's widest line.
func (v *requestView) stepsWidth() int {
	w := len("Steps")
	for _, l := range v.stepLines(func(int) string { return "  " }) {
		w = max(w, ansi.StringWidth(l))
	}
	return w
}

// stepLines are the Steps section's rows, name, milliseconds and capability, each after its
// mark.
func (v *requestView) stepLines(mark func(int) string) []string {
	rs := make([][]string, len(v.d.Steps))
	msw := 0
	for _, st := range v.d.Steps {
		msw = max(msw, len(fmtMs(st.Ms)))
	}
	for i, st := range v.d.Steps {
		rs[i] = []string{clean(st.Name), padLeft(fmtMs(st.Ms), msw), st.Cap}
	}
	var out []string
	for i, l := range look.Columns(rs, 2) {
		out = append(out, mark(i)+l)
	}
	return out
}

// sectionLines is section s in exactly h lines of w cells: its heading in bold, then its rows,
// scrolled to keep the cursor in view; the focused section marks the row under its cursor.
// The Logs rows are `level: message` without a stamp: a request's lines share their second.
func (v *requestView) sectionLines(s, w, h int) []string {
	var title string
	var body []string
	at := -1 // the body line of the cursor
	mark := func(i int) string {
		if s == v.section && i == v.cursor[s] {
			at = len(body)
			return "> "
		}
		return "  "
	}
	switch s {
	case secSteps:
		title = "Steps"
		body = v.stepLines(mark)
	case secSQL:
		groups := "groups"
		if v.groups == 1 {
			groups = "group"
		}
		title = "SQL (" + strconv.Itoa(len(v.d.SQL)) + ", " + strconv.Itoa(v.groups) + " repeat " + groups + ")"
		body = v.sqlLines(w, mark)
	case secLogs:
		title = "Logs (" + strconv.Itoa(len(v.d.Logs)) + ")"
		for i, l := range v.d.Logs {
			body = append(body, mark(i)+levelWord(l.Level)+": "+clean(l.Msg))
		}
	}
	title = look.Palette.Bold(title)
	if len(body) == 0 {
		body = []string{"  " + look.Palette.Dim("none")}
	}
	room := max(h-1, 1)
	off := 0
	if at >= room {
		off = at - room + 1
	}
	body = body[off:min(len(body), off+room)]
	out := make([]string, 0, h)
	for _, l := range append([]string{title}, body...) {
		out = append(out, padRight(ansi.Truncate(l, w, "…"), w))
	}
	for len(out) < h {
		out = append(out, strings.Repeat(" ", w))
	}
	return out[:h]
}

// sqlChrome is the cells of a SQL row that are not its text: the mark, the fold mark and its
// space, two spaces after the statement and one between the count, milliseconds and rows.
const sqlChrome = 2 + 2 + 2 + 1 + 1

// sqlCells are each SQL row's count (`×N`, then `n+1` for an N+1 group), milliseconds and rows,
// and the width of each of those columns.
func (v *requestView) sqlCells() ([][]string, []int) {
	cs := make([][]string, len(v.sql))
	tw := make([]int, 3)
	for i, r := range v.sql {
		count := "×" + strconv.Itoa(r.n)
		if r.repeat {
			count += " n+1"
		}
		rows := strconv.FormatInt(r.rows, 10) + " rows"
		if r.rows == 1 {
			rows = "1 row"
		}
		cs[i] = []string{count, fmtMs(r.ms), rows}
		for j, x := range cs[i] {
			tw[j] = max(tw[j], ansi.StringWidth(x))
		}
	}
	return cs, tw
}

// sqlWidth is the width of the SQL section with no statement cut: its widest row, or its heading.
func (v *requestView) sqlWidth() int {
	_, tw := v.sqlCells()
	w := 0
	for _, r := range v.sql {
		w = max(w, ansi.StringWidth(r.stmt))
	}
	return max(w+sqlChrome+tw[0]+tw[1]+tw[2], len("SQL (")+20)
}

// sqlLines are the SQL section's lines in w cells: each row as its fold mark (▸ folded, ▾
// unfolded, for a row with arguments), its statement cut to fit, `×N` and `n+1` for an N+1
// group, its milliseconds and rows. An unfolded row is followed by its statement in full when it
// was cut, then its statements' arguments, wrapped.
func (v *requestView) sqlLines(w int, mark func(int) string) []string {
	cs, tw := v.sqlCells()
	stmtW := max(w-sqlChrome-tw[0]-tw[1]-tw[2], 8)
	var out []string
	for i, r := range v.sql {
		fold := " "
		if hasArgs(r) {
			fold = "▸"
			if v.open[i] {
				fold = "▾"
			}
		}
		c := cs[i]
		count := padRight(c[0], tw[0])
		if r.repeat {
			count = strings.Replace(count, "n+1", look.Palette.Yellow("n+1"), 1)
		}
		out = append(out, mark(i)+fold+" "+padRight(ansi.Truncate(r.stmt, stmtW, "…"), stmtW)+"  "+count+
			" "+padLeft(c[1], tw[1])+" "+c[2])
		if v.open[i] {
			if ansi.StringWidth(r.stmt) > stmtW {
				out = append(out, wrap("      ", r.stmt, w)...)
			}
			out = append(out, argLines(r.args, w)...)
		}
	}
	return out
}

// wrap is s cut into lines of w cells after the indent lead.
func wrap(lead, s string, w int) []string {
	room := max(w-len(lead), 8)
	var out []string
	for r := []rune(s); len(r) > 0; {
		n := min(len(r), room)
		out = append(out, lead+string(r[:n]))
		r = r[n:]
	}
	return out
}

// hasArgs reports whether any of a row's statements has arguments.
func hasArgs(r sqlRow) bool {
	for _, a := range r.args {
		if len(a) > 0 {
			return true
		}
	}
	return false
}

// argLines are the arguments of each statement of a row, `args [42] [42] …`, wrapped to w cells
// under the statement.
func argLines(args [][]string, w int) []string {
	const lead, cont = "      args ", "           "
	var out []string
	line := lead
	for i, a := range args {
		item := "[" + clean(strings.Join(a, ", ")) + "]"
		switch {
		case i == 0:
			line += item
		case ansi.StringWidth(line)+1+ansi.StringWidth(item) > w:
			out = append(out, line)
			line = cont + item
		default:
			line += " " + item
		}
	}
	return append(out, line)
}

// levelWord is a log level as `alder run` writes it, lower case: warn yellow, error bold red,
// debug dim, info plain.
func levelWord(level string) string {
	l := strings.ToLower(level)
	switch l {
	case "warn":
		return look.Palette.Yellow(l)
	case "error":
		return look.Palette.BoldRed(l)
	case "debug":
		return look.Palette.Dim(l)
	}
	return l
}
