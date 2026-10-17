package db

import (
	"bytes"
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

// A database on Turso (libSQL), reached over its HTTP protocol (Hrana over HTTP, version 2):
// ALDER_DB=libsql://<host> makes the shared store one, and the silos of an app deployed with a
// `Turso` database are its child databases (turso.go). The Store is the SQLite one: its
// database/sql handle is opened over the driver below instead of modernc's, so every statement,
// transaction, migration and check is the same code.
//
// The protocol surface used, all of it (anything else is refused with a sentence):
//
//   - POST <https://host>/v2/pipeline with `Authorization: Bearer <token>` and a body
//     {"baton": null | "<baton>", "requests": [...]}; the answer {"baton", "base_url",
//     "results": [{"type": "ok", "response": …} | {"type": "error", "error": {"message",
//     "code"}}]}.
//   - Requests of two types: "execute" {"stmt": {"sql", "args": [values], "want_rows": true}}
//     (positional arguments only: named arguments are refused) and "close".
//   - Values: {"type": "null"}, {"type": "integer", "value": "<decimal text>"}, {"type":
//     "float", "value": <number>}, {"type": "text", "value": "<text>"}, {"type": "blob",
//     "base64": "<base64>"}.
//   - One stream per database/sql connection (the baton carries it from one request to the
//     next; base_url, when the server gives one, is where the stream's next request goes):
//     its first request also runs PRAGMA foreign_keys = ON (and, for a reader, PRAGMA
//     query_only = 1, whose failure is ignored: the Store refuses a reader's writes itself). A
//     transaction is BEGIN IMMEDIATE (BEGIN for a reader) … COMMIT or ROLLBACK on that stream,
//     one request per statement, so a Task's Go code runs between its statements as it does on
//     a file.
//   - A stream the server dropped (an idle one expires) is driver.ErrBadConn outside a
//     transaction (database/sql then takes another connection; nothing ran) and an error
//     inside one (the transaction is gone with it). Idle connections are closed after
//     hranaIdle, before the server's expiry.
//   - A 401 asks the token source for a fresh token and sends the request once more.
//
// "batch", "sequence", "describe", "store_sql" and the cursor endpoint are not used.

// hranaIdle is how long a connection (a stream) stays open unused: below the server's expiry of
// an idle stream (10 s on Turso).
const hranaIdle = 5 * time.Second

// TokenSource is the auth token of a database: a fresh one (not the cached) when refresh.
type TokenSource func(ctx context.Context, refresh bool) (string, error)

// IsRemote reports whether path names a database on Turso (libsql://host) instead of a file.
func IsRemote(path string) bool { return strings.HasPrefix(path, "libsql://") }

// hranaClient is one database's HTTP endpoint.
type hranaClient struct {
	base  string // https://host (no trailing slash)
	token TokenSource
	http  *http.Client
}

// hranaError is a statement the server refused: its message is SQLite's (a UNIQUE or FOREIGN
// KEY failure reads as it does on a file, which is what the Store's refusals look for).
type hranaError struct {
	Message string `json:"message"`
	Code    string `json:"code"`
}

func (e *hranaError) Error() string {
	if e.Code != "" && !strings.Contains(e.Message, e.Code) {
		return e.Message + " (" + e.Code + ")"
	}
	return e.Message
}

type hranaStmt struct {
	SQL      string `json:"sql"`
	Args     []any  `json:"args"`
	WantRows bool   `json:"want_rows"`
}

type hranaCol struct {
	Name     string  `json:"name"`
	Decltype *string `json:"decltype"`
}

type hranaResult struct {
	Cols             []hranaCol          `json:"cols"`
	Rows             [][]json.RawMessage `json:"rows"`
	AffectedRowCount int64               `json:"affected_row_count"`
	LastInsertRowid  *string             `json:"last_insert_rowid"`
}

type hranaStreamResult struct {
	Type     string `json:"type"`
	Response *struct {
		Type   string       `json:"type"`
		Result *hranaResult `json:"result"`
	} `json:"response"`
	Error *hranaError `json:"error"`
}

type hranaPipelineResp struct {
	Baton   *string             `json:"baton"`
	BaseURL *string             `json:"base_url"`
	Results []hranaStreamResult `json:"results"`
}

// errStreamGone is a request on a stream the server no longer has.
var errStreamGone = errors.New("hrana: the stream expired")

// pipeline sends requests on the stream baton ("" opens one) at url and returns the answer.
func (c *hranaClient) pipeline(ctx context.Context, url, baton string, reqs []any) (*hranaPipelineResp, error) {
	body := map[string]any{"baton": nil, "requests": reqs}
	if baton != "" {
		body["baton"] = baton
	}
	b, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	for attempt := 0; ; attempt++ {
		tok, err := c.token(ctx, attempt > 0)
		if err != nil {
			return nil, err
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, url+"/v2/pipeline", bytes.NewReader(b))
		if err != nil {
			return nil, err
		}
		req.Header.Set("Content-Type", "application/json")
		if tok != "" {
			req.Header.Set("Authorization", "Bearer "+tok)
		}
		res, err := c.http.Do(req)
		if err != nil {
			return nil, fmt.Errorf("db: %s: %w", url, err)
		}
		data, err := io.ReadAll(io.LimitReader(res.Body, 256<<20))
		res.Body.Close()
		if err != nil {
			return nil, fmt.Errorf("db: %s: %w", url, err)
		}
		switch {
		case res.StatusCode == http.StatusUnauthorized && attempt == 0:
			continue // a token the server no longer takes: a fresh one, once
		case res.StatusCode == http.StatusOK:
			var out hranaPipelineResp
			if err := json.Unmarshal(data, &out); err != nil {
				return nil, fmt.Errorf("db: %s answered something that is not a pipeline result: %v", url, err)
			}
			if len(out.Results) != len(reqs) {
				return nil, fmt.Errorf("db: %s answered %d results for %d requests", url, len(out.Results), len(reqs))
			}
			return &out, nil
		case baton != "" && res.StatusCode >= 400 && res.StatusCode < 500 && res.StatusCode != http.StatusUnauthorized:
			return nil, fmt.Errorf("%w (%s: %d %s)", errStreamGone, url, res.StatusCode, strings.TrimSpace(firstLineOf(string(data))))
		}
		return nil, fmt.Errorf("db: %s: %d %s", url, res.StatusCode, strings.TrimSpace(firstLineOf(string(data))))
	}
}

func firstLineOf(s string) string {
	l, _, _ := strings.Cut(s, "\n")
	if len(l) > 300 {
		l = l[:300]
	}
	return l
}

// hranaValue encodes a database/sql argument as a protocol value.
func hranaValue(v any) (any, error) {
	switch x := v.(type) {
	case nil:
		return map[string]any{"type": "null"}, nil
	case int64:
		return map[string]any{"type": "integer", "value": strconv.FormatInt(x, 10)}, nil
	case int:
		return map[string]any{"type": "integer", "value": strconv.Itoa(x)}, nil
	case bool:
		n := "0"
		if x {
			n = "1"
		}
		return map[string]any{"type": "integer", "value": n}, nil
	case float64:
		return map[string]any{"type": "float", "value": x}, nil
	case string:
		return map[string]any{"type": "text", "value": x}, nil
	case []byte:
		return map[string]any{"type": "blob", "base64": base64.StdEncoding.EncodeToString(x)}, nil
	case time.Time:
		return map[string]any{"type": "text", "value": x.UTC().Format(time.RFC3339Nano)}, nil
	}
	return nil, fmt.Errorf("db: a %T cannot be sent to a Turso database", v)
}

// decodeValue is a protocol value as a database/sql value: int64, float64, string, []byte or
// nil, what the SQLite driver gives for the same column.
func decodeValue(raw json.RawMessage) (driver.Value, error) {
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
		var s string
		if err := json.Unmarshal(v.Value, &s); err != nil {
			var n int64 // a number, from a server that sends one
			if err2 := json.Unmarshal(v.Value, &n); err2 != nil {
				return nil, err
			}
			return n, nil
		}
		return strconv.ParseInt(s, 10, 64)
	case "float":
		var f float64
		err := json.Unmarshal(v.Value, &f)
		return f, err
	case "text":
		var s string
		err := json.Unmarshal(v.Value, &s)
		return s, err
	case "blob":
		return base64.StdEncoding.DecodeString(strings.TrimRight(v.Base64, "="))
	}
	return nil, fmt.Errorf("db: a value of type %q", v.Type)
}

// ---- the database/sql driver

// openHrana is a database/sql handle on the database at base (https://host) whose connections
// are streams; reader opens them with PRAGMA query_only.
func openHrana(base string, token TokenSource, hc *http.Client, reader bool) *sql.DB {
	if hc == nil {
		hc = &http.Client{Timeout: 60 * time.Second}
	}
	d := sql.OpenDB(hranaConnector{c: &hranaClient{base: strings.TrimRight(base, "/"), token: token, http: hc}, reader: reader})
	d.SetConnMaxIdleTime(hranaIdle)
	d.SetMaxIdleConns(4)
	d.SetMaxOpenConns(16)
	return d
}

type hranaConnector struct {
	c      *hranaClient
	reader bool
}

func (h hranaConnector) Connect(context.Context) (driver.Conn, error) {
	return &hranaConn{c: h.c, url: h.c.base, reader: h.reader}, nil
}

func (h hranaConnector) Driver() driver.Driver { return hranaDriver{} }

type hranaDriver struct{}

func (hranaDriver) Open(string) (driver.Conn, error) {
	return nil, errors.New("db: the Turso driver opens through its connector (db.OpenSQLite with a libsql:// URL)")
}

// hranaConn is one stream: opened by its first request, carried by its baton.
type hranaConn struct {
	c      *hranaClient
	url    string
	reader bool
	baton  string
	opened bool
	inTx   bool
	bad    bool

	mu sync.Mutex
}

var (
	_ driver.ExecerContext      = (*hranaConn)(nil)
	_ driver.QueryerContext     = (*hranaConn)(nil)
	_ driver.ConnBeginTx        = (*hranaConn)(nil)
	_ driver.SessionResetter    = (*hranaConn)(nil)
	_ driver.Validator          = (*hranaConn)(nil)
	_ driver.NamedValueChecker  = (*hranaConn)(nil)
	_ driver.ConnPrepareContext = (*hranaConn)(nil)
)

// run executes one statement on the stream (opening it first when needed) and returns its
// result.
func (c *hranaConn) run(ctx context.Context, query string, args []driver.NamedValue) (*hranaResult, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.bad {
		return nil, driver.ErrBadConn
	}
	vals := make([]any, len(args))
	for i, a := range args {
		if a.Name != "" {
			return nil, fmt.Errorf("db: named argument %q: alder's Turso driver sends positional arguments only", a.Name)
		}
		v, err := hranaValue(a.Value)
		if err != nil {
			return nil, err
		}
		vals[i] = v
	}
	var reqs []any
	if !c.opened {
		reqs = pragmaReqs(c.reader)
	}
	pragmas := len(reqs)
	reqs = append(reqs, map[string]any{"type": "execute", "stmt": hranaStmt{SQL: query, Args: vals, WantRows: true}})
	resp, err := c.c.pipeline(ctx, c.url, c.baton, reqs)
	if errors.Is(err, errStreamGone) && !c.inTx {
		// the server dropped the idle stream: nothing ran; a new one (its pragmas again) takes
		// the statement
		c.baton, c.opened, c.url = "", false, c.c.base
		reqs = append(pragmaReqs(c.reader), reqs[pragmas:]...)
		pragmas = len(reqs) - 1
		resp, err = c.c.pipeline(ctx, c.url, "", reqs)
	}
	if err != nil {
		if errors.Is(err, errStreamGone) {
			c.bad = true
			return nil, fmt.Errorf("db: the transaction's stream on %s expired: %w", c.c.base, err)
		}
		if !c.opened {
			c.bad = true // nothing is known about the stream: never reuse it
		}
		return nil, err
	}
	c.opened = true
	if resp.Baton != nil {
		c.baton = *resp.Baton
	} else {
		c.baton, c.bad = "", true // the server closed the stream
	}
	if resp.BaseURL != nil && *resp.BaseURL != "" {
		c.url = strings.TrimRight(*resp.BaseURL, "/")
	}
	if pragmas > 0 {
		if r := resp.Results[0]; r.Type != "ok" {
			c.bad = true
			return nil, fmt.Errorf("db: PRAGMA foreign_keys = ON on %s: %v", c.c.base, r.Error)
		}
	}
	r := resp.Results[pragmas]
	if r.Type != "ok" || r.Response == nil || r.Response.Result == nil {
		if r.Error != nil {
			return nil, r.Error
		}
		return nil, fmt.Errorf("db: %s answered %q to a statement", c.c.base, r.Type)
	}
	return r.Response.Result, nil
}

// pragmaReqs are the requests that open a stream: foreign keys on, and a reader's query_only.
func pragmaReqs(reader bool) []any {
	reqs := []any{map[string]any{"type": "execute", "stmt": hranaStmt{SQL: "PRAGMA foreign_keys = ON", Args: []any{}}}}
	if reader {
		reqs = append(reqs, map[string]any{"type": "execute", "stmt": hranaStmt{SQL: "PRAGMA query_only = 1", Args: []any{}}})
	}
	return reqs
}

func (c *hranaConn) CheckNamedValue(nv *driver.NamedValue) error {
	switch v := nv.Value.(type) {
	case nil, int64, float64, bool, string, []byte, time.Time:
		return nil
	case int:
		nv.Value = int64(v)
		return nil
	case int32:
		nv.Value = int64(v)
		return nil
	}
	var err error
	nv.Value, err = driver.DefaultParameterConverter.ConvertValue(nv.Value)
	return err
}

func (c *hranaConn) ExecContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Result, error) {
	r, err := c.run(ctx, query, args)
	if err != nil {
		return nil, err
	}
	res := hranaExecResult{affected: r.AffectedRowCount}
	if r.LastInsertRowid != nil {
		res.lastID, _ = strconv.ParseInt(*r.LastInsertRowid, 10, 64)
	}
	return res, nil
}

func (c *hranaConn) QueryContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
	r, err := c.run(ctx, query, args)
	if err != nil {
		return nil, err
	}
	return &hranaRows{res: r}, nil
}

func (c *hranaConn) PrepareContext(_ context.Context, query string) (driver.Stmt, error) {
	return &hranaPrepared{c: c, query: query}, nil
}

func (c *hranaConn) Prepare(query string) (driver.Stmt, error) {
	return c.PrepareContext(context.Background(), query)
}

func (c *hranaConn) Begin() (driver.Tx, error) {
	return c.BeginTx(context.Background(), driver.TxOptions{})
}

func (c *hranaConn) BeginTx(ctx context.Context, opts driver.TxOptions) (driver.Tx, error) {
	if opts.Isolation != 0 {
		return nil, errors.New("db: a Turso transaction has SQLite's isolation only")
	}
	stmt := "BEGIN IMMEDIATE"
	if c.reader || opts.ReadOnly {
		stmt = "BEGIN"
	}
	if _, err := c.run(ctx, stmt, nil); err != nil {
		return nil, err
	}
	c.inTx = true
	return &hranaTx{c: c}, nil
}

func (c *hranaConn) ResetSession(context.Context) error {
	if c.bad {
		return driver.ErrBadConn
	}
	return nil
}

func (c *hranaConn) IsValid() bool { return !c.bad }

// Close closes the stream (best effort: an unanswered close expires at the server).
func (c *hranaConn) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.baton != "" {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		c.c.pipeline(ctx, c.url, c.baton, []any{map[string]any{"type": "close"}})
		cancel()
		c.baton = ""
	}
	c.bad = true
	return nil
}

type hranaTx struct{ c *hranaConn }

func (t *hranaTx) Commit() error   { return t.end("COMMIT") }
func (t *hranaTx) Rollback() error { return t.end("ROLLBACK") }

func (t *hranaTx) end(stmt string) error {
	_, err := t.c.run(context.Background(), stmt, nil)
	t.c.inTx = false
	if err != nil && stmt == "ROLLBACK" && strings.Contains(err.Error(), "no transaction is active") {
		return nil // SQLite rolled it back already (a failed COMMIT, say)
	}
	return err
}

type hranaPrepared struct {
	c     *hranaConn
	query string
}

func (s *hranaPrepared) Close() error  { return nil }
func (s *hranaPrepared) NumInput() int { return -1 }

func (s *hranaPrepared) Exec(args []driver.Value) (driver.Result, error) {
	return s.c.ExecContext(context.Background(), s.query, named(args))
}

func (s *hranaPrepared) Query(args []driver.Value) (driver.Rows, error) {
	return s.c.QueryContext(context.Background(), s.query, named(args))
}

func (s *hranaPrepared) ExecContext(ctx context.Context, args []driver.NamedValue) (driver.Result, error) {
	return s.c.ExecContext(ctx, s.query, args)
}

func (s *hranaPrepared) QueryContext(ctx context.Context, args []driver.NamedValue) (driver.Rows, error) {
	return s.c.QueryContext(ctx, s.query, args)
}

func named(args []driver.Value) []driver.NamedValue {
	out := make([]driver.NamedValue, len(args))
	for i, a := range args {
		out[i] = driver.NamedValue{Ordinal: i + 1, Value: a}
	}
	return out
}

type hranaExecResult struct{ affected, lastID int64 }

func (r hranaExecResult) LastInsertId() (int64, error) { return r.lastID, nil }
func (r hranaExecResult) RowsAffected() (int64, error) { return r.affected, nil }

type hranaRows struct {
	res *hranaResult
	i   int
}

func (r *hranaRows) Columns() []string {
	out := make([]string, len(r.res.Cols))
	for i, c := range r.res.Cols {
		out[i] = c.Name
	}
	return out
}

func (r *hranaRows) Close() error { return nil }

func (r *hranaRows) Next(dest []driver.Value) error {
	if r.i >= len(r.res.Rows) {
		return io.EOF
	}
	row := r.res.Rows[r.i]
	r.i++
	for i := range dest {
		if i >= len(row) {
			dest[i] = nil
			continue
		}
		v, err := decodeValue(row[i])
		if err != nil {
			return err
		}
		dest[i] = v
	}
	return nil
}
