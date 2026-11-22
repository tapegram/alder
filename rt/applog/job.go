package applog

import (
	"fmt"
	"log/slog"
	"math"
	"strconv"
	"time"
)

// LockMark is how long a job attempt may hold the database's write transaction before its line
// is marked (spec §6a): while it holds it, every request's write waits.
const LockMark = 2 * time.Second

// Job is one job attempt's line (Logger.Job): what the worker knows once the attempt has ended.
type Job struct {
	ID, Kind string // the job's id (the line's id, as a request's) and its kind, "Signup.SendWelcome"
	// Outcome is "done", "failed" (queued again for a retry), "dead" (past its retries, or its
	// params cannot be decoded) or "stopped" (the workers stopped; queued again, not counted).
	Outcome  string
	Dur      time.Duration // the attempt's time
	Attempt  int           // this attempt, 1 for the first
	Attempts int           // the most the job may have: its retries plus one
	Err      string        // the failure's text ("failed", "dead")
	Next     time.Duration // "failed": the wait before the retry
	LockHeld time.Duration // how long the attempt held the write transaction; above LockMark the line is marked
}

// Level is the line's level: done and stopped at Info, failed at Warn, dead at Error; a line
// that is marked (LockHeld above LockMark) is at least Warn.
func (j *Job) Level() Level {
	lv := Info
	switch j.Outcome {
	case "failed":
		lv = Warn
	case "dead":
		lv = Error
	}
	if lv < Warn && j.LockHeld > LockMark {
		lv = Warn
	}
	return lv
}

// Text is the line's message:
//
//	job Signup.SendWelcome a1b2c3d4 done 120ms attempt 1
//	job Signup.SendWelcome a1b2c3d4 failed attempt 2/5 "smtp: timeout", next in 1m
//	job Signup.SendWelcome a1b2c3d4 failed attempt 5/5 "smtp: timeout", dead
//	job Signup.SendWelcome a1b2c3d4 stopped, queued again
//
// followed, when LockHeld is above LockMark, by ", held the write lock 41 s: write last, or fan
// out — book 17". The error is quoted (Go syntax: a line break in it is \n).
func (j *Job) Text() string {
	b := make([]byte, 0, 96)
	b = append(b, "job "...)
	b = append(b, j.Kind...)
	b = append(b, ' ')
	b = append(b, j.ID...)
	switch j.Outcome {
	case "done":
		b = append(b, " done "...)
		b = strconv.AppendInt(b, j.Dur.Milliseconds(), 10)
		b = append(b, "ms attempt "...)
		b = strconv.AppendInt(b, int64(j.Attempt), 10)
	case "failed", "dead":
		b = append(b, " failed attempt "...)
		b = strconv.AppendInt(b, int64(j.Attempt), 10)
		b = append(b, '/')
		b = strconv.AppendInt(b, int64(j.Attempts), 10)
		b = append(b, ' ')
		b = strconv.AppendQuote(b, CutText(j.Err))
		if j.Outcome == "dead" {
			b = append(b, ", dead"...)
		} else {
			b = append(b, ", next in "...)
			b = append(b, durationText(j.Next)...)
		}
	default:
		b = append(b, ' ')
		b = append(b, j.Outcome...)
		if j.Outcome == "stopped" {
			b = append(b, ", queued again"...)
		}
	}
	if j.LockHeld > LockMark {
		b = append(b, ", "...)
		b = append(b, LockText(j.LockHeld)...)
	}
	return string(b)
}

// LockText is the mark of an attempt that held the write transaction for d (spec §6a), as its
// line and the dev panel's Jobs page show it: "held the write lock 41 s: write last, or fan out
// — book 17" (whole seconds, rounded). Its callers mark only a d above LockMark.
func LockText(d time.Duration) string {
	return "held the write lock " + strconv.FormatInt(int64(math.Round(d.Seconds())), 10) + " s: write last, or fan out — book 17"
}

// Job writes j's line at j.Level(): as text, Text; as JSON, Text as the message, with the job's
// id as rid (the key a request's lines carry), job (its kind), outcome, ms and attempt, and
// lock_ms when the line is marked. Render shows such a record as its text line.
func (l *Logger) Job(j *Job) {
	lv := j.Level()
	if !l.Enabled(lv) {
		return
	}
	msg := j.Text()
	if l.format == JSON {
		r := slog.NewRecord(l.now().UTC(), slogLevel(lv), msg, 0)
		r.AddAttrs(slog.String("rid", j.ID), slog.String("job", j.Kind), slog.String("outcome", j.Outcome),
			slog.Float64("ms", tenths(j.Dur)), slog.Int("attempt", j.Attempt))
		if j.LockHeld > LockMark {
			r.AddAttrs(slog.Float64("lock_ms", tenths(j.LockHeld)))
		}
		l.handle(r)
		return
	}
	bp := l.start(lv)
	*bp = appendText(*bp, msg)
	l.end(bp)
}

// durationText is d as an Alder duration literal: "10s", "1m", "1h" (d.String() when no single
// unit divides it).
func durationText(d time.Duration) string {
	for _, u := range []struct {
		d    time.Duration
		name string
	}{{time.Hour, "h"}, {time.Minute, "m"}, {time.Second, "s"}, {time.Millisecond, "ms"}} {
		if d > 0 && d%u.d == 0 {
			return fmt.Sprintf("%d%s", d/u.d, u.name)
		}
	}
	return d.String()
}
