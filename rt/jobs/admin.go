package jobs

import (
	"cmp"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"alder/rt/db"
)

// The admin operations on one store's jobs table (spec §9): the rows the dev panel's Jobs page
// lists, one row, a retry by hand and a run now. They take the table's store as a db.Execer and
// the time as now, so the dev panel (Panel, over every store its process sees) and `alder jobs`
// (over a database file) share them.

// ShownDone is how many done rows a listing keeps: the fifty finished last (spec §9).
const ShownDone = 50

// rowCols are the jobs table's columns in the order scanRow reads them.
const rowCols = `"id", "kind", "params", "key", "state", "run_at", "created", "started", "finished", "attempts", "error", "lease_until", "worker", "schedule", "scheduled_for", "priority"`

// ErrNoJob is the error of an operation on a job id no row has (never enqueued in the stores
// looked in, or pruned).
var ErrNoJob = errors.New("no such job")

// StateError is an operation refused because of the job's state: only a dead or cancelled job is retried,
// only a queued one is run now, only a queued or running one is cancelled.
type StateError struct {
	ID    string
	State State // the row's
	Want  State // the state the operation takes
}

// OrphanedError is an operation on a silo no store of the app holds any more (db.Orphans).
type OrphanedError struct{ Silo string }

func (e *OrphanedError) Error() string {
	return fmt.Sprintf("silo %s is orphaned: nothing runs there (alder silos prune)", e.Silo)
}

func (e *StateError) Error() string {
	want, op := string(e.Want), "retried"
	switch e.Want {
	case Queued:
		op = "run now"
	case Dead:
		want = "dead or cancelled"
	case Cancelled:
		want, op = "queued or running", "cancelled"
	}
	return fmt.Sprintf("job %s is %s: only a %s job is %s", e.ID, e.State, want, op)
}

// TickError is a cancel refused because the row is a schedule's queued tick (spec §3): the way to
// stop a schedule is to remove it from the job.
type TickError struct{ ID, Kind string }

func (e *TickError) Error() string {
	_, short := cutLast(e.Kind, ".")
	return fmt.Sprintf("job %s: it is `%s`'s queued tick; remove its schedule to stop it", e.ID, short)
}

// cutLast cuts s around the last sep: before is "" when s has none.
func cutLast(s, sep string) (before, after string) {
	if i := strings.LastIndex(s, sep); i >= 0 {
		return s[:i], s[i+len(sep):]
	}
	return "", s
}

// KeyHeldError is a retry refused because a queued or running job of the kind (Holder) holds the
// dead job's key: the retry would be a second job with that key (enqueueOnce's rule).
type KeyHeldError struct {
	ID, Holder, Kind, Key string
}

func (e *KeyHeldError) Error() string {
	return fmt.Sprintf("job %s is not retried: job %s of %s holds its key %q (queued or running)", e.ID, e.Holder, e.Kind, e.Key)
}

// Listing is the rows of a jobs table as the Jobs page lists them. A row's In is its store when
// the listing covers several (Panel.List); List of one store leaves it zero.
type Listing struct {
	Queued  []Row // in claim order: priority (highest first), run_at, created; a kind's queued schedule tick (Schedule) among them
	Running []Row // by started
	Dead    []Row // the last finished first
	Done    []Row // the ShownDone finished last, the last first
	// Cancelled is the ShownDone cancelled rows finished last, the last first.
	Cancelled []Row
	// Missed names the stores that could not be read, each with its error ("silo org/42: …"):
	// their rows are not in the listing.
	Missed []string
}

// List is the rows of q's jobs table: every queued, running and dead row and the ShownDone done
// and the ShownDone cancelled rows finished last.
func List(ctx context.Context, q db.Execer) (Listing, error) {
	var l Listing
	var err error
	if l.Queued, err = rowsWhere(ctx, q, `"state" = 'queued' ORDER BY "priority" DESC, "run_at", "created"`); err != nil {
		return l, err
	}
	if l.Running, err = rowsWhere(ctx, q, `"state" = 'running' ORDER BY "started", "created"`); err != nil {
		return l, err
	}
	if l.Dead, err = rowsWhere(ctx, q, `"state" = 'dead' ORDER BY "finished" DESC, "created" DESC`); err != nil {
		return l, err
	}
	if l.Done, err = rowsWhere(ctx, q, fmt.Sprintf(`"state" = 'done' ORDER BY "finished" DESC, "created" DESC LIMIT %d`, ShownDone)); err != nil {
		return l, err
	}
	l.Cancelled, err = rowsWhere(ctx, q, fmt.Sprintf(`"state" = 'cancelled' ORDER BY "finished" DESC, "created" DESC LIMIT %d`, ShownDone))
	return l, err
}

// Get is job id's row in q's jobs table; nil, nil when there is none.
func Get(ctx context.Context, q db.Execer, id string) (*Row, error) {
	r, err := scanRow(q.QueryRowContext(ctx, `SELECT `+rowCols+` FROM "_alder_jobs" WHERE "id" = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return r, err
}

// Retry queues dead or cancelled job id again in q's jobs table, due at now, as if it had never run: its
// attempts 0, its error, started, finished, lease and worker cleared (spec §6: "attempts reset to
// 0, state queued"). A schedule tick keeps its scheduled_for; its claim set its schedule to 0, so
// the retry runs it once and inserts no next tick (ClaimedTick does nothing for it), and the
// kind's queued tick is left as it is. ErrNoJob when there is no such row, a *StateError when it
// is neither dead nor cancelled, and a *KeyHeldError when it has a key that a queued or running job of its kind
// holds now (enqueueOnce's: the retry would be a second job with that key).
func Retry(ctx context.Context, q db.Execer, id string, now time.Time) error {
	r, err := Get(ctx, q, id)
	if err != nil {
		return err
	}
	if r == nil {
		return fmt.Errorf("%w %s", ErrNoJob, id)
	}
	if s := State(r.State); s != Dead && s != Cancelled {
		return &StateError{ID: id, State: State(r.State), Want: Dead}
	}
	if r.Key != nil {
		var other string
		err := q.QueryRowContext(ctx, `SELECT "id" FROM "_alder_jobs" WHERE "kind" = ? AND "key" = ? AND "state" IN ('queued', 'running')`, r.Kind, *r.Key).Scan(&other)
		if err == nil {
			return &KeyHeldError{ID: id, Holder: other, Kind: r.Kind, Key: *r.Key}
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return err
		}
	}
	return update(ctx, q, id, Dead, `UPDATE "_alder_jobs" SET "state" = 'queued', "attempts" = 0, "error" = NULL, "run_at" = ?, "started" = NULL, "finished" = NULL, "lease_until" = NULL, "worker" = NULL WHERE "id" = ? AND "state" IN ('dead', 'cancelled')`, now.UnixMilli(), id)
}

// RunNow makes queued job id in q's jobs table due at now. A queued schedule tick run so runs in
// place of its slot: its claim inserts the next tick after its scheduled_for (ClaimedTick), not
// after now. ErrNoJob when there is no such row, a *StateError when it is not queued.
func RunNow(ctx context.Context, q db.Execer, id string, now time.Time) error {
	r, err := Get(ctx, q, id)
	if err != nil {
		return err
	}
	if r == nil {
		return fmt.Errorf("%w %s", ErrNoJob, id)
	}
	if State(r.State) != Queued {
		return &StateError{ID: id, State: State(r.State), Want: Queued}
	}
	return update(ctx, q, id, Queued, `UPDATE "_alder_jobs" SET "run_at" = ? WHERE "id" = ? AND "state" = 'queued'`, now.UnixMilli(), id)
}

// CancelRow cancels queued or running job id in q's jobs table (cancelStmt), finished at now.
// ErrNoJob when there is no such row, a *TickError when it is a schedule's queued tick, a
// *StateError when it is final. It only marks the row: an attempt running in this process is cut
// off by Panel.Cancel (cutOff), one in another by that process's watcher (watch).
func CancelRow(ctx context.Context, q db.Execer, id string, now time.Time) error {
	r, err := Get(ctx, q, id)
	if err != nil {
		return err
	}
	if r == nil {
		return fmt.Errorf("%w %s", ErrNoJob, id)
	}
	switch s := State(r.State); {
	case s == Queued && r.Schedule:
		return &TickError{ID: id, Kind: r.Kind}
	case s != Queued && s != Running:
		return &StateError{ID: id, State: s, Want: Cancelled}
	}
	return update(ctx, q, id, Cancelled, cancelStmt, now.UnixMilli(), id)
}

// Counts is the number of rows of q's jobs table in each state (a state with none is absent).
func Counts(ctx context.Context, q db.Execer) (map[State]int, error) {
	rows, err := q.QueryContext(ctx, `SELECT "state", COUNT(*) FROM "_alder_jobs" GROUP BY "state"`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[State]int{}
	for rows.Next() {
		var (
			st string
			n  int
		)
		if err := rows.Scan(&st, &n); err != nil {
			return nil, err
		}
		out[State(st)] = n
	}
	return out, rows.Err()
}

// RetryDead retries every dead row of q's jobs table, as Retry does each, the last finished
// first: retried are the rows it queued again (as they were before), refused the Retry errors of
// the ones it did not (a key a queued or running job holds, or a row a worker changed
// meanwhile). err is a failure to read the table or a statement's.
func RetryDead(ctx context.Context, q db.Execer, now time.Time) (retried []Row, refused []error, err error) {
	dead, err := rowsWhere(ctx, q, `"state" = 'dead' ORDER BY "finished" DESC, "created" DESC`)
	if err != nil {
		return nil, nil, err
	}
	for _, r := range dead {
		err := Retry(ctx, q, r.ID, now)
		var (
			se *StateError
			ke *KeyHeldError
		)
		switch {
		case err == nil:
			retried = append(retried, r)
		case errors.As(err, &se), errors.As(err, &ke), errors.Is(err, ErrNoJob):
			refused = append(refused, err)
		default:
			return retried, refused, err
		}
	}
	return retried, refused, nil
}

// Manual queues a run of kind k by hand (`alder jobs run`, spec §4) in q's jobs table: a row of
// k's name and priority with the parameters `{}`, due and created at now, with no key and not a schedule tick (schedule 0,
// no scheduled_for), so a scheduled kind's ticks are left as they are. It returns the row's id
// (NewID; an id another row has is drawn again, up to idTries times). The caller checks that kind
// takes no parameters.
func Manual(ctx context.Context, q db.Execer, k Kind, now time.Time) (string, error) {
	var id string
	for range idTries {
		id = newID()
		res, err := q.ExecContext(ctx, `INSERT INTO "_alder_jobs" ("id", "kind", "params", "state", "run_at", "created", "priority") VALUES (?, ?, '{}', 'queued', ?, ?, ?) ON CONFLICT DO NOTHING`,
			id, k.Name, now.UnixMilli(), now.UnixMilli(), k.Priority)
		if err != nil {
			return "", err
		}
		if n, err := res.RowsAffected(); err != nil || n == 1 {
			return id, err
		}
	}
	return "", fmt.Errorf("jobs: job id %s is taken (%d ids drawn)", id, idTries)
}

// update runs stmt, which changes job id while it is in state want; a row whose state changed
// since it was read (a worker claimed it) is a *StateError.
func update(ctx context.Context, q db.Execer, id string, want State, stmt string, args ...any) error {
	res, err := q.ExecContext(ctx, stmt, args...)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil || n == 1 {
		return err
	}
	r, err := Get(ctx, q, id)
	switch {
	case err != nil:
		return err
	case r == nil:
		return fmt.Errorf("%w %s", ErrNoJob, id)
	}
	return &StateError{ID: id, State: State(r.State), Want: want}
}

// rowsWhere is the rows of q's jobs table matching where (with its ORDER BY).
func rowsWhere(ctx context.Context, q db.Execer, where string) ([]Row, error) {
	rows, err := q.QueryContext(ctx, `SELECT `+rowCols+` FROM "_alder_jobs" WHERE `+where)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Row
	for rows.Next() {
		r, err := scanRow(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *r)
	}
	return out, rows.Err()
}

// merge adds l's rows to m, each with its store's place in.
func (m *Listing) merge(l Listing, in db.Place) {
	for _, part := range []struct{ to, from *[]Row }{{&m.Queued, &l.Queued}, {&m.Running, &l.Running}, {&m.Dead, &l.Dead}, {&m.Done, &l.Done}, {&m.Cancelled, &l.Cancelled}} {
		for _, r := range *part.from {
			r.In = in
			*part.to = append(*part.to, r)
		}
	}
}

// sort puts a merged listing in List's orders and keeps ShownDone done rows.
func (m *Listing) sort() {
	at := func(p *int64) int64 {
		if p == nil {
			return 0
		}
		return *p
	}
	slices.SortStableFunc(m.Queued, func(a, b Row) int {
		return cmp.Or(cmp.Compare(b.Priority, a.Priority), cmp.Compare(a.RunAt, b.RunAt), cmp.Compare(a.Created, b.Created))
	})
	slices.SortStableFunc(m.Running, func(a, b Row) int {
		return cmp.Or(cmp.Compare(at(a.Started), at(b.Started)), cmp.Compare(a.Created, b.Created))
	})
	last := func(a, b Row) int {
		return cmp.Or(cmp.Compare(at(b.Finished), at(a.Finished)), cmp.Compare(b.Created, a.Created))
	}
	slices.SortStableFunc(m.Dead, last)
	slices.SortStableFunc(m.Done, last)
	slices.SortStableFunc(m.Cancelled, last)
	if len(m.Done) > ShownDone {
		m.Done = m.Done[:ShownDone]
	}
	if len(m.Cancelled) > ShownDone {
		m.Cancelled = m.Cancelled[:ShownDone]
	}
}
