package rtesting

import (
	"slices"
	"strings"
	"testing"
	"time"

	"alder/rt"
	"alder/rt/eff"
	"alder/rt/jobs"
	"alder/rt/std"
)

// welcomeJob is the value of `SendWelcome { … }` in module Signup.
var welcomeJob = rt.Job{Kind: "Signup.SendWelcome", Params: `{"user":"u1"}`}

// jobKind is a kind named name whose run is task, with the spec's defaults (4 retries, 5m).
func jobKind(name string, task func() eff.Task[struct{}]) jobs.Kind {
	return jobs.Kind{
		Name:    name,
		Run:     func(*eff.Env, string) (eff.Task[struct{}], error) { return task(), nil },
		Retries: 4,
		Timeout: 5 * time.Minute,
	}
}

// insertUnit is insertStep without its id.
func insertUnit(title string) eff.Task[struct{}] {
	return eff.Map(func(string) struct{} { return struct{}{} }, insertStep("Db.insert T", title))
}

// enqueueUnit is Job.enqueue j without its id.
func enqueueUnit(j rt.Job) eff.Task[struct{}] {
	return eff.Map(func(string) struct{} { return struct{}{} }, std.JobEnqueue(j))
}

// runJobs runs task as the one case of a test with the store's tables and gives its result.
func runJobs(t *testing.T, task eff.Task[bool]) CaseResult {
	t.Helper()
	rs := one(Case{Module: "M", Name: "jobs", Task: func() eff.Task[bool] { return task }})
	if len(rs) != 1 {
		t.Fatalf("%+v", rs)
	}
	return rs[0]
}

// The spec's welcome mail, in Go: an enqueued job is listed, Test.work runs it Ok, and its
// write is the test's; Test.jobs then shows it Done with one attempt.
func TestJobs_WorkRunsAnEnqueuedJob(t *testing.T) {
	kinds := []jobs.Kind{jobKind(welcomeJob.Kind, func() eff.Task[struct{}] { return insertUnit("mail") })}
	var before, after []JobRow
	var runs []JobRun
	var n int64
	task := then(SetClock(setAt), eff.Bind(std.JobEnqueue(welcomeJob), func(id string) eff.Task[bool] {
		return eff.Bind(Jobs(kinds), func(js []JobRow) eff.Task[bool] {
			before = js
			return eff.Bind(Work(kinds), func(rs []JobRun) eff.Task[bool] {
				runs = rs
				return eff.Bind(Jobs(kinds), func(js []JobRow) eff.Task[bool] {
					after = js
					return eff.Map(func(c int64) bool { n = c; return true }, countStep())
				})
			})
		})
	}))
	r := runJobs(t, task)
	if !r.OK {
		t.Fatalf("%+v", r)
	}
	if len(before) != 1 || before[0].Job != welcomeJob || before[0].Status.State.Tag != std.JobQueued ||
		before[0].Status.Attempts != 0 || !before[0].Status.RunAt.Ok || !before[0].Status.RunAt.Val.Equal(setAt) {
		t.Fatalf("before %+v", before)
	}
	if len(runs) != 1 || runs[0].Id != before[0].Id || runs[0].Job != welcomeJob || !runs[0].Result.IsOk {
		t.Fatalf("runs %+v", runs)
	}
	if n != 1 {
		t.Fatalf("the job's write: %d rows", n)
	}
	if len(after) != 1 || after[0].Status.State.Tag != std.JobDone || after[0].Status.Attempts != 1 || after[0].Status.RunAt.Ok || after[0].Status.Error.Ok {
		t.Fatalf("after %+v", after)
	}
}

// work runs nothing that is not due: a job a minute ahead waits for Test.advance.
func TestJobs_WorkRunsNothingNotDue(t *testing.T) {
	kinds := []jobs.Kind{jobKind(welcomeJob.Kind, func() eff.Task[struct{}] { return eff.Pure(struct{}{}) })}
	var first, second []JobRun
	task := then(SetClock(setAt), then(std.JobEnqueueAt(setAt.Add(time.Minute), welcomeJob),
		eff.Bind(Work(kinds), func(rs []JobRun) eff.Task[bool] {
			first = rs
			return then(Advance(60000), eff.Map(func(rs []JobRun) bool { second = rs; return true }, Work(kinds)))
		})))
	if r := runJobs(t, task); !r.OK {
		t.Fatalf("%+v", r)
	}
	if first == nil || len(first) != 0 {
		t.Fatalf("first work %#v, want []", first)
	}
	if len(second) != 1 || !second[0].Result.IsOk {
		t.Fatalf("second work %+v", second)
	}
}

// A failing job is retried on the clock: its writes are undone, its row is queued again with
// attempt 1, its error and a run_at 10 s on.
func TestJobs_WorkFailureRetriesOnTheClock(t *testing.T) {
	kinds := []jobs.Kind{jobKind(welcomeJob.Kind, func() eff.Task[struct{}] { return then(insertUnit("half"), eff.Fail[struct{}]("smtp down")) })}
	var runs []JobRun
	var left []JobRow
	var n int64 = -1
	task := then(SetClock(setAt), then(std.JobEnqueue(welcomeJob), eff.Bind(Work(kinds), func(rs []JobRun) eff.Task[bool] {
		runs = rs
		return eff.Bind(Jobs(kinds), func(js []JobRow) eff.Task[bool] {
			left = js
			return eff.Map(func(c int64) bool { n = c; return true }, countStep())
		})
	})))
	if r := runJobs(t, task); !r.OK {
		t.Fatalf("%+v", r)
	}
	if len(runs) != 1 || runs[0].Result.IsOk || runs[0].Result.Err != "smtp down" {
		t.Fatalf("runs %+v", runs)
	}
	if len(left) != 1 {
		t.Fatalf("left %+v", left)
	}
	s := left[0].Status
	if s.State.Tag != std.JobQueued || s.Attempts != 1 || !s.Error.Ok || s.Error.Val != "smtp down" ||
		!s.RunAt.Ok || !s.RunAt.Val.Equal(setAt.Add(10*time.Second)) {
		t.Fatalf("left %+v", left)
	}
	if n != 0 {
		t.Fatalf("the failed job's write was kept: %d rows", n)
	}
}

// A job past its retries is dead.
func TestJobs_WorkFailurePastRetriesIsDead(t *testing.T) {
	k := jobKind(welcomeJob.Kind, func() eff.Task[struct{}] { return eff.Fail[struct{}]("no") })
	k.Retries = 0
	var left []JobRow
	task := then(std.JobEnqueue(welcomeJob), then(Work([]jobs.Kind{k}),
		eff.Map(func(js []JobRow) bool { left = js; return true }, Jobs([]jobs.Kind{k}))))
	if r := runJobs(t, task); !r.OK {
		t.Fatalf("%+v", r)
	}
	if len(left) != 1 || left[0].Status.State.Tag != std.JobDead || left[0].Status.Attempts != 1 || left[0].Status.RunAt.Ok {
		t.Fatalf("left %+v", left)
	}
}

// Jobs that enqueue jobs run in the same work, in order.
func TestJobs_WorkRunsJobsEnqueuedByJobs(t *testing.T) {
	b := rt.Job{Kind: "Signup.B", Params: `{}`}
	a := rt.Job{Kind: "Signup.A", Params: `{}`}
	kinds := []jobs.Kind{
		jobKind(a.Kind, func() eff.Task[struct{}] { return enqueueUnit(b) }),
		jobKind(b.Kind, func() eff.Task[struct{}] { return eff.Pure(struct{}{}) }),
	}
	var runs []JobRun
	task := then(std.JobEnqueue(a), eff.Map(func(rs []JobRun) bool { runs = rs; return true }, Work(kinds)))
	if r := runJobs(t, task); !r.OK {
		t.Fatalf("%+v", r)
	}
	if len(runs) != 2 || runs[0].Job != a || runs[1].Job != b || !runs[0].Result.IsOk || !runs[1].Result.IsOk {
		t.Fatalf("runs %+v", runs)
	}
}

// A job that enqueues itself forever fails the test at the cap.
func TestJobs_WorkCap(t *testing.T) {
	kinds := []jobs.Kind{jobKind(welcomeJob.Kind, func() eff.Task[struct{}] { return enqueueUnit(welcomeJob) })}
	r := runJobs(t, then(std.JobEnqueue(welcomeJob), eff.Map(func([]JobRun) bool { return true }, Work(kinds))))
	if r.OK || r.Error != "Test.work: jobs keep enqueueing jobs" {
		t.Fatalf("%+v", r)
	}
}

// A Test.run that enqueues and then fails leaves no job.
func TestJobs_FailedRunLeavesNoJob(t *testing.T) {
	var left []JobRow
	task := then(testRun(then(std.JobEnqueue(welcomeJob), eff.Fail[int]("refused"))),
		eff.Map(func(js []JobRow) bool { left = js; return true }, Jobs(nil)))
	if r := runJobs(t, task); !r.OK {
		t.Fatalf("%+v", r)
	}
	if left == nil || len(left) != 0 {
		t.Fatalf("left %#v, want []", left)
	}
}

// The trace names the job a step enqueues.
func TestJobs_TraceNamesTheJob(t *testing.T) {
	var trace []string
	r := runJobs(t, then(std.JobEnqueue(welcomeJob), traceInto(&trace)))
	if !r.OK || !slices.Contains(trace, "Job.enqueue SendWelcome") {
		t.Fatalf("%+v trace %v", r, trace)
	}
	if r.OK && strings.Contains(strings.Join(trace, "\n"), "Signup.SendWelcome") {
		t.Fatalf("trace %v", trace)
	}
}

// Schedule ticks in a test (spec §8): the first Test.work creates the tick from the clock, so
// with the clock at 02:59 UTC a nightly job runs once after Test.advance of two minutes; after
// three days more it runs once, not three times.
func TestJobs_WorkTicks(t *testing.T) {
	nightly := jobKind("Jobs.Nightly", func() eff.Task[struct{}] { return eff.Pure(struct{}{}) })
	nightly.Cron = "0 3 * * *"
	kinds := []jobs.Kind{nightly}
	var first, second, third []JobRun
	var left []JobRow
	task := then(SetClock(time.Date(2026, 3, 4, 2, 59, 0, 0, time.UTC)), eff.Bind(Work(kinds), func(rs []JobRun) eff.Task[bool] {
		first = rs
		return then(Advance(120000), eff.Bind(Work(kinds), func(rs []JobRun) eff.Task[bool] {
			second = rs
			return then(Advance(3*86400000), eff.Bind(Work(kinds), func(rs []JobRun) eff.Task[bool] {
				third = rs
				return eff.Map(func(js []JobRow) bool { left = js; return true }, Jobs(kinds))
			}))
		}))
	}))
	if r := runJobs(t, task); !r.OK {
		t.Fatalf("%+v", r)
	}
	tick := rt.Job{Kind: "Jobs.Nightly", Params: "{}"}
	if len(first) != 0 || len(second) != 1 || second[0].Job != tick || !second[0].Result.IsOk || len(third) != 1 || !third[0].Result.IsOk {
		t.Fatalf("runs: %+v, %+v, %+v", first, second, third)
	}
	// two done, and the next tick queued for 8 March 03:00
	if len(left) != 3 || left[2].Status.State.Tag != std.JobQueued || !left[2].Status.RunAt.Val.Equal(time.Date(2026, 3, 8, 3, 0, 0, 0, time.UTC)) {
		t.Fatalf("left %+v", left)
	}
}
