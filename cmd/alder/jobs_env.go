package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"alder/internal/deploy"
	"alder/internal/ir"
	"alder/internal/migrate"
	"alder/rt/db"
	"alder/rt/jobs"
)

// `alder jobs --env <name>` (#285, spec §1, §3, §6-§8): the operations of `alder jobs` against a
// deployed environment. At a box or a machine (Ssh, with a file or on Turso; Fly) the current
// build's executable runs its --jobs task there (deploy.JobsRunner, rt/jobs/admintask.go), whose
// JSON lines (jobs.AdminLine) are read back into the local command's rows and printed by its
// printers. On Render this machine opens the shared Turso database and its silo databases
// (deploy.JobStorer) and runs the local command's operations over them. A write asks first
// (--yes answers) and runs under the deploy lock.

// runJobsEnv is `alder jobs [retry <id> | retry --dead | run <Kind>] [dir] --env env [--host
// host] [--yes]`: op is "", "retry" or "run", target retry's id or run's kind, dead retry
// --dead's flag, dir the app's directory (read for its `deploy` declaration). Exit 0; 1 for a
// refused operation, the deploy lock held, a target not reached, an environment not deployed and
// a build older than --jobs; 2 for a missing declaration or environment, a Script target, a write
// not confirmed, a store not read and `run` of a kind the build lacks or one with parameters.
func runJobsEnv(ctx context.Context, op, target, env, dir, host string, dead, yes bool, stdout, stderr io.Writer, e deployEnv) int {
	fail := func(code int, format string, a ...any) int {
		fmt.Fprintf(stderr, "%s %s\n", errTag(stderr, "alder jobs:"), fmt.Sprintf(format, a...))
		return code
	}
	t, runner, err := envTargetRunner("jobs", env, dir, host, e)
	if err != nil {
		return fail(2, "%v", err)
	}
	defer t.Close()
	jr, runs := t.(deploy.JobsRunner)
	js, direct := t.(deploy.JobStorer)
	if !runs && !direct { // every target but Script (refused above) has one
		return fail(2, "internal error: %s has no way to its jobs table", t.Describe())
	}
	name := targetEnv(env, host)
	notDeployed := func() int {
		return fail(1, "environment %s is not deployed yet: no build serves there", name)
	}
	if _, err := t.Connect(ctx); err != nil {
		return fail(1, "%v", err)
	}
	id, err := t.Current(ctx)
	if err != nil {
		return fail(1, "%v", err)
	}
	if id == "" {
		return notDeployed()
	}
	hostArg := ""
	if host != "" {
		hostArg = " --host " + host
	}
	at := jobsPlace{head: "at " + t.Describe() + ", build " + id, flags: " --env " + env + hostArg, who: "build"}

	// write asks question unless --yes, then takes the deploy lock: ok false when the command is
	// over (code is its exit); release it with unlock, and run the write on wctx. The question is
	// asked with the default signal handling, so an interrupt there ends the command (as
	// restore's); from the lock on, SIGINT and SIGTERM cancel wctx instead, so the lock is
	// released (the reads before keep the default: they hold nothing).
	write := func(question string) (wctx context.Context, unlock func(), code int, ok bool) {
		if !yes && !confirm(question, stdout) {
			return nil, nil, fail(2, "not confirmed: nothing was queued (pass --yes to confirm without a terminal)"), false
		}
		wctx, stop := signal.NotifyContext(ctx, syscall.SIGINT, syscall.SIGTERM)
		if l, isLocker := t.(deploy.Locker); isLocker {
			u, err := l.Lock(wctx)
			if err != nil {
				stop()
				return nil, nil, fail(1, "%v", err), false
			}
			return wctx, func() { u(); stop() }, 0, true
		}
		return wctx, stop, 0, true
	}
	if direct {
		return runJobsStores(ctx, js, runner, t, at, id, op, target, dead, write, fail, notDeployed, stdout, stderr)
	}

	// task runs --jobs args at the target: its report lines, its messages printed on stderr, and
	// its exit code; ok false when the command is over (code is its exit).
	task := func(args ...string) (lines []jobs.AdminLine, code int, ok bool) {
		out, code, err := jr.Jobs(ctx, args...)
		switch {
		case errors.Is(err, deploy.ErrNotDeployed):
			return nil, notDeployed(), false
		case err != nil:
			return nil, fail(1, "%v", err), false
		case oldBuild(out, code):
			return nil, fail(1, "build %s at %s was made by an alder without `alder jobs --env`: `alder deploy %s%s` with this alder, then try again", id, name, env, hostArg), false
		}
		code = jobsExit(code)
		lines, msgs := jobs.DecodeAdmin(out)
		for _, m := range msgs {
			fmt.Fprintf(stderr, "%s %s\n", errTag(stderr, "alder jobs:"), m)
		}
		return lines, code, true
	}
	switch {
	case op == "": // list: read-only, no question, no lock
		lines, code, ok := task("list")
		if !ok {
			return code
		}
		if len(lines) == 0 { // the task failed before its first store (its messages say why)
			return max(code, 2)
		}
		r := reportOf(lines)
		return max(printJobs(stdout, stderr, at.withSilos(r.silos), r), code)

	case op == "retry" && dead:
		lines, code, ok := task("list")
		if !ok {
			return code
		}
		r := reportOf(lines)
		if len(r.dead) == 0 { // nothing to ask about or lock for (Review Focus 2)
			fmt.Fprintln(stdout, "no dead job")
			for _, m := range r.missed {
				fmt.Fprintf(stderr, "%s %s\n", errTag(stderr, "alder jobs:"), m)
			}
			if len(lines) == 0 || len(r.missed) > 0 {
				return max(code, 2)
			}
			return 0
		}
		question := fmt.Sprintf("queue the %d dead jobs at %s again, due now? type yes: ", len(r.dead), t.Describe())
		if len(r.dead) == 1 {
			question = fmt.Sprintf("queue the 1 dead job at %s again, due now? type yes: ", t.Describe())
		}
		wctx, unlock, code, ok := write(question)
		if !ok {
			return code
		}
		defer unlock()
		ctx = wctx // task runs on it
		lines, code, ok = task("retry", "--dead")
		if !ok {
			return code
		}
		var rows []placed
		refused := false
		for _, l := range lines {
			if l.Row != nil {
				rows = append(rows, placedOf(l))
			}
			for _, x := range l.Refused {
				fmt.Fprintf(stderr, "%s %s\n", errTag(stderr, "alder jobs:"), x)
				refused = true
			}
		}
		if code != 2 || len(rows) > 0 { // a failure part way prints the rows already queued
			printRetriedDead(stdout, at, rows, refused)
		}
		return code

	case op == "retry":
		wctx, unlock, code, ok := write(fmt.Sprintf("queue job %s again at %s, due now? type yes: ", target, t.Describe()))
		if !ok {
			return code
		}
		defer unlock()
		ctx = wctx // task runs on it
		lines, code, ok := task("retry", target)
		if !ok {
			return code
		}
		for _, l := range lines {
			if l.Row != nil {
				printRetried(stdout, at, placedOf(l))
			}
			for _, x := range l.Refused {
				fmt.Fprintf(stderr, "%s %s\n", errTag(stderr, "alder jobs:"), x)
			}
		}
		return code
	}

	// run <Kind>: the build resolves the kind (its own jobs)
	wctx, unlock, code, ok := write(fmt.Sprintf("queue a run of %s at %s, build %s, due now? type yes: ", target, t.Describe(), id))
	if !ok {
		return code
	}
	defer unlock()
	ctx = wctx // task runs on it
	lines, code, ok := task("run", target)
	if !ok {
		return code
	}
	for _, l := range lines {
		if l.ID != "" {
			fmt.Fprintf(stdout, "queued a run of %s, due now: job %s at %s (not a schedule tick: the job's schedule is left as it is)\n", l.Kind, l.ID, t.Describe())
		}
	}
	return code
}

// runJobsStores is runJobsEnv at a target whose databases this machine opens (deploy.JobStorer:
// Render's shared Turso database and its silo databases): the local command's operations over
// them, as jobsStores, once the shared database's jobs table passed the schema check (spec §7).
// A retry in an orphaned silo is refused (db.Orphans over the app's silo declarations, as the
// --jobs task refuses one); run's kind is the working tree's (jobs.ResolveKind, spec Decision 4),
// queued for whichever build serves. build is the current build; write, fail and notDeployed are
// runJobsEnv's. The lock is taken only after JobStores found the database, so a service without
// one creates nothing.
func runJobsStores(ctx context.Context, js deploy.JobStorer, r *deployRunner, t deploy.Target, at jobsPlace, build, op, target string, dead bool,
	write func(question string) (context.Context, func(), int, bool), fail func(code int, format string, a ...any) int, notDeployed func() int, stdout, stderr io.Writer) int {
	found, err := js.JobStores(ctx)
	switch {
	case errors.Is(err, deploy.ErrNotDeployed):
		return notDeployed()
	case err != nil:
		return fail(1, "%v", err)
	}
	stores := make([]jobsStore, len(found))
	for i, s := range found {
		silo, where := "", t.Describe()
		if s.Key != "" {
			silo = s.Key + "/" + s.Value
			where = "silo " + silo
		}
		open := s.Open
		stores[i] = jobsStore{silo: silo, where: where, readOld: true, open: func(ctx context.Context, write bool) (*db.SQLite, error) { return open(ctx, !write) }}
	}
	has, schema, err := jobsTableOf(ctx, stores[0])
	switch {
	case err != nil:
		return fail(2, "%s: %v", stores[0].where, err)
	case newerJobs(schema):
		return fail(2, "the jobs table at %s is schema %s, newer than this alder's %s: upgrade alder", t.Describe(), schema, db.JobsSchema)
	}
	noTable := fmt.Sprintf("no jobs table at %s: the app declares no job, or build %s was made before background jobs", t.Describe(), build)

	switch {
	case op == "": // list: read-only, no question, no lock
		if !has {
			printJobs(stdout, stderr, at.withSilos(len(stores)-1), jobsReport{counts: map[jobs.State]int{}})
			fmt.Fprintln(stdout, noTable)
			return 0
		}
		rep := gatherJobs(ctx, stores)
		return printJobs(stdout, stderr, at.withSilos(rep.silos), rep)

	case op == "retry" && !has:
		return fail(1, "%s", noTable)

	case op == "retry":
		question := fmt.Sprintf("queue job %s again at %s, due now? type yes: ", target, t.Describe())
		if dead {
			rep := gatherJobs(ctx, stores)
			if len(rep.dead) == 0 { // nothing to ask about or lock for
				fmt.Fprintln(stdout, "no dead job")
				for _, m := range rep.missed {
					fmt.Fprintf(stderr, "%s %s\n", errTag(stderr, "alder jobs:"), m)
				}
				if len(rep.missed) > 0 {
					return 2
				}
				return 0
			}
			question = fmt.Sprintf("queue the %d dead jobs at %s again, due now? type yes: ", len(rep.dead), t.Describe())
			if len(rep.dead) == 1 {
				question = fmt.Sprintf("queue the 1 dead job at %s again, due now? type yes: ", t.Describe())
			}
		}
		// The orphan check and the reads above happen before the question, and take no lock. The
		// lock (Render.Lock runs Turso.Preflight, which would create a missing database) is
		// reached only after JobStores found the database.
		if err := markOrphans(ctx, stores, r.prog); err != nil {
			return fail(2, "%s: %v", stores[0].where, err)
		}
		wctx, unlock, code, ok := write(question)
		if !ok {
			return code
		}
		defer unlock()
		if dead {
			return jobsRetryDead(wctx, at, stores, stdout, stderr)
		}
		return jobsRetry(wctx, at, stores, target, stdout, stderr)
	}

	// run <Kind>: the working tree's jobs resolve the kind; the row is queued for the build that
	// serves, whichever jobs it declares
	kinds, code := appJobKinds(r.dir, "alder jobs", stderr)
	if code != 0 {
		return code
	}
	k, err := jobs.ResolveKind(kinds, target)
	if err != nil {
		return fail(2, "%v", err)
	}
	wctx, unlock, code, ok := write(fmt.Sprintf("queue a run of %s at %s, build %s, due now? type yes: ", k.Name, t.Describe(), build))
	if !ok {
		return code
	}
	defer unlock()
	st, err := stores[0].open(wctx, true)
	if err != nil {
		return fail(2, "%s: %v", stores[0].where, err)
	}
	defer st.Close()
	var id string
	q, err := db.On(st, nil)
	if err == nil {
		err = jobs.EnsureTable(wctx, q)
	}
	if err == nil {
		id, err = jobs.Manual(wctx, q, k, time.Now())
	}
	if err != nil {
		return fail(2, "%s: %v", stores[0].where, err)
	}
	fmt.Fprintf(stdout, "queued a run of %s, due now: job %s at %s (not a schedule tick: the job's schedule is left as it is)\n", k.Name, id, t.Describe())
	fmt.Fprintf(stdout, "queued for build %s; a kind that build does not declare waits for one that does\n", build)
	return 0
}

// jobsTableOf is whether shared has a jobs table, and its jobs_schema ("" without one), read
// as a reader.
func jobsTableOf(ctx context.Context, shared jobsStore) (has bool, schema string, err error) {
	st, err := shared.open(ctx, false)
	if err != nil {
		return false, "", err
	}
	defer st.Close()
	q, err := db.On(st, nil)
	if err != nil {
		return false, "", err
	}
	if schema, err = db.JobsSchemaOf(ctx, q); err != nil {
		return false, "", err
	}
	var one int
	err = q.QueryRowContext(ctx, `SELECT 1 FROM sqlite_schema WHERE "type" = 'table' AND "name" = '_alder_jobs'`).Scan(&one)
	if errors.Is(err, sql.ErrNoRows) {
		return false, schema, nil
	}
	return err == nil, schema, err
}

// newerJobs is whether a jobs table of jobs_schema v was made by a newer alder than this one
// (db.JobsSchema): the schema only gains columns, so an older table is read fine, but a retry
// could leave a newer table's invariant unset. An unreadable v counts as newer.
func newerJobs(v string) bool {
	if v == "" {
		return false
	}
	n, err := strconv.Atoi(v)
	cur, _ := strconv.Atoi(db.JobsSchema)
	return err != nil || n > cur
}

// markOrphans marks each silo store of stores whose key row is gone from the shared store
// (stores[0]): the silos of the app's declarations prog (migrate.SiloDecls), checked as the
// --jobs task checks them (db.Orphans). Nothing is marked without a program or a silo store.
func markOrphans(ctx context.Context, stores []jobsStore, prog *ir.Program) error {
	if prog == nil || len(stores) < 2 {
		return nil
	}
	st, err := stores[0].open(ctx, false)
	if err != nil {
		return err
	}
	defer st.Close()
	var descs []db.TableDesc
	for _, tb := range migrate.FromProgram(prog).Tables {
		descs = append(descs, tb.Desc())
	}
	orphaned := db.Orphans(ctx, st, migrate.SiloDecls(prog), descs)
	for i := range stores[1:] {
		key, value, _ := strings.Cut(stores[i+1].silo, "/")
		stores[i+1].orphaned = orphaned(key, value)
	}
	return nil
}

// reportOf is the jobsReport of the --jobs task's list lines: each store's counts, queued ticks
// and dead rows, placed in its silo, and a missed line for each store the task could not read.
func reportOf(lines []jobs.AdminLine) jobsReport {
	r := jobsReport{counts: map[jobs.State]int{}}
	for _, l := range lines {
		if l.Counts == nil && l.Error == "" {
			continue // not a list line
		}
		if l.Silo != "" {
			r.silos++
		}
		if l.Error != "" {
			where := "the shared database"
			if l.Silo != "" {
				where = "silo " + l.Silo
			}
			r.missed = append(r.missed, where+": not read: "+l.Error)
			continue
		}
		for st, n := range l.Counts {
			r.counts[st] += n
		}
		for _, row := range l.Ticks {
			r.ticks = append(r.ticks, placed{Row: row, silo: l.Silo})
		}
		for _, row := range l.Dead {
			r.dead = append(r.dead, placed{Row: row, silo: l.Silo})
		}
	}
	return r
}

// withSilos is at with its header naming the n silo databases read (", and 1 silo database").
func (at jobsPlace) withSilos(n int) jobsPlace {
	if n > 0 {
		at.head += "," + siloWords(n, "and")
	}
	return at
}

// placedOf is the row of a retry line, where it was, and what is behind in its silo.
func placedOf(l jobs.AdminLine) placed {
	return placed{Row: *l.Row, silo: l.Silo, maintenance: l.Maintenance}
}

// oldBuild is whether out and code are a build's answer to a --jobs it does not know: its
// `maintain` prints its usage and exits 2 (no report line; a log line may come before).
func oldBuild(out string, code int) bool {
	if code != 2 {
		return false
	}
	lines, msgs := jobs.DecodeAdmin(out)
	if len(lines) > 0 {
		return false
	}
	for _, m := range msgs {
		if strings.HasPrefix(m, "usage:") {
			return true
		}
	}
	return false
}

// jobsExit is a --jobs task's exit code as the command's: 0, 1 and 2 are the task's own (done,
// refused, failed); any other (a shell's 127 for a missing run script, a signal's 128+n) is a
// target not reached, 1.
func jobsExit(code int) int {
	if code < 0 || code > 2 {
		return 1
	}
	return code
}
