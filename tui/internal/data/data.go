// Package data reads the app's development database for the Data screen: the map of its tables
// that the app serves at /_dev/map.json, the database files (the shared one and, for an app with
// silos, one per silo value), and the tables' row counts, pages of rows and read-only queries.
// Every database is opened read-only with query_only set, so nothing here can write to it.
package data

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	_ "modernc.org/sqlite" // the "sqlite" driver
)

// PageRows is the number of rows a page holds; QueryRows is the most rows a query returns.
const (
	PageRows  = 100
	QueryRows = 1000
)

// maxMap is the most of /_dev/map.json that is read.
const maxMap = 16 << 20

// queryTimeout bounds a query typed at the prompt.
const queryTimeout = 5 * time.Second

// Map is what the Data screen reads of /_dev/map.json: the build id, the app's tables, its silos
// and the migrations applied.
type Map struct {
	Build      string
	Tables     []Table
	Silos      []Silo
	Migrations []string
}

// Table is one table: its Alder name (Module.Name), its SQL name, the key of its silo ("" for a
// shared table), its columns and the SQL name of its id column ("" when the map names none).
type Table struct {
	Name, SQL, Silo string
	ID              string
	Cols            []Col
}

// Col is one column: its Alder name and type, its SQL name and its SQL storage type.
type Col struct {
	Name, Type, SQL, SQLType string
}

// Silo is one silo: its key, the table its values are ids of, the path of one value's database
// relative to the shared database's directory ({value} stands for the value's file name), and
// the Alder names of its tables.
type Silo struct {
	Key, Ref, Path string
	Tables         []string
}

// wireMap is /_dev/map.json as the app writes it (rt/web's devMapView), the fields the Data
// screen reads.
type wireMap struct {
	Build string `json:"build"`
	Map   struct {
		Tables []struct {
			Name string `json:"name"`
			SQL  string `json:"sql"`
			ID   string `json:"id"`
			Silo string `json:"silo"`
			Cols []struct {
				Name    string `json:"name"`
				Type    string `json:"type"`
				SQL     string `json:"sql"`
				SQLType string `json:"sqlType"`
			} `json:"cols"`
		} `json:"tables"`
		Silos []struct {
			Key    string   `json:"key"`
			Ref    string   `json:"ref"`
			Tables []string `json:"tables"`
			Path   string   `json:"path"`
		} `json:"silos"`
	} `json:"map"`
	Migrations []string `json:"migrations"`
}

// ParseMap reads the body of /_dev/map.json.
func ParseMap(b []byte) (Map, error) {
	var w wireMap
	if err := json.Unmarshal(b, &w); err != nil {
		return Map{}, fmt.Errorf("reading the map: %w", err)
	}
	m := Map{Build: w.Build, Migrations: w.Migrations}
	for _, t := range w.Map.Tables {
		tb := Table{Name: t.Name, SQL: t.SQL, Silo: t.Silo, ID: t.ID, Cols: make([]Col, 0, len(t.Cols))}
		for _, c := range t.Cols {
			tb.Cols = append(tb.Cols, Col{Name: c.Name, Type: c.Type, SQL: c.SQL, SQLType: c.SQLType})
		}
		m.Tables = append(m.Tables, tb)
	}
	for _, s := range w.Map.Silos {
		m.Silos = append(m.Silos, Silo{Key: s.Key, Ref: s.Ref, Path: s.Path, Tables: s.Tables})
	}
	return m, nil
}

// FetchMap gets the map from the app at addr, GET http://<addr>/_dev/map.json.
func FetchMap(ctx context.Context, client *http.Client, addr string) (Map, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://"+addr+"/_dev/map.json", nil)
	if err != nil {
		return Map{}, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return Map{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return Map{}, fmt.Errorf("GET /_dev/map.json: %s", resp.Status)
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, maxMap))
	if err != nil {
		return Map{}, err
	}
	return ParseMap(b)
}

// Choice is one database the screen can browse: its label (shared, or <key>/<value> for a silo
// value), its file, and the key of its silo ("" for the shared database).
type Choice struct {
	Label, Path, Silo string
}

// Databases is the databases of the app built in buildDir: the shared one, dev.db, then for each
// of m's silos, in the map's order, one per database file of that silo, by file name. A silo's
// files are where the silo's path rule puts them (silos/<key>/{value}.db); the label shows the
// file's name without .db, which is the value's file name rather than its id.
func Databases(buildDir string, m Map) []Choice {
	out := []Choice{{Label: "shared", Path: filepath.Join(buildDir, "dev.db")}}
	for _, s := range m.Silos {
		rule := s.Path
		if !strings.Contains(rule, "{value}") {
			rule = "silos/" + s.Key + "/{value}.db"
		}
		dir, file := filepath.Split(filepath.Join(buildDir, filepath.FromSlash(rule)))
		before, after, _ := strings.Cut(file, "{value}")
		ents, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		var names []string
		for _, e := range ents {
			n := e.Name()
			if e.Type().IsRegular() && len(n) > len(before)+len(after) && strings.HasPrefix(n, before) && strings.HasSuffix(n, after) {
				names = append(names, n)
			}
		}
		slices.Sort(names)
		for _, n := range names {
			value := strings.TrimSuffix(strings.TrimPrefix(n, before), after)
			out = append(out, Choice{Label: s.Key + "/" + value, Path: filepath.Join(dir, n), Silo: s.Key})
		}
	}
	return out
}

// TablesFor is the tables of m that the database c holds: the siloed tables of c's silo for a
// silo database, the shared tables for the shared one. (The shared database also has the siloed
// tables, always empty, so they are left out.)
func (m Map) TablesFor(c Choice) []Table {
	var out []Table
	for _, t := range m.Tables {
		if t.Silo == c.Silo {
			out = append(out, t)
		}
	}
	return out
}

// DB is a database opened read-only.
type DB struct {
	db *sql.DB
}

// Open opens the database file at path read-only with query_only set, waiting at most 200 ms for
// a lock. A missing file is an error, not a new empty database.
func Open(path string) (*DB, error) {
	db, err := sql.Open("sqlite", "file:"+uriPath(path)+"?mode=ro&_pragma=query_only(1)&_pragma=busy_timeout(200)")
	if err != nil {
		return nil, err
	}
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, err
	}
	return &DB{db: db}, nil
}

// uriPath is path as the path of a file: URI: with forward slashes, and %, ? and # escaped so
// they stay part of the path.
func uriPath(path string) string {
	return strings.NewReplacer("%", "%25", "?", "%3f", "#", "%23").Replace(filepath.ToSlash(path))
}

// Close closes the database.
func (db *DB) Close() error { return db.db.Close() }

// Count is the number of rows of t.
func (db *DB) Count(ctx context.Context, t Table) (int64, error) {
	var n int64
	err := db.db.QueryRowContext(ctx, "SELECT count(*) FROM "+quote(t.SQL)).Scan(&n)
	return n, err
}

// Page is the page of t's rows from offset, PageRows of them, by the column orderBy (ascending, or
// descending when desc). orderBy must be the SQL name of one of t's columns; anything else orders
// by t's id column, or by rowid when the map names none.
func (db *DB) Page(ctx context.Context, t Table, orderBy string, desc bool, offset int) ([]string, [][]string, error) {
	by := "rowid"
	if t.ID != "" {
		by = quote(t.ID)
	}
	for _, c := range t.Cols {
		if c.SQL == orderBy && orderBy != "" {
			by = quote(orderBy)
		}
	}
	if desc {
		by += " DESC"
	}
	stmt := "SELECT * FROM " + quote(t.SQL) + " ORDER BY " + by + " LIMIT " + strconv.Itoa(PageRows) + " OFFSET " + strconv.Itoa(max(offset, 0))
	return db.rows(ctx, stmt, PageRows)
}

// Query runs stmt, which Guard must accept, and is its columns and at most QueryRows of its rows.
// It gives up after five seconds.
func (db *DB) Query(ctx context.Context, stmt string) ([]string, [][]string, error) {
	if err := Guard(stmt); err != nil {
		return nil, nil, err
	}
	ctx, cancel := context.WithTimeout(ctx, queryTimeout)
	defer cancel()
	return db.rows(ctx, stmt, QueryRows)
}

// rows runs stmt and is its columns and at most limit rows, each value as text: NULL as "NULL",
// a blob as its bytes.
func (db *DB) rows(ctx context.Context, stmt string, limit int) ([]string, [][]string, error) {
	rs, err := db.db.QueryContext(ctx, stmt)
	if err != nil {
		return nil, nil, err
	}
	defer rs.Close()
	cols, err := rs.Columns()
	if err != nil {
		return nil, nil, err
	}
	var out [][]string
	vals := make([]any, len(cols))
	ptrs := make([]any, len(cols))
	for i := range vals {
		ptrs[i] = &vals[i]
	}
	for len(out) < limit && rs.Next() {
		if err := rs.Scan(ptrs...); err != nil {
			return nil, nil, err
		}
		row := make([]string, len(cols))
		for i, v := range vals {
			row[i] = text(v)
		}
		out = append(out, row)
	}
	if err := rs.Err(); err != nil {
		return nil, nil, err
	}
	return cols, out, nil
}

// exec runs stmt without Guard. Only the tests use it, to show that SQLite itself refuses a
// write.
func (db *DB) exec(ctx context.Context, stmt string) error {
	_, err := db.db.ExecContext(ctx, stmt)
	return err
}

// text is a value SQLite returned, as text.
func text(v any) string {
	switch v := v.(type) {
	case nil:
		return "NULL"
	case []byte:
		return string(v)
	case string:
		return v
	case int64:
		return strconv.FormatInt(v, 10)
	case float64:
		return strconv.FormatFloat(v, 'g', -1, 64)
	case time.Time:
		return v.Format(time.RFC3339Nano)
	}
	return fmt.Sprint(v)
}

// Message is err as the screen shows it: without the driver's "SQL logic error: " in front and
// SQLite's result code, such as " (1)", at the end.
func Message(err error) string {
	msg := strings.TrimPrefix(err.Error(), "SQL logic error: ")
	if i := strings.LastIndex(msg, " ("); i > 0 && strings.HasSuffix(msg, ")") {
		if _, perr := strconv.Atoi(msg[i+2 : len(msg)-1]); perr == nil {
			msg = msg[:i]
		}
	}
	return msg
}

// quote makes a double-quoted SQL identifier.
func quote(name string) string { return `"` + strings.ReplaceAll(name, `"`, `""`) + `"` }
