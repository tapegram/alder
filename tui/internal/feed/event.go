// Package feed reads an alder dev server's event feed, GET /_dev/events: the event types, the
// decoding of one event's JSON (an SSE data line, or a line of an NDJSON fixture), and the client
// that keeps a stream open across dev server restarts.
package feed

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

// Event is one event of the feed: its type, its time T and its seq (counting from 1 since the dev
// server started), with the payload of its type. A static event has no payload, nor does an event
// of a type this package does not know. The client adds two synthetic types, without a seq:
// "disconnected" when the stream drops, and "gap" (with Gap) when a reconnect finds that the dev
// server no longer keeps some of the events the client has not seen.
type Event struct {
	Type  string
	T     time.Time
	Seq   int64
	Hello *Hello
	Build *Build
	App   *App
	Log   *Log
	Run   *Run
	Gap   *Gap
}

// Hello is the first event of every stream: the dev server's app, its directory, alder's version,
// the dev server's address and the child's (nil while no child runs), the generation, the build
// state, when the dev server started, its boot nonce (another boot is another dev server) and the
// seq of the oldest event it keeps. A hello's own seq is the dev server's current one.
type Hello struct {
	App, Dir, Alder, Addr, Gen, Build, Boot string
	Child                                   *string
	Since                                   time.Time
	First                                   int64
}

// Build is a build event: its state (building, ok or failed), generation, the new child's address
// (ok), the failure's text when it has no diagnostics, the check's diagnostics (failed) or
// warnings (ok), and the build's milliseconds (ok).
type Build struct {
	State, Gen, Child, Text string
	Diagnostics, Warnings   []Diag
	Ms                      float64
}

// Diag is one diagnostic as `alder check --json` prints it.
type Diag struct {
	Code     string `json:"code"`
	Severity string `json:"severity"`
	File     string `json:"file"`
	Line     int    `json:"line"`
	Col      int    `json:"col"`
	EndLine  int    `json:"endLine"`
	EndCol   int    `json:"endCol"`
	Message  string `json:"message"`
}

// App is an app event: the child started (with its address) or exited (with its exit status and
// the last lines of its stderr).
type App struct {
	State, Child string
	Exit         *Exit
	Tail         []string
}

// Exit is an exited child's status: its code (-1 when a signal ended it) and the signal's name.
type Exit struct {
	Code   int    `json:"code"`
	Signal string `json:"signal"`
}

// Log is a log event: one line of the child's stderr, as its JSON log record when it is one,
// otherwise as Raw.
type Log struct {
	Raw    string
	Record *Record
}

// Record is a JSON log record of the app. Level is the record's own name (DEBUG, INFO, WARN or
// ERROR). The access fields are zero when the record does not carry them; HasAccess is true
// when it carries rid, status, method and path, the record of one answered request.
type Record struct {
	Time                            time.Time
	Level, Msg, Rid, Method, Path   string
	Route                           string
	Status, Steps, DBStmts, Repeats int
	Ms, DBMs                        float64
	Bytes                           int64
	Slow                            bool
	HasAccess                       bool
}

// Gap is a gap event: the client reconnected to the same dev server, but the oldest event its
// ring keeps comes after the next one the client lacks, so Missed events are lost (the ring
// overflowed while the client was away).
type Gap struct {
	Missed uint64
}

// Run is a run event: one of the dev server's own status lines.
type Run struct {
	Text string
}

// wire is every field an event of any type may carry, as the dev server writes them.
type wire struct {
	Type string    `json:"type"`
	T    time.Time `json:"t"`
	Seq  int64     `json:"seq"`

	App   string    `json:"app"`
	Dir   string    `json:"dir"`
	Alder string    `json:"alder"`
	Addr  string    `json:"addr"`
	Child *string   `json:"child"`
	Gen   string    `json:"gen"`
	Build string    `json:"build"`
	Since time.Time `json:"since"`
	Boot  string    `json:"boot"`
	First int64     `json:"first"`

	State       string   `json:"state"`
	Text        string   `json:"text"`
	Diagnostics []Diag   `json:"diagnostics"`
	Warnings    []Diag   `json:"warnings"`
	Ms          float64  `json:"ms"`
	Exit        *Exit    `json:"exit"`
	Tail        []string `json:"tail"`

	Record json.RawMessage `json:"record"`
	Raw    string          `json:"raw"`
}

// record is a log record's keys this package reads; a pointer tells a missing key from a zero.
type record struct {
	Time    time.Time `json:"time"`
	Level   string    `json:"level"`
	Msg     string    `json:"msg"`
	Rid     *string   `json:"rid"`
	Method  *string   `json:"method"`
	Path    *string   `json:"path"`
	Status  *int      `json:"status"`
	Route   string    `json:"route"`
	Ms      float64   `json:"ms"`
	Bytes   int64     `json:"bytes"`
	Steps   int       `json:"steps"`
	DBStmts int       `json:"db_stmts"`
	DBMs    float64   `json:"db_ms"`
	Repeats int       `json:"repeats"`
	Slow    bool      `json:"slow"`
}

// Decode is the event whose JSON is line. It fails when line is not a JSON object with a type. A
// log event whose record does not decode (not an object, or a key of the wrong type) is kept as
// a raw line, the record's JSON, with no Record.
func Decode(line []byte) (Event, error) {
	var w wire
	if err := json.Unmarshal(line, &w); err != nil {
		return Event{}, fmt.Errorf("feed: %w", err)
	}
	if w.Type == "" {
		return Event{}, errors.New("feed: an event without a type")
	}
	ev := Event{Type: w.Type, T: w.T, Seq: w.Seq}
	child := ""
	if w.Child != nil {
		child = *w.Child
	}
	switch w.Type {
	case "hello":
		ev.Hello = &Hello{App: w.App, Dir: w.Dir, Alder: w.Alder, Addr: w.Addr, Gen: w.Gen, Build: w.Build,
			Boot: w.Boot, Child: w.Child, Since: w.Since, First: w.First}
	case "build":
		ev.Build = &Build{State: w.State, Gen: w.Gen, Child: child, Text: w.Text, Diagnostics: w.Diagnostics,
			Warnings: w.Warnings, Ms: w.Ms}
	case "app":
		ev.App = &App{State: w.State, Child: child, Exit: w.Exit, Tail: w.Tail}
	case "run":
		ev.Run = &Run{Text: w.Text}
	case "log":
		ev.Log = &Log{Raw: w.Raw}
		if len(w.Record) > 0 && string(w.Record) != "null" {
			var r record
			if err := json.Unmarshal(w.Record, &r); err != nil {
				if ev.Log.Raw == "" {
					ev.Log.Raw = string(w.Record)
				}
				return ev, nil
			}
			ev.Log.Record = &Record{Time: r.Time, Level: r.Level, Msg: r.Msg, Rid: deref(r.Rid),
				Method: deref(r.Method), Path: deref(r.Path), Route: r.Route, Ms: r.Ms, Bytes: r.Bytes,
				Steps: r.Steps, DBStmts: r.DBStmts, DBMs: r.DBMs, Repeats: r.Repeats, Slow: r.Slow,
				HasAccess: r.Rid != nil && r.Status != nil && r.Method != nil && r.Path != nil}
			if r.Status != nil {
				ev.Log.Record.Status = *r.Status
			}
		}
	}
	return ev, nil
}

// deref is *p, or "" when p is nil.
func deref(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}
