package main

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"alder/internal/tursofake"
	"alder/rt/db"
)

// TestStartCheck_Binary serves testdata/corpus/silo-ok (the M15 silo fixture, with a home page)
// with ALDER_DB=libsql://… at the fake Turso: the binary checks the shared database instead of
// migrating it (several instances would race). Two builds: v1 (0001) and v2 (0002 adds
// Project.due).
//   - Behind: the database at 0001, v2 exits 1 with "the database is behind this build: run
//     `alder deploy`", and nothing is migrated.
//   - Ahead: the database at 0002, v1 serves the maintenance page (503) on every route and
//     /_alder/health answers 503 "ahead: …".
//   - Equal: v2 serves, and /_alder/health answers 200 with ALDER_BUILD; with the fake closed,
//     503 {"db":"unreachable"} (no host, no driver error).
//   - FileStillMigrates: v2 with a file ALDER_DB migrates it on start (M14 unchanged).
func TestStartCheck_Binary(t *testing.T) {
	// serial: tempApp t.Chdir()s to the copy's parent, and the test names the app by a relative path
	needGoTool(t)
	if testing.Short() {
		t.Skip("builds two binaries")
	}
	dir := tempApp(t, "../../testdata/corpus/silo-ok", "siloapp")
	tmp, _ := os.Getwd()
	os.RemoveAll(filepath.Join(dir, "build"))
	writeFile(t, filepath.Join(dir, "app", "Home.alder"), "module Home\n\npage \"/\" { }\n\ntype Model = { }\n\nload : Params -> Session -> Query Model\nload params session = Query.succeed { }\n\nview : Model -> Session -> Html Msg\nview model session = <html><body><h1>home</h1></body></html>\n")
	build := func(name string) string {
		t.Helper()
		if code, out, errs := runMigrate(t, dir); code != 0 {
			t.Fatalf("migrate %s: %d\n%s%s", name, code, out, errs)
		}
		if code, out, errs := runBuild(t, dir); code != 0 {
			t.Fatalf("build %s: %d\n%s%s", name, code, out, errs)
		}
		bin := filepath.Join(tmp, name)
		if err := os.Rename(filepath.Join(testBuildDir(t), dir, dir), bin); err != nil {
			t.Fatal(err)
		}
		return bin
	}
	v1 := build("v1")
	editApp(t, dir, "Project.alder", "  name: Text,\n", "  name: Text,\n  due: Maybe Text = Nothing,\n")
	v2 := build("v2")

	f := tursofake.New(t, "acme")
	t.Setenv(db.EnvTursoAPI, f.URL)
	t.Setenv(db.EnvTursoOrg, f.Org)
	t.Setenv(db.EnvTursoGroup, "default")
	t.Setenv(db.EnvTursoApp, "siloapp")
	t.Setenv(db.EnvTursoToken, f.APIToken)
	t.Setenv(db.EnvTursoDB, "siloapp")
	t.Setenv(db.EnvDBToken, "")
	if err := f.Create("siloapp", "default"); err != nil {
		t.Fatal(err)
	}
	d, ok, err := db.NewTurso(db.TursoConfigFromEnv()).Database(context.Background(), "siloapp")
	if err != nil || !ok {
		t.Fatalf("%v %v", ok, err)
	}
	remote := "libsql://" + d.Hostname
	base := []string{"ALDER_DEV=", "ALDER_SECRET=" + siloSecret, "ALDER_ADDR=127.0.0.1:0"}
	run := func(bin, dbPath string, args ...string) (int, string) {
		t.Helper()
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, bin, args...)
		cmd.Env = append(append(os.Environ(), base...), "ALDER_DB="+dbPath)
		out, err := cmd.CombinedOutput()
		if ctx.Err() != nil {
			t.Fatalf("%s %v still running after 60 s (it served?):\n%s", bin, args, out)
		}
		code := 0
		if ee, ok := err.(*exec.ExitError); ok {
			code = ee.ExitCode()
		} else if err != nil {
			t.Fatal(err)
		}
		return code, string(out)
	}
	// serveBin starts bin as build/siloapp/siloapp (a new file each time: macOS kills a process
	// whose executable was rewritten in place), stopped when t ends
	serveBin := func(t *testing.T, bin string, env ...string) *running {
		t.Helper()
		exe := filepath.Join(tmp, "build", "siloapp", "siloapp")
		os.MkdirAll(filepath.Dir(exe), 0o755)
		os.Remove(exe)
		b, err := os.ReadFile(bin)
		if err == nil {
			err = os.WriteFile(exe, b, 0o755)
		}
		if err != nil {
			t.Fatal(err)
		}
		return startAppNamed(t, tmp, "siloapp", append(base, env...)...)
	}
	get := func(t *testing.T, app *running, path string) (int, string) {
		t.Helper()
		res, err := http.Get("http://" + app.addr + path)
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()
		b, _ := io.ReadAll(res.Body)
		return res.StatusCode, string(b)
	}
	recorded := func() string {
		t.Helper()
		rows, err := f.DB("siloapp").Query(`SELECT "name" FROM "_alder_migrations" ORDER BY "name"`)
		if err != nil {
			t.Fatal(err)
		}
		defer rows.Close()
		var names []string
		for rows.Next() {
			var n string
			rows.Scan(&n)
			names = append(names, n)
		}
		return strings.Join(names, " ")
	}
	if code, out := run(v1, remote, "--migrate-only"); code != 0 {
		t.Fatalf("v1 --migrate-only: %d\n%s", code, out)
	}

	t.Run("Behind", func(t *testing.T) {
		code, out := run(v2, remote)
		if code != 1 || !strings.Contains(out, "alder: the database is behind this build: run `alder deploy`\n") {
			t.Fatalf("%d\n%s", code, out)
		}
		if got := recorded(); got != "0001_initial" {
			t.Fatalf("recorded after the refusal: %q", got)
		}
	})

	if code, out := run(v2, remote, "--migrate-only"); code != 0 {
		t.Fatalf("v2 --migrate-only: %d\n%s", code, out)
	}
	t.Run("Ahead", func(t *testing.T) {
		app := serveBin(t, v1, "ALDER_DB="+remote, "ALDER_BUILD=b1")
		for _, path := range []string{"/", "/projects", "/nowhere"} {
			if code, body := get(t, app, path); code != 503 || !strings.Contains(body, "down for maintenance") { // App.maintenance
				t.Fatalf("%s: %d\n%s", path, code, body)
			}
		}
		code, body := get(t, app, "/_alder/health")
		var h map[string]string
		json.Unmarshal([]byte(body), &h)
		if code != 503 || h["build"] != "b1" || !strings.HasPrefix(h["db"], "ahead: a newer build migrated it (to 0002_") || !strings.HasSuffix(h["db"], ", which this build does not have)") {
			t.Fatalf("health: %d %s", code, body)
		}
		if strings.Contains(body, d.Hostname) || strings.Contains(body, "libsql") || strings.Contains(body, f.URL) {
			t.Fatalf("health names the database: %s", body)
		}
		if got := recorded(); !strings.HasPrefix(got, "0001_initial 0002_") {
			t.Fatalf("recorded: %q", got)
		}
	})

	t.Run("Equal", func(t *testing.T) {
		app := serveBin(t, v2, "ALDER_DB="+remote, "ALDER_BUILD=b2")
		if code, body := get(t, app, "/_alder/health"); code != 200 || body != `{"build":"b2","db":"ok"}`+"\n" {
			t.Fatalf("health: %d %q", code, body)
		}
		if code, body := get(t, app, "/"); code != 200 || !strings.Contains(body, "home") {
			t.Fatalf("home: %d\n%s", code, body)
		}
		// the database goes away: 503 "unreachable", and nothing of the driver's error
		f.Close()
		if code, body := get(t, app, "/_alder/health"); code != 503 || body != `{"build":"b2","db":"unreachable"}`+"\n" {
			t.Fatalf("health with the database gone: %d %q", code, body)
		}
	})

	t.Run("FileStillMigrates", func(t *testing.T) {
		file := filepath.Join(t.TempDir(), "app.db")
		app := serveBin(t, v2, "ALDER_DB="+file)
		if code, _ := get(t, app, "/_alder/health"); code != 200 {
			t.Fatalf("health: %d", code)
		}
		if n := sqlCount(t, file, `SELECT COUNT(*) FROM "_alder_migrations"`); n != 2 {
			t.Fatalf("the file has %d migrations recorded", n)
		}
	})
}
