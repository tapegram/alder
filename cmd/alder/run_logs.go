package main

import (
	"bytes"
	"encoding/json"
	"io"
	"strings"
	"sync"

	"alder/internal/style"
	"alder/rt/applog"
)

// userLogging is how the person running `alder run` wants the app's log shown: the level
// ALDER_LOG names (default info) and whether ALDER_LOG_FORMAT is json (the lines are then passed
// through as the app writes them). An unknown value of either is an error. The child itself
// always logs JSON at debug (childEnv), so the feed gets every record whatever the person chose.
func userLogging(getenv func(string) string) (applog.Level, bool, error) {
	level, format := applog.Info, applog.Text
	if v := getenv("ALDER_LOG"); strings.TrimSpace(v) != "" {
		var err error
		if level, err = applog.ParseLevel(v); err != nil {
			return applog.Info, false, err
		}
	}
	if v := getenv("ALDER_LOG_FORMAT"); strings.TrimSpace(v) != "" {
		var err error
		if format, err = applog.ParseFormat(v); err != nil {
			return applog.Info, false, err
		}
	}
	return level, format == applog.JSON, nil
}

// logPipe takes one output stream of the child, line by line, with no limit on a line's length.
// Each line is a `log` event on the feed: with record, the decoded JSON object, when it is a
// record the app's logger writes (applog.Render), else with raw, the line as it is. It is also
// written to the terminal as the person asked for it (userLogging): a record below their level
// is dropped; a record at or above it is its text line (Render), or the JSON line unchanged when
// they asked for JSON; any other line goes through unchanged. On a styled terminal each line is
// styled as the app's text log is (logLine). It is safe for concurrent use.
//
// Write never fails, and never waits for more than the terminal's write: the child must not
// block on its own output.
type logPipe struct {
	term     io.Writer // the terminal (the dev server's locked stderr)
	tail     io.Writer // when set, also gets each terminal line unstyled (the child's tailBuffer)
	st       style.Style
	level    applog.Level
	passJSON bool
	hub      *eventHub

	mu      sync.Mutex
	buf     []byte // the partial line so far
	scanned int    // how much of buf holds no newline
}

func newLogPipe(term io.Writer, st style.Style, level applog.Level, passJSON bool, hub *eventHub) *logPipe {
	return &logPipe{term: term, st: st, level: level, passJSON: passJSON, hub: hub}
}

func (p *logPipe) Write(b []byte) (int, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.buf = append(p.buf, b...)
	start := 0
	for {
		i := bytes.IndexByte(p.buf[p.scanned:], '\n')
		if i < 0 {
			break
		}
		end := p.scanned + i
		p.line(p.buf[start:end], true)
		start, p.scanned = end+1, end+1
	}
	if start > 0 { // keep only the partial line
		n := copy(p.buf, p.buf[start:])
		p.buf = p.buf[:n]
	}
	p.scanned = len(p.buf)
	return len(b), nil
}

// Flush sends the partial last line, if any, as a line; on the terminal it ends without a
// newline unless it is a rendered record.
func (p *logPipe) Flush() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if len(p.buf) == 0 {
		return nil
	}
	p.line(p.buf, false)
	p.buf, p.scanned = nil, 0
	return nil
}

// line handles one line l (without its newline; nl says whether it had one).
func (p *logPipe) line(l []byte, nl bool) {
	text, ok := applog.Render(l, p.level)
	if ok {
		p.hub.emit("log", map[string]any{"record": json.RawMessage(bytes.TrimSpace(l))})
	} else {
		p.hub.emit("log", map[string]any{"raw": string(l)})
	}
	var out []byte
	switch {
	case ok && text == nil: // below the person's level
		return
	case ok && !p.passJSON:
		out = text
	default:
		out = l
		if nl {
			out = append(l[:len(l):len(l)], '\n')
		}
	}
	if p.tail != nil {
		_, _ = p.tail.Write(out)
	}
	if p.st.On() { // each terminal line on its own, as style.Lines did: a record's message may span several
		var sb strings.Builder
		style := logLine(p.st)
		for l := range strings.Lines(string(out)) {
			s, hadNL := strings.CutSuffix(l, "\n")
			sb.WriteString(style(s))
			if hadNL {
				sb.WriteByte('\n')
			}
		}
		_, _ = io.WriteString(p.term, sb.String())
		return
	}
	_, _ = p.term.Write(out)
}
