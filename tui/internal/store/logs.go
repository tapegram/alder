// Package store holds what the TUI keeps of the dev server's feed: the log scrollback, the table
// of answered requests and the 60 s stats strip over them.
package store

import (
	"regexp"
	"strconv"
	"strings"
	"time"

	"alder/tui/internal/feed"
)

// stampLayout is a text line's stamp, as `alder run` writes it, in UTC.
const stampLayout = "2006/01/02 15:04:05"

// Line is one line of the log scrollback. Text is the line as `alder run` renders it: the stamp,
// the level, then the message, or for an access line the text logger's form. Level is the
// lower-case level name (debug, info, warn or error); a run line and a raw line are info. Effect
// marks an effect step, Run one of the dev server's own status lines, Access the line of an
// answered request and Gap the marker of events the feed lost (at level error, so that every
// level filter shows it). Rid is the request the line belongs to, if any.
type Line struct {
	Seq                 int64
	T                   time.Time
	Level               string
	Text                string
	Rid                 string
	Effect, Run, Access bool
	Gap                 bool

	rec *feed.Record // the record behind the line, to render an access line again at debug
}

// Filter narrows the scrollback; each field that is set must hold for a line. Level shows the
// lines at that level or above (debug, info, warn or error; empty shows everything), Rid the
// lines of one request and Re the lines whose text it matches. A level of debug shows an access
// line with its steps.
type Filter struct {
	Level string
	Rid   string
	Re    *regexp.Regexp
}

// Logs is the log scrollback: a ring of the newest lines. Each line added gets the next
// position, counted from 0 and never reused, even across Reset, so a reader can keep positions
// and read only the lines added since it last looked.
type Logs struct {
	ring  ring[Line]
	added int64 // the lines ever added; the position the next one gets
}

// NewLogs is an empty scrollback keeping the last max lines.
func NewLogs(max int) *Logs { return &Logs{ring: newRing[Line](max)} }

// Add appends the line of a log, run or gap event; any other event is ignored.
func (l *Logs) Add(e feed.Event) {
	if ln, ok := LineOf(e); ok {
		l.add(ln)
	}
}

// LineOf is the scrollback line of a log, run or gap event, and whether e is one; it needs no
// Logs, so a caller can render a single event.
func LineOf(e feed.Event) (Line, bool) {
	switch {
	case e.Type == "gap" && e.Gap != nil:
		return Line{Seq: e.Seq, T: e.T, Level: "error", Gap: true,
			Text: "… " + strconv.FormatUint(e.Gap.Missed, 10) + " events missed (ring overflow)"}, true
	case e.Type == "run" && e.Run != nil:
		return Line{Seq: e.Seq, T: e.T, Level: "info", Text: e.Run.Text, Run: true}, true
	case e.Type == "log" && e.Log != nil && e.Log.Record != nil:
		r := e.Log.Record
		lv := strings.ToLower(r.Level)
		ln := Line{Seq: e.Seq, T: r.Time, Level: lv, Rid: r.Rid, rec: r,
			Effect: lv == "debug" && strings.HasPrefix(r.Msg, "effect "), Access: r.HasAccess}
		ln.Text = renderText(r, false)
		return ln, true
	case e.Type == "log" && e.Log != nil:
		return Line{Seq: e.Seq, T: e.T, Level: "info", Text: e.Log.Raw}, true
	}
	return Line{}, false
}

func (l *Logs) add(ln Line) {
	l.ring.add(ln)
	l.added++
}

// View is the lines that pass f, oldest first.
func (l *Logs) View(f Filter) []Line {
	var out []Line
	for i := l.Start(); i < l.End(); i++ {
		if ln, ok := l.Get(i, f); ok {
			out = append(out, ln)
		}
	}
	return out
}

// Start is the position of the oldest line kept; End is the position the next line added gets.
// The lines kept are those in [Start, End).
func (l *Logs) Start() int64 { return l.added - int64(l.ring.n) }
func (l *Logs) End() int64   { return l.added }

// Get is the line at position i, which is in [Start, End), as f shows it, and whether it passes
// f. A level of debug shows an access line with its steps.
func (l *Logs) Get(i int64, f Filter) (Line, bool) {
	ln := l.ring.at(int(i - l.Start()))
	if levelRank(ln.Level) < levelRank(f.Level) || f.Rid != "" && ln.Rid != f.Rid {
		return Line{}, false
	}
	if f.Level == "debug" && ln.Access {
		ln.Text = renderText(ln.rec, true)
	}
	if f.Re != nil && !f.Re.MatchString(ln.Text) {
		return Line{}, false
	}
	return ln, true
}

// Reset empties the scrollback. Positions go on from where they were.
func (l *Logs) Reset() { l.ring.reset() }

// levelRank orders the levels: debug (and anything unknown, or empty) 0, info 1, warn 2, error 3.
func levelRank(s string) int {
	switch s {
	case "info":
		return 1
	case "warn":
		return 2
	case "error":
		return 3
	}
	return 0
}

// renderText is the text line `alder run` shows for r, without the line break. An access line
// ends with its steps only when debug is set and the request was not slow.
func renderText(r *feed.Record, debug bool) string {
	b := r.Time.UTC().AppendFormat(nil, stampLayout)
	b = append(b, ' ')
	b = append(b, strings.ToLower(r.Level)...)
	b = append(b, ": "...)
	if !r.HasAccess {
		return string(appendText(b, strings.TrimRight(r.Msg, "\r\n")))
	}
	b = append(b, r.Method...)
	b = append(b, ' ')
	b = append(b, r.Path...)
	b = append(b, ' ')
	b = strconv.AppendInt(b, int64(r.Status), 10)
	b = append(b, ' ')
	b = strconv.AppendFloat(b, r.Ms, 'f', 1, 64)
	b = append(b, "ms "...)
	b = strconv.AppendInt(b, r.Bytes, 10)
	b = append(b, "B rid="...)
	b = append(b, r.Rid...)
	switch {
	case r.Slow:
		b = append(b, " slow route="...)
		b = appendText(b, r.Route)
		b = append(b, " steps="...)
		b = strconv.AppendInt(b, int64(r.Steps), 10)
		b = append(b, " db="...)
		b = strconv.AppendInt(b, int64(r.DBStmts), 10)
		b = append(b, '/')
		b = strconv.AppendFloat(b, r.DBMs, 'f', 1, 64)
		b = append(b, "ms"...)
	case debug:
		b = append(b, " steps="...)
		b = strconv.AppendInt(b, int64(r.Steps), 10)
	}
	return string(b)
}

// appendText appends s to a text line so that nothing it holds can pass for a line of its own:
// each line break (\n, \r, \r\n) becomes "\n\t", and any other control character (C0, DEL, C1)
// is written escaped, `\x1b`. It is the text logger's rule, ported from the runtime's.
func appendText(b []byte, s string) []byte {
	const hex = "0123456789abcdef"
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c == '\r' && i+1 < len(s) && s[i+1] == '\n':
			// the \n that follows makes the break
		case c == '\n' || c == '\r':
			b = append(b, '\n', '\t')
		case c < 0x20 && c != '\t' || c == 0x7f:
			b = append(b, '\\', 'x', hex[c>>4], hex[c&15])
		case c == 0xc2 && i+1 < len(s) && s[i+1] >= 0x80 && s[i+1] <= 0x9f: // a C1 control, U+0080–U+009F
			b = append(b, '\\', 'u', '0', '0', hex[s[i+1]>>4], hex[s[i+1]&15])
			i++
		default:
			b = append(b, c)
		}
	}
	return b
}

// ring is a fixed-capacity buffer that drops its oldest item when full.
type ring[T any] struct {
	buf   []T
	start int
	n     int
}

func newRing[T any](max int) ring[T] {
	if max < 1 {
		max = 1
	}
	return ring[T]{buf: make([]T, max)}
}

func (r *ring[T]) add(v T) {
	if r.n < len(r.buf) {
		r.buf[(r.start+r.n)%len(r.buf)] = v
		r.n++
		return
	}
	r.buf[r.start] = v
	r.start = (r.start + 1) % len(r.buf)
}

// each calls f on every item, oldest first.
func (r *ring[T]) each(f func(T)) {
	for i := 0; i < r.n; i++ {
		f(r.buf[(r.start+i)%len(r.buf)])
	}
}

// at is the i-th item, 0 the oldest.
func (r *ring[T]) at(i int) T { return r.buf[(r.start+i)%len(r.buf)] }

func (r *ring[T]) reset() {
	clear(r.buf)
	r.start, r.n = 0, 0
}
