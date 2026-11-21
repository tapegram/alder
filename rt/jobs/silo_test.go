package jobs_test

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"alder/rt"
	"alder/rt/db"
	"alder/rt/eff"
	"alder/rt/jobs"
)

// Silos (spec §7): a row lands in the store of the Task that enqueues it; a `silo k` job runs
// bound to the silo its parameter names; when the row's store is not the run's, the done mark is
// a second transaction that never resurrects a row a lease expiry re-queued.

type project struct{ ID, Name, Org string }

var (
	orgDesc  = db.TableDesc{Name: "Org.Org", ID: "id", Columns: []db.Column{{Name: "id", Type: db.CText}, {Name: "name", Type: db.CText}}}
	projDesc = db.TableDesc{Name: "Project.Project", ID: "id", Columns: []db.Column{{Name: "id", Type: db.CText}, {Name: "name", Type: db.CText}, {Name: "org", Type: db.CText, Ref: "Org.Org", OnDelete: db.Cascade}}}
	projT    = &db.Table[project]{Name: projDesc.Name, Columns: projDesc.Columns, ID: "id",
		Encode: func(p project) []any { return []any{p.ID, p.Name, p.Org} },
		Decode: func(v []any) (p project, err error) {
			p.ID, _ = v[0].(string)
			p.Name, _ = v[1].(string)
			p.Org, _ = v[2].(string)
			return p, nil
		},
		ReadID: func(p project) string { return p.ID }, SetID: func(p project, id string) project { p.ID = id; return p },
		Owner: "org", Key: "org", Silo: "org"}
	siloCfg = db.StoresConfig{Silos: []db.Silo{{Key: "org", Tables: []string{projDesc.Name}, Ref: orgDesc.Name}}, Tables: []db.TableDesc{orgDesc, projDesc, noteTable}, Dev: true}
)

// siloApp is an app with `silo org { Project }` on files under a temporary directory: the
// shared store and its Stores, with orgs A and B.
func siloApp(t *testing.T) (dir string, shared *db.SQLite, s *db.Stores) {
	t.Helper()
	ctx := context.Background()
	dir = t.TempDir()
	path := filepath.Join(dir, "app.db")
	shared, err := db.OpenSQLite(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { shared.Close() })
	if err := shared.Ensure(ctx, siloCfg.Tables, true); err != nil {
		t.Fatal(err)
	}
	cfg := siloCfg
	cfg.Dir = dir
	if s, err = db.OpenStores(shared, path, cfg); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	insertOrgs(t, shared, "A", "B")
	return dir, shared, s
}

// insertOrgs inserts orgs of these ids in st (Insert would draw ids of its own).
func insertOrgs(t *testing.T, st *db.SQLite, ids ...string) {
	t.Helper()
	for _, id := range ids {
		if _, err := raw(t, st).ExecContext(context.Background(), `INSERT INTO "Org__Org" ("id", "name") VALUES (?, ?)`, id, "org "+id); err != nil {
			t.Fatal(err)
		}
	}
}

// sharedEnv is an Env on the shared store of s, as App.JobEnv and a request's start build it.
func sharedEnv(shared *db.SQLite, s *db.Stores) *eff.Env {
	return &eff.Env{Ctx: context.Background(), Store: shared, Stores: s, Now: func() time.Time { return t0 }}
}

// orgEnv is sharedEnv narrowed to org and bound to its silo, as a `requires org` route's update.
func orgEnv(t *testing.T, shared *db.SQLite, s *db.Stores, org string) *eff.Env {
	t.Helper()
	env := sharedEnv(shared, s)
	db.Narrow(env, "org", org)
	if err := db.Enter(env, "org"); err != nil {
		t.Fatal(err)
	}
	return env
}

// siloFile opens the file of org's silo under dir, outside Stores.
func siloFile(t *testing.T, dir, org string) *db.SQLite {
	t.Helper()
	p, err := db.SiloPath(dir, "org", org)
	if err != nil {
		t.Fatal(err)
	}
	f, err := db.OpenSQLite(p)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { f.Close() })
	return f
}

// digest is `job Digest = { org: Org.OrgId } silo org` as the generator makes it: Run decodes
// the parameters and narrows the Env to their org; its run inserts a project there.
var digest = jobs.Kind{
	Name: "Reports.Digest", Retries: 4, Timeout: 5 * time.Minute, SiloParam: "org",
	Run: func(e *eff.Env, params string) (eff.Task[struct{}], error) {
		var p struct{ Org string }
		if err := json.Unmarshal([]byte(params), &p); err != nil {
			return eff.Task[struct{}]{}, err
		}
		db.Narrow(e, "org", p.Org)
		return eff.Call("Db.insert Project", true, func(e *eff.Env) (struct{}, error) {
			_, err := db.InsertRow(e.Ctx, e.Store.(db.Store), db.TxOf(e), projT, []any{nil, "digest", p.Org}, e.Now())
			return struct{}{}, err
		}), nil
	},
}

func digestOf(org string) rt.Job { return rt.Job{Kind: digest.Name, Params: `{"org":"` + org + `"}`} }

// placeOf is where a Task on env writes: its store, and its silo's key and value when it is bound
// to one (db.Enter), as the worker's claim records a row's Place.
func placeOf(env *eff.Env) db.Place {
	return db.Place{Store: env.Store, Key: env.SiloKey, Value: env.SiloValue}
}

// claimOn is the worker's claim of id's row at at (the oldest due: the only one).
func claimOn(t *testing.T, at db.Place, kinds []jobs.Kind, id string) jobs.Row {
	t.Helper()
	row, err := jobs.Claim(context.Background(), at.Store, kinds, t0, "test:1:1")
	if err != nil || row == nil || row.ID != id {
		t.Fatalf("claim: %+v %v (want %s)", row, err, id)
	}
	row.In = at
	return *row
}

// A `requires org` route's enqueue lands in that org's silo, not in the shared store: on files
// (production) and in a test's stores (db.NewTestStores), where the Task's Store stays the
// TestStore whatever it is bound to.
func TestEnqueue_LandsInSilo(t *testing.T) {
	t.Run("file", func(t *testing.T) {
		dir, shared, s := siloApp(t)
		id := runEnqueue(t, orgEnv(t, shared, s, "A"), welcome, nil)
		if n := count(t, siloFile(t, dir, "A"), `SELECT COUNT(*) FROM "_alder_jobs" WHERE "id" = ?`, id); n != 1 {
			t.Fatalf("silo A holds %d rows of %s", n, id)
		}
		if n := count(t, shared, `SELECT COUNT(*) FROM "_alder_jobs"`); n != 0 {
			t.Fatalf("the shared store holds %d rows", n)
		}
	})
	t.Run("test stores", func(t *testing.T) {
		shared := open(t)
		if err := shared.Ensure(context.Background(), siloCfg.Tables, true); err != nil {
			t.Fatal(err)
		}
		s, err := db.NewTestStores(shared, siloCfg)
		if err != nil {
			t.Fatal(err)
		}
		defer s.Close()
		insertOrgs(t, shared, "A")
		env := &eff.Env{Ctx: context.Background(), Store: db.TestStore{SQLite: shared, Stores: s}, Stores: s, Now: func() time.Time { return t0 }}
		db.Narrow(env, "org", "A")
		if err := db.Enter(env, "org"); err != nil {
			t.Fatal(err)
		}
		id := runEnqueue(t, env, welcome, nil)
		silos, err := s.OpenSilos(context.Background()) // the test silo, through its transaction
		if err != nil || len(silos) != 1 || silos[0].Value != "A" {
			t.Fatalf("open silos %+v %v", silos, err)
		}
		q, err := db.On(silos[0].Store, silos[0].Tx)
		if err != nil {
			t.Fatal(err)
		}
		var n int
		if err := q.QueryRowContext(context.Background(), `SELECT COUNT(*) FROM "_alder_jobs" WHERE "id" = ?`, id).Scan(&n); err != nil || n != 1 {
			t.Fatalf("test silo A holds %d rows of %s (%v)", n, id, err)
		}
		if n := count(t, shared, `SELECT COUNT(*) FROM "_alder_jobs"`); n != 0 {
			t.Fatalf("the shared store holds %d rows", n)
		}
	})
}

// `job Digest = { org: Org.OrgId } silo org` enqueued on the shared store runs bound to org A's
// silo: its insert lands there, and the shared row is done (a second transaction).
func TestRun_SiloJob(t *testing.T) {
	dir, shared, s := siloApp(t)
	id := runEnqueue(t, sharedEnv(shared, s), digestOf("A"), nil)
	row := claimOn(t, db.Place{Store: shared}, []jobs.Kind{digest}, id)
	out := jobs.RunOne(sharedEnv(shared, s), digest, row)
	if out.Err != nil || out.Dead {
		t.Fatalf("outcome %+v", out)
	}
	if r := read(t, shared, id); r.state != "done" || r.finished == nil || r.leaseUntil != nil {
		t.Fatalf("shared row %+v", r)
	}
	a := siloFile(t, dir, "A")
	if n := count(t, a, `SELECT COUNT(*) FROM "Project__Project" WHERE "org" = 'A'`); n != 1 {
		t.Fatalf("silo A holds %d projects", n)
	}
	if n := count(t, a, `SELECT COUNT(*) FROM "_alder_jobs"`); n != 0 {
		t.Fatalf("silo A holds %d job rows", n)
	}
	if n := count(t, shared, `SELECT COUNT(*) FROM "Project__Project"`); n != 0 {
		t.Fatalf("the shared store holds %d projects", n)
	}

	// a silo job enqueued from its own silo: one store, the mark in the job's transaction
	env := orgEnv(t, shared, s, "B")
	id = runEnqueue(t, env, digestOf("B"), nil)
	row = claimOn(t, placeOf(env), []jobs.Kind{digest}, id)
	stats := &eff.Stats{}
	je := sharedEnv(shared, s)
	je.Stats = stats
	if out := jobs.RunOne(je, digest, row); out.Err != nil {
		t.Fatalf("outcome %+v", out)
	}
	if stats.Exec != 2 {
		t.Fatalf("statements %+v: want the insert and the mark, in one transaction", stats)
	}
	if n := count(t, siloFile(t, dir, "B"), `SELECT COUNT(*) FROM "_alder_jobs" WHERE "id" = ? AND "state" = 'done'`, id); n != 1 {
		t.Fatal("silo B's row is not done")
	}

	// a failure is recorded in the row's store (the shared one), not the run's
	failing := digest
	failing.Run = func(e *eff.Env, params string) (eff.Task[struct{}], error) {
		db.Narrow(e, "org", "A")
		return eff.Fail[struct{}]("x"), nil
	}
	id = runEnqueue(t, sharedEnv(shared, s), digestOf("A"), nil)
	out = jobs.RunOne(sharedEnv(shared, s), failing, claimOn(t, db.Place{Store: shared}, []jobs.Kind{failing}, id))
	if r := read(t, shared, id); out.Err == nil || r.state != "queued" || r.err == nil || *r.err != "x" {
		t.Fatalf("outcome %+v, row %+v", out, r)
	}
}

// Between the run's commit and the done mark in the row's store, a lease expiry re-queues the
// row: the mark (WHERE state = 'running') changes nothing, and the re-queued row stays.
func TestDoneMark_NoResurrect(t *testing.T) {
	_, shared, s := siloApp(t)
	env := orgEnv(t, shared, s, "A")
	silo := env.Store
	q := raw(t, silo.(*db.SQLite))

	// a job of the shared store whose row is in silo A: done by a second transaction
	plain := kind(insertNote("n"))
	id := runEnqueue(t, env, welcome, nil)
	if out := jobs.RunOne(sharedEnv(shared, s), plain, claimOn(t, placeOf(env), []jobs.Kind{plain}, id)); out.Err != nil {
		t.Fatalf("outcome %+v", out)
	}
	if r := read(t, silo.(*db.SQLite), id); r.state != "done" {
		t.Fatalf("row %+v: not marked done", r)
	}
	if n := notes(t, shared); n != 1 {
		t.Fatalf("notes %d", n)
	}

	// the same, with the lease re-queue after the commit and before the mark
	id = runEnqueue(t, env, welcome, nil)
	requeue := eff.Call("requeue", true, func(e *eff.Env) (struct{}, error) {
		e.AfterCommit(func() {
			if _, err := q.ExecContext(context.Background(), `UPDATE "_alder_jobs" SET "state" = 'queued', "error" = ?, "lease_until" = NULL WHERE "id" = ?`, jobs.LostError, id); err != nil {
				t.Error(err)
			}
		})
		return struct{}{}, nil
	})
	lost := kind(then(insertNote("again"), requeue))
	if out := jobs.RunOne(sharedEnv(shared, s), lost, claimOn(t, placeOf(env), []jobs.Kind{lost}, id)); out.Err != nil {
		t.Fatalf("outcome %+v", out)
	}
	r := read(t, silo.(*db.SQLite), id)
	if r.state != "queued" || r.finished != nil || r.err == nil || *r.err != jobs.LostError {
		t.Fatalf("row %+v: the done mark resurrected it", r)
	}
	if n := notes(t, shared); n != 2 {
		t.Fatalf("notes %d: the job's own commit is kept", n)
	}

	// the same, with another worker's claim of the row (its lease ran out, it was re-queued and
	// claimed again) after the commit and before the mark: the mark leaves that claim alone
	setState(t, silo.(*db.SQLite), id, "dead") // the re-queued row above, out of the way
	id = runEnqueue(t, env, welcome, nil)
	reclaim := eff.Call("reclaim", true, func(e *eff.Env) (struct{}, error) {
		e.AfterCommit(func() {
			if _, err := q.ExecContext(context.Background(), `UPDATE "_alder_jobs" SET "worker" = 'other:2:1', "attempts" = "attempts" + 1 WHERE "id" = ?`, id); err != nil {
				t.Error(err)
			}
		})
		return struct{}{}, nil
	})
	stolen := kind(reclaim)
	if out := jobs.RunOne(sharedEnv(shared, s), stolen, claimOn(t, placeOf(env), []jobs.Kind{stolen}, id)); out.Err != nil {
		t.Fatalf("outcome %+v", out)
	}
	if r := read(t, silo.(*db.SQLite), id); r.state != "running" || r.finished != nil || r.attempts != 2 {
		t.Fatalf("row %+v: the done mark took another worker's claim", r)
	}
}

// The idle sweep may close a silo between the claim of one of its rows and the run, and the
// next use opens it again (a new store): the row's silo is still the run's (one transaction, the
// mark in it), and a mark or record made apart goes to the silo as it is open now.
func TestRun_SiloReopened(t *testing.T) {
	dir, shared, s1 := siloApp(t)
	cfg := siloCfg
	cfg.Dir = dir
	s2, err := db.OpenStores(shared, filepath.Join(dir, "app.db"), cfg) // the silos opened again
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s2.Close() })
	silos := func() db.Place {
		ps, err := s1.OpenSilos(context.Background())
		if err != nil || len(ps) != 1 || ps[0].Value != "A" {
			t.Fatalf("open silos %+v %v", ps, err)
		}
		return ps[0]
	}
	env2 := func() *eff.Env { return sharedEnv(shared, s2) }

	// a silo job of A whose row is in A: one database, the mark in the job's transaction
	id := runEnqueue(t, orgEnv(t, shared, s1, "A"), digestOf("A"), nil)
	at := silos()
	row, err := jobs.Claim(context.Background(), at.Store, []jobs.Kind{digest}, t0, "test:1:1")
	if err != nil || row == nil {
		t.Fatal(row, err)
	}
	row.In = at
	// a job of the shared store whose row is in A: marked apart
	plain := kind(insertNote("n"))
	id2 := runEnqueue(t, orgEnv(t, shared, s1, "A"), welcome, nil)
	row2, err := jobs.Claim(context.Background(), at.Store, []jobs.Kind{plain}, t0, "test:1:2")
	if err != nil || row2 == nil {
		t.Fatal(row2, err)
	}
	row2.In = at
	s1.Close() // the claimed-from store is closed

	stats := &eff.Stats{}
	e := env2()
	e.Stats = stats
	if out := jobs.RunOne(e, digest, *row); out.Err != nil {
		t.Fatalf("outcome %+v", out)
	}
	if stats.Exec != 2 {
		t.Fatalf("statements %+v: want the insert and the mark, in one transaction", stats)
	}
	if out := jobs.RunOne(env2(), plain, *row2); out.Err != nil {
		t.Fatalf("outcome %+v", out)
	}
	a := siloFile(t, dir, "A")
	for _, id := range []string{id, id2} {
		if r := read(t, a, id); r.state != "done" {
			t.Fatalf("row %s %+v: not marked done", id, r)
		}
	}

	// a failure apart is recorded in the silo as it is open now
	failing := kind(eff.Fail[struct{}]("x"))
	id3 := runEnqueue(t, orgEnv(t, shared, s2, "A"), welcome, nil)
	ps, _ := s2.OpenSilos(context.Background())
	row3, err := jobs.Claim(context.Background(), ps[0].Store, []jobs.Kind{failing}, t0, "test:1:3")
	if err != nil || row3 == nil {
		t.Fatal(row3, err)
	}
	row3.In = db.Place{Store: siloFile(t, dir, "B"), Key: "org", Value: "A"} // a stale store: not used
	if out := jobs.RunOne(env2(), failing, *row3); out.Err == nil {
		t.Fatal("no failure")
	}
	if r := read(t, a, id3); r.state != "queued" || r.err == nil || *r.err != "x" {
		t.Fatalf("row %+v", r)
	}
}

// Job.status reads the store the effect is bound to, then the shared store when that is a silo
// (spec §2); a page narrowed to an org (not bound: a load) finds the org's rows first. A row in
// another org's silo is Nothing.
func TestStatusOf_SiloFallback(t *testing.T) {
	_, shared, s := siloApp(t)
	inShared := runEnqueue(t, sharedEnv(shared, s), welcome, nil)
	inA := runEnqueue(t, orgEnv(t, shared, s, "A"), welcome, nil)
	inB := runEnqueue(t, orgEnv(t, shared, s, "B"), welcome, nil)
	status := func(env *eff.Env, id string) *jobs.Status {
		t.Helper()
		st, err := eff.RunQuery(env, eff.Q(eff.Call("Job.status", false, func(e *eff.Env) (*jobs.Status, error) { return jobs.StatusOf(e, id) })))
		if err != nil {
			t.Fatal(err)
		}
		return st
	}
	narrowed := func() *eff.Env {
		env := sharedEnv(shared, s)
		db.Narrow(env, "org", "A")
		return env
	}
	for name, c := range map[string]struct {
		env   *eff.Env
		id    string
		found bool
	}{
		"bound to A: its own row":         {orgEnv(t, shared, s, "A"), inA, true},
		"bound to A: the shared row":      {orgEnv(t, shared, s, "A"), inShared, true},
		"bound to A: B's row":             {orgEnv(t, shared, s, "A"), inB, false},
		"narrowed to A: its row":          {narrowed(), inA, true},
		"narrowed to A: the shared row":   {narrowed(), inShared, true},
		"shared: a silo's row is Nothing": {sharedEnv(shared, s), inA, false},
	} {
		if st := status(c.env, c.id); (st != nil) != c.found || (st != nil && st.State != jobs.Queued) {
			t.Errorf("%s: %+v", name, st)
		}
	}
}

// The worker claims from the shared store and from the silos Stores has open: a row enqueued in
// silo A runs, and its done mark lands there.
func TestWorker_SiloRows(t *testing.T) {
	_, shared, s := siloApp(t)
	env := orgEnv(t, shared, s, "A")
	silo := env.Store.(*db.SQLite)
	id := runEnqueue(t, env, welcome, nil)
	outs := make(chan jobs.Outcome, 4)
	stop := jobs.Start(context.Background(), jobs.Options{
		Store: shared, Stores: s, Kinds: []jobs.Kind{kind(insertNote("from a silo row"))}, Workers: 1,
		Env:       func(ctx context.Context, k jobs.Kind) *eff.Env { return &eff.Env{Ctx: ctx, Store: shared, Stores: s} },
		Now:       func() time.Time { return t0 },
		OnAttempt: func(o jobs.Outcome) { outs <- o },
	})
	defer stop(0)
	select {
	case o := <-outs:
		if o.ID != id || o.Err != nil {
			t.Fatalf("outcome %+v", o)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the silo's row did not run")
	}
	stop(time.Second)
	if r := read(t, silo, id); r.state != "done" {
		t.Fatalf("row %+v", r)
	}
	if n := notes(t, shared); n != 1 {
		t.Fatalf("notes %d", n)
	}
}

// Housekeeping keeps the open silos' tables too: a running row whose lease ran out is re-queued
// there; ticks are made in the shared store only.
func TestHousekeep_SiloLeases(t *testing.T) {
	_, shared, s := siloApp(t)
	env := orgEnv(t, shared, s, "A")
	silo := env.Store.(*db.SQLite)
	id := runEnqueue(t, env, welcome, nil)
	k := kind(eff.Pure(struct{}{}))
	claimOn(t, placeOf(env), []jobs.Kind{k}, id)
	hourly := jobs.Kind{Name: "Reports.Hourly", Every: time.Hour, Retries: 4, Timeout: time.Minute, Run: k.Run}
	var warns []string
	keep := jobs.SiloHousekeeper(shared, s, []jobs.Kind{k, hourly}, func() time.Time { return t0 }, func(level, msg string) { warns = append(warns, level+" "+msg) })
	keep(context.Background(), t0.Add(jobs.Lease(k)+time.Second))
	if r := read(t, silo, id); r.state != "queued" || r.err == nil || *r.err != jobs.LostError {
		t.Fatalf("silo row %+v, log %q", r, warns)
	}
	if n := count(t, silo, `SELECT COUNT(*) FROM "_alder_jobs" WHERE "scheduled_for" IS NOT NULL`); n != 0 {
		t.Fatalf("silo A holds %d ticks", n)
	}
	if n := count(t, shared, `SELECT COUNT(*) FROM "_alder_jobs" WHERE "kind" = 'Reports.Hourly' AND "schedule" = 1`); n != 1 {
		t.Fatalf("the shared store holds %d ticks", n)
	}
}
