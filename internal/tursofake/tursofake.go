// Package tursofake is a fake Turso for the tests: the platform API (the calls rt/db's Turso
// client makes) and the database HTTP protocol (Hrana over HTTP v2: execute and close on
// streams carried by batons), served by net/http/httptest, each database an SQLite file in a
// temporary directory.
//
// It models what alder relies on: groups; databases, schema databases and their children; a
// schema database passing its committed schema changes (CREATE, ALTER, DROP) on to its
// children, at once or — for a child marked held, a straggler — when the test releases it;
// a new child taking its schema database's schema; auth tokens per database (a deleted and
// recreated database's old tokens are refused with 401); storage sizes; point-in-time copies
// (a snapshot after every committed write, a seed with a timestamp copies the last one at or
// before it); deletion. It is not Turso: no replicas, no limits, no other request types.
package tursofake

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

// Server is a running fake.
type Server struct {
	URL      string // http://127.0.0.1:<port>: the API's base and every database's host
	Org      string
	APIToken string

	srv *httptest.Server
	dir string

	mu      sync.Mutex
	now     func() time.Time
	groups  map[string]bool
	dbs     map[string]*database
	gen     int
	held    map[string]bool // children whose schema changes wait (stragglers)
	calls   []string
	failAPI map[string]int // "METHOD /path-suffix" → the status to answer once
}

type database struct {
	name, group, schema string
	isSchema            bool
	path                string
	db                  *sql.DB
	gen                 int
	tokens              map[string]bool
	streams             map[string]*stream
	snaps               []snapshot
	ddl                 []string // a schema database's committed schema changes, in order
	pending             []string // a held child's changes not applied yet
}

type snapshot struct {
	at   time.Time
	path string
}

type stream struct {
	mu   sync.Mutex
	conn *sql.Conn
	inTx bool
	ddl  []string // a schema database's changes inside the open transaction
}

// New starts a fake for org with group "default"; it is closed when the test ends.
func New(t testing.TB, org string) *Server {
	t.Helper()
	s := &Server{Org: org, APIToken: "api-" + random(8), dir: t.TempDir(), now: time.Now,
		groups: map[string]bool{"default": true}, dbs: map[string]*database{}, held: map[string]bool{}, failAPI: map[string]int{}}
	s.srv = httptest.NewServer(http.HandlerFunc(s.serve))
	s.URL = s.srv.URL
	t.Cleanup(s.Close)
	return s
}

// Close stops the server and closes every database.
func (s *Server) Close() {
	s.srv.Close()
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, d := range s.dbs {
		d.close()
	}
}

// SetNow sets the fake's clock (the snapshots' times).
func (s *Server) SetNow(now func() time.Time) {
	s.mu.Lock()
	s.now = now
	s.mu.Unlock()
}

// AddGroup adds a group.
func (s *Server) AddGroup(name string) {
	s.mu.Lock()
	s.groups[name] = true
	s.mu.Unlock()
}

// Hold makes child name a straggler: its schema database's changes wait until Release.
func (s *Server) Hold(name string) {
	s.mu.Lock()
	s.held[name] = true
	s.mu.Unlock()
}

// Release applies the changes child name was held from, and stops holding it.
func (s *Server) Release(name string) error {
	s.mu.Lock()
	delete(s.held, name)
	d := s.dbs[name]
	var pending []string
	if d != nil {
		pending, d.pending = d.pending, nil
	}
	s.mu.Unlock()
	if d == nil {
		return fmt.Errorf("no database %s", name)
	}
	return applyDDL(d.db, pending)
}

// ExpireStreams drops every open stream (an idle stream expires at Turso): the next request on
// one is answered 400.
func (s *Server) ExpireStreams() {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, d := range s.dbs {
		for b, st := range d.streams {
			release(st.conn)
			delete(d.streams, b)
		}
	}
}

// FailNext makes the next API request whose method and path end as "POST /databases" answer
// status.
func (s *Server) FailNext(methodPath string, status int) {
	s.mu.Lock()
	s.failAPI[methodPath] = status
	s.mu.Unlock()
}

// Calls are the API requests served so far ("POST /databases {name}" …), in order.
func (s *Server) Calls() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string{}, s.calls...)
}

// Names are the databases, sorted.
func (s *Server) Names() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []string
	for n := range s.dbs {
		out = append(out, n)
	}
	sort.Strings(out)
	return out
}

// DB is database name's SQLite handle for the test's own checks (nil when it does not exist).
func (s *Server) DB(name string) *sql.DB {
	s.mu.Lock()
	defer s.mu.Unlock()
	if d := s.dbs[name]; d != nil {
		return d.db
	}
	return nil
}

// Exec runs stmt on database name directly (a change by hand), outside the protocol.
func (s *Server) Exec(name, stmt string, args ...any) error {
	d := s.DB(name)
	if d == nil {
		return fmt.Errorf("no database %s", name)
	}
	_, err := d.Exec(stmt, args...)
	return err
}

// Create creates a database as the API would (for a test's setup).
func (s *Server) Create(name, group, schema string, isSchema bool) error {
	if _, st := s.create(name, group, schema, isSchema, nil); st != nil {
		return fmt.Errorf("%d %s", st.code, st.msg)
	}
	return nil
}

// release gives a stream's connection back to the pool without what the stream set on it.
func release(conn *sql.Conn) {
	ctx := context.Background()
	conn.ExecContext(ctx, "ROLLBACK")
	conn.ExecContext(ctx, "PRAGMA query_only = 0")
	conn.Close()
}

func random(n int) string {
	b := make([]byte, n)
	rand.Read(b)
	return hex.EncodeToString(b)
}

func (d *database) close() {
	for _, st := range d.streams {
		st.conn.Close()
	}
	d.streams = nil
	if d.db != nil {
		d.db.Close()
	}
}

func openFile(path string) (*sql.DB, error) {
	return sql.Open("sqlite", path+"?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)")
}

// ---- the API

func (s *Server) serve(w http.ResponseWriter, r *http.Request) {
	if rest, ok := strings.CutPrefix(r.URL.Path, "/db/"); ok {
		name, tail, _ := strings.Cut(rest, "/")
		if tail != "v2/pipeline" || r.Method != http.MethodPost {
			http.Error(w, "the fake serves POST /v2/pipeline only", http.StatusNotFound)
			return
		}
		s.pipeline(w, r, name)
		return
	}
	prefix := "/v1/organizations/" + s.Org
	rest, ok := strings.CutPrefix(r.URL.Path, prefix)
	if !ok {
		apiError(w, http.StatusNotFound, "no such organization")
		return
	}
	if r.Header.Get("Authorization") != "Bearer "+s.APIToken {
		apiError(w, http.StatusUnauthorized, "bad API token")
		return
	}
	s.mu.Lock()
	s.calls = append(s.calls, r.Method+" "+rest)
	for k, status := range s.failAPI {
		if strings.HasPrefix(k, r.Method+" ") && strings.HasSuffix(rest, strings.TrimPrefix(k, r.Method+" ")) {
			delete(s.failAPI, k)
			s.mu.Unlock()
			apiError(w, status, "injected failure")
			return
		}
	}
	s.mu.Unlock()
	parts := strings.Split(strings.Trim(rest, "/"), "/")
	switch {
	case len(parts) == 2 && parts[0] == "groups" && r.Method == http.MethodGet:
		s.mu.Lock()
		ok := s.groups[parts[1]]
		s.mu.Unlock()
		if !ok {
			apiError(w, http.StatusNotFound, "group not found")
			return
		}
		writeJSON(w, map[string]any{"group": map[string]any{"name": parts[1]}})
	case len(parts) == 1 && parts[0] == "databases" && r.Method == http.MethodGet:
		parent := r.URL.Query().Get("schema")
		s.mu.Lock()
		var list []map[string]any
		for _, d := range s.dbs {
			if parent == "" || d.schema == parent {
				list = append(list, s.describe(d))
			}
		}
		s.mu.Unlock()
		sort.Slice(list, func(i, j int) bool { return list[i]["Name"].(string) < list[j]["Name"].(string) })
		writeJSON(w, map[string]any{"databases": list})
	case len(parts) == 1 && parts[0] == "databases" && r.Method == http.MethodPost:
		var body struct {
			Name     string `json:"name"`
			Group    string `json:"group"`
			IsSchema bool   `json:"is_schema"`
			Schema   string `json:"schema"`
			Seed     *struct {
				Type      string `json:"type"`
				Name      string `json:"name"`
				Timestamp string `json:"timestamp"`
			} `json:"seed"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			apiError(w, http.StatusBadRequest, err.Error())
			return
		}
		var seed *seedSpec
		if body.Seed != nil {
			if body.Seed.Type != "database" {
				apiError(w, http.StatusBadRequest, "the fake seeds from a database only")
				return
			}
			seed = &seedSpec{name: body.Seed.Name}
			if body.Seed.Timestamp != "" {
				t, err := time.Parse(time.RFC3339, body.Seed.Timestamp)
				if err != nil {
					apiError(w, http.StatusBadRequest, "bad timestamp")
					return
				}
				seed.at = t
			}
		}
		d, status := s.create(body.Name, body.Group, body.Schema, body.IsSchema, seed)
		if status != nil {
			apiError(w, status.code, status.msg)
			return
		}
		s.mu.Lock()
		desc := s.describe(d)
		s.mu.Unlock()
		writeJSON(w, map[string]any{"database": desc})
	case len(parts) == 2 && parts[0] == "databases" && r.Method == http.MethodGet:
		s.mu.Lock()
		d := s.dbs[parts[1]]
		var desc map[string]any
		if d != nil {
			desc = s.describe(d)
		}
		s.mu.Unlock()
		if d == nil {
			apiError(w, http.StatusNotFound, "database not found")
			return
		}
		writeJSON(w, map[string]any{"database": desc})
	case len(parts) == 2 && parts[0] == "databases" && r.Method == http.MethodDelete:
		s.mu.Lock()
		d := s.dbs[parts[1]]
		delete(s.dbs, parts[1])
		s.mu.Unlock()
		if d == nil {
			apiError(w, http.StatusNotFound, "database not found")
			return
		}
		d.close()
		os.Remove(d.path)
		os.Remove(d.path + "-wal")
		os.Remove(d.path + "-shm")
		writeJSON(w, map[string]any{"database": parts[1]})
	case len(parts) == 3 && parts[0] == "databases" && parts[2] == "usage" && r.Method == http.MethodGet:
		s.mu.Lock()
		d := s.dbs[parts[1]]
		s.mu.Unlock()
		if d == nil {
			apiError(w, http.StatusNotFound, "database not found")
			return
		}
		var n int64
		for _, x := range []string{"", "-wal"} {
			if fi, err := os.Stat(d.path + x); err == nil {
				n += fi.Size()
			}
		}
		writeJSON(w, map[string]any{"database": map[string]any{"uuid": d.name, "usage": map[string]any{"rows_read": 0, "rows_written": 0, "storage_bytes": n}}})
	case len(parts) == 4 && parts[0] == "databases" && parts[2] == "auth" && parts[3] == "tokens" && r.Method == http.MethodPost:
		s.mu.Lock()
		d := s.dbs[parts[1]]
		tok := ""
		if d != nil {
			tok = "tok-" + d.name + "-" + strconv.Itoa(d.gen) + "-" + random(4)
			d.tokens[tok] = true
		}
		s.mu.Unlock()
		if d == nil {
			apiError(w, http.StatusNotFound, "database not found")
			return
		}
		writeJSON(w, map[string]any{"jwt": tok})
	default:
		apiError(w, http.StatusNotFound, "the fake does not serve "+r.Method+" "+rest)
	}
}

type seedSpec struct {
	name string
	at   time.Time
}

type apiStatus struct {
	code int
	msg  string
}

// create makes database name: a schema database, a child of schema (its schema database's
// changes applied), or a copy of a seed.
func (s *Server) create(name, group, schema string, isSchema bool, seed *seedSpec) (*database, *apiStatus) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if name == "" {
		return nil, &apiStatus{http.StatusBadRequest, "a database needs a name"}
	}
	if s.dbs[name] != nil {
		return nil, &apiStatus{http.StatusConflict, "database " + name + " already exists"}
	}
	if group == "" {
		group = "default"
	}
	if !s.groups[group] {
		return nil, &apiStatus{http.StatusNotFound, "group " + group + " not found"}
	}
	var parent *database
	if schema != "" {
		if parent = s.dbs[schema]; parent == nil || !parent.isSchema {
			return nil, &apiStatus{http.StatusBadRequest, "schema database " + schema + " not found"}
		}
	}
	s.gen++
	d := &database{name: name, group: group, schema: schema, isSchema: isSchema, gen: s.gen, tokens: map[string]bool{}, streams: map[string]*stream{},
		path: filepath.Join(s.dir, name+"-"+strconv.Itoa(s.gen)+".db")}
	if seed != nil {
		src := s.dbs[seed.name]
		if src == nil {
			return nil, &apiStatus{http.StatusBadRequest, "seed database " + seed.name + " not found"}
		}
		from := ""
		if seed.at.IsZero() {
			if err := vacuumInto(src.db, d.path); err != nil {
				return nil, &apiStatus{http.StatusInternalServerError, err.Error()}
			}
		} else {
			for _, sn := range src.snaps {
				if !sn.at.After(seed.at) {
					from = sn.path
				}
			}
			if from == "" {
				return nil, &apiStatus{http.StatusBadRequest, "no point in time at or before " + seed.at.UTC().Format(time.RFC3339) + " for " + seed.name}
			}
			b, err := os.ReadFile(from)
			if err == nil {
				err = os.WriteFile(d.path, b, 0o600)
			}
			if err != nil {
				return nil, &apiStatus{http.StatusInternalServerError, err.Error()}
			}
		}
	}
	db, err := openFile(d.path)
	if err != nil {
		return nil, &apiStatus{http.StatusInternalServerError, err.Error()}
	}
	d.db = db
	if parent != nil && seed == nil {
		if err := applyDDL(db, parent.ddl); err != nil {
			db.Close()
			return nil, &apiStatus{http.StatusInternalServerError, err.Error()}
		}
	}
	s.dbs[name] = d
	s.snapshotLocked(d)
	return d, nil
}

func (s *Server) describe(d *database) map[string]any {
	host := strings.TrimPrefix(s.URL, "http://") + "/db/" + d.name
	return map[string]any{"Name": d.name, "DbId": "id-" + d.name + "-" + strconv.Itoa(d.gen), "Hostname": host, "group": d.group, "schema": d.schema, "is_schema": d.isSchema}
}

func vacuumInto(db *sql.DB, path string) error {
	_, err := db.Exec("VACUUM INTO ?", path)
	return err
}

// snapshotLocked records database d as it is now (a point in time a seed can copy).
func (s *Server) snapshotLocked(d *database) {
	p := d.path + ".snap" + strconv.Itoa(len(d.snaps))
	if err := vacuumInto(d.db, p); err == nil {
		d.snaps = append(d.snaps, snapshot{at: s.now(), path: p})
	}
}

func applyDDL(db *sql.DB, stmts []string) error {
	if len(stmts) == 0 {
		return nil
	}
	ctx := context.Background()
	conn, err := db.Conn(ctx)
	if err != nil {
		return err
	}
	defer conn.Close()
	if _, err := conn.ExecContext(ctx, "PRAGMA foreign_keys = OFF"); err != nil {
		return err
	}
	for _, st := range stmts {
		if _, err := conn.ExecContext(ctx, st); err != nil {
			return fmt.Errorf("propagating %q: %v", st, err)
		}
	}
	return nil
}

func apiError(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}

// ---- the database protocol

func (s *Server) pipeline(w http.ResponseWriter, r *http.Request, name string) {
	s.mu.Lock()
	d := s.dbs[name]
	authorized := d != nil && d.tokens[strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")]
	s.mu.Unlock()
	if d == nil {
		http.Error(w, "no database "+name, http.StatusNotFound)
		return
	}
	if !authorized {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	var body struct {
		Baton    *string           `json:"baton"`
		Requests []json.RawMessage `json:"requests"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	var st *stream
	baton := ""
	if body.Baton != nil {
		baton = *body.Baton
		s.mu.Lock()
		st = d.streams[baton]
		s.mu.Unlock()
		if st == nil {
			http.Error(w, "stream not found (expired)", http.StatusBadRequest)
			return
		}
	} else {
		conn, err := d.db.Conn(context.Background())
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		// a new stream is a new connection: what an earlier stream set on this pooled one goes
		for _, q := range []string{"PRAGMA query_only = 0", "PRAGMA foreign_keys = OFF"} {
			conn.ExecContext(context.Background(), q)
		}
		st = &stream{conn: conn}
		baton = random(8)
		s.mu.Lock()
		if d.streams == nil { // deleted meanwhile
			s.mu.Unlock()
			conn.Close()
			http.Error(w, "no database "+name, http.StatusNotFound)
			return
		}
		d.streams[baton] = st
		s.mu.Unlock()
	}
	st.mu.Lock()
	defer st.mu.Unlock()
	var results []any
	closed := false
	for _, raw := range body.Requests {
		var req struct {
			Type string `json:"type"`
			Stmt *struct {
				SQL       string            `json:"sql"`
				Args      []json.RawMessage `json:"args"`
				NamedArgs []json.RawMessage `json:"named_args"`
				WantRows  bool              `json:"want_rows"`
			} `json:"stmt"`
		}
		if err := json.Unmarshal(raw, &req); err != nil {
			results = append(results, errResult(err.Error()))
			continue
		}
		switch {
		case req.Type == "close":
			release(st.conn)
			s.mu.Lock()
			delete(d.streams, baton)
			s.mu.Unlock()
			closed = true
			results = append(results, map[string]any{"type": "ok", "response": map[string]any{"type": "close"}})
		case req.Type == "execute" && req.Stmt != nil && len(req.Stmt.NamedArgs) == 0:
			args := make([]any, len(req.Stmt.Args))
			bad := ""
			for i, a := range req.Stmt.Args {
				v, err := decode(a)
				if err != nil {
					bad = err.Error()
				}
				args[i] = v
			}
			if bad != "" {
				results = append(results, errResult(bad))
				continue
			}
			res, err := s.execute(d, st, req.Stmt.SQL, args)
			if err != nil {
				results = append(results, errResult(err.Error()))
				continue
			}
			results = append(results, map[string]any{"type": "ok", "response": map[string]any{"type": "execute", "result": res}})
		default:
			results = append(results, errResult("the fake serves execute (positional arguments) and close only, not "+req.Type))
		}
	}
	out := map[string]any{"baton": baton, "base_url": nil, "results": results}
	if closed {
		out["baton"] = nil
	}
	writeJSON(w, out)
}

func errResult(msg string) map[string]any {
	return map[string]any{"type": "error", "error": map[string]any{"message": msg, "code": "SQLITE_ERROR"}}
}

func verb(q string) string {
	f := strings.Fields(strings.TrimSpace(q))
	if len(f) == 0 {
		return ""
	}
	return strings.ToUpper(f[0])
}

// execute runs q on stream st of database d: a schema database's schema changes are passed on
// to its children when they commit.
func (s *Server) execute(d *database, st *stream, q string, args []any) (map[string]any, error) {
	ctx := context.Background()
	v := verb(q)
	rowsWanted := v == "SELECT" || v == "WITH" || v == "VALUES" || v == "EXPLAIN" || (v == "PRAGMA" && !strings.Contains(q, "="))
	res := map[string]any{"cols": []any{}, "rows": []any{}, "affected_row_count": 0, "last_insert_rowid": nil}
	if rowsWanted {
		rows, err := st.conn.QueryContext(ctx, q, args...)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		names, _ := rows.Columns()
		cols := make([]any, len(names))
		for i, n := range names {
			cols[i] = map[string]any{"name": n, "decltype": nil}
		}
		var out []any
		for rows.Next() {
			vals := make([]any, len(names))
			ptrs := make([]any, len(names))
			for i := range vals {
				ptrs[i] = &vals[i]
			}
			if err := rows.Scan(ptrs...); err != nil {
				return nil, err
			}
			row := make([]any, len(vals))
			for i, x := range vals {
				row[i] = encode(x)
			}
			out = append(out, row)
		}
		if err := rows.Err(); err != nil {
			return nil, err
		}
		res["cols"] = cols
		if out != nil {
			res["rows"] = out
		}
		return res, nil
	}
	r, err := st.conn.ExecContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	n, _ := r.RowsAffected()
	id, _ := r.LastInsertId()
	res["affected_row_count"] = n
	res["last_insert_rowid"] = strconv.FormatInt(id, 10)
	schemaChange := v == "CREATE" || v == "ALTER" || v == "DROP"
	switch {
	case v == "BEGIN":
		st.inTx, st.ddl = true, nil
	case v == "COMMIT" || v == "END":
		st.inTx = false
		s.committed(d, st.ddl)
		st.ddl = nil
	case v == "ROLLBACK":
		st.inTx, st.ddl = false, nil
	case schemaChange && d.isSchema && st.inTx:
		st.ddl = append(st.ddl, q)
	case !st.inTx:
		var ddl []string
		if schemaChange && d.isSchema {
			ddl = []string{q}
		}
		s.committed(d, ddl)
	}
	return res, nil
}

// committed records a commit on d: a point in time, and a schema database's changes passed on.
func (s *Server) committed(d *database, ddl []string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.snapshotLocked(d)
	if !d.isSchema || len(ddl) == 0 {
		return
	}
	d.ddl = append(d.ddl, ddl...)
	for _, c := range s.dbs {
		if c.schema != d.name {
			continue
		}
		if s.held[c.name] {
			c.pending = append(c.pending, ddl...)
			continue
		}
		applyDDL(c.db, ddl)
	}
}

func decode(raw json.RawMessage) (any, error) {
	var v struct {
		Type   string          `json:"type"`
		Value  json.RawMessage `json:"value"`
		Base64 string          `json:"base64"`
	}
	if err := json.Unmarshal(raw, &v); err != nil {
		return nil, err
	}
	switch v.Type {
	case "null":
		return nil, nil
	case "integer":
		var t string
		if err := json.Unmarshal(v.Value, &t); err != nil {
			return nil, fmt.Errorf("an integer's value is a string of digits: %s", v.Value)
		}
		return strconv.ParseInt(t, 10, 64)
	case "float":
		var f float64
		err := json.Unmarshal(v.Value, &f)
		return f, err
	case "text":
		var t string
		err := json.Unmarshal(v.Value, &t)
		return t, err
	case "blob":
		return base64.StdEncoding.DecodeString(v.Base64)
	}
	return nil, fmt.Errorf("unknown value type %q", v.Type)
}

func encode(x any) map[string]any {
	switch v := x.(type) {
	case nil:
		return map[string]any{"type": "null"}
	case int64:
		return map[string]any{"type": "integer", "value": strconv.FormatInt(v, 10)}
	case float64:
		return map[string]any{"type": "float", "value": v}
	case string:
		return map[string]any{"type": "text", "value": v}
	case []byte:
		return map[string]any{"type": "blob", "base64": base64.StdEncoding.EncodeToString(v)}
	case bool:
		if v {
			return map[string]any{"type": "integer", "value": "1"}
		}
		return map[string]any{"type": "integer", "value": "0"}
	case time.Time:
		return map[string]any{"type": "text", "value": v.UTC().Format(time.RFC3339Nano)}
	}
	return map[string]any{"type": "text", "value": fmt.Sprint(x)}
}
