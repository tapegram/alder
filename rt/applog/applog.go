// Package applog is a served app's log: one line per request it routes (Access) and the lines
// the runtime and the app's Log.* write (Log), filtered by level (ALDER_LOG) and written as text
// or JSON (ALDER_LOG_FORMAT). A line about a request carries its id (RequestID) and its URL
// masked (MaskURL); headers, cookies, bodies and sessions are never given to it.
//
// A line below the level is dropped before anything is formatted: it allocates nothing.
package applog

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"math"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"
)

// Level is how much a line matters; a Logger writes the lines at or above its own.
type Level int

const (
	Debug Level = iota
	Info
	Warn
	Error
)

var levelNames = [...]string{"debug", "info", "warn", "error"}

func (l Level) String() string {
	if l >= Debug && l <= Error {
		return levelNames[l]
	}
	return "level(" + strconv.Itoa(int(l)) + ")"
}

// ParseLevel is the level named debug, info, warn or error (any case, spaces trimmed).
func ParseLevel(s string) (Level, error) {
	t := strings.ToLower(strings.TrimSpace(s))
	for i, n := range levelNames {
		if t == n {
			return Level(i), nil
		}
	}
	return Info, fmt.Errorf("ALDER_LOG=%s: want debug, info, warn or error", strings.TrimSpace(s))
}

// Format is a line's shape: Text (the stamp, the level, the message) or JSON (one object per
// line).
type Format int

const (
	Text Format = iota
	JSON
)

// ParseFormat is the format named text or json (any case, spaces trimmed).
func ParseFormat(s string) (Format, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "text":
		return Text, nil
	case "json":
		return JSON, nil
	}
	return Text, fmt.Errorf("ALDER_LOG_FORMAT=%s: want text or json", strings.TrimSpace(s))
}

// Logger writes lines at or above its level to its writer, one Write per line. It is safe for
// concurrent use.
type Logger struct {
	w      io.Writer
	min    Level
	format Format
	mu     sync.Mutex // one line at a time on w (text; slog's handler has its own)
	json   *slog.JSONHandler
	now    func() time.Time
}

// New is a Logger writing lines at or above min to w in format f.
func New(w io.Writer, min Level, f Format) *Logger {
	l := &Logger{w: w, min: min, format: f, now: time.Now}
	if f == JSON {
		l.json = slog.NewJSONHandler(w, &slog.HandlerOptions{Level: slog.LevelDebug})
	}
	return l
}

// FromEnv is New with the level ALDER_LOG names (default info) and the format ALDER_LOG_FORMAT
// names (default text); an unknown value of either is an error, which the generated main
// reports before serving.
func FromEnv(w io.Writer) (*Logger, error) {
	return fromEnv(w, os.Getenv)
}

func fromEnv(w io.Writer, getenv func(string) string) (*Logger, error) {
	min, f := Info, Text
	if v := getenv("ALDER_LOG"); strings.TrimSpace(v) != "" {
		var err error
		if min, err = ParseLevel(v); err != nil {
			return nil, err
		}
	}
	if v := getenv("ALDER_LOG_FORMAT"); strings.TrimSpace(v) != "" {
		var err error
		if f, err = ParseFormat(v); err != nil {
			return nil, err
		}
	}
	return New(w, min, f), nil
}

// Enabled reports whether a line at level is written.
func (l *Logger) Enabled(level Level) bool { return level >= l.min }

// Log writes msg at the level named level: debug, info, warn or error, or "effect" (an effect
// step, written at Debug as "effect <msg>"). An unknown level is written at Info with its name
// kept in the text ("<level>: <msg>"), so nothing is lost. It is the runtime's Env.Log hook.
func (l *Logger) Log(level, msg string) {
	lv, prefix := Info, ""
	switch level {
	case "debug":
		lv = Debug
	case "info":
	case "warn":
		lv = Warn
	case "error":
		lv = Error
	case "effect":
		lv, prefix = Debug, "effect "
	default:
		prefix = level + ": "
	}
	if !l.Enabled(lv) {
		return
	}
	if l.format == JSON {
		l.handle(slog.NewRecord(l.now().UTC(), slogLevel(lv), prefix+msg, 0))
		return
	}
	bp := l.start(lv)
	b := append(append(*bp, prefix...), msg...)
	*bp = b
	l.end(bp)
}

// Access is one request's line (Logger.Access): what the router knows once it has answered.
type Access struct {
	ID           string        // the request id (RequestID)
	Method, Path string        // Path is already masked (MaskURL); "": URL masked when the line is written
	URL          *url.URL      // the request's URL, masked (MaskURL) only for a line that is written
	Status       int           // 0: the handler wrote nothing
	Dur          time.Duration // from the request's arrival to the answer written
	Bytes        int64         // the body bytes written
	Quiet        bool          // a file answered 2xx/304: logged at Debug, not Info
	Steps        int           // effect calls run; shown at Debug only
}

// Access writes a's line at Info (Debug when a.Quiet). As text:
//
//	GET /start?key=••••&seed=7 200 12.3ms 4821B rid=5f3a2c1e
//
// ending " steps=N" when the Logger's level is Debug. As JSON the message is the method and the
// path, with method, path, status, ms, bytes and rid (and steps) as keys of their own.
func (l *Logger) Access(a *Access) {
	lv := Info
	if a.Quiet {
		lv = Debug
	}
	if !l.Enabled(lv) {
		return
	}
	ms := math.Round(float64(a.Dur)/float64(time.Millisecond)*10) / 10
	path := a.MaskedPath()
	if l.format == JSON {
		r := slog.NewRecord(l.now().UTC(), slogLevel(lv), a.Method+" "+path, 0)
		r.AddAttrs(slog.String("method", a.Method), slog.String("path", path), slog.Int("status", a.Status),
			slog.Float64("ms", ms), slog.Int64("bytes", a.Bytes), slog.String("rid", a.ID))
		if l.min == Debug {
			r.AddAttrs(slog.Int("steps", a.Steps))
		}
		l.handle(r)
		return
	}
	bp := l.start(lv)
	b := *bp
	b = append(b, a.Method...)
	b = append(b, ' ')
	b = append(b, path...)
	b = append(b, ' ')
	b = strconv.AppendInt(b, int64(a.Status), 10)
	b = append(b, ' ')
	b = strconv.AppendFloat(b, ms, 'f', 1, 64)
	b = append(b, "ms "...)
	b = strconv.AppendInt(b, a.Bytes, 10)
	b = append(b, "B rid="...)
	b = append(b, a.ID...)
	if l.min == Debug {
		b = append(b, " steps="...)
		b = strconv.AppendInt(b, int64(a.Steps), 10)
	}
	*bp = b
	l.end(bp)
}

// MaxPath is the longest path an access line shows, in bytes: a longer one is cut, ending "…".
const MaxPath = 4 << 10

// MaskedPath is the path a's line shows: Path, or else URL masked (MaskURL), cut at MaxPath.
func (a *Access) MaskedPath() string {
	p := a.Path
	if p == "" && a.URL != nil {
		p = MaskURL(a.URL)
	}
	if len(p) > MaxPath {
		i := MaxPath
		for i > 0 && !utf8.RuneStart(p[i]) {
			i--
		}
		p = p[:i] + "…"
	}
	return p
}

// bufs are the text lines' buffers.
var bufs = sync.Pool{New: func() any { b := make([]byte, 0, 256); return &b }}

// stampLayout is a text line's stamp, log.Printf's shape, in UTC.
const stampLayout = "2006/01/02 15:04:05"

// start is a pooled buffer holding a text line's "<stamp> <level>: ".
func (l *Logger) start(lv Level) *[]byte {
	bp := bufs.Get().(*[]byte)
	b := l.now().UTC().AppendFormat((*bp)[:0], stampLayout)
	b = append(b, ' ')
	b = append(b, levelNames[lv]...)
	b = append(b, ": "...)
	*bp = b
	return bp
}

// end ends the text line in *bp with a newline (unless it ends with one), writes it in one Write
// and returns the buffer to the pool.
func (l *Logger) end(bp *[]byte) {
	b := *bp
	if len(b) == 0 || b[len(b)-1] != '\n' {
		b = append(b, '\n')
	}
	l.mu.Lock()
	_, _ = l.w.Write(b)
	l.mu.Unlock()
	if cap(b) <= 64<<10 { // a huge line (a long stack) is not kept
		*bp = b[:0]
		bufs.Put(bp)
	}
}

func (l *Logger) handle(r slog.Record) { _ = l.json.Handle(context.Background(), r) }

func slogLevel(lv Level) slog.Level {
	switch lv {
	case Debug:
		return slog.LevelDebug
	case Warn:
		return slog.LevelWarn
	case Error:
		return slog.LevelError
	}
	return slog.LevelInfo
}
