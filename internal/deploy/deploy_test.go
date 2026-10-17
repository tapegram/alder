package deploy

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"alder/internal/ir"
	"alder/internal/migrate"
	"alder/rt/db"
)

// fakeTarget records the calls the engine makes, in order, and answers from its fields.
type fakeTarget struct {
	calls      []string
	missing    []string
	live       []byte
	builds     []Build
	current    string
	schemas    map[string][]byte
	startErr   error
	health     error
	records    map[string]Record
	applied    []string            // the live database's migrations
	migrations map[string][]string // each build's
}

func (f *fakeTarget) call(s string)    { f.calls = append(f.calls, s) }
func (f *fakeTarget) Describe() string { return "Fake box:/srv/demo" }
func (f *fakeTarget) Connect(context.Context) (Platform, error) {
	f.call("connect")
	return Platform{"linux", "amd64"}, nil
}
func (f *fakeTarget) Requires(_ context.Context, names []string) ([]string, error) {
	f.call("requires " + strings.Join(names, ","))
	return f.missing, nil
}
func (f *fakeTarget) Schema(context.Context) (Schema, error) {
	f.call("schema")
	return Schema{Have: f.live != nil, Snapshot: f.live, Migrations: f.applied}, nil
}
func (f *fakeTarget) BuildSchema(_ context.Context, id string) (Schema, error) {
	b, ok := f.schemas[id]
	return Schema{Have: ok, Snapshot: b, Migrations: f.migrations[id]}, nil
}
func (f *fakeTarget) Current(context.Context) (string, error)      { return f.current, nil }
func (f *fakeTarget) Builds(context.Context) ([]Build, error)      { return f.builds, nil }
func (f *fakeTarget) Upload(_ context.Context, _, id string) error { f.call("upload"); return nil }
func (f *fakeTarget) Backup(_ context.Context, id string) (string, error) {
	f.call("backup")
	if f.live == nil {
		return "", nil
	}
	return "builds/" + id + "/backup.db", nil
}
func (f *fakeTarget) Migrate(_ context.Context, id string) (string, error) {
	f.call("migrate")
	return "migrated 0002_drop_summary\n", nil
}
func (f *fakeTarget) Start(_ context.Context, id string) (int, error) {
	f.call("start " + id)
	return 4100, f.startErr
}
func (f *fakeTarget) Health(_ context.Context, id string, port int, _ time.Duration) error {
	f.call("health")
	return f.health
}
func (f *fakeTarget) Switch(_ context.Context, id string, port int) (int, error) {
	f.call("switch " + id)
	return port, nil
}
func (f *fakeTarget) Stop(_ context.Context, id string, d time.Duration) error {
	f.call(fmt.Sprintf("stop %s %s", id, d))
	return nil
}
func (f *fakeTarget) Record(_ context.Context, id string, rec []byte) error {
	f.call("record")
	var r Record
	if err := json.Unmarshal(rec, &r); err != nil {
		return err
	}
	if f.records == nil {
		f.records = map[string]Record{}
	}
	f.records[id] = r
	return nil
}
func (f *fakeTarget) Prune(context.Context, int, []string) ([]string, error) {
	f.call("prune")
	return nil, nil
}
func (f *fakeTarget) Info(context.Context) (Info, error)          { return Info{}, nil }
func (f *fakeTarget) Logs(context.Context, bool, io.Writer) error { return nil }
func (f *fakeTarget) Restore(context.Context, string) error       { return nil }
func (f *fakeTarget) Close() error                                { return nil }
func (f *fakeTarget) shipped() bool {
	for _, c := range f.calls {
		if c == "upload" {
			return true
		}
	}
	return false
}

// passRunner is a gate that passes with a score of 0.9.
type passRunner struct{}

func (passRunner) Check() (int, int, int, string)            { return 0, 0, 0, "" }
func (passRunner) Build() (string, bool)                     { return "", true }
func (passRunner) Test() (TestResult, string, bool)          { return TestResult{Passed: 3}, "", true }
func (passRunner) Mutate() (float64, int, int, string, bool) { return 0.9, 10, 1, "", true }

// schemas are the snapshots of a Post table with a summary (s1) and without it (s2).
func schemas() (s1, s2 migrate.Schema) {
	cols := []migrate.Column{{Name: "id", Type: db.CText}, {Name: "title", Type: db.CText}}
	s2 = migrate.Schema{Format: db.Format, Tables: []migrate.Table{{Name: "Post.Post", ID: "id", Columns: cols}}}
	s1 = migrate.Schema{Format: db.Format, Tables: []migrate.Table{{Name: "Post.Post", ID: "id",
		Columns: append(append([]migrate.Column{}, cols...), migrate.Column{Name: "summary", Type: db.CText, Nullable: true})}}}
	return s1, s2
}

// appWithDrop is an app directory whose migrations create Post with a summary and then drop
// it; it returns the directory and the live snapshot after the first migration.
func appWithDrop(t *testing.T) (string, []byte) {
	t.Helper()
	dir := t.TempDir()
	mig := filepath.Join(dir, "migrations")
	os.MkdirAll(mig, 0o755)
	os.MkdirAll(filepath.Join(dir, "build", "static"), 0o755)
	s1, s2 := schemas()
	m1, err := migrate.Plan(migrate.Schema{Format: db.Format}, s1, "0001_initial")
	if err != nil {
		t.Fatal(err)
	}
	m2, err := migrate.Plan(s1, s2, "0002_drop_summary")
	if err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(mig, "0001_initial.sql"), []byte(migrate.Render(m1)), 0o644)
	os.WriteFile(filepath.Join(mig, "0002_drop_summary.sql"), []byte(migrate.Render(m2)), 0o644)
	if err := migrate.WriteSnapshot(filepath.Join(mig, "schema.json"), s2); err != nil {
		t.Fatal(err)
	}
	live, _ := json.Marshal(s1)
	return dir, live
}

func options(t *testing.T, dir string, out io.Writer) Options {
	return Options{
		App: "demo", Env: "production", AppDir: dir, Built: filepath.Join(dir, "build"), ArtifactRoot: filepath.Join(dir, "build", "deploy"),
		Decl:   &ir.Deploy{Requires: []string{"SMTP_URL"}, Backup: &ir.Backup{Kind: "s3", URL: "s3://b/demo"}},
		Runner: passRunner{},
		Binary: func(p Platform, path string) error {
			return os.WriteFile(path, []byte("binary for "+p.String()), 0o755)
		},
		Out: out, Drain: -1, User: "ada",
	}
}

// RF1: a migration that drops data is refused without --allow-data-loss, with the DROPS DATA
// line, before anything is uploaded; with the flag the backup comes before the migration, and
// the record says so.
func TestDeploy_DataLoss(t *testing.T) {
	dir, live := appWithDrop(t)
	var out bytes.Buffer
	f := &fakeTarget{live: live, current: "1111111111111111"}
	_, err := Deploy(context.Background(), options(t, dir, &out), GateResult{Check: "ok"}, f, nil)
	var r *Refusal
	if !errors.As(err, &r) || !strings.Contains(err.Error(), "refused: the migration drops data\n  DROPS DATA: 0002_drop_summary: column Post.summary\nrun again with --allow-data-loss") {
		t.Fatalf("err = %v\n%s", err, out.String())
	}
	if f.shipped() {
		t.Errorf("uploaded before refusing: %v", f.calls)
	}
	if !strings.Contains(out.String(), "  0002_drop_summary\n    drop column Post.summary\n    DROPS DATA: column Post.summary\n") {
		t.Errorf("plan:\n%s", out.String())
	}

	f = &fakeTarget{live: live, current: "1111111111111111"}
	o := options(t, dir, &out)
	o.AllowDataLoss = true
	res, err := Deploy(context.Background(), o, GateResult{Check: "ok"}, f, nil)
	if err != nil {
		t.Fatal(err)
	}
	id := res.Record.Build
	want := []string{"connect", "requires SMTP_URL,ALDER_SECRET", "schema", "upload", "backup", "migrate", "start " + id, "health", "switch " + id, "record", "stop 1111111111111111 0s", "prune"}
	if strings.Join(f.calls, "|") != strings.Join(want, "|") {
		t.Errorf("calls\n%v\nwant\n%v", f.calls, want)
	}
	rec := f.records[id]
	if !rec.AllowDataLoss || rec.Backup != "builds/"+id+"/backup.db" || rec.Previous != "1111111111111111" || rec.By != "local ada" ||
		strings.Join(rec.Applied, ",") != "0002_drop_summary" || rec.When == nil || rec.BackupURL != "s3://b/demo" {
		t.Errorf("deployed.json: %+v", rec)
	}
	desc, _ := os.ReadFile(filepath.Join(res.Dir, ArtifactFile))
	if !bytes.Contains(desc, []byte(`"dropsData": [`)) || !bytes.Contains(desc, []byte(`"allowDataLoss": true`)) {
		t.Errorf("alder-deploy.json:\n%s", desc)
	}
}

// A first deploy has nothing to lose: no refusal, every migration pending.
func TestDeploy_FirstDeployDropsNothing(t *testing.T) {
	dir, _ := appWithDrop(t)
	var out bytes.Buffer
	f := &fakeTarget{}
	if _, err := Deploy(context.Background(), options(t, dir, &out), GateResult{Check: "ok"}, f, nil); err != nil {
		t.Fatalf("%v\n%s", err, out.String())
	}
	if !strings.Contains(out.String(), "first deploy: the database is created by every migration\n  0001_initial\n") || strings.Contains(out.String(), "DROPS DATA") {
		t.Errorf("plan:\n%s", out.String())
	}
	if !strings.Contains(out.String(), "  backup   ok  no database yet\n") {
		t.Errorf("output:\n%s", out.String())
	}
}

// RF2: a build that does not pass its health check is stopped; nothing is switched or
// recorded, the previous build is not stopped, and the refusal carries the build's log.
func TestDeploy_HealthFails(t *testing.T) {
	for _, c := range []struct {
		name          string
		start, health error
	}{
		{"exits at start", errors.New("build exited at start:\nboom: the mail server is unreachable"), nil},
		{"never answers", nil, errors.New("no answer with build id after 60s; its log:\nlistening on 127.0.0.1:4100")},
	} {
		t.Run(c.name, func(t *testing.T) {
			dir, live := appWithDrop(t)
			var out bytes.Buffer
			f := &fakeTarget{live: live, current: "1111111111111111", startErr: c.start, health: c.health}
			o := options(t, dir, &out)
			o.AllowDataLoss = true
			res, err := Deploy(context.Background(), o, GateResult{Check: "ok"}, f, nil)
			if err == nil {
				t.Fatal("deployed")
			}
			msg := err.Error()
			id := res.Record.Build
			if !strings.Contains(msg, "refused: build "+id+" did not pass its health check: it was stopped and builds/"+id+"/ is kept for inspection; nothing was switched; build 1111111111111111 still serves") {
				t.Errorf("refusal: %s", msg)
			}
			if c.start != nil && !strings.Contains(msg, "boom: the mail server is unreachable") || c.health != nil && !strings.Contains(msg, "listening on") {
				t.Errorf("the refusal lacks the log: %s", msg)
			}
			for _, call := range f.calls {
				if strings.HasPrefix(call, "switch") || call == "record" || strings.HasPrefix(call, "stop 1111") {
					t.Errorf("%s after a failed health check: %v", call, f.calls)
				}
			}
			if c.health != nil && f.calls[len(f.calls)-1] != "stop "+id+" 0s" {
				t.Errorf("the new build was not stopped: %v", f.calls)
			}
		})
	}
}

// RF4: a `requires` name the target does not set is refused, naming it, before any upload.
func TestDeploy_RequiresMissing(t *testing.T) {
	dir, live := appWithDrop(t)
	var out bytes.Buffer
	f := &fakeTarget{live: live, missing: []string{"SMTP_URL"}}
	_, err := Deploy(context.Background(), options(t, dir, &out), GateResult{Check: "ok"}, f, nil)
	if err == nil || !strings.Contains(err.Error(), "refused: the environment at Fake box:/srv/demo does not set SMTP_URL: add it to its environment file") {
		t.Fatalf("err = %v", err)
	}
	if f.shipped() {
		t.Errorf("uploaded: %v", f.calls)
	}
}

// --dry-run fetches the live schema and prints the plan, and changes nothing at the target.
func TestDeploy_DryRunTouchesNothing(t *testing.T) {
	dir, live := appWithDrop(t)
	var out bytes.Buffer
	f := &fakeTarget{live: live}
	o := options(t, dir, &out)
	o.DryRun, o.AllowDataLoss = true, true
	if _, err := Deploy(context.Background(), o, GateResult{Check: "ok"}, f, nil); err != nil {
		t.Fatal(err)
	}
	if strings.Join(f.calls, "|") != "connect|requires SMTP_URL,ALDER_SECRET|schema" {
		t.Errorf("calls: %v", f.calls)
	}
}

// The floor defaults to the score the current build was deployed with.
func TestDeploy_FloorFromLastDeploy(t *testing.T) {
	dir, live := appWithDrop(t)
	var out bytes.Buffer
	f := &fakeTarget{live: live, current: "1111111111111111", builds: []Build{{ID: "1111111111111111", Record: &Record{Gate: GateResult{Mutate: &MutateResult{Score: 0.95}}}}}}
	_, err := Deploy(context.Background(), options(t, dir, &out), GateResult{Check: "ok"}, f, nil)
	if err == nil || !strings.Contains(err.Error(), "refused: the mutation score 0.90 is below the floor 0.95 (last deploy): 1 mutant survived") {
		t.Fatalf("err = %v", err)
	}
}

// RF3: rolling back to a build whose snapshot is older than the live schema is refused with
// the changed tables; --allow-schema-mismatch proceeds (the data is not touched either way).
func TestRollback_SchemaMismatch(t *testing.T) {
	s1, s2 := schemas()
	j1, _ := json.Marshal(s1)
	j2, _ := json.Marshal(s2)
	now := time.Now()
	mk := func() *fakeTarget {
		return &fakeTarget{live: j2, current: "2222222222222222",
			builds: []Build{
				{ID: "2222222222222222", Record: &Record{Previous: "1111111111111111", When: &now}},
				{ID: "1111111111111111", Record: &Record{When: &now}},
			},
			schemas: map[string][]byte{"1111111111111111": j1, "2222222222222222": j2}}
	}
	var out bytes.Buffer
	f := mk()
	_, err := Rollback(context.Background(), f, RollbackOptions{Out: &out, Drain: -1})
	if err == nil || !strings.Contains(err.Error(), "refused: the live database's schema "+SchemaHash(j2)+" is not build 1111111111111111's "+SchemaHash(j1)+" (changed tables: Post.Post)") {
		t.Fatalf("err = %v", err)
	}
	for _, c := range f.calls {
		if strings.HasPrefix(c, "start") || strings.HasPrefix(c, "switch") {
			t.Errorf("%s after the refusal", c)
		}
	}
	f = mk()
	to, err := Rollback(context.Background(), f, RollbackOptions{Out: &out, Drain: -1, AllowSchemaMismatch: true})
	if err != nil || to != "1111111111111111" {
		t.Fatalf("to %s err %v", to, err)
	}
	want := "connect|schema|start 1111111111111111|health|switch 1111111111111111|stop 2222222222222222 0s"
	if strings.Join(f.calls, "|") != want {
		t.Errorf("calls %v", f.calls)
	}
	for _, c := range f.calls {
		if c == "migrate" || c == "backup" {
			t.Errorf("a rollback touched the data: %v", f.calls)
		}
	}
	// the same schema: no refusal
	f = mk()
	f.schemas["1111111111111111"] = j2
	if _, err := Rollback(context.Background(), f, RollbackOptions{Out: &out, Drain: -1}); err != nil {
		t.Errorf("same schema: %v", err)
	}
	// the same schema reached by migrations the build does not have (a column added, then
	// dropped): the runtime would refuse it, so the rollback does
	f = mk()
	f.schemas["1111111111111111"] = j2
	f.applied = []string{"0001_initial", "0002_add", "0003_drop"}
	f.migrations = map[string][]string{"1111111111111111": {"0001_initial"}}
	if _, err := Rollback(context.Background(), f, RollbackOptions{Out: &out, Drain: -1}); err == nil ||
		!strings.Contains(err.Error(), "refused: the live database has migrations build 1111111111111111 does not have (0002_add, 0003_drop): that build would refuse to start on it") {
		t.Errorf("history: %v", err)
	}
}

// The plan places the build's migrations by the live schema's hash, and refuses a live schema
// that none leads to.
func TestMakePlan(t *testing.T) {
	dir, live := appWithDrop(t)
	mig := filepath.Join(dir, "migrations")
	p, err := MakePlan(mig, live, true, true)
	if err != nil || len(p.Migrations) != 1 || p.Migrations[0].Name != "0002_drop_summary" || strings.Join(p.DropsData(), ";") != "0002_drop_summary: column Post.summary" {
		t.Fatalf("%+v %v", p, err)
	}
	_, s2 := schemas()
	j2, _ := json.Marshal(s2)
	if p, err := MakePlan(mig, j2, true, true); err != nil || len(p.Migrations) != 0 || p.Lines()[0] != "no pending migration: the live schema "+p.Live+" is the build's" {
		t.Errorf("%+v %v", p, err)
	}
	other := migrate.Schema{Format: db.Format, Tables: []migrate.Table{{Name: "Tag.Tag", ID: "id", Columns: []migrate.Column{{Name: "id", Type: db.CText}}}}}
	jo, _ := json.Marshal(other)
	if _, err := MakePlan(mig, jo, true, true); err == nil || !strings.Contains(err.Error(), "which none of this build's migrations leads to") {
		t.Errorf("err = %v", err)
	}
	if p, err := MakePlan(filepath.Join(dir, "none"), nil, false, true); err != nil || !p.NoMigrations {
		t.Errorf("%+v %v", p, err)
	}
}
