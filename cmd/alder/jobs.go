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

const jobsUsage = "usage: alder jobs [retry <id> | retry --dead | run <Kind>] [dir] [--db FILE] [--build-dir DIR]"

// jobsHelpNote follows `alder jobs --help`'s usage line.
const jobsHelpNote = `
alder jobs lists the jobs in the app's dev database (build/<app>/dev.db, or the file --db names)
and in its silo databases (silos/<key>/*.db beside it): the count in each state, each scheduled
job's next tick, and the dead jobs with their attempts and error.
  alder jobs retry <id>     queue a dead job again, due now: attempts 0, its error cleared
  alder jobs retry --dead   the same for every dead job
  alder jobs run <Kind>     queue a run of a job without parameters, due now, beside its schedule
A retried schedule tick runs once: no next tick is queued after it (the job's queued tick stays).
The file is changed directly: a running app's workers find a job at their next look, in a silo
only while the silo is in their active set (a job queued or running there in the last hour) or
after they restart. A database on Turso is not read (--db takes a file).`

// jobsStore is one database of `alder jobs`: the shared one (silo "") or a silo's.
type jobsStore struct {
	path, silo string // silo: "org/42"
}

// runJobsTo is `alder jobs [retry <id> | retry --dead | run <Kind>] [dir] [--db FILE]
// [--build-dir DIR]`. Exit 0; 1 when an operation is refused (an unknown id, a job not dead, a
// key held, an app with errors); 2 for usage, a database that cannot be read, and `run` of a
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
	if len(pos) > 0 && (pos[0] == "retry" || pos[0] == "run") {
		op, pos = pos[0], pos[1:]
	}
	var target string // retry's id, run's kind
	switch {
	case dead && op != "retry":
		return usage()
	case op != "" && !dead: // retry <id>, run <Kind>
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
	switch {
	case op == "":
		return jobsList(ctx, dbPath, stores, stdout, stderr)
	case op == "retry" && dead:
		return jobsRetryDead(ctx, stores, stdout, stderr)
	case op == "retry":
		return jobsRetry(ctx, dbPath, stores, target, stdout, stderr)
	}
	return jobsRun(ctx, dir, dbPath, target, stdout, stderr)
}

// jobsStores is the shared database at path and the silo databases beside it
// (db.SiloDir(path)/silos/<key>/<file>.db, db.SiloFiles: the files the app's own --silo-status
// and its workers' start scan list), every key directory found there.
func jobsStores(path string) ([]jobsStore, error) {
	out := []jobsStore{{path: path}}
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
		out = append(out, jobsStore{path: f.Path, silo: f.Key + "/" + f.Value})
	}
	return out, nil
}

// withJobs opens s (read-only unless write) and calls f with its jobs table; a database without
// one (an app that never ran with a job, a silo with none yet) is not f's: ok false.
func withJobs(ctx context.Context, s jobsStore, write bool, f func(q db.Execer) error) (ok bool, err error) {
	open := db.OpenSQLiteReader
	if write {
		open = db.OpenSQLite
	}
	st, err := open(s.path)
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
	return true, f(q)
}

// where names s for an error: its file, and its silo.
func (s jobsStore) where() string {
	if s.silo == "" {
		return s.path
	}
	return "silo " + s.silo + " (" + s.path + ")"
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

// jobsList is `alder jobs`: the counts by state, the scheduled kinds' queued ticks and the dead
// rows of every store, the last finished first.
func jobsList(ctx context.Context, path string, stores []jobsStore, stdout, stderr io.Writer) int {
	counts := map[jobs.State]int{}
	type placed struct {
		jobs.Row
		silo string
	}
	var ticks, dead []placed
	for _, s := range stores {
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
			for _, r := range l.Queued {
				if r.Schedule {
					ticks = append(ticks, placed{r, s.silo})
				}
			}
			for _, r := range l.Dead {
				dead = append(dead, placed{r, s.silo})
			}
			return nil
		})
		if err != nil {
			fmt.Fprintf(stderr, "%s %s: %v\n", errTag(stderr, "alder jobs:"), s.where(), err)
			return 2
		}
	}
	at := func(p *int64) int64 {
		if p == nil {
			return 0
		}
		return *p
	}
	slices.SortStableFunc(ticks, func(a, b placed) int { return cmp.Or(cmp.Compare(a.RunAt, b.RunAt), cmp.Compare(a.Kind, b.Kind)) })
	slices.SortStableFunc(dead, func(a, b placed) int {
		return cmp.Or(cmp.Compare(at(b.Finished), at(a.Finished)), cmp.Compare(b.Created, a.Created))
	})
	fmt.Fprintf(stdout, "jobs in %s%s:\n", path, siloCount(stores, "and"))
	tw := tabwriter.NewWriter(stdout, 0, 0, 2, ' ', 0)
	for _, st := range []jobs.State{jobs.Queued, jobs.Running, jobs.Dead, jobs.Done} {
		fmt.Fprintf(tw, "  %s\t%d\n", st, counts[st])
	}
	tw.Flush()
	if len(ticks) > 0 {
		fmt.Fprintln(stdout, "next schedule ticks:")
		for _, r := range ticks {
			fmt.Fprintf(tw, "  %s\t%s\n", r.Kind, utcMinute(r.RunAt))
		}
		tw.Flush()
	}
	if len(dead) == 0 {
		fmt.Fprintln(stdout, "no dead job")
		return 0
	}
	fmt.Fprintln(stdout, "dead jobs, the last finished first:")
	for _, r := range dead {
		attempts := fmt.Sprintf("%d attempts", r.Attempts)
		if r.Attempts == 1 {
			attempts = "1 attempt"
		}
		text := ""
		if r.Error != nil {
			first, rest, more := strings.Cut(*r.Error, "\n")
			text = first
			if more && strings.TrimSpace(rest) != "" {
				text += " …"
			}
		}
		if r.ScheduledFor != nil {
			text += " (the schedule tick of " + utcMinute(*r.ScheduledFor) + ")"
		}
		if r.silo != "" {
			text += " (in silo " + r.silo + ")"
		}
		fmt.Fprintf(tw, "  %s\t%s\t%s\t%s\n", r.ID, r.Kind, attempts, strings.TrimSpace(text))
	}
	tw.Flush()
	fmt.Fprintln(stdout, "`alder jobs retry <id>` queues one again; `alder jobs retry --dead` queues them all again")
	return 0
}

// retriedNotes are what a retry of r in silo says after its line: a tick runs once, and a silo's
// job waits for the workers to look in the silo.
func retriedNotes(r jobs.Row, silo string) []string {
	var out []string
	if r.ScheduledFor != nil {
		out = append(out, fmt.Sprintf("it is the %s schedule tick of %s: it runs once: no next tick is queued after it (the job's queued tick stays as it is)", r.Kind, utcMinute(*r.ScheduledFor)))
	}
	if silo != "" {
		out = append(out, fmt.Sprintf("in silo %s, a running app's workers find it only while the silo is in their active set (a job queued or running there in the last hour) or after they restart", silo))
	}
	return out
}

// jobsRetry is `alder jobs retry <id>`: the store that has row id queues it again (jobs.Retry).
func jobsRetry(ctx context.Context, path string, stores []jobsStore, id string, stdout, stderr io.Writer) int {
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
			fmt.Fprintf(stderr, "%s %s: %v\n", errTag(stderr, "alder jobs:"), s.where(), err)
			return 2
		}
		in := ""
		if s.silo != "" {
			in = " in silo " + s.silo
		}
		fmt.Fprintf(stdout, "queued job %s (%s) again%s, due now: attempts 0, its error cleared\n", id, row.Kind, in)
		for _, note := range retriedNotes(*row, s.silo) {
			fmt.Fprintln(stdout, "  "+note)
		}
		return 0
	}
	fmt.Fprintf(stderr, "%s no job %s in %s%s\n", errTag(stderr, "alder jobs:"), id, path, siloCount(stores, "or its"))
	return 1
}

// jobsRetryDead is `alder jobs retry --dead`: every store's dead rows queued again
// (jobs.RetryDead); exit 1 when one was refused. A failure part way (a store that cannot be
// read, a statement of RetryDead's) exits 2 after the summary of the rows already queued again:
// each retry commits on its own, so they stay queued.
func jobsRetryDead(ctx context.Context, stores []jobsStore, stdout, stderr io.Writer) int {
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
				for _, note := range retriedNotes(r, s.silo) {
					notes = append(notes, "  "+r.ID+": "+note)
				}
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
			fmt.Fprintf(stderr, "%s %s: %v\n", errTag(stderr, "alder jobs:"), s.where(), err)
			return 2
		}
	}
	summary()
	return code
}

// jobsRun is `alder jobs run <Kind>`: the app in dir's job named kind ("Nightly", or qualified
// "Reports.Nightly"), which must take no parameters, queued once in the shared database at path
// (jobs.Manual: `{}`, not a schedule tick).
func jobsRun(ctx context.Context, dir, path, kind string, stdout, stderr io.Writer) int {
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
	var all, match []string
	params := map[string][]string{}
	for _, mod := range slices.Sorted(maps.Keys(res.Jobs)) {
		for _, j := range res.Jobs[mod] {
			q := j.Module + "." + j.Name
			all = append(all, q)
			if kind == j.Name || kind == q {
				match = append(match, q)
				var names []string
				for _, f := range j.Params.Fields {
					names = append(names, f.Name)
				}
				params[q] = names
			}
		}
	}
	switch {
	case len(match) == 0 && len(all) == 0:
		return fail(2, "the app has no job %s: it declares no job", kind)
	case len(match) == 0:
		return fail(2, "the app has no job %s; its jobs: %s", kind, strings.Join(all, ", "))
	case len(match) > 1:
		return fail(2, "%s names several jobs: %s; name one with its module", kind, strings.Join(match, ", "))
	}
	name := match[0]
	if ps := params[name]; len(ps) > 0 {
		return fail(2, "%s takes parameters (%s): only a job without parameters is run from here; queue it from the app with Job.enqueue", name, strings.Join(ps, ", "))
	}
	var id string
	st, err := db.OpenSQLite(path)
	if err != nil {
		return fail(2, "%s: %v", path, err)
	}
	defer st.Close()
	q, err := db.On(st, nil)
	if err == nil {
		err = jobs.EnsureTable(ctx, q)
	}
	if err == nil {
		id, err = jobs.Manual(ctx, q, name, time.Now())
	}
	if err != nil {
		return fail(2, "%s: %v", path, err)
	}
	fmt.Fprintf(stdout, "queued a run of %s, due now: job %s in %s (not a schedule tick: the job's schedule is left as it is)\n", name, id, path)
	return 0
}
