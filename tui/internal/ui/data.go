package ui

import (
	"context"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"alder/tui/internal/data"
	"alder/tui/internal/feed"
	"alder/tui/internal/look"
)

// The Data screen refreshes its counts dataDebounce after the last request that wrote; it gives
// up on the map after mapTimeout and on a table's count or page after tableTimeout.
const (
	dataDebounce = 500 * time.Millisecond
	mapTimeout   = 5 * time.Second
	tableTimeout = 5 * time.Second
)

// The widest a column of rows is drawn, and the narrowest and widest the tables pane is.
const (
	dataColMax  = 30
	dataLeftMin = 18
	dataLeftMax = 44
)

// The messages of the Data screen: the map fetched with the generation gen (or why not); the
// counts and the selected table's page read by load gen and page gen; a page read alone; a
// query's result; and the debounce of the write with the given id has passed.
type (
	dataMapMsg struct {
		gen int
		m   data.Map
		err string
	}
	dataLoadedMsg struct {
		gen, pageGen int
		counts       []tableCount
		dbErr        string
		page         dataPage
	}
	dataPageMsg struct {
		gen  int
		page dataPage
	}
	dataQueryMsg struct {
		gen  int
		page dataPage
	}
	dataWroteMsg struct{ id int }
)

// tableCount is one table's row count, or why it could not be read.
type tableCount struct {
	n   int64
	err string
}

// dataPage is rows read from a table or a query: the columns, the rows, or why there are none.
type dataPage struct {
	cols []string
	rows [][]string
	err  string
}

// dataQuery is a statement typed at the prompt and its result, once it has come.
type dataQuery struct {
	stmt string
	done bool
	page dataPage
}

// dataScreen is the Data tab. The left pane lists the app's tables, by their Alder names, from
// /_dev/map.json, each with its row count, then the migrations applied; when the app has silos a
// line above them names the database shown, which d changes. The right pane shows the selected
// table's rows, a page of 100 at a time ([ and ] page), or the result of a statement typed at
// the `:` prompt. enter moves into the rows; there j/k move between rows and h/l between
// columns, s sorts by the column under the cursor (again for descending), enter shows the row
// vertically with long text unfolded, and esc goes back.
//
// The database is opened read-only for each read, in a command, never in Update: alder run
// recreates dev.db when it starts, so a handle kept open would read a deleted file. A table that
// cannot be read (missing from the database after a rename between rebuilds, or a database
// locked mid-migration) shows its error on its row, in red, and the rest keeps working.
//
// The map is fetched when the program starts, when the tab is opened before it has one, on every
// build that succeeds and whenever the app starts; the counts and the page are read again
// dataDebounce after a request that wrote: one whose method is not GET or HEAD that ran
// statements, or any POST.
type dataScreen struct {
	after func(d time.Duration, msg tea.Msg) tea.Cmd // sends msg after d; tests replace it

	m       data.Map
	loaded  bool   // a map has been fetched
	mapErr  string // why the last fetch failed, or ""
	mapGen  int
	dbs     []data.Choice
	db      int
	tables  []data.Table
	counts  []tableCount // by table; nil until read
	dbErr   string       // why the database could not be opened, or ""
	loadGen int
	cursor  int // the selected table
	right   bool

	offset  int
	sortBy  string // the SQL name of the column the page is sorted by, or "" for the id
	desc    bool
	page    dataPage
	pageGen int
	row     int // the row and column under the cursor in the right pane
	col     int
	top     int // the first row drawn
	detail  bool
	dtop    int // the first line of the row view drawn

	typing   bool
	input    string
	query    *dataQuery // the query shown in place of the table, or nil
	queryGen int
	wrote    int // counts the writes seen, so only the last one's debounce refreshes
}

func newDataScreen() *dataScreen {
	return &dataScreen{after: func(d time.Duration, msg tea.Msg) tea.Cmd {
		return tea.Tick(d, func(time.Time) tea.Msg { return msg })
	}}
}

func (*dataScreen) Title() string { return "Data" }

// Start fetches the map when the program starts.
func (s *dataScreen) Start(c *Ctx) tea.Cmd { return s.fetch(c) }

// Open fetches the map when there is none yet, such as when the dev server was not answering at
// start, and otherwise reads the counts and the page again.
func (s *dataScreen) Open(c *Ctx) tea.Cmd {
	if !s.loaded {
		return s.fetch(c)
	}
	return s.load()
}

// Claims takes every key while the prompt is active (ctrl+c aside, which the frame keeps).
func (s *dataScreen) Claims(tea.KeyMsg) bool { return s.typing }

// buildDir is the directory of the app's build, which holds dev.db and the silos.
func buildDir(c *Ctx) string { return filepath.Join(c.Opts.Dir, "build", appName(c)) }

// fetch gets the map, GET http://<addr>/_dev/map.json.
func (s *dataScreen) fetch(c *Ctx) tea.Cmd {
	s.mapGen++
	gen, client, addr := s.mapGen, c.Opts.HTTP, c.Opts.Addr
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), mapTimeout)
		defer cancel()
		m, err := data.FetchMap(ctx, client, addr)
		if err != nil {
			return dataMapMsg{gen: gen, err: err.Error()}
		}
		return dataMapMsg{gen: gen, m: m}
	}
}

// selected is the selected table, if there is one.
func (s *dataScreen) selected() (data.Table, bool) {
	if s.cursor < len(s.tables) {
		return s.tables[s.cursor], true
	}
	return data.Table{}, false
}

// load reads, in one opening of the database, every table's count and the selected table's page.
func (s *dataScreen) load() tea.Cmd {
	if !s.loaded || s.db >= len(s.dbs) {
		return nil
	}
	s.loadGen++
	s.pageGen++
	gen, pageGen, path, tables := s.loadGen, s.pageGen, s.dbs[s.db].Path, s.tables
	t, ok := s.selected()
	by, desc, offset := s.sortBy, s.desc, s.offset
	return func() tea.Msg {
		msg := dataLoadedMsg{gen: gen, pageGen: pageGen}
		db, err := data.Open(path)
		if err != nil {
			msg.dbErr = data.Message(err)
			msg.page.err = data.Message(err)
			return msg
		}
		defer db.Close()
		msg.counts = make([]tableCount, len(tables))
		for i, tb := range tables {
			ctx, cancel := context.WithTimeout(context.Background(), tableTimeout)
			n, err := db.Count(ctx, tb)
			cancel()
			msg.counts[i] = tableCount{n: n}
			if err != nil {
				msg.counts[i].err = data.Message(err)
			}
		}
		if ok {
			msg.page = readPage(db, t, by, desc, offset)
		}
		return msg
	}
}

// loadPage reads the selected table's page alone.
func (s *dataScreen) loadPage() tea.Cmd {
	t, ok := s.selected()
	if !ok || s.db >= len(s.dbs) {
		return nil
	}
	s.pageGen++
	gen, path, by, desc, offset := s.pageGen, s.dbs[s.db].Path, s.sortBy, s.desc, s.offset
	return func() tea.Msg {
		db, err := data.Open(path)
		if err != nil {
			return dataPageMsg{gen: gen, page: dataPage{err: data.Message(err)}}
		}
		defer db.Close()
		return dataPageMsg{gen: gen, page: readPage(db, t, by, desc, offset)}
	}
}

func readPage(db *data.DB, t data.Table, by string, desc bool, offset int) dataPage {
	ctx, cancel := context.WithTimeout(context.Background(), tableTimeout)
	defer cancel()
	cols, rows, err := db.Page(ctx, t, by, desc, offset)
	if err != nil {
		return dataPage{err: data.Message(err)}
	}
	return dataPage{cols: cols, rows: rows}
}

// runQuery runs stmt on the database shown.
func (s *dataScreen) runQuery(stmt string) tea.Cmd {
	s.queryGen++
	s.query = &dataQuery{stmt: stmt}
	s.row, s.col, s.top, s.right = 0, 0, 0, true
	if err := data.Guard(stmt); err != nil {
		s.query.done, s.query.page.err = true, data.Message(err)
		return nil
	}
	if s.db >= len(s.dbs) {
		s.query.done, s.query.page.err = true, "no database: the map has not been fetched"
		return nil
	}
	gen, path := s.queryGen, s.dbs[s.db].Path
	return func() tea.Msg {
		db, err := data.Open(path)
		if err != nil {
			return dataQueryMsg{gen: gen, page: dataPage{err: data.Message(err)}}
		}
		defer db.Close()
		cols, rows, err := db.Query(context.Background(), stmt)
		if err != nil {
			return dataQueryMsg{gen: gen, page: dataPage{err: data.Message(err)}}
		}
		return dataQueryMsg{gen: gen, page: dataPage{cols: cols, rows: rows}}
	}
}

func (s *dataScreen) Update(msg tea.Msg, c *Ctx) (Screen, tea.Cmd) {
	switch msg := msg.(type) {
	case dataMapMsg:
		if msg.gen != s.mapGen {
			return s, nil
		}
		if msg.err != "" {
			s.mapErr = msg.err
			return s, nil
		}
		s.mapErr, s.loaded = "", true
		s.setMap(msg.m, c)
		return s, s.load()
	case dataLoadedMsg:
		if msg.gen == s.loadGen {
			s.counts, s.dbErr = msg.counts, msg.dbErr
		}
		if msg.pageGen == s.pageGen {
			s.setPage(msg.page)
		}
	case dataPageMsg:
		if msg.gen == s.pageGen {
			s.setPage(msg.page)
		}
	case dataQueryMsg:
		if s.query != nil && msg.gen == s.queryGen {
			s.query.done, s.query.page = true, msg.page
		}
	case dataWroteMsg:
		if msg.id == s.wrote {
			return s, s.load()
		}
	case eventMsg:
		return s, s.event(feed.Event(msg), c)
	case tea.KeyMsg:
		if s.typing {
			return s, s.inputKey(msg)
		}
		return s, s.key(msg.String())
	}
	return s, nil
}

// event fetches the map again on a build that succeeds and when the app starts, and schedules a
// refresh of the counts after a request that wrote.
func (s *dataScreen) event(e feed.Event, c *Ctx) tea.Cmd {
	switch {
	case e.Type == "build" && e.Build != nil && e.Build.State == "ok",
		e.Type == "app" && e.App != nil && e.App.State == "started":
		return s.fetch(c)
	case e.Type == "log" && e.Log != nil && e.Log.Record != nil && e.Log.Record.HasAccess:
		if r := e.Log.Record; wrote(r.Method, r.DBStmts) {
			s.wrote++
			return s.after(dataDebounce, dataWroteMsg{id: s.wrote})
		}
	}
	return nil
}

// wrote reports whether a request with method that ran stmts statements may have written: a
// method other than GET and HEAD that ran statements, or any POST.
func wrote(method string, stmts int) bool {
	switch method {
	case "POST":
		return true
	case "GET", "HEAD":
		return false
	}
	return stmts > 0
}

// setMap takes a new map: the databases (staying on the one shown, by its label, when it is still
// there), its tables and the selected table (staying on it, by its name, when it is still there).
func (s *dataScreen) setMap(m data.Map, c *Ctx) {
	label, name := "", ""
	if s.db < len(s.dbs) {
		label = s.dbs[s.db].Label
	}
	if t, ok := s.selected(); ok {
		name = t.Name
	}
	s.m, s.dbs, s.db = m, data.Databases(buildDir(c), m), 0
	for i, d := range s.dbs {
		if d.Label == label {
			s.db = i
		}
	}
	s.setTables(name)
}

// setTables lists the tables of the database shown and selects the one called name, or the first.
func (s *dataScreen) setTables(name string) {
	s.tables, s.counts, s.dbErr = s.m.TablesFor(s.dbs[s.db]), nil, ""
	cur, found := 0, false
	for i, t := range s.tables {
		if t.Name == name {
			cur, found = i, true
		}
	}
	if !found {
		s.resetTable()
	}
	s.cursor = cur
}

// resetTable forgets the page of the table that was selected, its sort and cursor.
func (s *dataScreen) resetTable() {
	s.offset, s.sortBy, s.desc, s.page = 0, "", false, dataPage{}
	s.row, s.col, s.top, s.detail = 0, 0, 0, false
}

// setPage takes a page that has been read, keeping the cursor within it.
func (s *dataScreen) setPage(p dataPage) {
	s.page = p
	s.row = max(min(s.row, len(p.rows)-1), 0)
	s.col = max(min(s.col, len(p.cols)-1), 0)
}

// inputKey edits the prompt: enter runs the statement (an empty one closes the prompt), esc
// closes it, backspace drops the last character and any other text is appended.
func (s *dataScreen) inputKey(msg tea.KeyMsg) tea.Cmd {
	switch msg.Type {
	case tea.KeyEnter:
		s.typing = false
		if stmt := strings.TrimSpace(s.input); stmt != "" {
			return s.runQuery(stmt)
		}
	case tea.KeyEsc:
		s.typing = false
	case tea.KeyBackspace:
		if r := []rune(s.input); len(r) > 0 {
			s.input = string(r[:len(r)-1])
		}
	case tea.KeyRunes, tea.KeySpace:
		s.input += string(msg.Runes)
	}
	return nil
}

// shown is what the right pane shows: the query's result, or the table's page.
func (s *dataScreen) shown() dataPage {
	if s.query != nil {
		return s.query.page
	}
	return s.page
}

// key answers a key outside the prompt.
func (s *dataScreen) key(k string) tea.Cmd {
	p := s.shown()
	if s.detail {
		switch k {
		case "esc", "enter", "left", "backspace", "h":
			s.detail = false
		case "j", "down":
			s.dtop++
		case "k", "up":
			s.dtop = max(s.dtop-1, 0)
		}
		return nil
	}
	switch k {
	case ":":
		s.typing, s.input = true, ""
		if s.query != nil {
			s.input = s.query.stmt
		}
		return nil
	case "d":
		if len(s.dbs) < 2 {
			return nil
		}
		s.db = (s.db + 1) % len(s.dbs)
		s.query, s.right = nil, false
		s.setTables("")
		return s.load()
	}
	if !s.right {
		switch k {
		case "k", "up":
			return s.move(s.cursor - 1)
		case "j", "down":
			return s.move(s.cursor + 1)
		case "enter", "l", "right":
			s.right = len(p.rows) > 0
		case "[", "]":
			return s.turn(k)
		}
		return nil
	}
	switch k {
	case "esc":
		if s.query != nil {
			s.query, s.row, s.col, s.top = nil, 0, 0, 0
			s.setPage(s.page)
			return nil
		}
		s.right = false
	case "k", "up":
		s.row = max(s.row-1, 0)
	case "j", "down":
		s.row = max(min(s.row+1, len(p.rows)-1), 0)
	case "h", "left":
		s.col = max(s.col-1, 0)
	case "l", "right":
		s.col = max(min(s.col+1, len(p.cols)-1), 0)
	case "enter":
		s.detail, s.dtop = s.row < len(p.rows), 0
	case "s":
		return s.sort()
	case "[", "]":
		return s.turn(k)
	}
	return nil
}

// move selects the table at i and reads its page.
func (s *dataScreen) move(i int) tea.Cmd {
	i = max(min(i, len(s.tables)-1), 0)
	if i == s.cursor || len(s.tables) == 0 {
		return nil
	}
	s.cursor = i
	s.resetTable()
	return s.loadPage()
}

// turn moves to the previous ([) or next (]) page of the table, within its count when it is known.
func (s *dataScreen) turn(k string) tea.Cmd {
	if s.query != nil {
		return nil
	}
	next := s.offset - data.PageRows
	if k == "]" {
		next = s.offset + data.PageRows
		if s.cursor < len(s.counts) && s.counts[s.cursor].err == "" {
			if int64(next) >= s.counts[s.cursor].n {
				return nil
			}
		} else if len(s.page.rows) < data.PageRows {
			return nil
		}
	}
	if next < 0 || next == s.offset {
		return nil
	}
	s.offset, s.row, s.top = next, 0, 0
	return s.loadPage()
}

// sort sorts the table by the column under the cursor, or, when it is already sorted by it, in
// the other direction. A column that is not one of the map's for the table sorts nothing.
func (s *dataScreen) sort() tea.Cmd {
	t, ok := s.selected()
	if s.query != nil || !ok || s.col >= len(s.page.cols) {
		return nil
	}
	name := s.page.cols[s.col]
	known := false
	for _, cl := range t.Cols {
		known = known || cl.SQL == name
	}
	if !known {
		return nil
	}
	if s.sortBy == name {
		s.desc = !s.desc
	} else {
		s.sortBy, s.desc = name, false
	}
	s.offset, s.row, s.top = 0, 0, 0
	return s.loadPage()
}

// View draws the tables pane and the rows pane side by side, a rule between them.
func (s *dataScreen) View(width, height int, c *Ctx) string {
	if !s.loaded {
		if s.mapErr != "" {
			return look.Palette.Yellow(ansi.Truncate("no map: "+clean(s.mapErr), max(width, 1), "…")) + "\n" +
				look.Palette.Dim("the map is fetched from the app at /_dev/map.json; it comes back on the next build")
		}
		return "loading the map"
	}
	left := s.leftPane(width)
	lw := 0
	for _, l := range left {
		lw = max(lw, ansi.StringWidth(l))
	}
	lw = max(min(lw, dataLeftMax, width/2), min(dataLeftMin, width/2))
	rw := max(width-lw-3, 1)
	var right []string
	if s.detail {
		right = s.rowView(rw, height)
	} else {
		right = s.rightPane(rw, height)
	}
	lines := make([]string, height)
	for i := range height {
		l, r := "", ""
		if i < len(left) {
			l = ansi.Truncate(left[i], lw, "…")
		}
		if i < len(right) {
			r = ansi.Truncate(right[i], rw, "…")
		}
		lines[i] = padRight(l, lw) + " │ " + r
		if r == "" {
			lines[i] = strings.TrimRight(lines[i], " ")
		}
	}
	return strings.Join(lines, "\n")
}

// leftPane is the tables pane's lines: the database shown when there are silos, the tables with
// their counts (or their errors, in red) and the migrations applied. Tables keep their natural
// width here; View cuts them to the pane.
func (s *dataScreen) leftPane(width int) []string {
	var lines []string
	if len(s.m.Silos) > 0 && s.db < len(s.dbs) {
		lines = append(lines, "database "+look.Palette.Bold(clean(s.dbs[s.db].Label)), "")
	}
	lines = append(lines, look.Palette.Bold("Tables"))
	if s.dbErr != "" {
		lines = append(lines, look.Palette.BoldRed(clean(s.dbErr)))
	}
	nw, cw := 0, 0
	for i, t := range s.tables {
		nw = max(nw, ansi.StringWidth(clean(t.Name)))
		if i < len(s.counts) && s.counts[i].err == "" {
			cw = max(cw, len(strconv.FormatInt(s.counts[i].n, 10)))
		}
	}
	for i, t := range s.tables {
		mark := "  "
		if i == s.cursor {
			mark = "> "
		}
		count := look.Palette.Dim("…")
		switch {
		case i < len(s.counts) && s.counts[i].err != "":
			count = look.Palette.BoldRed(ansi.Truncate(clean(s.counts[i].err), max(width/2, 20), "…"))
		case i < len(s.counts):
			count = padLeft(strconv.FormatInt(s.counts[i].n, 10), cw)
		case s.dbErr != "":
			count = ""
		}
		lines = append(lines, mark+padRight(clean(t.Name), nw)+"  "+count)
	}
	if len(s.tables) == 0 {
		lines = append(lines, look.Palette.Dim("  no tables"))
	}
	lines = append(lines, "", look.Palette.Bold("Migrations"))
	for _, mg := range s.m.Migrations {
		lines = append(lines, "  "+look.Palette.Dim(clean(mg)))
	}
	if len(s.m.Migrations) == 0 {
		lines = append(lines, look.Palette.Dim("  none"))
	}
	return lines
}

// rightPane is the rows pane's lines: the prompt or a heading, then the rows as a grid.
func (s *dataScreen) rightPane(width, height int) []string {
	var head string
	p := s.shown()
	switch {
	case s.typing:
		head = look.Palette.Bold(":" + clean(s.input))
	case s.query != nil:
		head = look.Palette.Bold("query") + "  " + look.Palette.Dim(clean(s.query.stmt))
	default:
		t, ok := s.selected()
		if !ok {
			return nil
		}
		head = look.Palette.Bold(clean(t.Name))
		if n := len(p.rows); n > 0 {
			head += "  rows " + strconv.Itoa(s.offset+1) + "–" + strconv.Itoa(s.offset+n)
			if s.cursor < len(s.counts) && s.counts[s.cursor].err == "" {
				head += " of " + strconv.FormatInt(s.counts[s.cursor].n, 10)
			}
		}
		if s.sortBy != "" {
			head += "  by " + clean(s.sortBy)
			if s.desc {
				head += " desc"
			}
		}
	}
	lines := []string{head}
	switch {
	case s.query != nil && !s.query.done:
		return append(lines, look.Palette.Dim("running"))
	case p.err != "":
		return append(lines, look.Palette.BoldRed(clean(p.err)))
	case s.query != nil:
		n := strconv.Itoa(len(p.rows)) + " rows"
		if len(p.rows) == 1 {
			n = "1 row"
		}
		if len(p.rows) == data.QueryRows {
			n = "the first " + n
		}
		lines = append(lines, n)
	case len(p.cols) == 0:
		return lines
	}
	return append(lines, s.grid(p, width, max(height-len(lines), 2))...)
}

// grid draws p's columns and rows in width×height cells: a bold header with the column under the
// cursor in cyan, then the rows that fit around the cursor. Columns are as wide as their widest
// cell, up to dataColMax; those that do not fit to the right of the cursor's are left off, and
// the grid starts further right when the cursor's column would not fit.
func (s *dataScreen) grid(p dataPage, width, height int) []string {
	w := make([]int, len(p.cols))
	for i, cl := range p.cols {
		w[i] = ansi.StringWidth(clean(cl))
		for _, r := range p.rows {
			w[i] = max(w[i], ansi.StringWidth(clean(r[i])))
		}
		w[i] = min(w[i], dataColMax)
	}
	room := width - 2
	first := 0
	for first < s.col {
		used := 0
		for i := first; i <= s.col; i++ {
			used += w[i] + 2
		}
		if used-2 <= room {
			break
		}
		first++
	}
	last := first
	for used := 0; last < len(p.cols) && (last == first || used+w[last] <= room); last++ {
		used += w[last] + 2
	}
	cells := func(vals []string, style func(int, string) string) string {
		out := make([]string, 0, last-first)
		for i := first; i < last; i++ {
			out = append(out, padRight(style(i, ansi.Truncate(clean(vals[i]), w[i], "…")), w[i]))
		}
		return strings.TrimRight(strings.Join(out, "  "), " ")
	}
	header := cells(p.cols, func(i int, v string) string {
		if s.right && i == s.col {
			return look.Palette.Cyan(v)
		}
		return look.Palette.Bold(v)
	})
	lines := []string{"  " + header}
	h := max(height-1, 1)
	s.top = max(min(s.top, s.row, len(p.rows)-h), 0)
	if s.row >= s.top+h {
		s.top = s.row - h + 1
	}
	for i := s.top; i < min(s.top+h, len(p.rows)); i++ {
		mark := "  "
		if s.right && i == s.row {
			mark = "> "
		}
		lines = append(lines, mark+cells(p.rows[i], func(_ int, v string) string { return v }))
	}
	return lines
}

// rowView is the row under the cursor, one column per line: its name, then its value unfolded
// over as many lines as it takes.
func (s *dataScreen) rowView(width, height int) []string {
	p := s.shown()
	if s.row >= len(p.rows) {
		return nil
	}
	title, n := "query", s.row+1
	if t, ok := s.selected(); ok && s.query == nil {
		title, n = clean(t.Name), s.offset+s.row+1
	}
	lines := []string{look.Palette.Bold(title) + "  row " + strconv.Itoa(n)}
	nw := 0
	for _, cl := range p.cols {
		nw = max(nw, ansi.StringWidth(clean(cl)))
	}
	vw := max(width-nw-2, 10)
	var body []string
	for i, cl := range p.cols {
		var vals []string
		for _, part := range strings.Split(p.rows[s.row][i], "\n") {
			vals = append(vals, strings.Split(ansi.Wrap(clean(part), vw, ""), "\n")...)
		}
		for j, v := range vals {
			if j > 0 {
				body = append(body, strings.TrimRight(strings.Repeat(" ", nw+2)+v, " "))
				continue
			}
			body = append(body, strings.TrimRight(padRight(look.Palette.Bold(clean(cl)), nw)+"  "+v, " "))
		}
	}
	s.dtop = max(min(s.dtop, len(body)-(height-1)), 0)
	return append(lines, body[s.dtop:]...)
}

func (s *dataScreen) Keys() []Key {
	switch {
	case s.typing:
		return []Key{{"enter", "run"}, {"esc", "cancel"}}
	case s.detail:
		return []Key{{"j/k", "scroll"}, {"esc", "back"}}
	case s.right && s.query != nil:
		return []Key{{"enter", "row"}, {":", "sql"}, {"esc", "back"}}
	case s.right:
		return []Key{{"enter", "row"}, {"s", "sort"}, {"[ ]", "page"}, {":", "sql"}, {"esc", "back"}}
	}
	keys := []Key{{"enter", "rows"}, {"[ ]", "page"}, {":", "sql"}}
	if len(s.dbs) > 1 {
		keys = append(keys, Key{"d", "database"})
	}
	return keys
}

// Command is sqlite3 on the database shown, with the statement while the prompt is active or a
// query's result is shown.
func (s *dataScreen) Command(c *Ctx) string {
	path := "build/" + appName(c) + "/dev.db"
	if s.db < len(s.dbs) && s.dbs[s.db].Silo != "" {
		if rel, err := filepath.Rel(buildDir(c), s.dbs[s.db].Path); err == nil {
			path = "build/" + appName(c) + "/" + filepath.ToSlash(rel)
		}
	}
	args := []string{path}
	switch {
	case s.typing && s.input != "":
		args = append(args, s.input)
	case !s.typing && s.query != nil:
		args = append(args, s.query.stmt)
	}
	return "sqlite3 " + shellJoin(args)
}
