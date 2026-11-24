package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os/signal"
	"strings"
	"syscall"

	"alder/internal/deploy"
	"alder/rt/jobs"
)

// `alder jobs --env <name>` (#285, spec §1, §3, §6-§8): the operations of `alder jobs` against a
// deployed environment. At a box or a machine (Ssh, with a file or on Turso; Fly) the current
// build's executable runs its --jobs task there (deploy.JobsRunner, rt/jobs/admintask.go), whose
// JSON lines (jobs.AdminLine) are read back into the local command's rows and printed by its
// printers. A write asks first (--yes answers) and runs under the deploy lock.

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
	t, _, err := envTarget("jobs", env, dir, host, e)
	if err != nil {
		return fail(2, "%v", err)
	}
	defer t.Close()
	jr, ok := t.(deploy.JobsRunner)
	if _, render := t.(*deploy.Render); render && !ok {
		return fail(2, "alder jobs --env does not reach a Render environment yet")
	}
	if !ok { // every target but Script (refused above) and Render runs the task
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
	// write asks question unless --yes, then takes the deploy lock: ok false when the command is
	// over (code is its exit); release it with unlock. The question is asked with the default
	// signal handling, so an interrupt there ends the command (as restore's); from the lock on,
	// SIGINT and SIGTERM cancel ctx instead, so the lock is released (the reads before keep the
	// default: they hold nothing).
	write := func(question string) (unlock func(), code int, ok bool) {
		if !yes && !confirm(question, stdout) {
			return nil, fail(2, "not confirmed: nothing was queued (pass --yes to confirm without a terminal)"), false
		}
		var stop context.CancelFunc
		ctx, stop = signal.NotifyContext(ctx, syscall.SIGINT, syscall.SIGTERM) // task runs on it too
		if l, isLocker := t.(deploy.Locker); isLocker {
			u, err := l.Lock(ctx)
			if err != nil {
				stop()
				return nil, fail(1, "%v", err), false
			}
			return func() { u(); stop() }, 0, true
		}
		return stop, 0, true
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
		unlock, code, ok := write(question)
		if !ok {
			return code
		}
		defer unlock()
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
		unlock, code, ok := write(fmt.Sprintf("queue job %s again at %s, due now? type yes: ", target, t.Describe()))
		if !ok {
			return code
		}
		defer unlock()
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
	unlock, code, ok := write(fmt.Sprintf("queue a run of %s at %s, build %s, due now? type yes: ", target, t.Describe(), id))
	if !ok {
		return code
	}
	defer unlock()
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
