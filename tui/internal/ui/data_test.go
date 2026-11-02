package ui

import (
	"database/sql"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/exp/teatest"

	"alder/tui/internal/feed"
)

// shelfMap is /_dev/map.json of the shelf app: books and loans, two migrations applied.
const shelfMap = `{"build":"b7","map":{"pages":[],"tables":[` +
	`{"name":"Books.Book","sql":"Books__Book","columns":[],"cols":[` +
	`{"name":"id","type":"Id Book","sql":"id","sqlType":"TEXT"},` +
	`{"name":"title","type":"String","sql":"title","sqlType":"TEXT"},` +
	`{"name":"year","type":"Int","sql":"year","sqlType":"INTEGER"}],"id":"id"},` +
	`{"name":"Lending.Loan","sql":"Lending__Loan","columns":[],"cols":[` +
	`{"name":"id","type":"Id Loan","sql":"id","sqlType":"TEXT"},` +
	`{"name":"book","type":"Id Book","sql":"book","sqlType":"TEXT"},` +
	`{"name":"note","type":"Maybe String","sql":"note","sqlType":"TEXT"}],"id":"id"}` +
	`%s],"session":[]},"caps":[],"migrations":["0001_init.sql","0002_loans.sql"],"routes":[]}`

// goneTable is a table the map lists that dev.db does not have, as after a rename between
// rebuilds.
const goneTable = `,{"name":"Books.Shelf","sql":"Books__Shelf","columns":[],"cols":[],"id":"id"}`

// dataApp makes the shelf app's build directory with dev.db holding three books and one loan,
// and serves mapJSON at /_dev/map.json. It is options() with the app's directory and the server's
// address, and the path of dev.db.
func dataApp(t *testing.T, mapJSON string) (Options, string) {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "build", "shelf", "dev.db")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	execDB(t, path,
		`CREATE TABLE "Books__Book" ("id" TEXT PRIMARY KEY, "title" TEXT NOT NULL, "year" INTEGER NOT NULL)`,
		`CREATE TABLE "Lending__Loan" ("id" TEXT PRIMARY KEY, "book" TEXT NOT NULL, "note" TEXT)`,
		`INSERT INTO "Books__Book" VALUES ('b1', 'Middlemarch', 1871), ('b2', 'Dune', 1965), ('b3', 'Beloved', 1987)`,
		`INSERT INTO "Lending__Loan" VALUES ('l1', 'b2', NULL)`)
	o := options(nil)
	o.Dir = dir
	o.HTTP = serve(func() string { return mapJSON })
	return o, path
}

// serve is a client whose requests the app answers in process, whatever their address: the
// body of /_dev/map.json, as body gives it at the time, and 404 for anything else.
func serve(body func() string) *http.Client {
	return &http.Client{Transport: handlerTransport(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/_dev/map.json" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body()))
	})}
}

// handlerTransport answers each request with the handler.
type handlerTransport http.HandlerFunc

func (h handlerTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	rec := httptest.NewRecorder()
	h(rec, r)
	return rec.Result(), nil
}

// execDB runs stmts on the database at path, creating it.
func execDB(t *testing.T, path string, stmts ...string) {
	t.Helper()
	db, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	for _, s := range stmts {
		if _, err := db.Exec(s); err != nil {
			t.Fatal(err)
		}
	}
}

// runAll is run, also running each command of a batch.
func runAll(m *root, msg tea.Msg) {
	if b, ok := msg.(tea.BatchMsg); ok {
		for _, cmd := range b {
			if cmd != nil {
				if next := cmd(); next != nil {
					runAll(m, next)
				}
			}
		}
		return
	}
	_, cmd := m.Update(msg)
	if cmd == nil {
		return
	}
	if next := cmd(); next != nil {
		if _, ok := next.(tea.QuitMsg); !ok {
			runAll(m, next)
		}
	}
}

func dataScreenOf(m *root) *dataScreen { return m.screens[2].(*dataScreen) }

func TestData_Screen(t *testing.T) {
	o, _ := dataApp(t, strings.Replace(shelfMap, "%s", "", 1))
	m := newRoot(o)
	press(m, "3")
	frame := m.View()
	for _, want := range []string{"> Books.Book    3", "  Lending.Loan  1", "0001_init.sql", "0002_loans.sql",
		"b1  Middlemarch  1871", "b3  Beloved      1987"} {
		if rowOf(frame, want) == "" {
			t.Errorf("no row %q in\n%s", want, frame)
		}
	}
	lines := strings.Split(frame, "\n")
	if !strings.HasSuffix(lines[len(lines)-1], "\x1b[2msqlite3 build/shelf/dev.db\x1b[0m") {
		t.Errorf("footer = %q", lines[len(lines)-1])
	}
	teatest.RequireEqualOutput(t, []byte(frame))

	// j selects the loans; enter moves into the rows, s sorts by the column under the cursor,
	// and enter shows the row vertically.
	press(m, "j")
	if f := m.View(); rowOf(f, "l1  b2    NULL") == "" {
		t.Errorf("the loans' rows are not shown:\n%s", f)
	}
	press(m, "k", "enter", "l", "s")
	if f := m.View(); !strings.Contains(rowOf(f, "│ > "), "Beloved") || rowOf(f, "by title") == "" {
		t.Errorf("s on title did not sort by it:\n%s", f)
	}
	press(m, "s")
	if f := m.View(); !strings.Contains(rowOf(f, "│ > "), "Middlemarch") || rowOf(f, "by title desc") == "" {
		t.Errorf("s again did not sort descending:\n%s", f)
	}
	press(m, "enter")
	f := ansi.Strip(m.View())
	for _, want := range []string{"title  Middlemarch", "year   1871"} {
		if rowOf(f, want) == "" {
			t.Errorf("the row view has no %q:\n%s", want, f)
		}
	}
	press(m, "esc", "esc")
	if f := m.View(); rowOf(f, "> Books.Book") == "" {
		t.Errorf("esc twice did not go back to the tables:\n%s", f)
	}
}

func TestData_Prompt(t *testing.T) {
	o, _ := dataApp(t, strings.Replace(shelfMap, "%s", "", 1))
	m := newRoot(o)
	press(m, "3", ":", "delete from x")
	lines := strings.Split(m.View(), "\n")
	if want := "\x1b[2msqlite3 build/shelf/dev.db 'delete from x'\x1b[0m"; !strings.HasSuffix(lines[len(lines)-1], want) {
		t.Errorf("footer with the prompt = %q, want the SQL", lines[len(lines)-1])
	}
	// q is typed, not the frame's quit.
	press(m, "q")
	run(m, tea.KeyMsg{Type: tea.KeyBackspace})
	press(m, "enter")
	frame := m.View()
	want := "\x1b[1;31monly select, with and explain run here (the database is opened read-only)\x1b[0m"
	if !strings.Contains(frame, want) {
		t.Errorf("no guard message in\n%s", frame)
	}
	teatest.RequireEqualOutput(t, []byte(frame))

	// esc closes the refused statement; a select runs and shows its rows; esc goes back to the
	// table. (: on a query's result starts from its statement.)
	press(m, "esc", ":", "select title, year from Books__Book where year > 1900 order by year", "enter")
	frame = m.View()
	for _, want := range []string{"Dune     1965", "Beloved  1987", "2 rows"} {
		if rowOf(frame, want) == "" {
			t.Errorf("no %q in the query's result:\n%s", want, frame)
		}
	}
	press(m, "esc")
	if rowOf(m.View(), "Middlemarch") == "" {
		t.Errorf("esc did not go back to the table:\n%s", m.View())
	}
}

func TestData_ErrorRow(t *testing.T) {
	o, _ := dataApp(t, strings.Replace(shelfMap, "%s", goneTable, 1))
	m := newRoot(o)
	press(m, "3")
	frame := m.View()
	if r := rowOf(frame, "Books.Shelf"); !strings.Contains(r, "\x1b[1;31mno such table") {
		t.Errorf("the missing table's row is %q, want its error in red", r)
	}
	for _, want := range []string{"Books.Book    3", "Lending.Loan  1"} {
		if rowOf(frame, want) == "" {
			t.Errorf("the other tables are not counted: no %q in\n%s", want, frame)
		}
	}
	// Selecting it shows the error in the rows pane; the screen keeps working.
	press(m, "j", "j")
	if !strings.Contains(m.View(), "\x1b[1;31mno such table: Books__Shelf") {
		t.Errorf("the rows pane does not show the error:\n%s", m.View())
	}
	press(m, "k")
	if rowOf(m.View(), "l1  b2") == "" {
		t.Errorf("after the error the loans do not show:\n%s", m.View())
	}
	teatest.RequireEqualOutput(t, []byte(frame))
}

// write is a debug access record of a request with method that ran stmts statements.
func write(seq int64, method string, stmts int) feed.Event {
	e := access(seq, clock, "5f3a2c99", "/books")
	r := *e.Log.Record
	r.Method, r.DBStmts, r.Msg = method, stmts, method+" /books"
	e.Log = &feed.Log{Record: &r}
	return e
}

func TestData_WroteRefresh(t *testing.T) {
	o, path := dataApp(t, strings.Replace(shelfMap, "%s", "", 1))
	m := newRoot(o)
	var fired []tea.Msg
	var waits []time.Duration
	dataScreenOf(m).after = func(d time.Duration, msg tea.Msg) tea.Cmd {
		waits, fired = append(waits, d), append(fired, msg)
		return nil
	}
	press(m, "3")
	execDB(t, path, `INSERT INTO "Books__Book" VALUES ('b4', 'Emma', 1815)`)

	// A GET that ran statements, and a PUT that ran none, did not write.
	runAll(m, eventMsg(write(20, "GET", 3)))
	runAll(m, eventMsg(write(21, "PUT", 0)))
	if len(fired) != 0 {
		t.Fatalf("a request that did not write scheduled a refresh: %v", fired)
	}
	// A DELETE that ran statements wrote; so did a POST, whatever it ran. The count changes
	// once the debounce has passed, and only the last of the two refreshes runs.
	runAll(m, eventMsg(write(22, "DELETE", 2)))
	runAll(m, eventMsg(write(23, "POST", 0)))
	if len(fired) != 2 || waits[0] != 500*time.Millisecond {
		t.Fatalf("refreshes scheduled: %v after %v", fired, waits)
	}
	if rowOf(m.View(), "Books.Book    3") == "" {
		t.Fatalf("the count changed before the debounce:\n%s", m.View())
	}
	runAll(m, fired[0])
	if rowOf(m.View(), "Books.Book    3") == "" {
		t.Errorf("a superseded refresh ran:\n%s", m.View())
	}
	runAll(m, fired[1])
	if rowOf(m.View(), "Books.Book    4") == "" {
		t.Errorf("the count did not refresh after the debounce:\n%s", m.View())
	}
}

// The map is fetched again on a build that succeeds, and the tables follow it.
func TestData_RefreshOnBuild(t *testing.T) {
	served := strings.Replace(shelfMap, "%s", "", 1)
	o, _ := dataApp(t, "")
	o.HTTP = serve(func() string { return served })
	m := newRoot(o)
	press(m, "3")
	served = strings.Replace(shelfMap, "%s", goneTable, 1)
	runAll(m, eventMsg(buildEvent("building")))
	if rowOf(m.View(), "Books.Shelf") != "" {
		t.Fatal("a build that has started fetched the map")
	}
	runAll(m, eventMsg(buildEvent("ok")))
	if rowOf(m.View(), "Books.Shelf") == "" {
		t.Errorf("a build ok did not fetch the map:\n%s", m.View())
	}
}

// Without a dev server the screen says why it has no tables, and does not crash.
func TestData_NoMap(t *testing.T) {
	o := options(nil)
	o.Addr = "127.0.0.1:1"
	m := newRoot(o)
	press(m, "3", "j", "enter", "s", "]", "[")
	if f := m.View(); rowOf(f, "no map") == "" {
		t.Errorf("no reason shown:\n%s", f)
	}
}

// The row view unfolds a long value over as many lines as it takes, keeping its line breaks.
func TestData_RowUnfold(t *testing.T) {
	o, path := dataApp(t, strings.Replace(shelfMap, "%s", "", 1))
	long := strings.Repeat("overdue ", 30) + "end\nsecond line"
	execDB(t, path, `UPDATE "Lending__Loan" SET note = '`+long+`'`)
	m := newRoot(o)
	press(m, "3", "j", "enter", "enter")
	f := ansi.Strip(m.View())
	if !strings.Contains(f, "end") || rowOf(f, "second line") == "" || strings.Contains(f, "…") {
		t.Errorf("the note is not unfolded:\n%s", f)
	}
	if n := strings.Count(f, "overdue"); n != 30 {
		t.Errorf("the row view shows %d of the 30 words", n)
	}
}

// An app with silos shows the database above the tables; d moves to the next one, which lists
// the silo's tables only, and the footer names its file.
func TestData_Silos(t *testing.T) {
	siloed := `,{"name":"Issues.Issue","sql":"Issues__Issue","columns":[],"cols":[` +
		`{"name":"id","type":"Id Issue","sql":"id","sqlType":"TEXT"}],"id":"id","silo":"org"}`
	mp := strings.Replace(strings.Replace(shelfMap, "%s", siloed, 1), `"session":[]`,
		`"session":[],"silos":[{"key":"org","ref":"Orgs.Org","tables":["Issues.Issue"],"path":"silos/org/{value}.db"}]`, 1)
	o, path := dataApp(t, mp)
	silo := filepath.Join(filepath.Dir(path), "silos", "org", "c4.db")
	if err := os.MkdirAll(filepath.Dir(silo), 0o755); err != nil {
		t.Fatal(err)
	}
	execDB(t, silo, `CREATE TABLE "Issues__Issue" ("id" TEXT PRIMARY KEY)`, `INSERT INTO "Issues__Issue" VALUES ('i1'), ('i2')`)
	m := newRoot(o)
	m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	press(m, "3")
	f := ansi.Strip(m.View())
	if rowOf(f, "database shared") == "" || rowOf(f, "Issues.Issue") != "" || rowOf(f, "Books.Book    3") == "" {
		t.Errorf("the shared database's view:\n%s", f)
	}
	press(m, "d")
	f = ansi.Strip(m.View())
	if rowOf(f, "database org/c4") == "" || rowOf(f, "> Issues.Issue  2") == "" || rowOf(f, "Books.Book") != "" {
		t.Errorf("the silo's view:\n%s", f)
	}
	lines := strings.Split(f, "\n")
	if len(lines) != 24 || !strings.HasSuffix(lines[23], "sqlite3 build/shelf/silos/org/c4.db") {
		t.Errorf("%d lines, footer %q", len(lines), lines[len(lines)-1])
	}
	for i, l := range lines {
		if w := ansi.StringWidth(l); w > 80 {
			t.Errorf("line %d is %d wide: %q", i, w, l)
		}
	}
}
