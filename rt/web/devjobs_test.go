package web

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"alder/rt/applog"
	"alder/rt/db"
	"alder/rt/eff"
	"alder/rt/jobs"
)

var devJobsT0 = time.Date(2026, 11, 16, 12, 0, 0, 0, time.UTC)

// jobsStore is a file store with the jobs table.
func jobsStore(t *testing.T) *db.SQLite {
	t.Helper()
	st, err := db.OpenSQLite(filepath.Join(t.TempDir(), "app.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	if err := st.Ensure(context.Background(), nil, true); err != nil { // the jobs table
		t.Fatal(err)
	}
	return st
}

// insertJob inserts a row; finished and errTxt are left NULL when zero.
func insertJob(t *testing.T, st *db.SQLite, id, kind, params, state string, runAt time.Time, attempts int, finished time.Time, errTxt string) {
	t.Helper()
	q, err := db.On(st, nil)
	if err != nil {
		t.Fatal(err)
	}
	var fin, e any
	if !finished.IsZero() {
		fin = finished.UnixMilli()
	}
	if errTxt != "" {
		e = errTxt
	}
	var started any
	if state != "queued" {
		started = runAt.UnixMilli()
	}
	if _, err := q.ExecContext(context.Background(), `INSERT INTO "_alder_jobs" ("id", "kind", "params", "state", "run_at", "created", "started", "finished", "attempts", "error") VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		id, kind, params, state, runAt.UnixMilli(), runAt.UnixMilli(), started, fin, attempts, e); err != nil {
		t.Fatal(err)
	}
}

var (
	sendKind    = jobs.Kind{Name: "Signup.SendWelcome", Retries: 4, Run: func(*eff.Env, string) (eff.Task[struct{}], error) { return eff.Pure(struct{}{}), nil }}
	nightlyKind = jobs.Kind{Name: "Signup.Nightly", Every: time.Hour, Run: func(*eff.Env, string) (eff.Task[struct{}], error) { return eff.Pure(struct{}{}), nil }}
)

// jobsFixture is a store with a queued row due at t0 + 5m, a running one, a dead one, 55 done
// ones (done00 finished first) and Nightly's tick (every 1h: due 13:00), and a dev router whose
// panel reads it at t0.
func jobsFixture(t *testing.T) (*db.SQLite, *jobs.Panel, *router) {
	t.Helper()
	st := jobsStore(t)
	insertJob(t, st, "queued01", sendKind.Name, `{"user":"u1"}`, "queued", devJobsT0.Add(5*time.Minute), 0, time.Time{}, "")
	insertJob(t, st, "runnin01", sendKind.Name, `{"user":"u2"}`, "running", devJobsT0.Add(-time.Minute), 1, time.Time{}, "")
	insertJob(t, st, "deadjob1", sendKind.Name, `{"user":"u3"}`, "dead", devJobsT0.Add(-time.Hour), 5, devJobsT0.Add(-30*time.Minute), jobs.LostError)
	insertJob(t, st, "queued02", sendKind.Name, `{"user":"u4"}`, "queued", devJobsT0.Add(10*time.Minute), 0, time.Time{}, "")
	q, err := db.On(st, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := q.ExecContext(context.Background(), `UPDATE "_alder_jobs" SET "priority" = 10 WHERE "id" = 'queued02'`); err != nil {
		t.Fatal(err)
	}
	for i := range 55 {
		at := devJobsT0.Add(-time.Duration(60-i) * time.Minute)
		insertJob(t, st, fmt.Sprintf("done%04d", i), sendKind.Name, `{"user":"u"}`, "done", at, 1, at.Add(time.Second), "")
	}
	if err := jobs.EnsureTicks(context.Background(), st, []jobs.Kind{nightlyKind}, devJobsT0); err != nil {
		t.Fatal(err)
	}
	p := &jobs.Panel{Store: st, Kinds: []jobs.Kind{sendKind, nightlyKind}, Now: func() time.Time { return devJobsT0 }}
	return st, p, devRouter(t, nil, func(o *Options) { o.Store, o.DevJobs = st, p })
}

// Spec §9: /_dev/jobs lists queued rows with their due time, running, dead, the last fifty done,
// and each scheduled kind with its next time.
func TestDevJobs_List(t *testing.T) {
	_, _, h := jobsFixture(t)
	w := serve(h, httptest.NewRequest("GET", "/_dev/jobs", nil))
	body := w.Body.String()
	if w.Code != 200 || !strings.HasPrefix(w.Header().Get("Content-Type"), "text/html") {
		t.Fatalf("%d %q", w.Code, w.Header().Get("Content-Type"))
	}
	for _, want := range []string{
		`href="/_dev/jobs/queued01"`, "2026-11-16 12:05:00 UTC", // queued, with its due time
		`href="/_dev/jobs/runnin01"`,
		`href="/_dev/jobs/deadjob1"`, "lost: the process stopped",
		`href="/_dev/jobs/done0054"`, `href="/_dev/jobs/done0005"`, // the last fifty done
		"Signup.Nightly", "every 1h", "2026-11-16 13:00:00 UTC", // a schedule's next tick
		"Done (last 50)",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("/_dev/jobs lacks %q:\n%s", want, body)
		}
	}
	if !strings.Contains(body, "priority 10") || strings.Count(body, "priority ") != 1 {
		t.Errorf("/_dev/jobs does not show priority 10 once (and no priority 0):\n%s", body)
	}
	if i, j := strings.Index(body, `href="/_dev/jobs/queued02"`), strings.Index(body, `href="/_dev/jobs/queued01"`); i < 0 || j < 0 || i > j {
		t.Errorf("the priority 10 row is not before the priority 0 row (%d, %d)", i, j)
	}
	if strings.Contains(body, `href="/_dev/jobs/done0004"`) {
		t.Errorf("/_dev/jobs shows more than the last fifty done rows")
	}
	if !strings.Contains(body, "No job worker runs in this process") {
		t.Errorf("/_dev/jobs does not say no worker runs here:\n%s", body)
	}
}

// /_dev/jobs.json carries the page's rows, times in milliseconds since the epoch.
func TestDevJobs_JSON(t *testing.T) {
	_, _, h := jobsFixture(t)
	w := serve(h, httptest.NewRequest("GET", "/_dev/jobs.json", nil))
	if w.Code != 200 || !strings.HasPrefix(w.Header().Get("Content-Type"), "application/json") || w.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("%d %q %s", w.Code, w.Header().Get("Content-Type"), w.Body.String())
	}
	var v JobsView
	if err := json.Unmarshal(w.Body.Bytes(), &v); err != nil {
		t.Fatalf("%v: %s", err, w.Body.String())
	}
	if len(v.Queued) != 3 || v.Queued[0].ID != "queued02" || v.Queued[0].Priority != 10 || v.Queued[1].ID != "queued01" || v.Queued[1].Priority != 0 ||
		v.Queued[1].RunAt != devJobsT0.Add(5*time.Minute).UnixMilli() || !v.Queued[2].Tick {
		t.Errorf("queued = %+v", v.Queued)
	}
	if len(v.Running) != 1 || v.Running[0].ID != "runnin01" || v.Running[0].Started == nil {
		t.Errorf("running = %+v", v.Running)
	}
	if len(v.Dead) != 1 || v.Dead[0].Error == nil || *v.Dead[0].Error != jobs.LostError || v.Dead[0].Attempts != 5 {
		t.Errorf("dead = %+v", v.Dead)
	}
	if len(v.Done) != 50 || v.Done[0].ID != "done0054" || v.Done[49].ID != "done0005" {
		t.Errorf("done: %d rows, first %+v", len(v.Done), v.Done[0])
	}
	if len(v.Schedules) != 1 || v.Schedules[0].Kind != "Signup.Nightly" || v.Schedules[0].Rule != "every 1h" ||
		v.Schedules[0].Next == nil || *v.Schedules[0].Next != devJobsT0.Add(time.Hour).UnixMilli() || v.Schedules[0].ID != v.Queued[2].ID {
		t.Errorf("schedules = %+v", v.Schedules)
	}
	if v.Workers {
		t.Errorf("workers = true, want false: none started")
	}
}

// POST Retry on a dead row queues it again with attempts 0 and no error; Run now on a queued
// row makes it due now. Each answers 303 to the row's page; a row in another state is 409.
func TestDevJobs_Retry(t *testing.T) {
	st, p, h := jobsFixture(t)
	post := func(path string) *httptest.ResponseRecorder {
		return serve(h, httptest.NewRequest("POST", path, nil))
	}
	if w := post("/_dev/jobs/deadjob1/retry"); w.Code != 303 || w.Header().Get("Location") != "/_dev/jobs/deadjob1" {
		t.Fatalf("retry: %d %q %s", w.Code, w.Header().Get("Location"), w.Body.String())
	}
	r, err := p.Get(context.Background(), "deadjob1")
	if err != nil || r == nil || r.State != "queued" || r.Attempts != 0 || r.Error != nil || r.RunAt != devJobsT0.UnixMilli() {
		t.Fatalf("after retry: %+v %v", r, err)
	}
	if w := post("/_dev/jobs/queued01/run"); w.Code != 303 || w.Header().Get("Location") != "/_dev/jobs/queued01" {
		t.Fatalf("run now: %d %s", w.Code, w.Body.String())
	}
	if r, _ := p.Get(context.Background(), "queued01"); r == nil || r.RunAt != devJobsT0.UnixMilli() || r.State != "queued" {
		t.Fatalf("after run now: %+v", r)
	}
	if w := post("/_dev/jobs/done0001/retry"); w.Code != 409 || !strings.Contains(w.Body.String(), "only a dead job is retried") {
		t.Errorf("retry of a done row: %d %s", w.Code, w.Body.String())
	}
	if w := post("/_dev/jobs/runnin01/run"); w.Code != 409 {
		t.Errorf("run now of a running row: %d %s", w.Code, w.Body.String())
	}
	if w := post("/_dev/jobs/nosuch01/retry"); w.Code != 404 {
		t.Errorf("retry of no row: %d %s", w.Code, w.Body.String())
	}
	// another site's form cannot post to the dev server
	req := httptest.NewRequest("POST", "/_dev/jobs/queued01/run", nil)
	req.Header.Set("Sec-Fetch-Site", "cross-site")
	if w := serve(h, req); w.Code != 403 {
		t.Errorf("a cross-site POST: %d", w.Code)
	}
	_ = st
}

// A dead tick retried runs once and inserts no next tick: the kind's queued tick is left alone.
func TestDevJobs_RetryTick(t *testing.T) {
	st := jobsStore(t)
	ctx := context.Background()
	if err := jobs.EnsureTicks(ctx, st, []jobs.Kind{nightlyKind}, devJobsT0); err != nil {
		t.Fatal(err)
	}
	// the 13:00 tick claimed at 13:00 (the 14:00 tick inserted), then dead
	r, err := jobs.Claim(ctx, st, []jobs.Kind{nightlyKind}, devJobsT0.Add(time.Hour), "w")
	if err != nil || r == nil {
		t.Fatalf("claim: %+v %v", r, err)
	}
	q, _ := db.On(st, nil)
	if _, err := q.ExecContext(ctx, `UPDATE "_alder_jobs" SET "state" = 'dead', "error" = 'boom' WHERE "id" = ?`, r.ID); err != nil {
		t.Fatal(err)
	}
	p := &jobs.Panel{Store: st, Kinds: []jobs.Kind{nightlyKind}, Now: func() time.Time { return devJobsT0.Add(90 * time.Minute) }}
	h := devRouter(t, nil, func(o *Options) { o.Store, o.DevJobs = st, p })
	page := serve(h, httptest.NewRequest("GET", "/_dev/jobs/"+r.ID, nil)).Body.String()
	if !strings.Contains(page, "A retried tick runs once, now, and inserts no next tick") {
		t.Errorf("the dead tick's page does not say what its retry does:\n%s", page)
	}
	if w := serve(h, httptest.NewRequest("POST", "/_dev/jobs/"+r.ID+"/retry", nil)); w.Code != 303 {
		t.Fatalf("retry: %d %s", w.Code, w.Body.String())
	}
	again, err := jobs.Claim(ctx, st, []jobs.Kind{nightlyKind}, devJobsT0.Add(90*time.Minute), "w")
	if err != nil || again == nil || again.ID != r.ID || again.Schedule {
		t.Fatalf("claim of the retried tick: %+v %v", again, err)
	}
	var ticks int
	var next int64
	if err := q.QueryRowContext(ctx, `SELECT COUNT(*), MAX("run_at") FROM "_alder_jobs" WHERE "schedule" = 1 AND "state" = 'queued'`).Scan(&ticks, &next); err != nil {
		t.Fatal(err)
	}
	if ticks != 1 || next != devJobsT0.Add(2*time.Hour).UnixMilli() {
		t.Errorf("queued ticks after the retried tick ran: %d, due %v; want the 14:00 tick alone", ticks, time.UnixMilli(next).UTC())
	}
}

// Spec §9, §6a: /_dev/jobs/{id} shows the row's params, error and attempts and the last
// attempt's trace, marked when it held the write lock more than 2 s.
func TestDevJobs_Detail(t *testing.T) {
	st := jobsStore(t)
	c := &stepClock{t: devJobsT0}
	// a job that writes first, then waits 41 s (an AI call) with the transaction open
	slow := jobs.Kind{Name: "Digest.Send", Retries: 2, Run: func(env *eff.Env, params string) (eff.Task[struct{}], error) {
		return eff.Call("Db.update", true, func(e *eff.Env) (struct{}, error) {
			pl, err := db.RuntimeOn(e)
			if err != nil {
				return struct{}{}, err
			}
			q, err := db.On(pl.Store, pl.Tx)
			if err != nil {
				return struct{}{}, err
			}
			if _, err := q.ExecContext(e.Ctx, `UPDATE "_alder_jobs" SET "key" = NULL WHERE "id" = 'nothing'`); err != nil {
				return struct{}{}, err
			}
			c.Set(devJobsT0.Add(41 * time.Second))
			return struct{}{}, nil
		}), nil
	}}
	bad := jobs.Kind{Name: "Signup.SendWelcome", Retries: 4, Run: func(*eff.Env, string) (eff.Task[struct{}], error) {
		return eff.Task[struct{}]{}, errors.New(`user: missing`)
	}}
	insertJob(t, st, "slowjob1", slow.Name, `{"day":"2026-11-16"}`, "queued", devJobsT0, 0, time.Time{}, "")
	insertJob(t, st, "badjob01", bad.Name, `{"usr":"u1"}`, "queued", devJobsT0, 0, time.Time{}, "")
	p := &jobs.Panel{Store: st, Kinds: []jobs.Kind{slow, bad}, Now: c.Now}
	stop := jobs.Start(context.Background(), jobs.Options{
		Store: st, Kinds: []jobs.Kind{slow, bad}, Workers: 1, Now: c.Now, Panel: p,
		Env:  func(ctx context.Context, k jobs.Kind) *eff.Env { return &eff.Env{Ctx: ctx, Store: st} },
		Poll: 10 * time.Millisecond, Housekeep: time.Hour,
	})
	t.Cleanup(func() { stop(0) })
	for deadline := time.Now().Add(10 * time.Second); ; time.Sleep(time.Millisecond) {
		_, a := p.Last("slowjob1")
		_, b := p.Last("badjob01")
		if a && b {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("the workers never ran both jobs")
		}
	}
	h := devRouter(t, nil, func(o *Options) { o.Store, o.DevJobs = st, p })
	w := serve(h, httptest.NewRequest("GET", "/_dev/jobs/slowjob1", nil))
	body := w.Body.String()
	if w.Code != 200 {
		t.Fatalf("%d %s", w.Code, body)
	}
	for _, want := range []string{
		"Digest.Send", "done", `{&#34;day&#34;:&#34;2026-11-16&#34;}`, // params
		"attempts 1 of 3",
		"Db.update", // the trace's step
		`UPDATE &#34;_alder_jobs&#34; SET &#34;key&#34; = NULL`, // its statement
		"held the write lock 41 s: write last, or fan out — book 17",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("/_dev/jobs/slowjob1 lacks %q:\n%s", want, body)
		}
	}
	body = serve(h, httptest.NewRequest("GET", "/_dev/jobs/badjob01", nil)).Body.String()
	for _, want := range []string{"dead", "cannot decode the parameters: user: missing", `action="/_dev/jobs/badjob01/retry"`, "attempts 1 of 5"} {
		if !strings.Contains(body, want) {
			t.Errorf("/_dev/jobs/badjob01 lacks %q:\n%s", want, body)
		}
	}
	if strings.Contains(body, "held the write lock") {
		t.Errorf("a job that held no lock for long is marked:\n%s", body)
	}
	if w := serve(h, httptest.NewRequest("GET", "/_dev/jobs/nosuch01", nil)); w.Code != 404 {
		t.Errorf("no such job: %d", w.Code)
	}
	// the list says workers run here
	if body := serve(h, httptest.NewRequest("GET", "/_dev/jobs", nil)).Body.String(); strings.Contains(body, "No job worker runs in this process") {
		t.Errorf("/_dev/jobs says no worker runs while one does")
	}
}

// Outside the dev server, /_dev/jobs and its JSON and actions are not served.
func TestDevJobs_NotInProd(t *testing.T) {
	st := jobsStore(t)
	insertJob(t, st, "deadjob1", sendKind.Name, `{}`, "dead", devJobsT0, 5, devJobsT0, "boom")
	p := &jobs.Panel{Store: st, Kinds: []jobs.Kind{sendKind}}
	h := NewRouter(nil, testOpts(func(o *Options) { o.Store, o.DevJobs = st, p }))
	for _, rq := range []struct{ method, path string }{{"GET", "/_dev/jobs"}, {"GET", "/_dev/jobs.json"}, {"GET", "/_dev/jobs/deadjob1"}, {"POST", "/_dev/jobs/deadjob1/retry"}} {
		if w := serve(h, httptest.NewRequest(rq.method, rq.path, nil)); w.Code != 404 {
			t.Errorf("%s %s outside dev mode: %d", rq.method, rq.path, w.Code)
		}
	}
	if r, _ := p.Get(context.Background(), "deadjob1"); r == nil || r.State != "dead" {
		t.Errorf("the row changed: %+v", r)
	}
	// dev mode without jobs (an app that declares none): not served either
	h = devRouter(t, nil, nil)
	if w := serve(h, httptest.NewRequest("GET", "/_dev/jobs", nil)); w.Code != 404 {
		t.Errorf("/_dev/jobs of an app without jobs: %d", w.Code)
	}
}

// The dev map and the dev panel's bar link to the Jobs page in an app with jobs, and only there.
func TestDevJobs_Nav(t *testing.T) {
	_, _, h := jobsFixture(t)
	if body := serve(h, httptest.NewRequest("GET", "/_dev", nil)).Body.String(); !strings.Contains(body, `<a href="/_dev/jobs">Jobs</a>`) {
		t.Errorf("/_dev does not link to the Jobs page:\n%s", body)
	}
	st := jobsStore(t)
	page := devRouter(t, []Route{pageRoute("/a")}, func(o *Options) { o.Store = st; o.DevJobs = &jobs.Panel{Store: st} })
	if body := serve(page, httptest.NewRequest("GET", "/a", nil)).Body.String(); !strings.Contains(body, `<a href="/_dev/jobs" data-jobs>Jobs</a>`) {
		t.Errorf("the dev panel's bar does not link to the Jobs page:\n%s", body)
	}
	plain := devRouter(t, []Route{pageRoute("/a")}, nil)
	if body := serve(plain, httptest.NewRequest("GET", "/a", nil)).Body.String(); strings.Contains(body, "/_dev/jobs") {
		t.Errorf("an app without jobs links to the Jobs page")
	}
	if body := serve(plain, httptest.NewRequest("GET", "/_dev", nil)).Body.String(); strings.Contains(body, "/_dev/jobs") {
		t.Errorf("/_dev of an app without jobs links to the Jobs page")
	}
}

// A silo's row: listed with its silo, retried there, and a retry it refuses is 409, not a 303.
func TestDevJobs_SiloRow(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	path := filepath.Join(dir, "app.db")
	org := db.TableDesc{Name: "Org.Org", ID: "id", Columns: []db.Column{{Name: "id", Type: db.CText}}}
	proj := db.TableDesc{Name: "Project.Project", ID: "id", Columns: []db.Column{{Name: "id", Type: db.CText}, {Name: "org", Type: db.CText, Ref: "Org.Org", OnDelete: db.Cascade}}}
	cfg := db.StoresConfig{Dir: dir, Silos: []db.Silo{{Key: "org", Tables: []string{proj.Name}, Ref: org.Name}}, Tables: []db.TableDesc{org, proj}, Dev: true}
	st, err := db.OpenSQLite(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	if err := st.Ensure(ctx, cfg.Tables, true); err != nil {
		t.Fatal(err)
	}
	s, err := db.OpenStores(st, path, cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	q, _ := db.On(st, nil)
	if _, err := q.ExecContext(ctx, `INSERT INTO "Org__Org" ("id") VALUES ('A')`); err != nil {
		t.Fatal(err)
	}
	pl, err := s.Place(ctx, "org", "A")
	if err != nil {
		t.Fatal(err)
	}
	sq, _ := db.On(pl.Store, nil)
	for _, r := range []struct{ id, state string }{{"deadA001", "dead"}, {"doneA001", "done"}} {
		if _, err := sq.ExecContext(ctx, `INSERT INTO "_alder_jobs" ("id", "kind", "params", "state", "run_at", "created", "attempts", "error") VALUES (?, ?, '{}', ?, ?, ?, 1, 'boom')`,
			r.id, sendKind.Name, r.state, devJobsT0.UnixMilli(), devJobsT0.UnixMilli()); err != nil {
			t.Fatal(err)
		}
	}
	p := &jobs.Panel{Store: st, Stores: s, Kinds: []jobs.Kind{sendKind}, Now: func() time.Time { return devJobsT0 }}
	h := devRouter(t, nil, func(o *Options) { o.Store, o.Stores, o.DevJobs = st, s, p })
	if body := serve(h, httptest.NewRequest("GET", "/_dev/jobs", nil)).Body.String(); !strings.Contains(body, "silo org/A") {
		t.Errorf("/_dev/jobs does not show the row's silo:\n%s", body)
	}
	if w := serve(h, httptest.NewRequest("POST", "/_dev/jobs/doneA001/retry", nil)); w.Code != 409 || !strings.Contains(w.Body.String(), "only a dead job is retried") {
		t.Errorf("retry of a done silo row: %d %s", w.Code, w.Body.String())
	}
	if w := serve(h, httptest.NewRequest("POST", "/_dev/jobs/deadA001/retry", nil)); w.Code != 303 {
		t.Fatalf("retry of a dead silo row: %d %s", w.Code, w.Body.String())
	}
	if r, err := p.Get(ctx, "deadA001"); err != nil || r == nil || r.State != "queued" || r.In.Value != "A" {
		t.Errorf("after retry: %+v %v", r, err)
	}
}

// A job's params are shown with each top-level member whose name says it holds a secret masked,
// on its page and in the JSON, as the request panel masks a form field; the JSON stays valid.
func TestDevJobs_MasksParams(t *testing.T) {
	st := jobsStore(t)
	insertJob(t, st, "secret01", sendKind.Name, `{"user":"u1","password":"hunter2","apiToken":"tok-123"}`, "queued", devJobsT0, 0, time.Time{}, "")
	p := &jobs.Panel{Store: st, Kinds: []jobs.Kind{sendKind}, Now: func() time.Time { return devJobsT0 }}
	h := devRouter(t, nil, func(o *Options) { o.Store, o.DevJobs = st, p })
	page := serve(h, httptest.NewRequest("GET", "/_dev/jobs/secret01", nil)).Body.String()
	raw := serve(h, httptest.NewRequest("GET", "/_dev/jobs.json", nil)).Body.String()
	for name, body := range map[string]string{"page": page, "json": raw} {
		if strings.Contains(body, "hunter2") || strings.Contains(body, "tok-123") || !strings.Contains(body, applog.Mask) || !strings.Contains(body, "u1") {
			t.Errorf("%s: params not masked:\n%s", name, body)
		}
	}
	var v JobsView
	if err := json.Unmarshal([]byte(raw), &v); err != nil || len(v.Queued) != 1 {
		t.Fatalf("%v: %s", err, raw)
	}
	var params map[string]string
	if err := json.Unmarshal([]byte(v.Queued[0].Params), &params); err != nil || params["password"] != applog.Mask || params["user"] != "u1" {
		t.Errorf("params %q: %v", v.Queued[0].Params, err)
	}
}

// A retry refused because its key is held is 409, as a row in another state is.
func TestDevJobs_RetryKeyHeld(t *testing.T) {
	st := jobsStore(t)
	q, _ := db.On(st, nil)
	for _, r := range []struct{ id, state string }{{"deadk001", "dead"}, {"livek001", "queued"}} {
		if _, err := q.ExecContext(context.Background(), `INSERT INTO "_alder_jobs" ("id", "kind", "params", "key", "state", "run_at", "created", "attempts") VALUES (?, ?, '{}', 'k', ?, ?, ?, 1)`,
			r.id, sendKind.Name, r.state, devJobsT0.UnixMilli(), devJobsT0.UnixMilli()); err != nil {
			t.Fatal(err)
		}
	}
	p := &jobs.Panel{Store: st, Kinds: []jobs.Kind{sendKind}, Now: func() time.Time { return devJobsT0 }}
	h := devRouter(t, nil, func(o *Options) { o.Store, o.DevJobs = st, p })
	if w := serve(h, httptest.NewRequest("POST", "/_dev/jobs/deadk001/retry", nil)); w.Code != 409 || !strings.Contains(w.Body.String(), "holds its key") {
		t.Errorf("retry with the key held: %d %s", w.Code, w.Body.String())
	}
}

// The Run now note of a schedule tick says the next tick follows the later of its slot and the
// claim (ClaimedTick), not its slot alone.
func TestDevJobs_RunNowTickNote(t *testing.T) {
	_, _, h := jobsFixture(t)
	var v JobsView
	if err := json.Unmarshal(serve(h, httptest.NewRequest("GET", "/_dev/jobs.json", nil)).Body.Bytes(), &v); err != nil {
		t.Fatal(err)
	}
	body := serve(h, httptest.NewRequest("GET", "/_dev/jobs/"+v.Schedules[0].ID, nil)).Body.String()
	if !strings.Contains(body, "the first after the later of 2026-11-16 13:00:00 UTC and the time it is claimed") {
		t.Errorf("tick note:\n%s", body)
	}
}
