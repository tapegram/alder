package jobs

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"time"

	"alder/rt/db"
)

// The app binary's --jobs task (spec §4, §5): `alder jobs --env` runs it in the environment, by
// the binary that serves it, and reads its JSON lines (AdminLine) back with DecodeAdmin.

// adminUsage is the task's grammar, printed on stderr for anything else.
const adminUsage = "usage: --jobs list | --jobs retry <id> | --jobs retry --dead | --jobs run <Kind>"

// adminStates are the states a list line counts, every one present (0 when none).
var adminStates = []State{Queued, Running, Dead, Done, Cancelled}

// Admin is the --jobs task: args is `list`, `retry <id>`, `retry --dead` or `run <Kind>` over
// the app's shared store (cfg.Shared) and its silo databases (db.SiloDBs), kinds the app's jobs
// (app.Jobs()). It prints AdminLine JSON lines on stdout and every message on stderr, and returns
// the exit code: 0, 1 when something was refused, 2 on a failure or a store missed. It checks
// no migration and no ALDER_SECRET (as --silo-status); list opens every store read-only.
func Admin(ctx context.Context, cfg db.StoresConfig, kinds []Kind, args []string, stdout, stderr io.Writer) int {
	a := &admin{cfg: cfg, now: time.Now(), out: json.NewEncoder(stdout), stderr: stderr}
	switch {
	case len(args) == 1 && args[0] == "list":
		return a.list(ctx)
	case len(args) == 2 && args[0] == "retry" && args[1] == "--dead":
		return a.retryDead(ctx)
	case len(args) == 2 && args[0] == "retry" && args[1] != "" && args[1][0] != '-':
		return a.retry(ctx, args[1])
	case len(args) == 2 && args[0] == "run":
		return a.run(ctx, kinds, args[1])
	}
	fmt.Fprintln(stderr, adminUsage)
	return 2
}

// admin is one run of the task.
type admin struct {
	cfg    db.StoresConfig
	now    time.Time
	out    *json.Encoder
	stderr io.Writer
}

// adminStore is one store the task looks in: the shared one (silo "") or a silo database.
type adminStore struct {
	silo       string // "key/value"; "" for the shared store
	key, value string
	orphaned   bool
	open       func(ctx context.Context, reader bool) (*db.SQLite, error)
	// check is the silo database's state (db.SiloDB.Check); nil for the shared store.
	check func(ctx context.Context, st *db.SQLite) (db.SiloState, string, error)
}

// where names s in a message.
func (s adminStore) where() string {
	if s.silo == "" {
		return "the shared database"
	}
	return "silo " + s.silo
}

// stores is the shared store, then every silo database in db.SiloDBs' order. err is a failure
// to list the silos: stores then holds the shared store only, and a caller acts on it before it
// reports err.
func (a *admin) stores(ctx context.Context) ([]adminStore, error) {
	shared := adminStore{open: func(ctx context.Context, reader bool) (*db.SQLite, error) {
		if reader {
			return db.OpenSQLiteReader(a.cfg.Shared)
		}
		return db.OpenSQLite(a.cfg.Shared)
	}}
	out := []adminStore{shared}
	silos, err := db.SiloDBs(ctx, a.cfg)
	if err != nil {
		return out, fmt.Errorf("the silo databases: %w", err)
	}
	for _, d := range silos {
		out = append(out, adminStore{silo: d.Key + "/" + d.Value, key: d.Key, value: d.Value, orphaned: d.Orphaned, open: d.Open, check: d.Check})
	}
	return out, nil
}

// with opens s (reader: read-only) and runs f on it; has is whether it has a jobs table.
func (s adminStore) with(ctx context.Context, reader bool, f func(st *db.SQLite, q db.Execer, has bool) error) error {
	st, err := s.open(ctx, reader)
	if err != nil {
		return err
	}
	defer st.Close()
	q, err := db.On(st, nil)
	if err != nil {
		return err
	}
	var one int
	err = q.QueryRowContext(ctx, `SELECT 1 FROM sqlite_schema WHERE "type" = 'table' AND "name" = '_alder_jobs'`).Scan(&one)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	return f(st, q, err == nil)
}

// maintenance is the detail of silo database st's state when it is not served (the workers do
// not look in it until `alder migrate silos` serves it again), "" when it is or s is the shared
// store.
func (s adminStore) maintenance(ctx context.Context, st *db.SQLite) (string, error) {
	if s.check == nil {
		return "", nil
	}
	state, detail, err := s.check(ctx, st)
	if err != nil || state == db.SiloOK {
		return "", err
	}
	if detail == "" {
		return string(state), nil
	}
	return detail, nil
}

// fail prints err on stderr, after where when it is not ""; the task exits 2.
func (a *admin) fail(where string, err error) int {
	if where != "" {
		fmt.Fprintf(a.stderr, "%s: %v\n", where, err)
	} else {
		fmt.Fprintln(a.stderr, err)
	}
	return 2
}

// hint writes the hint of silo s in the shared store (Hint), so a --jobs-only worker finds the
// row a retry queued there within a poll.
func (a *admin) hint(ctx context.Context, s adminStore) error {
	st, err := db.OpenSQLite(a.cfg.Shared)
	if err != nil {
		return err
	}
	defer st.Close()
	q, err := db.On(st, nil)
	if err != nil {
		return err
	}
	if err := EnsureTable(ctx, q); err != nil {
		return err
	}
	return Hint(ctx, q, s.key, s.value, a.now)
}

// list is `list`: one line per store, its counts, its queued schedule ticks and its dead rows;
// a store that cannot be read is an error line and the rest are still listed (exit 2).
func (a *admin) list(ctx context.Context) int {
	stores, serr := a.stores(ctx)
	code := 0
	for _, s := range stores {
		line := AdminLine{Silo: s.silo, Counts: map[State]int{}}
		for _, st := range adminStates {
			line.Counts[st] = 0
		}
		err := s.with(ctx, true, func(_ *db.SQLite, q db.Execer, has bool) error {
			if !has {
				return nil
			}
			counts, err := Counts(ctx, q)
			if err != nil {
				return err
			}
			for st, n := range counts {
				line.Counts[st] = n
			}
			l, err := List(ctx, q)
			if err != nil {
				return err
			}
			for _, r := range l.Queued {
				if r.Schedule {
					line.Ticks = append(line.Ticks, r)
				}
			}
			line.Dead = l.Dead
			return nil
		})
		if err != nil {
			line = AdminLine{Silo: s.silo, Error: err.Error()}
			code = 2
		}
		a.out.Encode(line)
	}
	if serr != nil {
		return a.fail("", serr)
	}
	return code
}

// refusal is whether err is a retry refused (as `alder jobs retry` tells one from a failure).
func refusal(err error) bool {
	var (
		se *StateError
		ke *KeyHeldError
		oe *OrphanedError
	)
	return errors.As(err, &se) || errors.As(err, &ke) || errors.As(err, &oe) || errors.Is(err, ErrNoJob)
}

// retry is `retry <id>`: the first store with row id queues it again (Retry), as `alder jobs
// retry` does; a retry in a silo writes its hint. A retry in an orphaned silo is refused.
func (a *admin) retry(ctx context.Context, id string) int {
	stores, serr := a.stores(ctx)
	for _, s := range stores {
		var (
			row      *Row
			maint    string
			checkErr error
		)
		err := s.with(ctx, false, func(st *db.SQLite, q db.Execer, has bool) error {
			if !has {
				return nil
			}
			r, err := Get(ctx, q, id)
			if err != nil || r == nil {
				return err
			}
			row = r
			if s.orphaned {
				return &OrphanedError{Silo: s.silo}
			}
			if err := Retry(ctx, q, id, a.now); err != nil {
				return err
			}
			maint, checkErr = s.maintenance(ctx, st)
			return nil
		})
		if err == nil && row == nil { // not in this store
			continue
		}
		switch {
		case err != nil && refusal(err):
			a.out.Encode(AdminLine{Silo: s.silo, Refused: []string{err.Error()}})
			return 1
		case err != nil:
			return a.fail(s.where(), err)
		}
		a.out.Encode(AdminLine{Silo: s.silo, Row: row, Maintenance: maint})
		if s.silo != "" {
			if err := a.hint(ctx, s); err != nil {
				return a.fail("the shared database", err)
			}
		}
		if checkErr != nil {
			return a.fail(s.where(), checkErr)
		}
		return 0
	}
	if serr != nil {
		return a.fail("", serr)
	}
	a.out.Encode(AdminLine{Refused: []string{fmt.Errorf("%w %s", ErrNoJob, id).Error()}})
	return 1
}

// retryDead is `retry --dead`: every store's dead rows queued again (RetryDead), a line per row,
// then one line of the refusals; an orphaned silo's dead rows are each refused. A failure part
// way exits 2 after the lines so far: each retry commits on its own.
func (a *admin) retryDead(ctx context.Context) int {
	stores, serr := a.stores(ctx)
	var refused []string
	refusedLine := func() {
		if len(refused) > 0 {
			a.out.Encode(AdminLine{Refused: refused})
		}
	}
	for _, s := range stores {
		var (
			retried  []Row
			maint    string
			checkErr error
		)
		err := s.with(ctx, false, func(st *db.SQLite, q db.Execer, has bool) error {
			if !has {
				return nil
			}
			if s.orphaned {
				l, err := List(ctx, q)
				for range l.Dead {
					refused = append(refused, (&OrphanedError{Silo: s.silo}).Error())
				}
				return err
			}
			r, refs, err := RetryDead(ctx, q, a.now)
			retried = r
			for _, e := range refs {
				refused = append(refused, e.Error())
			}
			if len(retried) > 0 {
				maint, checkErr = s.maintenance(ctx, st)
			}
			return err
		})
		for i := range retried {
			a.out.Encode(AdminLine{Silo: s.silo, Row: &retried[i], Maintenance: maint})
		}
		if len(retried) > 0 && s.silo != "" {
			if herr := a.hint(ctx, s); herr != nil {
				refusedLine()
				return a.fail("the shared database", herr)
			}
		}
		if err == nil {
			err = checkErr
		}
		if err != nil {
			refusedLine()
			return a.fail(s.where(), err)
		}
	}
	refusedLine()
	if serr != nil {
		return a.fail("", serr)
	}
	if len(refused) > 0 {
		return 1
	}
	return 0
}

// run is `run <Kind>`: the job kinds names, which takes no parameters, queued once in the
// shared store (Manual).
func (a *admin) run(ctx context.Context, kinds []Kind, name string) int {
	k, err := ResolveKind(kinds, name)
	if err != nil {
		return a.fail("", err)
	}
	st, err := db.OpenSQLite(a.cfg.Shared)
	if err != nil {
		return a.fail("the shared database", err)
	}
	defer st.Close()
	q, err := db.On(st, nil)
	if err != nil {
		return a.fail("the shared database", err)
	}
	if err := EnsureTable(ctx, q); err != nil {
		return a.fail("the shared database", err)
	}
	id, err := Manual(ctx, q, k, a.now)
	if err != nil {
		return a.fail("the shared database", err)
	}
	a.out.Encode(AdminLine{ID: id, Kind: k.Name})
	return 0
}
