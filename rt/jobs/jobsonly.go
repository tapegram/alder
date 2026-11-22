package jobs

import (
	"context"
	"errors"
	"time"

	"alder/rt/db"
)

// AheadPoll is how often `app --jobs-only` reads the shared database's schema again while the
// database is ahead of its build (spec §7a).
const AheadPoll = 10 * time.Second

// errBehind is the web build's refusal of a database behind it (the generated Open's).
var errBehind = errors.New("the database is behind this build: run `alder deploy`")

// AwaitBuild is `app --jobs-only`'s form of the maintenance page (spec §7a): its shared
// database on Turso is ahead of its build (a newer build migrated it), and running jobs with this
// build's code would be stale, so it claims nothing and waits. Every AheadPoll (after(AheadPoll);
// nil: time.After), starting AheadPoll from the call, check reads how the database stands (the
// generated App's db.CheckShared): db.SharedOK returns nil, and the workers may start; behind or
// fresh (an older database restored) returns the web build's error "the database is behind this
// build: run `alder deploy`", logged first as Open logs it; ahead again waits on; a failed read is
// a warn line and the wait goes on. ctx ending ends the wait with ctx's error.
func AwaitBuild(ctx context.Context, check func(context.Context) (db.SharedState, string, error), after func(time.Duration) <-chan time.Time, log func(level, msg string)) error {
	if after == nil {
		after = time.After
	}
	if log == nil {
		log = func(string, string) {}
	}
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-after(AheadPoll):
		}
		state, detail, err := check(ctx)
		switch {
		case ctx.Err() != nil:
			return ctx.Err()
		case err != nil:
			log("warn", "jobs: reading the database's schema: "+err.Error())
		case state == db.SharedOK:
			log("info", "jobs: the database is this build's now: the job workers start")
			return nil
		case state == db.SharedBehind || state == db.SharedFresh:
			log("error", "the database: "+detail)
			return errBehind
		}
	}
}
