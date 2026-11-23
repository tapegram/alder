package rtesting

import (
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"alder/rt"
	"alder/rt/db"
	"alder/rt/eff"
	"alder/rt/jobs"
	"alder/rt/std"
	"alder/rt/web"
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
		return eff.Bind(Jobs(), func(js []JobRow) eff.Task[bool] {
			before = js
			return eff.Bind(Work(kinds), func(rs []JobRun) eff.Task[bool] {
				runs = rs
				return eff.Bind(Jobs(), func(js []JobRow) eff.Task[bool] {
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
		return eff.Bind(Jobs(), func(js []JobRow) eff.Task[bool] {
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
		eff.Map(func(js []JobRow) bool { left = js; return true }, Jobs())))
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
		eff.Map(func(js []JobRow) bool { left = js; return true }, Jobs()))
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
				return eff.Map(func(js []JobRow) bool { left = js; return true }, Jobs())
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

// An app with `silo org { Project }` (spec §8): Test.jobs and Test.work cover the test's shared
// store and every test silo, oldest row first across them. A row enqueued by a request bound to
// org A's silo is in that silo; a `silo org` job queued in the shared store runs bound to its
// org's silo; ticks are only ever made in the shared store.
type siloProject struct{ ID, Name, Org string }

var (
	siloOrg = &db.Table[siloProject]{Name: "Org.Org", Columns: []db.Column{{Name: "id", Type: db.CText}, {Name: "name", Type: db.CText}}, ID: "id",
		Encode: func(p siloProject) []any { return []any{p.ID, p.Name} },
		Decode: func(v []any) (p siloProject, err error) {
			p.ID, _ = v[0].(string)
			p.Name, _ = v[1].(string)
			return p, nil
		},
		ReadID: func(p siloProject) string { return p.ID }, SetID: func(p siloProject, id string) siloProject { p.ID = id; return p }}
	siloProj = &db.Table[siloProject]{Name: "Project.Project", ID: "id",
		Columns: []db.Column{{Name: "id", Type: db.CText}, {Name: "name", Type: db.CText}, {Name: "org", Type: db.CText, Ref: "Org.Org", OnDelete: db.Cascade}},
		Encode:  func(p siloProject) []any { return []any{p.ID, p.Name, p.Org} },
		Decode: func(v []any) (p siloProject, err error) {
			p.ID, _ = v[0].(string)
			p.Name, _ = v[1].(string)
			p.Org, _ = v[2].(string)
			return p, nil
		},
		ReadID: func(p siloProject) string { return p.ID }, SetID: func(p siloProject, id string) siloProject { p.ID = id; return p },
		Owner: "org", Key: "org", Silo: "org"}
)

// asRequest runs t as a `requires org` request of org's would: narrowed and bound to its silo,
// Within the test's transaction (rt/testing's browser builds such an Env per request).
func asRequest[T any](org string, t eff.Task[T]) eff.Task[T] {
	return eff.Call("request", true, func(e *eff.Env) (T, error) {
		var zero T
		tx, err := e.Transaction()
		if err != nil {
			return zero, err
		}
		sp, _ := e.Store.(eff.Savepointer)
		r := &eff.Env{Ctx: e.Ctx, Store: e.Store, Stores: e.Stores, Caps: e.Caps, Now: e.Now, Within: &eff.Within{Tx: tx, Savepoints: sp}}
		db.Narrow(r, "org", org)
		if err := db.Enter(r, "org"); err != nil {
			return zero, err
		}
		return eff.Run(r, eff.KTask, t)
	})
}

// projectsOf counts org's projects, read as a request narrowed to org reads them.
func projectsOf(org string) eff.Task[int64] {
	return eff.Call("Db.count Project", false, func(e *eff.Env) (int64, error) {
		tx, err := e.Transaction()
		if err != nil {
			return 0, err
		}
		r := &eff.Env{Ctx: e.Ctx, Store: e.Store, Stores: e.Stores, Now: e.Now, Within: &eff.Within{Tx: tx}}
		db.Narrow(r, "org", org)
		return eff.RunQuery(r, eff.Q(eff.Call("Db.count Project", false, func(e *eff.Env) (int64, error) {
			return db.CountRows(e.Ctx, e.Store.(db.Store), db.TxOf(e), db.From(siloProj))
		})))
	})
}

// insertOrg inserts org id in the shared store, in the test's transaction (Db.insert would draw
// an id of its own).
func insertOrg(id string) eff.Task[struct{}] {
	return eff.Call("Db.insert Org", true, func(e *eff.Env) (struct{}, error) {
		tx, err := e.Transaction()
		if err != nil {
			return struct{}{}, err
		}
		q, err := db.On(e.Store, tx)
		if err == nil {
			_, err = q.ExecContext(e.Ctx, `INSERT INTO "Org__Org" ("id", "name") VALUES (?, ?)`, id, "org "+id)
		}
		return struct{}{}, err
	})
}

// siloRows is query's count in the test silo of org, in its transaction.
func siloRows(org, query string) eff.Task[int64] {
	return eff.Call("count", false, func(e *eff.Env) (int64, error) {
		silos, err := e.Stores.(*db.Stores).OpenSilos(e.Ctx)
		if err != nil {
			return 0, err
		}
		for _, s := range silos {
			if s.Key == "org" && s.Value == org {
				q, err := db.On(s.Store, s.Tx)
				if err != nil {
					return 0, err
				}
				var n int64
				return n, q.QueryRowContext(e.Ctx, query).Scan(&n)
			}
		}
		return 0, errors.New("no test silo org/" + org)
	})
}

func TestJobs_WorkAcrossSilos(t *testing.T) {
	inSilo := rt.Job{Kind: "Signup.InSilo", Params: `{}`}
	shared := rt.Job{Kind: "Signup.Shared", Params: `{}`}
	digest := rt.Job{Kind: "Reports.Digest", Params: `{"org":"A"}`}
	dk := jobKind(digest.Kind, nil)
	dk.SiloParam = "org"
	dk.Run = func(e *eff.Env, params string) (eff.Task[struct{}], error) {
		db.Narrow(e, "org", "A") // the generated Run narrows to the decoded parameter
		return eff.Call("Db.insert Project", true, func(e *eff.Env) (struct{}, error) {
			_, err := db.InsertRow(e.Ctx, e.Store.(db.Store), db.TxOf(e), siloProj, []any{nil, "digest", "A"}, e.Now())
			return struct{}{}, err
		}), nil
	}
	hourly := jobKind("Reports.Hourly", func() eff.Task[struct{}] { return eff.Pure(struct{}{}) })
	hourly.Every = time.Hour
	kinds := []jobs.Kind{
		jobKind(inSilo.Kind, func() eff.Task[struct{}] { return eff.Pure(struct{}{}) }),
		jobKind(shared.Kind, func() eff.Task[struct{}] { return eff.Pure(struct{}{}) }),
		dk, hourly,
	}
	var before, after []JobRow
	var runs []JobRun
	var projects int64 = -1
	var siloAll, siloDone int64 = -1, -1
	task := then(SetClock(setAt),
		then(then(insertOrg("A"), asRequest("A", std.JobEnqueue(inSilo))), // oldest, in silo A
			then(Advance(1000), then(std.JobEnqueue(shared), then(std.JobEnqueue(digest),
				eff.Bind(Jobs(), func(js []JobRow) eff.Task[bool] {
					before = js
					return eff.Bind(Work(kinds), func(rs []JobRun) eff.Task[bool] {
						runs = rs
						return eff.Bind(Jobs(), func(js []JobRow) eff.Task[bool] {
							after = js
							return eff.Bind(projectsOf("A"), func(n int64) eff.Task[bool] {
								projects = n
								return eff.Bind(siloRows("A", `SELECT COUNT(*) FROM "_alder_jobs"`), func(n int64) eff.Task[bool] {
									siloAll = n
									return eff.Map(func(n int64) bool { siloDone = n; return true }, siloRows("A", `SELECT COUNT(*) FROM "_alder_jobs" WHERE "kind" = 'Signup.InSilo' AND "state" = 'done'`))
								})
							})
						})
					})
				}))))))
	rs := Run([]Case{{Module: "M", Name: "silos", Task: func() eff.Task[bool] { return task }}}, Options{
		Tables: []db.TableDesc{siloOrg.Desc(), siloProj.Desc()},
		Web:    web.Options{Silos: []db.Silo{{Key: "org", Tables: []string{"Project.Project"}, Ref: "Org.Org"}}},
	})
	if len(rs) != 1 || !rs[0].OK {
		t.Fatalf("%+v", rs)
	}
	kindsOf := func(js []JobRow) (out []string) {
		for _, j := range js {
			out = append(out, j.Job.Kind+" "+[]string{"Queued", "Running", "Done", "Dead"}[j.Status.State.Tag])
		}
		return out
	}
	if got := kindsOf(before); !slices.Equal(got, []string{"Signup.InSilo Queued", "Signup.Shared Queued", "Reports.Digest Queued"}) {
		t.Fatalf("before %v", got)
	}
	var ran []string
	for _, r := range runs {
		ran = append(ran, r.Job.Kind)
		if !r.Result.IsOk {
			t.Fatalf("runs %+v", runs)
		}
	}
	if !slices.Equal(ran, []string{"Signup.InSilo", "Signup.Shared", "Reports.Digest"}) {
		t.Fatalf("ran %v: want oldest first across the stores", ran)
	}
	if got := kindsOf(after); !slices.Equal(got[:3], []string{"Signup.InSilo Done", "Signup.Shared Done", "Reports.Digest Done"}) || len(got) != 4 || got[3] != "Reports.Hourly Queued" {
		t.Fatalf("after %v", got)
	}
	if projects != 1 {
		t.Fatalf("org A's projects: %d", projects)
	}
	if siloAll != 1 || siloDone != 1 {
		t.Fatalf("silo A holds %d rows, %d of them the request's, done: want only that one (no tick)", siloAll, siloDone)
	}
}

// Test.work runs the highest priority first, as the worker's claim does.
func TestJobs_WorkPriorityOrder(t *testing.T) {
	low := rt.Job{Kind: "Q.Low", Params: `{}`}
	high := rt.Job{Kind: "Q.High", Params: `{}`, Priority: 10}
	noop := func() eff.Task[struct{}] { return eff.Pure(struct{}{}) }
	kinds := []jobs.Kind{jobKind(low.Kind, noop), jobKind(high.Kind, noop)}
	var runs []JobRun
	task := then(std.JobEnqueue(low), then(std.JobEnqueue(high), eff.Map(func(rs []JobRun) bool { runs = rs; return true }, Work(kinds))))
	if r := runJobs(t, task); !r.OK {
		t.Fatalf("%+v", r)
	}
	if len(runs) != 2 || runs[0].Job != high || runs[1].Job != low {
		t.Fatalf("runs %+v", runs)
	}
}

// A job a run enqueues is in the next read: a higher-priority one runs before the jobs already due.
func TestJobs_WorkEnqueuedHigherRunsNext(t *testing.T) {
	first := rt.Job{Kind: "Q.First", Params: `{}`}
	other := rt.Job{Kind: "Q.Other", Params: `{}`}
	urgent := rt.Job{Kind: "Q.Urgent", Params: `{}`, Priority: 10}
	noop := func() eff.Task[struct{}] { return eff.Pure(struct{}{}) }
	kinds := []jobs.Kind{
		jobKind(first.Kind, func() eff.Task[struct{}] { return enqueueUnit(urgent) }),
		jobKind(other.Kind, noop), jobKind(urgent.Kind, noop),
	}
	var runs []JobRun
	task := then(std.JobEnqueue(first), then(std.JobEnqueue(other), eff.Map(func(rs []JobRun) bool { runs = rs; return true }, Work(kinds))))
	if r := runJobs(t, task); !r.OK {
		t.Fatalf("%+v", r)
	}
	if len(runs) != 3 || runs[0].Job != first || runs[1].Job != urgent || runs[2].Job != other {
		t.Fatalf("runs %+v", runs)
	}
}

// The rt.Job values of Test.jobs and Test.work carry the row's priority.
func TestJobs_RowsCarryPriority(t *testing.T) {
	j := rt.Job{Kind: "Q.High", Params: `{}`, Priority: 10}
	kinds := []jobs.Kind{jobKind(j.Kind, func() eff.Task[struct{}] { return eff.Pure(struct{}{}) })}
	var listed []JobRow
	var runs []JobRun
	task := then(std.JobEnqueue(j), then(eff.Map(func(js []JobRow) bool { listed = js; return true }, Jobs()),
		eff.Map(func(rs []JobRun) bool { runs = rs; return true }, Work(kinds))))
	if r := runJobs(t, task); !r.OK {
		t.Fatalf("%+v", r)
	}
	if len(listed) != 1 || listed[0].Job != j || len(runs) != 1 || runs[0].Job != j {
		t.Fatalf("listed %+v runs %+v", listed, runs)
	}
}

// A job that cancels itself (spec §3): the cancel is in its own transaction, the done mark
// refuses, and the Task fails "cancelled" and rolls back, cancel included: the row is running.
func TestJobs_WorkSelfCancel(t *testing.T) {
	var id string
	kinds := []jobs.Kind{jobKind(welcomeJob.Kind, func() eff.Task[struct{}] {
		return then(insertUnit("half"), eff.Map(func(bool) struct{} { return struct{}{} }, std.JobCancel(id)))
	})}
	var runs []JobRun
	var left []JobRow
	var n int64 = -1
	task := eff.Bind(std.JobEnqueue(welcomeJob), func(got string) eff.Task[bool] {
		id = got
		return eff.Bind(Work(kinds), func(rs []JobRun) eff.Task[bool] {
			runs = rs
			return eff.Bind(Jobs(), func(js []JobRow) eff.Task[bool] {
				left = js
				return eff.Map(func(c int64) bool { n = c; return true }, countStep())
			})
		})
	})
	if r := runJobs(t, task); !r.OK {
		t.Fatalf("%+v", r)
	}
	if len(runs) != 1 || runs[0].Result.IsOk || runs[0].Result.Err != "cancelled" {
		t.Fatalf("runs %+v", runs)
	}
	if n != 0 {
		t.Fatalf("the self-cancelled job's write was kept: %d rows", n)
	}
	if len(left) != 1 || left[0].Status.State.Tag != std.JobRunning {
		t.Fatalf("left %+v", left)
	}
}

// A job that cancels another queued job: that row is Cancelled and Test.work does not run it.
func TestJobs_WorkCancelQueuedFromRun(t *testing.T) {
	b := rt.Job{Kind: "Signup.B", Params: `{}`}
	a := rt.Job{Kind: "Signup.A", Params: `{}`}
	var bID string
	kinds := []jobs.Kind{
		jobKind(a.Kind, func() eff.Task[struct{}] {
			return eff.Map(func(bool) struct{} { return struct{}{} }, std.JobCancel(bID))
		}),
		jobKind(b.Kind, func() eff.Task[struct{}] { return insertUnit("b ran") }),
	}
	var runs []JobRun
	var left []JobRow
	task := then(std.JobEnqueue(a), eff.Bind(std.JobEnqueue(b), func(got string) eff.Task[bool] {
		bID = got
		return eff.Bind(Work(kinds), func(rs []JobRun) eff.Task[bool] {
			runs = rs
			return eff.Map(func(js []JobRow) bool { left = js; return true }, Jobs())
		})
	}))
	if r := runJobs(t, task); !r.OK {
		t.Fatalf("%+v", r)
	}
	if len(runs) != 1 || runs[0].Job != a || !runs[0].Result.IsOk {
		t.Fatalf("runs %+v", runs)
	}
	if len(left) != 2 || left[1].Id != bID || left[1].Status.State.Tag != std.JobCancelled {
		t.Fatalf("left %+v", left)
	}
}
