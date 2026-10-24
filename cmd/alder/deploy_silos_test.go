package main

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"alder/internal/deploy"
	"alder/internal/ir"
	"alder/internal/tursofake"
	"alder/rt/db"
	"alder/rt/web"
)

// The deploys of an app with silos (M15): testdata/corpus/silo-ok (Project in `silo org`) with
// a home page, at an Ssh box — its silos files under <path>/db/silos/ (TestDeploySsh_Silos), or
// databases of their own at Turso, the fake one (TestDeployTurso_Lifecycle).

// siloDeployApp copies silo-ok as "siloapp" (the working directory its parent), adds a home page
// and Project's `note` (dropped by a later migration in TestDeploySsh_Silos), writes migration
// 0001 and appends decl, with the box's knobs set as sshScaffold sets them.
func siloDeployApp(t *testing.T, box *sshBox, decl string) string {
	t.Helper()
	needGoTool(t)
	if box.container != "" {
		t.Skip("the silo deploys read and write the box's files from the test: a box on this machine only")
	}
	dir := tempApp(t, "../../testdata/corpus/silo-ok", "siloapp")
	os.RemoveAll(filepath.Join(dir, "build"))
	writeFile(t, filepath.Join(dir, "app", "Home.alder"), "module Home\n\npage \"/\" { }\n\ntype Model = { }\n\nload : Params -> Session -> Query Model\nload params session = Query.succeed { }\n\nview : Model -> Session -> Html Msg\nview model session = <html><body><h1>home</h1></body></html>\n")
	editApp(t, dir, "Project.alder", "  name: Text,\n", "  name: Text,\n  note: Maybe Text = Nothing,\n")
	if code, out, errs := runMigrate(t, dir); code != 0 {
		t.Fatalf("migrate: %d\n%s%s", code, out, errs)
	}
	appendTo(t, filepath.Join(dir, "app", "App.alder"), "\n"+decl+"\n")
	t.Setenv(deploy.EnvSSHConfig, box.config)
	t.Setenv(envDrain, "0")
	t.Setenv(envHealthTimeout, "30s")
	t.Setenv(deploy.EnvSupervisor, "process")
	box.sh(t, fmt.Sprintf("mkdir -p %s && echo %d > %s/port", box.qr(), box.port, box.qr()))
	return dir
}

// siloClient sends requests to the app at the box's public port as a signed-in session.
type siloClient struct {
	t    *testing.T
	base string
	c    *http.Client
}

func newSiloClient(t *testing.T, box *sshBox) *siloClient {
	return &siloClient{t: t, base: fmt.Sprintf("http://127.0.0.1:%d", box.port), c: &http.Client{Timeout: 20 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}
}

func (s *siloClient) do(method, path string, sess siloSession, form url.Values) (int, string) {
	s.t.Helper()
	cookie, err := web.SessionCookie([]byte(siloSecret), siloCodec{}, sess, time.Now())
	if err != nil {
		s.t.Fatal(err)
	}
	req, _ := http.NewRequest(method, s.base+path, strings.NewReader(form.Encode()))
	if method == http.MethodPost {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	req.AddCookie(&http.Cookie{Name: web.CookieName, Value: cookie})
	res, err := s.c.Do(req)
	if err != nil {
		s.t.Fatal(err)
	}
	defer res.Body.Close()
	b, _ := io.ReadAll(res.Body)
	return res.StatusCode, string(b)
}

// add adds project name in org's silo as user me.
func (s *siloClient) add(org, name string) {
	s.t.Helper()
	if code, body := s.do("POST", "/_m/Projects.Index/Add", siloSession{"me", org}, url.Values{"name": {name}}); code != http.StatusSeeOther {
		s.t.Fatalf("add %s in %s: %d\n%s", name, org, code, body)
	}
}

// projects is org's projects page (its status and body).
func (s *siloClient) projects(org string) (int, string) {
	s.t.Helper()
	return s.do("GET", "/projects", siloSession{"me", org}, nil)
}

// seedShared are the shared rows: two users, orgs A and B, me a member of both.
var seedShared = []string{
	`INSERT INTO "User__User" ("id", "name") VALUES ('me', 'Me'), ('them', 'Them')`,
	`INSERT INTO "Org__Org" ("id", "name") VALUES ('A', 'Org A'), ('B', 'Org B')`,
	`INSERT INTO "Membership__Membership" ("id", "user", "org") VALUES ('m1', 'me', 'A'), ('m2', 'me', 'B')`,
}

func statusOf(t *testing.T, dir string) string {
	t.Helper()
	var so, se bytes.Buffer
	if code := runStatusTo([]string{"production", dir}, &so, &se); code != 0 {
		t.Fatalf("status: %d\n%s%s", code, so.String(), se.String())
	}
	return so.String()
}

func runCmd(t *testing.T, f func([]string, io.Writer, io.Writer) int, args ...string) (int, string) {
	t.Helper()
	var so, se bytes.Buffer
	code := f(args, &so, &se)
	return code, so.String() + se.String()
}

// Silos as files at an Ssh box: each org's file under <path>/db/silos/org/, streamed one by
// one to the backup location; `alder status`'s silo table; `alder restore --silo` putting one
// silo back as it was (the others untouched); an orphaned silo (its Org deleted) listed and
// removed by `alder silos prune`; and RF3 at a box: a deploy whose migration drops a column of
// the silo's table leaves the silo `pending (data loss)` (maintenance for that org) until
// `alder migrate silos --allow-data-loss`.
func TestDeploySsh_Silos(t *testing.T) {
	if testing.Short() {
		t.Skip("a deploy end to end: real builds, app processes and drain windows")
	}
	box := startBox(t)
	dir := siloDeployApp(t, box, fmt.Sprintf(`deploy {
  production: Ssh %q { domain: "localhost", path: %q },
  proxy: None,
  keep: 3,
}`, box.host, box.root))
	bucket := filepath.Join(box.root, "bucket")
	box.write(t, "env", fmt.Sprintf("ALDER_SECRET=%s\nALDER_BACKUP_URL=%q\nALDER_BACKUP_INTERVAL=%s\nALDER_SILO_RECHECK=300ms\n", siloSecret, "file://"+bucket, siloShipEvery))
	code, out, errs := runDeploy(t, "production", dir, "--unsafe-skip-tests")
	if code != 0 {
		t.Fatalf("deploy: %d\n%s%s", code, out, errs)
	}
	shared := filepath.Join(box.root, "db", "app.db")
	for _, st := range seedShared {
		sqlExec(t, shared, st)
	}
	c := newSiloClient(t, box)
	c.add("A", "a1")
	c.add("B", "b1")
	silos := filepath.Join(box.root, "db", "silos", "org")
	if n := sqlCount(t, filepath.Join(silos, db.SiloFileName("A")), `SELECT COUNT(*) FROM "Project__Project"`); n != 1 {
		t.Fatalf("A's file holds %d projects", n)
	}
	if st := statusOf(t, dir); !strings.Contains(st, "silos    org: 2 ok, 0 migrating, 0 pending (data loss), 0 ahead, 0 failed\n") {
		t.Fatalf("status:\n%s", st)
	}

	// restore A as it was after a1 (a2 is gone from it; B untouched). The shipper is the app's
	// process, on the real clock: the point is a millisecond one (the segments' keys carry
	// milliseconds) between a1's shipment and a2
	waitShipped(t, filepath.Join(silos, db.SiloFileName("A")), time.Now().Add(siloShipEvery+100*time.Millisecond))
	at := time.Now().UTC()
	time.Sleep(20 * time.Millisecond)
	c.add("A", "a2")
	c.add("B", "b2")
	waitShipped(t, filepath.Join(silos, db.SiloFileName("A")), time.Now().Add(siloShipEvery+100*time.Millisecond))
	code, out = runCmd(t, runRestoreTo, "production", "--dir", dir, "--silo", "org:A", "--from", "file://"+bucket, "--to", at.Format(time.RFC3339Nano), "--yes")
	if code != 0 || !strings.Contains(out, "replaced silo org/A's database at Ssh alder-test-box:"+box.root+" (the previous one is kept as db/silos/org/"+db.SiloFileName("A")+".before-restore-") {
		t.Fatalf("restore --silo: %d\n%s", code, out)
	}
	if _, body := c.projects("A"); !strings.Contains(body, ">Org A: a1<") || strings.Contains(body, ">Org A: a2<") {
		t.Fatalf("A after the restore:\n%s", body)
	}
	if _, body := c.projects("B"); !strings.Contains(body, "b1") || !strings.Contains(body, "b2") {
		t.Fatalf("B after A's restore:\n%s", body)
	}

	// an orphan: B's Org deleted; listed, then pruned
	sqlExec(t, shared, `DELETE FROM "Membership__Membership" WHERE "org" = 'B'`) // as the cascade would
	sqlExec(t, shared, `DELETE FROM "Org__Org" WHERE "id" = 'B'`)
	if st := statusOf(t, dir); !strings.Contains(st, "org: 1 ok, 0 migrating, 0 pending (data loss), 0 ahead, 0 failed, 1 orphaned (`alder silos prune`)") || !strings.Contains(st, "org/B  orphaned: no Org row has this id any more") {
		t.Fatalf("status with an orphan:\n%s", st)
	}
	if code, out := runCmd(t, runSilosTo, "prune", "production", dir); code != 2 || !strings.Contains(out, "not confirmed: nothing was removed") {
		t.Fatalf("prune without --yes: %d\n%s", code, out)
	}
	if code, out := runCmd(t, runSilosTo, "prune", "production", dir, "--yes"); code != 0 || !strings.Contains(out, "  org/B\n") || !strings.Contains(out, "org/B: removed") {
		t.Fatalf("prune: %d\n%s", code, out)
	}
	if _, err := os.Stat(filepath.Join(silos, db.SiloFileName("B"))); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("B's file: %v", err)
	}

	// v2 drops Project.note: the deploy migrates the shared database; A, behind by a migration
	// that drops data, is in maintenance until `alder migrate silos --allow-data-loss`
	editApp(t, dir, "Project.alder", "  note: Maybe Text = Nothing,\n", "")
	if code, out, errs := runMigrate(t, dir); code != 0 || !strings.Contains(out, "DROPS DATA: column Project.note") {
		t.Fatalf("migrate 0002: %d\n%s%s", code, out, errs)
	}
	if code, out, errs := runDeploy(t, "production", dir, "--unsafe-skip-tests", "--allow-data-loss"); code != 0 {
		t.Fatalf("deploy v2: %d\n%s%s", code, out, errs)
	}
	if code, body := c.projects("A"); code != 503 || !strings.Contains(body, "down for maintenance") {
		t.Fatalf("A on v2: %d\n%s", code, body)
	}
	if st := statusOf(t, dir); !strings.Contains(st, "org: 0 ok, 0 migrating, 1 pending (data loss), 0 ahead, 0 failed") || !strings.Contains(st, "org/A  pending-data-loss: a pending migration DROPS DATA (0002_") {
		t.Fatalf("status on v2:\n%s", st)
	}
	if code, out := runCmd(t, runMigrateTo, "silos", "production", dir); code != 1 || !strings.Contains(out, "org/A: pending-data-loss: ") {
		t.Fatalf("migrate silos: %d\n%s", code, out)
	}
	if code, out := runCmd(t, runMigrateTo, "silos", "production", dir, "--allow-data-loss"); code != 0 || !strings.Contains(out, "org/A: migrated 0002_") {
		t.Fatalf("migrate silos --allow-data-loss: %d\n%s", code, out)
	}
	// the running app opens A again after its recheck interval (shortened for the test)
	pollUntil(5*time.Second, func() bool { code, _ := c.projects("A"); return code == 200 })
	if code, body := c.projects("A"); code != 200 || !strings.Contains(body, "a1") {
		t.Fatalf("A after migrate silos: %d\n%s", code, body)
	}
	if st := statusOf(t, dir); !strings.Contains(st, "silos    org: 1 ok, 0 migrating") {
		t.Fatalf("status after:\n%s", st)
	}
}

// siloShipEvery is the shipper's interval in TestDeploySsh_Silos (ALDER_BACKUP_INTERVAL), short
// so its waits for a shipment are.
const siloShipEvery = 200 * time.Millisecond

func irSsh(box *sshBox) ir.Target {
	return ir.Target{Kind: "ssh", Host: box.host, Domain: "localhost", Path: box.root}
}

var irDeployNone = ir.Deploy{Proxy: "None"}

// waitShipped waits until the shipper's status file of the database at path names an upload
// after after.
func waitShipped(t *testing.T, path string, after time.Time) {
	t.Helper()
	deadline := time.Now().Add(20 * time.Second)
	for {
		var st struct {
			Last time.Time `json:"last"`
		}
		b, _ := os.ReadFile(path + "-backup.json")
		json.Unmarshal(b, &st)
		if st.Last.After(after) {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("%s was not shipped after %s", path, after)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// With `backup` and more silo files than the shipper streams, the preflight refuses, pointing
// at Turso.
func TestDeploySsh_SiloBound(t *testing.T) {
	box := startBox(t)
	if box.container != "" {
		t.Skip("writes the box's files from the test")
	}
	t.Setenv(deploy.EnvSSHConfig, box.config)
	t.Setenv(deploy.EnvSupervisor, "process")
	os.MkdirAll(filepath.Join(box.root, "db", "silos", "org"), 0o700)
	for i := 0; i <= deploy.MaxSiloFiles; i++ {
		os.WriteFile(filepath.Join(box.root, "db", "silos", "org", fmt.Sprintf("s%d.db", i)), nil, 0o600)
	}
	s := deploy.NewSsh("siloapp", "production", irSsh(box), &irDeployNone)
	s.Silos, s.BackupURL, s.Proxy = []string{"org"}, "s3://bucket/x", "None"
	if _, err := s.Connect(t.Context()); err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	err := s.Preflight(t.Context())
	if err == nil || !strings.Contains(err.Error(), "db/silos holds 201 silo files, more than the 200 the backup shipper streams one by one: with this many silos the data belongs on Turso") {
		t.Fatalf("%v", err)
	}
}

// The Turso database of an Ssh environment, against the fake Turso: the first deploy creates
// the shared database (migrated from here); the app creates each org's database on first use;
// `alder status` shows the database and the silo table; RF4: a deploy whose plan fails on one of
// the sampled silo databases is refused naming it, with nothing migrated (the shared database
// still at 0001, the build before still serving); then the deploy passes, both silos are
// `migrating` (behind), one is migrated by its next request, the other by `alder migrate
// silos`; `alder restore --silo` puts one silo's database back as it was (Turso's
// point-in-time copy); `rollback --restore-backup` makes a copy of the shared database as it
// was before the migration the app's; an orphaned silo's database is pruned.
func TestDeployTurso_Lifecycle(t *testing.T) {
	if testing.Short() {
		t.Skip("a deploy end to end: real builds, app processes and drain windows")
	}
	box := startBox(t)
	f := tursofake.New(t, "acme")
	// one fake clock for the deploys' points in time and the fake's commits: the waits for the
	// next whole second advance it instead of sleeping
	clock := useFakeClock(t)
	f.SetNow(clock.Now)
	dir := siloDeployApp(t, box, fmt.Sprintf(`deploy {
  production: Ssh %q { domain: "localhost", path: %q, database: Turso "acme" { app: "siloapp", sample: 2 } },
  proxy: None,
  keep: 4,
}`, box.host, box.root))
	t.Setenv(db.EnvTursoToken, f.APIToken)
	t.Setenv(db.EnvTursoAPI, f.URL)
	box.write(t, "env", fmt.Sprintf("ALDER_SECRET=%s\nTURSO_API_TOKEN=%s\nALDER_TURSO_API=%s\n", siloSecret, f.APIToken, f.URL))
	code, out, errs := runDeploy(t, "production", dir, "--unsafe-skip-tests")
	if code != 0 {
		t.Fatalf("deploy: %d\n%s%s", code, out, errs)
	}
	id1 := deployedID(t, out)
	for _, want := range []string{"alder deploy production → Ssh alder-test-box:" + box.root + ", database Turso acme/siloapp\n", "  requires ok  ALDER_SECRET\n", "first deploy: the database is created by every migration", "  migrate  ok  applied 0001_initial\n"} {
		if !strings.Contains(out, want) {
			t.Errorf("first deploy lacks %q:\n%s", want, out)
		}
	}
	if names := strings.Join(f.Names(), " "); names != "siloapp" {
		t.Fatalf("databases after the first deploy: %s", names)
	}
	if env := box.sh(t, "cat "+box.qr()+"/db/turso.env"); !strings.Contains(env, "ALDER_DB=libsql://127.0.0.1:") || !strings.Contains(env, "ALDER_TURSO_DB=siloapp\n") {
		t.Fatalf("turso.env: %s", env)
	}
	for _, st := range seedShared {
		if err := f.Exec("siloapp", st); err != nil {
			t.Fatal(err)
		}
	}
	c := newSiloClient(t, box)
	c.add("A", "a1")
	c.add("A", "a2")
	c.add("B", "b1")
	childA, childB := db.ChildName("siloapp", "org", "A"), db.ChildName("siloapp", "org", "B")
	var n int
	f.DB(childA).QueryRow(`SELECT COUNT(*) FROM "Project__Project"`).Scan(&n)
	if n != 2 {
		t.Fatalf("A's database holds %d projects", n)
	}
	st := statusOf(t, dir)
	if !strings.Contains(st, "silos    org: 2 ok, 0 migrating, 0 pending (data loss), 0 ahead, 0 failed\n") || !strings.Contains(st, "continuous: Turso's point-in-time recovery of Turso acme/siloapp") {
		t.Fatalf("status:\n%s", st)
	}

	// v2 adds Project.due. RF4: B's database was changed by hand, and the plan fails on it
	editApp(t, dir, "Project.alder", "  org: Org.OrgId @ref Org @partition org,\n", "  org: Org.OrgId @ref Org @partition org,\n  due: Maybe Text = Nothing,\n")
	if code, out, errs := runMigrate(t, dir); code != 0 {
		t.Fatalf("migrate 0002: %d\n%s%s", code, out, errs)
	}
	f.Exec(childB, `CREATE TABLE "Stray" ("x" TEXT)`)
	code, out, errs = runDeploy(t, "production", dir, "--unsafe-skip-tests")
	if code != 1 || !strings.Contains(errs, "refused: the migrations fail on 1 of the largest silo databases they were tried on (each in a transaction, rolled back): nothing was migrated or switched, and the shared database is as it was") ||
		!strings.Contains(errs, "org/B (database "+childB+"): FAIL: after the migrations its tables") {
		t.Fatalf("RF4: %d\n%s%s", code, out, errs)
	}
	if got := box.serving(t); got != id1 {
		t.Fatalf("after RF4 the box serves %q, want %s", got, id1)
	}
	f.DB("siloapp").QueryRow(`SELECT COUNT(*) FROM "_alder_migrations"`).Scan(&n)
	if n != 1 {
		t.Fatalf("after RF4 the shared database has %d migrations applied", n)
	}
	if code, body := c.projects("A"); code != 200 || !strings.Contains(body, "a1") {
		t.Fatalf("A after RF4: %d\n%s", code, body)
	}

	// the deploy passes; both silos are behind: A is migrated by its next request (on open),
	// B by `alder migrate silos`
	f.Exec(childB, `DROP TABLE "Stray"`)
	code, out, errs = runDeploy(t, "production", dir, "--unsafe-skip-tests")
	if code != 0 || !strings.Contains(out, "  migrate  ok  applied 0002_") {
		t.Fatalf("deploy v2: %d\n%s%s", code, out, errs)
	}
	id2 := deployedID(t, out)
	if st := statusOf(t, dir); !strings.Contains(st, "org: 0 ok, 2 migrating, 0 pending (data loss), 0 ahead, 0 failed") {
		t.Fatalf("status after the switch:\n%s", st)
	}
	if code, body := c.projects("A"); code != 200 || !strings.Contains(body, "a1") {
		t.Fatalf("A on v2: %d\n%s", code, body)
	}
	if st := statusOf(t, dir); !strings.Contains(st, "org: 1 ok, 1 migrating") || !strings.Contains(st, "org/B  migrating") {
		t.Fatalf("status with B behind:\n%s", st)
	}
	if code, out := runCmd(t, runMigrateTo, "silos", "production", dir); code != 0 || !strings.Contains(out, "org/B: migrated 0002_") || !strings.Contains(out, "org/A: ok") {
		t.Fatalf("migrate silos: %d\n%s", code, out)
	}
	if st := statusOf(t, dir); !strings.Contains(st, "org: 2 ok, 0 migrating") {
		t.Fatalf("status after migrate silos:\n%s", st)
	}
	if code, body := c.projects("B"); code != 200 || !strings.Contains(body, "b1") {
		t.Fatalf("B on v2: %d\n%s", code, body)
	}

	// restore A as it was before a3 (Turso's point in time)
	clock.Sleep(1100 * time.Millisecond)
	before := clock.Now().UTC().Truncate(time.Second)
	clock.Sleep(1100 * time.Millisecond)
	c.add("A", "a3")
	code, out = runCmd(t, runRestoreTo, "production", "--dir", dir, "--silo", "org:A", "--to", before.Format(time.RFC3339), "--yes")
	if code != 0 || !strings.Contains(out, "put silo org/A's database back as it was at "+before.Format(time.RFC3339)) {
		t.Fatalf("restore --silo: %d\n%s", code, out)
	}
	if _, body := c.projects("A"); !strings.Contains(body, ">Org A: a2<") || strings.Contains(body, ">Org A: a3<") {
		t.Fatalf("A after the restore:\n%s", body)
	}
	if code, out := runCmd(t, runRestoreTo, "production", "--dir", dir, "--from", "s3://x/y", "--to", before.Format(time.RFC3339)); code != 2 || !strings.Contains(out, "is on Turso: it is restored to a point in time with --to") {
		t.Fatalf("restore --from on Turso: %d\n%s", code, out)
	}

	// an orphan: B's Org deleted at the shared database; pruned
	f.Exec("siloapp", `DELETE FROM "Membership__Membership" WHERE "org" = 'B'`)
	f.Exec("siloapp", `DELETE FROM "Org__Org" WHERE "id" = 'B'`)
	if code, out := runCmd(t, runSilosTo, "prune", "production", dir, "--yes"); code != 0 || !strings.Contains(out, "org/B: removed") {
		t.Fatalf("prune: %d\n%s", code, out)
	}
	if strings.Contains(strings.Join(f.Names(), " "), childB) {
		t.Fatal("B's database is still at Turso")
	}

	// rollback --allow-schema-mismatch to v1 on the database at 0002: v1 starts and serves only
	// its maintenance page (503, with its own build id in the alder-build meta); the health check
	// requires a 200, so it fails and v2 keeps serving
	t.Setenv("ALDER_DEPLOY_HEALTH_TIMEOUT", "4s")
	code, out = runCmd(t, runRollbackTo, "production", id1, "--dir", dir, "--allow-schema-mismatch", "--yes")
	if code == 0 || !strings.Contains(out, "answers GET / with status 503") {
		t.Fatalf("rollback --allow-schema-mismatch onto the maintenance page: %d\n%s", code, out)
	}
	if got := box.serving(t); got != id2 {
		t.Fatalf("after the refused rollback the box serves %q, want %s", got, id2)
	}
	if st := statusOf(t, dir); !strings.Contains(st, id2) {
		t.Fatalf("status after the refused rollback:\n%s", st)
	}

	// rollback --restore-backup: the shared database as it was before 0002 (Turso's point in
	// time recorded in builds/<v2>/backup.pitr) becomes the app's; the replaced one is kept
	code, out = runCmd(t, runRollbackTo, "production", id1, "--dir", dir, "--restore-backup", "--yes")
	if code != 0 || !strings.Contains(out, "rolled back to "+id1+" with the data of ") {
		t.Fatalf("rollback --restore-backup: %d\n%s", code, out)
	}
	if env := box.sh(t, "cat "+box.qr()+"/db/turso.env"); !strings.Contains(env, "ALDER_TURSO_DB=siloapp-r") {
		t.Fatalf("turso.env after the rollback: %s", env)
	}
	if got := box.serving(t); got != id1 {
		t.Fatalf("after the rollback the box serves %q, want %s", got, id1)
	}
	_ = id2
	if !strings.Contains(strings.Join(f.Names(), " "), "siloapp ") {
		t.Fatalf("the replaced shared database was not kept: %v", f.Names())
	}
	var shared *sql.DB
	for _, n := range f.Names() {
		if strings.HasPrefix(n, "siloapp-r") {
			shared = f.DB(n)
		}
	}
	shared.QueryRow(`SELECT COUNT(*) FROM "_alder_migrations"`).Scan(&n)
	if n != 1 {
		t.Fatalf("the restored shared database has %d migrations applied, want 1 (before 0002)", n)
	}
}
