package db

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"
)

// Turso's platform API (api.turso.tech), over net/http: what an app deployed with a `Turso`
// database (chapter 16) needs at run time — its silos' databases, created on first use, and
// an auth token per database — and what `alder deploy` needs from the deploying machine.
//
// The calls, all of them (paths under /v1/organizations/{org}):
//
//	GET    /groups/{group}                          the group exists (deploy's preflight)
//	GET    /databases?group={group}                 the silos' databases (by their names' prefix)
//	GET    /databases/{name}                        one database (its Hostname); 404: none
//	POST   /databases                               create: {"name", "group"}, and for a copy
//	                                                at a point in time (a restore) {"seed":
//	                                                {"type": "database", "name", "timestamp"}}
//	DELETE /databases/{name}                        `alder silos prune`, a restore's swap
//	GET    /databases/{name}/usage                  storage_bytes: the deploy samples the largest
//	POST   /databases/{name}/auth/tokens?authorization=full-access   {"jwt"}: a database's token
//
// Every request carries `Authorization: Bearer $TURSO_API_TOKEN`.
//
// Names. The shared database is the declaration's app ("blog"); a silo's database is
// "<app>-<key>-<value>", its value written as its
// silo file is named (SiloFileName: lowercase base32hex, no padding): Turso's names are
// lowercase letters, digits and dashes, and an id's text (base64url) has capitals and `_`.

// The runtime's Turso settings, read from the environment (the Ssh target's run script sets them
// for a `database: Turso "org" { … }` environment; TURSO_API_TOKEN is the operator's, in the
// environment file).
const (
	EnvTursoAPI   = "ALDER_TURSO_API" // the platform API's base URL (default DefaultTursoAPI); a fake's in the tests
	EnvTursoOrg   = "ALDER_TURSO_ORG"
	EnvTursoGroup = "ALDER_TURSO_GROUP"
	EnvTursoApp   = "ALDER_TURSO_APP"
	EnvTursoDB    = "ALDER_TURSO_DB" // the shared database's name (its token is minted for it)
	EnvTursoToken = "TURSO_API_TOKEN"
	EnvDBToken    = "ALDER_DB_TOKEN" // a token for ALDER_DB itself, instead of one minted with the API token
)

// DefaultTursoAPI is the platform API's address.
const DefaultTursoAPI = "https://api.turso.tech"

// MaxTursoName is the longest database name Turso takes.
const MaxTursoName = 64

// TursoConfig is how to reach an organization's databases.
type TursoConfig struct {
	API, Org, Group, App, Token string
	HTTP                        *http.Client // nil: a client with a 60 s timeout
}

// TursoConfigFromEnv is the runtime's TursoConfig (its Org empty when ALDER_TURSO_ORG is unset).
func TursoConfigFromEnv() TursoConfig {
	c := TursoConfig{API: os.Getenv(EnvTursoAPI), Org: os.Getenv(EnvTursoOrg), Group: os.Getenv(EnvTursoGroup), App: os.Getenv(EnvTursoApp), Token: os.Getenv(EnvTursoToken)}
	if c.API == "" {
		c.API = DefaultTursoAPI
	}
	if c.Group == "" {
		c.Group = "default"
	}
	return c
}

// Turso is a client of the platform API: it creates and finds databases, mints and caches
// their tokens, and opens them as Stores.
type Turso struct {
	cfg  TursoConfig
	http *http.Client

	mu     sync.Mutex
	tokens map[string]string // by database name
	hosts  map[string]string // by database name
}

// TursoDB is a database as the API describes it.
type TursoDB struct {
	Name     string `json:"Name"`
	ID       string `json:"DbId"`
	Hostname string `json:"Hostname"`
	Group    string `json:"group"`
}

// NewTurso is a client for cfg.
func NewTurso(cfg TursoConfig) *Turso {
	hc := cfg.HTTP
	if hc == nil {
		hc = &http.Client{Timeout: 60 * time.Second}
	}
	if cfg.API == "" {
		cfg.API = DefaultTursoAPI
	}
	cfg.API = strings.TrimRight(cfg.API, "/")
	return &Turso{cfg: cfg, http: hc, tokens: map[string]string{}, hosts: map[string]string{}}
}

// Config is the client's configuration.
func (t *Turso) Config() TursoConfig { return t.cfg }

// TursoError is an answer of the platform API other than success.
type TursoError struct {
	Method, Path string
	Status       int
	Message      string
}

func (e *TursoError) Error() string {
	return fmt.Sprintf("the Turso API answered %s %s with %d: %s", e.Method, e.Path, e.Status, e.Message)
}

// IsTursoNotFound reports whether err is the API's 404.
func IsTursoNotFound(err error) bool {
	var te *TursoError
	return errors.As(err, &te) && te.Status == http.StatusNotFound
}

// call sends method path (under the organization) with body (nil for none) and decodes the
// answer into out (nil: ignored).
func (t *Turso) call(ctx context.Context, method, path string, body, out any) error {
	if t.cfg.Org == "" {
		return errors.New("db: Turso: no organization (ALDER_TURSO_ORG)")
	}
	if t.cfg.Token == "" {
		return fmt.Errorf("db: Turso: %s is not set: the platform API needs it", EnvTursoToken)
	}
	full := "/v1/organizations/" + url.PathEscape(t.cfg.Org) + path
	var rd io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return err
		}
		rd = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, t.cfg.API+full, rd)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+t.cfg.Token)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	res, err := t.http.Do(req)
	if err != nil {
		return fmt.Errorf("db: the Turso API: %w", err)
	}
	defer res.Body.Close()
	data, err := io.ReadAll(io.LimitReader(res.Body, 64<<20))
	if err != nil {
		return err
	}
	if res.StatusCode/100 != 2 {
		msg := strings.TrimSpace(firstLineOf(string(data)))
		var e struct {
			Error string `json:"error"`
		}
		if json.Unmarshal(data, &e) == nil && e.Error != "" {
			msg = e.Error
		}
		return &TursoError{Method: method, Path: full, Status: res.StatusCode, Message: msg}
	}
	if out == nil {
		return nil
	}
	if err := json.Unmarshal(data, out); err != nil {
		return fmt.Errorf("db: the Turso API's answer to %s %s does not read: %v", method, full, err)
	}
	return nil
}

// GroupExists reports whether the organization has group name.
func (t *Turso) GroupExists(ctx context.Context, name string) (bool, error) {
	err := t.call(ctx, http.MethodGet, "/groups/"+url.PathEscape(name), nil, nil)
	if IsTursoNotFound(err) {
		return false, nil
	}
	return err == nil, err
}

// Database is database name; found false when it does not exist.
func (t *Turso) Database(ctx context.Context, name string) (TursoDB, bool, error) {
	var out struct {
		Database TursoDB `json:"database"`
	}
	err := t.call(ctx, http.MethodGet, "/databases/"+url.PathEscape(name), nil, &out)
	if IsTursoNotFound(err) {
		return TursoDB{}, false, nil
	}
	if err != nil {
		return TursoDB{}, false, err
	}
	t.remember(out.Database)
	return out.Database, true, nil
}

// Children are the databases of silo key of app (their names begin "<app>-<key>-" and decode
// to an id: ChildValue), sorted by name. The list is the organization's databases (the group's,
// when the client has one), filtered here.
func (t *Turso) Children(ctx context.Context, app, key string) ([]TursoDB, error) {
	path := "/databases"
	if t.cfg.Group != "" {
		path += "?group=" + url.QueryEscape(t.cfg.Group)
	}
	var out struct {
		Databases []TursoDB `json:"databases"`
	}
	if err := t.call(ctx, http.MethodGet, path, nil, &out); err != nil {
		return nil, err
	}
	var kids []TursoDB
	for _, d := range out.Databases {
		if _, ok := ChildValue(app, key, d.Name); ok {
			kids = append(kids, d)
			t.remember(d)
		}
	}
	sortDBs(kids)
	return kids, nil
}

func sortDBs(ds []TursoDB) {
	for i := 1; i < len(ds); i++ {
		for j := i; j > 0 && ds[j].Name < ds[j-1].Name; j-- {
			ds[j], ds[j-1] = ds[j-1], ds[j]
		}
	}
}

// TursoSeed is a new database's content: a copy of database Name as it was at Timestamp (the
// zero time: now).
type TursoSeed struct {
	Name      string
	Timestamp time.Time
}

// CreateOptions describe a new database.
type CreateOptions struct {
	Name, Group string
	Seed        *TursoSeed // a copy of another database (point-in-time restore)
}

// Create creates a database.
func (t *Turso) Create(ctx context.Context, o CreateOptions) (TursoDB, error) {
	if err := TursoName(o.Name); err != nil {
		return TursoDB{}, err
	}
	body := map[string]any{"name": o.Name, "group": o.Group}
	if o.Group == "" {
		body["group"] = t.cfg.Group
	}
	if o.Seed != nil {
		seed := map[string]any{"type": "database", "name": o.Seed.Name}
		if !o.Seed.Timestamp.IsZero() {
			seed["timestamp"] = o.Seed.Timestamp.UTC().Format(time.RFC3339)
		}
		body["seed"] = seed
	}
	var out struct {
		Database TursoDB `json:"database"`
	}
	if err := t.call(ctx, http.MethodPost, "/databases", body, &out); err != nil {
		return TursoDB{}, err
	}
	d := out.Database
	if d.Name == "" {
		d.Name = o.Name
	}
	t.remember(d)
	return d, nil
}

// Delete deletes database name (its cached token and host are forgotten).
func (t *Turso) Delete(ctx context.Context, name string) error {
	err := t.call(ctx, http.MethodDelete, "/databases/"+url.PathEscape(name), nil, nil)
	t.mu.Lock()
	delete(t.tokens, name)
	delete(t.hosts, name)
	t.mu.Unlock()
	return err
}

// StorageBytes is database name's size as the API reports it.
func (t *Turso) StorageBytes(ctx context.Context, name string) (int64, error) {
	var out struct {
		Database struct {
			Usage struct {
				StorageBytes int64 `json:"storage_bytes"`
			} `json:"usage"`
		} `json:"database"`
	}
	err := t.call(ctx, http.MethodGet, "/databases/"+url.PathEscape(name)+"/usage", nil, &out)
	return out.Database.Usage.StorageBytes, err
}

// Token is database name's auth token, minted once and cached (refresh: a new one).
func (t *Turso) Token(ctx context.Context, name string, refresh bool) (string, error) {
	t.mu.Lock()
	tok, ok := t.tokens[name]
	t.mu.Unlock()
	if ok && !refresh {
		return tok, nil
	}
	var out struct {
		JWT string `json:"jwt"`
	}
	if err := t.call(ctx, http.MethodPost, "/databases/"+url.PathEscape(name)+"/auth/tokens?authorization=full-access", nil, &out); err != nil {
		return "", err
	}
	t.mu.Lock()
	t.tokens[name] = out.JWT
	t.mu.Unlock()
	return out.JWT, nil
}

func (t *Turso) remember(d TursoDB) {
	if d.Hostname == "" {
		return
	}
	t.mu.Lock()
	t.hosts[d.Name] = d.Hostname
	t.mu.Unlock()
}

// URL is the HTTP address of a database at hostname: https, or http when the API itself is
// reached over http (a fake in the tests).
func (t *Turso) URL(hostname string) string {
	scheme := "https://"
	if strings.HasPrefix(t.cfg.API, "http://") {
		scheme = "http://"
	}
	return scheme + strings.TrimPrefix(strings.TrimPrefix(hostname, "libsql://"), "https://")
}

// Open is database name as a Store (reader: a read-only one), its token minted with the API
// token.
func (t *Turso) Open(ctx context.Context, name string, reader bool) (*SQLite, error) {
	t.mu.Lock()
	host := t.hosts[name]
	t.mu.Unlock()
	if host == "" {
		d, ok, err := t.Database(ctx, name)
		if err != nil {
			return nil, err
		}
		if !ok {
			return nil, fmt.Errorf("db: Turso has no database %s in organization %s", name, t.cfg.Org)
		}
		host = d.Hostname
	}
	tok := func(ctx context.Context, refresh bool) (string, error) { return t.Token(ctx, name, refresh) }
	return &SQLite{db: openHrana(t.URL(host), tok, t.http, reader), tables: map[string]TableDesc{}, remote: true}, nil
}

var tursoNameChars = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]*[a-z0-9])?$`)

// TursoName refuses a name Turso does not take, with a sentence.
func TursoName(name string) error {
	switch {
	case len(name) > MaxTursoName:
		return fmt.Errorf("db: the Turso database name %s is %d characters, more than Turso's %d: use a shorter app name in the declaration", name, len(name), MaxTursoName)
	case !tursoNameChars.MatchString(name):
		return fmt.Errorf("db: %q is not a Turso database name (lowercase letters, digits and `-`, not at either end)", name)
	}
	return nil
}

// ChildName is the database of silo key's value: "<app>-<key>-<the value as its silo file is
// named, without .db>" (SiloFileName: lowercase base32hex, so two ids that differ in case are
// two databases).
func ChildName(app, key, value string) string {
	return app + "-" + strings.ToLower(key) + "-" + strings.TrimSuffix(SiloFileName(value), ".db")
}

// ChildValue is the silo value database name holds (ok false for a name that is not one of
// key's children).
func ChildValue(app, key, name string) (string, bool) {
	enc, ok := strings.CutPrefix(name, app+"-"+strings.ToLower(key)+"-")
	if !ok {
		return "", false
	}
	return SiloValue(enc + ".db")
}

// openRemote is ALDER_DB=libsql://host as a Store: its token is ALDER_DB_TOKEN, or minted
// with TURSO_API_TOKEN for the database ALDER_TURSO_DB names.
func openRemote(path string, reader bool) (*SQLite, error) {
	host := strings.TrimPrefix(path, "libsql://")
	if host == "" || strings.ContainsAny(host, "?#") {
		return nil, fmt.Errorf("db: ALDER_DB %q: write libsql://<host>", path)
	}
	cfg := TursoConfigFromEnv()
	t := NewTurso(cfg)
	var tok TokenSource
	switch {
	case os.Getenv(EnvDBToken) != "":
		fixed := os.Getenv(EnvDBToken)
		tok = func(context.Context, bool) (string, error) { return fixed, nil }
	case cfg.Token != "" && cfg.Org != "" && os.Getenv(EnvTursoDB) != "":
		name := os.Getenv(EnvTursoDB)
		tok = func(ctx context.Context, refresh bool) (string, error) { return t.Token(ctx, name, refresh) }
	default:
		return nil, fmt.Errorf("db: ALDER_DB is a Turso database (%s) but there is no token for it: set %s, or %s, %s and %s", path, EnvDBToken, EnvTursoToken, EnvTursoOrg, EnvTursoDB)
	}
	return &SQLite{db: openHrana(t.URL(host), tok, t.http, reader), tables: map[string]TableDesc{}, remote: true}, nil
}

// ExecRaw runs one statement on st outside the Store's operations (alder's deploy tools and
// their tests).
func ExecRaw(ctx context.Context, st *SQLite, stmt string, args ...any) error {
	_, err := st.db.ExecContext(ctx, stmt, args...)
	return err
}
