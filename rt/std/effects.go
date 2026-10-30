package std

// The effectful standard library: Query.*, Task.*, Session.put, Log.*, Random.*, Time.now,
// Date.today and Http.*. Each function only builds an effect tree (eff.Call, eff.Pure, …);
// nothing happens until eff.Run interprets it. The Db.* functions live in rt/db.

import (
	"context"
	"fmt"
	"io"
	"math/rand/v2"
	"net/http"
	"strings"
	"time"

	"alder/rt"
	"alder/rt/applog"
	"alder/rt/db"
	"alder/rt/eff"
)

// QuerySucceed is Query.succeed.
func QuerySucceed[T any](v T) eff.Query[T] { return eff.Q(eff.Pure(v)) }

// TaskSucceed is Task.succeed.
func TaskSucceed[T any](v T) eff.Task[T] { return eff.Pure(v) }

// QueryMap is Query.map.
func QueryMap[A, B any](f func(A) B, q eff.Query[A]) eff.Query[B] {
	return eff.Q(eff.Map(f, eff.T(q)))
}

// TaskMap is Task.map.
func TaskMap[A, B any](f func(A) B, t eff.Task[A]) eff.Task[B] { return eff.Map(f, t) }

// QueryAll is Query.all: the queries run in order.
func QueryAll[A any](qs []eff.Query[A]) eff.Query[[]A] {
	ts := make([]eff.Task[A], len(qs))
	for i, q := range qs {
		ts[i] = eff.T(q)
	}
	return eff.Q(eff.All(ts))
}

// TaskAll is Task.all: the tasks run in order; the first failure stops it.
func TaskAll[A any](ts []eff.Task[A]) eff.Task[[]A] { return eff.All(ts) }

// QueryNotFound is Query.notFound.
func QueryNotFound[T any]() eff.Query[T] { return eff.Q(eff.NotFound[T]()) }

// TaskNotFound is Task.notFound: the Task fails with eff.ErrNotFound (a route answers 404), its
// writes rolled back like any failed Task's.
func TaskNotFound[T any]() eff.Task[T] { return eff.NotFound[T]() }

// TaskFail is Task.fail.
func TaskFail[T any](msg string) eff.Task[T] { return eff.Fail[T](msg) }

// SessionPut is Session.put: it replaces Env.Session (of the app's Session type S) with f of it,
// once Env.CheckSession (an app with a `via` field: the membership) accepts the change.
// It is a write: it never runs inside a Query.
func SessionPut[S any](f func(S) S) eff.Task[struct{}] {
	return eff.Call("Session.put", true, func(e *eff.Env) (struct{}, error) {
		next := f(e.Session.(S))
		var s any = next
		if e.CheckSession != nil {
			var err error
			if s, err = e.CheckSession(e, e.Session, next); err != nil {
				return struct{}{}, err
			}
		}
		e.Session = s
		e.SessionPut = true
		return struct{}{}, nil
	})
}

// LogInfo is Log.info: msg goes to Env.Log at level "info" (nowhere when Env.Log is nil).
func LogInfo(msg string) eff.Task[struct{}] { return logAt("Log.info", "info", msg) }

// LogWarn is Log.warn.
func LogWarn(msg string) eff.Task[struct{}] { return logAt("Log.warn", "warn", msg) }

// LogDebug is Log.debug: written only under ALDER_LOG=debug.
func LogDebug(msg string) eff.Task[struct{}] { return logAt("Log.debug", "debug", msg) }

// LogError is Log.error.
func LogError(msg string) eff.Task[struct{}] { return logAt("Log.error", "error", msg) }

func logAt(name, level, msg string) eff.Task[struct{}] {
	return eff.Call(name, false, func(e *eff.Env) (struct{}, error) {
		if e.Log != nil {
			e.Log(level, applog.CutText(msg)) // at most 8 KB
		}
		return struct{}{}, nil
	})
}

// RandomInt is Random.int: a uniformly random integer in [Min, Max]; Min when Max < Min.
func RandomInt(r struct{ Min, Max int64 }) eff.Task[int64] {
	return eff.Call("Random.int", false, func(*eff.Env) (int64, error) {
		if r.Max <= r.Min {
			return r.Min, nil
		}
		span := uint64(r.Max - r.Min) // < 2^64: Max > Min
		if span == ^uint64(0) {
			return int64(rand.Uint64()), nil
		}
		return r.Min + int64(rand.Uint64N(span+1)), nil
	})
}

// RandomId is Random.id: a fresh 20-character URL-safe id (db.NewID).
func RandomId() eff.Task[string] {
	return eff.Call("Random.id", false, func(*eff.Env) (string, error) { return db.NewID(), nil })
}

// TimeNow is Time.now: Env.Now(), or the wall clock when Env.Now is nil.
func TimeNow() eff.Query[time.Time] {
	return eff.Q(eff.Call("Time.now", false, func(e *eff.Env) (time.Time, error) { return now(e), nil }))
}

// QueryDelay is Query.delay: wait ms milliseconds (clamped to 0..60000) through Env.Sleep.
func QueryDelay(ms int64) eff.Query[struct{}] { return eff.Q(delay("Query.delay", ms)) }

// TaskSleep is Task.sleep: a request-scoped wait, not a scheduler.
func TaskSleep(ms int64) eff.Task[struct{}] { return delay("Task.sleep", ms) }

func delay(name string, ms int64) eff.Task[struct{}] {
	ms = min(max(ms, 0), 60000)
	d := time.Duration(ms) * time.Millisecond
	return eff.Call(fmt.Sprintf("%s %dms", name, ms), false, func(e *eff.Env) (struct{}, error) {
		if e.Sleep != nil {
			e.Sleep(d)
			return struct{}{}, nil
		}
		ctx := e.Ctx
		if ctx == nil {
			ctx = context.Background()
		}
		t := time.NewTimer(d)
		defer t.Stop()
		select {
		case <-t.C:
			return struct{}{}, nil
		case <-ctx.Done():
			return struct{}{}, ctx.Err()
		}
	})
}

// DateToday is Date.today: the UTC calendar date of Env.Now().
func DateToday() eff.Query[rt.Date] {
	return eff.Q(eff.Call("Date.today", false, func(e *eff.Env) (rt.Date, error) { return rt.DateOf(now(e)), nil }))
}

func now(e *eff.Env) time.Time {
	if e.Now != nil {
		return e.Now()
	}
	return time.Now()
}

// httpTimeout bounds every Http.get and Http.post.
const httpTimeout = 10 * time.Second

var httpClient = &http.Client{Timeout: httpTimeout}

// HttpGet is Http.get: Ok with the body of a 2xx response; Err with the status and body of any
// other response, or with status 0 and the error when there is no response.
func HttpGet(r struct{ Url string }) eff.Task[rt.Result[rt.HttpError, string]] {
	return eff.Call("Http.get", false, func(e *eff.Env) (rt.Result[rt.HttpError, string], error) {
		return httpDo(e, http.MethodGet, r.Url, nil), nil
	})
}

// HttpPost is Http.post: as HttpGet, sending Body (as text/plain; charset=utf-8).
func HttpPost(r struct{ Url, Body string }) eff.Task[rt.Result[rt.HttpError, string]] {
	return eff.Call("Http.post", false, func(e *eff.Env) (rt.Result[rt.HttpError, string], error) {
		return httpDo(e, http.MethodPost, r.Url, strings.NewReader(r.Body)), nil
	})
}

func httpDo(e *eff.Env, method, url string, body io.Reader) rt.Result[rt.HttpError, string] {
	ctx := e.Ctx
	if ctx == nil {
		ctx = context.Background()
	}
	req, err := http.NewRequestWithContext(ctx, method, url, body)
	if err != nil {
		return rt.Err[rt.HttpError, string](rt.HttpError{Message: err.Error()})
	}
	if body != nil {
		req.Header.Set("Content-Type", "text/plain; charset=utf-8")
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return rt.Err[rt.HttpError, string](rt.HttpError{Message: err.Error()})
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return rt.Err[rt.HttpError, string](rt.HttpError{Status: int64(resp.StatusCode), Message: err.Error()})
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		msg := string(b)
		if msg == "" {
			msg = http.StatusText(resp.StatusCode)
		}
		return rt.Err[rt.HttpError, string](rt.HttpError{Status: int64(resp.StatusCode), Message: msg})
	}
	return rt.Ok[rt.HttpError](string(b))
}
