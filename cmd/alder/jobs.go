package main

import (
	"cmp"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"text/tabwriter"
	"time"

	"alder/internal/buildkit"
	"alder/rt/db"
	"alder/rt/jobs"
)

// `alder jobs` (#168, spec §9): the jobs table of a database file — the app's dev database by
// default — and of the silo databases beside it, read and changed directly, as `alder dump` reads
// files; a running app's workers see a change at their next look. The operations are rt/jobs's
// admin ones (admin.go), which the dev panel's Jobs page uses too.

const jobsUsage = "usage: alder jobs [retry <id> | retry --dead | run <Kind> | cancel <id>] [dir] [--db FILE] [--build-dir DIR]"

// jobsHelpNote follows `alder jobs --help`'s usage line.
const jobsHelpNote = `
alder jobs lists the jobs in the app's dev database (build/<app>/dev.db, or the file --db names)
and in its silo databases (silos/<key>/*.db beside it): the count in each state, each scheduled
job's next tick, and the dead jobs with their attempts and error.
  alder jobs retry <id>     queue a dead or cancelled job again, due now: attempts 0, its error cleared
  alder jobs retry --dead   the same for every dead job
  alder jobs run <Kind>     queue a run of a job without parameters, due now, beside its schedule
  alder jobs cancel <id>    cancel a queued or running job (not a schedule's queued tick)
A retried schedule tick runs once: no next tick is queued after it (the job's queued tick stays).
The file is changed directly: a running app's workers find a job at their next look, in a silo
only while the silo is in their active set (a job queued or running there in the last hour) or
after they restart. A database on Turso is not read (--db takes a file).`

// jobsStore is one database of `alder jobs`: the shared one (silo "") or a silo's. where names it
// for an error (its file, and its silo); open opens it, read-only unless write.
type jobsStore struct {
	silo, where string // silo: "org/42"
	open        func(ctx context.Context, write bool) (*db.SQLite, error)
}

// placed is a job row and where it was found: its silo ("org/42", "" for the shared store) and,
// when the silo is under maintenance, what is behind (a source that knows says so).
type placed struct {
	jobs.Row
	silo        string
	maintenance string
}

// jobsReport is what a source gathered for `alder jobs`: the counts by state, the scheduled kinds'
// queued ticks and the dead rows of every store, and a line for each store that could not be read
// (`<where>: not read: <err>`). silos is how many silo databases were looked at.
type jobsReport struct {
	counts      map[jobs.State]int
	ticks, dead []placed
	missed      []string
	silos       int
}

// jobsPlace is how a source words itself: head, the words after "jobs " in the header ("in
// build/x/dev.db and 1 silo database"); flags, appended to the closing hint's two commands ("" or
// " --env production"); who, the workers a retry's note speaks of ("app": "a running app's
// workers", "build").
type jobsPlace struct {
	head, flags, who string
}

// runJobsTo is `alder jobs [retry <id> | retry --dead | run <Kind> | cancel <id>] [dir] [--db FILE]
// [--build-dir DIR]`. Exit 0; 1 when an operation is refused (an unknown id, a job neither dead nor cancelled
// (retry) or neither queued nor running (cancel), a schedule's queued tick, a key held, an app with errors); 2 for usage, a database that cannot be read, and `run` of a
// kind the app lacks or one with parameters.
func runJobsTo(args []string, stdout, stderr io.Writer) int {
	fail := func(code int, format string, a ...any) int {
		fmt.Fprintf(stderr, "%s %s\n", errTag(stderr, "alder jobs:"), fmt.Sprintf(format, a...))
		return code
	}
	usage := func() int { fmt.Fprintln(stderr, jobsUsage); return 2 }
	var (
		dbPath, bd string
		dead       bool
		pos        []string
	)
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case (a == "--db" || a == "--build-dir") && i+1 < len(args):
			if a == "--db" {
				dbPath = args[i+1]
			} else {
				bd = args[i+1]
			}
			i++
		case strings.HasPrefix(a, "--db="):
			if dbPath = strings.TrimPrefix(a, "--db="); dbPath == "" {
				return usage() // an empty file name is not "the default"
			}
		case strings.HasPrefix(a, "--build-dir="):
			bd = strings.TrimPrefix(a, "--build-dir=")
		case a == "--dead":
			dead = true
		case strings.HasPrefix(a, "-"):
			return usage()
		default:
			pos = append(pos, a)
		}
	}
	op := ""
	if len(pos) > 0 && (pos[0] == "retry" || pos[0] == "run" || pos[0] == "cancel") {
		op, pos = pos[0], pos[1:]
	}
	var target string // retry's and cancel's id, run's kind
	switch {
	case dead && op != "retry":
		return usage()
	case op != "" && !dead: // retry <id>, run <Kind>, cancel <id>
		if len(pos) == 0 {
			return usage()
		}
		target, pos = pos[0], pos[1:]
	}
	if len(pos) > 1 {
		return usage()
	}
	dir := "."
	if len(pos) == 1 {
		dir = pos[0]
		if info, err := os.Stat(dir); dead && (err != nil || !info.IsDir()) {
			return usage() // `retry <id> --dead`: --dead takes no id, and what follows is the app dir
		}
	}
	if dbPath == "" {
		dbPath = filepath.Join(appBuildDir(bd, dir), "dev.db")
	}
	if db.IsRemote(dbPath) {
		return fail(2, "--db takes a database file, not a database on Turso (%s)", dbPath)
	}
	if info, err := os.Stat(dbPath); err != nil || info.IsDir() {
		return fail(2, "no database at %s: `alder run` makes the app's dev database, or name a file with --db", dbPath)
	}
	stores, err := jobsStores(dbPath)
	if err != nil {
		return fail(2, "%v", err)
	}
	ctx := context.Background()
	at := jobsPlace{head: "in " + dbPath + siloCount(stores, "and"), who: "app"}
	switch {
	case op == "":
		return printJobs(stdout, stderr, at, gatherJobs(ctx, stores))
	case op == "retry" && dead:
		return jobsRetryDead(ctx, at, stores, stdout, stderr)
	case op == "retry":
		return jobsRetry(ctx, at, stores, target, stdout, stderr)
	case op == "cancel":
		return jobsCancel(ctx, at, stores, target, stdout, stderr)
	}
	return jobsRun(ctx, dir, stores[0], target, stdout, stderr)
}

// jobsStores is the shared database at path and the silo databases beside it
// (db.SiloDir(path)/silos/<key>/<file>.db, db.SiloFiles: the files the app's own --silo-status
// and its workers' start scan list), every key directory found there.
func jobsStores(path string) ([]jobsStore, error) {
	out := []jobsStore{fileStore(path, "", path)}
	ents, err := os.ReadDir(filepath.Join(db.SiloDir(path), "silos"))
	if errors.Is(err, os.ErrNotExist) {
		return out, nil
	}
	if err != nil {
		return nil, err
	}
	var keys []db.Silo
	for _, e := range ents {
		if e.IsDir() {
			keys = append(keys, db.Silo{Key: e.Name()})
		}
	}
	files, err := db.SiloFiles(db.SiloDir(path), keys)
	if err != nil {
		return nil, err
	}
	for _, f := range files {
		silo := f.Key + "/" + f.Value
		out = append(out, fileStore(f.Path, silo, "silo "+silo+" ("+f.Path+")"))
	}
	return out, nil
}

// fileStore is the database file path as a store.
func fileStore(path, silo, where string) jobsStore {
	return jobsStore{silo: silo, where: where, open: func(_ context.Context, write bool) (*db.SQLite, error) {
		if write {
			return db.OpenSQLite(path)
		}
		return db.OpenSQLiteReader(path)
	}}
}

// withJobs opens s (read-only unless write) and calls f with its jobs table; a database without
// one (an app that never ran with a job, a silo with none yet) is not f's: ok false, and is left
// without one. A jobs table an older build made (schema 1) is upgraded first, as the app's open
// does (jobs.EnsureTable, in a transaction of its own), for a read as for a write: every read
// names the current columns.
func withJobs(ctx context.Context, s jobsStore, write bool, f func(q db.Execer) error) (ok bool, err error) {
	st, err := s.open(ctx, write)
	if err != nil {
		return false, err
	}
	defer st.Close()
	q, err := db.On(st, nil)
	if err != nil {
		return false, err
	}
	var one int
	err = q.QueryRowContext(ctx, `SELECT 1 FROM sqlite_schema WHERE "type" = 'table' AND "name" = '_alder_jobs'`).Scan(&one)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	old, err := db.JobsTableOld(ctx, q)
	if err != nil {
		return false, err
	}
	if old {
		if err := upgradeJobs(ctx, s); err != nil {
			return false, err
		}
	}
	return true, f(q)
}

// upgradeJobs moves the schema-1 jobs table of s to the current schema.
func upgradeJobs(ctx context.Context, s jobsStore) error {
	st, err := s.open(ctx, true)
	if err != nil {
		return err
	}
	defer st.Close()
	q, err := db.On(st, nil)
	if err != nil {
		return err
	}
	return jobs.EnsureTable(ctx, q)
}

// siloCount is " and N silo database(s)" for the silos of stores ("" for none).
func siloCount(stores []jobsStore, and string) string {
	switch n := len(stores) - 1; n {
	case 0:
		return ""
	case 1:
		return " " + and + " 1 silo database"
	default:
		return fmt.Sprintf(" %s %d silo databases", and, n)
	}
}

// utcMinute is the time of ms (milliseconds since the epoch) as `2026-11-17 03:00 UTC`.
func utcMinute(ms int64) string { return time.UnixMilli(ms).UTC().Format("2006-01-02 15:04 UTC") }

// gatherJobs reads the counts, the scheduled kinds' queued ticks and the dead rows of every store;
// a store that cannot be read is a line of missed and the rest are still read.
func gatherJobs(ctx context.Context, stores []jobsStore) jobsReport {
	r := jobsReport{counts: map[jobs.State]int{}, silos: max(len(stores)-1, 0)}
	for _, s := range stores {
		// a store's rows are kept only when it was read whole
		var ticks, dead []placed
		counts := map[jobs.State]int{}
		_, err := withJobs(ctx, s, false, func(q db.Execer) error {
			c, err := jobs.Counts(ctx, q)
			if err != nil {
				return err
			}
			for st, n := range c {
				counts[st] += n
			}
			l, err := jobs.List(ctx, q)
			if err != nil {
				return err
			}
			for _, row := range l.Queued {
				if row.Schedule {
					ticks = append(ticks, placed{Row: row, silo: s.silo})
				}
			}
			for _, row := range l.Dead {
				dead = append(dead, placed{Row: row, silo: s.silo})
			}
			return nil
		})
		if err != nil {
			r.missed = append(r.missed, s.where+": not read: "+err.Error())
			continue
		}
		for st, n := range counts {
			r.counts[st] += n
		}
		r.ticks = append(r.ticks, ticks...)
		r.dead = append(r.dead, dead...)
	}
	return r
}

// printJobs is `alder jobs`: the counts by state, the scheduled kinds' queued ticks and the dead
// rows of every store, the last finished first; then, on stderr, a line for each store that was
// not read. Exit 2 when there is one, else 0.
func printJobs(stdout, stderr io.Writer, at jobsPlace, r jobsReport) int {
	ms := func(p *int64) int64 {
		if p == nil {
			return 0
		}
		return *p
	}
	ticks, dead := slices.Clone(r.ticks), slices.Clone(r.dead)
	slices.SortStableFunc(ticks, func(a, b placed) int { return cmp.Or(cmp.Compare(a.RunAt, b.RunAt), cmp.Compare(a.Kind, b.Kind)) })
	slices.SortStableFunc(dead, func(a, b placed) int {
		return cmp.Or(cmp.Compare(ms(b.Finished), ms(a.Finished)), cmp.Compare(b.Created, a.Created))
	})
	fmt.Fprintf(stdout, "jobs %s:\n", at.head)
	tw := tabwriter.NewWriter(stdout, 0, 0, 2, ' ', 0)
	for _, st := range []jobs.State{jobs.Queued, jobs.Running, jobs.Dead, jobs.Cancelled, jobs.Done} {
		fmt.Fprintf(tw, "  %s\t%d\n", st, r.counts[st])
	}
	tw.Flush()
	if len(ticks) > 0 {
		fmt.Fprintln(stdout, "next schedule ticks:")
		for _, t := range ticks {
			prio := ""
			if t.Priority != 0 {
				prio = fmt.Sprintf(" priority %d", t.Priority)
			}
			fmt.Fprintf(tw, "  %s\t%s%s\n", t.Kind, utcMinute(t.RunAt), prio)
		}
		tw.Flush()
	}
	if len(dead) == 0 {
		fmt.Fprintln(stdout, "no dead job")
	} else {
		fmt.Fprintln(stdout, "dead jobs, the last finished first:")
		for _, d := range dead {
			attempts := fmt.Sprintf("%d attempts", d.Attempts)
			if d.Attempts == 1 {
				attempts = "1 attempt"
			}
			text := ""
			if d.Error != nil {
				first, rest, more := strings.Cut(*d.Error, "\n")
				text = first
				if more && strings.TrimSpace(rest) != "" {
					text += " …"
				}
			}
			if d.ScheduledFor != nil {
				text += " (the schedule tick of " + utcMinute(*d.ScheduledFor) + ")"
			}
			if d.silo != "" {
				text += " (in silo " + d.silo + ")"
			}
			fmt.Fprintf(tw, "  %s\t%s\t%s\t%s\n", d.ID, d.Kind, attempts, strings.TrimSpace(text))
		}
		tw.Flush()
		fmt.Fprintf(stdout, "`alder jobs retry <id>%s` queues one again; `alder jobs retry --dead%s` queues them all again\n", at.flags, at.flags)
	}
	for _, m := range r.missed {
		fmt.Fprintf(stderr, "%s %s\n", errTag(stderr, "alder jobs:"), m)
	}
	if len(r.missed) > 0 {
		return 2
	}
	return 0
}

// siloNote is what a job in silo waits for: the workers look in a silo only while it is in their
// active set. hint: the retry also wrote the silo's hint in the shared store, which a `--jobs-only`
// worker reads each poll (rt/jobs/active.go:192-199; its generated main sets Options.Hints,
// rt/jobs/worker.go:76).
func siloNote(silo, who string, hint bool) string {
	note := fmt.Sprintf("in silo %s, a running %s's workers find it only while the silo is in their active set (a job queued or running there in the last hour) or after they restart", silo, who)
	if hint {
		note += "; a `--jobs-only` worker reads the silo hint within a poll"
	}
	return note
}

// retriedNotes are what a retry of p says after its line: a tick runs once, and a silo's job
// waits for the workers to look in the silo (its hint was written); a silo under maintenance is
// not looked in at all.
func retriedNotes(p placed, who string) []string {
	var out []string
	if p.ScheduledFor != nil {
		out = append(out, fmt.Sprintf("it is the %s schedule tick of %s: it runs once: no next tick is queued after it (the job's queued tick stays as it is)", p.Kind, utcMinute(*p.ScheduledFor)))
	}
	if p.silo != "" {
		out = append(out, siloNote(p.silo, who, true))
		if p.maintenance != "" {
			out = append(out, fmt.Sprintf("in silo %s, under maintenance (%s): the workers do not look there until `alder migrate silos` serves it again", p.silo, p.maintenance))
		}
	}
	return out
}

// printRetried is the lines of a retry of p: it is queued again, due now, and what it waits for.
func printRetried(stdout io.Writer, at jobsPlace, p placed) {
	in := ""
	if p.silo != "" {
		in = " in silo " + p.silo
	}
	fmt.Fprintf(stdout, "queued job %s (%s) again%s, due now: attempts 0, its error cleared\n", p.ID, p.Kind, in)
	for _, note := range retriedNotes(p, at.who) {
		fmt.Fprintln(stdout, "  "+note)
	}
}

// hintSilo writes the hint of silo ("org/42") in the shared store, so a `--jobs-only` worker finds
// a job retried there (jobs.EnsureTable, then jobs.Hint).
func hintSilo(ctx context.Context, shared jobsStore, silo string) error {
	key, value, _ := strings.Cut(silo, "/")
	st, err := shared.open(ctx, true)
	if err != nil {
		return fmt.Errorf("the silo hint: %w", err)
	}
	defer st.Close()
	q, err := db.On(st, nil)
	if err == nil {
		err = jobs.EnsureTable(ctx, q)
	}
	if err == nil {
		err = jobs.Hint(ctx, q, key, value, time.Now())
	}
	if err != nil {
		return fmt.Errorf("the silo hint in %s: %w", shared.where, err)
	}
	return nil
}

// jobsRetry is `alder jobs retry <id>`: the store that has row id queues it again (jobs.Retry).
func jobsRetry(ctx context.Context, at jobsPlace, stores []jobsStore, id string, stdout, stderr io.Writer) int {
	for _, s := range stores {
		var row *jobs.Row
		_, err := withJobs(ctx, s, true, func(q db.Execer) error {
			r, err := jobs.Get(ctx, q, id)
			if err != nil || r == nil {
				return err
			}
			row = r
			return jobs.Retry(ctx, q, id, time.Now())
		})
		if err == nil && row != nil && s.silo != "" {
			err = hintSilo(ctx, stores[0], s.silo)
		}
		if err == nil && row == nil { // not in this store
			continue
		}
		var (
			se *jobs.StateError
			ke *jobs.KeyHeldError
		)
		switch {
		case errors.As(err, &se), errors.As(err, &ke), errors.Is(err, jobs.ErrNoJob):
			fmt.Fprintf(stderr, "%s %v\n", errTag(stderr, "alder jobs:"), err)
			return 1
		case err != nil:
			fmt.Fprintf(stderr, "%s %s: %v\n", errTag(stderr, "alder jobs:"), s.where, err)
			return 2
		}
		printRetried(stdout, at, placed{Row: *row, silo: s.silo})
		return 0
	}
	fmt.Fprintf(stderr, "%s no job %s in %s%s\n", errTag(stderr, "alder jobs:"), id, stores[0].where, siloCount(stores, "or its"))
	return 1
}

// jobsCancel is `alder jobs cancel <id>`: the store that has row id cancels it (jobs.CancelRow). The
// file is all this changes: a running app's workers cut off a running attempt when their watcher
// next reads its row (every poll, while the row's silo is in their active set), else at its end.
func jobsCancel(ctx context.Context, at jobsPlace, stores []jobsStore, id string, stdout, stderr io.Writer) int {
	for _, s := range stores {
		var row *jobs.Row
		_, err := withJobs(ctx, s, true, func(q db.Execer) error {
			r, err := jobs.Get(ctx, q, id)
			if err != nil || r == nil {
				return err
			}
			row = r
			return jobs.CancelRow(ctx, q, id, time.Now())
		})
		if err == nil && row == nil { // not in this store
			continue
		}
		var (
			se *jobs.StateError
			te *jobs.TickError
		)
		switch {
		case errors.As(err, &se), errors.As(err, &te), errors.Is(err, jobs.ErrNoJob):
			fmt.Fprintf(stderr, "%s %v\n", errTag(stderr, "alder jobs:"), err)
			return 1
		case err != nil:
			fmt.Fprintf(stderr, "%s %s: %v\n", errTag(stderr, "alder jobs:"), s.where, err)
			return 2
		}
		in := ""
		if s.silo != "" {
			in = " in silo " + s.silo
		}
		fmt.Fprintf(stdout, "cancelled job %s (%s)%s\n", id, row.Kind, in)
		fmt.Fprintln(stdout, "  a running app's workers stop it within a poll if it is running there")
		if s.silo != "" {
			fmt.Fprintln(stdout, "  "+siloNote(s.silo, at.who, false))
		}
		return 0
	}
	fmt.Fprintf(stderr, "%s no job %s in %s%s\n", errTag(stderr, "alder jobs:"), id, stores[0].where, siloCount(stores, "or its"))
	return 1
}

// jobsRetryDead is `alder jobs retry --dead`: every store's dead rows queued again
// (jobs.RetryDead); exit 1 when one was refused. A failure part way (a store that cannot be
// read, a statement of RetryDead's) exits 2 after the summary of the rows already queued again:
// each retry commits on its own, so they stay queued.
func jobsRetryDead(ctx context.Context, at jobsPlace, stores []jobsStore, stdout, stderr io.Writer) int {
	n, code := 0, 0
	var ids, notes []string
	summary := func() {
		switch n {
		case 0:
			if code == 0 {
				fmt.Fprintln(stdout, "no dead job")
			} else {
				fmt.Fprintln(stdout, "no dead job was queued again")
			}
		case 1:
			fmt.Fprintf(stdout, "queued 1 dead job again, due now: %s\n", ids[0])
		default:
			fmt.Fprintf(stdout, "queued %d dead jobs again, due now: %s\n", n, strings.Join(ids, ", "))
		}
		for _, note := range notes {
			fmt.Fprintln(stdout, note)
		}
	}
	for _, s := range stores {
		_, err := withJobs(ctx, s, true, func(q db.Execer) error {
			retried, refused, err := jobs.RetryDead(ctx, q, time.Now())
			for _, r := range retried {
				n++
				ids = append(ids, r.ID)
				for _, note := range retriedNotes(placed{Row: r, silo: s.silo}, at.who) {
					notes = append(notes, "  "+r.ID+": "+note)
				}
			}
			if len(retried) > 0 && s.silo != "" && err == nil {
				err = hintSilo(ctx, stores[0], s.silo)
			}
			for _, e := range refused {
				fmt.Fprintf(stderr, "%s %v\n", errTag(stderr, "alder jobs:"), e)
				code = 1
			}
			return err
		})
		if err != nil {
			if n > 0 {
				summary()
			}
			fmt.Fprintf(stderr, "%s %s: %v\n", errTag(stderr, "alder jobs:"), s.where, err)
			return 2
		}
	}
	summary()
	return code
}

// jobsRun is `alder jobs run <Kind>`: the app in dir's job named kind ("Nightly", or qualified
// "Reports.Nightly"), which must take no parameters (jobs.ResolveKind), queued once in the shared
// store (jobs.Manual: `{}`, not a schedule tick).
func jobsRun(ctx context.Context, dir string, shared jobsStore, kind string, stdout, stderr io.Writer) int {
	fail := func(code int, format string, a ...any) int {
		fmt.Fprintf(stderr, "%s %s\n", errTag(stderr, "alder jobs run:"), fmt.Sprintf(format, a...))
		return code
	}
	if !dirHasApp(dir) {
		return fail(2, "no .alder files under %s/app: `alder jobs run` reads the app's jobs (give the app's directory)", dir)
	}
	_, res, bag, ioErrs := buildkit.LoadChecked(dir, nil)
	if reportLoadErrs("alder jobs run", ioErrs, stderr) {
		return 2
	}
	if res == nil || bag.HasErrors() {
		return fail(1, "the app in %s has errors (`alder check` lists them): fix them first", dir)
	}
	var kinds []jobs.Kind
	for _, mod := range slices.Sorted(maps.Keys(res.Jobs)) {
		for _, j := range res.Jobs[mod] {
			k := jobs.Kind{Name: j.Module + "." + j.Name, Priority: j.Priority}
			for _, f := range j.Params.Fields {
				k.Params = append(k.Params, f.Name)
			}
			kinds = append(kinds, k)
		}
	}
	k, err := jobs.ResolveKind(kinds, kind)
	if err != nil {
		return fail(2, "%v", err)
	}
	var id string
	st, err := shared.open(ctx, true)
	if err != nil {
		return fail(2, "%s: %v", shared.where, err)
	}
	defer st.Close()
	q, err := db.On(st, nil)
	if err == nil {
		err = jobs.EnsureTable(ctx, q)
	}
	if err == nil {
		id, err = jobs.Manual(ctx, q, k, time.Now())
	}
	if err != nil {
		return fail(2, "%s: %v", shared.where, err)
	}
	fmt.Fprintf(stdout, "queued a run of %s, due now: job %s in %s (not a schedule tick: the job's schedule is left as it is)\n", k.Name, id, shared.where)
	return 0
}
