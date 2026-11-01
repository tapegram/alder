package applog

import (
	"bytes"
	"encoding/json"
	"strings"
	"time"
)

// record is the JSON line a Logger writes, the keys Render reads; any other key is ignored.
type record struct {
	Time   time.Time `json:"time"`
	Level  string    `json:"level"`
	Msg    *string   `json:"msg"`
	Method *string   `json:"method"`
	Path   *string   `json:"path"`
	Status *int      `json:"status"`
	Ms     *float64  `json:"ms"`
	Bytes  *int64    `json:"bytes"`
	Rid    *string   `json:"rid"`
	Steps  *int      `json:"steps"`
	Slow   *bool     `json:"slow"`
	Route  *string   `json:"route"`
	DBStmt *int      `json:"db_stmts"`
	DBMs   *float64  `json:"db_ms"`
}

// Render is the text line a Logger at level min writes for the JSON record in line, newline
// included: what `alder run` shows of a child that logs JSON. ok is false when line is not a
// record this package writes (not a JSON object, or without time, level and msg), and the caller
// passes the line through. ok with a nil text is a record below min, which is dropped.
func Render(line []byte, min Level) (text []byte, ok bool) {
	line = bytes.TrimSpace(line)
	if len(line) == 0 || line[0] != '{' {
		return nil, false
	}
	var r record
	if json.Unmarshal(line, &r) != nil || r.Time.IsZero() || r.Msg == nil {
		return nil, false
	}
	var lv Level
	switch r.Level {
	case "DEBUG":
		lv = Debug
	case "INFO":
		lv = Info
	case "WARN":
		lv = Warn
	case "ERROR":
		lv = Error
	default:
		return nil, false
	}
	if lv < min {
		return nil, true
	}
	b := r.Time.UTC().AppendFormat(nil, stampLayout)
	b = append(b, ' ')
	b = append(b, levelNames[lv]...)
	b = append(b, ": "...)
	if r.Rid != nil && r.Status != nil && r.Method != nil && r.Path != nil {
		a := accessText{Method: *r.Method, Path: *r.Path, Status: *r.Status, ID: *r.Rid, Debug: min == Debug}
		if r.Ms != nil {
			a.Ms = *r.Ms
		}
		if r.Bytes != nil {
			a.Bytes = *r.Bytes
		}
		if r.Steps != nil {
			a.Steps = *r.Steps
		}
		if r.Slow != nil {
			a.Slow = *r.Slow
		}
		if r.Route != nil {
			a.Route = *r.Route
		}
		if r.DBStmt != nil {
			a.Stmts = *r.DBStmt
		}
		if r.DBMs != nil {
			a.DBMs = *r.DBMs
		}
		b = appendAccess(b, &a)
	} else {
		b = appendText(b, strings.TrimRight(*r.Msg, "\r\n"))
	}
	if b[len(b)-1] != '\n' {
		b = append(b, '\n')
	}
	return b, true
}
