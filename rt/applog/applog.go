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
	b := appendText(*bp, prefix)
	b = appendText(b, strings.TrimRight(msg, "\r\n"))
	*bp = b
	l.end(bp)
}

// appendText appends s to a text line so that nothing it holds can pass for a line of its own:
// each line break (\n, \r, \r\n) becomes "\n\t", so a continuation line (a stack's, or text
// a message carries from a user) is indented and never starts at column 0 the way every line's
// stamp does; any other control character (C0, DEL, C1) is written escaped, `\x1b`.
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

// MaxText is the longest message (an app's Log.* text, what a router error line says) kept, in
// bytes: a longer one is cut, ending "…".
const MaxText = 8 << 10

// CutText is s cut at MaxText bytes, on a character boundary, ending "…"; s itself when it is
// not longer.
func CutText(s string) string {
	if len(s) <= MaxText {
		return s
	}
	i := MaxText
	for i > 0 && !utf8.RuneStart(s[i]) {
		i--
	}
	return s[:i] + "…"
}

// Access is one request's line (Logger.Access): what the router knows once it has answered.
type Access struct {
	ID           string   // the request id (RequestID)
	Method, Path string   // Path is already masked (MaskURL); "": URL masked when the line is written
	URL          *url.URL // the request's URL, masked (MaskURL) only for a line that is written
	// Masker, when set, masks URL instead of MaskURL: the router's, which also hides the path
	// parameters whose name is Sensitive (MaskURLSegments).
	Masker func(method string, u *url.URL) string
	Status int           // 0: the handler wrote nothing
	Dur    time.Duration // from the request's arrival to the answer written
	Bytes  int64         // the body bytes written
	Quiet  bool          // a file answered 2xx/304: logged at Debug, not Info
	Steps  int           // effect calls run; shown at Debug only (and on a Slow line)
	// Slow is set by the router when the request took ALDER_SLOW_MS or longer (web.Options.
	// SlowAfter): the line is written at Warn with Route, Steps, Stmts and DBDur appended.
	Slow  bool
	Route string        // the matched route pattern, or the router's fixed name (a Slow line's route=)
	Stmts int           // database statements run (eff.Stats; counted when metrics or slow is on)
	DBDur time.Duration // their time
}

// Access writes a's line at Info (Debug when a.Quiet; Warn when a.Slow). As text:
//
//	GET /start?key=••••&seed=7 200 12.3ms 4821B rid=5f3a2c1e
//
// ending " steps=N" when the Logger's level is Debug. As JSON the message is the method and the
// path, with method, path, status, ms, bytes and rid (and steps) as keys of their own. A Slow
// line ends with its breakdown, whatever the level:
//
//	GET /todos/3 200 812.4ms 4821B rid=5f3a2c1e slow route=/todos/{id} steps=12 db=9/640.1ms
//
// and as JSON carries slow, route, steps, db_stmts and db_ms.
func (l *Logger) Access(a *Access) {
	lv := Info
	switch {
	case a.Slow:
		lv = Warn
	case a.Quiet:
		lv = Debug
	}
	if !l.Enabled(lv) {
		return
	}
	ms := tenths(a.Dur)
	path := a.MaskedPath()
	if l.format == JSON {
		r := slog.NewRecord(l.now().UTC(), slogLevel(lv), a.Method+" "+path, 0)
		r.AddAttrs(slog.String("method", a.Method), slog.String("path", path), slog.Int("status", a.Status),
			slog.Float64("ms", ms), slog.Int64("bytes", a.Bytes), slog.String("rid", a.ID))
		if a.Slow {
			r.AddAttrs(slog.Bool("slow", true), slog.String("route", a.Route), slog.Int("steps", a.Steps),
				slog.Int("db_stmts", a.Stmts), slog.Float64("db_ms", tenths(a.DBDur)))
		} else if l.min == Debug {
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
	switch {
	case a.Slow:
		b = append(b, " slow route="...)
		b = appendText(b, a.Route)
		b = append(b, " steps="...)
		b = strconv.AppendInt(b, int64(a.Steps), 10)
		b = append(b, " db="...)
		b = strconv.AppendInt(b, int64(a.Stmts), 10)
		b = append(b, '/')
		b = strconv.AppendFloat(b, tenths(a.DBDur), 'f', 1, 64)
		b = append(b, "ms"...)
	case l.min == Debug:
		b = append(b, " steps="...)
		b = strconv.AppendInt(b, int64(a.Steps), 10)
	}
	*bp = b
	l.end(bp)
}

// tenths is d in milliseconds rounded to one decimal.
func tenths(d time.Duration) float64 {
	return math.Round(float64(d)/float64(time.Millisecond)*10) / 10
}

// SlowEnv is the variable SlowFromEnv reads.
const SlowEnv = "ALDER_SLOW_MS"

// SlowFromEnv is the threshold ALDER_SLOW_MS sets: a request that takes this long or longer is
// a warn line with its breakdown (Access.Slow). Unset, empty or 0: 0, off. Anything but a whole
// number of milliseconds is an error, which the generated main reports before serving.
func SlowFromEnv() (time.Duration, error) { return slowFromEnv(os.Getenv) }

func slowFromEnv(getenv func(string) string) (time.Duration, error) {
	v := strings.TrimSpace(getenv(SlowEnv))
	if v == "" {
		return 0, nil
	}
	n, err := strconv.ParseUint(v, 10, 32)
	if err != nil {
		return 0, fmt.Errorf("%s=%s: want a whole number of milliseconds (0: off)", SlowEnv, v)
	}
	return time.Duration(n) * time.Millisecond, nil
}

// MaxPath is the longest path an access line shows, in bytes: a longer one is cut, ending "…".
const MaxPath = 4 << 10

// MaskedPath is the path a's line shows: Path, or else URL masked (MaskURL), cut at MaxPath.
func (a *Access) MaskedPath() string {
	p := a.Path
	if p == "" && a.URL != nil {
		if a.Masker != nil {
			p = a.Masker(a.Method, a.URL)
		} else {
			p = MaskURL(a.URL)
		}
	}
	return CutPath(p)
}

// CutPath is p cut at MaxPath bytes, on a character boundary, ending "…"; p itself when it is
// not longer. Access lines and the router's error lines show a URL through it.
func CutPath(p string) string {
	if len(p) <= MaxPath {
		return p
	}
	i := MaxPath
	for i > 0 && !utf8.RuneStart(p[i]) {
		i--
	}
	return p[:i] + "…"
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
