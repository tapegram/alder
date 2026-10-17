package main

import (
	"database/sql"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"alder/rt"
	"alder/rt/web"

	_ "modernc.org/sqlite"
)

// siloCodec encodes the silo-ok app's session as its generated codec does (a JSON object of the
// Go field names), so the test signs its own cookies.
type siloCodec struct{}

type siloSession struct{ user, org string }

func (siloCodec) Default() any { return siloSession{} }

func (siloCodec) Encode(v any) ([]byte, error) {
	s := v.(siloSession)
	m := func(x string) rt.Maybe[string] {
		if x == "" {
			return rt.Nothing[string]()
		}
		return rt.Just(x)
	}
	return json.Marshal(map[string]any{"User": m(s.user), "Org": m(s.org), "Role": "Reader"})
}

func (siloCodec) Decode([]byte) (any, error) { return siloSession{}, nil }

const siloSecret = "0123456789abcdef-silos"

// TestSilos_Binary builds testdata/corpus/silo-ok at three versions — 0001 (Project with a
// `note`), 0002 (drops it: DROPS DATA), 0003 (adds `due`) — and checks the binary of the last
// one: --silo-status over a directory with one silo behind (made by 0002's build: migrates),
// one behind by the data-dropping 0002 (made by 0001's: pending-data-loss) and one ahead (a
// newer build's migration); then serves: two orgs see only their own projects (and B's file lacks
// A's row), a crafted switch to a non-member org is refused, the data-loss straggler and the
// ahead silo answer the maintenance page (503) while the others serve, and --migrate-silos
// [--allow-data-loss] fixes the first.
func TestSilos_Binary(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go is not on PATH")
	}
	if testing.Short() {
		t.Skip("builds three binaries")
	}
	dir := tempApp(t, "../../testdata/corpus/silo-ok", "siloapp")
	tmp, _ := os.Getwd()
	os.RemoveAll(filepath.Join(dir, "build"))
	build := func(name string) string {
		t.Helper()
		if code, out, errs := runBuild(t, dir); code != 0 {
			t.Fatalf("build %s: %d\n%s%s", name, code, out, errs)
		}
		bin := filepath.Join(tmp, name)
		if err := os.Rename(filepath.Join(tmp, "build", dir, dir), bin); err != nil {
			t.Fatal(err)
		}
		return bin
	}
	editApp(t, dir, "Project.alder", "  name: Text,\n", "  name: Text,\n  note: Maybe Text = Nothing,\n")
	mustMigrate(t, "wrote siloapp/migrations/0001_initial.sql\n  create table Label\n  create table Membership\n  create table Note\n  create table Org\n  create table Project\n  create table User\n  add index on Label.org\n  add index on Membership.org\n  add index on Membership.user\n  add index on Note.owner\n  add index on Project.org\nupdated siloapp/migrations/schema.json\n", dir)
	v1 := build("v1")
	_ = v1
	editApp(t, dir, "Project.alder", "  note: Maybe Text = Nothing,\n", "")
	if code, out, errs := runMigrate(t, dir); code != 0 || !strings.Contains(out, "DROPS DATA: column Project.note") {
		t.Fatalf("migrate 0002: %d\n%s%s", code, out, errs)
	}
	v2 := build("v2")
	editApp(t, dir, "Project.alder", "  name: Text,\n", "  name: Text,\n  due: Maybe Text = Nothing,\n")
	if code, out, errs := runMigrate(t, dir); code != 0 || !strings.Contains(out, "Project.due added") {
		t.Fatalf("migrate 0003: %d\n%s%s", code, out, errs)
	}
	v3 := build("v3")

	data := filepath.Join(tmp, "data")
	silos := filepath.Join(data, "silos", "org")
	os.MkdirAll(silos, 0o755)
	appDB := filepath.Join(data, "app.db")
	maintainAt := func(db, bin string, args ...string) (int, string) {
		t.Helper()
		cmd := exec.Command(bin, args...)
		cmd.Env = append(os.Environ(), "ALDER_DB="+db, "ALDER_DEV=", "ALDER_SECRET="+siloSecret)
		out, err := cmd.CombinedOutput()
		code := 0
		if ee, ok := err.(*exec.ExitError); ok {
			code = ee.ExitCode()
		} else if err != nil {
			t.Fatal(err)
		}
		return code, string(out)
	}
	maintain := func(bin string, args ...string) (int, string) { return maintainAt(appDB, bin, args...) }
	// an empty file is a new silo: --migrate-silos creates it at that build's schema
	makeSilo := func(bin, id string) {
		t.Helper()
		stage := t.TempDir()
		os.MkdirAll(filepath.Join(stage, "silos", "org"), 0o755)
		os.WriteFile(filepath.Join(stage, "silos", "org", id+".db"), nil, 0o644)
		if code, out := maintainAt(filepath.Join(stage, "app.db"), bin, "--migrate-silos"); code != 0 || out != "org/"+id+": ok\n" {
			t.Fatalf("making silo %s: %d %s", id, code, out)
		}
		if err := os.Rename(filepath.Join(stage, "silos", "org", id+".db"), filepath.Join(silos, id+".db")); err != nil {
			t.Fatal(err)
		}
	}
	makeSilo(v1, "loss")
	makeSilo(v2, "behind")
	makeSilo(v3, "ahead")
	sqlExec(t, filepath.Join(silos, "ahead.db"), `UPDATE "_alder_schema" SET "hash" = 'f00d', "migration" = '9999_future'`)

	code, out := maintain(v3, "--silo-status")
	want := []string{
		`{"key":"org","id":"ahead","state":"ahead","detail":"a newer build migrated it (to 9999_future, which this build does not have): deploy that build again, or restore the silo from a backup taken before it (` + "`alder restore <env> --silo org:<id> --to <time>`" + `)"}`,
		`{"key":"org","id":"behind","state":"migrating"}`,
		`{"key":"org","id":"loss","state":"pending-data-loss","detail":"a pending migration DROPS DATA (0002_drop_note: column Project.note): take a backup and run ` + "`alder migrate silos <env> --allow-data-loss`" + `"}`,
	}
	if code != 0 || strings.TrimSpace(out) != strings.Join(want, "\n") {
		t.Fatalf("--silo-status: %d\n%s\nwant\n%s", code, out, strings.Join(want, "\n"))
	}

	// serve the newest build
	os.MkdirAll(filepath.Join(tmp, "build", "siloapp"), 0o755)
	if b, err := os.ReadFile(v3); err != nil || os.WriteFile(filepath.Join(tmp, "build", "siloapp", "siloapp"), b, 0o755) != nil {
		t.Fatal(err)
	}
	app := startAppNamed(t, tmp, "siloapp", "ALDER_DB="+appDB, "ALDER_DEV=", "ALDER_SECRET="+siloSecret)
	for _, st := range []string{
		`INSERT INTO "User__User" ("id", "name") VALUES ('me', 'Me'), ('them', 'Them')`,
		`INSERT INTO "Org__Org" ("id", "name") VALUES ('A', 'Org A'), ('B', 'Org B'), ('loss', 'L'), ('behind', 'Be'), ('ahead', 'Ah')`,
		`INSERT INTO "Membership__Membership" ("id", "user", "org") VALUES ('m1', 'me', 'A'), ('m2', 'them', 'B'), ('m3', 'me', 'loss'), ('m4', 'me', 'behind'), ('m5', 'me', 'ahead')`,
	} {
		sqlExec(t, appDB, st)
	}
	c := &http.Client{Timeout: 10 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	do := func(method, path string, s siloSession, form url.Values) (*http.Response, string) {
		t.Helper()
		cookie, err := web.SessionCookie([]byte(siloSecret), siloCodec{}, s, time.Now())
		if err != nil {
			t.Fatal(err)
		}
		req, _ := http.NewRequest(method, "http://"+app.addr+path, strings.NewReader(form.Encode()))
		if method == http.MethodPost {
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		}
		req.AddCookie(&http.Cookie{Name: web.CookieName, Value: cookie})
		res, err := c.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()
		b, _ := io.ReadAll(res.Body)
		return res, string(b)
	}
	me, them := siloSession{"me", "A"}, siloSession{"them", "B"}
	if res, body := do("POST", "/_m/Projects.Index/Add", me, url.Values{"name": {"mine"}}); res.StatusCode != http.StatusSeeOther {
		t.Fatalf("add: %d\n%s", res.StatusCode, body)
	}
	if res, body := do("GET", "/projects", me, nil); res.StatusCode != 200 || !strings.Contains(body, "Org A: mine") {
		t.Fatalf("A's page: %d\n%s", res.StatusCode, body)
	}
	if res, body := do("GET", "/projects", them, nil); res.StatusCode != 200 || strings.Contains(body, "mine") || !strings.Contains(body, "Org B") {
		t.Fatalf("B's page: %d\n%s", res.StatusCode, body)
	}
	if n := sqlCount(t, filepath.Join(silos, "A.db"), `SELECT COUNT(*) FROM "Project__Project" WHERE "name" = 'Org A: mine'`); n != 1 {
		t.Fatalf("A's file holds %d", n)
	}
	if n := sqlCount(t, filepath.Join(silos, "B.db"), `SELECT COUNT(*) FROM "Project__Project"`); n != 0 {
		t.Fatalf("B's file holds %d projects", n)
	}
	if n := sqlCount(t, appDB, `SELECT COUNT(*) FROM "Project__Project"`); n != 0 {
		t.Fatalf("the shared file holds %d projects", n)
	}
	// a crafted switch to an org me is no member of: refused, the session unchanged
	if res, _ := do("POST", "/_m/Orgs/Pick?org=B", me, nil); res.StatusCode != http.StatusInternalServerError || res.Header.Get("Set-Cookie") != "" {
		t.Fatalf("crafted switch: %d %q", res.StatusCode, res.Header.Get("Set-Cookie"))
	}
	if !strings.Contains(app.stderr.String(), "Session.put org: no Membership for this user and org") {
		t.Fatalf("log:\n%s", app.stderr.String())
	}
	// the stragglers: maintenance for those two orgs only; the one behind was migrated on open
	for org, status := range map[string]int{"loss": 503, "ahead": 503, "behind": 200, "A": 200} {
		res, body := do("GET", "/projects", siloSession{"me", org}, nil)
		if res.StatusCode != status || status == 503 && (res.Header.Get("Alder-Status") != "503" || !strings.Contains(body, "down for maintenance")) {
			t.Fatalf("org %s: %d %q\n%s", org, res.StatusCode, res.Header.Get("Alder-Status"), body)
		}
	}
	if code := app.stop(t, os.Interrupt); code != 0 {
		t.Fatalf("exit %d\n%s", code, app.stderr.String())
	}
	// alder migrate silos: refused without --allow-data-loss, applied with it; ahead stays
	code, out = maintain(v3, "--migrate-silos")
	if code != 1 || !strings.Contains(out, "org/loss: pending-data-loss: ") || !strings.Contains(out, "org/behind: ok\n") || !strings.Contains(out, "org/ahead: ahead: ") {
		t.Fatalf("--migrate-silos: %d\n%s", code, out)
	}
	code, out = maintain(v3, "--migrate-silos", "--allow-data-loss")
	if code != 1 || !strings.Contains(out, "org/loss: migrated 0002_drop_note, 0003_change_project\n") || !strings.Contains(out, "alder: 1 silo(s) not migrated") {
		t.Fatalf("--migrate-silos --allow-data-loss: %d\n%s", code, out)
	}
	_, out = maintain(v3, "--silo-status")
	if strings.Count(out, `"state":"ok"`) != 4 || !strings.Contains(out, `"id":"ahead","state":"ahead"`) {
		t.Fatalf("--silo-status after:\n%s", out)
	}
	// the build before it sees the migrated silos as ahead
	if _, out := maintain(v2, "--silo-status"); !strings.Contains(out, `"id":"A","state":"ahead"`) {
		t.Fatalf("v2 --silo-status:\n%s", out)
	}
}

// sqlCount is the integer stmt selects from the SQLite file path.
func sqlCount(t *testing.T, path, stmt string) int {
	t.Helper()
	d, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	var n int
	if err := d.QueryRow(stmt).Scan(&n); err != nil {
		t.Fatalf("%s: %v", stmt, err)
	}
	return n
}

// sqlExec runs stmt on the SQLite file path.
func sqlExec(t *testing.T, path, stmt string) {
	t.Helper()
	d, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	if _, err := d.Exec(stmt); err != nil {
		t.Fatalf("%s: %v", stmt, err)
	}
}
